// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package clips

import (
	"context"
	"errors"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var samplePattern = regexp.MustCompile(`nb_samples:([0-9]+)`)
var ratePattern = regexp.MustCompile(`rate:([0-9]+)`)
var ptsPattern = regexp.MustCompile(`pts_time:([+-]?[0-9]+(?:\.[0-9]+)?)`)

type renderLog struct {
	audioFirst, audioLast    float64
	audioSamples, sampleRate int64
	pending                  string
	first, last, maxGap      float64
	frames                   int64
	bad                      bool
}

func (w *renderLog) Write(p []byte) (int, error) {
	w.pending += string(p)
	for {
		n := strings.IndexByte(w.pending, '\n')
		if n < 0 {
			break
		}
		line := w.pending[:n]
		w.pending = w.pending[n+1:]
		if strings.Contains(line, "ashowinfo") {
			pts := ptsPattern.FindStringSubmatch(line)
			samples := samplePattern.FindStringSubmatch(line)
			rate := ratePattern.FindStringSubmatch(line)
			if len(pts) == 2 && len(samples) == 2 && len(rate) == 2 {
				v, _ := strconv.ParseFloat(pts[1], 64)
				n, _ := strconv.ParseInt(samples[1], 10, 64)
				r, _ := strconv.ParseInt(rate[1], 10, 64)
				if r <= 0 || n <= 0 {
					w.bad = true
				} else {
					if w.audioSamples == 0 {
						w.audioFirst = v
						w.sampleRate = r
					}
					if w.sampleRate != r {
						w.bad = true
					}
					w.audioLast = v + float64(n-1)/float64(r)
					w.audioSamples += n
				}
			}
		} else if strings.Contains(line, "showinfo") && strings.Contains(line, " n:") {
			m := ptsPattern.FindStringSubmatch(line)
			if len(m) == 2 {
				pts, e := strconv.ParseFloat(m[1], 64)
				if e != nil || math.IsNaN(pts) || math.IsInf(pts, 0) {
					w.bad = true
					continue
				}
				if w.frames == 0 {
					w.first = pts
				} else {
					if pts < w.last {
						w.bad = true
					}
					w.maxGap = math.Max(w.maxGap, pts-w.last)
				}
				w.last = pts
				w.frames++
			}
		}
	}
	if len(w.pending) > 65536 {
		w.pending = ""
	}
	return len(p), nil
}

type processProgress struct {
	pending  string
	duration float64
	state    core.ClipState
	observer func(core.ClipProgress)
	lastTime float64
}

func (w *processProgress) Write(p []byte) (int, error) {
	w.pending += string(p)
	for {
		n := strings.IndexByte(w.pending, '\n')
		if n < 0 {
			break
		}
		line := w.pending[:n]
		w.pending = w.pending[n+1:]
		if strings.HasPrefix(line, "out_time_us=") {
			v, e := strconv.ParseFloat(strings.TrimPrefix(line, "out_time_us="), 64)
			if e == nil && v >= 0 {
				w.lastTime = v / 1e6
				percent := math.Min(99.9, 100*w.lastTime/w.duration)
				m := core.Metrics{Percent: &percent, At: time.Now()}
				if w.observer != nil {
					w.observer(core.ClipProgress{State: w.state, Metrics: m})
				}
			}
		}
	}
	if len(w.pending) > 8192 {
		w.pending = ""
	}
	return len(p), nil
}
func (b *Backend) Render(ctx context.Context, source, candidate string, plan core.ClipPlan, segment core.ClipSegment, observer func(core.ClipProgress)) (core.ClipEvidence, error) {
	ev := core.ClipEvidence{Pipeline: core.ClipPipelineVersion}
	if e := checkSpace(filepath.Dir(candidate)); e != nil {
		return ev, e
	}
	p, e := b.probe(ctx, source, false)
	if e != nil {
		return ev, e
	}
	desc, origin, e := describe(p)
	if e != nil {
		return ev, e
	}
	desc.Identity = "prepared"
	if e = core.ValidateClipSource(desc, core.ClipRequest{Mode: plan.Request.Mode, Segments: []core.ClipSegment{segment}}); e != nil {
		return ev, e
	}
	start := origin + float64(segment.Range.StartMS)/1000
	end := origin + float64(segment.Range.EndMS)/1000
	duration := float64(segment.Range.EndMS-segment.Range.StartMS) / 1000
	audio := fmt.Sprintf("[0:a:0]atrim=start=%.6f:end=%.6f,ashowinfo,asetpts=PTS-%.6f/TB[a]", start, end, start)
	args := []string{"-hide_banner", "-nostdin", "-v", "info", "-nostats", "-y", "-copyts", "-i", source}
	if plan.Request.Mode == "video" {
		video := fmt.Sprintf("[0:v:0]trim=start=%.6f:end=%.6f,showinfo,setpts=PTS-%.6f/TB", start, end, start)
		if plan.Request.Quality != "best" {
			height, _ := strconv.Atoi(plan.Request.Quality)
			if desc.Height > height {
				video += fmt.Sprintf(",scale=-2:%d", height-height%2)
			}
		}
		video += ",pad=ceil(iw/2)*2:ceil(ih/2)*2[v]"
		args = append(args, "-filter_complex", video+";"+audio, "-map", "[v]", "-map", "[a]", "-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p", "-fps_mode", "passthrough", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart")
	} else {
		args = append(args, "-filter_complex", audio, "-map", "[a]")
		switch plan.Request.Audio {
		case "m4a":
			args = append(args, "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart")
		case "mp3":
			args = append(args, "-c:a", "libmp3lame", "-q:a", "2")
		case "opus":
			args = append(args, "-c:a", "libopus", "-b:a", "160k")
		case "flac":
			args = append(args, "-c:a", "flac")
		case "wav":
			args = append(args, "-c:a", "pcm_s16le")
		default:
			return ev, errors.New("unsupported clip audio format")
		}
	}
	args = append(args, "-map_metadata", "-1", "-map_chapters", "-1", "-progress", "pipe:1", candidate)
	log := &renderLog{}
	progress := &processProgress{duration: duration, state: core.ClipRendering, observer: observer}
	c := command(ctx, b.FFmpeg, args...)
	c.Stdout = progress
	c.Stderr = log
	if e = c.Run(); e != nil {
		if ctx.Err() != nil {
			return ev, ctx.Err()
		}
		return ev, errors.New("precise clip rendering failed; check the source and FFmpeg encoders")
	}
	if log.bad || log.audioSamples == 0 || log.audioFirst < start-0.0001 || log.audioLast >= end+0.0001 {
		return ev, core.ContractError("rendered audio does not establish requested source sample boundaries")
	}
	if plan.Request.Mode == "video" && (log.bad || log.frames == 0 || log.first < start-0.00001 || log.last >= end+0.00001) {
		return ev, core.ContractError("rendered video does not establish requested source boundaries")
	}
	if observer != nil {
		observer(core.ClipProgress{State: core.ClipVerifying, Metrics: core.Metrics{At: time.Now()}})
	}
	// Decode every selected stream. Decoder warnings promoted by -xerror fail the
	// candidate. No output is published by this adapter.
	c = command(ctx, b.FFmpeg, "-hide_banner", "-nostdin", "-v", "error", "-xerror", "-i", candidate, "-map", "0:a:0", "-map", "0:v:0?", "-progress", "pipe:1", "-f", "null", "-")
	c.Stdout = &processProgress{duration: duration, state: core.ClipVerifying, observer: observer}
	var diagnostics tail
	c.Stderr = &diagnostics
	if e = c.Run(); e != nil {
		if ctx.Err() != nil {
			return ev, ctx.Err()
		}
		return ev, errors.New("clip failed full-decode verification")
	}
	output, e := b.probe(ctx, candidate, true)
	if e != nil {
		return ev, e
	}
	actual, _, e := describe(output)
	if e != nil {
		return ev, e
	}
	if !actual.Audio || plan.Request.Mode == "video" && !actual.Video {
		return ev, errors.New("clip output is missing a required stream")
	}
	for _, stream := range output.Streams {
		if stream.Type == "video" {
			ev.Width = stream.Width
			ev.Height = stream.Height
			ev.VideoCodec = stream.Codec
		}
		if stream.Type == "audio" {
			ev.AudioCodec = stream.Codec
		}
	}
	actualDuration := float64(actual.DurationMS) / 1000
	tolerance := math.Max(0.1, 2*log.maxGap)
	if math.Abs(actualDuration-duration) > tolerance {
		return ev, errors.New("clip duration does not match its selected source interval")
	}
	if plan.Request.Mode == "video" {
		for _, st := range output.Streams {
			if st.Type == "video" {
				n, e := strconv.ParseInt(st.Frames, 10, 64)
				if e != nil || n != log.frames || st.Codec != "h264" {
					return ev, errors.New("clip output frame count or codec verification failed")
				}
				ev.OutputFrames = n
			}
			if st.Type == "audio" && st.Codec != "aac" {
				return ev, errors.New("clip audio codec verification failed")
			}
		}
	}
	if plan.Request.Mode == "video" {
		var videoStart, audioStart float64
		for _, stream := range output.Streams {
			v, e := finite(stream.Start)
			if e != nil {
				return ev, errors.New("output stream timeline unavailable")
			}
			if stream.Type == "video" {
				videoStart = v
			}
			if stream.Type == "audio" {
				audioStart = v
			}
		}
		ev.AVOffsetErrorMS = math.Abs((videoStart-audioStart)-(log.first-log.audioFirst)) * 1000
		if ev.AVOffsetErrorMS > math.Max(50, (log.maxGap+1024/float64(log.sampleRate))*1000) {
			return ev, errors.New("clip introduced an unsupported audio/video offset")
		}
	}
	ev.SourceOrigin = origin
	ev.FirstAudioPTS = log.audioFirst - origin
	ev.LastAudioPTS = log.audioLast - origin
	ev.AudioSamples = log.audioSamples
	ev.SampleRate = log.sampleRate
	ev.Frames = log.frames
	ev.FirstSourcePTS = log.first - origin
	ev.LastSourcePTS = log.last - origin
	ev.DurationMS = actual.DurationMS
	ev.SHA256, ev.Size, e = hash(ctx, candidate)
	if e != nil {
		return ev, e
	}
	// Tool version data is bounded and captured per candidate, never shell output.
	version := command(ctx, b.FFmpeg, "-version")
	var v limited
	v.max = 65536
	version.Stdout = &v
	if version.Run() == nil {
		ev.Toolchain = strings.SplitN(v.String(), "\n", 2)[0]
	}
	if st, e := os.Stat(candidate); e != nil || !st.Mode().IsRegular() {
		return ev, errors.New("clip candidate is unavailable")
	}
	return ev, nil
}

// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
// Package clips implements private source acquisition and precise media rendering.
package clips

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Backend struct{ YTDLP, FFmpeg, FFprobe string }

func New() *Backend {
	resolve := func(name string) string {
		if path, e := exec.LookPath(name); e == nil {
			return path
		}
		return name
	}
	return &Backend{YTDLP: resolve("yt-dlp"), FFmpeg: resolve("ffmpeg"), FFprobe: resolve("ffprobe")}
}
func command(ctx context.Context, path string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, path, args...)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if file, ok := ctx.Value(lockKey{}).(*os.File); ok {
		c.ExtraFiles = []*os.File{file}
	}
	c.Cancel = func() error { return syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }
	c.WaitDelay = 2 * time.Second
	return c
}

type limited struct {
	bytes.Buffer
	max int
}

func (b *limited) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		return 0, errors.New("media metadata exceeds size limit")
	}
	return b.Buffer.Write(p)
}
func runJSON(ctx context.Context, exe string, args []string, v any) error {
	c := command(ctx, exe, args...)
	out := &limited{max: 4 << 20}
	c.Stdout = out
	var stderr tail
	c.Stderr = &stderr
	if e := c.Run(); e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("media inspection failed (%s)", filepath.Base(exe))
	}
	if e := json.Unmarshal(out.Bytes(), v); e != nil {
		return errors.New("invalid media metadata")
	}
	return nil
}

type tail struct{ data []byte }

func (t *tail) Write(p []byte) (int, error) {
	n := len(p)
	t.data = append(t.data, p...)
	if len(t.data) > 8192 {
		t.data = t.data[len(t.data)-8192:]
	}
	return n, nil
}

type probe struct {
	Format struct {
		Duration string `json:"duration"`
		Start    string `json:"start_time"`
	} `json:"format"`
	Streams []struct {
		Index    int    `json:"index"`
		Type     string `json:"codec_type"`
		Codec    string `json:"codec_name"`
		Start    string `json:"start_time"`
		Duration string `json:"duration"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		Transfer string `json:"color_transfer"`
		Frames   string `json:"nb_read_frames"`
	} `json:"streams"`
	Chapters []struct {
		Start string `json:"start_time"`
		End   string `json:"end_time"`
		Tags  struct {
			Title string `json:"title"`
		} `json:"tags"`
	} `json:"chapters"`
}

func (b *Backend) probe(ctx context.Context, path string, count bool) (probe, error) {
	var p probe
	args := []string{"-v", "error", "-show_format", "-show_streams", "-show_chapters"}
	if count {
		args = append(args, "-count_frames")
	}
	args = append(args, "-of", "json", path)
	e := runJSON(ctx, b.FFprobe, args, &p)
	return p, e
}
func finite(s string) (float64, error) {
	v, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.New("unsupported media timeline")
	}
	return v, nil
}
func describe(p probe) (core.ClipSource, float64, error) {
	s := core.ClipSource{Chapters: []core.ClipChapter{}}
	duration, e := finite(p.Format.Duration)
	if e != nil || duration <= 0 {
		return s, 0, errors.New("source duration is unknown")
	}
	origin := math.Inf(1)
	for _, st := range p.Streams {
		if st.Type != "video" && st.Type != "audio" {
			continue
		}
		start, e := finite(st.Start)
		if e != nil && len(p.Streams) == 1 && st.Type == "audio" && st.Start == "" {
			start = 0
			e = nil
		}
		if e != nil {
			return s, 0, errors.New("stream start timestamp is unavailable")
		}
		if start < origin {
			origin = start
		}
		if st.Type == "video" && !s.Video {
			s.Video = true
			s.Width = st.Width
			s.Height = st.Height
			if st.Transfer == "smpte2084" || st.Transfer == "arib-std-b67" {
				return s, 0, errors.New("HDR clipping is not supported")
			}
		}
		if st.Type == "audio" {
			s.Audio = true
		}
	}
	if math.IsInf(origin, 0) {
		return s, 0, errors.New("no supported streams")
	}
	// Containers with unavailable per-stream durations use their normalized
	// format start. Fully described streams provide the more direct end bound.
	maxEnd := origin
	complete := true
	for _, st := range p.Streams {
		if st.Type != "video" && st.Type != "audio" {
			continue
		}
		start, e1 := finite(st.Start)
		d, e2 := finite(st.Duration)
		if e1 != nil || e2 != nil || d <= 0 {
			complete = false
			break
		}
		maxEnd = math.Max(maxEnd, start+d)
	}
	if complete {
		duration = maxEnd - origin
	} else if formatStart, e := finite(p.Format.Start); e == nil && formatStart > 0 {
		duration -= formatStart
	}
	s.DurationMS = int64(math.Round(duration * 1000))
	for i, c := range p.Chapters {
		a, e1 := finite(c.Start)
		z, e2 := finite(c.End)
		r := core.ClipRange{StartMS: int64(math.Round((a - origin) * 1000)), EndMS: int64(math.Round((z - origin) * 1000))}
		if e1 == nil && e2 == nil && core.ValidateClipRange(r, s.DurationMS) == nil {
			s.Chapters = append(s.Chapters, core.ClipChapter{Title: c.Tags.Title, Range: r, Index: i})
		}
	}
	return s, origin, nil
}
func (b *Backend) Inspect(ctx context.Context, r core.ClipRequest, local string) (core.ClipSource, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if local != "" {
		p, e := b.probe(ctx, local, false)
		if e != nil {
			return core.ClipSource{}, e
		}
		s, _, e := describe(p)
		if e != nil {
			return s, e
		}
		digest, _, e := hash(ctx, local)
		s.Identity = "asset:" + r.Source.AssetID
		s.SHA256 = digest
		s.Title = strings.TrimSuffix(filepath.Base(local), filepath.Ext(local))
		return s, e
	}
	var info struct {
		ID         string  `json:"id"`
		Extractor  string  `json:"extractor_key"`
		Title      string  `json:"title"`
		Duration   float64 `json:"duration"`
		Type       string  `json:"_type"`
		Live       bool    `json:"is_live"`
		LiveStatus string  `json:"live_status"`
		DRM        bool    `json:"has_drm"`
		Formats    []struct {
			VCodec string `json:"vcodec"`
			ACodec string `json:"acodec"`
		} `json:"formats"`
		Chapters []struct {
			Title string  `json:"title"`
			Start float64 `json:"start_time"`
			End   float64 `json:"end_time"`
		} `json:"chapters"`
	}
	args := []string{"--ignore-config", "--no-playlist", "--skip-download", "--dump-single-json", "--socket-timeout", "20"}
	if r.Browser != "" {
		args = append(args, "--cookies-from-browser", r.Browser)
	}
	args = append(args, "--", r.Source.URL)
	if e := runJSON(ctx, b.YTDLP, args, &info); e != nil {
		return core.ClipSource{}, e
	}
	if info.ID == "" || info.Type == "playlist" || info.Type == "multi_video" || info.Live || info.LiveStatus == "is_upcoming" || info.DRM || math.IsNaN(info.Duration) || math.IsInf(info.Duration, 0) {
		return core.ClipSource{}, errors.New("a single finite non-DRM recording is required")
	}
	s := core.ClipSource{Identity: info.Extractor + ":" + info.ID, Title: info.Title, DurationMS: int64(math.Round(info.Duration * 1000)), Chapters: []core.ClipChapter{}}
	for _, f := range info.Formats {
		if f.VCodec != "" && f.VCodec != "none" {
			s.Video = true
		}
		if f.ACodec != "" && f.ACodec != "none" {
			s.Audio = true
		}
	}
	for i, c := range info.Chapters {
		r := core.ClipRange{StartMS: int64(math.Round(c.Start * 1000)), EndMS: int64(math.Round(c.End * 1000))}
		if core.ValidateClipRange(r, s.DurationMS) == nil {
			s.Chapters = append(s.Chapters, core.ClipChapter{Title: c.Title, Range: r, Index: i})
		}
	}
	return s, nil
}
func (b *Backend) Prepare(ctx context.Context, plan core.ClipPlan, local, work string, observer func(core.ClipProgress)) (string, core.ClipSource, error) {
	if e := checkSpace(work); e != nil {
		return "", core.ClipSource{}, e
	}
	path := filepath.Join(work, "source.mkv")
	// The directory is private and this workspace has one owner. Incomplete data
	// is reused only by the external downloader, never treated as verified media.
	if local != "" {
		_ = os.Remove(path)
		if e := copySource(ctx, local, path); e != nil {
			return "", core.ClipSource{}, e
		}
	} else {
		current := plan.Source
		var e error
		format := "bestaudio/best"
		if plan.Request.Mode == "video" {
			format = "bestvideo/best,bestaudio/best"
			if plan.Request.Quality != "best" {
				format = "bestvideo[height<=" + plan.Request.Quality + "]/best[height<=" + plan.Request.Quality + "],bestaudio/best"
			}
		}
		args := []string{"--ignore-config", "--no-playlist", "--no-simulate", "--no-warnings", "--fixup", "never", "--abort-on-unavailable-fragments", "--socket-timeout", "20", "--retries", "3", "--fragment-retries", "3", "--newline", "--progress", "--progress-template", "download:CLIP %(progress)j", "--print", "after_move:CLIP_PATH %(filepath)s", "--print", "before_dl:CLIP_ID %(extractor_key)s:%(id)s", "-f", format, "-o", filepath.Join(work, "source-%(format_id)s.%(ext)s"), "--ffmpeg-location", b.FFmpeg}
		if plan.Request.Browser != "" {
			args = append(args, "--cookies-from-browser", plan.Request.Browser)
		}
		if rate := core.DownloadRate(ctx, 0); rate > 0 {
			args = append(args, "--limit-rate", strconv.FormatInt(rate, 10))
		}
		args = append(args, "--", plan.Request.Source.URL)
		c := command(ctx, b.YTDLP, args...)
		writer := &downloadProgress{observer: observer}
		c.Stdout = writer
		var errlog tail
		c.Stderr = &errlog
		if e = c.Run(); e != nil {
			if ctx.Err() != nil {
				return "", current, ctx.Err()
			}
			return "", current, errors.New("source acquisition failed; check source access, browser authentication and download tools")
		}
		if writer.identity != "" && writer.identity != plan.Source.Identity {
			return "", current, &core.Failure{Code: core.CodeSourceChanged, Message: "downloaded source identity changed"}
		}
		paths := []string{}
		seen := map[string]bool{}
		for _, downloaded := range writer.paths {
			clean, err := filepath.Abs(downloaded)
			if err != nil || filepath.Dir(clean) != work {
				return "", current, core.ContractError("source file escaped private staging")
			}
			if !seen[clean] {
				seen[clean] = true
				paths = append(paths, clean)
			}
		}
		if len(paths) < 1 || len(paths) > 2 {
			return "", current, core.ContractError("source acquisition returned unexpected stream count")
		}
		if e = core.EnterStage(ctx, core.Processing); e != nil {
			return "", current, e
		}
		if len(paths) == 1 {
			path = paths[0]
		} else {
			c := command(ctx, b.FFmpeg, "-hide_banner", "-nostdin", "-v", "error", "-y", "-copyts", "-i", paths[0], "-i", paths[1], "-map", "0:v:0", "-map", "1:a:0", "-c", "copy", "-avoid_negative_ts", "disabled", path)
			var diagnostic tail
			c.Stderr = &diagnostic
			if e = c.Run(); e != nil {
				if ctx.Err() != nil {
					return "", current, ctx.Err()
				}
				return "", current, errors.New("private source stream merge failed")
			}
		}

	}
	p, e := b.probe(ctx, path, false)
	if e != nil {
		return "", core.ClipSource{}, e
	}
	source, _, e := describe(p)
	if e != nil {
		return "", source, e
	}
	source.Identity = plan.Source.Identity
	source.Title = plan.Source.Title
	source.SHA256, _, e = hash(ctx, path)
	return path, source, e
}

type downloadProgress struct {
	identity string
	pending  string
	paths    []string
	observer func(core.ClipProgress)
}

func (w *downloadProgress) Write(p []byte) (int, error) {
	w.pending += string(p)
	for {
		n := strings.IndexByte(w.pending, '\n')
		if n < 0 {
			break
		}
		line := strings.TrimSpace(w.pending[:n])
		w.pending = w.pending[n+1:]
		if strings.HasPrefix(line, "CLIP_ID ") {
			w.identity = strings.TrimPrefix(line, "CLIP_ID ")
		}
		if strings.HasPrefix(line, "CLIP_PATH ") {
			w.paths = append(w.paths, strings.TrimPrefix(line, "CLIP_PATH "))
		}
		if strings.HasPrefix(line, "CLIP ") {
			var v struct {
				Downloaded int64    `json:"downloaded_bytes"`
				Total      int64    `json:"total_bytes"`
				Estimate   int64    `json:"total_bytes_estimate"`
				Speed      *float64 `json:"speed"`
				ETA        *float64 `json:"eta"`
			}
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "CLIP ")), &v) == nil {
				m := core.Metrics{DownloadedBytes: v.Downloaded, SpeedBPS: v.Speed, ETASeconds: v.ETA, At: time.Now()}
				if v.Total <= 0 {
					v.Total = v.Estimate
					m.ProgressEstimated = true
				}
				if v.Total > 0 {
					m.TotalBytes = &v.Total
					percent := math.Min(100, 100*float64(v.Downloaded)/float64(v.Total))
					m.Percent = &percent
				}
				if w.observer != nil {
					w.observer(core.ClipProgress{State: core.ClipDownloading, Metrics: m})
				}
			}
		}
	}
	if len(w.pending) > 65536 {
		w.pending = ""
	}
	return len(p), nil
}

package clips

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func fixture(t *testing.T) string {
	t.Helper()
	if _, e := exec.LookPath("ffmpeg"); e != nil {
		t.Skip("ffmpeg unavailable")
	}
	if _, e := exec.LookPath("ffprobe"); e != nil {
		t.Skip("ffprobe unavailable")
	}
	path := filepath.Join(t.TempDir(), "source.mp4")
	c := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10:duration=8", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=8", "-c:v", "libx264", "-g", "40", "-c:a", "aac", "-shortest", path)
	if out, e := c.CombinedOutput(); e != nil {
		t.Fatalf("fixture: %v %s", e, out)
	}
	return path
}
func TestPreciseRenderAndJournalReplay(t *testing.T) {
	source := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	b := New()
	r := core.ClipRequest{Source: core.ClipSourceRef{AssetID: "asset"}, Mode: "video", Accuracy: "precise", Quality: "best", Audio: "m4a", OutputDir: t.TempDir(), Segments: []core.ClipSegment{{Range: core.ClipRange{StartMS: 1250, EndMS: 4550}}}}
	desc, e := b.Inspect(ctx, r, source)
	if e != nil {
		t.Fatal(e)
	}
	plan := core.ClipPlan{Request: r, Source: desc}
	files := Files{Root: t.TempDir()}
	work, candidate, final, e := files.Paths(ctx, "batch", "task", r.OutputDir, "mp4", "Fixture")
	if e != nil {
		t.Fatal(e)
	}
	prepared, info, e := b.Prepare(ctx, plan, source, work, nil)
	if e != nil {
		t.Fatal(e)
	}
	if info.SHA256 != desc.SHA256 {
		t.Fatal("source copy changed")
	}
	ev, e := b.Render(ctx, prepared, candidate, plan, r.Segments[0], nil)
	if e != nil {
		t.Fatal(e)
	}
	if ev.Frames != 33 || ev.OutputFrames != 33 || ev.FirstSourcePTS < 1.25 || ev.FirstSourcePTS > 1.301 || ev.LastSourcePTS >= 4.55 {
		t.Fatalf("wrong source frame evidence: %+v", ev)
	}
	if e = files.Publish(ctx, candidate, final, ev); e != nil {
		t.Fatal(e)
	}
	if e = files.Publish(ctx, candidate, final, ev); e != nil {
		t.Fatal("publication replay:", e)
	}
	if e = os.WriteFile(final, []byte("unrelated"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = files.Publish(ctx, candidate, final, ev); core.CodeOf(e) != core.CodeConflict {
		t.Fatal("overwritten conflict", e)
	}
}
func TestAudioPreciseProfiles(t *testing.T) {
	source := fixture(t)
	for _, profile := range []string{"m4a", "mp3", "opus", "flac", "wav"} {
		t.Run(profile, func(t *testing.T) {
			ctx, c := context.WithTimeout(context.Background(), 20*time.Second)
			defer c()
			r := core.ClipRequest{Mode: "audio", Audio: profile, Quality: "best"}
			ev, e := New().Render(ctx, source, filepath.Join(t.TempDir(), "clip."+profile), core.ClipPlan{Request: r}, core.ClipSegment{Range: core.ClipRange{StartMS: 1500, EndMS: 3500}}, nil)
			if e != nil {
				t.Fatal(e)
			}
			if ev.DurationMS < 1900 || ev.DurationMS > 2100 {
				t.Fatal(ev)
			}
		})
	}
}

func TestPreciseTimelineFixturesPreserveSelectedContent(t *testing.T) {
	if _, e := exec.LookPath("ffmpeg"); e != nil {
		t.Skip("ffmpeg unavailable")
	}
	for _, kind := range []string{"vfr", "positive_origin", "audio_leading"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			source := filepath.Join(t.TempDir(), "marker.mkv")
			filter := "geq=lum='16+N*2':cb=128:cr=128"
			if kind == "vfr" {
				filter += ",select='if(lt(n,20),1,not(mod(n,2)))'"
			}
			if kind == "audio_leading" {
				filter += ",setpts=PTS+0.2/TB"
			}
			args := []string{"-v", "error", "-f", "lavfi", "-i", "color=size=64x64:rate=10:duration=8", "-f", "lavfi", "-i", "sine=sample_rate=48000:duration=8", "-vf", filter, "-fps_mode", "passthrough", "-c:v", "ffv1", "-c:a", "pcm_s16le"}
			if kind == "positive_origin" {
				args = append(args, "-output_ts_offset", "5")
			}
			args = append(args, source)
			if out, e := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput(); e != nil {
				t.Fatalf("fixture: %v %s", e, out)
			}
			backend := New()
			p, e := backend.probe(ctx, source, false)
			if e != nil {
				t.Fatal(e)
			}
			desc, origin, e := describe(p)
			if e != nil {
				t.Fatal(e)
			}
			if desc.DurationMS < 7900 || desc.DurationMS > 8300 {
				t.Fatalf("wrong normalized duration: %d origin %v", desc.DurationMS, origin)
			}
			var frames struct {
				Frames []struct {
					PTS string `json:"best_effort_timestamp_time"`
				} `json:"frames"`
			}
			if e = runJSON(ctx, "ffprobe", []string{"-v", "error", "-select_streams", "v:0", "-show_frames", "-show_entries", "frame=best_effort_timestamp_time", "-of", "json", source}, &frames); e != nil {
				t.Fatal(e)
			}
			selected := []int{}
			for i, f := range frames.Frames {
				pts, e := finite(f.PTS)
				if e != nil {
					t.Fatal(e)
				}
				if pts-origin >= 1.25-1e-6 && pts-origin < 4.55-1e-6 {
					selected = append(selected, i)
				}
			}
			plan := core.ClipPlan{Request: core.ClipRequest{Mode: "video", Quality: "best", Audio: "m4a"}}
			output := filepath.Join(t.TempDir(), "clip.mp4")
			ev, e := backend.Render(ctx, source, output, plan, core.ClipSegment{Range: core.ClipRange{StartMS: 1250, EndMS: 4550}}, nil)
			if e != nil {
				t.Fatal(e)
			}
			if int(ev.Frames) != len(selected) {
				t.Fatal("wrong frame selection", ev.Frames, len(selected))
			}
			decode := func(path string) []byte {
				c := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-i", path, "-map", "0:v:0", "-fps_mode", "passthrough", "-pix_fmt", "gray", "-f", "rawvideo", "-")
				data, e := c.Output()
				if e != nil {
					t.Fatal(e)
				}
				return data
			}
			original := decode(source)
			actual := decode(output)
			mean := func(data []byte, index int) float64 {
				start := index * 64 * 64
				if start+4096 > len(data) {
					t.Fatal("missing decoded frame", index, len(data))
				}
				sum := 0
				for _, v := range data[start : start+4096] {
					sum += int(v)
				}
				return float64(sum) / 4096
			}
			if math.Abs(mean(original, selected[0])-mean(actual, 0)) > 3 || math.Abs(mean(original, selected[len(selected)-1])-mean(actual, len(selected)-1)) > 3 {
				t.Fatal("output contains incorrect source frame IDs")
			}
			if ev.AudioSamples < 1 || ev.SampleRate != 48000 || ev.AVOffsetErrorMS > 150 {
				t.Fatalf("missing audio/sync evidence %+v", ev)
			}
		})
	}
}

func TestRemotePreparationUsesOnePrivateAcquisitionAndExplicitControls(t *testing.T) {
	source := fixture(t)
	dir := t.TempDir()
	program := filepath.Join(dir, "yt-dlp-fixture")
	count := filepath.Join(dir, "acquisitions")
	src, _ := json.Marshal(source)
	counter, _ := json.Marshal(count)
	script := fmt.Sprintf(`#!/usr/bin/env python3
import json, pathlib, shutil, sys
args=sys.argv[1:]
if '--dump-single-json' in args:
 print(json.dumps({'id':'video','extractor_key':'fixture','title':'Remote fixture','duration':8,'formats':[{'vcodec':'h264','acodec':'aac'}]}))
else:
 assert args[args.index('--fixup')+1]=='never'
 assert args[args.index('--limit-rate')+1]=='12345'
 assert args[args.index('--cookies-from-browser')+1]=='firefox'
 target=args[args.index('-o')+1].replace('%%(format_id)s','combined').replace('%%(ext)s','mp4')
 shutil.copyfile(%s,target)
 with open(%s,'a') as count:count.write('one\n')
 print('CLIP_ID fixture:video')
 print('CLIP_PATH '+target)
`, src, counter)
	if e := os.WriteFile(program, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	b := New()
	b.YTDLP = program
	r := core.ClipRequest{Source: core.ClipSourceRef{URL: "https://example.test/video"}, Mode: "video", Quality: "best", Browser: "firefox"}
	desc, e := b.Inspect(context.Background(), r, "")
	if e != nil {
		t.Fatal(e)
	}
	work := filepath.Join(dir, "private")
	os.Mkdir(work, 0700)
	resource := &testClipResource{}
	ctx := core.WithResources(context.Background(), resource)
	path, prepared, e := b.Prepare(ctx, core.ClipPlan{Request: r, Source: desc}, "", work, nil)
	if e != nil {
		t.Fatal(e)
	}
	if filepath.Dir(path) != work || prepared.SHA256 == "" || resource.processing != 1 {
		t.Fatal("private acquisition evidence", path, resource.processing)
	}
	data, e := os.ReadFile(count)
	if e != nil || string(data) != "one\n" {
		t.Fatal("duplicate source acquisition", string(data), e)
	}
}

type testClipResource struct{ processing int }

func (r *testClipResource) Enter(_ context.Context, stage core.Stage) error {
	if stage == core.Processing {
		r.processing++
	}
	return nil
}
func (r *testClipResource) RateBPS() int64 { return 12345 }

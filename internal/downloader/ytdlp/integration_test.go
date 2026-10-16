// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package ytdlp

import (
	"bytes"
	"context"
	"errors"
	"github.com/bfcamel/video-downloader/internal/core"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func defaults() options {
	return options{URL: "https://example.com/video", Mode: "video", Quality: "1080", Audio: "mp3", Out: "/tmp/downloads"}
}
func makeFixture(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skip(name + " unavailable")
		}
	}
	path := filepath.Join(t.TempDir(), "vp9-opus.mp4")
	c := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=10:duration=1", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:v", "libvpx-vp9", "-c:a", "libopus", "-shortest", path)
	if data, err := c.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, data)
	}
	return path
}
func TestCompatibilityAndIntegrity(t *testing.T) {
	source := makeFixture(t)
	ctx := context.Background()
	output, err := compatibleMP4(ctx, source, t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyMedia(ctx, output, true, 1); err != nil {
		t.Fatal(err)
	}
	info, err := probeMedia(ctx, output, true, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range info.Streams {
		if s.Type == "video" && (s.Codec != "h264" || s.Pixel != "yuv420p") {
			t.Fatal(s)
		}
		if s.Type == "audio" && s.Codec != "aac" {
			t.Fatal(s)
		}
	}
	if _, err = probeMedia(ctx, output, true, 60); err == nil {
		t.Fatal("truncated duration accepted")
	}
	bad := filepath.Join(t.TempDir(), "bad.mp4")
	os.WriteFile(bad, []byte("broken"), 0600)
	if verifyMedia(ctx, bad, true, 0) == nil {
		t.Fatal("corruption accepted")
	}
}
func TestVerifiedDownloadPublication(t *testing.T) {
	source := makeFixture(t)
	t.Setenv("MACDOWN_TEST_FIXTURE", source)
	dir := t.TempDir()
	downloader := filepath.Join(dir, "downloader")
	script := `#!/bin/sh
while [ "$#" -gt 0 ]; do
 case "$1" in
 --paths) dest="$2"; shift 2;;
 --print-to-file) manifest="$3"; shift 3;;
 *) shift;;
 esac
done
cp "$MACDOWN_TEST_FIXTURE" "$dest/clip.mp4"
printf '"%s"\n' "$dest/clip.mp4" > "$manifest"
`
	if err := os.WriteFile(downloader, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	o := defaults()
	cwd, _ := os.Getwd()
	o.Out, _ = filepath.Rel(cwd, filepath.Join(dir, "out"))
	o.Duration = 1
	var log bytes.Buffer
	if err := verifiedDownload(context.Background(), downloader, o, &log); err != nil {
		t.Fatal(err, log.String())
	}
	target := filepath.Join(o.Out, "clip.mp4")
	if err := verifyMedia(context.Background(), target, true, 1); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(target)
	if err := verifiedDownload(context.Background(), downloader, o, &log); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(target)
	if !bytes.Equal(original, after) {
		t.Fatal("existing file overwritten")
	}
	t.Setenv("MACDOWN_TEST_FIXTURE", filepath.Join(dir, "broken"))
	os.WriteFile(filepath.Join(dir, "broken"), []byte("bad"), 0600)
	o.Out = filepath.Join(dir, "bad-out")
	if verifiedDownload(context.Background(), downloader, o, &log) == nil {
		t.Fatal("bad file reported success")
	}
	files, _ := filepath.Glob(filepath.Join(o.Out, "*.mp4"))
	if len(files) != 0 {
		t.Fatal("invalid download entered library")
	}
}

func TestAllVideoProfilesProduceSelectedCodecsAndContainers(t *testing.T) {
	ctx := context.Background()
	fixture := makeFixture(t)
	base, err := convertVideo(ctx, fixture, t.TempDir(), options{Duration: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoders, _ := exec.Command("ffmpeg", "-hide_banner", "-encoders").CombinedOutput()
	for _, codec := range []string{"h264", "hevc", "vp9", "av1"} {
		for _, container := range videoContainerChoices(codec) {
			t.Run(codec+"/"+container.Value, func(t *testing.T) {
				encoder := map[string]string{"h264": "libx264", "hevc": "libx265", "vp9": "libvpx-vp9", "av1": "libsvtav1"}[codec]
				if !strings.Contains(string(encoders), encoder) {
					t.Skip("encoder unavailable: " + encoder)
				}
				o := options{Codec: codec, Container: container.Value, Duration: 1}
				result, err := convertVideo(ctx, base, t.TempDir(), o, nil)
				if err != nil {
					t.Fatal(err)
				}
				if err = verifyMedia(ctx, result, true, 1); err != nil {
					t.Fatal(err)
				}
				if err = verifyVideoProfile(ctx, result, o); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
func TestPipelineProgressAndMergerStages(t *testing.T) {
	var events []string
	observer := func(stage, detail string) { events = append(events, stage+":"+detail) }
	p := &ffmpegProgress{duration: 10, stage: "processing", observer: observer}
	p.Write([]byte("out_ti"))
	p.Write([]byte("me_us=5000000\nout_time_us=12000000\n"))
	if !reflect.DeepEqual(events, []string{"processing:50%", "processing:99%"}) {
		t.Fatal(events)
	}
	events = nil
	var output bytes.Buffer
	w := &downloaderOutput{out: &output, observer: observer}
	w.Write([]byte("[Mer"))
	w.Write([]byte("ger] Combining\n[VideoRemuxer] Done\n"))
	if !reflect.DeepEqual(events, []string{"processing:"}) {
		t.Fatal(events)
	}
}
func videoContainerChoices(codec string) []struct{ Value string } {
	v := []struct{ Value string }{{"mp4"}, {"mkv"}}
	if codec == "vp9" || codec == "av1" {
		v = append(v, struct{ Value string }{"webm"})
	}
	return v
}

func TestStandaloneEngineCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "yt-dlp")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nsleep 5\n"), 0700); err != nil {
		t.Fatal(err)
	}
	engine := New(path)
	r := defaults()
	r.Out = t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := engine.Inspect(ctx, r); !errors.Is(err, core.ErrCanceled) {
		t.Fatal(err)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	if _, err := engine.Download(ctx2, r, nil); !errors.Is(err, core.ErrCanceled) {
		t.Fatal(err)
	}
}

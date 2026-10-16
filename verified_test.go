package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthenticationDiagnosticsAndProfiles(t *testing.T) {
	raw := "ERROR: Sign in to confirm you're not a bot"
	got := explain(raw)
	if !strings.Contains(got, "не бот") || !strings.Contains(got, raw) {
		t.Fatal(got)
	}
	for _, browser := range []string{"firefox", "firefox:default-release", "firefox:/tmp/my profile::Personal", "chrome:Default"} {
		o := defaults()
		o.Cookies = browser
		if err := validate(o); err != nil {
			t.Fatal(browser, err)
		}
	}
	o := defaults()
	o.Cookies = "unknown:profile"
	if validate(o) == nil {
		t.Fatal("unknown browser accepted")
	}
}
func TestQueueAllowsNewFormatsAndDistinctSites(t *testing.T) {
	p := newJobPool(context.Background(), "")
	o := defaults()
	o.Mode = "audio"
	p.add(o)
	p.jobs[0].state = "done"
	o.Audio = "wav"
	if !p.add(o) {
		t.Fatal("new audio format blocked")
	}
	p = newJobPool(context.Background(), "")
	o = defaults()
	o.URL = "https://one.example/video/123"
	p.add(o)
	p.jobs[0].info = mediaInfo{ID: "123", Extractor: "One"}
	p.jobs[0].state = "done"
	o.URL = "https://two.example/video/123"
	p.add(o)
	j := p.apply(jobEvent{id: 2, kind: "inspected", info: mediaInfo{ID: "123", Extractor: "Two"}})
	if j.state != "choose" {
		t.Fatal(j.state)
	}
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
	o.duration = 1
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
	files, _ := listMedia(o.Out)
	if len(files) != 0 {
		t.Fatal("invalid download entered library")
	}
}

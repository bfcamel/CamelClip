package ytdlp

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDifferentExistingMediaIsPublicationConflict(t *testing.T) {
	fixture := makeFixture(t)
	t.Setenv("CAMELCLIP_TEST_FIXTURE", fixture)
	dir := t.TempDir()
	downloader := filepath.Join(dir, "downloader")
	script := `#!/bin/sh
while [ "$#" -gt 0 ]; do
 case "$1" in --paths) dest="$2"; shift 2;; --print-to-file) manifest="$3"; shift 3;; *) shift;; esac
done
cp "$CAMELCLIP_TEST_FIXTURE" "$dest/clip.mp4"
printf '"%s"\n' "$dest/clip.mp4" > "$manifest"
`
	if e := os.WriteFile(downloader, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	r := defaults()
	r.Out = filepath.Join(dir, "out")
	r.Duration = 1
	os.Mkdir(r.Out, 0700)
	target := filepath.Join(r.Out, "clip.mp4")
	c := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=red:size=64x64:rate=10:duration=1", "-f", "lavfi", "-i", "sine=frequency=880:duration=1", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", target)
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatal(e, string(b))
	}
	original, _ := os.ReadFile(target)
	_, e := New(downloader).Download(context.Background(), r, nil)
	if e == nil || !strings.Contains(e.Error(), "publication conflict") {
		t.Fatal("different media accepted", e)
	}
	after, _ := os.ReadFile(target)
	if !bytes.Equal(original, after) {
		t.Fatal("existing file modified")
	}
	staged, _ := filepath.Glob(filepath.Join(r.Out, ".camelclip-*", "converted.mp4"))
	if len(staged) != 1 {
		t.Fatal("staged result discarded")
	}
}

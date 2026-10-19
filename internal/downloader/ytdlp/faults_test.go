// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package ytdlp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bfcamel/CamelClip/internal/core"
)

func TestFailuresNeverPublishUnverifiedFiles(t *testing.T) {
	fixture := makeFixture(t)
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	tools := t.TempDir()
	downloader := filepath.Join(tools, "downloader")
	script := `#!/bin/sh
while [ "$#" -gt 0 ]; do
 case "$1" in
 --paths) dest="$2"; shift 2;;
 --print-to-file) manifest="$3"; shift 3;;
 *) shift;;
 esac
done
cp "$CAMELCLIP_TEST_FIXTURE" "$dest/clip.mp4"
printf '"%s"\n' "$dest/clip.mp4" > "$manifest"
if [ "$CAMELCLIP_FAIL_STAGE" = downloading ]; then echo 'ERROR: connection reset' >&2; exit 1; fi
`
	os.WriteFile(downloader, []byte(script), 0700)
	wrapper := `#!/bin/sh
stage=processing
for arg in "$@"; do [ "$arg" != null ] || stage=verifying; done
if [ "$stage" = "$CAMELCLIP_FAIL_STAGE" ]; then echo 'No space left on device' >&2; exit 1; fi
exec "$CAMELCLIP_REAL_FFMPEG" "$@"
`
	os.WriteFile(filepath.Join(tools, "ffmpeg"), []byte(wrapper), 0700)
	t.Setenv("CAMELCLIP_REAL_FFMPEG", ffmpeg)
	t.Setenv("CAMELCLIP_TEST_FIXTURE", fixture)
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, stage := range []string{"downloading", "processing", "verifying"} {
		t.Run(stage, func(t *testing.T) {
			t.Setenv("CAMELCLIP_FAIL_STAGE", stage)
			request := defaults()
			request.Out = t.TempDir()
			request.Duration = 1
			engine := New(downloader)
			if _, err := engine.Download(context.Background(), request, nil); err == nil {
				t.Fatal("failure was reported as success")
			}
			files, _ := filepath.Glob(filepath.Join(request.Out, "*.mp4"))
			if len(files) != 0 {
				t.Fatal("unverified file published", files)
			}
			// Staging survives the error and can be retried successfully.
			t.Setenv("CAMELCLIP_FAIL_STAGE", "")
			if _, err := engine.Download(context.Background(), request, nil); err != nil {
				t.Fatal("retry failed", err)
			}
			files, _ = filepath.Glob(filepath.Join(request.Out, "*.mp4"))
			if len(files) != 1 {
				t.Fatal("retry did not publish exactly once", files)
			}
		})
	}
}
func TestCancelTerminatesDescendants(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "survived")
	ready := filepath.Join(dir, "ready")
	t.Setenv("CAMELCLIP_CHILD_MARKER", marker)
	t.Setenv("CAMELCLIP_CHILD_READY", ready)
	path := filepath.Join(dir, "downloader")
	script := "#!/bin/sh\n(sleep 1; echo survived > \"$CAMELCLIP_CHILD_MARKER\") &\necho ready > \"$CAMELCLIP_CHILD_READY\"\nwait\n"
	os.WriteFile(path, []byte(script), 0700)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := New(path).Inspect(ctx, defaults()); result <- err }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, core.ErrCanceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not return")
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("descendant survived cancellation")
	}
}
func TestInvalidExistingFileIsPreserved(t *testing.T) {
	fixture := makeFixture(t)
	t.Setenv("CAMELCLIP_TEST_FIXTURE", fixture)
	dir := t.TempDir()
	path := filepath.Join(dir, "downloader")
	os.WriteFile(path, []byte(`#!/bin/sh
while [ "$#" -gt 0 ]; do
 case "$1" in --paths) dest="$2"; shift 2;; --print-to-file) manifest="$3"; shift 3;; *) shift;; esac
done
cp "$CAMELCLIP_TEST_FIXTURE" "$dest/clip.mp4"
printf '"%s"\n' "$dest/clip.mp4" > "$manifest"
`), 0700)
	r := defaults()
	r.Out = filepath.Join(dir, "out")
	os.Mkdir(r.Out, 0700)
	target := filepath.Join(r.Out, "clip.mp4")
	original := []byte("previous broken file")
	os.WriteFile(target, original, 0600)
	_, err := New(path).Download(context.Background(), r, nil)
	if err == nil || !strings.Contains(err.Error(), "kept unchanged") {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(target)
	if !bytes.Equal(original, after) {
		t.Fatal("existing user file replaced")
	}
}

// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func command(ctx context.Context, path string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, path, args...)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if lock, ok := ctx.Value(stageLockContextKey{}).(*os.File); ok {
		c.ExtraFiles = []*os.File{lock}
	}
	c.Cancel = func() error { return syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }
	c.WaitDelay = 2 * time.Second
	return c
}
func inspect(ctx context.Context, path string, o options) (mediaInfo, error) {
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	a := []string{"--ignore-config", "--no-playlist", "--skip-download", "--dump-single-json", "--socket-timeout", "20"}
	if o.Cookies != "" {
		a = append(a, "--cookies-from-browser", o.Cookies)
	}
	c := command(ctx, path, append(a, "--", o.URL)...)
	var out bytes.Buffer
	var tail tailBuffer
	c.Stdout = &out
	c.Stderr = &tail
	if err := c.Run(); err != nil {
		if ctx.Err() != nil {
			if parent.Err() != nil {
				return mediaInfo{}, errCanceled
			}
			return mediaInfo{}, &core.Failure{Code: core.CodeBackend, Message: "metadata request exceeded 90 seconds", Temporary: true}
		}
		return mediaInfo{}, backendFailure(fmt.Errorf("yt-dlp metadata: %w: %s", err, tail.String()))
	}
	var info mediaInfo
	if err := json.Unmarshal(out.Bytes(), &info); err != nil {
		return info, errors.New("invalid metadata returned by yt-dlp")
	}
	if info.Type == "playlist" || info.Type == "multi_video" {
		return info, errors.New("a single video URL is required")
	}
	if info.Live {
		return info, errors.New("ongoing livestreams are not supported")
	}
	return info, nil
}

// Bound retained diagnostics; progress continues to stream without buffering.
type tailBuffer struct{ data []byte }

func (t *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	t.data = append(t.data, p...)
	if len(t.data) > 16384 {
		t.data = t.data[len(t.data)-16384:]
	}
	return n, nil
}
func (t *tailBuffer) String() string { return string(t.data) }

// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package gui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bfcamel/CamelClip/internal/config"
)

func Defaults(home string) config.Settings {
	return config.Defaults(filepath.Join(downloadsDirectory(home), "CamelClip"))
}
func downloadsDirectory(home string) string {
	if runtime.GOOS == "linux" {
		dir := os.Getenv("XDG_CONFIG_HOME")
		if dir == "" {
			dir = filepath.Join(home, ".config")
		}
		if data, e := os.ReadFile(filepath.Join(dir, "user-dirs.dirs")); e == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "XDG_DOWNLOAD_DIR=") {
					v := strings.Trim(strings.TrimSpace(strings.SplitN(line, "=", 2)[1]), "\"")
					v = strings.ReplaceAll(v, "$HOME", home)
					if filepath.IsAbs(v) {
						return v
					}
				}
			}
		}
	}
	return filepath.Join(home, "Downloads")
}
func OpenMedia(ctx context.Context, path string, reveal bool) error {
	path, e := filepath.Abs(path)
	if e != nil {
		return e
	}
	if reveal {
		stat, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("file unavailable: %w", err)
		}
		if !stat.Mode().IsRegular() {
			return fmt.Errorf("cannot reveal a video: path is not a regular file")
		}
	}
	if runtime.GOOS == "darwin" {
		args := []string{path}
		if reveal {
			args = []string{"-R", path}
		}
		return exec.CommandContext(ctx, "open", args...).Run()
	}
	if reveal {
		path = filepath.Dir(path)
	}
	return exec.CommandContext(ctx, "xdg-open", path).Run()
}
func OpenBrowser(ctx context.Context, url string) error {
	command := "xdg-open"
	if runtime.GOOS == "darwin" {
		command = "open"
	}
	return exec.CommandContext(ctx, command, url).Run()
}

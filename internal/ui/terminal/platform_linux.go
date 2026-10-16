// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// xdg-open delegates to the user's desktop. Reveal opens the containing folder;
// headless hosts can still download and use the library without this action.
func openMedia(ctx context.Context, path string, reveal bool) error {
	target, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if reveal {
		target = filepath.Dir(target)
	}
	if err := exec.CommandContext(ctx, "xdg-open", target).Run(); err != nil {
		return fmt.Errorf("xdg-open: %w", err)
	}
	return nil
}
func preferredSystemLanguage() string       { return "en" }
func platformMessage(message string) string { return strings.ReplaceAll(message, "Finder", "Files") }
func dependencyHint() string {
	return tr("Linux: установите yt-dlp, FFmpeg/ffprobe и Deno; для открытия файлов — xdg-utils. См. README.")
}

// Respect XDG user directories, including localized Downloads names. This file
// is data, never a shell script: only a quoted absolute path or $HOME is accepted.
func downloadsDirectory(home string) string {
	fallback := filepath.Join(home, "Downloads")
	base, err := os.UserConfigDir()
	if err != nil {
		return fallback
	}
	data, err := os.ReadFile(filepath.Join(base, "user-dirs.dirs"))
	if err != nil {
		return fallback
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.TrimSpace(key) != "XDG_DOWNLOAD_DIR" {
			continue
		}
		value, err = strconv.Unquote(strings.TrimSpace(value))
		if err != nil {
			return fallback
		}
		if value == "$HOME" {
			value = home
		} else if strings.HasPrefix(value, "$HOME/") {
			value = filepath.Join(home, strings.TrimPrefix(value, "$HOME/"))
		}
		if !filepath.IsAbs(value) || strings.ContainsAny(value, "$`\x00\r\n") {
			return fallback
		}
		return filepath.Clean(value)
	}
	return fallback
}

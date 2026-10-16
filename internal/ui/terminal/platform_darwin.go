// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func openMedia(ctx context.Context, path string, reveal bool) error {
	args := []string{}
	if reveal {
		args = append(args, "-R")
	}
	args = append(args, "--", path)
	return exec.CommandContext(ctx, "open", args...).Run()
}
func preferredSystemLanguage() string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleLanguages").Output(); err == nil {
		tokens := strings.FieldsFunc(string(out), func(r rune) bool { return strings.ContainsRune("()\", \n\t\r", r) })
		if len(tokens) > 0 {
			if value := localeLanguage(tokens[0]); value != "" {
				return value
			}
		}
	}
	return "en"
}
func downloadsDirectory(home string) string { return filepath.Join(home, "Downloads") }
func platformMessage(message string) string { return message }
func dependencyHint() string                { return tr("Зависимости: brew install yt-dlp ffmpeg deno") }

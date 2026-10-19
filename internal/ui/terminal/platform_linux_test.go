// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxDownloadsDirectory(t *testing.T) {
	config := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	for _, tc := range []struct{ data, want string }{
		{`XDG_DOWNLOAD_DIR="$HOME/Загрузки"`, filepath.Join(home, "Загрузки")},
		{`XDG_DOWNLOAD_DIR="/srv/shared downloads"`, "/srv/shared downloads"},
		{`XDG_DOWNLOAD_DIR="$HOME"`, home},
		{`XDG_DOWNLOAD_DIR="relative/path"`, filepath.Join(home, "Downloads")},
		{`XDG_DOWNLOAD_DIR="$(touch /tmp/camelclip-should-not-exist)"`, filepath.Join(home, "Downloads")},
		{`XDG_DOWNLOAD_DIR="$OTHER/path"`, filepath.Join(home, "Downloads")},
		{`XDG_DOWNLOAD_DIR=unquoted`, filepath.Join(home, "Downloads")},
		{`XDG_DESKTOP_DIR="$HOME/Desktop"`, filepath.Join(home, "Downloads")},
	} {
		if err := os.WriteFile(filepath.Join(config, "user-dirs.dirs"), []byte(tc.data), 0600); err != nil {
			t.Fatal(err)
		}
		if got := downloadsDirectory(home); got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.data, got, tc.want)
		}
	}
}

func TestLinuxOpenFileAndReveal(t *testing.T) {
	bin := t.TempDir()
	record := filepath.Join(bin, "args")
	t.Setenv("CAMELCLIP_OPEN_RECORD", record)
	t.Setenv("PATH", bin)
	if err := os.WriteFile(filepath.Join(bin, "xdg-open"), []byte("#!/bin/sh\nprintf '%s\\n' \"$#\" \"$1\" > \"$CAMELCLIP_OPEN_RECORD\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "-movie $(literal).mp4")
	for _, reveal := range []bool{false, true} {
		if err := openMedia(context.Background(), path, reveal); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(record)
		if err != nil {
			t.Fatal(err)
		}
		want := path
		if reveal {
			want = filepath.Dir(path)
		}
		if string(data) != "1\n"+want+"\n" {
			t.Fatalf("unexpected opener arguments: %s", data)
		}
	}
	t.Setenv("PATH", t.TempDir())
	if err := openMedia(context.Background(), path, false); err == nil {
		t.Fatal("missing desktop opener should report an error")
	}
}

func TestLinuxMessagesAndLocale(t *testing.T) {
	for _, lang := range []string{"en", "ru"} {
		useLanguage(lang, "en")
		if strings.Contains(dependencyHint(), "brew") {
			t.Fatal("macOS dependency hint on Linux")
		}
		if strings.Contains(tr("Показать в Finder"), "Finder") {
			t.Fatal("Finder on Linux")
		}
	}
	useLanguage("ru", "en")
	if preferredSystemLanguage() != "en" {
		t.Fatal("unexpected fallback")
	}
}

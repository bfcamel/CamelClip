// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package wsl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsDesktopCommands(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("RECORD", filepath.Join(dir, "record"))
	for name, body := range map[string]string{
		"powershell.exe": `printf '%s\n' "$@" > "$RECORD.args"
cat > "$RECORD"
case "$*" in *shell:Downloads*) printf 'D:\\Shared Downloads\r\n';; esac`,
		"wslpath": `case "$1" in
 -u) test "$2" = 'D:\Shared Downloads' && printf '/mnt/d/Shared Downloads\n';;
 -w) test "$2" = "$SOURCE" && printf '%s\n' "$TARGET";;
 esac`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if dir := Downloads(); dir != "/mnt/d/Shared Downloads" {
		t.Fatalf("wrong Downloads folder %q", dir)
	}
	path := "/mnt/d/O'Brien & Видео.mp4"
	target := `D:\O'Brien & Видео.mp4`
	t.Setenv("SOURCE", path)
	t.Setenv("TARGET", target)
	ctx := context.Background()
	for _, reveal := range []bool{false, true} {
		if err := OpenMedia(ctx, path, reveal); err != nil {
			t.Fatal(err)
		}
		checkCommand(t, target, map[bool]string{false: "Invoke-Item", true: "/select,"}[reveal])
	}
	url := "http://127.0.0.1:17823/#private"
	if err := OpenBrowser(ctx, url); err != nil {
		t.Fatal(err)
	}
	checkCommand(t, url, "Start-Process")
	t.Setenv("PATH", t.TempDir())
	if Downloads() != "" || OpenBrowser(ctx, url) == nil || OpenMedia(ctx, path, true) == nil {
		t.Fatal("missing interoperability tools must report failure")
	}
}

func checkCommand(t *testing.T, target, operation string) {
	t.Helper()
	data, err := os.ReadFile(os.Getenv("RECORD"))
	if err != nil || string(data) != target {
		t.Fatalf("wrong target %q: %v", data, err)
	}
	args, err := os.ReadFile(os.Getenv("RECORD") + ".args")
	if err != nil || strings.Contains(string(args), target) || !strings.Contains(string(args), operation) {
		t.Fatalf("unexpected command %q: %v", args, err)
	}
}

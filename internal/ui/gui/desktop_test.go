// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package gui_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bfcamel/CamelClip/internal/ui/gui"
)

func TestRevealSelectsExactFileAndRejectsMissingOutput(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS Finder selection contract")
	}
	dir := t.TempDir()
	tool := filepath.Join(dir, "open")
	args := filepath.Join(dir, "arguments")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CAMELCLIP_OPEN_ARGS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CAMELCLIP_OPEN_ARGS", args)
	video := filepath.Join(dir, "A video — 01.mp4")
	if err := os.WriteFile(video, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := gui.OpenMedia(context.Background(), video, true); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(args)
	if err != nil || string(got) != "-R\n"+video+"\n" {
		t.Fatalf("Finder did not receive exact file: %q %v", got, err)
	}
	os.Remove(args)
	os.Remove(video)
	if gui.OpenMedia(context.Background(), video, true) == nil {
		t.Fatal("missing video revealed")
	}
	if gui.OpenMedia(context.Background(), dir, true) == nil {
		t.Fatal("directory accepted as video")
	}
	if _, err := os.Stat(args); !os.IsNotExist(err) {
		t.Fatal("file manager launched for unavailable output")
	}
}

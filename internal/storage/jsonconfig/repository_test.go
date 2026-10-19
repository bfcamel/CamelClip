// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package jsonconfig

import (
	"context"
	"github.com/bfcamel/CamelClip/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestCanceledSavePreservesPreferencesAndFailedRenameCleansTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s := config.Settings{Language: "en", Mode: "video", Quality: "best", Audio: "mp3", Out: dir, Codec: "h264", Container: "mp4"}
	f := File{Path: path, Defaults: s}
	if err := f.Save(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(path)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.Language = "ru"
	if err := f.Save(ctx, s); err == nil {
		t.Fatal("canceled save accepted")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("preferences changed")
	}
	os.Remove(path)
	os.Mkdir(path, 0700)
	if err := f.Save(context.Background(), s); err == nil {
		t.Fatal("failed rename accepted")
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "settings-*.tmp"))
	if len(matches) != 0 {
		t.Fatal("temporary files left", matches)
	}
}

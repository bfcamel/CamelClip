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

func TestExistingSettingsGainOnlyMissingResourceDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original := `{"language":"ru","mode":"video","quality":"720","audio_format":"opus","output_directory":"/tmp/personal","cookies_browser":"firefox:work","video_codec":"hevc","video_container":"mkv"}`
	os.WriteFile(path, []byte(original), 0600)
	f := File{Path: path, Defaults: config.Settings{MaxDownloads: 5}}
	s, err := f.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.MaxDownloads != 5 || s.MaxProcessing != 0 || s.Out != "/tmp/personal" || s.Browser != "firefox:work" || s.Codec != "hevc" {
		t.Fatal(s)
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatal("read mutated saved preferences")
	}
	if err = f.Save(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	roundtrip, err := f.Load(context.Background())
	if err != nil || roundtrip != s {
		t.Fatal(roundtrip, err)
	}
	os.WriteFile(path, []byte("{broken"), 0600)
	if err = f.Save(context.Background(), s); err == nil {
		t.Fatal("corrupt settings silently replaced")
	}
	data, _ = os.ReadFile(path)
	if string(data) != "{broken" {
		t.Fatal("bad file was changed")
	}
}

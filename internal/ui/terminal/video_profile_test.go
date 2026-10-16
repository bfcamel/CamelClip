// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVideoProfileSettingsMigrationAndCLI(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.json")
	os.WriteFile(path, []byte(`{"language":"ru","mode":"video","quality":"best","audio_format":"mp3","output_directory":"/tmp/videos"}`), 0600)
	o := testOptions(root)
	if err := loadSettings(path, &o); err != nil {
		t.Fatal(err)
	}
	if o.Codec != "h264" || o.Container != "mp4" {
		t.Fatal("old settings lost defaults")
	}
	if _, err := parseCLI([]string{"download", "--video-codec", "vp9", "https://example.com/a", "--video-format", "webm"}, &o); err != nil {
		t.Fatal(err)
	}
	if o.Codec != "vp9" || o.Container != "webm" {
		t.Fatal(o)
	}
	if err := saveSettings(path, o); err != nil {
		t.Fatal(err)
	}
	loaded := testOptions(root)
	if err := loadSettings(path, &loaded); err != nil || loaded.Codec != "vp9" || loaded.Container != "webm" {
		t.Fatal(loaded, err)
	}
	loaded.Codec = "h264"
	if validateVideoProfile(loaded) == nil {
		t.Fatal("H264/WebM accepted")
	}
}
func TestCodecAndContainerUISettingsPersist(t *testing.T) {
	root := t.TempDir()
	o := testOptions(root)
	o.Codec = "av1"
	o.Container = "webm"
	u := panelUI{ctx: context.Background(), o: &o, home: root, config: filepath.Join(root, "settings.json"), system: "ru", pool: &jobPool{}}
	u.showChoices("video-codec", "codec", videoCodecChoices(), "h264")
	u.modalKey("enter")
	if o.Codec != "h264" || o.Container != "mkv" {
		t.Fatal(o)
	}
	u.showChoices("video-format", "container", videoContainerChoices(o.Codec), "mp4")
	u.modalKey("enter")
	loaded := testOptions(root)
	if err := loadSettings(u.config, &loaded); err != nil || loaded.Codec != "h264" || loaded.Container != "mp4" {
		t.Fatal(loaded, err)
	}
}

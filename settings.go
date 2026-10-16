package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type settings struct {
	Language string `json:"language"`
	Mode     string `json:"mode"`
	Quality  string `json:"quality"`
	Audio    string `json:"audio_format"`
	Out      string `json:"output_directory"`
	Browser  string `json:"cookies_browser,omitempty"`
}

func defaultOptions(home string) options {
	return options{Lang: "auto", Mode: "video", Quality: "best", Audio: "mp3", Out: filepath.Join(home, "Downloads", "MacDown")}
}
func loadSettings(path string, o *options) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var s settings
	if err = json.Unmarshal(data, &s); err != nil {
		return err
	}
	candidate := *o
	candidate.Lang = s.Language
	if candidate.Lang == "" {
		candidate.Lang = "auto"
	}
	candidate.Mode = s.Mode
	candidate.Quality = s.Quality
	candidate.Audio = s.Audio
	candidate.Out = s.Out
	candidate.Cookies = s.Browser
	candidate.URL = "https://example.com/video"
	if err = validate(candidate); err != nil {
		return fmt.Errorf(tr("некорректные настройки: %w"), err)
	}
	candidate.URL = o.URL
	*o = candidate
	return nil
}
func saveSettings(path string, o options) error {
	data, err := json.MarshalIndent(settings{Language: o.Lang, Mode: o.Mode, Quality: o.Quality, Audio: o.Audio, Out: o.Out, Browser: o.Cookies}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "settings-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

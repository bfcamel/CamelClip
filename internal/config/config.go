// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

// Package config defines application preferences and their repository contract.
package config

import (
	"context"
	"fmt"

	"github.com/bfcamel/video-downloader/internal/core"
)

type Settings struct {
	Language  string `json:"language"`
	Mode      string `json:"mode"`
	Quality   string `json:"quality"`
	Audio     string `json:"audio_format"`
	Out       string `json:"output_directory"`
	Browser   string `json:"cookies_browser,omitempty"`
	Codec     string `json:"video_codec"`
	Container string `json:"video_container"`
}
type Repository interface {
	Load(context.Context) (Settings, error)
	Save(context.Context, Settings) error
}

func Validate(s Settings) error {
	if s.Codec == "" || s.Container == "" {
		return fmt.Errorf("video_codec and video_container are required")
	}
	if s.Language != "auto" && s.Language != "ru" && s.Language != "en" {
		return fmt.Errorf("invalid language")
	}
	return core.Validate(core.Request{URL: "https://example.com/video", Mode: s.Mode, Quality: s.Quality, Audio: s.Audio, Out: s.Out, Cookies: s.Browser, Codec: s.Codec, Container: s.Container})
}

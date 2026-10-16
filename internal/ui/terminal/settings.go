// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/bfcamel/video-downloader/internal/config"
)

type settings = config.Settings

func settingsFor(o options) settings {
	if o.MaxDownloads == 0 {
		o.MaxDownloads = 5
	}
	if o.Lang == "" {
		o.Lang = "auto"
	}
	return settings{Language: o.Lang, Mode: o.Mode, Quality: o.Quality, Audio: o.Audio, Out: o.Out, Browser: o.Cookies, Codec: videoCodec(o), Container: videoContainer(o), MaxDownloads: o.MaxDownloads, MaxProcessing: o.MaxProcessing, RateLimitBPS: o.RateLimitBPS, JobRateLimitBPS: o.JobRateLimitBPS}
}
func defaultOptions(home string) options {
	return options{MaxDownloads: 5, Lang: "auto", Mode: "video", Quality: "best", Audio: "mp3", Codec: "h264", Container: "mp4", Out: filepath.Join(downloadsDirectory(home), "Clipnaro")}
}
func loadSettings(path string, o *options) error {
	if o.runtime == nil || o.runtime.OpenConfig == nil {
		return errors.New("config repository dependency is not configured")
	}
	repository := o.runtime.OpenConfig(path, settingsFor(*o))
	s, err := repository.Load(context.Background())
	if err != nil {
		return err
	}
	o.Lang = s.Language
	o.Mode = s.Mode
	o.Quality = s.Quality
	o.Audio = s.Audio
	o.Out = s.Out
	o.Cookies = s.Browser
	o.Codec = s.Codec
	o.Container = s.Container
	o.MaxDownloads = s.MaxDownloads
	o.MaxProcessing = s.MaxProcessing
	o.RateLimitBPS = s.RateLimitBPS
	o.JobRateLimitBPS = s.JobRateLimitBPS
	return nil
}
func saveSettings(path string, o options) error {
	if o.runtime == nil || o.runtime.OpenConfig == nil {
		return errors.New("config repository dependency is not configured")
	}
	repository := o.runtime.OpenConfig(path, settingsFor(o))
	return repository.Save(context.Background(), settingsFor(o))
}

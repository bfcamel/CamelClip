// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/bfcamel/CamelClip/internal/config"
)

type settings = config.Settings

func settingsFor(o options) settings {
	if o.MaxDownloads == 0 {
		o.MaxDownloads = 5
	}
	if o.Lang == "" {
		o.Lang = "auto"
	}
	return settings{MaxUploads: o.MaxUploads, UploadParts: o.UploadParts, RetryAttempts: o.RetryAttempts, StagingFiles: o.StagingFiles, StagingBytes: o.StagingBytes, MinFreeBytes: o.MinFreeBytes, UploadRateBPS: o.UploadRateBPS, Language: o.Lang, Theme: o.Theme, Mode: o.Mode, Quality: o.Quality, Audio: o.Audio, Out: o.Out, Browser: o.Cookies, Codec: videoCodec(o), Container: videoContainer(o), MaxDownloads: o.MaxDownloads, MaxProcessing: o.MaxProcessing, RateLimitBPS: o.RateLimitBPS, JobRateLimitBPS: o.JobRateLimitBPS}
}
func defaultOptions(home string) options {
	v := config.Defaults(filepath.Join(downloadsDirectory(home), "CamelClip"))
	return options{MaxUploads: v.MaxUploads, UploadParts: v.UploadParts, StagingFiles: v.StagingFiles, StagingBytes: v.StagingBytes, MinFreeBytes: v.MinFreeBytes, MaxDownloads: v.MaxDownloads, Lang: v.Language, Theme: v.Theme, Mode: v.Mode, Quality: v.Quality, Audio: v.Audio, Codec: v.Codec, Container: v.Container, Out: v.Out}
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
	o.MaxUploads = s.MaxUploads
	o.UploadParts = s.UploadParts
	o.RetryAttempts = s.RetryAttempts
	o.StagingFiles = s.StagingFiles
	o.StagingBytes = s.StagingBytes
	o.MinFreeBytes = s.MinFreeBytes
	o.UploadRateBPS = s.UploadRateBPS
	o.Lang = s.Language
	o.Theme = s.Theme
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

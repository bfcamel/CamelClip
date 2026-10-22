// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

// Package config defines application preferences and their repository contract.
package config

import (
	"context"
	"fmt"

	"github.com/bfcamel/CamelClip/internal/core"
)

type Settings struct {
	MaxUploads    int   `json:"max_concurrent_s3_uploads"`
	UploadParts   int   `json:"s3_multipart_parts"`
	RetryAttempts int   `json:"retry_attempts"`
	StagingFiles  int   `json:"max_staging_files"`
	StagingBytes  int64 `json:"max_staging_bytes"`
	MinFreeBytes  int64 `json:"min_free_bytes"`
	UploadRateBPS int64 `json:"s3_upload_rate_limit_bps"`

	Language        string `json:"language"`
	Mode            string `json:"mode"`
	Quality         string `json:"quality"`
	Audio           string `json:"audio_format"`
	Out             string `json:"output_directory"`
	Browser         string `json:"cookies_browser,omitempty"`
	Codec           string `json:"video_codec"`
	Container       string `json:"video_container"`
	MaxDownloads    int    `json:"max_concurrent_downloads"`
	MaxProcessing   int    `json:"max_concurrent_processing"`
	RateLimitBPS    int64  `json:"download_rate_limit_bps"`
	JobRateLimitBPS int64  `json:"job_rate_limit_bps"`
}
type Repository interface {
	Load(context.Context) (Settings, error)
	Save(context.Context, Settings) error
}

func (s Settings) Limits() core.Limits {
	return core.Limits{Downloads: s.MaxDownloads, Processing: s.MaxProcessing, GlobalBPS: s.RateLimitBPS, JobBPS: s.JobRateLimitBPS}
}
func (s Settings) QueuePolicy() core.QueuePolicy {
	return core.QueuePolicy{RetryAttempts: s.RetryAttempts, PendingFiles: s.StagingFiles, PendingBytes: s.StagingBytes, MinFreeBytes: s.MinFreeBytes}
}
func (s Settings) UploadOptions() core.UploadOptions {
	return (core.UploadOptions{Objects: s.MaxUploads, Parts: s.UploadParts, RetryAttempts: s.RetryAttempts, RateBPS: s.UploadRateBPS}).WithDefaults()
}
func Validate(s Settings) error {
	if err := s.QueuePolicy().Validate(); err != nil {
		return err
	}
	if err := s.UploadOptions().Validate(); err != nil {
		return err
	}
	if err := s.Limits().Validate(); err != nil {
		return err
	}
	if s.Codec == "" || s.Container == "" {
		return fmt.Errorf("video_codec and video_container are required")
	}
	if s.Language != "auto" && s.Language != "ru" && s.Language != "en" {
		return fmt.Errorf("invalid language")
	}
	return core.Validate(core.Request{URL: "https://example.com/video", Mode: s.Mode, Quality: s.Quality, Audio: s.Audio, Out: s.Out, Cookies: s.Browser, Codec: s.Codec, Container: s.Container})
}

// Defaults owns resource and media defaults shared by every local frontend.
func Defaults(out string) Settings {
	return Settings{Language: "auto", Mode: "video", Quality: "best", Audio: "mp3", Codec: "h264", Container: "mp4", Out: out, MaxDownloads: 5, MaxUploads: 2, UploadParts: 2, StagingFiles: 64, StagingBytes: 20 << 30, MinFreeBytes: 512 << 20}
}

// Normalize resolves documented zero resource defaults before validation/storage.
func Normalize(s Settings) (Settings, error) {
	if s.MaxDownloads == 0 {
		s.MaxDownloads = 5
	}
	uploads := s.UploadOptions()
	s.MaxUploads, s.UploadParts = uploads.Objects, uploads.Parts
	if err := Validate(s); err != nil {
		return s, err
	}
	r, err := core.NormalizeRequest(core.Request{URL: "https://example.com", Mode: s.Mode, Quality: s.Quality, Audio: s.Audio, Out: s.Out, Cookies: s.Browser, Codec: s.Codec, Container: s.Container})
	if err != nil {
		return s, err
	}
	s.Out = r.Out
	return s, nil
}

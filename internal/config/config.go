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
func Validate(s Settings) error {
	if s.MaxUploads < 0 || s.MaxUploads > 32 || s.UploadParts < 0 || s.UploadParts > 4 || s.RetryAttempts < 0 || s.RetryAttempts > 10 || s.StagingFiles < 0 || s.StagingBytes < 0 || s.MinFreeBytes < 0 || s.UploadRateBPS < 0 {
		return fmt.Errorf("invalid queue/upload limits")
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

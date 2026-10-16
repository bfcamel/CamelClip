// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

// Package bootstrap wires concrete adapters. No domain or UI package imports it.
package bootstrap

import (
	"context"
	"os/exec"
	"path/filepath"

	"github.com/bfcamel/video-downloader/internal/application"
	"github.com/bfcamel/video-downloader/internal/config"
	"github.com/bfcamel/video-downloader/internal/core"
	"github.com/bfcamel/video-downloader/internal/downloader/ytdlp"
	"github.com/bfcamel/video-downloader/internal/storage/jsonconfig"
	"github.com/bfcamel/video-downloader/internal/storage/sqlite"
)

func Downloader() core.Downloader {
	path, err := exec.LookPath("yt-dlp")
	if err != nil {
		path = "yt-dlp"
	}
	return ytdlp.New(path)
}
func OpenQueue(backend core.Downloader) func(context.Context, string, int) (application.Queue, error) {
	return func(ctx context.Context, config string, workers int) (application.Queue, error) {
		repo, err := sqlite.Open(ctx, filepath.Dir(config))
		if err != nil {
			return nil, err
		}
		service, err := application.New(ctx, backend, repo, workers)
		if err != nil {
			repo.Close()
			return nil, err
		}
		return service, nil
	}
}

func OpenConfig(path string, defaults config.Settings) config.Repository {
	return jsonconfig.File{Path: path, Defaults: defaults}
}

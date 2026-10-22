// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

// Package bootstrap wires concrete adapters. No domain or UI package imports it.
package bootstrap

import (
	"context"
	"os/exec"
	"path/filepath"

	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/config"
	"github.com/bfcamel/CamelClip/internal/core"
	"github.com/bfcamel/CamelClip/internal/downloader/ytdlp"
	"github.com/bfcamel/CamelClip/internal/library"
	"github.com/bfcamel/CamelClip/internal/security"
	"github.com/bfcamel/CamelClip/internal/storage/jsonconfig"
	"github.com/bfcamel/CamelClip/internal/storage/sqlite"
	"github.com/bfcamel/CamelClip/internal/systemmetrics"
	s3adapter "github.com/bfcamel/CamelClip/internal/uploader/s3"
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

func OpenWorkspace(backend core.Downloader) func(context.Context, string) (*application.Workspace, error) {
	return func(ctx context.Context, config string) (*application.Workspace, error) {
		repo, err := sqlite.Open(ctx, filepath.Dir(config))
		if err != nil {
			return nil, err
		}
		q, err := application.New(ctx, backend, repo, 5)
		if err != nil {
			repo.Close()
			return nil, err
		}
		lib := application.NewLibrary(repo, library.Prober{})
		lib.Enrich(ctx)
		return &application.Workspace{Queue: q, Library: lib, Cloud: application.NewCloud(ctx, repo, repo, repo, security.New(), func() core.CredentialVault { return security.New() }, s3adapter.Adapter{}), Monitor: application.NewSystemMonitor(ctx, systemmetrics.New())}, nil
	}
}

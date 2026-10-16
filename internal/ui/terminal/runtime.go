// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"context"

	"github.com/bfcamel/video-downloader/internal/application"
	"github.com/bfcamel/video-downloader/internal/config"
	"github.com/bfcamel/video-downloader/internal/core"
)

// Dependencies is supplied only by the composition root. This frontend knows no
// concrete downloader or database implementation and can be replaced by a GUI.
type Dependencies struct {
	OpenConfig    func(string, config.Settings) config.Repository
	Downloader    core.Downloader
	OpenWorkspace func(context.Context, string) (*application.Workspace, error)
	OpenQueue     func(context.Context, string, int) (application.Queue, error)
}

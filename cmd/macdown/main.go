// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/bfcamel/video-downloader/internal/bootstrap"
	"github.com/bfcamel/video-downloader/internal/ui/terminal"
)

func main() {
	backend := bootstrap.Downloader()
	terminal.Main(terminal.Dependencies{OpenConfig: bootstrap.OpenConfig, Downloader: backend, OpenQueue: bootstrap.OpenQueue(backend)})
}

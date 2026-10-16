// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

// Package ytdlp implements the core.Downloader port with yt-dlp and FFmpeg.
package ytdlp

import (
	"context"
	"io"
	"strings"

	"github.com/bfcamel/video-downloader/internal/core"
)

type options = core.Request
type mediaInfo = core.MediaInfo

var errCanceled = core.ErrCanceled
var videoCodec = core.VideoCodec
var videoContainer = core.VideoContainer
var validateVideoProfile = core.ValidateVideoProfile

func contains(v []string, s string) bool {
	for _, x := range v {
		if x == s {
			return true
		}
	}
	return false
}
func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, s)
}
func outputVariant(o options) string {
	if o.Mode == "audio" {
		return "audio-" + o.Audio
	}
	return "video-" + o.Quality + "-" + videoCodec(o) + "-" + videoContainer(o)
}

type Engine struct{ Executable string }

func New(executable string) *Engine { return &Engine{Executable: executable} }

var _ core.Downloader = (*Engine)(nil)

func (e *Engine) Inspect(ctx context.Context, r core.Request) (core.MediaInfo, error) {
	if err := core.Validate(r); err != nil {
		return core.MediaInfo{}, err
	}
	return inspect(ctx, e.Executable, r)
}

type eventWriter struct {
	observer core.Observer
	pending  string
	last     string
}

func (w *eventWriter) Write(data []byte) (int, error) {
	w.pending += string(data)
	for {
		i := strings.IndexAny(w.pending, "\r\n")
		if i < 0 {
			break
		}
		line := safeText(w.pending[:i])
		w.pending = w.pending[i+1:]
		if w.observer != nil && line != "" {
			kind := core.Log
			if strings.HasPrefix(line, "[download]") {
				kind = core.Progress
			}
			w.observer(core.Event{Kind: kind, Stage: core.Downloading, Detail: line})
		}
	}
	if len(w.pending) > 16384 {
		w.pending = ""
	}
	return len(data), nil
}
func (e *Engine) Download(ctx context.Context, r core.Request, observer core.Observer) (core.Result, error) {
	if err := core.Validate(r); err != nil {
		return core.Result{}, err
	}
	result := core.Result{}
	w := &eventWriter{observer: observer}
	err := verifiedDownloadWithObserver(ctx, e.Executable, r, w, func(stage, detail string) {
		if stage == "published" {
			result.Path = detail
			if observer != nil {
				observer(core.Event{Kind: core.Published, Path: detail})
			}
			return
		}
		if observer != nil {
			kind := core.StageChanged
			if detail != "" {
				kind = core.Progress
			}
			observer(core.Event{Kind: kind, Stage: core.Stage(stage), Detail: detail})
		}
	})
	if ctx.Err() != nil {
		return result, core.ErrCanceled
	}
	return result, err
}
func (e *Engine) Formats(ctx context.Context, r core.Request, observer core.Observer) error {
	if err := core.Validate(r); err != nil {
		return err
	}
	c := command(ctx, e.Executable, formatArguments(r)...)
	w := &eventWriter{observer: observer}
	c.Stdout = w
	c.Stderr = w
	err := c.Run()
	if ctx.Err() != nil {
		return core.ErrCanceled
	}
	return err
}

// Compile-time interface checks include standard stream adapters.
var _ io.Writer = (*eventWriter)(nil)

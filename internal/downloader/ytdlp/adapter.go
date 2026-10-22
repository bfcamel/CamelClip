// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

// Package ytdlp implements the core.Downloader port with yt-dlp and FFmpeg.
package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/bfcamel/CamelClip/internal/core"
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
	observer    core.Observer
	pending     string
	last        string
	files       map[string]int64
	totals      map[string]int64
	parts       int
	finished    map[string]bool
	ratios      map[string]float64
	order       []string
	lastPercent float64
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
		if strings.HasPrefix(line, "CAMELCLIP_PROGRESS ") {
			w.measure(strings.TrimPrefix(line, "CAMELCLIP_PROGRESS "))
			continue
		}
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
	parts := 1
	if r.Mode == "video" {
		parts = 2
	}
	if r.ExpectedParts > 0 {
		parts = r.ExpectedParts
	}
	w := &eventWriter{observer: observer, parts: parts}
	err := verifiedDownloadWithObserver(ctx, e.Executable, r, w, func(event core.Event) {
		if event.Kind == core.Published {
			result.Path = event.Path
		}
		if observer != nil {
			observer(event)
		}
	})
	if ctx.Err() != nil {
		return result, core.ErrCanceled
	}
	return result, backendFailure(err)
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

func number(raw json.RawMessage) *float64 {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var n float64
	if json.Unmarshal(raw, &n) != nil {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return nil
		}
		v, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil
		}
		n = v
	}
	if n < 0 || n > 1e18 || math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	return &n
}
func (w *eventWriter) measure(line string) {
	var data struct {
		Downloaded json.RawMessage `json:"downloaded_bytes"`
		Total      json.RawMessage `json:"total_bytes"`
		Speed      json.RawMessage `json:"speed"`
		ETA        json.RawMessage `json:"eta"`
		Filename   string          `json:"filename"`
		Estimate   json.RawMessage `json:"total_bytes_estimate"`
		Fragment   json.RawMessage `json:"fragment_index"`
		Fragments  json.RawMessage `json:"fragment_count"`
		Status     string          `json:"status"`
	}
	if json.Unmarshal([]byte(line), &data) != nil || w.observer == nil {
		return
	}
	if w.files == nil {
		w.files = map[string]int64{}
		w.totals = map[string]int64{}
		w.finished = map[string]bool{}
		w.ratios = map[string]float64{}
	}
	downloaded := number(data.Downloaded)
	if downloaded == nil {
		return
	}
	if _, exists := w.files[data.Filename]; !exists && len(w.files) >= 4 {
		return
	}
	if _, ok := w.files[data.Filename]; !ok {
		w.order = append(w.order, data.Filename)
	}
	w.files[data.Filename] = int64(*downloaded)
	if total := number(data.Total); total != nil && *total > 0 {
		w.totals[data.Filename] = int64(*total)
	}
	m := core.Metrics{SpeedBPS: number(data.Speed), ETASeconds: number(data.ETA), At: time.Now()}
	for _, n := range w.files {
		m.DownloadedBytes += n
	}
	parts := max(max(w.parts, 1), len(w.files))
	m.PartCount = parts
	m.PartIndex = len(w.order)
	ratio := 0.0
	known := false
	if total := number(data.Total); total != nil && *total > 0 {
		ratio = min(1, *downloaded / *total)
		known = true
	} else if total := number(data.Estimate); total != nil && *total > 0 {
		ratio = min(1, *downloaded / *total)
		known = true
		m.ProgressEstimated = true
	} else if count := number(data.Fragments); count != nil && *count > 0 {
		if index := number(data.Fragment); index != nil {
			ratio = min(1, *index / *count)
			known = true
			m.ProgressEstimated = true
		}
	}
	if data.Status == "finished" {
		ratio = 1
		known = true
		w.finished[data.Filename] = true
	}
	if known {
		w.ratios[data.Filename] = max(w.ratios[data.Filename], ratio)
		sum := 0.0
		for _, v := range w.ratios {
			sum += v
		}
		percent := min(99.9, sum/float64(parts)*100)
		w.lastPercent = max(w.lastPercent, percent)
		percent = w.lastPercent
		m.Percent = &percent
	}
	if len(w.totals) >= parts && len(w.totals) == len(w.files) {
		total := int64(0)
		for _, n := range w.totals {
			total += n
		}
		m.TotalBytes = &total
	}
	// Separate tracks have unknown relative sizes until both are reported. Use an
	// explicitly approximate equal-track phase percentage rather than hiding it.
	if parts > 1 {
		m.ProgressEstimated = true
	}

	detail := ""
	if m.Percent != nil {
		detail = strconv.FormatFloat(*m.Percent, 'f', 0, 64) + "%"
	}
	w.observer(core.Event{Kind: core.Progress, Stage: core.Downloading, Metrics: &m, Detail: detail})
}

func backendFailure(err error) error {
	if err == nil {
		return nil
	}
	if core.CodeOf(err) != core.CodeBackend {
		return err
	}
	text := strings.ToLower(err.Error())
	for _, v := range []string{"premiere will begin", "live event will begin", "not yet available"} {
		if strings.Contains(text, v) {
			return &core.Failure{Code: core.CodeNotReady, Message: err.Error(), Cause: err}
		}
	}
	temporary := false
	for _, fragment := range []string{"timeout", "timed out", "connection reset", "connection refused", "temporary", "429", "502", "503", "504", "unexpected eof"} {
		if strings.Contains(text, fragment) {
			temporary = true
			break
		}
	}
	return &core.Failure{Code: core.CodeBackend, Message: err.Error(), Temporary: temporary, Cause: err}
}

// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

// Package core defines UI-independent downloader contracts and value types.
package core

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

var ErrCanceled = errors.New("operation canceled")

// Request is an immutable description of one download and its desired output.
type Request struct {
	URL, Mode, Quality, Audio, Out, Cookies, Codec, Container string
	Duration                                                  float64
	RateLimitBPS                                              int64
	ExpectedParts                                             int
}
type MediaFormat struct {
	Height int    `json:"height"`
	VCodec string `json:"vcodec"`
	ACodec string `json:"acodec"`
	DRM    bool   `json:"has_drm"`
}
type MediaInfo struct {
	ID         string        `json:"id"`
	Extractor  string        `json:"extractor_key"`
	Title      string        `json:"title"`
	Duration   float64       `json:"duration"`
	Formats    []MediaFormat `json:"formats"`
	Type       string        `json:"_type"`
	LiveStatus string        `json:"live_status"`
	Live       bool          `json:"is_live"`
}

func Validate(o Request) error {
	if !finiteNonnegative(o.Duration) || o.ExpectedParts < 0 || o.ExpectedParts > 4 {
		return errors.New("invalid duration or expected part count")
	}
	if strings.ContainsRune(o.Out, '\x00') {
		return errors.New("output path contains a null byte")
	}
	if o.RateLimitBPS < 0 {
		return errors.New("rate limit must be nonnegative")
	}
	u, err := url.Parse(o.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("a valid http:// or https:// URL is required")
	}
	if o.Mode != "video" && o.Mode != "audio" {
		return errors.New("mode must be video or audio")
	}
	if o.Quality != "best" {
		n, err := strconv.Atoi(o.Quality)
		if err != nil || n < 1 || n > 16384 {
			return errors.New("quality must be best or a height from 1 to 16384")
		}
	}
	if !contains([]string{"mp3", "m4a", "opus", "flac", "wav", "best"}, o.Audio) {
		return errors.New("unsupported audio format")
	}
	if o.Cookies != "" && !ValidBrowser(o.Cookies) {
		return errors.New("unsupported browser")
	}
	if err := ValidateVideoProfile(o); err != nil {
		return err
	}
	if strings.TrimSpace(o.Out) == "" {
		return errors.New("output directory cannot be empty")
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func ValidBrowser(value string) bool {
	name := strings.SplitN(value, ":", 2)[0]
	return contains([]string{"safari", "chrome", "firefox", "edge", "brave"}, name) && !strings.ContainsAny(value, "\r\n\x00")
}

type Stage string

const (
	Downloading Stage = "downloading"
	Processing  Stage = "processing"
	Verifying   Stage = "verifying"
)

type EventKind string

const (
	StageChanged EventKind = "stage"
	Progress     EventKind = "progress"
	Log          EventKind = "log"
	Published    EventKind = "published"
)

// Event is independent of display, localization and persistence.
type Event struct {
	Kind    EventKind
	Stage   Stage
	Detail  string
	Path    string
	Metrics *Metrics
}

// Observer receives serialized notifications for a single download.
type Observer func(Event)
type Result struct{ Path string }

// Downloader is the only interface required by the queue service. Implementations
// must honor cancellation, finish child processes, and serialize observer calls.
// Callbacks finish before Download returns, must not reenter the queue, and may
// block for resource acquisition. Stages move forward (repeats are allowed).
// Metrics must be finite/nonnegative. A successful result has an absolute path
// inside Request.Out; if Published is emitted its path equals Result.Path.
type Downloader interface {
	Inspect(context.Context, Request) (MediaInfo, error)
	Download(context.Context, Request, Observer) (Result, error)
	Formats(context.Context, Request, Observer) error
}

func VideoCodec(o Request) string {
	if o.Codec == "" {
		return "h264"
	}
	return o.Codec
}
func VideoContainer(o Request) string {
	if o.Container == "" {
		return "mp4"
	}
	return o.Container
}
func ValidateVideoProfile(o Request) error {
	codec, container := VideoCodec(o), VideoContainer(o)
	if !contains([]string{"h264", "hevc", "vp9", "av1"}, codec) {
		return fmt.Errorf("unsupported video codec: %s", codec)
	}
	if !contains([]string{"mp4", "mkv", "webm"}, container) {
		return fmt.Errorf("unsupported video container: %s", container)
	}
	if container == "webm" && codec != "vp9" && codec != "av1" {
		return fmt.Errorf("WebM supports only VP9 or AV1")
	}
	return nil
}

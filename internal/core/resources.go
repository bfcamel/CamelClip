// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package core

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Limits struct {
	Downloads  int   `json:"max_concurrent_downloads"`
	Processing int   `json:"max_concurrent_processing"` // 0 = auto
	GlobalBPS  int64 `json:"download_rate_limit_bps"`
	JobBPS     int64 `json:"job_rate_limit_bps"`
}

func AutoProcessing(cpus int) int { return max(cpus/4, 2) }
func (l Limits) Effective() Limits {
	if l.Downloads == 0 {
		l.Downloads = 5
	}
	if l.Processing == 0 {
		l.Processing = AutoProcessing(runtime.NumCPU())
	}
	return l
}
func (l Limits) Validate() error {
	if l.Downloads < 0 || l.Downloads > 32 || l.Processing < 0 || l.Processing > 32 || l.GlobalBPS < 0 || l.JobBPS < 0 {
		return fmt.Errorf("concurrency must be 1..32 (processing: 0 = auto); rates must be nonnegative")
	}
	if l.GlobalBPS > 0 && l.GlobalBPS < int64(l.Effective().Downloads) {
		return fmt.Errorf("global budget must allow at least 1 byte/s per download")
	}
	return nil
}
func ParseRate(value string) (int64, error) {
	text := strings.TrimSpace(value)
	if text == "0" {
		return 0, nil
	}
	units := []struct {
		suffix string
		scale  float64
	}{{"MiB/s", 1024 * 1024}, {"KiB/s", 1024}, {"B/s", 1}}
	scale := float64(1)
	for _, unit := range units {
		if strings.HasSuffix(text, unit.suffix) {
			text = strings.TrimSpace(strings.TrimSuffix(text, unit.suffix))
			scale = unit.scale
			break
		}
	}
	n, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 || n*scale >= float64(math.MaxInt64) || math.Trunc(n*scale) != n*scale {
		return 0, fmt.Errorf("use 0, a positive byte/s integer, or e.g. 512KiB/s / 2MiB/s")
	}
	return int64(n * scale), nil
}

// Metrics are numerical measurements from the backend, not rendered text.
type Metrics struct {
	DownloadedBytes int64     `json:"downloaded_bytes"`
	TotalBytes      *int64    `json:"total_bytes,omitempty"`
	SpeedBPS        *float64  `json:"speed_bps,omitempty"`
	ETASeconds      *float64  `json:"eta_seconds,omitempty"`
	Percent         *float64  `json:"phase_progress,omitempty"`
	At              time.Time `json:"timestamp"`
}

func CloneMetrics(m Metrics) Metrics {
	if m.TotalBytes != nil {
		v := *m.TotalBytes
		m.TotalBytes = &v
	}
	if m.SpeedBPS != nil {
		v := *m.SpeedBPS
		m.SpeedBPS = &v
	}
	if m.ETASeconds != nil {
		v := *m.ETASeconds
		m.ETASeconds = &v
	}
	if m.Percent != nil {
		v := *m.Percent
		m.Percent = &v
	}
	return m
}

type Resources interface {
	Enter(context.Context, Stage) error
	RateBPS() int64
}
type resourceKey struct{}

func WithResources(ctx context.Context, r Resources) context.Context {
	return context.WithValue(ctx, resourceKey{}, r)
}
func EnterStage(ctx context.Context, phase Stage) error {
	if r, ok := ctx.Value(resourceKey{}).(Resources); ok {
		return r.Enter(ctx, phase)
	}
	return ctx.Err()
}
func DownloadRate(ctx context.Context, fallback int64) int64 {
	if r, ok := ctx.Value(resourceKey{}).(Resources); ok {
		rate := r.RateBPS()
		if fallback > 0 && (rate == 0 || fallback < rate) {
			return fallback
		}
		return rate
	}
	return fallback
}

// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package ytdlp

import (
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"math"
	"strconv"
	"strings"
	"time"
)

type pipelineObserver func(core.Event)

func notifyStage(observer pipelineObserver, stage string) {
	if observer != nil {
		observer(core.Event{Kind: core.StageChanged, Stage: core.Stage(stage)})
	}

}

// FFmpeg emits machine-readable microsecond timestamps on stdout. Percentages belong
// to the current stage, and never indicate that the whole job is already complete.
type ffmpegProgress struct {
	pending  string
	duration float64
	stage    string
	observer pipelineObserver
	last     int
	reported bool
}

func (p *ffmpegProgress) Write(data []byte) (int, error) {
	p.pending += string(data)
	for {
		i := strings.IndexByte(p.pending, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimSpace(p.pending[:i])
		p.pending = p.pending[i+1:]
		if !strings.HasPrefix(line, "out_time_us=") || p.duration <= 0 || p.observer == nil {
			continue
		}
		micros, err := strconv.ParseFloat(strings.TrimPrefix(line, "out_time_us="), 64)
		if err != nil || math.IsNaN(micros) || math.IsInf(micros, 0) {
			continue
		}
		percent := int(micros / 1e6 / p.duration * 100)
		if percent < 0 {
			percent = 0
		}
		if percent > 99 {
			percent = 99
		}
		if !p.reported || percent != p.last {
			p.last = percent
			p.reported = true
			value := float64(percent)
			p.observer(core.Event{Kind: core.Progress, Stage: core.Stage(p.stage), Detail: fmt.Sprintf("%d%%", percent), Metrics: &core.Metrics{Percent: &value, At: time.Now()}})
		}
	}
	if len(p.pending) > 16384 {
		p.pending = ""
	}
	return len(data), nil
}

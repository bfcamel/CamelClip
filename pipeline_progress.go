package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

type pipelineObserver func(stage, detail string)

func notifyStage(observer pipelineObserver, output io.Writer, stage string) {
	if observer != nil {
		observer(stage, "")
	}
	fmt.Fprintln(output, jobState(stage)+"…")
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
		if err != nil {
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
			p.observer(p.stage, fmt.Sprintf("%d%%", percent))
		}
	}
	if len(p.pending) > 16384 {
		p.pending = ""
	}
	return len(data), nil
}

type downloaderOutput struct {
	out        io.Writer
	observer   pipelineObserver
	pending    string
	processing bool
}

func (w *downloaderOutput) Write(data []byte) (int, error) {
	w.pending += string(data)
	for {
		i := strings.IndexAny(w.pending, "\r\n")
		if i < 0 {
			break
		}
		line := w.pending[:i]
		w.pending = w.pending[i+1:]
		if !w.processing && (strings.HasPrefix(line, "[Merger]") || strings.HasPrefix(line, "[ExtractAudio]") || strings.HasPrefix(line, "[VideoRemuxer]")) {
			w.processing = true
			if w.observer != nil {
				w.observer("processing", "")
			}
		}
	}
	if len(w.pending) > 16384 {
		w.pending = ""
	}
	return w.out.Write(data)
}

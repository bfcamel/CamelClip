// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"github.com/bfcamel/CamelClip/internal/core"
	"time"
)

type rateSample struct {
	at    time.Time
	speed float64
}
type Statistics struct {
	Downloads, Processing, Metadata, WaitingDownloads, WaitingProcessing int
	Limits                                                               core.Limits
	SpeedBPS                                                             *float64
	SessionBytes                                                         int64
}

func (s *Service) SetLimits(l core.Limits) error {
	if err := l.Validate(); err != nil {
		return err
	}
	s.resources.update(l)
	return nil
}
func (s *Service) resourcePhase(id int, stage core.Stage, waiting bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.findLocked(id)
	if err != nil || s.closing || s.jobs[i].State == Canceling {
		return
	}
	state := State(stage)
	if waiting {
		switch stage {
		case core.Downloading:
			state = WaitingDownload
		case core.Processing:
			state = WaitingProcessing
		case core.Verifying:
			state = WaitingVerification
		}
	}
	j := &s.jobs[i]
	if j.State == state {
		return
	}
	old := j.State
	j.State = state
	s.syncOrderLocked()
	j.Detail = ""
	j.Monitoring.UpdatedAt = time.Now()
	if stage != core.Downloading && old == Downloading {
		n := 100.0
		j.Monitoring.Download.Percent = &n
	}
	if stage == core.Verifying && old == Processing {
		n := 100.0
		j.Monitoring.Process.Percent = &n
	}
	if stage != core.Downloading {
		j.Monitoring.Download.SpeedBPS = nil
		j.Monitoring.Download.ETASeconds = nil
	}
	if s.saveLocked() != nil {
		return
	}
	s.emitLocked(*j, core.StageChanged, "")
}
func (s *Service) measureLocked(j *Job, e core.Event) {
	if e.Metrics == nil {
		return
	}
	m := core.CloneMetrics(*e.Metrics)
	if m.At.IsZero() {
		m.At = time.Now()
	}
	j.Monitoring.UpdatedAt = m.At
	switch e.Stage {
	case core.Downloading:
		s.sessionBytes += max(int64(0), m.DownloadedBytes-j.Monitoring.Download.DownloadedBytes)
		if m.SpeedBPS != nil {
			points := s.samples[j.ID]
			cut := m.At.Add(-3 * time.Second)
			start := 0
			for start < len(points) && points[start].at.Before(cut) {
				start++
			}
			points = append(points[start:], rateSample{m.At, *m.SpeedBPS})
			if len(points) > 64 {
				points = points[len(points)-64:]
			}
			s.samples[j.ID] = points
			sum := 0.0
			for _, p := range points {
				sum += p.speed
			}
			average := sum / float64(len(points))
			m.SpeedBPS = &average
		}
		j.Monitoring.Download = m
	case core.Processing:
		j.Monitoring.Process = m
	case core.Verifying:
		j.Monitoring.Verify = m
	}
}
func (s *Service) Statistics() Statistics {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources.mu.Lock()
	r := Statistics{Downloads: s.resources.active["downloading"], Processing: s.resources.active["processing"], Metadata: s.metadataActive, Limits: s.resources.limits, SessionBytes: s.sessionBytes}
	s.resources.mu.Unlock()
	speed := 0.0
	known := false
	for _, j := range s.jobs {
		if j.State == WaitingDownload || j.State == WaitingDisk {
			r.WaitingDownloads++
		}
		if j.State == WaitingProcessing || j.State == WaitingVerification {
			r.WaitingProcessing++
		}
		if j.State == Downloading && time.Since(j.Monitoring.Download.At) < 3*time.Second && j.Monitoring.Download.SpeedBPS != nil {
			speed += *j.Monitoring.Download.SpeedBPS
			known = true
		}
	}
	if known {
		r.SpeedBPS = &speed
	}
	return r
}

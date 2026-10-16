// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package systemmetrics

import (
	"context"
	"github.com/bfcamel/video-downloader/internal/core"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
	"math"
	"os"
	"sync"
	"time"
)

type Collector struct {
	mu       sync.Mutex
	previous []cpu.TimesStat
}

func New() *Collector { return &Collector{} }
func total(t cpu.TimesStat) float64 {
	return t.User + t.System + t.Nice + t.Idle + t.Iowait + t.Irq + t.Softirq + t.Steal
}
func Percent(old, next cpu.TimesStat) *float64 {
	all := total(next) - total(old)
	idle := next.Idle + next.Iowait - old.Idle - old.Iowait
	if all <= 0 || idle < 0 || math.IsNaN(all) || math.IsInf(all, 0) {
		return nil
	}
	n := max(0.0, min(100.0, (all-idle)/all*100))
	return &n
}
func (c *Collector) Collect(ctx context.Context) (core.SystemSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := core.SystemSnapshot{At: time.Now()}
	times, e := cpu.TimesWithContext(ctx, true)
	if e == nil {
		s.Cores = make([]*float64, len(times))
		sum, count := 0.0, 0
		for i, t := range times {
			if i < len(c.previous) && c.previous[i].CPU == t.CPU {
				s.Cores[i] = Percent(c.previous[i], t)
				if s.Cores[i] != nil {
					sum += *s.Cores[i]
					count++
				}
			}
		}
		if count == len(times) && count > 0 {
			v := sum / float64(count)
			s.CPU = &v
		}
		c.previous = times
	} else {
		s.Error = "CPU unavailable"
	}
	v, e := mem.VirtualMemoryWithContext(ctx)
	if e == nil && v.Total > 0 && v.Available <= v.Total {
		used := v.Total - v.Available
		s.RAMTotal = &v.Total
		s.RAMAvailable = &v.Available
		s.RAMUsed = &used
	} else {
		s.Error = "System metrics partly unavailable"
	}
	if p, e := process.NewProcessWithContext(ctx, int32(os.Getpid())); e == nil {
		if m, e := p.MemoryInfoWithContext(ctx); e == nil {
			s.ProcessRSS = &m.RSS
		}
	}
	return s, nil
}

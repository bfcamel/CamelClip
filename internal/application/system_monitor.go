// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"github.com/bfcamel/CamelClip/internal/core"
	"sync"
	"time"
)

type SystemMonitor struct {
	mu       sync.RWMutex
	snapshot core.SystemSnapshot
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func NewSystemMonitor(parent context.Context, c core.SystemMetricsCollector) *SystemMonitor {
	ctx, cancel := context.WithCancel(parent)
	m := &SystemMonitor{cancel: cancel}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			callCtx, stop := context.WithTimeout(ctx, 800*time.Millisecond)
			s, e := c.Collect(callCtx)
			stop()
			if e != nil {
				s = core.SystemSnapshot{At: time.Now(), Error: "System metrics unavailable"}
			}
			m.mu.Lock()
			m.snapshot = s
			m.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
	return m
}
func (m *SystemMonitor) Snapshot() core.SystemSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.snapshot
	s.Cores = append([]*float64(nil), s.Cores...)
	copyFloat := func(p *float64) *float64 {
		if p == nil {
			return nil
		}
		v := *p
		return &v
	}
	s.CPU = copyFloat(s.CPU)
	for i, p := range s.Cores {
		s.Cores[i] = copyFloat(p)
	}
	copyUint := func(p *uint64) *uint64 {
		if p == nil {
			return nil
		}
		v := *p
		return &v
	}
	s.RAMUsed = copyUint(s.RAMUsed)
	s.RAMTotal = copyUint(s.RAMTotal)
	s.RAMAvailable = copyUint(s.RAMAvailable)
	s.ProcessRSS = copyUint(s.ProcessRSS)
	return s
}
func (m *SystemMonitor) Close() { m.cancel(); m.wg.Wait() }

// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"github.com/bfcamel/video-downloader/internal/core"
	"sync"
)

type permit struct {
	pool    string
	ready   chan struct{}
	rate    int64
	granted bool
}
type resources struct {
	mu       sync.Mutex
	limits   core.Limits
	active   map[string]int
	reserved int64
	waiting  []*permit
}

func newResources() *resources {
	return &resources{limits: core.Limits{Downloads: 5}, active: map[string]int{}}
}
func (r *resources) update(l core.Limits) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limits = l
	r.dispatch()
}
func (r *resources) dispatch() {
	limits := r.limits.Effective()
	rest := r.waiting[:0]
	blocked := map[string]bool{}
	for _, p := range r.waiting {
		maximum := limits.Downloads
		if p.pool == "processing" {
			maximum = limits.Processing
		}
		if r.active[p.pool] >= maximum || blocked[p.pool] {
			rest = append(rest, p)
			continue
		}
		rate := limits.JobBPS
		if p.pool == "downloading" && limits.GlobalBPS > 0 {
			share := limits.GlobalBPS / int64(limits.Downloads)
			if rate == 0 || share < rate {
				rate = share
			}
			// Older children keep their reserved budget until they have actually exited.
			if r.reserved+rate > limits.GlobalBPS {
				blocked[p.pool] = true
				rest = append(rest, p)
				continue
			}
		}
		p.rate = rate
		p.granted = true
		r.active[p.pool]++
		if p.pool == "downloading" {
			r.reserved += p.rate
		}
		close(p.ready)
	}
	r.waiting = rest
}
func (r *resources) acquire(ctx context.Context, pool string) (*permit, error) {
	p := &permit{pool: pool, ready: make(chan struct{})}
	r.mu.Lock()
	r.waiting = append(r.waiting, p)
	r.dispatch()
	r.mu.Unlock()
	select {
	case <-p.ready:
		if ctx.Err() != nil {
			r.release(p)
			return nil, ctx.Err()
		}
		return p, nil
	case <-ctx.Done():
		r.mu.Lock()
		if p.granted {
			r.active[p.pool]--
			if p.pool == "downloading" {
				r.reserved -= p.rate
			}
		} else {
			for i, v := range r.waiting {
				if v == p {
					r.waiting = append(r.waiting[:i], r.waiting[i+1:]...)
					break
				}
			}
		}
		r.dispatch()
		r.mu.Unlock()
		return nil, ctx.Err()
	}
}
func (r *resources) release(p *permit) {
	if p == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active[p.pool]--
	if p.pool == "downloading" {
		r.reserved -= p.rate
	}
	r.dispatch()
}

type lease struct {
	manager *resources
	permit  *permit
	phase   func(core.Stage, bool)
	mu      sync.Mutex
}

func (l *lease) Enter(ctx context.Context, stage core.Stage) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	pool := "processing"
	if stage == core.Downloading {
		pool = "downloading"
	}
	if l.permit != nil && l.permit.pool == pool {
		l.phase(stage, false)
		return nil
	}
	l.manager.release(l.permit)
	l.permit = nil
	l.phase(stage, true)
	p, err := l.manager.acquire(ctx, pool)
	if err != nil {
		return err
	}
	l.permit = p
	l.phase(stage, false)
	return nil
}
func (l *lease) RateBPS() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.permit == nil {
		return 0
	}
	return l.permit.rate
}
func (l *lease) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.manager.release(l.permit)
	l.permit = nil
}

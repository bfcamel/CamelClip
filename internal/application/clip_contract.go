// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"github.com/bfcamel/CamelClip/internal/core"
	"sync"
)

// A closed observer ignores stale callbacks. Concurrent callbacks are serialized
// before touching application progress; invalid metrics fail the current attempt.
type clipObserver struct {
	mu      sync.Mutex
	closed  bool
	failure error
	last    int
	render  bool
	next    func(core.ClipProgress)
}

func (g *clipObserver) observe(p core.ClipProgress) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || g.failure != nil {
		return
	}
	if e := core.ValidateMetrics(p.Metrics); e != nil {
		g.failure = e
		return
	}
	rank := 0
	if g.render {
		switch p.State {
		case core.ClipRendering:
			rank = 1
		case core.ClipVerifying:
			rank = 2
		}
	} else {
		switch p.State {
		case core.ClipDownloading:
			rank = 1
		case core.ClipWaitingProcessing, core.ClipRendering:
			rank = 2
		}
	}
	if rank == 0 || rank < g.last {
		g.failure = core.ContractError("invalid clip adapter phase")
		return
	}
	g.last = rank
	p.Metrics = core.CloneMetrics(p.Metrics)
	if g.next != nil {
		g.next(p)
	}
}
func (g *clipObserver) close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closed = true
	return g.failure
}
func safeClipPrepare(backend core.ClipBackend, ctx context.Context, p core.ClipPlan, local, work string, observer func(core.ClipProgress)) (path string, source core.ClipSource, err error) {
	g := &clipObserver{next: observer}
	defer func() {
		if recover() != nil {
			err = core.ContractError("clip source adapter panicked")
		}
		if e := g.close(); e != nil {
			err = e
		}
	}()
	return backend.Prepare(ctx, p, local, work, g.observe)
}
func safeClipRender(backend core.ClipBackend, ctx context.Context, source, candidate string, p core.ClipPlan, seg core.ClipSegment, observer func(core.ClipProgress)) (ev core.ClipEvidence, err error) {
	g := &clipObserver{render: true, next: observer}
	defer func() {
		if recover() != nil {
			err = core.ContractError("clip renderer panicked")
		}
		if e := g.close(); e != nil {
			err = e
		}
	}()
	return backend.Render(ctx, source, candidate, p, seg, g.observe)
}
func safeClipInspect(backend core.ClipBackend, ctx context.Context, r core.ClipRequest, local string) (source core.ClipSource, err error) {
	defer func() {
		if recover() != nil {
			err = core.ContractError("clip inspection adapter panicked")
		}
	}()
	return backend.Inspect(ctx, r, local)
}

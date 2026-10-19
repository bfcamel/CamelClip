// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"sync/atomic"
	"testing"
	"time"
)

type stagedBackend struct {
	fetch, process                                                            chan struct{}
	downloads, processing, metadata, maxDownloads, maxProcessing, maxMetadata atomic.Int32
	starts                                                                    atomic.Int32
	holdMetadata                                                              bool
}

func maximum(counter *atomic.Int32, n int32) {
	for {
		old := counter.Load()
		if n <= old || counter.CompareAndSwap(old, n) {
			return
		}
	}
}
func hold(ctx context.Context, ch <-chan struct{}) error {
	select {
	case <-ctx.Done():
		return core.ErrCanceled
	case <-ch:
		return nil
	}
}
func (b *stagedBackend) Inspect(ctx context.Context, r core.Request) (core.MediaInfo, error) {
	n := b.metadata.Add(1)
	maximum(&b.maxMetadata, n)
	defer b.metadata.Add(-1)
	if b.holdMetadata {
		if err := hold(ctx, b.fetch); err != nil {
			return core.MediaInfo{}, err
		}
	}
	return core.MediaInfo{ID: r.URL, Title: r.URL}, nil
}
func (b *stagedBackend) Download(ctx context.Context, r core.Request, observe core.Observer) (core.Result, error) {
	observe(core.Event{Kind: core.StageChanged, Stage: core.Downloading})
	if ctx.Err() != nil {
		return core.Result{}, core.ErrCanceled
	}
	n := b.downloads.Add(1)
	maximum(&b.maxDownloads, n)
	b.starts.Add(1)
	err := hold(ctx, b.fetch)
	b.downloads.Add(-1)
	if err != nil {
		return core.Result{}, err
	}
	observe(core.Event{Kind: core.StageChanged, Stage: core.Processing})
	if ctx.Err() != nil {
		return core.Result{}, core.ErrCanceled
	}
	n = b.processing.Add(1)
	maximum(&b.maxProcessing, n)
	err = hold(ctx, b.process)
	b.processing.Add(-1)
	if err != nil {
		return core.Result{}, err
	}
	observe(core.Event{Kind: core.StageChanged, Stage: core.Verifying})
	return core.Result{Path: r.Out + "/verified.mp4"}, nil
}
func (b *stagedBackend) Formats(context.Context, core.Request, core.Observer) error { return nil }
func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition timed out")
}
func TestIndependentPoolsLiveResizeAndCancellation(t *testing.T) {
	b := &stagedBackend{fetch: make(chan struct{}, 100), process: make(chan struct{}, 100)}
	q, err := New(context.Background(), b, &memoryRepository{}, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	if err = q.SetLimits(core.Limits{Downloads: 5, Processing: 2}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		r := req()
		r.URL = fmt.Sprintf("https://example.com/%d", i)
		if _, err = q.Submit(r, false); err != nil {
			t.Fatal(err)
		}
	}
	q.Start()
	waitUntil(t, func() bool { return q.Statistics().Downloads == 5 })
	b.fetch <- struct{}{}
	b.fetch <- struct{}{}
	waitUntil(t, func() bool { s := q.Statistics(); return s.Downloads == 5 && s.Processing == 2 })
	for i := 0; i < 5; i++ {
		b.fetch <- struct{}{}
	}
	waitUntil(t, func() bool { s := q.Statistics(); return s.WaitingProcessing >= 5 && s.Downloads == 5 })
	if b.maxDownloads.Load() > 5 || b.maxProcessing.Load() > 2 || b.maxMetadata.Load() > 5 {
		t.Fatal("resource ceiling exceeded")
	}
	if err = q.SetLimits(core.Limits{Downloads: 2, Processing: 2}); err != nil {
		t.Fatal(err)
	}
	before := b.starts.Load()
	b.fetch <- struct{}{}
	waitUntil(t, func() bool { return q.Statistics().Downloads == 4 })
	time.Sleep(40 * time.Millisecond)
	if b.starts.Load() != before {
		t.Fatal("lowering limit started another child")
	}
	if err = q.SetLimits(core.Limits{Downloads: 8, Processing: 2}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return q.Statistics().Downloads == 8 })
	for _, job := range q.Jobs() {
		if err = q.Cancel(job.ID); err != nil {
			t.Fatal(err)
		}
	}
	waitUntil(t, func() bool { s := q.Statistics(); return s.Downloads == 0 && s.Processing == 0 && s.Metadata == 0 })
}
func TestMetadataHasItsOwnBound(t *testing.T) {
	b := &stagedBackend{fetch: make(chan struct{}, 100), process: make(chan struct{}, 100), holdMetadata: true}
	q, err := New(context.Background(), b, &memoryRepository{}, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	for i := 0; i < 20; i++ {
		r := req()
		r.URL = fmt.Sprintf("https://example.com/%d", i)
		q.Submit(r, false)
	}
	q.Start()
	waitUntil(t, func() bool { return q.Statistics().Metadata == 5 })
	time.Sleep(30 * time.Millisecond)
	if b.metadata.Load() != 5 || b.maxMetadata.Load() > 5 || q.Statistics().Downloads != 0 {
		t.Fatal("metadata pool is unbounded or shares a download slot")
	}
}
func TestFullProcessingPoolReleasesDownloadPermitsBeforeWaiting(t *testing.T) {
	manager := newResources()
	manager.update(core.Limits{Downloads: 1, Processing: 1})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, _ := manager.acquire(ctx, "processing")
	l := &lease{manager: manager, phase: func(core.Stage, bool) {}}
	if err := l.Enter(ctx, core.Downloading); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- l.Enter(ctx, core.Processing) }()
	waitUntil(t, func() bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		return manager.active["downloading"] == 0 && len(manager.waiting) == 1
	})
	second, err := manager.acquire(ctx, "downloading")
	if err != nil {
		t.Fatal(err)
	}
	manager.release(second)
	manager.release(first)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	l.close()
}

// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"testing"
	"time"

	"github.com/bfcamel/CamelClip/internal/core"
)

type countedRepository struct {
	memoryRepository
	writes int
}

func (r *countedRepository) Save(ctx context.Context, snapshot Snapshot) error {
	r.writes++
	return r.memoryRepository.Save(ctx, snapshot)
}

func TestMetricsStayInMemoryAndSnapshotsOwnPointers(t *testing.T) {
	repo := &countedRepository{}
	s, err := New(context.Background(), &fakeDownloader{}, repo, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	j, err := s.Submit(req(), false)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.jobs[0].State = Downloading
	s.mu.Unlock()
	writes := repo.writes
	rate := 1024.0
	for i := 1; i <= 2000; i++ {
		s.observe(j.ID, core.Event{Kind: core.Progress, Stage: core.Downloading, Metrics: &core.Metrics{DownloadedBytes: int64(i), SpeedBPS: &rate, At: time.Now()}})
	}
	if repo.writes != writes {
		t.Fatal("progress wrote persistent storage")
	}
	stats := s.Statistics()
	if stats.SpeedBPS == nil || *stats.SpeedBPS != rate || stats.SessionBytes != 2000 {
		t.Fatal(stats)
	}
	if len(s.samples[j.ID]) > 64 {
		t.Fatal("unbounded samples")
	}
	copy := s.Jobs()
	*copy[0].Monitoring.Download.SpeedBPS = 999
	if *s.Jobs()[0].Monitoring.Download.SpeedBPS != rate {
		t.Fatal("snapshot aliases internal metrics")
	}
	s.mu.Lock()
	s.jobs[0].Monitoring.Download.At = time.Now().Add(-4 * time.Second)
	s.mu.Unlock()
	if s.Statistics().SpeedBPS != nil {
		t.Fatal("stale aggregate speed")
	}
}

func TestFixedRateSharesAndCancellationReleaseReservations(t *testing.T) {
	r := newResources()
	r.update(core.Limits{Downloads: 5, Processing: 2, GlobalBPS: 1000, JobBPS: 300})
	p, err := r.acquire(context.Background(), "downloading")
	if err != nil || p.rate != 200 {
		t.Fatalf("share: %+v %v", p, err)
	}
	r.update(core.Limits{Downloads: 5, Processing: 2, GlobalBPS: 1000, JobBPS: 100})
	q, err := r.acquire(context.Background(), "downloading")
	if err != nil || q.rate != 100 {
		t.Fatalf("per-job cap: %+v %v", q, err)
	}
	r.release(p)
	r.release(q)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.acquire(ctx, "downloading"); err == nil {
		t.Fatal("canceled acquire")
	}
	if r.reserved != 0 || r.active["downloading"] != 0 || len(r.waiting) != 0 {
		t.Fatal("reservation leaked")
	}
}

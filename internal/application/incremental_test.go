package application

import (
	"context"
	"testing"
)

type changesRepository struct {
	memoryRepository
	calls         []Changes
	snapshotSaves int
}

func (r *changesRepository) Save(context.Context, Snapshot) error { r.snapshotSaves++; return nil }
func (r *changesRepository) ApplyChanges(_ context.Context, c Changes) error {
	r.calls = append(r.calls, c)
	return nil
}
func TestQueuePersistsOnlyChangedJobsAndExplicitRemovals(t *testing.T) {
	r := &changesRepository{}
	for i := 1; i <= 100; i++ {
		r.snapshot.Jobs = append(r.snapshot.Jobs, Job{ID: i, Request: req(), State: Canceled})
	}
	r.snapshot.NextID = 100
	s, e := New(context.Background(), &fakeDownloader{}, r, 1)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.SetPriority(1, 1); e != nil {
		t.Fatal(e)
	} // Flush recovered state once.
	r.calls = nil
	if e = s.SetPriority(2, -1); e != nil {
		t.Fatal(e)
	}
	if len(r.calls) != 1 || len(r.calls[0].Upserts) != 1 || r.calls[0].Upserts[0].ID != 2 || len(r.calls[0].Removed) != 0 {
		t.Fatal(r.calls)
	}
	r.calls = nil
	if e = s.Remove(3); e != nil {
		t.Fatal(e)
	}
	if len(r.calls) != 1 || len(r.calls[0].Upserts) != 0 || len(r.calls[0].Removed) != 1 || r.calls[0].Removed[0] != 3 {
		t.Fatal(r.calls)
	}
	if r.snapshotSaves != 0 {
		t.Fatal("full snapshot path used")
	}
}

func TestQueuePageIsBoundedCopiedAndIndexSurvivesRemoval(t *testing.T) {
	r := &memoryRepository{}
	for i := 1; i <= 300; i++ {
		r.snapshot.Jobs = append(r.snapshot.Jobs, Job{ID: i, Request: req(), State: Canceled})
	}
	r.snapshot.NextID = 300
	s, e := New(context.Background(), &fakeDownloader{}, r, 1)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	page := s.JobsPage(0, 10000)
	if len(page.Items) != 200 || page.Total != 300 {
		t.Fatal(page.Total, len(page.Items))
	}
	page.Items[0].Title = "mutated"
	if s.JobsPage(0, 1).Items[0].Title == "mutated" {
		t.Fatal("page alias")
	}
	if e = s.Remove(150); e != nil {
		t.Fatal(e)
	}
	if e = s.SetPriority(151, 1); e != nil {
		t.Fatal(e)
	}
	if e = s.SetPriority(150, 1); e == nil {
		t.Fatal("removed index retained")
	}
	if got := s.JobsPage(10000, 50); len(got.Items) != 0 || got.Total != 299 {
		t.Fatal(got)
	}
}

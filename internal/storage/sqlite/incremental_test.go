package sqlite

import (
	"context"
	"github.com/bfcamel/CamelClip/internal/application"
	"testing"
)

func TestIncrementalChangesRetainUnmentionedJobsAndRollbackAtomically(t *testing.T) {
	ctx := context.Background()
	r, e := Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	if e = r.Save(ctx, application.Snapshot{NextID: 3, Jobs: []application.Job{job(1), job(2), job(3)}}); e != nil {
		t.Fatal(e)
	}
	changed := job(1)
	changed.State = application.Canceled
	if e = r.ApplyChanges(ctx, application.Changes{NextID: 3, Upserts: []application.Job{changed}, Removed: []int{3}}); e != nil {
		t.Fatal(e)
	}
	saved, e := r.Load(ctx)
	if e != nil || len(saved.Jobs) != 2 || saved.Jobs[0].State != application.Canceled || saved.Jobs[1].ID != 2 {
		t.Fatal(saved, e)
	}
	bad := job(4)
	bad.State = "invalid"
	changed.State = application.Done
	if e = r.ApplyChanges(ctx, application.Changes{NextID: 4, Upserts: []application.Job{changed, bad}, Removed: []int{2}}); e == nil {
		t.Fatal("invalid transaction accepted")
	}
	saved, _ = r.Load(ctx)
	if len(saved.Jobs) != 2 || saved.NextID != 3 || saved.Jobs[0].State != application.Canceled {
		t.Fatal("partial commit", saved)
	}
	if e = r.ApplyChanges(ctx, application.Changes{NextID: 3, Upserts: []application.Job{changed}, Removed: []int{1}}); e == nil {
		t.Fatal("conflicting transaction accepted")
	}
}

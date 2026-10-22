package sqlite

import (
	"context"
	"github.com/bfcamel/CamelClip/internal/application"
	"strconv"
	"testing"
)

func BenchmarkQueueSnapshot1000(b *testing.B) {
	ctx := context.Background()
	r, e := Open(ctx, b.TempDir())
	if e != nil {
		b.Fatal(e)
	}
	defer r.Close()
	snapshot := application.Snapshot{NextID: 1000}
	for i := 1; i <= 1000; i++ {
		snapshot.Jobs = append(snapshot.Jobs, job(i))
	}
	if e = r.Save(ctx, snapshot); e != nil {
		b.Fatal(e)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snapshot.Jobs[0].Title = strconv.Itoa(i)
		if e = r.Save(ctx, snapshot); e != nil {
			b.Fatal(e)
		}
	}
}

func BenchmarkQueueIncremental1000(b *testing.B) {
	ctx := context.Background()
	r, e := Open(ctx, b.TempDir())
	if e != nil {
		b.Fatal(e)
	}
	defer r.Close()
	snapshot := application.Snapshot{NextID: 1000}
	for i := 1; i <= 1000; i++ {
		snapshot.Jobs = append(snapshot.Jobs, job(i))
	}
	if e = r.Save(ctx, snapshot); e != nil {
		b.Fatal(e)
	}
	changes := application.Changes{NextID: 1000, Upserts: []application.Job{snapshot.Jobs[0]}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		changes.Upserts[0].Title = strconv.Itoa(i)
		if e = r.ApplyChanges(ctx, changes); e != nil {
			b.Fatal(e)
		}
	}
}

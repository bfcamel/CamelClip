package sqlite

import (
	"context"
	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
	"os"
	"path/filepath"
	"testing"
)

func TestDistinctPublicationsOfSameJobAreIndexed(t *testing.T) {
	ctx := context.Background()
	r, e := Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	dir := t.TempDir()
	first := filepath.Join(dir, "first.mp4")
	second := filepath.Join(dir, "second.mp4")
	os.WriteFile(first, []byte("first"), 0600)
	os.WriteFile(second, []byte("second"), 0600)
	req := core.Request{URL: "https://example.com/video", Mode: "video", Quality: "best", Audio: "mp3", Out: dir}
	j := application.Job{ID: 1, Request: req, State: application.Failed, OutputPath: first}
	if e = r.Save(ctx, application.Snapshot{NextID: 1, Jobs: []application.Job{j}}); e != nil {
		t.Fatal(e)
	}
	j.State = application.Done
	j.OutputPath = second
	if e = r.Save(ctx, application.Snapshot{NextID: 1, Jobs: []application.Job{j}}); e != nil {
		t.Fatal(e)
	}
	page, e := r.Assets(ctx, core.AssetQuery{Limit: 50})
	if e != nil {
		t.Fatal(e)
	}
	if len(page.Items) != 2 {
		t.Fatal("observation changed", page)
	}
	if e = r.Save(ctx, application.Snapshot{NextID: 1, Jobs: []application.Job{j}}); e != nil {
		t.Fatal(e)
	}
	page, _ = r.Assets(ctx, core.AssetQuery{Limit: 50})
	if len(page.Items) != 2 {
		t.Fatal("publication duplicated")
	}
}

func TestVersionThreePublicationMigrationPreservesHistory(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	r, e := Open(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{"DROP TABLE publication_outbox", "CREATE TABLE publication_outbox(job_id INTEGER PRIMARY KEY,asset_id TEXT NOT NULL,path TEXT NOT NULL)", "INSERT INTO publication_outbox VALUES(7,'asset','/kept.mp4')", "PRAGMA user_version=3"} {
		if _, e = r.db.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	r.Close()
	r, e = Open(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	var path string
	if e = r.db.QueryRow("SELECT path FROM publication_outbox WHERE job_id=7").Scan(&path); e != nil || path != "/kept.mp4" {
		t.Fatal(path, e)
	}
	if _, e = r.db.Exec("INSERT INTO publication_outbox VALUES(7,'second','/new.mp4')"); e != nil {
		t.Fatal(e)
	}
}

// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bfcamel/video-downloader/internal/application"
	"github.com/bfcamel/video-downloader/internal/core"
)

func job(id int) application.Job {
	return application.Job{ID: id, Request: core.Request{URL: "https://example.com/video", Mode: "video", Quality: "720", Audio: "mp3", Out: "/tmp/videos", Codec: "hevc", Container: "mkv"}, Title: "Test", State: application.Queued}
}
func TestSQLiteRoundTripPermissionsLockAndRollback(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	original := application.Snapshot{NextID: 1, Jobs: []application.Job{job(1)}}
	if err = r.Save(ctx, original); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(ctx, dir); err == nil {
		t.Fatal("concurrent owner accepted")
	}
	bad := original
	bad.Jobs = append(bad.Jobs, job(1))
	if err = r.Save(ctx, bad); err == nil {
		t.Fatal("duplicate ID accepted")
	}
	loaded, err := r.Load(ctx)
	if err != nil || len(loaded.Jobs) != 1 || loaded.Jobs[0].Request.Codec != "hevc" {
		t.Fatal(loaded, err)
	}
	r.Close()
	data, err := os.ReadFile(filepath.Join(dir, "queue.sqlite3"))
	if err != nil || string(data[:16]) != "SQLite format 3\x00" {
		t.Fatal("not a SQLite database", err)
	}
	stat, _ := os.Stat(filepath.Join(dir, "queue.sqlite3"))
	if stat.Mode().Perm() != 0600 {
		t.Fatal(stat.Mode())
	}
	reopened, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loaded, err = reopened.Load(ctx)
	if err != nil || loaded.NextID != 1 {
		t.Fatal(loaded, err)
	}
}

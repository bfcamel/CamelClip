// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package sqlite

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bfcamel/CamelClip/internal/application"
)

func TestDatabaseFullRollsBackAndRemainsRecoverable(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	baseline := application.Snapshot{NextID: 1, Jobs: []application.Job{job(1)}}
	if err = r.Save(ctx, baseline); err != nil {
		t.Fatal(err)
	}
	var pages int
	if err = r.db.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
		t.Fatal(err)
	}
	// An actual SQLite storage limit exercises SQLITE_FULL without filling the host disk.
	if _, err = r.db.Exec("PRAGMA max_page_count=" + fmt.Sprint(pages)); err != nil {
		t.Fatal(err)
	}
	oversized := job(2)
	oversized.Title = strings.Repeat("x", 2*1024*1024)
	err = r.Save(ctx, application.Snapshot{NextID: 2, Jobs: []application.Job{job(1), oversized}})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "full") {
		t.Fatalf("expected SQLITE_FULL: %v", err)
	}
	got, err := r.Load(ctx)
	if err != nil || len(got.Jobs) != 1 || got.NextID != 1 {
		t.Fatalf("transaction was not rolled back: %+v %v", got, err)
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	r, err = Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got, err = r.Load(ctx)
	if err != nil || len(got.Jobs) != 1 || got.Jobs[0].Title != "Test" {
		t.Fatal(got, err)
	}
}
func TestCorruptDatabaseIsNeverReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.sqlite3")
	original := []byte("not a SQLite database\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if r, err := Open(context.Background(), dir); err == nil {
		r.Close()
		t.Fatal("corrupt database accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, original) {
		t.Fatal("corrupt database was replaced")
	}
}

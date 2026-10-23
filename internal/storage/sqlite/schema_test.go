// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/bfcamel/CamelClip/internal/application"
	"path/filepath"
	"testing"
)

func TestVersionOneQueueMigratesWithoutLosingHistory(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "queue.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{`CREATE TABLE metadata(key TEXT PRIMARY KEY,value INTEGER NOT NULL)`, `INSERT INTO metadata VALUES('next_id',7)`, `CREATE TABLE jobs(id INTEGER PRIMARY KEY,url TEXT,title TEXT,state TEXT,detail TEXT,output_path TEXT,needs_quality INTEGER,request_json TEXT,media_json TEXT)`, `PRAGMA user_version=1`}
	for _, statement := range statements {
		if _, err = db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	j := job(7)
	j.State = application.Done
	j.OutputPath = "/tmp/kept.mp4"
	r, _ := json.Marshal(j.Request)
	info, _ := json.Marshal(j.Info)
	_, err = db.Exec(`INSERT INTO jobs VALUES(?,?,?,?,?,?,?,?,?)`, j.ID, j.Request.URL, j.Title, j.State, "", j.OutputPath, 0, string(r), string(info))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	repo, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	saved, err := repo.Load(context.Background())
	if err != nil || len(saved.Jobs) != 1 || saved.NextID != 7 || saved.Jobs[0].OutputPath != "/tmp/kept.mp4" {
		t.Fatal(saved, err)
	}
	saved.Jobs[0].State = application.WaitingProcessing
	if err = repo.Save(context.Background(), saved); err != nil {
		t.Fatal("new waiting state not persisted", err)
	}
	var version int
	repo.db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 7 {
		t.Fatal(version)
	}
}

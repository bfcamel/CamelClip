// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

// Package sqlite implements the application.Repository port using SQLite transactions.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/bfcamel/video-downloader/internal/application"
	"github.com/bfcamel/video-downloader/internal/core"
	_ "modernc.org/sqlite"
)

type Repository struct {
	db   *sql.DB
	lock *os.File
}

var _ application.Repository = (*Repository)(nil)

func Open(ctx context.Context, dir string) (*Repository, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "queue.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("queue is already open in another Clipnaro session")
	}
	path := filepath.Join(dir, "queue.sqlite3")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		lock.Close()
		return nil, err
	}
	file.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		lock.Close()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	r := &Repository{db: db, lock: lock}
	fail := func(e error) (*Repository, error) { r.Close(); return nil, e }
	var version int
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	if version > 3 {
		return fail(fmt.Errorf("unsupported database schema version %d", version))
	}
	for _, statement := range []string{"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "PRAGMA foreign_keys=ON"} {
		if _, err = db.ExecContext(ctx, statement); err != nil {
			return fail(err)
		}
	}
	if err = r.schema(ctx, version); err != nil {
		return fail(err)
	}
	return r, nil
}
func (r *Repository) Load(ctx context.Context) (application.Snapshot, error) {
	snapshot := application.Snapshot{Jobs: []application.Job{}}
	err := r.db.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key='next_id'").Scan(&snapshot.NextID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return snapshot, err
	}
	rows, err := r.db.QueryContext(ctx, "SELECT id,url,title,state,detail,output_path,needs_quality,request_json,media_json,monitor_json,control_json,history_json FROM jobs ORDER BY id")
	if err != nil {
		return snapshot, err
	}
	defer rows.Close()
	for rows.Next() {
		var j application.Job
		var request, info, monitor, control, history string
		var url string
		if err = rows.Scan(&j.ID, &url, &j.Title, &j.State, &j.Detail, &j.OutputPath, &j.NeedsQuality, &request, &info, &monitor, &control, &history); err != nil {
			return snapshot, err
		}
		if err = json.Unmarshal([]byte(request), &j.Request); err != nil {
			return snapshot, err
		}
		if err = json.Unmarshal([]byte(info), &j.Info); err != nil {
			return snapshot, err
		}
		if err = json.Unmarshal([]byte(monitor), &j.Monitoring); err != nil {
			return snapshot, err
		}
		if err = json.Unmarshal([]byte(control), &j.Control); err != nil {
			return snapshot, err
		}
		if err = json.Unmarshal([]byte(history), &j.History); err != nil {
			return snapshot, err
		}
		if j.Request.URL != url {
			return snapshot, errors.New("inconsistent stored request URL")
		}
		snapshot.Jobs = append(snapshot.Jobs, j)
	}
	return snapshot, rows.Err()
}
func saveTx(ctx context.Context, tx *sql.Tx, snapshot application.Snapshot) error {
	if snapshot.NextID < 0 {
		return errors.New("invalid next job ID")
	}
	if _, err := tx.ExecContext(ctx, "CREATE TEMP TABLE IF NOT EXISTS retained_jobs(id INTEGER PRIMARY KEY)"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM retained_jobs"); err != nil {
		return err
	}

	seen := map[int]bool{}
	for _, j := range snapshot.Jobs {
		if seen[j.ID] {
			return errors.New("duplicate job ID")
		}
		seen[j.ID] = true
		if _, err := tx.ExecContext(ctx, "INSERT INTO retained_jobs(id) VALUES(?)", j.ID); err != nil {
			return err
		}
		if j.ID <= 0 || !j.State.Valid() {
			return errors.New("invalid job record")
		}
		if err := core.Validate(j.Request); err != nil {
			return err
		}
		request, err := json.Marshal(j.Request)
		if err != nil {
			return err
		}
		info, err := json.Marshal(j.Info)
		if err != nil {
			return err
		}
		monitor, err := json.Marshal(j.Monitoring)
		if err != nil {
			return err
		}
		control, err := json.Marshal(j.Control)
		if err != nil {
			return err
		}
		history, err := json.Marshal(j.History)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO jobs(id,url,title,state,detail,output_path,needs_quality,request_json,media_json,monitor_json,control_json,history_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET url=excluded.url,title=excluded.title,state=excluded.state,detail=excluded.detail,output_path=excluded.output_path,needs_quality=excluded.needs_quality,request_json=excluded.request_json,media_json=excluded.media_json,monitor_json=excluded.monitor_json,control_json=excluded.control_json,history_json=excluded.history_json WHERE jobs.url!=excluded.url OR jobs.title!=excluded.title OR jobs.state!=excluded.state OR jobs.detail!=excluded.detail OR jobs.output_path!=excluded.output_path OR jobs.needs_quality!=excluded.needs_quality OR jobs.request_json!=excluded.request_json OR jobs.media_json!=excluded.media_json OR jobs.monitor_json!=excluded.monitor_json OR jobs.control_json!=excluded.control_json OR jobs.history_json!=excluded.history_json`, j.ID, j.Request.URL, j.Title, string(j.State), j.Detail, j.OutputPath, j.NeedsQuality, string(request), string(info), string(monitor), string(control), string(history)); err != nil {
			return err
		}
		if j.OutputPath != "" {
			if err = reconcilePublishedTx(ctx, tx, j.OutputPath, j.Title, j.Request.URL, j.ID, j.Request.Mode, j.Info.Duration); err != nil {
				return err
			}
			var assetID string
			if e := tx.QueryRowContext(ctx, "SELECT id FROM assets WHERE path=?", j.OutputPath).Scan(&assetID); e == nil {
				if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO asset_sources(asset_id,job_id,request,media) VALUES(?,?,?,?)", assetID, j.ID, string(request), string(info)); err != nil {
					return err
				}
			}
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM jobs WHERE NOT EXISTS (SELECT 1 FROM retained_jobs WHERE retained_jobs.id=jobs.id)"); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO metadata(key,value) VALUES('next_id',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, snapshot.NextID)
	return err
}
func (r *Repository) Save(ctx context.Context, snapshot application.Snapshot) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = saveTx(ctx, tx, snapshot); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) Close() error {
	err := r.db.Close()
	syscall.Flock(int(r.lock.Fd()), syscall.LOCK_UN)
	if e := r.lock.Close(); err == nil {
		err = e
	}
	return err
}

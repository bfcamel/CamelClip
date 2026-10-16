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
	lock, err := os.OpenFile(filepath.Join(dir, "queue.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("queue is already open in another MacDown session")
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
	if version > 1 {
		return fail(fmt.Errorf("unsupported database schema version %d", version))
	}
	for _, statement := range []string{
		"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "PRAGMA foreign_keys=ON",
		`CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY,value INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS jobs (
    id INTEGER PRIMARY KEY CHECK(id>0),url TEXT NOT NULL,title TEXT NOT NULL,
    state TEXT NOT NULL CHECK(state IN ('queued','inspecting','choose','ready','downloading','processing','verifying','done','failed','canceling','canceled')),
    detail TEXT NOT NULL,output_path TEXT NOT NULL,needs_quality INTEGER NOT NULL CHECK(needs_quality IN (0,1)),
    request_json TEXT NOT NULL,media_json TEXT NOT NULL)`,
		"PRAGMA user_version=1",
	} {
		if _, err = db.ExecContext(ctx, statement); err != nil {
			return fail(err)
		}
	}
	if err = r.migrateLegacy(ctx, filepath.Join(dir, "queue.json")); err != nil {
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
	rows, err := r.db.QueryContext(ctx, "SELECT id,url,title,state,detail,output_path,needs_quality,request_json,media_json FROM jobs ORDER BY id")
	if err != nil {
		return snapshot, err
	}
	defer rows.Close()
	for rows.Next() {
		var j application.Job
		var request, info string
		var url string
		if err = rows.Scan(&j.ID, &url, &j.Title, &j.State, &j.Detail, &j.OutputPath, &j.NeedsQuality, &request, &info); err != nil {
			return snapshot, err
		}
		if err = json.Unmarshal([]byte(request), &j.Request); err != nil {
			return snapshot, err
		}
		if err = json.Unmarshal([]byte(info), &j.Info); err != nil {
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
		if _, err = tx.ExecContext(ctx, `INSERT INTO jobs(id,url,title,state,detail,output_path,needs_quality,request_json,media_json) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET url=excluded.url,title=excluded.title,state=excluded.state,detail=excluded.detail,output_path=excluded.output_path,needs_quality=excluded.needs_quality,request_json=excluded.request_json,media_json=excluded.media_json WHERE jobs.url!=excluded.url OR jobs.title!=excluded.title OR jobs.state!=excluded.state OR jobs.detail!=excluded.detail OR jobs.output_path!=excluded.output_path OR jobs.needs_quality!=excluded.needs_quality OR jobs.request_json!=excluded.request_json OR jobs.media_json!=excluded.media_json`, j.ID, j.Request.URL, j.Title, string(j.State), j.Detail, j.OutputPath, j.NeedsQuality, string(request), string(info)); err != nil {
			return err
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

// Import legacy JSON exactly once. Its original bytes remain as a backup; the
// migration marker and every imported job are committed in the same transaction.
func (r *Repository) migrateLegacy(ctx context.Context, path string) error {
	var imported int
	err := r.db.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key='legacy_imported'").Scan(&imported)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var count int
	if err = r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs").Scan(&count); err != nil {
		return err
	}
	snapshot := application.Snapshot{}
	if count == 0 {
		data, e := os.ReadFile(path)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e == nil {
			var legacy struct {
				Version int `json:"version"`
				NextID  int `json:"next_id"`
				Jobs    []struct {
					ID       int               `json:"id"`
					URL      string            `json:"url"`
					Title    string            `json:"title"`
					State    application.State `json:"state"`
					Detail   string            `json:"detail"`
					Info     core.MediaInfo    `json:"media"`
					Settings struct {
						Language  string `json:"language"`
						Mode      string `json:"mode"`
						Quality   string `json:"quality"`
						Audio     string `json:"audio_format"`
						Out       string `json:"output_directory"`
						Browser   string `json:"cookies_browser"`
						Codec     string `json:"video_codec"`
						Container string `json:"video_container"`
					} `json:"settings"`
				} `json:"jobs"`
			}
			if e = json.Unmarshal(data, &legacy); e != nil {
				return fmt.Errorf("legacy queue retained unchanged: %w", e)
			}
			if legacy.Version != 1 || legacy.NextID < 0 {
				return errors.New("unsupported legacy queue version")
			}
			snapshot.NextID = legacy.NextID
			seen := map[int]bool{}
			for _, j := range legacy.Jobs {
				if j.ID <= 0 || seen[j.ID] || !j.State.Valid() {
					return errors.New("invalid legacy queue job")
				}
				seen[j.ID] = true
				s := j.Settings
				request := core.Request{URL: j.URL, Mode: s.Mode, Quality: s.Quality, Audio: s.Audio, Out: s.Out, Cookies: s.Browser, Codec: s.Codec, Container: s.Container, Duration: j.Info.Duration}
				if e = core.Validate(request); e != nil {
					return fmt.Errorf("invalid legacy job: %w", e)
				}
				snapshot.Jobs = append(snapshot.Jobs, application.Job{ID: j.ID, Request: request, Title: j.Title, State: j.State, Detail: j.Detail, Info: j.Info, NeedsQuality: true})
				if j.ID > snapshot.NextID {
					snapshot.NextID = j.ID
				}
			}
		}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if count == 0 {
		if err = saveTx(ctx, tx, snapshot); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO metadata(key,value) VALUES('legacy_imported',1)"); err != nil {
		return err
	}
	return tx.Commit()
}

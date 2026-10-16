// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"encoding/json"
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
func legacyBytes() []byte {
	data, _ := json.Marshal(map[string]any{"version": 1, "next_id": 7, "jobs": []any{map[string]any{"id": 7, "url": "https://example.com/video", "title": "Legacy", "state": "processing", "settings": map[string]any{"language": "ru", "mode": "video", "quality": "720", "audio_format": "mp3", "output_directory": "/tmp/videos", "cookies_browser": "firefox:work", "video_codec": "av1", "video_container": "webm"}, "media": map[string]any{"id": "video", "duration": 12}}}})
	return data
}
func TestLegacyMigrationIsAtomicIdempotentAndKeepsBackup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.json")
	original := legacyBytes()
	os.WriteFile(path, original, 0600)
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := r.Load(ctx)
	if err != nil || len(s.Jobs) != 1 || s.Jobs[0].Request.Codec != "av1" || s.Jobs[0].State != application.Processing || s.NextID != 7 {
		t.Fatal(s, err)
	}
	if err = r.Save(ctx, application.Snapshot{NextID: 7}); err != nil {
		t.Fatal(err)
	}
	r.Close()
	r, err = Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	s, err = r.Load(ctx)
	if err != nil || len(s.Jobs) != 0 {
		t.Fatal("legacy queue reimported", s, err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("legacy backup changed")
	}
}
func TestInvalidLegacyDataNeverBecomesEmptyQueue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.json")
	os.WriteFile(path, []byte("{broken"), 0600)
	if _, err := Open(context.Background(), dir); err == nil {
		t.Fatal("corrupt JSON ignored")
	}
	after, _ := os.ReadFile(path)
	if string(after) != "{broken" {
		t.Fatal("backup overwritten")
	}
	os.WriteFile(path, legacyBytes(), 0600)
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	s, _ := r.Load(context.Background())
	if len(s.Jobs) != 1 {
		t.Fatal("failed import marked as successful")
	}
}

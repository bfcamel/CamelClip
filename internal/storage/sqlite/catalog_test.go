// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
	"github.com/bfcamel/CamelClip/internal/security"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPublishedAssetsSurviveHistoryRemovalAndAutoUploadIsIdempotent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	r, e := Open(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	v := security.New()
	state, e := v.Initialize([]byte("some master password"))
	if e != nil {
		t.Fatal(e)
	}
	if e = r.SetVault(ctx, state, nil); e != nil {
		t.Fatal(e)
	}
	p := core.S3Profile{ID: core.NewID(), Name: "Archive", Region: "auto", Bucket: "bucket", Style: "path_style", Enabled: true, AutoUpload: true}
	secret, e := v.Seal(p.ID, core.Credentials{AccessKey: "plain-access-secret", SecretKey: "plain-private-secret"})
	if e != nil {
		t.Fatal(e)
	}
	if e = r.SaveProfile(ctx, p, &secret); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "movie.mp4")
	os.WriteFile(path, []byte("fixture"), 0600)
	request := core.Request{URL: "https://example.test/a", Mode: "video", Quality: "best", Audio: "mp3", Out: dir}
	snapshot := application.Snapshot{NextID: 1, Jobs: []application.Job{{ID: 1, Request: request, Title: "Русское видео", State: application.Done, OutputPath: path}}}
	if e = r.Save(ctx, snapshot); e != nil {
		t.Fatal(e)
	}
	if e = r.Save(ctx, snapshot); e != nil {
		t.Fatal(e)
	}
	assets, e := r.Assets(ctx, core.AssetQuery{Search: "РУССКОЕ", Limit: 50})
	if e != nil || len(assets.Items) != 1 {
		t.Fatal(assets, e)
	}
	jobs, e := r.Uploads(ctx, 50, 0)
	if e != nil || len(jobs) != 1 {
		t.Fatal(jobs, e)
	}
	if e = r.Save(ctx, application.Snapshot{NextID: 1}); e != nil {
		t.Fatal(e)
	}
	assets, e = r.Assets(ctx, core.AssetQuery{Limit: 50})
	if e != nil || assets.Total != 1 {
		t.Fatal("asset lost with history", e)
	}
	var sources int
	r.db.QueryRow("SELECT count(*) FROM asset_sources").Scan(&sources)
	if sources != 1 {
		t.Fatal("source metadata lost")
	}
	for _, name := range []string{"queue.sqlite3", "queue.sqlite3-wal", "queue.sqlite3-shm"} {
		data, _ := os.ReadFile(filepath.Join(dir, name))
		if bytes.Contains(data, []byte("plain-private-secret")) || bytes.Contains(data, []byte("plain-access-secret")) {
			t.Fatal("unencrypted credentials", name)
		}
	}
	raw, _ := json.Marshal(p)
	if bytes.Contains(raw, []byte("plain-private-secret")) {
		t.Fatal("secret in public profile")
	}
}
func TestDeleteMissingAssetAllowsRediscovery(t *testing.T) {
	ctx := context.Background()
	r, e := Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	a, e := r.PutAsset(ctx, core.Asset{Path: filepath.Join(t.TempDir(), "absent.mp4"), Title: "Test", Status: "available", Added: time.Now()})
	if e != nil {
		t.Fatal(e)
	}
	if e = r.DeleteAssetFile(ctx, a.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Asset(ctx, a.ID); e == nil {
		t.Fatal("missing entry retained")
	}
	if _, e = r.PutAsset(ctx, a); e != nil {
		t.Fatal(e)
	}
}

func TestTenThousandAssetsUsePagedQueriesAndUnicodeSearch(t *testing.T) {
	ctx := context.Background()
	r, e := Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 10000; i++ {
		a := core.Asset{Path: fmt.Sprintf("/catalog/%d.mp4", i), Title: fmt.Sprintf("Клип %d", i), Status: "available", Kind: "video", Container: "mp4", Height: 1080}
		if _, e = putAssetTx(ctx, tx, a); e != nil {
			tx.Rollback()
			t.Fatal(e)
		}
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	page, e := r.Assets(ctx, core.AssetQuery{Limit: 50, Offset: 9950, MinHeight: 1080})
	if e != nil || page.Total != 10000 || len(page.Items) != 50 {
		t.Fatal(page.Total, len(page.Items), e)
	}
	page, e = r.Assets(ctx, core.AssetQuery{Search: "КЛИП 9999", Limit: 50})
	if e != nil || page.Total != 1 {
		t.Fatal(page, e)
	}
}

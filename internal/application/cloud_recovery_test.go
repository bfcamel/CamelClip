// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
	"github.com/bfcamel/CamelClip/internal/security"
	"github.com/bfcamel/CamelClip/internal/storage/sqlite"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type verifiedProfileBackend struct{ fakeCloud }

func (f *verifiedProfileBackend) Upload(ctx context.Context, p core.S3Profile, c core.Credentials, key string, src core.UploadSource, size int64, sha string, parts int, rate int64, emit func(core.UploadEvent)) (core.RemoteObject, error) {
	if p.Endpoint != "" && !p.ConditionalVerified {
		return core.RemoteObject{}, errors.New("ConditionalWriteTestRequired")
	}
	return f.fakeCloud.Upload(ctx, p, c, key, src, size, sha, parts, rate, emit)
}
func newRecoveryCloud(t *testing.T) (context.Context, *sqlite.Repository, *application.CloudService, core.S3Profile, core.Asset) {
	t.Helper()
	ctx := context.Background()
	r, e := sqlite.Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	factory := func() core.CredentialVault { return security.New() }
	b := &verifiedProfileBackend{fakeCloud: fakeCloud{remote: map[string]core.RemoteObject{}}}
	c := application.NewCloud(ctx, r, r, r, factory(), factory, b)
	c.Restrict([]string{})
	t.Cleanup(func() { c.Close(); r.Close() })
	if e = c.Initialize(ctx, []byte("audit master password")); e != nil {
		t.Fatal(e)
	}
	p := core.S3Profile{ID: core.NewID(), Name: "audit", Region: "auto", Bucket: "bucket", Style: "path_style", Enabled: true, Endpoint: "https://example.com"}
	if e = c.SaveProfile(ctx, p, &core.Credentials{AccessKey: "access", SecretKey: "private-key"}); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "source.mp4")
	if e = os.WriteFile(path, []byte("bytes"), 0600); e != nil {
		t.Fatal(e)
	}
	st, e := os.Stat(path)
	if e != nil {
		t.Fatal(e)
	}
	a, e := r.PutAsset(ctx, core.Asset{Path: path, Size: st.Size(), Modified: st.ModTime(), Status: "available", Kind: "video"})
	if e != nil {
		t.Fatal(e)
	}
	return ctx, r, c, p, a
}
func TestRestrictedUploadBeyondFirstPage(t *testing.T) {
	ctx, r, c, p, a := newRecoveryCloud(t)
	p.ConditionalVerified = true
	if e := c.SaveProfile(ctx, p, nil); e != nil {
		t.Fatal(e)
	}
	var last core.UploadJob
	for i := 0; i < 5; i++ {
		j := core.UploadJob{ID: core.NewID(), AssetID: a.ID, ProfileID: p.ID, Profile: p, Key: fmt.Sprint(i), State: "queued", Size: a.Size, Modified: a.Modified}
		var e error
		last, e = r.EnqueueUpload(ctx, j)
		if e != nil {
			t.Fatal(e)
		}
	}
	c.Restrict([]string{last.ID})
	waitUploadDone(t, ctx, c, last.ID)
	j, e := c.Get(ctx, last.ID)
	if e != nil {
		t.Fatal(e)
	}
	if j.State != "done" {
		t.Fatalf("selected fifth upload starved: state=%s", j.State)
	}
}
func TestWriteTestRefreshesQueuedProfile(t *testing.T) {
	ctx, _, c, p, a := newRecoveryCloud(t)
	jobs, e := c.Enqueue(ctx, a.ID, []string{p.ID})
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Test(ctx, p.ID, true); e != nil {
		t.Fatal(e)
	}
	if e = c.Retry(ctx, jobs[0].ID, false); e != nil {
		t.Fatal(e)
	}
	c.Restrict([]string{jobs[0].ID})
	waitUploadDone(t, ctx, c, jobs[0].ID)
	j, e := c.Get(ctx, jobs[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	if j.State != "done" {
		t.Fatalf("write test succeeded but retry state=%s error=%s snapshot.verified=%v", j.State, j.Error, j.Profile.ConditionalVerified)
	}
}

func waitUploadDone(t *testing.T, ctx context.Context, c *application.CloudService, id string) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for {
		j, err := c.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if j.State == "done" {
			return
		}
		if j.Terminal() || time.Now().After(until) {
			t.Fatalf("upload did not finish: state=%s error=%s", j.State, j.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

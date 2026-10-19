// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application_test

import (
	"context"
	"errors"
	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
	"github.com/bfcamel/CamelClip/internal/security"
	"github.com/bfcamel/CamelClip/internal/storage/sqlite"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeCloud struct {
	mu          sync.Mutex
	active, max int
	uploads     int
	remote      map[string]core.RemoteObject
	gate        chan struct{}
}

func (f *fakeCloud) Test(context.Context, core.S3Profile, core.Credentials, bool) error { return nil }
func (f *fakeCloud) Check(_ context.Context, p core.S3Profile, _ core.Credentials, key string) (core.RemoteObject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.remote[p.ID+key], nil
}
func (f *fakeCloud) Cleanup(context.Context, core.S3Profile, core.Credentials, string, string) error {
	return nil
}
func (f *fakeCloud) Upload(ctx context.Context, p core.S3Profile, c core.Credentials, key string, _ core.UploadSource, size int64, _ string, _ int, _ int64, emit func(core.UploadEvent)) (core.RemoteObject, error) {
	if c.SecretKey != "private-key" {
		return core.RemoteObject{}, errors.New("wrong credentials")
	}
	f.mu.Lock()
	f.active++
	f.uploads++
	f.max = max(f.max, f.active)
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.active--; f.mu.Unlock() }()
	if p.Name == "bad" {
		return core.RemoteObject{}, errors.New("AccessDenied")
	}
	if f.gate != nil {
		select {
		case <-ctx.Done():
			return core.RemoteObject{}, ctx.Err()
		case <-f.gate:
		}
	}
	out := core.RemoteObject{Size: size, ETag: "opaque-etag", Verification: "size_verified"}
	f.mu.Lock()
	f.remote[p.ID+key] = out
	f.mu.Unlock()
	emit(core.UploadEvent{Stage: "uploaded", Bytes: size, Total: size, ETag: out.ETag})
	emit(core.UploadEvent{Stage: "remote_check", Bytes: size, Total: size})
	return out, nil
}
func TestCloudQueuesIsolationLockedRestartAndPasswordRotation(t *testing.T) {
	ctx := context.Background()
	r, e := sqlite.Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	backend := &fakeCloud{remote: map[string]core.RemoteObject{}, gate: make(chan struct{})}
	factory := func() core.CredentialVault { return security.New() }
	cloud := application.NewCloud(ctx, r, r, r, factory(), factory, backend)
	defer cloud.Close()
	if e = cloud.Initialize(ctx, []byte("first master password")); e != nil {
		t.Fatal(e)
	}
	profiles := []string{}
	for _, name := range []string{"good-a", "good-b", "bad"} {
		p := core.S3Profile{ID: core.NewID(), Name: name, Region: "auto", Bucket: "bucket", Style: "path_style", Enabled: true}
		if e = cloud.SaveProfile(ctx, p, &core.Credentials{AccessKey: "access", SecretKey: "private-key"}); e != nil {
			t.Fatal(e)
		}
		profiles = append(profiles, p.ID)
	}
	path := filepath.Join(t.TempDir(), "source.mp4")
	os.WriteFile(path, []byte("test media bytes"), 0600)
	st, _ := os.Stat(path)
	asset, e := r.PutAsset(ctx, core.Asset{Path: path, Title: "Fixture", Size: st.Size(), Modified: st.ModTime(), Status: "available", Kind: "video"})
	if e != nil {
		t.Fatal(e)
	}
	cloud.Lock()
	jobs, e := cloud.Enqueue(ctx, asset.ID, profiles)
	if e != nil || len(jobs) != 3 {
		t.Fatal(e)
	}
	for _, j := range jobs {
		if j.State != "waiting_unlock" {
			t.Fatal(j.State)
		}
	}
	if e = cloud.Unlock(ctx, []byte("first master password")); e != nil {
		t.Fatal(e)
	}
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		active, _, _ := cloud.Stats()
		if active == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	active, _, _ := cloud.Stats()
	if active != 2 {
		t.Fatal("independent S3 limit not reached", active)
	}
	close(backend.gate)
	until = time.Now().Add(6 * time.Second)
	for time.Now().Before(until) {
		rows, _ := cloud.List(ctx, 10, 0)
		done := 0
		for _, j := range rows {
			if j.Terminal() {
				done++
			}
		}
		if done == 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	rows, _ := cloud.List(ctx, 10, 0)
	success, failed := 0, 0
	for _, j := range rows {
		if j.State == "done" {
			success++
		}
		if j.State == "failed" {
			failed++
		}
	}
	if success != 2 || failed != 1 || backend.max > 2 {
		t.Fatal(rows, backend.max)
	}
	if _, e = os.Stat(path); e != nil {
		t.Fatal("source removed")
	}
	if e = cloud.Rotate(ctx, []byte("first master password"), []byte("second master password")); e != nil {
		t.Fatal(e)
	}
	cloud.Close()
	backend.mu.Lock()
	before := backend.uploads
	backend.mu.Unlock()
	replay := rows[0]
	if replay.State != "done" {
		replay = rows[1]
	}
	replay.State = "uploading"
	if e = r.SaveUpload(ctx, replay); e != nil {
		t.Fatal(e)
	}
	cloud = application.NewCloud(ctx, r, r, r, factory(), factory, backend)
	defer cloud.Close()
	if !cloud.Locked() {
		t.Fatal("vault unlocked after restart")
	}
	if e = cloud.Unlock(ctx, []byte("first master password")); e == nil {
		t.Fatal("old password accepted")
	}
	if e = cloud.Unlock(ctx, []byte("second master password")); e != nil {
		t.Fatal(e)
	}
	until = time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		j, e := cloud.Get(ctx, replay.ID)
		if e == nil && j.State == "done" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	j, e := cloud.Get(ctx, replay.ID)
	if e != nil || j.State != "done" {
		t.Fatal("checkpoint not recovered", j, e)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.uploads != before {
		t.Fatal("acknowledged object was uploaded again")
	}

}

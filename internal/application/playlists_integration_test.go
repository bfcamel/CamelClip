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
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type playlistFixture struct {
	mu      sync.Mutex
	entries []core.PlaylistEntry
	err     error
	calls   int
}

func (f *playlistFixture) Enumerate(ctx context.Context, _ core.PlaylistScan, emit func([]core.PlaylistEntry) error) error {
	f.mu.Lock()
	entries := append([]core.PlaylistEntry(nil), f.entries...)
	err := f.err
	f.calls++
	f.mu.Unlock()
	for len(entries) > 0 {
		n := min(len(entries), 250)
		if e := emit(entries[:n]); e != nil {
			return e
		}
		entries = entries[n:]
	}
	return err
}
func (f *playlistFixture) set(ids ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = nil
	for _, id := range ids {
		f.entries = append(f.entries, core.PlaylistEntry{ID: id, Title: id})
	}
	f.err = nil
}

type playlistDownloader struct {
	mu    sync.Mutex
	calls int
}

func (d *playlistDownloader) Inspect(context.Context, core.Request) (core.MediaInfo, error) {
	return core.MediaInfo{Title: "Playlist fixture", Duration: 1}, nil
}
func (d *playlistDownloader) Formats(context.Context, core.Request, core.Observer) error { return nil }
func (d *playlistDownloader) Download(ctx context.Context, r core.Request, emit core.Observer) (core.Result, error) {
	d.mu.Lock()
	d.calls++
	d.mu.Unlock()
	u, _ := url.Parse(r.URL)
	os.MkdirAll(r.Out, 0700)
	path := filepath.Join(r.Out, u.Query().Get("v")+".mp4")
	if e := os.WriteFile(path, []byte("verified fixture"), 0600); e != nil {
		return core.Result{}, e
	}
	emit(core.Event{Kind: core.Published, Path: path})
	return core.Result{Path: path}, nil
}

type playlistEnv struct {
	r      *sqlite.Repository
	s      *application.PlaylistService
	q      *application.Service
	source *playlistFixture
	engine *playlistDownloader
	out    string
}

func newPlaylistEnv(t *testing.T) *playlistEnv {
	t.Helper()
	ctx := context.Background()
	r, e := sqlite.Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	d := &playlistDownloader{}
	q, e := application.New(ctx, d, r, 2)
	if e != nil {
		t.Fatal(e)
	}
	src := &playlistFixture{}
	s, e := application.NewPlaylists(ctx, r, src, q)
	if e != nil {
		t.Fatal(e)
	}
	env := &playlistEnv{r, s, q, src, d, t.TempDir()}
	t.Cleanup(func() { s.Close(); q.Close() })
	return env
}
func (e *playlistEnv) add(t *testing.T, playlist, initial string, profiles ...string) core.PlaylistSubscription {
	t.Helper()
	p, err := e.s.Save(context.Background(), core.PlaylistSubscription{PlaylistID: playlist, Enabled: true, InitialMode: initial, IntervalMinutes: 15, Request: core.Request{Mode: "video", Quality: "720", Audio: "mp3", Codec: "h264", Container: "mp4", Out: e.out}, S3Profiles: profiles})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func waitPlaylistJob(t *testing.T, q application.Queue, id int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		for _, j := range q.Jobs() {
			if j.ID == id {
				if j.State == application.Done {
					return
				}
				if j.State.Terminal() {
					t.Fatal(j)
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("playlist job did not complete", q.Jobs(), q.Err())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func TestPlaylistBaselinePartialScanAndMembershipIdentity(t *testing.T) {
	e := newPlaylistEnv(t)
	ctx := context.Background()
	p := e.add(t, "PLfixture00001", "new_only")
	e.source.set("AAAAAAAAAAA", "BBBBBBBBBBB")
	e.source.err = errors.New("partial enumeration")
	if err := e.s.Check(ctx, p.ID); err == nil {
		t.Fatal("partial scan accepted")
	}
	page, _ := e.s.Items(ctx, p.ID, 50, 0)
	if page.Total != 0 {
		t.Fatal("partial baseline leaked", page)
	}
	e.source.err = nil
	if err := e.s.Check(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	page, _ = e.s.Items(ctx, p.ID, 50, 0)
	if page.Total != 2 || page.Items[0].Eligible {
		t.Fatal(page)
	}
	e.source.set("BBBBBBBBBBB", "CCCCCCCCCCC", "AAAAAAAAAAA", "CCCCCCCCCCC")
	if err := e.s.Check(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	candidates, err := e.r.PlaylistCandidates(ctx, 10)
	if err != nil || len(candidates) != 1 || candidates[0].VideoID != "CCCCCCCCCCC" {
		t.Fatal(candidates, err)
	}
	j, err := e.q.AdmitPlaylist(ctx, p, "CCCCCCCCCCC")
	if err != nil {
		t.Fatal(err)
	}
	again, err := e.q.AdmitPlaylist(ctx, p, "CCCCCCCCCCC")
	if err != nil || again.ID != j.ID || len(e.q.Jobs()) != 1 {
		t.Fatal(again, err)
	}
	e.q.Start()
	waitPlaylistJob(t, e.q, j.ID)
	if err = e.q.Remove(j.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = e.q.AdmitPlaylist(ctx, p, "CCCCCCCCCCC"); err != nil {
		t.Fatal(err)
	}
	if len(e.q.Jobs()) != 0 {
		t.Fatal("history removal recreated download")
	}
	if err = e.s.Check(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	candidates, _ = e.r.PlaylistCandidates(ctx, 10)
	if len(candidates) != 0 {
		t.Fatal("known item resurrected", candidates)
	}
}
func TestPlaylistLocalOnlyOverridesGlobalS3AndSharedAssetGetsSelectedDelivery(t *testing.T) {
	e := newPlaylistEnv(t)
	ctx := context.Background()
	factory := func() core.CredentialVault { return security.New() }
	cloud := application.NewCloud(ctx, e.r, e.r, e.r, factory(), factory, &fakeCloud{})
	defer cloud.Close()
	cloud.Restrict([]string{})
	if err := cloud.Initialize(ctx, []byte("playlist test password")); err != nil {
		t.Fatal(err)
	}
	selected := ""
	for _, name := range []string{"global", "selected"} {
		p := core.S3Profile{ID: core.NewID(), Name: name, Region: "auto", Bucket: "bucket", Style: "path_style", Enabled: true, AutoUpload: true}
		if err := cloud.SaveProfile(ctx, p, &core.Credentials{AccessKey: "access", SecretKey: "private-key"}); err != nil {
			t.Fatal(err)
		}
		if name == "selected" {
			selected = p.ID
		}
	}
	a := e.add(t, "PLfixture00002", "all_existing")
	e.source.set("AAAAAAAAAAA")
	if err := e.s.Check(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	j, err := e.q.AdmitPlaylist(ctx, a, "AAAAAAAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	e.q.Start()
	waitPlaylistJob(t, e.q, j.ID)
	uploads, err := cloud.List(ctx, 100, 0)
	if err != nil || len(uploads) != 0 {
		t.Fatal("local-only leaked to global auto-upload", uploads, err)
	}
	b := e.add(t, "PLfixture00003", "all_existing", selected)
	if err = e.s.Check(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	shared, err := e.q.AdmitPlaylist(ctx, b, "AAAAAAAAAAA")
	if err != nil || shared.ID != j.ID {
		t.Fatal(shared, err)
	}
	uploads, err = cloud.List(ctx, 100, 0)
	if err != nil || len(uploads) != 1 || uploads[0].ProfileID != selected || uploads[0].State != "waiting_unlock" {
		t.Fatal(uploads, err)
	}
	if _, err = e.q.AdmitPlaylist(ctx, b, "AAAAAAAAAAA"); err != nil {
		t.Fatal(err)
	}
	uploads, _ = cloud.List(ctx, 100, 0)
	if len(uploads) != 1 {
		t.Fatal("delivery duplicated")
	}
	items, err := e.s.Items(ctx, b.ID, 50, 0)
	if err != nil || items.Items[0].State != "waiting_delivery" {
		t.Fatal(items, err)
	}
	profile, err := e.r.Profile(ctx, selected)
	if err != nil {
		t.Fatal(err)
	}
	profile.Prefix = "changed-destination"
	if err = cloud.SaveProfile(ctx, profile, nil); err != nil {
		t.Fatal(err)
	}
	c := e.add(t, "PLfixture00049", "all_existing", selected)
	if err = e.s.Check(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = e.q.AdmitPlaylist(ctx, c, "AAAAAAAAAAA"); err != nil {
		t.Fatal(err)
	}
	copied, err := e.s.Items(ctx, c.ID, 50, 0)
	if err != nil || len(copied.Items[0].Deliveries) != 1 || copied.Items[0].Deliveries[0].Profile.Prefix != "changed-destination" {
		t.Fatal("new subscription silently reused old destination", copied, err)
	}
	old, err := e.s.Items(ctx, b.ID, 50, 0)
	if err != nil || len(old.Items[0].Deliveries) != 1 || old.Items[0].Deliveries[0].Profile.Prefix != "" {
		t.Fatal("existing destination was redirected", old, err)
	}
	e.engine.mu.Lock()
	defer e.engine.mu.Unlock()
	if e.engine.calls != 1 {
		t.Fatal("shared file downloaded twice")
	}
}
func TestPlaylistPauseRevisionAndRestartRecovery(t *testing.T) {
	e := newPlaylistEnv(t)
	ctx := context.Background()
	p := e.add(t, "PLfixture00004", "new_only")
	scan, err := e.r.BeginPlaylistScan(ctx, p.ID, p.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.r.ObservePlaylist(ctx, scan, []core.PlaylistEntry{{ID: "AAAAAAAAAAA"}}); err != nil {
		t.Fatal(err)
	}
	if err = e.s.SetEnabled(ctx, p.ID, false); err != nil {
		t.Fatal(err)
	}
	if err = e.r.FinishPlaylistScan(ctx, scan, nil); err != nil {
		t.Fatal(err)
	}
	p, _ = e.r.Subscription(ctx, p.ID)
	if p.Initialized || p.Status != "paused" {
		t.Fatal(p)
	}
	if err = e.s.SetEnabled(ctx, p.ID, true); err != nil {
		t.Fatal(err)
	}
	p, _ = e.r.Subscription(ctx, p.ID)
	if _, err = e.r.BeginPlaylistScan(ctx, p.ID, p.Revision); err != nil {
		t.Fatal(err)
	}
	if err = e.r.RecoverPlaylistScans(ctx); err != nil {
		t.Fatal(err)
	}
	e.source.set("BBBBBBBBBBB")
	if err = e.s.Check(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	items, _ := e.s.Items(ctx, p.ID, 50, 0)
	if items.Total != 1 || items.Items[0].VideoID != "BBBBBBBBBBB" || items.Items[0].Eligible {
		t.Fatal(items)
	}
	old := p
	fresh := p
	fresh.Name = "updated"
	if _, err = e.s.Save(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	old.Name = "stale"
	if _, err = e.s.Save(ctx, old); core.CodeOf(err) != core.CodeConflict {
		t.Fatal("stale revision accepted", err)
	}
}
func TestPlaylistLargeStreamingBaselineAndPagedSubscriptions(t *testing.T) {
	e := newPlaylistEnv(t)
	ctx := context.Background()
	p := e.add(t, "PLfixture00005", "new_only")
	for i := 0; i < 10000; i++ {
		e.source.entries = append(e.source.entries, core.PlaylistEntry{ID: fmt.Sprintf("%011d", i), Title: "fixture"})
	}
	if err := e.s.Check(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	page, err := e.s.Items(ctx, p.ID, 50, 9950)
	if err != nil || page.Total != 10000 || len(page.Items) != 50 {
		t.Fatal(page.Total, len(page.Items), err)
	}
	for i := 0; i < 205; i++ {
		e.add(t, fmt.Sprintf("PLfixture%05d", i+100), "new_only")
	}
	subs, err := e.s.List(ctx, 50, 200)
	if err != nil || subs.Total != 206 || len(subs.Items) != 6 {
		t.Fatal(subs, err)
	}
}
func TestPlaylistSchedulerFairAdmissionAndNoImplicitStart(t *testing.T) {
	e := newPlaylistEnv(t)
	ctx := context.Background()
	a := e.add(t, "PLfixture00006", "all_existing")
	b := e.add(t, "PLfixture00007", "all_existing")
	e.source.set("AAAAAAAAAAA", "BBBBBBBBBBB", "CCCCCCCCCCC")
	if err := e.s.Check(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Check(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if len(e.q.Jobs()) != 0 {
		t.Fatal("read-only workspace started transfers")
	}
	e.q.Start()
	e.s.Start()
	deadline := time.Now().Add(5 * time.Second)
	for {
		items, _ := e.s.Items(ctx, b.ID, 50, 0)
		count := 0
		for _, v := range items.Items {
			if v.JobID > 0 {
				count++
			}
		}
		if count == 3 {
			break
		}
		if e.s.Err() != nil {
			t.Fatal(e.s.Err())
		}
		if time.Now().After(deadline) {
			t.Fatal("backlog starved", items)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(e.q.Jobs()) != 3 {
		t.Fatal("compatible subscriptions did not share work", len(e.q.Jobs()))
	}
}

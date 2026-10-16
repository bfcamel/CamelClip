// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bfcamel/video-downloader/internal/core"
)

type memoryRepository struct {
	mu       sync.Mutex
	snapshot Snapshot
	fail     bool
}

func (r *memoryRepository) Load(context.Context) (Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshot, nil
}
func (r *memoryRepository) Save(_ context.Context, s Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errors.New("disk full")
	}
	r.snapshot = s
	return nil
}
func (r *memoryRepository) Close() error { return nil }

type fakeDownloader struct {
	block bool
	mu    sync.Mutex
	calls int
}

func (d *fakeDownloader) Inspect(ctx context.Context, r core.Request) (core.MediaInfo, error) {
	return core.MediaInfo{ID: r.URL, Title: "Fixture", Duration: 1, Formats: []core.MediaFormat{{Height: 720, VCodec: "h264", ACodec: "aac"}}}, nil
}
func (d *fakeDownloader) Download(ctx context.Context, r core.Request, observe core.Observer) (core.Result, error) {
	d.mu.Lock()
	d.calls++
	d.mu.Unlock()
	observe(core.Event{Kind: core.StageChanged, Stage: core.Processing})
	observe(core.Event{Kind: core.Progress, Stage: core.Processing, Detail: "50%"})
	if d.block {
		<-ctx.Done()
		return core.Result{}, core.ErrCanceled
	}
	observe(core.Event{Kind: core.StageChanged, Stage: core.Verifying})
	return core.Result{Path: r.Out + "/video.mp4"}, nil
}
func (d *fakeDownloader) Formats(context.Context, core.Request, core.Observer) error { return nil }
func req() core.Request {
	return core.Request{URL: "https://example.com/video", Mode: "video", Quality: "best", Audio: "mp3", Out: "/tmp/videos", Codec: "h264", Container: "mp4"}
}
func waitState(t *testing.T, s Queue, id int, wanted State) Job {
	t.Helper()
	end := time.Now().Add(3 * time.Second)
	for time.Now().Before(end) {
		for _, j := range s.Jobs() {
			if j.ID == id && j.State == wanted {
				return j
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("state %s not reached: %#v error=%v", wanted, s.Jobs(), s.Err())
	return Job{}
}
func TestHeadlessQueueUsesStrictPortsAndImmutableSnapshots(t *testing.T) {
	repo := &memoryRepository{}
	s, err := New(context.Background(), &fakeDownloader{}, repo, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	j, err := s.Submit(req(), true)
	if err != nil {
		t.Fatal(err)
	}
	s.Start()
	waitState(t, s, j.ID, Choosing)
	snapshot := s.Jobs()
	snapshot[0].Info.Formats[0].Height = 999
	if s.Jobs()[0].Info.Formats[0].Height != 720 {
		t.Fatal("frontend mutated application state")
	}
	if err = s.Choose(j.ID, "invalid"); err == nil {
		t.Fatal("invalid quality accepted")
	}
	if err = s.Choose(j.ID, "720"); err != nil {
		t.Fatal(err)
	}
	done := waitState(t, s, j.ID, Done)
	if done.OutputPath == "" || done.Request.Quality != "720" {
		t.Fatal(done)
	}
	saved, _ := repo.Load(context.Background())
	if saved.Jobs[0].State != Done {
		t.Fatal("completion not persisted")
	}
}
func TestCancelAndRestoreDuringProcessing(t *testing.T) {
	repo := &memoryRepository{}
	d := &fakeDownloader{block: true}
	s, err := New(context.Background(), d, repo, 1)
	if err != nil {
		t.Fatal(err)
	}
	j, _ := s.Submit(req(), false)
	s.Start()
	waitState(t, s, j.ID, Processing)
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := New(context.Background(), &fakeDownloader{block: true}, repo, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if restored.Jobs()[0].State != Ready {
		t.Fatal("interrupted processing was not resumable")
	}
	restored.Start()
	waitState(t, restored, j.ID, Processing)
	if err = restored.Cancel(j.ID); err != nil {
		t.Fatal(err)
	}
	waitState(t, restored, j.ID, Canceled)
	if err = restored.Retry(j.ID, "firefox:work"); err != nil {
		t.Fatal(err)
	}
	waitState(t, restored, j.ID, Processing)
}
func TestPersistenceFailurePreventsStartingWork(t *testing.T) {
	repo := &memoryRepository{fail: true}
	d := &fakeDownloader{}
	s, err := New(context.Background(), d, repo, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Submit(req(), false); err == nil {
		t.Fatal("save failure ignored")
	}
	s.Start()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.calls != 0 {
		t.Fatal("download started without persistence")
	}
}
func TestExplicitCLISelectionDoesNotRunUnrelatedJobs(t *testing.T) {
	s, err := New(context.Background(), &fakeDownloader{}, &memoryRepository{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, _ := s.Submit(req(), false)
	r := req()
	r.URL += "/second"
	b, _ := s.Submit(r, false)
	s.StartOnly([]int{b.ID})
	waitState(t, s, b.ID, Done)
	if s.Jobs()[0].ID != a.ID || s.Jobs()[0].State != Queued {
		t.Fatal("unselected job started")
	}
}
func TestRestoreEveryLifecycleState(t *testing.T) {
	states := []State{Queued, Inspecting, Choosing, Ready, Downloading, Processing, Verifying, Canceling, Done, Failed, Canceled, WaitingDownload, WaitingProcessing, WaitingVerification}
	expected := []State{Queued, Queued, Choosing, Ready, Ready, Ready, Ready, Canceled, Done, Failed, Canceled, Ready, Ready, Ready}
	repo := &memoryRepository{}
	for i, state := range states {
		repo.snapshot.Jobs = append(repo.snapshot.Jobs, Job{ID: i + 1, Request: req(), State: state, Info: core.MediaInfo{Duration: 12}})
	}
	repo.snapshot.NextID = 11
	s, err := New(context.Background(), &fakeDownloader{}, repo, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i, j := range s.Jobs() {
		if j.State != expected[i] || j.Request.Duration != 12 {
			t.Fatal(j)
		}
	}
}

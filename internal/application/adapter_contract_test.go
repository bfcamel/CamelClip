package application

import (
	"context"
	"errors"
	"github.com/bfcamel/CamelClip/internal/core"
	"math"
	"testing"
	"time"
)

type malformedDownloader struct {
	fakeDownloader
	scenario string
}

func (d *malformedDownloader) Inspect(ctx context.Context, r core.Request) (core.MediaInfo, error) {
	if d.scenario == "metadata" {
		return core.MediaInfo{Duration: math.NaN()}, nil
	}
	return d.fakeDownloader.Inspect(ctx, r)
}
func (d *malformedDownloader) Download(ctx context.Context, r core.Request, emit core.Observer) (core.Result, error) {
	switch d.scenario {
	case "empty":
		return core.Result{}, nil
	case "outside":
		return core.Result{Path: "/outside.mp4"}, nil
	case "metrics":
		n := math.NaN()
		emit(core.Event{Kind: core.Progress, Stage: core.Downloading, Metrics: &core.Metrics{SpeedBPS: &n}})
	case "stages":
		emit(core.Event{Kind: core.StageChanged, Stage: core.Verifying})
		emit(core.Event{Kind: core.StageChanged, Stage: core.Downloading})
	case "event":
		emit(core.Event{Kind: "unknown"})
	case "canceled":
		return core.Result{}, context.Canceled
	}
	return core.Result{Path: r.Out + "/video.mp4"}, nil
}
func TestInvalidAdapterOutputFailsOnlyItsJob(t *testing.T) {
	for _, scenario := range []string{"metadata", "empty", "outside", "metrics", "stages", "event", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			s, e := New(context.Background(), &malformedDownloader{scenario: scenario}, &memoryRepository{}, 1)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			j, e := s.Submit(req(), false)
			if e != nil {
				t.Fatal(e)
			}
			s.Start()
			state := Failed
			if scenario == "canceled" {
				state = Canceled
			}
			got := waitState(t, s, j.ID, state)
			if state == Failed && got.Monitoring.ErrorCode != string(core.CodeContract) {
				t.Fatal(got)
			}
			if s.Err() != nil {
				t.Fatal("adapter error poisoned service", s.Err())
			}
			r := req()
			r.URL += "/second"
			if _, e = s.Submit(r, false); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestRetryOptionsPreserveReplaceAndClearBrowser(t *testing.T) {
	for _, v := range []struct {
		name    string
		browser *string
		want    string
	}{{"preserve", nil, "firefox:old"}, {"clear", new(string), ""}} {
		t.Run(v.name, func(t *testing.T) {
			r := req()
			r.Cookies = "firefox:old"
			s, e := New(context.Background(), &fakeDownloader{}, &memoryRepository{snapshot: Snapshot{NextID: 1, Jobs: []Job{{ID: 1, Request: r, State: Failed}}}}, 1)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			if e = s.RetryWithOptions(1, RetryOptions{Browser: v.browser}); e != nil {
				t.Fatal(e)
			}
			if s.Jobs()[0].Request.Cookies != v.want {
				t.Fatal(s.Jobs())
			}
		})
	}
}
func TestTypedRetryPolicyDoesNotParseDiagnostics(t *testing.T) {
	if retryable(errors.New("503 timeout connection reset")) {
		t.Fatal("message treated as policy")
	}
	if !retryable(&core.Failure{Code: core.CodeBackend, Temporary: true, Message: "arbitrary diagnostic"}) {
		t.Fatal("typed temporary error ignored")
	}
	if !retryable(context.DeadlineExceeded) {
		t.Fatal("deadline not retryable")
	}
}

type deferredDownloader struct{ fakeDownloader }

func (d *deferredDownloader) Inspect(context.Context, core.Request) (core.MediaInfo, error) {
	return core.MediaInfo{}, &core.Failure{Code: core.CodeNotReady, Message: "premiere pending"}
}
func TestUpcomingMediaDefersWithoutConsumingRetryBudget(t *testing.T) {
	s, e := New(context.Background(), &deferredDownloader{}, &memoryRepository{}, 1)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	j, e := s.Submit(req(), false)
	if e != nil {
		t.Fatal(e)
	}
	s.Start()
	deadline := time.Now().Add(time.Second)
	for {
		v := s.Jobs()[0]
		if v.State == Queued && !v.Control.NextAttempt.IsZero() {
			if v.Monitoring.AutomaticRetries != 0 || v.ID != j.ID {
				t.Fatal(v)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal(s.Jobs())
		}
		time.Sleep(time.Millisecond)
	}
}

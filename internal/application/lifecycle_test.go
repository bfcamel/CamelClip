package application

import (
	"context"
	"errors"
	"github.com/bfcamel/CamelClip/internal/core"
	"sync"
	"testing"
	"time"
)

func TestQueueRejectsConfigurationAfterClose(t *testing.T) {
	s, e := New(context.Background(), &fakeDownloader{}, &memoryRepository{}, 1)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	for _, e := range []error{s.SetPolicy(QueuePolicy{}), s.SetLimits(core.Limits{Downloads: 2})} {
		if !errors.Is(e, ErrClosed) {
			t.Fatal(e)
		}
	}
}
func TestLibraryRepeatedEnrichmentAndConcurrentClose(t *testing.T) {
	l := NewLibrary(nil, nil)
	l.Enrich(context.Background())
	l.Enrich(context.Background())
	done := make(chan struct{})
	go func() { l.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("enrichment leaked")
	}
	if e := l.StartScan(context.Background(), t.TempDir(), false); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
	if _, e := l.List(context.Background(), core.AssetQuery{}); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
	l.Enrich(context.Background())
	l.Close()
}
func TestLifetimeCancelsAndJoinsAdmittedCalls(t *testing.T) {
	l := newLifetime(context.Background())
	ctx, done, e := l.begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	l.stop()
	<-ctx.Done()
	if _, _, e = l.begin(context.Background()); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
	joined := make(chan struct{})
	go func() { l.wait(); close(joined) }()
	select {
	case <-joined:
		t.Fatal("returned before admitted call completed")
	default:
	}
	done()
	<-joined
}
func TestLibraryAdmissionAndCloseAreSerialized(t *testing.T) {
	for i := 0; i < 20; i++ {
		l := NewLibrary(nil, nil)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); l.Enrich(context.Background()) }()
		go func() { defer wg.Done(); l.Close() }()
		wg.Wait()
		l.Close()
	}
}

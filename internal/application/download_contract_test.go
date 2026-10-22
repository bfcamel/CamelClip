// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"github.com/bfcamel/CamelClip/internal/core"
	"sync"
	"testing"
)

type retainedObserverDownloader struct {
	fakeDownloader
	observer         core.Observer
	panicAfterEvents bool
}

func (d *retainedObserverDownloader) Download(_ context.Context, r core.Request, emit core.Observer) (core.Result, error) {
	d.observer = emit
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); emit(core.Event{Kind: core.Progress, Stage: core.Downloading}) }()
	}
	wg.Wait()
	if d.panicAfterEvents {
		panic("fixture adapter panic")
	}
	return core.Result{Path: r.Out + "/verified.mp4"}, nil
}

func TestDownloadBoundaryRejectsLateEventsAfterReturnOrPanic(t *testing.T) {
	for _, panics := range []bool{false, true} {
		t.Run(map[bool]string{false: "return", true: "panic"}[panics], func(t *testing.T) {
			d := &retainedObserverDownloader{panicAfterEvents: panics}
			r := req()
			r.Out = t.TempDir()
			observed := 0
			recovered := false
			func() {
				defer func() { recovered = recover() != nil }()
				_, err := downloadWithContract(context.Background(), d, r, func(core.Event) { observed++ })
				if err != nil {
					t.Fatal(err)
				}
			}()
			if recovered != panics || observed != 20 {
				t.Fatalf("panic=%v observed=%d", recovered, observed)
			}
			var wg sync.WaitGroup
			for range 20 {
				wg.Add(1)
				go func() { defer wg.Done(); d.observer(core.Event{Kind: core.Log, Detail: "late"}) }()
			}
			wg.Wait()
			if observed != 20 {
				t.Fatal("late callbacks mutated completed work")
			}
		})
	}
}

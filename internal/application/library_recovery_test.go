// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application_test

import (
	"context"
	"errors"
	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
	"github.com/bfcamel/CamelClip/internal/storage/sqlite"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type recoveringProber struct{ available atomic.Bool }

func (p *recoveringProber) Probe(_ context.Context, path string) (core.Asset, error) {
	if filepath.Base(path) == "missing.mp4" {
		return core.Asset{}, os.ErrNotExist
	}
	if !p.available.Load() {
		return core.Asset{}, errors.New("ffprobe temporarily unavailable")
	}
	return core.Asset{Path: path, Status: "available", Duration: 10, Kind: "video"}, nil
}
func TestLibraryEnrichmentPreservesMissingAndRetriesTransientFailures(t *testing.T) {
	ctx := context.Background()
	r, err := sqlite.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	probe := &recoveringProber{}
	l := application.NewLibrary(r, probe)
	defer l.Close()
	root := t.TempDir()
	missing, err := r.PutAsset(ctx, core.Asset{Path: filepath.Join(root, "missing.mp4"), Status: "available", NeedsProbe: true})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := r.PutAsset(ctx, core.Asset{Path: filepath.Join(root, "pending.mp4"), Status: "available", NeedsProbe: true})
	if err != nil {
		t.Fatal(err)
	}
	l.Enrich(ctx)
	until := time.Now().Add(8 * time.Second)
	for {
		a, e := r.Asset(ctx, missing.ID)
		if e != nil {
			t.Fatal(e)
		}
		if a.Status == "missing" && !a.NeedsProbe {
			break
		}
		if time.Now().After(until) {
			t.Fatal("missing status not retained", a)
		}
		time.Sleep(20 * time.Millisecond)
	}
	a, err := r.Asset(ctx, pending.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !a.NeedsProbe {
		t.Fatal("transient probe failure discarded retry")
	}
	probe.available.Store(true)
	until = time.Now().Add(38 * time.Second)
	for {
		a, err = r.Asset(ctx, pending.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !a.NeedsProbe && a.Duration == 10 {
			break
		}
		if time.Now().After(until) {
			t.Fatal("metadata did not recover after tool became available", a)
		}
		time.Sleep(20 * time.Millisecond)
	}
	a, err = r.Asset(ctx, missing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != "missing" {
		t.Fatal("stale snapshot overwrote missing status", a)
	}
}

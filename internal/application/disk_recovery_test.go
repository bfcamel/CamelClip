// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiskRecoveryWithoutExistingWorker(t *testing.T) {
	out := t.TempDir()
	stage := filepath.Join(out, ".camelclip-fixture")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(stage, "partial")
	if err := os.WriteFile(partial, []byte("ab"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(context.Background(), &fakeDownloader{}, &memoryRepository{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.SetPolicy(QueuePolicy{PendingBytes: 1}); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.staged[out] = 2
	s.mu.Unlock()
	r := req()
	r.Out = out
	j, err := s.Submit(r, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Start()
	waitState(t, s, j.ID, WaitingDisk)
	if err = os.Remove(partial); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, j.ID, Done)
}

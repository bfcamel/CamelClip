// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"github.com/bfcamel/CamelClip/internal/core"
	"testing"
	"time"
)

func TestPendingPriorityReorderingAndPauseUseActualResourceOrder(t *testing.T) {
	manager := newResources()
	manager.update(core.Limits{Downloads: 1, Processing: 1})
	manager.reorder(map[int]JobControl{1: {Priority: 0, Position: 1}, 2: {Priority: -1, Position: 2}, 3: {Priority: 1, Position: 3}, 4: {Priority: 0, Position: 4, Paused: true}})
	first := manager.prepare("downloading", 1)
	low := manager.prepare("downloading", 2)
	high := manager.prepare("downloading", 3)
	paused := manager.prepare("downloading", 4)
	manager.release(first)
	select {
	case <-high.ready:
	case <-time.After(time.Second):
		t.Fatal("high priority did not run")
	}
	select {
	case <-low.ready:
		t.Fatal("low priority ran first")
	default:
	}
	manager.release(high)
	select {
	case <-low.ready:
	case <-time.After(time.Second):
		t.Fatal("low priority starved")
	}
	manager.release(low)
	select {
	case <-paused.ready:
		t.Fatal("paused job acquired slot")
	default:
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := manager.wait(ctx, paused); e == nil {
		t.Fatal("canceled waiter")
	}
}
func TestDiskPressureBlocksAlreadyWaitingDownloads(t *testing.T) {
	manager := newResources()
	manager.update(core.Limits{Downloads: 1, Processing: 1})
	manager.reorder(map[int]JobControl{1: {Position: 1}, 2: {Position: 2}})
	first := manager.prepare("downloading", 1)
	second := manager.prepare("downloading", 2)
	manager.reorder(map[int]JobControl{1: {Position: 1}, 2: {Position: 2, DiskBlocked: true}})
	manager.release(first)
	select {
	case <-second.ready:
		t.Fatal("disk pressure ignored")
	default:
	}
	manager.reorder(map[int]JobControl{2: {Position: 2}})
	select {
	case <-second.ready:
	case <-time.After(time.Second):
		t.Fatal("disk capacity did not resume scheduling")
	}
	manager.release(second)
}

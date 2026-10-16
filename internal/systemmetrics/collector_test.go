// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package systemmetrics

import (
	"context"
	"github.com/shirou/gopsutil/v4/cpu"
	"testing"
)

func TestCPUDeltasAndReset(t *testing.T) {
	old := cpu.TimesStat{User: 10, Idle: 10}
	for _, tc := range []struct {
		next    cpu.TimesStat
		want    float64
		unknown bool
	}{{cpu.TimesStat{User: 20, Idle: 10}, 100, false}, {cpu.TimesStat{User: 10, Idle: 20}, 0, false}, {cpu.TimesStat{User: 20, Idle: 20}, 50, false}, {cpu.TimesStat{User: 1, Idle: 1}, 0, true}, {old, 0, true}} {
		got := Percent(old, tc.next)
		if tc.unknown {
			if got != nil {
				t.Fatal(got)
			}
		} else if got == nil || *got != tc.want {
			t.Fatal(got, tc)
		}
	}
}
func TestLocalMetricsFirstCPUSampleUnknown(t *testing.T) {
	c := New()
	s, e := c.Collect(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if s.CPU != nil {
		t.Fatal("fabricated initial CPU")
	}
	if s.RAMTotal == nil || *s.RAMTotal == 0 {
		t.Fatal("RAM unavailable")
	}
	if len(s.Cores) == 0 {
		t.Fatal("per-core counters unavailable")
	}
}

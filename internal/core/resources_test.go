// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package core

import "testing"

func TestProcessingDefaultFollowsUserFormula(t *testing.T) {
	for cpus, want := range map[int]int{1: 2, 2: 2, 4: 2, 8: 2, 12: 3, 16: 4, 128: 32, 256: 64} {
		if got := AutoProcessing(cpus); got != want {
			t.Fatalf("%d CPUs: got %d want %d", cpus, got, want)
		}
	}
}
func TestRateUnitsAndInvalidInput(t *testing.T) {
	for text, want := range map[string]int64{"0": 0, "1024": 1024, "512KiB/s": 512 * 1024, "1.5MiB/s": 1572864, "2 MiB/s": 2097152} {
		n, err := ParseRate(text)
		if err != nil || n != want {
			t.Fatal(text, n, err)
		}
	}
	for _, text := range []string{"", "-1", "-2MiB/s", "abc", "0.01B/s", "NaN", "Inf", "999999999999999999999999999MiB/s"} {
		if _, err := ParseRate(text); err == nil {
			t.Fatal("accepted", text)
		}
	}
}

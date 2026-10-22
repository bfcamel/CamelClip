// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package ytdlp

import (
	"github.com/bfcamel/CamelClip/internal/core"
	"testing"
)

func TestStructuredMetricsAndSeparateTrackTotals(t *testing.T) {
	events := []core.Event{}
	w := eventWriter{parts: 2, observer: func(e core.Event) { events = append(events, e) }}
	w.Write([]byte("CAMELCLIP_PROGRESS {\"downloaded_bytes\":1024,\"total_bytes\":2048,\"speed\":null,\"eta\":null,\"filename\":\"v.webm\"}\n"))
	m := events[0].Metrics
	if m.SpeedBPS != nil || m.ETASeconds != nil || m.TotalBytes != nil || m.Percent == nil || *m.Percent != 25 || !m.ProgressEstimated {
		t.Fatal("missing values or first-track progress incorrect", m)
	}
	w.Write([]byte("CAMELCLIP_PROGRESS {\"downloaded_bytes\":1024,\"total_bytes\":2048,\"speed\":4096,\"eta\":2,\"filename\":\"a.webm\"}\n"))
	m = events[1].Metrics
	if m.DownloadedBytes != 2048 || *m.TotalBytes != 4096 || *m.Percent != 50 || *m.SpeedBPS != 4096 {
		t.Fatal(m)
	}
	for _, raw := range []string{"null", `"null"`, `"NaN"`, `"Inf"`} {
		if number([]byte(raw)) != nil {
			t.Fatal("invalid metric accepted", raw)
		}
	}
}
func TestFetchNeverStartsHeavyPostprocessorsAndHasIndividualLimit(t *testing.T) {
	r := defaults()
	r.RateLimitBPS = 2097152
	args := arguments(r)
	for _, flag := range []string{"--extract-audio", "--remux-video", "--merge-output-format"} {
		if contains(args, flag) {
			t.Fatal("heavy work in download slot", args)
		}
	}
	if !contains(args, "--limit-rate") || !contains(args, "2097152") || !contains(args, "--fixup") || !contains(args, "never") {
		t.Fatal(args)
	}
}

func TestFragmentProgressAndTrackCompletionRemainVisible(t *testing.T) {
	var events []core.Event
	w := eventWriter{parts: 2, observer: func(e core.Event) { events = append(events, e) }}
	w.Write([]byte("CAMELCLIP_PROGRESS {\"downloaded_bytes\":100,\"fragment_index\":2,\"fragment_count\":10,\"filename\":\"video\"}\n"))
	m := events[len(events)-1].Metrics
	if m.Percent == nil || *m.Percent != 10 || !m.ProgressEstimated || m.PartIndex != 1 || m.PartCount != 2 {
		t.Fatal(m)
	}
	w.Write([]byte("CAMELCLIP_PROGRESS {\"downloaded_bytes\":1000,\"status\":\"finished\",\"filename\":\"video\"}\n"))
	m = events[len(events)-1].Metrics
	if m.Percent == nil || *m.Percent != 50 {
		t.Fatal(m)
	}
	w.Write([]byte("CAMELCLIP_PROGRESS {\"downloaded_bytes\":50,\"total_bytes_estimate\":100,\"filename\":\"audio\"}\n"))
	m = events[len(events)-1].Metrics
	if m.Percent == nil || *m.Percent != 75 || m.PartIndex != 2 {
		t.Fatal(m)
	}
}

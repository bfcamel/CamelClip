// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package ytdlp

import (
	"github.com/bfcamel/video-downloader/internal/core"
	"testing"
)

func TestStructuredMetricsAndSeparateTrackTotals(t *testing.T) {
	events := []core.Event{}
	w := eventWriter{parts: 2, observer: func(e core.Event) { events = append(events, e) }}
	w.Write([]byte("CLIPNARO_PROGRESS {\"downloaded_bytes\":1024,\"total_bytes\":2048,\"speed\":null,\"eta\":null,\"filename\":\"v.webm\"}\n"))
	m := events[0].Metrics
	if m.SpeedBPS != nil || m.ETASeconds != nil || m.TotalBytes != nil || m.Percent != nil {
		t.Fatal("unknown values fabricated", m)
	}
	w.Write([]byte("CLIPNARO_PROGRESS {\"downloaded_bytes\":1024,\"total_bytes\":2048,\"speed\":4096,\"eta\":2,\"filename\":\"a.webm\"}\n"))
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

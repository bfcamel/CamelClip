// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import (
	"bytes"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
	"strings"
	"testing"
	"time"
)

func TestFiltersKeepSelectionByIDAndSearchByURL(t *testing.T) {
	o := testOptions(t.TempDir())
	pool := &jobPool{jobs: []*job{{id: 10, title: "First", state: "done"}, {id: 20, title: "Second", state: "downloading", options: options{URL: "https://site.test/unique"}}}}
	u := panelUI{o: &o, pool: pool, selectedID: 20}
	u.reconcile()
	pool.jobs = append([]*job{{id: 5, title: "Earlier", state: "queued"}}, pool.jobs...)
	u.reconcile()
	if u.selectedJob().id != 20 || u.jobIndex != 2 {
		t.Fatal("selection moved", u.selectedJob())
	}
	u.search = "unique"
	u.reconcile()
	if len(u.visibleJobs()) != 1 || u.selectedJob().id != 20 {
		t.Fatal("URL search failed")
	}
	u.tab = 4
	u.reconcile()
	if len(u.visibleJobs()) != 0 {
		t.Fatal("filter mismatch")
	}
}
func TestResponsiveViewsAndOneThousandJobsStayBounded(t *testing.T) {
	old := currentLanguage
	defer useLanguage(old, "ru")
	useLanguage("en", "en")
	o := testOptions(t.TempDir())
	p := &jobPool{}
	for i := 1; i <= 1000; i++ {
		j := application.Job{ID: i, Title: fmt.Sprintf("Видео 中文 🎬 %d", i), State: application.Queued, Request: request(o)}
		p.jobs = append(p.jobs, view(j))
	}
	var output bytes.Buffer
	u := panelUI{o: &o, pool: p, out: &output, selectedID: 900}
	for _, size := range [][2]int{{130, 40}, {100, 25}, {73, 20}, {40, 10}, {20, 6}} {
		u.width, u.height = size[0], size[1]
		u.lastFrame = ""
		output.Reset()
		u.render()
		if output.Len() > size[0]*size[1]*32 {
			t.Fatal("render output not bounded by viewport")
		}
		if strings.Contains(output.String(), "\033[2J") {
			t.Fatal("full screen clearing on each frame")
		}
	}
	u.width, u.height = 130, 40
	start := time.Now()
	for i := 0; i < 20; i++ {
		output.Reset()
		u.render()
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("large-history UI stalled")
	}
}
func TestUnknownAndStaleMeasurementsAreNotFabricated(t *testing.T) {
	m := core.Metrics{}
	if percent(m) != "—" || speed(m) != "—" || eta(m) != "—" {
		t.Fatal("unknown values rendered as zero")
	}
	n := 1024.0
	m.SpeedBPS = &n
	m.At = time.Now().Add(-5 * time.Second)
	if speed(m) != "—*" {
		t.Fatal("stale speed not marked")
	}
	j := &job{state: "processing", record: application.Job{Monitoring: application.Monitoring{Download: core.Metrics{Percent: floatPointer(100)}}}}
	if strings.Contains(progressCell(j, 4), "100%") {
		t.Fatal("download completion masquerades as processing completion")
	}
}
func floatPointer(n float64) *float64 { return &n }

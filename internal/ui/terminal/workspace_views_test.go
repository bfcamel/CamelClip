// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"bytes"
	"context"
	"github.com/bfcamel/video-downloader/internal/application"
	"github.com/bfcamel/video-downloader/internal/bootstrap"
	"github.com/bfcamel/video-downloader/internal/core"
	"strings"
	"testing"
	"time"
)

type fixedCollector struct{ cores int }

func (f fixedCollector) Collect(context.Context) (core.SystemSnapshot, error) {
	v := 50.0
	total := uint64(16 << 30)
	used := uint64(8 << 30)
	s := core.SystemSnapshot{CPU: &v, RAMTotal: &total, RAMUsed: &used}
	for i := 0; i < f.cores; i++ {
		n := float64(i % 101)
		s.Cores = append(s.Cores, &n)
	}
	return s, nil
}
func TestSecretFormsNeverRenderCredentials(t *testing.T) {
	o := testOptions(t.TempDir())
	for _, kind := range []string{"vault", "profile-field"} {
		var out bytes.Buffer
		u := panelUI{o: &o, out: &out, width: 100, height: 30, modal: kind, title: "Credentials", input: "SUPER-PRIVATE-SECRET", formStep: 8, pool: &jobPool{}}
		u.render()
		if strings.Contains(out.String(), "SUPER-PRIVATE") {
			t.Fatal("secret rendered")
		}
		if !strings.Contains(out.String(), "•") {
			t.Fatal("secret input not masked")
		}
	}
}
func TestResourcePanelIncludesEveryLogicalCPUAndSupportsScroll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w, e := bootstrap.OpenWorkspace(bootstrap.Downloader())(ctx, t.TempDir()+"/settings.json")
	if e != nil {
		t.Fatal(e)
	}
	defer w.Close()
	w.Monitor.Close()
	w.Monitor = application.NewSystemMonitor(ctx, fixedCollector{32})
	for until := time.Now().Add(time.Second); time.Now().Before(until); {
		if len(w.Monitor.Snapshot().Cores) == 32 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	o := testOptions(t.TempDir())
	u := panelUI{ctx: ctx, o: &o, pool: &jobPool{workspace: w, service: w.Queue}}
	text := u.resourcesText()
	if !strings.Contains(text, "CPU 31") {
		t.Fatal("last CPU omitted", text)
	}
	u.modal = "resources-panel"
	u.modalKey("down")
	if u.detailOffset != 1 {
		t.Fatal("resources cannot scroll")
	}
}

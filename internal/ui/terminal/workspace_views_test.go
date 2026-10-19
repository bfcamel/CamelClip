// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"bytes"
	"context"
	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/bootstrap"
	"github.com/bfcamel/CamelClip/internal/core"
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

func TestNewProfilePromptsForVaultAndContinues(t *testing.T) {
	ctx := context.Background()
	w, err := bootstrap.OpenWorkspace(bootstrap.Downloader())(ctx, t.TempDir()+"/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	u := panelUI{ctx: ctx, pool: &jobPool{workspace: w, service: w.Queue}, section: 3}
	u.workspaceKey("n")
	if u.modal != "vault" || u.secretMode != "initialize" {
		t.Fatalf("new profile did not open vault setup: %q %q", u.modal, u.secretMode)
	}
	u.workspaceModalKey("escape")
	if u.newProfileAfterUnlock || u.input != "" {
		t.Fatal("cancel retained pending creation")
	}
	u.workspaceKey("n")
	u.modal = ""
	u.resumeProfileCreation()
	if u.modal != "" {
		t.Fatal("continued before successful unlock")
	}
	if err := w.Cloud.Initialize(ctx, []byte("test-password-123")); err != nil {
		t.Fatal(err)
	}
	u.resumeProfileCreation()
	if u.modal != "profile-field" || u.formStep != 0 || u.profileForm.ID == "" {
		t.Fatal("profile wizard did not continue")
	}
	w.Cloud.Lock()
	u.modal = ""
	u.workspaceKey("n")
	if u.modal != "vault" || u.secretMode != "unlock" {
		t.Fatal("existing vault did not request unlock")
	}
	u.workspaceModalKey("escape")
	if err := w.Cloud.Unlock(ctx, []byte("test-password-123")); err != nil {
		t.Fatal(err)
	}
	u.resumeProfileCreation()
	if u.modal != "" {
		t.Fatal("canceled wizard resumed")
	}
	u.workspaceKey("n")
	if u.modal != "profile-field" {
		t.Fatal("unlocked vault did not open profile wizard directly")
	}
}

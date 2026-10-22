// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func fixture(t *testing.T) (Layout, string, string) {
	t.Helper()
	root := t.TempDir()
	canonical, e := filepath.EvalSymlinks(root)
	if e != nil {
		t.Fatal(e)
	}
	root = canonical
	l, e := UserLayout(filepath.Join(root, "prefix with ' quote"))
	if e != nil {
		t.Fatal(e)
	}
	source := filepath.Join(root, "source")
	os.MkdirAll(source, 0755)
	m := Manifest{Schema: 1, Version: "0.12.1", OS: runtime.GOOS, Arch: runtime.GOARCH, Components: []Component{}}
	for _, id := range []string{"manager", "terminal", "gui", "tui", "cli"} {
		data := []byte("payload " + id)
		name := id
		if id == "terminal" {
			name = "runtime/terminal"
		}
		area := "bin"
		if id == "terminal" {
			area = "data"
		}
		os.MkdirAll(filepath.Dir(filepath.Join(source, id)), 0755)
		os.WriteFile(filepath.Join(source, id), data, 0755)
		m.Components = append(m.Components, Component{ID: id, Version: m.Version, Files: []File{{Source: id, Area: area, Path: name, SHA256: digest(data), Size: int64(len(data)), Mode: 0755}}})
	}
	data, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(source, "manifest.json"), data, 0644)
	config := filepath.Join(root, "config")
	os.MkdirAll(config, 0700)
	os.WriteFile(filepath.Join(config, "settings.json"), []byte("user settings"), 0600)
	return l, source, config
}
func TestAllSevenSelectionsAndUninstall(t *testing.T) {
	for mask := 1; mask < 8; mask++ {
		t.Run(string(rune('0'+mask)), func(t *testing.T) {
			l, source, config := fixture(t)
			ids := []string{}
			for i, id := range []string{"gui", "tui", "cli"} {
				if mask&(1<<i) != 0 {
					ids = append(ids, id)
				}
			}
			o := Options{Source: source, Components: ids, ConfigDir: config}
			if e := Apply(l, o); e != nil {
				t.Fatal(e)
			}
			if e := Apply(l, o); e != nil {
				t.Fatal("reinstall:", e)
			}
			installed, e := ReadInstalled(l)
			if e != nil {
				t.Fatal(e)
			}
			for _, id := range []string{"gui", "tui", "cli", "manager", "terminal"} {
				_, got := installed[id]
				want := expand(ids)[id]
				if got != want {
					t.Fatalf("%s got=%v want=%v", id, got, want)
				}
			}
			if e = Apply(l, Options{Uninstall: true, ConfigDir: config}); e != nil {
				t.Fatal(e)
			}
			installed, e = ReadInstalled(l)
			if e != nil || len(installed) != 0 {
				t.Fatal(installed, e)
			}
			data, _ := os.ReadFile(filepath.Join(config, "settings.json"))
			if string(data) != "user settings" {
				t.Fatal("settings changed")
			}
		})
	}
}
func TestAddRemovePreservesSharedRuntime(t *testing.T) {
	l, source, config := fixture(t)
	for _, id := range []string{"cli", "tui", "gui"} {
		if e := Apply(l, Options{Source: source, Components: []string{id}, ConfigDir: config}); e != nil {
			t.Fatal(e)
		}
	}
	for _, id := range []string{"cli", "tui"} {
		if e := Apply(l, Options{Remove: []string{id}, ConfigDir: config}); e != nil {
			t.Fatal(e)
		}
		installed, _ := ReadInstalled(l)
		_, terminal := installed["terminal"]
		if terminal != (id == "cli") {
			t.Fatalf("shared runtime after removing %s", id)
		}
	}
	installed, _ := ReadInstalled(l)
	if len(Visible(installed)) != 1 || Visible(installed)[0] != "gui" {
		t.Fatal(installed)
	}
}
func TestUnownedAndModifiedPathsArePreserved(t *testing.T) {
	l, source, config := fixture(t)
	os.MkdirAll(filepath.Join(l.Prefix, "bin"), 0755)
	path := filepath.Join(l.Prefix, "bin", "cli")
	os.WriteFile(path, []byte("unrelated"), 0755)
	e := Apply(l, Options{Source: source, Components: []string{"cli"}, ConfigDir: config})
	if e == nil || !strings.Contains(e.Error(), "unowned") {
		t.Fatal(e)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "unrelated" {
		t.Fatal("unowned file changed")
	}
	os.Remove(path)
	if e = Apply(l, Options{Source: source, Components: []string{"cli"}, ConfigDir: config}); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(path, []byte("modified"), 0755)
	if e = Apply(l, Options{Uninstall: true, ConfigDir: config}); e == nil {
		t.Fatal("modified file was deleted")
	}
}
func TestCorruptPayloadAndRollback(t *testing.T) {
	l, source, config := fixture(t)
	o := Options{Source: source, Components: []string{"cli", "gui"}, ConfigDir: config, failAt: 2}
	if e := Apply(l, o); e == nil {
		t.Fatal("expected injected failure")
	}
	installed, _ := ReadInstalled(l)
	if len(installed) != 0 {
		t.Fatal("partial inventory committed")
	}
	if _, e := os.Stat(filepath.Join(l.Prefix, "bin", "cli")); !os.IsNotExist(e) {
		t.Fatal("new executable not rolled back")
	}
	o.failAt = 0
	if e := Apply(l, o); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(source, "cli"), []byte("tampered"), 0755)
	if e := Apply(l, o); e == nil {
		t.Fatal("tampered payload accepted")
	}
	data, _ := os.ReadFile(filepath.Join(l.Prefix, "bin", "cli"))
	if string(data) != "payload cli" {
		t.Fatal("existing payload changed on failed update")
	}
}
func TestRunningWorkspaceAndConcurrentSetup(t *testing.T) {
	l, source, config := fixture(t)
	path := filepath.Join(config, "queue.lock")
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	if e = Apply(l, Options{Source: source, Components: []string{"cli"}, ConfigDir: config}); e == nil {
		t.Fatal("running workspace was ignored")
	}
}
func TestWrongArchitecturePathsAndSymlinks(t *testing.T) {
	l, source, config := fixture(t)
	for _, s := range []string{"server", "", "gui,server"} {
		if _, e := Choices(s); e == nil {
			t.Fatalf("invalid choices accepted: %q", s)
		}
	}
	for _, p := range []string{"../escape", "/absolute", "x/../../escape"} {
		if _, e := l.Destination(File{Area: "bin", Path: p}); e == nil {
			t.Fatal("unsafe destination")
		}
	}
	os.MkdirAll(l.Prefix, 0755)
	outside := t.TempDir()
	if e := os.Symlink(outside, filepath.Join(l.Prefix, "bin")); e != nil {
		t.Fatal(e)
	}
	if e := Apply(l, Options{Source: source, Components: []string{"cli"}, ConfigDir: config}); e == nil {
		t.Fatal("symlink accepted")
	}
	var m Manifest
	parse(filepath.Join(source, "manifest.json"), &m)
	m.Arch = "unsupported"
	data, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(source, "manifest.json"), data, 0644)
	if _, e := Load(source); e == nil {
		t.Fatal("wrong architecture accepted")
	}
}
func TestPackagePreflightAndDowngrade(t *testing.T) {
	l, source, config := fixture(t)
	if e := Apply(l, Options{Source: source, Components: []string{"cli"}, ConfigDir: config}); e != nil {
		t.Fatal(e)
	}
	installed, _ := ReadInstalled(l)
	c := installed["cli"]
	record := filepath.Join(source, "incoming.json")
	data, _ := json.Marshal(c)
	os.WriteFile(record, data, 0644)
	if e := PreflightRecord(l, record); e != nil {
		t.Fatal(e)
	}
	c.Version = "0.11.0"
	data, _ = json.Marshal(c)
	os.WriteFile(record, data, 0644)
	if e := PreflightRecord(l, record); e == nil {
		t.Fatal("package downgrade allowed")
	}
	if !older("0.9.0", "0.12.1") || older("0.12.1", "0.9.0") {
		t.Fatal("version order")
	}
}

func TestFailedUpgradeRestoresExistingExecutablesAndInventory(t *testing.T) {
	l, source, config := fixture(t)
	o := Options{Source: source, Components: []string{"cli", "gui"}, ConfigDir: config}
	if e := Apply(l, o); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(componentPath(l, "cli"))
	if e != nil {
		t.Fatal(e)
	}
	var m Manifest
	if e = parse(filepath.Join(source, "manifest.json"), &m); e != nil {
		t.Fatal(e)
	}
	m.Version = "0.13.0"
	for i := range m.Components {
		m.Components[i].Version = m.Version
		for j := range m.Components[i].Files {
			f := &m.Components[i].Files[j]
			data := []byte("updated " + m.Components[i].ID)
			os.WriteFile(filepath.Join(source, f.Source), data, 0755)
			f.SHA256 = digest(data)
			f.Size = int64(len(data))
		}
	}
	data, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(source, "manifest.json"), data, 0644)
	// Four executable writes and the first metadata write complete before failure.
	o.failAt = 6
	if e = Apply(l, o); e == nil {
		t.Fatal("expected failed upgrade")
	}
	after, _ := os.ReadFile(componentPath(l, "cli"))
	if string(before) != string(after) {
		t.Fatal("inventory was not rolled back")
	}
	old, _ := os.ReadFile(filepath.Join(l.Prefix, "bin", "cli"))
	if string(old) != "payload cli" {
		t.Fatal("executable was not rolled back")
	}
	o.failAt = 0
	if e = Apply(l, o); e != nil {
		t.Fatal(e)
	}
}

// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package gui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bfcamel/CamelClip/internal/bootstrap"
	"github.com/bfcamel/CamelClip/internal/core"
	"github.com/bfcamel/CamelClip/internal/ui/gui"
)

type fixtureDownloader struct{}

func (fixtureDownloader) Inspect(context.Context, core.Request) (core.MediaInfo, error) {
	return core.MediaInfo{Title: "Test <video>", Formats: []core.MediaFormat{{Height: 720}, {Height: 1080}}}, nil
}
func (fixtureDownloader) Download(ctx context.Context, r core.Request, o core.Observer) (core.Result, error) {
	<-ctx.Done()
	return core.Result{}, ctx.Err()
}
func (fixtureDownloader) Formats(context.Context, core.Request, core.Observer) error { return nil }
func fixture(t *testing.T) (*gui.Server, string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	host := "127.0.0.1:12345"
	deps := gui.Dependencies{OpenConfig: bootstrap.OpenConfig, OpenWorkspace: bootstrap.OpenWorkspace(fixtureDownloader{})}
	s, e := gui.New(context.Background(), deps, path, gui.Defaults(dir), host, func() {})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	})
	return s, path, strings.Split(s.URL(), "#")[1]
}
func call(t *testing.T, s *gui.Server, token, method, path string, data any) (int, map[string]any, []byte) {
	t.Helper()
	b, _ := json.Marshal(data)
	r := httptest.NewRequest(method, "http://127.0.0.1:12345"+path, bytes.NewReader(b))
	r.Header.Set("X-CamelClip-Token", token)
	r.Header.Set("Origin", "http://127.0.0.1:12345")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	var v map[string]any
	json.Unmarshal(w.Body.Bytes(), &v)
	return w.Code, v, w.Body.Bytes()
}
func TestSessionBoundary(t *testing.T) {
	s, _, token := fixture(t)
	for _, path := range []string{"/api/state", "/api/library"} {
		code, _, _ := call(t, s, "", http.MethodGet, path, nil)
		if code != 401 {
			t.Fatalf("unauthenticated %s: %d", path, code)
		}
	}
	for _, kind := range []string{"host", "origin"} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:12345/api/action", strings.NewReader(`{"action":"initialize","password":"secret"}`))
		r.Header.Set("X-CamelClip-Token", token)
		if kind == "host" {
			r.Host = "evil.example"
		} else {
			r.Header.Set("Origin", "https://evil.example")
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("invalid %s allowed", kind)
		}
	}
	code, _, _ := call(t, s, token, "GET", "/api/state", nil)
	if code != 200 {
		t.Fatal(code)
	}
}
func TestExistingOwnerLockAndSettings(t *testing.T) {
	s, path, token := fixture(t)
	_, err := bootstrap.OpenWorkspace(fixtureDownloader{})(context.Background(), path)
	if err == nil {
		t.Fatal("GUI did not hold the common queue lock")
	}
	code, v, _ := call(t, s, token, "GET", "/api/state", nil)
	if code != 200 || v["firstRun"] != true {
		t.Fatal(v)
	}
	settings := gui.Defaults(filepath.Dir(path))
	settings.Out = filepath.Join(filepath.Dir(path), "output")
	code, v, _ = call(t, s, token, "POST", "/api/settings", settings)
	if code != 200 {
		t.Fatal(v)
	}
	saved, e := os.ReadFile(path)
	if e != nil || !bytes.Contains(saved, []byte(`"output_directory"`)) {
		t.Fatal(e)
	}
	_, v, _ = call(t, s, token, "GET", "/api/state", nil)
	if v["firstRun"] != false {
		t.Fatal("setup still pending")
	}
	settings.MaxDownloads = 33
	code, _, _ = call(t, s, token, "POST", "/api/settings", settings)
	if code != 400 {
		t.Fatal("invalid settings accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(saved, after) {
		t.Fatal("invalid write changed preferences")
	}
}
func TestBatchValidationAndQuality(t *testing.T) {
	s, path, token := fixture(t)
	settings := gui.Defaults(filepath.Dir(path))
	call(t, s, token, "POST", "/api/settings", settings)
	code, _, _ := call(t, s, token, "POST", "/api/action", map[string]any{"action": "submit", "urls": []string{"https://example.com/a", "file:///secret"}})
	if code != 400 {
		t.Fatal(code)
	}
	_, v, _ := call(t, s, token, "GET", "/api/state", nil)
	if len(v["jobs"].([]any)) != 0 {
		t.Fatal("invalid batch partially submitted")
	}
	code, _, body := call(t, s, token, "POST", "/api/action", map[string]any{"action": "submit", "urls": []string{"https://example.com/a"}, "choose": true})
	if code != 200 {
		t.Fatal(string(body))
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, v, _ = call(t, s, token, "GET", "/api/state", nil)
		jobs := v["jobs"].([]any)
		if len(jobs) == 1 && jobs[0].(map[string]any)["State"] == "choose" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(v)
		}
		time.Sleep(10 * time.Millisecond)
	}
	code, v, _ = call(t, s, token, "POST", "/api/action", map[string]any{"action": "choose", "id": "1", "value": "1080"})
	if code != 200 {
		t.Fatal(v)
	}
	code, _, _ = call(t, s, token, "POST", "/api/action", map[string]any{"action": "remove", "id": "1"})
	if code != 400 {
		t.Fatal("unconfirmed history removal accepted")
	}
}
func TestStrictRequestsAndPrivacy(t *testing.T) {
	s, _, token := fixture(t)
	code, _, _ := call(t, s, token, "POST", "/api/action", map[string]any{"action": "lock", "unexpected": "secret"})
	if code != 400 {
		t.Fatal("unknown request field accepted")
	}
	code, _, body := call(t, s, token, "GET", "/api/state", nil)
	if code != 200 || bytes.Contains(body, []byte(token)) {
		t.Fatal("session token leaked")
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:12345/", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("missing frame isolation")
	}
}

// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package diagnostics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bfcamel/CamelClip/internal/config"
)

func testSettings(dir string) config.Settings {
	return config.Settings{Language: "en", Mode: "video", Quality: "best", Audio: "mp3", Out: dir, Codec: "h264", Container: "mp4", Browser: "firefox:/private/secret-profile"}
}
func TestReportRedactsToolsPathsAndCookieSelectors(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "tools")
	os.Mkdir(bin, 0700)
	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "deno"} {
		script := "#!/bin/sh\nif [ \"$1\" = -hide_banner ]; then printf ' V..... libx264 encoder\\n A..... aac encoder\\n'; else echo 'tool 1.2.3 /private/user-secret https://example.com/secret?token=123'; fi\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	s := testSettings(dir)
	report := Collect(context.Background(), "test", "development", filepath.Join(dir, "settings.json"), s)
	if report.Failed() {
		t.Fatal(report)
	}
	data, _ := json.Marshal(report)
	for _, secret := range []string{dir, "secret-profile", "user-secret", "example.com", "token=123"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("private data in report: %s", data)
		}
	}
	if !strings.Contains(string(data), "1.2.3") {
		t.Fatal("version missing")
	}
}
func TestMissingEncodersInvalidSettingsAndCorruptQueue(t *testing.T) {
	dir := t.TempDir()
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "deno"} {
		os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\necho '1.2.3'\n"), 0700)
	}
	configPath := filepath.Join(dir, "settings.json")
	os.WriteFile(configPath, []byte("{broken"), 0600)
	queue := filepath.Join(dir, "queue.sqlite3")
	os.WriteFile(queue, []byte("corrupted database"), 0600)
	report := Collect(context.Background(), "test", "dev", configPath, testSettings(dir))
	bad := map[string]bool{}
	for _, check := range report.Checks {
		if check.Status == "FAIL" {
			bad[check.ID] = true
		}
	}
	for _, id := range []string{"encoder:libx264", "encoder:aac", "settings", "queue"} {
		if !bad[id] {
			t.Fatalf("missing failure %s: %+v", id, report)
		}
	}
	after, _ := os.ReadFile(queue)
	if string(after) != "corrupted database" {
		t.Fatal("diagnostics changed corrupt queue")
	}
}

// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

// Package diagnostics produces a deliberately bounded, shareable report. It does
// not read browser cookies or include URLs, usernames, paths or raw tool output.
package diagnostics

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/bfcamel/CamelClip/internal/config"
	_ "modernc.org/sqlite"
)

type Check struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}
type Report struct {
	Version string  `json:"version"`
	Commit  string  `json:"commit"`
	OS      string  `json:"os"`
	Arch    string  `json:"arch"`
	Go      string  `json:"go"`
	Profile Profile `json:"profile"`
	Checks  []Check `json:"checks"`
}
type Profile struct {
	Mode           string `json:"mode"`
	VideoCodec     string `json:"video_codec"`
	Container      string `json:"container"`
	Audio          string `json:"audio"`
	BrowserCookies bool   `json:"browser_cookies_enabled"`
}

func (r Report) Failed() bool {
	for _, c := range r.Checks {
		if c.Status == "FAIL" {
			return true
		}
	}
	return false
}
func (r *Report) add(id, status, message, hint string) {
	r.Checks = append(r.Checks, Check{id, status, message, hint})
}

var versionPattern = regexp.MustCompile(`\b[0-9]+(?:[.][0-9]+){1,3}\b`)

func Collect(ctx context.Context, version, commit, configPath string, s config.Settings) Report {
	r := Report{Version: version, Commit: commit, OS: runtime.GOOS, Arch: runtime.GOARCH, Go: runtime.Version(), Profile: Profile{s.Mode, s.Codec, s.Container, s.Audio, s.Browser != ""}}
	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "deno"} {
		status := "FAIL"
		if name == "deno" {
			status = "WARN"
		}
		path, err := exec.LookPath(name)
		if err != nil {
			r.add(name, status, "Executable is missing from PATH.", "Install "+name+" and run doctor again; see README for your OS.")
			continue
		}
		args := []string{"--version"}
		if name == "ffmpeg" || name == "ffprobe" {
			args = []string{"-version"}
		}
		out, err := tool(ctx, path, args...)
		if err != nil {
			r.add(name, status, "Executable could not be started or timed out.", "Reinstall the dependency and check execute permissions.")
			continue
		}
		// The executable can print arbitrary local data. Retain only numeric versions.
		v := versionPattern.FindString(strings.SplitN(string(out), "\n", 2)[0])
		if v == "" {
			v = "available"
		}
		r.add(name, "OK", v, "")
	}
	if out, err := tool(ctx, "ffmpeg", "-hide_banner", "-encoders"); err == nil {
		required := []string{}
		if s.Mode == "audio" {
			encoder := map[string]string{"mp3": "libmp3lame", "m4a": "aac", "opus": "libopus", "flac": "flac", "wav": "pcm_s16le", "aac": "aac", "ogg": "libvorbis"}[s.Audio]
			if encoder != "" {
				required = append(required, encoder)
			}
		} else {
			encoder := map[string]string{"h264": "libx264", "hevc": "libx265", "vp9": "libvpx-vp9", "av1": "libsvtav1"}[s.Codec]
			if encoder != "" {
				required = append(required, encoder)
			}
			audio := "aac"
			if s.Container == "webm" {
				audio = "libopus"
			}
			required = append(required, audio)
		}
		for _, encoder := range required {
			found := false
			for _, line := range strings.Split(string(out), "\n") {
				fields := strings.Fields(line)
				if len(fields) > 1 && fields[1] == encoder {
					found = true
					break
				}
			}
			if found {
				r.add("encoder:"+encoder, "OK", "Selected encoder is available.", "")
			} else {
				r.add("encoder:"+encoder, "FAIL", "Selected encoder is absent from this FFmpeg build.", "Install a full FFmpeg build or choose another codec in Settings.")
			}
		}
	}
	for _, location := range []struct{ id, path string }{{"config-directory", filepath.Dir(configPath)}, {"output-directory", s.Out}} {
		checkDirectory(&r, location.id, location.path)
	}
	checkSettings(&r, configPath)
	checkQueue(ctx, &r, filepath.Join(filepath.Dir(configPath), "queue.sqlite3"))
	if s.Browser != "" {
		r.add("browser-cookies", "INFO", "Browser cookies are selected; contents were not read.", "Use the same Firefox/Chrome profile and network. Close the browser if its database is locked. Flatpak/Snap profiles may require an explicit profile path.")
	}
	r.add("youtube-ejs", "INFO", "Offline checks cannot verify YouTube authentication or JavaScript challenges.", "Install yt-dlp EJS components and a supported JavaScript runtime; see README.")
	if runtime.GOOS == "linux" {
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			r.add("desktop", "INFO", "Headless session: downloads work; opening files requires a desktop.", "")
		} else if _, err := exec.LookPath("xdg-open"); err != nil {
			r.add("desktop", "WARN", "xdg-open is missing.", "Install xdg-utils to open files from the library.")
		} else {
			r.add("desktop", "OK", "Desktop opener is available.", "")
		}
	}
	return r
}
func tool(parent context.Context, path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.WaitDelay = 250 * time.Millisecond
	return cmd.CombinedOutput()
}
func checkDirectory(r *Report, id, path string) {
	// Probe an existing ancestor without creating configured directories.
	existing := path
	for {
		info, err := os.Stat(existing)
		if err == nil {
			if !info.IsDir() {
				r.add(id, "FAIL", "A path component is a file.", "Choose an accessible directory in Settings.")
				return
			}
			break
		}
		if !os.IsNotExist(err) || filepath.Dir(existing) == existing {
			r.add(id, "FAIL", "Directory cannot be accessed.", "Check directory permissions in Settings.")
			return
		}
		existing = filepath.Dir(existing)
	}
	f, err := os.CreateTemp(existing, ".camelclip-doctor-*")
	if err != nil {
		r.add(id, "FAIL", "Directory is not writable.", "Check permissions or choose another directory.")
		return
	}
	name := f.Name()
	err = f.Close()
	os.Remove(name)
	if err != nil {
		r.add(id, "FAIL", "Write probe failed.", "Check storage availability.")
		return
	}
	var stat syscall.Statfs_t
	if err = syscall.Statfs(existing, &stat); err == nil && uint64(stat.Bavail)*uint64(stat.Bsize) < 512*1024*1024 {
		r.add(id, "WARN", "Less than 512 MiB of free space.", "Free disk space; conversion temporarily needs space for source and result.")
		return
	}
	r.add(id, "OK", "Directory is writable.", "")
}
func checkSettings(r *Report, path string) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		r.add("settings", "INFO", "No saved settings; defaults are in use.", "")
		return
	}
	var s config.Settings
	if err == nil {
		err = json.Unmarshal(data, &s)
	}
	if err == nil {
		err = config.Validate(s)
	}
	if err != nil {
		r.add("settings", "FAIL", "Saved settings are unreadable or invalid; startup uses defaults.", "Back up settings.json and correct or rename it.")
		return
	}
	r.add("settings", "OK", "Saved settings are valid.", "")
}
func checkQueue(ctx context.Context, r *Report, path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		r.add("queue", "INFO", "Queue has not been created yet.", "")
		return
	}
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "ro")
	q.Set("_pragma", "busy_timeout(1000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err == nil {
		defer db.Close()
		check, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		var result string
		err = db.QueryRowContext(check, "PRAGMA quick_check").Scan(&result)
		if err == nil {
			var schema int
			err = db.QueryRowContext(check, "PRAGMA user_version").Scan(&schema)
			if err == nil && (schema < 1 || schema > 5) {
				err = fmt.Errorf("unsupported schema")
			}
		}
		if err == nil && result != "ok" {
			err = fmt.Errorf("integrity check failed")
		}
	}
	if err != nil {
		r.add("queue", "FAIL", "Queue could not be read or failed its integrity check.", "Close other sessions, back up queue.sqlite3 with its -wal/-shm files, then follow docs/troubleshooting.md. Do not delete the database.")
		return
	}
	r.add("queue", "OK", "SQLite integrity check passed.", "")
}

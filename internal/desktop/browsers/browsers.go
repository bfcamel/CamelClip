// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package browsers

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type browserOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
type Catalog struct {
	Options   []browserOption `json:"options"`
	Suggested string          `json:"suggested"`
}

func Discover(ctx context.Context) Catalog {
	home, err := os.UserHomeDir()
	if err != nil {
		return Catalog{}
	}
	configDir, _ := os.UserConfigDir()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		const script = `ObjC.import('AppKit'); const app=$.NSWorkspace.sharedWorkspace.URLForApplicationToOpenURL($.NSURL.URLWithString('https://example.invalid')); ObjC.unwrap($.NSBundle.bundleWithURL(app).bundleIdentifier)`
		cmd = exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-e", script)
	} else {
		cmd = exec.CommandContext(ctx, "xdg-settings", "get", "default-web-browser")
	}
	data, _ := cmd.Output()
	return browserProfiles(home, configDir, runtime.GOOS, strings.ToLower(strings.TrimSpace(string(data))))
}

// Inspect only file metadata. Never open a cookie store during discovery.
func browserProfiles(home, configDir, platform, handler string) Catalog {
	var result Catalog
	defaults := map[string]string{
		"com.apple.safari": "safari", "com.google.chrome": "chrome", "com.microsoft.edgemac": "edge", "com.brave.browser": "brave", "org.mozilla.firefox": "firefox",
		"google-chrome.desktop": "chrome", "com.google.chrome.desktop": "chrome", "microsoft-edge.desktop": "edge", "com.microsoft.edge.desktop": "edge", "brave-browser.desktop": "brave", "com.brave.browser.desktop": "brave", "firefox.desktop": "firefox", "firefox_firefox.desktop": "firefox", "org.mozilla.firefox.desktop": "firefox",
	}
	type location struct{ browser, label, root string }
	var roots []location
	if platform == "darwin" {
		roots = []location{{"chrome", "Chrome", filepath.Join(configDir, "Google/Chrome")}, {"edge", "Edge", filepath.Join(configDir, "Microsoft Edge")}, {"brave", "Brave", filepath.Join(configDir, "BraveSoftware/Brave-Browser")}, {"firefox", "Firefox", filepath.Join(configDir, "Firefox/Profiles")}}
		for _, p := range []string{"Library/Cookies/Cookies.binarycookies", "Library/Containers/com.apple.Safari/Data/Library/Cookies/Cookies.binarycookies"} {
			if regularFile(filepath.Join(home, p)) {
				result.Options = append(result.Options, browserOption{"safari", "Safari"})
				break
			}
		}
	} else {
		roots = []location{{"chrome", "Chrome", filepath.Join(configDir, "google-chrome")}, {"edge", "Edge", filepath.Join(configDir, "microsoft-edge")}, {"brave", "Brave", filepath.Join(configDir, "BraveSoftware/Brave-Browser")}}
		for _, p := range []string{".mozilla/firefox", "snap/firefox/common/.mozilla/firefox", ".var/app/org.mozilla.firefox/.mozilla/firefox"} {
			roots = append(roots, location{"firefox", "Firefox", filepath.Join(home, p)})
		}
	}
	for _, r := range roots {
		entries, _ := os.ReadDir(r.root)
		for _, entry := range entries {
			if !entry.IsDir() || strings.ContainsAny(entry.Name(), ":\n\r") {
				continue
			}
			dir := filepath.Join(r.root, entry.Name())
			if r.browser == "firefox" {
				if !regularFile(filepath.Join(dir, "cookies.sqlite")) {
					continue
				}
			} else if !regularFile(filepath.Join(dir, "Cookies")) && !regularFile(filepath.Join(dir, "Network", "Cookies")) {
				continue
			}
			result.Options = append(result.Options, browserOption{r.browser + ":" + dir, r.label + " — " + entry.Name()})
		}
	}
	// A browser-only selector delegates profile choice to yt-dlp. For multiple
	// profiles, keep explicit profile choices alongside that browser-level default.
	count := 0
	browserLabel := ""
	for _, o := range result.Options {
		name, _, _ := strings.Cut(o.Value, ":")
		if name == defaults[handler] {
			count++
			browserLabel, _, _ = strings.Cut(o.Label, " — ")
			result.Suggested = o.Value
		}
	}
	if count > 1 {
		result.Suggested = defaults[handler]
		result.Options = append([]browserOption{{result.Suggested, browserLabel}}, result.Options...)
	}
	return result
}
func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

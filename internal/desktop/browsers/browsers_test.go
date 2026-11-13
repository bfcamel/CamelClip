package browsers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFirstSetupBrowserSuggestion(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config")
	store := func(profile string) string {
		dir := filepath.Join(configDir, "google-chrome", profile)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Cookies"), nil, 0600); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	discover := func(handler string) Catalog { return browserProfiles(home, configDir, "linux", handler) }
	if got := discover("google-chrome.desktop"); len(got.Options) != 0 || got.Suggested != "" {
		t.Fatal(got)
	}
	dir := store("Default")
	if got := discover("google-chrome.desktop"); len(got.Options) != 1 || got.Suggested != "chrome:"+dir {
		t.Fatal(got)
	}
	if got := discover("firefox.desktop"); got.Suggested != "" {
		t.Fatal(got)
	}
	store("Profile 1")
	if got := discover("google-chrome.desktop"); len(got.Options) != 3 || got.Suggested != "chrome" {
		t.Fatal(got)
	}
}

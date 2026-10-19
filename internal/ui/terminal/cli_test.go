// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCLICommandsAndInterspersedFlags(t *testing.T) {
	for _, tc := range []struct {
		args                   []string
		command, mode, quality string
		urls                   []string
	}{
		{[]string{"download", "https://example.com/a", "--quality", "720", "https://example.com/b"}, "download", "video", "720", []string{"https://example.com/a", "https://example.com/b"}},
		{[]string{"--lang", "en", "audio", "https://example.com/a", "--audio-format", "opus"}, "audio", "audio", "best", []string{"https://example.com/a"}},
		{[]string{"formats", "--", "https://example.com/a?x=download"}, "formats", "video", "best", []string{"https://example.com/a?x=download"}},
	} {
		o := testOptions("/tmp")
		r, e := parseCLI(tc.args, &o)
		if e != nil || r.command != tc.command || o.Mode != tc.mode || o.Quality != tc.quality || !reflect.DeepEqual([]string(r.urls), tc.urls) {
			t.Fatalf("%v: %+v %+v %v", tc.args, r, o, e)
		}
	}
}
func TestExplicitCommandOverridesSavedMode(t *testing.T) {
	o := testOptions("/tmp")
	o.Mode = "audio"
	_, e := parseCLI([]string{"download", "https://example.com/a"}, &o)
	if e != nil || o.Mode != "video" {
		t.Fatal(o, e)
	}
}
func TestCLIRejectsInvalidUsage(t *testing.T) {
	for _, args := range [][]string{{"-url", "https://example.com/a"}, {"-mode", "audio"}, {"migrate"}, {"https://example.com/a"}, {"download"}, {"audio", "--quality"}, {"doctor", "https://example.com/a"}, {"wat"}, {"download", "--mode", "audio", "https://example.com/a"}, {"--lang", "xx"}, {"config", "--out", "/tmp"}, {"download", "https://example.com/a", "--typo"}} {
		o := testOptions("/tmp")
		if _, e := parseCLI(args, &o); e == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
func TestCLIHelpVersionAndList(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"help", "download"}, {"audio", "--help"}, {"--help"}} {
		o := testOptions("/tmp")
		r, e := parseCLI(args, &o)
		if e != nil || !r.help {
			t.Fatal(args, r, e)
		}
	}
	o := testOptions("/tmp")
	r, e := parseCLI([]string{"--version"}, &o)
	if e != nil || r.command != "version" {
		t.Fatal(r, e)
	}
	o = testOptions("/tmp")
	r, e = parseCLI([]string{"download", "--list", "a file.txt"}, &o)
	if e != nil || r.list != "a file.txt" {
		t.Fatal(r, e)
	}
}

func TestEnglishHelpIncludesVideoProfileFlags(t *testing.T) {
	old := currentLanguage
	useLanguage("en", "en")
	defer useLanguage(old, "ru")
	previous := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer func() { os.Stdout = previous }()
	os.Stdout = w
	printHelp("download")
	w.Close()
	os.Stdout = previous
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "Usage:") || strings.Contains(text, "Использование:") || !strings.Contains(text, "--video-codec") || !strings.Contains(text, "--video-format") {
		t.Fatal(text)
	}
}

func TestDoctorJSONFlagIsScopedToDiagnostics(t *testing.T) {
	o := defaultOptions(t.TempDir())
	request, err := parseCLI([]string{"doctor", "--json"}, &o)
	if err != nil || request.command != "doctor" || !request.json {
		t.Fatal(request, err)
	}
	if _, err = parseCLI([]string{"download", "--json", "https://example.com/video"}, &o); err == nil {
		t.Fatal("diagnostic flag accepted for download")
	}
}

func TestExplicitTUI(t *testing.T) {
	for _, args := range [][]string{{"tui"}, {"--lang", "en", "tui"}, {"tui", "--lang", "ru"}} {
		o := testOptions(t.TempDir())
		r, err := parseCLI(args, &o)
		if err != nil || r.command != "tui" || len(r.urls) != 0 {
			t.Fatalf("%v: %+v %v", args, r, err)
		}
	}
	o := testOptions(t.TempDir())
	if _, err := parseCLI([]string{"tui", "https://example.com"}, &o); err == nil {
		t.Fatal("TUI accepted URL")
	}
}
func TestQueueAddRequiresURL(t *testing.T) {
	if err := queueCLI(nil, nil, []string{"add"}, catalogFlags{}, options{}); err == nil {
		t.Fatal("empty submission accepted")
	}
}

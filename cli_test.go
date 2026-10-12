package main

import (
	"reflect"
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
		{[]string{"-url", "https://example.com/a", "-mode", "audio"}, "", "audio", "best", []string{"https://example.com/a"}},
		{[]string{"formats", "--", "https://example.com/a?x=download"}, "formats", "video", "best", []string{"https://example.com/a?x=download"}},
	} {
		o := defaultOptions("/tmp")
		r, e := parseCLI(tc.args, &o)
		if e != nil || r.command != tc.command || o.Mode != tc.mode || o.Quality != tc.quality || !reflect.DeepEqual([]string(r.urls), tc.urls) {
			t.Fatalf("%v: %+v %+v %v", tc.args, r, o, e)
		}
	}
}
func TestExplicitCommandOverridesSavedMode(t *testing.T) {
	o := defaultOptions("/tmp")
	o.Mode = "audio"
	_, e := parseCLI([]string{"download", "https://example.com/a"}, &o)
	if e != nil || o.Mode != "video" {
		t.Fatal(o, e)
	}
}
func TestCLIRejectsInvalidUsage(t *testing.T) {
	for _, args := range [][]string{{"download"}, {"audio", "--quality"}, {"doctor", "https://example.com/a"}, {"wat"}, {"download", "--mode", "audio", "https://example.com/a"}, {"--lang", "xx"}, {"config", "--out", "/tmp"}, {"download", "https://example.com/a", "--typo"}} {
		o := defaultOptions("/tmp")
		if _, e := parseCLI(args, &o); e == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
func TestCLIHelpVersionAndList(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"help", "download"}, {"audio", "--help"}, {"--help"}} {
		o := defaultOptions("/tmp")
		r, e := parseCLI(args, &o)
		if e != nil || !r.help {
			t.Fatal(args, r, e)
		}
	}
	o := defaultOptions("/tmp")
	r, e := parseCLI([]string{"--version"}, &o)
	if e != nil || r.command != "version" {
		t.Fatal(r, e)
	}
	o = defaultOptions("/tmp")
	r, e = parseCLI([]string{"download", "--list", "a file.txt"}, &o)
	if e != nil || r.list != "a file.txt" {
		t.Fatal(r, e)
	}
}

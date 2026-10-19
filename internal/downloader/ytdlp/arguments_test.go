// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package ytdlp

import (
	"reflect"
	"strings"
	"testing"

	"github.com/bfcamel/CamelClip/internal/core"
)

func validate(o options) error { return core.Validate(o) }
func TestVideoLimitAndURLIsolation(t *testing.T) {
	o := defaults()
	a := arguments(o)
	if !contains(a, "bestvideo[height<=1080]/best[height<=1080],bestaudio/best[height<=1080]") {
		t.Fatal(a)
	}
	if !reflect.DeepEqual(a[len(a)-2:], []string{"--", o.URL}) {
		t.Fatal("URL must remain one positional argument")
	}
	if !contains(a, o.Out) {
		t.Fatal("path split")
	}
	o.Quality = "best"
	if !contains(arguments(o), "bestvideo/best,bestaudio/best") {
		t.Fatal(arguments(o))
	}
}

func TestAudioAndListing(t *testing.T) {
	o := defaults()
	o.Mode = "audio"
	a := arguments(o)
	if contains(a, "--extract-audio") || contains(a, "--remux-video") || contains(a, "--merge-output-format") || !contains(a, "never") {
		t.Fatal(a)
	}
	o.Cookies = "firefox"
	a = formatArguments(o)
	if !contains(a, "--list-formats") || !contains(a, "firefox") || contains(a, "--extract-audio") || contains(a, "--paths") {
		t.Fatal(a)
	}
}

func TestValidation(t *testing.T) {
	o := defaults()
	o.URL = "https://example.com/video"
	if err := validate(o); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"0", "-1", "99999", "1080]/best", "abc"} {
		bad := o
		bad.Quality = q
		if validate(bad) == nil {
			t.Fatalf("accepted %q", q)
		}
	}
	for _, u := range []string{"file:///tmp/video", "--exec=bad", "https://"} {
		bad := o
		bad.URL = u
		if validate(bad) == nil {
			t.Fatalf("accepted %q", u)
		}
	}
	bad := o
	bad.Mode = "other"
	if err := validate(bad); err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatal(err)
	}
}

func TestIntermediateContainerAndAudioArguments(t *testing.T) {
	o := defaults()
	a := arguments(o)
	for i, v := range a {
		if v == "--merge-output-format" || v == "--remux-video" {
			if a[i+1] != "mkv" {
				t.Fatal(a)
			}
		}
	}
	o.Mode = "audio"
	if contains(arguments(o), "--remux-video") {
		t.Fatal("audio must not be remuxed into MP4")
	}
}

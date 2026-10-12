package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestExpandHome(t *testing.T) {
	for input, expected := range map[string]string{"~": "/Users/test", "~/My Videos": "/Users/test/My Videos", "/tmp/video": "/tmp/video", "relative": "relative"} {
		if actual := expandHome(input, "/Users/test"); actual != expected {
			t.Fatalf("%q: got %q", input, actual)
		}
	}
}

func defaults() options {
	return options{URL: "https://example.com/watch?v=123&x=$(touch nope)", Mode: "video", Quality: "1080", Audio: "mp3", Out: "/tmp/My Downloads"}
}

func TestVideoLimitAndURLIsolation(t *testing.T) {
	o := defaults()
	a := arguments(o)
	if !contains(a, "bestvideo[height<=1080]+bestaudio/best[height<=1080]") {
		t.Fatal(a)
	}
	if !reflect.DeepEqual(a[len(a)-2:], []string{"--", o.URL}) {
		t.Fatal("URL must remain one positional argument")
	}
	if !contains(a, o.Out) {
		t.Fatal("path split")
	}
	o.Quality = "best"
	if !contains(arguments(o), "bestvideo+bestaudio/best") {
		t.Fatal(arguments(o))
	}
}

func TestAudioAndListing(t *testing.T) {
	o := defaults()
	o.Mode = "audio"
	a := arguments(o)
	if !contains(a, "--extract-audio") || contains(a, "--remux-video") {
		t.Fatal(a)
	}
	o.Formats = true
	o.Cookies = "firefox"
	a = arguments(o)
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

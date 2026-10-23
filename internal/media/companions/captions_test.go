// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package companions

import (
	"context"
	"github.com/bfcamel/CamelClip/internal/core"
	"os"
	"path/filepath"
	"testing"
)

func TestCaptionsPreserveOverlapAndRepeatedSpeech(t *testing.T) {
	data := []byte("WEBVTT\n\nNOTE ignored\nhello\n\n1\n00:00:01.000 --> 00:00:03.000 align:start\n<v Bob>Hello &amp; goodbye</v>\n\n2\n00:00:02.000 --> 00:00:04.000\nHello &amp; goodbye\n")
	v, e := ParseCaptions(data)
	if e != nil || len(v) != 2 || v[0].Text != "Hello & goodbye" || v[1].StartMS != 2000 {
		t.Fatal(v, e)
	}
	again, e := ParseCaptions(core.RenderWebVTT(v))
	if e != nil || again[0].Text != v[0].Text {
		t.Fatal(again, e)
	}
}
func TestCaptionInputRejections(t *testing.T) {
	for _, v := range []string{"", "WEBVTT\n\n00:00:02.000 --> 00:00:01.000\nbad", "WEBVTT\n\n00:01.000 --> 00:02.000\nx\n\n00:00.000 --> 00:01.000\ny", string([]byte{255})} {
		if _, e := ParseCaptions([]byte(v)); e == nil {
			t.Fatalf("accepted %q", v)
		}
	}
	v, e := ParseCaptions([]byte("1\r\n00:00:01,000 --> 00:00:02,500\r\nText\r\n"))
	if e != nil || v[0].EndMS != 2500 {
		t.Fatal(v, e)
	}
}
func TestFilesIntegrityAndNonOverwrite(t *testing.T) {
	ctx := context.Background()
	f := Files{Root: t.TempDir()}
	scope := core.ClipDigest("scope")
	b, e := f.Put(ctx, scope, "caption_raw", "vtt", "text/vtt", []byte("bytes"))
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "export.vtt")
	if e = f.Export(ctx, b, p); e != nil {
		t.Fatal(e)
	}
	if e = f.Export(ctx, b, p); e == nil {
		t.Fatal("overwrote export")
	}
	path, _ := f.path(b)
	if e = os.WriteFile(path, []byte("wrong"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = f.Read(ctx, b, 100); e == nil {
		t.Fatal("accepted changed blob")
	}
	b.Scope = "../escape"
	if _, e = f.Read(ctx, b, 100); e == nil {
		t.Fatal("accepted path traversal")
	}
}

func TestTrackOriginsAndTranslation(t *testing.T) {
	m := metadata{Subtitles: map[string][]struct{ Ext, Name, URL string }{"en": {{Ext: "vtt", Name: "English", URL: "https://example.test/manual"}}}, Automatic: map[string][]struct{ Ext, Name, URL string }{"en": {{Ext: "vtt", URL: "https://example.test/auto"}}, "ru": {{Ext: "vtt", URL: "https://example.test/auto?tlang=ru"}}, "live_chat": {{Ext: "vtt"}}}}
	tracks := tracks(m)
	if len(tracks) != 3 {
		t.Fatal(tracks)
	}
	seen := map[string]bool{}
	for _, v := range tracks {
		if seen[v.ID] {
			t.Fatal("track identity collision")
		}
		seen[v.ID] = true
		if v.Origin == "manual" && v.Translated != "false" {
			t.Fatal(v)
		}
		if v.Language == "ru" && v.Translated != "true" {
			t.Fatal(v)
		}
	}
}

func TestDifferentRolesDoNotShareEvictionPath(t *testing.T) {
	ctx := context.Background()
	f := Files{Root: t.TempDir()}
	scope := core.ClipDigest("scope")
	raw, e := f.Put(ctx, scope, "caption_raw", "vtt", "text/vtt", []byte("WEBVTT"))
	if e != nil {
		t.Fatal(e)
	}
	export, e := f.Put(ctx, scope, "caption_export", "vtt", "text/vtt", []byte("WEBVTT"))
	if e != nil {
		t.Fatal(e)
	}
	if e = f.Remove(ctx, export); e != nil {
		t.Fatal(e)
	}
	if _, e = f.Read(ctx, raw, 100); e != nil {
		t.Fatal("export eviction removed retained raw bytes", e)
	}
}

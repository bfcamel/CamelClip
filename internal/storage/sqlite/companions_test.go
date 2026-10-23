// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package sqlite

import (
	"context"
	"encoding/json"
	"github.com/bfcamel/CamelClip/internal/core"
	"strings"
	"testing"
	"time"
)

func TestCompanionPersistenceAndRetention(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	r, e := Open(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	c := core.CompanionCatalog{ID: core.NewID(), Scope: core.ClipDigest("scope"), AuthSession: "private-session", Request: core.CompanionRequest{Source: core.ClipSourceRef{URL: "https://example.test/video"}, Browser: "firefox"}, Source: core.ClipSource{Identity: "test:1"}, Created: time.Now()}
	if e = r.SaveCompanionCatalog(ctx, c); e != nil {
		t.Fatal(e)
	}
	data, _ := json.Marshal(c)
	if strings.Contains(string(data), "firefox") || strings.Contains(string(data), "private-session") {
		t.Fatal("private request exposed")
	}
	for _, id := range []string{"raw", "cues"} {
		b := core.CompanionBlob{ID: id, Scope: c.Scope, SHA256: core.ClipDigest(id), Kind: id, Size: 10}
		if e = r.SaveCompanionBlob(ctx, b); e != nil {
			t.Fatal(e)
		}
	}
	snapshot := core.CaptionSnapshot{ID: core.NewID(), CatalogID: c.ID, RawID: "raw", CuesID: "cues", CueCount: 2}
	if e = r.SaveCaption(ctx, snapshot); e != nil {
		t.Fatal(e)
	}
	if e = r.LinkCompanion(ctx, "asset", "raw", "caption_raw", true); e != nil {
		t.Fatal(e)
	}
	if e = r.DeleteCompanionBlob(ctx, "raw"); e == nil {
		t.Fatal("kept blob removed")
	}
	if e = r.Close(); e != nil {
		t.Fatal(e)
	}
	r, e = Open(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	restored, e := r.CompanionCatalog(ctx, c.ID)
	if e != nil || restored.Request.Browser != "firefox" || restored.Scope != c.Scope || restored.AuthSession != c.AuthSession {
		t.Fatal(restored, e)
	}
	captions, e := r.Captions(ctx, c.ID)
	if e != nil || len(captions) != 1 {
		t.Fatal(captions, e)
	}
	if e = r.DeleteCompanionBlob(ctx, "cues"); e != nil {
		t.Fatal(e)
	}
	captions, e = r.Captions(ctx, c.ID)
	if e != nil || len(captions) != 0 {
		t.Fatal("dangling snapshot", captions, e)
	}
	if _, e = r.CompanionBlob(ctx, "raw"); e != nil {
		t.Fatal("kept peer removed", e)
	}
}

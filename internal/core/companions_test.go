// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package core

import (
	"strings"
	"testing"
)

func TestCaptionSelectionAndIntervals(t *testing.T) {
	c := CompanionCatalog{OriginalLanguage: "en", Tracks: []CaptionDescriptor{{ID: "auto", Language: "en", Origin: "automatic"}, {ID: "manual", Language: "en", Origin: "manual"}, {ID: "translated", Language: "ru", Origin: "manual", Translated: "true"}}}
	if v := SelectCaption(c, DefaultCompanionPolicy(), "ru"); v == nil || v.ID != "manual" {
		t.Fatal(v)
	}
	cues := []CaptionCue{{Index: 0, StartMS: 500, EndMS: 2500, Text: "<Hello>"}, {Index: 1, StartMS: 2000, EndMS: 4000, Text: "again"}}
	r, e := CaptionInterval(cues, 0, 1, 5000)
	if e != nil || r.StartMS != 500 || r.EndMS != 4000 {
		t.Fatal(r, e)
	}
	if _, e = CaptionInterval(cues, 1, 0, 5000); e == nil {
		t.Fatal("reversed selection accepted")
	}
	trim, e := TrimCaptionCues(cues, ClipRange{StartMS: 1000, EndMS: 3000})
	if e != nil || len(trim) != 2 || trim[0].StartMS != 0 || trim[1].EndMS != 2000 {
		t.Fatal(trim, e)
	}
	if !strings.Contains(string(RenderWebVTT(trim)), "&lt;Hello&gt;") {
		t.Fatal("unsafe export")
	}
}

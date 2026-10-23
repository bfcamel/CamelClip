package core

import "testing"

func TestClipTimeCanonicalization(t *testing.T) {
	for _, s := range []string{"95.250", "01:35.250", "00:01:35.250"} {
		v, e := ParseClipTime(s)
		if e != nil || v != 95250 {
			t.Fatal(s, v, e)
		}
	}
	for _, s := range []string{"-1", "1e2", "1:90", "01:00.1234", "NaN", "1,2", "1.2:03", "99999999999999999", "169:00:00"} {
		if _, e := ParseClipTime(s); e == nil {
			t.Fatal("accepted", s)
		}
	}
}
func TestClipDomainValidation(t *testing.T) {
	r := ClipRequest{Source: ClipSourceRef{URL: "https://example.test/video"}, OutputDir: "/clips", Segments: []ClipSegment{{Range: ClipRange{0, 1000}}, {Range: ClipRange{500, 2000}}}}
	if _, e := NormalizeClipRequest(r); e != nil {
		t.Fatal("overlap should work", e)
	}
	r.Segments[1].Range = r.Segments[0].Range
	if _, e := NormalizeClipRequest(r); e == nil {
		t.Fatal("duplicate accepted")
	}
	r.Segments = r.Segments[:1]
	r.Accuracy = "fast_copy"
	if _, e := NormalizeClipRequest(r); e == nil {
		t.Fatal("unsupported capability accepted")
	}
}
func TestClipAggregate(t *testing.T) {
	b := ClipBatch{Tasks: []ClipTask{{State: ClipDone}, {State: ClipFailed}}}
	if b.Status() != "partial_failure" {
		t.Fatal(b.Status())
	}
	b.Tasks[1].State = ClipDone
	b.CancelRequested = true
	if b.Status() != "completed" {
		t.Fatal("late cancel changed completed batch")
	}
}
func FuzzClipTime(f *testing.F) {
	for _, s := range []string{"0", "1:20.002", "99999999999", "-10", "00:00:01"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, e := ParseClipTime(s)
		if e == nil && (v < 0 || v > ClipMaxDurationMS) {
			t.Fatal(v)
		}
	})
}

package jsonconfig

import (
	"context"
	"github.com/bfcamel/CamelClip/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestZeroDefaultsAndPathsRoundTripCanonically(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "settings.json")
	s := config.Defaults("relative-output")
	s.MaxDownloads = 0
	s.MaxUploads = 0
	s.UploadParts = 0
	f := File{Path: path, Defaults: s}
	if e := f.Save(ctx, s); e != nil {
		t.Fatal(e)
	}
	got, e := f.Load(ctx)
	if e != nil {
		t.Fatal(e)
	}
	want, e := config.Normalize(s)
	if e != nil || got != want {
		t.Fatal(got, want, e)
	}
	if !filepath.IsAbs(got.Out) {
		t.Fatal(got.Out)
	}
	before, _ := os.ReadFile(path)
	if _, e = f.Load(ctx); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("read changed file")
	}
}

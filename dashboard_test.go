package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLibraryReadsActualMediaOnly(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"older.mp4", "newer.MP3", "notes.txt", "video.mp4.part", "video.f137.mp4", ".hidden.mp4"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	os.Mkdir(filepath.Join(dir, "folder.mp4"), 0700)
	os.Symlink(filepath.Join(dir, "older.mp4"), filepath.Join(dir, "link.mp4"))
	past := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(dir, "older.mp4"), past, past)
	files, err := listMedia(dir)
	if err != nil || len(files) != 2 || files[0].Name != "newer.MP3" || files[1].Size != 4 {
		t.Fatal(files, err)
	}
	os.Remove(filepath.Join(dir, "newer.MP3"))
	files, err = listMedia(dir)
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	files, err = listMedia(filepath.Join(dir, "missing"))
	if err != nil || len(files) != 0 {
		t.Fatal(files, err)
	}
}
func TestDashboardReturnsFromLibraryAndPersistsSettings(t *testing.T) {
	old := currentLanguage
	currentLanguage = "en"
	defer func() { currentLanguage = old }()
	root := t.TempDir()
	out := filepath.Join(root, "downloads")
	os.Mkdir(out, 0700)
	os.WriteFile(filepath.Join(out, "movie.mp4"), []byte("video"), 0600)
	o := defaultOptions(root)
	o.Out = out
	o.Lang = "en"
	config := filepath.Join(root, "config", "settings.json")
	// Library -> back -> settings -> change audio -> back -> exit.
	input := "3\n4\n4\n4\n3\n5\n6\n"
	var output bytes.Buffer
	m := &menu{in: bufio.NewReader(strings.NewReader(input)), out: &output}
	if err := dashboard(context.Background(), m, &o, config, "en", root); err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), "Main menu") < 3 || !strings.Contains(output.String(), "movie.mp4") {
		t.Fatal(output.String())
	}
	saved := defaultOptions(root)
	if err := loadSettings(config, &saved); err != nil || saved.Audio != "opus" || saved.Out != out {
		t.Fatal(saved, err)
	}
}
func TestWritableFolderAndShortName(t *testing.T) {
	root := t.TempDir()
	p, err := writableFolder("~/Media", root)
	if err != nil || p != filepath.Join(root, "Media") {
		t.Fatal(p, err)
	}
	entries, _ := os.ReadDir(p)
	if len(entries) != 0 {
		t.Fatal(entries)
	}
	f := filepath.Join(root, "file")
	os.WriteFile(f, nil, 0600)
	if _, err = writableFolder(f, root); err == nil {
		t.Fatal("file accepted as folder")
	}
	if got := shortName("Пример длинного имени", 8); len([]rune(got)) != 8 {
		t.Fatal(got)
	}
}

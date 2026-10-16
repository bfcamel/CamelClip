package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentQueueRestoresAllStatesAndSettings(t *testing.T) {
	config := filepath.Join(t.TempDir(), "settings.json")
	p, err := openPersistentPool(context.Background(), "", config)
	if err != nil {
		t.Fatal(err)
	}
	states := []string{"queued", "inspecting", "choose", "ready", "downloading", "processing", "verifying", "canceling", "done", "failed", "canceled"}
	for i, state := range states {
		o := defaultOptions(t.TempDir())
		o.URL = "https://example.com/" + state
		o.Codec = "av1"
		o.Container = "webm"
		o.Quality = "720"
		o.Cookies = "firefox:work"
		p.add(o)
		j := p.jobs[i]
		j.state = state
		j.title = "Example " + state
		j.info = mediaInfo{ID: state, Title: j.title, Duration: 23, Formats: []mediaFormat{{Height: 720, VCodec: "av1"}}}
		j.detail = "detail"
	}
	if err = p.saveQueue(); err != nil {
		t.Fatal(err)
	}
	path := p.store.path
	stat, err := os.Stat(path)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal(stat, err)
	}
	if _, err = openPersistentPool(context.Background(), "", config); err == nil {
		t.Fatal("second process bypassed queue lock")
	}
	p.store.close()
	restored, err := openPersistentPool(context.Background(), "", config)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.store.close()
	if len(restored.jobs) != len(states) || restored.active != 0 || restored.nextID != len(states) {
		t.Fatal("queue bookkeeping lost")
	}
	expected := []string{"queued", "queued", "choose", "ready", "ready", "ready", "ready", "canceled", "done", "failed", "canceled"}
	for i, j := range restored.jobs {
		if j.state != expected[i] || j.options.Codec != "av1" || j.options.Container != "webm" || j.options.Quality != "720" || j.options.Cookies != "firefox:work" || j.options.duration != 23 || len(j.info.Formats) != 1 || j.cancel != nil {
			t.Fatalf("bad restore: %#v", j)
		}
	}
	if err = restored.saveQueue(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var disk queueSnapshot
	if err = json.Unmarshal(data, &disk); err != nil || len(disk.Jobs) != len(states) {
		t.Fatal(err)
	}
}
func TestCorruptQueueRemainsUntouched(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "queue.json")
	bad := []byte("{broken")
	os.WriteFile(path, bad, 0600)
	if _, err := openPersistentPool(context.Background(), "", filepath.Join(root, "settings.json")); err == nil {
		t.Fatal("corruption ignored")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(bad) {
		t.Fatal("corrupt queue overwritten")
	}
	// A failed restore releases the lock.
	os.Remove(path)
	p, err := openPersistentPool(context.Background(), "", filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	p.store.close()
}
func TestQueueRejectsUnknownVersionAndDuplicateIDs(t *testing.T) {
	for _, invalid := range []string{`{"version":2,"jobs":[]}`, `{"version":1,"jobs":[{"id":0}]}`} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "queue.json"), []byte(invalid), 0600)
		if _, err := openPersistentPool(context.Background(), "", filepath.Join(dir, "settings.json")); err == nil {
			t.Fatal(invalid)
		}
	}
}
func TestStageEventsKeepWorkerAndCancellation(t *testing.T) {
	p := newJobPool(context.Background(), "")
	o := defaultOptions(t.TempDir())
	o.URL = "https://example.com/v"
	p.add(o)
	j := p.jobs[0]
	j.state = "downloading"
	p.active = 1
	called := false
	j.cancel = func() { called = true }
	for _, stage := range []string{"processing", "verifying"} {
		p.apply(jobEvent{id: j.id, kind: "stage", text: stage})
		p.apply(jobEvent{id: j.id, kind: "progress", text: "99%"})
		if j.state != stage || j.detail != "99%" || p.active != 1 || called {
			t.Fatal("stage released worker or marked completed")
		}
	}
	p.cancelJob(j)
	p.apply(jobEvent{id: j.id, kind: "stage", text: "processing"})
	if j.state != "canceling" {
		t.Fatal("stage resurrected canceled job")
	}
	p.apply(jobEvent{id: j.id, kind: "finished", err: errCanceled})
	if j.state != "canceled" || p.active != 0 {
		t.Fatal(j.state)
	}
}

func TestInterruptedCLIListIsSavedBeforeDownloading(t *testing.T) {
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skip(name + " unavailable")
		}
	}
	root := t.TempDir()
	bin := filepath.Join(root, "yt-dlp")
	script := `#!/bin/sh
case " $* " in
 *--dump-single-json*) printf '%s\n' '{"id":"fixture","title":"Fixture","duration":1,"formats":[{"height":64,"vcodec":"h264","acodec":"aac"}]}';;
 *) sleep 5;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	o := defaultOptions(root)
	o.Out = filepath.Join(root, "downloads")
	config := filepath.Join(root, "settings.json")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := runQueue(ctx, []string{"https://example.com/one", "https://example.com/two"}, o, config, nil)
	if !errors.Is(err, errCanceled) {
		t.Fatal(err)
	}
	restored, err := openPersistentPool(context.Background(), "", config)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.store.close()
	if len(restored.jobs) != 2 || restored.jobs[1].state != "queued" || contains([]string{"done", "canceled", "failed"}, restored.jobs[0].state) {
		t.Fatal("pending CLI list lost after cancellation")
	}
}

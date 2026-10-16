package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

type queueRecord struct {
	ID       int       `json:"id"`
	URL      string    `json:"url"`
	Settings settings  `json:"settings"`
	Title    string    `json:"title"`
	State    string    `json:"state"`
	Detail   string    `json:"detail,omitempty"`
	Info     mediaInfo `json:"media"`
}
type queueSnapshot struct {
	Version int           `json:"version"`
	NextID  int           `json:"next_id"`
	Jobs    []queueRecord `json:"jobs"`
}
type queueStore struct {
	path     string
	lock     *os.File
	snapshot []byte
}

func settingsFor(o options) settings {
	return settings{Language: o.Lang, Mode: o.Mode, Quality: o.Quality, Audio: o.Audio, Out: o.Out, Browser: o.Cookies, Codec: videoCodec(o), Container: videoContainer(o)}
}
func optionsFor(s settings, url string) options {
	return options{URL: url, Lang: s.Language, Mode: s.Mode, Quality: s.Quality, Audio: s.Audio, Out: s.Out, Cookies: s.Browser, Codec: s.Codec, Container: s.Container}
}
func openPersistentPool(ctx context.Context, executable, config string) (*jobPool, error) {
	p := newJobPool(ctx, executable)
	dir := filepath.Dir(config)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "queue.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf(tr("Очередь уже открыта в другом окне MacDown."))
	}
	store := &queueStore{path: filepath.Join(dir, "queue.json"), lock: lock}
	p.store = store
	if err = p.restoreQueue(); err != nil {
		store.close()
		return nil, err
	}
	return p, nil
}
func (s *queueStore) close() { syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN); s.lock.Close() }
func (p *jobPool) restoreQueue() error {
	f, err := os.Open(p.store.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 32*1024*1024+1))
	if err != nil {
		return err
	}
	if len(data) > 32*1024*1024 {
		return fmt.Errorf("queue file exceeds 32 MiB")
	}
	var snapshot queueSnapshot
	if err = json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf(tr("Не удалось прочитать очередь; файл сохранён без изменений: %w"), err)
	}
	if snapshot.Version != 1 || snapshot.NextID < 0 {
		return fmt.Errorf("unsupported queue file version or invalid ID counter")
	}
	jobs := make([]*job, 0, len(snapshot.Jobs))
	seen := map[int]bool{}
	next := snapshot.NextID
	for _, r := range snapshot.Jobs {
		if r.ID <= 0 || seen[r.ID] {
			return fmt.Errorf("duplicate or invalid queue job ID")
		}
		seen[r.ID] = true
		if r.ID > next {
			next = r.ID
		}
		o := optionsFor(r.Settings, r.URL)
		if err = validate(o); err != nil {
			return fmt.Errorf("invalid queue job #%d: %w", r.ID, err)
		}
		state := r.State
		switch state {
		case "inspecting":
			state = "queued"
		case "downloading", "processing", "verifying":
			state = "ready"
		case "canceling":
			state = "canceled"
		case "queued", "ready", "choose", "done", "failed", "canceled":
		default:
			return fmt.Errorf("unknown queue state %q", state)
		}
		o.duration = r.Info.Duration
		detail := r.Detail
		if state == "queued" || state == "ready" {
			detail = ""
		}
		jobs = append(jobs, &job{id: r.ID, options: o, title: r.Title, state: state, detail: detail, info: r.Info})
	}
	p.jobs = jobs
	p.nextID = next
	return nil
}
func (p *jobPool) saveQueue() error {
	if p.store == nil {
		return nil
	}
	snapshot := queueSnapshot{Version: 1, NextID: p.nextID, Jobs: make([]queueRecord, 0, len(p.jobs))}
	for _, j := range p.jobs {
		detail := ""
		if contains([]string{"done", "failed", "canceled"}, j.state) {
			detail = j.detail
		}
		snapshot.Jobs = append(snapshot.Jobs, queueRecord{ID: j.id, URL: j.options.URL, Settings: settingsFor(j.options), Title: j.title, State: j.state, Detail: detail, Info: j.info})
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	if bytes.Equal(data, p.store.snapshot) {
		return nil
	}
	if len(data) > 32*1024*1024 {
		return fmt.Errorf("queue exceeds 32 MiB; remove finished history entries")
	}
	if err = atomicWrite(p.store.path, data); err != nil {
		return fmt.Errorf(tr("Не удалось сохранить очередь: %w"), err)
	}
	p.store.snapshot = data
	return nil
}
func atomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".macdown-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	// Sync the directory entry as well as the file contents before acknowledging a change.
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func storedQueueMenu(ctx context.Context, m *menu, config string) error {
	executable, _ := exec.LookPath("yt-dlp")
	p, err := openPersistentPool(ctx, executable, config)
	if err != nil {
		return err
	}
	defer p.store.close()
	if len(p.jobs) == 0 {
		fmt.Fprintln(m.out, tr("Очередь пуста."))
		return nil
	}
	for _, j := range p.jobs {
		fmt.Fprintf(m.out, "#%d %s: %s\n", j.id, safeText(j.title), jobState(j.state))
	}
	action, err := m.selectItem(ctx, tr("Сохранённая очередь"), []choice{{tr("Продолжить незавершённые"), "resume"}, {tr("Повторить ошибки и отменённые"), "retry"}, {tr("Убрать завершённые записи"), "clear"}, {tr("← Главное меню"), "back"}}, "resume")
	if err != nil {
		return err
	}
	selected := []*job{}
	if action == "clear" {
		kept := []*job{}
		for _, j := range p.jobs {
			if !contains([]string{"done", "failed", "canceled"}, j.state) {
				kept = append(kept, j)
			}
		}
		p.jobs = kept
		return p.saveQueue()
	}
	for _, j := range p.jobs {
		if action == "resume" && !contains([]string{"done", "failed", "canceled"}, j.state) {
			selected = append(selected, j)
		}
		if action == "retry" && contains([]string{"failed", "canceled"}, j.state) {
			j.state = "queued"
			current := j.options
			if loadSettings(config, &current) == nil {
				j.options.Cookies = current.Cookies
			}
			j.detail = ""
			selected = append(selected, j)
		}
	}
	if len(selected) == 0 {
		return nil
	}
	if executable == "" {
		return fmt.Errorf(tr("не найден yt-dlp. Установка: brew install yt-dlp ffmpeg deno"))
	}
	if err = p.saveQueue(); err != nil {
		return err
	}
	return runSequentialJobs(ctx, p, selected, m, config)
}

package main

import (
	"context"
	"net/url"
	"strings"
	"sync"
)

type job struct {
	id                   int
	options              options
	title, state, detail string
	info                 mediaInfo
	cancel               context.CancelFunc
}
type jobEvent struct {
	id         int
	kind, text string
	info       mediaInfo
	err        error
}
type jobPool struct {
	ctx            context.Context
	events         chan jobEvent
	jobs           []*job
	wg             sync.WaitGroup
	active, nextID int
	executable     string
	store          *queueStore
}

func newJobPool(ctx context.Context, path string) *jobPool {
	return &jobPool{ctx: ctx, events: make(chan jobEvent, 256), executable: path}
}
func (p *jobPool) add(o options) bool {
	for _, j := range p.jobs {
		if j.options.URL == o.URL && j.options.Out == o.Out && j.options.Mode == o.Mode && j.state != "done" && j.state != "failed" && j.state != "canceled" && outputVariant(j.options) == outputVariant(o) {
			return false
		}
	}
	p.nextID++
	p.jobs = append(p.jobs, &job{id: p.nextID, options: o, title: o.URL, state: "queued"})
	return true
}
func (p *jobPool) send(ctx context.Context, e jobEvent) {
	select {
	case p.events <- e:
	case <-ctx.Done():
	}
}
func (p *jobPool) schedule() {
	for _, j := range p.jobs {
		if p.active >= 2 || p.ctx.Err() != nil {
			return
		}
		if j.state != "queued" && j.state != "ready" {
			continue
		}
		phase := j.state
		ctx, cancel := context.WithCancel(p.ctx)
		j.cancel = cancel
		p.active++
		if phase == "queued" {
			j.state = "inspecting"
		} else {
			j.state = "downloading"
		}
		id, o := j.id, j.options
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			defer cancel()
			event := jobEvent{id: id, kind: "finished"}
			if phase == "queued" {
				event.kind = "inspected"
				event.info, event.err = inspect(ctx, p.executable, o)
			} else {
				event.err = p.download(ctx, id, o)
			}
			// Completion must be delivered even after a per-job cancellation, so slots can be reused.
			if ctx.Err() != nil {
				event.err = errCanceled
			}
			p.send(p.ctx, event)
		}()
	}
}
func (p *jobPool) download(ctx context.Context, id int, o options) error {
	w := &jobOutput{emit: func(s string) {
		select {
		case p.events <- jobEvent{id: id, kind: "progress", text: s}:
		default:
		}
	}}
	defer w.flush()
	return verifiedDownloadWithObserver(ctx, p.executable, o, w, func(stage, detail string) {
		if detail == "" {
			p.send(ctx, jobEvent{id: id, kind: "stage", text: stage})
		} else {
			p.send(ctx, jobEvent{id: id, kind: "progress", text: detail})
		}
	})
}
func (p *jobPool) apply(e jobEvent) *job {
	var j *job
	for _, v := range p.jobs {
		if v.id == e.id {
			j = v
			break
		}
	}
	if j == nil {
		return nil
	}
	if e.kind == "stage" {
		if j.state == e.text {
			return nil
		}
		if j.state != "canceling" && contains([]string{"downloading", "processing", "verifying"}, e.text) {
			j.state = e.text
			j.detail = ""
		}
		return j
	}
	if e.kind == "progress" {
		if contains([]string{"downloading", "processing", "verifying"}, j.state) {
			j.detail = e.text
		}
		return j
	}
	if j.cancel != nil {
		j.cancel()
		j.cancel = nil
		p.active--
	}
	if j.state == "canceling" || e.err == errCanceled {
		j.state = "canceled"
		j.detail = ""
		return j
	}
	if e.err != nil {
		j.state = "failed"
		j.detail = e.err.Error()
		return j
	}
	if e.kind == "inspected" {
		j.info = e.info
		j.options.duration = e.info.Duration
		j.title = e.info.Title
		// Prevent two aliases of the same video writing the same output concurrently.
		if j.info.ID != "" {
			for _, other := range p.jobs {
				if other.id != j.id && other.info.ID == j.info.ID && other.info.Extractor == j.info.Extractor && (j.info.Extractor != "" || sameSite(other.options.URL, j.options.URL)) && other.options.Out == j.options.Out && other.options.Mode == j.options.Mode && other.state != "done" && other.state != "failed" && other.state != "canceled" && outputVariant(other.options) == outputVariant(j.options) {
					j.state = "canceled"
					j.detail = tr("Этот ролик уже есть в очереди.")
					return j
				}
			}
		}
		if j.options.Mode == "video" {
			j.state = "choose"
		} else {
			j.state = "ready"
		}
	} else {
		j.state = "done"
		j.detail = "100%"
	}
	return j
}
func (p *jobPool) cancelJob(j *job) {
	if j.cancel != nil {
		j.state = "canceling"
		j.cancel()
	} else if j.state != "done" && j.state != "failed" {
		j.state = "canceled"
	}
}
func (p *jobPool) busy() bool {
	for _, j := range p.jobs {
		if j.state != "done" && j.state != "failed" && j.state != "canceled" {
			return true
		}
	}
	return false
}

// Called after the dashboard context is canceled; all subprocesses are reaped before exit.
func (p *jobPool) wait() { p.wg.Wait() }

type jobOutput struct {
	pending string
	tail    tailBuffer
	emit    func(string)
}

func (w *jobOutput) Write(data []byte) (int, error) {
	w.tail.Write(data)
	w.pending += string(data)
	for {
		i := strings.IndexAny(w.pending, "\r\n")
		if i < 0 {
			break
		}
		line := w.pending[:i]
		w.pending = w.pending[i+1:]
		if line != "" {
			w.emit(safeText(line))
		}
	}
	if len(w.pending) > 16384 {
		w.emit(safeText(w.pending[:16384]))
		w.pending = ""
	}
	return len(data), nil
}
func (w *jobOutput) flush() {
	if w.pending != "" {
		w.emit(safeText(w.pending))
		w.pending = ""
	}
}

func sameSite(a, b string) bool {
	x, e := url.Parse(a)
	y, f := url.Parse(b)
	return e == nil && f == nil && x.Hostname() == y.Hostname()
}

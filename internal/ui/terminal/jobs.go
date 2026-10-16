// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"context"
	"errors"

	"github.com/bfcamel/video-downloader/internal/application"
	"github.com/bfcamel/video-downloader/internal/core"
)

// job is a terminal view model, never the authoritative application state.
type job struct {
	id                   int
	options              options
	title, state, detail string
	info                 mediaInfo
	record               application.Job
}
type jobPool struct {
	service application.Queue
	jobs    []*job
	active  int
	lastErr error
	events  <-chan application.Event
}

func openPersistentPool(ctx context.Context, o options, config string, workers int) (*jobPool, error) {
	if o.runtime == nil || o.runtime.OpenQueue == nil {
		return nil, errors.New("queue dependency is not configured")
	}
	q, err := o.runtime.OpenQueue(ctx, config, workers)
	if err != nil {
		return nil, err
	}
	if err = q.SetLimits(settingsFor(o).Limits()); err != nil {
		q.Close()
		return nil, err
	}
	p := &jobPool{service: q, events: q.Events()}
	p.refresh()
	return p, nil
}
func view(j application.Job) *job {
	if j.State == application.Failed {
		j.Detail = explain(j.Detail)
	}
	return &job{id: j.ID, options: optionsFromRequest(j.Request), title: j.Title, state: string(j.State), detail: j.Detail, info: j.Info, record: j}
}
func optionsFromRequest(r core.Request) options {
	return options{URL: r.URL, Mode: r.Mode, Quality: r.Quality, Audio: r.Audio, Out: r.Out, Cookies: r.Cookies, Codec: r.Codec, Container: r.Container, duration: r.Duration}
}
func (p *jobPool) refresh() {
	if p.service == nil {
		return
	}
	items := p.service.Jobs()
	p.jobs = make([]*job, len(items))
	for i, j := range items {
		p.jobs[i] = view(j)
	}
	p.active = application.Active(items)
}
func (p *jobPool) add(o options) bool {
	_, err := p.service.Submit(request(o), true)
	p.lastErr = err
	p.refresh()
	return err == nil
}
func (p *jobPool) schedule() { p.service.Start(); p.refresh() }
func (p *jobPool) saveQueue() error {
	if p.lastErr != nil && !errors.Is(p.lastErr, application.ErrDuplicate) {
		return p.lastErr
	}
	return p.service.Err()
}
func (p *jobPool) apply(e application.Event) *job {
	p.refresh()
	for _, j := range p.jobs {
		if j.id == e.Job.ID {
			return j
		}
	}
	return nil
}
func (p *jobPool) cancelJob(j *job) { p.lastErr = p.service.Cancel(j.id); p.refresh() }
func (p *jobPool) retry(j *job, browser string) {
	p.lastErr = p.service.Retry(j.id, browser)
	p.refresh()
}
func (p *jobPool) choose(id int, quality string) {
	p.lastErr = p.service.Choose(id, quality)
	p.refresh()
}
func (p *jobPool) remove(id int) { p.lastErr = p.service.Remove(id); p.refresh() }
func (p *jobPool) busy() bool {
	for _, j := range p.jobs {
		if !contains([]string{"done", "failed", "canceled"}, j.state) {
			return true
		}
	}
	return false
}
func (p *jobPool) wait() { p.lastErr = p.service.Close() }

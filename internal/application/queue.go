// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

// Package application owns job lifecycle and persistence. It has no UI or SQLite dependency.
package application

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/bfcamel/video-downloader/internal/core"
)

type State string

const (
	Queued      State = "queued"
	Inspecting  State = "inspecting"
	Choosing    State = "choose"
	Ready       State = "ready"
	Downloading State = "downloading"
	Processing  State = "processing"
	Verifying   State = "verifying"
	Done        State = "done"
	Failed      State = "failed"
	Canceling   State = "canceling"
	Canceled    State = "canceled"
)

func (s State) Terminal() bool { return s == Done || s == Failed || s == Canceled }
func (s State) Valid() bool {
	switch s {
	case Queued, Inspecting, Choosing, Ready, Downloading, Processing, Verifying, Done, Failed, Canceling, Canceled:
		return true
	}
	return false
}

type Job struct {
	ID           int
	Request      core.Request
	Title        string
	State        State
	Detail       string
	Info         core.MediaInfo
	OutputPath   string
	NeedsQuality bool
}
type Snapshot struct {
	NextID int
	Jobs   []Job
}

// Repository must atomically save the snapshot and reject concurrent owners.
type Repository interface {
	Load(context.Context) (Snapshot, error)
	Save(context.Context, Snapshot) error
	Close() error
}
type Event struct {
	Job    Job
	Kind   core.EventKind
	Detail string
}

// Queue is the complete boundary used by frontends. Returned values are copies;
// frontends cannot mutate jobs, execute processes, or write persistence directly.
type Queue interface {
	Submit(core.Request, bool) (Job, error)
	Jobs() []Job
	Choose(int, string) error
	Cancel(int) error
	Retry(int, string) error
	Remove(int) error
	Start()
	StartOnly([]int)
	Events() <-chan Event
	Err() error
	Close() error
}

var ErrDuplicate = errors.New("this video is already queued")

type Service struct {
	mu                      sync.Mutex
	ctx                     context.Context
	cancel                  context.CancelFunc
	backend                 core.Downloader
	repository              Repository
	jobs                    []Job
	nextID, active, workers int
	cancels                 map[int]context.CancelFunc
	events                  chan Event
	started, closing        bool
	allowed                 map[int]bool
	failure                 error
	wg                      sync.WaitGroup
	closeOnce               sync.Once
	closeErr                error
}

var _ Queue = (*Service)(nil)

func New(ctx context.Context, backend core.Downloader, repository Repository, workers int) (*Service, error) {
	if backend == nil || repository == nil {
		return nil, errors.New("downloader and repository are required")
	}
	if workers < 1 {
		workers = 1
	}
	snapshot, err := repository.Load(ctx)
	if err != nil {
		return nil, err
	}
	c, cancel := context.WithCancel(ctx)
	s := &Service{ctx: c, cancel: cancel, backend: backend, repository: repository, nextID: snapshot.NextID, workers: workers, cancels: make(map[int]context.CancelFunc), events: make(chan Event, 256)}
	seen := map[int]bool{}
	for _, j := range snapshot.Jobs {
		if j.ID <= 0 || seen[j.ID] || !j.State.Valid() {
			cancel()
			return nil, errors.New("invalid persisted job")
		}
		seen[j.ID] = true
		if err = core.Validate(j.Request); err != nil {
			cancel()
			return nil, fmt.Errorf("invalid job #%d: %w", j.ID, err)
		}
		switch j.State {
		case Inspecting:
			j.State = Queued
		case Downloading, Processing, Verifying:
			j.State = Ready
		case Canceling:
			j.State = Canceled
		}
		j.Request.Duration = j.Info.Duration
		if !j.State.Terminal() {
			j.Detail = ""
		}
		s.jobs = append(s.jobs, clone(j))
		if j.ID > s.nextID {
			s.nextID = j.ID
		}
	}
	return s, nil
}
func clone(j Job) Job { j.Info.Formats = append([]core.MediaFormat(nil), j.Info.Formats...); return j }
func (s *Service) snapshotLocked() Snapshot {
	v := Snapshot{NextID: s.nextID, Jobs: make([]Job, len(s.jobs))}
	for i, j := range s.jobs {
		v.Jobs[i] = clone(j)
		if !j.State.Terminal() {
			v.Jobs[i].Detail = ""
		}
	}
	return v
}
func (s *Service) saveLocked() error {
	if s.failure != nil {
		return s.failure
	}
	if err := s.repository.Save(context.Background(), s.snapshotLocked()); err != nil {
		s.failure = fmt.Errorf("cannot save queue: %w", err)
		s.cancel()
		return s.failure
	}
	return nil
}
func (s *Service) availableLocked() error {
	if s.failure != nil {
		return s.failure
	}
	if s.closing || s.ctx.Err() != nil {
		return core.ErrCanceled
	}
	return nil
}
func (s *Service) findLocked(id int) (int, error) {
	for i := range s.jobs {
		if s.jobs[i].ID == id {
			return i, nil
		}
	}
	return -1, fmt.Errorf("job #%d not found", id)
}
func variant(r core.Request) string {
	if r.Mode == "audio" {
		return r.Audio
	}
	return r.Quality + ":" + core.VideoCodec(r) + ":" + core.VideoContainer(r)
}
func (s *Service) Submit(r core.Request, choose bool) (Job, error) {
	if err := core.Validate(r); err != nil {
		return Job{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.availableLocked(); err != nil {
		return Job{}, err
	}
	for _, j := range s.jobs {
		if !j.State.Terminal() && j.Request.URL == r.URL && j.Request.Out == r.Out && j.Request.Mode == r.Mode && variant(j.Request) == variant(r) {
			return clone(j), ErrDuplicate
		}
	}
	s.nextID++
	j := Job{ID: s.nextID, Request: r, Title: r.URL, State: Queued, NeedsQuality: choose}
	s.jobs = append(s.jobs, j)
	if err := s.saveLocked(); err != nil {
		return Job{}, err
	}
	s.emitLocked(j, core.StageChanged, "")
	s.scheduleLocked()
	return clone(j), nil
}
func (s *Service) Jobs() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Job, len(s.jobs))
	for i, j := range s.jobs {
		result[i] = clone(j)
	}
	return result
}
func (s *Service) Choose(id int, quality string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.availableLocked(); err != nil {
		return err
	}
	i, err := s.findLocked(id)
	if err != nil {
		return err
	}
	if s.jobs[i].State != Choosing {
		return errors.New("job is not waiting for quality")
	}
	candidate := s.jobs[i].Request
	candidate.Quality = quality
	if err = core.Validate(candidate); err != nil {
		return err
	}
	s.jobs[i].Request = candidate
	s.jobs[i].State = Ready
	if err = s.saveLocked(); err != nil {
		return err
	}
	s.emitLocked(s.jobs[i], core.StageChanged, "")
	s.scheduleLocked()
	return nil
}
func (s *Service) Cancel(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.availableLocked(); err != nil {
		return err
	}
	i, err := s.findLocked(id)
	if err != nil {
		return err
	}
	if s.jobs[i].State.Terminal() {
		return nil
	}
	if cancel := s.cancels[id]; cancel != nil {
		s.jobs[i].State = Canceling
		cancel()
	} else {
		s.jobs[i].State = Canceled
	}
	if err = s.saveLocked(); err != nil {
		return err
	}
	s.emitLocked(s.jobs[i], core.StageChanged, "")
	return nil
}
func (s *Service) Retry(id int, browser string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.availableLocked(); err != nil {
		return err
	}
	i, err := s.findLocked(id)
	if err != nil {
		return err
	}
	if s.jobs[i].State != Failed && s.jobs[i].State != Canceled {
		return errors.New("only failed or canceled jobs can be retried")
	}
	if browser != "" && !core.ValidBrowser(browser) {
		return errors.New("invalid browser")
	}
	s.jobs[i].State = Queued
	s.jobs[i].Detail = ""
	s.jobs[i].Info = core.MediaInfo{}
	s.jobs[i].Request.Cookies = browser
	if err = s.saveLocked(); err != nil {
		return err
	}
	s.scheduleLocked()
	return nil
}
func (s *Service) Remove(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.availableLocked(); err != nil {
		return err
	}
	i, err := s.findLocked(id)
	if err != nil {
		return err
	}
	if !s.jobs[i].State.Terminal() {
		return errors.New("cancel active jobs before removing")
	}
	s.jobs = append(s.jobs[:i], s.jobs[i+1:]...)
	return s.saveLocked()
}
func (s *Service) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowed = nil
	s.started = true
	s.scheduleLocked()
}
func (s *Service) Events() <-chan Event { return s.events }
func (s *Service) Err() error           { s.mu.Lock(); defer s.mu.Unlock(); return s.failure }
func (s *Service) emitLocked(j Job, kind core.EventKind, detail string) {
	select {
	case s.events <- Event{Job: clone(j), Kind: kind, Detail: detail}:
	default:
	}
}
func (s *Service) scheduleLocked() {
	if !s.started || s.closing || s.failure != nil || s.ctx.Err() != nil {
		return
	}
	for i := range s.jobs {
		if s.active >= s.workers {
			return
		}
		j := &s.jobs[i]
		if s.allowed != nil && !s.allowed[j.ID] {
			continue
		}
		if j.State != Queued && j.State != Ready {
			continue
		}
		inspecting := j.State == Queued
		if inspecting {
			j.State = Inspecting
		} else {
			j.State = Downloading
		}
		j.Detail = ""
		if err := s.saveLocked(); err != nil {
			return
		}
		ctx, cancel := context.WithCancel(s.ctx)
		s.cancels[j.ID] = cancel
		s.active++
		request := j.Request
		id := j.ID
		s.emitLocked(*j, core.StageChanged, "")
		s.wg.Add(1)
		go s.execute(ctx, id, request, inspecting)
	}
}
func (s *Service) execute(ctx context.Context, id int, r core.Request, inspecting bool) {
	defer s.wg.Done()
	var info core.MediaInfo
	var result core.Result
	var err error
	if inspecting {
		info, err = s.backend.Inspect(ctx, r)
	} else {
		result, err = s.backend.Download(ctx, r, func(e core.Event) { s.observe(id, e) })
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel := s.cancels[id]; cancel != nil {
		cancel()
		delete(s.cancels, id)
	}
	s.active--
	i, found := s.findLocked(id)
	if found != nil || s.closing || s.ctx.Err() != nil {
		return
	}
	j := &s.jobs[i]
	if j.State == Canceling || errors.Is(err, core.ErrCanceled) {
		j.State = Canceled
		j.Detail = ""
	} else if err != nil {
		j.State = Failed
		j.Detail = err.Error()
	} else if inspecting {
		j.Info = info
		j.Title = info.Title
		j.Request.Duration = info.Duration
		if r.Mode == "video" && j.NeedsQuality {
			j.State = Choosing
		} else {
			j.State = Ready
		}
	} else {
		j.State = Done
		j.OutputPath = result.Path
		j.Detail = "100%"
	}
	if s.saveLocked() != nil {
		return
	}
	s.emitLocked(*j, core.StageChanged, "")
	s.scheduleLocked()
}
func (s *Service) observe(id int, e core.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.findLocked(id)
	if err != nil || s.closing || s.failure != nil {
		return
	}
	j := &s.jobs[i]
	if s.allowed != nil && !s.allowed[j.ID] {
		return
	}
	if j.State == Canceling {
		return
	}
	switch e.Kind {
	case core.StageChanged:
		state := State(e.Stage)
		if state != Downloading && state != Processing && state != Verifying {
			return
		}
		if j.State == state {
			return
		}
		j.State = state
		j.Detail = ""
		if s.saveLocked() != nil {
			return
		}
	case core.Progress:
		j.Detail = e.Detail
	case core.Published:
		j.OutputPath = e.Path
	}
	s.emitLocked(*j, e.Kind, e.Detail)
}
func (s *Service) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closing = true
		s.closeErr = s.saveLocked()
		s.cancel()
		s.mu.Unlock()
		s.wg.Wait()
		if err := s.repository.Close(); s.closeErr == nil {
			s.closeErr = err
		}
		close(s.events)
	})
	return s.closeErr
}

// Active reports running work from a snapshot without exposing mutable state.
func Active(jobs []Job) int {
	n := 0
	for _, j := range jobs {
		switch j.State {
		case Inspecting, Downloading, Processing, Verifying, Canceling:
			n++
		}
	}
	return n
}

func (s *Service) StartOnly(ids []int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowed = map[int]bool{}
	for _, id := range ids {
		s.allowed[id] = true
	}
	s.started = true
	s.scheduleLocked()
}

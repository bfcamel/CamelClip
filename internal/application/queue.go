// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

// Package application owns job lifecycle and persistence. It has no UI or SQLite dependency.
package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/bfcamel/CamelClip/internal/core"
)

type State string

const (
	Queued              State = "queued"
	Inspecting          State = "inspecting"
	Choosing            State = "choose"
	Ready               State = "ready"
	WaitingDownload     State = "waiting_download"
	WaitingProcessing   State = "waiting_processing"
	WaitingVerification State = "waiting_verification"
	Downloading         State = "downloading"
	Processing          State = "processing"
	Verifying           State = "verifying"
	Done                State = "done"
	Failed              State = "failed"
	Canceling           State = "canceling"
	Canceled            State = "canceled"
	WaitingDisk         State = "waiting_disk_capacity"
)

func (s State) Terminal() bool { return s == Done || s == Failed || s == Canceled }
func (s State) Valid() bool {
	switch s {
	case WaitingDisk, Queued, Inspecting, Choosing, Ready, WaitingDownload, WaitingProcessing, WaitingVerification, Downloading, Processing, Verifying, Done, Failed, Canceling, Canceled:
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
	Monitoring   Monitoring
	Control      JobControl
	History      []JobEvent
}
type Monitoring struct {
	AttemptID        string
	Executions       int
	AutomaticRetries int
	CreatedAt        time.Time
	StartedAt        time.Time
	UpdatedAt        time.Time
	FinishedAt       time.Time
	Attempts         int
	LastError        string
	ErrorCode        string
	Download         core.Metrics
	Process          core.Metrics
	Verify           core.Metrics
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

// Changes is an atomic incremental transaction. Unmentioned jobs are retained.
type Changes struct {
	NextID  int
	Upserts []Job
	Removed []int
}

// IncrementalRepository is optional; alternative repositories may use snapshots.
type IncrementalRepository interface {
	ApplyChanges(context.Context, Changes) error
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
	JobsPage(int, int) JobPage
	Choose(int, string) error
	Cancel(int) error
	Retry(int, string) error
	RetryWithOptions(int, RetryOptions) error
	Remove(int) error
	SetLimits(core.Limits) error
	Statistics() Statistics
	SetPolicy(QueuePolicy) error
	SetPriority(int, int) error
	Reorder(int, string) error
	PausePending(int, bool) error
	BulkAction([]int, string) []ActionResult
	BulkActionWithOptions([]int, string, RetryOptions) []ActionResult
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
	metadataActive          int
	resources               *resources
	samples                 map[int][]rateSample
	sessionBytes            int64
	cancels                 map[int]context.CancelFunc
	events                  chan Event
	started, closing        bool
	allowed                 map[int]bool
	failure                 error
	wg                      sync.WaitGroup
	closeOnce               sync.Once
	closeErr                error
	policy                  QueuePolicy
	staged                  map[string]int64
	dirty                   map[int]bool
	removed                 map[int]bool
	index                   map[int]int
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
	s := &Service{index: map[int]int{}, dirty: map[int]bool{}, removed: map[int]bool{}, ctx: c, cancel: cancel, backend: backend, repository: repository, nextID: snapshot.NextID, workers: workers, resources: newResources(), staged: map[string]int64{}, samples: map[int][]rateSample{}, cancels: make(map[int]context.CancelFunc), events: make(chan Event, 256)}
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
		case WaitingDisk, WaitingDownload, WaitingProcessing, WaitingVerification, Downloading, Processing, Verifying:
			j.State = Ready
		case Canceling:
			j.State = Canceled
		}
		j.Request.Duration = j.Info.Duration
		j.Monitoring.Download.SpeedBPS = nil
		j.Monitoring.Download.ETASeconds = nil
		if !j.State.Terminal() {
			j.Detail = ""
		}
		if j.Control.Position == 0 {
			j.Control.Position = int64(j.ID)
		}
		s.index[j.ID] = len(s.jobs)
		s.jobs = append(s.jobs, clone(j))
		s.dirty[j.ID] = true
		if j.ID > s.nextID {
			s.nextID = j.ID
		}
	}
	s.syncOrderLocked()
	s.wg.Add(1)
	go s.maintain()
	return s, nil
}
func clone(j Job) Job {
	j.History = append([]JobEvent(nil), j.History...)
	j.Info.Formats = append([]core.MediaFormat(nil), j.Info.Formats...)
	j.Monitoring.Download = core.CloneMetrics(j.Monitoring.Download)
	j.Monitoring.Process = core.CloneMetrics(j.Monitoring.Process)
	j.Monitoring.Verify = core.CloneMetrics(j.Monitoring.Verify)
	return j
}
func persistedJob(j *Job) Job {
	message := ""
	if j.State == Failed {
		message = j.Detail
	}
	if len(j.History) == 0 || j.History[len(j.History)-1].State != j.State {
		j.History = append(j.History, JobEvent{At: time.Now(), State: j.State, Message: message})
		if len(j.History) > 100 {
			j.History = j.History[len(j.History)-100:]
		}
	}
	v := clone(*j)
	if !j.State.Terminal() {
		v.Detail = ""
	}
	v.Monitoring.Download.SpeedBPS = nil
	v.Monitoring.Download.ETASeconds = nil
	return v
}
func (s *Service) snapshotLocked() Snapshot {
	v := Snapshot{NextID: s.nextID, Jobs: make([]Job, len(s.jobs))}
	for i := range s.jobs {
		v.Jobs[i] = persistedJob(&s.jobs[i])
	}
	return v
}
func (s *Service) saveJobLocked(id int) error { s.dirty[id] = true; return s.saveLocked() }

func (s *Service) saveLocked() error {
	if s.failure != nil {
		return s.failure
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var err error
	if incremental, ok := s.repository.(IncrementalRepository); ok {
		changes := Changes{NextID: s.nextID}
		ids := make([]int, 0, len(s.dirty))
		for id := range s.dirty {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		for _, id := range ids {
			if i, ok := s.index[id]; ok {
				changes.Upserts = append(changes.Upserts, persistedJob(&s.jobs[i]))
			}
		}
		for id := range s.removed {
			changes.Removed = append(changes.Removed, id)
		}
		err = incremental.ApplyChanges(ctx, changes)
	} else {
		err = s.repository.Save(ctx, s.snapshotLocked())
	}
	if err != nil {
		s.failure = fmt.Errorf("cannot save queue: %w", err)
		s.cancel()
		return s.failure
	}
	clear(s.dirty)
	clear(s.removed)
	return nil
}
func (s *Service) availableLocked() error {
	if s.failure != nil {
		return s.failure
	}
	if s.closing {
		return ErrClosed
	}
	if s.ctx.Err() != nil {
		return core.ErrCanceled
	}
	return nil
}
func (s *Service) findLocked(id int) (int, error) {
	if i, ok := s.index[id]; ok {
		return i, nil
	}
	return -1, &core.Failure{Code: core.CodeNotFound, Message: fmt.Sprintf("job #%d not found", id)}
}

func variant(r core.Request) string {
	if r.Mode == "audio" {
		return r.Audio
	}
	return r.Quality + ":" + core.VideoCodec(r) + ":" + core.VideoContainer(r)
}
func (s *Service) Submit(r core.Request, choose bool) (Job, error) {
	var err error
	r, err = core.NormalizeRequest(r)
	if err != nil {
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
	j := Job{ID: s.nextID, Request: r, Title: r.URL, State: Queued, NeedsQuality: choose, Control: JobControl{Position: time.Now().UnixNano()}, Monitoring: Monitoring{CreatedAt: time.Now(), UpdatedAt: time.Now()}}
	s.index[j.ID] = len(s.jobs)
	s.jobs = append(s.jobs, j)
	s.syncOrderLocked()
	if err := s.saveJobLocked(j.ID); err != nil {
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

// JobsPage provides bounded immutable reads for additional clients.
type JobPage struct {
	Items []Job
	Total int
}

func (s *Service) JobsPage(offset, limit int) JobPage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	offset = min(max(offset, 0), len(s.jobs))
	page := JobPage{Total: len(s.jobs), Items: make([]Job, 0, min(limit, len(s.jobs)-offset))}
	for _, j := range s.jobs[offset:min(len(s.jobs), offset+limit)] {
		page.Items = append(page.Items, clone(j))
	}
	return page
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
	s.jobs[i].Monitoring.UpdatedAt = time.Now()
	if s.jobs[i].State.Terminal() {
		s.jobs[i].Monitoring.FinishedAt = time.Now()
	}
	if err = s.saveJobLocked(id); err != nil {
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
	s.jobs[i].Monitoring.UpdatedAt = time.Now()
	if s.jobs[i].State.Terminal() {
		s.jobs[i].Monitoring.FinishedAt = time.Now()
	}
	if err = s.saveJobLocked(id); err != nil {
		return err
	}
	s.emitLocked(s.jobs[i], core.StageChanged, "")
	return nil
}

// Retry preserves the original browser when browser is empty (legacy API).
func (s *Service) Retry(id int, browser string) error {
	options := RetryOptions{}
	if browser != "" {
		options.Browser = &browser
	}
	return s.RetryWithOptions(id, options)
}

// A nil Browser preserves authentication; a pointer to an empty string clears it.
type RetryOptions struct{ Browser *string }

func (s *Service) RetryWithOptions(id int, options RetryOptions) error {
	browser := ""
	if options.Browser != nil {
		browser = *options.Browser
	}

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
	s.jobs[i].OutputPath = ""
	s.jobs[i].Request.Duration = 0
	s.jobs[i].Request.ExpectedParts = 0
	s.jobs[i].Monitoring.AttemptID = ""
	s.jobs[i].Monitoring.AutomaticRetries = 0
	s.jobs[i].Monitoring.LastError = ""
	s.jobs[i].Monitoring.ErrorCode = ""
	s.jobs[i].Monitoring.UpdatedAt = time.Now()
	s.jobs[i].Control.NextAttempt = time.Time{}
	s.jobs[i].Monitoring.Attempts++
	s.jobs[i].Monitoring.Download = core.Metrics{}
	s.jobs[i].Monitoring.Process = core.Metrics{}
	s.jobs[i].Monitoring.Verify = core.Metrics{}
	s.jobs[i].Monitoring.StartedAt = time.Time{}
	s.jobs[i].Monitoring.FinishedAt = time.Time{}
	delete(s.samples, id)
	if options.Browser != nil {
		s.jobs[i].Request.Cookies = browser
	}
	if err = s.saveJobLocked(id); err != nil {
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
	delete(s.samples, id)
	delete(s.dirty, id)
	s.removed[id] = true
	s.jobs = append(s.jobs[:i], s.jobs[i+1:]...)
	delete(s.index, id)
	for n := i; n < len(s.jobs); n++ {
		s.index[s.jobs[n].ID] = n
	}
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
	for _, i := range s.orderedLocked() {
		j := &s.jobs[i]
		if j.Control.Paused || time.Now().Before(j.Control.NextAttempt) {
			continue
		}
		if s.allowed != nil && !s.allowed[j.ID] {
			continue
		}
		if j.State != Queued && j.State != Ready && j.State != WaitingDisk {
			continue
		}
		inspecting := j.State == Queued
		if !inspecting && s.diskBlockedLocked(j.Request.Out) {
			if j.State != WaitingDisk {
				j.State = WaitingDisk
				j.Detail = "Waiting for disk/staging capacity"
				if s.saveJobLocked(j.ID) != nil {
					return
				}
			}
			continue
		}
		if inspecting && s.metadataActive >= s.workers {
			continue
		}
		if inspecting {
			if j.Monitoring.AttemptID == "" {
				j.Monitoring.StartedAt = time.Time{}
				j.Monitoring.FinishedAt = time.Time{}
				j.Monitoring.LastError = ""
				j.Monitoring.ErrorCode = ""
				j.Monitoring.AttemptID = core.NewID()
				j.Monitoring.Executions++
			}
			j.State = Inspecting
			s.metadataActive++
		} else {
			j.State = WaitingDownload
		}
		j.Monitoring.UpdatedAt = time.Now()
		if j.Monitoring.StartedAt.IsZero() {
			j.Monitoring.StartedAt = time.Now()
		}
		j.Detail = ""
		if err := s.saveJobLocked(j.ID); err != nil {
			if inspecting {
				s.metadataActive--
			}
			return
		}
		ctx, cancel := context.WithCancel(s.ctx)
		s.cancels[j.ID] = cancel
		s.active++
		request := j.Request
		id := j.ID
		s.emitLocked(*j, core.StageChanged, "")
		s.wg.Add(1)
		var prepared *permit
		if !inspecting {
			prepared = s.resources.prepare("downloading", id)
		}
		go s.execute(ctx, id, request, inspecting, prepared)
	}
}
func (s *Service) execute(ctx context.Context, id int, r core.Request, inspecting bool, prepared *permit) {
	defer s.wg.Done()
	var info core.MediaInfo
	var result core.Result
	var err error
	var resource *lease
	if !inspecting {
		resource = &lease{initial: prepared, id: id, manager: s.resources, phase: func(stage core.Stage, waiting bool) { s.resourcePhase(id, stage, waiting) }}
		ctx = core.WithResources(ctx, resource)
		err = resource.Enter(ctx, core.Downloading)
	}
	func() {
		defer func() {
			if value := recover(); value != nil {
				err = core.ContractError(fmt.Sprintf("backend panic: %v", value))
			}
		}()
		if err != nil { // canceled while waiting for a resource
		} else if inspecting {
			info, err = s.backend.Inspect(ctx, r)
			if err == nil {
				err = core.ValidateMediaInfo(info)
			}
		} else {
			r.RateLimitBPS = core.DownloadRate(ctx, r.RateLimitBPS)

			result, err = downloadWithContract(ctx, s.backend, r, func(e core.Event) {
				if e.Kind == core.StageChanged {
					if gateErr := resource.Enter(ctx, e.Stage); gateErr != nil {
						return
					}
				}
				s.observe(id, e)
			})

		}
	}()
	if resource != nil {
		resource.close()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if inspecting {
		s.metadataActive--
	}
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
	j.Monitoring.UpdatedAt = time.Now()
	j.Monitoring.Download.SpeedBPS = nil
	j.Monitoring.Download.ETASeconds = nil
	if j.State == Canceling || errors.Is(err, core.ErrCanceled) || errors.Is(err, context.Canceled) {
		j.State = Canceled
		j.Detail = ""
	} else if err != nil {
		j.State = Failed
		j.Detail = err.Error()
		j.Monitoring.LastError = err.Error()
		j.Monitoring.ErrorCode = string(core.CodeOf(err))
		if core.CodeOf(err) == core.CodeNotReady {
			j.State = Queued
			j.Control.NextAttempt = time.Now().Add(15 * time.Minute)
			j.Detail = "Waiting for source availability"
			j.Monitoring.AttemptID = ""
		} else if retryable(err) && j.Monitoring.AutomaticRetries < s.policy.RetryAttempts {
			j.Monitoring.Attempts++
			j.Monitoring.AutomaticRetries++
			j.Monitoring.AttemptID = ""
			j.OutputPath = ""
			j.Control.NextAttempt = time.Now().Add(retryDelay(j.Monitoring.Attempts))
			j.State = Queued
			j.Detail = "Retry scheduled"
		}
	} else if inspecting {
		j.Info = info
		j.Title = info.Title
		j.Request.Duration = info.Duration
		j.Request.ExpectedParts = 1
		videoOnly, audioOnly := false, false
		for _, format := range info.Formats {
			if format.VCodec != "" && format.VCodec != "none" && format.ACodec == "none" {
				videoOnly = true
			}
			if format.ACodec != "" && format.ACodec != "none" && format.VCodec == "none" {
				audioOnly = true
			}
		}
		if r.Mode == "video" && videoOnly && audioOnly {
			j.Request.ExpectedParts = 2
		}
		if r.Mode == "video" && j.NeedsQuality {
			j.State = Choosing
		} else {
			j.State = Ready
		}
	} else {
		j.State = Done
		j.Monitoring.LastError = ""
		j.Monitoring.ErrorCode = ""
		n := 100.0
		j.Monitoring.Verify.Percent = &n
		j.OutputPath = result.Path
		j.Detail = "100%"
	}
	if j.State.Terminal() {
		j.Monitoring.FinishedAt = time.Now()
	}
	if s.saveJobLocked(id) != nil {
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
		j.Monitoring.UpdatedAt = time.Now()
		state := State(e.Stage)
		if state != Downloading && state != Processing && state != Verifying {
			return
		}
		if j.State == state {
			return
		}
		j.State = state
		j.Detail = ""
		if s.saveJobLocked(id) != nil {
			return
		}
	case core.Progress:
		j.Detail = e.Detail
		s.measureLocked(j, e)
	case core.Published:
		j.OutputPath = e.Path
		if s.saveJobLocked(id) != nil {
			return
		}
	}
	s.emitLocked(*j, e.Kind, e.Detail)
}
func (s *Service) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closing = true
		for _, j := range s.jobs {
			if !j.State.Terminal() {
				s.dirty[j.ID] = true
			}
		}
		s.closeErr = s.saveLocked()
		s.cancel()
		s.mu.Unlock()
		s.wg.Wait()
		s.closeErr = errors.Join(s.closeErr, s.repository.Close())
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

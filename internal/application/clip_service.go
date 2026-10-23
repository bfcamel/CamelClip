// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"errors"
	"github.com/bfcamel/CamelClip/internal/core"
	"regexp"
	"sync"
	"time"
)

type ClipService struct {
	mu           sync.Mutex
	repo         ClipRepository
	assets       AssetRepository
	profiles     ProfileRepository
	backend      core.ClipBackend
	files        ClipFiles
	resources    *resources
	cloud        *CloudService
	life         *lifetime
	wg           sync.WaitGroup
	once         sync.Once
	started      bool
	closed       bool
	only         string
	active       string
	activeTask   string
	activeCancel context.CancelFunc
	failure      error
}

func NewClips(ctx context.Context, repo ClipRepository, assets AssetRepository, profiles ProfileRepository, backend core.ClipBackend, files ClipFiles, queue *Service, cloud *CloudService) (*ClipService, error) {
	if repo == nil || assets == nil || backend == nil || files == nil || queue == nil {
		return nil, errors.New("clip dependencies are required")
	}
	return &ClipService{repo: repo, assets: assets, profiles: profiles, backend: backend, files: files, resources: queue.resources, cloud: cloud, life: newLifetime(ctx)}, nil
}
func (s *ClipService) Err() error { s.mu.Lock(); defer s.mu.Unlock(); return s.failure }

var clipKey = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

func (s *ClipService) Admit(ctx context.Context, planID string, revision int64, digest, key string) (core.ClipBatch, error) {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return core.ClipBatch{}, e
	}
	defer done()
	if !clipKey.MatchString(key) {
		return core.ClipBatch{}, &core.Failure{Code: core.CodeValidation, Message: "idempotency key must be 1..128 ASCII letters, digits, _, -, . or :"}
	}
	p, e := s.repo.ClipPlan(ctx, planID)
	if e != nil {
		return core.ClipBatch{}, e
	}
	if p.Revision != revision || p.Digest != digest {
		return core.ClipBatch{}, &core.Failure{Code: core.CodeConflict, Message: "plan revision or digest changed"}
	}
	b := core.ClipBatch{ID: core.NewID(), Revision: 1, ControlRevision: 1, Key: key, Plan: p, SourceState: core.ClipQueued, Created: time.Now().UTC(), Updated: time.Now().UTC(), Tasks: []core.ClipTask{}}
	for i, seg := range p.Request.Segments {
		b.Tasks = append(b.Tasks, core.ClipTask{ID: core.NewID(), Ordinal: i, Segment: seg, State: core.ClipWaitingSource})
	}
	return s.repo.AdmitClip(ctx, b)
}
func (s *ClipService) Get(ctx context.Context, id string) (core.ClipBatch, error) {
	ctx, d, e := s.life.begin(ctx)
	if e != nil {
		return core.ClipBatch{}, e
	}
	defer d()
	return s.repo.ClipBatch(ctx, id)
}
func (s *ClipService) List(ctx context.Context, limit int, cursor string) (core.ClipBatchPage, error) {
	ctx, d, e := s.life.begin(ctx)
	if e != nil {
		return core.ClipBatchPage{}, e
	}
	defer d()
	return s.repo.ClipBatches(ctx, limit, cursor)
}
func (s *ClipService) Provenance(ctx context.Context, id string) ([]ClipProvenance, error) {
	ctx, d, e := s.life.begin(ctx)
	if e != nil {
		return nil, e
	}
	defer d()
	return s.repo.ClipProvenance(ctx, id)
}
func (s *ClipService) Deliveries(ctx context.Context) ([]ClipDelivery, error) {
	ctx, d, e := s.life.begin(ctx)
	if e != nil {
		return nil, e
	}
	defer d()
	return s.repo.ClipDeliveries(ctx)
}
func (s *ClipService) save(ctx context.Context, b *core.ClipBatch) error {
	expected := b.Revision
	b.Revision++
	b.Updated = time.Now().UTC()
	return s.repo.SaveClip(ctx, *b, expected)
}
func (s *ClipService) Control(ctx context.Context, id string, expected int64, action, taskID string) (core.ClipBatch, error) {
	ctx, d, e := s.life.begin(ctx)
	if e != nil {
		return core.ClipBatch{}, e
	}
	defer d()
	s.mu.Lock()
	defer s.mu.Unlock()
	b, e := s.repo.ClipBatch(ctx, id)
	if e != nil {
		return b, e
	}
	if b.ControlRevision != expected {
		return b, &core.Failure{Code: core.CodeConflict, Message: "clip control revision changed; refresh and retry"}
	}
	if action != "pause" && action != "resume" && action != "cancel" && action != "retry" && action != "cleanup" {
		return b, errors.New("unknown clip control")
	}
	found := taskID == ""
	for _, t := range b.Tasks {
		if t.ID == taskID {
			found = true
			if action == "retry" && t.State == core.ClipDone {
				return b, errors.New("completed clips cannot be retried")
			}
		}
	}
	if !found {
		return b, errors.New("clip task not found")
	}
	if taskID != "" && (action == "pause" || action == "resume") {
		return b, errors.New("pause/resume applies to the whole batch")
	}
	if action == "cleanup" {
		if !b.Settled() || s.active == id {
			return b, errors.New("source cache can be cleared only after the batch has stopped")
		}
		if e = s.files.Cleanup(ctx, id); e != nil {
			return b, e
		}
		for _, t := range b.Tasks {
			if t.Candidate != "" {
				_ = s.files.CleanupCandidate(ctx, t.Candidate)
			}
		}
		b.SourcePath = ""
		b.ControlRevision++
		e = s.save(ctx, &b)
		return b, e
	}
	switch action {
	case "pause":
		if !b.Settled() {
			b.Paused = true
		}
	case "resume":
		b.Paused = false
	case "cancel":
		if taskID == "" && !b.Settled() {
			b.CancelRequested = true
			b.Paused = false
		}
		for i := range b.Tasks {
			t := &b.Tasks[i]
			if (taskID == "" || t.ID == taskID) && !t.State.Terminal() && t.State != core.ClipPublishing {
				t.State = core.ClipCanceled
			}
		}
	case "retry":
		if s.active == id {
			return b, errors.New("wait for the active stage to stop before retrying")
		}
		b.CancelRequested = false
		b.Paused = false
		b.SourceError = ""
		for i := range b.Tasks {
			t := &b.Tasks[i]
			if (taskID == "" || t.ID == taskID) && (t.State == core.ClipFailed || t.State == core.ClipCanceled) {
				t.State = core.ClipWaitingSource
				t.Error = ""
				t.ErrorCode = ""
				t.Metrics = core.Metrics{}
			}
		}
	}
	b.ControlRevision++
	e = s.save(ctx, &b)
	if e == nil && action == "cancel" && s.active == id && (taskID == "" || taskID == s.activeTask) && s.activeCancel != nil {
		s.activeCancel()
	}
	return b, e
}

// Start enables durable workers. A nonempty ID restricts one-shot CLI execution.
func (s *ClipService) Start(only string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.closed || s.life.ctx.Err() != nil {
		return
	}
	s.started = true
	s.only = only
	s.wg.Add(1)
	go s.loop()
}
func (s *ClipService) Close() error {
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.life.stop()
		s.mu.Unlock()
		s.wg.Wait()
		s.life.wait()
	})
	return s.Err()
}
func (s *ClipService) loop() {
	defer s.wg.Done()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		s.mu.Lock()
		b, e := s.repo.NextClip(s.life.ctx, s.only)
		s.mu.Unlock()
		if e == nil {
			s.execute(b.ID)
		}
		s.deliver()
		select {
		case <-s.life.ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (s *ClipService) mutate(id string, fn func(*core.ClipBatch) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.life.ctx.Err() != nil {
		return s.life.ctx.Err()
	}
	b, e := s.repo.ClipBatch(s.life.ctx, id)
	if e != nil {
		return e
	}
	if e = fn(&b); e != nil {
		return e
	}
	e = s.save(s.life.ctx, &b)
	if e != nil {
		s.failure = e
	}
	return e
}
func (s *ClipService) progress(id, taskID string, attempt int) func(core.ClipProgress) {
	last := time.Time{}
	var phase core.ClipState
	return func(p core.ClipProgress) {
		if p.State == phase && time.Since(last) < time.Second {
			return
		}
		last = time.Now()
		phase = p.State
		_ = s.mutate(id, func(b *core.ClipBatch) error {
			if taskID == "" {
				b.SourceState = p.State
				b.SourceMetrics = core.CloneMetrics(p.Metrics)
				return nil
			}
			for i := range b.Tasks {
				t := &b.Tasks[i]
				if t.ID == taskID && t.Attempt == attempt && !t.State.Terminal() && t.State != core.ClipPublishing {
					t.State = p.State
					t.Metrics = core.CloneMetrics(p.Metrics)
				}
			}
			return nil
		})
	}
}
func (s *ClipService) fail(id, taskID string, err error) {
	if s.life.ctx.Err() != nil {
		return
	}
	_ = s.mutate(id, func(b *core.ClipBatch) error {
		for i := range b.Tasks {
			t := &b.Tasks[i]
			if (taskID == "" || t.ID == taskID) && !t.State.Terminal() && t.State != core.ClipPublishing {
				t.State = core.ClipFailed
				t.Error = err.Error()
				t.ErrorCode = core.CodeOf(err)
			}
		}
		if taskID == "" {
			b.SourceError = err.Error()
			b.SourceState = core.ClipFailed
		}
		return nil
	})
}

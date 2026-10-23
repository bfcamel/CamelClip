// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"errors"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"sync"
	"time"
)

func (s *ClipService) execute(id string) {
	ctx, cancel := context.WithCancel(s.life.ctx)
	s.mu.Lock()
	s.active = id
	s.activeCancel = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		s.active = ""
		s.activeTask = ""
		s.activeCancel = nil
		s.mu.Unlock()
	}()
	ctx, release, e := s.files.Acquire(ctx, id)
	if e != nil {
		return
	}
	defer release()
	b, e := s.repo.ClipBatch(ctx, id)
	if e != nil {
		return
	}
	// Reconcile journaled publication before cancel/retry or source work.
	for i, t := range b.Tasks {
		if t.State == core.ClipPublishing {
			if e = s.publish(id, i); e != nil {
				s.mu.Lock()
				s.failure = e
				s.mu.Unlock()
				return
			}
		}
	}
	b, e = s.repo.ClipBatch(ctx, id)
	if e != nil || b.Paused || b.Settled() {
		return
	}
	work, _, _, e := s.files.Paths(ctx, id, "source", b.Plan.Request.OutputDir, "", "")
	if e != nil {
		s.fail(id, "", e)
		return
	}
	if b.SourcePath == "" || s.files.Validate(ctx, b.SourcePath, b.Prepared.SHA256) != nil {
		if b.SourcePath != "" && b.Prepared.SHA256 != "" {
			s.fail(id, "", &core.Failure{Code: core.CodeSourceChanged, Message: "prepared source missing or changed; create a new reviewed plan"})
			return
		}
		if e = s.mutate(id, func(b *core.ClipBatch) error { b.SourceState = core.ClipDownloading; return nil }); e != nil {
			return
		}
		local, e := s.local(ctx, b.Plan.Request)
		if e != nil {
			s.fail(id, "", e)
			return
		}
		if local == "" {
			refreshed, err := s.Inspect(ctx, b.Plan.Request)
			if err != nil {
				s.fail(id, "", err)
				return
			}
			if refreshed.Identity != b.Plan.Source.Identity {
				s.fail(id, "", &core.Failure{Code: core.CodeSourceChanged, Message: "source changed since review"})
				return
			}
		}
		pool := "downloading"
		if local != "" {
			pool = "processing"
		}
		p, e := s.resources.acquireNamed(ctx, pool, "clip-source:"+id)
		if e != nil {
			return
		}
		resource := &clipResource{manager: s.resources, permit: p, owner: "clip-source:" + id}
		child := core.WithResources(ctx, resource)
		path, source, e := safeClipPrepare(s.backend, child, b.Plan, local, work, s.progress(id, "", 0))
		resource.close()
		if e != nil {
			s.fail(id, "", e)
			return
		}
		if source.Identity != b.Plan.Source.Identity || b.Plan.Source.SHA256 != "" && source.SHA256 != b.Plan.Source.SHA256 || b.Prepared.SHA256 != "" && source.SHA256 != b.Prepared.SHA256 {
			e = &core.Failure{Code: core.CodeSourceChanged, Message: "source changed since review"}
		} else {
			e = core.ValidateClipSource(source, b.Plan.Request)
		}
		if e != nil {
			s.fail(id, "", e)
			return
		}
		if e = s.mutate(id, func(b *core.ClipBatch) error {
			b.SourcePath = path
			b.Prepared = source
			b.SourceState = core.ClipDone
			return nil
		}); e != nil {
			return
		}
	}
	for index := range b.Tasks {
		b, e = s.repo.ClipBatch(ctx, id)
		if e != nil || b.Paused || b.CancelRequested {
			return
		}
		t := b.Tasks[index]
		if t.State.Terminal() {
			continue
		}
		s.mu.Lock()
		s.activeTask = t.ID
		s.mu.Unlock()
		_, candidate, final, e := s.files.Paths(ctx, id, t.ID, b.Plan.Request.OutputDir, clipExtension(b.Plan.Request), clipOutputTitle(b, t))
		if e != nil {
			s.fail(id, t.ID, e)
			continue
		}
		e = s.mutate(id, func(b *core.ClipBatch) error {
			t := &b.Tasks[index]
			if t.State.Terminal() {
				return context.Canceled
			}
			t.Attempt++
			t.State = core.ClipWaitingProcessing
			t.Candidate = candidate
			t.FinalPath = final
			return nil
		})
		if e != nil {
			continue
		}
		b, e = s.repo.ClipBatch(ctx, id)
		if e != nil {
			return
		}
		t = b.Tasks[index]
		p, e := s.resources.acquireNamed(ctx, "processing", "clip-task:"+t.ID)
		if e != nil {
			return
		}
		// Observe a pause received while waiting for the shared permit.
		latest, e := s.repo.ClipBatch(ctx, id)
		if e != nil || latest.Paused || latest.Tasks[index].State.Terminal() {
			s.resources.release(p)
			return
		}
		evidence, e := safeClipRender(s.backend, ctx, b.SourcePath, candidate, b.Plan, t.Segment, s.progress(id, t.ID, t.Attempt))
		s.resources.release(p)
		if e != nil {
			s.fail(id, t.ID, e)
			if ctx.Err() != nil {
				return
			}
			continue
		}
		if e = s.mutate(id, func(b *core.ClipBatch) error {
			t := &b.Tasks[index]
			if t.State.Terminal() || b.CancelRequested {
				return context.Canceled
			}
			if err := core.ValidateClipEvidence(b.Plan.Request, t.Segment, evidence); err != nil {
				return err
			}
			t.Evidence = &evidence
			t.AssetID = core.NewID()
			t.State = core.ClipPublishing
			return nil
		}); e != nil {
			if !errors.Is(e, context.Canceled) {
				s.fail(id, t.ID, e)
			}
			continue
		}
		if e = s.publish(id, index); e != nil {
			s.mu.Lock()
			s.failure = e
			s.mu.Unlock()
			return
		}
	}
	// Source is private and can be removed once no task needs it; failed tasks
	// retain it for an explicit retry. Published outputs are never cleaned here.
	b, e = s.repo.ClipBatch(s.life.ctx, id)
	if e == nil && b.Settled() {
		allDone := true
		for _, t := range b.Tasks {
			allDone = allDone && t.State == core.ClipDone
		}
		if allDone {
			_ = s.files.Cleanup(s.life.ctx, id)
		}
	}
}

type clipResource struct {
	mu      sync.Mutex
	manager *resources
	permit  *permit
	owner   string
}

func (r *clipResource) Enter(ctx context.Context, stage core.Stage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	pool := "processing"
	if stage == core.Downloading {
		pool = "downloading"
	}
	if r.permit != nil && r.permit.pool == pool {
		return ctx.Err()
	}
	r.manager.release(r.permit)
	r.permit = nil
	p, e := r.manager.acquireNamed(ctx, pool, r.owner)
	if e == nil {
		r.permit = p
	}
	return e
}
func (r *clipResource) RateBPS() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.permit == nil {
		return 0
	}
	return r.permit.rate
}
func (r *clipResource) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.manager.release(r.permit)
	r.permit = nil
}
func clipExtension(r core.ClipRequest) string {
	if r.Mode == "video" {
		return "mp4"
	}
	if r.Audio == "opus" {
		return "opus"
	}
	return r.Audio
}
func (s *ClipService) publish(id string, index int) error {
	b, e := s.repo.ClipBatch(s.life.ctx, id)
	if e != nil {
		return e
	}
	t := b.Tasks[index]
	if t.State != core.ClipPublishing || t.Evidence == nil {
		return nil
	}
	if e = s.files.Publish(s.life.ctx, t.Candidate, t.FinalPath, *t.Evidence); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, e = s.repo.ClipBatch(s.life.ctx, id)
	if e != nil {
		return e
	}
	t = b.Tasks[index]
	if t.State != core.ClipPublishing {
		return nil
	}
	a := core.Asset{ID: t.AssetID, Path: t.FinalPath, Title: t.Segment.Title, Kind: b.Plan.Request.Mode, Container: clipExtension(b.Plan.Request), Duration: float64(t.Evidence.DurationMS) / 1000, Size: t.Evidence.Size, SHA256: t.Evidence.SHA256, Status: "available", LocalVerification: "clip_verified", Width: t.Evidence.Width, Height: t.Evidence.Height, VideoCodec: t.Evidence.VideoCodec, AudioCodec: t.Evidence.AudioCodec, Added: time.Now().UTC(), SourceURL: b.Plan.Request.Source.URL}
	if a.Title == "" {
		a.Title = fmt.Sprintf("%s — %d", b.Prepared.Title, index+1)
	}
	// Obtain filesystem metadata through the injected asset prober/publication port.
	metadata, e := s.files.Metadata(s.life.ctx, a.Path)
	if e != nil {
		return e
	}
	a.Modified = metadata.Modified
	expected := b.Revision
	b.Revision++
	b.Updated = time.Now().UTC()
	b.Tasks[index].State = core.ClipDone
	b.Tasks[index].Metrics.Percent = ptrFloat(100)
	e = s.repo.CommitClip(s.life.ctx, b, expected, index, a)
	if e == nil {
		s.failure = nil
		_ = s.files.CleanupCandidate(s.life.ctx, t.Candidate)
	}
	return e
}
func ptrFloat(v float64) *float64 { return &v }
func clipOutputTitle(b core.ClipBatch, t core.ClipTask) string {
	title := t.Segment.Title
	if title == "" {
		title = b.Plan.Source.Title
	}
	return fmt.Sprintf("%s [%d-%d]", title, t.Segment.Range.StartMS, t.Segment.Range.EndMS)
}

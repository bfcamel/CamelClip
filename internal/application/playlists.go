// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"sync"
	"time"
)

type PlaylistRepository interface {
	SaveSubscription(context.Context, core.PlaylistSubscription) (core.PlaylistSubscription, error)
	Subscriptions(context.Context, int, int) (core.PlaylistPage, error)
	Subscription(context.Context, string) (core.PlaylistSubscription, error)
	SubscriptionItems(context.Context, string, int, int) (core.PlaylistItemPage, error)
	RecoverPlaylistScans(context.Context) error
	DueSubscriptions(context.Context, time.Time, int) ([]core.PlaylistSubscription, error)
	BeginPlaylistScan(context.Context, string, int) (string, error)
	ObservePlaylist(context.Context, string, []core.PlaylistEntry) error
	FinishPlaylistScan(context.Context, string, error) error
	SyncSubscription(context.Context, string) error
	BackfillSubscription(context.Context, string) error
	PlaylistCandidates(context.Context, int) ([]core.PlaylistItem, error)
	AdmitPlaylistJob(context.Context, string, string, string, int, Job) (Job, bool, error)
	PlaylistItemJob(context.Context, string, string) (int, error)
	BlockPlaylistItem(context.Context, string, string, string) error
}

// PlaylistQueue deliberately exposes no concrete repository or downloader.
type PlaylistQueue interface {
	AdmitPlaylist(context.Context, core.PlaylistSubscription, string) (Job, error)
	PendingJobs() int
	Start()
	Retry(int, string) error
}
type PlaylistService struct {
	repo      PlaylistRepository
	source    core.PlaylistSource
	queue     PlaylistQueue
	life      *lifetime
	mu        sync.Mutex
	started   bool
	active    map[string]context.CancelFunc
	wg        sync.WaitGroup
	failure   error
	closeOnce sync.Once
}

func NewPlaylists(ctx context.Context, repo PlaylistRepository, source core.PlaylistSource, q PlaylistQueue) (*PlaylistService, error) {
	if repo == nil || source == nil || q == nil {
		return nil, errors.New("playlist repository, source and queue required")
	}
	if e := repo.RecoverPlaylistScans(ctx); e != nil {
		return nil, e
	}
	s := &PlaylistService{repo: repo, source: source, queue: q, life: newLifetime(ctx), active: map[string]context.CancelFunc{}}
	s.wg.Add(1)
	go s.loop()
	return s, nil
}
func (s *PlaylistService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.life.ctx.Err() == nil {
		s.started = true
	}
}
func (s *PlaylistService) Err() error { s.mu.Lock(); defer s.mu.Unlock(); return s.failure }
func (s *PlaylistService) Close() error {
	s.closeOnce.Do(func() {
		s.life.stop()
		s.mu.Lock()
		for _, cancel := range s.active {
			cancel()
		}
		s.mu.Unlock()
		s.wg.Wait()
		s.life.wait()
	})
	return s.Err()
}
func (s *PlaylistService) Save(ctx context.Context, p core.PlaylistSubscription) (core.PlaylistSubscription, error) {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return p, e
	}
	defer done()
	p, e = core.ValidateSubscription(p)
	if e != nil {
		return p, e
	}
	p, e = s.repo.SaveSubscription(ctx, p)
	if e == nil {
		s.mu.Lock()
		if cancel := s.active[p.ID]; cancel != nil {
			cancel()
		}
		s.mu.Unlock()
	}
	return p, e
}
func (s *PlaylistService) List(ctx context.Context, limit, offset int) (core.PlaylistPage, error) {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return core.PlaylistPage{}, e
	}
	defer done()
	return s.repo.Subscriptions(ctx, limit, offset)
}
func (s *PlaylistService) Items(ctx context.Context, id string, limit, offset int) (core.PlaylistItemPage, error) {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return core.PlaylistItemPage{}, e
	}
	defer done()
	return s.repo.SubscriptionItems(ctx, id, limit, offset)
}
func (s *PlaylistService) SetEnabled(ctx context.Context, id string, enabled bool) error {
	p, e := s.repo.Subscription(ctx, id)
	if e != nil {
		return e
	}
	p.Enabled = enabled
	_, e = s.Save(ctx, p)
	return e
}
func (s *PlaylistService) Sync(ctx context.Context, id string) error {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	return s.repo.SyncSubscription(ctx, id)
}
func (s *PlaylistService) Backfill(ctx context.Context, id string) error {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	return s.repo.BackfillSubscription(ctx, id)
}
func (s *PlaylistService) Retry(ctx context.Context, id, video string) error {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	p, e := s.repo.Subscription(ctx, id)
	if e != nil {
		return e
	}
	jobID, e := s.repo.PlaylistItemJob(ctx, id, video)
	if e != nil {
		return e
	}
	return s.queue.Retry(jobID, p.Request.Cookies)
}

func (s *PlaylistService) loop() {
	defer s.wg.Done()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-s.life.ctx.Done():
			return
		case <-tick.C:
			s.step()
		}
	}
}
func (s *PlaylistService) step() {
	s.mu.Lock()
	if !s.started || s.failure != nil || s.life.ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	slots := 2 - len(s.active)
	s.mu.Unlock()
	if slots > 0 {
		due, e := s.repo.DueSubscriptions(s.life.ctx, time.Now(), slots)
		if e != nil {
			s.fail(e)
			return
		}
		for _, p := range due {
			scan, e := s.repo.BeginPlaylistScan(s.life.ctx, p.ID, p.Revision)
			if e != nil {
				continue
			}
			ctx, cancel := context.WithCancel(s.life.ctx)
			s.mu.Lock()
			s.active[p.ID] = cancel
			s.wg.Add(1)
			s.mu.Unlock()
			go s.scan(ctx, cancel, p, scan)
		}
	}
	pending := s.queue.PendingJobs()
	if pending >= 1000 {
		return
	}
	for i := 0; i < min(10, 1000-pending); i++ {
		items, e := s.repo.PlaylistCandidates(s.life.ctx, 1)
		if e != nil {
			s.fail(e)
			return
		}
		if len(items) == 0 {
			return
		}
		item := items[0]
		p, e := s.repo.Subscription(s.life.ctx, item.SubscriptionID)
		if e != nil {
			s.fail(e)
			return
		}
		if _, e = s.queue.AdmitPlaylist(s.life.ctx, p, item.VideoID); e != nil {
			if s.life.ctx.Err() != nil {
				return
			}
			if core.CodeOf(e) == core.CodeConflict {
				continue
			}
			if core.CodeOf(e) == core.CodeValidation {
				if err := s.repo.BlockPlaylistItem(s.life.ctx, p.ID, item.VideoID, e.Error()); err != nil {
					s.fail(err)
					return
				}
				continue
			}
			s.fail(fmt.Errorf("playlist admission: %w", e))
			return
		}
	}
}
func (s *PlaylistService) scan(ctx context.Context, cancel context.CancelFunc, p core.PlaylistSubscription, scan string) {
	defer s.wg.Done()
	defer cancel()
	defer func() { s.mu.Lock(); delete(s.active, p.ID); s.mu.Unlock() }()
	s.enumerateAndCommit(ctx, p, scan)
}
func (s *PlaylistService) Check(ctx context.Context, id string) error {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	p, e := s.repo.Subscription(ctx, id)
	if e != nil {
		return e
	}
	scan, e := s.repo.BeginPlaylistScan(ctx, id, p.Revision)
	if e != nil {
		return e
	}
	return s.enumerateAndCommit(ctx, p, scan)
}
func (s *PlaylistService) enumerateAndCommit(ctx context.Context, p core.PlaylistSubscription, scan string) error {
	var err error
	func() {
		defer func() {
			if v := recover(); v != nil {
				err = core.ContractError(fmt.Sprintf("playlist adapter panic: %v", v))
			}
		}()
		err = s.source.Enumerate(ctx, core.PlaylistScan{PlaylistID: p.PlaylistID, Browser: p.Request.Cookies}, func(entries []core.PlaylistEntry) error {
			if len(entries) > 250 {
				return core.ContractError("playlist source batch exceeds 250 entries")
			}
			for _, v := range entries {
				if !core.YouTubeVideoID(v.ID) || len(v.Title) > 4096 {
					return core.ContractError("invalid video identity")
				}
			}
			return s.repo.ObservePlaylist(ctx, scan, entries)
		})
	}()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	finish, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()
	if e := s.repo.FinishPlaylistScan(finish, scan, err); e != nil {
		s.fail(e)
		return e
	}
	return err
}

func (s *PlaylistService) fail(e error) { s.mu.Lock(); defer s.mu.Unlock(); s.failure = e }
func playlistMaterializationKey(r core.Request, video string) string {
	v := struct{ Video, Mode, Quality, Audio, Codec, Container, Out string }{video, r.Mode, r.Quality, r.Audio, core.VideoCodec(r), core.VideoContainer(r), r.Out}
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func (s *Service) AdmitPlaylist(ctx context.Context, p core.PlaylistSubscription, video string) (Job, error) {
	if !core.YouTubeVideoID(video) {
		return Job{}, errors.New("invalid video ID")
	}
	r := p.Request
	r.URL = "https://www.youtube.com/watch?v=" + video
	r, e := core.NormalizeRequest(r)
	if e != nil {
		return Job{}, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e = s.availableLocked(); e != nil {
		return Job{}, e
	}
	repo, ok := s.repository.(PlaylistRepository)
	if !ok {
		return Job{}, errors.New("playlist admission unavailable")
	}
	// Persist restored/dirty work before the atomic idempotent admission transaction.
	if len(s.dirty) > 0 || len(s.removed) > 0 {
		if e = s.saveLocked(); e != nil {
			return Job{}, e
		}
	}
	candidate := Job{ID: s.nextID + 1, Request: r, Title: r.URL, State: Queued, Control: JobControl{Position: time.Now().UnixNano()}, Monitoring: Monitoring{CreatedAt: time.Now(), UpdatedAt: time.Now()}}
	j, created, e := repo.AdmitPlaylistJob(ctx, playlistMaterializationKey(r, video), p.ID, video, p.Revision, candidate)
	if e != nil {
		return Job{}, e
	}
	if created {
		s.nextID = j.ID
		s.index[j.ID] = len(s.jobs)
		s.jobs = append(s.jobs, clone(j))
		s.syncOrderLocked()
		s.emitLocked(j, core.StageChanged, "")
		s.scheduleLocked()
	}
	return clone(j), nil
}

func (s *Service) PendingJobs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, j := range s.jobs {
		if !j.State.Terminal() {
			n++
		}
	}
	return n
}

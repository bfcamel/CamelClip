// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CompanionService serializes cache mutation and reads, independently of frontends.
// Files are bounded in-memory reads: eviction cannot remove an active reader's bytes.
type CompanionService struct {
	mu        sync.Mutex
	repo      CompanionRepository
	assets    AssetRepository
	backend   core.CompanionBackend
	files     CompanionFiles
	parser    CaptionParser
	resources *resources
	life      *lifetime
	session   string
	policy    core.CompanionPolicy
}

func NewCompanions(ctx context.Context, repo CompanionRepository, assets AssetRepository, backend core.CompanionBackend, files CompanionFiles, parser CaptionParser, queue *Service) *CompanionService {
	return &CompanionService{repo: repo, assets: assets, backend: backend, files: files, parser: parser, resources: queue.resources, life: newLifetime(ctx), session: core.NewID(), policy: core.DefaultCompanionPolicy()}
}
func (s *CompanionService) Close() error { s.life.stop(); s.life.wait(); return nil }
func (s *CompanionService) local(ctx context.Context, r core.CompanionRequest) (string, error) {
	if r.Source.AssetID == "" {
		return "", nil
	}
	a, e := s.assets.Asset(ctx, r.Source.AssetID)
	return a.Path, e
}
func (s *CompanionService) Discover(ctx context.Context, r core.CompanionRequest) (core.CompanionCatalog, error) {
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return core.CompanionCatalog{}, err
	}
	defer done()
	normalized, err := core.NormalizeClipRequest(core.ClipRequest{Source: r.Source, Browser: r.Browser, Segments: []core.ClipSegment{{Range: core.ClipRange{StartMS: 0, EndMS: 1000}}}, OutputDir: "."})
	if err != nil {
		return core.CompanionCatalog{}, err
	}
	r.Source = normalized.Source
	r.Browser = normalized.Browser
	local, err := s.local(ctx, r)
	if err != nil {
		return core.CompanionCatalog{}, err
	}
	p, err := s.resources.acquireNamed(ctx, "metadata", "companions:"+core.NewID())
	if err != nil {
		return core.CompanionCatalog{}, err
	}
	defer s.resources.release(p)
	c, err := companionCall(func() (core.CompanionCatalog, error) { return s.backend.Discover(ctx, r, local) })
	if err != nil {
		return c, err
	}
	if c.Source.Identity == "" || c.Source.DurationMS <= 0 {
		return c, &core.Failure{Code: core.CodeContract, Message: "invalid companion source descriptor"}
	}
	c.ID = core.NewID()
	c.SchemaVersion = 1
	c.Request = r
	c.Created = time.Now().UTC()
	c.Expires = c.Created.Add(6 * time.Hour)
	c.AuthSession = s.session
	c.Scope = core.ClipDigest([]string{c.Source.Identity, c.Source.SHA256, r.Browser, s.session})
	if c.Tracks == nil {
		c.Tracks = []core.CaptionDescriptor{}
	}
	err = s.repo.SaveCompanionCatalog(ctx, c)
	return c, err
}
func (s *CompanionService) catalog(ctx context.Context, id string) (core.CompanionCatalog, string, error) {
	c, e := s.repo.CompanionCatalog(ctx, id)
	if e != nil {
		return c, "", e
	}
	if c.Request.Browser != "" && c.AuthSession != s.session {
		return c, "", errors.New("inspect again to authorize browser cookies in this session")
	}
	if time.Now().After(c.Expires) {
		return c, "", errors.New("catalog expired; inspect source again")
	}
	p, e := s.local(ctx, c.Request)
	return c, p, e
}
func (s *CompanionService) put(ctx context.Context, c core.CompanionCatalog, kind, ext, mime string, data []byte) (core.CompanionBlob, error) {
	b, e := s.files.Put(ctx, c.Scope, kind, ext, mime, data)
	if e != nil {
		return b, e
	}
	if e = s.repo.SaveCompanionBlob(ctx, b); e != nil {
		return b, e
	}
	e = s.repo.LinkCompanion(ctx, c.ID, b.ID, kind, false)
	if e == nil && c.Request.Source.AssetID != "" && s.policy.KeepLoaded {
		links, err := s.repo.CompanionLinks(ctx, c.Request.Source.AssetID)
		if err != nil {
			return b, err
		}
		count := 0
		for _, v := range links {
			if v.Kind == kind {
				count++
			}
		}
		maximum := 2
		if kind == "poster" {
			maximum = 1
		}
		if count < maximum {
			e = s.repo.LinkCompanion(ctx, c.Request.Source.AssetID, b.ID, kind, true)
		}
	}
	return b, e
}
func (s *CompanionService) Poster(ctx context.Context, id string) (core.CompanionBlob, error) {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return core.CompanionBlob{}, e
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.cacheLocked(ctx, true); err != nil {
		return core.CompanionBlob{}, err
	}
	c, path, e := s.catalog(ctx, id)
	if e != nil {
		return core.CompanionBlob{}, e
	}
	links, e := s.repo.CompanionLinks(ctx, id)
	if e != nil {
		return core.CompanionBlob{}, e
	}
	for _, b := range links {
		if b.Kind == "poster" {
			return b, nil
		}
	}
	if !c.Posters {
		return core.CompanionBlob{}, errors.New("no poster available")
	}
	resource := &clipResource{manager: s.resources, owner: "companion:" + id}
	defer resource.close()
	ctx = core.WithResources(ctx, resource)
	data, e := companionCall(func() ([]byte, error) { return s.backend.Poster(ctx, c, path) })
	if e != nil {
		return core.CompanionBlob{}, e
	}
	if len(data) == 0 || len(data) > 8<<20 {
		return core.CompanionBlob{}, errors.New("invalid poster size")
	}
	return s.put(ctx, c, "poster", "jpg", "image/jpeg", data)
}
func (s *CompanionService) FetchCaption(ctx context.Context, id, trackID string) (core.CaptionSnapshot, error) {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return core.CaptionSnapshot{}, e
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.cacheLocked(ctx, true); err != nil {
		return core.CaptionSnapshot{}, err
	}
	c, path, e := s.catalog(ctx, id)
	if e != nil {
		return core.CaptionSnapshot{}, e
	}
	snapshots, e := s.repo.Captions(ctx, id)
	if e != nil {
		return core.CaptionSnapshot{}, e
	}
	for _, v := range snapshots {
		if v.Track.ID == trackID {
			return v, nil
		}
	}
	var track *core.CaptionDescriptor
	for _, v := range c.Tracks {
		if v.ID == trackID {
			copy := v
			track = &copy
			break
		}
	}
	if track == nil {
		return core.CaptionSnapshot{}, errors.New("track not in catalog")
	}
	resource := &clipResource{manager: s.resources, owner: "companion:" + id}
	defer resource.close()
	ctx = core.WithResources(ctx, resource)
	data, e := companionCall(func() ([]byte, error) { return s.backend.Caption(ctx, c, *track, path) })
	if e != nil {
		return core.CaptionSnapshot{}, e
	}
	cues, e := companionCall(func() ([]core.CaptionCue, error) { return s.parser.Parse(data) })
	if e != nil {
		return core.CaptionSnapshot{}, e
	}
	normalized, e := json.Marshal(cues)
	if e != nil {
		return core.CaptionSnapshot{}, e
	}
	if len(normalized) > 32<<20 {
		return core.CaptionSnapshot{}, errors.New("normalized captions exceed limit")
	}
	raw, e := s.put(ctx, c, "caption_raw", "vtt", "text/vtt", data)
	if e != nil {
		return core.CaptionSnapshot{}, e
	}
	parsed, e := s.put(ctx, c, "caption_cues", "json", "application/json", normalized)
	if e != nil {
		return core.CaptionSnapshot{}, e
	}
	v := core.CaptionSnapshot{SchemaVersion: 1, ID: core.NewID(), CatalogID: id, Track: *track, RawID: raw.ID, CuesID: parsed.ID, CueCount: len(cues), Timing: "source", Normalization: "plain-text-v1", Created: time.Now().UTC()}
	e = s.repo.SaveCaption(ctx, v)
	return v, e
}
func (s *CompanionService) readCues(ctx context.Context, id string) (core.CaptionSnapshot, []core.CaptionCue, error) {
	v, e := s.repo.Caption(ctx, id)
	if e != nil {
		return v, nil, e
	}
	b, e := s.repo.CompanionBlob(ctx, v.CuesID)
	if e != nil {
		return v, nil, e
	}
	data, e := s.files.Read(ctx, b, 32<<20)
	if e != nil {
		return v, nil, e
	}
	var cues []core.CaptionCue
	e = json.Unmarshal(data, &cues)
	return v, cues, e
}
func (s *CompanionService) Cues(ctx context.Context, id, query, cursor string, limit int) (core.CaptionPage, error) {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return core.CaptionPage{}, e
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit < 1 || limit > 500 || len(query) > 512 {
		return core.CaptionPage{}, errors.New("invalid transcript query")
	}
	offset := 0
	if cursor != "" {
		offset, e = strconv.Atoi(cursor)
		if e != nil || offset < 0 {
			return core.CaptionPage{}, errors.New("invalid transcript cursor")
		}
	}
	snap, cues, e := s.readCues(ctx, id)
	if e != nil {
		return core.CaptionPage{}, e
	}
	page := core.CaptionPage{SchemaVersion: 1, Snapshot: snap, Items: []core.CaptionCue{}}
	query = strings.ToLower(query)
	for _, c := range cues {
		if !strings.Contains(strings.ToLower(c.Text), query) {
			continue
		}
		if page.Total >= offset && len(page.Items) < limit {
			page.Items = append(page.Items, c)
		}
		page.Total++
	}
	if offset+len(page.Items) < page.Total {
		page.NextCursor = strconv.Itoa(offset + len(page.Items))
	}
	return page, nil
}
func (s *CompanionService) Propose(ctx context.Context, id string, first, last int) (core.ClipRange, error) {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return core.ClipRange{}, e
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, cues, e := s.readCues(ctx, id)
	if e != nil {
		return core.ClipRange{}, e
	}
	if snap.Timing != "source" {
		return core.ClipRange{}, errors.New("imported caption timing is unverified; review times manually")
	}
	c, e := s.repo.CompanionCatalog(ctx, snap.CatalogID)
	if e != nil {
		return core.ClipRange{}, e
	}
	return core.CaptionInterval(cues, first, last, c.Source.DurationMS)
}
func (s *CompanionService) Blob(ctx context.Context, id string) (core.CompanionBlob, []byte, error) {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return core.CompanionBlob{}, nil, e
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	b, e := s.repo.CompanionBlob(ctx, id)
	if e != nil {
		return b, nil, e
	}
	data, e := s.files.Read(ctx, b, 32<<20)
	if e == nil {
		b.Used = time.Now().UTC()
		e = s.repo.SaveCompanionBlob(ctx, b)
	}
	return b, data, e
}
func (s *CompanionService) Links(ctx context.Context, owner string) ([]core.CompanionBlob, error) {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.repo.CompanionLinks(ctx, owner)
}
func (s *CompanionService) Keep(ctx context.Context, owner, id string, keep bool) error {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	links, e := s.repo.CompanionLinks(ctx, owner)
	if e != nil {
		return e
	}
	for _, b := range links {
		if b.ID == id {
			return s.repo.LinkCompanion(ctx, owner, id, b.Kind, keep)
		}
	}
	return errors.New("companion is not linked to this owner")
}
func (s *CompanionService) Cache(ctx context.Context, prune bool) (core.CompanionCacheStats, error) {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return core.CompanionCacheStats{}, e
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cacheLocked(ctx, prune)
}
func (s *CompanionService) cacheLocked(ctx context.Context, prune bool) (core.CompanionCacheStats, error) {
	items, e := s.repo.CompanionCache(ctx)
	stats := core.CompanionCacheStats{SchemaVersion: 1}
	if e != nil {
		return stats, e
	}
	for _, v := range items {
		if v.Kept {
			stats.KeptBytes += v.Blob.Size
		} else {
			stats.CachedBytes += v.Blob.Size
		}
		stats.Blobs++
	}
	if !prune {
		return stats, nil
	}
	for _, v := range items {
		if v.Kept {
			continue
		}
		if stats.CachedBytes <= s.policy.CacheBytes && time.Since(v.Blob.Used) < time.Duration(s.policy.CacheHours)*time.Hour {
			continue
		}
		if e = s.files.Remove(ctx, v.Blob); e != nil {
			return stats, e
		}
		if e = s.repo.DeleteCompanionBlob(ctx, v.Blob.ID); e != nil {
			return stats, e
		}
		stats.CachedBytes -= v.Blob.Size
		stats.Removed++
		stats.Blobs--
	}
	return stats, nil
}

// Import accepts only explicitly supplied caption bytes; it never scans adjacent files.
func (s *CompanionService) Import(ctx context.Context, catalogID, language string, data []byte) (core.CaptionSnapshot, error) {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return core.CaptionSnapshot{}, e
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.cacheLocked(ctx, true); err != nil {
		return core.CaptionSnapshot{}, err
	}
	if len(language) > 64 {
		return core.CaptionSnapshot{}, errors.New("invalid language")
	}
	c, e := s.repo.CompanionCatalog(ctx, catalogID)
	if e != nil {
		return core.CaptionSnapshot{}, e
	}
	cues, e := companionCall(func() ([]core.CaptionCue, error) { return s.parser.Parse(data) })
	if e != nil {
		return core.CaptionSnapshot{}, e
	}
	normalized, e := json.Marshal(cues)
	if e != nil || len(normalized) > 32<<20 {
		return core.CaptionSnapshot{}, errors.New("normalized captions exceed limit")
	}
	raw, e := s.put(ctx, c, "caption_raw", "txt", "text/plain", data)
	if e != nil {
		return core.CaptionSnapshot{}, e
	}
	parsed, e := s.put(ctx, c, "caption_cues", "json", "application/json", normalized)
	if e != nil {
		return core.CaptionSnapshot{}, e
	}
	track := core.CaptionDescriptor{ID: core.NewID(), Language: language, Origin: "imported", Translated: "unknown", Format: "vtt"}
	v := core.CaptionSnapshot{SchemaVersion: 1, ID: core.NewID(), CatalogID: c.ID, Track: track, RawID: raw.ID, CuesID: parsed.ID, CueCount: len(cues), Timing: "unverified", Normalization: "plain-text-v1", Created: time.Now().UTC()}
	e = s.repo.SaveCaption(ctx, v)
	return v, e
}
func (s *CompanionService) Export(ctx context.Context, snapshot, path string) error {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, cues, e := s.readCues(ctx, snapshot)
	if e != nil {
		return e
	}
	c, e := s.repo.CompanionCatalog(ctx, snap.CatalogID)
	if e != nil {
		return e
	}
	blob, e := s.put(ctx, c, "caption_export", "vtt", "text/vtt", core.RenderWebVTT(cues))
	if e != nil {
		return e
	}
	return s.files.Export(ctx, blob, path)
}

func companionCall[T any](fn func() (T, error)) (v T, err error) {
	defer func() {
		if recover() != nil {
			err = &core.Failure{Code: core.CodeBackend, Message: fmt.Sprint("companion adapter failed")}
		}
	}()
	return fn()
}

// WebVTT exports normalized safe text through the same Core formatter for every UI.
func (s *CompanionService) WebVTT(ctx context.Context, id string) ([]byte, error) {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	_, cues, e := s.readCues(ctx, id)
	if e != nil {
		return nil, e
	}
	return core.RenderWebVTT(cues), nil
}

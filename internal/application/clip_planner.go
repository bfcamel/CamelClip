// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"errors"
	"github.com/bfcamel/CamelClip/internal/core"
	"path/filepath"
	"sort"
	"time"
)

func (s *ClipService) local(ctx context.Context, r core.ClipRequest) (string, error) {
	if r.Source.AssetID == "" {
		return "", nil
	}
	a, e := s.assets.Asset(ctx, r.Source.AssetID)
	if e != nil {
		return "", e
	}
	return a.Path, nil
}
func (s *ClipService) Plan(ctx context.Context, r core.ClipRequest) (core.ClipPlan, error) {
	ctx, done, e := s.life.begin(ctx)
	if e != nil {
		return core.ClipPlan{}, e
	}
	defer done()
	r, e = core.NormalizeClipRequest(r)
	if e != nil {
		return core.ClipPlan{}, e
	}
	r.OutputDir, e = filepath.Abs(r.OutputDir)
	if e != nil {
		return core.ClipPlan{}, e
	}
	if path, err := filepath.EvalSymlinks(r.OutputDir); err == nil {
		r.OutputDir = path
	}
	local, e := s.local(ctx, r)
	if e != nil {
		return core.ClipPlan{}, e
	}
	permit, e := s.resources.acquireNamed(ctx, "metadata", "clip-inspect:"+core.NewID())
	if e != nil {
		return core.ClipPlan{}, e
	}
	source, e := safeClipInspect(s.backend, ctx, r, local)
	s.resources.release(permit)
	if e != nil {
		return core.ClipPlan{}, e
	}
	r, e = core.ResolveClipChapters(r, source)
	if e != nil {
		return core.ClipPlan{}, e
	}
	if e = core.ValidateClipSource(source, r); e != nil {
		return core.ClipPlan{}, e
	}
	p := core.ClipPlan{ID: core.NewID(), Revision: 1, Created: time.Now().UTC(), Request: r, Source: source, Destinations: []core.S3Profile{}, Warnings: []string{}}
	p.Expires = p.Created.Add(24 * time.Hour)
	if local == "" {
		p.Warnings = append(p.Warnings, "The entire selected source will be downloaded once. Keep CamelClip running while clips are processed.")
	}
	seen := map[string]bool{}
	sort.Strings(r.S3Profiles)
	p.Request.S3Profiles = nil
	for _, id := range r.S3Profiles {
		if seen[id] {
			continue
		}
		seen[id] = true
		if s.profiles == nil {
			return p, errors.New("S3 profiles unavailable")
		}
		d, e := s.profiles.Profile(ctx, id)
		if e != nil {
			return p, e
		}
		if !d.Enabled {
			return p, errors.New("selected S3 profile is disabled")
		}
		p.Destinations = append(p.Destinations, d)
		p.Request.S3Profiles = append(p.Request.S3Profiles, id)
	}
	p.Digest = core.ClipDigest(struct {
		Request      core.ClipRequest
		Source       core.ClipSource
		Destinations []core.S3Profile
		Pipeline     string
	}{p.Request, p.Source, p.Destinations, core.ClipPipelineVersion})
	e = s.repo.SaveClipPlan(ctx, p)
	return p, e
}

func (s *ClipService) Inspect(ctx context.Context, r core.ClipRequest) (core.ClipSource, error) {
	ctx, d, e := s.life.begin(ctx)
	if e != nil {
		return core.ClipSource{}, e
	}
	defer d()
	r.Segments = []core.ClipSegment{{Range: core.ClipRange{StartMS: 0, EndMS: 1000}}}
	r.ChapterIndices = nil
	r, e = core.NormalizeClipRequest(r)
	if e != nil {
		return core.ClipSource{}, e
	}
	local, e := s.local(ctx, r)
	if e != nil {
		return core.ClipSource{}, e
	}
	p, e := s.resources.acquireNamed(ctx, "metadata", "clip-inspection:"+core.NewID())
	if e != nil {
		return core.ClipSource{}, e
	}
	defer s.resources.release(p)
	return safeClipInspect(s.backend, ctx, r, local)
}

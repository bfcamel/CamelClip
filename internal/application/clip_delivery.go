// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"github.com/bfcamel/CamelClip/internal/core"
)

func (s *ClipService) deliver() {
	if s.cloud == nil || s.life.ctx.Err() != nil {
		return
	}
	items, e := s.repo.ClipDeliveries(s.life.ctx)
	if e != nil {
		return
	}
	for _, d := range items {
		if s.only != "" {
			b, e := s.repo.ClipBatch(s.life.ctx, s.only)
			if e != nil {
				continue
			}
			found := false
			for _, t := range b.Tasks {
				if t.AssetID == d.AssetID {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		current, e := s.profiles.Profile(s.life.ctx, d.Destination.ID)
		if e != nil || !current.Enabled || core.ClipDigest(current) != core.ClipDigest(d.Destination) {
			d.State = "blocked_profile_changed"
			d.Error = "destination changed; use library upload to select the current destination"
		} else {
			_, e = s.cloud.Enqueue(s.life.ctx, d.AssetID, []string{d.Destination.ID})
			if e == nil {
				d.State = "enqueued"
				d.Error = ""
			} else {
				d.Error = e.Error()
			}
		}
		_ = s.repo.SaveClipDelivery(s.life.ctx, d)
	}
}

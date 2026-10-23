// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import (
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"strconv"
	"strings"
)

type companionPanelResult struct {
	Catalog  *core.CompanionCatalog
	Snapshot *core.CaptionSnapshot
	Page     *core.CaptionPage
	Poster   *core.CompanionBlob
	Range    *core.ClipRange
}

func (u *panelUI) companionPrompt() {
	u.modal = "companion-source"
	u.title = tr("Ссылка или asset:ID из медиатеки")
	u.input = ""
	if a := u.selectedAsset(); u.section == 1 && a != nil {
		u.input = "asset:" + a.ID
	}
}
func (u *panelUI) companionField(value string) {
	s := u.pool.workspace.Companions
	if s == nil {
		return
	}
	if u.companionResults == nil {
		u.companionResults = make(chan companionPanelResult, 1)
	}
	out := u.companionResults
	mode := u.modal
	catalog := u.companionCatalog
	snapshot := u.companionSnapshot
	u.modal = ""
	u.background(func() error {
		result := companionPanelResult{}
		switch mode {
		case "companion-source":
			ref := core.ClipSourceRef{URL: strings.TrimSpace(value)}
			if strings.HasPrefix(ref.URL, "asset:") {
				ref.AssetID = strings.TrimPrefix(ref.URL, "asset:")
				ref.URL = ""
			}
			c, e := s.Discover(u.ctx, core.CompanionRequest{Source: ref, Browser: u.o.Cookies})
			if e != nil {
				return e
			}
			result.Catalog = &c
		case "companion-track":
			if value == "poster" {
				b, e := s.Poster(u.ctx, catalog.ID)
				if e != nil {
					return e
				}
				result.Poster = &b
			} else {
				index, e := strconv.Atoi(value)
				if e != nil || index < 0 || index >= len(catalog.Tracks) {
					return fmt.Errorf("invalid caption index")
				}
				v, e := s.FetchCaption(u.ctx, catalog.ID, catalog.Tracks[index].ID)
				if e != nil {
					return e
				}
				page, e := s.Cues(u.ctx, v.ID, "", "", 100)
				if e != nil {
					return e
				}
				result.Snapshot = &v
				result.Page = &page
			}
		case "companion-query":
			page, e := s.Cues(u.ctx, snapshot.ID, value, "", 100)
			if e != nil {
				return e
			}
			result.Page = &page
		case "companion-range":
			parts := strings.Split(value, ":")
			if len(parts) != 2 {
				return fmt.Errorf("expected FIRST:LAST")
			}
			first, e := strconv.Atoi(parts[0])
			if e != nil {
				return e
			}
			last, e := strconv.Atoi(parts[1])
			if e != nil {
				return e
			}
			r, e := s.Propose(u.ctx, snapshot.ID, first, last)
			if e != nil {
				return e
			}
			result.Range = &r
		}
		select {
		case out <- result:
		case <-u.ctx.Done():
			return u.ctx.Err()
		}
		return nil
	})
}
func (u *panelUI) receiveCompanion() {
	select {
	case v := <-u.companionResults:
		if v.Catalog != nil {
			u.companionCatalog = *v.Catalog
			u.modal = "companion-track"
			u.title = tr("Субтитры: номер дорожки или poster для обложки")
			u.input = ""
			text := v.Catalog.Source.Title
			for i, t := range v.Catalog.Tracks {
				text += fmt.Sprintf("\n%d: %s · %s", i, t.Language, t.Origin)
			}
			u.log(text)
		}
		if v.Snapshot != nil {
			u.companionSnapshot = *v.Snapshot
		}
		if v.Page != nil {
			u.modal = "details"
			u.title = tr("Субтитры: s — поиск, r — интервал")
			u.input = fmt.Sprintf("%d / %d\n", len(v.Page.Items), v.Page.Total)
			for _, c := range v.Page.Items {
				u.input += fmt.Sprintf("%d · %.3f..%.3f\n%s\n\n", c.Index, float64(c.StartMS)/1000, float64(c.EndMS)/1000, c.Text)
			}
			u.detailOffset = 0
		}
		if v.Poster != nil {
			u.modal = "details"
			u.title = tr("Обложка сохранена")
			u.input = v.Poster.ID
		}
		if v.Range != nil {
			u.clipForm = core.ClipRequest{Source: u.companionCatalog.Request.Source, Segments: []core.ClipSegment{{Range: *v.Range}}, Mode: u.o.Mode, Audio: "m4a", Quality: u.o.Quality, OutputDir: u.o.Out, Browser: u.o.Cookies, Accuracy: "precise"}
			u.clipStep = 2
			u.nextClipField()
		}
		u.dirty = true
	default:
	}
}

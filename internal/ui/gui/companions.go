// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package gui

import (
	"github.com/bfcamel/CamelClip/internal/core"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) companionAPI(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/companion") {
		return false
	}
	service := s.workspace.Companions
	if service == nil {
		http.Error(w, "companion service unavailable", 503)
		return true
	}
	ctx := r.Context()
	q := r.URL.Query()
	if r.Method == "GET" {
		switch r.URL.Path {
		case "/api/companion-vtt":
			data, e := service.WebVTT(ctx, q.Get("id"))
			if e != nil {
				clipReply(w, nil, e)
				break
			}
			w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
			w.Write(data)
		case "/api/companion-cues":
			limit := 100
			if q.Get("limit") != "" {
				limit, _ = strconv.Atoi(q.Get("limit"))
			}
			v, e := service.Cues(ctx, q.Get("id"), q.Get("query"), q.Get("cursor"), limit)
			clipReply(w, v, e)
		case "/api/companion-links":
			v, e := service.Links(ctx, q.Get("owner"))
			clipReply(w, v, e)
		case "/api/companion-blob":
			b, data, e := service.Blob(ctx, q.Get("id"))
			if e != nil {
				clipReply(w, nil, e)
				break
			}
			w.Header().Set("Content-Type", b.MIME)
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.Write(data)
		case "/api/companion-cache":
			v, e := service.Cache(ctx, false)
			clipReply(w, v, e)
		default:
			http.NotFound(w, r)
		}
		return true
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return true
	}
	if r.URL.Path == "/api/companion-discover" {
		var req core.CompanionRequest
		if e := decode(w, r, &req); e != nil {
			clipReply(w, nil, e)
			return true
		}
		v, e := service.Discover(ctx, req)
		clipReply(w, v, e)
		return true
	}
	var req struct {
		ID    string `json:"id"`
		Track string `json:"track"`
		Owner string `json:"owner"`
		Keep  bool   `json:"keep"`
		First int    `json:"first"`
		Last  int    `json:"last"`
	}
	if e := decode(w, r, &req); e != nil {
		clipReply(w, nil, e)
		return true
	}
	switch r.URL.Path {
	case "/api/companion-poster":
		v, e := service.Poster(ctx, req.ID)
		clipReply(w, v, e)
	case "/api/companion-caption":
		v, e := service.FetchCaption(ctx, req.ID, req.Track)
		clipReply(w, v, e)
	case "/api/companion-range":
		v, e := service.Propose(ctx, req.ID, req.First, req.Last)
		clipReply(w, v, e)
	case "/api/companion-keep":
		e := service.Keep(ctx, req.Owner, req.ID, req.Keep)
		clipReply(w, map[string]bool{"ok": e == nil}, e)
	case "/api/companion-prune":
		v, e := service.Cache(ctx, true)
		clipReply(w, v, e)
	default:
		http.NotFound(w, r)
	}
	return true
}

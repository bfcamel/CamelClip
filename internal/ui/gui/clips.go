// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package gui

import (
	"encoding/json"
	"errors"
	"github.com/bfcamel/CamelClip/internal/core"
	"net/http"
	"strings"
)

func (s *Server) clipAPI(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/clip") {
		return false
	}
	clips := s.workspace.Clips
	if clips == nil {
		clipReply(w, nil, errors.New("clip service unavailable"))
		return true
	}
	if r.Method == "GET" {
		switch r.URL.Path {
		case "/api/clips":
			v, e := clips.List(r.Context(), 50, r.URL.Query().Get("cursor"))
			clipReply(w, v, e)
		case "/api/clip":
			v, e := clips.Get(r.Context(), r.URL.Query().Get("id"))
			clipReply(w, v, e)
		case "/api/clip-provenance":
			v, e := clips.Provenance(r.Context(), r.URL.Query().Get("id"))
			clipReply(w, v, e)
		case "/api/clip-deliveries":
			v, e := clips.Deliveries(r.Context())
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
	switch r.URL.Path {
	case "/api/clip-inspect":
		var request core.ClipRequest
		if e := decode(w, r, &request); e != nil {
			clipReply(w, nil, e)
			break
		}
		v, e := clips.Inspect(r.Context(), request)
		clipReply(w, v, e)
	case "/api/clip-plan":
		var v struct {
			Request core.ClipRequest `json:"request"`
			Ranges  string           `json:"ranges"`
		}
		if e := decode(w, r, &v); e != nil {
			clipReply(w, nil, e)
			break
		}
		if v.Ranges != "" {
			segments, e := core.ParseClipSegments(v.Ranges)
			if e != nil {
				clipReply(w, nil, e)
				break
			}
			v.Request.Segments = segments
		}
		p, e := clips.Plan(r.Context(), v.Request)
		clipReply(w, p, e)
	case "/api/clip-admit":
		var v struct {
			PlanID   string `json:"plan_id"`
			Revision int64  `json:"revision"`
			Digest   string `json:"digest"`
			Key      string `json:"idempotency_key"`
		}
		if e := decode(w, r, &v); e != nil {
			clipReply(w, nil, e)
			break
		}
		b, e := clips.Admit(r.Context(), v.PlanID, v.Revision, v.Digest, v.Key)
		if e == nil {
			clips.Start("")
		}
		clipReply(w, b, e)
	case "/api/clip-control":
		var v struct {
			ID       string `json:"id"`
			Revision int64  `json:"revision"`
			Action   string `json:"action"`
			TaskID   string `json:"task_id"`
		}
		if e := decode(w, r, &v); e != nil {
			clipReply(w, nil, e)
			break
		}
		b, e := clips.Control(r.Context(), v.ID, v.Revision, v.Action, v.TaskID)
		clipReply(w, b, e)
	default:
		http.NotFound(w, r)
	}
	return true
}

func clipReply(w http.ResponseWriter, v any, err error) {
	if err == nil {
		reply(w, v, nil)
		return
	}
	status := http.StatusBadRequest
	code := core.CodeOf(err)
	if code == core.CodeConflict {
		status = http.StatusConflict
	} else if code == core.CodeNotFound {
		status = http.StatusNotFound
	} else if code == core.CodeContract {
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": 1, "error": err.Error(), "code": code, "retryable": core.Retryable(err)})
}

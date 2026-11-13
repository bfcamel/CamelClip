// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

// Package gui presents the shared workspace through a loopback-only desktop UI.
package gui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/config"
	"github.com/bfcamel/CamelClip/internal/core"
	"github.com/bfcamel/CamelClip/internal/diagnostics"
)

//go:embed web/*
var assets embed.FS

type Dependencies struct {
	OpenConfig    func(string, config.Settings) config.Repository
	OpenWorkspace func(context.Context, string) (*application.Workspace, error)
	OpenMedia     func(context.Context, string, bool) error
}

type Server struct {
	workspace  *application.Workspace
	repository config.Repository
	path       string
	mu         sync.Mutex // serializes settings and mutations, never held by background workers
	settings   config.Settings
	browsers   browserCatalog
	firstRun   bool
	token      string
	host       string
	quit       func()
	openMedia  func(context.Context, string, bool) error
}

func New(ctx context.Context, deps Dependencies, path string, defaults config.Settings, host string, quit func()) (*Server, error) {
	repo := deps.OpenConfig(path, defaults)
	settings, err := repo.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	w, err := deps.OpenWorkspace(ctx, path)
	if err != nil {
		return nil, err
	} // the existing SQLite owner lock covers both frontends
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		w.Close()
		return nil, err
	}
	_, statErr := os.Stat(path)
	s := &Server{browsers: discoverBrowsers(ctx), workspace: w, repository: repo, path: path, settings: settings, firstRun: os.IsNotExist(statErr), token: hex.EncodeToString(token), host: host, quit: quit, openMedia: deps.OpenMedia}
	if err = s.apply(settings); err != nil {
		w.Close()
		return nil, err
	}
	if !s.firstRun {
		w.Queue.Start()
		if w.Clips != nil {
			w.Clips.Start("")
		}
		if w.Playlists != nil {
			w.Playlists.Start()
		}
	}
	// Notifications have one consumer. Views always read authoritative snapshots.
	go func() {
		for range w.Queue.Events() {
		}
	}()
	return s, nil
}

func (s *Server) Close() error { return s.workspace.Close() }
func (s *Server) URL() string  { return "http://" + s.host + "/#" + s.token }
func (s *Server) apply(v config.Settings) error {
	if err := config.Validate(v); err != nil {
		return err
	}
	if err := s.workspace.Queue.SetLimits(v.Limits()); err != nil {
		return err
	}
	if err := s.workspace.Queue.SetPolicy(v.QueuePolicy()); err != nil {
		return err
	}
	return s.workspace.Cloud.SetOptions(v.UploadOptions())
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	if r.Host != s.host {
		http.Error(w, "invalid host", http.StatusForbidden)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+s.host {
		http.Error(w, "invalid origin", http.StatusForbidden)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CamelClip-Token")), []byte(s.token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		s.api(w, r)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	name := map[string]string{"/": "index.html", "/app.js": "app.js", "/clips.js": "clips.js", "/companions.js": "companions.js", "/style.css": "style.css", "/logo.png": "logo.png"}[r.URL.Path]
	if name == "" {
		http.NotFound(w, r)
		return
	}
	data, err := assets.ReadFile("web/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", map[string]string{"logo.png": "image/png", "index.html": "text/html; charset=utf-8", "app.js": "text/javascript; charset=utf-8", "clips.js": "text/javascript; charset=utf-8", "companions.js": "text/javascript; charset=utf-8", "style.css": "text/css; charset=utf-8"}[name])
	w.Write(data)
}
func reply(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if v == nil {
		v = map[string]bool{"ok": true}
	}
	json.NewEncoder(w).Encode(v)
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("expected one JSON object")
	}
	return nil
}
func number(v string, fallback int) int {
	n, e := strconv.Atoi(v)
	if e != nil {
		return fallback
	}
	return n
}

func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	if s.companionAPI(w, r) {
		return
	}
	if s.clipAPI(w, r) {
		return
	}
	ctx := r.Context()
	ws := s.workspace
	if r.Method == "GET" {
		switch r.URL.Path {
		case "/api/state":
			s.mu.Lock()
			v, first := s.settings, s.firstRun
			s.mu.Unlock()
			profiles, e := ws.Cloud.Profiles(ctx)
			if e != nil {
				reply(w, nil, e)
				return
			}
			uploads, e := ws.Cloud.List(ctx, 100, max(0, number(r.URL.Query().Get("upload_offset"), 0)))
			if e != nil {
				reply(w, nil, e)
				return
			}
			scanning, report := ws.Library.ScanStatus()
			playlistError := ""
			if ws.Playlists != nil {
				if e := ws.Playlists.Err(); e != nil {
					playlistError = e.Error()
				}
			}
			clipError := ""
			if ws.Clips != nil && ws.Clips.Err() != nil {
				clipError = ws.Clips.Err().Error()
			}
			qerr := ""
			if e := ws.Queue.Err(); e != nil {
				qerr = e.Error()
			}
			reply(w, map[string]any{"jobs": ws.Queue.Jobs(), "statistics": ws.Queue.Statistics(), "profiles": profiles, "uploads": uploads, "settings": v, "firstRun": first, "browsers": s.browsers, "locked": ws.Cloud.Locked(), "hasVault": ws.Cloud.HasVault(ctx), "system": ws.Monitor.Snapshot(), "scanning": scanning, "scanReport": report, "queueError": qerr, "playlistError": playlistError, "clipError": clipError}, nil)
		case "/api/playlists":
			if ws.Playlists == nil {
				reply(w, core.PlaylistPage{Items: []core.PlaylistSubscription{}}, nil)
				return
			}
			v, e := ws.Playlists.List(ctx, 50, max(0, number(r.URL.Query().Get("offset"), 0)))
			reply(w, v, e)
		case "/api/playlist-items":
			if ws.Playlists == nil {
				reply(w, nil, errors.New("playlist service unavailable"))
				return
			}
			v, e := ws.Playlists.Items(ctx, r.URL.Query().Get("id"), 50, max(0, number(r.URL.Query().Get("offset"), 0)))
			reply(w, v, e)
		case "/api/library":
			q := r.URL.Query()
			query := core.AssetQuery{Search: q.Get("search"), Kind: q.Get("kind"), Status: q.Get("status"), Container: q.Get("container"), Sort: q.Get("sort"), MinHeight: number(q.Get("height"), 0), Offset: max(0, number(q.Get("offset"), 0)), Limit: 50}
			if q.Get("copies") != "" {
				b := q.Get("copies") == "true"
				query.HasCopies = &b
			}
			if q.Get("since") != "" {
				v, e := time.Parse("2006-01-02", q.Get("since"))
				if e != nil {
					reply(w, nil, e)
					return
				}
				query.Since = v
			}
			if q.Get("until") != "" {
				v, e := time.Parse("2006-01-02", q.Get("until"))
				if e != nil {
					reply(w, nil, e)
					return
				}
				query.Until = v.Add(24 * time.Hour)
			}
			v, e := ws.Library.List(ctx, query)
			reply(w, v, e)
		default:
			http.NotFound(w, r)
		}
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if r.URL.Path == "/api/doctor" {
		s.mu.Lock()
		v := s.settings
		s.mu.Unlock()
		ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		reply(w, diagnostics.Collect(ctx, "gui-development", "development", s.path, v), nil)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.URL.Path == "/api/settings" {
		var v config.Settings
		if e := decode(w, r, &v); e != nil {
			reply(w, nil, e)
			return
		}
		var normalizeErr error
		v, normalizeErr = config.Normalize(v)
		if e := normalizeErr; e != nil {
			reply(w, nil, e)
			return
		}
		if v.MaxUploads == 0 {
			v.MaxUploads = 2
		}
		if v.UploadParts == 0 {
			v.UploadParts = 2
		}
		if v.MaxDownloads < 1 {
			reply(w, nil, errors.New("downloads must be 1..32"))
			return
		}
		if !filepath.IsAbs(v.Out) {
			reply(w, nil, errors.New("output directory must be absolute"))
			return
		}
		if e := os.MkdirAll(v.Out, 0755); e != nil {
			reply(w, nil, e)
			return
		}
		if e := s.repository.Save(ctx, v); e != nil {
			reply(w, nil, e)
			return
		}
		e := s.apply(v)
		if e == nil {
			s.settings = v
			s.firstRun = false
			ws.Queue.Start()
			if ws.Clips != nil {
				ws.Clips.Start("")
			}
			if ws.Playlists != nil {
				ws.Playlists.Start()
			}
		}
		reply(w, nil, e)
		return
	}
	if r.URL.Path != "/api/action" {
		http.NotFound(w, r)
		return
	}
	var a struct {
		Subscription *core.PlaylistSubscription `json:"subscription"`
		AssetIDs     []string                   `json:"assetIds"`
		Action       string                     `json:"action"`
		IDs          []int                      `json:"ids"`
		ID           string                     `json:"id"`
		URLs         []string                   `json:"urls"`
		Request      *core.Request              `json:"request"`
		Choose       bool                       `json:"choose"`
		Value        string                     `json:"value"`
		Path         string                     `json:"path"`
		Confirm      bool                       `json:"confirm"`
		Recursive    bool                       `json:"recursive"`
		Profiles     []string                   `json:"profiles"`
		Profile      core.S3Profile             `json:"profile"`
		Credentials  *struct {
			AccessKey    string `json:"accessKey"`
			SecretKey    string `json:"secretKey"`
			SessionToken string `json:"sessionToken"`
		} `json:"credentials"`
		Password    string `json:"password"`
		NewPassword string `json:"newPassword"`
	}
	if e := decode(w, r, &a); e != nil {
		reply(w, nil, e)
		return
	}
	var result any
	var err error
	id := number(a.ID, 0)
	if strings.HasPrefix(a.Action, "playlist-") && ws.Playlists == nil {
		reply(w, nil, errors.New("playlist service unavailable"))
		return
	}
	switch a.Action {
	case "playlist-save":
		if a.Subscription == nil {
			err = errors.New("subscription required")
		} else {
			result, err = ws.Playlists.Save(ctx, *a.Subscription)
		}
	case "playlist-sync":
		err = ws.Playlists.Sync(ctx, a.ID)
	case "playlist-pause", "playlist-resume":
		err = ws.Playlists.SetEnabled(ctx, a.ID, a.Action == "playlist-resume")
	case "playlist-backfill":
		if !a.Confirm {
			err = errors.New("confirmation required")
		} else {
			err = ws.Playlists.Backfill(ctx, a.ID)
		}
	case "playlist-retry":
		err = ws.Playlists.Retry(ctx, a.ID, a.Value)

	case "submit":
		if s.firstRun {
			err = errors.New("complete setup first")
			break
		}
		req := core.Request{Mode: s.settings.Mode, Quality: s.settings.Quality, Audio: s.settings.Audio, Out: s.settings.Out, Cookies: s.settings.Browser, Codec: s.settings.Codec, Container: s.settings.Container, RateLimitBPS: s.settings.JobRateLimitBPS}
		if a.Request != nil {
			req = *a.Request
		}
		if len(a.URLs) == 0 || len(a.URLs) > 500 {
			err = errors.New("provide 1..500 URLs")
			break
		}
		// Validate the entire batch before submitting any request.
		for _, u := range a.URLs {
			req.URL = strings.TrimSpace(u)
			if err = core.Validate(req); err != nil {
				break
			}
		}
		if err != nil {
			break
		}
		results := []map[string]any{}
		for _, u := range a.URLs {
			req.URL = strings.TrimSpace(u)
			j, e := ws.Queue.Submit(req, a.Choose)
			entry := map[string]any{"url": u, "id": j.ID}
			if e != nil {
				entry["error"] = e.Error()
			}
			results = append(results, entry)
		}
		result = results
	case "choose":
		err = ws.Queue.Choose(id, a.Value)
	case "retry":
		err = ws.Queue.RetryWithOptions(id, application.RetryOptions{Browser: &s.settings.Browser})
	case "cancel":
		err = ws.Queue.Cancel(id)
	case "remove":
		if !a.Confirm {
			err = errors.New("confirmation required")
		} else {
			err = ws.Queue.Remove(id)
		}
	case "pause", "resume":
		err = ws.Queue.PausePending(id, a.Action == "pause")
	case "priority":
		err = ws.Queue.SetPriority(id, number(a.Value, 9))
	case "reorder":
		err = ws.Queue.Reorder(id, a.Value)
	case "bulk":
		if a.Value == "remove" && !a.Confirm {
			err = errors.New("confirmation required")
		} else {
			result = ws.Queue.BulkActionWithOptions(a.IDs, a.Value, application.RetryOptions{Browser: &s.settings.Browser})
		}
	case "scan":
		path := a.Path
		if path == "" {
			path = s.settings.Out
		}
		err = ws.Library.StartScan(context.WithoutCancel(ctx), path, a.Recursive)
	case "import":
		result, err = ws.Library.Import(ctx, a.Path)
	case "refresh":
		result, err = ws.Library.Refresh(ctx, a.ID)
	case "delete-file":
		err = ws.Library.DeleteFile(ctx, a.ID, a.Confirm)
	case "open", "reveal":
		var path string
		if a.ID == "output" {
			path = s.settings.Out
		} else if id > 0 && a.Value == "job" {
			for _, j := range ws.Queue.Jobs() {
				if j.ID == id && j.State == application.Done {
					path = j.OutputPath
					break
				}
			}
		} else {
			var asset core.Asset
			asset, err = ws.Library.Get(ctx, a.ID)
			path = asset.Path
		}
		if err == nil {
			if path == "" {
				err = errors.New("file unavailable")
			} else if s.openMedia == nil {
				err = errors.New("desktop opening unavailable")
			} else {
				err = s.openMedia(ctx, path, a.Action == "reveal")
			}
		}
	case "unlock":
		err = ws.Cloud.Unlock(ctx, []byte(a.Password))
	case "initialize":
		err = ws.Cloud.Initialize(ctx, []byte(a.Password))
	case "lock":
		ws.Cloud.Lock()
	case "rotate":
		err = ws.Cloud.Rotate(ctx, []byte(a.Password), []byte(a.NewPassword))
	case "reset":
		err = ws.Cloud.Reset(ctx, a.Confirm)
	case "save-profile":
		var c *core.Credentials
		if a.Credentials != nil {
			c = &core.Credentials{AccessKey: a.Credentials.AccessKey, SecretKey: a.Credentials.SecretKey, SessionToken: a.Credentials.SessionToken}
		}
		err = ws.Cloud.SaveProfile(ctx, a.Profile, c)
	case "delete-profile":
		if !a.Confirm {
			err = errors.New("confirmation required")
		} else {
			err = ws.Cloud.DeleteProfile(ctx, a.ID)
		}
	case "test-profile", "test-write":
		if a.Action == "test-write" && !a.Confirm {
			err = errors.New("confirmation required")
		} else {
			err = ws.Cloud.Test(ctx, a.ID, a.Action == "test-write")
		}
	case "preview-upload":
		ids := a.AssetIDs
		if len(ids) == 0 {
			ids = []string{a.ID}
		}
		if len(ids) > 500 {
			err = errors.New("too many assets")
			break
		}
		profiles, e := ws.Cloud.Profiles(ctx)
		if e != nil {
			err = e
			break
		}
		entries := []map[string]any{}
		for _, id := range ids {
			asset, e := ws.Library.Get(ctx, id)
			if e != nil {
				err = e
				break
			}
			keys := map[string]string{}
			for _, p := range profiles {
				keys[p.ID] = core.DestinationKey(p, asset)
			}
			entries = append(entries, map[string]any{"asset": asset, "keys": keys})
		}
		result = entries
	case "library-bulk":
		if len(a.AssetIDs) == 0 || len(a.AssetIDs) > 500 {
			err = errors.New("provide 1..500 asset IDs")
			break
		}
		if a.Value == "delete-file" && !a.Confirm {
			err = errors.New("confirmation required")
			break
		}
		entries := []map[string]string{}
		for _, id := range a.AssetIDs {
			var e error
			switch a.Value {
			case "delete-file":
				e = ws.Library.DeleteFile(ctx, id, a.Confirm)
			case "refresh":
				_, e = ws.Library.Refresh(ctx, id)
			case "upload":
				_, e = ws.Cloud.Enqueue(ctx, id, a.Profiles)
			default:
				e = errors.New("unknown library action")
			}
			entry := map[string]string{"id": id}
			if e != nil {
				entry["error"] = e.Error()
			}
			entries = append(entries, entry)
		}
		result = entries
	case "upload":
		result, err = ws.Cloud.Enqueue(ctx, a.ID, a.Profiles)
	case "retry-upload":
		err = ws.Cloud.Retry(ctx, a.ID, a.Confirm)
	case "cancel-upload":
		err = ws.Cloud.Cancel(ctx, a.ID)
	case "cleanup-upload":
		if !a.Confirm {
			err = errors.New("confirmation required")
		} else {
			err = ws.Cloud.Cleanup(ctx, a.ID)
		}
	case "quit":
		if a.Confirm && s.quit != nil {
			go s.quit()
		} else {
			err = errors.New("confirmation required")
		}
	default:
		err = errors.New("unknown action")
	}
	reply(w, result, err)
}

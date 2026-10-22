// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package core

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var playlistIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{10,150}$`)
var videoIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

func YouTubeVideoID(id string) bool { return videoIDPattern.MatchString(id) }
func PlaylistID(value string) (string, error) {
	id := strings.TrimSpace(value)
	if strings.Contains(id, "://") {
		u, e := url.Parse(id)
		if e != nil || u.User != nil || u.Scheme != "https" {
			return "", fmt.Errorf("use an HTTPS YouTube playlist URL")
		}
		host := strings.ToLower(u.Hostname())
		if host != "youtube.com" && host != "www.youtube.com" && host != "m.youtube.com" && host != "music.youtube.com" {
			return "", fmt.Errorf("YouTube playlist URL required")
		}
		id = u.Query().Get("list")
	}
	if !playlistIDPattern.MatchString(id) || !(strings.HasPrefix(id, "PL") || strings.HasPrefix(id, "UU") || strings.HasPrefix(id, "FL") || strings.HasPrefix(id, "OLAK5uy_")) {
		return "", fmt.Errorf("a persistent YouTube playlist ID is required; mixes and watch-context lists are unsupported")
	}
	return id, nil
}

type PlaylistEntry struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Availability string `json:"availability,omitempty"`
}
type PlaylistScan struct {
	PlaylistID string
	Browser    string
}

// Enumerate streams stable video identities. It returns nil only after a complete
// enumeration; unavailable identified entries are observations, never downloads.
type PlaylistSource interface {
	Enumerate(context.Context, PlaylistScan, func([]PlaylistEntry) error) error
}
type PlaylistSubscription struct {
	ID              string    `json:"id"`
	PlaylistID      string    `json:"playlist_id"`
	Name            string    `json:"name"`
	Revision        int       `json:"revision"`
	Enabled         bool      `json:"enabled"`
	InitialMode     string    `json:"initial_mode"`
	IntervalMinutes int       `json:"interval_minutes"`
	Request         Request   `json:"request"`
	S3Profiles      []string  `json:"s3_profiles"`
	Initialized     bool      `json:"initialized"`
	Status          string    `json:"status"`
	LastError       string    `json:"last_error,omitempty"`
	LastScan        time.Time `json:"last_scan"`
	NextScan        time.Time `json:"next_scan"`
	LastAdmission   time.Time `json:"-"`
	Failures        int       `json:"failures"`
	Scanned         int       `json:"scanned"`
	Created         time.Time `json:"created"`
}
type PlaylistPage struct {
	Items []PlaylistSubscription `json:"items"`
	Total int                    `json:"total"`
}
type PlaylistItem struct {
	SubscriptionID string      `json:"subscription_id"`
	VideoID        string      `json:"video_id"`
	Title          string      `json:"title"`
	State          string      `json:"state"`
	Eligible       bool        `json:"eligible"`
	JobID          int         `json:"job_id"`
	AssetID        string      `json:"asset_id,omitempty"`
	LastError      string      `json:"last_error,omitempty"`
	LocalState     string      `json:"local_state"`
	Deliveries     []UploadJob `json:"deliveries"`
}
type PlaylistItemPage struct {
	Items []PlaylistItem `json:"items"`
	Total int            `json:"total"`
}

func ValidateSubscription(p PlaylistSubscription) (PlaylistSubscription, error) {
	id, e := PlaylistID(p.PlaylistID)
	if e != nil {
		return p, e
	}
	p.PlaylistID = id
	if p.InitialMode != "new_only" && p.InitialMode != "all_existing" {
		return p, fmt.Errorf("initial mode: new_only/all_existing")
	}
	if p.IntervalMinutes < 5 || p.IntervalMinutes > 10080 {
		return p, fmt.Errorf("check interval must be 5..10080 minutes")
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		p.Name = id
	}
	if len(p.Name) > 300 {
		return p, fmt.Errorf("playlist name is too long")
	}
	p.Request.URL = "https://www.youtube.com/watch?v=AAAAAAAAAAA"
	p.Request.Duration = 0
	p.Request.ExpectedParts = 0
	p.Request, e = NormalizeRequest(p.Request)
	if e != nil {
		return p, e
	}
	seen := map[string]bool{}
	targets := []string{}
	for _, id := range p.S3Profiles {
		if id == "" {
			return p, fmt.Errorf("empty S3 profile ID")
		}
		if !seen[id] {
			targets = append(targets, id)
			seen[id] = true
		}
	}
	p.S3Profiles = targets
	return p, nil
}

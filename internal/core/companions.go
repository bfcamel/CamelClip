// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package core

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

type CompanionPolicy struct {
	CacheBytes     int64    `json:"cache_bytes"`
	CacheHours     int      `json:"cache_hours"`
	KeepLoaded     bool     `json:"keep_loaded"`
	AllowAutomatic bool     `json:"allow_automatic"`
	Languages      []string `json:"languages"`
}

func DefaultCompanionPolicy() CompanionPolicy {
	return CompanionPolicy{CacheBytes: 512 << 20, CacheHours: 168, KeepLoaded: true, AllowAutomatic: true, Languages: []string{"original", "interface"}}
}
func (p CompanionPolicy) Validate() error {
	if p.CacheBytes < 1<<20 || p.CacheBytes > 20<<30 || p.CacheHours < 1 || p.CacheHours > 8760 || len(p.Languages) > 16 {
		return &Failure{Code: CodeValidation, Message: "invalid companion cache/language policy"}
	}
	for _, l := range p.Languages {
		if len(l) > 64 || strings.ContainsAny(l, "\x00\r\n") {
			return &Failure{Code: CodeValidation, Message: "invalid caption language"}
		}
	}
	return nil
}

type CompanionRequest struct {
	Source  ClipSourceRef `json:"source"`
	Browser string        `json:"browser,omitempty"`
}
type CaptionDescriptor struct {
	ID          string `json:"id"`
	Language    string `json:"language"`
	Name        string `json:"name"`
	Origin      string `json:"origin"`
	Translated  string `json:"translated"`
	Format      string `json:"format"`
	StreamIndex int    `json:"stream_index,omitempty"`
}
type CompanionCatalog struct {
	SchemaVersion    int                 `json:"schema_version"`
	ID               string              `json:"id"`
	Scope            string              `json:"-"`
	Source           ClipSource          `json:"source"`
	Posters          bool                `json:"posters"`
	Tracks           []CaptionDescriptor `json:"tracks"`
	OriginalLanguage string              `json:"original_language,omitempty"`
	Created          time.Time           `json:"created"`
	Expires          time.Time           `json:"expires"`
	Local            bool                `json:"local"`
	Truncated        bool                `json:"truncated"`
	Request          CompanionRequest    `json:"-"`
	AuthSession      string              `json:"-"`
}
type CompanionBlob struct {
	ID      string    `json:"id"`
	Scope   string    `json:"-"`
	SHA256  string    `json:"sha256"`
	Kind    string    `json:"kind"`
	Ext     string    `json:"ext"`
	MIME    string    `json:"mime"`
	Size    int64     `json:"size"`
	Created time.Time `json:"created"`
	Used    time.Time `json:"used"`
}
type CaptionCue struct {
	Index   int    `json:"index"`
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"`
}
type CaptionSnapshot struct {
	SchemaVersion int               `json:"schema_version"`
	ID            string            `json:"id"`
	CatalogID     string            `json:"catalog_id"`
	Track         CaptionDescriptor `json:"track"`
	RawID         string            `json:"raw_id"`
	CuesID        string            `json:"cues_id"`
	CueCount      int               `json:"cue_count"`
	Timing        string            `json:"timing"`
	Normalization string            `json:"normalization"`
	Created       time.Time         `json:"created"`
}
type CaptionPage struct {
	SchemaVersion int             `json:"schema_version"`
	Snapshot      CaptionSnapshot `json:"snapshot"`
	Items         []CaptionCue    `json:"items"`
	Total         int             `json:"total"`
	NextCursor    string          `json:"next_cursor,omitempty"`
}

// CompanionBackend supplies facts and private bytes, never UI or library jobs.
type CompanionBackend interface {
	Discover(context.Context, CompanionRequest, string) (CompanionCatalog, error)
	Poster(context.Context, CompanionCatalog, string) ([]byte, error)
	Caption(context.Context, CompanionCatalog, CaptionDescriptor, string) ([]byte, error)
}
type CompanionRead interface{ io.ReadSeekCloser }
type CompanionCacheStats struct {
	SchemaVersion int   `json:"schema_version"`
	CachedBytes   int64 `json:"cached_bytes"`
	KeptBytes     int64 `json:"kept_bytes"`
	Blobs         int   `json:"blobs"`
	Removed       int   `json:"removed"`
}

func SelectCaption(c CompanionCatalog, p CompanionPolicy, language string) *CaptionDescriptor {
	for _, preferred := range p.Languages {
		if preferred == "original" {
			preferred = c.OriginalLanguage
		}
		if preferred == "interface" {
			preferred = language
		}
		if preferred == "" {
			continue
		}
		for _, exact := range []bool{true, false} {
			for _, origin := range []string{"manual", "embedded", "imported", "automatic"} {
				if origin == "automatic" && !p.AllowAutomatic {
					continue
				}
				for _, t := range c.Tracks {
					match := strings.EqualFold(t.Language, preferred)
					if !exact {
						match = strings.EqualFold(strings.Split(t.Language, "-")[0], strings.Split(preferred, "-")[0])
					}
					if match && t.Origin == origin && t.Translated != "true" {
						v := t
						return &v
					}
				}
			}
		}
	}
	return nil
}
func CaptionInterval(cues []CaptionCue, first, last int, duration int64) (ClipRange, error) {
	if first < 0 || last < first || last >= len(cues) {
		return ClipRange{}, &Failure{Code: CodeValidation, Message: "select a contiguous valid cue interval"}
	}
	r := ClipRange{StartMS: cues[first].StartMS, EndMS: cues[first].EndMS}
	for _, c := range cues[first : last+1] {
		r.StartMS = min(r.StartMS, c.StartMS)
		r.EndMS = max(r.EndMS, c.EndMS)
	}
	if e := ValidateClipRange(r, duration); e != nil {
		return r, e
	}
	return r, nil
}
func TrimCaptionCues(cues []CaptionCue, r ClipRange) ([]CaptionCue, error) {
	if r.StartMS < 0 || r.EndMS <= r.StartMS {
		return nil, &Failure{Code: CodeValidation, Message: "invalid caption trim interval"}
	}
	v := []CaptionCue{}
	for _, c := range cues {
		if c.StartMS < r.EndMS && c.EndMS > r.StartMS {
			c.Index = len(v)
			c.StartMS = max(c.StartMS, r.StartMS) - r.StartMS
			c.EndMS = min(c.EndMS, r.EndMS) - r.StartMS
			v = append(v, c)
		}
	}
	return v, nil
}
func RenderWebVTT(cues []CaptionCue) []byte {
	var s strings.Builder
	s.WriteString("WEBVTT\n\n")
	for _, c := range cues {
		fmt.Fprintf(&s, "%d\n%s --> %s\n%s\n\n", c.Index+1, captionTime(c.StartMS), captionTime(c.EndMS), strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(c.Text))
	}
	return []byte(s.String())
}
func captionTime(ms int64) string {
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

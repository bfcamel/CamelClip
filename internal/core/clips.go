// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const ClipPipelineVersion = "precise-1"
const ClipMaxDurationMS int64 = 7 * 24 * 60 * 60 * 1000

type ClipRange struct {
	StartMS int64 `json:"start_ms"`
	EndMS   int64 `json:"end_ms"`
}
type ClipSegment struct {
	Title string    `json:"title"`
	Range ClipRange `json:"range"`
}
type ClipSourceRef struct {
	URL     string `json:"url,omitempty"`
	AssetID string `json:"asset_id,omitempty"`
}
type ClipRequest struct {
	ChapterIndices []int         `json:"chapter_indices,omitempty"`
	Source         ClipSourceRef `json:"source"`
	Segments       []ClipSegment `json:"segments"`
	Mode           string        `json:"mode"`
	Audio          string        `json:"audio"`
	Quality        string        `json:"quality"`
	Accuracy       string        `json:"accuracy"`
	OutputDir      string        `json:"output_dir"`
	Browser        string        `json:"browser,omitempty"`
	S3Profiles     []string      `json:"s3_profiles"`
}
type ClipChapter struct {
	Title string    `json:"title"`
	Range ClipRange `json:"range"`
	Index int       `json:"index"`
}
type ClipSource struct {
	Identity   string        `json:"identity"`
	Title      string        `json:"title"`
	DurationMS int64         `json:"duration_ms"`
	SHA256     string        `json:"sha256,omitempty"`
	Chapters   []ClipChapter `json:"chapters"`
	Width      int           `json:"width"`
	Height     int           `json:"height"`
	Video      bool          `json:"video"`
	Audio      bool          `json:"audio"`
}
type ClipPlan struct {
	ID           string      `json:"id"`
	Revision     int64       `json:"revision"`
	Digest       string      `json:"digest"`
	Created      time.Time   `json:"created"`
	Expires      time.Time   `json:"expires"`
	Request      ClipRequest `json:"request"`
	Source       ClipSource  `json:"source"`
	Destinations []S3Profile `json:"destinations"`
	Warnings     []string    `json:"warnings"`
}
type ClipState string

const (
	ClipQueued            ClipState = "queued"
	ClipWaitingSource     ClipState = "waiting_source"
	ClipDownloading       ClipState = "downloading"
	ClipWaitingProcessing ClipState = "waiting_processing"
	ClipRendering         ClipState = "rendering"
	ClipVerifying         ClipState = "verifying"
	ClipPublishing        ClipState = "publishing"
	ClipDone              ClipState = "done"
	ClipFailed            ClipState = "failed"
	ClipCanceled          ClipState = "canceled"
)

func (s ClipState) Terminal() bool { return s == ClipDone || s == ClipFailed || s == ClipCanceled }

type ClipEvidence struct {
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	VideoCodec      string  `json:"video_codec,omitempty"`
	AudioCodec      string  `json:"audio_codec"`
	SourceOrigin    float64 `json:"source_origin"`
	FirstAudioPTS   float64 `json:"first_audio_pts"`
	LastAudioPTS    float64 `json:"last_audio_pts"`
	AudioSamples    int64   `json:"audio_samples"`
	SampleRate      int64   `json:"sample_rate"`
	AVOffsetErrorMS float64 `json:"av_offset_error_ms"`
	Pipeline        string  `json:"pipeline"`
	Toolchain       string  `json:"toolchain"`
	FirstSourcePTS  float64 `json:"first_source_pts"`
	LastSourcePTS   float64 `json:"last_source_pts"`
	Frames          int64   `json:"frames"`
	OutputFrames    int64   `json:"output_frames"`
	DurationMS      int64   `json:"duration_ms"`
	SHA256          string  `json:"sha256"`
	Size            int64   `json:"size"`
}
type ClipTask struct {
	ID        string        `json:"id"`
	Ordinal   int           `json:"ordinal"`
	Segment   ClipSegment   `json:"segment"`
	State     ClipState     `json:"state"`
	Attempt   int           `json:"attempt"`
	Error     string        `json:"error,omitempty"`
	ErrorCode ErrorCode     `json:"error_code,omitempty"`
	Metrics   Metrics       `json:"metrics"`
	AssetID   string        `json:"asset_id,omitempty"`
	Candidate string        `json:"-"`
	FinalPath string        `json:"path,omitempty"`
	Evidence  *ClipEvidence `json:"evidence,omitempty"`
}
type ClipBatch struct {
	ID              string     `json:"id"`
	Revision        int64      `json:"revision"`
	ControlRevision int64      `json:"control_revision"`
	Key             string     `json:"idempotency_key"`
	Plan            ClipPlan   `json:"plan"`
	Tasks           []ClipTask `json:"tasks"`
	Paused          bool       `json:"paused"`
	CancelRequested bool       `json:"cancel_requested"`
	SourceState     ClipState  `json:"source_state"`
	SourceMetrics   Metrics    `json:"source_metrics"`
	Prepared        ClipSource `json:"prepared"`
	SourceError     string     `json:"source_error,omitempty"`
	SourcePath      string     `json:"-"`
	Created         time.Time  `json:"created"`
	Updated         time.Time  `json:"updated"`
}

func (b ClipBatch) Settled() bool {
	for _, t := range b.Tasks {
		if !t.State.Terminal() {
			return false
		}
	}
	return true
}
func (b ClipBatch) Status() string {
	done, failed, canceled := 0, 0, 0
	for _, t := range b.Tasks {
		switch t.State {
		case ClipDone:
			done++
		case ClipFailed:
			failed++
		case ClipCanceled:
			canceled++
		}
	}
	if b.Settled() {
		if done == len(b.Tasks) {
			return "completed"
		}
		if b.CancelRequested || canceled > 0 && failed == 0 {
			return "canceled"
		}
		if done > 0 {
			return "partial_failure"
		}
		return "failed"
	}
	if b.CancelRequested {
		return "canceling"
	}
	if b.Paused {
		return "paused"
	}
	return "running"
}

type ClipBatchPage struct {
	Items      []ClipBatch `json:"items"`
	Total      int         `json:"total"`
	NextCursor string      `json:"next_cursor,omitempty"`
}
type ClipProgress struct {
	State   ClipState
	Metrics Metrics
}

// ClipBackend produces private sources and candidates. It never publishes assets.
// Callbacks are serialized and joined before return. All processes honor context.
type ClipBackend interface {
	Inspect(ctx context.Context, request ClipRequest, localPath string) (ClipSource, error)
	Prepare(ctx context.Context, plan ClipPlan, localPath, workDir string, observer func(ClipProgress)) (string, ClipSource, error)
	Render(ctx context.Context, sourcePath, candidate string, plan ClipPlan, segment ClipSegment, observer func(ClipProgress)) (ClipEvidence, error)
}

var clipTime = regexp.MustCompile(`^[0-9]+(?:\.[0-9]{1,3})?$`)

func ParseClipTime(s string) (int64, error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) > 3 || len(parts) == 0 {
		return 0, clipInvalid("invalid time")
	}
	var total int64
	for i, p := range parts {
		if !clipTime.MatchString(p) || i < len(parts)-1 && strings.Contains(p, ".") {
			return 0, clipInvalid("use seconds, MM:SS or HH:MM:SS with up to three decimals")
		}
		pair := strings.SplitN(p, ".", 2)
		n, e := strconv.ParseInt(pair[0], 10, 64)
		if e != nil || n > ClipMaxDurationMS/1000 || i > 0 && n > 59 {
			return 0, clipInvalid("invalid time field")
		}
		if total > ClipMaxDurationMS/60000 {
			return 0, clipInvalid("time exceeds seven days")
		}
		total = total*60 + n
		if i == len(parts)-1 {
			total *= 1000
			if len(pair) == 2 {
				frac, _ := strconv.ParseInt(pair[1]+strings.Repeat("0", 3-len(pair[1])), 10, 64)
				total += frac
			}
		}
	}
	if total > ClipMaxDurationMS {
		return 0, clipInvalid("time exceeds seven days")
	}
	return total, nil
}
func ParseClipRange(s string) (ClipRange, error) {
	p := strings.Split(s, "..")
	if len(p) != 2 {
		return ClipRange{}, clipInvalid("range must be START..END")
	}
	a, e := ParseClipTime(p[0])
	if e != nil {
		return ClipRange{}, e
	}
	b, e := ParseClipTime(p[1])
	if e != nil {
		return ClipRange{}, e
	}
	r := ClipRange{a, b}
	return r, ValidateClipRange(r, ClipMaxDurationMS)
}
func clipInvalid(s string) error { return &Failure{Code: CodeValidation, Message: s} }
func ValidateClipRange(r ClipRange, duration int64) error {
	if r.StartMS < 0 || r.EndMS > duration || r.EndMS <= r.StartMS || r.EndMS-r.StartMS < 1000 {
		return clipInvalid("clip range must be within the finite source and at least one second long")
	}
	return nil
}
func NormalizeClipRequest(r ClipRequest) (ClipRequest, error) {
	if (r.Source.URL == "") == (r.Source.AssetID == "") {
		return r, clipInvalid("choose exactly one URL or library asset")
	}
	if r.Source.URL != "" {
		u, e := url.Parse(strings.TrimSpace(r.Source.URL))
		if e != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return r, clipInvalid("a valid HTTP(S) source without embedded credentials is required")
		}
		r.Source.URL = u.String()
	}
	if (len(r.Segments) < 1 && len(r.ChapterIndices) == 0) || len(r.Segments)+len(r.ChapterIndices) > 100 {
		return r, clipInvalid("provide 1..100 clip ranges")
	}
	if r.Mode == "" {
		r.Mode = "video"
	}
	if r.Mode != "video" && r.Mode != "audio" {
		return r, clipInvalid("mode must be video or audio")
	}
	if r.Audio == "" {
		r.Audio = "m4a"
	}
	if !contains([]string{"m4a", "mp3", "opus", "flac", "wav"}, r.Audio) {
		return r, clipInvalid("unsupported clip audio profile")
	}
	if r.Quality == "" {
		r.Quality = "best"
	}
	if r.Quality != "best" {
		h, e := strconv.Atoi(r.Quality)
		if e != nil || h < 1 || h > 16384 {
			return r, clipInvalid("invalid height")
		}
	}
	if r.Accuracy == "" {
		r.Accuracy = "precise"
	}
	if r.Accuracy != "precise" {
		return r, clipInvalid("this increment supports precise clipping only; fast-copy is not available")
	}
	if strings.TrimSpace(r.OutputDir) == "" || strings.ContainsRune(r.OutputDir, 0) {
		return r, clipInvalid("output directory required")
	}
	if r.Browser != "" && !ValidBrowser(r.Browser) {
		return r, clipInvalid("unsupported browser")
	}
	r.Segments = append([]ClipSegment(nil), r.Segments...)
	r.S3Profiles = append([]string(nil), r.S3Profiles...)
	r.ChapterIndices = append([]int(nil), r.ChapterIndices...)
	seen := map[ClipRange]bool{}
	var sum int64
	for i := range r.Segments {
		s := &r.Segments[i]
		s.Title = strings.TrimSpace(s.Title)
		if !utf8.ValidString(s.Title) || utf8.RuneCountInString(s.Title) > 200 || len(s.Title) > 800 {
			return r, clipInvalid("clip title exceeds 200 characters or 800 bytes")
		}
		if e := ValidateClipRange(s.Range, ClipMaxDurationMS); e != nil {
			return r, fmt.Errorf("segments[%d]: %w", i, e)
		}
		if seen[s.Range] {
			return r, clipInvalid("duplicate clip interval")
		}
		seen[s.Range] = true
		sum += s.Range.EndMS - s.Range.StartMS
	}
	if sum > 24*60*60*1000 {
		return r, clipInvalid("sum of clip durations exceeds 24 hours")
	}
	return r, nil
}
func ClipDigest(v any) string {
	b, _ := json.Marshal(v)
	s := sha256.Sum256(append([]byte("camelclip/clip-v1\x00"), b...))
	return hex.EncodeToString(s[:])
}
func ValidateClipSource(s ClipSource, r ClipRequest) error {
	if s.Identity == "" || s.DurationMS <= 0 || s.DurationMS > ClipMaxDurationMS || !s.Audio || r.Mode == "video" && !s.Video {
		return clipInvalid("source must have a finite supported timeline and required streams")
	}
	for i, seg := range r.Segments {
		if e := ValidateClipRange(seg.Range, s.DurationMS); e != nil {
			return fmt.Errorf("segments[%d]: %w", i, e)
		}
	}
	return nil
}

func (b ClipBatch) MarshalJSON() ([]byte, error) {
	type plain ClipBatch
	return json.Marshal(struct {
		plain
		Status string `json:"status"`
	}{plain(b), b.Status()})
}

// ParseClipSegments is shared by text entry in GUI, TUI and CLI manifests.
func ParseClipSegments(text string) ([]ClipSegment, error) {
	segments := []ClipSegment{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		p := strings.SplitN(line, "|", 2)
		r, e := ParseClipRange(strings.TrimSpace(p[0]))
		if e != nil {
			return nil, e
		}
		s := ClipSegment{Range: r}
		if len(p) == 2 {
			s.Title = strings.TrimSpace(p[1])
		}
		segments = append(segments, s)
	}
	if len(segments) < 1 || len(segments) > 100 {
		return nil, clipInvalid("provide 1..100 ranges")
	}
	return segments, nil
}

func ResolveClipChapters(r ClipRequest, s ClipSource) (ClipRequest, error) {
	for _, index := range r.ChapterIndices {
		found := false
		for _, c := range s.Chapters {
			if c.Index == index {
				r.Segments = append(r.Segments, ClipSegment{Title: c.Title, Range: c.Range})
				found = true
				break
			}
		}
		if !found {
			return r, clipInvalid("selected chapter is unavailable")
		}
	}
	r.ChapterIndices = nil
	return NormalizeClipRequest(r)
}

func (s ClipState) Valid() bool {
	switch s {
	case ClipQueued, ClipWaitingSource, ClipDownloading, ClipWaitingProcessing, ClipRendering, ClipVerifying, ClipPublishing, ClipDone, ClipFailed, ClipCanceled:
		return true
	}
	return false
}
func ValidateClipBatch(b ClipBatch) error {
	if b.ID == "" || b.Key == "" || b.Revision < 1 || b.ControlRevision < 1 || b.Plan.Digest == "" || len(b.Tasks) != len(b.Plan.Request.Segments) {
		return ContractError("invalid persisted clip batch")
	}
	if _, e := NormalizeClipRequest(b.Plan.Request); e != nil {
		return e
	}
	if e := ValidateClipSource(b.Plan.Source, b.Plan.Request); e != nil {
		return e
	}
	seen := map[string]bool{}
	for i, t := range b.Tasks {
		if t.ID == "" || seen[t.ID] || t.Ordinal != i || t.Segment != b.Plan.Request.Segments[i] || !t.State.Valid() || t.Attempt < 0 {
			return ContractError("invalid persisted clip task")
		}
		seen[t.ID] = true
		if e := ValidateMetrics(t.Metrics); e != nil {
			return e
		}
		if t.State == ClipDone && (t.AssetID == "" || t.Evidence == nil || t.Evidence.SHA256 == "" || t.FinalPath == "") {
			return ContractError("completed clip lacks publication evidence")
		}
	}
	return nil
}

func ValidateClipEvidence(r ClipRequest, seg ClipSegment, e ClipEvidence) error {
	digest, err := hex.DecodeString(e.SHA256)
	if err != nil || len(digest) != 32 || e.Size <= 0 || e.DurationMS <= 0 || e.Pipeline != ClipPipelineVersion {
		return ContractError("clip lacks valid verification evidence")
	}
	for _, v := range []float64{e.SourceOrigin, e.FirstSourcePTS, e.LastSourcePTS, e.FirstAudioPTS, e.LastAudioPTS, e.AVOffsetErrorMS} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return ContractError("nonfinite clip timeline evidence")
		}
	}
	start, end := float64(seg.Range.StartMS)/1000, float64(seg.Range.EndMS)/1000
	if e.AudioSamples <= 0 || e.SampleRate <= 0 || e.FirstAudioPTS < start-0.001 || e.LastAudioPTS >= end+0.001 || e.LastAudioPTS < e.FirstAudioPTS {
		return ContractError("clip audio boundary evidence is invalid")
	}
	if r.Mode == "video" && (e.Width <= 0 || e.Height <= 0 || e.VideoCodec != "h264" || e.AudioCodec != "aac" || e.Frames <= 0 || e.Frames != e.OutputFrames || e.FirstSourcePTS < start-0.001 || e.LastSourcePTS >= end+0.001 || e.LastSourcePTS < e.FirstSourcePTS) {
		return ContractError("clip video boundary evidence is invalid")
	}
	return nil
}

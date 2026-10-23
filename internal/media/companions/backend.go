// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package companions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Backend struct {
	YTDLP, FFmpeg, FFprobe string
	Inspector              core.ClipBackend
}

func New(inspector core.ClipBackend) *Backend {
	return &Backend{YTDLP: "yt-dlp", FFmpeg: "ffmpeg", FFprobe: "ffprobe", Inspector: inspector}
}

type bounded struct {
	bytes.Buffer
	max int
}

func (b *bounded) Write(p []byte) (int, error) {
	if len(p) > b.max-b.Len() {
		return 0, errors.New("companion output exceeds limit")
	}
	return b.Buffer.Write(p)
}
func run(ctx context.Context, name string, args []string, max int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	out := &bounded{max: max}
	cmd.Stdout = out
	cmd.Stderr = &bounded{max: 64 << 10}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("companion operation failed (%s)", filepath.Base(name))
	}
	return out.Bytes(), nil
}

// runInDir bounds private temporary output while the child is running.
func runInDir(ctx context.Context, name string, args []string, dir string, max int64) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	finished := make(chan struct{})
	joined := make(chan struct{})
	var exceeded bool
	go func() {
		defer close(joined)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-finished:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				var total int64
				_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if !entry.IsDir() {
						info, e := entry.Info()
						if e != nil {
							return e
						}
						total += info.Size()
					}
					return nil
				})
				if total > max {
					exceeded = true
					cancel()
					return
				}
			}
		}
	}()
	data, err := run(ctx, name, args, 1<<20)
	close(finished)
	<-joined
	if exceeded {
		return nil, errors.New("companion temporary storage exceeds limit")
	}
	return data, err
}

type metadata struct {
	ID, Extractor, Title, Language string
	Thumbnail                      string
	Subtitles, Automatic           map[string][]struct{ Ext, Name, URL string } `json:"-"`
}

func (b *Backend) metadata(ctx context.Context, r core.CompanionRequest) (metadata, error) {
	args := []string{"--ignore-config", "--no-playlist", "--skip-download", "--dump-single-json", "--no-warnings"}
	if r.Browser != "" {
		args = append(args, "--cookies-from-browser", r.Browser)
	}
	args = append(args, "--", r.Source.URL)
	data, err := run(ctx, b.YTDLP, args, 4<<20)
	if err != nil {
		return metadata{}, err
	}
	var v struct {
		ID, Extractor, Title, Language, Thumbnail string
		Subtitles                                 map[string][]struct{ Ext, Name, URL string }
		Automatic                                 map[string][]struct{ Ext, Name, URL string } `json:"automatic_captions"`
	}
	err = json.Unmarshal(data, &v)
	return metadata{ID: v.ID, Extractor: v.Extractor, Title: v.Title, Language: v.Language, Thumbnail: v.Thumbnail, Subtitles: v.Subtitles, Automatic: v.Automatic}, err
}
func tracks(m metadata) []core.CaptionDescriptor {
	result := []core.CaptionDescriptor{}
	for _, group := range []struct {
		origin string
		items  map[string][]struct{ Ext, Name, URL string }
	}{{"manual", m.Subtitles}, {"automatic", m.Automatic}} {
		for lang, formats := range group.items {
			if lang == "live_chat" {
				continue
			}
			for _, v := range formats {
				if v.Ext != "vtt" {
					continue
				}
				translated := "unknown"
				if strings.Contains(v.URL, "tlang=") {
					translated = "true"
				}
				if group.origin == "manual" {
					translated = "false"
				}
				result = append(result, core.CaptionDescriptor{ID: core.ClipDigest([]string{group.origin, lang, "vtt"}), Language: lang, Name: v.Name, Origin: group.origin, Translated: translated, Format: "vtt"})
				break
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
func (b *Backend) Discover(ctx context.Context, r core.CompanionRequest, local string) (core.CompanionCatalog, error) {
	c := core.CompanionCatalog{Tracks: []core.CaptionDescriptor{}, Local: local != ""}
	source, err := b.Inspector.Inspect(ctx, core.ClipRequest{Source: r.Source, Browser: r.Browser, Mode: "video"}, local)
	if err != nil {
		return c, err
	}
	c.Source = source
	if local == "" {
		m, e := b.metadata(ctx, r)
		if e != nil {
			return c, e
		}
		c.Posters = m.Thumbnail != ""
		c.OriginalLanguage = m.Language
		c.Tracks = tracks(m)
	} else {
		data, e := run(ctx, b.FFprobe, []string{"-v", "error", "-show_streams", "-of", "json", local}, 4<<20)
		if e != nil {
			return c, e
		}
		var p struct {
			Streams []struct {
				Index int
				Type  string `json:"codec_type"`
				Codec string `json:"codec_name"`
				Tags  struct{ Language, Title string }
			}
		}
		if e = json.Unmarshal(data, &p); e != nil {
			return c, e
		}
		c.Posters = source.Video
		for _, s := range p.Streams {
			if s.Type == "subtitle" && (s.Codec == "subrip" || s.Codec == "webvtt" || s.Codec == "mov_text") {
				c.Tracks = append(c.Tracks, core.CaptionDescriptor{ID: core.ClipDigest([]string{source.Identity, strconv.Itoa(s.Index)}), Language: s.Tags.Language, Name: s.Tags.Title, Origin: "embedded", Translated: "unknown", Format: "vtt", StreamIndex: s.Index})
			}
		}
	}
	if len(c.Tracks) > 200 {
		c.Tracks = c.Tracks[:200]
		c.Truncated = true
	}
	return c, nil
}
func (b *Backend) validate(ctx context.Context, c core.CompanionCatalog, local string) error {
	s, err := b.Inspector.Inspect(ctx, core.ClipRequest{Source: c.Request.Source, Browser: c.Request.Browser, Mode: "video"}, local)
	if err != nil {
		return err
	}
	if s.Identity != c.Source.Identity || s.SHA256 != c.Source.SHA256 || s.DurationMS != c.Source.DurationMS {
		return &core.Failure{Code: core.CodeSourceChanged, Message: "source changed; inspect it again"}
	}
	return nil
}
func (b *Backend) Poster(ctx context.Context, c core.CompanionCatalog, local string) ([]byte, error) {
	stage := core.Downloading
	if local != "" {
		stage = core.Processing
	}
	if e := core.EnterStage(ctx, stage); e != nil {
		return nil, e
	}
	if err := b.validate(ctx, c, local); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "camelclip-poster-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	input := local
	if local == "" {
		args := []string{"--ignore-config", "--no-playlist", "--skip-download", "--write-thumbnail", "--max-filesize", "8M", "-o", filepath.Join(dir, "poster.%(ext)s")}
		if c.Request.Browser != "" {
			args = append(args, "--cookies-from-browser", c.Request.Browser)
		}
		if rate := core.DownloadRate(ctx, 0); rate > 0 {
			args = append(args, "--limit-rate", strconv.FormatInt(rate, 10))
		}
		args = append(args, "--", c.Request.Source.URL)
		if _, err = runInDir(ctx, b.YTDLP, args, dir, 24<<20); err != nil {
			return nil, err
		}
		matches, _ := filepath.Glob(filepath.Join(dir, "poster.*"))
		if len(matches) != 1 {
			return nil, errors.New("poster unavailable")
		}
		input = matches[0]
		info, e := os.Stat(input)
		if e != nil || info.Size() > 8<<20 {
			return nil, errors.New("poster unavailable or too large")
		}
	}
	if local == "" {
		file, e := os.Open(input)
		if e != nil {
			return nil, e
		}
		header := make([]byte, 12)
		n, _ := file.Read(header)
		file.Close()
		header = header[:n]
		jpeg := len(header) >= 3 && bytes.Equal(header[:3], []byte{255, 216, 255})
		png := len(header) >= 8 && bytes.Equal(header[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10})
		webp := len(header) >= 12 && string(header[:4]) == "RIFF" && string(header[8:12]) == "WEBP"
		if !jpeg && !png && !webp {
			return nil, errors.New("unsupported poster format")
		}
	}
	probeData, e := run(ctx, b.FFprobe, []string{"-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "json", input}, 1<<20)
	if e != nil {
		return nil, e
	}
	var dimensions struct {
		Streams []struct{ Width, Height int64 }
	}
	if json.Unmarshal(probeData, &dimensions) != nil || len(dimensions.Streams) != 1 || dimensions.Streams[0].Width <= 0 || dimensions.Streams[0].Height <= 0 || dimensions.Streams[0].Width*dimensions.Streams[0].Height > 20000000 {
		return nil, errors.New("unsupported poster dimensions")
	}
	if e := core.EnterStage(ctx, core.Processing); e != nil {
		return nil, e
	}
	return run(ctx, b.FFmpeg, []string{"-v", "error", "-nostdin", "-i", input, "-frames:v", "1", "-vf", "scale=640:640:force_original_aspect_ratio=decrease", "-f", "image2pipe", "-c:v", "mjpeg", "pipe:1"}, 8<<20)
}
func (b *Backend) Caption(ctx context.Context, c core.CompanionCatalog, t core.CaptionDescriptor, local string) ([]byte, error) {
	stage := core.Downloading
	if local != "" {
		stage = core.Processing
	}
	if e := core.EnterStage(ctx, stage); e != nil {
		return nil, e
	}
	if err := b.validate(ctx, c, local); err != nil {
		return nil, err
	}
	if local != "" {
		return run(ctx, b.FFmpeg, []string{"-v", "error", "-nostdin", "-i", local, "-map", "0:" + strconv.Itoa(t.StreamIndex), "-f", "webvtt", "pipe:1"}, 20<<20)
	}
	m, err := b.metadata(ctx, c.Request)
	if err != nil {
		return nil, err
	}
	found := false
	for _, v := range tracks(m) {
		if v.ID == t.ID {
			found = true
		}
	}
	if !found {
		return nil, errors.New("caption track changed; inspect again")
	}
	dir, err := os.MkdirTemp("", "camelclip-caption-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	flag := "--write-subs"
	if t.Origin == "automatic" {
		flag = "--write-auto-subs"
	}
	// Anchor and escape the language expression: yt-dlp interprets --sub-langs as regex.
	language := "^" + regexp.QuoteMeta(t.Language) + "$"
	args := []string{"--ignore-config", "--no-playlist", "--skip-download", flag, "--sub-langs", language, "--sub-format", "vtt", "-o", filepath.Join(dir, "caption.%(ext)s")}
	if c.Request.Browser != "" {
		args = append(args, "--cookies-from-browser", c.Request.Browser)
	}
	if rate := core.DownloadRate(ctx, 0); rate > 0 {
		args = append(args, "--limit-rate", strconv.FormatInt(rate, 10))
	}
	args = append(args, "--", c.Request.Source.URL)
	if _, err = runInDir(ctx, b.YTDLP, args, dir, 24<<20); err != nil {
		return nil, err
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "caption.*.vtt"))
	if len(matches) != 1 {
		return nil, errors.New("selected caption unavailable")
	}
	file, err := os.Open(matches[0])
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 20<<20+1))
	if len(data) > 20<<20 {
		return nil, errors.New("caption too large")
	}
	return data, err
}

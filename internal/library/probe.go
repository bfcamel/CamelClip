// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package library

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Prober struct{}
type bounded struct{ data []byte }

func (b *bounded) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > 1024*1024 {
		return 0, fmt.Errorf("metadata exceeds limit")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
func (Prober) Probe(ctx context.Context, path string) (core.Asset, error) {
	a := core.Asset{Path: path, Title: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), Container: strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."), MIME: mime.TypeByExtension(filepath.Ext(path)), Status: "available"}
	st, e := os.Lstat(path)
	if e != nil {
		return a, e
	}
	if !st.Mode().IsRegular() {
		return a, fmt.Errorf("not a regular file")
	}
	a.Size = st.Size()
	a.Modified = st.ModTime()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration,format_name:stream=codec_type,codec_name,width,height", "-of", "json", path)
	var out bounded
	c.Stdout = &out
	if e = c.Run(); e != nil {
		return a, fmt.Errorf("media metadata unavailable")
	}
	var data struct {
		Format  struct{ Duration string }
		Streams []struct {
			Type          string `json:"codec_type"`
			Codec         string `json:"codec_name"`
			Width, Height int
		}
	}
	if e = json.Unmarshal(out.data, &data); e != nil {
		return a, e
	}
	a.Duration, _ = strconv.ParseFloat(data.Format.Duration, 64)
	for _, s := range data.Streams {
		if s.Type == "video" {
			a.Kind = "video"
			a.VideoCodec = s.Codec
			a.Width = s.Width
			a.Height = s.Height
		}
		if s.Type == "audio" {
			if a.Kind == "" {
				a.Kind = "audio"
			}
			a.AudioCodec = s.Codec
		}
	}
	if a.Kind == "" || a.Duration <= 0 {
		return a, fmt.Errorf("file is not playable media")
	}
	return a, nil
}

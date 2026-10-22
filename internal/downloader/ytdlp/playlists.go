// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package ytdlp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"io"
	"strings"
)

func (e *Engine) Enumerate(ctx context.Context, r core.PlaylistScan, emit func([]core.PlaylistEntry) error) error {
	id, err := core.PlaylistID(r.PlaylistID)
	if err != nil {
		return err
	}
	args := []string{"--ignore-config", "--flat-playlist", "--lazy-playlist", "--dump-json", "--skip-download", "--yes-playlist", "--no-warnings", "--socket-timeout", "30", "--extractor-retries", "2"}
	if r.Browser != "" {
		if !core.ValidBrowser(r.Browser) {
			return fmt.Errorf("invalid browser")
		}
		args = append(args, "--cookies-from-browser", r.Browser)
	}
	args = append(args, "--", "https://www.youtube.com/playlist?list="+id)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c := command(ctx, e.Executable, args...)
	out, err := c.StdoutPipe()
	if err != nil {
		return err
	}
	var tail tailBuffer
	c.Stderr = &tail
	if err = c.Start(); err != nil {
		return err
	}
	scan := bufio.NewScanner(out)
	scan.Buffer(make([]byte, 64*1024), 1024*1024)
	batch := make([]core.PlaylistEntry, 0, 250)
	var parseErr error
	for scan.Scan() {
		line := scan.Bytes()
		if len(line) == 0 {
			continue
		}
		var item struct {
			ID, Title, Availability string
			Type                    string `json:"_type"`
		}
		if err = json.Unmarshal(line, &item); err != nil {
			parseErr = fmt.Errorf("invalid playlist metadata: %w", err)
			break
		}
		if !core.YouTubeVideoID(item.ID) {
			parseErr = fmt.Errorf("playlist returned an entry without a valid video ID")
			break
		}
		if len(item.Title) > 4096 {
			parseErr = core.ContractError("playlist entry title exceeds limit")
			break
		}
		batch = append(batch, core.PlaylistEntry{ID: item.ID, Title: safeText(item.Title), Availability: item.Availability})
		if len(batch) == 250 {
			if parseErr = emit(batch); parseErr != nil {
				break
			}
			batch = make([]core.PlaylistEntry, 0, 250)
		}
	}
	if parseErr == nil {
		parseErr = scan.Err()
	}
	if parseErr != nil {
		cancel()
		io.Copy(io.Discard, out)
	}
	processErr := c.Wait()
	if parseErr != nil {
		return parseErr
	}
	if ctx.Err() != nil {
		return core.ErrCanceled
	}
	if processErr != nil {
		return backendFailure(fmt.Errorf("playlist enumeration: %w: %s", processErr, strings.TrimSpace(tail.String())))
	}
	if len(batch) > 0 {
		return emit(batch)
	}
	return nil
}

var _ core.PlaylistSource = (*Engine)(nil)

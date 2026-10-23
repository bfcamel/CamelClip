// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package companions

import (
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

var tags = regexp.MustCompile(`<[^>]*>`)

// ParseCaptions retains overlaps, repeated speech and source order. Styling is
// intentionally reduced to plain text; raw bytes are stored separately.
func ParseCaptions(data []byte) ([]core.CaptionCue, error) {
	if len(data) > 20<<20 || !utf8.Valid(data) {
		return nil, fmt.Errorf("caption file exceeds 20 MiB or is not UTF-8")
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if strings.Contains(text, "X-TIMESTAMP-MAP") {
		return nil, fmt.Errorf("mapped caption timelines require explicit synchronization")
	}
	vtt := strings.HasPrefix(text, "WEBVTT")
	if !vtt && !strings.Contains(text, " --> ") {
		return nil, fmt.Errorf("expected WebVTT or SRT captions")
	}
	lines := strings.Split(text, "\n")
	out := []core.CaptionCue{}
	var previous int64 = -1
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if vtt && (line == "NOTE" || strings.HasPrefix(line, "NOTE ") || line == "STYLE" || line == "REGION") {
			for i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
				i++
			}
			continue
		}
		if !strings.Contains(line, "-->") {
			continue
		}
		pair := strings.Split(line, "-->")
		if len(pair) != 2 {
			return nil, fmt.Errorf("invalid caption timing")
		}
		endFields := strings.Fields(pair[1])
		if len(endFields) == 0 {
			return nil, fmt.Errorf("missing cue end")
		}
		start, e := core.ParseClipTime(strings.ReplaceAll(strings.TrimSpace(pair[0]), ",", "."))
		if e != nil {
			return nil, e
		}
		end, e := core.ParseClipTime(strings.ReplaceAll(endFields[0], ",", "."))
		if e != nil || end <= start || start < previous {
			return nil, fmt.Errorf("caption timestamps must be ordered with positive durations")
		}
		previous = start
		var payload []string
		for i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
			i++
			payload = append(payload, lines[i])
		}
		plain := strings.TrimSpace(html.UnescapeString(tags.ReplaceAllString(strings.Join(payload, "\n"), "")))
		if len(plain) > 16<<10 {
			return nil, fmt.Errorf("caption cue exceeds 16 KiB")
		}
		if plain == "" {
			continue
		}
		out = append(out, core.CaptionCue{Index: len(out), StartMS: start, EndMS: end, Text: plain})
		if len(out) > 100000 {
			return nil, fmt.Errorf("caption track exceeds 100000 cues")
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("caption track contains no supported timed text")
	}
	return out, nil
}

// Parser implements the application caption parsing port.
type Parser struct{}

func (Parser) Parse(data []byte) ([]core.CaptionCue, error) { return ParseCaptions(data) }

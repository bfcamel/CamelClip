// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package ytdlp

import "strconv"

func arguments(o options) []string       { return buildArguments(o, false) }
func formatArguments(o options) []string { return buildArguments(o, true) }
func buildArguments(o options, list bool) []string {
	// progressWriter redraws download updates when attached to a terminal.
	// Child output is captured; keep progress visible in both interfaces.
	a := []string{"--ignore-config", "--no-playlist", "--no-overwrites", "--no-quiet", "--progress", "--abort-on-unavailable-fragments", "--fixup", "never", "--newline", "--progress-delta", "0.25", "--progress-template", `download:CAMELCLIP_PROGRESS {"downloaded_bytes":%(progress.downloaded_bytes|0)j,"total_bytes":%(progress.total_bytes,progress.total_bytes_estimate|null)j,"speed":%(progress.speed|null)j,"eta":%(progress.eta|null)j,"filename":%(progress.filename)j}`}
	if o.Cookies != "" {
		a = append(a, "--cookies-from-browser", o.Cookies)
	}
	if list {
		return append(a, "--list-formats", "--", o.URL)
	}
	if o.RateLimitBPS > 0 {
		a = append(a, "--limit-rate", strconv.FormatInt(o.RateLimitBPS, 10), "--http-chunk-size", "256K")
	}
	a = append(a, "--paths", o.Out, "--output", "%(title).120B [%(extractor_key)s-%(id)s] ["+outputVariant(o)+"] [%(format_id)s].%(ext)s")
	if o.Mode == "audio" {
		a = append(a, "--format", "bestaudio/best")
	} else {
		filter := ""
		if o.Quality != "best" {
			filter = "[height<=" + o.Quality + "]"
		}
		// Every fallback respects the height limit and includes audio.
		selector := "bestvideo" + filter + "/best" + filter + ",bestaudio/best" + filter
		a = append(a, "--format", selector)
	}
	return append(a, "--", o.URL)
}

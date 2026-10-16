// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package ytdlp

func arguments(o options) []string       { return buildArguments(o, false) }
func formatArguments(o options) []string { return buildArguments(o, true) }
func buildArguments(o options, list bool) []string {
	// progressWriter redraws download updates when attached to a terminal.
	// Child output is captured; keep progress visible in both interfaces.
	a := []string{"--ignore-config", "--no-playlist", "--no-overwrites", "--no-quiet", "--progress", "--abort-on-unavailable-fragments"}
	if o.Cookies != "" {
		a = append(a, "--cookies-from-browser", o.Cookies)
	}
	if list {
		return append(a, "--list-formats", "--", o.URL)
	}
	a = append(a, "--paths", o.Out, "--output", "%(title).120B [%(extractor_key)s-%(id)s] ["+outputVariant(o)+"].%(ext)s")
	if o.Mode == "audio" {
		a = append(a, "--format", "bestaudio/best", "--extract-audio", "--audio-format", o.Audio, "--audio-quality", "0")
	} else {
		filter := ""
		if o.Quality != "best" {
			filter = "[height<=" + o.Quality + "]"
		}
		// Every fallback respects the height limit and includes audio.
		selector := "bestvideo" + filter + "+bestaudio/best" + filter
		a = append(a, "--format", selector, "--merge-output-format", "mkv", "--remux-video", "mkv")
	}
	return append(a, "--", o.URL)
}

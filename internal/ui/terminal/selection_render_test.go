// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

// Interpret SGR attributes as a terminal does: changing foreground does not reset
// a background. Inspect the rendered stream, rather than just canvas cell styles.
func TestSelectionBackgroundCannotCrossPanelBorder(t *testing.T) {
	for _, width := range []int{73, 100, 131} {
		for _, focus := range []int{0, 1} {
			var output bytes.Buffer
			o := defaultOptions(t.TempDir())
			u := panelUI{out: &output, o: &o, width: width, height: 30, focus: focus, files: []libraryFile{{Name: "中文 файл 🎬.mp4", Size: 1024}}, pool: &jobPool{jobs: []*job{{id: 1, title: "Queued video", state: "ready"}}}}
			u.render()
			x, y, background := 0, 0, 0
			highlighted := 0
			data := []rune(output.String())
			for i := 0; i < len(data); i++ {
				r := data[i]
				if r == 27 && i+1 < len(data) && data[i+1] == '[' {
					j := i + 2
					for j < len(data) && !(data[j] >= '@' && data[j] <= '~') {
						j++
					}
					if j == len(data) {
						t.Fatal("incomplete ANSI sequence")
					}
					if data[j] == 'm' {
						for _, p := range strings.Split(string(data[i+2:j]), ";") {
							n, _ := strconv.Atoi(p)
							if n == 0 || n == 49 {
								background = 0
							}
							if n >= 40 && n <= 47 {
								background = n
							}
						}
					}
					if data[j] == 'H' {
						x, y = 0, 0
					}
					i = j
					continue
				}
				if r == '\r' {
					x = 0
					continue
				}
				if r == '\n' {
					y++
					continue
				}
				if background == 46 {
					highlighted++
					allowed := (y == 3 || y == 6) && x >= 1 && x < width-2
					if !allowed {
						t.Fatalf("width=%d focus=%d: selection leaked to (%d,%d)", width, focus, x, y)
					}
				}
				x += cellWidth(r)
			}
			if highlighted == 0 {
				t.Fatal("selection was not rendered")
			}
		}
	}
}

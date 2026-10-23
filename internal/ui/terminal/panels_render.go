// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"strings"
	"unicode"
)

type screenCell struct{ text, style string }
type panelCanvas struct {
	cells [][]screenCell
	w, h  int
}

const (
	normal           = "\033[0m"
	cyan             = "\033[1;36m"
	dim              = "\033[2m"
	selected         = "\033[30;46m"
	selectedProgress = "\033[97;44m"
	green            = "\033[32m"
)

func newCanvas(w, h int) *panelCanvas {
	c := &panelCanvas{w: w, h: h}
	for y := 0; y < h; y++ {
		row := make([]screenCell, w)
		for x := range row {
			row[x] = screenCell{" ", normal}
		}
		c.cells = append(c.cells, row)
	}
	return c
}
func cellWidth(r rune) int {
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || r == 0x200d || r == 0xfe0f {
		return 0
	}
	if r >= 0x1100 && (r <= 0x115f || r >= 0x2e80 && r <= 0xa4cf || r >= 0xac00 && r <= 0xd7a3 || r >= 0xf900 && r <= 0xfaff || r >= 0xfe10 && r <= 0xfe6f || r >= 0xff00 && r <= 0xff60 || r >= 0x1f300 && r <= 0x1faff || r >= 0x20000) {
		return 2
	}
	return 1
}
func (c *panelCanvas) clearCell(x, y int) {
	if c.cells[y][x].text == "" && x > 0 {
		c.cells[y][x-1] = screenCell{" ", normal}
	}
	if x+1 < c.w && c.cells[y][x+1].text == "" {
		c.cells[y][x+1] = screenCell{" ", normal}
	}
	c.cells[y][x] = screenCell{" ", normal}
}
func (c *panelCanvas) text(x, y, max int, s, style string) {
	if y < 0 || y >= c.h || x < 0 || x >= c.w {
		return
	}
	end := minInt(c.w, x+max)
	for _, r := range safeText(s) {
		w := cellWidth(r)
		if w == 0 {
			continue
		}
		if x+w > end {
			return
		}
		c.clearCell(x, y)
		if w == 2 {
			c.clearCell(x+1, y)
		}
		c.cells[y][x] = screenCell{string(r), style}
		if w == 2 {
			c.cells[y][x+1] = screenCell{"", style}
		}
		x += w
	}
}
func (c *panelCanvas) fill(x, y, w int, style string) {
	for i := maxInt(0, x); i < minInt(c.w, x+w); i++ {
		if y >= 0 && y < c.h {
			c.clearCell(i, y)
			c.cells[y][i] = screenCell{" ", style}
		}
	}
}
func (c *panelCanvas) box(x, y, w, h int, title string, active bool) {
	if w < 2 || h < 2 {
		return
	}
	style := dim
	if active {
		style = cyan
	}
	c.text(x, y, w, "┌"+strings.Repeat("─", w-2)+"┐", style)
	c.text(x, y+h-1, w, "└"+strings.Repeat("─", w-2)+"┘", style)
	for row := y + 1; row < y+h-1; row++ {
		c.fill(x, row, w, normal)
		c.text(x, row, 1, "│", style)
		c.text(x+w-1, row, 1, "│", style)
	}
	c.text(x+2, y, w-4, " "+title+" ", style)
}
func (c *panelCanvas) String() string {
	var b strings.Builder
	b.WriteString("\033[H")
	style := ""
	for y, row := range c.cells {
		if y > 0 {
			b.WriteString("\r\n")
		}
		for _, cell := range row {
			if cell.style != style {
				b.WriteString(normal)
				if cell.style != normal {
					b.WriteString(cell.style)
				}
				style = cell.style
			}
			b.WriteString(cell.text)
		}
	}
	b.WriteString(normal)
	return b.String()
}
func (u *panelUI) drawModal(c *panelCanvas) {
	width := minInt(86, c.w-8)
	height := 7
	if len(u.choices) > 0 && u.modal != "url" && u.modal != "folder" && u.modal != "browser" && u.modal != "details" && u.modal != "search" && !strings.HasPrefix(u.modal, "resource:") && !u.workspaceInput() {
		height = minInt(c.h-4, len(u.choices)+5)
	}
	if u.modal == "details" || u.modal == "help" || u.modal == "logs" || u.modal == "resources-panel" || u.modal == "upload-review" || u.modal == "clip-review" {
		height = minInt(c.h-4, 20)
	}
	x, y := (c.w-width)/2, (c.h-height)/2
	c.box(x, y, width, height, u.title, true)
	if u.modal == "url" || u.modal == "folder" || u.modal == "browser" || u.modal == "search" || u.workspaceInput() || strings.HasPrefix(u.modal, "resource:") {
		value := []rune(u.input)
		if u.secretInput() {
			value = []rune(strings.Repeat("•", len(value)))
		}
		if len(value) > width-8 {
			value = value[len(value)-(width-8):]
		}
		c.fill(x+2, y+2, width-4, selected)
		c.text(x+2, y+2, width-4, "> "+string(value)+"▏", selected)
		c.text(x+2, y+height-2, width-4, tr("Enter: подтвердить · Esc: назад. Загрузки продолжаются."), dim)
	} else if u.modal == "details" || u.modal == "help" || u.modal == "logs" || u.modal == "resources-panel" || u.modal == "upload-review" || u.modal == "clip-review" {
		lines := wrapLines(u.input, width-4)
		start := min(u.detailOffset, max(0, len(lines)-(height-3)))
		for row := 0; row < height-3 && row+start < len(lines); row++ {
			c.text(x+2, y+1+row, width-4, lines[row+start], normal)
		}
		c.text(x+2, y+height-2, width-4, "Enter / Esc", dim)
	} else {
		visible := height - 4
		start := maxInt(0, u.choiceIndex-visible+1)
		for i := start; i < len(u.choices) && i < start+visible; i++ {
			style := normal
			prefix := "  "
			if i == u.choiceIndex {
				style = selected
				prefix = "› "
			}
			c.fill(x+1, y+2+i-start, width-2, style)
			if u.modal == "upload-targets" && u.profileTargets[u.choices[i].Value] {
				prefix = "✓ "
			}
			c.text(x+2, y+2+i-start, width-4, prefix+u.choices[i].Label, style)
		}
		c.text(x+2, y+height-2, width-4, tr("↑/↓: выбор  Enter: подтвердить  Esc/←: назад"), dim)
	}
}

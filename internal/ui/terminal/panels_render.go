// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"fmt"
	"strings"
	"unicode"
)

type screenCell struct{ text, style string }
type panelCanvas struct {
	cells [][]screenCell
	w, h  int
}

const (
	normal   = "\033[0m"
	cyan     = "\033[1;36m"
	dim      = "\033[2m"
	selected = "\033[30;46m"
	green    = "\033[32m"
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
func (u *panelUI) render() {
	w, h := minInt(u.width-1, 240), minInt(u.height-1, 80)
	if w < 1 || h < 1 {
		return
	}
	c := newCanvas(w, h)
	c.text(1, 0, w-2, fmt.Sprintf("CLIPNARO %s  |  %s: %d/2", version, tr("Активно"), u.pool.active), cyan)
	if w < 72 || h < 19 {
		c.text(1, 2, w-2, tr("Увеличьте терминал: минимум 73 × 20."), normal)
		c.text(1, 4, w-2, "q: "+tr("Выход"), normal)
		if u.modal == "quit" {
			c.text(1, 6, w-2, tr("Остановить задания и выйти?"), normal)
			c.text(1, 7, w-2, "↑/↓ Enter: "+u.choices[u.choiceIndex].Label, cyan)
		}
		fmt.Fprint(u.out, c.String())
		return
	}
	c.text(1, 1, w-2, u.o.Out, dim)
	logH := maxInt(5, minInt(9, h/4))
	topH := h - logH - 5
	leftW := w / 2
	rightX := leftW
	rightW := w - rightX
	c.box(0, 3, leftW, topH, tr("Мои файлы")+fmt.Sprintf(" [%d]", len(u.files)), u.focus == 0)
	c.box(rightX, 3, rightW, topH, tr("Задания")+fmt.Sprintf(" [%d]", len(u.pool.jobs)), u.focus == 1)
	rows := topH - 2
	start := maxInt(0, u.fileIndex-rows+1)
	if len(u.files) == 0 {
		c.text(2, 5, leftW-4, tr("Здесь пока нет видео или аудио."), dim)
	}
	for i := start; i < len(u.files) && i < start+rows; i++ {
		style := normal
		prefix := "  "
		if i == u.fileIndex {
			prefix = "› "
			if u.focus == 0 {
				style = selected
			}
		}
		row := 4 + i - start
		c.fill(1, row, leftW-2, style)
		c.text(2, row, leftW-16, prefix+u.files[i].Name, style)
		c.text(leftW-13, row, 11, fileSize(u.files[i].Size), style)
	}
	jobRows := maxInt(1, rows/2)
	jobStart := maxInt(0, u.jobIndex-jobRows+1)
	if len(u.pool.jobs) == 0 {
		c.text(rightX+2, 5, rightW-4, tr("v — видео · a — аудио"), dim)
	}
	for i := jobStart; i < len(u.pool.jobs) && i < jobStart+jobRows; i++ {
		j := u.pool.jobs[i]
		row := 4 + (i-jobStart)*2
		style := normal
		if j.state == "done" {
			style = green
		}
		prefix := "  "
		if i == u.jobIndex {
			prefix = "› "
			if u.focus == 1 {
				style = selected
			}
		}
		c.fill(rightX+1, row, rightW-2, style)
		c.text(rightX+2, row, rightW-4, fmt.Sprintf("%s#%d %s", prefix, j.id, j.title), style)
		detail := jobState(j.state)
		if contains([]string{"downloading", "processing", "verifying"}, j.state) {
			detail += " " + j.detail
		}
		c.text(rightX+3, row+1, rightW-5, detail, dim)
	}
	logY := 3 + topH
	c.box(0, logY, w, logH, tr("Журнал событий"), u.focus == 2)
	last := maxInt(0, len(u.logs)-u.logOffset)
	first := maxInt(0, last-(logH-2))
	for i := first; i < last; i++ {
		c.text(2, logY+1+i-first, w-4, u.logs[i], normal)
	}
	c.text(1, h-2, w-2, tr("Tab: панель  ↑↓: выбор  Enter: открыть/качество  v/a: добавить  s: настройки"), cyan)
	c.text(1, h-1, w-2, tr("s: Настройки f:папка o:показать x:отмена r:повтор d:убрать q:выход"), dim)
	if u.modal != "" {
		u.drawModal(c)
	}
	fmt.Fprint(u.out, c.String())
}
func (u *panelUI) drawModal(c *panelCanvas) {
	width := minInt(86, c.w-8)
	height := 7
	if len(u.choices) > 0 && u.modal != "url" && u.modal != "folder" && u.modal != "browser" && u.modal != "details" {
		height = minInt(c.h-4, len(u.choices)+5)
	}
	if u.modal == "details" {
		height = minInt(c.h-4, 12)
	}
	x, y := (c.w-width)/2, (c.h-height)/2
	c.box(x, y, width, height, u.title, true)
	if u.modal == "url" || u.modal == "folder" || u.modal == "browser" {
		value := []rune(u.input)
		if len(value) > width-8 {
			value = value[len(value)-(width-8):]
		}
		c.fill(x+2, y+2, width-4, selected)
		c.text(x+2, y+2, width-4, "> "+string(value)+"▏", selected)
		c.text(x+2, y+height-2, width-4, tr("Enter: подтвердить · Esc: назад. Загрузки продолжаются."), dim)
	} else if u.modal == "details" {
		runes := []rune(safeText(u.input))
		for row := 0; row < height-3 && len(runes) > 0; row++ {
			n := minInt(len(runes), width-6)
			c.text(x+2, y+1+row, width-4, string(runes[:n]), normal)
			runes = runes[n:]
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
			c.text(x+2, y+2+i-start, width-4, prefix+u.choices[i].Label, style)
		}
		c.text(x+2, y+height-2, width-4, tr("↑/↓: выбор  Enter: подтвердить  Esc/←: назад"), dim)
	}
}

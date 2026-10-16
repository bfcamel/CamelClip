// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/bfcamel/video-downloader/internal/core"
)

var errCanceled = core.ErrCanceled

type choice struct{ Label, Value string }
type menu struct {
	in       *bufio.Reader
	out      io.Writer
	terminal bool
	ctx      context.Context
}

func newMenu() *menu {
	c := exec.Command("stty", "-g")
	c.Stdin = os.Stdin
	_, err := c.Output()
	out, e := os.Stdout.Stat()
	return &menu{in: bufio.NewReader(os.Stdin), out: os.Stdout, terminal: err == nil && e == nil && out.Mode()&os.ModeCharDevice != 0 && os.Getenv("TERM") != "dumb"}
}
func (m *menu) line(label, fallback string) (string, error) {
	fmt.Fprintf(m.out, "%s [%s]: ", label, fallback)
	type result struct {
		s   string
		err error
	}
	ch := make(chan result, 1)
	go func() { s, e := m.in.ReadString('\n'); ch <- result{s, e} }()
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	var s string
	var err error
	select {
	case r := <-ch:
		s, err = r.s, r.err
	case <-ctx.Done():
		return "", errCanceled
	}
	if err != nil && (err != io.EOF || s == "") {
		return "", io.EOF
	}
	s = strings.TrimSpace(s)
	if s == "" {
		s = fallback
	}
	return s, nil
}
func (m *menu) selectItem(ctx context.Context, label string, items []choice, preferred string) (string, error) {
	if len(items) == 0 {
		return "", errors.New(tr("нет доступных вариантов"))
	}
	selected := 0
	for i, v := range items {
		if v.Value == preferred {
			selected = i
		}
	}
	if !m.terminal {
		for {
			fmt.Fprintln(m.out, label)
			fmt.Fprintln(m.out, tr("0 / b — назад"))
			for i, v := range items {
				fmt.Fprintf(m.out, "  %d. %s\n", i+1, v.Label)
			}
			s, err := m.line(tr("Номер"), strconv.Itoa(selected+1))
			if err != nil {
				return "", err
			}
			if s == "q" || s == "b" || s == "back" || s == "0" {
				return "", errCanceled
			}
			n, err := strconv.Atoi(s)
			if err == nil && n >= 1 && n <= len(items) {
				return items[n-1].Value, nil
			}
			fmt.Fprintln(m.out, tr("Введите номер из списка."))
		}
	}
	state := exec.Command("stty", "-g")
	state.Stdin = os.Stdin
	saved, err := state.Output()
	if err != nil {
		return "", err
	}
	set := func(args ...string) error { c := exec.Command("stty", args...); c.Stdin = os.Stdin; return c.Run() }
	if err = set("-icanon", "-echo", "min", "0", "time", "1"); err != nil {
		return "", err
	}
	defer set(strings.TrimSpace(string(saved)))
	fmt.Fprintln(m.out, label+tr(" — ↑/↓, Enter; q — отмена"))
	visible := len(items)
	if visible > 8 {
		visible = 8
	}
	fmt.Fprint(m.out, "\033[?25l")
	defer fmt.Fprint(m.out, "\033[?25h")
	draw := func() {
		start := selected - visible + 1
		if start < 0 {
			start = 0
		}
		if start+visible > len(items) {
			start = len(items) - visible
		}
		for i := start; i < start+visible; i++ {
			v := items[i]
			mark := "  "
			if i == selected {
				mark = "› "
			}
			fmt.Fprintf(m.out, "\r\033[2K%s%s\n", mark, v.Label)
		}
		fmt.Fprintf(m.out, "\r\033[2K  %d / %d\n", selected+1, len(items))
	}
	draw()
	escape := 0
	var b [1]byte
	for {
		if ctx.Err() != nil {
			return "", errCanceled
		}
		n, e := os.Stdin.Read(b[:])
		if n == 0 {
			if e != nil && e != io.EOF {
				return "", e
			}
			continue
		}
		switch b[0] {
		case 27:
			escape = 1
			continue
		case '[':
			if escape == 1 {
				escape = 2
				continue
			}
		case 'A':
			if escape == 2 {
				selected = (selected + len(items) - 1) % len(items)
			}
		case 'B':
			if escape == 2 {
				selected = (selected + 1) % len(items)
			}
		case '\n', '\r':
			return items[selected].Value, nil
		case 'q', 3:
			return "", errCanceled
		}
		escape = 0
		fmt.Fprintf(m.out, "\033[%dA", visible+1)
		draw()
	}
}

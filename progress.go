package main

import (
	"fmt"
	"io"
	"strings"
)

type progressWriter struct {
	out              io.Writer
	tail             *tailBuffer
	terminal, active bool
	pending          string
}

func (p *progressWriter) Write(data []byte) (int, error) {
	if p.tail != nil {
		p.tail.Write(data)
	}
	p.pending += string(data)
	for {
		i := strings.IndexAny(p.pending, "\r\n")
		if i < 0 {
			break
		}
		line := p.pending[:i]
		p.pending = p.pending[i+1:]
		p.line(line)
	}
	if len(p.pending) > 16384 {
		p.line(p.pending)
		p.pending = ""
	}
	return len(data), nil
}
func (p *progressWriter) line(s string) {
	if s == "" {
		return
	}
	progress := strings.HasPrefix(s, "[download]") && (strings.Contains(s, "%") || strings.Contains(s, " at "))
	if p.terminal && progress {
		fmt.Fprintf(p.out, "\r\033[2K%s", s)
		p.active = true
		return
	}
	if p.active {
		fmt.Fprintln(p.out)
		p.active = false
	}
	fmt.Fprintln(p.out, s)
}
func (p *progressWriter) finish() {
	if p.pending != "" {
		p.line(p.pending)
		p.pending = ""
	}
	if p.active {
		fmt.Fprintln(p.out)
		p.active = false
	}
}

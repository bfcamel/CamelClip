package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestKeyDecoderAndUnicode(t *testing.T) {
	d := keyDecoder{}
	if len(d.feed([]byte("\033["), false)) != 0 {
		t.Fatal("partial arrow emitted")
	}
	got := d.feed([]byte("B\tя"), false)
	if strings.Join(got, ",") != "down,tab,я" {
		t.Fatal(got)
	}
	if len(d.feed([]byte("\033"), false)) != 0 {
		t.Fatal("early escape")
	}
	if got = d.feed(nil, true); len(got) != 1 || got[0] != "escape" {
		t.Fatal(got)
	}
}
func TestJobStateTransitionsAndCancellation(t *testing.T) {
	p := newJobPool(context.Background(), "")
	o := defaultOptions(t.TempDir())
	o.URL = "https://example.com/a"
	if !p.add(o) || p.add(o) {
		t.Fatal("duplicate URL accepted")
	}
	j := p.jobs[0]
	p.active = 1
	j.cancel = func() {}
	j.state = "inspecting"
	p.apply(jobEvent{id: j.id, kind: "inspected", info: mediaInfo{Title: "Example", ID: "a"}})
	if p.active != 0 || j.state != "choose" {
		t.Fatal(j.state, p.active)
	}
	j.state = "downloading"
	p.active = 1
	called := false
	j.cancel = func() { called = true }
	p.cancelJob(j)
	if !called || j.state != "canceling" {
		t.Fatal(j.state)
	}
	p.apply(jobEvent{id: j.id, kind: "finished", err: errCanceled})
	if p.active != 0 || j.state != "canceled" {
		t.Fatal(j.state)
	}
	if !p.add(o) {
		t.Fatal("retry after cancellation blocked")
	}
	j = p.jobs[1]
	j.state = "inspecting"
	j.cancel = func() {}
	p.active = 1
	p.apply(jobEvent{id: j.id, kind: "inspected", err: errors.New("network")})
	if j.state != "failed" || j.detail != "network" {
		t.Fatal(j)
	}
}
func TestPanelFocusAndBackgroundProgressDuringModal(t *testing.T) {
	o := defaultOptions(t.TempDir())
	o.URL = "https://example.com/a"
	p := newJobPool(context.Background(), "")
	p.add(o)
	var out bytes.Buffer
	u := panelUI{ctx: context.Background(), out: &out, o: &o, pool: p, width: 100, height: 30, modal: "url", input: "https://example.com/"}
	p.jobs[0].state = "downloading"
	u.event(jobEvent{id: 1, kind: "progress", text: "[download] 50%"})
	if u.modal != "url" || !strings.Contains(p.jobs[0].detail, "50%") {
		t.Fatal("modal blocked progress")
	}
	u.render()
	s := out.String()
	if !strings.Contains(s, "MACDOWN") || !strings.Contains(s, "50%") {
		t.Fatal("missing independent panels")
	}
	u.key("escape")
	u.key("tab")
	if u.focus != 1 {
		t.Fatal(u.focus)
	}
}
func TestCanvasClipsUnicodeAndControls(t *testing.T) {
	c := newCanvas(10, 3)
	c.text(0, 0, 10, "中文abcdefgh", normal)
	count := 0
	for _, cell := range c.cells[0] {
		for _, r := range cell.text {
			count += cellWidth(r)
		}
	}
	if count != 10 {
		t.Fatal(count)
	}
	c.text(0, 1, 10, "\033[2Jevil", normal)
	if strings.Contains(c.String(), "\033[2J") {
		t.Fatal("untrusted escape sequence escaped into terminal")
	}
}
func TestJobOutputHandlesChunkedProgress(t *testing.T) {
	var got []string
	w := jobOutput{emit: func(s string) { got = append(got, s) }}
	w.Write([]byte("[download] 4"))
	w.Write([]byte("0%\r\nDone"))
	w.flush()
	if len(got) != 2 || got[0] != "[download] 40%" || got[1] != "Done" {
		t.Fatal(got)
	}
}

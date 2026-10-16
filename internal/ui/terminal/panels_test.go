// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"bytes"
	"path/filepath"
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

func TestSettingsSubmenusReturnToParent(t *testing.T) {
	root := t.TempDir()
	o := testOptions(root)
	o.Out = root
	u := panelUI{o: &o, config: filepath.Join(root, "settings.json"), home: root, system: "ru", pool: &jobPool{}}
	enter := func(value string) {
		t.Helper()
		for i, c := range u.choices {
			if c.Value == value {
				u.choiceIndex = i
				u.modalKey("enter")
				return
			}
		}
		t.Fatalf("choice %s missing", value)
	}
	for _, key := range []string{"escape", "left", "backspace"} {
		u.settings()
		enter("audio")
		if u.modal != "audio" {
			t.Fatal(u.modal)
		}
		u.modalKey(key)
		if u.modal != "settings" {
			t.Fatalf("%s did not return to settings", key)
		}
		u.modalKey("escape")
		if u.modal != "" {
			t.Fatal("root did not close")
		}
	}
	u.settings()
	enter("audio")
	enter("opus")
	if u.modal != "settings" || o.Audio != "opus" {
		t.Fatal("selection did not save and return to settings", u.modal, o.Audio)
	}
	enter("audio")
	enter("__back")
	if u.modal != "settings" {
		t.Fatal("visible Back failed")
	}
	enter("__back")
	if u.modal != "" {
		t.Fatal("root Back failed")
	}
}
func TestSettingsHintVisibleOnBottomLineAtMinimumSize(t *testing.T) {
	var out bytes.Buffer
	o := testOptions(t.TempDir())
	u := panelUI{out: &out, o: &o, pool: &jobPool{}, width: 73, height: 20}
	u.render()
	lines := strings.Split(out.String(), "\r\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "s:") {
		t.Fatal("settings hint truncated at minimum width", last)
	}
}

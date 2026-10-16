// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSettingsRoundTripAndCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	o := defaults()
	o.Cookies = "firefox"
	if err := saveSettings(path, o); err != nil {
		t.Fatal(err)
	}
	loaded := testOptions("/home/test")
	if err := loadSettings(path, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Quality != o.Quality || loaded.Out != o.Out || loaded.Cookies != "firefox" || loaded.URL != "" {
		t.Fatalf("%+v", loaded)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
	os.WriteFile(path, []byte(`{"mode":"invalid"}`), 0600)
	before := loaded
	if loadSettings(path, &loaded) == nil || loaded != before {
		t.Fatal("invalid settings must not replace defaults")
	}
}
func TestAvailableQualities(t *testing.T) {
	info := mediaInfo{Formats: []mediaFormat{{Height: 720, VCodec: "avc1", ACodec: "none"}, {Height: 1080, VCodec: "avc1", ACodec: "none"}, {Height: 1080, VCodec: "vp9", ACodec: "none"}, {Height: 2160, VCodec: "vp9", ACodec: "none", DRM: true}, {VCodec: "none", ACodec: "aac"}}}
	q := qualities(info)
	if len(q) != 3 || q[1].Value != "1080" || q[2].Value != "720" {
		t.Fatal(q)
	}
}
func TestNumberMenuRetries(t *testing.T) {
	var out bytes.Buffer
	m := menu{in: bufio.NewReader(strings.NewReader("oops\n9\n2\n")), out: &out}
	v, e := m.selectItem(context.Background(), "Mode", []choice{{"Video", "video"}, {"Audio", "audio"}}, "video")
	if e != nil || v != "audio" || strings.Count(out.String(), "Введите номер") != 2 {
		t.Fatal(v, e, out.String())
	}
}
func TestQueueParsing(t *testing.T) {
	urls, e := readLinks(strings.NewReader("# comment\n\n https://example.com/a \nhttps://example.com/b\r\n"))
	if e != nil || !reflect.DeepEqual(urls, []string{"https://example.com/a", "https://example.com/b"}) {
		t.Fatal(urls, e)
	}
}
func TestReadableErrors(t *testing.T) {
	for raw, want := range map[string]string{"ERROR: Sign in to confirm": "требует входа", "ERROR: HTTP Error 429": "ограничил", "ERROR: Requested format is not available": "качество", "ERROR: Connection timed out": "интернет", "ERROR: No space left on device": "места"} {
		if !strings.Contains(explain(raw), want) {
			t.Fatal(raw, explain(raw))
		}
	}
}
func TestProgressChunksAndCompletion(t *testing.T) {
	for _, terminal := range []bool{true, false} {
		var out bytes.Buffer
		p := progressWriter{out: &out, terminal: terminal}
		for _, chunk := range []string{"[download] 1", "0% at 1MiB/s\n[download] 20% at 2MiB/s\r", "\nСохранено: /tmp/a.mkv\n"} {
			p.Write([]byte(chunk))
		}
		p.finish()
		s := out.String()
		if terminal {
			if strings.Count(s, "\033[2K") != 2 || strings.Count(s, "\n") != 2 {
				t.Fatal(s)
			}
		} else if strings.Contains(s, "\033") || strings.Count(s, "\n") != 3 {
			t.Fatal(s)
		}
	}
}

func TestLanguages(t *testing.T) {
	old := currentLanguage
	defer func() { currentLanguage = old }()
	for locale, want := range map[string]string{"ru_RU.UTF-8": "ru", "ru-RU": "ru", "en_US.UTF-8": "en", "de-DE": "en", "C": ""} {
		if got := localeLanguage(locale); got != want {
			t.Fatalf("%s: %s", locale, got)
		}
	}
	useLanguage("en", "ru")
	if tr("Папка сохранения") != "Output directory" {
		t.Fatal("English translation missing")
	}
	useLanguage("auto", "ru")
	if tr("Папка сохранения") != "Папка сохранения" {
		t.Fatal("system default not used")
	}
	if requestedLanguage([]string{"-lang=en", "-lang", "ru"}, "auto") != "ru" {
		t.Fatal("explicit language precedence")
	}
}

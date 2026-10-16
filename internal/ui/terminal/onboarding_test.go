// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupSavesExplicitChoicesAndRetriesChecks(t *testing.T) {
	old := currentLanguage
	defer useLanguage(old, "en")
	home := t.TempDir()
	o := testOptions(home)
	path := filepath.Join(home, "config/settings.json")
	var out bytes.Buffer
	input := "1\n" + filepath.Join(home, "videos") + "\n2\n2\n2\n1\n"
	m := &menu{in: bufio.NewReader(strings.NewReader(input)), out: &out, ctx: context.Background()}
	calls := 0
	err := setup(context.Background(), m, &o, path, "en", home, func(context.Context, string, options) error {
		calls++
		if calls == 1 {
			return errors.New("missing dependency")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || o.Codec != "hevc" || o.Container != "mkv" || o.Cookies != "firefox" || o.Lang != "en" {
		t.Fatal(o, calls)
	}
	restored := testOptions(home)
	if err = loadSettings(path, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Cookies != "firefox" || restored.Out != o.Out {
		t.Fatal(restored)
	}
	if !strings.Contains(out.String(), "Retry checks") {
		t.Fatal(out.String())
	}
}
func TestCanceledSetupNeverWritesPreferences(t *testing.T) {
	old := currentLanguage
	defer useLanguage(old, "en")
	home := t.TempDir()
	o := testOptions(home)
	path := filepath.Join(home, "settings.json")
	var out bytes.Buffer
	m := &menu{in: bufio.NewReader(strings.NewReader("0\n")), out: &out, ctx: context.Background()}
	err := setup(context.Background(), m, &o, path, "en", home, func(context.Context, string, options) error { t.Fatal("unexpected check"); return nil })
	if !errors.Is(err, errCanceled) {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("canceled setup saved settings")
	}
}

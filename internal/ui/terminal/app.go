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
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

type links []string

func (l *links) String() string     { return strings.Join(*l, " ") }
func (l *links) Set(s string) error { *l = append(*l, s); return nil }
func run(deps Dependencies) error {
	system := systemLanguage()
	useLanguage(requestedLanguage(os.Args[1:], "auto"), system)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(configDir, "MacDown", "settings.json")
	if override := os.Getenv("MACDOWN_CONFIG_DIR"); override != "" {
		configPath = filepath.Join(override, "settings.json")
	}
	o := defaultOptions(home)
	o.runtime = &deps
	if err = loadSettings(configPath, &o); err != nil {
		fmt.Fprintln(os.Stderr, tr("Не удалось прочитать настройки; используются стандартные значения:"), err)
	}
	useLanguage(requestedLanguage(os.Args[1:], o.Lang), system)
	request, err := parseCLI(os.Args[1:], &o)
	if err != nil {
		return err
	}
	useLanguage(o.Lang, system)
	if request.help {
		printHelp(request.command)
		return nil
	}
	switch request.command {
	case "version":
		fmt.Println("macdown " + version)
		return nil
	case "config":
		return showConfig(configPath, o)
	case "migrate":
		queue, e := deps.OpenQueue(ctx, configPath, 1)
		if e != nil {
			return e
		}
		if e = queue.Close(); e != nil {
			return e
		}
		fmt.Println(tr("Очередь хранится в SQLite."))
		return nil
	case "doctor":
		return doctor(ctx)
	}
	urls, listPath := request.urls, request.list
	if listPath != "" {
		f, e := os.Open(expandHome(listPath, home))
		if e != nil {
			return fmt.Errorf(tr("не удалось открыть список: %w"), e)
		}
		more, e := readLinks(f)
		f.Close()
		if e != nil {
			return e
		}
		urls = append(urls, more...)
		if len(urls) == 0 {
			return errors.New(tr("файл списка не содержит ссылок"))
		}
	}
	o.Out, err = filepath.Abs(expandHome(o.Out, home))
	if err != nil {
		return err
	}
	if len(urls) == 0 {
		m := newMenu()
		m.ctx = ctx
		return dashboard(ctx, m, &o, configPath, system, home)
	}
	return runQueue(ctx, urls, o, configPath, nil)
}

func readLinks(r io.Reader) ([]string, error) {
	result := []string{}
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 4096), 1024*1024)
	for s.Scan() {
		v := strings.TrimSpace(s.Text())
		if v != "" && !strings.HasPrefix(v, "#") {
			result = append(result, v)
		}
	}
	return result, s.Err()
}
func validateLinks(urls []string, o options) error {
	for _, u := range urls {
		o.URL = u
		if err := validate(o); err != nil {
			return fmt.Errorf("%s: %w", u, err)
		}
	}
	return nil
}
func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, s)
}

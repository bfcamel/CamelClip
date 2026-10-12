package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

type links []string

func (l *links) String() string     { return strings.Join(*l, " ") }
func (l *links) Set(s string) error { *l = append(*l, s); return nil }
func run() error {
	system := systemLanguage()
	useLanguage(requestedLanguage(os.Args[1:], "auto"), system)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
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
	o.Out = expandHome(o.Out, home)
	if len(urls) == 0 {
		m := newMenu()
		m.ctx = ctx
		return dashboard(ctx, m, &o, configPath, system, home)
	}
	return runQueue(ctx, urls, o, configPath, nil)
}

func runQueue(ctx context.Context, urls []string, o options, configPath string, m *menu) error {
	var err error
	if err = validateLinks(urls, o); err != nil {
		return err
	}
	ytdlp, err := exec.LookPath("yt-dlp")
	if err != nil {
		return errors.New(tr("не найден yt-dlp. Установка: brew install yt-dlp ffmpeg deno"))
	}
	if !o.Formats {
		for _, name := range []string{"ffmpeg", "ffprobe"} {
			if _, err = exec.LookPath(name); err != nil {
				return fmt.Errorf(tr("не найден %s. Установка: brew install ffmpeg"), name)
			}
		}
		if err = os.MkdirAll(o.Out, 0755); err != nil {
			return fmt.Errorf(tr("не удалось создать папку %s: %w"), o.Out, err)
		}
	}
	failed := 0
	done := 0
	failures := []string{}
	for i, link := range urls {
		if ctx.Err() != nil {
			return errCanceled
		}
		o.URL = link
		fmt.Printf("\n[%d/%d] %s\n", i+1, len(urls), link)
		if o.Formats {
			c := command(ctx, ytdlp, arguments(o)...)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			err = c.Run()
		} else {
			fmt.Println(tr("Получение доступных качеств…"))
			var info mediaInfo
			info, err = inspect(ctx, ytdlp, o)
			if err == nil {
				fmt.Printf("%s", safeText(info.Title))
				if info.Duration > 0 {
					fmt.Printf(" · %d:%02d", int(info.Duration)/60, int(info.Duration)%60)
				}
				fmt.Println()
				items := qualities(info)
				if o.Mode == "video" {
					if m != nil {
						o.Quality, err = m.selectItem(ctx, tr("Качество (максимальная высота)"), items, o.Quality)
					} else {
						labels := []string{}
						for _, v := range items[1:] {
							labels = append(labels, v.Label)
						}
						if len(labels) > 0 {
							fmt.Println(tr("Доступно:"), strings.Join(labels, ", "))
						}
					}
				}
				if err == nil {
					fmt.Println(tr("Скачивание →"), o.Out)
					err = download(ctx, ytdlp, o)
				}
			}
		}
		if ctx.Err() != nil || errors.Is(err, errCanceled) {
			return errCanceled
		}
		if err != nil {
			failed++
			if len(failures) < 3 {
				failures = append(failures, fmt.Sprintf("%d. %s", i+1, err.Error()))
			}
			fmt.Fprintln(os.Stderr, tr("Не удалось:"), err)
			continue
		}
		done++
		if !o.Formats && m != nil {
			if e := saveSettings(configPath, o); e != nil {
				fmt.Fprintln(os.Stderr, tr("Не удалось сохранить настройки:"), e)
			}
		}
	}
	fmt.Printf(tr("\nЗавершено: %d. Ошибок: %d.\n"), done, failed)
	if failed > 0 {
		return fmt.Errorf("%s\n%s", tr("часть ссылок не обработана; причины указаны выше"), strings.Join(failures, "\n"))
	}
	return nil
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

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
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
	var urls links
	var listPath string
	flag.StringVar(&o.Lang, "lang", o.Lang, tr("Язык: auto, ru, en"))
	flag.Var(&urls, "url", tr("Ссылка; параметр можно повторять"))
	flag.StringVar(&listPath, "list", "", tr("Текстовый файл: одна ссылка в строке"))
	flag.StringVar(&o.Mode, "mode", o.Mode, tr("video — видео со звуком; audio — только звук"))
	flag.StringVar(&o.Quality, "quality", o.Quality, tr("Максимальная высота кадра или best"))
	flag.StringVar(&o.Audio, "audio-format", o.Audio, "mp3, m4a, opus, flac, wav, best")
	flag.StringVar(&o.Out, "out", o.Out, tr("Папка сохранения"))
	flag.StringVar(&o.Cookies, "cookies-browser", "", tr("safari, chrome, firefox, edge, brave (не запоминается)"))
	flag.BoolVar(&o.Formats, "formats", false, tr("Показать доступные форматы без скачивания"))
	flag.Parse()
	if !contains([]string{"auto", "ru", "en"}, o.Lang) {
		return errors.New(tr("язык должен быть auto, ru или en"))
	}
	useLanguage(o.Lang, system)
	urls = append(urls, flag.Args()...)
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
	interactive := len(urls) == 0
	m := newMenu()
	m.ctx = ctx
	if interactive {
		o.Lang, err = m.selectItem(ctx, "Language / Язык", []choice{{tr("Системный") + " (" + system + ")", "auto"}, {"English", "en"}, {"Русский", "ru"}}, o.Lang)
		if err != nil {
			return err
		}
		useLanguage(o.Lang, system)
		fmt.Println(tr("MacDown — видео и аудио. Настройки запоминаются автоматически."))
		for {
			s, e := m.line(tr("Вставьте одну или несколько ссылок через пробел"), "")
			if e != nil {
				return e
			}
			urls = strings.Fields(s)
			if len(urls) == 0 {
				continue
			}
			if e = validateLinks(urls, o); e != nil {
				fmt.Println(e)
				continue
			}
			break
		}
		if !o.Formats {
			o.Mode, err = m.selectItem(ctx, tr("Что скачать?"), []choice{{tr("Видео со звуком"), "video"}, {tr("Только звук"), "audio"}}, o.Mode)
			if err != nil {
				return err
			}
			if o.Mode == "audio" {
				items := []choice{}
				for _, a := range []string{"mp3", "m4a", "opus", "flac", "wav", "best"} {
					items = append(items, choice{a, a})
				}
				o.Audio, err = m.selectItem(ctx, tr("Формат аудио"), items, o.Audio)
				if err != nil {
					return err
				}
			}
			for {
				o.Out, err = m.line(tr("Папка сохранения"), o.Out)
				if err != nil {
					return err
				}
				if strings.TrimSpace(o.Out) != "" {
					break
				}
			}
		}
	}
	if err = validateLinks(urls, o); err != nil {
		return err
	}
	o.Out = expandHome(o.Out, home)
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
					if interactive {
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
			fmt.Fprintln(os.Stderr, tr("Не удалось:"), err)
			continue
		}
		done++
		if !o.Formats {
			if e := saveSettings(configPath, o); e != nil {
				fmt.Fprintln(os.Stderr, tr("Не удалось сохранить настройки:"), e)
			}
		}
	}
	fmt.Printf(tr("\nЗавершено: %d. Ошибок: %d.\n"), done, failed)
	if failed > 0 {
		return errors.New(tr("часть ссылок не обработана; причины указаны выше"))
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

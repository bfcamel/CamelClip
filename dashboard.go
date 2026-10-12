package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func screen(m *menu, title string, o options, notice string) {
	if m.terminal {
		fmt.Fprint(m.out, "\033[2J\033[H\033[1;36m")
	}
	fmt.Fprintf(m.out, "  MACDOWN  %s\n", version)
	if m.terminal {
		fmt.Fprint(m.out, "\033[0m")
	}
	fmt.Fprintf(m.out, "  %s\n  %s: %s\n", title, tr("Папка сохранения"), safeText(o.Out))
	fmt.Fprintln(m.out, "  ────────────────────────────────────────────────────────────")
	if notice != "" {
		for _, line := range strings.Split(notice, "\n") {
			fmt.Fprintln(m.out, safeText(line))
		}
	}
	fmt.Fprintln(m.out)
}
func dashboard(ctx context.Context, m *menu, o *options, configPath, system, home string) error {
	if m.terminal {
		return panelDashboard(ctx, m, o, configPath, system, home)
	}
	if m.terminal {
		fmt.Fprint(m.out, "\033[?1049h")
		defer fmt.Fprint(m.out, "\033[?25h\033[?1049l")
	}
	notice := ""
	for {
		if ctx.Err() != nil {
			return errCanceled
		}
		screen(m, tr("Главное меню"), *o, notice)
		notice = ""
		files, e := listMedia(o.Out)
		if e == nil {
			var size int64
			for _, f := range files {
				size += f.Size
			}
			fmt.Fprintf(m.out, tr("В библиотеке: %d файлов · %s\n\n"), len(files), fileSize(size))
		} else {
			fmt.Fprintln(m.out, e)
		}
		action, err := m.selectItem(ctx, tr("Что сделать?"), []choice{{tr("↓ Скачать видео"), "video"}, {tr("♫ Скачать аудио"), "audio"}, {tr("Мои файлы"), "files"}, {tr("Настройки"), "settings"}, {tr("Диагностика"), "doctor"}, {tr("Выход"), "exit"}}, "video")
		if errors.Is(err, io.EOF) {
			return nil
		}
		if errors.Is(err, errCanceled) && ctx.Err() == nil {
			return nil
		}
		if err != nil {
			return err
		}
		switch action {
		case "exit":
			return nil
		case "settings":
			err = editSettings(ctx, m, o, configPath, system, home)
		case "files":
			err = library(ctx, m, *o)
		case "doctor":
			screen(m, tr("Диагностика"), *o, "")
			err = doctor(ctx)
			if err != nil {
				fmt.Fprintln(m.out, err)
			}
			_, err = m.line(tr("Enter — главное меню"), "")
		default:
			candidate := *o
			candidate.Mode = action
			candidate.Formats = false
			screen(m, tr("Новая загрузка"), candidate, tr("Пустая строка — вернуться в меню."))
			var urls []string
			for {
				var text string
				text, err = m.line(tr("Вставьте одну или несколько ссылок через пробел"), "")
				if err != nil || text == "" {
					break
				}
				urls = strings.Fields(text)
				if e = validateLinks(urls, candidate); e != nil {
					fmt.Fprintln(m.out, e)
					urls = nil
					continue
				}
				break
			}
			if err == nil && len(urls) > 0 {
				err = runQueue(ctx, urls, candidate, configPath, m)
				// Reload persisted quality/mode after successful queue items, keeping the fixed folder.
				_ = loadSettings(configPath, o)
				if err == nil {
					notice = tr("Готово! Файлы доступны в разделе «Мои файлы».")
				}
			}
		}
		if ctx.Err() != nil {
			return errCanceled
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil && !errors.Is(err, errCanceled) {
			notice = err.Error()
		}
	}
}
func writableFolder(path, home string) (string, error) {
	path = expandHome(strings.TrimSpace(path), home)
	if path == "" {
		return "", errors.New(tr("папка сохранения не может быть пустой"))
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(path, 0755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(path, ".macdown-write-check-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	f.Close()
	if err = os.Remove(name); err != nil {
		return "", err
	}
	return path, nil
}
func editSettings(ctx context.Context, m *menu, o *options, configPath, system, home string) error {
	notice := ""
	for {
		screen(m, tr("Настройки"), *o, notice)
		notice = ""
		items := []choice{{tr("Папка сохранения") + ": " + shortName(o.Out, 36), "folder"}, {tr("Язык") + ": " + o.Lang, "language"}, {tr("Качество") + ": " + o.Quality, "quality"}, {tr("Формат аудио") + ": " + o.Audio, "audio"}, {tr("← Главное меню"), "back"}}
		action, err := m.selectItem(ctx, tr("Выберите настройку"), items, "")
		if err != nil {
			return err
		}
		if action == "back" {
			return nil
		}
		candidate := *o
		switch action {
		case "folder":
			candidate.Out, err = m.line(tr("Папка сохранения"), o.Out)
			if err == nil {
				candidate.Out, err = writableFolder(candidate.Out, home)
			}
		case "language":
			candidate.Lang, err = m.selectItem(ctx, "Language / Язык", []choice{{tr("Системный") + " (" + system + ")", "auto"}, {"English", "en"}, {"Русский", "ru"}}, o.Lang)
		case "quality":
			choices := []choice{{tr("Лучшее доступное"), "best"}}
			for _, q := range []string{"2160", "1440", "1080", "720", "480", "360"} {
				choices = append(choices, choice{q + "p", q})
			}
			candidate.Quality, err = m.selectItem(ctx, tr("Качество"), choices, o.Quality)
		case "audio":
			choices := []choice{}
			for _, a := range []string{"mp3", "m4a", "opus", "flac", "wav", "best"} {
				choices = append(choices, choice{a, a})
			}
			candidate.Audio, err = m.selectItem(ctx, tr("Формат аудио"), choices, o.Audio)
		}
		if ctx.Err() != nil || errors.Is(err, io.EOF) {
			return err
		}
		if errors.Is(err, errCanceled) {
			continue
		}
		if err == nil {
			err = saveSettings(configPath, candidate)
		}
		if err != nil {
			notice = err.Error()
			continue
		}
		*o = candidate
		useLanguage(o.Lang, system)
		notice = tr("Настройки сохранены.")
	}
}

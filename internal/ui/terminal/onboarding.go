// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import (
	"context"
	"fmt"
)

func firstRun(ctx context.Context, m *menu, o *options, path, system, home string) error {
	return setup(ctx, m, o, path, system, home, func(ctx context.Context, path string, o options) error { return doctor(ctx, path, o, false) })
}

// Setup saves only after confirmation. CLI commands and existing preferences
// bypass the wizard; browser access remains an explicit choice.
func setup(ctx context.Context, m *menu, o *options, path, system, home string, check func(context.Context, string, options) error) error {
	candidate := *o
	screen(m, "Welcome to Clipnaro / Добро пожаловать", candidate, "")
	preferred := candidate.Lang
	if preferred == "auto" || preferred == "" {
		preferred = system
	}
	language, err := m.selectItem(ctx, "Language / Язык", []choice{{"English", "en"}, {"Русский", "ru"}}, preferred)
	if err != nil {
		return err
	}
	candidate.Lang = language
	useLanguage(language, system)
	fmt.Fprintln(m.out, tr("Скачивание, обработка и проверка видео. Очередь сохраняется автоматически."))
	for {
		folder, e := m.line(tr("Куда сохранять файлы?"), candidate.Out)
		if e != nil {
			return e
		}
		candidate.Out, err = writableFolder(folder, home)
		if err == nil {
			break
		}
		fmt.Fprintln(m.out, err)
	}
	profile, err := m.selectItem(ctx, tr("Формат видео — можно изменить позже"), []choice{{"H.264 / MP4 — " + tr("максимальная совместимость"), "h264"}, {"HEVC / MKV", "hevc"}, {"VP9 / WebM", "vp9"}, {"AV1 / MKV", "av1"}}, videoCodec(candidate))
	if err != nil {
		return err
	}
	candidate.Codec = profile
	candidate.Container = map[string]string{"h264": "mp4", "hevc": "mkv", "vp9": "webm", "av1": "mkv"}[profile]
	fmt.Fprintln(m.out, tr("Cookies браузера нужны для роликов с ограничением доступа. Они используются только с вашего разрешения."))
	candidate.Cookies, err = m.selectItem(ctx, tr("Использовать вход из браузера?"), []choice{{tr("Не использовать"), ""}, {"Firefox", "firefox"}, {"Chrome", "chrome"}, {"Edge", "edge"}, {"Brave", "brave"}}, candidate.Cookies)
	if err != nil {
		return err
	}
	for {
		screen(m, tr("Проверка перед началом работы"), candidate, "")
		checkErr := check(ctx, path, candidate)
		if ctx.Err() != nil {
			return errCanceled
		}
		fmt.Fprintf(m.out, "\n%s\n", tr("v/a — добавить видео/аудио · Tab — сменить панель · s — настройки · q — выход"))
		label := tr("Сохранить и открыть Clipnaro")
		if checkErr != nil {
			fmt.Fprintln(m.out, tr("Есть проблемы с зависимостями или хранилищем. Их можно исправить и повторить проверку."))
			label = tr("Сохранить и открыть — исправлю позже")
		}
		action, e := m.selectItem(ctx, tr("Начать работу?"), []choice{{label, "start"}, {tr("Повторить проверку"), "retry"}, {tr("Выйти без сохранения"), "quit"}}, "start")
		if e != nil {
			return e
		}
		if action == "quit" {
			return errCanceled
		}
		if action == "retry" {
			continue
		}
		if err = saveSettings(path, candidate); err != nil {
			return err
		}
		*o = candidate
		return nil
	}
}

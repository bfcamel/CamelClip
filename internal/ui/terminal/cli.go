// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bfcamel/video-downloader/internal/diagnostics"
)

const version = "0.10.0"

var buildCommit = "development"

type usageError struct{ message string }

func (e *usageError) Error() string { return e.message }

type cliRequest struct {
	command string
	urls    []string
	list    string
	help    bool
	json    bool
}

var commandNames = []string{"download", "audio", "formats", "setup", "config", "doctor", "version", "help"}

func parseCLI(args []string, o *options) (cliRequest, error) {
	r := cliRequest{}
	// Locate the command after any leading global flags without confusing flag values with commands.
	valueFlags := map[string]bool{"lang": true, "list": true, "quality": true, "audio-format": true, "out": true, "cookies-browser": true, "video-codec": true, "video-format": true}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") {
			name := strings.TrimLeft(strings.SplitN(a, "=", 2)[0], "-")
			if valueFlags[name] && !strings.Contains(a, "=") {
				i++
			}
			continue
		}
		if contains(commandNames, a) {
			r.command = a
			args = append(append([]string{}, args[:i]...), args[i+1:]...)
		} else {
			return r, &usageError{fmt.Sprintf(tr("Неизвестная команда: %s. Справка: clipnaro help"), a)}
		}
		break
	}
	if r.command == "help" {
		r.help = true
		r.command = ""
		if len(args) > 0 && contains(commandNames, args[0]) {
			r.command = args[0]
			args = args[1:]
		}
	}
	fs := flag.NewFlagSet("clipnaro", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.Lang, "lang", o.Lang, "auto, ru, en")
	fs.BoolVar(&r.help, "help", r.help, "")
	fs.BoolVar(&r.help, "h", r.help, "")
	if r.command == "doctor" {
		fs.BoolVar(&r.json, "json", false, "")
	}
	var showVersion bool
	fs.BoolVar(&showVersion, "version", false, "")
	transfer := r.command == "download" || r.command == "audio" || r.command == "formats"
	if transfer {
		fs.StringVar(&r.list, "list", "", "")
		fs.StringVar(&o.Quality, "quality", o.Quality, "")
		fs.StringVar(&o.Audio, "audio-format", o.Audio, "")
		fs.StringVar(&o.Out, "out", o.Out, "")
		fs.StringVar(&o.Cookies, "cookies-browser", o.Cookies, "")
		fs.StringVar(&o.Codec, "video-codec", videoCodec(*o), "")
		fs.StringVar(&o.Container, "video-format", videoContainer(*o), "")

	}
	// Go's flag parser stops at the first positional value. Reorder only known flags, respecting -- and flag values.
	flags := []string{}
	pos := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			pos = append(pos, a)
			continue
		}
		name := strings.TrimLeft(strings.SplitN(a, "=", 2)[0], "-")
		f := fs.Lookup(name)
		if f == nil {
			return r, &usageError{fmt.Sprintf(tr("Неизвестный параметр: %s. Справка: clipnaro help"), a)}
		}
		flags = append(flags, a)
		if b, ok := f.Value.(interface{ IsBoolFlag() bool }); (!ok || !b.IsBoolFlag()) && !strings.Contains(a, "=") {
			i++
			if i >= len(args) {
				return r, &usageError{fmt.Sprintf(tr("Параметру %s нужно значение"), a)}
			}
			flags = append(flags, args[i])
		}
	}
	if err := fs.Parse(flags); err != nil {
		return r, &usageError{err.Error()}
	}
	if !contains([]string{"auto", "ru", "en"}, o.Lang) {
		return r, &usageError{tr("язык должен быть auto, ru или en")}
	}
	if showVersion {
		r.command = "version"
		transfer = false
	}
	if !transfer && len(pos) > 0 {
		return r, &usageError{tr("Эта команда не принимает ссылки или дополнительные аргументы")}
	}
	r.urls = append(r.urls, pos...)
	switch r.command {
	case "download":
		o.Mode = "video"
	case "audio":
		o.Mode = "audio"
	case "formats":
		o.Formats = true
	}
	if transfer && r.command != "" && len(r.urls) == 0 && r.list == "" && !r.help {
		return r, &usageError{tr("Укажите URL или --list FILE. Для меню запустите clipnaro без команды.")}
	}
	return r, nil
}
func printHelp(command string) {
	fmt.Printf("Clipnaro %s\n\n", version)
	if command != "" {
		fmt.Printf("clipnaro %s", command)
		if contains([]string{"download", "audio", "formats"}, command) {
			fmt.Print(" [options] URL...")
		}
		fmt.Print("\n\n")
	}
	fmt.Println(tr(`Использование:
  clipnaro                         Интерактивное меню
  clipnaro download URL...         Видео со звуком в выбранном формате
  clipnaro audio URL...            Только звук
  clipnaro formats URL...          Доступные форматы
  clipnaro setup                   Настроить Clipnaro заново
  clipnaro config                  Настройки и путь к файлу (JSON)
  clipnaro doctor [--json]         Проверка зависимостей и хранилища
  clipnaro version                 Версия программы
  clipnaro help [COMMAND]          Справка

Параметры загрузки (до или после URL):
  --quality HEIGHT|best          Максимальная высота видео
  --video-codec h264|hevc|vp9|av1 Кодек видео (по умолчанию h264)
  --video-format mp4|mkv|webm     Контейнер видео (по умолчанию mp4)
  --audio-format mp3|m4a|opus|flac|wav|best
  --out DIR                      Папка сохранения
  --list FILE                    Файл со ссылками, по одной в строке
  --cookies-browser BROWSER      Авторизация через браузер

Общие параметры:
  --lang auto|en|ru               Язык
  --help, -h                     Справка
  --version                      Версия

Примеры:
  clipnaro download 'URL' --quality 1080
  clipnaro audio 'URL' --audio-format mp3
  clipnaro download --list links.txt`))
}
func showConfig(path string, o options) error {
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	return e.Encode(struct {
		Path     string   `json:"path"`
		Settings settings `json:"settings"`
		Language string   `json:"effective_language"`
	}{path, settings{Language: o.Lang, Mode: o.Mode, Quality: o.Quality, Audio: o.Audio, Out: o.Out, Browser: o.Cookies, Codec: videoCodec(o), Container: videoContainer(o)}, currentLanguage})
}
func doctor(ctx context.Context, path string, o options, asJSON bool) error {
	report := diagnostics.Collect(ctx, version, buildCommit, path, settingsFor(o))
	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			return err
		}
	} else {
		fmt.Printf("Clipnaro %s (%s/%s)\n", report.Version, report.OS, report.Arch)
		for _, check := range report.Checks {
			fmt.Printf("%-4s %s: %s\n", check.Status, check.ID, diagnosticText(check.Message))
			if check.Hint != "" {
				fmt.Printf("     %s\n", diagnosticText(check.Hint))
			}
		}
		fmt.Println(dependencyHint())
		fmt.Println(tr("Отчёт для обращения об ошибке: clipnaro doctor --json"))
	}
	if ctx.Err() != nil {
		return errCanceled
	}
	if report.Failed() {
		return errors.New(tr("Проверка выявила проблемы. См. подсказки в отчёте."))
	}
	return nil
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const version = "0.3.0"

type usageError struct{ message string }

func (e *usageError) Error() string { return e.message }

type cliRequest struct {
	command string
	urls    links
	list    string
	help    bool
}

var commandNames = []string{"download", "audio", "formats", "config", "doctor", "version", "help"}

func parseCLI(args []string, o *options) (cliRequest, error) {
	r := cliRequest{}
	// Locate the command after any leading global flags without confusing flag values with commands.
	valueFlags := map[string]bool{"lang": true, "url": true, "list": true, "mode": true, "quality": true, "audio-format": true, "out": true, "cookies-browser": true}
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
		} else if !strings.HasPrefix(a, "http://") && !strings.HasPrefix(a, "https://") {
			return r, &usageError{fmt.Sprintf(tr("Неизвестная команда: %s. Справка: macdown help"), a)}
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
	fs := flag.NewFlagSet("macdown", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.Lang, "lang", o.Lang, "auto, ru, en")
	fs.BoolVar(&r.help, "help", r.help, "")
	fs.BoolVar(&r.help, "h", r.help, "")
	var showVersion bool
	fs.BoolVar(&showVersion, "version", false, "")
	transfer := r.command == "" || r.command == "download" || r.command == "audio" || r.command == "formats"
	if transfer {
		fs.Var(&r.urls, "url", "")
		fs.StringVar(&r.list, "list", "", "")
		fs.StringVar(&o.Quality, "quality", o.Quality, "")
		fs.StringVar(&o.Audio, "audio-format", o.Audio, "")
		fs.StringVar(&o.Out, "out", o.Out, "")
		fs.StringVar(&o.Cookies, "cookies-browser", "", "")
		// Legacy flags remain available only without a subcommand, so explicit commands always mean what they say.
		if r.command == "" {
			fs.StringVar(&o.Mode, "mode", o.Mode, "")
			fs.BoolVar(&o.Formats, "formats", false, "")
		}
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
			return r, &usageError{fmt.Sprintf(tr("Неизвестный параметр: %s. Справка: macdown help"), a)}
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
		return r, &usageError{tr("Укажите URL или --list FILE. Для меню запустите macdown без команды.")}
	}
	return r, nil
}
func printHelp(command string) {
	fmt.Printf("MacDown %s\n\n", version)
	if command != "" {
		fmt.Printf("macdown %s", command)
		if contains([]string{"download", "audio", "formats"}, command) {
			fmt.Print(" [options] URL...")
		}
		fmt.Print("\n\n")
	}
	fmt.Println(tr(`Использование:
  macdown                         Интерактивное меню
  macdown download URL...         Видео со звуком в MP4
  macdown audio URL...            Только звук
  macdown formats URL...          Доступные форматы
  macdown config                  Настройки и путь к файлу (JSON)
  macdown doctor                  Проверка зависимостей
  macdown version                 Версия программы
  macdown help [COMMAND]          Справка

Параметры загрузки (до или после URL):
  --quality HEIGHT|best          Максимальная высота видео
  --audio-format mp3|m4a|opus|flac|wav|best
  --out DIR                      Папка сохранения
  --url URL                      Добавить ссылку (можно повторять)
  --list FILE                    Файл со ссылками, по одной в строке
  --cookies-browser BROWSER      Авторизация через браузер

Общие параметры:
  --lang auto|en|ru               Язык
  --help, -h                     Справка
  --version                      Версия

Примеры:
  macdown download 'URL' --quality 1080
  macdown audio 'URL' --audio-format mp3
  macdown download --list links.txt

Старый синтаксис macdown -url URL -mode audio также поддерживается.`))
}
func showConfig(path string, o options) error {
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	return e.Encode(struct {
		Path     string   `json:"path"`
		Settings settings `json:"settings"`
		Language string   `json:"effective_language"`
	}{path, settings{o.Lang, o.Mode, o.Quality, o.Audio, o.Out}, currentLanguage})
}
func doctor(ctx context.Context) error {
	fmt.Printf("MacDown %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
	missing := false
	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "deno"} {
		path, err := exec.LookPath(name)
		optional := name == "deno"
		if err != nil {
			if optional {
				fmt.Printf("WARN %s: %s\n", name, tr("рекомендуется для YouTube"))
			} else {
				missing = true
				fmt.Printf("FAIL %s: %s\n", name, tr("не найден"))
			}
			continue
		}
		args := []string{"--version"}
		if name == "ffmpeg" || name == "ffprobe" {
			args = []string{"-version"}
		}
		check, cancel := context.WithTimeout(ctx, 5*time.Second)
		out, err := exec.CommandContext(check, path, args...).CombinedOutput()
		cancel()
		if ctx.Err() != nil {
			return errCanceled
		}
		if err != nil {
			if !optional {
				missing = true
			}
			fmt.Printf("FAIL %s: %s\n", name, tr("не удалось проверить версию"))
			continue
		}
		first := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
		fmt.Printf("OK   %s: %s\n     %s\n", name, safeText(first), path)
	}
	fmt.Println(tr("Зависимости: brew install yt-dlp ffmpeg deno"))
	fmt.Println(tr("Проверка версий не гарантирует доступ к сайтам; EJS: https://github.com/yt-dlp/yt-dlp/wiki/EJS"))
	if missing {
		return errors.New(tr("отсутствуют или не работают обязательные зависимости"))
	}
	return nil
}

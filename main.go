package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type options struct {
	URL, Mode, Quality, Audio, Out, Cookies string
	Formats                                 bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Ошибка:", err)
		os.Exit(1)
	}
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	o := options{}
	flag.StringVar(&o.URL, "url", "", "Ссылка на видео")
	flag.StringVar(&o.Mode, "mode", "video", "video — видео со звуком; audio — только звук")
	flag.StringVar(&o.Quality, "quality", "best", "Максимальная высота: 360, 480, 720, 1080, 1440, 2160 или best")
	flag.StringVar(&o.Audio, "audio-format", "mp3", "Аудиоформат: mp3, m4a, opus, flac, wav, best")
	flag.StringVar(&o.Out, "out", filepath.Join(home, "Downloads", "MacDown"), "Папка сохранения")
	flag.StringVar(&o.Cookies, "cookies-browser", "", "Браузер для авторизации: safari, chrome, firefox, edge, brave")
	flag.BoolVar(&o.Formats, "formats", false, "Показать доступные форматы без скачивания")
	flag.Parse()
	if flag.NArg() > 0 {
		return errors.New("передайте ссылку через -url; справка: -h")
	}
	if o.URL == "" {
		if err := interactive(&o); err != nil {
			return err
		}
	}
	if err := validate(o); err != nil {
		return err
	}
	o.Out = expandHome(o.Out, home)
	ytdlp, err := exec.LookPath("yt-dlp")
	if err != nil {
		return errors.New("не найден yt-dlp. Установите зависимости: brew install yt-dlp ffmpeg deno")
	}
	if !o.Formats {
		for _, name := range []string{"ffmpeg", "ffprobe"} {
			if _, err := exec.LookPath(name); err != nil {
				return fmt.Errorf("не найден %s. Выполните: brew install ffmpeg", name)
			}
		}
		if err := os.MkdirAll(o.Out, 0755); err != nil {
			return fmt.Errorf("создание папки: %w", err)
		}
		fmt.Println("Папка сохранения:", o.Out)
	}
	// No shell: the URL and all user values remain individual arguments.
	// Inherited terminal process group lets Ctrl+C reach yt-dlp and FFmpeg.
	cmd := exec.Command(ytdlp, arguments(o)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("yt-dlp завершился с ошибкой: %w. Подробности выше; обновление: brew upgrade yt-dlp", err)
	}
	if !o.Formats {
		fmt.Println("Готово.")
	}
	return nil
}

func validate(o options) error {
	u, err := url.Parse(o.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("нужна корректная ссылка http:// или https://")
	}
	if o.Mode != "video" && o.Mode != "audio" {
		return errors.New("mode должен быть video или audio")
	}
	if o.Quality != "best" {
		n, err := strconv.Atoi(o.Quality)
		if err != nil || n < 1 || n > 16384 {
			return errors.New("quality должен быть best или числом от 1 до 16384")
		}
	}
	if !contains([]string{"mp3", "m4a", "opus", "flac", "wav", "best"}, o.Audio) {
		return errors.New("неподдерживаемый аудиоформат")
	}
	if o.Cookies != "" && !contains([]string{"safari", "chrome", "firefox", "edge", "brave"}, o.Cookies) {
		return errors.New("неподдерживаемый браузер")
	}
	if strings.TrimSpace(o.Out) == "" {
		return errors.New("папка сохранения не может быть пустой")
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func arguments(o options) []string {
	// Let yt-dlp redraw progress in place when attached to a terminal.
	// --print implies quiet mode, so explicitly keep status and progress visible.
	a := []string{"--ignore-config", "--no-playlist", "--no-overwrites", "--no-quiet", "--progress"}
	if o.Cookies != "" {
		a = append(a, "--cookies-from-browser", o.Cookies)
	}
	if o.Formats {
		return append(a, "--list-formats", "--", o.URL)
	}
	a = append(a, "--paths", o.Out, "--output", "%(title).150B [%(id)s].%(ext)s", "--print", "after_move:Сохранено: %(filepath)s")
	if o.Mode == "audio" {
		a = append(a, "--format", "bestaudio/best", "--extract-audio", "--audio-format", o.Audio, "--audio-quality", "0")
	} else {
		filter := ""
		if o.Quality != "best" {
			filter = "[height<=" + o.Quality + "]"
		}
		// Every fallback respects the height limit and includes audio.
		selector := "bestvideo" + filter + "+bestaudio/best" + filter
		a = append(a, "--format", selector, "--merge-output-format", "mkv", "--remux-video", "mkv")
	}
	return append(a, "--", o.URL)
}

func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func interactive(o *options) error {
	return prompt(o, os.Stdin, os.Stdout)
}

func prompt(o *options, input io.Reader, output io.Writer) error {
	r := bufio.NewReader(input)
	ask := func(label, fallback string) (string, error) {
		fmt.Fprintf(output, "%s [%s]: ", label, fallback)
		line, err := r.ReadString('\n')
		if err != nil && (err != io.EOF || line == "") {
			return "", errors.New("ввод завершён; используйте флаги из -h")
		}
		line = strings.TrimSpace(line)
		if line == "" {
			line = fallback
		}
		return line, nil
	}
	fmt.Fprintln(output, "MacDown — скачать видео или аудио. Enter выбирает значение в скобках.")
	var err error
	if o.URL, err = ask("Ссылка", ""); err != nil {
		return err
	}
	if o.Formats {
		return nil
	}
	defaultMode := "1"
	if o.Mode == "audio" {
		defaultMode = "2"
	}
	if o.Mode, err = ask("Режим: 1 — видео со звуком, 2 — только звук", defaultMode); err != nil {
		return err
	}
	switch o.Mode {
	case "1":
		o.Mode = "video"
	case "2":
		o.Mode = "audio"
	}
	if o.Mode != "video" && o.Mode != "audio" {
		return errors.New("выберите режим 1 или 2")
	}
	if o.Mode == "audio" {
		if o.Audio, err = ask("Формат: mp3 / m4a / opus / flac / wav / best", o.Audio); err != nil {
			return err
		}
	} else {
		if o.Quality, err = ask("Качество: best / 360 / 480 / 720 / 1080 / 1440 / 2160", o.Quality); err != nil {
			return err
		}
	}
	if o.Out, err = ask("Папка сохранения", o.Out); err != nil {
		return err
	}
	return err
}

package main

import (
	"errors"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

type options struct {
	URL, Mode, Quality, Audio, Out, Cookies, Lang string
	Formats                                       bool
}

func validate(o options) error {
	if o.Lang != "" && !contains([]string{"auto", "ru", "en"}, o.Lang) {
		return errors.New(tr("язык должен быть auto, ru или en"))
	}
	u, err := url.Parse(o.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New(tr("нужна корректная ссылка http:// или https://"))
	}
	if o.Mode != "video" && o.Mode != "audio" {
		return errors.New(tr("mode должен быть video или audio"))
	}
	if o.Quality != "best" {
		n, err := strconv.Atoi(o.Quality)
		if err != nil || n < 1 || n > 16384 {
			return errors.New(tr("quality должен быть best или числом от 1 до 16384"))
		}
	}
	if !contains([]string{"mp3", "m4a", "opus", "flac", "wav", "best"}, o.Audio) {
		return errors.New(tr("неподдерживаемый аудиоформат"))
	}
	if o.Cookies != "" && !contains([]string{"safari", "chrome", "firefox", "edge", "brave"}, o.Cookies) {
		return errors.New(tr("неподдерживаемый браузер"))
	}
	if strings.TrimSpace(o.Out) == "" {
		return errors.New(tr("папка сохранения не может быть пустой"))
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
	// progressWriter redraws download updates when attached to a terminal.
	// --print implies quiet mode, so explicitly keep status and progress visible.
	a := []string{"--ignore-config", "--no-playlist", "--no-overwrites", "--no-quiet", "--progress"}
	if o.Cookies != "" {
		a = append(a, "--cookies-from-browser", o.Cookies)
	}
	if o.Formats {
		return append(a, "--list-formats", "--", o.URL)
	}
	a = append(a, "--paths", o.Out, "--output", "%(title).150B [%(id)s].%(ext)s", "--print", tr("after_move:Сохранено: %(filepath)s"))
	if o.Mode == "audio" {
		a = append(a, "--format", "bestaudio/best", "--extract-audio", "--audio-format", o.Audio, "--audio-quality", "0")
	} else {
		filter := ""
		if o.Quality != "best" {
			filter = "[height<=" + o.Quality + "]"
		}
		// Every fallback respects the height limit and includes audio.
		selector := "bestvideo" + filter + "+bestaudio/best" + filter
		a = append(a, "--format", selector, "--merge-output-format", "mp4", "--remux-video", "mp4")
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

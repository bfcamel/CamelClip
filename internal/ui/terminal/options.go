// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"errors"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bfcamel/video-downloader/internal/core"
)

type options struct {
	URL, Mode, Quality, Audio, Out, Cookies, Lang string
	Codec, Container                              string
	Formats                                       bool
	duration                                      float64
	runtime                                       *Dependencies
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
	if o.Cookies != "" && !validBrowser(o.Cookies) {
		return errors.New(tr("неподдерживаемый браузер"))
	}
	if err := validateVideoProfile(o); err != nil {
		return err
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

func validBrowser(value string) bool {
	name := strings.SplitN(value, ":", 2)[0]
	return contains([]string{"safari", "chrome", "firefox", "edge", "brave"}, name) && !strings.ContainsAny(value, "\r\n\x00")
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

func outputVariant(o options) string {
	if o.Mode == "audio" {
		return "audio-" + o.Audio
	}
	return "video-" + o.Quality + "-" + videoCodec(o) + "-" + videoContainer(o)
}

func request(o options) core.Request {
	return core.Request{URL: o.URL, Mode: o.Mode, Quality: o.Quality, Audio: o.Audio, Out: o.Out, Cookies: o.Cookies, Codec: o.Codec, Container: o.Container, Duration: o.duration}
}

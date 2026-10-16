// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bfcamel/video-downloader/internal/core"
)

type mediaInfo = core.MediaInfo
type mediaFormat = core.MediaFormat

func qualities(info mediaInfo) []choice {
	audio := false
	for _, f := range info.Formats {
		if !f.DRM && f.ACodec != "" && f.ACodec != "none" {
			audio = true
		}
	}
	heights := map[int]bool{}
	for _, f := range info.Formats {
		if !f.DRM && f.Height > 0 && f.VCodec != "" && f.VCodec != "none" && (audio || f.ACodec != "" && f.ACodec != "none") {
			heights[f.Height] = true
		}
	}
	ns := []int{}
	for n := range heights {
		ns = append(ns, n)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ns)))
	items := []choice{{tr("Лучшее доступное"), "best"}}
	for _, n := range ns {
		items = append(items, choice{fmt.Sprintf(tr("До %dp"), n), strconv.Itoa(n)})
	}
	return items
}

func explain(raw string) string {
	summary := explainSummary(raw)
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r", "\n"), "\n") {
		if strings.Contains(line, "ERROR:") && !strings.Contains(summary, strings.TrimSpace(line)) {
			return summary + "\n" + safeText(strings.TrimSpace(line))
		}
	}
	return summary
}
func explainSummary(raw string) string {
	s := strings.ToLower(raw)
	switch {
	case strings.Contains(s, "no space left"):
		return tr("Недостаточно места на диске. Освободите место или выберите другую папку.")
	case strings.Contains(s, "permission denied"):
		return tr("Нет доступа к файлу или папке. Проверьте права и выбранную папку.")
	case strings.Contains(s, "cookies") && (strings.Contains(s, "failed") || strings.Contains(s, "could not")):
		return tr("Не удалось прочитать cookies браузера. Проверьте профиль и разрешения macOS или выберите другой браузер.")
	case strings.Contains(s, "not a bot"):
		return tr("YouTube просит подтвердить, что вы не бот. Проверьте cookies выбранного профиля Firefox и доступ через ту же сеть.")
	case strings.Contains(s, "sign in") || strings.Contains(s, "login") || strings.Contains(s, "private video") || strings.Contains(s, "age-restricted"):
		return tr("Ролик требует входа. Войдите в браузере и повторите с -cookies-browser firefox (или chrome/safari).")
	case strings.Contains(s, "429"):
		return tr("Сайт временно ограничил запросы. Подождите и повторите загрузку.")
	case strings.Contains(s, "403"):
		return tr("Сайт отказал в доступе. Обновите yt-dlp; если ролик требует входа, укажите -cookies-browser.")
	case strings.Contains(s, "not available in your country") || strings.Contains(s, "geo"):
		return tr("Ролик недоступен в вашем регионе.")
	case strings.Contains(s, "unavailable") || strings.Contains(s, "removed") || strings.Contains(s, "404"):
		return tr("Ролик удалён или недоступен. Проверьте ссылку и доступ в браузере.")
	case strings.Contains(s, "requested format"):
		return tr("Выбранное качество недоступно. Выберите другое качество или best.")
	case strings.Contains(s, "timed out") || strings.Contains(s, "resolve") || strings.Contains(s, "connection") || strings.Contains(s, "network"):
		return tr("Не удалось подключиться к сайту. Проверьте интернет и повторите попытку.")
	case strings.Contains(s, "unsupported url"):
		return tr("Эта ссылка не поддерживается. Укажите прямую ссылку на страницу ролика.")
	case strings.Contains(s, "drm"):
		return tr("Видео защищено DRM и не может быть скачано этой программой.")
	}
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r", "\n"), "\n") {
		if strings.Contains(line, "ERROR:") {
			return strings.TrimSpace(line) + tr("\nПопробуйте обновить yt-dlp: brew upgrade yt-dlp")
		}
	}
	return tr("Загрузка не завершена. Проверьте сообщения выше; обновление: brew upgrade yt-dlp.")
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type mediaFormat struct {
	Height int    `json:"height"`
	VCodec string `json:"vcodec"`
	ACodec string `json:"acodec"`
	DRM    bool   `json:"has_drm"`
}
type mediaInfo struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	Duration float64       `json:"duration"`
	Formats  []mediaFormat `json:"formats"`
	Type     string        `json:"_type"`
	Live     bool          `json:"is_live"`
}

func command(ctx context.Context, path string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, path, args...)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error { return syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }
	c.WaitDelay = 2 * time.Second
	return c
}
func inspect(ctx context.Context, path string, o options) (mediaInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	a := []string{"--ignore-config", "--no-playlist", "--skip-download", "--dump-single-json", "--no-warnings", "--socket-timeout", "20"}
	if o.Cookies != "" {
		a = append(a, "--cookies-from-browser", o.Cookies)
	}
	c := command(ctx, path, append(a, "--", o.URL)...)
	var out bytes.Buffer
	var tail tailBuffer
	c.Stdout = &out
	c.Stderr = &tail
	if err := c.Run(); err != nil {
		if ctx.Err() != nil {
			return mediaInfo{}, errors.New(tr("получение данных прервано или заняло больше 90 секунд"))
		}
		return mediaInfo{}, fmt.Errorf("%s", explain(tail.String()))
	}
	var info mediaInfo
	if err := json.Unmarshal(out.Bytes(), &info); err != nil {
		return info, errors.New(tr("yt-dlp вернул непонятные данные; обновите его: brew upgrade yt-dlp"))
	}
	if info.Type == "playlist" || info.Type == "multi_video" {
		return info, errors.New(tr("нужна ссылка на отдельный ролик, а не на плейлист"))
	}
	if info.Live {
		return info, errors.New(tr("это прямая трансляция; очередь поддерживает завершённые ролики"))
	}
	return info, nil
}
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

// Bound retained diagnostics; progress continues to stream without buffering.
type tailBuffer struct{ data []byte }

func (t *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	t.data = append(t.data, p...)
	if len(t.data) > 16384 {
		t.data = t.data[len(t.data)-16384:]
	}
	return n, nil
}
func (t *tailBuffer) String() string { return string(t.data) }
func download(ctx context.Context, path string, o options) error {
	// Keep progress compact even when yt-dlp sees its stderr as a pipe.
	var tail tailBuffer
	stat, _ := os.Stderr.Stat()
	renderer := &progressWriter{out: os.Stderr, tail: &tail, terminal: stat != nil && stat.Mode()&os.ModeCharDevice != 0}
	defer renderer.finish()
	c := command(ctx, path, arguments(o)...)
	c.Stdout = renderer
	c.Stderr = renderer
	if err := c.Run(); err != nil {
		if ctx.Err() != nil {
			return errCanceled
		}
		return fmt.Errorf("%s", explain(tail.String()))
	}
	return nil
}
func explain(raw string) string {
	s := strings.ToLower(raw)
	switch {
	case strings.Contains(s, "no space left"):
		return tr("Недостаточно места на диске. Освободите место или выберите другую папку.")
	case strings.Contains(s, "permission denied"):
		return tr("Нет доступа к файлу или папке. Проверьте права и выбранную папку.")
	case strings.Contains(s, "cookies") && (strings.Contains(s, "failed") || strings.Contains(s, "could not")):
		return tr("Не удалось прочитать cookies браузера. Проверьте профиль и разрешения macOS или выберите другой браузер.")
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

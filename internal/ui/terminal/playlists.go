// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import (
	"context"
	"errors"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
	"strconv"
	"strings"
	"time"
)

func playlistCLI(ctx context.Context, w *application.Workspace, pos []string, f catalogFlags, o options) error {
	s := w.Playlists
	if s == nil {
		return errors.New("playlist service unavailable")
	}
	id := ""
	if len(pos) > 1 {
		id = pos[1]
	}
	switch pos[0] {
	case "list":
		v, e := s.List(ctx, f.limit, f.offset)
		if e != nil {
			return e
		}
		return emitJSON(v)
	case "items":
		v, e := s.Items(ctx, id, f.limit, f.offset)
		if e != nil {
			return e
		}
		return emitJSON(v)
	case "add", "edit":
		p := core.PlaylistSubscription{PlaylistID: id, Name: f.name, Enabled: true, InitialMode: f.initial, IntervalMinutes: f.interval, Request: request(o)}
		if pos[0] == "edit" {
			page, e := s.List(ctx, 200, 0)
			if e != nil {
				return e
			}
			found := false
			for offset := 0; ; offset += 200 {
				if offset > 0 {
					page, e = s.List(ctx, 200, offset)
					if e != nil {
						return e
					}
				}
				for _, v := range page.Items {
					if v.ID == id {
						p = v
						found = true
						break
					}
				}
				if found || offset+200 >= page.Total {
					break
				}
			}
			if !found {
				return errors.New("subscription not found")
			}
		}
		if f.present["name"] {
			p.Name = f.name
		}
		if f.present["interval"] {
			p.IntervalMinutes = f.interval
		}
		if f.present["quality"] {
			p.Request.Quality = f.quality
		}
		if f.present["out"] {
			p.Request.Out = f.out
		}
		if f.present["mode"] {
			p.Request.Mode = f.mode
		}
		if f.present["codec"] {
			p.Request.Codec = f.codec
		}
		if f.present["audio"] {
			p.Request.Audio = f.audio
		}
		if f.present["container"] {
			p.Request.Container = f.container
		}
		if f.present["profile"] {
			p.S3Profiles = nil
			for _, v := range strings.Split(f.profile, ",") {
				if v = strings.TrimSpace(v); v != "" {
					p.S3Profiles = append(p.S3Profiles, v)
				}
			}
		}
		v, e := s.Save(ctx, p)
		if e != nil {
			return e
		}
		return emitJSON(v)
	case "pause", "resume":
		return s.SetEnabled(ctx, id, pos[0] == "resume")
	case "sync":
		return s.Check(ctx, id)
	case "backfill":
		if !f.yes {
			return errors.New("backfill requires --yes")
		}
		return s.Backfill(ctx, id)
	case "retry":
		if len(pos) < 3 {
			return errors.New("subscription ID and video ID required")
		}
		return s.Retry(ctx, id, pos[2])
	case "watch":
		if f.passwordFD >= 0 {
			if e := unlockCLI(ctx, w, f.passwordFD, false); e != nil {
				return e
			}
		}
		w.Cloud.Restrict(nil)
		w.Queue.Start()
		s.Start()
		fmt.Println("Monitoring playlists; press Ctrl+C to stop. S3 waits for vault unlock when locked.")
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-tick.C:
				if e := s.Err(); e != nil {
					return e
				}
				if e := w.Queue.Err(); e != nil {
					return e
				}
				if e := w.Cloud.Err(); e != nil {
					return e
				}
			}
		}
	}
	return errors.New("unknown playlists command")
}
func (u *panelUI) playlistKey(key string) bool {
	s := u.pool.workspace.Playlists
	if s == nil {
		u.log("Playlist service unavailable")
		return true
	}
	var p *core.PlaylistSubscription
	if u.assetIndex >= 0 && u.assetIndex < len(u.playlists) {
		v := u.playlists[u.assetIndex]
		p = &v
	}
	switch key {
	case "n", "e":
		u.playlistForm = core.PlaylistSubscription{Enabled: true, InitialMode: "new_only", IntervalMinutes: 15, Request: request(*u.o)}
		if key == "e" {
			if p == nil {
				return true
			}
			u.playlistForm = *p
		}
		u.playlistStep = 0
		u.nextPlaylistField()
	case "p":
		if p != nil {
			id, enabled := p.ID, !p.Enabled
			u.background(func() error { return s.SetEnabled(u.ctx, id, enabled) })
		}
	case "s":
		if p != nil {
			id := p.ID
			u.background(func() error { return s.Sync(u.ctx, id) })
		}
	case "b":
		if p != nil {
			u.action = "playlist-backfill"
			u.actionIDs = []string{p.ID}
			u.confirmAction(tr("Скачать существующие видео плейлиста?"))
		}
	case "enter":
		if p != nil {
			items, e := s.Items(u.ctx, p.ID, 50, 0)
			if e != nil {
				u.log(e.Error())
				return true
			}
			lines := []string{p.PlaylistID, p.LastError}
			for _, v := range items.Items {
				lines = append(lines, fmt.Sprintf("%s · %s · #%d · %s", v.Title, v.LocalState, v.JobID, v.State))
			}
			u.modal = "details"
			u.title = p.Name
			u.input = strings.Join(lines, "\n")
			u.detailOffset = 0
		}
	default:
		return false
	}
	return true
}
func (u *panelUI) nextPlaylistField() {
	p := u.playlistForm
	labels := []string{"Плейлист YouTube", "Название", "Начальный режим: new_only / all_existing", "Интервал проверки (минуты)", "Папка сохранения", "Качество: best / 1080 / 720", "Режим: video / audio", "Кодек: h264 / hevc / vp9 / av1", "Контейнер: mp4 / mkv / webm", "Аудиоформат: mp3 / m4a / opus / flac / wav / best", "Cookies браузера (пусто: отключены)", "S3: имена профилей через запятую (пусто: локально)"}
	names := []string{}
	for _, id := range p.S3Profiles {
		for _, profile := range u.profiles {
			if profile.ID == id {
				names = append(names, profile.Name)
			}
		}
	}
	values := []string{p.PlaylistID, p.Name, p.InitialMode, strconv.Itoa(p.IntervalMinutes), p.Request.Out, p.Request.Quality, p.Request.Mode, core.VideoCodec(p.Request), core.VideoContainer(p.Request), p.Request.Audio, p.Request.Cookies, strings.Join(names, ",")}
	if p.ID != "" && (u.playlistStep == 0 || u.playlistStep == 2) {
		u.playlistStep++
		u.nextPlaylistField()
		return
	}
	if u.playlistStep >= len(labels) {
		candidate := u.playlistForm
		if _, err := core.ValidateSubscription(candidate); err != nil {
			u.log(err.Error())
			u.playlistStep = 0
			u.nextPlaylistField()
			return
		}
		u.modal = ""
		u.background(func() error { _, e := u.pool.workspace.Playlists.Save(u.ctx, candidate); return e })
		return
	}
	if u.playlistStep == 10 {
		u.pickBrowser("playlist-browser", values[u.playlistStep])
		return
	}
	u.modal = "playlist-field"
	u.title = tr(labels[u.playlistStep])
	u.input = values[u.playlistStep]
}
func (u *panelUI) playlistField(value string) {
	p := &u.playlistForm
	switch u.playlistStep {
	case 0:
		p.PlaylistID = value
	case 1:
		p.Name = value
	case 2:
		p.InitialMode = value
	case 3:
		n, e := strconv.Atoi(value)
		if e != nil {
			u.log(e.Error())
			return
		}
		p.IntervalMinutes = n
	case 4:
		p.Request.Out = value
	case 5:
		p.Request.Quality = value
	case 6:
		p.Request.Mode = value
	case 7:
		p.Request.Codec = value
	case 8:
		p.Request.Container = value
	case 9:
		p.Request.Audio = value
	case 10:
		p.Request.Cookies = value
	case 11:
		p.S3Profiles = nil
		for _, name := range strings.Split(value, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			found := false
			for _, profile := range u.profiles {
				if profile.Name == name {
					p.S3Profiles = append(p.S3Profiles, profile.ID)
					found = true
					break
				}
			}
			if !found {
				u.log("S3 profile not found: " + name)
				return
			}
		}
	}
	u.playlistStep++
	u.nextPlaylistField()
}

func init() {
	for k, v := range map[string]string{
		"Плейлисты": "Playlists", "Скачать существующие видео плейлиста?": "Queue existing playlist videos?", "Плейлист YouTube": "YouTube playlist", "Название": "Name", "Начальный режим: new_only / all_existing": "Initial mode: new_only / all_existing", "Интервал проверки (минуты)": "Check interval (minutes)", "Папка сохранения": "Output folder", "Качество: best / 1080 / 720": "Quality: best / 1080 / 720", "Режим: video / audio": "Mode: video / audio", "Кодек: h264 / hevc / vp9 / av1": "Codec: h264 / hevc / vp9 / av1", "Контейнер: mp4 / mkv / webm": "Container: mp4 / mkv / webm", "Аудиоформат: mp3 / m4a / opus / flac / wav / best": "Audio format: mp3 / m4a / opus / flac / wav / best", "Cookies браузера (пусто: отключены)": "Browser cookies (blank: disabled)", "S3: имена профилей через запятую (пусто: локально)": "S3: comma-separated profile names (blank: local only)", "n:новый e:изменить p:пауза s:проверить b:существующие": "n:add e:edit p:pause s:sync b:backfill", "1–5:экраны c:CPU/RAM Enter:сведения ?:помощь q:выход": "1–5:views c:CPU/RAM Enter:details ?:help q:quit",
	} {
		if _, ok := english[k]; !ok {
			english[k] = v
		}
	}
}

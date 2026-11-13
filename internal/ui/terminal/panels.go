// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
)

type panelUI struct {
	companionResults                                  chan companionPanelResult
	companionCatalog                                  core.CompanionCatalog
	companionSnapshot                                 core.CaptionSnapshot
	clips                                             []core.ClipBatch
	clipForm                                          core.ClipRequest
	clipStep                                          int
	clipPlan                                          core.ClipPlan
	clipPlans                                         chan core.ClipPlan
	playlists                                         []core.PlaylistSubscription
	playlistForm                                      core.PlaylistSubscription
	playlistStep                                      int
	ctx                                               context.Context
	out                                               io.Writer
	o                                                 *options
	config, system, home                              string
	pool                                              *jobPool
	files                                             []libraryFile
	logs                                              []string
	focus, fileIndex, jobIndex, logOffset             int
	width, height                                     int
	modal, title, input, mode                         string
	addBrowser                                        string
	askQuality                                        bool
	choices                                           []choice
	choiceIndex, target                               int
	quit, dirty                                       bool
	parents                                           []modalFrame
	tab, selectedID, detailOffset                     int
	search                                            string
	showLog                                           bool
	lastFrame                                         string
	section, page, assetIndex, assetTotal             int
	assets                                            []core.Asset
	profiles                                          []core.S3Profile
	uploads                                           []core.UploadJob
	marked                                            map[string]bool
	assetKind, assetStatus, assetContainer, assetSort string
	assetCopies                                       *bool
	assetMinHeight                                    int
	assetSince, assetUntil                            time.Time
	action                                            string
	actionIDs                                         []string
	profileTargets                                    map[string]bool
	actions                                           chan string
	profileForm                                       core.S3Profile
	credentialForm                                    core.Credentials
	formStep                                          int
	formCredentials                                   bool
	newProfileAfterUnlock                             bool
	secretMode                                        string
	secretFirst                                       string
	rotationPassword                                  string
	bulkAction                                        string
	bulkIDs                                           []int
}

func panelDashboard(parent context.Context, m *menu, o *options, config, system, home string) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	stty := func(args ...string) ([]byte, error) {
		c := exec.Command("stty", args...)
		c.Stdin = os.Stdin
		return c.Output()
	}
	saved, err := stty("-g")
	if err != nil {
		return err
	}
	if _, err = stty("-icanon", "-echo", "min", "0", "time", "1"); err != nil {
		return err
	}
	defer stty(strings.TrimSpace(string(saved)))
	fmt.Fprint(m.out, "\033[?1049h\033[?25l\033[?2004h")
	defer fmt.Fprint(m.out, "\033[?2004l\033[?25h\033[?1049l")
	pool, err := openPersistentPool(ctx, *o, config, 2)
	if err != nil {
		return err
	}
	u := &panelUI{ctx: ctx, out: m.out, o: o, config: config, system: system, home: home, pool: pool, width: 100, height: 30, dirty: true}
	defer func() { cancel(); u.pool.wait() }()
	u.refresh()
	u.pool.schedule()
	u.log(tr("Tab / Shift+Tab — следующая / предыдущая вкладка"))
	decoder := keyDecoder{}
	nextRefresh := time.Time{}
	nextSize := time.Time{}
	nextDraw := time.Time{}
	var input [1024]byte
	for !u.quit && ctx.Err() == nil {
		now := time.Now()
		if now.After(nextRefresh) {
			if now.After(nextSize) {
				nextSize = now.Add(500 * time.Millisecond)
				if size, e := stty("size"); e == nil {
					var h, w int
					if _, e = fmt.Sscanf(string(size), "%d %d", &h, &w); e == nil && h > 0 && w > 0 {
						if h != u.height || w != u.width {
							u.height = h
							u.width = w
							u.dirty = true
							fmt.Fprint(u.out, "\033[2J")
						}
					}
				}
			}
			u.pool.refresh()
			u.receiveClipPlan()
			u.receiveCompanion()
			u.workspaceRefresh()
			u.resumeProfileCreation()
			if u.modal == "resources-panel" {
				u.input = u.resourcesText()
			}
			if u.actions != nil {
				for {
					select {
					case message := <-u.actions:
						u.log(message)
					default:
						goto drained
					}
				}
			drained:
			}
			u.reconcile()

			u.refresh()
			nextRefresh = now.Add(250 * time.Millisecond)
		}
		for pending := true; pending; {
			select {
			case event := <-u.pool.events:
				u.event(event)
			default:
				pending = false
			}
		}
		if err = u.pool.saveQueue(); err != nil {
			return err
		}
		if err = u.pool.saveQueue(); err != nil {
			return err
		}
		if u.dirty && now.After(nextDraw) {
			nextDraw = now.Add(250 * time.Millisecond)
			u.render()
			u.dirty = false
		}
		n, e := os.Stdin.Read(input[:])
		if e != nil && e != io.EOF {
			return e
		}
		keys := decoder.feed(input[:n], n == 0)
		for _, key := range keys {
			u.key(key)
			u.dirty = true
		}
	}
	if err = u.pool.saveQueue(); err != nil {
		return err
	}
	if parent.Err() != nil {
		return errCanceled
	}
	return nil
}
func (u *panelUI) log(s string) {
	if u.logOffset > 0 {
		u.logOffset++
	}
	u.logs = append(u.logs, time.Now().Format("15:04:05")+" "+safeText(s))
	if len(u.logs) > 200 {
		u.logs = u.logs[len(u.logs)-200:]
	}
	u.dirty = true
}
func (u *panelUI) refresh() {
	selectedPath := ""
	if u.fileIndex < len(u.files) && u.fileIndex >= 0 {
		selectedPath = u.files[u.fileIndex].Path
	}
	files, e := listMedia(u.o.Out)
	if e != nil {
		if len(u.logs) == 0 || !strings.Contains(u.logs[len(u.logs)-1], e.Error()) {
			u.log(e.Error())
		}
	} else {
		u.files = files
		for i, f := range files {
			if f.Path == selectedPath {
				u.fileIndex = i
				break
			}
		}
	}
	if u.fileIndex >= len(u.files) {
		u.fileIndex = maxInt(0, len(u.files)-1)
	}
	u.reconcile()
	u.dirty = true
}
func (u *panelUI) event(e application.Event) {
	// Metrics use the periodic authoritative snapshot to bound rendering work.
	if e.Kind == core.Progress {
		return
	}
	if e.Kind == core.Log {
		u.log(fmt.Sprintf("#%d %s", e.Job.ID, e.Detail))
		return
	}
	j := u.pool.apply(e)
	if j == nil {
		return
	}
	u.dirty = true
	if u.modal == "details" && u.target == j.id {
		u.input = detailsText(j)
		if j.state == "choose" {
			u.showChoices("quality", j.title, qualities(j.info), j.options.Quality)
		}
	}
	if e.Kind != core.Progress && e.Kind != core.Log {
		u.log(fmt.Sprintf("#%d %s: %s", j.id, j.title, jobState(j.state)))
		if j.state == "failed" || j.state == "canceled" && j.detail != "" {
			u.log(j.detail)
		}
		if j.state == "done" {
			u.refresh()
		}
	}
}
func jobState(s string) string {
	labels := map[string]string{"queued": "В очереди", "inspecting": "Получение данных", "choose": "Выберите качество: Enter", "ready": "Готов к загрузке", "waiting_download": "Ожидание скачивания", "waiting_processing": "Ожидание обработки", "waiting_verification": "Ожидание проверки", "waiting_disk_capacity": "Ожидание места на диске", "downloading": "Загрузка", "processing": "Обработка", "verifying": "Проверка", "done": "Готово", "failed": "Ошибка", "canceling": "Отмена…", "canceled": "Отменено"}
	return tr(labels[s])
}
func (u *panelUI) selectedJob() *job {
	items := u.visibleJobs()
	if len(items) == 0 {
		return nil
	}
	u.jobIndex = max(0, min(u.jobIndex, len(items)-1))
	return items[u.jobIndex]
}
func (u *panelUI) showChoices(kind, title string, choices []choice, preferred string) {
	u.modal = kind
	u.title = title
	u.choices = append(append([]choice(nil), choices...), choice{tr("← Назад"), "__back"})
	u.choiceIndex = 0
	for i, c := range choices {
		if c.Value == preferred {
			u.choiceIndex = i
		}
	}
}
func (u *panelUI) key(key string) {
	if u.pool != nil && u.pool.service != nil {
		u.pool.refresh()
		u.reconcile()
	}
	if u.modal != "" {
		u.modalKey(key)
		return
	}
	if u.workspaceKey(key) {
		return
	}
	if u.queueControlKey(key) {
		return
	}
	switch key {
	case "tab", "shift-tab":
		delta := 1
		if key == "shift-tab" {
			delta = 5
		}
		u.tab = (u.tab + delta) % 6
		u.reconcile()
	case "up", "down", "j", "k", "pageup", "pagedown":
		delta := 1
		if key == "up" || key == "k" || key == "pageup" {
			delta = -1
		}
		if key == "pageup" || key == "pagedown" {
			delta *= 10
		}
		items := u.visibleJobs()
		u.jobIndex = max(0, min(u.jobIndex+delta, len(items)-1))
		if len(items) > 0 {
			u.selectedID = items[u.jobIndex].id
		}
	case "a":
		u.showChoices("add-mode", tr("Добавить"), []choice{{tr("Видео"), "video"}, {tr("Аудио"), "audio"}, {tr("Видео: выбрать качество после анализа"), "manual"}}, "video")
	case "v":
		u.addBrowser = u.o.Cookies
		u.askQuality = false
		u.mode = "video"
		u.modal = "url"
		u.title = tr("Вставьте одну или несколько ссылок через пробел")
		u.input = ""
	case "/":
		u.modal = "search"
		u.title = tr("Поиск по названию и URL")
		u.input = u.search
	case "escape":
		u.search = ""
		u.reconcile()
	case "enter":
		j := u.selectedJob()
		if j == nil {
			return
		}
		u.target = j.id
		if j.state == "choose" {
			u.showChoices("quality", j.title, qualities(j.info), j.options.Quality)
		} else {
			u.modal = "details"
			u.title = j.title
			u.input = detailsText(j)
			u.detailOffset = 0
		}
	case "d":
		if j := u.selectedJob(); j != nil && contains([]string{"done", "failed", "canceled"}, j.state) {
			u.target = j.id
			u.showChoices("remove", tr("Удалить только запись? Файл сохранится."), []choice{{tr("Нет"), "no"}, {tr("Да"), "yes"}}, "no")
		}
	case "x":
		if j := u.selectedJob(); j != nil && !j.record.State.Terminal() {
			u.target = j.id
			u.showChoices("cancel", tr("Отменить задание?"), []choice{{tr("Нет"), "no"}, {tr("Да"), "yes"}}, "no")
		}
	case "r":
		if j := u.selectedJob(); j != nil && (j.state == "failed" || j.state == "canceled") {
			u.pool.retry(j, u.o.Cookies)
			u.log(tr("Повтор задания"))
		}
	case "o":
		if j := u.selectedJob(); j != nil && j.record.OutputPath != "" {
			u.open(j.record.OutputPath, true)
		}
	case "f":
		if err := os.MkdirAll(u.o.Out, 0755); err != nil {
			u.log(err.Error())
		} else {
			u.open(u.o.Out, false)
		}
	case "s":
		u.settings()
	case "?":
		u.modal = "help"
		u.title = tr("Справка")
		u.input = helpText()
		u.detailOffset = 0
	case "l":
		u.modal = "logs"
		u.title = tr("Журнал событий")
		u.input = strings.Join(u.logs, "\n")
		u.detailOffset = max(0, len(u.logs)-max(1, u.height-10))
	case "q":
		if u.hasActiveWork() {
			u.showChoices("quit", tr("Остановить работу и выйти? Очередь восстановится."), []choice{{tr("Продолжить работу"), "no"}, {tr("Остановить и выйти"), "yes"}}, "no")
		} else {
			u.quit = true
		}
	}
}
func (u *panelUI) open(path string, reveal bool) {
	// The platform opener never writes child-process output over the dashboard.
	if err := openMedia(u.ctx, path, reveal); err != nil {
		u.log(err.Error())
	}
}
func (u *panelUI) settings() {
	items := []choice{{tr("Папка сохранения") + ": " + u.o.Out, "folder"}, {tr("Язык") + ": " + u.o.Lang, "language"}, {tr("Качество") + ": " + u.o.Quality, "default-quality"}, {tr("Формат аудио") + ": " + u.o.Audio, "audio"}, {tr("Кодек видео") + ": " + codecLabel(videoCodec(*u.o)), "video-codec"}, {tr("Формат видео") + ": " + strings.ToUpper(videoContainer(*u.o)), "video-format"}, {tr("Браузер cookies") + ": " + u.o.Cookies, "browser"}, {tr("Лимиты и хранилище"), "limits"}}
	u.showChoices("settings", tr("Настройки"), items, "")
}
func (u *panelUI) limitsMenu() {
	items := []choice{{resourceTitle("downloads") + ": " + resourceValue(*u.o, "downloads"), "downloads"}, {resourceTitle("processing-limit") + ": " + resourceDisplay(*u.o, "processing-limit"), "processing-limit"}, {resourceTitle("global-rate") + ": " + resourceValue(*u.o, "global-rate"), "global-rate"}, {resourceTitle("job-rate") + ": " + resourceValue(*u.o, "job-rate"), "job-rate"}}
	for _, key := range extraResourceKeys {
		items = append(items, choice{resourceTitle(key) + ": " + resourceValue(*u.o, key), key})
	}
	u.showChoices("limits", tr("Лимиты и хранилище"), items, "")
}
func (u *panelUI) save(candidate options) bool {
	if err := saveSettings(u.config, candidate); err != nil {
		u.log(err.Error())
		return false
	}
	if u.pool != nil && u.pool.service != nil {
		_ = u.pool.service.SetPolicy(queuePolicy(candidate))
		if u.pool.workspace != nil {
			_ = u.pool.workspace.Cloud.SetOptions(uploadOptions(candidate))
		}
		if err := u.pool.service.SetLimits(settingsFor(candidate).Limits()); err != nil {
			u.log(err.Error())
			return false
		}
	}
	*u.o = candidate
	useLanguage(u.o.Lang, u.system)
	u.log(tr("Настройки сохранены."))
	u.refresh()
	return true
}
func (u *panelUI) modalKey(key string) {
	if u.modal == "bulk-confirm" && key == "enter" {
		yes := u.choices[u.choiceIndex].Value == "yes"
		u.modal = ""
		if yes {
			ids := append([]int(nil), u.bulkIDs...)
			action := u.bulkAction
			browser := u.o.Cookies
			u.background(func() error {
				failed := []string{}
				for _, r := range u.pool.service.BulkActionWithOptions(ids, action, application.RetryOptions{Browser: &browser}) {
					if r.Error != "" {
						failed = append(failed, fmt.Sprintf("#%d: %s", r.ID, r.Error))
					}
				}
				if len(failed) > 0 {
					return fmt.Errorf("%s", strings.Join(failed, "; "))
				}
				return nil
			})
		}
		return
	}
	if u.workspaceModalKey(key) {
		return
	}
	if key == "escape" {
		u.back()
		return
	}
	if u.modal == "details" || u.modal == "help" || u.modal == "logs" || u.modal == "resources-panel" {
		if key == "up" || key == "k" {
			u.detailOffset = max(0, u.detailOffset-1)
		}
		if key == "down" || key == "j" {
			u.detailOffset++
		}
		if key == "enter" || key == "q" {
			u.modal = ""
		}
		return
	}
	if u.modal == "url" || u.modal == "folder" || u.modal == "search" || u.workspaceInput() || strings.HasPrefix(u.modal, "resource:") {
		if u.modal == "url" && key == "tab" {
			u.parents = append(u.parents, u.frame())
			u.pickBrowser("add-browser", u.addBrowser)
			return
		}
		if strings.HasPrefix(key, "paste:") {
			value := strings.TrimPrefix(key, "paste:")
			value = strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " ")
			u.input += safeText(value)
			if len(u.input) > 16384 {
				u.input = string([]rune(u.input)[:minInt(len([]rune(u.input)), 8192)])
			}
			return
		}
		switch key {
		case "ctrl-u":
			u.input = ""
		case "backspace":
			_, n := utf8.DecodeLastRuneInString(u.input)
			if n > 0 {
				u.input = u.input[:len(u.input)-n]
			}
		case "enter":
			if u.modal == "search" {
				u.search = strings.TrimSpace(u.input)
				u.modal = ""
				u.reconcile()
				u.page = 0
				u.workspaceRefresh()
				return
			}
			if strings.HasPrefix(u.modal, "resource:") {
				candidate := *u.o
				if err := setResource(&candidate, strings.TrimPrefix(u.modal, "resource:"), u.input); err != nil {
					u.log(err.Error())
					return
				}
				if u.save(candidate) {
					u.back()
				}
				return
			}
			if u.modal == "folder" {
				path, e := writableFolder(u.input, u.home)
				if e != nil {
					u.log(e.Error())
					return
				}
				candidate := *u.o
				candidate.Out = path
				if u.save(candidate) {
					u.back()
				}
				return
			}
			urls := strings.Fields(u.input)
			if len(urls) == 0 {
				u.modal = ""
				return
			}
			o := *u.o
			o.Mode = u.mode
			o.Cookies = u.addBrowser
			o.chooseQuality = u.askQuality && u.mode == "video"
			o.Formats = false
			if e := validateLinks(urls, o); e != nil {
				u.log(e.Error())
				return
			}
			for _, url := range urls {
				o.URL = url
				if u.pool.add(o) {
					u.log(tr("Добавлено в очередь") + ": " + url)
				} else {
					u.log(tr("Этот ролик уже есть в очереди."))
				}
			}
			u.tab = 0
			u.selectedID = 0
			u.jobIndex = max(0, len(u.pool.jobs)-len(urls))
			u.reconcile()
			u.modal = ""
		default:
			if utf8.RuneCountInString(key) == 1 && key != "\t" {
				if len(u.input) < 16384 {
					u.input += key
				}
			}
		}
		return
	}
	switch key {
	case "left", "backspace", "q":
		u.back()
		return
	case "up":
		u.choiceIndex = (u.choiceIndex + len(u.choices) - 1) % len(u.choices)
	case "down":
		u.choiceIndex = (u.choiceIndex + 1) % len(u.choices)
	case "enter":
		kind := u.modal
		value := u.choices[u.choiceIndex].Value
		if value == "__back" {
			u.back()
			return
		}
		if kind == "settings" || kind == "limits" {
			u.parents = append(u.parents, u.frame())
		}
		u.modal = ""
		switch kind {
		case "quit":
			u.quit = value == "yes"
		case "remove":
			if value == "yes" {
				u.pool.remove(u.target)
				u.reconcile()
			}
		case "cancel":
			if value == "yes" {
				for _, j := range u.pool.jobs {
					if j.id == u.target {
						u.pool.cancelJob(j)
						break
					}
				}
			}
		case "navigation":
			switch value {
			case "library":
				u.switchSection(1)
			case "settings":
				u.settings()
			case "downloads":
				u.showChoices("section", tr("Загрузки"), []choice{{tr("Задания"), "1"}, {tr("Плейлисты"), "5"}, {tr("Фрагменты"), "6"}}, "1")
			case "cloud":
				u.showChoices("section", tr("Облако"), []choice{{"S3", "3"}, {tr("Профили"), "4"}}, "3")
			}
		case "section":
			u.switchSection(int(value[0] - '1'))
		case "settings-browser":
			candidate := *u.o
			candidate.Cookies = value
			if u.save(candidate) {
				u.back()
			}
		case "add-browser":
			u.addBrowser = value
			u.back()
		case "clip-browser":
			u.clipField(value)
		case "playlist-browser":
			u.playlistField(value)
		case "add-mode":
			u.addBrowser = u.o.Cookies
			u.askQuality = value == "manual"
			u.mode = value
			if u.askQuality {
				u.mode = "video"
			}
			u.modal = "url"
			u.title = tr("Вставьте одну или несколько ссылок через пробел")
			u.input = ""
		case "quality":
			for _, j := range u.pool.jobs {
				if j.id == u.target && j.state == "choose" {
					u.pool.choose(j.id, value)

				}
			}
		case "settings", "limits":
			switch value {
			case "limits":
				u.limitsMenu()
			case "downloads", "processing-limit", "global-rate", "job-rate", "uploads", "upload-parts", "retry-attempts", "staging-files", "staging-bytes", "min-free", "upload-rate":
				u.modal = "resource:" + value
				u.title = resourceTitle(value)
				u.input = resourceValue(*u.o, value)
			case "video-codec":
				u.showChoices("video-codec", tr("Кодек видео"), videoCodecChoices(), videoCodec(*u.o))
			case "video-format":
				u.showChoices("video-format", tr("Формат видео"), videoContainerChoices(videoCodec(*u.o)), videoContainer(*u.o))
			case "browser":
				u.pickBrowser("settings-browser", u.o.Cookies)
			case "folder":
				u.modal = "folder"
				u.title = tr("Папка сохранения")
				u.input = u.o.Out
			case "language":
				u.showChoices("language", "Language / Язык", []choice{{"English", "en"}, {"Русский", "ru"}, {tr("Системный"), "auto"}}, u.o.Lang)
			case "default-quality":
				list := []choice{{tr("Лучшее доступное"), "best"}}
				for _, q := range []string{"2160", "1440", "1080", "720", "480", "360"} {
					list = append(list, choice{q + "p", q})
				}
				u.showChoices("default-quality", tr("Качество"), list, u.o.Quality)
			case "audio":
				list := []choice{}
				for _, a := range []string{"mp3", "m4a", "opus", "flac", "wav", "best"} {
					list = append(list, choice{a, a})
				}
				u.showChoices("audio", tr("Формат аудио"), list, u.o.Audio)
			}
		case "language", "default-quality", "audio", "video-codec", "video-format":
			candidate := *u.o
			switch kind {
			case "video-codec":
				candidate.Codec = value
				if validateVideoProfile(candidate) != nil {
					candidate.Container = "mkv"
				}
			case "video-format":
				candidate.Container = value
			case "language":
				candidate.Lang = value
			case "default-quality":
				candidate.Quality = value
			case "audio":
				candidate.Audio = value
			}
			if u.save(candidate) {
				u.back()
			}
		}
	}
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Decode UTF-8 and terminal escape sequences without leaking arrow-key bytes into URL input.
type keyDecoder struct {
	pending string
	pasting bool
}

func (d *keyDecoder) feed(data []byte, timeout bool) []string {
	d.pending += string(data)
	keys := []string{}
	sequences := map[string]string{"\033[A": "up", "\033[B": "down", "\033[C": "right", "\033[D": "left", "\033[Z": "shift-tab", "\033[5~": "pageup", "\033[6~": "pagedown"}
	for len(d.pending) > 0 {
		if d.pasting {
			i := strings.Index(d.pending, "\033[201~")
			if i < 0 {
				if len(d.pending) > 65536 {
					d.pending = d.pending[len(d.pending)-65536:]
				}
				break
			}
			keys = append(keys, "paste:"+d.pending[:i])
			d.pending = d.pending[i+6:]
			d.pasting = false
			continue
		}
		if strings.HasPrefix(d.pending, "\033[200~") {
			d.pending = d.pending[6:]
			d.pasting = true
			continue
		}
		if strings.HasPrefix("\033[200~", d.pending) && !timeout {
			break
		}
		if d.pending[0] == 27 {
			found := false
			partial := false
			for seq, key := range sequences {
				if strings.HasPrefix(d.pending, seq) {
					keys = append(keys, key)
					d.pending = d.pending[len(seq):]
					found = true
					break
				}
				if strings.HasPrefix(seq, d.pending) {
					partial = true
				}
			}
			if found {
				continue
			}
			if partial && !timeout {
				break
			}
			keys = append(keys, "escape")
			d.pending = d.pending[1:]
			continue
		}
		if !utf8.FullRuneInString(d.pending) {
			break
		}
		r, n := utf8.DecodeRuneInString(d.pending)
		d.pending = d.pending[n:]
		switch r {
		case '\t':
			keys = append(keys, "tab")
		case '\n', '\r':
			keys = append(keys, "enter")
		case 21:
			keys = append(keys, "ctrl-u")
		case 127, 8:
			keys = append(keys, "backspace")
		default:
			if r >= 32 && r != utf8.RuneError {
				keys = append(keys, string(r))
			}
		}
	}
	return keys
}

type modalFrame struct {
	modal, title, input string
	choices             []choice
	index, target       int
}

func (u *panelUI) frame() modalFrame {
	return modalFrame{u.modal, u.title, u.input, append([]choice(nil), u.choices...), u.choiceIndex, u.target}
}
func (u *panelUI) back() {
	if len(u.parents) == 0 {
		u.modal = ""
		return
	}
	parent := u.parents[len(u.parents)-1]
	u.parents = u.parents[:len(u.parents)-1]
	u.modal = parent.modal
	u.title = parent.title
	u.input = parent.input
	u.choices = parent.choices
	u.choiceIndex = parent.index
	u.target = parent.target
	if parent.modal == "limits" {
		u.limitsMenu()
		u.choiceIndex = minInt(parent.index, len(u.choices)-1)
	}
	if parent.modal == "settings" {
		u.settings()
		u.choiceIndex = minInt(parent.index, len(u.choices)-1)
	}
}

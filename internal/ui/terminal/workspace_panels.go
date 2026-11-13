// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
)

func (u *panelUI) workspaceRefresh() {
	if u.pool == nil || u.pool.workspace == nil {
		return
	}
	w := u.pool.workspace
	ctx, cancel := context.WithTimeout(u.ctx, 2*time.Second)
	defer cancel()
	oldID := u.selectedItemID()
	switch u.section {
	case 5:
		if w.Clips != nil {
			cursor := ""
			var v core.ClipBatchPage
			var e error
			for i := 0; i <= u.page; i++ {
				v, e = w.Clips.List(ctx, 50, cursor)
				if e != nil || i == u.page || v.NextCursor == "" {
					break
				}
				cursor = v.NextCursor
			}
			if e == nil {
				u.clips = v.Items
				u.assetIndex = max(0, min(u.assetIndex, len(u.clips)-1))
			}
		}
	case 4:
		if w.Playlists != nil {
			v, e := w.Playlists.List(ctx, 50, u.page*50)
			if e == nil {
				u.playlists = v.Items
				u.assetIndex = max(0, min(u.assetIndex, len(v.Items)-1))
			}
		}
		if profiles, e := w.Cloud.Profiles(ctx); e == nil {
			u.profiles = profiles
		}
	case 1:
		page, e := w.Library.List(ctx, core.AssetQuery{Search: u.search, Offset: u.page * 50, Limit: 50, Kind: u.assetKind, Status: u.assetStatus, Container: u.assetContainer, HasCopies: u.assetCopies, Sort: u.assetSort, MinHeight: u.assetMinHeight, Since: u.assetSince, Until: u.assetUntil})
		if e == nil {
			u.assets = page.Items
			u.assetTotal = page.Total
			u.assetIndex = max(0, min(u.assetIndex, len(u.assets)-1))
		}
	case 2:
		items, e := w.Cloud.List(ctx, 100, u.page*100)
		if e == nil {
			u.uploads = items
			u.assetIndex = max(0, min(u.assetIndex, len(items)-1))
		}
	case 3:
		p, e := w.Cloud.Profiles(ctx)
		if e == nil {
			u.profiles = p
			u.assetIndex = max(0, min(u.assetIndex, len(p)-1))
		}
	}
	if oldID != "" {
		for i := 0; i < u.sectionLength(); i++ {
			if u.itemID(i) == oldID {
				u.assetIndex = i
				break
			}
		}
	}

}
func (u *panelUI) resourceLine() string {
	if u.pool == nil || u.pool.workspace == nil {
		return ""
	}
	w := u.pool.workspace
	s := w.Monitor.Snapshot()
	cpu := "—"
	if s.CPU != nil {
		cpu = fmt.Sprintf("%.0f%%", *s.CPU)
	}
	ram := "—"
	if s.RAMUsed != nil && s.RAMTotal != nil && *s.RAMTotal > 0 {
		ram = fmt.Sprintf("%.1f/%.1f GiB %.0f%%", float64(*s.RAMUsed)/(1<<30), float64(*s.RAMTotal)/(1<<30), float64(*s.RAMUsed)/float64(*s.RAMTotal)*100)
	}
	return "CPU " + cpu + " | RAM " + ram
}
func (u *panelUI) switchSection(section int) {
	u.section = section
	u.page = 0
	u.assetIndex = 0
	u.search = ""
	u.workspaceRefresh()
}
func (u *panelUI) navigationHeader() string {
	labels := []string{"Задания", "Медиатека", "S3", "Профили", "Плейлисты", "Фрагменты"}
	return "CamelClip " + version + "  ·  " + tr(labels[u.section]) + "  ·  m: " + tr("Разделы") + "  ·  c: CPU/RAM"
}
func (u *panelUI) workspaceKey(key string) bool {
	if key == "h" && (u.section == 1 || u.section == 5) {
		u.companionPrompt()
		return true
	}
	if u.pool.workspace == nil {
		return false
	}
	if key == "1" || key == "2" || key == "3" || key == "4" || key == "5" || key == "6" {
		u.switchSection(int(key[0] - '1'))
		return true
	}
	if key == "m" {
		u.showChoices("navigation", tr("Разделы"), []choice{{tr("Загрузки"), "downloads"}, {tr("Медиатека"), "library"}, {tr("Облако"), "cloud"}, {tr("Настройки"), "settings"}}, "")
		return true
	}
	if key == "c" {
		u.modal = "resources-panel"
		u.title = tr("Ресурсы")
		u.detailOffset = 0
		u.input = u.resourcesText()
		return true
	}
	if u.section == 0 {
		return false
	}
	switch key {
	case "q":
		if u.pool.busy() {
			u.showChoices("quit", tr("Остановить работу и выйти? Очередь восстановится."), []choice{{tr("Нет"), "no"}, {tr("Да"), "yes"}}, "no")
		} else {
			active, _, _ := u.pool.workspace.Cloud.Stats()
			if active > 0 {
				u.showChoices("quit", tr("Остановить работу и выйти? Очередь восстановится."), []choice{{tr("Нет"), "no"}, {tr("Да"), "yes"}}, "no")
			} else {
				u.quit = true
			}
		}
		return true
	case "up", "k":
		u.assetIndex = max(0, u.assetIndex-1)
		return true
	case "down", "j":
		u.assetIndex = min(u.sectionLength()-1, u.assetIndex+1)
		u.assetIndex = max(0, u.assetIndex)
		return true
	case "pageup":
		u.page = max(0, u.page-1)
		u.assetIndex = 0
		u.workspaceRefresh()
		return true
	case "pagedown":
		u.page++
		u.assetIndex = 0
		u.workspaceRefresh()
		if u.sectionLength() == 0 && u.page > 0 {
			u.page--
			u.workspaceRefresh()
		}
		return true
	case "/":
		u.modal = "search"
		u.title = tr("Поиск по названию и URL")
		u.input = u.search
		return true
	case "escape":
		u.search = ""
		u.page = 0
		u.workspaceRefresh()
		return true
	case "s":
		u.settings()
		return true
	case "?":
		u.modal = "help"
		u.title = tr("Справка")
		u.input = tr("1: задания · 2: медиатека · 3: S3-отправки · 4: профили\nc: CPU/RAM · PgUp/PgDn: страницы · Space/A: выделение\nМедиатека: / поиск · t фильтры · b сортировка · g сканирование · i импорт\nEnter: сведения · o: показать · f: открыть файл · v: обновить\nu: отправить · D: удалить файл\nS3: r повтор с начала · x отменить · z очистить multipart\nПрофили: n создать · e изменить · k заменить ключи · u разблокировать\nl заблокировать · m сменить пароль · R сбросить профили\nt проверить чтение · w проверить запись · a автоотправка · p включить/отключить")
		u.detailOffset = 0
		return true
	case "space", " ":
		u.toggleMark()
		return true
	case "A":
		u.markAll()
		return true
	}
	if u.section == 1 {
		return u.libraryKey(key)
	}
	if u.section == 2 {
		return u.uploadKey(key)
	}
	if u.section == 5 {
		return u.clipKey(key)
	}
	if u.section == 4 {
		return u.playlistKey(key)
	}
	return u.profileKey(key)
}
func (u *panelUI) sectionLength() int {
	switch u.section {
	case 5:
		return len(u.clips)
	case 1:
		return len(u.assets)
	case 2:
		return len(u.uploads)
	case 4:
		return len(u.playlists)
	case 3:
		return len(u.profiles)
	}
	return 0
}
func (u *panelUI) selectedAsset() *core.Asset {
	if u.assetIndex < 0 || u.assetIndex >= len(u.assets) {
		return nil
	}
	return &u.assets[u.assetIndex]
}
func (u *panelUI) markID() string {
	switch u.section {
	case 1:
		if a := u.selectedAsset(); a != nil {
			return "asset:" + a.ID
		}
	case 0:
		if j := u.selectedJob(); j != nil {
			return fmt.Sprintf("job:%d", j.id)
		}
	}
	return ""
}
func (u *panelUI) toggleMark() {
	if u.marked == nil {
		u.marked = map[string]bool{}
	}
	id := u.markID()
	if id != "" {
		u.marked[id] = !u.marked[id]
	}
}
func (u *panelUI) markAll() {
	if u.marked == nil {
		u.marked = map[string]bool{}
	}
	if u.section == 0 {
		for _, j := range u.visibleJobs() {
			u.marked[fmt.Sprintf("job:%d", j.id)] = true
		}
	} else if u.section == 1 {
		for _, a := range u.assets {
			u.marked["asset:"+a.ID] = true
		}
	}
}
func (u *panelUI) libraryIDs() []string {
	ids := []string{}
	for id, on := range u.marked {
		if on && strings.HasPrefix(id, "asset:") {
			ids = append(ids, strings.TrimPrefix(id, "asset:"))
		}
	}
	if len(ids) == 0 {
		if a := u.selectedAsset(); a != nil {
			ids = append(ids, a.ID)
		}
	}
	return ids
}
func (u *panelUI) libraryKey(key string) bool {
	w := u.pool.workspace
	a := u.selectedAsset()
	switch key {
	case "g":
		if e := w.Library.StartScan(u.ctx, u.o.Out, false); e != nil {
			u.log(e.Error())
		} else {
			u.log(tr("Сканирование запущено"))
		}
	case "i":
		u.modal = "library-import"
		u.title = tr("Путь к локальному медиафайлу")
		u.input = ""
	case "t":
		u.showChoices("library-filter", tr("Фильтр медиатеки"), []choice{{tr("Все"), "all"}, {tr("Видео"), "video"}, {tr("Аудио"), "audio"}, {tr("Нет локально"), "missing"}, {tr("Не отправлено в S3"), "not-uploaded"}, {tr("Есть S3-копия"), "uploaded"}, {tr("Контейнер"), "container"}, {tr("Минимальная высота"), "height"}, {tr("Добавлено с даты"), "since"}, {tr("Добавлено до даты"), "until"}}, "all")
	case "b":
		u.showChoices("library-sort", tr("Сортировка"), []choice{{tr("Дата"), "date"}, {tr("Название"), "name"}, {tr("Размер"), "size"}, {tr("Продолжительность"), "duration"}}, u.assetSort)
	case "enter":
		if a != nil {
			u.modal = "details"
			u.title = a.Title
			u.input = fmt.Sprintf("ID: %s\n%s\n%s · %s · %.1fs · %dx%d\n%s / %s\nURL: %s\nS3: %d\nSHA-256: %s", a.ID, a.Path, assetStatusLabel(a.Status), fileSize(a.Size), a.Duration, a.Width, a.Height, a.VideoCodec, a.AudioCodec, a.SourceURL, a.RemoteCopies, a.SHA256)
			u.detailOffset = 0
		}
	case "o":
		if a != nil {
			u.open(a.Path, true)
		}
	case "f":
		if a != nil {
			u.open(a.Path, false)
		}
	case "v":
		if a != nil {
			id := a.ID
			u.background(func() error { _, e := w.Library.Refresh(u.ctx, id); return e })
		}
	case "u":
		u.beginUpload()
	case "D":
		if a != nil {
			u.actionIDs = u.libraryIDs()
			u.action = "delete-file"
			size := int64(0)
			for _, id := range u.actionIDs {
				if item, e := w.Library.Get(u.ctx, id); e == nil {
					size += item.Size
				}
			}
			title := fmt.Sprintf(tr("Удалить с диска: %d файлов (%s)?"), len(u.actionIDs), fileSize(size))
			u.showChoices("workspace-confirm", title, []choice{{tr("Нет"), "no"}, {tr("Да"), "yes"}}, "no")
		}
	default:
		return false
	}
	return true
}
func (u *panelUI) beginUpload() {
	profiles, e := u.pool.workspace.Cloud.Profiles(u.ctx)
	if e != nil {
		u.log(e.Error())
		return
	}
	u.actionIDs = u.libraryIDs()
	if len(u.actionIDs) == 0 {
		return
	}
	u.profileTargets = map[string]bool{}
	u.choices = nil
	for _, p := range profiles {
		if p.Enabled {
			u.choices = append(u.choices, choice{p.Name + " → " + p.Bucket + "/" + p.Prefix, p.ID})
		}
	}
	if len(u.choices) == 0 {
		u.log(tr("Сначала создайте S3-профиль: клавиша 4"))
		return
	}
	u.modal = "upload-targets"
	u.title = tr("Space: выбрать профили · Enter: назначение")
	u.choiceIndex = 0
}
func (u *panelUI) uploadKey(key string) bool {
	if len(u.uploads) == 0 {
		return true
	}
	j := u.uploads[u.assetIndex]
	switch key {
	case "enter":
		u.modal = "details"
		u.title = j.Profile.Name
		u.input = fmt.Sprintf("ID: %s\n%s · %s\n%s/%s\n%s\n%s\n%s\nMultipart: %s", j.ID, uploadStateLabel(string(j.State)), j.Profile.Name, j.Profile.Bucket, j.Key, remoteVerificationLabel(j.Verification), j.Error, tr("Повтор начинает отправку с начала"), j.MultipartID)
		u.detailOffset = 0
	case "r":
		u.action = "retry-upload"
		if j.State == "blocked_profile_changed" {
			u.action = "accept-upload"
		}
		u.actionIDs = []string{j.ID}
		u.confirmAction(tr("Повторить отправку с начала?"))
	case "x":
		u.action = "cancel-upload"
		u.actionIDs = []string{j.ID}
		u.confirmAction(tr("Отменить отправку?"))
	case "z":
		u.action = "cleanup-upload"
		u.actionIDs = []string{j.ID}
		u.confirmAction(tr("Удалить незавершённые multipart-части?"))
	default:
		return false
	}
	return true
}
func (u *panelUI) confirmAction(title string) {
	u.showChoices("workspace-confirm", title, []choice{{tr("Нет"), "no"}, {tr("Да"), "yes"}}, "no")
}
func (u *panelUI) resourcesText() string {
	if u.pool.workspace == nil {
		return "—"
	}
	w := u.pool.workspace
	s := w.Monitor.Snapshot()
	lines := []string{u.resourceLine(), fmt.Sprintf("%s: %d", tr("Логических CPU"), len(s.Cores))}
	for i, p := range s.Cores {
		v := "—"
		if p != nil {
			v = fmt.Sprintf("%3.0f%% %s", *p, strings.Repeat("█", int(*p/10))+strings.Repeat("░", 10-int(*p/10)))
		}
		lines = append(lines, fmt.Sprintf("CPU %d  %s", i, v))
	}
	stats := u.stats()
	active, _, _ := w.Cloud.Stats()
	mode := "manual"
	if u.o.MaxProcessing == 0 {
		mode = "auto"
	}
	lines = append(lines, "Processing: "+mode+" · max(CPU / 4, 2)")
	lines = append(lines, fmt.Sprintf("Download %d/%d | Processing %d/%d | S3 %d/%d", stats.Downloads, stats.Limits.Effective().Downloads, stats.Processing, stats.Limits.Effective().Processing, active, w.Cloud.Options().Objects))
	if s.ProcessRSS != nil {
		lines = append(lines, "CamelClip RSS: "+fileSize(int64(*s.ProcessRSS)))
	}
	lines = append(lines, s.Error)
	return strings.Join(lines, "\n")
}
func (u *panelUI) background(fn func() error) {
	if u.actions == nil {
		u.actions = make(chan string, 32)
	}
	out := u.actions
	go func() {
		message := tr("Готово")
		if e := fn(); e != nil {
			message = e.Error()
		}
		select {
		case out <- message:
		case <-u.ctx.Done():
		}
	}()
}
func (u *panelUI) renderWorkspace() {
	w, h := min(u.width-1, 240), min(u.height-1, 80)
	if w < 1 || h < 1 {
		return
	}
	c := newCanvas(w, h)
	c.text(1, 0, w-2, u.navigationHeader(), cyan)
	if w < 39 || h < 9 {
		c.text(1, 2, w-2, tr("Увеличьте терминал: минимум 40 × 10."), normal)
		c.text(1, 4, w-2, "q: "+tr("Выход"), dim)
		u.draw(c)
		return
	}
	c.text(1, 1, w-2, u.resourceLine(), normal)
	active, uploadSpeed, _ := u.pool.workspace.Cloud.Stats()
	rate := "—"
	if uploadSpeed != nil {
		rate = rateText(*uploadSpeed)
	}
	locked := "UNLOCKED"
	if u.pool.workspace.Cloud.Locked() {
		locked = "LOCKED"
	}
	c.text(1, 2, w-2, fmt.Sprintf("↑ S3 %s · %d/%d · %s", rate, active, u.pool.workspace.Cloud.Options().Objects, locked), dim)
	title := []string{"", tr("Медиатека"), tr("S3-отправки"), tr("S3-профили"), tr("Плейлисты"), tr("Фрагменты")}[u.section]
	c.box(0, 3, w, h-6, title, true)
	rows := max(1, h-8)
	start := max(0, u.assetIndex-rows+1)
	for i := start; i < u.sectionLength() && i < start+rows; i++ {
		style := normal
		if i == u.assetIndex {
			style = selected
			c.fill(1, 4+i-start, w-2, style)
		}
		text := ""
		switch u.section {
		case 5:
			b := u.clips[i]
			done := 0
			for _, task := range b.Tasks {
				if task.State == core.ClipDone {
					done++
				}
			}
			text = fmt.Sprintf("%s · %s · %d/%d", b.Plan.Source.Title, b.Status(), done, len(b.Tasks))
		case 4:
			p := u.playlists[i]
			text = fmt.Sprintf("%s · %s · %d min · S3:%d · %s", p.Name, p.Status, p.IntervalMinutes, len(p.S3Profiles), p.LastError)
		case 1:
			a := u.assets[i]
			mark := "  "
			if u.marked["asset:"+a.ID] {
				mark = "✓ "
			}
			text = fmt.Sprintf("%s%s · %s · %.0fs · %dx%d · %s · S3:%d", mark, filepath.Base(a.Path), fileSize(a.Size), a.Duration, a.Width, a.Height, assetStatusLabel(a.Status), a.RemoteCopies)
		case 2:
			j := u.uploads[i]
			text = fmt.Sprintf("%s · %s · %s · %s · %s · ETA %s", j.Profile.Name, uploadStateLabel(string(j.State)), percent(j.Metrics), speed(j.Metrics), j.Key, eta(j.Metrics))
		case 3:
			p := u.profiles[i]
			enabled := tr("Отключено")
			if p.Enabled {
				enabled = tr("Включено")
			}
			automatic := tr("Вручную")
			if p.AutoUpload {
				automatic = tr("Автоотправка")
			}
			text = fmt.Sprintf("%s · %s · %s/%s · %s · %s", p.Name, p.Endpoint, p.Bucket, p.Prefix, enabled, automatic)
		}
		c.text(2, 4+i-start, w-4, text, style)
	}
	if u.sectionLength() == 0 {
		c.text(2, 5, w-4, tr("Нет записей"), dim)
	}
	c.text(1, h-2, w-2, tr("1–6:экраны c:CPU/RAM Enter:сведения ?:помощь q:выход"), cyan)
	footer := tr("g:сканировать i:импорт u:S3 D:удалить /:поиск h:субтитры")
	if u.section == 2 {
		footer = tr("r:повтор x:отмена z:очистить multipart")
	}
	if u.section == 3 {
		footer = tr("n:новый e:изменить u:разблокировать t:проверить a:авто")
	}
	if u.section == 4 {
		footer = tr("n:новый e:изменить p:пауза s:проверить b:существующие")
	}
	if u.section == 5 {
		footer = tr("n:нарезать p:пауза/продолжить x:отмена r:повтор Enter:сведения h:субтитры")
	}
	c.text(1, h-1, w-2, footer, dim)
	if u.modal != "" {
		u.drawModal(c)
	}
	u.draw(c)
}

func (u *panelUI) itemID(index int) string {
	if u.section == 5 && index >= 0 && index < len(u.clips) {
		return u.clips[index].ID
	}
	if index < 0 {
		return ""
	}
	switch u.section {
	case 4:
		if index < len(u.playlists) {
			return u.playlists[index].ID
		}
	case 1:
		if index < len(u.assets) {
			return u.assets[index].ID
		}
	case 2:
		if index < len(u.uploads) {
			return u.uploads[index].ID
		}
	case 3:
		if index < len(u.profiles) {
			return u.profiles[index].ID
		}
	}
	return ""
}
func (u *panelUI) selectedItemID() string { return u.itemID(u.assetIndex) }
func jobHistoryText(events []application.JobEvent) string {
	var b strings.Builder
	for _, e := range events {
		fmt.Fprintf(&b, "%s · %s · %s\n", stamp(e.At), e.State, e.Message)
	}
	return b.String()
}

func (u *panelUI) hasActiveWork() bool {
	if u.pool.busy() {
		return true
	}
	if u.pool.workspace != nil {
		active, _, _ := u.pool.workspace.Cloud.Stats()
		return active > 0
	}
	return false
}

func uploadStateLabel(state string) string {
	labels := map[string]string{"queued": "В очереди", "waiting_unlock": "Нужен мастер-пароль", "waiting_slot": "Ожидание слота", "uploading": "Отправка", "remote_check": "Проверка в S3", "done": "Готово", "failed": "Ошибка", "canceled": "Отменено", "canceling": "Отмена…", "paused": "Приостановлено", "blocked_profile_changed": "Назначение изменилось", "source_missing": "Файл отсутствует или изменён", "conflict": "Объект уже существует"}
	if label, ok := labels[state]; ok {
		return tr(label)
	}
	return state
}
func remoteVerificationLabel(level string) string {
	return tr(map[string]string{"uploaded_unverified": "Удалённый файл ещё не проверен", "size_verified": "Размер подтверждён", "checksum_verified": "Контрольная сумма подтверждена", "verification_failed": "Проверка не пройдена"}[level])
}
func assetStatusLabel(status string) string {
	if status == "available" {
		return tr("Есть локально")
	}
	if status == "missing" {
		return tr("Нет локально")
	}
	return status
}

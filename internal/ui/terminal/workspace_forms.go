// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import (
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"strconv"
	"strings"
	"time"
)

func (u *panelUI) workspaceInput() bool {
	return u.modal == "vault" || u.modal == "profile-field" || u.modal == "library-import" || u.modal == "library-filter-value"
}
func (u *panelUI) secretInput() bool {
	return u.modal == "vault" || u.modal == "profile-field" && u.formStep >= 7
}
func (u *panelUI) profileKey(key string) bool {
	w := u.pool.workspace
	var p *core.S3Profile
	if u.assetIndex >= 0 && u.assetIndex < len(u.profiles) {
		p = &u.profiles[u.assetIndex]
	}
	switch key {
	case "u":
		u.secretMode = "unlock"
		if !w.Cloud.HasVault(u.ctx) {
			u.secretMode = "initialize"
		}
		u.modal = "vault"
		u.title = tr("Мастер-пароль S3 (минимум 10 символов)")
		u.input = ""
	case "l":
		u.background(func() error { w.Cloud.Lock(); return nil })
	case "m":
		u.secretMode = "rotate-old"
		u.modal = "vault"
		u.title = tr("Текущий мастер-пароль")
		u.input = ""
	case "R":
		u.action = "reset-profiles"
		u.confirmAction(tr("Сбросить S3-профили и ключи? История файлов сохранится."))
	case "n":
		if w.Cloud.Locked() {
			u.newProfileAfterUnlock = true
			u.profileKey("u")
			return true
		}
		u.newProfileAfterUnlock = false
		u.credentialForm = core.Credentials{}
		u.profileForm = core.S3Profile{ID: core.NewID(), Enabled: true, Style: "auto", Region: "auto"}
		u.formStep = 0
		u.formCredentials = true
		u.nextProfileField()
	case "e", "k":
		if p != nil {
			u.profileForm = *p
			u.formStep = 0
			u.formCredentials = false
			if key == "k" {
				u.formStep = 7
				u.formCredentials = true
			}
			u.nextProfileField()
		}
	case "d":
		if p != nil {
			u.action = "delete-profile"
			u.actionIDs = []string{p.ID}
			u.confirmAction(tr("Удалить профиль? Файлы и история сохранятся."))
		}
	case "a", "p":
		if p != nil {
			u.profileForm = *p
			u.action = "toggle-auto"
			if key == "p" {
				u.action = "toggle-profile"
			}
			u.confirmAction(tr("Изменить правило профиля? Автоотправка применяется к новым публикациям."))
		}
	case "t":
		if p != nil {
			id := p.ID
			u.background(func() error { return w.Cloud.Test(u.ctx, id, false) })
		}
	case "w":
		if p != nil {
			u.action = "write-test"
			u.actionIDs = []string{p.ID}
			u.confirmAction(tr("Создать и удалить тестовый объект? Возможны API-расходы."))
		}
	case "enter":
		if p != nil {
			u.modal = "details"
			u.title = p.Name
			u.input = fmt.Sprintf("ID: %s\n%s\n%s\nBucket: %s\nPrefix: %s\nRevision: %d\n%s", p.ID, p.Name, p.Endpoint, p.Bucket, p.Prefix, p.Revision, tr("Профиль сохранён только на этом компьютере"))
			u.detailOffset = 0
		}
	default:
		return false
	}
	return true
}
func (u *panelUI) nextProfileField() {
	titles := []string{"Имя профиля", "Endpoint HTTPS (пусто: AWS)", "Регион", "Bucket", "Prefix (может быть пустым)", "Стиль URL: auto / path_style / virtual_host", "Разрешить HTTP только loopback? yes / no", "Access key ID", "Secret access key", "Session token (может быть пустым)"}
	values := []string{u.profileForm.Name, u.profileForm.Endpoint, u.profileForm.Region, u.profileForm.Bucket, u.profileForm.Prefix, u.profileForm.Style, "no", "", "", ""}
	if u.profileForm.AllowLoopbackHTTP {
		values[6] = "yes"
	}
	if u.formStep == 7 && !u.formCredentials || u.formStep >= len(titles) {
		u.action = "save-profile"
		u.confirmAction(tr("Сохранить профиль только на этом компьютере?"))
		return
	}
	u.modal = "profile-field"
	u.title = tr(titles[u.formStep])
	u.input = values[u.formStep]
}
func (u *panelUI) workspaceModalKey(key string) bool {
	if u.modal == "upload-targets" {
		switch key {
		case "escape":
			u.modal = ""
		case "up", "k":
			u.choiceIndex = max(0, u.choiceIndex-1)
		case "down", "j":
			u.choiceIndex = min(len(u.choices)-1, u.choiceIndex+1)
		case "space", " ":
			id := u.choices[u.choiceIndex].Value
			u.profileTargets[id] = !u.profileTargets[id]
		case "enter":
			any := false
			for _, on := range u.profileTargets {
				any = any || on
			}
			if !any {
				u.profileTargets[u.choices[u.choiceIndex].Value] = true
			}
			lines := []string{}
			for _, assetID := range u.actionIDs {
				a, e := u.pool.workspace.Library.Get(u.ctx, assetID)
				if e != nil {
					u.log(e.Error())
					continue
				}
				for _, p := range u.profilesForTargets() {
					lines = append(lines, fmt.Sprintf("%s → %s / %s", a.Title, p.Bucket, core.DestinationKey(p, a)))
				}
			}
			u.modal = "upload-review"
			u.title = tr("Проверьте назначения · Enter: продолжить")
			u.input = strings.Join(lines, "\n")
			u.detailOffset = 0
		}
		return true
	}
	if u.modal == "upload-review" {
		switch key {
		case "escape":
			u.modal = ""
		case "down", "j":
			u.detailOffset++
		case "up", "k":
			u.detailOffset = max(0, u.detailOffset-1)
		case "enter":
			u.action = "enqueue"
			u.confirmAction(fmt.Sprintf(tr("Отправить файлов: %d?"), len(u.actionIDs)))
		}
		return true
	}
	if u.modal == "workspace-confirm" && key == "enter" {
		value := u.choices[u.choiceIndex].Value
		u.modal = ""
		if value == "yes" {
			u.executeWorkspaceAction()
		}
		return true
	}
	if (u.modal == "library-filter" || u.modal == "library-sort") && key == "enter" {
		kind := u.modal
		v := u.choices[u.choiceIndex].Value
		u.modal = ""
		if v == "__back" {
			return true
		}
		if kind == "library-filter" && (v == "container" || v == "height" || v == "since" || v == "until") {
			u.action = v
			u.modal = "library-filter-value"
			u.title = map[string]string{"container": tr("Контейнер (mp4/mkv/…; пусто — все)"), "height": tr("Минимальная высота (0 — любая)"), "since": tr("Добавлено с даты YYYY-MM-DD (пусто — все)"), "until": tr("Добавлено до даты YYYY-MM-DD (пусто — все)")}[v]
			u.input = ""
			return true
		}
		if kind == "library-sort" {
			u.assetSort = v
		} else {
			u.assetKind = ""
			u.assetStatus = ""
			u.assetCopies = nil
			switch v {
			case "all":
				u.assetContainer = ""
				u.assetMinHeight = 0
				u.assetSince = time.Time{}
				u.assetUntil = time.Time{}
			case "video", "audio":
				u.assetKind = v
			case "missing":
				u.assetStatus = v
			case "not-uploaded":
				b := false
				u.assetCopies = &b
			case "uploaded":
				b := true
				u.assetCopies = &b
			}
		}
		u.page = 0
		u.workspaceRefresh()
		return true
	}
	if !u.workspaceInput() {
		return false
	}
	if key == "escape" {
		u.newProfileAfterUnlock = false
		u.input = ""
		u.secretFirst = ""
		u.rotationPassword = ""
		u.credentialForm = core.Credentials{}
		u.modal = ""
		return true
	}
	if key != "enter" {
		return false
	}
	value := u.input
	u.input = ""
	switch u.modal {
	case "library-filter-value":
		value = strings.TrimSpace(value)
		switch u.action {
		case "container":
			u.assetContainer = strings.TrimPrefix(strings.ToLower(value), ".")
		case "height":
			n, e := strconv.Atoi(value)
			if e != nil || n < 0 || n > 16384 {
				u.log("0..16384")
				return true
			}
			u.assetMinHeight = n
		case "since", "until":
			date := time.Time{}
			var e error
			if value != "" {
				date, e = time.Parse("2006-01-02", value)
				if e != nil {
					u.log("YYYY-MM-DD")
					return true
				}
				if u.action == "until" {
					date = date.Add(24*time.Hour - time.Nanosecond)
				}
			}
			if u.action == "since" {
				u.assetSince = date
			} else {
				u.assetUntil = date
			}
		}
		u.modal = ""
		u.page = 0
		u.workspaceRefresh()
	case "library-import":
		u.modal = ""
		w := u.pool.workspace
		u.background(func() error { _, e := w.Library.Import(u.ctx, value); return e })
	case "vault":
		mode := u.secretMode
		cloud := u.pool.workspace.Cloud
		if mode == "initialize" || mode == "rotate-new" {
			u.secretFirst = value
			u.secretMode = mode + "-confirm"
			u.title = tr("Повторите новый мастер-пароль")
			return true
		}
		if strings.HasSuffix(mode, "-confirm") {
			if value != u.secretFirst {
				u.secretFirst = ""
				u.secretMode = strings.TrimSuffix(mode, "-confirm")
				u.title = tr("Пароли не совпали. Введите новый мастер-пароль")
				return true
			}
			password := []byte(value)
			old := []byte(u.rotationPassword)
			u.rotationPassword = ""
			u.secretFirst = ""
			u.modal = ""
			u.background(func() error {
				defer clear(password)
				defer clear(old)
				if mode == "initialize-confirm" {
					return cloud.Initialize(u.ctx, password)
				}
				return cloud.Rotate(u.ctx, old, password)
			})
			return true
		}
		if mode == "rotate-old" {
			u.rotationPassword = value
			u.secretMode = "rotate-new"
			u.title = tr("Новый мастер-пароль")
			return true
		}
		password := []byte(value)
		u.modal = ""
		u.background(func() error { defer clear(password); return cloud.Unlock(u.ctx, password) })
	case "profile-field":
		switch u.formStep {
		case 0:
			u.profileForm.Name = strings.TrimSpace(value)
		case 1:
			u.profileForm.Endpoint = strings.TrimSpace(value)
		case 2:
			u.profileForm.Region = strings.TrimSpace(value)
		case 3:
			u.profileForm.Bucket = strings.TrimSpace(value)
		case 4:
			u.profileForm.Prefix = strings.Trim(value, " /")
		case 5:
			u.profileForm.Style = strings.TrimSpace(value)
		case 6:
			u.profileForm.AllowLoopbackHTTP = value == "yes"
		case 7:
			u.credentialForm.AccessKey = value
		case 8:
			u.credentialForm.SecretKey = value
		case 9:
			u.credentialForm.SessionToken = value
		}
		u.formStep++
		u.nextProfileField()
	}
	return true
}
func (u *panelUI) profilesForTargets() []core.S3Profile {
	all, e := u.pool.workspace.Cloud.Profiles(u.ctx)
	if e != nil {
		return nil
	}
	result := []core.S3Profile{}
	for _, p := range all {
		if u.profileTargets[p.ID] {
			result = append(result, p)
		}
	}
	return result
}
func (u *panelUI) executeWorkspaceAction() {
	w := u.pool.workspace
	action := u.action
	ids := append([]string(nil), u.actionIDs...)
	profile := u.profileForm
	credentials := u.credentialForm
	hasCredentials := u.formCredentials
	u.credentialForm = core.Credentials{}
	targets := []string{}
	for id, on := range u.profileTargets {
		if on {
			targets = append(targets, id)
		}
	}
	u.background(func() error {
		failures := []string{}
		for _, id := range ids {
			var e error
			switch action {
			case "forget":
				e = w.Library.Forget(u.ctx, id)
			case "delete-file":
				e = w.Library.DeleteFile(u.ctx, id, true)
			case "enqueue":
				_, e = w.Cloud.Enqueue(u.ctx, id, targets)
			case "accept-upload":
				e = w.Cloud.Retry(u.ctx, id, true)
			case "retry-upload":
				e = w.Cloud.Retry(u.ctx, id, false)
			case "cancel-upload":
				e = w.Cloud.Cancel(u.ctx, id)
			case "cleanup-upload":
				e = w.Cloud.Cleanup(u.ctx, id)
			case "delete-profile":
				e = w.Cloud.DeleteProfile(u.ctx, id)
			case "write-test":
				e = w.Cloud.Test(u.ctx, id, true)
			}
			if e != nil {
				failures = append(failures, id+": "+e.Error())
			}
		}
		switch action {
		case "save-profile":
			var c *core.Credentials
			if hasCredentials {
				c = &credentials
			}
			if e := w.Cloud.SaveProfile(u.ctx, profile, c); e != nil {
				return e
			}
		case "toggle-auto":
			profile.AutoUpload = !profile.AutoUpload
			return w.Cloud.SaveProfile(u.ctx, profile, nil)
		case "toggle-profile":
			profile.Enabled = !profile.Enabled
			return w.Cloud.SaveProfile(u.ctx, profile, nil)
		case "reset-profiles":
			return w.Cloud.Reset(u.ctx, true)
		}
		if len(failures) > 0 {
			return fmt.Errorf("%s", strings.Join(failures, "; "))
		}
		return nil
	})
}
func (u *panelUI) jobLabel(j *job) string {
	mark := ""
	if u.marked[fmt.Sprintf("job:%d", j.id)] {
		mark = "✓ "
	}
	priority := "N"
	if j.record.Control.Priority > 0 {
		priority = "H"
	}
	if j.record.Control.Priority < 0 {
		priority = "L"
	}
	paused := ""
	if j.record.Control.Paused {
		paused = " ⏸"
	}
	return fmt.Sprintf("%s#%d %s%s %s", mark, j.id, priority, paused, j.title)
}
func (u *panelUI) queueControlKey(key string) bool {
	switch key {
	case "space", " ":
		u.toggleMark()
		return true
	case "A":
		u.markAll()
		return true
	}
	action := ""
	switch key {
	case "p":
		action = "pause"
	case "[":
		action = "low"
	case "]":
		action = "high"
	case "=":
		action = "normal"
	case "<", ">", "{", "}":
		if j := u.selectedJob(); j != nil {
			move := map[string]string{"<": "up", ">": "down", "{": "first", "}": "last"}[key]
			if e := u.pool.service.Reorder(j.id, move); e != nil {
				u.log(e.Error())
			}
		}
		return true
	case "r", "x", "d":
		for id, on := range u.marked {
			if on && strings.HasPrefix(id, "job:") {
				action = map[string]string{"r": "retry", "x": "cancel", "d": "remove"}[key]
				break
			}
		}
	}
	if action == "" {
		return false
	}
	ids := []int{}
	for id, on := range u.marked {
		if on && strings.HasPrefix(id, "job:") {
			n, e := strconv.Atoi(strings.TrimPrefix(id, "job:"))
			if e == nil {
				ids = append(ids, n)
			}
		}
	}
	if len(ids) == 0 {
		if j := u.selectedJob(); j != nil {
			ids = []int{j.id}
			if key == "p" && j.record.Control.Paused {
				action = "resume"
			}
		}
	}
	if len(ids) == 0 {
		return true
	}
	if action == "pause" {
		allPaused := true
		selected := map[int]bool{}
		for _, id := range ids {
			selected[id] = true
		}
		for _, j := range u.pool.service.Jobs() {
			if selected[j.ID] && !j.Control.Paused {
				allPaused = false
			}
		}
		if allPaused {
			action = "resume"
		}
	}
	u.bulkIDs = ids
	u.bulkAction = action
	u.showChoices("bulk-confirm", fmt.Sprintf(tr("Выполнить %s для %d заданий?"), action, len(ids)), []choice{{tr("Нет"), "no"}, {tr("Да"), "yes"}}, "no")
	return true
}

// Continue on the UI goroutine only after the asynchronous vault operation succeeds.
func (u *panelUI) resumeProfileCreation() {
	if u.newProfileAfterUnlock && u.modal == "" && !u.pool.workspace.Cloud.Locked() {
		u.profileKey("n")
	}
}

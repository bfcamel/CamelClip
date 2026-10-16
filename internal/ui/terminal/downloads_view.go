// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import (
	"fmt"
	"github.com/bfcamel/video-downloader/internal/application"
	"github.com/bfcamel/video-downloader/internal/core"
	"strings"
	"time"
)

var tabNames = []string{"Все", "Скачиваются", "Ожидают", "Обрабатываются", "Готово", "Ошибки"}

func tabMatches(tab int, state string) bool {
	switch tab {
	case 1:
		return state == "downloading"
	case 2:
		return contains([]string{"queued", "inspecting", "choose", "ready", "waiting_download", "waiting_processing", "waiting_verification", "waiting_disk_capacity"}, state)
	case 3:
		return state == "processing" || state == "verifying"
	case 4:
		return state == "done"
	case 5:
		return state == "failed" || state == "canceled"
	}
	return true
}
func (u *panelUI) visibleJobs() []*job {
	if u.pool == nil {
		return nil
	}
	result := make([]*job, 0, len(u.pool.jobs))
	query := strings.ToLower(u.search)
	for _, j := range u.pool.jobs {
		if tabMatches(u.tab, j.state) && (query == "" || strings.Contains(strings.ToLower(fmt.Sprint(j.id)+" "+j.title+" "+j.options.URL), query)) {
			result = append(result, j)
		}
	}
	return result
}
func (u *panelUI) reconcile() {
	items := u.visibleJobs()
	if len(items) == 0 {
		u.jobIndex = 0
		return
	}
	found := false
	for i, j := range items {
		if j.id == u.selectedID {
			u.jobIndex = i
			found = true
			break
		}
	}
	if !found {
		u.jobIndex = max(0, min(u.jobIndex, len(items)-1))
	}
	u.selectedID = items[u.jobIndex].id
	if u.modal == "details" {
		for _, j := range u.pool.jobs {
			if j.id == u.target {
				u.input = detailsText(j)
				break
			}
		}
	}
}
func (u *panelUI) stats() application.Statistics {
	if u.pool != nil && u.pool.service != nil {
		return u.pool.service.Statistics()
	}
	return application.Statistics{Limits: settingsFor(*u.o).Limits()}
}
func percent(m core.Metrics) string {
	if m.Percent == nil {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", *m.Percent)
}
func speed(m core.Metrics) string {
	if m.SpeedBPS == nil {
		return "—"
	}
	if time.Since(m.At) > 3*time.Second {
		return "—*"
	}
	return rateText(*m.SpeedBPS)
}
func rateText(n float64) string {
	if n >= 1024*1024 {
		return fmt.Sprintf("%.1f MiB/s", n/(1024*1024))
	}
	if n >= 1024 {
		return fmt.Sprintf("%.0f KiB/s", n/1024)
	}
	return fmt.Sprintf("%.0f B/s", n)
}
func eta(m core.Metrics) string {
	if m.ETASeconds == nil || time.Since(m.At) > 3*time.Second {
		return "—"
	}
	return elapsed(time.Duration(*m.ETASeconds * float64(time.Second)))
}
func elapsed(d time.Duration) string {
	seconds := int(d.Seconds())
	if seconds < 0 {
		seconds = 0
	}
	if seconds >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
	}
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}
func stageMetrics(j *job) core.Metrics {
	switch j.state {
	case "downloading", "waiting_processing":
		return j.record.Monitoring.Download
	case "processing", "waiting_verification":
		return j.record.Monitoring.Process
	case "verifying":
		return j.record.Monitoring.Verify
	}
	return core.Metrics{}
}
func progressCell(j *job, bars int) string {
	if j.state == "done" {
		return strings.Repeat("█", bars) + " 100%"
	}
	m := stageMetrics(j)
	if m.Percent == nil {
		return strings.Repeat("░", bars) + " —"
	}
	done := max(0, min(bars, int(*m.Percent/100*float64(bars))))
	return strings.Repeat("█", done) + strings.Repeat("░", bars-done) + " " + percent(m)
}
func bytesText(m core.Metrics) string {
	total := "—"
	if m.TotalBytes != nil {
		total = fileSize(*m.TotalBytes)
	}
	if m.At.IsZero() {
		return "— / " + total
	}
	return fileSize(m.DownloadedBytes) + " / " + total
}
func stamp(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("02.01 15:04:05")
}
func detailsText(j *job) string {
	m := j.record.Monitoring
	duration := "—"
	if !m.StartedAt.IsZero() {
		end := m.FinishedAt
		if end.IsZero() {
			end = time.Now()
		}
		duration = elapsed(end.Sub(m.StartedAt))
	}
	output := j.record.OutputPath
	if output == "" {
		output = "—"
	}
	return strings.Join([]string{
		fmt.Sprintf("#%d %s · %s", j.id, j.title, jobState(j.state)),
		fmt.Sprintf("%s / %s / %s · %s", j.options.Mode, codecLabel(videoCodec(j.options)), videoContainer(j.options), j.options.Quality),
		tr("Скачивание") + ": " + percent(m.Download) + " · " + bytesText(m.Download) + " · " + speed(m.Download) + " · ETA " + eta(m.Download),
		tr("Обработка") + ": " + percent(m.Process) + " · " + tr("Проверка") + ": " + percent(m.Verify),
		"URL: " + j.options.URL,
		tr("Источник") + ": " + j.info.Extractor,
		tr("Создано") + ": " + stamp(m.CreatedAt) + " · " + tr("Начато") + ": " + stamp(m.StartedAt),
		tr("Обновлено") + ": " + stamp(m.UpdatedAt) + " · " + tr("Завершено") + ": " + stamp(m.FinishedAt),
		tr("Затрачено") + ": " + duration + fmt.Sprintf(" · %s: %d", tr("Повторы"), m.Attempts),
		tr("Файл") + ": " + output,
		tr("Последняя ошибка") + ": " + j.record.Monitoring.LastError,
		fmt.Sprintf("Priority: %d · Position: %d · Paused: %t", j.record.Control.Priority, j.record.Control.Position, j.record.Control.Paused),
		jobHistoryText(j.record.History),
	}, "\n")
}
func helpText() string {
	return strings.Join([]string{
		tr("1–4:экраны c:CPU/RAM Enter:сведения ?:помощь q:выход"),
		"Space/A: select · p: pause/resume · [/]: priority · </>: order · {/}: first/last",
		tr("↑/↓ или j/k — выбрать задание"), tr("Tab / Shift+Tab — следующая / предыдущая вкладка"),
		tr("Enter — сведения или выбор качества"), tr("a — добавить видео или аудио; v — быстро добавить видео"),
		tr("/ — поиск по названию и URL; Esc — снять поиск"), tr("x — отменить; r — повторить; d — удалить только историю"),
		tr("s — настройки; f — папка загрузок; o — показать готовый файл"), tr("l — журнал; ? — справка; q — выход"),
		tr("Скорость с * устарела. Проценты относятся к отдельной стадии."), tr("Бюджет скорости делится на максимум загрузок; изменения — для новых процессов."),
	}, "\n")
}
func wrapLines(text string, width int) []string {
	result := []string{}
	if width < 1 {
		return result
	}
	for _, line := range strings.Split(text, "\n") {
		var b strings.Builder
		n := 0
		for _, r := range safeText(line) {
			size := cellWidth(r)
			if n+size > width {
				result = append(result, b.String())
				b.Reset()
				n = 0
			}
			b.WriteRune(r)
			n += size
		}
		result = append(result, b.String())
	}
	return result
}
func (u *panelUI) render() {
	if u.section != 0 && u.pool.workspace != nil {
		u.renderWorkspace()
		return
	}
	w, h := min(u.width-1, 240), min(u.height-1, 80)
	if w < 1 || h < 1 {
		return
	}
	c := newCanvas(w, h)
	c.text(1, 0, w-2, "CLIPNARO "+version+"  1: "+tr("Задания")+"  2: "+tr("Медиатека")+"  3: S3  4: "+tr("Профили")+"  c: CPU/RAM", cyan)
	if w < 39 || h < 9 {
		c.text(1, 2, w-2, tr("Увеличьте терминал: минимум 40 × 10."), normal)
		c.text(1, 4, w-2, "q: "+tr("Выход"), normal)
		if u.modal == "quit" {
			c.text(1, 5, w-2, u.choices[u.choiceIndex].Label, cyan)
			c.text(1, 6, w-2, "↑↓ Enter / Esc", normal)
		}
		u.draw(c)
		return
	}
	st := u.stats()
	limits := st.Limits.Effective()
	network := "—"
	if st.SpeedBPS != nil {
		network = rateText(*st.SpeedBPS)
	}
	budget := tr("Без ограничений")
	if limits.GlobalBPS > 0 {
		budget = rateText(float64(limits.GlobalBPS)) + " (" + tr("доли") + ")"
	}
	topHeader := fmt.Sprintf("↓ %s · DL %d/%d · CPU %d/%d", network, st.Downloads, limits.Downloads, st.Processing, limits.Processing)
	if u.pool.workspace != nil {
		active, rate, _ := u.pool.workspace.Cloud.Stats()
		speed := "—"
		if rate != nil {
			speed = rateText(*rate)
		}
		topHeader += fmt.Sprintf(" · ↑ S3 %s %d/%d", speed, active, u.pool.workspace.Cloud.Options().Objects)
	}
	topHeader += " · " + budget
	c.text(1, 1, w-2, topHeader, normal)
	summary := fmt.Sprintf("%s: ↓ %d / CPU %d · %s: %s", tr("Ожидают"), st.WaitingDownloads, st.WaitingProcessing, tr("Сессия"), fileSize(st.SessionBytes))
	if u.pool.workspace != nil {
		summary += " · " + u.resourceLine()
	}
	c.text(1, 2, w-2, summary, dim)
	if w >= 79 {
		x := 1
		labels := []string{"Все", "↓", "Ожидают", "CPU", "Готово", "Ошибки"}
		if w >= 119 {
			labels = tabNames
		}
		for tab, name := range labels {
			count := 0
			for _, j := range u.pool.jobs {
				if tabMatches(tab, j.state) {
					count++
				}
			}
			text := fmt.Sprintf(" %s:%d ", tr(name), count)
			style := dim
			if tab == u.tab {
				style = selected
			}
			c.text(x, 3, w-x-1, text, style)
			for _, r := range text {
				x += cellWidth(r)
			}
		}
	} else {
		c.text(1, 3, w-2, fmt.Sprintf("Tab %d/6: %s · / %s", u.tab+1, tr(tabNames[u.tab]), u.search), cyan)
	}
	detailsH, logH := 0, 0
	if w >= 119 && h >= 29 {
		detailsH = 7
		logH = 6
	} else if w >= 79 && h >= 19 {
		detailsH = 5
	}
	tableH := h - 6 - detailsH - logH
	c.box(0, 4, w, tableH, tr("Задания"), true)
	items := u.visibleJobs()
	u.reconcile()
	header := tr("Название")
	rowStart := 6
	if tableH < 4 {
		rowStart = 5
	} else {
		c.text(2, 5, w-4, header, dim)
	}
	progressX, speedX, etaX, stageX := w-64, w-46, w-32, w-23
	if w < 120 {
		progressX, speedX, etaX, stageX = w-50, w-37, w-25, w-17
	}
	if w >= 79 && tableH >= 4 {
		c.text(progressX, 5, 16, tr("Прогресс"), dim)
		c.text(speedX, 5, 12, tr("Скорость"), dim)
		c.text(etaX, 5, 7, "ETA", dim)
		c.text(stageX, 5, w-stageX-2, tr("Этап"), dim)
	}
	rows := max(1, tableH-2-(rowStart-5))
	start := max(0, u.jobIndex-rows+1)
	for i := start; i < len(items) && i < start+rows; i++ {
		j := items[i]
		y := rowStart + i - start
		style := normal
		prefix := "  "
		if j.id == u.selectedID {
			style = selected
			prefix = "› "
			c.fill(1, y, w-2, style)
		}
		if w < 79 {
			nameW := max(8, w-28)
			c.text(2, y, nameW, prefix+u.jobLabel(j), style)
			c.text(nameW+3, y, w-nameW-5, jobState(j.state)+" "+percent(stageMetrics(j)), style)
		} else {
			c.text(2, y, progressX-4, prefix+u.jobLabel(j), style)
			bars := 8
			if w < 120 {
				bars = 4
			}
			progressStyle := style
			if j.id == u.selectedID {
				progressStyle = selectedProgress
			}
			c.text(progressX, y, speedX-progressX-1, progressCell(j, bars), progressStyle)
			if j.state == "downloading" {
				c.text(speedX, y, etaX-speedX-1, speed(j.record.Monitoring.Download), style)
				c.text(etaX, y, stageX-etaX-1, eta(j.record.Monitoring.Download), style)
			} else {
				c.text(speedX, y, etaX-speedX-1, "—", style)
				c.text(etaX, y, stageX-etaX-1, "—", style)
			}
			c.text(stageX, y, w-stageX-2, jobState(j.state), style)
		}
	}
	if len(items) == 0 {
		c.text(2, 6, w-4, tr("Нет заданий. a — добавить; / — поиск."), dim)
	}
	y := 4 + tableH
	if detailsH > 0 {
		c.box(0, y, w, detailsH, tr("Сведения о задании"), false)
		if j := u.selectedJob(); j != nil {
			lines := strings.Split(detailsText(j), "\n")
			for i := 0; i < detailsH-2 && i < len(lines); i++ {
				c.text(2, y+1+i, w-4, lines[i], normal)
			}
		}
		y += detailsH
	}
	if logH > 0 {
		c.box(0, y, w, logH, tr("Журнал событий"), false)
		end := len(u.logs)
		start := max(0, end-logH+2)
		for i := start; i < end; i++ {
			c.text(2, y+1+i-start, w-4, u.logs[i], dim)
		}
	}
	if w >= 79 {
		c.text(1, h-2, w-2, tr("a:добавить Enter:сведения /:поиск x:отмена r:повтор d:история"), cyan)
		c.text(1, h-1, w-2, tr("s: Настройки Tab:вкладки f:папка o:показать l:журнал ?:помощь q:выход"), dim)
	} else {
		c.text(1, h-2, w-2, tr("a:+ Enter:сведения /:поиск x:отмена"), cyan)
		c.text(1, h-1, w-2, tr("s: Настройки Tab:вкладки ?:помощь q:выход"), dim)
	}
	if u.modal != "" {
		u.drawModal(c)
	}
	u.draw(c)
}
func (u *panelUI) draw(c *panelCanvas) {
	frame := c.String()
	if frame != u.lastFrame {
		fmt.Fprint(u.out, frame)
		u.lastFrame = frame
	}
}

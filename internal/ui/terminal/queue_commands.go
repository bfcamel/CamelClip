// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
)

func runQueue(ctx context.Context, urls []string, o options, config string, m *menu) error {
	if err := validateLinks(urls, o); err != nil {
		return err
	}
	if o.runtime == nil || o.runtime.Downloader == nil {
		return errors.New("downloader dependency is not configured")
	}
	if o.Formats {
		for _, url := range urls {
			o.URL = url
			if err := o.runtime.Downloader.Formats(ctx, request(o), func(e core.Event) { fmt.Println(e.Detail) }); err != nil {
				return err
			}
		}
		return nil
	}
	absolute, err := filepath.Abs(o.Out)
	if err != nil {
		return err
	}
	o.Out = absolute
	pool, err := openPersistentPool(ctx, o, config, 1)
	if err != nil {
		return err
	}
	defer pool.wait()
	ids := []int{}
	seen := map[int]bool{}
	for _, url := range urls {
		o.URL = url
		j, err := pool.service.Submit(request(o), o.chooseQuality)
		if err != nil && !errors.Is(err, application.ErrDuplicate) {
			return err
		}
		if !seen[j.ID] {
			ids = append(ids, j.ID)
			seen[j.ID] = true
		}
	}
	return runQueueJobs(ctx, pool.service, ids, o, config, m)
}
func runQueueJobs(ctx context.Context, q application.Queue, ids []int, o options, config string, m *menu) error {
	selected := map[int]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	q.StartOnly(ids)
	renderer := &progressWriter{out: os.Stderr}
	defer renderer.finish()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := q.Err(); err != nil {
			return err
		}
		pending, failed, done := 0, 0, 0
		for _, j := range q.Jobs() {
			if !selected[j.ID] {
				continue
			}
			switch j.State {
			case application.Done:
				done++
			case application.Failed, application.Canceled:
				failed++
			default:
				pending++
			}
			if j.State == application.Choosing {
				value := j.Request.Quality
				if m != nil {
					var err error
					value, err = m.selectItem(ctx, tr("Качество (максимальная высота)"), qualities(j.Info), value)
					if err != nil {
						if ctx.Err() != nil {
							return errCanceled
						}
						q.Cancel(j.ID)
						continue
					}
				}
				if err := q.Choose(j.ID, value); err != nil {
					return err
				}
			}
		}
		if pending == 0 {
			fmt.Printf(tr("\nЗавершено: %d. Ошибок: %d.\n"), done, failed)
			if failed > 0 {
				return errors.New(tr("часть ссылок не обработана; причины указаны выше"))
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return errCanceled
		case e := <-q.Events():
			if !selected[e.Job.ID] {
				continue
			}
			switch e.Kind {
			case core.StageChanged:
				fmt.Fprintln(os.Stderr, jobState(string(e.Job.State)), safeText(e.Job.Title))
				if e.Job.State == application.Failed {
					fmt.Fprintln(os.Stderr, explain(e.Job.Detail))
				}
			case core.Progress, core.Log:
				renderer.line(safeText(e.Detail))
			case core.Published:
				fmt.Fprintln(os.Stderr, tr("Сохранено:"), e.Job.OutputPath)
			}
		case <-ticker.C:
		}
	}
}
func storedQueueMenu(ctx context.Context, m *menu, o options, config string) error {
	p, err := openPersistentPool(ctx, o, config, 1)
	if err != nil {
		return err
	}
	defer p.wait()
	jobs := p.service.Jobs()
	if len(jobs) == 0 {
		fmt.Fprintln(m.out, tr("Очередь пуста."))
		return nil
	}
	for _, j := range jobs {
		fmt.Fprintf(m.out, "#%d %s: %s\n", j.ID, safeText(j.Title), jobState(string(j.State)))
	}
	action, err := m.selectItem(ctx, tr("Сохранённая очередь"), []choice{{tr("Продолжить незавершённые"), "resume"}, {tr("Повторить ошибки и отменённые"), "retry"}, {tr("Убрать завершённые записи"), "clear"}, {tr("← Главное меню"), "back"}}, "resume")
	if err != nil {
		return err
	}
	ids := []int{}
	for _, j := range jobs {
		if action == "clear" && j.State.Terminal() {
			if err = p.service.Remove(j.ID); err != nil {
				return err
			}
		}
		if action == "resume" && !j.State.Terminal() {
			ids = append(ids, j.ID)
		}
		if action == "retry" && (j.State == application.Failed || j.State == application.Canceled) {
			if err = p.service.Retry(j.ID, o.Cookies); err != nil {
				return err
			}
			ids = append(ids, j.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return runQueueJobs(ctx, p.service, ids, o, config, m)
}

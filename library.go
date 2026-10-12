package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type libraryFile struct {
	Name, Path string
	Size       int64
	Modified   time.Time
}

var intermediateName = regexp.MustCompile(`\.f\d+\.[^.]+$`)

func listMedia(dir string) ([]libraryFile, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	files := []libraryFile{}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || intermediateName.MatchString(name) {
			continue
		}
		if !contains([]string{".mp4", ".mkv", ".webm", ".mov", ".m4v", ".mp3", ".m4a", ".opus", ".flac", ".wav", ".ogg", ".aac"}, strings.ToLower(filepath.Ext(name))) {
			continue
		}
		info, e := entry.Info()
		if e != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, libraryFile{name, filepath.Join(dir, name), info.Size(), info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].Modified.Equal(files[j].Modified) {
			return files[i].Name < files[j].Name
		}
		return files[i].Modified.After(files[j].Modified)
	})
	return files, nil
}
func fileSize(n int64) string {
	v := float64(n)
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}
func shortName(s string, max int) string {
	r := []rune(safeText(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max-1]) + "…"
}
func openInFinder(ctx context.Context, path string, reveal bool) error {
	args := []string{}
	if reveal {
		args = append(args, "-R")
	}
	args = append(args, "--", path)
	return exec.CommandContext(ctx, "open", args...).Run()
}
func library(ctx context.Context, m *menu, o options) error {
	page := 0
	notice := ""
	for {
		files, err := listMedia(o.Out)
		if err != nil {
			return err
		}
		pages := (len(files) + 5) / 6
		if pages == 0 {
			pages = 1
		}
		if page >= pages {
			page = pages - 1
		}
		screen(m, tr("Мои файлы"), o, notice)
		notice = ""
		fmt.Fprintf(m.out, tr("Файлов: %d · Страница %d/%d\n"), len(files), page+1, pages)
		items := []choice{}
		if len(files) == 0 {
			fmt.Fprintln(m.out, tr("Здесь пока нет видео или аудио."))
		}
		for i := page * 6; i < len(files) && i < (page+1)*6; i++ {
			f := files[i]
			items = append(items, choice{fmt.Sprintf("%-38s  %9s  %s", shortName(f.Name, 38), fileSize(f.Size), f.Modified.Format("02 Jan 15:04")), f.Path})
		}
		if page+1 < pages {
			items = append(items, choice{tr("Следующая страница →"), "next"})
		}
		if page > 0 {
			items = append(items, choice{tr("← Предыдущая страница"), "prev"})
		}
		items = append(items, choice{tr("Открыть папку в Finder"), "folder"}, choice{tr("Обновить список"), "refresh"}, choice{tr("← Главное меню"), "back"})
		selected, e := m.selectItem(ctx, tr("Выберите файл или действие"), items, "")
		if e != nil {
			return e
		}
		switch selected {
		case "back":
			return nil
		case "next":
			page++
		case "prev":
			page--
		case "refresh":
		case "folder":
			if e = os.MkdirAll(o.Out, 0755); e == nil {
				e = openInFinder(ctx, o.Out, false)
			}
			if e != nil {
				notice = e.Error()
			}
		default:
			screen(m, tr("Файл"), o, "")
			fmt.Fprintln(m.out, safeText(filepath.Base(selected)))
			for _, f := range files {
				if f.Path == selected {
					fmt.Fprintf(m.out, "%s · %s\n\n", fileSize(f.Size), f.Modified.Format("2006-01-02 15:04"))
				}
			}
			action, e := m.selectItem(ctx, tr("Действие"), []choice{{tr("Открыть файл"), "open"}, {tr("Показать в Finder"), "reveal"}, {tr("← К списку файлов"), "back"}}, "open")
			if e != nil {
				if ctx.Err() != nil {
					return e
				}
				continue
			}
			if action != "back" {
				if e = openInFinder(ctx, selected, action == "reveal"); e != nil {
					notice = e.Error()
				}
			}
		}
	}
}

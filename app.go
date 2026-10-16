package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

type links []string

func (l *links) String() string     { return strings.Join(*l, " ") }
func (l *links) Set(s string) error { *l = append(*l, s); return nil }
func run() error {
	system := systemLanguage()
	useLanguage(requestedLanguage(os.Args[1:], "auto"), system)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(configDir, "MacDown", "settings.json")
	if override := os.Getenv("MACDOWN_CONFIG_DIR"); override != "" {
		configPath = filepath.Join(override, "settings.json")
	}
	o := defaultOptions(home)
	if err = loadSettings(configPath, &o); err != nil {
		fmt.Fprintln(os.Stderr, tr("Не удалось прочитать настройки; используются стандартные значения:"), err)
	}
	useLanguage(requestedLanguage(os.Args[1:], o.Lang), system)
	request, err := parseCLI(os.Args[1:], &o)
	if err != nil {
		return err
	}
	useLanguage(o.Lang, system)
	if request.help {
		printHelp(request.command)
		return nil
	}
	switch request.command {
	case "version":
		fmt.Println("macdown " + version)
		return nil
	case "config":
		return showConfig(configPath, o)
	case "doctor":
		return doctor(ctx)
	}
	urls, listPath := request.urls, request.list
	if listPath != "" {
		f, e := os.Open(expandHome(listPath, home))
		if e != nil {
			return fmt.Errorf(tr("не удалось открыть список: %w"), e)
		}
		more, e := readLinks(f)
		f.Close()
		if e != nil {
			return e
		}
		urls = append(urls, more...)
		if len(urls) == 0 {
			return errors.New(tr("файл списка не содержит ссылок"))
		}
	}
	o.Out, err = filepath.Abs(expandHome(o.Out, home))
	if err != nil {
		return err
	}
	if len(urls) == 0 {
		m := newMenu()
		m.ctx = ctx
		return dashboard(ctx, m, &o, configPath, system, home)
	}
	return runQueue(ctx, urls, o, configPath, nil)
}

func runQueue(ctx context.Context, urls []string, o options, configPath string, m *menu) error {
	if err := validateLinks(urls, o); err != nil {
		return err
	}
	executable, err := exec.LookPath("yt-dlp")
	if err != nil {
		return errors.New(tr("не найден yt-dlp. Установка: brew install yt-dlp ffmpeg deno"))
	}
	if o.Formats {
		failed := 0
		for _, link := range urls {
			o.URL = link
			c := command(ctx, executable, arguments(o)...)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if e := c.Run(); e != nil {
				if ctx.Err() != nil {
					return errCanceled
				}
				failed++
			}
		}
		if failed > 0 {
			return fmt.Errorf("%d format queries failed", failed)
		}
		return nil
	}
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if _, err = exec.LookPath(name); err != nil {
			return fmt.Errorf(tr("не найден %s. Установка: brew install ffmpeg"), name)
		}
	}
	o.Out, err = filepath.Abs(o.Out)
	if err != nil {
		return err
	}
	pool, err := openPersistentPool(ctx, executable, configPath)
	if err != nil {
		return err
	}
	defer pool.store.close()
	selected := []*job{}
	seen := map[int]bool{}
	for _, link := range urls {
		o.URL = link
		var item *job
		if pool.add(o) {
			item = pool.jobs[len(pool.jobs)-1]
		} else {
			for _, j := range pool.jobs {
				if j.options.URL == link && j.options.Out == o.Out && outputVariant(j.options) == outputVariant(o) && !contains([]string{"done", "failed", "canceled"}, j.state) {
					item = j
					break
				}
			}
		}
		if item != nil && !seen[item.id] {
			selected = append(selected, item)
			seen[item.id] = true
		}
	}
	// Persist the entire requested list before starting the first network operation.
	if err = pool.saveQueue(); err != nil {
		return err
	}
	return runSequentialJobs(ctx, pool, selected, m, configPath)
}
func runSequentialJobs(ctx context.Context, pool *jobPool, jobs []*job, m *menu, config string) error {
	failed, done := 0, 0
	for index, j := range jobs {
		if ctx.Err() != nil {
			return errCanceled
		}
		o := j.options
		fmt.Printf("\n[%d/%d] %s\n", index+1, len(jobs), o.URL)
		var err error
		if j.state != "ready" && j.state != "choose" {
			j.state = "inspecting"
			j.detail = ""
			if err = pool.saveQueue(); err != nil {
				return err
			}
			fmt.Println(tr("Получение доступных качеств…"))
			j.info, err = inspect(ctx, pool.executable, o)
			if err == nil {
				j.title = j.info.Title
				o.duration = j.info.Duration
				j.options = o
				if o.Mode == "video" && m != nil {
					j.state = "choose"
				} else {
					j.state = "ready"
				}
			}
		}
		if err == nil {
			fmt.Println(safeText(j.title))
			if j.state == "choose" && m != nil {
				o.Quality, err = m.selectItem(ctx, tr("Качество (максимальная высота)"), qualities(j.info), o.Quality)
			}
			if err == nil {
				j.options = o
				j.state = "ready"
				if err = pool.saveQueue(); err != nil {
					return err
				}
				j.state = "downloading"
				if err = pool.saveQueue(); err != nil {
					return err
				}
				renderer := &progressWriter{out: os.Stderr}
				var storeErr error
				err = verifiedDownloadWithObserver(ctx, pool.executable, o, renderer, func(stage, detail string) {
					if storeErr != nil {
						return
					}
					if detail == "" {
						j.state = stage
						j.detail = ""
						storeErr = pool.saveQueue()
					}
				})
				renderer.finish()
				if storeErr != nil {
					return storeErr
				}
			}
		}
		if ctx.Err() != nil {
			return errCanceled
		} // active state remains resumable on disk
		if errors.Is(err, errCanceled) {
			j.state = "canceled"
			j.detail = ""
		} else if err != nil {
			j.state = "failed"
			j.detail = err.Error()
		} else {
			j.state = "done"
			j.detail = "100%"
		}
		if saveErr := pool.saveQueue(); saveErr != nil {
			return saveErr
		}
		if err != nil {
			failed++
			fmt.Fprintln(os.Stderr, tr("Не удалось:"), err)
		} else {
			done++
		}
		if err == nil && m != nil {
			if e := saveSettings(config, o); e != nil {
				fmt.Fprintln(os.Stderr, e)
			}
		}
	}
	fmt.Printf(tr("\nЗавершено: %d. Ошибок: %d.\n"), done, failed)
	if failed > 0 {
		return errors.New(tr("часть ссылок не обработана; причины указаны выше"))
	}
	return nil
}

func readLinks(r io.Reader) ([]string, error) {
	result := []string{}
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 4096), 1024*1024)
	for s.Scan() {
		v := strings.TrimSpace(s.Text())
		if v != "" && !strings.HasPrefix(v, "#") {
			result = append(result, v)
		}
	}
	return result, s.Err()
}
func validateLinks(urls []string, o options) error {
	for _, u := range urls {
		o.URL = u
		if err := validate(o); err != nil {
			return fmt.Errorf("%s: %w", u, err)
		}
	}
	return nil
}
func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, s)
}

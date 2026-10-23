// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type clipRanges []string

func (v *clipRanges) String() string     { return strings.Join(*v, "\n") }
func (v *clipRanges) Set(s string) error { *v = append(*v, s); return nil }
func runClipsCLI(ctx context.Context, o options, config string, args []string) error {
	help := `Clip by Link (precise processing; no background daemon)
  camelclip clips inspect --url URL [--browser firefox] --json
  camelclip clips plan --url URL --range '00:01..00:05 | Title' --out DIR
  camelclip clips plan --asset ASSET_ID --range '0..5' [--profile ID,ID]
  camelclip clips create --plan-id ID --revision 1 --digest DIGEST --idempotency-key KEY
  camelclip clips run BATCH_ID
  camelclip clips list [--cursor CURSOR] [--limit 50]
  camelclip clips items BATCH_ID
  camelclip clips pause|resume|cancel|retry|cleanup BATCH_ID --revision CONTROL_REVISION [--task TASK_ID]
  camelclip clips provenance TASK_ID
  camelclip clips deliveries
Options: --mode video|audio --quality best|1080 --audio m4a|mp3|opus|flac|wav
         --ranges-file FILE (one START..END | title per line)
         --chapters 0,1 (source chapter indices) --browser NAME --json
S3 delivery is explicit; empty selection means local only. Full-source acquisition
is required for remote URLs. Keep the worker or desktop application running.
`
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Print(help)
		return nil
	}
	command := args[0]
	rest := args[1:]
	id := ""
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		id = rest[0]
		rest = rest[1:]
	}
	set := flag.NewFlagSet("clips", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	var url, asset, out, mode, audio, quality, browser, profile, plan, digest, key, cursor, task, rangesFile, chapters string
	var ranges clipRanges
	var revision int64
	var limit int
	var jsonOutput, showHelp bool
	set.StringVar(&url, "url", "", "")
	set.StringVar(&asset, "asset", "", "")
	set.StringVar(&out, "out", o.Out, "")
	set.StringVar(&mode, "mode", o.Mode, "")
	set.StringVar(&audio, "audio", "m4a", "")
	set.StringVar(&quality, "quality", o.Quality, "")
	set.StringVar(&browser, "browser", o.Cookies, "")
	set.StringVar(&profile, "profile", "", "")
	set.StringVar(&plan, "plan-id", "", "")
	set.StringVar(&digest, "digest", "", "")
	set.StringVar(&key, "idempotency-key", "", "")
	set.StringVar(&cursor, "cursor", "", "")
	set.StringVar(&task, "task", "", "")
	set.StringVar(&rangesFile, "ranges-file", "", "")
	set.StringVar(&chapters, "chapters", "", "")
	set.Int64Var(&revision, "revision", 1, "")
	set.IntVar(&limit, "limit", 50, "")
	set.Var(&ranges, "range", "")
	set.BoolVar(&jsonOutput, "json", false, "")
	set.BoolVar(&showHelp, "help", false, "")
	if e := set.Parse(rest); e != nil {
		return &usageError{e.Error()}
	}
	if set.NArg() > 0 {
		return &usageError{"unexpected clip arguments"}
	}
	if showHelp {
		fmt.Print(help)
		return nil
	}
	p, e := openPersistentPool(ctx, o, config, 5)
	if e != nil {
		return e
	}
	defer p.wait()
	if p.workspace == nil || p.workspace.Clips == nil {
		return errors.New("clip service unavailable")
	}
	s := p.workspace.Clips
	p.workspace.Cloud.Restrict([]string{})
	switch command {
	case "inspect", "plan":
		out, e = filepath.Abs(out)
		if e != nil {
			return e
		}
		r := core.ClipRequest{Source: core.ClipSourceRef{URL: url, AssetID: asset}, Mode: mode, Audio: audio, Quality: quality, OutputDir: out, Browser: browser, Accuracy: "precise"}
		if command == "inspect" {
			v, e := s.Inspect(ctx, r)
			if e != nil {
				return e
			}
			return emitJSON(v)
		}
		if profile != "" {
			r.S3Profiles = strings.Split(profile, ",")
		}
		if rangesFile != "" {
			f, e := os.Open(rangesFile)
			if e != nil {
				return e
			}
			data, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
			f.Close()
			if e != nil {
				return e
			}
			if len(data) > 1<<20 {
				return errors.New("range manifest exceeds 1 MiB")
			}
			ranges = append(ranges, string(data))
		}
		if len(ranges) > 0 {
			r.Segments, e = core.ParseClipSegments(strings.Join(ranges, "\n"))
			if e != nil {
				return e
			}
		}
		if chapters != "" {
			for _, part := range strings.Split(chapters, ",") {
				var index int
				if _, e = fmt.Sscanf(part, "%d", &index); e != nil || index < 0 {
					return &usageError{"invalid chapter index"}
				}
				r.ChapterIndices = append(r.ChapterIndices, index)
			}
		}
		v, e := s.Plan(ctx, r)
		if e != nil {
			return e
		}
		return emitJSON(v)
	case "create":
		v, e := s.Admit(ctx, plan, revision, digest, key)
		if e != nil {
			return e
		}
		return emitJSON(v)
	case "list":
		v, e := s.List(ctx, limit, cursor)
		if e != nil {
			return e
		}
		return emitJSON(v)
	case "items":
		v, e := s.Get(ctx, id)
		if e != nil {
			return e
		}
		return emitJSON(v)
	case "provenance":
		v, e := s.Provenance(ctx, id)
		if e != nil {
			return e
		}
		return emitJSON(v)
	case "deliveries":
		v, e := s.Deliveries(ctx)
		if e != nil {
			return e
		}
		return emitJSON(v)
	case "pause", "resume", "cancel", "retry", "cleanup":
		v, e := s.Control(ctx, id, revision, command, task)
		if e != nil {
			return e
		}
		return emitJSON(v)
	case "run":
		if id == "" {
			return &usageError{"batch ID required"}
		}
		if _, e = s.Get(ctx, id); e != nil {
			return e
		}
		s.Start(id)
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		for {
			v, e := s.Get(ctx, id)
			if e != nil {
				return e
			}
			if v.Paused {
				return errors.New("batch is paused; resume before running")
			}
			if v.Settled() {
				_ = emitJSON(v)
				if v.Status() != "completed" {
					return errors.New("clip batch settled with failed or canceled tasks")
				}
				return nil
			}
			if e = s.Err(); e != nil {
				return e
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-tick.C:
			}
		}
	default:
		return &usageError{"unknown clips command"}
	}
}

// Use the ordinary details modal for readable, safely escaped terminal output.
func clipDetails(b core.ClipBatch) string {
	data, _ := json.MarshalIndent(b, "", "  ")
	return string(data)
}

func (u *panelUI) clipKey(key string) bool {
	s := u.pool.workspace.Clips
	if s == nil {
		return false
	}
	if key == "n" {
		u.clipForm = core.ClipRequest{Mode: u.o.Mode, Audio: "m4a", Quality: u.o.Quality, OutputDir: u.o.Out, Browser: u.o.Cookies, Accuracy: "precise"}
		u.clipStep = 0
		u.nextClipField()
		return true
	}
	if u.assetIndex < 0 || u.assetIndex >= len(u.clips) {
		return false
	}
	b := u.clips[u.assetIndex]
	switch key {
	case "enter":
		u.modal = "details"
		u.title = tr("Фрагменты")
		u.input = clipDetails(b)
		u.detailOffset = 0
	case "p", "x", "r":
		action := "pause"
		if key == "p" && b.Paused {
			action = "resume"
		}
		if key == "x" {
			action = "cancel"
		}
		if key == "r" {
			action = "retry"
		}
		u.background(func() error { _, e := s.Control(u.ctx, b.ID, b.ControlRevision, action, ""); return e })
	default:
		return false
	}
	return true
}
func (u *panelUI) nextClipField() {
	labels := []string{"Ссылка или asset:ID из медиатеки", "Интервалы: 00:01..00:05 | Название; 00:10..00:15 | Название", "Папка сохранения", "Режим: video / audio", "Качество: best / 1080 / 720", "Аудиоформат: m4a / mp3 / opus / flac / wav", "Cookies браузера (пусто: отключены)", "S3: ID профилей через запятую (пусто: локально)"}
	values := []string{"", "", u.clipForm.OutputDir, u.clipForm.Mode, u.clipForm.Quality, u.clipForm.Audio, u.clipForm.Browser, ""}
	if u.clipStep >= len(labels) {
		r := u.clipForm
		u.modal = ""
		if u.clipPlans == nil {
			u.clipPlans = make(chan core.ClipPlan, 1)
		}
		out := u.clipPlans
		s := u.pool.workspace.Clips
		u.background(func() error {
			p, e := s.Plan(u.ctx, r)
			if e != nil {
				return e
			}
			select {
			case out <- p:
			case <-u.ctx.Done():
				return u.ctx.Err()
			}
			return nil
		})
		return
	}
	u.modal = "clip-field"
	u.title = tr(labels[u.clipStep])
	u.input = values[u.clipStep]
}
func (u *panelUI) clipField(value string) {
	r := &u.clipForm
	switch u.clipStep {
	case 0:
		if strings.HasPrefix(value, "asset:") {
			r.Source.AssetID = strings.TrimPrefix(value, "asset:")
		} else {
			r.Source.URL = value
		}
	case 1:
		segments, e := core.ParseClipSegments(strings.ReplaceAll(value, ";", "\n"))
		if e != nil {
			u.log(e.Error())
			return
		}
		r.Segments = segments
	case 2:
		r.OutputDir = value
	case 3:
		r.Mode = value
	case 4:
		r.Quality = value
	case 5:
		r.Audio = value
	case 6:
		r.Browser = value
	case 7:
		if strings.TrimSpace(value) != "" {
			for _, id := range strings.Split(value, ",") {
				r.S3Profiles = append(r.S3Profiles, strings.TrimSpace(id))
			}
		}
	}
	u.clipStep++
	u.nextClipField()
}
func (u *panelUI) receiveClipPlan() {
	if u.clipPlans == nil {
		return
	}
	select {
	case p := <-u.clipPlans:
		u.clipPlan = p
		u.action = "clip-admit"
		text := fmt.Sprintf("%s · %d %s\n%s\n", p.Source.Title, len(p.Request.Segments), tr("Фрагменты"), p.Request.OutputDir)
		for _, seg := range p.Request.Segments {
			text += fmt.Sprintf("%.3f..%.3f · %s\n", float64(seg.Range.StartMS)/1000, float64(seg.Range.EndMS)/1000, seg.Title)
		}
		text += tr("Точная нарезка. Источник скачивается целиком один раз. Создать?")
		u.modal = "clip-review"
		u.title = tr("Проверка нарезки")
		u.input = text
		u.detailOffset = 0
		u.choices = nil
	default:
	}
}

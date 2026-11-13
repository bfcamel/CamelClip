// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
	"golang.org/x/term"
)

func workspaceCommand(args []string) bool {
	return len(args) > 0 && (args[0] == "playlists" || args[0] == "queue" || args[0] == "library" || args[0] == "storage")
}

type catalogFlags struct {
	initial, quality, out, mode, codec, audio                                                           string
	interval                                                                                            int
	help                                                                                                bool
	present                                                                                             map[string]bool
	enabled, provider                                                                                   string
	json, yes, recursive, auto, loopback                                                                bool
	profile, name, endpoint, region, bucket, prefix, style, kind, status, container, sort, since, until string
	passwordFD, credentialsFD, limit, offset, height                                                    int
}

func parseCatalogFlags(args []string) (catalogFlags, []string, error) {
	o := catalogFlags{}
	set := flag.NewFlagSet("camelclip", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&o.initial, "initial", "new_only", "")
	set.IntVar(&o.interval, "interval", 15, "")
	set.StringVar(&o.quality, "quality", "", "")
	set.StringVar(&o.out, "out", "", "")
	set.StringVar(&o.mode, "mode", "", "")
	set.StringVar(&o.codec, "codec", "", "")
	set.StringVar(&o.audio, "audio", "", "")
	set.BoolVar(&o.help, "help", false, "")
	set.BoolVar(&o.help, "h", false, "")
	set.BoolVar(&o.json, "json", false, "")
	set.BoolVar(&o.yes, "yes", false, "")
	set.BoolVar(&o.recursive, "recursive", false, "")
	set.BoolVar(&o.auto, "auto", false, "")
	set.BoolVar(&o.loopback, "allow-loopback-http", false, "")
	for key, value := range map[string]*string{"profile": &o.profile, "name": &o.name, "endpoint": &o.endpoint, "region": &o.region, "bucket": &o.bucket, "prefix": &o.prefix, "style": &o.style, "kind": &o.kind, "status": &o.status, "container": &o.container, "sort": &o.sort, "enabled": &o.enabled, "provider": &o.provider, "since": &o.since, "until": &o.until} {
		set.StringVar(value, key, "", "")
	}
	set.IntVar(&o.passwordFD, "password-fd", -1, "")
	set.IntVar(&o.credentialsFD, "credentials-fd", -1, "")
	set.IntVar(&o.limit, "limit", 50, "")
	set.IntVar(&o.offset, "offset", 0, "")
	set.IntVar(&o.height, "min-height", 0, "")
	flags, positions := []string{}, []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positions = append(positions, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") {
			positions = append(positions, a)
			continue
		}
		name := strings.TrimLeft(strings.SplitN(a, "=", 2)[0], "-")
		f := set.Lookup(name)
		if f == nil {
			return o, nil, &usageError{"unknown option: " + name}
		}
		flags = append(flags, a)
		if b, ok := f.Value.(interface{ IsBoolFlag() bool }); (!ok || !b.IsBoolFlag()) && !strings.Contains(a, "=") {
			i++
			if i >= len(args) {
				return o, nil, &usageError{"option needs a value: " + name}
			}
			flags = append(flags, args[i])
		}
	}
	if e := set.Parse(flags); e != nil {
		return o, nil, &usageError{e.Error()}
	}
	o.present = map[string]bool{}
	set.Visit(func(f *flag.Flag) { o.present[f.Name] = true })
	return o, positions, nil
}
func emitJSON(v any) error {
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	return e.Encode(v)
}
func secretFD(fd int) ([]byte, error) {
	if fd < 3 {
		return nil, errors.New("secret descriptor must be >= 3; use a private pipe or file")
	}
	f := os.NewFile(uintptr(fd), "private input")
	if f == nil {
		return nil, errors.New("invalid secret descriptor")
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if st.Mode().IsRegular() && st.Mode().Perm()&0077 != 0 {
		return nil, errors.New("secret file must have owner-only permissions")
	}
	data, e := io.ReadAll(io.LimitReader(f, 65537))
	if len(data) > 65536 {
		clear(data)
		return nil, errors.New("secret input too large")
	}
	return data, e
}
func masterPassword(fd int) ([]byte, error) {
	if fd >= 0 {
		data, e := secretFD(fd)
		if e != nil {
			return nil, e
		}
		return bytes.TrimRight(data, "\r\n"), nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, errors.New("S3 requires a TTY password or --password-fd (no password in command arguments)")
	}
	fmt.Fprint(os.Stderr, tr("Мастер-пароль S3: "))
	data, e := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return data, e
}
func unlockCLI(ctx context.Context, w *application.Workspace, fd int, initialize bool) error {
	password, e := masterPassword(fd)
	if e != nil {
		return e
	}
	defer clear(password)
	if !w.Cloud.HasVault(ctx) {
		if !initialize {
			return errors.New("configure an S3 profile first")
		}
		if fd < 0 {
			fmt.Fprint(os.Stderr, tr("Повторите мастер-пароль: "))
			confirm, e := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
			if e != nil {
				return e
			}
			same := string(password) == string(confirm)
			clear(confirm)
			if !same {
				return errors.New("passwords do not match")
			}
		}
		return w.Cloud.Initialize(ctx, password)
	}
	return w.Cloud.Unlock(ctx, password)
}
func readCredentialInput(fd int) (core.Credentials, error) {
	if fd >= 0 {
		data, e := secretFD(fd)
		if e != nil {
			return core.Credentials{}, e
		}
		defer clear(data)
		var input struct {
			Access string `json:"access_key_id"`
			Secret string `json:"secret_access_key"`
			Token  string `json:"session_token"`
		}
		if e = json.Unmarshal(data, &input); e != nil {
			return core.Credentials{}, errors.New("invalid credential JSON on private descriptor")
		}
		return core.Credentials{AccessKey: input.Access, SecretKey: input.Secret, SessionToken: input.Token}, nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return core.Credentials{}, errors.New("credentials require TTY or --credentials-fd")
	}
	values := make([]string, 3)
	for i, label := range []string{"Access key ID: ", "Secret access key: ", "Session token (optional): "} {
		fmt.Fprint(os.Stderr, label)
		b, e := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if e != nil {
			return core.Credentials{}, e
		}
		values[i] = string(b)
		clear(b)
	}
	return core.Credentials{AccessKey: values[0], SecretKey: values[1], SessionToken: values[2]}, nil
}
func runWorkspaceCLI(ctx context.Context, o options, config string, args []string) error {
	f, pos, e := parseCatalogFlags(args)
	if e != nil {
		return e
	}
	if f.help {
		printWorkspaceHelp()
		return nil
	}
	if len(pos) < 2 {
		return &usageError{"queue/library/storage requires a subcommand"}
	}
	p, e := openPersistentPool(ctx, o, config, 5)
	if e != nil {
		return e
	}
	defer p.wait()
	if p.workspace == nil {
		return errors.New("workspace services unavailable")
	}
	w := p.workspace
	w.Cloud.Restrict([]string{})
	switch pos[0] {
	case "playlists":
		return playlistCLI(ctx, w, pos[1:], f, o)
	case "queue":
		return queueCLI(ctx, p, pos[1:], f, o)
	case "library":
		return libraryCLI(ctx, w, pos[1:], f, o)
	case "storage":
		return storageCLI(ctx, w, pos[1:], f)
	}
	return errors.New("unknown workspace command")
}
func queueCLI(ctx context.Context, p *jobPool, pos []string, f catalogFlags, o options) error {
	switch pos[0] {
	case "list":
		jobs := p.service.Jobs()
		if f.json {
			return emitJSON(jobs)
		}
		for _, j := range jobs {
			fmt.Printf("#%d %s %s priority=%d paused=%t\n", j.ID, j.State, safeText(j.Title), j.Control.Priority, j.Control.Paused)
		}
		return nil
	case "add":
		if len(pos) < 2 {
			return &usageError{"queue add requires at least one URL"}
		}
		results := []application.ActionResult{}
		for _, url := range pos[1:] {
			r := request(o)
			r.URL = url
			j, e := p.service.Submit(r, false)
			result := application.ActionResult{ID: j.ID}
			if e != nil {
				result.Error = e.Error()
			}
			results = append(results, result)
		}
		return emitActionResults(results)
	}
	if len(pos) < 2 {
		return &usageError{"job ID required"}
	}
	ids := []int{}
	for _, raw := range strings.Split(pos[1], ",") {
		id, e := strconv.Atoi(raw)
		if e != nil {
			return &usageError{"invalid job ID"}
		}
		ids = append(ids, id)
	}
	switch pos[0] {
	case "priority":
		if len(pos) < 3 {
			return &usageError{"low/normal/high required"}
		}
		for _, id := range ids {
			n, ok := map[string]int{"low": -1, "normal": 0, "high": 1}[pos[2]]
			if !ok {
				return &usageError{"low/normal/high required"}
			}
			if e := p.service.SetPriority(id, n); e != nil {
				return e
			}
		}
		return nil
	case "reorder":
		if len(pos) < 3 {
			return &usageError{"up/down/first/last required"}
		}
		for _, id := range ids {
			if e := p.service.Reorder(id, pos[2]); e != nil {
				return e
			}
		}
		return nil
	case "pause", "resume", "retry", "cancel", "remove":
		if pos[0] == "remove" && !f.yes {
			return errors.New("history removal requires --yes; local files are kept")
		}
		return emitActionResults(p.service.BulkActionWithOptions(ids, pos[0], application.RetryOptions{Browser: &o.Cookies}))
	}
	return &usageError{"unknown queue action"}
}
func emitActionResults(results []application.ActionResult) error {
	if err := emitJSON(results); err != nil {
		return err
	}
	for _, result := range results {
		if result.Error != "" {
			return errors.New("one or more queue actions failed; see JSON results")
		}
	}
	return nil
}
func libraryCLI(ctx context.Context, w *application.Workspace, pos []string, f catalogFlags, o options) error {
	switch pos[0] {
	case "list", "search":
		q := core.AssetQuery{Limit: f.limit, Offset: f.offset, Kind: f.kind, Status: f.status, Container: f.container, MinHeight: f.height, Sort: f.sort}
		if len(pos) > 1 {
			q.Search = pos[1]
		}
		if f.since != "" {
			t, e := time.Parse("2006-01-02", f.since)
			if e != nil {
				return e
			}
			q.Since = t
		}
		if f.until != "" {
			t, e := time.Parse("2006-01-02", f.until)
			if e != nil {
				return e
			}
			q.Until = t.Add(24*time.Hour - time.Nanosecond)
		}
		page, e := w.Library.List(ctx, q)
		if e != nil {
			return e
		}
		if f.json {
			return emitJSON(page)
		}
		for _, a := range page.Items {
			fmt.Printf("%s %s %s %s\n", a.ID, a.Status, fileSize(a.Size), safeText(a.Path))
		}
		return nil
	case "scan":
		root := o.Out
		if len(pos) > 1 {
			root = pos[1]
		}
		r, e := w.Library.Scan(ctx, root, f.recursive)
		if e != nil {
			return e
		}
		return emitJSON(r)
	}
	if len(pos) < 2 {
		return &usageError{"asset ID or path required"}
	}
	switch pos[0] {
	case "import":
		a, e := w.Library.Import(ctx, pos[1])
		if e != nil {
			return e
		}
		return emitJSON(a)
	case "refresh":
		a, e := w.Library.Refresh(ctx, pos[1])
		if e != nil {
			return e
		}
		return emitJSON(a)
	case "delete-file":
		return w.Library.DeleteFile(ctx, pos[1], f.yes)
	case "show", "open", "reveal":
		a, e := w.Library.Get(ctx, pos[1])
		if e != nil {
			return e
		}
		if pos[0] == "show" {
			return emitJSON(a)
		}
		return openMedia(ctx, a.Path, pos[0] == "reveal")
	}
	return &usageError{"unknown library action"}
}
func storageCLI(ctx context.Context, w *application.Workspace, pos []string, f catalogFlags) error {
	switch pos[0] {
	case "unlock":
		if f.passwordFD >= 0 || !term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("standalone unlock is an interactive session; use --password-fd with the upload command in headless mode")
		}
		if e := unlockCLI(ctx, w, -1, false); e != nil {
			return e
		}
		w.Cloud.Restrict(nil)
		fmt.Println(tr("Хранилище разблокировано в этом сеансе. Enter — заблокировать и выйти."))
		_, e := bufio.NewReader(os.Stdin).ReadString('\n')
		return e
	case "reset":
		return w.Cloud.Reset(ctx, f.yes)
	case "change-password":
		if e := unlockCLI(ctx, w, f.passwordFD, false); e != nil {
			return e
		}
		return errors.New("change the master password in TUI: 4 then m")
	case "profiles":
		return profilesCLI(ctx, w, pos[1:], f)
	case "uploads":
		if len(pos) < 2 || pos[1] == "list" {
			jobs, e := w.Cloud.List(ctx, f.limit, f.offset)
			if e != nil {
				return e
			}
			return emitJSON(jobs)
		}
		if len(pos) < 3 {
			return &usageError{"upload ID required"}
		}
		id := pos[2]
		switch pos[1] {
		case "cancel":
			return w.Cloud.Cancel(ctx, id)
		case "retry":
			if e := w.Cloud.Retry(ctx, id, f.yes); e != nil {
				return e
			}
			if f.passwordFD < 0 && !term.IsTerminal(int(os.Stdin.Fd())) {
				return nil
			}
			if e := unlockCLI(ctx, w, f.passwordFD, false); e != nil {
				return e
			}
			w.Cloud.Restrict([]string{id})
			return waitUploads(ctx, w, []string{id}, f.json)
		case "cleanup":
			if !f.yes {
				return errors.New("multipart cleanup requires --yes")
			}
			if e := unlockCLI(ctx, w, f.passwordFD, false); e != nil {
				return e
			}
			return w.Cloud.Cleanup(ctx, id)
		}
	case "upload":
		if len(pos) < 2 || f.profile == "" {
			return &usageError{"storage upload ASSET_ID --profile ID[,ID]"}
		}
		if e := unlockCLI(ctx, w, f.passwordFD, false); e != nil {
			return e
		}
		jobs, e := w.Cloud.Enqueue(ctx, pos[1], strings.Split(f.profile, ","))
		if e != nil {
			return e
		}
		ids := []string{}
		for _, j := range jobs {
			ids = append(ids, j.ID)
		}
		w.Cloud.Restrict(ids)
		return waitUploads(ctx, w, ids, f.json)
	}
	return &usageError{"unknown storage action"}
}
func profilesCLI(ctx context.Context, w *application.Workspace, pos []string, f catalogFlags) error {
	if len(pos) == 0 {
		return &usageError{"profiles list/add/edit/test/delete required"}
	}
	if pos[0] == "list" {
		p, e := w.Cloud.Profiles(ctx)
		if e != nil {
			return e
		}
		return emitJSON(p)
	}
	if pos[0] == "delete" {
		if len(pos) < 2 || !f.yes {
			return errors.New("profile deletion requires ID and --yes")
		}
		return w.Cloud.DeleteProfile(ctx, pos[1])
	}
	if e := unlockCLI(ctx, w, f.passwordFD, pos[0] == "add"); e != nil {
		return e
	}
	if pos[0] == "test" || pos[0] == "test-write" {
		if len(pos) < 2 {
			return &usageError{"profile ID required"}
		}
		if pos[0] == "test-write" && !f.yes {
			return errors.New("write test creates/deletes an object and requires --yes")
		}
		return w.Cloud.Test(ctx, pos[1], pos[0] == "test-write")
	}
	if pos[0] != "add" && pos[0] != "edit" && pos[0] != "keys" {
		return &usageError{"unknown profile action"}
	}
	p := core.S3Profile{Enabled: true, Style: "auto", Region: "auto"}
	if pos[0] != "add" {
		if len(pos) < 2 {
			return &usageError{"profile ID required"}
		}
		profiles, e := w.Cloud.Profiles(ctx)
		if e != nil {
			return e
		}
		found := false
		for _, item := range profiles {
			if item.ID == pos[1] {
				p = item
				found = true
			}
		}
		if !found {
			return errors.New("profile not found")
		}
	}
	if f.present["name"] {
		p.Name = f.name
	}
	if f.present["endpoint"] {
		p.Endpoint = f.endpoint
	}
	if f.present["region"] {
		p.Region = f.region
	}
	if f.present["bucket"] {
		p.Bucket = f.bucket
	}
	if f.present["prefix"] {
		p.Prefix = f.prefix
	}
	if f.present["style"] {
		p.Style = f.style
	}
	if f.loopback {
		p.AllowLoopbackHTTP = true
	}
	if f.present["auto"] {
		if f.auto && !f.yes {
			return errors.New("enabling auto upload requires --yes")
		}
		p.AutoUpload = f.auto
	}
	if f.present["enabled"] {
		v, e := strconv.ParseBool(f.enabled)
		if e != nil {
			return e
		}
		p.Enabled = v
	}
	if f.present["provider"] {
		p.Provider = f.provider
	}
	if pos[0] == "add" && term.IsTerminal(int(os.Stdin.Fd())) && (p.Name == "" || p.Bucket == "") {
		var e error
		for _, field := range []struct {
			label string
			value *string
		}{{"Name", &p.Name}, {"Endpoint (empty: AWS)", &p.Endpoint}, {"Region", &p.Region}, {"Bucket", &p.Bucket}, {"Prefix", &p.Prefix}, {"Style: auto/path_style/virtual_host", &p.Style}} {
			*field.value, e = ttyLine(field.label, *field.value)
			if e != nil {
				return e
			}
		}
		if strings.HasPrefix(p.Endpoint, "http://") {
			answer, e := ttyLine("HTTP exposes credentials; allow only a local loopback test server? yes/no", "no")
			if e != nil {
				return e
			}
			p.AllowLoopbackHTTP = answer == "yes"
		}
	}
	if e := core.ValidateProfile(p); e != nil {
		return e
	}
	var c *core.Credentials
	if pos[0] == "add" || pos[0] == "keys" {
		credentials, e := readCredentialInput(f.credentialsFD)
		if e != nil {
			return e
		}
		c = &credentials
	}
	if e := w.Cloud.SaveProfile(ctx, p, c); e != nil {
		return e
	}
	fmt.Println(tr("Профиль сохранён только на этом компьютере"))
	return nil
}
func waitUploads(ctx context.Context, w *application.Workspace, ids []string, asJSON bool) error {
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		jobs := []core.UploadJob{}
		for id := range wanted {
			j, e := w.Cloud.Get(ctx, id)
			if e != nil {
				return e
			}
			jobs = append(jobs, j)
		}
		finished := true
		selected := []core.UploadJob{}
		fail := false
		for _, j := range jobs {
			if !wanted[j.ID] {
				continue
			}
			selected = append(selected, j)
			if j.State != "done" {
				if j.Terminal() || j.State.NeedsAttention() {
					fail = true
				} else {
					finished = false
				}
			}
		}
		if finished {
			if asJSON {
				_ = emitJSON(selected)
			} else {
				for _, j := range selected {
					fmt.Printf("%s %s %s\n", j.ID, j.State, j.Verification)
				}
			}
			if fail {
				return errors.New("one or more S3 uploads failed; see storage uploads list")
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return core.ErrCanceled
		case <-ticker.C:
		}
	}
}

func ttyLine(label, fallback string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s [%s]: ", label, safeText(fallback))
	var b strings.Builder
	one := make([]byte, 1)
	for b.Len() < 4096 {
		n, e := os.Stdin.Read(one)
		if e != nil {
			return "", e
		}
		if n == 0 {
			continue
		}
		if one[0] == '\n' {
			text := strings.TrimSpace(b.String())
			if text == "" {
				return fallback, nil
			}
			return text, nil
		}
		if one[0] != '\r' {
			b.WriteByte(one[0])
		}
	}
	return "", errors.New("input too long")
}

func printWorkspaceHelp() {
	fmt.Println(`
Workspace:
  camelclip playlists list|items|add|edit|pause|resume|sync|backfill|retry|watch
  camelclip playlists add URL --initial new_only|all_existing --interval 15
      [--name LABEL] [--quality 1080] [--out PATH] [--profile ID[,ID]]
  camelclip playlists watch [--password-fd N]  # continuous monitoring/downloads
  camelclip playlists sync ID                # one discovery scan, no transfers
  camelclip queue list --json
  camelclip queue priority JOB_ID low|normal|high
  camelclip queue reorder JOB_ID up|down|first|last
  camelclip queue pause|resume|retry|cancel JOB_ID
  camelclip library list|search|scan|import|refresh|show|open|reveal
  camelclip storage profiles list|add|edit|keys|test|test-write|delete
  camelclip storage upload ASSET_ID --profile ID[,ID] [--json]
  camelclip storage uploads list|retry|cancel|cleanup
  camelclip storage unlock
  camelclip storage reset --yes
Secret input: TTY prompts or --password-fd N / --credentials-fd N (N >= 3).
S3 uploads use a separate pool. Secrets stay encrypted in local SQLite.`)
}

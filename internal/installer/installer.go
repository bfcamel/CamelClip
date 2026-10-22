// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
// Package installer installs prebuilt components without touching application data.
package installer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

type File struct {
	Source   string `json:"source,omitempty"`
	Area     string `json:"area"`
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Mode     uint32 `json:"mode"`
	Template bool   `json:"template,omitempty"`
}
type Component struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Files   []File `json:"files"`
}
type Manifest struct {
	Schema     int         `json:"schema"`
	Version    string      `json:"version"`
	Commit     string      `json:"commit"`
	OS         string      `json:"os"`
	Arch       string      `json:"arch"`
	Components []Component `json:"components"`
}
type Layout struct {
	Prefix, Data, Apps, Desktop, Icons string
	System                             bool
}

func UserLayout(prefix string) (Layout, error) {
	home, e := os.UserHomeDir()
	if e != nil {
		return Layout{}, e
	}
	custom := prefix != ""
	if !custom {
		prefix = filepath.Join(home, ".local")
	}
	if !filepath.IsAbs(prefix) || filepath.Clean(prefix) == "/" || strings.ContainsAny(prefix, "\x00\r\n") {
		return Layout{}, errors.New("prefix must be an absolute directory other than /")
	}
	prefix, e = canonicalPrefix(filepath.Clean(prefix))
	if e != nil {
		return Layout{}, e
	}
	apps := filepath.Join(prefix, "Applications")
	desktop := filepath.Join(prefix, "share", "applications")
	if runtime.GOOS == "darwin" {
		desktop = apps
		if !custom {
			apps = filepath.Join(home, "Applications")
			desktop = apps
		}
	}
	return Layout{Prefix: prefix, Data: filepath.Join(prefix, "share", "CamelClip"), Apps: apps, Desktop: desktop, Icons: filepath.Join(prefix, "share", "pixmaps")}, nil
}

// Resolve an explicitly chosen root (including macOS /tmp -> /private/tmp),
// while refusing symlinks inside the installation once that root is established.
func canonicalPrefix(path string) (string, error) {
	suffix := []string{}
	current := path
	for {
		if _, e := os.Lstat(current); e == nil {
			base, e := filepath.EvalSymlinks(current)
			if e != nil {
				return "", e
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				base = filepath.Join(base, suffix[i])
			}
			return base, nil
		} else if !os.IsNotExist(e) {
			return "", e
		}
		suffix = append(suffix, filepath.Base(current))
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("cannot resolve install prefix")
		}
		current = parent
	}
}
func SystemLayout() (Layout, error) {
	if runtime.GOOS != "darwin" {
		return Layout{}, errors.New("system package management is currently available only on macOS; Linux setup is user-scoped")
	}
	return Layout{Prefix: "/usr/local", Data: "/Library/Application Support/CamelClip", Apps: "/Applications", Desktop: "/Applications", Icons: "/usr/local/share/pixmaps", System: true}, nil
}
func (l Layout) Root(area string) string {
	switch area {
	case "bin":
		return filepath.Join(l.Prefix, "bin")
	case "data":
		return l.Data
	case "apps":
		return l.Apps
	case "desktop":
		return l.Desktop
	case "icons":
		return l.Icons
	}
	return ""
}
func safeRelative(path string) bool {
	return path != "" && !filepath.IsAbs(path) && filepath.Clean(path) == path && path != "." && path != ".." && !strings.HasPrefix(path, ".."+string(filepath.Separator)) && !strings.ContainsAny(path, "\x00\r\n")
}
func (l Layout) Destination(f File) (string, error) {
	root := l.Root(f.Area)
	if root == "" || !safeRelative(f.Path) {
		return "", fmt.Errorf("invalid install destination %s/%s", f.Area, f.Path)
	}
	if f.Area == "data" && (f.Path == "installation" || strings.HasPrefix(f.Path, "installation/")) {
		return "", errors.New("payload cannot own installation metadata")
	}
	return filepath.Join(root, f.Path), nil
}
func noSymlinks(path string) error {
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink in installation path: %s", p)
		}
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return nil
}
func parse(path string, v any) error {
	data, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if e = d.Decode(v); e != nil {
		return e
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}
func Load(source string) (Manifest, error) {
	var m Manifest
	e := parse(filepath.Join(source, "manifest.json"), &m)
	if e != nil {
		return m, e
	}
	if m.Schema != 1 || m.Version == "" || m.OS != runtime.GOOS || m.Arch != runtime.GOARCH {
		return m, fmt.Errorf("unsupported release: schema=%d %s/%s, running %s/%s", m.Schema, m.OS, m.Arch, runtime.GOOS, runtime.GOARCH)
	}
	seen := map[string]bool{}
	for _, c := range m.Components {
		if !validID(c.ID) || seen[c.ID] || c.Version != m.Version {
			return m, errors.New("invalid component manifest")
		}
		seen[c.ID] = true
	}
	for _, id := range []string{"manager", "terminal", "gui", "tui", "cli"} {
		if !seen[id] {
			return m, fmt.Errorf("missing component %s", id)
		}
	}
	return m, nil
}
func validID(id string) bool {
	return id == "manager" || id == "terminal" || id == "gui" || id == "tui" || id == "cli"
}
func Choices(text string) ([]string, error) {
	seen := map[string]bool{}
	for _, id := range strings.Split(text, ",") {
		id = strings.TrimSpace(id)
		if id != "gui" && id != "tui" && id != "cli" {
			return nil, fmt.Errorf("unknown component %q; available: gui,tui,cli (server is not released)", id)
		}
		seen[id] = true
	}
	return sortedKeys(seen), nil
}
func sortedKeys(m map[string]bool) []string {
	keys := []string{}
	for k, v := range m {
		if v {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}
func ReadInstalled(l Layout) (map[string]Component, error) {
	out := map[string]Component{}
	dir := filepath.Join(l.Data, "installation", "components")
	entries, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return out, nil
	}
	if e != nil {
		return nil, e
	}
	if e = noSymlinks(dir); e != nil {
		return nil, e
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, errors.New("unexpected installation metadata; inspect before continuing")
		}
		var c Component
		if e = parse(filepath.Join(dir, entry.Name()), &c); e != nil {
			return nil, e
		}
		if !validID(c.ID) || entry.Name() != c.ID+".json" {
			return nil, errors.New("invalid installed component record")
		}
		for _, f := range c.Files {
			if _, e = l.Destination(f); e != nil {
				return nil, e
			}
		}
		out[c.ID] = c
	}
	return out, nil
}
func Visible(installed map[string]Component) []string {
	ids := []string{}
	for _, id := range []string{"gui", "tui", "cli"} {
		if _, ok := installed[id]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}
func expand(ids []string) map[string]bool {
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = true
	}
	if len(ids) > 0 {
		out["manager"] = true
	}
	if out["tui"] || out["cli"] {
		out["terminal"] = true
	}
	return out
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func readRegular(path string) ([]byte, error) {
	if e := noSymlinks(path); e != nil {
		return nil, e
	}
	info, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", path)
	}
	return os.ReadFile(path)
}
func componentPath(l Layout, id string) string {
	return filepath.Join(l.Data, "installation", "components", id+".json")
}
func lock(path string) (func(), error) {
	if e := noSymlinks(path); e != nil {
		return nil, e
	}
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New("another setup or CamelClip session owns the lock; close it first")
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}

// LockWorkspace opens only an existing lock and never initializes or modifies a queue.
func LockWorkspace(configDir string) (func(), error) {
	resolved, e := filepath.EvalSymlinks(configDir)
	if os.IsNotExist(e) {
		return func() {}, nil
	}
	if e != nil {
		return nil, e
	}
	path := filepath.Join(resolved, "queue.lock")
	if _, e := os.Stat(path); os.IsNotExist(e) {
		return func() {}, nil
	} else if e != nil {
		return nil, e
	}
	return lock(path)
}

type change struct {
	path   string
	data   []byte
	mode   os.FileMode
	remove bool
}
type previous struct {
	path    string
	data    []byte
	mode    os.FileMode
	existed bool
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if e := noSymlinks(path); e != nil {
		return e
	}
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".camelclip-setup-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}

// transact backs up before changing any file and rolls back if a write fails.
// Individual file publication is atomic; a multi-file install is not crash-atomic.
func transact(changes []change, failAt int) error {
	old := make([]previous, 0, len(changes))
	for _, c := range changes {
		if e := noSymlinks(c.path); e != nil {
			return e
		}
		p := previous{path: c.path}
		data, e := readRegular(c.path)
		if e == nil {
			info, _ := os.Stat(c.path)
			p.data = data
			p.mode = info.Mode().Perm()
			p.existed = true
		} else if !os.IsNotExist(e) {
			return e
		}
		old = append(old, p)
	}
	applied := 0
	var err error
	for i, c := range changes {
		if failAt > 0 && i+1 == failAt {
			err = errors.New("injected install failure")
		} else if c.remove {
			err = os.Remove(c.path)
			if os.IsNotExist(err) {
				err = nil
			}
		} else {
			err = atomicWrite(c.path, c.data, c.mode)
		}
		if err != nil {
			break
		}
		applied++
	}
	if err != nil {
		var restore []error
		for i := applied - 1; i >= 0; i-- {
			p := old[i]
			if p.existed {
				restore = append(restore, atomicWrite(p.path, p.data, p.mode))
			} else {
				e := os.Remove(p.path)
				if !os.IsNotExist(e) {
					restore = append(restore, e)
				}
			}
		}
		return errors.Join(append([]error{err}, restore...)...)
	}
	return nil
}

type Options struct {
	Source     string
	Components []string
	Remove     []string
	Uninstall  bool
	ConfigDir  string
	failAt     int
}

func Apply(l Layout, o Options) error {
	if l.System {
		for _, name := range []string{"camelclip", "camelclip-gui", "CamelClipWindow"} {
			err := exec.Command("/usr/bin/pgrep", "-x", name).Run()
			if err == nil {
				return errors.New("close all CamelClip sessions before system removal")
			}
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				return fmt.Errorf("cannot check running sessions: %w", err)
			}
		}
	}

	if l.System && len(o.Components) > 0 {
		return errors.New("use the macOS .pkg to install/update system components; setup supports status and removal")
	}
	unlock, e := lock(filepath.Join(l.Data, "installation", "setup.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	release, e := LockWorkspace(o.ConfigDir)
	if e != nil {
		return e
	}
	defer release()
	installed, e := ReadInstalled(l)
	if e != nil {
		return e
	}
	wanted := map[string]bool{}
	for _, id := range Visible(installed) {
		wanted[id] = true
	}
	for _, id := range o.Components {
		if _, e = Choices(id); e != nil {
			return e
		}
		wanted[id] = true
	}
	for _, id := range o.Remove {
		if _, e = Choices(id); e != nil {
			return e
		}
		delete(wanted, id)
	}
	if o.Uninstall {
		wanted = map[string]bool{}
	}
	ids := sortedKeys(wanted)
	keep := expand(ids)
	owned := map[string]File{}
	for _, c := range installed {
		for _, f := range c.Files {
			path, e := l.Destination(f)
			if e != nil {
				return e
			}
			if _, ok := owned[path]; ok {
				return fmt.Errorf("multiple components own %s", path)
			}
			owned[path] = f
		}
	}
	// Verify ownership before overwriting or deleting anything, including modified files.
	for path, f := range owned {
		data, e := readRegular(path)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if digest(data) != f.SHA256 {
			return fmt.Errorf("installed file was modified; back it up and repair it first: %s", path)
		}
	}
	desired := map[string]Component{}
	newData := map[string][]byte{}
	if len(o.Components) > 0 {
		resolvedSource, sourceErr := filepath.EvalSymlinks(o.Source)
		if sourceErr != nil {
			return sourceErr
		}
		o.Source = resolvedSource
		m, e := Load(o.Source)
		if e != nil {
			return e
		}
		for _, c := range installed {
			if older(m.Version, c.Version) {
				return errors.New("downgrade refused; preserve your data and install a compatible newer release")
			}
		}

		// An upgrade preserves all installed choices and updates them as one release.
		for _, c := range m.Components {
			if !keep[c.ID] {
				continue
			}
			record := Component{ID: c.ID, Version: m.Version, Files: []File{}}
			for _, f := range c.Files {
				path, e := l.Destination(f)
				if e != nil {
					return e
				}
				if _, ok := newData[path]; ok {
					return fmt.Errorf("duplicate destination: %s", path)
				}
				if !safeRelative(f.Source) || f.Mode != 0644 && f.Mode != 0755 || f.Size < 0 {
					return errors.New("invalid payload entry")
				}
				data, e := readRegular(filepath.Join(o.Source, f.Source))
				if e != nil {
					return e
				}
				if int64(len(data)) != f.Size || digest(data) != f.SHA256 {
					return fmt.Errorf("payload checksum mismatch: %s", f.Source)
				}
				if f.Template {
					data = bytes.ReplaceAll(data, []byte("@DATA_SHELL@"), []byte(shellQuote(l.Data)))
					data = bytes.ReplaceAll(data, []byte("@PREFIX_SHELL@"), []byte(shellQuote(l.Prefix)))
					data = bytes.ReplaceAll(data, []byte("@BIN_DESKTOP@"), []byte(desktopQuote(filepath.Join(l.Prefix, "bin"))))
				}
				if _, ours := owned[path]; !ours {
					if _, e = os.Lstat(path); e == nil {
						return fmt.Errorf("refusing to replace an unowned path: %s", path)
					} else if !os.IsNotExist(e) {
						return e
					}
				}
				f.Source = ""
				f.Template = false
				f.Size = int64(len(data))
				f.SHA256 = digest(data)
				record.Files = append(record.Files, f)
				newData[path] = data
			}
			desired[c.ID] = record
		}
		for id := range keep {
			if _, ok := desired[id]; !ok {
				return fmt.Errorf("release is missing selected component %s", id)
			}
		}
	} else {
		for id, c := range installed {
			if keep[id] {
				desired[id] = c
			}
		}
	}
	changes := []change{}
	desiredPaths := map[string]bool{}
	for _, c := range desired {
		for _, f := range c.Files {
			path, _ := l.Destination(f)
			desiredPaths[path] = true
			if data, ok := newData[path]; ok {
				changes = append(changes, change{path: path, data: data, mode: os.FileMode(f.Mode)})
			}
		}
	}
	for path := range owned {
		if !desiredPaths[path] {
			changes = append(changes, change{path: path, remove: true})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].path < changes[j].path })
	// Inventory commits last; preferences and media are never part of this transaction.
	for _, id := range []string{"manager", "terminal", "gui", "tui", "cli"} {
		c, ok := desired[id]
		if ok {
			data, _ := json.MarshalIndent(c, "", "  ")
			changes = append(changes, change{path: componentPath(l, id), data: append(data, '\n'), mode: 0644})
		} else if _, exists := installed[id]; exists {
			changes = append(changes, change{path: componentPath(l, id), remove: true})
		}
	}
	if e = transact(changes, o.failAt); e != nil {
		return e
	}
	// Remove empty product directories only; never recursively delete user-created files.
	for path := range owned {
		if desiredPaths[path] {
			continue
		}
		for p := filepath.Dir(path); p != filepath.Dir(p); p = filepath.Dir(p) {
			if p == l.Prefix || p == l.Apps || p == l.Desktop || p == l.Icons || p == l.Data {
				break
			}
			if os.Remove(p) != nil {
				break
			}
		}
	}
	return nil
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func desktopQuote(s string) string {
	s = strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "`", "\\`", "$", "\\$", "%", "%%").Replace(s)
	return s
}

func older(a, b string) bool {
	av, bv := strings.Split(a, "."), strings.Split(b, ".")
	if len(av) != 3 || len(bv) != 3 {
		return a != b
	}
	for i := 0; i < 3; i++ {
		x, ex := strconv.Atoi(av[i])
		y, ey := strconv.Atoi(bv[i])
		if ex != nil || ey != nil {
			return a != b
		}
		if x != y {
			return x < y
		}
	}
	return false
}

// PreflightRecord validates package-owned paths before the system Installer writes.
// The package includes its incoming record and this same setup executable.
func PreflightRecord(l Layout, path string) error {
	var incoming Component
	if e := parse(path, &incoming); e != nil {
		return e
	}
	if !validID(incoming.ID) {
		return errors.New("invalid package component")
	}
	installed, e := ReadInstalled(l)
	if e != nil {
		return e
	}
	owned := map[string]File{}
	for _, c := range installed {
		if older(incoming.Version, c.Version) {
			return errors.New("package downgrade refused")
		}
		for _, f := range c.Files {
			p, e := l.Destination(f)
			if e != nil {
				return e
			}
			owned[p] = f
		}
	}
	for _, f := range incoming.Files {
		p, e := l.Destination(f)
		if e != nil {
			return e
		}
		if e = noSymlinks(p); e != nil {
			return e
		}
		data, e := readRegular(p)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		prior, ok := owned[p]
		if !ok {
			return fmt.Errorf("refusing to overwrite an unowned path: %s", p)
		}
		if digest(data) != prior.SHA256 {
			return fmt.Errorf("installed file was modified: %s", p)
		}
	}
	return nil
}

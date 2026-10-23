// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package clips

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
)

type Files struct{ Root string }

var safeID = regexp.MustCompile(`^[a-zA-Z0-9-]{1,80}$`)

func (f Files) Paths(ctx context.Context, batch, task, out, ext, title string) (string, string, string, error) {
	if !safeID.MatchString(batch) || !safeID.MatchString(task) {
		return "", "", "", errors.New("invalid owned clip reference")
	}
	work := filepath.Join(f.Root, batch)
	if e := os.MkdirAll(work, 0700); e != nil {
		return "", "", "", e
	}
	if task == "source" {
		return work, "", "", nil
	}
	if !filepath.IsAbs(out) || !safeID.MatchString(ext) {
		return "", "", "", errors.New("invalid clip output location")
	}
	if e := os.MkdirAll(out, 0755); e != nil {
		return "", "", "", e
	}
	staging := filepath.Join(out, ".camelclip-clips", batch)
	if e := ownedDirs(out, staging); e != nil {
		return "", "", "", e
	}
	if e := os.MkdirAll(staging, 0700); e != nil {
		return "", "", "", e
	}
	return work, filepath.Join(staging, task+"."+ext), filepath.Join(out, safeTitle(title)+" ["+task+"]."+ext), nil
}
func hash(ctx context.Context, path string) (string, int64, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", 0, e
	}
	defer f.Close()
	before, e := f.Stat()
	if e != nil || !before.Mode().IsRegular() {
		return "", 0, errors.New("clip source is not a regular file")
	}
	h := sha256.New()
	buf := make([]byte, 128*1024)
	var size int64
	for {
		if e = ctx.Err(); e != nil {
			return "", 0, e
		}
		n, err := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
			size += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", 0, err
		}
	}
	after, e := f.Stat()
	if e != nil || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", 0, &core.Failure{Code: core.CodeSourceChanged, Message: "source changed while hashing"}
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}
func (f Files) Validate(ctx context.Context, path, digest string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return errors.New("expected a regular owned media file")
	}
	if digest == "" {
		return errors.New("source has no content evidence")
	}
	actual, _, e := hash(ctx, path)
	if e == nil && actual != digest {
		return &core.Failure{Code: core.CodeSourceChanged, Message: "clip content changed"}
	}
	return e
}
func (f Files) Publish(ctx context.Context, candidate, final string, ev core.ClipEvidence) error {
	// The matching durable intent is checked by the application before this call.
	if e := f.Validate(ctx, final, ev.SHA256); e == nil {
		a, ae := os.Stat(candidate)
		z, ze := os.Stat(final)
		if ae != nil || ze != nil || !os.SameFile(a, z) {
			return &core.Failure{Code: core.CodeConflict, Message: "existing output does not belong to this publication intent"}
		}
		return nil
	} else if !errors.Is(e, os.ErrNotExist) {
		return &core.Failure{Code: core.CodeConflict, Message: "clip output path is occupied or changed"}
	}
	if e := f.Validate(ctx, candidate, ev.SHA256); e != nil {
		return e
	}
	file, e := os.OpenFile(candidate, os.O_RDWR, 0)
	if e != nil {
		return e
	}
	e = file.Sync()
	file.Close()
	if e != nil {
		return e
	}
	if e = os.Link(candidate, final); e != nil {
		if errors.Is(e, os.ErrExist) {
			return f.Validate(ctx, final, ev.SHA256)
		}
		return e
	}
	if d, e := os.Open(filepath.Dir(final)); e == nil {
		e = d.Sync()
		d.Close()
		if e != nil {
			return e
		}
	}
	return nil
}
func (f Files) Metadata(ctx context.Context, path string) (core.Asset, error) {
	s, e := os.Stat(path)
	if e != nil {
		return core.Asset{}, e
	}
	return core.Asset{Size: s.Size(), Modified: s.ModTime()}, ctx.Err()
}
func (f Files) Cleanup(ctx context.Context, id string) error {
	if !safeID.MatchString(id) {
		return errors.New("invalid cache reference")
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	return os.RemoveAll(filepath.Join(f.Root, id))
}
func checkSpace(path string) error {
	var s syscall.Statfs_t
	if e := syscall.Statfs(path, &s); e != nil {
		return e
	}
	if uint64(s.Bavail)*uint64(s.Bsize) < 64*1024*1024 {
		return fmt.Errorf("clip staging needs at least 64 MiB free space")
	}
	return nil
}
func copySource(ctx context.Context, source, target string) error {
	in, e := os.Open(source)
	if e != nil {
		return e
	}
	defer in.Close()
	st, e := in.Stat()
	if e != nil || !st.Mode().IsRegular() {
		return errors.New("local source must be a regular file")
	}
	out, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer out.Close()
	buf := make([]byte, 128*1024)
	for {
		if e = ctx.Err(); e != nil {
			return e
		}
		n, err := in.Read(buf)
		if n > 0 {
			if _, e = out.Write(buf[:n]); e != nil {
				return e
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	now, e := in.Stat()
	if e != nil || now.Size() != st.Size() || !now.ModTime().Equal(st.ModTime()) {
		return &core.Failure{Code: core.CodeSourceChanged, Message: "local source changed during preparation"}
	}
	return out.Sync()
}

type lockKey struct{}

func (f Files) Acquire(ctx context.Context, id string) (context.Context, func(), error) {
	if !safeID.MatchString(id) {
		return ctx, nil, errors.New("invalid cache identity")
	}
	dir := filepath.Join(f.Root, id)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return ctx, nil, e
	}
	file, e := os.OpenFile(filepath.Join(dir, "stage.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return ctx, nil, e
	}
	for {
		e = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if e == nil {
			break
		}
		if e != syscall.EWOULDBLOCK && e != syscall.EAGAIN {
			file.Close()
			return ctx, nil, e
		}
		select {
		case <-ctx.Done():
			file.Close()
			return ctx, nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return context.WithValue(ctx, lockKey{}, file), func() { syscall.Flock(int(file.Fd()), syscall.LOCK_UN); file.Close() }, nil
}
func ownedDirs(out, staging string) error {
	rel, e := filepath.Rel(out, staging)
	if e != nil {
		return e
	}
	p := out
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		p = filepath.Join(p, part)
		if e = os.Mkdir(p, 0700); e != nil && !errors.Is(e, os.ErrExist) {
			return e
		}
		s, e := os.Lstat(p)
		if e != nil || !s.IsDir() || s.Mode()&os.ModeSymlink != 0 {
			return errors.New("clip staging must be a real owned directory")
		}
	}
	return nil
}
func (f Files) CleanupCandidate(ctx context.Context, path string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if !strings.Contains(path, string(filepath.Separator)+".camelclip-clips"+string(filepath.Separator)) {
		return errors.New("candidate is outside private staging")
	}
	e := os.Remove(path)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e == nil {
		_ = os.Remove(filepath.Dir(path))
		_ = os.Remove(filepath.Dir(filepath.Dir(path)))
	}
	return e
}

func safeTitle(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune("/\\:*?\"<>|", r) {
			return '_'
		}
		return r
	}, s)
	s = strings.Trim(s, " .")
	if s == "" {
		s = "Clip"
	}
	for len(s) > 120 {
		_, n := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-n]
	}
	return s
}

// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"errors"
	"fmt"
	"github.com/bfcamel/video-downloader/internal/core"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type LibraryService struct {
	repo             AssetRepository
	probe            core.MediaProber
	slots            chan struct{}
	mu               sync.Mutex
	scanning         bool
	report           core.ScanReport
	cancel           context.CancelFunc
	enrichmentCancel context.CancelFunc
	wg               sync.WaitGroup
}

func NewLibrary(repo AssetRepository, probe core.MediaProber) *LibraryService {
	return &LibraryService{repo: repo, probe: probe, slots: make(chan struct{}, 2)}
}
func (l *LibraryService) List(ctx context.Context, q core.AssetQuery) (core.AssetPage, error) {
	if q.Limit == 0 {
		q.Limit = 50
	}
	return l.repo.Assets(ctx, q)
}
func (l *LibraryService) Get(ctx context.Context, id string) (core.Asset, error) {
	return l.repo.Asset(ctx, id)
}
func mediaPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".mkv", ".webm", ".mov", ".m4v", ".mp3", ".m4a", ".opus", ".flac", ".wav", ".ogg", ".aac", ".mka":
		return true
	}
	return false
}
func (l *LibraryService) Refresh(ctx context.Context, id string) (core.Asset, error) {
	old, e := l.repo.Asset(ctx, id)
	if e != nil {
		return old, e
	}
	a, e := l.inspect(ctx, old.Path)
	if e != nil {
		if os.IsNotExist(e) {
			old.Status = "missing"
			_, _ = l.repo.PutAsset(ctx, old)
		}
		return old, e
	}
	a.ID = old.ID
	a.Added = old.Added
	a.SourceURL = old.SourceURL
	a.DownloadID = old.DownloadID
	if a.Size == old.Size && a.Modified.Equal(old.Modified) {
		a.LocalVerification = old.LocalVerification
	}
	return l.repo.PutAsset(ctx, a)
}
func (l *LibraryService) inspect(ctx context.Context, path string) (core.Asset, error) {
	select {
	case l.slots <- struct{}{}:
	case <-ctx.Done():
		return core.Asset{}, ctx.Err()
	}
	defer func() { <-l.slots }()
	return l.probe.Probe(ctx, path)
}
func (l *LibraryService) Import(ctx context.Context, path string) (core.Asset, error) {
	path, e := filepath.Abs(path)
	if e != nil {
		return core.Asset{}, e
	}
	if !mediaPath(path) {
		return core.Asset{}, errors.New("unsupported media extension")
	}
	a, e := l.inspect(ctx, path)
	if e != nil {
		return a, e
	}
	if e = l.repo.AllowAsset(ctx, path); e != nil {
		return a, e
	}
	return l.repo.PutAsset(ctx, a)
}
func (l *LibraryService) Forget(ctx context.Context, id string) error {
	return l.repo.ForgetAsset(ctx, id)
}
func (l *LibraryService) DeleteFile(ctx context.Context, id string, confirmed bool) error {
	if !confirmed {
		return errors.New("explicit file deletion confirmation required")
	}
	a, e := l.repo.Asset(ctx, id)
	if e != nil {
		return e
	}
	st, e := os.Lstat(a.Path)
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() || st.Size() != a.Size || !st.ModTime().Equal(a.Modified) {
		return errors.New("file changed; refresh and confirm again")
	}
	if e = l.repo.CanDeleteAsset(ctx, id); e != nil {
		return e
	}
	if e = os.Remove(a.Path); e != nil {
		return fmt.Errorf("file was not deleted: %w", e)
	}
	a.Status = "missing"
	_, e = l.repo.PutAsset(ctx, a)
	return e
}
func (l *LibraryService) Scan(ctx context.Context, root string, recursive bool) (core.ScanReport, error) {
	report := core.ScanReport{}
	started := time.Now()
	root, e := filepath.Abs(root)
	if e != nil {
		return report, e
	}
	st, e := os.Lstat(root)
	if e != nil {
		return report, e
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return report, errors.New("scan root must be a real directory")
	}
	seen := []string{}
	count := 0
	e = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if len(report.Errors) < 100 {
				report.Errors = append(report.Errors, "Cannot read directory entry")
			}
			return nil
		}
		if path == root {
			return nil
		}
		count++
		if count > 100000 {
			return errors.New("scan entry limit reached")
		}
		if d.IsDir() {
			if !recursive || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || strings.HasPrefix(d.Name(), ".") || !mediaPath(path) {
			report.Skipped++
			return nil
		}
		seen = append(seen, path)
		a, e := l.inspect(ctx, path)
		if e != nil {
			report.Skipped++
			if len(report.Errors) < 100 {
				report.Errors = append(report.Errors, filepath.Base(path)+": metadata unavailable")
			}
			return nil
		}
		stored, saveErr := l.repo.PutAsset(ctx, a)
		e = saveErr
		if e != nil {
			if e.Error() == "file excluded from index" {
				report.Skipped++
				return nil
			}
			return e
		}
		if stored.Added.Before(started) {
			report.Updated++
		} else {
			report.Added++
		}
		return nil
	})
	if e != nil {
		return report, e
	}
	report.Missing, e = l.repo.MarkMissing(ctx, root, seen, recursive)
	return report, e
}
func (l *LibraryService) StartScan(parent context.Context, root string, recursive bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.scanning {
		return errors.New("library scan is already running")
	}
	ctx, cancel := context.WithCancel(parent)
	l.cancel = cancel
	l.scanning = true
	l.report = core.ScanReport{}
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		r, e := l.Scan(ctx, root, recursive)
		if e != nil {
			r.Errors = append(r.Errors, e.Error())
		}
		l.mu.Lock()
		l.report = r
		l.scanning = false
		l.mu.Unlock()
	}()
	return nil
}
func (l *LibraryService) ScanStatus() (bool, core.ScanReport) {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := l.report
	r.Errors = append([]string(nil), r.Errors...)
	return l.scanning, r
}
func (l *LibraryService) Close() {
	l.mu.Lock()
	if l.enrichmentCancel != nil {
		l.enrichmentCancel()
	}
	if l.cancel != nil {
		l.cancel()
	}
	l.mu.Unlock()
	l.wg.Wait()
}

func (l *LibraryService) Enrich(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	l.mu.Lock()
	l.enrichmentCancel = cancel
	l.mu.Unlock()
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				page, e := l.repo.Assets(ctx, core.AssetQuery{NeedsProbe: true, Limit: 2})
				if e != nil {
					continue
				}
				for _, a := range page.Items {
					if _, e = l.Refresh(ctx, a.ID); e != nil {
						a.NeedsProbe = false
						_, _ = l.repo.PutAsset(ctx, a)
					}
				}
			}
		}
	}()
}

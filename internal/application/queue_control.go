// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type QueuePolicy struct {
	RetryAttempts int   `json:"retry_attempts"`
	PendingFiles  int   `json:"max_staging_files"`
	PendingBytes  int64 `json:"max_staging_bytes"`
	MinFreeBytes  int64 `json:"min_free_bytes"`
}
type JobControl struct {
	DiskBlocked bool      `json:"-"`
	Priority    int       `json:"priority"` // -1 low, 0 normal, 1 high
	Position    int64     `json:"position"`
	Paused      bool      `json:"paused"`
	NextAttempt time.Time `json:"next_attempt,omitempty"`
}
type ActionResult struct {
	ID    int    `json:"id"`
	Error string `json:"error,omitempty"`
}
type JobEvent struct {
	At      time.Time `json:"at"`
	State   State     `json:"state"`
	Message string    `json:"message,omitempty"`
}

func (s *Service) SetPolicy(p QueuePolicy) error {
	if p.RetryAttempts < 0 || p.RetryAttempts > 10 || p.PendingFiles < 0 || p.PendingBytes < 0 || p.MinFreeBytes < 0 {
		return errors.New("invalid queue policy")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policy = p
	s.scheduleLocked()
	return nil
}
func (s *Service) SetPriority(id, priority int) error {
	if priority < -1 || priority > 1 {
		return errors.New("priority must be low, normal or high")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.availableLocked(); err != nil {
		return err
	}
	i, err := s.findLocked(id)
	if err != nil {
		return err
	}
	s.jobs[i].Control.Priority = priority
	s.jobs[i].Control.Position = time.Now().UnixNano()
	s.syncOrderLocked()
	return s.saveLocked()
}
func (s *Service) Reorder(id int, move string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.availableLocked(); err != nil {
		return err
	}
	i, err := s.findLocked(id)
	if err != nil {
		return err
	}
	order := s.orderedLocked()
	indices := []int{}
	for _, v := range order {
		if s.jobs[v].Control.Priority == s.jobs[i].Control.Priority {
			indices = append(indices, v)
		}
	}
	at := 0
	for n, v := range indices {
		if v == i {
			at = n
		}
	}
	target := at
	switch move {
	case "up":
		target = max(0, at-1)
	case "down":
		target = min(len(indices)-1, at+1)
	case "first":
		target = 0
	case "last":
		target = len(indices) - 1
	default:
		return errors.New("move: up/down/first/last")
	}
	v := indices[at]
	indices = append(indices[:at], indices[at+1:]...)
	indices = append(indices, 0)
	copy(indices[target+1:], indices[target:])
	indices[target] = v
	for n, v := range indices {
		s.jobs[v].Control.Position = int64(n + 1)
	}
	s.syncOrderLocked()
	return s.saveLocked()
}
func (s *Service) PausePending(id int, paused bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.availableLocked(); err != nil {
		return err
	}
	i, err := s.findLocked(id)
	if err != nil {
		return err
	}
	j := &s.jobs[i]
	if j.State.Terminal() || containsState(j.State, Inspecting, Downloading, Processing, Verifying, Canceling) {
		return errors.New("only pending jobs can be paused; cancel an active transfer instead")
	}
	if paused && s.resources.running(id) {
		return errors.New("job is already starting; cancel it instead")
	}
	j.Control.Paused = paused
	j.Monitoring.UpdatedAt = time.Now()
	s.syncOrderLocked()
	if err = s.saveLocked(); err != nil {
		return err
	}
	s.scheduleLocked()
	return nil
}
func containsState(s State, values ...State) bool {
	for _, v := range values {
		if v == s {
			return true
		}
	}
	return false
}
func (s *Service) BulkAction(ids []int, action string) []ActionResult {
	result := make([]ActionResult, 0, len(ids))
	seen := map[int]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		var err error
		switch action {
		case "pause":
			err = s.PausePending(id, true)
		case "resume":
			err = s.PausePending(id, false)
		case "cancel":
			err = s.Cancel(id)
		case "retry":
			err = s.Retry(id, "")
		case "remove":
			err = s.Remove(id)
		case "low":
			err = s.SetPriority(id, -1)
		case "normal":
			err = s.SetPriority(id, 0)
		case "high":
			err = s.SetPriority(id, 1)
		default:
			err = errors.New("unknown bulk action")
		}
		r := ActionResult{ID: id}
		if err != nil {
			r.Error = err.Error()
		}
		result = append(result, r)
	}
	return result
}
func (s *Service) orderedLocked() []int {
	v := make([]int, len(s.jobs))
	for i := range v {
		v[i] = i
	}
	sort.SliceStable(v, func(a, b int) bool {
		x, y := s.jobs[v[a]], s.jobs[v[b]]
		if x.Control.Priority != y.Control.Priority {
			return x.Control.Priority > y.Control.Priority
		}
		return x.Control.Position < y.Control.Position
	})
	return v
}
func (s *Service) syncOrderLocked() {
	ranks := map[int]JobControl{}
	blocked := map[string]bool{}
	for _, j := range s.jobs {
		if _, ok := blocked[j.Request.Out]; !ok {
			blocked[j.Request.Out] = s.diskBlockedLocked(j.Request.Out)
		}
		control := j.Control
		control.DiskBlocked = blocked[j.Request.Out]
		ranks[j.ID] = control
	}
	s.resources.reorder(ranks)
}
func (s *Service) maintain() {
	defer s.wg.Done()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-tick.C:
			s.mu.Lock()
			paths := map[string]bool{}
			for _, j := range s.jobs {
				if !j.State.Terminal() {
					paths[j.Request.Out] = true
				}
			}
			s.mu.Unlock()
			sizes := map[string]int64{}
			for out := range paths {
				sizes[out] = stagingBytes(out)
			}
			s.mu.Lock()
			s.staged = sizes
			s.syncOrderLocked()
			changed := false
			for i := range s.jobs {
				j := &s.jobs[i]
				if containsState(j.State, WaitingDownload, WaitingDisk) && !s.resources.running(j.ID) {
					next := WaitingDownload
					if s.diskBlockedLocked(j.Request.Out) {
						next = WaitingDisk
					}
					if j.State != next {
						j.State = next
						changed = true
					}
				}
			}
			if changed {
				_ = s.saveLocked()
			}
			s.scheduleLocked()
			s.mu.Unlock()
		}
	}
}
func (s *Service) diskBlockedLocked(out string) bool {
	p := s.policy
	if p.PendingFiles == 0 && p.PendingBytes == 0 && p.MinFreeBytes == 0 {
		return false
	}
	if p.MinFreeBytes > 0 {
		var st syscall.Statfs_t
		if syscall.Statfs(out, &st) == nil && uint64(st.Bavail)*uint64(st.Bsize) < uint64(p.MinFreeBytes) {
			return true
		}
	}
	files, bytes := 0, int64(0)
	for _, j := range s.jobs {
		if j.Request.Out == out && containsState(j.State, WaitingProcessing, WaitingVerification, Processing, Verifying) {
			files++
			bytes += j.Monitoring.Download.DownloadedBytes
		}
	}
	bytes = max(bytes, s.staged[out])
	if p.PendingFiles > 0 && files >= p.PendingFiles || p.PendingBytes > 0 && bytes >= p.PendingBytes {
		return true
	}
	return false
}
func retryable(err error) bool {
	if err == nil {
		return false
	}
	t := strings.ToLower(err.Error())
	for _, v := range []string{"timeout", "timed out", "connection reset", "connection refused", "temporary", "429", "502", "503", "504", "unexpected eof"} {
		if strings.Contains(t, v) {
			return true
		}
	}
	return false
}
func retryDelay(attempt int) time.Duration {
	return time.Duration((1<<min(attempt, 8)))*time.Second + time.Duration(rand.IntN(1000))*time.Millisecond
}

// PendingBytes reconciles completed staging files when the downloader could not report a size.
func stagingBytes(out string) int64 {
	var total int64
	entries, _ := os.ReadDir(out)
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), ".clipnaro-") {
			continue
		}
		_ = filepath.WalkDir(filepath.Join(out, e.Name()), func(_ string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && d.Type().IsRegular() {
				if st, e := d.Info(); e == nil {
					total += st.Size()
				}
			}
			return nil
		})
	}
	return total
}

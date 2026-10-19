// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bfcamel/CamelClip/internal/core"
)

type CloudService struct {
	ctx              context.Context
	cancel           context.CancelFunc
	profiles         ProfileRepository
	uploads          UploadRepository
	assets           AssetRepository
	vault            core.CredentialVault
	factory          func() core.CredentialVault
	backend          core.Uploader
	mu               sync.Mutex
	options          core.UploadOptions
	active           map[string]context.CancelFunc
	progress         map[string]core.Metrics
	wg               sync.WaitGroup
	workers          sync.WaitGroup
	failure          error
	closing, locking bool
	allowed          map[string]bool
}

func NewCloud(parent context.Context, p ProfileRepository, u UploadRepository, a AssetRepository, vault core.CredentialVault, factory func() core.CredentialVault, backend core.Uploader) *CloudService {
	ctx, cancel := context.WithCancel(parent)
	s := &CloudService{ctx: ctx, cancel: cancel, profiles: p, uploads: u, assets: a, vault: vault, factory: factory, backend: backend, options: core.UploadOptions{Objects: 2, Parts: 2}, active: map[string]context.CancelFunc{}, progress: map[string]core.Metrics{}}
	s.failure = s.uploads.RecoverUploads(parent)
	s.wg.Add(1)
	go s.loop()
	return s
}
func (s *CloudService) Locked() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.vault.Locked() }
func (s *CloudService) Options() core.UploadOptions {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.options
}
func (s *CloudService) SetOptions(o core.UploadOptions) error {
	if o.Objects < 1 || o.Objects > 32 || o.Parts < 1 || o.Parts > 4 || o.RetryAttempts < 0 || o.RetryAttempts > 10 || o.RateBPS < 0 {
		return errors.New("invalid upload limits")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.options = o
	return nil
}
func (s *CloudService) Profiles(ctx context.Context) ([]core.S3Profile, error) {
	return s.profiles.Profiles(ctx)
}
func (s *CloudService) Unlock(ctx context.Context, password []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, e := s.profiles.VaultState(ctx)
	if e != nil {
		return errors.New("create a master password first")
	}
	return s.vault.Unlock(state, password)
}
func (s *CloudService) Initialize(ctx context.Context, password []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, e := s.profiles.VaultState(ctx); e == nil {
		return errors.New("vault already exists; unlock it")
	} else if !errors.Is(e, ErrVaultUnconfigured) {
		return errors.New("existing vault cannot be read; restore a backup or explicitly reset credentials")
	}
	v := s.factory()
	state, e := v.Initialize(password)
	if e != nil {
		return e
	}
	if e = s.profiles.SetVault(ctx, state, nil); e != nil {
		v.Lock()
		return e
	}
	s.vault.Lock()
	s.vault = v
	return nil
}
func (s *CloudService) Rotate(ctx context.Context, oldPassword, newPassword []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.active) > 0 {
		return errors.New("wait for active uploads before rotating password")
	}
	state, e := s.profiles.VaultState(ctx)
	if e != nil {
		return e
	}
	if e = s.vault.Unlock(state, oldPassword); e != nil {
		return e
	}
	profiles, e := s.profiles.Profiles(ctx)
	if e != nil {
		return e
	}
	candidate := s.factory()
	next, e := candidate.Initialize(newPassword)
	if e != nil {
		return e
	}
	encrypted := map[string]core.SealedSecret{}
	for _, p := range profiles {
		secret, e := s.profiles.Secret(ctx, p.ID)
		if e != nil {
			candidate.Lock()
			return e
		}
		plain, e := s.vault.Open(p.ID, secret)
		if e != nil {
			candidate.Lock()
			return e
		}
		sealed, e := candidate.Seal(p.ID, plain)
		if e != nil {
			candidate.Lock()
			return e
		}
		encrypted[p.ID] = sealed
	}
	if e = s.profiles.SetVault(ctx, next, encrypted); e != nil {
		candidate.Lock()
		return e
	}
	s.vault.Lock()
	s.vault = candidate
	return nil
}
func (s *CloudService) Lock() {
	s.mu.Lock()
	s.locking = true
	for _, cancel := range s.active {
		cancel()
	}
	s.mu.Unlock()
	s.workers.Wait()
	s.mu.Lock()
	s.vault.Lock()
	s.locking = false
	s.mu.Unlock()
}
func (s *CloudService) SaveProfile(ctx context.Context, p core.S3Profile, c *core.Credentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := core.ValidateProfile(p); e != nil {
		return e
	}
	if p.ID == "" {
		p.ID = core.NewID()
	}
	var sealed *core.SealedSecret
	if c != nil {
		if c.AccessKey == "" || c.SecretKey == "" {
			return errors.New("access key and secret key required")
		}
		v, e := s.vault.Seal(p.ID, *c)
		if e != nil {
			return e
		}
		sealed = &v
	}
	return s.profiles.SaveProfile(ctx, p, sealed)
}
func (s *CloudService) DeleteProfile(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profiles.DeleteProfile(ctx, id)
}
func (s *CloudService) Reset(ctx context.Context, confirmed bool) error {
	if !confirmed {
		return errors.New("explicit reset confirmation required")
	}
	s.Lock()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profiles.ResetProfiles(ctx)
}
func (s *CloudService) credentials(ctx context.Context, id string) (core.Credentials, error) {
	secret, e := s.profiles.Secret(ctx, id)
	if e != nil {
		return core.Credentials{}, errors.New("credentials unavailable")
	}
	return s.vault.Open(id, secret)
}
func (s *CloudService) Test(ctx context.Context, id string, write bool) error {
	s.mu.Lock()
	p, e := s.profiles.Profile(ctx, id)
	if e != nil {
		s.mu.Unlock()
		return e
	}
	c, e := s.credentials(ctx, id)
	s.mu.Unlock()
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if e = s.backend.Test(ctx, p, c, write); e != nil {
		return e
	}
	if write {
		s.mu.Lock()
		defer s.mu.Unlock()
		current, e := s.profiles.Profile(ctx, id)
		if e != nil {
			return e
		}
		if current.Revision != p.Revision {
			return errors.New("profile changed during test; repeat")
		}
		current.ConditionalVerified = true
		return s.profiles.SaveProfile(ctx, current, nil)
	}
	return nil
}
func (s *CloudService) Enqueue(ctx context.Context, assetID string, profileIDs []string) ([]core.UploadJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, e := s.assets.Asset(ctx, assetID)
	if e != nil {
		return nil, e
	}
	st, e := os.Lstat(a.Path)
	if e != nil || !st.Mode().IsRegular() || st.Size() != a.Size || !st.ModTime().Equal(a.Modified) {
		return nil, errors.New("source is missing or changed; refresh asset first")
	}
	profiles := []core.S3Profile{}
	for _, id := range profileIDs {
		p, e := s.profiles.Profile(ctx, id)
		if e != nil {
			return nil, e
		}
		if !p.Enabled {
			return nil, errors.New("profile disabled")
		}
		profiles = append(profiles, p)
	}
	jobs := []core.UploadJob{}
	for _, p := range profiles {
		j := core.UploadJob{ID: core.NewID(), AssetID: a.ID, ProfileID: p.ID, Profile: p, Key: core.DestinationKey(p, a), State: "queued", Created: time.Now(), Updated: time.Now(), Size: a.Size, Modified: a.Modified, SHA256: a.SHA256, Verification: "uploaded_unverified"}
		if s.vault.Locked() {
			j.State = "waiting_unlock"
		}
		j, e = s.uploads.EnqueueUpload(ctx, j)
		if e != nil {
			return jobs, e
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}
func (s *CloudService) List(ctx context.Context, limit, offset int) ([]core.UploadJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, e := s.uploads.Uploads(ctx, limit, offset)
	for i := range items {
		if m, ok := s.progress[items[i].ID]; ok {
			items[i].Metrics = core.CloneMetrics(m)
		}
	}
	return items, e
}
func (s *CloudService) Retry(ctx context.Context, id string, acceptDestination bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[id] != nil {
		return errors.New("upload is active")
	}
	j, e := s.uploads.Upload(ctx, id)
	if e != nil {
		return e
	}
	p, e := s.profiles.Profile(ctx, j.ProfileID)
	if e != nil {
		return e
	}
	if !destinationEqual(j.Profile, p) && !acceptDestination {
		return errors.New("profile destination changed; explicit confirmation required")
	}
	if acceptDestination {
		if j.MultipartID != "" {
			return errors.New("clean up multipart at its original destination first")
		}
		if j.ETag != "" || j.VersionID != "" {
			return errors.New("an acknowledged remote copy must be checked at its original destination; create a separate upload for the new destination")
		}
		a, e := s.assets.Asset(ctx, j.AssetID)
		if e != nil {
			return e
		}
		j.Profile = p
		j.Key = core.DestinationKey(p, a)
	}
	if j.State == "done" {
		return errors.New("already uploaded")
	}
	j.State = "queued"
	j.Error = ""
	j.ErrorCode = ""
	j.NextAttempt = time.Time{}
	j.Attempts = 0
	j.Updated = time.Now()
	return s.uploads.SaveUpload(ctx, j)
}
func (s *CloudService) Cancel(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, e := s.uploads.Upload(ctx, id)
	if e != nil {
		return e
	}
	if j.Terminal() {
		return nil
	}
	if cancel := s.active[id]; cancel != nil {
		j.State = "canceling"
		cancel()
	} else {
		j.State = "canceled"
	}
	j.Updated = time.Now()
	return s.uploads.SaveUpload(ctx, j)
}
func (s *CloudService) Cleanup(ctx context.Context, id string) error {
	s.mu.Lock()
	j, e := s.uploads.Upload(ctx, id)
	if e != nil {
		s.mu.Unlock()
		return e
	}
	if s.active[id] != nil {
		s.mu.Unlock()
		return errors.New("upload is active")
	}
	c, e := s.credentials(ctx, j.ProfileID)
	s.mu.Unlock()
	if e != nil {
		return e
	}
	if j.MultipartID == "" {
		return nil
	}
	if e = s.backend.Cleanup(ctx, j.Profile, c, j.Key, j.MultipartID); e != nil {
		return e
	}
	j.MultipartID = ""
	return s.uploads.SaveUpload(ctx, j)
}
func destinationEqual(a, b core.S3Profile) bool {
	return a.Endpoint == b.Endpoint && a.Region == b.Region && a.Bucket == b.Bucket && a.Prefix == b.Prefix && a.Style == b.Style && a.KeyTemplate == b.KeyTemplate
}
func (s *CloudService) loop() {
	defer s.wg.Done()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-tick.C:
			s.schedule()
		}
	}
}
func (s *CloudService) schedule() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing || s.locking || s.failure != nil {
		return
	}
	if s.vault.Locked() {
		return
	}
	jobs, e := s.uploads.PendingUploads(s.ctx, s.options.Objects*2)
	if e != nil {
		s.failure = e
		return
	}
	for _, j := range jobs {
		if s.allowed != nil && !s.allowed[j.ID] {
			continue
		}
		if j.Terminal() || s.active[j.ID] != nil || j.State == "blocked_profile_changed" || j.State == "source_missing" || j.State == "conflict" || time.Now().Before(j.NextAttempt) {
			continue
		}
		p, e := s.profiles.Profile(s.ctx, j.ProfileID)
		if e != nil {
			continue
		}
		if !destinationEqual(p, j.Profile) {
			j.State = "blocked_profile_changed"
			j.ErrorCode = "profile_changed"
		} else if !p.Enabled {
			j.State = "paused"
		} else if s.vault.Locked() {
			j.State = "waiting_unlock"
		} else if len(s.active) >= s.options.Objects {
			j.State = "waiting_slot"
		} else {
			if j.State == "uploading" || j.State == "remote_check" || j.State == "canceling" {
				j.State = "queued"
			}
			c, e := s.credentials(s.ctx, j.ProfileID)
			if e != nil {
				j.State = "waiting_unlock"
			} else {
				j.State = "uploading"
				j.Updated = time.Now()
				if e = s.uploads.SaveUpload(s.ctx, j); e != nil {
					s.failure = e
					return
				}
				ctx, cancel := context.WithCancel(s.ctx)
				s.active[j.ID] = cancel
				s.workers.Add(1)
				go s.execute(ctx, j, c, s.options)
				continue
			}
		}
		old, e := s.uploads.Upload(s.ctx, j.ID)
		if e == nil && old.State != j.State {
			j.Updated = time.Now()
			if e = s.uploads.SaveUpload(s.ctx, j); e != nil {
				s.failure = e
				return
			}
		}
	}
}
func sourceFile(a core.Asset, j core.UploadJob) (*os.File, error) {
	fd, e := syscall.Open(a.Path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, errors.New("source_missing")
	}
	f := os.NewFile(uintptr(fd), a.Path)
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Size() != j.Size || !st.ModTime().Equal(j.Modified) {
		f.Close()
		return nil, errors.New("source_changed")
	}
	return f, nil
}
func digest(ctx context.Context, f *os.File) (string, error) {
	if _, e := f.Seek(0, io.SeekStart); e != nil {
		return "", e
	}
	h := sha256.New()
	b := make([]byte, 128*1024)
	for {
		if e := ctx.Err(); e != nil {
			return "", e
		}
		n, e := f.Read(b)
		if n > 0 {
			_, _ = h.Write(b[:n])
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", e
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (s *CloudService) execute(ctx context.Context, j core.UploadJob, c core.Credentials, o core.UploadOptions) {
	defer s.workers.Done()
	var result core.RemoteObject
	var err error
	a, e := s.assets.Asset(ctx, j.AssetID)
	if e != nil {
		err = errors.New("source_missing")
	} else {
		var f *os.File
		f, err = sourceFile(a, j)
		if err == nil {
			defer f.Close()
			var sum string
			sum, err = digest(ctx, f)
			if err == nil && j.SHA256 != "" && j.SHA256 != sum {
				err = errors.New("source_changed")
			}
			if err == nil {
				j.SHA256 = sum
				_, err = f.Seek(0, io.SeekStart)
			}
			if err == nil && j.MultipartID != "" {
				err = s.backend.Cleanup(ctx, j.Profile, c, j.Key, j.MultipartID)
				if err == nil {
					j.MultipartID = ""
					err = s.uploads.SaveUpload(ctx, j)
				}
			}
			checkpoint := false
			if err == nil && (j.ETag != "" || j.VersionID != "") {
				result, err = s.backend.Check(ctx, j.Profile, c, j.Key)
				if err == nil {
					if result.Size != j.Size || j.ETag != "" && result.ETag != j.ETag || j.VersionID != "" && result.VersionID != j.VersionID {
						err = errors.New("remote_object_changed")
					} else {
						checkpoint = true
						result.Verification = "size_verified"
						if result.ChecksumSHA256 != "" {
							bytes, _ := hex.DecodeString(sum)
							if base64.StdEncoding.EncodeToString(bytes) != result.ChecksumSHA256 {
								err = errors.New("remote_checksum_mismatch")
							} else {
								result.Verification = "checksum_verified"
							}
						}
					}
				}
			}
			if err == nil && !checkpoint {
				started := time.Now()
				var previousAt time.Time
				var previousBytes int64
				err = s.uploads.SaveUpload(ctx, j)
				if err == nil {
					result, err = s.backend.Upload(ctx, j.Profile, c, j.Key, f, j.Size, sum, o.Parts, o.RateBPS, func(event core.UploadEvent) {
						s.mu.Lock()
						defer s.mu.Unlock()
						if event.MultipartID != "" {
							j.MultipartID = event.MultipartID
							if e := s.uploads.SaveUpload(context.Background(), j); e != nil {
								s.failure = e
								s.active[j.ID]()
							}
						}
						if event.Cleanup {
							j.MultipartID = ""
							if e := s.uploads.SaveUpload(context.Background(), j); e != nil {
								s.failure = e
							}
						}
						if event.Stage == "uploaded" {
							j.ETag = event.ETag
							j.VersionID = event.VersionID
							j.Verification = "uploaded_unverified"
							j.State = "remote_check"
							if e := s.uploads.SaveUpload(context.Background(), j); e != nil {
								s.failure = e
								s.active[j.ID]()
							}
						}
						if event.Stage == "remote_check" {
							j.State = "remote_check"
							j.Updated = time.Now()
							if e := s.uploads.SaveUpload(context.Background(), j); e != nil {
								s.failure = e
								s.active[j.ID]()
							}
						}
						if event.Total > 0 {
							now := time.Now()
							m := core.Metrics{DownloadedBytes: event.Bytes, TotalBytes: &event.Total, At: now}
							if now.Sub(started) > 0 {
								rate := float64(event.Bytes) / now.Sub(started).Seconds()
								m.SpeedBPS = &rate
								if rate > 0 {
									eta := float64(event.Total-event.Bytes) / rate
									m.ETASeconds = &eta
								}
							}
							percent := min(99.0, float64(event.Bytes)/float64(event.Total)*100)
							m.Percent = &percent
							if now.Sub(previousAt) > 200*time.Millisecond || event.Bytes == event.Total {
								s.progress[j.ID] = m
								previousAt = now
								previousBytes = event.Bytes
							}
							_ = previousBytes
						}
					})
				}
			}
			if err == nil {
				after, e := digest(ctx, f)
				st, statErr := f.Stat()
				current, pathErr := os.Lstat(a.Path)
				if e != nil || statErr != nil || pathErr != nil || !os.SameFile(st, current) || st.Size() != j.Size || !st.ModTime().Equal(j.Modified) || after != sum {
					err = errors.New("source_changed")
				}
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, j.ID)
	delete(s.progress, j.ID)
	j.Updated = time.Now()
	if err == nil {
		j.State = "done"
		j.Verification = result.Verification
		j.ETag = result.ETag
		j.VersionID = result.VersionID
		j.Finished = time.Now()
		j.Error = ""
		j.ErrorCode = ""
	} else if ctx.Err() != nil {
		if s.closing || s.locking {
			j.State = "waiting_unlock"
		} else {
			j.State = "canceled"
		}
		j.ErrorCode = "canceled"
		j.Error = "Transfer stopped; retry starts from the beginning"
	} else {
		j.State = "failed"
		j.Verification = "verification_failed"
		j.Error = err.Error()
		j.ErrorCode = "upload_failed"
		if strings.Contains(j.Error, "source_") {
			j.State = "source_missing"
			j.ErrorCode = "source_changed"
		}
		if strings.Contains(j.Error, "PreconditionFailed") || strings.Contains(j.Error, "ConditionalRequestConflict") {
			j.State = "conflict"
			j.ErrorCode = "conflict"
		}
		temporary := retryable(err)
		if x, ok := err.(interface{ Retryable() bool }); ok {
			temporary = x.Retryable()
		}
		if temporary && j.Attempts < o.RetryAttempts {
			j.Attempts++
			j.State = "queued"
			j.NextAttempt = time.Now().Add(retryDelay(j.Attempts))
		}
	}
	if e := s.uploads.SaveUpload(context.Background(), j); e != nil {
		s.failure = e
	}
}
func (s *CloudService) Stats() (int, *float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	speed := 0.0
	known := false
	for _, m := range s.progress {
		if time.Since(m.At) < 3*time.Second && m.SpeedBPS != nil {
			speed += *m.SpeedBPS
			known = true
		}
	}
	if known {
		return len(s.active), &speed, s.failure
	}
	return len(s.active), nil, s.failure
}
func (s *CloudService) Close() {
	s.mu.Lock()
	s.closing = true
	s.cancel()
	for _, cancel := range s.active {
		cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
	s.workers.Wait()
	s.vault.Lock()
}

func (s *CloudService) HasVault(ctx context.Context) bool {
	_, e := s.profiles.VaultState(ctx)
	return e == nil
}

func (s *CloudService) Restrict(ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ids == nil {
		s.allowed = nil
		return
	}
	s.allowed = map[string]bool{}
	for _, id := range ids {
		s.allowed[id] = true
	}
}

func (s *CloudService) Get(ctx context.Context, id string) (core.UploadJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, e := s.uploads.Upload(ctx, id)
	if m, ok := s.progress[id]; ok {
		j.Metrics = core.CloneMetrics(m)
	}
	return j, e
}

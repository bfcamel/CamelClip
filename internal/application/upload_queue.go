// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/bfcamel/CamelClip/internal/core"
)

type CloudService struct {
	life             *lifetime
	closeOnce        sync.Once
	closeErr         error
	lockMu           sync.Mutex
	cleanups         map[string]bool
	network          map[string]context.CancelFunc
	operations       sync.WaitGroup
	ctx              context.Context
	cancel           context.CancelFunc
	profiles         ProfileRepository
	uploads          UploadRepository
	assets           AssetRepository
	profileStore     *profileManager
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
	s := &CloudService{
		life: newLifetime(ctx), ctx: ctx, cancel: cancel,
		cleanups: map[string]bool{}, network: map[string]context.CancelFunc{},
		profiles: p, uploads: u, assets: a,
		profileStore: &profileManager{profiles: p, vault: vault, factory: factory},
		backend:      backend, options: (core.UploadOptions{}).WithDefaults(),
		active: map[string]context.CancelFunc{}, progress: map[string]core.Metrics{},
	}
	s.failure = s.uploads.RecoverUploads(parent)
	s.wg.Add(1)
	go s.loop()
	return s
}
func (s *CloudService) Locked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profileStore.Locked()
}
func (s *CloudService) Options() core.UploadOptions {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.options
}
func (s *CloudService) SetOptions(o core.UploadOptions) error {
	if err := o.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing || s.ctx.Err() != nil {
		return ErrClosed
	}
	if s.failure != nil {
		return s.failure
	}
	s.options = o
	return nil
}
func (s *CloudService) Profiles(ctx context.Context) ([]core.S3Profile, error) {
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	return s.profiles.Profiles(ctx)
}
func (s *CloudService) Unlock(ctx context.Context, password []byte) error {
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profileStore.Unlock(ctx, password)
}
func (s *CloudService) Initialize(ctx context.Context, password []byte) error {
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profileStore.Initialize(ctx, password)
}
func (s *CloudService) Rotate(ctx context.Context, oldPassword, newPassword []byte) error {
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.active) > 0 {
		return errors.New("wait for active uploads before rotating password")
	}
	return s.profileStore.Rotate(ctx, oldPassword, newPassword)
}
func (s *CloudService) Lock() {
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	s.mu.Lock()
	s.locking = true
	for _, cancel := range s.network {
		cancel()
	}
	for _, cancel := range s.active {
		cancel()
	}
	s.mu.Unlock()
	s.workers.Wait()
	s.operations.Wait()
	s.mu.Lock()
	s.profileStore.Lock()
	s.locking = false
	s.mu.Unlock()
}
func (s *CloudService) SaveProfile(ctx context.Context, p core.S3Profile, c *core.Credentials) error {
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profileStore.SaveProfile(ctx, p, c)
}
func (s *CloudService) DeleteProfile(ctx context.Context, id string) error {
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profiles.DeleteProfile(ctx, id)
}
func (s *CloudService) Reset(ctx context.Context, confirmed bool) error {
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	if !confirmed {
		return errors.New("explicit reset confirmation required")
	}
	s.Lock()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profiles.ResetProfiles(ctx)
}
func (s *CloudService) credentials(ctx context.Context, id string) (core.Credentials, error) {
	return s.profileStore.credentials(ctx, id)
}
func (s *CloudService) Test(ctx context.Context, id string, write bool) error {
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	s.mu.Lock()
	p, e := s.profiles.Profile(ctx, id)
	if e != nil {
		s.mu.Unlock()
		return e
	}
	c, e := s.credentials(ctx, id)
	if e == nil && s.locking {
		e = errors.New("vault is locking")
	}
	if e == nil {
		var finish func()
		ctx, finish = s.networkOperationLocked(ctx)
		defer finish()
	}
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
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
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
		if s.profileStore.Locked() {
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
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil {
		return nil, s.failure
	}
	items, e := s.uploads.Uploads(ctx, limit, offset)
	for i := range items {
		if m, ok := s.progress[items[i].ID]; ok {
			items[i].Metrics = core.CloneMetrics(m)
		}
	}
	return items, e
}
func (s *CloudService) Retry(ctx context.Context, id string, acceptDestination bool) error {
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[id] != nil || s.cleanups[id] {
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
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
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
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	s.mu.Lock()
	j, e := s.uploads.Upload(ctx, id)
	if e != nil {
		s.mu.Unlock()
		return e
	}
	if s.active[id] != nil || s.cleanups[id] {
		s.mu.Unlock()
		return errors.New("upload is active")
	}
	c, e := s.credentials(ctx, j.ProfileID)
	if e == nil && s.locking {
		e = errors.New("vault is locking")
	}
	if e == nil {
		s.cleanups[id] = true
		var finish func()
		ctx, finish = s.networkOperationLocked(ctx)
		defer finish()
		defer func() { s.mu.Lock(); delete(s.cleanups, id); s.mu.Unlock() }()
	}
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
	s.mu.Lock()
	defer s.mu.Unlock()
	current, e := s.uploads.Upload(ctx, id)
	if e != nil {
		return e
	}
	if current.MultipartID != j.MultipartID {
		return errors.New("multipart checkpoint changed during cleanup")
	}
	current.MultipartID = ""
	return s.uploads.SaveUpload(ctx, current)
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
	if s.profileStore.Locked() {
		return
	}
	var allowed []string
	if s.allowed != nil {
		allowed = make([]string, 0, len(s.allowed))
		for id := range s.allowed {
			allowed = append(allowed, id)
		}
	}
	jobs, e := s.uploads.PendingUploads(s.ctx, s.options.Objects*2, allowed)
	if e != nil {
		s.failure = e
		return
	}
	for _, j := range jobs {
		if s.allowed != nil && !s.allowed[j.ID] {
			continue
		}
		if j.Terminal() || s.active[j.ID] != nil || s.cleanups[j.ID] || j.State == "blocked_profile_changed" || j.State == "source_missing" || j.State == "conflict" || time.Now().Before(j.NextAttempt) {
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
		} else if s.profileStore.Locked() {
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
				// Destination equality was checked above. Refresh verified capabilities
				// without silently accepting a different upload destination.
				j.Profile = p
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
func (s *CloudService) execute(ctx context.Context, j core.UploadJob, c core.Credentials, o core.UploadOptions) {
	defer s.workers.Done()
	var result core.RemoteObject
	var err error
	defer func() {
		if value := recover(); value != nil {
			err = core.ContractError(fmt.Sprintf("upload adapter panic: %v", value))
		}
		s.finishUpload(ctx, j, result, err, o)
	}()
	a, e := s.assets.Asset(ctx, j.AssetID)
	if e != nil {
		err = &core.Failure{Code: core.CodeSourceChanged, Message: "source_missing"}
	} else {
		var f *os.File
		f, err = sourceFile(a, j)
		if err == nil {
			defer f.Close()
			var sum string
			sum, err = digest(ctx, f)
			if err == nil && j.SHA256 != "" && j.SHA256 != sum {
				err = &core.Failure{Code: core.CodeSourceChanged, Message: "source_changed"}
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
				var observerMu sync.Mutex
				accepting := true
				var callbackErr error
				err = s.uploads.SaveUpload(ctx, j)
				if err == nil {
					func() {
						defer func() { observerMu.Lock(); accepting = false; observerMu.Unlock() }()
						result, err = s.backend.Upload(ctx, j.Profile, c, j.Key, f, j.Size, sum, o.Parts, o.RateBPS, func(event core.UploadEvent) {
							observerMu.Lock()
							defer observerMu.Unlock()
							if !accepting || callbackErr != nil {
								return
							}
							if event.Bytes < 0 || event.Total < 0 || event.Total > 0 && event.Bytes > event.Total {
								callbackErr = core.ContractError("upload adapter returned invalid byte counts")
								return
							}
							switch event.Stage {
							case "", "uploading", "uploaded", "remote_check":
							default:
								callbackErr = core.ContractError("upload adapter returned an unknown stage")
								return
							}
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
					}()
					if callbackErr != nil {
						err = callbackErr
					}
					if err == nil && (result.Size != j.Size || (result.Verification != "size_verified" && result.Verification != "checksum_verified")) {
						err = core.ContractError("upload adapter returned an unverified or mismatched result")
					}
				}
			}
			if err == nil {
				after, e := digest(ctx, f)
				st, statErr := f.Stat()
				current, pathErr := os.Lstat(a.Path)
				if e != nil || statErr != nil || pathErr != nil || !os.SameFile(st, current) || st.Size() != j.Size || !st.ModTime().Equal(j.Modified) || after != sum {
					err = &core.Failure{Code: core.CodeSourceChanged, Message: "source_changed"}
				}
			}
		}
	}
}

func (s *CloudService) finishUpload(ctx context.Context, j core.UploadJob, result core.RemoteObject, err error, o core.UploadOptions) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, j.ID)
	delete(s.progress, j.ID)
	j.Updated = time.Now()
	if err == nil && ctx.Err() == nil {
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
		j.ErrorCode = string(core.CodeOf(err))
		if core.CodeOf(err) == core.CodeSourceChanged {
			j.State = "source_missing"
			j.ErrorCode = "source_changed"
		}
		if core.CodeOf(err) == core.CodeConflict {
			j.State = "conflict"
			j.ErrorCode = "conflict"
		}
		temporary := retryable(err)
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
func (s *CloudService) Close() error {
	s.closeOnce.Do(func() {
		s.life.stop()
		s.mu.Lock()
		s.closing = true
		s.cancel()
		for _, cancel := range s.active {
			cancel()
		}
		s.mu.Unlock()
		s.wg.Wait()
		s.workers.Wait()
		s.life.wait()
		s.lockMu.Lock()
		defer s.lockMu.Unlock()
		s.mu.Lock()
		defer s.mu.Unlock()
		s.profileStore.Lock()
		s.closeErr = s.failure
	})
	return s.closeErr
}

func (s *CloudService) Err() error { s.mu.Lock(); defer s.mu.Unlock(); return s.failure }

// Called with mu held; Lock prevents admission before waiting on operations.
func (s *CloudService) networkOperationLocked(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	id := core.NewID()
	s.network[id] = cancel
	s.operations.Add(1)
	return ctx, func() { cancel(); s.mu.Lock(); delete(s.network, id); s.mu.Unlock(); s.operations.Done() }
}

func (s *CloudService) HasVault(ctx context.Context) bool {
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return false
	}
	defer done()
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
	ctx, done, err := s.life.begin(ctx)
	if err != nil {
		return core.UploadJob{}, err
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil {
		return core.UploadJob{}, s.failure
	}
	j, e := s.uploads.Upload(ctx, id)
	if m, ok := s.progress[id]; ok {
		j.Metrics = core.CloneMetrics(m)
	}
	return j, e
}

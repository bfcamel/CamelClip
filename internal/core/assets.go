// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package core

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode"
)

func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

type Asset struct {
	NeedsProbe        bool      `json:"needs_probe"`
	LocalVerification string    `json:"local_verification"`
	ID                string    `json:"id"`
	Path              string    `json:"path"`
	Title             string    `json:"title"`
	Size              int64     `json:"size"`
	Modified          time.Time `json:"modified"`
	Added             time.Time `json:"added"`
	Kind              string    `json:"kind"`
	Container         string    `json:"container"`
	MIME              string    `json:"mime"`
	Duration          float64   `json:"duration"`
	Width             int       `json:"width"`
	Height            int       `json:"height"`
	VideoCodec        string    `json:"video_codec,omitempty"`
	AudioCodec        string    `json:"audio_codec,omitempty"`
	SourceURL         string    `json:"source_url,omitempty"`
	DownloadID        int       `json:"download_job_id,omitempty"`
	Status            string    `json:"status"`
	SHA256            string    `json:"sha256,omitempty"`
	RemoteCopies      int       `json:"remote_copies"`
}
type AssetQuery struct {
	NeedsProbe                            bool
	Search, Kind, Status, Container, Sort string
	MinHeight                             int
	HasCopies                             *bool
	Since, Until                          time.Time
	Offset, Limit                         int
}
type AssetPage struct {
	Items []Asset `json:"items"`
	Total int     `json:"total"`
}
type MediaProber interface {
	Probe(context.Context, string) (Asset, error)
}
type ScanReport struct {
	Added, Updated, Missing, Skipped int
	Errors                           []string
}
type S3Profile struct {
	ConditionalVerified bool   `json:"conditional_writes_verified"`
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Endpoint            string `json:"endpoint"`
	Region              string `json:"region"`
	Bucket              string `json:"bucket"`
	Prefix              string `json:"prefix"`
	Style               string `json:"addressing_style"`
	Provider            string `json:"provider,omitempty"`
	Enabled             bool   `json:"enabled"`
	AllowLoopbackHTTP   bool   `json:"allow_loopback_http"`
	Revision            int    `json:"revision"`
	Created, Updated    time.Time
	KeyTemplate         string `json:"key_template,omitempty"`
	AutoUpload          bool   `json:"auto_upload"`
}
type Credentials struct {
	AccessKey    string `json:"-"`
	SecretKey    string `json:"-"`
	SessionToken string `json:"-"`
}
type SealedSecret struct {
	Version     int
	Nonce, Blob []byte
}
type VaultState struct {
	Version    int
	Salt       []byte
	Memory     uint32
	Iterations uint32
	Parallel   uint8
	Check      SealedSecret
}
type CredentialVault interface {
	Initialize([]byte) (VaultState, error)
	Unlock(VaultState, []byte) error
	Locked() bool
	Lock()
	Seal(string, Credentials) (SealedSecret, error)
	Open(string, SealedSecret) (Credentials, error)
}

func ValidateProfile(p S3Profile) error {
	if strings.TrimSpace(p.Name) == "" || len(p.Name) > 120 || p.Region == "" || p.Bucket == "" || strings.ContainsAny(p.Bucket, "/\\ \r\n\t") {
		return fmt.Errorf("profile requires name, region and bucket")
	}
	if p.Style != "auto" && p.Style != "virtual_host" && p.Style != "path_style" {
		return fmt.Errorf("addressing style: auto/virtual_host/path_style")
	}
	if p.Endpoint != "" {
		u, e := url.Parse(p.Endpoint)
		if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
			return fmt.Errorf("endpoint must be a plain HTTPS origin")
		}
		if u.Scheme != "https" {
			ip := net.ParseIP(u.Hostname())
			if !(u.Scheme == "http" && p.AllowLoopbackHTTP && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())) {
				return fmt.Errorf("HTTPS required; explicit HTTP exception is limited to loopback IP addresses")
			}
		}
	}
	if strings.HasPrefix(p.Prefix, "/") || strings.Contains(p.Prefix, "\\") {
		return fmt.Errorf("prefix must be relative")
	}
	for _, part := range strings.Split(p.Prefix, "/") {
		if part == ".." || part == "." {
			return fmt.Errorf("invalid prefix")
		}
	}
	for _, r := range p.Prefix + p.Name + p.Region + p.Bucket {
		if unicode.IsControl(r) {
			return fmt.Errorf("control characters are not allowed")
		}
	}
	if p.KeyTemplate != "" && p.KeyTemplate != "YYYY/MM/{filename}" && p.KeyTemplate != "{filename}" {
		return fmt.Errorf("key template: {filename} or YYYY/MM/{filename}")
	}
	return nil
}
func DestinationKey(p S3Profile, a Asset) string {
	name := path.Base(strings.ReplaceAll(a.Path, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '_'
		}
		return r
	}, name)
	if name == "." || name == "/" || name == "" {
		name = a.ID
	}
	prefix := strings.Trim(p.Prefix, "/")
	if p.KeyTemplate == "YYYY/MM/{filename}" {
		date := a.Added
		if date.IsZero() {
			date = time.Now()
		}
		prefix = path.Join(prefix, date.Format("2006/01"))
	}
	return path.Join(prefix, name)
}

type UploadJob struct {
	ID                         string      `json:"id"`
	AssetID                    string      `json:"asset_id"`
	ProfileID                  string      `json:"profile_id"`
	Profile                    S3Profile   `json:"destination"`
	Key                        string      `json:"key"`
	State                      UploadState `json:"state"`
	ErrorCode                  string      `json:"error_code,omitempty"`
	Error                      string      `json:"error,omitempty"`
	Attempts                   int         `json:"attempts"`
	Created, Updated, Finished time.Time
	NextAttempt                time.Time `json:"next_attempt"`
	Size                       int64     `json:"size"`
	Modified                   time.Time
	SHA256                     string `json:"sha256,omitempty"`
	MultipartID                string `json:"multipart_id,omitempty"`
	ETag, VersionID            string
	Verification               string  `json:"verification"`
	Metrics                    Metrics `json:"metrics"`
	Auto                       bool    `json:"automatic"`
}

type UploadState string

const (
	UploadQueued         UploadState = "queued"
	UploadWaitingUnlock  UploadState = "waiting_unlock"
	UploadWaitingSlot    UploadState = "waiting_slot"
	UploadPaused         UploadState = "paused"
	UploadRunning        UploadState = "uploading"
	UploadRemoteCheck    UploadState = "remote_check"
	UploadCanceling      UploadState = "canceling"
	UploadCanceled       UploadState = "canceled"
	UploadDone           UploadState = "done"
	UploadFailed         UploadState = "failed"
	UploadConflict       UploadState = "conflict"
	UploadSourceMissing  UploadState = "source_missing"
	UploadProfileChanged UploadState = "blocked_profile_changed"
)

func (s UploadState) Valid() bool {
	switch s {
	case UploadQueued, UploadWaitingUnlock, UploadWaitingSlot, UploadPaused, UploadRunning, UploadRemoteCheck, UploadCanceling, UploadCanceled, UploadDone, UploadFailed, UploadConflict, UploadSourceMissing, UploadProfileChanged:
		return true
	}
	return false
}
func (s UploadState) Terminal() bool {
	return s == UploadDone || s == UploadFailed || s == UploadCanceled
}
func (s UploadState) NeedsAttention() bool {
	return s == UploadConflict || s == UploadSourceMissing || s == UploadProfileChanged
}
func (j UploadJob) Terminal() bool { return j.State.Terminal() }

type UploadOptions struct {
	Objects, Parts int
	RetryAttempts  int
	RateBPS        int64
}
type UploadEvent struct {
	ETag, VersionID string
	Stage           string
	Bytes           int64
	Total           int64
	MultipartID     string
	Cleanup         bool
}
type RemoteObject struct {
	Size                                          int64
	ETag, VersionID, ChecksumSHA256, Verification string
}
type UploadSource interface {
	io.ReadSeeker
	io.ReaderAt
}

// Uploader serializes callbacks per Upload and joins them before returning.
// Callbacks must return promptly and must not reenter their owning service.
// Successful uploads return a verified object of the requested size; cancellation
// must stop all parts and report whether multipart cleanup is still required.
type Uploader interface {
	Test(context.Context, S3Profile, Credentials, bool) error
	Upload(context.Context, S3Profile, Credentials, string, UploadSource, int64, string, int, int64, func(UploadEvent)) (RemoteObject, error)
	Check(context.Context, S3Profile, Credentials, string) (RemoteObject, error)
	Cleanup(context.Context, S3Profile, Credentials, string, string) error
}
type SystemSnapshot struct {
	At                              time.Time
	CPU                             *float64
	Cores                           []*float64
	RAMUsed, RAMTotal, RAMAvailable *uint64
	ProcessRSS                      *uint64
	Error                           string
}
type SystemMetricsCollector interface {
	Collect(context.Context) (SystemSnapshot, error)
}

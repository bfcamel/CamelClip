// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package core

import "errors"

// QueuePolicy defines shared retry and staging bounds for every entry point.
type QueuePolicy struct {
	RetryAttempts int   `json:"retry_attempts"`
	PendingFiles  int   `json:"max_staging_files"`
	PendingBytes  int64 `json:"max_staging_bytes"`
	MinFreeBytes  int64 `json:"min_free_bytes"`
}

func (p QueuePolicy) Validate() error {
	if p.RetryAttempts < 0 || p.RetryAttempts > 10 || p.PendingFiles < 0 || p.PendingBytes < 0 || p.MinFreeBytes < 0 {
		return errors.New("invalid queue policy")
	}
	return nil
}
func (o UploadOptions) Validate() error {
	if o.Objects < 1 || o.Objects > 32 || o.Parts < 1 || o.Parts > 4 || o.RetryAttempts < 0 || o.RetryAttempts > 10 || o.RateBPS < 0 {
		return errors.New("invalid upload limits")
	}
	return nil
}

// WithDefaults resolves the persisted zero values accepted by settings. Runtime
// services use Validate directly and require explicit positive concurrency.
func (o UploadOptions) WithDefaults() UploadOptions {
	if o.Objects == 0 {
		o.Objects = 2
	}
	if o.Parts == 0 {
		o.Parts = 2
	}
	return o
}

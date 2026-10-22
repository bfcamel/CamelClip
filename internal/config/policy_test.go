// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package config

import "testing"

func TestSettingsAndRuntimePolicyBoundsAgree(t *testing.T) {
	for _, tc := range []struct {
		name   string
		valid  bool
		mutate func(*Settings)
	}{
		{"defaults", true, func(*Settings) {}},
		{"legacy zero uploads", true, func(s *Settings) { s.MaxUploads = 0; s.UploadParts = 0 }},
		{"maximum", true, func(s *Settings) { s.MaxUploads = 32; s.UploadParts = 4; s.RetryAttempts = 10 }},
		{"negative uploads", false, func(s *Settings) { s.MaxUploads = -1 }},
		{"excess uploads", false, func(s *Settings) { s.MaxUploads = 33 }},
		{"negative parts", false, func(s *Settings) { s.UploadParts = -1 }},
		{"excess parts", false, func(s *Settings) { s.UploadParts = 5 }},
		{"negative retries", false, func(s *Settings) { s.RetryAttempts = -1 }},
		{"excess retries", false, func(s *Settings) { s.RetryAttempts = 11 }},
		{"negative rate", false, func(s *Settings) { s.UploadRateBPS = -1 }},
		{"negative staging files", false, func(s *Settings) { s.StagingFiles = -1 }},
		{"negative staging bytes", false, func(s *Settings) { s.StagingBytes = -1 }},
		{"negative free space", false, func(s *Settings) { s.MinFreeBytes = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Defaults(t.TempDir())
			tc.mutate(&s)
			runtimeValid := s.QueuePolicy().Validate() == nil && s.UploadOptions().Validate() == nil
			if runtimeValid != tc.valid {
				t.Fatalf("runtime validity = %v, want %v", runtimeValid, tc.valid)
			}
			if (Validate(s) == nil) != runtimeValid {
				t.Fatalf("settings/runtime validation disagree: %+v", s)
			}
			normalized, err := Normalize(s)
			if (err == nil) != runtimeValid {
				t.Fatalf("normalization/runtime validation disagree: %v", err)
			}
			if err == nil && (normalized.MaxUploads != s.UploadOptions().Objects || normalized.UploadParts != s.UploadOptions().Parts) {
				t.Fatal("normalization changed runtime defaults")
			}
		})
	}
}

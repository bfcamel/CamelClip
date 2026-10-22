// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package core

import "testing"

func TestUploadRuntimeRequiresExplicitConcurrency(t *testing.T) {
	zero := UploadOptions{}
	if zero.Validate() == nil {
		t.Fatal("runtime accepted unresolved defaults")
	}
	resolved := zero.WithDefaults()
	if resolved.Objects != 2 || resolved.Parts != 2 || resolved.Validate() != nil {
		t.Fatal(resolved)
	}
	invalid := UploadOptions{Objects: -1, Parts: 5, RetryAttempts: 11, RateBPS: -1}
	if invalid.WithDefaults() != invalid || invalid.Validate() == nil {
		t.Fatal("defaults hid invalid explicit values")
	}
}

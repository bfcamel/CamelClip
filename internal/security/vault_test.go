// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package security

import (
	"bytes"
	"github.com/bfcamel/video-downloader/internal/core"
	"testing"
)

func TestVaultBindingPasswordsAndTampering(t *testing.T) {
	v := New()
	password := []byte("long secret password")
	state, e := v.Initialize(password)
	if e != nil {
		t.Fatal(e)
	}
	credentials := core.Credentials{AccessKey: "private-id", SecretKey: "very-secret-key", SessionToken: "private-token"}
	sealed, e := v.Seal("profile-a", credentials)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(sealed.Blob, []byte(credentials.SecretKey)) {
		t.Fatal("plaintext")
	}
	other, _ := v.Seal("profile-a", credentials)
	if bytes.Equal(sealed.Nonce, other.Nonce) {
		t.Fatal("nonce reused")
	}
	if _, e = v.Open("profile-b", sealed); e == nil {
		t.Fatal("record transplant accepted")
	}
	v.Lock()
	if _, e = v.Open("profile-a", sealed); e == nil {
		t.Fatal("locked read")
	}
	if e = v.Unlock(state, []byte("wrong password value")); e == nil {
		t.Fatal("wrong password accepted")
	}
	if e = v.Unlock(state, password); e != nil {
		t.Fatal(e)
	}
	got, e := v.Open("profile-a", sealed)
	if e != nil || got != credentials {
		t.Fatal(e)
	}
	sealed.Blob[0] ^= 1
	if _, e = v.Open("profile-a", sealed); e == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}

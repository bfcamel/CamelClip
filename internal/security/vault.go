// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"sync"
	"unicode/utf8"

	"github.com/bfcamel/CamelClip/internal/core"
	"golang.org/x/crypto/argon2"
)

type Vault struct {
	mu  sync.Mutex
	key []byte
}

func New() *Vault { return &Vault{} }
func derive(s core.VaultState, password []byte) ([]byte, error) {
	if s.Version != 1 || len(s.Salt) != 16 || s.Memory < 8192 || s.Memory > 262144 || s.Iterations < 1 || s.Iterations > 5 || s.Parallel < 1 || s.Parallel > 4 {
		return nil, errors.New("unsupported vault parameters")
	}
	if !utf8.Valid(password) || utf8.RuneCount(password) < 10 || len(password) > 1024 {
		return nil, errors.New("master password must contain at least 10 Unicode characters and at most 1024 bytes")
	}
	return argon2.IDKey(password, s.Salt, s.Iterations, s.Memory, s.Parallel, 32), nil
}
func seal(key []byte, id string, plain []byte) (core.SealedSecret, error) {
	block, e := aes.NewCipher(key)
	if e != nil {
		return core.SealedSecret{}, e
	}
	a, e := cipher.NewGCM(block)
	if e != nil {
		return core.SealedSecret{}, e
	}
	n := make([]byte, a.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return core.SealedSecret{}, e
	}
	return core.SealedSecret{Version: 1, Nonce: n, Blob: a.Seal(nil, n, plain, []byte("CamelClip/v1/"+id))}, nil
}
func open(key []byte, id string, s core.SealedSecret) ([]byte, error) {
	if s.Version != 1 {
		return nil, errors.New("unsupported cipher")
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	a, e := cipher.NewGCM(block)
	if e != nil || len(s.Nonce) != a.NonceSize() {
		return nil, errors.New("invalid encrypted record")
	}
	data, e := a.Open(nil, s.Nonce, s.Blob, []byte("CamelClip/v1/"+id))
	if e != nil {
		return nil, errors.New("incorrect password or damaged encrypted record")
	}
	return data, nil
}
func (v *Vault) Initialize(password []byte) (core.VaultState, error) {
	s := core.VaultState{Version: 1, Salt: make([]byte, 16), Memory: 65536, Iterations: 3, Parallel: 2}
	if _, e := rand.Read(s.Salt); e != nil {
		return s, e
	}
	key, e := derive(s, password)
	if e != nil {
		return s, e
	}
	defer clear(key)
	s.Check, e = seal(key, "vault-check", []byte("CamelClip vault"))
	if e != nil {
		return s, e
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	clear(v.key)
	v.key = append([]byte(nil), key...)
	return s, nil
}
func (v *Vault) Unlock(s core.VaultState, password []byte) error {
	key, e := derive(s, password)
	if e != nil {
		return e
	}
	defer clear(key)
	data, e := open(key, "vault-check", s.Check)
	if e != nil {
		return e
	}
	clear(data)
	v.mu.Lock()
	defer v.mu.Unlock()
	clear(v.key)
	v.key = append([]byte(nil), key...)
	return nil
}
func (v *Vault) Locked() bool { v.mu.Lock(); defer v.mu.Unlock(); return len(v.key) == 0 }
func (v *Vault) Lock()        { v.mu.Lock(); defer v.mu.Unlock(); clear(v.key); v.key = nil }

type payload struct{ Access, Secret, Token string }

func (v *Vault) Seal(id string, c core.Credentials) (core.SealedSecret, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return core.SealedSecret{}, errors.New("vault is locked")
	}
	data, e := json.Marshal(payload{c.AccessKey, c.SecretKey, c.SessionToken})
	if e != nil {
		return core.SealedSecret{}, e
	}
	defer clear(data)
	return seal(v.key, id, data)
}
func (v *Vault) Open(id string, s core.SealedSecret) (core.Credentials, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return core.Credentials{}, errors.New("vault is locked")
	}
	data, e := open(v.key, id, s)
	if e != nil {
		return core.Credentials{}, e
	}
	defer clear(data)
	var p payload
	if e = json.Unmarshal(data, &p); e != nil {
		return core.Credentials{}, errors.New("invalid credential record")
	}
	return core.Credentials{AccessKey: p.Access, SecretKey: p.Secret, SessionToken: p.Token}, nil
}

var _ core.CredentialVault = (*Vault)(nil)

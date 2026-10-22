// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"errors"
	"github.com/bfcamel/CamelClip/internal/core"
)

// profileManager owns credential encryption and atomic profile/vault persistence.
// CloudService holds its mutex for every access; it coordinates in-flight work
// before locking or replacing the vault. This component never schedules uploads.
type profileManager struct {
	profiles ProfileRepository
	vault    core.CredentialVault
	factory  func() core.CredentialVault
}

func (s *profileManager) Unlock(ctx context.Context, password []byte) error {
	state, e := s.profiles.VaultState(ctx)
	if e != nil {
		return errors.New("create a master password first")
	}
	return s.vault.Unlock(state, password)
}

func (s *profileManager) Initialize(ctx context.Context, password []byte) error {
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

func (s *profileManager) Rotate(ctx context.Context, oldPassword, newPassword []byte) error {
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

func (s *profileManager) SaveProfile(ctx context.Context, p core.S3Profile, c *core.Credentials) error {
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

func (s *profileManager) credentials(ctx context.Context, id string) (core.Credentials, error) {
	secret, e := s.profiles.Secret(ctx, id)
	if e != nil {
		return core.Credentials{}, errors.New("credentials unavailable")
	}
	return s.vault.Open(id, secret)
}

func (s *profileManager) Locked() bool { return s.vault.Locked() }
func (s *profileManager) Lock()        { s.vault.Lock() }

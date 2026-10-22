// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

// Package jsonconfig implements config.Repository with an atomic JSON file.
package jsonconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bfcamel/CamelClip/internal/config"
)

type File struct {
	Path     string
	Defaults config.Settings
}

var _ config.Repository = File{}

func (f File) Load(ctx context.Context) (config.Settings, error) {
	if err := ctx.Err(); err != nil {
		return f.Defaults, err
	}
	data, err := os.ReadFile(f.Path)
	if os.IsNotExist(err) {
		return config.Normalize(f.Defaults)
	}
	if err != nil {
		return f.Defaults, err
	}
	s := f.Defaults
	if s.MaxDownloads == 0 {
		s.MaxDownloads = 5
	}
	if err = json.Unmarshal(data, &s); err != nil {
		return f.Defaults, err
	}
	s, err = config.Normalize(s)
	if err != nil {
		return f.Defaults, err
	}
	return s, nil
}
func (f File) Save(ctx context.Context, s config.Settings) error {
	var err error
	s, err = config.Normalize(s)
	if err != nil {
		return err
	}
	if _, err := f.Load(ctx); err != nil {
		return fmt.Errorf("existing settings are invalid; back up and correct the file before saving: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := config.Validate(s); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(f.Path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.Path), "settings-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.Path)
}

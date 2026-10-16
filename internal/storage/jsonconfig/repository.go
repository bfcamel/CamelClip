// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

// Package jsonconfig implements config.Repository with an atomic JSON file.
package jsonconfig

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bfcamel/video-downloader/internal/config"
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
		return f.Defaults, nil
	}
	if err != nil {
		return f.Defaults, err
	}
	var s config.Settings
	if err = json.Unmarshal(data, &s); err != nil {
		return f.Defaults, err
	}
	if err = config.Validate(s); err != nil {
		return f.Defaults, err
	}
	return s, nil
}
func (f File) Save(ctx context.Context, s config.Settings) error {
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

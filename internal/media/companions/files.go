// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package companions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/bfcamel/CamelClip/internal/core"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

type Files struct{ Root string }

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var extensionPattern = regexp.MustCompile(`^[a-z0-9]{1,8}$`)
var kindPattern = regexp.MustCompile(`^[a-z_]{1,32}$`)

func (f Files) path(b core.CompanionBlob) (string, error) {
	if !digestPattern.MatchString(b.Scope) || !digestPattern.MatchString(b.SHA256) || !extensionPattern.MatchString(b.Ext) || !kindPattern.MatchString(b.Kind) {
		return "", errors.New("invalid companion storage identity")
	}
	return filepath.Join(f.Root, "blobs", b.Scope, b.Kind, b.SHA256[:2], b.SHA256+"."+b.Ext), nil
}
func (f Files) Put(ctx context.Context, scope, kind, ext, mime string, data []byte) (core.CompanionBlob, error) {
	if err := ctx.Err(); err != nil {
		return core.CompanionBlob{}, err
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	now := time.Now().UTC()
	identity := sha256.Sum256([]byte(scope + ":" + kind + ":" + ext + ":" + digest))
	b := core.CompanionBlob{ID: hex.EncodeToString(identity[:]), Scope: scope, SHA256: digest, Kind: kind, Ext: ext, MIME: mime, Size: int64(len(data)), Created: now, Used: now}
	path, err := f.path(b)
	if err != nil {
		return b, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return b, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".companion-")
	if err != nil {
		return b, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err = tmp.Write(data); err != nil {
		return b, err
	}
	if err = tmp.Sync(); err != nil {
		return b, err
	}
	if err = tmp.Close(); err != nil {
		return b, err
	}
	if err = os.Link(tmp.Name(), path); err != nil && !os.IsExist(err) {
		return b, err
	}
	return b, nil
}
func (f Files) Read(ctx context.Context, b core.CompanionBlob, max int64) ([]byte, error) {
	if b.Size > max {
		return nil, errors.New("companion exceeds read limit")
	}
	file, err := f.Open(ctx, b)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != b.Size || hex.EncodeToString(sum[:]) != b.SHA256 {
		return nil, errors.New("companion integrity check failed")
	}
	return data, ctx.Err()
}
func (f Files) Open(ctx context.Context, b core.CompanionBlob) (core.CompanionRead, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := f.path(b)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("companion must be a regular file")
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
func (f Files) Remove(ctx context.Context, b core.CompanionBlob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, err := f.path(b)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func (f Files) Export(ctx context.Context, b core.CompanionBlob, path string) error {
	data, err := f.Read(ctx, b, 32<<20)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err != nil {
		os.Remove(path)
		return err
	}
	return closeErr
}

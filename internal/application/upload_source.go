// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/bfcamel/CamelClip/internal/core"
	"io"
	"os"
	"syscall"
)

func sourceFile(a core.Asset, j core.UploadJob) (*os.File, error) {
	fd, e := syscall.Open(a.Path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, &core.Failure{Code: core.CodeSourceChanged, Message: "source_missing"}
	}
	f := os.NewFile(uintptr(fd), a.Path)
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Size() != j.Size || !st.ModTime().Equal(j.Modified) {
		f.Close()
		return nil, &core.Failure{Code: core.CodeSourceChanged, Message: "source_changed"}
	}
	return f, nil
}
func digest(ctx context.Context, f *os.File) (string, error) {
	if _, e := f.Seek(0, io.SeekStart); e != nil {
		return "", e
	}
	h := sha256.New()
	b := make([]byte, 128*1024)
	for {
		if e := ctx.Err(); e != nil {
			return "", e
		}
		n, e := f.Read(b)
		if n > 0 {
			_, _ = h.Write(b[:n])
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", e
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

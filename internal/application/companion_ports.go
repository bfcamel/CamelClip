// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"github.com/bfcamel/CamelClip/internal/core"
)

type CompanionRepository interface {
	SaveCompanionCatalog(context.Context, core.CompanionCatalog) error
	CompanionCatalog(context.Context, string) (core.CompanionCatalog, error)
	SaveCompanionBlob(context.Context, core.CompanionBlob) error
	CompanionBlob(context.Context, string) (core.CompanionBlob, error)
	LinkCompanion(context.Context, string, string, string, bool) error
	CompanionLinks(context.Context, string) ([]core.CompanionBlob, error)
	SaveCaption(context.Context, core.CaptionSnapshot) error
	Caption(context.Context, string) (core.CaptionSnapshot, error)
	Captions(context.Context, string) ([]core.CaptionSnapshot, error)
	CompanionCache(context.Context) ([]CompanionCacheEntry, error)
	DeleteCompanionBlob(context.Context, string) error
}
type CompanionCacheEntry struct {
	Blob core.CompanionBlob
	Kept bool
}
type CompanionFiles interface {
	Put(context.Context, string, string, string, string, []byte) (core.CompanionBlob, error)
	Read(context.Context, core.CompanionBlob, int64) ([]byte, error)
	Open(context.Context, core.CompanionBlob) (core.CompanionRead, error)
	Remove(context.Context, core.CompanionBlob) error
	Export(context.Context, core.CompanionBlob, string) error
}
type CaptionParser interface {
	Parse([]byte) ([]core.CaptionCue, error)
}

// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"errors"
	"github.com/bfcamel/CamelClip/internal/core"
)

type AssetRepository interface {
	Assets(context.Context, core.AssetQuery) (core.AssetPage, error)
	Asset(context.Context, string) (core.Asset, error)
	PutAsset(context.Context, core.Asset) (core.Asset, error)
	AllowAsset(context.Context, string) error
	ForgetAsset(context.Context, string) error
	CanDeleteAsset(context.Context, string) error
	MarkMissing(context.Context, string, []string, bool) (int, error)
}
type ProfileRepository interface {
	Profiles(context.Context) ([]core.S3Profile, error)
	Profile(context.Context, string) (core.S3Profile, error)
	SaveProfile(context.Context, core.S3Profile, *core.SealedSecret) error
	Secret(context.Context, string) (core.SealedSecret, error)
	DeleteProfile(context.Context, string) error
	VaultState(context.Context) (core.VaultState, error)
	SetVault(context.Context, core.VaultState, map[string]core.SealedSecret) error
	ResetProfiles(context.Context) error
}
type UploadRepository interface {
	RecoverUploads(context.Context) error
	PendingUploads(context.Context, int) ([]core.UploadJob, error)
	Uploads(context.Context, int, int) ([]core.UploadJob, error)
	Upload(context.Context, string) (core.UploadJob, error)
	SaveUpload(context.Context, core.UploadJob) error
	EnqueueUpload(context.Context, core.UploadJob) (core.UploadJob, error)
}

var ErrVaultUnconfigured = errors.New("vault has not been configured")

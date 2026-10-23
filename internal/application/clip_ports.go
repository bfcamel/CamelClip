// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"github.com/bfcamel/CamelClip/internal/core"
)

// ClipRepository operations are atomic. Save uses optimistic revision fencing.
// Private file references survive reopen but must not appear in public JSON.
type ClipRepository interface {
	SaveClipPlan(context.Context, core.ClipPlan) error
	ClipPlan(context.Context, string) (core.ClipPlan, error)
	AdmitClip(context.Context, core.ClipBatch) (core.ClipBatch, error)
	ClipBatch(context.Context, string) (core.ClipBatch, error)
	ClipBatches(context.Context, int, string) (core.ClipBatchPage, error)
	NextClip(context.Context, string) (core.ClipBatch, error)
	SaveClip(context.Context, core.ClipBatch, int64) error
	CommitClip(context.Context, core.ClipBatch, int64, int, core.Asset) error
	ClipDeliveries(context.Context) ([]ClipDelivery, error)
	SaveClipDelivery(context.Context, ClipDelivery) error
	ClipProvenance(context.Context, string) ([]ClipProvenance, error)
}
type ClipDelivery struct {
	ID          string         `json:"id"`
	AssetID     string         `json:"asset_id"`
	Destination core.S3Profile `json:"destination"`
	State       string         `json:"state"`
	Error       string         `json:"error,omitempty"`
}
type ClipProvenance struct {
	TaskID   string            `json:"task_id"`
	BatchID  string            `json:"batch_id"`
	Source   core.ClipSource   `json:"source"`
	Segment  core.ClipSegment  `json:"segment"`
	Evidence core.ClipEvidence `json:"evidence"`
}

// ClipFiles owns only private files and journaled, non-overwriting publication.
type ClipFiles interface {
	Acquire(context.Context, string) (context.Context, func(), error)
	CleanupCandidate(context.Context, string) error
	Paths(context.Context, string, string, string, string, string) (workDir, candidate, finalPath string, err error)
	Publish(context.Context, string, string, core.ClipEvidence) error
	Validate(context.Context, string, string) error
	Cleanup(context.Context, string) error
	Metadata(context.Context, string) (core.Asset, error)
}

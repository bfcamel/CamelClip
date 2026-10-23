// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
	"strconv"
	"time"
)

const clipSchema = `
CREATE TABLE IF NOT EXISTS clip_plans(id TEXT PRIMARY KEY,record TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS clip_batches(seq INTEGER PRIMARY KEY AUTOINCREMENT,id TEXT NOT NULL UNIQUE,admission_key TEXT NOT NULL UNIQUE,digest TEXT NOT NULL,revision INTEGER NOT NULL,settled INTEGER NOT NULL,paused INTEGER NOT NULL,record TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS clips_pending ON clip_batches(settled,paused,seq);
CREATE TABLE IF NOT EXISTS clip_provenance(asset_id TEXT NOT NULL,task_id TEXT NOT NULL UNIQUE,record TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS clip_deliveries(id TEXT PRIMARY KEY,asset_id TEXT NOT NULL,profile_id TEXT NOT NULL,state TEXT NOT NULL,record TEXT NOT NULL,UNIQUE(asset_id,profile_id));
CREATE INDEX IF NOT EXISTS clip_delivery_pending ON clip_deliveries(state);
`

// Keep private references in storage, outside the public representation.
type clipRecord struct {
	Batch      core.ClipBatch `json:"batch"`
	SourcePath string         `json:"source_path"`
	Candidates []string       `json:"candidates"`
}

func clipEncode(b core.ClipBatch) (string, error) {
	r := clipRecord{Batch: b, SourcePath: b.SourcePath}
	for _, t := range b.Tasks {
		r.Candidates = append(r.Candidates, t.Candidate)
	}
	return encode(r)
}
func clipDecode(s string) (core.ClipBatch, error) {
	var r clipRecord
	e := json.Unmarshal([]byte(s), &r)
	r.Batch.SourcePath = r.SourcePath
	for i := range r.Batch.Tasks {
		if i < len(r.Candidates) {
			r.Batch.Tasks[i].Candidate = r.Candidates[i]
		}
	}
	return r.Batch, e
}
func clipConflict() error {
	return &core.Failure{Code: core.CodeConflict, Message: "clip revision or idempotency conflict"}
}
func (r *Repository) SaveClipPlan(ctx context.Context, p core.ClipPlan) error {
	s, e := encode(p)
	if e != nil {
		return e
	}
	_, e = r.db.ExecContext(ctx, "INSERT INTO clip_plans(id,record) VALUES(?,?)", p.ID, s)
	return e
}
func (r *Repository) ClipPlan(ctx context.Context, id string) (core.ClipPlan, error) {
	return decodeRow[core.ClipPlan](r.db.QueryRowContext(ctx, "SELECT record FROM clip_plans WHERE id=?", id))
}
func (r *Repository) ClipBatch(ctx context.Context, id string) (core.ClipBatch, error) {
	var s string
	e := r.db.QueryRowContext(ctx, "SELECT record FROM clip_batches WHERE id=?", id).Scan(&s)
	if e != nil {
		return core.ClipBatch{}, e
	}
	return clipDecode(s)
}
func (r *Repository) AdmitClip(ctx context.Context, b core.ClipBatch) (core.ClipBatch, error) {
	if e := core.ValidateClipBatch(b); e != nil {
		return b, e
	}
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return b, e
	}
	defer tx.Rollback()
	var s, d string
	e = tx.QueryRowContext(ctx, "SELECT record,digest FROM clip_batches WHERE admission_key=?", b.Key).Scan(&s, &d)
	if e == nil {
		if d != b.Plan.Digest {
			return b, clipConflict()
		}
		return clipDecode(s)
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return b, e
	}
	if !time.Now().Before(b.Plan.Expires) {
		return b, &core.Failure{Code: core.CodeValidation, Message: "clip plan expired; review a new plan"}
	}
	var pending int
	if e = tx.QueryRowContext(ctx, "SELECT COALESCE(SUM(json_array_length(json_extract(record,'$.batch.tasks'))),0) FROM clip_batches WHERE settled=0").Scan(&pending); e != nil {
		return b, e
	}
	if pending+len(b.Tasks) > 1000 {
		return b, fmt.Errorf("workspace clip capacity reached (1000 unfinished tasks)")
	}
	s, e = clipEncode(b)
	if e != nil {
		return b, e
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO clip_batches(id,admission_key,digest,revision,settled,paused,record) VALUES(?,?,?,?,?,?,?)", b.ID, b.Key, b.Plan.Digest, b.Revision, b.Settled(), b.Paused, s)
	if e != nil {
		return b, e
	}
	return b, tx.Commit()
}
func saveClipTx(ctx context.Context, tx *sql.Tx, b core.ClipBatch, expected int64) error {
	if e := core.ValidateClipBatch(b); e != nil {
		return e
	}
	if b.Revision != expected+1 {
		return clipConflict()
	}
	var previous string
	if e := tx.QueryRowContext(ctx, "SELECT record FROM clip_batches WHERE id=? AND revision=? AND digest=? AND admission_key=?", b.ID, expected, b.Plan.Digest, b.Key).Scan(&previous); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			return clipConflict()
		}
		return e
	}
	old, e := clipDecode(previous)
	if e != nil {
		return e
	}
	if len(old.Tasks) != len(b.Tasks) || core.ClipDigest(old.Plan) != core.ClipDigest(b.Plan) {
		return clipConflict()
	}
	for i, t := range old.Tasks {
		if t.ID != b.Tasks[i].ID || t.Segment != b.Tasks[i].Segment || t.State == core.ClipDone && b.Tasks[i].State != core.ClipDone {
			return clipConflict()
		}
	}
	s, e := clipEncode(b)
	if e != nil {
		return e
	}
	result, e := tx.ExecContext(ctx, "UPDATE clip_batches SET revision=?,settled=?,paused=?,record=? WHERE id=? AND revision=?", b.Revision, b.Settled(), b.Paused, s, b.ID, expected)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e == nil && n != 1 {
		return clipConflict()
	}
	return e
}
func (r *Repository) SaveClip(ctx context.Context, b core.ClipBatch, expected int64) error {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = saveClipTx(ctx, tx, b, expected); e != nil {
		return e
	}
	return tx.Commit()
}
func (r *Repository) ClipBatches(ctx context.Context, limit int, cursor string) (core.ClipBatchPage, error) {
	p := core.ClipBatchPage{Items: []core.ClipBatch{}}
	if limit < 1 || limit > 200 {
		return p, fmt.Errorf("limit must be 1..200")
	}
	var after int64
	if cursor != "" {
		var err error
		after, err = strconv.ParseInt(cursor, 10, 64)
		if err != nil || after < 0 {
			return p, fmt.Errorf("invalid cursor")
		}
	}
	tx, e := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return p, e
	}
	defer tx.Rollback()
	if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM clip_batches").Scan(&p.Total); e != nil {
		return p, e
	}
	rows, e := tx.QueryContext(ctx, "SELECT seq,record FROM clip_batches WHERE seq>? ORDER BY seq LIMIT ?", after, limit+1)
	if e != nil {
		return p, e
	}
	defer rows.Close()
	var last int64
	for rows.Next() {
		var seq int64
		var s string
		if e = rows.Scan(&seq, &s); e != nil {
			return p, e
		}
		if len(p.Items) == limit {
			p.NextCursor = fmt.Sprint(last)
			break
		}
		b, e := clipDecode(s)
		if e != nil {
			return p, e
		}
		p.Items = append(p.Items, b)
		last = seq
	}
	return p, rows.Err()
}
func (r *Repository) NextClip(ctx context.Context, only string) (core.ClipBatch, error) {
	var s string
	var e error
	if only != "" {
		e = r.db.QueryRowContext(ctx, "SELECT record FROM clip_batches WHERE id=? AND settled=0 AND paused=0", only).Scan(&s)
	} else {
		e = r.db.QueryRowContext(ctx, "SELECT record FROM clip_batches WHERE settled=0 AND paused=0 ORDER BY seq LIMIT 1").Scan(&s)
	}
	if e != nil {
		return core.ClipBatch{}, e
	}
	return clipDecode(s)
}
func (r *Repository) CommitClip(ctx context.Context, b core.ClipBatch, expected int64, index int, a core.Asset) error {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	// Never adopt a path belonging to an unrelated asset.
	var old string
	e = tx.QueryRowContext(ctx, "SELECT id FROM assets WHERE path=?", a.Path).Scan(&old)
	if e == nil && old != a.ID {
		return clipConflict()
	}
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if _, e = putAssetTx(ctx, tx, a); e != nil {
		return e
	}
	t := b.Tasks[index]
	prov := application.ClipProvenance{TaskID: t.ID, BatchID: b.ID, Source: b.Prepared, Segment: t.Segment, Evidence: *t.Evidence}
	record, e := encode(prov)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO clip_provenance(asset_id,task_id,record) VALUES(?,?,?) ON CONFLICT(task_id) DO NOTHING", a.ID, t.ID, record); e != nil {
		return e
	}
	for _, p := range b.Plan.Destinations {
		d := application.ClipDelivery{ID: core.ClipDigest([]string{a.ID, p.ID}), AssetID: a.ID, Destination: p, State: "pending"}
		s, e := encode(d)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO clip_deliveries(id,asset_id,profile_id,state,record) VALUES(?,?,?,?,?)", d.ID, a.ID, p.ID, d.State, s); e != nil {
			return e
		}
	}
	if e = saveClipTx(ctx, tx, b, expected); e != nil {
		return e
	}
	return tx.Commit()
}
func (r *Repository) ClipDeliveries(ctx context.Context) ([]application.ClipDelivery, error) {
	rows, e := r.db.QueryContext(ctx, "SELECT record FROM clip_deliveries WHERE state!='enqueued' LIMIT 200")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []application.ClipDelivery{}
	for rows.Next() {
		var s string
		var d application.ClipDelivery
		if e = rows.Scan(&s); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(s), &d); e != nil {
			return nil, e
		}
		v = append(v, d)
	}
	return v, rows.Err()
}
func (r *Repository) SaveClipDelivery(ctx context.Context, d application.ClipDelivery) error {
	s, e := encode(d)
	if e != nil {
		return e
	}
	_, e = r.db.ExecContext(ctx, "UPDATE clip_deliveries SET state=?,record=? WHERE id=?", d.State, s, d.ID)
	return e
}
func (r *Repository) ClipProvenance(ctx context.Context, id string) ([]application.ClipProvenance, error) {
	rows, e := r.db.QueryContext(ctx, "SELECT record FROM clip_provenance WHERE task_id=? OR asset_id=?", id, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []application.ClipProvenance{}
	for rows.Next() {
		var s string
		var p application.ClipProvenance
		if e = rows.Scan(&s); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(s), &p); e != nil {
			return nil, e
		}
		v = append(v, p)
	}
	return v, rows.Err()
}

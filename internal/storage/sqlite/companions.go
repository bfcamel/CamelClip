// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
)

const companionSchema = `
CREATE TABLE IF NOT EXISTS companion_catalogs(id TEXT PRIMARY KEY,scope TEXT NOT NULL,record TEXT NOT NULL,request TEXT NOT NULL,auth_session TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS companion_blobs(id TEXT PRIMARY KEY,scope TEXT NOT NULL,bytes INTEGER NOT NULL,used INTEGER NOT NULL,record TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS companion_refs(owner TEXT NOT NULL,blob_id TEXT NOT NULL REFERENCES companion_blobs(id) ON DELETE CASCADE,role TEXT NOT NULL,kept INTEGER NOT NULL,PRIMARY KEY(owner,blob_id,role));
CREATE INDEX IF NOT EXISTS companion_refs_blob ON companion_refs(blob_id,kept);
CREATE TABLE IF NOT EXISTS companion_captions(id TEXT PRIMARY KEY,catalog_id TEXT NOT NULL,raw_id TEXT NOT NULL,cues_id TEXT NOT NULL,record TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS companion_captions_catalog ON companion_captions(catalog_id);
`

func (r *Repository) SaveCompanionCatalog(ctx context.Context, c core.CompanionCatalog) error {
	v, e := encode(c)
	if e != nil {
		return e
	}
	req, e := encode(c.Request)
	if e != nil {
		return e
	}
	_, e = r.db.ExecContext(ctx, "INSERT INTO companion_catalogs(id,scope,record,request,auth_session) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET record=excluded.record", c.ID, c.Scope, v, req, c.AuthSession)
	return e
}
func (r *Repository) CompanionCatalog(ctx context.Context, id string) (core.CompanionCatalog, error) {
	var c core.CompanionCatalog
	var v, req string
	e := r.db.QueryRowContext(ctx, "SELECT record,request,scope,auth_session FROM companion_catalogs WHERE id=?", id).Scan(&v, &req, &c.Scope, &c.AuthSession)
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal([]byte(v), &c); e != nil {
		return c, e
	}
	e = json.Unmarshal([]byte(req), &c.Request)
	return c, e
}
func (r *Repository) SaveCompanionBlob(ctx context.Context, b core.CompanionBlob) error {
	v, e := encode(b)
	if e != nil {
		return e
	}
	_, e = r.db.ExecContext(ctx, "INSERT INTO companion_blobs(id,scope,bytes,used,record) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET used=excluded.used,record=excluded.record", b.ID, b.Scope, b.Size, b.Used.Unix(), v)
	return e
}
func (r *Repository) CompanionBlob(ctx context.Context, id string) (core.CompanionBlob, error) {
	var b core.CompanionBlob
	var v string
	e := r.db.QueryRowContext(ctx, "SELECT record,scope FROM companion_blobs WHERE id=?", id).Scan(&v, &b.Scope)
	if e != nil {
		return b, e
	}
	e = json.Unmarshal([]byte(v), &b)
	return b, e
}
func (r *Repository) LinkCompanion(ctx context.Context, owner, id, role string, kept bool) error {
	_, e := r.db.ExecContext(ctx, "INSERT INTO companion_refs(owner,blob_id,role,kept) VALUES(?,?,?,?) ON CONFLICT(owner,blob_id,role) DO UPDATE SET kept=excluded.kept", owner, id, role, kept)
	return e
}
func (r *Repository) CompanionLinks(ctx context.Context, owner string) ([]core.CompanionBlob, error) {
	rows, e := r.db.QueryContext(ctx, "SELECT DISTINCT b.record,b.scope FROM companion_blobs b JOIN companion_refs f ON f.blob_id=b.id WHERE f.owner=? ORDER BY b.id", owner)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []core.CompanionBlob{}
	for rows.Next() {
		var b core.CompanionBlob
		var s string
		if e = rows.Scan(&s, &b.Scope); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(s), &b); e != nil {
			return nil, e
		}
		v = append(v, b)
	}
	return v, rows.Err()
}
func (r *Repository) SaveCaption(ctx context.Context, s core.CaptionSnapshot) error {
	v, e := encode(s)
	if e != nil {
		return e
	}
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO companion_captions(id,catalog_id,raw_id,cues_id,record) VALUES(?,?,?,?,?)", s.ID, s.CatalogID, s.RawID, s.CuesID, v); e != nil {
		return e
	}
	for _, id := range []string{s.RawID, s.CuesID} {
		if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO companion_refs(owner,blob_id,role,kept) VALUES(?,?,?,0)", s.CatalogID, id, "caption"); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (r *Repository) Caption(ctx context.Context, id string) (core.CaptionSnapshot, error) {
	return decodeRow[core.CaptionSnapshot](r.db.QueryRowContext(ctx, "SELECT record FROM companion_captions WHERE id=?", id))
}
func (r *Repository) Captions(ctx context.Context, id string) ([]core.CaptionSnapshot, error) {
	rows, e := r.db.QueryContext(ctx, "SELECT record FROM companion_captions WHERE catalog_id=? ORDER BY rowid DESC LIMIT 200", id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []core.CaptionSnapshot{}
	for rows.Next() {
		var s string
		var c core.CaptionSnapshot
		if e = rows.Scan(&s); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(s), &c); e != nil {
			return nil, e
		}
		v = append(v, c)
	}
	return v, rows.Err()
}
func (r *Repository) CompanionCache(ctx context.Context) ([]application.CompanionCacheEntry, error) {
	rows, e := r.db.QueryContext(ctx, "SELECT b.record,b.scope,EXISTS(SELECT 1 FROM companion_refs f WHERE f.blob_id=b.id AND f.kept=1) FROM companion_blobs b ORDER BY b.used,b.id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []application.CompanionCacheEntry{}
	for rows.Next() {
		var item application.CompanionCacheEntry
		var s string
		if e = rows.Scan(&s, &item.Blob.Scope, &item.Kept); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(s), &item.Blob); e != nil {
			return nil, e
		}
		v = append(v, item)
	}
	return v, rows.Err()
}
func (r *Repository) DeleteCompanionBlob(ctx context.Context, id string) error {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var kept bool
	if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM companion_refs WHERE blob_id=? AND kept=1)", id).Scan(&kept); e != nil {
		return e
	}
	if kept {
		return errors.New("kept companions cannot be evicted")
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM companion_captions WHERE raw_id=? OR cues_id=?", id, id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM companion_blobs WHERE id=?", id); e != nil {
		return e
	}
	return tx.Commit()
}

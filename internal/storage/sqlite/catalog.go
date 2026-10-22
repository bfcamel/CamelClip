// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
)

const catalogSchema = `
CREATE TABLE IF NOT EXISTS assets(id TEXT PRIMARY KEY,path TEXT NOT NULL UNIQUE,search_text TEXT NOT NULL,kind TEXT NOT NULL,status TEXT NOT NULL,container TEXT NOT NULL,height INTEGER NOT NULL,added INTEGER NOT NULL,record TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS assets_status ON assets(status,kind,added);
CREATE TABLE IF NOT EXISTS asset_exclusions(path TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS asset_sources(asset_id TEXT NOT NULL,job_id INTEGER NOT NULL,request TEXT NOT NULL,media TEXT NOT NULL,PRIMARY KEY(asset_id,job_id));
CREATE TABLE IF NOT EXISTS s3_profiles(id TEXT PRIMARY KEY,name_fold TEXT NOT NULL UNIQUE,record TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS s3_profile_secrets(profile_id TEXT PRIMARY KEY REFERENCES s3_profiles(id) ON DELETE CASCADE,record BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS s3_secret_store(id INTEGER PRIMARY KEY CHECK(id=1),record BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS s3_upload_jobs(id TEXT PRIMARY KEY,asset_id TEXT NOT NULL,profile_id TEXT NOT NULL,state TEXT NOT NULL,identity TEXT NOT NULL UNIQUE,record TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS uploads_state ON s3_upload_jobs(profile_id,state);
CREATE TABLE IF NOT EXISTS publication_outbox(job_id INTEGER NOT NULL,asset_id TEXT NOT NULL,path TEXT NOT NULL,PRIMARY KEY(job_id,path));
`

func encode(v any) (string, error) { b, e := json.Marshal(v); return string(b), e }
func decodeRow[T any](row *sql.Row) (v T, err error) {
	var b string
	if err = row.Scan(&b); err != nil {
		return
	}
	err = json.Unmarshal([]byte(b), &v)
	return
}
func (r *Repository) Asset(ctx context.Context, id string) (core.Asset, error) {
	return decodeRow[core.Asset](r.db.QueryRowContext(ctx, "SELECT record FROM assets WHERE id=?", id))
}
func putAssetTx(ctx context.Context, tx *sql.Tx, a core.Asset) (core.Asset, error) {
	var excluded int
	if e := tx.QueryRowContext(ctx, "SELECT 1 FROM asset_exclusions WHERE path=?", a.Path).Scan(&excluded); e == nil {
		return core.Asset{}, errors.New("file excluded from index")
	}
	old, e := decodeRow[core.Asset](tx.QueryRowContext(ctx, "SELECT record FROM assets WHERE path=?", a.Path))
	if e == nil {
		a.ID = old.ID
		a.Added = old.Added
		if a.SourceURL == "" {
			a.SourceURL = old.SourceURL
			a.DownloadID = old.DownloadID
			a.Title = old.Title
		}
		if a.SHA256 == "" && a.Size == old.Size && a.Modified.Equal(old.Modified) {
			a.SHA256 = old.SHA256
		}
	} else if !errors.Is(e, sql.ErrNoRows) {
		return a, e
	}
	if a.ID == "" {
		a.ID = core.NewID()
	}
	if a.Added.IsZero() {
		a.Added = time.Now()
	}
	b, e := encode(a)
	if e != nil {
		return a, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO assets(id,path,search_text,kind,status,container,height,added,record) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET search_text=excluded.search_text,kind=excluded.kind,status=excluded.status,container=excluded.container,height=excluded.height,record=excluded.record`, a.ID, a.Path, strings.ToLower(a.Title+" "+filepath.Base(a.Path)+" "+a.SourceURL), a.Kind, a.Status, a.Container, a.Height, a.Added.Unix(), b)
	return a, e
}
func (r *Repository) PutAsset(ctx context.Context, a core.Asset) (core.Asset, error) {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return a, e
	}
	defer tx.Rollback()
	a, e = putAssetTx(ctx, tx, a)
	if e != nil {
		return a, e
	}
	return a, tx.Commit()
}
func (r *Repository) Assets(ctx context.Context, q core.AssetQuery) (core.AssetPage, error) {
	where := " WHERE 1=1"
	args := []any{}
	add := func(sql string, v any) { where += " AND " + sql; args = append(args, v) }
	if q.NeedsProbe {
		where += " AND json_extract(record,'$.needs_probe')=1"
	}
	if q.Search != "" {
		term := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.ToLower(q.Search))
		add("search_text LIKE ? ESCAPE '\\'", "%"+term+"%")
	}
	if q.Kind != "" {
		add("kind=?", q.Kind)
	}
	if q.Status != "" {
		add("status=?", q.Status)
	}
	if q.Container != "" {
		add("container=?", q.Container)
	}
	if q.MinHeight > 0 {
		add("height>=?", q.MinHeight)
	}
	if !q.Since.IsZero() {
		add("added>=?", q.Since.Unix())
	}
	if !q.Until.IsZero() {
		add("added<=?", q.Until.Unix())
	}
	if q.HasCopies != nil {
		condition := "EXISTS"
		if !*q.HasCopies {
			condition = "NOT EXISTS"
		}
		where += " AND " + condition + " (SELECT 1 FROM s3_upload_jobs u WHERE u.asset_id=assets.id AND u.state='done')"
	}
	page := core.AssetPage{Items: []core.Asset{}}
	if e := r.db.QueryRowContext(ctx, "SELECT count(*) FROM assets"+where, args...).Scan(&page.Total); e != nil {
		return page, e
	}
	order := "added DESC,id"
	switch q.Sort {
	case "name":
		order = "search_text,id"
	case "size":
		order = "CAST(json_extract(record,'$.size') AS INTEGER) DESC,id"
	case "duration":
		order = "CAST(json_extract(record,'$.duration') AS REAL) DESC,id"
	}
	args = append(args, min(max(q.Limit, 1), 200), max(q.Offset, 0))
	rows, e := r.db.QueryContext(ctx, "SELECT record,(SELECT count(*) FROM s3_upload_jobs u WHERE u.asset_id=assets.id AND u.state='done') FROM assets"+where+" ORDER BY "+order+" LIMIT ? OFFSET ?", args...)
	if e != nil {
		return page, e
	}
	defer rows.Close()
	for rows.Next() {
		var a core.Asset
		var b string
		var copies int
		if e = rows.Scan(&b, &copies); e != nil {
			return page, e
		}
		if e = json.Unmarshal([]byte(b), &a); e != nil {
			return page, e
		}
		a.RemoteCopies = copies
		page.Items = append(page.Items, a)
	}
	return page, rows.Err()
}
func (r *Repository) ForgetAsset(ctx context.Context, id string) error {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var path string
	if e = tx.QueryRowContext(ctx, "SELECT path FROM assets WHERE id=?", id).Scan(&path); e != nil {
		return e
	}
	var n int
	if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM s3_upload_jobs WHERE asset_id=? AND state NOT IN ('done','failed','canceled')", id).Scan(&n); e != nil {
		return e
	}
	if n > 0 {
		return errors.New("cancel pending uploads before removing asset")
	}
	if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO asset_exclusions(path) VALUES(?)", path); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM assets WHERE id=?", id); e != nil {
		return e
	}
	return tx.Commit()
}
func (r *Repository) MarkMissing(ctx context.Context, root string, seen []string, recursive bool) (int, error) {
	known := map[string]bool{}
	for _, p := range seen {
		known[p] = true
	}
	rows, e := r.db.QueryContext(ctx, "SELECT record FROM assets")
	if e != nil {
		return 0, e
	}
	items := []core.Asset{}
	for rows.Next() {
		var b string
		var a core.Asset
		if e = rows.Scan(&b); e != nil {
			rows.Close()
			return 0, e
		}
		if e = json.Unmarshal([]byte(b), &a); e != nil {
			rows.Close()
			return 0, e
		}
		rel, err := filepath.Rel(root, a.Path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && (recursive || filepath.Dir(a.Path) == root) && !known[a.Path] {
			items = append(items, a)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	n := 0
	for _, a := range items {
		if a.Status != "missing" {
			if _, err := os.Lstat(a.Path); !os.IsNotExist(err) {
				continue
			}
			a.Status = "missing"
			if _, e = r.PutAsset(ctx, a); e != nil {
				return n, e
			}
			n++
		}
	}
	return n, nil
}
func profilesTx(ctx context.Context, tx *sql.Tx) ([]core.S3Profile, error) {
	rows, e := tx.QueryContext(ctx, "SELECT record FROM s3_profiles ORDER BY name_fold")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	list := []core.S3Profile{}
	for rows.Next() {
		var p core.S3Profile
		var b string
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &p); e != nil {
			return nil, e
		}
		list = append(list, p)
	}
	return list, rows.Err()
}
func (r *Repository) Profiles(ctx context.Context) ([]core.S3Profile, error) {
	tx, e := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	return profilesTx(ctx, tx)
}
func (r *Repository) Profile(ctx context.Context, id string) (core.S3Profile, error) {
	return decodeRow[core.S3Profile](r.db.QueryRowContext(ctx, "SELECT record FROM s3_profiles WHERE id=?", id))
}
func sameDestination(a, b core.S3Profile) bool {
	return a.Endpoint == b.Endpoint && a.Region == b.Region && a.Bucket == b.Bucket && a.Prefix == b.Prefix && a.Style == b.Style && a.KeyTemplate == b.KeyTemplate
}
func (r *Repository) SaveProfile(ctx context.Context, p core.S3Profile, secret *core.SealedSecret) error {
	if e := core.ValidateProfile(p); e != nil {
		return e
	}
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	old, err := decodeRow[core.S3Profile](tx.QueryRowContext(ctx, "SELECT record FROM s3_profiles WHERE id=?", p.ID))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		p.Created = old.Created
		p.Revision = old.Revision + 1
		if !sameDestination(old, p) {
			p.ConditionalVerified = false
		}
	} else {
		if secret == nil {
			return errors.New("new profile requires credentials")
		}
		p.Created = time.Now()
		p.Revision = 1
	}
	p.Updated = time.Now()
	b, e := encode(p)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO s3_profiles(id,name_fold,record) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET name_fold=excluded.name_fold,record=excluded.record", p.ID, strings.ToLower(strings.TrimSpace(p.Name)), b); e != nil {
		return errors.New("cannot save profile; name may already exist")
	}
	if secret != nil {
		data, e := encode(secret)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO s3_profile_secrets(profile_id,record) VALUES(?,?) ON CONFLICT(profile_id) DO UPDATE SET record=excluded.record", p.ID, data); e != nil {
			return e
		}
	}
	if err == nil && !sameDestination(old, p) {
		jobs, e := uploadsTx(ctx, tx, 100000, 0)
		if e != nil {
			return e
		}
		for _, j := range jobs {
			if j.ProfileID == p.ID && !j.Terminal() {
				if j.State == "uploading" || j.State == "remote_check" {
					return errors.New("cancel active uploads before changing destination")
				}
				j.State = "blocked_profile_changed"
				j.ErrorCode = "profile_changed"
				if e = saveUploadTx(ctx, tx, j, false); e != nil {
					return e
				}
			}
		}
	}
	return tx.Commit()
}
func (r *Repository) Secret(ctx context.Context, id string) (core.SealedSecret, error) {
	return decodeRow[core.SealedSecret](r.db.QueryRowContext(ctx, "SELECT record FROM s3_profile_secrets WHERE profile_id=?", id))
}
func (r *Repository) DeleteProfile(ctx context.Context, id string) error {
	var n int
	if e := r.db.QueryRowContext(ctx, "SELECT count(*) FROM s3_upload_jobs WHERE profile_id=? AND (state NOT IN ('done','failed','canceled') OR COALESCE(json_extract(record,'$.multipart_id'),'')!='')", id).Scan(&n); e != nil {
		return e
	}
	if n > 0 {
		return errors.New("cancel or reassign unfinished uploads first")
	}
	_, e := r.db.ExecContext(ctx, "DELETE FROM s3_profiles WHERE id=?", id)
	return e
}
func (r *Repository) VaultState(ctx context.Context) (core.VaultState, error) {
	v, e := decodeRow[core.VaultState](r.db.QueryRowContext(ctx, "SELECT record FROM s3_secret_store WHERE id=1"))
	if errors.Is(e, sql.ErrNoRows) {
		e = application.ErrVaultUnconfigured
	}
	return v, e
}
func (r *Repository) SetVault(ctx context.Context, v core.VaultState, secrets map[string]core.SealedSecret) error {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	b, e := encode(v)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO s3_secret_store(id,record) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET record=excluded.record", b); e != nil {
		return e
	}
	for id, s := range secrets {
		b, e := encode(s)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE s3_profile_secrets SET record=? WHERE profile_id=?", b, id); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (r *Repository) ResetProfiles(ctx context.Context) error {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "UPDATE s3_upload_jobs SET state='canceled',record=json_set(record,'$.state','canceled') WHERE state NOT IN ('done','failed','canceled')"); e != nil {
		return e
	}
	for _, q := range []string{"DELETE FROM s3_profile_secrets", "DELETE FROM s3_profiles", "DELETE FROM s3_secret_store"} {
		if _, e = tx.ExecContext(ctx, q); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func uploadsTx(ctx context.Context, tx *sql.Tx, limit, offset int) ([]core.UploadJob, error) {
	rows, e := tx.QueryContext(ctx, "SELECT record FROM s3_upload_jobs ORDER BY rowid LIMIT ? OFFSET ?", limit, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []core.UploadJob{}
	for rows.Next() {
		var b string
		var j core.UploadJob
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &j); e != nil {
			return nil, e
		}
		v = append(v, j)
	}
	return v, rows.Err()
}
func (r *Repository) Uploads(ctx context.Context, limit, offset int) ([]core.UploadJob, error) {
	tx, e := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	return uploadsTx(ctx, tx, min(max(limit, 1), 100000), max(offset, 0))
}
func (r *Repository) Upload(ctx context.Context, id string) (core.UploadJob, error) {
	return decodeRow[core.UploadJob](r.db.QueryRowContext(ctx, "SELECT record FROM s3_upload_jobs WHERE id=?", id))
}
func uploadIdentity(j core.UploadJob) string {
	return fmt.Sprintf("%s\x00%d\x00%d\x00%s\x00%s\x00%s\x00%s\x00%s", j.AssetID, j.Size, j.Modified.UnixNano(), j.ProfileID, j.Profile.Endpoint, j.Profile.Region, j.Profile.Bucket, j.Key)
}
func saveUploadTx(ctx context.Context, tx *sql.Tx, j core.UploadJob, insert bool) error {
	if !j.State.Valid() {
		return core.ContractError("invalid upload state")
	}
	persisted := j
	persisted.Metrics = core.Metrics{}
	b, e := encode(persisted)
	if e != nil {
		return e
	}
	if insert {
		_, e = tx.ExecContext(ctx, "INSERT INTO s3_upload_jobs(id,asset_id,profile_id,state,identity,record) VALUES(?,?,?,?,?,?)", j.ID, j.AssetID, j.ProfileID, j.State, uploadIdentity(j), b)
	} else {
		_, e = tx.ExecContext(ctx, "UPDATE s3_upload_jobs SET state=?,identity=?,record=? WHERE id=?", j.State, uploadIdentity(j), b, j.ID)
	}
	return e
}
func (r *Repository) SaveUpload(ctx context.Context, j core.UploadJob) error {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = saveUploadTx(ctx, tx, j, false); e != nil {
		return e
	}
	return tx.Commit()
}
func (r *Repository) EnqueueUpload(ctx context.Context, j core.UploadJob) (core.UploadJob, error) {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return j, e
	}
	defer tx.Rollback()
	old, err := decodeRow[core.UploadJob](tx.QueryRowContext(ctx, "SELECT record FROM s3_upload_jobs WHERE identity=?", uploadIdentity(j)))
	if err == nil {
		return old, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return j, err
	}
	if e = saveUploadTx(ctx, tx, j, true); e != nil {
		return j, e
	}
	return j, tx.Commit()
}

// Publication and automatic upload creation are part of the queue's transaction.
func reconcilePublishedTx(ctx context.Context, tx *sql.Tx, path string, title string, url string, jobID int, mode string, duration float64) error {
	var done int
	if e := tx.QueryRowContext(ctx, "SELECT 1 FROM publication_outbox WHERE job_id=? AND path=?", jobID, path).Scan(&done); e == nil {
		return nil
	}
	var id string
	e := tx.QueryRowContext(ctx, "SELECT id FROM assets WHERE path=?", path).Scan(&id)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	var ignored int
	if tx.QueryRowContext(ctx, "SELECT 1 FROM asset_exclusions WHERE path=?", path).Scan(&ignored) == nil {
		return nil
	}
	st, e := os.Lstat(path)
	if e != nil || !st.Mode().IsRegular() {
		return nil
	}
	a := core.Asset{NeedsProbe: true, LocalVerification: "full_decode_verified", ID: id, Path: path, Title: title, Size: st.Size(), Modified: st.ModTime(), Kind: mode, Container: strings.TrimPrefix(filepath.Ext(path), "."), SourceURL: url, DownloadID: jobID, Duration: duration, Status: "available"}
	a, e = putAssetTx(ctx, tx, a)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO publication_outbox(job_id,asset_id,path) VALUES(?,?,?)", jobID, a.ID, path); e != nil {
		return e
	}
	profiles, e := profilesTx(ctx, tx)
	if e != nil {
		return e
	}
	for _, p := range profiles {
		if !p.Enabled || !p.AutoUpload {
			continue
		}
		j := core.UploadJob{ID: core.NewID(), AssetID: a.ID, ProfileID: p.ID, Profile: p, Key: core.DestinationKey(p, a), State: "waiting_unlock", Size: a.Size, Modified: a.Modified, Created: time.Now(), Updated: time.Now(), Auto: true, Verification: "uploaded_unverified"}
		if e = saveUploadTx(ctx, tx, j, true); e != nil {
			return e
		}
	}
	return nil
}

func (r *Repository) CanDeleteAsset(ctx context.Context, id string) error {
	var n int
	if e := r.db.QueryRowContext(ctx, "SELECT count(*) FROM s3_upload_jobs WHERE asset_id=? AND state NOT IN ('done','failed','canceled')", id).Scan(&n); e != nil {
		return e
	}
	if n > 0 {
		return errors.New("cancel pending uploads before deleting source")
	}
	return nil
}

func (r *Repository) RecoverUploads(ctx context.Context) error {
	_, e := r.db.ExecContext(ctx, `UPDATE s3_upload_jobs SET state='waiting_unlock',record=json_set(record,'$.state','waiting_unlock') WHERE state IN ('uploading','remote_check','canceling')`)
	return e
}
func (r *Repository) PendingUploads(ctx context.Context, limit int, allowed []string) ([]core.UploadJob, error) {
	query := `SELECT u.record FROM s3_upload_jobs u JOIN s3_profiles p ON p.id=u.profile_id WHERE u.state IN ('queued','waiting_unlock','waiting_slot','paused') AND json_extract(p.record,'$.enabled')=1 AND COALESCE(julianday(json_extract(u.record,'$.next_attempt')),0)<=julianday('now')`
	args := []any{}
	if allowed != nil {
		if len(allowed) == 0 {
			return []core.UploadJob{}, nil
		}
		// JSON keeps the number of SQL parameters bounded for large selections.
		ids, err := json.Marshal(allowed)
		if err != nil {
			return nil, err
		}
		query += " AND u.id IN (SELECT value FROM json_each(?))"
		args = append(args, string(ids))
	}
	args = append(args, min(max(limit, 1), 128))
	rows, e := r.db.QueryContext(ctx, query+" ORDER BY u.rowid LIMIT ?", args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []core.UploadJob{}
	for rows.Next() {
		var b string
		var j core.UploadJob
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &j); e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (r *Repository) AllowAsset(ctx context.Context, path string) error {
	_, e := r.db.ExecContext(ctx, "DELETE FROM asset_exclusions WHERE path=?", path)
	return e
}

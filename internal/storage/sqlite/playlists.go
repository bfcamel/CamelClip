// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
	"path"
	"path/filepath"
	"time"
)

const playlistSchema = `
CREATE TABLE IF NOT EXISTS playlist_subscriptions(id TEXT PRIMARY KEY,playlist_id TEXT NOT NULL UNIQUE,enabled INTEGER NOT NULL,next_due INTEGER NOT NULL,last_admission INTEGER NOT NULL DEFAULT 0,scan_id TEXT NOT NULL DEFAULT '',record TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS playlist_due ON playlist_subscriptions(enabled,next_due);
CREATE TABLE IF NOT EXISTS playlist_scans(id TEXT PRIMARY KEY,subscription_id TEXT NOT NULL,revision INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS playlist_observations(scan_id TEXT NOT NULL,video_id TEXT NOT NULL,title TEXT NOT NULL,PRIMARY KEY(scan_id,video_id));
CREATE TABLE IF NOT EXISTS playlist_members(subscription_id TEXT NOT NULL,video_id TEXT NOT NULL,title TEXT NOT NULL,eligible INTEGER NOT NULL,admission_error TEXT NOT NULL DEFAULT '',job_id INTEGER NOT NULL DEFAULT 0,materialization_key TEXT NOT NULL DEFAULT '',first_seen INTEGER NOT NULL,last_seen INTEGER NOT NULL,PRIMARY KEY(subscription_id,video_id));
CREATE INDEX IF NOT EXISTS playlist_pending ON playlist_members(subscription_id,eligible,job_id,first_seen);
CREATE TABLE IF NOT EXISTS playlist_materializations(key TEXT PRIMARY KEY,job_id INTEGER NOT NULL UNIQUE,video_id TEXT NOT NULL,asset_id TEXT NOT NULL DEFAULT '',record TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS playlist_deliveries(materialization_key TEXT NOT NULL,profile_id TEXT NOT NULL,destination_key TEXT NOT NULL,profile TEXT NOT NULL,upload_id TEXT NOT NULL DEFAULT '',PRIMARY KEY(materialization_key,destination_key));
CREATE TABLE IF NOT EXISTS playlist_delivery_bindings(subscription_id TEXT NOT NULL,video_id TEXT NOT NULL,profile_id TEXT NOT NULL,materialization_key TEXT NOT NULL,destination_key TEXT NOT NULL,PRIMARY KEY(subscription_id,video_id,profile_id));
`

func subscriptionTx(ctx context.Context, tx *sql.Tx, id string) (core.PlaylistSubscription, error) {
	return decodeRow[core.PlaylistSubscription](tx.QueryRowContext(ctx, "SELECT record FROM playlist_subscriptions WHERE id=?", id))
}
func saveSubscriptionTx(ctx context.Context, tx *sql.Tx, p core.PlaylistSubscription) error {
	b, e := encode(p)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO playlist_subscriptions(id,playlist_id,enabled,next_due,last_admission,record) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET enabled=excluded.enabled,next_due=excluded.next_due,record=excluded.record`, p.ID, p.PlaylistID, p.Enabled, p.NextScan.UnixNano(), p.LastAdmission.UnixNano(), b)
	return e
}
func (r *Repository) SaveSubscription(ctx context.Context, p core.PlaylistSubscription) (core.PlaylistSubscription, error) {
	p, e := core.ValidateSubscription(p)
	if e != nil {
		return p, e
	}
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return p, e
	}
	defer tx.Rollback()
	if p.ID == "" {
		var existing string
		err := tx.QueryRowContext(ctx, "SELECT id FROM playlist_subscriptions WHERE playlist_id=?", p.PlaylistID).Scan(&existing)
		if err == nil {
			return p, &core.Failure{Code: core.CodeConflict, Message: "playlist is already subscribed; edit the existing subscription"}
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return p, err
		}
		p.ID = core.NewID()
		p.Revision = 1
		p.Created = time.Now()
		p.Status = "pending"
		p.Initialized = false
		p.LastError = ""
		p.LastScan = time.Time{}
		p.Failures = 0
		p.Scanned = 0
	} else {
		old, e := subscriptionTx(ctx, tx, p.ID)
		if e != nil {
			return p, e
		}
		if old.Revision != p.Revision {
			return p, &core.Failure{Code: core.CodeConflict, Message: "subscription changed; reload before saving"}
		}
		if old.PlaylistID != p.PlaylistID || old.InitialMode != p.InitialMode {
			return p, errors.New("playlist identity and initial mode cannot be changed")
		}
		p.Revision++
		p.Created = old.Created
		p.Initialized = old.Initialized
		p.LastScan = old.LastScan
		p.Failures = old.Failures
		p.Scanned = old.Scanned
		p.Status = old.Status
		p.LastError = old.LastError
	}
	for _, id := range p.S3Profiles {
		if _, e = decodeRow[core.S3Profile](tx.QueryRowContext(ctx, "SELECT record FROM s3_profiles WHERE id=?", id)); e != nil {
			return p, fmt.Errorf("S3 profile unavailable: %w", e)
		}
	}
	p.NextScan = time.Now()
	if !p.Enabled {
		p.Status = "paused"
	} else if p.Status == "paused" {
		p.Status = "pending"
	}
	if e = saveSubscriptionTx(ctx, tx, p); e != nil {
		return p, e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE playlist_members SET admission_error='' WHERE subscription_id=? AND job_id=0", p.ID); e != nil {
		return p, e
	}
	return p, tx.Commit()
}
func (r *Repository) Subscription(ctx context.Context, id string) (core.PlaylistSubscription, error) {
	return decodeRow[core.PlaylistSubscription](r.db.QueryRowContext(ctx, "SELECT record FROM playlist_subscriptions WHERE id=?", id))
}
func (r *Repository) Subscriptions(ctx context.Context, limit, offset int) (core.PlaylistPage, error) {
	page := core.PlaylistPage{Items: []core.PlaylistSubscription{}}
	if e := r.db.QueryRowContext(ctx, "SELECT count(*) FROM playlist_subscriptions").Scan(&page.Total); e != nil {
		return page, e
	}
	rows, e := r.db.QueryContext(ctx, "SELECT record FROM playlist_subscriptions ORDER BY rowid LIMIT ? OFFSET ?", min(max(limit, 1), 200), max(offset, 0))
	if e != nil {
		return page, e
	}
	defer rows.Close()
	for rows.Next() {
		var b string
		var p core.PlaylistSubscription
		if e = rows.Scan(&b); e != nil {
			return page, e
		}
		if e = json.Unmarshal([]byte(b), &p); e != nil {
			return page, e
		}
		page.Items = append(page.Items, p)
	}
	return page, rows.Err()
}
func (r *Repository) RecoverPlaylistScans(ctx context.Context) error {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, q := range []string{`UPDATE playlist_subscriptions SET scan_id='',record=json_set(record,'$.status',CASE WHEN enabled=1 THEN 'pending' ELSE 'paused' END) WHERE scan_id!=''`, `DELETE FROM playlist_observations`, `DELETE FROM playlist_scans`} {
		if _, e = tx.ExecContext(ctx, q); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (r *Repository) DueSubscriptions(ctx context.Context, now time.Time, limit int) ([]core.PlaylistSubscription, error) {
	rows, e := r.db.QueryContext(ctx, "SELECT record FROM playlist_subscriptions WHERE enabled=1 AND scan_id='' AND next_due<=? ORDER BY next_due,rowid LIMIT ?", now.UnixNano(), min(max(limit, 1), 2))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []core.PlaylistSubscription{}
	for rows.Next() {
		var b string
		var p core.PlaylistSubscription
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &p); e != nil {
			return nil, e
		}
		items = append(items, p)
	}
	return items, rows.Err()
}
func (r *Repository) BeginPlaylistScan(ctx context.Context, id string, revision int) (string, error) {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	p, e := subscriptionTx(ctx, tx, id)
	if e != nil {
		return "", e
	}
	if !p.Enabled || p.Revision != revision {
		return "", errors.New("subscription paused or changed")
	}
	scan := core.NewID()
	result, e := tx.ExecContext(ctx, "UPDATE playlist_subscriptions SET scan_id=? WHERE id=? AND scan_id=''", scan, id)
	if e != nil {
		return "", e
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return "", errors.New("playlist scan already running")
	}
	p.Status = "scanning"
	p.Scanned = 0
	if e = saveSubscriptionTx(ctx, tx, p); e != nil {
		return "", e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO playlist_scans VALUES(?,?,?)", scan, id, revision); e != nil {
		return "", e
	}
	return scan, tx.Commit()
}
func (r *Repository) ObservePlaylist(ctx context.Context, scan string, entries []core.PlaylistEntry) error {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var id string
	if e = tx.QueryRowContext(ctx, "SELECT subscription_id FROM playlist_scans WHERE id=?", scan).Scan(&id); e != nil {
		return e
	}
	stmt, e := tx.PrepareContext(ctx, "INSERT INTO playlist_observations VALUES(?,?,?) ON CONFLICT(scan_id,video_id) DO UPDATE SET title=excluded.title")
	if e != nil {
		return e
	}
	defer stmt.Close()
	for _, entry := range entries {
		if !core.YouTubeVideoID(entry.ID) {
			return core.ContractError("invalid playlist video identity")
		}
		if _, e = stmt.ExecContext(ctx, scan, entry.ID, entry.Title); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE playlist_subscriptions SET record=json_set(record,'$.scanned',(SELECT count(*) FROM playlist_observations WHERE scan_id=?)) WHERE id=?`, scan, id); e != nil {
		return e
	}
	return tx.Commit()
}
func (r *Repository) FinishPlaylistScan(ctx context.Context, scan string, scanErr error) error {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var id string
	var revision int
	if e = tx.QueryRowContext(ctx, "SELECT subscription_id,revision FROM playlist_scans WHERE id=?", scan).Scan(&id, &revision); e != nil {
		return e
	}
	p, e := subscriptionTx(ctx, tx, id)
	if e != nil {
		return e
	}
	if p.Revision == revision && p.Enabled {
		if scanErr == nil {
			eligible := p.Initialized || p.InitialMode == "all_existing"
			now := time.Now()
			_, e = tx.ExecContext(ctx, `INSERT INTO playlist_members(subscription_id,video_id,title,eligible,first_seen,last_seen) SELECT ?,video_id,title,?,?,? FROM playlist_observations WHERE scan_id=? ON CONFLICT(subscription_id,video_id) DO UPDATE SET title=excluded.title,last_seen=excluded.last_seen`, id, eligible, now.UnixNano(), now.UnixNano(), scan)
			if e != nil {
				return e
			}
			p.Initialized = true
			p.LastScan = now
			p.Status = "watching"
			p.LastError = ""
			p.Failures = 0
			p.NextScan = now.Add(time.Duration(p.IntervalMinutes) * time.Minute)
		} else {
			p.Status = "error"
			p.LastError = scanErr.Error()
			if len(p.LastError) > 2000 {
				p.LastError = p.LastError[:2000]
			}
			p.Failures++
			p.NextScan = time.Now().Add(min(24*time.Hour, 5*time.Minute*time.Duration(1<<min(p.Failures-1, 8))))
		}
		if e = saveSubscriptionTx(ctx, tx, p); e != nil {
			return e
		}
	}
	for _, q := range []string{"DELETE FROM playlist_observations WHERE scan_id=?", "DELETE FROM playlist_scans WHERE id=?"} {
		if _, e = tx.ExecContext(ctx, q, scan); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, "UPDATE playlist_subscriptions SET scan_id='' WHERE id=? AND scan_id=?", id, scan); e != nil {
		return e
	}
	return tx.Commit()
}
func (r *Repository) SyncSubscription(ctx context.Context, id string) error {
	res, e := r.db.ExecContext(ctx, `UPDATE playlist_subscriptions SET next_due=0,record=json_set(record,'$.next_scan',?) WHERE id=? AND enabled=1`, time.Now().UTC().Format(time.RFC3339Nano), id)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("subscription missing or paused")
	}
	return nil
}
func (r *Repository) BackfillSubscription(ctx context.Context, id string) error {
	p, e := r.Subscription(ctx, id)
	if e != nil {
		return e
	}
	if !p.Initialized {
		return errors.New("wait for initial synchronization")
	}
	_, e = r.db.ExecContext(ctx, "UPDATE playlist_members SET eligible=1 WHERE subscription_id=? AND job_id=0", id)
	return e
}
func (r *Repository) PlaylistCandidates(ctx context.Context, limit int) ([]core.PlaylistItem, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT m.subscription_id,m.video_id FROM playlist_members m JOIN playlist_subscriptions s ON s.id=m.subscription_id WHERE s.enabled=1 AND m.eligible=1 AND m.job_id=0 AND m.admission_error='' AND m.video_id=(SELECT p.video_id FROM playlist_members p WHERE p.subscription_id=m.subscription_id AND p.eligible=1 AND p.job_id=0 AND p.admission_error='' ORDER BY p.first_seen,p.video_id LIMIT 1) ORDER BY s.last_admission,s.rowid LIMIT ?`, min(max(limit, 1), 10))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []core.PlaylistItem{}
	for rows.Next() {
		var v core.PlaylistItem
		if e = rows.Scan(&v.SubscriptionID, &v.VideoID); e != nil {
			return nil, e
		}
		items = append(items, v)
	}
	return items, rows.Err()
}
func (r *Repository) AdmitPlaylistJob(ctx context.Context, key, subID, video string, expectedRevision int, candidate application.Job) (application.Job, bool, error) {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return candidate, false, e
	}
	defer tx.Rollback()
	p, e := subscriptionTx(ctx, tx, subID)
	if e != nil {
		return candidate, false, e
	}
	if !p.Enabled || p.Revision != expectedRevision {
		return candidate, false, &core.Failure{Code: core.CodeConflict, Message: "subscription paused or changed"}
	}
	var eligible bool
	var admitted int
	var bound string
	if e = tx.QueryRowContext(ctx, "SELECT eligible,materialization_key,job_id FROM playlist_members WHERE subscription_id=? AND video_id=?", subID, video).Scan(&eligible, &bound, &admitted); e != nil {
		return candidate, false, e
	}
	if bound != "" {
		key = bound
	}
	var b, asset string
	j := candidate
	created := false
	e = tx.QueryRowContext(ctx, "SELECT record,asset_id FROM playlist_materializations WHERE key=?", key).Scan(&b, &asset)
	if e == nil {
		if e = json.Unmarshal([]byte(b), &j); e != nil {
			return candidate, false, e
		}
	} else if errors.Is(e, sql.ErrNoRows) {
		if !eligible || candidate.ID == 0 {
			return candidate, false, errors.New("playlist item is not eligible")
		}
		if e = saveChangesTx(ctx, tx, application.Changes{NextID: candidate.ID, Upserts: []application.Job{candidate}}, false); e != nil {
			return candidate, false, e
		}
		b, e = encode(candidate)
		if e != nil {
			return candidate, false, e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO playlist_materializations(key,job_id,video_id,record) VALUES(?,?,?,?)", key, candidate.ID, video, b); e != nil {
			return candidate, false, e
		}
		created = true
	} else {
		return candidate, false, e
	}
	if admitted > 0 {
		return j, false, tx.Commit()
	}
	if _, e = tx.ExecContext(ctx, "UPDATE playlist_members SET job_id=?,materialization_key=? WHERE subscription_id=? AND video_id=?", j.ID, key, subID, video); e != nil {
		return candidate, false, e
	}
	// A playlist materialization always overrides global AutoUpload, even local-only.
	for _, id := range p.S3Profiles {
		profile, e := decodeRow[core.S3Profile](tx.QueryRowContext(ctx, "SELECT record FROM s3_profiles WHERE id=?", id))
		if e != nil {
			return candidate, false, &core.Failure{Code: core.CodeValidation, Message: "S3 profile unavailable; edit the subscription to select another destination", Cause: e}
		}
		profileJSON, e := encode(profile)
		if e != nil {
			return candidate, false, e
		}
		if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO playlist_deliveries(materialization_key,profile_id,destination_key,profile) VALUES(?,?,?,?)", key, id, playlistDestination(profile), profileJSON); e != nil {
			return candidate, false, e
		}
		if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO playlist_delivery_bindings VALUES(?,?,?,?,?)", subID, video, id, key, playlistDestination(profile)); e != nil {
			return candidate, false, e
		}
	}
	if asset != "" {
		if e = enqueuePlaylistDeliveriesTx(ctx, tx, key, asset); e != nil {
			return candidate, false, e
		}
	}
	if _, e = tx.ExecContext(ctx, "UPDATE playlist_subscriptions SET last_admission=? WHERE id=?", time.Now().UnixNano(), subID); e != nil {
		return candidate, false, e
	}
	return j, created, tx.Commit()
}
func enqueuePlaylistDeliveriesTx(ctx context.Context, tx *sql.Tx, key, assetID string) error {
	a, e := decodeRow[core.Asset](tx.QueryRowContext(ctx, "SELECT record FROM assets WHERE id=?", assetID))
	if e != nil {
		return e
	}
	rows, e := tx.QueryContext(ctx, "SELECT destination_key,profile FROM playlist_deliveries WHERE materialization_key=? AND upload_id=''", key)
	if e != nil {
		return e
	}
	profiles := []core.S3Profile{}
	for rows.Next() {
		var id, b string
		if e = rows.Scan(&id, &b); e != nil {
			rows.Close()
			return e
		}
		var p core.S3Profile
		if e = json.Unmarshal([]byte(b), &p); e != nil {
			rows.Close()
			return e
		}
		profiles = append(profiles, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	var video string
	if e = tx.QueryRowContext(ctx, "SELECT video_id FROM playlist_materializations WHERE key=?", key).Scan(&video); e != nil {
		return e
	}
	for _, p := range profiles {
		objectKey := path.Join(p.Prefix, "youtube", video, key+filepath.Ext(a.Path))
		j := core.UploadJob{ID: core.NewID(), AssetID: a.ID, ProfileID: p.ID, Profile: p, Key: objectKey, State: core.UploadWaitingUnlock, Created: time.Now(), Updated: time.Now(), Size: a.Size, Modified: a.Modified, SHA256: a.SHA256, Auto: true, Verification: "uploaded_unverified"}
		if e = saveUploadTx(ctx, tx, j, true); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE playlist_deliveries SET upload_id=? WHERE materialization_key=? AND destination_key=?", j.ID, key, playlistDestination(p)); e != nil {
			return e
		}
	}
	return nil
}
func (r *Repository) SubscriptionItems(ctx context.Context, id string, limit, offset int) (core.PlaylistItemPage, error) {
	page := core.PlaylistItemPage{Items: []core.PlaylistItem{}}
	if _, e := r.Subscription(ctx, id); e != nil {
		return page, e
	}
	if e := r.db.QueryRowContext(ctx, "SELECT count(*) FROM playlist_members WHERE subscription_id=?", id).Scan(&page.Total); e != nil {
		return page, e
	}
	rows, e := r.db.QueryContext(ctx, `SELECT m.video_id,m.title,m.eligible,m.job_id,COALESCE(x.asset_id,''),COALESCE(x.record,''),m.admission_error FROM playlist_members m LEFT JOIN playlist_materializations x ON x.key=m.materialization_key WHERE m.subscription_id=? ORDER BY m.first_seen DESC,m.video_id LIMIT ? OFFSET ?`, id, min(max(limit, 1), 200), max(offset, 0))
	if e != nil {
		return page, e
	}
	for rows.Next() {
		v := core.PlaylistItem{SubscriptionID: id, Deliveries: []core.UploadJob{}}
		var record string
		if e = rows.Scan(&v.VideoID, &v.Title, &v.Eligible, &v.JobID, &v.AssetID, &record, &v.LastError); e != nil {
			rows.Close()
			return page, e
		}
		v.LocalState = "pending"
		if !v.Eligible {
			v.LocalState = "baseline"
		}
		if record != "" {
			var j application.Job
			if e = json.Unmarshal([]byte(record), &j); e != nil {
				rows.Close()
				return page, e
			}
			v.LocalState = string(j.State)
			v.LastError = j.Monitoring.LastError
		}
		if v.LastError != "" {
			v.LocalState = "blocked"
		}
		v.State = v.LocalState
		page.Items = append(page.Items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return page, e
	}
	for i := range page.Items {
		v := &page.Items[i]
		rs, e := r.db.QueryContext(ctx, `SELECT u.record FROM playlist_delivery_bindings b JOIN playlist_deliveries d ON d.materialization_key=b.materialization_key AND d.destination_key=b.destination_key LEFT JOIN s3_upload_jobs u ON u.id=d.upload_id WHERE b.subscription_id=? AND b.video_id=?`, id, v.VideoID)
		if e != nil {
			return page, e
		}
		for rs.Next() {
			var b sql.NullString
			if e = rs.Scan(&b); e != nil {
				rs.Close()
				return page, e
			}
			if b.Valid {
				var j core.UploadJob
				if e = json.Unmarshal([]byte(b.String), &j); e != nil {
					rs.Close()
					return page, e
				}
				v.Deliveries = append(v.Deliveries, j)
			}
		}
		e = rs.Err()
		rs.Close()
		if e != nil {
			return page, e
		}
		if v.LocalState == "done" {
			v.State = "delivered"
			for _, j := range v.Deliveries {
				if j.State != "done" {
					v.State = "waiting_delivery"
					break
				}
			}
		}
	}
	return page, nil
}

var _ application.PlaylistRepository = (*Repository)(nil)

func (r *Repository) PlaylistItemJob(ctx context.Context, id, video string) (int, error) {
	var job int
	e := r.db.QueryRowContext(ctx, "SELECT job_id FROM playlist_members WHERE subscription_id=? AND video_id=?", id, video).Scan(&job)
	if e == nil && job == 0 {
		e = errors.New("item is not admitted yet")
	}
	return job, e
}

func (r *Repository) BlockPlaylistItem(ctx context.Context, id, video, message string) error {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "UPDATE playlist_members SET admission_error=? WHERE subscription_id=? AND video_id=? AND job_id=0", message, id, video); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE playlist_subscriptions SET last_admission=? WHERE id=?", time.Now().UnixNano(), id); e != nil {
		return e
	}
	return tx.Commit()
}

func playlistDestination(p core.S3Profile) string {
	sum := sha256.Sum256([]byte(p.ID + "\x00" + p.Endpoint + "\x00" + p.Region + "\x00" + p.Bucket + "\x00" + p.Prefix + "\x00" + p.Style))
	return fmt.Sprintf("%x", sum[:])
}

// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package sqlite

import "context"

const createJobs = `CREATE TABLE IF NOT EXISTS jobs (
 id INTEGER PRIMARY KEY CHECK(id>0),url TEXT NOT NULL,title TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('queued','inspecting','choose','ready','waiting_download','downloading','waiting_processing','processing','waiting_verification','verifying','done','failed','canceling','canceled','waiting_disk_capacity')),
 detail TEXT NOT NULL,output_path TEXT NOT NULL,needs_quality INTEGER NOT NULL CHECK(needs_quality IN (0,1)),request_json TEXT NOT NULL,media_json TEXT NOT NULL,
 monitor_json TEXT NOT NULL DEFAULT '{}',control_json TEXT NOT NULL DEFAULT '{}',history_json TEXT NOT NULL DEFAULT '[]')`

func (r *Repository) schema(ctx context.Context, version int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS metadata(key TEXT PRIMARY KEY,value INTEGER NOT NULL)`); err != nil {
		return err
	}
	if version == 1 || version == 2 {
		if _, err = tx.ExecContext(ctx, "ALTER TABLE jobs RENAME TO jobs_v1"); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, createJobs); err != nil {
		return err
	}
	if version == 1 {
		if _, err = tx.ExecContext(ctx, `INSERT INTO jobs(id,url,title,state,detail,output_path,needs_quality,request_json,media_json) SELECT id,url,title,state,detail,output_path,needs_quality,request_json,media_json FROM jobs_v1`); err != nil {
			return err
		}
	} else if version == 2 {
		if _, err = tx.ExecContext(ctx, `INSERT INTO jobs(id,url,title,state,detail,output_path,needs_quality,request_json,media_json,monitor_json) SELECT id,url,title,state,detail,output_path,needs_quality,request_json,media_json,monitor_json FROM jobs_v1`); err != nil {
			return err
		}
	}
	if version == 1 || version == 2 {
		if _, err = tx.ExecContext(ctx, "DROP TABLE jobs_v1"); err != nil {
			return err
		}
	}
	if version == 3 {
		// Retain all previously committed publications while widening their identity.
		if _, err = tx.ExecContext(ctx, "ALTER TABLE publication_outbox RENAME TO publication_outbox_v3"); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, catalogSchema); err != nil {
		return err
	}
	if version == 3 {
		if _, err = tx.ExecContext(ctx, "INSERT INTO publication_outbox SELECT job_id,asset_id,path FROM publication_outbox_v3"); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "DROP TABLE publication_outbox_v3"); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "PRAGMA user_version=4"); err != nil {
		return err
	}
	return tx.Commit()
}

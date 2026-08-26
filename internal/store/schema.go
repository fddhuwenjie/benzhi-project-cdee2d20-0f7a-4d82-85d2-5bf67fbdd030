package store

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 5

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS schema_versions (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS missions (
		id TEXT PRIMARY KEY, title TEXT NOT NULL, cave_site TEXT NOT NULL, cave_site_key TEXT NOT NULL, target_depth_m REAL NOT NULL,
        window_start TEXT NOT NULL, window_end TEXT NOT NULL, status TEXT NOT NULL, revision INTEGER NOT NULL,
        leader_id TEXT NOT NULL, release_digest TEXT NOT NULL DEFAULT '', release_checklist TEXT NOT NULL DEFAULT '[]',
		signed_by TEXT NOT NULL DEFAULT '', archive_digest TEXT NOT NULL DEFAULT '', risk_summary TEXT NOT NULL DEFAULT '{}',
		last_release_rejection TEXT NOT NULL DEFAULT 'null', created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL, archived_at TEXT, plan_history TEXT NOT NULL DEFAULT '[]', verification_history TEXT NOT NULL DEFAULT '[]'
		, template_mission_id TEXT NOT NULL DEFAULT '', template_archive_digest TEXT NOT NULL DEFAULT ''
    )`,
	`CREATE TABLE IF NOT EXISTS mission_segments (mission_id TEXT NOT NULL, position INTEGER NOT NULL, name TEXT NOT NULL, PRIMARY KEY(mission_id, position), UNIQUE(mission_id, name), FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT)`,
	`CREATE TABLE IF NOT EXISTS mission_members (mission_id TEXT NOT NULL, person_id TEXT NOT NULL, name TEXT NOT NULL, role TEXT NOT NULL, PRIMARY KEY(mission_id, person_id), UNIQUE(mission_id, role), FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT)`,
	`CREATE TABLE IF NOT EXISTS segment_risks (id TEXT PRIMARY KEY, mission_id TEXT NOT NULL, segment_name TEXT NOT NULL, current_level INTEGER NOT NULL, visibility_m REAL NOT NULL, restriction_grade INTEGER NOT NULL, exit_limit_min INTEGER NOT NULL, hazards TEXT NOT NULL, mitigations TEXT NOT NULL, risk_level TEXT NOT NULL, risk_explanation TEXT NOT NULL, score INTEGER NOT NULL, score_breakdown TEXT NOT NULL, assessed_by TEXT NOT NULL, UNIQUE(mission_id, segment_name), FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT)`,
	`CREATE TABLE IF NOT EXISTS life_support_plans (id TEXT PRIMARY KEY, mission_id TEXT NOT NULL UNIQUE, members TEXT NOT NULL, gas_mixes TEXT NOT NULL, turn_pressure_bar INTEGER NOT NULL, reserve_rule TEXT NOT NULL, support_assignments TEXT NOT NULL, review_status TEXT NOT NULL, reviewed_by TEXT NOT NULL, review_note TEXT NOT NULL, cross_check TEXT NOT NULL, failed_rules TEXT NOT NULL, plan_metadata TEXT NOT NULL DEFAULT '{}', FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT)`,
	`CREATE TABLE IF NOT EXISTS verification_records (id TEXT PRIMARY KEY, mission_id TEXT NOT NULL, record_type TEXT NOT NULL, check_code TEXT NOT NULL, outcome TEXT NOT NULL, evidence_digest TEXT NOT NULL, deviation TEXT NOT NULL, corrective_action TEXT NOT NULL, review_marker TEXT NOT NULL, verified_by TEXT NOT NULL, recorded_at TEXT NOT NULL, asset_id TEXT NOT NULL DEFAULT '', inspected_at TEXT NOT NULL DEFAULT '', valid_until TEXT NOT NULL DEFAULT '', conducted_at TEXT NOT NULL DEFAULT '', duration_seconds INTEGER NOT NULL DEFAULT 0, verification_metadata TEXT NOT NULL DEFAULT '{}', UNIQUE(mission_id, record_type, check_code), FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT)`,
	`CREATE TABLE IF NOT EXISTS command_results (request_id TEXT PRIMARY KEY, mission_id TEXT NOT NULL, operation TEXT NOT NULL, payload_fingerprint TEXT NOT NULL, status_code INTEGER NOT NULL, response_body BLOB NOT NULL, created_at TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS audit_events (mission_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_type TEXT NOT NULL, actor_id TEXT NOT NULL, request_id TEXT NOT NULL, from_revision INTEGER NOT NULL, to_revision INTEGER NOT NULL, status_after TEXT NOT NULL, payload_digest TEXT NOT NULL, previous_hash TEXT NOT NULL, event_hash TEXT NOT NULL, occurred_at TEXT NOT NULL, data BLOB NOT NULL, PRIMARY KEY(mission_id, sequence), UNIQUE(request_id), FOREIGN KEY(mission_id) REFERENCES missions(id) ON DELETE RESTRICT)`,
	`CREATE TRIGGER IF NOT EXISTS audit_events_no_update BEFORE UPDATE ON audit_events BEGIN SELECT RAISE(ABORT, 'audit events are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS audit_events_no_delete BEFORE DELETE ON audit_events BEGIN SELECT RAISE(ABORT, 'audit events are append-only'); END`,
	`CREATE INDEX IF NOT EXISTS idx_audit_events_mission_sequence ON audit_events(mission_id, sequence)`,
	`CREATE INDEX IF NOT EXISTS idx_verifications_mission ON verification_records(mission_id, record_type, check_code)`,
}

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range migrations {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("执行数据库迁移: %w", err)
		}
	}
	columns := []struct{ table, name, definition string }{
		{"missions", "cave_site_key", "TEXT NOT NULL DEFAULT ''"}, {"missions", "risk_summary", "TEXT NOT NULL DEFAULT '{}'"}, {"missions", "last_release_rejection", "TEXT NOT NULL DEFAULT 'null'"},
		{"segment_risks", "risk_explanation", "TEXT NOT NULL DEFAULT ''"}, {"segment_risks", "score", "INTEGER NOT NULL DEFAULT 0"}, {"segment_risks", "score_breakdown", "TEXT NOT NULL DEFAULT '{}'"},
		{"life_support_plans", "cross_check", "TEXT NOT NULL DEFAULT '{}'"}, {"life_support_plans", "failed_rules", "TEXT NOT NULL DEFAULT '[]'"},
		{"life_support_plans", "plan_metadata", "TEXT NOT NULL DEFAULT '{}'"},
		{"verification_records", "review_marker", "TEXT NOT NULL DEFAULT ''"}, {"command_results", "payload_fingerprint", "TEXT NOT NULL DEFAULT ''"},
		{"missions", "plan_history", "TEXT NOT NULL DEFAULT '[]'"}, {"missions", "verification_history", "TEXT NOT NULL DEFAULT '[]'"},
		{"missions", "qualification_snapshot", "TEXT NOT NULL DEFAULT '[]'"},
		{"missions", "risk_history", "TEXT NOT NULL DEFAULT '[]'"},
		{"segment_risks", "mitigation_actions", "TEXT NOT NULL DEFAULT '[]'"},
		{"verification_records", "asset_id", "TEXT NOT NULL DEFAULT ''"}, {"verification_records", "inspected_at", "TEXT NOT NULL DEFAULT ''"}, {"verification_records", "valid_until", "TEXT NOT NULL DEFAULT ''"}, {"verification_records", "conducted_at", "TEXT NOT NULL DEFAULT ''"}, {"verification_records", "duration_seconds", "INTEGER NOT NULL DEFAULT 0"},
		{"verification_records", "verification_metadata", "TEXT NOT NULL DEFAULT '{}'"}, {"missions", "template_mission_id", "TEXT NOT NULL DEFAULT ''"}, {"missions", "template_archive_digest", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if err := ensureColumn(ctx, tx, column.table, column.name, column.definition); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE missions SET cave_site_key=lower(trim(cave_site)) WHERE cave_site_key=''`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_missions_site_window ON missions(cave_site_key, status, window_start, window_end)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_versions(version, applied_at) VALUES (?, datetime('now'))`, schemaVersion); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_versions`).Scan(&version); err != nil {
		return err
	}
	if version != schemaVersion {
		return fmt.Errorf("不支持的数据库模式版本: %d", version)
	}
	return tx.Commit()
}

func ensureColumn(ctx context.Context, tx *sql.Tx, table, column, definition string) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid int
		var name, kind string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		found = found || name == column
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition); err != nil {
		return fmt.Errorf("升级数据库字段 %s.%s: %w", table, column, err)
	}
	return nil
}

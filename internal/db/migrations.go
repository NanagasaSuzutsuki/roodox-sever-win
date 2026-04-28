package db

import (
	"database/sql"
	"fmt"
)

const CurrentSchemaVersion = 2

type schemaMigration struct {
	version int
	name    string
	apply   func(*sql.Tx) error
}

var schemaMigrations = []schemaMigration{
	{
		version: 1,
		name:    "core_file_tables",
		apply:   migrateToSchemaV1,
	},
	{
		version: 2,
		name:    "control_plane_tables",
		apply:   migrateToSchemaV2,
	},
}

func applyMigrations(database *sql.DB) error {
	version, err := currentSchemaVersion(database)
	if err != nil {
		return err
	}

	for _, migration := range schemaMigrations {
		if version >= migration.version {
			continue
		}
		if err := runMigration(database, migration); err != nil {
			return fmt.Errorf("apply schema migration v%d (%s): %w", migration.version, migration.name, err)
		}
		version = migration.version
	}
	return nil
}

func currentSchemaVersion(database *sql.DB) (int, error) {
	var version int
	if err := database.QueryRow(`PRAGMA user_version;`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func runMigration(database *sql.DB, migration schemaMigration) error {
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := migration.apply(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d;`, migration.version)); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateToSchemaV1(tx *sql.Tx) error {
	return execStatements(tx,
		`
CREATE TABLE IF NOT EXISTS meta (
    path       TEXT PRIMARY KEY,
    is_dir     INTEGER,
    size       INTEGER,
    mtime_unix INTEGER,
    hash       TEXT,
    version    INTEGER
);
`,
		`
CREATE TABLE IF NOT EXISTS version (
    path        TEXT,
    version     INTEGER,
    mtime_unix  INTEGER,
    hash        TEXT,
    size        INTEGER,
    client_id   TEXT,
    change_type TEXT,
    PRIMARY KEY(path, version)
);
`,
		`
CREATE TABLE IF NOT EXISTS version_blob (
    path    TEXT,
    version INTEGER,
    data    BLOB,
    PRIMARY KEY(path, version),
    FOREIGN KEY(path, version) REFERENCES version(path, version) ON DELETE CASCADE
);
`,
		`
CREATE TABLE IF NOT EXISTS file_head (
    path            TEXT PRIMARY KEY,
    current_version INTEGER NOT NULL
);
`,
	)
}

func migrateToSchemaV2(tx *sql.Tx) error {
	if err := execStatements(tx,
		`
CREATE TABLE IF NOT EXISTS device_registry (
    device_id TEXT PRIMARY KEY,
    device_name TEXT NOT NULL DEFAULT '',
    device_role TEXT NOT NULL DEFAULT '',
    assigned_device_label TEXT NOT NULL DEFAULT '',
    client_version TEXT NOT NULL DEFAULT '',
    platform TEXT NOT NULL DEFAULT '',
    overlay_provider TEXT NOT NULL DEFAULT '',
    overlay_address TEXT NOT NULL DEFAULT '',
    capabilities_json TEXT NOT NULL DEFAULT '[]',
    server_id TEXT NOT NULL DEFAULT '',
    device_group TEXT NOT NULL DEFAULT '',
    last_session_id TEXT NOT NULL DEFAULT '',
    heartbeat_interval_seconds INTEGER NOT NULL DEFAULT 15,
    policy_revision INTEGER NOT NULL DEFAULT 1,
    requires_policy_pull INTEGER NOT NULL DEFAULT 1,
    overlay_connected INTEGER NOT NULL DEFAULT 0,
    grpc_connected INTEGER NOT NULL DEFAULT 0,
    last_sync_time_unix INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    mount_state TEXT NOT NULL DEFAULT '',
    mounted INTEGER NOT NULL DEFAULT 0,
    mount_path TEXT NOT NULL DEFAULT '',
    last_mount_time_unix INTEGER NOT NULL DEFAULT 0,
    mount_last_error TEXT NOT NULL DEFAULT '',
    sync_state_summary TEXT NOT NULL DEFAULT '',
    current_task_count INTEGER NOT NULL DEFAULT 0,
    sync_last_success_time INTEGER NOT NULL DEFAULT 0,
    sync_last_error TEXT NOT NULL DEFAULT '',
    conflict_count INTEGER NOT NULL DEFAULT 0,
    queue_depth INTEGER NOT NULL DEFAULT 0,
    sync_summary TEXT NOT NULL DEFAULT '',
    created_at_unix INTEGER NOT NULL DEFAULT 0,
    updated_at_unix INTEGER NOT NULL DEFAULT 0,
    last_registered_at_unix INTEGER NOT NULL DEFAULT 0,
    last_heartbeat_at_unix INTEGER NOT NULL DEFAULT 0,
    last_sync_report_at_unix INTEGER NOT NULL DEFAULT 0,
    last_mount_report_at_unix INTEGER NOT NULL DEFAULT 0,
    last_seen_at_unix INTEGER NOT NULL DEFAULT 0
);
`,
		`
CREATE INDEX IF NOT EXISTS idx_device_registry_last_seen
ON device_registry(last_seen_at_unix DESC, updated_at_unix DESC);
`,
		`
CREATE TABLE IF NOT EXISTS device_policy (
    device_id TEXT PRIMARY KEY,
    mount_path TEXT NOT NULL DEFAULT '',
    sync_roots_json TEXT NOT NULL DEFAULT '[]',
    conflict_policy TEXT NOT NULL DEFAULT '',
    read_only INTEGER NOT NULL DEFAULT 0,
    auto_connect INTEGER NOT NULL DEFAULT 1,
    bandwidth_limit INTEGER NOT NULL DEFAULT 0,
    log_level TEXT NOT NULL DEFAULT '',
    large_file_threshold INTEGER NOT NULL DEFAULT 0,
    policy_revision INTEGER NOT NULL DEFAULT 1,
    updated_at_unix INTEGER NOT NULL DEFAULT 0
);
`,
		`
CREATE TABLE IF NOT EXISTS device_actions (
    action_id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    action_type TEXT NOT NULL,
    payload_json TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    requested_at_unix INTEGER NOT NULL DEFAULT 0,
    delivered_at_unix INTEGER NOT NULL DEFAULT 0,
    completed_at_unix INTEGER NOT NULL DEFAULT 0
);
`,
		`
CREATE INDEX IF NOT EXISTS idx_device_actions_pending
ON device_actions(device_id, status, requested_at_unix DESC);
`,
		`
CREATE TABLE IF NOT EXISTS device_diagnostics (
    diagnostics_id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    payload BLOB NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    uploaded_at_unix INTEGER NOT NULL DEFAULT 0
);
`,
		`
CREATE INDEX IF NOT EXISTS idx_device_diagnostics_recent
ON device_diagnostics(device_id, uploaded_at_unix DESC, diagnostics_id DESC);
`,
	); err != nil {
		return err
	}

	return ensureTableColumns(tx, "device_registry", []columnDefinition{
		{Name: "mounted", Definition: "mounted INTEGER NOT NULL DEFAULT 0"},
		{Name: "mount_path", Definition: "mount_path TEXT NOT NULL DEFAULT ''"},
		{Name: "last_mount_time_unix", Definition: "last_mount_time_unix INTEGER NOT NULL DEFAULT 0"},
		{Name: "mount_last_error", Definition: "mount_last_error TEXT NOT NULL DEFAULT ''"},
		{Name: "last_mount_report_at_unix", Definition: "last_mount_report_at_unix INTEGER NOT NULL DEFAULT 0"},
	})
}

func execStatements(tx *sql.Tx, statements ...string) error {
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

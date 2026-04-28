package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpenAppliesSchemaMigrations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "schema.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer database.Sql.Close()

	version, err := database.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion returned error: %v", err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, CurrentSchemaVersion)
	}

	for _, table := range []string{
		"meta",
		"version",
		"version_blob",
		"file_head",
		"device_registry",
		"device_policy",
		"device_actions",
		"device_diagnostics",
	} {
		if !tableExists(t, database.Sql, table) {
			t.Fatalf("expected table %q to exist", table)
		}
	}
}

func TestOpenMigratesLegacyDeviceRegistrySchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open returned error: %v", err)
	}
	if _, err := rawDB.Exec(`
CREATE TABLE device_registry (
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
    last_seen_at_unix INTEGER NOT NULL DEFAULT 0
);`); err != nil {
		t.Fatalf("CREATE TABLE legacy device_registry returned error: %v", err)
	}
	if _, err := rawDB.Exec(`INSERT INTO device_registry(device_id, device_name) VALUES ('dev-1', 'legacy');`); err != nil {
		t.Fatalf("INSERT legacy row returned error: %v", err)
	}
	if _, err := rawDB.Exec(`PRAGMA user_version = 0;`); err != nil {
		t.Fatalf("reset user_version returned error: %v", err)
	}
	if err := rawDB.Close(); err != nil {
		t.Fatalf("Close(rawDB) returned error: %v", err)
	}

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer database.Sql.Close()

	version, err := database.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion returned error: %v", err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, CurrentSchemaVersion)
	}

	for _, column := range []string{"mounted", "mount_path", "last_mount_time_unix", "mount_last_error", "last_mount_report_at_unix"} {
		if !tableHasColumn(t, database.Sql, "device_registry", column) {
			t.Fatalf("expected column %q to exist after migration", column)
		}
	}
	if !tableExists(t, database.Sql, "device_policy") {
		t.Fatal("expected device_policy table to exist after migration")
	}
}

func TestRestoreFromBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "restore.db")
	backupPath := filepath.Join(dir, "backups", "restore-backup.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if _, err := database.Sql.Exec(`CREATE TABLE demo (id INTEGER PRIMARY KEY, name TEXT);`); err != nil {
		t.Fatalf("CREATE TABLE demo returned error: %v", err)
	}
	if _, err := database.Sql.Exec(`INSERT INTO demo(name) VALUES ('alpha');`); err != nil {
		t.Fatalf("INSERT alpha returned error: %v", err)
	}
	if _, err := database.Checkpoint("truncate"); err != nil {
		t.Fatalf("Checkpoint returned error: %v", err)
	}
	if err := database.BackupInto(backupPath); err != nil {
		t.Fatalf("BackupInto returned error: %v", err)
	}
	if _, err := database.Sql.Exec(`INSERT INTO demo(name) VALUES ('beta');`); err != nil {
		t.Fatalf("INSERT beta returned error: %v", err)
	}
	if err := database.Sql.Close(); err != nil {
		t.Fatalf("Close(database) returned error: %v", err)
	}

	result, err := RestoreFromBackup(dbPath, backupPath, RestoreOptions{CreateSafetyBackup: true})
	if err != nil {
		t.Fatalf("RestoreFromBackup returned error: %v", err)
	}
	if result.SourcePath == "" || result.TargetPath == "" {
		t.Fatalf("restore result paths should not be empty: %+v", result)
	}
	if result.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("restore schema version = %d, want %d", result.SchemaVersion, CurrentSchemaVersion)
	}
	if result.SafetyBackupPath == "" {
		t.Fatal("expected safety backup path to be populated")
	}
	if _, err := os.Stat(result.SafetyBackupPath); err != nil {
		t.Fatalf("Stat(safety backup) returned error: %v", err)
	}

	restoredDB, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(restored) returned error: %v", err)
	}
	defer restoredDB.Sql.Close()

	var restoredCount int
	if err := restoredDB.Sql.QueryRow(`SELECT COUNT(*) FROM demo;`).Scan(&restoredCount); err != nil {
		t.Fatalf("SELECT COUNT restored returned error: %v", err)
	}
	if restoredCount != 1 {
		t.Fatalf("restored row count = %d, want 1", restoredCount)
	}

	safetyDB, err := sql.Open("sqlite", result.SafetyBackupPath)
	if err != nil {
		t.Fatalf("sql.Open(safety backup) returned error: %v", err)
	}
	defer safetyDB.Close()

	var safetyCount int
	if err := safetyDB.QueryRow(`SELECT COUNT(*) FROM demo;`).Scan(&safetyCount); err != nil {
		t.Fatalf("SELECT COUNT safety backup returned error: %v", err)
	}
	if safetyCount != 2 {
		t.Fatalf("safety backup row count = %d, want 2", safetyCount)
	}
}

func tableExists(t *testing.T, database *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := database.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?;`, table).Scan(&name)
	return err == nil && name == table
}

func tableHasColumn(t *testing.T, database *sql.DB, table string, column string) bool {
	t.Helper()
	rows, err := database.Query(`PRAGMA table_info(` + table + `);`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s) returned error: %v", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			t.Fatalf("scan table_info(%s) returned error: %v", table, err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error for table_info(%s): %v", table, err)
	}
	return false
}

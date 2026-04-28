package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDBCheckpointAndStats(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "checkpoint.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer database.Sql.Close()

	if _, err := database.Sql.Exec(`CREATE TABLE demo (id INTEGER PRIMARY KEY, name TEXT);`); err != nil {
		t.Fatalf("CREATE TABLE returned error: %v", err)
	}
	if _, err := database.Sql.Exec(`INSERT INTO demo(name) VALUES ('alice'), ('bob');`); err != nil {
		t.Fatalf("INSERT returned error: %v", err)
	}

	stats, err := database.Stats()
	if err != nil {
		t.Fatalf("Stats returned error: %v", err)
	}
	if !stats.DBFile.Exists {
		t.Fatal("expected main db file to exist")
	}

	checkpoint, err := database.Checkpoint("truncate")
	if err != nil {
		t.Fatalf("Checkpoint returned error: %v", err)
	}
	if checkpoint.Mode != "truncate" {
		t.Fatalf("Checkpoint mode = %q, want %q", checkpoint.Mode, "truncate")
	}
}

func TestDBBackupIntoCreatesSnapshot(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "backup.db")
	backupPath := filepath.Join(dir, "backups", "backup-copy.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer database.Sql.Close()

	if _, err := database.Sql.Exec(`CREATE TABLE demo (id INTEGER PRIMARY KEY, name TEXT);`); err != nil {
		t.Fatalf("CREATE TABLE returned error: %v", err)
	}
	if _, err := database.Sql.Exec(`INSERT INTO demo(name) VALUES ('alice');`); err != nil {
		t.Fatalf("INSERT returned error: %v", err)
	}

	if _, err := database.Checkpoint("truncate"); err != nil {
		t.Fatalf("Checkpoint returned error: %v", err)
	}
	if err := database.BackupInto(backupPath); err != nil {
		t.Fatalf("BackupInto returned error: %v", err)
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("Stat(backup) returned error: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("backup file size should be > 0")
	}

	backupDB, err := Open(backupPath)
	if err != nil {
		t.Fatalf("Open(backup) returned error: %v", err)
	}
	defer backupDB.Sql.Close()

	var count int
	if err := backupDB.Sql.QueryRow(`SELECT COUNT(*) FROM demo;`).Scan(&count); err != nil {
		t.Fatalf("SELECT COUNT returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("backup row count = %d, want 1", count)
	}
}

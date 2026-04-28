package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	Sql  *sql.DB
	path string
}

type FileStat struct {
	Path           string
	Exists         bool
	SizeBytes      int64
	ModifiedAtUnix int64
}

type Stats struct {
	DBFile  FileStat
	WALFile FileStat
	SHMFile FileStat
}

type CheckpointResult struct {
	Mode               string
	BusyReaders        int64
	LogFrames          int64
	CheckpointedFrames int64
	CompletedAtUnix    int64
}

func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// SQLite write contention is much easier to reason about with a single shared
	// connection inside one process.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec(`PRAGMA journal_mode = WAL;`); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA synchronous = NORMAL;`); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000;`); err != nil {
		return nil, err
	}
	if err := applyMigrations(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &DB{Sql: db, path: path}, nil
}

func (d *DB) Path() string {
	if d == nil {
		return ""
	}
	return d.path
}

func (d *DB) Ping() error {
	if d == nil || d.Sql == nil {
		return fmt.Errorf("database is not initialized")
	}
	if err := d.Sql.Ping(); err != nil {
		return err
	}
	var one int
	return d.Sql.QueryRow(`SELECT 1;`).Scan(&one)
}

func (d *DB) SchemaVersion() (int, error) {
	if d == nil || d.Sql == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	return currentSchemaVersion(d.Sql)
}

func (d *DB) QuickCheck() error {
	if d == nil || d.Sql == nil {
		return fmt.Errorf("database is not initialized")
	}

	var result string
	if err := d.Sql.QueryRow(`PRAGMA quick_check(1);`).Scan(&result); err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(result)) != "ok" {
		return fmt.Errorf("sqlite quick_check failed: %s", result)
	}
	return nil
}

func (d *DB) Stats() (Stats, error) {
	if d == nil {
		return Stats{}, fmt.Errorf("database is not initialized")
	}
	return Stats{
		DBFile:  fileStat(d.path),
		WALFile: fileStat(d.path + "-wal"),
		SHMFile: fileStat(d.path + "-shm"),
	}, nil
}

func (d *DB) Checkpoint(mode string) (CheckpointResult, error) {
	if d == nil || d.Sql == nil {
		return CheckpointResult{}, fmt.Errorf("database is not initialized")
	}

	mode = normalizeCheckpointMode(mode)
	row := d.Sql.QueryRow(fmt.Sprintf(`PRAGMA wal_checkpoint(%s);`, strings.ToUpper(mode)))

	var result CheckpointResult
	result.Mode = mode
	result.CompletedAtUnix = time.Now().Unix()
	if err := row.Scan(&result.BusyReaders, &result.LogFrames, &result.CheckpointedFrames); err != nil {
		return CheckpointResult{}, err
	}
	return result, nil
}

func (d *DB) BackupInto(destPath string) error {
	if d == nil || d.Sql == nil {
		return fmt.Errorf("database is not initialized")
	}
	if strings.TrimSpace(destPath) == "" {
		return fmt.Errorf("backup destination path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("backup destination already exists: %s", destPath)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	quoted := sqliteQuote(destPath)
	_, err := d.Sql.Exec(fmt.Sprintf(`VACUUM INTO '%s';`, quoted))
	return err
}

func normalizeCheckpointMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "full":
		return "full"
	case "restart":
		return "restart"
	case "truncate":
		return "truncate"
	default:
		return "passive"
	}
}

func sqliteQuote(value string) string {
	return strings.ReplaceAll(value, `'`, `''`)
}

func fileStat(path string) FileStat {
	item := FileStat{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		return item
	}
	item.Exists = true
	item.SizeBytes = info.Size()
	item.ModifiedAtUnix = info.ModTime().Unix()
	return item
}

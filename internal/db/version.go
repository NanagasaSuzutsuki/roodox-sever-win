package db

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"roodox_server/internal/fs"
)

type VersionStore struct {
	db *sql.DB
}

func NewVersionStore(database *DB) (*VersionStore, error) {
	return &VersionStore{db: database.Sql}, nil
}

func (v *VersionStore) Append(path string, rec fs.VersionRecord) error {
	return v.AppendSnapshot(path, rec, nil)
}

func (v *VersionStore) AppendSnapshot(path string, rec fs.VersionRecord, data []byte) error {
	return v.withWriteTx("append_snapshot", path, "", func(tx *sql.Tx) error {
		if err := v.ensureFileHeadTx(tx, path); err != nil {
			return err
		}
		if err := v.setCurrentVersionIfGreaterTx(tx, path, rec.Version); err != nil {
			return err
		}
		return v.insertSnapshotTx(tx, path, rec, data)
	})
}

func (v *VersionStore) SaveFileSnapshot(meta *fs.Meta, clientID, requestID, changeType string, data []byte) (uint64, error) {
	var newVersion uint64
	err := v.withWriteTx("save_file_snapshot", meta.Path, requestID, func(tx *sql.Tx) error {
		version, err := v.nextVersionTx(tx, meta.Path)
		if err != nil {
			return err
		}

		meta.Version = version
		if err := v.upsertMetaTx(tx, meta); err != nil {
			return err
		}

		newVersion = version
		return v.insertSnapshotTx(tx, meta.Path, fs.VersionRecord{
			Version:    version,
			MtimeUnix:  meta.MtimeUnix,
			Hash:       meta.Hash,
			Size:       meta.Size,
			ClientID:   clientID,
			ChangeType: changeType,
		}, data)
	})
	return newVersion, err
}

func (v *VersionStore) AppendDeleteSnapshot(path string, size int64, mtimeUnix int64, clientID, requestID string) (uint64, error) {
	var newVersion uint64
	err := v.withWriteTx("append_delete_snapshot", path, requestID, func(tx *sql.Tx) error {
		version, err := v.nextVersionTx(tx, path)
		if err != nil {
			return err
		}
		if err := v.deleteMetaTx(tx, path); err != nil {
			return err
		}

		newVersion = version
		return v.insertSnapshotTx(tx, path, fs.VersionRecord{
			Version:    version,
			MtimeUnix:  mtimeUnix,
			Hash:       "",
			Size:       size,
			ClientID:   clientID,
			ChangeType: "delete",
		}, nil)
	})
	return newVersion, err
}

func (v *VersionStore) RenamePathTree(oldPath, newPath string) error {
	if oldPath == newPath {
		return nil
	}

	prefixPattern := escapeSQLiteLikePattern(oldPath) + "/%"
	childSuffixStart := utf8.RuneCountInString(oldPath) + 1

	return v.withWriteTx("rename_path_tree", oldPath, "", func(tx *sql.Tx) error {
		for _, table := range []string{"meta", "file_head", "version", "version_blob"} {
			if err := renamePathPrefixTx(tx, table, oldPath, newPath, prefixPattern, childSuffixStart); err != nil {
				return err
			}
		}
		return nil
	})
}

func (v *VersionStore) GetHistory(path string) ([]fs.VersionRecord, error) {
	rows, err := v.db.Query(`
SELECT version, mtime_unix, hash, size, client_id, change_type
FROM version
WHERE path = ?
ORDER BY version ASC;
`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hist []fs.VersionRecord
	for rows.Next() {
		var r fs.VersionRecord
		if err := rows.Scan(&r.Version, &r.MtimeUnix, &r.Hash, &r.Size, &r.ClientID, &r.ChangeType); err != nil {
			continue
		}
		hist = append(hist, r)
	}
	return hist, nil
}

func (v *VersionStore) GetRecord(path string, version uint64) (*fs.VersionRecord, error) {
	row := v.db.QueryRow(`
SELECT version, mtime_unix, hash, size, client_id, change_type
FROM version
WHERE path = ? AND version = ?;
`, path, version)

	var rec fs.VersionRecord
	if err := row.Scan(&rec.Version, &rec.MtimeUnix, &rec.Hash, &rec.Size, &rec.ClientID, &rec.ChangeType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("version record not found: %w", os.ErrNotExist)
		}
		return nil, err
	}
	return &rec, nil
}

func (v *VersionStore) GetLatestRecord(path string) (*fs.VersionRecord, error) {
	row := v.db.QueryRow(`
SELECT version, mtime_unix, hash, size, client_id, change_type
FROM version
WHERE path = ?
ORDER BY version DESC
LIMIT 1;
`, path)

	var rec fs.VersionRecord
	if err := row.Scan(&rec.Version, &rec.MtimeUnix, &rec.Hash, &rec.Size, &rec.ClientID, &rec.ChangeType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("latest version record not found: %w", os.ErrNotExist)
		}
		return nil, err
	}
	return &rec, nil
}

func (v *VersionStore) GetVersionData(path string, version uint64) ([]byte, error) {
	row := v.db.QueryRow(`
SELECT data
FROM version_blob
WHERE path = ? AND version = ?;
`, path, version)

	var data []byte
	if err := row.Scan(&data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("version data not found: %w", os.ErrNotExist)
		}
		return nil, err
	}
	return data, nil
}

func (v *VersionStore) withWriteTx(operation, path, requestID string, fn func(tx *sql.Tx) error) error {
	const maxAttempts = 5

	var lastErr error
	totalWait := time.Duration(0)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		tx, err := v.db.Begin()
		if err != nil {
			lastErr = err
		} else {
			runErr := fn(tx)
			if runErr == nil {
				runErr = tx.Commit()
			}
			if runErr != nil {
				_ = tx.Rollback()
				lastErr = runErr
			} else {
				return nil
			}
		}

		if !isSQLiteBusy(lastErr) || attempt == maxAttempts-1 {
			if attempt > 0 || totalWait > 0 {
				log.Printf(
					"component=db operation=%s request_id=%s path=%q retry_count=%d db_wait_ms=%d final_error=%q",
					operation,
					requestID,
					path,
					attempt,
					totalWait.Milliseconds(),
					errString(lastErr),
				)
			}
			return lastErr
		}
		wait := time.Duration(25*(1<<attempt)) * time.Millisecond
		totalWait += wait
		log.Printf(
			"component=db operation=%s request_id=%s path=%q retry_count=%d db_wait_ms=%d retry_reason=%q",
			operation,
			requestID,
			path,
			attempt+1,
			totalWait.Milliseconds(),
			errString(lastErr),
		)
		time.Sleep(wait)
	}
	return lastErr
}

func (v *VersionStore) nextVersionTx(tx *sql.Tx, path string) (uint64, error) {
	if err := v.ensureFileHeadTx(tx, path); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(`
UPDATE file_head
SET current_version = current_version + 1
WHERE path = ?;
`, path); err != nil {
		return 0, err
	}

	var version uint64
	if err := tx.QueryRow(`
SELECT current_version
FROM file_head
WHERE path = ?;
`, path).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func (v *VersionStore) ensureFileHeadTx(tx *sql.Tx, path string) error {
	var currentVersion uint64
	err := tx.QueryRow(`
SELECT current_version
FROM file_head
WHERE path = ?;
`, path).Scan(&currentVersion)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	var versionMax uint64
	if err := tx.QueryRow(`
SELECT COALESCE(MAX(version), 0)
FROM version
WHERE path = ?;
`, path).Scan(&versionMax); err != nil {
		return err
	}

	var metaVersion uint64
	metaErr := tx.QueryRow(`
SELECT version
FROM meta
WHERE path = ?;
`, path).Scan(&metaVersion)
	if errors.Is(metaErr, sql.ErrNoRows) {
		metaErr = nil
	}
	if metaErr != nil {
		if strings.Contains(strings.ToLower(metaErr.Error()), "no such table: meta") {
			metaErr = nil
		} else {
			return metaErr
		}
	}
	if metaErr == nil && metaVersion > versionMax {
		versionMax = metaVersion
	}

	_, err = tx.Exec(`
INSERT OR IGNORE INTO file_head(path, current_version)
VALUES (?, ?);
`, path, versionMax)
	return err
}

func (v *VersionStore) setCurrentVersionIfGreaterTx(tx *sql.Tx, path string, version uint64) error {
	_, err := tx.Exec(`
UPDATE file_head
SET current_version = CASE
    WHEN current_version < ? THEN ?
    ELSE current_version
END
WHERE path = ?;
`, version, version, path)
	return err
}

func (v *VersionStore) upsertMetaTx(tx *sql.Tx, meta *fs.Meta) error {
	isDirI := 0
	if meta.IsDir {
		isDirI = 1
	}

	_, err := tx.Exec(`
INSERT INTO meta(path, is_dir, size, mtime_unix, hash, version)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(path) DO UPDATE SET
    is_dir     = excluded.is_dir,
    size       = excluded.size,
    mtime_unix = excluded.mtime_unix,
    hash       = excluded.hash,
    version    = excluded.version;
`,
		meta.Path, isDirI, meta.Size, meta.MtimeUnix, meta.Hash, meta.Version,
	)
	return err
}

func (v *VersionStore) deleteMetaTx(tx *sql.Tx, path string) error {
	_, err := tx.Exec(`DELETE FROM meta WHERE path = ?`, path)
	return err
}

func (v *VersionStore) insertSnapshotTx(tx *sql.Tx, path string, rec fs.VersionRecord, data []byte) error {
	if _, err := tx.Exec(`
INSERT INTO version(path, version, mtime_unix, hash, size, client_id, change_type)
VALUES (?, ?, ?, ?, ?, ?, ?);
`,
		path, rec.Version, rec.MtimeUnix, rec.Hash, rec.Size, rec.ClientID, rec.ChangeType,
	); err != nil {
		return err
	}

	if data == nil {
		return nil
	}

	_, err := tx.Exec(`
INSERT INTO version_blob(path, version, data)
VALUES (?, ?, ?);
`,
		path, rec.Version, data,
	)
	return err
}

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "sqlite_busy") || strings.Contains(msg, "database is locked")
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func renamePathPrefixTx(tx *sql.Tx, table, oldPath, newPath, prefixPattern string, childSuffixStart int) error {
	query := fmt.Sprintf(`
UPDATE %s
SET path = CASE
    WHEN path = ? THEN ?
    ELSE ? || substr(path, ?)
END
WHERE path = ? OR path LIKE ? ESCAPE '\';
`, table)

	_, err := tx.Exec(query, oldPath, newPath, newPath, childSuffixStart, oldPath, prefixPattern)
	return err
}

func escapeSQLiteLikePattern(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

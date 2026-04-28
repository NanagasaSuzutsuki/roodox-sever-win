package db

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RestoreOptions struct {
	CreateSafetyBackup bool
}

type RestoreResult struct {
	SourcePath       string
	TargetPath       string
	SafetyBackupPath string
	SchemaVersion    int
}

func RestoreFromBackup(targetPath, backupPath string, options RestoreOptions) (RestoreResult, error) {
	targetAbs, err := filepath.Abs(strings.TrimSpace(targetPath))
	if err != nil {
		return RestoreResult{}, err
	}
	backupAbs, err := filepath.Abs(strings.TrimSpace(backupPath))
	if err != nil {
		return RestoreResult{}, err
	}
	if targetAbs == "" || backupAbs == "" {
		return RestoreResult{}, fmt.Errorf("target and backup paths are required")
	}
	if samePath(targetAbs, backupAbs) {
		return RestoreResult{}, fmt.Errorf("backup path must differ from target path")
	}

	lock, err := acquireResourceLock(targetAbs)
	if err != nil {
		return RestoreResult{}, err
	}
	defer func() {
		_ = lock.Release()
	}()

	if err := validateSQLiteFile(backupAbs); err != nil {
		return RestoreResult{}, fmt.Errorf("validate backup: %w", err)
	}

	result := RestoreResult{
		SourcePath: backupAbs,
		TargetPath: targetAbs,
	}

	targetInfo, targetExists, err := statIfExists(targetAbs)
	if err != nil {
		return RestoreResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
		return RestoreResult{}, err
	}

	if options.CreateSafetyBackup && targetExists {
		safetyPath := nextSafetyBackupPath(targetAbs)
		if err := copyFile(targetAbs, safetyPath, targetInfo.Mode()); err != nil {
			return RestoreResult{}, fmt.Errorf("create safety backup: %w", err)
		}
		result.SafetyBackupPath = safetyPath
	}

	tmpPath := nextRestoreTempPath(targetAbs)
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	backupInfo, err := os.Stat(backupAbs)
	if err != nil {
		return RestoreResult{}, err
	}
	if err := copyFile(backupAbs, tmpPath, backupInfo.Mode()); err != nil {
		return RestoreResult{}, fmt.Errorf("stage restore file: %w", err)
	}

	if err := removeSQLiteSidecars(targetAbs); err != nil {
		return RestoreResult{}, err
	}
	if err := removeIfExists(targetAbs); err != nil {
		return RestoreResult{}, err
	}
	if err := os.Rename(tmpPath, targetAbs); err != nil {
		return RestoreResult{}, err
	}

	if err := finalizeRestoredDatabase(targetAbs, &result); err != nil {
		if rollbackErr := rollbackRestore(targetAbs, result.SafetyBackupPath); rollbackErr != nil {
			return RestoreResult{}, fmt.Errorf("finalize restored database: %v; rollback failed: %v", err, rollbackErr)
		}
		return RestoreResult{}, fmt.Errorf("finalize restored database: %w", err)
	}

	return result, nil
}

func finalizeRestoredDatabase(targetPath string, result *RestoreResult) error {
	restoredDB, err := Open(targetPath)
	if err != nil {
		return err
	}
	defer restoredDB.Sql.Close()

	if err := restoredDB.QuickCheck(); err != nil {
		return err
	}
	version, err := restoredDB.SchemaVersion()
	if err != nil {
		return err
	}
	if result != nil {
		result.SchemaVersion = version
	}
	return nil
}

func rollbackRestore(targetPath, safetyBackupPath string) error {
	if strings.TrimSpace(safetyBackupPath) == "" {
		return nil
	}

	info, err := os.Stat(safetyBackupPath)
	if err != nil {
		return err
	}
	if err := removeSQLiteSidecars(targetPath); err != nil {
		return err
	}
	if err := removeIfExists(targetPath); err != nil {
		return err
	}
	return copyFile(safetyBackupPath, targetPath, info.Mode())
}

func validateSQLiteFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("sqlite file path is a directory: %s", path)
	}

	database, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer database.Close()

	var result string
	if err := database.QueryRow(`PRAGMA quick_check(1);`).Scan(&result); err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(result)) != "ok" {
		return fmt.Errorf("sqlite quick_check failed: %s", result)
	}
	return nil
}

func nextSafetyBackupPath(targetPath string) string {
	dir := filepath.Dir(targetPath)
	base := strings.TrimSuffix(filepath.Base(targetPath), filepath.Ext(targetPath))
	if base == "" {
		base = "roodox"
	}
	return filepath.Join(dir, fmt.Sprintf("%s-pre-restore-%s.db", base, time.Now().Format("20060102-150405")))
}

func nextRestoreTempPath(targetPath string) string {
	dir := filepath.Dir(targetPath)
	base := filepath.Base(targetPath)
	return filepath.Join(dir, fmt.Sprintf(".%s.restore-%d.tmp", base, time.Now().UnixNano()))
}

func statIfExists(path string) (os.FileInfo, bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		return info, true, nil
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return nil, false, err
}

func removeSQLiteSidecars(targetPath string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := removeIfExists(targetPath + suffix); err != nil {
			return err
		}
	}
	return nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func samePath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

package serverapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"roodox_server/internal/cleanup"
	"roodox_server/internal/db"
	"roodox_server/internal/server"
)

type DatabaseMaintenanceRuntimeConfig struct {
	CheckpointInterval time.Duration
	CheckpointMode     string
	BackupDir          string
	BackupInterval     time.Duration
	BackupKeepLatest   int
}

type databaseMaintenanceManager struct {
	database *db.DB
	cfg      DatabaseMaintenanceRuntimeConfig
	runner   *cleanup.Runner

	mu         sync.RWMutex
	checkpoint server.DatabaseCheckpointSnapshot
	backup     server.DatabaseBackupSnapshot
}

func newDatabaseMaintenanceManager(database *db.DB, cfg DatabaseMaintenanceRuntimeConfig) *databaseMaintenanceManager {
	if database == nil {
		return nil
	}

	cfg.CheckpointMode = strings.ToLower(strings.TrimSpace(cfg.CheckpointMode))
	if cfg.CheckpointMode == "" {
		cfg.CheckpointMode = "truncate"
	}
	if cfg.BackupKeepLatest < 0 {
		cfg.BackupKeepLatest = 0
	}

	m := &databaseMaintenanceManager{
		database: database,
		cfg:      cfg,
		checkpoint: server.DatabaseCheckpointSnapshot{
			Mode: cfg.CheckpointMode,
		},
		backup: server.DatabaseBackupSnapshot{
			Dir:             cfg.BackupDir,
			IntervalSeconds: int64(cfg.BackupInterval / time.Second),
			KeepLatest:      uint32(cfg.BackupKeepLatest),
		},
	}
	if cfg.CheckpointInterval > 0 || (cfg.BackupInterval > 0 && strings.TrimSpace(cfg.BackupDir) != "") {
		m.runner = cleanup.NewRunner(0, m.run)
	}
	return m
}

func (m *databaseMaintenanceManager) Close() {
	if m == nil || m.runner == nil {
		return
	}
	m.runner.Close()
}

func (m *databaseMaintenanceManager) Trigger() {
	if m == nil || m.runner == nil {
		return
	}
	m.runner.Trigger()
}

func (m *databaseMaintenanceManager) Snapshot() (server.DatabaseCheckpointSnapshot, server.DatabaseBackupSnapshot) {
	if m == nil {
		return server.DatabaseCheckpointSnapshot{}, server.DatabaseBackupSnapshot{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.checkpoint, m.backup
}

func (m *databaseMaintenanceManager) BackupNow(_ context.Context) (server.DatabaseBackupSnapshot, error) {
	if m == nil {
		return server.DatabaseBackupSnapshot{}, fmt.Errorf("database maintenance is not configured")
	}
	if strings.TrimSpace(m.cfg.BackupDir) == "" {
		return server.DatabaseBackupSnapshot{}, fmt.Errorf("database backup_dir is not configured")
	}

	_ = m.checkpointNow()
	if err := m.backupNow(); err != nil {
		return server.DatabaseBackupSnapshot{}, err
	}
	_, backup := m.Snapshot()
	return backup, nil
}

func (m *databaseMaintenanceManager) run(now time.Time) time.Time {
	if m == nil {
		return time.Time{}
	}

	nextDue := time.Time{}
	if due, shouldRun := m.nextCheckpointDue(now); !due.IsZero() {
		if shouldRun {
			_ = m.checkpointNow()
			due = now.Add(m.cfg.CheckpointInterval)
		}
		nextDue = earliestTime(nextDue, due)
	}
	if due, shouldRun := m.nextBackupDue(now); !due.IsZero() {
		if shouldRun {
			_ = m.backupNow()
			due = now.Add(m.cfg.BackupInterval)
		}
		nextDue = earliestTime(nextDue, due)
	}
	return nextDue
}

func (m *databaseMaintenanceManager) nextCheckpointDue(now time.Time) (time.Time, bool) {
	if m.cfg.CheckpointInterval <= 0 {
		return time.Time{}, false
	}

	m.mu.RLock()
	lastUnix := m.checkpoint.LastCheckpointAtUnix
	m.mu.RUnlock()
	if lastUnix == 0 {
		return now.Add(m.cfg.CheckpointInterval), false
	}
	last := time.Unix(lastUnix, 0)
	due := last.Add(m.cfg.CheckpointInterval)
	return due, !now.Before(due)
}

func (m *databaseMaintenanceManager) nextBackupDue(now time.Time) (time.Time, bool) {
	if m.cfg.BackupInterval <= 0 || strings.TrimSpace(m.cfg.BackupDir) == "" {
		return time.Time{}, false
	}

	m.mu.RLock()
	lastUnix := m.backup.LastBackupAtUnix
	m.mu.RUnlock()
	if lastUnix == 0 {
		return now.Add(m.cfg.BackupInterval), false
	}
	last := time.Unix(lastUnix, 0)
	due := last.Add(m.cfg.BackupInterval)
	return due, !now.Before(due)
}

func (m *databaseMaintenanceManager) checkpointNow() error {
	result, err := m.database.Checkpoint(m.cfg.CheckpointMode)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkpoint.Mode = m.cfg.CheckpointMode
	if err != nil {
		m.checkpoint.LastError = err.Error()
		return err
	}
	m.checkpoint.LastCheckpointAtUnix = result.CompletedAtUnix
	m.checkpoint.BusyReaders = result.BusyReaders
	m.checkpoint.LogFrames = result.LogFrames
	m.checkpoint.CheckpointedFrames = result.CheckpointedFrames
	m.checkpoint.LastError = ""
	return nil
}

func (m *databaseMaintenanceManager) backupNow() error {
	backupPath, createdAt, err := m.nextBackupPath()
	if err != nil {
		m.setBackupError(err)
		return err
	}
	if err := m.database.BackupInto(backupPath); err != nil {
		m.setBackupError(err)
		return err
	}
	if err := m.cleanupOldBackups(); err != nil {
		m.setBackupError(err)
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.backup.LastBackupAtUnix = createdAt.Unix()
	m.backup.LastBackupPath = backupPath
	m.backup.LastError = ""
	return nil
}

func (m *databaseMaintenanceManager) setBackupError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil {
		m.backup.LastError = ""
		return
	}
	m.backup.LastError = err.Error()
}

func (m *databaseMaintenanceManager) nextBackupPath() (string, time.Time, error) {
	dir := strings.TrimSpace(m.cfg.BackupDir)
	if dir == "" {
		return "", time.Time{}, fmt.Errorf("database backup_dir is not configured")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", time.Time{}, err
	}

	now := time.Now()
	base := strings.TrimSuffix(filepath.Base(m.database.Path()), filepath.Ext(m.database.Path()))
	if base == "" {
		base = "roodox"
	}
	name := fmt.Sprintf("%s-backup-%s.db", base, now.Format("20060102-150405"))
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		name = fmt.Sprintf("%s-backup-%s-%09d.db", base, now.Format("20060102-150405"), now.Nanosecond())
		path = filepath.Join(dir, name)
	} else if err != nil && !os.IsNotExist(err) {
		return "", time.Time{}, err
	}
	return path, now, nil
}

func (m *databaseMaintenanceManager) cleanupOldBackups() error {
	if m.cfg.BackupKeepLatest <= 0 || strings.TrimSpace(m.cfg.BackupDir) == "" {
		return nil
	}

	base := strings.TrimSuffix(filepath.Base(m.database.Path()), filepath.Ext(m.database.Path()))
	pattern := filepath.Join(m.cfg.BackupDir, base+"-backup-*.db")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	if len(matches) <= m.cfg.BackupKeepLatest {
		return nil
	}

	sort.Slice(matches, func(i, j int) bool {
		leftInfo, leftErr := os.Stat(matches[i])
		rightInfo, rightErr := os.Stat(matches[j])
		if leftErr != nil || rightErr != nil {
			return matches[i] < matches[j]
		}
		if leftInfo.ModTime().Equal(rightInfo.ModTime()) {
			return matches[i] > matches[j]
		}
		return leftInfo.ModTime().After(rightInfo.ModTime())
	})

	for _, path := range matches[m.cfg.BackupKeepLatest:] {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func earliestTime(current, candidate time.Time) time.Time {
	if candidate.IsZero() {
		return current
	}
	if current.IsZero() || candidate.Before(current) {
		return candidate
	}
	return current
}

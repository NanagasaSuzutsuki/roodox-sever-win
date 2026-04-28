package serverapp

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"roodox_server/internal/cleanup"
	"roodox_server/internal/fs"
)

type backgroundJanitor interface {
	Close()
	Trigger()
}

type ConflictFileCleanupRuntimeConfig struct {
	Enabled   bool
	Interval  time.Duration
	Retention time.Duration
	MaxBytes  int64
}

type LogCleanupRuntimeConfig struct {
	Enabled   bool
	Interval  time.Duration
	Retention time.Duration
	MaxBytes  int64
	Patterns  []string
}

type conflictFileJanitor struct {
	rootDir   string
	interval  time.Duration
	retention time.Duration
	maxBytes  int64

	runner *cleanup.Runner
}

type logFileJanitor struct {
	dir       string
	interval  time.Duration
	retention time.Duration
	maxBytes  int64
	patterns  []string

	runner *cleanup.Runner
}

func newConflictFileJanitor(rootDir string, cfg ConflictFileCleanupRuntimeConfig) *conflictFileJanitor {
	if strings.TrimSpace(rootDir) == "" || !cfg.Enabled {
		return nil
	}
	if cfg.Retention <= 0 && cfg.MaxBytes <= 0 {
		return nil
	}
	if cfg.Interval < 0 {
		cfg.Interval = 0
	}

	j := &conflictFileJanitor{
		rootDir:   rootDir,
		interval:  cfg.Interval,
		retention: cfg.Retention,
		maxBytes:  cfg.MaxBytes,
	}
	j.runner = cleanup.NewRunner(j.interval, j.cleanup)
	return j
}

func (j *conflictFileJanitor) Close() {
	if j == nil {
		return
	}
	if j.runner != nil {
		j.runner.Close()
	}
}

func (j *conflictFileJanitor) Trigger() {
	if j == nil {
		return
	}
	if j.runner != nil {
		j.runner.Trigger()
		return
	}
	j.cleanup(time.Now())
}

func (j *conflictFileJanitor) cleanup(now time.Time) time.Time {
	items, err := j.collectItems()
	if err != nil {
		log.Printf("component=janitor op=collect_conflict_files root=%q error=%q", j.rootDir, err.Error())
		return time.Time{}
	}
	removedCount, removedBytes, nextDue := cleanupFilesByPolicy(items, now, j.retention, j.maxBytes, j.removeItem)
	if removedCount > 0 {
		log.Printf("component=janitor op=cleanup_conflict_files root=%q removed_count=%d removed_bytes=%d", j.rootDir, removedCount, removedBytes)
	}
	return nextDue
}

func (j *conflictFileJanitor) collectItems() ([]artifactCleanupItem, error) {
	items := make([]artifactCleanupItem, 0)
	err := filepath.WalkDir(j.rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != j.rootDir && fs.ShouldIgnore(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !fs.IsConflictPath(d.Name()) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		items = append(items, artifactCleanupItem{
			path:    path,
			name:    d.Name(),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
		return nil
	})
	return items, err
}

func (j *conflictFileJanitor) removeItem(item artifactCleanupItem, reason string) bool {
	if err := os.Remove(item.path); err != nil && !os.IsNotExist(err) {
		log.Printf("component=janitor op=remove_conflict_file path=%q reason=%q error=%q", item.path, reason, err.Error())
		return false
	}
	return true
}

func newLogFileJanitor(dir string, cfg LogCleanupRuntimeConfig) *logFileJanitor {
	if strings.TrimSpace(dir) == "" || !cfg.Enabled {
		return nil
	}
	if cfg.Retention <= 0 && cfg.MaxBytes <= 0 {
		return nil
	}
	if len(cfg.Patterns) == 0 {
		return nil
	}
	if cfg.Interval < 0 {
		cfg.Interval = 0
	}

	j := &logFileJanitor{
		dir:       dir,
		interval:  cfg.Interval,
		retention: cfg.Retention,
		maxBytes:  cfg.MaxBytes,
		patterns:  append([]string(nil), cfg.Patterns...),
	}
	j.runner = cleanup.NewRunner(j.interval, j.cleanup)
	return j
}

func (j *logFileJanitor) Close() {
	if j == nil {
		return
	}
	if j.runner != nil {
		j.runner.Close()
	}
}

func (j *logFileJanitor) Trigger() {
	if j == nil {
		return
	}
	if j.runner != nil {
		j.runner.Trigger()
		return
	}
	j.cleanup(time.Now())
}

func (j *logFileJanitor) cleanup(now time.Time) time.Time {
	items, err := j.collectItems()
	if err != nil {
		log.Printf("component=janitor op=collect_log_files dir=%q error=%q", j.dir, err.Error())
		return time.Time{}
	}
	removedCount, removedBytes, nextDue := cleanupFilesByPolicy(items, now, j.retention, j.maxBytes, j.removeItem)
	if removedCount > 0 {
		log.Printf("component=janitor op=cleanup_log_files dir=%q removed_count=%d removed_bytes=%d", j.dir, removedCount, removedBytes)
	}
	return nextDue
}

func (j *logFileJanitor) collectItems() ([]artifactCleanupItem, error) {
	entries, err := os.ReadDir(j.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	items := make([]artifactCleanupItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		if !matchesLogPattern(name, j.patterns) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		items = append(items, artifactCleanupItem{
			path:    filepath.Join(j.dir, name),
			name:    name,
			size:    info.Size(),
			modTime: info.ModTime(),
		})
	}
	return items, nil
}

func (j *logFileJanitor) removeItem(item artifactCleanupItem, reason string) bool {
	if err := os.Remove(item.path); err != nil && !os.IsNotExist(err) {
		log.Printf("component=janitor op=remove_log_file path=%q reason=%q error=%q", item.path, reason, err.Error())
		return false
	}
	return true
}

func cleanupFilesByPolicy(items []artifactCleanupItem, now time.Time, retention time.Duration, maxBytes int64, remove func(item artifactCleanupItem, reason string) bool) (int, int64, time.Time) {
	if len(items) == 0 {
		return 0, 0, time.Time{}
	}

	sort.Slice(items, func(i, k int) bool {
		if items[i].modTime.Equal(items[k].modTime) {
			return items[i].name < items[k].name
		}
		return items[i].modTime.Before(items[k].modTime)
	})

	removedCount := 0
	removedBytes := int64(0)
	kept := make([]artifactCleanupItem, 0, len(items))
	for _, item := range items {
		if retention > 0 && now.Sub(item.modTime) > retention {
			if remove(item, "retention") {
				removedCount++
				removedBytes += item.size
			}
			continue
		}
		kept = append(kept, item)
	}

	if maxBytes > 0 {
		totalBytes := int64(0)
		for _, item := range kept {
			totalBytes += item.size
		}
		finalKept := kept[:0]
		for _, item := range kept {
			if totalBytes <= maxBytes {
				finalKept = append(finalKept, item)
				continue
			}
			if remove(item, "max_bytes") {
				removedCount++
				removedBytes += item.size
				totalBytes -= item.size
				continue
			}
			finalKept = append(finalKept, item)
		}
		kept = finalKept
	}

	nextDue := time.Time{}
	if retention > 0 {
		for _, item := range kept {
			expireAt := item.modTime.Add(retention)
			if nextDue.IsZero() || expireAt.Before(nextDue) {
				nextDue = expireAt
			}
		}
	}

	return removedCount, removedBytes, nextDue
}

func matchesLogPattern(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, err := filepath.Match(pattern, name); err == nil && matched {
			return true
		}
		if strings.EqualFold(pattern, name) {
			return true
		}
	}
	return false
}

package serverapp

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"roodox_server/internal/cleanup"
)

type artifactJanitor struct {
	rootDir   string
	interval  time.Duration
	retention time.Duration
	maxBytes  int64
	prefixes  []string

	runner *cleanup.Runner
}

type artifactCleanupItem struct {
	path    string
	name    string
	size    int64
	modTime time.Time
}

func newArtifactJanitor(rootDir string, cfg ArtifactCleanupRuntimeConfig) *artifactJanitor {
	if strings.TrimSpace(rootDir) == "" || !cfg.Enabled {
		return nil
	}
	if len(cfg.Prefixes) == 0 {
		return nil
	}
	if cfg.Retention <= 0 && cfg.MaxBytes <= 0 {
		return nil
	}
	if cfg.Interval < 0 {
		cfg.Interval = 0
	}

	j := &artifactJanitor{
		rootDir:   rootDir,
		interval:  cfg.Interval,
		retention: cfg.Retention,
		maxBytes:  cfg.MaxBytes,
		prefixes:  append([]string(nil), cfg.Prefixes...),
	}
	j.runner = cleanup.NewRunner(j.interval, j.cleanup)
	return j
}

func (j *artifactJanitor) Close() {
	if j == nil {
		return
	}
	if j.runner != nil {
		j.runner.Close()
	}
}

func (j *artifactJanitor) Trigger() {
	if j == nil {
		return
	}
	if j.runner != nil {
		j.runner.Trigger()
		return
	}
	j.cleanup(time.Now())
}

func (j *artifactJanitor) cleanup(now time.Time) time.Time {
	items, err := j.collectItems()
	if err != nil {
		log.Printf("component=janitor op=collect_artifacts root=%q error=%q", j.rootDir, err.Error())
		return time.Time{}
	}
	if len(items) == 0 {
		return time.Time{}
	}
	removedCount, removedBytes, nextDue := cleanupFilesByPolicy(items, now, j.retention, j.maxBytes, j.removeItem)

	if removedCount > 0 {
		log.Printf(
			"component=janitor op=cleanup_artifacts root=%q removed_count=%d removed_bytes=%d",
			j.rootDir,
			removedCount,
			removedBytes,
		)
	}
	return nextDue
}

func (j *artifactJanitor) collectItems() ([]artifactCleanupItem, error) {
	entries, err := os.ReadDir(j.rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	items := make([]artifactCleanupItem, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !hasArtifactPrefix(name, j.prefixes) {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}

		path := filepath.Join(j.rootDir, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}
		size, err := pathSize(path)
		if err != nil {
			log.Printf("component=janitor op=stat_artifact path=%q error=%q", path, err.Error())
			continue
		}
		items = append(items, artifactCleanupItem{
			path:    path,
			name:    name,
			size:    size,
			modTime: info.ModTime(),
		})
	}

	return items, nil
}

func (j *artifactJanitor) removeItem(item artifactCleanupItem, reason string) bool {
	if err := os.RemoveAll(item.path); err != nil && !os.IsNotExist(err) {
		log.Printf("component=janitor op=remove_artifact path=%q reason=%q error=%q", item.path, reason, err.Error())
		return false
	}
	return true
}

func hasArtifactPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func pathSize(path string) (int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return info.Size(), nil
	}

	var total int64
	err = filepath.Walk(path, func(current string, currentInfo os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if currentInfo.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if currentInfo.IsDir() {
			return nil
		}
		total += currentInfo.Size()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

type ArtifactCleanupRuntimeConfig struct {
	Enabled   bool
	Interval  time.Duration
	Retention time.Duration
	MaxBytes  int64
	Prefixes  []string
}

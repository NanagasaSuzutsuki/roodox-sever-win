package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"roodox_server/internal/conflict"
	"roodox_server/internal/db"
	"roodox_server/internal/fs"
)

type fileHeadState struct {
	Exists    bool
	Deleted   bool
	Version   uint64
	Size      int64
	MtimeUnix int64
	Hash      string
}

type mutationSupport struct {
	fsys         *fs.FileSystem
	metaStore    *db.MetaStore
	versionStore *db.VersionStore
	cleanupHooks CleanupHooks
	metrics      RuntimeMetrics
}

func newMutationSupport(fsys *fs.FileSystem, metaStore *db.MetaStore, versionStore *db.VersionStore) *mutationSupport {
	return &mutationSupport{
		fsys:         fsys,
		metaStore:    metaStore,
		versionStore: versionStore,
	}
}

func (s *mutationSupport) ConfigureCleanupHooks(hooks CleanupHooks) {
	if s == nil {
		return
	}
	s.cleanupHooks = hooks
}

func (s *mutationSupport) ConfigureMetrics(metrics RuntimeMetrics) {
	if s == nil {
		return
	}
	s.metrics = metrics
}

func (s *mutationSupport) ResolveFileHead(path string) (fileHeadState, error) {
	var head fileHeadState

	if s == nil {
		return head, nil
	}

	if s.versionStore != nil {
		rec, err := s.versionStore.GetLatestRecord(path)
		switch {
		case err == nil:
			head.Version = rec.Version
			head.MtimeUnix = rec.MtimeUnix
			head.Hash = rec.Hash
			if rec.ChangeType == "delete" {
				head.Deleted = true
				head.Size = 0
			} else {
				head.Size = rec.Size
			}
		case errors.Is(err, os.ErrNotExist):
		default:
			return fileHeadState{}, err
		}
	}

	if s.metaStore != nil {
		meta, err := s.metaStore.GetMeta(path)
		switch {
		case err == nil:
			head.Exists = true
			head.Deleted = false
			head.Version = meta.Version
			head.Size = meta.Size
			head.MtimeUnix = meta.MtimeUnix
			head.Hash = meta.Hash
			return head, nil
		case errors.Is(err, os.ErrNotExist):
		default:
			return fileHeadState{}, err
		}
	}

	if head.Deleted {
		return head, nil
	}

	if s.fsys == nil {
		return head, nil
	}

	info, err := s.fsys.Stat(path)
	switch {
	case err == nil:
		head.Exists = true
		head.Size = info.Size()
		head.MtimeUnix = info.ModTime().Unix()
		return head, nil
	case errors.Is(err, os.ErrNotExist):
		return head, nil
	default:
		return fileHeadState{}, err
	}
}

func (s *mutationSupport) SaveFileSnapshot(ctx context.Context, path string, existed bool, data []byte) (uint64, error) {
	if s == nil || s.versionStore == nil {
		return 0, nil
	}

	now := time.Now().Unix()
	changeType := "modify"
	if !existed {
		changeType = "create"
	}

	newVersion, err := s.versionStore.SaveFileSnapshot(&fs.Meta{
		Path:      path,
		IsDir:     false,
		Size:      int64(len(data)),
		MtimeUnix: now,
		Hash:      fs.ComputeHash(data),
	}, "", RequestIDFromContext(ctx), changeType, data)
	if err != nil {
		return 0, err
	}

	s.cleanupHooks.TriggerMutation()
	return newVersion, nil
}

func (s *mutationSupport) writeConflictFile(path string, data []byte, perm os.FileMode) (string, error) {
	const maxConflictWriteAttempts = 32

	for attempt := 0; attempt < maxConflictWriteAttempts; attempt++ {
		conflictPath := conflict.GenerateConflictPath(path)
		if err := s.writeFileExclusive(conflictPath, data, perm); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", err
		}
		s.cleanupHooks.TriggerConflict()
		return conflictPath, nil
	}

	return "", os.ErrExist
}

func (s *mutationSupport) writeFileExclusive(path string, data []byte, perm os.FileMode) error {
	if s == nil || s.fsys == nil {
		return os.ErrInvalid
	}

	fullPath, err := fs.ResolvePath(s.fsys.RootDir, path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}

	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(fullPath)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(fullPath)
		return closeErr
	}
	return nil
}

func (s *mutationSupport) loadVersionSnapshot(path string, version uint64) ([]byte, error) {
	if version == 0 {
		return []byte{}, nil
	}
	if s == nil || s.versionStore == nil {
		return nil, os.ErrNotExist
	}

	rec, err := s.versionStore.GetRecord(path, version)
	if err != nil {
		return nil, err
	}
	if rec.ChangeType == "delete" {
		return []byte{}, nil
	}

	return s.versionStore.GetVersionData(path, version)
}

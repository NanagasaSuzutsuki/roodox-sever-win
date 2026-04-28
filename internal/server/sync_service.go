package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"roodox_server/internal/db"
	"roodox_server/internal/fs"
	pb "roodox_server/proto"
)

const (
	defaultConflictFileTTL          = 7 * 24 * time.Hour
	defaultConflictMaxCopiesPerPath = 20
	defaultConflictCleanupInterval  = time.Hour
)

type SyncService struct {
	pb.UnimplementedSyncServiceServer

	FS           *fs.FileSystem
	MetaStore    *db.MetaStore
	VersionStore *db.VersionStore
	pathLocker   *PathLocker
	mutations    *mutationSupport

	conflictFileTTL          time.Duration
	conflictMaxCopiesPerPath int
	conflictCleanupInterval  time.Duration
	conflictCleanupMu        sync.Mutex
	lastConflictCleanup      time.Time
	cleanupHooks             CleanupHooks
}

type ConflictCleanupPolicy struct {
	FileTTL          time.Duration
	MaxCopiesPerPath int
	CleanupInterval  time.Duration
}

func NewSyncService(fsys *fs.FileSystem, meta *db.MetaStore, ver *db.VersionStore, pathLocker *PathLocker) *SyncService {
	if pathLocker == nil {
		pathLocker = NewPathLocker()
	}
	mutations := newMutationSupport(fsys, meta, ver)
	return &SyncService{
		FS:                       fsys,
		MetaStore:                meta,
		VersionStore:             ver,
		pathLocker:               pathLocker,
		mutations:                mutations,
		conflictFileTTL:          defaultConflictFileTTL,
		conflictMaxCopiesPerPath: defaultConflictMaxCopiesPerPath,
		conflictCleanupInterval:  defaultConflictCleanupInterval,
	}
}

func (s *SyncService) ConfigureConflictCleanup(policy ConflictCleanupPolicy) {
	if s == nil {
		return
	}
	s.conflictFileTTL = policy.FileTTL
	s.conflictMaxCopiesPerPath = policy.MaxCopiesPerPath
	s.conflictCleanupInterval = policy.CleanupInterval
}

func (s *SyncService) ConfigureCleanupHooks(hooks CleanupHooks) {
	if s == nil {
		return
	}
	s.cleanupHooks = hooks
	if s.mutations != nil {
		s.mutations.ConfigureCleanupHooks(hooks)
	}
}

func (s *SyncService) ConfigureMetrics(metrics RuntimeMetrics) {
	if s == nil || s.mutations == nil {
		return
	}
	s.mutations.ConfigureMetrics(metrics)
}

func (s *SyncService) GetFileMeta(ctx context.Context, req *pb.GetFileMetaRequest) (*pb.GetFileMetaResponse, error) {
	path, err := normalizeProjectPath(s.FS.RootDir, req.Path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	m, err := s.MetaStore.GetMeta(path)
	if err == nil {
		return &pb.GetFileMetaResponse{
			Meta: &pb.FileMeta{
				Path:      m.Path,
				Version:   m.Version,
				MtimeUnix: m.MtimeUnix,
				Hash:      m.Hash,
				Size:      m.Size,
			},
		}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, toGrpcError(err)
	}

	info, statErr := s.FS.Stat(path)
	if statErr == nil {
		return &pb.GetFileMetaResponse{
			Meta: &pb.FileMeta{
				Path:      path,
				MtimeUnix: info.ModTime().Unix(),
				Size:      info.Size(),
			},
		}, nil
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return nil, toGrpcError(statErr)
	}

	return &pb.GetFileMetaResponse{Meta: &pb.FileMeta{Path: path}}, nil
}

func (s *SyncService) WriteFile(ctx context.Context, req *pb.WriteFileRequest) (*pb.WriteFileResponse, error) {
	return s.doWriteFile(ctx, req.Path, req.Data, req.BaseVersion)
}

func (s *SyncService) doWriteFile(ctx context.Context, path string, data []byte, baseVersion uint64) (*pb.WriteFileResponse, error) {
	path, err := normalizeProjectPath(s.FS.RootDir, path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	unlock := s.pathLocker.Lock(path)
	defer unlock()
	LogRequestEvent(ctx, "component=sync op=WriteFile path=%q base_version=%d", path, baseVersion)

	head, err := s.mutations.ResolveFileHead(path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	if baseVersion != 0 && head.Version != 0 && baseVersion != head.Version {
		conflictPath, err := s.writeConflictFile(path, data, 0o644)
		if err != nil {
			return nil, toGrpcError(err)
		}
		LogRequestEvent(ctx, "component=sync op=WriteFile conflict_path=%q path=%q base_version=%d current_version=%d", conflictPath, path, baseVersion, head.Version)
		return &pb.WriteFileResponse{
			Conflicted:   true,
			ConflictPath: conflictPath,
			NewVersion:   head.Version,
		}, nil
	}

	newHash := fs.ComputeHash(data)
	if head.Exists && head.Version != 0 && newHash == head.Hash {
		if _, err := s.FS.Stat(path); err == nil {
			return &pb.WriteFileResponse{
				Conflicted:   false,
				ConflictPath: "",
				NewVersion:   head.Version,
			}, nil
		}
	}

	if err := s.FS.WriteFile(path, data, 0o644); err != nil {
		return nil, toGrpcError(err)
	}

	newVersion, err := s.mutations.SaveFileSnapshot(ctx, path, head.Exists, data)
	if err != nil {
		return nil, toGrpcError(err)
	}

	return &pb.WriteFileResponse{
		Conflicted:   false,
		ConflictPath: "",
		NewVersion:   newVersion,
	}, nil
}

func (s *SyncService) ListChangedFiles(ctx context.Context, req *pb.ListChangedFilesRequest) (*pb.ListChangedFilesResponse, error) {
	if req.SinceVersion != 0 {
		return nil, status.Error(codes.InvalidArgument, "since_version is not supported; use since_mtime_unix for incremental scans")
	}

	metas, err := s.MetaStore.AllMetas()
	if err != nil {
		return nil, toGrpcError(err)
	}
	resp := &pb.ListChangedFilesResponse{}

	for _, m := range metas {
		if req.SinceMtimeUnix != 0 && m.MtimeUnix <= req.SinceMtimeUnix {
			continue
		}

		resp.Metas = append(resp.Metas, &pb.FileMeta{
			Path:      m.Path,
			Version:   m.Version,
			MtimeUnix: m.MtimeUnix,
			Hash:      m.Hash,
			Size:      m.Size,
		})
	}

	return resp, nil
}

func (s *SyncService) writeConflictFile(path string, data []byte, perm os.FileMode) (string, error) {
	conflictPath, err := s.mutations.writeConflictFile(path, data, perm)
	if err != nil {
		return "", fmt.Errorf("generate unique conflict file for %q: %w", path, err)
	}

	now := time.Now()
	if err := s.cleanupConflictCopiesForPath(path, now); err != nil {
		log.Printf("component=sync op=cleanup_conflict_copies path=%q error=%q", path, err.Error())
	}
	s.maybeCleanupExpiredConflictFiles(now)
	return conflictPath, nil
}

type conflictFileInfo struct {
	fullPath string
	name     string
	modTime  time.Time
}

func (s *SyncService) cleanupConflictCopiesForPath(path string, now time.Time) error {
	fullPath, err := fs.ResolvePath(s.FS.RootDir, path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(fullPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	prefix := filepath.Base(fullPath) + ".roodox-conflict-"
	candidates := make([]conflictFileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !stringsHasConflictPrefix(name, prefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, conflictFileInfo{
			fullPath: filepath.Join(dir, name),
			name:     name,
			modTime:  info.ModTime(),
		})
	}

	if len(candidates) == 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].name > candidates[j].name
		}
		return candidates[i].modTime.After(candidates[j].modTime)
	})

	for idx, candidate := range candidates {
		remove := false
		if s.conflictFileTTL > 0 && now.Sub(candidate.modTime) > s.conflictFileTTL {
			remove = true
		}
		if !remove && s.conflictMaxCopiesPerPath > 0 && idx >= s.conflictMaxCopiesPerPath {
			remove = true
		}
		if !remove {
			continue
		}
		if err := os.Remove(candidate.fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("component=sync op=cleanup_conflict_copy file=%q error=%q", candidate.fullPath, err.Error())
		}
	}

	return nil
}

func (s *SyncService) maybeCleanupExpiredConflictFiles(now time.Time) {
	if s.conflictFileTTL <= 0 || s.conflictCleanupInterval <= 0 {
		return
	}

	s.conflictCleanupMu.Lock()
	defer s.conflictCleanupMu.Unlock()

	if !s.lastConflictCleanup.IsZero() && now.Sub(s.lastConflictCleanup) < s.conflictCleanupInterval {
		return
	}
	s.lastConflictCleanup = now

	if err := s.cleanupExpiredConflictFiles(now); err != nil {
		log.Printf("component=sync op=cleanup_expired_conflicts root=%q error=%q", s.FS.RootDir, err.Error())
	}
}

func (s *SyncService) cleanupExpiredConflictFiles(now time.Time) error {
	if s.conflictFileTTL <= 0 {
		return nil
	}

	return filepath.WalkDir(s.FS.RootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != s.FS.RootDir && fs.ShouldIgnore(d.Name()) {
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
		if now.Sub(info.ModTime()) <= s.conflictFileTTL {
			return nil
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("component=sync op=remove_expired_conflict file=%q error=%q", path, err.Error())
		}
		return nil
	})
}

func stringsHasConflictPrefix(name, prefix string) bool {
	return len(name) > len(prefix) && name[:len(prefix)] == prefix
}

func (s *SyncService) WriteFileStream(stream pb.SyncService_WriteFileStreamServer) error {
	var (
		path        string
		baseVersion uint64
		buf         bytes.Buffer
		first       = true
	)

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			if first {
				return status.Error(codes.InvalidArgument, "write stream missing first chunk")
			}
			resp, werr := s.doWriteFile(stream.Context(), path, buf.Bytes(), baseVersion)
			if werr != nil {
				return werr
			}
			return stream.SendAndClose(resp)
		}
		if err != nil {
			return err
		}

		if first {
			path = chunk.Path
			baseVersion = chunk.BaseVersion
			LogRequestEvent(stream.Context(), "component=sync op=WriteFileStream path=%q", path)
			first = false
		}
		buf.Write(chunk.Data)
	}
}

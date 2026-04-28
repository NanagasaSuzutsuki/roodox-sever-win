package server

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"roodox_server/internal/db"
	"roodox_server/internal/fs"
	pb "roodox_server/proto"
)

type CoreService struct {
	pb.UnimplementedCoreServiceServer

	FS           *fs.FileSystem
	MetaStore    *db.MetaStore
	VersionStore *db.VersionStore
	pathLocker   *PathLocker
	cleanupHooks CleanupHooks
	mutations    *mutationSupport
}

func NewCoreService(fsys *fs.FileSystem, meta *db.MetaStore, ver *db.VersionStore, pathLocker *PathLocker) *CoreService {
	if pathLocker == nil {
		pathLocker = NewPathLocker()
	}
	mutations := newMutationSupport(fsys, meta, ver)
	return &CoreService{
		FS:           fsys,
		MetaStore:    meta,
		VersionStore: ver,
		pathLocker:   pathLocker,
		mutations:    mutations,
	}
}

func (s *CoreService) ConfigureCleanupHooks(hooks CleanupHooks) {
	if s == nil {
		return
	}
	s.cleanupHooks = hooks
	if s.mutations != nil {
		s.mutations.ConfigureCleanupHooks(hooks)
	}
}

func (s *CoreService) ConfigureMetrics(metrics RuntimeMetrics) {
	if s == nil || s.mutations == nil {
		return
	}
	s.mutations.ConfigureMetrics(metrics)
}

func joinRelPath(dir, name string) string {
	if dir == "" || dir == "." {
		return name
	}
	return filepath.ToSlash(filepath.Join(dir, name))
}

func (s *CoreService) ListDir(ctx context.Context, req *pb.ListDirRequest) (*pb.ListDirResponse, error) {
	path, err := normalizeProjectPath(s.FS.RootDir, req.Path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	entries, err := s.FS.ListDir(path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	resp := &pb.ListDirResponse{}
	for _, e := range entries {
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if fs.ShouldIgnore(e.Name()) {
			continue
		}

		relPath := joinRelPath(path, e.Name())
		m, err := s.MetaStore.GetMeta(relPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, toGrpcError(err)
		}
		version := uint64(0)
		hash := ""
		if err == nil {
			version = m.Version
			hash = m.Hash
		}

		resp.Entries = append(resp.Entries, &pb.FileInfo{
			Path:      relPath,
			Name:      e.Name(),
			IsDir:     e.IsDir(),
			Size:      fi.Size(),
			MtimeUnix: fi.ModTime().Unix(),
			Version:   version,
			Hash:      hash,
		})
	}
	return resp, nil
}

func (s *CoreService) Stat(ctx context.Context, req *pb.StatRequest) (*pb.StatResponse, error) {
	path, err := normalizeProjectPath(s.FS.RootDir, req.Path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	fi, err := s.FS.Stat(path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	m, err := s.MetaStore.GetMeta(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, toGrpcError(err)
	}
	version := uint64(0)
	hash := ""
	if err == nil {
		version = m.Version
		hash = m.Hash
	}

	return &pb.StatResponse{
		Info: &pb.FileInfo{
			Path:      path,
			Name:      fi.Name(),
			IsDir:     fi.IsDir(),
			Size:      fi.Size(),
			MtimeUnix: fi.ModTime().Unix(),
			Version:   version,
			Hash:      hash,
		},
	}, nil
}

func (s *CoreService) ReadFile(ctx context.Context, req *pb.ReadFileRequest) (*pb.ReadFileResponse, error) {
	path, err := normalizeProjectPath(s.FS.RootDir, req.Path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	data, err := s.FS.ReadFile(path)
	if err != nil {
		return nil, toGrpcError(err)
	}
	return &pb.ReadFileResponse{Data: data}, nil
}

func (s *CoreService) ReadFileRange(ctx context.Context, req *pb.ReadFileRangeRequest) (*pb.ReadFileRangeResponse, error) {
	path, err := normalizeProjectPath(s.FS.RootDir, req.Path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	data, size, err := s.FS.ReadAt(path, req.Offset, req.Length)
	if err != nil {
		return nil, toGrpcError(err)
	}
	return &pb.ReadFileRangeResponse{Data: data, FileSize: size}, nil
}

func (s *CoreService) WriteFileRange(ctx context.Context, req *pb.WriteFileRangeRequest) (*pb.WriteFileRangeResponse, error) {
	if req.Offset < 0 {
		return nil, invalidArgument("offset must be >= 0")
	}

	path, err := normalizeProjectPath(s.FS.RootDir, req.Path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	unlock := s.pathLocker.Lock(path)
	defer unlock()
	LogRequestEvent(ctx, "component=core op=WriteFileRange path=%q base_version=%d", path, req.BaseVersion)

	head, err := s.mutations.ResolveFileHead(path)
	if err != nil {
		return nil, toGrpcError(err)
	}
	conflicted := false
	defer func() {
		if s.mutations != nil && s.mutations.metrics != nil {
			s.mutations.metrics.RecordRangeWrite(path, int64(len(req.Data)), conflicted)
		}
	}()

	if req.BaseVersion != 0 && head.Version != 0 && req.BaseVersion != head.Version {
		conflicted = true
		conflictPath := ""
		conflictData, buildErr := s.buildRangeConflictData(path, req.BaseVersion, req.Offset, req.Data)
		if buildErr != nil {
			log.Printf("component=core op=WriteFileRange build_conflict_data_failed path=%q base_version=%d current_version=%d error=%q", path, req.BaseVersion, head.Version, buildErr.Error())
		} else {
			conflictPath, buildErr = s.mutations.writeConflictFile(path, conflictData, 0o644)
			if buildErr != nil {
				log.Printf("component=core op=WriteFileRange write_conflict_file_failed path=%q base_version=%d current_version=%d error=%q", path, req.BaseVersion, head.Version, buildErr.Error())
				conflictPath = ""
			}
		}
		LogRequestEvent(ctx, "component=core op=WriteFileRange conflict_path=%q path=%q base_version=%d current_version=%d", conflictPath, path, req.BaseVersion, head.Version)
		return &pb.WriteFileRangeResponse{
			BytesWritten: 0,
			FileSize:     head.Size,
			NewVersion:   head.Version,
			Conflicted:   true,
			ConflictPath: conflictPath,
		}, nil
	}

	bytesWritten, fileSize, err := s.FS.WriteAt(path, req.Offset, req.Data, 0o644)
	if err != nil {
		return nil, toGrpcError(err)
	}

	fullData, err := s.FS.ReadFile(path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	newVersion, err := s.mutations.SaveFileSnapshot(ctx, path, head.Exists, fullData)
	if err != nil {
		return nil, toGrpcError(err)
	}

	return &pb.WriteFileRangeResponse{
		BytesWritten: bytesWritten,
		FileSize:     fileSize,
		NewVersion:   newVersion,
	}, nil
}

func (s *CoreService) SetFileSize(ctx context.Context, req *pb.SetFileSizeRequest) (*pb.SetFileSizeResponse, error) {
	if req.Size < 0 {
		return nil, invalidArgument("size must be >= 0")
	}

	path, err := normalizeProjectPath(s.FS.RootDir, req.Path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	unlock := s.pathLocker.Lock(path)
	defer unlock()
	LogRequestEvent(ctx, "component=core op=SetFileSize path=%q size=%d base_version=%d", path, req.Size, req.BaseVersion)

	head, err := s.mutations.ResolveFileHead(path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	if req.BaseVersion != 0 && head.Version != 0 && req.BaseVersion != head.Version {
		conflictPath := ""
		conflictData, buildErr := s.buildSetFileSizeConflictData(path, req.BaseVersion, req.Size)
		if buildErr != nil {
			log.Printf("component=core op=SetFileSize build_conflict_data_failed path=%q base_version=%d current_version=%d error=%q", path, req.BaseVersion, head.Version, buildErr.Error())
		} else {
			conflictPath, buildErr = s.mutations.writeConflictFile(path, conflictData, 0o644)
			if buildErr != nil {
				log.Printf("component=core op=SetFileSize write_conflict_file_failed path=%q base_version=%d current_version=%d error=%q", path, req.BaseVersion, head.Version, buildErr.Error())
				conflictPath = ""
			}
		}
		LogRequestEvent(ctx, "component=core op=SetFileSize conflict_path=%q path=%q size=%d base_version=%d current_version=%d", conflictPath, path, req.Size, req.BaseVersion, head.Version)
		return &pb.SetFileSizeResponse{
			FileSize:     head.Size,
			NewVersion:   head.Version,
			Conflicted:   true,
			ConflictPath: conflictPath,
		}, nil
	}

	if head.Exists && head.Size == req.Size {
		if _, statErr := s.FS.Stat(path); statErr == nil {
			return &pb.SetFileSizeResponse{
				FileSize:   head.Size,
				NewVersion: head.Version,
			}, nil
		}
	}

	fileSize, err := s.FS.SetFileSize(path, req.Size, 0o644)
	if err != nil {
		return nil, toGrpcError(err)
	}

	fullData, err := s.FS.ReadFile(path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	newVersion, err := s.mutations.SaveFileSnapshot(ctx, path, head.Exists, fullData)
	if err != nil {
		return nil, toGrpcError(err)
	}

	return &pb.SetFileSizeResponse{
		FileSize:   fileSize,
		NewVersion: newVersion,
	}, nil
}

func (s *CoreService) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	path, err := normalizeProjectPath(s.FS.RootDir, req.Path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	unlock := s.pathLocker.Lock(path)
	defer unlock()
	LogRequestEvent(ctx, "component=core op=Delete path=%q", path)

	var size int64
	m, err := s.MetaStore.GetMeta(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, toGrpcError(err)
	}
	if err == nil {
		size = m.Size
	}

	if err := s.FS.Remove(path); err != nil {
		return nil, toGrpcError(err)
	}

	if _, err := s.VersionStore.AppendDeleteSnapshot(path, size, time.Now().Unix(), "", RequestIDFromContext(ctx)); err != nil {
		return nil, toGrpcError(err)
	}

	s.cleanupHooks.TriggerMutation()

	return &pb.DeleteResponse{}, nil
}

func (s *CoreService) Rename(ctx context.Context, req *pb.RenameRequest) (*pb.RenameResponse, error) {
	oldPath, err := normalizeProjectPath(s.FS.RootDir, req.OldPath)
	if err != nil {
		return nil, toGrpcError(err)
	}
	newPath, err := normalizeProjectPath(s.FS.RootDir, req.NewPath)
	if err != nil {
		return nil, toGrpcError(err)
	}
	if oldPath == newPath {
		return &pb.RenameResponse{}, nil
	}

	unlock := s.pathLocker.LockMany(oldPath, newPath)
	defer unlock()
	LogRequestEvent(ctx, "component=core op=Rename old_path=%q new_path=%q", oldPath, newPath)

	if err := s.FS.Rename(oldPath, newPath); err != nil {
		return nil, toGrpcError(err)
	}

	if err := s.VersionStore.RenamePathTree(oldPath, newPath); err != nil {
		if revertErr := s.FS.Rename(newPath, oldPath); revertErr != nil {
			log.Printf("component=core op=Rename revert_failed old_path=%q new_path=%q error=%q", oldPath, newPath, revertErr.Error())
		}
		return nil, toGrpcError(err)
	}

	s.cleanupHooks.TriggerMutation()

	return &pb.RenameResponse{}, nil
}

func (s *CoreService) Mkdir(ctx context.Context, req *pb.MkdirRequest) (*pb.MkdirResponse, error) {
	path, err := normalizeProjectPath(s.FS.RootDir, req.Path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	unlock := s.pathLocker.Lock(path)
	defer unlock()
	LogRequestEvent(ctx, "component=core op=Mkdir path=%q", path)

	if err := s.FS.Mkdir(path, 0o755); err != nil {
		return nil, toGrpcError(err)
	}
	if err := s.MetaStore.SetMeta(&fs.Meta{
		Path:      path,
		IsDir:     true,
		Size:      0,
		MtimeUnix: 0,
		Hash:      "",
		Version:   0,
	}); err != nil {
		return nil, toGrpcError(err)
	}
	s.cleanupHooks.TriggerMutation()
	return &pb.MkdirResponse{}, nil
}

func (s *CoreService) buildRangeConflictData(path string, baseVersion uint64, offset int64, data []byte) ([]byte, error) {
	baseData, err := s.mutations.loadVersionSnapshot(path, baseVersion)
	if err != nil {
		return nil, err
	}
	return applyRangeMutation(baseData, offset, data)
}

func (s *CoreService) buildSetFileSizeConflictData(path string, baseVersion uint64, size int64) ([]byte, error) {
	baseData, err := s.mutations.loadVersionSnapshot(path, baseVersion)
	if err != nil {
		return nil, err
	}
	return resizeFileData(baseData, size)
}

func applyRangeMutation(baseData []byte, offset int64, data []byte) ([]byte, error) {
	if offset < 0 {
		return nil, errors.New("offset must be >= 0")
	}

	end := offset + int64(len(data))
	if end < offset {
		return nil, errors.New("offset overflow")
	}

	size := int64(len(baseData))
	if end > size {
		size = end
	}
	if size > maxSliceLen() {
		return nil, errors.New("resulting file is too large")
	}

	out := make([]byte, int(size))
	copy(out, baseData)
	copy(out[int(offset):], data)
	return out, nil
}

func resizeFileData(baseData []byte, size int64) ([]byte, error) {
	if size < 0 {
		return nil, errors.New("size must be >= 0")
	}
	if size > maxSliceLen() {
		return nil, errors.New("resulting file is too large")
	}

	out := make([]byte, int(size))
	copy(out, baseData)
	return out, nil
}

func maxSliceLen() int64 {
	return int64(^uint(0) >> 1)
}

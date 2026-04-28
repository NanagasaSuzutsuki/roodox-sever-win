package server

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"roodox_server/internal/db"
	"roodox_server/internal/fs"
	pb "roodox_server/proto"
)

func TestCoreServiceWriteFileRangeConcurrentSamePath(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "core.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Sql.Close()

	metaStore, err := db.NewMetaStore(database)
	if err != nil {
		t.Fatalf("db.NewMetaStore returned error: %v", err)
	}
	versionStore, err := db.NewVersionStore(database)
	if err != nil {
		t.Fatalf("db.NewVersionStore returned error: %v", err)
	}

	svc := NewCoreService(fs.NewFileSystem(rootDir), metaStore, versionStore, NewPathLocker())

	const workers = 8
	errCh := make(chan error, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, callErr := svc.WriteFileRange(context.Background(), &pb.WriteFileRangeRequest{
				Path:   "range.txt",
				Offset: int64(i),
				Data:   []byte{'a' + byte(i)},
			})
			errCh <- callErr
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("WriteFileRange returned error: %v", err)
		}
	}

	data, err := svc.FS.ReadFile("range.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if len(data) < workers {
		t.Fatalf("file size = %d, want at least %d", len(data), workers)
	}
}

func TestSharedPathLockerSerializesCoreAndSyncWrites(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "shared-lock.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Sql.Close()

	metaStore, err := db.NewMetaStore(database)
	if err != nil {
		t.Fatalf("db.NewMetaStore returned error: %v", err)
	}
	versionStore, err := db.NewVersionStore(database)
	if err != nil {
		t.Fatalf("db.NewVersionStore returned error: %v", err)
	}

	locker := NewPathLocker()
	fileSystem := fs.NewFileSystem(rootDir)
	coreSvc := NewCoreService(fileSystem, metaStore, versionStore, locker)
	syncSvc := NewSyncService(fileSystem, metaStore, versionStore, locker)

	const workers = 6
	start := make(chan struct{})
	errCh := make(chan error, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start

			var callErr error
			if i%2 == 0 {
				_, callErr = syncSvc.WriteFile(context.Background(), &pb.WriteFileRequest{
					Path: "shared.txt",
					Data: []byte{byte('a' + i), byte('A' + i)},
				})
			} else {
				_, callErr = coreSvc.WriteFileRange(context.Background(), &pb.WriteFileRangeRequest{
					Path:   "shared.txt",
					Offset: int64(i - 1),
					Data:   []byte{byte('0' + i)},
				})
			}
			errCh <- callErr
		}(i)
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent write returned error: %v", err)
		}
	}

	history, err := versionStore.GetHistory("shared.txt")
	if err != nil {
		t.Fatalf("GetHistory returned error: %v", err)
	}
	if len(history) != workers {
		t.Fatalf("history length = %d, want %d", len(history), workers)
	}
}

func TestCoreServiceNormalizesAliasedPaths(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "normalized.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Sql.Close()

	metaStore, err := db.NewMetaStore(database)
	if err != nil {
		t.Fatalf("db.NewMetaStore returned error: %v", err)
	}
	versionStore, err := db.NewVersionStore(database)
	if err != nil {
		t.Fatalf("db.NewVersionStore returned error: %v", err)
	}

	svc := NewCoreService(fs.NewFileSystem(rootDir), metaStore, versionStore, NewPathLocker())

	aliasPath := filepath.Join("dir", ".", "note.txt")
	if _, err := svc.WriteFileRange(context.Background(), &pb.WriteFileRangeRequest{
		Path: aliasPath,
		Data: []byte("A"),
	}); err != nil {
		t.Fatalf("first WriteFileRange returned error: %v", err)
	}
	if _, err := svc.WriteFileRange(context.Background(), &pb.WriteFileRangeRequest{
		Path:   "dir/note.txt",
		Offset: 1,
		Data:   []byte("B"),
	}); err != nil {
		t.Fatalf("second WriteFileRange returned error: %v", err)
	}

	meta, err := metaStore.GetMeta("dir/note.txt")
	if err != nil {
		t.Fatalf("GetMeta(normalized) returned error: %v", err)
	}
	if meta.Version != 2 {
		t.Fatalf("normalized meta version = %d, want 2", meta.Version)
	}

	_, err = metaStore.GetMeta(aliasPath)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("GetMeta(alias) error = %v, want os.ErrNotExist", err)
	}

	history, err := versionStore.GetHistory("dir/note.txt")
	if err != nil {
		t.Fatalf("GetHistory returned error: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
}

func TestCoreServiceRenameMovesSubtreeMetadataAndHistory(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "rename-tree.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Sql.Close()

	metaStore, err := db.NewMetaStore(database)
	if err != nil {
		t.Fatalf("db.NewMetaStore returned error: %v", err)
	}
	versionStore, err := db.NewVersionStore(database)
	if err != nil {
		t.Fatalf("db.NewVersionStore returned error: %v", err)
	}

	locker := NewPathLocker()
	fileSystem := fs.NewFileSystem(rootDir)
	coreSvc := NewCoreService(fileSystem, metaStore, versionStore, locker)
	syncSvc := NewSyncService(fileSystem, metaStore, versionStore, locker)

	for _, path := range []string{"src", "src/nested"} {
		if _, err := coreSvc.Mkdir(context.Background(), &pb.MkdirRequest{Path: path}); err != nil {
			t.Fatalf("Mkdir(%q) returned error: %v", path, err)
		}
	}
	for path, data := range map[string]string{
		"src/app.txt":        "app",
		"src/nested/lib.txt": "lib",
	} {
		if _, err := syncSvc.WriteFile(context.Background(), &pb.WriteFileRequest{
			Path: path,
			Data: []byte(data),
		}); err != nil {
			t.Fatalf("WriteFile(%q) returned error: %v", path, err)
		}
	}

	if _, err := coreSvc.Rename(context.Background(), &pb.RenameRequest{
		OldPath: "src",
		NewPath: "dst",
	}); err != nil {
		t.Fatalf("Rename returned error: %v", err)
	}

	for _, path := range []string{"dst", "dst/nested", "dst/app.txt", "dst/nested/lib.txt"} {
		if _, err := metaStore.GetMeta(path); err != nil {
			t.Fatalf("GetMeta(%q) returned error: %v", path, err)
		}
	}
	for _, path := range []string{"src", "src/nested", "src/app.txt", "src/nested/lib.txt"} {
		if _, err := metaStore.GetMeta(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("GetMeta(%q) error = %v, want os.ErrNotExist", path, err)
		}
	}

	for _, path := range []string{"dst/app.txt", "dst/nested/lib.txt"} {
		history, err := versionStore.GetHistory(path)
		if err != nil {
			t.Fatalf("GetHistory(%q) returned error: %v", path, err)
		}
		if len(history) != 1 {
			t.Fatalf("GetHistory(%q) length = %d, want 1", path, len(history))
		}
	}
	for _, path := range []string{"src/app.txt", "src/nested/lib.txt"} {
		history, err := versionStore.GetHistory(path)
		if err != nil {
			t.Fatalf("GetHistory(%q) returned error: %v", path, err)
		}
		if len(history) != 0 {
			t.Fatalf("GetHistory(%q) length = %d, want 0", path, len(history))
		}
	}
}

func TestCoreServiceWriteFileRangeReturnsConflictForStaleBaseVersion(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "range-conflict.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Sql.Close()

	metaStore, err := db.NewMetaStore(database)
	if err != nil {
		t.Fatalf("db.NewMetaStore returned error: %v", err)
	}
	versionStore, err := db.NewVersionStore(database)
	if err != nil {
		t.Fatalf("db.NewVersionStore returned error: %v", err)
	}

	svc := NewCoreService(fs.NewFileSystem(rootDir), metaStore, versionStore, NewPathLocker())

	first, err := svc.WriteFileRange(context.Background(), &pb.WriteFileRangeRequest{
		Path: "range-conflict.txt",
		Data: []byte("hello"),
	})
	if err != nil {
		t.Fatalf("first WriteFileRange returned error: %v", err)
	}
	second, err := svc.WriteFileRange(context.Background(), &pb.WriteFileRangeRequest{
		Path:        "range-conflict.txt",
		Offset:      0,
		Data:        []byte("HELLO"),
		BaseVersion: first.NewVersion,
	})
	if err != nil {
		t.Fatalf("second WriteFileRange returned error: %v", err)
	}

	conflictResp, err := svc.WriteFileRange(context.Background(), &pb.WriteFileRangeRequest{
		Path:        "range-conflict.txt",
		Offset:      5,
		Data:        []byte("!"),
		BaseVersion: first.NewVersion,
	})
	if err != nil {
		t.Fatalf("stale WriteFileRange returned error: %v", err)
	}
	if !conflictResp.Conflicted {
		t.Fatal("expected stale range write to return conflicted=true")
	}
	if conflictResp.NewVersion != second.NewVersion {
		t.Fatalf("conflict NewVersion = %d, want %d", conflictResp.NewVersion, second.NewVersion)
	}
	if conflictResp.ConflictPath == "" {
		t.Fatal("expected conflict path to be populated")
	}
	if conflictResp.FileSize != 5 {
		t.Fatalf("conflict FileSize = %d, want 5", conflictResp.FileSize)
	}

	actual, err := svc.FS.ReadFile("range-conflict.txt")
	if err != nil {
		t.Fatalf("ReadFile(current) returned error: %v", err)
	}
	if string(actual) != "HELLO" {
		t.Fatalf("current file = %q, want %q", string(actual), "HELLO")
	}

	conflictData, err := svc.FS.ReadFile(conflictResp.ConflictPath)
	if err != nil {
		t.Fatalf("ReadFile(conflict) returned error: %v", err)
	}
	if string(conflictData) != "hello!" {
		t.Fatalf("conflict file = %q, want %q", string(conflictData), "hello!")
	}
}

func TestCoreServiceWriteFileRangeReturnsConflictAfterDeleteHead(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "range-delete-head.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Sql.Close()

	metaStore, err := db.NewMetaStore(database)
	if err != nil {
		t.Fatalf("db.NewMetaStore returned error: %v", err)
	}
	versionStore, err := db.NewVersionStore(database)
	if err != nil {
		t.Fatalf("db.NewVersionStore returned error: %v", err)
	}

	locker := NewPathLocker()
	fileSystem := fs.NewFileSystem(rootDir)
	coreSvc := NewCoreService(fileSystem, metaStore, versionStore, locker)
	syncSvc := NewSyncService(fileSystem, metaStore, versionStore, locker)

	initial, err := syncSvc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path: "deleted-range.txt",
		Data: []byte("hello"),
	})
	if err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if _, err := coreSvc.Delete(context.Background(), &pb.DeleteRequest{Path: "deleted-range.txt"}); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	conflictResp, err := coreSvc.WriteFileRange(context.Background(), &pb.WriteFileRangeRequest{
		Path:        "deleted-range.txt",
		Offset:      5,
		Data:        []byte("!"),
		BaseVersion: initial.NewVersion,
	})
	if err != nil {
		t.Fatalf("WriteFileRange returned error: %v", err)
	}
	if !conflictResp.Conflicted {
		t.Fatal("expected stale range write after delete to return conflicted=true")
	}
	if conflictResp.FileSize != 0 {
		t.Fatalf("conflict FileSize = %d, want 0", conflictResp.FileSize)
	}
	if conflictResp.NewVersion != 2 {
		t.Fatalf("conflict NewVersion = %d, want 2", conflictResp.NewVersion)
	}
	if conflictResp.ConflictPath == "" {
		t.Fatal("expected conflict path to be populated")
	}

	conflictData, err := coreSvc.FS.ReadFile(conflictResp.ConflictPath)
	if err != nil {
		t.Fatalf("ReadFile(conflict) returned error: %v", err)
	}
	if string(conflictData) != "hello!" {
		t.Fatalf("conflict file = %q, want %q", string(conflictData), "hello!")
	}
}

func TestCoreServiceSetFileSizeShrinkAndExpand(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "set-file-size.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Sql.Close()

	metaStore, err := db.NewMetaStore(database)
	if err != nil {
		t.Fatalf("db.NewMetaStore returned error: %v", err)
	}
	versionStore, err := db.NewVersionStore(database)
	if err != nil {
		t.Fatalf("db.NewVersionStore returned error: %v", err)
	}

	locker := NewPathLocker()
	fileSystem := fs.NewFileSystem(rootDir)
	coreSvc := NewCoreService(fileSystem, metaStore, versionStore, locker)
	syncSvc := NewSyncService(fileSystem, metaStore, versionStore, locker)

	initial, err := syncSvc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path: "size.txt",
		Data: []byte("abcdef"),
	})
	if err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	shrinkResp, err := coreSvc.SetFileSize(context.Background(), &pb.SetFileSizeRequest{
		Path:        "size.txt",
		Size:        3,
		BaseVersion: initial.NewVersion,
	})
	if err != nil {
		t.Fatalf("SetFileSize(shrink) returned error: %v", err)
	}
	if shrinkResp.Conflicted {
		t.Fatal("expected shrink to succeed without conflict")
	}
	if shrinkResp.FileSize != 3 {
		t.Fatalf("shrink file size = %d, want 3", shrinkResp.FileSize)
	}

	shrunk, err := coreSvc.FS.ReadFile("size.txt")
	if err != nil {
		t.Fatalf("ReadFile(shrunk) returned error: %v", err)
	}
	if string(shrunk) != "abc" {
		t.Fatalf("shrunk content = %q, want %q", string(shrunk), "abc")
	}

	expandResp, err := coreSvc.SetFileSize(context.Background(), &pb.SetFileSizeRequest{
		Path:        "size.txt",
		Size:        6,
		BaseVersion: shrinkResp.NewVersion,
	})
	if err != nil {
		t.Fatalf("SetFileSize(expand) returned error: %v", err)
	}
	if expandResp.Conflicted {
		t.Fatal("expected expand to succeed without conflict")
	}
	if expandResp.FileSize != 6 {
		t.Fatalf("expand file size = %d, want 6", expandResp.FileSize)
	}

	expanded, err := coreSvc.FS.ReadFile("size.txt")
	if err != nil {
		t.Fatalf("ReadFile(expanded) returned error: %v", err)
	}
	wantExpanded := []byte{'a', 'b', 'c', 0, 0, 0}
	if !bytes.Equal(expanded, wantExpanded) {
		t.Fatalf("expanded content = %v, want %v", expanded, wantExpanded)
	}

	history, err := versionStore.GetHistory("size.txt")
	if err != nil {
		t.Fatalf("GetHistory returned error: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("history length = %d, want 3", len(history))
	}
}

func TestCoreServiceSetFileSizeReturnsConflictForStaleBaseVersion(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "set-file-size-conflict.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Sql.Close()

	metaStore, err := db.NewMetaStore(database)
	if err != nil {
		t.Fatalf("db.NewMetaStore returned error: %v", err)
	}
	versionStore, err := db.NewVersionStore(database)
	if err != nil {
		t.Fatalf("db.NewVersionStore returned error: %v", err)
	}

	locker := NewPathLocker()
	fileSystem := fs.NewFileSystem(rootDir)
	coreSvc := NewCoreService(fileSystem, metaStore, versionStore, locker)
	syncSvc := NewSyncService(fileSystem, metaStore, versionStore, locker)

	initial, err := syncSvc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path: "conflict-size.txt",
		Data: []byte("abcdef"),
	})
	if err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	current, err := coreSvc.SetFileSize(context.Background(), &pb.SetFileSizeRequest{
		Path:        "conflict-size.txt",
		Size:        3,
		BaseVersion: initial.NewVersion,
	})
	if err != nil {
		t.Fatalf("SetFileSize(current) returned error: %v", err)
	}

	conflictResp, err := coreSvc.SetFileSize(context.Background(), &pb.SetFileSizeRequest{
		Path:        "conflict-size.txt",
		Size:        8,
		BaseVersion: initial.NewVersion,
	})
	if err != nil {
		t.Fatalf("SetFileSize(stale) returned error: %v", err)
	}
	if !conflictResp.Conflicted {
		t.Fatal("expected stale resize to return conflicted=true")
	}
	if conflictResp.NewVersion != current.NewVersion {
		t.Fatalf("conflict NewVersion = %d, want %d", conflictResp.NewVersion, current.NewVersion)
	}
	if conflictResp.ConflictPath == "" {
		t.Fatal("expected conflict path to be populated")
	}
	if conflictResp.FileSize != 3 {
		t.Fatalf("conflict FileSize = %d, want 3", conflictResp.FileSize)
	}

	actual, err := coreSvc.FS.ReadFile("conflict-size.txt")
	if err != nil {
		t.Fatalf("ReadFile(current) returned error: %v", err)
	}
	if string(actual) != "abc" {
		t.Fatalf("current file = %q, want %q", string(actual), "abc")
	}

	conflictData, err := coreSvc.FS.ReadFile(conflictResp.ConflictPath)
	if err != nil {
		t.Fatalf("ReadFile(conflict) returned error: %v", err)
	}
	wantConflict := []byte{'a', 'b', 'c', 'd', 'e', 'f', 0, 0}
	if !bytes.Equal(conflictData, wantConflict) {
		t.Fatalf("conflict file = %v, want %v", conflictData, wantConflict)
	}
}

func TestCoreServiceSetFileSizeReturnsConflictAfterDeleteHead(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "set-file-size-delete-head.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Sql.Close()

	metaStore, err := db.NewMetaStore(database)
	if err != nil {
		t.Fatalf("db.NewMetaStore returned error: %v", err)
	}
	versionStore, err := db.NewVersionStore(database)
	if err != nil {
		t.Fatalf("db.NewVersionStore returned error: %v", err)
	}

	locker := NewPathLocker()
	fileSystem := fs.NewFileSystem(rootDir)
	coreSvc := NewCoreService(fileSystem, metaStore, versionStore, locker)
	syncSvc := NewSyncService(fileSystem, metaStore, versionStore, locker)

	initial, err := syncSvc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path: "deleted-size.txt",
		Data: []byte("abcdef"),
	})
	if err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if _, err := coreSvc.Delete(context.Background(), &pb.DeleteRequest{Path: "deleted-size.txt"}); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	conflictResp, err := coreSvc.SetFileSize(context.Background(), &pb.SetFileSizeRequest{
		Path:        "deleted-size.txt",
		Size:        8,
		BaseVersion: initial.NewVersion,
	})
	if err != nil {
		t.Fatalf("SetFileSize returned error: %v", err)
	}
	if !conflictResp.Conflicted {
		t.Fatal("expected stale resize after delete to return conflicted=true")
	}
	if conflictResp.FileSize != 0 {
		t.Fatalf("conflict FileSize = %d, want 0", conflictResp.FileSize)
	}
	if conflictResp.NewVersion != 2 {
		t.Fatalf("conflict NewVersion = %d, want 2", conflictResp.NewVersion)
	}
	if conflictResp.ConflictPath == "" {
		t.Fatal("expected conflict path to be populated")
	}

	conflictData, err := coreSvc.FS.ReadFile(conflictResp.ConflictPath)
	if err != nil {
		t.Fatalf("ReadFile(conflict) returned error: %v", err)
	}
	wantConflict := []byte{'a', 'b', 'c', 'd', 'e', 'f', 0, 0}
	if !bytes.Equal(conflictData, wantConflict) {
		t.Fatalf("conflict file = %v, want %v", conflictData, wantConflict)
	}
}

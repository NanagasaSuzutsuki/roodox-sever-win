package server

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"roodox_server/internal/db"
	"roodox_server/internal/fs"
	pb "roodox_server/proto"
)

func TestSyncServiceWriteFileSkipsDuplicateSnapshot(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "sync.db"))
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

	svc := NewSyncService(fs.NewFileSystem(rootDir), metaStore, versionStore, NewPathLocker())

	first, err := svc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path: "same.txt",
		Data: []byte("hello"),
	})
	if err != nil {
		t.Fatalf("first WriteFile returned error: %v", err)
	}
	if first.NewVersion != 1 {
		t.Fatalf("first version = %d, want 1", first.NewVersion)
	}

	second, err := svc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path: "same.txt",
		Data: []byte("hello"),
	})
	if err != nil {
		t.Fatalf("second WriteFile returned error: %v", err)
	}
	if second.NewVersion != 1 {
		t.Fatalf("second version = %d, want 1", second.NewVersion)
	}

	history, err := versionStore.GetHistory("same.txt")
	if err != nil {
		t.Fatalf("GetHistory returned error: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}
}

func TestSyncServiceWriteFileReturnsConflictForStaleBaseVersion(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "sync-conflict.db"))
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

	svc := NewSyncService(fs.NewFileSystem(rootDir), metaStore, versionStore, NewPathLocker())

	first, err := svc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path: "conflict.txt",
		Data: []byte("v1"),
	})
	if err != nil {
		t.Fatalf("first WriteFile returned error: %v", err)
	}
	second, err := svc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:        "conflict.txt",
		Data:        []byte("v2"),
		BaseVersion: first.NewVersion,
	})
	if err != nil {
		t.Fatalf("second WriteFile returned error: %v", err)
	}

	conflictResp, err := svc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:        "conflict.txt",
		Data:        []byte("stale"),
		BaseVersion: first.NewVersion,
	})
	if err != nil {
		t.Fatalf("stale WriteFile returned error: %v", err)
	}
	if !conflictResp.Conflicted {
		t.Fatal("expected stale write to return conflicted=true")
	}
	if conflictResp.NewVersion != second.NewVersion {
		t.Fatalf("conflict NewVersion = %d, want %d", conflictResp.NewVersion, second.NewVersion)
	}
	if conflictResp.ConflictPath == "" {
		t.Fatal("expected conflict path to be populated")
	}
	if _, statErr := svc.FS.Stat(conflictResp.ConflictPath); statErr != nil {
		t.Fatalf("expected conflict file to exist: %v", statErr)
	}
}

func TestSyncServiceWriteFileReturnsConflictAfterDeleteHead(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "sync-delete-head.db"))
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
	syncSvc := NewSyncService(fileSystem, metaStore, versionStore, locker)
	coreSvc := NewCoreService(fileSystem, metaStore, versionStore, locker)

	first, err := syncSvc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path: "deleted-sync.txt",
		Data: []byte("v1"),
	})
	if err != nil {
		t.Fatalf("first WriteFile returned error: %v", err)
	}
	if _, err := coreSvc.Delete(context.Background(), &pb.DeleteRequest{Path: "deleted-sync.txt"}); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	conflictResp, err := syncSvc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:        "deleted-sync.txt",
		Data:        []byte("stale"),
		BaseVersion: first.NewVersion,
	})
	if err != nil {
		t.Fatalf("stale WriteFile returned error: %v", err)
	}
	if !conflictResp.Conflicted {
		t.Fatal("expected stale write after delete to return conflicted=true")
	}
	if conflictResp.NewVersion != 2 {
		t.Fatalf("conflict NewVersion = %d, want 2", conflictResp.NewVersion)
	}
	if conflictResp.ConflictPath == "" {
		t.Fatal("expected conflict path to be populated")
	}
}

func TestSyncServiceConflictFilesUseUniqueNames(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "sync-unique-conflict.db"))
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

	svc := NewSyncService(fs.NewFileSystem(rootDir), metaStore, versionStore, NewPathLocker())

	first, err := svc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path: "history.txt",
		Data: []byte("v1"),
	})
	if err != nil {
		t.Fatalf("first WriteFile returned error: %v", err)
	}
	if _, err := svc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:        "history.txt",
		Data:        []byte("v2"),
		BaseVersion: first.NewVersion,
	}); err != nil {
		t.Fatalf("second WriteFile returned error: %v", err)
	}

	conflictOne, err := svc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:        "history.txt",
		Data:        []byte("stale-one"),
		BaseVersion: first.NewVersion,
	})
	if err != nil {
		t.Fatalf("conflict one returned error: %v", err)
	}
	conflictTwo, err := svc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:        "history.txt",
		Data:        []byte("stale-two"),
		BaseVersion: first.NewVersion,
	})
	if err != nil {
		t.Fatalf("conflict two returned error: %v", err)
	}

	if !conflictOne.Conflicted || !conflictTwo.Conflicted {
		t.Fatal("expected both stale writes to produce conflict files")
	}
	if conflictOne.ConflictPath == conflictTwo.ConflictPath {
		t.Fatalf("conflict paths should be unique, both were %q", conflictOne.ConflictPath)
	}

	pattern := regexp.MustCompile(`^history\.txt\.roodox-conflict-\d{8}-\d{6}\.\d{9}-[0-9a-f]{6}$`)
	for _, conflictPath := range []string{conflictOne.ConflictPath, conflictTwo.ConflictPath} {
		base := filepath.Base(conflictPath)
		if !pattern.MatchString(base) {
			t.Fatalf("conflict path %q does not match expected format", conflictPath)
		}
	}

	dataOne, err := svc.FS.ReadFile(conflictOne.ConflictPath)
	if err != nil {
		t.Fatalf("ReadFile(conflict one) returned error: %v", err)
	}
	dataTwo, err := svc.FS.ReadFile(conflictTwo.ConflictPath)
	if err != nil {
		t.Fatalf("ReadFile(conflict two) returned error: %v", err)
	}
	if string(dataOne) != "stale-one" {
		t.Fatalf("conflict one content = %q, want %q", string(dataOne), "stale-one")
	}
	if string(dataTwo) != "stale-two" {
		t.Fatalf("conflict two content = %q, want %q", string(dataTwo), "stale-two")
	}
}

func TestSyncServiceListChangedFilesPropagatesMetaStoreErrors(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "sync-meta-error.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}

	metaStore, err := db.NewMetaStore(database)
	if err != nil {
		t.Fatalf("db.NewMetaStore returned error: %v", err)
	}
	versionStore, err := db.NewVersionStore(database)
	if err != nil {
		t.Fatalf("db.NewVersionStore returned error: %v", err)
	}

	svc := NewSyncService(fs.NewFileSystem(rootDir), metaStore, versionStore, NewPathLocker())

	if err := database.Sql.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	_, err = svc.ListChangedFiles(context.Background(), &pb.ListChangedFilesRequest{})
	if status.Code(err) != codes.Internal {
		t.Fatalf("ListChangedFiles error code = %v, want %v", status.Code(err), codes.Internal)
	}
}

func TestSyncServiceListChangedFilesRejectsSinceVersionCursor(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "sync-since-version.db"))
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

	svc := NewSyncService(fs.NewFileSystem(rootDir), metaStore, versionStore, NewPathLocker())

	_, err = svc.ListChangedFiles(context.Background(), &pb.ListChangedFilesRequest{SinceVersion: 1})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("ListChangedFiles error code = %v, want %v", status.Code(err), codes.InvalidArgument)
	}
}

func TestSyncServiceConflictFilesLifecycleKeepsLatestCopies(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "sync-conflict-lifecycle.db"))
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

	svc := NewSyncService(fs.NewFileSystem(rootDir), metaStore, versionStore, NewPathLocker())
	svc.conflictMaxCopiesPerPath = 2
	svc.conflictFileTTL = 365 * 24 * time.Hour
	svc.conflictCleanupInterval = time.Hour

	first, err := svc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path: "history.txt",
		Data: []byte("v1"),
	})
	if err != nil {
		t.Fatalf("first WriteFile returned error: %v", err)
	}
	if _, err := svc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:        "history.txt",
		Data:        []byte("v2"),
		BaseVersion: first.NewVersion,
	}); err != nil {
		t.Fatalf("second WriteFile returned error: %v", err)
	}

	if _, err := svc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:        "history.txt",
		Data:        []byte("stale-one"),
		BaseVersion: first.NewVersion,
	}); err != nil {
		t.Fatalf("third WriteFile returned error: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := svc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:        "history.txt",
		Data:        []byte("stale-two"),
		BaseVersion: first.NewVersion,
	}); err != nil {
		t.Fatalf("fourth WriteFile returned error: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := svc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:        "history.txt",
		Data:        []byte("stale-three"),
		BaseVersion: first.NewVersion,
	}); err != nil {
		t.Fatalf("fifth WriteFile returned error: %v", err)
	}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}

	conflicts := make([]string, 0, len(entries))
	for _, entry := range entries {
		if fs.IsConflictPath(entry.Name()) {
			conflicts = append(conflicts, entry.Name())
		}
	}
	if len(conflicts) != 2 {
		t.Fatalf("conflict file count = %d, want 2 (%v)", len(conflicts), conflicts)
	}

	sort.Strings(conflicts)
	contents := make([]string, 0, len(conflicts))
	for _, name := range conflicts {
		data, err := os.ReadFile(filepath.Join(rootDir, name))
		if err != nil {
			t.Fatalf("ReadFile(%q) returned error: %v", name, err)
		}
		contents = append(contents, string(data))
	}
	sort.Strings(contents)
	if len(contents) != 2 || contents[0] != "stale-three" || contents[1] != "stale-two" {
		t.Fatalf("conflict file contents = %v, want [stale-three stale-two]", contents)
	}
}

func TestSyncServiceCleanupExpiredConflictFiles(t *testing.T) {
	rootDir := t.TempDir()
	svc := NewSyncService(fs.NewFileSystem(rootDir), nil, nil, NewPathLocker())
	svc.conflictFileTTL = time.Hour

	oldConflict := "demo.txt.roodox-conflict-20260322-120102.123456789-ab12cd"
	newConflict := "demo.txt.roodox-conflict-20260322-130102.123456789-bc23de"
	normalFile := "demo.txt"

	for _, rel := range []string{oldConflict, newConflict, normalFile} {
		if err := svc.FS.WriteFile(rel, []byte(rel), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) returned error: %v", rel, err)
		}
	}

	oldFull, err := fs.ResolvePath(rootDir, oldConflict)
	if err != nil {
		t.Fatalf("ResolvePath(oldConflict) returned error: %v", err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldFull, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes(oldConflict) returned error: %v", err)
	}

	if err := svc.cleanupExpiredConflictFiles(time.Now()); err != nil {
		t.Fatalf("cleanupExpiredConflictFiles returned error: %v", err)
	}

	if _, err := svc.FS.Stat(oldConflict); !os.IsNotExist(err) {
		t.Fatalf("expected old conflict copy to be removed, stat err=%v", err)
	}
	if _, err := svc.FS.Stat(newConflict); err != nil {
		t.Fatalf("expected new conflict copy to remain: %v", err)
	}
	if _, err := svc.FS.Stat(normalFile); err != nil {
		t.Fatalf("expected normal file to remain: %v", err)
	}
}

func TestSyncServiceMetaResponsesIncludeSize(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "sync-meta-size.db"))
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

	svc := NewSyncService(fs.NewFileSystem(rootDir), metaStore, versionStore, NewPathLocker())

	if _, err := svc.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path: "meta-size.txt",
		Data: []byte("hello"),
	}); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	metaResp, err := svc.GetFileMeta(context.Background(), &pb.GetFileMetaRequest{Path: "meta-size.txt"})
	if err != nil {
		t.Fatalf("GetFileMeta returned error: %v", err)
	}
	if metaResp.Meta == nil {
		t.Fatal("expected meta response to be populated")
	}
	if metaResp.Meta.Size != 5 {
		t.Fatalf("GetFileMeta size = %d, want 5", metaResp.Meta.Size)
	}

	changedResp, err := svc.ListChangedFiles(context.Background(), &pb.ListChangedFilesRequest{})
	if err != nil {
		t.Fatalf("ListChangedFiles returned error: %v", err)
	}
	if len(changedResp.Metas) != 1 {
		t.Fatalf("ListChangedFiles count = %d, want 1", len(changedResp.Metas))
	}
	if changedResp.Metas[0].Size != 5 {
		t.Fatalf("ListChangedFiles size = %d, want 5", changedResp.Metas[0].Size)
	}
}

func TestSyncServiceGetFileMetaFallsBackToFilesystemSize(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "sync-meta-fallback.db"))
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

	svc := NewSyncService(fs.NewFileSystem(rootDir), metaStore, versionStore, NewPathLocker())

	if err := svc.FS.WriteFile("orphan.txt", []byte("hello"), 0o644); err != nil {
		t.Fatalf("FS.WriteFile returned error: %v", err)
	}

	metaResp, err := svc.GetFileMeta(context.Background(), &pb.GetFileMetaRequest{Path: "orphan.txt"})
	if err != nil {
		t.Fatalf("GetFileMeta returned error: %v", err)
	}
	if metaResp.Meta == nil {
		t.Fatal("expected meta response to be populated")
	}
	if metaResp.Meta.Size != 5 {
		t.Fatalf("GetFileMeta size = %d, want 5", metaResp.Meta.Size)
	}
	if metaResp.Meta.MtimeUnix == 0 {
		t.Fatal("expected GetFileMeta mtime to be populated from filesystem fallback")
	}
}

func TestBuildServiceQueuesWhenWorkersBusy(t *testing.T) {
	rootDir := t.TempDir()
	svc := NewBuildService(BuildConfig{
		RootDir:       rootDir,
		RemoteEnabled: true,
	})
	defer svc.Close()
	svc.slots = make(chan struct{}, 1)
	svc.slots <- struct{}{}

	resp, err := svc.StartBuild(context.Background(), &pb.StartBuildRequest{
		UnitPath: ".",
	})
	if err != nil {
		t.Fatalf("StartBuild returned error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	statusResp, err := svc.GetBuildStatus(context.Background(), &pb.GetBuildStatusRequest{
		BuildId: resp.BuildId,
	})
	if err != nil {
		t.Fatalf("GetBuildStatus returned error: %v", err)
	}
	if statusResp.Status != "queued" {
		t.Fatalf("status = %q, want queued", statusResp.Status)
	}

	<-svc.slots
}

func TestBuildServiceCleanupRemovesExpiredJobs(t *testing.T) {
	rootDir := t.TempDir()
	buildTempRoot := filepath.Join(t.TempDir(), "roodox-builds")
	svc := NewBuildService(BuildConfig{
		RootDir:         rootDir,
		RemoteEnabled:   true,
		JobTTL:          20 * time.Millisecond,
		CleanupInterval: time.Hour,
		MaxRetainedJobs: 200,
		TempRoot:        buildTempRoot,
	})
	defer svc.Close()

	svc.runBuildFn = func(unitAbs, buildRoot, target string, appendLog func(string, ...any)) (string, error) {
		if err := os.MkdirAll(buildRoot, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(buildRoot, "marker.txt"), []byte("ok"), 0o644); err != nil {
			return "", err
		}
		return "", nil
	}

	resp, err := svc.StartBuild(context.Background(), &pb.StartBuildRequest{
		UnitPath: ".",
	})
	if err != nil {
		t.Fatalf("StartBuild returned error: %v", err)
	}

	job, err := svc.getJob(resp.BuildId)
	if err != nil {
		t.Fatalf("getJob returned error: %v", err)
	}
	<-job.done

	buildDir := filepath.Join(buildTempRoot, resp.BuildId)
	if _, err := os.Stat(buildDir); err != nil {
		t.Fatalf("expected build dir %q to exist before cleanup: %v", buildDir, err)
	}

	svc.cleanupJobs(time.Now().Add(2 * svc.jobTTL))

	if _, err := svc.getJob(resp.BuildId); status.Code(err) != codes.NotFound {
		t.Fatalf("getJob after cleanup code = %v, want %v", status.Code(err), codes.NotFound)
	}
	if _, err := os.Stat(buildDir); !os.IsNotExist(err) {
		t.Fatalf("expected build dir %q to be removed, stat err=%v", buildDir, err)
	}
}

func TestBuildServiceQueueWaitUsesQueuedAtForSerializedUnitPath(t *testing.T) {
	rootDir := t.TempDir()
	svc := NewBuildService(BuildConfig{
		RootDir:         rootDir,
		RemoteEnabled:   true,
		MaxWorkers:      2,
		CleanupInterval: time.Hour,
		JobTTL:          time.Hour,
	})
	defer svc.Close()

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})

	var mu sync.Mutex
	callCount := 0
	svc.runBuildFn = func(unitAbs, buildRoot, target string, appendLog func(string, ...any)) (string, error) {
		mu.Lock()
		callCount++
		n := callCount
		mu.Unlock()

		if n == 1 {
			close(firstStarted)
			<-releaseFirst
		}
		return "", nil
	}

	first, err := svc.StartBuild(context.Background(), &pb.StartBuildRequest{
		UnitPath: "same-unit",
	})
	if err != nil {
		t.Fatalf("first StartBuild returned error: %v", err)
	}

	firstJob, err := svc.getJob(first.BuildId)
	if err != nil {
		t.Fatalf("getJob(first) returned error: %v", err)
	}
	<-firstStarted

	second, err := svc.StartBuild(context.Background(), &pb.StartBuildRequest{
		UnitPath: "same-unit",
	})
	if err != nil {
		t.Fatalf("second StartBuild returned error: %v", err)
	}

	secondJob, err := svc.getJob(second.BuildId)
	if err != nil {
		t.Fatalf("getJob(second) returned error: %v", err)
	}

	time.Sleep(200 * time.Millisecond)
	close(releaseFirst)

	<-firstJob.done
	<-secondJob.done

	secondJob.mu.Lock()
	queuedAt := secondJob.queuedAt
	startedAt := secondJob.startedAt
	queueWait := startedAt.Sub(queuedAt)
	secondJob.mu.Unlock()

	if queuedAt.IsZero() {
		t.Fatal("queuedAt should be populated")
	}
	if startedAt.IsZero() {
		t.Fatal("startedAt should be populated")
	}
	if queueWait < 200*time.Millisecond {
		t.Fatalf("queueWait = %v, want at least %v", queueWait, 200*time.Millisecond)
	}
}

package db

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"roodox_server/internal/fs"
)

func TestVersionStoreSnapshots(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "version.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer database.Sql.Close()

	store, err := NewVersionStore(database)
	if err != nil {
		t.Fatalf("NewVersionStore returned error: %v", err)
	}

	if err := store.AppendSnapshot("demo.txt", fs.VersionRecord{
		Version:    1,
		MtimeUnix:  100,
		Hash:       "h1",
		Size:       2,
		ClientID:   "c1",
		ChangeType: "create",
	}, []byte("v1")); err != nil {
		t.Fatalf("AppendSnapshot(create) returned error: %v", err)
	}

	if err := store.AppendSnapshot("demo.txt", fs.VersionRecord{
		Version:    2,
		MtimeUnix:  200,
		Hash:       "",
		Size:       0,
		ClientID:   "c1",
		ChangeType: "delete",
	}, nil); err != nil {
		t.Fatalf("AppendSnapshot(delete) returned error: %v", err)
	}

	history, err := store.GetHistory("demo.txt")
	if err != nil {
		t.Fatalf("GetHistory returned error: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("GetHistory length = %d, want 2", len(history))
	}

	data, err := store.GetVersionData("demo.txt", 1)
	if err != nil {
		t.Fatalf("GetVersionData(v1) returned error: %v", err)
	}
	if string(data) != "v1" {
		t.Fatalf("GetVersionData(v1) = %q, want %q", string(data), "v1")
	}

	rec, err := store.GetRecord("demo.txt", 2)
	if err != nil {
		t.Fatalf("GetRecord(v2) returned error: %v", err)
	}
	if rec.ChangeType != "delete" {
		t.Fatalf("GetRecord(v2).ChangeType = %q, want delete", rec.ChangeType)
	}

	_, err = store.GetVersionData("demo.txt", 2)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("GetVersionData(v2) error = %v, want os.ErrNotExist", err)
	}
}

func TestVersionStoreSaveFileSnapshotContinuesExistingVersion(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "version-next.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer database.Sql.Close()

	metaStore, err := NewMetaStore(database)
	if err != nil {
		t.Fatalf("NewMetaStore returned error: %v", err)
	}
	store, err := NewVersionStore(database)
	if err != nil {
		t.Fatalf("NewVersionStore returned error: %v", err)
	}

	if err := metaStore.SetMeta(&fs.Meta{
		Path:      "demo.txt",
		IsDir:     false,
		Size:      2,
		MtimeUnix: 100,
		Hash:      "h2",
		Version:   2,
	}); err != nil {
		t.Fatalf("SetMeta returned error: %v", err)
	}
	if err := store.AppendSnapshot("demo.txt", fs.VersionRecord{
		Version:    2,
		MtimeUnix:  100,
		Hash:       "h2",
		Size:       2,
		ClientID:   "c1",
		ChangeType: "modify",
	}, []byte("v2")); err != nil {
		t.Fatalf("AppendSnapshot returned error: %v", err)
	}

	meta := &fs.Meta{
		Path:      "demo.txt",
		IsDir:     false,
		Size:      2,
		MtimeUnix: 200,
		Hash:      "h3",
	}
	version, err := store.SaveFileSnapshot(meta, "c1", "", "modify", []byte("v3"))
	if err != nil {
		t.Fatalf("SaveFileSnapshot returned error: %v", err)
	}
	if version != 3 {
		t.Fatalf("SaveFileSnapshot version = %d, want 3", version)
	}

	history, err := store.GetHistory("demo.txt")
	if err != nil {
		t.Fatalf("GetHistory returned error: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("GetHistory length = %d, want 2", len(history))
	}
	if history[1].Version != 3 {
		t.Fatalf("history[1].Version = %d, want 3", history[1].Version)
	}
}

func TestVersionStoreGetLatestRecordReturnsDeleteHead(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "version-latest.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer database.Sql.Close()

	store, err := NewVersionStore(database)
	if err != nil {
		t.Fatalf("NewVersionStore returned error: %v", err)
	}

	if err := store.AppendSnapshot("demo.txt", fs.VersionRecord{
		Version:    1,
		MtimeUnix:  100,
		Hash:       "h1",
		Size:       2,
		ClientID:   "c1",
		ChangeType: "create",
	}, []byte("v1")); err != nil {
		t.Fatalf("AppendSnapshot(create) returned error: %v", err)
	}
	if err := store.AppendSnapshot("demo.txt", fs.VersionRecord{
		Version:    2,
		MtimeUnix:  200,
		Hash:       "",
		Size:       0,
		ClientID:   "c1",
		ChangeType: "delete",
	}, nil); err != nil {
		t.Fatalf("AppendSnapshot(delete) returned error: %v", err)
	}

	latest, err := store.GetLatestRecord("demo.txt")
	if err != nil {
		t.Fatalf("GetLatestRecord returned error: %v", err)
	}
	if latest.Version != 2 {
		t.Fatalf("latest.Version = %d, want 2", latest.Version)
	}
	if latest.ChangeType != "delete" {
		t.Fatalf("latest.ChangeType = %q, want %q", latest.ChangeType, "delete")
	}
	if latest.Size != 0 {
		t.Fatalf("latest.Size = %d, want 0", latest.Size)
	}
}

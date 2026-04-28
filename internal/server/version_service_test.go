package server

import (
	"context"
	"path/filepath"
	"testing"

	"roodox_server/internal/db"
	"roodox_server/internal/fs"
	pb "roodox_server/proto"
)

func TestVersionServiceGetVersionReturnsSnapshot(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Sql.Close()

	store, err := db.NewVersionStore(database)
	if err != nil {
		t.Fatalf("db.NewVersionStore returned error: %v", err)
	}

	if err := store.AppendSnapshot("demo.txt", fs.VersionRecord{
		Version:    1,
		MtimeUnix:  100,
		Hash:       "h1",
		Size:       2,
		ClientID:   "client",
		ChangeType: "create",
	}, []byte("v1")); err != nil {
		t.Fatalf("AppendSnapshot returned error: %v", err)
	}

	svc := NewVersionService(store)
	resp, err := svc.GetVersion(context.Background(), &pb.GetVersionRequest{Path: "demo.txt", Version: 1})
	if err != nil {
		t.Fatalf("GetVersion returned error: %v", err)
	}
	if string(resp.Data) != "v1" {
		t.Fatalf("GetVersion data = %q, want %q", string(resp.Data), "v1")
	}
}

func TestVersionServiceDeleteVersionReturnsEmptyData(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "service-delete.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Sql.Close()

	store, err := db.NewVersionStore(database)
	if err != nil {
		t.Fatalf("db.NewVersionStore returned error: %v", err)
	}

	if err := store.AppendSnapshot("demo.txt", fs.VersionRecord{
		Version:    2,
		MtimeUnix:  200,
		Hash:       "",
		Size:       0,
		ClientID:   "client",
		ChangeType: "delete",
	}, nil); err != nil {
		t.Fatalf("AppendSnapshot returned error: %v", err)
	}

	svc := NewVersionService(store)
	resp, err := svc.GetVersion(context.Background(), &pb.GetVersionRequest{Path: "demo.txt", Version: 2})
	if err != nil {
		t.Fatalf("GetVersion returned error: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Fatalf("GetVersion delete data length = %d, want 0", len(resp.Data))
	}
}

func TestVersionServiceNormalizesAliasedPaths(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "service-normalized.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Sql.Close()

	store, err := db.NewVersionStore(database)
	if err != nil {
		t.Fatalf("db.NewVersionStore returned error: %v", err)
	}

	if err := store.AppendSnapshot("demo.txt", fs.VersionRecord{
		Version:    1,
		MtimeUnix:  100,
		Hash:       "h1",
		Size:       2,
		ClientID:   "client",
		ChangeType: "create",
	}, []byte("v1")); err != nil {
		t.Fatalf("AppendSnapshot returned error: %v", err)
	}

	svc := NewVersionService(store)
	resp, err := svc.GetVersion(context.Background(), &pb.GetVersionRequest{
		Path:    filepath.Join(".", "demo.txt"),
		Version: 1,
	})
	if err != nil {
		t.Fatalf("GetVersion returned error: %v", err)
	}
	if string(resp.Data) != "v1" {
		t.Fatalf("GetVersion data = %q, want %q", string(resp.Data), "v1")
	}
}

func TestBuildServiceResolveUnitPathBlocksEscape(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	svc := NewBuildService(BuildConfig{RootDir: root})

	_, err := svc.resolveUnitPath(filepath.Join("..", "root-evil"))
	if err == nil {
		t.Fatal("resolveUnitPath unexpectedly allowed escaping path")
	}
}

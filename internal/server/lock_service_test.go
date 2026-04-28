package server

import (
	"context"
	"testing"

	"roodox_server/internal/lock"
	pb "roodox_server/proto"
)

func TestLockServiceNormalizesAliasedPaths(t *testing.T) {
	svc := NewLockService(lock.NewManager(0))

	first, err := svc.AcquireLock(context.Background(), &pb.AcquireLockRequest{
		Path:       "dir/./note.txt",
		ClientId:   "client-a",
		TtlSeconds: 30,
	})
	if err != nil {
		t.Fatalf("AcquireLock(first) returned error: %v", err)
	}
	if !first.Ok {
		t.Fatal("expected first lock acquisition to succeed")
	}

	second, err := svc.AcquireLock(context.Background(), &pb.AcquireLockRequest{
		Path:       "dir/note.txt",
		ClientId:   "client-b",
		TtlSeconds: 30,
	})
	if err != nil {
		t.Fatalf("AcquireLock(second) returned error: %v", err)
	}
	if second.Ok {
		t.Fatal("expected aliased path lock acquisition to fail")
	}
	if second.Owner != "client-a" {
		t.Fatalf("owner = %q, want %q", second.Owner, "client-a")
	}
}

// internal/server/lock_service.go
package server

import (
	"context"
	"time"

	"roodox_server/internal/fs"
	"roodox_server/internal/lock"
	pb "roodox_server/proto"
)

type LockService struct {
	pb.UnimplementedLockServiceServer

	Manager *lock.Manager
}

func NewLockService(m *lock.Manager) *LockService {
	return &LockService{Manager: m}
}

func (s *LockService) AcquireLock(ctx context.Context, req *pb.AcquireLockRequest) (*pb.AcquireLockResponse, error) {
	path, err := fs.NormalizeRelativePath(req.Path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	ttl := time.Duration(req.TtlSeconds) * time.Second
	ok, owner, exp := s.Manager.Acquire(path, req.ClientId, ttl)

	return &pb.AcquireLockResponse{
		Ok:       ok,
		Owner:    owner,
		ExpireAt: exp.Unix(),
	}, nil
}

func (s *LockService) RenewLock(ctx context.Context, req *pb.RenewLockRequest) (*pb.RenewLockResponse, error) {
	path, err := fs.NormalizeRelativePath(req.Path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	ttl := time.Duration(req.TtlSeconds) * time.Second
	ok, exp := s.Manager.Renew(path, req.ClientId, ttl)

	return &pb.RenewLockResponse{
		Ok:       ok,
		ExpireAt: exp.Unix(),
	}, nil
}

func (s *LockService) ReleaseLock(ctx context.Context, req *pb.ReleaseLockRequest) (*pb.ReleaseLockResponse, error) {
	path, err := fs.NormalizeRelativePath(req.Path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	ok := s.Manager.Release(path, req.ClientId)
	return &pb.ReleaseLockResponse{Ok: ok}, nil
}

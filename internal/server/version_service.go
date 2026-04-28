package server

import (
	"context"

	"roodox_server/internal/db"
	"roodox_server/internal/fs"
	pb "roodox_server/proto"
)

type VersionService struct {
	pb.UnimplementedVersionServiceServer

	VersionStore *db.VersionStore
}

func NewVersionService(ver *db.VersionStore) *VersionService {
	return &VersionService{VersionStore: ver}
}

func (s *VersionService) GetHistory(ctx context.Context, req *pb.GetHistoryRequest) (*pb.GetHistoryResponse, error) {
	path, err := fs.NormalizeRelativePath(req.Path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	hist, err := s.VersionStore.GetHistory(path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	resp := &pb.GetHistoryResponse{}
	for _, rec := range hist {
		resp.Records = append(resp.Records, &pb.VersionRecord{
			Version:    rec.Version,
			MtimeUnix:  rec.MtimeUnix,
			Hash:       rec.Hash,
			Size:       rec.Size,
			ClientId:   rec.ClientID,
			ChangeType: rec.ChangeType,
		})
	}
	return resp, nil
}

func (s *VersionService) GetVersion(ctx context.Context, req *pb.GetVersionRequest) (*pb.GetVersionResponse, error) {
	path, err := fs.NormalizeRelativePath(req.Path)
	if err != nil {
		return nil, toGrpcError(err)
	}

	rec, err := s.VersionStore.GetRecord(path, req.Version)
	if err != nil {
		return nil, toGrpcError(err)
	}
	if rec.ChangeType == "delete" {
		return &pb.GetVersionResponse{Data: []byte{}}, nil
	}

	data, err := s.VersionStore.GetVersionData(path, req.Version)
	if err != nil {
		return nil, toGrpcError(err)
	}
	return &pb.GetVersionResponse{Data: data}, nil
}

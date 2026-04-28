package server

import (
	"context"

	"roodox_server/internal/analyze"
	pb "roodox_server/proto"
)

type AnalyzeService struct {
	pb.UnimplementedAnalyzeServiceServer
	analyzer *analyze.Analyzer
}

func NewAnalyzeService(a *analyze.Analyzer) *AnalyzeService {
	return &AnalyzeService{analyzer: a}
}

func (s *AnalyzeService) AnalyzeBuildUnits(ctx context.Context, req *pb.AnalyzeBuildUnitsRequest) (*pb.AnalyzeBuildUnitsResponse, error) {
	units, err := s.analyzer.Scan(req.Root)
	if err != nil {
		return nil, toGrpcError(err)
	}

	resp := &pb.AnalyzeBuildUnitsResponse{}
	for _, u := range units {
		resp.Units = append(resp.Units, &pb.BuildUnit{
			Path: u.Path,
			Type: u.Type,
		})
	}
	return resp, nil
}

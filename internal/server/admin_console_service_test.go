package server

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"roodox_server/internal/observability"
	pb "roodox_server/proto"
)

type stubRuntimeProvider struct {
	runtime       ServerRuntimeSnapshot
	observability ServerObservabilitySnapshot
	backup        DatabaseBackupSnapshot
	shutdown      ServerShutdownSnapshot
	err           error
}

func (s stubRuntimeProvider) GetServerRuntimeSnapshot(context.Context) (ServerRuntimeSnapshot, error) {
	return s.runtime, s.err
}

func (s stubRuntimeProvider) GetServerObservabilitySnapshot(context.Context) (ServerObservabilitySnapshot, error) {
	return s.observability, s.err
}

func (s stubRuntimeProvider) TriggerServerBackup(context.Context) (DatabaseBackupSnapshot, error) {
	return s.backup, s.err
}

func (s stubRuntimeProvider) RequestServerShutdown(context.Context, string) (ServerShutdownSnapshot, error) {
	return s.shutdown, s.err
}

func TestAdminConsoleServiceServerRuntimeEndpoints(t *testing.T) {
	svc := NewAdminConsoleService(nil, ControlPlaneConfig{})
	svc.ConfigureRuntime(stubRuntimeProvider{
		runtime: ServerRuntimeSnapshot{
			ServerID:      "srv-main",
			ListenAddr:    ":50051",
			RootDir:       "E:/share",
			DBPath:        "E:/data/roodox.db",
			TLSEnabled:    true,
			AuthEnabled:   true,
			StartedAtUnix: 123,
			HealthState:   "serving",
			HealthMessage: "ok",
			DBFile: FileStatSnapshot{
				Path:           "E:/data/roodox.db",
				Exists:         true,
				SizeBytes:      1024,
				ModifiedAtUnix: 123,
			},
			WALFile: FileStatSnapshot{
				Path:           "E:/data/roodox.db-wal",
				Exists:         true,
				SizeBytes:      256,
				ModifiedAtUnix: 123,
			},
			SHMFile: FileStatSnapshot{
				Path:           "E:/data/roodox.db-shm",
				Exists:         true,
				SizeBytes:      64,
				ModifiedAtUnix: 123,
			},
			Checkpoint: DatabaseCheckpointSnapshot{
				LastCheckpointAtUnix: 120,
				Mode:                 "truncate",
				BusyReaders:          0,
				LogFrames:            8,
				CheckpointedFrames:   8,
			},
			Backup: DatabaseBackupSnapshot{
				Dir:              "E:/backups",
				IntervalSeconds:  86400,
				KeepLatest:       7,
				LastBackupAtUnix: 121,
				LastBackupPath:   "E:/backups/roodox-backup-20260401-150000.db",
			},
		},
		observability: ServerObservabilitySnapshot{
			FileRangeWriteCalls:     10,
			FileRangeWriteBytes:     1024,
			FileRangeWriteConflicts: 1,
			SmallWriteBursts:        2,
			SmallWriteHotPaths: []observability.PathCount{
				{Path: "src/app.txt", Count: 2},
			},
			BuildSuccessCount: 3,
			BuildFailureCount: 1,
			BuildLogBytes:     4096,
			BuildQueueWait: observability.DurationStats{
				Count: 3,
				P50Ms: 10,
				P95Ms: 20,
				P99Ms: 30,
			},
			BuildDuration: observability.DurationStats{
				Count: 4,
				P50Ms: 100,
				P95Ms: 200,
				P99Ms: 300,
			},
			RPCMetrics: []observability.RPCLatency{
				{
					Method:     "/roodox.core.v1.CoreService/WriteFileRange",
					Count:      5,
					ErrorCount: 1,
					P50Ms:      4,
					P95Ms:      8,
					P99Ms:      9,
				},
			},
		},
		backup: DatabaseBackupSnapshot{
			LastBackupAtUnix: 122,
			LastBackupPath:   "E:/backups/manual.db",
		},
		shutdown: ServerShutdownSnapshot{
			Accepted:        true,
			RequestedAtUnix: 123,
			Message:         "test shutdown",
		},
	})

	runtimeResp, err := svc.GetServerRuntime(context.Background(), &pb.GetServerRuntimeRequest{})
	if err != nil {
		t.Fatalf("GetServerRuntime returned error: %v", err)
	}
	if runtimeResp.ServerId != "srv-main" {
		t.Fatalf("ServerId = %q, want %q", runtimeResp.ServerId, "srv-main")
	}
	if runtimeResp.Checkpoint.GetMode() != "truncate" {
		t.Fatalf("Checkpoint.Mode = %q, want %q", runtimeResp.Checkpoint.GetMode(), "truncate")
	}
	if runtimeResp.Backup.GetLastBackupPath() == "" {
		t.Fatal("expected backup path to be populated")
	}

	observabilityResp, err := svc.GetServerObservability(context.Background(), &pb.GetServerObservabilityRequest{})
	if err != nil {
		t.Fatalf("GetServerObservability returned error: %v", err)
	}
	if observabilityResp.WriteFileRangeCalls != 10 {
		t.Fatalf("WriteFileRangeCalls = %d, want 10", observabilityResp.WriteFileRangeCalls)
	}
	if len(observabilityResp.RpcMetrics) != 1 {
		t.Fatalf("RpcMetrics len = %d, want 1", len(observabilityResp.RpcMetrics))
	}
	if observabilityResp.Build.GetSuccessCount() != 3 {
		t.Fatalf("Build.SuccessCount = %d, want 3", observabilityResp.Build.GetSuccessCount())
	}

	backupResp, err := svc.TriggerServerBackup(context.Background(), &pb.TriggerServerBackupRequest{})
	if err != nil {
		t.Fatalf("TriggerServerBackup returned error: %v", err)
	}
	if backupResp.Path != "E:/backups/manual.db" {
		t.Fatalf("TriggerServerBackup path = %q, want %q", backupResp.Path, "E:/backups/manual.db")
	}

	shutdownResp, err := svc.ShutdownServer(context.Background(), &pb.ShutdownServerRequest{Reason: "test shutdown"})
	if err != nil {
		t.Fatalf("ShutdownServer returned error: %v", err)
	}
	if !shutdownResp.Accepted {
		t.Fatal("ShutdownServer should be accepted")
	}
	if shutdownResp.Message != "test shutdown" {
		t.Fatalf("ShutdownServer message = %q, want %q", shutdownResp.Message, "test shutdown")
	}
}

func TestAdminConsoleServiceServerRuntimeRequiresProvider(t *testing.T) {
	svc := NewAdminConsoleService(nil, ControlPlaneConfig{})

	if _, err := svc.GetServerRuntime(context.Background(), &pb.GetServerRuntimeRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("GetServerRuntime code = %v, want %v", status.Code(err), codes.FailedPrecondition)
	}
	if _, err := svc.GetServerObservability(context.Background(), &pb.GetServerObservabilityRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("GetServerObservability code = %v, want %v", status.Code(err), codes.FailedPrecondition)
	}
	if _, err := svc.TriggerServerBackup(context.Background(), &pb.TriggerServerBackupRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("TriggerServerBackup code = %v, want %v", status.Code(err), codes.FailedPrecondition)
	}
	if _, err := svc.ShutdownServer(context.Background(), &pb.ShutdownServerRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("ShutdownServer code = %v, want %v", status.Code(err), codes.FailedPrecondition)
	}
}

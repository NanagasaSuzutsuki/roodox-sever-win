package server

import (
	"context"

	"roodox_server/internal/observability"
)

type FileStatSnapshot struct {
	Path           string
	Exists         bool
	SizeBytes      int64
	ModifiedAtUnix int64
}

type DatabaseCheckpointSnapshot struct {
	LastCheckpointAtUnix int64
	Mode                 string
	BusyReaders          int64
	LogFrames            int64
	CheckpointedFrames   int64
	LastError            string
}

type DatabaseBackupSnapshot struct {
	Dir              string
	IntervalSeconds  int64
	KeepLatest       uint32
	LastBackupAtUnix int64
	LastBackupPath   string
	LastError        string
}

type ServerShutdownSnapshot struct {
	Accepted          bool
	AlreadyInProgress bool
	RequestedAtUnix   int64
	Message           string
}

type ServerRuntimeSnapshot struct {
	ServerID      string
	ListenAddr    string
	RootDir       string
	DBPath        string
	TLSEnabled    bool
	AuthEnabled   bool
	StartedAtUnix int64
	HealthState   string
	HealthMessage string
	DBFile        FileStatSnapshot
	WALFile       FileStatSnapshot
	SHMFile       FileStatSnapshot
	Checkpoint    DatabaseCheckpointSnapshot
	Backup        DatabaseBackupSnapshot
}

type ServerObservabilitySnapshot struct {
	FileRangeWriteCalls     int64
	FileRangeWriteBytes     int64
	FileRangeWriteConflicts int64
	SmallWriteBursts        int64
	SmallWriteHotPaths      []observability.PathCount
	BuildSuccessCount       int64
	BuildFailureCount       int64
	BuildLogBytes           int64
	BuildQueueWait          observability.DurationStats
	BuildDuration           observability.DurationStats
	RPCMetrics              []observability.RPCLatency
}

type ServerRuntimeProvider interface {
	GetServerRuntimeSnapshot(ctx context.Context) (ServerRuntimeSnapshot, error)
	GetServerObservabilitySnapshot(ctx context.Context) (ServerObservabilitySnapshot, error)
	TriggerServerBackup(ctx context.Context) (DatabaseBackupSnapshot, error)
	RequestServerShutdown(ctx context.Context, reason string) (ServerShutdownSnapshot, error)
}

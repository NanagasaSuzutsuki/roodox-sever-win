package serverapp

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"roodox_server/internal/db"
	"roodox_server/internal/observability"
	"roodox_server/internal/server"
)

func (r *Runtime) GetServerRuntimeSnapshot(_ context.Context) (server.ServerRuntimeSnapshot, error) {
	if r == nil {
		return server.ServerRuntimeSnapshot{}, fmt.Errorf("runtime is not initialized")
	}

	r.mu.RLock()
	cfg := r.cfg
	running := r.running
	lastErr := r.lastErr
	startedAt := r.startedAt
	r.mu.RUnlock()

	listenAddr := cfg.Addr
	if r.lis != nil {
		listenAddr = r.lis.Addr().String()
	}

	stats, err := r.database.Stats()
	if err != nil {
		return server.ServerRuntimeSnapshot{}, err
	}
	if err := r.database.Ping(); err != nil && lastErr == "" {
		lastErr = err.Error()
	}

	checkpoint := server.DatabaseCheckpointSnapshot{}
	backup := server.DatabaseBackupSnapshot{}
	if r.dbMaint != nil {
		checkpoint, backup = r.dbMaint.Snapshot()
	}

	healthState := "serving"
	healthMessage := "ok"
	switch {
	case !running:
		healthState = "stopped"
		healthMessage = "runtime is not running"
	case lastErr != "":
		healthState = "degraded"
		healthMessage = lastErr
	}

	controlPlaneConfig := buildControlPlaneConfig(cfg)

	return server.ServerRuntimeSnapshot{
		ServerID:      controlPlaneConfig.ServerID,
		ListenAddr:    listenAddr,
		RootDir:       cfg.RootDir,
		DBPath:        r.database.Path(),
		TLSEnabled:    cfg.TLSEnabled,
		AuthEnabled:   cfg.AuthEnabled,
		StartedAtUnix: startedAt.Unix(),
		HealthState:   healthState,
		HealthMessage: healthMessage,
		DBFile:        fileStatToSnapshot(stats.DBFile),
		WALFile:       fileStatToSnapshot(stats.WALFile),
		SHMFile:       fileStatToSnapshot(stats.SHMFile),
		Checkpoint:    checkpoint,
		Backup:        backup,
	}, nil
}

func (r *Runtime) GetServerObservabilitySnapshot(_ context.Context) (server.ServerObservabilitySnapshot, error) {
	if r == nil || r.metrics == nil {
		return server.ServerObservabilitySnapshot{}, fmt.Errorf("observability recorder is not initialized")
	}

	snap := r.metrics.Snapshot()
	return server.ServerObservabilitySnapshot{
		FileRangeWriteCalls:     snap.RangeWriteCalls,
		FileRangeWriteBytes:     snap.RangeWriteBytes,
		FileRangeWriteConflicts: snap.RangeWriteConflictCalls,
		SmallWriteBursts:        snap.SmallWriteBursts,
		SmallWriteHotPaths:      append([]observability.PathCount(nil), snap.SmallWriteHotPaths...),
		BuildSuccessCount:       snap.BuildSuccessCount,
		BuildFailureCount:       snap.BuildFailureCount,
		BuildLogBytes:           snap.BuildLogBytes,
		BuildQueueWait:          snap.BuildQueueWait,
		BuildDuration:           snap.BuildDuration,
		RPCMetrics:              append([]observability.RPCLatency(nil), snap.RPCLatencies...),
	}, nil
}

func (r *Runtime) TriggerServerBackup(ctx context.Context) (server.DatabaseBackupSnapshot, error) {
	if r == nil || r.dbMaint == nil {
		return server.DatabaseBackupSnapshot{}, fmt.Errorf("database maintenance is not configured")
	}
	return r.dbMaint.BackupNow(ctx)
}

func (r *Runtime) RequestServerShutdown(_ context.Context, reason string) (server.ServerShutdownSnapshot, error) {
	if r == nil {
		return server.ServerShutdownSnapshot{}, fmt.Errorf("runtime is not initialized")
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "admin_console_shutdown"
	}

	requestedAt := time.Now()

	r.mu.Lock()
	if !r.running {
		lastErr := r.lastErr
		r.mu.Unlock()
		message := "server is not running"
		if lastErr != "" {
			message = lastErr
		}
		return server.ServerShutdownSnapshot{
			Accepted:          false,
			AlreadyInProgress: false,
			RequestedAtUnix:   requestedAt.Unix(),
			Message:           message,
		}, nil
	}
	if !r.shutdownAt.IsZero() {
		inFlightAt := r.shutdownAt
		message := r.shutdownMsg
		r.mu.Unlock()
		if message == "" {
			message = "shutdown already in progress"
		}
		return server.ServerShutdownSnapshot{
			Accepted:          false,
			AlreadyInProgress: true,
			RequestedAtUnix:   inFlightAt.Unix(),
			Message:           message,
		}, nil
	}
	r.shutdownAt = requestedAt
	r.shutdownMsg = reason
	r.mu.Unlock()

	go func() {
		time.Sleep(200 * time.Millisecond)
		log.Printf("component=runtime op=shutdown requested_at=%d reason=%q", requestedAt.Unix(), reason)
		r.Stop()
	}()

	return server.ServerShutdownSnapshot{
		Accepted:          true,
		AlreadyInProgress: false,
		RequestedAtUnix:   requestedAt.Unix(),
		Message:           reason,
	}, nil
}

func fileStatToSnapshot(item db.FileStat) server.FileStatSnapshot {
	return server.FileStatSnapshot{
		Path:           item.Path,
		Exists:         item.Exists,
		SizeBytes:      item.SizeBytes,
		ModifiedAtUnix: item.ModifiedAtUnix,
	}
}

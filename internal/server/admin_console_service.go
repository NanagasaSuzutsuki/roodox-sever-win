package server

import (
	"context"
	"strings"
	"time"

	"roodox_server/internal/accessbundle"
	"roodox_server/internal/db"
	pb "roodox_server/proto"
)

type AdminConsoleService struct {
	pb.UnimplementedAdminConsoleServiceServer

	registry *db.DeviceRegistry
	config   ControlPlaneConfig
	runtime  ServerRuntimeProvider
}

func NewAdminConsoleService(registry *db.DeviceRegistry, config ControlPlaneConfig) *AdminConsoleService {
	return &AdminConsoleService{
		registry: registry,
		config:   normalizeControlPlaneConfig(config),
	}
}

func (s *AdminConsoleService) ConfigureRuntime(provider ServerRuntimeProvider) {
	if s == nil {
		return
	}
	s.runtime = provider
}

func (s *AdminConsoleService) ListDevices(ctx context.Context, _ *pb.ListDevicesRequest) (*pb.ListDevicesResponse, error) {
	records, err := s.registry.List()
	if err != nil {
		return nil, toGrpcError(err)
	}

	now := time.Now()
	resp := &pb.ListDevicesResponse{
		Devices: make([]*pb.DeviceSummary, 0, len(records)),
	}
	for _, record := range records {
		resp.Devices = append(resp.Devices, buildDeviceSummary(record, now))
	}

	LogRequestEvent(ctx, "component=admin_console op=ListDevices count=%d", len(resp.Devices))
	return resp, nil
}

func (s *AdminConsoleService) GetDeviceDetail(ctx context.Context, req *pb.GetDeviceDetailRequest) (*pb.GetDeviceDetailResponse, error) {
	deviceID := strings.TrimSpace(req.GetDeviceId())
	if deviceID == "" {
		return nil, invalidArgument("device_id is required")
	}

	record, err := s.registry.Get(deviceID)
	if err != nil {
		return nil, toGrpcError(err)
	}

	assignedConfig := s.config.DefaultAssignedConfig
	if policy, err := s.registry.GetPolicy(deviceID); err == nil {
		assignedConfig = assignedConfigFromPolicyRecord(policy)
	} else if err != nil && !isNotExistError(err) {
		return nil, toGrpcError(err)
	}

	actions, err := s.registry.ListActiveActions(deviceID, 16)
	if err != nil {
		return nil, toGrpcError(err)
	}
	diagnostics, err := s.registry.ListDiagnostics(deviceID, s.config.DiagnosticsKeepLatest)
	if err != nil {
		return nil, toGrpcError(err)
	}

	actionDetails := make([]*pb.ClientAction, 0, len(actions))
	for _, action := range actions {
		actionDetails = append(actionDetails, clientActionToProto(action, false))
	}
	diagnosticSummaries := make([]*pb.DiagnosticSummary, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		diagnosticSummaries = append(diagnosticSummaries, diagnosticSummaryToProto(diagnostic))
	}

	now := time.Now()
	resp := &pb.GetDeviceDetailResponse{
		Summary:             buildDeviceSummary(record, now),
		DeviceName:          record.DeviceName,
		Platform:            record.Platform,
		Capabilities:        append([]string(nil), record.Capabilities...),
		ServerId:            record.ServerID,
		DeviceGroup:         record.DeviceGroup,
		SessionId:           record.LastSessionID,
		OverlayConnected:    record.OverlayConnected,
		GrpcConnected:       record.GRPCConnected,
		LastError:           record.LastError,
		LastSyncTimeUnix:    record.LastSyncTimeUnix,
		CurrentTaskCount:    record.CurrentTaskCount,
		SyncLastSuccessTime: record.SyncLastSuccessTime,
		SyncLastError:       record.SyncLastError,
		ConflictCount:       record.ConflictCount,
		QueueDepth:          record.QueueDepth,
		SyncSummary:         record.SyncSummary,
		Mounted:             record.Mounted,
		MountPath:           record.MountPath,
		LastMountTimeUnix:   record.LastMountTimeUnix,
		MountLastError:      record.MountLastError,
		AssignedConfig:      assignedConfigToProto(assignedConfig),
		PendingActions:      actionDetails,
		RecentDiagnostics:   diagnosticSummaries,
		AvailableActions:    append([]string(nil), s.config.AvailableActions...),
		RequiresPolicyPull:  record.RequiresPolicyPull,
		LastRegisteredAt:    record.LastRegisteredAtUnix,
		LastHeartbeatAt:     record.LastHeartbeatAtUnix,
		LastSyncReportAt:    record.LastSyncReportAtUnix,
		LastMountReportAt:   record.LastMountReportAtUnix,
	}

	LogRequestEvent(ctx, "component=admin_console op=GetDeviceDetail device_id=%q", deviceID)
	return resp, nil
}

func (s *AdminConsoleService) IssueJoinBundle(ctx context.Context, req *pb.IssueJoinBundleRequest) (*pb.IssueJoinBundleResponse, error) {
	overlayProvider := coalesceNonEmpty(req.GetOverlayProvider(), s.config.JoinBundle.OverlayProvider)
	if overlayProvider == "" {
		return nil, failedPrecondition("join bundle overlay provider is not configured")
	}
	serviceDiscovery := s.config.JoinBundle.ServiceDiscovery
	if strings.TrimSpace(serviceDiscovery.Host) == "" || serviceDiscovery.Port == 0 {
		return nil, failedPrecondition("join bundle service discovery host/port is not configured")
	}

	overlayJoinConfigJSON := strings.TrimSpace(req.GetOverlayJoinConfigJson())
	if overlayJoinConfigJSON == "" {
		overlayJoinConfigJSON = s.config.JoinBundle.OverlayJoinConfigJSON
	}
	if overlayJoinConfigJSON == "" {
		overlayJoinConfigJSON = "{}"
	}

	deviceGroup := coalesceNonEmpty(req.GetDeviceGroup(), s.config.DefaultDeviceGroup)
	bundleModel := accessbundle.Bundle{
		Version: accessbundle.DefaultVersion,
		Overlay: accessbundle.Overlay{
			Provider:   overlayProvider,
			JoinConfig: []byte(overlayJoinConfigJSON),
		},
		ServiceDiscovery: accessbundle.ServiceDiscovery{
			Mode:          serviceDiscovery.Mode,
			Host:          serviceDiscovery.Host,
			Port:          serviceDiscovery.Port,
			UseTLS:        serviceDiscovery.UseTLS,
			TLSServerName: serviceDiscovery.TLSServerName,
		},
		Roodox: accessbundle.Roodox{
			ServerID:     s.config.ServerID,
			DeviceGroup:  deviceGroup,
			SharedSecret: s.config.JoinBundle.SharedSecret,
			DeviceID:     strings.TrimSpace(req.GetDeviceId()),
			DeviceName:   strings.TrimSpace(req.GetDeviceName()),
			DeviceRole:   strings.TrimSpace(req.GetDeviceRole()),
		},
	}
	normalizedBundle := bundleModel.Normalize()
	bundleJSON, err := normalizedBundle.MarshalJSONFile()
	if err != nil {
		return nil, invalidArgument(err.Error())
	}

	bundle := &pb.JoinBundle{
		Version:               normalizedBundle.Version,
		OverlayProvider:       normalizedBundle.Overlay.Provider,
		OverlayJoinConfigJson: string(normalizedBundle.Overlay.JoinConfig),
		ServiceDiscoveryMode:  normalizedBundle.ServiceDiscovery.Mode,
		ServiceHost:           normalizedBundle.ServiceDiscovery.Host,
		ServicePort:           normalizedBundle.ServiceDiscovery.Port,
		UseTls:                normalizedBundle.ServiceDiscovery.UseTLS,
		TlsServerName:         normalizedBundle.ServiceDiscovery.TLSServerName,
		ServerId:              s.config.ServerID,
		DeviceGroup:           deviceGroup,
		SharedSecret:          s.config.JoinBundle.SharedSecret,
		DeviceId:              strings.TrimSpace(req.GetDeviceId()),
		DeviceName:            strings.TrimSpace(req.GetDeviceName()),
		DeviceRole:            strings.TrimSpace(req.GetDeviceRole()),
	}

	LogRequestEvent(ctx, "component=admin_console op=IssueJoinBundle device_id=%q overlay_provider=%q", bundle.GetDeviceId(), bundle.GetOverlayProvider())
	return &pb.IssueJoinBundleResponse{
		BundleJson: bundleJSON,
		Bundle:     bundle,
	}, nil
}

func (s *AdminConsoleService) UpdateDevicePolicy(ctx context.Context, req *pb.UpdateDevicePolicyRequest) (*pb.UpdateDevicePolicyResponse, error) {
	deviceID := strings.TrimSpace(req.GetDeviceId())
	if deviceID == "" {
		return nil, invalidArgument("device_id is required")
	}

	record, err := s.registry.Get(deviceID)
	if err != nil {
		return nil, toGrpcError(err)
	}
	if expected := req.GetExpectedPolicyRevision(); expected != 0 && expected != record.PolicyRevision {
		return nil, failedPrecondition("expected_policy_revision does not match current policy_revision")
	}

	nextRevision := record.PolicyRevision + 1
	if nextRevision == 0 {
		nextRevision = 1
	}

	effectiveConfig := s.config.DefaultAssignedConfig
	if req.GetResetToDefault() {
		if err := s.registry.ResetPolicy(deviceID, nextRevision); err != nil {
			return nil, toGrpcError(err)
		}
	} else {
		if req.GetPolicy() == nil {
			return nil, invalidArgument("policy is required unless reset_to_default is true")
		}
		effectiveConfig = assignedConfigFromProto(req.GetPolicy())
		if _, err := s.registry.UpsertPolicy(db.DevicePolicyRecord{
			DeviceID:           deviceID,
			MountPath:          effectiveConfig.MountPath,
			SyncRoots:          effectiveConfig.SyncRoots,
			ConflictPolicy:     effectiveConfig.ConflictPolicy,
			ReadOnly:           effectiveConfig.ReadOnly,
			AutoConnect:        effectiveConfig.AutoConnect,
			BandwidthLimit:     effectiveConfig.BandwidthLimit,
			LogLevel:           effectiveConfig.LogLevel,
			LargeFileThreshold: effectiveConfig.LargeFileThreshold,
			PolicyRevision:     nextRevision,
		}); err != nil {
			return nil, toGrpcError(err)
		}
	}

	LogRequestEvent(ctx, "component=admin_console op=UpdateDevicePolicy device_id=%q policy_revision=%d reset=%t", deviceID, nextRevision, req.GetResetToDefault())
	return &pb.UpdateDevicePolicyResponse{
		PolicyRevision:     nextRevision,
		RequiresPolicyPull: true,
		EffectivePolicy:    assignedConfigToProto(effectiveConfig),
	}, nil
}

func (s *AdminConsoleService) RequestClientAction(ctx context.Context, req *pb.RequestClientActionRequest) (*pb.RequestClientActionResponse, error) {
	deviceID := strings.TrimSpace(req.GetDeviceId())
	if deviceID == "" {
		return nil, invalidArgument("device_id is required")
	}
	if strings.TrimSpace(req.GetActionType()) == "" {
		return nil, invalidArgument("action_type is required")
	}
	if _, err := s.registry.Get(deviceID); err != nil {
		return nil, toGrpcError(err)
	}

	action, err := s.registry.EnqueueAction(db.DeviceActionRequest{
		DeviceID:              deviceID,
		ActionType:            req.GetActionType(),
		PayloadJSON:           req.GetPayloadJson(),
		ReplaceSimilarPending: req.GetReplaceSimilarPending(),
	})
	if err != nil {
		return nil, toGrpcError(err)
	}

	LogRequestEvent(ctx, "component=admin_console op=RequestClientAction device_id=%q action_type=%q action_id=%q", deviceID, action.ActionType, action.ActionID)
	return &pb.RequestClientActionResponse{
		Action: clientActionToProto(action, false),
	}, nil
}

func (s *AdminConsoleService) GetServerRuntime(ctx context.Context, _ *pb.GetServerRuntimeRequest) (*pb.GetServerRuntimeResponse, error) {
	if s.runtime == nil {
		return nil, failedPrecondition("server runtime provider is not configured")
	}

	snapshot, err := s.runtime.GetServerRuntimeSnapshot(ctx)
	if err != nil {
		return nil, toGrpcError(err)
	}

	LogRequestEvent(ctx, "component=admin_console op=GetServerRuntime")
	return &pb.GetServerRuntimeResponse{
		ServerId:      snapshot.ServerID,
		ListenAddr:    snapshot.ListenAddr,
		RootDir:       snapshot.RootDir,
		DbPath:        snapshot.DBPath,
		TlsEnabled:    snapshot.TLSEnabled,
		AuthEnabled:   snapshot.AuthEnabled,
		StartedAtUnix: snapshot.StartedAtUnix,
		HealthState:   snapshot.HealthState,
		HealthMessage: snapshot.HealthMessage,
		DbFile:        fileStatSnapshotToProto(snapshot.DBFile),
		WalFile:       fileStatSnapshotToProto(snapshot.WALFile),
		ShmFile:       fileStatSnapshotToProto(snapshot.SHMFile),
		Checkpoint: &pb.DatabaseCheckpointStatus{
			LastCheckpointAtUnix: snapshot.Checkpoint.LastCheckpointAtUnix,
			Mode:                 snapshot.Checkpoint.Mode,
			BusyReaders:          snapshot.Checkpoint.BusyReaders,
			LogFrames:            snapshot.Checkpoint.LogFrames,
			CheckpointedFrames:   snapshot.Checkpoint.CheckpointedFrames,
			LastError:            snapshot.Checkpoint.LastError,
		},
		Backup: &pb.DatabaseBackupStatus{
			Dir:              snapshot.Backup.Dir,
			IntervalSeconds:  snapshot.Backup.IntervalSeconds,
			KeepLatest:       snapshot.Backup.KeepLatest,
			LastBackupAtUnix: snapshot.Backup.LastBackupAtUnix,
			LastBackupPath:   snapshot.Backup.LastBackupPath,
			LastError:        snapshot.Backup.LastError,
		},
	}, nil
}

func (s *AdminConsoleService) GetServerObservability(ctx context.Context, _ *pb.GetServerObservabilityRequest) (*pb.GetServerObservabilityResponse, error) {
	if s.runtime == nil {
		return nil, failedPrecondition("server runtime provider is not configured")
	}

	snapshot, err := s.runtime.GetServerObservabilitySnapshot(ctx)
	if err != nil {
		return nil, toGrpcError(err)
	}

	resp := &pb.GetServerObservabilityResponse{
		WriteFileRangeCalls:     snapshot.FileRangeWriteCalls,
		WriteFileRangeBytes:     snapshot.FileRangeWriteBytes,
		WriteFileRangeConflicts: snapshot.FileRangeWriteConflicts,
		SmallWriteBursts:        snapshot.SmallWriteBursts,
		Build: &pb.BuildObservability{
			SuccessCount:   snapshot.BuildSuccessCount,
			FailureCount:   snapshot.BuildFailureCount,
			LogBytes:       snapshot.BuildLogBytes,
			QueueWaitCount: snapshot.BuildQueueWait.Count,
			QueueWaitP50Ms: snapshot.BuildQueueWait.P50Ms,
			QueueWaitP95Ms: snapshot.BuildQueueWait.P95Ms,
			QueueWaitP99Ms: snapshot.BuildQueueWait.P99Ms,
			DurationCount:  snapshot.BuildDuration.Count,
			DurationP50Ms:  snapshot.BuildDuration.P50Ms,
			DurationP95Ms:  snapshot.BuildDuration.P95Ms,
			DurationP99Ms:  snapshot.BuildDuration.P99Ms,
		},
	}
	for _, item := range snapshot.SmallWriteHotPaths {
		resp.SmallWriteHotPaths = append(resp.SmallWriteHotPaths, &pb.HotPathMetric{
			Path:  item.Path,
			Count: item.Count,
		})
	}
	for _, item := range snapshot.RPCMetrics {
		resp.RpcMetrics = append(resp.RpcMetrics, &pb.RpcLatencyMetric{
			Method:     item.Method,
			Count:      item.Count,
			ErrorCount: item.ErrorCount,
			P50Ms:      item.P50Ms,
			P95Ms:      item.P95Ms,
			P99Ms:      item.P99Ms,
		})
	}

	LogRequestEvent(ctx, "component=admin_console op=GetServerObservability")
	return resp, nil
}

func (s *AdminConsoleService) TriggerServerBackup(ctx context.Context, _ *pb.TriggerServerBackupRequest) (*pb.TriggerServerBackupResponse, error) {
	if s.runtime == nil {
		return nil, failedPrecondition("server runtime provider is not configured")
	}

	backup, err := s.runtime.TriggerServerBackup(ctx)
	if err != nil {
		return nil, toGrpcError(err)
	}

	LogRequestEvent(ctx, "component=admin_console op=TriggerServerBackup path=%q", backup.LastBackupPath)
	return &pb.TriggerServerBackupResponse{
		CreatedAtUnix: backup.LastBackupAtUnix,
		Path:          backup.LastBackupPath,
	}, nil
}

func (s *AdminConsoleService) ShutdownServer(ctx context.Context, req *pb.ShutdownServerRequest) (*pb.ShutdownServerResponse, error) {
	if s.runtime == nil {
		return nil, failedPrecondition("server runtime provider is not configured")
	}

	shutdown, err := s.runtime.RequestServerShutdown(ctx, req.GetReason())
	if err != nil {
		return nil, toGrpcError(err)
	}

	LogRequestEvent(ctx, "component=admin_console op=ShutdownServer accepted=%t already_in_progress=%t", shutdown.Accepted, shutdown.AlreadyInProgress)
	return &pb.ShutdownServerResponse{
		Accepted:          shutdown.Accepted,
		AlreadyInProgress: shutdown.AlreadyInProgress,
		RequestedAtUnix:   shutdown.RequestedAtUnix,
		Message:           shutdown.Message,
	}, nil
}

func deviceDisplayName(record *db.DeviceRecord) string {
	if trimmed := strings.TrimSpace(record.AssignedDeviceLabel); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(record.DeviceName); trimmed != "" {
		return trimmed
	}
	return record.DeviceID
}

func deviceOnlineState(record *db.DeviceRecord, now time.Time) string {
	lastSeenUnix := record.LastSeenAtUnix
	if lastSeenUnix == 0 {
		return "unknown"
	}

	interval := int64(record.HeartbeatIntervalSeconds)
	if interval <= 0 {
		interval = int64(defaultHeartbeatIntervalSeconds)
	}
	offlineAfter := interval * 3
	if offlineAfter < 45 {
		offlineAfter = 45
	}
	if now.Unix()-lastSeenUnix > offlineAfter {
		return "offline"
	}

	if !record.GRPCConnected {
		return "degraded"
	}
	if strings.TrimSpace(record.OverlayProvider) != "" && !record.OverlayConnected {
		return "degraded"
	}
	if strings.TrimSpace(record.LastError) != "" || strings.TrimSpace(record.SyncLastError) != "" {
		return "degraded"
	}
	return "online"
}

func deviceSyncState(record *db.DeviceRecord) string {
	if trimmed := strings.TrimSpace(record.SyncSummary); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(record.SyncStateSummary); trimmed != "" {
		return trimmed
	}
	if strings.TrimSpace(record.SyncLastError) != "" {
		return "error"
	}
	if record.CurrentTaskCount > 0 {
		return "running"
	}
	return "idle"
}

func deviceMountState(record *db.DeviceRecord) string {
	if trimmed := strings.TrimSpace(record.MountState); trimmed != "" {
		return trimmed
	}
	return "unknown"
}

func fileStatSnapshotToProto(item FileStatSnapshot) *pb.FileStatSummary {
	return &pb.FileStatSummary{
		Path:           item.Path,
		Exists:         item.Exists,
		SizeBytes:      item.SizeBytes,
		ModifiedAtUnix: item.ModifiedAtUnix,
	}
}

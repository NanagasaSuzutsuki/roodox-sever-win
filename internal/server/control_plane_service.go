package server

import (
	"context"
	"strings"

	"roodox_server/internal/db"
	pb "roodox_server/proto"
)

const (
	defaultHeartbeatIntervalSeconds       uint32 = 15
	defaultPolicyRevision                 uint64 = 1
	defaultConflictPolicy                        = "manual"
	defaultLogLevel                              = "info"
	defaultLargeFileThreshold             int64  = 64 << 20
	defaultDiagnosticsKeepLatestPerDevice        = 20
)

type AssignedConfig struct {
	MountPath          string
	SyncRoots          []string
	ConflictPolicy     string
	ReadOnly           bool
	AutoConnect        bool
	BandwidthLimit     int64
	LogLevel           string
	LargeFileThreshold int64
}

type JoinBundleConfig struct {
	OverlayProvider       string
	OverlayJoinConfigJSON string
	ServiceDiscovery      ServiceDiscoveryConfig
	SharedSecret          string
}

type ServiceDiscoveryConfig struct {
	Mode          string
	Host          string
	Port          uint32
	UseTLS        bool
	TLSServerName string
}

type ControlPlaneConfig struct {
	ServerID                 string
	DefaultDeviceGroup       string
	HeartbeatIntervalSeconds uint32
	DefaultPolicyRevision    uint64
	DefaultAssignedConfig    AssignedConfig
	JoinBundle               JoinBundleConfig
	AvailableActions         []string
	DiagnosticsKeepLatest    int
}

type ControlPlaneService struct {
	pb.UnimplementedControlPlaneServiceServer

	registry *db.DeviceRegistry
	config   ControlPlaneConfig
}

func NewControlPlaneService(registry *db.DeviceRegistry, config ControlPlaneConfig) *ControlPlaneService {
	return &ControlPlaneService{
		registry: registry,
		config:   normalizeControlPlaneConfig(config),
	}
}

func (s *ControlPlaneService) RegisterDevice(ctx context.Context, req *pb.RegisterDeviceRequest) (*pb.RegisterDeviceResponse, error) {
	deviceID := strings.TrimSpace(req.GetDeviceId())
	if deviceID == "" {
		return nil, invalidArgument("device_id is required")
	}

	record, err := s.registry.Register(db.DeviceRegistration{
		DeviceID:                 deviceID,
		DeviceName:               req.GetDeviceName(),
		DeviceRole:               req.GetDeviceRole(),
		AssignedDeviceLabel:      fallbackDisplayName(req.GetDeviceName(), deviceID),
		ClientVersion:            req.GetClientVersion(),
		Platform:                 req.GetPlatform(),
		OverlayProvider:          req.GetOverlayProvider(),
		OverlayAddress:           req.GetOverlayAddress(),
		Capabilities:             req.GetCapabilities(),
		ServerID:                 coalesceNonEmpty(s.config.ServerID, req.GetServerId()),
		DeviceGroup:              coalesceNonEmpty(req.GetDeviceGroup(), s.config.DefaultDeviceGroup),
		HeartbeatIntervalSeconds: s.config.HeartbeatIntervalSeconds,
		PolicyRevision:           s.config.DefaultPolicyRevision,
		RequiresPolicyPull:       true,
		OverlayConnected:         true,
	})
	if err != nil {
		return nil, toGrpcError(err)
	}

	LogRequestEvent(
		ctx,
		"component=control_plane op=RegisterDevice device_id=%q device_name=%q overlay_provider=%q device_group=%q",
		record.DeviceID,
		record.DeviceName,
		record.OverlayProvider,
		record.DeviceGroup,
	)

	return &pb.RegisterDeviceResponse{
		Accepted:                 true,
		AssignedDeviceLabel:      record.AssignedDeviceLabel,
		HeartbeatIntervalSeconds: record.HeartbeatIntervalSeconds,
		PolicyRevision:           record.PolicyRevision,
		RequiresPolicyPull:       record.RequiresPolicyPull,
	}, nil
}

func (s *ControlPlaneService) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	deviceID := strings.TrimSpace(req.GetDeviceId())
	if deviceID == "" {
		return nil, invalidArgument("device_id is required")
	}

	record, err := s.registry.UpdateHeartbeat(db.DeviceHeartbeat{
		DeviceID:         deviceID,
		SessionID:        req.GetSessionId(),
		TimestampUnix:    req.GetTimestampUnix(),
		OverlayConnected: req.GetOverlayConnected(),
		GRPCConnected:    req.GetGrpcConnected(),
		LastSyncTimeUnix: req.GetLastSyncTimeUnix(),
		LastError:        req.GetLastError(),
		MountState:       req.GetMountState(),
		SyncStateSummary: req.GetSyncStateSummary(),
	})
	if err != nil {
		return nil, toGrpcError(err)
	}

	actions, err := s.registry.ListPendingActions(deviceID, 16)
	if err != nil {
		return nil, toGrpcError(err)
	}
	actionIDs := make([]string, 0, len(actions))
	actionNames := make([]string, 0, len(actions))
	actionDetails := make([]*pb.ClientAction, 0, len(actions))
	for _, action := range actions {
		actionIDs = append(actionIDs, action.ActionID)
		actionNames = append(actionNames, action.ActionType)
		actionDetails = append(actionDetails, clientActionToProto(action, true))
	}
	if err := s.registry.MarkActionsDelivered(actionIDs); err != nil {
		return nil, toGrpcError(err)
	}

	LogRequestEvent(
		ctx,
		"component=control_plane op=Heartbeat device_id=%q session_id=%q overlay_connected=%t grpc_connected=%t",
		record.DeviceID,
		record.LastSessionID,
		record.OverlayConnected,
		record.GRPCConnected,
	)

	return &pb.HeartbeatResponse{
		NextHeartbeatSeconds: record.HeartbeatIntervalSeconds,
		PolicyRevision:       record.PolicyRevision,
		PendingActions:       actionNames,
		PendingActionDetails: actionDetails,
	}, nil
}

func (s *ControlPlaneService) GetAssignedConfig(ctx context.Context, req *pb.GetAssignedConfigRequest) (*pb.GetAssignedConfigResponse, error) {
	deviceID := strings.TrimSpace(req.GetDeviceId())
	if deviceID == "" {
		return nil, invalidArgument("device_id is required")
	}

	record, err := s.registry.Get(deviceID)
	if err != nil {
		return nil, toGrpcError(err)
	}
	if err := s.registry.MarkPolicyPulled(deviceID); err != nil {
		return nil, toGrpcError(err)
	}

	config := s.config.DefaultAssignedConfig
	if override, err := s.registry.GetPolicy(deviceID); err == nil {
		config = assignedConfigFromPolicyRecord(override)
	} else if err != nil && !isNotExistError(err) {
		return nil, toGrpcError(err)
	}
	LogRequestEvent(
		ctx,
		"component=control_plane op=GetAssignedConfig device_id=%q policy_revision=%d sync_roots=%d",
		record.DeviceID,
		record.PolicyRevision,
		len(config.SyncRoots),
	)

	return &pb.GetAssignedConfigResponse{
		MountPath:          config.MountPath,
		SyncRoots:          append([]string(nil), config.SyncRoots...),
		ConflictPolicy:     config.ConflictPolicy,
		ReadOnly:           config.ReadOnly,
		AutoConnect:        config.AutoConnect,
		BandwidthLimit:     config.BandwidthLimit,
		LogLevel:           config.LogLevel,
		LargeFileThreshold: config.LargeFileThreshold,
		PolicyRevision:     record.PolicyRevision,
	}, nil
}

func (s *ControlPlaneService) ReportSyncState(ctx context.Context, req *pb.ReportSyncStateRequest) (*pb.ReportSyncStateResponse, error) {
	deviceID := strings.TrimSpace(req.GetDeviceId())
	if deviceID == "" {
		return nil, invalidArgument("device_id is required")
	}

	record, err := s.registry.UpdateSyncState(db.DeviceSyncStateReport{
		DeviceID:         deviceID,
		CurrentTaskCount: uint32(req.GetCurrentTaskCount()),
		LastSuccessTime:  req.GetLastSuccessTime(),
		LastError:        req.GetLastError(),
		ConflictCount:    req.GetConflictCount(),
		QueueDepth:       uint32(req.GetQueueDepth()),
		Summary:          req.GetSummary(),
	})
	if err != nil {
		return nil, toGrpcError(err)
	}

	LogRequestEvent(
		ctx,
		"component=control_plane op=ReportSyncState device_id=%q queue_depth=%d conflict_count=%d",
		record.DeviceID,
		record.QueueDepth,
		record.ConflictCount,
	)

	return &pb.ReportSyncStateResponse{}, nil
}

func (s *ControlPlaneService) ReportMountState(ctx context.Context, req *pb.ReportMountStateRequest) (*pb.ReportMountStateResponse, error) {
	deviceID := strings.TrimSpace(req.GetDeviceId())
	if deviceID == "" {
		return nil, invalidArgument("device_id is required")
	}

	record, err := s.registry.UpdateMountState(db.DeviceMountStateReport{
		DeviceID:      deviceID,
		Mounted:       req.GetMounted(),
		MountPath:     req.GetMountPath(),
		LastMountTime: req.GetLastMountTime(),
		LastError:     req.GetLastError(),
	})
	if err != nil {
		return nil, toGrpcError(err)
	}

	LogRequestEvent(
		ctx,
		"component=control_plane op=ReportMountState device_id=%q mounted=%t mount_path=%q",
		record.DeviceID,
		record.Mounted,
		record.MountPath,
	)
	return &pb.ReportMountStateResponse{}, nil
}

func (s *ControlPlaneService) UploadDiagnostics(ctx context.Context, req *pb.UploadDiagnosticsRequest) (*pb.UploadDiagnosticsResponse, error) {
	deviceID := strings.TrimSpace(req.GetDeviceId())
	if deviceID == "" {
		return nil, invalidArgument("device_id is required")
	}
	if _, err := s.registry.Get(deviceID); err != nil {
		return nil, toGrpcError(err)
	}

	record, err := s.registry.InsertDiagnostic(db.DeviceDiagnosticRecord{
		DeviceID:    deviceID,
		Category:    req.GetCategory(),
		ContentType: req.GetContentType(),
		Summary:     req.GetSummary(),
		Payload:     req.GetData(),
		SizeBytes:   int64(len(req.GetData())),
	}, s.config.DiagnosticsKeepLatest)
	if err != nil {
		return nil, toGrpcError(err)
	}

	LogRequestEvent(
		ctx,
		"component=control_plane op=UploadDiagnostics device_id=%q category=%q size_bytes=%d",
		deviceID,
		record.Category,
		record.SizeBytes,
	)
	return &pb.UploadDiagnosticsResponse{
		DiagnosticsId:  record.DiagnosticsID,
		UploadedAtUnix: record.UploadedAtUnix,
		SizeBytes:      record.SizeBytes,
	}, nil
}

func normalizeControlPlaneConfig(config ControlPlaneConfig) ControlPlaneConfig {
	config.ServerID = strings.TrimSpace(config.ServerID)
	config.DefaultDeviceGroup = strings.TrimSpace(config.DefaultDeviceGroup)
	if config.DefaultDeviceGroup == "" {
		config.DefaultDeviceGroup = "default"
	}
	if config.HeartbeatIntervalSeconds == 0 {
		config.HeartbeatIntervalSeconds = defaultHeartbeatIntervalSeconds
	}
	if config.DefaultPolicyRevision == 0 {
		config.DefaultPolicyRevision = defaultPolicyRevision
	}
	if config.DiagnosticsKeepLatest <= 0 {
		config.DiagnosticsKeepLatest = defaultDiagnosticsKeepLatestPerDevice
	}

	config.DefaultAssignedConfig.MountPath = strings.TrimSpace(config.DefaultAssignedConfig.MountPath)
	config.DefaultAssignedConfig.SyncRoots = normalizeStringList(config.DefaultAssignedConfig.SyncRoots)
	if len(config.DefaultAssignedConfig.SyncRoots) == 0 {
		config.DefaultAssignedConfig.SyncRoots = []string{"."}
	}
	config.DefaultAssignedConfig.ConflictPolicy = strings.TrimSpace(config.DefaultAssignedConfig.ConflictPolicy)
	if config.DefaultAssignedConfig.ConflictPolicy == "" {
		config.DefaultAssignedConfig.ConflictPolicy = defaultConflictPolicy
	}
	config.DefaultAssignedConfig.LogLevel = strings.TrimSpace(config.DefaultAssignedConfig.LogLevel)
	if config.DefaultAssignedConfig.LogLevel == "" {
		config.DefaultAssignedConfig.LogLevel = defaultLogLevel
	}
	if config.DefaultAssignedConfig.LargeFileThreshold <= 0 {
		config.DefaultAssignedConfig.LargeFileThreshold = defaultLargeFileThreshold
	}
	config.JoinBundle.OverlayProvider = strings.TrimSpace(config.JoinBundle.OverlayProvider)
	config.JoinBundle.OverlayJoinConfigJSON = strings.TrimSpace(config.JoinBundle.OverlayJoinConfigJSON)
	config.JoinBundle.ServiceDiscovery.Mode = strings.TrimSpace(config.JoinBundle.ServiceDiscovery.Mode)
	if config.JoinBundle.ServiceDiscovery.Mode == "" {
		config.JoinBundle.ServiceDiscovery.Mode = "static"
	}
	config.JoinBundle.ServiceDiscovery.Host = strings.TrimSpace(config.JoinBundle.ServiceDiscovery.Host)
	config.JoinBundle.ServiceDiscovery.TLSServerName = strings.TrimSpace(config.JoinBundle.ServiceDiscovery.TLSServerName)
	config.JoinBundle.SharedSecret = strings.TrimSpace(config.JoinBundle.SharedSecret)
	config.AvailableActions = normalizeStringList(config.AvailableActions)
	if len(config.AvailableActions) == 0 {
		config.AvailableActions = []string{
			"reconnect_overlay",
			"resync",
			"remount",
			"collect_diagnostics",
		}
	}
	return config
}

func fallbackDisplayName(deviceName, deviceID string) string {
	if trimmed := strings.TrimSpace(deviceName); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(deviceID)
}

func normalizeStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func coalesceNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

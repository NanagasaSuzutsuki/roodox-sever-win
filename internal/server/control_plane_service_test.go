package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"roodox_server/internal/db"
	pb "roodox_server/proto"
)

func TestControlPlaneServiceLifecycle(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "control-plane.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Sql.Close()

	registry, err := db.NewDeviceRegistry(database)
	if err != nil {
		t.Fatalf("db.NewDeviceRegistry returned error: %v", err)
	}

	controlSvc := NewControlPlaneService(registry, ControlPlaneConfig{
		ServerID:                 "srv-main",
		DefaultDeviceGroup:       "default",
		HeartbeatIntervalSeconds: 20,
		DefaultPolicyRevision:    7,
		DefaultAssignedConfig: AssignedConfig{
			SyncRoots:          []string{"."},
			ConflictPolicy:     "manual",
			AutoConnect:        true,
			LogLevel:           "debug",
			LargeFileThreshold: 8 << 20,
		},
		JoinBundle: JoinBundleConfig{
			OverlayProvider:       "easytier",
			OverlayJoinConfigJSON: `{"networkName":"roodox-prod","peerTargets":["tcp://cp.roodox.internal:11010"]}`,
			ServiceDiscovery: ServiceDiscoveryConfig{
				Mode:          "static",
				Host:          "cp.roodox.internal",
				Port:          50051,
				UseTLS:        true,
				TLSServerName: "roodox.internal",
			},
			SharedSecret: "shared-secret-for-tests",
		},
		AvailableActions:      []string{"resync", "remount", "collect_diagnostics"},
		DiagnosticsKeepLatest: 4,
	})
	adminSvc := NewAdminConsoleService(registry, controlSvc.config)

	registerResp, err := controlSvc.RegisterDevice(context.Background(), &pb.RegisterDeviceRequest{
		DeviceId:        "device-1",
		DeviceName:      "Laptop",
		DeviceRole:      "developer",
		ClientVersion:   "1.2.3",
		Platform:        "windows",
		OverlayProvider: "easytier",
		OverlayAddress:  "10.144.0.12",
		Capabilities:    []string{"sync", "mount"},
		DeviceGroup:     "team-a",
	})
	if err != nil {
		t.Fatalf("RegisterDevice returned error: %v", err)
	}
	if !registerResp.Accepted {
		t.Fatal("RegisterDevice response was not accepted")
	}
	if registerResp.AssignedDeviceLabel != "Laptop" {
		t.Fatalf("AssignedDeviceLabel = %q, want %q", registerResp.AssignedDeviceLabel, "Laptop")
	}
	if registerResp.HeartbeatIntervalSeconds != 20 {
		t.Fatalf("HeartbeatIntervalSeconds = %d, want %d", registerResp.HeartbeatIntervalSeconds, 20)
	}
	if !registerResp.RequiresPolicyPull {
		t.Fatal("RequiresPolicyPull = false, want true")
	}

	configResp, err := controlSvc.GetAssignedConfig(context.Background(), &pb.GetAssignedConfigRequest{
		DeviceId: "device-1",
	})
	if err != nil {
		t.Fatalf("GetAssignedConfig returned error: %v", err)
	}
	if configResp.PolicyRevision != 7 {
		t.Fatalf("PolicyRevision = %d, want %d", configResp.PolicyRevision, 7)
	}
	if len(configResp.SyncRoots) != 1 || configResp.SyncRoots[0] != "." {
		t.Fatalf("SyncRoots = %v, want [.]", configResp.SyncRoots)
	}
	if !configResp.AutoConnect {
		t.Fatal("AutoConnect = false, want true")
	}

	record, err := registry.Get("device-1")
	if err != nil {
		t.Fatalf("registry.Get returned error: %v", err)
	}
	if record.RequiresPolicyPull {
		t.Fatal("RequiresPolicyPull still true after GetAssignedConfig")
	}

	nowUnix := time.Now().Unix()
	heartbeatResp, err := controlSvc.Heartbeat(context.Background(), &pb.HeartbeatRequest{
		DeviceId:         "device-1",
		SessionId:        "session-1",
		TimestampUnix:    nowUnix,
		OverlayConnected: true,
		GrpcConnected:    true,
		LastSyncTimeUnix: nowUnix,
		MountState:       "mounted",
		SyncStateSummary: "idle",
	})
	if err != nil {
		t.Fatalf("Heartbeat returned error: %v", err)
	}
	if heartbeatResp.NextHeartbeatSeconds != 20 {
		t.Fatalf("NextHeartbeatSeconds = %d, want %d", heartbeatResp.NextHeartbeatSeconds, 20)
	}
	if heartbeatResp.PolicyRevision != 7 {
		t.Fatalf("Heartbeat PolicyRevision = %d, want %d", heartbeatResp.PolicyRevision, 7)
	}

	if _, err := controlSvc.ReportSyncState(context.Background(), &pb.ReportSyncStateRequest{
		DeviceId:         "device-1",
		CurrentTaskCount: 2,
		LastSuccessTime:  nowUnix,
		ConflictCount:    1,
		QueueDepth:       4,
		Summary:          "running",
	}); err != nil {
		t.Fatalf("ReportSyncState returned error: %v", err)
	}

	if _, err := controlSvc.ReportMountState(context.Background(), &pb.ReportMountStateRequest{
		DeviceId:      "device-1",
		Mounted:       true,
		MountPath:     "/Volumes/Roodox",
		LastMountTime: nowUnix,
	}); err != nil {
		t.Fatalf("ReportMountState returned error: %v", err)
	}

	diagResp, err := controlSvc.UploadDiagnostics(context.Background(), &pb.UploadDiagnosticsRequest{
		DeviceId:    "device-1",
		Category:    "sync",
		ContentType: "application/json",
		Summary:     "latest sync trace",
		Data:        []byte(`{"ok":true}`),
	})
	if err != nil {
		t.Fatalf("UploadDiagnostics returned error: %v", err)
	}
	if diagResp.DiagnosticsId == "" {
		t.Fatal("UploadDiagnostics returned empty diagnostics id")
	}

	actionResp, err := adminSvc.RequestClientAction(context.Background(), &pb.RequestClientActionRequest{
		DeviceId:              "device-1",
		ActionType:            "resync",
		PayloadJson:           `{"scope":"full"}`,
		ReplaceSimilarPending: true,
	})
	if err != nil {
		t.Fatalf("RequestClientAction returned error: %v", err)
	}
	if actionResp.Action == nil || actionResp.Action.ActionId == "" {
		t.Fatal("RequestClientAction returned empty action")
	}

	heartbeatWithAction, err := controlSvc.Heartbeat(context.Background(), &pb.HeartbeatRequest{
		DeviceId:         "device-1",
		SessionId:        "session-2",
		TimestampUnix:    nowUnix + 1,
		OverlayConnected: true,
		GrpcConnected:    true,
		LastSyncTimeUnix: nowUnix,
		MountState:       "mounted",
		SyncStateSummary: "running",
	})
	if err != nil {
		t.Fatalf("Heartbeat(with action) returned error: %v", err)
	}
	if len(heartbeatWithAction.PendingActions) != 1 || heartbeatWithAction.PendingActions[0] != "resync" {
		t.Fatalf("Heartbeat PendingActions = %v, want [resync]", heartbeatWithAction.PendingActions)
	}
	if len(heartbeatWithAction.PendingActionDetails) != 1 {
		t.Fatalf("Heartbeat PendingActionDetails len = %d, want 1", len(heartbeatWithAction.PendingActionDetails))
	}
	if heartbeatWithAction.PendingActionDetails[0].Status != "delivered" {
		t.Fatalf("Heartbeat action status = %q, want delivered", heartbeatWithAction.PendingActionDetails[0].Status)
	}

	listResp, err := adminSvc.ListDevices(context.Background(), &pb.ListDevicesRequest{})
	if err != nil {
		t.Fatalf("ListDevices returned error: %v", err)
	}
	if len(listResp.Devices) != 1 {
		t.Fatalf("ListDevices returned %d devices, want 1", len(listResp.Devices))
	}

	device := listResp.Devices[0]
	if device.DeviceId != "device-1" {
		t.Fatalf("DeviceId = %q, want %q", device.DeviceId, "device-1")
	}
	if device.DisplayName != "Laptop" {
		t.Fatalf("DisplayName = %q, want %q", device.DisplayName, "Laptop")
	}
	if device.OnlineState != "online" {
		t.Fatalf("OnlineState = %q, want %q", device.OnlineState, "online")
	}
	if device.SyncState != "running" {
		t.Fatalf("SyncState = %q, want %q", device.SyncState, "running")
	}
	if device.MountState != "mounted" {
		t.Fatalf("MountState = %q, want %q", device.MountState, "mounted")
	}
	if device.PolicyRevision != 7 {
		t.Fatalf("PolicyRevision = %d, want %d", device.PolicyRevision, 7)
	}

	detailResp, err := adminSvc.GetDeviceDetail(context.Background(), &pb.GetDeviceDetailRequest{
		DeviceId: "device-1",
	})
	if err != nil {
		t.Fatalf("GetDeviceDetail returned error: %v", err)
	}
	if !detailResp.Mounted {
		t.Fatal("GetDeviceDetail Mounted = false, want true")
	}
	if detailResp.MountPath != "/Volumes/Roodox" {
		t.Fatalf("GetDeviceDetail MountPath = %q, want %q", detailResp.MountPath, "/Volumes/Roodox")
	}
	if len(detailResp.PendingActions) != 1 || detailResp.PendingActions[0].ActionType != "resync" {
		t.Fatalf("GetDeviceDetail PendingActions = %v, want one resync action", detailResp.PendingActions)
	}
	if len(detailResp.RecentDiagnostics) != 1 || detailResp.RecentDiagnostics[0].DiagnosticsId != diagResp.DiagnosticsId {
		t.Fatalf("GetDeviceDetail RecentDiagnostics = %v, want diagnostics %q", detailResp.RecentDiagnostics, diagResp.DiagnosticsId)
	}
	if len(detailResp.AvailableActions) != 3 {
		t.Fatalf("GetDeviceDetail AvailableActions len = %d, want 3", len(detailResp.AvailableActions))
	}

	policyResp, err := adminSvc.UpdateDevicePolicy(context.Background(), &pb.UpdateDevicePolicyRequest{
		DeviceId:               "device-1",
		ExpectedPolicyRevision: 7,
		Policy: &pb.AssignedConfigPolicy{
			MountPath:          "/custom/mount",
			SyncRoots:          []string{"src", "docs"},
			ConflictPolicy:     "server_wins",
			ReadOnly:           true,
			AutoConnect:        false,
			BandwidthLimit:     1024,
			LogLevel:           "warn",
			LargeFileThreshold: 16 << 20,
		},
	})
	if err != nil {
		t.Fatalf("UpdateDevicePolicy returned error: %v", err)
	}
	if policyResp.PolicyRevision != 8 {
		t.Fatalf("UpdateDevicePolicy PolicyRevision = %d, want 8", policyResp.PolicyRevision)
	}

	recordAfterPolicy, err := registry.Get("device-1")
	if err != nil {
		t.Fatalf("registry.Get(after policy) returned error: %v", err)
	}
	if !recordAfterPolicy.RequiresPolicyPull {
		t.Fatal("RequiresPolicyPull = false after UpdateDevicePolicy, want true")
	}

	policyConfigResp, err := controlSvc.GetAssignedConfig(context.Background(), &pb.GetAssignedConfigRequest{
		DeviceId: "device-1",
	})
	if err != nil {
		t.Fatalf("GetAssignedConfig(after policy) returned error: %v", err)
	}
	if policyConfigResp.PolicyRevision != 8 {
		t.Fatalf("GetAssignedConfig(after policy) PolicyRevision = %d, want 8", policyConfigResp.PolicyRevision)
	}
	if policyConfigResp.MountPath != "/custom/mount" {
		t.Fatalf("GetAssignedConfig(after policy) MountPath = %q, want %q", policyConfigResp.MountPath, "/custom/mount")
	}
	if len(policyConfigResp.SyncRoots) != 2 {
		t.Fatalf("GetAssignedConfig(after policy) SyncRoots = %v, want 2 entries", policyConfigResp.SyncRoots)
	}

	resetResp, err := adminSvc.UpdateDevicePolicy(context.Background(), &pb.UpdateDevicePolicyRequest{
		DeviceId:               "device-1",
		ExpectedPolicyRevision: 8,
		ResetToDefault:         true,
	})
	if err != nil {
		t.Fatalf("UpdateDevicePolicy(reset) returned error: %v", err)
	}
	if resetResp.PolicyRevision != 9 {
		t.Fatalf("UpdateDevicePolicy(reset) PolicyRevision = %d, want 9", resetResp.PolicyRevision)
	}

	resetConfigResp, err := controlSvc.GetAssignedConfig(context.Background(), &pb.GetAssignedConfigRequest{
		DeviceId: "device-1",
	})
	if err != nil {
		t.Fatalf("GetAssignedConfig(after reset) returned error: %v", err)
	}
	if resetConfigResp.PolicyRevision != 9 {
		t.Fatalf("GetAssignedConfig(after reset) PolicyRevision = %d, want 9", resetConfigResp.PolicyRevision)
	}
	if resetConfigResp.ConflictPolicy != "manual" {
		t.Fatalf("GetAssignedConfig(after reset) ConflictPolicy = %q, want %q", resetConfigResp.ConflictPolicy, "manual")
	}

	bundleResp, err := adminSvc.IssueJoinBundle(context.Background(), &pb.IssueJoinBundleRequest{
		DeviceId:    "device-1",
		DeviceName:  "Laptop",
		DeviceRole:  "developer",
		DeviceGroup: "team-a",
	})
	if err != nil {
		t.Fatalf("IssueJoinBundle returned error: %v", err)
	}
	if bundleResp.Bundle == nil {
		t.Fatal("IssueJoinBundle returned nil bundle")
	}
	if bundleResp.Bundle.ServiceHost != "cp.roodox.internal" {
		t.Fatalf("IssueJoinBundle ServiceHost = %q, want %q", bundleResp.Bundle.ServiceHost, "cp.roodox.internal")
	}
	if bundleResp.BundleJson == "" {
		t.Fatal("IssueJoinBundle returned empty bundle_json")
	}
}

func TestControlPlaneServiceRejectsHeartbeatForUnknownDevice(t *testing.T) {
	rootDir := t.TempDir()

	database, err := db.Open(filepath.Join(rootDir, "control-plane-missing.db"))
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Sql.Close()

	registry, err := db.NewDeviceRegistry(database)
	if err != nil {
		t.Fatalf("db.NewDeviceRegistry returned error: %v", err)
	}

	controlSvc := NewControlPlaneService(registry, ControlPlaneConfig{})
	_, err = controlSvc.Heartbeat(context.Background(), &pb.HeartbeatRequest{
		DeviceId:         "missing-device",
		OverlayConnected: true,
		GrpcConnected:    true,
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("Heartbeat code = %v, want %v", status.Code(err), codes.NotFound)
	}
}

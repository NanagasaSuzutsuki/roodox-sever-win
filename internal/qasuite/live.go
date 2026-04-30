package qasuite

import (
	"context"
	"fmt"
	"strings"
	"time"

	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"

	pb "roodox_server/proto"
)

type LiveOptions struct {
	KeepArtifacts bool
}

func RunLive(ctx context.Context, rt Runtime, opts LiveOptions) error {
	runRoot := BuildRunRelRoot("live")
	buildUnit := JoinRunPath(runRoot, "build-unit")
	filePath := JoinRunPath(runRoot, "data.txt")
	deviceID := "qa-" + BuildRunID("live-device")
	buildID := "skipped"

	if !opts.KeepArtifacts {
		defer func() { _ = RemoveRunRoot(rt.RootDir, runRoot) }()
	}

	if err := EnsureCMakeBuildUnit(rt.RootDir, buildUnit, liveBuildUnitContents()); err != nil {
		return err
	}

	c, err := rt.Dial()
	if err != nil {
		return err
	}
	defer c.Close()

	fmt.Printf("[live] dial=%s tls=%t server_name=%q root=%s\n", rt.DialAddr, rt.TLSEnabled, rt.TLSServerName, rt.RootDir)

	opCtx, cancel := OpContext(ctx, 8*time.Second)
	healthResp, err := c.HealthCheck(opCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	if healthResp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		return fmt.Errorf("health status = %s, want SERVING", healthResp.GetStatus())
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	runtimeResp, err := c.GetServerRuntime(opCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("GetServerRuntime failed: %w", err)
	}
	if err := EnsureNonEmpty(runtimeResp.GetListenAddr(), "listen_addr"); err != nil {
		return err
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	obsResp, err := c.GetServerObservability(opCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("GetServerObservability failed: %w", err)
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	registerResp, err := c.RegisterDevice(opCtx, &pb.RegisterDeviceRequest{
		DeviceId:        deviceID,
		DeviceName:      "qa-live-client",
		DeviceRole:      "tester",
		ClientVersion:   "qa-live",
		Platform:        "windows",
		OverlayProvider: "local",
		OverlayAddress:  "127.0.0.1",
		Capabilities:    BuildCapabilitySet(rt),
		ServerId:        rt.ServerID,
		DeviceGroup:     "default",
	})
	cancel()
	if err != nil {
		return fmt.Errorf("RegisterDevice failed: %w", err)
	}
	if !registerResp.GetAccepted() {
		return fmt.Errorf("RegisterDevice was rejected")
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	cfgResp, err := c.GetAssignedConfig(opCtx, deviceID)
	cancel()
	if err != nil {
		return fmt.Errorf("GetAssignedConfig failed: %w", err)
	}
	if len(cfgResp.GetSyncRoots()) == 0 {
		return fmt.Errorf("GetAssignedConfig returned empty sync_roots")
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	if _, err := c.ReportSyncState(opCtx, &pb.ReportSyncStateRequest{
		DeviceId:         deviceID,
		CurrentTaskCount: 1,
		LastSuccessTime:  time.Now().Unix(),
		ConflictCount:    0,
		QueueDepth:       0,
		Summary:          "qa-live",
	}); err != nil {
		cancel()
		return fmt.Errorf("ReportSyncState failed: %w", err)
	}
	cancel()

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	heartbeatResp, err := c.Heartbeat(opCtx, &pb.HeartbeatRequest{
		DeviceId:         deviceID,
		SessionId:        fmt.Sprintf("qa-live-%d", time.Now().UnixNano()),
		TimestampUnix:    time.Now().Unix(),
		OverlayConnected: true,
		GrpcConnected:    true,
		LastSyncTimeUnix: time.Now().Unix(),
		MountState:       "unmounted",
		SyncStateSummary: "qa-live",
	})
	cancel()
	if err != nil {
		return fmt.Errorf("Heartbeat failed: %w", err)
	}
	if heartbeatResp.GetNextHeartbeatSeconds() == 0 {
		return fmt.Errorf("Heartbeat returned next_heartbeat_seconds=0")
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	firstWrite, err := c.WriteFile(opCtx, filePath, []byte("hello-live"), 0)
	cancel()
	if err != nil {
		return fmt.Errorf("WriteFile failed: %w", err)
	}
	if firstWrite.GetConflicted() {
		return fmt.Errorf("initial WriteFile unexpectedly conflicted")
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	rangeWrite, err := c.WriteFileRange(opCtx, filePath, int64(len("hello-live")), []byte("|tail|"))
	cancel()
	if err != nil {
		return fmt.Errorf("WriteFileRange failed: %w", err)
	}
	if rangeWrite.GetConflicted() {
		return fmt.Errorf("WriteFileRange unexpectedly conflicted")
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	data, err := c.ReadFile(opCtx, filePath)
	cancel()
	if err != nil {
		return fmt.Errorf("ReadFile failed: %w", err)
	}
	text := string(data)
	if !strings.Contains(text, "hello-live") || !strings.Contains(text, "|tail|") {
		return fmt.Errorf("ReadFile returned unexpected payload: %q", text)
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	lockResp, err := c.AcquireLock(opCtx, filePath, deviceID, 15*time.Second)
	cancel()
	if err != nil {
		return fmt.Errorf("AcquireLock failed: %w", err)
	}
	if !lockResp.GetOk() {
		return fmt.Errorf("AcquireLock rejected: owner=%q", lockResp.GetOwner())
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	if _, err := c.ReleaseLock(opCtx, filePath, deviceID); err != nil {
		cancel()
		return fmt.Errorf("ReleaseLock failed: %w", err)
	}
	cancel()

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	history, err := c.GetHistory(opCtx, filePath)
	cancel()
	if err != nil {
		return fmt.Errorf("GetHistory failed: %w", err)
	}
	if len(history) < 2 {
		return fmt.Errorf("GetHistory returned %d records, want at least 2", len(history))
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	versionData, err := c.GetVersion(opCtx, filePath, history[len(history)-1].GetVersion())
	cancel()
	if err != nil {
		return fmt.Errorf("GetVersion failed: %w", err)
	}
	if len(versionData) == 0 {
		return fmt.Errorf("GetVersion returned empty data")
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	entries, err := c.ListDir(opCtx, runRoot)
	cancel()
	if err != nil {
		return fmt.Errorf("ListDir failed: %w", err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("ListDir returned no entries for %q", runRoot)
	}

	if rt.RemoteBuildEnabled {
		opCtx, cancel = OpContext(ctx, 8*time.Second)
		buildResp, err := c.StartBuild(opCtx, buildUnit, "smoke")
		cancel()
		if err != nil {
			return fmt.Errorf("StartBuild failed: %w", err)
		}
		buildID = buildResp.GetBuildId()
		buildStatus, buildLog, err := WaitBuildTerminal(ctx, c, buildResp.GetBuildId(), 30*time.Second)
		if err != nil {
			return fmt.Errorf("WaitBuildTerminal failed: %w", err)
		}
		if buildStatus.GetStatus() != "success" {
			return fmt.Errorf("build %q ended with status=%q error=%q", buildResp.GetBuildId(), buildStatus.GetStatus(), buildStatus.GetError())
		}
		if strings.TrimSpace(buildLog.GetText()) == "" {
			return fmt.Errorf("FetchBuildLog returned empty log")
		}
	} else {
		fmt.Printf("[live] skip build validation because remote build is disabled by config\n")
	}

	opCtx, cancel = OpContext(ctx, 8*time.Second)
	devices, err := c.ListDevices(opCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("ListDevices failed: %w", err)
	}
	foundDevice := false
	for _, device := range devices {
		if device.GetDeviceId() == deviceID {
			foundDevice = true
			break
		}
	}
	if !foundDevice {
		return fmt.Errorf("ListDevices did not include registered device %q", deviceID)
	}

	opCtx, cancel = OpContext(ctx, 12*time.Second)
	backupResp, err := c.TriggerServerBackup(opCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("TriggerServerBackup failed: %w", err)
	}
	if err := EnsureNonEmpty(backupResp.GetPath(), "backup path"); err != nil {
		return err
	}

	fmt.Printf("[live] ok device=%s build=%s backup=%s write_calls=%d\n", deviceID, buildID, backupResp.GetPath(), obsResp.GetWriteFileRangeCalls())
	return nil
}

func liveBuildUnitContents() string {
	return "cmake_minimum_required(VERSION 3.20)\nproject(RoodoxLiveQA NONE)\nadd_custom_target(smoke\n  COMMAND ${CMAKE_COMMAND} -E echo live-qa > artifact.txt\n  BYPRODUCTS artifact.txt\n  VERBATIM\n)\n"
}

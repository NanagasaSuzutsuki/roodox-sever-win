package serverapp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"roodox_server/client"
	"roodox_server/internal/appconfig"
	"roodox_server/internal/server"
	pb "roodox_server/proto"
)

func TestStartRejectsSecondInstanceForSameDB(t *testing.T) {
	cfg := testConfig(t)

	rt, err := Start(cfg)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer stopRuntime(t, rt)

	cfg2 := cfg
	cfg2.Addr = "127.0.0.1:0"
	if _, err := Start(cfg2); err == nil {
		t.Fatal("second Start unexpectedly succeeded with same db_path")
	}
}

func TestStartRejectsSecondInstanceForSameRuntimeStateDir(t *testing.T) {
	cfg := testConfig(t)

	rt, err := Start(cfg)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer stopRuntime(t, rt)

	cfg2 := testConfig(t)
	cfg2.Addr = "127.0.0.1:0"
	cfg2.Runtime = cfg.Runtime
	if _, err := Start(cfg2); err == nil {
		t.Fatal("second Start unexpectedly succeeded with same runtime.state_dir")
	}
}

func TestStartRejectsInsecureAuthOnNonLoopback(t *testing.T) {
	cfg := testConfig(t)
	cfg.Addr = ":0"

	if _, err := Start(cfg); err == nil {
		t.Fatal("Start unexpectedly succeeded with insecure auth on wildcard bind")
	}
}

func TestRuntimeRestartRecovery(t *testing.T) {
	cfg := testConfig(t)

	rt, err := Start(cfg)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	addr := rt.lis.Addr().String()
	c := newClient(t, addr, cfg.SharedSecret)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.WriteFile(ctx, "restart/data.txt", []byte("before-restart"), 0); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	start := time.Now()
	rt.Stop()
	if err := rt.Wait(); err != nil {
		t.Fatalf("Wait after Stop returned error: %v", err)
	}

	requestStart := time.Now()
	failCtx, failCancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer failCancel()
	_, err = c.ListDir(failCtx, ".")
	requireUnavailable(t, err)
	if elapsed := time.Since(requestStart); elapsed > time.Second {
		t.Fatalf("request during restart took too long: %v", elapsed)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("stop path took too long: %v", elapsed)
	}

	cfg.Addr = addr
	rt2, err := Start(cfg)
	if err != nil {
		t.Fatalf("restart Start returned error: %v", err)
	}
	defer stopRuntime(t, rt2)

	c2 := newClient(t, addr, cfg.SharedSecret)
	defer c2.Close()

	recoverCtx, recoverCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer recoverCancel()
	history, err := c2.GetHistory(recoverCtx, "restart/data.txt")
	if err != nil {
		t.Fatalf("GetHistory after restart returned error: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history length after restart = %d, want 1", len(history))
	}
}

func TestRuntimeHealthAndAdminEndpoints(t *testing.T) {
	cfg := testConfig(t)
	cfg.ControlPlane.ServerID = "srv-test"
	cfg.Database.CheckpointMode = "truncate"
	cfg.Database.BackupDir = filepath.Join(filepath.Dir(cfg.DBPath), "backups")
	cfg.Database.BackupIntervalSeconds = 3600
	cfg.Database.BackupKeepLatest = 2

	rt, err := Start(cfg)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer stopRuntime(t, rt)

	addr := rt.lis.Addr().String()
	c := newClient(t, addr, cfg.SharedSecret)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	healthResp, err := c.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck returned error: %v", err)
	}
	if healthResp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("HealthCheck status = %v, want %v", healthResp.GetStatus(), grpc_health_v1.HealthCheckResponse_SERVING)
	}

	if _, err := c.WriteFileRange(ctx, "runtime/metrics.txt", 0, []byte("hello-runtime")); err != nil {
		t.Fatalf("WriteFileRange returned error: %v", err)
	}

	runtimeResp, err := c.GetServerRuntime(ctx)
	if err != nil {
		t.Fatalf("GetServerRuntime returned error: %v", err)
	}
	if runtimeResp.GetServerId() != "srv-test" {
		t.Fatalf("server_id = %q, want %q", runtimeResp.GetServerId(), "srv-test")
	}
	if runtimeResp.GetListenAddr() != addr {
		t.Fatalf("listen_addr = %q, want %q", runtimeResp.GetListenAddr(), addr)
	}
	if runtimeResp.GetHealthState() != "serving" {
		t.Fatalf("health_state = %q, want %q", runtimeResp.GetHealthState(), "serving")
	}
	if runtimeResp.GetCheckpoint().GetMode() != "truncate" {
		t.Fatalf("checkpoint.mode = %q, want %q", runtimeResp.GetCheckpoint().GetMode(), "truncate")
	}
	if runtimeResp.GetBackup().GetDir() != cfg.Database.BackupDir {
		t.Fatalf("backup.dir = %q, want %q", runtimeResp.GetBackup().GetDir(), cfg.Database.BackupDir)
	}

	obsResp, err := c.GetServerObservability(ctx)
	if err != nil {
		t.Fatalf("GetServerObservability returned error: %v", err)
	}
	if obsResp.GetWriteFileRangeCalls() < 1 {
		t.Fatalf("write_file_range_calls = %d, want >= 1", obsResp.GetWriteFileRangeCalls())
	}

	backupResp, err := c.TriggerServerBackup(ctx)
	if err != nil {
		t.Fatalf("TriggerServerBackup returned error: %v", err)
	}
	if backupResp.GetPath() == "" {
		t.Fatal("TriggerServerBackup returned empty path")
	}
	if _, err := os.Stat(backupResp.GetPath()); err != nil {
		t.Fatalf("backup file %q stat returned error: %v", backupResp.GetPath(), err)
	}

	runtimeResp, err = c.GetServerRuntime(ctx)
	if err != nil {
		t.Fatalf("GetServerRuntime after backup returned error: %v", err)
	}
	if runtimeResp.GetBackup().GetLastBackupPath() != backupResp.GetPath() {
		t.Fatalf("last_backup_path = %q, want %q", runtimeResp.GetBackup().GetLastBackupPath(), backupResp.GetPath())
	}
	if runtimeResp.GetBackup().GetKeepLatest() != uint32(cfg.Database.BackupKeepLatest) {
		t.Fatalf("backup.keep_latest = %d, want %d", runtimeResp.GetBackup().GetKeepLatest(), cfg.Database.BackupKeepLatest)
	}
}

func TestRuntimeShutdownViaAdminEndpoint(t *testing.T) {
	cfg := testConfig(t)

	rt, err := Start(cfg)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	addr := rt.lis.Addr().String()
	c := newClient(t, addr, cfg.SharedSecret)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := c.ShutdownServer(ctx, "test shutdown")
	if err != nil {
		t.Fatalf("ShutdownServer returned error: %v", err)
	}
	if !resp.GetAccepted() {
		t.Fatalf("ShutdownServer accepted = %t, want true", resp.GetAccepted())
	}

	waitDeadline := time.After(5 * time.Second)
	select {
	case err := <-rt.doneCh:
		if err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	case <-waitDeadline:
		t.Fatal("runtime did not stop after shutdown request")
	}
}

func TestRuntimeStabilitySoak(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping soak test in short mode")
	}
	ensureWorkingCMakeBuildEnv(t)

	cfg := buildTestConfig(t)
	projectDir := filepath.Join(cfg.RootDir, "smoke_build")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	cmake := "cmake_minimum_required(VERSION 3.20)\nproject(RoodoxSoak NONE)\nadd_custom_target(smoke ALL\n  COMMAND ${CMAKE_COMMAND} -E sleep 1\n  COMMAND ${CMAKE_COMMAND} -E echo ok > artifact.txt\n  BYPRODUCTS artifact.txt\n  VERBATIM\n)\n"
	if err := os.WriteFile(filepath.Join(projectDir, "CMakeLists.txt"), []byte(cmake), 0o644); err != nil {
		t.Fatalf("WriteFile(CMakeLists.txt) returned error: %v", err)
	}

	rt, err := Start(cfg)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer stopRuntime(t, rt)

	addr := rt.lis.Addr().String()
	c := newClient(t, addr, cfg.SharedSecret)
	defer c.Close()

	deadline := time.Now().Add(3 * time.Second)
	errCh := make(chan error, 32)
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			index := int64(worker)
			for time.Now().Before(deadline) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_, err := c.WriteFileRange(ctx, "soak/hot.txt", index%16, []byte{byte('a' + worker)})
				cancel()
				if err != nil {
					errCh <- err
					return
				}
				index++
				time.Sleep(20 * time.Millisecond)
			}
		}(worker)
	}

	buildIDs := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		resp, err := c.StartBuild(ctx, "smoke_build", "smoke")
		cancel()
		if err != nil {
			t.Fatalf("StartBuild returned error: %v", err)
		}
		buildIDs = append(buildIDs, resp.BuildId)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("soak write returned error: %v", err)
		}
	}

	seenQueued := false
	pollDeadline := time.Now().Add(15 * time.Second)
	done := map[string]bool{}
	for time.Now().Before(pollDeadline) {
		allDone := true
		for _, buildID := range buildIDs {
			if done[buildID] {
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			statusResp, err := c.GetBuildStatus(ctx, buildID)
			cancel()
			if err != nil {
				t.Fatalf("GetBuildStatus(%s) returned error: %v", buildID, err)
			}
			if statusResp.Status == "queued" {
				seenQueued = true
			}
			switch statusResp.Status {
			case "success":
				done[buildID] = true
			case "failed":
				t.Fatalf("build %s failed: %s", buildID, statusResp.Error)
			default:
				allDone = false
			}
		}
		if allDone {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if len(done) != len(buildIDs) {
		t.Fatalf("not all builds reached terminal state, done=%d want=%d", len(done), len(buildIDs))
	}
	if !seenQueued {
		t.Fatal("expected at least one build to enter queued state")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	history, err := c.GetHistory(ctx, "soak/hot.txt")
	if err != nil {
		t.Fatalf("GetHistory returned error: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("expected soak writes to create version history")
	}
}

func TestBuildServiceCleansUpFinishedJobs(t *testing.T) {
	rootDir := t.TempDir()
	buildTempRoot := filepath.Join(t.TempDir(), "roodox-builds")

	svc := server.NewBuildService(server.BuildConfig{
		RootDir:         rootDir,
		RemoteEnabled:   true,
		MaxWorkers:      1,
		JobTTL:          50 * time.Millisecond,
		CleanupInterval: 10 * time.Millisecond,
		MaxRetainedJobs: 200,
		TempRoot:        buildTempRoot,
		RunBuild: func(unitAbs, buildRoot, target string, appendLog func(string, ...any)) (string, error) {
			if err := os.MkdirAll(buildRoot, 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(filepath.Join(buildRoot, "marker.txt"), []byte("ok"), 0o644); err != nil {
				return "", err
			}
			appendLog("stub build finished\n")
			return "", nil
		},
	})
	defer svc.Close()

	resp, err := svc.StartBuild(context.Background(), &pb.StartBuildRequest{
		UnitPath: "cleanup_build",
	})
	if err != nil {
		t.Fatalf("StartBuild returned error: %v", err)
	}

	statusResp := waitForBuildDone(t, svc, resp.BuildId, 5*time.Second)
	if statusResp.Status != "success" {
		t.Fatalf("final status = %q, want success", statusResp.Status)
	}

	buildDir := filepath.Join(buildTempRoot, resp.BuildId)
	if _, err := os.Stat(buildDir); err != nil {
		t.Fatalf("expected build work dir %q to exist before cleanup: %v", buildDir, err)
	}

	waitForBuildRemoval(t, svc, resp.BuildId, 5*time.Second)
	if _, err := os.Stat(buildDir); !os.IsNotExist(err) {
		t.Fatalf("expected build work dir %q to be removed, stat err=%v", buildDir, err)
	}
}

func TestBuildServiceQueueWaitUsesQueuedAt(t *testing.T) {
	rootDir := t.TempDir()
	buildTempRoot := filepath.Join(t.TempDir(), "roodox-builds")
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})

	var mu sync.Mutex
	callCount := 0
	svc := server.NewBuildService(server.BuildConfig{
		RootDir:         rootDir,
		RemoteEnabled:   true,
		MaxWorkers:      2,
		JobTTL:          time.Minute,
		CleanupInterval: time.Minute,
		MaxRetainedJobs: 200,
		TempRoot:        buildTempRoot,
		RunBuild: func(unitAbs, buildRoot, target string, appendLog func(string, ...any)) (string, error) {
			if err := os.MkdirAll(buildRoot, 0o755); err != nil {
				return "", err
			}

			mu.Lock()
			callCount++
			n := callCount
			mu.Unlock()

			if n == 1 {
				appendLog("first stub build waiting\n")
				close(firstStarted)
				<-releaseFirst
			}
			appendLog("stub build done\n")
			return "", nil
		},
	})
	defer svc.Close()

	first, err := svc.StartBuild(context.Background(), &pb.StartBuildRequest{
		UnitPath: "queue_build",
	})
	if err != nil {
		t.Fatalf("first StartBuild returned error: %v", err)
	}
	<-firstStarted

	second, err := svc.StartBuild(context.Background(), &pb.StartBuildRequest{
		UnitPath: "queue_build",
	})
	if err != nil {
		t.Fatalf("second StartBuild returned error: %v", err)
	}

	time.Sleep(200 * time.Millisecond)
	close(releaseFirst)

	waitForBuildDone(t, svc, first.BuildId, 5*time.Second)
	waitForBuildDone(t, svc, second.BuildId, 5*time.Second)

	logResp, err := svc.FetchBuildLog(context.Background(), &pb.FetchBuildLogRequest{BuildId: second.BuildId})
	if err != nil {
		t.Fatalf("FetchBuildLog returned error: %v", err)
	}

	queueWait := parseQueuedDuration(t, logResp.Text)
	if queueWait < 180*time.Millisecond {
		t.Fatalf("queued duration = %v, want at least %v; log=%q", queueWait, 180*time.Millisecond, logResp.Text)
	}
}
func testConfig(t *testing.T) appconfig.Config {
	t.Helper()

	base := t.TempDir()
	rootDir := filepath.Join(base, "share")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(rootDir) returned error: %v", err)
	}

	return appconfig.Config{
		Addr:    "127.0.0.1:0",
		RootDir: rootDir,
		DBPath:  filepath.Join(base, "roodox.db"),
		Runtime: appconfig.RuntimeConfig{
			BinaryPath:    filepath.Join(base, "roodox_server.exe"),
			StateDir:      filepath.Join(base, "runtime"),
			PIDFile:       filepath.Join(base, "runtime", "roodox_server.pid"),
			LogDir:        filepath.Join(base, "runtime", "logs"),
			StdoutLogName: "server.stdout.log",
			StderrLogName: "server.stderr.log",
		},
		RemoteBuildEnabled: false,
		AuthEnabled:        true,
		SharedSecret:       "secret-123",
	}
}

func buildTestConfig(t *testing.T) appconfig.Config {
	t.Helper()

	cfg := testConfig(t)
	cmakePath, err := exec.LookPath("cmake")
	if err != nil {
		t.Fatalf("LookPath(cmake) returned error: %v", err)
	}
	cfg.RemoteBuildEnabled = true
	cfg.BuildToolDirs = []string{filepath.Dir(cmakePath)}
	cfg.RequiredBuildTools = []string{"cmake"}
	return cfg
}

func ensureWorkingCMakeBuildEnv(t *testing.T) {
	t.Helper()

	cmakePath, err := exec.LookPath("cmake")
	if err != nil {
		t.Skip("cmake not available")
	}

	root := t.TempDir()
	srcDir := filepath.Join(root, "src")
	buildDir := filepath.Join(root, "build")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(srcDir) returned error: %v", err)
	}

	cmakeLists := "cmake_minimum_required(VERSION 3.20)\nproject(RoodoxEnvProbe NONE)\nadd_custom_target(smoke ALL COMMAND  -E echo ok > artifact.txt BYPRODUCTS artifact.txt VERBATIM)\n"
	if err := os.WriteFile(filepath.Join(srcDir, "CMakeLists.txt"), []byte(cmakeLists), 0o644); err != nil {
		t.Fatalf("WriteFile(CMakeLists.txt) returned error: %v", err)
	}

	configureCmd := exec.Command(cmakePath, "-S", srcDir, "-B", buildDir)
	configureOut, err := configureCmd.CombinedOutput()
	if err != nil {
		t.Skipf("skipping soak test: cmake configure probe failed: %v\n%s", err, trimProbeOutput(string(configureOut)))
	}

	buildCmd := exec.Command(cmakePath, "--build", buildDir)
	buildOut, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Skipf("skipping soak test: cmake build probe failed: %v\n%s", err, trimProbeOutput(string(buildOut)))
	}
}

func trimProbeOutput(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= 600 {
		return output
	}
	return output[:600] + "..."
}

func newClient(t *testing.T, addr, secret string) *client.RoodoxClient {
	t.Helper()

	c, err := client.NewRoodoxClientWithOptions(addr, client.ConnectionOptions{
		SharedSecret: secret,
	})
	if err != nil {
		t.Fatalf("NewRoodoxClientWithOptions returned error: %v", err)
	}
	return c
}

func stopRuntime(t *testing.T, rt *Runtime) {
	t.Helper()
	if rt == nil {
		return
	}
	rt.Stop()
	if err := rt.Wait(); err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
}

func requireUnavailable(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected unavailable error")
	}
	code := status.Code(err)
	if code != codes.Unavailable && code != codes.DeadlineExceeded {
		t.Fatalf("unexpected error code: %v", code)
	}
}

func writeCMakeBuildProject(t *testing.T, rootDir, relPath string, sleepSeconds int) {
	t.Helper()

	projectDir := filepath.Join(rootDir, relPath)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	cmake := fmt.Sprintf(
		"cmake_minimum_required(VERSION 3.20)\nproject(RoodoxBuildTest NONE)\nadd_custom_target(smoke\n  COMMAND ${CMAKE_COMMAND} -E sleep %d\n  COMMAND ${CMAKE_COMMAND} -E touch artifact.txt\n  BYPRODUCTS artifact.txt\n  VERBATIM\n)\n",
		sleepSeconds,
	)
	if err := os.WriteFile(filepath.Join(projectDir, "CMakeLists.txt"), []byte(cmake), 0o644); err != nil {
		t.Fatalf("WriteFile(CMakeLists.txt) returned error: %v", err)
	}
}

func waitForBuildDone(t *testing.T, svc *server.BuildService, buildID string, timeout time.Duration) *pb.GetBuildStatusResponse {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := svc.GetBuildStatus(context.Background(), &pb.GetBuildStatusRequest{BuildId: buildID})
		if err != nil {
			t.Fatalf("GetBuildStatus(%s) returned error: %v", buildID, err)
		}
		switch resp.Status {
		case "success":
			return resp
		case "failed":
			t.Fatalf("build %s failed: %s", buildID, resp.Error)
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("build %s did not finish within %v", buildID, timeout)
	return nil
}

func waitForBuildRemoval(t *testing.T, svc *server.BuildService, buildID string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := svc.GetBuildStatus(context.Background(), &pb.GetBuildStatusRequest{BuildId: buildID})
		if status.Code(err) == codes.NotFound {
			return
		}
		if err != nil {
			t.Fatalf("GetBuildStatus(%s) returned unexpected error: %v", buildID, err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("build %s was not cleaned up within %v", buildID, timeout)
}

func parseQueuedDuration(t *testing.T, logText string) time.Duration {
	t.Helper()

	for _, line := range strings.Split(logText, "\n") {
		if !strings.Contains(line, "queued for ") || !strings.Contains(line, " before starting") {
			continue
		}

		start := strings.Index(line, "queued for ")
		if start < 0 {
			continue
		}
		start += len("queued for ")

		end := strings.Index(line[start:], " before starting")
		if end < 0 {
			continue
		}

		durText := line[start : start+end]
		dur, err := time.ParseDuration(durText)
		if err != nil {
			t.Fatalf("ParseDuration(%q) returned error: %v", durText, err)
		}
		return dur
	}

	t.Fatalf("build log did not contain queued duration: %q", logText)
	return 0
}

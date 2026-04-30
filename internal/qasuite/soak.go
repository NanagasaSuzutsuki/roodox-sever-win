package qasuite

import (
	"context"
	"fmt"
	"math/rand"
	pathpkg "path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pb "roodox_server/proto"
)

type SoakOptions struct {
	Duration      time.Duration
	Workers       int
	BuildInterval time.Duration
	BackupOnce    bool
	KeepArtifacts bool
}

func RunSoak(ctx context.Context, rt Runtime, opts SoakOptions) error {
	if opts.Duration <= 0 {
		opts.Duration = 2 * time.Minute
	}
	if opts.Workers <= 0 {
		opts.Workers = 4
	}
	if opts.BuildInterval < 0 {
		opts.BuildInterval = 0
	}
	if !rt.RemoteBuildEnabled && opts.BuildInterval > 0 {
		fmt.Printf("[soak] remote build disabled by config, forcing build interval to 0\n")
		opts.BuildInterval = 0
	}

	runRoot := BuildRunRelRoot("soak")
	buildUnit := JoinRunPath(runRoot, "build-unit")
	deviceID := "qa-" + BuildRunID("soak-device")

	if !opts.KeepArtifacts {
		defer func() { _ = RemoveRunRoot(rt.RootDir, runRoot) }()
	}

	if err := EnsureCMakeBuildUnit(rt.RootDir, buildUnit, soakBuildUnitContents()); err != nil {
		return err
	}

	c, err := rt.Dial()
	if err != nil {
		return err
	}
	defer c.Close()

	opCtx, cancel := OpContext(ctx, 8*time.Second)
	if _, err := c.RegisterDevice(opCtx, &pb.RegisterDeviceRequest{
		DeviceId:        deviceID,
		DeviceName:      "qa-soak-client",
		DeviceRole:      "tester",
		ClientVersion:   "qa-soak",
		Platform:        "windows",
		OverlayProvider: "local",
		OverlayAddress:  "127.0.0.1",
		Capabilities:    BuildCapabilitySet(rt),
		ServerId:        rt.ServerID,
		DeviceGroup:     "default",
	}); err != nil {
		cancel()
		return fmt.Errorf("RegisterDevice failed: %w", err)
	}
	cancel()

	runCtx, stop := context.WithCancel(ctx)
	defer stop()

	var (
		writeOps     atomic.Int64
		readOps      atomic.Int64
		historyOps   atomic.Int64
		lockOps      atomic.Int64
		heartbeatOps atomic.Int64
		syncOps      atomic.Int64
		adminOps     atomic.Int64
		buildOps     atomic.Int64
		buildOK      atomic.Int64
	)

	var (
		firstErr error
		once     sync.Once
	)
	fail := func(err error) {
		if err == nil {
			return
		}
		once.Do(func() {
			firstErr = err
			stop()
		})
	}

	deadline := time.Now().Add(opts.Duration)
	op := func(timeout time.Duration, fn func(context.Context) error) {
		if runCtx.Err() != nil || time.Now().After(deadline) {
			return
		}
		opCtx, cancel := OpContext(runCtx, timeout)
		err := fn(opCtx)
		cancel()
		fail(err)
	}

	var wg sync.WaitGroup

	for worker := 0; worker < opts.Workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(worker)))
			seq := 0
			filePath := pathpkg.Join(runRoot, fmt.Sprintf("worker-%d.txt", worker))
			lockPath := pathpkg.Join(runRoot, fmt.Sprintf("lock-%d.txt", worker))
			for runCtx.Err() == nil && time.Now().Before(deadline) {
				payload := fmt.Sprintf("worker=%d seq=%d at=%d", worker, seq, time.Now().UnixNano())
				op(8*time.Second, func(ctx context.Context) error {
					resp, err := c.WriteFile(ctx, filePath, []byte(payload), 0)
					if err != nil {
						return fmt.Errorf("WriteFile(%s) failed: %w", filePath, err)
					}
					if resp.GetConflicted() {
						return fmt.Errorf("WriteFile(%s) unexpectedly conflicted", filePath)
					}
					writeOps.Add(1)
					return nil
				})
				op(8*time.Second, func(ctx context.Context) error {
					resp, err := c.WriteFileRange(ctx, filePath, int64(len(payload)), []byte(fmt.Sprintf("|tail=%02d|", seq%100)))
					if err != nil {
						return fmt.Errorf("WriteFileRange(%s) failed: %w", filePath, err)
					}
					if resp.GetConflicted() {
						return fmt.Errorf("WriteFileRange(%s) unexpectedly conflicted", filePath)
					}
					writeOps.Add(1)
					return nil
				})
				op(8*time.Second, func(ctx context.Context) error {
					data, err := c.ReadFile(ctx, filePath)
					if err != nil {
						return fmt.Errorf("ReadFile(%s) failed: %w", filePath, err)
					}
					text := string(data)
					if !strings.Contains(text, fmt.Sprintf("worker=%d", worker)) || !strings.Contains(text, "|tail=") {
						return fmt.Errorf("ReadFile(%s) returned unexpected payload: %q", filePath, text)
					}
					readOps.Add(1)
					return nil
				})
				if seq%5 == 0 {
					op(8*time.Second, func(ctx context.Context) error {
						history, err := c.GetHistory(ctx, filePath)
						if err != nil {
							return fmt.Errorf("GetHistory(%s) failed: %w", filePath, err)
						}
						if len(history) == 0 {
							return fmt.Errorf("GetHistory(%s) returned empty history", filePath)
						}
						historyOps.Add(1)
						return nil
					})
				}
				if seq%7 == 0 {
					op(8*time.Second, func(ctx context.Context) error {
						lockResp, err := c.AcquireLock(ctx, lockPath, deviceID, 10*time.Second)
						if err != nil {
							return fmt.Errorf("AcquireLock(%s) failed: %w", lockPath, err)
						}
						if !lockResp.GetOk() {
							return fmt.Errorf("AcquireLock(%s) rejected owner=%q", lockPath, lockResp.GetOwner())
						}
						lockOps.Add(1)
						return nil
					})
					op(8*time.Second, func(ctx context.Context) error {
						if _, err := c.ReleaseLock(ctx, lockPath, deviceID); err != nil {
							return fmt.Errorf("ReleaseLock(%s) failed: %w", lockPath, err)
						}
						lockOps.Add(1)
						return nil
					})
				}
				seq++
				time.Sleep(time.Duration(80+rng.Intn(120)) * time.Millisecond)
			}
		}(worker)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for runCtx.Err() == nil && time.Now().Before(deadline) {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				op(8*time.Second, func(ctx context.Context) error {
					if _, err := c.ReportSyncState(ctx, &pb.ReportSyncStateRequest{
						DeviceId:         deviceID,
						CurrentTaskCount: uint32(opts.Workers),
						LastSuccessTime:  time.Now().Unix(),
						ConflictCount:    0,
						QueueDepth:       0,
						Summary:          "qa-soak",
					}); err != nil {
						return fmt.Errorf("ReportSyncState failed: %w", err)
					}
					syncOps.Add(1)
					return nil
				})
				op(8*time.Second, func(ctx context.Context) error {
					if _, err := c.Heartbeat(ctx, &pb.HeartbeatRequest{
						DeviceId:         deviceID,
						SessionId:        fmt.Sprintf("qa-soak-%d", time.Now().UnixNano()),
						TimestampUnix:    time.Now().Unix(),
						OverlayConnected: true,
						GrpcConnected:    true,
						LastSyncTimeUnix: time.Now().Unix(),
						MountState:       "unmounted",
						SyncStateSummary: "qa-soak",
					}); err != nil {
						return fmt.Errorf("Heartbeat failed: %w", err)
					}
					heartbeatOps.Add(1)
					return nil
				})
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		backupIssued := !opts.BackupOnce
		for runCtx.Err() == nil && time.Now().Before(deadline) {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				op(8*time.Second, func(ctx context.Context) error {
					if _, err := c.GetServerRuntime(ctx); err != nil {
						return fmt.Errorf("GetServerRuntime failed: %w", err)
					}
					adminOps.Add(1)
					return nil
				})
				op(8*time.Second, func(ctx context.Context) error {
					if _, err := c.GetServerObservability(ctx); err != nil {
						return fmt.Errorf("GetServerObservability failed: %w", err)
					}
					adminOps.Add(1)
					return nil
				})
				op(8*time.Second, func(ctx context.Context) error {
					if _, err := c.ListDevices(ctx); err != nil {
						return fmt.Errorf("ListDevices failed: %w", err)
					}
					adminOps.Add(1)
					return nil
				})
				if !backupIssued {
					op(15*time.Second, func(ctx context.Context) error {
						if _, err := c.TriggerServerBackup(ctx); err != nil {
							return fmt.Errorf("TriggerServerBackup failed: %w", err)
						}
						adminOps.Add(1)
						backupIssued = true
						return nil
					})
				}
			}
		}
	}()

	if opts.BuildInterval > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(opts.BuildInterval)
			defer ticker.Stop()
			for runCtx.Err() == nil && time.Now().Before(deadline) {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
					var buildID string
					op(20*time.Second, func(ctx context.Context) error {
						resp, err := c.StartBuild(ctx, buildUnit, "smoke")
						if err != nil {
							return fmt.Errorf("StartBuild failed: %w", err)
						}
						buildID = resp.GetBuildId()
						buildOps.Add(1)
						return nil
					})
					if buildID == "" || runCtx.Err() != nil {
						continue
					}
					statusResp, logResp, err := WaitBuildTerminal(runCtx, c, buildID, 30*time.Second)
					if err != nil {
						fail(fmt.Errorf("WaitBuildTerminal(%s) failed: %w", buildID, err))
						continue
					}
					if statusResp.GetStatus() != "success" {
						fail(fmt.Errorf("build %q ended with status=%q error=%q log=%q", buildID, statusResp.GetStatus(), statusResp.GetError(), logResp.GetText()))
						continue
					}
					buildOK.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	fmt.Printf("[soak] writes=%d reads=%d history=%d locks=%d heartbeats=%d sync=%d admin=%d builds=%d build_ok=%d\n",
		writeOps.Load(),
		readOps.Load(),
		historyOps.Load(),
		lockOps.Load(),
		heartbeatOps.Load(),
		syncOps.Load(),
		adminOps.Load(),
		buildOps.Load(),
		buildOK.Load(),
	)

	if firstErr != nil {
		return firstErr
	}
	if opts.BuildInterval > 0 && buildOK.Load() == 0 {
		return fmt.Errorf("soak completed without a successful build")
	}
	return nil
}

func soakBuildUnitContents() string {
	return "cmake_minimum_required(VERSION 3.20)\nproject(RoodoxSoakQA NONE)\nadd_custom_target(smoke\n  COMMAND ${CMAKE_COMMAND} -E echo soak-qa > artifact.txt\n  BYPRODUCTS artifact.txt\n  VERBATIM\n)\n"
}

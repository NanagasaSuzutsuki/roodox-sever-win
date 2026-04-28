package serverapp

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"

	"roodox_server/internal/analyze"
	"roodox_server/internal/appconfig"
	"roodox_server/internal/db"
	"roodox_server/internal/fs"
	"roodox_server/internal/lock"
	"roodox_server/internal/observability"
	"roodox_server/internal/server"
	pb "roodox_server/proto"
)

type Runtime struct {
	grpcServer  *grpc.Server
	healthSrv   *health.Server
	lis         net.Listener
	serviceLock instanceLock
	dbLock      instanceLock
	database    *db.DB
	buildSvc    *server.BuildService
	metrics     *observability.Recorder
	dbMaint     *databaseMaintenanceManager
	janitors    []backgroundJanitor
	restoreLog  func()

	doneCh chan error

	mu          sync.RWMutex
	running     bool
	lastErr     string
	cfg         appconfig.Config
	startedAt   time.Time
	shutdownAt  time.Time
	shutdownMsg string
	cleanupOnce sync.Once
}

func Start(cfg appconfig.Config) (*Runtime, error) {
	if len(cfg.BuildToolDirs) > 0 {
		_ = os.Setenv("ROODOX_BUILD_TOOL_DIRS", strings.Join(cfg.BuildToolDirs, string(os.PathListSeparator)))
	}
	if len(cfg.RequiredBuildTools) > 0 {
		_ = os.Setenv("ROODOX_BUILD_REQUIRED_TOOLS", strings.Join(cfg.RequiredBuildTools, ","))
	}
	if err := validateInsecureAuthBinding(cfg); err != nil {
		return nil, err
	}

	startupCheck, err := server.RunStartupChecks(cfg.RootDir, cfg.RemoteBuildEnabled)
	if err != nil {
		return nil, err
	}
	log.Printf("startup check passed: user=%s, remoteBuildEnabled=%v", startupCheck.CurrentUser, cfg.RemoteBuildEnabled)

	dbPath := cfg.DBPath
	if strings.TrimSpace(dbPath) == "" {
		dbPath = "roodox.db"
	}
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Clean(dbPath)
	}

	stateDir := cfg.Runtime.StateDir
	if strings.TrimSpace(stateDir) == "" {
		stateDir = "runtime"
	}
	if !filepath.IsAbs(stateDir) {
		if cwd, err := os.Getwd(); err == nil {
			stateDir = filepath.Join(cwd, stateDir)
		} else {
			stateDir = filepath.Clean(stateDir)
		}
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("create runtime state dir failed: %w", err)
	}

	serviceLock, err := acquireInstanceLock(filepath.Join(stateDir, "service.instance"))
	if err != nil {
		return nil, fmt.Errorf("acquire runtime instance lock failed: %w", err)
	}

	dbLock, err := acquireInstanceLock(dbPath)
	if err != nil {
		_ = serviceLock.Release()
		return nil, fmt.Errorf("acquire db instance lock failed: %w", err)
	}

	fsys := fs.NewFileSystem(cfg.RootDir)

	database, err := db.Open(dbPath)
	if err != nil {
		_ = dbLock.Release()
		_ = serviceLock.Release()
		return nil, fmt.Errorf("open db failed: %w", err)
	}
	metaStore, err := db.NewMetaStore(database)
	if err != nil {
		_ = database.Sql.Close()
		_ = dbLock.Release()
		_ = serviceLock.Release()
		return nil, fmt.Errorf("init meta store failed: %w", err)
	}
	versionStore, err := db.NewVersionStore(database)
	if err != nil {
		_ = database.Sql.Close()
		_ = dbLock.Release()
		_ = serviceLock.Release()
		return nil, fmt.Errorf("init version store failed: %w", err)
	}
	deviceRegistry, err := db.NewDeviceRegistry(database)
	if err != nil {
		_ = database.Sql.Close()
		_ = dbLock.Release()
		_ = serviceLock.Release()
		return nil, fmt.Errorf("init device registry failed: %w", err)
	}

	lockMgr := lock.NewManager(30 * time.Second)
	pathLocker := server.NewPathLocker()
	analyzer := analyze.NewAnalyzer(cfg.RootDir)
	metrics := observability.NewRecorder(observability.DefaultConfig())

	coreSvc := server.NewCoreService(fsys, metaStore, versionStore, pathLocker)
	lockSvc := server.NewLockService(lockMgr)
	syncSvc := server.NewSyncService(fsys, metaStore, versionStore, pathLocker)
	coreSvc.ConfigureMetrics(metrics)
	syncSvc.ConfigureMetrics(metrics)
	conflictPolicy := server.ConflictCleanupPolicy{}
	if cfg.Cleanup.ConflictFiles.Enabled {
		conflictPolicy = server.ConflictCleanupPolicy{
			FileTTL:          time.Duration(cfg.Cleanup.ConflictFiles.RetentionSeconds) * time.Second,
			MaxCopiesPerPath: cfg.Cleanup.ConflictFiles.MaxCopiesPerPath,
			CleanupInterval:  time.Duration(cfg.Cleanup.ConflictFiles.IntervalSeconds) * time.Second,
		}
	}
	syncSvc.ConfigureConflictCleanup(conflictPolicy)
	versionSvc := server.NewVersionService(versionStore)
	analyzeSvc := server.NewAnalyzeService(analyzer)
	buildSvc := server.NewBuildService(server.BuildConfig{
		RootDir:          cfg.RootDir,
		RemoteEnabled:    cfg.RemoteBuildEnabled,
		JobTTL:           time.Duration(cfg.Cleanup.BuildWorkdirs.RetentionSeconds) * time.Second,
		CleanupInterval:  time.Duration(cfg.Cleanup.BuildWorkdirs.IntervalSeconds) * time.Second,
		MaxRetainedBytes: cfg.Cleanup.BuildWorkdirs.MaxBytes,
		Metrics:          metrics,
	})
	backupDir := strings.TrimSpace(cfg.Database.BackupDir)
	if backupDir != "" && !filepath.IsAbs(backupDir) {
		if cwd, err := os.Getwd(); err == nil {
			backupDir = filepath.Join(cwd, backupDir)
		}
	}
	dbMaint := newDatabaseMaintenanceManager(database, DatabaseMaintenanceRuntimeConfig{
		CheckpointInterval: time.Duration(cfg.Database.CheckpointIntervalSeconds) * time.Second,
		CheckpointMode:     cfg.Database.CheckpointMode,
		BackupDir:          backupDir,
		BackupInterval:     time.Duration(cfg.Database.BackupIntervalSeconds) * time.Second,
		BackupKeepLatest:   cfg.Database.BackupKeepLatest,
	})
	controlPlaneConfig := buildControlPlaneConfig(cfg)
	controlPlaneSvc := server.NewControlPlaneService(deviceRegistry, controlPlaneConfig)
	adminConsoleSvc := server.NewAdminConsoleService(deviceRegistry, controlPlaneConfig)
	janitors := make([]backgroundJanitor, 0, 3)
	var artifactJanitor *artifactJanitor
	if janitor := newArtifactJanitor(cfg.RootDir, ArtifactCleanupRuntimeConfig{
		Enabled:   cfg.Cleanup.TempArtifacts.Enabled,
		Interval:  time.Duration(cfg.Cleanup.TempArtifacts.IntervalSeconds) * time.Second,
		Retention: time.Duration(cfg.Cleanup.TempArtifacts.RetentionSeconds) * time.Second,
		MaxBytes:  cfg.Cleanup.TempArtifacts.MaxBytes,
		Prefixes:  cfg.Cleanup.TempArtifacts.Prefixes,
	}); janitor != nil {
		artifactJanitor = janitor
		janitors = append(janitors, janitor)
	}
	var conflictJanitor *conflictFileJanitor
	if janitor := newConflictFileJanitor(cfg.RootDir, ConflictFileCleanupRuntimeConfig{
		Enabled:   cfg.Cleanup.ConflictFiles.Enabled,
		Interval:  time.Duration(cfg.Cleanup.ConflictFiles.IntervalSeconds) * time.Second,
		Retention: time.Duration(cfg.Cleanup.ConflictFiles.RetentionSeconds) * time.Second,
		MaxBytes:  cfg.Cleanup.ConflictFiles.MaxBytes,
	}); janitor != nil {
		conflictJanitor = janitor
		janitors = append(janitors, janitor)
	}
	logDir := cfg.Cleanup.LogFiles.Dir
	if !filepath.IsAbs(logDir) {
		if cwd, err := os.Getwd(); err == nil {
			logDir = filepath.Join(cwd, logDir)
		}
	}
	var logJanitor *logFileJanitor
	if janitor := newLogFileJanitor(logDir, LogCleanupRuntimeConfig{
		Enabled:   cfg.Cleanup.LogFiles.Enabled,
		Interval:  time.Duration(cfg.Cleanup.LogFiles.IntervalSeconds) * time.Second,
		Retention: time.Duration(cfg.Cleanup.LogFiles.RetentionSeconds) * time.Second,
		MaxBytes:  cfg.Cleanup.LogFiles.MaxBytes,
		Patterns:  cfg.Cleanup.LogFiles.Patterns,
	}); janitor != nil {
		logJanitor = janitor
		janitors = append(janitors, janitor)
	}
	if reporter := newObservabilityReporter(metrics, defaultObservabilityReportInterval); reporter != nil {
		janitors = append(janitors, reporter)
	}
	if dbMaint != nil {
		janitors = append(janitors, dbMaint)
	}
	cleanupHooks := server.CleanupHooks{}
	if artifactJanitor != nil {
		cleanupHooks.OnMutation = artifactJanitor.Trigger
	}
	if conflictJanitor != nil {
		cleanupHooks.OnConflict = conflictJanitor.Trigger
	}
	coreSvc.ConfigureCleanupHooks(cleanupHooks)
	syncSvc.ConfigureCleanupHooks(cleanupHooks)
	restoreLog := func() {}
	if logJanitor != nil {
		restoreLog = installLogTrigger(logJanitor.Trigger)
	}

	grpcOpts, err := server.BuildGRPCServerOptions(server.SecurityConfig{
		AuthEnabled:  cfg.AuthEnabled,
		SharedSecret: cfg.SharedSecret,
		TLSEnabled:   cfg.TLSEnabled,
		TLSCertPath:  cfg.TLSCertPath,
		TLSKeyPath:   cfg.TLSKeyPath,
		Metrics:      metrics,
	})
	if err != nil {
		restoreLog()
		for _, janitor := range janitors {
			if janitor != nil {
				janitor.Close()
			}
		}
		_ = database.Sql.Close()
		_ = dbLock.Release()
		_ = serviceLock.Release()
		return nil, fmt.Errorf("configure grpc security failed: %w", err)
	}

	lis, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		restoreLog()
		for _, janitor := range janitors {
			if janitor != nil {
				janitor.Close()
			}
		}
		_ = database.Sql.Close()
		_ = dbLock.Release()
		_ = serviceLock.Release()
		return nil, fmt.Errorf("listen failed: %w", err)
	}

	grpcServer := grpc.NewServer(grpcOpts...)
	healthSrv := health.NewServer()
	pb.RegisterCoreServiceServer(grpcServer, coreSvc)
	pb.RegisterLockServiceServer(grpcServer, lockSvc)
	pb.RegisterSyncServiceServer(grpcServer, syncSvc)
	pb.RegisterVersionServiceServer(grpcServer, versionSvc)
	pb.RegisterAnalyzeServiceServer(grpcServer, analyzeSvc)
	pb.RegisterBuildServiceServer(grpcServer, buildSvc)
	pb.RegisterControlPlaneServiceServer(grpcServer, controlPlaneSvc)
	pb.RegisterAdminConsoleServiceServer(grpcServer, adminConsoleSvc)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthSrv)

	rt := &Runtime{
		grpcServer:  grpcServer,
		healthSrv:   healthSrv,
		lis:         lis,
		serviceLock: serviceLock,
		dbLock:      dbLock,
		database:    database,
		buildSvc:    buildSvc,
		metrics:     metrics,
		dbMaint:     dbMaint,
		janitors:    janitors,
		restoreLog:  restoreLog,
		doneCh:      make(chan error, 1),
		running:     true,
		cfg:         cfg,
		startedAt:   time.Now(),
	}
	adminConsoleSvc.ConfigureRuntime(rt)
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	log.Printf(
		"Roodox server listening on %s, rootDir=%s, remoteBuildEnabled=%v, authEnabled=%v, tlsEnabled=%v",
		cfg.Addr,
		cfg.RootDir,
		cfg.RemoteBuildEnabled,
		cfg.AuthEnabled,
		cfg.TLSEnabled,
	)
	go func() {
		err := grpcServer.Serve(lis)
		rt.cleanup()
		if err != nil && !isExpectedServerStop(err) {
			rt.setLastErr(err.Error())
			rt.doneCh <- err
		} else {
			rt.doneCh <- nil
		}
		close(rt.doneCh)
	}()
	return rt, nil
}

func (r *Runtime) Wait() error {
	return <-r.doneCh
}

func (r *Runtime) Stop() {
	if r.healthSrv != nil {
		r.healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	}
	r.mu.RLock()
	if !r.running {
		r.mu.RUnlock()
		return
	}
	r.mu.RUnlock()

	done := make(chan struct{})
	go func() {
		r.grpcServer.GracefulStop()
		close(done)
	}()

	timeout := r.stopTimeout()
	select {
	case <-done:
	case <-time.After(timeout):
		r.grpcServer.Stop()
	}
	_ = r.lis.Close()
}

func (r *Runtime) Status() (running bool, addr string, rootDir string, remote bool, lastErr string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running, r.cfg.Addr, r.cfg.RootDir, r.cfg.RemoteBuildEnabled, r.lastErr
}

func (r *Runtime) setRunning(v bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = v
}

func (r *Runtime) setLastErr(v string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastErr = v
}

func (r *Runtime) EnsureHealthy() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.running {
		if r.lastErr != "" {
			return errors.New(r.lastErr)
		}
		return errors.New("server is not running")
	}
	return nil
}

func (r *Runtime) cleanup() {
	r.cleanupOnce.Do(func() {
		if r.healthSrv != nil {
			r.healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
		}
		if r.lis != nil {
			_ = r.lis.Close()
		}
		if r.buildSvc != nil {
			r.buildSvc.Close()
		}
		if r.restoreLog != nil {
			r.restoreLog()
		}
		for _, janitor := range r.janitors {
			if janitor != nil {
				janitor.Close()
			}
		}
		if r.serviceLock != nil {
			if err := r.serviceLock.Release(); err != nil {
				log.Printf("release runtime instance lock failed: %v", err)
			}
		}
		if r.dbLock != nil {
			if err := r.dbLock.Release(); err != nil {
				log.Printf("release db instance lock failed: %v", err)
			}
		}
		if r.database != nil {
			if err := r.database.Sql.Close(); err != nil {
				log.Printf("close database failed: %v", err)
			}
		}
		r.setRunning(false)
	})
}

func (r *Runtime) stopTimeout() time.Duration {
	if r == nil {
		return 3 * time.Second
	}

	timeoutSeconds := r.cfg.Runtime.GracefulStopTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 3
	}
	return time.Duration(timeoutSeconds) * time.Second
}

func isExpectedServerStop(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, grpc.ErrServerStopped) || errors.Is(err, net.ErrClosed) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "use of closed network connection")
}

func validateInsecureAuthBinding(cfg appconfig.Config) error {
	if !cfg.AuthEnabled || cfg.TLSEnabled {
		return nil
	}
	if isLoopbackListenAddress(cfg.Addr) {
		return nil
	}
	return fmt.Errorf("auth without tls is only allowed on loopback addresses; enable tls or bind addr to 127.0.0.1/::1")
}

func isLoopbackListenAddress(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"roodox_server/client"
	"roodox_server/internal/appconfig"
	"roodox_server/internal/db"
	"roodox_server/internal/server"
	"roodox_server/internal/serverapp"
	pb "roodox_server/proto"
)

func main() {
	configPath := flag.String("config", appconfig.ConfigPath, "Path to the Roodox server config file")
	requestShutdown := flag.Bool("request-shutdown", false, "Request graceful shutdown of the running server described by the config and exit")
	shutdownReason := flag.String("shutdown-reason", "local admin request", "Reason recorded for a graceful shutdown request")
	restoreDBFrom := flag.String("restore-db-from", "", "Restore the configured SQLite database from the specified backup file and exit")
	restoreDBNoSafetyBackup := flag.Bool("restore-db-no-safety-backup", false, "Skip the automatic pre-restore safety backup copy")
	tlsStatus := flag.Bool("tls-status", false, "Print TLS certificate status as JSON and exit")
	rotateTLS := flag.Bool("rotate-tls", false, "Rotate the TLS server certificate and exit")
	rotateTLSRootCA := flag.Bool("rotate-tls-root-ca", false, "Rotate the TLS root CA together with the server certificate when used with -rotate-tls")
	tlsBackupDir := flag.String("tls-backup-dir", "", "Optional backup directory used by -rotate-tls")
	exportClientCA := flag.String("export-client-ca", "", "Copy the client trust root certificate to the specified path and exit")
	issueJoinBundleJSON := flag.Bool("issue-join-bundle-json", false, "Print a client join bundle as JSON and exit")
	joinDeviceID := flag.String("join-device-id", "", "Optional device id embedded in the issued join bundle")
	joinDeviceName := flag.String("join-device-name", "", "Optional device name embedded in the issued join bundle")
	joinDeviceRole := flag.String("join-device-role", "", "Optional device role embedded in the issued join bundle")
	joinDeviceGroup := flag.String("join-device-group", "", "Optional device group override embedded in the issued join bundle")
	workbenchSnapshotJSON := flag.Bool("workbench-snapshot-json", false, "Print a GUI-oriented admin snapshot as JSON and exit")
	workbenchObservabilityJSON := flag.Bool("workbench-observability-json", false, "Print a GUI-oriented observability snapshot as JSON and exit")
	triggerServerBackupJSON := flag.Bool("trigger-server-backup-json", false, "Trigger a server backup and print the result as JSON")
	serviceNameOverride := flag.String("service-name", "", "Override Windows service name when running under SCM")
	flag.Parse()

	resolvedConfigPath, err := filepath.Abs(*configPath)
	if err != nil {
		log.Fatalf("resolve config path failed: %v", err)
	}
	if err := os.Chdir(filepath.Dir(resolvedConfigPath)); err != nil {
		log.Fatalf("switch to config dir failed: %v", err)
	}

	cfg, err := appconfig.Load(resolvedConfigPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	if *requestShutdown {
		if err := requestServerShutdown(cfg, *shutdownReason); err != nil {
			log.Fatalf("request shutdown failed: %v", err)
		}
		return
	}
	if strings.TrimSpace(*restoreDBFrom) != "" {
		if err := restoreDatabase(cfg, *restoreDBFrom, !*restoreDBNoSafetyBackup); err != nil {
			log.Fatalf("restore database failed: %v", err)
		}
		return
	}
	if *tlsStatus {
		if err := printTLSStatus(cfg); err != nil {
			log.Fatalf("print tls status failed: %v", err)
		}
		return
	}
	if *rotateTLS {
		if err := rotateTLSArtifacts(cfg, *rotateTLSRootCA, *tlsBackupDir); err != nil {
			log.Fatalf("rotate tls artifacts failed: %v", err)
		}
		return
	}
	if strings.TrimSpace(*exportClientCA) != "" {
		if err := exportTLSRootCA(cfg, *exportClientCA); err != nil {
			log.Fatalf("export client ca failed: %v", err)
		}
		return
	}
	if *issueJoinBundleJSON {
		if err := issueJoinBundle(cfg, issueJoinBundleOptions{
			DeviceID:    *joinDeviceID,
			DeviceName:  *joinDeviceName,
			DeviceRole:  *joinDeviceRole,
			DeviceGroup: *joinDeviceGroup,
		}); err != nil {
			log.Fatalf("issue join bundle failed: %v", err)
		}
		return
	}
	if *workbenchSnapshotJSON {
		if err := printWorkbenchSnapshot(cfg); err != nil {
			log.Fatalf("print workbench snapshot failed: %v", err)
		}
		return
	}
	if *workbenchObservabilityJSON {
		if err := printWorkbenchObservability(cfg); err != nil {
			log.Fatalf("print workbench observability failed: %v", err)
		}
		return
	}
	if *triggerServerBackupJSON {
		if err := triggerServerBackup(cfg); err != nil {
			log.Fatalf("trigger server backup failed: %v", err)
		}
		return
	}

	serviceName := strings.TrimSpace(*serviceNameOverride)
	if serviceName == "" {
		serviceName = cfg.Runtime.WindowsService.Name
	}

	if err := runServer(cfg, serviceName); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}

func runForegroundServer(cfg appconfig.Config) error {
	rt, err := serverapp.Start(cfg)
	if err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		sig := <-sigCh
		log.Printf("component=runtime op=signal_shutdown signal=%q", sig.String())
		rt.Stop()
	}()

	return rt.Wait()
}

func requestServerShutdown(cfg appconfig.Config, reason string) error {
	dialAddr := normalizeLocalDialAddr(cfg.Addr)
	if strings.TrimSpace(dialAddr) == "" {
		return errors.New("dial address is empty")
	}

	opts := client.ConnectionOptions{
		SharedSecret: strings.TrimSpace(cfg.SharedSecret),
		TLSEnabled:   cfg.TLSEnabled,
	}
	if cfg.TLSEnabled {
		opts.TLSRootCertPath = defaultLocalTLSRootCertPath(cfg.TLSCertPath)
		opts.TLSServerName = defaultLocalTLSServerName(cfg, dialAddr)
	}

	c, err := client.NewRoodoxClientWithOptions(dialAddr, opts)
	if err != nil {
		return err
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.ShutdownServer(ctx, reason)
	if err != nil {
		return err
	}
	if !resp.GetAccepted() && !resp.GetAlreadyInProgress() {
		if msg := strings.TrimSpace(resp.GetMessage()); msg != "" {
			return errors.New(msg)
		}
		return errors.New("shutdown request was rejected")
	}
	return nil
}

func restoreDatabase(cfg appconfig.Config, backupPath string, createSafetyBackup bool) error {
	dbPath := strings.TrimSpace(cfg.DBPath)
	if dbPath == "" {
		return errors.New("database path is empty")
	}

	targetPath, err := filepath.Abs(dbPath)
	if err != nil {
		return err
	}
	resolvedBackupPath, err := filepath.Abs(strings.TrimSpace(backupPath))
	if err != nil {
		return err
	}

	result, err := db.RestoreFromBackup(targetPath, resolvedBackupPath, db.RestoreOptions{
		CreateSafetyBackup: createSafetyBackup,
	})
	if err != nil {
		return err
	}

	log.Printf(
		"component=db op=restore target=%q source=%q schema_version=%d safety_backup=%q",
		result.TargetPath,
		result.SourcePath,
		result.SchemaVersion,
		result.SafetyBackupPath,
	)
	return nil
}

func printTLSStatus(cfg appconfig.Config) error {
	status, err := server.InspectTLSArtifacts(cfg.TLSCertPath, cfg.TLSKeyPath)
	if err != nil && status.CertPath == "" {
		return err
	}
	return writeJSON(status)
}

func rotateTLSArtifacts(cfg appconfig.Config, rotateRootCA bool, backupDir string) error {
	result, err := server.RotateTLSArtifacts(cfg.TLSCertPath, cfg.TLSKeyPath, server.TLSRotateOptions{
		RotateRootCA: rotateRootCA,
		BackupDir:    strings.TrimSpace(backupDir),
	})
	if err != nil {
		return err
	}
	return writeJSON(result)
}

func exportTLSRootCA(cfg appconfig.Config, destPath string) error {
	if err := server.ExportTLSRootCertificate(cfg.TLSCertPath, destPath); err != nil {
		return err
	}
	return writeJSON(map[string]string{
		"root_cert_path": defaultLocalTLSRootCertPath(cfg.TLSCertPath),
		"exported_path":  strings.TrimSpace(destPath),
	})
}

type issueJoinBundleOptions struct {
	DeviceID    string
	DeviceName  string
	DeviceRole  string
	DeviceGroup string
}

func issueJoinBundle(cfg appconfig.Config, opts issueJoinBundleOptions) error {
	controlPlane := serverapp.BuildControlPlaneConfig(cfg)
	if controlPlane.JoinBundle.ServiceDiscovery.UseTLS &&
		strings.TrimSpace(controlPlane.JoinBundle.ServiceDiscovery.TLSServerName) == "" {
		controlPlane.JoinBundle.ServiceDiscovery.TLSServerName = defaultLocalTLSServerName(
			cfg,
			normalizeLocalDialAddr(cfg.Addr),
		)
	}

	service := server.NewAdminConsoleService(nil, controlPlane)
	resp, err := service.IssueJoinBundle(context.Background(), &pb.IssueJoinBundleRequest{
		DeviceId:    strings.TrimSpace(opts.DeviceID),
		DeviceName:  strings.TrimSpace(opts.DeviceName),
		DeviceRole:  strings.TrimSpace(opts.DeviceRole),
		DeviceGroup: strings.TrimSpace(opts.DeviceGroup),
	})
	if err != nil {
		return err
	}
	return writeJSON(resp)
}

type workbenchRuntimeSnapshot struct {
	ServerID      string                    `json:"server_id"`
	ListenAddr    string                    `json:"listen_addr"`
	RootDir       string                    `json:"root_dir"`
	DBPath        string                    `json:"db_path"`
	TLSEnabled    bool                      `json:"tls_enabled"`
	AuthEnabled   bool                      `json:"auth_enabled"`
	StartedAtUnix int64                     `json:"started_at_unix"`
	HealthState   string                    `json:"health_state"`
	HealthMessage string                    `json:"health_message"`
	DBFile        workbenchFileStatSummary  `json:"db_file"`
	WALFile       workbenchFileStatSummary  `json:"wal_file"`
	SHMFile       workbenchFileStatSummary  `json:"shm_file"`
	Checkpoint    workbenchCheckpointStatus `json:"checkpoint"`
	Backup        workbenchBackupStatus     `json:"backup"`
}

type workbenchFileStatSummary struct {
	Path           string `json:"path"`
	Exists         bool   `json:"exists"`
	SizeBytes      int64  `json:"size_bytes"`
	ModifiedAtUnix int64  `json:"modified_at_unix"`
}

type workbenchCheckpointStatus struct {
	LastCheckpointAtUnix int64  `json:"last_checkpoint_at_unix"`
	Mode                 string `json:"mode"`
	BusyReaders          int64  `json:"busy_readers"`
	LogFrames            int64  `json:"log_frames"`
	CheckpointedFrames   int64  `json:"checkpointed_frames"`
	LastError            string `json:"last_error"`
}

type workbenchBackupStatus struct {
	Dir              string `json:"dir"`
	IntervalSeconds  int64  `json:"interval_seconds"`
	KeepLatest       uint32 `json:"keep_latest"`
	LastBackupAtUnix int64  `json:"last_backup_at_unix"`
	LastBackupPath   string `json:"last_backup_path"`
	LastError        string `json:"last_error"`
}

type workbenchDeviceSummary struct {
	DeviceID        string `json:"device_id"`
	DisplayName     string `json:"display_name"`
	Role            string `json:"role"`
	OverlayProvider string `json:"overlay_provider"`
	OverlayAddress  string `json:"overlay_address"`
	OnlineState     string `json:"online_state"`
	LastSeenAt      int64  `json:"last_seen_at"`
	SyncState       string `json:"sync_state"`
	MountState      string `json:"mount_state"`
	ClientVersion   string `json:"client_version"`
	PolicyRevision  uint64 `json:"policy_revision"`
}

type workbenchSnapshot struct {
	Runtime         workbenchRuntimeSnapshot `json:"runtime"`
	Devices         []workbenchDeviceSummary `json:"devices"`
	CollectedAtUnix int64                    `json:"collected_at_unix"`
}

type workbenchHotPathMetric struct {
	Path  string `json:"path"`
	Count int64  `json:"count"`
}

type workbenchRPCMetric struct {
	Method     string `json:"method"`
	Count      int64  `json:"count"`
	ErrorCount int64  `json:"error_count"`
	P50Ms      int64  `json:"p50_ms"`
	P95Ms      int64  `json:"p95_ms"`
	P99Ms      int64  `json:"p99_ms"`
}

type workbenchBuildObservability struct {
	SuccessCount   int64 `json:"success_count"`
	FailureCount   int64 `json:"failure_count"`
	LogBytes       int64 `json:"log_bytes"`
	QueueWaitCount int64 `json:"queue_wait_count"`
	QueueWaitP50Ms int64 `json:"queue_wait_p50_ms"`
	QueueWaitP95Ms int64 `json:"queue_wait_p95_ms"`
	QueueWaitP99Ms int64 `json:"queue_wait_p99_ms"`
	DurationCount  int64 `json:"duration_count"`
	DurationP50Ms  int64 `json:"duration_p50_ms"`
	DurationP95Ms  int64 `json:"duration_p95_ms"`
	DurationP99Ms  int64 `json:"duration_p99_ms"`
}

type workbenchObservabilitySnapshot struct {
	WriteFileRangeCalls     int64                       `json:"write_file_range_calls"`
	WriteFileRangeBytes     int64                       `json:"write_file_range_bytes"`
	WriteFileRangeConflicts int64                       `json:"write_file_range_conflicts"`
	SmallWriteBursts        int64                       `json:"small_write_bursts"`
	SmallWriteHotPaths      []workbenchHotPathMetric    `json:"small_write_hot_paths"`
	Build                   workbenchBuildObservability `json:"build"`
	RPCMetrics              []workbenchRPCMetric        `json:"rpc_metrics"`
	CollectedAtUnix         int64                       `json:"collected_at_unix"`
}

type workbenchBackupTriggerResult struct {
	CreatedAtUnix int64  `json:"created_at_unix"`
	Path          string `json:"path"`
}

func printWorkbenchSnapshot(cfg appconfig.Config) error {
	c, err := newLocalAdminClient(cfg)
	if err != nil {
		return err
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runtimeResp, err := c.GetServerRuntime(ctx)
	if err != nil {
		return err
	}
	devices, err := c.ListDevices(ctx)
	if err != nil {
		return err
	}

	snapshot := workbenchSnapshot{
		Runtime: workbenchRuntimeSnapshot{
			ServerID:      runtimeResp.GetServerId(),
			ListenAddr:    runtimeResp.GetListenAddr(),
			RootDir:       runtimeResp.GetRootDir(),
			DBPath:        runtimeResp.GetDbPath(),
			TLSEnabled:    runtimeResp.GetTlsEnabled(),
			AuthEnabled:   runtimeResp.GetAuthEnabled(),
			StartedAtUnix: runtimeResp.GetStartedAtUnix(),
			HealthState:   runtimeResp.GetHealthState(),
			HealthMessage: runtimeResp.GetHealthMessage(),
			DBFile:        workbenchFileStatFromProto(runtimeResp.GetDbFile()),
			WALFile:       workbenchFileStatFromProto(runtimeResp.GetWalFile()),
			SHMFile:       workbenchFileStatFromProto(runtimeResp.GetShmFile()),
			Checkpoint:    workbenchCheckpointFromProto(runtimeResp.GetCheckpoint()),
			Backup:        workbenchBackupFromProto(runtimeResp.GetBackup()),
		},
		Devices:         make([]workbenchDeviceSummary, 0, len(devices)),
		CollectedAtUnix: time.Now().Unix(),
	}
	for _, item := range devices {
		if item == nil {
			continue
		}
		snapshot.Devices = append(snapshot.Devices, workbenchDeviceSummary{
			DeviceID:        item.GetDeviceId(),
			DisplayName:     item.GetDisplayName(),
			Role:            item.GetRole(),
			OverlayProvider: item.GetOverlayProvider(),
			OverlayAddress:  item.GetOverlayAddress(),
			OnlineState:     item.GetOnlineState(),
			LastSeenAt:      item.GetLastSeenAt(),
			SyncState:       item.GetSyncState(),
			MountState:      item.GetMountState(),
			ClientVersion:   item.GetClientVersion(),
			PolicyRevision:  item.GetPolicyRevision(),
		})
	}
	return writeJSON(snapshot)
}

func printWorkbenchObservability(cfg appconfig.Config) error {
	c, err := newLocalAdminClient(cfg)
	if err != nil {
		return err
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.GetServerObservability(ctx)
	if err != nil {
		return err
	}

	snapshot := workbenchObservabilitySnapshot{
		WriteFileRangeCalls:     resp.GetWriteFileRangeCalls(),
		WriteFileRangeBytes:     resp.GetWriteFileRangeBytes(),
		WriteFileRangeConflicts: resp.GetWriteFileRangeConflicts(),
		SmallWriteBursts:        resp.GetSmallWriteBursts(),
		SmallWriteHotPaths:      make([]workbenchHotPathMetric, 0, len(resp.GetSmallWriteHotPaths())),
		Build:                   workbenchBuildFromProto(resp.GetBuild()),
		RPCMetrics:              make([]workbenchRPCMetric, 0, len(resp.GetRpcMetrics())),
		CollectedAtUnix:         time.Now().Unix(),
	}
	for _, item := range resp.GetSmallWriteHotPaths() {
		if item == nil {
			continue
		}
		snapshot.SmallWriteHotPaths = append(snapshot.SmallWriteHotPaths, workbenchHotPathMetric{
			Path:  item.GetPath(),
			Count: item.GetCount(),
		})
	}
	for _, item := range resp.GetRpcMetrics() {
		if item == nil {
			continue
		}
		snapshot.RPCMetrics = append(snapshot.RPCMetrics, workbenchRPCMetric{
			Method:     item.GetMethod(),
			Count:      item.GetCount(),
			ErrorCount: item.GetErrorCount(),
			P50Ms:      item.GetP50Ms(),
			P95Ms:      item.GetP95Ms(),
			P99Ms:      item.GetP99Ms(),
		})
	}
	return writeJSON(snapshot)
}

func triggerServerBackup(cfg appconfig.Config) error {
	c, err := newLocalAdminClient(cfg)
	if err != nil {
		return err
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := c.TriggerServerBackup(ctx)
	if err != nil {
		return err
	}
	return writeJSON(workbenchBackupTriggerResult{
		CreatedAtUnix: resp.GetCreatedAtUnix(),
		Path:          resp.GetPath(),
	})
}

func newLocalAdminClient(cfg appconfig.Config) (*client.RoodoxClient, error) {
	dialAddr := normalizeLocalDialAddr(cfg.Addr)
	if strings.TrimSpace(dialAddr) == "" {
		return nil, errors.New("dial address is empty")
	}

	opts := client.ConnectionOptions{
		SharedSecret: strings.TrimSpace(cfg.SharedSecret),
		TLSEnabled:   cfg.TLSEnabled,
	}
	if cfg.TLSEnabled {
		opts.TLSRootCertPath = defaultLocalTLSRootCertPath(cfg.TLSCertPath)
		opts.TLSServerName = defaultLocalTLSServerName(cfg, dialAddr)
	}

	return client.NewRoodoxClientWithOptions(dialAddr, opts)
}

func normalizeLocalDialAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "127.0.0.1:50051"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			return "127.0.0.1" + addr
		}
		return addr
	}

	switch strings.Trim(host, "[]") {
	case "", "0.0.0.0", "::":
		return net.JoinHostPort("127.0.0.1", port)
	default:
		return net.JoinHostPort(strings.Trim(host, "[]"), port)
	}
}

func defaultLocalTLSRootCertPath(certPath string) string {
	certPath = strings.TrimSpace(certPath)
	if certPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(certPath), "roodox-ca-cert.pem")
}

func workbenchFileStatFromProto(item interface {
	GetPath() string
	GetExists() bool
	GetSizeBytes() int64
	GetModifiedAtUnix() int64
}) workbenchFileStatSummary {
	if item == nil {
		return workbenchFileStatSummary{}
	}
	return workbenchFileStatSummary{
		Path:           item.GetPath(),
		Exists:         item.GetExists(),
		SizeBytes:      item.GetSizeBytes(),
		ModifiedAtUnix: item.GetModifiedAtUnix(),
	}
}

func workbenchCheckpointFromProto(item interface {
	GetLastCheckpointAtUnix() int64
	GetMode() string
	GetBusyReaders() int64
	GetLogFrames() int64
	GetCheckpointedFrames() int64
	GetLastError() string
}) workbenchCheckpointStatus {
	if item == nil {
		return workbenchCheckpointStatus{}
	}
	return workbenchCheckpointStatus{
		LastCheckpointAtUnix: item.GetLastCheckpointAtUnix(),
		Mode:                 item.GetMode(),
		BusyReaders:          item.GetBusyReaders(),
		LogFrames:            item.GetLogFrames(),
		CheckpointedFrames:   item.GetCheckpointedFrames(),
		LastError:            item.GetLastError(),
	}
}

func workbenchBackupFromProto(item interface {
	GetDir() string
	GetIntervalSeconds() int64
	GetKeepLatest() uint32
	GetLastBackupAtUnix() int64
	GetLastBackupPath() string
	GetLastError() string
}) workbenchBackupStatus {
	if item == nil {
		return workbenchBackupStatus{}
	}
	return workbenchBackupStatus{
		Dir:              item.GetDir(),
		IntervalSeconds:  item.GetIntervalSeconds(),
		KeepLatest:       item.GetKeepLatest(),
		LastBackupAtUnix: item.GetLastBackupAtUnix(),
		LastBackupPath:   item.GetLastBackupPath(),
		LastError:        item.GetLastError(),
	}
}

func workbenchBuildFromProto(item interface {
	GetSuccessCount() int64
	GetFailureCount() int64
	GetLogBytes() int64
	GetQueueWaitCount() int64
	GetQueueWaitP50Ms() int64
	GetQueueWaitP95Ms() int64
	GetQueueWaitP99Ms() int64
	GetDurationCount() int64
	GetDurationP50Ms() int64
	GetDurationP95Ms() int64
	GetDurationP99Ms() int64
}) workbenchBuildObservability {
	if item == nil {
		return workbenchBuildObservability{}
	}
	return workbenchBuildObservability{
		SuccessCount:   item.GetSuccessCount(),
		FailureCount:   item.GetFailureCount(),
		LogBytes:       item.GetLogBytes(),
		QueueWaitCount: item.GetQueueWaitCount(),
		QueueWaitP50Ms: item.GetQueueWaitP50Ms(),
		QueueWaitP95Ms: item.GetQueueWaitP95Ms(),
		QueueWaitP99Ms: item.GetQueueWaitP99Ms(),
		DurationCount:  item.GetDurationCount(),
		DurationP50Ms:  item.GetDurationP50Ms(),
		DurationP95Ms:  item.GetDurationP95Ms(),
		DurationP99Ms:  item.GetDurationP99Ms(),
	}
}

func writeJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func defaultLocalTLSServerName(cfg appconfig.Config, dialAddr string) string {
	host, _, err := net.SplitHostPort(dialAddr)
	if err == nil {
		host = strings.Trim(host, "[]")
		if host != "" {
			if ip := net.ParseIP(host); ip == nil {
				return host
			}
			if host == "127.0.0.1" || host == "::1" {
				return "localhost"
			}
		}
	}
	if v := suggestedServerNameFromCertificate(cfg.TLSCertPath); v != "" {
		return v
	}
	return "localhost"
}

func suggestedServerNameFromCertificate(certPath string) string {
	certPath = strings.TrimSpace(certPath)
	if certPath == "" {
		return ""
	}
	pemData, err := os.ReadFile(certPath)
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(pemData)
	if block == nil {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	for _, dns := range cert.DNSNames {
		dns = strings.TrimSpace(dns)
		if dns != "" && !strings.EqualFold(dns, "localhost") {
			return dns
		}
	}
	return strings.TrimSpace(cert.Subject.CommonName)
}

package appconfig

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"strconv"
	"strings"
)

const ConfigPath = "roodox.config.json"

const (
	defaultAddr                        = ":50051"
	defaultRootDir                     = "D:/RoodoxShare"
	defaultDBPath                      = "roodox.db"
	defaultTLSCertPath                 = "certs/roodox-server-cert.pem"
	defaultTLSKeyPath                  = "certs/roodox-server-key.pem"
	defaultRuntimeBinaryPath           = "roodox_server.exe"
	defaultRuntimeStateDir             = "runtime"
	defaultRuntimePIDFile              = "runtime/roodox_server.pid"
	defaultRuntimeLogDir               = "runtime/logs"
	defaultRuntimeStdoutLogName        = "server.stdout.log"
	defaultRuntimeStderrLogName        = "server.stderr.log"
	defaultRuntimeStopTimeoutSeconds   = 10
	defaultWindowsServiceName          = "RoodoxServer"
	defaultWindowsServiceDisplayName   = "Roodox Server"
	defaultWindowsServiceDescription   = "Roodox gRPC server"
	defaultWindowsServiceStartType     = "auto"
	defaultDBCheckpointIntervalSeconds = 300
	defaultDBCheckpointMode            = "truncate"
	defaultDBBackupDir                 = "backups"
	defaultDBBackupIntervalSeconds     = 24 * 60 * 60
	defaultDBBackupKeepLatest          = 7

	defaultArtifactCleanupIntervalSeconds       = 300
	defaultArtifactRetentionSeconds             = 24 * 60 * 60
	defaultArtifactMaxBytes               int64 = 512 << 20
	defaultBuildCleanupIntervalSeconds          = 60
	defaultBuildRetentionSeconds                = 30 * 60
	defaultBuildMaxBytes                  int64 = 2 << 30
	defaultConflictCleanupIntervalSeconds       = 3600
	defaultConflictRetentionSeconds             = 7 * 24 * 60 * 60
	defaultConflictMaxBytes               int64 = 256 << 20
	defaultConflictMaxCopiesPerPath             = 20
	defaultLogCleanupIntervalSeconds            = 900
	defaultLogRetentionSeconds                  = 7 * 24 * 60 * 60
	defaultLogMaxBytes                    int64 = 256 << 20
	defaultLogDir                               = defaultRuntimeLogDir
)

type Config struct {
	Addr               string             `json:"addr"`
	DataRoot           string             `json:"data_root"`
	RootDir            string             `json:"root_dir"`
	DBPath             string             `json:"db_path"`
	Runtime            RuntimeConfig      `json:"runtime"`
	RemoteBuildEnabled bool               `json:"remote_build_enabled"`
	BuildToolDirs      []string           `json:"build_tool_dirs"`
	RequiredBuildTools []string           `json:"required_build_tools"`
	AuthEnabled        bool               `json:"auth_enabled"`
	SharedSecret       string             `json:"shared_secret"`
	TLSEnabled         bool               `json:"tls_enabled"`
	TLSCertPath        string             `json:"tls_cert_path"`
	TLSKeyPath         string             `json:"tls_key_path"`
	Database           DatabaseConfig     `json:"database"`
	ControlPlane       ControlPlaneConfig `json:"control_plane"`
	Cleanup            CleanupConfig      `json:"cleanup"`
}

type DatabaseConfig struct {
	CheckpointIntervalSeconds int    `json:"checkpoint_interval_seconds"`
	CheckpointMode            string `json:"checkpoint_mode"`
	BackupDir                 string `json:"backup_dir"`
	BackupIntervalSeconds     int    `json:"backup_interval_seconds"`
	BackupKeepLatest          int    `json:"backup_keep_latest"`
}

type RuntimeConfig struct {
	BinaryPath                 string               `json:"binary_path"`
	StateDir                   string               `json:"state_dir"`
	PIDFile                    string               `json:"pid_file"`
	LogDir                     string               `json:"log_dir"`
	StdoutLogName              string               `json:"stdout_log_name"`
	StderrLogName              string               `json:"stderr_log_name"`
	GracefulStopTimeoutSeconds int                  `json:"graceful_stop_timeout_seconds"`
	WindowsService             WindowsServiceConfig `json:"windows_service"`
}

type WindowsServiceConfig struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	StartType   string `json:"start_type"`
}

type ControlPlaneConfig struct {
	ServerID                 string                  `json:"server_id"`
	DefaultDeviceGroup       string                  `json:"default_device_group"`
	HeartbeatIntervalSeconds int                     `json:"heartbeat_interval_seconds"`
	DefaultPolicyRevision    uint64                  `json:"default_policy_revision"`
	AvailableActions         []string                `json:"available_actions"`
	DiagnosticsKeepLatest    int                     `json:"diagnostics_keep_latest"`
	AssignedConfig           ClientAssignedConfig    `json:"assigned_config"`
	JoinBundle               JoinBundleControlConfig `json:"join_bundle"`
}

type ClientAssignedConfig struct {
	MountPath          string   `json:"mount_path"`
	SyncRoots          []string `json:"sync_roots"`
	ConflictPolicy     string   `json:"conflict_policy"`
	ReadOnly           bool     `json:"read_only"`
	AutoConnect        bool     `json:"auto_connect"`
	BandwidthLimit     int64    `json:"bandwidth_limit"`
	LogLevel           string   `json:"log_level"`
	LargeFileThreshold int64    `json:"large_file_threshold"`
}

type JoinBundleControlConfig struct {
	OverlayProvider       string                 `json:"overlay_provider"`
	OverlayJoinConfigJSON string                 `json:"overlay_join_config_json"`
	ServiceDiscovery      ServiceDiscoveryConfig `json:"service_discovery"`
}

type ServiceDiscoveryConfig struct {
	Mode          string `json:"mode"`
	Host          string `json:"host"`
	Port          uint32 `json:"port"`
	UseTLS        bool   `json:"use_tls"`
	TLSServerName string `json:"tls_server_name"`
}

type CleanupConfig struct {
	TempArtifacts ArtifactCleanupConfig `json:"temp_artifacts"`
	BuildWorkdirs BuildCleanupConfig    `json:"build_workdirs"`
	ConflictFiles ConflictCleanupConfig `json:"conflict_files"`
	LogFiles      LogCleanupConfig      `json:"log_files"`
}

type ArtifactCleanupConfig struct {
	Enabled          bool     `json:"enabled"`
	IntervalSeconds  int      `json:"interval_seconds"`
	RetentionSeconds int64    `json:"retention_seconds"`
	MaxBytes         int64    `json:"max_bytes"`
	Prefixes         []string `json:"prefixes"`
}

type BuildCleanupConfig struct {
	IntervalSeconds  int   `json:"interval_seconds"`
	RetentionSeconds int64 `json:"retention_seconds"`
	MaxBytes         int64 `json:"max_bytes"`
}

type ConflictCleanupConfig struct {
	Enabled          bool  `json:"enabled"`
	IntervalSeconds  int   `json:"interval_seconds"`
	RetentionSeconds int64 `json:"retention_seconds"`
	MaxBytes         int64 `json:"max_bytes"`
	MaxCopiesPerPath int   `json:"max_copies_per_path"`
}

type LogCleanupConfig struct {
	Enabled          bool     `json:"enabled"`
	Dir              string   `json:"dir"`
	Patterns         []string `json:"patterns"`
	IntervalSeconds  int      `json:"interval_seconds"`
	RetentionSeconds int64    `json:"retention_seconds"`
	MaxBytes         int64    `json:"max_bytes"`
}

func Default() Config {
	return Config{
		Addr:    defaultAddr,
		RootDir: defaultRootDir,
		DBPath:  defaultDBPath,
		Runtime: RuntimeConfig{
			BinaryPath:                 defaultRuntimeBinaryPath,
			StateDir:                   defaultRuntimeStateDir,
			PIDFile:                    defaultRuntimePIDFile,
			LogDir:                     defaultRuntimeLogDir,
			StdoutLogName:              defaultRuntimeStdoutLogName,
			StderrLogName:              defaultRuntimeStderrLogName,
			GracefulStopTimeoutSeconds: defaultRuntimeStopTimeoutSeconds,
			WindowsService: WindowsServiceConfig{
				Name:        defaultWindowsServiceName,
				DisplayName: defaultWindowsServiceDisplayName,
				Description: defaultWindowsServiceDescription,
				StartType:   defaultWindowsServiceStartType,
			},
		},
		RemoteBuildEnabled: true,
		BuildToolDirs:      []string{},
		RequiredBuildTools: []string{"cmake", "make", "build-essential"},
		AuthEnabled:        false,
		SharedSecret:       "",
		TLSEnabled:         false,
		TLSCertPath:        defaultTLSCertPath,
		TLSKeyPath:         defaultTLSKeyPath,
		Database: DatabaseConfig{
			CheckpointIntervalSeconds: defaultDBCheckpointIntervalSeconds,
			CheckpointMode:            defaultDBCheckpointMode,
			BackupDir:                 defaultDBBackupDir,
			BackupIntervalSeconds:     defaultDBBackupIntervalSeconds,
			BackupKeepLatest:          defaultDBBackupKeepLatest,
		},
		ControlPlane: ControlPlaneConfig{
			DefaultDeviceGroup:       "default",
			HeartbeatIntervalSeconds: 15,
			DefaultPolicyRevision:    1,
			AvailableActions: []string{
				"reconnect_overlay",
				"resync",
				"remount",
				"collect_diagnostics",
			},
			DiagnosticsKeepLatest: 20,
			AssignedConfig: ClientAssignedConfig{
				SyncRoots:          []string{"."},
				ConflictPolicy:     "manual",
				ReadOnly:           false,
				AutoConnect:        true,
				BandwidthLimit:     0,
				LogLevel:           "info",
				LargeFileThreshold: 64 << 20,
			},
			JoinBundle: JoinBundleControlConfig{
				ServiceDiscovery: ServiceDiscoveryConfig{
					Mode: DefaultServiceDiscoveryMode(),
				},
			},
		},
		Cleanup: CleanupConfig{
			TempArtifacts: ArtifactCleanupConfig{
				Enabled:          true,
				IntervalSeconds:  defaultArtifactCleanupIntervalSeconds,
				RetentionSeconds: defaultArtifactRetentionSeconds,
				MaxBytes:         defaultArtifactMaxBytes,
				Prefixes: []string{
					"roodox-suite-",
					"roodox-suite-build-",
					"roodox-stress-",
				},
			},
			BuildWorkdirs: BuildCleanupConfig{
				IntervalSeconds:  defaultBuildCleanupIntervalSeconds,
				RetentionSeconds: defaultBuildRetentionSeconds,
				MaxBytes:         defaultBuildMaxBytes,
			},
			ConflictFiles: ConflictCleanupConfig{
				Enabled:          true,
				IntervalSeconds:  defaultConflictCleanupIntervalSeconds,
				RetentionSeconds: defaultConflictRetentionSeconds,
				MaxBytes:         defaultConflictMaxBytes,
				MaxCopiesPerPath: defaultConflictMaxCopiesPerPath,
			},
			LogFiles: LogCleanupConfig{
				Enabled:          true,
				Dir:              defaultLogDir,
				Patterns:         []string{"server*.log"},
				IntervalSeconds:  defaultLogCleanupIntervalSeconds,
				RetentionSeconds: defaultLogRetentionSeconds,
				MaxBytes:         defaultLogMaxBytes,
			},
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()

	b, err := os.ReadFile(path)
	if err == nil {
		if unmarshalErr := json.Unmarshal(b, &cfg); unmarshalErr != nil {
			return Config{}, unmarshalErr
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}

	applyEnvOverrides(&cfg)
	return cfg, nil
}

func Save(path string, cfg Config) error {
	normalized := normalize(cfg)
	b, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func normalize(cfg Config) Config {
	cfg.DataRoot = strings.TrimSpace(cfg.DataRoot)
	if strings.TrimSpace(cfg.Addr) == "" {
		cfg.Addr = defaultAddr
	}
	if strings.TrimSpace(cfg.RootDir) == "" {
		cfg.RootDir = defaultRootDir
	}
	cfg.Runtime.BinaryPath = strings.TrimSpace(cfg.Runtime.BinaryPath)
	if cfg.Runtime.BinaryPath == "" {
		cfg.Runtime.BinaryPath = defaultRuntimeBinaryPath
	}
	cfg = normalizeRuntime(cfg)
	cfg.BuildToolDirs = normalizeList(cfg.BuildToolDirs)
	cfg.RequiredBuildTools = normalizeList(cfg.RequiredBuildTools)
	if len(cfg.RequiredBuildTools) == 0 {
		cfg.RequiredBuildTools = []string{"cmake", "make", "build-essential"}
	}
	cfg = normalizeStoragePaths(cfg)
	if cfg.Database.CheckpointIntervalSeconds <= 0 {
		cfg.Database.CheckpointIntervalSeconds = defaultDBCheckpointIntervalSeconds
	}
	cfg.Database.CheckpointMode = strings.ToLower(strings.TrimSpace(cfg.Database.CheckpointMode))
	switch cfg.Database.CheckpointMode {
	case "passive", "full", "restart", "truncate":
	default:
		cfg.Database.CheckpointMode = defaultDBCheckpointMode
	}
	if cfg.Database.BackupIntervalSeconds <= 0 {
		cfg.Database.BackupIntervalSeconds = defaultDBBackupIntervalSeconds
	}
	if cfg.Database.BackupKeepLatest <= 0 {
		cfg.Database.BackupKeepLatest = defaultDBBackupKeepLatest
	}
	cfg.SharedSecret = strings.TrimSpace(cfg.SharedSecret)
	cfg.ControlPlane.ServerID = strings.TrimSpace(cfg.ControlPlane.ServerID)
	cfg.ControlPlane.DefaultDeviceGroup = strings.TrimSpace(cfg.ControlPlane.DefaultDeviceGroup)
	if cfg.ControlPlane.DefaultDeviceGroup == "" {
		cfg.ControlPlane.DefaultDeviceGroup = "default"
	}
	if cfg.ControlPlane.HeartbeatIntervalSeconds <= 0 {
		cfg.ControlPlane.HeartbeatIntervalSeconds = 15
	}
	if cfg.ControlPlane.DefaultPolicyRevision == 0 {
		cfg.ControlPlane.DefaultPolicyRevision = 1
	}
	cfg.ControlPlane.AvailableActions = normalizeList(cfg.ControlPlane.AvailableActions)
	if len(cfg.ControlPlane.AvailableActions) == 0 {
		cfg.ControlPlane.AvailableActions = []string{
			"reconnect_overlay",
			"resync",
			"remount",
			"collect_diagnostics",
		}
	}
	if cfg.ControlPlane.DiagnosticsKeepLatest <= 0 {
		cfg.ControlPlane.DiagnosticsKeepLatest = 20
	}
	cfg.ControlPlane.AssignedConfig.MountPath = strings.TrimSpace(cfg.ControlPlane.AssignedConfig.MountPath)
	cfg.ControlPlane.AssignedConfig.SyncRoots = normalizeList(cfg.ControlPlane.AssignedConfig.SyncRoots)
	if len(cfg.ControlPlane.AssignedConfig.SyncRoots) == 0 {
		cfg.ControlPlane.AssignedConfig.SyncRoots = []string{"."}
	}
	cfg.ControlPlane.AssignedConfig.ConflictPolicy = strings.TrimSpace(cfg.ControlPlane.AssignedConfig.ConflictPolicy)
	if cfg.ControlPlane.AssignedConfig.ConflictPolicy == "" {
		cfg.ControlPlane.AssignedConfig.ConflictPolicy = "manual"
	}
	cfg.ControlPlane.AssignedConfig.LogLevel = strings.TrimSpace(cfg.ControlPlane.AssignedConfig.LogLevel)
	if cfg.ControlPlane.AssignedConfig.LogLevel == "" {
		cfg.ControlPlane.AssignedConfig.LogLevel = "info"
	}
	if cfg.ControlPlane.AssignedConfig.LargeFileThreshold <= 0 {
		cfg.ControlPlane.AssignedConfig.LargeFileThreshold = 64 << 20
	}
	cfg.ControlPlane.JoinBundle.OverlayProvider = strings.TrimSpace(cfg.ControlPlane.JoinBundle.OverlayProvider)
	cfg.ControlPlane.JoinBundle.OverlayJoinConfigJSON = strings.TrimSpace(cfg.ControlPlane.JoinBundle.OverlayJoinConfigJSON)
	cfg.ControlPlane.JoinBundle.ServiceDiscovery.Mode = strings.TrimSpace(cfg.ControlPlane.JoinBundle.ServiceDiscovery.Mode)
	if cfg.ControlPlane.JoinBundle.ServiceDiscovery.Mode == "" {
		cfg.ControlPlane.JoinBundle.ServiceDiscovery.Mode = DefaultServiceDiscoveryMode()
	}
	cfg.ControlPlane.JoinBundle.ServiceDiscovery.Host = strings.TrimSpace(cfg.ControlPlane.JoinBundle.ServiceDiscovery.Host)
	cfg.ControlPlane.JoinBundle.ServiceDiscovery.TLSServerName = strings.TrimSpace(cfg.ControlPlane.JoinBundle.ServiceDiscovery.TLSServerName)
	if cfg.Cleanup.TempArtifacts.IntervalSeconds <= 0 {
		cfg.Cleanup.TempArtifacts.IntervalSeconds = defaultArtifactCleanupIntervalSeconds
	}
	if cfg.Cleanup.TempArtifacts.RetentionSeconds <= 0 {
		cfg.Cleanup.TempArtifacts.RetentionSeconds = defaultArtifactRetentionSeconds
	}
	if cfg.Cleanup.TempArtifacts.MaxBytes < 0 {
		cfg.Cleanup.TempArtifacts.MaxBytes = 0
	}
	cfg.Cleanup.TempArtifacts.Prefixes = normalizeList(cfg.Cleanup.TempArtifacts.Prefixes)
	if len(cfg.Cleanup.TempArtifacts.Prefixes) == 0 {
		cfg.Cleanup.TempArtifacts.Prefixes = []string{
			"roodox-suite-",
			"roodox-suite-build-",
			"roodox-stress-",
		}
	}
	if cfg.Cleanup.BuildWorkdirs.IntervalSeconds <= 0 {
		cfg.Cleanup.BuildWorkdirs.IntervalSeconds = defaultBuildCleanupIntervalSeconds
	}
	if cfg.Cleanup.BuildWorkdirs.RetentionSeconds <= 0 {
		cfg.Cleanup.BuildWorkdirs.RetentionSeconds = defaultBuildRetentionSeconds
	}
	if cfg.Cleanup.BuildWorkdirs.MaxBytes < 0 {
		cfg.Cleanup.BuildWorkdirs.MaxBytes = 0
	}
	if cfg.Cleanup.ConflictFiles.IntervalSeconds <= 0 {
		cfg.Cleanup.ConflictFiles.IntervalSeconds = defaultConflictCleanupIntervalSeconds
	}
	if cfg.Cleanup.ConflictFiles.RetentionSeconds <= 0 {
		cfg.Cleanup.ConflictFiles.RetentionSeconds = defaultConflictRetentionSeconds
	}
	if cfg.Cleanup.ConflictFiles.MaxBytes < 0 {
		cfg.Cleanup.ConflictFiles.MaxBytes = 0
	}
	if cfg.Cleanup.ConflictFiles.MaxCopiesPerPath < 0 {
		cfg.Cleanup.ConflictFiles.MaxCopiesPerPath = 0
	}
	if cfg.Cleanup.ConflictFiles.Enabled && cfg.Cleanup.ConflictFiles.MaxCopiesPerPath == 0 {
		cfg.Cleanup.ConflictFiles.MaxCopiesPerPath = defaultConflictMaxCopiesPerPath
	}
	if strings.TrimSpace(cfg.Cleanup.LogFiles.Dir) == "" {
		cfg.Cleanup.LogFiles.Dir = cfg.Runtime.LogDir
	}
	cfg.Cleanup.LogFiles.Patterns = normalizeList(cfg.Cleanup.LogFiles.Patterns)
	if len(cfg.Cleanup.LogFiles.Patterns) == 0 {
		cfg.Cleanup.LogFiles.Patterns = []string{"server*.log"}
	}
	if cfg.Cleanup.LogFiles.IntervalSeconds <= 0 {
		cfg.Cleanup.LogFiles.IntervalSeconds = defaultLogCleanupIntervalSeconds
	}
	if cfg.Cleanup.LogFiles.RetentionSeconds <= 0 {
		cfg.Cleanup.LogFiles.RetentionSeconds = defaultLogRetentionSeconds
	}
	if cfg.Cleanup.LogFiles.MaxBytes < 0 {
		cfg.Cleanup.LogFiles.MaxBytes = 0
	}
	return cfg
}
func normalizeRuntime(cfg Config) Config {
	cfg.Runtime.StateDir = strings.TrimSpace(cfg.Runtime.StateDir)
	if cfg.Runtime.StateDir == "" || (cfg.DataRoot != "" && sameConfigPath(cfg.Runtime.StateDir, defaultRuntimeStateDir)) {
		cfg.Runtime.StateDir = deriveDataPath(cfg.DataRoot, defaultRuntimeStateDir)
	}

	cfg.Runtime.PIDFile = strings.TrimSpace(cfg.Runtime.PIDFile)
	if cfg.Runtime.PIDFile == "" || sameConfigPath(cfg.Runtime.PIDFile, defaultRuntimePIDFile) {
		cfg.Runtime.PIDFile = joinConfigPath(cfg.Runtime.StateDir, "roodox_server.pid")
	}

	cfg.Runtime.LogDir = strings.TrimSpace(cfg.Runtime.LogDir)
	if cfg.Runtime.LogDir == "" || sameConfigPath(cfg.Runtime.LogDir, defaultRuntimeLogDir) {
		cfg.Runtime.LogDir = joinConfigPath(cfg.Runtime.StateDir, "logs")
	}

	cfg.Runtime.StdoutLogName = strings.TrimSpace(cfg.Runtime.StdoutLogName)
	if cfg.Runtime.StdoutLogName == "" {
		cfg.Runtime.StdoutLogName = defaultRuntimeStdoutLogName
	}
	cfg.Runtime.StderrLogName = strings.TrimSpace(cfg.Runtime.StderrLogName)
	if cfg.Runtime.StderrLogName == "" {
		cfg.Runtime.StderrLogName = defaultRuntimeStderrLogName
	}
	if cfg.Runtime.GracefulStopTimeoutSeconds <= 0 {
		cfg.Runtime.GracefulStopTimeoutSeconds = defaultRuntimeStopTimeoutSeconds
	}
	cfg.Runtime.WindowsService.Name = strings.TrimSpace(cfg.Runtime.WindowsService.Name)
	if cfg.Runtime.WindowsService.Name == "" {
		cfg.Runtime.WindowsService.Name = defaultWindowsServiceName
	}
	cfg.Runtime.WindowsService.DisplayName = strings.TrimSpace(cfg.Runtime.WindowsService.DisplayName)
	if cfg.Runtime.WindowsService.DisplayName == "" {
		cfg.Runtime.WindowsService.DisplayName = defaultWindowsServiceDisplayName
	}
	cfg.Runtime.WindowsService.Description = strings.TrimSpace(cfg.Runtime.WindowsService.Description)
	if cfg.Runtime.WindowsService.Description == "" {
		cfg.Runtime.WindowsService.Description = defaultWindowsServiceDescription
	}
	cfg.Runtime.WindowsService.StartType = strings.ToLower(strings.TrimSpace(cfg.Runtime.WindowsService.StartType))
	switch cfg.Runtime.WindowsService.StartType {
	case "auto", "manual", "disabled":
	default:
		cfg.Runtime.WindowsService.StartType = defaultWindowsServiceStartType
	}
	return cfg
}

func normalizeStoragePaths(cfg Config) Config {
	cfg.DBPath = strings.TrimSpace(cfg.DBPath)
	if cfg.DBPath == "" || (cfg.DataRoot != "" && sameConfigPath(cfg.DBPath, defaultDBPath)) {
		cfg.DBPath = deriveDataPath(cfg.DataRoot, defaultDBPath)
	}

	cfg.TLSCertPath = strings.TrimSpace(cfg.TLSCertPath)
	if cfg.TLSCertPath == "" || (cfg.DataRoot != "" && sameConfigPath(cfg.TLSCertPath, defaultTLSCertPath)) {
		cfg.TLSCertPath = deriveDataPath(cfg.DataRoot, defaultTLSCertPath)
	}

	cfg.TLSKeyPath = strings.TrimSpace(cfg.TLSKeyPath)
	if cfg.TLSKeyPath == "" || (cfg.DataRoot != "" && sameConfigPath(cfg.TLSKeyPath, defaultTLSKeyPath)) {
		cfg.TLSKeyPath = deriveDataPath(cfg.DataRoot, defaultTLSKeyPath)
	}

	cfg.Database.BackupDir = strings.TrimSpace(cfg.Database.BackupDir)
	if cfg.Database.BackupDir == "" || (cfg.DataRoot != "" && sameConfigPath(cfg.Database.BackupDir, defaultDBBackupDir)) {
		cfg.Database.BackupDir = deriveDataPath(cfg.DataRoot, defaultDBBackupDir)
	}

	cfg.Cleanup.LogFiles.Dir = strings.TrimSpace(cfg.Cleanup.LogFiles.Dir)
	if cfg.Cleanup.LogFiles.Dir == "" || sameConfigPath(cfg.Cleanup.LogFiles.Dir, defaultLogDir) {
		cfg.Cleanup.LogFiles.Dir = cfg.Runtime.LogDir
	}
	return cfg
}

func deriveDataPath(dataRoot, fallback string) string {
	dataRoot = strings.TrimSpace(dataRoot)
	if dataRoot == "" {
		return fallback
	}
	return joinConfigPath(dataRoot, fallback)
}

func joinConfigPath(base, rel string) string {
	base = strings.TrimSpace(strings.ReplaceAll(base, "\\", "/"))
	rel = strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/"))
	if base == "" {
		return rel
	}
	if rel == "" {
		return base
	}
	return path.Clean(strings.TrimPrefix(base, "./") + "/" + strings.TrimPrefix(rel, "./"))
}

func sameConfigPath(a, b string) bool {
	return strings.EqualFold(cleanConfigPath(a), cleanConfigPath(b))
}

func cleanConfigPath(v string) string {
	v = strings.TrimSpace(strings.ReplaceAll(v, "\\", "/"))
	if v == "" {
		return ""
	}
	return path.Clean(v)
}

func applyEnvOverrides(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("ROODOX_ADDR")); v != "" {
		cfg.Addr = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_DATA_ROOT")); v != "" {
		cfg.DataRoot = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_ROOT_DIR")); v != "" {
		cfg.RootDir = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_DB_PATH")); v != "" {
		cfg.DBPath = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_RUNTIME_BINARY_PATH")); v != "" {
		cfg.Runtime.BinaryPath = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_RUNTIME_STATE_DIR")); v != "" {
		cfg.Runtime.StateDir = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_RUNTIME_PID_FILE")); v != "" {
		cfg.Runtime.PIDFile = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_RUNTIME_LOG_DIR")); v != "" {
		cfg.Runtime.LogDir = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_RUNTIME_STDOUT_LOG_NAME")); v != "" {
		cfg.Runtime.StdoutLogName = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_RUNTIME_STDERR_LOG_NAME")); v != "" {
		cfg.Runtime.StderrLogName = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_RUNTIME_GRACEFUL_STOP_TIMEOUT_SECONDS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.Runtime.GracefulStopTimeoutSeconds = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_WINDOWS_SERVICE_NAME")); v != "" {
		cfg.Runtime.WindowsService.Name = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_WINDOWS_SERVICE_DISPLAY_NAME")); v != "" {
		cfg.Runtime.WindowsService.DisplayName = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_WINDOWS_SERVICE_DESCRIPTION")); v != "" {
		cfg.Runtime.WindowsService.Description = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_WINDOWS_SERVICE_START_TYPE")); v != "" {
		cfg.Runtime.WindowsService.StartType = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_REMOTE_BUILD_ENABLED")); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.RemoteBuildEnabled = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_BUILD_TOOL_DIRS")); v != "" {
		cfg.BuildToolDirs = normalizeList(strings.Split(v, string(os.PathListSeparator)))
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_BUILD_REQUIRED_TOOLS")); v != "" {
		cfg.RequiredBuildTools = normalizeList(strings.Split(v, ","))
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_AUTH_ENABLED")); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.AuthEnabled = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_SHARED_SECRET")); v != "" {
		cfg.SharedSecret = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_TLS_ENABLED")); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.TLSEnabled = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_TLS_CERT_PATH")); v != "" {
		cfg.TLSCertPath = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_TLS_KEY_PATH")); v != "" {
		cfg.TLSKeyPath = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_DB_CHECKPOINT_INTERVAL_SECONDS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.Database.CheckpointIntervalSeconds = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_DB_CHECKPOINT_MODE")); v != "" {
		cfg.Database.CheckpointMode = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_DB_BACKUP_DIR")); v != "" {
		cfg.Database.BackupDir = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_DB_BACKUP_INTERVAL_SECONDS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.Database.BackupIntervalSeconds = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_DB_BACKUP_KEEP_LATEST")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.Database.BackupKeepLatest = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_SERVER_ID")); v != "" {
		cfg.ControlPlane.ServerID = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_DEVICE_GROUP")); v != "" {
		cfg.ControlPlane.DefaultDeviceGroup = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_HEARTBEAT_INTERVAL_SECONDS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.ControlPlane.HeartbeatIntervalSeconds = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_POLICY_REVISION")); v != "" {
		if parsed, err := strconv.ParseUint(v, 10, 64); err == nil {
			cfg.ControlPlane.DefaultPolicyRevision = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_CLIENT_MOUNT_PATH")); v != "" {
		cfg.ControlPlane.AssignedConfig.MountPath = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_SYNC_ROOTS")); v != "" {
		cfg.ControlPlane.AssignedConfig.SyncRoots = normalizeList(strings.Split(v, ","))
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_CONFLICT_POLICY")); v != "" {
		cfg.ControlPlane.AssignedConfig.ConflictPolicy = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_READ_ONLY")); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.ControlPlane.AssignedConfig.ReadOnly = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_AUTO_CONNECT")); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.ControlPlane.AssignedConfig.AutoConnect = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_BANDWIDTH_LIMIT")); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.ControlPlane.AssignedConfig.BandwidthLimit = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_LOG_LEVEL")); v != "" {
		cfg.ControlPlane.AssignedConfig.LogLevel = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_LARGE_FILE_THRESHOLD")); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.ControlPlane.AssignedConfig.LargeFileThreshold = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_BUNDLE_OVERLAY_PROVIDER")); v != "" {
		cfg.ControlPlane.JoinBundle.OverlayProvider = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_BUNDLE_OVERLAY_JOIN_CONFIG_JSON")); v != "" {
		cfg.ControlPlane.JoinBundle.OverlayJoinConfigJSON = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_BUNDLE_SERVICE_DISCOVERY_MODE")); v != "" {
		cfg.ControlPlane.JoinBundle.ServiceDiscovery.Mode = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_BUNDLE_SERVICE_HOST")); v != "" {
		cfg.ControlPlane.JoinBundle.ServiceDiscovery.Host = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_BUNDLE_SERVICE_PORT")); v != "" {
		if parsed, err := strconv.ParseUint(v, 10, 32); err == nil {
			cfg.ControlPlane.JoinBundle.ServiceDiscovery.Port = uint32(parsed)
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_BUNDLE_USE_TLS")); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.ControlPlane.JoinBundle.ServiceDiscovery.UseTLS = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_BUNDLE_TLS_SERVER_NAME")); v != "" {
		cfg.ControlPlane.JoinBundle.ServiceDiscovery.TLSServerName = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_AVAILABLE_ACTIONS")); v != "" {
		cfg.ControlPlane.AvailableActions = normalizeList(strings.Split(v, ","))
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_DIAGNOSTICS_KEEP_LATEST")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.ControlPlane.DiagnosticsKeepLatest = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_ARTIFACT_CLEANUP_ENABLED")); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.Cleanup.TempArtifacts.Enabled = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_ARTIFACT_CLEANUP_INTERVAL_SECONDS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.Cleanup.TempArtifacts.IntervalSeconds = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_ARTIFACT_RETENTION_SECONDS")); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Cleanup.TempArtifacts.RetentionSeconds = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_ARTIFACT_MAX_BYTES")); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Cleanup.TempArtifacts.MaxBytes = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_ARTIFACT_PREFIXES")); v != "" {
		cfg.Cleanup.TempArtifacts.Prefixes = normalizeList(strings.Split(v, ","))
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_BUILD_CLEANUP_INTERVAL_SECONDS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.Cleanup.BuildWorkdirs.IntervalSeconds = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_BUILD_RETENTION_SECONDS")); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Cleanup.BuildWorkdirs.RetentionSeconds = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_BUILD_MAX_BYTES")); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Cleanup.BuildWorkdirs.MaxBytes = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_CONFLICT_CLEANUP_ENABLED")); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.Cleanup.ConflictFiles.Enabled = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_CONFLICT_CLEANUP_INTERVAL_SECONDS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.Cleanup.ConflictFiles.IntervalSeconds = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_CONFLICT_RETENTION_SECONDS")); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Cleanup.ConflictFiles.RetentionSeconds = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_CONFLICT_MAX_BYTES")); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Cleanup.ConflictFiles.MaxBytes = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_CONFLICT_MAX_COPIES_PER_PATH")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.Cleanup.ConflictFiles.MaxCopiesPerPath = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_LOG_CLEANUP_ENABLED")); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.Cleanup.LogFiles.Enabled = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_LOG_CLEANUP_DIR")); v != "" {
		cfg.Cleanup.LogFiles.Dir = v
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_LOG_CLEANUP_PATTERNS")); v != "" {
		cfg.Cleanup.LogFiles.Patterns = normalizeList(strings.Split(v, ","))
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_LOG_CLEANUP_INTERVAL_SECONDS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.Cleanup.LogFiles.IntervalSeconds = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_LOG_RETENTION_SECONDS")); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Cleanup.LogFiles.RetentionSeconds = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROODOX_LOG_MAX_BYTES")); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Cleanup.LogFiles.MaxBytes = parsed
		}
	}
	*cfg = normalize(*cfg)
}

func normalizeList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, v := range in {
		s := strings.TrimSpace(v)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

func DefaultServiceDiscoveryMode() string {
	return "static"
}

package appconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppliesCleanupDefaultsAndOverrides(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "roodox.config.json")
	content := `{
  "root_dir": "E:/share",
  "runtime": {
    "binary_path": "./bin/roodox_server.exe",
    "state_dir": "./runtime-state",
    "pid_file": "./runtime-state/server.pid",
    "log_dir": "./runtime-state/logs",
    "stdout_log_name": "runtime.stdout.log",
    "stderr_log_name": "runtime.stderr.log",
    "graceful_stop_timeout_seconds": 25,
    "windows_service": {
      "name": "RoodoxServerDev",
      "display_name": "Roodox Server Dev",
      "description": "Roodox test service",
      "start_type": "manual"
    }
  },
  "control_plane": {
    "server_id": "srv-main",
    "default_device_group": "team-a",
    "heartbeat_interval_seconds": 20,
    "default_policy_revision": 7,
    "available_actions": ["resync", "remount"],
    "diagnostics_keep_latest": 8,
    "assigned_config": {
      "mount_path": "/Volumes/Roodox",
      "sync_roots": ["src", "docs"],
      "conflict_policy": "server_wins",
      "read_only": true,
      "auto_connect": false,
      "bandwidth_limit": 1024,
      "log_level": "warn",
      "large_file_threshold": 8388608
    },
    "join_bundle": {
      "overlay_provider": "tailscale",
      "overlay_join_config_json": "{\"authKey\":\"tskey\"}",
      "service_discovery": {
        "mode": "static",
        "host": "100.86.84.37",
        "port": 50051,
        "use_tls": true,
        "tls_server_name": "roodox.internal"
      }
    }
  },
  "cleanup": {
    "temp_artifacts": {
      "enabled": true,
      "interval_seconds": 120,
      "retention_seconds": 7200,
      "max_bytes": 1048576,
      "prefixes": ["custom-a-", "custom-b-"]
    },
    "build_workdirs": {
      "interval_seconds": 30,
      "retention_seconds": 900,
      "max_bytes": 2048
    },
    "conflict_files": {
      "enabled": true,
      "interval_seconds": 600,
      "retention_seconds": 3600,
      "max_bytes": 4096,
      "max_copies_per_path": 7
    },
    "log_files": {
      "enabled": true,
      "dir": "./logs",
      "patterns": ["server*.log", "custom.log"],
      "interval_seconds": 120,
      "retention_seconds": 1800,
      "max_bytes": 8192
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Cleanup.TempArtifacts.IntervalSeconds != 120 {
		t.Fatalf("TempArtifacts.IntervalSeconds = %d, want 120", cfg.Cleanup.TempArtifacts.IntervalSeconds)
	}
	if cfg.Runtime.BinaryPath != "./bin/roodox_server.exe" {
		t.Fatalf("Runtime.BinaryPath = %q, want %q", cfg.Runtime.BinaryPath, "./bin/roodox_server.exe")
	}
	if cfg.Runtime.StateDir != "./runtime-state" {
		t.Fatalf("Runtime.StateDir = %q, want %q", cfg.Runtime.StateDir, "./runtime-state")
	}
	if cfg.Runtime.PIDFile != "./runtime-state/server.pid" {
		t.Fatalf("Runtime.PIDFile = %q, want %q", cfg.Runtime.PIDFile, "./runtime-state/server.pid")
	}
	if cfg.Runtime.LogDir != "./runtime-state/logs" {
		t.Fatalf("Runtime.LogDir = %q, want %q", cfg.Runtime.LogDir, "./runtime-state/logs")
	}
	if cfg.Runtime.StdoutLogName != "runtime.stdout.log" {
		t.Fatalf("Runtime.StdoutLogName = %q, want %q", cfg.Runtime.StdoutLogName, "runtime.stdout.log")
	}
	if cfg.Runtime.StderrLogName != "runtime.stderr.log" {
		t.Fatalf("Runtime.StderrLogName = %q, want %q", cfg.Runtime.StderrLogName, "runtime.stderr.log")
	}
	if cfg.Runtime.GracefulStopTimeoutSeconds != 25 {
		t.Fatalf("Runtime.GracefulStopTimeoutSeconds = %d, want %d", cfg.Runtime.GracefulStopTimeoutSeconds, 25)
	}
	if cfg.Runtime.WindowsService.Name != "RoodoxServerDev" {
		t.Fatalf("Runtime.WindowsService.Name = %q, want %q", cfg.Runtime.WindowsService.Name, "RoodoxServerDev")
	}
	if cfg.Runtime.WindowsService.DisplayName != "Roodox Server Dev" {
		t.Fatalf("Runtime.WindowsService.DisplayName = %q, want %q", cfg.Runtime.WindowsService.DisplayName, "Roodox Server Dev")
	}
	if cfg.Runtime.WindowsService.Description != "Roodox test service" {
		t.Fatalf("Runtime.WindowsService.Description = %q, want %q", cfg.Runtime.WindowsService.Description, "Roodox test service")
	}
	if cfg.Runtime.WindowsService.StartType != "manual" {
		t.Fatalf("Runtime.WindowsService.StartType = %q, want %q", cfg.Runtime.WindowsService.StartType, "manual")
	}
	if cfg.ControlPlane.ServerID != "srv-main" {
		t.Fatalf("ControlPlane.ServerID = %q, want %q", cfg.ControlPlane.ServerID, "srv-main")
	}
	if cfg.ControlPlane.DefaultDeviceGroup != "team-a" {
		t.Fatalf("ControlPlane.DefaultDeviceGroup = %q, want %q", cfg.ControlPlane.DefaultDeviceGroup, "team-a")
	}
	if cfg.ControlPlane.HeartbeatIntervalSeconds != 20 {
		t.Fatalf("ControlPlane.HeartbeatIntervalSeconds = %d, want 20", cfg.ControlPlane.HeartbeatIntervalSeconds)
	}
	if cfg.ControlPlane.DefaultPolicyRevision != 7 {
		t.Fatalf("ControlPlane.DefaultPolicyRevision = %d, want 7", cfg.ControlPlane.DefaultPolicyRevision)
	}
	if cfg.ControlPlane.JoinBundle.OverlayProvider != "tailscale" {
		t.Fatalf("JoinBundle.OverlayProvider = %q, want %q", cfg.ControlPlane.JoinBundle.OverlayProvider, "tailscale")
	}
	if cfg.ControlPlane.JoinBundle.ServiceDiscovery.Host != "100.86.84.37" {
		t.Fatalf("JoinBundle.ServiceDiscovery.Host = %q, want %q", cfg.ControlPlane.JoinBundle.ServiceDiscovery.Host, "100.86.84.37")
	}
	if cfg.Database.CheckpointMode != defaultDBCheckpointMode {
		t.Fatalf("Database.CheckpointMode = %q, want %q", cfg.Database.CheckpointMode, defaultDBCheckpointMode)
	}
	if cfg.Database.BackupDir != defaultDBBackupDir {
		t.Fatalf("Database.BackupDir = %q, want %q", cfg.Database.BackupDir, defaultDBBackupDir)
	}
	if cfg.ControlPlane.AssignedConfig.MountPath != "/Volumes/Roodox" {
		t.Fatalf("AssignedConfig.MountPath = %q, want %q", cfg.ControlPlane.AssignedConfig.MountPath, "/Volumes/Roodox")
	}
	if len(cfg.ControlPlane.AvailableActions) != 2 {
		t.Fatalf("AvailableActions length = %d, want 2", len(cfg.ControlPlane.AvailableActions))
	}
	if cfg.ControlPlane.DiagnosticsKeepLatest != 8 {
		t.Fatalf("DiagnosticsKeepLatest = %d, want 8", cfg.ControlPlane.DiagnosticsKeepLatest)
	}
	if cfg.Cleanup.TempArtifacts.RetentionSeconds != 7200 {
		t.Fatalf("TempArtifacts.RetentionSeconds = %d, want 7200", cfg.Cleanup.TempArtifacts.RetentionSeconds)
	}
	if cfg.Cleanup.TempArtifacts.MaxBytes != 1048576 {
		t.Fatalf("TempArtifacts.MaxBytes = %d, want 1048576", cfg.Cleanup.TempArtifacts.MaxBytes)
	}
	if got, want := len(cfg.Cleanup.TempArtifacts.Prefixes), 2; got != want {
		t.Fatalf("TempArtifacts.Prefixes length = %d, want %d", got, want)
	}
	if cfg.Cleanup.BuildWorkdirs.IntervalSeconds != 30 {
		t.Fatalf("BuildWorkdirs.IntervalSeconds = %d, want 30", cfg.Cleanup.BuildWorkdirs.IntervalSeconds)
	}
	if cfg.Cleanup.BuildWorkdirs.RetentionSeconds != 900 {
		t.Fatalf("BuildWorkdirs.RetentionSeconds = %d, want 900", cfg.Cleanup.BuildWorkdirs.RetentionSeconds)
	}
	if cfg.Cleanup.BuildWorkdirs.MaxBytes != 2048 {
		t.Fatalf("BuildWorkdirs.MaxBytes = %d, want 2048", cfg.Cleanup.BuildWorkdirs.MaxBytes)
	}
	if cfg.Cleanup.ConflictFiles.IntervalSeconds != 600 {
		t.Fatalf("ConflictFiles.IntervalSeconds = %d, want 600", cfg.Cleanup.ConflictFiles.IntervalSeconds)
	}
	if cfg.Cleanup.ConflictFiles.RetentionSeconds != 3600 {
		t.Fatalf("ConflictFiles.RetentionSeconds = %d, want 3600", cfg.Cleanup.ConflictFiles.RetentionSeconds)
	}
	if cfg.Cleanup.ConflictFiles.MaxBytes != 4096 {
		t.Fatalf("ConflictFiles.MaxBytes = %d, want 4096", cfg.Cleanup.ConflictFiles.MaxBytes)
	}
	if cfg.Cleanup.ConflictFiles.MaxCopiesPerPath != 7 {
		t.Fatalf("ConflictFiles.MaxCopiesPerPath = %d, want 7", cfg.Cleanup.ConflictFiles.MaxCopiesPerPath)
	}
	if cfg.Cleanup.LogFiles.Dir != "./logs" {
		t.Fatalf("LogFiles.Dir = %q, want %q", cfg.Cleanup.LogFiles.Dir, "./logs")
	}
	if got, want := len(cfg.Cleanup.LogFiles.Patterns), 2; got != want {
		t.Fatalf("LogFiles.Patterns length = %d, want %d", got, want)
	}
	if cfg.Cleanup.LogFiles.MaxBytes != 8192 {
		t.Fatalf("LogFiles.MaxBytes = %d, want 8192", cfg.Cleanup.LogFiles.MaxBytes)
	}
}

func TestLoadFillsMissingCleanupDefaults(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "roodox.config.json")
	if err := os.WriteFile(configPath, []byte(`{"root_dir":"E:/share"}`), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if !cfg.Cleanup.TempArtifacts.Enabled {
		t.Fatal("TempArtifacts.Enabled should default to true")
	}
	if cfg.Runtime.BinaryPath != defaultRuntimeBinaryPath {
		t.Fatalf("Runtime.BinaryPath = %q, want %q", cfg.Runtime.BinaryPath, defaultRuntimeBinaryPath)
	}
	if cfg.Runtime.StateDir != defaultRuntimeStateDir {
		t.Fatalf("Runtime.StateDir = %q, want %q", cfg.Runtime.StateDir, defaultRuntimeStateDir)
	}
	if cfg.Runtime.PIDFile != defaultRuntimePIDFile {
		t.Fatalf("Runtime.PIDFile = %q, want %q", cfg.Runtime.PIDFile, defaultRuntimePIDFile)
	}
	if cfg.Runtime.LogDir != defaultRuntimeLogDir {
		t.Fatalf("Runtime.LogDir = %q, want %q", cfg.Runtime.LogDir, defaultRuntimeLogDir)
	}
	if cfg.Runtime.StdoutLogName != defaultRuntimeStdoutLogName {
		t.Fatalf("Runtime.StdoutLogName = %q, want %q", cfg.Runtime.StdoutLogName, defaultRuntimeStdoutLogName)
	}
	if cfg.Runtime.StderrLogName != defaultRuntimeStderrLogName {
		t.Fatalf("Runtime.StderrLogName = %q, want %q", cfg.Runtime.StderrLogName, defaultRuntimeStderrLogName)
	}
	if cfg.Runtime.GracefulStopTimeoutSeconds != defaultRuntimeStopTimeoutSeconds {
		t.Fatalf("Runtime.GracefulStopTimeoutSeconds = %d, want %d", cfg.Runtime.GracefulStopTimeoutSeconds, defaultRuntimeStopTimeoutSeconds)
	}
	if cfg.Runtime.WindowsService.Name != defaultWindowsServiceName {
		t.Fatalf("Runtime.WindowsService.Name = %q, want %q", cfg.Runtime.WindowsService.Name, defaultWindowsServiceName)
	}
	if cfg.Runtime.WindowsService.DisplayName != defaultWindowsServiceDisplayName {
		t.Fatalf("Runtime.WindowsService.DisplayName = %q, want %q", cfg.Runtime.WindowsService.DisplayName, defaultWindowsServiceDisplayName)
	}
	if cfg.Runtime.WindowsService.Description != defaultWindowsServiceDescription {
		t.Fatalf("Runtime.WindowsService.Description = %q, want %q", cfg.Runtime.WindowsService.Description, defaultWindowsServiceDescription)
	}
	if cfg.Runtime.WindowsService.StartType != defaultWindowsServiceStartType {
		t.Fatalf("Runtime.WindowsService.StartType = %q, want %q", cfg.Runtime.WindowsService.StartType, defaultWindowsServiceStartType)
	}
	if cfg.Database.CheckpointIntervalSeconds != defaultDBCheckpointIntervalSeconds {
		t.Fatalf("Database.CheckpointIntervalSeconds = %d, want %d", cfg.Database.CheckpointIntervalSeconds, defaultDBCheckpointIntervalSeconds)
	}
	if cfg.Database.BackupIntervalSeconds != defaultDBBackupIntervalSeconds {
		t.Fatalf("Database.BackupIntervalSeconds = %d, want %d", cfg.Database.BackupIntervalSeconds, defaultDBBackupIntervalSeconds)
	}
	if cfg.Database.BackupKeepLatest != defaultDBBackupKeepLatest {
		t.Fatalf("Database.BackupKeepLatest = %d, want %d", cfg.Database.BackupKeepLatest, defaultDBBackupKeepLatest)
	}
	if cfg.ControlPlane.DefaultDeviceGroup != "default" {
		t.Fatalf("ControlPlane.DefaultDeviceGroup = %q, want %q", cfg.ControlPlane.DefaultDeviceGroup, "default")
	}
	if cfg.ControlPlane.HeartbeatIntervalSeconds != 15 {
		t.Fatalf("ControlPlane.HeartbeatIntervalSeconds = %d, want 15", cfg.ControlPlane.HeartbeatIntervalSeconds)
	}
	if cfg.ControlPlane.DefaultPolicyRevision != 1 {
		t.Fatalf("ControlPlane.DefaultPolicyRevision = %d, want 1", cfg.ControlPlane.DefaultPolicyRevision)
	}
	if cfg.ControlPlane.JoinBundle.ServiceDiscovery.Mode != DefaultServiceDiscoveryMode() {
		t.Fatalf("JoinBundle.ServiceDiscovery.Mode = %q, want %q", cfg.ControlPlane.JoinBundle.ServiceDiscovery.Mode, DefaultServiceDiscoveryMode())
	}
	if len(cfg.ControlPlane.AssignedConfig.SyncRoots) != 1 || cfg.ControlPlane.AssignedConfig.SyncRoots[0] != "." {
		t.Fatalf("AssignedConfig.SyncRoots = %v, want [.]", cfg.ControlPlane.AssignedConfig.SyncRoots)
	}
	if cfg.Cleanup.TempArtifacts.IntervalSeconds != defaultArtifactCleanupIntervalSeconds {
		t.Fatalf("TempArtifacts.IntervalSeconds = %d, want %d", cfg.Cleanup.TempArtifacts.IntervalSeconds, defaultArtifactCleanupIntervalSeconds)
	}
	if cfg.Cleanup.TempArtifacts.RetentionSeconds != defaultArtifactRetentionSeconds {
		t.Fatalf("TempArtifacts.RetentionSeconds = %d, want %d", cfg.Cleanup.TempArtifacts.RetentionSeconds, defaultArtifactRetentionSeconds)
	}
	if cfg.Cleanup.BuildWorkdirs.IntervalSeconds != defaultBuildCleanupIntervalSeconds {
		t.Fatalf("BuildWorkdirs.IntervalSeconds = %d, want %d", cfg.Cleanup.BuildWorkdirs.IntervalSeconds, defaultBuildCleanupIntervalSeconds)
	}
	if cfg.Cleanup.BuildWorkdirs.RetentionSeconds != defaultBuildRetentionSeconds {
		t.Fatalf("BuildWorkdirs.RetentionSeconds = %d, want %d", cfg.Cleanup.BuildWorkdirs.RetentionSeconds, defaultBuildRetentionSeconds)
	}
	if cfg.Cleanup.BuildWorkdirs.MaxBytes != defaultBuildMaxBytes {
		t.Fatalf("BuildWorkdirs.MaxBytes = %d, want %d", cfg.Cleanup.BuildWorkdirs.MaxBytes, defaultBuildMaxBytes)
	}
	if cfg.Cleanup.ConflictFiles.IntervalSeconds != defaultConflictCleanupIntervalSeconds {
		t.Fatalf("ConflictFiles.IntervalSeconds = %d, want %d", cfg.Cleanup.ConflictFiles.IntervalSeconds, defaultConflictCleanupIntervalSeconds)
	}
	if cfg.Cleanup.ConflictFiles.RetentionSeconds != defaultConflictRetentionSeconds {
		t.Fatalf("ConflictFiles.RetentionSeconds = %d, want %d", cfg.Cleanup.ConflictFiles.RetentionSeconds, defaultConflictRetentionSeconds)
	}
	if cfg.Cleanup.ConflictFiles.MaxCopiesPerPath != defaultConflictMaxCopiesPerPath {
		t.Fatalf("ConflictFiles.MaxCopiesPerPath = %d, want %d", cfg.Cleanup.ConflictFiles.MaxCopiesPerPath, defaultConflictMaxCopiesPerPath)
	}
	if cfg.Cleanup.LogFiles.Dir != defaultLogDir {
		t.Fatalf("LogFiles.Dir = %q, want %q", cfg.Cleanup.LogFiles.Dir, defaultLogDir)
	}
	if cfg.Cleanup.LogFiles.IntervalSeconds != defaultLogCleanupIntervalSeconds {
		t.Fatalf("LogFiles.IntervalSeconds = %d, want %d", cfg.Cleanup.LogFiles.IntervalSeconds, defaultLogCleanupIntervalSeconds)
	}
	if cfg.Cleanup.LogFiles.MaxBytes != defaultLogMaxBytes {
		t.Fatalf("LogFiles.MaxBytes = %d, want %d", cfg.Cleanup.LogFiles.MaxBytes, defaultLogMaxBytes)
	}
}
func TestLoadDerivesStoragePathsFromDataRoot(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "roodox.config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "data_root": "data",
  "root_dir": "E:/share"
}`), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.DataRoot != "data" {
		t.Fatalf("DataRoot = %q, want %q", cfg.DataRoot, "data")
	}
	if cfg.DBPath != "data/roodox.db" {
		t.Fatalf("DBPath = %q, want %q", cfg.DBPath, "data/roodox.db")
	}
	if cfg.Runtime.StateDir != "data/runtime" {
		t.Fatalf("Runtime.StateDir = %q, want %q", cfg.Runtime.StateDir, "data/runtime")
	}
	if cfg.Runtime.PIDFile != "data/runtime/roodox_server.pid" {
		t.Fatalf("Runtime.PIDFile = %q, want %q", cfg.Runtime.PIDFile, "data/runtime/roodox_server.pid")
	}
	if cfg.Runtime.LogDir != "data/runtime/logs" {
		t.Fatalf("Runtime.LogDir = %q, want %q", cfg.Runtime.LogDir, "data/runtime/logs")
	}
	if cfg.TLSCertPath != "data/certs/roodox-server-cert.pem" {
		t.Fatalf("TLSCertPath = %q, want %q", cfg.TLSCertPath, "data/certs/roodox-server-cert.pem")
	}
	if cfg.TLSKeyPath != "data/certs/roodox-server-key.pem" {
		t.Fatalf("TLSKeyPath = %q, want %q", cfg.TLSKeyPath, "data/certs/roodox-server-key.pem")
	}
	if cfg.Database.BackupDir != "data/backups" {
		t.Fatalf("Database.BackupDir = %q, want %q", cfg.Database.BackupDir, "data/backups")
	}
	if cfg.Cleanup.LogFiles.Dir != "data/runtime/logs" {
		t.Fatalf("Cleanup.LogFiles.Dir = %q, want %q", cfg.Cleanup.LogFiles.Dir, "data/runtime/logs")
	}
}

func TestLoadKeepsExplicitStorageOverridesWithDataRoot(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "roodox.config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "data_root": "data",
  "root_dir": "E:/share",
  "db_path": "./custom/roodox.db",
  "tls_cert_path": "./custom/server-cert.pem",
  "tls_key_path": "./custom/server-key.pem",
  "database": {
    "backup_dir": "./custom/backups"
  },
  "runtime": {
    "state_dir": "./custom/runtime",
    "pid_file": "./custom/runtime/pid.txt",
    "log_dir": "./custom/runtime/logs"
  },
  "cleanup": {
    "log_files": {
      "dir": "./custom/runtime/logs"
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.DBPath != "./custom/roodox.db" {
		t.Fatalf("DBPath = %q, want %q", cfg.DBPath, "./custom/roodox.db")
	}
	if cfg.Runtime.StateDir != "./custom/runtime" {
		t.Fatalf("Runtime.StateDir = %q, want %q", cfg.Runtime.StateDir, "./custom/runtime")
	}
	if cfg.Runtime.PIDFile != "./custom/runtime/pid.txt" {
		t.Fatalf("Runtime.PIDFile = %q, want %q", cfg.Runtime.PIDFile, "./custom/runtime/pid.txt")
	}
	if cfg.Runtime.LogDir != "./custom/runtime/logs" {
		t.Fatalf("Runtime.LogDir = %q, want %q", cfg.Runtime.LogDir, "./custom/runtime/logs")
	}
	if cfg.TLSCertPath != "./custom/server-cert.pem" {
		t.Fatalf("TLSCertPath = %q, want %q", cfg.TLSCertPath, "./custom/server-cert.pem")
	}
	if cfg.TLSKeyPath != "./custom/server-key.pem" {
		t.Fatalf("TLSKeyPath = %q, want %q", cfg.TLSKeyPath, "./custom/server-key.pem")
	}
	if cfg.Database.BackupDir != "./custom/backups" {
		t.Fatalf("Database.BackupDir = %q, want %q", cfg.Database.BackupDir, "./custom/backups")
	}
	if cfg.Cleanup.LogFiles.Dir != "./custom/runtime/logs" {
		t.Fatalf("Cleanup.LogFiles.Dir = %q, want %q", cfg.Cleanup.LogFiles.Dir, "./custom/runtime/logs")
	}
}

export type Lang = "zh" | "en";
export type ViewKey = "dashboard" | "devices" | "operations" | "access" | "logs" | "settings";
export type DeviceFilter = "all" | "online" | "degraded" | "offline" | "mounted" | `role:${string}` | `overlay:${string}`;
export type DeviceSort = "recent" | "name";

export type AppConfig = {
  addr: string;
  data_root: string;
  root_dir: string;
  remote_build_enabled: boolean;
  auth_enabled: boolean;
  shared_secret: string;
  tls_enabled: boolean;
  tls_cert_path: string;
  tls_key_path: string;
  bundle_default_device_group: string;
  bundle_overlay_provider: string;
  bundle_overlay_join_config_json: string;
  bundle_service_mode: string;
  bundle_service_host: string;
  bundle_service_port: number;
  bundle_use_tls: boolean;
  bundle_tls_server_name: string;
};

export type ServerStatus = {
  running: boolean;
  addr?: string;
  root_dir?: string;
  remote_build?: boolean;
  last_error?: string;
  installing?: boolean;
};

export type ToolInfo = { installed: boolean; path?: string; version?: string; error?: string };
export type EnvCheck = {
  os: string;
  winget_installed: boolean;
  tools: Record<string, ToolInfo>;
};

export type FileStatSummary = { path: string; exists: boolean; size_bytes: number; modified_at_unix: number };
export type CheckpointStatus = {
  last_checkpoint_at_unix: number;
  mode: string;
  busy_readers: number;
  log_frames: number;
  checkpointed_frames: number;
  last_error: string;
};
export type BackupStatus = {
  dir: string;
  interval_seconds: number;
  keep_latest: number;
  last_backup_at_unix: number;
  last_backup_path: string;
  last_error: string;
};

export type WorkbenchRuntime = {
  server_id: string;
  listen_addr: string;
  root_dir: string;
  db_path: string;
  tls_enabled: boolean;
  auth_enabled: boolean;
  started_at_unix: number;
  health_state: string;
  health_message: string;
  db_file: FileStatSummary;
  wal_file: FileStatSummary;
  shm_file: FileStatSummary;
  checkpoint: CheckpointStatus;
  backup: BackupStatus;
};

export type DeviceSummary = {
  device_id: string;
  display_name: string;
  role: string;
  overlay_provider: string;
  overlay_address: string;
  online_state: string;
  last_seen_at: number;
  sync_state: string;
  mount_state: string;
  client_version: string;
  policy_revision: number;
};

export type WorkbenchSnapshot = {
  runtime: WorkbenchRuntime | null;
  devices: DeviceSummary[];
  collected_at_unix: number;
  query_error?: string | null;
};

export type HotPathMetric = { path: string; count: number };
export type RPCMetric = { method: string; count: number; error_count: number; p50_ms: number; p95_ms: number; p99_ms: number };
export type BuildObservability = {
  success_count: number;
  failure_count: number;
  log_bytes: number;
  queue_wait_count: number;
  queue_wait_p50_ms: number;
  queue_wait_p95_ms: number;
  queue_wait_p99_ms: number;
  duration_count: number;
  duration_p50_ms: number;
  duration_p95_ms: number;
  duration_p99_ms: number;
};
export type WorkbenchObservabilitySnapshot = {
  write_file_range_calls: number;
  write_file_range_bytes: number;
  write_file_range_conflicts: number;
  small_write_bursts: number;
  small_write_hot_paths: HotPathMetric[];
  build: BuildObservability;
  rpc_metrics: RPCMetric[];
  collected_at_unix: number;
};

export type TLSStatus = {
  cert_path: string;
  key_path: string;
  root_cert_path: string;
  root_key_path: string;
  server_cert_exists: boolean;
  server_key_exists: boolean;
  root_cert_exists: boolean;
  root_key_exists: boolean;
  server_subject: string;
  root_subject: string;
  server_dns_names: string[];
  server_not_before_unix: number;
  server_not_after_unix: number;
  root_not_before_unix: number;
  root_not_after_unix: number;
  root_is_ca: boolean;
  server_valid: boolean;
  root_valid: boolean;
  overall_valid: boolean;
};

export type BackupTriggerResult = { created_at_unix: number; path: string };
export type ExportClientCAResult = { root_cert_path: string; exported_path: string };
export type JoinBundleRequest = { device_id: string; device_name: string; device_role: string; device_group: string };
export type JoinBundleView = {
  version: number;
  overlay_provider: string;
  overlay_join_config_json: string;
  service_discovery_mode: string;
  service_host: string;
  service_port: number;
  use_tls: boolean;
  tls_server_name: string;
  server_id: string;
  device_group: string;
  shared_secret: string;
  device_id: string;
  device_name: string;
  device_role: string;
};
export type IssueJoinBundleResult = { bundle_json: string; bundle: JoinBundleView };
export type ExportClientAccessResult = { export_dir: string; bundle_path: string; ca_path?: string | null };

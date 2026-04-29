import type {
  AppConfig,
  JoinBundleRequest,
  TLSStatus,
  WorkbenchObservabilitySnapshot,
  WorkbenchSnapshot
} from "./types";

export const defaultConfig: AppConfig = {
  addr: ":50051",
  data_root: "",
  root_dir: "",
  remote_build_enabled: true,
  build_tool_dirs: [],
  required_build_tools: ["cmake", "make", "build-essential"],
  auth_enabled: false,
  shared_secret: "",
  tls_enabled: false,
  tls_cert_path: "certs/roodox-server-cert.pem",
  tls_key_path: "certs/roodox-server-key.pem",
  bundle_default_device_group: "default",
  bundle_overlay_provider: "",
  bundle_overlay_join_config_json: "{}",
  bundle_service_mode: "static",
  bundle_service_host: "",
  bundle_service_port: 50051,
  bundle_use_tls: false,
  bundle_tls_server_name: ""
};

export const emptySnapshot: WorkbenchSnapshot = { runtime: null, devices: [], collected_at_unix: 0, query_error: null };

export const emptyObservability: WorkbenchObservabilitySnapshot = {
  write_file_range_calls: 0,
  write_file_range_bytes: 0,
  write_file_range_conflicts: 0,
  small_write_bursts: 0,
  small_write_hot_paths: [],
  build: {
    success_count: 0,
    failure_count: 0,
    log_bytes: 0,
    queue_wait_count: 0,
    queue_wait_p50_ms: 0,
    queue_wait_p95_ms: 0,
    queue_wait_p99_ms: 0,
    duration_count: 0,
    duration_p50_ms: 0,
    duration_p95_ms: 0,
    duration_p99_ms: 0
  },
  rpc_metrics: [],
  collected_at_unix: 0
};

export const emptyTLS: TLSStatus = {
  cert_path: "",
  key_path: "",
  root_cert_path: "",
  root_key_path: "",
  server_cert_exists: false,
  server_key_exists: false,
  root_cert_exists: false,
  root_key_exists: false,
  server_subject: "",
  root_subject: "",
  server_dns_names: [],
  server_not_before_unix: 0,
  server_not_after_unix: 0,
  root_not_before_unix: 0,
  root_not_after_unix: 0,
  root_is_ca: false,
  server_valid: false,
  root_valid: false,
  overall_valid: false
};

export const defaultJoinRequest: JoinBundleRequest = {
  device_id: "",
  device_name: "",
  device_role: "",
  device_group: ""
};

export const defaultClientCAExportPath = "artifacts/handoff/roodox-ca-cert.pem";
export const defaultClientAccessExportDir = "artifacts/handoff/client-access";

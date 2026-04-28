use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AppConfig {
    pub addr: String,
    pub data_root: String,
    pub root_dir: String,
    pub remote_build_enabled: bool,
    pub build_tool_dirs: Vec<String>,
    pub required_build_tools: Vec<String>,
    pub auth_enabled: bool,
    pub shared_secret: String,
    pub tls_enabled: bool,
    pub tls_cert_path: String,
    pub tls_key_path: String,
    pub bundle_default_device_group: String,
    pub bundle_overlay_provider: String,
    pub bundle_overlay_join_config_json: String,
    pub bundle_service_mode: String,
    pub bundle_service_host: String,
    pub bundle_service_port: u32,
    pub bundle_use_tls: bool,
    pub bundle_tls_server_name: String,
}

impl Default for AppConfig {
    fn default() -> Self {
        Self {
            addr: ":50051".to_string(),
            data_root: String::new(),
            root_dir: String::new(),
            remote_build_enabled: true,
            build_tool_dirs: Vec::new(),
            required_build_tools: vec![
                "cmake".to_string(),
                "make".to_string(),
                "build-essential".to_string(),
            ],
            auth_enabled: false,
            shared_secret: String::new(),
            tls_enabled: false,
            tls_cert_path: "certs/roodox-server-cert.pem".to_string(),
            tls_key_path: "certs/roodox-server-key.pem".to_string(),
            bundle_default_device_group: "default".to_string(),
            bundle_overlay_provider: String::new(),
            bundle_overlay_join_config_json: "{}".to_string(),
            bundle_service_mode: "static".to_string(),
            bundle_service_host: String::new(),
            bundle_service_port: 50051,
            bundle_use_tls: false,
            bundle_tls_server_name: String::new(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WorkbenchRuntime {
    pub server_id: String,
    pub listen_addr: String,
    pub root_dir: String,
    pub db_path: String,
    pub tls_enabled: bool,
    pub auth_enabled: bool,
    pub started_at_unix: i64,
    pub health_state: String,
    pub health_message: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DeviceSummaryView {
    pub device_id: String,
    pub display_name: String,
    pub role: String,
    pub overlay_provider: String,
    pub overlay_address: String,
    pub online_state: String,
    pub last_seen_at: i64,
    pub sync_state: String,
    pub mount_state: String,
    pub client_version: String,
    pub policy_revision: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WorkbenchSnapshot {
    pub runtime: Option<WorkbenchRuntime>,
    pub devices: Vec<DeviceSummaryView>,
    pub collected_at_unix: i64,
    pub query_error: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WorkbenchHotPathMetric {
    pub path: String,
    pub count: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WorkbenchRPCMetric {
    pub method: String,
    pub count: i64,
    pub error_count: i64,
    pub p50_ms: i64,
    pub p95_ms: i64,
    pub p99_ms: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WorkbenchBuildObservability {
    pub success_count: i64,
    pub failure_count: i64,
    pub log_bytes: i64,
    pub queue_wait_count: i64,
    pub queue_wait_p50_ms: i64,
    pub queue_wait_p95_ms: i64,
    pub queue_wait_p99_ms: i64,
    pub duration_count: i64,
    pub duration_p50_ms: i64,
    pub duration_p95_ms: i64,
    pub duration_p99_ms: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WorkbenchObservabilitySnapshot {
    pub write_file_range_calls: i64,
    pub write_file_range_bytes: i64,
    pub write_file_range_conflicts: i64,
    pub small_write_bursts: i64,
    pub small_write_hot_paths: Vec<WorkbenchHotPathMetric>,
    pub build: WorkbenchBuildObservability,
    pub rpc_metrics: Vec<WorkbenchRPCMetric>,
    pub collected_at_unix: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TLSStatus {
    pub cert_path: String,
    pub key_path: String,
    pub root_cert_path: String,
    pub root_key_path: String,
    pub server_cert_exists: bool,
    pub server_key_exists: bool,
    pub root_cert_exists: bool,
    pub root_key_exists: bool,
    pub server_subject: String,
    pub root_subject: String,
    pub server_dns_names: Vec<String>,
    pub server_not_before_unix: i64,
    pub server_not_after_unix: i64,
    pub root_not_before_unix: i64,
    pub root_not_after_unix: i64,
    pub root_is_ca: bool,
    pub server_valid: bool,
    pub root_valid: bool,
    pub overall_valid: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BackupTriggerResult {
    pub created_at_unix: i64,
    pub path: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ExportClientCAResult {
    pub root_cert_path: String,
    pub exported_path: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct JoinBundleRequest {
    pub device_id: String,
    pub device_name: String,
    pub device_role: String,
    pub device_group: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct JoinBundleView {
    pub version: u32,
    pub overlay_provider: String,
    pub overlay_join_config_json: String,
    pub service_discovery_mode: String,
    pub service_host: String,
    pub service_port: u32,
    pub use_tls: bool,
    pub tls_server_name: String,
    pub server_id: String,
    pub device_group: String,
    pub shared_secret: String,
    pub device_id: String,
    pub device_name: String,
    pub device_role: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IssueJoinBundleResult {
    pub bundle_json: String,
    pub bundle: JoinBundleView,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ExportClientAccessResult {
    pub export_dir: String,
    pub bundle_path: String,
    pub ca_path: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct ServerStatus {
    pub running: bool,
    pub addr: Option<String>,
    pub root_dir: Option<String>,
    pub remote_build: Option<bool>,
    pub last_error: Option<String>,
    pub installing: bool,
}

#[derive(Debug, Serialize)]
pub struct ToolInfo {
    pub installed: bool,
    pub path: Option<String>,
    pub version: Option<String>,
    pub error: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct EnvCheck {
    pub os: String,
    pub winget_installed: bool,
    pub tools: HashMap<String, ToolInfo>,
    pub recommended_tool_dirs: Vec<String>,
    pub config_tool_dirs: Vec<String>,
}

#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use once_cell::sync::Lazy;
use serde::{Deserialize, Serialize};
use serde_json::{Map, Value};
use std::collections::HashSet;
use std::fs;
use std::io::{BufRead, BufReader};
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use std::thread;

static CHILD: Lazy<Mutex<Option<Child>>> = Lazy::new(|| Mutex::new(None));
static LOGS: Lazy<Mutex<Vec<String>>> = Lazy::new(|| Mutex::new(Vec::new()));
static INSTALLING: Lazy<Mutex<bool>> = Lazy::new(|| Mutex::new(false));
const BOOTSTRAP_FILE_NAME: &str = "roodox-workbench.bootstrap.json";

#[derive(Debug, Clone, Serialize, Deserialize)]
struct AppConfig {
    addr: String,
    data_root: String,
    root_dir: String,
    remote_build_enabled: bool,
    build_tool_dirs: Vec<String>,
    required_build_tools: Vec<String>,
    auth_enabled: bool,
    shared_secret: String,
    tls_enabled: bool,
    tls_cert_path: String,
    tls_key_path: String,
    bundle_default_device_group: String,
    bundle_overlay_provider: String,
    bundle_overlay_join_config_json: String,
    bundle_service_mode: String,
    bundle_service_host: String,
    bundle_service_port: u32,
    bundle_use_tls: bool,
    bundle_tls_server_name: String,
}

#[derive(Debug, Clone, Deserialize)]
struct BootstrapConfig {
    project_root: Option<String>,
    config_path: Option<String>,
}

#[derive(Debug, Serialize)]
struct ServerStatus {
    running: bool,
    addr: Option<String>,
    root_dir: Option<String>,
    remote_build: Option<bool>,
    last_error: Option<String>,
    installing: bool,
}

#[derive(Debug, Serialize)]
struct ToolInfo {
    installed: bool,
    path: Option<String>,
    version: Option<String>,
    error: Option<String>,
}

#[derive(Debug, Serialize)]
struct EnvCheck {
    os: String,
    winget_installed: bool,
    tools: std::collections::HashMap<String, ToolInfo>,
    recommended_tool_dirs: Vec<String>,
    config_tool_dirs: Vec<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct WorkbenchRuntime {
    server_id: String,
    listen_addr: String,
    root_dir: String,
    db_path: String,
    tls_enabled: bool,
    auth_enabled: bool,
    started_at_unix: i64,
    health_state: String,
    health_message: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct DeviceSummaryView {
    device_id: String,
    display_name: String,
    role: String,
    overlay_provider: String,
    overlay_address: String,
    online_state: String,
    last_seen_at: i64,
    sync_state: String,
    mount_state: String,
    client_version: String,
    policy_revision: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct WorkbenchSnapshot {
    runtime: Option<WorkbenchRuntime>,
    devices: Vec<DeviceSummaryView>,
    collected_at_unix: i64,
    query_error: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct WorkbenchHotPathMetric {
    path: String,
    count: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct WorkbenchRPCMetric {
    method: String,
    count: i64,
    error_count: i64,
    p50_ms: i64,
    p95_ms: i64,
    p99_ms: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct WorkbenchBuildObservability {
    success_count: i64,
    failure_count: i64,
    log_bytes: i64,
    queue_wait_count: i64,
    queue_wait_p50_ms: i64,
    queue_wait_p95_ms: i64,
    queue_wait_p99_ms: i64,
    duration_count: i64,
    duration_p50_ms: i64,
    duration_p95_ms: i64,
    duration_p99_ms: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct WorkbenchObservabilitySnapshot {
    write_file_range_calls: i64,
    write_file_range_bytes: i64,
    write_file_range_conflicts: i64,
    small_write_bursts: i64,
    small_write_hot_paths: Vec<WorkbenchHotPathMetric>,
    build: WorkbenchBuildObservability,
    rpc_metrics: Vec<WorkbenchRPCMetric>,
    collected_at_unix: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct TLSStatus {
    cert_path: String,
    key_path: String,
    root_cert_path: String,
    root_key_path: String,
    server_cert_exists: bool,
    server_key_exists: bool,
    root_cert_exists: bool,
    root_key_exists: bool,
    server_subject: String,
    root_subject: String,
    server_dns_names: Vec<String>,
    server_not_before_unix: i64,
    server_not_after_unix: i64,
    root_not_before_unix: i64,
    root_not_after_unix: i64,
    root_is_ca: bool,
    server_valid: bool,
    root_valid: bool,
    overall_valid: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct BackupTriggerResult {
    created_at_unix: i64,
    path: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct ExportClientCAResult {
    root_cert_path: String,
    exported_path: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
struct JoinBundleRequest {
    device_id: String,
    device_name: String,
    device_role: String,
    device_group: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct JoinBundleView {
    version: u32,
    overlay_provider: String,
    overlay_join_config_json: String,
    service_discovery_mode: String,
    service_host: String,
    service_port: u32,
    use_tls: bool,
    tls_server_name: String,
    server_id: String,
    device_group: String,
    shared_secret: String,
    device_id: String,
    device_name: String,
    device_role: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct IssueJoinBundleResult {
    bundle_json: String,
    bundle: JoinBundleView,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct ExportClientAccessResult {
    export_dir: String,
    bundle_path: String,
    ca_path: Option<String>,
}

fn default_config() -> AppConfig {
    AppConfig {
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

fn push_log(line: impl Into<String>) {
    if let Ok(mut logs) = LOGS.lock() {
        logs.push(line.into());
        if logs.len() > 800 {
            let keep_from = logs.len() - 800;
            let keep = logs.split_off(keep_from);
            *logs = keep;
        }
    }
}

fn now_hms() -> String {
    let secs = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs() % 86_400)
        .unwrap_or(0);
    let h = secs / 3600;
    let m = (secs % 3600) / 60;
    let s = secs % 60;
    format!("{:02}:{:02}:{:02}", h, m, s)
}

fn push_candidate(candidates: &mut Vec<PathBuf>, candidate: PathBuf) {
    if !candidates.iter().any(|existing| existing == &candidate) {
        candidates.push(candidate);
    }
}

fn add_candidate_ancestors(candidates: &mut Vec<PathBuf>, start: &Path, max_depth: usize) {
    let mut current = Some(start);
    let mut depth = 0usize;
    while let Some(dir) = current {
        push_candidate(candidates, dir.to_path_buf());
        if depth >= max_depth {
            break;
        }
        current = dir.parent();
        depth += 1;
    }
}

fn config_path_from_env() -> Option<PathBuf> {
    ["ROODOX_WORKBENCH_CONFIG_PATH", "ROODOX_CONFIG_PATH"]
        .iter()
        .find_map(|key| {
            let value = std::env::var(key).ok()?;
            let trimmed = value.trim();
            if trimmed.is_empty() {
                None
            } else {
                Some(PathBuf::from(trimmed))
            }
        })
}

fn bootstrap_path() -> Option<PathBuf> {
    let exe = std::env::current_exe().ok()?;
    let dir = exe.parent()?;
    Some(dir.join(BOOTSTRAP_FILE_NAME))
}

fn read_bootstrap() -> Option<BootstrapConfig> {
    let path = bootstrap_path()?;
    if !path.exists() {
        return None;
    }
    let text = fs::read_to_string(path).ok()?;
    serde_json::from_str(&text).ok()
}

fn config_path_from_bootstrap() -> Option<PathBuf> {
    let bootstrap = read_bootstrap()?;
    let path = bootstrap.config_path?;
    let trimmed = path.trim();
    if trimmed.is_empty() {
        None
    } else {
        Some(PathBuf::from(trimmed))
    }
}

fn project_root_from_env() -> Option<PathBuf> {
    let value = std::env::var("ROODOX_WORKBENCH_ROOT").ok()?;
    let trimmed = value.trim();
    if trimmed.is_empty() {
        None
    } else {
        Some(PathBuf::from(trimmed))
    }
}

fn project_root_from_bootstrap() -> Option<PathBuf> {
    let bootstrap = read_bootstrap()?;
    let path = bootstrap.project_root?;
    let trimmed = path.trim();
    if trimmed.is_empty() {
        None
    } else {
        Some(PathBuf::from(trimmed))
    }
}

fn candidate_search_roots() -> Vec<PathBuf> {
    let cwd = std::env::current_dir().unwrap_or_else(|_| PathBuf::from("."));
    let exe_dir = std::env::current_exe()
        .ok()
        .and_then(|p| p.parent().map(|v| v.to_path_buf()))
        .unwrap_or_else(|| cwd.clone());

    let mut candidates = Vec::new();
    add_candidate_ancestors(&mut candidates, &cwd, 8);
    add_candidate_ancestors(&mut candidates, &exe_dir, 10);
    if let Some(root) = project_root_from_env() {
        add_candidate_ancestors(&mut candidates, &root, 2);
    }
    candidates
}

fn discover_config_path() -> Option<PathBuf> {
    if let Some(path) = config_path_from_env() {
        return Some(path);
    }
    if let Some(path) = config_path_from_bootstrap() {
        return Some(path);
    }

    for candidate in candidate_search_roots() {
        let config = candidate.join("roodox.config.json");
        if config.exists() {
            return Some(config);
        }
    }
    None
}

fn project_root() -> PathBuf {
    if let Some(root) = project_root_from_env() {
        return root;
    }
    if let Some(root) = project_root_from_bootstrap() {
        return root;
    }
    if let Some(config) = discover_config_path() {
        if let Some(parent) = config.parent() {
            return parent.to_path_buf();
        }
    }
    for candidate in candidate_search_roots() {
        if candidate.join("go.mod").exists() && candidate.join("cmd").join("roodox_server").exists()
        {
            return candidate;
        }
        if candidate.join("roodox_server.exe").exists() {
            return candidate;
        }
    }
    std::env::current_dir().unwrap_or_else(|_| PathBuf::from("."))
}

fn config_path() -> PathBuf {
    discover_config_path().unwrap_or_else(|| project_root().join("roodox.config.json"))
}

fn normalize_list(input: Vec<String>) -> Vec<String> {
    let mut seen = HashSet::new();
    let mut out = Vec::new();
    for item in input {
        let trimmed = item.trim();
        if trimmed.is_empty() {
            continue;
        }
        let key = trimmed.to_lowercase();
        if seen.insert(key) {
            out.push(trimmed.to_string());
        }
    }
    out
}

fn parse_discovery_addr(addr: &str) -> (String, u32) {
    let trimmed = addr.trim();
    if trimmed.is_empty() {
        return (String::new(), 50051);
    }
    if let Some(port_text) = trimmed.strip_prefix(':') {
        return (
            String::new(),
            port_text.trim().parse::<u32>().unwrap_or(50051),
        );
    }
    if let Some(rest) = trimmed.strip_prefix('[') {
        if let Some((host_part, port_part)) = rest.split_once("]:") {
            return (
                host_part.trim().to_string(),
                port_part.trim().parse::<u32>().unwrap_or(50051),
            );
        }
    }
    if let Some((host_part, port_part)) = trimmed.rsplit_once(':') {
        let host = host_part.trim();
        let host = match host {
            "" | "0.0.0.0" | "::" => "",
            value => value,
        };
        return (
            host.trim_matches(|c| c == '[' || c == ']').to_string(),
            port_part.trim().parse::<u32>().unwrap_or(50051),
        );
    }
    (trimmed.to_string(), 50051)
}

fn get_nested_value<'a>(value: &'a Value, path: &[&str]) -> Option<&'a Value> {
    let mut current = value;
    for key in path {
        current = current.get(*key)?;
    }
    Some(current)
}

fn read_nested_string(value: &Value, path: &[&str], default: &str) -> String {
    get_nested_value(value, path)
        .and_then(Value::as_str)
        .unwrap_or(default)
        .to_string()
}

fn read_nested_bool(value: &Value, path: &[&str], default: bool) -> bool {
    get_nested_value(value, path)
        .and_then(Value::as_bool)
        .unwrap_or(default)
}

fn read_nested_u32(value: &Value, path: &[&str], default: u32) -> u32 {
    get_nested_value(value, path)
        .and_then(Value::as_u64)
        .and_then(|v| u32::try_from(v).ok())
        .unwrap_or(default)
}

fn ensure_object_mut<'a>(
    value: &'a mut Value,
    path: &[&str],
) -> Result<&'a mut Map<String, Value>, String> {
    if path.is_empty() {
        return value
            .as_object_mut()
            .ok_or_else(|| "config root is not a JSON object".to_string());
    }

    if !value.is_object() {
        *value = Value::Object(Map::new());
    }
    let mut current = value
        .as_object_mut()
        .ok_or_else(|| "config root is not a JSON object".to_string())?;
    for key in path {
        let entry = current
            .entry((*key).to_string())
            .or_insert_with(|| Value::Object(Map::new()));
        if !entry.is_object() {
            *entry = Value::Object(Map::new());
        }
        current = entry
            .as_object_mut()
            .ok_or_else(|| format!("config path is not an object: {}", path.join(".")))?;
    }
    Ok(current)
}

fn normalize_config(mut cfg: AppConfig) -> AppConfig {
    let (_, default_port) = parse_discovery_addr(&cfg.addr);
    cfg.data_root = cfg.data_root.trim().to_string();
    if cfg.addr.trim().is_empty() {
        cfg.addr = ":50051".to_string();
    }
    cfg.root_dir = cfg.root_dir.trim().to_string();
    cfg.build_tool_dirs = normalize_list(cfg.build_tool_dirs);
    cfg.required_build_tools = normalize_list(cfg.required_build_tools);
    if cfg.required_build_tools.is_empty() {
        cfg.required_build_tools = vec![
            "cmake".to_string(),
            "make".to_string(),
            "build-essential".to_string(),
        ];
    }
    cfg.shared_secret = cfg.shared_secret.trim().to_string();
    if cfg.tls_cert_path.trim().is_empty() {
        cfg.tls_cert_path = "certs/roodox-server-cert.pem".to_string();
    }
    if cfg.tls_key_path.trim().is_empty() {
        cfg.tls_key_path = "certs/roodox-server-key.pem".to_string();
    }
    cfg.bundle_default_device_group = cfg.bundle_default_device_group.trim().to_string();
    if cfg.bundle_default_device_group.is_empty() {
        cfg.bundle_default_device_group = "default".to_string();
    }
    cfg.bundle_overlay_provider = cfg.bundle_overlay_provider.trim().to_string();
    cfg.bundle_overlay_join_config_json = cfg.bundle_overlay_join_config_json.trim().to_string();
    if cfg.bundle_overlay_join_config_json.is_empty() {
        cfg.bundle_overlay_join_config_json = "{}".to_string();
    }
    cfg.bundle_service_mode = cfg.bundle_service_mode.trim().to_string();
    if cfg.bundle_service_mode.is_empty() {
        cfg.bundle_service_mode = "static".to_string();
    }
    cfg.bundle_service_host = cfg.bundle_service_host.trim().to_string();
    if cfg.bundle_service_port == 0 {
        cfg.bundle_service_port = default_port.max(1);
    }
    cfg.bundle_tls_server_name = cfg.bundle_tls_server_name.trim().to_string();
    cfg
}

fn read_config_value(path: &Path) -> Result<Value, String> {
    if !path.exists() {
        return Ok(Value::Object(Map::new()));
    }
    let text = fs::read_to_string(path).map_err(|e| format!("read config failed: {e}"))?;
    serde_json::from_str(&text).map_err(|e| format!("parse config failed: {e}"))
}

fn read_string(value: &Value, key: &str, default: &str) -> String {
    value
        .get(key)
        .and_then(Value::as_str)
        .unwrap_or(default)
        .to_string()
}

fn read_bool(value: &Value, key: &str, default: bool) -> bool {
    value.get(key).and_then(Value::as_bool).unwrap_or(default)
}

fn read_string_list(value: &Value, key: &str, default: &[&str]) -> Vec<String> {
    let items = value
        .get(key)
        .and_then(Value::as_array)
        .map(|arr| {
            arr.iter()
                .filter_map(Value::as_str)
                .map(ToString::to_string)
                .collect::<Vec<_>>()
        })
        .unwrap_or_else(|| default.iter().map(|item| item.to_string()).collect());
    normalize_list(items)
}

fn extract_app_config(value: &Value) -> AppConfig {
    let default = default_config();
    let addr = read_string(value, "addr", &default.addr);
    let (derived_host, derived_port) = parse_discovery_addr(&addr);
    let tls_enabled = read_bool(value, "tls_enabled", default.tls_enabled);
    normalize_config(AppConfig {
        addr,
        data_root: read_string(value, "data_root", &default.data_root),
        root_dir: read_string(value, "root_dir", &default.root_dir),
        remote_build_enabled: read_bool(
            value,
            "remote_build_enabled",
            default.remote_build_enabled,
        ),
        build_tool_dirs: read_string_list(value, "build_tool_dirs", &[]),
        required_build_tools: read_string_list(
            value,
            "required_build_tools",
            &["cmake", "make", "build-essential"],
        ),
        auth_enabled: read_bool(value, "auth_enabled", default.auth_enabled),
        shared_secret: read_string(value, "shared_secret", &default.shared_secret),
        tls_enabled,
        tls_cert_path: read_string(value, "tls_cert_path", &default.tls_cert_path),
        tls_key_path: read_string(value, "tls_key_path", &default.tls_key_path),
        bundle_default_device_group: read_nested_string(
            value,
            &["control_plane", "default_device_group"],
            &default.bundle_default_device_group,
        ),
        bundle_overlay_provider: read_nested_string(
            value,
            &["control_plane", "join_bundle", "overlay_provider"],
            &default.bundle_overlay_provider,
        ),
        bundle_overlay_join_config_json: read_nested_string(
            value,
            &["control_plane", "join_bundle", "overlay_join_config_json"],
            &default.bundle_overlay_join_config_json,
        ),
        bundle_service_mode: read_nested_string(
            value,
            &["control_plane", "join_bundle", "service_discovery", "mode"],
            &default.bundle_service_mode,
        ),
        bundle_service_host: read_nested_string(
            value,
            &["control_plane", "join_bundle", "service_discovery", "host"],
            &derived_host,
        ),
        bundle_service_port: read_nested_u32(
            value,
            &["control_plane", "join_bundle", "service_discovery", "port"],
            derived_port.max(1),
        ),
        bundle_use_tls: read_nested_bool(
            value,
            &[
                "control_plane",
                "join_bundle",
                "service_discovery",
                "use_tls",
            ],
            tls_enabled,
        ),
        bundle_tls_server_name: read_nested_string(
            value,
            &[
                "control_plane",
                "join_bundle",
                "service_discovery",
                "tls_server_name",
            ],
            &default.bundle_tls_server_name,
        ),
    })
}

fn read_config_file() -> Result<AppConfig, String> {
    let path = config_path();
    let value = read_config_value(&path)?;
    Ok(extract_app_config(&value))
}

fn write_config_file(cfg: AppConfig) -> Result<AppConfig, String> {
    let path = config_path();
    let normalized = normalize_config(cfg);
    let mut value = read_config_value(&path)?;
    if !value.is_object() {
        value = Value::Object(Map::new());
    }
    {
        let object = value
            .as_object_mut()
            .ok_or_else(|| "config root is not a JSON object".to_string())?;
        object.insert("addr".to_string(), Value::String(normalized.addr.clone()));
        object.insert(
            "data_root".to_string(),
            Value::String(normalized.data_root.clone()),
        );
        object.insert(
            "root_dir".to_string(),
            Value::String(normalized.root_dir.clone()),
        );
        object.insert(
            "remote_build_enabled".to_string(),
            Value::Bool(normalized.remote_build_enabled),
        );
        object.insert(
            "build_tool_dirs".to_string(),
            Value::Array(
                normalized
                    .build_tool_dirs
                    .iter()
                    .cloned()
                    .map(Value::String)
                    .collect(),
            ),
        );
        object.insert(
            "required_build_tools".to_string(),
            Value::Array(
                normalized
                    .required_build_tools
                    .iter()
                    .cloned()
                    .map(Value::String)
                    .collect(),
            ),
        );
        object.insert(
            "auth_enabled".to_string(),
            Value::Bool(normalized.auth_enabled),
        );
        object.insert(
            "shared_secret".to_string(),
            Value::String(normalized.shared_secret.clone()),
        );
        object.insert(
            "tls_enabled".to_string(),
            Value::Bool(normalized.tls_enabled),
        );
        object.insert(
            "tls_cert_path".to_string(),
            Value::String(normalized.tls_cert_path.clone()),
        );
        object.insert(
            "tls_key_path".to_string(),
            Value::String(normalized.tls_key_path.clone()),
        );
    }

    let control_plane = ensure_object_mut(&mut value, &["control_plane"])?;
    control_plane.insert(
        "default_device_group".to_string(),
        Value::String(normalized.bundle_default_device_group.clone()),
    );
    let join_bundle = ensure_object_mut(&mut value, &["control_plane", "join_bundle"])?;
    join_bundle.insert(
        "overlay_provider".to_string(),
        Value::String(normalized.bundle_overlay_provider.clone()),
    );
    join_bundle.insert(
        "overlay_join_config_json".to_string(),
        Value::String(normalized.bundle_overlay_join_config_json.clone()),
    );
    let service_discovery = ensure_object_mut(
        &mut value,
        &["control_plane", "join_bundle", "service_discovery"],
    )?;
    service_discovery.insert(
        "mode".to_string(),
        Value::String(normalized.bundle_service_mode.clone()),
    );
    service_discovery.insert(
        "host".to_string(),
        Value::String(normalized.bundle_service_host.clone()),
    );
    service_discovery.insert(
        "port".to_string(),
        Value::Number(normalized.bundle_service_port.into()),
    );
    service_discovery.insert(
        "use_tls".to_string(),
        Value::Bool(normalized.bundle_use_tls),
    );
    service_discovery.insert(
        "tls_server_name".to_string(),
        Value::String(normalized.bundle_tls_server_name.clone()),
    );

    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).map_err(|e| format!("create config dir failed: {e}"))?;
    }
    let text =
        serde_json::to_string_pretty(&value).map_err(|e| format!("encode config failed: {e}"))?;
    fs::write(&path, text).map_err(|e| format!("write config failed: {e}"))?;
    Ok(normalized)
}

fn runtime_binary_path_from_value(value: &Value, config_path: &Path) -> Option<PathBuf> {
    let binary = value
        .get("runtime")
        .and_then(Value::as_object)
        .and_then(|runtime| runtime.get("binary_path"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())?;

    let path = PathBuf::from(binary);
    if path.is_absolute() {
        Some(path)
    } else {
        Some(config_path.parent().unwrap_or(Path::new(".")).join(path))
    }
}

fn runtime_state_dir_from_value(value: &Value, config_path: &Path) -> PathBuf {
    let data_root = value
        .get("data_root")
        .and_then(Value::as_str)
        .map(str::trim)
        .unwrap_or("");
    let state_dir = value
        .get("runtime")
        .and_then(Value::as_object)
        .and_then(|runtime| runtime.get("state_dir"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
        .unwrap_or_else(|| {
            if data_root.is_empty() {
                "runtime".to_string()
            } else {
                format!("{data_root}/runtime")
            }
        });

    let path = PathBuf::from(state_dir);
    if path.is_absolute() {
        path
    } else {
        config_path.parent().unwrap_or(Path::new(".")).join(path)
    }
}

fn server_binary_path(config_path: &Path) -> Option<PathBuf> {
    let config_value = read_config_value(config_path).unwrap_or_else(|_| Value::Object(Map::new()));
    if let Some(binary) = runtime_binary_path_from_value(&config_value, config_path) {
        return Some(binary);
    }

    let root = project_root();
    let server_exe = root.join("roodox_server.exe");
    if server_exe.exists()
        || (root.join("go.mod").exists() && root.join("cmd").join("roodox_server").exists())
    {
        Some(server_exe)
    } else {
        None
    }
}

fn newer_time(target: &mut Option<std::time::SystemTime>, candidate: std::time::SystemTime) {
    match target {
        Some(current) if *current >= candidate => {}
        _ => *target = Some(candidate),
    }
}

fn newest_timestamp_in_dir(dir: &Path) -> Option<std::time::SystemTime> {
    let entries = fs::read_dir(dir).ok()?;
    let mut latest = None;
    for entry in entries.flatten() {
        let path = entry.path();
        let metadata = match entry.metadata() {
            Ok(value) => value,
            Err(_) => continue,
        };
        if metadata.is_dir() {
            if let Some(candidate) = newest_timestamp_in_dir(&path) {
                newer_time(&mut latest, candidate);
            }
            continue;
        }
        let ext = path
            .extension()
            .and_then(|value| value.to_str())
            .map(|value| value.to_ascii_lowercase());
        if !matches!(ext.as_deref(), Some("go") | Some("proto")) {
            continue;
        }
        if let Ok(modified) = metadata.modified() {
            newer_time(&mut latest, modified);
        }
    }
    latest
}

fn latest_server_source_timestamp(root: &Path) -> Option<std::time::SystemTime> {
    if !(root.join("go.mod").exists() && root.join("cmd").join("roodox_server").exists()) {
        return None;
    }

    let mut latest = None;
    for file_name in ["go.mod", "go.sum"] {
        let path = root.join(file_name);
        if let Ok(metadata) = fs::metadata(path) {
            if let Ok(modified) = metadata.modified() {
                newer_time(&mut latest, modified);
            }
        }
    }
    for dir_name in ["cmd", "internal", "proto"] {
        let dir = root.join(dir_name);
        if let Some(candidate) = newest_timestamp_in_dir(&dir) {
            newer_time(&mut latest, candidate);
        }
    }
    latest
}

fn server_binary_is_stale(config_path: &Path) -> Result<bool, String> {
    let binary_path = match server_binary_path(config_path) {
        Some(path) => path,
        None => return Ok(false),
    };
    if !binary_path.exists() {
        return Ok(true);
    }

    let root = project_root();
    let source_timestamp = match latest_server_source_timestamp(&root) {
        Some(value) => value,
        None => return Ok(false),
    };
    let binary_timestamp = fs::metadata(&binary_path)
        .and_then(|metadata| metadata.modified())
        .map_err(|e| format!("read server binary timestamp failed: {e}"))?;
    Ok(binary_timestamp < source_timestamp)
}

fn ensure_server_binary_current(config_path: &Path) -> Result<Option<PathBuf>, String> {
    let binary_path = match server_binary_path(config_path) {
        Some(path) => path,
        None => return Ok(None),
    };

    let root = project_root();
    if latest_server_source_timestamp(&root).is_none() {
        return Ok(Some(binary_path));
    }

    let needs_build = !binary_path.exists() || server_binary_is_stale(config_path)?;
    if !needs_build {
        return Ok(Some(binary_path));
    }

    if let Some(parent) = binary_path.parent() {
        fs::create_dir_all(parent).map_err(|e| format!("create server binary dir failed: {e}"))?;
    }

    let config_value = read_config_value(config_path).unwrap_or_else(|_| Value::Object(Map::new()));
    let state_dir = runtime_state_dir_from_value(&config_value, config_path);
    let go_cache = state_dir.join(".gocache");
    let go_mod_cache = state_dir.join(".gomodcache");
    fs::create_dir_all(&go_cache).map_err(|e| format!("create go cache dir failed: {e}"))?;
    fs::create_dir_all(&go_mod_cache)
        .map_err(|e| format!("create go mod cache dir failed: {e}"))?;

    let output = Command::new("go")
        .arg("build")
        .arg("-o")
        .arg(&binary_path)
        .arg("./cmd/roodox_server")
        .current_dir(&root)
        .env("GOCACHE", &go_cache)
        .env("GOMODCACHE", &go_mod_cache)
        .output()
        .map_err(|e| format!("build server binary failed: {e}"))?;
    if !output.status.success() {
        let stderr = output_text(&output.stderr);
        let stdout = output_text(&output.stdout);
        let detail = if !stderr.is_empty() { stderr } else { stdout };
        return Err(if detail.is_empty() {
            format!("build server binary failed: {}", output.status)
        } else {
            format!("build server binary failed: {detail}")
        });
    }

    Ok(Some(binary_path))
}

fn detect_server_command(config_path: &Path) -> Result<Command, String> {
    let config_dir = config_path.parent().unwrap_or(Path::new(".")).to_path_buf();
    if let Some(binary) = server_binary_path(config_path) {
        if binary.exists() {
            let mut cmd = Command::new(binary);
            cmd.current_dir(&config_dir);
            return Ok(cmd);
        }
    }

    let root = project_root();
    if root.join("go.mod").exists() && root.join("cmd").join("roodox_server").exists() {
        let mut cmd = Command::new("go");
        cmd.arg("run").arg("./cmd/roodox_server");
        cmd.current_dir(root);
        return Ok(cmd);
    }

    Err("server binary not found and repo root is unavailable".to_string())
}

fn source_server_command() -> Option<Command> {
    let root = project_root();
    if !(root.join("go.mod").exists() && root.join("cmd").join("roodox_server").exists()) {
        return None;
    }

    let mut cmd = Command::new("go");
    cmd.arg("run").arg("./cmd/roodox_server");
    cmd.current_dir(root);
    Some(cmd)
}

fn first_line(output: &[u8]) -> String {
    String::from_utf8_lossy(output)
        .lines()
        .next()
        .unwrap_or("")
        .trim()
        .to_string()
}

fn unix_now() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}

fn output_text(output: &[u8]) -> String {
    String::from_utf8_lossy(output).trim().to_string()
}

fn run_server_admin_command_owned(
    config_path: &Path,
    args: &[String],
) -> Result<std::process::Output, String> {
    let mut cmd = detect_server_command(config_path)?;
    cmd.arg("-config")
        .arg(config_path.to_string_lossy().to_string());
    for arg in args {
        cmd.arg(arg);
    }
    let output = cmd
        .output()
        .map_err(|e| format!("run server admin command failed: {e}"))?;
    let stderr = output_text(&output.stderr);
    let stdout = output_text(&output.stdout);
    let detail = if !stderr.is_empty() { stderr } else { stdout };
    if output.status.success() || !detail.contains("flag provided but not defined:") {
        return Ok(output);
    }

    let mut fallback = source_server_command().ok_or_else(|| {
        "server admin flag is unavailable and no source fallback exists".to_string()
    })?;
    fallback
        .arg("-config")
        .arg(config_path.to_string_lossy().to_string());
    for arg in args {
        fallback.arg(arg);
    }
    fallback
        .output()
        .map_err(|e| format!("run fallback server admin command failed: {e}"))
}

fn run_server_admin_command(
    config_path: &Path,
    args: &[&str],
) -> Result<std::process::Output, String> {
    let owned_args = args
        .iter()
        .map(|arg| (*arg).to_string())
        .collect::<Vec<_>>();
    run_server_admin_command_owned(config_path, &owned_args)
}

fn run_server_admin_json<T>(config_path: &Path, args: &[&str]) -> Result<T, String>
where
    T: for<'de> Deserialize<'de>,
{
    let output = run_server_admin_command(config_path, args)?;
    if !output.status.success() {
        let stderr = output_text(&output.stderr);
        let stdout = output_text(&output.stdout);
        let detail = if !stderr.is_empty() { stderr } else { stdout };
        return Err(if detail.is_empty() {
            format!("server admin command failed: {}", output.status)
        } else {
            detail
        });
    }

    serde_json::from_slice(&output.stdout).map_err(|e| format!("parse admin json failed: {e}"))
}

fn tool_info_from_path(path: String) -> ToolInfo {
    let version_output = Command::new(&path).arg("--version").output();
    let version = match version_output {
        Ok(out) if out.status.success() => Some(first_line(&out.stdout)),
        Ok(out) => Some(first_line(&out.stderr)),
        Err(_) => None,
    };

    ToolInfo {
        installed: true,
        path: Some(path),
        version,
        error: None,
    }
}

fn find_tool(tool: &str) -> ToolInfo {
    find_tool_with_dirs(tool, &[])
}

fn find_tool_with_dirs(tool: &str, extra_dirs: &[String]) -> ToolInfo {
    #[cfg(target_os = "windows")]
    let mut where_cmd = {
        let mut c = Command::new("where");
        c.arg(tool);
        c
    };

    #[cfg(not(target_os = "windows"))]
    let mut where_cmd = {
        let mut c = Command::new("which");
        c.arg(tool);
        c
    };

    let mut where_err = String::new();
    if let Ok(out) = where_cmd.output() {
        if out.status.success() {
            let path = first_line(&out.stdout);
            if !path.is_empty() {
                return tool_info_from_path(path);
            }
        } else {
            where_err = first_line(&out.stderr);
        }
    }

    #[cfg(target_os = "windows")]
    let names = vec![format!("{tool}.exe"), tool.to_string()];

    #[cfg(not(target_os = "windows"))]
    let names = vec![tool.to_string()];

    for dir in extra_dirs {
        let base = PathBuf::from(dir);
        for name in &names {
            let p = base.join(name);
            if p.exists() {
                return tool_info_from_path(p.to_string_lossy().to_string());
            }
        }
    }

    let err = if where_err.is_empty() {
        format!("not found in PATH and configured tool dirs: {tool}")
    } else {
        where_err
    };

    ToolInfo {
        installed: false,
        path: None,
        version: None,
        error: Some(err),
    }
}
fn recommended_tool_dirs() -> Vec<String> {
    let mut dirs = Vec::new();
    #[cfg(target_os = "windows")]
    {
        let candidates = [
            r"C:\Program Files\CMake\bin",
            r"C:\Program Files (x86)\GnuWin32\bin",
        ];
        for c in candidates {
            let p = PathBuf::from(c);
            if p.exists() {
                dirs.push(p.to_string_lossy().to_string());
            }
        }
    }
    dirs
}

#[tauri::command]
fn load_config() -> Result<AppConfig, String> {
    read_config_file()
}

#[tauri::command]
fn save_config(cfg: AppConfig) -> Result<AppConfig, String> {
    write_config_file(cfg)
}

#[tauri::command]
fn read_logs() -> Result<Vec<String>, String> {
    LOGS.lock()
        .map(|v| v.clone())
        .map_err(|e| format!("log lock failed: {e}"))
}

#[tauri::command]
fn load_workbench_snapshot() -> Result<WorkbenchSnapshot, String> {
    let cfg_path = config_path();
    match run_server_admin_json::<WorkbenchSnapshot>(&cfg_path, &["-workbench-snapshot-json"]) {
        Ok(mut snapshot) => {
            snapshot.query_error = None;
            Ok(snapshot)
        }
        Err(err) => Ok(WorkbenchSnapshot {
            runtime: None,
            devices: Vec::new(),
            collected_at_unix: unix_now(),
            query_error: Some(err),
        }),
    }
}

#[tauri::command]
fn load_workbench_observability() -> Result<WorkbenchObservabilitySnapshot, String> {
    let cfg_path = config_path();
    run_server_admin_json::<WorkbenchObservabilitySnapshot>(
        &cfg_path,
        &["-workbench-observability-json"],
    )
}

#[tauri::command]
fn load_tls_status() -> Result<TLSStatus, String> {
    let cfg_path = config_path();
    run_server_admin_json::<TLSStatus>(&cfg_path, &["-tls-status"])
}

#[tauri::command]
fn trigger_server_backup() -> Result<BackupTriggerResult, String> {
    let cfg_path = config_path();
    run_server_admin_json::<BackupTriggerResult>(&cfg_path, &["-trigger-server-backup-json"])
}

fn default_client_ca_export_path() -> PathBuf {
    project_root()
        .join("artifacts")
        .join("handoff")
        .join("roodox-ca-cert.pem")
}

fn default_client_access_export_dir() -> PathBuf {
    project_root()
        .join("artifacts")
        .join("handoff")
        .join("client-access")
}

fn issue_join_bundle_internal(
    config_path: &Path,
    request: JoinBundleRequest,
) -> Result<IssueJoinBundleResult, String> {
    let mut args = vec!["-issue-join-bundle-json".to_string()];
    let device_id = request.device_id.trim();
    if !device_id.is_empty() {
        args.push("-join-device-id".to_string());
        args.push(device_id.to_string());
    }
    let device_name = request.device_name.trim();
    if !device_name.is_empty() {
        args.push("-join-device-name".to_string());
        args.push(device_name.to_string());
    }
    let device_role = request.device_role.trim();
    if !device_role.is_empty() {
        args.push("-join-device-role".to_string());
        args.push(device_role.to_string());
    }
    let device_group = request.device_group.trim();
    if !device_group.is_empty() {
        args.push("-join-device-group".to_string());
        args.push(device_group.to_string());
    }

    let output = run_server_admin_command_owned(config_path, &args)?;
    if !output.status.success() {
        let stderr = output_text(&output.stderr);
        let stdout = output_text(&output.stdout);
        let detail = if !stderr.is_empty() { stderr } else { stdout };
        return Err(if detail.is_empty() {
            format!("issue join bundle failed: {}", output.status)
        } else {
            detail
        });
    }

    serde_json::from_slice(&output.stdout)
        .map_err(|e| format!("parse join bundle json failed: {e}"))
}

#[tauri::command]
fn export_client_ca(destination_path: Option<String>) -> Result<ExportClientCAResult, String> {
    let cfg_path = config_path();
    let raw = destination_path
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
        .unwrap_or_else(default_client_ca_export_path);
    let export_path = if raw.is_absolute() {
        raw
    } else {
        project_root().join(raw)
    };
    if let Some(parent) = export_path.parent() {
        fs::create_dir_all(parent).map_err(|e| format!("create export dir failed: {e}"))?;
    }

    let args = vec![
        "-export-client-ca".to_string(),
        export_path.to_string_lossy().to_string(),
    ];
    let output = run_server_admin_command_owned(&cfg_path, &args)?;
    if !output.status.success() {
        let stderr = output_text(&output.stderr);
        let stdout = output_text(&output.stdout);
        let detail = if !stderr.is_empty() { stderr } else { stdout };
        return Err(if detail.is_empty() {
            format!("export client ca failed: {}", output.status)
        } else {
            detail
        });
    }

    serde_json::from_slice(&output.stdout).map_err(|e| format!("parse export json failed: {e}"))
}

#[tauri::command]
fn issue_join_bundle(request: Option<JoinBundleRequest>) -> Result<IssueJoinBundleResult, String> {
    let cfg_path = config_path();
    issue_join_bundle_internal(&cfg_path, request.unwrap_or_default())
}

#[tauri::command]
fn export_client_access_bundle(
    request: Option<JoinBundleRequest>,
    destination_dir: Option<String>,
) -> Result<ExportClientAccessResult, String> {
    let cfg_path = config_path();
    let bundle = issue_join_bundle_internal(&cfg_path, request.unwrap_or_default())?;
    let raw = destination_dir
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
        .unwrap_or_else(default_client_access_export_dir);
    let export_dir = if raw.is_absolute() {
        raw
    } else {
        project_root().join(raw)
    };
    fs::create_dir_all(&export_dir).map_err(|e| format!("create access export dir failed: {e}"))?;

    let bundle_path = export_dir.join("roodox-client-access.json");
    fs::write(&bundle_path, &bundle.bundle_json)
        .map_err(|e| format!("write join bundle failed: {e}"))?;

    let ca_path = if bundle.bundle.use_tls {
        let target = export_dir.join("roodox-ca-cert.pem");
        let args = vec![
            "-export-client-ca".to_string(),
            target.to_string_lossy().to_string(),
        ];
        let output = run_server_admin_command_owned(&cfg_path, &args)?;
        if !output.status.success() {
            let stderr = output_text(&output.stderr);
            let stdout = output_text(&output.stdout);
            let detail = if !stderr.is_empty() { stderr } else { stdout };
            return Err(if detail.is_empty() {
                format!("export client ca failed: {}", output.status)
            } else {
                detail
            });
        }
        Some(target.to_string_lossy().to_string())
    } else {
        None
    };

    Ok(ExportClientAccessResult {
        export_dir: export_dir.to_string_lossy().to_string(),
        bundle_path: bundle_path.to_string_lossy().to_string(),
        ca_path,
    })
}

#[tauri::command]
fn start_server() -> Result<(), String> {
    let mut guard = CHILD
        .lock()
        .map_err(|e| format!("child lock failed: {e}"))?;
    if let Some(child) = guard.as_mut() {
        match child.try_wait() {
            Ok(None) => return Ok(()),
            Ok(Some(_)) => {
                *guard = None;
            }
            Err(e) => return Err(format!("check child status failed: {e}")),
        }
    }

    let cfg_path = config_path();
    if let Ok(snapshot) =
        run_server_admin_json::<WorkbenchSnapshot>(&cfg_path, &["-workbench-snapshot-json"])
    {
        if snapshot.runtime.is_some() {
            push_log(format!(
                "{} server already running outside GUI child tracking",
                now_hms()
            ));
            return Ok(());
        }
    }

    ensure_server_binary_current(&cfg_path)?;
    let mut cmd = detect_server_command(&cfg_path)?;
    cmd.arg("-config")
        .arg(cfg_path.to_string_lossy().to_string());
    cmd.stdout(Stdio::piped());
    cmd.stderr(Stdio::piped());

    let mut child = cmd
        .spawn()
        .map_err(|e| format!("start server failed: {e}"))?;

    if let Some(stdout) = child.stdout.take() {
        thread::spawn(move || {
            let reader = BufReader::new(stdout);
            for line in reader.lines() {
                if let Ok(l) = line {
                    push_log(format!("{} [server] {l}", now_hms()));
                }
            }
        });
    }

    if let Some(stderr) = child.stderr.take() {
        thread::spawn(move || {
            let reader = BufReader::new(stderr);
            for line in reader.lines() {
                if let Ok(l) = line {
                    push_log(format!("{} [server-err] {l}", now_hms()));
                }
            }
        });
    }

    push_log(format!("{} server start requested", now_hms()));
    *guard = Some(child);
    Ok(())
}
#[tauri::command]
fn stop_server() -> Result<(), String> {
    let cfg_path = config_path();
    let graceful_requested = run_server_admin_command(
        &cfg_path,
        &[
            "-request-shutdown",
            "-shutdown-reason",
            "workbench stop request",
        ],
    )
    .is_ok();
    if graceful_requested {
        push_log(format!("{} graceful shutdown requested", now_hms()));
    }

    let mut guard = CHILD
        .lock()
        .map_err(|e| format!("child lock failed: {e}"))?;
    if let Some(child) = guard.as_mut() {
        if graceful_requested {
            push_log(format!("{} server stop requested", now_hms()));
        } else {
            child
                .kill()
                .map_err(|e| format!("stop server failed: {e}"))?;
            push_log(format!("{} server stop requested", now_hms()));
        }
    }
    if !graceful_requested {
        *guard = None;
    }
    Ok(())
}

#[tauri::command]
fn server_status() -> Result<ServerStatus, String> {
    let cfg = read_config_file().unwrap_or_else(|_| default_config());
    let cfg_path = config_path();
    let installing = INSTALLING.lock().map(|f| *f).unwrap_or(false);

    let mut running = false;
    let mut last_error = None;

    if let Ok(mut guard) = CHILD.lock() {
        if let Some(child) = guard.as_mut() {
            match child.try_wait() {
                Ok(None) => {
                    running = true;
                }
                Ok(Some(status)) => {
                    last_error = Some(format!("server exited: {status}"));
                    *guard = None;
                }
                Err(e) => {
                    last_error = Some(format!("status check failed: {e}"));
                }
            }
        }
    }

    if !running {
        match run_server_admin_json::<WorkbenchSnapshot>(&cfg_path, &["-workbench-snapshot-json"]) {
            Ok(snapshot) => {
                if snapshot.runtime.is_some() {
                    running = true;
                    last_error = None;
                }
            }
            Err(err) => {
                if last_error.is_none() {
                    last_error = Some(err);
                }
            }
        }
    }

    Ok(ServerStatus {
        running,
        addr: Some(cfg.addr),
        root_dir: Some(cfg.root_dir),
        remote_build: Some(cfg.remote_build_enabled),
        last_error,
        installing,
    })
}

#[tauri::command]
fn check_environment() -> Result<EnvCheck, String> {
    let cfg = read_config_file().unwrap_or_else(|_| default_config());
    let mut search_dirs = cfg.build_tool_dirs.clone();
    for d in recommended_tool_dirs() {
        if !search_dirs.iter().any(|v| v.eq_ignore_ascii_case(&d)) {
            search_dirs.push(d);
        }
    }

    let mut tools = std::collections::HashMap::new();
    tools.insert(
        "cmake".to_string(),
        find_tool_with_dirs("cmake", &search_dirs),
    );
    tools.insert(
        "make".to_string(),
        find_tool_with_dirs("make", &search_dirs),
    );

    let winget_installed = find_tool("winget").installed;

    Ok(EnvCheck {
        os: std::env::consts::OS.to_string(),
        winget_installed,
        tools,
        recommended_tool_dirs: recommended_tool_dirs(),
        config_tool_dirs: cfg.build_tool_dirs,
    })
}
fn run_winget_install(pkg: &str) -> Result<(), String> {
    let output = Command::new("winget")
        .arg("install")
        .arg("--id")
        .arg(pkg)
        .arg("-e")
        .arg("--accept-package-agreements")
        .arg("--accept-source-agreements")
        .output()
        .map_err(|e| format!("run winget failed: {e}"))?;

    push_log(format!(
        "{} install {} => {}",
        now_hms(),
        pkg,
        first_line(&output.stdout)
    ));

    if output.status.success() {
        Ok(())
    } else {
        Err(format!(
            "install {} failed: {}",
            pkg,
            first_line(&output.stderr)
        ))
    }
}

#[tauri::command]
fn install_missing_tools() -> Result<(), String> {
    {
        let mut flag = INSTALLING
            .lock()
            .map_err(|e| format!("install flag lock failed: {e}"))?;
        if *flag {
            return Err("installer already running".to_string());
        }
        *flag = true;
    }

    thread::spawn(|| {
        push_log(format!("{} installer started", now_hms()));

        let cmake = find_tool("cmake");
        if !cmake.installed {
            if let Err(e) = run_winget_install("Kitware.CMake") {
                push_log(format!("{} {e}", now_hms()));
            }
        }

        let make = find_tool("make");
        if !make.installed {
            if let Err(e) = run_winget_install("GnuWin32.Make") {
                push_log(format!("{} {e}", now_hms()));
            }
        }

        if let Ok(mut cfg) = read_config_file() {
            let mut merged = cfg.build_tool_dirs.clone();
            let mut seen: HashSet<String> = merged.iter().map(|s| s.to_lowercase()).collect();
            for d in recommended_tool_dirs() {
                let key = d.to_lowercase();
                if seen.insert(key) {
                    merged.push(d);
                }
            }
            cfg.build_tool_dirs = merged;
            let _ = write_config_file(cfg);
        }

        if let Ok(mut flag) = INSTALLING.lock() {
            *flag = false;
        }
        push_log(format!("{} installer finished", now_hms()));
    });

    Ok(())
}

fn main() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![
            load_config,
            save_config,
            read_logs,
            load_workbench_snapshot,
            load_workbench_observability,
            load_tls_status,
            trigger_server_backup,
            export_client_ca,
            issue_join_bundle,
            export_client_access_bundle,
            start_server,
            stop_server,
            server_status,
            check_environment,
            install_missing_tools
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

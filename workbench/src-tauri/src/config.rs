use crate::models::AppConfig;
use serde_json::{Map, Value};
use std::collections::HashSet;
use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

const BOOTSTRAP_FILE_NAME: &str = "roodox-workbench.bootstrap.json";

#[derive(Debug, Clone, serde::Deserialize)]
struct BootstrapConfig {
    project_root: Option<String>,
    config_path: Option<String>,
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

fn resolve_bootstrap_value(path: &str) -> Option<PathBuf> {
    let trimmed = path.trim();
    if trimmed.is_empty() {
        return None;
    }

    let candidate = PathBuf::from(trimmed);
    if candidate.is_absolute() {
        return Some(candidate);
    }

    let bootstrap = bootstrap_path()?;
    let base_dir = bootstrap.parent()?;
    Some(base_dir.join(candidate))
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
    resolve_bootstrap_value(&path)
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
    resolve_bootstrap_value(&path)
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

pub fn project_root() -> PathBuf {
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

pub fn config_path() -> PathBuf {
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
    cfg.bundle_service_port = if cfg.bundle_service_port == 0 {
        default_port.max(1)
    } else {
        cfg.bundle_service_port
    };
    cfg.bundle_tls_server_name = cfg.bundle_tls_server_name.trim().to_string();
    cfg
}

fn read_config_value(path: &Path) -> Result<Value, String> {
    let text = fs::read_to_string(path).map_err(|e| format!("read config failed: {e}"))?;
    serde_json::from_str::<Value>(&text).map_err(|e| format!("parse config failed: {e}"))
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
    value
        .get(key)
        .and_then(Value::as_array)
        .map(|items| {
            items
                .iter()
                .filter_map(Value::as_str)
                .map(ToString::to_string)
                .collect::<Vec<_>>()
        })
        .unwrap_or_else(|| default.iter().map(|item| (*item).to_string()).collect())
}

fn extract_app_config(value: &Value) -> AppConfig {
    let default = AppConfig::default();
    let (derived_host, derived_port) = parse_discovery_addr(&read_string(value, "addr", &default.addr));
    let tls_enabled = read_bool(value, "tls_enabled", default.tls_enabled);
    normalize_config(AppConfig {
        addr: read_string(value, "addr", &default.addr),
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
            &["control_plane", "join_bundle", "service_discovery", "use_tls"],
            tls_enabled,
        ),
        bundle_tls_server_name: read_nested_string(
            value,
            &["control_plane", "join_bundle", "service_discovery", "tls_server_name"],
            &default.bundle_tls_server_name,
        ),
    })
}

pub fn read_config_file() -> Result<AppConfig, String> {
    let path = config_path();
    let value = read_config_value(&path)?;
    Ok(extract_app_config(&value))
}

pub fn write_config_file(cfg: AppConfig) -> Result<AppConfig, String> {
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

pub fn ensure_server_binary_current(config_path: &Path) -> Result<Option<PathBuf>, String> {
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

pub fn detect_server_command(config_path: &Path) -> Result<Command, String> {
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

pub fn source_server_command() -> Option<Command> {
    let root = project_root();
    if !(root.join("go.mod").exists() && root.join("cmd").join("roodox_server").exists()) {
        return None;
    }

    let mut cmd = Command::new("go");
    cmd.arg("run").arg("./cmd/roodox_server");
    cmd.current_dir(root);
    Some(cmd)
}

fn output_text(output: &[u8]) -> String {
    String::from_utf8_lossy(output).trim().to_string()
}

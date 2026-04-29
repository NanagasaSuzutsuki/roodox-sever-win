#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod config;
mod models;

use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use base64::Engine;
use config::{
    config_path, detect_server_command, ensure_server_binary_current, project_root,
    read_config_file, source_server_command, suppress_command_window, write_config_file,
};
use models::*;
use once_cell::sync::Lazy;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashSet;
use std::fs;
use std::io::{BufRead, BufReader};
use std::net::Ipv4Addr;
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use std::thread;
#[cfg(target_os = "windows")]
use windows::Win32::Foundation::{LPARAM, WPARAM};
#[cfg(target_os = "windows")]
use windows::Win32::UI::Input::KeyboardAndMouse::ReleaseCapture;
#[cfg(target_os = "windows")]
use windows::Win32::UI::WindowsAndMessaging::{SendMessageW, HTCAPTION, WM_NCLBUTTONDOWN};

static CHILD: Lazy<Mutex<Option<Child>>> = Lazy::new(|| Mutex::new(None));
static LOGS: Lazy<Mutex<Vec<String>>> = Lazy::new(|| Mutex::new(Vec::new()));
static INSTALLING: Lazy<Mutex<bool>> = Lazy::new(|| Mutex::new(false));

fn should_prefer_source_admin_command(config_path: &Path) -> bool {
    let root = project_root();
    if !(root.join("go.mod").exists() && root.join("cmd").join("roodox_server").exists()) {
        return false;
    }
    let config_dir = config_path.parent().unwrap_or(Path::new("."));
    let canonical_config_dir = match fs::canonicalize(config_dir) {
        Ok(value) => value,
        Err(_) => return false,
    };
    let canonical_root = match fs::canonicalize(&root) {
        Ok(value) => value,
        Err(_) => return false,
    };
    !canonical_config_dir.starts_with(canonical_root)
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

#[derive(Debug, Deserialize)]
struct PowerShellNetIpRow {
    #[serde(rename = "InterfaceAlias")]
    interface_alias: Option<String>,
    #[serde(rename = "IPAddress")]
    ip_address: String,
}

#[derive(Debug, Serialize, Deserialize)]
struct ConnectionCodePayload {
    version: u32,
    bundle: Value,
    #[serde(skip_serializing_if = "Option::is_none")]
    ca_pem: Option<String>,
}

fn resolve_path_from_config(config_path: &Path, raw: &str) -> PathBuf {
    let candidate = PathBuf::from(raw.trim());
    if candidate.is_absolute() {
        candidate
    } else {
        config_path
            .parent()
            .unwrap_or_else(|| Path::new("."))
            .join(candidate)
    }
}

fn default_local_tls_root_cert_path(config_path: &Path, cert_path: &str) -> PathBuf {
    let resolved_cert_path = resolve_path_from_config(config_path, cert_path);
    resolved_cert_path
        .parent()
        .unwrap_or_else(|| Path::new("."))
        .join("roodox-ca-cert.pem")
}

fn load_client_ca_pem(config_path: &Path, cert_path: &str) -> Result<String, String> {
    let pem_path = default_local_tls_root_cert_path(config_path, cert_path);
    let pem = fs::read_to_string(&pem_path)
        .map_err(|e| format!("read client ca failed ({}): {e}", pem_path.display()))?;
    let trimmed = pem.trim();
    if trimmed.is_empty() {
        return Err(format!("client ca is empty: {}", pem_path.display()));
    }
    Ok(format!("{trimmed}\n"))
}

fn build_connection_code_result(
    bundle_json: &str,
    ca_pem: Option<&str>,
) -> Result<ConnectionCodeResult, String> {
    let bundle_value: Value = serde_json::from_str(bundle_json)
        .map_err(|e| format!("parse join bundle json failed: {e}"))?;
    let payload = ConnectionCodePayload {
        version: 1,
        bundle: bundle_value,
        ca_pem: ca_pem
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(|value| format!("{value}\n")),
    };
    let compact_payload =
        serde_json::to_vec(&payload).map_err(|e| format!("encode connection code failed: {e}"))?;
    let encoded = URL_SAFE_NO_PAD.encode(&compact_payload);
    Ok(ConnectionCodeResult {
        format: "roodox:1".to_string(),
        code: format!("roodox:1:{encoded}"),
        uri: format!("roodox://connect?v=1&payload={encoded}"),
        payload_size: compact_payload.len(),
    })
}

fn write_client_import_readme(
    export_dir: &Path,
    connection_code_path: &Path,
    importer_path: Option<&Path>,
) -> Result<PathBuf, String> {
    let readme_path = export_dir.join("README-client-import.txt");
    let importer_name = importer_path
        .and_then(|path| path.file_name())
        .and_then(|value| value.to_str())
        .unwrap_or("roodox_client_import.exe");
    let lines = vec![
        "Roodox client handoff".to_string(),
        String::new(),
        "Included files:".to_string(),
        "- roodox-client-access.json: join bundle for manual import".to_string(),
        "- roodox-ca-cert.pem: TLS trust root when TLS is enabled".to_string(),
        "- roodox-connection-code.txt: self-contained connection code".to_string(),
        "- roodox_client_import.exe: optional importer helper when bundled".to_string(),
        String::new(),
        "Recommended import:".to_string(),
        format!(
            "{importer_name} -connection-code-file \"{}\" -output-dir . -probe",
            connection_code_path.display()
        ),
        String::new(),
        "Manual fallback:".to_string(),
        "1. Keep roodox-client-access.json available to the client side.".to_string(),
        "2. If TLS is enabled, keep roodox-ca-cert.pem alongside the client config.".to_string(),
        "3. Configure the client to use the exported host, port, TLS server name, and shared secret."
            .to_string(),
    ];
    fs::write(&readme_path, lines.join("\r\n"))
        .map_err(|e| format!("write client import readme failed: {e}"))?;
    Ok(readme_path)
}

fn ensure_client_importer_binary() -> Result<Option<PathBuf>, String> {
    let root = project_root();
    let binary_path = root.join("roodox_client_import.exe");
    if binary_path.exists() {
        return Ok(Some(binary_path));
    }
    if !(root.join("go.mod").exists() && root.join("cmd").join("roodox_client_import").exists()) {
        return Ok(None);
    }

    let mut cmd = Command::new("go");
    suppress_command_window(&mut cmd);
    let output = cmd
        .arg("build")
        .arg("-o")
        .arg(&binary_path)
        .arg("./cmd/roodox_client_import")
        .current_dir(&root)
        .output()
        .map_err(|e| format!("build client importer failed: {e}"))?;
    if !output.status.success() {
        let stderr = output_text(&output.stderr);
        let stdout = output_text(&output.stdout);
        let detail = if !stderr.is_empty() { stderr } else { stdout };
        return Err(if detail.is_empty() {
            format!("build client importer failed: {}", output.status)
        } else {
            format!("build client importer failed: {detail}")
        });
    }

    Ok(Some(binary_path))
}

fn run_hidden_command(program: &str, args: &[&str]) -> Result<std::process::Output, String> {
    let mut cmd = Command::new(program);
    suppress_command_window(&mut cmd);
    for arg in args {
        cmd.arg(arg);
    }
    cmd.output()
        .map_err(|e| format!("run command {program} failed: {e}"))
}

fn run_powershell_script(script: &str) -> Result<std::process::Output, String> {
    run_hidden_command(
        "powershell",
        &[
            "-NoProfile",
            "-ExecutionPolicy",
            "Bypass",
            "-Command",
            script,
        ],
    )
}

fn parse_json_rows<T>(text: &str) -> Result<Vec<T>, String>
where
    T: for<'de> Deserialize<'de>,
{
    let trimmed = text.trim();
    if trimmed.is_empty() {
        return Ok(Vec::new());
    }

    let value: Value =
        serde_json::from_str(trimmed).map_err(|e| format!("parse json output failed: {e}"))?;
    match value {
        Value::Array(items) => items
            .into_iter()
            .map(|item| serde_json::from_value(item).map_err(|e| format!("parse row failed: {e}")))
            .collect(),
        Value::Null => Ok(Vec::new()),
        other => serde_json::from_value(other)
            .map(|item| vec![item])
            .map_err(|e| format!("parse row failed: {e}")),
    }
}

fn parse_ipv4(ip: &str) -> Option<Ipv4Addr> {
    ip.trim().parse::<Ipv4Addr>().ok()
}

fn is_private_lan_ip(ip: &str) -> bool {
    let Some(addr) = parse_ipv4(ip) else {
        return false;
    };
    let octets = addr.octets();
    octets[0] == 10
        || (octets[0] == 172 && (16..=31).contains(&octets[1]))
        || (octets[0] == 192 && octets[1] == 168)
}

fn is_tailscale_ip(ip: &str) -> bool {
    let Some(addr) = parse_ipv4(ip) else {
        return false;
    };
    let octets = addr.octets();
    octets[0] == 100 && (64..=127).contains(&octets[1])
}

fn looks_virtual_interface(alias: &str) -> bool {
    let value = alias.trim().to_lowercase();
    [
        "vmware",
        "hyper-v",
        "vethernet",
        "virtualbox",
        "loopback",
        "wsl",
        "docker",
        "tun",
        "tap",
        "tailscale",
        "easytier",
        "zerotier",
        "wireguard",
        "hamachi",
        "bluetooth",
        "teredo",
        "isatap",
        "vpn",
    ]
    .iter()
    .any(|needle| value.contains(needle))
}

fn lan_interface_priority(alias: Option<&str>) -> u8 {
    let value = alias.unwrap_or("").trim().to_lowercase();
    if value.is_empty() {
        return 5;
    }
    if [
        "ethernet",
        "以太网",
        "wlan",
        "wi-fi",
        "wifi",
        "wireless",
    ]
    .iter()
    .any(|needle| value.contains(needle))
    {
        return 0;
    }
    if ["local area", "lan"].iter().any(|needle| value.contains(needle)) {
        return 1;
    }
    if ["usb", "rndis", "mobile", "phone", "hotspot"]
        .iter()
        .any(|needle| value.contains(needle))
    {
        return 2;
    }
    3
}

fn lan_ip_priority(ip: &str) -> u8 {
    let Some(addr) = parse_ipv4(ip) else {
        return 9;
    };
    let octets = addr.octets();
    if octets[0] == 192 && octets[1] == 168 {
        return 0;
    }
    if octets[0] == 10 {
        return 1;
    }
    if octets[0] == 172 && (16..=31).contains(&octets[1]) {
        return 2;
    }
    3
}

fn collect_ipv4_rows() -> Result<Vec<PowerShellNetIpRow>, String> {
    let script = "$ProgressPreference='SilentlyContinue'; Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.IPAddress -notlike '127.*' -and $_.IPAddress -notlike '169.254.*' } | Select-Object InterfaceAlias,IPAddress | ConvertTo-Json -Compress";
    let output = run_powershell_script(script)?;
    if !output.status.success() {
        let stderr = output_text(&output.stderr);
        let stdout = output_text(&output.stdout);
        let detail = if !stderr.is_empty() { stderr } else { stdout };
        return Err(if detail.is_empty() {
            format!("network discovery failed: {}", output.status)
        } else {
            detail
        });
    }

    parse_json_rows::<PowerShellNetIpRow>(&String::from_utf8_lossy(&output.stdout))
}

fn easytier_candidate_paths() -> Vec<PathBuf> {
    let mut paths = Vec::new();
    let mut push = |base: Option<std::ffi::OsString>, relative: &str| {
        if let Some(root) = base.clone() {
            paths.push(PathBuf::from(root).join(relative));
        }
    };

    push(
        std::env::var_os("LOCALAPPDATA"),
        r"Programs\easytier-gui\EasyTier GUI.exe",
    );
    push(
        std::env::var_os("PROGRAMFILES"),
        r"EasyTier GUI\EasyTier GUI.exe",
    );
    push(
        std::env::var_os("PROGRAMFILES"),
        r"EasyTier\EasyTier GUI.exe",
    );
    push(
        std::env::var_os("PROGRAMFILES(X86)"),
        r"EasyTier GUI\EasyTier GUI.exe",
    );
    push(
        std::env::var_os("PROGRAMFILES"),
        r"EasyTier\easytier-core.exe",
    );
    paths
}

fn find_tool_or_paths(names: &[&str], paths: &[PathBuf]) -> ToolInfo {
    for name in names {
        let info = find_tool(name);
        if info.installed {
            return info;
        }
    }

    for path in paths {
        if path.exists() {
            return tool_info_from_path(path.to_string_lossy().to_string());
        }
    }

    ToolInfo {
        installed: false,
        path: None,
        version: None,
        error: Some("not found".to_string()),
    }
}

fn detect_tailscale_host(rows: &[PowerShellNetIpRow]) -> Option<String> {
    if let Ok(output) = run_hidden_command("tailscale", &["status", "--json"]) {
        if output.status.success() {
            if let Ok(value) = serde_json::from_slice::<Value>(&output.stdout) {
                if let Some(dns_name) = value
                    .get("Self")
                    .and_then(|self_value| self_value.get("DNSName"))
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|value| !value.is_empty())
                {
                    return Some(dns_name.trim_end_matches('.').to_string());
                }
            }
        }
    }

    rows.iter()
        .find(|row| {
            row.interface_alias
                .as_deref()
                .map(|alias| alias.to_lowercase().contains("tailscale"))
                .unwrap_or(false)
                || is_tailscale_ip(&row.ip_address)
        })
        .map(|row| row.ip_address.clone())
        .or_else(|| {
            let output = run_hidden_command("tailscale", &["ip", "-4"]).ok()?;
            if !output.status.success() {
                return None;
            }
            let host = first_line(&output.stdout);
            if host.is_empty() {
                None
            } else {
                Some(host)
            }
        })
}

fn detect_easytier_host(rows: &[PowerShellNetIpRow]) -> Option<String> {
    rows.iter()
        .find(|row| {
            row.interface_alias
                .as_deref()
                .map(|alias| alias.to_lowercase().contains("easytier"))
                .unwrap_or(false)
        })
        .map(|row| row.ip_address.clone())
}

fn run_server_admin_command_owned(
    config_path: &Path,
    args: &[String],
) -> Result<std::process::Output, String> {
    if should_prefer_source_admin_command(config_path) {
        let mut source = source_server_command().ok_or_else(|| {
            "source admin command is preferred but no source fallback exists".to_string()
        })?;
        source
            .arg("-config")
            .arg(config_path.to_string_lossy().to_string());
        for arg in args {
            source.arg(arg);
        }
        return source
            .output()
            .map_err(|e| format!("run preferred source admin command failed: {e}"));
    }

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

fn apply_initial_path(
    mut dialog: rfd::FileDialog,
    initial_path: Option<String>,
) -> rfd::FileDialog {
    let Some(raw) = initial_path else {
        return dialog;
    };
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return dialog;
    }

    let path = PathBuf::from(trimmed);
    if path.is_dir() {
        return dialog.set_directory(path);
    }

    if let Some(parent) = path.parent() {
        if !parent.as_os_str().is_empty() {
            dialog = dialog.set_directory(parent);
        }
    }
    if let Some(name) = path.file_name().and_then(|value| value.to_str()) {
        dialog = dialog.set_file_name(name);
    }
    dialog
}

#[tauri::command]
fn pick_directory(initial_path: Option<String>) -> Result<Option<String>, String> {
    let dialog = apply_initial_path(rfd::FileDialog::new(), initial_path);
    Ok(dialog
        .pick_folder()
        .map(|path| path.to_string_lossy().to_string()))
}

#[tauri::command]
fn pick_file(initial_path: Option<String>) -> Result<Option<String>, String> {
    let dialog = apply_initial_path(rfd::FileDialog::new(), initial_path);
    Ok(dialog
        .pick_file()
        .map(|path| path.to_string_lossy().to_string()))
}

#[tauri::command]
fn save_file_path(initial_path: Option<String>) -> Result<Option<String>, String> {
    let dialog = apply_initial_path(rfd::FileDialog::new(), initial_path);
    Ok(dialog
        .save_file()
        .map(|path| path.to_string_lossy().to_string()))
}

fn tool_info_from_path(path: String) -> ToolInfo {
    let mut version_cmd = Command::new(&path);
    suppress_command_window(&mut version_cmd);
    let version_output = version_cmd.arg("--version").output();
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
        suppress_command_window(&mut c);
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

fn discover_build_tool(tool: &str) -> ToolInfo {
    find_tool_with_dirs(tool, &recommended_tool_dirs())
}

fn build_lan_candidates(rows: &[PowerShellNetIpRow]) -> Vec<AccessHostCandidate> {
    let mut seen = HashSet::new();
    let mut candidates = rows
        .iter()
        .filter(|row| is_private_lan_ip(&row.ip_address))
        .filter(|row| {
            row.interface_alias
                .as_deref()
                .map(|alias| !looks_virtual_interface(alias))
                .unwrap_or(true)
        })
        .filter(|row| seen.insert(row.ip_address.clone()))
        .map(|row| {
            let interface_alias = row.interface_alias.clone();
            let label = match interface_alias.as_deref() {
                Some(alias) if !alias.trim().is_empty() => {
                    format!("{} · {}", row.ip_address, alias)
                }
                _ => row.ip_address.clone(),
            };
            AccessHostCandidate {
                kind: "lan".to_string(),
                label,
                host: row.ip_address.clone(),
                interface_alias,
                source: "recommended-lan".to_string(),
            }
        })
        .collect::<Vec<_>>();

    candidates.sort_by(|left, right| {
        let left_key = (
            lan_interface_priority(left.interface_alias.as_deref()),
            lan_ip_priority(&left.host),
            left.label.clone(),
        );
        let right_key = (
            lan_interface_priority(right.interface_alias.as_deref()),
            lan_ip_priority(&right.host),
            right.label.clone(),
        );
        left_key.cmp(&right_key)
    });
    for (index, candidate) in candidates.iter_mut().enumerate() {
        candidate.source = if index == 0 {
            "recommended-lan".to_string()
        } else {
            "auto-detected".to_string()
        };
    }
    candidates
}

fn tailscale_provider(rows: &[PowerShellNetIpRow]) -> AccessProviderInfo {
    let tool = find_tool("tailscale");
    AccessProviderInfo {
        id: "tailscale".to_string(),
        installed: tool.installed,
        version: tool.version,
        path: tool.path,
        host: detect_tailscale_host(rows),
    }
}

fn easytier_provider(rows: Option<&[PowerShellNetIpRow]>) -> AccessProviderInfo {
    let tool = find_tool_or_paths(
        &["easytier-core", "easytier-cli", "easytier-gui"],
        &easytier_candidate_paths(),
    );
    AccessProviderInfo {
        id: "easytier".to_string(),
        installed: tool.installed,
        version: tool.version,
        path: tool.path,
        host: rows
            .and_then(detect_easytier_host)
            .or_else(|| collect_ipv4_rows().ok().and_then(|items| detect_easytier_host(&items))),
    }
}

fn launch_easytier_installer() -> Result<String, String> {
    let script = "$ProgressPreference='SilentlyContinue'; $release = Invoke-RestMethod -Uri 'https://api.github.com/repos/EasyTier/EasyTier/releases/latest' -Headers @{ 'User-Agent'='roodox-workbench' }; $asset = $release.assets | Where-Object { $_.name -match 'easytier-gui_.*_x64-setup\\.exe$' } | Select-Object -First 1; if ($null -eq $asset) { throw 'EasyTier x64 installer not found in latest release'; }; $target = Join-Path $env:TEMP $asset.name; Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $target; Start-Process -FilePath $target; Write-Output $target";
    let output = run_powershell_script(script)?;
    if !output.status.success() {
        let stderr = output_text(&output.stderr);
        let stdout = output_text(&output.stdout);
        let detail = if !stderr.is_empty() { stderr } else { stdout };
        return Err(if detail.is_empty() {
            format!("launch EasyTier installer failed: {}", output.status)
        } else {
            detail
        });
    }

    let path = first_line(&output.stdout);
    if path.is_empty() {
        Ok("EasyTier installer launched".to_string())
    } else {
        Ok(format!("EasyTier installer launched: {path}"))
    }
}

#[tauri::command]
fn discover_access_setup() -> Result<AccessSetupResult, String> {
    let rows = collect_ipv4_rows().unwrap_or_default();
    let lan_candidates = build_lan_candidates(&rows);
    let providers = vec![tailscale_provider(&rows), easytier_provider(Some(&rows))];

    Ok(AccessSetupResult {
        computer_name: std::env::var("COMPUTERNAME").unwrap_or_default(),
        recommended_lan_host: lan_candidates.first().map(|item| item.host.clone()),
        lan_candidates,
        providers,
    })
}

#[tauri::command]
fn install_access_provider(provider: String) -> Result<String, String> {
    match provider.trim().to_lowercase().as_str() {
        "tailscale" => {
            if find_tool("tailscale").installed {
                return Ok("Tailscale already installed".to_string());
            }
            run_winget_install("Tailscale.Tailscale")?;
            Ok("Tailscale installed".to_string())
        }
        "easytier" => {
            if easytier_provider(None).installed {
                return Ok("EasyTier already installed".to_string());
            }
            launch_easytier_installer()
        }
        other => Err(format!("unsupported access provider: {other}")),
    }
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

fn deployment_root() -> PathBuf {
    let cfg_path = config_path();
    cfg_path
        .parent()
        .map(Path::to_path_buf)
        .unwrap_or_else(project_root)
}

fn default_handoff_root() -> PathBuf {
    deployment_root().join("artifacts").join("handoff")
}

fn default_client_ca_export_path() -> PathBuf {
    default_handoff_root().join("roodox-ca-cert.pem")
}

fn default_client_access_export_dir() -> PathBuf {
    default_handoff_root().join("client-access")
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
        deployment_root().join(raw)
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
fn generate_connection_code(
    request: Option<JoinBundleRequest>,
) -> Result<ConnectionCodeResult, String> {
    let cfg_path = config_path();
    let cfg = read_config_file()?;
    let bundle = issue_join_bundle_internal(&cfg_path, request.unwrap_or_default())?;
    let ca_pem = if bundle.bundle.use_tls || cfg.tls_enabled || cfg.bundle_use_tls {
        Some(load_client_ca_pem(&cfg_path, &cfg.tls_cert_path)?)
    } else {
        None
    };
    build_connection_code_result(&bundle.bundle_json, ca_pem.as_deref())
}

#[tauri::command]
fn export_client_access_bundle(
    request: Option<JoinBundleRequest>,
    destination_dir: Option<String>,
) -> Result<ExportClientAccessResult, String> {
    let cfg_path = config_path();
    let cfg = read_config_file()?;
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
        deployment_root().join(raw)
    };
    fs::create_dir_all(&export_dir).map_err(|e| format!("create access export dir failed: {e}"))?;

    let bundle_path = export_dir.join("roodox-client-access.json");
    fs::write(&bundle_path, &bundle.bundle_json)
        .map_err(|e| format!("write join bundle failed: {e}"))?;

    let ca_pem = if bundle.bundle.use_tls || cfg.tls_enabled || cfg.bundle_use_tls {
        Some(load_client_ca_pem(&cfg_path, &cfg.tls_cert_path)?)
    } else {
        None
    };
    let ca_path = if let Some(pem) = ca_pem.as_deref() {
        let target = export_dir.join("roodox-ca-cert.pem");
        fs::write(&target, pem).map_err(|e| format!("write client ca failed: {e}"))?;
        Some(target.to_string_lossy().to_string())
    } else {
        None
    };

    let connection_code = build_connection_code_result(&bundle.bundle_json, ca_pem.as_deref())?;
    let connection_code_path = export_dir.join("roodox-connection-code.txt");
    fs::write(
        &connection_code_path,
        format!("{}\r\n", connection_code.code),
    )
    .map_err(|e| format!("write connection code failed: {e}"))?;

    let importer_path = match ensure_client_importer_binary() {
        Ok(Some(source_path)) => {
            let target = export_dir.join("roodox_client_import.exe");
            fs::copy(&source_path, &target).map_err(|e| {
                format!(
                    "copy client importer failed ({} -> {}): {e}",
                    source_path.display(),
                    target.display()
                )
            })?;
            Some(target)
        }
        Ok(None) => None,
        Err(_) => None,
    };

    let readme_path =
        write_client_import_readme(&export_dir, &connection_code_path, importer_path.as_deref())?;

    Ok(ExportClientAccessResult {
        export_dir: export_dir.to_string_lossy().to_string(),
        bundle_path: bundle_path.to_string_lossy().to_string(),
        ca_path,
        connection_code_path: Some(connection_code_path.to_string_lossy().to_string()),
        importer_path: importer_path.map(|path| path.to_string_lossy().to_string()),
        readme_path: Some(readme_path.to_string_lossy().to_string()),
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
    let cfg = read_config_file().unwrap_or_else(|_| AppConfig::default());
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
    let mut tools = std::collections::HashMap::new();
    tools.insert("cmake".to_string(), discover_build_tool("cmake"));
    tools.insert("make".to_string(), discover_build_tool("make"));

    let winget_installed = find_tool("winget").installed;

    Ok(EnvCheck {
        os: std::env::consts::OS.to_string(),
        winget_installed,
        tools,
    })
}

fn run_winget_install(pkg: &str) -> Result<(), String> {
    let mut cmd = Command::new("winget");
    suppress_command_window(&mut cmd);
    let output = cmd
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

        let cmake = discover_build_tool("cmake");
        if !cmake.installed {
            if let Err(e) = run_winget_install("Kitware.CMake") {
                push_log(format!("{} {e}", now_hms()));
            }
        }

        let make = discover_build_tool("make");
        if !make.installed {
            if let Err(e) = run_winget_install("GnuWin32.Make") {
                push_log(format!("{} {e}", now_hms()));
            }
        }

        if let Ok(mut flag) = INSTALLING.lock() {
            *flag = false;
        }
        push_log(format!("{} installer finished", now_hms()));
    });

    Ok(())
}

#[tauri::command]
fn window_start_drag(window: tauri::Window) -> Result<(), String> {
    #[cfg(target_os = "windows")]
    {
        let hwnd = window.hwnd().map_err(|e| e.to_string())?;
        unsafe {
            ReleaseCapture().map_err(|e| e.to_string())?;
            SendMessageW(
                hwnd,
                WM_NCLBUTTONDOWN,
                Some(WPARAM(HTCAPTION as usize)),
                Some(LPARAM(0)),
            );
        }
        Ok(())
    }

    #[cfg(not(target_os = "windows"))]
    {
        window.start_dragging().map_err(|e| e.to_string())
    }
}

#[tauri::command]
fn window_minimize(window: tauri::Window) -> Result<(), String> {
    window.minimize().map_err(|e| e.to_string())
}

#[tauri::command]
fn window_toggle_maximize(window: tauri::Window) -> Result<(), String> {
    if window.is_maximized().map_err(|e| e.to_string())? {
        window.unmaximize().map_err(|e| e.to_string())
    } else {
        window.maximize().map_err(|e| e.to_string())
    }
}

#[tauri::command]
fn window_is_maximized(window: tauri::Window) -> Result<bool, String> {
    window.is_maximized().map_err(|e| e.to_string())
}

#[tauri::command]
fn window_close(window: tauri::Window) -> Result<(), String> {
    window.close().map_err(|e| e.to_string())
}

fn main() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![
            load_config,
            save_config,
            pick_directory,
            pick_file,
            save_file_path,
            read_logs,
            load_workbench_snapshot,
            load_workbench_observability,
            load_tls_status,
            trigger_server_backup,
            export_client_ca,
            discover_access_setup,
            install_access_provider,
            issue_join_bundle,
            generate_connection_code,
            export_client_access_bundle,
            start_server,
            stop_server,
            server_status,
            check_environment,
            install_missing_tools,
            window_start_drag,
            window_minimize,
            window_toggle_maximize,
            window_is_maximized,
            window_close
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

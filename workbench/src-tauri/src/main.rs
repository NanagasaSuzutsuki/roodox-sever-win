#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod config;
mod models;

use config::{
    config_path, detect_server_command, ensure_server_binary_current, project_root,
    read_config_file, source_server_command, write_config_file,
};
use models::*;
use once_cell::sync::Lazy;
use serde::Deserialize;
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
        deployment_root().join(raw)
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
    let cfg = read_config_file().unwrap_or_else(|_| AppConfig::default());
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

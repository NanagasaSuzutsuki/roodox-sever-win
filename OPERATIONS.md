# Roodox Server Operations / Roodox 服务端运维

本文描述当前公开仓库中的服务端运维面、GUI 运维面、TLS 生命周期、数据库维护、升级和回滚入口。  
This document describes the server operations surface, GUI operations surface, TLS lifecycle, database maintenance, upgrade, and rollback entrypoints in the public repository.

## Runtime Management Surface / 运行态管理接口

- 健康检查：`grpc.health.v1.Health/Check`  
  Health check: `grpc.health.v1.Health/Check`
- 运行态快照：`AdminConsoleService/GetServerRuntime`  
  Admin runtime snapshot: `AdminConsoleService/GetServerRuntime`
- 可观测性快照：`AdminConsoleService/GetServerObservability`  
  Admin observability snapshot: `AdminConsoleService/GetServerObservability`
- 手动数据库备份：`AdminConsoleService/TriggerServerBackup`  
  Manual database backup: `AdminConsoleService/TriggerServerBackup`

原则上，GUI 或外部管理工具应通过这些 gRPC 接口读取状态，而不是直接解析 SQLite、日志文件或进程内结构。  
GUI or external admin tools should consume these gRPC APIs instead of reading SQLite files, log files, or in-memory structures directly.

## Background Service Startup / 后台服务启动

支持的本地进程管理脚本位于 [`scripts/server`](scripts/server)：  
Supported local process-management scripts live under [`scripts/server`](scripts/server):

- [`start-server.ps1`](scripts/server/start-server.ps1)
- [`stop-server.ps1`](scripts/server/stop-server.ps1)
- [`status-server.ps1`](scripts/server/status-server.ps1)
- [`restart-server.ps1`](scripts/server/restart-server.ps1)
- [`install-windows-service.ps1`](scripts/server/install-windows-service.ps1)
- [`uninstall-windows-service.ps1`](scripts/server/uninstall-windows-service.ps1)
- [`start-windows-service.ps1`](scripts/server/start-windows-service.ps1)
- [`stop-windows-service.ps1`](scripts/server/stop-windows-service.ps1)
- [`restart-windows-service.ps1`](scripts/server/restart-windows-service.ps1)
- [`status-windows-service.ps1`](scripts/server/status-windows-service.ps1)
- [`restore-database.ps1`](scripts/server/restore-database.ps1)
- [`certificate-status.ps1`](scripts/server/certificate-status.ps1)
- [`rotate-certificates.ps1`](scripts/server/rotate-certificates.ps1)
- [`export-client-ca.ps1`](scripts/server/export-client-ca.ps1)
- [`install-deployment.ps1`](scripts/server/install-deployment.ps1)
- [`upgrade-deployment.ps1`](scripts/server/upgrade-deployment.ps1)
- [`rollback-deployment.ps1`](scripts/server/rollback-deployment.ps1)
- [`list-release-snapshots.ps1`](scripts/server/list-release-snapshots.ps1)

这些脚本统一了以下行为：  
These scripts standardize:

- 带 stdout/stderr 重定向的后台启动  
  Background startup with redirected stdout/stderr logs
- 基于 PID 文件的进程所有权管理  
  PID-file based process ownership
- 失效 PID 清理  
  Stale PID cleanup
- 基于 `runtime.state_dir` 的部署级运行态目录  
  Deployment-scoped runtime state under `runtime.state_dir`
- 先走 gRPC 管理面优雅关闭，再回退到进程停止  
  Graceful shutdown through the admin gRPC plane before falling back to direct stop
- 可选的 Windows Service 注册和 SCM 生命周期  
  Optional Windows Service registration and SCM-based lifecycle management

典型用法：  
Typical usage:

```powershell
.\scripts\server\start-server.ps1
.\scripts\server\status-server.ps1
.\scripts\server\restart-server.ps1
.\scripts\server\stop-server.ps1
```

常用开关：  
Useful switches:

- `start-server.ps1 -Rebuild`
- `restart-server.ps1 -Rebuild`
- `stop-server.ps1 -StopUnmanaged`
- `restart-server.ps1 -StopUnmanaged`

Windows Service 相关命令：  
Windows Service usage:

```powershell
.\scripts\server\install-windows-service.ps1
.\scripts\server\status-windows-service.ps1
.\scripts\server\start-windows-service.ps1
.\scripts\server\stop-windows-service.ps1
.\scripts\server\uninstall-windows-service.ps1
.\scripts\server\restore-database.ps1 -Latest
.\scripts\server\certificate-status.ps1
.\scripts\server\rotate-certificates.ps1 -RestartAfter
.\scripts\server\export-client-ca.ps1 -DestinationPath .\handoff\roodox-ca-cert.pem
.\scripts\server\upgrade-deployment.ps1 -Rebuild
.\scripts\server\rollback-deployment.ps1 -Latest
```

安装或移除 Windows Service 需要管理员 PowerShell。  
Installing or removing the Windows Service requires an elevated PowerShell session.

`restore-database.ps1` 默认只允许离线恢复，不会在服务仍运行时覆盖 SQLite 文件。  
`restore-database.ps1` is intentionally offline-only and refuses to overwrite the SQLite file while the server is still running.

## Workbench GUI / 运维工作台

GUI 启动与打包脚本位于 [`scripts/workbench`](scripts/workbench)：  
GUI entrypoints live under [`scripts/workbench`](scripts/workbench):

- [`start-gui.ps1`](scripts/workbench/start-gui.ps1)
- [`start-gui.cmd`](scripts/workbench/start-gui.cmd)
- [`build-gui.ps1`](scripts/workbench/build-gui.ps1)
- [`build-gui.cmd`](scripts/workbench/build-gui.cmd)

约定如下：  
Rules:

- `start-gui.*` 是标准本地启动方式，会在需要时通过 Tauri 构建 GUI，并写入 bootstrap 文件。  
  `start-gui.*` is the supported local launch path. It builds the GUI through Tauri when needed and writes a bootstrap file.
- `build-gui.*` 是标准交付构建方式，会输出 MSI 和本地便携包。  
  `build-gui.*` is the supported distribution path. It builds the MSI and stages a local portable package.
- 不建议直接运行未经 Tauri 启动流程产出的原始 Rust 输出。  
  Directly launching raw Rust outputs is discouraged unless they were produced through the Tauri build path.

当前工作台范围：  
Current workbench scope:

- dashboard：运行态总览和最近设备  
  Dashboard: runtime summary and recent devices
- devices：设备清单、搜索、筛选  
  Devices: searchable and filterable device inventory
- operations：备份、TLS、CA 导出、观测数据  
  Operations: backup, TLS, CA export, observability metrics
- access：客户端接入参数、Join Bundle 预览、导出交付包  
  Access: client-facing connection inputs, join-bundle preview, exportable handoff package
- logs：GUI 当前会话日志  
  Logs: current GUI session service output
- settings/security：本地配置和环境检查  
  Settings/security: local config, environment checks, TLS/auth inputs

典型用法：  
Typical usage:

```powershell
.\scripts\workbench\start-gui.cmd
.\scripts\workbench\build-gui.cmd
```

## TLS Certificate Lifecycle / TLS 证书生命周期

TLS 材料通常位于 `tls_cert_path`、`tls_key_path` 以及相邻的根 CA 文件：  
TLS artifacts remain local deployment assets under `tls_cert_path`, `tls_key_path`, and the sibling root CA files:

- `roodox-server-cert.pem`
- `roodox-server-key.pem`
- `roodox-ca-cert.pem`
- `roodox-ca-key.pem`

支持的入口：  
Supported entrypoints:

- `certificate-status.ps1`：检查当前证书和根证书有效性与过期时间  
  Inspect current cert/root validity and expiry
- `rotate-certificates.ps1`：轮换服务端叶子证书  
  Rotate the server leaf certificate
- `rotate-certificates.ps1 -RotateRootCA`：同时轮换根 CA 和叶子证书  
  Rotate both the root CA and leaf certificate
- `export-client-ca.ps1`：导出客户端信任根  
  Copy the current client trust root to a handoff path

轮换规则：  
Rotation rules:

- 只轮换叶子证书时，客户端可继续信任原 `roodox-ca-cert.pem`。  
  Leaf-only rotation keeps the existing root CA, so clients can continue trusting the same `roodox-ca-cert.pem`.
- 轮换根 CA 后，必须重新导出并分发新的客户端 CA。  
  Root CA rotation changes the client trust root and requires redistributing the new CA.
- 如果服务正在运行，轮换只会更新磁盘文件；需要单独重启进程或服务。  
  Rotating certificates while the server is running only updates files on disk; restart separately to reload them.

二进制也支持以下一次性参数：  
The server binary also exposes these one-shot admin flags:

- `-tls-status`
- `-rotate-tls`
- `-rotate-tls-root-ca`
- `-tls-backup-dir`
- `-export-client-ca <path>`

## Runtime Config / 运行时配置

进程管理路径通过 `runtime` 配置：  
Process-management paths are configurable through `runtime`:

```json
{
  "runtime": {
    "binary_path": "roodox_server.exe",
    "state_dir": "runtime",
    "pid_file": "runtime/roodox_server.pid",
    "log_dir": "runtime/logs",
    "stdout_log_name": "server.stdout.log",
    "stderr_log_name": "server.stderr.log",
    "graceful_stop_timeout_seconds": 10,
    "windows_service": {
      "name": "RoodoxServer",
      "display_name": "Roodox Server",
      "description": "Roodox gRPC server",
      "start_type": "auto"
    }
  }
}
```

环境变量覆盖项：  
Environment overrides:

- `ROODOX_RUNTIME_BINARY_PATH`
- `ROODOX_RUNTIME_STATE_DIR`
- `ROODOX_RUNTIME_PID_FILE`
- `ROODOX_RUNTIME_LOG_DIR`
- `ROODOX_RUNTIME_STDOUT_LOG_NAME`
- `ROODOX_RUNTIME_STDERR_LOG_NAME`
- `ROODOX_RUNTIME_GRACEFUL_STOP_TIMEOUT_SECONDS`
- `ROODOX_WINDOWS_SERVICE_NAME`
- `ROODOX_WINDOWS_SERVICE_DISPLAY_NAME`
- `ROODOX_WINDOWS_SERVICE_DESCRIPTION`
- `ROODOX_WINDOWS_SERVICE_START_TYPE`

服务还会在 `runtime.state_dir` 下获取部署级锁，避免同一部署被重复启动。  
The server also acquires a deployment-level lock inside `runtime.state_dir` to reject duplicate starts of the same deployment earlier and more clearly.

## Graceful Shutdown / 优雅关闭

`AdminConsoleService/ShutdownServer` 是当前支持的本地优雅关闭入口。  
`AdminConsoleService/ShutdownServer` is the supported control-plane action for local graceful shutdown.

`stop-server.ps1` 会优先尝试这一条路径，只有在控制请求无法送达时才回退到直接停进程。  
`stop-server.ps1` uses this path first and only falls back to direct process stop if the control request cannot be delivered.

服务也会处理这些关闭源：  
The server also handles:

- 控制台 `Ctrl+C` / `SIGTERM`  
  Console `Ctrl+C` / `SIGTERM`
- Windows Service `STOP` / `SHUTDOWN`  
  Windows Service `STOP` / `SHUTDOWN`

## Database Maintenance / 数据库维护

数据库通过 `database` 配置支持周期性 WAL checkpoint 和备份轮换：  
The server supports periodic SQLite WAL checkpointing and backup rotation through `database` config:

```json
{
  "database": {
    "checkpoint_interval_seconds": 300,
    "checkpoint_mode": "truncate",
    "backup_dir": "backups",
    "backup_interval_seconds": 86400,
    "backup_keep_latest": 7
  }
}
```

环境变量覆盖项：  
Environment overrides:

- `ROODOX_DB_CHECKPOINT_INTERVAL_SECONDS`
- `ROODOX_DB_CHECKPOINT_MODE`
- `ROODOX_DB_BACKUP_DIR`
- `ROODOX_DB_BACKUP_INTERVAL_SECONDS`
- `ROODOX_DB_BACKUP_KEEP_LATEST`

行为说明：  
Behavior:

- 按配置间隔执行 checkpoint  
  Checkpoints run on the configured interval
- 按配置间隔执行备份，只保留最新的 `backup_keep_latest` 份  
  Backups run on the configured interval and keep only the newest `backup_keep_latest` files
- `TriggerServerBackup` 会先执行 checkpoint，再创建快照  
  `TriggerServerBackup` forces a checkpoint first, then creates a snapshot
- 运行态快照会暴露 DB、WAL、SHM、最近 checkpoint 和最近备份信息  
  Runtime status exposes DB, WAL, SHM, last checkpoint result, and last backup result

恢复流程：  
Restore flow:

1. 先停止托管进程或 Windows Service。  
   Stop the managed process or Windows Service first.
2. 运行 `restore-database.ps1 -BackupPath <file>` 或 `restore-database.ps1 -Latest`。  
   Run `restore-database.ps1 -BackupPath <file>` or `restore-database.ps1 -Latest`.
3. 恢复流程会执行 SQLite `quick_check`、原子替换数据库，并重新应用 schema migration。  
   The restore path validates the backup with SQLite `quick_check`, replaces the live DB atomically, and reapplies schema migrations.

## Schema Migration / Schema 迁移

SQLite 文件使用 `PRAGMA user_version` 和顺序 migration。  
The SQLite file uses `PRAGMA user_version` with ordered schema migrations.

当前规则：  
Current rules:

- `db.Open(...)` 总会在服务提供流量前应用未完成 migration。  
  `db.Open(...)` always applies pending migrations before serving traffic.
- 新数据库和旧版无版本数据库都会收敛到同一 schema 版本。  
  Fresh databases and legacy pre-versioned databases converge onto the same schema version.
- `NewMetaStore`、`NewVersionStore`、`NewDeviceRegistry` 不再各自拥有建表逻辑。  
  Constructors such as `NewMetaStore`, `NewVersionStore`, and `NewDeviceRegistry` no longer own table creation.

## Install, Upgrade, Rollback / 安装、升级、回滚

部署物保护与数据库备份是分开的：  
Deployment packaging is intentionally separate from database backup/restore:

- 数据库备份/恢复保护状态数据  
  Database backup/restore protects state
- 安装/升级/回滚保护可执行交付物  
  Install/upgrade/rollback protects deployable artifacts

部署快照通常包括：  
Deployable artifact snapshots include:

- `roodox_server.exe`
- 当前配置文件  
  The active config file
- 服务端叶子证书和私钥  
  Server leaf cert/key
- 根 CA 证书和私钥  
  Root CA cert/key

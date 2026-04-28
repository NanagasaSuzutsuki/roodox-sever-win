# Roodox Server Operations

This server now exposes a stable management surface for later GUI work. GUI or external admin tools should consume gRPC management APIs and the standard health service instead of reading SQLite files, log files, or in-memory structures directly.

## Runtime Management Surface

- Health check: standard `grpc.health.v1.Health/Check`
- Admin runtime snapshot: `AdminConsoleService/GetServerRuntime`
- Admin observability snapshot: `AdminConsoleService/GetServerObservability`
- Manual database backup: `AdminConsoleService/TriggerServerBackup`

## Background Service Startup

The supported local process-management entrypoints now live under [`scripts/server`](scripts/server):

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

These scripts standardize:

- background startup with redirected stdout/stderr logs
- PID-file based process ownership
- stale PID cleanup
- deployment-scoped runtime state under `runtime.state_dir`
- graceful local shutdown through the admin gRPC control plane before falling back to process stop
- optional Windows Service registration and SCM-based lifecycle management

Typical usage:

```powershell
.\scripts\server\start-server.ps1
.\scripts\server\status-server.ps1
.\scripts\server\restart-server.ps1
.\scripts\server\stop-server.ps1
```

Useful switches:

- `start-server.ps1 -Rebuild`
- `restart-server.ps1 -Rebuild`
- `stop-server.ps1 -StopUnmanaged`
- `restart-server.ps1 -StopUnmanaged`

Windows Service usage:

```powershell
.\scripts\server\install-windows-service.ps1
.\scripts\server\status-windows-service.ps1
.\scripts\server\start-windows-service.ps1
.\scripts\server\stop-windows-service.ps1
.\scripts\server\uninstall-windows-service.ps1
.\\scripts\\server\\restore-database.ps1 -Latest
.\\scripts\\server\\certificate-status.ps1
.\\scripts\\server\\rotate-certificates.ps1 -RestartAfter
.\\scripts\\server\\export-client-ca.ps1 -DestinationPath .\handoff\roodox-ca-cert.pem
.\\scripts\\server\\upgrade-deployment.ps1 -Rebuild
.\\scripts\\server\\rollback-deployment.ps1 -Latest
```

Installing or removing the Windows Service requires an elevated PowerShell session.

`restore-database.ps1` is intentionally offline-only. It refuses to overwrite the SQLite file while the server process or Windows Service is still running. By default it creates a same-directory `*-pre-restore-*.db` safety copy before replacing the live database, and it can restore either an explicit `-BackupPath` or the newest file under `database.backup_dir` via `-Latest`.

Legacy batch files in the repo root are now compatibility wrappers around `start-server.ps1`.

## Workbench GUI

The supported GUI entrypoints now live under [`scripts/workbench`](scripts/workbench):

- [`start-gui.ps1`](scripts/workbench/start-gui.ps1)
- [`start-gui.cmd`](scripts/workbench/start-gui.cmd)
- [`build-gui.ps1`](scripts/workbench/build-gui.ps1)
- [`build-gui.cmd`](scripts/workbench/build-gui.cmd)

Rules:

- `start-gui.*` is the supported local launch path. It always builds the GUI through Tauri when needed and writes a sidecar bootstrap file so the GUI can resolve the active repo root and config path.
- `build-gui.*` is the supported distribution path. It builds the GUI through Tauri, generates the MSI bundle, and stages a repo-local portable package plus the MSI under `artifacts/workbench`.
- Directly launching raw Rust build outputs should be avoided unless they were produced through the Tauri build path, because the GUI may otherwise fall back to the development `localhost` URL.

Current Workbench scope:

- dashboard: server runtime summary plus recent-device overview
- devices: searchable and filterable device inventory
- operations: backup status, manual backup trigger, TLS status, client CA export, observability metrics
- access: client-facing connection inputs, join-bundle preview, and exportable handoff package
- logs: current GUI session service output
- settings/security: local config, environment checks, TLS/auth inputs

Typical usage:

```powershell
.\scripts\workbench\start-gui.cmd
.\scripts\workbench\build-gui.cmd
```

## TLS Certificate Lifecycle

TLS artifacts remain local deployment assets under `tls_cert_path`, `tls_key_path`, and the sibling root CA files:

- `roodox-server-cert.pem`
- `roodox-server-key.pem`
- `roodox-ca-cert.pem`
- `roodox-ca-key.pem`

Supported entrypoints:

- `certificate-status.ps1`: inspect current cert/root validity and expiry.
- `rotate-certificates.ps1`: rotate the server leaf certificate.
- `rotate-certificates.ps1 -RotateRootCA`: rotate both the root CA and leaf certificate.
- `export-client-ca.ps1`: copy the current client trust root to a handoff path.

Rotation rules:

- Leaf-only rotation keeps the existing root CA, so clients can continue trusting the same `roodox-ca-cert.pem`.
- Root CA rotation changes the client trust root. Export the new CA and redistribute it before restarting clients.
- Rotating certificates while the server is running only updates files on disk. Use `-RestartAfter` or restart the service/process separately so the gRPC server reloads the new certificates.

The server binary also exposes these one-shot admin flags:

- `-tls-status`
- `-rotate-tls`
- `-rotate-tls-root-ca`
- `-tls-backup-dir`
- `-export-client-ca <path>`

`rotate-certificates.ps1` automatically snapshots the previous certificate files into a backup folder before overwriting them.

## Runtime Config

Process-management paths are now configurable through `runtime`:

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

The server also acquires a deployment-level lock inside `runtime.state_dir`, in addition to the database lock, to reject duplicate starts of the same deployment earlier and more clearly.

## Graceful Shutdown

`AdminConsoleService` now exposes `ShutdownServer`, which is the supported control-plane action for local graceful shutdown. `stop-server.ps1` uses this path first, and only falls back to direct process stop if the control request cannot be delivered.

The server also handles:

- console `Ctrl+C` / `SIGTERM`
- Windows Service `STOP` / `SHUTDOWN`

All three paths converge on the same runtime shutdown flow and `runtime.graceful_stop_timeout_seconds`.

These APIs are transport-safe and reusable for CLI, GUI, and remote admin tooling.

## Database Maintenance

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

Environment overrides:

- `ROODOX_DB_CHECKPOINT_INTERVAL_SECONDS`
- `ROODOX_DB_CHECKPOINT_MODE`
- `ROODOX_DB_BACKUP_DIR`
- `ROODOX_DB_BACKUP_INTERVAL_SECONDS`
- `ROODOX_DB_BACKUP_KEEP_LATEST`

Behavior:

- Checkpoints run on the configured interval.
- Backups run on the configured interval and keep only the newest `backup_keep_latest` files.
- `TriggerServerBackup` forces a checkpoint first, then creates a snapshot in `backup_dir`.
- Runtime status exposes DB file, WAL file, SHM file, last checkpoint result, and last backup result.

Restore flow:

- Stop the managed process or Windows Service first.
- Run `restore-database.ps1 -BackupPath <file>` or `restore-database.ps1 -Latest`.
- The restore path validates the backup with SQLite `quick_check`, replaces the live DB atomically, reapplies the current schema migrations, and leaves a pre-restore safety snapshot unless `-NoSafetyBackup` is specified.

## Schema Migration

The SQLite file now uses `PRAGMA user_version` with ordered schema migrations owned by the `internal/db` package.

Current rules:

- `db.Open(...)` always applies pending migrations before the server starts serving traffic.
- Fresh databases and legacy pre-versioned databases both converge onto the same schema version.
- Constructors such as `NewMetaStore`, `NewVersionStore`, and `NewDeviceRegistry` no longer own table creation; schema changes belong in the migration chain.

This is the supported base for future app upgrades. New schema changes should be added as a new migration version instead of sprinkling `CREATE TABLE IF NOT EXISTS` or `ALTER TABLE` calls across service constructors.

## Install, Upgrade, Rollback

Deployment packaging is intentionally separate from database backup/restore.

- Database backup/restore protects state.
- Install/upgrade/rollback protects deployable artifacts.

Deployable artifact snapshots include:

- `roodox_server.exe`
- the active config file
- server leaf cert/key
- root CA cert/key

Supported entrypoints:

- `install-deployment.ps1`
- `upgrade-deployment.ps1`
- `rollback-deployment.ps1`
- `list-release-snapshots.ps1`

Behavior:

- Every install/upgrade creates a release snapshot under `runtime.state_dir/releases`.
- Upgrade stops the managed process or Windows Service, snapshots current deployable files, applies the new binary and optional certificate rotation, then restarts the previous run mode.
- If upgrade fails after the snapshot is taken, the script restores the snapshot before exiting.
- Rollback restores deployable artifacts only. It does not restore the SQLite database; use the database restore flow for that.

## Observability Surface

`GetServerObservability` exposes query-oriented metrics intended for future GUI dashboards:

- `write_file_range_calls`
- `write_file_range_bytes`
- `write_file_range_conflicts`
- `small_write_bursts`
- `small_write_hot_paths`
- build success/failure counts
- build queue wait percentiles
- build duration percentiles
- per-RPC latency summaries

This is the supported way to build dashboard views. Do not infer these values by scraping logs.

## Control Plane And GUI Boundary

Future GUI code should stay outside the server core and only depend on:

- `AdminConsoleService`
- `ControlPlaneService`
- `grpc.health.v1.Health`

The GUI should not:

- access SQLite tables directly
- parse log files for status
- depend on a specific overlay implementation

Overlay/network providers remain deployment-specific and are intentionally isolated behind join bundle and service discovery configuration.

## Overlay Isolation

The join bundle remains provider-neutral:

- `control_plane.join_bundle.overlay_provider`
- `control_plane.join_bundle.overlay_join_config_json`
- `control_plane.join_bundle.service_discovery`

That means switching between EasyTier, Tailscale, IPv6+TLS, or another overlay should only require provider/join configuration changes plus client-side provider implementation changes. Control plane and data plane APIs stay stable.

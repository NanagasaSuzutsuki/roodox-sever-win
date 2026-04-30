# Script Reference / 脚本索引

## Scope / 范围

本章覆盖：

- `scripts/server/*.ps1`
- `scripts/qa/*.ps1`
- `scripts/workbench/*`

`common.ps1` 这类文件是共享函数库，不是直接入口。  
Files such as `common.ps1` are shared helpers, not direct entrypoints.

## Server Scripts / 服务端脚本

### Runtime lifecycle / 进程生命周期

| 脚本 / Script | 关键参数 / Key params | 作用 / Purpose | 备注 / Notes |
| --- | --- | --- | --- |
| `start-server.ps1` | `ConfigPath`, `Foreground`, `BuildIfMissing`, `Rebuild`, `RebuildIfStale`, `StartupSeconds` | 启动服务端进程 | 可前台，也可后台并写 PID/log |
| `status-server.ps1` | `ConfigPath` | 查询本地进程状态 | 读取 PID 和运行状态 |
| `stop-server.ps1` | `ConfigPath`, `TimeoutSeconds`, `Force`, `StopUnmanaged` | 停止服务端进程 | 优先优雅关闭，再按需强停 |
| `restart-server.ps1` | `ConfigPath`, `BuildIfMissing`, `Rebuild`, `StartupSeconds`, `StopTimeoutSeconds`, `Force`, `StopUnmanaged` | 重启服务端 | 组合 stop/start 行为 |

### Windows Service / Windows 服务

| 脚本 / Script | 关键参数 / Key params | 作用 / Purpose | 备注 / Notes |
| --- | --- | --- | --- |
| `install-windows-service.ps1` | `ConfigPath`, `BuildIfMissing`, `Rebuild`, `StartAfterInstall` | 安装 Windows Service | 通常需要管理员权限 |
| `uninstall-windows-service.ps1` | `ConfigPath`, `Force` | 卸载 Windows Service | 可选强制 |
| `start-windows-service.ps1` | `ConfigPath`, `TimeoutSeconds` | 启动 SCM 服务 | 等待启动完成 |
| `stop-windows-service.ps1` | `ConfigPath`, `TimeoutSeconds`, `Force` | 停止 SCM 服务 | 可选强制 |
| `restart-windows-service.ps1` | `ConfigPath`, `TimeoutSeconds`, `Force` | 重启 SCM 服务 | 一次性封装 |
| `status-windows-service.ps1` | `ConfigPath` | 查询 SCM 服务状态 | 只看 Service，不看 unmanaged 进程 |

### TLS, database, deployment / TLS、数据库、部署

| 脚本 / Script | 关键参数 / Key params | 作用 / Purpose | 备注 / Notes |
| --- | --- | --- | --- |
| `certificate-status.ps1` | `ConfigPath`, `RawJson`, `BuildIfMissing`, `Rebuild` | 查询证书状态 | 可输出原始 JSON |
| `rotate-certificates.ps1` | `ConfigPath`, `RotateRootCA`, `RestartAfter`, `BackupDir`, `BuildIfMissing`, `Rebuild` | 轮换服务端证书，必要时轮换根 CA | 轮换根 CA 会影响所有客户端信任 |
| `export-client-ca.ps1` | `ConfigPath`, `DestinationPath`, `BuildIfMissing`, `Rebuild` | 导出客户端 CA 根证书 | 客户端交付常用 |
| `restore-database.ps1` | `ConfigPath`, `BackupPath`, `Latest`, `NoSafetyBackup` | 恢复数据库 | 恢复前通常应停服务 |
| `install-deployment.ps1` | `ConfigPath`, `AsService`, `StartAfterInstall`, `BuildIfMissing`, `Rebuild` | 安装部署 | 可直接注册为 Service |
| `upgrade-deployment.ps1` | `ConfigPath`, `Label`, `BuildIfMissing`, `Rebuild`, `RotateServerCert`, `RotateRootCA`, `StartAfterUpgrade` | 做升级快照并升级 | 需要确保没有 unmanaged 进程占用 |
| `rollback-deployment.ps1` | `ConfigPath`, `SnapshotLabel`, `Latest`, `StartAfterRollback` | 从升级快照回滚 | 回滚前也要保证文件未被占用 |
| `list-release-snapshots.ps1` | `ConfigPath` | 列出升级快照 | 用于回滚前选快照 |
| `validate-deployment-lifecycle.ps1` | `ConfigPath`, `Rebuild`, `KeepArtifacts` | 验证部署生命周期 | 偏 QA/冒烟验证 |

## QA Scripts / QA 脚本

| 脚本 / Script | 关键参数 / Key params | 作用 / Purpose |
| --- | --- | --- |
| `run-live-regression.ps1` | `ConfigPath`, `KeepArtifacts` | 跑一次活体回归 |
| `run-fault-injection.ps1` | `ConfigPath`, `KeepArtifacts` | 跑一次故障注入验证 |
| `run-soak.ps1` | `ConfigPath`, `Duration`, `Workers`, `BuildInterval`, `KeepArtifacts` | 长稳压测，`BuildInterval=0` 可禁用构建压测 |
| `run-restart-recovery.ps1` | `ConfigPath`, `PreSeconds`, `DownSeconds`, `PostSeconds`, `KeepLogs`, `CaptureRestartServerLogs` | 重启恢复探针 |
| `run-full-qa.ps1` | `ConfigPath`, `SoakDuration`, `SoakWorkers`, `BuildInterval` | 串联一整套 QA，必要时自动拉起 smoke 服务 |

这些脚本的本质是给 `cmd/roodox_qa` 套一层 Windows 友好的参数和流程。  
They are Windows-friendly wrappers over `cmd/roodox_qa`.

## Workbench Scripts / GUI 脚本

| 脚本 / Script | 关键参数 / Key params | 作用 / Purpose | 备注 / Notes |
| --- | --- | --- | --- |
| `build-gui.ps1` | `ConfigPath` | 构建 Workbench | PowerShell 入口 |
| `start-gui.ps1` | `ConfigPath`, `BuildIfMissing`, `Rebuild`, `Wait` | 启动 Workbench | 可按需先构建 |
| `build-gui.cmd` | none | 调用 PowerShell 构建脚本 | 方便双击或 CMD 调用 |
| `start-gui.cmd` | none | 调用 PowerShell 启动脚本 | 方便双击或 CMD 调用 |

## Common Failure Modes / 常见失败场景

| 场景 / Scenario | 常见表现 / Symptom | 处理思路 / Fix direction |
| --- | --- | --- |
| 升级前仍有 unmanaged 进程 | `unmanaged server processes are running` | 先停掉前台/残留进程，再跑升级 |
| 可执行文件被占用 | `roodox_server.exe is being used by another process` | 先停服务和残留进程，再执行升级或回滚 |
| 证书轮换后客户端失败 | TLS 校验报错 | 重新交付新的 CA 根证书或确认 server name |
| 数据库恢复失败 | 备份路径不对或文件仍被占用 | 先停服务，确认目标备份文件，再恢复 |
| GUI 启动找不到配置 | Workbench 无法发现项目根 | 检查 `roodox-workbench.bootstrap.json` 或 `ConfigPath` |

## Maintainer Notes / 维护备注

- `upgrade-deployment.ps1` 和 `rollback-deployment.ps1` 涉及文件替换，最怕“进程没停干净”。  
  File replacement workflows are most sensitive to leftover processes.
- `RotateRootCA` 不是普通小改动，它会改变客户端信任根。  
  Rotating the root CA is a client-impacting change.
- 如果你新增了管理型 CLI 能力，通常需要同步补一个脚本入口，否则现场运维会退化成手敲命令。  
  New admin CLI capabilities often deserve a wrapper script for field operations.

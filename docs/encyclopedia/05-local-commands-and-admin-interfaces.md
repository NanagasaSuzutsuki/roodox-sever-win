# Local Commands And Admin Interfaces / 本地命令与管理接口

## Scope / 范围

本章覆盖：

- `roodox_server.exe` 的 CLI 管理开关
- `cmd/roodox_qa` 的本地 QA 子命令
- `cmd/testclient` 的环境变量接入方式
- Workbench Tauri 暴露命令

## `roodox_server.exe` Flags / 服务端 CLI 开关

### Lifecycle and service / 生命周期与服务

| Flag | 作用 / Effect | 备注 / Notes |
| --- | --- | --- |
| `-config` | 指定配置文件路径 | 默认是 `roodox.config.json` |
| `-request-shutdown` | 请求已运行实例优雅关闭 | 不启动主服务，发完即退 |
| `-shutdown-reason` | 给优雅关闭请求附带原因 | 默认 `local admin request` |
| `-service-name` | 覆盖 Windows Service 名称 | 主要用于 SCM 场景 |

### Database and recovery / 数据库与恢复

| Flag | 作用 / Effect | 备注 / Notes |
| --- | --- | --- |
| `-restore-db-from` | 从指定备份文件恢复 SQLite | 恢复后退出 |
| `-restore-db-no-safety-backup` | 恢复前不做安全备份 | 风险高，仅在确定时使用 |
| `-trigger-server-backup-json` | 触发一次备份并输出 JSON | 管理型 CLI 入口 |

### TLS and trust / TLS 与信任

| Flag | 作用 / Effect | 备注 / Notes |
| --- | --- | --- |
| `-tls-status` | 输出 TLS 状态 JSON | 包含证书路径、主题、有效期 |
| `-rotate-tls` | 轮换服务端证书 | 通常搭配重启使用 |
| `-rotate-tls-root-ca` | 与 `-rotate-tls` 一起轮换根 CA | 会改变客户端信任根 |
| `-tls-backup-dir` | 为证书轮换提供备份目录 | 可选 |
| `-export-client-ca` | 导出客户端要信任的根证书 | 交付客户端常用 |

### Client handoff and Workbench JSON / 客户端交付与 Workbench JSON

| Flag | 作用 / Effect | 备注 / Notes |
| --- | --- | --- |
| `-issue-join-bundle-json` | 输出 Join Bundle JSON | 管理面常用，高敏感 |
| `-join-device-id` | 生成 Bundle 时嵌入设备 ID | 可选 |
| `-join-device-name` | 生成 Bundle 时嵌入设备名 | 可选 |
| `-join-device-role` | 生成 Bundle 时嵌入设备角色 | 可选 |
| `-join-device-group` | 生成 Bundle 时覆盖设备组 | 可选 |
| `-workbench-snapshot-json` | 输出 GUI 友好的运行时快照 | Workbench 调用 |
| `-workbench-observability-json` | 输出 GUI 友好的观测快照 | Workbench 调用 |

## `cmd/roodox_qa` / QA 命令行

用法：

```powershell
go run ./cmd/roodox_qa <live|soak|faults|probe> [flags]
```

### Shared override flags / 共享覆盖参数

| Flag | 作用 / Effect |
| --- | --- |
| `-config` | 指定配置文件 |
| `-addr` | 覆盖拨号地址 |
| `-root-dir` | 覆盖 QA 使用的 `root_dir` |
| `-shared-secret` | 覆盖连接使用的共享密钥 |
| `-tls-root-cert` | 覆盖 TLS 根证书路径 |
| `-tls-server-name` | 覆盖 TLS server name |
| `-server-id` | 覆盖注册时使用的 `server_id` |

### Subcommands / 子命令

| Subcommand | 关键参数 / Key flags | 作用 / Purpose |
| --- | --- | --- |
| `live` | `-keep-artifacts` | 活体回归验证 |
| `soak` | `-duration`, `-workers`, `-build-interval`, `-backup-once`, `-keep-artifacts` | 长稳压测 |
| `faults` | `-keep-artifacts` | 故障注入验证 |
| `probe` | `-pre`, `-down`, `-post`, `-interval` | 重启恢复探针 |

## `cmd/testclient` / 最小接入样例

这个程序固定拨号 `127.0.0.1:50051`，通过环境变量决定 TLS 与认证参数。  
This sample dials `127.0.0.1:50051` and uses environment variables for TLS/auth settings.

| 环境变量 / Env | 作用 / Effect |
| --- | --- |
| `ROODOX_SHARED_SECRET` | 共享密钥 |
| `ROODOX_TLS_ENABLED` | 是否启用 TLS |
| `ROODOX_TLS_ROOT_CERT_PATH` | TLS 根证书路径 |
| `ROODOX_TLS_SERVER_NAME` | TLS server name |

它会做两件事：

1. `WriteFile("test.txt", "hello roodox")`
2. `GetHistory("test.txt")`

这说明它更像连通性样例，而不是完整客户端。  
It is a connectivity sample, not a full client.

## Workbench Tauri Commands / Workbench 本地命令

这些命令定义在 `workbench/src-tauri/src/main.rs`，由前端 `workbench/src/App.tsx` 调用。  
These commands are defined in `workbench/src-tauri/src/main.rs` and invoked by `workbench/src/App.tsx`.

| Command | 作用 / Effect | 后端动作 / Backend action | 敏感性 / Sensitivity |
| --- | --- | --- | --- |
| `load_config` | 读取配置 | 直接读 JSON 并抽取 GUI 关心的字段 | 高 |
| `save_config` | 保存配置 | 回写 JSON | 高 |
| `read_logs` | 读取本地 Workbench 收集的日志行 | 读本地日志缓冲 | 中 |
| `load_workbench_snapshot` | 读取运行时快照 | 调 `roodox_server.exe -workbench-snapshot-json` | 高 |
| `load_workbench_observability` | 读取观测快照 | 调 `roodox_server.exe -workbench-observability-json` | 中 |
| `load_tls_status` | 读取 TLS 状态 | 调 `roodox_server.exe -tls-status` | 高 |
| `trigger_server_backup` | 触发服务器备份 | 调 `roodox_server.exe -trigger-server-backup-json` | 高 |
| `export_client_ca` | 导出客户端 CA | 调 `roodox_server.exe -export-client-ca` | 高 |
| `issue_join_bundle` | 生成 Join Bundle 预览 | 调 `roodox_server.exe -issue-join-bundle-json` | 高 |
| `export_client_access_bundle` | 导出 Join Bundle 与可选 CA | 本地写交付目录，并在 TLS 下附带导出的 CA | 高 |
| `start_server` | 启动服务 | 调本地启动脚本/命令 | 高 |
| `stop_server` | 停止服务 | 调本地停止逻辑 | 高 |
| `server_status` | 查询服务状态 | 结合本地状态与检测结果 | 中 |
| `check_environment` | 检查依赖工具 | 查 winget 与构建工具 | 中 |
| `install_missing_tools` | 尝试安装缺失工具 | 走 winget 安装 | 高 |

## Workbench Export Defaults / Workbench 默认导出位置

| 功能 / Function | 默认路径 / Default path |
| --- | --- |
| 导出客户端 CA | `artifacts/handoff/roodox-ca-cert.pem` |
| 导出客户端接入包 | `artifacts/handoff/client-access/` |
| Join Bundle 文件名 | `roodox-client-access.json` |

如果 `bundle.use_tls=true`，`export_client_access_bundle` 会同时导出 `roodox-ca-cert.pem`。  
When `bundle.use_tls=true`, the access export also includes `roodox-ca-cert.pem`.

## Bootstrap and discovery / Bootstrapping 与发现

Workbench 还会读取一个本地 bootstrap 文件：

- `roodox-workbench.bootstrap.json`

它可以提供：

- `project_root`
- `config_path`

这使 GUI 能在不同目录布局下找到项目根和配置文件。  
This allows the GUI to locate the project root and config path under different layouts.

## Maintainer Notes / 维护备注

- 任何新加的管理型 CLI 开关，如果未来要进 GUI，通常还要在 `main.rs` 里再包一层 Tauri command。  
  New admin CLI flags usually need a Tauri wrapper to reach the GUI.
- `issue_join_bundle` 和 `export_client_access_bundle` 是两个不同层级：前者偏“查看”，后者偏“落盘交付”。  
  One previews, the other writes deliverables.

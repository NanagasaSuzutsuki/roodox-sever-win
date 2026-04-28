# Source Map / 源码地图

## Top-Level Map / 顶层地图

| 目录 / Path | 作用 / Responsibility |
| --- | --- |
| `cmd/roodox_server` | 服务端主入口，CLI 管理开关 |
| `cmd/roodox_qa` | QA 命令行入口 |
| `cmd/testclient` | 极简连接样例 |
| `client/` | Go 客户端包装 |
| `internal/accessbundle` | Join Bundle 模型与 JSON 导出 |
| `internal/analyze` | 构建单元分析 |
| `internal/appconfig` | 配置结构、默认值、环境变量覆盖 |
| `internal/cleanup` | 清理通用逻辑 |
| `internal/conflict` | 冲突文件相关逻辑 |
| `internal/db` | SQLite 打开、迁移、设备/版本持久化 |
| `internal/fs` | 文件系统辅助 |
| `internal/lock` | 锁相关逻辑 |
| `internal/observability` | 观测指标聚合 |
| `internal/qasuite` | QA 场景实现 |
| `internal/server` | gRPC 服务实现 |
| `internal/serverapp` | 运行时组装、Service、维护、观测、清理 |
| `proto/` | protobuf 与 gRPC 定义 |
| `scripts/` | 运维、QA、GUI 包装脚本 |
| `workbench/` | Tauri + React GUI |

## `cmd/` Entrypoints / 入口程序

| 文件 / File | 作用 / Responsibility |
| --- | --- |
| `cmd/roodox_server/main.go` | 主二进制入口；处理 `-tls-status`, `-issue-join-bundle-json`, `-trigger-server-backup-json` 等管理开关 |
| `cmd/roodox_qa/main.go` | QA 子命令路由：`live`, `soak`, `faults`, `probe` |
| `cmd/testclient/main.go` | 通过环境变量配置 TLS/secret 的最小拨号样例 |

## `client/` / Go 客户端

| 文件 / File | 作用 / Responsibility |
| --- | --- |
| `client/roodox_client.go` | 拨号、TLS root 加载、共享密钥 metadata、各服务 RPC 包装 |

如果你要改客户端连接认证方式，第一站就是这个文件。  
If you change client auth or dialing behavior, start here.

## `internal/accessbundle`

| 文件 / File | 作用 / Responsibility |
| --- | --- |
| `bundle.go` | Bundle 规范化、校验、导出为客户端 JSON 文件 |
| `bundle_test.go` | Bundle 结构测试 |

如果你要改客户端交付文件格式，这是主入口。  
This is the main entry for client handoff format changes.

## `internal/appconfig`

| 文件 / File | 作用 / Responsibility |
| --- | --- |
| `config.go` | 配置结构、默认值、路径派生、环境变量覆盖、保存/加载 |

改配置字段时，通常必须同时改：

- `internal/appconfig/config.go`
- `roodox.config.example.json`
- `workbench/src-tauri/src/main.rs`
- `workbench/src/App.tsx`

## `internal/server`

| 文件 / File | 作用 / Responsibility |
| --- | --- |
| `core_service.go` | `CoreService` 文件面 |
| `sync_service.go` | `SyncService` 同步面 |
| `lock_service.go` | `LockService` 路径锁 |
| `version_service.go` | `VersionService` 历史版本 |
| `analyze_service.go` | `AnalyzeService` 构建分析 |
| `build_service.go` | `BuildService` 构建任务 |
| `control_plane_service.go` | `ControlPlaneService` 设备控制面 |
| `control_plane_support.go` | 控制面辅助结构和转换 |
| `admin_console_service.go` | `AdminConsoleService` 管理面 |
| `admin_runtime.go` | 管理面 runtime provider 结构 |
| `security.go` | TLS、共享密钥认证、证书导出、证书轮换 |
| `runtime_metrics.go` | 运行时观测指标接口 |
| `path_normalization.go` | 路径标准化 |
| `path_locker.go` | 路径锁辅助 |
| `file_mutation_support.go` | 文件变更辅助逻辑 |
| `preflight.go` | 预检查逻辑 |
| `cleanup_hooks.go` | 清理钩子 |
| `logging.go` | 请求日志与指标记录 |
| `error.go` | gRPC 错误转换 |

## `internal/serverapp`

| 文件 / File | 作用 / Responsibility |
| --- | --- |
| `runtime.go` | 运行时启动、停止、等待 |
| `runtime_admin.go` | 运行时快照、观测快照、备份、关停入口 |
| `control_plane_config.go` | 从 app config 生成 server control-plane config |
| `database_maintenance.go` | checkpoint 与备份维护 |
| `observability_reporter.go` | 观测快照与上报辅助 |
| `artifact_janitor.go` | 临时工件清理 |
| `file_janitors.go` | 构建目录、冲突文件、日志清理 |
| `log_trigger_writer.go` | 日志触发写入辅助 |
| `instance_lock_windows.go` | Windows 单实例锁 |
| `instance_lock_other.go` | 非 Windows 单实例锁 |
| `service_windows.go` | Windows Service 集成 |
| `service_other.go` | 非 Windows service 适配 |

## `internal/db`

| 文件 / File | 作用 / Responsibility |
| --- | --- |
| `db.go` | SQLite 打开、WAL、checkpoint、备份基础能力 |
| `migrations.go` | schema 迁移定义 |
| `meta.go` | 文件元数据读写 |
| `version.go` | 版本记录与内容存储 |
| `device_registry.go` | 设备注册、心跳、同步状态、基础查询 |
| `device_registry_admin.go` | 策略覆盖、动作队列、诊断持久化 |
| `restore.go` | 从备份恢复数据库 |
| `resource_lock_windows.go` | Windows 锁持久化 |
| `resource_lock_other.go` | 非 Windows 锁持久化 |

## `workbench/`

| 文件 / File | 作用 / Responsibility |
| --- | --- |
| `workbench/src/App.tsx` | 前端页面、表单、视图、调用 Tauri commands |
| `workbench/src/main.tsx` | 前端入口 |
| `workbench/src/styles.css` | GUI 样式 |
| `workbench/src-tauri/src/main.rs` | Tauri 后端命令、配置读写、调用服务端 CLI |

## Change Recipes / 改动路线图

| 你要改什么 / Goal | 先看哪几处 / Start here |
| --- | --- |
| 文件读写行为 | `proto/roodox_core.proto`, `internal/server/core_service.go`, `internal/server/sync_service.go`, `internal/db/meta.go`, `internal/db/version.go` |
| 认证或 TLS | `internal/server/security.go`, `client/roodox_client.go`, `scripts/server/rotate-certificates.ps1`, `scripts/server/export-client-ca.ps1` |
| Join Bundle 字段 | `internal/accessbundle/bundle.go`, `internal/server/admin_console_service.go`, `internal/serverapp/control_plane_config.go`, `workbench/src-tauri/src/main.rs` |
| 设备策略或诊断 | `internal/server/control_plane_service.go`, `internal/server/admin_console_service.go`, `internal/db/device_registry*.go` |
| 新增 GUI 按钮 | `workbench/src/App.tsx`, `workbench/src-tauri/src/main.rs`, 可能还要改 CLI 或 gRPC |
| 新增配置字段 | `internal/appconfig/config.go`, `roodox.config.example.json`, `workbench/src/App.tsx`, 文档 |

## Maintainer Notes / 维护备注

- 大部分“看起来只是前端展示”的需求，最后都会落到 `main.rs` 和 `cmd/roodox_server/main.go` 的管理型 CLI 接口。  
  Many UI-only requests ultimately require changes in admin CLI surfaces.
- 涉及路径、内容、版本的改动，最终几乎一定会触到 `internal/db`。  
  Path/content/version changes almost always touch `internal/db`.

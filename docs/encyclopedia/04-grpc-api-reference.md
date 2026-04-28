# gRPC API Reference / gRPC 接口总表

## Scope / 范围

本文覆盖 `proto/roodox_core.proto` 里定义的全部服务与消息视角下的接口语义。  
This chapter covers the full service surface defined in `proto/roodox_core.proto`.

包名：

- `roodox.core.v1`

## Transport Rules / 传输规则

- 服务监听地址来自 `addr`。  
  Listen address comes from `addr`.
- 启用 TLS 时，客户端应使用导出的 CA 根证书校验服务端。  
  When TLS is enabled, clients should trust the exported CA root.
- 启用共享密钥认证时，请求 metadata 里需要带 `x-roodox-secret`。  
  When shared-secret auth is enabled, requests must include `x-roodox-secret`.
- 共享密钥只做应用层认证，不替代 TLS。  
  The shared secret is application auth, not transport encryption.

## Service Index / 服务索引

| 服务 / Service | 主要调用方 / Typical caller | 是否写数据 / Writes data? | 敏感性 / Sensitivity |
| --- | --- | --- | --- |
| `CoreService` | 客户端同步/挂载层 | 是 | 高 |
| `SyncService` | 客户端同步层 | 是 | 高 |
| `LockService` | 客户端协同写入层 | 是 | 中 |
| `VersionService` | 客户端/管理端 | 否 | 高 |
| `AnalyzeService` | 构建客户端或服务端工具 | 否 | 中 |
| `BuildService` | 客户端/运维工具 | 是 | 高 |
| `ControlPlaneService` | 客户端代理 / device agent | 是 | 高 |
| `AdminConsoleService` | 管理员 CLI / Workbench | 是 | 高 |

## Shared Structures / 共享结构

这些结构会在多个 RPC 里复用，先统一记住：

| 结构 / Type | 字段 / Fields |
| --- | --- |
| `FileInfo` | `path`, `name`, `is_dir`, `size`, `mtime_unix`, `version`, `hash` |
| `FileMeta` | `path`, `version`, `mtime_unix`, `hash`, `size` |
| `VersionRecord` | `version`, `mtime_unix`, `hash`, `size`, `client_id`, `change_type` |
| `DeviceSummary` | `device_id`, `display_name`, `role`, `overlay_provider`, `overlay_address`, `online_state`, `last_seen_at`, `sync_state`, `mount_state`, `client_version`, `policy_revision` |
| `AssignedConfigPolicy` | `mount_path`, `sync_roots`, `conflict_policy`, `read_only`, `auto_connect`, `bandwidth_limit`, `log_level`, `large_file_threshold` |
| `ClientAction` | `action_id`, `action_type`, `payload_json`, `status`, `requested_at_unix`, `delivered_at_unix`, `completed_at_unix` |
| `DiagnosticSummary` | `diagnostics_id`, `category`, `content_type`, `summary`, `size_bytes`, `uploaded_at_unix` |
| `JoinBundle` | `version`, `overlay_provider`, `overlay_join_config_json`, `service_discovery_mode`, `service_host`, `service_port`, `use_tls`, `tls_server_name`, `server_id`, `device_group`, `shared_secret`, `device_id`, `device_name`, `device_role` |
| `FileStatSummary` | `path`, `exists`, `size_bytes`, `modified_at_unix` |
| `DatabaseCheckpointStatus` | `last_checkpoint_at_unix`, `mode`, `busy_readers`, `log_frames`, `checkpointed_frames`, `last_error` |
| `DatabaseBackupStatus` | `dir`, `interval_seconds`, `keep_latest`, `last_backup_at_unix`, `last_backup_path`, `last_error` |
| `HotPathMetric` | `path`, `count` |
| `RpcLatencyMetric` | `method`, `count`, `error_count`, `p50_ms`, `p95_ms`, `p99_ms` |
| `BuildObservability` | `success_count`, `failure_count`, `log_bytes`, `queue_wait_count`, `queue_wait_p50_ms`, `queue_wait_p95_ms`, `queue_wait_p99_ms`, `duration_count`, `duration_p50_ms`, `duration_p95_ms`, `duration_p99_ms` |

## `CoreService`

| RPC | Request 字段 / Request fields | Response 字段 / Response fields | 说明 / Notes |
| --- | --- | --- | --- |
| `ListDir` | `path` | `entries[]: FileInfo` | 列目录，返回 metadata，不返回文件内容 |
| `Stat` | `path` | `info: FileInfo` | 查询单路径元数据 |
| `ReadFile` | `path` | `data` | 读取整文件字节 |
| `ReadFileRange` | `path`, `offset`, `length` | `data`, `file_size` | 区间读取 |
| `WriteFileRange` | `path`, `offset`, `data`, `base_version` | `bytes_written`, `file_size`, `new_version`, `conflicted`, `conflict_path` | 带版本感知的区间写 |
| `SetFileSize` | `path`, `size`, `base_version` | `file_size`, `new_version`, `conflicted`, `conflict_path` | 截断或扩展文件 |
| `Delete` | `path` | empty | 删除文件或目录入口 |
| `Rename` | `old_path`, `new_path` | empty | 重命名 |
| `Mkdir` | `path` | empty | 创建目录 |

## `SyncService`

| RPC | Request 字段 / Request fields | Response 字段 / Response fields | 说明 / Notes |
| --- | --- | --- | --- |
| `GetFileMeta` | `path` | `meta: FileMeta` | 获取单文件版本信息 |
| `WriteFile` | `path`, `data`, `base_version` | `conflicted`, `conflict_path`, `new_version` | 整文件写入 |
| `WriteFileStream` | stream of `WriteFileChunk{path,base_version,data}` | `conflicted`, `conflict_path`, `new_version` | 流式整文件写入 |
| `ListChangedFiles` | `since_mtime_unix`, `since_version` | `metas[]: FileMeta` | 列出变更文件 |

## `LockService`

| RPC | Request 字段 / Request fields | Response 字段 / Response fields | 说明 / Notes |
| --- | --- | --- | --- |
| `AcquireLock` | `path`, `client_id`, `ttl_seconds` | `ok`, `owner`, `expire_at` | 获取路径锁 |
| `RenewLock` | `path`, `client_id`, `ttl_seconds` | `ok`, `expire_at` | 续租路径锁 |
| `ReleaseLock` | `path`, `client_id` | `ok` | 释放路径锁 |

## `VersionService`

| RPC | Request 字段 / Request fields | Response 字段 / Response fields | 说明 / Notes |
| --- | --- | --- | --- |
| `GetHistory` | `path` | `records[]: VersionRecord` | 返回版本历史摘要 |
| `GetVersion` | `path`, `version` | `data` | 返回指定版本的内容字节 |

## `AnalyzeService`

| RPC | Request 字段 / Request fields | Response 字段 / Response fields | 说明 / Notes |
| --- | --- | --- | --- |
| `AnalyzeBuildUnits` | `root` | `units[]: BuildUnit{path,type}` | 分析构建单元 |

## `BuildService`

| RPC | Request 字段 / Request fields | Response 字段 / Response fields | 说明 / Notes |
| --- | --- | --- | --- |
| `StartBuild` | `unit_path`, `target` | `build_id` | 启动构建任务 |
| `GetBuildStatus` | `build_id` | `status`, `error`, `started_at_unix`, `finished_at_unix`, `product_name` | 查询状态 |
| `FetchBuildLog` | `build_id` | `text` | 获取构建日志文本 |
| `GetBuildProduct` | `build_id` | `name`, `data` | 获取构建产物 |

## `ControlPlaneService`

| RPC | Request 字段 / Request fields | Response 字段 / Response fields | 说明 / Notes |
| --- | --- | --- | --- |
| `RegisterDevice` | `device_id`, `device_name`, `device_role`, `client_version`, `platform`, `overlay_provider`, `overlay_address`, `capabilities[]`, `server_id`, `device_group` | `accepted`, `assigned_device_label`, `heartbeat_interval_seconds`, `policy_revision`, `requires_policy_pull` | 设备首次注册或重注册 |
| `Heartbeat` | `device_id`, `session_id`, `timestamp_unix`, `overlay_connected`, `grpc_connected`, `last_sync_time_unix`, `last_error`, `mount_state`, `sync_state_summary` | `next_heartbeat_seconds`, `policy_revision`, `pending_actions[]`, `pending_action_details[]: ClientAction` | 心跳与动作拉取 |
| `GetAssignedConfig` | `device_id` | `mount_path`, `sync_roots[]`, `conflict_policy`, `read_only`, `auto_connect`, `bandwidth_limit`, `log_level`, `large_file_threshold`, `policy_revision` | 拉取设备策略 |
| `ReportSyncState` | `device_id`, `current_task_count`, `last_success_time`, `last_error`, `conflict_count`, `queue_depth`, `summary` | empty | 上报同步状态 |
| `ReportMountState` | `device_id`, `mounted`, `mount_path`, `last_mount_time`, `last_error` | empty | 上报挂载状态 |
| `UploadDiagnostics` | `device_id`, `category`, `content_type`, `summary`, `data` | `diagnostics_id`, `uploaded_at_unix`, `size_bytes` | 上传诊断负载，敏感度高 |

## `AdminConsoleService`

| RPC | Request 字段 / Request fields | Response 字段 / Response fields | 说明 / Notes |
| --- | --- | --- | --- |
| `ListDevices` | empty | `devices[]: DeviceSummary` | 列出设备摘要 |
| `GetDeviceDetail` | `device_id` | `summary`, `device_name`, `platform`, `capabilities[]`, `server_id`, `device_group`, `session_id`, `overlay_connected`, `grpc_connected`, `last_error`, `last_sync_time_unix`, `current_task_count`, `sync_last_success_time`, `sync_last_error`, `conflict_count`, `queue_depth`, `sync_summary`, `mounted`, `mount_path`, `last_mount_time_unix`, `mount_last_error`, `assigned_config`, `pending_actions[]`, `recent_diagnostics[]`, `available_actions[]`, `requires_policy_pull`, `last_registered_at`, `last_heartbeat_at`, `last_sync_report_at`, `last_mount_report_at` | 设备全量详情 |
| `IssueJoinBundle` | `device_id`, `device_name`, `device_role`, `device_group`, `overlay_provider`, `overlay_join_config_json` | `bundle_json`, `bundle: JoinBundle` | 生成客户端交付包 |
| `UpdateDevicePolicy` | `device_id`, `policy: AssignedConfigPolicy`, `expected_policy_revision`, `reset_to_default` | `policy_revision`, `requires_policy_pull`, `effective_policy: AssignedConfigPolicy` | 更新或重置设备策略 |
| `RequestClientAction` | `device_id`, `action_type`, `payload_json`, `replace_similar_pending` | `action: ClientAction` | 排队客户端动作 |
| `GetServerRuntime` | empty | `server_id`, `listen_addr`, `root_dir`, `db_path`, `tls_enabled`, `auth_enabled`, `started_at_unix`, `health_state`, `health_message`, `db_file: FileStatSummary`, `wal_file: FileStatSummary`, `shm_file: FileStatSummary`, `checkpoint: DatabaseCheckpointStatus`, `backup: DatabaseBackupStatus` | 查询服务端运行态 |
| `GetServerObservability` | empty | `write_file_range_calls`, `write_file_range_bytes`, `write_file_range_conflicts`, `small_write_bursts`, `small_write_hot_paths[]: HotPathMetric`, `build: BuildObservability`, `rpc_metrics[]: RpcLatencyMetric` | 查询观测指标 |
| `TriggerServerBackup` | empty | `created_at_unix`, `path` | 立即创建 DB 备份 |
| `ShutdownServer` | `reason` | `accepted`, `already_in_progress`, `requested_at_unix`, `message` | 请求优雅关停 |

## Message Types Used Only Once / 单次使用结构

| 类型 / Type | 字段 / Fields | 用途 / Use |
| --- | --- | --- |
| `BuildUnit` | `path`, `type` | 构建单元描述 |
| `WriteFileChunk` | `path`, `base_version`, `data` | 流式写文件 chunk |
| `RegisterDeviceResponse` | `accepted`, `assigned_device_label`, `heartbeat_interval_seconds`, `policy_revision`, `requires_policy_pull` | 注册结果 |
| `UploadDiagnosticsResponse` | `diagnostics_id`, `uploaded_at_unix`, `size_bytes` | 诊断写入结果 |
| `TriggerServerBackupResponse` | `created_at_unix`, `path` | 备份创建结果 |
| `ShutdownServerResponse` | `accepted`, `already_in_progress`, `requested_at_unix`, `message` | 关停请求结果 |

## Maintainer Notes / 维护备注

- 改 RPC 时，先改 `proto/roodox_core.proto`，再同步 Go 服务实现、客户端包装、Workbench 本地调用和文档。  
  Change the proto first, then propagate to server, client, Workbench, and docs.
- `IssueJoinBundle`、`UploadDiagnostics`、`GetServerRuntime`、`TriggerServerBackup` 是隐私风险最高的几个管理面 RPC。  
  These are among the highest-risk admin/control-plane RPCs.
- `VersionService/GetVersion` 和数据库备份都能拿到历史内容，因此版本系统不只是审计面，也是数据暴露面。  
  Version retrieval and backups expose historical content, not just metadata.

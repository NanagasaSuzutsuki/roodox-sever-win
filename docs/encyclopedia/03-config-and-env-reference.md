# Config And Environment Reference / 配置与环境变量总表

## Resolution Order / 配置解析顺序

Roodox 的配置不是只有一层。实际顺序是：

1. `internal/appconfig/config.go` 里的默认值
2. `roodox.config.json` 文件内容
3. `ROODOX_*` 环境变量覆盖
4. 路径归一化和派生逻辑

因此你看到的“最终生效值”可能不是 JSON 里写的原文。  
The final effective value may differ from the raw JSON because environment overrides and path derivation run afterward.

## Path Rules / 路径规则

- 默认配置文件名：`roodox.config.json`  
  Default config file name: `roodox.config.json`
- 相对路径相对于配置文件所在目录解析。  
  Relative paths are resolved from the config file directory.
- 如果设置了 `data_root`，一些默认路径会自动挂到这个目录下。  
  If `data_root` is set, several default storage paths are derived under it.

### `data_root` 影响的默认派生路径

| 逻辑项 / Logical item | 默认值 / Default | 设置 `data_root` 后 / Derived under `data_root` |
| --- | --- | --- |
| `db_path` | `roodox.db` | `data_root/roodox.db` |
| `runtime.state_dir` | `runtime` | `data_root/runtime` |
| `runtime.pid_file` | `runtime/roodox_server.pid` | `data_root/runtime/roodox_server.pid` |
| `runtime.log_dir` | `runtime/logs` | `data_root/runtime/logs` |
| `tls_cert_path` | `certs/roodox-server-cert.pem` | `data_root/certs/roodox-server-cert.pem` |
| `tls_key_path` | `certs/roodox-server-key.pem` | `data_root/certs/roodox-server-key.pem` |
| `database.backup_dir` | `backups` | `data_root/backups` |

## Top-Level JSON Fields / 顶层字段

| 字段 / Field | 类型 / Type | 默认 / Default | 说明 / Notes | 敏感性 / Sensitivity |
| --- | --- | --- | --- | --- |
| `addr` | string | `:50051` | gRPC 监听地址 | 中 |
| `data_root` | string | empty | 运行时数据根目录 | 高 |
| `root_dir` | string | code default `D:/RoodoxShare`, example uses `share` | 实际文件共享根目录 | 高 |
| `db_path` | string | `roodox.db` | SQLite 主库路径 | 高 |
| `runtime` | object | see below | 运行时二进制、日志、PID、Service | 高 |
| `remote_build_enabled` | bool | `true` | 是否允许远程构建接口 | 中 |
| `build_tool_dirs` | string[] | empty | 额外构建工具搜索目录 | 中 |
| `required_build_tools` | string[] | `cmake,make,build-essential` | 构建工具必需列表 | 低 |
| `auth_enabled` | bool | `false` | 是否启用共享密钥认证 | 高 |
| `shared_secret` | string | empty | 共享密钥 | 高 |
| `tls_enabled` | bool | `false` | 是否启用 TLS | 高 |
| `tls_cert_path` | string | `certs/roodox-server-cert.pem` | 服务端证书路径 | 高 |
| `tls_key_path` | string | `certs/roodox-server-key.pem` | 服务端私钥路径 | 高 |
| `database` | object | see below | DB checkpoint/backup 策略 | 高 |
| `control_plane` | object | see below | 设备控制面、Join Bundle、默认策略 | 高 |
| `cleanup` | object | see below | 临时工件、构建目录、冲突文件、日志清理 | 中 |

## `runtime` / 运行时配置

| 字段 / Field | 默认 / Default | 说明 / Notes |
| --- | --- | --- |
| `binary_path` | `roodox_server.exe` | 服务端二进制位置 |
| `state_dir` | `runtime` | 运行时目录，常含 PID、升级快照 |
| `pid_file` | `runtime/roodox_server.pid` | 进程 PID 文件 |
| `log_dir` | `runtime/logs` | stdout/stderr 和其他日志目录 |
| `stdout_log_name` | `server.stdout.log` | 标准输出日志文件名 |
| `stderr_log_name` | `server.stderr.log` | 标准错误日志文件名 |
| `graceful_stop_timeout_seconds` | `10` | 优雅关停等待时间 |

### `runtime.windows_service`

| 字段 / Field | 默认 / Default | 说明 / Notes |
| --- | --- | --- |
| `name` | `RoodoxServer` | SCM 服务名 |
| `display_name` | `Roodox Server` | SCM 显示名 |
| `description` | `Roodox gRPC server` | SCM 描述 |
| `start_type` | `auto` | `auto`, `manual`, `disabled` |

## `database` / 数据库维护

| 字段 / Field | 默认 / Default | 说明 / Notes |
| --- | --- | --- |
| `checkpoint_interval_seconds` | `300` | 自动 checkpoint 间隔 |
| `checkpoint_mode` | `truncate` | 支持 `passive`, `full`, `restart`, `truncate` |
| `backup_dir` | `backups` | 备份输出目录 |
| `backup_interval_seconds` | `86400` | 自动备份间隔 |
| `backup_keep_latest` | `7` | 保留最近几份备份 |

## `control_plane` / 控制面

| 字段 / Field | 默认 / Default | 说明 / Notes | 敏感性 / Sensitivity |
| --- | --- | --- | --- |
| `server_id` | empty, runtime fallback to hostname-derived value | 服务端标识 | 中 |
| `default_device_group` | `default` | 默认设备组 | 中 |
| `heartbeat_interval_seconds` | `15` | 建议客户端心跳间隔 | 中 |
| `default_policy_revision` | `1` | 默认策略版本号 | 中 |
| `available_actions` | `reconnect_overlay,resync,remount,collect_diagnostics` | 可下发客户端动作白名单 | 中 |
| `diagnostics_keep_latest` | `20` | 每设备保留诊断数量 | 高 |
| `assigned_config` | object | 默认客户端策略 | 高 |
| `join_bundle` | object | 客户端交付和连接发现配置 | 高 |

### `control_plane.assigned_config`

| 字段 / Field | 默认 / Default | 说明 / Notes |
| --- | --- | --- |
| `mount_path` | empty | 客户端挂载路径 |
| `sync_roots` | `["."]` | 允许同步的逻辑根 |
| `conflict_policy` | `manual` | 冲突处理策略 |
| `read_only` | `false` | 是否只读 |
| `auto_connect` | `true` | 是否自动连接 |
| `bandwidth_limit` | `0` | 带宽限制，`0` 通常代表不限制 |
| `log_level` | `info` | 客户端日志级别 |
| `large_file_threshold` | `67108864` | 大文件阈值，默认 64 MiB |

### `control_plane.join_bundle`

| 字段 / Field | 默认 / Default | 说明 / Notes | 敏感性 / Sensitivity |
| --- | --- | --- | --- |
| `overlay_provider` | empty in code, example uses `direct` | overlay 标签，例如 `direct`, `tailscale`, `easytier` | 中 |
| `overlay_join_config_json` | empty in code, normalized to trimmed string | 给客户端 bootstrap 的原始 JSON 串 | 高 |
| `service_discovery` | object | 交付给客户端的地址发现参数 | 高 |

### `control_plane.join_bundle.service_discovery`

| 字段 / Field | 默认 / Default | 说明 / Notes |
| --- | --- | --- |
| `mode` | `static` | 目前主要使用静态发现 |
| `host` | empty, runtime may derive from `addr` | 客户端连接主机名或 overlay 地址 |
| `port` | `0` in struct, example uses `50051` | 客户端连接端口 |
| `use_tls` | follows `tls_enabled` when unspecified | 客户端是否按 TLS 连接 |
| `tls_server_name` | empty | 客户端证书校验用的 SNI / SAN 名称 |

## `cleanup` / 清理策略

### `cleanup.temp_artifacts`

| 字段 / Field | 默认 / Default | 说明 / Notes |
| --- | --- | --- |
| `enabled` | `true` | 是否清理临时工件 |
| `interval_seconds` | `300` | 运行频率 |
| `retention_seconds` | `86400` | 保留时长 |
| `max_bytes` | `536870912` | 总大小上限，512 MiB |
| `prefixes` | `roodox-suite-`, `roodox-suite-build-`, `roodox-stress-` | 识别临时工件的前缀 |

### `cleanup.build_workdirs`

| 字段 / Field | 默认 / Default | 说明 / Notes |
| --- | --- | --- |
| `interval_seconds` | `60` | 构建工作目录清理周期 |
| `retention_seconds` | `1800` | 保留时长 |
| `max_bytes` | `2147483648` | 总大小上限，2 GiB |

### `cleanup.conflict_files`

| 字段 / Field | 默认 / Default | 说明 / Notes |
| --- | --- | --- |
| `enabled` | `true` | 是否清理冲突副本 |
| `interval_seconds` | `3600` | 运行频率 |
| `retention_seconds` | `604800` | 保留时长，默认 7 天 |
| `max_bytes` | `268435456` | 总大小上限，256 MiB |
| `max_copies_per_path` | `20` | 单路径最大冲突副本数 |

### `cleanup.log_files`

| 字段 / Field | 默认 / Default | 说明 / Notes |
| --- | --- | --- |
| `enabled` | `true` | 是否清理日志 |
| `dir` | runtime log dir | 日志目录 |
| `patterns` | `server*.log` | 参与清理的文件模式 |
| `interval_seconds` | `900` | 运行频率 |
| `retention_seconds` | `604800` | 保留时长 |
| `max_bytes` | `268435456` | 总大小上限 |

## Environment Variable Overrides / 环境变量覆盖

### Core and paths / 核心与路径

| 环境变量 / Env | 覆盖字段 / Field |
| --- | --- |
| `ROODOX_ADDR` | `addr` |
| `ROODOX_DATA_ROOT` | `data_root` |
| `ROODOX_ROOT_DIR` | `root_dir` |
| `ROODOX_DB_PATH` | `db_path` |
| `ROODOX_RUNTIME_BINARY_PATH` | `runtime.binary_path` |
| `ROODOX_RUNTIME_STATE_DIR` | `runtime.state_dir` |
| `ROODOX_RUNTIME_PID_FILE` | `runtime.pid_file` |
| `ROODOX_RUNTIME_LOG_DIR` | `runtime.log_dir` |
| `ROODOX_RUNTIME_STDOUT_LOG_NAME` | `runtime.stdout_log_name` |
| `ROODOX_RUNTIME_STDERR_LOG_NAME` | `runtime.stderr_log_name` |
| `ROODOX_RUNTIME_GRACEFUL_STOP_TIMEOUT_SECONDS` | `runtime.graceful_stop_timeout_seconds` |

### Windows Service / Windows 服务

| 环境变量 / Env | 覆盖字段 / Field |
| --- | --- |
| `ROODOX_WINDOWS_SERVICE_NAME` | `runtime.windows_service.name` |
| `ROODOX_WINDOWS_SERVICE_DISPLAY_NAME` | `runtime.windows_service.display_name` |
| `ROODOX_WINDOWS_SERVICE_DESCRIPTION` | `runtime.windows_service.description` |
| `ROODOX_WINDOWS_SERVICE_START_TYPE` | `runtime.windows_service.start_type` |

### Build and security / 构建与安全

| 环境变量 / Env | 覆盖字段 / Field | 说明 / Notes |
| --- | --- | --- |
| `ROODOX_REMOTE_BUILD_ENABLED` | `remote_build_enabled` | bool |
| `ROODOX_BUILD_TOOL_DIRS` | `build_tool_dirs` | 用 OS path separator 分隔，Windows 下一般是 `;` |
| `ROODOX_BUILD_REQUIRED_TOOLS` | `required_build_tools` | 逗号分隔 |
| `ROODOX_AUTH_ENABLED` | `auth_enabled` | bool |
| `ROODOX_SHARED_SECRET` | `shared_secret` | 高敏感 |
| `ROODOX_TLS_ENABLED` | `tls_enabled` | bool |
| `ROODOX_TLS_CERT_PATH` | `tls_cert_path` | 路径 |
| `ROODOX_TLS_KEY_PATH` | `tls_key_path` | 路径，高敏感 |

### Database / 数据库

| 环境变量 / Env | 覆盖字段 / Field |
| --- | --- |
| `ROODOX_DB_CHECKPOINT_INTERVAL_SECONDS` | `database.checkpoint_interval_seconds` |
| `ROODOX_DB_CHECKPOINT_MODE` | `database.checkpoint_mode` |
| `ROODOX_DB_BACKUP_DIR` | `database.backup_dir` |
| `ROODOX_DB_BACKUP_INTERVAL_SECONDS` | `database.backup_interval_seconds` |
| `ROODOX_DB_BACKUP_KEEP_LATEST` | `database.backup_keep_latest` |

### Control plane / 控制面

| 环境变量 / Env | 覆盖字段 / Field |
| --- | --- |
| `ROODOX_SERVER_ID` | `control_plane.server_id` |
| `ROODOX_DEVICE_GROUP` | `control_plane.default_device_group` |
| `ROODOX_HEARTBEAT_INTERVAL_SECONDS` | `control_plane.heartbeat_interval_seconds` |
| `ROODOX_POLICY_REVISION` | `control_plane.default_policy_revision` |
| `ROODOX_AVAILABLE_ACTIONS` | `control_plane.available_actions` |
| `ROODOX_DIAGNOSTICS_KEEP_LATEST` | `control_plane.diagnostics_keep_latest` |

### Assigned client config / 默认客户端配置

| 环境变量 / Env | 覆盖字段 / Field |
| --- | --- |
| `ROODOX_CLIENT_MOUNT_PATH` | `control_plane.assigned_config.mount_path` |
| `ROODOX_SYNC_ROOTS` | `control_plane.assigned_config.sync_roots` |
| `ROODOX_CONFLICT_POLICY` | `control_plane.assigned_config.conflict_policy` |
| `ROODOX_READ_ONLY` | `control_plane.assigned_config.read_only` |
| `ROODOX_AUTO_CONNECT` | `control_plane.assigned_config.auto_connect` |
| `ROODOX_BANDWIDTH_LIMIT` | `control_plane.assigned_config.bandwidth_limit` |
| `ROODOX_LOG_LEVEL` | `control_plane.assigned_config.log_level` |
| `ROODOX_LARGE_FILE_THRESHOLD` | `control_plane.assigned_config.large_file_threshold` |

### Join bundle / 客户端交付

| 环境变量 / Env | 覆盖字段 / Field |
| --- | --- |
| `ROODOX_BUNDLE_OVERLAY_PROVIDER` | `control_plane.join_bundle.overlay_provider` |
| `ROODOX_BUNDLE_OVERLAY_JOIN_CONFIG_JSON` | `control_plane.join_bundle.overlay_join_config_json` |
| `ROODOX_BUNDLE_SERVICE_DISCOVERY_MODE` | `control_plane.join_bundle.service_discovery.mode` |
| `ROODOX_BUNDLE_SERVICE_HOST` | `control_plane.join_bundle.service_discovery.host` |
| `ROODOX_BUNDLE_SERVICE_PORT` | `control_plane.join_bundle.service_discovery.port` |
| `ROODOX_BUNDLE_USE_TLS` | `control_plane.join_bundle.service_discovery.use_tls` |
| `ROODOX_BUNDLE_TLS_SERVER_NAME` | `control_plane.join_bundle.service_discovery.tls_server_name` |

### Cleanup / 清理策略

| 环境变量 / Env | 覆盖字段 / Field |
| --- | --- |
| `ROODOX_ARTIFACT_CLEANUP_ENABLED` | `cleanup.temp_artifacts.enabled` |
| `ROODOX_ARTIFACT_CLEANUP_INTERVAL_SECONDS` | `cleanup.temp_artifacts.interval_seconds` |
| `ROODOX_ARTIFACT_RETENTION_SECONDS` | `cleanup.temp_artifacts.retention_seconds` |
| `ROODOX_ARTIFACT_MAX_BYTES` | `cleanup.temp_artifacts.max_bytes` |
| `ROODOX_ARTIFACT_PREFIXES` | `cleanup.temp_artifacts.prefixes` |
| `ROODOX_BUILD_CLEANUP_INTERVAL_SECONDS` | `cleanup.build_workdirs.interval_seconds` |
| `ROODOX_BUILD_RETENTION_SECONDS` | `cleanup.build_workdirs.retention_seconds` |
| `ROODOX_BUILD_MAX_BYTES` | `cleanup.build_workdirs.max_bytes` |
| `ROODOX_CONFLICT_CLEANUP_ENABLED` | `cleanup.conflict_files.enabled` |
| `ROODOX_CONFLICT_CLEANUP_INTERVAL_SECONDS` | `cleanup.conflict_files.interval_seconds` |
| `ROODOX_CONFLICT_RETENTION_SECONDS` | `cleanup.conflict_files.retention_seconds` |
| `ROODOX_CONFLICT_MAX_BYTES` | `cleanup.conflict_files.max_bytes` |
| `ROODOX_CONFLICT_MAX_COPIES_PER_PATH` | `cleanup.conflict_files.max_copies_per_path` |
| `ROODOX_LOG_CLEANUP_ENABLED` | `cleanup.log_files.enabled` |
| `ROODOX_LOG_CLEANUP_DIR` | `cleanup.log_files.dir` |
| `ROODOX_LOG_CLEANUP_PATTERNS` | `cleanup.log_files.patterns` |
| `ROODOX_LOG_CLEANUP_INTERVAL_SECONDS` | `cleanup.log_files.interval_seconds` |
| `ROODOX_LOG_RETENTION_SECONDS` | `cleanup.log_files.retention_seconds` |
| `ROODOX_LOG_MAX_BYTES` | `cleanup.log_files.max_bytes` |

## Minimal Secure Example / 最小安全示例

```json
{
  "addr": ":50051",
  "data_root": "data",
  "root_dir": "share",
  "auth_enabled": true,
  "shared_secret": "replace-with-a-long-random-secret",
  "tls_enabled": true,
  "tls_cert_path": "certs/roodox-server-cert.pem",
  "tls_key_path": "certs/roodox-server-key.pem",
  "control_plane": {
    "server_id": "srv-main",
    "default_device_group": "default",
    "join_bundle": {
      "overlay_provider": "direct",
      "overlay_join_config_json": "{}",
      "service_discovery": {
        "mode": "static",
        "host": "roodox.example.com",
        "port": 50051,
        "use_tls": true,
        "tls_server_name": "roodox.example.com"
      }
    }
  }
}
```

## Maintainer Advice / 维护建议

- 如果你是改部署位置，优先改 `data_root`，而不是手工分别改 6 个路径。  
  Prefer changing `data_root` instead of editing many storage paths one by one.
- 如果你是改客户端交付方式，别只改 `join_bundle`，还要联动 Workbench 导出逻辑和文档。  
  Join-bundle changes usually require Workbench and docs changes too.
- 如果你要上真实环境，示例文件里的占位 secret 一定要替换。  
  Replace placeholder secrets before real deployment.

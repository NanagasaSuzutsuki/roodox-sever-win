# Runtime Storage And Sensitive Files / 运行时存储与敏感文件

## Why This Matters / 为什么这一章重要

Roodox 的敏感面不只在配置文件。  
The sensitive surface is not limited to the config file.

真正需要重点看守的是：

- `root_dir` 里的真实共享内容
- SQLite 主库、WAL、SHM、备份
- TLS 私钥与 CA 私钥
- Join Bundle 与客户端交付目录
- 运行日志和升级快照

## Runtime Path Map / 运行时路径地图

| 逻辑项 / Logical item | 默认路径 / Default path | 常见派生路径 / Derived path | 敏感性 / Sensitivity | 说明 / Notes |
| --- | --- | --- | --- | --- |
| 配置文件 | `roodox.config.json` | operator-chosen | 高 | 含 secret、路径、部署参数 |
| 数据根目录 | empty | custom `data_root` | 高 | 多个高敏感文件的统一父目录 |
| 文件共享根 | `root_dir` | custom | 高 | 用户文件本体 |
| SQLite 主库 | `roodox.db` | `data_root/roodox.db` | 高 | 含元数据、历史内容、控制面数据 |
| SQLite WAL | `roodox.db-wal` | same dir as DB | 高 | 可能残留最近写入页面 |
| SQLite SHM | `roodox.db-shm` | same dir as DB | 中 | 协调共享内存文件 |
| 备份目录 | `backups/` | `data_root/backups/` | 高 | DB 备份集合 |
| 运行时目录 | `runtime/` | `data_root/runtime/` | 高 | PID、日志、升级快照 |
| PID 文件 | `runtime/roodox_server.pid` | derived | 低 | 进程 ID |
| 日志目录 | `runtime/logs/` | derived | 中 | stdout/stderr 及其他日志 |
| 服务端证书 | `certs/roodox-server-cert.pem` | `data_root/certs/...` | 中 | 公钥证书 |
| 服务端私钥 | `certs/roodox-server-key.pem` | `data_root/certs/...` | 高 | 绝不可公开 |
| CA 根证书 | `certs/roodox-ca-cert.pem` | same dir | 中 | 供客户端信任 |
| CA 根私钥 | `certs/roodox-ca-key.pem` | same dir | 高 | 绝不可公开 |
| 升级快照目录 | `runtime/releases/` | derived | 高 | 可能含旧二进制和部署快照 |
| 客户端 CA 导出 | `artifacts/handoff/roodox-ca-cert.pem` | Workbench/CLI output | 中 | 对客户端可分享 |
| 客户端接入导出目录 | `artifacts/handoff/client-access/` | Workbench output | 高 | 常含 Join Bundle 和可选 CA |
| Workbench bootstrap | `roodox-workbench.bootstrap.json` | project root or nearby | 中 | 可暴露项目根和配置路径 |

## SQLite Schema / SQLite 表结构与敏感性

当前 schema version: `2`

### V1 core file tables / 文件版本表

| 表 / Table | 主要内容 / Contents | 敏感性 / Sensitivity |
| --- | --- | --- |
| `meta` | 当前文件元数据：路径、大小、mtime、hash、version | 高 |
| `version` | 历史版本索引：路径、版本、hash、大小、client_id、change_type | 高 |
| `version_blob` | 历史版本实际字节内容 | 高 |
| `file_head` | 文件当前版本号 | 中 |

### V2 control plane tables / 控制面表

| 表 / Table | 主要内容 / Contents | 敏感性 / Sensitivity |
| --- | --- | --- |
| `device_registry` | 设备身份、平台、overlay、挂载/同步状态、错误摘要 | 高 |
| `device_policy` | 每设备策略覆盖 | 高 |
| `device_actions` | 管理员下发动作队列 | 中 |
| `device_diagnostics` | 诊断摘要与 `payload` BLOB | 高 |

## Important Consequence / 一个关键后果

因为 `version_blob` 会持久化历史内容，所以：

- `roodox.db` 不是单纯的“控制数据库”
- `roodox.db-wal` 也可能带有高敏感页面
- 数据库备份相当于把“文件历史 + 设备控制面状态”一起打包

这也是为什么数据库和备份必须按高敏感数据处理。  
This is why database files and backups must be treated as high-sensitivity artifacts.

## Handoff Artifacts / 客户端交付物

### Join Bundle JSON

Workbench 和管理面最终交付给客户端的 JSON 文件通常是：

- `roodox-client-access.json`

其内容来自 `internal/accessbundle/bundle.go`，字段是客户端导向的 camelCase 结构，而不是 proto 里的 snake_case 风格：

- `version`
- `overlayProvider`
- `overlayJoinConfig`
- `serviceDiscovery`
- `roodox`

`roodox.sharedSecret` 一旦存在，整个文件就应按高敏感处理。  
If `roodox.sharedSecret` is present, the entire file becomes high-sensitivity.

### Exported CA

如果 `useTLS=true`，Workbbench 导出客户端接入目录时还会附带：

- `roodox-ca-cert.pem`

这个文件是“可给客户端”的，但不应与 `roodox-ca-key.pem` 混淆。  
This file is shareable with clients; the CA private key is not.

## Logs / 日志

默认日志文件包括：

- `server.stdout.log`
- `server.stderr.log`

日志常见可能暴露：

- 共享路径
- 设备 ID / 设备组
- 证书状态
- 错误字符串
- 升级/回滚时间点

如果要拿日志做公开 issue 或截图，先做路径与设备名脱敏。  
Redact paths and device names before posting logs publicly.

## Release Snapshots / 升级快照

`scripts/server/upgrade-deployment.ps1` 会在 `runtime/releases/` 下留下升级前快照，用于回滚。  
Upgrade snapshots are stored under `runtime/releases/` for rollback.

这类快照要视作高敏感，原因不是它们一定含 secret，而是它们可能包含：

- 旧版可执行文件
- 与部署相关的运行布局
- 部分可回滚状态

## Cleanup Owners / 谁负责清理什么

| 清理目标 / Target | 负责模块 / Owner |
| --- | --- |
| 临时工件 | `cleanup.temp_artifacts` / `internal/serverapp/artifact_janitor.go` |
| 构建工作目录 | `cleanup.build_workdirs` / `internal/serverapp/file_janitors.go` |
| 冲突文件 | `cleanup.conflict_files` / `internal/serverapp/file_janitors.go` |
| 日志文件 | `cleanup.log_files` / `internal/serverapp/file_janitors.go` |
| 设备诊断保留数量 | `internal/db/device_registry_admin.go` |
| 数据库备份数量 | `database.backup_keep_latest` / `internal/serverapp/database_maintenance.go` |

## Safe Sharing Matrix / 文件分享规则

| 文件或目录 / Item | 可发客户端 / Share with client | 可公开 / Publish | 备注 / Notes |
| --- | --- | --- | --- |
| `roodox-ca-cert.pem` | 可以 | 示例可以，真实导出不建议长期公开 | trust input |
| `roodox-server-cert.pem` | 一般不需要 | 不建议 | 服务端材料 |
| `roodox-server-key.pem` | 不可以 | 不可以 | 私钥 |
| `roodox-ca-key.pem` | 不可以 | 不可以 | 根签发私钥 |
| `roodox-client-access.json` | 仅目标客户端 | 不可以 | 可能含 secret |
| `roodox.db` / backups | 不可以 | 不可以 | 含内容与控制面数据 |
| `runtime/logs/` | 谨慎 | 不建议 | 易带路径与设备信息 |
| `root_dir/` | 按业务 | 不可以默认公开 | 用户数据本体 |

## Maintainer Notes / 维护备注

- 真要做更强隐私隔离，优先考虑把 `data_root` 放到受控目录，并对其做备份与权限管理。  
  Put `data_root` under a controlled path with explicit backup and permission policy.
- 任何“把数据库拿出来做分析”的动作，都要先意识到那不只是 metadata。  
  Treat any database export as content-bearing.

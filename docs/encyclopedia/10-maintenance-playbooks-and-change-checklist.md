# Maintenance Playbooks And Change Checklist / 维护手册与改动清单

## Daily Rule / 日常规则

每次动项目前先问自己三件事：

1. 这次改动会不会碰到敏感数据？
2. 这次改动会不会影响客户端连接契约？
3. 这次改动除了代码，还要不要同步脚本、GUI、示例配置和文档？

## Playbook 1: Change deployment paths / 改部署路径

目标：换盘、换目录、统一运行时布局。

建议顺序：

1. 优先改 `data_root`
2. 再检查 `root_dir`
3. 仅在必要时单独覆盖 `db_path`、`runtime.state_dir`、`tls_*_path`
4. 用 `status-server.ps1` 或 GUI 确认新路径生效
5. 确认旧目录未被残留进程继续占用

## Playbook 2: Turn on secure baseline / 打开安全基线

目标：把项目从“能跑”提升到“不是裸奔”。

步骤：

1. `tls_enabled = true`
2. `auth_enabled = true`
3. 设置高熵 `shared_secret`
4. 配好 `service_discovery.use_tls = true`
5. 配好 `tls_server_name`
6. 导出客户端 CA
7. 用最小客户端或 QA 验证连接

不要做：

- `auth_enabled=true` 但 `tls_enabled=false`
- 直接把真实 secret 写进公开文档或 issue

## Playbook 3: Rotate certificates / 轮换证书

目标：更新服务端证书，必要时更新根 CA。

步骤：

1. 跑 `certificate-status.ps1`
2. 跑 `rotate-certificates.ps1`
3. 如果用了 `-RotateRootCA`，准备重新向所有客户端交付新 CA
4. 视情况重启服务
5. 用客户端样例或 QA 确认 TLS 可连

## Playbook 4: Backup and restore DB / 备份与恢复数据库

备份：

- 管理面：`TriggerServerBackup`
- CLI：`-trigger-server-backup-json`

恢复：

1. 停止服务
2. 确认恢复源文件
3. 运行 `restore-database.ps1`
4. 除非特别确定，否则不要加 `-NoSafetyBackup`
5. 恢复后做 `go test ./...` 不够，还应实际拉起服务验证

## Playbook 5: Add or change an RPC / 新增或修改 RPC

建议顺序：

1. 改 `proto/roodox_core.proto`
2. 改 `internal/server` 对应服务实现
3. 如涉及持久化，改 `internal/db`
4. 改 `client/roodox_client.go`
5. 如需要本地管理或 GUI，改 `cmd/roodox_server/main.go` 和 `workbench/src-tauri/src/main.rs`
6. 改文档
7. 跑 `go test ./...`

## Playbook 6: Add a GUI feature / 加一个 GUI 功能

通常需要同时看：

- `workbench/src/App.tsx`
- `workbench/src-tauri/src/main.rs`
- 必要时 `cmd/roodox_server/main.go`
- 必要时后端 gRPC 或 DB 代码

不要只改前端状态管理。  
Do not treat the GUI as a purely cosmetic layer.

## Playbook 7: Public release hygiene / 公开发布前卫生检查

每次发公开仓库、安装包、截图或文档前，至少检查：

1. `runtime/`, `backups/`, `certs/`, `artifacts/handoff/` 没有被打包或提交
2. 示例配置仍然是占位值
3. README 和 docs 没有真实地址、真实 secret、真实设备名
4. 如导出过 Join Bundle，没有误留在仓库或截图目录
5. `docs/PRIVACY_AUDIT.md` 的检查口径仍然成立

## Verification Ladder / 验证梯度

从低到高建议至少做：

1. `go test ./...`
2. `scripts/qa/run-live-regression.ps1`
3. 如改了运维或交付路径，再跑 `scripts/server/validate-deployment-lifecycle.ps1`
4. 如改了稳定性或构建面，再跑 `scripts/qa/run-full-qa.ps1`

## Change Review Checklist / 改动复查清单

### 配置层

- `roodox.config.example.json` 是否仍与代码一致
- Workbench 是否还能读写新字段
- 环境变量覆盖是否需要新增

### 安全面

- 是否意外把敏感值写进日志
- 是否引入了可以绕过 TLS 的新交付方式
- 是否需要更新客户端 CA 或 Join Bundle 格式

### 数据层

- 是否改变了 SQLite schema
- 是否需要迁移逻辑
- 是否会增加数据库敏感内容驻留

### 运维层

- 脚本是否需要同步更新
- Windows Service 路径或名称是否受影响
- 升级/回滚流程是否仍成立

## If You Need To Simplify Further / 如果还要继续精简项目

下一轮最安全的精简方向通常是：

1. 继续把“说明性文档”都收进 `docs/`
2. 保持根目录只留入口文件
3. 避免把运行时产物、交付物、私人操作记录混进源码树

不要优先删测试、删脚本、删 proto。  
Do not start by deleting tests, scripts, or proto definitions.

## Maintainer Notes / 维护备注

- 精简结构最怕“看起来干净了，但现场运维入口没了”。  
  Simplification should not remove operational entrypoints.
- 先收口文档和目录布局，再动核心接口，是更稳的节奏。  
  Clean up documentation and layout before touching core interfaces when possible.

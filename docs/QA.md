# Roodox QA Scripts / Roodox QA 脚本

这个仓库包含可复用的本地 QA 入口，用于活体验证、长稳压测、故障注入和重启恢复验证。  
This repository includes reusable local QA entrypoints for live regression, soak testing, fault injection, and restart recovery.

它们的目的，是把一次性命令行片段收敛成稳定、可重复的验证入口。  
They are intended to replace one-off terminal snippets with stable and repeatable verification commands.

## Go QA Tool / Go QA 工具

核心命令：  
Core entrypoints:

- `go run ./cmd/roodox_qa live`
- `go run ./cmd/roodox_qa soak`
- `go run ./cmd/roodox_qa faults`
- `go run ./cmd/roodox_qa probe`

常用覆盖参数：  
Common overrides:

- `-config`
- `-addr`
- `-root-dir`
- `-shared-secret`
- `-tls-root-cert`
- `-tls-server-name`
- `-server-id`

默认情况下，这个工具会读取 `roodox.config.json`，自动推导拨号地址和 TLS 根证书路径，并在 `root_dir/qa/...` 下生成临时 QA 工件。  
By default, the tool loads `roodox.config.json`, derives the dial address and TLS root CA path from local server configuration, and creates temporary QA artifacts under `root_dir/qa/...`.

如果没有显式指定 `-keep-artifacts`，这些临时 QA 工件会自动清理。  
Unless `-keep-artifacts` is specified, those temporary QA artifacts are cleaned up automatically.

When `remote_build_enabled` is `false` in the selected config, `live` and `faults` skip build-specific assertions, and `soak` automatically forces `-build-interval 0`.

## PowerShell Wrappers / PowerShell 包装脚本

Windows 友好的包装脚本位于 [`../scripts/qa`](../scripts/qa)：  
Windows-friendly wrappers live under [`../scripts/qa`](../scripts/qa):

- [`run-live-regression.ps1`](../scripts/qa/run-live-regression.ps1)
- [`run-fault-injection.ps1`](../scripts/qa/run-fault-injection.ps1)
- [`run-soak.ps1`](../scripts/qa/run-soak.ps1)
- [`run-restart-recovery.ps1`](../scripts/qa/run-restart-recovery.ps1)
- [`run-full-qa.ps1`](../scripts/qa/run-full-qa.ps1)

示例：  
Examples:

```powershell
.\scripts\qa\run-live-regression.ps1
.\scripts\qa\run-fault-injection.ps1
.\scripts\qa\run-soak.ps1 -Duration 5m -Workers 6 -BuildInterval 30s
.\scripts\qa\run-restart-recovery.ps1 -PreSeconds 5 -DownSeconds 7 -PostSeconds 14
.\scripts\qa\run-restart-recovery.ps1 -KeepLogs -CaptureRestartServerLogs
.\scripts\qa\run-full-qa.ps1 -SoakDuration 3m
```

`run-restart-recovery.ps1` 默认在成功时删除 probe 日志，并避免让长期运行的服务进程依赖 `%TEMP%\roodox-qa` 文件。  
`run-restart-recovery.ps1` now deletes probe logs on success by default and avoids binding the long-running server process to `%TEMP%\roodox-qa` files.

只有在需要保留探针输出或抓启动重定向日志做调试时，才建议使用 `-KeepLogs` 或 `-CaptureRestartServerLogs`。  
Use `-KeepLogs` or `-CaptureRestartServerLogs` only when probe output or redirected startup logs are explicitly needed for debugging.

## Deployment Lifecycle Smoke / 部署生命周期冒烟验证

打包与证书生命周期的冒烟验证脚本位于 [`../scripts/server/validate-deployment-lifecycle.ps1`](../scripts/server/validate-deployment-lifecycle.ps1)。  
Reusable packaging and certificate lifecycle smoke validation lives under [`../scripts/server/validate-deployment-lifecycle.ps1`](../scripts/server/validate-deployment-lifecycle.ps1).

默认夹具配置：  
Default fixture config:

- [`../testdata/deployment-smoke/roodox-smoke.config.json`](../testdata/deployment-smoke/roodox-smoke.config.json)

示例：  
Example:

```powershell
.\scripts\server\validate-deployment-lifecycle.ps1 -Rebuild
```

这套验证会在隔离的部署目录中检查：  
This validation checks, in an isolated deployment directory:

- 安装快照是否创建成功  
  Install snapshot creation
- 客户端 CA 导出是否正常  
  TLS root export for client handoff
- 升级时只轮换叶子证书是否正常  
  Leaf-only certificate rotation during upgrade
- 回滚是否能恢复升级前快照  
  Rollback restoring the pre-upgrade deployment snapshot

## Coverage Summary / 覆盖范围概览

`live` 覆盖：  
`live` covers:

- TLS 与共享密钥连接  
  TLS and shared-secret connection
- gRPC health  
  gRPC health
- 运行态和观测类管理接口  
  Runtime and observability admin APIs
- 设备注册和配置拉取  
  Device registration and config pull
- 心跳与同步状态上报  
  Heartbeat and sync-state reporting
- 文件写、区间写、读、锁、历史、版本查询  
  File write, range write, read, lock, history, version lookup
- 远程构建  
  Remote build
- 设备列表查询  
  Device list query
- 手动备份触发  
  Manual backup trigger

`soak` 覆盖：  
`soak` covers:

- 混合并发文件 IO  
  Mixed concurrent file IO
- 重复历史与锁调用  
  Repeated history and lock calls
- 周期性心跳与同步状态上报  
  Periodic heartbeat and sync-state reporting
- 管理面运行态与观测轮询  
  Admin runtime and observability polling
- 可选备份触发  
  Optional backup trigger
- 周期性远程构建  
  Periodic remote builds
  When remote build is disabled by config, build pressure is skipped instead of failing the suite.

`faults` 覆盖：  
`faults` covers:

- 错误共享密钥  
  Wrong shared secret
- 错误 TLS server name  
  Wrong TLS server name
- 非法 TLS 根证书输入  
  Invalid TLS root certificate input
- `WriteFile`、`WriteFileRange`、`SetFileSize` 的旧版本冲突  
  Stale version conflicts for `WriteFile`, `WriteFileRange`, `SetFileSize`
- 缺失构建单元的失败路径  
  Missing build unit failure path
  This case is skipped when remote build is disabled by config.
- 未知设备控制面错误  
  Unknown device control-plane errors

`probe` 用于重启恢复验证：  
`probe` is used by restart recovery to verify:

- 重启前服务健康  
  Healthy service before restart
- 故障窗口中的真实连接失败  
  Actual connection failures during outage
- 重启后的健康恢复  
  Healthy recovery after restart

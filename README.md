# Roodox

Roodox 是一个以 Windows 为优先目标的 gRPC 文件服务，包含设备控制面、TLS/认证交付、远程构建编排，以及基于 Tauri 的运维工作台。  
Roodox is a Windows-first gRPC file service with device control-plane, TLS/auth handoff, build orchestration, and a Tauri-based operator workbench.

## Latest Release / 最新发布

当前公开发布版本：`v0.1.7`  
Current public release: `v0.1.7`

- Release 页面 / release page: <https://github.com/NanagasaSuzutsuki/roodox-sever-win/releases/tag/v0.1.7>
- `roodox-server-win-v0.1.7-portable.zip`
  - 便携包，包含服务端、Workbench、客户端导入器、脚本、文档和示例配置  
    Portable bundle with server, workbench, client importer, scripts, docs, and sample config
- `roodox-server-win-v0.1.7-setup.exe`
  - 一体化 Windows 安装包，适合直接交付给最终用户  
    All-in-one Windows installer for direct end-user deployment

## Repository Contents / 仓库内容

- Go 服务端，提供文件、同步、锁、版本、构建和管理接口  
  Go server with file, sync, lock, version, build, and admin APIs
- 本地工具和测试使用的客户端连接库  
  Client connection library used by local tools and tests
- PowerShell 部署、升级、回滚和运维脚本  
  PowerShell deployment, upgrade, rollback, and operations scripts
- Tauri + React 运维工作台  
  Tauri + React operator workbench
- 用于客户端交付的 Join Bundle 格式  
  Join-bundle format for client-facing access handoff

## Key Capabilities / 功能概览

- 基于 gRPC 的文件与目录操作  
  File and directory operations over gRPC
- 带版本控制的整文件写、区间写和截断  
  Version-aware whole-file writes, range writes, and truncate support
- 设备注册、心跳、挂载/同步状态上报、诊断上传  
  Device registration, heartbeat, mount/sync reporting, and diagnostics upload
- 基于 SQLite 的运行态、历史、锁和观测数据  
  SQLite-backed runtime state, history, lock, and observability data
- TLS 证书检查、轮换、客户端 CA 导出  
  TLS certificate inspection, rotation, and client CA export
- Windows 进程和服务生命周期管理  
  Windows process and service lifecycle management
- GUI 侧运维与客户端接入包导出  
  GUI-based operations and client access export
- 连接码导出与客户端导入器  
  Self-contained connection-code export and client importer

## Repository Layout / 目录结构

- `cmd/roodox_server`: 服务端入口 / server binary entrypoint
- `cmd/roodox_client_import`: 客户端连接码导入器 / client connection-code importer
- `cmd/roodox_qa`: QA 与回归工具 / QA and regression tool
- `client/`: Go 客户端辅助库 / Go client helpers
- `internal/`: 服务端、运行时、数据库、清理、控制面逻辑 / server, runtime, DB, cleanup, and control-plane packages
- `proto/`: protobuf 与 gRPC 定义 / protobuf and gRPC definitions
- `scripts/server/`: 服务生命周期、TLS、备份、升级、回滚 / service lifecycle, TLS, backup, upgrade, rollback
- `scripts/qa/`: QA 包装脚本 / reusable QA wrappers
- `scripts/workbench/`: GUI 启动与打包脚本 / GUI launch and packaging scripts
- `workbench/`: Tauri + React 工作台 / Tauri + React workbench

## Documentation Map / 文档地图

为了让根目录保持精简，补充文档已收拢到 `docs/`：  
To keep the repository root lean, supporting docs now live under `docs/`:

- [`docs/README.md`](docs/README.md)
- [`docs/USER_INSTALL.md`](docs/USER_INSTALL.md)
- [`docs/encyclopedia/README.md`](docs/encyclopedia/README.md)
- [`docs/OPERATIONS.md`](docs/OPERATIONS.md)
- [`docs/QA.md`](docs/QA.md)
- [`SECURITY.md`](SECURITY.md)

## Security Model / 安全模型

Roodox 可以按以下方式运行：  
Roodox can run with the following security baseline:

- 启用 TLS  
  TLS enabled
- 启用共享密钥认证  
  Shared-secret authentication enabled
- 以 CA 根证书作为客户端信任输入  
  Client trust distributed as a CA root certificate

面向客户端的最小交付材料通常包括：  
The intended client handoff baseline usually includes:

- `host:port`
- `tls_enabled`
- `tls_server_name`
- 导出的客户端 CA 根证书  
  Exported client CA root
- 认证开启时所需的共享密钥  
  Shared secret when auth is enabled
- 可选的 Join Bundle  
  Optional join bundle with overlay and device bootstrap metadata

## Quick Start / 快速开始

### 1. Prepare Config / 准备配置

先从示例配置复制一份本地配置：  
Create a local config from the example:

```powershell
Copy-Item .\roodox.config.example.json .\roodox.config.json
```

至少修改这些字段：  
At minimum, update these fields:

- `root_dir`
- `shared_secret`
- `tls_enabled`
- `control_plane.server_id`
- `control_plane.join_bundle.service_discovery.host`
- `control_plane.join_bundle.service_discovery.tls_server_name`

如果你是通过 `setup.exe` 安装包部署，安装器会把实际可写配置放到 `C:\ProgramData\Roodox\roodox.config.json`。这时优先修改那份安装后的配置，而不是仓库里的示例文件。  
If you deploy through the `setup.exe` installer, the writable live config is placed at `C:\ProgramData\Roodox\roodox.config.json`. Update that installed config first instead of the in-repo sample file.

### 2. Start the Server / 启动服务

最简单的本地启动方式：  
The simplest local startup path is:

```powershell
.\scripts\server\start-server.ps1 -BuildIfMissing
```

常用生命周期命令：  
Common lifecycle commands:

```powershell
.\scripts\server\status-server.ps1
.\scripts\server\restart-server.ps1 -Rebuild
.\scripts\server\stop-server.ps1
```

如果需要前台运行：  
To run in the foreground instead:

```powershell
.\scripts\server\start-server.ps1 -Foreground -BuildIfMissing
```

### 3. Open the Workbench / 打开工作台

```powershell
.\scripts\workbench\start-gui.cmd
```

工作台覆盖这些功能：  
The workbench covers:

- 运行态健康与最近活跃设备  
  Runtime health and recent devices
- 设备清单和 overlay 标签  
  Device inventory and overlay labels
- 备份、TLS、观测指标  
  Backup, TLS, and observability
- 客户端接入基线、Join Bundle 预览和导出  
  Client access baseline, join-bundle preview, and export
- 本地日志和配置编辑  
  Local logs and config editing

### 4. Release Package Usage / 发布包使用方式

如果你不从源码运行，而是直接使用公开 release：

If you are using the public release instead of running from source:

1. 便携包：解压 `roodox-server-win-v0.1.7-portable.zip`。  
   Portable: extract `roodox-server-win-v0.1.7-portable.zip`.
2. 一体化安装包：运行 `roodox-server-win-v0.1.7-setup.exe`。  
   All-in-one installer: run `roodox-server-win-v0.1.7-setup.exe`.
3. 安装包默认把程序文件放到 `Program Files\Roodox Server`，把可写配置和运行数据放到 `C:\ProgramData\Roodox`。  
   The installer places app files under `Program Files\Roodox Server` and writable config/runtime data under `C:\ProgramData\Roodox`.
4. 对客户端交付时，可直接分发这些文件：  
   For client handoff, you can directly distribute:
   - `roodox-client-access.json`
   - `roodox-ca-cert.pem`
   - `roodox-connection-code.txt`
   - `roodox_client_import.exe`

## Join Bundle and Overlay Strategy / Join Bundle 与 Overlay 策略

Roodox 不直接实现 Tailscale 或 EasyTier。  
Roodox does not implement Tailscale or EasyTier itself.

Roodox 的做法是把 overlay 视为独立网络层，并通过 Join Bundle 向客户端传递这些信息：  
Instead, Roodox treats the overlay as a separate network layer and ships overlay metadata through the join bundle so clients know:

- 预期的 overlay provider 是什么  
  Which overlay provider is expected
- overlay 启动所需的 JSON 引导参数  
  Which overlay bootstrap JSON should be consumed by the client bootstrap layer
- overlay 就绪后应连接的 Roodox 服务地址、TLS 和认证参数  
  Which Roodox service host, port, TLS, and auth values to use after the overlay is up

Join Bundle 负载字段包括：  
The join bundle payload includes:

- `overlay_provider`
- `overlay_join_config_json`
- `service_discovery_mode`
- `service_host`
- `service_port`
- `use_tls`
- `tls_server_name`
- `server_id`
- `device_group`
- `shared_secret`
- 可选设备身份字段  
  Optional device identity fields

服务端示例命令：  
Server-side examples:

- 直接输出 Join Bundle JSON  
  Issue bundle as JSON

```powershell
.\roodox_server.exe -config .\roodox.config.json -issue-join-bundle-json
```

- 导出客户端 CA  
  Export client CA

```powershell
.\scripts\server\export-client-ca.ps1 -DestinationPath .\handoff\roodox-ca-cert.pem
```

### Direct / 无 Overlay

如果客户端直接连接服务端：  
If clients connect directly to the server address:

```json
{
  "control_plane": {
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

### Tailscale Usage / Tailscale 用法

适用于你希望通过私有 tailnet 建立点对点可达性，而不直接暴露 gRPC 端口到公网的场景。  
Recommended when you want private point-to-point reachability without exposing the gRPC endpoint directly to the public internet.

Roodox 中的使用方式：  
How Roodox uses it:

- `overlay_provider` 设为 `tailscale`  
  Set `overlay_provider` to `tailscale`
- 把 Tailscale 的引导参数写入 `overlay_join_config_json`  
  Put Tailscale-specific bootstrap data into `overlay_join_config_json`
- `service_discovery.host` 使用 tailnet 内可达的 Tailscale IP 或 MagicDNS 名称  
  Set `service_discovery.host` to a Tailscale IP or MagicDNS name reachable inside the tailnet
- `tls_server_name` 应与服务端证书 SAN 保持一致，而不是简单等于 overlay IP  
  Keep `tls_server_name` aligned with the server certificate SAN, not just the overlay IP

示例：  
Example:

```json
{
  "control_plane": {
    "join_bundle": {
      "overlay_provider": "tailscale",
      "overlay_join_config_json": "{\"tailnet\":\"example.ts.net\",\"hostname\":\"roodox-client-01\",\"authKey\":\"replace-with-tailscale-auth-key\"}",
      "service_discovery": {
        "mode": "static",
        "host": "server-1.tailnet.ts.net",
        "port": 50051,
        "use_tls": true,
        "tls_server_name": "roodox.example.com"
      }
    }
  }
}
```

实践注意事项：  
Practical notes:

- 如果 `service_discovery.host` 使用 MagicDNS，证书里的 DNS 名称仍然要和 `tls_server_name` 对齐。  
  If you use MagicDNS for `service_discovery.host`, clients still need a certificate whose DNS name matches `tls_server_name`.
- 如果 TLS 终止在 Roodox 服务端自身，客户端应安装导出的 CA 根证书。  
  If you terminate TLS on the Roodox server itself, distribute the exported CA root to clients.
- `overlay_join_config_json` 对 Roodox 而言是透明载荷，由你的客户端 bootstrap 或 launcher 负责消费。  
  The overlay JSON is treated as opaque by Roodox. Your client bootstrap or launcher decides how to consume it.

### EasyTier Usage / EasyTier 用法

适用于你希望客户端自行管理 overlay、并显式控制 peer bootstrap 和网络拓扑的场景。  
Recommended when you want a user-managed overlay with explicit peer bootstrap and custom network topology.

Roodox 中的使用方式：  
How Roodox uses it:

- `overlay_provider` 设为 `easytier`  
  Set `overlay_provider` to `easytier`
- 把 EasyTier 引导参数写入 `overlay_join_config_json`  
  Put EasyTier bootstrap parameters into `overlay_join_config_json`
- `service_discovery.host` 指向 EasyTier 建链后客户端可达的地址  
  Set `service_discovery.host` to the address reachable after EasyTier is connected

示例：  
Example:

```json
{
  "control_plane": {
    "join_bundle": {
      "overlay_provider": "easytier",
      "overlay_join_config_json": "{\"networkName\":\"roodox-prod\",\"peerTargets\":[\"tcp://overlay-gateway.example.com:11010\"],\"token\":\"replace-me\"}",
      "service_discovery": {
        "mode": "static",
        "host": "10.144.0.10",
        "port": 50051,
        "use_tls": true,
        "tls_server_name": "roodox.example.com"
      }
    }
  }
}
```

实践注意事项：  
Practical notes:

- 将 `overlay_join_config_json` 视为客户端 bootstrap 合同，而不是由服务端强校验的数据模型。  
  Treat `overlay_join_config_json` as the client bootstrap contract for EasyTier, not as a server-enforced schema.
- 即使客户端通过 overlay IP 连接，也应让 `tls_server_name` 绑定到真实证书身份。  
  If clients connect through an overlay IP, keep `tls_server_name` pinned to the certificate identity you actually issued.
- 工作台的接入页可以预览并导出这份交付包。  
  The workbench access page can preview and export this bundle for handoff.

## Workbench Access Flow / 工作台接入流程

工作台有单独的接入页，用来维护：  
The workbench has a dedicated access page that lets operators maintain:

- 面向客户端的 host 和 port  
  Client-facing host and port
- TLS 开关和 server name  
  TLS on/off and server name
- 共享密钥展示与导出  
  Shared-secret visibility and export
- overlay provider 和 overlay bootstrap JSON  
  Overlay provider and overlay bootstrap JSON
- 写入导出包中的设备标签  
  Device labels written into the exported bundle

典型操作流程：  
Typical operator flow:

1. 设置客户端可达的 host、port、TLS 和 overlay 字段。  
   Set the client-facing host, port, TLS, and overlay fields.
2. 保存接入配置。  
   Save access settings.
3. 刷新 Join Bundle 预览。  
   Refresh the join-bundle preview.
4. 导出客户端接入包、连接码和 CA 根证书。  
   Export the client access bundle, connection code, and CA root.
5. 如需交给终端用户自助导入，可一并分发 `roodox_client_import.exe`。  
   If you want end users to self-import the handoff, ship `roodox_client_import.exe` together with it.

## TLS and Certificate Operations / TLS 与证书操作

检查证书状态：  
Inspect TLS material:

```powershell
.\scripts\server\certificate-status.ps1
```

轮换服务端叶子证书：  
Rotate the server leaf certificate:

```powershell
.\scripts\server\rotate-certificates.ps1 -RestartAfter
```

同时轮换根 CA 和叶子证书：  
Rotate both root CA and leaf certificate:

```powershell
.\scripts\server\rotate-certificates.ps1 -RotateRootCA -RestartAfter
```

重要规则：  
Important rules:

- 只轮换叶子证书时，客户端信任根通常不变。  
  Leaf-only rotation keeps the same client trust root.
- 轮换根 CA 后，必须重新分发新的客户端 CA。  
  Root CA rotation requires redistributing a new client CA.
- 客户端应信任导出的 CA 根，而不是叶子证书。  
  Clients should trust the exported CA root, not the leaf certificate.

## GUI Build / GUI 构建

开发环境要求：  
Development requirements:

- Go `1.25`
- Node.js with `npm`
- Rust toolchain
- Tauri Windows 构建依赖  
  Tauri Windows build prerequisites for packaged builds

运行前端构建：  
Run the web build:

```powershell
cd .\workbench
npm install
npm run build
```

构建便携版工作台交付包：  
Build the portable workbench delivery package:

```powershell
.\scripts\workbench\build-gui.cmd
```

构建完整发布产物（便携包 + 一体化安装包）：  
Build the full release set (portable bundle + all-in-one installer):

```powershell
.\scripts\release\build-release.ps1 -BuildInstaller
```

一体化安装包默认把程序文件放到 `Program Files\Roodox Server`，把可写配置和运行数据放到 `C:\ProgramData\Roodox`。  
The all-in-one installer places application files under `Program Files\Roodox Server` and writable config/runtime data under `C:\ProgramData\Roodox`.

安装器首次部署会自动：  
The installer automatically:

- 生成 `roodox.config.json`（如不存在）  
  Create `roodox.config.json` if it does not exist
- 把 `runtime.binary_path` 指向安装目录下的 `roodox_server.exe`  
  Point `runtime.binary_path` at the installed `roodox_server.exe`
- 为默认占位 `shared_secret` 生成随机值  
  Replace the placeholder `shared_secret` with a random value
- 写入 `roodox-workbench.bootstrap.json`  
  Write `roodox-workbench.bootstrap.json`

## Testing / 测试

运行完整 Go 测试：  
Run the full Go test suite:

```powershell
go test ./...
```

运行 QA 脚本：  
Run QA wrappers:

```powershell
.\scripts\qa\run-live-regression.ps1
.\scripts\qa\run-full-qa.ps1
```

补充文档：  
Related documents:

- [docs/OPERATIONS.md](docs/OPERATIONS.md)
- [docs/QA.md](docs/QA.md)
- [docs/README.md](docs/README.md)
- [docs/encyclopedia/README.md](docs/encyclopedia/README.md)
- [SECURITY.md](SECURITY.md)

## License / 许可证

本仓库采用 `Apache-2.0` 许可证。  
This repository is released under `Apache-2.0`.

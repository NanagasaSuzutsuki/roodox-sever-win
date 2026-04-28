# Client Integration And Handoff / 客户端对接与交付

## Goal / 目标

这一章回答三个最实际的问题：

1. 客户端到底拿什么才能连上服务？
2. 连接加密和认证怎么保证不是裸奔？
3. Tailscale / EasyTier 在这个项目里到底放在哪一层？

## Minimal Client Delivery Set / 客户端最小交付集合

### 直接交参数的最小集合

- `host:port`
- `tls_enabled`
- `tls_server_name`
- `shared_secret`（仅当 `auth_enabled=true`）
- 客户端要信任的 CA 根证书

### 更推荐的交付方式

更推荐交：

- Join Bundle JSON
- 可选导出的 `roodox-ca-cert.pem`

这样客户端可以一次拿到 overlay 和 Roodox 连接参数。  
This packages overlay metadata and Roodox connection parameters together.

## Security Matrix / 连接安全组合

| TLS | Shared secret | 结果 / Result | 是否推荐 / Recommended |
| --- | --- | --- | --- |
| off | off | 明文、无认证 | 不推荐 |
| off | on | 明文、但带共享密钥认证 | 不推荐 |
| on | off | 加密、但无应用认证 | 一般不推荐 |
| on | on | 加密 + 应用认证 | 推荐 |

## Join Bundle File Format / 客户端交付 JSON 格式

客户端实际拿到的 JSON 来自 `internal/accessbundle/bundle.go`，结构是：

```json
{
  "version": 1,
  "overlayProvider": "direct",
  "overlayJoinConfig": {},
  "serviceDiscovery": {
    "mode": "static",
    "host": "roodox.example.com",
    "port": 50051,
    "useTLS": true,
    "tlsServerName": "roodox.example.com"
  },
  "roodox": {
    "serverID": "srv-main",
    "deviceGroup": "default",
    "sharedSecret": "replace-with-a-long-random-secret",
    "deviceID": "optional-device-id",
    "deviceName": "optional-device-name",
    "deviceRole": "optional-device-role"
  }
}
```

## Three Integration Modes / 三种对接模式

### 1. Direct / 直连

适合：

- 内网部署
- 固定地址
- 不依赖 overlay 网络

要求：

- `overlayProvider = direct`
- `serviceDiscovery.host` 填真实可达地址
- `useTLS` 和 `tlsServerName` 与服务端配置对齐

### 2. Tailscale

适合：

- 不想把 gRPC 端口直接暴露到公网
- 希望靠 tailnet 做私有可达性

做法：

- `overlayProvider = tailscale`
- `overlayJoinConfig` 放 Tailscale bootstrap JSON
- `serviceDiscovery.host` 通常填 Tailscale IP 或 MagicDNS 名
- `tlsServerName` 仍应与证书 SAN 对齐，不要直接等于 overlay IP 除非证书也这样签

### 3. EasyTier

适合：

- 自己控制 overlay 拓扑
- 需要自定义 peer bootstrap 方式

做法：

- `overlayProvider = easytier`
- `overlayJoinConfig` 放 EasyTier 客户端 bootstrap 所需 JSON
- `serviceDiscovery` 在 overlay 建立后再用于连接 Roodox

## CLI Handoff Paths / 命令行交付路径

### 生成 Join Bundle

```powershell
.\roodox_server.exe -config .\roodox.config.json -issue-join-bundle-json
```

可选嵌入设备字段：

- `-join-device-id`
- `-join-device-name`
- `-join-device-role`
- `-join-device-group`

### 导出客户端 CA

```powershell
.\scripts\server\export-client-ca.ps1 -DestinationPath .\handoff\roodox-ca-cert.pem
```

## Workbench Handoff Paths / GUI 交付路径

Workbench 里有两条常用命令：

- `issue_join_bundle`: 生成并预览 Join Bundle
- `export_client_access_bundle`: 落盘导出客户端接入目录

默认导出目录：

- `artifacts/handoff/client-access/`

其中通常包含：

- `roodox-client-access.json`
- `roodox-ca-cert.pem`（仅当 `useTLS=true`）

## Client Implementation Checklist / 客户端实现清单

客户端最少应实现：

1. 读取 Join Bundle 或配置参数
2. 如需要，先建立 overlay 网络
3. 如 `useTLS=true`，加载 CA 根证书
4. 如存在 `sharedSecret`，在每次 RPC 请求上附带 `x-roodox-secret`
5. 连接 `serviceDiscovery.host:serviceDiscovery.port`
6. 在注册控制面时带上设备 ID、平台、overlay 信息

## Test Path / 最小测试路径

本仓库自带的最小接入样例是 `cmd/testclient`。它使用环境变量：

- `ROODOX_SHARED_SECRET`
- `ROODOX_TLS_ENABLED`
- `ROODOX_TLS_ROOT_CERT_PATH`
- `ROODOX_TLS_SERVER_NAME`

然后固定拨号 `127.0.0.1:50051`。  
It is a local connectivity sample, not a general-purpose client.

## Common Integration Mistakes / 常见对接错误

| 错误 / Mistake | 后果 / Consequence |
| --- | --- |
| 只发 `host:port`，不发 CA 根证书 | TLS 连接失败 |
| 开了 `auth_enabled` 却没发 secret | 所有 RPC 会被拒绝 |
| 发了 secret，但没开 TLS | 共享密钥可能在链路上明文暴露 |
| 把 `tlsServerName` 直接写成 overlay IP，而证书 SAN 不是它 | TLS 校验失败 |
| 把 Join Bundle 发进公开 issue、公开仓库或长期可见聊天记录 | 泄露接入信息 |

## Maintainer Notes / 维护备注

- 客户端真正消费的是 `accessbundle` JSON 格式，不是 proto 里的 `JoinBundle` 字段命名风格。  
  The client-facing JSON format is the access-bundle schema, not the proto naming style.
- 如果以后要做官方客户端，优先稳定的不是前端样式，而是交付格式和连接契约。  
  Stabilize the handoff format and connection contract before polishing any official client.

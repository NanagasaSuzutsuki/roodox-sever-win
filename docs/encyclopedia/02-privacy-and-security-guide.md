# Privacy And Security Guide / 隐私与安全指南

## Short Answer / 先给结论

这个项目支持安全连接，但代码不强制安全默认开启。  
The project supports secure connections, but the secure baseline is not enforced by default.

要达到“不是裸奔”的最低线，请至少满足：

1. `tls_enabled = true`
2. `auth_enabled = true`
3. 客户端安装导出的 CA 根证书
4. 不把 gRPC 端口直接无防护暴露到公网
5. 不把 Join Bundle、数据库备份、证书私钥、诊断负载公开或误传

## What The Code Actually Does / 代码实际上做了什么

### Transport encryption / 传输加密

- 当 `tls_enabled=true` 时，服务端使用 `internal/server/security.go` 加载或生成 TLS 服务端证书。  
  When `tls_enabled=true`, the server loads or generates TLS artifacts.
- 如果证书缺失，代码会自动生成本地 CA 与服务端证书。  
  Missing certs are auto-generated from a local CA.
- 客户端通过 `client/roodox_client.go` 加载导出的 CA 根证书并校验证书链。  
  Clients validate the chain using the exported CA root.

### Application authentication / 应用层认证

- 当 `auth_enabled=true` 时，服务端要求 gRPC metadata 里带 `x-roodox-secret`。  
  When `auth_enabled=true`, the server requires `x-roodox-secret`.
- 认证是“共享密钥模式”，不是每设备独立凭证，也不是 OAuth，也不是 mTLS。  
  This is shared-secret auth, not per-device credentials, OAuth, or mTLS.

### Critical caveat / 关键注意

`client/roodox_client.go` 里的 `sharedSecretCredentials.RequireTransportSecurity()` 返回 `false`。这意味着：

- 如果你开启了 `auth_enabled=true` 但没有开启 TLS，客户端仍然会把共享密钥发出去。  
  If auth is enabled without TLS, the client still sends the secret.
- 这种模式只能算“有认证、无保密”，不能算隐私安全。  
  This is authentication without confidentiality, not a privacy-safe setup.

所以，**共享密钥不能替代 TLS**。  
The shared secret does not replace TLS.

## Recommended Security Baseline / 推荐安全基线

| 控制项 / Control | 推荐值 / Recommended | 理由 / Why |
| --- | --- | --- |
| `tls_enabled` | `true` | 防止内容、路径、认证头在链路上明文暴露 |
| `auth_enabled` | `true` | 防止知道地址的人直接调用 RPC |
| `shared_secret` | 长随机串 / long random secret | 降低被猜中和复用风险 |
| `tls_server_name` | 与服务端证书 SAN 对齐 | 避免客户端证书校验失败或被错误绕过 |
| 网络暴露 | 内网、Tailscale、EasyTier 或防火墙白名单 | 降低公网攻击面 |
| CA 导出 | 只导出根证书，不导出根私钥 | 客户端只需 trust，不需签发能力 |
| 诊断上传 | 最小化内容 / minimize payload | 减少隐私内容在 DB 中驻留 |

## Sensitive Assets / 敏感资产分级

| 等级 / Level | 资产 / Asset | 位置 / Where | 为什么敏感 / Why |
| --- | --- | --- | --- |
| 高 / High | `shared_secret` | 配置、Join Bundle、内存、请求头 | 可直接用于冒充客户端 |
| 高 / High | `roodox-ca-key.pem` | `certs/` 或 `data_root/certs/` | 可签发受信任证书 |
| 高 / High | `roodox-server-key.pem` | 同上 | 泄露后可解密/冒充服务端 |
| 高 / High | Join Bundle | CLI 输出、Workbench 导出目录 | 可能包含地址、TLS 参数、共享密钥 |
| 高 / High | SQLite `version_blob` / DB backups / WAL | `roodox.db`, `roodox.db-wal`, `backups/` | 内含文件历史内容，不只是元数据 |
| 高 / High | `root_dir` 真实内容 | `root_dir` | 就是用户文件本体 |
| 高 / High | 设备诊断 payload | SQLite `device_diagnostics.payload` | 可能含日志、路径、错误、环境信息 |
| 中 / Medium | 运行日志 | `runtime/logs` | 可能含路径、设备 ID、错误信息 |
| 中 / Medium | 设备注册信息 | `device_registry` | 设备名、平台、overlay 地址 |
| 低 / Low | 示例配置与占位符 | `roodox.config.example.json`, 文档 | 仅在占位值规范时安全 |

## Sensitive Interfaces / 高敏感接口

| 接口 / Interface | 风险 / Risk | 处理原则 / Handling rule |
| --- | --- | --- |
| `IssueJoinBundle` | 直接产出可交付连接信息 | 只交给目标客户端，避免长期留在聊天记录或公开目录 |
| `TriggerServerBackup` | 生成高敏感 DB 备份 | 备份目录必须受控，不能误同步到公开仓库 |
| `UploadDiagnostics` | 客户端可上传高熵信息 | 客户端侧先做最小化，服务端限制保留数量 |
| `GetServerRuntime` | 暴露数据库路径、健康状态 | 只向管理员开放 |
| `GetDeviceDetail` | 暴露设备状态、动作、诊断摘要 | 仅管理面使用 |

## Overlay Positioning / Overlay 的正确定位

Roodox 对 Tailscale/EasyTier 的态度是“外部网络层，不内建，不接管”：

- `overlay_provider` 只是一个字符串标签。  
  It is just a label.
- `overlay_join_config_json` 对 Roodox 来说是透明 JSON 载荷。  
  It is opaque JSON to Roodox.
- 真正加入 tailnet 或 overlay mesh 的动作，要由客户端 bootstrap/launcher 负责。  
  The actual overlay join flow belongs to the client bootstrap layer.

因此：

- 用了 Tailscale 也不代表可以关闭 TLS。  
  Tailscale does not automatically justify disabling TLS.
- 用了 EasyTier 也不代表可以把 Join Bundle 随意公开。  
  EasyTier does not make the bundle non-sensitive.

## What Roodox Does Not Provide / 当前没做的安全能力

- 没有 mTLS 设备证书双向认证  
  No mutual TLS device identity
- 没有每设备独立 secret 或 token 轮换协议  
  No per-device secret rotation protocol
- 没有数据库透明加密  
  No transparent database encryption at rest
- 没有内建公网接入防护、WAF、账户系统  
  No built-in public-edge protection or account system
- 没有诊断内容自动脱敏  
  No automatic scrubbing of uploaded diagnostics

## Safe Sharing Matrix / 哪些可以分享、哪些不能分享

| 对象 / Object | 能否发给客户端 / Share with client? | 能否发到公开仓库 / Publish? | 备注 / Notes |
| --- | --- | --- | --- |
| CA 根证书 | 可以 / Yes | 示例可以，真实导出不建议 / Example only | 根证书可公开信任，但真实交付件不建议长期暴露 |
| CA 根私钥 | 不可以 / No | 绝对不可以 / Never | 最高敏感 |
| 服务端私钥 | 不可以 / No | 绝对不可以 / Never | 最高敏感 |
| Join Bundle | 按需 / Only target client | 不可以 / No | 如启用 auth 可能含 secret |
| 示例配置 | 可以 / Yes | 可以 / Yes | 必须用占位值 |
| 数据库备份 | 不可以 / No | 绝对不可以 / Never | 含版本内容和控制面数据 |
| 运行日志 | 谨慎 / Carefully | 不建议 / Avoid | 常含路径和设备信息 |

## Minimal Privacy Checklist / 最小隐私检查

每次准备发版、发安装包、发截图或发交付包前，至少检查：

1. 是否启用了 TLS。  
2. 是否启用了共享密钥认证。  
3. 文档、测试夹具、截图里是否出现真实地址、真实设备名、真实 secret。  
4. `runtime/`、`backups/`、`certs/`、`artifacts/handoff/` 是否被排除在公开提交之外。  
5. 数据库、WAL、备份是否被当作“内容级高敏感数据”对待。

## If You Need Stronger Security / 如果以后要继续加固

建议优先顺序：

1. 强制生产配置要求 `tls_enabled=true`。  
2. 禁止 `auth_enabled=true && tls_enabled=false` 的组合。  
3. 把共享密钥改成每设备凭证或短期 token。  
4. 给 Join Bundle 增加到期时间与最小权限模型。  
5. 对诊断上传增加大小、类型、内容审查与脱敏策略。

# Roodox Client Integration Report / Roodox 客户端对接报告

## Current Delivery Baseline / 当前交付基线

截至 `v0.1.7`，Roodox 已不是“只有服务端和一个早期 GUI”的阶段，当前对外交付基线是：

- Windows 一体化安装包 `roodox-server-win-v0.1.7-setup.exe`
- Windows 便携包 `roodox-server-win-v0.1.7-portable.zip`
- Workbench 图形工作台
- 连接码导出
- 客户端接入包导出
- 客户端导入器 `roodox_client_import.exe`

## What The Client Receives / 客户端最终拿到什么

标准客户端交付物通常是：

- `roodox-connection-code.txt`
- `roodox-client-access.json`
- `roodox-ca-cert.pem`，仅在启用 TLS 时需要
- `roodox_client_import.exe`

这意味着客户端不需要自己手填 `host`、`port`、`tls_server_name`、`shared_secret` 这类高出错字段。

## Connection Contract / 连接契约

当前推荐基线是：

- `tls_enabled = true`
- `auth_enabled = true`
- 客户端信任导出的 `roodox-ca-cert.pem`
- 客户端按交付包中的 `tlsServerName` 做证书校验
- 客户端在需要时附带 `sharedSecret`

如果你关闭 TLS，但仍启用共享密钥，那不是合格交付基线，只能算测试配置。

## Workbench Handoff Flow / Workbench 交付流

当前 Workbench 基线不是早期“只读状态面板”，而是包含首页接入流：

1. 自动发现安装实例配置
2. 设置客户端接入方式
3. 生成连接码
4. 预览 Join Bundle
5. 导出客户端接入目录

安装版默认交付目录通常在：

- `C:\ProgramData\Roodox\artifacts\handoff`

## Supported Integration Modes / 当前支持的对接模式

- `direct`
  - 适合内网直连或固定公网地址
- `tailscale`
  - 由外部 tailnet 提供可达性，Roodox 只分发连接元数据
- `easytier`
  - 由外部 overlay 提供可达性，Roodox 只分发 bootstrap 元数据

Roodox 不自己实现 Tailscale 或 EasyTier，它负责的是交付连接契约，不是替代 overlay 本身。

## Deployment Readiness / 当前成熟度判断

当前版本已经具备这些“可以对外发”的条件：

- 有安装包和便携包
- 有 GUI 入口
- 有连接码与客户端接入包导出
- 有 TLS / CA / shared-secret 交付路径
- 有 smoke QA、full QA、部署生命周期验证
- 有升级、回滚、证书轮换脚本

还不该夸大的地方也要说清楚：

- 这不是一个已经拥有完整官方客户端生态的成品平台
- overlay 网络仍依赖外部方案
- 客户端导入器更像交付助手，不是完整同步客户端

## Recommended External Message / 对外建议口径

对外更合适的表述应是：

`Roodox v0.1.7` 已提供 Windows 安装包、Workbench 图形工作台、TLS/认证交付、连接码导出和客户端接入包导出，可用于小规模正式交付与受控环境部署。

不建议再用“早期 GUI”或“仅服务端原型”这类说法，因为这会低估当前交付完成度。

## Pointers / 进一步说明

- [`USER_INSTALL.md`](USER_INSTALL.md)
- [`OPERATIONS.md`](OPERATIONS.md)
- [`encyclopedia/09-client-integration-and-handoff.md`](encyclopedia/09-client-integration-and-handoff.md)

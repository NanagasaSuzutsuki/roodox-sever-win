# Roodox Encyclopedia / Roodox 项目百科

这套文档是给“后续维护者和你自己”看的，不是宣传页。写法会尽量偏工程手册：先讲边界，再讲接口，再讲敏感面，最后讲改法。  
This set is written for maintainers and future self-service modification, not for marketing.

当前这套百科与 `v0.1.6` 公开发布内容保持同步，默认包含这些交付前提：  
This encyclopedia is aligned with the public `v0.1.6` release and assumes the following handoff baseline:

- 存在便携包 `roodox-server-win-v0.1.6-portable.zip`  
  The portable release `roodox-server-win-v0.1.6-portable.zip` exists
- 存在一体化安装包 `roodox-server-win-v0.1.6-setup.exe`  
  The all-in-one installer `roodox-server-win-v0.1.6-setup.exe` exists
- 客户端连接码导入器 `roodox_client_import.exe` 已进入正式交付物  
  `roodox_client_import.exe` is part of the shipped client handoff set
- Workbench 首页接入流、连接码生成和安装实例配置发现逻辑是当前基线  
  The home-page access flow, connection-code generation, and installed-config discovery are the current baseline

## How To Read / 建议阅读顺序

1. [`01-architecture-and-boundaries.md`](01-architecture-and-boundaries.md)  
   先搞清楚组件边界、信任边界、数据面与控制面。
2. [`02-privacy-and-security-guide.md`](02-privacy-and-security-guide.md)  
   先建立“哪些东西不能裸奔、哪些东西绝不能公开”的判断标准。
3. [`03-config-and-env-reference.md`](03-config-and-env-reference.md)  
   改配置前先看这里，避免只改 JSON 忘了环境变量覆盖。
4. [`04-grpc-api-reference.md`](04-grpc-api-reference.md)  
   所有远程接口的总表，包含文件面、同步面、控制面、管理面。
5. [`05-local-commands-and-admin-interfaces.md`](05-local-commands-and-admin-interfaces.md)  
   本地 CLI、QA 工具、Workbench Tauri 命令的总入口。
6. [`06-script-reference.md`](06-script-reference.md)  
   PowerShell 运维脚本、QA 包装脚本、GUI 启动/构建脚本。
7. [`07-runtime-storage-and-sensitive-files.md`](07-runtime-storage-and-sensitive-files.md)  
   运行时文件、数据库、WAL、证书、备份、交付物。
8. [`08-source-map.md`](08-source-map.md)  
   想改功能时先查改哪几个包和哪几个文件。
9. [`09-client-integration-and-handoff.md`](09-client-integration-and-handoff.md)  
   给客户端交付连接方式、Join Bundle、CA、认证参数的方法。
10. [`10-maintenance-playbooks-and-change-checklist.md`](10-maintenance-playbooks-and-change-checklist.md)  
   常见改法、发版前检查、隐私复查与维护操作清单。

## Core Judgements / 三个核心判断

- 这个项目支持加密，但代码并不强制你开启安全基线。真正的隐私安全基线应是 `tls_enabled=true` 且 `auth_enabled=true`。  
  The code supports encryption, but it does not force a secure baseline.
- 这个项目的 SQLite 不是“只有元数据”的轻数据库。它还会持久化文件版本内容，因此数据库、WAL、备份都应视作高敏感数据。  
  The SQLite database is content-bearing, not metadata-only.
- Tailscale 和 EasyTier 不是 Roodox 内建网络层。Roodox 只分发 overlay 元数据，真正的 overlay 安全和接入仍由外部网络系统负责。  
  Tailscale and EasyTier are external overlays, not embedded network stacks inside Roodox.

## Document Style / 文档风格

- 中文为主，关键术语保留英文。  
  Chinese-first, with key English terms preserved.
- 每章都尽量回答三个问题：它是什么、它改哪、它会泄露什么。  
  Each chapter tries to answer what it is, where to change it, and what it may leak.
- 如果你只看一份隐私手册，请先看 [`02-privacy-and-security-guide.md`](02-privacy-and-security-guide.md)。  
  If you only read one privacy-oriented chapter, start with `02-privacy-and-security-guide.md`.

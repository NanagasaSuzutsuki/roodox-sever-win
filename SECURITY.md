# Security Policy / 安全策略

## Scope / 适用范围

本仓库包含以下几类与安全直接相关的能力：  
This repository contains several security-relevant areas:

- TLS 证书生成、轮换与客户端 CA 导出  
  TLS certificate generation, rotation, and client CA export
- 共享密钥认证  
  Shared-secret authentication
- 客户端接入包与 Join Bundle  
  Client access bundles and join bundles
- 服务端运行时、数据库和部署脚本  
  Server runtime, database, and deployment scripts

## Supported Security Baseline / 当前支持的安全基线

建议生产使用时至少满足：  
For production use, the minimum recommended baseline is:

- 启用 TLS  
  Enable TLS
- 使用高强度随机 `shared_secret`  
  Use a strong random `shared_secret`
- 只向客户端分发 CA 根证书，不分发私钥  
  Distribute only the CA root certificate to clients, never private keys
- 将 `certs/`、`runtime/`、`artifacts/`、`backups/` 保持为私有目录  
  Keep `certs/`, `runtime/`, `artifacts/`, and `backups/` private
- 升级或公开发布前轮换任何曾暴露过的 secret 或证书  
  Rotate any secret or certificate that may have been exposed before upgrade or publication

## Reporting a Vulnerability / 漏洞报告

如果你发现疑似安全问题，请不要在公开 issue 中直接贴出：  
If you discover a suspected security issue, do not post the following directly in a public issue:

- 私钥  
  Private keys
- 有效共享密钥  
  Valid shared secrets
- 客户端接入包原文  
  Raw client access bundles
- 可复现攻击所需的敏感部署细节  
  Sensitive deployment details needed to reproduce an attack

当前推荐的报告方式：  
Recommended reporting path:

1. 优先使用 GitHub 的私密漏洞上报功能。  
   Please use GitHub private vulnerability reporting first.
2. 如果私密上报不可用，再通过备用邮箱联系：`roodox.manager@outlook.com`。  
   If private reporting is unavailable, use the backup contact: `roodox.manager@outlook.com`.
3. 不要在公开 issue 中直接贴出 secret、私钥、客户端接入包或真实部署材料。  
   Do not post secrets, private keys, client access bundles, or live deployment material in public issues.

## What to Include / 报告建议包含内容

- 受影响模块或脚本  
  Affected module or script
- 影响范围  
  Impact scope
- 复现步骤  
  Reproduction steps
- 需要满足的前置条件  
  Required preconditions
- 你认为的缓解方案  
  Suggested mitigation, if any

## Repository Privacy Rules / 仓库隐私规则

提交代码前，默认不应包含：  
Before committing, the repository should not contain:

- `certs/` 下的任何私钥  
  Any private keys under `certs/`
- `roodox.config.json` 这类真实部署配置  
  Real deployment config such as `roodox.config.json`
- `roodox.db*` 这类运行数据库  
  Runtime databases such as `roodox.db*`
- `artifacts/handoff/` 中的交付材料  
  Delivery material under `artifacts/handoff/`
- `runtime/`、`backups/` 中的真实环境数据  
  Live environment data under `runtime/` or `backups/`

## Hardening Checklist / 加固清单

- 发布前运行一次敏感串扫描  
  Run a sensitive-string scan before release
- 检查文档里是否出现本机路径、主机名或客户名  
  Check documents for local paths, hostnames, or client names
- 重新确认 README 中所有 token、host、secret 示例都是占位值  
  Confirm that all tokens, hosts, and secrets in the README are placeholders
- 确认证书轮换和客户端 CA 导出流程可用  
  Verify certificate rotation and client CA export flows
- 对外发布前重新执行 `go test ./...`  
  Re-run `go test ./...` before publishing

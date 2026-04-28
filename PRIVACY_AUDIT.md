# Privacy Audit / 隐私审计

更新时间 / Updated: `2026-04-28`

## Audit Goal / 审计目标

这份清单用于公开发布前确认仓库中不含真实环境隐私和敏感运行数据。  
This checklist is used before public release to confirm that the repository does not contain live-environment privacy or sensitive runtime data.

## Checked Areas / 已检查范围

- 当前工作树  
  Current working tree
- 已推送的 `main` 分支提交历史  
  Pushed `main` branch history
- 文档、示例配置、测试夹具  
  Docs, example configs, and test fixtures

## Sensitive Patterns Reviewed / 已检查的敏感模式

- 本机主机名  
  Local machine hostname
- 本机绝对路径  
  Local absolute paths
- 旧共享密钥样例  
  Previously used shared-secret samples
- 私钥标记，如 `BEGIN PRIVATE KEY`  
  Private-key markers such as `BEGIN PRIVATE KEY`
- 证书私钥文件名引用  
  Certificate private-key filename references
- 类似真实 token 的示例串  
  Placeholder strings that look like real tokens

## Current Findings / 当前结论

- 未发现先前已知的真实主机名进入公开历史。  
  No previously known real hostname was found in the public history.
- 未发现先前已知的真实共享密钥进入公开历史。  
  No previously known real shared secret was found in the public history.
- 未发现本机源码目录绝对路径残留在当前文档中。  
  No local source-directory absolute path remains in the current docs.
- 仓库中没有实际私钥文件。  
  No actual private-key files are present in the repository.
- README 与测试夹具中容易被误判为真实凭据的示例值已改为显式占位值。  
  Placeholder values in the README and test fixtures that could be mistaken for real credentials were rewritten as explicit placeholders.

## Expected Non-Secrets / 允许保留的非敏感项

以下内容属于预期存在，不应误判为泄漏：  
The following are expected and should not be treated as leaks by themselves:

- `roodox-ca-key.pem`、`roodox-server-key.pem` 这类文件名字符串  
  Filename strings such as `roodox-ca-key.pem` and `roodox-server-key.pem`
- `replace-with-*` 形式的占位 secret  
  `replace-with-*` placeholder secrets
- `example.ts.net`、`roodox.example.com` 这类示例域名  
  Example domains such as `example.ts.net` or `roodox.example.com`
- `127.0.0.1`、`localhost` 这类本地测试地址  
  Local test addresses such as `127.0.0.1` or `localhost`

## Release Checklist / 发布检查清单

每次公开发布前应重复执行：  
Before each public release, repeat the following:

1. 扫描本机路径、主机名、客户名、secret、token、私钥标记。  
   Scan for local paths, hostnames, client names, secrets, tokens, and private-key markers.
2. 确认 `.gitignore` 仍然挡住运行态、证书、数据库和交付物。  
   Confirm `.gitignore` still excludes runtime state, certificates, databases, and delivery artifacts.
3. 检查 README、QA、Operations、handoff 相关文档里的示例值。  
   Review example values in README, QA, Operations, and handoff-related docs.
4. 检查测试夹具是否误用了真实共享密钥或真实接入地址。  
   Ensure test fixtures do not use real shared secrets or live access endpoints.
5. 重新跑 `go test ./...`。  
   Re-run `go test ./...`.

# Roodox Open Source Split Guide / Roodox 开源拆分指南

更新时间 / Updated: `2026-04-27`

## Conclusion / 结论

当前这个项目不能直接把运行中的工作目录整仓推到公开仓库。  
The live working directory should not be pushed directly to a public repository as-is.

原因很简单：当前目录混合了源码、现网配置、证书、数据库、运行日志和交付物。  
The reason is straightforward: the current directory mixes source code, live configuration, certificates, databases, logs, and delivery artifacts.

推荐路线是 `allow-list` 导出，而不是“整目录公开”。  
The recommended approach is an allow-list export, not a whole-directory public push.

建议流程：  
Recommended flow:

1. 新建一个干净的公开仓库目录。  
   Create a clean public-repository workspace.
2. 只拷贝允许公开的源码和文档。  
   Copy only the source and documents that are safe to publish.
3. 不要在当前运行目录上直接 `git init` 然后推送。  
   Do not `git init` directly inside the live deployment directory and push it.

## Safe to Publish / 可以公开的内容

以下目录和文件原则上适合公开：  
The following directories and files are generally safe to publish:

- `cmd/`
- `client/`
- `internal/`
- `proto/`
- `scripts/`
- `testdata/`
- `workbench/src/`
- `workbench/src-tauri/src/`
- `workbench/src-tauri/icons/`
- `workbench/src-tauri/build.rs`
- `workbench/src-tauri/Cargo.toml`
- `workbench/src-tauri/Cargo.lock`
- `workbench/src-tauri/tauri.conf.json`
- `workbench/package.json`
- `workbench/package-lock.json`
- `workbench/tsconfig.json`
- `workbench/vite.config.ts`
- `workbench/index.html`
- `go.mod`
- `go.sum`
- `QA.md`
- `OPERATIONS.md`
- `README.md`
- `roodox.config.example.json`

补充说明：  
Notes:

- `workbench/src-tauri/icons/` 只有在图标版权明确属于你，或具备可再分发权利时才建议公开。  
  Publish `workbench/src-tauri/icons/` only if the icon rights clearly belong to you or are redistributable.
- `OPERATIONS.md`、`QA.md` 适合公开，但发布前要过一轮措辞和敏感信息检查。  
  `OPERATIONS.md` and `QA.md` are publishable, but they should be reviewed for wording and residual sensitive information.

## Publish After Sanitizing / 脱敏后再公开

以下内容不应直接公开原件，而应改为示例版或公开版：  
The following items should not be published raw and should be converted into example or public-safe versions:

- `roodox.config.json`  
  应替换为 `roodox.config.example.json`  
  Should be replaced with `roodox.config.example.json`
- `CLIENT_HANDOFF.md`  
  建议拆成公开版接入文档和私有客户交付版  
  Should be split into a public integration doc and a private client handoff doc
- 测试中的主机名、域名、共享密钥示例  
  Hostnames, domains, and shared-secret examples in tests

当前仓库里已经识别出的敏感项包括：  
Sensitive items already identified in the live workspace include:

- 真实 `shared_secret`  
  Real `shared_secret`
- 本机主机名  
  Local machine hostname
- 现网 TLS key 路径  
  Live TLS key path
- 运行数据库中的客户端接入记录  
  Client access records inside the runtime database

## Never Publish As-Is / 默认不能公开的内容

以下内容默认不应进入公开仓库：  
The following should not go into a public repository by default:

- `certs/`
- `runtime/`
- `artifacts/`
- `backups/`
- `share/`
- `roodox.config.json`
- `roodox.db`
- `roodox.db-shm`
- `roodox.db-wal`
- `roodox.db.lock`
- `roodox_server.exe`
- `roodox_qa.exe`
- `run-server.bat`
- `run-server-tls-auth.bat`
- `artifacts/handoff/`
- `artifacts/workbench/`

原因如下：  
Reasons:

- `certs/` 包含 CA 私钥和服务端私钥  
  `certs/` contains CA private keys and server private keys
- `runtime/` 包含日志、快照、缓存和回滚材料  
  `runtime/` contains logs, snapshots, caches, and rollback material
- `artifacts/` 包含交付包、CA 导出文件和 Join Bundle 导出物  
  `artifacts/` contains delivery packages, exported CA files, and join-bundle exports
- `share/` 是运行根目录，不是源码目录  
  `share/` is a runtime root, not a source directory
- `roodox.db*` 包含设备注册、接入参数和运行痕迹  
  `roodox.db*` contains device registrations, access parameters, and runtime traces
- 根目录 `*.exe/*.msi/*.zip` 是构建或交付产物  
  Root-level `*.exe/*.msi/*.zip` files are build or delivery artifacts

## Suggested Open-Core Boundary / 建议的 Open-Core 边界

当前阶段更适合 `open-core`，不建议把所有交付物和运行态材料一起公开。  
At this stage, an open-core boundary is more appropriate than publishing every delivery and runtime artifact.

建议公开：  
Recommended to publish:

- gRPC 协议  
  gRPC protocol definitions
- 服务端核心能力  
  Core server capabilities
- 客户端接入库  
  Client integration library
- Workbench 基础 GUI  
  Base workbench GUI
- 基础部署和验证脚本  
  Base deployment and validation scripts

建议私有保留或延后公开：  
Recommended to keep private or defer:

- 客户专用交付包  
  Client-specific delivery packages
- 现场运维材料  
  On-site operations material
- 已生成的接入包和证书  
  Generated access bundles and certificates
- 若版权不明确的品牌/图标素材  
  Brand or icon assets with unclear redistribution rights

## Required Actions Before Publishing / 发布前必须执行的动作

1. 轮换当前 `shared_secret`。  
   Rotate the current `shared_secret`.
2. 重新生成 `certs/` 下的 CA 和服务端证书。  
   Regenerate the CA and server certificates under `certs/`.
3. 清空或替换当前 `roodox.db`。  
   Clear or replace the current `roodox.db`.
4. 只保留示例配置，不保留真实 `roodox.config.json`。  
   Keep only the example config, not the real `roodox.config.json`.
5. 复查 `CLIENT_HANDOFF.md`、`OPERATIONS.md` 等文档是否仍含客户名、主机名、地址或本机路径。  
   Recheck docs such as `CLIENT_HANDOFF.md` and `OPERATIONS.md` for client names, hostnames, addresses, or local paths.
6. 确认 GUI 图标和安装器素材的版权归属。  
   Confirm the redistribution rights of GUI icons and installer assets.
7. 在公开仓库补齐 `README.md`、`LICENSE`、`SECURITY.md`。  
   Add `README.md`, `LICENSE`, and `SECURITY.md` to the public repository.

## License Suggestion / 许可证建议

如果目标是尽快建立信任、允许商业合作，并降低许可证摩擦，优先考虑：  
If the goal is to build trust quickly, allow commercial collaboration, and reduce license friction, prefer:

- `Apache-2.0`

如果目标是允许商用，同时希望对核心文件的修改有一定回传约束，可以考虑：  
If the goal is to allow commercial use while encouraging changes to core files to remain shareable, consider:

- `MPL-2.0`

除非你明确想限制闭源集成，否则当前阶段不建议一开始就使用强传染型许可证。  
Unless you explicitly want to restrict closed-source integration, a strongly viral license is usually not the right starting point at this stage.

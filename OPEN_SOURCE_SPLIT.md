# Roodox 开源拆分清单

更新时间：2026-04-27

## 结论

当前这个工作目录不能直接作为公开仓库推上去。

原因不是代码不能开，而是这个目录混合了四类东西：

- 源码
- 现网/本机运行配置
- 证书和认证材料
- 安装产物、数据库、日志、回滚快照

如果要公开，建议走 `allow-list` 路线：

1. 新建一个干净的公开仓库目录。
2. 只拷贝允许公开的源码和文档进去。
3. 不要在当前运行目录上直接 `git init` 后推公开远端。

## 可以公开的内容

下面这些目录和文件，原则上适合进入公开仓库：

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
- `roodox.config.example.json`

说明：

- `workbench/src-tauri/icons/` 只有在图标版权完全属于你，或来源清晰可再分发时才建议公开。
- `OPERATIONS.md` 和 `QA.md` 适合公开，但发布前应再过一轮措辞，删掉纯内部运维习惯用语。

## 需要脱敏后再公开的内容

下面这些内容不要直接公开原件，而是改成示例版：

- `roodox.config.json`
  改为 `roodox.config.example.json`
- `CLIENT_HANDOFF.md`
  建议拆成公开版接入文档和私有客户交付版
- 测试中的主机名、域名、共享密钥样例
  已建议统一改成通用占位值

当前仓库里已经发现的敏感/环境绑定信息包括：

- 真实 `shared_secret`
- 本机主机名（已脱敏）
- 现网 TLS key 路径
- 运行时数据库中的客户端接入记录

## 不能公开的内容

下面这些目录和文件默认都不应该进公开仓库：

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

原因分别是：

- `certs/` 含 CA 私钥和服务端私钥
- `runtime/` 含日志、快照、构建缓存、回滚材料
- `artifacts/` 含客户交付物、CA 导出文件、Join Bundle 导出物
- `share/` 是运行根目录，不是源码目录
- `roodox.db*` 含设备注册、接入参数、运行痕迹
- 根目录 `*.exe/*.msi/*.zip` 都是构建或交付产物，不是源码

## 建议的开源边界

现阶段更适合 `open-core`，不建议“一把梭”全开。

建议公开：

- gRPC 协议
- 服务端核心能力
- 客户端接入库
- Workbench 基础 GUI
- 基础部署与验证脚本

建议保留私有或延后公开：

- 客户专用交付包
- 现场运维材料
- 已生成的接入包和证书
- 若图标或品牌素材版权来源不明，先不要公开素材文件

## 发布前必须执行的动作

1. 轮换当前 `shared_secret`。
2. 重新生成 `certs/` 下的 CA 和服务端证书。
3. 清空或替换当前 `roodox.db`。
4. 只保留示例配置，不保留真实 `roodox.config.json`。
5. 重新检查 `CLIENT_HANDOFF.md`、`OPERATIONS.md` 是否包含客户名、主机名、地址、路径。
6. 确认 GUI 图标和安装器素材的版权归属。
7. 在公开仓库补 `README.md`、`LICENSE`、`SECURITY.md`。

## 许可建议

如果你的目标是“先建立信任、允许商业合作、减少许可证摩擦”，优先考虑：

- `Apache-2.0`

如果你的目标是“允许商用，但希望改动核心文件的人把修改回传”，可以考虑：

- `MPL-2.0`

当前阶段不建议一上来用强传染型许可证，除非你明确希望限制闭源集成。

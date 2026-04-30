# Roodox Docs / Roodox 文档总览

这个目录包含 Roodox 的功能说明、运维指引和对接文档。  
This directory contains feature, operations, and integration documentation for Roodox.

## Release Entry / 发布入口

当前公开发布版本：[`v0.1.6`](https://github.com/NanagasaSuzutsuki/roodox-sever-win/releases/tag/v0.1.6)

交付给用户时，优先区分两类产物：

When handing Roodox to users, distinguish these two release forms first:

- `roodox-server-win-v0.1.6-portable.zip`
  - 面向技术用户、测试和便携分发  
    For technical users, testing, and portable delivery
- `roodox-server-win-v0.1.6-setup.exe`
  - 面向普通 Windows 用户的一体化安装  
    For standard Windows users who want a single installer

## Start Here / 从这里开始

- [`../README.md`](../README.md): 项目总览、快速启动、Join Bundle、TLS、GUI 入口  
  Project overview, quick start, join bundle, TLS, and GUI entrypoints
- [`../README.md#4-release-package-usage--发布包使用方式`](../README.md#4-release-package-usage--发布包使用方式): release 包的安装、解压和交付方式  
  How to install, extract, and hand off the published release packages
- [`OPERATIONS.md`](OPERATIONS.md): 服务端运维、TLS 生命周期、升级回滚、GUI 运维入口  
  Server operations, TLS lifecycle, upgrade/rollback, and GUI operations surface
- [`QA.md`](QA.md): QA 命令、包装脚本、冒烟与回归入口  
  QA commands, wrappers, smoke checks, and regression entrypoints
- [`encyclopedia/README.md`](encyclopedia/README.md): 分章节的项目百科、接口索引、隐私与维护手册  
  Topic-split project encyclopedia, interface index, privacy, and maintenance guide
- [`../SECURITY.md`](../SECURITY.md): 安全报告方式与联系方式  
  Vulnerability reporting and security contact paths

## Advanced Reference / 进阶参考

后续更细的项目百科、接口清单和隐私导向手册放在 [`encyclopedia/`](encyclopedia/README.md)。  
The deeper project encyclopedia, interface references, and privacy-first guides live under [`encyclopedia/`](encyclopedia/README.md).

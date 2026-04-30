# Roodox User Install Guide / Roodox 普通用户安装指南

这份文档是给“第一次安装、不是自己改源码、只想先把程序跑起来”的用户看的。  
This guide is for first-time users who are not modifying source code and just want to get Roodox running.

## 1. Download / 下载

当前公开版本：`v0.1.6`

Release 页面：

- <https://github.com/NanagasaSuzutsuki/roodox-sever-win/releases/tag/v0.1.6>

你会看到两个主要文件：

You will see two main downloads:

- `roodox-server-win-v0.1.6-setup.exe`
  - 推荐大多数用户使用  
    Recommended for most users
- `roodox-server-win-v0.1.6-portable.zip`
  - 适合技术用户、测试或手动部署  
    For technical users, testing, or manual deployment

## 2. Which One Should I Use / 我该下哪个

### 推荐：`setup.exe`

如果你只是想安装后直接打开和使用，选这个。

Use this if you want the simplest path: install, open, and configure.

优点：

- 自动安装到 Windows 常用目录
- 自动准备可写配置目录
- 更适合后续升级
- 更适合交付给普通用户

### 可选：`portable.zip`

如果你要自己控制目录结构、自己改脚本、或者只是在测试，选这个。

Use this if you want to control folder layout yourself or are only testing.

## 3. Install With `setup.exe` / 使用安装包安装

1. 双击 `roodox-server-win-v0.1.6-setup.exe`
2. 按向导完成安装
3. 安装完成后，打开 `Roodox Workbench`

默认目录：

- 程序文件：`C:\Program Files\Roodox Server`
- 可写配置和运行数据：`C:\ProgramData\Roodox`

安装后最重要的配置文件：

- `C:\ProgramData\Roodox\roodox.config.json`

注意：

- 以后如果你是通过安装包使用 Roodox，优先改这份 `C:\ProgramData\Roodox\roodox.config.json`
- 不要把仓库里的示例配置当成实际运行配置

## 4. Use The Portable Zip / 使用便携包

1. 解压 `roodox-server-win-v0.1.6-portable.zip`
2. 打开解压后的目录
3. 先看里面的 `README.md` 和 `RELEASE.txt`
4. 修改目录里的 `roodox.config.json`
5. 运行 `start-roodox-workbench.cmd`

便携包更适合：

- 自己手动改配置
- 自己手动启动服务
- 自己控制脚本和交付物

## 5. First Launch / 第一次打开后做什么

首次打开 Workbench 后，建议按这个顺序处理：

1. 确认服务是否运行
2. 检查数据目录和共享目录
3. 设置客户端接入方式
4. 生成连接码
5. 导出客户端交付文件

如果你是给别人接入客户端，最终通常会导出这些文件：

- `roodox-client-access.json`
- `roodox-ca-cert.pem`
- `roodox-connection-code.txt`
- `roodox_client_import.exe`

## 6. What The Client Uses / 客户端怎么用

客户端不一定要手动填一堆参数。

The client side does not have to manually fill many fields.

标准做法是把下面这些交付给客户端：

The standard handoff is to give the client:

- 连接码文件 `roodox-connection-code.txt`
- CA 根证书 `roodox-ca-cert.pem`
- 导入器 `roodox_client_import.exe`
- 如有需要，再附上 `roodox-client-access.json`

## 7. Common Paths / 常见路径

- 安装版程序目录：`C:\Program Files\Roodox Server`
- 安装版配置目录：`C:\ProgramData\Roodox`
- 安装版配置文件：`C:\ProgramData\Roodox\roodox.config.json`
- 安装版日志目录：`C:\ProgramData\Roodox\runtime\logs`
- 安装版交付物目录：`C:\ProgramData\Roodox\artifacts\handoff`

## 8. If Something Looks Wrong / 如果看起来不对

优先检查这些：

1. 你改的是不是安装后的配置文件，而不是示例文件
2. 服务是否真的已经启动
3. Workbench 当前连接的是不是安装实例配置
4. TLS、主机名、共享密钥是否一致

如果只是想重新走一遍交付流程，通常从 Workbench 首页重新生成连接码即可。

## 9. Related Docs / 相关文档

- [`../README.md`](../README.md)
- [`README.md`](README.md)
- [`OPERATIONS.md`](OPERATIONS.md)
- [`encyclopedia/README.md`](encyclopedia/README.md)

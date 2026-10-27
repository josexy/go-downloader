# 下载器 · Downloader

使用 Go 与 [MyGo](https://github.com/egoist/mygo) 原生 UI 编写的桌面下载器，支持 HTTP/HTTPS 下载、断点续传、代理和文件管理。界面由 MyGo 绘制，不使用 WebView 或 HTML/JavaScript 前端。

界面为简体中文，支持深色 / 浅色 / 跟随系统，以及列表与卡片两种文件管理方式。浅色主题采用冷灰底色与白色面板，深色主题采用分层石板灰，统一使用蓝色强调操作与选中状态。列表通过细分隔线组织任务，完成、暂停与失败在状态标签和进度条中保持一致的语义色。

## 界面预览

![浅色模式下的下载列表](docs/screenshot-light.png)

![深色模式下的下载卡片](docs/screenshot-dark.png)

## 功能

- **从链接下载**：粘贴一条或多条 HTTP/HTTPS 链接；自动补全 `https://`，跟随重定向，从 `Content-Disposition`、URL 或 `Content-Type` 推断文件名；支持 URL 内嵌的用户名/密码（Basic Auth）。
- **批量与并发**：一次添加多条链接，按设定的并发数排队下载；支持全部暂停 / 全部继续。
- **代理**：
  - 无代理（直连）
  - 系统代理：Windows 读取 WinINet 注册表，macOS 读取 `scutil --proxy`，同时读取 `HTTP_PROXY` / `HTTPS_PROXY` / `ALL_PROXY` / `NO_PROXY` 环境变量
  - 自定义代理：HTTP、HTTPS、SOCKS5，支持用户名/密码与跳过列表（`localhost`、`*.local`、`10.0.0.0/8` 等），可一键测试连接
- **保存目录**：默认系统下载文件夹，可在设置中选择并校验可写；新任务即时使用新目录。
- **文件管理**：列表 / 卡片切换、搜索、状态筛选、排序、多选批量操作、双击或点击打开文件、在文件夹中显示、复制链接或保存路径、从列表移除、删除文件。
- **断点续传**：HTTP Range + `If-Range`，下载中的数据保存在 `<文件名>.part`，元信息在同名 `.part.meta`；暂停、退出、应用重启后都能继续。服务器不支持 Range、没有可用的强 ETag/Last-Modified、文件版本变化或分段不匹配时，会自动从头重下，避免拼接不同版本的数据。
- **体验**：自定义标题栏与窗口按钮，支持拖动、双击最大化/还原和 Windows 11 贴靠布局；深/浅色与跟随系统、下载完成系统通知、添加后自动开始（关闭时新任务以暂停状态加入）、并发数设置，两个主题都经过逐帧渲染检查。
- **中文优先**：应用文案为中文，MyGo 自带的控件、菜单与对话框也通过 `zh-Hans` 语言包显示中文。

## 快速开始

需要 Go 1.27.3 或更高版本；当前 MyGo 版本为 0.7.0，依赖版本以 [go.mod](go.mod) 为准。应用构建不需要 Node.js、Bun 或 cgo。

| 运行平台 | 要求 |
| --- | --- |
| Windows | Windows 10/11，无需 WebView2 Runtime |
| macOS | macOS 12 或更高版本 |
| Linux | GTK 3，无需 WebKitGTK |

在仓库根目录执行：

```sh
go mod download
go run .
```

开发时可自动重建并重启：

```sh
go tool mygo dev
```

首次下载依赖和工具需要网络；后续运行使用 Go 模块缓存。

## 构建与打包

检查所有包是否能够编译：

```sh
go build ./...
```

打包当前平台：

```sh
go tool mygo build
```

产物位于 `build/<os>-<arch>/`，应用名称和版本由 [mygo.json](mygo.json) 配置：

| 平台 | 主要产物 |
| --- | --- |
| Windows | `Downloader.exe` 与 `downloader-setup-<版本>-<架构>.exe` |
| macOS | `Downloader.app`，在 macOS 上可生成 `.dmg` |
| Linux | 可执行文件、`.desktop`、图标与 `.deb` |

跨平台打包：

```sh
go tool mygo build -platform windows/amd64,darwin/universal,linux/amd64
```

跨平台编译可在其他主机上进行，但 macOS 签名和 `.dmg` 制作需要 macOS。Windows 安装包使用 NSIS，MyGo 可在 Windows 上自动下载缺失的打包工具。

## 使用

1. 在顶部输入框粘贴链接，每行一个；点击「添加」或按 `Ctrl/Cmd + Enter`。
2. 在侧边栏切换「全部 / 正在下载 / 暂停中 / 下载完成 / 下载失败」，用顶部搜索框过滤。
3. 每一行右侧：暂停/继续/重试、在文件夹中显示、移除；右键还有复制链接、复制路径、删除文件等。
4. 点击右上角设置按钮，或按 `Ctrl/Cmd + ,`：
   - 常规：保存位置、同时下载数、完成通知、添加后自动开始
   - 代理：无代理 / 系统代理 / 自定义；可查看系统代理探测结果、重新检测、测试连接
   - 外观：浅色 / 深色 / 跟随系统，默认列表或卡片
   - 关于：版本、数据目录、开源框架链接

Windows/Linux 使用 `Ctrl`，macOS 使用 `Cmd`：

| 快捷键 | 操作 |
| --- | --- |
| `Ctrl/Cmd + Enter` | 添加输入框中的下载链接 |
| `Ctrl/Cmd + ,` | 打开设置 |
| `Ctrl/Cmd + F` | 聚焦搜索框 |
| `Ctrl/Cmd + V` | 在当前输入框中粘贴 |
| `Ctrl/Cmd + A` | 下载列表获得焦点时全选任务 |
| `Esc` | 关闭当前弹窗 |

## 代理说明

- **系统代理**优先读取系统的原生设置；Windows 读取
  `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings` 的
  `ProxyEnable`、`ProxyServer`、`ProxyOverride`。若系统只配置了 PAC
  （`AutoConfigURL`），本应用不会执行 PAC 脚本，会回退到环境变量或直连，并在设置页说明。
- 环境变量支持大小写两种写法与 `ALL_PROXY`；`NO_PROXY`（或自定义的跳过列表）支持
  `<local>`、`*.example.com`、`.example.com`、`host:port`、CIDR 与 `*`。
- 代理设置**对之后开始的连接生效**，正在下载的任务不会被中断。
- 代理密码不会写入 `settings.json`：它保存在系统凭据库（Windows 凭据管理器 / macOS 钥匙串 /
  Linux Secret Service，密钥为 `proxy-password`）。凭据库不可用时，密码只在本次运行有效。
- 下载请求使用 `Accept-Encoding: identity`，避免压缩破坏 Content-Length 与 Range 语义。

## 断点续传与数据

- 下载中的内容写入 `<保存目录>/<文件名>.part`，旁边是记录 ETag / Last-Modified / 总大小的
  `.part.meta`。完成时用 `rename` 改名为最终文件并删除元信息。
- 应用每两秒把任务记录写入 `tasks.json`；退出时会把正在下载的任务置为暂停并保存。
- 数据文件位于系统的应用数据目录（Windows 通常是 `%AppData%\Downloader\`）：
  - `settings.json`：设置（不含代理密码）
  - `tasks.json`：下载记录
  - 损坏的设置或记录文件会被重命名为 `*.bad`，应用以默认值/空列表继续启动。
- 应用每两秒在后台检查已完成的文件；文件被外部移动或删除后，记录会显示「文件缺失」，可直接重新下载。
- 使用 `go run .` 与安装版时，应用数据目录可能因应用名不同而不同。

## 项目结构

```text
main.go                         程序入口，调用 app.Run()
internal/app/
  run.go                        应用装配、单实例、语言设置与生命周期回调
  app*.go / deps.go             窗口状态、操作、设置、持久化与平台服务
  view*.go                      UI 绘制：标题栏、侧边栏、下载列表、设置与弹窗
  style.go / icons.go / format.go  主题、图标与显示格式化
  *_test.go / testdata/          应用交互测试与生成的视觉快照
internal/download/              下载队列、Range 续传、重试、文件名与任务记录
internal/proxy/                 代理配置、系统代理探测、HTTP/SOCKS5 传输
internal/settings/              设置类型
internal/store/                 设置与任务记录的 JSON 持久化
resources/ / mygo.json          应用图标与 MyGo 打包配置
scripts/make-icon.py            生成 resources/icon.png
```

UI 绘制、窗口状态和操作回调集中在 `internal/app`，只通过 `Run()` 向入口提供启动能力；新界面与应用操作放在该包中，下载、代理和存储等独立能力按职责放在对应的 `internal` 包中。

## 验证与视觉快照

常规检查：

```sh
go test ./...
go tool mygo vet .
```

`go tool mygo vet .` 包含 `go vet ./...`，并检查 MyGo UI 构建值的生命周期。纯 Go 验证可设置 `CGO_ENABLED=0`：

```powershell
# PowerShell
$env:CGO_ENABLED = "0"
go test ./...
```

```sh
# Bash
CGO_ENABLED=0 go test ./...
```

下载引擎的测试使用本地 `httptest` 服务器覆盖完整下载、暂停/继续、重启续传、服务器忽略 Range、
ETag 变化、并发上限、重试、404、0 字节、未知长度、基本认证、移除/删除与恢复；代理测试包含真实的
本地 HTTP 代理与最小 SOCKS5 服务器。UI 测试通过 `ui.Tester` 在内存中运行，无需打开窗口；系统代理探测测试会读取当前主机配置，但不依赖外部下载站点。

界面快照（浅/深 × 列表/卡片、选中状态、空状态、设置与确认弹窗，以及 960×640 最小窗口）用 `ui.Tester` 渲染真实像素：

```powershell
# PowerShell
$env:DOWNLOADER_SNAPSHOT = "1"
go test -run '^TestSnapshots$' -count=1 ./internal/app
```

```sh
# Bash
DOWNLOADER_SNAPSHOT=1 go test -run '^TestSnapshots$' -count=1 ./internal/app
```

快照输出到 `internal/app/testdata/snapshots/`，已被 Git 忽略。`-count=1` 用于重新执行测试，避免缓存结果。UI 有可见变化时，可从生成结果更新 `docs/` 中的对应截图。窗口拖动、贴靠及平台通知等原生行为仍需在目标系统中检查。

## 参与开发

代码组织、MyGo UI 生命周期、下载与数据约束、测试和交付要求见 [AGENTS.md](AGENTS.md)。UI 相关开发还应参考仓库中的 [mygo-maintenance 技能](.agents/skills/mygo-maintenance/SKILL.md)。

应用图标由 `scripts/make-icon.py` 生成，脚本需要 Pillow 与 NumPy；这些依赖仅用于图标生成，应用运行不需要 Python。

## 已知限制

- 不解析 PAC 脚本；不支持 BT/磁力、网页中的链接抓取、多连接分片下载、计划任务。
- 单窗口、单实例；退出应用会暂停进行中的下载（下次启动可继续）。
- 代理设置不作用于已经建立的连接。
- URL 内嵌的密码只保存在内存中，不写入磁盘；重启后重新添加带认证信息的同一链接，会为原暂停任务补回凭据，保留已有下载进度，并遵循「添加后自动开始」设置。
- 界面为简体中文；未提供多语言切换。

## 许可证

本项目采用 [MIT 许可证](LICENSE)。使用的框架 [egoist/mygo](https://github.com/egoist/mygo) 同样采用 MIT 许可证。

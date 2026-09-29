# Veda

Veda 是一款所见即所得（WYSIWYG）的桌面文档编辑器。你在编辑器里看到的样子，就是文档最终保存和呈现的样子，无需在「源码」与「预览」之间来回切换。

- **通用 Markdown 格式**：文档以标准 Markdown 保存，不引入私有格式或专有标记，可以被任何编辑器、静态站点生成器或版本控制工具直接读取。
- **文件本地存储**：所有文档都存放在你自己的磁盘上，Veda 只是打开和编辑它们，不接管、不上传、不加密你的文件。
- **没有云端服务**：不需要注册账号、不需要登录、不依赖任何在线服务，断网也能完整使用。
- **所见即所得编辑**：直接在排版结果上输入和修改，表格、列表、代码块、图片等元素即时呈现。
- **未来支持更多文件格式**：除 Markdown 外，后续会逐步支持更多文档格式。

> **平台支持**：Veda 目前仅支持 **macOS**（12.0 及以上）与 **Windows**，不提供 Linux 及其他平台的构建与运行支持。

## 从源码构建

### 前置依赖

| 依赖 | 版本 / 说明 |
| --- | --- |
| [Go](https://go.dev/dl/) | 1.25 或更高 |
| [Node.js](https://nodejs.org/)（含 npm） | 用于构建前端资源 |
| [Wails v3 CLI](https://v3alpha.wails.io/) | 本项目基于 `v3.0.0-beta.23` |
| [Task](https://taskfile.dev/) | 任务编排，构建脚本的入口 |

安装 Wails CLI 与 Task：

```bash
# Wails v3 CLI（与项目使用的版本保持一致）
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.23

# Task
# macOS
brew install go-task
# Windows（任选其一）
winget install Task.Task
choco install go-task
```

macOS 还需要 Xcode Command Line Tools（`xcode-select --install`）；Windows 需要 WebView2 运行时，安装包会自动处理，源码运行时可从微软官网获取。

### 开发运行

```bash
wails3 dev
```

该命令会同时启动前端开发服务器与桌面应用，并在 Go 代码变更时自动重新构建。

### 打包

在仓库根目录执行，`GOOS` 指定目标系统、`GOARCH` 指定目标架构，产物输出到 `bin/`。三种目标平台的命令如下：

```bash
# Intel macOS（x86_64）→ bin/Veda.app
wails3 package GOOS=darwin GOARCH=amd64

# Apple Silicon macOS（arm64）→ bin/Veda.app
wails3 package GOOS=darwin GOARCH=arm64

# x86 Windows（amd64）→ bin/Veda.exe 及 NSIS 安装程序
wails3 package GOOS=windows GOARCH=amd64
```

只想生成可执行文件、不打成安装包时，把 `package` 换成 `build` 即可（例如 `wails3 build GOOS=windows GOARCH=amd64`）。

如果不指定 `GOOS` / `GOARCH`，则按当前机器的系统和架构构建。

需要注意：

- macOS 的两种架构产物都落在 `bin/Veda.app`，连续构建不同架构前请先清理 `bin/`，或把上一次的产物移走。
- macOS 上 `package` 只生成 `.app` 并做 ad-hoc 签名；需要 `.dmg` 时使用 `wails3 task darwin:package:dmg ARCH=amd64`（或 `ARCH=arm64`）。对外分发还需自备 Developer ID 签名与公证。
- Windows 打包依赖 [NSIS](https://nsis.sourceforge.io/)（`makensis`），构建过程中会自动生成 WebView2 引导程序。

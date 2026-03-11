# parquet-export-gui

一个使用 Go 开发的 Parquet 导出工具，用于把 Oracle（Thin 模式）、MySQL、PostgreSQL、MaxCompute 中的单表导出为本地 Parquet 文件。

当前包含两条发布线：

- 主线 GUI/CLI：基于 Go 1.24，GUI 使用 Wails，适合 Windows 10+ 和 macOS
- `legacy-cli/`：为 Windows 7 / 8 兼容 CLI 准备的独立模块，已完成隔离、旧依赖线回退和功能验证，模块元数据已压到 `go 1.20`

## 功能

- 支持 Oracle Thin 模式连接，不依赖本地 Oracle Client
- 支持 MySQL、PostgreSQL 表导出
- 支持 MaxCompute 直连导出
- 按批次读取并写入 Parquet，避免一次性占满内存
- Wails 原生桌面窗口，前后端分离更清晰
- 提供低版本 Windows 和 Linux 可用的交互式命令行版本
- 提供 macOS 本机构建 GUI，以及 macOS 交叉编译 Windows GUI / Linux CLI 的脚本

## 环境要求

- 主线：Go 1.24+
- `legacy-cli/`：目标为 Go 1.20.x，当前模块元数据已压到 Go 1.20
- Node.js 18+
- 当前提供的构建脚本以 macOS 为主
  - macOS GUI 需要在 macOS 上构建
  - Windows GUI / CLI 与 Linux CLI 通过 macOS 交叉编译生成

## 安装

```bash
cd frontend && npm install
```

Windows PowerShell:

```powershell
Set-Location frontend
npm install
```

## 启动

CLI:

```bash
go run .
```

Legacy CLI:

```bash
cd legacy-cli
go run .
```

GUI:

```bash
cd frontend && npm run build
cd ..
go run -tags desktop .
```

## 使用说明

1. 选择数据源类型。
2. 填写连接信息和表信息。
3. 指定本地输出路径，例如 `/Users/me/Downloads/orders.parquet` 或 `C:\Users\me\Downloads\orders.parquet`。
4. 点击“测试连接”确认配置正确。
5. 点击“开始导出”。

说明：

- Oracle 的“数据库/服务名”请填写 `service_name`。
- MaxCompute 的 `Endpoint` 形如 `https://service.cn-hangzhou.maxcompute.aliyun.com/api`。
- MaxCompute 的 `Project`、`Access ID`、`Access Key` 为必填。
- 如需导出分区表，可填写 `Partition Spec`，例如 `ds='2026-03-07', region='cn'`。

## 构建

macOS GUI:

```bash
./scripts/build_macos.sh
```

macOS 交叉编译 Windows GUI `.exe`:

```bash
./scripts/build_windows_from_macos.sh
```

macOS 交叉编译主线 Windows CLI `.exe`:

```bash
./scripts/build_windows_cli_from_macos.sh
```

macOS 交叉编译 Linux x64 CLI:

```bash
./scripts/build_linux_cli_from_macos.sh
```

macOS 交叉编译 Windows 7 / 8 兼容 CLI `.exe`（隔离发布线）:

```bash
./scripts/build_legacy_windows_cli_from_macos.sh
```

运行前提：

- 该脚本只能在 macOS 上运行
- 需要可用的 Go 1.20.x 工具链
- 脚本会优先使用 `.tmp/go-toolchains/go1.20.14/go/bin/go`
- 如果未准备上述临时工具链，需要显式设置 `LEGACY_GO_BIN=/path/to/go1.20.x/bin/go`

默认输出目录：

- macOS: `dist/macos/Parquet Export Studio.app`
- Windows GUI: `dist/windows/Parquet Export Studio.exe`
- Windows CLI: `dist/windows-cli/Parquet Export Studio CLI.exe`
- Windows 7/8 Legacy CLI: `dist/windows7-cli/Parquet Export Studio Legacy CLI.exe`
- Linux CLI: `dist/linux-cli/parquet-export-studio-cli-linux-amd64`

说明：

- GUI 构建脚本直接使用 `npm run build` 和 `go build`，不依赖 Wails CLI
- `build_windows_from_macos.sh` 只能在 macOS 上生成 GUI `.exe`，不包含 Windows 安装器或签名
- `build_windows_cli_from_macos.sh` 不依赖 Wails / WebView2，但仍使用主线模块和 Go 1.24 依赖
- `build_legacy_windows_cli_from_macos.sh` 面向 `legacy-cli/` 独立模块，可显式指定 `LEGACY_GO_BIN` 选择 Go 1.20.x 工具链
- `build_linux_cli_from_macos.sh` 不依赖 Wails / WebView2，可直接从 macOS 交叉编译 Linux x64 命令行可执行文件

## 目录结构

```text
core/
main.go
main_cli.go
legacy-cli/
frontend/
build/
scripts/
  build_macos.sh
  build_windows_from_macos.sh
  build_windows_cli_from_macos.sh
  build_legacy_windows_cli_from_macos.sh
  build_linux_cli_from_macos.sh
```

## 限制

- 当前版本按“整表导出”设计，不包含字段筛选和增量同步。
- `legacy-cli/` 的功能验证已经完成，当前仍按 Go 1.20.x 兼容线维护。

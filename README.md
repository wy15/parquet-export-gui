# parquet-export-gui

一个使用 Go + Wails 重构的桌面导出工具，用于把 Oracle（Thin 模式）、MySQL、PostgreSQL、MaxCompute 中的单表导出为本地 Parquet 文件。

当前仓库采用分阶段迁移方案：

- 桌面壳和前端界面已经切到 Wails
- 连接测试和导出逻辑暂时仍复用 Python 实现，通过 bridge 子进程调用
- 因此本地开发和当前打包产物都仍需要可用的 Python 环境与依赖

## 功能

- 支持 Oracle Thin 模式连接，不依赖本地 Oracle Client
- 支持 MySQL、PostgreSQL、MaxCompute 表导出
- 按批次读取并写入 Parquet，避免一次性占满内存
- Wails 原生桌面窗口，前后端分离更清晰
- 提供 Windows 和 macOS 两套 Go + Vite 构建脚本

## 环境要求

- Go 1.23+
- Node.js 18+
- Python 3.11 到 3.13
- 请在目标平台本机执行构建
  - macOS 包需要在 macOS 上构建
  - Windows 包需要在 Windows 上构建

## 安装

```bash
python -m venv .venv
source .venv/bin/activate
pip install -U pip
pip install -e .
cd frontend && npm install
```

Windows PowerShell:

```powershell
python -m venv .venv
.venv\Scripts\Activate.ps1
pip install -U pip
pip install -e .
Set-Location frontend
npm install
```

## 启动

```bash
cd frontend && npm run build
cd ..
go run .
```

## 使用说明

1. 选择数据源类型。
2. 填写连接信息和表信息。
3. 指定本地输出路径，例如 `/Users/me/Downloads/orders.parquet` 或 `C:\Users\me\Downloads\orders.parquet`。
4. 点击“测试连接”确认配置正确。
5. 点击“开始导出”。

说明：

- Oracle 的“数据库/服务名”请填写 `service_name`。
- MaxCompute 需要填写 `Endpoint`、`Project`、`Access ID`、`Access Key`。
- MaxCompute 分区表可选填 `partition spec`，例如 `ds=20260306,region=cn`。

## 构建

macOS:

```bash
./scripts/build_macos.sh
```

Windows:

```powershell
./scripts/build_windows.ps1
```

默认输出目录：

- macOS: `dist/macos/Parquet Export Studio.app`
- Windows: `dist/windows/Parquet Export Studio.exe`

说明：

- 当前 Wails 桌面程序会调用仓库里的 Python bridge
- 运行时会优先查找 `PYTHON_BIN`，否则尝试从仓库根目录或其祖先目录下发现 `.venv`
- 如果你把构建产物单独拷贝到别的目录，Python bridge 目前不会自动随包迁移
- 当前构建脚本直接使用 `npm run build` 和 `go build`，不依赖 Wails CLI

## 目录结构

```text
app.go
main.go
frontend/
build/
src/parquet_export_gui/
  bridge.py
  __main__.py
  exporters.py
  gui.py
  main.py
  models.py
scripts/
  build_macos.sh
  build_windows.ps1
```

## 限制

- 这次重构优先替换桌面壳和 UI，导出内核还没有完全迁移到 Go
- 当前打包产物仍依赖 Python 运行时与相关三方库
- 当前版本按“整表导出”设计，不包含字段筛选和增量同步。
- MaxCompute 走表读取接口导出，不处理自定义 SQL 结果集。

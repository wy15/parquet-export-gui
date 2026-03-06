# parquet-export-gui

一个使用 NiceGUI 开发的桌面导出工具，用于把 Oracle（Thin 模式）、MySQL、PostgreSQL、MaxCompute 中的单表导出为本地 Parquet 文件，并通过 Nuitka 打包为 Windows / macOS 可执行程序。

## 功能

- 支持 Oracle Thin 模式连接，不依赖本地 Oracle Client
- 支持 MySQL、PostgreSQL、MaxCompute 表导出
- 按批次读取并写入 Parquet，避免一次性占满内存
- NiceGUI 原生桌面窗口模式，适合 Nuitka 打包
- 提供 Windows 和 macOS 两套构建脚本

## 环境要求

- Python 3.11 到 3.13
- 打包时请在目标平台本机执行 Nuitka
  - macOS 包需要在 macOS 上构建
  - Windows 包需要在 Windows 上构建

## 安装

```bash
python -m venv .venv
source .venv/bin/activate
pip install -U pip
pip install -e ".[build]"
```

Windows PowerShell:

```powershell
python -m venv .venv
.venv\Scripts\Activate.ps1
pip install -U pip
pip install -e ".[build]"
```

## 启动

```bash
parquet-export-gui
```

或者：

```bash
python -m parquet_export_gui
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

## 打包

macOS:

```bash
./scripts/build_macos.sh
```

Windows:

```powershell
./scripts/build_windows.ps1
```

默认输出目录：

- macOS: `dist/macos`
- Windows: `dist/windows`

## 目录结构

```text
src/parquet_export_gui/
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

- 当前版本按“整表导出”设计，不包含字段筛选和增量同步。
- MaxCompute 走表读取接口导出，不处理自定义 SQL 结果集。

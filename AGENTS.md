# Codex 协作说明（parquet-export-gui）

本仓库包含两个版本：Wails 桌面 GUI 和纯 Go CLI。请先确认当前任务针对哪一端。

## 运行与构建

CLI（交互式终端）：

```bash
go run .
```

GUI（Wails，桌面）：

```bash
cd frontend && npm install
cd frontend && npm run build
cd ..
go run -tags desktop .
```

说明：

- GUI 入口走 `main.go`，仅在 `desktop` build tag 下生效
- GUI 启动前需要先构建 `frontend/dist`，因为 Go 会将其 embed 到桌面程序中

构建脚本在 `scripts/`：

- macOS GUI：`./scripts/build_macos.sh`
- Windows GUI：`./scripts/build_windows.ps1`
- macOS 交叉编译 Windows GUI：`./scripts/build_windows_from_macos.sh`
- macOS 交叉编译 Windows CLI：`./scripts/build_windows_cli_from_macos.sh`
- macOS 交叉编译 Linux CLI：`./scripts/build_linux_cli_from_macos.sh`

## 代码结构

- Go 入口：`main.go`（默认 CLI）、`main_cli.go`（CLI UI）、`app.go` / `app_desktop.go`（GUI/Wails）
- 导出核心：`native_export.go`
- 前端（Wails）：`frontend/`（构建后产物进入 `frontend/dist`）
- 构建产物：`dist/`

## 修改约定

- 如果任务涉及 GUI，请同时检查 `app.go`、`app_desktop.go` 与 `frontend/` 的接口契约（导出方法、事件名、参数结构、JSON 字段）。
- 如果任务涉及 CLI，请优先修改 `main.go` / `main_cli.go`，避免引入 Wails 依赖。
- 导出逻辑尽量集中在 `native_export.go`，避免在 UI 层实现导出细节。

## 测试与验证

当前没有自动化测试。修改后请说明你如何手动验证：

- CLI：说明输入流程是否走通
- GUI：说明“测试连接”“开始导出”关键路径是否可用

## 输出期望

当你完成改动，请在回复中包含：

- 变更摘要（1-3 条）
- 关键文件清单
- 若未运行验证，明确说明原因

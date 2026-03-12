# Codex 协作说明（parquet-export-gui）

本仓库包含两个版本：Wails 桌面 GUI 和纯 Go CLI。请先确认当前任务针对哪一端。

## 运行与构建

CLI（交互式终端）：

```bash
go run ./cmd/cli
```

GUI（Wails，桌面）：

```bash
cd frontend && npm install
wails dev
```

说明：

- GUI 入口走根目录 `main.go`
- GUI 生产构建走 `wails build`
- `frontend/dist` 仍然会在生产构建时被 embed 到桌面程序中
- `build_release_matrix.sh` 会集中产出 GUI、主线 CLI、Legacy CLI 的发布包；当前 `windows/arm64` CLI 默认跳过 UPX 压缩

构建脚本在 `scripts/`：

- 全量发布矩阵：`./scripts/build_release_matrix.sh`
- macOS GUI：`./scripts/build_macos.sh`
- macOS 交叉编译 Windows GUI：`./scripts/build_windows_from_macos.sh`
- macOS 交叉编译 Windows CLI：`./scripts/build_windows_cli_from_macos.sh`
- macOS 交叉编译 Linux CLI：`./scripts/build_linux_cli_from_macos.sh`
- macOS 交叉编译 Windows 7 / 8 Legacy CLI：`./scripts/build_legacy_windows_cli_from_macos.sh`

## 代码结构

- Go 入口：`main.go`（GUI/Wails）、`cmd/cli/main.go`（CLI）
- 桌面绑定包装：`app.go` / `app_desktop.go`
- 导出核心：`internal/appcore/`
- 前端（Wails）：`frontend/`（构建后产物进入 `frontend/dist`）
- GUI 构建产物：`build/bin/`
- CLI 构建产物：`dist/`

## 修改约定

- 如果任务涉及 GUI，请同时检查 `app.go`、`app_desktop.go` 与 `frontend/` 的接口契约（导出方法、事件名、参数结构、JSON 字段）。
- 如果任务涉及 CLI，请优先修改 `cmd/cli/`，避免引入 Wails 依赖。
- 导出逻辑尽量集中在 `internal/appcore/`，避免在 UI 层实现导出细节。

## 测试与验证

当前没有自动化测试。修改后请说明你如何手动验证：

- CLI：说明输入流程是否走通
- GUI：说明“测试连接”“开始导出”关键路径是否可用

## 输出期望

当你完成改动，请在回复中包含：

- 变更摘要（1-3 条）
- 关键文件清单
- 若未运行验证，明确说明原因

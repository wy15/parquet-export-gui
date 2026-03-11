# legacy-cli

这个目录是给 Windows 7 / 8 兼容 CLI 准备的独立模块。

当前状态：

- 已经和主线 GUI / Wails 模块隔离
- 可以单独维护自己的 `go.mod`
- 已经把 `mysql`、`pgx`、`Arrow` 主依赖降到了旧版本线，并修掉了旧版 SDK 的 API 差异
- 模块 `go.mod` 已经压到 `go 1.20`
- 当前仓库内已用现有工具链完成离线构建验证，但还没有用真实 `go1.20.x` 二进制复测

当前剩余的主要工作：

- 用真实 `go1.20.x` 二进制再跑一轮构建
- 在真实的 Windows 7 / 8 环境验证导出流程

构建入口：

```bash
./scripts/build_legacy_windows_cli_from_macos.sh
```

如果仓库里存在 `.tmp/go-toolchains/go1.20.14/go/bin/go`，脚本会优先使用它。

如果本机同时装了 Go 1.20，可显式指定：

```bash
LEGACY_GO_BIN=/path/to/go1.20.x/bin/go ./scripts/build_legacy_windows_cli_from_macos.sh
```

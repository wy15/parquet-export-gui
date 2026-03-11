# legacy-cli

这个目录是给 Windows 7 / 8 兼容 CLI 准备的独立模块。

当前状态：

- 已经和主线 GUI / Wails 模块隔离
- 可以单独维护自己的 `go.mod`
- 已经把 `mysql`、`pgx`、`Arrow` 主依赖降到了旧版本线，并修掉了旧版 SDK 的 API 差异
- 模块 `go.mod` 已经压到 `go 1.20`
- 交互式导出流程的功能验证已经完成

构建入口：

```bash
./scripts/build_legacy_windows_cli_from_macos.sh
```

运行前提：

- 该脚本只能在 macOS 上运行
- 必须提供可用的 Go 1.20.x 工具链
- 脚本会先检查 `LEGACY_GO_BIN`，未设置时再回退到仓库内的 `.tmp/go-toolchains/go1.20.14/go/bin/go`
- 如果找到的 `go` 不是 1.20.x，脚本会直接退出

如果仓库里存在 `.tmp/go-toolchains/go1.20.14/go/bin/go`，脚本会优先使用它。

如果本机同时装了 Go 1.20，可显式指定：

```bash
LEGACY_GO_BIN=/path/to/go1.20.x/bin/go ./scripts/build_legacy_windows_cli_from_macos.sh
```

编译 32 位 x86：

```bash
WINDOWS_ARCH=386 LEGACY_GO_BIN=/path/to/go1.20.x/bin/go ./scripts/build_legacy_windows_cli_from_macos.sh
```

未设置时默认使用 `WINDOWS_ARCH=amd64`。

本地运行：

```bash
go run .
```

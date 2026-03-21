#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This script must be run on macOS." >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RELEASE_DIR="$ROOT_DIR/dist/release"
MACOS_DIR="$RELEASE_DIR/macos"
WINDOWS_GUI_DIR="$RELEASE_DIR/windows-gui"
WINDOWS_CLI_DIR="$RELEASE_DIR/windows-cli"
LINUX_CLI_DIR="$RELEASE_DIR/linux-cli"
WINDOWS_LEGACY_DIR="$RELEASE_DIR/windows-legacy-cli"
BUILD_BIN_DIR="$ROOT_DIR/build/bin"

APP_NAME="Parquet Export Studio"
WINDOWS_GUI_NAME="$APP_NAME.exe"
WINDOWS_CLI_X64_NAME="Parquet Export Studio CLI x64.exe"
WINDOWS_CLI_ARM64_NAME="Parquet Export Studio CLI arm64.exe"
LINUX_CLI_X64_NAME="parquet-export-studio-cli-linux-amd64"
LINUX_CLI_ARM64_NAME="parquet-export-studio-cli-linux-arm64"
LEGACY_X64_NAME="Parquet Export Studio Legacy CLI x64.exe"
LEGACY_X86_NAME="Parquet Export Studio Legacy CLI x86.exe"

export GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.cache/go-mod}"

WAILS_BIN="${WAILS_BIN:-}"
if [[ -z "$WAILS_BIN" ]]; then
  if command -v wails >/dev/null 2>&1; then
    WAILS_BIN="$(command -v wails)"
  elif [[ -x "$HOME/go/bin/wails" ]]; then
    WAILS_BIN="$HOME/go/bin/wails"
  else
    echo "wails CLI not found. Set WAILS_BIN or install it with:" >&2
    echo "  go install github.com/wailsapp/wails/v2/cmd/wails@latest" >&2
    exit 1
  fi
fi

UPX_BIN="${UPX_BIN:-}"
if [[ -z "$UPX_BIN" ]]; then
  if command -v upx >/dev/null 2>&1; then
    UPX_BIN="$(command -v upx)"
  else
    echo "upx not found. Install it first or set UPX_BIN." >&2
    exit 1
  fi
fi

mkdir -p \
  "$MACOS_DIR" \
  "$WINDOWS_GUI_DIR" \
  "$WINDOWS_CLI_DIR" \
  "$LINUX_CLI_DIR" \
  "$WINDOWS_LEGACY_DIR" \
  "$GOCACHE" \
  "$GOMODCACHE"

compress_with_upx() {
  local target="$1"
  echo "Compressing with UPX: $target"
  if ! "$UPX_BIN" --best --lzma "$target"; then
    echo "UPX compression failed for: $target" >&2
    echo "Current UPX binary: $UPX_BIN" >&2
    echo "If this target format is not supported by your UPX build, the file will remain uncompressed." >&2
    return 1
  fi
}

copy_clean() {
  local source="$1"
  local target="$2"

  rm -rf "$target"
  mkdir -p "$(dirname "$target")"
  cp -R "$source" "$target"
}

build_macos_gui() {
  echo "==> Building macOS GUI"
  (
    cd "$ROOT_DIR"
    "$WAILS_BIN" build \
      --clean \
      --platform darwin/arm64 \
      -o "$APP_NAME"
  )

  copy_clean "$BUILD_BIN_DIR/$APP_NAME.app" "$MACOS_DIR/$APP_NAME.app"
}

build_windows_gui() {
  echo "==> Building Windows GUI x64"
  (
    cd "$ROOT_DIR"
    "$WAILS_BIN" build \
      --clean \
      --platform windows/amd64 \
      --webview2 download \
      -upx \
      -upxflags "--best --lzma" \
      -o "$WINDOWS_GUI_NAME"
  )

  copy_clean "$BUILD_BIN_DIR/$WINDOWS_GUI_NAME" "$WINDOWS_GUI_DIR/$WINDOWS_GUI_NAME"
}

build_cross_cli() {
  local goos="$1"
  local goarch="$2"
  local output="$3"

  echo "==> Building ${goos}/${goarch} CLI"
  (
    cd "$ROOT_DIR"
    GOOS="$goos" \
    GOARCH="$goarch" \
    CGO_ENABLED=0 \
    go build \
      -buildvcs=false \
      -trimpath \
      -ldflags "-w -s" \
      -o "$output" \
      ./cmd/cli
  )
}

build_legacy_cli() {
  local arch="$1"
  local final_name="$2"

  echo "==> Building legacy Windows CLI $arch"
  (
    cd "$ROOT_DIR"
    WINDOWS_ARCH="$arch" ./scripts/build_legacy_windows_cli_from_macos.sh
  )

  copy_clean \
    "$ROOT_DIR/dist/windows7-cli/Parquet Export Studio Legacy CLI.exe" \
    "$WINDOWS_LEGACY_DIR/$final_name"
  compress_with_upx "$WINDOWS_LEGACY_DIR/$final_name"
}

build_macos_gui
build_windows_gui
build_cross_cli windows amd64 "$WINDOWS_CLI_DIR/$WINDOWS_CLI_X64_NAME"
# compress_with_upx "$WINDOWS_CLI_DIR/$WINDOWS_CLI_X64_NAME"
build_cross_cli linux amd64 "$LINUX_CLI_DIR/$LINUX_CLI_X64_NAME"
# compress_with_upx "$LINUX_CLI_DIR/$LINUX_CLI_X64_NAME"
build_cross_cli linux arm64 "$LINUX_CLI_DIR/$LINUX_CLI_ARM64_NAME"
# compress_with_upx "$LINUX_CLI_DIR/$LINUX_CLI_ARM64_NAME"
build_legacy_cli amd64 "$LEGACY_X64_NAME"
build_legacy_cli 386 "$LEGACY_X86_NAME"
build_cross_cli windows arm64 "$WINDOWS_CLI_DIR/$WINDOWS_CLI_ARM64_NAME"
echo "Skipping UPX for $WINDOWS_CLI_DIR/$WINDOWS_CLI_ARM64_NAME (current UPX does not support windows/arm64)"

echo
echo "Build matrix finished."
echo "Artifacts:"
echo "  macOS GUI:          $MACOS_DIR/$APP_NAME.app"
echo "  Windows GUI x64:    $WINDOWS_GUI_DIR/$WINDOWS_GUI_NAME"
echo "  Windows CLI x64:    $WINDOWS_CLI_DIR/$WINDOWS_CLI_X64_NAME"
echo "  Windows CLI arm64:  $WINDOWS_CLI_DIR/$WINDOWS_CLI_ARM64_NAME (uncompressed)"
echo "  Linux CLI amd64:    $LINUX_CLI_DIR/$LINUX_CLI_X64_NAME"
echo "  Linux CLI arm64:    $LINUX_CLI_DIR/$LINUX_CLI_ARM64_NAME"
echo "  Legacy CLI x64:     $WINDOWS_LEGACY_DIR/$LEGACY_X64_NAME"
echo "  Legacy CLI x86:     $WINDOWS_LEGACY_DIR/$LEGACY_X86_NAME"

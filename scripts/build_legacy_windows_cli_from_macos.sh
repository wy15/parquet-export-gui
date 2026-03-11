#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This script must be run on macOS." >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LEGACY_DIR="$ROOT_DIR/legacy-cli"
OUTPUT_DIR="$ROOT_DIR/dist/windows7-cli"
APP_NAME="Parquet Export Studio Legacy CLI"
BUILD_OUTPUT="$OUTPUT_DIR/$APP_NAME.exe"
WINDOWS_ARCH="${WINDOWS_ARCH:-amd64}"
TEMP_GO_BIN="$ROOT_DIR/.tmp/go-toolchains/go1.20.14/go/bin/go"
GO_BIN="${LEGACY_GO_BIN:-}"

if [[ -z "$GO_BIN" && -x "$TEMP_GO_BIN" ]]; then
  GO_BIN="$TEMP_GO_BIN"
fi

if [[ -z "$GO_BIN" ]]; then
  GO_BIN="go"
fi

export GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.cache/go-mod}"

mkdir -p "$OUTPUT_DIR" "$GOCACHE" "$GOMODCACHE"

pushd "$LEGACY_DIR" >/dev/null
GOOS=windows \
GOARCH="$WINDOWS_ARCH" \
CGO_ENABLED=0 \
"$GO_BIN" build \
  -buildvcs=false \
  -trimpath \
  -ldflags "-w -s" \
  -o "$BUILD_OUTPUT" \
  .
popd >/dev/null

echo "Legacy Windows CLI build finished: $BUILD_OUTPUT"
"$GO_BIN" version
echo "Set LEGACY_GO_BIN=/path/to/go1.20.x/bin/go to override the bundled temporary toolchain."

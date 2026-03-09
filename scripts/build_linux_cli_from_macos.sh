#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This script must be run on macOS." >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="$ROOT_DIR/dist/linux-cli"
APP_NAME="parquet-export-studio-cli-linux-amd64"
BUILD_OUTPUT="$OUTPUT_DIR/$APP_NAME"

export GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.cache/go-mod}"

mkdir -p "$OUTPUT_DIR" "$GOCACHE" "$GOMODCACHE"

GOOS=linux \
GOARCH=amd64 \
CGO_ENABLED=0 \
go build \
  -buildvcs=false \
  -trimpath \
  -ldflags "-w -s" \
  -o "$BUILD_OUTPUT" \
  .

chmod +x "$BUILD_OUTPUT"

echo "Linux x64 CLI cross-build finished: $BUILD_OUTPUT"
echo "This build excludes Wails/WebView2 and is intended for Linux command-line environments."

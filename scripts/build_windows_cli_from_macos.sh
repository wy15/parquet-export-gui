#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This script must be run on macOS." >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="$ROOT_DIR/dist/windows-cli"
APP_NAME="Parquet Export Studio CLI"
BUILD_OUTPUT="$OUTPUT_DIR/$APP_NAME.exe"
WINDOWS_ARCH="${WINDOWS_ARCH:-amd64}"

export GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.cache/go-mod}"

mkdir -p "$OUTPUT_DIR" "$GOCACHE" "$GOMODCACHE"

GOOS=windows \
GOARCH="$WINDOWS_ARCH" \
CGO_ENABLED=0 \
go build \
  -buildvcs=false \
  -trimpath \
  -ldflags "-w -s" \
  -o "$BUILD_OUTPUT" \
  ./cmd/cli

echo "Windows CLI cross-build finished: $BUILD_OUTPUT"
echo "This build excludes Wails/WebView2 and is intended for lower-version Windows environments."

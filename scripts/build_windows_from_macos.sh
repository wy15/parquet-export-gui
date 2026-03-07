#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This script must be run on macOS." >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="$ROOT_DIR/dist/windows"
FRONTEND_DIR="$ROOT_DIR/frontend"
APP_NAME="Parquet Export Studio"
BUILD_OUTPUT="$OUTPUT_DIR/$APP_NAME.exe"
WINDOWS_ARCH="${WINDOWS_ARCH:-amd64}"

export GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.cache/go-mod}"

mkdir -p "$OUTPUT_DIR" "$GOCACHE" "$GOMODCACHE"

if [[ ! -d "$FRONTEND_DIR/node_modules" ]]; then
  (
    cd "$FRONTEND_DIR"
    npm install
  )
fi

(
  cd "$FRONTEND_DIR"
  npm run build
)

GOOS=windows \
GOARCH="$WINDOWS_ARCH" \
CGO_ENABLED=0 \
go build \
  -buildvcs=false \
  -tags desktop,wv2runtime.download,production \
  -ldflags "-H=windowsgui -w -s" \
  -o "$BUILD_OUTPUT"

echo "Windows cross-build finished: $BUILD_OUTPUT"
echo "Note: this produces a standalone .exe from macOS, not a signed installer."

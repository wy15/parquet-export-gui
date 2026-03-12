#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This script must be run on macOS." >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WINDOWS_ARCH="${WINDOWS_ARCH:-amd64}"

cd "$ROOT_DIR"
wails build --clean --platform "windows/${WINDOWS_ARCH}" --webview2 download -o "Parquet Export Studio.exe"

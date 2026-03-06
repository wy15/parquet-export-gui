#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PYTHON_BIN="${PYTHON_BIN:-$ROOT_DIR/.venv/bin/python}"
OUTPUT_DIR="$ROOT_DIR/dist/macos"

"$PYTHON_BIN" -m nuitka \
  --standalone \
  --macos-create-app-bundle \
  --assume-yes-for-downloads \
  --include-package=parquet_export_gui \
  --include-package=nicegui \
  --include-package=webview \
  --include-package=pyarrow \
  --include-package=odps \
  --nofollow-import-to=tkinter,pytest \
  --product-name="Parquet Export Studio" \
  --output-dir="$OUTPUT_DIR" \
  "$ROOT_DIR/src/parquet_export_gui/__main__.py"

echo "macOS build finished: $OUTPUT_DIR"

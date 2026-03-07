#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PYTHON_BIN="${PYTHON_BIN:-$ROOT_DIR/.venv/bin/python}"
OUTPUT_DIR="$ROOT_DIR/dist/macos"
CACHE_DIR="${NUITKA_CACHE_DIR:-$ROOT_DIR/.cache/nuitka}"
APP_NAME="Parquet Export Studio"

mkdir -p "$CACHE_DIR"

NUITKA_CACHE_DIR="$CACHE_DIR" "$PYTHON_BIN" -m nuitka \
  --standalone \
  --macos-create-app-bundle \
  --assume-yes-for-downloads \
  --python-flag=-m \
  --include-package-data=nicegui:templates/index.html \
  --include-package-data=nicegui:elements/*.js \
  --include-package-data=nicegui:elements/*.vue \
  --no-deployment-flag=self-execution \
  --noinclude-data-files=nicegui/elements/lib/mermaid/chunks/mermaid.esm.min \
  --nofollow-import-to=tkinter,pytest,pyarrow.tests \
  --output-filename="$APP_NAME" \
  --output-folder-name="$APP_NAME.app" \
  --macos-app-name="$APP_NAME" \
  --product-name="$APP_NAME" \
  --output-dir="$OUTPUT_DIR" \
  "$ROOT_DIR/src/parquet_export_gui"

echo "macOS build finished: $OUTPUT_DIR"

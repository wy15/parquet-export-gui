$ErrorActionPreference = "Stop"

$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$python = if ($env:PYTHON_BIN) { $env:PYTHON_BIN } else { Join-Path $root ".venv\Scripts\python.exe" }
$output = Join-Path $root "dist\windows"
$cache = if ($env:NUITKA_CACHE_DIR) { $env:NUITKA_CACHE_DIR } else { Join-Path $root ".cache\nuitka" }
$appName = "Parquet Export Studio"

New-Item -ItemType Directory -Force -Path $cache | Out-Null

& {
  $env:NUITKA_CACHE_DIR = $cache
  & $python -m nuitka `
  --standalone `
  --assume-yes-for-downloads `
  --windows-console-mode=disable `
  --python-flag=-m `
  --include-package-data=nicegui:templates/index.html `
  --include-package-data=nicegui:elements/*.js `
  --include-package-data=nicegui:elements/*.vue `
  --no-deployment-flag=self-execution `
  --noinclude-data-files=nicegui/elements/lib/mermaid/chunks/mermaid.esm.min `
  --nofollow-import-to=tkinter,pytest,pyarrow.tests `
  --output-filename=$appName `
  --product-name=$appName `
  --output-dir=$output `
  (Join-Path $root "src\parquet_export_gui")
}

Write-Host "Windows build finished: $output"

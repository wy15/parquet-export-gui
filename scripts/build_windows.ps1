$ErrorActionPreference = "Stop"

$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$python = if ($env:PYTHON_BIN) { $env:PYTHON_BIN } else { Join-Path $root ".venv\Scripts\python.exe" }
$output = Join-Path $root "dist\windows"

& $python -m nuitka `
  --standalone `
  --assume-yes-for-downloads `
  --windows-console-mode=disable `
  --include-package=parquet_export_gui `
  --include-package=nicegui `
  --include-package=webview `
  --include-package=pyarrow `
  --include-package=odps `
  --nofollow-import-to=tkinter,pytest `
  --product-name="Parquet Export Studio" `
  --output-dir=$output `
  (Join-Path $root "src\parquet_export_gui\__main__.py")

Write-Host "Windows build finished: $output"

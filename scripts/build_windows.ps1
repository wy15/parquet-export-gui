$ErrorActionPreference = "Stop"

$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$output = Join-Path $root "dist\windows"
$frontend = Join-Path $root "frontend"
$appName = "Parquet Export Studio"
$buildOutput = Join-Path $output "$appName.exe"

$env:GOCACHE = if ($env:GOCACHE) { $env:GOCACHE } else { Join-Path $root ".cache\go-build" }
$env:GOMODCACHE = if ($env:GOMODCACHE) { $env:GOMODCACHE } else { Join-Path $root ".cache\go-mod" }

New-Item -ItemType Directory -Force -Path $output, $env:GOCACHE, $env:GOMODCACHE | Out-Null

if (-not (Test-Path (Join-Path $frontend "node_modules"))) {
  Push-Location $frontend
  npm install
  Pop-Location
}

Push-Location $frontend
npm run build
Pop-Location

go build `
  -buildvcs=false `
  -tags "desktop,wv2runtime.download,production" `
  -ldflags "-H=windowsgui -w -s" `
  -o $buildOutput

Write-Host "Windows build finished: $buildOutput"

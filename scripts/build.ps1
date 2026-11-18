# Builds ZVintedWatch from source on Windows: web interface, then a single binary.
#
#   powershell -ExecutionPolicy Bypass -File scripts\build.ps1
#   $env:GOOS = "linux"; $env:GOARCH = "amd64"; .\scripts\build.ps1   # cross-compile
#
# Requirements: Go (see go.mod) and Node.js 20+.
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

$version = $env:VERSION
if (-not $version) {
    $version = (git describe --tags --always --dirty 2>$null)
    if (-not $version) { $version = "dev" }
    $version = $version -replace '^v', ''
}
$commit = (git rev-parse --short HEAD 2>$null)
if (-not $commit) { $commit = "none" }
$date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$pkg = "github.com/Nikola-Putnik/ZVintedWatch/internal/version"

Write-Host "==> Building the web interface"
npm --prefix web ci --no-audit --no-fund
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
npm --prefix web run build
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$goos = if ($env:GOOS) { $env:GOOS } else { (go env GOOS) }
$goarch = if ($env:GOARCH) { $env:GOARCH } else { (go env GOARCH) }
$ext = if ($goos -eq "windows") { ".exe" } else { "" }
$out = "bin/zvintedwatch_${goos}_${goarch}${ext}"

Write-Host "==> Building $out (version $version)"
$env:CGO_ENABLED = "0"
$env:GOOS = $goos
$env:GOARCH = $goarch
go build -trimpath -ldflags "-s -w -X $pkg.Version=$version -X $pkg.Commit=$commit -X $pkg.Date=$date" -o $out ./cmd/zvintedwatch
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "==> Done: $out"

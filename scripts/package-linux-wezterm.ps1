# The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
# you may not use this file except in compliance with the License. You may obtain a copy of the License at
# https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
# basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
# Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
# wangsychn. All Rights Reserved. Contributor(s): .

param(
    [string]$Version = "latest",
    [string]$BuildVersion = "dev",
    [switch]$Prepare,
    [switch]$SkipArchive
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "wezterm-common.ps1")

$repoRoot = Get-PhytozomeRepoRoot
$release = Resolve-WezTermLinuxRelease $Version
$cacheRoot = Get-LinuxWezTermCacheRoot $repoRoot
$assetDir = Join-Path $cacheRoot $release.Tag
$appImagePath = Join-Path $assetDir $release.Name
$bundleDir = Join-Path $repoRoot "bin\phytozome-go_linux_amd64_wezterm"
$archivePath = Join-Path $repoRoot "bin\phytozome-go_linux_amd64_wezterm.tar.gz"
$launcherPath = Join-Path $bundleDir "phytozome-go"
$binaryPath = Join-Path $bundleDir "phytozome-go.bin"
$cleanCachePath = Join-Path $bundleDir "phytozome-go-cleancache.bin"

if ($Prepare -or -not (Test-Path -LiteralPath $appImagePath -PathType Leaf)) {
    New-Item -ItemType Directory -Force -Path $assetDir | Out-Null
    $ProgressPreference = "SilentlyContinue"
    Invoke-WebRequest -Uri $release.URL -OutFile $appImagePath
}

Remove-Item -LiteralPath $bundleDir -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $bundleDir | Out-Null
Copy-Item -LiteralPath $appImagePath -Destination (Join-Path $bundleDir "wezterm.AppImage") -Force
Write-PhytozomeWezTermConfig -Path (Join-Path $bundleDir "wezterm.lua") -Version $BuildVersion

$weztermScript = @'
#!/bin/sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec "$SCRIPT_DIR/wezterm.AppImage" --config-file "$SCRIPT_DIR/wezterm.lua" "$@"
'@
$weztermScript | Set-Content -LiteralPath (Join-Path $bundleDir "wezterm") -Encoding Ascii

$launcherScript = @'
#!/bin/sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
export WEZTERM_CONFIG_FILE="$SCRIPT_DIR/wezterm.lua"
if [ "$#" -eq 0 ]; then
  exec "$SCRIPT_DIR/wezterm" start --always-new-process --cwd "$SCRIPT_DIR"
fi
exec "$SCRIPT_DIR/wezterm" start --always-new-process --cwd "$SCRIPT_DIR" -- "$SCRIPT_DIR/phytozome-go.bin" "$@"
'@
$launcherScript | Set-Content -LiteralPath $launcherPath -Encoding Ascii

Push-Location $repoRoot
try {
    $oldGOOS = $env:GOOS
    $oldGOARCH = $env:GOARCH
    $oldCGO = $env:CGO_ENABLED
    try {
        $env:GOOS = "linux"
        $env:GOARCH = "amd64"
        $env:CGO_ENABLED = "0"
        go build -trimpath -ldflags="-X main.version=$BuildVersion" -o $binaryPath .\cmd\phytozome-go
        if ($LASTEXITCODE -ne 0) {
            throw "go build linux/amd64 phytozome-go failed"
        }
        go build -trimpath -ldflags="-X main.version=$BuildVersion" -o $cleanCachePath .\cmd\phytozome-go-cleancache
        if ($LASTEXITCODE -ne 0) {
            throw "go build linux/amd64 phytozome-go-cleancache failed"
        }
    } finally {
        if ($null -eq $oldGOOS) { Remove-Item -LiteralPath Env:\GOOS -ErrorAction SilentlyContinue } else { Set-Item -LiteralPath Env:\GOOS -Value $oldGOOS }
        if ($null -eq $oldGOARCH) { Remove-Item -LiteralPath Env:\GOARCH -ErrorAction SilentlyContinue } else { Set-Item -LiteralPath Env:\GOARCH -Value $oldGOARCH }
        if ($null -eq $oldCGO) { Remove-Item -LiteralPath Env:\CGO_ENABLED -ErrorAction SilentlyContinue } else { Set-Item -LiteralPath Env:\CGO_ENABLED -Value $oldCGO }
    }
} finally {
    Pop-Location
}

if (-not $SkipArchive) {
    Remove-Item -LiteralPath $archivePath -Force -ErrorAction SilentlyContinue
    python (Join-Path $PSScriptRoot "create-tar.py") $archivePath (Join-Path $repoRoot "bin\phytozome-go_linux_amd64_wezterm") "phytozome-go_linux_amd64_wezterm" "phytozome-go" "phytozome-go.bin" "phytozome-go-cleancache.bin" "wezterm" "wezterm.AppImage"
    if ($LASTEXITCODE -ne 0) {
        throw "Could not create Linux WezTerm archive: $archivePath"
    }
}

Write-Host "Linux WezTerm bundle staged at: $bundleDir"
if (-not $SkipArchive) {
    Write-Host "Archive written to: $archivePath"
}

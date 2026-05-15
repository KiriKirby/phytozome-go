# The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
# you may not use this file except in compliance with the License. You may obtain a copy of the License at
# https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
# basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
# Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
# wangsychn. All Rights Reserved. Contributor(s): .

param(
    [string]$Version = "latest",
    [string]$BuildVersion = "dev",
    [ValidateSet("amd64", "arm64")]
    [string]$GOARCH = "arm64",
    [switch]$Prepare,
    [switch]$SkipArchive
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "wezterm-common.ps1")

$repoRoot = Get-PhytozomeRepoRoot
$release = Resolve-WezTermMacOSRelease $Version
$cacheRoot = Get-MacOSWezTermCacheRoot $repoRoot
$assetDir = Join-Path $cacheRoot $release.Tag
$zipPath = Join-Path $assetDir $release.Name
$expandedDir = Join-Path $assetDir "expanded"
$sourceAppDir = Join-Path $expandedDir "WezTerm-macos-$($release.Tag)\WezTerm.app"
$bundleDir = Join-Path $repoRoot "bin\phytozome-go_macos_${GOARCH}_wezterm"
$appBundleDir = Join-Path $bundleDir "phytozome GO.app"
$appMacOSDir = Join-Path $appBundleDir "Contents\MacOS"
$appResourcesDir = Join-Path $appBundleDir "Contents\Resources"
$archivePath = Join-Path $repoRoot "bin\phytozome-go_macos_${GOARCH}_wezterm.tar.gz"
$cleanCachePath = Join-Path $appMacOSDir "phytozome-go-cleancache.bin"

if ($Prepare -or -not (Test-Path -LiteralPath $sourceAppDir -PathType Container)) {
    New-Item -ItemType Directory -Force -Path $assetDir | Out-Null
    if (-not (Test-Path -LiteralPath $zipPath -PathType Leaf)) {
        $ProgressPreference = "SilentlyContinue"
        Invoke-WebRequest -Uri $release.URL -OutFile $zipPath
    }
    Remove-Item -LiteralPath $expandedDir -Recurse -Force -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Force -Path $expandedDir | Out-Null
    tar -xf $zipPath -C $expandedDir
    if ($LASTEXITCODE -ne 0) {
        throw "Could not extract macOS WezTerm archive: $zipPath"
    }
}

if (-not (Test-Path -LiteralPath $sourceAppDir -PathType Container)) {
    throw "Prepared macOS WezTerm app bundle is missing: $sourceAppDir"
}

Remove-Item -LiteralPath $bundleDir -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $bundleDir | Out-Null
New-Item -ItemType Directory -Force -Path $appBundleDir | Out-Null
New-Item -ItemType Directory -Force -Path $appMacOSDir | Out-Null
New-Item -ItemType Directory -Force -Path $appResourcesDir | Out-Null
Copy-Item -LiteralPath (Join-Path $sourceAppDir '*') -Destination $appBundleDir -Recurse -Force
Remove-Item -LiteralPath (Join-Path $appBundleDir "Contents\_CodeSignature") -Recurse -Force -ErrorAction SilentlyContinue

$launcherScript = @'
#!/bin/sh
set -eu
APP_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
WEZTERM_BIN="$APP_DIR/wezterm"
if [ "$#" -eq 0 ]; then
  exec "$WEZTERM_BIN" start --always-new-process --cwd "$APP_DIR"
fi
exec "$WEZTERM_BIN" start --always-new-process --cwd "$APP_DIR" -- "$APP_DIR/phytozome-go.bin" "$@"
'@
$launcherPath = Join-Path $appMacOSDir "phytozome-go"
$launcherScript | Set-Content -LiteralPath $launcherPath -Encoding Ascii

$weztermScript = @'
#!/bin/sh
set -eu
APP_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
CONFIG_FILE="$APP_DIR/../Resources/wezterm.lua"
exec "$APP_DIR/wezterm-gui" --config-file "$CONFIG_FILE" "$@"
'@
$weztermScript | Set-Content -LiteralPath (Join-Path $appMacOSDir "wezterm") -Encoding Ascii

$safeVersion = ($BuildVersion -replace '^v', '')

$plistPath = Join-Path $appBundleDir "Contents\Info.plist"
$plist = @"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleDisplayName</key>
  <string>phytozome GO</string>
  <key>CFBundleExecutable</key>
  <string>phytozome-go</string>
  <key>CFBundleIconFile</key>
  <string>terminal.icns</string>
  <key>CFBundleIdentifier</key>
  <string>org.phytozome.go</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>phytozome GO</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>$safeVersion</string>
  <key>CFBundleVersion</key>
  <string>$safeVersion</string>
  <key>LSMinimumSystemVersion</key>
  <string>10.15</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
"@
$plist | Set-Content -LiteralPath $plistPath -Encoding UTF8

Write-PhytozomeWezTermConfig -Path (Join-Path $appResourcesDir "wezterm.lua") -Version $BuildVersion

Push-Location $repoRoot
try {
    $oldGOOS = $env:GOOS
    $oldGOARCH = $env:GOARCH
    $oldCGO = $env:CGO_ENABLED
    try {
        $env:GOOS = "darwin"
        $env:GOARCH = $GOARCH
        $env:CGO_ENABLED = "0"
        go build -trimpath -ldflags="-X main.version=$BuildVersion" -o (Join-Path $appMacOSDir "phytozome-go.bin") .\cmd\phytozome-go
        go build -trimpath -ldflags="-X main.version=$BuildVersion" -o $cleanCachePath .\cmd\phytozome-go-cleancache
        if ($LASTEXITCODE -ne 0) {
            throw "go build darwin/$GOARCH failed"
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
    python (Join-Path $PSScriptRoot "create-tar.py") $archivePath $appBundleDir "phytozome GO.app" "Contents/MacOS" "Contents/MacOS/wezterm"
    if ($LASTEXITCODE -ne 0) {
        throw "Could not create macOS WezTerm archive: $archivePath"
    }
}

Write-Host "macOS WezTerm bundle staged at: $appBundleDir"
if (-not $SkipArchive) {
    Write-Host "Archive written to: $archivePath"
}

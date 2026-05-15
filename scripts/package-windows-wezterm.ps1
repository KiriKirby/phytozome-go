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
    [switch]$SkipZip
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "windows-wezterm-common.ps1")

$repoRoot = Get-PhytozomeRepoRoot
$release = Resolve-WezTermWindowsRelease $Version
$preparedDir = Get-PreparedWindowsWezTermDir $repoRoot $release.Tag
$bundleDir = Join-Path $repoRoot "bin\phytozome-go_windows_amd64_wezterm"
$appPath = Join-Path $bundleDir "phytozome-go.bin"
$zipPath = Join-Path $repoRoot "bin\phytozome-go_windows_amd64_wezterm.zip"

if ($Prepare -or -not (Test-Path -LiteralPath (Join-Path $preparedDir "wezterm.bin") -PathType Leaf) -or -not (Test-Path -LiteralPath (Join-Path $preparedDir "phytozome-go.exe") -PathType Leaf)) {
    & (Join-Path $PSScriptRoot "prepare-windows-wezterm.ps1") -Version $release.Tag
}

Remove-Item -LiteralPath $bundleDir -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $bundleDir | Out-Null

Copy-Item -Path (Join-Path $preparedDir "*") -Destination $bundleDir -Recurse -Force
Write-PhytozomeWezTermConfig -Path (Join-Path $bundleDir "wezterm.lua") -Version $BuildVersion
Remove-Item -LiteralPath (Join-Path $bundleDir "phytozome-go-window-icon.png") -Force -ErrorAction SilentlyContinue
& (Join-Path $PSScriptRoot "update-windows-icon.ps1") -Source "docs\logo2.png"

Push-Location $repoRoot
try {
	go build -trimpath -ldflags="-X main.version=$BuildVersion" -o $appPath .\cmd\phytozome-go
	go build -trimpath -ldflags="-H=windowsgui -X main.version=$BuildVersion" -o (Join-Path $bundleDir "phytozome-go.exe") .\cmd\phytozome-go-winlauncher
	go build -trimpath -ldflags="-X main.version=$BuildVersion" -o (Join-Path $bundleDir "phytozome-go-cleancache.bin") .\cmd\phytozome-go-cleancache
} finally {
	Pop-Location
}

& (Join-Path $PSScriptRoot "set-exe-icon.ps1") -ExePath (Join-Path $bundleDir "wezterm.bin") -IconPath (Join-Path $repoRoot "cmd\phytozome-go-winlauncher\phytozome-go.ico")
Copy-Item -LiteralPath (Join-Path $bundleDir "wezterm.bin") -Destination (Join-Path $bundleDir "wezterm-cli.bin") -Force

if (-not $SkipZip) {
    Remove-Item -LiteralPath $zipPath -Force -ErrorAction SilentlyContinue
    Compress-Archive -Path (Join-Path $bundleDir "*") -DestinationPath $zipPath -Force
}

Write-Host "Windows WezTerm bundle staged at: $bundleDir"
if (-not $SkipZip) {
    Write-Host "Zip written to: $zipPath"
}

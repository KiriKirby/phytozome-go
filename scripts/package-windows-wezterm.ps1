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

Push-Location $repoRoot
try {
    go build -trimpath -ldflags="-X main.version=$BuildVersion" -o $appPath .\cmd\phytozome-go
} finally {
    Pop-Location
}

if (-not $SkipZip) {
    Remove-Item -LiteralPath $zipPath -Force -ErrorAction SilentlyContinue
    Compress-Archive -Path (Join-Path $bundleDir "*") -DestinationPath $zipPath -Force
}

Write-Host "Windows WezTerm bundle staged at: $bundleDir"
if (-not $SkipZip) {
    Write-Host "Zip written to: $zipPath"
}

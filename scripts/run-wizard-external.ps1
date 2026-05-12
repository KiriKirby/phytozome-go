# The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
# you may not use this file except in compliance with the License. You may obtain a copy of the License at
# https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
# basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
# Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
# wangsychn. All Rights Reserved. Contributor(s): .

param(
    [string]$Version = "latest",
    [switch]$Prepare
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "windows-wezterm-common.ps1")

$repoRoot = Get-PhytozomeRepoRoot
$bundleDir = Join-Path $repoRoot "bin\phytozome-go_windows_amd64_wezterm"
$launcherPath = Join-Path $bundleDir "phytozome-go.exe"
$appPath = Join-Path $bundleDir "phytozome-go.bin"
$terminalPath = Join-Path $bundleDir "wezterm.bin"
$configPath = Join-Path $bundleDir "wezterm.lua"

if ($Prepare) {
    & (Join-Path $PSScriptRoot "package-windows-wezterm.ps1") -Version $Version -BuildVersion "dev" -Prepare -SkipZip
} else {
    & (Join-Path $PSScriptRoot "package-windows-wezterm.ps1") -Version $Version -BuildVersion "dev" -SkipZip
}

foreach ($requiredFile in @($launcherPath, $appPath, $terminalPath, $configPath)) {
    if (-not (Test-Path -LiteralPath $requiredFile -PathType Leaf)) {
        throw "Missing debug bundle file: $requiredFile"
    }
}

# Start the same Windows bundle used by packaged builds. The GUI launcher
# detaches immediately and opens the app in the bundled WezTerm terminal.
$startProcessArgs = @{
    FilePath = $launcherPath
    WorkingDirectory = $bundleDir
    WindowStyle = "Hidden"
}

$separatorIndex = [Array]::IndexOf($args, "--")
if ($separatorIndex -ge 0 -and $separatorIndex -lt ($args.Count - 1)) {
    $startProcessArgs.ArgumentList = $args[($separatorIndex + 1)..($args.Count - 1)]
}

Start-Process @startProcessArgs
Write-Host "Started Windows WezTerm debug bundle:"
Write-Host "  $launcherPath"

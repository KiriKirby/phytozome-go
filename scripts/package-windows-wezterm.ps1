# The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
# you may not use this file except in compliance with the License. You may obtain a copy of the License at
# https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
# basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
# Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
# wangsychn. All Rights Reserved. Contributor(s): .

param(
    [string]$Version = "main",
    [string]$BuildVersion = "dev",
    [switch]$Prepare,
    [switch]$SkipZip
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "windows-wezterm-common.ps1")

function Invoke-GoBuild {
	param(
		[string]$Label,
		[string[]]$Arguments
	)
	go @Arguments
	if ($LASTEXITCODE -ne 0) {
		throw "$Label failed with exit code $LASTEXITCODE"
	}
}

$repoRoot = Get-PhytozomeRepoRoot
$sourceBuild = $Version -eq "main" -or $Version.StartsWith("source:", [System.StringComparison]::OrdinalIgnoreCase)
if ($sourceBuild) {
    $revision = if ($Version -eq "main") { "main" } else { $Version.Substring("source:".Length) }
    $preparedDir = (& (Join-Path $PSScriptRoot "prepare-windows-wezterm-source.ps1") -Revision $revision | Select-Object -Last 1)
    if ([string]::IsNullOrWhiteSpace($preparedDir)) {
        throw "WezTerm source build did not return a prepared runtime directory."
    }
} else {
    $release = Resolve-WezTermWindowsRelease $Version
    $preparedDir = Get-PreparedWindowsWezTermDir $repoRoot $release.Tag
}
$bundleDir = Join-Path $repoRoot "bin\phytozome-go_windows_amd64_wezterm"
$appPath = Join-Path $bundleDir "core.bin"
$zipPath = Join-Path $repoRoot "bin\phytozome-go_windows_amd64_wezterm.zip"
$runtimeSourceDir = Join-Path $repoRoot "assets\mega-phgo-runtime\windows-amd64\runtime"

if (-not $sourceBuild -and (
    $Prepare -or
    -not (Test-Path -LiteralPath (Join-Path $preparedDir "wezterm.bin") -PathType Leaf) -or
    -not (Test-Path -LiteralPath (Join-Path $preparedDir "phytozome-go.exe") -PathType Leaf) -or
    (Test-Path -LiteralPath (Join-Path $preparedDir "mesa") -PathType Container)
)) {
    & (Join-Path $PSScriptRoot "prepare-windows-wezterm.ps1") -Version $release.Tag
}
& (Join-Path $PSScriptRoot "prepare-mega-phgo-runtime.ps1")

Remove-Item -LiteralPath $bundleDir -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $bundleDir | Out-Null

Copy-Item -Path (Join-Path $preparedDir "*") -Destination $bundleDir -Recurse -Force
Copy-Item -Path (Join-Path $runtimeSourceDir "*") -Destination $bundleDir -Force
Remove-Item -LiteralPath (Join-Path $bundleDir "mesa") -Recurse -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath (Join-Path $bundleDir "opengl32.dll") -Force -ErrorAction SilentlyContinue
Write-PhytozomeWezTermConfig -Path (Join-Path $bundleDir "wezterm.lua") -Version $BuildVersion
$generatedConfigPath = Join-Path $bundleDir "wezterm.lua"
$generatedConfig = Get-Content -LiteralPath $generatedConfigPath -Raw
if ($generatedConfig -match '(?m)^\s*show_close_tab_button_in_tabs\s*=') {
    throw "Generated WezTerm config contains unsupported field show_close_tab_button_in_tabs."
}
foreach ($requiredConfig in @(
    'use_fancy_tab_bar = true',
    "window_decorations = is_windows and 'INTEGRATED_BUTTONS|RESIZE'",
    "integrated_title_button_style = 'Windows'",
    "integrated_title_buttons = { 'Hide', 'Maximize', 'Close' }"
)) {
    if (-not $generatedConfig.Contains($requiredConfig)) {
        throw "Generated WezTerm config is missing required window/tab setting: $requiredConfig"
    }
}
Remove-Item -LiteralPath (Join-Path $bundleDir "phytozome-go-window-icon.png") -Force -ErrorAction SilentlyContinue
& (Join-Path $PSScriptRoot "update-windows-icon.ps1") -SmallSource "docs\logo3small.png" -LargeSource "docs\logo3large.png"

Push-Location $repoRoot
try {
	$coreLdflags = "-X main.version=$BuildVersion"
	Invoke-GoBuild "go build core" @("build", "-trimpath", "-ldflags=$coreLdflags", "-o", $appPath, ".\cmd\phytozome-go")
	Invoke-GoBuild "go build launcher" @("build", "-trimpath", "-ldflags=-H=windowsgui -X main.version=$BuildVersion", "-o", (Join-Path $bundleDir "phytozome-go.exe"), ".\cmd\phytozome-go-winlauncher")
	Invoke-GoBuild "go build startup helper" @("build", "-trimpath", "-ldflags=-X main.version=$BuildVersion", "-o", (Join-Path $bundleDir "phgohelper.bin"), ".\cmd\phytozome-go-cleancache")
} finally {
	Pop-Location
}

& (Join-Path $PSScriptRoot "set-exe-icon.ps1") -ExePath (Join-Path $bundleDir "wezterm.bin") -IconPath (Join-Path $repoRoot "cmd\phytozome-go-winlauncher\phytozome-go.ico")

if (-not $SkipZip) {
    Remove-Item -LiteralPath $zipPath -Force -ErrorAction SilentlyContinue
    Compress-Archive -Path (Join-Path $bundleDir "*") -DestinationPath $zipPath -Force
}

Write-Host "Windows WezTerm bundle staged at: $bundleDir"
if (-not $SkipZip) {
    Write-Host "Zip written to: $zipPath"
}

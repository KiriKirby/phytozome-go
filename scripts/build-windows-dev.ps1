# The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
# you may not use this file except in compliance with the License. You may obtain a copy of the License at
# https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
# basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
# Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
# wangsychn. All Rights Reserved. Contributor(s): .

param(
    [string]$BuildVersion = "dev",
    [string]$WezTermVersion = "main",
    [switch]$SkipTests,
    [switch]$SkipVet,
    [switch]$SkipBuildCheck
)

$ErrorActionPreference = "Stop"

function Invoke-Checked {
    param(
        [string]$Label,
        [scriptblock]$Script
    )

    Write-Host ""
    Write-Host "==> $Label"
    & $Script
    if ($LASTEXITCODE -ne 0) {
        throw "$Label failed with exit code $LASTEXITCODE"
    }
}

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Split-Path -Parent $scriptDir
$binDir = Join-Path $repoRoot "bin"

Push-Location $repoRoot
try {
    $resolvedRepo = (Resolve-Path -LiteralPath $repoRoot).Path
    $resolvedBin = [System.IO.Path]::GetFullPath($binDir)
    if (-not $resolvedBin.StartsWith($resolvedRepo, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to clear unexpected bin path: $resolvedBin"
    }

    Remove-Item -LiteralPath $resolvedBin -Recurse -Force -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Force -Path $resolvedBin | Out-Null
    Set-Content -LiteralPath (Join-Path $resolvedBin "dev-build.txt") -Value $BuildVersion -Encoding ASCII

    if (-not $SkipTests) {
        Invoke-Checked "go test ./..." { go test ./... }
    }
    if (-not $SkipVet) {
        Invoke-Checked "go vet ./..." { go vet ./... }
    }
    if (-not $SkipBuildCheck) {
        Invoke-Checked "go build ./cmd/..." { go build ./cmd/... }
    }

    Invoke-Checked "Reactree tree viewer assets" {
        powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-tree-viewer.ps1
    }

    Invoke-Checked "Windows WezTerm dev bundle" {
        powershell -NoProfile -ExecutionPolicy Bypass -File scripts\package-windows-wezterm.ps1 -Version $WezTermVersion -BuildVersion $BuildVersion -SkipZip
    }
} finally {
    Pop-Location
}

Write-Host ""
Write-Host "Windows-only development build finished under:"
Write-Host "  $binDir"

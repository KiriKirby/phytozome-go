# The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
# you may not use this file except in compliance with the License. You may obtain a copy of the License at
# https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
# basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
# Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
# wangsychn. All Rights Reserved. Contributor(s): .

param(
    [string]$Tag = "latest",
    [switch]$Force
)

$ErrorActionPreference = "Stop"

function Invoke-Checked {
    param([string]$Label, [scriptblock]$Action)
    & $Action
    if ($LASTEXITCODE -ne 0) {
        throw "$Label failed with exit code $LASTEXITCODE"
    }
}

$assetName = "phytozome-go_windows_amd64_wezterm.zip"
$resolvedTag = $Tag.Trim()
if ([string]::IsNullOrWhiteSpace($resolvedTag) -or $resolvedTag -eq "latest") {
    $resolvedTag = (gh release view --json tagName --jq .tagName).Trim()
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($resolvedTag)) {
        throw "Could not resolve the latest PHgo release tag."
    }
}

$safeTag = $resolvedTag -replace '[^A-Za-z0-9._-]', '_'
$localAppData = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
if ([string]::IsNullOrWhiteSpace($localAppData)) {
    throw "LocalApplicationData is unavailable."
}
$cacheRoot = Join-Path $localAppData "phytozome-go\build-cache\wezterm\windows-phgo-release"
$preparedDir = Join-Path $cacheRoot ("prepared-" + $safeTag)
$metadataPath = Join-Path $preparedDir "wezterm-phgo-release.json"
$requiredFiles = @(
    "wezterm.bin",
    "wezterm-cli.bin",
    "wezterm-mux-server.bin",
    "openconsole.bin",
    "conpty.dll",
    "wezterm-source.json"
)

if (-not $Force) {
    $complete = $true
    foreach ($name in $requiredFiles) {
        $item = Get-Item -LiteralPath (Join-Path $preparedDir $name) -ErrorAction SilentlyContinue
        if (-not $item -or $item.Length -le 0) {
            $complete = $false
            break
        }
    }
    if ($complete) {
        try {
            $metadata = Get-Content -LiteralPath $metadataPath -Raw | ConvertFrom-Json
            $complete = $metadata.release_tag -eq $resolvedTag
        } catch {
            $complete = $false
        }
    }
    if ($complete) {
        Write-Host "Reusing WezTerm binaries from PHgo release $resolvedTag at $preparedDir"
        Write-Output $preparedDir
        return
    }
}

New-Item -ItemType Directory -Force -Path $cacheRoot | Out-Null
$workDir = Join-Path $cacheRoot ("download-" + [Guid]::NewGuid().ToString("N"))
$extractDir = Join-Path $workDir "extract"
New-Item -ItemType Directory -Force -Path $workDir | Out-Null

try {
    Invoke-Checked "download PHgo Windows release runtime" {
        gh release download $resolvedTag --pattern $assetName --pattern "SHA256SUMS.txt" --dir $workDir --clobber
    }

    $archivePath = Join-Path $workDir $assetName
    $sumsPath = Join-Path $workDir "SHA256SUMS.txt"
    $escapedAssetName = [Regex]::Escape($assetName)
    $sumLine = Get-Content -LiteralPath $sumsPath | Where-Object { $_ -match "^([0-9a-fA-F]{64})\s+\*?$escapedAssetName$" } | Select-Object -First 1
    if (-not $sumLine) {
        throw "SHA256SUMS.txt does not contain $assetName for PHgo release $resolvedTag."
    }
    $expectedHash = ([Regex]::Match($sumLine, '^([0-9a-fA-F]{64})')).Groups[1].Value.ToLowerInvariant()
    $actualHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $expectedHash) {
        throw "PHgo release asset SHA-256 mismatch for ${assetName}: expected $expectedHash, got $actualHash."
    }

    Expand-Archive -LiteralPath $archivePath -DestinationPath $extractDir -Force
    Remove-Item -LiteralPath $preparedDir -Recurse -Force -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Force -Path $preparedDir | Out-Null
    foreach ($name in $requiredFiles) {
        $source = Join-Path $extractDir $name
        if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
            throw "PHgo release $resolvedTag is missing required WezTerm runtime file: $name"
        }
        Copy-Item -LiteralPath $source -Destination (Join-Path $preparedDir $name) -Force
    }

    $sourceMetadata = Get-Content -LiteralPath (Join-Path $preparedDir "wezterm-source.json") -Raw | ConvertFrom-Json
    if ([string]::IsNullOrWhiteSpace([string]$sourceMetadata.commit)) {
        throw "PHgo release $resolvedTag has invalid wezterm-source.json metadata."
    }
    [ordered]@{
        release_tag = $resolvedTag
        asset = $assetName
        asset_sha256 = $actualHash
        wezterm_commit = [string]$sourceMetadata.commit
        prepared_utc = [DateTime]::UtcNow.ToString("o")
    } | ConvertTo-Json | Set-Content -LiteralPath $metadataPath -Encoding UTF8

    Write-Host "Prepared WezTerm binaries from PHgo release $resolvedTag at $preparedDir"
    Write-Output $preparedDir
} finally {
    Remove-Item -LiteralPath $workDir -Recurse -Force -ErrorAction SilentlyContinue
}

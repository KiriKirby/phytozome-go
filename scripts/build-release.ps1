# The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
# you may not use this file except in compliance with the License. You may obtain a copy of the License at
# https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
# basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
# Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
# wangsychn. All Rights Reserved. Contributor(s): .

param(
    [string]$BuildVersion = "",
    [string]$WezTermVersion = "latest",
    [switch]$SkipTests,
    [switch]$SkipVet,
    [switch]$SkipBuildCheck,
    [switch]$Publish,
    [string]$ReleaseTitle = "",
    [string]$ReleaseNotes = ""
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

function Restore-EnvValue {
    param(
        [string]$Name,
        [AllowNull()]
        [string]$Value
    )

    if ($null -eq $Value) {
        Remove-Item -LiteralPath "Env:\$Name" -ErrorAction SilentlyContinue
    } else {
        Set-Item -LiteralPath "Env:\$Name" -Value $Value
    }
}

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Split-Path -Parent $scriptDir
$binDir = Join-Path $repoRoot "bin"
$zipPath = Join-Path $binDir "phytozome-go_windows_amd64_wezterm.zip"
$linuxArchivePath = Join-Path $binDir "phytozome-go_linux_amd64_wezterm.tar.gz"
$macIntelArchivePath = Join-Path $binDir "phytozome-go_macos_amd64_wezterm.tar.gz"
$macArmArchivePath = Join-Path $binDir "phytozome-go_macos_arm64_wezterm.tar.gz"

if ([string]::IsNullOrWhiteSpace($BuildVersion)) {
    $BuildVersion = "v" + (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
}

Push-Location $repoRoot
try {
    $resolvedRepo = (Resolve-Path -LiteralPath $repoRoot).Path
    $resolvedBin = [System.IO.Path]::GetFullPath($binDir)
    if (-not $resolvedBin.StartsWith($resolvedRepo, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to clear unexpected bin path: $resolvedBin"
    }

    Remove-Item -LiteralPath $resolvedBin -Recurse -Force -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Force -Path $resolvedBin | Out-Null
    Set-Content -LiteralPath (Join-Path $resolvedBin "release-tag.txt") -Value $BuildVersion -Encoding ASCII

    if (-not $SkipTests) {
        Invoke-Checked "go test ./..." { go test ./... }
    }
    if (-not $SkipVet) {
        Invoke-Checked "go vet ./..." { go vet ./... }
    }
    if (-not $SkipBuildCheck) {
        Invoke-Checked "go build ./..." { go build ./... }
    }

    Invoke-Checked "Windows WezTerm package" {
        powershell -NoProfile -ExecutionPolicy Bypass -File scripts\package-windows-wezterm.ps1 -Version $WezTermVersion -BuildVersion $BuildVersion
    }
    Invoke-Checked "Linux WezTerm package" {
        powershell -NoProfile -ExecutionPolicy Bypass -File scripts\package-linux-wezterm.ps1 -Version $WezTermVersion -BuildVersion $BuildVersion
    }
    Invoke-Checked "macOS Intel WezTerm package" {
        powershell -NoProfile -ExecutionPolicy Bypass -File scripts\package-macos-wezterm.ps1 -Version $WezTermVersion -BuildVersion $BuildVersion -GOARCH amd64
    }
    Invoke-Checked "macOS Apple Silicon WezTerm package" {
        powershell -NoProfile -ExecutionPolicy Bypass -File scripts\package-macos-wezterm.ps1 -Version $WezTermVersion -BuildVersion $BuildVersion -GOARCH arm64
    }

    $entries = @(tar -tf $zipPath)
    foreach ($required in @("phytozome-go.exe", "phytozome-go.bin", "phytozome-go-cleancache.bin", "wezterm.bin", "wezterm-cli.bin", "wezterm.lua")) {
        if (-not ($entries -contains $required)) {
            throw "Windows zip is missing required file: $required"
        }
    }
    foreach ($forbidden in @("docs/logo.png", "docs/logo2.png", "logo.png", "logo2.png", "phytozome-go-window-icon.png")) {
        if ($entries -contains $forbidden) {
            throw "Windows zip must not package logo image file: $forbidden"
        }
    }

    $linuxEntries = @(tar -tf $linuxArchivePath)
    foreach ($required in @("phytozome-go_linux_amd64_wezterm/phytozome-go", "phytozome-go_linux_amd64_wezterm/phytozome-go.bin", "phytozome-go_linux_amd64_wezterm/phytozome-go-cleancache.bin", "phytozome-go_linux_amd64_wezterm/wezterm", "phytozome-go_linux_amd64_wezterm/wezterm.AppImage", "phytozome-go_linux_amd64_wezterm/wezterm.lua")) {
        if (-not ($linuxEntries -contains $required)) {
            throw "Linux archive is missing required file: $required"
        }
    }

    foreach ($macArchive in @($macIntelArchivePath, $macArmArchivePath)) {
        $macEntries = @(tar -tf $macArchive)
        foreach ($required in @("phytozome GO.app/Contents/Info.plist", "phytozome GO.app/Contents/MacOS/phytozome-go", "phytozome GO.app/Contents/MacOS/phytozome-go.bin", "phytozome GO.app/Contents/MacOS/phytozome-go-cleancache.bin", "phytozome GO.app/Contents/MacOS/wezterm", "phytozome GO.app/Contents/Resources/wezterm.lua")) {
            if (-not ($macEntries -contains $required)) {
                throw "macOS archive '$macArchive' is missing required file: $required"
            }
        }
    }

    Invoke-Checked "version check" {
        cmd /c bin\phytozome-go_windows_amd64_wezterm\phytozome-go.bin --version
    }

    Add-Type -AssemblyName System.Drawing
    $verifyDir = Join-Path $resolvedBin "verify-icons"
    New-Item -ItemType Directory -Force -Path $verifyDir | Out-Null
    foreach ($target in @(
        @{ Name = "launcher"; Path = "bin\phytozome-go_windows_amd64_wezterm\phytozome-go.exe"; Temp = $false },
        @{ Name = "window"; Path = "bin\phytozome-go_windows_amd64_wezterm\wezterm.bin"; Temp = $true }
    )) {
        $path = (Resolve-Path -LiteralPath $target.Path).Path
        $extractPath = $path
        if ($target.Temp) {
            $extractPath = Join-Path $env:TEMP "phytozome-go-window-icon-extract.exe"
            Copy-Item -LiteralPath $path -Destination $extractPath -Force
        }

        $icon = [System.Drawing.Icon]::ExtractAssociatedIcon($extractPath)
        if (-not $icon) {
            throw "Could not extract icon from $path"
        }
        try {
            $bitmap = $icon.ToBitmap()
            try {
                $bitmap.Save((Join-Path $verifyDir ($target.Name + "-icon.png")), [System.Drawing.Imaging.ImageFormat]::Png)
            } finally {
                $bitmap.Dispose()
            }
        } finally {
            $icon.Dispose()
            if ($target.Temp) {
                Remove-Item -LiteralPath $extractPath -Force -ErrorAction SilentlyContinue
            }
        }
    }

    foreach ($required in @($linuxArchivePath, $macIntelArchivePath, $macArmArchivePath)) {
        if (-not (Test-Path -LiteralPath $required -PathType Leaf)) {
            throw "Expected release archive is missing: $required"
        }
    }

    $assets = @(
        "bin\phytozome-go_windows_amd64_wezterm.zip",
        "bin\phytozome-go_linux_amd64_wezterm.tar.gz",
        "bin\phytozome-go_macos_amd64_wezterm.tar.gz",
        "bin\phytozome-go_macos_arm64_wezterm.tar.gz"
    )
    $hashLines = foreach ($asset in $assets) {
        $item = Get-Item -LiteralPath $asset
        $hash = (Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $($item.Name)"
    }
    $hashLines | Set-Content -LiteralPath "bin\SHA256SUMS.txt" -Encoding ASCII

    if ($Publish) {
        $dirty = git status --short
        if ($dirty) {
            throw "Refusing to publish from a dirty worktree. Commit or stash changes first."
        }

        $branch = (git branch --show-current).Trim()
        if ([string]::IsNullOrWhiteSpace($branch)) {
            throw "Could not determine the current git branch."
        }

        $existingTag = git tag --list $BuildVersion
        if (-not $existingTag) {
            Invoke-Checked "git tag $BuildVersion" {
                git tag -a $BuildVersion -m "Release $BuildVersion"
            }
        }
        Invoke-Checked "git push origin $branch" {
            git push origin $branch
        }
        Invoke-Checked "git push origin $BuildVersion" {
            git push origin $BuildVersion
        }

        if ([string]::IsNullOrWhiteSpace($ReleaseTitle)) {
            $ReleaseTitle = "phytozome GO $BuildVersion"
        }
        if ([string]::IsNullOrWhiteSpace($ReleaseNotes)) {
            $ReleaseNotes = @"
Release $BuildVersion

Validation:
- go test ./...
- go vet ./...
- go build ./...
- scripts\build-release.ps1

Assets:
- phytozome-go_windows_amd64_wezterm.zip
- phytozome-go_linux_amd64_wezterm.tar.gz
- phytozome-go_macos_amd64_wezterm.tar.gz
- phytozome-go_macos_arm64_wezterm.tar.gz
- SHA256SUMS.txt
"@
        }

        Invoke-Checked "GitHub release $BuildVersion" {
            gh release create $BuildVersion `
                bin\phytozome-go_windows_amd64_wezterm.zip `
                bin\phytozome-go_linux_amd64_wezterm.tar.gz `
                bin\phytozome-go_macos_amd64_wezterm.tar.gz `
                bin\phytozome-go_macos_arm64_wezterm.tar.gz `
                bin\SHA256SUMS.txt `
                --title $ReleaseTitle `
                --notes $ReleaseNotes
        }
    }

    Write-Host ""
    Write-Host "Release build complete: $BuildVersion"
    Write-Host "Artifacts:"
    Get-ChildItem -LiteralPath $resolvedBin -File | Where-Object { $_.Name -ne "release-tag.txt" } | Select-Object Name,Length | Format-Table -AutoSize
} finally {
    Pop-Location
}

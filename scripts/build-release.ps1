# The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
# you may not use this file except in compliance with the License. You may obtain a copy of the License at
# https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
# basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
# Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
# wangsychn. All Rights Reserved. Contributor(s): .

param(
    [string]$BuildVersion = "",
    [string]$WezTermVersion = "latest",
    [string]$WindowsWezTermVersion = "main",
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

function Get-DirtyWorktreeLines {
    return @(
        git status --short --untracked-files=all |
            Where-Object { -not [string]::IsNullOrWhiteSpace($_.Trim()) }
    )
}

$websiteRoot = "docs"
$websiteChangelogPath = "$websiteRoot/nac.html"

function Get-StatusPath {
    param(
        [string]$StatusLine
    )

    if ([string]::IsNullOrWhiteSpace($StatusLine)) {
        return ""
    }
    return (($StatusLine -replace '^[ MADRCU?!]{1,2}\s+', '').Trim())
}

function Test-WebsiteOnlyDirty {
    param(
        [string[]]$DirtyLines
    )

    if ($null -eq $DirtyLines -or $DirtyLines.Count -eq 0) {
        return $false
    }

    foreach ($line in $DirtyLines) {
        $path = Get-StatusPath $line
        if ([string]::IsNullOrWhiteSpace($path)) {
            return $false
        }
        if ($path -notmatch '^docs([\\/]|$)') {
            return $false
        }
    }
    return $true
}

function Commit-DirtyWebsiteIfAny {
    param(
        [string]$Label,
        [string]$CommitMessage
    )

    $dirty = Get-DirtyWorktreeLines
    if (-not (Test-WebsiteOnlyDirty $dirty)) {
        return $false
    }

    Invoke-Checked $Label {
        git add --all -- docs
        git commit -m $CommitMessage
    }
    return $true
}

function Assert-CleanWorktree {
    param(
        [string]$Message
    )

    $dirty = Get-DirtyWorktreeLines
    if ($dirty.Count -gt 0) {
        throw $Message
    }
}

Push-Location $repoRoot
try {
    if ($Publish) {
        Assert-CleanWorktree "Refusing to publish from a dirty worktree. Commit or stash changes first."
    }

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

Website:
- docs/nac.html updated with the latest release log at the top
"@
    }

    Invoke-Checked "Reactree tree viewer assets" {
        powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-tree-viewer.ps1
    }

    Invoke-Checked "Release notes page sync" {
        powershell -NoProfile -ExecutionPolicy Bypass -File scripts\sync-release-notes-page.ps1 -PendingTitle $ReleaseTitle -PendingBody $ReleaseNotes -PendingTag $BuildVersion
    }

    if ($Publish) {
        [void](Commit-DirtyWebsiteIfAny "Commit website changelog sync" "Sync website changelog for $BuildVersion")
    }

    Invoke-Checked "Windows WezTerm package" {
        powershell -NoProfile -ExecutionPolicy Bypass -File scripts\package-windows-wezterm.ps1 -Version $WindowsWezTermVersion -BuildVersion $BuildVersion
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
    foreach ($required in @("phytozome-go.exe", "core.bin", "phgohelper.bin", "wezterm.bin", "wezterm-cli.bin", "wezterm.lua", "mega-phgo-runtime.bin", "muscleWin64.bin")) {
        if (-not ($entries -contains $required)) {
            throw "Windows zip is missing required file: $required"
        }
    }
    if ($entries -contains "mesa/opengl32.dll") {
        throw "Windows zip must not package the legacy mesa directory."
    }
    if ($entries -contains "opengl32.dll") {
        throw "Windows zip must not package the Mesa software OpenGL fallback."
    }
    if (-not ($entries -contains "wezterm-source.json")) {
        throw "Windows zip must record the exact WezTerm source revision in wezterm-source.json."
    }
    $windowsConfigPath = Join-Path $resolvedBin "phytozome-go_windows_amd64_wezterm\wezterm.lua"
    $windowsConfig = Get-Content -LiteralPath $windowsConfigPath -Raw
    if ($windowsConfig -match '(?m)^\s*show_close_tab_button_in_tabs\s*=') {
        throw "Windows release contains unsupported WezTerm field show_close_tab_button_in_tabs."
    }
    foreach ($requiredConfig in @(
        'use_fancy_tab_bar = true',
        "window_decorations = is_windows and 'INTEGRATED_BUTTONS|RESIZE'",
        "integrated_title_button_style = 'Windows'",
        "integrated_title_buttons = { 'Hide', 'Maximize', 'Close' }"
    )) {
        if (-not $windowsConfig.Contains($requiredConfig)) {
            throw "Windows release is missing required window/tab setting: $requiredConfig"
        }
    }
    $rootExeEntries = @($entries | Where-Object { $_ -match '^[^/\\]+\.exe$' })
    if ($rootExeEntries.Count -ne 1 -or $rootExeEntries[0] -ne "phytozome-go.exe") {
        throw "Windows zip must contain exactly one root .exe (phytozome-go.exe). Found: $($rootExeEntries -join ', ')"
    }
    foreach ($forbidden in @("docs/logo.png", "docs/logo2.png", "docs/logo3large.png", "docs/logo3small.png", "logo.png", "logo2.png", "logo3large.png", "logo3small.png", "phytozome-go-window-icon.png")) {
        if ($entries -contains $forbidden) {
            throw "Windows zip must not package logo image file: $forbidden"
        }
    }

    $linuxEntries = @(tar -tf $linuxArchivePath)
    foreach ($required in @("phytozome-go_linux_amd64_wezterm/phytozome-go", "phytozome-go_linux_amd64_wezterm/core.bin", "phytozome-go_linux_amd64_wezterm/phgohelper.bin", "phytozome-go_linux_amd64_wezterm/wezterm", "phytozome-go_linux_amd64_wezterm/wezterm.AppImage", "phytozome-go_linux_amd64_wezterm/wezterm.lua")) {
        if (-not ($linuxEntries -contains $required)) {
            throw "Linux archive is missing required file: $required"
        }
    }

    foreach ($macArchive in @($macIntelArchivePath, $macArmArchivePath)) {
        $macEntries = @(tar -tf $macArchive)
        foreach ($required in @("phytozome GO.app/Contents/Info.plist", "phytozome GO.app/Contents/MacOS/phytozome-go", "phytozome GO.app/Contents/MacOS/core.bin", "phytozome GO.app/Contents/MacOS/phgohelper.bin", "phytozome GO.app/Contents/MacOS/wezterm", "phytozome GO.app/Contents/Resources/wezterm.lua")) {
            if (-not ($macEntries -contains $required)) {
                throw "macOS archive '$macArchive' is missing required file: $required"
            }
        }
    }

    Invoke-Checked "version check" {
        cmd /c bin\phytozome-go_windows_amd64_wezterm\core.bin --version
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
        if (-not (Commit-DirtyWebsiteIfAny "Commit website pages after packaging" "Update website pages for $BuildVersion")) {
            Assert-CleanWorktree "Refusing to publish because the worktree changed after packaging. Commit or stash changes first."
        }
        Assert-CleanWorktree "Refusing to publish because the worktree changed after packaging. Commit or stash changes first."

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

        Invoke-Checked "GitHub release $BuildVersion" {
            $releaseAssets = @(
                "bin\phytozome-go_windows_amd64_wezterm.zip",
                "bin\phytozome-go_linux_amd64_wezterm.tar.gz",
                "bin\phytozome-go_macos_amd64_wezterm.tar.gz",
                "bin\phytozome-go_macos_arm64_wezterm.tar.gz",
                "bin\SHA256SUMS.txt"
            )
            gh release create $BuildVersion `
                @releaseAssets `
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

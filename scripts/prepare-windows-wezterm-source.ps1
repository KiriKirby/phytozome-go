# The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
# you may not use this file except in compliance with the License. You may obtain a copy of the License at
# https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
# basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
# Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
# wangsychn. All Rights Reserved. Contributor(s): .

param(
    [string]$Revision = "main",
    [switch]$Force
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "windows-wezterm-common.ps1")

function Invoke-Checked {
    param([string]$Label, [scriptblock]$Action)
    & $Action
    if ($LASTEXITCODE -ne 0) {
        throw "$Label failed with exit code $LASTEXITCODE"
    }
}

function Resolve-CargoPath {
    $candidate = Join-Path ([Environment]::GetFolderPath("UserProfile")) ".cargo\bin\cargo.exe"
    if (Test-Path -LiteralPath $candidate -PathType Leaf) {
        return $candidate
    }
    $command = Get-Command cargo.exe -ErrorAction SilentlyContinue
    if ($command) {
        return $command.Source
    }
    throw "Rust cargo was not found. Install rustup with the stable x86_64-pc-windows-msvc toolchain before building WezTerm from source."
}

function Import-VisualStudioBuildEnvironment {
    $vswhere = Join-Path ${env:ProgramFiles(x86)} "Microsoft Visual Studio\Installer\vswhere.exe"
    if (-not (Test-Path -LiteralPath $vswhere -PathType Leaf)) {
        throw "Visual Studio Build Tools were not found. Install the C++ build workload before building WezTerm from source."
    }
    $installation = (& $vswhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath | Select-Object -First 1)
    if ([string]::IsNullOrWhiteSpace($installation)) {
        throw "Visual Studio C++ Build Tools were not found. Install Microsoft.VisualStudio.Workload.VCTools."
    }
    $devCmd = Join-Path $installation "Common7\Tools\VsDevCmd.bat"
    if (-not (Test-Path -LiteralPath $devCmd -PathType Leaf)) {
        throw "Visual Studio developer environment script is missing: $devCmd"
    }
    $environmentLines = & $env:ComSpec /d /s /c ('"' + $devCmd + '" -no_logo -arch=amd64 -host_arch=amd64 >nul && set')
    if ($LASTEXITCODE -ne 0) {
        throw "Visual Studio developer environment initialization failed with exit code $LASTEXITCODE"
    }
    foreach ($line in $environmentLines) {
        $separator = $line.IndexOf('=')
        if ($separator -le 0) {
            continue
        }
        $name = $line.Substring(0, $separator)
        $value = $line.Substring($separator + 1)
        [Environment]::SetEnvironmentVariable($name, $value, [EnvironmentVariableTarget]::Process)
    }
    if (-not (Get-Command cl.exe -ErrorAction SilentlyContinue)) {
        throw "Visual Studio environment loaded but cl.exe is unavailable."
    }
}

$repoRoot = Get-PhytozomeRepoRoot
$cacheOverride = [Environment]::GetEnvironmentVariable("PHGO_WEZTERM_SOURCE_BUILD_CACHE")
if ([string]::IsNullOrWhiteSpace($cacheOverride)) {
    $localAppData = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
    if ([string]::IsNullOrWhiteSpace($localAppData)) {
        throw "LocalApplicationData is unavailable. Set PHGO_WEZTERM_SOURCE_BUILD_CACHE to a persistent build-cache directory."
    }
    # Do not put this under the repository's .cache directory. Packaged and
    # development startup intentionally clean app-local .cache trees, and a
    # WezTerm Release/LTO build is too expensive to lose between test/release
    # packaging runs.
    $cacheRoot = Join-Path $localAppData "phytozome-go\build-cache\wezterm\windows-source"
} else {
    $cacheRoot = [System.IO.Path]::GetFullPath($cacheOverride)
}
$sourceDir = Join-Path $cacheRoot "wezterm"
$lockPath = Join-Path $cacheRoot "prepare-source.lock"
New-Item -ItemType Directory -Force -Path $cacheRoot | Out-Null

$lockStream = $null
try {
    $deadline = (Get-Date).AddMinutes(5)
    while ($true) {
        try {
            $lockStream = [System.IO.File]::Open($lockPath, [System.IO.FileMode]::OpenOrCreate, [System.IO.FileAccess]::ReadWrite, [System.IO.FileShare]::None)
            break
        } catch {
            if ((Get-Date) -ge $deadline) {
                throw "Timed out waiting for the WezTerm source preparation lock: $lockPath"
            }
            Start-Sleep -Milliseconds 500
        }
    }

    if (-not (Test-Path -LiteralPath (Join-Path $sourceDir ".git") -PathType Container)) {
        Invoke-Checked "clone WezTerm source" {
            git clone --filter=blob:none https://github.com/wezterm/wezterm.git $sourceDir
        }
    }

    Push-Location $sourceDir
    try {
        Invoke-Checked "fetch WezTerm revision" { git fetch --prune origin $Revision }
        Invoke-Checked "checkout WezTerm revision" { git checkout --detach FETCH_HEAD }
        $commit = (git rev-parse HEAD).Trim()
    } finally {
        Pop-Location
    }

    $preparedDir = Join-Path $cacheRoot ("prepared-" + $commit.Substring(0, 12))
    $metadataPath = Join-Path $preparedDir "wezterm-source.json"
    $requiredPreparedFiles = @(
        "wezterm.bin",
        "wezterm-cli.bin",
        "wezterm-mux-server.bin",
        "openconsole.bin",
        "conpty.dll",
        "wezterm-source.json"
    )
    if (-not $Force) {
        $complete = $true
        foreach ($name in $requiredPreparedFiles) {
            $item = Get-Item -LiteralPath (Join-Path $preparedDir $name) -ErrorAction SilentlyContinue
            if (-not $item -or $item.Length -le 0) {
                $complete = $false
                break
            }
        }
        if ($complete) {
            try {
                $metadata = Get-Content -LiteralPath $metadataPath -Raw | ConvertFrom-Json
                $complete = $metadata.commit -eq $commit
            } catch {
                $complete = $false
            }
        }
        if ($complete) {
            Write-Host "Reusing cached WezTerm source build for commit $commit at $preparedDir"
            Write-Output $preparedDir
            return
        }
    }

    Push-Location $sourceDir
    try {
        Invoke-Checked "update WezTerm submodules" { git submodule update --init --recursive --depth 1 --jobs ([Environment]::ProcessorCount) }
    } finally {
        Pop-Location
    }

    $cargo = Resolve-CargoPath
    Import-VisualStudioBuildEnvironment
    $portablePerl = Join-Path $repoRoot ".cache\build\tools\strawberry-perl\perl\bin"
    $systemPerl = "C:\Strawberry\perl\bin"
    $strawberryPerl = if (Test-Path -LiteralPath $portablePerl -PathType Container) { $portablePerl } else { $systemPerl }
    if (-not (Test-Path -LiteralPath (Join-Path $strawberryPerl "perl.exe") -PathType Leaf)) {
        throw "Native Strawberry Perl was not found. WezTerm's vendored OpenSSL build requires a Windows Perl implementation (osname=MSWin32), not MSYS/Cygwin Perl."
    }
    $env:Path = $strawberryPerl + [IO.Path]::PathSeparator + $env:Path
    # The hosting shell may export a Unix-only C.UTF-8 locale. Native Perl
    # correctly uses the Windows system locale when these are absent.
    Remove-Item Env:\LC_ALL -ErrorAction SilentlyContinue
    Remove-Item Env:\LC_CTYPE -ErrorAction SilentlyContinue
    Remove-Item Env:\LANG -ErrorAction SilentlyContinue
    $perlOS = (& (Join-Path $strawberryPerl "perl.exe") -V:osname) -join ""
    if ($perlOS -notmatch "MSWin32") {
        throw "The selected Perl is not a native Windows build: $perlOS"
    }
    $oldIncremental = $env:CARGO_INCREMENTAL
    $oldJobs = $env:CARGO_BUILD_JOBS
    $oldLTO = $env:CARGO_PROFILE_RELEASE_LTO
    $oldCodegen = $env:CARGO_PROFILE_RELEASE_CODEGEN_UNITS
    $oldStrip = $env:CARGO_PROFILE_RELEASE_STRIP
    try {
        # Cargo/cc already parallelize independent crates and native objects.
        # ProcessorCount is the fastest safe default on Windows without
        # oversubscribing memory-heavy LLVM link jobs.
        $env:CARGO_INCREMENTAL = "0"
        $env:CARGO_BUILD_JOBS = [string][Environment]::ProcessorCount
        $env:CARGO_PROFILE_RELEASE_LTO = "thin"
        $env:CARGO_PROFILE_RELEASE_CODEGEN_UNITS = "1"
        $env:CARGO_PROFILE_RELEASE_STRIP = "symbols"
        Push-Location $sourceDir
        try {
            Invoke-Checked "build latest WezTerm source" {
                & $cargo build --locked --release -p wezterm -p wezterm-gui -p wezterm-mux-server
            }
        } finally {
            Pop-Location
        }
    } finally {
        $env:CARGO_INCREMENTAL = $oldIncremental
        $env:CARGO_BUILD_JOBS = $oldJobs
        $env:CARGO_PROFILE_RELEASE_LTO = $oldLTO
        $env:CARGO_PROFILE_RELEASE_CODEGEN_UNITS = $oldCodegen
        $env:CARGO_PROFILE_RELEASE_STRIP = $oldStrip
    }

    Remove-Item -LiteralPath $preparedDir -Recurse -Force -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Force -Path $preparedDir | Out-Null
    $releaseDir = Join-Path $sourceDir "target\release"
    Copy-Item -LiteralPath (Join-Path $releaseDir "wezterm-gui.exe") -Destination (Join-Path $preparedDir "wezterm.bin") -Force
    Copy-Item -LiteralPath (Join-Path $releaseDir "wezterm.exe") -Destination (Join-Path $preparedDir "wezterm-cli.bin") -Force
    Copy-Item -LiteralPath (Join-Path $releaseDir "wezterm-mux-server.exe") -Destination (Join-Path $preparedDir "wezterm-mux-server.bin") -Force
    Copy-Item -LiteralPath (Join-Path $sourceDir "assets\windows\conhost\OpenConsole.exe") -Destination (Join-Path $preparedDir "openconsole.bin") -Force
    Copy-Item -LiteralPath (Join-Path $sourceDir "assets\windows\conhost\conpty.dll") -Destination (Join-Path $preparedDir "conpty.dll") -Force

    [ordered]@{
        repository = "https://github.com/wezterm/wezterm"
        requested_revision = $Revision
        commit = $commit
        built_utc = [DateTime]::UtcNow.ToString("o")
        cargo_jobs = [Environment]::ProcessorCount
        profile = "release; opt-level=3; thin-lto; codegen-units=1; stripped"
    } | ConvertTo-Json | Set-Content -LiteralPath $metadataPath -Encoding UTF8
    Write-Output $preparedDir
} finally {
    if ($lockStream) {
        $lockStream.Dispose()
    }
}

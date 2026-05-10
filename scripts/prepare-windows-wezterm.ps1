param(
    [string]$Version = "latest",
    [switch]$Force
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "windows-wezterm-common.ps1")

$repoRoot = Get-PhytozomeRepoRoot
$cacheRoot = Get-WindowsWezTermCacheRoot $repoRoot
$release = Resolve-WezTermWindowsRelease $Version
$preparedDir = Get-PreparedWindowsWezTermDir $repoRoot $release.Tag
$downloadZip = Join-Path $cacheRoot $release.ZipName
$extractDir = Join-Path $cacheRoot ("extract-" + [IO.Path]::GetFileNameWithoutExtension($release.ZipName))

if ($Force) {
    Remove-Item -LiteralPath $preparedDir -Recurse -Force -ErrorAction SilentlyContinue
}

New-Item -ItemType Directory -Force -Path $cacheRoot | Out-Null

if (-not (Test-Path -LiteralPath $downloadZip -PathType Leaf)) {
    Invoke-WebRequest -Uri $release.URL -OutFile $downloadZip
}

if ($Force -or -not (Test-Path -LiteralPath $extractDir -PathType Container)) {
    Remove-Item -LiteralPath $extractDir -Recurse -Force -ErrorAction SilentlyContinue
    Expand-Archive -LiteralPath $downloadZip -DestinationPath $extractDir -Force
}

$wezRoot = Get-ChildItem -LiteralPath $extractDir -Directory | Select-Object -First 1
if (-not $wezRoot) {
    throw "Could not find extracted WezTerm directory in: $extractDir"
}

New-Item -ItemType Directory -Force -Path $preparedDir | Out-Null
Copy-WezTermRuntimeFiles -WezRoot $wezRoot.FullName -Destination $preparedDir
Write-PhytozomeWezTermConfig -Path (Join-Path $preparedDir "wezterm.lua")

Push-Location $repoRoot
try {
    go build -trimpath -ldflags="-H=windowsgui -X main.version=dev" -o (Join-Path $preparedDir "phytozome-go.exe") .\cmd\phytozome-go-winlauncher
} finally {
    Pop-Location
}

Write-Host "Prepared Windows WezTerm runtime:"
Write-Host "  $preparedDir"
Write-Host "Release:"
Write-Host "  $($release.Tag)"

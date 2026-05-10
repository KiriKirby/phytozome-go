param(
    [string]$Version = "latest"
)

$ErrorActionPreference = "Stop"
& (Join-Path $PSScriptRoot "package-windows-wezterm.ps1") -Version $Version -BuildVersion "dev" -SkipZip

# The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
# you may not use this file except in compliance with the License. You may obtain a copy of the License at
# https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
# basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
# Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
# wangsychn. All Rights Reserved. Contributor(s): .

param(
    [string]$Source = "docs\logo2.png"
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$sourcePath = Join-Path $repoRoot $Source
$launcherDir = Join-Path $repoRoot "cmd\phytozome-go-winlauncher"
$iconPath = Join-Path $launcherDir "phytozome-go.ico"
$sysoPath = Join-Path $launcherDir "rsrc_windows_amd64.syso"

if (-not (Test-Path -LiteralPath $sourcePath -PathType Leaf)) {
    throw "Icon source not found: $sourcePath"
}

Add-Type -AssemblyName System.Drawing

function New-IconPngBytes {
    param(
        [System.Drawing.Image]$SourceImage,
        [int]$Size
    )

    $bitmap = New-Object System.Drawing.Bitmap $Size, $Size, ([System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
    try {
        $graphics.Clear([System.Drawing.Color]::Transparent)
        $graphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        $graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::HighQuality
        $graphics.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
        $graphics.CompositingQuality = [System.Drawing.Drawing2D.CompositingQuality]::HighQuality
        $graphics.DrawImage($SourceImage, 0, 0, $Size, $Size)

        $stream = New-Object System.IO.MemoryStream
        try {
            $bitmap.Save($stream, [System.Drawing.Imaging.ImageFormat]::Png)
            return $stream.ToArray()
        } finally {
            $stream.Dispose()
        }
    } finally {
        $graphics.Dispose()
        $bitmap.Dispose()
    }
}

function Write-UInt16LE {
    param(
        [System.IO.BinaryWriter]$Writer,
        [int]$Value
    )
    $Writer.Write([uint16]$Value)
}

function Write-UInt32LE {
    param(
        [System.IO.BinaryWriter]$Writer,
        [int]$Value
    )
    $Writer.Write([uint32]$Value)
}

$sizes = @(16, 24, 32, 48, 64, 128, 256)
$image = [System.Drawing.Image]::FromFile($sourcePath)
try {
    $entries = foreach ($size in $sizes) {
        [pscustomobject]@{
            Size = $size
            Bytes = New-IconPngBytes -SourceImage $image -Size $size
        }
    }
} finally {
    $image.Dispose()
}

$iconStream = New-Object System.IO.MemoryStream
$writer = New-Object System.IO.BinaryWriter $iconStream
try {
    Write-UInt16LE $writer 0
    Write-UInt16LE $writer 1
    Write-UInt16LE $writer $entries.Count

    $offset = 6 + (16 * $entries.Count)
    foreach ($entry in $entries) {
        $dimension = if ($entry.Size -eq 256) { 0 } else { $entry.Size }
        $writer.Write([byte]$dimension)
        $writer.Write([byte]$dimension)
        $writer.Write([byte]0)
        $writer.Write([byte]0)
        Write-UInt16LE $writer 1
        Write-UInt16LE $writer 32
        Write-UInt32LE $writer $entry.Bytes.Length
        Write-UInt32LE $writer $offset
        $offset += $entry.Bytes.Length
    }

    foreach ($entry in $entries) {
        $writer.Write([byte[]]$entry.Bytes)
    }
    [System.IO.File]::WriteAllBytes($iconPath, $iconStream.ToArray())
} finally {
    $writer.Dispose()
    $iconStream.Dispose()
}

& rsrc -arch amd64 -ico $iconPath -o $sysoPath

Write-Host "Icon written to: $iconPath"
Write-Host "Resource written to: $sysoPath"

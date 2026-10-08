# The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
# you may not use this file except in compliance with the License. You may obtain a copy of the License at
# https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
# basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
# Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
# wangsychn. All Rights Reserved. Contributor(s): .

param(
    [string]$PendingTitle = "",
    [string]$PendingBody = "",
    [string]$PendingTag = ""
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$targetPath = Join-Path $repoRoot "docs\nac.html"
$metaPath = Join-Path $repoRoot "docs\_vti_cnf\nac.html"

if (-not (Test-Path -LiteralPath $targetPath -PathType Leaf)) {
    throw "Release notes page not found: $targetPath"
}

$releaseJson = gh api repos/KiriKirby/phytozome-go/releases --paginate
$releases = $releaseJson | ConvertFrom-Json

function Normalize-Text {
    param([string]$Value)
    if ($null -eq $Value) {
        return ""
    }
    return ($Value -replace "`r`n", "`n").Trim()
}

function Encode-Html {
    param([string]$Value)
    return [System.Net.WebUtility]::HtmlEncode($Value)
}

$bodyLines = New-Object System.Collections.Generic.List[string]
$entries = New-Object System.Collections.Generic.List[object]
if (-not [string]::IsNullOrWhiteSpace((Normalize-Text $PendingTitle))) {
    $entries.Add([pscustomobject]@{
        name = Normalize-Text $PendingTitle
        body = Normalize-Text $PendingBody
        tag_name = Normalize-Text $PendingTag
        published_at = (Get-Date).ToUniversalTime().ToString("o")
        draft = $false
    })
}
foreach ($release in ($releases | Where-Object { -not $_.draft } | Sort-Object {
    if ($_.published_at) { [datetime]$_.published_at } else { [datetime]$_.created_at }
} -Descending)) {
    if (-not [string]::IsNullOrWhiteSpace($PendingTag) -and ((Normalize-Text $release.tag_name) -eq (Normalize-Text $PendingTag))) {
        continue
    }
    $entries.Add($release)
}

foreach ($release in $entries) {
    $title = Normalize-Text $release.name
    if ([string]::IsNullOrWhiteSpace($title)) {
        $title = Normalize-Text $release.tag_name
    }
    $body = Normalize-Text $release.body

    $bodyLines.Add("<p>$(Encode-Html $title)</p>")
    if (-not [string]::IsNullOrWhiteSpace($body)) {
        foreach ($line in ($body -split "`n")) {
            if ([string]::IsNullOrWhiteSpace($line)) {
                continue
            }
            $bodyLines.Add("<p>$(Encode-Html $line)</p>")
        }
    }
    $bodyLines.Add("<p>&nbsp;</p>")
}

$content = @(
    "<html>",
    "",
    "<head>",
    "<meta http-equiv=""Content-Type"" content=""text/html; charset=utf-8"">",
    "<title>PHgo - Release Notes</title>",
    "</head>",
    "",
    "<body bgcolor=""#FFFFFF"">",
    ""
) + $bodyLines + @(
    "</body>",
    "",
    "</html>",
    ""
)

Set-Content -LiteralPath $targetPath -Value $content -Encoding UTF8
if (Test-Path -LiteralPath $metaPath -PathType Leaf) {
    $meta = Get-Content -LiteralPath $metaPath
    $updated = foreach ($line in $meta) {
        if ($line -like "vti_filesize:IR|*") {
            "vti_filesize:IR|$((Get-Item -LiteralPath $targetPath).Length)"
        } elseif ($line -like "vti_cachedtitle:SR|*") {
            "vti_cachedtitle:SR|PHgo - Release Notes"
        } elseif ($line -like "vti_title:SR|*") {
            "vti_title:SR|PHgo - Release Notes"
        } else {
            $line
        }
    }
    Set-Content -LiteralPath $metaPath -Value $updated -Encoding UTF8
}
Write-Host "Release notes page updated:"
Write-Host "  $targetPath"

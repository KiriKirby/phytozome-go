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

$forward = @{
    WezTermVersion = $WezTermVersion
}
if (-not [string]::IsNullOrWhiteSpace($BuildVersion)) {
    $forward.BuildVersion = $BuildVersion
}
if ($SkipTests) {
    $forward.SkipTests = $true
}
if ($SkipVet) {
    $forward.SkipVet = $true
}
if ($SkipBuildCheck) {
    $forward.SkipBuildCheck = $true
}
if ($Publish) {
    $forward.Publish = $true
}
if (-not [string]::IsNullOrWhiteSpace($ReleaseTitle)) {
    $forward.ReleaseTitle = $ReleaseTitle
}
if (-not [string]::IsNullOrWhiteSpace($ReleaseNotes)) {
    $forward.ReleaseNotes = $ReleaseNotes
}

& (Join-Path $PSScriptRoot "build-release.ps1") @forward

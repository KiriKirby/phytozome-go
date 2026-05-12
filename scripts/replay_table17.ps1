# The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
# you may not use this file except in compliance with the License. You may obtain a copy of the License at
# https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
# basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
# Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
# wangsychn. All Rights Reserved. Contributor(s): .

$ErrorActionPreference = "Stop"

$repo = "C:\Users\wangsychn\Documents\GitHub\phytozome-batch-cli"
$out = "C:\Users\wangsychn\Documents\GitHub\phytozome-batch-cli\.codex\replay"
New-Item -ItemType Directory -Force -Path $out | Out-Null

$monolignol = "C:\Users\wangsychn\Desktop\新建文件夹\Monolignol Biosynthesis.txt"
$cellulose = "C:\Users\wangsychn\Desktop\新建文件夹\Cellulose.txt"
$hemi = "C:\Users\wangsychn\Desktop\新建文件夹\Hemicelluloses.txt"

Write-Host "Replay staging only:"
Write-Host "  repo: $repo"
Write-Host "  out:  $out"
Write-Host "  Monolignol: $monolignol"
Write-Host "  Cellulose:  $cellulose"
Write-Host "  Hemicelluloses: $hemi"
Write-Host ""
Write-Host "The programmatic replay harness is being added in code; this script just pins the paths for repeatable runs."

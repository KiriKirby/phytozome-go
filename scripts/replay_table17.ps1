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

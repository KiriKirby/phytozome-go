$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$exePath = Join-Path $repoRoot "bin\phytozome-go-debug.exe"

New-Item -ItemType Directory -Force -Path (Split-Path -Parent $exePath) | Out-Null

Push-Location $repoRoot
try {
    go build -o $exePath .\cmd\phytozome-go
} finally {
    Pop-Location
}

$appArgs = @("blast", "wizard")

$quotedExe = '"' + $exePath + '"'
$quotedArgs = ($appArgs | ForEach-Object { '"' + ($_ -replace '"', '\"') + '"' }) -join " "
$command = "cd /d `"$repoRoot`" && $quotedExe $quotedArgs"

# This opens a real external Windows console host according to the user's
# Windows default terminal setting. Set Windows Terminal > Default terminal
# application to "Windows Console Host" when testing classic conhost.exe.
Start-Process -FilePath "cmd.exe" -ArgumentList "/k", $command

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$exePath = Join-Path $repoRoot "bin\phytozome-go-dev.exe"

New-Item -ItemType Directory -Force -Path (Split-Path -Parent $exePath) | Out-Null

Push-Location $repoRoot
try {
    go build -trimpath -ldflags="-X main.version=dev" -o $exePath .\cmd\phytozome-go
} finally {
    Pop-Location
}

$quotedExe = '"' + $exePath + '"'
$command = "cd /d `"$repoRoot`" && $quotedExe"

# This opens a real external Windows console host according to the user's
# Windows default terminal setting. Set Windows Terminal > Default terminal
# application to "Windows Console Host" when testing classic conhost.exe.
Start-Process -FilePath "cmd.exe" -ArgumentList "/k", $command

. (Join-Path $PSScriptRoot "wezterm-common.ps1")

function Copy-WezTermRuntimeFiles {
    param(
        [Parameter(Mandatory = $true)]
        [string]$WezRoot,
        [Parameter(Mandatory = $true)]
        [string]$Destination
    )

    New-Item -ItemType Directory -Force -Path $Destination | Out-Null

    $entries = Get-ChildItem -LiteralPath $WezRoot -Force
    foreach ($entry in $entries) {
        if ($entry.PSIsContainer) {
            if ($entry.Name -ieq "mesa") {
                Copy-Item -LiteralPath $entry.FullName -Destination (Join-Path $Destination $entry.Name) -Recurse -Force
            }
            continue
        }

        $targetName = switch -Regex ($entry.Name.ToLowerInvariant()) {
            '^wezterm-gui\.exe$' { 'wezterm.bin'; break }
            '^wezterm-cli\.exe$' { 'wezterm-cli.bin'; break }
            '^wezterm-mux-server\.exe$' { 'wezterm-mux-server.bin'; break }
            '^openconsole\.exe$' { 'openconsole.bin'; break }
            '\.dll$' { $entry.Name; break }
            default { $null }
        }

        if ([string]::IsNullOrWhiteSpace($targetName)) {
            continue
        }

        Copy-Item -LiteralPath $entry.FullName -Destination (Join-Path $Destination $targetName) -Force
    }
}

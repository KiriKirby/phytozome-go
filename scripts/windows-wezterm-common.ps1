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
            # PHgo requires the hardware WebGPU renderer. Do not flatten the
            # bundled Mesa opengl32.dll beside wezterm-gui: if WebGPU cannot be
            # created, silently falling back to CPU OpenGL makes a large
            # terminal repaint one scan line at a time.
            continue
        }

        $targetName = switch -Regex ($entry.Name.ToLowerInvariant()) {
            '^wezterm-gui\.exe$' { 'wezterm.bin'; break }
            '^wezterm\.exe$' { 'wezterm-cli.bin'; break }
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

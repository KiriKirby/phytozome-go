# The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
# you may not use this file except in compliance with the License. You may obtain a copy of the License at
# https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
# basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
# Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
# wangsychn. All Rights Reserved. Contributor(s): .

$ErrorActionPreference = "Stop"

function Get-PhytozomeRepoRoot {
    return (Split-Path -Parent $PSScriptRoot)
}

function Get-WindowsWezTermCacheRoot {
    param([string]$RepoRoot)
    return (Join-Path $RepoRoot "bin\tooling\windows-wezterm")
}

function Resolve-WezTermWindowsRelease {
    param([string]$Version = "latest")

    if ($Version -eq "latest") {
        $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/wezterm/wezterm/releases/latest"
        $tag = [string]$latest.tag_name
        $asset = $latest.assets | Where-Object { $_.name -eq "WezTerm-windows-$tag.zip" } | Select-Object -First 1
        if (-not $asset) {
            throw "Could not find WezTerm Windows zip asset for release $tag"
        }
        return [pscustomobject]@{
            Tag = $tag
            ZipName = [string]$asset.name
            URL = [string]$asset.browser_download_url
        }
    }

    $tag = $Version
    return [pscustomobject]@{
        Tag = $tag
        ZipName = "WezTerm-windows-$tag.zip"
        URL = "https://github.com/wezterm/wezterm/releases/download/$tag/WezTerm-windows-$tag.zip"
    }
}

function Write-PhytozomeWezTermConfig {
    param([string]$Path)

    @"
local wezterm = require 'wezterm'
local act = wezterm.action

return {
  enable_tab_bar = false,
  window_close_confirmation = 'NeverPrompt',
  automatically_reload_config = false,
  default_prog = { './phytozome-go.bin' },

  front_end = 'WebGpu',
  webgpu_power_preference = 'HighPerformance',

  font_size = 9.0,
  initial_cols = 120,
  initial_rows = 34,
  adjust_window_size_when_changing_font_size = false,
  max_fps = 240,
  animation_fps = 1,
  cursor_blink_rate = 0,
  text_blink_rate = 0,
  text_blink_rate_rapid = 0,
  warn_about_missing_glyphs = false,
  check_for_updates = false,
  show_update_window = false,
  use_fancy_tab_bar = false,
  show_tabs_in_tab_bar = false,

  enable_scroll_bar = false,
  scrollback_lines = 5000,
  scroll_to_bottom_on_input = true,
  alternate_buffer_wheel_scroll_speed = 1,
  mouse_wheel_scrolls_tabs = false,
  bypass_mouse_reporting_modifiers = 'CTRL',

  mouse_bindings = {
    {
      event = { Up = { streak = 1, button = 'Left' } },
      mods = 'NONE',
      action = act.OpenLinkAtMouseCursor,
    },
    {
      event = { Up = { streak = 1, button = 'Left' } },
      mods = 'CTRL',
      action = act.OpenLinkAtMouseCursor,
    },
  },
}
"@ | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Copy-WezTermRuntimeFiles {
    param(
        [string]$WezRoot,
        [string]$Destination
    )

    $runtimeFiles = @{
        "wezterm-gui.exe" = "wezterm.bin"
        "wezterm-mux-server.exe" = "wezterm-mux-server.bin"
        "OpenConsole.exe" = "openconsole.bin"
    }

    foreach ($entry in $runtimeFiles.GetEnumerator()) {
        $source = Join-Path $WezRoot $entry.Key
        if (Test-Path -LiteralPath $source -PathType Leaf) {
            Copy-Item -LiteralPath $source -Destination (Join-Path $Destination $entry.Value) -Force
        }
    }

    Get-ChildItem -LiteralPath $WezRoot -File | Where-Object { $_.Extension -eq ".dll" } | ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $Destination $_.Name) -Force
    }

    $mesaDir = Join-Path $WezRoot "mesa"
    if (Test-Path -LiteralPath $mesaDir -PathType Container) {
        Copy-Item -LiteralPath $mesaDir -Destination (Join-Path $Destination "mesa") -Recurse -Force
    }
}

function Get-PreparedWindowsWezTermDir {
    param(
        [string]$RepoRoot,
        [string]$Tag
    )

    return (Join-Path (Get-WindowsWezTermCacheRoot $RepoRoot) "prepared-$Tag")
}

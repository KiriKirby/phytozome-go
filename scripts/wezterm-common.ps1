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

function Get-LinuxWezTermCacheRoot {
    param([string]$RepoRoot)
    return (Join-Path $RepoRoot "bin\tooling\linux-wezterm")
}

function Get-MacOSWezTermCacheRoot {
    param([string]$RepoRoot)
    return (Join-Path $RepoRoot "bin\tooling\macos-wezterm")
}

function Get-PreparedWindowsWezTermDir {
    param(
        [string]$RepoRoot,
        [string]$Tag
    )
    return (Join-Path (Get-WindowsWezTermCacheRoot $RepoRoot) "prepared-$Tag")
}

function Resolve-WezTermWindowsRelease {
    param([string]$Version = "latest")
    if ($Version -eq "latest") {
        $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/wezterm/wezterm/releases/latest"
        $tag = [string]$latest.tag_name
        $asset = $latest.assets | Where-Object { $_.name -eq "WezTerm-windows-$tag.zip" } | Select-Object -First 1
        if (-not $asset) { throw "Could not find WezTerm Windows zip asset for release $tag" }
        return [pscustomobject]@{ Tag = $tag; ZipName = [string]$asset.name; URL = [string]$asset.browser_download_url }
    }
    $tag = $Version
    return [pscustomobject]@{ Tag = $tag; ZipName = "WezTerm-windows-$tag.zip"; URL = "https://github.com/wezterm/wezterm/releases/download/$tag/WezTerm-windows-$tag.zip" }
}

function Resolve-WezTermLinuxRelease {
    param([string]$Version = "latest")
    if ($Version -eq "latest") {
        $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/wezterm/wezterm/releases/latest"
        $tag = [string]$latest.tag_name
        $asset = $latest.assets | Where-Object { $_.name -eq "WezTerm-$tag-Ubuntu20.04.AppImage" } | Select-Object -First 1
        if (-not $asset) { throw "Could not find WezTerm Linux AppImage asset for release $tag" }
        return [pscustomobject]@{ Tag = $tag; Name = [string]$asset.name; URL = [string]$asset.browser_download_url }
    }
    $tag = $Version
    return [pscustomobject]@{ Tag = $tag; Name = "WezTerm-$tag-Ubuntu20.04.AppImage"; URL = "https://github.com/wezterm/wezterm/releases/download/$tag/WezTerm-$tag-Ubuntu20.04.AppImage" }
}

function Resolve-WezTermMacOSRelease {
    param([string]$Version = "latest")
    if ($Version -eq "latest") {
        $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/wezterm/wezterm/releases/latest"
        $tag = [string]$latest.tag_name
        $asset = $latest.assets | Where-Object { $_.name -eq "WezTerm-macos-$tag.zip" } | Select-Object -First 1
        if (-not $asset) { throw "Could not find WezTerm macOS zip asset for release $tag" }
        return [pscustomobject]@{ Tag = $tag; Name = [string]$asset.name; URL = [string]$asset.browser_download_url }
    }
    $tag = $Version
    return [pscustomobject]@{ Tag = $tag; Name = "WezTerm-macos-$tag.zip"; URL = "https://github.com/wezterm/wezterm/releases/download/$tag/WezTerm-macos-$tag.zip" }
}

function Write-PhytozomeWezTermConfig {
    param(
        [string]$Path,
        [string]$Version = "dev"
    )

    $safeVersion = ($Version -replace "'", "''")

    @"
local wezterm = require 'wezterm'
local act = wezterm.action
local mux = wezterm.mux
local is_windows = string.find(wezterm.target_triple or '', 'windows', 1, true) ~= nil
local clean_cache_prog = './phytozome-go-cleancache.bin'
local window_title = 'PHgo ($safeVersion)'
local title_prefix = '__PHGO__|'

local function normalize_prog(value)
  if value == nil then
    return ''
  end
  value = tostring(value)
  value = string.lower(value)
  value = string.gsub(value, '\\', '/')
  return value
end

local function command_targets_main(cmd)
  if cmd == nil or cmd.args == nil then
    return false
  end
  for _, value in ipairs(cmd.args) do
    local normalized = normalize_prog(value)
    if normalized == './phytozome-go.bin' or normalized == 'phytozome-go.bin' then
      return true
    end
  end
  return false
end

local function encoded_title(instance_id, run_id)
  instance_id = instance_id or ''
  run_id = run_id or ''
  if instance_id == '' then
    return ''
  end
  return title_prefix .. instance_id .. '|' .. run_id
end

local function parse_phgo_title_raw(raw)
  raw = raw or ''
  if string.sub(raw, 1, string.len(title_prefix)) ~= title_prefix then
    return '', ''
  end
  local payload = string.sub(raw, string.len(title_prefix) + 1)
  local first_sep = string.find(payload, '|', 1, true)
  if first_sep == nil then
    return payload, ''
  end
  local instance_id = string.sub(payload, 1, first_sep - 1)
  local run_id = string.sub(payload, first_sep + 1)
  return instance_id or '', run_id or ''
end

local function startup_spawn(cmd)
  if cmd ~= nil and cmd.args ~= nil and not command_targets_main(cmd) then
    return cmd
  end
  local extra = {}
  if cmd ~= nil and cmd.args ~= nil then
    local skip_first = true
    for _, value in ipairs(cmd.args) do
      local normalized = normalize_prog(value)
      if skip_first and (normalized == './phytozome-go.bin' or normalized == 'phytozome-go.bin') then
        skip_first = false
      else
        table.insert(extra, value)
      end
    end
  end
  local args = { clean_cache_prog }
  for _, value in ipairs(extra) do
    table.insert(args, value)
  end
  return {
    cwd = (cmd ~= nil and cmd.cwd) or '.',
    args = args,
  }
end

local function parse_phgo_title(tab)
  local raw = ''
  if tab ~= nil and tab.tab_title ~= nil then
    raw = tab.tab_title
  end
  return parse_phgo_title_raw(raw)
end

local function root_instance_number(instance_id)
  instance_id = tostring(instance_id or '')
  local first = string.match(instance_id, '^(%d+)')
  if first == nil then
    return nil
  end
  return tonumber(first)
end

local function next_root_instance_id(window)
  if window == nil then
    return '1'
  end
  local mux_window = window:mux_window()
  if mux_window == nil then
    return '1'
  end
  local max_root = 0
  for _, info in ipairs(mux_window:tabs_with_info() or {}) do
    local raw = ''
    if info.tab ~= nil and info.tab.get_title ~= nil then
      raw = info.tab:get_title() or ''
    end
    local instance_id, _ = parse_phgo_title_raw(raw)
    local root = root_instance_number(instance_id)
    if root ~= nil and root > max_root then
      max_root = root
    end
  end
  return tostring(max_root + 1)
end

local function run_id_for_window(window)
  if window == nil then
    return ''
  end
  local mux_window = window:mux_window()
  if mux_window == nil then
    return ''
  end
  for _, info in ipairs(mux_window:tabs_with_info() or {}) do
    if info.is_active and info.tab ~= nil and info.tab.get_title ~= nil then
      local _, run_id = parse_phgo_title_raw(info.tab:get_title() or '')
      if run_id ~= '' then
        return run_id
      end
    end
  end
  for _, info in ipairs(mux_window:tabs_with_info() or {}) do
    if info.tab ~= nil and info.tab.get_title ~= nil then
      local _, run_id = parse_phgo_title_raw(info.tab:get_title() or '')
      if run_id ~= '' then
        return run_id
      end
    end
  end
  return ''
end

local function sync_close_confirmation(window)
  if window == nil then
    return
  end
  local mux_window = window:mux_window()
  if mux_window == nil then
    return
  end
  local tab_count = #(mux_window:tabs() or {})
  local desired = tab_count > 1 and 'AlwaysPrompt' or 'NeverPrompt'
  local overrides = window:get_config_overrides() or {}
  if overrides.window_close_confirmation == desired then
    return
  end
  overrides.window_close_confirmation = desired
  window:set_config_overrides(overrides)
end

wezterm.on('format-window-title', function(tab, pane, tabs, panes, config)
  return window_title
end)

local function display_instance_title(tab)
  local instance_id, _ = parse_phgo_title(tab)
  if instance_id ~= nil and instance_id ~= '' then
    return instance_id
  end
  return ''
end

wezterm.on('format-tab-title', function(tab, tabs, panes, config, hover, max_width)
  local title = display_instance_title(tab)
  if title == nil or title == '' then
    title = tab.active_pane.title or ''
  end
  if title == nil or title == '' then
    title = tostring(tab.tab_index + 1)
  end
  return '[' .. title .. ']'
end)

wezterm.on('new-tab-button-click', function(window, pane, button, default_action)
  if button ~= 'Left' then
    return false
  end
  local cwd = nil
  if pane ~= nil then
    cwd = pane:get_current_working_dir()
  end
  local run_id = run_id_for_window(window)
  local instance_id = next_root_instance_id(window)
  local args = { './phytozome-go.bin' }
  if run_id ~= '' then
    table.insert(args, '--instance-run-id')
    table.insert(args, run_id)
  end
  if instance_id ~= '' then
    table.insert(args, '--instance-id')
    table.insert(args, instance_id)
  end
  local mux_window = window ~= nil and window:mux_window() or nil
  if mux_window == nil then
    return false
  end
  local tab, _, _ = mux_window:spawn_tab {
    cwd = cwd,
    args = args,
  }
  if tab ~= nil then
    tab:set_title(encoded_title(instance_id, run_id))
  end
  sync_close_confirmation(window)
  return false
end)

wezterm.on('gui-startup', function(cmd)
  local spawn = startup_spawn(cmd)
  if spawn.cwd == nil then
    spawn.cwd = '.'
  end
  local _, _, window = mux.spawn_window(spawn)
  if window ~= nil and is_windows then
    window:gui_window():maximize()
  end
  if window ~= nil then
    local gui_window = window:gui_window()
    if gui_window ~= nil then
      sync_close_confirmation(gui_window)
    end
  end
end)

wezterm.on('update-status', function(window, pane)
  window:set_left_status('')
  window:set_right_status('')
  sync_close_confirmation(window)
end)

return {
  enable_tab_bar = true,
  hide_tab_bar_if_only_one_tab = false,
  show_new_tab_button_in_tab_bar = true,
  window_decorations = is_windows and 'INTEGRATED_BUTTONS|RESIZE' or 'TITLE|RESIZE',
  integrated_title_button_style = 'Windows',
  integrated_title_buttons = { 'Hide', 'Maximize', 'Close' },
  window_close_confirmation = 'NeverPrompt',
  automatically_reload_config = false,
  quit_when_all_windows_are_closed = true,
  default_prog = is_windows and { '.\\phytozome-go.bin' } or { './phytozome-go.bin' },

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
  use_fancy_tab_bar = true,
  show_tabs_in_tab_bar = true,
  show_tab_index_in_tab_bar = false,
  switch_to_last_active_tab_when_closing_tab = true,
  status_update_interval = 500,

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
    {
      event = { Up = { streak = 1, button = 'Right' } },
      mods = 'NONE',
      action = act.Nop,
    },
  },
}
"@ | Set-Content -LiteralPath $Path -Encoding UTF8
}

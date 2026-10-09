# Startup Home Design Notes

This document records confirmed requirements for the new startup/home interface.

## Source Sketch Rule

The user's sketch is a conceptual reminder only. Do not copy its control labels, typography, sizes, spacing, border weights, or visual proportions. The real implementation must follow the current application's established TUI design language and shared tview patterns.

Reference sketch:

![Startup home conceptual sketch](./assets/startup-home-sketch.png)

The saved sketch is only a visual reference for the rough structure described by the user. It is not a UI specification and must not override the written requirements or the existing application style.

## Overall Structure

- The current three startup modes should become three tabs on the new home interface.
- The tabs switch the concrete configuration content shown inside the home interface.
- The user should remain on this combined interface while configuring the selected workflow, instead of moving through many separate setup screens one by one.

## Startup Description

- The current top startup description should be compressed to save vertical space.
- Preserve the original meaning/content, but rewrite it as one concise paragraph instead of multiple separate lines.
- Do not remove important guidance only for compactness.
- Author, repository, and license metadata are written inline as `Author:`, `Repository:`, and `License:`. The labels stay in the normal text color; the values are highlighted.

## Tabs

- There are three top-level tabs matching the current three modes:
  - Keyword
  - BLAST
  - Explore
- The tabs are text tabs in the same visual family as the existing detail-page tabs, not button-like pills.
- Only the active tab content area is enclosed by the tab frame. The startup description above, contextual button bar below, and bottom shortcut hints stay outside that frame, matching the rest of the application page structure.
- The tab content frame contains only the tab's modules, such as Search/BLAST Options and Search/BLAST Content.
- The tab labels are embedded into the top-left border of that frame, in the form of adjacent bracketed tabs.
- The outer frame does not use a separate "Startup" title; the embedded tabs replace that title position.
- The selected tab's visual state should clearly change with the active tab.
- The tab labels show only the tab names: `Keyword`, `BLAST`, and `Explore`.
- `PgUp` and `PgDn` switch to the previous or next top-level tab.
- Plain `1`, `2`, and `3` must remain normal text input where a focused control accepts text.
- Mouse clicking a top tab switches to that tab.
- The plain `Tab` key must not switch top-level tabs. It moves focus between modules inside the active tab.
- Arrow keys move between controls inside the currently focused module, except inside the table editor where arrows belong to cell/caret navigation.
- Focused dropdowns and buttons are activated with either `Space` or `Enter`.
- While a dropdown is open, it captures Up/Down, Space, Enter, and Esc. Up/Down must navigate dropdown items instead of moving focus to another control; Space/Enter accepts the highlighted item; Esc closes the dropdown without changing the selected value.
- `Esc` on the main page does not close the page. If a dropdown is open, `Esc` closes only that dropdown.
- There is a one-line visual gap between the startup description paragraph and the tab content frame.

## Button Bar

- The button bar is contextual.
- Its visible buttons/actions change according to:
  - the active top-level tab
  - the current content/state inside that tab
- Current implementation:
  - Keyword: Clear content, Paste, optional Wide search, Search.
  - BLAST: Clear content, Paste, Run BLAST.
  - Explore: Start.
  - Existing shortcuts are reused where they do not conflict with the new focus rule: Clear content uses `Ctrl+N`, Paste uses `Ctrl+V`, and Wide search uses `Ctrl+W`.
  - The tab's main action uses `Ctrl+R`: Search on Keyword, Run BLAST on BLAST, and Start on Explore.
  - Focused module buttons are activated with `Space` or `Enter`.

## Shortcut Help

- Shortcut help remains in the same general bottom location as the current application.
- The help text should be concise.
- It should list only shortcuts that do not already have visible buttons.
- Shortcuts with visible button equivalents should not be repeated in the bottom help unless a later requirement explicitly asks for that.

## Module Titles

- Titles inside the operation frame are right-aligned to match the compact tab-frame layout.
- The currently focused small module uses the same light-blue focus color as the rest of the application, not just title bolding.
- The top-level tab frame is not a focused small module; the focus highlight belongs to the active module inside the tab.

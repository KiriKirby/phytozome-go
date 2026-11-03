# TUI Shortcut Map

`internal/tui/ShortcutMap` is the runtime authority. This file records the same contract for UI review and future changes.

| Action | Key | Scope |
|---|---|---|
| Back / close / cancel | `Esc` | Page, modal, task |
| Home | `Ctrl+O` | Root tabs only |
| Next / confirm / apply | `Enter` | Choice, single-line input, final field |
| Run / apply multiline input | `Ctrl+R` | Multiline input and run settings; bundled WezTerm forwards it through F13 |
| Toggle / activate control | `Space` | Button, checkbox, radio, dropdown |
| Paste | `Ctrl+V` | Input |
| Open file | `Ctrl+F` | File input |
| Filter | `Ctrl+F` | Result table |
| Select all | `Ctrl+A` | Result table |
| Clear selection / new group | `F4` | Table / group editor |
| Refresh / retry | `F5` | Settings / recovery |
| Clear filter / aliases | `Ctrl+L` | Disjoint filter and label contexts |
| Copy current value | `Ctrl+Y` | Table and detail views |
| Export | `Ctrl+G` | Results |
| Export all scope / Canvas add | `Ctrl+D` | Disjoint batch and Canvas contexts |
| Rename | `F2` | Editable item |
| Delete | `Del` | Editable item |
| Tree tools / tree focus | `F6` / `Ctrl+E` | Canvas |
| Open preview | `F7` | Canvas |
| Run BLAST | `Ctrl+B` | Results |
| Wide search | `F8` | Search |
| Help | `F1` | Help-enabled screens |
| Skip optional input | `Ctrl+K` | Optional input |

Keys that PHgo must not bind: `Ctrl+C`, `Ctrl+Enter`, `Ctrl+Shift+C`, `Ctrl+Shift+V`, `Ctrl+H`, `Ctrl+J`, `Ctrl+M`, bare `Ctrl+N`, and terminal control-code aliases for Tab/Enter/Backspace.

Bundled WezTerm owns `Ctrl+Tab`, `Ctrl+Shift+Tab`, `Ctrl+PageUp`, `Ctrl+PageDown`, `Shift+PageUp`, `Shift+PageDown`, `Alt+Enter`, `Ctrl+Shift+1..9`, `Ctrl+0`, font-size keys, `Ctrl+Shift+C/V/F/K/L/M/N/P/R/T/U/W/X/Z`, `Ctrl+Shift+Arrow`, `Ctrl+Alt+Shift+Arrow`, `Shift+Insert`, `Ctrl+Insert`, and `Ctrl+Shift+Space`. PHgo uses F6/F7/F8 for the former Ctrl+T/Ctrl+P/Ctrl+W actions. Ctrl+R is forwarded through F13 by the bundled config so the visible multiline shortcut remains Ctrl+R without invoking terminal reload.

Matching is exact: `Ctrl+Shift+R` is not `Ctrl+R`, and an unmodified digit is not `Ctrl+digit`.

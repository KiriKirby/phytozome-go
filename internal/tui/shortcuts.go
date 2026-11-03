package tui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
)

// ShortcutMap is the shared action registry. Reuse is allowed only between
// disjoint scopes. Page-specific numeric choices are intentionally excluded.
var ShortcutMap = map[string]struct{ Key, Scope, Description string }{
	"back":         {ShortcutBack, "page/modal/task", "Back, close, or cancel"},
	"home":         {ShortcutHome, "root tabs", "Return home"},
	"confirm":      {ShortcutConfirm, "choice/single-line/final field", "Next or apply"},
	"run":          {ShortcutRun, "multiline/settings", "Apply or run"},
	"paste":        {ShortcutPaste, "input", "Paste clipboard"},
	"open_file":    {ShortcutOpenFile, "file input", "Open a file"},
	"filter":       {ShortcutFilter, "table", "Open filter"},
	"select_all":   {ShortcutSelectAll, "table", "Select all rows"},
	"clear":        {ShortcutClear, "table", "Clear selection"},
	"new_group":    {ShortcutNewGroup, "group editor", "Create group"},
	"clear_filter": {ShortcutClearFilter, "filter", "Clear filter"},
	"aliases":      {ShortcutAliases, "label cell/group editor", "Choose alias"},
	"toggle":       {ShortcutToggle, "control", "Activate or toggle"},
	"copy":         {ShortcutCopy, "table/detail", "Copy current value"},
	"export":       {ShortcutExport, "results", "Export"},
	"groups":       {ShortcutExport, "family settings", "Customize groups"},
	"export_all":   {ShortcutExportAll, "batch results", "Choose all-table export"},
	"add":          {ShortcutAdd, "Canvas", "Add canvas or rows"},
	"rename":       {ShortcutRename, "editable item", "Rename"},
	"delete":       {ShortcutDelete, "editable item", "Delete"},
	"tree_tools":   {ShortcutTreeTools, "Canvas", "Show tree tools"},
	"tree_focus":   {ShortcutTreeFocus, "Canvas", "Focus tree tools"},
	"preview":      {ShortcutPreview, "Canvas", "Open preview"},
	"blast":        {ShortcutBlast, "results", "Run BLAST"},
	"retry":        {ShortcutRetry, "recovery", "Retry"},
	"refresh":      {ShortcutRefresh, "settings", "Refresh preview"},
	"wide_search":  {ShortcutWideSearch, "search", "Wide search"},
	"help":         {ShortcutHelp, "help-enabled screen", "Open help"},
	"skip":         {"Ctrl+K", "optional input", "Skip"},
}

// shortcutMatchesEvent compares modifiers exactly. KeyCtrlX already encodes
// Ctrl but not Shift. Terminal control-code aliases are deliberately excluded.
func shortcutMatchesEvent(shortcut string, event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	parts := strings.Split(strings.ToLower(strings.TrimSpace(shortcut)), "+")
	want := tcell.ModNone
	key := ""
	for _, part := range parts {
		switch strings.TrimSpace(part) {
		case "ctrl":
			want |= tcell.ModCtrl
		case "shift":
			want |= tcell.ModShift
		case "alt":
			want |= tcell.ModAlt
		default:
			if key != "" {
				return false
			}
			key = strings.TrimSpace(part)
		}
	}
	if key == "" || (want&tcell.ModCtrl != 0 && strings.Contains(" c enter i j m h ", " "+key+" ")) {
		return false
	}
	got := event.Modifiers() & (tcell.ModCtrl | tcell.ModShift | tcell.ModAlt | tcell.ModMeta)
	if event.Key() >= tcell.KeyCtrlA && event.Key() <= tcell.KeyCtrlZ &&
		event.Key() != tcell.KeyTab && event.Key() != tcell.KeyEnter && event.Key() != tcell.KeyBackspace {
		got |= tcell.ModCtrl
		return want == got && key == string(rune('a'+event.Key()-tcell.KeyCtrlA))
	}
	if got != want {
		return false
	}
	if strings.HasPrefix(key, "f") {
		for n := 1; n <= 12; n++ {
			if key == fmt.Sprintf("f%d", n) && event.Key() == tcell.KeyF1+tcell.Key(n-1) {
				return true
			}
		}
	}
	switch key {
	case "esc", "escape":
		return event.Key() == tcell.KeyEscape
	case "enter":
		return event.Key() == tcell.KeyEnter
	case "tab":
		return event.Key() == tcell.KeyTab
	case "space":
		return event.Key() == tcell.KeyRune && event.Rune() == ' '
	case "del", "delete":
		return event.Key() == tcell.KeyDelete || event.Key() == tcell.KeyDEL
	}
	return event.Key() == tcell.KeyRune && key == string(unicode.ToLower(event.Rune()))
}

// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package tui

const (
	ButtonBack        = "Back"
	ButtonHome        = "Home"
	ButtonClose       = "Close"
	ButtonOK          = "OK"
	ButtonStart       = "Start"
	ButtonSelect      = "Select"
	ButtonSearch      = "Search"
	ButtonWideSearch  = "Wide search"
	ButtonRunBLAST    = "Run BLAST"
	ButtonApply       = "Apply"
	ButtonAuto        = "Auto identify"
	ButtonSave        = "Save"
	ButtonOpen        = "Open"
	ButtonOpenFile    = "Open..."
	ButtonView        = "View"
	ButtonHelp        = "Help"
	ButtonFilter      = "Filter"
	ButtonCopy        = "Copy"
	ButtonExport      = "Export"
	ButtonExportAll   = "Export all"
	ButtonPaste       = "Paste"
	ButtonCancel      = "Cancel"
	ButtonRetry       = "Retry"
	ButtonSkip        = "Skip"
	ButtonInstall     = "Install"
	ButtonYes         = "Yes"
	ButtonNo          = "No"
	ButtonSelectAll   = "Select all"
	ButtonClear       = "Clear"
	ButtonClearFilter = "Clear filter"
	ButtonToggle      = "Toggle"
)

const (
	ShortcutBack      = "Esc"
	ShortcutHome      = "Ctrl+O"
	ShortcutConfirm   = "Enter"
	ShortcutApply     = "Enter"
	ShortcutPaste     = "Ctrl+V"
	ShortcutOpenFile  = "Ctrl+F"
	ShortcutCancel    = "Esc"
	ShortcutAuto      = "Enter"
	ShortcutSelectAll = "Ctrl+A"
	// Ctrl+N can be emitted as a line-feed/control character during
	// bracketed multiline paste. Keep clear actions off that key.
	ShortcutClear       = "Ctrl+Shift+N"
	ShortcutClearFilter = "Ctrl+L"
	ShortcutToggle      = "Space"
	ShortcutCopy        = "Ctrl+Y"
	ShortcutTreeFocus   = "Ctrl+E"
	ShortcutFilter      = "Ctrl+F"
	ShortcutExport      = "Ctrl+G"
	ShortcutExportAll   = "Ctrl+D"
	ShortcutPreview     = "Ctrl+P"
	ShortcutBlast       = "Ctrl+B"
	// Ctrl+R is reserved for the current page's Run/primary action.
	// Retry remains available without shadowing Run.
	ShortcutRetry      = "Ctrl+Shift+R"
	ShortcutWideSearch = "Ctrl+W"
	ShortcutHelp       = "F1"
)

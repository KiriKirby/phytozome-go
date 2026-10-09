// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package tui

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/KiriKirby/phytozome-go/internal/ncbi"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type MainInterfacePage struct {
	Info          StartupInfo
	State         MainInterfaceState
	SpeciesLoader MainSpeciesLoader
	// SpeciesPreflight runs before the species modal is created.  It is used
	// for database installation/update gates so a download page never gets
	// mounted underneath the species selector.
	SpeciesPreflight func(context.Context, MainSpeciesRequest) error
}

type MainInterfaceResult struct {
	Action string
	State  MainInterfaceState
}

const (
	MainActionKeywordSearch     = "keyword_search"
	MainActionKeywordWideSearch = "keyword_wide_search"
	MainActionBlastRun          = "blast_run"
	MainActionExploreTool       = "explore_tool"
	MainActionExit              = "exit"
)

type MainSpeciesLoader func(context.Context, MainSpeciesRequest) ([]MainSpeciesOption, error)

type MainSpeciesOption struct {
	Key         string
	Label       string
	Description string
	SearchText  string
	Disabled    bool
}

type MainInterfaceState struct {
	ActiveTab string
	Keyword   MainKeywordState
	Blast     MainBlastState
	Explore   MainExploreState
}

type MainKeywordState struct {
	DatabaseID                string
	SearchTypeID              string
	GeneLocusPriorityDatabase string
	PLAZAGeneLocusPriority    bool // Deprecated compatibility alias for the old PLAZA checkbox state.
	SpeciesKey                string
	SpeciesLabel              string
	SpeciesMode               string
	FallbackSpeciesKey        string
	FallbackSpeciesLabel      string
	Rows                      []MainKeywordRow
	Grid                      GridEditorState
}

const (
	GeneLocusPriorityNone  = ""
	GeneLocusPriorityNCBI  = "ncbi"
	GeneLocusPriorityPLAZA = "plaza"
)

type MainKeywordRow struct {
	SearchTerm   string
	SymbolName   string
	GeneLocus    string
	SpeciesKey   string
	SpeciesLabel string
	Aliases      []string
}

type MainBlastState struct {
	DatabaseID   string
	SpeciesKey   string
	SpeciesLabel string
	ProgramID    string
	Rows         []MainBlastRow
	Grid         GridEditorState
}

type MainBlastRow struct {
	FASTA      string
	SymbolName string
	GeneLocus  string
	Aliases    []string
}

type MainExploreState struct {
	Tool string
}

type MainSpeciesRequest struct {
	Mode       string
	DatabaseID string
}

func (r MainInterfaceResult) SpeciesRequest() MainSpeciesRequest {
	switch r.State.ActiveTab {
	case "blast":
		return MainSpeciesRequest{Mode: "blast", DatabaseID: r.State.Blast.DatabaseID}
	default:
		return MainSpeciesRequest{Mode: "keyword", DatabaseID: r.State.Keyword.DatabaseID}
	}
}

type mainCapability struct {
	ID                  string
	Label               string
	Description         string
	SearchTypes         []mainSearchTypeCapability
	DefaultSearchTypeID string
	RequiresSpecies     bool
	ShowsSpecies        bool
	ShowsSymbolName     bool
	ShowsGeneLocus      bool
	SupportsWide        bool
	BlastPrograms       []string
}

type mainSearchTypeCapability struct {
	ID              string
	Label           string
	RequiresSpecies bool
	ShowsSpecies    bool
	ShowsSymbolName bool
	ShowsGeneLocus  bool
	SupportsWide    bool
}

type GridEditorState struct {
	ActiveRow int
	ActiveCol int
}

type gridColumn struct {
	ID       string
	Title    string
	Get      func(int) string
	Set      func(int, string)
	Min      int
	Weight   int
	KeepNL   bool
	Fixed    int
	Wrap     bool
	ReadOnly bool
}

type mainModuleFocus struct {
	box      *tview.Box
	controls []tview.Primitive
	index    int
}

type mainGridEditor struct {
	*tview.Box
	columns       []gridColumn
	rowCount      func() int
	ensureRows    func(int)
	cleanRows     func()
	state         *GridEditorState
	caretByCell   map[string]int
	rowOffset     int
	colOffset     int
	status        *tview.TextView
	pasteLines    func(string, int) []string
	enterRows     bool
	fastaGrid     bool
	onFocusCell   func()
	onSpecialCell func(row int, column string)
}

type gridVisualLine struct {
	Text  string
	Start int
	End   int
}

type mainTabFrame struct {
	*tview.Box
	child    tview.Primitive
	active   string
	onSelect func(string)
	onFocus  func(tview.Primitive)
}

type mainExploreList struct {
	*tview.List
}

type mainControlField struct {
	*tview.Box
	label string
	child tview.Primitive
}

func DefaultMainInterfaceState() MainInterfaceState {
	return NormalizeMainInterfaceState(MainInterfaceState{})
}

func NormalizeMainInterfaceState(state MainInterfaceState) MainInterfaceState {
	state.ActiveTab = normalizeMainTab(state.ActiveTab)
	if state.Keyword.DatabaseID == "" {
		state.Keyword.DatabaseID = "phytozome"
	}
	if state.Keyword.SearchTypeID == "" {
		state.Keyword.SearchTypeID = mainKeywordSearchType(state.Keyword.DatabaseID).ID
	}
	if state.Keyword.SpeciesMode == "" {
		state.Keyword.SpeciesMode = "custom"
	}
	state.Keyword.GeneLocusPriorityDatabase = normalizeGeneLocusPriorityDatabase(state.Keyword.GeneLocusPriorityDatabase)
	if state.Keyword.GeneLocusPriorityDatabase == GeneLocusPriorityNone && state.Keyword.PLAZAGeneLocusPriority {
		state.Keyword.GeneLocusPriorityDatabase = GeneLocusPriorityPLAZA
	}
	state.Keyword.PLAZAGeneLocusPriority = state.Keyword.GeneLocusPriorityDatabase == GeneLocusPriorityPLAZA
	if len(state.Keyword.Rows) == 0 {
		state.Keyword.Rows = []MainKeywordRow{{}}
	}
	if state.Blast.DatabaseID == "" {
		state.Blast.DatabaseID = "phytozome"
	}
	if state.Blast.ProgramID == "" {
		state.Blast.ProgramID = "blastn"
	}
	if len(state.Blast.Rows) == 0 {
		state.Blast.Rows = []MainBlastRow{{}}
	}
	if state.Explore.Tool == "" {
		state.Explore.Tool = "open_session"
	}
	state.Keyword.Grid.ActiveRow = clampInt(state.Keyword.Grid.ActiveRow, 0, maxInt(0, len(state.Keyword.Rows)-1))
	state.Blast.Grid.ActiveRow = clampInt(state.Blast.Grid.ActiveRow, 0, maxInt(0, len(state.Blast.Rows)-1))
	return state
}

func normalizeGeneLocusPriorityDatabase(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case GeneLocusPriorityNCBI:
		return GeneLocusPriorityNCBI
	case GeneLocusPriorityPLAZA:
		return GeneLocusPriorityPLAZA
	default:
		return GeneLocusPriorityNone
	}
}

func MainKeywordRowsForExecution(rows []MainKeywordRow) []MainKeywordRow {
	out := make([]MainKeywordRow, 0, len(rows))
	for _, row := range rows {
		normalized := MainKeywordRow{
			SearchTerm: normalizeMainGridValue(row.SearchTerm),
			SymbolName: normalizeMainGridValue(row.SymbolName),
			GeneLocus:  normalizeMainGridValue(row.GeneLocus),
			SpeciesKey: normalizeMainSpeciesKey(row.SpeciesKey), SpeciesLabel: normalizeMainSpeciesValue(row.SpeciesLabel),
			Aliases: mainAliasChoices(row.SymbolName, row.Aliases),
		}
		if normalized.SearchTerm == "" && normalized.SymbolName == "" && normalized.GeneLocus == "" && normalized.SpeciesLabel == "" {
			continue
		}
		out = append(out, normalized)
	}
	return out
}

func MainBlastRowsForExecution(rows []MainBlastRow) []MainBlastRow {
	out := make([]MainBlastRow, 0, len(rows))
	for _, row := range rows {
		normalized := MainBlastRow{
			FASTA:      normalizeMainGridValueForColumn(row.FASTA, gridColumn{KeepNL: true}),
			SymbolName: normalizeMainGridValue(row.SymbolName),
			GeneLocus:  normalizeMainGridValue(row.GeneLocus),
			Aliases:    mainAliasChoices(row.SymbolName, row.Aliases),
		}
		if normalized.FASTA == "" && normalized.SymbolName == "" && normalized.GeneLocus == "" {
			continue
		}
		out = append(out, normalized)
	}
	return out
}

func normalizeMainSpeciesOptions(options []MainSpeciesOption) []MainSpeciesOption {
	out := make([]MainSpeciesOption, 0, len(options))
	seen := map[string]bool{}
	for _, option := range options {
		option.Key = strings.TrimSpace(option.Key)
		option.Label = strings.TrimSpace(option.Label)
		option.Description = strings.TrimSpace(option.Description)
		option.SearchText = strings.TrimSpace(option.SearchText)
		if option.Key == "" || option.Label == "" || seen[option.Key] {
			continue
		}
		if option.SearchText == "" {
			option.SearchText = strings.Join([]string{option.Label, option.Description, option.Key}, " ")
		}
		seen[option.Key] = true
		out = append(out, option)
	}
	return out
}

// normalizeMainSpeciesValue preserves the spaces that are significant in
// scientific names while removing terminal/control whitespace introduced by
// terminal editing or pasted values.
func normalizeMainSpeciesValue(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.Join(strings.Fields(value), " ")
	if value == "~" {
		return ""
	}
	return value
}

// SpeciesKey is an opaque UI identity and may contain NUL separators. Do not
// run it through whitespace normalization, which would destroy that identity.
func normalizeMainSpeciesKey(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r", ""), "\n", ""))
	if value == "~" {
		return ""
	}
	return value
}

func filterMainSpeciesOptions(query string, options []MainSpeciesOption) []MainSpeciesOption {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]MainSpeciesOption(nil), options...)
	}
	parts := strings.Fields(query)
	out := make([]MainSpeciesOption, 0, len(options))
	for _, option := range options {
		haystack := strings.ToLower(strings.Join([]string{option.Label, option.Description, option.SearchText, option.Key}, " "))
		matched := true
		for _, part := range parts {
			if !strings.Contains(haystack, part) {
				matched = false
				break
			}
		}
		if matched {
			out = append(out, option)
		}
	}
	return out
}

func MainKeywordValidationIssues(state MainKeywordState) []string {
	cap := mainKeywordCapability(state.DatabaseID)
	searchType := mainKeywordSearchTypeFor(cap, state.SearchTypeID)
	var issues []string
	if strings.TrimSpace(state.DatabaseID) == "" {
		issues = append(issues, "Choose a database.")
	}
	if searchType.RequiresSpecies && state.DatabaseID != "cyp" && strings.TrimSpace(state.SpeciesLabel) == "" {
		issues = append(issues, "Set species.")
	}
	if state.DatabaseID == "cyp" && state.SpeciesMode != "custom" && strings.TrimSpace(state.FallbackSpeciesLabel) == "" {
		issues = append(issues, "Set the CYP species for this mode.")
	}
	rows := MainKeywordRowsForExecution(state.Rows)
	if len(rows) == 0 {
		issues = append(issues, "Enter at least one search term.")
		return issues
	}
	for i, row := range rows {
		if strings.TrimSpace(row.SearchTerm) == "" {
			issues = append(issues, fmt.Sprintf("Row %d is missing search term.", i+1))
		}
		if state.DatabaseID == "cyp" && state.SpeciesMode == "custom" && strings.TrimSpace(row.SpeciesLabel) == "" {
			inherited := false
			for j := i - 1; j >= 0; j-- {
				if strings.TrimSpace(rows[j].SpeciesLabel) != "" {
					inherited = true
					break
				}
			}
			if !inherited {
				issues = append(issues, fmt.Sprintf("Row %d has no species and no earlier row species to inherit.", i+1))
			}
		}
	}
	return issues
}

func MainBlastValidationIssues(state MainBlastState) []string {
	cap := mainBlastCapability(state.DatabaseID)
	var issues []string
	if strings.TrimSpace(state.DatabaseID) == "" {
		issues = append(issues, "Choose a database.")
	}
	if cap.RequiresSpecies && strings.TrimSpace(state.SpeciesLabel) == "" {
		issues = append(issues, "Set species.")
	}
	if len(cap.BlastPrograms) > 0 && strings.TrimSpace(state.ProgramID) == "" {
		issues = append(issues, "Choose a BLAST program.")
	}
	rows := MainBlastRowsForExecution(state.Rows)
	if len(rows) == 0 {
		issues = append(issues, "Enter at least one FASTA or sequence.")
		return issues
	}
	for i, row := range rows {
		if strings.TrimSpace(row.FASTA) == "" {
			issues = append(issues, fmt.Sprintf("Row %d is missing FASTA or sequence.", i+1))
		}
	}
	return issues
}

func RunMainInterfacePage(page MainInterfacePage) (MainInterfaceResult, error) {
	state := NormalizeMainInterfaceState(page.State)
	app := newApp()
	var result MainInterfaceResult
	var modules []mainModuleFocus
	moduleIndex := 0
	var body *buttonFlex
	var root tview.Primitive
	var keywordGrid *mainGridEditor
	var blastGrid *mainGridEditor
	var actionRow *buttonRowPrimitive
	var modalKeyHandler func(*tcell.EventKey) bool
	var speciesLoadSeq atomic.Uint64
	var focusCurrentModule func()
	var syncModuleFocus func(tview.Primitive)
	var lockedDropDown *tview.DropDown
	var rebuildActionButtons func()
	var showAliasModal func()
	var validateKeywordAndStop func(string)
	var validateBlastAndStop func()

	stopWith := func(action string) {
		result.Action = action
		result.State = NormalizeMainInterfaceState(state)
		if keywordGrid != nil {
			result.State.Keyword.Grid = *keywordGrid.state
		}
		if blastGrid != nil {
			result.State.Blast.Grid = *blastGrid.state
		}
		app.Stop()
	}
	showModal := func(title string, message string) {
		if strings.TrimSpace(message) == "" {
			message = "No details."
		}
		modalBody := newButtonFlex()
		modalBody.SetBorder(true)
		modalBody.SetTitle(" " + trimColon(title) + " ")
		modalBody.SetTitleAlign(tview.AlignCenter)
		setFocusBorder(modalBody.Box, true)
		attachFocusBorder(modalBody.Box)
		modalBody.AddItem(textPanel(title, message).SetScrollable(true), 0, 1, true)
		close := func() {
			modalKeyHandler = nil
			setPageRoot(app, root)
			focusCurrentModule()
		}
		buttons := closeOnlyModalButtons(nil, true, ButtonOK, ShortcutConfirm, close, close)
		addButtonRow(modalBody, buttons)
		setPageRoot(app, overlayRootOn(root, modalBody, 90, 18))
		app.SetFocus(buttons)
		modalKeyHandler = func(event *tcell.EventKey) bool {
			if buttonRowHandlesShortcut(buttons, event) {
				return true
			}
			if event.Key() == tcell.KeyEnter || event.Key() == tcell.KeyRune && event.Rune() == ' ' {
				if handler := buttons.InputHandler(); handler != nil {
					handler(event, func(p tview.Primitive) {
						if p != nil {
							app.SetFocus(p)
						}
					})
					return true
				}
			}
			if event.Key() == tcell.KeyEscape {
				close()
				return true
			}
			return false
		}
	}

	var rebuild func()
	focusCurrentModule = func() {
		if len(modules) == 0 {
			return
		}
		if moduleIndex < 0 {
			moduleIndex = len(modules) - 1
		}
		if moduleIndex >= len(modules) {
			moduleIndex = 0
		}
		module := &modules[moduleIndex]
		for i := range modules {
			if modules[i].box != nil {
				setFocusBorder(modules[i].box, i == moduleIndex)
			}
		}
		if len(module.controls) == 0 {
			return
		}
		if module.index < 0 {
			module.index = len(module.controls) - 1
		}
		if module.index >= len(module.controls) {
			module.index = 0
		}
		app.SetFocus(module.controls[module.index])
	}
	syncModuleFocus = func(focus tview.Primitive) {
		if focus == nil || len(modules) == 0 {
			return
		}
		for i := range modules {
			for j, control := range modules[i].controls {
				if control == focus {
					moduleIndex = i
					modules[i].index = j
					for k := range modules {
						if modules[k].box != nil {
							setFocusBorder(modules[k].box, k == moduleIndex)
						}
					}
					return
				}
			}
		}
	}
	focusModule := func(delta int) {
		if len(modules) == 0 {
			return
		}
		moduleIndex += delta
		if moduleIndex < 0 {
			moduleIndex = len(modules) - 1
		}
		if moduleIndex >= len(modules) {
			moduleIndex = 0
		}
		focusCurrentModule()
	}
	focusWithinModule := func(delta int) {
		if len(modules) == 0 || moduleIndex < 0 || moduleIndex >= len(modules) {
			return
		}
		module := &modules[moduleIndex]
		if len(module.controls) == 0 {
			return
		}
		module.index += delta
		if module.index < 0 {
			module.index = len(module.controls) - 1
		}
		if module.index >= len(module.controls) {
			module.index = 0
		}
		app.SetFocus(module.controls[module.index])
	}
	switchTab := func(tab string) {
		state.ActiveTab = normalizeMainTab(tab)
		moduleIndex = 0
		rebuild()
	}
	switchTabDelta := func(delta int) {
		tabs := mainTabs()
		current := 0
		for i, tab := range tabs {
			if tab.id == normalizeMainTab(state.ActiveTab) {
				current = i
				break
			}
		}
		next := current + delta
		if next < 0 {
			next = len(tabs) - 1
		}
		if next >= len(tabs) {
			next = 0
		}
		switchTab(tabs[next].id)
	}
	validateKeywordAndStop = func(action string) {
		if issues := MainKeywordValidationIssues(state.Keyword); len(issues) > 0 {
			showModal("Keyword input", strings.Join(issues, "\n"))
			return
		}
		stopWith(action)
	}
	validateBlastAndStop = func() {
		if issues := MainBlastValidationIssues(state.Blast); len(issues) > 0 {
			showModal("BLAST input", strings.Join(issues, "\n"))
			return
		}
		stopWith(MainActionBlastRun)
	}
	clearActiveGrid := func() {
		switch state.ActiveTab {
		case "blast":
			state.Blast.Rows = []MainBlastRow{{}}
			state.Blast.Grid = GridEditorState{}
		default:
			state.Keyword.Rows = []MainKeywordRow{{}}
			state.Keyword.Grid = GridEditorState{}
		}
		rebuild()
	}
	pasteActiveGrid := func() {
		switch state.ActiveTab {
		case "blast":
			if blastGrid != nil {
				blastGrid.PasteClipboard(app)
			}
		default:
			if keywordGrid != nil {
				keywordGrid.PasteClipboard(app)
			}
		}
	}
	activeGrid := func() *mainGridEditor {
		switch state.ActiveTab {
		case "blast":
			return blastGrid
		case "keyword":
			return keywordGrid
		default:
			return nil
		}
	}
	activeSymbolAliases := func() []string {
		grid := activeGrid()
		if grid == nil || grid.state == nil || !grid.activeColumnIs("symbol") {
			return nil
		}
		row := grid.state.ActiveRow
		switch state.ActiveTab {
		case "blast":
			if row >= 0 && row < len(state.Blast.Rows) {
				return mainAliasChoices(state.Blast.Rows[row].SymbolName, state.Blast.Rows[row].Aliases)
			}
		case "keyword":
			if row >= 0 && row < len(state.Keyword.Rows) {
				return mainAliasChoices(state.Keyword.Rows[row].SymbolName, state.Keyword.Rows[row].Aliases)
			}
		}
		return nil
	}
	applyActiveSymbolAlias := func(alias string) {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			return
		}
		grid := activeGrid()
		if grid == nil || grid.state == nil {
			return
		}
		row := grid.state.ActiveRow
		switch state.ActiveTab {
		case "blast":
			if row >= 0 && row < len(state.Blast.Rows) {
				state.Blast.Rows[row].SymbolName = alias
			}
		case "keyword":
			if row >= 0 && row < len(state.Keyword.Rows) {
				state.Keyword.Rows[row].SymbolName = alias
			}
		}
	}
	showAliasModal = func() {
		grid := activeGrid()
		if grid == nil || grid.state == nil || !grid.activeColumnIs("symbol") {
			showModal("Aliases", "Aliases are available only when a Symbol name cell is selected.")
			return
		}
		aliases := activeSymbolAliases()
		if len(aliases) == 0 {
			showModal("Aliases", "No alias symbol names are available for this row.")
			return
		}
		list := tview.NewList()
		list.ShowSecondaryText(false)
		list.SetSelectedTextColor(tcell.ColorBlack)
		list.SetSelectedBackgroundColor(tcell.ColorWhite)
		list.SetSelectedFocusOnly(false)
		list.SetBorder(true).SetTitle(" Alias symbol names ").SetTitleAlign(tview.AlignCenter)
		for _, alias := range aliases {
			list.AddItem(alias, "", 0, nil)
		}
		for i, alias := range aliases {
			if strings.EqualFold(alias, grid.currentValue()) {
				list.SetCurrentItem(i)
				break
			}
		}
		list.SetSelectedFunc(func(index int, _ string, _ string, _ rune) {
			if index >= 0 && index < len(aliases) {
				list.SetCurrentItem(index)
			}
		})
		selectedAlias := func() string {
			index := list.GetCurrentItem()
			if index < 0 {
				index = 0
			}
			if index >= len(aliases) {
				index = len(aliases) - 1
			}
			if index < 0 || index >= len(aliases) {
				return ""
			}
			return aliases[index]
		}
		closeAliasModal := func() {
			modalKeyHandler = nil
			setPageRoot(app, root)
			if grid != nil {
				app.SetFocus(grid)
				syncModuleFocus(grid)
			} else {
				focusCurrentModule()
			}
		}
		copyAlias := func() {
			alias := selectedAlias()
			if alias == "" {
				return
			}
			if err := writeClipboardText(alias); err != nil {
				showModal("Copy failed", err.Error())
			}
		}
		setAlias := func(alias string) {
			if strings.TrimSpace(alias) == "" {
				return
			}
			applyActiveSymbolAlias(alias)
			closeAliasModal()
		}
		box := newButtonFlex()
		box.SetBorder(true)
		box.SetTitle(" Aliases ")
		box.SetTitleAlign(tview.AlignCenter)
		box.AddItem(textBlock("Choose an alias symbol name. Copy copies the selected alias; Set as symbol name fixes it as this row's symbol name."), 3, 0, false)
		box.AddItem(list, 0, 1, true)
		buttons := buttonRow(
			buttonSpec{Label: ButtonClose, Shortcut: ShortcutBack, Action: closeAliasModal, Visible: true},
			buttonSpec{Label: ButtonCopy, Shortcut: ShortcutCopy, Action: copyAlias, Visible: true},
			buttonSpec{Label: "Set as symbol name", Shortcut: ShortcutApply, Action: func() { setAlias(selectedAlias()) }, Visible: true, Primary: true},
		)
		addButtonRow(box, buttons)
		modalKeyHandler = func(event *tcell.EventKey) bool {
			if event == nil {
				return true
			}
			if buttonRowHandlesShortcut(buttons, event) {
				return true
			}
			if isCopyShortcut(event) {
				copyAlias()
				return true
			}
			switch event.Key() {
			case tcell.KeyEscape:
				closeAliasModal()
				return true
			case tcell.KeyEnter:
				if event.Modifiers()&tcell.ModCtrl == 0 {
					setAlias(selectedAlias())
				}
				return true
			case tcell.KeyUp:
				if list.GetCurrentItem() > 0 {
					list.SetCurrentItem(list.GetCurrentItem() - 1)
				}
				return true
			case tcell.KeyDown:
				if list.GetCurrentItem() < len(aliases)-1 {
					list.SetCurrentItem(list.GetCurrentItem() + 1)
				}
				return true
			}
			if handler := list.InputHandler(); handler != nil {
				handler(event, func(p tview.Primitive) {
					if p != nil {
						app.SetFocus(p)
					}
				})
			}
			return true
		}
		setPageRoot(app, overlayRootOn(root, box, 68, rowSelectionAliasOverlayHeight(len(aliases))))
		app.SetFocus(list)
	}
	rebuildActionButtons = func() {
		if actionRow == nil {
			return
		}
		buttons := []buttonSpec{
			{Label: ButtonClear + " content", Shortcut: ShortcutClear, Action: clearActiveGrid, Visible: state.ActiveTab == "keyword" || state.ActiveTab == "blast", LeftPrimary: true},
			{Label: ButtonPaste, Shortcut: ShortcutPaste, Action: pasteActiveGrid, Visible: state.ActiveTab == "keyword" || state.ActiveTab == "blast", LeftPrimary: true},
			{Label: "Aliases", Shortcut: "Ctrl+L", Action: showAliasModal, Visible: activeGrid() != nil && activeGrid().activeColumnIs("symbol"), LeftPrimary: true},
		}
		switch state.ActiveTab {
		case "blast":
			buttons = append(buttons, buttonSpec{Label: ButtonRunBLAST, Shortcut: "Ctrl+R", Action: validateBlastAndStop, Visible: true, Primary: true})
		case "explore":
			buttons = append(buttons, buttonSpec{Label: ButtonStart, Shortcut: "Ctrl+R", Action: func() { stopWith(MainActionExploreTool) }, Visible: true, Primary: true})
		default:
			searchType := mainKeywordSearchType(state.Keyword.DatabaseID)
			if searchType.SupportsWide {
				buttons = append(buttons, buttonSpec{Label: ButtonWideSearch, Shortcut: ShortcutWideSearch, Action: func() { validateKeywordAndStop(MainActionKeywordWideSearch) }, Visible: true, Primary: true})
			}
			buttons = append(buttons, buttonSpec{Label: ButtonSearch, Shortcut: "Ctrl+R", Action: func() { validateKeywordAndStop(MainActionKeywordSearch) }, Visible: true, Primary: true})
		}
		actionRow.buttons = buttons
		if body != nil {
			body.invalidateLayout()
		}
	}
	showSpeciesModal := func(tab string) {
		state.ActiveTab = normalizeMainTab(tab)
		request := MainSpeciesRequest{Mode: "keyword", DatabaseID: state.Keyword.DatabaseID}
		if state.ActiveTab == "blast" {
			request = MainSpeciesRequest{Mode: "blast", DatabaseID: state.Blast.DatabaseID}
		}
		if page.SpeciesLoader == nil {
			showModal("Species", "Species selection is not available in this build.")
			return
		}
		if page.SpeciesPreflight != nil {
			if err := page.SpeciesPreflight(context.Background(), request); err != nil {
				showModal("CYP database", err.Error())
				return
			}
		}
		seq := speciesLoadSeq.Add(1)
		ctx, cancel := context.WithCancel(context.Background())
		modalBody := newButtonFlex()
		modalBody.SetBorder(true)
		modalBody.SetTitle(" Species ")
		modalBody.SetTitleAlign(tview.AlignCenter)
		setFocusBorder(modalBody.Box, true)
		attachFocusBorder(modalBody.Box)

		input := tview.NewInputField().
			SetLabel("Search ").
			SetPlaceholder("type species, alias, release, or ID").
			SetFieldWidth(-1)
		input.SetBorder(true)
		input.SetTitle(" Search ")
		input.SetTitleAlign(tview.AlignCenter)
		setFocusBorder(input.Box, true)
		attachFocusBorder(input.Box)
		inputFrame := clipPrimitive(input)
		results := tview.NewTable().SetBorders(false).SetSelectable(false, false)
		results.SetBorder(true)
		results.SetTitle(" Candidates ")
		results.SetTitleAlign(tview.AlignCenter)
		setFocusBorder(results.Box, false)
		attachFocusBorder(results.Box)
		status := hintView("Loading species candidates...")
		modalBody.AddItem(inputFrame, 0, 0, true)
		modalBody.AddItem(results, 0, 1, false)
		if request.DatabaseID == "cyp" {
			modalBody.AddItem(hintView("Category filter: a:/A: animals, p:/P: plants, f:/F: fungi, b:/B: bacteria; add a space before the search term."), 1, 0, false)
		}
		modalBody.AddItem(status, 1, 0, false)

		var options []MainSpeciesOption
		var filtered []MainSpeciesOption
		selected := 0
		loading := true
		useSearch := true
		confirmed := false
		var close func()
		close = func() {
			cancel()
			modalKeyHandler = nil
			reverted := request.DatabaseID == "cyp" && request.Mode == "keyword" && !confirmed && state.Keyword.SpeciesMode != "custom"
			if reverted {
				state.Keyword.SpeciesMode = "custom"
				state.Keyword.FallbackSpeciesKey = ""
				state.Keyword.FallbackSpeciesLabel = ""
			}
			setPageRoot(app, root)
			if reverted {
				rebuild()
			} else {
				focusCurrentModule()
			}
		}
		applyFilter := func() {
			if useSearch {
				filtered = filterMainSpeciesOptions(input.GetText(), options)
			} else {
				filtered = append([]MainSpeciesOption(nil), options...)
			}
			if selected >= len(filtered) {
				selected = len(filtered) - 1
			}
			if selected < 0 {
				selected = 0
			}
		}
		var render func()
		render = func() {
			results.Clear()
			if loading {
				results.SetCell(0, 0, tview.NewTableCell("- Loading").SetTextColor(colorAction).SetSelectable(false))
				results.SetCell(1, 0, tview.NewTableCell("  Fetching candidates from the selected database.").SetTextColor(tview.Styles.SecondaryTextColor).SetSelectable(false))
				return
			}
			if len(filtered) == 0 {
				results.SetCell(0, 0, tview.NewTableCell("- No matches").SetTextColor(tview.Styles.PrimaryTextColor).SetSelectable(false))
				results.SetCell(1, 0, tview.NewTableCell("  Edit the search box to search again.").SetTextColor(tview.Styles.SecondaryTextColor).SetSelectable(false))
				status.SetText("No matching species.")
				return
			}
			for i, option := range filtered {
				nameStyle := tview.Styles.PrimaryTextColor
				detailStyle := tview.Styles.SecondaryTextColor
				if i == selected {
					nameStyle = colorAction
					detailStyle = colorAction
				}
				index := i
				prefix := ""
				if !useSearch && i < 9 {
					prefix = fmt.Sprintf("%d. ", i+1)
				}
				label := prefix + option.Label
				if option.Disabled {
					label += " [no data]"
					nameStyle = colorMuted
					detailStyle = colorMuted
				}
				results.SetCell(i*2, 0, tview.NewTableCell(label).
					SetTextColor(nameStyle).
					SetExpansion(1).
					SetClickedFunc(func() bool {
						selected = index
						render()
						return true
					}))
				results.SetCell(i*2+1, 0, tview.NewTableCell(indentSecondary(option.Description)).
					SetTextColor(detailStyle).
					SetExpansion(1).
					SetClickedFunc(func() bool {
						selected = index
						render()
						return true
					}))
			}
			_, _, _, height := results.GetInnerRect()
			results.SetOffset(searchResultOffsetForSelection(0, selected, len(filtered), height), 0)
			if useSearch {
				status.SetText(fmt.Sprintf("%d candidate(s). Type to filter, Up/Down selects, Enter sets species, Esc closes.", len(filtered)))
			} else {
				status.SetText(fmt.Sprintf("%d candidate(s). 1-9 selects directly, Up/Down selects, Enter sets species, Esc closes.", len(filtered)))
			}
		}
		confirm := func() {
			if loading || selected < 0 || selected >= len(filtered) {
				return
			}
			option := filtered[selected]
			if option.Disabled {
				status.SetText("This species has no CYP data and cannot be selected.")
				return
			}
			if state.ActiveTab == "blast" {
				state.Blast.SpeciesKey = option.Key
				state.Blast.SpeciesLabel = option.Label
			} else {
				if state.Keyword.DatabaseID == "cyp" {
					if state.Keyword.SpeciesMode == "custom" && state.Keyword.Grid.ActiveRow >= 0 && state.Keyword.Grid.ActiveRow < len(state.Keyword.Rows) {
						state.Keyword.Rows[state.Keyword.Grid.ActiveRow].SpeciesKey = option.Key
						state.Keyword.Rows[state.Keyword.Grid.ActiveRow].SpeciesLabel = option.Label
					} else {
						state.Keyword.FallbackSpeciesKey = option.Key
						state.Keyword.FallbackSpeciesLabel = option.Label
					}
					if state.Keyword.SpeciesMode == "all" {
						state.Keyword.SpeciesKey = option.Key
						state.Keyword.SpeciesLabel = option.Label
					}
				} else {
					state.Keyword.SpeciesKey = option.Key
					state.Keyword.SpeciesLabel = option.Label
				}
			}
			confirmed = true
			close()
			rebuild()
		}
		input.SetChangedFunc(func(string) {
			selected = 0
			applyFilter()
			render()
		})
		addButtonRow(modalBody, closeOnlyModalButtons(nil, true, ButtonSelect, "Enter", close, confirm))
		setPageRoot(app, overlayRootOn(root, modalBody, 10000, 10000))
		app.SetFocus(modalBody)
		modalKeyHandler = func(event *tcell.EventKey) bool {
			if event == nil {
				return false
			}
			switch event.Key() {
			case tcell.KeyEscape:
				close()
				return true
			case tcell.KeyEnter:
				confirm()
				return true
			case tcell.KeyUp:
				if selected > 0 {
					selected--
				} else if len(filtered) > 0 {
					selected = len(filtered) - 1
				}
				render()
				return true
			case tcell.KeyDown:
				if len(filtered) > 0 {
					selected = (selected + 1) % len(filtered)
				}
				render()
				return true
			case tcell.KeyBackspace, tcell.KeyBackspace2, tcell.KeyDelete, tcell.KeyLeft, tcell.KeyRight, tcell.KeyHome, tcell.KeyEnd:
				if !useSearch {
					return true
				}
				app.SetFocus(inputFrame)
				if handler := input.InputHandler(); handler != nil {
					handler(event, func(p tview.Primitive) { app.SetFocus(p) })
				}
				return true
			case tcell.KeyRune:
				if event.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 {
					return false
				}
				if !useSearch {
					if event.Rune() >= '1' && event.Rune() <= '9' {
						index := int(event.Rune() - '1')
						if index >= 0 && index < len(filtered) {
							selected = index
							confirm()
						}
					}
					return true
				}
				app.SetFocus(inputFrame)
				if handler := input.InputHandler(); handler != nil {
					handler(event, func(p tview.Primitive) { app.SetFocus(p) })
				}
				return true
			}
			if row, ok := app.GetFocus().(*buttonRowPrimitive); ok {
				if buttonRowHandlesShortcut(row, event) {
					return true
				}
			}
			return false
		}
		go func() {
			loaded, err := page.SpeciesLoader(ctx, request)
			_ = app.QueueUpdateDraw(func() {
				if speciesLoadSeq.Load() != seq || modalKeyHandler == nil {
					return
				}
				loading = false
				if err != nil {
					status.SetTextColor(colorMuted)
					status.SetText("Load failed: " + err.Error())
					results.Clear()
					results.SetCell(0, 0, tview.NewTableCell("- Load failed").SetTextColor(colorMuted).SetSelectable(false))
					results.SetCell(1, 0, tview.NewTableCell("  "+err.Error()).SetTextColor(tview.Styles.SecondaryTextColor).SetSelectable(false))
					return
				}
				options = normalizeMainSpeciesOptions(loaded)
				useSearch = len(options) > 16
				if useSearch {
					modalBody.ResizeItem(inputFrame, 3, 0)
					app.SetFocus(inputFrame)
				} else {
					input.SetText("")
					modalBody.ResizeItem(inputFrame, 0, 0)
					app.SetFocus(modalBody)
				}
				applyFilter()
				render()
			})
		}()
	}

	rebuild = func() {
		state = NormalizeMainInterfaceState(state)
		body = newButtonFlex()
		body.AddItem(mainInterfaceIntro(page.Info), 3, 0, false)
		body.AddItem(nil, 1, 0, false)
		tabContent := newButtonFlex()
		modules = nil
		keywordGrid = nil
		blastGrid = nil
		switch state.ActiveTab {
		case "blast":
			module, groups, grid := buildMainBlastTab(app, &state, showSpeciesModal, rebuild)
			grid.onFocusCell = rebuildActionButtons
			tabContent.AddItem(module, 0, 1, true)
			modules = append(modules, groups...)
			blastGrid = grid
		case "explore":
			module, groups := buildMainExploreTab(app, &state)
			tabContent.AddItem(module, 0, 1, true)
			modules = append(modules, groups...)
		default:
			module, groups, grid := buildMainKeywordTab(app, &state, showSpeciesModal, rebuild)
			grid.onFocusCell = rebuildActionButtons
			tabContent.AddItem(module, 0, 1, true)
			modules = append(modules, groups...)
			keywordGrid = grid
		}
		body.AddItem(newMainTabFrame(tabContent, state.ActiveTab, switchTab, syncModuleFocus), 0, 1, true)
		actionRow = buttonRow()
		rebuildActionButtons()
		addButtonRow(body, actionRow)
		addHints(body, []string{"PgUp/PgDn tabs | Tab modules | Arrows move/select | Space/Enter activates | Ctrl+R runs"})
		root = pageFrame(productName(page.Info)+" > Main", body)
		setPageRoot(app, root)
		focusCurrentModule()
	}

	rebuild()
	installInputCapture(app, func(event *tcell.EventKey) *tcell.EventKey {
		if event == nil {
			return nil
		}
		if modalKeyHandler != nil && modalKeyHandler(event) {
			return nil
		}
		if syncModuleFocus != nil {
			syncModuleFocus(app.GetFocus())
		}
		if lockedDropDown != nil {
			if handleMainDropDownKey(lockedDropDown, event, app) {
				if !lockedDropDown.IsOpen() {
					lockedDropDown = nil
				}
				return nil
			}
			lockedDropDown = nil
		}
		switch event.Key() {
		case tcell.KeyPgUp:
			switchTabDelta(-1)
			return nil
		case tcell.KeyPgDn:
			switchTabDelta(1)
			return nil
		}
		if app.GetFocus() != nil {
			switch {
			case shortcutMatchesEvent("Ctrl+R", event):
				switch state.ActiveTab {
				case "blast":
					validateBlastAndStop()
				case "explore":
					stopWith(MainActionExploreTool)
				default:
					validateKeywordAndStop(MainActionKeywordSearch)
				}
				return nil
			case shortcutMatchesEvent(ShortcutClear, event) && (state.ActiveTab == "keyword" || state.ActiveTab == "blast"):
				clearActiveGrid()
				return nil
			case shortcutMatchesEvent(ShortcutPaste, event) && (state.ActiveTab == "keyword" || state.ActiveTab == "blast"):
				pasteActiveGrid()
				return nil
			case shortcutMatchesEvent("Ctrl+L", event) && activeGrid() != nil && activeGrid().activeColumnIs("symbol"):
				showAliasModal()
				return nil
			case shortcutMatchesEvent(ShortcutWideSearch, event) && state.ActiveTab == "keyword" && mainKeywordSearchType(state.Keyword.DatabaseID).SupportsWide:
				validateKeywordAndStop(MainActionKeywordWideSearch)
				return nil
			}
		}
		if modal, ok := app.GetFocus().(*buttonFlex); ok && modal != body {
			if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyEnter {
				setPageRoot(app, root)
				focusCurrentModule()
				return nil
			}
		}
		focus := app.GetFocus()
		if grid, ok := focus.(*mainGridEditor); ok {
			if grid.HandleKey(event, app) {
				return nil
			}
		}
		if dropDown, ok := focus.(*tview.DropDown); ok {
			if handleMainDropDownKey(dropDown, event, app) {
				if dropDown.IsOpen() {
					lockedDropDown = dropDown
				}
				return nil
			}
		}
		if list, ok := focus.(*mainExploreList); ok {
			if handleMainExploreListKey(list, event, app) {
				return nil
			}
		}
		switch event.Key() {
		case tcell.KeyEscape:
			return nil
		case tcell.KeyTab:
			focusModule(1)
			return nil
		case tcell.KeyBacktab:
			focusModule(-1)
			return nil
		case tcell.KeyLeft, tcell.KeyUp:
			focusWithinModule(-1)
			return nil
		case tcell.KeyRight, tcell.KeyDown:
			focusWithinModule(1)
			return nil
		case tcell.KeyEnter:
			if button, ok := focus.(*mainActionButton); ok {
				button.action()
				return nil
			}
			return nil
		case tcell.KeyRune:
			if event.Rune() == ' ' {
				if button, ok := focus.(*mainActionButton); ok {
					button.action()
					return nil
				}
			}
		}
		if row, ok := focus.(*buttonRowPrimitive); ok {
			if handler := row.InputHandler(); handler != nil {
				handler(event, func(p tview.Primitive) {
					if p != nil {
						app.SetFocus(p)
					}
				})
				return nil
			}
		}
		return event
	})
	if err := runApp(app); err != nil {
		return MainInterfaceResult{}, err
	}
	if strings.TrimSpace(result.Action) == "" {
		result.Action = MainActionExit
		result.State = NormalizeMainInterfaceState(state)
	}
	return result, nil
}

func buildMainKeywordTab(app *tview.Application, state *MainInterfaceState, requestSpecies func(string), refresh func()) (tview.Primitive, []mainModuleFocus, *mainGridEditor) {
	cap := mainKeywordCapability(state.Keyword.DatabaseID)
	searchType := mainKeywordSearchTypeFor(cap, state.Keyword.SearchTypeID)
	module := newButtonFlex()
	options := newButtonFlex()
	options.SetDirection(tview.FlexColumn)
	options.SetBorder(true)
	options.SetTitle(" Search options ")
	options.SetTitleAlign(tview.AlignRight)
	setFocusBorder(options.Box, false)
	attachFocusBorder(options.Box)
	var optionControls []tview.Primitive
	db := mainDropDownWithRefresh("Database", mainKeywordOptions(), state.Keyword.DatabaseID, func(value string) {
		state.Keyword.DatabaseID = value
		state.Keyword.SearchTypeID = mainKeywordSearchType(value).ID
		state.Keyword.GeneLocusPriorityDatabase = GeneLocusPriorityNone
		state.Keyword.PLAZAGeneLocusPriority = false
		state.Keyword.SpeciesLabel = ""
		state.Keyword.SpeciesKey = ""
	}, refresh)
	options.AddItem(newMainControlField("Database", db), 0, 1, true)
	optionControls = append(optionControls, db)
	if len(cap.SearchTypes) > 0 {
		searchDrop := mainDropDownWithRefresh("Search type", mainSearchTypeOptions(cap), searchType.ID, func(value string) {
			state.Keyword.SearchTypeID = value
			if !mainKeywordSearchTypeFor(mainKeywordCapability(state.Keyword.DatabaseID), value).ShowsSpecies {
				state.Keyword.SpeciesLabel = ""
				state.Keyword.SpeciesKey = ""
			}
		}, refresh)
		options.AddItem(newMainControlField("Search type", searchDrop), 0, 1, false)
		optionControls = append(optionControls, searchDrop)
	}
	if state.Keyword.DatabaseID == "ncbi" && searchType.ShowsGeneLocus {
		priority := mainDropDownWithRefresh("Gene locus", mainGeneLocusPriorityOptions(), state.Keyword.GeneLocusPriorityDatabase, func(value string) {
			state.Keyword.GeneLocusPriorityDatabase = normalizeGeneLocusPriorityDatabase(value)
			state.Keyword.PLAZAGeneLocusPriority = state.Keyword.GeneLocusPriorityDatabase == GeneLocusPriorityPLAZA
		}, nil)
		options.AddItem(newMainControlField("Gene locus", priority), 0, 1, false)
		optionControls = append(optionControls, priority)
	} else {
		state.Keyword.GeneLocusPriorityDatabase = GeneLocusPriorityNone
		state.Keyword.PLAZAGeneLocusPriority = false
	}
	if searchType.ShowsSpecies && state.Keyword.DatabaseID != "cyp" {
		button := newMainActionButton("Species", mainSpeciesButtonLabel(state.Keyword.SpeciesLabel), func() {
			if requestSpecies != nil {
				requestSpecies("keyword")
			}
		})
		options.AddItem(newMainControlField("Species", button), 0, 1, false)
		optionControls = append(optionControls, button)
	}
	if state.Keyword.DatabaseID == "cyp" {
		mode := mainDropDownWithRefresh("Species", []Option{{Value: "custom", Label: "Custom"}, {Value: "all", Label: "Set all"}, {Value: "fallback", Label: "Set remaining"}}, state.Keyword.SpeciesMode, func(value string) {
			state.Keyword.SpeciesMode = value
		}, refresh)
		mode.SetSelectedFunc(func(_ string, index int) {
			values := []string{"custom", "all", "fallback"}
			if index < 0 || index >= len(values) {
				return
			}
			value := values[index]
			state.Keyword.SpeciesMode = value
			if value != "custom" && requestSpecies != nil {
				requestSpecies("keyword")
			} else if refresh != nil {
				refresh()
			}
		})
		options.AddItem(newMainControlField("Species", mode), 0, 1, false)
		optionControls = append(optionControls, mode)
		if state.Keyword.SpeciesMode != "custom" {
			// The selected species button remains visible as the all/fallback target.
			state.Keyword.SpeciesLabel = state.Keyword.FallbackSpeciesLabel
		}
	}
	module.AddItem(options, mainOptionsHeight(searchType.ShowsSpecies || state.Keyword.DatabaseID == "cyp", len(cap.SearchTypes) > 0), 0, false)

	status := hintView("")
	grid := newKeywordGridEditor(&state.Keyword.Rows, &state.Keyword.Grid, status, searchType, state.Keyword.SpeciesMode)
	grid.onSpecialCell = func(row int, column string) {
		if column == "species" && state.Keyword.DatabaseID == "cyp" {
			state.Keyword.Grid.ActiveRow = row
			if requestSpecies != nil {
				requestSpecies("keyword")
			}
		}
	}
	content := newButtonFlex()
	content.SetBorder(true)
	content.SetTitle(" Search content ")
	content.SetTitleAlign(tview.AlignRight)
	setFocusBorder(content.Box, false)
	attachFocusBorder(content.Box)
	content.AddItem(grid, 0, 1, true)
	module.AddItem(content, 0, 1, true)
	return module, []mainModuleFocus{{box: options.Box, controls: optionControls}, {box: content.Box, controls: []tview.Primitive{grid}}}, grid
}

func buildMainBlastTab(app *tview.Application, state *MainInterfaceState, requestSpecies func(string), refresh func()) (tview.Primitive, []mainModuleFocus, *mainGridEditor) {
	cap := mainBlastCapability(state.Blast.DatabaseID)
	module := newButtonFlex()
	options := newButtonFlex()
	options.SetDirection(tview.FlexColumn)
	options.SetBorder(true)
	options.SetTitle(" BLAST options ")
	options.SetTitleAlign(tview.AlignRight)
	setFocusBorder(options.Box, false)
	attachFocusBorder(options.Box)
	var optionControls []tview.Primitive
	db := mainDropDownWithRefresh("Database", mainBlastOptions(), state.Blast.DatabaseID, func(value string) {
		state.Blast.DatabaseID = value
		state.Blast.SpeciesLabel = ""
		state.Blast.SpeciesKey = ""
		cap := mainBlastCapability(value)
		state.Blast.ProgramID = firstString(cap.BlastPrograms, "blastn")
	}, refresh)
	options.AddItem(newMainControlField("Database", db), 0, 1, true)
	optionControls = append(optionControls, db)
	if cap.ShowsSpecies {
		button := newMainActionButton("Species", mainSpeciesButtonLabel(state.Blast.SpeciesLabel), func() {
			if requestSpecies != nil {
				requestSpecies("blast")
			}
		})
		options.AddItem(newMainControlField("Species", button), 0, 1, false)
		optionControls = append(optionControls, button)
	}
	if len(cap.BlastPrograms) > 0 {
		programDrop := mainDropDownWithRefresh("Program", mainProgramOptions(cap.BlastPrograms), state.Blast.ProgramID, func(value string) {
			state.Blast.ProgramID = value
		}, refresh)
		options.AddItem(newMainControlField("Program", programDrop), 0, 1, false)
		optionControls = append(optionControls, programDrop)
	}
	module.AddItem(options, mainOptionsHeight(cap.ShowsSpecies, len(cap.BlastPrograms) > 0), 0, false)

	status := hintView("")
	grid := newBlastGridEditor(&state.Blast.Rows, &state.Blast.Grid, status, cap)
	content := newButtonFlex()
	content.SetBorder(true)
	content.SetTitle(" BLAST content ")
	content.SetTitleAlign(tview.AlignRight)
	setFocusBorder(content.Box, false)
	attachFocusBorder(content.Box)
	content.AddItem(grid, 0, 1, true)
	module.AddItem(content, 0, 1, true)
	return module, []mainModuleFocus{{box: options.Box, controls: optionControls}, {box: content.Box, controls: []tview.Primitive{grid}}}, grid
}

func buildMainExploreTab(_ *tview.Application, state *MainInterfaceState) (tview.Primitive, []mainModuleFocus) {
	options := []Option{
		{Value: "open_session", Label: "Open session", Description: "open a saved .pgo session snapshot"},
		{Value: "new_canvas", Label: "New canvas", Description: "create a blank canvas workspace"},
		{Value: "nwk_browser", Label: "Tree viewer browser", Description: "open a .nwk or .pgv file in the local tree viewer"},
		// Temporarily disabled until these discovery workflows are reliable:
		// {Value: "tair_family", Label: "TAIR database family index", Description: "TAIR family index search"},
		// {Value: "pathway_search", Label: "Pathway search", Description: "pathway-guided protein discovery entry point"},
	}
	list := optionListWithStart("", options, 1)
	list.SetBorder(false)
	list.SetTitle("")
	list.SetWrapAround(false)
	for i, option := range options {
		if option.Value == state.Explore.Tool {
			list.SetCurrentItem(i)
			break
		}
	}
	list.SetChangedFunc(func(index int, _ string, _ string, _ rune) {
		if index >= 0 && index < len(options) {
			state.Explore.Tool = options[index].Value
		}
	})
	list.SetSelectedFunc(func(index int, _ string, _ string, _ rune) {
		if index >= 0 && index < len(options) {
			state.Explore.Tool = options[index].Value
		}
	})
	exploreList := &mainExploreList{List: list}
	module := newButtonFlex()
	module.SetBorder(true)
	module.SetTitle("")
	setFocusBorder(module.Box, false)
	attachFocusBorder(module.Box)
	module.AddItem(exploreList, 0, 1, true)
	return module, []mainModuleFocus{{box: module.Box, controls: []tview.Primitive{exploreList}}}
}

func newKeywordGridEditor(rows *[]MainKeywordRow, state *GridEditorState, status *tview.TextView, capability mainSearchTypeCapability, speciesMode ...string) *mainGridEditor {
	ensure := func(n int) {
		for len(*rows) < n {
			*rows = append(*rows, MainKeywordRow{})
		}
	}
	clean := func() {
		cleaned := MainKeywordRowsForDisplay(*rows)
		*rows = cleaned
	}
	cols := []gridColumn{
		{ID: "term", Title: "Search term", Weight: 3, Get: func(i int) string { return (*rows)[i].SearchTerm }, Set: func(i int, v string) { (*rows)[i].SearchTerm = v }},
	}
	if capability.ShowsSymbolName {
		cols = append(cols, gridColumn{ID: "symbol", Title: "Symbol name", Weight: 2, Get: func(i int) string { return (*rows)[i].SymbolName }, Set: func(i int, v string) { (*rows)[i].SymbolName = v }})
	}
	if capability.ShowsGeneLocus {
		cols = append(cols, gridColumn{ID: "locus", Title: "Gene locus", Weight: 2, Get: func(i int) string { return (*rows)[i].GeneLocus }, Set: func(i int, v string) { (*rows)[i].GeneLocus = v }})
	}
	showCYPRowSpecies := capability.ID == "cyp" && (len(speciesMode) == 0 || speciesMode[0] != "all")
	if showCYPRowSpecies {
		cols = append(cols, gridColumn{ID: "species", Title: "Species", Weight: 3, ReadOnly: true, Get: func(i int) string {
			row := (*rows)[i]
			if strings.TrimSpace(row.SearchTerm) == "" && strings.TrimSpace(row.SymbolName) == "" && strings.TrimSpace(row.GeneLocus) == "" {
				return ""
			}
			if value := strings.TrimSpace(row.SpeciesLabel); value != "" {
				return value
			}
			return "[ set species... ]"
		}, Set: func(int, string) {}})
	}
	grid := newMainGridEditor(cols, func() int { return len(*rows) }, ensure, clean, state, status)
	grid.enterRows = true
	return grid
}

func newBlastGridEditor(rows *[]MainBlastRow, state *GridEditorState, status *tview.TextView, capability mainCapability) *mainGridEditor {
	ensure := func(n int) {
		for len(*rows) < n {
			*rows = append(*rows, MainBlastRow{})
		}
	}
	clean := func() {
		cleaned := MainBlastRowsForDisplay(*rows)
		*rows = cleaned
	}
	cols := []gridColumn{
		{ID: "fasta", Title: "FASTA", Weight: 4, KeepNL: true, Wrap: true, Get: func(i int) string { return (*rows)[i].FASTA }, Set: func(i int, v string) { (*rows)[i].FASTA = v }},
	}
	if capability.ShowsSymbolName {
		cols = append(cols, gridColumn{ID: "symbol", Title: "Symbol name", Weight: 2, Get: func(i int) string { return (*rows)[i].SymbolName }, Set: func(i int, v string) { (*rows)[i].SymbolName = v }})
	}
	if capability.ShowsGeneLocus {
		cols = append(cols, gridColumn{ID: "locus", Title: "Gene locus", Weight: 2, Get: func(i int) string { return (*rows)[i].GeneLocus }, Set: func(i int, v string) { (*rows)[i].GeneLocus = v }})
	}
	grid := newMainGridEditor(cols, func() int { return len(*rows) }, ensure, clean, state, status)
	grid.fastaGrid = true
	grid.pasteLines = func(text string, activeCol int) []string {
		if activeCol == 0 {
			return splitMainBlastPasteRecords(text)
		}
		return splitMainPasteLines(text)
	}
	return grid
}

func newMainGridEditor(columns []gridColumn, rowCount func() int, ensureRows func(int), cleanRows func(), state *GridEditorState, status *tview.TextView) *mainGridEditor {
	if state == nil {
		state = &GridEditorState{}
	}
	editor := &mainGridEditor{
		Box:         tview.NewBox().SetBorder(false),
		columns:     columns,
		rowCount:    rowCount,
		ensureRows:  ensureRows,
		cleanRows:   cleanRows,
		state:       state,
		caretByCell: make(map[string]int),
		status:      status,
	}
	editor.ensureRows(1)
	editor.normalizeCursor()
	return editor
}

func MainKeywordRowsForDisplay(rows []MainKeywordRow) []MainKeywordRow {
	out := make([]MainKeywordRow, 0, len(rows)+1)
	for _, row := range rows {
		normalized := MainKeywordRow{
			SearchTerm:   normalizeMainGridValue(row.SearchTerm),
			SymbolName:   normalizeMainGridValue(row.SymbolName),
			GeneLocus:    normalizeMainGridValue(row.GeneLocus),
			SpeciesKey:   normalizeMainSpeciesKey(row.SpeciesKey),
			SpeciesLabel: normalizeMainSpeciesValue(row.SpeciesLabel),
			Aliases:      mainAliasChoices(row.SymbolName, row.Aliases),
		}
		if normalized.SearchTerm == "" && normalized.SymbolName == "" && normalized.GeneLocus == "" {
			continue
		}
		out = append(out, normalized)
	}
	out = append(out, MainKeywordRow{})
	return out
}

func MainBlastRowsForDisplay(rows []MainBlastRow) []MainBlastRow {
	out := make([]MainBlastRow, 0, len(rows)+1)
	for _, row := range rows {
		normalized := MainBlastRow{
			FASTA:      normalizeMainGridValueForColumn(row.FASTA, gridColumn{KeepNL: true}),
			SymbolName: normalizeMainGridValue(row.SymbolName),
			GeneLocus:  normalizeMainGridValue(row.GeneLocus),
			Aliases:    mainAliasChoices(row.SymbolName, row.Aliases),
		}
		if normalized.FASTA == "" && normalized.SymbolName == "" && normalized.GeneLocus == "" {
			continue
		}
		out = append(out, normalized)
	}
	out = append(out, MainBlastRow{})
	return out
}

func (g *mainGridEditor) Draw(screen tcell.Screen) {
	g.Box.DrawForSubclass(screen, g)
	x, y, width, height := g.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}
	g.normalizeCursor()
	rowColWidth := g.rowColumnWidth()
	colWidths := g.columnWidths(maxInt(1, width-rowColWidth-1))
	headerStyle := tcell.StyleDefault.Foreground(tview.Styles.PrimaryTextColor).Bold(true)
	cellStyle := tcell.StyleDefault.Foreground(tview.Styles.PrimaryTextColor)
	activeStyle := tcell.StyleDefault.Foreground(colorActionText).Background(colorAction).Bold(true)
	printMainGridCell(screen, x, y, rowColWidth, headerStyle, "row")
	dividerStyle := tcell.StyleDefault.Foreground(colorInactiveText).Background(tview.Styles.PrimitiveBackgroundColor)
	drawGridDivider := func(dividerX int, top int, h int) {
		for yy := top; yy < top+h; yy++ {
			screen.SetContent(dividerX, yy, tview.Borders.Vertical, nil, dividerStyle)
		}
	}
	drawGridDivider(x+rowColWidth, y, height)
	cx := x + rowColWidth + 1
	for c, col := range g.columns {
		if c < g.colOffset {
			continue
		}
		w := colWidths[c]
		if cx+w > x+width {
			break
		}
		printMainGridCell(screen, cx, y, w, headerStyle, strings.ReplaceAll(col.Title, "\n", " "))
		drawGridDivider(cx+w, y, height)
		cx += w + 1
	}
	dividerY := y + 1
	if dividerY < y+height {
		for xx := x; xx < x+width; xx++ {
			screen.SetContent(xx, dividerY, tview.Borders.Horizontal, nil, dividerStyle)
		}
	}
	drawY := y + 2
	for row := g.rowOffset; row < g.rowCount() && drawY < y+height; row++ {
		rowHeight := g.rowVisualHeight(row, colWidths)
		if drawY+rowHeight > y+height {
			break
		}
		printMainGridCell(screen, x, drawY, rowColWidth, cellStyle, g.rowDisplayNumber(row))
		cx = x + rowColWidth + 1
		for c := range g.columns {
			if c < g.colOffset {
				continue
			}
			w := colWidths[c]
			if cx+w > x+width {
				break
			}
			contentWidth := mainGridContentWidth(w)
			lines := g.cellVisualLines(row, c, contentWidth)
			topPad := maxInt(0, (rowHeight-len(lines))/2)
			style := cellStyle
			if row == g.state.ActiveRow && c == g.state.ActiveCol {
				style = activeStyle
			}
			for i, line := range lines {
				printMainGridCell(screen, cx, drawY+topPad+i, w, style, line)
			}
			if row == g.state.ActiveRow && c == g.state.ActiveCol && len(lines) < rowHeight {
				for pad := 0; pad < rowHeight; pad++ {
					if pad >= topPad && pad < topPad+len(lines) {
						continue
					}
					printMainGridCell(screen, cx, drawY+pad, w, style, "")
				}
			}
			cx += w + 1
		}
		drawY += rowHeight
	}
	if g.HasFocus() {
		g.drawCursor(screen, x, y, width, colWidths)
	}
	g.updateStatus()
}

func mainGridCellText(text string) string {
	return "  " + text + "  "
}

func printMainGridCell(screen tcell.Screen, x int, y int, width int, style tcell.Style, text string) {
	if width <= 0 {
		return
	}
	runes := []rune(text)
	contentWidth := mainGridContentWidth(width)
	if len(runes) > contentWidth {
		if contentWidth <= 3 {
			runes = runes[:contentWidth]
		} else {
			runes = append(runes[:contentWidth-3], '.', '.', '.')
		}
	}
	out := make([]rune, width)
	for i := range out {
		out[i] = ' '
	}
	offset := mainGridContentOffset()
	for i, r := range runes {
		if offset+i >= 0 && offset+i < len(out) {
			out[offset+i] = r
		}
	}
	for i, r := range out {
		screen.SetContent(x+i, y, r, nil, style)
	}
}

func mainGridContentWidth(width int) int {
	return maxInt(1, width-4)
}

func mainGridContentOffset() int {
	return 2
}

func (g *mainGridEditor) rowScreenY(row int, colWidths []int) int {
	_, y, _, height := g.GetInnerRect()
	drawY := y + 2
	for r := g.rowOffset; r < g.rowCount() && drawY < y+height; r++ {
		if r == row {
			return drawY
		}
		drawY += g.rowVisualHeight(r, colWidths)
	}
	return -1
}

func (g *mainGridEditor) rowColumnWidth() int {
	digits := len(fmt.Sprintf("%d", maxInt(1, g.rowCount())))
	return maxInt(taggedPaddedWidth("row"), taggedPaddedWidth(strings.Repeat("9", digits)))
}

func (g *mainGridEditor) rowDisplayNumber(row int) string {
	if row < 0 || row >= g.rowCount() || !g.rowHasContent(row) {
		return ""
	}
	n := 0
	for i := 0; i <= row; i++ {
		if g.rowHasContent(i) {
			n++
		}
	}
	return fmt.Sprintf("%d", n)
}

func (g *mainGridEditor) rowHasContent(row int) bool {
	if row < 0 || row >= g.rowCount() {
		return false
	}
	for _, col := range g.columns {
		if strings.TrimSpace(col.Get(row)) != "" {
			return true
		}
	}
	return false
}

func (g *mainGridEditor) rowVisualHeight(row int, colWidths []int) int {
	height := 1
	for c := range g.columns {
		width := 1
		if c >= 0 && c < len(colWidths) {
			width = colWidths[c]
		}
		height = maxInt(height, len(g.cellVisualLines(row, c, mainGridContentWidth(width))))
	}
	return height
}

func (g *mainGridEditor) cellVisualLines(row int, col int, width int) []string {
	segments := g.cellVisualSegments(row, col, width)
	lines := make([]string, len(segments))
	for i, segment := range segments {
		lines[i] = segment.Text
	}
	return lines
}

func (g *mainGridEditor) cellVisualSegments(row int, col int, width int) []gridVisualLine {
	if row < 0 || row >= g.rowCount() || col < 0 || col >= len(g.columns) {
		return []gridVisualLine{{}}
	}
	width = maxInt(1, width)
	value := g.columns[col].Get(row)
	if !g.columns[col].Wrap {
		text := strings.ReplaceAll(value, "\n", " ")
		return []gridVisualLine{{Text: text, Start: 0, End: utf8.RuneCountInString(value)}}
	}
	value = strings.ReplaceAll(value, "\r", "")
	parts := strings.Split(value, "\n")
	lines := make([]gridVisualLine, 0, len(parts))
	offset := 0
	for partIndex, part := range parts {
		runes := []rune(part)
		if len(runes) == 0 {
			lines = append(lines, gridVisualLine{Start: offset, End: offset})
			if partIndex < len(parts)-1 {
				offset++
			}
			continue
		}
		partOffset := 0
		for len(runes) > 0 {
			take := minInt(width, len(runes))
			lines = append(lines, gridVisualLine{
				Text:  string(runes[:take]),
				Start: offset + partOffset,
				End:   offset + partOffset + take,
			})
			runes = runes[take:]
			partOffset += take
		}
		offset += utf8.RuneCountInString(part)
		if partIndex < len(parts)-1 {
			offset++
		}
	}
	if len(lines) == 0 {
		return []gridVisualLine{{}}
	}
	return lines
}

func (g *mainGridEditor) caretVisualPosition(width int) (line int, col int) {
	segments := g.cellVisualSegments(g.state.ActiveRow, g.state.ActiveCol, width)
	caret := clampInt(g.caret(), 0, utf8.RuneCountInString(g.currentValue()))
	for i, segment := range segments {
		if caret >= segment.Start && caret <= segment.End {
			return i, minInt(caret-segment.Start, utf8.RuneCountInString(segment.Text))
		}
		if caret < segment.Start {
			return i, 0
		}
	}
	last := segments[len(segments)-1]
	return len(segments) - 1, utf8.RuneCountInString(last.Text)
}

func (g *mainGridEditor) setCaretFromVisualPosition(targetLine int, targetCol int, width int) {
	segments := g.cellVisualSegments(g.state.ActiveRow, g.state.ActiveCol, width)
	targetLine = clampInt(targetLine, 0, len(segments)-1)
	segment := segments[targetLine]
	offset := segment.Start + minInt(maxInt(0, targetCol), utf8.RuneCountInString(segment.Text))
	g.setCaret(minInt(offset, utf8.RuneCountInString(g.currentValue())))
}

func (g *mainGridEditor) moveVisualUp() {
	width := g.activeColumnWidth()
	line, col := g.caretVisualPosition(width)
	if line > 0 {
		g.setCaretFromVisualPosition(line-1, col, width)
		return
	}
	if g.state.ActiveRow > 0 {
		g.state.ActiveRow--
		g.setCaretFromVisualPosition(len(g.cellVisualLines(g.state.ActiveRow, g.state.ActiveCol, width))-1, col, width)
	}
}

func (g *mainGridEditor) moveVisualDown() {
	width := g.activeColumnWidth()
	line, col := g.caretVisualPosition(width)
	lines := g.cellVisualLines(g.state.ActiveRow, g.state.ActiveCol, width)
	if line < len(lines)-1 {
		g.setCaretFromVisualPosition(line+1, col, width)
		return
	}
	g.state.ActiveRow++
	if g.state.ActiveRow >= g.rowCount() {
		g.ensureRows(g.state.ActiveRow + 1)
	}
	g.setCaretFromVisualPosition(0, col, width)
}

func (g *mainGridEditor) activeColumnWidth() int {
	_, _, width, _ := g.GetInnerRect()
	widths := g.columnWidths(maxInt(1, width-g.rowColumnWidth()-1))
	if g.state.ActiveCol >= 0 && g.state.ActiveCol < len(widths) {
		return mainGridContentWidth(widths[g.state.ActiveCol])
	}
	return 1
}

func (g *mainGridEditor) activeColumnIs(id string) bool {
	if g == nil || g.state == nil || g.state.ActiveCol < 0 || g.state.ActiveCol >= len(g.columns) {
		return false
	}
	return strings.EqualFold(g.columns[g.state.ActiveCol].ID, id)
}

func (g *mainGridEditor) notifyFocusCell() {
	if g != nil && g.onFocusCell != nil {
		g.onFocusCell()
	}
}

func (g *mainGridEditor) HandleKey(event *tcell.EventKey, app *tview.Application) bool {
	if event == nil {
		return false
	}
	g.normalizeCursor()
	readOnly := g.state.ActiveCol >= 0 && g.state.ActiveCol < len(g.columns) && g.columns[g.state.ActiveCol].ReadOnly
	switch event.Key() {
	case tcell.KeyUp:
		g.moveVisualUp()
		g.normalizeCursor()
		g.notifyFocusCell()
		return true
	case tcell.KeyDown:
		g.moveVisualDown()
		g.normalizeCursor()
		g.notifyFocusCell()
		return true
	case tcell.KeyLeft:
		g.moveLeft()
		g.notifyFocusCell()
		return true
	case tcell.KeyRight:
		g.moveRight()
		g.notifyFocusCell()
		return true
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if readOnly {
			return true
		}
		g.backspace()
		return true
	case tcell.KeyDelete:
		if readOnly {
			return true
		}
		g.deleteAtCaret()
		return true
	case tcell.KeyHome:
		if readOnly {
			return true
		}
		g.setCaret(0)
		return true
	case tcell.KeyEnd:
		if readOnly {
			return true
		}
		g.setCaret(utf8.RuneCountInString(g.currentValue()))
		return true
	case tcell.KeyEnter:
		if event.Modifiers()&tcell.ModCtrl != 0 {
			return false
		}
		if g.activeColumnKeepsNewlines() {
			g.insertText("\n")
			return true
		}
		if g.enterRows {
			if g.onSpecialCell != nil && g.state.ActiveCol >= 0 && g.state.ActiveCol < len(g.columns) {
				if g.columns[g.state.ActiveCol].ID == "species" {
					g.onSpecialCell(g.state.ActiveRow, "species")
					return true
				}
			}
			line, col := g.caretVisualPosition(g.activeColumnWidth())
			_ = line
			g.state.ActiveRow++
			if g.state.ActiveRow >= g.rowCount() {
				g.ensureRows(g.state.ActiveRow + 1)
			}
			g.setCaretFromVisualPosition(0, col, g.activeColumnWidth())
			g.normalizeCursor()
			g.notifyFocusCell()
			return true
		}
	case tcell.KeyCtrlJ:
		return false
	case tcell.KeyRune:
		if event.Rune() == ' ' {
			if g.onSpecialCell != nil && g.state.ActiveCol >= 0 && g.state.ActiveCol < len(g.columns) && g.columns[g.state.ActiveCol].ID == "species" {
				g.onSpecialCell(g.state.ActiveRow, "species")
				return true
			}
			g.nextColumn()
			g.notifyFocusCell()
			return true
		}
		if event.Rune() == '\t' {
			return false
		}
		if readOnly {
			return true
		}
		if event.Rune() == 0 || event.Rune() < 32 {
			return false
		}
		if strings.ContainsRune("\r\n", event.Rune()) {
			return true
		}
		g.insertText(string(event.Rune()))
		return true
	case tcell.KeyCtrlV:
		if readOnly {
			return true
		}
		g.PasteClipboard(app)
		return true
	}
	return false
}

func (g *mainGridEditor) activeColumnKeepsNewlines() bool {
	if g == nil || g.state == nil || g.state.ActiveCol < 0 || g.state.ActiveCol >= len(g.columns) {
		return false
	}
	return g.columns[g.state.ActiveCol].KeepNL
}

func (g *mainGridEditor) PasteClipboard(app *tview.Application) {
	runInlinePaste(app, newPasteStatus(func() { app.SetFocus(g) }), func(text string) {
		g.PasteText(text)
	})
}

func (g *mainGridEditor) PasteText(text string) {
	if g != nil && g.state != nil && g.state.ActiveCol >= 0 && g.state.ActiveCol < len(g.columns) && g.columns[g.state.ActiveCol].ReadOnly {
		return
	}
	lines := splitMainPasteLines(text)
	if g.pasteLines != nil {
		lines = g.pasteLines(text, g.state.ActiveCol)
	}
	if len(lines) == 0 {
		return
	}
	start := g.state.ActiveRow
	g.ensureRows(start + len(lines))
	for i, line := range lines {
		line = normalizeMainGridValueForColumn(line, g.columns[g.state.ActiveCol])
		g.columns[g.state.ActiveCol].Set(start+i, line)
		g.setCaretFor(start+i, g.state.ActiveCol, utf8.RuneCountInString(line))
	}
	g.state.ActiveRow = start + len(lines) - 1
	g.cleanRows()
	g.normalizeCursor()
}

func splitMainPasteLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func splitMainBlastPasteRecords(text string) []string {
	text = strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	records := make([]string, 0, 4)
	current := make([]string, 0, len(lines))
	currentFASTA := false
	flush := func() {
		if len(current) == 0 {
			return
		}
		record := strings.TrimSpace(strings.Join(current, "\n"))
		if record != "" {
			records = append(records, record)
		}
		current = current[:0]
		currentFASTA = false
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, ">") {
			flush()
			current = append(current, line)
			currentFASTA = true
			continue
		}
		if currentFASTA {
			current = append(current, line)
			continue
		}
		if isMainBlastPlainSequenceLine(line) {
			current = append(current, line)
			continue
		}
		flush()
		records = append(records, splitMainBlastPasteTokens(line)...)
	}
	flush()
	return records
}

func isMainBlastPlainSequenceLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, ">") || strings.ContainsAny(line, " \t") {
		return false
	}
	hasLetter := false
	for _, r := range line {
		switch {
		case r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= 'a' && r <= 'z':
			hasLetter = true
		case strings.ContainsRune("*.-?", r):
		default:
			return false
		}
	}
	return hasLetter
}

func splitMainBlastPasteTokens(line string) []string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		out = append(out, field)
	}
	return out
}

func (g *mainGridEditor) Focus(delegate func(p tview.Primitive)) {
	g.Box.Focus(delegate)
}

func (g *mainGridEditor) HasFocus() bool {
	return g.Box.HasFocus()
}

func (g *mainGridEditor) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return g.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if event == nil || !g.InRect(event.Position()) {
			return false, nil
		}
		switch action {
		case tview.MouseScrollLeft:
			g.scrollColumns(-1)
			return true, g
		case tview.MouseScrollRight:
			g.scrollColumns(1)
			return true, g
		case tview.MouseScrollUp:
			if event.Modifiers()&tcell.ModShift != 0 {
				g.scrollColumns(-1)
				return true, g
			}
			g.scrollRows(-1)
			return true, g
		case tview.MouseScrollDown:
			if event.Modifiers()&tcell.ModShift != 0 {
				g.scrollColumns(1)
				return true, g
			}
			g.scrollRows(1)
			return true, g
		}
		if action == tview.MouseLeftClick {
			setFocus(g)
			x, y, width, _ := g.GetInnerRect()
			mx, my := event.Position()
			rowColWidth := g.rowColumnWidth()
			colWidths := g.columnWidths(maxInt(1, width-rowColWidth-1))
			if my <= y {
				return true, g
			}
			row, rowTop, ok := g.rowAtVisualY(my, y, colWidths)
			if ok {
				g.ensureRows(row + 1)
				g.state.ActiveRow = row
			}
			cx := x + rowColWidth + 1
			selectedColX := cx
			for c := range g.columns {
				if c < g.colOffset {
					continue
				}
				w := colWidths[c]
				if mx >= cx && mx < cx+w {
					g.state.ActiveCol = c
					selectedColX = cx
					break
				}
				cx += w + 1
			}
			g.normalizeCursor()
			if ok {
				w := colWidths[g.state.ActiveCol]
				rowHeight := g.rowVisualHeight(g.state.ActiveRow, colWidths)
				contentWidth := mainGridContentWidth(w)
				lines := g.cellVisualLines(g.state.ActiveRow, g.state.ActiveCol, contentWidth)
				topPad := maxInt(0, (rowHeight-len(lines))/2)
				targetLine := my - rowTop - topPad
				targetCol := mx - selectedColX - mainGridContentOffset()
				g.setCaretFromVisualPosition(targetLine, targetCol, contentWidth)
				if g.onSpecialCell != nil && g.state.ActiveCol >= 0 && g.state.ActiveCol < len(g.columns) && g.columns[g.state.ActiveCol].ID == "species" {
					g.onSpecialCell(g.state.ActiveRow, "species")
				}
			}
			g.notifyFocusCell()
			return true, g
		}
		return false, nil
	})
}

func (g *mainGridEditor) scrollRows(delta int) {
	if g == nil || delta == 0 {
		return
	}
	g.normalizeCursor()
	g.rowOffset = clampInt(g.rowOffset+delta, 0, maxInt(0, g.rowCount()-1))
}

func (g *mainGridEditor) scrollColumns(delta int) {
	if g == nil || delta == 0 || len(g.columns) == 0 {
		return
	}
	g.normalizeCursor()
	g.colOffset = clampInt(g.colOffset+delta, 0, maxInt(0, len(g.columns)-1))
}

func (g *mainGridEditor) rowAtVisualY(screenY int, innerY int, colWidths []int) (row int, rowTop int, ok bool) {
	drawY := innerY + 2
	for r := g.rowOffset; r < g.rowCount(); r++ {
		rowHeight := g.rowVisualHeight(r, colWidths)
		if screenY >= drawY && screenY < drawY+rowHeight {
			return r, drawY, true
		}
		drawY += rowHeight
	}
	return 0, 0, false
}

func (g *mainGridEditor) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return g.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		_ = g.HandleKey(event, nil)
	})
}

func (g *mainGridEditor) normalizeCursor() {
	g.ensureRows(1)
	if g.state.ActiveRow < 0 {
		g.state.ActiveRow = 0
	}
	if g.state.ActiveRow >= g.rowCount() {
		g.ensureRows(g.state.ActiveRow + 1)
	}
	if g.state.ActiveCol < 0 {
		g.state.ActiveCol = 0
	}
	if g.state.ActiveCol >= len(g.columns) {
		g.state.ActiveCol = len(g.columns) - 1
	}
	g.setCaret(minInt(g.caret(), utf8.RuneCountInString(g.currentValue())))
	if g.state.ActiveRow < g.rowOffset {
		g.rowOffset = g.state.ActiveRow
	}
	_, _, width, height := g.GetInnerRect()
	colWidths := g.columnWidths(maxInt(1, width-g.rowColumnWidth()-1))
	if height > 1 {
		for g.rowScreenY(g.state.ActiveRow, colWidths) < 0 && g.rowOffset < g.state.ActiveRow {
			g.rowOffset++
		}
	}
	if g.state.ActiveCol < g.colOffset {
		g.colOffset = g.state.ActiveCol
	}
}

func (g *mainGridEditor) columnWidths(width int) []int {
	if len(g.columns) == 0 {
		return nil
	}
	widths := make([]int, len(g.columns))
	for i, col := range g.columns {
		if g.fastaGrid && col.ID == "fasta" {
			widths[i] = maxInt(utf8.RuneCountInString(col.Title), width/2)
			continue
		}
		if col.Fixed > 0 {
			widths[i] = col.Fixed
			continue
		}
		maxLen := taggedPaddedWidth(col.Title)
		for row := 0; row < g.rowCount(); row++ {
			for _, line := range strings.Split(strings.ReplaceAll(col.Get(row), "\r", ""), "\n") {
				maxLen = maxInt(maxLen, taggedPaddedWidth(line))
			}
		}
		widths[i] = maxInt(1, minInt(maxLen, maxInt(1, width)))
	}
	return widths
}

func (g *mainGridEditor) drawCursor(screen tcell.Screen, x, y, width int, colWidths []int) {
	if g.state.ActiveRow < g.rowOffset {
		return
	}
	rowY := g.rowScreenY(g.state.ActiveRow, colWidths)
	_, _, _, height := g.GetInnerRect()
	if rowY < y+1 || rowY >= y+height {
		return
	}
	cx := x + g.rowColumnWidth() + 1
	for c := range g.columns {
		if c < g.colOffset {
			continue
		}
		w := colWidths[c]
		if cx+w > x+width {
			break
		}
		if c == g.state.ActiveCol {
			contentWidth := mainGridContentWidth(w)
			line, col := g.caretVisualPosition(contentWidth)
			rowHeight := g.rowVisualHeight(g.state.ActiveRow, colWidths)
			lines := g.cellVisualLines(g.state.ActiveRow, c, contentWidth)
			topPad := maxInt(0, (rowHeight-len(lines))/2)
			visualLen := 0
			if line >= 0 && line < len(lines) {
				visualLen = utf8.RuneCountInString(lines[line])
			}
			cursorCol := minInt(maxInt(0, col), maxInt(0, contentWidth-1))
			cursorX := cx + mainGridContentOffset() + cursorCol
			cursorY := rowY + topPad + line
			if cursorY >= y+2 && cursorY < y+height {
				mainc, combc, style, _ := screen.GetContent(cursorX, cursorY)
				if mainc == 0 || cursorCol >= visualLen {
					mainc = ' '
					combc = nil
				}
				style = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite).Bold(true)
				screen.SetContent(cursorX, cursorY, mainc, combc, style)
			}
			return
		}
		cx += w + 1
	}
}

func (g *mainGridEditor) currentValue() string {
	return g.columns[g.state.ActiveCol].Get(g.state.ActiveRow)
}

func (g *mainGridEditor) setCurrentValue(value string) {
	g.columns[g.state.ActiveCol].Set(g.state.ActiveRow, normalizeMainGridEditValueForColumn(value, g.columns[g.state.ActiveCol]))
}

func (g *mainGridEditor) cellKey(row, col int) string {
	return fmt.Sprintf("%d:%d", row, col)
}

func (g *mainGridEditor) caret() int {
	return g.caretByCell[g.cellKey(g.state.ActiveRow, g.state.ActiveCol)]
}

func (g *mainGridEditor) setCaret(pos int) {
	g.setCaretFor(g.state.ActiveRow, g.state.ActiveCol, pos)
}

func (g *mainGridEditor) setCaretFor(row, col, pos int) {
	if pos < 0 {
		pos = 0
	}
	g.caretByCell[g.cellKey(row, col)] = pos
}

func (g *mainGridEditor) moveLeft() {
	if g.caret() > 0 {
		g.setCaret(g.caret() - 1)
		return
	}
	if g.state.ActiveCol > 0 {
		g.state.ActiveCol--
		g.setCaret(utf8.RuneCountInString(g.currentValue()))
	}
}

func (g *mainGridEditor) moveRight() {
	if g.caret() < utf8.RuneCountInString(g.currentValue()) {
		g.setCaret(g.caret() + 1)
		return
	}
	if g.state.ActiveCol < len(g.columns)-1 {
		g.state.ActiveCol++
		g.setCaret(0)
	}
}

func (g *mainGridEditor) nextColumn() {
	g.state.ActiveCol = (g.state.ActiveCol + 1) % len(g.columns)
	g.setCaret(minInt(g.caret(), utf8.RuneCountInString(g.currentValue())))
}

func (g *mainGridEditor) insertText(text string) {
	text = strings.ReplaceAll(text, " ", "")
	text = strings.ReplaceAll(text, "\t", "")
	if g.activeColumnKeepsNewlines() {
		text = strings.ReplaceAll(text, "\r\n", "\n")
		text = strings.ReplaceAll(text, "\r", "\n")
	} else {
		text = strings.ReplaceAll(text, "\n", "")
		text = strings.ReplaceAll(text, "\r", "")
	}
	if text == "" {
		return
	}
	value := []rune(g.currentValue())
	pos := clampInt(g.caret(), 0, len(value))
	next := string(value[:pos]) + text + string(value[pos:])
	g.setCurrentValue(next)
	g.setCaret(pos + utf8.RuneCountInString(text))
}

func (g *mainGridEditor) backspace() {
	value := []rune(g.currentValue())
	pos := clampInt(g.caret(), 0, len(value))
	if pos == 0 {
		g.moveLeft()
		return
	}
	value = append(value[:pos-1], value[pos:]...)
	g.setCurrentValue(string(value))
	g.setCaret(pos - 1)
}

func (g *mainGridEditor) deleteAtCaret() {
	value := []rune(g.currentValue())
	pos := clampInt(g.caret(), 0, len(value))
	if pos >= len(value) {
		return
	}
	value = append(value[:pos], value[pos+1:]...)
	g.setCurrentValue(string(value))
	g.setCaret(pos)
}

func (g *mainGridEditor) updateStatus() {
	if g.status == nil {
		return
	}
	g.status.SetText(fmt.Sprintf("Row %d, %s. Space switches columns; arrows edit inside the grid.", g.state.ActiveRow+1, g.columns[g.state.ActiveCol].Title))
}

type mainActionButton struct {
	*tview.Box
	label  string
	value  string
	action func()
}

func newMainActionButton(label string, value string, action func()) *mainActionButton {
	button := &mainActionButton{
		Box:    tview.NewBox().SetBorder(false),
		label:  strings.TrimSpace(label),
		value:  strings.TrimSpace(value),
		action: action,
	}
	return button
}

func (b *mainActionButton) Draw(screen tcell.Screen) {
	b.Box.DrawForSubclass(screen, b)
	x, y, width, height := b.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}
	buttonStyle := tcell.StyleDefault.Foreground(tview.Styles.PrimaryTextColor).Background(tview.Styles.PrimitiveBackgroundColor)
	if b.HasFocus() {
		buttonStyle = tcell.StyleDefault.Foreground(colorAction).Background(tview.Styles.PrimitiveBackgroundColor).Bold(true)
	}
	value := elideText(firstNonEmptyText(b.value, "set species..."), maxInt(1, width-4))
	text := "[ " + value + " ]"
	printClipped(screen, x, y+height/2, width, buttonStyle, text)
}

func (b *mainActionButton) Focus(delegate func(p tview.Primitive)) {
	b.Box.Focus(delegate)
}

func (b *mainActionButton) HasFocus() bool {
	return b.Box.HasFocus()
}

func (b *mainActionButton) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return b.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		if event != nil && event.Key() == tcell.KeyRune && event.Rune() == ' ' && b.action != nil {
			b.action()
		}
	})
}

func (b *mainActionButton) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return b.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if event == nil || !b.InRect(event.Position()) {
			return false, nil
		}
		if action == tview.MouseLeftDown || action == tview.MouseLeftClick {
			if setFocus != nil {
				setFocus(b)
			}
			if action == tview.MouseLeftClick && b.action != nil {
				b.action()
			}
			return true, b
		}
		return false, nil
	})
}

func newMainControlField(label string, child tview.Primitive) *mainControlField {
	return &mainControlField{
		Box:   tview.NewBox(),
		label: strings.TrimSpace(label),
		child: child,
	}
}

func (f *mainControlField) Draw(screen tcell.Screen) {
	f.Box.DrawForSubclass(screen, f)
	x, y, width, height := f.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}
	labelStyle := tcell.StyleDefault.Foreground(tview.Styles.SecondaryTextColor).Background(tview.Styles.PrimitiveBackgroundColor)
	printClipped(screen, x, y, width, labelStyle, f.label)
	if f.child != nil && height > 1 {
		f.child.SetRect(x, y+1, width, height-1)
		f.child.Draw(screen)
	}
}

func (f *mainControlField) Focus(delegate func(tview.Primitive)) {
	if f.child != nil {
		delegate(f.child)
		return
	}
	f.Box.Focus(delegate)
}

func (f *mainControlField) HasFocus() bool {
	if f.child != nil && f.child.HasFocus() {
		return true
	}
	return f.Box.HasFocus()
}

func (f *mainControlField) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return f.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if f.child == nil || event == nil || !f.InRect(event.Position()) {
			return false, nil
		}
		if handler := f.child.MouseHandler(); handler != nil {
			return handler(action, event, setFocus)
		}
		return false, nil
	})
}

func (f *mainControlField) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return f.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		if f.child == nil {
			return
		}
		if handler := f.child.InputHandler(); handler != nil {
			handler(event, setFocus)
		}
	})
}

func newMainTabFrame(child tview.Primitive, active string, onSelect func(string), onFocus func(tview.Primitive)) *mainTabFrame {
	return &mainTabFrame{
		Box:      tview.NewBox().SetBorder(true),
		child:    child,
		active:   normalizeMainTab(active),
		onSelect: onSelect,
		onFocus:  onFocus,
	}
}

func (f *mainTabFrame) Draw(screen tcell.Screen) {
	f.Box.DrawForSubclass(screen, f)
	x, y, width, height := f.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}
	if f.child != nil {
		f.child.SetRect(x, y, width, height)
		f.child.Draw(screen)
	}
	f.drawFrameBorder(screen)
	f.drawTabs(screen)
}

func (f *mainTabFrame) Focus(delegate func(tview.Primitive)) {
	f.Box.Focus(delegate)
	if f.child != nil {
		f.child.Focus(delegate)
	}
}

func (f *mainTabFrame) HasFocus() bool {
	return f.Box.HasFocus()
}

func (f *mainTabFrame) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return f.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if event == nil || !f.InRect(event.Position()) {
			return false, nil
		}
		if action == tview.MouseLeftClick {
			mx, my := event.Position()
			x, y, width, _ := f.GetRect()
			if my == y && mx >= x && mx < x+width {
				relX := mx - x
				for _, pos := range f.tabPositions(width) {
					if relX >= pos.left && relX < pos.right {
						if f.onSelect != nil {
							f.onSelect(pos.id)
						}
						return true, f
					}
				}
			}
		}
		if f.child != nil {
			if handler := f.child.MouseHandler(); handler != nil {
				return handler(action, event, func(p tview.Primitive) {
					if setFocus != nil {
						setFocus(p)
					}
					if f.onFocus != nil {
						f.onFocus(p)
					}
				})
			}
		}
		return false, nil
	})
}

func (f *mainTabFrame) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return f.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		if f.child == nil {
			return
		}
		if handler := f.child.InputHandler(); handler != nil {
			handler(event, setFocus)
		}
	})
}

func handleMainExploreListKey(list *mainExploreList, event *tcell.EventKey, app *tview.Application) bool {
	if list == nil || event == nil {
		return false
	}
	switch event.Key() {
	case tcell.KeyUp, tcell.KeyDown, tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn, tcell.KeyEnter:
	case tcell.KeyRune:
		r := event.Rune()
		if r != ' ' && (r < '1' || r > '9') {
			return false
		}
	default:
		return false
	}
	if handler := list.InputHandler(); handler != nil {
		handler(event, func(p tview.Primitive) {
			if p != nil && app != nil {
				app.SetFocus(p)
			}
		})
		return true
	}
	return false
}

type mainTabPosition struct {
	id    string
	left  int
	right int
}

func (f *mainTabFrame) tabPositions(width int) []mainTabPosition {
	tabs := mainTabs()
	positions := make([]mainTabPosition, 0, len(tabs))
	x := 2
	for _, tab := range tabs {
		label := "[ " + tab.label + " ]"
		w := utf8.RuneCountInString(label)
		positions = append(positions, mainTabPosition{id: tab.id, left: x, right: x + w})
		x += w + 1
	}
	return positions
}

func (f *mainTabFrame) drawTabs(screen tcell.Screen) {
	x, y, width, _ := f.GetRect()
	if width <= 0 {
		return
	}
	baseStyle := tcell.StyleDefault.Foreground(tview.Styles.SecondaryTextColor).Background(tview.Styles.PrimitiveBackgroundColor)
	activeStyle := tcell.StyleDefault.Foreground(colorActionText).Background(colorAction).Bold(true)
	tabs := mainTabs()
	for i, pos := range f.tabPositions(width) {
		if i >= len(tabs) || pos.left >= width {
			break
		}
		label := fitTextRunes("[ "+tabs[i].label+" ]", maxInt(1, width-pos.left-1))
		style := baseStyle
		if tabs[i].id == normalizeMainTab(f.active) {
			style = activeStyle
		}
		printStyledText(screen, x+pos.left, y, width-pos.left, style, label)
	}
}

func (f *mainTabFrame) drawFrameBorder(screen tcell.Screen) {
	x, y, width, height := f.GetRect()
	if width < 2 || height < 2 {
		return
	}
	style := tcell.StyleDefault.Foreground(tview.Styles.BorderColor).Background(tview.Styles.PrimitiveBackgroundColor)
	horizontal := tview.Borders.Horizontal
	vertical := tview.Borders.Vertical
	topLeft := tview.Borders.TopLeft
	topRight := tview.Borders.TopRight
	bottomLeft := tview.Borders.BottomLeft
	bottomRight := tview.Borders.BottomRight
	if f.HasFocus() {
		style = tcell.StyleDefault.Foreground(colorAction).Background(tview.Styles.PrimitiveBackgroundColor)
		horizontal = tview.Borders.HorizontalFocus
		vertical = tview.Borders.VerticalFocus
		topLeft = tview.Borders.TopLeftFocus
		topRight = tview.Borders.TopRightFocus
		bottomLeft = tview.Borders.BottomLeftFocus
		bottomRight = tview.Borders.BottomRightFocus
	}
	for col := x + 1; col < x+width-1; col++ {
		screen.SetContent(col, y, horizontal, nil, style)
		screen.SetContent(col, y+height-1, horizontal, nil, style)
	}
	for row := y + 1; row < y+height-1; row++ {
		screen.SetContent(x, row, vertical, nil, style)
		screen.SetContent(x+width-1, row, vertical, nil, style)
	}
	screen.SetContent(x, y, topLeft, nil, style)
	screen.SetContent(x+width-1, y, topRight, nil, style)
	screen.SetContent(x, y+height-1, bottomLeft, nil, style)
	screen.SetContent(x+width-1, y+height-1, bottomRight, nil, style)
}

func mainTabs() []struct {
	id    string
	label string
} {
	return []struct {
		id    string
		label string
	}{
		{"keyword", "Keyword"},
		{"blast", "BLAST"},
		{"explore", "Explore"},
	}
}

func mainDropDown(label string, options []Option, current string, changed func(string)) *tview.DropDown {
	return mainDropDownWithRefresh(label, options, current, changed, nil)
}

func mainDropDownWithRefresh(label string, options []Option, current string, changed func(string), refresh func()) *tview.DropDown {
	values := make([]string, len(options))
	labels := make([]string, len(options))
	currentIndex := 0
	for i, option := range options {
		values[i] = option.Value
		labels[i] = option.Label
		if option.Value == current {
			currentIndex = i
		}
	}
	dropDown := tview.NewDropDown()
	initializing := true
	dropDown.SetOptions(labels, func(_ string, index int) {
		if initializing {
			return
		}
		if index >= 0 && index < len(values) && changed != nil {
			changed(values[index])
		}
		if refresh != nil {
			refresh()
		}
	})
	dropDown.SetCurrentOption(currentIndex)
	initializing = false
	dropDown.SetFieldBackgroundColor(colorPanel)
	dropDown.SetFieldTextColor(tview.Styles.PrimaryTextColor)
	dropDown.SetLabelColor(tview.Styles.SecondaryTextColor)
	return dropDown
}

func handleMainDropDownKey(dropDown *tview.DropDown, event *tcell.EventKey, app *tview.Application) bool {
	if dropDown == nil || event == nil {
		return false
	}
	if dropDown.IsOpen() {
		switch event.Key() {
		case tcell.KeyUp, tcell.KeyDown, tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn, tcell.KeyEscape, tcell.KeyEnter:
			deliverDropDownKey(dropDown, event, app)
			return true
		case tcell.KeyRune:
			if event.Rune() == ' ' {
				deliverDropDownKey(dropDown, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), app)
				return true
			}
			deliverDropDownKey(dropDown, event, app)
			return true
		default:
			deliverDropDownKey(dropDown, event, app)
			return true
		}
	}
	if event.Key() == tcell.KeyEnter || event.Key() == tcell.KeyRune && event.Rune() == ' ' {
		deliverDropDownKey(dropDown, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), app)
		return true
	}
	return false
}

func mainAliasChoices(current string, aliases []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(aliases)+1)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || value == "~" {
			return
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	add(current)
	for _, alias := range aliases {
		add(alias)
	}
	return out
}

func mainInterfaceIntro(info StartupInfo) *tview.TextView {
	text := fmt.Sprintf("[white]%s runs keyword searches, BLAST workflows, and exploration tools across plant protein resources; choose a tab, finish its settings, then start the action. Author: [yellow]%s[white]  Repository: [yellow]%s[white]  License: [yellow]%s%s[white]",
		productName(info),
		fallbackText(info.Author, "unknown"),
		fallbackText(info.RepoURL, "unknown"),
		fallbackText(info.LicenseName, "unknown"),
		formatLicenseID(info.LicenseID),
	)
	return tview.NewTextView().SetDynamicColors(true).SetTextColor(tview.Styles.PrimaryTextColor).SetText(text)
}

func mainKeywordOptions() []Option {
	caps := mainKeywordCapabilities()
	out := make([]Option, 0, len(caps))
	for _, cap := range caps {
		out = append(out, Option{Value: cap.ID, Label: cap.Label, Description: cap.Description})
	}
	return out
}

func mainBlastOptions() []Option {
	caps := mainBlastCapabilities()
	out := make([]Option, 0, len(caps))
	for _, cap := range caps {
		out = append(out, Option{Value: cap.ID, Label: cap.Label, Description: cap.Description})
	}
	return out
}

func mainSearchTypeOptions(cap mainCapability) []Option {
	out := make([]Option, 0, len(cap.SearchTypes))
	for _, searchType := range cap.SearchTypes {
		out = append(out, Option{Value: searchType.ID, Label: searchType.Label})
	}
	return out
}

func mainProgramOptions(programs []string) []Option {
	out := make([]Option, 0, len(programs))
	for _, program := range programs {
		out = append(out, Option{Value: program, Label: mainProgramLabel(program)})
	}
	return out
}

func mainProgramLabel(program string) string {
	switch strings.ToLower(strings.TrimSpace(program)) {
	case "blastn":
		return "BLASTN  nucleotide -> nucleotide"
	case "blastx":
		return "BLASTX  translated nucleotide -> protein"
	case "tblastn":
		return "TBLASTN  protein -> translated nucleotide"
	case "blastp":
		return "BLASTP  protein -> protein"
	default:
		return strings.ToUpper(strings.TrimSpace(program))
	}
}

func mainGeneLocusPriorityOptions() []Option {
	return []Option{
		{Value: GeneLocusPriorityNone, Label: "None"},
		{Value: GeneLocusPriorityNCBI, Label: "Use NCBI database"},
		{Value: GeneLocusPriorityPLAZA, Label: "Use PLAZA database"},
	}
}

func mainKeywordCapabilities() []mainCapability {
	return []mainCapability{
		{ID: "phytozome", Label: "Phytozome keyword", Description: "keyword search in Phytozome species", RequiresSpecies: true, ShowsSpecies: true, ShowsSymbolName: true, SupportsWide: true},
		{ID: "cyp", Label: "CYP / P450 keyword", Description: "local CYP database from Dr Nelson's four categories", RequiresSpecies: true, ShowsSpecies: true, ShowsSymbolName: true, ShowsGeneLocus: true},
		{ID: "lemna", Label: "lemna keyword", Description: "keyword search in lemna.org releases", RequiresSpecies: true, ShowsSpecies: true, ShowsSymbolName: true, SupportsWide: false},
		{ID: "tair", Label: "TAIR keyword", Description: "keyword search in TAIR Arabidopsis releases", RequiresSpecies: true, ShowsSpecies: true, ShowsSymbolName: true, SupportsWide: false},
		{ID: "ncbi", Label: "NCBI Entrez keyword", Description: "Entrez/E-utilities search across NCBI database types", SearchTypes: mainNCBISearchTypeCapabilities(), DefaultSearchTypeID: "protein", ShowsSymbolName: true, ShowsGeneLocus: true},
	}
}

func mainNCBISearchTypeCapabilities() []mainSearchTypeCapability {
	specs := ncbi.SearchableSearchTypes()
	out := make([]mainSearchTypeCapability, 0, len(specs))
	for _, spec := range specs {
		out = append(out, mainSearchTypeCapability{
			ID:              spec.ID,
			Label:           spec.Label,
			RequiresSpecies: false,
			ShowsSpecies:    false,
			ShowsSymbolName: spec.ShowsSymbolName,
			ShowsGeneLocus:  spec.ShowsGeneLocus,
			SupportsWide:    spec.SupportsWide,
		})
	}
	return out
}

func mainBlastCapabilities() []mainCapability {
	return []mainCapability{
		{ID: "phytozome", Label: "Phytozome blast", Description: "BLAST against a selected Phytozome species", RequiresSpecies: true, ShowsSpecies: true, ShowsSymbolName: true},
		{ID: "lemna", Label: "lemna blast", Description: "BLAST against lemna.org releases", RequiresSpecies: true, ShowsSpecies: true, ShowsSymbolName: true, BlastPrograms: []string{"blastn", "blastx", "tblastn", "blastp"}},
		{ID: "tair", Label: "TAIR blast", Description: "BLAST against TAIR Arabidopsis releases", RequiresSpecies: true, ShowsSpecies: true, ShowsSymbolName: true, BlastPrograms: []string{"blastn", "blastx", "tblastn", "blastp"}},
	}
}

func mainKeywordCapability(id string) mainCapability {
	return mainCapabilityByID(mainKeywordCapabilities(), id)
}

func mainBlastCapability(id string) mainCapability {
	return mainCapabilityByID(mainBlastCapabilities(), id)
}

func mainCapabilityByID(caps []mainCapability, id string) mainCapability {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, cap := range caps {
		if cap.ID == id {
			return cap
		}
	}
	if len(caps) == 0 {
		return mainCapability{}
	}
	return caps[0]
}

func mainKeywordSearchType(databaseID string) mainSearchTypeCapability {
	cap := mainKeywordCapability(databaseID)
	return mainKeywordSearchTypeFor(cap, cap.DefaultSearchTypeID)
}

func mainKeywordSearchTypeFor(cap mainCapability, id string) mainSearchTypeCapability {
	if len(cap.SearchTypes) == 0 {
		return mainSearchTypeCapability{
			ID:              firstString([]string{cap.ID}, "keyword"),
			Label:           "Keyword",
			RequiresSpecies: cap.RequiresSpecies,
			ShowsSpecies:    cap.ShowsSpecies,
			ShowsSymbolName: cap.ShowsSymbolName,
			ShowsGeneLocus:  cap.ShowsGeneLocus,
			SupportsWide:    cap.SupportsWide,
		}
	}
	id = strings.TrimSpace(id)
	for _, searchType := range cap.SearchTypes {
		if searchType.ID == id {
			return searchType
		}
	}
	return cap.SearchTypes[0]
}

func normalizeMainTab(tab string) string {
	switch strings.ToLower(strings.TrimSpace(tab)) {
	case "blast":
		return "blast"
	case "explore":
		return "explore"
	default:
		return "keyword"
	}
}

func normalizeMainGridValue(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r", ""), "\t", ""))
	value = strings.ReplaceAll(value, " ", "")
	if value == "~" {
		return ""
	}
	return value
}

func normalizeMainGridValueForColumn(value string, column gridColumn) string {
	if !column.KeepNL {
		return normalizeMainGridValue(value)
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(lines[i]), "\t", ""), " ", "")
	}
	value = strings.Join(lines, "\n")
	value = strings.Trim(value, " \t\r")
	if strings.TrimSpace(value) == "~" {
		return ""
	}
	return value
}

func normalizeMainGridEditValueForColumn(value string, column gridColumn) string {
	if !column.KeepNL {
		value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r", ""), "\t", ""))
		return strings.ReplaceAll(value, " ", "")
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(lines[i]), "\t", ""), " ", "")
	}
	return strings.Trim(strings.Join(lines, "\n"), " \t\r")
}

func mainSpeciesButtonLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return "set species..."
	}
	return elideText(label, 32)
}

func mainOptionsHeight(showSpecies bool, showExtra bool) int {
	_ = showSpecies
	_ = showExtra
	return 4
}

func printClipped(screen tcell.Screen, x, y, width int, style tcell.Style, text string) {
	if width <= 0 {
		return
	}
	text = elideText(text, width)
	for len([]rune(text)) < width {
		text += " "
	}
	printStyledText(screen, x, y, width, style, text)
}

func elideText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= width {
		return string(runes)
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}

func firstString(values []string, fallback string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return fallback
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

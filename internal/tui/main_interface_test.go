// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package tui

import (
	"strings"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/ncbi"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestMainKeywordRowsNormalizeExecutionRows(t *testing.T) {
	rows := []MainKeywordRow{
		{SearchTerm: " PAL1 ", SymbolName: " C4H ", GeneLocus: "~"},
		{},
		{SearchTerm: "~", SymbolName: "~", GeneLocus: "~"},
	}
	got := MainKeywordRowsForExecution(rows)
	if len(got) != 1 {
		t.Fatalf("expected 1 executable row, got %d: %#v", len(got), got)
	}
	if got[0].SearchTerm != "PAL1" || got[0].SymbolName != "C4H" || got[0].GeneLocus != "" {
		t.Fatalf("unexpected normalized row: %#v", got[0])
	}
}

func TestMainBlastRowsNormalizeDisplayRows(t *testing.T) {
	rows := []MainBlastRow{
		{FASTA: " MEP NTM ", SymbolName: "~"},
		{},
	}
	got := MainBlastRowsForDisplay(rows)
	if len(got) != 2 {
		t.Fatalf("expected one populated row plus trailing empty row, got %d: %#v", len(got), got)
	}
	if got[0].FASTA != "MEPNTM" || got[0].SymbolName != "" {
		t.Fatalf("unexpected normalized row: %#v", got[0])
	}
}

func TestMainGridPasteOnlyCurrentColumn(t *testing.T) {
	rows := []MainKeywordRow{{SearchTerm: "A", SymbolName: "S1"}}
	state := GridEditorState{ActiveRow: 0, ActiveCol: 1}
	grid := newKeywordGridEditor(&rows, &state, nil, mainSearchTypeCapability{ShowsSymbolName: true, ShowsGeneLocus: true})
	grid.PasteText("B\nC")
	if len(rows) != 3 {
		t.Fatalf("expected two pasted rows plus trailing empty row, got %d: %#v", len(rows), rows)
	}
	if rows[0].SearchTerm != "A" || rows[0].SymbolName != "B" {
		t.Fatalf("paste should replace only active column in current row: %#v", rows[0])
	}
	if rows[1].SearchTerm != "" || rows[1].SymbolName != "C" {
		t.Fatalf("paste should fill downward in active column only: %#v", rows[1])
	}
}

func TestMainGridPasteIgnoresTrailingClipboardNewline(t *testing.T) {
	rows := []MainKeywordRow{{}}
	state := GridEditorState{ActiveRow: 0, ActiveCol: 0}
	grid := newKeywordGridEditor(&rows, &state, nil, mainSearchTypeCapability{ShowsSymbolName: true})
	grid.PasteText("A\nB\n")
	got := MainKeywordRowsForExecution(rows)
	if len(got) != 2 {
		t.Fatalf("expected two executable rows, got %d: %#v", len(got), got)
	}
	if state.ActiveRow != 1 {
		t.Fatalf("caret should land on last pasted value, got row %d", state.ActiveRow)
	}
}

func TestMainBlastGridPasteKeepsMultilineFastaTogether(t *testing.T) {
	rows := []MainBlastRow{{}}
	state := GridEditorState{ActiveRow: 0, ActiveCol: 0}
	grid := newBlastGridEditor(&rows, &state, nil, mainCapability{ShowsSymbolName: true})
	grid.PasteText(">q1\nMEPNTM\nASDF\n>q2\nQQQQ")
	if len(rows) != 3 {
		t.Fatalf("expected two FASTA rows plus trailing empty row, got %d: %#v", len(rows), rows)
	}
	if rows[0].FASTA != ">q1\nMEPNTM\nASDF" {
		t.Fatalf("first FASTA record was split incorrectly: %#v", rows[0])
	}
	if rows[1].FASTA != ">q2\nQQQQ" {
		t.Fatalf("second FASTA record was split incorrectly: %#v", rows[1])
	}
}

func TestMainBlastPasteGroupsHeaderlessWrappedSequence(t *testing.T) {
	got := splitMainBlastPasteRecords("MEPNTM\nASDFGH\nQQQQ")
	want := []string{"MEPNTM\nASDFGH\nQQQQ"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected headerless split:\nwant %#v\ngot  %#v", want, got)
	}
}

func TestMainBlastPasteSplitsHeaderlessRecordsOnBlankLines(t *testing.T) {
	got := splitMainBlastPasteRecords("MEPNTM\nASDFGH\n\nQQQQ\nTTTT")
	want := []string{"MEPNTM\nASDFGH", "QQQQ\nTTTT"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected blank-line split:\nwant %#v\ngot  %#v", want, got)
	}
}

func TestMainBlastPasteHandlesMixedFastaAndHeaderlessRecords(t *testing.T) {
	got := splitMainBlastPasteRecords(">q1\nMEPNTM\nASDFGH\n\nQQQQ\nTTTT\n>q2\nCCCC")
	want := []string{">q1\nMEPNTM\nASDFGH", "QQQQ\nTTTT", ">q2\nCCCC"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected mixed split:\nwant %#v\ngot  %#v", want, got)
	}
}

func TestMainBlastPasteSplitsSingleLineTokens(t *testing.T) {
	got := splitMainBlastPasteRecords("MEPNTM ASDFGH https://example.test/report")
	want := []string{"MEPNTM", "ASDFGH", "https://example.test/report"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected token split:\nwant %#v\ngot  %#v", want, got)
	}
}

func TestMainBlastPastePreservesURLAndIDLines(t *testing.T) {
	got := splitMainBlastPasteRecords("https://example.test/report/1\nAT1G01010\n\nhttps://example.test/report/2")
	want := []string{"https://example.test/report/1", "AT1G01010", "https://example.test/report/2"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected URL/ID split:\nwant %#v\ngot  %#v", want, got)
	}
}

func TestMainBlastGridVisualDownMovesInsideWrappedFastaBeforeNextRow(t *testing.T) {
	rows := []MainBlastRow{
		{FASTA: ">q1\nABCDEFGHIJK"},
		{FASTA: ">q2\nZZ"},
	}
	state := GridEditorState{ActiveRow: 0, ActiveCol: 0}
	grid := newBlastGridEditor(&rows, &state, nil, mainCapability{ShowsSymbolName: true})
	grid.SetRect(0, 0, 80, 20)
	grid.setCaret(0)
	width := grid.activeColumnWidth()
	line, _ := grid.caretVisualPosition(width)
	if line != 0 {
		t.Fatalf("expected caret to start on visual line 0, got %d", line)
	}
	grid.moveVisualDown()
	if state.ActiveRow != 0 {
		t.Fatalf("first Down should stay in same logical row, row=%d", state.ActiveRow)
	}
	line, _ = grid.caretVisualPosition(width)
	if line != 1 {
		t.Fatalf("first Down should move to next visual line, got %d", line)
	}
	for state.ActiveRow == 0 {
		grid.moveVisualDown()
	}
	if state.ActiveRow != 1 {
		t.Fatalf("Down from wrapped cell bottom should move to next logical row, got %d", state.ActiveRow)
	}
}

func TestMainBlastGridFastaColumnWidthIsFixed(t *testing.T) {
	rows := []MainBlastRow{{FASTA: ">q1\n" + strings.Repeat("A", 200)}}
	state := GridEditorState{}
	grid := newBlastGridEditor(&rows, &state, nil, mainCapability{ShowsSymbolName: true})
	widths := grid.columnWidths(120)
	if len(widths) == 0 {
		t.Fatal("expected column widths")
	}
	if widths[0] != 60 {
		t.Fatalf("FASTA column should stay half the grid width, got %d", widths[0])
	}
}

func TestMainGridColumnWidthsStartAtHeaderWidth(t *testing.T) {
	rows := []MainKeywordRow{{}}
	state := GridEditorState{}
	grid := newKeywordGridEditor(&rows, &state, nil, mainKeywordSearchType("phytozome"))
	widths := grid.columnWidths(120)
	if widths[0] != taggedPaddedWidth("Search term") {
		t.Fatalf("search term width = %d, want header width", widths[0])
	}
	if widths[1] != taggedPaddedWidth("Symbol name") {
		t.Fatalf("symbol width = %d, want header width", widths[1])
	}
	rows[0].SearchTerm = "Arabidopsis"
	widths = grid.columnWidths(120)
	if widths[0] != taggedPaddedWidth("Arabidopsis") {
		t.Fatalf("search term width should grow with content, got %d", widths[0])
	}
}

func TestMainGridRowNumbersOnlyForPopulatedRows(t *testing.T) {
	rows := []MainKeywordRow{{SearchTerm: "PAL"}, {}}
	state := GridEditorState{}
	grid := newKeywordGridEditor(&rows, &state, nil, mainKeywordSearchType("phytozome"))
	if got := grid.rowDisplayNumber(0); got != "1" {
		t.Fatalf("row 0 display number = %q, want 1", got)
	}
	if got := grid.rowDisplayNumber(1); got != "" {
		t.Fatalf("empty trailing row display number = %q, want empty", got)
	}
}

func TestMainKeywordValidationRequiresSpeciesAndTerm(t *testing.T) {
	issues := MainKeywordValidationIssues(MainKeywordState{
		DatabaseID:   "phytozome",
		SearchTypeID: "keyword",
		Rows:         []MainKeywordRow{{SymbolName: "PAL"}},
	})
	if len(issues) == 0 {
		t.Fatal("expected validation issues")
	}
	if !containsString(issues, "Set species.") || !containsString(issues, "Row 1 is missing search term.") {
		t.Fatalf("missing expected issues: %#v", issues)
	}
}

func TestMainKeywordGridColumnsFollowCapability(t *testing.T) {
	rows := []MainKeywordRow{{}}
	state := GridEditorState{}
	nonNCBI := newKeywordGridEditor(&rows, &state, nil, mainKeywordSearchType("phytozome"))
	if got := columnIDs(nonNCBI.columns); strings.Join(got, ",") != "term,symbol" {
		t.Fatalf("unexpected Phytozome keyword columns: %#v", got)
	}
	ncbi := newKeywordGridEditor(&rows, &state, nil, mainKeywordSearchType("ncbi"))
	if got := columnIDs(ncbi.columns); strings.Join(got, ",") != "term,symbol,locus" {
		t.Fatalf("unexpected NCBI keyword columns: %#v", got)
	}
}

func TestMainBlastGridHidesGeneLocusForCurrentCapabilities(t *testing.T) {
	rows := []MainBlastRow{{}}
	state := GridEditorState{}
	grid := newBlastGridEditor(&rows, &state, nil, mainBlastCapability("phytozome"))
	if got := columnIDs(grid.columns); strings.Join(got, ",") != "fasta,symbol" {
		t.Fatalf("unexpected BLAST columns: %#v", got)
	}
}

func TestMainBlastFastaCellEnterInsertsNewline(t *testing.T) {
	rows := []MainBlastRow{{FASTA: ">q1"}}
	state := GridEditorState{ActiveRow: 0, ActiveCol: 0}
	grid := newBlastGridEditor(&rows, &state, nil, mainBlastCapability("phytozome"))
	grid.setCaret(3)
	if !grid.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), nil) {
		t.Fatal("Enter should be handled in FASTA column")
	}
	grid.insertText("AAAA")
	if rows[0].FASTA != ">q1\nAAAA" {
		t.Fatalf("FASTA newline insert failed: %#v", rows[0].FASTA)
	}
}

func TestMainBlastFastaCellCtrlEnterReservedForPrimaryAction(t *testing.T) {
	rows := []MainBlastRow{{FASTA: ">q1"}}
	state := GridEditorState{ActiveRow: 0, ActiveCol: 0}
	grid := newBlastGridEditor(&rows, &state, nil, mainBlastCapability("phytozome"))
	grid.setCaret(3)
	if grid.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModCtrl), nil) {
		t.Fatal("Ctrl+Enter should not be consumed by the FASTA grid")
	}
	if rows[0].FASTA != ">q1" {
		t.Fatalf("Ctrl+Enter changed FASTA: %#v", rows[0].FASTA)
	}
}

func TestMainGridTypedTildeAndShiftPunctuationAreKeptWhileEditing(t *testing.T) {
	rows := []MainKeywordRow{{}}
	state := GridEditorState{ActiveRow: 0, ActiveCol: 0}
	grid := newKeywordGridEditor(&rows, &state, nil, mainKeywordSearchType("phytozome"))
	if !grid.HandleKey(tcell.NewEventKey(tcell.KeyRune, '~', tcell.ModNone), nil) {
		t.Fatal("tilde should be handled")
	}
	if rows[0].SearchTerm != "~" {
		t.Fatalf("typed tilde should remain visible while editing, got %q", rows[0].SearchTerm)
	}
	if !grid.HandleKey(tcell.NewEventKey(tcell.KeyRune, '!', tcell.ModShift), nil) {
		t.Fatal("shift punctuation should be handled")
	}
	if rows[0].SearchTerm != "~!" {
		t.Fatalf("typed punctuation should be inserted once, got %q", rows[0].SearchTerm)
	}
}

func TestMainKeywordGridEnterMovesToNextRow(t *testing.T) {
	rows := []MainKeywordRow{{SearchTerm: "PAL"}}
	state := GridEditorState{ActiveRow: 0, ActiveCol: 0}
	grid := newKeywordGridEditor(&rows, &state, nil, mainKeywordSearchType("phytozome"))
	grid.setCaret(1)
	if !grid.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), nil) {
		t.Fatal("Enter should be handled in keyword grid")
	}
	if state.ActiveRow != 1 || state.ActiveCol != 0 {
		t.Fatalf("keyword Enter should move to next row in same column, state=%#v", state)
	}
	if len(rows) < 2 {
		t.Fatalf("keyword Enter should create next row, rows=%#v", rows)
	}
}

func TestMainTabsHideShortcutMarkers(t *testing.T) {
	labels := make([]string, 0, len(mainTabs()))
	for _, tab := range mainTabs() {
		labels = append(labels, tab.label)
	}
	if labels[0] != "Keyword" || labels[1] != "BLAST" || labels[2] != "Explore" {
		t.Fatalf("tab labels should not include shortcut markers, got %#v", labels)
	}
}

func TestMainGridDrawsColumnSeparators(t *testing.T) {
	rows := []MainKeywordRow{{SearchTerm: "PAL", SymbolName: "PAL1"}}
	state := GridEditorState{}
	grid := newKeywordGridEditor(&rows, &state, nil, mainKeywordSearchType("phytozome"))
	grid.SetRect(0, 0, 80, 8)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init failed: %v", err)
	}
	grid.Draw(screen)
	found := false
	for y := 0; y < 8 && !found; y++ {
		for x := 0; x < 80; x++ {
			r, _, _, _ := screen.GetContent(x, y)
			if r == tview.Borders.Vertical {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("expected grid draw to include visible column separators")
	}
}

func TestMainGridWrappedFastaCaretUsesContentWidth(t *testing.T) {
	rows := []MainBlastRow{{FASTA: strings.Repeat("A", 20)}}
	state := GridEditorState{ActiveRow: 0, ActiveCol: 0}
	grid := newBlastGridEditor(&rows, &state, nil, mainBlastCapability("phytozome"))
	grid.SetRect(0, 0, 40, 8)
	colWidths := grid.columnWidths(40 - grid.rowColumnWidth() - 1)
	contentWidth := mainGridContentWidth(colWidths[0])
	if contentWidth <= 0 {
		t.Fatalf("bad content width %d from widths %#v", contentWidth, colWidths)
	}
	grid.setCaret(contentWidth + 1)
	line, col := grid.caretVisualPosition(grid.activeColumnWidth())
	if line != 1 || col != 1 {
		t.Fatalf("caret should wrap by content width, got line=%d col=%d contentWidth=%d", line, col, contentWidth)
	}
}

func TestMainGridDrawCursorHighlightsCellPosition(t *testing.T) {
	rows := []MainKeywordRow{{SearchTerm: "PAL"}}
	state := GridEditorState{ActiveRow: 0, ActiveCol: 0}
	grid := newKeywordGridEditor(&rows, &state, nil, mainKeywordSearchType("phytozome"))
	grid.SetRect(0, 0, 80, 8)
	grid.setCaret(1)
	grid.Focus(func(p tview.Primitive) {})
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init failed: %v", err)
	}
	grid.Draw(screen)
	widths := grid.columnWidths(80 - grid.rowColumnWidth() - 1)
	cursorX := grid.rowColumnWidth() + 1 + mainGridContentOffset() + 1
	cursorY := grid.rowScreenY(0, widths)
	_, _, style, _ := screen.GetContent(cursorX, cursorY)
	fg, bg, _ := style.Decompose()
	if fg != tcell.ColorBlack || bg != tcell.ColorWhite {
		t.Fatalf("cursor cell should use input cursor colors at %d,%d, got fg=%v bg=%v", cursorX, cursorY, fg, bg)
	}
}

func TestMainGridDrawCursorAtInsertionPointAfterText(t *testing.T) {
	rows := []MainKeywordRow{{SearchTerm: "qqq"}}
	state := GridEditorState{ActiveRow: 0, ActiveCol: 0}
	grid := newKeywordGridEditor(&rows, &state, nil, mainKeywordSearchType("phytozome"))
	grid.SetRect(0, 0, 80, 8)
	grid.setCaret(3)
	grid.Focus(func(p tview.Primitive) {})
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init failed: %v", err)
	}
	grid.Draw(screen)
	widths := grid.columnWidths(80 - grid.rowColumnWidth() - 1)
	rowY := grid.rowScreenY(0, widths)
	textX := -1
	for x := 0; x < 77; x++ {
		r0, _, _, _ := screen.GetContent(x, rowY)
		r1, _, _, _ := screen.GetContent(x+1, rowY)
		r2, _, _, _ := screen.GetContent(x+2, rowY)
		if r0 == 'q' && r1 == 'q' && r2 == 'q' {
			textX = x
			break
		}
	}
	if textX < 0 {
		t.Fatalf("rendered row does not contain qqq")
	}
	for i, want := range []rune{'q', 'q', 'q'} {
		got, _, style, _ := screen.GetContent(textX+i, rowY)
		_, bg, _ := style.Decompose()
		if got != want || bg != colorAction {
			t.Fatalf("text cell %d = %q bg=%v, want %q action bg", i, got, bg, want)
		}
	}
	got, _, style, _ := screen.GetContent(textX+3, rowY)
	fg, bg, _ := style.Decompose()
	if got != ' ' || fg != tcell.ColorBlack || bg != tcell.ColorWhite {
		t.Fatalf("insertion cursor = %q fg=%v bg=%v, want blank black-on-white after text", got, fg, bg)
	}
}

func TestMainGridMouseClickUsesWrappedRowHeights(t *testing.T) {
	rows := []MainBlastRow{
		{FASTA: ">q1\n" + strings.Repeat("A", 90), SymbolName: "S1"},
		{FASTA: ">q2", SymbolName: "S2"},
	}
	state := GridEditorState{}
	grid := newBlastGridEditor(&rows, &state, nil, mainBlastCapability("phytozome"))
	grid.SetRect(0, 0, 80, 12)
	widths := grid.columnWidths(80)
	row0Height := grid.rowVisualHeight(0, widths)
	if row0Height < 3 {
		t.Fatalf("test setup expected wrapped first row, height=%d", row0Height)
	}
	row, _, ok := grid.rowAtVisualY(2+row0Height, 0, widths)
	if !ok || row != 1 {
		t.Fatalf("click below wrapped first row mapped to row=%d ok=%v, want row 1", row, ok)
	}
}

func TestMainGridMouseWheelScrollsRowsAndColumns(t *testing.T) {
	rows := []MainKeywordRow{
		{SearchTerm: "A", SymbolName: "S1", GeneLocus: "G1"},
		{SearchTerm: "B", SymbolName: "S2", GeneLocus: "G2"},
		{SearchTerm: "C", SymbolName: "S3", GeneLocus: "G3"},
	}
	state := GridEditorState{}
	grid := newKeywordGridEditor(&rows, &state, nil, mainKeywordSearchType("ncbi"))
	grid.SetRect(0, 0, 80, 6)
	handler := grid.MouseHandler()
	if handler == nil {
		t.Fatal("expected mouse handler")
	}
	consumed, _ := handler(tview.MouseScrollDown, tcell.NewEventMouse(5, 3, tcell.ButtonNone, 0), func(tview.Primitive) {})
	if !consumed || grid.rowOffset != 1 {
		t.Fatalf("scroll down should move row offset to 1, consumed=%v offset=%d", consumed, grid.rowOffset)
	}
	consumed, _ = handler(tview.MouseScrollRight, tcell.NewEventMouse(5, 3, tcell.ButtonNone, 0), func(tview.Primitive) {})
	if !consumed || grid.colOffset != 1 {
		t.Fatalf("scroll right should move col offset to 1, consumed=%v offset=%d", consumed, grid.colOffset)
	}
}

func TestMainExploreTabHasUntitledContentFrame(t *testing.T) {
	state := MainInterfaceState{Explore: MainExploreState{Tool: "open_session"}}
	module, _ := buildMainExploreTab(tview.NewApplication(), &state)
	module.SetRect(0, 0, 80, 12)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init failed: %v", err)
	}
	module.Draw(screen)
	for _, point := range []struct {
		x    int
		y    int
		want rune
	}{
		{0, 0, tview.Borders.TopLeft},
		{79, 0, tview.Borders.TopRight},
		{0, 11, tview.Borders.BottomLeft},
		{79, 11, tview.Borders.BottomRight},
		{0, 5, tview.Borders.Vertical},
		{79, 5, tview.Borders.Vertical},
		{40, 0, tview.Borders.Horizontal},
		{40, 11, tview.Borders.Horizontal},
	} {
		got, _, _, _ := screen.GetContent(point.x, point.y)
		if got != point.want {
			t.Fatalf("explore content frame at %d,%d = %q, want %q", point.x, point.y, got, point.want)
		}
	}
	var topBuilder strings.Builder
	for x := 0; x < 80; x++ {
		r, _, _, _ := screen.GetContent(x, 0)
		topBuilder.WriteRune(r)
	}
	topLine := topBuilder.String()
	if strings.Contains(topLine, "Explore") {
		t.Fatalf("explore content frame should be untitled, top line: %q", topLine)
	}
}

func TestMainExploreTabKeepsSharedOuterFrame(t *testing.T) {
	state := MainInterfaceState{Explore: MainExploreState{Tool: "open_session"}}
	module, _ := buildMainExploreTab(tview.NewApplication(), &state)
	frame := newMainTabFrame(module, "explore", nil, nil)
	frame.SetRect(0, 0, 80, 12)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init failed: %v", err)
	}
	frame.Draw(screen)
	for _, point := range []struct {
		x    int
		y    int
		want rune
	}{
		{0, 0, tview.Borders.TopLeft},
		{79, 0, tview.Borders.TopRight},
		{0, 11, tview.Borders.BottomLeft},
		{79, 11, tview.Borders.BottomRight},
		{0, 5, tview.Borders.Vertical},
		{79, 5, tview.Borders.Vertical},
		{40, 11, tview.Borders.Horizontal},
	} {
		got, _, _, _ := screen.GetContent(point.x, point.y)
		if got != point.want {
			t.Fatalf("shared tab frame at %d,%d = %q, want %q", point.x, point.y, got, point.want)
		}
	}
	var tabBuilder strings.Builder
	for x := 0; x < 36; x++ {
		r, _, _, _ := screen.GetContent(x, 0)
		tabBuilder.WriteRune(r)
	}
	tabText := tabBuilder.String()
	if !strings.Contains(tabText, "Keyword") || !strings.Contains(tabText, "BLAST") || !strings.Contains(tabText, "Explore") {
		t.Fatalf("shared tab frame did not draw top tabs: %q", tabText)
	}
}

func TestMainExploreListKeyboardNavigationAndNumberSelection(t *testing.T) {
	state := MainInterfaceState{Explore: MainExploreState{Tool: "open_session"}}
	_, groups := buildMainExploreTab(tview.NewApplication(), &state)
	if len(groups) != 1 || len(groups[0].controls) != 1 {
		t.Fatalf("unexpected explore focus groups: %#v", groups)
	}
	list, ok := groups[0].controls[0].(*mainExploreList)
	if !ok {
		t.Fatalf("explore control = %T, want *mainExploreList", groups[0].controls[0])
	}
	app := tview.NewApplication()
	handleMainExploreListKey(list, tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), app)
	if got := list.GetCurrentItem(); got != 1 {
		t.Fatalf("Down selected item %d, want 1", got)
	}
	if state.Explore.Tool != "new_canvas" {
		t.Fatalf("Down updated tool to %q, want new_canvas", state.Explore.Tool)
	}
	handleMainExploreListKey(list, tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone), app)
	if got := list.GetCurrentItem(); got != 0 {
		t.Fatalf("Up selected item %d, want 0", got)
	}
	handleMainExploreListKey(list, tcell.NewEventKey(tcell.KeyRune, '3', tcell.ModNone), app)
	if got := list.GetCurrentItem(); got != 2 {
		t.Fatalf("shortcut 3 selected item %d, want 2", got)
	}
	if state.Explore.Tool != "nwk_browser" {
		t.Fatalf("shortcut 3 updated tool to %q, want nwk_browser", state.Explore.Tool)
	}
	handleMainExploreListKey(list, tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), app)
	handleMainExploreListKey(list, tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), app)
	if got := list.GetCurrentItem(); got != 2 {
		t.Fatalf("Down should clamp at last item without wrapping, got %d", got)
	}
}

func TestMainDropDownClosedDoesNotConsumeArrowKeys(t *testing.T) {
	changed := ""
	drop := mainDropDown("Database", []Option{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}, "a", func(value string) {
		changed = value
	})
	app := tview.NewApplication()
	if handleMainDropDownKey(drop, tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), app) {
		t.Fatal("closed dropdown must not consume Down; module focus should handle it")
	}
	if drop.IsOpen() {
		t.Fatal("closed dropdown should not open on Down")
	}
	if changed != "" {
		t.Fatalf("Down changed dropdown value to %q", changed)
	}
}

func TestMainDropDownOpenConsumesArrowKeys(t *testing.T) {
	drop := mainDropDown("Database", []Option{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}, "a", nil)
	app := tview.NewApplication()
	if !handleMainDropDownKey(drop, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), app) {
		t.Fatal("Enter should open focused dropdown")
	}
	if !drop.IsOpen() {
		t.Fatal("dropdown should be open after Enter")
	}
	if !handleMainDropDownKey(drop, tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), app) {
		t.Fatal("open dropdown should consume Down")
	}
}

func TestMainAliasButtonVisibleOnlyOnSymbolColumn(t *testing.T) {
	rows := []MainKeywordRow{{SearchTerm: "PAL", SymbolName: "PAL1", Aliases: []string{"PAL1", "PAL2"}}}
	state := GridEditorState{ActiveRow: 0, ActiveCol: 0}
	grid := newKeywordGridEditor(&rows, &state, nil, mainKeywordSearchType("phytozome"))
	if grid.activeColumnIs("symbol") {
		t.Fatal("test setup should start on search term")
	}
	state.ActiveCol = 1
	if !grid.activeColumnIs("symbol") {
		t.Fatal("symbol column should be detected")
	}
	aliases := mainAliasChoices(rows[0].SymbolName, rows[0].Aliases)
	if strings.Join(aliases, ",") != "PAL1,PAL2" {
		t.Fatalf("unexpected alias choices: %#v", aliases)
	}
}

func TestMainNCBIKeywordExposesOnlySearchableSearchTypeOptions(t *testing.T) {
	cap := mainKeywordCapability("ncbi")
	if len(cap.SearchTypes) != len(ncbi.SearchableSearchTypes()) {
		t.Fatalf("expected %d visible NCBI search types, got %d", len(ncbi.SearchableSearchTypes()), len(cap.SearchTypes))
	}
	options := mainSearchTypeOptions(cap)
	if len(options) != len(ncbi.SearchableSearchTypes()) || options[0].Value != "protein" {
		t.Fatalf("unexpected NCBI search type options: %#v", options)
	}
	for _, hidden := range []string{"pubmed", "pmc", "books", "mesh", "gds", "geoprofiles", "pcassay"} {
		for _, option := range options {
			if option.Value == hidden {
				t.Fatalf("hidden NCBI search type %q should not appear in main search options", hidden)
			}
		}
	}
}

func TestMainNCBIKeywordShowsGeneLocusPriorityMenuOnlyForSupportedSearchTypes(t *testing.T) {
	app := tview.NewApplication()
	state := NormalizeMainInterfaceState(MainInterfaceState{Keyword: MainKeywordState{DatabaseID: "ncbi", SearchTypeID: "protein"}})
	module, _, _ := buildMainKeywordTab(app, &state, nil, nil)
	priority, ok := mainKeywordGeneLocusPriorityDropDown(module)
	if !ok {
		t.Fatal("NCBI protein search type should expose the Gene locus priority menu")
	}
	if _, label := priority.GetCurrentOption(); label != "None" {
		t.Fatalf("Gene locus priority default = %q, want None", label)
	}
	if state.Keyword.GeneLocusPriorityDatabase != GeneLocusPriorityNone || state.Keyword.PLAZAGeneLocusPriority {
		t.Fatalf("Gene locus priority must default to none: %#v", state.Keyword)
	}
	state.Keyword.SearchTypeID = "nuccore"
	state.Keyword.GeneLocusPriorityDatabase = GeneLocusPriorityPLAZA
	module, _, _ = buildMainKeywordTab(app, &state, nil, nil)
	if _, ok := mainKeywordGeneLocusPriorityDropDown(module); ok {
		t.Fatal("NCBI nuccore search type must not expose Gene locus priority")
	}
	if state.Keyword.GeneLocusPriorityDatabase != GeneLocusPriorityNone || state.Keyword.PLAZAGeneLocusPriority {
		t.Fatalf("hidden Gene locus priority must be reset: %#v", state.Keyword)
	}
}

func TestMainGeneLocusPriorityOptions(t *testing.T) {
	options := mainGeneLocusPriorityOptions()
	if got, want := len(options), 3; got != want {
		t.Fatalf("priority option count = %d, want %d", got, want)
	}
	for index, want := range []Option{
		{Value: GeneLocusPriorityNone, Label: "None"},
		{Value: GeneLocusPriorityNCBI, Label: "Use NCBI database"},
		{Value: GeneLocusPriorityPLAZA, Label: "Use PLAZA database"},
	} {
		if options[index] != want {
			t.Fatalf("option %d = %#v, want %#v", index, options[index], want)
		}
	}
}

func mainKeywordGeneLocusPriorityDropDown(module tview.Primitive) (*tview.DropDown, bool) {
	body, ok := module.(*buttonFlex)
	if !ok || body.GetItemCount() == 0 {
		return nil, false
	}
	options, ok := body.GetItem(0).(*buttonFlex)
	if !ok {
		return nil, false
	}
	for index := 0; index < options.GetItemCount(); index++ {
		field, ok := options.GetItem(index).(*mainControlField)
		if !ok || field.label != "Gene locus" {
			continue
		}
		dropDown, ok := field.child.(*tview.DropDown)
		return dropDown, ok
	}
	return nil, false
}

func TestMainProgramOptionsIncludeDirectionLabels(t *testing.T) {
	options := mainProgramOptions([]string{"blastn", "blastx", "tblastn", "blastp"})
	labels := make([]string, 0, len(options))
	for _, option := range options {
		labels = append(labels, option.Label)
	}
	joined := strings.Join(labels, "\n")
	for _, want := range []string{"nucleotide -> nucleotide", "translated nucleotide -> protein", "protein -> translated nucleotide", "protein -> protein"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("program labels missing %q:\n%s", want, joined)
		}
	}
}

func TestRunMainInterfacePageBuildsWithoutTerminalPanic(t *testing.T) {
	oldNewApp := newApp
	oldRunApp := runApp
	defer func() {
		newApp = oldNewApp
		runApp = oldRunApp
	}()
	var app *tview.Application
	newApp = func() *tview.Application {
		app = tview.NewApplication()
		return app
	}
	runApp = func(app *tview.Application) error {
		if app == nil {
			t.Fatal("expected application")
		}
		app.Stop()
		return nil
	}
	_, err := RunMainInterfacePage(MainInterfacePage{State: DefaultMainInterfaceState()})
	if err != nil {
		t.Fatalf("RunMainInterfacePage returned error: %v", err)
	}
}

func columnIDs(columns []gridColumn) []string {
	out := make([]string, 0, len(columns))
	for _, column := range columns {
		out = append(out, column.ID)
	}
	return out
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

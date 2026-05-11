package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestBlastSettingsModalHeightsFitCurrentContent(t *testing.T) {
	externalHeight := modalHeightForContent(3+3+2+2+3+7+1+2+1+5+1+4, 36, 46)
	if externalHeight < 36 || externalHeight > 46 {
		t.Fatalf("external reference modal height = %d, want within [36,46]", externalHeight)
	}

	familySettingsRows := 3 + 1 + 4 + 1 + 7 + 1 + 4
	familyContentRows := maxInt(18, familySettingsRows+2)
	familyHeight := modalHeightForContent(3+3+1+1+2+4+familyContentRows, 34, 48)
	if familyHeight < familySettingsRows+10 {
		t.Fatalf("family modal height = %d, too small for settings rows %d", familyHeight, familySettingsRows)
	}
	if familyHeight > 48 {
		t.Fatalf("family modal height = %d, want <= 48", familyHeight)
	}

	filterHeight := modalHeightForContent(3+maxInt(31, 46)+3+2, 50, 58)
	if filterHeight < 52 || filterHeight > 58 {
		t.Fatalf("filter modal height = %d, want within [52,58]", filterHeight)
	}
}

func TestBlastFilterSecondPageThreeColumnRowsFitModal(t *testing.T) {
	rankingRows := 2 + 1 + 5 + 1 + 2 + 10 + 1 + 4
	softScoreRows := 3 + 1 + 4 + 1 + 6 + 1 + 2
	referenceScoreRows := 2 + 1 + 5 + 1 + 4 + 1 + 2 + 1 + 6
	secondPageRows := maxInt(rankingRows, maxInt(softScoreRows, referenceScoreRows))
	firstPageRows := maxInt(31, 46)
	filterHeight := modalHeightForContent(3+maxInt(firstPageRows, secondPageRows)+3+2, 50, 58)

	if secondPageRows > firstPageRows {
		t.Fatalf("second page rows = %d, should fit within first-page height budget %d", secondPageRows, firstPageRows)
	}
	if filterHeight < 54 || filterHeight > 58 {
		t.Fatalf("filter modal height = %d, want within [54,58]", filterHeight)
	}
}

func TestBlastFilterRankingOrderInputFitsThreeColumnLayout(t *testing.T) {
	labelWidth := len([]rune("order "))
	fieldWidth := 24
	panelInnerWidth := 148/3 - 4

	if labelWidth+fieldWidth > panelInnerWidth {
		t.Fatalf("ranking priority input width = %d, panel inner width = %d", labelWidth+fieldWidth, panelInnerWidth)
	}
}

func TestBlastSettingsModalLabelsUseReadableText(t *testing.T) {
	for _, text := range []string{
		"Add UniProt annotation columns",
		"Add InterPro domain-evidence columns",
		"Group related queries as one family result",
		"Reject rows below the identity cutoff",
		"InterPro rule: use conserved-region status",
	} {
		if strings.Contains(text, "UseTarget") || strings.Contains(text, "InterProDomainMode") {
			t.Fatalf("label %q still looks like an internal field name", text)
		}
	}
}

func TestButtonRowKeepsLeftAndPrimaryButtonsVisibleOnWideRows(t *testing.T) {
	row := buttonRow(
		buttonSpec{Label: ButtonBack, Shortcut: ShortcutBack, Visible: true},
		buttonSpec{Label: ButtonHome, Shortcut: ShortcutHome, Visible: true},
		buttonSpec{Label: ButtonSelectAll, Shortcut: ShortcutSelectAll, Visible: true},
		buttonSpec{Label: ButtonClear, Shortcut: ShortcutClear, Visible: true},
		buttonSpec{Label: ButtonToggle, Shortcut: ShortcutToggle, Visible: true},
		buttonSpec{Label: ButtonExport, Shortcut: ShortcutExport, Visible: true, Primary: true},
		buttonSpec{Label: ButtonView, Shortcut: ShortcutConfirm, Visible: true, Primary: true},
	)

	positions := row.buttonPositions(180)
	if len(positions) != 7 {
		t.Fatalf("unexpected visible button count: got %d want 7", len(positions))
	}
	for _, pos := range positions {
		if pos.row != 0 {
			t.Fatalf("wide button row should not wrap, got %q on row %d", pos.label, pos.row)
		}
	}
	if got := row.requiredHeight(180); got != 1 {
		t.Fatalf("wide button row height = %d, want 1", got)
	}
}

func TestButtonRowWrapsOnlyWhenLeftAndPrimaryGroupsOverlap(t *testing.T) {
	row := buttonRow(
		buttonSpec{Label: ButtonBack, Shortcut: ShortcutBack, Visible: true},
		buttonSpec{Label: ButtonHome, Shortcut: ShortcutHome, Visible: true},
		buttonSpec{Label: ButtonSelectAll, Shortcut: ShortcutSelectAll, Visible: true},
		buttonSpec{Label: ButtonClear, Shortcut: ShortcutClear, Visible: true},
		buttonSpec{Label: ButtonToggle, Shortcut: ShortcutToggle, Visible: true},
		buttonSpec{Label: ButtonExport, Shortcut: ShortcutExport, Visible: true, Primary: true},
		buttonSpec{Label: ButtonView, Shortcut: ShortcutConfirm, Visible: true, Primary: true},
	)

	if got := row.requiredHeight(48); got <= 1 {
		t.Fatalf("narrow button row should wrap, got height %d", got)
	}
}

func TestButtonRowPositionsFitInsideRequiredHeightAtCommonWidths(t *testing.T) {
	row := buttonRow(
		buttonSpec{Label: ButtonBack, Shortcut: ShortcutBack, Visible: true},
		buttonSpec{Label: ButtonHome, Shortcut: ShortcutHome, Visible: true},
		buttonSpec{Label: ButtonSelectAll, Shortcut: ShortcutSelectAll, Visible: true},
		buttonSpec{Label: ButtonClear, Shortcut: ShortcutClear, Visible: true},
		buttonSpec{Label: ButtonToggle, Shortcut: ShortcutToggle, Visible: true},
		buttonSpec{Label: ButtonExport, Shortcut: ShortcutExport, Visible: true, Primary: true},
		buttonSpec{Label: ButtonView, Shortcut: ShortcutConfirm, Visible: true, Primary: true},
	)

	for _, width := range []int{64, 96, 128, 180} {
		height := row.requiredHeight(width)
		for _, pos := range row.buttonPositions(width) {
			if pos.row < 0 || pos.row >= height {
				t.Fatalf("button %q row %d is outside required height %d at width %d", pos.label, pos.row, height, width)
			}
			if pos.left < 0 || pos.right > width || pos.left >= pos.right {
				t.Fatalf("button %q has invalid x range [%d,%d) at width %d", pos.label, pos.left, pos.right, width)
			}
		}
	}
}

func TestButtonRowMouseLeftClickActivatesButton(t *testing.T) {
	activated := false
	row := buttonRow(buttonSpec{
		Label:    ButtonSearch,
		Shortcut: ShortcutApply,
		Action:   func() { activated = true },
		Visible:  true,
		Primary:  true,
	})
	row.SetRect(0, 0, 40, row.requiredHeight(40))
	positions := row.buttonPositions(40)
	if len(positions) != 1 {
		t.Fatalf("unexpected positions: got %d want 1", len(positions))
	}
	x := positions[0].left + (positions[0].right-positions[0].left)/2

	consumed, _ := row.MouseHandler()(tview.MouseLeftClick, tcell.NewEventMouse(x, positions[0].row, tcell.ButtonNone, 0), nil)
	if !consumed {
		t.Fatal("button row should consume mouse left click inside a button")
	}
	if !activated {
		t.Fatal("button mouse left click should activate the button action")
	}
}

func TestButtonRowPrimaryLabelUpdatesOnlyPrimaryButtons(t *testing.T) {
	row := buttonRow(
		buttonSpec{Label: ButtonSkip, Shortcut: ShortcutRetry, Visible: true},
		buttonSpec{Label: ButtonApply, Shortcut: ShortcutApply, Visible: true, Primary: true},
	)

	row.setPrimaryLabel(ButtonAuto)

	if row.buttons[0].Label != ButtonSkip {
		t.Fatalf("non-primary skip button label changed to %q", row.buttons[0].Label)
	}
	if row.buttons[1].Label != ButtonAuto {
		t.Fatalf("primary button label = %q, want %q", row.buttons[1].Label, ButtonAuto)
	}
}

func TestFamilyBlastCustomizeButtonSitsLeftOfApply(t *testing.T) {
	row := buttonRow(
		buttonSpec{Label: ButtonBack, Shortcut: ShortcutBack, Visible: true},
		buttonSpec{Label: ButtonHelp, Shortcut: ShortcutHelp, Visible: true},
		buttonSpec{Label: "Refresh", Shortcut: "Ctrl+R", Visible: true},
		buttonSpec{Label: "Customize groups", Shortcut: "Ctrl+G", Visible: true, Primary: true},
		buttonSpec{Label: ButtonApply, Shortcut: ShortcutApply, Visible: true, Primary: true},
	)
	positions := row.buttonPositions(132)
	customizeLeft := -1
	applyLeft := -1
	for _, pos := range positions {
		switch pos.button.Label {
		case "Customize groups":
			customizeLeft = pos.left
		case ButtonApply:
			applyLeft = pos.left
		}
	}
	if customizeLeft < 0 || applyLeft < 0 {
		t.Fatalf("missing primary buttons in positions: %#v", positions)
	}
	if customizeLeft >= applyLeft {
		t.Fatalf("customize button should sit left of Apply, got customize x=%d apply x=%d", customizeLeft, applyLeft)
	}
}

func TestButtonRowMouseDoesNotCaptureButton(t *testing.T) {
	row := buttonRow(buttonSpec{
		Label:    ButtonPaste,
		Shortcut: ShortcutPaste,
		Visible:  true,
	})
	row.SetRect(0, 0, 40, row.requiredHeight(40))
	positions := row.buttonPositions(40)
	if len(positions) != 1 {
		t.Fatalf("unexpected positions: got %d want 1", len(positions))
	}
	x := positions[0].left + (positions[0].right-positions[0].left)/2

	consumed, capture := row.MouseHandler()(tview.MouseLeftDown, tcell.NewEventMouse(x, positions[0].row, tcell.Button1, 0), nil)
	if !consumed {
		t.Fatal("button row should consume mouse left down inside a button")
	}
	if capture != nil {
		t.Fatal("button row should not capture mouse state after mouse down")
	}
}

func TestButtonFlexUsesDefaultMouseRoutingForButtonRows(t *testing.T) {
	activated := false
	body := newButtonFlex()
	content := tview.NewTextArea().SetText("", true)
	body.AddItem(content, 0, 1, true)
	row := buttonRow(buttonSpec{
		Label:    ButtonSearch,
		Shortcut: ShortcutApply,
		Action:   func() { activated = true },
		Visible:  true,
		Primary:  true,
	})
	addButtonRow(body, row)

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init failed: %v", err)
	}
	screen.SetSize(80, 12)
	body.SetRect(0, 0, 80, 12)
	body.Draw(screen)

	positions := row.buttonPositions(80)
	if len(positions) != 1 {
		t.Fatalf("unexpected button positions: got %d want 1", len(positions))
	}
	_, rowY, _, _ := row.GetRect()
	x := positions[0].left + (positions[0].right-positions[0].left)/2
	y := rowY + positions[0].row
	consumed, _ := body.MouseHandler()(tview.MouseLeftClick, tcell.NewEventMouse(x, y, tcell.ButtonNone, 0), nil)
	if !consumed {
		t.Fatal("button flex should route clicks to button rows")
	}
	if !activated {
		t.Fatal("button row should activate through default flex mouse routing")
	}
}

func TestResolveInputFileTextKeepsOrdinaryText(t *testing.T) {
	text, err := resolveInputFileText("LOC_Os03g11614\nOsMADS1")
	if err != nil {
		t.Fatalf("ordinary text should be accepted: %v", err)
	}
	if text != "LOC_Os03g11614\nOsMADS1" {
		t.Fatalf("text = %q", text)
	}
}

func TestResolveInputFileTextReadsFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.txt")
	if err := os.WriteFile(path, []byte("ATPAL1\nATPAL2\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	text, err := resolveInputFileText(`"` + path + `"`)
	if err != nil {
		t.Fatalf("file path should be read: %v", err)
	}
	if text != "ATPAL1\nATPAL2" {
		t.Fatalf("text = %q", text)
	}
}

func TestResolveInputFileTextRejectsUnreadableFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.txt")
	if text, err := resolveInputFileText(`"` + path + `"`); err == nil || text != "" {
		t.Fatalf("missing file should be rejected, got text=%q err=%v", text, err)
	}
}

func TestSearchResultOffsetKeepsSelectionVisibleWhenMovingDown(t *testing.T) {
	offset := searchResultOffsetForSelection(0, 3, 10, 4)
	if offset != 4 {
		t.Fatalf("offset = %d, want 4", offset)
	}
}

func TestSearchResultOffsetKeepsSelectionVisibleWhenMovingUp(t *testing.T) {
	offset := searchResultOffsetForSelection(8, 2, 10, 4)
	if offset != 4 {
		t.Fatalf("offset = %d, want 4", offset)
	}
}

func TestSearchResultOffsetStaysZeroWhenViewportFitsPage(t *testing.T) {
	offset := searchResultOffsetForSelection(0, 9, 10, 20)
	if offset != 0 {
		t.Fatalf("offset = %d, want 0", offset)
	}
}

func TestPageSelectorClickSelectsPageNumber(t *testing.T) {
	selector := &pageSelectorPrimitive{Box: tview.NewBox(), totalPages: 3, currentPage: 0, summary: "Settings page 1/3"}
	selected := -1
	selector.onSelect = func(page int) {
		selected = page
	}
	selector.SetRect(0, 0, 40, 3)

	lines := selector.pageLines(40, 3)
	if len(lines) == 0 || len(lines[0].segments) < 2 {
		t.Fatalf("page selector did not expose page segments: %#v", lines)
	}
	lineWidth := len([]rune(lines[0].text))
	left := (40 - lineWidth) / 2
	clickX := left + lines[0].segments[1].left + 1
	clickY := 1
	consumed, _ := selector.MouseHandler()(tview.MouseLeftClick, tcell.NewEventMouse(clickX, clickY, tcell.ButtonNone, 0), nil)

	if !consumed {
		t.Fatal("page selector should consume clicks on page numbers")
	}
	if selected != 1 {
		t.Fatalf("selected page = %d, want 1", selected)
	}
}

func TestRowSelectionGroupsKeepEmptyExplicitGroups(t *testing.T) {
	rows := []TableRow{
		{Group: "alpha", Cells: []string{"A"}},
		{Group: "gamma", Cells: []string{"G"}},
	}
	groups := rowSelectionGroups(rows, []string{"alpha", "beta", "gamma"})
	if len(groups) != 3 {
		t.Fatalf("group count = %d, want 3", len(groups))
	}
	if groups[1].Label != "beta" || len(groups[1].Rows) != 0 || !groups[1].Explicit {
		t.Fatalf("empty explicit group not preserved: %#v", groups[1])
	}
	if len(groups[0].Rows) != 1 || groups[0].Rows[0] != 0 {
		t.Fatalf("alpha rows not linked: %#v", groups[0])
	}
	if len(groups[2].Rows) != 1 || groups[2].Rows[0] != 1 {
		t.Fatalf("gamma rows not linked: %#v", groups[2])
	}
}

func TestChoiceModalOptionsPrependCloseWhenAllowed(t *testing.T) {
	choices := choiceModalOptions(ChoiceModalPage{
		AllowClose: true,
		Choices: []Choice{{
			Value:       "next",
			Label:       "Next",
			Description: "continue",
		}},
	})
	if len(choices) != 2 {
		t.Fatalf("choice count = %d, want 2", len(choices))
	}
	if choices[0].Value != "close" || choices[0].Label != ButtonClose {
		t.Fatalf("first choice should be Close, got %#v", choices[0])
	}
	if choices[1].Value != "next" {
		t.Fatalf("original choice shifted incorrectly: %#v", choices[1])
	}
}

func TestBlastHeaderSplitsIntoTwoRowsWithSlash(t *testing.T) {
	top, bottom := tableHeaderLines("align_len /\nquery_length (%)")
	if top != "align_len /" {
		t.Fatalf("top header = %q, want slash on first line", top)
	}
	if bottom != "query_length (%)" {
		t.Fatalf("bottom header = %q", bottom)
	}

	layout := newRowSelectionLayout([]TableColumn{{Header: "align_len /\nquery_length (%)"}})
	if !layout.headerTwoLine || layout.firstDataRow != 3 || layout.dividerRow != 2 {
		t.Fatalf("two-line layout not activated: %#v", layout)
	}
}

func TestUniProtReviewedCellColor(t *testing.T) {
	column := TableColumn{ID: "uniprot_reviewed"}
	if got := tableCellColor(column, "reviewed"); got != colorSelectionOn {
		t.Fatalf("reviewed color = %v", got)
	}
	if got := tableCellColor(column, "unreviewed"); got != colorMuted {
		t.Fatalf("unreviewed color = %v", got)
	}
	if got := tableCellColor(column, ""); got != tview.Styles.PrimaryTextColor {
		t.Fatalf("empty reviewed color = %v", got)
	}
}

func TestIndentSecondaryPreservesMultiLineDetails(t *testing.T) {
	got := indentSecondary("PAL4\n5 lines")
	if got != "  PAL4\n  5 lines" {
		t.Fatalf("secondary text = %q", got)
	}
}

func TestBlastRunSidebarDrawsSecondaryAsTwoPhysicalLines(t *testing.T) {
	sidebar := newBlastRunSidebar()
	sidebar.SetItems([]blastRunSidebarItem{{
		Primary:   "AT1G12345",
		Secondary: []string{"PAL4"},
		Lines:     "5 lines",
	}})
	sidebar.SetCurrentItem(0)

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init failed: %v", err)
	}
	screen.SetSize(24, 7)
	sidebar.SetRect(0, 0, 24, 7)
	sidebar.Draw(screen)

	if !containsText(screenLine(screen, 1, 24), "AT1G12345") {
		t.Fatalf("primary line missing: %q", screenLine(screen, 1, 24))
	}
	if !containsText(screenLine(screen, 2, 24), "PAL4") {
		t.Fatalf("label line missing: %q", screenLine(screen, 2, 24))
	}
	if !containsText(screenLine(screen, 3, 24), "5 lines") {
		t.Fatalf("lines line missing: %q", screenLine(screen, 3, 24))
	}
}

func TestBlastRunSidebarDrawsMemberLabelsAsSeparateLines(t *testing.T) {
	sidebar := newBlastRunSidebar()
	sidebar.SetItems([]blastRunSidebarItem{{
		Primary:   "AT1G12345.1",
		Secondary: []string{"[VND]", "VND6", "VND7"},
		Lines:     "12/12 lines",
	}})
	sidebar.SetCurrentItem(0)

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init failed: %v", err)
	}
	screen.SetSize(24, 8)
	sidebar.SetRect(0, 0, 24, 8)
	sidebar.Draw(screen)

	if !containsText(screenLine(screen, 1, 24), "AT1G12345.1") {
		t.Fatalf("primary line missing: %q", screenLine(screen, 1, 24))
	}
	if !containsText(screenLine(screen, 2, 24), "[VND]") {
		t.Fatalf("family label line missing: %q", screenLine(screen, 2, 24))
	}
	if !containsText(screenLine(screen, 3, 24), "VND6") {
		t.Fatalf("first member line missing: %q", screenLine(screen, 3, 24))
	}
	if !containsText(screenLine(screen, 4, 24), "VND7") {
		t.Fatalf("second member line missing: %q", screenLine(screen, 4, 24))
	}
	if !containsText(screenLine(screen, 5, 24), "12/12 lines") {
		t.Fatalf("lines line missing: %q", screenLine(screen, 5, 24))
	}
}

func TestRowSelectionTableKeepsTrailingAreaDrawable(t *testing.T) {
	table := &rowSelectionTable{Table: tview.NewTable().
		SetBorders(false).
		SetSeparator(tview.Borders.Vertical).
		SetSelectable(true, true).
		SetFixed(2, 2).
		SetEvaluateAllRows(true)}
	table.SetCell(0, 0, paddedTableCell("[x]"))
	table.SetCell(0, 1, paddedTableCell("row"))
	table.SetCell(0, 2, paddedTableCell("short"))
	table.SetCell(0, 3, paddedTableCell("very_long_column_header"))
	table.SetCell(1, 0, paddedTableCell(""))
	table.SetCell(1, 1, paddedTableCell(""))
	table.SetCell(1, 2, paddedTableCell(""))
	table.SetCell(1, 3, paddedTableCell(""))
	table.SetCell(2, 0, paddedTableCell("[x]"))
	table.SetCell(2, 1, paddedTableCell("1"))
	table.SetCell(2, 2, paddedTableCell("A"))
	table.SetCell(2, 3, paddedTableCell("BBBBBBBBBBBBBBBBBBBB"))

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init failed: %v", err)
	}
	screen.SetSize(38, 6)
	table.SetRect(0, 0, 38, 6)
	table.Draw(screen)

	line := screenLine(screen, 0, 38)
	if !containsText(line, "short") {
		t.Fatalf("complete first data column should remain visible: %q", line)
	}
	if containsText(line, "very_long_column_header") {
		t.Fatalf("full oversized trailing data column should not be forced into the viewport: %q", line)
	}
}

func TestRowSelectionTableDoesNotBlankWideViewport(t *testing.T) {
	table := &rowSelectionTable{Table: tview.NewTable().
		SetBorders(false).
		SetSeparator(tview.Borders.Vertical).
		SetSelectable(true, true).
		SetFixed(2, 2).
		SetEvaluateAllRows(true)}
	table.SetCell(0, 0, paddedTableCell("[x]"))
	table.SetCell(0, 1, paddedTableCell("row"))
	table.SetCell(0, 2, paddedTableCell("very_very_very_very_wide_header"))
	table.SetCell(0, 3, paddedTableCell("fit"))
	table.SetCell(1, 0, paddedTableCell(""))
	table.SetCell(1, 1, paddedTableCell(""))
	table.SetCell(1, 2, paddedTableCell(""))
	table.SetCell(1, 3, paddedTableCell(""))
	table.SetCell(2, 0, paddedTableCell("[x]"))
	table.SetCell(2, 1, paddedTableCell("1"))
	table.SetCell(2, 2, paddedTableCell("AAAAAAAAAAAAAAAAAAAAAAAAAAAA"))
	table.SetCell(2, 3, paddedTableCell("B"))
	table.SetOffset(0, 0)

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init failed: %v", err)
	}
	screen.SetSize(80, 6)
	table.SetRect(0, 0, 80, 6)
	table.Draw(screen)

	line := screenLine(screen, 0, 80)
	if !containsText(line, "fit") {
		t.Fatalf("trailing complete data column should remain visible on wide screens: %q", line)
	}
}

func TestClippedPrimitiveBlocksChildOverflowBelowItsRect(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init failed: %v", err)
	}
	screen.SetSize(20, 6)
	bgStyle := tcell.StyleDefault.Foreground(tcell.ColorWhite)
	for y := 0; y < 6; y++ {
		for x := 0; x < 20; x++ {
			screen.SetContent(x, y, '.', nil, bgStyle)
		}
	}

	child := &overflowPrimitive{Box: tview.NewBox()}
	clipped := clipPrimitive(child)
	clipped.SetRect(2, 1, 8, 2)
	clipped.Draw(screen)

	if main, _, _, _ := screen.GetContent(3, 1); main != 'I' {
		t.Fatalf("expected child content inside clip rect, got %q", main)
	}
	if main, _, _, _ := screen.GetContent(3, 4); main != '.' {
		t.Fatalf("expected overflow below clip rect to be blocked, got %q", main)
	}
}

func TestFamilyBlastCustomizeModalStartsInteractiveImmediately(t *testing.T) {
	app := newApp()
	var result FamilyBlastResult
	modal := buildFamilyBlastCustomizeModal(FamilyBlastCustomizePage{
		Title:     "Customize Family BLAST groups",
		Groups:    []FamilyBlastCustomGroup{{Name: "PAL", Labels: []string{"PAL1", "PAL2"}}},
		Ungrouped: []string{"PAL3", "PAL4"},
		AllowBack: true,
	}, app, &result)

	if modal == nil || modal.groupedList == nil || modal.rightList == nil {
		t.Fatal("expected customize modal to expose interactive lists")
	}
	if app.GetFocus() != modal.groupedList {
		t.Fatalf("initial focus = %T, want grouped list", app.GetFocus())
	}
	if got := modal.groupedList.GetCurrentItem(); got != 0 {
		t.Fatalf("initial grouped selection = %d, want 0", got)
	}

	if app.GetFocus() != modal.groupedList {
		t.Fatalf("focus should stay on grouped list without deferred first-draw focus reset, got %T", app.GetFocus())
	}
}

func TestFamilyBlastCustomizeModalKeyboardNavigationAndTabSwitch(t *testing.T) {
	app := newApp()
	var result FamilyBlastResult
	modal := buildFamilyBlastCustomizeModal(FamilyBlastCustomizePage{
		Title: "Customize Family BLAST groups",
		Groups: []FamilyBlastCustomGroup{
			{Name: "PAL", Labels: []string{"PAL1", "PAL2"}},
			{Name: "CAD", Labels: []string{"CAD1", "CAD2"}},
		},
		Ungrouped: []string{"X1", "X2", "X3"},
		AllowBack: true,
	}, app, &result)

	capture := app.GetInputCapture()
	if capture == nil {
		t.Fatal("expected input capture to be installed")
	}
	capture(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	if got := modal.groupedList.GetCurrentItem(); got != 1 {
		t.Fatalf("grouped selection after Down = %d, want 1", got)
	}
	capture(tcell.NewEventKey(tcell.KeyTab, 0, 0))
	if app.GetFocus() != modal.rightList {
		t.Fatalf("focus after Tab = %T, want right list", app.GetFocus())
	}
	capture(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	if got := modal.rightList.GetCurrentItem(); got != 1 {
		t.Fatalf("right selection after Down = %d, want 1", got)
	}
	capture(tcell.NewEventKey(tcell.KeyEnd, 0, 0))
	if got := modal.rightList.GetCurrentItem(); got != 2 {
		t.Fatalf("right selection after End = %d, want 2", got)
	}
}

func TestFamilyBlastCustomizeModalDisplaysMemberProteinIDs(t *testing.T) {
	app := newApp()
	var result FamilyBlastResult
	modal := buildFamilyBlastCustomizeModal(FamilyBlastCustomizePage{
		Title: "Customize Family BLAST groups",
		Groups: []FamilyBlastCustomGroup{{
			Name: "PAL",
			Members: []FamilyBlastMember{
				{LabelName: "PAL1", ProteinID: "PAC:1", SourceKey: "pal1"},
				{LabelName: "PAL2", ProteinID: "PAC:2", SourceKey: "pal2"},
			},
		}},
		UngroupedMembers: []FamilyBlastMember{{LabelName: "PAL3", ProteinID: "PAC:3", SourceKey: "pal3"}},
		AllowBack:        true,
	}, app, &result)

	if modal.groupedList.GetItemCount() < 2 {
		t.Fatalf("grouped item count = %d, want member row", modal.groupedList.GetItemCount())
	}
	mainText, secondary := modal.groupedList.GetItemText(1)
	if !strings.Contains(mainText, "PAL1") || !strings.Contains(mainText, "[yellow][PAC:1][-]") || secondary != "" {
		t.Fatalf("grouped member row = %q / %q, want inline PAL1 [yellow][PAC:1][-]", mainText, secondary)
	}
	mainText, secondary = modal.rightList.GetItemText(0)
	if !strings.Contains(mainText, "PAL3") || !strings.Contains(mainText, "[yellow][PAC:3][-]") || secondary != "" {
		t.Fatalf("right member row = %q / %q, want inline PAL3 [yellow][PAC:3][-]", mainText, secondary)
	}
}

func TestFamilyBlastCustomizeModalMouseSelectsRightPaneWithoutSnapBack(t *testing.T) {
	app := newApp()
	var result FamilyBlastResult
	modal := buildFamilyBlastCustomizeModal(FamilyBlastCustomizePage{
		Title: "Customize Family BLAST groups",
		Groups: []FamilyBlastCustomGroup{
			{Name: "PAL", Labels: []string{"PAL1", "PAL2"}},
		},
		Ungrouped: []string{"X1", "X2", "X3"},
		AllowBack: true,
	}, app, &result)

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init failed: %v", err)
	}
	screen.SetSize(160, 40)
	modal.root.SetRect(0, 0, 160, 40)
	modal.root.Draw(screen)

	x, y, _, _ := modal.rightList.GetInnerRect()
	mouse := tcell.NewEventMouse(x+1, y+1, tcell.Button1, 0)
	consumed, _ := modal.rightList.MouseHandler()(tview.MouseLeftClick, mouse, func(p tview.Primitive) {
		app.SetFocus(p)
	})
	if !consumed {
		t.Fatal("right list should consume mouse click")
	}
	if app.GetFocus() != modal.rightList {
		t.Fatalf("focus after right click = %T, want right list", app.GetFocus())
	}
}

func TestFamilyBlastCustomizeModalMouseDownDoesNotSwitchActivePane(t *testing.T) {
	app := newApp()
	var result FamilyBlastResult
	modal := buildFamilyBlastCustomizeModal(FamilyBlastCustomizePage{
		Title: "Customize Family BLAST groups",
		Groups: []FamilyBlastCustomGroup{
			{Name: "PAL", Labels: []string{"PAL1", "PAL2"}},
		},
		Ungrouped: []string{"X1", "X2", "X3"},
		AllowBack: true,
	}, app, &result)

	capture := app.GetInputCapture()
	capture(tcell.NewEventKey(tcell.KeyTab, 0, 0))
	if app.GetFocus() != modal.rightList {
		t.Fatalf("focus after Tab = %T, want right list", app.GetFocus())
	}

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init failed: %v", err)
	}
	screen.SetSize(160, 40)
	modal.root.SetRect(0, 0, 160, 40)
	modal.root.Draw(screen)

	x, y, _, _ := modal.groupedList.GetInnerRect()
	mouse := tcell.NewEventMouse(x+1, y+1, tcell.Button1, 0)
	modal.groupedList.MouseHandler()(tview.MouseLeftDown, mouse, func(p tview.Primitive) {
		app.SetFocus(p)
	})
	if app.GetFocus() != modal.rightList {
		t.Fatalf("focus after grouped mouse down = %T, want right list until click", app.GetFocus())
	}

	consumed, _ := modal.groupedList.MouseHandler()(tview.MouseLeftClick, mouse, func(p tview.Primitive) {
		app.SetFocus(p)
	})
	if !consumed {
		t.Fatal("grouped list should consume mouse click")
	}
	if app.GetFocus() != modal.groupedList {
		t.Fatalf("focus after grouped mouse click = %T, want grouped list", app.GetFocus())
	}
}

func TestFamilyBlastCustomizeModalChooseGroupOverlayLeavesExtraRows(t *testing.T) {
	app := newApp()
	var result FamilyBlastResult
	modal := buildFamilyBlastCustomizeModal(FamilyBlastCustomizePage{
		Title: "Customize Family BLAST groups",
		Groups: []FamilyBlastCustomGroup{
			{Name: "PAL", Labels: []string{"PAL1", "PAL2"}},
			{Name: "CAD", Labels: []string{"CAD1", "CAD2"}},
			{Name: "CCR", Labels: []string{"CCR1", "CCR2"}},
		},
		Ungrouped: []string{"X1", "X2", "X3"},
		AllowBack: true,
	}, app, &result)

	capture := app.GetInputCapture()
	capture(tcell.NewEventKey(tcell.KeyTab, 0, 0))
	capture(tcell.NewEventKey(tcell.KeyEnter, 0, 0))

	if got, wantMin := modal.chooseGroupOverlayHeight, 12; got < wantMin {
		t.Fatalf("choose-group overlay height = %d, want at least %d", got, wantMin)
	}
	if got, wantExact := modal.chooseGroupOverlayHeight, 12; got != wantExact {
		t.Fatalf("choose-group overlay height = %d, want %d for 3 groups with extra padding", got, wantExact)
	}
}

func TestFamilyBlastCustomizeModalShowsOnlyActiveListSelection(t *testing.T) {
	app := newApp()
	var result FamilyBlastResult
	modal := buildFamilyBlastCustomizeModal(FamilyBlastCustomizePage{
		Title:     "Customize Family BLAST groups",
		Groups:    []FamilyBlastCustomGroup{{Name: "PAL", Labels: []string{"PAL1", "PAL2"}}},
		Ungrouped: []string{"X1", "X2"},
		AllowBack: true,
	}, app, &result)

	if listSelectedFocusOnly(modal.groupedList) {
		t.Fatal("active grouped list should show its selected row")
	}
	if !listSelectedFocusOnly(modal.rightList) {
		t.Fatal("inactive right list should hide its selected row")
	}

	capture := app.GetInputCapture()
	capture(tcell.NewEventKey(tcell.KeyTab, 0, 0))
	if !listSelectedFocusOnly(modal.groupedList) {
		t.Fatal("inactive grouped list should hide its selected row after Tab")
	}
	if listSelectedFocusOnly(modal.rightList) {
		t.Fatal("active right list should show its selected row after Tab")
	}
}

func TestFamilyBlastCustomizeSubModalRestoresParentSelection(t *testing.T) {
	app := newApp()
	var result FamilyBlastResult
	modal := buildFamilyBlastCustomizeModal(FamilyBlastCustomizePage{
		Title: "Customize Family BLAST groups",
		Groups: []FamilyBlastCustomGroup{
			{Name: "PAL", Labels: []string{"PAL1", "PAL2"}},
			{Name: "CAD", Labels: []string{"CAD1", "CAD2"}},
		},
		Ungrouped: []string{"X1", "X2", "X3"},
		AllowBack: true,
	}, app, &result)

	capture := app.GetInputCapture()
	capture(tcell.NewEventKey(tcell.KeyTab, 0, 0))
	capture(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	if got := modal.rightList.GetCurrentItem(); got != 1 {
		t.Fatalf("right selection before modal = %d, want 1", got)
	}

	capture(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	capture(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	capture(tcell.NewEventKey(tcell.KeyEscape, 0, 0))

	if app.GetFocus() != modal.rightList {
		t.Fatalf("focus after closing submodal = %T, want right list", app.GetFocus())
	}
	if got := modal.rightList.GetCurrentItem(); got != 1 {
		t.Fatalf("right selection after closing submodal = %d, want 1", got)
	}
	if listSelectedFocusOnly(modal.rightList) {
		t.Fatal("right list should remain the single active selected list after closing submodal")
	}
	if !listSelectedFocusOnly(modal.groupedList) {
		t.Fatal("grouped list should remain visually inactive after closing submodal")
	}
}

func listSelectedFocusOnly(list *tview.List) bool {
	value := reflect.ValueOf(list).Elem().FieldByName("selectedFocusOnly")
	if !value.IsValid() || value.Kind() != reflect.Bool {
		return false
	}
	return value.Bool()
}

type overflowPrimitive struct {
	*tview.Box
}

func screenLine(screen tcell.SimulationScreen, y int, width int) string {
	runes := make([]rune, 0, width)
	for x := 0; x < width; x++ {
		main, _, _, _ := screen.GetContent(x, y)
		if main == 0 {
			main = ' '
		}
		runes = append(runes, main)
	}
	return string(runes)
}

func containsText(value string, text string) bool {
	return strings.Contains(value, text)
}

func (o *overflowPrimitive) Draw(screen tcell.Screen) {
	x, y, width, height := o.GetRect()
	for row := 0; row < height+3; row++ {
		for col := 0; col < width; col++ {
			ch := 'O'
			if row < height {
				ch = 'I'
			}
			screen.SetContent(x+col, y+row, ch, nil, tcell.StyleDefault)
		}
	}
}

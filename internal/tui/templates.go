package tui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/KiriKirby/phytozome-go/internal/perf"
)

var ErrTaskCancelled = errors.New("task cancelled")

type NavAction string

const (
	NavNone    NavAction = ""
	NavBack    NavAction = "back"
	NavHome    NavAction = "home"
	NavExit    NavAction = "exit"
	NavRefresh NavAction = "refresh"
)

type Choice struct {
	Value       string
	Label       string
	Description string
}

type ChoicePage struct {
	Breadcrumb  string
	Path        []string
	Title       string
	Description string
	Choices     []Choice
	AllowBack   bool
	AllowHome   bool
	ConfirmText string
	Hints       []string
}

type ChoiceGroup struct {
	Label       string
	Description string
	Choices     []Choice
}

type GroupedChoicePage struct {
	Breadcrumb  string
	Path        []string
	Title       string
	Description string
	Groups      []ChoiceGroup
	Initial     string
	AllowBack   bool
	AllowHome   bool
	ConfirmText string
	Hints       []string
}

type ChoiceResult struct {
	Value string
	Nav   NavAction
}

type TextInputPage struct {
	Breadcrumb    string
	Path          []string
	Title         string
	Description   string
	Label         string
	Initial       string
	Placeholder   string
	AllowEmpty    bool
	SkipWhenEmpty bool
	AllowBack     bool
	AllowHome     bool
	ConfirmText   string
	Hints         []string
}

type TextInputResult struct {
	Text string
	Nav  NavAction
}

type MultiLinePage struct {
	Breadcrumb    string
	Path          []string
	Title         string
	Description   string
	Initial       string
	AllowEmpty    bool
	SkipWhenEmpty bool
	AllowBack     bool
	AllowHome     bool
	ConfirmText   string
	EmptyText     string
	EmptyAction   string
	SkipText      string
	SkipShortcut  string
	Hints         []string
}

type MultiLineResult struct {
	Text   string
	Nav    NavAction
	Action string
}

type TableColumn struct {
	ID        string
	Header    string
	Width     int
	Sortable  bool
	Reference string
	Help      string
}

type TableRow struct {
	Cells        []string
	Group        string
	Detail       string
	FilterFlag   bool
	FilterReason string
}

type SortDirection int

const (
	SortAscending SortDirection = iota
	SortDescending
)

const (
	rowSelectionHeaderRow       = 0
	rowSelectionDividerRow      = 1
	rowSelectionFirstDataRow    = 2
	rowSelectionFirstDataColumn = 2
)

type rowSelectionLayout struct {
	headerRows    int
	dividerRow    int
	firstDataRow  int
	headerTwoLine bool
}

func newRowSelectionLayout(columns []TableColumn) rowSelectionLayout {
	layout := rowSelectionLayout{
		headerRows:   1,
		dividerRow:   rowSelectionDividerRow,
		firstDataRow: rowSelectionFirstDataRow,
	}
	for _, column := range columns {
		header := tableHeaderText(firstNonEmptyText(column.Header, column.ID))
		if strings.Contains(header, "\n") {
			layout.headerRows = 2
			layout.dividerRow = 2
			layout.firstDataRow = 3
			layout.headerTwoLine = true
			return layout
		}
	}
	return layout
}

type TableSort struct {
	Column    int
	Direction SortDirection
}

type RowSelectionPage struct {
	Breadcrumb   string
	Path         []string
	Title        string
	Description  string
	Columns      []TableColumn
	Rows         []TableRow
	Selected     []bool
	FilterFlags  []bool
	Sort         TableSort
	GroupSort    bool
	GroupLabels  []string
	AllowFilter  bool
	FilterText   string
	AllowDoneAll bool
	AllowBack    bool
	AllowHome    bool
	ConfirmText  string
	GenerateText string
	DoneAllText  string
	Hints        []string
	State        RowSelectionState
}

type RowSelectionState struct {
	Valid          bool
	SelectedRow    int
	SelectedColumn int
	RowOffset      int
	ColumnOffset   int
	Sort           TableSort
	ControlHeaders bool
	HeaderColumn   int
}

type BlastRunItem struct {
	Label       string
	AltLabel    string
	Description string
	Columns     []TableColumn
	Rows        []TableRow
	Selected    []bool
	FilterFlags []bool
}

type BlastRunSelectionPage struct {
	Breadcrumb   string
	Path         []string
	Title        string
	Description  string
	Items        []BlastRunItem
	AllowFilter  bool
	FilterText   string
	AllowBack    bool
	AllowHome    bool
	ConfirmText  string
	GenerateText string
	DoneAllText  string
	Hints        []string
	State        BlastRunSelectionState
}

type BlastRunTableState struct {
	Valid          bool
	SelectedRow    int
	SelectedColumn int
	RowOffset      int
	ColumnOffset   int
}

type BlastRunSelectionState struct {
	Valid        bool
	CurrentRun   int
	ControlMode  int
	ListOffset   int
	Sort         TableSort
	HeaderColumn int
	Tables       []BlastRunTableState
}

type BlastRunSelectionResult struct {
	RunIndex         int
	Selected         []bool
	SelectedByRun    [][]bool
	FilterFlagsByRun [][]bool
	FilterRequested  bool
	GenerateFile     bool
	DoneAll          bool
	Nav              NavAction
	State            BlastRunSelectionState
}

type buttonRowPrimitive struct {
	*tview.Box
	buttons []buttonSpec
}

type buttonFlex struct {
	*tview.Flex
	rows            []*buttonRowPrimitive
	lastLayoutWidth int
}

type localizedHelpPage struct {
	Label    string
	Shortcut string
	Title    string
	Text     string
}

type localizedHelpModal struct {
	pages       []localizedHelpPage
	helpBody    *buttonFlex
	helpTitle   *tview.TextView
	helpText    *tview.TextView
	helpButtons *buttonRowPrimitive
	index       int
}

type clippedPrimitive struct {
	*tview.Box
	child tview.Primitive
}

type focusProxyPrimitive struct {
	*tview.Box
	child       tview.Primitive
	focusTarget func() tview.Primitive
}

type clippingScreen struct {
	tcell.Screen
	x      int
	y      int
	width  int
	height int
}

type pageSelectorPrimitive struct {
	*tview.Box
	totalPages  int
	currentPage int
	matches     int
	summary     string
	onSelect    func(page int)
}

type blastRunSidebarItem struct {
	Primary   string
	Secondary []string
	Lines     string
}

type blastRunSidebar struct {
	*tview.Box
	items   []blastRunSidebarItem
	current int
	offset  int
	changed func(index int)
}

type pasteStatus struct {
	view  *tview.TextView
	seq   atomic.Uint64
	focus func()
}

type rowSelectionTable struct {
	*tview.Table
	dividerRow   int
	columnWidths []int
}

type checkboxModule struct {
	*tview.Box
	label   string
	checked func() bool
	toggle  func()
}

func newCheckboxModule(label string, checked func() bool, toggle func()) *checkboxModule {
	return &checkboxModule{
		Box:     tview.NewBox(),
		label:   strings.TrimSpace(label),
		checked: checked,
		toggle:  toggle,
	}
}

func newBlastRunSidebar() *blastRunSidebar {
	sidebar := &blastRunSidebar{Box: tview.NewBox()}
	sidebar.SetBorder(true)
	sidebar.SetTitle(" BLAST queries ")
	sidebar.SetTitleAlign(tview.AlignCenter)
	return sidebar
}

func (b *blastRunSidebar) SetItems(items []blastRunSidebarItem) {
	b.items = append([]blastRunSidebarItem(nil), items...)
	b.clamp()
}

func (b *blastRunSidebar) SetCurrentItem(index int) {
	b.current = index
	b.clamp()
	b.keepCurrentVisible()
}

func (b *blastRunSidebar) GetOffset() int {
	b.clamp()
	return b.offset
}

func (b *blastRunSidebar) SetOffset(offset int) {
	b.offset = offset
	b.clamp()
}

func (b *blastRunSidebar) SetChangedFunc(changed func(index int)) {
	b.changed = changed
}

func (b *blastRunSidebar) clamp() {
	if len(b.items) == 0 {
		b.current = 0
		b.offset = 0
		return
	}
	if b.current < 0 {
		b.current = 0
	}
	if b.current >= len(b.items) {
		b.current = len(b.items) - 1
	}
	maxOffset := b.totalRows() - 1
	if b.offset < 0 {
		b.offset = 0
	}
	if b.offset > maxOffset {
		b.offset = maxOffset
	}
}

func (b *blastRunSidebar) keepCurrentVisible() {
	_, _, _, height := b.GetInnerRect()
	if height <= 0 {
		return
	}
	top := b.itemStartRow(b.current)
	bottom := top + b.itemHeight(b.current) - 1
	if top < b.offset {
		b.offset = top
	} else if bottom >= b.offset+height {
		b.offset = bottom - height + 1
	}
	b.clamp()
}

func (b *blastRunSidebar) itemHeight(index int) int {
	if index < 0 || index >= len(b.items) {
		return 0
	}
	height := 2 + len(b.items[index].Secondary)
	if height < 3 {
		height = 3
	}
	return height
}

func (b *blastRunSidebar) itemStartRow(index int) int {
	if index <= 0 {
		return 0
	}
	if index > len(b.items) {
		index = len(b.items)
	}
	row := 0
	for i := 0; i < index; i++ {
		row += b.itemHeight(i)
	}
	return row
}

func (b *blastRunSidebar) totalRows() int {
	total := 0
	for i := range b.items {
		total += b.itemHeight(i)
	}
	return total
}

func (b *blastRunSidebar) rowToItem(physicalRow int) (int, int) {
	if physicalRow < 0 {
		return -1, 0
	}
	row := 0
	for i := range b.items {
		height := b.itemHeight(i)
		if physicalRow < row+height {
			return i, physicalRow - row
		}
		row += height
	}
	return -1, 0
}

func (b *blastRunSidebar) choose(index int) {
	if index < 0 || index >= len(b.items) {
		return
	}
	changed := index != b.current
	b.current = index
	b.keepCurrentVisible()
	if changed && b.changed != nil {
		b.changed(index)
	}
}

func (b *blastRunSidebar) Draw(screen tcell.Screen) {
	b.Box.DrawForSubclass(screen, b)
	innerX, innerY, innerWidth, innerHeight := b.GetInnerRect()
	if innerWidth <= 0 || innerHeight <= 0 {
		return
	}
	b.clamp()
	selectedStyle := tcell.StyleDefault.Background(tview.Styles.ContrastBackgroundColor).Foreground(tview.Styles.InverseTextColor)
	primaryStyle := tcell.StyleDefault.Background(tview.Styles.PrimitiveBackgroundColor).Foreground(tview.Styles.PrimaryTextColor)
	secondaryStyle := tcell.StyleDefault.Background(tview.Styles.PrimitiveBackgroundColor).Foreground(tview.Styles.SecondaryTextColor)
	for row := 0; row < innerHeight; row++ {
		physicalRow := b.offset + row
		itemIndex, lineIndex := b.rowToItem(physicalRow)
		if itemIndex < 0 || itemIndex >= len(b.items) {
			continue
		}
		item := b.items[itemIndex]
		text := item.Primary
		style := primaryStyle
		if lineIndex > 0 && lineIndex <= len(item.Secondary) {
			text = "  " + item.Secondary[lineIndex-1]
			style = secondaryStyle
		} else if lineIndex == b.itemHeight(itemIndex)-1 {
			text = "  " + item.Lines
			style = secondaryStyle
		}
		if itemIndex == b.current {
			style = selectedStyle
		}
		for x := 0; x < innerWidth; x++ {
			screen.SetContent(innerX+x, innerY+row, ' ', nil, style)
		}
		printStyledText(screen, innerX, innerY+row, innerWidth, style, text)
	}
}

func (b *blastRunSidebar) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return b.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyUp:
			b.choose(b.current - 1)
		case tcell.KeyDown:
			b.choose(b.current + 1)
		case tcell.KeyHome:
			b.choose(0)
		case tcell.KeyEnd:
			b.choose(len(b.items) - 1)
		case tcell.KeyPgUp:
			_, _, _, height := b.GetInnerRect()
			b.choose(b.current - maxInt(1, height/maxInt(1, b.itemHeight(b.current))))
		case tcell.KeyPgDn:
			_, _, _, height := b.GetInnerRect()
			b.choose(b.current + maxInt(1, height/maxInt(1, b.itemHeight(b.current))))
		}
	})
}

func (b *blastRunSidebar) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return b.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		if event == nil || !b.InRect(event.Position()) {
			return false, nil
		}
		x, y := event.Position()
		innerX, innerY, innerWidth, innerHeight := b.GetInnerRect()
		if x < innerX || x >= innerX+innerWidth || y < innerY || y >= innerY+innerHeight {
			return false, nil
		}
		switch action {
		case tview.MouseLeftDown:
			if setFocus != nil {
				setFocus(b)
			}
			return true, nil
		case tview.MouseLeftClick:
			if setFocus != nil {
				setFocus(b)
			}
			itemIndex, _ := b.rowToItem(b.offset + y - innerY)
			b.choose(itemIndex)
			return true, nil
		case tview.MouseScrollUp:
			b.offset--
			b.clamp()
			return true, nil
		case tview.MouseScrollDown:
			b.offset++
			b.clamp()
			return true, nil
		}
		return false, nil
	})
}

func (t *rowSelectionTable) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	base := t.Table.MouseHandler()
	return t.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		if event == nil || !t.InRect(event.Position()) {
			return false, nil
		}
		switch action {
		case tview.MouseScrollLeft:
			t.scrollColumns(-1)
			return true, t
		case tview.MouseScrollRight:
			t.scrollColumns(1)
			return true, t
		}
		if base != nil {
			return base(action, event, setFocus)
		}
		return false, nil
	})
}

func (t *rowSelectionTable) scrollColumns(delta int) {
	if t == nil || delta == 0 {
		return
	}
	rowOffset, columnOffset := t.GetOffset()
	columnOffset += delta
	if columnOffset < 0 {
		columnOffset = 0
	}
	maxOffset := t.GetColumnCount() - rowSelectionFirstDataColumn - 1
	if maxOffset < 0 {
		maxOffset = 0
	}
	if columnOffset > maxOffset {
		columnOffset = maxOffset
	}
	t.SetOffset(rowOffset, columnOffset)
}

func (c *checkboxModule) toggleChecked() {
	if c == nil || c.toggle == nil {
		return
	}
	c.toggle()
}

func (c *checkboxModule) Draw(screen tcell.Screen) {
	c.Box.DrawForSubclass(screen, c)
	x, y, width, height := c.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}
	checked := false
	if c.checked != nil {
		checked = c.checked()
	}
	marker := "[ ]"
	markerStyle := tcell.StyleDefault.Foreground(colorSelectionOff).Background(tview.Styles.PrimitiveBackgroundColor)
	if checked {
		marker = "[x]"
		markerStyle = tcell.StyleDefault.Foreground(colorSelectionOn).Background(tview.Styles.PrimitiveBackgroundColor)
	}
	textStyle := tcell.StyleDefault.Foreground(tview.Styles.PrimaryTextColor).Background(tview.Styles.PrimitiveBackgroundColor)
	if c.HasFocus() {
		textStyle = tcell.StyleDefault.Foreground(colorAction).Background(tview.Styles.PrimitiveBackgroundColor).Bold(true)
	}
	lineY := y
	if height == 1 {
		lineY = y + height/2
	}
	labelX := x + 5
	labelWidth := maxInt(0, width-5)
	printStyledText(screen, x+1, lineY, width-1, markerStyle, marker)
	label := c.label
	if label == "" {
		label = "Option"
	}
	for i, line := range wrapPlainText(label, labelWidth) {
		if i >= height {
			break
		}
		printStyledText(screen, labelX, y+i, labelWidth, textStyle, line)
	}
}

func (c *checkboxModule) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return c.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if event == nil {
			return
		}
		if event.Key() == tcell.KeyRune && event.Rune() == ' ' {
			c.toggleChecked()
		}
	})
}

func (c *checkboxModule) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return c.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		if event == nil || !c.InRect(event.Position()) {
			return false, nil
		}
		if action == tview.MouseLeftDown {
			if setFocus != nil {
				setFocus(c)
			}
			return true, nil
		}
		if action == tview.MouseLeftClick {
			if setFocus != nil {
				setFocus(c)
			}
			c.toggleChecked()
			return true, nil
		}
		return false, nil
	})
}

type rowSelectionGroup struct {
	Label    string
	Rows     []int
	Explicit bool
}

type rowSelectionDisplayRow struct {
	IsGroup     bool
	Group       string
	OriginalRow int
}

type RowSelectionResult struct {
	Selected        []bool
	FilterFlags     []bool
	FilterRequested bool
	GenerateFile    bool
	DoneAll         bool
	Nav             NavAction
	State           RowSelectionState
}

type TaskPage struct {
	Breadcrumb  string
	Path        []string
	Title       string
	Description string
	Initial     string
	Total       int
	CancelError error
}

type InfoPage struct {
	Breadcrumb  string
	Path        []string
	Title       string
	Message     string
	AllowBack   bool
	AllowHome   bool
	ConfirmText string
	Hints       []string
}

type InfoResult struct {
	Nav NavAction
}

type Action struct {
	Value    string
	Label    string
	Shortcut string
	Primary  bool
}

type ActionModalPage struct {
	Breadcrumb   string
	Path         []string
	Title        string
	Message      string
	Actions      []Action
	ConfirmText  string
	ConfirmValue string
}

type ActionModalResult struct {
	Value string
	Nav   NavAction
}

type RecoveryModalPage struct {
	Breadcrumb string
	Path       []string
	Title      string
	Message    string
	AllowSkip  bool
}

type ChoiceModalPage struct {
	Breadcrumb  string
	Path        []string
	Title       string
	Message     string
	Choices     []Choice
	ConfirmText string
	AllowClose  bool
}

type ExportSettingsPage struct {
	Breadcrumb     string
	Path           []string
	Title          string
	Message        string
	FileLabel      string
	FileInitial    string
	FolderLabel    string
	AllowFolder    bool
	AllowEmptyFile bool
	ReportLabel    string
	ReportInitial  bool
	WriteText      bool
	WriteExcel     bool
	WriteRawExcel  bool
	AllowBack      bool
	AllowHome      bool
	ConfirmText    string
}

type ExportSettingsResult struct {
	FileName      string
	FolderName    string
	WriteReport   bool
	WriteText     bool
	WriteExcel    bool
	WriteRawExcel bool
	Nav           NavAction
}

type ExternalReferencePage struct {
	Breadcrumb       string
	Path             []string
	Title            string
	Message          string
	UniProtLabel     string
	UniProtInitial   bool
	InterProLabel    string
	InterProInitial  bool
	InterProSettings InterProConservedRegionSettings
	AllowBack        bool
	AllowHome        bool
	ConfirmText      string
}

type ExternalReferenceResult struct {
	UseUniProt       bool
	UseInterPro      bool
	InterProSettings InterProConservedRegionSettings
	Nav              NavAction
}

type BlastFilterPage struct {
	Breadcrumb  string
	Path        []string
	Title       string
	Message     string
	Settings    BlastFilterSettings
	AllowBack   bool
	AllowHome   bool
	ConfirmText string
}

type BlastFilterResult struct {
	Settings    BlastFilterSettings
	ClearFilter bool
	Nav         NavAction
}

type FamilyBlastPage struct {
	Breadcrumb              string
	Path                    []string
	Title                   string
	Message                 string
	Reference               string
	Groups                  []FamilyBlastGroup
	PreviewNote             string
	PreviewUngrouped        []string
	PreviewUngroupedMembers []FamilyBlastMember
	Settings                FamilyBlastSettings
	AllowBack               bool
	AllowHome               bool
	ConfirmText             string
}

type FamilyBlastGroup struct {
	Name    string
	Labels  []string
	Members []FamilyBlastMember
	Queries int
}

type FamilyBlastCustomGroup struct {
	Name    string
	Labels  []string
	Members []FamilyBlastMember
}

type FamilyBlastMember struct {
	LabelName         string
	ProteinID         string
	Aliases           []string
	OriginalLabelName string
	SourceKey         string
}

type FamilyBlastCustomizePage struct {
	Breadcrumb       string
	Path             []string
	Title            string
	Message          string
	Groups           []FamilyBlastCustomGroup
	Ungrouped        []string
	UngroupedMembers []FamilyBlastMember
	AllowBack        bool
	AllowHome        bool
	ConfirmText      string
}

type FamilyBlastResult struct {
	Settings     FamilyBlastSettings
	CustomGroups []FamilyBlastCustomGroup
	Nav          NavAction
}

type FamilyBlastSettings struct {
	Enabled                    bool
	GroupByDetectedPrefix      bool
	MergeRowsByTarget          bool
	KeepBestHitPerTarget       bool
	PrependOnlyFirstQuery      bool
	CustomizeGroups            bool
	MinimumGroupSize           string
	StripArabidopsisPrefix     bool
	StripLeadingSpeciesPrefix  bool
	StripTrailingQueryIndex    bool
	StripAfterNumberSuffix     bool
	NormalizeInnerPunctuation  bool
	StripTerminalSubtypeSuffix bool
	KeepDistinctQuerySubgroups bool
	UseUniProtReference        bool
	UseInterProReference       bool
	RankingTieBreakerOrder     string
}

type BlastFilterSettings struct {
	MinIdentityPercent                        string
	MinAlignQueryCoveragePercent              string
	MaxEValue                                 string
	UseTargetCanonicalLengthRatio             bool
	RequireTargetCanonicalLengthRatio         bool
	MinTargetCanonicalLengthPercent           string
	MaxTargetCanonicalLengthPercent           string
	UseTargetQueryLengthRatio                 bool
	RequireTargetQueryLengthRatio             bool
	MinTargetQueryLengthPercent               string
	MaxTargetQueryLengthPercent               string
	RequireUniProtAccession                   bool
	PreferUniProtReviewed                     bool
	RejectUniProtFragments                    bool
	RejectUniProtSequenceCautions             bool
	InterProDomainMode                        string
	RequireInterProConservedRegion            bool
	AllowInterProPartial                      bool
	RejectInterProMissing                     bool
	RejectInterProUncertain                   bool
	MinInterProCoveragePercent                string
	RequireInterProCoverageWhenUsed           bool
	AllowStrongBlastFallbackWithoutReferences bool
	StrongBlastFallbackMinIdentityPercent     string
	StrongBlastFallbackMaxEValue              string
	StrongBlastFallbackMinTargetQueryPercent  string
	StrongBlastFallbackMaxTargetQueryPercent  string
	RequireFamilyConsensusForStrongFallback   bool
	StrongFallbackMinFamilyConsensusSupport   string
	StrongFallbackMinFamilyConsensusPercent   string
	UseFamilySemanticAgreement                bool
	RequireFamilySemanticAgreement            bool
	FamilySemanticMinTokenMatches             string
	FamilySemanticMinAgreementPercent         string
	FamilySemanticAllowStrongReferenceBypass  bool
	KeepBestIsoformPerTargetGene              bool
	KeepTopHitsPerQuery                       bool
	TopHitsPerQuery                           string
	RankingTieBreakerOrder                    string
	PreferHigherFilterScoreWhenRanking        bool
	PreferLowerEValueWhenTies                 bool
	PreferHigherIdentityWhenTies              bool
	PreferHigherCoverageWhenTies              bool
	PreferHigherReferenceScoreWhenTies        bool
	PreferHigherBitscoreWhenTies              bool
	RejectIfAnyHardRuleFails                  bool
	EnableSoftScore                           bool
	MinSoftScore                              string
	IdentityWeight                            string
	CoverageWeight                            string
	LengthRatioWeight                         string
	TargetQueryLengthWeight                   string
	InterProWeight                            string
	InterProPartialWeight                     string
	InterProCoverageWeight                    string
	UniProtReviewedWeight                     string
	UniProtAnnotationWeight                   string
	FamilySemanticAgreementWeight             string
	PenaltySequenceCaution                    string
	PenaltyFragment                           string
	InterProPresentReferenceScore             string
	InterProPartialReferenceScore             string
	InterProUncertainReferenceScore           string
	InterProMissingReferencePenalty           string
	InterProCoverageReferenceDivisor          string
	UniProtAccessionReferenceScore            string
	UniProtReviewedReferenceScore             string
	UniProtAnnotationReferenceScore           string
	FamilySemanticReferenceScore              string
	FragmentReferencePenaltyMultiplier        string
	SequenceCautionReferencePenaltyMultiplier string
	LengthNearDistancePercent                 string
	LengthNearReferenceScore                  string
	LengthAcceptableDistancePercent           string
	LengthAcceptableReferenceScore            string
	LengthFarDistancePercent                  string
	LengthFarReferencePenalty                 string
}

type InterProConservedRegionSettings struct {
	UsePfamAccession       bool
	UseInterProAccession   bool
	UseSignatureAccession  bool
	UseEntryType           bool
	UseEntryName           bool
	UseCoverage            bool
	UseMatchRegions        bool
	PresentMinCoverage     string
	PartialMinCoverage     string
	PresentMinMatchedItems string
	PartialMinMatchedItems string
}

type SearchPage struct {
	Breadcrumb  string
	Path        []string
	Title       string
	Description string
	Label       string
	Initial     string
	Placeholder string
	Choices     []Choice
	Filter      func(query string, choices []Choice) []Choice
	AllowBack   bool
	AllowHome   bool
	Hints       []string
}

type SearchResult struct {
	Value string
	Query string
	Nav   NavAction
}

func RunChoicePage(page ChoicePage) (ChoiceResult, error) {
	if len(page.Choices) == 0 {
		return ChoiceResult{}, fmt.Errorf("missing choices")
	}

	app := newApp()
	var result ChoiceResult
	list := tview.NewList()
	for i, choice := range page.Choices {
		shortcut := rune(0)
		if strings.TrimSpace(choice.Value) != "" {
			shortcut = rune('1' + i)
		}
		list.AddItem(choice.Label, indentSecondary(choice.Description), shortcut, nil)
	}
	list.SetBorder(true)
	list.SetTitle(" " + trimColon(page.Title) + " ")
	list.SetTitleAlign(tview.AlignCenter)
	list.ShowSecondaryText(true)
	list.SetMainTextColor(tview.Styles.PrimaryTextColor)
	list.SetSecondaryTextColor(tview.Styles.SecondaryTextColor)
	list.SetSelectedTextColor(tview.Styles.InverseTextColor)
	list.SetSelectedBackgroundColor(tview.Styles.ContrastBackgroundColor)
	setFocusBorder(list.Box, true)
	attachFocusBorder(list.Box)

	confirm := func() {
		index := currentItem(list)
		if index < 0 || index >= len(page.Choices) || strings.TrimSpace(page.Choices[index].Value) == "" {
			moveChoiceSelection(list, page.Choices, 1)
			index = currentItem(list)
			if index < 0 || index >= len(page.Choices) || strings.TrimSpace(page.Choices[index].Value) == "" {
				return
			}
		}
		result.Value = page.Choices[index].Value
		app.Stop()
	}
	body := newButtonFlex()
	if strings.TrimSpace(page.Description) != "" {
		body.AddItem(textBlock(page.Description), 2, 0, false)
	}
	body.AddItem(list, 0, 1, true)
	addButtonRow(body, navButtons(page.AllowBack, page.AllowHome, true, page.ConfirmText, func(nav NavAction) {
		result.Nav = nav
		app.Stop()
	}, confirm))
	addHints(body, page.Hints)

	setPageRoot(app, pageFrame(pageBreadcrumb(page.Breadcrumb, page.Path), body))
	app.SetFocus(list)
	moveChoiceSelection(list, page.Choices, 1)
	installInputCapture(app, navCapture(app, page.AllowBack, page.AllowHome, func(nav NavAction) {
		result.Nav = nav
		app.Stop()
	}, keyBinding{Match: func(event *tcell.EventKey) bool {
		if selectionKey(event) && app.GetFocus() != list {
			app.SetFocus(list)
		}
		return false
	}}, keyBinding{Key: tcell.KeyEnter, Action: confirm}, keyBinding{Key: tcell.KeyUp, Action: func() { moveChoiceSelection(list, page.Choices, -1) }}, keyBinding{Key: tcell.KeyDown, Action: func() { moveChoiceSelection(list, page.Choices, 1) }}))

	if err := runApp(app); err != nil {
		return ChoiceResult{}, err
	}
	return result, nil
}

func RunGroupedChoicePage(page GroupedChoicePage) (ChoiceResult, error) {
	if len(page.Groups) == 0 {
		return ChoiceResult{}, fmt.Errorf("missing choice groups")
	}

	app := newApp()
	var result ChoiceResult
	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false)
	table.SetBorder(true)
	table.SetTitle(" " + trimColon(page.Title) + " ")
	table.SetTitleAlign(tview.AlignCenter)
	table.SetSelectedStyle(tcell.StyleDefault.Background(colorPanel).Foreground(colorAction).Bold(true))
	setFocusBorder(table.Box, true)
	attachFocusBorder(table.Box)

	rowValues := map[int]string{}
	optionRows := []int{}
	row := 0
	optionNumber := 1
	firstSelectableRow := -1
	initialRow := -1
	initialValue := strings.TrimSpace(page.Initial)
	for _, group := range page.Groups {
		groupLabel := strings.TrimSpace(group.Label)
		if groupLabel != "" {
			table.SetCell(row, 0, tview.NewTableCell(groupLabel).
				SetTextColor(colorAction).
				SetAttributes(tcell.AttrBold).
				SetSelectable(false))
			row++
			if description := strings.TrimSpace(group.Description); description != "" {
				table.SetCell(row, 1, tview.NewTableCell(indentSecondary(description)).
					SetTextColor(tview.Styles.SecondaryTextColor).
					SetExpansion(1).
					SetSelectable(false))
				row++
			}
		}
		for _, choice := range group.Choices {
			value := strings.TrimSpace(choice.Value)
			if value == "" {
				continue
			}
			numberText := fmt.Sprintf("(%d)", optionNumber)
			currentRow := row
			rowValues[currentRow] = value
			optionRows = append(optionRows, currentRow)
			if initialRow < 0 && initialValue != "" && strings.EqualFold(value, initialValue) {
				initialRow = currentRow
			}
			table.SetCell(currentRow, 0, tview.NewTableCell(numberText).
				SetTextColor(tview.Styles.SecondaryTextColor).
				SetAlign(tview.AlignRight).
				SetSelectable(true))
			table.SetCell(currentRow, 1, tview.NewTableCell(strings.TrimSpace(choice.Label)).
				SetTextColor(tview.Styles.PrimaryTextColor).
				SetAttributes(tcell.AttrBold).
				SetExpansion(1).
				SetSelectable(true))
			if firstSelectableRow < 0 {
				firstSelectableRow = currentRow
			}
			row++
			if description := strings.TrimSpace(choice.Description); description != "" {
				table.SetCell(row, 1, tview.NewTableCell(indentSecondary(description)).
					SetTextColor(tview.Styles.SecondaryTextColor).
					SetExpansion(1).
					SetSelectable(false))
				row++
			}
			optionNumber++
		}
		row++
	}
	if len(rowValues) == 0 {
		return ChoiceResult{}, fmt.Errorf("missing selectable choices")
	}

	selectRow := func(target int, delta int) {
		if delta == 0 {
			delta = 1
		}
		if target < 0 {
			target = firstSelectableRow
		}
		for step := 0; step <= row; step++ {
			if _, ok := rowValues[target]; ok {
				table.Select(target, 0)
				return
			}
			target = (target + delta + row + 1) % (row + 1)
		}
		table.Select(firstSelectableRow, 0)
	}
	confirm := func() {
		selectedRow, _ := table.GetSelection()
		value := rowValues[selectedRow]
		if strings.TrimSpace(value) == "" {
			selectRow(selectedRow, 1)
			selectedRow, _ = table.GetSelection()
			value = rowValues[selectedRow]
		}
		if strings.TrimSpace(value) == "" {
			return
		}
		result.Value = value
		app.Stop()
	}
	table.SetSelectedFunc(func(row int, _ int) {
		if _, ok := rowValues[row]; ok {
			confirm()
		}
	})

	body := newButtonFlex()
	if strings.TrimSpace(page.Description) != "" {
		body.AddItem(textBlock(page.Description), 2, 0, false)
	}
	body.AddItem(table, 0, 1, true)
	addButtonRow(body, navButtons(page.AllowBack, page.AllowHome, true, page.ConfirmText, func(nav NavAction) {
		result.Nav = nav
		app.Stop()
	}, confirm))
	addHints(body, page.Hints)

	setPageRoot(app, pageFrame(pageBreadcrumb(page.Breadcrumb, page.Path), body))
	app.SetFocus(table)
	if initialRow < 0 {
		initialRow = firstSelectableRow
	}
	table.Select(initialRow, 0)
	installInputCapture(app, navCapture(app, page.AllowBack, page.AllowHome, func(nav NavAction) {
		result.Nav = nav
		app.Stop()
	}, keyBinding{Match: func(event *tcell.EventKey) bool {
		if selectionKey(event) && app.GetFocus() != table {
			app.SetFocus(table)
		}
		return false
	}}, keyBinding{Key: tcell.KeyEnter, Action: confirm}, keyBinding{Key: tcell.KeyUp, Action: func() {
		current, _ := table.GetSelection()
		selectRow(current-1, -1)
	}}, keyBinding{Key: tcell.KeyDown, Action: func() {
		current, _ := table.GetSelection()
		selectRow(current+1, 1)
	}}, keyBinding{Match: func(event *tcell.EventKey) bool {
		return event.Key() == tcell.KeyRune && event.Rune() >= '1' && event.Rune() <= '9'
	}, ActionEvent: func(event *tcell.EventKey) {
		index := int(event.Rune() - '1')
		if index >= 0 && index < len(optionRows) {
			table.Select(optionRows[index], 0)
		}
	}}))

	if err := runApp(app); err != nil {
		return ChoiceResult{}, err
	}
	return result, nil
}

func RunTextInputPage(page TextInputPage) (TextInputResult, error) {
	app := newApp()
	var result TextInputResult
	input := tview.NewInputField().
		SetLabel(page.Label + " ").
		SetText(page.Initial).
		SetPlaceholder(page.Placeholder).
		SetFieldWidth(-1)
	input.SetBorder(true)
	input.SetTitle(" " + trimColon(page.Title) + " ")
	input.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(input.Box, true)
	attachFocusBorder(input.Box)

	pasteStatus := newPasteStatus(func() { app.SetFocus(input) })
	confirm := func() {
		text, err := resolveInputFileText(input.GetText())
		if err != nil {
			showInputFileError(pasteStatus, err)
			return
		}
		if text == "" && !page.AllowEmpty {
			return
		}
		result.Text = text
		app.Stop()
	}
	paste := func() {
		runInlinePaste(app, pasteStatus, func(text string) {
			if handler := input.PasteHandler(); handler != nil {
				handler(text, func(p tview.Primitive) { app.SetFocus(p) })
			}
		})
	}
	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			confirm()
		}
	})
	body := newButtonFlex()
	if strings.TrimSpace(page.Description) != "" {
		body.AddItem(textBlock(page.Description), 3, 0, false)
	}
	inputFrame := clipPrimitive(input)
	body.AddItem(inputFrame, 3, 0, false)
	body.AddItem(pasteStatus.view, 1, 0, false)
	buttons := inputButtons(page.AllowBack, page.AllowHome, inputConfirmText(page.ConfirmText, page.SkipWhenEmpty, "", input.GetText()), "Enter", paste, func(nav NavAction) {
		result.Nav = nav
		app.Stop()
	}, confirm)
	input.SetChangedFunc(func(text string) {
		buttons.setPrimaryLabel(inputConfirmText(page.ConfirmText, page.SkipWhenEmpty, "", text))
	})
	addButtonRow(body, buttons)
	addHints(body, page.Hints)

	root := pageFrame(pageBreadcrumb(page.Breadcrumb, page.Path), body)
	setPageRoot(app, root)
	app.SetFocus(inputFrame)
	installInputCapture(app, navCapture(app, page.AllowBack, page.AllowHome, func(nav NavAction) {
		result.Nav = nav
		app.Stop()
	}, keyBinding{Key: tcell.KeyCtrlV, Action: paste}))

	if err := runApp(app); err != nil {
		return TextInputResult{}, err
	}
	return result, nil
}

func RunMultiLinePage(page MultiLinePage) (MultiLineResult, error) {
	app := newApp()
	var result MultiLineResult
	area := tview.NewTextArea().
		SetText(page.Initial, true).
		SetPlaceholder("Type or paste text here.")
	area.SetBorder(true)
	area.SetTitle(" " + trimColon(page.Title) + " ")
	area.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(area.Box, true)
	attachFocusBorder(area.Box)

	pasteStatus := newPasteStatus(func() { app.SetFocus(area) })
	confirm := func() {
		text, err := resolveInputFileText(area.GetText())
		if err != nil {
			showInputFileError(pasteStatus, err)
			return
		}
		if text == "" && !page.AllowEmpty && strings.TrimSpace(page.EmptyAction) == "" {
			return
		}
		result.Text = text
		if text == "" && strings.TrimSpace(page.EmptyAction) != "" {
			result.Action = strings.TrimSpace(page.EmptyAction)
		} else {
			result.Action = "apply"
		}
		app.Stop()
	}
	skip := func() {
		result.Text = ""
		result.Action = "skip"
		app.Stop()
	}
	paste := func() {
		runInlinePaste(app, pasteStatus, func(text string) {
			if handler := area.PasteHandler(); handler != nil {
				handler(text, func(p tview.Primitive) { app.SetFocus(p) })
			}
		})
	}
	body := newButtonFlex()
	if strings.TrimSpace(page.Description) != "" {
		body.AddItem(textBlock(page.Description), 4, 0, false)
	}
	areaFrame := clipPrimitive(area)
	body.AddItem(areaFrame, 0, 1, false)
	body.AddItem(pasteStatus.view, 1, 0, false)
	primaryLabel := inputConfirmText(page.ConfirmText, page.SkipWhenEmpty, page.EmptyText, area.GetText())
	primaryShortcut := "Ctrl+Enter"
	buttons := buttonRow(
		buttonSpec{Label: ButtonBack, Shortcut: ShortcutBack, Action: func() { result.Nav = NavBack; app.Stop() }, Visible: page.AllowBack},
		buttonSpec{Label: ButtonHome, Shortcut: ShortcutHome, Action: func() { result.Nav = NavHome; app.Stop() }, Visible: page.AllowHome},
		buttonSpec{Label: ButtonPaste, Shortcut: ShortcutPaste, Action: paste, Visible: paste != nil},
	)
	if strings.TrimSpace(page.SkipText) != "" {
		shortcut := firstNonEmptyText(page.SkipShortcut, "Ctrl+K")
		buttons.buttons = append(buttons.buttons, buttonSpec{
			Label:    page.SkipText,
			Shortcut: shortcut,
			Action:   skip,
			Visible:  true,
		})
	}
	buttons.buttons = append(buttons.buttons, buttonSpec{
		Label:    conciseActionLabel(primaryLabel, ButtonApply),
		Shortcut: primaryShortcut,
		Action:   confirm,
		Visible:  true,
		Primary:  true,
	})
	area.SetChangedFunc(func() {
		buttons.setPrimaryLabel(inputConfirmText(page.ConfirmText, page.SkipWhenEmpty, page.EmptyText, area.GetText()))
		if buttons.primaryButton() != nil {
			buttons.primaryButton().Shortcut = primaryShortcut
		}
	})
	addButtonRow(body, buttons)
	addHints(body, append(page.Hints, "Ctrl+Enter uses the main action button. Enter inserts a new line. Paste (Ctrl+V) reads plain text from the clipboard."))

	root := pageFrame(pageBreadcrumb(page.Breadcrumb, page.Path), body)
	setPageRoot(app, root)
	app.SetFocus(areaFrame)
	installInputCapture(app, navCapture(app, page.AllowBack, page.AllowHome, func(nav NavAction) {
		result.Nav = nav
		app.Stop()
	}, keyBinding{Key: tcell.KeyCtrlV, Action: paste}, keyBinding{Match: func(event *tcell.EventKey) bool {
		return strings.TrimSpace(page.SkipText) != "" && shortcutMatchesEvent(firstNonEmptyText(page.SkipShortcut, "Ctrl+K"), event)
	}, Action: skip}, keyBinding{Match: isCtrlEnter, Action: confirm}))

	if err := runApp(app); err != nil {
		return MultiLineResult{}, err
	}
	return result, nil
}

func RunSearchPage(page SearchPage) (SearchResult, error) {
	if len(page.Choices) == 0 {
		return SearchResult{}, fmt.Errorf("missing search choices")
	}

	const pageSize = 10

	app := newApp()
	var result SearchResult
	query := strings.TrimSpace(page.Initial)
	filtered := make([]Choice, 0, len(page.Choices))
	currentPage := 0
	selectedIndex := 0
	resultRowOffset := 0
	var filterSeq atomic.Uint64
	filterReady := true

	input := tview.NewInputField().
		SetLabel(strings.TrimSpace(page.Label) + " ").
		SetText(query).
		SetPlaceholder(page.Placeholder).
		SetFieldWidth(-1)
	input.SetBorder(true)
	input.SetTitle(" " + trimColon(page.Title) + " ")
	input.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(input.Box, true)
	attachFocusBorder(input.Box)

	results := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false)
	results.SetBorder(true)
	results.SetTitle(" Results ")
	results.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(results.Box, false)
	attachFocusBorder(results.Box)

	pageBar := &pageSelectorPrimitive{Box: tview.NewBox()}

	clampSearchPage := func() {
		if len(filtered) == 0 {
			currentPage = 0
			return
		}
		maxPage := (len(filtered) - 1) / pageSize
		if currentPage > maxPage {
			currentPage = maxPage
			resultRowOffset = 0
		}
		if currentPage < 0 {
			currentPage = 0
			resultRowOffset = 0
		}
		start := currentPage * pageSize
		visible := len(filtered) - start
		if visible > pageSize {
			visible = pageSize
		}
		if visible <= 0 {
			selectedIndex = 0
		} else if selectedIndex >= visible {
			selectedIndex = visible - 1
		}
	}
	applyFilter := func() {
		if page.Filter != nil {
			filtered = page.Filter(query, page.Choices)
		} else {
			filtered = defaultChoiceFilter(query, page.Choices)
		}
		clampSearchPage()
	}
	var refresh func()
	var renderSearchResults func()
	selectCurrent := func(index int) {
		absolute := currentPage*pageSize + index
		if absolute < 0 || absolute >= len(filtered) {
			return
		}
		result.Value = filtered[absolute].Value
		result.Query = query
		app.Stop()
	}
	confirmCurrent := func() {
		selectCurrent(selectedIndex)
	}
	gotoPage := func(delta int) {
		if len(filtered) == 0 {
			return
		}
		maxPage := (len(filtered) - 1) / pageSize
		currentPage += delta
		if currentPage < 0 {
			currentPage = maxPage
		}
		if currentPage > maxPage {
			currentPage = 0
		}
		selectedIndex = 0
		resultRowOffset = 0
		refresh()
	}
	visibleCount := func() int {
		if len(filtered) == 0 {
			return 0
		}
		start := currentPage * pageSize
		visible := len(filtered) - start
		if visible > pageSize {
			visible = pageSize
		}
		if visible < 0 {
			return 0
		}
		return visible
	}
	selectResultIndex := func(index int, stop bool) {
		if index < 0 || index >= visibleCount() {
			return
		}
		selectedIndex = index
		if refresh != nil {
			refresh()
		}
		app.SetFocus(input)
		if stop {
			selectCurrent(index)
		}
	}
	moveSelection := func(delta int) {
		if len(filtered) == 0 {
			return
		}
		visible := visibleCount()
		if visible <= 0 {
			return
		}
		selectedIndex += delta
		if selectedIndex < 0 {
			gotoPage(-1)
			visible = visibleCount()
			if visible > 0 {
				selectedIndex = visible - 1
			}
			refresh()
			return
		}
		if selectedIndex >= visible {
			gotoPage(1)
			selectedIndex = 0
			refresh()
			return
		}
		refresh()
	}
	scrollCurrentSearchPage := func(delta int) {
		visible := visibleCount()
		if visible <= 0 {
			return
		}
		_, _, _, height := results.GetInnerRect()
		totalRows := visible * 2
		maxOffset := totalRows - height
		if maxOffset < 0 {
			maxOffset = 0
		}
		resultRowOffset += delta * 2
		if resultRowOffset < 0 {
			resultRowOffset = 0
		}
		if resultRowOffset > maxOffset {
			resultRowOffset = maxOffset
		}
		results.SetOffset(resultRowOffset, 0)
	}
	ensureSelectedResultVisible := func() {
		_, _, _, height := results.GetInnerRect()
		resultRowOffset = searchResultOffsetForSelection(resultRowOffset, selectedIndex, visibleCount(), height)
	}
	selectPage := func(pageIndex int) {
		if len(filtered) == 0 {
			return
		}
		maxPage := (len(filtered) - 1) / pageSize
		if pageIndex < 0 || pageIndex > maxPage {
			return
		}
		currentPage = pageIndex
		selectedIndex = 0
		resultRowOffset = 0
		refresh()
		app.SetFocus(input)
	}
	pageBar.onSelect = selectPage
	pasteStatus := newPasteStatus(func() { app.SetFocus(input) })
	paste := func() {
		runInlinePaste(app, pasteStatus, func(text string) {
			if handler := input.PasteHandler(); handler != nil {
				handler(text, func(p tview.Primitive) { app.SetFocus(p) })
			}
		})
	}
	renderSearchResults = func() {
		results.Clear()
		if !filterReady {
			resultRowOffset = 0
			results.SetCell(0, 0, tview.NewTableCell("- Searching").SetTextColor(colorAction).SetSelectable(false))
			results.SetCell(1, 0, tview.NewTableCell("  Filtering choices in the background...").SetTextColor(tview.Styles.SecondaryTextColor).SetSelectable(false))
		} else if len(filtered) == 0 {
			resultRowOffset = 0
			results.SetCell(0, 0, tview.NewTableCell("- No matches").SetTextColor(tview.Styles.PrimaryTextColor).SetSelectable(false))
			results.SetCell(1, 0, tview.NewTableCell("  Edit the search box to search again.").SetTextColor(tview.Styles.SecondaryTextColor).SetSelectable(false))
		} else {
			ensureSelectedResultVisible()
			start := currentPage * pageSize
			end := start + pageSize
			if end > len(filtered) {
				end = len(filtered)
			}
			for i, choice := range filtered[start:end] {
				nameStyle := tview.Styles.PrimaryTextColor
				detailStyle := tview.Styles.SecondaryTextColor
				if i == selectedIndex {
					nameStyle = colorAction
					detailStyle = colorAction
				}
				index := i
				results.SetCell(i*2, 0, tview.NewTableCell(choice.Label).
					SetTextColor(nameStyle).
					SetExpansion(1).
					SetClickedFunc(func() bool {
						selectResultIndex(index, false)
						return true
					}))
				results.SetCell(i*2+1, 0, tview.NewTableCell(indentSecondary(choice.Description)).
					SetTextColor(detailStyle).
					SetExpansion(1).
					SetClickedFunc(func() bool {
						selectResultIndex(index, false)
						return true
					}))
			}
		}
		results.SetOffset(resultRowOffset, 0)
		totalPages := 1
		if len(filtered) > 0 {
			totalPages = (len(filtered)-1)/pageSize + 1
		}
		pageBar.totalPages = totalPages
		pageBar.currentPage = currentPage
		pageBar.matches = len(filtered)
	}
	refresh = func() {
		applyFilter()
		renderSearchResults()
	}

	input.SetChangedFunc(func(text string) {
		query = text
		currentPage = 0
		selectedIndex = 0
		resultRowOffset = 0
		id := filterSeq.Add(1)
		if len(page.Choices) < 1000 {
			filterReady = true
			refresh()
			return
		}
		filterReady = false
		refresh()
		go func(querySnapshot string, seq uint64) {
			time.Sleep(perf.SearchDebounce())
			if filterSeq.Load() != seq {
				return
			}
			var next []Choice
			if page.Filter != nil {
				next = page.Filter(querySnapshot, page.Choices)
			} else {
				next = defaultChoiceFilter(querySnapshot, page.Choices)
			}
			app.QueueUpdateDraw(func() {
				if filterSeq.Load() != seq {
					return
				}
				filterReady = true
				filtered = next
				clampSearchPage()
				renderSearchResults()
			})
		}(text, id)
	})
	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			confirmCurrent()
		}
	})
	results.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		switch action {
		case tview.MouseScrollUp:
			scrollCurrentSearchPage(-1)
			return tview.MouseConsumed, nil
		case tview.MouseScrollDown:
			scrollCurrentSearchPage(1)
			return tview.MouseConsumed, nil
		default:
			return action, event
		}
	})

	body := newButtonFlex()
	if strings.TrimSpace(page.Description) != "" {
		body.AddItem(textBlock(page.Description), 2, 0, false)
	}
	inputFrame := clipPrimitive(input)
	body.AddItem(inputFrame, 3, 0, true)
	body.AddItem(pasteStatus.view, 1, 0, false)
	body.AddItem(results, 0, 1, false)
	body.AddItem(pageBar, 5, 0, false)
	addButtonRow(body, buttonRow(
		buttonSpec{Label: ButtonBack, Shortcut: ShortcutBack, Action: func() { result.Nav = NavBack; result.Query = query; app.Stop() }, Visible: page.AllowBack},
		buttonSpec{Label: ButtonHome, Shortcut: ShortcutHome, Action: func() { result.Nav = NavHome; result.Query = query; app.Stop() }, Visible: page.AllowHome},
		buttonSpec{Label: ButtonPaste, Shortcut: ShortcutPaste, Action: paste, Visible: true},
		buttonSpec{Label: ButtonSelect, Shortcut: ShortcutConfirm, Action: confirmCurrent, Visible: true, Primary: true},
	))
	addHints(body, append(page.Hints, "Type, delete, paste, and move the cursor at any time. Paste (Ctrl+V) reads from the clipboard. Up/Down choose a result. Enter uses the selected result. Tab/PgUp/PgDn changes page."))

	refresh()
	root := pageFrame(pageBreadcrumb(page.Breadcrumb, page.Path), body)
	setPageRoot(app, root)
	app.SetFocus(inputFrame)
	forwardToSearchInput := func(event *tcell.EventKey) {
		app.SetFocus(inputFrame)
		if handler := input.InputHandler(); handler != nil {
			handler(event, func(p tview.Primitive) { app.SetFocus(p) })
		}
	}
	installInputCapture(app, func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			if page.AllowBack {
				result.Nav = NavBack
				result.Query = query
				app.Stop()
				return nil
			}
		case tcell.KeyCtrlO:
			if page.AllowHome {
				result.Nav = NavHome
				result.Query = query
				app.Stop()
				return nil
			}
		case tcell.KeyCtrlV:
			paste()
			return nil
		case tcell.KeyRune:
			if event.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 {
				return event
			}
			forwardToSearchInput(event)
			return nil
		case tcell.KeyBackspace, tcell.KeyBackspace2, tcell.KeyDelete, tcell.KeyLeft, tcell.KeyRight, tcell.KeyHome, tcell.KeyEnd:
			forwardToSearchInput(event)
			return nil
		case tcell.KeyCtrlA, tcell.KeyCtrlE, tcell.KeyCtrlD, tcell.KeyCtrlK, tcell.KeyCtrlU, tcell.KeyCtrlW:
			forwardToSearchInput(event)
			return nil
		case tcell.KeyUp:
			moveSelection(-1)
			return nil
		case tcell.KeyDown:
			moveSelection(1)
			return nil
		case tcell.KeyTab, tcell.KeyPgDn:
			gotoPage(1)
			return nil
		case tcell.KeyBacktab, tcell.KeyPgUp:
			gotoPage(-1)
			return nil
		case tcell.KeyEnter:
			confirmCurrent()
			return nil
		}
		return event
	})

	if err := runApp(app); err != nil {
		return SearchResult{}, err
	}
	return result, nil
}

func RunRowSelectionPage(page RowSelectionPage) (RowSelectionResult, error) {
	if len(page.Rows) == 0 {
		return RowSelectionResult{Selected: append([]bool{}, page.Selected...)}, nil
	}
	app := newApp()
	selected := normalizeSelection(page.Selected, len(page.Rows), true)
	filterFlags := normalizeSelection(page.FilterFlags, len(page.Rows), false)
	order := make([]int, len(page.Rows))
	for i := range order {
		order[i] = i
	}
	groups := rowSelectionGroups(page.Rows, page.GroupLabels)
	sortState := page.Sort
	if sortState.Column < -1 || sortState.Column >= len(page.Columns) {
		sortState = TableSort{Column: -1, Direction: SortAscending}
	}
	if sortState.Column >= 0 && !page.Columns[sortState.Column].Sortable {
		sortState = TableSort{Column: -1, Direction: SortAscending}
	}
	if page.State.Valid && page.State.Sort.Column >= -1 && page.State.Sort.Column < len(page.Columns) {
		sortState = page.State.Sort
		if sortState.Column >= 0 && !page.Columns[sortState.Column].Sortable {
			sortState = TableSort{Column: -1, Direction: SortAscending}
		}
	}
	var result RowSelectionResult

	baseTable := tview.NewTable().
		SetBorders(false).
		SetSeparator(tview.Borders.Vertical).
		SetBordersColor(colorInactiveText).
		SetSelectable(true, true).
		SetFixed(2, 2).
		SetEvaluateAllRows(true).
		Select(2, 2)
	layout := newRowSelectionLayout(page.Columns)
	baseTable.SetFixed(layout.firstDataRow, rowSelectionFirstDataColumn).Select(layout.firstDataRow, rowSelectionFirstDataColumn)
	table := &rowSelectionTable{Table: baseTable, dividerRow: layout.dividerRow}
	table.SetBorder(true)
	table.SetTitle(" " + tableTitleWithCount(page.Title, countSelectedBools(selected), len(page.Rows)) + " ")
	table.SetTitleAlign(tview.AlignCenter)
	table.SetSelectedStyle(tcell.StyleDefault.Background(colorAction).Foreground(colorActionText).Bold(true))
	setFocusBorder(table.Box, true)
	attachFocusBorder(table.Box)

	modeText := tview.NewTextView().
		SetDynamicColors(true).
		SetTextColor(tview.Styles.SecondaryTextColor).
		SetTextAlign(tview.AlignCenter)
	controlHeaders := false
	headerColumn := rowSelectionFirstSortableColumn(page.Columns)
	if headerColumn < -1 {
		headerColumn = -1
	}
	if page.State.Valid && page.State.HeaderColumn >= -1 && page.State.HeaderColumn < len(page.Columns) {
		headerColumn = page.State.HeaderColumn
	}
	controlHeaders = page.State.Valid && page.State.ControlHeaders
	captureState := func() RowSelectionState {
		row, column := table.GetSelection()
		rowOffset, columnOffset := table.GetOffset()
		return RowSelectionState{
			Valid:          true,
			SelectedRow:    row,
			SelectedColumn: column,
			RowOffset:      rowOffset,
			ColumnOffset:   columnOffset,
			Sort:           sortState,
			ControlHeaders: controlHeaders,
			HeaderColumn:   headerColumn,
		}
	}
	displayColumn := func(dataColumn int) int {
		return dataColumn + 2
	}
	sortableDataColumn := func(dataColumn int) bool {
		if dataColumn == -1 {
			return true
		}
		return dataColumn >= 0 && dataColumn < len(page.Columns) && page.Columns[dataColumn].Sortable
	}
	allSelected := func() bool {
		if len(selected) == 0 {
			return false
		}
		for _, ok := range selected {
			if !ok {
				return false
			}
		}
		return true
	}
	var body *buttonFlex
	var pageRoot tview.Primitive
	modalOpen := false
	var modalText *tview.TextView
	var copyButtonRow *buttonRowPrimitive
	closeModal := func() {}
	var helpModal *localizedHelpModal
	showInfoModal := func(title string, message string, width int, height int) {
		title = strings.TrimSpace(title)
		if title == "" {
			title = "Details"
		}
		message = strings.TrimSpace(message)
		if message == "" {
			message = "No details are available."
		}
		modalBody := newButtonFlex()
		modalText = textPanel(title, message).SetScrollable(true)
		modalBody.AddItem(modalText, 0, 1, true)
		closeModal = func() {
			modalOpen = false
			modalText = nil
			if pageRoot != nil {
				app.SetRoot(pageRoot, true)
			}
			app.SetFocus(table)
		}
		addButtonRow(modalBody, modalButtons(nil, true, ButtonOK, "Enter", func(nav NavAction) {}, closeModal))
		modalOpen = true
		if pageRoot == nil {
			pageRoot = pageFrame(pageBreadcrumb(page.Breadcrumb, page.Path), body)
		}
		app.SetRoot(overlayRootOn(pageRoot, modalBody, width, height), true)
		app.SetFocus(modalBody)
	}
	showHelpModal := func(pages []localizedHelpPage, width int, height int) {
		closeModal = func() {
			modalOpen = false
			helpModal = nil
			if pageRoot != nil {
				app.SetRoot(pageRoot, true)
			}
			app.SetFocus(table)
		}
		helpModal = newLocalizedHelpModal(app, pages, closeModal)
		modalOpen = true
		if pageRoot == nil {
			pageRoot = pageFrame(pageBreadcrumb(page.Breadcrumb, page.Path), body)
		}
		app.SetRoot(overlayRootOn(pageRoot, helpModal.Body(), width, height), true)
		app.SetFocus(helpModal.TextView())
	}
	updateModeText := func() {
		if controlHeaders {
			modeText.SetText("[yellow]Header control[white]  Left/Right choose sortable headers  Up/Down changes sort  Tab returns to cells")
			return
		}
		modeText.SetText("[yellow]Table control[white]  Arrow keys move by cell  Space toggles row  Tab controls headers")
	}
	updateModeText()

	var refresh func()
	sortRows := func() {
		if page.GroupSort && len(groups) > 0 {
			order = order[:0]
			for _, group := range groups {
				if len(group.Rows) == 0 {
					continue
				}
				groupRows := append([]int(nil), group.Rows...)
				sort.SliceStable(groupRows, func(i, j int) bool {
					return compareRowOrder(page.Rows, groupRows[i], groupRows[j], sortState) < 0
				})
				order = append(order, groupRows...)
			}
			return
		}
		sort.SliceStable(order, func(i, j int) bool {
			return compareRowOrder(page.Rows, order[i], order[j], sortState) < 0
		})
	}
	sortRows()
	table.columnWidths = rowSelectionColumnWidths(page.Columns, page.Rows, layout, page.GroupSort)

	buildVisibleRows := func() []rowSelectionDisplayRow {
		out := make([]rowSelectionDisplayRow, 0, len(order)+len(groups))
		if page.GroupSort && len(groups) > 0 {
			rowToGroup := make(map[int]string, len(page.Rows))
			explicitGroups := false
			for _, group := range groups {
				if group.Explicit {
					explicitGroups = true
				}
				if len(group.Rows) == 0 {
					out = append(out, rowSelectionDisplayRow{IsGroup: true, Group: group.Label, OriginalRow: -1})
					continue
				}
				if group.Explicit {
					out = append(out, rowSelectionDisplayRow{IsGroup: true, Group: group.Label, OriginalRow: -1})
				}
				for _, row := range group.Rows {
					rowToGroup[row] = group.Label
				}
				for _, originalRow := range order {
					if rowToGroup[originalRow] == group.Label {
						out = append(out, rowSelectionDisplayRow{OriginalRow: originalRow})
					}
				}
			}
			if explicitGroups {
				return out
			}
			currentGroup := ""
			for _, originalRow := range order {
				group := rowToGroup[originalRow]
				if group != currentGroup {
					currentGroup = group
					out = append(out, rowSelectionDisplayRow{IsGroup: true, Group: group, OriginalRow: -1})
				}
				out = append(out, rowSelectionDisplayRow{OriginalRow: originalRow})
			}
			return out
		}
		for _, originalRow := range order {
			out = append(out, rowSelectionDisplayRow{OriginalRow: originalRow})
		}
		return out
	}
	displayRowsCache := buildVisibleRows()
	rebuildDisplayRows := func() {
		displayRowsCache = buildVisibleRows()
	}
	visibleRows := func() []rowSelectionDisplayRow {
		return displayRowsCache
	}

	currentOriginalRow := func() int {
		row, _ := table.GetSelection()
		if row < layout.firstDataRow {
			return -1
		}
		for displayRow, item := range visibleRows() {
			if displayRow+layout.firstDataRow == row && item.OriginalRow >= 0 {
				return item.OriginalRow
			}
		}
		return -1
	}
	canViewCurrent := func() bool {
		if controlHeaders {
			return false
		}
		_, column := table.GetSelection()
		if column < 1 {
			return false
		}
		originalRow := currentOriginalRow()
		return originalRow >= 0 && originalRow < len(page.Rows)
	}
	showViewUnavailable := func(message string) {
		showInfoModal(page.Title, message, 58, 10)
	}
	currentHeaderColumn := func() (TableColumn, bool) {
		if !controlHeaders {
			return TableColumn{}, false
		}
		if headerColumn < 0 {
			return TableColumn{
				ID:       "row",
				Header:   "row",
				Sortable: true,
				Help:     "EN: Sequential row number in the current table view. This is not a biological field from the source database; it is a display-order helper used for quick visual navigation, export traceability, and restoring the row order currently on screen.\n中文：当前表格视图中的顺序行号。这不是原始数据库中的生物学字段，而是一个用于快速浏览、导出追踪以及恢复当前屏幕显示顺序的辅助显示列。\n日本語：現在の表ビューにおける連番です。これは元データベース由来の生物学的項目ではなく、素早い視認、エクスポートの追跡、画面上の並び順の復元に使う表示補助列です。",
			}, true
		}
		if headerColumn >= len(page.Columns) {
			return TableColumn{}, false
		}
		return page.Columns[headerColumn], true
	}
	viewCurrentHeader := func() {
		column, ok := currentHeaderColumn()
		if !ok {
			showViewUnavailable("No column header is selected.")
			return
		}
		showHelpModal(columnHelpPages(column), 92, 24)
	}
	currentRowDetail := func(originalRow int) string {
		if originalRow < 0 || originalRow >= len(page.Rows) {
			return ""
		}
		row := page.Rows[originalRow]
		if strings.TrimSpace(row.Detail) != "" {
			return row.Detail
		}
		lines := []string{fmt.Sprintf("row: %d", originalRow+1)}
		for i, column := range page.Columns {
			value := ""
			if i < len(row.Cells) {
				value = row.Cells[i]
			}
			lines = append(lines, fmt.Sprintf("%s: %s", firstNonEmptyText(column.Header, column.ID), displayModalValue(value)))
		}
		return strings.Join(lines, "\n")
	}
	viewCurrent := func() {
		if controlHeaders {
			viewCurrentHeader()
			return
		}
		if !canViewCurrent() {
			showViewUnavailable("View is available only when a normal data cell is selected.")
			return
		}
		originalRow := currentOriginalRow()
		detail := strings.TrimSpace(currentRowDetail(originalRow))
		if detail == "" {
			showViewUnavailable("No details are available for the current row.")
			return
		}
		showInfoModal("Row details", detail, 110, 30)
	}
	currentCellText := func() (string, bool) {
		if controlHeaders {
			return "", false
		}
		_, column := table.GetSelection()
		if column < 1 {
			return "", false
		}
		originalRow := currentOriginalRow()
		if originalRow < 0 || originalRow >= len(page.Rows) {
			return "", false
		}
		if column == 1 {
			return fmt.Sprintf("%d", originalRow+1), true
		}
		cellIndex := column - rowSelectionFirstDataColumn
		if cellIndex < 0 || cellIndex >= len(page.Rows[originalRow].Cells) {
			return "", false
		}
		return strings.TrimSpace(page.Rows[originalRow].Cells[cellIndex]), true
	}
	copyCurrent := func() {
		text, ok := currentCellText()
		if !ok {
			showInfoModal(page.Title, "Copy is available only when a normal data cell is selected.", 58, 10)
			return
		}
		if text == "" {
			showInfoModal(page.Title, "The selected cell is empty.", 58, 10)
			return
		}
		if err := writeClipboardText(text); err != nil {
			showInfoModal("Copy failed", err.Error(), 72, 12)
			return
		}
	}
	headerDisplayColumn := func() int {
		if headerColumn == -1 {
			return 1
		}
		return displayColumn(headerColumn)
	}
	dataColumnFromSelection := func() int {
		row, column := table.GetSelection()
		if row < layout.firstDataRow || column < rowSelectionFirstDataColumn {
			return 0
		}
		dataColumn := column - rowSelectionFirstDataColumn
		if dataColumn < 0 || dataColumn >= len(page.Columns) {
			return 0
		}
		return dataColumn
	}
	keepSelectionVisible := func(row int, column int) {
		if row < layout.firstDataRow {
			row = layout.firstDataRow
		}
		maxRow := len(visibleRows()) + layout.firstDataRow - 1
		if row > maxRow {
			row = maxRow
		}
		if maxRow < layout.firstDataRow {
			return
		}
		if column < 1 {
			column = 1
		}
		lastColumn := len(page.Columns) + 1
		if column > lastColumn {
			column = lastColumn
		}
		table.Select(row, column)
	}
	selectOriginalRow := func(originalRow int, column int) {
		for displayRow, item := range visibleRows() {
			if item.OriginalRow == originalRow {
				keepSelectionVisible(displayRow+layout.firstDataRow, column)
				return
			}
		}
		keepSelectionVisible(layout.firstDataRow, column)
	}
	setSort := func(column int, direction SortDirection) {
		if !sortableDataColumn(column) {
			return
		}
		original := -1
		selectedColumn := headerDisplayColumn()
		if !controlHeaders {
			original = currentOriginalRow()
			_, selectedColumn = table.GetSelection()
		}
		sortState = TableSort{Column: column, Direction: direction}
		headerColumn = column
		sortRows()
		rebuildDisplayRows()
		refresh()
		if controlHeaders {
			table.Select(0, headerDisplayColumn())
		} else if original >= 0 {
			selectOriginalRow(original, selectedColumn)
		}
	}
	cycleSort := func(column int) {
		if !sortableDataColumn(column) {
			return
		}
		direction := SortAscending
		if sortState.Column == column && sortState.Direction == SortAscending {
			direction = SortDescending
		}
		setSort(column, direction)
	}
	moveHeader := func(delta int) {
		if delta == 0 {
			return
		}
		column := headerColumn
		last := len(page.Columns) - 1
		for {
			column += delta
			if column < -1 {
				column = last
			}
			if column > last {
				column = -1
			}
			if sortableDataColumn(column) {
				headerColumn = column
				table.Select(0, headerDisplayColumn())
				return
			}
		}
	}
	headerSortDirection := func(delta int) {
		if delta < 0 {
			setSort(headerColumn, SortAscending)
			return
		}
		setSort(headerColumn, SortDescending)
	}
	toggleHeaderMode := func() {
		controlHeaders = !controlHeaders
		if controlHeaders {
			headerColumn = dataColumnFromSelection()
			table.Select(0, headerDisplayColumn())
		} else {
			row, column := table.GetSelection()
			if row < layout.firstDataRow {
				row = layout.firstDataRow
			}
			keepSelectionVisible(row, column)
		}
		updateModeText()
		refresh()
	}
	var setSelectionHeader func()
	var updateMarkerRow func(int, int)
	setSelectionHeader = func() {
		marker := "[ ]"
		markerColor := colorSelectionOff
		if allSelected() {
			marker = "[x]"
			markerColor = colorSelectionOn
		}
		table.SetCell(rowSelectionHeaderRow, 0, paddedTableCell(tview.Escape(marker)).
			SetTextColor(markerColor).
			SetAlign(tview.AlignCenter).
			SetSelectable(false).
			SetClickedFunc(func() bool {
				setAll(selected, !allSelected())
				refresh()
				return true
			}))
		table.SetTitle(" " + tableTitleWithCount(page.Title, countSelectedBools(selected), len(page.Rows)) + " ")
	}
	updateMarkerRow = func(displayRow int, originalRow int) {
		if originalRow < 0 || originalRow >= len(selected) {
			return
		}
		rowMarker := "[ ]"
		rowMarkerColor := colorSelectionOff
		if selected[originalRow] {
			rowMarker = "[x]"
			rowMarkerColor = colorSelectionOn
		}
		rowIndex := originalRow
		table.SetCell(displayRow, 0, paddedTableCell(tview.Escape(rowMarker)).
			SetTextColor(rowMarkerColor).
			SetAlign(tview.AlignCenter).
			SetClickedFunc(func() bool {
				selected[rowIndex] = !selected[rowIndex]
				updateMarkerRow(displayRow, rowIndex)
				setSelectionHeader()
				selectOriginalRow(rowIndex, 2)
				return true
			}))
	}
	refresh = func() {
		table.SetTitle(" " + tableTitleWithCount(page.Title, countSelectedBools(selected), len(page.Rows)) + " ")
		row, column := table.GetSelection()
		if !controlHeaders && row < layout.firstDataRow {
			row = layout.firstDataRow
		}
		if !controlHeaders {
			displayRows := visibleRows()
			for row >= layout.firstDataRow && row <= len(displayRows)+layout.firstDataRow-1 && displayRows[row-layout.firstDataRow].IsGroup {
				row++
			}
			if row > len(displayRows)+layout.firstDataRow-1 {
				row = len(displayRows) + layout.firstDataRow - 1
			}
		}
		table.Clear()
		headerStyle := tcell.StyleDefault.Foreground(tview.Styles.PrimaryTextColor).Bold(true)
		sortedHeaderStyle := tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tview.Styles.PrimaryTextColor).Bold(true)
		activeHeaderStyle := tcell.StyleDefault.Background(colorAction).Foreground(colorActionText).Bold(true)
		groupStyle := tcell.StyleDefault.Background(colorPanel).Foreground(colorMuted).Bold(true)
		setSelectionHeader()
		rowHeader := "row"
		if sortState.Column == -1 {
			rowHeader += rowSelectionSortArrow(sortState.Direction)
		}
		rowHeaderCell := paddedTableCell(rowHeader).SetSelectable(controlHeaders).SetClickedFunc(func() bool {
			cycleSort(-1)
			return true
		})
		if controlHeaders && headerColumn == -1 {
			rowHeaderCell.SetStyle(activeHeaderStyle).SetSelectedStyle(activeHeaderStyle).SetTransparency(false)
		} else if sortState.Column == -1 {
			rowHeaderCell.SetStyle(sortedHeaderStyle).SetTransparency(false)
		} else {
			rowHeaderCell.SetStyle(headerStyle)
		}
		table.SetCell(rowSelectionHeaderRow, 1, rowHeaderCell)
		if layout.headerTwoLine {
			table.SetCell(1, 0, paddedTableCell("").SetSelectable(false).SetTextColor(colorSelectionOff).SetAlign(tview.AlignCenter))
			table.SetCell(1, 1, paddedTableCell("").SetSelectable(false).SetStyle(headerStyle))
		}
		for i, col := range page.Columns {
			header, subheader := tableHeaderLines(firstNonEmptyText(col.Header, col.ID))
			if sortState.Column == i {
				header += rowSelectionSortArrow(sortState.Direction)
			}
			cell := paddedTableCell(header).SetSelectable(controlHeaders && col.Sortable)
			if col.Sortable {
				columnIndex := i
				cell.SetClickedFunc(func() bool {
					cycleSort(columnIndex)
					return true
				})
			}
			if controlHeaders && headerColumn == i {
				cell.SetStyle(activeHeaderStyle).SetSelectedStyle(activeHeaderStyle).SetTransparency(false)
			} else if sortState.Column == i {
				cell.SetStyle(sortedHeaderStyle).SetTransparency(false)
			} else if strings.EqualFold(col.Reference, "uniprot") || strings.EqualFold(col.Reference, "interpro") {
				cell.SetStyle(tableHeaderStyle(col))
			} else {
				cell.SetStyle(headerStyle)
			}
			table.SetCell(rowSelectionHeaderRow, i+2, cell)
			if layout.headerTwoLine {
				subCell := paddedTableCell(subheader).SetSelectable(false).SetStyle(headerStyle)
				if controlHeaders && headerColumn == i {
					subCell.SetStyle(activeHeaderStyle).SetSelectedStyle(activeHeaderStyle).SetTransparency(false)
				} else if sortState.Column == i {
					subCell.SetStyle(sortedHeaderStyle).SetTransparency(false)
				} else if strings.EqualFold(col.Reference, "uniprot") || strings.EqualFold(col.Reference, "interpro") {
					subCell.SetStyle(tableHeaderStyle(col))
				}
				table.SetCell(1, i+2, subCell)
			}
		}
		dividerStyle := tcell.StyleDefault.Foreground(colorInactiveText)
		for column := 0; column < len(page.Columns)+2; column++ {
			table.SetCell(layout.dividerRow, column, paddedTableCell("").
				SetAlign(tview.AlignCenter).
				SetSelectable(false).
				SetStyle(dividerStyle))
		}
		displayRows := visibleRows()
		for displayRow, item := range displayRows {
			rowNumber := displayRow + layout.firstDataRow
			if item.IsGroup {
				label := item.Group
				if strings.TrimSpace(label) == "" {
					label = "Search term"
				}
				table.SetCell(rowNumber, 0, paddedTableCell("").SetSelectable(false).SetStyle(groupStyle))
				table.SetCell(rowNumber, 1, paddedTableCell("").SetSelectable(false).SetStyle(groupStyle))
				table.SetCell(rowNumber, 2, paddedTableCell(label).SetAlign(tview.AlignCenter).SetSelectable(false).SetStyle(groupStyle))
				for c := 1; c < len(page.Columns); c++ {
					table.SetCell(rowNumber, c+2, paddedTableCell("").SetSelectable(false).SetStyle(groupStyle))
				}
				continue
			}
			originalRow := item.OriginalRow
			rowData := page.Rows[originalRow]
			updateMarkerRow(rowNumber, originalRow)
			numberCell := paddedTableCell(fmt.Sprintf("%d", originalRow+1)).
				SetTextColor(tview.Styles.PrimaryTextColor).
				SetSelectable(true)
			if originalRow >= 0 && originalRow < len(filterFlags) && filterFlags[originalRow] {
				numberCell.SetTextColor(colorSelectionOff)
			}
			table.SetCell(rowNumber, 1, numberCell)
			for c := range page.Columns {
				value := ""
				if c < len(rowData.Cells) {
					value = rowData.Cells[c]
				}
				cell := paddedTableCell(value).SetTextColor(tableCellColor(page.Columns[c], value)).SetSelectable(true)
				table.SetCell(rowNumber, c+2, cell)
			}
		}
		if controlHeaders {
			table.Select(0, headerDisplayColumn())
			return
		}
		keepSelectionVisible(row, column)
	}
	refresh()
	if page.State.Valid {
		if controlHeaders {
			table.Select(0, headerDisplayColumn())
		} else {
			keepSelectionVisible(page.State.SelectedRow, page.State.SelectedColumn)
		}
		table.SetOffset(page.State.RowOffset, page.State.ColumnOffset)
	}

	generate := func(doneAll bool) {
		result.Selected = append([]bool{}, selected...)
		result.FilterFlags = append([]bool{}, filterFlags...)
		result.GenerateFile = true
		result.DoneAll = doneAll
		result.State = captureState()
		app.Stop()
	}
	requestFilter := func() {
		result.Selected = append([]bool{}, selected...)
		result.FilterFlags = append([]bool{}, filterFlags...)
		result.FilterRequested = true
		result.State = captureState()
		app.Stop()
	}
	toggleCurrent := func() {
		if controlHeaders {
			return
		}
		row, column := table.GetSelection()
		if row < layout.firstDataRow {
			return
		}
		originalRow := currentOriginalRow()
		if originalRow < 0 || originalRow >= len(selected) {
			return
		}
		selected[originalRow] = !selected[originalRow]
		updateMarkerRow(row, originalRow)
		setSelectionHeader()
		table.Select(row, column)
	}
	table.SetSelectedFunc(func(row int, column int) {
		if controlHeaders {
			cycleSort(headerColumn)
			return
		}
		if row > 0 && column == 0 {
			toggleCurrent()
		}
	})

	actions := []buttonSpec{
		{Label: ButtonBack, Shortcut: ShortcutBack, Action: func() { result.Nav = NavBack; result.State = captureState(); app.Stop() }, Visible: page.AllowBack},
		{Label: ButtonHome, Shortcut: ShortcutHome, Action: func() { result.Nav = NavHome; result.State = captureState(); app.Stop() }, Visible: page.AllowHome},
		{Label: ButtonCopy, Shortcut: ShortcutCopy, Action: copyCurrent, Visible: true},
		{Label: conciseActionLabel(page.FilterText, ButtonFilter), Shortcut: ShortcutFilter, Action: requestFilter, Visible: page.AllowFilter},
		{Label: conciseActionLabel(page.DoneAllText, ButtonExportAll), Shortcut: ShortcutExportAll, Action: func() { generate(true) }, Visible: page.AllowDoneAll, Primary: true},
		{Label: conciseActionLabel(page.GenerateText, ButtonExport), Shortcut: ShortcutExport, Action: func() { generate(false) }, Visible: true, Primary: true},
		{Label: conciseActionLabel(page.ConfirmText, ButtonView), Shortcut: ShortcutConfirm, Action: viewCurrent, Visible: true, Primary: true},
	}

	body = newButtonFlex()
	if strings.TrimSpace(page.Description) != "" {
		body.AddItem(textBlock(page.Description), 2, 0, false)
	}
	body.AddItem(table, 0, 1, true)
	copyButtonRow = buttonRow(actions...)
	addButtonRow(body, copyButtonRow)
	body.AddItem(modeText, 1, 0, false)
	addHints(body, append(page.Hints, "Table control: Arrow keys move by cell | Space toggles row | Tab controls headers", "Ctrl+A selects all | Ctrl+N clears all | Ctrl+F opens filter when available | Ctrl+Shift+C copies current cell | Header control: Left/Right choose sortable headers | Up/Down changes sort | Tab returns to cells"))

	pageRoot = pageFrame(pageBreadcrumb(page.Breadcrumb, page.Path), body)
	setPageRoot(app, pageRoot)
	app.SetFocus(table)
	installInputCapture(app, func(event *tcell.EventKey) *tcell.EventKey {
		if modalOpen {
			if helpModal != nil {
				_ = helpModal.HandleKey(app, event, closeModal)
				return nil
			}
			switch event.Key() {
			case tcell.KeyEnter, tcell.KeyEscape:
				closeModal()
			case tcell.KeyUp:
				scrollTextView(modalText, -1)
			case tcell.KeyDown:
				scrollTextView(modalText, 1)
			case tcell.KeyPgUp:
				scrollTextView(modalText, -8)
			case tcell.KeyPgDn:
				scrollTextView(modalText, 8)
			}
			return nil
		}
		if selectionKey(event) && app.GetFocus() != table {
			app.SetFocus(table)
		}
		switch event.Key() {
		case tcell.KeyEscape:
			if page.AllowBack {
				result.Nav = NavBack
				result.State = captureState()
				app.Stop()
				return nil
			}
		case tcell.KeyCtrlO:
			if page.AllowHome {
				result.Nav = NavHome
				result.State = captureState()
				app.Stop()
				return nil
			}
		case tcell.KeyCtrlA:
			setAll(selected, true)
			refresh()
			return nil
		case tcell.KeyCtrlN:
			setAll(selected, false)
			refresh()
			return nil
		case tcell.KeyCtrlF:
			if page.AllowFilter {
				requestFilter()
				return nil
			}
		case tcell.KeyCtrlC:
			if isCopyShortcut(event) {
				copyCurrent()
				return nil
			}
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			controlHeaders = false
			keepSelectionVisible(layout.firstDataRow, rowSelectionFirstDataColumn)
			table.SetOffset(0, 0)
			updateModeText()
			refresh()
			return nil
		case tcell.KeyTab:
			toggleHeaderMode()
			return nil
		case tcell.KeyCtrlD:
			if page.AllowDoneAll {
				generate(true)
				return nil
			}
		case tcell.KeyCtrlG:
			generate(false)
			return nil
		case tcell.KeyEnter:
			if controlHeaders {
				viewCurrentHeader()
				return nil
			}
			viewCurrent()
			return nil
		case tcell.KeyUp:
			if controlHeaders {
				headerSortDirection(-1)
				return nil
			}
			row, column := table.GetSelection()
			displayRows := visibleRows()
			target := row - 1
			for target >= layout.firstDataRow && target <= len(displayRows)+layout.firstDataRow-1 && displayRows[target-layout.firstDataRow].IsGroup {
				target--
			}
			if target < layout.firstDataRow {
				return nil
			}
			table.Select(target, column)
			return nil
		case tcell.KeyDown:
			if controlHeaders {
				headerSortDirection(1)
				return nil
			}
			row, column := table.GetSelection()
			displayRows := visibleRows()
			target := row + 1
			lastRow := len(displayRows) + layout.firstDataRow - 1
			for target <= lastRow && displayRows[target-layout.firstDataRow].IsGroup {
				target++
			}
			if target > lastRow {
				return nil
			}
			table.Select(target, column)
			return nil
		case tcell.KeyLeft:
			if controlHeaders {
				moveHeader(-1)
				return nil
			}
		case tcell.KeyRight:
			if controlHeaders {
				moveHeader(1)
				return nil
			}
		case tcell.KeyRune:
			if isCopyShortcut(event) {
				copyCurrent()
				return nil
			}
			if event.Rune() == ' ' {
				toggleCurrent()
				return nil
			}
		}
		return event
	})

	if err := runApp(app); err != nil {
		return RowSelectionResult{}, err
	}
	if result.Selected == nil {
		result.Selected = append([]bool{}, selected...)
	}
	if result.FilterFlags == nil {
		result.FilterFlags = append([]bool{}, filterFlags...)
	}
	if !result.State.Valid {
		result.State = captureState()
	}
	return result, nil
}

func RunBlastRunSelectionPage(page BlastRunSelectionPage) (BlastRunSelectionResult, error) {
	app := newApp()
	var result BlastRunSelectionResult
	if len(page.Items) == 0 {
		return result, nil
	}
	initializing := true
	currentRun := 0
	if page.State.Valid {
		currentRun = page.State.CurrentRun
	}
	if currentRun < 0 || currentRun >= len(page.Items) {
		currentRun = 0
	}
	controlMode := 0 // 0 table, 1 headers, 2 list
	if page.State.Valid {
		controlMode = page.State.ControlMode
	}
	if controlMode < 0 || controlMode > 2 {
		controlMode = 0
	}
	selectedByRun := make([][]bool, len(page.Items))
	filterFlagsByRun := make([][]bool, len(page.Items))
	for i, item := range page.Items {
		selectedByRun[i] = normalizeSelection(item.Selected, len(item.Rows), true)
		filterFlagsByRun[i] = normalizeSelection(item.FilterFlags, len(item.Rows), false)
	}
	tableStates := make([]BlastRunTableState, len(page.Items))
	if page.State.Valid {
		copy(tableStates, page.State.Tables)
	}
	columnWidthsByRun := make([][]int, len(page.Items))
	for i, item := range page.Items {
		itemLayout := newRowSelectionLayout(item.Columns)
		columnWidthsByRun[i] = rowSelectionColumnWidths(item.Columns, item.Rows, itemLayout, false)
	}

	list := newBlastRunSidebar()
	setFocusBorder(list.Box, false)
	attachFocusBorder(list.Box)

	tableBase := tview.NewTable().
		SetBorders(false).
		SetSeparator(tview.Borders.Vertical).
		SetBordersColor(colorInactiveText).
		SetSelectable(true, true).
		SetFixed(2, 2).
		SetEvaluateAllRows(true).
		Select(2, 2)
	layout := newRowSelectionLayout(page.Items[currentRun].Columns)
	tableBase.SetFixed(layout.firstDataRow, rowSelectionFirstDataColumn).Select(layout.firstDataRow, rowSelectionFirstDataColumn)
	table := &rowSelectionTable{Table: tableBase, dividerRow: layout.dividerRow}
	table.SetBorder(true)
	table.SetTitle(" " + tableTitleWithCount(page.Title, countSelectedBools(selectedByRun[currentRun]), len(page.Items[currentRun].Rows)) + " ")
	table.SetTitleAlign(tview.AlignCenter)
	table.SetSelectedStyle(tcell.StyleDefault.Background(colorAction).Foreground(colorActionText).Bold(true))
	setFocusBorder(table.Box, true)
	attachFocusBorder(table.Box)

	emptyView := textPanel("No BLAST results", "No BLAST hits returned for the selected query.")
	emptyView.SetBorder(true)
	emptyView.SetTitle(" BLAST results ")
	emptyView.SetTitleAlign(tview.AlignCenter)

	var right tview.Primitive = table
	var left tview.Primitive = list
	var content *tview.Flex
	var pageRoot tview.Primitive
	modalOpen := false
	var modalText *tview.TextView
	var helpModal *localizedHelpModal
	closeModal := func() {}

	sortState := TableSort{Column: -1, Direction: SortAscending}
	if page.State.Valid {
		sortState = page.State.Sort
	}
	if sortState.Column < -1 || sortState.Column >= len(page.Items[currentRun].Columns) {
		sortState = TableSort{Column: -1, Direction: SortAscending}
	}
	headerColumn := -1
	if page.State.Valid {
		headerColumn = page.State.HeaderColumn
	}
	if headerColumn < -1 || headerColumn >= len(page.Items[currentRun].Columns) {
		headerColumn = -1
	}
	order := []int{}
	captureCurrentTableState := func() BlastRunTableState {
		row, column := table.GetSelection()
		rowOffset, columnOffset := table.GetOffset()
		return BlastRunTableState{
			Valid:          true,
			SelectedRow:    row,
			SelectedColumn: column,
			RowOffset:      rowOffset,
			ColumnOffset:   columnOffset,
		}
	}
	saveCurrentTableState := func() {
		if currentRun >= 0 && currentRun < len(tableStates) {
			tableStates[currentRun] = captureCurrentTableState()
		}
	}
	captureState := func() BlastRunSelectionState {
		saveCurrentTableState()
		return BlastRunSelectionState{
			Valid:        true,
			CurrentRun:   currentRun,
			ControlMode:  controlMode,
			ListOffset:   list.GetOffset(),
			Sort:         sortState,
			HeaderColumn: headerColumn,
			Tables:       append([]BlastRunTableState(nil), tableStates...),
		}
	}
	var refresh func()

	listText := func(item BlastRunItem, index int) string {
		return firstNonEmptyText(item.Label, item.AltLabel, fmt.Sprintf("query %d", index+1))
	}
	listSecondaryLines := func(item BlastRunItem) []string {
		values := splitSidebarLines(item.AltLabel)
		if len(values) == 0 {
			return nil
		}
		primary := strings.TrimSpace(item.Label)
		if len(values) == 1 && strings.EqualFold(strings.Trim(values[0], "[]"), primary) {
			return nil
		}
		return values
	}
	listWidth := func() int {
		width := len([]rune(" BLAST queries "))
		for i, item := range page.Items {
			values := append([]string{listText(item, i), item.Description}, listSecondaryLines(item)...)
			for _, value := range values {
				if n := len([]rune(value)) + 4; n > width {
					width = n
				}
			}
		}
		return maxInt(width, 18)
	}
	refreshList := func() {
		items := make([]blastRunSidebarItem, 0, len(page.Items))
		for i, item := range page.Items {
			lineCount := tableLineCountLabel(countSelectedBools(selectedByRun[i]), len(item.Rows))
			items = append(items, blastRunSidebarItem{
				Primary:   listText(item, i),
				Secondary: listSecondaryLines(item),
				Lines:     lineCount,
			})
		}
		list.SetItems(items)
		list.SetCurrentItem(currentRun)
		if initializing && page.State.Valid && page.State.ListOffset > 0 {
			list.SetOffset(page.State.ListOffset)
		}
	}
	currentItem := func() BlastRunItem {
		return page.Items[currentRun]
	}
	currentSelected := func() []bool {
		return selectedByRun[currentRun]
	}
	currentFilterFlags := func() []bool {
		return filterFlagsByRun[currentRun]
	}
	displayColumn := func(dataColumn int) int { return dataColumn + 2 }
	dataColumnFromSelection := func() int {
		row, column := table.GetSelection()
		if row < layout.firstDataRow || column < rowSelectionFirstDataColumn {
			return 0
		}
		item := currentItem()
		dataColumn := column - rowSelectionFirstDataColumn
		if dataColumn < 0 || dataColumn >= len(item.Columns) {
			return 0
		}
		return dataColumn
	}
	sortableDataColumn := func(dataColumn int) bool {
		item := currentItem()
		if dataColumn == -1 {
			return true
		}
		return dataColumn >= 0 && dataColumn < len(item.Columns) && item.Columns[dataColumn].Sortable
	}
	headerDisplayColumn := func() int {
		if headerColumn == -1 {
			return 1
		}
		return displayColumn(headerColumn)
	}
	sortRows := func() {
		item := currentItem()
		order = make([]int, len(item.Rows))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(i, j int) bool {
			return compareRowOrder(item.Rows, order[i], order[j], sortState) < 0
		})
	}
	currentOriginalRow := func() int {
		row, _ := table.GetSelection()
		if row < layout.firstDataRow {
			return -1
		}
		idx := row - layout.firstDataRow
		if idx < 0 || idx >= len(order) {
			return -1
		}
		return order[idx]
	}
	showInfoModal := func(title string, message string, width int, height int) {
		modalBody := newButtonFlex()
		modalText = textPanel(title, message).SetScrollable(true)
		modalBody.AddItem(modalText, 0, 1, true)
		closeModal = func() {
			modalOpen = false
			modalText = nil
			if pageRoot != nil {
				app.SetRoot(pageRoot, true)
			}
			if controlMode == 2 {
				app.SetFocus(list)
			} else {
				app.SetFocus(table)
			}
		}
		addButtonRow(modalBody, modalButtons(nil, true, ButtonOK, "Enter", func(nav NavAction) {}, closeModal))
		modalOpen = true
		app.SetRoot(overlayRootOn(pageRoot, modalBody, width, height), true)
		app.SetFocus(modalBody)
	}
	showHelpModal := func(pages []localizedHelpPage, width int, height int) {
		helpModal = newLocalizedHelpModal(app, pages, closeModal)
		closeModal = func() {
			modalOpen = false
			helpModal = nil
			if pageRoot != nil {
				app.SetRoot(pageRoot, true)
			}
			if controlMode == 2 {
				app.SetFocus(list)
			} else {
				app.SetFocus(table)
			}
		}
		modalOpen = true
		app.SetRoot(overlayRootOn(pageRoot, helpModal.Body(), width, height), true)
		app.SetFocus(helpModal.TextView())
	}
	currentRowDetail := func(originalRow int) string {
		item := currentItem()
		if originalRow < 0 || originalRow >= len(item.Rows) {
			return ""
		}
		row := item.Rows[originalRow]
		if strings.TrimSpace(row.Detail) != "" {
			return row.Detail
		}
		lines := []string{fmt.Sprintf("row: %d", originalRow+1)}
		for i, column := range item.Columns {
			value := ""
			if i < len(row.Cells) {
				value = row.Cells[i]
			}
			lines = append(lines, fmt.Sprintf("%s: %s", firstNonEmptyText(column.Header, column.ID), displayModalValue(value)))
		}
		return strings.Join(lines, "\n")
	}
	currentHeaderColumn := func() (TableColumn, bool) {
		if controlMode != 1 {
			return TableColumn{}, false
		}
		if headerColumn < 0 {
			return TableColumn{
				ID:       "row",
				Header:   "row",
				Sortable: true,
				Help:     "EN: Sequential row number in the current table view. This is not a biological field from the source database; it is a display-order helper used for quick visual navigation, export traceability, and restoring the row order currently on screen.\n中文：当前表格视图中的顺序行号。这不是原始数据库中的生物学字段，而是一个用于快速浏览、导出追踪以及恢复当前屏幕显示顺序的辅助显示列。\n日本語：現在の表ビューにおける連番です。これは元データベース由来の生物学的項目ではなく、素早い視認、エクスポートの追跡、画面上の並び順の復元に使う表示補助列です。",
			}, true
		}
		item := currentItem()
		if headerColumn >= len(item.Columns) {
			return TableColumn{}, false
		}
		return item.Columns[headerColumn], true
	}
	viewCurrentHeader := func() {
		column, ok := currentHeaderColumn()
		if !ok {
			showInfoModal(page.Title, "No column header is selected.", 58, 10)
			return
		}
		showHelpModal(columnHelpPages(column), 92, 24)
	}
	allSelected := func() bool {
		selected := currentSelected()
		if len(selected) == 0 {
			return false
		}
		for _, ok := range selected {
			if !ok {
				return false
			}
		}
		return true
	}
	keepSelectionVisible := func(row int, column int) {
		item := currentItem()
		if len(item.Rows) == 0 {
			return
		}
		if row < layout.firstDataRow {
			row = layout.firstDataRow
		}
		maxRow := len(item.Rows) + layout.firstDataRow - 1
		if row > maxRow {
			row = maxRow
		}
		if column < 1 {
			column = 1
		}
		lastColumn := len(item.Columns) + 1
		if column > lastColumn {
			column = lastColumn
		}
		table.Select(row, column)
	}
	modeHint := func() string {
		switch controlMode {
		case 1:
			return "Header control: Left/Right choose sortable headers | Up/Down changes sort | Tab controls query list"
		case 2:
			return "Query list: Up/Down choose query | Tab returns to table"
		default:
			return "Table control: Arrow keys move by cell | Space toggles row | Tab controls headers"
		}
	}
	var setSelectionHeader func()
	var updateMarkerRow func(int, int)
	setSelectionHeader = func() {
		item := currentItem()
		marker := "[ ]"
		markerColor := colorSelectionOff
		if allSelected() {
			marker = "[x]"
			markerColor = colorSelectionOn
		}
		table.SetCell(rowSelectionHeaderRow, 0, paddedTableCell(tview.Escape(marker)).SetTextColor(markerColor).SetAlign(tview.AlignCenter).SetSelectable(false).SetClickedFunc(func() bool {
			setAll(currentSelected(), !allSelected())
			refresh()
			return true
		}))
		table.SetTitle(" " + tableTitleWithCount(page.Title, countSelectedBools(currentSelected()), len(item.Rows)) + " ")
	}
	updateMarkerRow = func(displayRow int, originalRow int) {
		if originalRow < 0 || originalRow >= len(currentSelected()) {
			return
		}
		rowMarker := "[ ]"
		rowMarkerColor := colorSelectionOff
		if currentSelected()[originalRow] {
			rowMarker = "[x]"
			rowMarkerColor = colorSelectionOn
		}
		rowIndex := originalRow
		table.SetCell(displayRow, 0, paddedTableCell(tview.Escape(rowMarker)).SetTextColor(rowMarkerColor).SetAlign(tview.AlignCenter).SetClickedFunc(func() bool {
			currentSelected()[rowIndex] = !currentSelected()[rowIndex]
			updateMarkerRow(displayRow, rowIndex)
			setSelectionHeader()
			refreshList()
			return true
		}))
	}
	refresh = func() {
		item := currentItem()
		layout = newRowSelectionLayout(item.Columns)
		table.dividerRow = layout.dividerRow
		if currentRun >= 0 && currentRun < len(columnWidthsByRun) {
			table.columnWidths = columnWidthsByRun[currentRun]
		}
		table.SetFixed(layout.firstDataRow, rowSelectionFirstDataColumn)
		refreshList()
		if len(item.Rows) == 0 {
			right = emptyView
		} else {
			right = table
			table.SetTitle(" " + tableTitleWithCount(page.Title, countSelectedBools(currentSelected()), len(item.Rows)) + " ")
			sortRows()
			table.Clear()
			headerStyle := tcell.StyleDefault.Foreground(tview.Styles.PrimaryTextColor).Bold(true)
			sortedHeaderStyle := tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tview.Styles.PrimaryTextColor).Bold(true)
			activeHeaderStyle := tcell.StyleDefault.Background(colorAction).Foreground(colorActionText).Bold(true)
			setSelectionHeader()
			rowHeader := "row"
			if sortState.Column == -1 {
				rowHeader += rowSelectionSortArrow(sortState.Direction)
			}
			rowHeaderCell := paddedTableCell(rowHeader).SetSelectable(controlMode == 1).SetClickedFunc(func() bool {
				if sortState.Column == -1 && sortState.Direction == SortAscending {
					sortState.Direction = SortDescending
				} else {
					sortState = TableSort{Column: -1, Direction: SortAscending}
				}
				headerColumn = -1
				refresh()
				return true
			})
			if controlMode == 1 && headerColumn == -1 {
				rowHeaderCell.SetStyle(activeHeaderStyle).SetSelectedStyle(activeHeaderStyle).SetTransparency(false)
			} else if sortState.Column == -1 {
				rowHeaderCell.SetStyle(sortedHeaderStyle).SetTransparency(false)
			} else {
				rowHeaderCell.SetStyle(headerStyle)
			}
			table.SetCell(rowSelectionHeaderRow, 1, rowHeaderCell)
			if layout.headerTwoLine {
				table.SetCell(1, 0, paddedTableCell("").SetTextColor(colorSelectionOff).SetAlign(tview.AlignCenter).SetSelectable(false))
				table.SetCell(1, 1, paddedTableCell("").SetStyle(headerStyle).SetSelectable(false))
			}
			for i, col := range item.Columns {
				header, subheader := tableHeaderLines(firstNonEmptyText(col.Header, col.ID))
				if sortState.Column == i {
					header += rowSelectionSortArrow(sortState.Direction)
				}
				columnIndex := i
				cell := paddedTableCell(header).SetSelectable(controlMode == 1 && col.Sortable)
				if col.Sortable {
					cell.SetClickedFunc(func() bool {
						direction := SortAscending
						if sortState.Column == columnIndex && sortState.Direction == SortAscending {
							direction = SortDescending
						}
						sortState = TableSort{Column: columnIndex, Direction: direction}
						headerColumn = columnIndex
						refresh()
						return true
					})
				}
				if controlMode == 1 && headerColumn == i {
					cell.SetStyle(activeHeaderStyle).SetSelectedStyle(activeHeaderStyle).SetTransparency(false)
				} else if sortState.Column == i {
					cell.SetStyle(sortedHeaderStyle).SetTransparency(false)
				} else if strings.EqualFold(col.Reference, "uniprot") || strings.EqualFold(col.Reference, "interpro") {
					cell.SetStyle(tableHeaderStyle(col))
				} else {
					cell.SetStyle(headerStyle)
				}
				table.SetCell(rowSelectionHeaderRow, i+2, cell)
				if layout.headerTwoLine {
					subCell := paddedTableCell(subheader).SetSelectable(false).SetStyle(headerStyle)
					if controlMode == 1 && headerColumn == i {
						subCell.SetStyle(activeHeaderStyle).SetSelectedStyle(activeHeaderStyle).SetTransparency(false)
					} else if sortState.Column == i {
						subCell.SetStyle(sortedHeaderStyle).SetTransparency(false)
					} else if strings.EqualFold(col.Reference, "uniprot") || strings.EqualFold(col.Reference, "interpro") {
						subCell.SetStyle(tableHeaderStyle(col))
					}
					table.SetCell(1, i+2, subCell)
				}
			}
			dividerStyle := tcell.StyleDefault.Foreground(colorInactiveText)
			for column := 0; column < len(item.Columns)+2; column++ {
				table.SetCell(layout.dividerRow, column, paddedTableCell("").SetAlign(tview.AlignCenter).SetSelectable(false).SetStyle(dividerStyle))
			}
			for displayRow, originalRow := range order {
				rowNumber := displayRow + layout.firstDataRow
				rowData := item.Rows[originalRow]
				updateMarkerRow(rowNumber, originalRow)
				numberCell := paddedTableCell(fmt.Sprintf("%d", originalRow+1)).SetTextColor(tview.Styles.PrimaryTextColor).SetSelectable(true)
				if originalRow >= 0 && originalRow < len(currentFilterFlags()) && currentFilterFlags()[originalRow] {
					numberCell.SetTextColor(colorSelectionOff)
				}
				table.SetCell(rowNumber, 1, numberCell)
				for c := range item.Columns {
					value := ""
					if c < len(rowData.Cells) {
						value = rowData.Cells[c]
					}
					table.SetCell(rowNumber, c+2, paddedTableCell(value).SetTextColor(tableCellColor(item.Columns[c], value)).SetSelectable(true))
				}
			}
			if controlMode == 1 {
				table.Select(0, headerDisplayColumn())
			} else {
				state := BlastRunTableState{}
				if initializing {
					state = tableStates[currentRun]
				}
				row, column := table.GetSelection()
				if state.Valid {
					row = state.SelectedRow
					column = state.SelectedColumn
				}
				keepSelectionVisible(row, column)
				if state.Valid {
					table.SetOffset(state.RowOffset, state.ColumnOffset)
				}
			}
		}
		leftColumn := tview.NewFlex().SetDirection(tview.FlexRow)
		leftColumn.AddItem(list, 0, 1, controlMode == 2)
		left = leftColumn
		content.Clear()
		content.AddItem(left, listWidth(), 0, controlMode == 2)
		content.AddItem(right, 0, 1, controlMode != 2)
	}
	setCurrentRun := func(index int) {
		if index < 0 || index >= len(page.Items) {
			return
		}
		if index != currentRun {
			saveCurrentTableState()
			currentRun = index
			if sortState.Column < -1 || sortState.Column >= len(page.Items[currentRun].Columns) {
				sortState = TableSort{Column: -1, Direction: SortAscending}
			}
			if headerColumn < -1 || headerColumn >= len(page.Items[currentRun].Columns) {
				headerColumn = -1
			}
			refresh()
		} else {
			list.SetCurrentItem(currentRun)
		}
		if controlMode == 2 {
			app.SetFocus(list)
		}
	}
	list.SetChangedFunc(setCurrentRun)
	generate := func(doneAll bool) {
		result.RunIndex = currentRun
		result.Selected = append([]bool(nil), currentSelected()...)
		result.SelectedByRun = cloneBoolMatrix(selectedByRun)
		result.FilterFlagsByRun = cloneBoolMatrix(filterFlagsByRun)
		result.GenerateFile = !doneAll
		result.DoneAll = doneAll
		result.State = captureState()
		app.Stop()
	}
	requestFilter := func() {
		result.RunIndex = currentRun
		result.Selected = append([]bool(nil), currentSelected()...)
		result.SelectedByRun = cloneBoolMatrix(selectedByRun)
		result.FilterFlagsByRun = cloneBoolMatrix(filterFlagsByRun)
		result.FilterRequested = true
		result.State = captureState()
		app.Stop()
	}
	viewCurrent := func() {
		if controlMode == 1 {
			viewCurrentHeader()
			return
		}
		if len(currentItem().Rows) == 0 {
			showInfoModal("No BLAST results", "No BLAST hits returned for the selected query.", 64, 10)
			return
		}
		originalRow := currentOriginalRow()
		detail := strings.TrimSpace(currentRowDetail(originalRow))
		if detail == "" {
			showInfoModal("Row details", "No details are available for the current row.", 64, 10)
			return
		}
		showInfoModal("Row details", detail, 110, 30)
	}
	toggleCurrent := func() {
		if controlMode != 0 || len(currentItem().Rows) == 0 {
			return
		}
		originalRow := currentOriginalRow()
		if originalRow >= 0 && originalRow < len(currentSelected()) {
			row, _ := table.GetSelection()
			currentSelected()[originalRow] = !currentSelected()[originalRow]
			updateMarkerRow(row, originalRow)
			setSelectionHeader()
			refreshList()
		}
	}
	copyCurrent := func() {
		if controlMode != 0 || len(currentItem().Rows) == 0 {
			showInfoModal(page.Title, "Copy is available only when a normal data cell is selected.", 58, 10)
			return
		}
		_, column := table.GetSelection()
		originalRow := currentOriginalRow()
		if originalRow < 0 || originalRow >= len(currentItem().Rows) || column < 1 {
			showInfoModal(page.Title, "Copy is available only when a normal data cell is selected.", 58, 10)
			return
		}
		text := ""
		if column == 1 {
			text = fmt.Sprintf("%d", originalRow+1)
		} else {
			cellIndex := column - rowSelectionFirstDataColumn
			if cellIndex >= 0 && cellIndex < len(currentItem().Rows[originalRow].Cells) {
				text = strings.TrimSpace(currentItem().Rows[originalRow].Cells[cellIndex])
			}
		}
		if text == "" {
			showInfoModal(page.Title, "The selected cell is empty.", 58, 10)
			return
		}
		if err := writeClipboardText(text); err != nil {
			showInfoModal("Copy failed", err.Error(), 72, 12)
		}
	}
	actions := []buttonSpec{
		{Label: ButtonBack, Shortcut: ShortcutBack, Action: func() { result.Nav = NavBack; result.State = captureState(); app.Stop() }, Visible: page.AllowBack},
		{Label: ButtonHome, Shortcut: ShortcutHome, Action: func() { result.Nav = NavHome; result.State = captureState(); app.Stop() }, Visible: page.AllowHome},
		{Label: ButtonCopy, Shortcut: ShortcutCopy, Action: copyCurrent, Visible: true},
		{Label: conciseActionLabel(page.FilterText, ButtonFilter), Shortcut: ShortcutFilter, Action: requestFilter, Visible: page.AllowFilter},
		{Label: conciseActionLabel(page.DoneAllText, ButtonExportAll), Shortcut: ShortcutExportAll, Action: func() { generate(true) }, Visible: true, Primary: true},
		{Label: conciseActionLabel(page.GenerateText, ButtonExport), Shortcut: ShortcutExport, Action: func() { generate(false) }, Visible: true, Primary: true},
		{Label: conciseActionLabel(page.ConfirmText, ButtonView), Shortcut: ShortcutConfirm, Action: viewCurrent, Visible: true, Primary: true},
	}
	body := newButtonFlex()
	if strings.TrimSpace(page.Description) != "" {
		body.AddItem(textBlock(page.Description), 2, 0, false)
	}
	content = tview.NewFlex().SetDirection(tview.FlexColumn)
	body.AddItem(content, 0, 1, true)
	addButtonRow(body, buttonRow(actions...))
	addHints(body, append(page.Hints, "Tab cycles table, headers, and query list. Ctrl+F opens filter when available. Ctrl+G exports current query; Ctrl+D exports all queries with results.", modeHint()))
	pageRoot = pageFrame(pageBreadcrumb(page.Breadcrumb, page.Path), body)
	refresh()
	initializing = false
	setPageRoot(app, pageRoot)
	app.SetFocus(table)
	installInputCapture(app, func(event *tcell.EventKey) *tcell.EventKey {
		if modalOpen {
			if helpModal != nil {
				_ = helpModal.HandleKey(app, event, closeModal)
				return nil
			}
			switch event.Key() {
			case tcell.KeyEnter, tcell.KeyEscape:
				closeModal()
			case tcell.KeyUp:
				scrollTextView(modalText, -1)
			case tcell.KeyDown:
				scrollTextView(modalText, 1)
			case tcell.KeyPgUp:
				scrollTextView(modalText, -8)
			case tcell.KeyPgDn:
				scrollTextView(modalText, 8)
			}
			return nil
		}
		switch event.Key() {
		case tcell.KeyEscape:
			if page.AllowBack {
				result.Nav = NavBack
				result.State = captureState()
				app.Stop()
				return nil
			}
		case tcell.KeyCtrlO:
			if page.AllowHome {
				result.Nav = NavHome
				result.State = captureState()
				app.Stop()
				return nil
			}
		case tcell.KeyTab:
			if controlMode != 1 && (controlMode+1)%3 == 1 {
				headerColumn = dataColumnFromSelection()
			}
			controlMode = (controlMode + 1) % 3
			if controlMode == 2 {
				app.SetFocus(list)
			} else {
				app.SetFocus(table)
			}
			refresh()
			return nil
		case tcell.KeyCtrlA:
			if len(currentSelected()) > 0 {
				setAll(currentSelected(), true)
				refresh()
			}
			return nil
		case tcell.KeyCtrlN:
			if len(currentSelected()) > 0 {
				setAll(currentSelected(), false)
				refresh()
			}
			return nil
		case tcell.KeyCtrlF:
			if page.AllowFilter {
				requestFilter()
				return nil
			}
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			controlMode = 0
			layout = newRowSelectionLayout(currentItem().Columns)
			table.SetFixed(layout.firstDataRow, rowSelectionFirstDataColumn)
			table.Select(layout.firstDataRow, rowSelectionFirstDataColumn)
			table.SetOffset(0, 0)
			app.SetFocus(table)
			refresh()
			return nil
		case tcell.KeyCtrlD:
			generate(true)
			return nil
		case tcell.KeyCtrlG:
			generate(false)
			return nil
		case tcell.KeyEnter:
			viewCurrent()
			return nil
		case tcell.KeyUp:
			if controlMode == 1 {
				if sortableDataColumn(headerColumn) {
					sortState = TableSort{Column: headerColumn, Direction: SortAscending}
					refresh()
				}
				return nil
			}
		case tcell.KeyDown:
			if controlMode == 1 {
				if sortableDataColumn(headerColumn) {
					sortState = TableSort{Column: headerColumn, Direction: SortDescending}
					refresh()
				}
				return nil
			}
		case tcell.KeyLeft:
			if controlMode == 1 {
				for {
					headerColumn--
					if headerColumn < -1 {
						headerColumn = len(currentItem().Columns) - 1
					}
					if sortableDataColumn(headerColumn) {
						break
					}
				}
				refresh()
				return nil
			}
		case tcell.KeyRight:
			if controlMode == 1 {
				for {
					headerColumn++
					if headerColumn >= len(currentItem().Columns) {
						headerColumn = -1
					}
					if sortableDataColumn(headerColumn) {
						break
					}
				}
				refresh()
				return nil
			}
		case tcell.KeyRune:
			if isCopyShortcut(event) {
				copyCurrent()
				return nil
			}
			if event.Rune() == ' ' {
				toggleCurrent()
				return nil
			}
		}
		return event
	})
	if err := runApp(app); err != nil {
		return BlastRunSelectionResult{}, err
	}
	if result.Selected == nil && currentRun >= 0 && currentRun < len(selectedByRun) {
		result.RunIndex = currentRun
		result.Selected = append([]bool(nil), selectedByRun[currentRun]...)
		result.SelectedByRun = cloneBoolMatrix(selectedByRun)
		result.FilterFlagsByRun = cloneBoolMatrix(filterFlagsByRun)
	}
	if !result.State.Valid {
		result.State = captureState()
	}
	return result, nil
}

func RunTaskPage(page TaskPage, task func(update func(string)) error) error {
	_, err := RunTaskValue[struct{}](page, func(update func(string)) (struct{}, error) {
		return struct{}{}, task(update)
	})
	return err
}

func RunTaskPageContext(page TaskPage, task func(ctx context.Context, update func(string)) error) error {
	_, err := RunTaskValueContext[struct{}](page, func(ctx context.Context, update func(string)) (struct{}, error) {
		return struct{}{}, task(ctx, update)
	})
	return err
}

func taskCancelError(page TaskPage) error {
	if page.CancelError != nil {
		return page.CancelError
	}
	return ErrTaskCancelled
}

func RunTaskValueContext[T any](page TaskPage, task func(ctx context.Context, update func(string)) (T, error)) (T, error) {
	return runTaskValue(page, task)
}

func RunProgressTaskValueContext[T any](page TaskPage, task func(ctx context.Context, update func(current int, message string)) (T, error)) (T, error) {
	return runProgressTaskValue(page, task)
}

func RunInfoPage(page InfoPage) (InfoResult, error) {
	app := newApp()
	var result InfoResult
	confirm := func() {
		app.Stop()
	}
	modalBody := newButtonFlex()
	modalBody.SetBorder(true)
	modalBody.SetTitle(" " + trimColon(page.Title) + " ")
	modalBody.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(modalBody.Box, true)
	attachFocusBorder(modalBody.Box)
	modalBody.AddItem(textPanel(page.Title, page.Message), 0, 1, true)
	addButtonRow(modalBody, modalButtons(nil, true, page.ConfirmText, "Enter", func(nav NavAction) {
		result.Nav = nav
		app.Stop()
	}, confirm))
	addHints(modalBody, page.Hints)

	app.SetRoot(infoModalRoot(modalFramePage(page.Breadcrumb, page.Path, page.Title), modalBody, 90, 18), true)
	app.SetFocus(modalBody)
	installInputCapture(app, navCapture(app, page.AllowBack, page.AllowHome, func(nav NavAction) {
		result.Nav = nav
		app.Stop()
	}, keyBinding{Key: tcell.KeyEnter, Action: confirm}))
	if err := runApp(app); err != nil {
		return InfoResult{}, err
	}
	return result, nil
}

func RunActionModalPage(page ActionModalPage) (ActionModalResult, error) {
	app := newApp()
	var result ActionModalResult
	choose := func(value string) {
		result.Value = value
		app.Stop()
	}
	closeValue := actionCloseValue(page.Actions)
	if closeValue == "" && actionLooksLikeClose(page.ConfirmText, page.ConfirmValue) {
		closeValue = page.ConfirmValue
	}
	actions := make([]buttonSpec, 0, len(page.Actions))
	for _, action := range page.Actions {
		action := action
		shortcut := action.Shortcut
		if shortcut == "" && actionLooksLikeClose(action.Label, action.Value) {
			shortcut = "Esc"
		}
		actions = append(actions, buttonSpec{
			Label:    action.Label,
			Shortcut: shortcut,
			Action:   func() { choose(action.Value) },
			Visible:  true,
			Primary:  false,
		})
	}
	confirm := func() {
		if strings.TrimSpace(page.ConfirmValue) != "" {
			choose(page.ConfirmValue)
			return
		}
		if len(page.Actions) > 0 {
			choose(page.Actions[0].Value)
			return
		}
		app.Stop()
	}

	modalBody := newButtonFlex()
	modalBody.SetBorder(true)
	modalBody.SetTitle(" " + trimColon(page.Title) + " ")
	modalBody.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(modalBody.Box, true)
	attachFocusBorder(modalBody.Box)
	modalBody.AddItem(textPanel(page.Title, page.Message), 0, 1, true)
	addButtonRow(modalBody, modalButtons(actions, true, page.ConfirmText, "Enter", func(nav NavAction) {
		result.Nav = nav
		app.Stop()
	}, confirm))

	app.SetRoot(infoModalRoot(modalFramePage(page.Breadcrumb, page.Path, page.Title), modalBody, 90, 18), true)
	app.SetFocus(modalBody)
	installInputCapture(app, func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEnter:
			confirm()
			return nil
		case tcell.KeyEscape:
			if closeValue != "" {
				choose(closeValue)
				return nil
			}
			app.Stop()
			return nil
		}
		for _, action := range page.Actions {
			if shortcutMatchesEvent(action.Shortcut, event) {
				choose(action.Value)
				return nil
			}
		}
		return nil
	})
	if err := runApp(app); err != nil {
		return ActionModalResult{}, err
	}
	return result, nil
}

func RunRecoveryModalPage(page RecoveryModalPage) (ActionModalResult, error) {
	actions := []Action{
		{Value: "retry", Label: ButtonRetry, Shortcut: ShortcutRetry},
	}
	confirmText := ButtonClose
	confirmValue := "close"
	if page.AllowSkip {
		actions = append([]Action{{Value: "close", Label: ButtonClose, Shortcut: ShortcutBack}}, actions...)
		confirmText = ButtonSkip
		confirmValue = "skip"
	}
	return RunActionModalPage(ActionModalPage{
		Breadcrumb:   page.Breadcrumb,
		Path:         page.Path,
		Title:        page.Title,
		Message:      page.Message,
		Actions:      actions,
		ConfirmText:  confirmText,
		ConfirmValue: confirmValue,
	})
}

func RunChoiceModalPage(page ChoiceModalPage) (ChoiceResult, error) {
	if len(page.Choices) == 0 {
		return ChoiceResult{}, fmt.Errorf("missing choices")
	}
	choices := choiceModalOptions(page)

	app := newApp()
	var result ChoiceResult
	list := tview.NewList()
	for i, choice := range choices {
		list.AddItem(choice.Label, indentSecondary(choice.Description), rune('1'+i), nil)
	}
	list.SetBorder(true)
	list.SetTitle(" " + trimColon(page.Title) + " ")
	list.SetTitleAlign(tview.AlignCenter)
	list.ShowSecondaryText(true)
	list.SetMainTextColor(tview.Styles.PrimaryTextColor)
	list.SetSecondaryTextColor(tview.Styles.SecondaryTextColor)
	list.SetSelectedTextColor(tview.Styles.InverseTextColor)
	list.SetSelectedBackgroundColor(tview.Styles.ContrastBackgroundColor)
	setFocusBorder(list.Box, true)
	attachFocusBorder(list.Box)

	confirm := func() {
		index := currentItem(list)
		if index < 0 || index >= len(choices) {
			index = 0
		}
		result.Value = choices[index].Value
		app.Stop()
	}

	modalBody := newButtonFlex()
	modalBody.SetBorder(true)
	modalBody.SetTitle(" " + trimColon(page.Title) + " ")
	modalBody.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(modalBody.Box, true)
	attachFocusBorder(modalBody.Box)
	if strings.TrimSpace(page.Message) != "" {
		modalBody.AddItem(textBlock(page.Message), 2, 0, false)
	}
	modalBody.AddItem(list, 0, 1, true)
	addButtonRow(modalBody, buttonRow(
		buttonSpec{
			Label:    ButtonClose,
			Shortcut: ShortcutBack,
			Action:   func() { result.Value = "close"; app.Stop() },
			Visible:  page.AllowClose,
		},
		buttonSpec{
			Label:    conciseActionLabel(page.ConfirmText, "OK"),
			Shortcut: "Enter",
			Action:   confirm,
			Visible:  true,
			Primary:  true,
		},
	))

	app.SetRoot(infoModalRoot(modalFramePage(page.Breadcrumb, page.Path, page.Title), modalBody, 90, 18), true)
	app.SetFocus(list)
	installInputCapture(app, func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEnter:
			confirm()
			return nil
		case tcell.KeyEscape:
			if page.AllowClose {
				result.Value = "close"
				app.Stop()
				return nil
			}
			app.Stop()
			return nil
		case tcell.KeyUp:
			list.SetCurrentItem(max(0, list.GetCurrentItem()-1))
			return nil
		case tcell.KeyDown:
			next := list.GetCurrentItem() + 1
			if next >= len(choices) {
				next = len(choices) - 1
			}
			list.SetCurrentItem(next)
			return nil
		case tcell.KeyRune:
			index := int(event.Rune() - '1')
			if index >= 0 && index < len(choices) {
				list.SetCurrentItem(index)
				return nil
			}
		}
		return nil
	})
	if err := runApp(app); err != nil {
		return ChoiceResult{}, err
	}
	return result, nil
}

func choiceModalOptions(page ChoiceModalPage) []Choice {
	choices := append([]Choice(nil), page.Choices...)
	if page.AllowClose {
		choices = append([]Choice{{
			Value:       "close",
			Label:       ButtonClose,
			Description: "Close this dialog",
		}}, choices...)
	}
	return choices
}

func RunExportSettingsModal(page ExportSettingsPage) (ExportSettingsResult, error) {
	app := newApp()
	var result ExportSettingsResult
	fileLabel := firstNonEmptyText(page.FileLabel, "File name")
	fileInput := tview.NewInputField().
		SetLabel(fileLabel + " ").
		SetText(page.FileInitial).
		SetFieldWidth(-1)
	fileInput.SetBorder(true)
	fileInput.SetTitle(" " + fileLabel + " ")
	fileInput.SetTitleAlign(tview.AlignCenter)
	fileInput.SetFieldBackgroundColor(colorPanel)
	fileInput.SetFieldTextColor(tview.Styles.PrimaryTextColor)
	setFocusBorder(fileInput.Box, true)
	attachFocusBorder(fileInput.Box)

	folderLabel := firstNonEmptyText(page.FolderLabel, "Output folder")
	folderInput := tview.NewInputField().
		SetLabel(folderLabel + " ").
		SetFieldWidth(-1)
	folderInput.SetBorder(true)
	folderInput.SetTitle(" " + folderLabel + " ")
	folderInput.SetTitleAlign(tview.AlignCenter)
	folderInput.SetFieldBackgroundColor(colorPanel)
	folderInput.SetFieldTextColor(tview.Styles.PrimaryTextColor)
	setFocusBorder(folderInput.Box, false)
	attachFocusBorder(folderInput.Box)

	writeReport := page.ReportInitial
	reportBox := newCheckboxModule(firstNonEmptyText(page.ReportLabel, "Data analysis report (PDF)"), func() bool {
		return writeReport
	}, func() {
		writeReport = !writeReport
	})
	reportBox.SetBorder(true)
	reportBox.SetTitle(" Data analysis report ")
	reportBox.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(reportBox.Box, false)
	attachFocusBorder(reportBox.Box)

	writeText := page.WriteText
	writeExcel := page.WriteExcel
	writeRawExcel := page.WriteRawExcel
	outputTextBox := newCheckboxModule("Write text file", func() bool { return writeText }, func() { writeText = !writeText })
	outputExcelBox := newCheckboxModule("Write Excel file", func() bool { return writeExcel }, func() { writeExcel = !writeExcel })
	outputRawBox := newCheckboxModule("Write raw Excel and raw text files", func() bool { return writeRawExcel }, func() { writeRawExcel = !writeRawExcel })
	for _, box := range []*checkboxModule{outputTextBox, outputExcelBox, outputRawBox} {
		box.SetBorder(false)
	}

	showFileModule := !page.AllowFolder || !page.AllowEmptyFile
	fileModule := clipPrimitive(fileInput)
	folderModule := clipPrimitive(folderInput)
	type exportModule struct {
		primitive tview.Primitive
		input     *tview.InputField
	}
	fields := make([]exportModule, 0, 3)
	if showFileModule {
		fields = append(fields, exportModule{primitive: fileModule, input: fileInput})
	}
	if page.AllowFolder {
		fields = append(fields, exportModule{primitive: folderModule, input: folderInput})
	}
	fields = append(fields,
		exportModule{primitive: outputTextBox},
		exportModule{primitive: outputExcelBox},
		exportModule{primitive: outputRawBox},
		exportModule{primitive: reportBox},
	)
	focusIndex := 0
	focusCurrent := func() {
		for i, field := range fields {
			if box := primitiveBox(field.primitive); box != nil {
				setFocusBorder(box, i == focusIndex)
			}
		}
		app.SetFocus(fields[focusIndex].primitive)
	}
	submitExportSettings := func() {
		result.FileName = strings.TrimSpace(fileInput.GetText())
		result.FolderName = strings.TrimSpace(folderInput.GetText())
		result.WriteReport = writeReport
		result.WriteText = writeText
		result.WriteExcel = writeExcel
		result.WriteRawExcel = writeRawExcel
		app.Stop()
	}
	focusNext := func() {
		if focusIndex < len(fields)-1 {
			focusIndex++
			focusCurrent()
			return
		}
		submitExportSettings()
	}
	focusPrevious := func() {
		focusIndex--
		if focusIndex < 0 {
			focusIndex = len(fields) - 1
		}
		focusCurrent()
	}
	closeWithNav := func(nav NavAction) {
		result.Nav = nav
		app.Stop()
	}
	pasteStatus := newPasteStatus(focusCurrent)
	paste := func() {
		target := fields[focusIndex].input
		if target == nil {
			pasteStatus.view.SetTextColor(colorMuted)
			pasteStatus.view.SetText("Paste is available only in text fields.")
			focusCurrent()
			return
		}
		runInlinePaste(app, pasteStatus, func(text string) {
			if handler := target.PasteHandler(); handler != nil {
				handler(text, func(p tview.Primitive) { app.SetFocus(p) })
			}
		})
	}

	modalBody := newButtonFlex()
	modalBody.SetBorder(true)
	modalBody.SetTitle(" " + trimColon(page.Title) + " ")
	modalBody.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(modalBody.Box, true)
	attachFocusBorder(modalBody.Box)
	messageHeight := 0
	if strings.TrimSpace(page.Message) != "" {
		messageHeight = minInt(4, maxInt(2, textViewLineCount(page.Message)))
		modalBody.AddItem(textBlock(page.Message), messageHeight, 0, false)
	}
	contentHeight := messageHeight
	if showFileModule {
		modalBody.AddItem(fileModule, 3, 0, true)
		contentHeight += 3
	}
	if page.AllowFolder {
		modalBody.AddItem(folderModule, 3, 0, !showFileModule)
		contentHeight += 3
	}
	outputGroup := newButtonFlex()
	outputGroup.SetBorder(true)
	outputGroup.SetTitle(" Files to generate ")
	outputGroup.SetTitleAlign(tview.AlignCenter)
	outputHelp := "Text exports selected peptide sequences. Excel exports selected rows.\nRaw exports every table row to _raw.xlsx, and also writes _raw.txt when text export is enabled."
	outputGroup.AddItem(textBlock(outputHelp), 3, 0, false)
	outputGroup.AddItem(outputTextBox, 1, 0, false)
	outputGroup.AddItem(outputExcelBox, 1, 0, false)
	outputGroup.AddItem(outputRawBox, 1, 0, false)
	outputGroupHeight := 8
	modalBody.AddItem(outputGroup, outputGroupHeight, 0, false)
	contentHeight += outputGroupHeight
	modalBody.AddItem(reportBox, 3, 0, false)
	contentHeight += 3
	modalBody.AddItem(pasteStatus.view, 1, 0, false)
	contentHeight += 1
	addButtonRow(modalBody, modalButtons([]buttonSpec{
		{Label: ButtonBack, Shortcut: ShortcutBack, Action: func() { closeWithNav(NavBack) }, Visible: page.AllowBack},
		{Label: ButtonPaste, Shortcut: ShortcutPaste, Action: paste, Visible: true},
	}, true, page.ConfirmText, "Enter", closeWithNav, submitExportSettings))
	contentHeight += 1
	addHints(modalBody, []string{"Tab switches fields. Enter moves forward; on the last field it starts export. Space toggles file options."})
	contentHeight += 1

	height := contentHeight + 2
	width := 118
	app.SetRoot(infoModalRoot(modalFramePage(page.Breadcrumb, page.Path, page.Title), modalBody, width, height), true)
	focusCurrent()
	fileInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			focusNext()
		}
	})
	folderInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			focusNext()
		}
	})
	installInputCapture(app, func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			if page.AllowBack {
				closeWithNav(NavBack)
				return nil
			}
		case tcell.KeyCtrlV:
			paste()
			return nil
		case tcell.KeyTab:
			focusIndex = (focusIndex + 1) % len(fields)
			focusCurrent()
			return nil
		case tcell.KeyBacktab:
			focusPrevious()
			return nil
		case tcell.KeyEnter:
			focusNext()
			return nil
		case tcell.KeyRune:
			if event.Rune() == ' ' && fields[focusIndex].input == nil {
				if box, ok := fields[focusIndex].primitive.(*checkboxModule); ok {
					box.toggleChecked()
				}
				return nil
			}
			if fields[focusIndex].input != nil {
				deliverInputFieldKey(fields[focusIndex].input, event, app)
				return nil
			}
		}
		if fields[focusIndex].input != nil && inputFieldEditKey(event) {
			deliverInputFieldKey(fields[focusIndex].input, event, app)
			return nil
		}
		return event
	})
	if err := runApp(app); err != nil {
		return ExportSettingsResult{}, err
	}
	return result, nil
}

func RunExternalReferenceModal(page ExternalReferencePage) (ExternalReferenceResult, error) {
	app := newApp()
	var result ExternalReferenceResult
	useUniProt := page.UniProtInitial
	useInterPro := page.InterProInitial
	settings := normalizeTUIInterProSettings(page.InterProSettings)
	helpVisible := false
	var mainRoot tview.Primitive
	uniProtBox := newCheckboxModule(firstNonEmptyText(page.UniProtLabel, "Add UniProt annotation columns"), func() bool {
		return useUniProt
	}, func() {
		useUniProt = !useUniProt
	})
	uniProtBox.SetBorder(true)
	uniProtBox.SetTitle(" UniProt ")
	uniProtBox.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(uniProtBox.Box, false)
	attachFocusBorder(uniProtBox.Box)
	interProBox := newCheckboxModule(firstNonEmptyText(page.InterProLabel, "Add InterPro domain-evidence columns"), func() bool {
		return useInterPro
	}, func() {
		useInterPro = !useInterPro
	})
	interProBox.SetBorder(false)
	setFocusBorder(interProBox.Box, false)
	attachFocusBorder(interProBox.Box)

	settingRows := []struct {
		label  string
		get    func() bool
		toggle func()
	}{
		{"Match Pfam IDs", func() bool { return settings.UsePfamAccession }, func() { settings.UsePfamAccession = !settings.UsePfamAccession }},
		{"Match InterPro IDs", func() bool { return settings.UseInterProAccession }, func() { settings.UseInterProAccession = !settings.UseInterProAccession }},
		{"Match member-database signature IDs", func() bool { return settings.UseSignatureAccession }, func() { settings.UseSignatureAccession = !settings.UseSignatureAccession }},
		{"Require compatible entry type", func() bool { return settings.UseEntryType }, func() { settings.UseEntryType = !settings.UseEntryType }},
		{"Also compare entry names", func() bool { return settings.UseEntryName }, func() { settings.UseEntryName = !settings.UseEntryName }},
		{"Use coverage cutoffs", func() bool { return settings.UseCoverage }, func() { settings.UseCoverage = !settings.UseCoverage }},
		{"Use coordinate overlap evidence", func() bool { return settings.UseMatchRegions }, func() { settings.UseMatchRegions = !settings.UseMatchRegions }},
	}
	settingBoxes := make([]*checkboxModule, 0, len(settingRows))
	for _, row := range settingRows {
		box := newCheckboxModule(row.label, row.get, row.toggle)
		box.SetBorder(false)
		settingBoxes = append(settingBoxes, box)
	}
	presentCoverage := tview.NewInputField().SetLabel("present coverage >= % ").SetText(settings.PresentMinCoverage).SetFieldWidth(8)
	partialCoverage := tview.NewInputField().SetLabel("partial coverage >= % ").SetText(settings.PartialMinCoverage).SetFieldWidth(8)
	presentItems := tview.NewInputField().SetLabel("present evidence count >= ").SetText(settings.PresentMinMatchedItems).SetFieldWidth(8)
	partialItems := tview.NewInputField().SetLabel("partial evidence count >= ").SetText(settings.PartialMinMatchedItems).SetFieldWidth(8)
	for _, input := range []*tview.InputField{presentCoverage, partialCoverage, presentItems, partialItems} {
		input.SetFieldTextColor(tview.Styles.PrimaryTextColor)
		input.SetLabelColor(tview.Styles.SecondaryTextColor)
	}

	params := newButtonFlex()
	params.SetBorder(true)
	params.SetTitle(" InterPro status rules ")
	params.SetTitleAlign(tview.AlignCenter)
	params.AddItem(textBlock("These rules decide how the InterPro status column is labeled. They add evidence for review; they do not remove or hide BLAST rows."), 2, 0, false)
	params.AddItem(sectionHeader("Identifier matches"), 1, 0, false)
	for _, box := range settingBoxes[:3] {
		params.AddItem(box, 1, 0, false)
	}
	params.AddItem(sectionHeader("Evidence quality checks"), 1, 0, false)
	for _, box := range settingBoxes[3:] {
		params.AddItem(box, 1, 0, false)
	}
	params.AddItem(sectionHeader("Status thresholds"), 1, 0, false)
	thresholds := tview.NewFlex().SetDirection(tview.FlexRow)
	presentThresholds := tview.NewFlex().SetDirection(tview.FlexColumn)
	partialThresholds := tview.NewFlex().SetDirection(tview.FlexColumn)
	presentThresholds.AddItem(presentCoverage, 0, 1, true)
	presentThresholds.AddItem(presentItems, 0, 1, false)
	partialThresholds.AddItem(partialCoverage, 0, 1, false)
	partialThresholds.AddItem(partialItems, 0, 1, false)
	thresholds.AddItem(presentThresholds, 1, 0, true)
	thresholds.AddItem(partialThresholds, 1, 0, false)
	params.AddItem(thresholds, 2, 0, true)
	params.AddItem(sectionHeader("Status meaning"), 1, 0, false)
	params.AddItem(textBlock("present: strong conserved-region support. partial: related support exists but is weaker or shorter. missing: InterPro returned data but no expected conserved evidence was found. uncertain: evidence is too weak or ambiguous to judge."), 5, 0, false)
	setFocusBorder(params.Box, false)
	attachFocusBorder(params.Box)

	interProModule := newButtonFlex()
	interProModule.SetBorder(true)
	interProModule.SetTitle(" InterPro ")
	interProModule.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(interProModule.Box, false)
	attachFocusBorder(interProModule.Box)
	interProModule.AddItem(interProBox, 2, 0, true)
	interProModule.AddItem(params, 0, 1, false)

	closeWithNav := func(nav NavAction) {
		result.Nav = nav
		app.Stop()
	}
	confirm := func() {
		result.UseUniProt = useUniProt
		result.UseInterPro = useInterPro
		result.InterProSettings = InterProConservedRegionSettings{
			UsePfamAccession:       settings.UsePfamAccession,
			UseInterProAccession:   settings.UseInterProAccession,
			UseSignatureAccession:  settings.UseSignatureAccession,
			UseEntryType:           settings.UseEntryType,
			UseEntryName:           settings.UseEntryName,
			UseCoverage:            settings.UseCoverage,
			UseMatchRegions:        settings.UseMatchRegions,
			PresentMinCoverage:     presentCoverage.GetText(),
			PartialMinCoverage:     partialCoverage.GetText(),
			PresentMinMatchedItems: presentItems.GetText(),
			PartialMinMatchedItems: partialItems.GetText(),
		}
		app.Stop()
	}

	interProControls := []tview.Primitive{interProBox}
	for _, box := range settingBoxes {
		interProControls = append(interProControls, box)
	}
	interProControls = append(interProControls, presentCoverage, presentItems, partialCoverage, partialItems)
	topIndex := 0
	interProIndex := 0
	setTopFocus := func(index int) {
		if index < 0 {
			index = 1
		}
		if index > 1 {
			index = 0
		}
		topIndex = index
		setFocusBorder(uniProtBox.Box, topIndex == 0)
		setFocusBorder(interProModule.Box, topIndex == 1)
		setFocusBorder(params.Box, topIndex == 1)
		if topIndex == 0 {
			app.SetFocus(uniProtBox)
			return
		}
		if interProIndex < 0 {
			interProIndex = 0
		}
		if interProIndex >= len(interProControls) {
			interProIndex = len(interProControls) - 1
		}
		app.SetFocus(interProControls[interProIndex])
	}
	setInterProFocus := func(index int) {
		if len(interProControls) == 0 {
			return
		}
		if index < 0 {
			index = len(interProControls) - 1
		}
		if index >= len(interProControls) {
			index = 0
		}
		interProIndex = index
		setTopFocus(1)
	}
	syncFocusFromApp := func() {
		current := app.GetFocus()
		if current == uniProtBox {
			topIndex = 0
			setFocusBorder(uniProtBox.Box, true)
			setFocusBorder(interProModule.Box, false)
			setFocusBorder(params.Box, false)
			return
		}
		for i, primitive := range interProControls {
			if current == primitive {
				topIndex = 1
				interProIndex = i
				setFocusBorder(uniProtBox.Box, false)
				setFocusBorder(interProModule.Box, true)
				setFocusBorder(params.Box, true)
				return
			}
		}
	}
	toggleFocused := func() bool {
		if topIndex == 0 {
			uniProtBox.toggleChecked()
			return true
		}
		if interProIndex >= 0 && interProIndex < len(interProControls) {
			if box, ok := interProControls[interProIndex].(*checkboxModule); ok {
				box.toggleChecked()
				return true
			}
		}
		return false
	}
	var closeHelp func()
	var helpModal *localizedHelpModal
	showHelp := func() {
		helpVisible = true
		if helpModal == nil {
			helpModal = newLocalizedHelpModal(app, interProParameterHelpPages(), func() {
				if closeHelp != nil {
					closeHelp()
				}
			})
		}
		helpModal.SetLanguage(app, int(helpLanguageIndex.Load()))
		app.SetRoot(infoModalRoot(modalFramePage(page.Breadcrumb, page.Path, helpModal.Title()), helpModal.Body(), 118, 40), true)
		app.SetFocus(helpModal.TextView())
	}
	closeHelp = func() {
		helpVisible = false
		app.SetRoot(mainRoot, true)
		setTopFocus(topIndex)
	}

	modalBody := newButtonFlex()
	modalBody.SetBorder(true)
	modalBody.SetTitle(" " + trimColon(page.Title) + " ")
	modalBody.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(modalBody.Box, true)
	attachFocusBorder(modalBody.Box)
	if strings.TrimSpace(page.Message) != "" {
		modalBody.AddItem(textBlock(page.Message), 3, 0, false)
	}
	modalBody.AddItem(uniProtBox, 3, 0, true)
	modalBody.AddItem(interProModule, 0, 1, false)
	addButtonRow(modalBody, modalButtons([]buttonSpec{
		{Label: ButtonBack, Shortcut: ShortcutBack, Action: func() { closeWithNav(NavBack) }, Visible: page.AllowBack},
		{Label: ButtonHelp, Shortcut: ShortcutHelp, Action: showHelp, Visible: true},
	}, true, firstNonEmptyText(page.ConfirmText, ButtonApply), "Enter", closeWithNav, confirm))
	addHints(modalBody, []string{"Tab switches between UniProt and InterPro. Up/Down moves through InterPro options. Space toggles a checkbox. Enter continues from UniProt or applies from InterPro. F1 opens parameter help."})

	messageHeight := 0
	if strings.TrimSpace(page.Message) != "" {
		messageHeight = 3
	}
	externalReferenceHeight := modalHeightForContent(messageHeight+3+2+2+3+len(settingBoxes)+1+2+1+5+1+4, 36, 46)
	mainRoot = infoModalRoot(modalFramePage(page.Breadcrumb, page.Path, page.Title), modalBody, 118, externalReferenceHeight)
	app.SetRoot(mainRoot, true)
	setTopFocus(0)
	installInputCapture(app, func(event *tcell.EventKey) *tcell.EventKey {
		if helpVisible {
			_ = helpModal.HandleKey(app, event, closeHelp)
			return nil
		}
		syncFocusFromApp()
		if shortcutMatchesEvent(ShortcutHelp, event) {
			showHelp()
			return nil
		}
		if topIndex == 1 {
			if input, ok := interProControls[interProIndex].(*tview.InputField); ok && inputFieldEditKey(event) {
				deliverInputFieldKey(input, event, app)
				return nil
			}
		}
		switch event.Key() {
		case tcell.KeyEscape:
			if page.AllowBack {
				closeWithNav(NavBack)
				return nil
			}
		case tcell.KeyTab:
			setTopFocus(topIndex + 1)
			return nil
		case tcell.KeyBacktab:
			setTopFocus(topIndex - 1)
			return nil
		case tcell.KeyEnter:
			if topIndex == 0 {
				setTopFocus(1)
			} else {
				confirm()
			}
			return nil
		case tcell.KeyUp:
			if topIndex == 1 {
				setInterProFocus(interProIndex - 1)
				return nil
			}
		case tcell.KeyDown:
			if topIndex == 1 {
				setInterProFocus(interProIndex + 1)
				return nil
			}
		case tcell.KeyLeft:
			if topIndex == 0 {
				setTopFocus(1)
				return nil
			}
		case tcell.KeyRight:
			if topIndex == 0 {
				setTopFocus(1)
				return nil
			}
		case tcell.KeyRune:
			if event.Rune() == ' ' {
				if toggleFocused() {
					return nil
				}
			}
		}
		return event
	})
	if err := runApp(app); err != nil {
		return ExternalReferenceResult{}, err
	}
	return result, nil
}

func RunFamilyBlastModal(page FamilyBlastPage) (FamilyBlastResult, error) {
	app := newApp()
	var result FamilyBlastResult
	settings := normalizeTUIFamilyBlastSettings(page.Settings)
	helpVisible := false
	var mainRoot tview.Primitive

	enableBox := newCheckboxModule("Group related queries as one family result", func() bool { return settings.Enabled }, func() { settings.Enabled = !settings.Enabled })
	detectBox := newCheckboxModule("Detect families from query names automatically", func() bool { return settings.GroupByDetectedPrefix }, func() { settings.GroupByDetectedPrefix = !settings.GroupByDetectedPrefix })
	mergeBox := newCheckboxModule("Merge rows that hit the same target gene/protein", func() bool { return settings.MergeRowsByTarget }, func() { settings.MergeRowsByTarget = !settings.MergeRowsByTarget })
	bestBox := newCheckboxModule("When merged, keep the strongest member hit", func() bool { return settings.KeepBestHitPerTarget }, func() { settings.KeepBestHitPerTarget = !settings.KeepBestHitPerTarget })
	prependFirstBox := newCheckboxModule("TXT export: include only the first query sequence", func() bool { return settings.PrependOnlyFirstQuery }, func() { settings.PrependOnlyFirstQuery = !settings.PrependOnlyFirstQuery })
	stripAtBox := newCheckboxModule("Remove Arabidopsis At/AT prefix for grouping", func() bool { return settings.StripArabidopsisPrefix }, func() { settings.StripArabidopsisPrefix = !settings.StripArabidopsisPrefix })
	stripSpeciesBox := newCheckboxModule("Remove leading species-style prefix", func() bool { return settings.StripLeadingSpeciesPrefix }, func() { settings.StripLeadingSpeciesPrefix = !settings.StripLeadingSpeciesPrefix })
	stripIndexBox := newCheckboxModule("Remove trailing member number", func() bool { return settings.StripTrailingQueryIndex }, func() { settings.StripTrailingQueryIndex = !settings.StripTrailingQueryIndex })
	stripAfterNumberBox := newCheckboxModule("Ignore suffix after a member number", func() bool { return settings.StripAfterNumberSuffix }, func() { settings.StripAfterNumberSuffix = !settings.StripAfterNumberSuffix })
	normalizePunctuationBox := newCheckboxModule("Treat punctuation as the same separator", func() bool { return settings.NormalizeInnerPunctuation }, func() { settings.NormalizeInnerPunctuation = !settings.NormalizeInnerPunctuation })
	stripSubtypeBox := newCheckboxModule("Remove terminal subtype suffix", func() bool { return settings.StripTerminalSubtypeSuffix }, func() { settings.StripTerminalSubtypeSuffix = !settings.StripTerminalSubtypeSuffix })
	keepSubgroupsBox := newCheckboxModule("Keep detected subgroups as separate families", func() bool { return settings.KeepDistinctQuerySubgroups }, func() { settings.KeepDistinctQuerySubgroups = !settings.KeepDistinctQuerySubgroups })
	for _, box := range []*checkboxModule{enableBox, detectBox, mergeBox, bestBox, prependFirstBox, stripAtBox, stripSpeciesBox, stripIndexBox, stripAfterNumberBox, normalizePunctuationBox, stripSubtypeBox, keepSubgroupsBox} {
		box.SetBorder(false)
	}
	minGroupInput := tview.NewInputField().SetLabel("minimum queries in a family ").SetText(settings.MinimumGroupSize).SetFieldWidth(8)
	rankingOrderInput := tview.NewInputField().SetLabel("best-hit ranking order ").SetText(settings.RankingTieBreakerOrder).SetFieldWidth(36)
	minGroupInput.SetFieldTextColor(tview.Styles.PrimaryTextColor)
	minGroupInput.SetLabelColor(tview.Styles.SecondaryTextColor)
	minGroupInput.SetFieldBackgroundColor(colorPanel)
	rankingOrderInput.SetFieldTextColor(tview.Styles.PrimaryTextColor)
	rankingOrderInput.SetLabelColor(tview.Styles.SecondaryTextColor)
	rankingOrderInput.SetFieldBackgroundColor(colorPanel)

	buildPreviewLines := func() []string {
		lines := make([]string, 0, len(page.Groups)*3+4)
		if note := strings.TrimSpace(page.PreviewNote); note != "" {
			lines = append(lines, note, "")
		}
		if len(page.Groups) == 0 {
			return append(lines, "No likely family groups were detected.")
		}
		totalQueries := 0
		for _, group := range page.Groups {
			totalQueries += group.Queries
		}
		lines = append(lines, fmt.Sprintf("%d family group(s), %d grouped query record(s).", len(page.Groups), totalQueries), "")
		for _, group := range page.Groups {
			members := compactFamilyBlastMembers(group.Members, group.Labels)
			lines = append(lines, fmt.Sprintf("%s (%d)", group.Name, group.Queries))
			if len(members) == 0 {
				lines = append(lines, "  (no members listed)")
				continue
			}
			for _, member := range members {
				lines = append(lines, "  - "+familyBlastPreviewMemberText(member))
			}
			lines = append(lines, "")
		}
		return lines
	}
	detectedModule := textPanel("Preview", strings.Join(buildPreviewLines(), "\n"))
	detectedModule.SetScrollable(true)
	settingsModule := newButtonFlex()
	settingsModule.SetBorder(true)
	settingsModule.SetTitle(" Family BLAST settings ")
	settingsModule.SetTitleAlign(tview.AlignCenter)
	settingsModule.AddItem(textBlock("Each query still runs its own BLAST job. Family BLAST only changes review/export: related query members are shown and exported as one family result."), 3, 0, false)
	settingsModule.AddItem(sectionHeader("Workflow"), 1, 0, false)
	for _, primitive := range []tview.Primitive{enableBox, detectBox, minGroupInput} {
		settingsModule.AddItem(primitive, 1, 0, primitive == minGroupInput)
	}
	settingsModule.AddItem(sectionHeader("Name cleanup before grouping"), 1, 0, false)
	for _, primitive := range []tview.Primitive{stripSpeciesBox, normalizePunctuationBox, stripIndexBox, stripAfterNumberBox, stripSubtypeBox, stripAtBox, keepSubgroupsBox} {
		settingsModule.AddItem(primitive, 1, 0, false)
	}
	settingsModule.AddItem(sectionHeader("Merged rows and export"), 1, 0, false)
	for _, primitive := range []tview.Primitive{mergeBox, bestBox, rankingOrderInput, prependFirstBox} {
		settingsModule.AddItem(primitive, 1, 0, primitive == rankingOrderInput)
	}
	setFocusBorder(settingsModule.Box, true)
	attachFocusBorder(settingsModule.Box)

	controls := []tview.Primitive{enableBox, detectBox, minGroupInput, stripSpeciesBox, normalizePunctuationBox, stripIndexBox, stripAfterNumberBox, stripSubtypeBox, stripAtBox, keepSubgroupsBox, mergeBox, bestBox, rankingOrderInput, prependFirstBox}
	focusIndex := 0
	setFocusAt := func(index int) {
		if index < 0 {
			index = len(controls) - 1
		}
		if index >= len(controls) {
			index = 0
		}
		focusIndex = index
		setFocusBorder(detectedModule.Box, false)
		setFocusBorder(settingsModule.Box, true)
		for _, primitive := range controls {
			if box := primitiveBox(primitive); box != nil {
				setFocusBorder(box, false)
			}
		}
		if box := primitiveBox(controls[focusIndex]); box != nil {
			setFocusBorder(box, true)
		}
		app.SetFocus(controls[focusIndex])
	}
	focusedInput := func() *tview.InputField {
		input, _ := controls[focusIndex].(*tview.InputField)
		return input
	}
	toggleFocused := func() bool {
		if box, ok := controls[focusIndex].(*checkboxModule); ok {
			box.toggleChecked()
			return true
		}
		return false
	}
	closeWithNav := func(nav NavAction) {
		result.Nav = nav
		app.Stop()
	}
	captureSettingsWithCustomize := func(customizeGroups bool) FamilyBlastSettings {
		return FamilyBlastSettings{
			Enabled:                    settings.Enabled,
			GroupByDetectedPrefix:      settings.GroupByDetectedPrefix,
			MergeRowsByTarget:          settings.MergeRowsByTarget,
			KeepBestHitPerTarget:       settings.KeepBestHitPerTarget,
			PrependOnlyFirstQuery:      settings.PrependOnlyFirstQuery,
			CustomizeGroups:            customizeGroups,
			MinimumGroupSize:           minGroupInput.GetText(),
			StripArabidopsisPrefix:     settings.StripArabidopsisPrefix,
			StripLeadingSpeciesPrefix:  settings.StripLeadingSpeciesPrefix,
			StripTrailingQueryIndex:    settings.StripTrailingQueryIndex,
			StripAfterNumberSuffix:     settings.StripAfterNumberSuffix,
			NormalizeInnerPunctuation:  settings.NormalizeInnerPunctuation,
			StripTerminalSubtypeSuffix: settings.StripTerminalSubtypeSuffix,
			KeepDistinctQuerySubgroups: settings.KeepDistinctQuerySubgroups,
			RankingTieBreakerOrder:     rankingOrderInput.GetText(),
		}
	}
	confirm := func() {
		result.Settings = captureSettingsWithCustomize(false)
		app.Stop()
	}
	captureSettings := func() FamilyBlastSettings {
		return captureSettingsWithCustomize(false)
	}
	customizeGroups := func() {
		settingsResult := captureSettingsWithCustomize(true)
		customPage := FamilyBlastCustomizePage{
			Breadcrumb:       page.Breadcrumb,
			Path:             append(append([]string(nil), page.Path...), "Customize groups"),
			Title:            "Customize Family BLAST groups",
			Message:          "Review the proposed family groups. Move a member out from the left pane, or add an ungrouped query from the right pane. New groups can be created when the automatic grouping is not right.",
			Groups:           groupsToCustomGroups(page.Groups),
			Ungrouped:        append([]string(nil), page.PreviewUngrouped...),
			UngroupedMembers: compactFamilyBlastMembers(page.PreviewUngroupedMembers, page.PreviewUngrouped),
			AllowBack:        true,
			AllowHome:        page.AllowHome,
			ConfirmText:      page.ConfirmText,
		}
		customResult, err := RunFamilyBlastCustomizeModal(customPage)
		if err != nil || customResult.Nav == NavBack {
			setFocusAt(focusIndex)
			return
		}
		result.Settings = settingsResult
		result.CustomGroups = append([]FamilyBlastCustomGroup(nil), customResult.CustomGroups...)
		app.Stop()
	}

	var closeHelp func()
	var helpModal *localizedHelpModal
	showHelp := func() {
		helpVisible = true
		if helpModal == nil {
			helpModal = newLocalizedHelpModal(app, familyBlastHelpPages(), func() {
				if closeHelp != nil {
					closeHelp()
				}
			})
		}
		helpModal.SetLanguage(app, int(helpLanguageIndex.Load()))
		app.SetRoot(infoModalRoot(modalFramePage(page.Breadcrumb, page.Path, helpModal.Title()), helpModal.Body(), 118, 40), true)
		app.SetFocus(helpModal.TextView())
	}
	closeHelp = func() {
		helpVisible = false
		app.SetRoot(mainRoot, true)
		setFocusAt(focusIndex)
	}

	body := newButtonFlex()
	body.SetBorder(true)
	body.SetTitle(" " + trimColon(page.Title) + " ")
	body.SetTitleAlign(tview.AlignCenter)
	messageHeight := 0
	if strings.TrimSpace(page.Message) != "" {
		messageHeight = 3
		body.AddItem(textBlock(page.Message), messageHeight, 0, false)
	}
	referenceHeight := 0
	if strings.TrimSpace(page.Reference) != "" {
		referenceHeight = 3
		body.AddItem(textBlock(page.Reference), referenceHeight, 0, false)
	}
	contentRow := tview.NewFlex().
		AddItem(detectedModule, 44, 0, false).
		AddItem(settingsModule, 0, 1, true)
	body.AddItem(contentRow, 0, 1, true)
	actionButtons := buttonRow(
		buttonSpec{Label: ButtonBack, Shortcut: ShortcutBack, Action: func() { closeWithNav(NavBack) }, Visible: page.AllowBack},
		buttonSpec{Label: ButtonHelp, Shortcut: ShortcutHelp, Action: showHelp, Visible: true},
		buttonSpec{Label: "Refresh", Shortcut: "Ctrl+R", Action: func() {
			result.Settings = captureSettings()
			result.Nav = NavRefresh
			app.Stop()
		}, Visible: true},
		buttonSpec{Label: "Customize groups", Shortcut: "Ctrl+G", Action: customizeGroups, Visible: true, Primary: true},
		buttonSpec{Label: conciseActionLabel(firstNonEmptyText(page.ConfirmText, ButtonApply), ButtonApply), Shortcut: ShortcutApply, Action: confirm, Visible: true, Primary: true},
	)
	addButtonRow(body, actionButtons)
	addHints(body, []string{"Up/Down moves through options. Space toggles a checkbox. Ctrl+G opens the group editor. Ctrl+R refreshes the preview after changing grouping rules. Enter applies. F1 opens help."})

	buttonHeight := 1
	if actionButtons != nil {
		buttonHeight = actionButtons.requiredHeight(132)
	}
	hintsHeight := 1
	bodyBorderHeight := 2
	framePaddingHeight := 4
	settingsRows := 3 + 1 + 4 + 1 + 7 + 1 + 4
	contentRows := maxInt(18, settingsRows+2)
	height := modalHeightForContent(messageHeight+referenceHeight+buttonHeight+hintsHeight+bodyBorderHeight+framePaddingHeight+contentRows, 34, 48)
	mainRoot = infoModalRoot(modalFramePage(page.Breadcrumb, page.Path, page.Title), body, 144, height)
	app.SetRoot(mainRoot, true)
	setFocusAt(focusIndex)
	installInputCapture(app, func(event *tcell.EventKey) *tcell.EventKey {
		if helpVisible {
			_ = helpModal.HandleKey(app, event, closeHelp)
			return nil
		}
		if shortcutMatchesEvent(ShortcutHelp, event) {
			showHelp()
			return nil
		}
		if event.Key() == tcell.KeyCtrlR {
			result.Settings = captureSettings()
			result.Nav = NavRefresh
			app.Stop()
			return nil
		}
		if shortcutMatchesEvent("Ctrl+G", event) {
			customizeGroups()
			return nil
		}
		if input := focusedInput(); input != nil && inputFieldEditKey(event) {
			deliverInputFieldKey(input, event, app)
			return nil
		}
		switch event.Key() {
		case tcell.KeyEscape:
			if page.AllowBack {
				closeWithNav(NavBack)
				return nil
			}
		case tcell.KeyUp:
			setFocusAt(focusIndex - 1)
			return nil
		case tcell.KeyDown:
			setFocusAt(focusIndex + 1)
			return nil
		case tcell.KeyTab:
			setFocusAt(focusIndex + 1)
			return nil
		case tcell.KeyBacktab:
			setFocusAt(focusIndex - 1)
			return nil
		case tcell.KeyEnter:
			confirm()
			return nil
		case tcell.KeyRune:
			if event.Rune() == ' ' && toggleFocused() {
				return nil
			}
		}
		return event
	})
	if err := runApp(app); err != nil {
		return FamilyBlastResult{}, err
	}
	return result, nil
}

func compactFamilyBlastGroupLabels(labels []string) []string {
	out := make([]string, 0, len(labels))
	seen := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		key := strings.ToLower(label)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, label)
	}
	return out
}

func compactFamilyBlastMembers(members []FamilyBlastMember, fallbackLabels []string) []FamilyBlastMember {
	out := make([]FamilyBlastMember, 0, maxInt(len(members), len(fallbackLabels)))
	seen := make(map[string]struct{}, maxInt(len(members), len(fallbackLabels)))
	add := func(member FamilyBlastMember) {
		member.LabelName = strings.TrimSpace(member.LabelName)
		member.ProteinID = strings.TrimSpace(member.ProteinID)
		member.OriginalLabelName = strings.TrimSpace(member.OriginalLabelName)
		member.SourceKey = strings.TrimSpace(member.SourceKey)
		if member.LabelName == "" {
			return
		}
		if member.OriginalLabelName == "" {
			member.OriginalLabelName = member.LabelName
		}
		if member.SourceKey == "" {
			member.SourceKey = firstNonEmptyText(member.ProteinID, member.OriginalLabelName, member.LabelName)
		}
		member.Aliases = compactFamilyBlastGroupLabels(append(member.Aliases, member.LabelName))
		key := strings.ToLower(firstNonEmptyText(member.SourceKey, member.ProteinID, member.OriginalLabelName, member.LabelName))
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, member)
	}
	for _, member := range members {
		add(member)
	}
	if len(out) == 0 {
		for _, label := range compactFamilyBlastGroupLabels(fallbackLabels) {
			add(FamilyBlastMember{
				LabelName:         label,
				OriginalLabelName: label,
				SourceKey:         label,
				Aliases:           []string{label},
			})
		}
	}
	return out
}

func groupsToCustomGroups(groups []FamilyBlastGroup) []FamilyBlastCustomGroup {
	out := make([]FamilyBlastCustomGroup, 0, len(groups))
	for _, group := range groups {
		members := compactFamilyBlastMembers(group.Members, group.Labels)
		labels := make([]string, 0, len(members))
		for _, member := range members {
			labels = append(labels, member.LabelName)
		}
		out = append(out, FamilyBlastCustomGroup{
			Name:    strings.TrimSpace(group.Name),
			Labels:  labels,
			Members: members,
		})
	}
	return out
}

func familyBlastMemberDisplay(member FamilyBlastMember) (string, string) {
	label := strings.TrimSpace(member.LabelName)
	if label == "" {
		label = "(unnamed)"
	}
	proteinID := strings.TrimSpace(member.ProteinID)
	if proteinID == "" {
		return label, ""
	}
	return label, "[" + proteinID + "]"
}

func familyBlastMemberInlineDisplay(member FamilyBlastMember) string {
	label := strings.TrimSpace(member.LabelName)
	if label == "" {
		label = "(unnamed)"
	}
	proteinID := strings.TrimSpace(member.ProteinID)
	if proteinID == "" {
		return tview.Escape(label)
	}
	return tview.Escape(label) + " [yellow][" + tview.Escape(proteinID) + "][-]"
}

func familyBlastPreviewMemberText(member FamilyBlastMember) string {
	return familyBlastMemberInlineDisplay(member)
}

func visibleTreeNodes(root *tview.TreeNode) []*tview.TreeNode {
	out := make([]*tview.TreeNode, 0, 16)
	var walk func(node *tview.TreeNode)
	walk = func(node *tview.TreeNode) {
		if node == nil {
			return
		}
		if node.GetReference() != nil {
			out = append(out, node)
		}
		if !node.IsExpanded() {
			return
		}
		for _, child := range node.GetChildren() {
			walk(child)
		}
	}
	walk(root)
	return out
}

func familyGroupUngroupedLabels(groups []FamilyBlastGroup) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, group := range groups {
		for _, label := range compactFamilyBlastGroupLabels(group.Labels) {
			key := strings.ToLower(strings.TrimSpace(label))
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, label)
		}
	}
	return out
}

func familyBlastMemberKey(member FamilyBlastMember) string {
	return strings.ToLower(firstNonEmptyText(member.SourceKey, member.ProteinID, member.OriginalLabelName, member.LabelName))
}

func sortFamilyBlastMembersStable(members []FamilyBlastMember) {
	sort.SliceStable(members, func(i, j int) bool {
		left := strings.ToLower(firstNonEmptyText(members[i].LabelName, members[i].ProteinID))
		right := strings.ToLower(firstNonEmptyText(members[j].LabelName, members[j].ProteinID))
		return left < right
	})
}

type familyBlastCustomizeModal struct {
	app                      *tview.Application
	root                     tview.Primitive
	groupedList              *tview.List
	rightList                *tview.List
	chooseGroupOverlayHeight int
}

func buildFamilyBlastCustomizeModal(page FamilyBlastCustomizePage, app *tview.Application, result *FamilyBlastResult) *familyBlastCustomizeModal {
	modal := &familyBlastCustomizeModal{app: app}
	var mainRoot tview.Primitive

	type editableGroup struct {
		Name    string
		Members []FamilyBlastMember
	}
	type groupedListRef struct {
		Group    int
		Member   int
		IsGroup  bool
		MemberID string
	}

	groups := make([]editableGroup, 0, len(page.Groups))
	for _, group := range page.Groups {
		members := compactFamilyBlastMembers(group.Members, group.Labels)
		sortFamilyBlastMembersStable(members)
		groups = append(groups, editableGroup{
			Name:    strings.TrimSpace(group.Name),
			Members: members,
		})
	}
	ungrouped := compactFamilyBlastMembers(page.UngroupedMembers, page.Ungrouped)
	sortFamilyBlastMembersStable(ungrouped)
	activePane := 0
	selectedGroup := 0
	selectedGroupedMemberID := ""
	selectedUngrouped := 0
	statusLine := ""
	subModalOpen := false
	var subModalCapture inputCaptureFunc
	groupedRows := make([]groupedListRef, 0, 16)

	groupedList := tview.NewList()
	groupedList.SetBorder(true).SetTitle(" Grouped items ").SetTitleAlign(tview.AlignCenter)
	groupedList.ShowSecondaryText(false)
	groupedList.SetMainTextColor(tview.Styles.PrimaryTextColor)
	groupedList.SetSelectedFocusOnly(true)
	groupedList.SetSelectedTextColor(tcell.ColorWhite)
	groupedList.SetSelectedBackgroundColor(tcell.ColorBlue)
	setFocusBorder(groupedList.Box, true)

	rightList := tview.NewList()
	rightList.SetBorder(true).SetTitle(" Ungrouped items ").SetTitleAlign(tview.AlignCenter)
	rightList.ShowSecondaryText(false)
	rightList.SetMainTextColor(tview.Styles.PrimaryTextColor)
	rightList.SetSelectedFocusOnly(true)
	rightList.SetSelectedTextColor(tcell.ColorWhite)
	rightList.SetSelectedBackgroundColor(tcell.ColorBlue)
	setFocusBorder(rightList.Box, false)

	statusView := hintView("")

	setActivePane := func(index int) {
		if index < 0 || index > 1 {
			index = 0
		}
		activePane = index
		setFocusBorder(groupedList.Box, index == 0)
		setFocusBorder(rightList.Box, index == 1)
	}

	setPaneFocus := func(index int) {
		setActivePane(index)
		if index == 0 {
			app.SetFocus(groupedList)
			return
		}
		app.SetFocus(rightList)
	}
	moveListSelection := func(list *tview.List, delta int) {
		if list == nil {
			return
		}
		count := list.GetItemCount()
		if count <= 0 {
			return
		}
		index := list.GetCurrentItem() + delta
		if index < 0 {
			index = 0
		}
		if index >= count {
			index = count - 1
		}
		list.SetCurrentItem(index)
	}
	setListSelection := func(list *tview.List, index int) {
		if list == nil {
			return
		}
		count := list.GetItemCount()
		if count <= 0 {
			return
		}
		if index < 0 {
			index = 0
		}
		if index >= count {
			index = count - 1
		}
		list.SetCurrentItem(index)
	}
	activeList := func() *tview.List {
		if activePane == 1 {
			return rightList
		}
		return groupedList
	}

	applyGroupedSelection := func(index int) {
		if index < 0 || index >= len(groupedRows) {
			if len(groups) == 0 {
				selectedGroup = 0
			}
			selectedGroupedMemberID = ""
			return
		}
		ref := groupedRows[index]
		selectedGroup = ref.Group
		if ref.IsGroup {
			selectedGroupedMemberID = ""
			return
		}
		selectedGroupedMemberID = ref.MemberID
	}

	var refreshGroupedList func()
	var refreshRightList func()
	var refreshStatus func()
	selectedGroupIndex := func() int {
		if len(groups) == 0 {
			return -1
		}
		if selectedGroup < 0 {
			selectedGroup = 0
		}
		if selectedGroup >= len(groups) {
			selectedGroup = len(groups) - 1
		}
		return selectedGroup
	}
	selectedUngroupedMember := func() (FamilyBlastMember, bool) {
		if len(ungrouped) == 0 {
			return FamilyBlastMember{}, false
		}
		if selectedUngrouped < 0 {
			selectedUngrouped = 0
		}
		if selectedUngrouped >= len(ungrouped) {
			selectedUngrouped = len(ungrouped) - 1
		}
		return ungrouped[selectedUngrouped], true
	}
	removeUngroupedAt := func(index int) (FamilyBlastMember, bool) {
		if index < 0 || index >= len(ungrouped) {
			return FamilyBlastMember{}, false
		}
		value := ungrouped[index]
		ungrouped = append(ungrouped[:index], ungrouped[index+1:]...)
		if selectedUngrouped >= len(ungrouped) && len(ungrouped) > 0 {
			selectedUngrouped = len(ungrouped) - 1
		}
		return value, true
	}
	removeGroupMember := func(groupIndex int, memberID string) (FamilyBlastMember, bool) {
		if groupIndex < 0 || groupIndex >= len(groups) {
			return FamilyBlastMember{}, false
		}
		for i, existing := range groups[groupIndex].Members {
			if familyBlastMemberKey(existing) == strings.ToLower(strings.TrimSpace(memberID)) {
				groups[groupIndex].Members = append(groups[groupIndex].Members[:i], groups[groupIndex].Members[i+1:]...)
				return existing, true
			}
		}
		return FamilyBlastMember{}, false
	}
	moveSelectedUngroupedToGroup := func(groupIndex int) {
		member, ok := selectedUngroupedMember()
		if !ok || groupIndex < 0 || groupIndex >= len(groups) {
			return
		}
		removeUngroupedAt(selectedUngrouped)
		groups[groupIndex].Members = append(groups[groupIndex].Members, member)
		sortFamilyBlastMembersStable(groups[groupIndex].Members)
		statusLine = fmt.Sprintf("Added %s to %s.", member.LabelName, groups[groupIndex].Name)
		selectedGroup = groupIndex
		selectedGroupedMemberID = familyBlastMemberKey(member)
		refreshGroupedList()
		refreshRightList()
		refreshStatus()
	}
	moveMemberOutOfGroup := func(groupIndex int, memberID string) {
		member, ok := removeGroupMember(groupIndex, memberID)
		if !ok {
			return
		}
		ungrouped = append(ungrouped, member)
		sortFamilyBlastMembersStable(ungrouped)
		statusLine = fmt.Sprintf("Removed %s from %s.", member.LabelName, groups[groupIndex].Name)
		selectedGroup = groupIndex
		selectedGroupedMemberID = ""
		refreshGroupedList()
		refreshRightList()
		refreshStatus()
	}
	deleteGroup := func(groupIndex int) {
		if groupIndex < 0 || groupIndex >= len(groups) {
			return
		}
		ungrouped = append(ungrouped, groups[groupIndex].Members...)
		sortFamilyBlastMembersStable(ungrouped)
		statusLine = fmt.Sprintf("Deleted group %s.", groups[groupIndex].Name)
		groups = append(groups[:groupIndex], groups[groupIndex+1:]...)
		if selectedGroup >= len(groups) && len(groups) > 0 {
			selectedGroup = len(groups) - 1
		}
		selectedGroupedMemberID = ""
		refreshGroupedList()
		refreshRightList()
		refreshStatus()
	}
	showNameInputModal := func(title string, confirmLabel string, initial string, onConfirm func(string) string) {
		input := tview.NewInputField().SetLabel("name ").SetText(strings.TrimSpace(initial)).SetFieldWidth(24)
		input.SetFieldTextColor(tview.Styles.PrimaryTextColor)
		input.SetLabelColor(tview.Styles.SecondaryTextColor)
		input.SetFieldBackgroundColor(colorPanel)
		message := hintView("")
		closeModal := func() {
			subModalOpen = false
			subModalCapture = nil
			app.SetRoot(mainRoot, true)
			setPaneFocus(activePane)
		}
		confirmModal := func() {
			if msg := onConfirm(strings.TrimSpace(input.GetText())); msg != "" {
				message.SetText(msg)
				return
			}
			closeModal()
		}
		box := newButtonFlex()
		box.SetBorder(true)
		box.SetTitle(" " + trimColon(title) + " ")
		box.SetTitleAlign(tview.AlignCenter)
		box.AddItem(input, 1, 0, true)
		box.AddItem(message, 1, 0, false)
		addButtonRow(box, modalButtons([]buttonSpec{
			{Label: ButtonClose, Shortcut: ShortcutBack, Action: closeModal, Visible: true},
		}, true, confirmLabel, ShortcutApply, func(NavAction) {}, confirmModal))
		subModalOpen = true
		subModalCapture = func(event *tcell.EventKey) *tcell.EventKey {
			if event == nil {
				return nil
			}
			switch event.Key() {
			case tcell.KeyEscape:
				closeModal()
				return nil
			case tcell.KeyEnter:
				confirmModal()
				return nil
			case tcell.KeyTab, tcell.KeyBacktab:
				return nil
			}
			if inputFieldEditKey(event) {
				deliverInputFieldKey(input, event, app)
				return nil
			}
			return nil
		}
		overlay := overlayRootOn(mainRoot, box, 40, 7)
		app.SetRoot(overlay, true)
		app.SetFocus(input)
	}
	createGroup := func() {
		showNameInputModal("New group", "Create", "", func(name string) string {
			if name == "" {
				return "Enter a group name."
			}
			for _, group := range groups {
				if strings.EqualFold(group.Name, name) {
					return "That group name already exists."
				}
			}
			groups = append(groups, editableGroup{Name: name})
			selectedGroup = len(groups) - 1
			statusLine = fmt.Sprintf("Created group %s.", name)
			selectedGroupedMemberID = ""
			refreshGroupedList()
			refreshStatus()
			return ""
		})
	}
	renameSelected := func() {
		if activePane == 0 {
			index := groupedList.GetCurrentItem()
			if index < 0 || index >= len(groupedRows) {
				return
			}
			ref := groupedRows[index]
			if ref.IsGroup {
				if ref.Group < 0 || ref.Group >= len(groups) {
					return
				}
				showNameInputModal("Rename group", "Rename", groups[ref.Group].Name, func(name string) string {
					if name == "" {
						return "Enter a group name."
					}
					for gi, group := range groups {
						if gi != ref.Group && strings.EqualFold(group.Name, name) {
							return "That group name already exists."
						}
					}
					groups[ref.Group].Name = name
					statusLine = fmt.Sprintf("Renamed group to %s.", name)
					refreshGroupedList()
					refreshStatus()
					return ""
				})
				return
			}
			if ref.Group < 0 || ref.Group >= len(groups) || ref.Member < 0 || ref.Member >= len(groups[ref.Group].Members) {
				return
			}
			showNameInputModal("Rename item labelname", "Rename", groups[ref.Group].Members[ref.Member].LabelName, func(name string) string {
				if name == "" {
					return "Enter a labelname."
				}
				groups[ref.Group].Members[ref.Member].LabelName = name
				groups[ref.Group].Members[ref.Member].Aliases = compactFamilyBlastGroupLabels(append(groups[ref.Group].Members[ref.Member].Aliases, name))
				selectedGroupedMemberID = familyBlastMemberKey(groups[ref.Group].Members[ref.Member])
				statusLine = fmt.Sprintf("Renamed item to %s.", name)
				refreshGroupedList()
				refreshStatus()
				return ""
			})
			return
		}
		member, ok := selectedUngroupedMember()
		if !ok {
			return
		}
		showNameInputModal("Rename item labelname", "Rename", member.LabelName, func(name string) string {
			if name == "" {
				return "Enter a labelname."
			}
			ungrouped[selectedUngrouped].LabelName = name
			ungrouped[selectedUngrouped].Aliases = compactFamilyBlastGroupLabels(append(ungrouped[selectedUngrouped].Aliases, name))
			statusLine = fmt.Sprintf("Renamed item to %s.", name)
			refreshRightList()
			refreshStatus()
			return ""
		})
	}
	showChooseGroupModal := func(member FamilyBlastMember) {
		if strings.TrimSpace(member.LabelName) == "" || len(groups) == 0 {
			statusLine = "Create a group first."
			refreshStatus()
			return
		}
		list := tview.NewList()
		list.ShowSecondaryText(false)
		list.SetSelectedTextColor(tcell.ColorBlack)
		list.SetSelectedBackgroundColor(tcell.ColorWhite)
		list.SetBorder(true).SetTitle(" Choose target group ").SetTitleAlign(tview.AlignCenter)
		for _, group := range groups {
			list.AddItem(fmt.Sprintf("%s (%d)", group.Name, len(group.Members)), "", 0, nil)
		}
		if idx := selectedGroupIndex(); idx >= 0 {
			list.SetCurrentItem(idx)
		}
		closeModal := func() {
			subModalOpen = false
			subModalCapture = nil
			app.SetRoot(mainRoot, true)
			setPaneFocus(1)
		}
		applyMove := func() {
			index := list.GetCurrentItem()
			if index < 0 || index >= len(groups) {
				return
			}
			key := familyBlastMemberKey(member)
			for ui, candidate := range ungrouped {
				if familyBlastMemberKey(candidate) == key {
					selectedUngrouped = ui
					break
				}
			}
			moveSelectedUngroupedToGroup(index)
		}
		box := newButtonFlex()
		box.SetBorder(true)
		box.SetTitle(" Add to group ")
		box.SetTitleAlign(tview.AlignCenter)
		box.AddItem(textBlock("Choose the destination group for the selected ungrouped item."), 2, 0, false)
		box.AddItem(list, 0, 1, true)
		addButtonRow(box, modalButtons([]buttonSpec{
			{Label: ButtonClose, Shortcut: ShortcutBack, Action: closeModal, Visible: true},
		}, true, "Add", ShortcutApply, func(NavAction) {}, func() {
			applyMove()
			closeModal()
		}))
		subModalOpen = true
		subModalCapture = func(event *tcell.EventKey) *tcell.EventKey {
			if event == nil {
				return nil
			}
			switch event.Key() {
			case tcell.KeyEscape:
				closeModal()
				return nil
			case tcell.KeyEnter:
				applyMove()
				closeModal()
				return nil
			}
			if handler := list.InputHandler(); handler != nil {
				handler(event, func(p tview.Primitive) {
					if p != nil {
						app.SetFocus(p)
					}
				})
			}
			return nil
		}
		overlayHeight := minInt(36, maxInt(12, len(groups)+8))
		modal.chooseGroupOverlayHeight = overlayHeight
		overlay := overlayRootOn(mainRoot, box, 60, overlayHeight)
		app.SetRoot(overlay, true)
		app.SetFocus(list)
	}
	selectedItemMember := func() (FamilyBlastMember, func(FamilyBlastMember), bool) {
		if activePane == 0 {
			index := groupedList.GetCurrentItem()
			if index < 0 || index >= len(groupedRows) {
				return FamilyBlastMember{}, nil, false
			}
			ref := groupedRows[index]
			if ref.IsGroup || ref.Group < 0 || ref.Group >= len(groups) || ref.Member < 0 || ref.Member >= len(groups[ref.Group].Members) {
				return FamilyBlastMember{}, nil, false
			}
			return groups[ref.Group].Members[ref.Member], func(next FamilyBlastMember) {
				groups[ref.Group].Members[ref.Member] = next
				selectedGroupedMemberID = familyBlastMemberKey(next)
				refreshGroupedList()
				refreshStatus()
			}, true
		}
		if selectedUngrouped < 0 || selectedUngrouped >= len(ungrouped) {
			return FamilyBlastMember{}, nil, false
		}
		return ungrouped[selectedUngrouped], func(next FamilyBlastMember) {
			ungrouped[selectedUngrouped] = next
			refreshRightList()
			refreshStatus()
		}, true
	}
	showAliasModal := func() {
		member, updateMember, ok := selectedItemMember()
		if !ok {
			statusLine = "Select an item to view aliases."
			refreshStatus()
			return
		}
		aliases := compactFamilyBlastGroupLabels(member.Aliases)
		if len(aliases) == 0 {
			aliases = []string{member.LabelName}
		}
		list := tview.NewList()
		list.ShowSecondaryText(false)
		list.SetSelectedTextColor(tcell.ColorBlack)
		list.SetSelectedBackgroundColor(tcell.ColorWhite)
		list.SetBorder(true).SetTitle(" Alias labelnames ").SetTitleAlign(tview.AlignCenter)
		for _, alias := range aliases {
			list.AddItem(alias, "", 0, nil)
		}
		closeModal := func() {
			subModalOpen = false
			subModalCapture = nil
			app.SetRoot(mainRoot, true)
			setPaneFocus(activePane)
		}
		selectedAlias := func() string {
			if len(aliases) == 0 {
				return ""
			}
			index := list.GetCurrentItem()
			if index < 0 {
				index = 0
			}
			if index >= len(aliases) {
				index = len(aliases) - 1
			}
			return aliases[index]
		}
		copyAlias := func() {
			alias := selectedAlias()
			if alias == "" {
				return
			}
			if err := writeClipboardText(alias); err != nil {
				statusLine = "Copy failed: " + err.Error()
			} else {
				statusLine = "Copied alias labelname."
			}
			refreshStatus()
		}
		setAliasAsLabel := func() {
			alias := selectedAlias()
			if alias == "" || updateMember == nil {
				return
			}
			member.LabelName = alias
			member.Aliases = compactFamilyBlastGroupLabels(append(member.Aliases, alias))
			updateMember(member)
			statusLine = fmt.Sprintf("Set labelname to %s.", alias)
			refreshStatus()
			closeModal()
		}
		box := newButtonFlex()
		box.SetBorder(true)
		box.SetTitle(" Aliases ")
		box.SetTitleAlign(tview.AlignCenter)
		box.AddItem(textBlock("Choose an alias labelname. Copy copies the selected alias; Set as labelname fixes it as this item's labelname."), 3, 0, false)
		box.AddItem(list, 0, 1, true)
		addButtonRow(box, buttonRow(
			buttonSpec{Label: ButtonClose, Shortcut: ShortcutBack, Action: closeModal, Visible: true},
			buttonSpec{Label: ButtonCopy, Shortcut: ShortcutCopy, Action: copyAlias, Visible: true},
			buttonSpec{Label: "Set as labelname", Shortcut: "F2", Action: setAliasAsLabel, Visible: true, Primary: true},
		))
		subModalOpen = true
		subModalCapture = func(event *tcell.EventKey) *tcell.EventKey {
			if event == nil {
				return nil
			}
			if isCopyShortcut(event) {
				copyAlias()
				return nil
			}
			if event.Key() == tcell.KeyF2 {
				setAliasAsLabel()
				return nil
			}
			switch event.Key() {
			case tcell.KeyEscape:
				closeModal()
				return nil
			case tcell.KeyEnter:
				setAliasAsLabel()
				return nil
			}
			if handler := list.InputHandler(); handler != nil {
				handler(event, func(p tview.Primitive) {
					if p != nil {
						app.SetFocus(p)
					}
				})
			}
			return nil
		}
		overlayHeight := minInt(36, maxInt(12, len(aliases)+8))
		overlay := overlayRootOn(mainRoot, box, 68, overlayHeight)
		app.SetRoot(overlay, true)
		app.SetFocus(list)
	}
	confirm := func() {
		if len(groups) == 0 {
			statusLine = "Create at least one group."
			refreshStatus()
			return
		}
		for _, group := range groups {
			if len(group.Members) < 2 {
				statusLine = fmt.Sprintf("Group %s must contain at least 2 items.", group.Name)
				refreshStatus()
				return
			}
		}
		result.CustomGroups = make([]FamilyBlastCustomGroup, 0, len(groups))
		for _, group := range groups {
			labels := make([]string, 0, len(group.Members))
			for _, member := range group.Members {
				labels = append(labels, member.LabelName)
			}
			result.CustomGroups = append(result.CustomGroups, FamilyBlastCustomGroup{
				Name:    group.Name,
				Labels:  labels,
				Members: append([]FamilyBlastMember(nil), group.Members...),
			})
		}
		app.Stop()
	}

	refreshGroupedList = func() {
		groupedList.Clear()
		groupedRows = groupedRows[:0]
		currentIndex := -1
		for gi, group := range groups {
			groupedRows = append(groupedRows, groupedListRef{Group: gi, IsGroup: true})
			groupedList.AddItem(fmt.Sprintf("%s (%d)", group.Name, len(group.Members)), "", 0, nil)
			if gi == selectedGroup && selectedGroupedMemberID == "" {
				currentIndex = len(groupedRows) - 1
			}
			for mi, member := range group.Members {
				memberID := familyBlastMemberKey(member)
				groupedRows = append(groupedRows, groupedListRef{Group: gi, Member: mi, MemberID: memberID})
				groupedList.AddItem("  - "+familyBlastMemberInlineDisplay(member), "", 0, nil)
				if gi == selectedGroup && selectedGroupedMemberID == memberID {
					currentIndex = len(groupedRows) - 1
				}
			}
		}
		if currentIndex < 0 && len(groupedRows) > 0 {
			groupIndex := selectedGroupIndex()
			for i, row := range groupedRows {
				if row.Group == groupIndex && row.IsGroup {
					currentIndex = i
					break
				}
			}
		}
		if currentIndex < 0 && len(groupedRows) > 0 {
			currentIndex = 0
		}
		if currentIndex >= 0 {
			groupedList.SetCurrentItem(currentIndex)
			applyGroupedSelection(currentIndex)
		} else {
			selectedGroupedMemberID = ""
		}
	}
	refreshRightList = func() {
		rightList.Clear()
		for _, member := range ungrouped {
			rightList.AddItem(familyBlastMemberInlineDisplay(member), "", 0, nil)
		}
		if len(ungrouped) > 0 {
			if selectedUngrouped >= len(ungrouped) {
				selectedUngrouped = len(ungrouped) - 1
			}
			rightList.SetCurrentItem(selectedUngrouped)
		} else {
			selectedUngrouped = 0
		}
	}
	refreshStatus = func() {
		statusView.SetText(strings.TrimSpace(statusLine))
	}

	groupedList.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		applyGroupedSelection(index)
	})
	rightList.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		selectedUngrouped = index
	})
	groupedList.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if event != nil && (action == tview.MouseLeftClick || action == tview.MouseLeftDown) {
			setPaneFocus(0)
		}
		return action, event
	})
	rightList.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if event != nil && (action == tview.MouseLeftClick || action == tview.MouseLeftDown) {
			setPaneFocus(1)
		}
		return action, event
	})

	body := newButtonFlex()
	body.SetBorder(true)
	body.SetTitle(" " + trimColon(page.Title) + " ")
	body.SetTitleAlign(tview.AlignCenter)
	if strings.TrimSpace(page.Message) != "" {
		body.AddItem(textBlock(page.Message), 4, 0, false)
	}

	workArea := tview.NewFlex().
		AddItem(groupedList, 0, 1, false).
		AddItem(rightList, 0, 1, false)
	content := &focusProxyPrimitive{
		Box:   tview.NewBox(),
		child: workArea,
		focusTarget: func() tview.Primitive {
			if activePane == 1 {
				return rightList
			}
			return groupedList
		},
	}
	body.AddItem(content, 0, 1, true)

	body.AddItem(statusView, 1, 0, false)

	actionButtons := buttonRow(
		buttonSpec{Label: ButtonBack, Shortcut: ShortcutBack, Action: func() { result.Nav = NavBack; app.Stop() }, Visible: page.AllowBack},
		buttonSpec{Label: "New group", Shortcut: "Ctrl+N", Action: createGroup, Visible: true},
		buttonSpec{Label: "Rename", Shortcut: "F2", Action: renameSelected, Visible: true},
		buttonSpec{Label: "Aliases", Shortcut: "Ctrl+L", Action: func() {
			if activePane == 0 {
				index := groupedList.GetCurrentItem()
				if index < 0 || index >= len(groupedRows) {
					return
				}
				ref := groupedRows[index]
				if ref.IsGroup {
					statusLine = "Select an item to view aliases."
					refreshStatus()
					return
				}
			}
			showAliasModal()
		}, Visible: true},
		buttonSpec{Label: "Remove / delete", Shortcut: "Del", Action: func() {
			if activePane != 0 {
				return
			}
			index := groupedList.GetCurrentItem()
			if index < 0 || index >= len(groupedRows) {
				return
			}
			ref := groupedRows[index]
			if !ref.IsGroup && ref.MemberID != "" {
				moveMemberOutOfGroup(ref.Group, ref.MemberID)
				return
			}
			if ref.IsGroup && ref.Group >= 0 {
				deleteGroup(ref.Group)
			}
		}, Visible: true},
		buttonSpec{Label: "Add to group", Shortcut: "Enter", Action: func() {
			if activePane == 1 {
				if member, ok := selectedUngroupedMember(); ok {
					showChooseGroupModal(member)
				}
			}
		}, Visible: true},
		buttonSpec{Label: conciseActionLabel(firstNonEmptyText(page.ConfirmText, ButtonApply), ButtonApply), Shortcut: "Ctrl+Enter", Action: confirm, Visible: true, Primary: true},
	)
	addButtonRow(body, actionButtons)
	addHints(body, []string{"Tab switches panes. Up/Down chooses items. Enter removes a grouped member or adds an ungrouped item. F2 renames the selected group/item. Ctrl+L opens item aliases. Ctrl+Shift+C copies an alias in the alias dialog. Delete removes/deletes. Ctrl+N creates a group. Ctrl+Enter applies."})

	mainRoot = infoModalRoot(modalFramePage(page.Breadcrumb, page.Path, page.Title), body, 148, 36)
	refreshGroupedList()
	refreshRightList()
	refreshStatus()
	app.SetRoot(mainRoot, true)
	setPaneFocus(0)
	modal.root = mainRoot
	modal.groupedList = groupedList
	modal.rightList = rightList

	installInputCapture(app, func(event *tcell.EventKey) *tcell.EventKey {
		if subModalOpen {
			if subModalCapture != nil {
				return subModalCapture(event)
			}
			return nil
		}
		switch event.Key() {
		case tcell.KeyEscape:
			if page.AllowBack {
				result.Nav = NavBack
				app.Stop()
				return nil
			}
		case tcell.KeyTab:
			setPaneFocus((activePane + 1) % 2)
			return nil
		case tcell.KeyBacktab:
			setPaneFocus((activePane + 1) % 2)
			return nil
		case tcell.KeyUp:
			moveListSelection(activeList(), -1)
			return nil
		case tcell.KeyDown:
			moveListSelection(activeList(), 1)
			return nil
		case tcell.KeyHome:
			setListSelection(activeList(), 0)
			return nil
		case tcell.KeyEnd:
			if list := activeList(); list != nil {
				setListSelection(list, list.GetItemCount()-1)
			}
			return nil
		case tcell.KeyPgUp:
			moveListSelection(activeList(), -8)
			return nil
		case tcell.KeyPgDn:
			moveListSelection(activeList(), 8)
			return nil
		case tcell.KeyDelete, tcell.KeyDEL:
			if activePane != 0 {
				return event
			}
			index := groupedList.GetCurrentItem()
			if index < 0 || index >= len(groupedRows) {
				return nil
			}
			ref := groupedRows[index]
			if !ref.IsGroup && ref.MemberID != "" {
				moveMemberOutOfGroup(ref.Group, ref.MemberID)
			} else if ref.IsGroup && ref.Group >= 0 {
				deleteGroup(ref.Group)
			}
			return nil
		case tcell.KeyF2:
			renameSelected()
			return nil
		case tcell.KeyEnter:
			if event.Modifiers()&tcell.ModCtrl != 0 {
				confirm()
				return nil
			}
			if activePane == 0 {
				index := groupedList.GetCurrentItem()
				if index < 0 || index >= len(groupedRows) {
					return event
				}
				ref := groupedRows[index]
				if !ref.IsGroup && ref.MemberID != "" {
					moveMemberOutOfGroup(ref.Group, ref.MemberID)
					return nil
				}
				return nil
			}
			if activePane == 1 {
				if member, ok := selectedUngroupedMember(); ok {
					showChooseGroupModal(member)
				}
				return nil
			}
		}
		if event.Key() == tcell.KeyCtrlN {
			createGroup()
			return nil
		}
		if shortcutMatchesEvent("Ctrl+L", event) {
			showAliasModal()
			return nil
		}
		return event
	})
	return modal
}

func RunFamilyBlastCustomizeModal(page FamilyBlastCustomizePage) (FamilyBlastResult, error) {
	app := newApp()
	var result FamilyBlastResult
	buildFamilyBlastCustomizeModal(page, app, &result)
	if err := runApp(app); err != nil {
		return FamilyBlastResult{}, err
	}
	return result, nil
}

func splitSidebarLines(value string) []string {
	lines := make([]string, 0, 4)
	for _, line := range strings.Split(strings.TrimSpace(value), "\n") {
		for _, part := range strings.Split(line, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				lines = append(lines, part)
			}
		}
	}
	return lines
}

func RunBlastFilterModal(page BlastFilterPage) (BlastFilterResult, error) {
	app := newApp()
	var result BlastFilterResult
	settings := normalizeTUIBlastFilterSettings(page.Settings)
	helpVisible := false
	var mainRoot tview.Primitive

	newInput := func(label string, text string, width int) *tview.InputField {
		input := tview.NewInputField().SetLabel(label + " ").SetText(text).SetFieldWidth(width)
		input.SetFieldTextColor(tview.Styles.PrimaryTextColor)
		input.SetLabelColor(tview.Styles.SecondaryTextColor)
		input.SetFieldBackgroundColor(colorPanel)
		return input
	}

	minIdentity := newInput("minimum identity (%)", settings.MinIdentityPercent, 6)
	minQueryCoverage := newInput("minimum query coverage (%)", settings.MinAlignQueryCoveragePercent, 6)
	maxEValue := newInput("maximum E-value", settings.MaxEValue, 10)
	minLengthRatio := newInput("min target/UniProt length (%)", settings.MinTargetCanonicalLengthPercent, 6)
	maxLengthRatio := newInput("max target/UniProt length (%)", settings.MaxTargetCanonicalLengthPercent, 6)
	minTargetQueryRatio := newInput("min target/query length (%)", settings.MinTargetQueryLengthPercent, 6)
	maxTargetQueryRatio := newInput("max target/query length (%)", settings.MaxTargetQueryLengthPercent, 6)
	minInterProCoverage := newInput("minimum InterPro coverage (%)", settings.MinInterProCoveragePercent, 6)
	strongFallbackMinIdentity := newInput("fallback minimum identity (%)", settings.StrongBlastFallbackMinIdentityPercent, 6)
	strongFallbackMaxEValue := newInput("fallback maximum E-value", settings.StrongBlastFallbackMaxEValue, 10)
	strongFallbackMinTargetQuery := newInput("fallback min target/query (%)", settings.StrongBlastFallbackMinTargetQueryPercent, 6)
	strongFallbackMaxTargetQuery := newInput("fallback max target/query (%)", settings.StrongBlastFallbackMaxTargetQueryPercent, 6)
	strongFallbackMinConsensusSupport := newInput("fallback family members >=", settings.StrongFallbackMinFamilyConsensusSupport, 6)
	strongFallbackMinConsensusPercent := newInput("fallback family support (%)", settings.StrongFallbackMinFamilyConsensusPercent, 6)
	familySemanticMinMatches := newInput("minimum token matches", settings.FamilySemanticMinTokenMatches, 6)
	familySemanticMinPercent := newInput("minimum agreement (%)", settings.FamilySemanticMinAgreementPercent, 6)
	topHitsPerQuery := newInput("rows to keep per query", settings.TopHitsPerQuery, 6)
	minSoftScore := newInput("minimum soft score", settings.MinSoftScore, 6)
	identityWeight := newInput("identity score weight", settings.IdentityWeight, 5)
	coverageWeight := newInput("query coverage score weight", settings.CoverageWeight, 5)
	lengthWeight := newInput("UniProt length score weight", settings.LengthRatioWeight, 5)
	targetQueryLengthWeight := newInput("target/query length score weight", settings.TargetQueryLengthWeight, 5)
	interProWeight := newInput("InterPro present score", settings.InterProWeight, 5)
	interProPartialWeight := newInput("InterPro partial score", settings.InterProPartialWeight, 5)
	interProCoverageWeight := newInput("InterPro coverage score", settings.InterProCoverageWeight, 5)
	reviewedWeight := newInput("reviewed UniProt score", settings.UniProtReviewedWeight, 5)
	annotationWeight := newInput("annotation richness score", settings.UniProtAnnotationWeight, 5)
	familySemanticWeight := newInput("semantic agreement score", settings.FamilySemanticAgreementWeight, 5)
	sequenceCautionPenalty := newInput("sequence caution penalty", settings.PenaltySequenceCaution, 5)
	fragmentPenalty := newInput("fragment record penalty", settings.PenaltyFragment, 5)
	presentReferenceScore := newInput("InterPro present rank score", settings.InterProPresentReferenceScore, 5)
	partialReferenceScore := newInput("InterPro partial rank score", settings.InterProPartialReferenceScore, 5)
	uncertainReferenceScore := newInput("InterPro uncertain rank score", settings.InterProUncertainReferenceScore, 5)
	missingReferencePenalty := newInput("InterPro missing rank penalty", settings.InterProMissingReferencePenalty, 5)
	interProCoverageReferenceDivisor := newInput("InterPro coverage score divisor", settings.InterProCoverageReferenceDivisor, 5)
	uniProtAccessionReferenceScore := newInput("UniProt accession rank score", settings.UniProtAccessionReferenceScore, 5)
	uniProtReviewedReferenceScore := newInput("reviewed UniProt rank score", settings.UniProtReviewedReferenceScore, 5)
	uniProtAnnotationReferenceScore := newInput("annotation rank score", settings.UniProtAnnotationReferenceScore, 5)
	familySemanticReferenceScore := newInput("semantic rank score", settings.FamilySemanticReferenceScore, 5)
	fragmentReferencePenaltyMultiplier := newInput("fragment rank penalty multiplier", settings.FragmentReferencePenaltyMultiplier, 5)
	cautionReferencePenaltyMultiplier := newInput("caution rank penalty multiplier", settings.SequenceCautionReferencePenaltyMultiplier, 5)
	lengthNearDistance := newInput("near length distance (%)", settings.LengthNearDistancePercent, 5)
	lengthNearScore := newInput("near length rank score", settings.LengthNearReferenceScore, 5)
	lengthAcceptableDistance := newInput("acceptable length distance (%)", settings.LengthAcceptableDistancePercent, 5)
	lengthAcceptableScore := newInput("acceptable length rank score", settings.LengthAcceptableReferenceScore, 5)
	lengthFarDistance := newInput("far length distance (%)", settings.LengthFarDistancePercent, 5)
	lengthFarPenalty := newInput("far length rank penalty", settings.LengthFarReferencePenalty, 5)

	identityBox := newCheckboxModule("Reject rows below the identity cutoff", func() bool {
		return strings.TrimSpace(minIdentity.GetText()) != "" && strings.TrimSpace(minIdentity.GetText()) != "0"
	}, func() {
		if strings.TrimSpace(minIdentity.GetText()) == "" || strings.TrimSpace(minIdentity.GetText()) == "0" {
			minIdentity.SetText("30")
		} else {
			minIdentity.SetText("0")
		}
	})
	identityBox.SetBorder(false)
	coverageBox := newCheckboxModule("Reject rows below the query-coverage cutoff", func() bool {
		return strings.TrimSpace(minQueryCoverage.GetText()) != "" && strings.TrimSpace(minQueryCoverage.GetText()) != "0"
	}, func() {
		if strings.TrimSpace(minQueryCoverage.GetText()) == "" || strings.TrimSpace(minQueryCoverage.GetText()) == "0" {
			minQueryCoverage.SetText("50")
		} else {
			minQueryCoverage.SetText("0")
		}
	})
	coverageBox.SetBorder(false)
	eValueBox := newCheckboxModule("Reject rows above the E-value cutoff", func() bool {
		return strings.TrimSpace(maxEValue.GetText()) != "" && strings.TrimSpace(maxEValue.GetText()) != "0"
	}, func() {
		if strings.TrimSpace(maxEValue.GetText()) == "" || strings.TrimSpace(maxEValue.GetText()) == "0" {
			maxEValue.SetText("1e-30")
		} else {
			maxEValue.SetText("0")
		}
	})
	eValueBox.SetBorder(false)
	lengthBox := newCheckboxModule("Check target length against UniProt canonical length", func() bool {
		return settings.UseTargetCanonicalLengthRatio
	}, func() {
		settings.UseTargetCanonicalLengthRatio = !settings.UseTargetCanonicalLengthRatio
	})
	targetQueryLengthBox := newCheckboxModule("Check target length against query length", func() bool {
		return settings.UseTargetQueryLengthRatio
	}, func() {
		settings.UseTargetQueryLengthRatio = !settings.UseTargetQueryLengthRatio
	})
	requireLengthRatio := newCheckboxModule("Require canonical-length data", func() bool { return settings.RequireTargetCanonicalLengthRatio }, func() { settings.RequireTargetCanonicalLengthRatio = !settings.RequireTargetCanonicalLengthRatio })
	requireTargetQueryRatio := newCheckboxModule("Reject rows missing target/query length data", func() bool { return settings.RequireTargetQueryLengthRatio }, func() { settings.RequireTargetQueryLengthRatio = !settings.RequireTargetQueryLengthRatio })
	requireUniProt := newCheckboxModule("Reject rows without a UniProt accession", func() bool { return settings.RequireUniProtAccession }, func() { settings.RequireUniProtAccession = !settings.RequireUniProtAccession })
	preferReviewed := newCheckboxModule("Prefer reviewed UniProt records", func() bool { return settings.PreferUniProtReviewed }, func() { settings.PreferUniProtReviewed = !settings.PreferUniProtReviewed })
	rejectFragments := newCheckboxModule("Reject UniProt fragment records", func() bool { return settings.RejectUniProtFragments }, func() { settings.RejectUniProtFragments = !settings.RejectUniProtFragments })
	rejectCautions := newCheckboxModule("Reject UniProt sequence cautions", func() bool { return settings.RejectUniProtSequenceCautions }, func() { settings.RejectUniProtSequenceCautions = !settings.RejectUniProtSequenceCautions })
	requireInterPro := newCheckboxModule("Require InterPro conserved-region support", func() bool { return settings.RequireInterProConservedRegion }, func() { settings.RequireInterProConservedRegion = !settings.RequireInterProConservedRegion })
	allowPartial := newCheckboxModule("Allow InterPro partial status", func() bool { return settings.AllowInterProPartial }, func() { settings.AllowInterProPartial = !settings.AllowInterProPartial })
	rejectMissing := newCheckboxModule("Reject InterPro status: missing", func() bool { return settings.RejectInterProMissing }, func() { settings.RejectInterProMissing = !settings.RejectInterProMissing })
	rejectUncertain := newCheckboxModule("Reject InterPro status: uncertain", func() bool { return settings.RejectInterProUncertain }, func() { settings.RejectInterProUncertain = !settings.RejectInterProUncertain })
	requireInterProCoverage := newCheckboxModule("Reject rows missing InterPro coverage when cutoff is used", func() bool { return settings.RequireInterProCoverageWhenUsed }, func() { settings.RequireInterProCoverageWhenUsed = !settings.RequireInterProCoverageWhenUsed })
	strongFallback := newCheckboxModule("Let very strong BLAST evidence rescue weak references", func() bool { return settings.AllowStrongBlastFallbackWithoutReferences }, func() {
		settings.AllowStrongBlastFallbackWithoutReferences = !settings.AllowStrongBlastFallbackWithoutReferences
	})
	requireFallbackConsensus := newCheckboxModule("Require family-member support for fallback rescue", func() bool { return settings.RequireFamilyConsensusForStrongFallback }, func() {
		settings.RequireFamilyConsensusForStrongFallback = !settings.RequireFamilyConsensusForStrongFallback
	})
	useFamilySemantic := newCheckboxModule("Compare query family name with annotations", func() bool { return settings.UseFamilySemanticAgreement }, func() {
		settings.UseFamilySemanticAgreement = !settings.UseFamilySemanticAgreement
	})
	requireFamilySemantic := newCheckboxModule("Reject rows with poor family-name agreement", func() bool { return settings.RequireFamilySemanticAgreement }, func() {
		settings.RequireFamilySemanticAgreement = !settings.RequireFamilySemanticAgreement
	})
	familySemanticBypass := newCheckboxModule("Let strong reference evidence override name mismatch", func() bool { return settings.FamilySemanticAllowStrongReferenceBypass }, func() {
		settings.FamilySemanticAllowStrongReferenceBypass = !settings.FamilySemanticAllowStrongReferenceBypass
	})
	interProAnyDomain := newCheckboxModule("InterPro rule: accept any domain evidence", func() bool { return strings.EqualFold(settings.InterProDomainMode, "any_domain") }, func() { settings.InterProDomainMode = "any_domain" })
	interProConservedDomain := newCheckboxModule("InterPro rule: use conserved-region status", func() bool { return strings.EqualFold(settings.InterProDomainMode, "conserved_region") }, func() { settings.InterProDomainMode = "conserved_region" })
	interProConsensusDomain := newCheckboxModule("InterPro rule: require family consensus domain", func() bool { return strings.EqualFold(settings.InterProDomainMode, "family_consensus_domain") }, func() { settings.InterProDomainMode = "family_consensus_domain" })
	interProOff := newCheckboxModule("InterPro rule: ignore InterPro domain evidence", func() bool { return strings.EqualFold(settings.InterProDomainMode, "off") }, func() { settings.InterProDomainMode = "off" })
	keepBestIsoform := newCheckboxModule("Keep only the best isoform per target gene", func() bool { return settings.KeepBestIsoformPerTargetGene }, func() { settings.KeepBestIsoformPerTargetGene = !settings.KeepBestIsoformPerTargetGene })
	keepTopHits := newCheckboxModule("Keep only the top-ranked rows per query", func() bool { return settings.KeepTopHitsPerQuery }, func() { settings.KeepTopHitsPerQuery = !settings.KeepTopHitsPerQuery })
	rankingOrder := newInput("order", settings.RankingTieBreakerOrder, 24)
	preferEValue := newCheckboxModule("Tie break: lower E-value wins", func() bool { return settings.PreferLowerEValueWhenTies }, func() { settings.PreferLowerEValueWhenTies = !settings.PreferLowerEValueWhenTies })
	preferIdentity := newCheckboxModule("Tie break: higher identity wins", func() bool { return settings.PreferHigherIdentityWhenTies }, func() { settings.PreferHigherIdentityWhenTies = !settings.PreferHigherIdentityWhenTies })
	preferCoverage := newCheckboxModule("Tie break: higher query coverage wins", func() bool { return settings.PreferHigherCoverageWhenTies }, func() { settings.PreferHigherCoverageWhenTies = !settings.PreferHigherCoverageWhenTies })
	preferReference := newCheckboxModule("Tie break: stronger reference evidence wins", func() bool { return settings.PreferHigherReferenceScoreWhenTies }, func() { settings.PreferHigherReferenceScoreWhenTies = !settings.PreferHigherReferenceScoreWhenTies })
	preferFilterScore := newCheckboxModule("Ranking: use the soft filter score first", func() bool { return settings.PreferHigherFilterScoreWhenRanking }, func() { settings.PreferHigherFilterScoreWhenRanking = !settings.PreferHigherFilterScoreWhenRanking })
	preferBitscore := newCheckboxModule("Tie break: higher BLAST bitscore wins", func() bool { return settings.PreferHigherBitscoreWhenTies }, func() { settings.PreferHigherBitscoreWhenTies = !settings.PreferHigherBitscoreWhenTies })
	hardFail := newCheckboxModule("Reject when any enabled hard rule fails", func() bool { return settings.RejectIfAnyHardRuleFails }, func() { settings.RejectIfAnyHardRuleFails = !settings.RejectIfAnyHardRuleFails })
	enableSoftScore := newCheckboxModule("Enable soft evidence score", func() bool { return settings.EnableSoftScore }, func() { settings.EnableSoftScore = !settings.EnableSoftScore })

	for _, box := range []*checkboxModule{identityBox, coverageBox, eValueBox, lengthBox, targetQueryLengthBox, requireLengthRatio, requireTargetQueryRatio, requireUniProt, preferReviewed, rejectFragments, rejectCautions, requireInterPro, allowPartial, rejectMissing, rejectUncertain, requireInterProCoverage, strongFallback, requireFallbackConsensus, useFamilySemantic, requireFamilySemantic, familySemanticBypass, interProAnyDomain, interProConservedDomain, interProConsensusDomain, interProOff, keepBestIsoform, keepTopHits, preferEValue, preferIdentity, preferCoverage, preferReference, preferFilterScore, preferBitscore, hardFail, enableSoftScore} {
		box.SetBorder(false)
	}

	module := newButtonFlex()
	module.SetBorder(true)
	module.SetTitle(" BLAST filter parameters ")
	module.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(module.Box, true)
	attachFocusBorder(module.Box)
	if strings.TrimSpace(page.Message) != "" {
		module.AddItem(textBlock(page.Message), 3, 0, false)
	}
	filterControlHeight := func(primitive tview.Primitive) int {
		if box, ok := primitive.(*checkboxModule); ok {
			if len([]rune(strings.TrimSpace(box.label))) > 34 {
				return 2
			}
		}
		return 1
	}
	thresholdModule := newButtonFlex()
	thresholdModule.SetBorder(true)
	thresholdModule.SetTitle(" Alignment and length rules ")
	thresholdModule.SetTitleAlign(tview.AlignCenter)
	thresholdModule.AddItem(textBlock("Use this page for hard pass/fail rules based on BLAST strength and protein length. Identity, query coverage, and E-value stay off by default; length checks are the conservative default guards."), 4, 0, false)
	thresholdModule.AddItem(sectionHeader("Optional BLAST-strength cutoffs"), 1, 0, false)
	for _, primitive := range []tview.Primitive{identityBox, minIdentity, coverageBox, minQueryCoverage, eValueBox, maxEValue} {
		thresholdModule.AddItem(primitive, filterControlHeight(primitive), 0, primitive == minIdentity)
	}
	thresholdModule.AddItem(sectionHeader("Target length compared with query"), 1, 0, false)
	thresholdModule.AddItem(targetQueryLengthBox, 1, 0, false)
	thresholdModule.AddItem(requireTargetQueryRatio, 1, 0, false)
	thresholdModule.AddItem(minTargetQueryRatio, 1, 0, true)
	thresholdModule.AddItem(maxTargetQueryRatio, 1, 0, true)
	thresholdModule.AddItem(sectionHeader("Target length compared with UniProt"), 1, 0, false)
	thresholdModule.AddItem(lengthBox, 1, 0, false)
	thresholdModule.AddItem(requireLengthRatio, 1, 0, false)
	thresholdModule.AddItem(minLengthRatio, 1, 0, true)
	thresholdModule.AddItem(maxLengthRatio, 1, 0, true)

	semanticModule := newButtonFlex()
	semanticModule.SetBorder(true)
	semanticModule.SetTitle(" Family-name agreement ")
	semanticModule.SetTitleAlign(tview.AlignCenter)
	semanticModule.AddItem(textBlock("Compare the query/family name with UniProt and InterPro annotation text. This helps catch plausible-looking hits from a neighboring but different family."), 4, 0, false)
	semanticModule.AddItem(sectionHeader("Name-agreement behavior"), 1, 0, false)
	for _, primitive := range []tview.Primitive{useFamilySemantic, requireFamilySemantic, familySemanticBypass} {
		semanticModule.AddItem(primitive, filterControlHeight(primitive), 0, false)
	}
	semanticModule.AddItem(sectionHeader("Minimum agreement"), 1, 0, false)
	for _, primitive := range []tview.Primitive{familySemanticMinMatches, familySemanticMinPercent} {
		semanticModule.AddItem(primitive, 1, 0, primitive == familySemanticMinMatches)
	}

	referenceModule := newButtonFlex()
	referenceModule.SetBorder(true)
	referenceModule.SetTitle(" External-reference rules ")
	referenceModule.SetTitleAlign(tview.AlignCenter)
	referenceModule.AddItem(sectionHeader("UniProt evidence"), 1, 0, false)
	for _, primitive := range []tview.Primitive{requireUniProt, preferReviewed, rejectFragments, rejectCautions} {
		referenceModule.AddItem(primitive, filterControlHeight(primitive), 0, false)
	}
	referenceModule.AddItem(sectionHeader("How to use InterPro"), 1, 0, false)
	for _, primitive := range []tview.Primitive{interProConservedDomain, interProAnyDomain, interProConsensusDomain, interProOff} {
		referenceModule.AddItem(primitive, filterControlHeight(primitive), 0, false)
	}
	referenceModule.AddItem(sectionHeader("InterPro status handling"), 1, 0, false)
	for _, primitive := range []tview.Primitive{requireInterPro, allowPartial, rejectMissing, rejectUncertain, requireInterProCoverage, minInterProCoverage} {
		referenceModule.AddItem(primitive, filterControlHeight(primitive), 0, primitive == minInterProCoverage)
	}
	referenceModule.AddItem(sectionHeader("Fallback rescue for very strong BLAST hits"), 1, 0, false)
	for _, primitive := range []tview.Primitive{strongFallback, strongFallbackMinIdentity, strongFallbackMaxEValue, strongFallbackMinTargetQuery, strongFallbackMaxTargetQuery, requireFallbackConsensus, strongFallbackMinConsensusSupport, strongFallbackMinConsensusPercent} {
		referenceModule.AddItem(primitive, filterControlHeight(primitive), 0, false)
	}

	rankingModule := newButtonFlex()
	rankingModule.SetBorder(true)
	rankingModule.SetTitle(" Keep and rank rows ")
	rankingModule.SetTitleAlign(tview.AlignCenter)
	rankingModule.AddItem(textBlock("Control how many rows stay selected, then choose the ranking order used when several hits look similar."), 2, 0, false)
	rankingModule.AddItem(sectionHeader("Rows kept after ranking"), 1, 0, false)
	for _, primitive := range []tview.Primitive{keepBestIsoform, keepTopHits, topHitsPerQuery} {
		rankingModule.AddItem(primitive, filterControlHeight(primitive), 0, primitive == topHitsPerQuery)
	}
	rankingModule.AddItem(sectionHeader("Priority list and tie breaks"), 1, 0, false)
	rankingModule.AddItem(textBlock("Ranking priority list. Edit comma-separated names in order."), 2, 0, false)
	for _, primitive := range []tview.Primitive{rankingOrder, preferFilterScore, preferEValue, preferIdentity, preferCoverage, preferReference, preferBitscore} {
		rankingModule.AddItem(primitive, filterControlHeight(primitive), 0, primitive == rankingOrder)
	}
	rankingModule.AddItem(sectionHeader("Final pass/fail score"), 1, 0, false)
	for _, primitive := range []tview.Primitive{hardFail, enableSoftScore, minSoftScore} {
		rankingModule.AddItem(primitive, filterControlHeight(primitive), 0, primitive == minSoftScore)
	}

	softScoreModule := newButtonFlex()
	softScoreModule.SetBorder(true)
	softScoreModule.SetTitle(" Soft evidence score ")
	softScoreModule.SetTitleAlign(tview.AlignCenter)
	referenceScoreModule := newButtonFlex()
	referenceScoreModule.SetBorder(true)
	referenceScoreModule.SetTitle(" Reference ranking score ")
	referenceScoreModule.SetTitleAlign(tview.AlignCenter)
	softScoreModule.AddItem(textBlock("Weights used by the optional soft score. Higher values make that evidence count more; penalties subtract from weak or risky rows."), 3, 0, false)
	softScoreModule.AddItem(sectionHeader("BLAST and length evidence"), 1, 0, false)
	for _, primitive := range []tview.Primitive{identityWeight, coverageWeight, lengthWeight, targetQueryLengthWeight} {
		softScoreModule.AddItem(primitive, 1, 0, false)
	}
	softScoreModule.AddItem(sectionHeader("Reference and name evidence"), 1, 0, false)
	for _, primitive := range []tview.Primitive{interProWeight, interProPartialWeight, interProCoverageWeight, reviewedWeight, annotationWeight, familySemanticWeight} {
		softScoreModule.AddItem(primitive, 1, 0, false)
	}
	softScoreModule.AddItem(sectionHeader("Penalties"), 1, 0, false)
	for _, primitive := range []tview.Primitive{sequenceCautionPenalty, fragmentPenalty} {
		softScoreModule.AddItem(primitive, 1, 0, false)
	}
	referenceScoreModule.AddItem(textBlock("Scores used to rank external-reference evidence before the best rows are kept."), 2, 0, false)
	referenceScoreModule.AddItem(sectionHeader("InterPro evidence"), 1, 0, false)
	for _, primitive := range []tview.Primitive{presentReferenceScore, partialReferenceScore, uncertainReferenceScore, missingReferencePenalty, interProCoverageReferenceDivisor} {
		referenceScoreModule.AddItem(primitive, 1, 0, false)
	}
	referenceScoreModule.AddItem(sectionHeader("UniProt and family-name evidence"), 1, 0, false)
	for _, primitive := range []tview.Primitive{uniProtAccessionReferenceScore, uniProtReviewedReferenceScore, uniProtAnnotationReferenceScore, familySemanticReferenceScore} {
		referenceScoreModule.AddItem(primitive, 1, 0, false)
	}
	referenceScoreModule.AddItem(sectionHeader("Fragment and caution penalties"), 1, 0, false)
	for _, primitive := range []tview.Primitive{fragmentReferencePenaltyMultiplier, cautionReferencePenaltyMultiplier} {
		referenceScoreModule.AddItem(primitive, 1, 0, false)
	}
	referenceScoreModule.AddItem(sectionHeader("Length-distance bands"), 1, 0, false)
	for _, primitive := range []tview.Primitive{lengthNearDistance, lengthNearScore, lengthAcceptableDistance, lengthAcceptableScore, lengthFarDistance, lengthFarPenalty} {
		referenceScoreModule.AddItem(primitive, 1, 0, false)
	}

	modules := []struct {
		page     int
		box      *buttonFlex
		controls []tview.Primitive
	}{
		{page: 0, box: thresholdModule, controls: []tview.Primitive{identityBox, minIdentity, coverageBox, minQueryCoverage, eValueBox, maxEValue, targetQueryLengthBox, requireTargetQueryRatio, minTargetQueryRatio, maxTargetQueryRatio, lengthBox, requireLengthRatio, minLengthRatio, maxLengthRatio}},
		{page: 0, box: referenceModule, controls: []tview.Primitive{requireUniProt, preferReviewed, rejectFragments, rejectCautions, interProAnyDomain, interProConservedDomain, interProConsensusDomain, interProOff, requireInterPro, allowPartial, rejectMissing, rejectUncertain, requireInterProCoverage, minInterProCoverage, strongFallback, requireFallbackConsensus, strongFallbackMinIdentity, strongFallbackMaxEValue, strongFallbackMinTargetQuery, strongFallbackMaxTargetQuery, strongFallbackMinConsensusSupport, strongFallbackMinConsensusPercent}},
		{page: 0, box: semanticModule, controls: []tview.Primitive{useFamilySemantic, requireFamilySemantic, familySemanticBypass, familySemanticMinMatches, familySemanticMinPercent}},
		{page: 1, box: rankingModule, controls: []tview.Primitive{keepBestIsoform, keepTopHits, topHitsPerQuery, rankingOrder, preferFilterScore, preferEValue, preferIdentity, preferCoverage, preferReference, preferBitscore, hardFail, enableSoftScore, minSoftScore}},
		{page: 1, box: softScoreModule, controls: []tview.Primitive{identityWeight, coverageWeight, lengthWeight, targetQueryLengthWeight, interProWeight, interProPartialWeight, interProCoverageWeight, reviewedWeight, annotationWeight, familySemanticWeight, sequenceCautionPenalty, fragmentPenalty}},
		{page: 1, box: referenceScoreModule, controls: []tview.Primitive{presentReferenceScore, partialReferenceScore, uncertainReferenceScore, missingReferencePenalty, interProCoverageReferenceDivisor, uniProtAccessionReferenceScore, uniProtReviewedReferenceScore, uniProtAnnotationReferenceScore, familySemanticReferenceScore, fragmentReferencePenaltyMultiplier, cautionReferencePenaltyMultiplier, lengthNearDistance, lengthNearScore, lengthAcceptableDistance, lengthAcceptableScore, lengthFarDistance, lengthFarPenalty}},
	}
	pageSelector := &pageSelectorPrimitive{Box: tview.NewBox(), totalPages: 2, currentPage: 0}

	pageOne := tview.NewFlex().SetDirection(tview.FlexColumn)
	pageOne.AddItem(thresholdModule, 0, 1, true)
	pageOne.AddItem(referenceModule, 0, 1, false)
	pageOne.AddItem(semanticModule, 0, 1, false)

	pageTwo := tview.NewFlex().SetDirection(tview.FlexColumn)
	pageTwo.AddItem(rankingModule, 0, 1, true)
	pageTwo.AddItem(softScoreModule, 0, 1, false)
	pageTwo.AddItem(referenceScoreModule, 0, 1, false)

	pageContainer := tview.NewPages()
	pageContainer.AddPage("page-0", pageOne, true, true)
	pageContainer.AddPage("page-1", pageTwo, true, false)
	module.AddItem(pageSelector, 3, 0, false)
	module.AddItem(pageContainer, 0, 1, true)

	moduleIndex := 0
	controlIndexes := make([]int, len(modules))
	controlIndexes[0] = 1
	activePage := 0
	setActivePage := func(pageIndex int) {
		if pageIndex < 0 {
			pageIndex = 1
		}
		if pageIndex > 1 {
			pageIndex = 0
		}
		activePage = pageIndex
		pageContainer.SwitchToPage(fmt.Sprintf("page-%d", pageIndex))
		pageSelector.currentPage = pageIndex
		if pageIndex == 0 {
			pageSelector.summary = "Settings page 1/2 | Rules and evidence"
		} else {
			pageSelector.summary = "Settings page 2/2 | Ranking and scores"
		}
	}
	currentControls := func() []tview.Primitive {
		if moduleIndex < 0 || moduleIndex >= len(modules) {
			return nil
		}
		return modules[moduleIndex].controls
	}
	setModuleFocus := func(index int) {
		if index < 0 {
			index = len(modules) - 1
		}
		if index >= len(modules) {
			index = 0
		}
		moduleIndex = index
		controls := currentControls()
		if len(controls) == 0 {
			return
		}
		if controlIndexes[moduleIndex] < 0 {
			controlIndexes[moduleIndex] = len(controls) - 1
		}
		if controlIndexes[moduleIndex] >= len(controls) {
			controlIndexes[moduleIndex] = 0
		}
		setActivePage(modules[moduleIndex].page)
		for i := range modules {
			setFocusBorder(modules[i].box.Box, i == moduleIndex)
			for _, primitive := range modules[i].controls {
				if box := primitiveBox(primitive); box != nil {
					setFocusBorder(box, false)
				}
			}
		}
		if box := primitiveBox(controls[controlIndexes[moduleIndex]]); box != nil {
			setFocusBorder(box, true)
		}
		app.SetFocus(controls[controlIndexes[moduleIndex]])
	}
	setControlFocus := func(index int) {
		controls := currentControls()
		if len(controls) == 0 {
			return
		}
		if index < 0 {
			index = len(controls) - 1
		}
		if index >= len(controls) {
			index = 0
		}
		controlIndexes[moduleIndex] = index
		setModuleFocus(moduleIndex)
	}
	pageSelector.onSelect = func(pageIndex int) {
		for i := range modules {
			if modules[i].page == pageIndex {
				setModuleFocus(i)
				return
			}
		}
		setActivePage(pageIndex)
	}
	focusedInput := func() *tview.InputField {
		controls := currentControls()
		if len(controls) == 0 {
			return nil
		}
		input, _ := controls[controlIndexes[moduleIndex]].(*tview.InputField)
		return input
	}
	toggleFocused := func() bool {
		controls := currentControls()
		if len(controls) == 0 {
			return false
		}
		if box, ok := controls[controlIndexes[moduleIndex]].(*checkboxModule); ok {
			box.toggleChecked()
			return true
		}
		return false
	}
	closeWithNav := func(nav NavAction) {
		result.Nav = nav
		app.Stop()
	}
	confirm := func() {
		result.Settings = BlastFilterSettings{
			MinIdentityPercent:                        minIdentity.GetText(),
			MinAlignQueryCoveragePercent:              minQueryCoverage.GetText(),
			MaxEValue:                                 maxEValue.GetText(),
			UseTargetCanonicalLengthRatio:             settings.UseTargetCanonicalLengthRatio,
			RequireTargetCanonicalLengthRatio:         settings.RequireTargetCanonicalLengthRatio,
			MinTargetCanonicalLengthPercent:           minLengthRatio.GetText(),
			MaxTargetCanonicalLengthPercent:           maxLengthRatio.GetText(),
			UseTargetQueryLengthRatio:                 settings.UseTargetQueryLengthRatio,
			RequireTargetQueryLengthRatio:             settings.RequireTargetQueryLengthRatio,
			MinTargetQueryLengthPercent:               minTargetQueryRatio.GetText(),
			MaxTargetQueryLengthPercent:               maxTargetQueryRatio.GetText(),
			RequireUniProtAccession:                   settings.RequireUniProtAccession,
			PreferUniProtReviewed:                     settings.PreferUniProtReviewed,
			RejectUniProtFragments:                    settings.RejectUniProtFragments,
			RejectUniProtSequenceCautions:             settings.RejectUniProtSequenceCautions,
			InterProDomainMode:                        settings.InterProDomainMode,
			RequireInterProConservedRegion:            settings.RequireInterProConservedRegion,
			AllowInterProPartial:                      settings.AllowInterProPartial,
			RejectInterProMissing:                     settings.RejectInterProMissing,
			RejectInterProUncertain:                   settings.RejectInterProUncertain,
			RequireInterProCoverageWhenUsed:           settings.RequireInterProCoverageWhenUsed,
			MinInterProCoveragePercent:                minInterProCoverage.GetText(),
			AllowStrongBlastFallbackWithoutReferences: settings.AllowStrongBlastFallbackWithoutReferences,
			StrongBlastFallbackMinIdentityPercent:     strongFallbackMinIdentity.GetText(),
			StrongBlastFallbackMaxEValue:              strongFallbackMaxEValue.GetText(),
			StrongBlastFallbackMinTargetQueryPercent:  strongFallbackMinTargetQuery.GetText(),
			StrongBlastFallbackMaxTargetQueryPercent:  strongFallbackMaxTargetQuery.GetText(),
			RequireFamilyConsensusForStrongFallback:   settings.RequireFamilyConsensusForStrongFallback,
			StrongFallbackMinFamilyConsensusSupport:   strongFallbackMinConsensusSupport.GetText(),
			StrongFallbackMinFamilyConsensusPercent:   strongFallbackMinConsensusPercent.GetText(),
			UseFamilySemanticAgreement:                settings.UseFamilySemanticAgreement,
			RequireFamilySemanticAgreement:            settings.RequireFamilySemanticAgreement,
			FamilySemanticMinTokenMatches:             familySemanticMinMatches.GetText(),
			FamilySemanticMinAgreementPercent:         familySemanticMinPercent.GetText(),
			FamilySemanticAllowStrongReferenceBypass:  settings.FamilySemanticAllowStrongReferenceBypass,
			KeepBestIsoformPerTargetGene:              settings.KeepBestIsoformPerTargetGene,
			KeepTopHitsPerQuery:                       settings.KeepTopHitsPerQuery,
			TopHitsPerQuery:                           topHitsPerQuery.GetText(),
			RankingTieBreakerOrder:                    rankingOrder.GetText(),
			PreferHigherFilterScoreWhenRanking:        settings.PreferHigherFilterScoreWhenRanking,
			PreferLowerEValueWhenTies:                 settings.PreferLowerEValueWhenTies,
			PreferHigherIdentityWhenTies:              settings.PreferHigherIdentityWhenTies,
			PreferHigherCoverageWhenTies:              settings.PreferHigherCoverageWhenTies,
			PreferHigherReferenceScoreWhenTies:        settings.PreferHigherReferenceScoreWhenTies,
			PreferHigherBitscoreWhenTies:              settings.PreferHigherBitscoreWhenTies,
			RejectIfAnyHardRuleFails:                  settings.RejectIfAnyHardRuleFails,
			EnableSoftScore:                           settings.EnableSoftScore,
			MinSoftScore:                              minSoftScore.GetText(),
			IdentityWeight:                            identityWeight.GetText(),
			CoverageWeight:                            coverageWeight.GetText(),
			LengthRatioWeight:                         lengthWeight.GetText(),
			TargetQueryLengthWeight:                   targetQueryLengthWeight.GetText(),
			InterProWeight:                            interProWeight.GetText(),
			InterProPartialWeight:                     interProPartialWeight.GetText(),
			InterProCoverageWeight:                    interProCoverageWeight.GetText(),
			UniProtReviewedWeight:                     reviewedWeight.GetText(),
			UniProtAnnotationWeight:                   annotationWeight.GetText(),
			FamilySemanticAgreementWeight:             familySemanticWeight.GetText(),
			PenaltySequenceCaution:                    sequenceCautionPenalty.GetText(),
			PenaltyFragment:                           fragmentPenalty.GetText(),
			InterProPresentReferenceScore:             presentReferenceScore.GetText(),
			InterProPartialReferenceScore:             partialReferenceScore.GetText(),
			InterProUncertainReferenceScore:           uncertainReferenceScore.GetText(),
			InterProMissingReferencePenalty:           missingReferencePenalty.GetText(),
			InterProCoverageReferenceDivisor:          interProCoverageReferenceDivisor.GetText(),
			UniProtAccessionReferenceScore:            uniProtAccessionReferenceScore.GetText(),
			UniProtReviewedReferenceScore:             uniProtReviewedReferenceScore.GetText(),
			UniProtAnnotationReferenceScore:           uniProtAnnotationReferenceScore.GetText(),
			FamilySemanticReferenceScore:              familySemanticReferenceScore.GetText(),
			FragmentReferencePenaltyMultiplier:        fragmentReferencePenaltyMultiplier.GetText(),
			SequenceCautionReferencePenaltyMultiplier: cautionReferencePenaltyMultiplier.GetText(),
			LengthNearDistancePercent:                 lengthNearDistance.GetText(),
			LengthNearReferenceScore:                  lengthNearScore.GetText(),
			LengthAcceptableDistancePercent:           lengthAcceptableDistance.GetText(),
			LengthAcceptableReferenceScore:            lengthAcceptableScore.GetText(),
			LengthFarDistancePercent:                  lengthFarDistance.GetText(),
			LengthFarReferencePenalty:                 lengthFarPenalty.GetText(),
		}
		app.Stop()
	}
	clearFilter := func() {
		result.ClearFilter = true
		app.Stop()
	}

	var closeHelp func()
	var helpModal *localizedHelpModal
	showHelp := func() {
		helpVisible = true
		if helpModal == nil {
			helpModal = newLocalizedHelpModal(app, blastFilterHelpPages(), func() {
				if closeHelp != nil {
					closeHelp()
				}
			})
		}
		helpModal.SetLanguage(app, int(helpLanguageIndex.Load()))
		app.SetRoot(infoModalRoot(modalFramePage(page.Breadcrumb, page.Path, helpModal.Title()), helpModal.Body(), 118, 40), true)
		app.SetFocus(helpModal.TextView())
	}
	closeHelp = func() {
		helpVisible = false
		app.SetRoot(mainRoot, true)
		setModuleFocus(moduleIndex)
	}

	addButtonRow(module, modalButtons([]buttonSpec{
		{Label: ButtonBack, Shortcut: ShortcutBack, Action: func() { closeWithNav(NavBack) }, Visible: page.AllowBack},
		{Label: ButtonClearFilter, Shortcut: ShortcutClearFilter, Action: clearFilter, Visible: true},
		{Label: ButtonHelp, Shortcut: ShortcutHelp, Action: showHelp, Visible: true},
	}, true, firstNonEmptyText(page.ConfirmText, ButtonFilter), ShortcutConfirm, closeWithNav, confirm))
	addHints(module, []string{"Tab switches sections. Up/Down moves within the active section. Space toggles a checkbox. Enter advances and applies from the final page. Ctrl+L clears filter marks and reselects all rows. F1 opens help."})

	filterContentRows := 3 + maxInt(31, 46)
	if strings.TrimSpace(page.Message) != "" {
		filterContentRows += 3
	}
	filterContentRows += 2
	mainRoot = infoModalRoot(modalFramePage(page.Breadcrumb, page.Path, page.Title), module, 148, modalHeightForContent(filterContentRows, 50, 58))
	app.SetRoot(mainRoot, true)
	setActivePage(activePage)
	setModuleFocus(moduleIndex)
	installInputCapture(app, func(event *tcell.EventKey) *tcell.EventKey {
		if helpVisible {
			_ = helpModal.HandleKey(app, event, closeHelp)
			return nil
		}
		if shortcutMatchesEvent(ShortcutHelp, event) {
			showHelp()
			return nil
		}
		if shortcutMatchesEvent(ShortcutClearFilter, event) {
			clearFilter()
			return nil
		}
		if input := focusedInput(); input != nil && inputFieldEditKey(event) {
			deliverInputFieldKey(input, event, app)
			return nil
		}
		switch event.Key() {
		case tcell.KeyEscape:
			if page.AllowBack {
				closeWithNav(NavBack)
				return nil
			}
		case tcell.KeyUp:
			setControlFocus(controlIndexes[moduleIndex] - 1)
			return nil
		case tcell.KeyDown:
			setControlFocus(controlIndexes[moduleIndex] + 1)
			return nil
		case tcell.KeyTab:
			setModuleFocus(moduleIndex + 1)
			return nil
		case tcell.KeyBacktab:
			setModuleFocus(moduleIndex - 1)
			return nil
		case tcell.KeyEnter:
			if moduleIndex >= len(modules)-1 {
				confirm()
			} else {
				setModuleFocus(moduleIndex + 1)
			}
			return nil
		case tcell.KeyRune:
			if event.Rune() == ' ' && toggleFocused() {
				return nil
			}
		}
		return event
	})
	if err := runApp(app); err != nil {
		return BlastFilterResult{}, err
	}
	return result, nil
}

func normalizeTUIInterProSettings(settings InterProConservedRegionSettings) InterProConservedRegionSettings {
	if !settings.UsePfamAccession && !settings.UseInterProAccession && !settings.UseSignatureAccession && !settings.UseEntryType && !settings.UseEntryName && !settings.UseCoverage && !settings.UseMatchRegions {
		settings.UsePfamAccession = true
		settings.UseInterProAccession = true
		settings.UseSignatureAccession = true
		settings.UseEntryType = true
		settings.UseCoverage = true
		settings.UseMatchRegions = true
	}
	if strings.TrimSpace(settings.PresentMinCoverage) == "" {
		settings.PresentMinCoverage = "70"
	}
	if strings.TrimSpace(settings.PartialMinCoverage) == "" {
		settings.PartialMinCoverage = "25"
	}
	if strings.TrimSpace(settings.PresentMinMatchedItems) == "" {
		settings.PresentMinMatchedItems = "1"
	}
	if strings.TrimSpace(settings.PartialMinMatchedItems) == "" {
		settings.PartialMinMatchedItems = "1"
	}
	return settings
}

func normalizeTUIBlastFilterSettings(settings BlastFilterSettings) BlastFilterSettings {
	if strings.TrimSpace(settings.MinIdentityPercent) == "" {
		settings.MinIdentityPercent = "0"
	}
	if strings.TrimSpace(settings.MinAlignQueryCoveragePercent) == "" {
		settings.MinAlignQueryCoveragePercent = "0"
	}
	if strings.TrimSpace(settings.MaxEValue) == "" {
		settings.MaxEValue = "0"
	}
	if !settings.UseTargetCanonicalLengthRatio {
		settings.RequireTargetCanonicalLengthRatio = false
	}
	if strings.TrimSpace(settings.MinTargetCanonicalLengthPercent) == "" {
		settings.MinTargetCanonicalLengthPercent = "70"
	}
	if strings.TrimSpace(settings.MaxTargetCanonicalLengthPercent) == "" {
		settings.MaxTargetCanonicalLengthPercent = "130"
	}
	if !settings.UseTargetQueryLengthRatio {
		settings.RequireTargetQueryLengthRatio = false
	}
	if strings.TrimSpace(settings.MinTargetQueryLengthPercent) == "" {
		settings.MinTargetQueryLengthPercent = "60"
	}
	if strings.TrimSpace(settings.MaxTargetQueryLengthPercent) == "" {
		settings.MaxTargetQueryLengthPercent = "160"
	}
	if strings.TrimSpace(settings.InterProDomainMode) == "" {
		settings.InterProDomainMode = "conserved_region"
	}
	if strings.TrimSpace(settings.MinInterProCoveragePercent) == "" {
		settings.MinInterProCoveragePercent = "0"
	}
	if strings.TrimSpace(settings.StrongBlastFallbackMinIdentityPercent) == "" {
		settings.StrongBlastFallbackMinIdentityPercent = "40"
	}
	if strings.TrimSpace(settings.StrongBlastFallbackMaxEValue) == "" {
		settings.StrongBlastFallbackMaxEValue = "1e-80"
	}
	if strings.TrimSpace(settings.StrongBlastFallbackMinTargetQueryPercent) == "" {
		settings.StrongBlastFallbackMinTargetQueryPercent = "80"
	}
	if strings.TrimSpace(settings.StrongBlastFallbackMaxTargetQueryPercent) == "" {
		settings.StrongBlastFallbackMaxTargetQueryPercent = "120"
	}
	if strings.TrimSpace(settings.StrongFallbackMinFamilyConsensusSupport) == "" {
		settings.StrongFallbackMinFamilyConsensusSupport = "2"
	}
	if strings.TrimSpace(settings.StrongFallbackMinFamilyConsensusPercent) == "" {
		settings.StrongFallbackMinFamilyConsensusPercent = "35"
	}
	if strings.TrimSpace(settings.FamilySemanticMinTokenMatches) == "" {
		settings.FamilySemanticMinTokenMatches = "1"
	}
	if strings.TrimSpace(settings.FamilySemanticMinAgreementPercent) == "" {
		settings.FamilySemanticMinAgreementPercent = "20"
	}
	if strings.TrimSpace(settings.RankingTieBreakerOrder) == "" {
		settings.RankingTieBreakerOrder = "score,identity,coverage,reference,evalue,bitscore"
	}
	if strings.TrimSpace(settings.TopHitsPerQuery) == "" {
		settings.TopHitsPerQuery = "10"
	}
	if strings.TrimSpace(settings.MinSoftScore) == "" {
		settings.MinSoftScore = "5"
	}
	if strings.TrimSpace(settings.IdentityWeight) == "" {
		settings.IdentityWeight = "2"
	}
	if strings.TrimSpace(settings.CoverageWeight) == "" {
		settings.CoverageWeight = "2"
	}
	if strings.TrimSpace(settings.LengthRatioWeight) == "" {
		settings.LengthRatioWeight = "2"
	}
	if strings.TrimSpace(settings.TargetQueryLengthWeight) == "" {
		settings.TargetQueryLengthWeight = "2"
	}
	if strings.TrimSpace(settings.InterProWeight) == "" {
		settings.InterProWeight = "3"
	}
	if strings.TrimSpace(settings.InterProPartialWeight) == "" {
		settings.InterProPartialWeight = "1"
	}
	if strings.TrimSpace(settings.InterProCoverageWeight) == "" {
		settings.InterProCoverageWeight = "1"
	}
	if strings.TrimSpace(settings.UniProtReviewedWeight) == "" {
		settings.UniProtReviewedWeight = "1"
	}
	if strings.TrimSpace(settings.UniProtAnnotationWeight) == "" {
		settings.UniProtAnnotationWeight = "1"
	}
	if strings.TrimSpace(settings.FamilySemanticAgreementWeight) == "" {
		settings.FamilySemanticAgreementWeight = "2"
	}
	if strings.TrimSpace(settings.PenaltySequenceCaution) == "" {
		settings.PenaltySequenceCaution = "2"
	}
	if strings.TrimSpace(settings.PenaltyFragment) == "" {
		settings.PenaltyFragment = "3"
	}
	if strings.TrimSpace(settings.InterProPresentReferenceScore) == "" {
		settings.InterProPresentReferenceScore = "80"
	}
	if strings.TrimSpace(settings.InterProPartialReferenceScore) == "" {
		settings.InterProPartialReferenceScore = "35"
	}
	if strings.TrimSpace(settings.InterProUncertainReferenceScore) == "" {
		settings.InterProUncertainReferenceScore = "5"
	}
	if strings.TrimSpace(settings.InterProMissingReferencePenalty) == "" {
		settings.InterProMissingReferencePenalty = "80"
	}
	if strings.TrimSpace(settings.InterProCoverageReferenceDivisor) == "" {
		settings.InterProCoverageReferenceDivisor = "10"
	}
	if strings.TrimSpace(settings.UniProtAccessionReferenceScore) == "" {
		settings.UniProtAccessionReferenceScore = "25"
	}
	if strings.TrimSpace(settings.UniProtReviewedReferenceScore) == "" {
		settings.UniProtReviewedReferenceScore = "25"
	}
	if strings.TrimSpace(settings.UniProtAnnotationReferenceScore) == "" {
		settings.UniProtAnnotationReferenceScore = "10"
	}
	if strings.TrimSpace(settings.FamilySemanticReferenceScore) == "" {
		settings.FamilySemanticReferenceScore = "20"
	}
	if strings.TrimSpace(settings.FragmentReferencePenaltyMultiplier) == "" {
		settings.FragmentReferencePenaltyMultiplier = "10"
	}
	if strings.TrimSpace(settings.SequenceCautionReferencePenaltyMultiplier) == "" {
		settings.SequenceCautionReferencePenaltyMultiplier = "5"
	}
	if strings.TrimSpace(settings.LengthNearDistancePercent) == "" {
		settings.LengthNearDistancePercent = "10"
	}
	if strings.TrimSpace(settings.LengthNearReferenceScore) == "" {
		settings.LengthNearReferenceScore = "20"
	}
	if strings.TrimSpace(settings.LengthAcceptableDistancePercent) == "" {
		settings.LengthAcceptableDistancePercent = "30"
	}
	if strings.TrimSpace(settings.LengthAcceptableReferenceScore) == "" {
		settings.LengthAcceptableReferenceScore = "8"
	}
	if strings.TrimSpace(settings.LengthFarDistancePercent) == "" {
		settings.LengthFarDistancePercent = "60"
	}
	if strings.TrimSpace(settings.LengthFarReferencePenalty) == "" {
		settings.LengthFarReferencePenalty = "20"
	}
	return settings
}

func normalizeTUIFamilyBlastSettings(settings FamilyBlastSettings) FamilyBlastSettings {
	if strings.TrimSpace(settings.MinimumGroupSize) == "" {
		settings.MinimumGroupSize = "2"
	}
	if strings.TrimSpace(settings.RankingTieBreakerOrder) == "" {
		settings.RankingTieBreakerOrder = "evalue,reference,identity,coverage,bitscore"
	}
	if settings.PrependOnlyFirstQuery {
		settings.PrependOnlyFirstQuery = true
	}
	return settings
}

func familyBlastHelpPages() []localizedHelpPage {
	return []localizedHelpPage{
		{
			Label:    "English",
			Shortcut: "1",
			Title:    "Family BLAST help",
			Text: strings.TrimSpace(`Family BLAST is for query sets where several proteins represent one functional gene family, such as NAME1, NAME2, NAME3, and NAME4. The BLAST jobs still run per query, but the review/export unit becomes the detected family.

Enable Family BLAST mode keeps the detected grouped workflow active. If it is off, the batch behaves like normal multi-file BLAST and each query remains separate.

Group queries by detected family prefix derives a family name from the query label without changing the original label_name. For example, NAME1/NAME2/NAME3/NAME4 becomes NAME, ATNAME1/ATNAME2 becomes ATNAME, and GROUP.1/GROUP2 becomes GROUP.

Ignore suffix after member number before grouping is on by default. Labels shaped like prefix + number + suffix, such as IRX10-like or IRX10_like, are grouped by the part through the number before normal member-number stripping. This makes IRX9, IRX14, IRX10, and IRX10-like group together as IRX.

Strip Arabidopsis At/AT prefix only for family_name is off by default. Turn it on only when you deliberately want Arabidopsis-style aliases such as ATPAL1/ATPAL2 to group as PAL instead of ATPAL. This never rewrites the original label_name.

Merge grouped result rows by target protein/gene removes duplicate target candidates inside a family. If the same target protein is hit by several grouped queries, it is counted once as one family candidate.

Keep best BLAST hit chooses the strongest row for a duplicated target, preferring lower E-value, then higher identity, query coverage, and bitscore.

minimum queries per group controls how many query members must share a detected family prefix before the modal offers Family BLAST. The default is 2.

This mode does not require UniProt or InterPro to run. External references still make the grouped result much more useful, and the automatic filter remains available only when all required external references are present.`),
		},
		{
			Label:    "中文",
			Shortcut: "2",
			Title:    "Family BLAST 帮助",
			Text: strings.TrimSpace(`Family BLAST 用于多个蛋白共同代表一个功能基因家族的情况，例如 NAME1、NAME2、NAME3、NAME4。BLAST 仍然按每条 query 分别执行，但查看和导出的单位会变成检测到的 family。

Enable Family BLAST mode 用来开启这个分组流程。关闭后，多文件 BLAST 会回到普通模式，每个 query 仍然是独立结果和独立文件。

Group queries by detected family prefix 会从 query label 推断 family 名，但不会修改原始 label_name。例如 NAME1/NAME2/NAME3/NAME4 会变成 NAME，ATNAME1/ATNAME2 会变成 ATNAME，GROUP.1/GROUP2 会变成 GROUP。

Ignore suffix after member number before grouping 默认开启。像 IRX10-like 或 IRX10_like 这种“前缀 + 数字 + 后缀”的 label，会先只保留到数字为止，再做普通的成员编号去除。因此 IRX9、IRX14、IRX10、IRX10-like 会一起分到 IRX。

Strip Arabidopsis At/AT prefix only for family_name 默认关闭。只有当你明确希望把 ATPAL1/ATPAL2 这类 Arabidopsis-style alias 按 PAL 而不是 ATPAL 分组时才打开。它永远不会改写原始 label_name。

Merge grouped result rows by target protein/gene 会在 family 内按目标蛋白或基因去重。同一个目标蛋白如果同时被多个同组 query 命中，只算一个 family candidate。

Keep best BLAST hit 会在重复 target 中选择证据最强的一行，优先看更低 E-value，然后看 identity、query coverage 和 bitscore。

minimum queries per group 控制至少多少条 query 共享同一个 family 前缀才弹出 Family BLAST。默认是 2。

这个模式本身不要求 UniProt 或 InterPro 才能运行。外部参考器会让合并结果更有判断价值，而自动筛选器仍然只在必要外部参考器都存在时可用。`),
		},
		{
			Label:    "日本語",
			Shortcut: "3",
			Title:    "Family BLAST ヘルプ",
			Text: strings.TrimSpace(`Family BLAST は、NAME1、NAME2、NAME3、NAME4 のように複数のタンパク質が一つの機能的遺伝子ファミリーを代表する場合に使います。BLAST 実行は各 query ごとに行いますが、確認と出力の単位は検出された family になります。

Enable Family BLAST mode は、このグループ化ワークフローを有効にします。無効にすると、通常の複数ファイル BLAST と同じく各 query が独立した結果になります。

Group queries by detected family prefix は元の label_name を変更せず、query label から family 名を推定します。たとえば NAME1/NAME2/NAME3/NAME4 は NAME、ATNAME1/ATNAME2 は ATNAME、GROUP.1/GROUP2 は GROUP になります。

Ignore suffix after member number before grouping は既定でオンです。IRX10-like や IRX10_like のような「prefix + number + suffix」形式の label は、通常のメンバー番号除去の前に数字までを使います。そのため IRX9、IRX14、IRX10、IRX10-like は IRX として同じグループになります。

Strip Arabidopsis At/AT prefix only for family_name は既定でオフです。ATPAL1/ATPAL2 のような Arabidopsis-style alias を ATPAL ではなく PAL としてグループ化したい場合だけオンにします。元の label_name は変更しません。

Merge grouped result rows by target protein/gene は family 内で target protein/gene を重複除去します。同じ target protein が複数の同じグループの query にヒットした場合、family candidate として一つだけ数えます。

Keep best BLAST hit は重複 target の中から最も強い行を選びます。低い E-value、高い identity、query coverage、bitscore を優先します。

minimum queries per group は、同じ family prefix を共有する query が何本以上なら Family BLAST を提案するかを決めます。既定値は 2 です。

このモード自体は UniProt や InterPro がなくても実行できます。ただし外部参照がある方が grouped result の判断は強くなり、自動フィルターは必要な外部参照が揃っている場合だけ利用できます。`),
		},
	}
}

func blastFilterHelpPages() []localizedHelpPage {
	return []localizedHelpPage{
		{
			Label:    "English",
			Shortcut: "1",
			Title:    "BLAST filter help",
			Text: strings.TrimSpace(`The BLAST filter is an automatic uncheck suggestion for result tables. It does not delete rows and it does not lock the selection. After applying it, rows suggested for removal are unchecked and their row numbers are shown in red, so you can still manually re-check anything that looks biologically meaningful.

The filter is available only when all external references are enabled for the BLAST run, because the default judgment depends on the original BLAST columns plus UniProt and InterPro evidence.

min identity (%) is an optional extra hard rule for removing weak local similarity. The default is 0, meaning identity does not reject rows by itself. Set a value such as 30% when you want BLAST similarity strength to become a hard cutoff in addition to reference evidence.

min align/query coverage (%) is an optional extra hard rule for removing hits that align only a small part of the query. The default is 0, meaning query coverage does not reject rows by itself. Turn it on when you specifically want to remove short local alignments before manual review.

max E-value is an optional extra hard rule for removing statistically weak BLAST hits. The default is 0, meaning E-value does not reject rows by itself. A strict value such as 1e-30 can be enabled for narrower follow-up passes, and it remains generic rather than tied to any species, pathway, or gene family.

target_length / UniProt canonical length range checks whether the original database target protein length is plausible compared with the UniProt canonical reference. The default 70-130% range is meant to catch clearly truncated or overly extended mappings without forcing every isoform to be exactly the same length. Require length ratio data is on by default, so rows without a UniProt canonical length ratio are treated as insufficiently supported instead of unknown-pass.

Require UniProt accession is off by default. Turn it on only when missing UniProt mapping should be treated as a failure in your current dataset.

Prefer UniProt reviewed entries adds positive score for Swiss-Prot reviewed records. It is not a hard default rejection, because many plant proteins are valid but still unreviewed.

Reject UniProt fragment entries removes rows mapped to UniProt fragment records. This helps avoid sequences that look homologous only because the reference itself is incomplete.

Reject UniProt sequence cautions removes rows with UniProt sequence caution text. It is off by default because caution notes need manual interpretation.

InterPro conserved region status is the most important external evidence for preserved conserved sequence/domain support. By default, conserved-region evidence is required: present and partial are kept, while missing, uncertain, and blank status are suggested for removal.

min InterPro coverage (%) is optional. Leave it at 0 to avoid using a numeric coverage cutoff; set it when you want InterPro coverage to act as an extra hard threshold. Require InterPro coverage when used controls the missing-data behavior for that threshold: if it is on, rows without a numeric InterPro coverage value fail once the coverage threshold is enabled.

Keep best isoform per target gene collapses transcript isoforms for the same target gene and keeps the strongest row by external-reference evidence, identity, coverage, and E-value. This is on by default because homolog review is usually clearer at the gene-candidate level before you inspect individual transcript isoforms.

Keep only top hits per query limits the number of surviving candidates after scoring. It is useful for very large BLAST tables but is off by default because gene-family expansion and recent duplication are often biologically meaningful.

Use soft evidence score turns on a weighted score in addition to hard rules. The default workflow uses hard rules only; enable scoring when you want reviewed status, annotation richness, and InterPro strength to contribute to a combined confidence threshold.

The score and ranking controls expose every internal value used by the filter. Soft-score weights set how much identity, query coverage, length ratio, InterPro present/partial/coverage, reviewed UniProt, annotation richness, fragments, and sequence cautions contribute when soft scoring is enabled. Reference-score controls set how duplicate isoforms and top-hit limiting rank otherwise similar rows, including InterPro status scores, coverage divisor, UniProt evidence scores, fragment/caution penalty multipliers, and length-distance bands. The tie-break order field accepts comma-separated items: score, identity, coverage, reference, evalue, bitscore. The matching checkboxes decide whether each item is active.

A practical default interpretation for pathway-family work is: remove rows with missing or abnormal canonical-length evidence, UniProt fragment warnings, or missing/uncertain/blank InterPro conserved-region evidence; keep present/partial conserved-domain rows visible for manual review. Identity, query coverage, and E-value remain available as optional stricter follow-up filters.`),
		},
		{
			Label:    "中文",
			Shortcut: "2",
			Title:    "BLAST 筛选器帮助",
			Text: strings.TrimSpace(`BLAST 筛选器是结果表格里的“自动取消勾选建议”。它不会删除行，也不会锁定选择状态。应用后，被建议移除的行会自动取消勾选，行号显示为红色；你仍然可以手动把任何看起来有生物学意义的行重新勾选回来。

只有 BLAST 运行启用了全部外部参考器时，筛选器才可用，因为默认判断依赖原始 BLAST 列、UniProt 证据和 InterPro 证据一起判断。

min identity (%) 是可选的额外硬阈值，用来去掉局部相似性太弱的命中。默认是 0，表示 identity 不会单独导致取消勾选。需要让 BLAST 相似性强度也变成硬筛选时，可以设成 30% 之类的值。

min align/query coverage (%) 是可选的额外硬阈值，用来去掉只覆盖 query 很小一部分的命中。默认是 0，表示 query coverage 不会单独导致取消勾选。需要在人工检查前先去掉短局部比对时再开启。

max E-value 是可选的额外硬阈值，用来去掉统计显著性不够强的 BLAST 命中。默认是 0，表示 E-value 不会单独导致取消勾选。需要更窄的后续筛选时可以设成 1e-30 之类的严格值；这个阈值仍然是通用的，不绑定任何物种、通路或基因家族。

target_length / UniProt canonical length range 用来判断原始数据库里的目标蛋白长度相对 UniProt canonical 参考是否合理。默认 70-130%，目的是抓住明显截短或明显过长的映射，同时不强迫所有 isoform 都完全等长。Require length ratio data 默认开启，所以没有 UniProt canonical length ratio 的行会被视为证据不足，而不是 unknown 后默认通过。

Require UniProt accession 默认关闭。只有在你的当前数据集中，缺少 UniProt 映射本身就应该被视为失败时，才建议打开。

Prefer UniProt reviewed entries 会给 Swiss-Prot reviewed 条目加分。它默认不是硬性剔除，因为很多植物蛋白是可信的，但仍然处于 unreviewed 状态。

Reject UniProt fragment entries 会移除映射到 UniProt fragment 记录的行。这有助于避免参考记录本身不完整导致的假阳性。

Reject UniProt sequence cautions 会移除带有 UniProt sequence caution 文本的行。默认关闭，因为 caution 的具体含义通常需要人工解释。

InterPro conserved region status 是判断保存配列、保守结构域或保守家族证据是否保留的最重要外部证据。默认要求必须有保守区域证据：present 和 partial 保留，missing、uncertain 和空状态都会建议移除。

min InterPro coverage (%) 是可选项。保持 0 表示不用数值覆盖度作为硬阈值；如果希望 InterPro coverage 也参与硬筛选，可以设置一个具体比例。Require InterPro coverage when used 控制这个阈值开启后的缺失数据处理：打开时，没有数值型 InterPro coverage 的行会直接不通过这个阈值。

Keep best isoform per target gene 会把同一目标基因的多个转录本 isoform 折叠为一条最佳行，优先保留外部参考证据、identity、coverage 和 E-value 最强的记录。默认开启，因为同源候选的第一轮检查通常先看基因候选层面，再回头检查具体转录本 isoform。

Keep only top hits per query 会在打分后限制每个 query 保留的候选数。它适合非常大的 BLAST 表，但默认关闭，因为基因家族扩张和近期复制经常有真实生物学意义。

Use soft evidence score 会在硬规则之外启用加权评分。默认流程只使用硬规则；当你希望 reviewed 状态、注释丰富度和 InterPro 强度组成一个综合置信度阈值时，可以打开评分。

Score 和 ranking 控制项会暴露筛选器内部用到的全部数值。Soft-score weights 决定开启软评分后 identity、query coverage、长度比例、InterPro present/partial/coverage、UniProt reviewed、注释丰富度、fragment 和 sequence caution 各自怎样加分或扣分。Reference-score 控制项决定同一目标基因 isoform 去重和 top-hit 限制时，类似记录之间如何排序，包括 InterPro 状态分、coverage divisor、UniProt 证据分、fragment/caution 惩罚倍率、以及长度距离分段。tie-break order 输入框接受逗号分隔的项目：score, identity, coverage, reference, evalue, bitscore。旁边的复选框决定每一项是否启用。

对功能通路家族研究来说，默认理解可以是：移除缺少或异常的 canonical length 证据、UniProt fragment 警告、或 InterPro 保存区域证据 missing/uncertain/空的行；present/partial 的保守结构域行保留下来人工检查。identity、query coverage 和 E-value 仍然可以作为后续更严格筛选的可选条件。`),
		},
		{
			Label:    "日本語",
			Shortcut: "3",
			Title:    "BLAST フィルターヘルプ",
			Text: strings.TrimSpace(`BLAST フィルターは、結果表で使う「自動チェック解除の提案」です。行を削除せず、選択状態を固定するものでもありません。適用後、除外候補の行は自動的にチェック解除され、行番号が赤で表示されます。その後でも、生物学的に意味がありそうな行は手動で再チェックできます。

このフィルターは、BLAST 実行で外部参照がすべて有効な場合だけ使えます。既定の判定が、元の BLAST 列、UniProt 証拠、InterPro 証拠を合わせて使うためです。

min identity (%) は、局所類似性が弱いヒットを除くための任意の追加 hard rule です。既定値は 0 で、identity だけでは行を除外しません。BLAST similarity の強さも硬い条件にしたい場合に、30% などの値を設定します。

min align/query coverage (%) は、query の一部だけにしかアラインしないヒットを除くための任意の追加 hard rule です。既定値は 0 で、query coverage だけでは行を除外しません。短い局所アラインメントを手動確認の前に落としたい場合に有効にします。

max E-value は、統計的に弱い BLAST ヒットを除くための任意の追加 hard rule です。既定値は 0 で、E-value だけでは行を除外しません。1e-30 のような厳しい値は、より狭い follow-up pass で有効にできます。この閾値も特定の種、経路、遺伝子ファミリーには結び付きません。

target_length / UniProt canonical length range は、元データベースのターゲットタンパク質長が UniProt canonical 参照と比べて妥当かを確認します。既定の 70-130% は、明らかな短縮や過度な延長を検出しつつ、すべての isoform に完全な同長性を要求しないための範囲です。Require length ratio data は既定で有効なので、UniProt canonical length ratio がない行は unknown-pass ではなく証拠不足として扱います。

Require UniProt accession は既定で無効です。現在のデータセットで UniProt マッピング欠如そのものを失敗扱いにしたい場合だけ有効にしてください。

Prefer UniProt reviewed entries は Swiss-Prot reviewed レコードに加点します。これは既定では硬い除外条件ではありません。植物タンパク質には妥当でも unreviewed のままのものが多いためです。

Reject UniProt fragment entries は UniProt fragment レコードに対応する行を除きます。参照自体が不完全なために生じる紛らわしいヒットを減らします。

Reject UniProt sequence cautions は UniProt の sequence caution テキストを持つ行を除きます。caution の意味は個別確認が必要なことが多いため、既定では無効です。

InterPro conserved region status は、保存配列、保存ドメイン、保存ファミリーの証拠が残っているかを見る最重要の外部証拠です。既定では保存領域証拠を必須とし、present と partial を残し、missing、uncertain、空状態は除外候補にします。

min InterPro coverage (%) は任意項目です。0 のままなら数値 coverage を硬い閾値にしません。InterPro coverage も厳密に使いたい場合に値を設定します。Require InterPro coverage when used は、この閾値を有効にしたときの欠損データ処理を決めます。有効な場合、数値 InterPro coverage がない行はこの閾値を満たさないものとして扱われます。

Keep best isoform per target gene は、同じターゲット遺伝子の複数 transcript isoform を 1 つにまとめ、外部参照証拠、identity、coverage、E-value が最も強い行を残します。ホモログ候補の一次確認では、まず gene candidate レベルで整理してから各 transcript isoform を確認する方が扱いやすいため、既定で有効です。

Keep only top hits per query は、スコアリング後に query ごとの残存候補数を制限します。非常に大きな BLAST 表では便利ですが、遺伝子ファミリー拡大や最近の重複が生物学的に重要なことがあるため、既定では無効です。

Use soft evidence score は硬いルールに加えて加重スコアを使います。既定ワークフローは硬いルール中心です。reviewed 状態、注釈量、InterPro 証拠の強さを総合的な信頼度閾値にしたい場合に有効にします。

Score と ranking のコントロールは、フィルター内部で使うすべての数値を公開します。Soft-score weights は、soft scoring 有効時に identity、query coverage、length ratio、InterPro present/partial/coverage、UniProt reviewed、annotation richness、fragment、sequence caution がどれだけ加点または減点されるかを決めます。Reference-score controls は、同一ターゲット遺伝子 isoform の整理や top-hit 制限で似た行を並べ替えるための値です。InterPro status score、coverage divisor、UniProt evidence score、fragment/caution penalty multiplier、length-distance band を含みます。tie-break order 欄には、score, identity, coverage, reference, evalue, bitscore をカンマ区切りで指定できます。対応するチェックボックスで各項目を有効または無効にします。

機能経路ファミリー研究での実用的な既定解釈は、canonical length 証拠がないまたは異常、UniProt fragment 警告がある、または InterPro 保存領域証拠が missing/uncertain/空の行を除外候補にし、present/partial の保存ドメイン行を人が確認できるよう残す、というものです。identity、query coverage、E-value は、より厳しい後続フィルターとして任意に使えます。`),
		},
	}
}

func interProParameterHelpPages() []localizedHelpPage {
	return []localizedHelpPage{
		{
			Label:    "English",
			Shortcut: "1",
			Title:    "InterPro parameter help",
			Text: strings.TrimSpace(`InterPro conserved region status is an external-reference judgment used to mark whether a BLAST hit still carries conserved-region evidence expected from the query protein or, when no query template is available, whether the hit itself has credible conserved-domain evidence. It is a reference column, not a filter; it helps you decide whether a hit is biologically plausible before exporting or running follow-up searches.

Use Pfam accession match gives the strongest evidence weight. Pfam accessions are stable member-database identifiers for protein families and domains. When a query match and a hit match share a Pfam accession, the hit is very likely to preserve the same conserved family/domain signal.

Use InterPro accession match compares InterPro entry accessions such as IPR identifiers. This is broader than Pfam because InterPro integrates multiple member databases, and it can confirm that two proteins belong to the same integrated domain, family, repeat, site, or homologous superfamily entry.

Use member signature accession match compares the underlying member-database signatures reported by InterPro. This helps when the integrated InterPro accession is absent or broad, but the same HMM/signature evidence is still present.

Use entry type agreement requires compatible InterPro entry types when scoring matches. The accepted conserved types are domain, family, homologous_superfamily, repeat, and site; this reduces false support from unrelated annotation categories.

Use entry name agreement adds a weak supporting check using the InterPro entry name. It is disabled by default because names can be broad, edited over time, or shared by related but not identical functional groups.

Use coverage thresholds makes present and partial depend on how much conserved-region span is covered. With a query template, coverage is calculated against matched query conserved-region span. Without a query template, the best conserved match coverage on the hit is used.

Use match-region evidence adds support when both query and hit have actual InterPro coordinate regions. It is a weak evidence item, but it helps distinguish real localized conserved regions from accession-only annotations.

present min coverage (%) is the minimum conserved-region coverage required for present. The default is 70, meaning the hit must preserve most of the expected conserved span before it is treated as present.

partial min coverage (%) is the minimum conserved-region coverage required for partial. The default is 25, allowing weaker but still meaningful conserved evidence to be marked as partial instead of missing.

present min matched items is the minimum number of conserved evidence items required for present. The default is 1 because many pathway enzymes have one dominant conserved family/domain that is sufficient for a first-pass reference judgment.

partial min matched items is the minimum number of conserved evidence items required for partial. The default is 1, so a single credible but incomplete conserved match can remain visible as partial for manual review.`),
		},
		{
			Label:    "中文",
			Shortcut: "2",
			Title:    "InterPro 参数帮助",
			Text: strings.TrimSpace(`InterPro conserved region status 是一个外部参考判断，用来标记 BLAST 命中蛋白是否仍然带有 query 蛋白预期的保守区域证据；如果没有可用的 query 模板，就保守地判断命中蛋白自身是否有可信的保守结构域证据。它是参考列，不是筛选器；用途是在导出或继续搜索前帮助判断命中是否具有生物学可信度。

Use Pfam accession match 是权重最高的证据项。Pfam accession 是蛋白家族和结构域的稳定成员数据库编号。如果 query 与 hit 共享同一个 Pfam accession，通常说明 hit 保留了相同的保守家族或结构域信号。

Use InterPro accession match 会比较 InterPro 条目 accession，例如 IPR 编号。它比 Pfam 更宽，因为 InterPro 整合多个成员数据库；如果两个蛋白共享同一个 InterPro 条目，可以支持它们属于同一个整合后的结构域、家族、repeat、site 或同源超家族。

Use member signature accession match 会比较 InterPro 返回的底层成员数据库 signature。它适合在 InterPro accession 缺失或过宽时使用，只要相同的 HMM/signature 证据仍然存在，就能提供支持。

Use entry type agreement 会在打分时要求 InterPro 条目类型相容。默认认为可用于保守区域判断的类型包括 domain、family、homologous_superfamily、repeat 和 site；这样可以减少不相关注释类型造成的误判。

Use entry name agreement 会使用 InterPro 条目名称作为较弱的辅助证据。它默认关闭，因为名称可能比较宽泛、会随数据库更新调整，或者被相近但并不完全相同的功能群共享。

Use coverage thresholds 会让 present 和 partial 依赖保守区域覆盖比例。有 query 模板时，覆盖度根据命中的 query 保守区域跨度计算；没有 query 模板时，会使用 hit 自身最佳保守匹配区域的覆盖度。

Use match-region evidence 会在 query 和 hit 都有实际 InterPro 坐标区域时增加支持。它是较弱证据，但能帮助区分真实的局部保守区域和只有 accession 的注释。

present min coverage (%) 是判定 present 所需的最低保守区域覆盖比例。默认值是 70，表示 hit 需要保留大部分预期保守区域跨度，才会被视为 present。

partial min coverage (%) 是判定 partial 所需的最低保守区域覆盖比例。默认值是 25，用来把较弱但仍有意义的保守证据标记为 partial，而不是 missing。

present min matched items 是判定 present 所需的最少保守证据项数量。默认值是 1，因为很多通路酶只有一个主要保守家族或结构域，对第一轮参考判断通常已经足够。

partial min matched items 是判定 partial 所需的最少保守证据项数量。默认值是 1，因此一个可信但不完整的保守匹配也会以 partial 保留下来，方便人工检查。`),
		},
		{
			Label:    "日本語",
			Shortcut: "3",
			Title:    "InterPro パラメータヘルプ",
			Text: strings.TrimSpace(`InterPro conserved region status は外部参照としての判定列です。BLAST ヒットが query タンパク質で期待される保存領域の証拠を保持しているか、query テンプレートがない場合はヒット自身に信頼できる保存ドメイン証拠があるかを示します。これはフィルターではなく参照列であり、エクスポートや追加検索の前に生物学的に妥当なヒットかを判断するために使います。

Use Pfam accession match は最も強い証拠として扱われます。Pfam accession はタンパク質ファミリーやドメインの安定したメンバーデータベース ID です。query と hit が同じ Pfam accession を共有する場合、hit は同じ保存ファミリーまたはドメインのシグナルを保持している可能性が高くなります。

Use InterPro accession match は IPR などの InterPro エントリー accession を比較します。InterPro は複数のメンバーデータベースを統合しているため Pfam より広い証拠であり、2 つのタンパク質が同じ統合ドメイン、ファミリー、リピート、サイト、または相同スーパーファミリーに属することを確認できます。

Use member signature accession match は InterPro が返す下位メンバーデータベースの signature を比較します。InterPro accession がない、または広すぎる場合でも、同じ HMM/signature 証拠が残っていれば補助証拠になります。

Use entry type agreement はマッチを評価するときに InterPro エントリータイプの一致を考慮します。保存領域判定に使う既定タイプは domain、family、homologous_superfamily、repeat、site で、無関係な注釈カテゴリによる誤判定を減らします。

Use entry name agreement は InterPro エントリー名を弱い補助証拠として使います。名称は広すぎる場合があり、更新で変わることもあり、近縁だが同一ではない機能群で共有されることもあるため、既定では無効です。

Use coverage thresholds は present と partial の判定を保存領域のカバレッジに依存させます。query テンプレートがある場合は、マッチした query 保存領域の長さに対するカバレッジを使います。query テンプレートがない場合は、hit 上の最良の保存マッチのカバレッジを使います。

Use match-region evidence は query と hit の両方に InterPro の座標領域がある場合に補助証拠を追加します。弱い証拠ですが、実際の局所的保存領域と accession だけの注釈を区別する助けになります。

present min coverage (%) は present と判定するために必要な保存領域カバレッジの下限です。既定値は 70 で、hit が期待される保存領域の大部分を保持している場合に present とみなします。

partial min coverage (%) は partial と判定するために必要な保存領域カバレッジの下限です。既定値は 25 で、弱いが意味のある保存証拠を missing ではなく partial として残します。

present min matched items は present と判定するために必要な保存証拠項目数の下限です。既定値は 1 です。多くの経路酵素では主要な保存ファミリーまたはドメインが 1 つあれば、初期の参照判定として十分な場合があります。

partial min matched items は partial と判定するために必要な保存証拠項目数の下限です。既定値は 1 で、信頼できるが不完全な保存マッチを partial として残し、人が確認できるようにします。`),
		},
	}
}

func actionCloseValue(actions []Action) string {
	for _, action := range actions {
		if actionLooksLikeClose(action.Label, action.Value) {
			return action.Value
		}
	}
	return ""
}

func actionLooksLikeClose(label string, value string) bool {
	label = strings.ToLower(strings.TrimSpace(label))
	value = strings.ToLower(strings.TrimSpace(value))
	return label == "close" || value == "close"
}

func RunTaskValue[T any](page TaskPage, task func(update func(string)) (T, error)) (T, error) {
	return runTaskValue(page, func(ctx context.Context, update func(string)) (T, error) {
		return task(update)
	})
}

func runTaskValue[T any](page TaskPage, task func(ctx context.Context, update func(string)) (T, error)) (T, error) {
	app := newApp()
	var zero T
	var result T
	var taskErr error
	var cancelled atomic.Bool
	var stopped atomic.Bool
	taskCtx, cancelTaskContext := context.WithCancel(context.Background())
	defer cancelTaskContext()

	status := strings.TrimSpace(page.Initial)
	if status == "" {
		status = strings.TrimSpace(page.Description)
	}
	if status == "" {
		status = strings.TrimSpace(page.Title)
	}

	statusView := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextColor(tview.Styles.PrimaryTextColor)
	statusView.SetText(status)

	modalBody := tview.NewFlex().SetDirection(tview.FlexRow)
	modalBody.SetBorder(true)
	modalBody.SetTitle(" " + trimColon(page.Title) + " ")
	modalBody.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(modalBody.Box, true)
	attachFocusBorder(modalBody.Box)
	if strings.TrimSpace(page.Description) != "" {
		modalBody.AddItem(textBlock(page.Description), 2, 0, false)
	}
	modalBody.AddItem(statusView, 0, 1, true)
	cancelTask := func() {
		if cancelled.CompareAndSwap(false, true) {
			cancelTaskContext()
			if stopped.CompareAndSwap(false, true) {
				app.Stop()
			}
		}
	}
	addButtonRow(modalBody, buttonRow(buttonSpec{
		Label:    ButtonCancel,
		Shortcut: ShortcutCancel,
		Action:   cancelTask,
		Visible:  true,
	}))
	app.SetRoot(taskModalRoot(page, modalBody, 90, 14), true)
	app.SetFocus(modalBody)
	taskReady := make(chan struct{})
	var taskReadyOnce sync.Once
	afterDraw := app.GetAfterDrawFunc()
	app.SetAfterDrawFunc(func(screen tcell.Screen) {
		if afterDraw != nil {
			afterDraw(screen)
		}
		taskReadyOnce.Do(func() {
			close(taskReady)
		})
	})

	var mu sync.Mutex
	lastDraw := time.Time{}
	setStatus := func(text string) {
		if cancelled.Load() {
			return
		}
		now := time.Now()
		mu.Lock()
		status = strings.TrimSpace(text)
		if now.Sub(lastDraw) < perf.UIThrottle() {
			mu.Unlock()
			return
		}
		lastDraw = now
		mu.Unlock()
		app.QueueUpdateDraw(func() {
			if cancelled.Load() {
				return
			}
			statusView.SetText(status)
		})
	}

	done := make(chan struct{})
	var stopAnimation sync.Once
	stop := func() {
		stopAnimation.Do(func() {
			close(done)
		})
	}
	go func() {
		select {
		case <-taskReady:
		case <-taskCtx.Done():
			return
		}
		frames := []string{"|", "/", "-", "\\"}
		ticker := time.NewTicker(perf.UIAnimationTick())
		defer ticker.Stop()
		frame := 0
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				mu.Lock()
				text := status
				mu.Unlock()
				app.QueueUpdateDraw(func() {
					if cancelled.Load() {
						return
					}
					statusView.SetText(fmt.Sprintf("[yellow]%s[white] %s", frames[frame%len(frames)], text))
				})
				frame++
			}
		}
	}()

	go func() {
		select {
		case <-taskReady:
		case <-taskCtx.Done():
			return
		}
		result, taskErr = task(taskCtx, setStatus)
		app.QueueUpdateDraw(func() {
			if stopped.CompareAndSwap(false, true) {
				app.Stop()
			}
		})
	}()

	installInputCapture(app, func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			cancelTask()
			return nil
		}
		return event
	})
	if err := runApp(app); err != nil {
		stop()
		return zero, err
	}
	stop()
	if cancelled.Load() {
		return zero, taskCancelError(page)
	}
	if taskErr != nil {
		return zero, taskErr
	}
	return result, nil
}

func RunProgressTaskValue[T any](page TaskPage, task func(update func(current int, message string)) (T, error)) (T, error) {
	return runProgressTaskValue(page, func(ctx context.Context, update func(current int, message string)) (T, error) {
		return task(update)
	})
}

func runProgressTaskValue[T any](page TaskPage, task func(ctx context.Context, update func(current int, message string)) (T, error)) (T, error) {
	app := newApp()
	var zero T
	var result T
	var taskErr error
	var cancelled atomic.Bool
	var stopped atomic.Bool
	taskCtx, cancelTaskContext := context.WithCancel(context.Background())
	defer cancelTaskContext()
	current := 0
	status := strings.TrimSpace(page.Initial)
	if status == "" {
		status = strings.TrimSpace(page.Description)
	}
	if status == "" {
		status = strings.TrimSpace(page.Title)
	}
	total := page.Total

	statusView := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextColor(tview.Styles.PrimaryTextColor)

	render := func(frame string) string {
		if total <= 0 {
			return fmt.Sprintf("[yellow]%s[white] %s", frame, status)
		}
		ratio := float64(current) / float64(total)
		if ratio < 0 {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
		width := 32
		filled := int(ratio * float64(width))
		return fmt.Sprintf("[yellow]%s[white] %s\n\n[deepskyblue]%s[white]%s  %d/%d (%3.0f%%)",
			frame,
			status,
			strings.Repeat("#", filled),
			strings.Repeat("-", width-filled),
			current,
			total,
			ratio*100,
		)
	}
	statusView.SetText(render("|"))

	modalBody := tview.NewFlex().SetDirection(tview.FlexRow)
	modalBody.SetBorder(true)
	modalBody.SetTitle(" " + trimColon(page.Title) + " ")
	modalBody.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(modalBody.Box, true)
	attachFocusBorder(modalBody.Box)
	if strings.TrimSpace(page.Description) != "" {
		modalBody.AddItem(textBlock(page.Description), 2, 0, false)
	}
	modalBody.AddItem(statusView, 0, 1, true)
	cancelTask := func() {
		if cancelled.CompareAndSwap(false, true) {
			cancelTaskContext()
			if stopped.CompareAndSwap(false, true) {
				app.Stop()
			}
		}
	}
	addButtonRow(modalBody, buttonRow(buttonSpec{
		Label:    ButtonCancel,
		Shortcut: ShortcutCancel,
		Action:   cancelTask,
		Visible:  true,
	}))
	app.SetRoot(taskModalRoot(page, modalBody, 90, 14), true)
	app.SetFocus(modalBody)
	taskReady := make(chan struct{})
	var taskReadyOnce sync.Once
	afterDraw := app.GetAfterDrawFunc()
	app.SetAfterDrawFunc(func(screen tcell.Screen) {
		if afterDraw != nil {
			afterDraw(screen)
		}
		taskReadyOnce.Do(func() {
			close(taskReady)
		})
	})

	var mu sync.Mutex
	lastDraw := time.Time{}
	updateStatus := func(next int, message string) {
		if cancelled.Load() {
			return
		}
		now := time.Now()
		mu.Lock()
		current = next
		if strings.TrimSpace(message) != "" {
			status = strings.TrimSpace(message)
		}
		if now.Sub(lastDraw) < perf.UIThrottle() && next < total {
			mu.Unlock()
			return
		}
		lastDraw = now
		mu.Unlock()
		app.QueueUpdateDraw(func() {
			if cancelled.Load() {
				return
			}
			statusView.SetText(render("|"))
		})
	}

	done := make(chan struct{})
	var stopAnimation sync.Once
	stop := func() {
		stopAnimation.Do(func() {
			close(done)
		})
	}
	go func() {
		select {
		case <-taskReady:
		case <-taskCtx.Done():
			return
		}
		frames := []string{"|", "/", "-", "\\"}
		ticker := time.NewTicker(perf.UIAnimationTick())
		defer ticker.Stop()
		frame := 0
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				mu.Lock()
				text := render(frames[frame%len(frames)])
				mu.Unlock()
				app.QueueUpdateDraw(func() {
					if cancelled.Load() {
						return
					}
					statusView.SetText(text)
				})
				frame++
			}
		}
	}()

	go func() {
		select {
		case <-taskReady:
		case <-taskCtx.Done():
			return
		}
		result, taskErr = task(taskCtx, updateStatus)
		app.QueueUpdateDraw(func() {
			if stopped.CompareAndSwap(false, true) {
				app.Stop()
			}
		})
	}()

	installInputCapture(app, func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			cancelTask()
			return nil
		}
		return event
	})
	if err := runApp(app); err != nil {
		stop()
		return zero, err
	}
	stop()
	if cancelled.Load() {
		return zero, taskCancelError(page)
	}
	if taskErr != nil {
		return zero, taskErr
	}
	return result, nil
}

func pageBreadcrumb(fallback string, path []string) string {
	if len(path) == 0 {
		return fallback
	}
	return strings.Join(path, " > ")
}

func setFocusBorder(box *tview.Box, focused bool) {
	if box == nil {
		return
	}
	if focused {
		box.SetBorderColor(colorAction)
		box.SetTitleColor(colorAction)
		return
	}
	box.SetBorderColor(tview.Styles.BorderColor)
	box.SetTitleColor(tview.Styles.TitleColor)
}

func attachFocusBorder(box *tview.Box) {
	if box == nil {
		return
	}
	box.SetFocusFunc(func() {
		setFocusBorder(box, true)
	})
	box.SetBlurFunc(func() {
		setFocusBorder(box, false)
	})
}

func normalizeSelection(values []bool, size int, defaultValue bool) []bool {
	out := make([]bool, size)
	for i := range out {
		out[i] = defaultValue
	}
	copy(out, values)
	return out
}

func cloneBoolMatrix(values [][]bool) [][]bool {
	out := make([][]bool, len(values))
	for i := range values {
		out[i] = append([]bool(nil), values[i]...)
	}
	return out
}

func setAll(values []bool, value bool) {
	for i := range values {
		values[i] = value
	}
}

func rowSelectionGroups(rows []TableRow, labels []string) []rowSelectionGroup {
	groups := make([]rowSelectionGroup, 0)
	groupIndex := map[string]int{}
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if _, ok := groupIndex[label]; ok {
			continue
		}
		groupIndex[label] = len(groups)
		groups = append(groups, rowSelectionGroup{Label: label, Explicit: true})
	}
	for i, row := range rows {
		label := strings.TrimSpace(row.Group)
		if label == "" {
			continue
		}
		index, ok := groupIndex[label]
		if !ok {
			index = len(groups)
			groupIndex[label] = index
			groups = append(groups, rowSelectionGroup{Label: label})
		}
		groups[index].Rows = append(groups[index].Rows, i)
	}
	return groups
}

func compareRowOrder(rows []TableRow, leftIndex int, rightIndex int, sortState TableSort) int {
	var cmp int
	if sortState.Column == -1 {
		cmp = leftIndex - rightIndex
	} else {
		left, right := "", ""
		if sortState.Column < len(rows[leftIndex].Cells) {
			left = rows[leftIndex].Cells[sortState.Column]
		}
		if sortState.Column < len(rows[rightIndex].Cells) {
			right = rows[rightIndex].Cells[sortState.Column]
		}
		cmp = compareTableValues(left, right)
		if cmp == 0 {
			cmp = leftIndex - rightIndex
		}
	}
	if sortState.Direction == SortDescending {
		cmp = -cmp
	}
	return cmp
}

func (t *rowSelectionTable) Draw(screen tcell.Screen) {
	t.normalizeHorizontalOffset(screen)
	t.Table.Draw(screen)
	t.drawHeaderDivider(screen)
}

func (t *rowSelectionTable) normalizeHorizontalOffset(screen tcell.Screen) {
	columns := t.GetColumnCount()
	if columns <= rowSelectionFirstDataColumn {
		return
	}
	_, columnOffset := t.GetOffset()
	if columnOffset < 0 {
		rowOffset, _ := t.GetOffset()
		t.SetOffset(rowOffset, 0)
		columnOffset = 0
	}
	maxOffset := columns - rowSelectionFirstDataColumn - 1
	if maxOffset < 0 {
		maxOffset = 0
	}
	if columnOffset > maxOffset {
		rowOffset, _ := t.GetOffset()
		t.SetOffset(rowOffset, maxOffset)
	}
}

func (t *rowSelectionTable) drawHeaderDivider(screen tcell.Screen) {
	innerX, innerY, innerWidth, _ := t.GetInnerRect()
	if innerWidth <= 0 {
		return
	}
	dividerRow := t.dividerRow
	if dividerRow <= 0 {
		dividerRow = rowSelectionDividerRow
	}
	dividerY := innerY + dividerRow
	_, screenHeight := screen.Size()
	if dividerY < 0 || dividerY >= screenHeight {
		return
	}
	style := tcell.StyleDefault.Foreground(colorInactiveText).Background(tview.Styles.PrimitiveBackgroundColor)
	for x := innerX; x < innerX+innerWidth; x++ {
		screen.SetContent(x, dividerY, tview.Borders.Horizontal, nil, style)
	}
}

func (t *rowSelectionTable) tableColumnWidth(column int) int {
	if t != nil && column >= 0 && column < len(t.columnWidths) && t.columnWidths[column] > 0 {
		return t.columnWidths[column]
	}
	if t == nil {
		return 0
	}
	return tableColumnWidth(t.Table, column)
}

func tableColumnWidth(table *tview.Table, column int) int {
	width := 0
	rows := table.GetRowCount()
	for row := 0; row < rows; row++ {
		for _, line := range strings.Split(table.GetCell(row, column).Text, "\n") {
			cellWidth := tview.TaggedStringWidth(line)
			if cellWidth > width {
				width = cellWidth
			}
		}
	}
	return width
}

func rowSelectionColumnWidths(columns []TableColumn, rows []TableRow, layout rowSelectionLayout, includeGroups bool) []int {
	widths := make([]int, len(columns)+rowSelectionFirstDataColumn)
	widths[0] = taggedPaddedWidth("[x]")
	widths[1] = taggedPaddedWidth("row" + rowSelectionSortArrow(SortDescending))
	if maxRowWidth := taggedPaddedWidth(strconv.Itoa(len(rows))); maxRowWidth > widths[1] {
		widths[1] = maxRowWidth
	}
	if layout.headerTwoLine {
		widths[0] = maxInt(widths[0], taggedPaddedWidth(""))
		widths[1] = maxInt(widths[1], taggedPaddedWidth(""))
	}
	for i, column := range columns {
		width := 0
		header, subheader := tableHeaderLines(firstNonEmptyText(column.Header, column.ID))
		for _, value := range []string{header + rowSelectionSortArrow(SortDescending), subheader} {
			if w := taggedPaddedWidth(value); w > width {
				width = w
			}
		}
		if column.Width > 0 && column.Width+4 > width {
			width = column.Width + 4
		}
		widths[i+rowSelectionFirstDataColumn] = width
	}
	for _, row := range rows {
		for i, value := range row.Cells {
			column := i + rowSelectionFirstDataColumn
			if column >= len(widths) {
				break
			}
			if width := taggedPaddedWidth(value); width > widths[column] {
				widths[column] = width
			}
		}
		if includeGroups && strings.TrimSpace(row.Group) != "" && rowSelectionFirstDataColumn < len(widths) {
			if width := taggedPaddedWidth(row.Group); width > widths[rowSelectionFirstDataColumn] {
				widths[rowSelectionFirstDataColumn] = width
			}
		}
	}
	for i := range widths {
		if widths[i] <= 0 {
			widths[i] = taggedPaddedWidth("")
		}
	}
	return widths
}

func taggedPaddedWidth(text string) int {
	return tview.TaggedStringWidth("  " + text + "  ")
}

func textViewLineCount(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(strings.ReplaceAll(text, "\r\n", "\n"), "\n") + 1
}

func paddedTableCell(text string) *tview.TableCell {
	return tview.NewTableCell("  " + text + "  ")
}

func tableHeaderText(text string) string {
	text = strings.TrimSpace(text)
	return text
}

func tableHeaderLines(text string) (string, string) {
	text = tableHeaderText(text)
	parts := strings.SplitN(text, "\n", 2)
	first := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return first, ""
	}
	return first, strings.TrimSpace(parts[1])
}

func tableCellColor(column TableColumn, value string) tcell.Color {
	if column.ID == "uniprot_reviewed" {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "reviewed":
			return colorSelectionOn
		case "unreviewed":
			return colorMuted
		}
	}
	if column.ID == "interpro_conserved_region_status" {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "present":
			return colorSelectionOn
		case "partial":
			return colorMuted
		case "missing":
			return colorSelectionOff
		case "uncertain":
			return colorAction
		}
	}
	return tview.Styles.PrimaryTextColor
}

func tableHeaderStyle(column TableColumn) tcell.Style {
	switch {
	case strings.EqualFold(column.Reference, "uniprot"):
		return tcell.StyleDefault.Foreground(colorMuted).Bold(true)
	case strings.EqualFold(column.Reference, "interpro"):
		return tcell.StyleDefault.Foreground(colorAccent).Bold(true)
	default:
		return tcell.StyleDefault.Foreground(tview.Styles.PrimaryTextColor).Bold(true)
	}
}

func columnHelpPages(column TableColumn) []localizedHelpPage {
	name := firstNonEmptyText(column.Header, column.ID)
	en, zh, ja := splitTrilingualHelp(column.Help)
	if strings.TrimSpace(en) == "" {
		en = fmt.Sprintf("Column `%s` in the current table. This column is shown so you can inspect the data at a glance, compare it across rows, and decide whether the current hit is worth closer review, export, or follow-up analysis.", name)
	}
	if strings.TrimSpace(zh) == "" {
		zh = fmt.Sprintf("当前表格中的 `%s` 列。显示这一列是为了让你快速查看数据、比较不同行之间的差异，并判断这个结果是否值得进一步检查、导出或继续分析。", name)
	}
	if strings.TrimSpace(ja) == "" {
		ja = fmt.Sprintf("現在の表にある `%s` 列です。この列は、データを一目で確認し、行どうしを比較し、現在の結果をさらに確認・出力・追加解析する価値があるか判断するために表示されます。", name)
	}
	return []localizedHelpPage{
		{Label: "English", Shortcut: "1", Title: "Column details", Text: columnHelpPageText("Column", name, column.ID, column.Reference, en)},
		{Label: "中文", Shortcut: "2", Title: "列标题详细信息", Text: columnHelpPageText("列", name, column.ID, column.Reference, zh)},
		{Label: "日本語", Shortcut: "3", Title: "列見出しの詳細", Text: columnHelpPageText("列", name, column.ID, column.Reference, ja)},
	}
}

func columnHelpPageText(columnLabel string, name string, id string, reference string, body string) string {
	lines := []string{
		columnLabel + ": " + strings.TrimSpace(name),
		"id: " + strings.TrimSpace(id),
	}
	if strings.TrimSpace(reference) != "" {
		lines = append(lines, "reference: "+strings.TrimSpace(reference))
	}
	lines = append(lines, "", strings.TrimSpace(body))
	return strings.Join(lines, "\n")
}

func splitTrilingualHelp(help string) (string, string, string) {
	help = strings.TrimSpace(help)
	if help == "" {
		return "", "", ""
	}
	markers := []struct {
		key    string
		prefix string
	}{
		{"en", "EN:"},
		{"zh", "中文："},
		{"zh", "中文:"},
		{"ja", "日本語："},
		{"ja", "日本語:"},
	}
	type match struct {
		key    string
		start  int
		prefix int
	}
	matches := []match{}
	for _, marker := range markers {
		if idx := strings.Index(help, marker.prefix); idx >= 0 {
			matches = append(matches, match{key: marker.key, start: idx, prefix: len(marker.prefix)})
		}
	}
	if len(matches) == 0 {
		return help, "", ""
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].start < matches[j].start })
	values := map[string]string{}
	for i, m := range matches {
		end := len(help)
		if i+1 < len(matches) {
			end = matches[i+1].start
		}
		if _, exists := values[m.key]; !exists {
			values[m.key] = strings.TrimSpace(help[m.start+m.prefix : end])
		}
	}
	return values["en"], values["zh"], values["ja"]
}

func tableTitleWithCount(title string, selected int, total int) string {
	title = trimColon(title)
	lineCount := tableLineCountLabel(selected, total)
	if title == "" {
		return lineCount
	}
	return fmt.Sprintf("%s (%s)", title, lineCount)
}

func tableLineCountLabel(selected int, total int) string {
	if selected < 0 {
		selected = 0
	}
	if total < 0 {
		total = 0
	}
	if selected > total {
		selected = total
	}
	return fmt.Sprintf("%d/%d lines", selected, total)
}

func countSelectedBools(values []bool) int {
	count := 0
	for _, ok := range values {
		if ok {
			count++
		}
	}
	return count
}

func displayModalValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "~"
	}
	return value
}

func wrapPlainText(text string, width int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{""}
	}
	if width <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	lines := make([]string, 0, len(words))
	current := ""
	for _, word := range words {
		if current == "" {
			current = word
			continue
		}
		if len([]rune(current))+1+len([]rune(word)) <= width {
			current += " " + word
			continue
		}
		lines = append(lines, current)
		current = word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func modalFramePage(breadcrumb string, path []string, title string) InfoPage {
	return InfoPage{
		Breadcrumb: breadcrumb,
		Path:       path,
		Title:      title,
	}
}

func newLocalizedHelpModal(app *tview.Application, pages []localizedHelpPage, close func()) *localizedHelpModal {
	modal := &localizedHelpModal{pages: normalizeLocalizedHelpPages(pages)}
	modal.index = int(helpLanguageIndex.Load())
	if modal.index < 0 || modal.index >= len(modal.pages) {
		modal.index = 0
	}
	modal.helpBody = newButtonFlex()
	modal.helpBody.SetBorder(true)
	modal.helpBody.SetTitle(" Help ")
	modal.helpBody.SetTitleAlign(tview.AlignCenter)
	modal.helpTitle = textBlock("")
	modal.helpTitle.SetTextAlign(tview.AlignCenter)
	modal.helpTitle.SetTextColor(tview.Styles.SecondaryTextColor)
	modal.helpBody.AddItem(modal.helpTitle, 1, 0, false)
	modal.helpText = textBlock("")
	modal.helpText.SetScrollable(true)
	modal.helpText.SetBorder(true)
	modal.helpBody.AddItem(modal.helpText, 0, 1, true)
	languageButtons := make([]buttonSpec, 0, len(modal.pages)+1)
	for i := range modal.pages {
		index := i
		page := modal.pages[i]
		languageButtons = append(languageButtons, buttonSpec{
			Label:    page.Label,
			Shortcut: page.Shortcut,
			Action: func() {
				modal.SetLanguage(app, index)
			},
			Visible: true,
		})
	}
	languageButtons = append(languageButtons, buttonSpec{Label: ButtonOK, Shortcut: "Enter", Action: close, Visible: true, Primary: true})
	modal.helpButtons = buttonRow(languageButtons...)
	addButtonRow(modal.helpBody, modal.helpButtons)
	addHints(modal.helpBody, []string{"1/2/3 switch English, 中文, and 日本語. Up/Down scroll. PageUp/PageDown scroll faster. Enter, Esc, or F1 closes help."})
	modal.SetLanguage(app, modal.index)
	return modal
}

func normalizeLocalizedHelpPages(pages []localizedHelpPage) []localizedHelpPage {
	out := append([]localizedHelpPage(nil), pages...)
	defaults := []localizedHelpPage{
		{Label: "English", Shortcut: "1", Title: "Help", Text: "No help text is available."},
		{Label: "中文", Shortcut: "2", Title: "帮助", Text: "没有可用的帮助内容。"},
		{Label: "日本語", Shortcut: "3", Title: "ヘルプ", Text: "利用できるヘルプはありません。"},
	}
	for len(out) < len(defaults) {
		out = append(out, defaults[len(out)])
	}
	for i := range defaults {
		if strings.TrimSpace(out[i].Label) == "" {
			out[i].Label = defaults[i].Label
		}
		if strings.TrimSpace(out[i].Shortcut) == "" {
			out[i].Shortcut = defaults[i].Shortcut
		}
		if strings.TrimSpace(out[i].Title) == "" {
			out[i].Title = defaults[i].Title
		}
		if strings.TrimSpace(out[i].Text) == "" {
			out[i].Text = defaults[i].Text
		}
	}
	return out[:len(defaults)]
}

func (m *localizedHelpModal) SetLanguage(app *tview.Application, index int) {
	if m == nil || len(m.pages) == 0 {
		return
	}
	if index < 0 {
		index = len(m.pages) - 1
	}
	if index >= len(m.pages) {
		index = 0
	}
	m.index = index
	helpLanguageIndex.Store(int32(index))
	active := m.pages[m.index]
	if m.helpTitle != nil {
		m.helpTitle.SetText(active.Title)
	}
	if m.helpText != nil {
		m.helpText.SetText(active.Text)
		m.helpText.ScrollToBeginning()
	}
	if app != nil && m.helpText != nil {
		app.SetFocus(m.helpText)
	}
}

func (m *localizedHelpModal) Body() tview.Primitive {
	if m == nil {
		return nil
	}
	return m.helpBody
}

func (m *localizedHelpModal) Title() string {
	if m == nil || len(m.pages) == 0 {
		return "Help"
	}
	return m.pages[m.index].Title
}

func (m *localizedHelpModal) TextView() *tview.TextView {
	if m == nil {
		return nil
	}
	return m.helpText
}

func (m *localizedHelpModal) HandleKey(app *tview.Application, event *tcell.EventKey, close func()) bool {
	if m == nil || event == nil {
		return false
	}
	switch {
	case event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyEnter || shortcutMatchesEvent(ShortcutHelp, event):
		if close != nil {
			close()
		}
		return true
	case shortcutMatchesEvent("1", event):
		m.SetLanguage(app, 0)
		return true
	case shortcutMatchesEvent("2", event):
		m.SetLanguage(app, 1)
		return true
	case shortcutMatchesEvent("3", event):
		m.SetLanguage(app, 2)
		return true
	case event.Key() == tcell.KeyUp:
		scrollTextView(m.helpText, -1)
		return true
	case event.Key() == tcell.KeyDown:
		scrollTextView(m.helpText, 1)
		return true
	case event.Key() == tcell.KeyPgUp:
		scrollTextView(m.helpText, -10)
		return true
	case event.Key() == tcell.KeyPgDn:
		scrollTextView(m.helpText, 10)
		return true
	}
	return false
}

func scrollTextView(view *tview.TextView, delta int) {
	if view == nil || delta == 0 {
		return
	}
	row, column := view.GetScrollOffset()
	row += delta
	if row < 0 {
		row = 0
	}
	view.ScrollTo(row, column)
}

func isCopyShortcut(event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	if event.Key() == tcell.KeyCtrlC {
		return event.Modifiers()&tcell.ModShift != 0
	}
	if event.Key() == tcell.KeyRune && (event.Rune() == 'C' || event.Rune() == 'c') {
		return event.Modifiers()&tcell.ModCtrl != 0 && event.Modifiers()&tcell.ModShift != 0
	}
	return false
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func modalHeightForContent(contentRows int, minHeight int, maxHeight int) int {
	height := contentRows
	if height < minHeight {
		height = minHeight
	}
	if maxHeight > 0 && height > maxHeight {
		height = maxHeight
	}
	return height
}

func rowSelectionFirstSortableColumn(columns []TableColumn) int {
	for i, column := range columns {
		if column.Sortable {
			return i
		}
	}
	return -1
}

func rowSelectionSortArrow(direction SortDirection) string {
	if direction == SortDescending {
		return " v"
	}
	return " ^"
}

var numericTableValuePattern = regexp.MustCompile(`^\s*-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\s*$`)

func compareTableValues(left, right string) int {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if numericTableValuePattern.MatchString(left) && numericTableValuePattern.MatchString(right) {
		leftNumber, leftErr := strconv.ParseFloat(left, 64)
		rightNumber, rightErr := strconv.ParseFloat(right, 64)
		if leftErr == nil && rightErr == nil {
			switch {
			case leftNumber < rightNumber:
				return -1
			case leftNumber > rightNumber:
				return 1
			default:
				return 0
			}
		}
	}
	return strings.Compare(strings.ToLower(left), strings.ToLower(right))
}

func firstNonEmptyText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func indentSecondary(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = "  " + strings.TrimSpace(lines[i])
	}
	return strings.Join(lines, "\n")
}

func defaultChoiceFilter(query string, choices []Choice) []Choice {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]Choice{}, choices...)
	}
	out := make([]Choice, 0, len(choices))
	for _, choice := range choices {
		haystack := strings.ToLower(choice.Label + " " + choice.Description + " " + choice.Value)
		if strings.Contains(haystack, query) {
			out = append(out, choice)
		}
	}
	return out
}

func searchResultOffsetForSelection(currentOffset int, selectedIndex int, visibleCount int, viewportHeight int) int {
	if visibleCount <= 0 || viewportHeight <= 0 {
		return 0
	}
	totalRows := visibleCount * 2
	maxOffset := totalRows - viewportHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	offset := currentOffset
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	selectedTop := selectedIndex * 2
	selectedBottom := selectedTop + 1
	if selectedTop < offset {
		offset = selectedTop
	} else if selectedBottom >= offset+viewportHeight {
		offset = selectedBottom - viewportHeight + 1
	}
	if offset < 0 {
		return 0
	}
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

func textBlock(text string) *tview.TextView {
	return tview.NewTextView().
		SetText(strings.TrimSpace(text)).
		SetTextColor(tview.Styles.PrimaryTextColor).
		SetDynamicColors(true).
		SetWrap(true)
}

func textPanel(title string, text string) *tview.TextView {
	view := textBlock(text)
	view.SetBorder(true)
	view.SetTitle(" " + trimColon(title) + " ")
	view.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(view.Box, true)
	attachFocusBorder(view.Box)
	return view
}

func sectionHeader(title string) *tview.TextView {
	title = strings.TrimSpace(trimColon(title))
	if title == "" {
		title = "Section"
	}
	return tview.NewTextView().
		SetText("[gray]-- " + title + " " + strings.Repeat("-", 48)).
		SetTextColor(tview.Styles.SecondaryTextColor).
		SetDynamicColors(true).
		SetWrap(false)
}

func centeredPrimitive(primitive tview.Primitive, width int, height int) tview.Primitive {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 16
	}
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(primitive, height, 0, true).
			AddItem(nil, 0, 1, false), width, 0, true).
		AddItem(nil, 0, 1, false)
}

type flexAdder interface {
	AddItem(tview.Primitive, int, int, bool) *tview.Flex
}

func addHints(body flexAdder, hints []string) {
	for _, hint := range hints {
		hint = strings.TrimSpace(hint)
		if hint == "" {
			continue
		}
		body.AddItem(hintView(hint), 1, 0, false)
	}
}

func navButtons(allowBack bool, allowHome bool, allowConfirm bool, confirmText string, nav func(NavAction), confirm func()) *buttonRowPrimitive {
	return navButtonsWithShortcut(allowBack, allowHome, allowConfirm, confirmText, "Enter", nav, confirm)
}

func navButtonsWithShortcut(allowBack bool, allowHome bool, allowConfirm bool, confirmText string, confirmShortcut string, nav func(NavAction), confirm func()) *buttonRowPrimitive {
	buttons := []buttonSpec{
		{Label: ButtonBack, Shortcut: ShortcutBack, Action: func() { nav(NavBack) }, Visible: allowBack},
		{Label: ButtonHome, Shortcut: ShortcutHome, Action: func() { nav(NavHome) }, Visible: allowHome},
		{Label: conciseActionLabel(confirmText, ButtonSelect), Shortcut: confirmShortcut, Action: confirm, Visible: allowConfirm, Primary: true},
	}
	return buttonRow(buttons...)
}

func modalButtons(actions []buttonSpec, allowConfirm bool, confirmText string, confirmShortcut string, nav func(NavAction), confirm func()) *buttonRowPrimitive {
	_ = nav
	buttons := make([]buttonSpec, 0, len(actions)+1)
	for _, action := range actions {
		action.Primary = false
		buttons = append(buttons, action)
	}
	buttons = append(buttons, buttonSpec{
		Label:    conciseActionLabel(confirmText, ButtonOK),
		Shortcut: confirmShortcut,
		Action:   confirm,
		Visible:  allowConfirm,
		Primary:  true,
	})
	return buttonRow(buttons...)
}

func inputButtons(allowBack bool, allowHome bool, confirmText string, confirmShortcut string, paste func(), nav func(NavAction), confirm func()) *buttonRowPrimitive {
	buttons := []buttonSpec{
		{Label: ButtonBack, Shortcut: ShortcutBack, Action: func() { nav(NavBack) }, Visible: allowBack},
		{Label: ButtonHome, Shortcut: ShortcutHome, Action: func() { nav(NavHome) }, Visible: allowHome},
		{Label: ButtonPaste, Shortcut: ShortcutPaste, Action: paste, Visible: paste != nil},
		{Label: conciseActionLabel(confirmText, ButtonApply), Shortcut: confirmShortcut, Action: confirm, Visible: true, Primary: true},
	}
	return buttonRow(buttons...)
}

func clipPrimitive(child tview.Primitive) *clippedPrimitive {
	return &clippedPrimitive{Box: tview.NewBox(), child: child}
}

func (c *clippedPrimitive) Draw(screen tcell.Screen) {
	if c == nil || c.child == nil {
		return
	}
	x, y, width, height := c.GetRect()
	if width <= 0 || height <= 0 {
		return
	}
	screenWidth, screenHeight := screen.Size()
	if screenWidth <= 0 || screenHeight <= 0 || x >= screenWidth || y >= screenHeight {
		return
	}
	if width > screenWidth-x {
		width = screenWidth - x
	}
	if height > screenHeight-y {
		height = screenHeight - y
	}
	c.child.SetRect(x, y, width, height)
	c.child.Draw(&clippingScreen{Screen: screen, x: x, y: y, width: width, height: height})
}

func (c *clippedPrimitive) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return c.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if c.child == nil {
			return
		}
		if handler := c.child.InputHandler(); handler != nil {
			handler(event, setFocus)
		}
	})
}

func (c *clippedPrimitive) Focus(delegate func(p tview.Primitive)) {
	if c.child != nil {
		delegate(c.child)
		return
	}
	c.Box.Focus(delegate)
}

func (c *clippedPrimitive) HasFocus() bool {
	if c.child != nil && c.child.HasFocus() {
		return true
	}
	return c.Box.HasFocus()
}

func (c *clippedPrimitive) Blur() {
	c.Box.Blur()
	if c.child != nil {
		c.child.Blur()
	}
}

func (c *clippedPrimitive) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return c.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		if c.child == nil || event == nil {
			return false, nil
		}
		mouseX, mouseY := event.Position()
		if !primitiveContains(c.child, mouseX, mouseY) {
			return false, nil
		}
		if handler := c.child.MouseHandler(); handler != nil {
			return handler(action, event, setFocus)
		}
		return false, nil
	})
}

func (c *clippedPrimitive) PasteHandler() func(text string, setFocus func(p tview.Primitive)) {
	return c.WrapPasteHandler(func(text string, setFocus func(p tview.Primitive)) {
		if c.child == nil {
			return
		}
		if handler := c.child.PasteHandler(); handler != nil {
			handler(text, setFocus)
		}
	})
}

func (p *focusProxyPrimitive) Draw(screen tcell.Screen) {
	p.Box.DrawForSubclass(screen, p)
	if p.child == nil {
		return
	}
	x, y, width, height := p.GetInnerRect()
	p.child.SetRect(x, y, width, height)
	p.child.Draw(screen)
}

func (p *focusProxyPrimitive) InputHandler() func(event *tcell.EventKey, setFocus func(pr tview.Primitive)) {
	return p.WrapInputHandler(func(event *tcell.EventKey, setFocus func(pr tview.Primitive)) {
		if p.child == nil {
			return
		}
		if handler := p.child.InputHandler(); handler != nil {
			handler(event, setFocus)
		}
	})
}

func (p *focusProxyPrimitive) Focus(delegate func(pr tview.Primitive)) {
	if p.focusTarget != nil {
		if target := p.focusTarget(); target != nil {
			delegate(target)
			return
		}
	}
	if p.child != nil {
		delegate(p.child)
		return
	}
	p.Box.Focus(delegate)
}

func (p *focusProxyPrimitive) HasFocus() bool {
	if p.child != nil && p.child.HasFocus() {
		return true
	}
	return p.Box.HasFocus()
}

func (p *focusProxyPrimitive) Blur() {
	p.Box.Blur()
	if p.child != nil {
		p.child.Blur()
	}
}

func (p *focusProxyPrimitive) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(pr tview.Primitive)) (bool, tview.Primitive) {
	return p.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(pr tview.Primitive)) (bool, tview.Primitive) {
		if p.child == nil || event == nil {
			return false, nil
		}
		mouseX, mouseY := event.Position()
		if !primitiveContains(p.child, mouseX, mouseY) {
			return false, nil
		}
		if handler := p.child.MouseHandler(); handler != nil {
			return handler(action, event, setFocus)
		}
		return false, nil
	})
}

func (p *focusProxyPrimitive) PasteHandler() func(text string, setFocus func(pr tview.Primitive)) {
	return p.WrapPasteHandler(func(text string, setFocus func(pr tview.Primitive)) {
		if p.child == nil {
			return
		}
		if handler := p.child.PasteHandler(); handler != nil {
			handler(text, setFocus)
		}
	})
}

func primitiveContains(primitive tview.Primitive, x int, y int) bool {
	rectX, rectY, width, height := primitive.GetRect()
	return x >= rectX && x < rectX+width && y >= rectY && y < rectY+height
}

func primitiveBox(primitive tview.Primitive) *tview.Box {
	switch p := primitive.(type) {
	case *tview.InputField:
		return p.Box
	case *tview.Checkbox:
		return p.Box
	case *checkboxModule:
		return p.Box
	case *tview.TextArea:
		return p.Box
	case *tview.TextView:
		return p.Box
	case *tview.Table:
		return p.Box
	case *tview.List:
		return p.Box
	case *clippedPrimitive:
		return primitiveBox(p.child)
	case *buttonFlex:
		return p.Box
	case *buttonRowPrimitive:
		return p.Box
	}
	return nil
}

func inputFieldEditKey(event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	switch event.Key() {
	case tcell.KeyRune, tcell.KeyBackspace, tcell.KeyBackspace2, tcell.KeyDelete, tcell.KeyLeft, tcell.KeyRight, tcell.KeyHome, tcell.KeyEnd:
		return true
	default:
		return false
	}
}

func deliverInputFieldKey(input *tview.InputField, event *tcell.EventKey, app *tview.Application) {
	if input == nil || event == nil {
		return
	}
	if handler := input.InputHandler(); handler != nil {
		handler(event, func(p tview.Primitive) {
			if app != nil && p != nil {
				app.SetFocus(p)
			}
		})
	}
}

func (s *clippingScreen) SetContent(x int, y int, mainc rune, combc []rune, style tcell.Style) {
	if x < s.x || x >= s.x+s.width || y < s.y || y >= s.y+s.height {
		return
	}
	s.Screen.SetContent(x, y, mainc, combc, style)
}

func (s *clippingScreen) ShowCursor(x int, y int) {
	if x < s.x || x >= s.x+s.width || y < s.y || y >= s.y+s.height {
		s.Screen.HideCursor()
		return
	}
	s.Screen.ShowCursor(x, y)
}

func newButtonFlex() *buttonFlex {
	return &buttonFlex{Flex: tview.NewFlex().SetDirection(tview.FlexRow)}
}

func addButtonRow(body flexAdder, row *buttonRowPrimitive) {
	if body == nil || row == nil {
		return
	}
	body.AddItem(row, 1, 0, false)
	if flex, ok := body.(*buttonFlex); ok {
		flex.rows = append(flex.rows, row)
	}
}

func (b *buttonFlex) Draw(screen tcell.Screen) {
	b.syncButtonRowHeights(screen)
	b.Flex.Draw(screen)
}

func (b *buttonFlex) syncButtonRowHeights(screen tcell.Screen) {
	x, _, width, _ := b.GetInnerRect()
	screenWidth, _ := screen.Size()
	if width <= 0 || screenWidth <= 0 || x >= screenWidth {
		return
	}
	if width > screenWidth-x {
		width = screenWidth - x
	}
	if width == b.lastLayoutWidth {
		return
	}
	b.lastLayoutWidth = width
	for _, row := range b.rows {
		height := row.requiredHeight(width)
		if height < 1 {
			height = 1
		}
		b.ResizeItem(row, height, 0)
	}
}

func (b *buttonRowPrimitive) Draw(screen tcell.Screen) {
	b.Box.DrawForSubclass(screen, b)
	x, y, width, height := b.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}
	screenWidth, screenHeight := screen.Size()
	if screenWidth <= 0 || screenHeight <= 0 || x >= screenWidth || y >= screenHeight {
		return
	}
	if width > screenWidth-x {
		width = screenWidth - x
	}
	for _, pos := range b.buttonPositions(width) {
		drawY := y + pos.row
		if drawY < 0 || drawY >= screenHeight {
			continue
		}
		style := tcell.StyleDefault.Background(tview.Styles.ContrastBackgroundColor).Foreground(tview.Styles.PrimaryTextColor)
		if pos.button.Primary {
			style = tcell.StyleDefault.Background(colorAction).Foreground(colorActionText).Bold(true)
		}
		printStyledText(screen, x+pos.left, drawY, pos.right-pos.left, style, " "+pos.label+" ")
	}
}

func newButtonRow(buttons ...buttonSpec) *buttonRowPrimitive {
	return &buttonRowPrimitive{
		Box:     tview.NewBox(),
		buttons: buttons,
	}
}

func (b *buttonRowPrimitive) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return b.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if event == nil {
			return
		}
		if event.Key() != tcell.KeyEnter && !(event.Key() == tcell.KeyRune && event.Rune() == ' ') {
			return
		}
		for _, button := range b.buttons {
			if button.Visible && button.Primary && button.Action != nil {
				button.Action()
				return
			}
		}
		for _, button := range b.buttons {
			if button.Visible && button.Action != nil {
				button.Action()
				return
			}
		}
	})
}

func (b *buttonRowPrimitive) setPrimaryLabel(label string) {
	label = conciseActionLabel(label, label)
	for i := range b.buttons {
		if b.buttons[i].Primary {
			b.buttons[i].Label = label
		}
	}
}

func (b *buttonRowPrimitive) primaryButton() *buttonSpec {
	if b == nil {
		return nil
	}
	for i := range b.buttons {
		if b.buttons[i].Primary {
			return &b.buttons[i]
		}
	}
	return nil
}

func inputConfirmText(confirmText string, skipWhenEmpty bool, emptyText string, text string) string {
	if skipWhenEmpty && strings.TrimSpace(text) == "" {
		if strings.TrimSpace(emptyText) != "" {
			return emptyText
		}
		return ButtonSkip
	}
	return confirmText
}

func compactButtonLabel(button buttonSpec, rowWidth int) string {
	label := strings.TrimSpace(button.Label)
	shortcut := strings.TrimSpace(button.Shortcut)
	if shortcut == "" {
		return label
	}
	return label + " (" + shortcut + ")"
}

func printStyledText(screen tcell.Screen, x int, y int, maxWidth int, style tcell.Style, text string) {
	screenWidth, screenHeight := screen.Size()
	if screenWidth <= 0 || screenHeight <= 0 || y < 0 || y >= screenHeight || maxWidth <= 0 {
		return
	}
	if x < 0 {
		drop := -x
		runes := []rune(text)
		if drop >= len(runes) {
			return
		}
		text = string(runes[drop:])
		maxWidth -= drop
		x = 0
	}
	if x >= screenWidth {
		return
	}
	if maxWidth > screenWidth-x {
		maxWidth = screenWidth - x
	}
	runes := []rune(text)
	if len(runes) > maxWidth {
		runes = runes[:maxWidth]
	}
	for i, r := range runes {
		screen.SetContent(x+i, y, r, nil, style)
	}
}

func (p *pageSelectorPrimitive) Draw(screen tcell.Screen) {
	p.Box.DrawForSubclass(screen, p)
	x, y, width, height := p.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}
	screenWidth, screenHeight := screen.Size()
	if screenWidth <= 0 || screenHeight <= 0 || x >= screenWidth || y >= screenHeight {
		return
	}
	if width > screenWidth-x {
		width = screenWidth - x
	}
	if height > screenHeight-y {
		height = screenHeight - y
	}
	style := tcell.StyleDefault.Foreground(tview.Styles.SecondaryTextColor).Background(tview.Styles.PrimitiveBackgroundColor)
	activeStyle := tcell.StyleDefault.Foreground(colorAction).Background(tview.Styles.PrimitiveBackgroundColor).Bold(true)
	total := p.totalPages
	if total < 1 {
		total = 1
	}
	lines := p.pageLines(width, total)
	header := strings.TrimSpace(p.summary)
	if header == "" {
		header = fmt.Sprintf("Page %d/%d | %d matches", p.currentPage+1, total, p.matches)
	}
	printCentered(screen, x, y, width, style, header)
	for i, line := range lines {
		if i+1 >= height {
			break
		}
		lineWidth := len([]rune(line.text))
		left := x + (width-lineWidth)/2
		if left < x {
			left = x
		}
		for _, segment := range line.segments {
			segStyle := style
			if segment.page == p.currentPage {
				segStyle = activeStyle
			}
			printStyledText(screen, left+segment.left, y+i+1, segment.right-segment.left, segStyle, segment.text)
		}
	}
}

func (p *pageSelectorPrimitive) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		if action != tview.MouseLeftClick && action != tview.MouseLeftDown && action != tview.MouseLeftUp {
			return false, nil
		}
		x, y, width, height := p.GetInnerRect()
		mouseX, mouseY := event.Position()
		if mouseX < x || mouseX >= x+width || mouseY < y || mouseY >= y+height {
			return false, nil
		}
		if setFocus != nil {
			setFocus(p)
		}
		if action == tview.MouseLeftDown {
			return true, p
		}
		total := p.totalPages
		if total < 1 {
			total = 1
		}
		lines := p.pageLines(width, total)
		lineIndex := mouseY - y - 1
		if lineIndex >= 0 && lineIndex < len(lines) {
			line := lines[lineIndex]
			lineWidth := len([]rune(line.text))
			left := x + (width-lineWidth)/2
			if left < x {
				left = x
			}
			relativeX := mouseX - left
			for _, segment := range line.segments {
				if relativeX >= segment.left && relativeX < segment.right {
					if p.onSelect != nil && action == tview.MouseLeftClick {
						p.onSelect(segment.page)
					}
					return true, p
				}
			}
		}
		return true, p
	}
}

type pageSelectorLine struct {
	text     string
	segments []pageSelectorSegment
}

type pageSelectorSegment struct {
	page  int
	text  string
	left  int
	right int
}

func (p *pageSelectorPrimitive) pageLines(width int, total int) []pageSelectorLine {
	if width <= 0 {
		width = 80
	}
	lines := []pageSelectorLine{}
	current := pageSelectorLine{}
	appendCurrent := func() {
		if current.text != "" {
			lines = append(lines, current)
			current = pageSelectorLine{}
		}
	}
	for i := 0; i < total; i++ {
		label := fmt.Sprintf(" %d ", i+1)
		if i == p.currentPage {
			label = fmt.Sprintf(" [%d] ", i+1)
		}
		labelWidth := len([]rune(label))
		if current.text != "" && len([]rune(current.text))+labelWidth > width {
			appendCurrent()
		}
		left := len([]rune(current.text))
		current.text += label
		current.segments = append(current.segments, pageSelectorSegment{page: i, text: label, left: left, right: left + labelWidth})
	}
	appendCurrent()
	if len(lines) == 0 {
		lines = append(lines, pageSelectorLine{text: " 1 ", segments: []pageSelectorSegment{{page: 0, text: " 1 ", left: 0, right: 3}}})
	}
	return lines
}

func printCentered(screen tcell.Screen, x int, y int, width int, style tcell.Style, text string) {
	screenWidth, screenHeight := screen.Size()
	if screenWidth <= 0 || screenHeight <= 0 || y < 0 || y >= screenHeight || width <= 0 {
		return
	}
	if x < 0 {
		width += x
		x = 0
	}
	if x >= screenWidth || width <= 0 {
		return
	}
	if width > screenWidth-x {
		width = screenWidth - x
	}
	textWidth := len([]rune(text))
	left := x + (width-textWidth)/2
	if left < x {
		left = x
	}
	printStyledText(screen, left, y, width, style, text)
}

func (b *buttonRowPrimitive) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return b.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		if action != tview.MouseLeftClick && action != tview.MouseLeftDown {
			return false, nil
		}
		mouseX, mouseY := event.Position()
		x, y, width, height := b.GetInnerRect()
		if mouseX < x || mouseX >= x+width || mouseY < y || mouseY >= y+height {
			return false, nil
		}
		for _, pos := range b.buttonPositions(width) {
			left := x + pos.left
			right := x + pos.right
			top := y + pos.row
			if mouseY != top || mouseX < left || mouseX >= right {
				continue
			}
			if action == tview.MouseLeftDown {
				if setFocus != nil {
					setFocus(b)
				}
				return true, nil
			}
			if pos.button.Action != nil {
				pos.button.Action()
			}
			return true, nil
		}
		return false, nil
	})
}

type buttonPosition struct {
	button buttonSpec
	label  string
	row    int
	left   int
	right  int
}

func (b *buttonRowPrimitive) visibleButtonGroups() ([]buttonSpec, []buttonSpec) {
	navButtons := []buttonSpec{}
	primaryButtons := []buttonSpec{}
	for _, button := range b.buttons {
		if !button.Visible {
			continue
		}
		if button.Primary {
			primaryButtons = append(primaryButtons, button)
		} else {
			navButtons = append(navButtons, button)
		}
	}
	return navButtons, primaryButtons
}

func (b *buttonRowPrimitive) buttonPositions(width int) []buttonPosition {
	if width <= 0 {
		return nil
	}
	positions := []buttonPosition{}
	navButtons, primaryButtons := b.visibleButtonGroups()
	left := 0
	row := 0
	for _, button := range navButtons {
		label := compactButtonLabel(button, width)
		used := buttonWidth(label, width)
		if left > 0 && left+used > width {
			row++
			left = 0
		}
		positions = append(positions, buttonPosition{button: button, label: label, row: row, left: left, right: left + used})
		left += used + 1
	}
	if len(primaryButtons) > 0 {
		primaryRow := 0
		right := width
		if len(navButtons) > 0 {
			primaryRow = row
		}
		for i := len(primaryButtons) - 1; i >= 0; i-- {
			button := primaryButtons[i]
			label := compactButtonLabel(button, width)
			used := buttonWidth(label, width)
			if right < width && right-used < 0 {
				primaryRow++
				right = width
			}
			left := right - used
			if left < 0 {
				left = 0
			}
			if primaryRow == row && overlapsButtonPositions(positions, primaryRow, left, left+used) {
				primaryRow++
				right = width
				left = right - used
				if left < 0 {
					left = 0
				}
			}
			positions = append(positions, buttonPosition{button: button, label: label, row: primaryRow, left: left, right: left + used})
			right = left - 1
		}
	}
	return positions
}

func overlapsButtonPositions(positions []buttonPosition, row int, left int, right int) bool {
	for _, pos := range positions {
		if pos.row != row {
			continue
		}
		if left < pos.right+1 && right+1 > pos.left {
			return true
		}
	}
	return false
}

func (b *buttonRowPrimitive) requiredHeight(width int) int {
	height := 1
	for _, pos := range b.buttonPositions(width) {
		if pos.row+1 > height {
			height = pos.row + 1
		}
	}
	return height
}

func buttonWidth(label string, rowWidth int) int {
	_ = rowWidth
	used := len([]rune(label)) + 2
	if used < 4 {
		used = 4
	}
	return used
}

type keyBinding struct {
	Key         tcell.Key
	Match       func(*tcell.EventKey) bool
	Action      func()
	ActionEvent func(*tcell.EventKey)
}

func navCapture(app *tview.Application, allowBack bool, allowHome bool, nav func(NavAction), bindings ...keyBinding) func(*tcell.EventKey) *tcell.EventKey {
	_ = app
	return func(event *tcell.EventKey) *tcell.EventKey {
		for _, binding := range bindings {
			matches := false
			if binding.Match != nil {
				matches = binding.Match(event)
			} else {
				matches = event.Key() == binding.Key
			}
			if matches {
				if binding.ActionEvent != nil {
					binding.ActionEvent(event)
				} else if binding.Action != nil {
					binding.Action()
				}
				return nil
			}
		}
		switch event.Key() {
		case tcell.KeyEscape:
			if allowBack {
				nav(NavBack)
				return nil
			}
		case tcell.KeyCtrlO:
			if allowHome {
				nav(NavHome)
				return nil
			}
		}
		return event
	}
}

func isCtrlEnter(event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	if event.Modifiers()&tcell.ModCtrl == 0 {
		return false
	}
	switch event.Key() {
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		return true
	default:
		return false
	}
}

func selectionKey(event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	if event.Key() == tcell.KeyEnter || event.Key() == tcell.KeyUp || event.Key() == tcell.KeyDown || event.Key() == tcell.KeyPgUp || event.Key() == tcell.KeyPgDn {
		return true
	}
	return event.Key() == tcell.KeyRune && event.Rune() >= '1' && event.Rune() <= '9'
}

func newPasteStatus(focus func()) *pasteStatus {
	return &pasteStatus{
		view:  hintView(""),
		focus: focus,
	}
}

func runInlinePaste(app *tview.Application, status *pasteStatus, insert func(string)) {
	if app == nil || status == nil || insert == nil {
		return
	}
	id := status.seq.Add(1)
	status.view.SetTextColor(tview.Styles.SecondaryTextColor)
	status.view.SetText("Reading clipboard...")
	if status.focus != nil {
		status.focus()
	}
	go func() {
		text, err := readClipboardText()
		if err == nil {
			text, err = sanitizePastedText(text)
		}
		app.QueueUpdateDraw(func() {
			if status.seq.Load() != id {
				return
			}
			if err != nil {
				status.view.SetTextColor(colorMuted)
				status.view.SetText("Paste failed: " + err.Error())
				if status.focus != nil {
					status.focus()
				}
				return
			}
			insert(text)
			status.view.SetText("")
			if status.focus != nil {
				status.focus()
			}
		})
	}()
}

func resolveInputFileText(text string) (string, error) {
	text = strings.TrimSpace(text)
	path, ok, err := pastedFilePath(text)
	if err != nil {
		return "", err
	}
	if !ok {
		return text, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	fileText, err := sanitizePastedText(string(data))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	return fileText, nil
}

func showInputFileError(status *pasteStatus, err error) {
	if status == nil || err == nil {
		return
	}
	status.view.SetTextColor(colorMuted)
	status.view.SetText("File read failed: " + err.Error())
	if status.focus != nil {
		status.focus()
	}
}

func pastedFilePath(text string) (string, bool, error) {
	path := strings.TrimSpace(text)
	if path == "" || strings.ContainsAny(path, "\n\r\t") {
		return "", false, nil
	}
	quoted := false
	if len(path) >= 1 {
		first := path[0]
		if first == '"' || first == '\'' {
			if len(path) < 2 || path[len(path)-1] != first {
				return "", true, fmt.Errorf("file path quote is not closed")
			}
			quoted = true
			path = strings.TrimSpace(path[1 : len(path)-1])
		}
	}
	if path == "" {
		return "", false, nil
	}
	if strings.HasPrefix(strings.ToLower(path), "file://") {
		u, err := url.Parse(path)
		if err != nil {
			return "", false, fmt.Errorf("parse dropped file path: %w", err)
		}
		path = u.Path
		if runtime.GOOS == "windows" && strings.HasPrefix(path, "/") && len(path) >= 3 && path[2] == ':' {
			path = path[1:]
		}
		if u.Host != "" && runtime.GOOS == "windows" {
			path = `\\` + u.Host + filepath.FromSlash(path)
		}
	}
	path = filepath.Clean(filepath.FromSlash(path))
	looksLikePath := quoted || filepath.IsAbs(path) || strings.ContainsAny(path, `\/`)
	info, err := os.Stat(path)
	if err != nil {
		if !looksLikePath && (os.IsNotExist(err) || os.IsPermission(err)) {
			return "", false, nil
		}
		return path, true, err
	}
	if info.IsDir() {
		return "", false, fmt.Errorf("dropped path is a folder, not a text file")
	}
	return path, true, nil
}

func sanitizePastedText(text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("clipboard is empty")
	}
	if !utf8.ValidString(text) {
		return "", fmt.Errorf("clipboard does not contain valid UTF-8 text")
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = stripANSIEscapeSequences(text)

	var builder strings.Builder
	builder.Grow(len(text))
	for _, r := range text {
		switch {
		case r == '\n' || r == '\t':
			builder.WriteRune(r)
		case r >= ' ' && r != 0x7f:
			builder.WriteRune(r)
		case r == '\uFEFF' || r == '\u200B' || r == '\u200C' || r == '\u200D':
			continue
		default:
			return "", fmt.Errorf("clipboard contains non-text control characters")
		}
	}
	cleaned := strings.TrimRight(builder.String(), "\n")
	if strings.TrimSpace(cleaned) == "" {
		return "", fmt.Errorf("clipboard has no pasteable text")
	}
	return cleaned, nil
}

func stripANSIEscapeSequences(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if runes[i] != 0x1b {
			builder.WriteRune(runes[i])
			continue
		}
		i++
		if i >= len(runes) {
			break
		}
		if runes[i] == '[' {
			for i+1 < len(runes) {
				i++
				if runes[i] >= '@' && runes[i] <= '~' {
					break
				}
			}
			continue
		}
	}
	return builder.String()
}

func conciseActionLabel(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	lower := strings.ToLower(value)
	for _, prefix := range []string{"use ", "select ", "choose "} {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(value[len(prefix):])
		}
	}
	switch lower {
	case "continue", "confirm", "next":
		return fallback
	case "done all":
		return ButtonExportAll
	case "generate file":
		return ButtonExport
	case "generate all":
		return ButtonExportAll
	case "write txt":
		return "Save txt"
	default:
		return value
	}
}

func shortcutMatchesEvent(shortcut string, event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	shortcut = strings.ToLower(strings.TrimSpace(shortcut))
	if shortcut == "" {
		return false
	}
	if strings.HasPrefix(shortcut, "ctrl+") {
		name := strings.TrimPrefix(shortcut, "ctrl+")
		if event.Modifiers()&tcell.ModCtrl != 0 && event.Key() == tcell.KeyRune {
			return strings.EqualFold(string(event.Rune()), name)
		}
		switch name {
		case "a":
			return event.Key() == tcell.KeyCtrlA
		case "d":
			return event.Key() == tcell.KeyCtrlD
		case "g":
			return event.Key() == tcell.KeyCtrlG
		case "h":
			return event.Key() == tcell.KeyCtrlH
		case "j":
			return event.Key() == tcell.KeyCtrlJ
		case "k":
			return event.Key() == tcell.KeyCtrlK
		case "l":
			return event.Key() == tcell.KeyCtrlL
		case "n":
			return event.Key() == tcell.KeyCtrlN
		case "o":
			return event.Key() == tcell.KeyCtrlO
		case "r":
			return event.Key() == tcell.KeyCtrlR
		case "u":
			return event.Key() == tcell.KeyCtrlU
		case "v":
			return event.Key() == tcell.KeyCtrlV
		case "w":
			return event.Key() == tcell.KeyCtrlW
		default:
			return false
		}
	}
	if shortcut == "f1" {
		return event.Key() == tcell.KeyF1
	}
	if strings.HasPrefix(shortcut, "alt+") {
		return false
	}
	if event.Key() != tcell.KeyRune {
		return false
	}
	key := strings.ToLower(string(event.Rune()))
	if key == "" {
		return false
	}
	return shortcut == key
}

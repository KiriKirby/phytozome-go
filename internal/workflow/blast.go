// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package workflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/blastplus"
	"github.com/KiriKirby/phytozome-go/internal/export"
	"github.com/KiriKirby/phytozome-go/internal/interpro"
	"github.com/KiriKirby/phytozome-go/internal/labelname"
	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/phytozome"
	"github.com/KiriKirby/phytozome-go/internal/progressctx"
	"github.com/KiriKirby/phytozome-go/internal/prompt"
	"github.com/KiriKirby/phytozome-go/internal/report"
	"github.com/KiriKirby/phytozome-go/internal/source"
	"github.com/KiriKirby/phytozome-go/internal/tui"
	"github.com/KiriKirby/phytozome-go/internal/uniprot"

	"golang.org/x/sync/singleflight"
)

type BlastWizard struct {
	httpClient             *http.Client
	source                 source.DataSource
	prompt                 *prompt.Prompter
	out                    io.Writer
	tuiInfo                tui.StartupInfo
	blastProgramPath       string
	pendingMode            QueryMode
	postRunBackTarget      error
	reuseLastBlastInput    bool
	reuseLastBlastRows     bool
	lastBlastRowContext    *blastRowContext
	lastBlastReviewContext *blastReviewContext
	lastBlastItems         []blastQueryItem
	rewindBlastToInput     bool
	reuseLastKeywordRows   bool
	lastKeywordGroups      []model.KeywordSearchGroup
	lastKeywordReport      *keywordReportRunContext
	lastKeywordSpecies     model.SpeciesCandidate
	rewindKeywordToInput   bool
	suppressTaskModals     bool

	speciesCandidatesMu    sync.Mutex
	speciesCandidatesCache map[string][]model.SpeciesCandidate

	blastLabelLookupMu    sync.Mutex
	blastLabelLookupCache map[string]blastAutoLabelResult

	blastHitLabelLookupMu    sync.RWMutex
	blastHitLabelLookupCache map[string]blastHitLabelIdentification

	uniProtClientMu sync.Mutex
	uniProtClient   *uniprot.Client

	interProClientMu sync.Mutex
	interProClient   *interpro.Client

	rowUniProtAccessionsMu    sync.Mutex
	rowUniProtAccessionsCache map[string][]string
	rowUniProtAccessionsKnown map[string]bool
	rowUniProtAccessionsGroup singleflight.Group

	uniProtLookupMu    sync.RWMutex
	uniProtLookupCache map[string]uniProtLookupResult

	interProLookupMu    sync.RWMutex
	interProLookupCache map[string]interProLookupResult

	keywordBlastItemMu    sync.RWMutex
	keywordBlastItemCache map[string]blastQueryItem

	querySourceResolveMu    sync.RWMutex
	querySourceResolveCache map[string]model.QuerySequenceSource

	keywordTermRowsMu    sync.RWMutex
	keywordTermRowsCache map[string][]model.KeywordResultRow
	keywordTermRowsGroup singleflight.Group

	proteinSequenceMu    sync.RWMutex
	proteinSequenceCache map[string]model.ProteinSequenceData
	proteinSequenceMiss  map[string]error
	proteinSequenceGroup singleflight.Group
}

type TUIInfo = tui.StartupInfo

const (
	maxParallelKeywordJobs  = 64
	maxParallelQueryJobs    = 64
	maxParallelFetchJobs    = 64
	maxParallelUniProtJobs  = 96
	maxParallelInterProJobs = 96
)

var (
	familySemanticTokenPattern           = regexp.MustCompile(`[A-Za-z0-9']+`)
	familyTargetTranscriptSuffixPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)_t\d+$`),
		regexp.MustCompile(`(?i)[._-]t\d+$`),
		regexp.MustCompile(`(?i)\.\d+$`),
	}
)

type wideKeywordSearcher interface {
	SearchKeywordRowsWide(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error)
}

type nucleotideSequenceResolver interface {
	FetchNucleotideSequence(ctx context.Context, targetID int, sequenceID string, program string) (model.ProteinSequenceData, error)
}

type keywordSearchResult struct {
	index   int
	started time.Time
	ended   time.Time
	rows    []model.KeywordResultRow
	err     error
}

type keywordSearchRecoveryError struct {
	Result  keywordSearchResult
	Keyword string
	Index   int
	Total   int
	Err     error
}

func (e *keywordSearchRecoveryError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("keyword %d/%d (%s): %v", e.Index+1, e.Total, e.Keyword, e.Err)
}

func (e *keywordSearchRecoveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func isKeywordSearchControlError(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, tui.ErrTaskCancelled) ||
		errors.Is(err, prompt.ErrBackToQueryInput) ||
		errors.Is(err, prompt.ErrBackToSpeciesSelection) ||
		errors.Is(err, prompt.ErrBackToModeSelection) ||
		errors.Is(err, prompt.ErrBackToDatabaseSelection) ||
		errors.Is(err, prompt.ErrExitRequested)
}

type recoveryDecision int

const (
	recoveryRetry recoveryDecision = iota + 1
	recoverySkip
	recoveryBack
	recoveryExit
)

func isMissingProteinSequenceError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "no protein sequence") {
		return true
	}
	if strings.Contains(message, "protein sequence response empty") {
		return true
	}
	return strings.Contains(message, "no lemna.org protein sequence matched")
}

type QueryMode string

const (
	ModeBlast   QueryMode = "blast"
	ModeKeyword QueryMode = "keyword"
)

type blastQueryItem struct {
	RawInput            string
	LabelName           string
	Sequence            string
	ProteinSequence     string
	NucleotideSequence  string
	QuerySource         *model.QuerySequenceSource
	FromKeyword         bool
	FamilyName          string
	MemberLabel         string
	FamilyGroupSource   string
	FamilyDetectionRule string
	FamilySources       []*model.QuerySequenceSource
	FamilySettings      model.FamilyBlastSettings
}

type blastBatchSettings struct {
	OutputDir      string
	ApproveAll     bool
	ReportPath     string
	AutoMode       bool
	AutoSelections bool
}

type blastQueryRun struct {
	Index           int
	Item            blastQueryItem
	Request         model.BlastRequest
	Results         model.BlastResult
	SelectedRows    []model.BlastResultRow
	ExcelPath       string
	TextPath        string
	RowsBeforeMerge int
	RowsAfterMerge  int
}

type exportSettings struct {
	BaseName      string
	OutputDir     string
	WriteReport   bool
	WriteText     bool
	WriteExcel    bool
	WriteRawExcel bool
	UsePhgoHeader bool
}

type exportFileResult struct {
	ExcelPath       string
	TextPath        string
	RawTextPath     string
	RawExcelPath    string
	ReportPath      string
	Steps           []report.GenerationStep
	SequenceAudit   report.SequenceAudit
	SequenceRecords []model.ProteinSequenceRecord
}

type blastBatchExportResult struct {
	Runs             []blastQueryRun
	Files            []exportFileResult
	RowsByRun        [][]model.BlastResultRow
	RowNumbersByRun  [][]int
	FilterFlagsByRun [][]bool
	SelectedByRun    [][]bool
}

type blastExportJob struct {
	exportIndex      int
	runPosition      int
	run              blastQueryRun
	rows             []model.BlastResultRow
	rowNumbers       []int
	filterFlags      []bool
	selectedRowsMask []bool
	displayName      string
	filePrefix       string
	txtHeaderLabel   string
}

type keywordReportRunContext struct {
	Selected      model.SpeciesCandidate
	QueryStarted  time.Time
	SearchEnded   time.Time
	ReviewStarted time.Time
	LabelMode     string
}

type blastRowContext struct {
	Rows             []model.BlastResultRow
	AllRows          []model.BlastResultRow
	Numbers          []int
	Flags            []bool
	SelectedRowsMask []bool
	Item             blastQueryItem
	Selected         model.SpeciesCandidate
	Request          model.BlastRequest
	Results          model.BlastResult
	Index            int
	FilterSettings   model.BlastFilterSettings
	FilterApplied    bool
	FilterCleared    bool
	FamilySettings   model.FamilyBlastSettings
}

type blastReviewContext struct {
	Selected          model.SpeciesCandidate
	Prepared          []blastQueryItem
	OriginalPrepared  []blastQueryItem
	Runs              []blastQueryRun
	OriginalRuns      []blastQueryRun
	ConfiguredRequest model.BlastRequest
	OriginalRunCount  int
}

type blastRequestConfig struct {
	Request model.BlastRequest
	Ready   bool
}

type externalReferenceConfig struct {
	AutoLabelBlastHits bool
	UseUniProt         bool
	UseInterPro        bool
	InterProSettings   model.InterProConservedRegionSettings
}

type familyBlastPlan struct {
	Settings model.FamilyBlastSettings
	Groups   []familyBlastGroup
}

type familyBlastGroup struct {
	Name          string
	Indexes       []int
	Labels        []string
	Members       []familyBlastMember
	GroupSource   string
	DetectionRule string
}

type familyBlastMember struct {
	LabelName         string
	ProteinID         string
	Aliases           []string
	OriginalLabelName string
	SourceKey         string
}

type sequenceFetchResult struct {
	data model.ProteinSequenceData
	err  error
}

type uniProtLookupResult struct {
	entry uniprot.Entry
	ok    bool
	err   error
}

type interProLookupResult struct {
	entry interpro.Entry
	ok    bool
	err   error
}

type contextUpdateKey struct{}
type blastReferenceConfigContextKey struct{}

func contextWithUpdate(ctx context.Context, update func(int, string)) context.Context {
	if update == nil {
		return ctx
	}
	ctx = context.WithValue(ctx, contextUpdateKey{}, update)
	return progressctx.WithProgress(ctx, update)
}

func updateFromContext(ctx context.Context) func(int, string) {
	if ctx == nil {
		return nil
	}
	if update, ok := ctx.Value(contextUpdateKey{}).(func(int, string)); ok {
		return updateWithContext(ctx, update)
	}
	return nil
}

func updateWithContext(ctx context.Context, update func(int, string)) func(int, string) {
	return func(current int, message string) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if update != nil {
			update(current, message)
		}
	}
}

func contextWithBlastReferenceConfig(ctx context.Context, config externalReferenceConfig) context.Context {
	return context.WithValue(ctx, blastReferenceConfigContextKey{}, config)
}

func blastReferenceConfigFromContext(ctx context.Context) externalReferenceConfig {
	if ctx == nil {
		return externalReferenceConfig{}
	}
	config, _ := ctx.Value(blastReferenceConfigContextKey{}).(externalReferenceConfig)
	return config
}

func safeProgress(update func(int, string)) func(int, string) {
	return func(current int, message string) {
		if update != nil {
			update(current, message)
		}
	}
}

func safeTaskUpdate(update func(string)) func(string) {
	return func(message string) {
		if update != nil {
			update(message)
		}
	}
}

func mergeContexts(parent context.Context, cancel context.Context) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	if cancel == nil {
		return parent
	}
	ctx, stop := context.WithCancel(parent)
	go func() {
		select {
		case <-parent.Done():
		case <-cancel.Done():
			stop()
		case <-ctx.Done():
		}
	}()
	return ctx
}

type blastBatchRunError struct {
	Stage string
	Index int
	Total int
	Label string
	Err   error
}

func (e *blastBatchRunError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("BLAST query %d/%d (%s): %s failed: %v", e.Index, e.Total, e.Label, e.Stage, e.Err)
}

func (e *blastBatchRunError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type blastBatchResolveFailure struct {
	Index int
	Total int
	Label string
	Err   error
}

type blastBatchResolveError struct {
	Total    int
	Prepared []blastQueryItem
	Failures []blastBatchResolveFailure
}

func (e *blastBatchResolveError) Error() string {
	if e == nil || len(e.Failures) == 0 {
		return ""
	}
	failure := e.Failures[0]
	if len(e.Failures) == 1 {
		return fmt.Sprintf("resolve BLAST query %d/%d (%s): %v", failure.Index, failure.Total, failure.Label, failure.Err)
	}
	total := e.Total
	if total <= 0 {
		total = len(e.Prepared) + len(e.Failures)
	}
	return fmt.Sprintf("resolve BLAST queries: %d of %d queries could not be resolved; first failure was query %d/%d (%s): %v", len(e.Failures), total, failure.Index, failure.Total, failure.Label, failure.Err)
}

func (e *blastBatchResolveError) Unwrap() error {
	if e == nil || len(e.Failures) == 0 {
		return nil
	}
	return e.Failures[0].Err
}

type blastBatchExportError struct {
	Run   blastQueryRun
	Label string
	Err   error
}

func (e *blastBatchExportError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("BLAST query %d (%s): export failed: %v", e.Run.Index, e.Label, e.Err)
}

func (e *blastBatchExportError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NewBlastWizard(out io.Writer) *BlastWizard {
	return NewBlastWizardWithTUIInfo(out, tui.StartupInfo{})
}

func NewBlastWizardWithTUIInfo(out io.Writer, tuiInfo tui.StartupInfo) *BlastWizard {
	w := &BlastWizard{
		httpClient:                defaultHTTPClient(),
		prompt:                    prompt.New(os.Stdin, out),
		out:                       out,
		tuiInfo:                   tuiInfo,
		speciesCandidatesCache:    make(map[string][]model.SpeciesCandidate),
		blastLabelLookupCache:     make(map[string]blastAutoLabelResult),
		blastHitLabelLookupCache:  make(map[string]blastHitLabelIdentification),
		rowUniProtAccessionsCache: make(map[string][]string),
		rowUniProtAccessionsKnown: make(map[string]bool),
		uniProtLookupCache:        make(map[string]uniProtLookupResult),
		interProLookupCache:       make(map[string]interProLookupResult),
		keywordBlastItemCache:     make(map[string]blastQueryItem),
		querySourceResolveCache:   make(map[string]model.QuerySequenceSource),
		keywordTermRowsCache:      make(map[string][]model.KeywordResultRow),
		proteinSequenceCache:      make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:       make(map[string]error),
	}
	w.prompt.SetDetailLoaders(w.loadKeywordDetailFASTA, w.loadBlastDetailFASTA)
	return w
}

func (w *BlastWizard) Run(ctx context.Context) error {
databaseLoop:
	for {
		dataSource, err := w.chooseDataSource()
		if errors.Is(err, prompt.ErrExitRequested) {
			return nil
		}
		if errors.Is(err, prompt.ErrBackToDatabaseSelection) {
			continue
		}
		if err != nil {
			return err
		}
		w.source = dataSource
		w.prompt.SetDatabaseContext(databaseDisplayName(w.source.Name()))
		w.setBlastProgramContext("")

		candidates, err := w.loadSpeciesCandidates(ctx)
		if errors.Is(err, prompt.ErrExitRequested) {
			return nil
		}
		if errors.Is(err, prompt.ErrBackToDatabaseSelection) {
			continue databaseLoop
		}
		if err != nil {
			return err
		}

	modeLoop:
		for {
			mode, err := w.chooseMode()
			if errors.Is(err, prompt.ErrExitRequested) {
				return nil
			}
			if errors.Is(err, prompt.ErrBackToDatabaseSelection) {
				continue databaseLoop
			}
			if err != nil {
				return err
			}
			if mode != ModeBlast {
				w.setBlastProgramContext("")
			}

			selected := model.SpeciesCandidate{}
			needSelect := true

		speciesLoop:
			for {
				if needSelect || selected.JBrowseName == "" {
					selected, err = w.selectSpecies(candidates)
					if errors.Is(err, prompt.ErrExitRequested) {
						return nil
					}
					if errors.Is(err, prompt.ErrBackToDatabaseSelection) {
						continue databaseLoop
					}
					if errors.Is(err, prompt.ErrBackToModeSelection) {
						continue modeLoop
					}
					if err != nil {
						return err
					}
				}

				switch mode {
				case ModeBlast:
					if err := w.runBlastMode(ctx, selected, candidates); err != nil {
						switch classifyWizardBack(err) {
						case wizardBackExit:
							return nil
						case wizardBackDatabase:
							continue databaseLoop
						case wizardBackMode:
							continue modeLoop
						case wizardBackSpecies:
							selected = model.SpeciesCandidate{}
							needSelect = true
							continue speciesLoop
						case wizardBackBlastProgram:
							w.reuseLastBlastInput = len(w.lastBlastItems) > 0
							needSelect = false
							continue speciesLoop
						case wizardBackQuery:
							w.rewindKeywordToInput = mode == ModeKeyword
							w.rewindBlastToInput = mode == ModeBlast
							needSelect = false
							continue speciesLoop
						case wizardBackRows:
							w.reuseLastBlastRows = w.lastBlastRowContext != nil
							needSelect = false
							continue speciesLoop
						}
						return err
					}
				case ModeKeyword:
					if err := w.runKeywordMode(ctx, selected); err != nil {
						switch classifyWizardBack(err) {
						case wizardBackExit:
							return nil
						case wizardBackDatabase:
							continue databaseLoop
						case wizardBackMode:
							continue modeLoop
						case wizardBackSpecies:
							selected = model.SpeciesCandidate{}
							needSelect = true
							continue speciesLoop
						case wizardBackQuery:
							w.rewindKeywordToInput = true
							needSelect = false
							continue speciesLoop
						case wizardBackRows:
							w.reuseLastKeywordRows = len(w.lastKeywordGroups) > 0
							needSelect = false
							continue speciesLoop
						}
						return err
					}
				default:
					return fmt.Errorf("unsupported mode %q", mode)
				}

				for {
					action, err := w.prompt.PostRunAction(string(mode), w.isLemnaSource(), w.postRunBackTarget)
					if errors.Is(err, prompt.ErrExitRequested) {
						return nil
					}
					if errors.Is(err, prompt.ErrBackToDatabaseSelection) {
						continue databaseLoop
					}
					if errors.Is(err, prompt.ErrBackToModeSelection) {
						continue modeLoop
					}
					if errors.Is(err, prompt.ErrBackToSpeciesSelection) {
						selected = model.SpeciesCandidate{}
						needSelect = true
						continue speciesLoop
					}
					if errors.Is(err, prompt.ErrBackToBlastProgram) {
						w.reuseLastBlastInput = mode == ModeBlast && len(w.lastBlastItems) > 0
						needSelect = false
						continue speciesLoop
					}
					if errors.Is(err, prompt.ErrBackToRowSelection) {
						w.reuseLastKeywordRows = mode == ModeKeyword && len(w.lastKeywordGroups) > 0
						w.reuseLastBlastRows = mode == ModeBlast && w.lastBlastRowContext != nil
						needSelect = false
						continue speciesLoop
					}
					if errors.Is(err, prompt.ErrBackToQueryInput) {
						w.rewindKeywordToInput = mode == ModeKeyword
						w.rewindBlastToInput = mode == ModeBlast
						needSelect = false
						continue speciesLoop
					}
					if err != nil {
						return err
					}

					switch action {
					case "stay":
						w.rewindModeToInput(mode)
						needSelect = false
						continue speciesLoop
					case "change_query":
						needSelect = false
						continue speciesLoop
					case "change_species":
						selected = model.SpeciesCandidate{}
						needSelect = true
						continue speciesLoop
					case "change_mode":
						continue modeLoop
					case "exit":
						return nil
					default:
						w.rewindModeToInput(mode)
						needSelect = false
						continue speciesLoop
					}
				}
			}
		}
	}
}

func (w *BlastWizard) chooseMode() (QueryMode, error) {
	for {
		if w.pendingMode != "" {
			mode := w.pendingMode
			w.pendingMode = ""
			return mode, nil
		}
		return "", prompt.ErrBackToDatabaseSelection
	}
}

func (w *BlastWizard) chooseDataSource() (source.DataSource, error) {
	for {
		choice, err := tui.SelectStartup(os.Stdin, w.out, w.tuiInfo)
		if err != nil {
			return nil, err
		}
		if choice.Tool != "" {
			if err := w.runStartupTool(choice.Tool); err != nil {
				if errors.Is(err, prompt.ErrBackToDatabaseSelection) {
					continue
				}
				return nil, err
			}
			continue
		}

		w.pendingMode = QueryMode(choice.Mode)
		switch choice.Database {
		case "phytozome":
			return phytozome.NewClient(w.httpClient), nil
		case "lemna":
			return lemna.NewClient(w.httpClient), nil
		default:
			return nil, fmt.Errorf("unsupported database %q", choice.Database)
		}
	}
}

func (w *BlastWizard) runStartupTool(tool string) error {
	switch strings.TrimSpace(tool) {
	case "pathway_search":
		return w.showInfo(
			"Pathway search",
			"Pathway search is reserved as the entry point for pathway-guided protein discovery.\n\nPlanned sources: Plant Reactome, PlantCyc, MetaCyc, UniProt, and InterPro.\n\nThis placeholder is active now; the implementation will be added step by step.",
			prompt.ErrBackToDatabaseSelection,
		)
	default:
		return fmt.Errorf("unsupported startup tool %q", tool)
	}
}

func (w *BlastWizard) isLemnaSource() bool {
	_, ok := w.source.(*lemna.Client)
	return ok
}

func (w *BlastWizard) setBlastProgramContext(program string) {
	w.blastProgramPath = strings.TrimSpace(program)
	w.prompt.SetBlastProgramContext(w.blastProgramPath)
}

func (w *BlastWizard) tuiPath(parts ...string) []string {
	path := []string{"phytozome GO"}
	if w.source != nil {
		if database := databaseDisplayName(w.source.Name()); strings.TrimSpace(database) != "" {
			path = append(path, database)
		}
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			path = append(path, part)
		}
	}
	return path
}

type wizardBackAction int

const (
	wizardBackNone wizardBackAction = iota
	wizardBackExit
	wizardBackDatabase
	wizardBackMode
	wizardBackSpecies
	wizardBackQuery
	wizardBackBlastProgram
	wizardBackRows
)

func classifyWizardBack(err error) wizardBackAction {
	switch {
	case err == nil:
		return wizardBackNone
	case errors.Is(err, prompt.ErrExitRequested):
		return wizardBackExit
	case errors.Is(err, tui.ErrTaskCancelled):
		return wizardBackQuery
	case errors.Is(err, prompt.ErrBackToDatabaseSelection):
		return wizardBackDatabase
	case errors.Is(err, prompt.ErrBackToModeSelection):
		return wizardBackMode
	case errors.Is(err, prompt.ErrBackToSpeciesSelection):
		return wizardBackSpecies
	case errors.Is(err, prompt.ErrBackToQueryInput):
		return wizardBackQuery
	case errors.Is(err, prompt.ErrBackToBlastProgram):
		return wizardBackBlastProgram
	case errors.Is(err, prompt.ErrBackToRowSelection):
		return wizardBackRows
	default:
		return wizardBackNone
	}
}

func (w *BlastWizard) consumeKeywordInputRewind() {
	if !w.rewindKeywordToInput {
		return
	}
	w.rewindKeywordToInput = false
	w.reuseLastKeywordRows = false
	w.lastKeywordReport = nil
}

func (w *BlastWizard) rewindKeywordRowsToInput() {
	w.rewindKeywordToInput = true
	w.reuseLastKeywordRows = false
	w.lastKeywordReport = nil
}

func (w *BlastWizard) consumeBlastInputRewind() {
	if !w.rewindBlastToInput {
		return
	}
	w.rewindBlastToInput = false
	w.reuseLastBlastInput = false
	w.reuseLastBlastRows = false
}

func (w *BlastWizard) rewindModeToInput(mode QueryMode) {
	switch mode {
	case ModeBlast:
		w.rewindBlastToInput = true
	case ModeKeyword:
		w.rewindKeywordToInput = true
	}
}

func (w *BlastWizard) configureBlastRequest(ctx context.Context, selected model.SpeciesCandidate, baseRequest model.BlastRequest) (model.BlastRequest, error) {
	request := baseRequest
	lc, ok := w.source.(*lemna.Client)
	if !ok {
		return request, nil
	}

	cap, err := w.detectLemnaBlastCapabilities(ctx, lc, selected, "Preparing BLAST program selection")
	if err != nil {
		return model.BlastRequest{}, err
	}
	progs := availableBlastProgramsFromCapability(cap)
	if len(progs) == 0 {
		return model.BlastRequest{}, fmt.Errorf("no BLAST programs are available for %s based on detected lemna.org capabilities", selected.DisplayLabel())
	}
	chosenProg, err := w.prompt.ChooseBlastProgram(progs)
	if err != nil {
		return model.BlastRequest{}, err
	}

	applyBlastProgram(&request, chosenProg)
	execChoice, err := w.chooseLemnaBlastExecution(cap, selected, chosenProg)
	if err != nil {
		return model.BlastRequest{}, err
	}
	if execChoice == "local" {
		request.Program = "local:" + request.Program
	}
	w.setBlastProgramContext(blastProgramPathLabel(request.Program))
	return request, nil
}

func (w *BlastWizard) configureBlastRequestBeforeInput(ctx context.Context, selected model.SpeciesCandidate) (blastRequestConfig, error) {
	if _, ok := w.source.(*lemna.Client); !ok {
		w.setBlastProgramContext("")
		return blastRequestConfig{}, nil
	}
	request, err := w.configureBlastRequest(ctx, selected, model.BlastRequest{
		Species:          selected,
		SequenceKind:     model.SequenceDNA,
		TargetType:       "genome",
		Program:          "BLASTN",
		EValue:           "-1",
		ComparisonMatrix: "BLOSUM62",
		WordLength:       "default",
		AlignmentsToShow: 100,
		AllowGaps:        true,
		FilterQuery:      true,
	})
	if err != nil {
		return blastRequestConfig{}, err
	}
	return blastRequestConfig{Request: request, Ready: true}, nil
}

func applyBlastProgram(request *model.BlastRequest, program string) {
	switch strings.ToLower(strings.TrimSpace(program)) {
	case "blastn":
		request.Program = "BLASTN"
		request.SequenceKind = model.SequenceDNA
		request.TargetType = "genome"
	case "blastx":
		request.Program = "BLASTX"
		request.SequenceKind = model.SequenceDNA
		request.TargetType = "proteome"
	case "tblastn":
		request.Program = "TBLASTN"
		request.SequenceKind = model.SequenceProtein
		request.TargetType = "genome"
	case "blastp":
		request.Program = "BLASTP"
		request.SequenceKind = model.SequenceProtein
		request.TargetType = "proteome"
	}
}

func blastProgramPathLabel(program string) string {
	program = strings.TrimSpace(program)
	if program == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(program), "local:") {
		return "local " + strings.ToUpper(strings.TrimSpace(program[len("local:"):]))
	}
	return strings.ToUpper(program)
}

func (w *BlastWizard) detectLemnaBlastCapabilities(ctx context.Context, lc *lemna.Client, selected model.SpeciesCandidate, title string) (lemna.BlastCapability, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Checking BLAST availability"
	}
	return tui.RunTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "Capability check"),
		Title:       title,
		Description: "Checking lemna.org server databases and local FASTA downloads for the selected species.",
		Initial:     fmt.Sprintf("Checking BLAST availability for %s...", selected.DisplayLabel()),
		CancelError: prompt.ErrBackToQueryInput,
	}, func(taskCtx context.Context, update func(string)) (lemna.BlastCapability, error) {
		safeTaskUpdate(update)("Checking online BLAST databases and local FASTA files...")
		return lc.DetectBlastCapabilities(mergeContexts(ctx, taskCtx), selected)
	})
}

func availableBlastProgramsFromCapability(cap lemna.BlastCapability) []string {
	progs := make([]string, 0, 4)
	if cap.ServerBlastNAvailable || cap.HasNucleotideFasta {
		progs = append(progs, "blastn")
	}
	if cap.ServerBlastXAvailable || cap.HasProteinFasta {
		progs = append(progs, "blastx")
	}
	if cap.ServerTBlastNAvailable || cap.HasNucleotideFasta {
		progs = append(progs, "tblastn")
	}
	if cap.ServerBlastPAvailable || cap.HasProteinFasta {
		progs = append(progs, "blastp")
	}
	return progs
}

func (w *BlastWizard) chooseLemnaBlastExecution(cap lemna.BlastCapability, selected model.SpeciesCandidate, program string) (string, error) {
	serverOK := false
	localOK := false
	switch strings.ToLower(strings.TrimSpace(program)) {
	case "blastn":
		serverOK = cap.ServerBlastNAvailable
		localOK = cap.HasNucleotideFasta
	case "tblastn":
		serverOK = cap.ServerTBlastNAvailable
		localOK = cap.HasNucleotideFasta
	case "blastx":
		serverOK = cap.ServerBlastXAvailable
		localOK = cap.HasProteinFasta
	case "blastp":
		serverOK = cap.ServerBlastPAvailable
		localOK = cap.HasProteinFasta
	}

	if serverOK {
		return "server", nil
	}
	if localOK {
		return "local", nil
	}
	return "", fmt.Errorf("no server or local execution target is available for %s on %s", program, selected.DisplayLabel())
}

func (w *BlastWizard) loadSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {
	for {
		label := fmt.Sprintf("Loading species candidates from %s...", w.source.Name())
		candidates, err := tui.RunTaskValueContext(tui.TaskPage{
			Path:        w.tuiPath("Startup", "Species"),
			Title:       "Loading species",
			Description: "Fetching available species candidates for the selected database.",
			Initial:     label,
			CancelError: prompt.ErrBackToDatabaseSelection,
		}, func(taskCtx context.Context, update func(string)) ([]model.SpeciesCandidate, error) {
			safeTaskUpdate(update)(label)
			return w.source.FetchSpeciesCandidates(mergeContexts(ctx, taskCtx))
		})
		if err == nil {
			w.cacheSpeciesCandidates(w.source.Name(), candidates)
			return candidates, nil
		}
		if errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) || errors.Is(err, tui.ErrTaskCancelled) {
			return nil, err
		}
		retry, navErr := w.retryWorkflowStep(fmt.Sprintf("load species candidates: %v", err), prompt.ErrBackToDatabaseSelection)
		if navErr != nil {
			return nil, navErr
		}
		if !retry {
			return nil, err
		}
	}
}

func (w *BlastWizard) cacheSpeciesCandidates(sourceName string, candidates []model.SpeciesCandidate) {
	w.speciesCandidatesMu.Lock()
	defer w.speciesCandidatesMu.Unlock()
	if w.speciesCandidatesCache == nil {
		w.speciesCandidatesCache = make(map[string][]model.SpeciesCandidate)
	}
	copyCandidates := make([]model.SpeciesCandidate, len(candidates))
	copy(copyCandidates, candidates)
	w.speciesCandidatesCache[strings.ToLower(strings.TrimSpace(sourceName))] = copyCandidates
}

func (w *BlastWizard) blastLabelLookupKey(src source.DataSource, species model.SpeciesCandidate, item blastQueryItem) string {
	sourceName := ""
	if src != nil {
		sourceName = src.Name()
	}
	terms := blastLabelSearchTerms(item)
	sort.Strings(terms)
	sourceDatabase := ""
	sourceProteomeID := 0
	sourceJBrowseName := ""
	sourceGenomeLabel := ""
	if item.QuerySource != nil {
		sourceDatabase = item.QuerySource.SourceDatabase
		sourceProteomeID = item.QuerySource.SourceProteomeID
		sourceJBrowseName = item.QuerySource.SourceJBrowseName
		sourceGenomeLabel = item.QuerySource.SourceGenomeLabel
	}
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(sourceName)),
		strconv.Itoa(species.ProteomeID),
		strings.ToLower(strings.TrimSpace(species.JBrowseName)),
		strings.ToLower(strings.TrimSpace(species.GenomeLabel)),
		strings.ToLower(strings.TrimSpace(sourceDatabase)),
		strconv.Itoa(sourceProteomeID),
		strings.ToLower(strings.TrimSpace(sourceJBrowseName)),
		strings.ToLower(strings.TrimSpace(sourceGenomeLabel)),
		strings.Join(terms, "\x00"),
	}, "|")
}

func (w *BlastWizard) cachedBlastLabelLookup(src source.DataSource, species model.SpeciesCandidate, item blastQueryItem) (blastAutoLabelResult, bool) {
	key := w.blastLabelLookupKey(src, species, item)
	w.blastLabelLookupMu.Lock()
	defer w.blastLabelLookupMu.Unlock()
	if w.blastLabelLookupCache == nil {
		w.blastLabelLookupCache = make(map[string]blastAutoLabelResult)
	}
	result, ok := w.blastLabelLookupCache[key]
	return result, ok
}

func (w *BlastWizard) storeBlastLabelLookup(src source.DataSource, species model.SpeciesCandidate, item blastQueryItem, result blastAutoLabelResult) {
	key := w.blastLabelLookupKey(src, species, item)
	w.blastLabelLookupMu.Lock()
	defer w.blastLabelLookupMu.Unlock()
	if w.blastLabelLookupCache == nil {
		w.blastLabelLookupCache = make(map[string]blastAutoLabelResult)
	}
	result.Label = strings.TrimSpace(result.Label)
	result.Aliases = uniqueStrings(result.Aliases)
	w.blastLabelLookupCache[key] = result
}

func (w *BlastWizard) sharedUniProtClient() *uniprot.Client {
	w.uniProtClientMu.Lock()
	defer w.uniProtClientMu.Unlock()
	if w.uniProtClient == nil {
		w.uniProtClient = uniprot.NewClient(w.httpClient)
	}
	return w.uniProtClient
}

func (w *BlastWizard) sharedInterProClient() *interpro.Client {
	w.interProClientMu.Lock()
	defer w.interProClientMu.Unlock()
	if w.interProClient == nil {
		w.interProClient = interpro.NewClient(w.httpClient)
	}
	return w.interProClient
}

func blastRowAccessionCacheKey(row model.BlastResultRow) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(row.SourceDatabase)),
		strconv.Itoa(row.TargetID),
		strings.ToLower(strings.TrimSpace(row.JBrowseName)),
		strings.ToLower(strings.TrimSpace(row.UniProtAccession)),
		strings.ToLower(strings.TrimSpace(row.Protein)),
		strings.ToLower(strings.TrimSpace(row.SubjectID)),
		strings.ToLower(strings.TrimSpace(row.SequenceID)),
		strings.ToLower(strings.TrimSpace(row.TranscriptID)),
		strings.ToLower(strings.TrimSpace(row.GeneReportURL)),
	}
	return strings.Join(parts, "|")
}

func (w *BlastWizard) cachedRowUniProtAccessions(row model.BlastResultRow) ([]string, bool) {
	key := blastRowAccessionCacheKey(row)
	w.rowUniProtAccessionsMu.Lock()
	defer w.rowUniProtAccessionsMu.Unlock()
	if w.rowUniProtAccessionsCache == nil {
		w.rowUniProtAccessionsCache = make(map[string][]string)
	}
	if w.rowUniProtAccessionsKnown == nil {
		w.rowUniProtAccessionsKnown = make(map[string]bool)
	}
	if !w.rowUniProtAccessionsKnown[key] {
		return nil, false
	}
	return append([]string(nil), w.rowUniProtAccessionsCache[key]...), true
}

func (w *BlastWizard) storeRowUniProtAccessions(row model.BlastResultRow, accessions []string) {
	key := blastRowAccessionCacheKey(row)
	w.rowUniProtAccessionsMu.Lock()
	defer w.rowUniProtAccessionsMu.Unlock()
	if w.rowUniProtAccessionsCache == nil {
		w.rowUniProtAccessionsCache = make(map[string][]string)
	}
	if w.rowUniProtAccessionsKnown == nil {
		w.rowUniProtAccessionsKnown = make(map[string]bool)
	}
	w.rowUniProtAccessionsCache[key] = uniqueStrings(accessions)
	w.rowUniProtAccessionsKnown[key] = true
}

func (w *BlastWizard) speciesCandidatesForSource(ctx context.Context, src source.DataSource, current []model.SpeciesCandidate) ([]model.SpeciesCandidate, error) {
	key := strings.ToLower(strings.TrimSpace(src.Name()))
	if key == "" {
		return nil, fmt.Errorf("source name is empty")
	}
	if w.source != nil && key == strings.ToLower(strings.TrimSpace(w.source.Name())) && len(current) > 0 {
		w.cacheSpeciesCandidates(src.Name(), current)
		return current, nil
	}

	w.speciesCandidatesMu.Lock()
	if cached, ok := w.speciesCandidatesCache[key]; ok {
		copyCandidates := make([]model.SpeciesCandidate, len(cached))
		copy(copyCandidates, cached)
		w.speciesCandidatesMu.Unlock()
		return copyCandidates, nil
	}
	w.speciesCandidatesMu.Unlock()

	candidates, err := src.FetchSpeciesCandidates(ctx)
	if err != nil {
		return nil, err
	}
	w.cacheSpeciesCandidates(src.Name(), candidates)
	return candidates, nil
}

func (w *BlastWizard) selectSpecies(candidates []model.SpeciesCandidate) (model.SpeciesCandidate, error) {
	// If lemna source and the candidate list is small, present the full list directly.
	const smallListThreshold = 16

	for {
		// If running against lemna and the candidate list is small, avoid the search flow
		// and present the full numbered list for direct selection.
		if _, ok := w.source.(*lemna.Client); ok && len(candidates) <= smallListThreshold {
			selected, err := w.prompt.SelectSpecies(candidates)
			if err == nil {
				return selected, nil
			}
			if errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
				return model.SpeciesCandidate{}, err
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("select species: %v", err), prompt.ErrBackToModeSelection)
			if navErr != nil {
				return model.SpeciesCandidate{}, navErr
			}
			if !retry {
				return model.SpeciesCandidate{}, err
			}
			// If user chose to retry, continue the loop and re-show full list.
			continue
		}

		// Otherwise use the existing search-and-select flow and appropriate filter.
		selected, err := w.prompt.SearchAndSelectSpecies(candidates, func(keyword string) []model.SpeciesCandidate {
			if _, ok := w.source.(*lemna.Client); ok {
				return lemna.FilterSpeciesCandidates(candidates, keyword)
			}
			return phytozome.FilterSpeciesCandidates(candidates, keyword)
		})
		if err == nil {
			return selected, nil
		}
		if errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
			return model.SpeciesCandidate{}, err
		}
		retry, navErr := w.retryWorkflowStep(fmt.Sprintf("select species: %v", err), prompt.ErrBackToModeSelection)
		if navErr != nil {
			return model.SpeciesCandidate{}, navErr
		}
		if !retry {
			return model.SpeciesCandidate{}, err
		}
	}
}

func (w *BlastWizard) runKeywordMode(ctx context.Context, selected model.SpeciesCandidate) error {
keywordInputLoop:
	for {
		var groups []model.KeywordSearchGroup
		var reportCtx *keywordReportRunContext
		w.consumeKeywordInputRewind()
		if w.reuseLastKeywordRows && len(w.lastKeywordGroups) > 0 {
			groups = cloneKeywordSearchGroups(w.lastKeywordGroups)
			if w.lastKeywordReport != nil {
				copied := *w.lastKeywordReport
				reportCtx = &copied
			}
			w.reuseLastKeywordRows = false
		} else {
			keywordInput, inputErr := w.prompt.KeywordInput()
			if inputErr != nil {
				if errors.Is(inputErr, prompt.ErrBackToSpeciesSelection) || errors.Is(inputErr, prompt.ErrBackToModeSelection) || errors.Is(inputErr, prompt.ErrBackToDatabaseSelection) || errors.Is(inputErr, prompt.ErrExitRequested) {
					return inputErr
				}
				retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read keyword input: %v", inputErr), prompt.ErrBackToSpeciesSelection)
				if navErr != nil {
					return navErr
				}
				if !retry {
					return inputErr
				}
				continue
			}
			queryStarted := time.Now()
			keywords := parseKeywordTerms(keywordInput.Text)
			if len(keywords) == 0 {
				if err := w.showInfo("Keyword input", "Keyword input was empty. Please enter a keyword query.", prompt.ErrBackToSpeciesSelection); err != nil {
					return err
				}
				continue
			}
			autoIdentifyLabels := false
			manualLabels, labelErr := w.prompt.KeywordLabelNames(len(keywords), prompt.ErrBackToQueryInput)
			identifications := manualKeywordLabelIdentifications(manualLabels, len(keywords))
			if errors.Is(labelErr, prompt.ErrAutoIdentifyRequested) {
				autoIdentifyLabels = true
				labelErr = nil
			}
			if labelErr != nil {
				if errors.Is(labelErr, prompt.ErrBackToQueryInput) {
					continue keywordInputLoop
				}
				if errors.Is(labelErr, prompt.ErrBackToSpeciesSelection) || errors.Is(labelErr, prompt.ErrBackToModeSelection) || errors.Is(labelErr, prompt.ErrBackToDatabaseSelection) || errors.Is(labelErr, prompt.ErrExitRequested) {
					return labelErr
				}
				retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read label names: %v", labelErr), prompt.ErrBackToQueryInput)
				if navErr != nil {
					return navErr
				}
				if !retry {
					return labelErr
				}
				continue
			}

			var err error
			groups, err = w.searchKeywordGroups(ctx, selected, keywords, nil, keywordInput.WideSearch)
			if err != nil {
				if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
					return err
				}
				retry, navErr := w.retryWorkflowStep(fmt.Sprintf("search keyword results: %v", err), prompt.ErrBackToSpeciesSelection)
				if navErr != nil {
					return navErr
				}
				if !retry {
					return err
				}
				continue
			}
			if autoIdentifyLabels {
				identifications, err = w.autoIdentifyKeywordLabelsWithProgress(ctx, selected, groups)
				if err != nil {
					if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
						return err
					}
					retry, navErr := w.retryWorkflowStep(fmt.Sprintf("auto identify keyword labels: %v", err), prompt.ErrBackToQueryInput)
					if navErr != nil {
						return navErr
					}
					if !retry {
						return err
					}
					continue
				}
			}
			labelMode := "manual labels"
			if autoIdentifyLabels {
				labelMode = "auto-identify labels"
			}
			annotateKeywordLabelSources(groups, identifications, labelMode)
			if len(identifications) == len(keywords) {
				applyKeywordLabelIdentifications(groups, identifications)
				applyKeywordLabelMethod(groups, labelMode)
			}
			reportCtx = &keywordReportRunContext{
				Selected:     selected,
				QueryStarted: queryStarted,
				SearchEnded:  keywordGroupsSearchEndedAt(groups),
				LabelMode:    labelMode,
			}
		}

		totalRows := countKeywordRows(groups)
		if totalRows == 0 {
			w.postRunBackTarget = prompt.ErrBackToQueryInput
			if err := w.showInfo("Keyword results", fmt.Sprintf("No keyword results were found in %s.\n\nThese identifiers may belong to a different species or may not exist in this proteome.", selected.DisplayLabel()), prompt.ErrBackToQueryInput); err != nil {
				if errors.Is(err, prompt.ErrBackToQueryInput) {
					w.rewindKeywordRowsToInput()
					continue keywordInputLoop
				}
				return err
			}
			w.rewindKeywordRowsToInput()
			continue keywordInputLoop
		}
		w.lastKeywordGroups = cloneKeywordSearchGroups(groups)
		w.lastKeywordSpecies = selected
		if reportCtx != nil {
			copied := *reportCtx
			w.lastKeywordReport = &copied
		}

	keywordRowLoop:
		for {
			if reportCtx != nil && reportCtx.ReviewStarted.IsZero() {
				reportCtx.ReviewStarted = time.Now()
				w.lastKeywordReport = &keywordReportRunContext{
					Selected:      reportCtx.Selected,
					QueryStarted:  reportCtx.QueryStarted,
					SearchEnded:   reportCtx.SearchEnded,
					ReviewStarted: reportCtx.ReviewStarted,
					LabelMode:     reportCtx.LabelMode,
				}
			}
			selection, err := w.selectKeywordRows(groups)
			if err != nil {
				if errors.Is(err, prompt.ErrBackToQueryInput) {
					w.rewindKeywordRowsToInput()
					continue keywordInputLoop
				}
				if errors.Is(err, prompt.ErrBackToRowSelection) {
					continue keywordRowLoop
				}
				if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
					return err
				}
				retry, navErr := w.retryWorkflowStep(fmt.Sprintf("select keyword rows: %v", err), prompt.ErrBackToQueryInput)
				if navErr != nil {
					return navErr
				}
				if !retry {
					return err
				}
				continue keywordRowLoop
			}
			if selection.RunBlast {
				if err := w.runKeywordBlastMode(ctx, selected, groups, selection.Rows, reportCtx); err != nil {
					if errors.Is(err, prompt.ErrBackToRowSelection) {
						continue keywordRowLoop
					}
					if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
						return err
					}
					return err
				}
				continue keywordRowLoop
			}
			w.warmKeywordSequenceCache(ctx, selected, groups)
			w.postRunBackTarget = prompt.ErrBackToRowSelection
			if !selection.GenerateFile {
				continue keywordRowLoop
			}

			if err := w.prepareAndExportKeywordSelection(ctx, selected, groups, selection.Rows, reportCtx); err != nil {
				if errors.Is(err, prompt.ErrBackToRowSelection) {
					continue keywordRowLoop
				}
				return err
			}
			continue keywordRowLoop
		}
	}
}

func (w *BlastWizard) runKeywordBlastMode(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup, rows []model.KeywordResultRow, reportCtx *keywordReportRunContext) error {
	if len(rows) == 0 {
		return nil
	}

	databaseName, err := w.prompt.ChooseBlastTargetDatabase()
	if err != nil {
		return err
	}

	prepared, err := w.resolveKeywordRowsToBlastItems(ctx, selected, rows)
	if err != nil {
		return err
	}
	if len(prepared) == 0 {
		return w.showInfo("Keyword BLAST", "No selected keyword rows could be converted into BLAST queries.", prompt.ErrBackToRowSelection)
	}
	prepared, err = w.prepareKeywordBlastItems(ctx, selected, prepared)
	if err != nil {
		return err
	}
	if len(prepared) == 0 {
		return w.showInfo("Keyword BLAST", "No selected keyword rows remained after BLAST label handling.", prompt.ErrBackToRowSelection)
	}

	var targetSource source.DataSource
	switch strings.ToLower(strings.TrimSpace(databaseName)) {
	case "phytozome":
		targetSource = phytozome.NewClient(w.httpClient)
	case "lemna":
		targetSource = lemna.NewClient(w.httpClient)
	default:
		return fmt.Errorf("unsupported BLAST target database %q", databaseName)
	}

	previousSource := w.source
	previousBlastProgram := w.blastProgramPath
	previousDB := databaseDisplayName(previousSource.Name())
	defer func() {
		w.source = previousSource
		w.prompt.SetDatabaseContext(previousDB)
		w.setBlastProgramContext(previousBlastProgram)
	}()

	w.source = targetSource
	w.prompt.SetDatabaseContext(databaseDisplayName(targetSource.Name()))
	w.setBlastProgramContext("")

	candidates, err := w.loadSpeciesCandidates(ctx)
	if err != nil {
		return err
	}
	targetSpecies, err := w.selectSpecies(candidates)
	if err != nil {
		return err
	}

	w.lastKeywordGroups = cloneKeywordSearchGroups(groups)
	w.lastKeywordSpecies = selected
	if reportCtx != nil {
		copied := *reportCtx
		w.lastKeywordReport = &copied
	}
	return w.executePreparedBlast(ctx, targetSpecies, prepared, blastRequestConfig{})
}

func (w *BlastWizard) prepareKeywordBlastItems(ctx context.Context, selected model.SpeciesCandidate, items []blastQueryItem) ([]blastQueryItem, error) {
	if len(items) == 0 {
		return nil, nil
	}

	for {
		prepared := cloneBlastQueryItems(items)
		autoIdentifyLabels := false
		var err error
		if len(prepared) > 1 {
			prepared, autoIdentifyLabels, err = w.collectBlastLabelsBeforeResolve(prepared)
		} else {
			prepared, err = w.collectBlastLabels(ctx, selected, prepared)
		}
		if err != nil {
			if errors.Is(err, prompt.ErrBackToQueryInput) {
				return nil, prompt.ErrBackToRowSelection
			}
			return nil, err
		}

		if autoIdentifyLabels {
			prepared, err = w.autoIdentifyBlastLabelsWithProgress(ctx, selected, prepared)
			if err != nil {
				if errors.Is(err, prompt.ErrBackToQueryInput) {
					return nil, prompt.ErrBackToRowSelection
				}
				retry, navErr := w.retryWorkflowStep(fmt.Sprintf("auto identify BLAST label names: %v", err), prompt.ErrBackToRowSelection)
				if navErr != nil {
					return nil, navErr
				}
				if !retry {
					return nil, err
				}
				continue
			}
			if !allLabelsPresent(prepared) {
				action, actionErr := w.prompt.FetchErrorAction("auto identify BLAST label names: one or more query labels could not be identified", prompt.ErrBackToRowSelection)
				if actionErr != nil {
					return nil, actionErr
				}
				decision, navErr := interpretRecoveryAction(action, prompt.ErrBackToRowSelection, false)
				if navErr != nil {
					return nil, navErr
				}
				switch decision {
				case recoveryRetry:
					continue
				default:
					continue
				}
			}
		}

		if allLabelsPresent(prepared) {
			if keywordBlastItemsHaveReusableAliases(prepared) {
				return prepared, nil
			}
			prepared, err = w.supplementBlastAliasesWithProgress(ctx, selected, prepared)
			if err != nil {
				if errors.Is(err, prompt.ErrBackToQueryInput) {
					return nil, prompt.ErrBackToRowSelection
				}
				retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read BLAST alias label names: %v", err), prompt.ErrBackToRowSelection)
				if navErr != nil {
					return nil, navErr
				}
				if !retry {
					return nil, err
				}
				continue
			}
		}
		return prepared, nil
	}
}

func keywordBlastItemsHaveReusableAliases(items []blastQueryItem) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if !item.FromKeyword || item.QuerySource == nil {
			return false
		}
		if strings.TrimSpace(item.QuerySource.PhgoAliases) == "" {
			return false
		}
	}
	return true
}

func blastItemsHaveReusableAliases(items []blastQueryItem) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if !querySourceHasReusableAliasData(item.QuerySource) {
			return false
		}
	}
	return true
}

func blastItemsNeedingAutoLabel(items []blastQueryItem) []int {
	indexes := make([]int, 0, len(items))
	for i, item := range items {
		if strings.TrimSpace(item.LabelName) == "" {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

func blastItemsNeedingAliasSupplement(items []blastQueryItem) []int {
	indexes := make([]int, 0, len(items))
	for i, item := range items {
		if !querySourceHasReusableAliasData(item.QuerySource) {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

func (w *BlastWizard) resolveKeywordRowsToBlastItems(ctx context.Context, selected model.SpeciesCandidate, rows []model.KeywordResultRow) ([]blastQueryItem, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	run := func(taskCtx context.Context, update func(int, string)) ([]blastQueryItem, error) {
		progress := safeProgress(update)
		resolveCtx := mergeContexts(ctx, taskCtx)
		progress(0, "Fetching keyword peptide sequences...")
		sequences := w.prefetchKeywordSequences(resolveCtx, selected, rows, func(current int, message string) {
			progress(current, message)
		})
		if err := resolveCtx.Err(); err != nil {
			return nil, err
		}
		progress(len(rows), "Building cached BLAST query items from selected keyword rows...")
		items, converted := w.keywordRowsToBlastItemsCached(resolveCtx, selected, rows, sequences)
		progress(len(rows)+converted, fmt.Sprintf("Resolved keyword rows for BLAST... %d/%d", converted, len(rows)))
		return items, nil
	}
	if w.suppressTaskModals {
		return run(ctx, nil)
	}
	results, err := tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Keyword", "BLAST", "Resolving selected rows"),
		Title:       "Resolving keyword rows for BLAST",
		Description: "Fetching peptide sequences for selected keyword rows using the current keyword result metadata and cache.",
		Initial:     "Resolving keyword rows for BLAST...",
		Total:       len(rows) * 2,
		CancelError: prompt.ErrBackToRowSelection,
	}, run)
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (w *BlastWizard) keywordRowsToBlastItemsCached(ctx context.Context, selected model.SpeciesCandidate, rows []model.KeywordResultRow, sequences map[string]sequenceFetchResult) ([]blastQueryItem, int) {
	out := make([]blastQueryItem, 0, len(rows))
	converted := 0
	builtByKey := make(map[string]blastQueryItem, len(rows))
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return out, converted
		}
		cacheKey := keywordBlastItemCacheKey(selected, row)
		sequenceID := strings.TrimSpace(row.SequenceID)
		if sequenceID == "" {
			continue
		}
		sequence := ""
		if fetched, ok := sequences[sequenceID]; ok && fetched.err == nil {
			sequence = strings.TrimSpace(fetched.data.Sequence)
		}
		if sequence == "" {
			continue
		}
		if cached, ok := w.cachedKeywordBlastItem(cacheKey, sequence); ok {
			out = append(out, cached)
			converted++
			continue
		}
		if built, ok := builtByKey[cacheKey]; ok {
			out = append(out, built)
			converted++
			continue
		}
		item := keywordBlastItemFromRow(selected, row, sequences)
		if item.QuerySource == nil || strings.TrimSpace(item.Sequence) == "" {
			continue
		}
		w.storeKeywordBlastItem(cacheKey, item)
		builtByKey[cacheKey] = item
		out = append(out, item)
		converted++
	}
	return out, converted
}

func keywordBlastItemCacheKey(selected model.SpeciesCandidate, row model.KeywordResultRow) string {
	return strings.Join([]string{
		strconv.Itoa(selected.ProteomeID),
		strings.ToLower(strings.TrimSpace(selected.JBrowseName)),
		strings.ToLower(strings.TrimSpace(row.SourceDatabase)),
		strings.TrimSpace(row.SequenceID),
		strings.TrimSpace(row.TranscriptID),
		strings.TrimSpace(row.GeneIdentifier),
		strings.TrimSpace(row.ProteinID),
		strings.TrimSpace(row.GeneReportURL),
		strings.TrimSpace(row.LabelName),
		strings.TrimSpace(row.PhgoAliases),
	}, "|")
}

func (w *BlastWizard) cachedKeywordBlastItem(cacheKey string, sequence string) (blastQueryItem, bool) {
	if strings.TrimSpace(cacheKey) == "" || strings.TrimSpace(sequence) == "" {
		return blastQueryItem{}, false
	}
	w.keywordBlastItemMu.RLock()
	item, ok := w.keywordBlastItemCache[cacheKey]
	w.keywordBlastItemMu.RUnlock()
	if !ok || item.QuerySource == nil {
		return blastQueryItem{}, false
	}
	cached := item
	cached.Sequence = sequence
	sourceCopy := *cached.QuerySource
	sourceCopy.Sequence = sequence
	cached.QuerySource = &sourceCopy
	return cached, true
}

func (w *BlastWizard) storeKeywordBlastItem(cacheKey string, item blastQueryItem) {
	if strings.TrimSpace(cacheKey) == "" || item.QuerySource == nil {
		return
	}
	copyItem := item
	sourceCopy := *item.QuerySource
	copyItem.QuerySource = &sourceCopy
	w.keywordBlastItemMu.Lock()
	if w.keywordBlastItemCache == nil {
		w.keywordBlastItemCache = make(map[string]blastQueryItem)
	}
	w.keywordBlastItemCache[cacheKey] = copyItem
	w.keywordBlastItemMu.Unlock()
}

func keywordRowsToBlastItems(selected model.SpeciesCandidate, rows []model.KeywordResultRow, sequences map[string]sequenceFetchResult) []blastQueryItem {
	out := make([]blastQueryItem, 0, len(rows))
	for _, row := range rows {
		item := keywordBlastItemFromRow(selected, row, sequences)
		if item.QuerySource != nil && strings.TrimSpace(item.Sequence) != "" {
			out = append(out, item)
		}
	}
	return out
}

func keywordBlastItemFromRow(selected model.SpeciesCandidate, row model.KeywordResultRow, sequences map[string]sequenceFetchResult) blastQueryItem {
	sequenceID := strings.TrimSpace(row.SequenceID)
	if sequenceID == "" {
		return blastQueryItem{}
	}
	sequence := ""
	if fetched, ok := sequences[sequenceID]; ok && fetched.err == nil {
		sequence = strings.TrimSpace(fetched.data.Sequence)
	}
	if sequence == "" {
		return blastQueryItem{}
	}
	querySource := &model.QuerySequenceSource{
		Sequence:            sequence,
		ProteinSequence:     sequence,
		SequenceKind:        model.SequenceProtein,
		PreferredSequenceID: keywordBlastPreferredSequenceID(row),
		SourceDatabase:      firstNonEmpty(row.SourceDatabase),
		SourceProteomeID:    selected.ProteomeID,
		SourceJBrowseName:   selected.JBrowseName,
		SourceGenomeLabel:   selected.GenomeLabel,
		OriginalInputURL:    strings.TrimSpace(row.GeneReportURL),
		NormalizedURL:       strings.TrimSpace(row.GeneReportURL),
		LabelName:           strings.TrimSpace(row.LabelName),
		PhgoAliases:         strings.TrimSpace(row.PhgoAliases),
		Symbols:             strings.TrimSpace(row.Symbols),
		Synonyms:            strings.TrimSpace(row.Synonyms),
		UniProtAccession:    strings.TrimSpace(row.UniProt),
		GeneID:              stripTranscriptDecorations(strings.TrimSpace(row.GeneIdentifier)),
		TranscriptID:        strings.TrimSpace(row.TranscriptID),
		ProteinID:           firstNonEmpty(row.ProteinID, row.TranscriptID, row.SequenceID),
		OrganismShort:       firstNonEmpty(strings.TrimSpace(row.SequenceHeaderLabel), strings.TrimSpace(row.Genome), selected.SearchAlias, selected.GenomeLabel),
		Annotation:          firstNonEmpty(strings.TrimSpace(row.Description), strings.TrimSpace(row.Comments), strings.TrimSpace(row.Genome), selected.GenomeLabel),
	}
	labelName := strings.TrimSpace(row.LabelName)
	return blastQueryItem{
		RawInput:        firstNonEmpty(row.GeneReportURL, row.SequenceID, row.TranscriptID, row.GeneIdentifier),
		LabelName:       labelName,
		Sequence:        sequence,
		ProteinSequence: sequence,
		QuerySource:     querySource,
		FromKeyword:     true,
	}
}

func keywordBlastPreferredSequenceID(row model.KeywordResultRow) string {
	return firstNonEmpty(
		strings.TrimSpace(row.ProteinID),
		strings.TrimSpace(row.SequenceID),
		strings.TrimSpace(row.TranscriptID),
		strings.TrimSpace(row.GeneIdentifier),
	)
}

func (w *BlastWizard) runBlastMode(ctx context.Context, selected model.SpeciesCandidate, candidates []model.SpeciesCandidate) error {
	if w.reuseLastBlastRows && w.lastBlastRowContext != nil {
		if w.lastBlastReviewContext != nil {
			reviewContext := *w.lastBlastReviewContext
			reviewContext.Prepared = cloneBlastQueryItems(w.lastBlastReviewContext.Prepared)
			reviewContext.Runs = cloneBlastQueryRuns(w.lastBlastReviewContext.Runs)
			w.reuseLastBlastRows = false
			return w.reviewBlastRuns(ctx, reviewContext.Selected, reviewContext.Prepared, reviewContext.Runs, reviewContext.ConfiguredRequest, reviewContext.OriginalRunCount)
		}
		rowContext := *w.lastBlastRowContext
		rowContext.Rows = append([]model.BlastResultRow(nil), w.lastBlastRowContext.Rows...)
		rowContext.AllRows = append([]model.BlastResultRow(nil), w.lastBlastRowContext.AllRows...)
		rowContext.Numbers = append([]int(nil), w.lastBlastRowContext.Numbers...)
		rowContext.Flags = append([]bool(nil), w.lastBlastRowContext.Flags...)
		rowContext.SelectedRowsMask = append([]bool(nil), w.lastBlastRowContext.SelectedRowsMask...)
		w.reuseLastBlastRows = false
		return w.resumeBlastRowSelection(ctx, rowContext)
	}

blastInputLoop:
	for {
		var prepared []blastQueryItem
		var requestConfig blastRequestConfig
		w.consumeBlastInputRewind()
		if w.reuseLastBlastInput && len(w.lastBlastItems) > 0 {
			prepared = cloneBlastQueryItems(w.lastBlastItems)
			w.reuseLastBlastInput = false
		} else {
			cfg, cfgErr := w.configureBlastRequestBeforeInput(ctx, selected)
			if cfgErr != nil {
				if errors.Is(cfgErr, prompt.ErrBackToQueryInput) || errors.Is(cfgErr, prompt.ErrBackToBlastProgram) {
					continue blastInputLoop
				}
				if errors.Is(cfgErr, prompt.ErrBackToSpeciesSelection) || errors.Is(cfgErr, prompt.ErrBackToModeSelection) || errors.Is(cfgErr, prompt.ErrBackToDatabaseSelection) || errors.Is(cfgErr, prompt.ErrExitRequested) {
					return cfgErr
				}
				retry, navErr := w.retryWorkflowStep(fmt.Sprintf("configure BLAST request: %v", cfgErr), prompt.ErrBackToSpeciesSelection)
				if navErr != nil {
					return navErr
				}
				if !retry {
					return cfgErr
				}
				continue
			}
			requestConfig = cfg

			items, collectErr := w.collectBlastQueryItems()
			if collectErr != nil {
				if errors.Is(collectErr, prompt.ErrBackToQueryInput) {
					continue
				}
				if errors.Is(collectErr, prompt.ErrBackToSpeciesSelection) || errors.Is(collectErr, prompt.ErrBackToModeSelection) || errors.Is(collectErr, prompt.ErrBackToDatabaseSelection) || errors.Is(collectErr, prompt.ErrExitRequested) {
					return collectErr
				}
				retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read BLAST input: %v", collectErr), prompt.ErrBackToSpeciesSelection)
				if navErr != nil {
					return navErr
				}
				if !retry {
					return collectErr
				}
				continue
			}
			if len(items) == 0 {
				if err := w.showInfo("BLAST input", "BLAST input was empty. Please paste one or more queries.", prompt.ErrBackToSpeciesSelection); err != nil {
					return err
				}
				continue
			}

			var autoIdentifyLabels bool
			prepared = items
			if len(items) > 1 {
				var labelErr error
				prepared, autoIdentifyLabels, labelErr = w.collectBlastLabelsBeforeResolve(items)
				if labelErr != nil {
					if errors.Is(labelErr, prompt.ErrBackToQueryInput) {
						continue blastInputLoop
					}
					if errors.Is(labelErr, prompt.ErrBackToSpeciesSelection) || errors.Is(labelErr, prompt.ErrBackToModeSelection) || errors.Is(labelErr, prompt.ErrBackToDatabaseSelection) || errors.Is(labelErr, prompt.ErrExitRequested) {
						return labelErr
					}
					retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read label names: %v", labelErr), prompt.ErrBackToQueryInput)
					if navErr != nil {
						return navErr
					}
					if !retry {
						return labelErr
					}
					continue
				}
			}

			var err error
			prepared, err = w.resolveBlastQueryItems(ctx, prepared, candidates)
			if err != nil {
				var resolveErr *blastBatchResolveError
				if errors.As(err, &resolveErr) {
					action, actionErr := w.prompt.FetchErrorAction(resolveErr.Error(), prompt.ErrBackToQueryInput)
					if actionErr != nil {
						return actionErr
					}
					decision, navErr := interpretRecoveryAction(action, prompt.ErrBackToQueryInput, true)
					if navErr != nil {
						if decision == recoveryBack || decision == recoveryExit {
							return navErr
						}
						return navErr
					}
					switch decision {
					case recoveryRetry:
						continue blastInputLoop
					case recoverySkip:
						prepared = resolveErr.Prepared
					default:
						continue blastInputLoop
					}
				} else {
					if errors.Is(err, prompt.ErrBackToQueryInput) {
						continue
					}
					if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
						return err
					}
					retry, navErr := w.retryWorkflowStep(fmt.Sprintf("resolve BLAST input: %v", err), prompt.ErrBackToSpeciesSelection)
					if navErr != nil {
						return navErr
					}
					if !retry {
						return err
					}
					continue
				}
			}
			if autoIdentifyLabels {
				prepared, err = w.autoIdentifyBlastLabelsWithProgress(ctx, selected, prepared)
				if err != nil {
					retry, navErr := w.retryWorkflowStep(fmt.Sprintf("auto identify BLAST label names: %v", err), prompt.ErrBackToQueryInput)
					if navErr != nil {
						return navErr
					}
					if !retry {
						return err
					}
					continue blastInputLoop
				}
				if !allLabelsPresent(prepared) {
					action, actionErr := w.prompt.FetchErrorAction("auto identify BLAST label names: one or more query labels could not be identified", prompt.ErrBackToQueryInput)
					if actionErr != nil {
						return actionErr
					}
					decision, navErr := interpretRecoveryAction(action, prompt.ErrBackToQueryInput, false)
					if navErr != nil {
						if decision == recoveryBack || decision == recoveryExit {
							return navErr
						}
						return navErr
					}
					switch decision {
					case recoveryRetry:
						continue blastInputLoop
					default:
						continue blastInputLoop
					}
				}
			} else if len(prepared) == 1 {
				var labelErr error
				prepared, labelErr = w.collectBlastLabels(ctx, selected, prepared)
				if labelErr != nil {
					if errors.Is(labelErr, prompt.ErrBackToQueryInput) {
						continue blastInputLoop
					}
					if errors.Is(labelErr, prompt.ErrBackToSpeciesSelection) || errors.Is(labelErr, prompt.ErrBackToModeSelection) || errors.Is(labelErr, prompt.ErrBackToDatabaseSelection) || errors.Is(labelErr, prompt.ErrExitRequested) {
						return labelErr
					}
					retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read label names: %v", labelErr), prompt.ErrBackToQueryInput)
					if navErr != nil {
						return navErr
					}
					if !retry {
						return labelErr
					}
					continue
				}
			}
			if allLabelsPresent(prepared) {
				if keywordBlastItemsHaveReusableAliases(prepared) {
					// Auto-identify may have already populated the reusable label and alias metadata.
					goto preparedBlastInput
				}
			}
			if !autoIdentifyLabels && allLabelsPresent(prepared) {
				prepared, err = w.supplementBlastAliasesWithProgress(ctx, selected, prepared)
				if err != nil {
					retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read BLAST alias label names: %v", err), prompt.ErrBackToQueryInput)
					if navErr != nil {
						return navErr
					}
					if !retry {
						return err
					}
					continue blastInputLoop
				}
			}
		}
	preparedBlastInput:
		if len(prepared) == 0 {
			return nil
		}
		if err := w.executePreparedBlast(ctx, selected, prepared, requestConfig); err != nil {
			if errors.Is(err, prompt.ErrBackToQueryInput) {
				continue blastInputLoop
			}
			return err
		}
		return nil
	}
}

func (w *BlastWizard) executePreparedBlast(ctx context.Context, selected model.SpeciesCandidate, prepared []blastQueryItem, requestConfig blastRequestConfig) error {
batchConfigLoop:
	for {
		baseRequest := buildBlastRequest(selected, prepared[0].Sequence)
		configuredRequest := baseRequest
		if requestConfig.Ready {
			configuredRequest = requestConfig.Request
			configuredRequest.Sequence = baseRequest.Sequence
		} else {
			var err error
			configuredRequest, err = w.configureBlastRequest(ctx, selected, baseRequest)
			if err != nil {
				if errors.Is(err, prompt.ErrBackToQueryInput) {
					return prompt.ErrBackToQueryInput
				}
				if errors.Is(err, prompt.ErrBackToBlastProgram) {
					continue batchConfigLoop
				}
				if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
					return err
				}
				retry, navErr := w.retryWorkflowStep(fmt.Sprintf("configure BLAST request: %v", err), prompt.ErrBackToSpeciesSelection)
				if navErr != nil {
					return navErr
				}
				if !retry {
					return err
				}
				continue
			}
		}

		references, refErr := w.collectExternalReferenceConfig()
		if refErr != nil {
			if errors.Is(refErr, prompt.ErrBackToQueryInput) {
				return refErr
			}
			if errors.Is(refErr, prompt.ErrBackToSpeciesSelection) || errors.Is(refErr, prompt.ErrBackToModeSelection) || errors.Is(refErr, prompt.ErrBackToDatabaseSelection) || errors.Is(refErr, prompt.ErrExitRequested) {
				return refErr
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("configure external references: %v", refErr), prompt.ErrBackToQueryInput)
			if navErr != nil {
				return navErr
			}
			if !retry {
				return refErr
			}
			continue
		}

		familyPlan, familyErr := w.collectFamilyBlastPlan(prepared, references)
		if familyErr != nil {
			if errors.Is(familyErr, prompt.ErrBackToQueryInput) {
				return familyErr
			}
			if errors.Is(familyErr, prompt.ErrBackToSpeciesSelection) || errors.Is(familyErr, prompt.ErrBackToModeSelection) || errors.Is(familyErr, prompt.ErrBackToDatabaseSelection) || errors.Is(familyErr, prompt.ErrExitRequested) {
				return familyErr
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("configure Family BLAST: %v", familyErr), prompt.ErrBackToQueryInput)
			if navErr != nil {
				return navErr
			}
			if !retry {
				return familyErr
			}
			continue
		}

		alignedPrepared, alignErr := w.alignPreparedBlastItemsToRequest(ctx, prepared, configuredRequest)
		if alignErr != nil {
			if errors.Is(alignErr, context.Canceled) || errors.Is(alignErr, tui.ErrTaskCancelled) || errors.Is(alignErr, prompt.ErrBackToQueryInput) {
				return alignErr
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("prepare BLAST query sequences: %v", alignErr), prompt.ErrBackToQueryInput)
			if navErr != nil {
				return navErr
			}
			if !retry {
				return alignErr
			}
			continue
		}

		w.lastBlastItems = cloneBlastQueryItems(alignedPrepared)
		return w.executeConfiguredBlastBatchWithReferences(ctx, selected, alignedPrepared, configuredRequest, references, familyPlan)
	}
}

func (w *BlastWizard) collectExternalReferenceConfig() (externalReferenceConfig, error) {
	settings, err := w.prompt.ExternalReferenceSettings(prompt.ErrBackToQueryInput)
	if err != nil {
		return externalReferenceConfig{}, err
	}
	return externalReferenceConfig{
		AutoLabelBlastHits: settings.AutoLabelBlastHits,
		UseUniProt:         settings.UseUniProt,
		UseInterPro:        settings.UseInterPro,
		InterProSettings:   settings.InterProSettings,
	}, nil
}

func (w *BlastWizard) collectFamilyBlastPlan(prepared []blastQueryItem, references externalReferenceConfig) (*familyBlastPlan, error) {
	if len(prepared) <= 1 {
		return nil, nil
	}
	defaults := model.DefaultFamilyBlastSettings()
	defaults.UseUniProtReference = references.UseUniProt
	defaults.UseInterProReference = references.UseInterPro
	settings := defaults
	for {
		groups := detectFamilyBlastGroups(prepared, settings)
		if len(groups) == 0 {
			return nil, nil
		}
		settingsResult, err := w.prompt.FamilyBlastSettings(buildPromptFamilyBlastPreview(prepared, groups), settings, prompt.ErrBackToQueryInput)
		if err != nil {
			return nil, err
		}
		settings = settingsResult.Settings
		if settingsResult.Refresh {
			continue
		}
		if !settings.Enabled {
			return nil, nil
		}
		if len(settingsResult.CustomGroups) > 0 {
			groups = customPromptFamilyBlastGroups(prepared, settingsResult.CustomGroups)
			applyFamilyBlastGroupLabels(prepared, groups)
		} else {
			groups = detectFamilyBlastGroups(prepared, settings)
		}
		if len(groups) == 0 {
			return nil, nil
		}
		return &familyBlastPlan{Settings: settings, Groups: groups}, nil
	}
}

func applyFamilyBlastGroupLabels(prepared []blastQueryItem, groups []familyBlastGroup) {
	for _, group := range groups {
		for memberIndex, preparedIndex := range group.Indexes {
			if preparedIndex < 0 || preparedIndex >= len(prepared) || memberIndex >= len(group.Members) {
				continue
			}
			setBlastQueryItemLabel(&prepared[preparedIndex], group.Members[memberIndex].LabelName)
		}
	}
}

func buildPromptFamilyBlastPreview(prepared []blastQueryItem, groups []familyBlastGroup) prompt.FamilyBlastPreview {
	preview := prompt.FamilyBlastPreview{
		Groups: promptFamilyBlastGroups(groups),
	}
	grouped := map[int]struct{}{}
	for _, group := range groups {
		for _, idx := range group.Indexes {
			grouped[idx] = struct{}{}
		}
	}
	for i, item := range prepared {
		if _, ok := grouped[i]; ok {
			continue
		}
		label := strings.TrimSpace(familyBlastQueryLabel(item))
		if label == "" {
			continue
		}
		preview.Ungrouped = append(preview.Ungrouped, label)
		preview.UngroupedMembers = append(preview.UngroupedMembers, promptFamilyBlastMember(familyBlastMemberForItem(item)))
	}
	return preview
}

func promptFamilyBlastGroups(groups []familyBlastGroup) []prompt.FamilyBlastGroup {
	out := make([]prompt.FamilyBlastGroup, 0, len(groups))
	for _, group := range groups {
		out = append(out, prompt.FamilyBlastGroup{
			Name:    group.Name,
			Labels:  append([]string(nil), group.Labels...),
			Members: promptFamilyBlastMembers(group.Members),
			Queries: len(group.Indexes),
		})
	}
	return out
}

func promptFamilyBlastMembers(members []familyBlastMember) []prompt.FamilyBlastMember {
	out := make([]prompt.FamilyBlastMember, 0, len(members))
	for _, member := range members {
		out = append(out, promptFamilyBlastMember(member))
	}
	return out
}

func promptFamilyBlastMember(member familyBlastMember) prompt.FamilyBlastMember {
	return prompt.FamilyBlastMember{
		LabelName:         strings.TrimSpace(member.LabelName),
		ProteinID:         strings.TrimSpace(member.ProteinID),
		Aliases:           append([]string(nil), member.Aliases...),
		OriginalLabelName: strings.TrimSpace(member.OriginalLabelName),
		SourceKey:         strings.TrimSpace(member.SourceKey),
	}
}

func customPromptFamilyBlastGroups(prepared []blastQueryItem, groups []prompt.FamilyBlastGroup) []familyBlastGroup {
	indexByLabel := make(map[string]int, len(prepared))
	indexBySourceKey := make(map[string]int, len(prepared))
	indexByProteinID := make(map[string]int, len(prepared))
	for i, item := range prepared {
		label := strings.TrimSpace(familyBlastQueryLabel(item))
		if label == "" {
			label = strings.TrimSpace(item.LabelName)
		}
		if label != "" {
			indexByLabel[strings.ToLower(label)] = i
		}
		member := familyBlastMemberForItem(item)
		if member.SourceKey != "" {
			indexBySourceKey[strings.ToLower(member.SourceKey)] = i
		}
		if member.ProteinID != "" {
			indexByProteinID[strings.ToLower(member.ProteinID)] = i
		}
	}
	out := make([]familyBlastGroup, 0, len(groups))
	for _, group := range groups {
		members := promptGroupMembers(group)
		indexes := make([]int, 0, len(members))
		labels := make([]string, 0, len(members))
		groupMembers := make([]familyBlastMember, 0, len(members))
		seen := map[int]struct{}{}
		for _, member := range members {
			label := strings.TrimSpace(member.LabelName)
			idx, ok := -1, false
			for _, key := range []struct {
				value string
				table map[string]int
			}{
				{member.SourceKey, indexBySourceKey},
				{member.ProteinID, indexByProteinID},
				{member.OriginalLabelName, indexByLabel},
				{member.LabelName, indexByLabel},
			} {
				if strings.TrimSpace(key.value) == "" {
					continue
				}
				if found, exists := key.table[strings.ToLower(strings.TrimSpace(key.value))]; exists {
					idx, ok = found, true
					break
				}
			}
			if !ok {
				continue
			}
			if _, exists := seen[idx]; exists {
				continue
			}
			seen[idx] = struct{}{}
			if label == "" {
				label = familyBlastQueryLabel(prepared[idx])
			}
			setBlastQueryItemLabel(&prepared[idx], label)
			indexes = append(indexes, idx)
			labels = append(labels, label)
			updatedMember := familyBlastMemberForItem(prepared[idx])
			if len(member.Aliases) > 0 {
				updatedMember.Aliases = uniqueStrings(append(updatedMember.Aliases, member.Aliases...))
			}
			groupMembers = append(groupMembers, updatedMember)
		}
		if len(indexes) < 2 {
			continue
		}
		out = append(out, familyBlastGroup{
			Name:          strings.TrimSpace(group.Name),
			Indexes:       indexes,
			Labels:        labels,
			Members:       groupMembers,
			GroupSource:   "customized groups",
			DetectionRule: "customized in Family BLAST group editor",
		})
	}
	return out
}

func promptGroupMembers(group prompt.FamilyBlastGroup) []prompt.FamilyBlastMember {
	if len(group.Members) > 0 {
		return append([]prompt.FamilyBlastMember(nil), group.Members...)
	}
	out := make([]prompt.FamilyBlastMember, 0, len(group.Labels))
	for _, label := range group.Labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		out = append(out, prompt.FamilyBlastMember{
			LabelName:         label,
			OriginalLabelName: label,
			SourceKey:         label,
		})
	}
	return out
}

func (w *BlastWizard) executeConfiguredBlastBatch(ctx context.Context, selected model.SpeciesCandidate, prepared []blastQueryItem, configuredRequest model.BlastRequest) error {
	return w.executeConfiguredBlastBatchWithReferences(ctx, selected, prepared, configuredRequest, externalReferenceConfig{}, nil)
}

func (w *BlastWizard) executeConfiguredBlastBatchWithReferences(ctx context.Context, selected model.SpeciesCandidate, prepared []blastQueryItem, configuredRequest model.BlastRequest, references externalReferenceConfig, familyPlan *familyBlastPlan) error {
	w.postRunBackTarget = prompt.ErrBackToQueryInput

	queryRuns := make([]blastQueryRun, 0, len(prepared))
	resumeIndex := 0
	for resumeIndex < len(prepared) {
		runs, err := w.executeConfiguredBlastBatchRuns(ctx, prepared[resumeIndex:], configuredRequest, references)
		queryRuns = append(queryRuns, runs...)
		if err == nil {
			break
		}
		var batchErr *blastBatchRunError
		if !errors.As(err, &batchErr) {
			return err
		}
		failedIndex := resumeIndex + batchErr.Index - 1
		if failedIndex < resumeIndex || failedIndex >= len(prepared) {
			return err
		}
		if blastplus.IsMissingToolsError(batchErr) {
			installed, installErr := w.promptInstallBlastPlus(ctx, batchErr.Error(), prompt.ErrBackToQueryInput)
			if installErr != nil {
				if errors.Is(installErr, prompt.ErrDialogClosed) {
					return prompt.ErrBackToQueryInput
				}
				return installErr
			}
			if installed {
				resumeIndex = failedIndex
				continue
			}
			return prompt.ErrBackToQueryInput
		}
		action, actionErr := w.prompt.FetchErrorAction(batchErr.Error(), prompt.ErrBackToQueryInput)
		if actionErr != nil {
			return actionErr
		}
		decision, navErr := interpretRecoveryAction(action, prompt.ErrBackToQueryInput, true)
		if navErr != nil {
			if decision == recoveryBack || decision == recoveryExit {
				return navErr
			}
			return navErr
		}
		switch decision {
		case recoveryRetry:
			resumeIndex = failedIndex
			continue
		case recoverySkip:
			resumeIndex = failedIndex + 1
			continue
		default:
			return fmt.Errorf("unsupported batch recovery action %q", action)
		}
	}

	w.lastBlastReviewContext = &blastReviewContext{
		Selected:          selected,
		Prepared:          cloneBlastQueryItems(prepared),
		OriginalPrepared:  cloneBlastQueryItems(prepared),
		Runs:              cloneBlastQueryRuns(queryRuns),
		OriginalRuns:      cloneBlastQueryRuns(queryRuns),
		ConfiguredRequest: configuredRequest,
		OriginalRunCount:  len(queryRuns),
	}
	originalRunCount := len(queryRuns)
	if familyPlan != nil && familyPlan.Settings.Enabled {
		prepared, queryRuns = applyFamilyBlastPlan(prepared, queryRuns, familyPlan)
		if w.lastBlastReviewContext != nil {
			w.lastBlastReviewContext.Prepared = cloneBlastQueryItems(prepared)
			w.lastBlastReviewContext.Runs = cloneBlastQueryRuns(queryRuns)
		}
	}
	return w.reviewBlastRuns(ctx, selected, prepared, queryRuns, configuredRequest, originalRunCount)
}

func (w *BlastWizard) executeConfiguredBlastBatchRuns(ctx context.Context, prepared []blastQueryItem, configuredRequest model.BlastRequest, references externalReferenceConfig) ([]blastQueryRun, error) {
	alignedPrepared, err := w.alignPreparedBlastItemsToRequest(ctx, prepared, configuredRequest)
	if err != nil {
		return nil, err
	}
	prepared = alignedPrepared
	run := func(update func(int, string)) ([]blastQueryRun, error) {
		baseProgress := updateWithContext(ctx, update)
		var progressMu sync.Mutex
		progress := func(current int, message string) {
			progressMu.Lock()
			defer progressMu.Unlock()
			baseProgress(current, message)
		}
		runCtx := contextWithBlastReferenceConfig(contextWithUpdate(ctx, progress), references)
		previousSuppress := w.suppressTaskModals
		batchMode := len(prepared) > 1
		suppressTaskModals := previousSuppress || batchMode
		w.suppressTaskModals = suppressTaskModals
		defer func() {
			w.suppressTaskModals = previousSuppress
		}()
		runOne := func(ctx context.Context, i int, item blastQueryItem) (blastQueryRun, error) {
			if err := ctx.Err(); err != nil {
				return blastQueryRun{}, err
			}
			request := configuredRequest
			request.Sequence = item.Sequence
			if item.QuerySource != nil {
				request.Sequence = item.QuerySource.Sequence
			}
			progressBase := i * 2
			label := oneLinePreview(reportQueryLabel(item))
			actionLabel := "Submitting"
			if isLocalBlastRequest(request) {
				actionLabel = "Running local"
			}
			progress(progressBase, fmt.Sprintf("%s BLAST query %d/%d (%s)...", actionLabel, i+1, len(prepared), label))

			for {
				job, err := w.submitBlastWithRetry(ctx, request)
				if errors.Is(err, prompt.ErrBackToBlastProgram) || errors.Is(err, prompt.ErrExitRequested) {
					return blastQueryRun{}, err
				}
				if err != nil {
					return blastQueryRun{}, &blastBatchRunError{Stage: "submit BLAST job", Index: i + 1, Total: len(prepared), Label: label, Err: err}
				}
				if isLocalBlastRequest(request) {
					progress(progressBase+1, fmt.Sprintf("Loading local BLAST results for query %d/%d (%s)...", i+1, len(prepared), label))
				} else {
					progress(progressBase+1, fmt.Sprintf("Waiting for BLAST query %d/%d (%s)...", i+1, len(prepared), label))
				}
				results, err := w.waitForBlastResultsWithRetry(ctx, job.JobID)
				if errors.Is(err, prompt.ErrExitRequested) || errors.Is(err, prompt.ErrBackToQueryInput) || errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) {
					return blastQueryRun{}, err
				}
				if err != nil {
					return blastQueryRun{}, &blastBatchRunError{Stage: "wait for results", Index: i + 1, Total: len(prepared), Label: label, Err: err}
				}
				if len(results.Rows) == 0 {
					if !suppressTaskModals {
						if err := w.showBlastResults(results); err != nil {
							return blastQueryRun{}, err
						}
					}
					progress(progressBase+2, fmt.Sprintf("Finished BLAST query %d/%d (%s).", i+1, len(prepared), label))
					return blastQueryRun{Index: i + 1, Item: item, Request: request, Results: results}, nil
				}
				results.Rows = prepareBlastRowsForReferences(results.Rows, item, request, w.source.Name())
				if references.UseUniProt {
					w.prefetchBlastRowUniProtAccessions(ctx, results.Rows)
					enriched, enrichErr := w.enrichBlastRowsWithUniProt(ctx, results.Rows)
					if errors.Is(enrichErr, context.Canceled) || errors.Is(enrichErr, tui.ErrTaskCancelled) || errors.Is(enrichErr, prompt.ErrBackToQueryInput) {
						return blastQueryRun{}, enrichErr
					}
					if enrichErr == nil {
						results.Rows = enriched
					}
				}
				if references.UseInterPro {
					enriched, enrichErr := w.enrichBlastRowsWithInterPro(ctx, item, results.Rows, references.InterProSettings)
					if errors.Is(enrichErr, context.Canceled) || errors.Is(enrichErr, tui.ErrTaskCancelled) || errors.Is(enrichErr, prompt.ErrBackToQueryInput) {
						return blastQueryRun{}, enrichErr
					}
					if enrichErr == nil {
						results.Rows = enriched
					}
				}
				if references.AutoLabelBlastHits {
					results.Rows = w.autoIdentifyBlastHitLabels(ctx, request.Species, item, results.Rows)
				}
				results.Rows = annotateBlastRowsForQueryContext(results.Rows, item)
				progress(progressBase+2, fmt.Sprintf("Finished BLAST query %d/%d (%s).", i+1, len(prepared), label))
				return blastQueryRun{Index: i + 1, Item: item, Request: request, Results: results}, nil
			}
		}
		if !batchMode {
			run, err := runOne(runCtx, 0, prepared[0])
			if err != nil {
				return nil, err
			}
			return []blastQueryRun{run}, nil
		}

		type runOutcome struct {
			index int
			run   blastQueryRun
			err   error
			ok    bool
		}
		outcomes := make(chan runOutcome, len(prepared))
		jobs := make(chan int)
		workerCount := batchBlastWorkerCount(len(prepared), configuredRequest)
		batchCtx := runCtx
		if isLocalBlastRequest(configuredRequest) {
			batchCtx = lemna.WithLocalBlastThreads(runCtx, localBlastThreadsPerWorker(workerCount, configuredRequest))
		}
		batchCtx, cancelBatch := context.WithCancel(batchCtx)
		defer cancelBatch()
		var workers sync.WaitGroup
		for range workerCount {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for i := range jobs {
					if err := batchCtx.Err(); err != nil {
						return
					}
					run, err := runOne(batchCtx, i, prepared[i])
					select {
					case <-batchCtx.Done():
						return
					case outcomes <- runOutcome{index: i, run: run, err: err, ok: true}:
					}
					if err != nil {
						cancelBatch()
					}
				}
			}()
		}
		go func() {
			defer close(jobs)
			for i := range prepared {
				select {
				case <-batchCtx.Done():
					return
				case jobs <- i:
				}
			}
		}()
		go func() {
			workers.Wait()
			close(outcomes)
		}()

		results := make([]runOutcome, len(prepared))
		firstErrIndex := -1
		var firstErr error
		for outcome := range outcomes {
			results[outcome.index] = outcome
			if outcome.err != nil && firstErr == nil {
				firstErrIndex = outcome.index
				firstErr = outcome.err
				cancelBatch()
			}
		}
		queryRuns := make([]blastQueryRun, 0, len(prepared))
		for i, outcome := range results {
			if outcome.err != nil {
				if isCancellationLikeError(outcome.err) {
					return queryRuns, outcome.err
				}
				if firstErrIndex == i {
					return queryRuns, outcome.err
				}
				return queryRuns, parallelBlastBatchResumeError(i, prepared, firstErrIndex, firstErr)
			}
			if !outcome.ok {
				if firstErr != nil {
					if isCancellationLikeError(firstErr) {
						return queryRuns, firstErr
					}
					return queryRuns, parallelBlastBatchResumeError(i, prepared, firstErrIndex, firstErr)
				}
				if err := batchCtx.Err(); err != nil {
					return queryRuns, err
				}
				return queryRuns, &blastBatchRunError{Stage: "run BLAST query", Index: i + 1, Total: len(prepared), Label: oneLinePreview(reportQueryLabel(prepared[i])), Err: fmt.Errorf("query did not complete")}
			}
			if outcome.run.Index == 0 {
				return queryRuns, &blastBatchRunError{Stage: "run BLAST query", Index: i + 1, Total: len(prepared), Label: oneLinePreview(reportQueryLabel(prepared[i])), Err: fmt.Errorf("query did not complete")}
			}
			queryRuns = append(queryRuns, outcome.run)
		}
		return queryRuns, nil
	}
	if len(prepared) <= 1 {
		return run(nil)
	}
	if w.suppressTaskModals {
		return run(nil)
	}
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "Running batch"),
		Title:       "Running BLAST batch",
		Description: batchBlastDescription(configuredRequest),
		Initial:     "Starting BLAST batch...",
		Total:       len(prepared) * 2,
		CancelError: prompt.ErrBackToQueryInput,
	}, func(taskCtx context.Context, update func(int, string)) ([]blastQueryRun, error) {
		return run(updateWithContext(mergeContexts(ctx, taskCtx), update))
	})
}

func parallelBlastBatchResumeError(resumeIndex int, prepared []blastQueryItem, failedIndex int, err error) error {
	if len(prepared) == 0 {
		return err
	}
	if resumeIndex < 0 {
		resumeIndex = 0
	}
	if resumeIndex >= len(prepared) {
		resumeIndex = len(prepared) - 1
	}
	label := oneLinePreview(reportQueryLabel(prepared[resumeIndex]))
	if failedIndex < 0 || failedIndex >= len(prepared) {
		return &blastBatchRunError{Stage: "run BLAST query", Index: resumeIndex + 1, Total: len(prepared), Label: label, Err: err}
	}
	var batchErr *blastBatchRunError
	if errors.As(err, &batchErr) {
		return &blastBatchRunError{
			Stage: batchErr.Stage,
			Index: resumeIndex + 1,
			Total: len(prepared),
			Label: label,
			Err:   fmt.Errorf("parallel query %d/%d (%s) failed: %w", batchErr.Index, batchErr.Total, batchErr.Label, batchErr.Err),
		}
	}
	return &blastBatchRunError{
		Stage: "run BLAST query",
		Index: resumeIndex + 1,
		Total: len(prepared),
		Label: label,
		Err:   fmt.Errorf("parallel query %d/%d (%s) failed: %w", failedIndex+1, len(prepared), oneLinePreview(reportQueryLabel(prepared[failedIndex])), err),
	}
}

func batchBlastDescription(request model.BlastRequest) string {
	if isLocalBlastRequest(request) {
		return "Running local BLAST+ queries and loading cached result tables."
	}
	return "Submitting BLAST queries and collecting results."
}

func (w *BlastWizard) resumeBlastRowSelection(ctx context.Context, rowContext blastRowContext) error {
	for {
		selection, err := w.prompt.SelectBlastRowsBatchWithBack(rowContext.Rows, prompt.ErrBackToQueryInput)
		if err != nil {
			if errors.Is(err, prompt.ErrBackToRowSelection) {
				continue
			}
			if errors.Is(err, prompt.ErrBackToBlastProgram) || errors.Is(err, prompt.ErrBackToQueryInput) || errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
				return err
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("select BLAST rows: %v", err), prompt.ErrBackToRowSelection)
			if navErr != nil {
				return navErr
			}
			if !retry {
				return err
			}
			continue
		}
		w.postRunBackTarget = prompt.ErrBackToQueryInput
		if !selection.GenerateFile {
			continue
		}
		rows := selection.Rows
		if len(rows) == 0 {
			return w.showInfo("BLAST export", "No rows selected for this query. Export will be skipped.", prompt.ErrBackToRowSelection)
		}
		exportItem, err := w.prepareBlastExportItem(rowContext.Item, false)
		if err != nil {
			return err
		}
		settings, err := w.prepareExportSettings(buildBlastOutputDisplayName(exportItem), false, true, true)
		if err != nil {
			return err
		}
		outputDir := settings.OutputDir
		displayName := settings.BaseName
		if displayName == "" {
			displayName = buildBlastOutputDisplayName(exportItem)
		}
		filePrefix := sanitizeExportName(displayName)
		for {
			txtHeaderLabel := blastTXTHeaderLabel(exportItem, displayName)
			allRows := rowContext.AllRows
			if len(allRows) == 0 {
				allRows = rowContext.Results.Rows
			}
			files, err := w.exportFamilyBlastSelectionsToDir(ctx, rows, allRows, rowContext.Numbers, rowContext.Flags, exportItemFamilySources(exportItem), displayName, txtHeaderLabel, filePrefix, outputDir, settings, rowContext.FamilySettings, true)
			if err == nil && settings.WriteReport && strings.TrimSpace(files.ReportPath) == "" {
				selectedMask := buildBlastSelectedMaskFromSelection(len(allRows), rowContext.Numbers)
				if len(selectedMask) == 0 {
					selectedMask = append([]bool(nil), rowContext.SelectedRowsMask...)
				}
				reportPath, reportErr := w.renderBlastReportForExport(ctx, blastReportExportContext{
					Selected:          rowContext.Selected,
					Prepared:          []blastQueryItem{rowContext.Item},
					InputPrepared:     blastReportInputPreparedForItem(w.lastBlastReviewContext, rowContext.Item),
					Run:               blastQueryRun{Index: rowContext.Index, Item: rowContext.Item, Request: rowContext.Request, Results: rowContext.Results, SelectedRows: rows},
					Runs:              []blastQueryRun{{Index: rowContext.Index, Item: rowContext.Item, Request: rowContext.Request, Results: rowContext.Results, SelectedRows: rows}},
					SelectedRows:      selectedMask,
					Request:           rowContext.Request,
					BlastProgram:      rowContext.Request.Program,
					UseUniProt:        blastRowsHaveUniProt(allRows),
					UseInterPro:       blastRowsHaveInterPro(allRows),
					Rows:              rows,
					AllRows:           allRows,
					RowNumbers:        rowContext.Numbers,
					FilterFlags:       rowContext.Flags,
					FilterSettings:    rowContext.FilterSettings,
					FilterApplied:     rowContext.FilterApplied,
					FilterCleared:     rowContext.FilterCleared,
					BaseName:          displayName,
					OutputDir:         outputDir,
					Settings:          settings,
					Files:             files,
					ExportStarted:     time.Now(),
					ReportGeneratedAt: time.Now(),
				})
				if reportErr != nil {
					err = reportErr
				} else {
					files.ReportPath = reportPath
				}
			}
			if err != nil {
				action, actionErr := w.prompt.FetchErrorAction(fmt.Sprintf("BLAST export failed: %v", err), prompt.ErrBackToRowSelection)
				if actionErr != nil {
					return actionErr
				}
				decision, navErr := interpretRecoveryAction(action, prompt.ErrBackToRowSelection, true)
				if navErr != nil {
					if decision == recoveryBack || decision == recoveryExit {
						return navErr
					}
					return navErr
				}
				switch decision {
				case recoveryRetry:
					continue
				case recoverySkip:
					return nil
				default:
					return fmt.Errorf("unsupported export recovery action %q", action)
				}
			}
			continue
		}
	}
}

func (w *BlastWizard) reviewBlastRuns(ctx context.Context, selected model.SpeciesCandidate, prepared []blastQueryItem, runs []blastQueryRun, configuredRequest model.BlastRequest, originalRunCount int) error {
	w.postRunBackTarget = prompt.ErrBackToQueryInput
	if len(runs) == 0 {
		return nil
	}
	if originalRunCount <= 1 && len(runs) == 1 {
		return w.reviewSingleBlastRun(ctx, selected, prepared, runs[0], configuredRequest)
	}
	return w.reviewMultiBlastRuns(ctx, selected, prepared, runs, configuredRequest)
}

func (w *BlastWizard) reviewSingleBlastRun(ctx context.Context, selected model.SpeciesCandidate, prepared []blastQueryItem, run blastQueryRun, configuredRequest model.BlastRequest) error {
	if len(run.Results.Rows) == 0 {
		return w.showInfo("BLAST results", "No BLAST hits returned.", prompt.ErrBackToQueryInput)
	}
	w.warmBlastSequenceCache(ctx, run.Results.Rows)
	for {
		w.lastBlastRowContext = &blastRowContext{
			Rows:           append([]model.BlastResultRow(nil), run.Results.Rows...),
			AllRows:        append([]model.BlastResultRow(nil), run.Results.Rows...),
			Item:           run.Item,
			Selected:       selected,
			Request:        run.Request,
			Results:        run.Results,
			Index:          run.Index,
			FamilySettings: run.Item.FamilySettings,
		}
		selection, err := w.prompt.SelectBlastRowsWithOptions(run.Results.Rows, prompt.ErrBackToQueryInput, false)
		if err != nil {
			if errors.Is(err, prompt.ErrBackToRowSelection) {
				continue
			}
			if errors.Is(err, prompt.ErrBackToBlastProgram) || errors.Is(err, prompt.ErrBackToQueryInput) || errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
				return err
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("select BLAST rows: %v", err), prompt.ErrBackToQueryInput)
			if navErr != nil {
				return navErr
			}
			if !retry {
				return err
			}
			continue
		}
		if !selection.GenerateFile {
			if w.lastBlastRowContext != nil {
				w.lastBlastRowContext.Rows = append([]model.BlastResultRow(nil), selection.Rows...)
				w.lastBlastRowContext.Numbers = append([]int(nil), selection.RowNumbers...)
				w.lastBlastRowContext.FilterSettings = selection.FilterSettings
				w.lastBlastRowContext.FilterApplied = selection.FilterApplied
				w.lastBlastRowContext.FilterCleared = selection.FilterCleared
				w.lastBlastRowContext.Flags = append([]bool(nil), selection.FilterFlags...)
				w.lastBlastRowContext.SelectedRowsMask = append([]bool(nil), selection.Selected...)
			}
			continue
		}
		if len(selection.Rows) == 0 {
			if err := w.showInfo("BLAST export", "No rows selected for this query. Export will be skipped.", prompt.ErrBackToRowSelection); err != nil {
				return err
			}
			continue
		}
		if err := w.exportSingleBlastRun(ctx, selected, prepared, run, selection.Rows, run.Results.Rows, selection.RowNumbers, selection.FilterFlags, configuredRequest, false, selection); err != nil {
			if errors.Is(err, prompt.ErrBackToRowSelection) {
				continue
			}
			return err
		}
		continue
	}
}

func (w *BlastWizard) reviewMultiBlastRuns(ctx context.Context, selected model.SpeciesCandidate, prepared []blastQueryItem, runs []blastQueryRun, configuredRequest model.BlastRequest) error {
	w.warmBlastRunsSequenceCache(ctx, runs)
	for {
		selection, err := w.prompt.SelectBlastRuns(blastRunViews(runs), prompt.ErrBackToQueryInput)
		if err != nil {
			if errors.Is(err, prompt.ErrBackToRowSelection) {
				continue
			}
			if errors.Is(err, prompt.ErrBackToBlastProgram) || errors.Is(err, prompt.ErrBackToQueryInput) || errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
				return err
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("select BLAST rows: %v", err), prompt.ErrBackToQueryInput)
			if navErr != nil {
				return navErr
			}
			if !retry {
				return err
			}
			continue
		}
		if selection.DoneAll {
			if err := w.exportAllBlastRuns(ctx, selected, prepared, runs, selection.RowsByRun, selection.RowNumbersByRun, selection.FilterFlagsByRun, selection.SelectedByRun, configuredRequest, selection.FilterSettings, selection.FilterApplied, selection.FilterCleared); err != nil {
				if errors.Is(err, prompt.ErrBackToRowSelection) {
					continue
				}
				return err
			}
			continue
		}
		if !selection.GenerateFile {
			continue
		}
		if selection.RunIndex < 0 || selection.RunIndex >= len(runs) {
			continue
		}
		run := runs[selection.RunIndex]
		if len(run.Results.Rows) == 0 {
			if err := w.showInfo("BLAST export", "This BLAST query has no result rows to export.", prompt.ErrBackToRowSelection); err != nil {
				return err
			}
			continue
		}
		if len(selection.Rows) == 0 {
			if err := w.showInfo("BLAST export", "No rows selected for this query. Export will be skipped.", prompt.ErrBackToRowSelection); err != nil {
				return err
			}
			continue
		}
		if err := w.exportSingleBlastRun(ctx, selected, prepared, run, selection.Rows, run.Results.Rows, selection.RowNumbers, selection.FilterFlags, configuredRequest, true, selection); err != nil {
			if errors.Is(err, prompt.ErrBackToRowSelection) {
				continue
			}
			return err
		}
		continue
	}
}

func blastRunViews(runs []blastQueryRun) []prompt.BlastRunView {
	views := make([]prompt.BlastRunView, 0, len(runs))
	for _, run := range runs {
		item := prompt.BlastQueryItemView{
			RawInput:    run.Item.RawInput,
			LabelName:   run.Item.LabelName,
			FamilyName:  run.Item.FamilyName,
			MemberLabel: run.Item.MemberLabel,
		}
		if run.Item.QuerySource != nil {
			item.GeneID = run.Item.QuerySource.GeneID
			item.TranscriptID = run.Item.QuerySource.TranscriptID
			item.ProteinID = run.Item.QuerySource.ProteinID
		}
		views = append(views, prompt.BlastRunView{Item: item, Rows: run.Results.Rows})
	}
	return views
}

func (w *BlastWizard) warmBlastRunsSequenceCache(ctx context.Context, runs []blastQueryRun) {
	rows := make([]model.BlastResultRow, 0)
	for _, run := range runs {
		rows = append(rows, run.Results.Rows...)
	}
	w.warmBlastSequenceCache(ctx, rows)
}

func (w *BlastWizard) warmBlastSequenceCache(ctx context.Context, rows []model.BlastResultRow) {
	if len(rows) == 0 {
		return
	}
	go func() {
		w.prefetchBlastSequences(ctx, rows, nil)
	}()
}

func (w *BlastWizard) warmKeywordSequenceCache(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup) {
	rows := flattenKeywordSearchGroups(groups)
	if len(rows) == 0 {
		return
	}
	go func() {
		w.prefetchKeywordSequences(ctx, selected, rows, nil)
	}()
}

func (w *BlastWizard) exportSingleBlastRun(ctx context.Context, selected model.SpeciesCandidate, prepared []blastQueryItem, run blastQueryRun, rows []model.BlastResultRow, allRows []model.BlastResultRow, rowNumbers []int, filterFlags []bool, configuredRequest model.BlastRequest, batch bool, selection prompt.BlastRowSelection) error {
	exportItem, err := w.prepareBlastExportItem(run.Item, batch)
	if err != nil {
		return err
	}
	defaultName := ""
	allowEmpty := false
	if strings.TrimSpace(exportItem.LabelName) != "" {
		defaultName = buildBlastOutputDisplayName(exportItem)
		allowEmpty = true
	}
	settings, err := w.prepareExportSettings(defaultName, false, allowEmpty, true)
	if err != nil {
		return err
	}
	displayName := settings.BaseName
	if displayName == "" {
		displayName = defaultName
	}
	filePrefix := sanitizeExportName(displayName)
	txtHeaderLabel := blastTXTHeaderLabel(exportItem, displayName)
	for {
		files, err := w.exportFamilyBlastSelectionsToDir(ctx, rows, allRows, rowNumbers, filterFlags, exportItemFamilySources(exportItem), displayName, txtHeaderLabel, filePrefix, settings.OutputDir, settings, exportItem.FamilySettings, true)
		if err == nil && settings.WriteReport {
			reportPath, reportErr := w.renderBlastReportForExport(ctx, blastReportExportContext{
				Selected:          selected,
				Prepared:          cloneBlastQueryItems(prepared),
				InputPrepared:     blastReportInputPreparedForItem(w.lastBlastReviewContext, run.Item),
				Run:               run,
				Runs:              []blastQueryRun{run},
				SelectedRows:      append([]bool(nil), selection.Selected...),
				Request:           configuredRequest,
				BlastProgram:      configuredRequest.Program,
				UseUniProt:        blastRowsHaveUniProt(allRows),
				UseInterPro:       blastRowsHaveInterPro(allRows),
				Rows:              rows,
				AllRows:           allRows,
				RowNumbers:        rowNumbers,
				FilterFlags:       filterFlags,
				FilterSettings:    selection.FilterSettings,
				FilterApplied:     selection.FilterApplied,
				FilterCleared:     selection.FilterCleared,
				BaseName:          displayName,
				OutputDir:         settings.OutputDir,
				Settings:          settings,
				Files:             files,
				ExportStarted:     time.Now(),
				ReportGeneratedAt: time.Now(),
			})
			if reportErr != nil {
				err = reportErr
			} else {
				files.ReportPath = reportPath
			}
		}
		if err != nil {
			action, actionErr := w.prompt.FetchErrorAction(fmt.Sprintf("BLAST query %d (%s): export failed: %v", run.Index, oneLinePreview(reportQueryLabel(exportItem)), err), prompt.ErrBackToRowSelection)
			if actionErr != nil {
				return actionErr
			}
			decision, navErr := interpretRecoveryAction(action, prompt.ErrBackToRowSelection, true)
			if navErr != nil {
				if decision == recoveryBack || decision == recoveryExit {
					return navErr
				}
				return navErr
			}
			switch decision {
			case recoveryRetry:
				continue
			case recoverySkip:
				return nil
			default:
				return fmt.Errorf("unsupported export recovery action %q", action)
			}
		}
		break
	}
	return nil
}

func (w *BlastWizard) exportAllBlastRuns(ctx context.Context, selected model.SpeciesCandidate, prepared []blastQueryItem, runs []blastQueryRun, rowsByRun [][]model.BlastResultRow, rowNumbersByRun [][]int, filterFlagsByRun [][]bool, selectedByRun [][]bool, configuredRequest model.BlastRequest, filterSettings model.BlastFilterSettings, filterApplied bool, filterCleared bool) error {
	settings, err := w.prepareBatchExportSettings()
	if err != nil {
		return err
	}
	reportPrepared := cloneBlastQueryItems(prepared)
	reportRuns := cloneBlastQueryRuns(runs)
	var exportedRuns []blastQueryRun
	var exportedFiles []exportFileResult
	var exportedRowsByRun [][]model.BlastResultRow
	var exportedRowNumbersByRun [][]int
	var exportedFilterFlagsByRun [][]bool
	var exportedSelectedByRun [][]bool
	for {
		batchResult, err := w.exportAllBlastRunsWithProgress(ctx, selected, prepared, runs, rowsByRun, rowNumbersByRun, filterFlagsByRun, selectedByRun, configuredRequest, settings)
		nextRuns := batchResult.Runs
		exportedRuns = append(exportedRuns, nextRuns...)
		exportedFiles = append(exportedFiles, batchResult.Files...)
		exportedRowsByRun = append(exportedRowsByRun, batchResult.RowsByRun...)
		exportedRowNumbersByRun = append(exportedRowNumbersByRun, batchResult.RowNumbersByRun...)
		exportedFilterFlagsByRun = append(exportedFilterFlagsByRun, batchResult.FilterFlagsByRun...)
		exportedSelectedByRun = append(exportedSelectedByRun, batchResult.SelectedByRun...)
		runs = removeExportedBlastRuns(runs, nextRuns)
		if err == nil {
			break
		}
		var exportErr *blastBatchExportError
		if !errors.As(err, &exportErr) {
			return err
		}
		action, actionErr := w.prompt.FetchErrorAction(exportErr.Error(), prompt.ErrBackToRowSelection)
		if actionErr != nil {
			return actionErr
		}
		decision, navErr := interpretRecoveryAction(action, prompt.ErrBackToRowSelection, true)
		if navErr != nil {
			if decision == recoveryBack || decision == recoveryExit {
				return navErr
			}
			return navErr
		}
		switch decision {
		case recoveryRetry:
			continue
		case recoverySkip:
			filteredRuns := make([]blastQueryRun, 0, len(runs))
			for _, run := range runs {
				if run.Index != exportErr.Run.Index {
					filteredRuns = append(filteredRuns, run)
				}
			}
			runs = filteredRuns
			continue
		default:
			return fmt.Errorf("unsupported export recovery action %q", action)
		}
	}
	if len(exportedRuns) == 0 {
		return w.showInfo("BLAST export", "No BLAST result rows were available to export.", prompt.ErrBackToRowSelection)
	}
	reportRowsByRun := rowsByRun
	reportRowNumbersByRun := rowNumbersByRun
	reportFilterFlagsByRun := filterFlagsByRun
	reportSelectedByRun := selectedByRun
	if reviewCtx := w.lastBlastReviewContext; reviewCtx != nil && len(reviewCtx.Runs) == len(reportRuns) {
		reportPrepared = cloneBlastQueryItems(reviewCtx.Prepared)
		reportRuns = cloneBlastQueryRuns(reviewCtx.Runs)
	}
	if settings.WriteReport {
		inputPrepared := reportPrepared
		if reviewCtx := w.lastBlastReviewContext; reviewCtx != nil && len(reviewCtx.OriginalPrepared) > 0 {
			inputPrepared = reviewCtx.OriginalPrepared
		}
		reportPath, reportErr := w.renderBlastBatchReport(ctx, selected, reportPrepared, inputPrepared, reportRuns, exportedFiles, reportRowsByRun, reportRowNumbersByRun, reportFilterFlagsByRun, reportSelectedByRun, settings.OutputDir, settings, configuredRequest, filterSettings, filterApplied, filterCleared)
		if reportErr != nil {
			return reportErr
		}
		if strings.TrimSpace(reportPath) != "" {
			exportedFiles = append(exportedFiles, exportFileResult{ReportPath: reportPath})
		}
	}
	return w.showInfo("Export complete", fmt.Sprintf("Exported %d BLAST queries to\n%s", len(exportedRuns), settings.OutputDir), prompt.ErrBackToRowSelection)
}

func (w *BlastWizard) exportAllBlastRunsWithProgress(ctx context.Context, selected model.SpeciesCandidate, prepared []blastQueryItem, runs []blastQueryRun, rowsByRun [][]model.BlastResultRow, rowNumbersByRun [][]int, filterFlagsByRun [][]bool, selectedByRun [][]bool, configuredRequest model.BlastRequest, settings exportSettings) (blastBatchExportResult, error) {
	exportable := 0
	for runPosition, run := range runs {
		rows := run.Results.Rows
		if runPosition >= 0 && runPosition < len(rowsByRun) {
			rows = rowsByRun[runPosition]
		}
		if len(rows) > 0 {
			exportable++
		}
	}
	if exportable == 0 {
		return blastBatchExportResult{}, nil
	}
	run := func(taskCtx context.Context, update func(int, string)) (blastBatchExportResult, error) {
		baseExportUpdate := safeProgress(update)
		var exportUpdateMu sync.Mutex
		exportUpdate := func(current int, message string) {
			exportUpdateMu.Lock()
			defer exportUpdateMu.Unlock()
			baseExportUpdate(current, message)
		}
		exportCtx := contextWithUpdate(mergeContexts(ctx, taskCtx), exportUpdate)
		usedNames := make(map[string]int, len(runs))
		jobs := make([]blastExportJob, 0, exportable)
		for runPosition, run := range runs {
			rows := run.Results.Rows
			if runPosition >= 0 && runPosition < len(rowsByRun) {
				rows = rowsByRun[runPosition]
			}
			if len(rows) == 0 {
				continue
			}
			exportItem := run.Item
			displayName := buildBlastOutputDisplayName(exportItem)
			var rowNumbers []int
			if runPosition >= 0 && runPosition < len(rowNumbersByRun) {
				rowNumbers = rowNumbersByRun[runPosition]
			}
			var filterFlags []bool
			if runPosition >= 0 && runPosition < len(filterFlagsByRun) {
				filterFlags = filterFlagsByRun[runPosition]
			}
			var selectedRowsMask []bool
			if runPosition >= 0 && runPosition < len(selectedByRun) {
				selectedRowsMask = selectedByRun[runPosition]
			}
			jobs = append(jobs, blastExportJob{
				exportIndex:      len(jobs),
				runPosition:      runPosition,
				run:              run,
				rows:             rows,
				rowNumbers:       rowNumbers,
				filterFlags:      filterFlags,
				selectedRowsMask: selectedRowsMask,
				displayName:      displayName,
				filePrefix:       uniqueExportPrefix(sanitizeExportName(displayName), usedNames),
				txtHeaderLabel:   blastTXTHeaderLabel(exportItem, displayName),
			})
		}
		previousSuppress := w.suppressTaskModals
		w.suppressTaskModals = true
		defer func() {
			w.suppressTaskModals = previousSuppress
		}()
		if settings.WriteText {
			w.prefetchBlastExportBatchSequences(exportCtx, jobs, settings, exportUpdate)
		}
		type exportOutcome struct {
			job   blastExportJob
			run   blastQueryRun
			files exportFileResult
			err   error
			ok    bool
		}
		outcomes := make(chan exportOutcome, len(jobs))
		exportWorkerCount := diskParallelismFor(len(jobs))
		jobQueue := make(chan blastExportJob)
		batchCtx, cancelBatch := context.WithCancel(exportCtx)
		defer cancelBatch()
		var workers sync.WaitGroup
		for range exportWorkerCount {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for job := range jobQueue {
					if err := batchCtx.Err(); err != nil {
						return
					}
					exportUpdate(job.exportIndex, fmt.Sprintf("Exporting BLAST query %d/%d (%s)...", job.exportIndex+1, exportable, oneLinePreview(job.displayName)))
					files, err := w.exportFamilyBlastSelectionsToDir(batchCtx, job.rows, job.run.Results.Rows, job.rowNumbers, job.filterFlags, exportItemFamilySources(job.run.Item), job.displayName, job.txtHeaderLabel, job.filePrefix, settings.OutputDir, settings, job.run.Item.FamilySettings, false)
					exported := job.run
					exported.Item = job.run.Item
					exported.SelectedRows = job.rows
					exported.ExcelPath = files.ExcelPath
					exported.TextPath = files.TextPath
					select {
					case <-batchCtx.Done():
						return
					case outcomes <- exportOutcome{job: job, run: exported, files: files, err: err, ok: true}:
					}
					if err != nil {
						cancelBatch()
					}
				}
			}()
		}
		go func() {
			defer close(jobQueue)
			for _, job := range jobs {
				select {
				case <-batchCtx.Done():
					return
				case jobQueue <- job:
				}
			}
		}()
		go func() {
			workers.Wait()
			close(outcomes)
		}()
		results := make([]exportOutcome, len(jobs))
		completed := 0
		firstErrIndex := -1
		var firstErr error
		for outcome := range outcomes {
			results[outcome.job.exportIndex] = outcome
			if outcome.err != nil && firstErr == nil {
				firstErrIndex = outcome.job.exportIndex
				firstErr = outcome.err
				cancelBatch()
			}
			if outcome.err == nil {
				completed++
				exportUpdate(completed, fmt.Sprintf("Exported BLAST query %d/%d (%s).", completed, exportable, oneLinePreview(outcome.job.displayName)))
			}
		}
		exportedRuns := make([]blastQueryRun, 0, len(jobs))
		exportedFiles := make([]exportFileResult, 0, len(jobs))
		exportedRowsByRun := make([][]model.BlastResultRow, 0, len(jobs))
		exportedRowNumbersByRun := make([][]int, 0, len(jobs))
		exportedFilterFlagsByRun := make([][]bool, 0, len(jobs))
		exportedSelectedByRun := make([][]bool, 0, len(jobs))
		for i, outcome := range results {
			if outcome.err != nil {
				if isCancellationLikeError(outcome.err) {
					return blastBatchExportResult{}, outcome.err
				}
				return blastBatchExportResult{
					Runs:             exportedRuns,
					Files:            exportedFiles,
					RowsByRun:        exportedRowsByRun,
					RowNumbersByRun:  exportedRowNumbersByRun,
					FilterFlagsByRun: exportedFilterFlagsByRun,
					SelectedByRun:    exportedSelectedByRun,
				}, &blastBatchExportError{Run: outcome.job.run, Label: oneLinePreview(reportQueryLabel(outcome.job.run.Item)), Err: outcome.err}
			}
			if !outcome.ok {
				if firstErr != nil {
					if isCancellationLikeError(firstErr) {
						return blastBatchExportResult{}, firstErr
					}
					failedRun, failedLabel, wrappedErr := parallelBlastExportResumeFailure(jobs, i, firstErrIndex, firstErr)
					return blastBatchExportResult{
						Runs:             exportedRuns,
						Files:            exportedFiles,
						RowsByRun:        exportedRowsByRun,
						RowNumbersByRun:  exportedRowNumbersByRun,
						FilterFlagsByRun: exportedFilterFlagsByRun,
						SelectedByRun:    exportedSelectedByRun,
					}, &blastBatchExportError{Run: failedRun, Label: failedLabel, Err: wrappedErr}
				}
				if err := batchCtx.Err(); err != nil {
					return blastBatchExportResult{}, err
				}
				return blastBatchExportResult{
					Runs:             exportedRuns,
					Files:            exportedFiles,
					RowsByRun:        exportedRowsByRun,
					RowNumbersByRun:  exportedRowNumbersByRun,
					FilterFlagsByRun: exportedFilterFlagsByRun,
					SelectedByRun:    exportedSelectedByRun,
				}, &blastBatchExportError{Run: jobs[i].run, Label: oneLinePreview(reportQueryLabel(jobs[i].run.Item)), Err: fmt.Errorf("export did not complete")}
			}
			exportedRuns = append(exportedRuns, outcome.run)
			exportedFiles = append(exportedFiles, outcome.files)
			exportedRowsByRun = append(exportedRowsByRun, outcome.job.rows)
			exportedRowNumbersByRun = append(exportedRowNumbersByRun, outcome.job.rowNumbers)
			exportedFilterFlagsByRun = append(exportedFilterFlagsByRun, outcome.job.filterFlags)
			exportedSelectedByRun = append(exportedSelectedByRun, outcome.job.selectedRowsMask)
		}
		return blastBatchExportResult{
			Runs:             exportedRuns,
			Files:            exportedFiles,
			RowsByRun:        exportedRowsByRun,
			RowNumbersByRun:  exportedRowNumbersByRun,
			FilterFlagsByRun: exportedFilterFlagsByRun,
			SelectedByRun:    exportedSelectedByRun,
		}, nil
	}
	if w.suppressTaskModals {
		return run(ctx, nil)
	}
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Export", "BLAST batch"),
		Title:       "Exporting BLAST batch",
		Description: "Writing all selected BLAST query files.",
		Initial:     "Starting BLAST batch export...",
		Total:       exportable,
		CancelError: prompt.ErrBackToRowSelection,
	}, run)
}

func parallelBlastExportResumeFailure(jobs []blastExportJob, resumeIndex int, failedIndex int, err error) (blastQueryRun, string, error) {
	if len(jobs) == 0 {
		return blastQueryRun{}, "", err
	}
	if resumeIndex < 0 {
		resumeIndex = 0
	}
	if resumeIndex >= len(jobs) {
		resumeIndex = len(jobs) - 1
	}
	resumeRun := jobs[resumeIndex].run
	resumeLabel := oneLinePreview(reportQueryLabel(resumeRun.Item))
	if failedIndex < 0 || failedIndex >= len(jobs) {
		return resumeRun, resumeLabel, err
	}
	failedRun := jobs[failedIndex].run
	failedLabel := oneLinePreview(reportQueryLabel(failedRun.Item))
	return resumeRun, resumeLabel, fmt.Errorf("parallel export for BLAST query %d (%s) failed: %w", failedRun.Index, failedLabel, err)
}

func isCancellationLikeError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, tui.ErrTaskCancelled) || errors.Is(err, prompt.ErrBackToQueryInput) || errors.Is(err, prompt.ErrBackToRowSelection) || errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrBackToBlastProgram) || errors.Is(err, prompt.ErrExitRequested)
}

func (w *BlastWizard) prefetchBlastExportBatchSequences(ctx context.Context, jobs []blastExportJob, settings exportSettings, update func(int, string)) {
	if len(jobs) == 0 {
		return
	}
	rows := make([]model.BlastResultRow, 0)
	for _, job := range jobs {
		rows = append(rows, job.rows...)
		if settings.WriteRawExcel && settings.WriteText {
			rows = append(rows, job.run.Results.Rows...)
		}
	}
	if len(rows) == 0 {
		return
	}
	progress := safeProgress(update)
	progress(0, "Preloading peptide sequences for all BLAST export files...")
	w.prefetchBlastSequences(ctx, rows, func(current int, message string) {
		_ = current
		progress(0, message)
	})
}

func uniqueExportPrefix(base string, used map[string]int) string {
	base = sanitizeExportName(base)
	if base == "" {
		base = "query"
	}
	count := used[base]
	used[base] = count + 1
	if count == 0 {
		return base
	}
	return fmt.Sprintf("%s_%d", base, count+1)
}

func buildBlastSelectedMaskFromSelection(total int, rowNumbers []int) []bool {
	if total <= 0 {
		return nil
	}
	mask := make([]bool, total)
	anySelected := false
	for _, rowNumber := range rowNumbers {
		if rowNumber <= 0 || rowNumber > total {
			continue
		}
		mask[rowNumber-1] = true
		anySelected = true
	}
	if !anySelected {
		return nil
	}
	return mask
}

func hasExportedBlastFiles(runs []blastQueryRun) bool {
	for _, run := range runs {
		if strings.TrimSpace(run.ExcelPath) != "" || strings.TrimSpace(run.TextPath) != "" {
			return true
		}
	}
	return false
}

func removeExportedBlastRuns(runs []blastQueryRun, exported []blastQueryRun) []blastQueryRun {
	if len(exported) == 0 {
		return runs
	}
	done := make(map[int]struct{}, len(exported))
	for _, run := range exported {
		done[run.Index] = struct{}{}
	}
	out := make([]blastQueryRun, 0, len(runs))
	for _, run := range runs {
		if _, ok := done[run.Index]; ok {
			continue
		}
		out = append(out, run)
	}
	return out
}

func blastReportInputPreparedForItem(ctx *blastReviewContext, item blastQueryItem) []blastQueryItem {
	if ctx == nil {
		return nil
	}
	if strings.TrimSpace(item.FamilyName) != "" && len(item.FamilySources) > 0 {
		out := make([]blastQueryItem, 0, len(item.FamilySources))
		for _, source := range item.FamilySources {
			if source == nil {
				continue
			}
			for _, original := range ctx.OriginalPrepared {
				if original.QuerySource == source || blastQuerySourceSame(original.QuerySource, source) {
					out = append(out, original)
					break
				}
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if len(ctx.OriginalPrepared) > 0 {
		return cloneBlastQueryItems(ctx.OriginalPrepared)
	}
	return nil
}

func blastQuerySourceSame(left *model.QuerySequenceSource, right *model.QuerySequenceSource) bool {
	if left == nil || right == nil {
		return false
	}
	return strings.TrimSpace(left.Sequence) != "" && strings.TrimSpace(left.Sequence) == strings.TrimSpace(right.Sequence) &&
		firstNonEmpty(left.LabelName, left.GeneID, left.TranscriptID, left.ProteinID) == firstNonEmpty(right.LabelName, right.GeneID, right.TranscriptID, right.ProteinID)
}

func prepareBlastRowsForReferences(rows []model.BlastResultRow, item blastQueryItem, request model.BlastRequest, sourceName string) []model.BlastResultRow {
	if len(rows) == 0 {
		return rows
	}
	out := make([]model.BlastResultRow, len(rows))
	copy(out, rows)
	label := blastQueryItemLabelName(item)
	geneID := blastQueryItemGeneID(item)
	sourceName = strings.TrimSpace(sourceName)
	program := canonicalBlastProgram(request.Program)
	queryLength := inferredBlastQueryLength(out, request.Sequence)
	for i := range out {
		out[i].BlastLabelName = label
		out[i].BlastGeneID = geneID
		if out[i].SourceDatabase == "" {
			out[i].SourceDatabase = sourceName
		}
		if out[i].BlastProgram == "" {
			out[i].BlastProgram = program
		}
		if out[i].TargetID == 0 {
			out[i].TargetID = request.Species.ProteomeID
		}
		if out[i].JBrowseName == "" {
			out[i].JBrowseName = request.Species.JBrowseName
		}
		if out[i].SubjectID == "" {
			out[i].SubjectID = out[i].Protein
		}
		if queryLength > 0 && out[i].QueryLength <= 0 {
			out[i].QueryLength = queryLength
		}
		if out[i].AlignQueryLengthPercent <= 0 && out[i].AlignLength > 0 && out[i].QueryLength > 0 {
			out[i].AlignQueryLengthPercent = float64(out[i].AlignLength) / float64(out[i].QueryLength) * 100
		}
	}
	return out
}

func inferredBlastQueryLength(rows []model.BlastResultRow, sequence string) int {
	queryLength := len(sanitizeSequence(sequence))
	if queryLength > 0 {
		return queryLength
	}
	for _, row := range rows {
		if row.QueryLength > 0 {
			return row.QueryLength
		}
		if span := coordinateSpan(row.QueryFrom, row.QueryTo); span > queryLength {
			queryLength = span
		}
	}
	return queryLength
}

func blastQueryItemGeneID(item blastQueryItem) string {
	if item.QuerySource != nil {
		return preferredPhgoIdentifier(item.QuerySource)
	}
	return strings.TrimSpace(item.RawInput)
}

func blastQueryItemLabelName(item blastQueryItem) string {
	if label := strings.TrimSpace(item.LabelName); label != "" {
		return label
	}
	if item.QuerySource != nil {
		return strings.TrimSpace(item.QuerySource.LabelName)
	}
	return ""
}

func (w *BlastWizard) autoIdentifyBlastHitLabels(ctx context.Context, selected model.SpeciesCandidate, item blastQueryItem, rows []model.BlastResultRow) []model.BlastResultRow {
	if inheritedUpdate := updateFromContext(ctx); inheritedUpdate != nil {
		return w.autoIdentifyBlastHitLabelsWithProgress(ctx, selected, item, rows, inheritedUpdate)
	}
	if w.suppressTaskModals {
		return w.autoIdentifyBlastHitLabelsWithProgress(ctx, selected, item, rows, nil)
	}
	out, err := tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "Auto identify hit labels"),
		Title:       "Auto identifying BLAST hit label names",
		Description: "Resolving BLAST hit row label names from Phytozome and local aliases.",
		Initial:     "Preparing BLAST hit label identification...",
		Total:       maxInt(1, len(rows)),
		CancelError: prompt.ErrBackToQueryInput,
	}, func(taskCtx context.Context, update func(int, string)) ([]model.BlastResultRow, error) {
		return w.autoIdentifyBlastHitLabelsWithProgress(mergeContexts(ctx, taskCtx), selected, item, rows, update), nil
	})
	if err != nil {
		return append([]model.BlastResultRow(nil), rows...)
	}
	return out
}

func (w *BlastWizard) autoIdentifyBlastHitLabelsWithProgress(ctx context.Context, selected model.SpeciesCandidate, item blastQueryItem, rows []model.BlastResultRow, update func(int, string)) []model.BlastResultRow {
	out := append([]model.BlastResultRow(nil), rows...)
	if len(out) == 0 {
		return out
	}
	progress := safeProgress(update)
	rowsNeedingLabel := blastRowsNeedingHitLabel(out)
	if len(rowsNeedingLabel) == 0 {
		for i := range out {
			if strings.TrimSpace(out[i].LabelNameType) == "" {
				out[i].LabelNameType = "existing row label_name"
			}
			if strings.TrimSpace(out[i].PhgoAliases) == "" {
				out[i].PhgoAliases = strings.TrimSpace(out[i].LabelName)
			}
		}
		progress(len(out), "BLAST hit labels already available.")
		return out
	}
	sourceLabel := blastQueryItemLabelName(item)
	labelSpecies := selected
	if species, ok := w.phytozomeKeywordLabelSpecies(ctx, selected); ok {
		labelSpecies = species
	}
	phytozomeSource, _ := w.phytozomeHitLabelSource(labelSpecies)
	progress(0, fmt.Sprintf("Prefetching BLAST hit label candidates for %d rows...", len(rowsNeedingLabel)))
	keywordRowsByTerm := w.fetchBlastHitKeywordRows(ctx, phytozomeSource, labelSpecies, rowsNeedingLabel)
	taskTimestamp := time.Now().UTC().Format(time.RFC3339Nano)
	identificationsByKey := make(map[string]blastHitLabelIdentification, len(out))
	aliasRequests := make([]labelname.AliasRankRequest, 0, len(rowsNeedingLabel))
	requestKeys := make([]string, 0, len(rowsNeedingLabel))
	requestTypes := make([]string, 0, len(rowsNeedingLabel))
	for _, row := range rowsNeedingLabel {
		cacheKey := blastHitLabelIdentificationCacheKey(row, sourceLabel)
		if _, seen := identificationsByKey[cacheKey]; seen {
			continue
		}
		if cached, ok := w.cachedBlastHitLabelIdentification(cacheKey); ok {
			identificationsByKey[cacheKey] = cached
			continue
		}
		request, labelType, done := blastHitLabelAliasRankRequest(row, sourceLabel, keywordRowsByTerm, taskTimestamp)
		if done {
			identificationsByKey[cacheKey] = blastHitLabelIdentification{
				LabelType: labelType,
			}
			continue
		}
		aliasRequests = append(aliasRequests, request)
		requestKeys = append(requestKeys, cacheKey)
		requestTypes = append(requestTypes, labelType)
	}
	if len(aliasRequests) > 0 {
		progress(0, fmt.Sprintf("Ranking BLAST hit label aliases for %d unique hits...", len(aliasRequests)))
		ranked := labelname.RankAliasBatch(aliasRequests)
		for i := range ranked {
			identification := blastHitLabelIdentification{
				LabelType: requestTypes[i],
				Aliases:   ranked[i].RankedAliases,
			}
			if len(identification.Aliases) > 0 {
				identification.Label = identification.Aliases[0]
			}
			identificationsByKey[requestKeys[i]] = identification
			w.storeBlastHitLabelIdentification(requestKeys[i], identification)
		}
	}
	completed := 0
	for i := range out {
		if strings.TrimSpace(out[i].LabelName) != "" {
			if strings.TrimSpace(out[i].LabelNameType) == "" {
				out[i].LabelNameType = "existing row label_name"
			}
			if strings.TrimSpace(out[i].PhgoAliases) == "" {
				out[i].PhgoAliases = strings.TrimSpace(out[i].LabelName)
			}
			completed++
			progress(minInt(completed, len(out)), fmt.Sprintf("Resolved BLAST hit label names... %d/%d", minInt(completed, len(out)), len(out)))
			continue
		}
		cacheKey := blastHitLabelIdentificationCacheKey(out[i], sourceLabel)
		identification, ok := identificationsByKey[cacheKey]
		if !ok {
			identification = autoIdentifyBlastHitLabelFromKeywordRows(out[i], sourceLabel, keywordRowsByTerm, taskTimestamp)
			identificationsByKey[cacheKey] = identification
			w.storeBlastHitLabelIdentification(cacheKey, identification)
		}
		out[i].LabelName = identification.Label
		out[i].LabelNameType = identification.LabelType
		out[i].PhgoAliases = strings.Join(identification.Aliases, "; ")
		completed++
		progress(minInt(completed, len(out)), fmt.Sprintf("Resolved BLAST hit label names... %d/%d", minInt(completed, len(out)), len(out)))
	}
	if len(out) > 0 {
		progress(len(out), "Finished BLAST hit label identification.")
	}
	return out
}

func (w *BlastWizard) phytozomeHitLabelSource(selected model.SpeciesCandidate) (source.DataSource, bool) {
	if w == nil || w.source == nil {
		return nil, false
	}
	if strings.EqualFold(strings.TrimSpace(w.source.Name()), "phytozome") {
		return w.source, true
	}
	if selected.ProteomeID == 0 && strings.TrimSpace(selected.JBrowseName) == "" {
		return nil, false
	}
	return phytozome.NewClient(w.httpClient), true
}

func blastRowsNeedingHitLabel(rows []model.BlastResultRow) []model.BlastResultRow {
	out := make([]model.BlastResultRow, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.LabelName) == "" {
			out = append(out, row)
		}
	}
	return out
}

func (w *BlastWizard) fetchBlastHitKeywordRows(ctx context.Context, phytozomeSource source.DataSource, selected model.SpeciesCandidate, rows []model.BlastResultRow) map[string][]model.KeywordResultRow {
	terms := make([]string, 0, len(rows)*2)
	for _, row := range rows {
		terms = append(terms, blastHitLabelSearchTerms(row)...)
	}
	return w.fetchKeywordRowsByTerms(ctx, phytozomeSource, selected, terms)
}

type blastHitLabelIdentification struct {
	Label     string
	LabelType string
	Aliases   []string
}

func (w *BlastWizard) cachedBlastHitLabelIdentification(cacheKey string) (blastHitLabelIdentification, bool) {
	cacheKey = strings.TrimSpace(cacheKey)
	if cacheKey == "" {
		return blastHitLabelIdentification{}, false
	}
	w.blastHitLabelLookupMu.RLock()
	result, ok := w.blastHitLabelLookupCache[cacheKey]
	w.blastHitLabelLookupMu.RUnlock()
	if !ok {
		return blastHitLabelIdentification{}, false
	}
	result.Aliases = uniqueStrings(result.Aliases)
	return result, true
}

func (w *BlastWizard) storeBlastHitLabelIdentification(cacheKey string, result blastHitLabelIdentification) {
	cacheKey = strings.TrimSpace(cacheKey)
	if cacheKey == "" {
		return
	}
	result.Label = strings.TrimSpace(result.Label)
	result.LabelType = strings.TrimSpace(result.LabelType)
	result.Aliases = uniqueStrings(result.Aliases)
	w.blastHitLabelLookupMu.Lock()
	if w.blastHitLabelLookupCache == nil {
		w.blastHitLabelLookupCache = make(map[string]blastHitLabelIdentification)
	}
	w.blastHitLabelLookupCache[cacheKey] = result
	w.blastHitLabelLookupMu.Unlock()
}

func autoIdentifyBlastHitLabelFromKeywordRows(row model.BlastResultRow, sourceLabel string, keywordRowsByTerm map[string][]model.KeywordResultRow, taskTimestamp string) blastHitLabelIdentification {
	request, labelType, done := blastHitLabelAliasRankRequest(row, sourceLabel, keywordRowsByTerm, taskTimestamp)
	if done {
		return blastHitLabelIdentification{LabelType: labelType}
	}
	ranked := labelname.RankAliases(request)
	identification := blastHitLabelIdentification{
		LabelType: labelType,
		Aliases:   ranked.RankedAliases,
	}
	if len(identification.Aliases) > 0 {
		identification.Label = identification.Aliases[0]
	}
	return identification
}

func blastHitLabelAliasRankRequest(row model.BlastResultRow, sourceLabel string, keywordRowsByTerm map[string][]model.KeywordResultRow, taskTimestamp string) (labelname.AliasRankRequest, string, bool) {
	taskTimestamp = strings.TrimSpace(taskTimestamp)
	if taskTimestamp == "" {
		taskTimestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	for _, term := range blastHitLabelSearchTerms(row) {
		rows := keywordRowsByTerm[strings.ToLower(strings.TrimSpace(term))]
		for _, candidateRows := range [][]model.KeywordResultRow{
			filterKeywordRowsForBlastHit(rows, row),
			rows,
		} {
			candidates, labelType := blastHitPhytozomeLabelCandidates(candidateRows)
			if len(candidates) == 0 {
				continue
			}
			return labelname.AliasRankRequest{
				TaskTimestamp: taskTimestamp,
				Aliases:       candidates,
			}, labelType, false
		}
	}
	if aliases := lemnaLocalBlastHitAliasCandidates(row); len(aliases) > 0 {
		return labelname.AliasRankRequest{
			TaskTimestamp: taskTimestamp,
			Aliases:       aliases,
		}, "lemna local aliases", false
	}
	if label := strings.TrimSpace(sourceLabel); label != "" {
		return labelname.AliasRankRequest{
			TaskTimestamp: taskTimestamp,
			Aliases:       []string{label},
		}, "blast source labelname fallback", false
	}
	return labelname.AliasRankRequest{TaskTimestamp: taskTimestamp}, "not available", true
}

func blastHitLabelIdentificationCacheKey(row model.BlastResultRow, sourceLabel string) string {
	parts := []string{
		sourceLabel,
		row.SourceDatabase,
		row.Protein,
		row.SubjectID,
		row.TranscriptID,
		row.SequenceID,
		row.Defline,
		row.UniProtGeneNames,
		row.UniProtProteinName,
	}
	for i := range parts {
		parts[i] = strings.ToLower(strings.TrimSpace(parts[i]))
	}
	return strings.Join(parts, "\x00")
}

func blastHitPhytozomeLabelCandidates(rows []model.KeywordResultRow) ([]string, string) {
	return phytozomeAliasCandidatesFromKeywordRows(rows)
}

func phytozomeAliasCandidatesFromKeywordRows(rows []model.KeywordResultRow) ([]string, string) {
	synonyms := make([]string, 0, len(rows)*2)
	symbols := make([]string, 0, len(rows)*2)
	autoDefine := make([]string, 0, len(rows)*2)
	for _, row := range rows {
		synonyms = append(synonyms, labelname.SplitAliases(row.Synonyms)...)
		symbols = append(symbols, labelname.SplitAliases(row.Symbols)...)
		autoDefine = append(autoDefine, labelname.AutoDefineCandidates(row.AutoDefine)...)
	}
	if synonyms = uniqueStrings(synonyms); len(synonyms) > 0 {
		return synonyms, "phytozome synonyms"
	}
	if symbols = uniqueStrings(symbols); len(symbols) > 0 {
		return symbols, "phytozome symbols"
	}
	if autoDefine = uniqueStrings(autoDefine); len(autoDefine) > 0 {
		return autoDefine, "phytozome auto_define"
	}
	return nil, ""
}

func blastHitLabelSearchTerms(row model.BlastResultRow) []string {
	terms := make([]string, 0, 6)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range terms {
			if strings.EqualFold(existing, value) {
				return
			}
		}
		terms = append(terms, value)
	}
	add(row.Protein)
	add(row.SubjectID)
	add(row.TranscriptID)
	add(row.SequenceID)
	if geneID := stripTranscriptSuffix(firstNonEmpty(row.TranscriptID, row.SequenceID, row.Protein, row.SubjectID)); geneID != "" {
		add(geneID)
	}
	return terms
}

func lemnaLocalBlastHitAliasCandidates(row model.BlastResultRow) []string {
	if !strings.EqualFold(strings.TrimSpace(row.SourceDatabase), "lemna") {
		return nil
	}
	return lemnaLocalAliasCandidates(lemnaLocalAliasSeed{
		LabelName:   row.LabelName,
		PhgoAliases: row.PhgoAliases,
		Aliases:     firstNonEmpty(row.UniProtGeneNames, row.UniProtProteinName),
		AutoDefine:  row.Defline,
	})
}

func filterKeywordRowsForBlastHit(rows []model.KeywordResultRow, hit model.BlastResultRow) []model.KeywordResultRow {
	if len(rows) == 0 {
		return nil
	}
	targets := make([]string, 0, 5)
	for _, value := range []string{hit.Protein, hit.SubjectID, hit.TranscriptID, hit.SequenceID, stripTranscriptSuffix(firstNonEmpty(hit.TranscriptID, hit.SequenceID, hit.Protein, hit.SubjectID))} {
		value = strings.TrimSpace(value)
		if value != "" {
			targets = append(targets, strings.ToLower(value))
		}
	}
	if len(targets) == 0 {
		return rows
	}
	matches := make([]model.KeywordResultRow, 0, len(rows))
	for _, row := range rows {
		haystack := strings.ToLower(strings.Join([]string{
			row.ProteinID,
			row.TranscriptID,
			row.SequenceID,
			row.GeneIdentifier,
		}, " "))
		for _, target := range targets {
			if strings.Contains(haystack, target) {
				matches = append(matches, row)
				break
			}
		}
	}
	return matches
}

func annotateBlastRowsForQueryContext(rows []model.BlastResultRow, item blastQueryItem) []model.BlastResultRow {
	if len(rows) == 0 {
		return rows
	}
	family := strings.TrimSpace(item.FamilyName)
	if family == "" {
		settings := model.DefaultFamilyBlastSettings()
		if detected := detectFamilyName(familyBlastQueryLabel(item), settings); detected != "" {
			family = detected
		}
	}
	if family == "" {
		return append([]model.BlastResultRow(nil), rows...)
	}
	memberLabels := []string{familyBlastQueryLabel(item)}
	aliasTexts := []string{
		strings.TrimSpace(item.LabelName),
	}
	if item.QuerySource != nil {
		aliasTexts = append(aliasTexts, storedQuerySourceAliases(item.QuerySource)...)
	}
	return annotateFamilyBlastConsensusRows(rows, family, uniqueStrings(memberLabels), uniqueStrings(aliasTexts))
}

func coordinateSpan(from int, to int) int {
	if from <= 0 || to <= 0 {
		return 0
	}
	if from > to {
		from, to = to, from
	}
	return to - from + 1
}

func (w *BlastWizard) enrichBlastRowsWithUniProt(ctx context.Context, rows []model.BlastResultRow) ([]model.BlastResultRow, error) {
	if len(rows) == 0 {
		return rows, nil
	}
	client := w.sharedUniProtClient()
	out := append([]model.BlastResultRow(nil), rows...)
	if inheritedUpdate := updateFromContext(ctx); inheritedUpdate != nil {
		return w.enrichBlastRowsWithUniProtProgress(ctx, client, out, inheritedUpdate)
	}
	if w.suppressTaskModals {
		return w.enrichBlastRowsWithUniProtProgress(ctx, client, out, nil)
	}
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "External references", "UniProt"),
		Title:       "Adding UniProt reference columns",
		Description: "Fetching UniProt annotations for BLAST result rows.",
		Initial:     "Fetching UniProt annotations...",
		Total:       uniProtLookupGroupCount(out),
		CancelError: prompt.ErrBackToQueryInput,
	}, func(taskCtx context.Context, update func(int, string)) ([]model.BlastResultRow, error) {
		return w.enrichBlastRowsWithUniProtProgress(mergeContexts(ctx, taskCtx), client, out, update)
	})
}

func (w *BlastWizard) enrichBlastRowsWithUniProtProgress(ctx context.Context, client *uniprot.Client, rows []model.BlastResultRow, update func(int, string)) ([]model.BlastResultRow, error) {
	progress := safeProgress(update)
	references := blastReferenceConfigFromContext(ctx)
	for i := range rows {
		rows[i].UniProtReferenceEnabled = true
	}
	progress(0, "Prefetching UniProt accessions...")
	w.prefetchBlastRowUniProtAccessions(ctx, rows)
	groups := uniProtLookupGroups(rows)
	progress(0, fmt.Sprintf("Resolving UniProt references... 0/%d", len(groups)))
	results := make(map[string]uniProtLookupResult, len(groups))
	var resultMu sync.Mutex
	jobs := make(chan int)
	workerCount := blastUniProtWorkerCountForConfig(len(groups), references)
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for groupIndex := range jobs {
				group := groups[groupIndex]
				result, cached := w.cachedUniProtLookupResult(group.Key)
				if !cached {
					entry, ok, err := w.lookupUniProtEntry(ctx, client, rows[group.Rows[0]])
					result = uniProtLookupResult{entry: entry, ok: ok, err: err}
					w.storeUniProtLookupResult(group.Key, result)
				}
				resultMu.Lock()
				results[group.Key] = result
				done := len(results)
				resultMu.Unlock()
				progress(done, fmt.Sprintf("Checked UniProt reference %d/%d", done, len(groups)))
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i := range groups {
			select {
			case <-ctx.Done():
				return
			case jobs <- i:
			}
		}
	}()
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, group := range groups {
		result := results[group.Key]
		if result.err != nil || !result.ok {
			continue
		}
		for _, rowIndex := range group.Rows {
			applyUniProtEntry(&rows[rowIndex], result.entry)
		}
	}
	return rows, nil
}

func (w *BlastWizard) prefetchBlastRowUniProtAccessions(ctx context.Context, rows []model.BlastResultRow) {
	if w == nil || len(rows) == 0 {
		return
	}
	pending := make([]model.BlastResultRow, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		key := blastRowAccessionCacheKey(row)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if _, ok := w.cachedRowUniProtAccessions(row); ok {
			continue
		}
		pending = append(pending, row)
	}
	if len(pending) == 0 {
		return
	}
	workerCount := blastUniProtAccessionWorkerCountForConfig(len(pending), blastReferenceConfigFromContext(ctx))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for idx := range jobs {
				_ = w.uniprotAccessionsForBlastRow(ctx, pending[idx])
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i := range pending {
			select {
			case <-ctx.Done():
				return
			case jobs <- i:
			}
		}
	}()
	workers.Wait()
}

type uniProtLookupGroup struct {
	Key  string
	Rows []int
}

func uniProtLookupGroups(rows []model.BlastResultRow) []uniProtLookupGroup {
	indexByKey := make(map[string]int, len(rows))
	groups := make([]uniProtLookupGroup, 0, len(rows))
	for i, row := range rows {
		key := uniProtLookupKey(row)
		if groupIndex, ok := indexByKey[key]; ok {
			groups[groupIndex].Rows = append(groups[groupIndex].Rows, i)
			continue
		}
		indexByKey[key] = len(groups)
		groups = append(groups, uniProtLookupGroup{Key: key, Rows: []int{i}})
	}
	return groups
}

func uniProtLookupGroupCount(rows []model.BlastResultRow) int {
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		seen[uniProtLookupKey(row)] = struct{}{}
	}
	return len(seen)
}

func uniProtLookupKey(row model.BlastResultRow) string {
	parts := []string{
		row.UniProtAccession,
		row.Protein,
		row.SubjectID,
		row.SequenceID,
		row.TranscriptID,
		row.Species,
		row.Defline,
	}
	for i := range parts {
		parts[i] = strings.ToLower(strings.TrimSpace(parts[i]))
	}
	return strings.Join(parts, "\x00")
}

func (w *BlastWizard) cachedUniProtLookupResult(key string) (uniProtLookupResult, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return uniProtLookupResult{}, false
	}
	w.uniProtLookupMu.RLock()
	result, ok := w.uniProtLookupCache[key]
	w.uniProtLookupMu.RUnlock()
	if !ok {
		return uniProtLookupResult{}, false
	}
	return result, true
}

func (w *BlastWizard) storeUniProtLookupResult(key string, result uniProtLookupResult) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	w.uniProtLookupMu.Lock()
	if w.uniProtLookupCache == nil {
		w.uniProtLookupCache = make(map[string]uniProtLookupResult)
	}
	w.uniProtLookupCache[key] = result
	w.uniProtLookupMu.Unlock()
}

func (w *BlastWizard) lookupUniProtEntry(ctx context.Context, client *uniprot.Client, row model.BlastResultRow) (uniprot.Entry, bool, error) {
	accessions := w.uniprotAccessionsForBlastRow(ctx, row)
	if strings.TrimSpace(row.UniProtAccession) != "" {
		accessions = append([]string{row.UniProtAccession}, accessions...)
	}
	accessions = uniqueStrings(accessions)
	var lastErr error
	for _, accession := range accessions {
		entry, ok, err := client.Lookup(ctx, accession, row)
		if err != nil {
			lastErr = err
			continue
		}
		if err == nil && ok {
			return entry, true, nil
		}
	}
	entry, ok, err := client.Lookup(ctx, "", row)
	if err != nil || !ok {
		if err != nil {
			lastErr = err
		}
		return uniprot.Entry{}, false, lastErr
	}
	return entry, true, nil
}

func uniqueStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (w *BlastWizard) uniprotAccessionsForBlastRow(ctx context.Context, row model.BlastResultRow) []string {
	if cached, ok := w.cachedRowUniProtAccessions(row); ok {
		return cached
	}
	cacheKey := blastRowAccessionCacheKey(row)
	resolver, ok := w.source.(source.UniProtResolver)
	if !ok {
		w.storeRowUniProtAccessions(row, nil)
		return nil
	}
	proteinID := firstNonEmpty(row.Protein, row.SubjectID, row.SequenceID, row.TranscriptID)
	if proteinID == "" {
		w.storeRowUniProtAccessions(row, nil)
		return nil
	}
	targetID := row.TargetID
	if targetID == 0 {
		targetID = w.phytozomeTargetIDForRow(ctx, row)
	}
	if targetID == 0 {
		w.storeRowUniProtAccessions(row, nil)
		return nil
	}
	value, err, _ := w.rowUniProtAccessionsGroup.Do(cacheKey, func() (any, error) {
		if cached, ok := w.cachedRowUniProtAccessions(row); ok {
			return cached, nil
		}
		accessions, err := resolver.FetchUniProtAccessions(ctx, targetID, proteinID)
		if err != nil {
			return nil, err
		}
		accessions = uniqueStrings(accessions)
		w.storeRowUniProtAccessions(row, accessions)
		return accessions, nil
	})
	if err != nil {
		w.storeRowUniProtAccessions(row, nil)
		return nil
	}
	accessions, _ := value.([]string)
	return append([]string(nil), accessions...)
}

func (w *BlastWizard) phytozomeTargetIDForRow(ctx context.Context, row model.BlastResultRow) int {
	jbrowseName := strings.TrimSpace(row.JBrowseName)
	if jbrowseName == "" {
		if normalizedURL, ok := normalizeGeneReportURL(row.GeneReportURL); ok {
			parsedJBrowseName, _, _, err := parseGeneReportURL(normalizedURL)
			if err == nil {
				jbrowseName = parsedJBrowseName
			}
		}
	}
	if jbrowseName == "" {
		return 0
	}
	candidates, err := w.speciesCandidatesForSource(ctx, w.source, nil)
	if err == nil {
		if species, ok := findSpeciesCandidateByJBrowseName(candidates, jbrowseName); ok {
			return species.ProteomeID
		}
	}
	if _, ok := w.source.(*phytozome.Client); ok {
		return 0
	}
	phytozomeSource := phytozome.NewClient(w.httpClient)
	candidates, err = w.speciesCandidatesForSource(ctx, phytozomeSource, nil)
	if err != nil {
		return 0
	}
	if species, ok := findSpeciesCandidateByJBrowseName(candidates, jbrowseName); ok {
		return species.ProteomeID
	}
	return 0
}

func applyUniProtEntry(row *model.BlastResultRow, entry uniprot.Entry) {
	row.UniProtAccession = entry.Accession
	row.UniProtReviewed = entry.Reviewed
	row.UniProtProteinName = entry.ProteinName
	row.UniProtGeneNames = entry.GeneNames
	row.UniProtKeywords = entry.Keywords
	row.UniProtEC = entry.EC
	row.UniProtGO = entry.GO
	row.UniProtCanonicalLength = ""
	if entry.Length > 0 {
		row.UniProtCanonicalLength = strconv.Itoa(entry.Length)
	}
	if row.TargetLength > 0 && entry.Length > 0 {
		row.TargetUniProtCanonicalLengthPercent = fmt.Sprintf("%.2f", float64(row.TargetLength)/float64(entry.Length)*100)
	}
	row.UniProtEntryName = entry.EntryName
	row.UniProtOrganism = entry.Organism
	row.UniProtOrganismID = entry.OrganismID
	row.UniProtFunction = entry.Function
	row.UniProtCatalyticActivity = entry.CatalyticActivity
	row.UniProtGOIDs = entry.GOIDs
	row.UniProtPathway = entry.Pathway
	row.UniProtSubcellularLocation = entry.SubcellularLocation
	row.UniProtProteinExistence = entry.ProteinExistence
	row.UniProtAnnotationScore = entry.AnnotationScore
	row.UniProtFragment = entry.Fragment
	row.UniProtSequenceCaution = entry.SequenceCaution
	row.UniProtPfam = entry.Pfam
	row.UniProtInterPro = entry.InterPro
	row.UniProtDomain = entry.Domain
	row.UniProtRegion = entry.Region
	row.UniProtMotif = entry.Motif
	row.UniProtActiveSite = entry.ActiveSite
	row.UniProtBindingSite = entry.BindingSite
	row.UniProtAlphaFoldDB = entry.AlphaFoldDB
	row.UniProtPDB = entry.PDB
}

func (w *BlastWizard) enrichBlastRowsWithInterPro(ctx context.Context, item blastQueryItem, rows []model.BlastResultRow, settings model.InterProConservedRegionSettings) ([]model.BlastResultRow, error) {
	if len(rows) == 0 {
		return rows, nil
	}
	settings = normalizeInterProConservedRegionSettings(settings)
	client := w.sharedInterProClient()
	out := append([]model.BlastResultRow(nil), rows...)
	if inheritedUpdate := updateFromContext(ctx); inheritedUpdate != nil {
		return w.enrichBlastRowsWithInterProProgress(ctx, client, item, out, settings, inheritedUpdate)
	}
	if w.suppressTaskModals {
		return w.enrichBlastRowsWithInterProProgress(ctx, client, item, out, settings, nil)
	}
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "External references", "InterPro"),
		Title:       "Adding InterPro reference columns",
		Description: "Fetching InterPro protein family, domain, motif, and signature matches for BLAST result rows.",
		Initial:     "Fetching InterPro annotations...",
		Total:       interProLookupGroupCount(out) + 1,
		CancelError: prompt.ErrBackToQueryInput,
	}, func(taskCtx context.Context, update func(int, string)) ([]model.BlastResultRow, error) {
		return w.enrichBlastRowsWithInterProProgress(mergeContexts(ctx, taskCtx), client, item, out, settings, update)
	})
}

func (w *BlastWizard) enrichBlastRowsWithInterProProgress(ctx context.Context, client *interpro.Client, item blastQueryItem, rows []model.BlastResultRow, settings model.InterProConservedRegionSettings, update func(int, string)) ([]model.BlastResultRow, error) {
	progress := safeProgress(update)
	references := blastReferenceConfigFromContext(ctx)
	for i := range rows {
		rows[i].InterProReferenceEnabled = true
	}
	progress(0, "Resolving InterPro query reference...")
	queryEntry, queryOK := w.lookupInterProQueryEntry(ctx, client, item)
	progress(1, "Checked InterPro query reference")
	groups := interProLookupGroups(rows)
	progress(1, fmt.Sprintf("Resolving InterPro hit references... 0/%d", len(groups)))
	results := make(map[string]interProLookupResult, len(groups))
	var resultMu sync.Mutex
	jobs := make(chan int)
	workerCount := blastInterProWorkerCountForConfig(len(groups), references)
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for groupIndex := range jobs {
				group := groups[groupIndex]
				result, cached := w.cachedInterProLookupResult(group.Key)
				if !cached {
					entry, ok, err := w.lookupInterProEntry(ctx, client, rows[group.Rows[0]])
					result = interProLookupResult{entry: entry, ok: ok, err: err}
					w.storeInterProLookupResult(group.Key, result)
				}
				resultMu.Lock()
				results[group.Key] = result
				done := len(results) + 1
				resultMu.Unlock()
				progress(done, fmt.Sprintf("Checked InterPro reference %d/%d", len(results), len(groups)))
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i := range groups {
			select {
			case <-ctx.Done():
				return
			case jobs <- i:
			}
		}
	}()
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, group := range groups {
		result := results[group.Key]
		if result.err != nil || !result.ok {
			continue
		}
		for _, rowIndex := range group.Rows {
			applyInterProEntry(&rows[rowIndex], result.entry)
			rows[rowIndex].InterProConservedRegionStatus = interProConservedRegionStatus(queryEntry, queryOK, result.entry, settings)
		}
	}
	return rows, nil
}

func (w *BlastWizard) lookupInterProQueryEntry(ctx context.Context, client *interpro.Client, item blastQueryItem) (interpro.Entry, bool) {
	if item.QuerySource == nil {
		return interpro.Entry{}, false
	}
	row := w.interProQueryLookupRow(item, ctx)
	entry, ok, _ := w.lookupInterProEntry(ctx, client, row)
	return entry, ok
}

func (w *BlastWizard) interProQueryLookupRow(item blastQueryItem, ctx context.Context) model.BlastResultRow {
	if item.QuerySource == nil {
		return model.BlastResultRow{}
	}
	source := item.QuerySource
	row := model.BlastResultRow{
		Protein:          firstNonEmpty(source.ProteinID, source.TranscriptID, source.GeneID),
		SubjectID:        firstNonEmpty(source.ProteinID, source.TranscriptID, source.GeneID),
		SequenceID:       firstNonEmpty(source.ProteinID, source.TranscriptID),
		TranscriptID:     source.TranscriptID,
		Species:          source.OrganismShort,
		GeneReportURL:    firstNonEmpty(source.NormalizedURL, source.OriginalInputURL),
		JBrowseName:      source.SourceJBrowseName,
		TargetID:         source.SourceProteomeID,
		Defline:          source.Annotation,
		UniProtAccession: strings.TrimSpace(source.UniProtAccession),
	}
	if strings.TrimSpace(row.UniProtAccession) == "" {
		if ctx == nil {
			ctx = context.Background()
		}
		if accessions := w.uniprotAccessionsForBlastRow(ctx, row); len(accessions) > 0 {
			row.UniProtAccession = strings.TrimSpace(accessions[0])
		}
	}
	return row
}

func (w *BlastWizard) lookupInterProEntry(ctx context.Context, client *interpro.Client, row model.BlastResultRow) (interpro.Entry, bool, error) {
	accessions := w.uniprotAccessionsForBlastRow(ctx, row)
	if strings.TrimSpace(row.UniProtAccession) != "" {
		accessions = append([]string{row.UniProtAccession}, accessions...)
	}
	accessions = uniqueStrings(accessions)
	for _, accession := range accessions {
		entry, ok, err := client.Lookup(ctx, accession)
		if err != nil {
			continue
		}
		if ok {
			return entry, true, nil
		}
	}
	return interpro.Entry{}, false, nil
}

type interProLookupGroup struct {
	Key  string
	Rows []int
}

func interProLookupGroups(rows []model.BlastResultRow) []interProLookupGroup {
	indexByKey := make(map[string]int, len(rows))
	groups := make([]interProLookupGroup, 0, len(rows))
	for i, row := range rows {
		key := interProLookupKey(row)
		if groupIndex, ok := indexByKey[key]; ok {
			groups[groupIndex].Rows = append(groups[groupIndex].Rows, i)
			continue
		}
		indexByKey[key] = len(groups)
		groups = append(groups, interProLookupGroup{Key: key, Rows: []int{i}})
	}
	return groups
}

func interProLookupGroupCount(rows []model.BlastResultRow) int {
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		seen[interProLookupKey(row)] = struct{}{}
	}
	return len(seen)
}

func interProLookupKey(row model.BlastResultRow) string {
	parts := []string{
		row.UniProtAccession,
		row.Protein,
		row.SubjectID,
		row.SequenceID,
		row.TranscriptID,
		row.Species,
		row.Defline,
	}
	for i := range parts {
		parts[i] = strings.ToLower(strings.TrimSpace(parts[i]))
	}
	return strings.Join(parts, "\x00")
}

func (w *BlastWizard) cachedInterProLookupResult(key string) (interProLookupResult, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return interProLookupResult{}, false
	}
	w.interProLookupMu.RLock()
	result, ok := w.interProLookupCache[key]
	w.interProLookupMu.RUnlock()
	if !ok {
		return interProLookupResult{}, false
	}
	return result, true
}

func (w *BlastWizard) storeInterProLookupResult(key string, result interProLookupResult) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	w.interProLookupMu.Lock()
	if w.interProLookupCache == nil {
		w.interProLookupCache = make(map[string]interProLookupResult)
	}
	w.interProLookupCache[key] = result
	w.interProLookupMu.Unlock()
}

func applyInterProEntry(row *model.BlastResultRow, entry interpro.Entry) {
	row.InterProAccessions = entry.Accessions
	row.InterProEntryName = entry.EntryNames
	row.InterProEntryType = entry.EntryTypes
	row.InterProCoveragePercent = entry.CoveragePercent
	row.InterProMatchRegions = entry.MatchRegions
	row.InterProSignatureAccessions = entry.SignatureAccessions
	row.InterProPfamAccessions = entry.PfamAccessions
}

func interProConservedRegionStatus(query interpro.Entry, queryOK bool, hit interpro.Entry, settings model.InterProConservedRegionSettings) string {
	if len(hit.Matches) == 0 {
		return ""
	}
	if !queryOK || len(query.Matches) == 0 {
		return interProSelfEvidenceStatus(hit, settings)
	}
	matchedItems, matchedCoverage := interProMatchedQueryEvidence(query, hit, settings)
	switch {
	case matchedItems >= settings.PresentMinMatchedItems && matchedCoverage >= settings.PresentMinCoverage:
		return "present"
	case matchedItems >= settings.PartialMinMatchedItems && matchedCoverage >= settings.PartialMinCoverage:
		return "partial"
	case matchedItems > 0:
		return "partial"
	default:
		return "missing"
	}
}

func interProSelfEvidenceStatus(hit interpro.Entry, settings model.InterProConservedRegionSettings) string {
	conservedItems := 0
	bestCoverage := 0.0
	for _, match := range hit.Matches {
		if !interProMatchIsConservedCandidate(match, settings) {
			continue
		}
		conservedItems++
		if match.CoveragePercent > bestCoverage {
			bestCoverage = match.CoveragePercent
		}
	}
	if conservedItems == 0 {
		return "missing"
	}
	if conservedItems >= settings.PresentMinMatchedItems && (!settings.UseCoverage || bestCoverage >= settings.PresentMinCoverage) {
		return "present"
	}
	if conservedItems >= settings.PartialMinMatchedItems && (!settings.UseCoverage || bestCoverage >= settings.PartialMinCoverage) {
		return "partial"
	}
	return "uncertain"
}

func interProMatchedQueryEvidence(query interpro.Entry, hit interpro.Entry, settings model.InterProConservedRegionSettings) (int, float64) {
	totalQueryCoverage := 0
	matchedQueryCoverage := 0
	matchedItems := 0
	for _, queryMatch := range query.Matches {
		if !interProMatchIsConservedCandidate(queryMatch, settings) {
			continue
		}
		if queryMatch.CoverageLength > 0 {
			totalQueryCoverage += queryMatch.CoverageLength
		}
		best := interProBestHitMatch(queryMatch, hit.Matches, settings)
		if best == nil {
			continue
		}
		matchedItems++
		if best.CoverageLength > 0 {
			matchedQueryCoverage += min(best.CoverageLength, queryMatch.CoverageLength)
		}
	}
	if totalQueryCoverage <= 0 {
		if matchedItems > 0 {
			return matchedItems, 100
		}
		return 0, 0
	}
	return matchedItems, float64(matchedQueryCoverage) / float64(totalQueryCoverage) * 100
}

func interProMatchIsConservedCandidate(match interpro.Match, settings model.InterProConservedRegionSettings) bool {
	if !settings.UseEntryType {
		return true
	}
	entryType := strings.ToLower(strings.TrimSpace(match.Type))
	return entryType == "" || entryType == "domain" || entryType == "family" || entryType == "homologous_superfamily" || entryType == "repeat" || entryType == "site"
}

func interProBestHitMatch(query interpro.Match, hits []interpro.Match, settings model.InterProConservedRegionSettings) *interpro.Match {
	bestIndex := -1
	bestScore := 0
	for i, hit := range hits {
		score := interProEvidenceScore(query, hit, settings)
		if score > bestScore {
			bestScore = score
			bestIndex = i
		}
	}
	if bestIndex < 0 || bestScore <= 0 {
		return nil
	}
	return &hits[bestIndex]
}

func interProEvidenceScore(query interpro.Match, hit interpro.Match, settings model.InterProConservedRegionSettings) int {
	score := 0
	if settings.UsePfamAccession && intersects(query.PfamAccessions, hit.PfamAccessions) {
		score += 5
	}
	if settings.UseInterProAccession && query.Accession != "" && hit.Accession != "" && strings.EqualFold(query.Accession, hit.Accession) {
		score += 4
	}
	if settings.UseSignatureAccession && intersects(query.SignatureAccessions, hit.SignatureAccessions) {
		score += 3
	}
	if settings.UseEntryType && query.Type != "" && hit.Type != "" && strings.EqualFold(query.Type, hit.Type) {
		score++
	}
	if settings.UseEntryName && query.Name != "" && hit.Name != "" && strings.EqualFold(query.Name, hit.Name) {
		score++
	}
	if settings.UseMatchRegions && query.CoverageLength > 0 && hit.CoverageLength > 0 {
		score++
	}
	return score
}

func intersects(left []string, right []string) bool {
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	for _, value := range right {
		value = strings.ToLower(strings.TrimSpace(value))
		if _, ok := seen[value]; ok {
			return true
		}
	}
	return false
}

func normalizeInterProConservedRegionSettings(settings model.InterProConservedRegionSettings) model.InterProConservedRegionSettings {
	defaults := model.DefaultInterProConservedRegionSettings()
	if !settings.UsePfamAccession && !settings.UseInterProAccession && !settings.UseSignatureAccession && !settings.UseEntryType && !settings.UseEntryName && !settings.UseCoverage && !settings.UseMatchRegions {
		return defaults
	}
	if settings.PresentMinCoverage <= 0 {
		settings.PresentMinCoverage = defaults.PresentMinCoverage
	}
	if settings.PartialMinCoverage <= 0 {
		settings.PartialMinCoverage = defaults.PartialMinCoverage
	}
	if settings.PresentMinMatchedItems <= 0 {
		settings.PresentMinMatchedItems = defaults.PresentMinMatchedItems
	}
	if settings.PartialMinMatchedItems <= 0 {
		settings.PartialMinMatchedItems = defaults.PartialMinMatchedItems
	}
	if settings.PartialMinCoverage > settings.PresentMinCoverage {
		settings.PartialMinCoverage = settings.PresentMinCoverage
	}
	return settings
}

func canonicalBlastProgram(program string) string {
	program = strings.TrimSpace(program)
	if strings.HasPrefix(strings.ToLower(program), "local:") {
		program = strings.TrimSpace(program[len("local:"):])
	}
	return strings.ToUpper(program)
}

func cloneBlastQueryItems(items []blastQueryItem) []blastQueryItem {
	out := make([]blastQueryItem, len(items))
	copy(out, items)
	for i := range out {
		if items[i].QuerySource != nil {
			source := *items[i].QuerySource
			out[i].QuerySource = &source
		}
		if len(items[i].FamilySources) > 0 {
			out[i].FamilySources = make([]*model.QuerySequenceSource, 0, len(items[i].FamilySources))
			for _, source := range items[i].FamilySources {
				if source == nil {
					out[i].FamilySources = append(out[i].FamilySources, nil)
					continue
				}
				sourceCopy := *source
				out[i].FamilySources = append(out[i].FamilySources, &sourceCopy)
			}
		}
	}
	return out
}

func cloneBlastQueryRuns(runs []blastQueryRun) []blastQueryRun {
	out := make([]blastQueryRun, len(runs))
	copy(out, runs)
	for i := range out {
		out[i].Results.Rows = append([]model.BlastResultRow(nil), runs[i].Results.Rows...)
		out[i].SelectedRows = append([]model.BlastResultRow(nil), runs[i].SelectedRows...)
	}
	return out
}

func detectFamilyBlastGroups(items []blastQueryItem, settings model.FamilyBlastSettings) []familyBlastGroup {
	if len(items) <= 1 || !settings.GroupByDetectedPrefix {
		return nil
	}
	if settings.MinimumGroupSize < 2 {
		settings.MinimumGroupSize = 2
	}
	indexesByFamily := make(map[string][]int, len(items))
	labelsByFamily := make(map[string][]string, len(items))
	membersByFamily := make(map[string][]familyBlastMember, len(items))
	order := make([]string, 0, len(items))
	for i, item := range items {
		label := familyBlastQueryLabel(item)
		family := detectFamilyName(label, settings)
		if family == "" {
			continue
		}
		groupKey := family
		if settings.KeepDistinctQuerySubgroups {
			if subgroup := familyBlastSubgroupKey(item, settings); subgroup != "" {
				groupKey = family + "|" + subgroup
			}
		}
		if _, ok := indexesByFamily[groupKey]; !ok {
			order = append(order, groupKey)
		}
		indexesByFamily[groupKey] = append(indexesByFamily[groupKey], i)
		labelsByFamily[groupKey] = append(labelsByFamily[groupKey], label)
		membersByFamily[groupKey] = append(membersByFamily[groupKey], familyBlastMemberForItem(item))
	}
	out := make([]familyBlastGroup, 0, len(order))
	for _, groupKey := range order {
		indexes := indexesByFamily[groupKey]
		if len(indexes) < settings.MinimumGroupSize {
			continue
		}
		family := groupKey
		if pipe := strings.Index(groupKey, "|"); pipe >= 0 {
			family = groupKey[:pipe]
		}
		out = append(out, familyBlastGroup{
			Name:          family,
			Indexes:       append([]int(nil), indexes...),
			Labels:        uniqueStrings(labelsByFamily[groupKey]),
			Members:       uniqueFamilyBlastMembers(membersByFamily[groupKey]),
			GroupSource:   "automatic detection",
			DetectionRule: familyBlastAutoDetectionRule(settings),
		})
	}
	return out
}

func uniqueFamilyBlastMembers(members []familyBlastMember) []familyBlastMember {
	out := make([]familyBlastMember, 0, len(members))
	seen := make(map[string]struct{}, len(members))
	for _, member := range members {
		key := strings.ToLower(firstNonEmpty(member.SourceKey, member.ProteinID, member.OriginalLabelName, member.LabelName))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, member)
	}
	return out
}

func familyBlastAutoDetectionRule(settings model.FamilyBlastSettings) string {
	parts := []string{"auto-detected from query labels"}
	modifiers := make([]string, 0, 5)
	if settings.StripLeadingSpeciesPrefix {
		modifiers = append(modifiers, "strip species prefix")
	}
	if settings.StripTrailingQueryIndex {
		modifiers = append(modifiers, "strip trailing index")
	}
	if settings.StripAfterNumberSuffix {
		modifiers = append(modifiers, "ignore post-number suffix")
	}
	if settings.StripTerminalSubtypeSuffix {
		modifiers = append(modifiers, "strip subtype suffix")
	}
	if settings.KeepDistinctQuerySubgroups {
		modifiers = append(modifiers, "keep subgroups distinct")
	}
	if len(modifiers) == 0 {
		return parts[0]
	}
	return parts[0] + "; " + strings.Join(modifiers, ", ")
}

func familyBlastSubgroupKey(item blastQueryItem, settings model.FamilyBlastSettings) string {
	for _, value := range []string{
		strings.TrimSpace(item.LabelName),
		func() string {
			if item.QuerySource == nil {
				return ""
			}
			return strings.TrimSpace(item.QuerySource.LabelName)
		}(),
		preferredStoredQuerySourceAlias(item.QuerySource),
	} {
		if value == "" {
			continue
		}
		if subgroup := familyBlastCanonicalSubgroupLabel(value, settings); subgroup != "" {
			return subgroup
		}
	}
	return ""
}

func familyBlastCanonicalSubgroupLabel(label string, settings model.FamilyBlastSettings) string {
	label = familyBlastCanonicalLabel(label, settings)
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	return strings.ToUpper(label)
}

func familyBlastQueryLabel(item blastQueryItem) string {
	for _, value := range []string{
		item.LabelName,
		func() string {
			if item.QuerySource == nil {
				return ""
			}
			return item.QuerySource.LabelName
		}(),
		preferredStoredQuerySourceAlias(item.QuerySource),
		func() string {
			if item.QuerySource == nil {
				return ""
			}
			return firstNonEmpty(item.QuerySource.GeneID, item.QuerySource.TranscriptID, item.QuerySource.ProteinID)
		}(),
		item.RawInput,
	} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func familyBlastMemberForItem(item blastQueryItem) familyBlastMember {
	label := strings.TrimSpace(familyBlastQueryLabel(item))
	if label == "" {
		label = strings.TrimSpace(item.LabelName)
	}
	proteinID := ""
	aliases := make([]string, 0, 8)
	if item.QuerySource != nil {
		proteinID = firstNonEmpty(item.QuerySource.ProteinID, item.QuerySource.TranscriptID, item.QuerySource.GeneID)
		aliases = append(aliases, storedQuerySourceAliases(item.QuerySource)...)
	}
	aliases = append(aliases, item.LabelName, label)
	sourceKey := familyBlastMemberSourceKey(item, label, proteinID)
	return familyBlastMember{
		LabelName:         label,
		ProteinID:         proteinID,
		Aliases:           uniqueStrings(aliases),
		OriginalLabelName: label,
		SourceKey:         sourceKey,
	}
}

func familyBlastMemberSourceKey(item blastQueryItem, label string, proteinID string) string {
	if item.QuerySource != nil {
		if proteinID != "" {
			return strings.Join([]string{
				strings.TrimSpace(item.QuerySource.SourceDatabase),
				strconv.Itoa(item.QuerySource.SourceProteomeID),
				proteinID,
			}, "|")
		}
		for _, value := range []string{item.QuerySource.OriginalInputURL, item.QuerySource.NormalizedURL, item.QuerySource.GeneID, item.QuerySource.TranscriptID} {
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	if strings.TrimSpace(item.RawInput) != "" {
		return strings.TrimSpace(item.RawInput)
	}
	return strings.TrimSpace(firstNonEmpty(proteinID, label))
}

func setBlastQueryItemLabel(item *blastQueryItem, label string) {
	if item == nil {
		return
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return
	}
	item.LabelName = label
	if item.QuerySource != nil {
		item.QuerySource.LabelName = label
	}
}

func mergeBlastQueryItemAliases(item *blastQueryItem, aliases []string) {
	if item == nil || len(aliases) == 0 {
		return
	}
	combined := append([]string(nil), storedQuerySourceAliases(item.QuerySource)...)
	if item.QuerySource != nil {
		combined = append(combined, labelname.SplitAliases(item.QuerySource.Aliases)...)
	}
	combined = append(combined, aliases...)
	combined = append(combined, item.LabelName)
	if item.QuerySource != nil {
		item.QuerySource.PhgoAliases = strings.Join(uniqueStrings(combined), "; ")
	}
}

func storedQuerySourceAliases(source *model.QuerySequenceSource) []string {
	if source == nil {
		return nil
	}
	aliases := make([]string, 0, 8)
	aliases = append(aliases, labelname.SplitAliases(source.PhgoAliases)...)
	if len(aliases) == 0 {
		aliases = append(aliases, source.LabelName)
		aliases = append(aliases, querySourceLabelnameCandidates(source)...)
		if len(aliases) == 0 {
			aliases = append(aliases, firstNonEmpty(source.ProteinID, source.TranscriptID, source.GeneID))
		}
	}
	return uniqueStrings(aliases)
}

func querySourceHasReusableAliasData(source *model.QuerySequenceSource) bool {
	if source == nil {
		return false
	}
	if len(labelname.SplitAliases(source.PhgoAliases)) > 0 {
		return true
	}
	return false
}

func preferredStoredQuerySourceAlias(source *model.QuerySequenceSource) string {
	aliases := storedQuerySourceAliases(source)
	if len(aliases) == 0 {
		return ""
	}
	return aliases[0]
}

func detectFamilyName(label string, settings model.FamilyBlastSettings) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	label = familyBlastCanonicalLabel(label, settings)
	if settings.StripAfterNumberSuffix {
		label = stripAfterFamilyMemberNumber(label)
	}
	if settings.StripTrailingQueryIndex {
		label = stripFamilyTrailingIndex(label)
	}
	label = strings.Trim(label, " ._-")
	if label == "" {
		return ""
	}
	return strings.ToUpper(label)
}

func familyBlastCanonicalLabel(label string, settings model.FamilyBlastSettings) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	if idx := strings.Index(label, "("); idx >= 0 {
		label = strings.TrimSpace(label[:idx])
	}
	fields := strings.Fields(label)
	if len(fields) > 0 {
		label = fields[0]
	}
	label = strings.Trim(label, " _-;:,()[]{}")
	if settings.NormalizeInnerPunctuation {
		label = normalizeFamilyPunctuation(label)
	}
	if settings.StripLeadingSpeciesPrefix {
		label = stripLeadingFamilySpeciesPrefix(label)
	}
	if settings.StripTerminalSubtypeSuffix {
		label = stripFamilyTerminalSubtypeSuffix(label)
	}
	label = strings.Trim(label, " ._-")
	return label
}

func normalizeFamilyPunctuation(label string) string {
	replacer := strings.NewReplacer("’", "'", ".", ".", "-", "-", "_", "_", "/", "-", ":", "-", " ", "")
	return replacer.Replace(label)
}

func stripLeadingFamilySpeciesPrefix(label string) string {
	if label == "" {
		return ""
	}
	for _, prefix := range []string{"sp", "le", "wo", "os", "at"} {
		if len(label) <= len(prefix)+1 {
			continue
		}
		if !strings.EqualFold(label[:len(prefix)], prefix) {
			continue
		}
		switch label[len(prefix)] {
		case '_', '-', '.', ':':
			rest := strings.TrimLeft(label[len(prefix)+1:], " _-.:")
			if rest != "" {
				return rest
			}
		}
	}
	return label
}

func stripFamilyTerminalSubtypeSuffix(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	lower := strings.ToLower(label)
	for _, suffix := range []string{"-like", "_like", ".like"} {
		if strings.HasSuffix(lower, suffix) && len(label) > len(suffix) {
			return strings.TrimSpace(label[:len(label)-len(suffix)])
		}
	}
	if idx := strings.LastIndexAny(label, "-_."); idx > 0 && idx < len(label)-1 {
		tail := label[idx+1:]
		hasLetter := false
		for _, r := range tail {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				hasLetter = true
			} else {
				hasLetter = false
				break
			}
		}
		if hasLetter && len(tail) <= 2 {
			return strings.TrimSpace(label[:idx])
		}
	}
	return label
}

func stripAfterFamilyMemberNumber(label string) string {
	label = strings.TrimSpace(label)
	for i, r := range label {
		if r < '0' || r > '9' {
			continue
		}
		j := i
		for j < len(label) {
			ch := label[j]
			if ch < '0' || ch > '9' {
				break
			}
			j++
		}
		if j < len(label) && isFamilyVariantSeparator(label[j]) {
			return strings.TrimSpace(label[:j])
		}
		return label
	}
	return label
}

func isFamilyVariantSeparator(ch byte) bool {
	return ch == '-' || ch == '_' || ch == '.' || ch == ':'
}

func stripFamilyTrailingIndex(label string) string {
	label = strings.TrimSpace(label)
	for len(label) > 0 {
		last := label[len(label)-1]
		if last >= '0' && last <= '9' {
			label = strings.TrimSpace(label[:len(label)-1])
			continue
		}
		break
	}
	label = strings.TrimRight(label, ".-_")
	return label
}

func applyFamilyBlastPlan(prepared []blastQueryItem, runs []blastQueryRun, plan *familyBlastPlan) ([]blastQueryItem, []blastQueryRun) {
	if plan == nil || !plan.Settings.Enabled || len(plan.Groups) == 0 {
		return prepared, runs
	}
	used := make(map[int]struct{}, len(runs))
	outItems := make([]blastQueryItem, 0, len(runs))
	outRuns := make([]blastQueryRun, 0, len(runs))
	for _, group := range plan.Groups {
		item, run, ok := buildFamilyBlastRun(group, prepared, runs, plan.Settings, len(outRuns)+1)
		if !ok {
			continue
		}
		outItems = append(outItems, item)
		outRuns = append(outRuns, run)
		for _, index := range group.Indexes {
			used[index] = struct{}{}
		}
	}
	for i, run := range runs {
		if _, ok := used[i]; ok {
			continue
		}
		run.Index = len(outRuns) + 1
		outItems = append(outItems, prepared[i])
		outRuns = append(outRuns, run)
	}
	return outItems, outRuns
}

func buildFamilyBlastRun(group familyBlastGroup, prepared []blastQueryItem, runs []blastQueryRun, settings model.FamilyBlastSettings, runIndex int) (blastQueryItem, blastQueryRun, bool) {
	if len(group.Indexes) == 0 {
		return blastQueryItem{}, blastQueryRun{}, false
	}
	memberLabels := make([]string, 0, len(group.Indexes))
	querySources := make([]*model.QuerySequenceSource, 0, len(group.Indexes))
	rows := make([]model.BlastResultRow, 0)
	sourceRuns := make([]blastQueryRun, 0, len(group.Indexes))
	for _, index := range group.Indexes {
		if index < 0 || index >= len(prepared) || index >= len(runs) {
			continue
		}
		member := prepared[index]
		memberLabel := familyBlastQueryLabel(member)
		memberLabels = append(memberLabels, memberLabel)
		if member.QuerySource != nil {
			querySources = append(querySources, member.QuerySource)
		}
		sourceRuns = append(sourceRuns, runs[index])
		for _, row := range runs[index].Results.Rows {
			if row.BlastLabelName == "" {
				row.BlastLabelName = memberLabel
			}
			if row.BlastGeneID == "" {
				row.BlastGeneID = blastQueryItemGeneID(member)
			}
			rows = append(rows, row)
		}
	}
	if len(sourceRuns) == 0 {
		return blastQueryItem{}, blastQueryRun{}, false
	}
	rowsBeforeMerge := len(rows)
	rows = prioritizeFamilyBlastRows(rows, settings)
	if settings.MergeRowsByTarget {
		rows = mergeFamilyBlastRowsByTarget(rows, settings)
	}
	aliasTexts := make([]string, 0, len(group.Indexes)*3)
	for _, index := range group.Indexes {
		item := prepared[index]
		aliasTexts = append(aliasTexts, strings.TrimSpace(item.LabelName))
		if item.QuerySource != nil {
			aliasTexts = append(aliasTexts, storedQuerySourceAliases(item.QuerySource)...)
		}
	}
	rows = annotateFamilyBlastConsensusRows(rows, group.Name, uniqueStrings(memberLabels), uniqueStrings(aliasTexts))
	item := blastQueryItem{
		RawInput:            strings.Join(memberLabels, "\n"),
		LabelName:           group.Name,
		FamilyName:          group.Name,
		MemberLabel:         strings.Join(uniqueStrings(memberLabels), "\n"),
		FamilyGroupSource:   strings.TrimSpace(group.GroupSource),
		FamilyDetectionRule: strings.TrimSpace(group.DetectionRule),
		QuerySource:         sourceRuns[0].Item.QuerySource,
		FamilySources:       querySources,
		FamilySettings:      settings,
	}
	result := sourceRuns[0].Results
	result.Rows = rows
	result.Message = strings.TrimSpace(result.Message)
	if result.Message != "" {
		result.Message += "\n"
	}
	result.Message += fmt.Sprintf("Family BLAST group %s merged %d query runs.", group.Name, len(sourceRuns))
	run := blastQueryRun{
		Index:           runIndex,
		Item:            item,
		Request:         sourceRuns[0].Request,
		Results:         result,
		RowsBeforeMerge: rowsBeforeMerge,
		RowsAfterMerge:  len(rows),
	}
	return item, run, true
}

func annotateFamilyBlastConsensusRows(rows []model.BlastResultRow, family string, memberLabels []string, aliasTexts []string) []model.BlastResultRow {
	if len(rows) == 0 {
		return rows
	}
	normalizedMembers := make([]string, 0, len(memberLabels))
	for _, label := range memberLabels {
		label = strings.TrimSpace(label)
		if label != "" {
			normalizedMembers = append(normalizedMembers, label)
		}
	}
	memberCount := len(uniqueStrings(normalizedMembers))
	semanticTokens := familySemanticTokensFromMembers(family, normalizedMembers, aliasTexts)
	allSemanticTokens := semanticTokens.All()
	semanticTokenText := strings.Join(semanticTokens.Core, "; ")
	semanticAliasText := strings.Join(semanticTokens.Aliases, "; ")
	totalSemanticTokens := len(allSemanticTokens)
	supportByTarget := map[string]map[string]struct{}{}
	bestLabelByTarget := map[string]string{}
	for _, row := range rows {
		target := familyBlastTargetKey(row)
		if target == "" {
			continue
		}
		if _, ok := supportByTarget[target]; !ok {
			supportByTarget[target] = map[string]struct{}{}
		}
		label := strings.TrimSpace(row.BlastLabelName)
		if label != "" {
			supportByTarget[target][label] = struct{}{}
			if bestLabelByTarget[target] == "" {
				bestLabelByTarget[target] = label
			}
		}
	}
	out := make([]model.BlastResultRow, len(rows))
	for i, row := range rows {
		row.FamilyName = family
		row.FamilyMemberLabels = strings.Join(uniqueStrings(normalizedMembers), "; ")
		row.FamilySemanticTokens = semanticTokenText
		row.FamilySemanticAliasTokens = semanticAliasText
		matches := familySemanticAnnotationAgreement(row, allSemanticTokens)
		row.FamilySemanticAnnotationMatchCount = len(matches)
		row.FamilySemanticAnnotationMatchTokens = strings.Join(matches, "; ")
		if totalSemanticTokens > 0 {
			row.FamilySemanticAgreementPercent = fmt.Sprintf("%.1f", float64(len(matches))/float64(totalSemanticTokens)*100)
		}
		target := familyBlastTargetKey(row)
		if target != "" {
			row.FamilyConsensusSupport = len(supportByTarget[target])
			if memberCount > 0 {
				row.FamilyConsensusSize = memberCount
				row.FamilyConsensusCoveragePercent = fmt.Sprintf("%.1f", float64(row.FamilyConsensusSupport)/float64(memberCount)*100)
			}
			row.FamilyConsensusPrimaryLabel = bestLabelByTarget[target]
		}
		out[i] = row
	}
	return out
}

type familySemanticTokenSet struct {
	Core    []string
	Aliases []string
}

func (set familySemanticTokenSet) All() []string {
	return uniqueStrings(append(append([]string(nil), set.Core...), set.Aliases...))
}

func familySemanticTokensFromMembers(family string, memberLabels []string, aliasTexts []string) familySemanticTokenSet {
	coreSeen := map[string]struct{}{}
	aliasSeen := map[string]struct{}{}
	core := make([]string, 0, 8)
	aliases := make([]string, 0, 16)
	addCore := func(value string) {
		value = normalizeFamilySemanticToken(value)
		if value == "" {
			return
		}
		if _, ok := coreSeen[value]; ok {
			return
		}
		coreSeen[value] = struct{}{}
		core = append(core, value)
	}
	addAlias := func(value string) {
		value = normalizeFamilySemanticToken(value)
		if value == "" {
			return
		}
		if _, ok := aliasSeen[value]; ok {
			return
		}
		aliasSeen[value] = struct{}{}
		aliases = append(aliases, value)
	}
	addCore(family)
	for _, label := range memberLabels {
		for _, token := range splitFamilySemanticTokens(label) {
			addAlias(token)
		}
	}
	for _, aliasText := range aliasTexts {
		for _, token := range splitFamilySemanticTokens(aliasText) {
			addAlias(token)
		}
	}
	for _, token := range foldFamilySemanticAliases(family) {
		addAlias(token)
	}
	return familySemanticTokenSet{Core: core, Aliases: aliases}
}

func familySemanticAnnotationAgreement(row model.BlastResultRow, allTokens []string) []string {
	if len(allTokens) == 0 {
		return nil
	}
	text := familySemanticAnnotationText(row)
	if text == "" {
		return nil
	}
	matches := make([]string, 0, 4)
	seen := map[string]struct{}{}
	for _, token := range allTokens {
		if token == "" {
			continue
		}
		if !strings.Contains(text, token) {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		matches = append(matches, token)
	}
	return matches
}

func familySemanticAnnotationText(row model.BlastResultRow) string {
	parts := []string{
		row.UniProtProteinName,
		row.UniProtEntryName,
		row.UniProtGeneNames,
		row.UniProtKeywords,
		row.UniProtFunction,
		row.UniProtCatalyticActivity,
		row.UniProtPathway,
		row.UniProtDomain,
		row.UniProtInterPro,
		row.PfamDomain,
		row.InterProEntryName,
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if normalized := normalizeFamilySemanticText(part); normalized != "" {
			out = append(out, normalized)
		}
	}
	return strings.Join(out, " ")
}

func normalizeFamilySemanticText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer("-", "", "_", "", "/", "", "\\", "", "'", "", "\"", "", "(", "", ")", "", "[", "", "]", "", "{", "", "}", "", ",", "", ";", "", ":", "", ".", "", " ", "")
	return replacer.Replace(value)
}

func normalizeFamilySemanticToken(value string) string {
	value = normalizeFamilySemanticText(value)
	if value == "" {
		return ""
	}
	if len(value) <= 1 {
		return ""
	}
	return value
}

func splitFamilySemanticTokens(label string) []string {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil
	}
	fields := familySemanticTokenPattern.FindAllString(label, -1)
	out := make([]string, 0, len(fields)+1)
	if canonical := normalizeFamilySemanticToken(label); canonical != "" {
		out = append(out, canonical)
	}
	for _, field := range fields {
		token := normalizeFamilySemanticToken(field)
		if token == "" {
			continue
		}
		out = append(out, token)
		token = strings.TrimRight(token, "0123456789")
		if token != "" {
			out = append(out, token)
		}
	}
	return uniqueStrings(out)
}

func foldFamilySemanticAliases(family string) []string {
	normalized := normalizeFamilySemanticToken(family)
	switch normalized {
	case "ccoamt", "ccoaomt":
		return []string{"ccoamt", "ccoaomt", "caffeoylcoao methyltransferase", "caffeoylcoaomethyltransferase"}
	case "comt", "omt":
		return []string{"comt", "omt", "caffeicacidomethyltransferase", "caffeateomethyltransferase", "ocaffeoylomt"}
	case "f5h", "fah":
		return []string{"f5h", "fah", "ferulate5hydroxylase", "ferulicacid5hydroxylase", "cyp84"}
	default:
		return nil
	}
}

func mergeFamilyBlastRowsByTarget(rows []model.BlastResultRow, settings model.FamilyBlastSettings) []model.BlastResultRow {
	if !settings.KeepBestHitPerTarget {
		return append([]model.BlastResultRow(nil), rows...)
	}
	indexByTarget := make(map[string]int, len(rows))
	out := make([]model.BlastResultRow, 0, len(rows))
	for _, row := range rows {
		key := familyBlastTargetKey(row)
		if key == "" {
			out = append(out, row)
			continue
		}
		if existing, ok := indexByTarget[key]; ok {
			out[existing] = betterFamilyBlastRow(out[existing], row, settings)
			continue
		}
		indexByTarget[key] = len(out)
		out = append(out, row)
	}
	return out
}

func prioritizeFamilyBlastRows(rows []model.BlastResultRow, settings model.FamilyBlastSettings) []model.BlastResultRow {
	type rankedFamilyBlastRow struct {
		row       model.BlastResultRow
		evidence  int
		coverage  float64
		targetKey string
	}
	ranked := make([]rankedFamilyBlastRow, len(rows))
	order := familyBlastRankingOrder(settings)
	for i := range rows {
		ranked[i] = rankedFamilyBlastRow{
			row:       rows[i],
			evidence:  familyBlastReferenceScore(rows[i], settings),
			coverage:  familyBlastCoverage(rows[i]),
			targetKey: familyBlastTargetKey(rows[i]),
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return familyBlastRowLessWithComputed(ranked[i].row, ranked[j].row, order, ranked[i].evidence, ranked[j].evidence, ranked[i].coverage, ranked[j].coverage, ranked[i].targetKey, ranked[j].targetKey)
	})
	out := make([]model.BlastResultRow, len(ranked))
	for i := range ranked {
		out[i] = ranked[i].row
	}
	return out
}

func familyBlastTargetKey(row model.BlastResultRow) string {
	for _, value := range []string{row.Protein, row.SubjectID, row.SequenceID, row.TranscriptID, row.GeneReportURL} {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			value = strings.TrimSuffix(value, "/")
			if slash := strings.LastIndex(value, "/"); slash >= 0 && slash < len(value)-1 {
				value = value[slash+1:]
			}
			for _, pattern := range familyTargetTranscriptSuffixPatterns {
				value = pattern.ReplaceAllString(value, "")
			}
			return value
		}
	}
	return ""
}

func betterFamilyBlastRow(left model.BlastResultRow, right model.BlastResultRow, settings model.FamilyBlastSettings) model.BlastResultRow {
	order := familyBlastRankingOrder(settings)
	leftEvidence := familyBlastReferenceScore(left, settings)
	rightEvidence := familyBlastReferenceScore(right, settings)
	leftCoverage := familyBlastCoverage(left)
	rightCoverage := familyBlastCoverage(right)
	leftTargetKey := familyBlastTargetKey(left)
	rightTargetKey := familyBlastTargetKey(right)
	if familyBlastRowLessWithComputed(right, left, order, rightEvidence, leftEvidence, rightCoverage, leftCoverage, rightTargetKey, leftTargetKey) {
		return right
	}
	return left
}

func familyBlastRowLess(left model.BlastResultRow, right model.BlastResultRow, settings model.FamilyBlastSettings) bool {
	return familyBlastRowLessWithComputed(
		left,
		right,
		familyBlastRankingOrder(settings),
		familyBlastReferenceScore(left, settings),
		familyBlastReferenceScore(right, settings),
		familyBlastCoverage(left),
		familyBlastCoverage(right),
		familyBlastTargetKey(left),
		familyBlastTargetKey(right),
	)
}

func familyBlastRowLessWithComputed(left model.BlastResultRow, right model.BlastResultRow, order []string, leftEvidence int, rightEvidence int, leftCoverage float64, rightCoverage float64, leftTargetKey string, rightTargetKey string) bool {
	for _, field := range order {
		switch field {
		case "reference":
			if leftEvidence != rightEvidence {
				return leftEvidence > rightEvidence
			}
		case "evalue":
			leftE := parseScientificFloatWorkflow(left.EValue, 1e300)
			rightE := parseScientificFloatWorkflow(right.EValue, 1e300)
			if leftE != rightE {
				return leftE < rightE
			}
		case "identity":
			if left.PercentIdentity != right.PercentIdentity {
				return left.PercentIdentity > right.PercentIdentity
			}
		case "coverage":
			if leftCoverage != rightCoverage {
				return leftCoverage > rightCoverage
			}
		case "targetlength":
			if left.TargetLength != right.TargetLength {
				return left.TargetLength > right.TargetLength
			}
		case "bitscore":
			if left.Bitscore != right.Bitscore {
				return left.Bitscore > right.Bitscore
			}
		}
	}
	return leftTargetKey < rightTargetKey
}

func familyBlastCoverage(row model.BlastResultRow) float64 {
	if row.AlignQueryLengthPercent > 0 {
		return row.AlignQueryLengthPercent
	}
	if row.AlignLength > 0 && row.QueryLength > 0 {
		return float64(row.AlignLength) / float64(row.QueryLength) * 100
	}
	return 0
}

func familyBlastRankingOrder(settings model.FamilyBlastSettings) []string {
	order := parseFamilyBlastRankingOrder(settings.RankingTieBreakerOrder)
	if len(order) == 0 {
		order = parseFamilyBlastRankingOrder("reference,evalue,identity,coverage,bitscore")
	}
	return order
}

func parseFamilyBlastRankingOrder(value string) []string {
	known := map[string]bool{
		"reference":    true,
		"evalue":       true,
		"identity":     true,
		"coverage":     true,
		"targetlength": true,
		"bitscore":     true,
	}
	seen := make(map[string]bool, len(known))
	out := make([]string, 0, len(known))
	for _, part := range strings.Split(value, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		part = strings.ReplaceAll(part, "-", "")
		part = strings.ReplaceAll(part, "_", "")
		switch part {
		case "ref", "referencescore", "externalevidence", "evidence":
			part = "reference"
		case "eval", "evaluecutoff":
			part = "evalue"
		case "querycoverage", "aligncoverage", "alignquerycoverage":
			part = "coverage"
		case "targetlen", "length", "targetlengthratio":
			part = "targetlength"
		case "bit", "bits":
			part = "bitscore"
		}
		if !known[part] || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}

func familyBlastReferenceScore(row model.BlastResultRow, settings model.FamilyBlastSettings) int {
	score := 0
	if settings.UseInterProReference {
		switch strings.ToLower(strings.TrimSpace(row.InterProConservedRegionStatus)) {
		case "present":
			score += 80
		case "partial":
			score += 40
		case "uncertain":
			score += 5
		case "missing":
			score -= 80
		}
		if coverage := parseScientificFloatWorkflow(row.InterProCoveragePercent, 0); coverage > 0 {
			score += int(coverage / 10)
		}
	}
	if settings.UseUniProtReference {
		if strings.TrimSpace(row.UniProtAccession) != "" {
			score += 20
		}
		if strings.EqualFold(strings.TrimSpace(row.UniProtReviewed), "reviewed") {
			score += 30
		}
		if isTruthyWorkflow(row.UniProtFragment) {
			score -= 30
		}
		if strings.TrimSpace(row.UniProtSequenceCaution) != "" {
			score -= 10
		}
		if ratio := parseScientificFloatWorkflow(row.TargetUniProtCanonicalLengthPercent, 0); ratio > 0 {
			distance := ratio - 100
			if distance < 0 {
				distance = -distance
			}
			switch {
			case distance <= 10:
				score += 25
			case distance <= 30:
				score += 10
			case distance >= 60:
				score -= 20
			}
		}
	}
	return score
}

func isTruthyWorkflow(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "y", "1", "fragment":
		return true
	default:
		return false
	}
}

func parseScientificFloatWorkflow(value string, fallback float64) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err == nil {
		return parsed
	}
	return fallback
}

func cloneKeywordSearchGroups(groups []model.KeywordSearchGroup) []model.KeywordSearchGroup {
	out := make([]model.KeywordSearchGroup, len(groups))
	for i, group := range groups {
		out[i] = group
		out[i].Rows = append([]model.KeywordResultRow(nil), group.Rows...)
	}
	return out
}

func (w *BlastWizard) collectBlastQueryItems() ([]blastQueryItem, error) {
	for {
		rawInput, err := w.prompt.SequenceInput()
		if err != nil {
			return nil, err
		}
		rawInput = strings.TrimSpace(rawInput)
		if rawInput == "" {
			return nil, nil
		}

		if loaded, ok, err := w.loadBlastInputFile(rawInput); err != nil {
			return nil, err
		} else if ok {
			rawInput = loaded
		}

		items, err := parseBlastQueryItems(rawInput)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, nil
		}

		return items, nil
	}
}

func allLabelsPresent(items []blastQueryItem) bool {
	for _, item := range items {
		if strings.TrimSpace(item.LabelName) == "" {
			return false
		}
	}
	return true
}

func (w *BlastWizard) collectBlastLabelsBeforeResolve(items []blastQueryItem) ([]blastQueryItem, bool, error) {
	if len(items) == 0 || allLabelsPresent(items) {
		return items, false, nil
	}
	labels, err := w.prompt.BlastLabelNames(len(items), true, prompt.ErrBackToQueryInput)
	if err != nil {
		if errors.Is(err, prompt.ErrAutoIdentifyRequested) {
			return items, true, nil
		}
		return nil, false, err
	}
	out := cloneBlastQueryItems(items)
	for i := range out {
		if i < len(labels) {
			setBlastQueryItemLabel(&out[i], labels[i])
		}
	}
	if !allLabelsPresent(out) {
		return nil, false, fmt.Errorf("label names are required for BLAST mode")
	}
	return out, false, nil
}

func (w *BlastWizard) collectBlastLabels(ctx context.Context, selected model.SpeciesCandidate, items []blastQueryItem) ([]blastQueryItem, error) {
	if len(items) == 0 {
		return items, nil
	}
	if allLabelsPresent(items) {
		return items, nil
	}
	labels, err := w.prompt.BlastLabelNames(len(items), true, prompt.ErrBackToQueryInput)
	if err != nil {
		if errors.Is(err, prompt.ErrAutoIdentifyRequested) {
			if blastItemsHaveReusableAliases(items) {
				out := cloneBlastQueryItems(items)
				for i := range out {
					if strings.TrimSpace(out[i].LabelName) == "" {
						if label := preferredStoredQuerySourceAlias(out[i].QuerySource); label != "" {
							out[i].LabelName = label
						}
					}
				}
				if allLabelsPresent(out) {
					return out, nil
				}
			}
			out, autoErr := w.autoIdentifyBlastLabelsWithProgress(ctx, selected, items)
			if autoErr != nil {
				return nil, autoErr
			}
			if !allLabelsPresent(out) {
				return nil, fmt.Errorf("could not auto identify label names for every BLAST query")
			}
			return out, nil
		}
		return nil, err
	}
	out := cloneBlastQueryItems(items)
	for i := range out {
		if i < len(labels) {
			setBlastQueryItemLabel(&out[i], labels[i])
		}
	}
	if !allLabelsPresent(out) {
		return nil, fmt.Errorf("label names are required for BLAST mode")
	}
	return out, nil
}

func (w *BlastWizard) prepareBlastExportItem(item blastQueryItem, batch bool) (blastQueryItem, error) {
	if strings.TrimSpace(item.LabelName) == "" {
		return blastQueryItem{}, fmt.Errorf("BLAST source label_name is required before export")
	}
	return item, nil
}

func (w *BlastWizard) autoIdentifyBlastLabelsWithProgress(ctx context.Context, selected model.SpeciesCandidate, items []blastQueryItem) ([]blastQueryItem, error) {
	if blastItemsHaveReusableAliases(items) {
		out := cloneBlastQueryItems(items)
		lockedLabels := blastAutoIdentifyLockedLabels(out)
		for i := range out {
			if strings.TrimSpace(out[i].LabelName) == "" {
				setBlastQueryItemLabel(&out[i], preferredStoredQuerySourceAlias(out[i].QuerySource))
			}
		}
		return harmonizeAutoIdentifiedBlastLabelsWithLocks(out, lockedLabels), nil
	}
	autoIndexes := blastItemsNeedingAutoLabel(items)
	if len(autoIndexes) == 0 {
		return items, nil
	}
	run := func(taskCtx context.Context, update func(string)) ([]blastQueryItem, error) {
		taskUpdate := safeTaskUpdate(update)
		labelCtx := mergeContexts(ctx, taskCtx)
		phytozomeSource := phytozome.NewClient(w.httpClient)
		out := cloneBlastQueryItems(items)
		taskTimestamp := time.Now().UTC().Format(time.RFC3339Nano)
		lockedLabels := blastAutoIdentifyLockedLabels(out)
		w.prefetchBlastLabelKeywordRows(labelCtx, phytozomeSource, selected, out, autoIndexes)
		workerCount := blastLabelWorkerCount(len(autoIndexes))
		type labelResult struct {
			index   int
			request labelname.AliasRankRequest
		}
		jobs := make(chan int)
		results := make(chan labelResult, len(out))
		var workers sync.WaitGroup
		for range workerCount {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for idx := range jobs {
					result := w.autoIdentifyBlastLabelResultForTask(labelCtx, phytozomeSource, selected, out[idx], taskTimestamp, idx)
					results <- labelResult{
						index: idx,
						request: labelname.AliasRankRequest{
							TaskTimestamp: result.TaskTimestamp,
							ItemIndex:     idx,
							Aliases:       result.Aliases,
						},
					}
				}
			}()
		}
		go func() {
			for _, i := range autoIndexes {
				select {
				case <-labelCtx.Done():
					close(jobs)
					return
				case jobs <- i:
				}
			}
			close(jobs)
		}()
		completed := 0
		requests := make([]labelname.AliasRankRequest, 0, len(autoIndexes))
		order := make([]int, 0, len(autoIndexes))
		for completed < len(autoIndexes) {
			select {
			case <-labelCtx.Done():
				workers.Wait()
				return nil, labelCtx.Err()
			case result := <-results:
				if result.index >= 0 && result.index < len(out) {
					requests = append(requests, result.request)
					order = append(order, result.index)
				}
				completed++
				taskUpdate(fmt.Sprintf("Collecting BLAST source label candidates... %d/%d", completed, len(autoIndexes)))
			}
		}
		workers.Wait()
		taskUpdate(fmt.Sprintf("Ranking BLAST source labels... %d items", len(requests)))
		ranked := labelname.RankAliasBatch(requests)
		for i, index := range order {
			aliases := ranked[i].RankedAliases
			if index >= 0 && index < len(out) {
				setBlastQueryItemLabel(&out[index], firstNonEmpty(out[index].LabelName, firstAliasOrEmpty(aliases)))
				mergeBlastQueryItemAliases(&out[index], aliases)
			}
		}
		out = harmonizeAutoIdentifiedBlastLabelsWithLocks(out, lockedLabels)
		return out, nil
	}
	if w.suppressTaskModals {
		return run(ctx, nil)
	}
	return tui.RunTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "Auto identify"),
		Title:       "Auto identifying BLAST label names",
		Description: "Reading Phytozome aliases for BLAST query labels.",
		Initial:     "Auto identifying BLAST label names...",
		CancelError: prompt.ErrBackToQueryInput,
	}, run)
}

func (w *BlastWizard) supplementBlastAliasesWithProgress(ctx context.Context, selected model.SpeciesCandidate, items []blastQueryItem) ([]blastQueryItem, error) {
	if len(items) == 0 {
		return items, nil
	}
	if blastItemsHaveReusableAliases(items) {
		return items, nil
	}
	aliasIndexes := blastItemsNeedingAliasSupplement(items)
	if len(aliasIndexes) == 0 {
		return items, nil
	}
	hasResolvable := false
	for _, idx := range aliasIndexes {
		if idx >= 0 && idx < len(items) && len(blastLabelSearchTerms(items[idx])) > 0 {
			hasResolvable = true
			break
		}
	}
	if !hasResolvable {
		return items, nil
	}
	run := func(taskCtx context.Context, update func(string)) ([]blastQueryItem, error) {
		return w.supplementBlastAliases(ctx, taskCtx, phytozome.NewClient(w.httpClient), selected, items, safeTaskUpdate(update))
	}
	if w.suppressTaskModals {
		return run(ctx, nil)
	}
	return tui.RunTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "Alias labels"),
		Title:       "Reading BLAST alias label names",
		Description: "Reading source-species aliases while preserving existing BLAST query label names.",
		Initial:     "Reading BLAST alias label names...",
		CancelError: prompt.ErrBackToQueryInput,
	}, run)
}

func (w *BlastWizard) supplementBlastAliases(ctx context.Context, taskCtx context.Context, phytozomeSource source.DataSource, selected model.SpeciesCandidate, items []blastQueryItem, update func(string)) ([]blastQueryItem, error) {
	labelCtx := mergeContexts(ctx, taskCtx)
	out := cloneBlastQueryItems(items)
	aliasIndexes := blastItemsNeedingAliasSupplement(out)
	if len(aliasIndexes) == 0 {
		return out, nil
	}
	taskTimestamp := time.Now().UTC().Format(time.RFC3339Nano)
	w.prefetchBlastLabelKeywordRows(labelCtx, phytozomeSource, selected, out, aliasIndexes)
	workerCount := blastLabelWorkerCount(len(aliasIndexes))
	type aliasResult struct {
		index   int
		request labelname.AliasRankRequest
	}
	jobs := make(chan int)
	results := make(chan aliasResult, len(out))
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for idx := range jobs {
				result := w.autoIdentifyBlastLabelResultForTask(labelCtx, phytozomeSource, selected, out[idx], taskTimestamp, idx)
				results <- aliasResult{
					index: idx,
					request: labelname.AliasRankRequest{
						TaskTimestamp: result.TaskTimestamp,
						ItemIndex:     idx,
						Aliases:       result.Aliases,
					},
				}
			}
		}()
	}
	go func() {
		for _, i := range aliasIndexes {
			select {
			case <-labelCtx.Done():
				close(jobs)
				return
			case jobs <- i:
			}
		}
		close(jobs)
	}()
	completed := 0
	requests := make([]labelname.AliasRankRequest, 0, len(aliasIndexes))
	order := make([]int, 0, len(aliasIndexes))
	for completed < len(aliasIndexes) {
		select {
		case <-labelCtx.Done():
			workers.Wait()
			return nil, labelCtx.Err()
		case result := <-results:
			if result.index >= 0 && result.index < len(out) {
				requests = append(requests, result.request)
				order = append(order, result.index)
			}
			completed++
			if update != nil {
				update(fmt.Sprintf("Collecting BLAST aliases... %d/%d", completed, len(aliasIndexes)))
			}
		}
	}
	workers.Wait()
	if update != nil {
		update(fmt.Sprintf("Ranking BLAST aliases... %d items", len(requests)))
	}
	ranked := labelname.RankAliasBatch(requests)
	for i, index := range order {
		if index >= 0 && index < len(out) {
			mergeBlastQueryItemAliases(&out[index], ranked[i].RankedAliases)
		}
	}
	return out, nil
}

type blastAutoLabelResult struct {
	Label         string
	Aliases       []string
	TaskTimestamp string
	ItemIndex     int
}

func harmonizeAutoIdentifiedBlastLabels(items []blastQueryItem) []blastQueryItem {
	return harmonizeAutoIdentifiedBlastLabelsWithLocks(items, nil)
}

func harmonizeAutoIdentifiedBlastLabelsWithLocks(items []blastQueryItem, lockedLabels []bool) []blastQueryItem {
	out := cloneBlastQueryItems(items)
	if len(out) <= 1 {
		return out
	}
	settings := model.DefaultFamilyBlastSettings()
	candidatesByIndex := make([][]string, len(out))
	familyCounts := map[string]int{}
	addFamily := func(label string) {
		if family := detectFamilyName(label, settings); family != "" {
			familyCounts[family]++
		}
	}
	for i, item := range out {
		candidates := blastAutoLabelCandidates(item)
		candidatesByIndex[i] = candidates
		for _, candidate := range candidates {
			addFamily(candidate)
		}
	}
	for i := range out {
		if i < len(lockedLabels) && lockedLabels[i] {
			continue
		}
		if strings.TrimSpace(out[i].LabelName) != "" {
			continue
		}
		best := ""
		bestScore := -1
		for _, candidate := range candidatesByIndex[i] {
			score := blastAutoLabelCoordinationScore(candidate, familyCounts, settings)
			if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(out[i].LabelName)) {
				score += 90
			}
			if score > bestScore || (score == bestScore && len(candidate) < len(best)) {
				best = candidate
				bestScore = score
			}
		}
		if strings.TrimSpace(best) != "" {
			out[i].LabelName = best
			if out[i].QuerySource != nil {
				out[i].QuerySource.LabelName = best
			}
		}
	}
	return out
}

func (w *BlastWizard) prefetchBlastLabelKeywordRows(ctx context.Context, phytozomeSource source.DataSource, selected model.SpeciesCandidate, items []blastQueryItem, indexes []int) {
	if w == nil || phytozomeSource == nil || len(indexes) == 0 {
		return
	}
	type labelPrefetchPlan struct {
		species []model.SpeciesCandidate
		terms   []string
	}
	plans := make([]labelPrefetchPlan, 0, len(indexes))
	for _, idx := range indexes {
		if idx < 0 || idx >= len(items) {
			continue
		}
		itemTerms := blastLabelSearchTerms(items[idx])
		if len(itemTerms) == 0 {
			continue
		}
		labelSpecies := w.phytozomeKeywordLabelSpeciesForItem(ctx, phytozomeSource, selected, items[idx])
		if len(labelSpecies) == 0 {
			continue
		}
		plans = append(plans, labelPrefetchPlan{species: labelSpecies, terms: itemTerms})
	}
	if len(plans) == 0 {
		return
	}
	speciesTerms := make(map[string][]string, len(plans))
	speciesByKey := make(map[string]model.SpeciesCandidate)
	for _, plan := range plans {
		for _, labelSpecies := range plan.species {
			key := w.keywordSpeciesKey(labelSpecies)
			speciesByKey[key] = labelSpecies
			speciesTerms[key] = append(speciesTerms[key], plan.terms...)
		}
	}
	for key, speciesTermsForKey := range speciesTerms {
		w.fetchKeywordRowsByTerms(ctx, phytozomeSource, speciesByKey[key], speciesTermsForKey)
	}
}

func (w *BlastWizard) keywordSpeciesKey(selected model.SpeciesCandidate) string {
	return strings.Join([]string{
		strconv.Itoa(selected.ProteomeID),
		strings.ToLower(strings.TrimSpace(selected.JBrowseName)),
		strings.ToLower(strings.TrimSpace(selected.GenomeLabel)),
	}, "|")
}

func blastAutoIdentifyLockedLabels(items []blastQueryItem) []bool {
	out := make([]bool, len(items))
	for i, item := range items {
		out[i] = strings.TrimSpace(item.LabelName) != ""
	}
	return out
}

func blastAutoLabelCandidates(item blastQueryItem) []string {
	candidates := storedQuerySourceAliases(item.QuerySource)
	if label := labelname.TrustedLabel(item.LabelName); label != "" {
		candidates = append(candidates, label)
	}
	return uniqueStrings(candidates)
}

func blastAutoLabelCoordinationScore(label string, familyCounts map[string]int, settings model.FamilyBlastSettings) int {
	label = strings.TrimSpace(label)
	if label == "" {
		return -1
	}
	score := labelname.AliasPreferenceScore(label) + labelname.QueryAliasPrimarySymbolBonus(label)
	if family := detectFamilyName(label, settings); family != "" {
		score += familyCounts[family] * 30
	}
	if looksLikeFamilyMemberStyleLabel(label) {
		score += 12
	}
	return score
}

func looksLikeFamilyMemberStyleLabel(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	hasLetter := false
	hasDigit := false
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == '-', r == '\'', r == '.':
		default:
			return false
		}
	}
	return hasLetter && hasDigit
}

func blastLabelFallbackSpecies(selected model.SpeciesCandidate, candidates []model.SpeciesCandidate) (model.SpeciesCandidate, bool) {
	if species, ok := matchPhytozomeSpeciesForLemna(selected, candidates); ok {
		return species, true
	}
	return model.SpeciesCandidate{}, false
}

func (w *BlastWizard) autoIdentifyBlastLabelFromPhytozome(ctx context.Context, phytozomeSource source.DataSource, species model.SpeciesCandidate, item blastQueryItem) string {
	return w.autoIdentifyBlastLabelResultFromPhytozome(ctx, phytozomeSource, species, item).Label
}

func (w *BlastWizard) autoIdentifyBlastLabelResultFromPhytozome(ctx context.Context, phytozomeSource source.DataSource, species model.SpeciesCandidate, item blastQueryItem) blastAutoLabelResult {
	if cached, ok := w.cachedBlastLabelLookup(phytozomeSource, species, item); ok {
		return cached
	}
	keywordRowsByTerm := w.fetchKeywordRowsByTerms(ctx, phytozomeSource, species, blastLabelSearchTerms(item))
	candidates, aliases := blastLabelCandidatesFromKeywordRows(item, keywordRowsByTerm)
	ranked := labelname.RankAliases(labelname.AliasRankRequest{
		TaskTimestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Aliases:       append(aliases, candidates...),
	})
	label := ""
	if len(ranked.RankedAliases) > 0 {
		label = ranked.RankedAliases[0]
	}
	result := blastAutoLabelResult{
		Label:         label,
		Aliases:       ranked.RankedAliases,
		TaskTimestamp: ranked.TaskTimestamp,
	}
	w.storeBlastLabelLookup(phytozomeSource, species, item, result)
	return result
}

func blastLabelCandidatesFromKeywordRows(item blastQueryItem, keywordRowsByTerm map[string][]model.KeywordResultRow) ([]string, []string) {
	candidates := make([]string, 0, 8)
	aliases := make([]string, 0, 16)
	for _, term := range blastLabelSearchTerms(item) {
		rows := keywordRowsByTerm[strings.ToLower(strings.TrimSpace(term))]
		if label := bestKeywordRowLabel(rows); label != "" {
			candidates = append(candidates, label)
		}
		aliases = append(aliases, keywordAliasesFromRows(rows)...)
	}
	return candidates, aliases
}

func (w *BlastWizard) autoIdentifyBlastLabel(ctx context.Context, phytozomeSource source.DataSource, selected model.SpeciesCandidate, item blastQueryItem) string {
	return w.autoIdentifyBlastLabelResult(ctx, phytozomeSource, selected, item).Label
}

func (w *BlastWizard) autoIdentifyBlastLabelResult(ctx context.Context, phytozomeSource source.DataSource, selected model.SpeciesCandidate, item blastQueryItem) blastAutoLabelResult {
	return w.autoIdentifyBlastLabelResultForTask(ctx, phytozomeSource, selected, item, time.Now().UTC().Format(time.RFC3339Nano), 0)
}

func (w *BlastWizard) autoIdentifyBlastLabelResultForTask(ctx context.Context, phytozomeSource source.DataSource, selected model.SpeciesCandidate, item blastQueryItem, taskTimestamp string, itemIndex int) blastAutoLabelResult {
	aliases := make([]string, 0, 16)
	pinnedLabel := strings.TrimSpace(item.LabelName)
	aliases = append(aliases, pinnedLabel)
	if pinnedLabel == "" && blastQueryItemSourceDatabase(item) == "lemna" {
		aliases = append(aliases, w.blastQueryPhytozomeAliasCandidates(ctx, phytozomeSource, selected, item)...)
		if len(uniqueStrings(aliases)) == 0 {
			aliases = append(aliases, collectBlastItemAliasCandidates(item)...)
		}
	} else {
		aliases = append(aliases, collectBlastItemAliasCandidates(item)...)
		for _, labelSpecies := range w.phytozomeKeywordLabelSpeciesForItem(ctx, phytozomeSource, selected, item) {
			result := w.autoIdentifyBlastLabelResultFromPhytozome(ctx, phytozomeSource, labelSpecies, item)
			aliases = append(aliases, result.Aliases...)
		}
	}
	aliases = uniqueStrings(aliases)
	if len(aliases) == 0 {
		aliases = append(aliases, fastaHeaderFallbackAliases(item)...)
	}
	request := labelname.AliasRankRequest{
		TaskTimestamp: taskTimestamp,
		ItemIndex:     itemIndex,
		Aliases:       aliases,
	}
	if pinnedLabel == "" && len(aliases) == 0 {
		fallback := blastLabelIdentityFallback(item)
		if item.QuerySource != nil {
			request.GeneID = item.QuerySource.GeneID
			request.TranscriptID = item.QuerySource.TranscriptID
			request.ProteinID = firstNonEmpty(fallback, item.QuerySource.ProteinID)
		} else {
			request.ProteinID = fallback
		}
	}
	ranked := labelname.RankAliases(request)
	label := ""
	if pinnedLabel != "" {
		label = pinnedLabel
		ranked.RankedAliases = uniqueStrings(append([]string{pinnedLabel}, ranked.RankedAliases...))
	} else if len(ranked.RankedAliases) > 0 {
		label = ranked.RankedAliases[0]
	}
	return blastAutoLabelResult{
		Label:         label,
		Aliases:       ranked.RankedAliases,
		TaskTimestamp: ranked.TaskTimestamp,
		ItemIndex:     ranked.ItemIndex,
	}
}

func collectBlastItemAliasCandidates(item blastQueryItem) []string {
	aliases := make([]string, 0, 12)
	if item.QuerySource != nil {
		aliases = append(aliases, querySourceLabelnameCandidates(item.QuerySource)...)
	}
	aliases = uniqueStrings(aliases)
	if len(aliases) == 0 && item.QuerySource != nil {
		aliases = append(aliases, labelname.SplitAliases(item.QuerySource.PhgoAliases)...)
	}
	return uniqueStrings(aliases)
}

func querySourceLabelnameCandidates(source *model.QuerySequenceSource) []string {
	if source == nil {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(source.SourceDatabase), "phytozome") {
		return phytozomeAliasCandidatesFromQuerySource(source)
	}
	if strings.EqualFold(strings.TrimSpace(source.SourceDatabase), "lemna") {
		return lemnaLocalQuerySourceAliasCandidates(source)
	}
	return lemnaLocalQuerySourceAliasCandidates(source)
}

func phytozomeAliasCandidatesFromQuerySource(source *model.QuerySequenceSource) []string {
	if source == nil {
		return nil
	}
	if candidates := labelname.SplitAliases(source.Synonyms); len(candidates) > 0 {
		return candidates
	}
	if candidates := labelname.SplitAliases(source.Symbols); len(candidates) > 0 {
		return candidates
	}
	return labelname.AutoDefineCandidates(source.AutoDefine)
}

func blastQueryItemSourceDatabase(item blastQueryItem) string {
	if item.QuerySource == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(item.QuerySource.SourceDatabase))
}

func (w *BlastWizard) blastQueryPhytozomeAliasCandidates(ctx context.Context, phytozomeSource source.DataSource, selected model.SpeciesCandidate, item blastQueryItem) []string {
	aliases := make([]string, 0, 16)
	for _, labelSpecies := range w.phytozomeKeywordLabelSpeciesForItem(ctx, phytozomeSource, selected, item) {
		result := w.autoIdentifyBlastLabelResultFromPhytozome(ctx, phytozomeSource, labelSpecies, item)
		aliases = append(aliases, result.Aliases...)
	}
	return uniqueStrings(aliases)
}

func fastaHeaderFallbackAliases(item blastQueryItem) []string {
	if item.QuerySource != nil {
		if label := strings.TrimSpace(item.QuerySource.LabelName); label != "" {
			return []string{label}
		}
	}
	if header, _ := splitFastaHeaderAndSequence(item.RawInput); header != "" {
		if parsed, ok := parsePhgoFastaHeader(header); ok && strings.TrimSpace(parsed.LabelName) != "" {
			return []string{parsed.LabelName}
		}
	}
	if false {
		if label := labelname.FastaHeaderLabelNameFromInput(item.RawInput); label != "" {
			return []string{label}
		}
	}
	return nil
}

func blastLabelSearchTerms(item blastQueryItem) []string {
	terms := make([]string, 0, 6)
	addTerm := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range terms {
			if strings.EqualFold(existing, value) {
				return
			}
		}
		terms = append(terms, value)
	}
	if item.QuerySource != nil {
		addTerm(item.QuerySource.ProteinID)
		addTerm(item.QuerySource.TranscriptID)
		addTerm(item.QuerySource.GeneID)
	}
	if header, _ := splitFastaHeaderAndSequence(item.RawInput); header != "" {
		if term := fastaHeaderKeywordSearchTerm(header); term != "" {
			addTerm(term)
		}
	}
	if _, _, identifier, err := parseGeneReportURL(strings.TrimSpace(item.RawInput)); err == nil {
		addTerm(identifier)
	}
	return terms
}

func fastaHeaderKeywordSearchTerm(header string) string {
	if parsed, ok := parsePhgoFastaHeader(header); ok {
		return strings.TrimSpace(parsed.GeneID)
	}
	return ""
}

func blastLabelIdentityFallback(item blastQueryItem) string {
	if item.QuerySource != nil {
		if label := firstNonEmpty(
			strings.TrimSpace(item.QuerySource.ProteinID),
			strings.TrimSpace(item.QuerySource.TranscriptID),
			strings.TrimSpace(item.QuerySource.GeneID),
		); label != "" {
			return label
		}
	}
	if header, _ := splitFastaHeaderAndSequence(item.RawInput); header != "" {
		if id := fastaHeaderPrimaryID(header); id != "" {
			return id
		}
	}
	return ""
}
func (w *BlastWizard) phytozomeKeywordLabelSpeciesForItem(ctx context.Context, phytozomeSource source.DataSource, selected model.SpeciesCandidate, item blastQueryItem) []model.SpeciesCandidate {
	out := make([]model.SpeciesCandidate, 0, 2)
	add := func(candidate model.SpeciesCandidate) {
		if candidate == (model.SpeciesCandidate{}) {
			return
		}
		for _, existing := range out {
			if existing.ProteomeID == candidate.ProteomeID && strings.EqualFold(existing.JBrowseName, candidate.JBrowseName) {
				return
			}
		}
		out = append(out, candidate)
	}
	if species, ok := w.phytozomeKeywordLabelSpeciesFromFastaHeader(ctx, phytozomeSource, item); ok {
		add(species)
		return out
	}
	if species, ok := w.phytozomeKeywordLabelSpeciesFromQuerySource(ctx, phytozomeSource, item); ok {
		add(species)
		return out
	}
	if species, ok := w.phytozomeKeywordLabelSpecies(ctx, selected); ok {
		add(species)
	}
	return out
}

func (w *BlastWizard) phytozomeKeywordLabelSpeciesFromFastaHeader(ctx context.Context, phytozomeSource source.DataSource, item blastQueryItem) (model.SpeciesCandidate, bool) {
	if item.QuerySource == nil || !strings.EqualFold(strings.TrimSpace(item.QuerySource.SourceDatabase), "fasta") {
		return model.SpeciesCandidate{}, false
	}
	organismText := strings.TrimSpace(item.QuerySource.OrganismShort + " " + item.QuerySource.Annotation)
	if organismText == "" {
		if header, _ := splitFastaHeaderAndSequence(item.RawInput); header != "" {
			organismText = header
		}
	}
	if organismText == "" {
		return model.SpeciesCandidate{}, false
	}
	candidates, err := w.speciesCandidatesForSource(ctx, phytozomeSource, nil)
	if err != nil {
		return model.SpeciesCandidate{}, false
	}
	return matchPhytozomeSpeciesForFastaHeader(organismText, candidates)
}

func (w *BlastWizard) phytozomeKeywordLabelSpeciesFromQuerySource(ctx context.Context, phytozomeSource source.DataSource, item blastQueryItem) (model.SpeciesCandidate, bool) {
	if item.QuerySource == nil {
		return model.SpeciesCandidate{}, false
	}
	candidates, err := w.speciesCandidatesForSource(ctx, phytozomeSource, nil)
	if err != nil {
		return model.SpeciesCandidate{}, false
	}
	if item.QuerySource.SourceJBrowseName != "" {
		if species, ok := findSpeciesCandidateByJBrowseName(candidates, item.QuerySource.SourceJBrowseName); ok {
			return species, true
		}
	}
	if item.QuerySource.SourceProteomeID > 0 {
		for _, candidate := range candidates {
			if candidate.ProteomeID == item.QuerySource.SourceProteomeID {
				return candidate, true
			}
		}
	}
	for _, value := range []string{item.QuerySource.SourceGenomeLabel, item.QuerySource.OrganismShort, item.QuerySource.Annotation} {
		if species, ok := matchPhytozomeSpeciesForFastaHeader(value, candidates); ok {
			return species, true
		}
	}
	return model.SpeciesCandidate{}, false
}

func (w *BlastWizard) phytozomeKeywordLabelSpecies(ctx context.Context, selected model.SpeciesCandidate) (model.SpeciesCandidate, bool) {
	if _, ok := w.source.(*lemna.Client); ok {
		phytozomeSource := phytozome.NewClient(w.httpClient)
		phytozomeCandidates, err := w.speciesCandidatesForSource(ctx, phytozomeSource, nil)
		if err != nil {
			return model.SpeciesCandidate{}, false
		}
		return blastLabelFallbackSpecies(selected, phytozomeCandidates)
	}
	return selected, true
}

func (w *BlastWizard) prepareExportSettings(defaultBaseName string, allowFolder bool, allowEmptyFileName bool, mentionBlastHeaderFallback bool) (exportSettings, error) {
	outputDir, err := appfs.OutputDir()
	if err != nil {
		return exportSettings{}, err
	}
	settings, err := w.prompt.ExportSettings("File name", allowFolder, allowEmptyFileName, mentionBlastHeaderFallback, prompt.ErrBackToRowSelection)
	if err != nil {
		return exportSettings{}, err
	}
	baseName := settings.BaseName
	if strings.TrimSpace(baseName) == "" {
		baseName = sanitizeExportName(defaultBaseName)
	}
	if baseName == "" {
		baseName = sanitizeExportName(time.Now().Format("20060102_150405"))
	}
	resolved := outputDir
	if allowFolder && strings.TrimSpace(settings.FolderName) != "" {
		resolved = filepath.Join(outputDir, sanitizeExportName(settings.FolderName))
		if err := os.MkdirAll(resolved, 0o755); err != nil {
			return exportSettings{}, fmt.Errorf("create output folder: %w", err)
		}
	}
	return exportSettingsFromPrompt(settings, baseName, resolved), nil
}

func (w *BlastWizard) prepareBatchExportSettings() (exportSettings, error) {
	outputDir, err := appfs.OutputDir()
	if err != nil {
		return exportSettings{}, err
	}
	settings, err := w.prompt.ExportSettings("Output folder", true, true, false, prompt.ErrBackToRowSelection)
	if err != nil {
		return exportSettings{}, err
	}
	resolved := outputDir
	if strings.TrimSpace(settings.FolderName) != "" {
		resolved = filepath.Join(outputDir, sanitizeExportName(settings.FolderName))
		if err := os.MkdirAll(resolved, 0o755); err != nil {
			return exportSettings{}, fmt.Errorf("create output folder: %w", err)
		}
	}
	return exportSettingsFromPrompt(settings, "", resolved), nil
}

func exportSettingsFromPrompt(settings prompt.ExportSettings, baseName string, outputDir string) exportSettings {
	return exportSettings{
		BaseName:      baseName,
		OutputDir:     outputDir,
		WriteReport:   settings.WriteReport,
		WriteText:     settings.WriteText,
		WriteExcel:    settings.WriteExcel,
		WriteRawExcel: settings.WriteRawExcel,
		UsePhgoHeader: settings.UsePhgoHeader,
	}
}

func (w *BlastWizard) loadBlastInputFile(rawInput string) (string, bool, error) {
	filename, ok := parseBlastLoadCommand(rawInput)
	if !ok {
		return "", false, nil
	}

	appDir, err := appfs.ApplicationDir()
	if err != nil {
		return "", false, err
	}
	path := filepath.Join(appDir, filename)
	data, err := withSpinnerValue(w.out, "Loading BLAST input file...", prompt.ErrBackToQueryInput, func(context.Context) ([]byte, error) {
		return os.ReadFile(path)
	})
	if err != nil {
		return "", false, fmt.Errorf("load BLAST input file %q: %w", filename, err)
	}
	if err := w.showInfo("BLAST input file", fmt.Sprintf("Loaded BLAST input from\n\n%s", path), prompt.ErrBackToQueryInput); err != nil {
		return "", false, err
	}
	return string(data), true, nil
}

func parseBlastLoadCommand(rawInput string) (string, bool) {
	value := strings.TrimSpace(rawInput)
	if len(value) < 5 || !strings.EqualFold(value[:4], "load") {
		return "", false
	}
	rest := strings.TrimSpace(value[4:])
	if rest == "" {
		return "", false
	}
	rest = strings.Trim(rest, "\"'")
	rest = filepath.Base(rest)
	if rest == "" || rest == "." || rest == ".." {
		return "", false
	}
	if !strings.HasSuffix(strings.ToLower(rest), ".txt") && !strings.HasSuffix(strings.ToLower(rest), ".fasta") && !strings.HasSuffix(strings.ToLower(rest), ".fa") {
		return "", false
	}
	return rest, true
}

func parseBlastQueryItems(rawInput string) ([]blastQueryItem, error) {
	text := strings.ReplaceAll(strings.TrimSpace(rawInput), "\r", "")
	if text == "" {
		return nil, nil
	}

	records := splitBlastInputRecords(text)
	if len(records) == 0 {
		return nil, nil
	}
	items := make([]blastQueryItem, 0, len(records))
	for _, record := range records {
		item, err := parseBlastQueryRecord(record)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func splitBlastInputRecords(text string) []string {
	value := strings.ReplaceAll(strings.TrimSpace(text), "\r", "")
	if value == "" {
		return nil
	}
	lines := strings.Split(value, "\n")
	records := make([]string, 0, 4)
	current := make([]string, 0, len(lines))
	currentKind := ""
	flush := func() {
		if len(current) == 0 {
			currentKind = ""
			return
		}
		record := strings.TrimSpace(strings.Join(current, "\n"))
		if record != "" {
			records = append(records, record)
		}
		current = current[:0]
		currentKind = ""
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if currentKind == "plain" || currentKind == "fasta" {
				flush()
			}
			continue
		}
		if tokens, ok := splitURLRecordTokens(line); ok {
			flush()
			records = append(records, tokens...)
			continue
		}
		if strings.HasPrefix(line, ">") {
			flush()
			currentKind = "fasta"
			current = append(current, line)
			continue
		}
		if currentKind == "fasta" {
			current = append(current, line)
			continue
		}
		if tokens, ok := splitInlineBlastRecordTokens(line); ok {
			flush()
			records = append(records, tokens...)
			continue
		}
		if currentKind == "" {
			currentKind = "plain"
		}
		current = append(current, line)
	}
	flush()
	return records
}

func splitURLRecordTokens(line string) ([]string, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return nil, false
	}
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		if _, ok := normalizeGeneReportURL(field); !ok {
			return nil, false
		}
		tokens = append(tokens, field)
	}
	return tokens, true
}

func splitInlineBlastRecordTokens(line string) ([]string, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) <= 1 {
		return nil, false
	}
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		if _, ok := normalizeGeneReportURL(field); ok {
			tokens = append(tokens, field)
			continue
		}
		if isLikelyInlineSequenceToken(field) {
			tokens = append(tokens, field)
			continue
		}
		return nil, false
	}
	return tokens, true
}

func isLikelyInlineSequenceToken(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	hasLetter := false
	for _, ch := range value {
		switch {
		case ch >= 'A' && ch <= 'Z', ch >= 'a' && ch <= 'z':
			hasLetter = true
		case ch == '*':
		default:
			return false
		}
	}
	return hasLetter
}

func parseBlastQueryRecord(record string) (blastQueryItem, error) {
	record = strings.TrimSpace(record)
	if record == "" {
		return blastQueryItem{}, nil
	}
	if strings.HasPrefix(record, ">") {
		source, ok := parseFastaQuerySequenceInput(record)
		if !ok {
			return blastQueryItem{}, fmt.Errorf("invalid FASTA BLAST input near %q", oneLinePreview(record))
		}
		return blastQueryItemFromFastaSource(record, source), nil
	}
	return blastQueryItem{RawInput: record}, nil
}

func blastQueryItemFromFastaSource(rawInput string, source *model.QuerySequenceSource) blastQueryItem {
	label := ""
	if source != nil {
		label = strings.TrimSpace(source.LabelName)
	}
	item := blastQueryItem{
		RawInput:    strings.TrimSpace(rawInput),
		LabelName:   label,
		QuerySource: source,
	}
	if source != nil {
		item.Sequence = source.Sequence
	}
	return item
}

func allLabelsBlank(items []blastQueryItem) bool {
	for _, item := range items {
		if strings.TrimSpace(item.LabelName) != "" {
			return false
		}
	}
	return true
}

func buildBlastOutputDisplayName(item blastQueryItem) string {
	if family := strings.TrimSpace(item.FamilyName); family != "" {
		return family
	}
	label := strings.TrimSpace(item.LabelName)
	if label == "" && item.QuerySource != nil {
		label = firstNonEmpty(item.QuerySource.GeneID, item.QuerySource.TranscriptID, item.QuerySource.ProteinID)
	}
	if label == "" {
		label = "query"
	}
	return label
}

func blastTXTHeaderLabel(item blastQueryItem, fileBaseName string) string {
	if label := strings.TrimSpace(item.LabelName); label != "" {
		return label
	}
	return strings.TrimSpace(fileBaseName)
}

func exportItemFamilySources(item blastQueryItem) []*model.QuerySequenceSource {
	if len(item.FamilySources) > 0 {
		return append([]*model.QuerySequenceSource(nil), item.FamilySources...)
	}
	if item.QuerySource != nil {
		return []*model.QuerySequenceSource{item.QuerySource}
	}
	return nil
}

func sanitizeExportName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "query"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	value = replacer.Replace(value)
	value = strings.Join(strings.Fields(value), "_")
	value = strings.Trim(value, " ._")
	if value == "" {
		return "query"
	}
	return value
}

func reportQueryLabel(item blastQueryItem) string {
	if family := strings.TrimSpace(item.FamilyName); family != "" {
		return family
	}
	label := strings.TrimSpace(item.LabelName)
	if label != "" {
		return label
	}
	if item.QuerySource != nil {
		return firstNonEmpty(item.QuerySource.GeneID, item.QuerySource.TranscriptID, item.QuerySource.ProteinID, "query")
	}
	return "query"
}

func blastExecutionLabel(program string) string {
	if strings.HasPrefix(strings.ToLower(program), "local:") {
		return "local"
	}
	return "server"
}

func (w *BlastWizard) resolveBlastQueryItems(ctx context.Context, items []blastQueryItem, candidates []model.SpeciesCandidate) ([]blastQueryItem, error) {
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "Resolving input"),
		Title:       "Resolving BLAST query inputs",
		Description: "Resolving URLs, FASTA headers, and sequence metadata before submission.",
		Initial:     "Resolving BLAST query inputs...",
		Total:       len(items),
		CancelError: prompt.ErrBackToQueryInput,
	}, func(taskCtx context.Context, update func(int, string)) ([]blastQueryItem, error) {
		return w.resolveBlastQueryItemsWithProgress(mergeContexts(ctx, taskCtx), items, candidates, update)
	})
}

func (w *BlastWizard) resolveBlastQueryItemsWithProgress(ctx context.Context, items []blastQueryItem, candidates []model.SpeciesCandidate, update func(int, string)) ([]blastQueryItem, error) {
	progress := safeProgress(update)
	type queryResolveResult struct {
		index       int
		querySource *model.QuerySequenceSource
		ok          bool
		err         error
	}

	prepared := make([]blastQueryItem, 0, len(items))
	progress(0, "Resolving BLAST query inputs...")

	results := make([]queryResolveResult, len(items))
	jobs := make(chan int)
	outcomes := make(chan queryResolveResult, len(items))
	workerCount := maxInt(parallelismFor(len(items), maxParallelQueryJobs), networkParallelismFor(len(items)))

	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for idx := range jobs {
				if items[idx].QuerySource != nil && strings.TrimSpace(items[idx].QuerySource.Sequence) != "" {
					outcomes <- queryResolveResult{index: idx, querySource: items[idx].QuerySource, ok: true}
					continue
				}
				if strings.TrimSpace(items[idx].Sequence) != "" {
					outcomes <- queryResolveResult{index: idx, querySource: &model.QuerySequenceSource{
						Sequence:       items[idx].Sequence,
						LabelName:      strings.TrimSpace(items[idx].LabelName),
						SourceDatabase: w.source.Name(),
					}, ok: true}
					continue
				}
				if resolver, ok := w.source.(source.QueryResolver); ok {
					selected := items[idx].QuerySource
					species := model.SpeciesCandidate{}
					if selected != nil && selected.SourceProteomeID != 0 {
						species = model.SpeciesCandidate{
							ProteomeID:  selected.SourceProteomeID,
							JBrowseName: selected.SourceJBrowseName,
							GenomeLabel: selected.SourceGenomeLabel,
						}
					}
					if species.ProteomeID == 0 {
						for _, candidate := range candidates {
							if candidate.ProteomeID != 0 {
								species = candidate
								break
							}
						}
					}
					if species.ProteomeID != 0 {
						if fastSource, resolved, err := w.tryResolveSourceQueryInput(ctx, resolver, species, items[idx].RawInput); err != nil {
							outcomes <- queryResolveResult{index: idx, err: err}
							continue
						} else if fastSource {
							outcomes <- queryResolveResult{index: idx, querySource: resolved, ok: true}
							continue
						}
					}
				}
				querySource, ok, err := w.resolveQuerySequenceInputBatchWithTimeout(ctx, candidates, items[idx].RawInput)
				outcomes <- queryResolveResult{index: idx, querySource: querySource, ok: ok, err: err}
			}
		}()
	}

	go func() {
		for i := range items {
			select {
			case <-ctx.Done():
				close(jobs)
				workers.Wait()
				close(outcomes)
				return
			case jobs <- i:
			}
		}
		close(jobs)
		workers.Wait()
		close(outcomes)
	}()

	doneCount := 0
	for {
		select {
		case <-ctx.Done():
			return prepared, ctx.Err()
		case result, ok := <-outcomes:
			if !ok {
				goto queryResolveDone
			}
			results[result.index] = result
			doneCount++
			progress(doneCount, fmt.Sprintf("Resolving BLAST query inputs... %d/%d", doneCount, len(items)))
		}
	}

queryResolveDone:
	failures := make([]blastBatchResolveFailure, 0)
	for i, item := range items {
		querySource := results[i].querySource
		ok := results[i].ok
		err := results[i].err
		if err != nil {
			failures = append(failures, blastBatchResolveFailure{
				Index: i + 1,
				Total: len(items),
				Label: oneLinePreview(reportQueryLabel(item)),
				Err:   err,
			})
			continue
		}
		sequence := item.RawInput
		if ok {
			sequence = querySource.Sequence
		}
		if strings.TrimSpace(sanitizeSequence(sequence)) == "" {
			progress(doneCount, fmt.Sprintf("Skipped BLAST query %d/%d because no usable sequence could be resolved.", i+1, len(items)))
			continue
		}
		if querySource == nil {
			querySource = &model.QuerySequenceSource{
				Sequence:       sequence,
				SourceDatabase: w.source.Name(),
			}
		}
		syncBlastQueryItemSourceLabel(&item, querySource)
		item.Sequence = normalizeBlastSequence(sequence, detectSequenceKind(sequence))
		switch detectSequenceKind(item.Sequence) {
		case model.SequenceDNA:
			item.NucleotideSequence = item.Sequence
		case model.SequenceProtein:
			item.ProteinSequence = item.Sequence
		}
		if querySource != nil {
			if querySource.PreferredSequenceID == "" {
				querySource.PreferredSequenceID = firstNonEmpty(
					strings.TrimSpace(querySource.ProteinID),
					strings.TrimSpace(querySource.TranscriptID),
					strings.TrimSpace(querySource.GeneID),
				)
			}
			querySource.Sequence = item.Sequence
			querySource.SequenceKind = detectSequenceKind(item.Sequence)
			switch querySource.SequenceKind {
			case model.SequenceDNA:
				if querySource.NucleotideSequence == "" {
					querySource.NucleotideSequence = item.Sequence
				}
			case model.SequenceProtein:
				if querySource.ProteinSequence == "" {
					querySource.ProteinSequence = item.Sequence
				}
			}
		}
		item.QuerySource = querySource
		prepared = append(prepared, item)
	}
	progress(len(items), "Resolved BLAST query inputs.")
	if len(failures) > 0 {
		return prepared, &blastBatchResolveError{
			Total:    len(items),
			Prepared: cloneBlastQueryItems(prepared),
			Failures: failures,
		}
	}
	return prepared, nil
}

func syncBlastQueryItemSourceLabel(item *blastQueryItem, source *model.QuerySequenceSource) {
	if item == nil || source == nil {
		return
	}
	itemLabel := strings.TrimSpace(item.LabelName)
	sourceLabel := strings.TrimSpace(source.LabelName)
	switch {
	case sourceLabel == "" && itemLabel != "":
		source.LabelName = itemLabel
	case itemLabel == "" && sourceLabel != "":
		item.LabelName = sourceLabel
	}
}

func blastQuerySequenceIdentifier(item blastQueryItem, kind model.SequenceKind) string {
	if item.QuerySource == nil {
		return ""
	}
	preferred := strings.TrimSpace(item.QuerySource.PreferredSequenceID)
	if kind == model.SequenceDNA {
		return firstNonEmpty(
			strings.TrimSpace(item.QuerySource.TranscriptID),
			strings.TrimSpace(item.QuerySource.GeneID),
			strings.TrimSpace(item.QuerySource.ProteinID),
			preferred,
		)
	}
	return firstNonEmpty(
		strings.TrimSpace(item.QuerySource.ProteinID),
		strings.TrimSpace(item.QuerySource.TranscriptID),
		strings.TrimSpace(item.QuerySource.GeneID),
		preferred,
	)
}

func blastQueryNeedsSequenceKind(item blastQueryItem, kind model.SequenceKind) bool {
	sequence := blastQuerySequenceForKind(item, kind)
	if sequence == "" {
		sequence = strings.TrimSpace(item.Sequence)
		if item.QuerySource != nil && strings.TrimSpace(item.QuerySource.Sequence) != "" {
			sequence = strings.TrimSpace(item.QuerySource.Sequence)
		}
	}
	if sequence == "" {
		return true
	}
	return detectSequenceKind(sequence) != kind
}

func blastQuerySequenceForKind(item blastQueryItem, kind model.SequenceKind) string {
	switch kind {
	case model.SequenceDNA:
		if seq := strings.TrimSpace(item.NucleotideSequence); seq != "" {
			return seq
		}
		if item.QuerySource != nil && strings.TrimSpace(item.QuerySource.NucleotideSequence) != "" {
			return strings.TrimSpace(item.QuerySource.NucleotideSequence)
		}
	case model.SequenceProtein:
		if seq := strings.TrimSpace(item.ProteinSequence); seq != "" {
			return seq
		}
		if item.QuerySource != nil && strings.TrimSpace(item.QuerySource.ProteinSequence) != "" {
			return strings.TrimSpace(item.QuerySource.ProteinSequence)
		}
	}
	return ""
}

func storeBlastQuerySequenceForKind(item *blastQueryItem, kind model.SequenceKind, sequence string) {
	if item == nil {
		return
	}
	normalized := normalizeBlastSequence(sequence, kind)
	if normalized == "" {
		return
	}
	item.Sequence = normalized
	switch kind {
	case model.SequenceDNA:
		item.NucleotideSequence = normalized
	case model.SequenceProtein:
		item.ProteinSequence = normalized
	}
	if item.QuerySource != nil {
		item.QuerySource.Sequence = normalized
		item.QuerySource.SequenceKind = kind
		switch kind {
		case model.SequenceDNA:
			item.QuerySource.NucleotideSequence = normalized
		case model.SequenceProtein:
			item.QuerySource.ProteinSequence = normalized
		}
	}
}

func (w *BlastWizard) alignPreparedBlastItemsToRequest(ctx context.Context, prepared []blastQueryItem, request model.BlastRequest) ([]blastQueryItem, error) {
	if len(prepared) == 0 {
		return prepared, nil
	}
	targetKind := request.SequenceKind
	program := normalizeWorkflowBlastProgram(request.Program)
	out := cloneBlastQueryItems(prepared)
	type sequenceTask struct {
		key        string
		indexes    []int
		targetID   int
		sequenceID string
	}
	taskByKey := make(map[string]*sequenceTask)
	for i := range out {
		if !blastQueryNeedsSequenceKind(out[i], targetKind) {
			normalized := blastQuerySequenceForKind(out[i], targetKind)
			if normalized == "" {
				normalized = out[i].Sequence
			}
			storeBlastQuerySequenceForKind(&out[i], targetKind, normalized)
			continue
		}
		targetID := 0
		if out[i].QuerySource != nil {
			targetID = out[i].QuerySource.SourceProteomeID
		}
		sequenceID := blastQuerySequenceIdentifier(out[i], targetKind)
		if targetID == 0 || sequenceID == "" {
			return nil, fmt.Errorf("BLAST query %q cannot be converted into a %s sequence for %s", reportQueryLabel(out[i]), targetKind, strings.ToUpper(program))
		}
		key := strings.Join([]string{string(targetKind), program, strconv.Itoa(targetID), strings.ToLower(strings.TrimSpace(sequenceID))}, "|")
		task := taskByKey[key]
		if task == nil {
			task = &sequenceTask{
				key:        key,
				targetID:   targetID,
				sequenceID: sequenceID,
			}
			taskByKey[key] = task
		}
		task.indexes = append(task.indexes, i)
	}
	if len(taskByKey) == 0 {
		return out, nil
	}
	tasks := make([]sequenceTask, 0, len(taskByKey))
	for _, task := range taskByKey {
		tasks = append(tasks, *task)
	}
	results := make([]model.ProteinSequenceData, len(tasks))
	if err := runParallel(ctx, len(tasks), blastSequenceFetchWorkerCount(len(tasks)), func(fetchCtx context.Context, index int) error {
		task := tasks[index]
		var (
			data model.ProteinSequenceData
			err  error
		)
		switch targetKind {
		case model.SequenceDNA:
			resolver, ok := w.source.(nucleotideSequenceResolver)
			if !ok {
				return fmt.Errorf("source %q does not support DNA BLAST query resolution", w.source.Name())
			}
			data, err = resolver.FetchNucleotideSequence(fetchCtx, task.targetID, task.sequenceID, program)
		default:
			data, err = w.source.FetchProteinSequence(fetchCtx, task.targetID, task.sequenceID)
		}
		if err != nil {
			return err
		}
		results[index] = data
		return nil
	}); err != nil {
		return nil, err
	}
	for taskIndex, task := range tasks {
		normalized := normalizeBlastSequence(results[taskIndex].Sequence, targetKind)
		if normalized == "" {
			return nil, fmt.Errorf("resolved empty %s sequence for %s", targetKind, task.sequenceID)
		}
		for _, itemIndex := range task.indexes {
			storeBlastQuerySequenceForKind(&out[itemIndex], targetKind, normalized)
		}
	}
	return out, nil
}

func (w *BlastWizard) resolveQuerySequenceInputBatchWithTimeout(ctx context.Context, candidates []model.SpeciesCandidate, input string) (*model.QuerySequenceSource, bool, error) {
	return w.resolveQuerySequenceInputBatch(ctx, candidates, input)
}

func (w *BlastWizard) tryResolveSourceQueryInput(ctx context.Context, resolver source.QueryResolver, species model.SpeciesCandidate, input string) (bool, *model.QuerySequenceSource, error) {
	if resolver == nil {
		return false, nil, nil
	}
	resolved, ok, err := resolver.ResolveQuerySequence(ctx, species, input)
	if err != nil || !ok || resolved == nil {
		return ok, resolved, err
	}
	if strings.TrimSpace(resolved.SourceDatabase) == "" {
		resolved.SourceDatabase = w.source.Name()
	}
	if resolved.SourceProteomeID == 0 {
		resolved.SourceProteomeID = species.ProteomeID
	}
	if strings.TrimSpace(resolved.SourceJBrowseName) == "" {
		resolved.SourceJBrowseName = species.JBrowseName
	}
	if strings.TrimSpace(resolved.SourceGenomeLabel) == "" {
		resolved.SourceGenomeLabel = species.GenomeLabel
	}
	return true, resolved, nil
}

func oneLinePreview(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if len(value) > 120 {
		return value[:117] + "..."
	}
	return value
}

func parallelismFor(total int, maxWorkers int) int {
	return clampWorkers(total, maxWorkers)
}

func networkParallelismFor(total int) int {
	return clampWorkers(total, defaultNetworkWorkers())
}

func diskParallelismFor(total int) int {
	return clampWorkers(total, defaultDiskWorkers())
}

func scaledAuxWorkerCount(total int, envName string, softCap int) int {
	if total <= 0 {
		return 0
	}
	if configured := configuredInt(envName, 0); configured > 0 {
		return boundedWorkerCount(total, configured)
	}
	cpu := currentCPUCount()
	limit := maxInt(4, minInt(softCap, cpu*2))
	return boundedWorkerCount(total, limit)
}

func scaledNetworkAuxWorkerCount(total int, envName string, softCap int) int {
	if total <= 0 {
		return 0
	}
	if configured := configuredInt(envName, 0); configured > 0 {
		return boundedWorkerCount(total, configured)
	}
	limit := maxInt(4, softCap)
	if envLimit := configuredInt("PHYTOZOME_GO_MAX_WORKERS", 0); envLimit > limit {
		limit = envLimit
	}
	return boundedWorkerCount(total, limit)
}

func tunedBlastNetworkWorkerLimit(total int, config externalReferenceConfig, base int, medium int, high int) int {
	load := blastReferenceLoadFactor(config)
	switch {
	case total <= 8:
		return maxInt(4, base+load)
	case total <= 32:
		return maxInt(6, medium+load)
	default:
		return maxInt(8, high+load*2)
	}
}

func blastReferenceLoadFactor(config externalReferenceConfig) int {
	load := 0
	if config.AutoLabelBlastHits {
		load++
	}
	if config.UseUniProt {
		load++
	}
	if config.UseInterPro {
		load++
	}
	return load
}

func blastUniProtWorkerCount(total int) int {
	return blastUniProtWorkerCountForConfig(total, externalReferenceConfig{})
}

func blastUniProtWorkerCountForConfig(total int, config externalReferenceConfig) int {
	softCap := tunedBlastNetworkWorkerLimit(total, config, 6, 9, 12)
	return scaledNetworkAuxWorkerCount(total, "PHYTOZOME_GO_BLAST_UNIPROT_WORKERS", softCap)
}

func blastUniProtAccessionWorkerCount(total int) int {
	return blastUniProtAccessionWorkerCountForConfig(total, externalReferenceConfig{})
}

func blastUniProtAccessionWorkerCountForConfig(total int, config externalReferenceConfig) int {
	softCap := tunedBlastNetworkWorkerLimit(total, config, 8, 13, 16)
	return scaledNetworkAuxWorkerCount(total, "PHYTOZOME_GO_BLAST_UNIPROT_ACCESSION_WORKERS", softCap)
}

func blastInterProWorkerCount(total int) int {
	return blastInterProWorkerCountForConfig(total, externalReferenceConfig{})
}

func blastInterProWorkerCountForConfig(total int, config externalReferenceConfig) int {
	softCap := tunedBlastNetworkWorkerLimit(total, config, 6, 10, 12)
	return scaledNetworkAuxWorkerCount(total, "PHYTOZOME_GO_BLAST_INTERPRO_WORKERS", softCap)
}

func blastLabelWorkerCount(total int) int {
	return scaledAuxWorkerCount(total, "PHYTOZOME_GO_BLAST_LABEL_WORKERS", 24)
}

func blastKeywordTermWorkerCount(total int) int {
	return scaledAuxWorkerCount(total, "PHYTOZOME_GO_BLAST_KEYWORD_TERM_WORKERS", 24)
}

func blastSequenceFetchWorkerCount(total int) int {
	return scaledAuxWorkerCount(total, "PHYTOZOME_GO_BLAST_SEQUENCE_FETCH_WORKERS", 20)
}

func batchBlastWorkerCount(total int, request model.BlastRequest) int {
	if isLocalBlastRequest(request) {
		if configured := configuredInt("PHYTOZOME_GO_LOCAL_BLAST_BATCH_WORKERS", 0); configured > 0 {
			return boundedWorkerCount(total, configured)
		}
		return defaultLocalBlastWorkerCount(total, request)
	} else {
		if configured := configuredInt("PHYTOZOME_GO_REMOTE_BLAST_BATCH_WORKERS", 0); configured > 0 {
			return clampWorkers(total, configured)
		}
		return boundedWorkerCount(total, 2)
	}
}

func boundedWorkerCount(total int, limit int) int {
	if total <= 0 {
		return 0
	}
	if limit <= 0 {
		limit = 1
	}
	if total < limit {
		return total
	}
	return limit
}

func keywordSearchWorkerCount(total int) int {
	return maxInt(parallelismFor(total, maxParallelKeywordJobs), networkParallelismFor(total))
}

func keywordSearchResultCompleted(result keywordSearchResult) bool {
	return result.err == nil && !result.ended.IsZero()
}

func countCompletedKeywordResults(results []keywordSearchResult) int {
	total := 0
	for _, result := range results {
		if keywordSearchResultCompleted(result) {
			total++
		}
	}
	return total
}

func defaultLocalBlastWorkerCount(total int, request model.BlastRequest) int {
	if total <= 0 {
		return 0
	}
	cpu := currentCPUCount()
	program := normalizeWorkflowBlastProgram(request.Program)
	limit := 1
	switch {
	case program == "blastx":
		limit = 1
	case program == "blastn" || program == "tblastn":
		if cpu >= 8 && total >= 2 {
			limit = 2
		}
	case program == "blastp":
		switch {
		case cpu >= 32 && total >= 6:
			limit = 3
		case cpu >= 12 && total >= 2:
			limit = 2
		}
	default:
		switch {
		case cpu >= 24:
			limit = 3
		case cpu >= 12:
			limit = 2
		}
	}
	return boundedWorkerCount(total, limit)
}

func localBlastThreadsPerWorker(workerCount int, request model.BlastRequest) int {
	if configured := configuredInt("PHYTOZOME_GO_LOCAL_BLAST_THREADS", 0); configured > 0 {
		return configured
	}
	cpu := currentCPUCount()
	if workerCount < 1 {
		workerCount = 1
	}
	threads := cpu / workerCount
	if threads < 1 {
		return 1
	}
	maxThreads := localBlastProgramThreadCap(normalizeWorkflowBlastProgram(request.Program), workerCount)
	if maxThreads > 0 && threads > maxThreads {
		threads = maxThreads
	}
	if threads < 1 {
		return 1
	}
	return threads
}

func localBlastProgramThreadCap(program string, workerCount int) int {
	switch normalizeWorkflowBlastProgram(program) {
	case "blastn", "tblastn":
		if workerCount >= 2 {
			return 2
		}
		return 4
	case "blastp", "blastx":
		return 8
	default:
		return 8
	}
}

func (w *BlastWizard) exportBlastSelectionsToDir(ctx context.Context, selectedRows []model.BlastResultRow, allRows []model.BlastResultRow, rowNumbers []int, filterFlags []bool, querySource *model.QuerySequenceSource, displayName string, txtHeaderLabel string, fileBaseName string, outputDir string, settings exportSettings, showComplete bool) (exportFileResult, error) {
	files := exportFileResult{SequenceAudit: report.SequenceAudit{Requested: settings.WriteText}}
	var prefetchedTextRecords []model.ProteinSequenceRecord
	textRecordsReady := false
	if settings.WriteExcel {
		excelPath := filepath.Join(outputDir, fileBaseName+".xlsx")
		exportMetadata := buildExportMetadata(displayName, querySource)
		stepStart := time.Now()
		if settings.WriteText {
			records, err := w.exportBlastExcelAndFetchRecords(ctx, selectedRows, rowNumbers, filterFlags, excelPath, exportMetadata)
			if err != nil {
				files.Steps = append(files.Steps, keywordReportStep("Write selected BLAST Excel and fetch peptide sequences", stepStart, time.Now(), "failed", err.Error()))
				return exportFileResult{}, err
			}
			prefetchedTextRecords = records
			textRecordsReady = true
			files.Steps = append(files.Steps, keywordReportStep("Write selected BLAST Excel and fetch peptide sequences", stepStart, time.Now(), "ok", fmt.Sprintf("%d selected rows written; %d peptide records available", len(selectedRows), len(records))))
		} else {
			writeExcel := func() error {
				return export.WriteBlastResultsExcelWithMetadata(excelPath, selectedRows, exportMetadata, &export.BlastExcelExportOptions{RowNumbers: rowNumbers, FilterFlags: filterFlags})
			}
			var err error
			if w.suppressTaskModals {
				err = writeExcel()
			} else {
				err = withSpinner(w.out, "Writing selected BLAST Excel file...", writeExcel)
			}
			if err != nil {
				files.Steps = append(files.Steps, keywordReportStep("Write selected BLAST Excel", stepStart, time.Now(), "failed", err.Error()))
				return exportFileResult{}, err
			}
			files.Steps = append(files.Steps, keywordReportStep("Write selected BLAST Excel", stepStart, time.Now(), "ok", fmt.Sprintf("%d selected rows written", len(selectedRows))))
		}
		files.ExcelPath = excelPath
	}
	if settings.WriteRawExcel && settings.WriteText {
		rawPath := filepath.Join(outputDir, fileBaseName+"_raw.xlsx")
		rawTextPath := filepath.Join(outputDir, fileBaseName+"_raw.fasta")
		exportMetadata := buildExportMetadata(displayName, querySource)
		rawExcelSteps, rawTextSteps, err := runParallelExportSteps(
			func() ([]report.GenerationStep, error) {
				stepStart := time.Now()
				err := export.WriteBlastResultsExcelWithMetadata(rawPath, allRows, exportMetadata, &export.BlastExcelExportOptions{FilterFlags: filterFlags})
				if err != nil {
					return []report.GenerationStep{keywordReportStep("Write raw BLAST Excel", stepStart, time.Now(), "failed", err.Error())}, err
				}
				return []report.GenerationStep{keywordReportStep("Write raw BLAST Excel", stepStart, time.Now(), "ok", fmt.Sprintf("%d current rows written", len(allRows)))}, nil
			},
			func() ([]report.GenerationStep, error) {
				steps := make([]report.GenerationStep, 0, 3)
				stepStart := time.Now()
				rawRecords, err := w.fetchBlastRecordsForExport(ctx, allRows, exportMetadata)
				if err != nil {
					return append(steps, keywordReportStep("Fetch raw BLAST peptide sequences", stepStart, time.Now(), "failed", err.Error())), err
				}
				steps = append(steps, keywordReportStep("Fetch raw BLAST peptide sequences", stepStart, time.Now(), "ok", fmt.Sprintf("%d peptide records available", len(rawRecords))))
				hitRecords := append([]model.ProteinSequenceRecord(nil), rawRecords...)
				prependStart := time.Now()
				rawRecords = prependQuerySequenceRecord(rawRecords, querySource, txtHeaderLabel)
				if settings.UsePhgoHeader {
					rawRecords = applyBlastPhgoHeaders(rawRecords, allRows, len(rawRecords)-len(hitRecords))
				} else {
					rawRecords = applyOriginalHeaders(rawRecords)
				}
				steps = append(steps, keywordReportStep("Prepend query sequence record to raw text", prependStart, time.Now(), "ok", blastQueryPrependStepDetails(querySource, rawRecords, hitRecords)))
				writeStart := time.Now()
				if err := export.WriteProteinSequencesText(rawTextPath, rawRecords); err != nil {
					return append(steps, keywordReportStep("Write raw BLAST peptide text", writeStart, time.Now(), "failed", err.Error())), err
				}
				return append(steps, keywordReportStep("Write raw BLAST peptide text", writeStart, time.Now(), "ok", fmt.Sprintf("%d sequence records written", len(rawRecords)))), nil
			},
			w.out,
			w.suppressTaskModals,
			"Writing raw BLAST export files...",
		)
		files.Steps = append(files.Steps, rawExcelSteps...)
		files.Steps = append(files.Steps, rawTextSteps...)
		if err != nil {
			return exportFileResult{}, err
		}
		files.RawExcelPath = rawPath
		files.RawTextPath = rawTextPath
	} else if settings.WriteRawExcel {
		rawPath := filepath.Join(outputDir, fileBaseName+"_raw.xlsx")
		stepStart := time.Now()
		writeRawExcel := func() error {
			return export.WriteBlastResultsExcelWithMetadata(rawPath, allRows, buildExportMetadata(displayName, querySource), &export.BlastExcelExportOptions{FilterFlags: filterFlags})
		}
		var err error
		if w.suppressTaskModals {
			err = writeRawExcel()
		} else {
			err = withSpinner(w.out, "Writing raw BLAST Excel file...", writeRawExcel)
		}
		if err != nil {
			files.Steps = append(files.Steps, keywordReportStep("Write raw BLAST Excel", stepStart, time.Now(), "failed", err.Error()))
			return exportFileResult{}, err
		}
		files.Steps = append(files.Steps, keywordReportStep("Write raw BLAST Excel", stepStart, time.Now(), "ok", fmt.Sprintf("%d current rows written", len(allRows))))
		files.RawExcelPath = rawPath
	}
	if settings.WriteText {
		textPath := filepath.Join(outputDir, fileBaseName+".fasta")
		exportMetadata := buildExportMetadata(displayName, querySource)
		records := prefetchedTextRecords
		if !textRecordsReady {
			stepStart := time.Now()
			var err error
			records, err = w.fetchBlastRecordsForExport(ctx, selectedRows, exportMetadata)
			if err != nil {
				files.Steps = append(files.Steps, keywordReportStep("Fetch BLAST peptide sequences", stepStart, time.Now(), "failed", err.Error()))
				return exportFileResult{}, err
			}
			files.Steps = append(files.Steps, keywordReportStep("Fetch BLAST peptide sequences", stepStart, time.Now(), "ok", fmt.Sprintf("%d peptide records available", len(records))))
		}
		hitRecords := append([]model.ProteinSequenceRecord(nil), records...)
		prependStart := time.Now()
		records = prependQuerySequenceRecord(records, querySource, txtHeaderLabel)
		if settings.UsePhgoHeader {
			records = applyBlastPhgoHeaders(records, selectedRows, len(records)-len(hitRecords))
		} else {
			records = applyOriginalHeaders(records)
		}
		files.Steps = append(files.Steps, keywordReportStep("Prepend query sequence record", prependStart, time.Now(), "ok", blastQueryPrependStepDetails(querySource, records, hitRecords)))
		writeText := func() error {
			return export.WriteProteinSequencesText(textPath, records)
		}
		var err error
		stepStart := time.Now()
		if w.suppressTaskModals {
			err = writeText()
		} else {
			err = withSpinner(w.out, "Writing peptide text file...", writeText)
		}
		if err != nil {
			files.Steps = append(files.Steps, keywordReportStep("Write BLAST peptide text", stepStart, time.Now(), "failed", err.Error()))
			return exportFileResult{}, err
		}
		files.Steps = append(files.Steps, keywordReportStep("Write BLAST peptide text", stepStart, time.Now(), "ok", fmt.Sprintf("%d sequence records written", len(records))))
		files.TextPath = textPath
		files.SequenceRecords = records
		files.SequenceAudit = buildBlastSequenceAudit(selectedRows, records, []*model.QuerySequenceSource{querySource}, true)
	}
	if showComplete {
		return files, w.showInfo("Export complete", filesSummary(files), prompt.ErrBackToRowSelection)
	}
	return files, nil
}

func (w *BlastWizard) exportFamilyBlastSelectionsToDir(ctx context.Context, selectedRows []model.BlastResultRow, allRows []model.BlastResultRow, rowNumbers []int, filterFlags []bool, querySources []*model.QuerySequenceSource, displayName string, txtHeaderLabel string, fileBaseName string, outputDir string, settings exportSettings, familySettings model.FamilyBlastSettings, showComplete bool) (exportFileResult, error) {
	if len(querySources) <= 1 {
		var querySource *model.QuerySequenceSource
		if len(querySources) == 1 {
			querySource = querySources[0]
		}
		return w.exportBlastSelectionsToDir(ctx, selectedRows, allRows, rowNumbers, filterFlags, querySource, displayName, txtHeaderLabel, fileBaseName, outputDir, settings, showComplete)
	}
	files := exportFileResult{SequenceAudit: report.SequenceAudit{Requested: settings.WriteText}}
	exportMetadata := buildFamilyExportMetadata(querySources)
	var prefetchedTextRecords []model.ProteinSequenceRecord
	textRecordsReady := false
	if settings.WriteExcel {
		excelPath := filepath.Join(outputDir, fileBaseName+".xlsx")
		stepStart := time.Now()
		if settings.WriteText {
			records, err := w.exportBlastExcelAndFetchRecords(ctx, selectedRows, rowNumbers, filterFlags, excelPath, exportMetadata)
			if err != nil {
				files.Steps = append(files.Steps, keywordReportStep("Write selected Family BLAST Excel and fetch peptide sequences", stepStart, time.Now(), "failed", err.Error()))
				return exportFileResult{}, err
			}
			prefetchedTextRecords = records
			textRecordsReady = true
			files.Steps = append(files.Steps, keywordReportStep("Write selected Family BLAST Excel and fetch peptide sequences", stepStart, time.Now(), "ok", fmt.Sprintf("%d selected rows written; %d peptide records available", len(selectedRows), len(records))))
		} else {
			writeExcel := func() error {
				return export.WriteBlastResultsExcelWithMetadata(excelPath, selectedRows, exportMetadata, &export.BlastExcelExportOptions{RowNumbers: rowNumbers, FilterFlags: filterFlags})
			}
			var err error
			if w.suppressTaskModals {
				err = writeExcel()
			} else {
				err = withSpinner(w.out, "Writing selected BLAST Excel file...", writeExcel)
			}
			if err != nil {
				files.Steps = append(files.Steps, keywordReportStep("Write selected Family BLAST Excel", stepStart, time.Now(), "failed", err.Error()))
				return exportFileResult{}, err
			}
			files.Steps = append(files.Steps, keywordReportStep("Write selected Family BLAST Excel", stepStart, time.Now(), "ok", fmt.Sprintf("%d selected rows written", len(selectedRows))))
		}
		files.ExcelPath = excelPath
	}
	if settings.WriteRawExcel && settings.WriteText {
		rawPath := filepath.Join(outputDir, fileBaseName+"_raw.xlsx")
		rawTextPath := filepath.Join(outputDir, fileBaseName+"_raw.fasta")
		rawExcelSteps, rawTextSteps, err := runParallelExportSteps(
			func() ([]report.GenerationStep, error) {
				stepStart := time.Now()
				err := export.WriteBlastResultsExcelWithMetadata(rawPath, allRows, exportMetadata, &export.BlastExcelExportOptions{FilterFlags: filterFlags})
				if err != nil {
					return []report.GenerationStep{keywordReportStep("Write raw Family BLAST Excel", stepStart, time.Now(), "failed", err.Error())}, err
				}
				return []report.GenerationStep{keywordReportStep("Write raw Family BLAST Excel", stepStart, time.Now(), "ok", fmt.Sprintf("%d current family rows written", len(allRows)))}, nil
			},
			func() ([]report.GenerationStep, error) {
				steps := make([]report.GenerationStep, 0, 3)
				stepStart := time.Now()
				rawRecords, err := w.fetchBlastRecordsForExport(ctx, allRows, nil)
				if err != nil {
					return append(steps, keywordReportStep("Fetch raw Family BLAST peptide sequences", stepStart, time.Now(), "failed", err.Error())), err
				}
				steps = append(steps, keywordReportStep("Fetch raw Family BLAST peptide sequences", stepStart, time.Now(), "ok", fmt.Sprintf("%d peptide records available", len(rawRecords))))
				hitRecords := append([]model.ProteinSequenceRecord(nil), rawRecords...)
				prependStart := time.Now()
				var prependedQueries int
				rawRecords, prependedQueries = prependFamilyQuerySequenceRecords(rawRecords, querySources, txtHeaderLabel, familySettings)
				if settings.UsePhgoHeader {
					rawRecords = applyBlastPhgoHeaders(rawRecords, allRows, prependedQueries)
				} else {
					rawRecords = applyOriginalHeaders(rawRecords)
				}
				steps = append(steps, keywordReportStep("Prepend Family BLAST query sequence records to raw text", prependStart, time.Now(), "ok", familyQueryPrependStepDetails(prependedQueries, len(querySources), familySettings.PrependOnlyFirstQuery, len(hitRecords))))
				writeStart := time.Now()
				if err := export.WriteProteinSequencesText(rawTextPath, rawRecords); err != nil {
					return append(steps, keywordReportStep("Write raw Family BLAST peptide text", writeStart, time.Now(), "failed", err.Error())), err
				}
				return append(steps, keywordReportStep("Write raw Family BLAST peptide text", writeStart, time.Now(), "ok", fmt.Sprintf("%d sequence records written", len(rawRecords)))), nil
			},
			w.out,
			w.suppressTaskModals,
			"Writing raw Family BLAST export files...",
		)
		files.Steps = append(files.Steps, rawExcelSteps...)
		files.Steps = append(files.Steps, rawTextSteps...)
		if err != nil {
			return exportFileResult{}, err
		}
		files.RawExcelPath = rawPath
		files.RawTextPath = rawTextPath
	} else if settings.WriteRawExcel {
		rawPath := filepath.Join(outputDir, fileBaseName+"_raw.xlsx")
		stepStart := time.Now()
		writeRawExcel := func() error {
			return export.WriteBlastResultsExcelWithMetadata(rawPath, allRows, exportMetadata, &export.BlastExcelExportOptions{FilterFlags: filterFlags})
		}
		var err error
		if w.suppressTaskModals {
			err = writeRawExcel()
		} else {
			err = withSpinner(w.out, "Writing raw BLAST Excel file...", writeRawExcel)
		}
		if err != nil {
			files.Steps = append(files.Steps, keywordReportStep("Write raw Family BLAST Excel", stepStart, time.Now(), "failed", err.Error()))
			return exportFileResult{}, err
		}
		files.Steps = append(files.Steps, keywordReportStep("Write raw Family BLAST Excel", stepStart, time.Now(), "ok", fmt.Sprintf("%d current family rows written", len(allRows))))
		files.RawExcelPath = rawPath
	}
	if settings.WriteText {
		textPath := filepath.Join(outputDir, fileBaseName+".fasta")
		records := prefetchedTextRecords
		if !textRecordsReady {
			stepStart := time.Now()
			var err error
			records, err = w.fetchBlastRecordsForExport(ctx, selectedRows, nil)
			if err != nil {
				files.Steps = append(files.Steps, keywordReportStep("Fetch Family BLAST peptide sequences", stepStart, time.Now(), "failed", err.Error()))
				return exportFileResult{}, err
			}
			files.Steps = append(files.Steps, keywordReportStep("Fetch Family BLAST peptide sequences", stepStart, time.Now(), "ok", fmt.Sprintf("%d peptide records available", len(records))))
		}
		hitRecords := append([]model.ProteinSequenceRecord(nil), records...)
		prependStart := time.Now()
		var prependedQueries int
		records, prependedQueries = prependFamilyQuerySequenceRecords(records, querySources, txtHeaderLabel, familySettings)
		if settings.UsePhgoHeader {
			records = applyBlastPhgoHeaders(records, selectedRows, prependedQueries)
		} else {
			records = applyOriginalHeaders(records)
		}
		files.Steps = append(files.Steps, keywordReportStep("Prepend Family BLAST query sequence records", prependStart, time.Now(), "ok", familyQueryPrependStepDetails(prependedQueries, len(querySources), familySettings.PrependOnlyFirstQuery, len(hitRecords))))
		writeText := func() error {
			return export.WriteProteinSequencesText(textPath, records)
		}
		var writeErr error
		stepStart := time.Now()
		if w.suppressTaskModals {
			writeErr = writeText()
		} else {
			writeErr = withSpinner(w.out, "Writing peptide text file...", writeText)
		}
		if writeErr != nil {
			files.Steps = append(files.Steps, keywordReportStep("Write Family BLAST peptide text", stepStart, time.Now(), "failed", writeErr.Error()))
			return exportFileResult{}, writeErr
		}
		files.Steps = append(files.Steps, keywordReportStep("Write Family BLAST peptide text", stepStart, time.Now(), "ok", fmt.Sprintf("%d sequence records written", len(records))))
		files.TextPath = textPath
		files.SequenceRecords = records
		files.SequenceAudit = buildBlastSequenceAudit(selectedRows, records, querySources, true)
		files.SequenceAudit.HeaderLabelMode = familySequenceHeaderMode(familySettings.PrependOnlyFirstQuery)
	}
	if showComplete {
		return files, w.showInfo("Export complete", filesSummary(files), prompt.ErrBackToRowSelection)
	}
	return files, nil
}

func runParallelExportSteps(left func() ([]report.GenerationStep, error), right func() ([]report.GenerationStep, error), out io.Writer, suppressModal bool, label string) ([]report.GenerationStep, []report.GenerationStep, error) {
	type result struct {
		steps []report.GenerationStep
		err   error
	}
	run := func() (result, result, error) {
		var wg sync.WaitGroup
		leftCh := make(chan result, 1)
		rightCh := make(chan result, 1)
		wg.Add(2)
		go func() {
			defer wg.Done()
			steps, err := left()
			leftCh <- result{steps: steps, err: err}
		}()
		go func() {
			defer wg.Done()
			steps, err := right()
			rightCh <- result{steps: steps, err: err}
		}()
		wg.Wait()
		close(leftCh)
		close(rightCh)
		leftResult := <-leftCh
		rightResult := <-rightCh
		if leftResult.err != nil {
			return leftResult, rightResult, leftResult.err
		}
		if rightResult.err != nil {
			return leftResult, rightResult, rightResult.err
		}
		return leftResult, rightResult, nil
	}
	var leftResult, rightResult result
	var err error
	if suppressModal {
		leftResult, rightResult, err = run()
	} else {
		err = withSpinner(out, label, func() error {
			var runErr error
			leftResult, rightResult, runErr = run()
			return runErr
		})
	}
	return leftResult.steps, rightResult.steps, err
}

func familyTXTQueryIndexes(querySources []*model.QuerySequenceSource, settings model.FamilyBlastSettings) []int {
	indexes := make([]int, 0, len(querySources))
	for i, source := range querySources {
		if source != nil {
			indexes = append(indexes, i)
		}
	}
	if settings.PrependOnlyFirstQuery && len(indexes) > 1 {
		return indexes[:1]
	}
	return indexes
}

func familyTXTHeaderLabel(source *model.QuerySequenceSource, fallback string) string {
	if source == nil {
		return strings.TrimSpace(fallback)
	}
	for _, value := range []string{
		strings.TrimSpace(source.LabelName),
		labelname.FirstAlias(source.Aliases),
		preferredPhgoIdentifier(source),
		strings.TrimSpace(fallback),
	} {
		if value != "" {
			return value
		}
	}
	return ""
}

func familyQueryPrependStepDetails(prependedQueries int, totalQueries int, onlyFirst bool, hitRecords int) string {
	switch {
	case onlyFirst:
		return fmt.Sprintf("%d of %d family query record(s) prepended (first query only mode); %d hit peptide records already available", prependedQueries, totalQueries, hitRecords)
	case prependedQueries == 1:
		return fmt.Sprintf("1 family query record prepended; %d hit peptide records already available", hitRecords)
	default:
		return fmt.Sprintf("%d family query records prepended; %d hit peptide records already available", prependedQueries, hitRecords)
	}
}

func familySequenceHeaderMode(onlyFirst bool) string {
	if onlyFirst {
		return "family text export prepends only the first family member query header; hit records append selected row label_name"
	}
	return "family text export prepends all family member query headers in run order; hit records append selected row label_name"
}

func prependFamilyQuerySequenceRecords(records []model.ProteinSequenceRecord, querySources []*model.QuerySequenceSource, fallback string, familySettings model.FamilyBlastSettings) ([]model.ProteinSequenceRecord, int) {
	prepended := 0
	queryIndexes := familyTXTQueryIndexes(querySources, familySettings)
	for i := len(queryIndexes) - 1; i >= 0; i-- {
		source := querySources[queryIndexes[i]]
		if source == nil {
			continue
		}
		headerLabel := familyTXTHeaderLabel(source, fallback)
		records = prependQuerySequenceRecord(records, source, headerLabel)
		prepended++
	}
	return records, prepended
}

func (w *BlastWizard) exportBlastExcelAndFetchRecords(ctx context.Context, rows []model.BlastResultRow, rowNumbers []int, filterFlags []bool, excelPath string, metadata *model.ExportMetadata) ([]model.ProteinSequenceRecord, error) {
	if w.suppressTaskModals {
		return w.exportBlastExcelAndFetchRecordsSilent(ctx, rows, rowNumbers, filterFlags, excelPath, metadata)
	}
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Export", "Writing files"),
		Title:       "Writing BLAST export files",
		Description: "Writing the Excel file while fetching peptide sequences for the text export.",
		Initial:     "Starting export...",
		Total:       len(rows) + 1,
		CancelError: prompt.ErrBackToRowSelection,
	}, func(taskCtx context.Context, update func(int, string)) ([]model.ProteinSequenceRecord, error) {
		exportCtx := mergeContexts(ctx, taskCtx)
		progress := safeProgress(update)
		type excelResult struct {
			err error
		}
		excelDone := make(chan excelResult, 1)
		go func() {
			excelDone <- excelResult{err: export.WriteBlastResultsExcelWithMetadata(excelPath, rows, metadata, &export.BlastExcelExportOptions{RowNumbers: rowNumbers, FilterFlags: filterFlags})}
		}()
		records, fetchErr := w.fetchProteinSequenceRecordsWithProgress(exportCtx, rows, func(current int, message string) {
			progress(current, message)
		})
		excel := <-excelDone
		if excel.err != nil {
			return nil, excel.err
		}
		progress(len(rows)+1, "Wrote Excel file and fetched peptide sequences.")
		if fetchErr != nil {
			return nil, fetchErr
		}
		return records, nil
	})
}

func (w *BlastWizard) exportBlastExcelAndFetchRecordsSilent(ctx context.Context, rows []model.BlastResultRow, rowNumbers []int, filterFlags []bool, excelPath string, metadata *model.ExportMetadata) ([]model.ProteinSequenceRecord, error) {
	type excelResult struct {
		err error
	}
	excelDone := make(chan excelResult, 1)
	go func() {
		excelDone <- excelResult{err: export.WriteBlastResultsExcelWithMetadata(excelPath, rows, metadata, &export.BlastExcelExportOptions{RowNumbers: rowNumbers, FilterFlags: filterFlags})}
	}()
	records, fetchErr := w.fetchProteinSequenceRecordsWithProgress(ctx, rows, nil)
	excel := <-excelDone
	if excel.err != nil {
		return nil, excel.err
	}
	if fetchErr != nil {
		return nil, fetchErr
	}
	return records, nil
}

func (w *BlastWizard) fetchBlastRecordsForExport(ctx context.Context, rows []model.BlastResultRow, metadata *model.ExportMetadata) ([]model.ProteinSequenceRecord, error) {
	if w.suppressTaskModals {
		return w.fetchProteinSequenceRecordsWithProgress(ctx, rows, nil)
	}
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Export", "Writing files"),
		Title:       "Preparing BLAST text export",
		Description: "Fetching peptide sequences for the text export.",
		Initial:     "Starting text export...",
		Total:       len(rows),
		CancelError: prompt.ErrBackToRowSelection,
	}, func(taskCtx context.Context, update func(int, string)) ([]model.ProteinSequenceRecord, error) {
		_ = metadata
		return w.fetchProteinSequenceRecordsWithProgress(mergeContexts(ctx, taskCtx), rows, func(current int, message string) {
			update(current, message)
		})
	})
}

func filesSummary(files exportFileResult) string {
	lines := []string{}
	if strings.TrimSpace(files.TextPath) != "" {
		lines = append(lines, "Text\n"+files.TextPath)
	}
	if strings.TrimSpace(files.ExcelPath) != "" {
		lines = append(lines, "Excel\n"+files.ExcelPath)
	}
	if strings.TrimSpace(files.RawExcelPath) != "" {
		lines = append(lines, "Raw Excel\n"+files.RawExcelPath)
	}
	if strings.TrimSpace(files.RawTextPath) != "" {
		lines = append(lines, "Raw text\n"+files.RawTextPath)
	}
	if strings.TrimSpace(files.ReportPath) != "" {
		lines = append(lines, "Data analysis report (PDF)\n"+files.ReportPath)
	}
	if len(lines) == 0 {
		return "No files were written."
	}
	return strings.Join(lines, "\n\n")
}

func (w *BlastWizard) collectQuerySequence(ctx context.Context, candidates []model.SpeciesCandidate) (string, *model.QuerySequenceSource, error) {
	for {
		sequenceInput, err := w.prompt.SequenceInput()
		if err != nil {
			if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
				return "", nil, err
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read query input: %v", err), prompt.ErrBackToSpeciesSelection)
			if navErr != nil {
				return "", nil, navErr
			}
			if !retry {
				return "", nil, err
			}
			continue
		}
		if strings.TrimSpace(sequenceInput) == "" {
			if err := w.showInfo("BLAST input", "Sequence input was empty. Please paste a sequence, FASTA entry, or Phytozome URL.", prompt.ErrBackToSpeciesSelection); err != nil {
				return "", nil, err
			}
			continue
		}

		sequence := sequenceInput
		var querySource *model.QuerySequenceSource
		if source, ok, err := w.resolveQuerySequenceInput(ctx, candidates, sequenceInput); err != nil {
			if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
				return "", nil, err
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("resolve query input: %v", err), prompt.ErrBackToSpeciesSelection)
			if navErr != nil {
				return "", nil, navErr
			}
			if !retry {
				return "", nil, err
			}
			continue
		} else if ok {
			querySource = source
			sequence = source.Sequence
			if err := w.showInfo("Query source", describeQuerySourceDetails(source, w.source.Name()), prompt.ErrBackToQueryInput); err != nil {
				return "", nil, err
			}
		}

		return sequence, querySource, nil
	}
}

func (w *BlastWizard) submitBlastWithRetry(ctx context.Context, request model.BlastRequest) (model.BlastJob, error) {
	if w.suppressTaskModals {
		return w.submitBlastOnce(ctx, request)
	}
	for {
		job, err := w.submitBlastOnce(ctx, request)
		if err == nil {
			return job, nil
		}
		var missingTools *blastplus.MissingToolsError
		if errors.As(err, &missingTools) {
			return model.BlastJob{}, err
		}
		if !isLocalBlastRequest(request) {
			localOK, localErr := w.canRunLocalBlastFallback(ctx, request)
			if localErr != nil {
				err = fmt.Errorf("%w; local fallback check failed: %v", err, localErr)
			} else if localOK {
				request.Program = "local:" + request.Program
				continue
			}
		}
		action, actionErr := w.prompt.BlastSubmitErrorAction(fmt.Sprintf("submit BLAST job: %v", err))
		if actionErr != nil {
			return model.BlastJob{}, actionErr
		}
		decision, navErr := interpretRecoveryAction(action, prompt.ErrBackToQueryInput, false)
		if navErr != nil {
			return model.BlastJob{}, navErr
		}
		switch decision {
		case recoveryRetry:
			continue
		default:
			return model.BlastJob{}, prompt.ErrBackToQueryInput
		}
	}
}

func (w *BlastWizard) promptInstallBlastPlus(ctx context.Context, description string, cancelTarget error) (bool, error) {
	action, actionErr := w.prompt.BlastPlusInstallAction(description)
	if actionErr != nil {
		return false, actionErr
	}
	if action != "install" {
		return false, nil
	}
	if _, installErr := tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "Install BLAST+"),
		Title:       "Installing BLAST+",
		Description: "Downloading and preparing managed NCBI BLAST+ tools for local BLAST.",
		Initial:     "Installing BLAST+...",
		Total:       100,
		CancelError: cancelTarget,
	}, func(taskCtx context.Context, update func(current int, message string)) (string, error) {
		progressCtx := progressctx.WithProgress(mergeContexts(ctx, taskCtx), update)
		update(0, "Downloading and extracting BLAST+...")
		return blastplus.InstallManaged(progressCtx, w.httpClient)
	}); installErr != nil {
		return false, fmt.Errorf("install BLAST+: %w", installErr)
	}
	return true, nil
}

func (w *BlastWizard) submitBlastOnce(ctx context.Context, request model.BlastRequest) (model.BlastJob, error) {
	if w.suppressTaskModals {
		if lc, ok := w.source.(*lemna.Client); ok {
			if isLocalBlastRequest(request) {
				return lc.SubmitBlast(ctx, request)
			}
			return lc.SubmitBlastServerOnly(ctx, request)
		}
		return w.source.SubmitBlast(ctx, request)
	}
	if lc, ok := w.source.(*lemna.Client); ok {
		if isLocalBlastRequest(request) {
			return tui.RunTaskValueContext(tui.TaskPage{
				Path:        w.tuiPath("BLAST", "Local BLAST"),
				Title:       "Running local BLAST",
				Description: "Downloading required FASTA files when needed, preparing BLAST databases, and running BLAST+ locally.",
				Initial:     "Starting local BLAST+...",
				CancelError: prompt.ErrBackToQueryInput,
			}, func(taskCtx context.Context, update func(string)) (model.BlastJob, error) {
				safeTaskUpdate(update)("Preparing local BLAST+ run...")
				return lc.SubmitBlast(mergeContexts(ctx, taskCtx), request)
			})
		}
		return tui.RunTaskValueContext(tui.TaskPage{
			Path:        w.tuiPath("BLAST", "Online BLAST"),
			Title:       "Trying online BLAST",
			Description: "Submitting the query to the lemna.org BLAST service. If it cannot return a usable result, the CLI will automatically continue with local BLAST+ when available.",
			Initial:     "Submitting to lemna.org...",
			CancelError: prompt.ErrBackToQueryInput,
		}, func(taskCtx context.Context, update func(string)) (model.BlastJob, error) {
			safeTaskUpdate(update)("Submitting to lemna.org BLAST...")
			return lc.SubmitBlastServerOnly(mergeContexts(ctx, taskCtx), request)
		})
	}

	return tui.RunTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "Online BLAST"),
		Title:       "Submitting BLAST job",
		Description: "Submitting the BLAST query to the selected remote service.",
		Initial:     "Submitting BLAST job...",
		CancelError: prompt.ErrBackToQueryInput,
	}, func(taskCtx context.Context, update func(string)) (model.BlastJob, error) {
		safeTaskUpdate(update)("Submitting BLAST job...")
		return w.source.SubmitBlast(mergeContexts(ctx, taskCtx), request)
	})
}

func (w *BlastWizard) canRunLocalBlastFallback(ctx context.Context, request model.BlastRequest) (bool, error) {
	lc, ok := w.source.(*lemna.Client)
	if !ok {
		return false, nil
	}
	if w.suppressTaskModals {
		cap, err := lc.DetectBlastCapabilities(ctx, request.Species)
		if err != nil {
			return false, err
		}
		switch normalizeWorkflowBlastProgram(request.Program) {
		case "blastn", "tblastn":
			return cap.HasNucleotideFasta, nil
		case "blastx", "blastp":
			return cap.HasProteinFasta, nil
		default:
			return false, nil
		}
	}
	cap, err := tui.RunTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "Local fallback"),
		Title:       "Checking local fallback",
		Description: "Checking whether the selected species has downloadable FASTA files for local BLAST+.",
		Initial:     "Checking local BLAST availability...",
		CancelError: prompt.ErrBackToQueryInput,
	}, func(taskCtx context.Context, update func(string)) (lemna.BlastCapability, error) {
		safeTaskUpdate(update)("Checking local FASTA downloads...")
		return lc.DetectBlastCapabilities(mergeContexts(ctx, taskCtx), request.Species)
	})
	if err != nil {
		return false, err
	}
	switch normalizeWorkflowBlastProgram(request.Program) {
	case "blastn", "tblastn":
		return cap.HasNucleotideFasta, nil
	case "blastx", "blastp":
		return cap.HasProteinFasta, nil
	default:
		return false, nil
	}
}

func isLocalBlastRequest(request model.BlastRequest) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(request.Program)), "local:")
}

func normalizeWorkflowBlastProgram(program string) string {
	program = strings.TrimSpace(strings.ToLower(program))
	program = strings.TrimPrefix(program, "local:")
	return program
}

func (w *BlastWizard) waitForBlastResultsWithRetry(ctx context.Context, jobID string) (model.BlastResult, error) {
	pollInterval := blastResultsPollInterval(w.source)
	if w.suppressTaskModals {
		return w.source.WaitForBlastResults(ctx, jobID, pollInterval, 0)
	}
	for {
		var results model.BlastResult
		var err error
		if w.suppressTaskModals {
			results, err = w.source.WaitForBlastResults(ctx, jobID, pollInterval, 0)
		} else {
			results, err = w.waitForBlastResultsWithProgress(ctx, jobID, pollInterval, 0)
		}
		if err == nil {
			return results, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, tui.ErrTaskCancelled) || errors.Is(err, prompt.ErrBackToQueryInput) {
			return model.BlastResult{}, prompt.ErrBackToQueryInput
		}
		retry, navErr := w.retryWorkflowStep(fmt.Sprintf("wait for BLAST results for job %s: %v", jobID, err), prompt.ErrBackToQueryInput)
		if navErr != nil {
			return model.BlastResult{}, navErr
		}
		if !retry {
			return model.BlastResult{}, err
		}
	}
}

func blastResultsPollInterval(src source.DataSource) time.Duration {
	if configured := configuredInt("PHYTOZOME_GO_BLAST_POLL_MS", 0); configured > 0 {
		return time.Duration(configured) * time.Millisecond
	}
	if src == nil {
		return 2 * time.Second
	}
	switch strings.ToLower(strings.TrimSpace(src.Name())) {
	case "phytozome":
		if configured := configuredInt("PHYTOZOME_GO_PHY_BLAST_POLL_MS", 0); configured > 0 {
			return time.Duration(configured) * time.Millisecond
		}
		return time.Second
	case "lemna":
		if configured := configuredInt("PHYTOZOME_GO_LEMNA_BLAST_POLL_MS", 0); configured > 0 {
			return time.Duration(configured) * time.Millisecond
		}
		return 2 * time.Second
	default:
		return 2 * time.Second
	}
}

func (w *BlastWizard) selectBlastRows(rows []model.BlastResultRow) ([]model.BlastResultRow, error) {
	for {
		selectedRows, err := w.prompt.SelectBlastRows(rows)
		if err == nil {
			return selectedRows, nil
		}
		if errors.Is(err, prompt.ErrBackToBlastProgram) || errors.Is(err, prompt.ErrBackToQueryInput) || errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
			return nil, err
		}
		retry, navErr := w.retryWorkflowStep(fmt.Sprintf("select BLAST rows: %v", err), prompt.ErrBackToQueryInput)
		if navErr != nil {
			return nil, navErr
		}
		if !retry {
			return nil, err
		}
	}
}

func (w *BlastWizard) selectKeywordRows(groups []model.KeywordSearchGroup) (prompt.KeywordRowSelection, error) {
	for {
		selection, err := w.prompt.SelectKeywordRows(groups)
		if err == nil {
			return selection, nil
		}
		if errors.Is(err, prompt.ErrBackToQueryInput) || errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
			return prompt.KeywordRowSelection{}, err
		}
		retry, navErr := w.retryWorkflowStep(fmt.Sprintf("select keyword rows: %v", err), prompt.ErrBackToQueryInput)
		if navErr != nil {
			return prompt.KeywordRowSelection{}, navErr
		}
		if !retry {
			return prompt.KeywordRowSelection{}, err
		}
	}
}

func (w *BlastWizard) exportSelectionsWithRetry(ctx context.Context, rows []model.BlastResultRow, querySource *model.QuerySequenceSource, baseName string, settings exportSettings) error {
	for {
		err := w.exportSelections(ctx, rows, rows, querySource, baseName, settings)
		if err == nil {
			return nil
		}
		if errors.Is(err, prompt.ErrBackToRowSelection) || errors.Is(err, prompt.ErrBackToQueryInput) || errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
			return err
		}
		retry, navErr := w.retryWorkflowStep(fmt.Sprintf("export selections: %v", err), prompt.ErrBackToRowSelection)
		if navErr != nil {
			return navErr
		}
		if !retry {
			return err
		}
	}
}

func (w *BlastWizard) exportKeywordSelectionsWithRetry(ctx context.Context, selected model.SpeciesCandidate, rows []model.KeywordResultRow, allRows []model.KeywordResultRow, groups []model.KeywordSearchGroup, baseName string, outputDir string, settings exportSettings, reportCtx *keywordReportRunContext) error {
	for {
		err := w.exportKeywordSelections(ctx, selected, rows, allRows, groups, baseName, outputDir, settings, reportCtx)
		if err == nil {
			return nil
		}
		if errors.Is(err, prompt.ErrBackToRowSelection) || errors.Is(err, prompt.ErrBackToQueryInput) || errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
			return err
		}
		retry, navErr := w.retryWorkflowStep(fmt.Sprintf("export keyword selections: %v", err), prompt.ErrBackToRowSelection)
		if navErr != nil {
			return navErr
		}
		if !retry {
			return err
		}
	}
}

func flattenKeywordSearchGroups(groups []model.KeywordSearchGroup) []model.KeywordResultRow {
	out := make([]model.KeywordResultRow, 0, countKeywordRows(groups))
	for _, group := range groups {
		out = append(out, group.Rows...)
	}
	return out
}

func (w *BlastWizard) prepareAndExportKeywordSelection(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup, rows []model.KeywordResultRow, reportCtx *keywordReportRunContext) error {
	exportRows := append([]model.KeywordResultRow(nil), rows...)
	settings, err := w.prepareExportSettings(defaultKeywordExportLabel(exportRows, groups), false, true, false)
	if err != nil {
		return err
	}
	baseName := settings.BaseName
	if err := w.exportKeywordSelectionsWithRetry(ctx, selected, exportRows, flattenKeywordSearchGroups(groups), groups, baseName, settings.OutputDir, settings, reportCtx); err != nil {
		return err
	}
	return nil
}

func (w *BlastWizard) retryWorkflowStep(description string, backTarget error) (bool, error) {
	action, err := w.prompt.WorkflowErrorAction(description, backTarget)
	if err != nil {
		return false, err
	}
	decision, navErr := interpretRecoveryAction(action, backTarget, false)
	if navErr != nil {
		return false, navErr
	}
	return decision == recoveryRetry, nil
}

func interpretRecoveryAction(action string, backTarget error, allowSkip bool) (recoveryDecision, error) {
	switch action {
	case "retry":
		return recoveryRetry, nil
	case "skip":
		if allowSkip {
			return recoverySkip, nil
		}
		return 0, fmt.Errorf("unsupported recovery action %q", action)
	case "back", "close":
		if backTarget != nil {
			return recoveryBack, backTarget
		}
		return recoveryBack, prompt.ErrBackToQueryInput
	case "exit":
		return recoveryExit, prompt.ErrExitRequested
	case "":
		if backTarget != nil {
			return recoveryBack, backTarget
		}
		return recoveryBack, prompt.ErrBackToQueryInput
	default:
		return 0, fmt.Errorf("unsupported recovery action %q", action)
	}
}

func (w *BlastWizard) showInfo(title string, message string, backTarget error) error {
	result, err := tui.RunInfoPage(tui.InfoPage{
		Path:        w.tuiPath("Status", title),
		Title:       title,
		Message:     message,
		AllowBack:   backTarget != nil,
		AllowHome:   true,
		ConfirmText: "OK",
	})
	if err != nil {
		return err
	}
	switch result.Nav {
	case tui.NavBack:
		if backTarget != nil {
			return backTarget
		}
	case tui.NavHome:
		return prompt.ErrBackToDatabaseSelection
	case tui.NavExit:
		return prompt.ErrExitRequested
	}
	return nil
}

func (w *BlastWizard) showSelection(ctx context.Context, candidate model.SpeciesCandidate) error {
	lines := []string{
		"Selected species",
		"",
		"Label: " + candidate.GenomeLabel,
	}
	if candidate.CommonName != "" {
		lines = append(lines, "Common name: "+candidate.CommonName)
	}
	lines = append(lines, "JBrowse name: "+candidate.JBrowseName)
	if candidate.ProteomeID != 0 {
		lines = append(lines, fmt.Sprintf("Target ID: %d", candidate.ProteomeID))
	}
	if candidate.ReleaseDate != "" {
		lines = append(lines, "Release date: "+candidate.ReleaseDate)
	}

	if c, ok := w.source.(*lemna.Client); ok {
		cap, err := c.DetectBlastCapabilities(ctx, candidate)
		lines = append(lines, "", "lemna.org capability summary")
		if err != nil {
			lines = append(lines, fmt.Sprintf("Could not detect capabilities: %v", err))
		} else {
			progs := c.AvailableBlastPrograms(ctx, candidate)
			if len(progs) > 0 {
				lines = append(lines, "Available programs: "+strings.Join(progs, ", "))
			} else {
				lines = append(lines, "Available programs: none detected")
			}

			if cap.ServerBlastNAvailable {
				lines = append(lines, fmt.Sprintf("Server BLASTn: available (DB id %d)", cap.BlastNDBID))
			} else {
				lines = append(lines, "Server BLASTn: unavailable or no DB id exposed")
			}
			if cap.ServerTBlastNAvailable {
				lines = append(lines, fmt.Sprintf("Server TBLASTN: available (DB id %d)", cap.BlastNDBID))
			} else {
				lines = append(lines, "Server TBLASTN: unavailable")
			}
			if cap.HasNucleotideFasta {
				lines = append(lines, "Nucleotide FASTA: "+cap.NucleotideFastaURL)
			}

			if cap.ServerBlastXAvailable {
				lines = append(lines, fmt.Sprintf("Server BLASTX: available (DB id %d)", cap.ProteinDBID))
			} else {
				lines = append(lines, "Server BLASTX: unavailable")
			}
			if cap.ServerBlastPAvailable {
				lines = append(lines, fmt.Sprintf("Server BLASTP: available (DB id %d)", cap.ProteinDBID))
			} else {
				lines = append(lines, "Server BLASTP: unavailable")
			}
			if cap.HasProteinFasta {
				lines = append(lines, "Protein FASTA: "+cap.ProteinFastaURL)
			} else {
				lines = append(lines, "Protein FASTA: unavailable")
			}
		}
	}

	return w.showInfo("Selected species", strings.Join(lines, "\n"), prompt.ErrBackToSpeciesSelection)
}

func (w *BlastWizard) showBlastResults(results model.BlastResult) error {
	if len(results.Rows) > 0 {
		return nil
	}
	lines := []string{"No BLAST hits returned."}
	if message := strings.TrimSpace(results.Message); message != "" {
		lines = append(lines, "", "Message: "+message)
	}
	return w.showInfo("BLAST results", strings.Join(lines, "\n"), prompt.ErrBackToQueryInput)
}

func buildBlastRequest(species model.SpeciesCandidate, sequence string) model.BlastRequest {
	kind := detectSequenceKind(sequence)
	normalizedSequence := normalizeBlastSequence(sequence, kind)
	request := model.BlastRequest{
		Species:          species,
		Sequence:         normalizedSequence,
		SequenceKind:     kind,
		TargetType:       "genome",
		Program:          "BLASTN",
		EValue:           "-1",
		ComparisonMatrix: "BLOSUM62",
		WordLength:       "default",
		AlignmentsToShow: 100,
		AllowGaps:        true,
		FilterQuery:      true,
	}
	if kind == model.SequenceProtein {
		request.TargetType = "proteome"
		request.Program = "BLASTP"
	}
	return request
}

func parseKeywordTerms(input string) []string {
	return strings.Fields(strings.TrimSpace(strings.ReplaceAll(input, "\r", "")))
}

func countKeywordRows(groups []model.KeywordSearchGroup) int {
	total := 0
	for _, group := range groups {
		total += len(group.Rows)
	}
	return total
}

type keywordLabelIdentification struct {
	TaskTimestamp string
	ItemIndex     int
	Aliases       []string
	SourceType    string
}

func autoIdentifyKeywordLabels(groups []model.KeywordSearchGroup) []string {
	return keywordIdentificationLabels(autoIdentifyKeywordLabelIdentifications(groups))
}

func autoIdentifyKeywordLabelIdentifications(groups []model.KeywordSearchGroup) []keywordLabelIdentification {
	taskTimestamp := keywordLabelTaskTimestamp(groups)
	requests := make([]labelname.AliasRankRequest, len(groups))
	for i, group := range groups {
		requests[i] = labelname.AliasRankRequest{
			TaskTimestamp: taskTimestamp,
			ItemIndex:     i,
			SearchTerm:    group.SearchTerm,
			Aliases:       collectKeywordGroupAliasCandidates(group),
		}
	}
	results := labelname.RankAliasBatch(requests)
	identifications := make([]keywordLabelIdentification, len(results))
	for i, result := range results {
		identifications[i] = keywordLabelIdentification{
			TaskTimestamp: result.TaskTimestamp,
			ItemIndex:     result.ItemIndex,
			Aliases:       result.RankedAliases,
		}
	}
	return identifications
}

func (w *BlastWizard) autoIdentifyKeywordLabelsWithProgress(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup) ([]keywordLabelIdentification, error) {
	return tui.RunTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Keyword", "Auto identify"),
		Title:       "Auto identifying label names",
		Description: "Inferring keyword label names from result rows.",
		Initial:     "Auto identifying label names...",
		CancelError: prompt.ErrBackToQueryInput,
	}, func(taskCtx context.Context, update func(string)) ([]keywordLabelIdentification, error) {
		taskUpdate := safeTaskUpdate(update)
		labelCtx := mergeContexts(ctx, taskCtx)
		taskUpdate("Reviewing keyword result rows...")
		working := cloneKeywordSearchGroups(groups)
		if _, ok := w.source.(*lemna.Client); ok {
			taskUpdate("Searching Phytozome label candidates for Lemna rows...")
			return w.autoIdentifyLemnaKeywordLabelsWithProgress(labelCtx, selected, working), nil
		}
		taskUpdate("Selecting label names...")
		return autoIdentifyKeywordLabelIdentifications(working), nil
	})
}

func (w *BlastWizard) autoIdentifyLemnaKeywordLabelsWithProgress(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup) []keywordLabelIdentification {
	return w.autoIdentifyLemnaKeywordLabels(ctx, selected, groups, phytozome.NewClient(w.httpClient))
}

func (w *BlastWizard) autoIdentifyLemnaKeywordLabels(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup, lookupSource source.DataSource) []keywordLabelIdentification {
	taskTimestamp := keywordLabelTaskTimestamp(groups)
	identifications := make([]keywordLabelIdentification, len(groups))
	for i := range identifications {
		identifications[i].TaskTimestamp = taskTimestamp
		identifications[i].ItemIndex = i
	}
	if lookupSource == nil {
		for i, group := range groups {
			aliases, sourceType := lemnaKeywordGroupAliasCandidates(group, nil)
			ranked := labelname.RankAliases(labelname.AliasRankRequest{
				TaskTimestamp: taskTimestamp,
				ItemIndex:     i,
				SearchTerm:    group.SearchTerm,
				Aliases:       aliases,
			})
			identifications[i].Aliases = ranked.RankedAliases
			identifications[i].SourceType = sourceType
		}
		return identifications
	}
	phytozomeSpecies, ok := w.phytozomeSpeciesForLemnaLabels(ctx, selected, lookupSource)
	keywordRowsByTerm := map[string][]model.KeywordResultRow{}
	if ok {
		terms := lemnaKeywordGroupsPhytozomeSearchTerms(groups)
		keywordRowsByTerm = w.fetchKeywordRowsByTerms(ctx, lookupSource, phytozomeSpecies, terms)
	}
	requests := make([]labelname.AliasRankRequest, len(groups))
	sourceTypes := make([]string, len(groups))
	for i, group := range groups {
		aliases, sourceType := lemnaKeywordGroupAliasCandidates(group, keywordRowsByTerm)
		requests[i] = labelname.AliasRankRequest{
			TaskTimestamp: taskTimestamp,
			ItemIndex:     i,
			SearchTerm:    group.SearchTerm,
			Aliases:       aliases,
		}
		sourceTypes[i] = sourceType
	}
	results := labelname.RankAliasBatch(requests)
	for i, ranked := range results {
		identifications[i].Aliases = ranked.RankedAliases
		identifications[i].SourceType = sourceTypes[i]
	}
	return identifications
}

func (w *BlastWizard) phytozomeSpeciesForLemnaLabels(ctx context.Context, selected model.SpeciesCandidate, lookupSource source.DataSource) (model.SpeciesCandidate, bool) {
	if lookupSource == nil {
		return model.SpeciesCandidate{}, false
	}
	candidates, err := w.speciesCandidatesForSource(ctx, lookupSource, nil)
	if err != nil {
		return model.SpeciesCandidate{}, false
	}
	return matchPhytozomeSpeciesForLemna(selected, candidates)
}

func lemnaKeywordGroupsPhytozomeSearchTerms(groups []model.KeywordSearchGroup) []string {
	terms := make([]string, 0, len(groups)*2)
	for _, group := range groups {
		for _, row := range group.Rows {
			terms = append(terms, lemnaKeywordRowPhytozomeSearchTerms(row)...)
		}
	}
	return uniqueStrings(terms)
}

func lemnaKeywordRowPhytozomeSearchTerms(row model.KeywordResultRow) []string {
	terms := make([]string, 0, 8)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			terms = append(terms, value)
		}
	}
	add(row.ProteinID)
	add(row.SequenceID)
	add(row.TranscriptID)
	add(row.GeneIdentifier)
	add(stripTranscriptSuffix(firstNonEmpty(row.TranscriptID, row.SequenceID, row.ProteinID, row.GeneIdentifier)))
	if row.ExtraColumns != nil {
		for _, key := range []string{"attr_ID", "attr_Name", "attr_Parent", "attr_protein_id", "attr_protein", "attr_protein_accession", "ahrd_protein_accession", "ahrd_blast_hit_accession"} {
			add(row.ExtraColumns[key])
		}
	}
	return uniqueStrings(terms)
}

func (w *BlastWizard) fetchKeywordRowsByTerms(ctx context.Context, lookupSource source.DataSource, selected model.SpeciesCandidate, terms []string) map[string][]model.KeywordResultRow {
	terms = uniqueStrings(terms)
	results := make(map[string][]model.KeywordResultRow, len(terms))
	if len(terms) == 0 || lookupSource == nil {
		return results
	}
	pendingTerms := make([]string, 0, len(terms))
	for _, term := range terms {
		cacheKey := w.keywordTermRowsCacheKey(lookupSource, selected, term)
		if rows, ok := w.cachedKeywordTermRows(cacheKey); ok {
			results[strings.ToLower(strings.TrimSpace(term))] = rows
			continue
		}
		pendingTerms = append(pendingTerms, term)
	}
	if len(pendingTerms) == 0 {
		return results
	}
	workerCount := blastKeywordTermWorkerCount(len(pendingTerms))
	jobs := make(chan string)
	type lookupResult struct {
		term string
		rows []model.KeywordResultRow
	}
	outcomes := make(chan lookupResult, len(pendingTerms))
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for term := range jobs {
				cacheKey := w.keywordTermRowsCacheKey(lookupSource, selected, term)
				value, err, _ := w.keywordTermRowsGroup.Do(cacheKey, func() (any, error) {
					if rows, ok := w.cachedKeywordTermRows(cacheKey); ok {
						return rows, nil
					}
					rows, err := lookupSource.SearchKeywordRows(ctx, selected, term)
					if err != nil {
						return nil, err
					}
					w.storeKeywordTermRows(cacheKey, rows)
					return cloneKeywordResultRows(rows), nil
				})
				if err != nil {
					outcomes <- lookupResult{term: term}
					continue
				}
				outcomes <- lookupResult{term: term, rows: value.([]model.KeywordResultRow)}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, term := range pendingTerms {
			select {
			case <-ctx.Done():
				return
			case jobs <- term:
			}
		}
	}()
	completed := 0
	for completed < len(pendingTerms) {
		select {
		case <-ctx.Done():
			workers.Wait()
			return results
		case result := <-outcomes:
			key := strings.ToLower(strings.TrimSpace(result.term))
			results[key] = result.rows
			completed++
		}
	}
	workers.Wait()
	return results
}

func (w *BlastWizard) keywordTermRowsCacheKey(src source.DataSource, selected model.SpeciesCandidate, term string) string {
	sourceName := ""
	if src != nil {
		sourceName = src.Name()
	}
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(sourceName)),
		strconv.Itoa(selected.ProteomeID),
		strings.ToLower(strings.TrimSpace(selected.JBrowseName)),
		strings.ToLower(strings.TrimSpace(term)),
	}, "|")
}

func cloneKeywordResultRows(rows []model.KeywordResultRow) []model.KeywordResultRow {
	out := append([]model.KeywordResultRow(nil), rows...)
	for i := range out {
		if out[i].ExtraColumns != nil {
			extra := make(map[string]string, len(out[i].ExtraColumns))
			for k, v := range out[i].ExtraColumns {
				extra[k] = v
			}
			out[i].ExtraColumns = extra
		}
	}
	return out
}

func (w *BlastWizard) cachedKeywordTermRows(cacheKey string) ([]model.KeywordResultRow, bool) {
	if strings.TrimSpace(cacheKey) == "" {
		return nil, false
	}
	w.keywordTermRowsMu.RLock()
	rows, ok := w.keywordTermRowsCache[cacheKey]
	w.keywordTermRowsMu.RUnlock()
	if !ok {
		return nil, false
	}
	return cloneKeywordResultRows(rows), true
}

func (w *BlastWizard) storeKeywordTermRows(cacheKey string, rows []model.KeywordResultRow) {
	if strings.TrimSpace(cacheKey) == "" {
		return
	}
	w.keywordTermRowsMu.Lock()
	if w.keywordTermRowsCache == nil {
		w.keywordTermRowsCache = make(map[string][]model.KeywordResultRow)
	}
	w.keywordTermRowsCache[cacheKey] = cloneKeywordResultRows(rows)
	w.keywordTermRowsMu.Unlock()
}

func lemnaKeywordGroupAliasCandidates(group model.KeywordSearchGroup, keywordRowsByTerm map[string][]model.KeywordResultRow) ([]string, string) {
	aliases := make([]string, 0, len(group.Rows)*8+1)
	aliases = append(aliases, group.LabelName)
	sourceType := ""
	for _, row := range group.Rows {
		rowAliases, rowSource := lemnaKeywordRowAliasCandidates(row, keywordRowsByTerm)
		if sourceType == "" && rowSource != "" {
			sourceType = rowSource
		}
		aliases = append(aliases, rowAliases...)
	}
	return uniqueStrings(aliases), sourceType
}

func lemnaKeywordRowAliasCandidates(row model.KeywordResultRow, keywordRowsByTerm map[string][]model.KeywordResultRow) ([]string, string) {
	if keywordRowsByTerm != nil {
		for _, term := range lemnaKeywordRowPhytozomeSearchTerms(row) {
			rows := keywordRowsByTerm[strings.ToLower(strings.TrimSpace(term))]
			for _, candidateRows := range [][]model.KeywordResultRow{
				filterKeywordRowsForLemnaKeyword(rows, row),
				rows,
			} {
				if candidates, labelType := phytozomeAliasCandidatesFromKeywordRows(candidateRows); len(candidates) > 0 {
					return candidates, labelType
				}
			}
		}
	}
	if candidates := lemnaLocalKeywordRowAliasCandidates(row); len(candidates) > 0 {
		return candidates, "lemna local aliases"
	}
	return nil, ""
}

func filterKeywordRowsForLemnaKeyword(rows []model.KeywordResultRow, row model.KeywordResultRow) []model.KeywordResultRow {
	if len(rows) == 0 {
		return nil
	}
	targets := make([]string, 0, 6)
	for _, value := range lemnaKeywordRowPhytozomeSearchTerms(row) {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" {
			targets = append(targets, value)
		}
	}
	if len(targets) == 0 {
		return rows
	}
	matches := make([]model.KeywordResultRow, 0, len(rows))
	for _, candidate := range rows {
		haystack := strings.ToLower(strings.Join([]string{
			candidate.ProteinID,
			candidate.TranscriptID,
			candidate.SequenceID,
			candidate.GeneIdentifier,
		}, " "))
		for _, target := range targets {
			if strings.Contains(haystack, target) {
				matches = append(matches, candidate)
				break
			}
		}
	}
	return matches
}

func keywordLabelTaskTimestamp(groups []model.KeywordSearchGroup) string {
	latest := keywordGroupsSearchEndedAt(groups)
	if latest.IsZero() {
		latest = time.Now()
	}
	return latest.UTC().Format(time.RFC3339Nano)
}

func collectKeywordGroupAliasCandidates(group model.KeywordSearchGroup) []string {
	aliases := make([]string, 0, len(group.Rows)*8+2)
	aliases = append(aliases, group.LabelName)
	for _, row := range group.Rows {
		aliases = append(aliases, keywordRowLabelnameCandidates(row)...)
	}
	return uniqueStrings(aliases)
}

func keywordRowLabelnameCandidates(row model.KeywordResultRow) []string {
	if strings.EqualFold(strings.TrimSpace(row.SourceDatabase), "phytozome") {
		if candidates, _ := phytozomeAliasCandidatesFromKeywordRows([]model.KeywordResultRow{row}); len(candidates) > 0 {
			return candidates
		}
		return phytozomeKeywordFallbackAliasCandidates(row)
	}
	return lemnaLocalKeywordRowAliasCandidates(row)
}

func phytozomeKeywordFallbackAliasCandidates(row model.KeywordResultRow) []string {
	aliases := make([]string, 0, 12)
	aliases = append(aliases, row.LabelName)
	aliases = append(aliases, labelname.SplitAliases(row.PhgoAliases)...)
	aliases = append(aliases, labelname.SplitAliases(row.Aliases)...)
	aliases = append(aliases, labelname.SplitAliases(row.UniProt)...)
	aliases = append(aliases, phytozomeUniProtAliasCandidates(row.UniProt)...)
	aliases = append(aliases, labelname.AutoDefineCandidates(row.Description)...)
	aliases = append(aliases, labelname.AutoDefineCandidates(row.Comments)...)
	if row.ExtraColumns != nil {
		for _, key := range []string{"attr_Alias", "attr_Name", "attr_gene_name", "attr_gene_symbol"} {
			aliases = append(aliases, labelname.SplitAliases(row.ExtraColumns[key])...)
		}
	}
	return uniqueStrings(aliases)
}

func phytozomeUniProtAliasCandidates(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ';', ',', '|', '\t', '\n', '\r':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if idx := strings.LastIndex(part, ":"); idx >= 0 {
			part = strings.TrimSpace(part[idx+1:])
		}
		part = strings.TrimSpace(strings.TrimSuffix(part, "_ORYSJ"))
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return uniqueStrings(out)
}

type lemnaLocalAliasSeed struct {
	LabelName    string
	PhgoAliases  string
	Aliases      string
	AutoDefine   string
	Description  string
	Comments     string
	UniProt      string
	ExtraColumns map[string]string
}

func lemnaLocalAliasCandidates(seed lemnaLocalAliasSeed) []string {
	aliases := make([]string, 0, 18)
	aliases = append(aliases, seed.LabelName)
	aliases = append(aliases, labelname.SplitAliases(seed.PhgoAliases)...)
	aliases = append(aliases, splitLocalAliasText(seed.Aliases)...)
	aliases = append(aliases, splitLocalAliasText(seed.UniProt)...)
	if seed.ExtraColumns != nil {
		for _, key := range []string{
			"attr_Alias",
			"attr_alias",
			"attr_Name",
			"attr_gene_name",
			"attr_gene_symbol",
			"attr_symbol",
			"attr_gene",
			"ahrd_blast_hit_accession",
		} {
			aliases = append(aliases, splitLocalAliasText(seed.ExtraColumns[key])...)
		}
	}
	if aliases = uniqueStrings(aliases); len(aliases) > 0 {
		return aliases
	}
	autoDefine := labelname.AutoDefineCandidates(seed.AutoDefine)
	if len(autoDefine) == 0 {
		autoDefine = append(autoDefine, labelname.AutoDefineCandidates(seed.Description)...)
		autoDefine = append(autoDefine, labelname.AutoDefineCandidates(seed.Comments)...)
	}
	return uniqueStrings(autoDefine)
}

func splitLocalAliasText(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ';', ',', '|', '\t', '\n', '\r', ':':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func lemnaLocalKeywordRowAliasCandidates(row model.KeywordResultRow) []string {
	return lemnaLocalAliasCandidates(lemnaLocalAliasSeed{
		LabelName:    row.LabelName,
		PhgoAliases:  row.PhgoAliases,
		Aliases:      row.Aliases,
		AutoDefine:   row.AutoDefine,
		Description:  row.Description,
		Comments:     row.Comments,
		UniProt:      row.UniProt,
		ExtraColumns: row.ExtraColumns,
	})
}

func lemnaLocalQuerySourceAliasCandidates(source *model.QuerySequenceSource) []string {
	if source == nil {
		return nil
	}
	return lemnaLocalAliasCandidates(lemnaLocalAliasSeed{
		LabelName:   source.LabelName,
		PhgoAliases: source.PhgoAliases,
		Aliases:     source.Aliases,
		AutoDefine:  source.AutoDefine,
	})
}

func keywordAliasesFromRows(rows []model.KeywordResultRow) []string {
	aliases := make([]string, 0, len(rows)*6)
	for _, row := range rows {
		aliases = append(aliases, collectKeywordGroupAliasCandidates(model.KeywordSearchGroup{Rows: []model.KeywordResultRow{row}})...)
	}
	return uniqueStrings(aliases)
}

func bestKeywordRowLabel(rows []model.KeywordResultRow) string {
	identifications := autoIdentifyKeywordLabelIdentifications([]model.KeywordSearchGroup{{Rows: rows}})
	if len(identifications) == 0 || len(identifications[0].Aliases) == 0 {
		return ""
	}
	return identifications[0].Aliases[0]
}

func matchPhytozomeSpeciesForLemna(lemnaSpecies model.SpeciesCandidate, candidates []model.SpeciesCandidate) (model.SpeciesCandidate, bool) {
	lemnaName := normalizedScientificName(lemnaSpecies)
	if lemnaName == "" {
		return model.SpeciesCandidate{}, false
	}
	var matched model.SpeciesCandidate
	matches := 0
	for _, candidate := range candidates {
		if normalizedScientificName(candidate) == lemnaName {
			matched = candidate
			matches++
		}
	}
	return matched, matches == 1
}

func normalizedScientificName(candidate model.SpeciesCandidate) string {
	text := strings.TrimSpace(candidate.SearchAlias)
	if text == "" {
		text = strings.TrimSpace(candidate.GenomeLabel)
	}
	text = strings.ReplaceAll(text, "_", " ")
	text = strings.ReplaceAll(text, ".", " ")
	fields := strings.Fields(text)
	if len(fields) < 2 {
		return ""
	}
	return strings.ToLower(fields[0] + " " + fields[1])
}

func matchPhytozomeSpeciesForFastaHeader(headerSpecies string, candidates []model.SpeciesCandidate) (model.SpeciesCandidate, bool) {
	name := normalizedFastaHeaderSpeciesName(headerSpecies)
	if name == "" {
		return model.SpeciesCandidate{}, false
	}
	var matched model.SpeciesCandidate
	matches := 0
	for _, candidate := range candidates {
		if normalizedScientificName(candidate) == name {
			matched = candidate
			matches++
		}
	}
	return matched, matches == 1
}

func normalizedFastaHeaderSpeciesName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, ".", " ")
	fields := strings.Fields(value)
	if len(fields) < 2 {
		return ""
	}
	return fields[0] + " " + fields[1]
}

func applyKeywordIdentifications(groups []model.KeywordSearchGroup, identifications []string) {
	applyKeywordLabelIdentifications(groups, manualKeywordLabelIdentifications(identifications, len(groups)))
}

func applyKeywordLabelIdentifications(groups []model.KeywordSearchGroup, identifications []keywordLabelIdentification) {
	if len(groups) != len(identifications) {
		return
	}
	for i := range groups {
		labelType := strings.TrimSpace(groups[i].LabelSourceField)
		aliases := uniqueStrings(identifications[i].Aliases)
		label := ""
		if len(aliases) > 0 {
			label = strings.TrimSpace(aliases[0])
		}
		aliasText := strings.Join(aliases, "; ")
		groups[i].LabelName = label
		for r := range groups[i].Rows {
			groups[i].Rows[r].LabelName = label
			groups[i].Rows[r].LabelNameType = labelType
			groups[i].Rows[r].PhgoAliases = aliasText
		}
	}
}

func manualKeywordLabelIdentifications(labels []string, total int) []keywordLabelIdentification {
	out := make([]keywordLabelIdentification, total)
	taskTimestamp := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range out {
		out[i].TaskTimestamp = taskTimestamp
		out[i].ItemIndex = i
		if i < len(labels) {
			label := strings.TrimSpace(labels[i])
			if label != "" {
				out[i].Aliases = []string{label}
			}
		}
	}
	return out
}

func keywordIdentificationLabels(identifications []keywordLabelIdentification) []string {
	labels := make([]string, len(identifications))
	for i, identification := range identifications {
		if len(identification.Aliases) > 0 {
			labels[i] = strings.TrimSpace(identification.Aliases[0])
		}
	}
	return labels
}

func applyKeywordLabelMethod(groups []model.KeywordSearchGroup, method string) {
	method = strings.TrimSpace(method)
	for i := range groups {
		groups[i].LabelMethod = method
	}
}

func annotateKeywordLabelSources(groups []model.KeywordSearchGroup, identifications []keywordLabelIdentification, method string) {
	if len(groups) != len(identifications) {
		return
	}
	for i := range groups {
		label := ""
		if len(identifications[i].Aliases) > 0 {
			label = strings.TrimSpace(identifications[i].Aliases[0])
		}
		if strings.Contains(strings.ToLower(method), "manual") {
			groups[i].LabelSourceField = "user input"
			groups[i].LabelSourceValue = firstNonEmpty(label, "blank label intentionally allowed")
			for r := range groups[i].Rows {
				groups[i].Rows[r].LabelNameType = groups[i].LabelSourceField
			}
			continue
		}
		if sourceType := strings.TrimSpace(identifications[i].SourceType); sourceType != "" {
			groups[i].LabelSourceField = sourceType
			groups[i].LabelSourceValue = firstNonEmpty(label, sourceType)
			for r := range groups[i].Rows {
				groups[i].Rows[r].LabelNameType = sourceType
			}
			continue
		}
		field, value := inferKeywordAutoLabelSource(groups[i], label)
		groups[i].LabelSourceField = field
		groups[i].LabelSourceValue = value
		for r := range groups[i].Rows {
			groups[i].Rows[r].LabelNameType = field
		}
	}
}

func inferKeywordAutoLabelSource(group model.KeywordSearchGroup, label string) (string, string) {
	label = strings.TrimSpace(label)
	for _, row := range group.Rows {
		if rowLabel := strings.TrimSpace(row.LabelName); rowLabel != "" && (label == "" || rowLabel == label) {
			return "row label_name", rowLabel
		}
	}
	for _, row := range group.Rows {
		for _, alias := range labelname.SplitAliases(row.PhgoAliases) {
			if alias != "" && (label == "" || alias == label) {
				return "best phgo alias candidate", alias
			}
		}
		for _, alias := range keywordRowLabelnameCandidates(row) {
			if alias != "" && (label == "" || alias == label) {
				return "source alias candidate", alias
			}
		}
	}
	for _, row := range group.Rows {
		if id := firstNonEmpty(row.GeneIdentifier, row.TranscriptID, row.SequenceID); id != "" && (label == "" || id == label) {
			return "gene/transcript/sequence identifier", id
		}
	}
	if label != "" {
		return "auto-identify result", label
	}
	return "not available in this run", "not available in this run"
}

func keywordGroupsSearchEndedAt(groups []model.KeywordSearchGroup) time.Time {
	var latest time.Time
	for _, group := range groups {
		if group.SearchEndedAt.After(latest) {
			latest = group.SearchEndedAt
		}
	}
	return latest
}

func rowKeywordLabelName(row model.KeywordResultRow) string {
	return strings.TrimSpace(row.LabelName)
}

func defaultKeywordExportLabel(rows []model.KeywordResultRow, groups []model.KeywordSearchGroup) string {
	label := ""
	for _, row := range rows {
		rowLabel := rowKeywordLabelName(row)
		if rowLabel == "" {
			continue
		}
		if label == "" {
			label = rowLabel
			continue
		}
		if label != rowLabel {
			return "keyword"
		}
	}
	if label != "" {
		return label
	}
	for _, group := range groups {
		groupLabel := strings.TrimSpace(group.LabelName)
		if groupLabel == "" {
			continue
		}
		if label == "" {
			label = groupLabel
			continue
		}
		if label != groupLabel {
			return "keyword"
		}
	}
	if label != "" {
		return label
	}
	return "keyword"
}

func keywordSearchTermLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return ""
	}
	if strings.ContainsAny(value, "/\\:;,.()[]{}") {
		return ""
	}
	if len(value) > 15 {
		return ""
	}
	hasLetter := false
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
			hasLetter = true
		case r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return ""
		}
	}
	if !hasLetter {
		return ""
	}
	return value
}

func firstFastaHeaderLine(input string) string {
	value := strings.TrimSpace(input)
	if value == "" || !strings.HasPrefix(value, ">") {
		return ""
	}
	value = strings.ReplaceAll(value, "\r", "")
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ">") {
			return strings.TrimSpace(strings.TrimPrefix(line, ">"))
		}
		return ""
	}
	return ""
}

func detectSequenceKind(sequence string) model.SequenceKind {
	cleaned := sanitizeSequence(sequence)
	if cleaned == "" {
		return model.SequenceDNA
	}

	dnaChars := 0
	proteinOnlyChars := 0
	for _, ch := range cleaned {
		switch ch {
		case 'A', 'C', 'G', 'T', 'U', 'N':
			dnaChars++
		case 'E', 'F', 'I', 'L', 'P', 'Q', 'X', '*', 'R', 'D', 'H', 'K', 'M', 'S', 'V', 'W', 'Y':
			proteinOnlyChars++
		}
	}

	if proteinOnlyChars > 0 && float64(dnaChars)/float64(len(cleaned)) < 0.9 {
		return model.SequenceProtein
	}
	return model.SequenceDNA
}

func sanitizeSequence(sequence string) string {
	lines := strings.Split(sequence, "\n")
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ">") {
			continue
		}
		parts = append(parts, line)
	}

	cleaned := strings.ToUpper(strings.Join(parts, ""))
	cleaned = strings.ReplaceAll(cleaned, "\r", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "*", "")
	return cleaned
}

func normalizeBlastSequence(sequence string, kind model.SequenceKind) string {
	cleaned := sanitizeSequence(sequence)
	if kind == model.SequenceProtein {
		cleaned = strings.ReplaceAll(cleaned, "*", "")
	}
	return cleaned
}

func (w *BlastWizard) exportSelections(ctx context.Context, rows []model.BlastResultRow, allRows []model.BlastResultRow, querySource *model.QuerySequenceSource, baseName string, settings exportSettings) error {
	outputDir, err := appfs.OutputDir()
	if err != nil {
		return err
	}
	_, err = w.exportSelectedBlastFiles(ctx, rows, allRows, nil, nil, querySource, baseName, outputDir, settings, true)
	return err
}

func (w *BlastWizard) exportKeywordSelections(ctx context.Context, selected model.SpeciesCandidate, rows []model.KeywordResultRow, allRows []model.KeywordResultRow, groups []model.KeywordSearchGroup, baseName string, outputDir string, settings exportSettings, reportCtx *keywordReportRunContext) error {
	return w.exportSelectedKeywordFiles(ctx, selected, rows, allRows, groups, baseName, outputDir, settings, reportCtx, true)
}

func (w *BlastWizard) exportSelectedBlastFiles(ctx context.Context, rows []model.BlastResultRow, allRows []model.BlastResultRow, rowNumbers []int, filterFlags []bool, querySource *model.QuerySequenceSource, baseName string, outputDir string, settings exportSettings, showComplete bool) (exportFileResult, error) {
	return w.exportBlastSelectionsToDir(ctx, rows, allRows, rowNumbers, filterFlags, querySource, baseName, baseName, sanitizeExportName(baseName), outputDir, settings, showComplete)
}

func (w *BlastWizard) exportSelectedKeywordFiles(ctx context.Context, selected model.SpeciesCandidate, rows []model.KeywordResultRow, allRows []model.KeywordResultRow, groups []model.KeywordSearchGroup, baseName string, outputDir string, settings exportSettings, reportCtx *keywordReportRunContext, showComplete bool) error {
	files := exportFileResult{}
	exportStarted := time.Now()
	steps := make([]report.GenerationStep, 0, 8)
	var selectedTextRecords []model.ProteinSequenceRecord
	selectedTextReady := false
	if settings.WriteExcel {
		excelPath := filepath.Join(outputDir, baseName+".xlsx")
		stepStart := time.Now()
		if settings.WriteText {
			records, err := w.exportKeywordExcelAndFetchRecords(ctx, selected, rows, excelPath)
			if err != nil {
				steps = append(steps, keywordReportStep("Write selected Excel and fetch peptide sequences", stepStart, time.Now(), "failed", err.Error()))
				return err
			}
			selectedTextRecords = records
			selectedTextReady = true
			steps = append(steps, keywordReportStep("Write selected Excel and fetch peptide sequences", stepStart, time.Now(), "ok", fmt.Sprintf("%d selected rows written; %d peptide records available", len(rows), len(records))))
		} else {
			if err := withSpinner(w.out, "Writing selected keyword Excel file...", func() error {
				return export.WriteKeywordResultsExcel(excelPath, rows)
			}); err != nil {
				steps = append(steps, keywordReportStep("Write selected Excel", stepStart, time.Now(), "failed", err.Error()))
				return err
			}
			steps = append(steps, keywordReportStep("Write selected Excel", stepStart, time.Now(), "ok", fmt.Sprintf("%d selected rows written", len(rows))))
		}
		files.ExcelPath = excelPath
	}
	if settings.WriteRawExcel && settings.WriteText {
		rawPath := filepath.Join(outputDir, baseName+"_raw.xlsx")
		rawTextPath := filepath.Join(outputDir, baseName+"_raw.fasta")
		rawExcelSteps, rawTextSteps, err := runParallelExportSteps(
			func() ([]report.GenerationStep, error) {
				stepStart := time.Now()
				if err := export.WriteKeywordResultsExcel(rawPath, allRows); err != nil {
					return []report.GenerationStep{keywordReportStep("Write raw Excel", stepStart, time.Now(), "failed", err.Error())}, err
				}
				return []report.GenerationStep{keywordReportStep("Write raw Excel", stepStart, time.Now(), "ok", fmt.Sprintf("%d current rows written", len(allRows)))}, nil
			},
			func() ([]report.GenerationStep, error) {
				steps := make([]report.GenerationStep, 0, 2)
				fetchStart := time.Now()
				var (
					rawRecords []model.ProteinSequenceRecord
					err        error
				)
				if w.suppressTaskModals {
					rawRecords, err = w.fetchKeywordProteinSequenceRecordsWithProgress(ctx, selected, allRows, nil)
				} else {
					rawRecords, err = w.fetchKeywordProteinSequenceRecords(ctx, selected, allRows)
				}
				if err != nil {
					return append(steps, keywordReportStep("Fetch/use raw peptide sequences", fetchStart, time.Now(), "failed", err.Error())), err
				}
				steps = append(steps, keywordReportStep("Fetch/use raw peptide sequences", fetchStart, time.Now(), "ok", fmt.Sprintf("%d sequence records available", len(rawRecords))))
				if settings.UsePhgoHeader {
					rawRecords = applyKeywordPhgoHeaders(rawRecords, allRows)
				} else {
					rawRecords = applyOriginalHeaders(rawRecords)
				}
				writeStart := time.Now()
				if err := export.WriteProteinSequencesText(rawTextPath, rawRecords); err != nil {
					return append(steps, keywordReportStep("Write raw peptide text", writeStart, time.Now(), "failed", err.Error())), err
				}
				return append(steps, keywordReportStep("Write raw peptide text", writeStart, time.Now(), "ok", fmt.Sprintf("%d peptide records written", len(rawRecords)))), nil
			},
			w.out,
			false,
			"Writing raw keyword export files...",
		)
		if err != nil {
			steps = append(steps, rawExcelSteps...)
			steps = append(steps, rawTextSteps...)
			return err
		}
		steps = append(steps, rawExcelSteps...)
		steps = append(steps, rawTextSteps...)
		files.RawExcelPath = rawPath
		files.RawTextPath = rawTextPath
	} else if settings.WriteRawExcel {
		rawPath := filepath.Join(outputDir, baseName+"_raw.xlsx")
		stepStart := time.Now()
		if err := withSpinner(w.out, "Writing raw keyword Excel file...", func() error {
			return export.WriteKeywordResultsExcel(rawPath, allRows)
		}); err != nil {
			steps = append(steps, keywordReportStep("Write raw Excel", stepStart, time.Now(), "failed", err.Error()))
			return err
		}
		steps = append(steps, keywordReportStep("Write raw Excel", stepStart, time.Now(), "ok", fmt.Sprintf("%d current rows written", len(allRows))))
		files.RawExcelPath = rawPath
	}
	if settings.WriteText && !settings.WriteRawExcel {
		preloadStart := time.Now()
		w.prefetchKeywordSequences(ctx, selected, rows, nil)
		steps = append(steps, keywordReportStep("Preload keyword peptide sequences", preloadStart, time.Now(), "ok", fmt.Sprintf("%d keyword rows checked before writing text files", len(rows))))
	}
	var sequenceRecords []model.ProteinSequenceRecord
	var sequenceAudit report.SequenceAudit
	if settings.WriteText {
		textPath := filepath.Join(outputDir, baseName+".fasta")
		records := selectedTextRecords
		if !selectedTextReady {
			fetchStart := time.Now()
			var err error
			if w.suppressTaskModals {
				records, err = w.fetchKeywordProteinSequenceRecordsWithProgress(ctx, selected, rows, nil)
			} else {
				records, err = w.fetchKeywordProteinSequenceRecords(ctx, selected, rows)
			}
			if err != nil {
				steps = append(steps, keywordReportStep("Fetch/use peptide sequences", fetchStart, time.Now(), "failed", err.Error()))
				return err
			}
			steps = append(steps, keywordReportStep("Fetch/use peptide sequences", fetchStart, time.Now(), "ok", fmt.Sprintf("%d sequence records available", len(records))))
		} else {
			steps = append(steps, keywordReportStep("Reuse prefetched peptide sequences", time.Now(), time.Now(), "ok", fmt.Sprintf("%d sequence records reused from parallel Excel export step", len(records))))
		}
		if settings.UsePhgoHeader {
			records = applyKeywordPhgoHeaders(records, rows)
		} else {
			records = applyOriginalHeaders(records)
		}
		sequenceRecords = records
		sequenceAudit = buildKeywordSequenceAudit(rows, records)
		writeStart := time.Now()
		if err := withSpinner(w.out, "Writing peptide text file...", func() error {
			return export.WriteProteinSequencesText(textPath, records)
		}); err != nil {
			steps = append(steps, keywordReportStep("Write peptide text", writeStart, time.Now(), "failed", err.Error()))
			return err
		}
		steps = append(steps, keywordReportStep("Write peptide text", writeStart, time.Now(), "ok", fmt.Sprintf("%d peptide records written", len(records))))
		files.TextPath = textPath
	} else {
		sequenceAudit = report.SequenceAudit{Requested: false}
	}
	if settings.WriteReport {
		reportPath, err := w.renderKeywordReportForExport(ctx, rows, allRows, groups, files, baseName, outputDir, settings, reportCtx, exportStarted, steps, sequenceAudit, sequenceRecords)
		if err != nil {
			return err
		}
		files.ReportPath = reportPath
	}
	if showComplete {
		return w.showInfo("Export complete", filesSummary(files), prompt.ErrBackToRowSelection)
	}
	return nil
}

func (w *BlastWizard) exportKeywordExcelAndFetchRecords(ctx context.Context, selected model.SpeciesCandidate, rows []model.KeywordResultRow, excelPath string) ([]model.ProteinSequenceRecord, error) {
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Export", "Writing keyword files"),
		Title:       "Writing keyword export files",
		Description: "Writing the keyword Excel file while fetching peptide sequences for the text export.",
		Initial:     "Starting keyword export...",
		Total:       len(rows) + 1,
		CancelError: prompt.ErrBackToRowSelection,
	}, func(taskCtx context.Context, update func(int, string)) ([]model.ProteinSequenceRecord, error) {
		exportCtx := mergeContexts(ctx, taskCtx)
		progress := safeProgress(update)
		type excelResult struct {
			err error
		}
		excelDone := make(chan excelResult, 1)
		go func() {
			excelDone <- excelResult{err: export.WriteKeywordResultsExcel(excelPath, rows)}
		}()
		records, fetchErr := w.fetchKeywordProteinSequenceRecordsWithProgress(exportCtx, selected, rows, func(current int, message string) {
			progress(current, message)
		})
		excel := <-excelDone
		if excel.err != nil {
			return nil, excel.err
		}
		progress(len(rows)+1, "Wrote keyword Excel file and fetched peptide sequences.")
		if fetchErr != nil {
			return nil, fetchErr
		}
		return records, nil
	})
}

func (w *BlastWizard) resolveQuerySequenceInput(ctx context.Context, candidates []model.SpeciesCandidate, input string) (*model.QuerySequenceSource, bool, error) {
	normalizedURL, ok := normalizeGeneReportURL(input)
	if ok {
		return w.resolveURLQuerySequenceInput(ctx, candidates, input, normalizedURL)
	}

	if source, ok := parseFastaQuerySequenceInput(input); ok {
		return source, true, nil
	}

	return nil, false, nil
}

func (w *BlastWizard) resolveQuerySequenceInputBatch(ctx context.Context, candidates []model.SpeciesCandidate, input string) (*model.QuerySequenceSource, bool, error) {
	normalizedURL, ok := normalizeGeneReportURL(input)
	if ok {
		return w.resolveURLQuerySequenceInputBatch(ctx, candidates, input, normalizedURL)
	}

	if source, ok := parseFastaQuerySequenceInput(input); ok {
		return source, true, nil
	}

	return nil, false, nil
}

func (w *BlastWizard) resolveURLQuerySequenceInput(ctx context.Context, candidates []model.SpeciesCandidate, input string, normalizedURL string) (*model.QuerySequenceSource, bool, error) {
	resolverSource, species, reportType, identifier, err := w.resolveGeneReportTarget(ctx, candidates, normalizedURL)
	if err != nil {
		return nil, false, err
	}
	resolveLabel := databaseDisplayName(resolverSource.Name())
	gene, err := withSpinnerValue(w.out, "Resolving "+resolveLabel+" gene report URL...", prompt.ErrBackToQueryInput, func(taskCtx context.Context) (*model.QuerySequenceSource, error) {
		return w.resolveGeneReportSequence(mergeContexts(ctx, taskCtx), resolverSource, species, reportType, identifier, input, normalizedURL)
	})
	if err != nil {
		return nil, false, err
	}
	return gene, true, nil
}

func (w *BlastWizard) resolveURLQuerySequenceInputBatch(ctx context.Context, candidates []model.SpeciesCandidate, input string, normalizedURL string) (*model.QuerySequenceSource, bool, error) {
	resolverSource, species, reportType, identifier, err := w.resolveGeneReportTarget(ctx, candidates, normalizedURL)
	if err != nil {
		return nil, false, err
	}
	gene, err := w.resolveGeneReportSequence(ctx, resolverSource, species, reportType, identifier, input, normalizedURL)
	if err != nil {
		return nil, false, err
	}
	return gene, true, nil
}

func (w *BlastWizard) resolveGeneReportTarget(ctx context.Context, candidates []model.SpeciesCandidate, normalizedURL string) (source.DataSource, model.SpeciesCandidate, string, string, error) {
	jbrowseName, reportType, identifier, err := parseGeneReportURL(normalizedURL)
	if err != nil {
		return nil, model.SpeciesCandidate{}, "", "", err
	}

	resolverSource := w.source
	resolverCandidates, err := w.speciesCandidatesForSource(ctx, resolverSource, candidates)
	if err != nil {
		return nil, model.SpeciesCandidate{}, "", "", fmt.Errorf("load %s species list for URL resolution: %w", resolverSource.Name(), err)
	}

	species, ok := findSpeciesCandidateByJBrowseName(resolverCandidates, jbrowseName)
	if !ok {
		phytozomeSource := phytozome.NewClient(w.httpClient)
		phytozomeCandidates, loadErr := w.speciesCandidatesForSource(ctx, phytozomeSource, nil)
		if loadErr == nil {
			if phytozomeSpecies, phytozomeOK := findSpeciesCandidateByJBrowseName(phytozomeCandidates, jbrowseName); phytozomeOK {
				resolverSource = phytozomeSource
				species = phytozomeSpecies
				ok = true
			}
		}
	}
	if !ok {
		return nil, model.SpeciesCandidate{}, "", "", fmt.Errorf("could not match gene report species %s to a known species in %s or phytozome", jbrowseName, w.source.Name())
	}
	return resolverSource, species, reportType, identifier, nil
}

func (w *BlastWizard) resolveGeneReportSequence(ctx context.Context, resolverSource source.DataSource, species model.SpeciesCandidate, reportType, identifier, input, normalizedURL string) (*model.QuerySequenceSource, error) {
	cacheKey := w.querySourceResolveKey(resolverSource, species, reportType, identifier, normalizedURL)
	if cached, ok := w.cachedResolvedQuerySource(cacheKey, input, normalizedURL); ok {
		return cached, nil
	}
	switch reportType {
	case "gene", "transcript":
		resolved, err := resolverSource.FetchGeneQuerySequence(ctx, species, reportType, identifier)
		if err != nil {
			return nil, err
		}
		gene := *resolved
		gene.OriginalInputURL = strings.TrimSpace(input)
		gene.NormalizedURL = normalizedURL
		if gene.SourceDatabase == "" {
			gene.SourceDatabase = resolverSource.Name()
		}
		if gene.SourceProteomeID == 0 {
			gene.SourceProteomeID = species.ProteomeID
		}
		if gene.SourceJBrowseName == "" {
			gene.SourceJBrowseName = species.JBrowseName
		}
		if gene.SourceGenomeLabel == "" {
			gene.SourceGenomeLabel = species.GenomeLabel
		}
		if gene.GeneID == "" {
			gene.GeneID = identifier
		}
		w.storeResolvedQuerySource(cacheKey, gene)
		return &gene, nil
	case "protein":
		resolver, ok := resolverSource.(source.ProteinReportResolver)
		if !ok {
			return nil, fmt.Errorf("%s does not support protein report URL resolution", databaseDisplayName(resolverSource.Name()))
		}
		resolved, err := resolver.FetchProteinQuerySequence(ctx, species, identifier)
		if err != nil {
			return nil, err
		}
		gene := *resolved
		gene.OriginalInputURL = strings.TrimSpace(input)
		gene.NormalizedURL = normalizedURL
		if gene.SourceDatabase == "" {
			gene.SourceDatabase = resolverSource.Name()
		}
		if gene.SourceProteomeID == 0 {
			gene.SourceProteomeID = species.ProteomeID
		}
		if gene.SourceJBrowseName == "" {
			gene.SourceJBrowseName = species.JBrowseName
		}
		if gene.SourceGenomeLabel == "" {
			gene.SourceGenomeLabel = species.GenomeLabel
		}
		if gene.ProteinID == "" {
			gene.ProteinID = identifier
		}
		w.storeResolvedQuerySource(cacheKey, gene)
		return &gene, nil
	default:
		return nil, fmt.Errorf("unsupported report URL type %q", reportType)
	}
}

func (w *BlastWizard) querySourceResolveKey(src source.DataSource, species model.SpeciesCandidate, reportType, identifier, normalizedURL string) string {
	sourceName := ""
	if src != nil {
		sourceName = src.Name()
	}
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(sourceName)),
		strconv.Itoa(species.ProteomeID),
		strings.ToLower(strings.TrimSpace(species.JBrowseName)),
		strings.ToLower(strings.TrimSpace(reportType)),
		strings.TrimSpace(identifier),
		strings.TrimSpace(normalizedURL),
	}, "|")
}

func (w *BlastWizard) cachedResolvedQuerySource(cacheKey, input, normalizedURL string) (*model.QuerySequenceSource, bool) {
	if strings.TrimSpace(cacheKey) == "" {
		return nil, false
	}
	w.querySourceResolveMu.RLock()
	cached, ok := w.querySourceResolveCache[cacheKey]
	w.querySourceResolveMu.RUnlock()
	if !ok {
		return nil, false
	}
	copySource := cached
	copySource.OriginalInputURL = strings.TrimSpace(input)
	copySource.NormalizedURL = normalizedURL
	return &copySource, true
}

func (w *BlastWizard) storeResolvedQuerySource(cacheKey string, source model.QuerySequenceSource) {
	if strings.TrimSpace(cacheKey) == "" {
		return
	}
	w.querySourceResolveMu.Lock()
	if w.querySourceResolveCache == nil {
		w.querySourceResolveCache = make(map[string]model.QuerySequenceSource)
	}
	w.querySourceResolveCache[cacheKey] = source
	w.querySourceResolveMu.Unlock()
}

func (w *BlastWizard) fetchProteinSequenceRecords(ctx context.Context, rows []model.BlastResultRow) ([]model.ProteinSequenceRecord, error) {
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Export", "Fetching peptides"),
		Title:       "Fetching peptide sequences",
		Description: "Fetching peptide sequences for selected BLAST rows.",
		Initial:     "Fetching peptide sequences...",
		Total:       len(rows),
		CancelError: prompt.ErrBackToRowSelection,
	}, func(taskCtx context.Context, update func(int, string)) ([]model.ProteinSequenceRecord, error) {
		return w.fetchProteinSequenceRecordsWithProgress(mergeContexts(ctx, taskCtx), rows, update)
	})
}

func (w *BlastWizard) fetchProteinSequenceRecordsMaybeSilent(ctx context.Context, rows []model.BlastResultRow) ([]model.ProteinSequenceRecord, error) {
	if w.suppressTaskModals {
		return w.fetchProteinSequenceRecordsWithProgress(ctx, rows, nil)
	}
	return w.fetchProteinSequenceRecords(ctx, rows)
}

func (w *BlastWizard) fetchProteinSequenceRecordsWithProgress(ctx context.Context, rows []model.BlastResultRow, update func(int, string)) ([]model.ProteinSequenceRecord, error) {
	progress := safeProgress(update)
	records := make([]model.ProteinSequenceRecord, 0, len(rows))

	results := w.prefetchBlastSequences(ctx, rows, update)

	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sequenceID := firstNonEmpty(row.SequenceID, row.TranscriptID, row.Protein)
		cacheKey := fmt.Sprintf("%d:%s", row.TargetID, sequenceID)

		prefetched, ok := results[cacheKey]
		if !ok || prefetched.err != nil {
			if ok && prefetched.err != nil && !isMissingProteinSequenceError(prefetched.err) {
				return nil, fmt.Errorf("protein sequence for %s: %w", sequenceID, prefetched.err)
			}
			progress(len(records), fmt.Sprintf("Skipped missing peptide sequence for %s.", sequenceID))
			continue
		}
		sequence := prefetched.data.Sequence
		originalHeader := strings.TrimSpace(prefetched.data.OriginalHeader)
		if originalHeader == "" {
			originalHeader = blastProteinSequenceHeader(row)
		}

		records = append(records, model.ProteinSequenceRecord{
			Header:         blastProteinSequenceHeader(row),
			OriginalHeader: originalHeader,
			SourceKey:      blastSequenceRecordSourceKey(row),
			Sequence:       sequence,
		})
	}

	progress(len(rows), "Fetched peptide sequences.")
	return records, nil
}

func (w *BlastWizard) fetchKeywordProteinSequenceRecords(ctx context.Context, selected model.SpeciesCandidate, rows []model.KeywordResultRow) ([]model.ProteinSequenceRecord, error) {
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Export", "Fetching keyword peptides"),
		Title:       "Fetching keyword peptide sequences",
		Description: "Fetching peptide sequences for selected keyword rows.",
		Initial:     "Fetching keyword peptide sequences...",
		Total:       len(rows),
		CancelError: prompt.ErrBackToRowSelection,
	}, func(taskCtx context.Context, update func(int, string)) ([]model.ProteinSequenceRecord, error) {
		return w.fetchKeywordProteinSequenceRecordsWithProgress(mergeContexts(ctx, taskCtx), selected, rows, update)
	})
}

func (w *BlastWizard) fetchKeywordProteinSequenceRecordsWithProgress(ctx context.Context, selected model.SpeciesCandidate, rows []model.KeywordResultRow, update func(int, string)) ([]model.ProteinSequenceRecord, error) {
	progress := safeProgress(update)
	records := make([]model.ProteinSequenceRecord, 0, len(rows))

	results := w.prefetchKeywordSequences(ctx, selected, rows, update)

	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sequenceID := strings.TrimSpace(row.SequenceID)
		if sequenceID == "" {
			return nil, fmt.Errorf("keyword row %s is missing sequence id", row.TranscriptID)
		}

		prefetched, ok := results[sequenceID]
		if !ok || prefetched.err != nil {
			if ok && prefetched.err != nil && !isMissingProteinSequenceError(prefetched.err) {
				return nil, fmt.Errorf("protein sequence for keyword row %s: %w", row.TranscriptID, prefetched.err)
			}
			progress(len(records), fmt.Sprintf("Skipped missing keyword peptide sequence for %s.", sequenceID))
			continue
		}
		sequence := prefetched.data.Sequence
		originalHeader := strings.TrimSpace(prefetched.data.OriginalHeader)
		if originalHeader == "" {
			originalHeader = keywordProteinSequenceHeader(row)
		}

		records = append(records, model.ProteinSequenceRecord{
			Header:         keywordProteinSequenceHeader(row),
			OriginalHeader: originalHeader,
			SourceKey:      keywordSequenceRecordSourceKey(row),
			Sequence:       sequence,
		})
	}

	progress(len(rows), "Fetched keyword peptide sequences.")
	return records, nil
}

func keywordProteinSequenceHeader(row model.KeywordResultRow) string {
	parts := make([]string, 0, 3)
	if label := strings.TrimSpace(row.SequenceHeaderLabel); label != "" {
		parts = append(parts, label)
	}
	if transcript := strings.TrimSpace(row.TranscriptID); transcript != "" {
		parts = append(parts, transcript)
	}
	if len(parts) == 0 {
		parts = append(parts, strings.TrimSpace(row.SequenceID))
	}
	header := ">" + strings.Join(parts, "|")
	if label := rowKeywordLabelName(row); label != "" {
		header += " (" + strings.TrimSpace(label) + ")"
	}
	return header
}

func blastProteinSequenceHeader(row model.BlastResultRow) string {
	return ">" + strings.TrimSpace(firstNonEmpty(
		strings.TrimSpace(row.Protein),
		strings.TrimSpace(row.SequenceID),
		strings.TrimSpace(row.TranscriptID),
		strings.TrimSpace(row.SubjectID),
	))
}

func (w *BlastWizard) proteinSequenceCacheKey(targetID int, sequenceID string) string {
	sourceName := "unknown"
	if w.source != nil {
		sourceName = w.source.Name()
		if strings.EqualFold(sourceName, "lemna") {
			targetID = 0
		}
	}
	return databaseDisplayName(sourceName) + ":" + strconv.Itoa(targetID) + ":" + strings.TrimSpace(sequenceID)
}

func (w *BlastWizard) cachedProteinSequence(cacheKey string) (model.ProteinSequenceData, bool) {
	w.proteinSequenceMu.RLock()
	sequence, ok := w.proteinSequenceCache[cacheKey]
	w.proteinSequenceMu.RUnlock()
	return sequence, ok && strings.TrimSpace(sequence.Sequence) != ""
}

func (w *BlastWizard) cachedProteinSequenceMiss(cacheKey string) error {
	w.proteinSequenceMu.RLock()
	err := w.proteinSequenceMiss[cacheKey]
	w.proteinSequenceMu.RUnlock()
	return err
}

func (w *BlastWizard) storeProteinSequence(cacheKey string, sequence model.ProteinSequenceData) {
	sequence.Sequence = strings.TrimSpace(sequence.Sequence)
	sequence.OriginalHeader = strings.TrimSpace(sequence.OriginalHeader)
	if cacheKey == "" || sequence.Sequence == "" {
		return
	}
	w.proteinSequenceMu.Lock()
	w.proteinSequenceCache[cacheKey] = sequence
	delete(w.proteinSequenceMiss, cacheKey)
	w.proteinSequenceMu.Unlock()
}

func (w *BlastWizard) storeProteinSequenceMiss(cacheKey string, err error) {
	if cacheKey == "" || err == nil {
		return
	}
	w.proteinSequenceMu.Lock()
	w.proteinSequenceMiss[cacheKey] = err
	w.proteinSequenceMu.Unlock()
}

func (w *BlastWizard) fetchProteinSequenceCached(ctx context.Context, targetID int, sequenceID string) (model.ProteinSequenceData, error) {
	sequenceID = strings.TrimSpace(sequenceID)
	if sequenceID == "" {
		return model.ProteinSequenceData{}, fmt.Errorf("empty protein sequence id")
	}
	cacheKey := w.proteinSequenceCacheKey(targetID, sequenceID)
	if sequence, ok := w.cachedProteinSequence(cacheKey); ok {
		return sequence, nil
	}
	if err := w.cachedProteinSequenceMiss(cacheKey); err != nil {
		return model.ProteinSequenceData{}, err
	}

	value, err, _ := w.proteinSequenceGroup.Do(cacheKey, func() (any, error) {
		if sequence, ok := w.cachedProteinSequence(cacheKey); ok {
			return sequence, nil
		}
		if err := w.cachedProteinSequenceMiss(cacheKey); err != nil {
			return model.ProteinSequenceData{}, err
		}
		sequence, err := w.source.FetchProteinSequence(ctx, targetID, sequenceID)
		if err != nil {
			w.storeProteinSequenceMiss(cacheKey, err)
			return model.ProteinSequenceData{}, err
		}
		w.storeProteinSequence(cacheKey, sequence)
		return sequence, nil
	})
	if err != nil {
		return model.ProteinSequenceData{}, err
	}
	return value.(model.ProteinSequenceData), nil
}

func (w *BlastWizard) prefetchBlastSequences(ctx context.Context, rows []model.BlastResultRow, update func(int, string)) map[string]sequenceFetchResult {
	progress := safeProgress(update)
	type fetchTask struct {
		key      string
		targetID int
		id       string
	}

	results := make(map[string]sequenceFetchResult, len(rows))
	taskByKey := make(map[string]fetchTask, len(rows))
	for _, row := range rows {
		sequenceID := firstNonEmpty(row.SequenceID, row.TranscriptID, row.Protein)
		if sequenceID == "" {
			continue
		}
		key := fmt.Sprintf("%d:%s", row.TargetID, sequenceID)
		cacheKey := w.proteinSequenceCacheKey(row.TargetID, sequenceID)
		if sequence, ok := w.cachedProteinSequence(cacheKey); ok {
			results[key] = sequenceFetchResult{data: sequence}
			continue
		}
		taskByKey[key] = fetchTask{key: key, targetID: row.TargetID, id: sequenceID}
	}
	if len(taskByKey) == 0 {
		progress(len(rows), "Fetched peptide sequences from cache.")
		return results
	}

	tasks := make([]fetchTask, 0, len(taskByKey))
	for _, task := range taskByKey {
		tasks = append(tasks, task)
	}

	var mu sync.Mutex
	jobs := make(chan fetchTask)
	done := make(chan struct{}, len(tasks))
	workerCount := blastSequenceFetchWorkerCount(len(tasks))

	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for task := range jobs {
				sequence, err := w.fetchProteinSequenceCached(ctx, task.targetID, task.id)
				mu.Lock()
				results[task.key] = sequenceFetchResult{data: sequence, err: err}
				mu.Unlock()
				done <- struct{}{}
			}
		}()
	}

	go func() {
		for _, task := range tasks {
			select {
			case <-ctx.Done():
				close(jobs)
				workers.Wait()
				close(done)
				return
			case jobs <- task:
			}
		}
		close(jobs)
		workers.Wait()
		close(done)
	}()

	completedCount := 0
	for range done {
		completedCount++
		progress(completedCount, fmt.Sprintf("Fetching peptide sequences... %d/%d", completedCount, len(tasks)))
	}
	return results
}

func (w *BlastWizard) prefetchKeywordSequences(ctx context.Context, selected model.SpeciesCandidate, rows []model.KeywordResultRow, update func(int, string)) map[string]sequenceFetchResult {
	progress := safeProgress(update)
	taskIDs := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	results := make(map[string]sequenceFetchResult, len(rows))
	targetID := keywordSequenceFetchTargetID(w.source, selected)
	for _, row := range rows {
		sequenceID := strings.TrimSpace(row.SequenceID)
		if sequenceID == "" {
			continue
		}
		if _, ok := seen[sequenceID]; ok {
			continue
		}
		seen[sequenceID] = struct{}{}
		cacheKey := w.proteinSequenceCacheKey(targetID, sequenceID)
		if sequence, ok := w.cachedProteinSequence(cacheKey); ok {
			results[sequenceID] = sequenceFetchResult{data: sequence}
			continue
		}
		taskIDs = append(taskIDs, sequenceID)
	}
	if len(taskIDs) == 0 {
		progress(len(rows), "Fetched keyword peptide sequences from cache.")
		return results
	}

	var mu sync.Mutex
	jobs := make(chan string)
	done := make(chan struct{}, len(taskIDs))
	workerCount := blastSequenceFetchWorkerCount(len(taskIDs))

	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for sequenceID := range jobs {
				sequence, err := w.fetchProteinSequenceCached(ctx, targetID, sequenceID)
				mu.Lock()
				results[sequenceID] = sequenceFetchResult{data: sequence, err: err}
				mu.Unlock()
				done <- struct{}{}
			}
		}()
	}

	go func() {
		for _, sequenceID := range taskIDs {
			select {
			case <-ctx.Done():
				close(jobs)
				workers.Wait()
				close(done)
				return
			case jobs <- sequenceID:
			}
		}
		close(jobs)
		workers.Wait()
		close(done)
	}()

	completedCount := 0
	for range done {
		completedCount++
		progress(completedCount, fmt.Sprintf("Fetching keyword peptide sequences... %d/%d", completedCount, len(taskIDs)))
	}
	return results
}

func keywordSequenceFetchTargetID(src source.DataSource, selected model.SpeciesCandidate) int {
	if src == nil {
		return 0
	}
	if _, ok := src.(*lemna.Client); ok {
		return selected.ProteomeID
	}
	return selected.ProteomeID
}

func (w *BlastWizard) loadKeywordDetailFASTA(row model.KeywordResultRow) (string, error) {
	sequenceID := strings.TrimSpace(row.SequenceID)
	if sequenceID == "" {
		return "", fmt.Errorf("keyword row is missing sequence id")
	}
	targetID := keywordSequenceFetchTargetID(w.source, w.lastKeywordSpecies)
	record, err := w.fetchProteinSequenceCached(context.Background(), targetID, sequenceID)
	if err != nil {
		return "", err
	}
	header := strings.TrimSpace(record.OriginalHeader)
	if header == "" {
		header = keywordProteinSequenceHeader(row)
	}
	return formatDetailFASTA(header, record.Sequence), nil
}

func (w *BlastWizard) loadBlastDetailFASTA(row model.BlastResultRow) (string, error) {
	sequenceID := strings.TrimSpace(firstNonEmpty(row.SequenceID, row.TranscriptID, row.Protein))
	if sequenceID == "" {
		return "", fmt.Errorf("BLAST row is missing sequence id")
	}
	targetID := row.TargetID
	if targetID == 0 {
		targetID = w.phytozomeTargetIDForRow(context.Background(), row)
	}
	record, err := w.fetchProteinSequenceCached(context.Background(), targetID, sequenceID)
	if err != nil {
		return "", err
	}
	header := strings.TrimSpace(record.OriginalHeader)
	if header == "" {
		header = ">" + firstNonEmpty(strings.TrimSpace(row.Protein), strings.TrimSpace(row.SequenceID), strings.TrimSpace(row.TranscriptID), strings.TrimSpace(row.SubjectID))
	}
	return formatDetailFASTA(header, record.Sequence), nil
}

func formatDetailFASTA(header string, sequence string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		header = ">sequence"
	}
	sequence = strings.TrimSpace(sequence)
	if sequence == "" {
		return header
	}
	lines := wrapSequenceForDetail(sequence, 70)
	return header + "\n" + strings.Join(lines, "\n")
}

func wrapSequenceForDetail(sequence string, width int) []string {
	sequence = strings.TrimSpace(sequence)
	if sequence == "" {
		return nil
	}
	if width <= 0 {
		return []string{sequence}
	}
	runes := []rune(sequence)
	lines := make([]string, 0, (len(runes)+width-1)/width)
	for len(runes) > width {
		lines = append(lines, string(runes[:width]))
		runes = runes[width:]
	}
	if len(runes) > 0 {
		lines = append(lines, string(runes))
	}
	return lines
}

func buildExportMetadata(baseName string, querySource *model.QuerySequenceSource) *model.ExportMetadata {
	if querySource == nil {
		return nil
	}

	return &model.ExportMetadata{
		GeneName:      baseName,
		GeneID:        querySourceGeneID(querySource),
		GeneReportURL: firstNonEmpty(querySource.OriginalInputURL, querySource.NormalizedURL),
		Queries:       exportQueryMetadataFromSources([]*model.QuerySequenceSource{querySource}),
	}
}

func buildFamilyExportMetadata(querySources []*model.QuerySequenceSource) *model.ExportMetadata {
	queries := exportQueryMetadataFromSources(querySources)
	if len(queries) == 0 {
		return nil
	}
	metadata := &model.ExportMetadata{Queries: queries}
	metadata.GeneName = queries[0].LabelName
	metadata.GeneID = queries[0].GeneID
	metadata.GeneReportURL = firstNonEmpty(queries[0].OriginalInputURL, queries[0].NormalizedURL)
	return metadata
}

func exportQueryMetadataFromSources(querySources []*model.QuerySequenceSource) []model.ExportQueryMetadata {
	out := make([]model.ExportQueryMetadata, 0, len(querySources))
	for _, source := range querySources {
		if source == nil {
			continue
		}
		out = append(out, model.ExportQueryMetadata{
			Index:             len(out) + 1,
			LabelName:         strings.TrimSpace(source.LabelName),
			GeneID:            querySourceGeneID(source),
			ProteinID:         strings.TrimSpace(source.ProteinID),
			TranscriptID:      strings.TrimSpace(source.TranscriptID),
			SourceDatabase:    strings.TrimSpace(source.SourceDatabase),
			SourceProteomeID:  source.SourceProteomeID,
			SourceJBrowseName: strings.TrimSpace(source.SourceJBrowseName),
			SourceGenomeLabel: strings.TrimSpace(source.SourceGenomeLabel),
			OriginalInputURL:  strings.TrimSpace(source.OriginalInputURL),
			NormalizedURL:     strings.TrimSpace(source.NormalizedURL),
			OrganismShort:     strings.TrimSpace(source.OrganismShort),
			Annotation:        strings.TrimSpace(source.Annotation),
			SequenceLength:    len(sanitizeSequence(source.Sequence)),
		})
	}
	return out
}

func querySourceGeneID(source *model.QuerySequenceSource) string {
	if source == nil {
		return ""
	}
	return preferredPhgoIdentifier(source)
}

func prependQuerySequenceRecord(records []model.ProteinSequenceRecord, querySource *model.QuerySequenceSource, baseName string) []model.ProteinSequenceRecord {
	if querySource == nil {
		return records
	}

	header := ">" + buildQuerySequenceHeaderID(querySource)
	label := strings.TrimSpace(baseName)
	if label != "" {
		header += " (" + label + ")"
	}

	queryRecord := model.ProteinSequenceRecord{
		Header:         header,
		OriginalHeader: header,
		SourceKey:      querySequenceRecordSourceKey(querySource),
		Sequence:       querySource.Sequence,
	}

	return append([]model.ProteinSequenceRecord{queryRecord}, records...)
}

func applyOriginalHeaders(records []model.ProteinSequenceRecord) []model.ProteinSequenceRecord {
	out := append([]model.ProteinSequenceRecord(nil), records...)
	for i := range out {
		header := strings.TrimSpace(out[i].OriginalHeader)
		if header == "" {
			header = strings.TrimSpace(out[i].Header)
		}
		out[i].Header = header
	}
	return out
}

func applyKeywordPhgoHeaders(records []model.ProteinSequenceRecord, rows []model.KeywordResultRow) []model.ProteinSequenceRecord {
	out := append([]model.ProteinSequenceRecord(nil), records...)
	limit := minInt(len(out), len(rows))
	for i := 0; i < limit; i++ {
		if header := keywordPhgoHeader(rows[i], i+1); header != "" {
			out[i].Header = header
		}
	}
	return out
}

func applyBlastPhgoHeaders(records []model.ProteinSequenceRecord, rows []model.BlastResultRow, prependedQueryCount int) []model.ProteinSequenceRecord {
	out := append([]model.ProteinSequenceRecord(nil), records...)
	start := minInt(prependedQueryCount, len(out))
	limit := minInt(len(out)-start, len(rows))
	for i := 0; i < limit; i++ {
		if header := blastPhgoHeader(rows[i], i+1); header != "" {
			out[start+i].Header = header
		}
	}
	return out
}

func keywordPhgoHeader(row model.KeywordResultRow, rowNumber int) string {
	return buildPhgoHeader(
		firstNonEmpty(strings.TrimSpace(row.SequenceHeaderLabel), strings.TrimSpace(row.Genome)),
		rowKeywordLabelName(row),
		firstNonEmpty(strings.TrimSpace(row.TranscriptID), stripTranscriptDecorations(strings.TrimSpace(row.GeneIdentifier))),
		rowNumber,
	)
}

func blastPhgoHeader(row model.BlastResultRow, rowNumber int) string {
	return buildPhgoHeader(
		strings.TrimSpace(row.Species),
		firstNonEmpty(strings.TrimSpace(row.LabelName), strings.TrimSpace(row.BlastLabelName)),
		strings.TrimSpace(row.BlastGeneID),
		rowNumber,
	)
}

func buildPhgoHeader(species string, label string, geneID string, rowNumber int) string {
	species = sanitizePhgoHeaderPart(species)
	label = sanitizePhgoHeaderPart(label)
	geneID = sanitizePhgoHeaderPart(geneID)
	if species == "" || label == "" || geneID == "" {
		return ""
	}
	header := ">phgo://" + species + "/" + label + "/" + geneID
	if rowNumber > 0 {
		header += "/" + strconv.Itoa(rowNumber)
	}
	return header
}

func stripTranscriptDecorations(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if open := strings.Index(value, " ("); open >= 0 {
		value = strings.TrimSpace(value[:open])
	}
	return value
}

func preferredPhgoIdentifier(source *model.QuerySequenceSource) string {
	if source == nil {
		return ""
	}
	for _, value := range []string{
		strings.TrimSpace(source.TranscriptID),
		strings.TrimSpace(source.GeneID),
	} {
		if value != "" {
			return stripTranscriptDecorations(value)
		}
	}
	return ""
}

func sanitizePhgoHeaderPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.Join(strings.Fields(value), " ")
	return strings.TrimSpace(value)
}

func keywordSequenceRecordSourceKey(row model.KeywordResultRow) string {
	return strings.Join([]string{
		"keyword",
		strings.ToLower(strings.TrimSpace(row.SourceDatabase)),
		strings.TrimSpace(row.SequenceID),
		strings.TrimSpace(row.TranscriptID),
		strings.TrimSpace(row.GeneIdentifier),
	}, "|")
}

func blastSequenceRecordSourceKey(row model.BlastResultRow) string {
	return strings.Join([]string{
		"blast",
		strings.ToLower(strings.TrimSpace(row.SourceDatabase)),
		strconv.Itoa(row.TargetID),
		strings.TrimSpace(row.SequenceID),
		strings.TrimSpace(row.TranscriptID),
		strings.TrimSpace(row.Protein),
	}, "|")
}

func querySequenceRecordSourceKey(source *model.QuerySequenceSource) string {
	if source == nil {
		return "query||"
	}
	return strings.Join([]string{
		"query",
		strings.ToLower(strings.TrimSpace(source.SourceDatabase)),
		strings.TrimSpace(source.ProteinID),
		strings.TrimSpace(source.TranscriptID),
		strings.TrimSpace(source.GeneID),
		strings.TrimSpace(source.NormalizedURL),
	}, "|")
}

func buildQuerySequenceHeaderID(querySource *model.QuerySequenceSource) string {
	parts := make([]string, 0, 2)
	left := strings.TrimSpace(strings.TrimSpace(querySource.OrganismShort) + " " + strings.TrimSpace(querySource.Annotation))
	if left != "" {
		parts = append(parts, left)
	}

	id := strings.TrimSpace(querySource.ProteinID)
	if id == "" {
		id = strings.TrimSpace(querySource.TranscriptID)
	}
	if id == "" {
		id = strings.TrimSpace(querySource.GeneID)
	}

	if len(parts) == 0 {
		return id
	}
	if id == "" {
		return parts[0]
	}
	return parts[0] + "|" + id
}

func describeQuerySource(source *model.QuerySequenceSource, targetDatabase string) string {
	switch {
	case source.NormalizedURL != "":
		sourceDatabase := databaseDisplayName(firstNonEmpty(source.SourceDatabase, inferSourceDatabaseFromURL(source.NormalizedURL)))
		targetDatabase = databaseDisplayName(targetDatabase)
		if sourceDatabase != "" && targetDatabase != "" && !strings.EqualFold(sourceDatabase, targetDatabase) {
			return fmt.Sprintf("Resolved peptide sequence from a %s gene report URL. The sequence will be fetched from %s and searched against the selected %s species.", sourceDatabase, sourceDatabase, targetDatabase)
		}
		if sourceDatabase != "" {
			return fmt.Sprintf("Resolved peptide sequence from a %s gene report URL.", sourceDatabase)
		}
		return "Resolved peptide sequence from gene report URL."
	case source.TranscriptID != "" || source.ProteinID != "" || source.GeneID != "":
		return "Resolved query metadata from FASTA header."
	default:
		return "Resolved query sequence metadata."
	}
}

func describeQuerySourceDetails(source *model.QuerySequenceSource, targetDatabase string) string {
	lines := []string{describeQuerySource(source, targetDatabase)}
	if source.GeneID != "" {
		lines = append(lines, "", "Gene ID: "+source.GeneID)
	}
	if source.TranscriptID != "" && source.TranscriptID != source.GeneID {
		lines = append(lines, "Transcript ID: "+source.TranscriptID)
	}
	if source.NormalizedURL != "" {
		lines = append(lines, "URL: "+source.NormalizedURL)
	}
	return strings.Join(lines, "\n")
}

func normalizeGeneReportURL(input string) (string, bool) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", false
	}
	if !strings.Contains(value, "://") {
		value = "https://" + strings.TrimPrefix(value, "//")
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	if !strings.EqualFold(parsed.Host, "phytozome-next.jgi.doe.gov") {
		return "", false
	}

	segments := nonEmptyPathSegments(parsed.Path)
	if len(segments) != 4 || !strings.EqualFold(segments[0], "report") {
		return "", false
	}
	if !slices.Contains([]string{"gene", "transcript", "protein"}, strings.ToLower(segments[1])) {
		return "", false
	}

	parsed.Scheme = "https"
	parsed.Host = "phytozome-next.jgi.doe.gov"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), true
}

func inferSourceDatabaseFromURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(parsed.Host)) {
	case "phytozome-next.jgi.doe.gov":
		return "phytozome"
	case "www.lemna.org", "lemna.org":
		return "lemna"
	default:
		return ""
	}
}

func databaseDisplayName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "phytozome":
		return "Phytozome"
	case "lemna":
		return "lemna.org"
	default:
		return strings.TrimSpace(name)
	}
}

func parseGeneReportURL(rawURL string) (jbrowseName string, reportType string, identifier string, err error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", fmt.Errorf("parse gene report URL: %w", err)
	}

	segments := nonEmptyPathSegments(parsed.Path)
	if len(segments) != 4 || !strings.EqualFold(segments[0], "report") {
		return "", "", "", fmt.Errorf("unsupported gene report URL path: %s", parsed.Path)
	}

	reportType = strings.ToLower(segments[1])
	jbrowseName = segments[2]
	identifier = segments[3]
	if jbrowseName == "" || identifier == "" {
		return "", "", "", fmt.Errorf("gene report URL is missing path identifiers")
	}
	return jbrowseName, reportType, identifier, nil
}

func nonEmptyPathSegments(path string) []string {
	parts := strings.Split(path, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			segments = append(segments, part)
		}
	}
	return segments
}

func findSpeciesCandidateByJBrowseName(candidates []model.SpeciesCandidate, jbrowseName string) (model.SpeciesCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.JBrowseName == jbrowseName {
			return candidate, true
		}
	}
	return model.SpeciesCandidate{}, false
}

func parseFastaQuerySequenceInput(input string) (*model.QuerySequenceSource, bool) {
	header, sequence := splitFastaHeaderAndSequence(input)
	if header == "" || sequence == "" {
		return nil, false
	}

	source := &model.QuerySequenceSource{
		Sequence:       sequence,
		Annotation:     strings.TrimSpace(header),
		SourceDatabase: "fasta",
	}
	if parsed, ok := parsePhgoFastaHeader(header); ok {
		source.LabelName = parsed.LabelName
		source.GeneID = parsed.GeneID
		source.OrganismShort = parsed.Species
		source.Annotation = parsed.RawHeader
	}

	return source, true
}

func splitFastaHeaderAndSequence(input string) (string, string) {
	value := strings.TrimSpace(input)
	if value == "" || !strings.HasPrefix(value, ">") {
		return "", ""
	}

	value = strings.ReplaceAll(value, "\r", "")
	lines := strings.Split(value, "\n")

	firstLine := ""
	remainingSequenceLines := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if firstLine == "" {
			firstLine = line
			continue
		}
		remainingSequenceLines = append(remainingSequenceLines, line)
	}

	if firstLine == "" {
		return "", ""
	}

	headerLine := strings.TrimSpace(strings.TrimPrefix(firstLine, ">"))
	headerLine = stripTrailingParentheticalLabel(headerLine)
	if headerLine == "" {
		return "", ""
	}

	if len(remainingSequenceLines) > 0 {
		sequence := sanitizeSequence(strings.Join(remainingSequenceLines, "\n"))
		return headerLine, sequence
	}

	if strings.HasPrefix(strings.ToLower(headerLine), "phgo://") {
		tokenIndex := findFirstWhitespace(headerLine)
		if tokenIndex < 0 {
			return headerLine, ""
		}
		header := strings.TrimSpace(headerLine[:tokenIndex])
		sequence := sanitizeSequence(headerLine[tokenIndex+1:])
		if header == "" || sequence == "" {
			return "", ""
		}
		return header, sequence
	}

	pipeIndex := strings.LastIndex(headerLine, "|")
	if pipeIndex < 0 {
		return "", ""
	}

	afterPipe := strings.TrimSpace(headerLine[pipeIndex+1:])
	if afterPipe == "" {
		return "", ""
	}

	tokenIndex := findFirstWhitespace(afterPipe)
	if tokenIndex < 0 {
		return headerLine, ""
	}

	idPart := strings.TrimSpace(afterPipe[:tokenIndex])
	sequencePart := strings.TrimSpace(afterPipe[tokenIndex+1:])
	if idPart == "" || sequencePart == "" {
		return "", ""
	}
	if strings.HasPrefix(sequencePart, "(") {
		if closeIndex := strings.Index(sequencePart, ")"); closeIndex >= 0 {
			sequencePart = strings.TrimSpace(sequencePart[closeIndex+1:])
		}
	}
	if sequencePart == "" {
		return "", ""
	}

	header := strings.TrimSpace(headerLine[:pipeIndex+1] + idPart)
	sequence := sanitizeSequence(sequencePart)
	return header, sequence
}

type phgoFastaHeader struct {
	RawHeader  string
	Species    string
	LabelName  string
	GeneID     string
	RowNumber  int
	HasRowPart bool
}

func parsePhgoFastaHeader(header string) (phgoFastaHeader, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return phgoFastaHeader{}, false
	}
	if !strings.HasPrefix(strings.ToLower(header), "phgo://") {
		return phgoFastaHeader{}, false
	}
	body := strings.TrimSpace(header[len("phgo://"):])
	if body == "" {
		return phgoFastaHeader{}, false
	}
	parts := strings.Split(body, "/")
	if len(parts) != 3 && len(parts) != 4 {
		return phgoFastaHeader{}, false
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		if parts[i] == "" {
			return phgoFastaHeader{}, false
		}
	}
	parsed := phgoFastaHeader{
		RawHeader: header,
		Species:   parts[0],
		LabelName: parts[1],
		GeneID:    parts[2],
	}
	if len(parts) == 4 {
		row, err := strconv.Atoi(parts[3])
		if err != nil || row <= 0 {
			return phgoFastaHeader{}, false
		}
		parsed.RowNumber = row
		parsed.HasRowPart = true
	}
	return parsed, true
}

func fastaHeaderPrimaryID(header string) string {
	if parsed, ok := parsePhgoFastaHeader(header); ok {
		return parsed.GeneID
	}
	return ""
}

func trailingParentheticalLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasSuffix(value, ")") {
		return ""
	}
	open := strings.LastIndex(value, " (")
	if open < 0 {
		return ""
	}
	label := strings.TrimSpace(value[open+2 : len(value)-1])
	if label == "" {
		return ""
	}
	for _, ch := range label {
		if ch == ' ' || ch == '\t' {
			return ""
		}
	}
	return label
}

func parentheticalHeaderLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	open := strings.LastIndex(value, " (")
	if open < 0 {
		return ""
	}
	rest := value[open+2:]
	closeIndex := strings.Index(rest, ")")
	if closeIndex < 0 {
		return ""
	}
	label := strings.TrimSpace(rest[:closeIndex])
	if label == "" {
		return ""
	}
	for _, ch := range label {
		if ch == ' ' || ch == '\t' {
			return ""
		}
	}
	return label
}

func stripTrailingParentheticalLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || trailingParentheticalLabel(value) == "" {
		return value
	}
	open := strings.LastIndex(value, " (")
	return strings.TrimSpace(value[:open])
}

func findFirstWhitespace(value string) int {
	for i, ch := range value {
		if ch == ' ' || ch == '\t' {
			return i
		}
	}
	return -1
}

func stripTranscriptSuffix(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	lastDot := strings.LastIndex(value, ".")
	if lastDot <= 0 || lastDot == len(value)-1 {
		return value
	}

	suffix := value[lastDot+1:]
	for _, ch := range suffix {
		if ch < '0' || ch > '9' {
			return value
		}
	}
	return value[:lastDot]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func firstAliasOrEmpty(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func (w *BlastWizard) searchKeywordGroups(ctx context.Context, species model.SpeciesCandidate, keywords []string, identifications []string, forceWideSearch bool) ([]model.KeywordSearchGroup, error) {
	if len(identifications) != 0 && len(identifications) != len(keywords) {
		return nil, fmt.Errorf("keyword label_name count %d does not match keyword count %d", len(identifications), len(keywords))
	}
	if len(keywords) == 0 {
		return nil, nil
	}

	results := make([]keywordSearchResult, len(keywords))
	resumeIndex := 0
	for resumeIndex < len(keywords) {
		partialResults, err := tui.RunProgressTaskValueContext(tui.TaskPage{
			Path:        w.tuiPath("Keyword", "Searching"),
			Title:       "Searching keyword terms",
			Description: "Searching annotation rows for each keyword.",
			Initial:     "Searching keyword terms...",
			Total:       len(keywords),
			CancelError: prompt.ErrBackToQueryInput,
		}, func(taskCtx context.Context, update func(int, string)) ([]keywordSearchResult, error) {
			return w.searchKeywordResultsWithProgress(mergeContexts(ctx, taskCtx), species, keywords, results, resumeIndex, forceWideSearch, update)
		})
		if partialResults != nil {
			results = partialResults
		}
		if err == nil {
			break
		}

		var recoverErr *keywordSearchRecoveryError
		if !errors.As(err, &recoverErr) {
			return nil, err
		}
		action, actionErr := w.prompt.FetchErrorAction(recoverErr.Error(), prompt.ErrBackToQueryInput)
		if actionErr != nil {
			return nil, actionErr
		}
		decision, navErr := interpretRecoveryAction(action, prompt.ErrBackToQueryInput, true)
		if navErr != nil {
			return nil, navErr
		}
		switch decision {
		case recoveryRetry:
			resumeIndex = recoverErr.Index
		case recoverySkip:
			skipped := recoverErr.Result
			skipped.err = nil
			skipped.rows = nil
			results[recoverErr.Index] = skipped
			resumeIndex = recoverErr.Index + 1
		default:
			return nil, fmt.Errorf("unsupported keyword recovery action %q", action)
		}
	}

	return buildKeywordSearchGroups(keywords, identifications, results, forceWideSearch), nil
}

func (w *BlastWizard) searchKeywordGroupsWithProgress(ctx context.Context, species model.SpeciesCandidate, keywords []string, identifications []string, forceWideSearch bool, update func(int, string)) ([]model.KeywordSearchGroup, error) {
	if len(identifications) != 0 && len(identifications) != len(keywords) {
		return nil, fmt.Errorf("keyword label_name count %d does not match keyword count %d", len(identifications), len(keywords))
	}

	results, err := w.searchKeywordResultsWithProgress(ctx, species, keywords, make([]keywordSearchResult, len(keywords)), 0, forceWideSearch, update)
	if err != nil {
		return nil, err
	}
	return buildKeywordSearchGroups(keywords, identifications, results, forceWideSearch), nil
}

func (w *BlastWizard) searchKeywordResultsWithProgress(ctx context.Context, species model.SpeciesCandidate, keywords []string, existing []keywordSearchResult, startIndex int, forceWideSearch bool, update func(int, string)) ([]keywordSearchResult, error) {
	progress := safeProgress(update)
	results := append([]keywordSearchResult(nil), existing...)
	if startIndex < 0 {
		startIndex = 0
	}
	completedCount := countCompletedKeywordResults(results)
	if completedCount < startIndex {
		completedCount = startIndex
	}
	progress(completedCount, "Searching keyword terms...")

	pending := make([]int, 0, len(keywords)-startIndex)
	for i := startIndex; i < len(keywords); i++ {
		if keywordSearchResultCompleted(results[i]) {
			continue
		}
		pending = append(pending, i)
	}

	var progressMu sync.Mutex
	advanceProgress := func() {
		progressMu.Lock()
		completedCount++
		current := completedCount
		progressMu.Unlock()
		progress(current, fmt.Sprintf("Searching keyword terms... %d/%d", current, len(keywords)))
	}

	for cursor := 0; cursor < len(pending); {
		remaining := len(pending) - cursor
		batchSize := keywordSearchWorkerCount(remaining)
		if batchSize <= 0 {
			break
		}
		if batchSize > remaining {
			batchSize = remaining
		}
		batch := pending[cursor : cursor+batchSize]
		batchResults := make([]keywordSearchResult, len(batch))
		var wg sync.WaitGroup
		for batchIndex, keywordIndex := range batch {
			wg.Add(1)
			go func(batchPosition int, resultIndex int) {
				defer wg.Done()
				started := time.Now()
				rows, err := w.searchKeywordRowsWithTimeout(ctx, species, keywords[resultIndex], forceWideSearch)
				result := keywordSearchResult{
					index:   resultIndex,
					started: started,
					ended:   time.Now(),
					rows:    rows,
					err:     err,
				}
				results[resultIndex] = result
				batchResults[batchPosition] = result
				if err == nil {
					advanceProgress()
				}
			}(batchIndex, keywordIndex)
		}
		wg.Wait()
		for batchIndex, keywordIndex := range batch {
			result := batchResults[batchIndex]
			if isKeywordSearchControlError(result.err) {
				return results, result.err
			}
			if result.err != nil {
				return results, &keywordSearchRecoveryError{
					Result:  result,
					Keyword: keywords[keywordIndex],
					Index:   keywordIndex,
					Total:   len(keywords),
					Err:     result.err,
				}
			}
		}
		cursor += batchSize
	}
	progress(len(keywords), "Keyword search completed.")
	return results, nil
}

func (w *BlastWizard) searchKeywordRowsWithTimeout(ctx context.Context, species model.SpeciesCandidate, keyword string, forceWideSearch bool) ([]model.KeywordResultRow, error) {
	if forceWideSearch {
		if wideSource, ok := w.source.(wideKeywordSearcher); ok {
			return wideSource.SearchKeywordRowsWide(ctx, species, keyword)
		}
	}
	return w.source.SearchKeywordRows(ctx, species, keyword)
}

func buildKeywordSearchGroups(keywords []string, identifications []string, results []keywordSearchResult, forceWideSearch bool) []model.KeywordSearchGroup {
	groups := make([]model.KeywordSearchGroup, len(keywords))
	for i, keyword := range keywords {
		rows := append([]model.KeywordResultRow(nil), results[i].rows...)
		labelName := ""
		if len(identifications) == len(keywords) {
			labelName = identifications[i]
		}
		for idx := range rows {
			rows[idx].SearchTerm = keyword
			if strings.TrimSpace(rows[idx].SearchType) == "" && forceWideSearch {
				rows[idx].SearchType = "wide search"
			}
			if strings.TrimSpace(rows[idx].SearchType) == "" {
				rows[idx].SearchType = classifyKeywordInputType(keyword)
			}
			if len(identifications) == len(keywords) {
				rows[idx].LabelName = labelName
			}
		}
		searchType := keywordRowsSearchType(rows, keyword, forceWideSearch)
		groups[i] = model.KeywordSearchGroup{
			SearchTerm:       keyword,
			SearchType:       searchType,
			LabelName:        labelName,
			SearchStartedAt:  results[i].started,
			SearchEndedAt:    results[i].ended,
			SearchDurationMS: results[i].ended.Sub(results[i].started).Milliseconds(),
			Rows:             rows,
		}
	}
	return groups
}

func keywordRowsSearchType(rows []model.KeywordResultRow, keyword string, forceWideSearch bool) string {
	for _, row := range rows {
		if value := strings.TrimSpace(row.SearchType); value != "" {
			return value
		}
	}
	if forceWideSearch {
		return "wide search"
	}
	return classifyKeywordInputType(keyword)
}

func (w *BlastWizard) waitForBlastResultsWithProgress(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, error) {
	type resultPayload struct {
		result model.BlastResult
		err    error
	}

	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "Waiting for results"),
		Title:       "Waiting for BLAST results",
		Description: "The BLAST job has been submitted. Waiting for the remote result page to become available.",
		Initial:     "Waiting for BLAST results...",
		Total:       1,
		CancelError: prompt.ErrBackToQueryInput,
	}, func(taskCtx context.Context, update func(int, string)) (model.BlastResult, error) {
		waitCtx := mergeContexts(ctx, taskCtx)
		progress := safeProgress(update)
		done := make(chan resultPayload, 1)
		go func() {
			result, err := w.source.WaitForBlastResults(waitCtx, jobID, pollInterval, timeout)
			done <- resultPayload{result: result, err: err}
		}()

		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		start := time.Now()
		for {
			select {
			case payload := <-done:
				if payload.err != nil {
					return model.BlastResult{}, payload.err
				}
				progress(1, "BLAST results are ready.")
				return payload.result, nil
			case <-ticker.C:
				elapsed := time.Since(start).Round(time.Second)
				progress(1, fmt.Sprintf("Waiting for BLAST results... %s elapsed", elapsed))
			case <-waitCtx.Done():
				return model.BlastResult{}, waitCtx.Err()
			}
		}
	})
}

func withSpinner(out io.Writer, label string, fn func() error) error {
	_, err := withSpinnerValue(out, label, prompt.ErrBackToRowSelection, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

func withSpinnerValue[T any](out io.Writer, label string, cancelError error, fn func(context.Context) (T, error)) (T, error) {
	return tui.RunTaskValueContext(tui.TaskPage{
		Path:        []string{"phytozome GO", "Task"},
		Title:       strings.TrimSuffix(strings.TrimSpace(label), "..."),
		Description: strings.TrimSpace(label),
		Initial:     strings.TrimSpace(label),
		CancelError: cancelError,
	}, func(taskCtx context.Context, update func(string)) (T, error) {
		safeTaskUpdate(update)(label)
		return fn(taskCtx)
	})
}

// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package workflow

import (
	"context"
	"encoding/json"
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
	"github.com/KiriKirby/phytozome-go/internal/cyp"
	"github.com/KiriKirby/phytozome-go/internal/export"
	"github.com/KiriKirby/phytozome-go/internal/fastautil"
	"github.com/KiriKirby/phytozome-go/internal/interpro"
	"github.com/KiriKirby/phytozome-go/internal/labelname"
	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/ncbi"
	"github.com/KiriKirby/phytozome-go/internal/notifyaudio"
	"github.com/KiriKirby/phytozome-go/internal/phylo"
	"github.com/KiriKirby/phytozome-go/internal/phytozome"
	"github.com/KiriKirby/phytozome-go/internal/progressctx"
	"github.com/KiriKirby/phytozome-go/internal/prompt"
	"github.com/KiriKirby/phytozome-go/internal/report"
	"github.com/KiriKirby/phytozome-go/internal/source"
	"github.com/KiriKirby/phytozome-go/internal/startupstate"
	"github.com/KiriKirby/phytozome-go/internal/tair"
	"github.com/KiriKirby/phytozome-go/internal/tui"
	"github.com/KiriKirby/phytozome-go/internal/uniprot"

	"golang.org/x/sync/singleflight"
)

type BlastWizard struct {
	httpClient              *http.Client
	source                  source.DataSource
	sourceFactory           func(string) source.DataSource
	chooseBlastTargetDB     func() (string, error)
	selectSpeciesHook       func([]model.SpeciesCandidate) (model.SpeciesCandidate, error)
	prompt                  *prompt.Prompter
	out                     io.Writer
	tuiInfo                 tui.StartupInfo
	instanceRunID           string
	instanceID              string
	parentInstanceID        string
	handoffPath             string
	blastProgramPath        string
	pendingMode             QueryMode
	postRunBackTarget       error
	reuseLastBlastInput     bool
	reuseLastBlastRows      bool
	lastBlastRowContext     *blastRowContext
	lastBlastReviewContext  *blastReviewContext
	lastBlastItems          []blastQueryItem
	rewindBlastToInput      bool
	reuseLastKeywordRows    bool
	lastKeywordGroups       []model.KeywordSearchGroup
	lastKeywordReport       *keywordReportRunContext
	lastKeywordSpecies      model.SpeciesCandidate
	rewindKeywordToInput    bool
	lastExternalRefs        externalReferenceConfig
	ncbiReplacementChoice   string
	suppressTaskModals      bool
	ensureCanvasTreeRuntime func(context.Context) error
	transferKind            string
	transferTargetDatabase  string
	transferKeywordRows     []model.KeywordResultRow
	transferBlastRows       []model.BlastResultRow
	transferCanvasItems     []model.CanvasItem
	transferCanvasCurrent   int
	transferCanvasNextID    int
	transferSourceSpecies   model.SpeciesCandidate

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

	canvasTreeViewerMu       sync.Mutex
	canvasTreeViewer         *phylo.ViewerServer
	canvasTreeViewerCancel   context.CancelFunc
	canvasTreeLastPayload    phylo.ViewerPayload
	canvasTreeLastMSAPayload phylo.ViewerPayload
	canvasTreeMSAState       phylo.MSAState
	canvasTreeViewerState    json.RawMessage
	canvasTreeLastPlan       phylo.RunPlan
	canvasTreeMSARowMap      map[string]phylo.InputRecord
	canvasTreeForceCompute   bool
	canvasTreeRefreshRun     func(context.Context, canvasLaunchState, phylo.TreeSettings) error
	canvasTreeMSAApplyRun    func(context.Context, canvasLaunchState, phylo.TreeSettings) error
	canvasTreeMSAApplyMu     sync.Mutex
	canvasTreeRecover        func(description string, backTarget error, allowSkip bool) (string, error)
}

type InstanceLaunchRequest struct {
	ParentInstanceID string
	InstanceID       string
	RunID            string
	HandoffPath      string
	Database         string
	Mode             QueryMode
	Handoff          InstanceHandoff
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

type keywordNoRowsError struct {
	Keyword string
}

func (e keywordNoRowsError) Error() string {
	if strings.TrimSpace(e.Keyword) == "" {
		return "no keyword search results were found"
	}
	return fmt.Sprintf("no keyword search results were found for %q", strings.TrimSpace(e.Keyword))
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
	if strings.Contains(message, "missing protein fasta url") || strings.Contains(message, "empty tair fasta url") {
		return true
	}
	if strings.Contains(message, "no external protein sequence source matched") {
		return true
	}
	if strings.Contains(message, "no tair protein sequence matched") {
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
	ModeFamily  QueryMode = "family"
	ModeCanvas  QueryMode = "canvas"
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
	BaseName              string
	OutputDir             string
	WriteReport           bool
	WriteSession          bool
	WriteText             bool
	WriteConvertedFasta   bool
	WriteAllRows          bool
	WriteExcel            bool
	WriteRawExcel         bool
	FastaHeaderMode       model.FastaHeaderMode
	UsePhgoHeader         bool
	PrependOnlyFirstQuery bool
	TreeSettings          phylo.TreeSettings
}

type exportFileResult struct {
	ExcelPath       string
	TextPath        string
	RawTextPath     string
	RawExcelPath    string
	ReportPath      string
	SessionPath     string
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

func (w *BlastWizard) symbolNameDatabasePath() (string, error) {
	appDir, err := appfs.ApplicationDir()
	if err == nil {
		return labelname.DefaultGeneInfoDatabasePath(appDir), nil
	}
	if path := labelname.DefaultGeneInfoDatabaseCurrentPath(); strings.TrimSpace(path) != "" {
		return path, nil
	}
	return "", err
}

func (w *BlastWizard) ensureSymbolNameDatabase(ctx context.Context, cancelError error) error {
	if err := w.waitForStartupSymbolNameDownload(ctx, cancelError); err != nil {
		return err
	}
	if labelname.DefaultGeneInfoDatabaseAvailable() {
		return nil
	}
	if w.suppressTaskModals {
		return w.ensureSymbolNameDatabaseWithUpdate(ctx, nil, false)
	}
	for {
		_, err := tui.RunProgressTaskValueContext(tui.TaskPage{
			Path:        w.tuiPath("Symbol names", "Database"),
			Title:       "Installing symbol name database",
			Description: "Downloading and building the local NCBI Gene symbol name library.",
			Initial:     "Preparing symbol name database...",
			Total:       1000,
			CancelError: cancelError,
		}, func(taskCtx context.Context, update func(int, string)) (struct{}, error) {
			return struct{}{}, w.ensureSymbolNameDatabaseWithProgress(mergeContexts(ctx, taskCtx), update, true)
		})
		if err == nil {
			return nil
		}
		if isCancellationLikeError(err) {
			return err
		}
		action, actionErr := w.prompt.WorkflowErrorAction(fmt.Sprintf("symbol name database install failed: %v", err), cancelError)
		if actionErr != nil {
			return actionErr
		}
		decision, navErr := interpretRecoveryAction(action, cancelError, false)
		if navErr != nil {
			return navErr
		}
		if decision != recoveryRetry {
			if cancelError != nil {
				return cancelError
			}
			return prompt.ErrBackToQueryInput
		}
	}
}

func (w *BlastWizard) ensureSymbolNameDatabaseWithProgress(ctx context.Context, update func(int, string), allowInstall bool) error {
	if err := w.pollStartupSymbolNameDownload(ctx, func(message string) {
		safeProgress(update)(0, message)
	}); err != nil {
		return err
	}
	if labelname.DefaultGeneInfoDatabaseAvailable() {
		return nil
	}
	path, err := w.symbolNameDatabasePath()
	if err != nil {
		return fmt.Errorf("resolve symbol name database path: %w", err)
	}
	labelname.SetDefaultGeneInfoDatabasePath(path)
	if labelname.DefaultGeneInfoDatabaseAvailable() {
		return nil
	}
	if !allowInstall {
		return fmt.Errorf("%w: missing %s", labelname.ErrGeneInfoDatabaseMissing, path)
	}
	safeProgress(update)(0, "Preparing symbol name database...")
	err = labelname.EnsureDefaultGeneInfoDatabaseProgress(ctx, path, func(event labelname.GeneInfoProgress) {
		safeProgress(update)(geneInfoProgressPermille(event), labelname.FormatGeneInfoProgress(event))
	})
	if err == nil {
		notifyaudio.PlayDone()
	}
	return err
}

func (w *BlastWizard) ensureSymbolNameDatabaseWithUpdate(ctx context.Context, update func(string), allowInstall bool) error {
	if err := w.pollStartupSymbolNameDownload(ctx, update); err != nil {
		return err
	}
	if labelname.DefaultGeneInfoDatabaseAvailable() {
		return nil
	}
	path, err := w.symbolNameDatabasePath()
	if err != nil {
		return fmt.Errorf("resolve symbol name database path: %w", err)
	}
	labelname.SetDefaultGeneInfoDatabasePath(path)
	if labelname.DefaultGeneInfoDatabaseAvailable() {
		return nil
	}
	if !allowInstall {
		return fmt.Errorf("%w: missing %s", labelname.ErrGeneInfoDatabaseMissing, path)
	}
	taskUpdate := safeTaskUpdate(update)
	taskUpdate("Preparing symbol name database...")
	err = labelname.EnsureDefaultGeneInfoDatabaseProgress(ctx, path, func(event labelname.GeneInfoProgress) {
		taskUpdate(labelname.FormatGeneInfoProgress(event))
	})
	if err == nil {
		notifyaudio.PlayDone()
	}
	return err
}

func (w *BlastWizard) waitForStartupInitializationIfNeeded(ctx context.Context) error {
	state, ok := readStartupState()
	if !ok || state.AllowUse {
		return nil
	}
	initial := strings.TrimSpace(state.Message)
	if initial == "" {
		initial = "Waiting for tab 0 initialization to finish..."
	}
	if w.suppressTaskModals {
		return w.pollStartupState(ctx, false, nil)
	}
	_, err := tui.RunTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Startup", "Initialization"),
		Title:       "Waiting for initialization",
		Description: "phytozome GO is waiting for tab 0 to finish startup initialization.",
		Initial:     initial,
		CancelError: prompt.ErrExitRequested,
	}, func(taskCtx context.Context, update func(string)) (struct{}, error) {
		return struct{}{}, w.pollStartupState(mergeContexts(ctx, taskCtx), false, update)
	})
	return err
}

func (w *BlastWizard) waitForStartupSymbolNameDownload(ctx context.Context, cancelError error) error {
	state, ok := readStartupState()
	if !ok || state.Status != startupstate.StatusDownloading {
		return nil
	}
	if w.suppressTaskModals {
		return w.pollStartupSymbolNameDownload(ctx, nil)
	}
	message := "The NCBI Gene symbol name library is already downloading in tab 0.\n\nWait here until tab 0 finishes, or cancel this symbol-name operation."
	if trimmed := strings.TrimSpace(state.Message); trimmed != "" {
		message += "\n\nStatus: " + trimmed
	}
	result, err := tui.RunActionModalPage(tui.ActionModalPage{
		Path:         w.tuiPath("Symbol names", "Database"),
		Title:        "Symbol name download in progress",
		Message:      message,
		Actions:      []tui.Action{{Value: "cancel", Label: tui.ButtonClose, Shortcut: "Esc"}},
		ConfirmText:  "Wait",
		ConfirmValue: "wait",
	})
	if err != nil {
		return err
	}
	if result.Value != "wait" {
		if cancelError != nil {
			return cancelError
		}
		return prompt.ErrBackToQueryInput
	}
	_, err = tui.RunTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Symbol names", "Database"),
		Title:       "Waiting for symbol name database",
		Description: "Waiting for tab 0 to finish the NCBI Gene symbol name library download.",
		Initial:     "Waiting for tab 0 symbol name download...",
		CancelError: cancelError,
	}, func(taskCtx context.Context, update func(string)) (struct{}, error) {
		return struct{}{}, w.pollStartupSymbolNameDownload(mergeContexts(ctx, taskCtx), update)
	})
	return err
}

func (w *BlastWizard) pollStartupSymbolNameDownload(ctx context.Context, update func(string)) error {
	return w.pollStartupState(ctx, true, update)
}

func (w *BlastWizard) pollStartupState(ctx context.Context, symbolOnly bool, update func(string)) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, ok := readStartupState()
		if !ok {
			return nil
		}
		if symbolOnly && state.Status != startupstate.StatusDownloading {
			return nil
		}
		if update != nil {
			message := strings.TrimSpace(state.Message)
			if message == "" {
				message = "Waiting for tab 0 to finish..."
			}
			update(message)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func readStartupState() (startupstate.State, bool) {
	appDir, err := appfs.ApplicationDir()
	if err != nil {
		return startupstate.State{}, false
	}
	return startupstate.Read(appDir)
}

func geneInfoProgressPermille(event labelname.GeneInfoProgress) int {
	if event.Done {
		return 1000
	}
	switch event.Stage {
	case "download":
		if event.TotalBytes > 0 && event.CurrentBytes > 0 {
			return maxInt(1, minInt(700, int(event.CurrentBytes*700/event.TotalBytes)))
		}
		return 1
	case "build":
		if event.TotalBytes > 0 && event.CurrentBytes > 0 {
			return 700 + maxInt(1, minInt(290, int(event.CurrentBytes*290/event.TotalBytes)))
		}
		return 700
	case "complete":
		return 1000
	default:
		return 0
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
	w.prompt.SetCanvasTreePanelChanged(func(state tui.CanvasTreePanelState, opened bool) {
		_ = state
		_ = opened
	})
	w.prompt.SetCanvasTreePreviewReady(w.canvasTreePreviewAvailable)
	w.prompt.SetHomeNavigationEnabled(true)
	return w
}

func NewBlastWizardWithLaunch(out io.Writer, tuiInfo tui.StartupInfo, launch InstanceLaunchRequest) *BlastWizard {
	w := NewBlastWizardWithTUIInfo(out, tuiInfo)
	w.instanceRunID = strings.TrimSpace(launch.RunID)
	w.instanceID = strings.TrimSpace(launch.InstanceID)
	w.parentInstanceID = strings.TrimSpace(launch.ParentInstanceID)
	w.handoffPath = strings.TrimSpace(launch.HandoffPath)
	if launch.Handoff.Kind == "blast-session" {
		w.pendingMode = QueryMode(launch.Handoff.BlastContext.PendingMode)
		w.transferKind = strings.TrimSpace(launch.Handoff.BlastContext.TransferKind)
		w.transferTargetDatabase = strings.TrimSpace(launch.Database)
		w.blastProgramPath = strings.TrimSpace(launch.Handoff.BlastContext.BlastProgramPath)
		w.reuseLastBlastInput = launch.Handoff.BlastContext.ReuseLastBlastInput
		w.reuseLastBlastRows = launch.Handoff.BlastContext.ReuseLastBlastRows
		w.reuseLastKeywordRows = launch.Handoff.BlastContext.ReuseLastKeywordRows
		w.rewindBlastToInput = launch.Handoff.BlastContext.RewindBlastToInput
		w.rewindKeywordToInput = launch.Handoff.BlastContext.RewindKeywordToInput
		w.transferSourceSpecies = launch.Handoff.BlastContext.TransferSourceSpecies
		w.transferKeywordRows = append([]model.KeywordResultRow(nil), launch.Handoff.BlastContext.TransferKeywordRows...)
		w.transferBlastRows = append([]model.BlastResultRow(nil), launch.Handoff.BlastContext.TransferBlastRows...)
		w.transferCanvasItems = cloneCanvasItems(launch.Handoff.BlastContext.TransferCanvasItems)
		w.transferCanvasCurrent = launch.Handoff.BlastContext.TransferCanvasCurrent
		w.transferCanvasNextID = launch.Handoff.BlastContext.TransferCanvasNextID
		w.lastBlastItems = cloneBlastQueryItems(launch.Handoff.BlastContext.LastBlastItems)
		w.lastKeywordGroups = cloneKeywordSearchGroups(launch.Handoff.BlastContext.LastKeywordGroups)
		w.lastKeywordSpecies = launch.Handoff.BlastContext.LastKeywordSpecies
		if launch.Handoff.BlastContext.LastKeywordReport != nil {
			reportCopy := *launch.Handoff.BlastContext.LastKeywordReport
			w.lastKeywordReport = &reportCopy
		}
		if launch.Handoff.BlastContext.LastBlastRowContext != nil {
			rowCopy := *launch.Handoff.BlastContext.LastBlastRowContext
			rowCopy.Rows = append([]model.BlastResultRow(nil), launch.Handoff.BlastContext.LastBlastRowContext.Rows...)
			rowCopy.AllRows = append([]model.BlastResultRow(nil), launch.Handoff.BlastContext.LastBlastRowContext.AllRows...)
			rowCopy.Numbers = append([]int(nil), launch.Handoff.BlastContext.LastBlastRowContext.Numbers...)
			rowCopy.Flags = append([]bool(nil), launch.Handoff.BlastContext.LastBlastRowContext.Flags...)
			rowCopy.SelectedRowsMask = append([]bool(nil), launch.Handoff.BlastContext.LastBlastRowContext.SelectedRowsMask...)
			w.lastBlastRowContext = &rowCopy
		}
		if launch.Handoff.BlastContext.LastBlastReviewContext != nil {
			reviewCopy := *launch.Handoff.BlastContext.LastBlastReviewContext
			reviewCopy.Prepared = cloneBlastQueryItems(launch.Handoff.BlastContext.LastBlastReviewContext.Prepared)
			reviewCopy.Runs = cloneBlastQueryRuns(launch.Handoff.BlastContext.LastBlastReviewContext.Runs)
			w.lastBlastReviewContext = &reviewCopy
		}
	}
	runID, instanceID, parentID, err := ensureLaunchSession(w.instanceRunID, w.instanceID, w.parentInstanceID, "blast")
	if err == nil {
		w.instanceRunID = runID
		w.instanceID = instanceID
		w.parentInstanceID = parentID
	}
	w.prompt.SetHomeNavigationEnabled(isRootInstanceID(w.instanceID))
	if w.instanceRunID != "" && w.instanceID != "" {
		_ = w.markInstanceActive()
		syncInstanceTerminalMetadata(w.tuiInfo.Version, w.instanceRunID, w.instanceID)
	}
	appendSessionDebugLog("launch run_id=%q instance_id=%q parent_id=%q handoff=%q transfer_kind=%q database=%q mode=%q", w.instanceRunID, w.instanceID, w.parentInstanceID, w.handoffPath, w.transferKind, strings.TrimSpace(launch.Database), strings.TrimSpace(string(launch.Mode)))
	return w
}

func (w *BlastWizard) Run(ctx context.Context) error {
	for {
		err := w.runWithPanicBoundary(ctx)
		if err == nil || errors.Is(err, prompt.ErrExitRequested) {
			return nil
		}
		decision, recoveryErr := w.recoverUnexpectedWorkflowError(err, prompt.ErrBackToDatabaseSelection)
		if recoveryErr != nil {
			if errors.Is(recoveryErr, prompt.ErrExitRequested) {
				return nil
			}
			if classifyWizardBack(recoveryErr) != wizardBackNone {
				continue
			}
			return recoveryErr
		}
		if decision != recoveryRetry {
			return nil
		}
	}
}

func (w *BlastWizard) runWithPanicBoundary(ctx context.Context) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("unexpected internal failure: %v", recovered)
			appendSessionDebugLog("recovered workflow panic: %v", recovered)
		}
	}()
	return w.run(ctx)
}

func (w *BlastWizard) run(ctx context.Context) error {
	if w.instanceRunID != "" && w.instanceID != "" {
		defer func() {
			_ = w.markInstanceInactive()
		}()
	}
	if err := w.waitForStartupInitializationIfNeeded(ctx); err != nil {
		if errors.Is(err, prompt.ErrExitRequested) {
			return nil
		}
		return err
	}
	if newMainInterfaceEnabled() && strings.TrimSpace(w.transferKind) == "" && (strings.TrimSpace(w.instanceID) == "" || isRootInstanceID(w.instanceID)) {
		if err := w.runNewMainInterface(ctx); err != nil {
			if errors.Is(err, prompt.ErrExitRequested) {
				return nil
			}
			return err
		}
		return nil
	}
databaseLoop:
	for {
		if strings.TrimSpace(w.transferKind) != "" {
			appendSessionDebugLog("run transfer_entry run_id=%q instance_id=%q transfer_kind=%q target_db=%q", w.instanceRunID, w.instanceID, w.transferKind, w.transferTargetDatabase)
			err := w.runTransferEntry(ctx)
			if errors.Is(err, prompt.ErrExitRequested) {
				return nil
			}
			if errors.Is(err, prompt.ErrBackToDatabaseSelection) {
				w.transferKind = ""
				w.transferTargetDatabase = ""
				w.transferKeywordRows = nil
				w.transferBlastRows = nil
				continue databaseLoop
			}
			return err
		}
		if w.instanceRunID != "" && w.instanceID != "" {
			_ = w.markInstanceActive()
			syncInstanceTerminalMetadata(w.tuiInfo.Version, w.instanceRunID, w.instanceID)
		}
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
			if mode == ModeCanvas {
				if err := w.runCanvasMode(ctx, canvasLaunchState{
					Items:         cloneCanvasItems(w.transferCanvasItems),
					CurrentItem:   w.transferCanvasCurrent,
					NextNumericID: w.transferCanvasNextID,
					SaveBaseName:  canvasDefaultSaveName(w.transferCanvasItems),
				}); err != nil {
					switch classifyWizardBack(err) {
					case wizardBackExit:
						return nil
					case wizardBackDatabase:
						continue databaseLoop
					case wizardBackMode:
						continue modeLoop
					}
					return err
				}
				continue databaseLoop
			}

			candidates, err := w.loadSpeciesCandidatesForMode(ctx, mode)
			if errors.Is(err, prompt.ErrExitRequested) {
				return nil
			}
			if errors.Is(err, prompt.ErrBackToDatabaseSelection) {
				continue databaseLoop
			}
			if err != nil {
				return err
			}

			selected := model.SpeciesCandidate{}
			needSelect := true

		speciesLoop:
			for {
				if needSelect || selected.JBrowseName == "" {
					selected, err = w.selectSpecies(candidates)
					if mode == ModeFamily {
						if _, familyErr := w.familySource(w.source); familyErr != nil {
							return familyErr
						}
					}
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
				case ModeFamily:
					if err := w.runFamilyMode(ctx, selected); err != nil {
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

func (w *BlastWizard) recoverUnexpectedWorkflowError(err error, backTarget error) (recoveryDecision, error) {
	if err == nil {
		return recoveryBack, nil
	}
	action, actionErr := w.prompt.WorkflowErrorAction(fmt.Sprintf("Unexpected workflow error: %v", err), backTarget)
	if actionErr != nil {
		return recoveryBack, actionErr
	}
	return interpretRecoveryAction(action, backTarget, false)
}

func (w *BlastWizard) runTransferEntry(ctx context.Context) error {
	if w.instanceRunID != "" && w.instanceID != "" {
		_ = w.markInstanceActive()
		syncInstanceTerminalMetadata(w.tuiInfo.Version, w.instanceRunID, w.instanceID)
	}

	if strings.EqualFold(strings.TrimSpace(w.transferKind), "canvas_items") {
		return w.runCanvasMode(ctx, canvasLaunchState{
			Items:         cloneCanvasItems(w.transferCanvasItems),
			CurrentItem:   w.transferCanvasCurrent,
			NextNumericID: w.transferCanvasNextID,
			SaveBaseName:  canvasDefaultSaveName(w.transferCanvasItems),
		})
	}

	err := w.runAgainstBlastTargetDatabase(ctx, w.transferTargetDatabase, func(database string) {
		w.transferTargetDatabase = strings.ToLower(strings.TrimSpace(database))
	}, func(targetSpecies model.SpeciesCandidate) error {
		return w.runTransferredBlastMode(ctx, targetSpecies)
	})
	if err == nil {
		w.clearTransferBlastState()
	}
	return err
}

func (w *BlastWizard) chooseBlastTargetDatabase() (string, error) {
	if w.chooseBlastTargetDB != nil {
		return w.chooseBlastTargetDB()
	}
	return w.prompt.ChooseBlastTargetDatabase()
}

func (w *BlastWizard) clearTransferBlastState() {
	w.transferKind = ""
	w.transferTargetDatabase = ""
	w.transferKeywordRows = nil
	w.transferBlastRows = nil
	w.transferSourceSpecies = model.SpeciesCandidate{}
}

func (w *BlastWizard) runAgainstBlastTargetDatabase(ctx context.Context, initialDatabase string, rememberDatabase func(string), run func(model.SpeciesCandidate) error) error {
	if run == nil {
		return fmt.Errorf("missing BLAST target runner")
	}

	previousSource := w.source
	previousBlastProgram := w.blastProgramPath
	previousDB := ""
	if previousSource != nil {
		previousDB = databaseDisplayName(previousSource.Name())
	}
	defer func() {
		w.source = previousSource
		w.prompt.SetDatabaseContext(previousDB)
		w.setBlastProgramContext(previousBlastProgram)
	}()

	targetDatabase := strings.ToLower(strings.TrimSpace(initialDatabase))
	var candidates []model.SpeciesCandidate
	var selected model.SpeciesCandidate

targetDatabaseLoop:
	for {
		if targetDatabase == "" {
			chosenDatabase, err := w.chooseBlastTargetDatabase()
			if err != nil {
				return err
			}
			targetDatabase = strings.ToLower(strings.TrimSpace(chosenDatabase))
			if rememberDatabase != nil {
				rememberDatabase(targetDatabase)
			}
		}

		dataSource, err := w.dataSourceForDatabase(targetDatabase)
		if err != nil {
			return fmt.Errorf("resolve BLAST target database: %w", err)
		}
		w.source = dataSource
		w.prompt.SetDatabaseContext(databaseDisplayName(w.source.Name()))
		w.setBlastProgramContext("")

		candidates, err = w.loadSpeciesCandidates(ctx)
		if err != nil {
			switch transferTargetBackAction(err) {
			case wizardBackExit:
				return err
			case wizardBackDatabase:
				targetDatabase = ""
				if rememberDatabase != nil {
					rememberDatabase("")
				}
				selected = model.SpeciesCandidate{}
				continue targetDatabaseLoop
			default:
				return err
			}
		}

		selected = model.SpeciesCandidate{}
	targetSpeciesLoop:
		for {
			if selected.JBrowseName == "" {
				var err error
				selected, err = w.selectSpecies(candidates)
				switch transferTargetBackAction(err) {
				case wizardBackNone:
				case wizardBackExit:
					return err
				case wizardBackDatabase:
					targetDatabase = ""
					if rememberDatabase != nil {
						rememberDatabase("")
					}
					selected = model.SpeciesCandidate{}
					continue targetDatabaseLoop
				default:
					if err != nil {
						return err
					}
				}
			}

			err := run(selected)
			switch transferTargetBackAction(err) {
			case wizardBackNone:
				return nil
			case wizardBackExit:
				return err
			case wizardBackDatabase:
				targetDatabase = ""
				if rememberDatabase != nil {
					rememberDatabase("")
				}
				selected = model.SpeciesCandidate{}
				continue targetDatabaseLoop
			case wizardBackSpecies:
				selected = model.SpeciesCandidate{}
				continue targetSpeciesLoop
			default:
				return err
			}
		}
	}
}

func transferTargetBackAction(err error) wizardBackAction {
	action := classifyWizardBack(err)
	switch action {
	case wizardBackMode:
		return wizardBackDatabase
	default:
		return action
	}
}

func (w *BlastWizard) shouldSpawnChildTab() bool {
	ok := strings.TrimSpace(w.instanceRunID) != "" && strings.TrimSpace(w.instanceID) != ""
	appendSessionDebugLog("should_spawn run_id=%q instance_id=%q parent_id=%q transfer_kind=%q ok=%t", w.instanceRunID, w.instanceID, w.parentInstanceID, w.transferKind, ok)
	return ok
}

func (w *BlastWizard) spawnChildTab(ctx context.Context, database string, mode QueryMode, handoff InstanceHandoff) error {
	if !w.shouldSpawnChildTab() {
		return fmt.Errorf("instance launch context is missing")
	}
	_, err := SpawnNewTab(ctx, SpawnLaunchOptions{
		RunID:    w.instanceRunID,
		ParentID: w.instanceID,
		Database: database,
		Mode:     mode,
		Handoff:  handoff,
	})
	return err
}

func (w *BlastWizard) dataSourceForDatabase(database string) (source.DataSource, error) {
	name := strings.ToLower(strings.TrimSpace(database))
	if name == "" {
		return nil, fmt.Errorf("empty database name")
	}
	if w.sourceFactory != nil {
		if src := w.sourceFactory(name); src != nil {
			return src, nil
		}
	}
	switch name {
	case "phytozome":
		return phytozome.NewClient(w.httpClient), nil
	case "lemna":
		return lemna.NewClient(w.httpClient), nil
	case "cyp":
		return cyp.NewClient(w.httpClient), nil
	case "tair":
		return tair.NewClient(w.httpClient), nil
	case "ncbi":
		return ncbi.NewClient(w.httpClient), nil
	default:
		return nil, fmt.Errorf("unsupported BLAST target database %q", database)
	}
}

func (w *BlastWizard) transferSourceForDatabase(database string) (source.DataSource, error) {
	name := strings.ToLower(strings.TrimSpace(database))
	if name == "" {
		return nil, nil
	}
	if w.source != nil && strings.EqualFold(strings.TrimSpace(w.source.Name()), name) {
		return w.source, nil
	}
	return w.dataSourceForDatabase(name)
}

func (w *BlastWizard) withTemporarySource(src source.DataSource, fn func() error) error {
	if src == nil || fn == nil {
		if fn == nil {
			return nil
		}
		return fn()
	}
	previous := w.source
	w.source = src
	defer func() {
		w.source = previous
	}()
	return fn()
}

func transferredKeywordRowsSourceDatabase(rows []model.KeywordResultRow) string {
	for _, row := range rows {
		if database := strings.TrimSpace(row.SourceDatabase); database != "" {
			return database
		}
	}
	return ""
}

func blastRowsSourceDatabase(rows []model.BlastResultRow) string {
	for _, row := range rows {
		if database := strings.TrimSpace(row.SourceDatabase); database != "" {
			return database
		}
	}
	return ""
}

func (w *BlastWizard) resolveTransferredKeywordRowsToBlastItems(ctx context.Context, selected model.SpeciesCandidate, rows []model.KeywordResultRow) ([]blastQueryItem, error) {
	override, err := w.transferSourceForDatabase(transferredKeywordRowsSourceDatabase(rows))
	if err != nil {
		return nil, err
	}
	var (
		items  []blastQueryItem
		runErr error
	)
	if err := w.withTemporarySource(override, func() error {
		items, runErr = w.resolveKeywordRowsToBlastItems(ctx, selected, rows)
		return runErr
	}); err != nil {
		return nil, err
	}
	return items, nil
}

func (w *BlastWizard) resolveTransferredBlastRowsToBlastItems(ctx context.Context, selected model.SpeciesCandidate, rows []model.BlastResultRow) ([]blastQueryItem, error) {
	override, err := w.transferSourceForDatabase(blastRowsSourceDatabase(rows))
	if err != nil {
		return nil, err
	}
	var (
		items  []blastQueryItem
		runErr error
	)
	if err := w.withTemporarySource(override, func() error {
		items, runErr = w.resolveBlastRowsToBlastItems(ctx, selected, rows)
		return runErr
	}); err != nil {
		return nil, err
	}
	return items, nil
}

func (w *BlastWizard) chooseMode() (QueryMode, error) {
	for {
		if strings.TrimSpace(w.transferKind) != "" {
			return ModeBlast, nil
		}
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
		case "tair":
			return tair.NewClient(w.httpClient), nil
		case "ncbi":
			return ncbi.NewClient(w.httpClient), nil
		default:
			return nil, fmt.Errorf("unsupported database %q", choice.Database)
		}
	}
}

func (w *BlastWizard) familySource(src source.DataSource) (source.DataSource, error) {
	if src == nil {
		return nil, fmt.Errorf("empty source")
	}
	if _, ok := src.(source.FamilyCandidateFetcher); !ok {
		return nil, fmt.Errorf("%s does not support family search", src.Name())
	}
	return src, nil
}

func (w *BlastWizard) runStartupTool(tool string) error {
	switch strings.TrimSpace(tool) {
	case "open_session":
		return w.openSessionSnapshotTool(context.Background())
	case "new_canvas":
		return w.runCanvasMode(context.Background(), canvasLaunchState{})
	case "nwk_browser":
		return w.runNewickBrowserTool(context.Background())
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
	case ModeFamily:
		w.rewindKeywordToInput = true
	}
}

func (w *BlastWizard) configureBlastRequest(ctx context.Context, selected model.SpeciesCandidate, baseRequest model.BlastRequest) (model.BlastRequest, error) {
	request := baseRequest
	switch src := w.source.(type) {
	case *lemna.Client:
		cap, err := w.detectLemnaBlastCapabilities(ctx, src, selected, "Preparing BLAST program selection")
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
	case *tair.Client:
		progs := []string{"blastn", "blastx", "tblastn", "blastp"}
		chosenProg, err := w.prompt.ChooseBlastProgram(progs)
		if err != nil {
			return model.BlastRequest{}, err
		}
		applyBlastProgram(&request, chosenProg)
		request.Program = "local:" + request.Program
		w.setBlastProgramContext(blastProgramPathLabel(request.Program))
		return request, nil
	default:
		return request, nil
	}
}

func (w *BlastWizard) configureBlastRequestBeforeInput(ctx context.Context, selected model.SpeciesCandidate) (blastRequestConfig, error) {
	switch w.source.(type) {
	case *lemna.Client, *tair.Client:
	default:
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
		Description: "Checking local FASTA downloads for the selected species so BLAST+ can run fully offline against local data.",
		Initial:     fmt.Sprintf("Checking BLAST availability for %s...", selected.DisplayLabel()),
		CancelError: prompt.ErrBackToQueryInput,
	}, func(taskCtx context.Context, update func(string)) (lemna.BlastCapability, error) {
		safeTaskUpdate(update)("Checking local FASTA files for offline BLAST+...")
		return lc.DetectBlastCapabilities(mergeContexts(ctx, taskCtx), selected)
	})
}

func availableBlastProgramsFromCapability(cap lemna.BlastCapability) []string {
	progs := make([]string, 0, 4)
	if cap.HasNucleotideFasta {
		progs = append(progs, "blastn")
	}
	if cap.HasProteinFasta {
		progs = append(progs, "blastx")
	}
	if cap.HasNucleotideFasta {
		progs = append(progs, "tblastn")
	}
	if cap.HasProteinFasta {
		progs = append(progs, "blastp")
	}
	return progs
}

func (w *BlastWizard) chooseLemnaBlastExecution(cap lemna.BlastCapability, selected model.SpeciesCandidate, program string) (string, error) {
	localOK := false
	switch strings.ToLower(strings.TrimSpace(program)) {
	case "blastn":
		localOK = cap.HasNucleotideFasta
	case "tblastn":
		localOK = cap.HasNucleotideFasta
	case "blastx":
		localOK = cap.HasProteinFasta
	case "blastp":
		localOK = cap.HasProteinFasta
	}

	if localOK {
		return "local", nil
	}
	return "", fmt.Errorf("no local FASTA execution target is available for %s on %s", program, selected.DisplayLabel())
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

func (w *BlastWizard) loadSpeciesCandidatesForMode(ctx context.Context, mode QueryMode) ([]model.SpeciesCandidate, error) {
	candidates, err := w.loadSpeciesCandidates(ctx)
	if err != nil {
		return nil, err
	}
	filtered := candidates
	switch src := w.source.(type) {
	case *tair.Client:
		filtered = src.FilterCandidatesForMode(candidates, string(mode))
	}
	if len(filtered) > 0 {
		return filtered, nil
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no species candidates were returned for %s", w.source.Name())
	}
	return nil, fmt.Errorf("no %s candidates are currently usable in %s mode", strings.ToUpper(strings.TrimSpace(w.source.Name())), strings.ToUpper(string(mode)))
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
	return w.cachedBlastLabelLookupByKey(key)
}

func (w *BlastWizard) cachedBlastLabelLookupByKey(key string) (blastAutoLabelResult, bool) {
	if strings.TrimSpace(key) == "" {
		return blastAutoLabelResult{}, false
	}
	w.blastLabelLookupMu.Lock()
	defer w.blastLabelLookupMu.Unlock()
	result, ok := w.blastLabelLookupCache[key]
	return result, ok
}

func (w *BlastWizard) storeBlastLabelLookup(src source.DataSource, species model.SpeciesCandidate, item blastQueryItem, result blastAutoLabelResult) {
	key := w.blastLabelLookupKey(src, species, item)
	w.storeBlastLabelLookupByKey(key, result)
}

func (w *BlastWizard) storeBlastLabelLookupByKey(key string, result blastAutoLabelResult) {
	if strings.TrimSpace(key) == "" {
		return
	}
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
	if w.selectSpeciesHook != nil {
		return w.selectSpeciesHook(candidates)
	}
	// If lemna source and the candidate list is small, present the full list directly.
	const smallListThreshold = 16

	for {
		if _, ok := w.source.(*ncbi.Client); ok && len(candidates) == 1 {
			return candidates[0], nil
		}

		if _, ok := w.source.(*tair.Client); ok && len(candidates) <= smallListThreshold {
			selected, err := w.prompt.SelectTAIRVersion(candidates)
			if err == nil {
				return selected, nil
			}
			if errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
				return model.SpeciesCandidate{}, err
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("select TAIR version: %v", err), prompt.ErrBackToModeSelection)
			if navErr != nil {
				return model.SpeciesCandidate{}, navErr
			}
			if !retry {
				return model.SpeciesCandidate{}, err
			}
			continue
		}

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
			if _, ok := w.source.(*tair.Client); ok {
				return tair.FilterSpeciesCandidates(candidates, keyword)
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

func (w *BlastWizard) selectFamily(version model.SpeciesCandidate, candidates []model.SpeciesCandidate) (model.SpeciesCandidate, error) {
	_ = version
	searchProvider := func(keyword string, scope []model.SpeciesCandidate) []model.SpeciesCandidate {
		base := candidates
		if len(scope) > 0 {
			base = scope
		}
		if familyFilter, ok := w.source.(source.FamilyCandidateFilter); ok {
			return familyFilter.FilterFamilyCandidates(base, keyword)
		}
		return base
	}
	for {
		restore := w.prompt.PushSessionContext("Family", "TAIR family")
		selected, err := w.prompt.SearchAndSelectFamilyWithProvider(candidates, searchProvider)
		restore()
		if err == nil {
			return selected, nil
		}
		if errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
			return model.SpeciesCandidate{}, err
		}
		retry, navErr := w.retryWorkflowStep(fmt.Sprintf("select family: %v", err), prompt.ErrBackToModeSelection)
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
			var manualLabels []string
			var labelErr error
			if strings.EqualFold(sourceDatabaseName(w.source), "cyp") {
				// CYP PGD rows already contain their authoritative symbol.  The
				// generic label prompt/global symbol DB is not applicable here.
				autoIdentifyLabels = true
			} else {
				manualLabels, labelErr = w.prompt.KeywordLabelNames(len(keywords), prompt.ErrBackToQueryInput)
			}
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
				retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read symbol names: %v", labelErr), prompt.ErrBackToQueryInput)
				if navErr != nil {
					return navErr
				}
				if !retry {
					return labelErr
				}
				continue
			}

			autoIdentifyGeneLoci := false
			var manualGeneLoci []string
			if _, ok := w.source.(*ncbi.Client); ok {
				loci, locusErr := w.prompt.KeywordGeneLoci(len(keywords), prompt.ErrBackToQueryInput)
				manualGeneLoci = loci
				if errors.Is(locusErr, prompt.ErrAutoIdentifyRequested) {
					autoIdentifyGeneLoci = true
					locusErr = nil
				}
				if locusErr != nil {
					if errors.Is(locusErr, prompt.ErrBackToQueryInput) {
						continue keywordInputLoop
					}
					if errors.Is(locusErr, prompt.ErrBackToSpeciesSelection) || errors.Is(locusErr, prompt.ErrBackToModeSelection) || errors.Is(locusErr, prompt.ErrBackToDatabaseSelection) || errors.Is(locusErr, prompt.ErrExitRequested) {
						return locusErr
					}
					retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read Gene locus values: %v", locusErr), prompt.ErrBackToQueryInput)
					if navErr != nil {
						return navErr
					}
					if !retry {
						return locusErr
					}
					continue
				}
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
			groups, err = w.applyNCBIReplacementChoicesWithProgress(ctx, selected, groups)
			if err != nil {
				if errors.Is(err, prompt.ErrDialogClosed) || errors.Is(err, prompt.ErrBackToQueryInput) {
					continue keywordInputLoop
				}
				if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
					return err
				}
				retry, navErr := w.retryWorkflowStep(fmt.Sprintf("resolve NCBI record updates: %v", err), prompt.ErrBackToQueryInput)
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
			if len(manualGeneLoci) == len(keywords) {
				applyKeywordGeneLoci(groups, manualGeneLoci, "user input")
			} else if autoIdentifyGeneLoci {
				if err := w.autoIdentifyNCBIKeywordGeneLociWithProgress(ctx, groups); err != nil {
					if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
						return err
					}
					retry, navErr := w.retryWorkflowStep(fmt.Sprintf("auto identify NCBI Gene locus values: %v", err), prompt.ErrBackToQueryInput)
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
		if w.prompt != nil {
			w.prompt.QueueKeywordResultTableCue()
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
			if selection.CreateCanvas {
				if err := w.runKeywordRowsCanvasMode(ctx, selected, groups, selection.Rows, selection.SelectedByGroup); err != nil {
					if errors.Is(err, prompt.ErrBackToRowSelection) {
						continue keywordRowLoop
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

			if err := w.prepareAndExportKeywordSelectionWithMask(ctx, selected, groups, selection.Rows, selection.Selected, ModeKeyword, reportCtx); err != nil {
				if errors.Is(err, prompt.ErrBackToRowSelection) {
					continue keywordRowLoop
				}
				return err
			}
			continue keywordRowLoop
		}
	}
}

func (w *BlastWizard) runFamilyMode(ctx context.Context, selected model.SpeciesCandidate) error {
	familySource, ok := w.source.(source.FamilyCandidateFetcher)
	if !ok {
		return fmt.Errorf("%s does not support family search", w.source.Name())
	}
	family, familyRows, err := w.searchFamilyRows(ctx, selected, familySource)
	if err != nil {
		return err
	}
	if len(familyRows) == 0 {
		w.postRunBackTarget = prompt.ErrBackToQueryInput
		return w.showInfo("TAIR family results", fmt.Sprintf("No family results were found in %s.", selected.DisplayLabel()), prompt.ErrBackToQueryInput)
	}
	groups := []model.KeywordSearchGroup{{
		SearchTerm:       strings.TrimSpace(firstNonEmpty(family.GenomeLabel, family.JBrowseName, selected.DisplayLabel())),
		SearchType:       "TAIR family",
		LabelName:        strings.TrimSpace(firstNonEmpty(family.LabelName, family.GenomeLabel, family.JBrowseName)),
		LabelSourceField: "family candidate aliases",
		LabelSourceValue: strings.TrimSpace(firstNonEmpty(family.LabelName, family.PhgoAliases, family.GenomeLabel)),
		SearchStartedAt:  time.Now(),
		SearchEndedAt:    time.Now(),
		SearchDurationMS: 0,
		Rows:             familyRows,
	}}
	if aliasText := strings.TrimSpace(family.PhgoAliases); aliasText != "" {
		for i := range groups[0].Rows {
			if strings.TrimSpace(groups[0].Rows[i].PhgoAliases) == "" {
				groups[0].Rows[i].PhgoAliases = aliasText
			}
			if strings.TrimSpace(groups[0].Rows[i].LabelName) == "" && strings.TrimSpace(family.LabelName) != "" {
				groups[0].Rows[i].LabelName = strings.TrimSpace(family.LabelName)
			}
		}
	}
	return w.runKeywordLikeResults(ctx, selected, groups, "family")
}

func (w *BlastWizard) searchFamilyRows(ctx context.Context, selected model.SpeciesCandidate, familySource source.FamilyCandidateFetcher) (model.SpeciesCandidate, []model.KeywordResultRow, error) {
	candidates, err := tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Family", "Load TAIR family candidates"),
		Title:       "Load TAIR family candidates",
		Description: "Loading TAIR family candidates for the selected release and preparing the searchable candidate list.",
		Initial:     "Loading family candidates...",
		Total:       2,
		AllowCancel: true,
	}, func(taskCtx context.Context, update func(int, string)) ([]model.SpeciesCandidate, error) {
		update(1, "Building TAIR release index and collecting family candidates...")
		candidates, err := familySource.FetchFamilyCandidates(taskCtx, selected)
		if err != nil {
			return nil, err
		}
		update(2, fmt.Sprintf("Prepared %d TAIR family candidate(s).", len(candidates)))
		return candidates, nil
	})
	if err != nil {
		return model.SpeciesCandidate{}, nil, err
	}
	if len(candidates) == 0 {
		return model.SpeciesCandidate{}, nil, nil
	}
	family, err := w.selectFamily(selected, candidates)
	if err != nil {
		return model.SpeciesCandidate{}, nil, err
	}
	if familySearcher, ok := w.source.(source.FamilyKeywordSearcher); ok {
		rows, err := tui.RunProgressTaskValueContext(tui.TaskPage{
			Path:        w.tuiPath("Family", "Search TAIR family rows"),
			Title:       "Search TAIR family rows",
			Description: "Loading the selected TAIR family rows and preparing the review table.",
			Initial:     "Searching selected family...",
			Total:       2,
			AllowCancel: true,
		}, func(taskCtx context.Context, update func(int, string)) ([]model.KeywordResultRow, error) {
			update(1, "Searching TAIR family records...")
			rows, err := familySearcher.SearchFamilyKeywordRows(taskCtx, selected, family.JBrowseName)
			if err != nil {
				return nil, err
			}
			update(2, fmt.Sprintf("Prepared %d TAIR family row(s).", len(rows)))
			return rows, nil
		})
		if err != nil {
			return model.SpeciesCandidate{}, nil, err
		}
		return family, rows, nil
	}
	return model.SpeciesCandidate{}, nil, fmt.Errorf("%s does not support family keyword search", w.source.Name())
}

func (w *BlastWizard) runKeywordLikeResults(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup, mode string) error {
	w.lastKeywordSpecies = selected
	if strings.EqualFold(strings.TrimSpace(mode), "family") {
		if identifications, err := w.autoIdentifyKeywordLabelsWithProgress(ctx, selected, groups); err == nil && len(identifications) == len(groups) {
			annotateKeywordLabelSources(groups, identifications, "auto-identify labels")
			applyKeywordLabelIdentifications(groups, identifications)
			applyKeywordLabelMethod(groups, "auto-identify labels")
		}
	}
keywordRowLoop:
	for {
		selection, err := w.selectKeywordRows(groups)
		if err != nil {
			if errors.Is(err, prompt.ErrBackToQueryInput) {
				return err
			}
			if errors.Is(err, prompt.ErrBackToRowSelection) {
				continue keywordRowLoop
			}
			if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
				return err
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("select TAIR rows: %v", err), prompt.ErrBackToQueryInput)
			if navErr != nil {
				return navErr
			}
			if !retry {
				return err
			}
			continue keywordRowLoop
		}
		if selection.RunBlast {
			if err := w.runKeywordBlastMode(ctx, selected, groups, selection.Rows, nil); err != nil {
				if errors.Is(err, prompt.ErrBackToRowSelection) {
					continue keywordRowLoop
				}
				return err
			}
			continue keywordRowLoop
		}
		w.postRunBackTarget = prompt.ErrBackToRowSelection
		if !selection.GenerateFile {
			continue keywordRowLoop
		}
		if err := w.prepareAndExportKeywordSelectionWithMask(ctx, selected, groups, selection.Rows, selection.Selected, QueryMode(mode), nil); err != nil {
			if errors.Is(err, prompt.ErrBackToRowSelection) {
				continue keywordRowLoop
			}
			return err
		}
		continue keywordRowLoop
	}
}

func (w *BlastWizard) runKeywordBlastMode(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup, rows []model.KeywordResultRow, reportCtx *keywordReportRunContext) error {
	if len(rows) == 0 {
		return nil
	}
	if w.shouldSpawnChildTab() {
		w.lastKeywordGroups = cloneKeywordSearchGroups(groups)
		w.lastKeywordSpecies = selected
		if reportCtx != nil {
			copied := *reportCtx
			w.lastKeywordReport = &copied
		}
		handoff := w.SnapshotHandoff("", ModeBlast, "", w.instanceID, w.instanceRunID)
		handoff.StartupSource = "blast-transfer"
		handoff.BlastContext.TransferKind = "keyword_rows"
		handoff.BlastContext.TransferSourceSpecies = selected
		handoff.BlastContext.TransferKeywordRows = append([]model.KeywordResultRow(nil), rows...)
		handoff.BlastContext.ReuseLastKeywordRows = true
		handoff.BlastContext.RewindKeywordToInput = false
		return w.spawnChildTab(ctx, "", ModeBlast, handoff)
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

	w.lastKeywordGroups = cloneKeywordSearchGroups(groups)
	w.lastKeywordSpecies = selected
	if reportCtx != nil {
		copied := *reportCtx
		w.lastKeywordReport = &copied
	}
	return w.runAgainstBlastTargetDatabase(ctx, "", nil, func(targetSpecies model.SpeciesCandidate) error {
		return w.executePreparedBlast(ctx, targetSpecies, prepared, blastRequestConfig{})
	})
}

func (w *BlastWizard) runBlastRowsBlastMode(ctx context.Context, selected model.SpeciesCandidate, rows []model.BlastResultRow) error {
	if len(rows) == 0 {
		return nil
	}
	if w.shouldSpawnChildTab() {
		handoff := w.SnapshotHandoff("", ModeBlast, "", w.instanceID, w.instanceRunID)
		handoff.StartupSource = "blast-transfer"
		handoff.BlastContext.TransferKind = "blast_rows"
		handoff.BlastContext.TransferSourceSpecies = selected
		handoff.BlastContext.TransferBlastRows = append([]model.BlastResultRow(nil), rows...)
		handoff.BlastContext.RewindBlastToInput = false
		return w.spawnChildTab(ctx, "", ModeBlast, handoff)
	}

	prepared, err := w.resolveBlastRowsToBlastItems(ctx, selected, rows)
	if err != nil {
		return err
	}
	if len(prepared) == 0 {
		return w.showInfo("BLAST", "No selected BLAST result rows could be converted into BLAST queries.", prompt.ErrBackToRowSelection)
	}
	prepared, err = w.prepareKeywordBlastItems(ctx, selected, prepared)
	if err != nil {
		return err
	}
	if len(prepared) == 0 {
		return w.showInfo("BLAST", "No selected BLAST result rows remained after BLAST label handling.", prompt.ErrBackToRowSelection)
	}

	return w.runAgainstBlastTargetDatabase(ctx, "", nil, func(targetSpecies model.SpeciesCandidate) error {
		return w.executePreparedBlast(ctx, targetSpecies, prepared, blastRequestConfig{})
	})
}

func (w *BlastWizard) resolveBlastRowsToBlastItems(ctx context.Context, selected model.SpeciesCandidate, rows []model.BlastResultRow) ([]blastQueryItem, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	run := func(taskCtx context.Context, update func(int, string)) ([]blastQueryItem, error) {
		progress := safeProgress(update)
		resolveCtx := mergeContexts(ctx, taskCtx)
		progress(0, "Fetching BLAST hit peptide sequences...")
		items := make([]blastQueryItem, len(rows))
		var mu sync.Mutex
		converted := 0
		if err := runParallel(resolveCtx, len(rows), blastSequenceFetchWorkerCount(len(rows)), func(fetchCtx context.Context, index int) error {
			item, err := w.blastRowToBlastQueryItem(fetchCtx, selected, rows[index])
			if err != nil {
				return err
			}
			mu.Lock()
			items[index] = item
			converted++
			progress(converted, fmt.Sprintf("Building BLAST queries from result rows... %d/%d", converted, len(rows)))
			mu.Unlock()
			return nil
		}); err != nil {
			return nil, err
		}
		out := make([]blastQueryItem, 0, len(rows))
		for _, item := range items {
			if item.QuerySource != nil && strings.TrimSpace(item.Sequence) != "" {
				out = append(out, item)
			}
		}
		progress(len(rows), fmt.Sprintf("Resolved BLAST result rows for BLAST... %d/%d", len(out), len(rows)))
		return out, nil
	}
	if w.suppressTaskModals {
		return run(ctx, nil)
	}
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "BLAST", "Resolving selected rows"),
		Title:       "Resolving BLAST rows for BLAST",
		Description: "Fetching peptide FASTA for selected BLAST result rows and converting them into new BLAST queries.",
		Initial:     "Resolving BLAST rows for BLAST...",
		Total:       len(rows),
		CancelError: prompt.ErrBackToRowSelection,
	}, run)
}

func (w *BlastWizard) blastRowToBlastQueryItem(ctx context.Context, selected model.SpeciesCandidate, row model.BlastResultRow) (blastQueryItem, error) {
	sequenceID := strings.TrimSpace(firstNonEmpty(row.SequenceID, row.TranscriptID, row.Protein))
	if sequenceID == "" {
		return blastQueryItem{}, fmt.Errorf("BLAST row is missing sequence id")
	}
	targetID := row.TargetID
	if targetID == 0 {
		targetID = w.phytozomeTargetIDForRow(ctx, row)
	}
	if targetID == 0 {
		targetID = selected.ProteomeID
	}
	record, err := w.fetchProteinSequenceCached(ctx, targetID, sequenceID)
	if err != nil {
		return blastQueryItem{}, err
	}
	sequence := strings.TrimSpace(record.Sequence)
	if sequence == "" {
		return blastQueryItem{}, fmt.Errorf("resolved empty peptide sequence for %s", sequenceID)
	}
	header := strings.TrimSpace(record.OriginalHeader)
	if header == "" {
		header = ">" + firstNonEmpty(strings.TrimSpace(row.Protein), strings.TrimSpace(row.SequenceID), strings.TrimSpace(row.TranscriptID), strings.TrimSpace(row.SubjectID))
	}
	fasta := formatDetailFASTA(header, sequence)
	headerText, parsedSequence := splitFastaHeaderAndSequence(fasta)
	if parsedSequence == "" {
		parsedSequence = normalizeBlastSequence(sequence, model.SequenceProtein)
	}
	source := &model.QuerySequenceSource{
		Sequence:            parsedSequence,
		ProteinSequence:     parsedSequence,
		SequenceKind:        model.SequenceProtein,
		PreferredSequenceID: firstNonEmpty(strings.TrimSpace(row.Protein), strings.TrimSpace(row.SequenceID), strings.TrimSpace(row.TranscriptID), strings.TrimSpace(row.SubjectID)),
		SourceDatabase:      firstNonEmpty(strings.TrimSpace(row.SourceDatabase), w.source.Name()),
		SourceProteomeID:    targetID,
		SourceJBrowseName:   firstNonEmpty(strings.TrimSpace(row.JBrowseName), selected.JBrowseName),
		SourceGenomeLabel:   firstNonEmpty(strings.TrimSpace(row.Species), selected.GenomeLabel),
		LabelName:           strings.TrimSpace(row.LabelName),
		PhgoAliases:         strings.TrimSpace(row.PhgoAliases),
		UniProtAccession:    strings.TrimSpace(row.UniProtAccession),
		GeneID:              blastHitRowGeneID(row),
		TranscriptID:        strings.TrimSpace(row.TranscriptID),
		ProteinID:           firstNonEmpty(strings.TrimSpace(row.Protein), strings.TrimSpace(row.SequenceID), strings.TrimSpace(row.SubjectID)),
		OrganismShort:       firstNonEmpty(strings.TrimSpace(row.Species), selected.SearchAlias, selected.GenomeLabel),
		Annotation:          firstNonEmpty(strings.TrimSpace(row.Defline), strings.TrimSpace(headerText), strings.TrimSpace(row.Species)),
	}
	return blastQueryItem{
		RawInput:        fasta,
		LabelName:       strings.TrimSpace(row.LabelName),
		Sequence:        parsedSequence,
		ProteinSequence: parsedSequence,
		QuerySource:     source,
	}, nil
}

func blastHitRowGeneID(row model.BlastResultRow) string {
	for _, value := range []string{
		strings.TrimSpace(row.TranscriptID),
		strings.TrimSpace(row.SequenceID),
	} {
		if geneID := stripTranscriptSuffix(value); geneID != "" && !strings.EqualFold(geneID, value) {
			return stripTranscriptDecorations(geneID)
		}
	}
	return ""
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
				retry, navErr := w.retryWorkflowStep(fmt.Sprintf("auto identify BLAST symbol names: %v", err), prompt.ErrBackToRowSelection)
				if navErr != nil {
					return nil, navErr
				}
				if !retry {
					return nil, err
				}
				continue
			}
			if !allLabelsPresent(prepared) {
				action, actionErr := w.prompt.FetchErrorAction("auto identify BLAST symbol names: one or more query symbols could not be identified", prompt.ErrBackToRowSelection)
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
			if blastItemsHaveReusableAliases(prepared) {
				return prepared, nil
			}
			prepared, err = w.supplementBlastAliasesWithProgress(ctx, selected, prepared)
			if err != nil {
				if errors.Is(err, prompt.ErrBackToQueryInput) {
					return nil, prompt.ErrBackToRowSelection
				}
				retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read BLAST alias symbol names: %v", err), prompt.ErrBackToRowSelection)
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
	sanitizeTransferredKeywordQuerySource(&sourceCopy)
	cached.QuerySource = &sourceCopy
	return cached, true
}

func (w *BlastWizard) storeKeywordBlastItem(cacheKey string, item blastQueryItem) {
	if strings.TrimSpace(cacheKey) == "" || item.QuerySource == nil {
		return
	}
	copyItem := item
	sourceCopy := *item.QuerySource
	sanitizeTransferredKeywordQuerySource(&sourceCopy)
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
		UniProtAccession:    strings.TrimSpace(row.UniProt),
		GeneID:              stripTranscriptDecorations(strings.TrimSpace(row.GeneIdentifier)),
		TranscriptID:        strings.TrimSpace(row.TranscriptID),
		ProteinID:           strings.TrimSpace(row.ProteinID),
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

func sanitizeTransferredKeywordQuerySource(source *model.QuerySequenceSource) {
	if source == nil {
		return
	}
	source.Aliases = ""
	source.Symbols = ""
	source.Synonyms = ""
	source.AutoDefine = ""
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
	if strings.TrimSpace(w.transferKind) != "" {
		return w.runTransferredBlastMode(ctx, selected)
	}
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
					retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read symbol names: %v", labelErr), prompt.ErrBackToQueryInput)
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
					retry, navErr := w.retryWorkflowStep(fmt.Sprintf("auto identify BLAST symbol names: %v", err), prompt.ErrBackToQueryInput)
					if navErr != nil {
						return navErr
					}
					if !retry {
						return err
					}
					continue blastInputLoop
				}
				if !allLabelsPresent(prepared) {
					action, actionErr := w.prompt.FetchErrorAction("auto identify BLAST symbol names: one or more query symbols could not be identified", prompt.ErrBackToQueryInput)
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
					retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read symbol names: %v", labelErr), prompt.ErrBackToQueryInput)
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
					retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read BLAST alias symbol names: %v", err), prompt.ErrBackToQueryInput)
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

func (w *BlastWizard) runTransferredBlastMode(ctx context.Context, targetSpecies model.SpeciesCandidate) error {
	transferKind := strings.TrimSpace(w.transferKind)
	sourceSpecies := w.transferSourceSpecies
	switch transferKind {
	case "keyword_rows":
		rows := append([]model.KeywordResultRow(nil), w.transferKeywordRows...)
		if len(rows) == 0 {
			return w.showInfo("Keyword BLAST", "No transferred keyword rows were available for BLAST.", prompt.ErrBackToDatabaseSelection)
		}
		return w.runTransferredKeywordBlastAgainstDatabase(ctx, sourceSpecies, cloneKeywordSearchGroups(w.lastKeywordGroups), rows, w.lastKeywordReport, targetSpecies)
	case "blast_rows":
		rows := append([]model.BlastResultRow(nil), w.transferBlastRows...)
		if len(rows) == 0 {
			return w.showInfo("BLAST", "No transferred BLAST result rows were available for BLAST.", prompt.ErrBackToDatabaseSelection)
		}
		return w.runTransferredBlastRowsAgainstDatabase(ctx, sourceSpecies, rows, targetSpecies)
	default:
		return fmt.Errorf("unsupported transferred blast kind %q", transferKind)
	}
}

func (w *BlastWizard) runTransferredKeywordBlastAgainstDatabase(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup, rows []model.KeywordResultRow, reportCtx *keywordReportRunContext, targetSpecies model.SpeciesCandidate) error {
	prepared, err := w.resolveTransferredKeywordRowsToBlastItems(ctx, selected, rows)
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
	return w.runPreparedBlastAgainstDatabase(ctx, selected, groups, reportCtx, prepared, targetSpecies)
}

func (w *BlastWizard) runTransferredBlastRowsAgainstDatabase(ctx context.Context, selected model.SpeciesCandidate, rows []model.BlastResultRow, targetSpecies model.SpeciesCandidate) error {
	prepared, err := w.resolveTransferredBlastRowsToBlastItems(ctx, selected, rows)
	if err != nil {
		return err
	}
	if len(prepared) == 0 {
		return w.showInfo("BLAST", "No selected BLAST result rows could be converted into BLAST queries.", prompt.ErrBackToRowSelection)
	}
	prepared, err = w.prepareKeywordBlastItems(ctx, selected, prepared)
	if err != nil {
		return err
	}
	if len(prepared) == 0 {
		return w.showInfo("BLAST", "No selected BLAST result rows remained after BLAST label handling.", prompt.ErrBackToRowSelection)
	}
	return w.runPreparedBlastAgainstDatabase(ctx, selected, nil, nil, prepared, targetSpecies)
}

func (w *BlastWizard) runPreparedBlastAgainstDatabase(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup, reportCtx *keywordReportRunContext, prepared []blastQueryItem, targetSpecies model.SpeciesCandidate) error {
	if len(groups) > 0 {
		w.lastKeywordGroups = cloneKeywordSearchGroups(groups)
	}
	w.lastKeywordSpecies = selected
	if reportCtx != nil {
		copied := *reportCtx
		w.lastKeywordReport = &copied
	}
	return w.executePreparedBlast(ctx, targetSpecies, prepared, blastRequestConfig{})
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
	config := externalReferenceConfig{
		AutoLabelBlastHits: settings.AutoLabelBlastHits,
		UseUniProt:         settings.UseUniProt,
		UseInterPro:        settings.UseInterPro,
		InterProSettings:   settings.InterProSettings,
	}
	w.lastExternalRefs = config
	return config, nil
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
	w.lastExternalRefs = references

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
		OriginalRunCount:  len(prepared),
	}
	originalRunCount := len(prepared)
	if familyPlan != nil && familyPlan.Settings.Enabled {
		prepared, queryRuns = applyFamilyBlastPlan(prepared, queryRuns, familyPlan)
		if w.lastBlastReviewContext != nil {
			w.lastBlastReviewContext.Prepared = cloneBlastQueryItems(prepared)
			w.lastBlastReviewContext.Runs = cloneBlastQueryRuns(queryRuns)
		}
	}
	if w.prompt != nil {
		w.prompt.QueueBlastResultTableCue()
	}
	return w.reviewBlastRuns(ctx, selected, prepared, queryRuns, configuredRequest, originalRunCount)
}

func (w *BlastWizard) executeConfiguredBlastBatchRuns(ctx context.Context, prepared []blastQueryItem, configuredRequest model.BlastRequest, references externalReferenceConfig) ([]blastQueryRun, error) {
	alignedPrepared, err := w.alignPreparedBlastItemsToRequest(ctx, prepared, configuredRequest)
	if err != nil {
		return nil, err
	}
	prepared = alignedPrepared
	const queryProgressSpan = 100
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
		suppressTaskModals := previousSuppress || update != nil
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
			progressBase := i * queryProgressSpan
			queryProgress := func(current int, message string) {
				if current < 0 {
					current = 0
				}
				if current > queryProgressSpan {
					current = queryProgressSpan
				}
				progress(progressBase+current, message)
			}
			queryCtx := contextWithBlastReferenceConfig(contextWithUpdate(ctx, queryProgress), references)
			label := oneLinePreview(reportQueryLabel(item))
			actionLabel := "Submitting"
			if isLocalBlastRequest(request) {
				actionLabel = "Running local"
			}
			progress(progressBase, fmt.Sprintf("%s BLAST query %d/%d (%s)...", actionLabel, i+1, len(prepared), label))

			for {
				job, effectiveRequest, err := w.submitBlastWithRetry(queryCtx, request)
				request = effectiveRequest
				if errors.Is(err, prompt.ErrBackToBlastProgram) || errors.Is(err, prompt.ErrExitRequested) {
					return blastQueryRun{}, err
				}
				if err != nil {
					return blastQueryRun{}, &blastBatchRunError{Stage: "submit BLAST job", Index: i + 1, Total: len(prepared), Label: label, Err: err}
				}
				if isLocalBlastRequest(request) {
					progress(progressBase+80, fmt.Sprintf("Loading local BLAST results for query %d/%d (%s)...", i+1, len(prepared), label))
				} else {
					progress(progressBase+35, fmt.Sprintf("Waiting for BLAST query %d/%d (%s)...", i+1, len(prepared), label))
				}
				results, err := w.waitForBlastResultsWithRetry(queryCtx, job.JobID)
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
					progress(progressBase+queryProgressSpan, fmt.Sprintf("Finished BLAST query %d/%d (%s).", i+1, len(prepared), label))
					return blastQueryRun{Index: i + 1, Item: item, Request: request, Results: results}, nil
				}
				results.Rows = prepareBlastRowsForReferences(results.Rows, item, request, w.source.Name())
				if references.UseUniProt {
					w.prefetchBlastRowUniProtAccessions(queryCtx, results.Rows)
					enriched, enrichErr := w.enrichBlastRowsWithUniProt(queryCtx, results.Rows)
					if errors.Is(enrichErr, context.Canceled) || errors.Is(enrichErr, tui.ErrTaskCancelled) || errors.Is(enrichErr, prompt.ErrBackToQueryInput) {
						return blastQueryRun{}, enrichErr
					}
					if enrichErr == nil {
						results.Rows = enriched
					}
				}
				if references.UseInterPro {
					enriched, enrichErr := w.enrichBlastRowsWithInterPro(queryCtx, item, results.Rows, references.InterProSettings)
					if errors.Is(enrichErr, context.Canceled) || errors.Is(enrichErr, tui.ErrTaskCancelled) || errors.Is(enrichErr, prompt.ErrBackToQueryInput) {
						return blastQueryRun{}, enrichErr
					}
					if enrichErr == nil {
						results.Rows = enriched
					}
				}
				if references.AutoLabelBlastHits {
					results.Rows = w.autoIdentifyBlastHitLabels(queryCtx, request.Species, item, results.Rows)
				}
				results.Rows = annotateBlastRowsForQueryContext(results.Rows, item)
				progress(progressBase+queryProgressSpan, fmt.Sprintf("Finished BLAST query %d/%d (%s).", i+1, len(prepared), label))
				return blastQueryRun{Index: i + 1, Item: item, Request: request, Results: results}, nil
			}
		}
		if len(prepared) <= 1 {
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
		} else if _, ok := w.source.(*lemna.Client); ok {
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
	if w.suppressTaskModals {
		return run(nil)
	}
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", blastRunTaskPath(len(prepared))),
		Title:       blastRunTaskTitle(len(prepared)),
		Description: batchBlastDescription(configuredRequest),
		Initial:     blastRunTaskInitial(len(prepared)),
		Total:       maxInt(1, len(prepared)*queryProgressSpan),
		CancelError: prompt.ErrBackToQueryInput,
	}, func(taskCtx context.Context, update func(int, string)) ([]blastQueryRun, error) {
		return run(updateWithContext(mergeContexts(ctx, taskCtx), update))
	})
}

func blastRunTaskTitle(queryCount int) string {
	if queryCount <= 1 {
		return "Running BLAST"
	}
	return "Running BLAST batch"
}

func blastRunTaskPath(queryCount int) string {
	if queryCount <= 1 {
		return "Running"
	}
	return "Running batch"
}

func blastRunTaskInitial(queryCount int) string {
	if queryCount <= 1 {
		return "Starting BLAST..."
	}
	return "Starting BLAST batch..."
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
		return "Running local BLAST+ with visible download, database, execution, and result-loading progress."
	}
	return "Submitting BLAST queries and collecting results with visible progress."
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
		if selection.RunBlast {
			if err := w.runBlastRowsBlastMode(ctx, rowContext.Selected, selection.Rows); err != nil {
				if errors.Is(err, prompt.ErrBackToRowSelection) {
					continue
				}
				if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
					return err
				}
				return err
			}
			continue
		}
		if selection.CreateCanvas {
			if err := w.runBlastRowsCanvasMode(ctx, rowContext.Item, selection.Rows); err != nil {
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
		rows := selection.Rows
		if len(rows) == 0 {
			return w.showInfo("BLAST export", "No rows selected for this query. Export will be skipped.", prompt.ErrBackToRowSelection)
		}
		exportItem, err := w.prepareBlastExportItem(rowContext.Item, false)
		if err != nil {
			return err
		}
		showFamilyQueryPrepend, prependOnlyFirstQuery := familyExportQueryPrependOptionForItem(exportItem)
		settings, err := w.prepareExportSettingsWithFamilyOption(buildBlastOutputDisplayName(exportItem), false, true, true, showFamilyQueryPrepend, prependOnlyFirstQuery)
		if err != nil {
			return err
		}
		if showFamilyQueryPrepend {
			exportItem.FamilySettings.PrependOnlyFirstQuery = settings.PrependOnlyFirstQuery
			rowContext.FamilySettings.PrependOnlyFirstQuery = settings.PrependOnlyFirstQuery
			rowContext.Item.FamilySettings.PrependOnlyFirstQuery = settings.PrependOnlyFirstQuery
		}
		outputDir := settings.OutputDir
		displayName := settings.BaseName
		if displayName == "" {
			displayName = buildBlastOutputDisplayName(exportItem)
		}
		filePrefix := sanitizeExportName(displayName)
		for {
			txtHeaderLabel := blastFastaHeaderLabel(exportItem, displayName)
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
	if useSingleBlastRunReview(originalRunCount, runs) {
		return w.reviewSingleBlastRun(ctx, selected, prepared, runs[0], configuredRequest)
	}
	return w.reviewMultiBlastRuns(ctx, selected, prepared, runs, configuredRequest, originalRunCount)
}

func useSingleBlastRunReview(originalRunCount int, runs []blastQueryRun) bool {
	return originalRunCount <= 1 && len(runs) == 1
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
		if selection.RunBlast {
			if err := w.runBlastRowsBlastMode(ctx, selected, selection.Rows); err != nil {
				if errors.Is(err, prompt.ErrBackToRowSelection) {
					continue
				}
				if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
					return err
				}
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

func (w *BlastWizard) reviewMultiBlastRuns(ctx context.Context, selected model.SpeciesCandidate, prepared []blastQueryItem, runs []blastQueryRun, configuredRequest model.BlastRequest, originalRunCount int) error {
	w.warmBlastRunsSequenceCache(ctx, runs)
	for {
		selection, err := w.prompt.SelectBlastRunsWithOptions(blastRunViews(runs), prompt.ErrBackToQueryInput, prompt.BlastRunSelectionOptions{OriginalRunCount: originalRunCount})
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
		if selection.RunBlast {
			if err := w.runBlastRowsBlastMode(ctx, selected, selection.Rows); err != nil {
				if errors.Is(err, prompt.ErrBackToRowSelection) {
					continue
				}
				if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
					return err
				}
				return err
			}
			continue
		}
		if selection.CreateCanvas {
			if err := w.runBlastRunsCanvasMode(ctx, runs, selection.SelectedByRun); err != nil {
				if errors.Is(err, prompt.ErrBackToRowSelection) {
					continue
				}
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
	showFamilyQueryPrepend, prependOnlyFirstQuery := familyExportQueryPrependOptionForItem(exportItem)
	settings, err := w.prepareExportSettingsWithFamilyOption(defaultName, false, allowEmpty, true, showFamilyQueryPrepend, prependOnlyFirstQuery)
	if err != nil {
		return err
	}
	if showFamilyQueryPrepend {
		exportItem.FamilySettings.PrependOnlyFirstQuery = settings.PrependOnlyFirstQuery
		run.Item.FamilySettings.PrependOnlyFirstQuery = settings.PrependOnlyFirstQuery
	}
	displayName := settings.BaseName
	if displayName == "" {
		displayName = defaultName
	}
	filePrefix := sanitizeExportName(displayName)
	txtHeaderLabel := blastFastaHeaderLabel(exportItem, displayName)
	files := exportFileResult{}
	for {
		writerSettings := settings
		writerSettings.WriteSession = false
		files, err = w.exportFamilyBlastSelectionsToDir(ctx, rows, allRows, rowNumbers, filterFlags, exportItemFamilySources(exportItem), displayName, txtHeaderLabel, filePrefix, settings.OutputDir, writerSettings, exportItem.FamilySettings, false)
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
		if settings.WriteSession {
			selectedByRun := [][]bool{append([]bool(nil), selection.Selected...)}
			sessionPath, sessionErr := w.writeBlastSessionSnapshot(selected, []blastQueryItem{exportItem}, []blastQueryRun{run}, configuredRequest, 1, selection.Selected, selectedByRun, filterFlags, [][]bool{append([]bool(nil), filterFlags...)}, selection.FilterSettings, selection.FilterApplied, selection.FilterCleared, settings)
			if sessionErr != nil {
				return sessionErr
			}
			files.SessionPath = sessionPath
		}
		break
	}
	return w.showInfo("Export complete", filesSummary(files), prompt.ErrBackToRowSelection)
}

func (w *BlastWizard) exportAllBlastRuns(ctx context.Context, selected model.SpeciesCandidate, prepared []blastQueryItem, runs []blastQueryRun, rowsByRun [][]model.BlastResultRow, rowNumbersByRun [][]int, filterFlagsByRun [][]bool, selectedByRun [][]bool, configuredRequest model.BlastRequest, filterSettings model.BlastFilterSettings, filterApplied bool, filterCleared bool) error {
	originalRunCount := blastSnapshotOriginalRunCount(w.lastBlastReviewContext, runs)
	settings, err := w.prepareBatchExportSettings(runs)
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
	if settings.WriteSession {
		sessionPath, err := w.writeBlastSessionSnapshot(selected, reportPrepared, reportRuns, configuredRequest, originalRunCount, nil, reportSelectedByRun, nil, reportFilterFlagsByRun, filterSettings, filterApplied, filterCleared, settings)
		if err != nil {
			return err
		}
		exportedFiles = append(exportedFiles, exportFileResult{SessionPath: sessionPath})
	}
	message := fmt.Sprintf("Exported %d BLAST queries to\n%s", len(exportedRuns), settings.OutputDir)
	if settings.WriteSession && len(exportedFiles) > 0 {
		message += "\n\n" + filesSummary(exportedFiles[len(exportedFiles)-1])
	}
	return w.showInfo("Export complete", message, prompt.ErrBackToRowSelection)
}

func blastSnapshotOriginalRunCount(reviewCtx *blastReviewContext, runs []blastQueryRun) int {
	if reviewCtx != nil && reviewCtx.OriginalRunCount > 0 {
		return reviewCtx.OriginalRunCount
	}
	return maxInt(1, len(runs))
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
			if show, _ := familyExportQueryPrependOptionForItem(exportItem); show {
				exportItem.FamilySettings.PrependOnlyFirstQuery = settings.PrependOnlyFirstQuery
				run.Item = exportItem
			}
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
				txtHeaderLabel:   blastFastaHeaderLabel(exportItem, displayName),
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
	id2 := blastQueryItemID2(item)
	sourceName = strings.TrimSpace(sourceName)
	program := canonicalBlastProgram(request.Program)
	queryLength := inferredBlastQueryLength(out, request.Sequence)
	for i := range out {
		out[i].BlastLabelName = label
		out[i].BlastGeneID = id2
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

func blastQueryItemID2(item blastQueryItem) string {
	if item.QuerySource != nil {
		return querySourceID2(item.QuerySource)
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
		if err := w.ensureSymbolNameDatabaseWithUpdate(ctx, func(message string) {
			inheritedUpdate(0, message)
		}, true); err != nil {
			return append([]model.BlastResultRow(nil), rows...)
		}
		return w.autoIdentifyBlastHitLabelsWithProgress(ctx, selected, item, rows, inheritedUpdate)
	}
	if w.suppressTaskModals || w.prompt == nil {
		return w.autoIdentifyBlastHitLabelsWithProgress(ctx, selected, item, rows, nil)
	}
	out, err := tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "Auto identify hit labels"),
		Title:       "Auto identifying BLAST hit symbol names",
		Description: "Resolving BLAST hit row symbol names from the local NCBI Gene library and source metadata.",
		Initial:     "Preparing BLAST hit symbol identification...",
		Total:       maxInt(1, len(rows)),
		CancelError: prompt.ErrBackToQueryInput,
	}, func(taskCtx context.Context, update func(int, string)) ([]model.BlastResultRow, error) {
		taskCtx = mergeContexts(ctx, taskCtx)
		if err := w.ensureSymbolNameDatabaseWithUpdate(taskCtx, func(message string) {
			update(0, message)
		}, true); err != nil {
			return nil, err
		}
		return w.autoIdentifyBlastHitLabelsWithProgress(taskCtx, selected, item, rows, update), nil
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
	progress(0, fmt.Sprintf("Collecting BLAST hit symbol requests for %d rows...", len(rowsNeedingLabel)))
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
		request, labelType, done := blastHitLabelAliasRankRequest(row, sourceLabel, nil, nil, taskTimestamp)
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
			progress(minInt(completed, len(out)), fmt.Sprintf("Resolved BLAST hit symbol names... %d/%d", minInt(completed, len(out)), len(out)))
			continue
		}
		cacheKey := blastHitLabelIdentificationCacheKey(out[i], sourceLabel)
		identification, ok := identificationsByKey[cacheKey]
		if !ok {
			identification = autoIdentifyBlastHitLabelFromKeywordRows(out[i], sourceLabel, nil, nil, taskTimestamp)
			identificationsByKey[cacheKey] = identification
			w.storeBlastHitLabelIdentification(cacheKey, identification)
		}
		out[i].LabelName = identification.Label
		out[i].LabelNameType = identification.LabelType
		out[i].PhgoAliases = strings.Join(identification.Aliases, "; ")
		completed++
		progress(minInt(completed, len(out)), fmt.Sprintf("Resolved BLAST hit symbol names... %d/%d", minInt(completed, len(out)), len(out)))
	}
	if len(out) > 0 {
		progress(len(out), "Finished BLAST hit label identification.")
	}
	return out
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

func autoIdentifyBlastHitLabelFromKeywordRows(row model.BlastResultRow, sourceLabel string, keywordRowsByTerm map[string][]model.KeywordResultRow, lemnaKeywordRowsByTerm map[string][]model.KeywordResultRow, taskTimestamp string) blastHitLabelIdentification {
	request, labelType, done := blastHitLabelAliasRankRequest(row, sourceLabel, keywordRowsByTerm, lemnaKeywordRowsByTerm, taskTimestamp)
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

func blastHitLabelAliasRankRequest(row model.BlastResultRow, sourceLabel string, keywordRowsByTerm map[string][]model.KeywordResultRow, lemnaKeywordRowsByTerm map[string][]model.KeywordResultRow, taskTimestamp string) (labelname.AliasRankRequest, string, bool) {
	taskTimestamp = strings.TrimSpace(taskTimestamp)
	if taskTimestamp == "" {
		taskTimestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return aliasRankRequestFromBlastRow(taskTimestamp, row, blastHitAliasCandidates(row)), "symbolname database", false
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

func blastHitAliasCandidates(row model.BlastResultRow) []string {
	aliases := make([]string, 0, 16)
	aliases = append(aliases, row.LabelName)
	aliases = append(aliases, labelname.SplitAliases(row.PhgoAliases)...)
	aliases = append(aliases, labelname.SplitAliases(row.UniProtGeneNames)...)
	aliases = append(aliases, labelname.SplitAliases(row.UniProtKeywords)...)
	aliases = append(aliases, labelname.SplitAliases(row.UniProtDomain)...)
	aliases = append(aliases, labelname.SplitAliases(row.UniProtRegion)...)
	aliases = append(aliases, labelname.SplitAliases(row.InterProEntryName)...)
	aliases = append(aliases, labelname.AutoDefineCandidates(row.Defline)...)
	aliases = append(aliases, labelname.AutoDefineCandidates(row.UniProtProteinName)...)
	aliases = append(aliases, labelname.AutoDefineCandidates(row.UniProtFunction)...)
	if strings.EqualFold(strings.TrimSpace(row.SourceDatabase), "lemna") {
		aliases = append(aliases, lemnaLocalBlastHitAliasCandidates(row)...)
	}
	if strings.EqualFold(strings.TrimSpace(row.SourceDatabase), "tair") {
		aliases = append(aliases, tairBlastHitAliasCandidates(row)...)
	}
	return uniqueStrings(aliases)
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
		Aliases:     row.UniProtGeneNames,
		AutoDefine:  firstNonEmpty(row.Defline, row.UniProtProteinName),
	})
}

func tairBlastHitAliasCandidates(row model.BlastResultRow) []string {
	aliases := make([]string, 0, 8)
	aliases = append(aliases, row.LabelName)
	aliases = append(aliases, labelname.SplitAliases(row.PhgoAliases)...)
	aliases = append(aliases, labelname.SplitAliases(row.UniProtGeneNames)...)
	aliases = append(aliases, labelname.AutoDefineCandidates(row.Defline)...)
	aliases = append(aliases, labelname.AutoDefineCandidates(row.UniProtProteinName)...)
	return uniqueStrings(aliases)
}

func tairBlastHitDBXrefs(row model.BlastResultRow) []string {
	out := make([]string, 0, 8)
	for _, value := range []string{row.Protein, row.SubjectID, row.TranscriptID, row.SequenceID} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, "TAIR:"+value)
		if gene := stripTranscriptSuffix(value); gene != "" && !strings.EqualFold(gene, value) {
			out = append(out, "TAIR:"+gene)
		}
	}
	return uniqueStrings(out)
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
				row.BlastGeneID = blastQueryItemID2(member)
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
		return nil, false, fmt.Errorf("symbol names are required for BLAST mode")
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
				return nil, fmt.Errorf("could not auto identify symbol names for every BLAST query")
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
		return nil, fmt.Errorf("symbol names are required for BLAST mode")
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
		out = harmonizeAutoIdentifiedBlastLabelsWithLocks(out, lockedLabels)
		if len(items) > 0 && allLabelsPresent(out) {
			notifyaudio.PlayDone()
		}
		return out, nil
	}
	autoIndexes := blastItemsNeedingAutoLabel(items)
	if len(autoIndexes) == 0 {
		return items, nil
	}
	if !w.suppressTaskModals {
		if err := w.ensureSymbolNameDatabase(ctx, prompt.ErrBackToQueryInput); err != nil {
			return nil, err
		}
	}
	run := func(taskCtx context.Context, update func(string)) ([]blastQueryItem, error) {
		taskUpdate := safeTaskUpdate(update)
		labelCtx := mergeContexts(ctx, taskCtx)
		if err := w.ensureSymbolNameDatabaseWithUpdate(labelCtx, update, update != nil || !w.suppressTaskModals); err != nil {
			return nil, err
		}
		out := cloneBlastQueryItems(items)
		taskTimestamp := time.Now().UTC().Format(time.RFC3339Nano)
		lockedLabels := blastAutoIdentifyLockedLabels(out)
		pendingIndexes := make([]int, 0, len(autoIndexes))
		cacheKeys := make(map[int]string, len(autoIndexes))
		for _, idx := range autoIndexes {
			if idx < 0 || idx >= len(out) {
				continue
			}
			cacheKey := w.blastLabelLookupKey(w.source, selected, out[idx])
			cacheKeys[idx] = cacheKey
			if cached, ok := w.cachedBlastLabelLookupByKey(cacheKey); ok {
				applyBlastAutoLabelResultToItem(&out[idx], cached, true)
				continue
			}
			pendingIndexes = append(pendingIndexes, idx)
		}
		workerCount := blastLabelWorkerCount(len(pendingIndexes))
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
					results <- labelResult{
						index:   idx,
						request: w.blastLabelAliasRankRequestForTask(out[idx], taskTimestamp, idx),
					}
				}
			}()
		}
		go func() {
			for _, i := range pendingIndexes {
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
		requests := make([]labelname.AliasRankRequest, 0, len(pendingIndexes))
		order := make([]int, 0, len(pendingIndexes))
		for completed < len(pendingIndexes) {
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
				taskUpdate(fmt.Sprintf("Collecting BLAST source label candidates... %d/%d", completed, len(pendingIndexes)))
			}
		}
		workers.Wait()
		taskUpdate(fmt.Sprintf("Ranking BLAST source labels... %d items", len(requests)))
		ranked := labelname.RankAliasBatch(requests)
		for i, index := range order {
			aliases := ranked[i].RankedAliases
			if index >= 0 && index < len(out) {
				result := blastAutoLabelResult{
					Label:         firstAliasOrEmpty(aliases),
					Aliases:       aliases,
					Request:       requests[i],
					TaskTimestamp: ranked[i].TaskTimestamp,
					ItemIndex:     ranked[i].ItemIndex,
				}
				applyBlastAutoLabelResultToItem(&out[index], result, true)
				w.storeBlastLabelLookupByKey(cacheKeys[index], result)
			}
		}
		out = harmonizeAutoIdentifiedBlastLabelsWithLocks(out, lockedLabels)
		return out, nil
	}
	if w.suppressTaskModals {
		out, err := run(ctx, nil)
		if err == nil && len(items) > 0 && allLabelsPresent(out) {
			notifyaudio.PlayDone()
		}
		return out, err
	}
	out, err := tui.RunTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "Auto identify"),
		Title:       "Auto identifying BLAST symbol names",
		Description: "Ranking BLAST query labels through the local symbol name database.",
		Initial:     "Auto identifying BLAST symbol names...",
		CancelError: prompt.ErrBackToQueryInput,
	}, run)
	if err == nil && len(items) > 0 && allLabelsPresent(out) {
		notifyaudio.PlayDone()
	}
	return out, err
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
	if !w.suppressTaskModals {
		if err := w.ensureSymbolNameDatabase(ctx, prompt.ErrBackToRowSelection); err != nil {
			return nil, err
		}
	}
	run := func(taskCtx context.Context, update func(string)) ([]blastQueryItem, error) {
		if err := w.ensureSymbolNameDatabaseWithUpdate(mergeContexts(ctx, taskCtx), update, update != nil || !w.suppressTaskModals); err != nil {
			return nil, err
		}
		return w.supplementBlastAliases(ctx, taskCtx, nil, selected, items, safeTaskUpdate(update))
	}
	if w.suppressTaskModals {
		return run(ctx, nil)
	}
	return tui.RunTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "Alias labels"),
		Title:       "Reading BLAST alias symbol names",
		Description: "Reading source-species aliases while preserving existing BLAST query symbol names.",
		Initial:     "Reading BLAST alias symbol names...",
		CancelError: prompt.ErrBackToQueryInput,
	}, run)
}

func (w *BlastWizard) supplementBlastAliases(ctx context.Context, taskCtx context.Context, phytozomeSource source.DataSource, selected model.SpeciesCandidate, items []blastQueryItem, update func(string)) ([]blastQueryItem, error) {
	labelCtx := mergeContexts(ctx, taskCtx)
	if err := w.ensureSymbolNameDatabaseWithUpdate(labelCtx, update, update != nil); err != nil {
		return nil, err
	}
	out := cloneBlastQueryItems(items)
	aliasIndexes := blastItemsNeedingAliasSupplement(out)
	if len(aliasIndexes) == 0 {
		return out, nil
	}
	cacheSource := w.source
	if phytozomeSource != nil {
		cacheSource = phytozomeSource
	}
	pendingIndexes := make([]int, 0, len(aliasIndexes))
	cacheKeys := make(map[int]string, len(aliasIndexes))
	for _, idx := range aliasIndexes {
		if idx < 0 || idx >= len(out) {
			continue
		}
		cacheKey := w.blastLabelLookupKey(cacheSource, selected, out[idx])
		cacheKeys[idx] = cacheKey
		if cached, ok := w.cachedBlastLabelLookupByKey(cacheKey); ok {
			applyBlastAutoLabelResultToItem(&out[idx], cached, false)
			continue
		}
		pendingIndexes = append(pendingIndexes, idx)
	}
	if len(pendingIndexes) == 0 {
		return out, nil
	}
	taskTimestamp := time.Now().UTC().Format(time.RFC3339Nano)
	workerCount := blastLabelWorkerCount(len(pendingIndexes))
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
				results <- aliasResult{
					index:   idx,
					request: w.blastLabelAliasRankRequestForTask(out[idx], taskTimestamp, idx),
				}
			}
		}()
	}
	go func() {
		for _, i := range pendingIndexes {
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
	requests := make([]labelname.AliasRankRequest, 0, len(pendingIndexes))
	order := make([]int, 0, len(pendingIndexes))
	for completed < len(pendingIndexes) {
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
				update(fmt.Sprintf("Collecting BLAST aliases... %d/%d", completed, len(pendingIndexes)))
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
			result := blastAutoLabelResult{
				Label:         firstAliasOrEmpty(ranked[i].RankedAliases),
				Aliases:       ranked[i].RankedAliases,
				Request:       requests[i],
				TaskTimestamp: ranked[i].TaskTimestamp,
				ItemIndex:     ranked[i].ItemIndex,
			}
			applyBlastAutoLabelResultToItem(&out[index], result, false)
			if strings.TrimSpace(items[index].LabelName) == "" {
				w.storeBlastLabelLookupByKey(cacheKeys[index], result)
			}
		}
	}
	return out, nil
}

type blastAutoLabelResult struct {
	Label         string
	Aliases       []string
	Request       labelname.AliasRankRequest
	TaskTimestamp string
	ItemIndex     int
}

func applyBlastAutoLabelResultToItem(item *blastQueryItem, result blastAutoLabelResult, setLabel bool) {
	if item == nil {
		return
	}
	aliases := uniqueStrings(result.Aliases)
	if setLabel {
		setBlastQueryItemLabel(item, firstNonEmpty(item.LabelName, result.Label, firstAliasOrEmpty(aliases)))
	}
	mergeBlastQueryItemAliases(item, aliases)
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

func (w *BlastWizard) autoIdentifyBlastLabel(ctx context.Context, phytozomeSource source.DataSource, selected model.SpeciesCandidate, item blastQueryItem) string {
	return w.autoIdentifyBlastLabelResult(ctx, phytozomeSource, selected, item).Label
}

func (w *BlastWizard) autoIdentifyBlastLabelResult(ctx context.Context, phytozomeSource source.DataSource, selected model.SpeciesCandidate, item blastQueryItem) blastAutoLabelResult {
	return w.autoIdentifyBlastLabelResultForTask(ctx, phytozomeSource, selected, item, time.Now().UTC().Format(time.RFC3339Nano), 0)
}

func (w *BlastWizard) autoIdentifyBlastLabelResultForTask(ctx context.Context, phytozomeSource source.DataSource, selected model.SpeciesCandidate, item blastQueryItem, taskTimestamp string, itemIndex int) blastAutoLabelResult {
	cacheKey := w.blastLabelLookupKey(phytozomeSource, selected, item)
	pinnedLabel := strings.TrimSpace(item.LabelName)
	if cached, ok := w.cachedBlastLabelLookupByKey(cacheKey); ok {
		if pinnedLabel != "" {
			cached.Label = pinnedLabel
			cached.Aliases = uniqueStrings(append([]string{pinnedLabel}, cached.Aliases...))
		}
		return cached
	}
	request := w.blastLabelAliasRankRequestForTask(item, taskTimestamp, itemIndex)
	ranked := labelname.RankAliases(request)
	label := ""
	if pinnedLabel != "" {
		label = pinnedLabel
		ranked.RankedAliases = uniqueStrings(append([]string{pinnedLabel}, ranked.RankedAliases...))
	} else if len(ranked.RankedAliases) > 0 {
		label = ranked.RankedAliases[0]
	}
	request.Aliases = ranked.RankedAliases
	result := blastAutoLabelResult{
		Label:         label,
		Aliases:       ranked.RankedAliases,
		Request:       request,
		TaskTimestamp: ranked.TaskTimestamp,
		ItemIndex:     ranked.ItemIndex,
	}
	if pinnedLabel == "" {
		w.storeBlastLabelLookupByKey(cacheKey, result)
	}
	return result
}

func (w *BlastWizard) blastLabelAliasRankRequestForTask(item blastQueryItem, taskTimestamp string, itemIndex int) labelname.AliasRankRequest {
	aliases := make([]string, 0, 16)
	pinnedLabel := strings.TrimSpace(item.LabelName)
	aliases = append(aliases, pinnedLabel)
	aliases = append(aliases, collectBlastItemAliasCandidates(item)...)
	aliases = uniqueStrings(aliases)
	request := aliasRankRequestFromBlastItem(taskTimestamp, itemIndex, item, aliases)
	excludeBlastItemHeaderLabelsFromAliasRankRequest(item, &request)
	return request
}

func excludeBlastItemHeaderLabelsFromAliasRankRequest(item blastQueryItem, request *labelname.AliasRankRequest) {
	if request == nil {
		return
	}
	headerLabels := map[string]struct{}{}
	if header, _ := splitFastaHeaderAndSequence(item.RawInput); header != "" {
		if parsed, ok := parsePhgoFastaHeader(header); ok {
			if key := strings.ToLower(strings.TrimSpace(parsed.LabelName)); key != "" {
				headerLabels[key] = struct{}{}
			}
		}
		if label := labelname.FastaHeaderLabelNameFromInput(item.RawInput); label != "" {
			headerLabels[strings.ToLower(strings.TrimSpace(label))] = struct{}{}
		}
	}
	if len(headerLabels) == 0 {
		return
	}
	clearIfHeaderLabel := func(value *string) {
		if value == nil {
			return
		}
		if _, ok := headerLabels[strings.ToLower(strings.TrimSpace(*value))]; ok {
			*value = ""
		}
	}
	clearIfHeaderLabel(&request.Symbol)
	clearIfHeaderLabel(&request.SymbolAuthority)
}

func aliasRankRequestFromBlastItem(taskTimestamp string, itemIndex int, item blastQueryItem, aliases []string) labelname.AliasRankRequest {
	request := labelname.AliasRankRequest{
		TaskTimestamp: taskTimestamp,
		ItemIndex:     itemIndex,
		SearchTerm:    strings.Join(blastLabelSearchTerms(item), "; "),
		Aliases:       uniqueStrings(aliases),
	}
	if item.QuerySource == nil {
		return request
	}
	source := item.QuerySource
	if taxID := symbolNameTaxIDForSourceDatabase(source.SourceDatabase, source.SourceGenomeLabel, source.OrganismShort); taxID != "" {
		request.TaxID = taxID
	}
	request.Symbol = strings.TrimSpace(source.LabelName)
	request.ProteinID = strings.TrimSpace(source.ProteinID)
	request.GeneID = strings.TrimSpace(source.GeneID)
	request.TranscriptID = strings.TrimSpace(source.TranscriptID)
	request.SequenceID = strings.TrimSpace(source.PreferredSequenceID)
	request.Synonyms = labelname.SplitAliases(source.Synonyms)
	request.DBXrefs = compactStrings(source.UniProtAccession, source.OriginalInputURL, source.NormalizedURL)
	request.DBXrefs = append(request.DBXrefs, querySourceDBXrefCandidates(source)...)
	request.Description = strings.TrimSpace(firstNonEmpty(source.Annotation, source.AutoDefine))
	request.SymbolAuthority = strings.TrimSpace(source.LabelName)
	request.FullNameAuthority = strings.TrimSpace(source.Annotation)
	request.OtherDesignations = append(labelname.SplitAliases(source.Aliases), labelname.SplitAliases(source.PhgoAliases)...)
	request.DBXrefs = uniqueStrings(request.DBXrefs)
	return request
}

func aliasRankRequestFromBlastRow(taskTimestamp string, row model.BlastResultRow, aliases []string) labelname.AliasRankRequest {
	request := labelname.AliasRankRequest{
		TaskTimestamp:     taskTimestamp,
		SearchTerm:        strings.Join(blastHitLabelSearchTerms(row), "; "),
		Symbol:            strings.TrimSpace(row.LabelName),
		ProteinID:         strings.TrimSpace(firstNonEmpty(row.Protein, row.SubjectID)),
		GeneID:            strings.TrimSpace(row.SubjectID),
		TranscriptID:      strings.TrimSpace(row.TranscriptID),
		SequenceID:        strings.TrimSpace(row.SequenceID),
		Aliases:           uniqueStrings(aliases),
		Synonyms:          labelname.SplitAliases(row.UniProtGeneNames),
		DBXrefs:           compactStrings(row.UniProtAccession, row.GeneReportURL, row.InterProAccessions, row.InterProSignatureAccessions, row.InterProPfamAccessions),
		Description:       strings.TrimSpace(firstNonEmpty(row.Defline, row.UniProtProteinName, row.UniProtFunction)),
		FullNameAuthority: strings.TrimSpace(row.UniProtProteinName),
		OtherDesignations: labelname.SplitAliases(strings.Join(compactStrings(row.UniProtKeywords, row.UniProtDomain, row.UniProtRegion, row.InterProEntryName), "; ")),
		FeatureType:       strings.TrimSpace(row.InterProEntryType),
	}
	if strings.EqualFold(strings.TrimSpace(row.SourceDatabase), "tair") {
		request.TaxID = "3702"
		request.GeneID = firstNonEmpty(stripTranscriptSuffix(firstNonEmpty(row.TranscriptID, row.SequenceID, row.Protein, row.SubjectID)), request.GeneID)
		request.DBXrefs = append(request.DBXrefs, tairBlastHitDBXrefs(row)...)
	}
	request.DBXrefs = uniqueStrings(request.DBXrefs)
	return request
}

func aliasRankRequestFromKeywordRows(taskTimestamp string, aliases []string, rows []model.KeywordResultRow) labelname.AliasRankRequest {
	request := labelname.AliasRankRequest{
		TaskTimestamp: taskTimestamp,
		Aliases:       uniqueStrings(aliases),
	}
	for _, row := range rows {
		if taxID := symbolNameTaxIDForSourceDatabase(row.SourceDatabase, row.Genome, row.SequenceHeaderLabel); taxID != "" {
			request.TaxID = firstNonEmpty(request.TaxID, taxID)
		}
		request.SearchTerm = firstNonEmpty(request.SearchTerm, row.SearchTerm)
		request.Symbol = firstNonEmpty(request.Symbol, row.LabelName, labelname.FirstAlias(row.Symbols))
		request.ProteinID = firstNonEmpty(request.ProteinID, row.ProteinID)
		request.GeneID = firstNonEmpty(request.GeneID, row.GeneIdentifier, row.GeneLocus)
		request.TranscriptID = firstNonEmpty(request.TranscriptID, row.TranscriptID)
		request.SequenceID = firstNonEmpty(request.SequenceID, row.SequenceID)
		request.Synonyms = append(request.Synonyms, labelname.SplitAliases(row.Synonyms)...)
		request.Synonyms = append(request.Synonyms, labelname.SplitAliases(row.Aliases)...)
		request.DBXrefs = append(request.DBXrefs, compactStrings(row.UniProt, row.GeneReportURL)...)
		request.DBXrefs = append(request.DBXrefs, keywordRowDBXrefCandidates(row)...)
		request.Description = firstNonEmpty(request.Description, row.Description, row.Comments, row.AutoDefine)
		request.FullNameAuthority = firstNonEmpty(request.FullNameAuthority, row.Description)
		request.OtherDesignations = append(request.OtherDesignations, labelname.AutoDefineCandidates(row.AutoDefine)...)
		request.OtherDesignations = append(request.OtherDesignations, labelname.AutoDefineCandidates(row.Description)...)
		request.OtherDesignations = append(request.OtherDesignations, labelname.AutoDefineCandidates(row.Comments)...)
	}
	request.Synonyms = uniqueStrings(request.Synonyms)
	request.DBXrefs = uniqueStrings(request.DBXrefs)
	request.OtherDesignations = uniqueStrings(request.OtherDesignations)
	return request
}

func keywordRowDBXrefCandidates(row model.KeywordResultRow) []string {
	out := make([]string, 0, 8)
	if strings.EqualFold(strings.TrimSpace(row.SourceDatabase), "tair") {
		for _, value := range []string{row.GeneIdentifier, row.TranscriptID, row.ProteinID, row.SequenceID} {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			out = append(out, "TAIR:"+value)
			if gene := stripTranscriptSuffix(value); gene != "" && !strings.EqualFold(gene, value) {
				out = append(out, "TAIR:"+gene)
			}
		}
	}
	return uniqueStrings(out)
}

func querySourceDBXrefCandidates(source *model.QuerySequenceSource) []string {
	if source == nil {
		return nil
	}
	out := make([]string, 0, 8)
	if strings.EqualFold(strings.TrimSpace(source.SourceDatabase), "tair") {
		for _, value := range []string{source.GeneID, source.TranscriptID, source.ProteinID, source.PreferredSequenceID} {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			out = append(out, "TAIR:"+value)
			if gene := stripTranscriptSuffix(value); gene != "" && !strings.EqualFold(gene, value) {
				out = append(out, "TAIR:"+gene)
			}
		}
	}
	return uniqueStrings(out)
}

func symbolNameTaxIDForSourceDatabase(database string, values ...string) string {
	db := strings.ToLower(strings.TrimSpace(database))
	text := strings.ToLower(strings.Join(values, " "))
	switch {
	case db == "tair":
		return "3702"
	case strings.Contains(text, "arabidopsis thaliana") || strings.Contains(text, "athaliana") || strings.Contains(text, "tair"):
		return "3702"
	default:
		return ""
	}
}

func compactStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func collectBlastItemAliasCandidates(item blastQueryItem) []string {
	aliases := make([]string, 0, 12)
	if item.QuerySource != nil {
		aliases = append(aliases, querySourceLabelnameCandidates(item.QuerySource)...)
	}
	aliases = excludeBlastItemHeaderLabelsFromAliases(item, aliases)
	aliases = uniqueStrings(aliases)
	if len(aliases) == 0 && item.QuerySource != nil {
		aliases = append(aliases, labelname.SplitAliases(item.QuerySource.PhgoAliases)...)
	}
	return uniqueStrings(aliases)
}

func excludeBlastItemHeaderLabelsFromAliases(item blastQueryItem, aliases []string) []string {
	if len(aliases) == 0 {
		return nil
	}
	headerLabels := map[string]struct{}{}
	if header, _ := splitFastaHeaderAndSequence(item.RawInput); header != "" {
		if parsed, ok := parsePhgoFastaHeader(header); ok {
			if key := strings.ToLower(strings.TrimSpace(parsed.LabelName)); key != "" {
				headerLabels[key] = struct{}{}
			}
		}
		if label := labelname.FastaHeaderLabelNameFromInput(item.RawInput); label != "" {
			headerLabels[strings.ToLower(strings.TrimSpace(label))] = struct{}{}
		}
	}
	if len(headerLabels) == 0 {
		return aliases
	}
	out := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		if _, skip := headerLabels[strings.ToLower(strings.TrimSpace(alias))]; skip {
			continue
		}
		out = append(out, alias)
	}
	return out
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

func (w *BlastWizard) prepareExportSettings(defaultBaseName string, allowFolder bool, allowEmptyFileName bool, mentionBlastHeaderFallback bool) (exportSettings, error) {
	return w.prepareExportSettingsWithFamilyOption(defaultBaseName, allowFolder, allowEmptyFileName, mentionBlastHeaderFallback, false, false)
}

func (w *BlastWizard) prepareExportSettingsWithFamilyOption(defaultBaseName string, allowFolder bool, allowEmptyFileName bool, mentionBlastHeaderFallback bool, showFamilyQueryPrepend bool, prependOnlyFirstQuery bool) (exportSettings, error) {
	defaultOutputDir, err := appfs.OutputDir()
	if err != nil {
		return exportSettings{}, err
	}
	settings, err := w.prompt.ExportSettingsWithOptions("File name", allowFolder, allowEmptyFileName, mentionBlastHeaderFallback, prompt.ErrBackToRowSelection, prompt.ExportSettingsOptions{
		ShowFamilyQueryPrepend: showFamilyQueryPrepend,
		PrependOnlyFirstQuery:  prependOnlyFirstQuery,
	})
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
	outputDir, err := w.selectExportOutputDir(defaultOutputDir)
	if err != nil {
		return exportSettings{}, err
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

func (w *BlastWizard) prepareBatchExportSettings(runs []blastQueryRun) (exportSettings, error) {
	defaultOutputDir, err := appfs.OutputDir()
	if err != nil {
		return exportSettings{}, err
	}
	showFamilyQueryPrepend, prependOnlyFirstQuery := familyExportQueryPrependOption(runs)
	settings, err := w.prompt.ExportSettingsWithOptions("Output folder", true, true, false, prompt.ErrBackToRowSelection, prompt.ExportSettingsOptions{
		ShowFamilyQueryPrepend: showFamilyQueryPrepend,
		PrependOnlyFirstQuery:  prependOnlyFirstQuery,
	})
	if err != nil {
		return exportSettings{}, err
	}
	outputDir, err := w.selectExportOutputDir(defaultOutputDir)
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

func (w *BlastWizard) selectExportOutputDir(defaultOutputDir string) (string, error) {
	if w.suppressTaskModals {
		return defaultOutputDir, nil
	}
	dir, err := appfs.SelectFolder("Select export folder", defaultOutputDir)
	if err != nil {
		if errors.Is(err, appfs.ErrFolderSelectionCancelled) {
			return "", prompt.ErrBackToRowSelection
		}
		return "", err
	}
	return dir, nil
}

func exportSettingsFromPrompt(settings prompt.ExportSettings, baseName string, outputDir string) exportSettings {
	headerMode := model.NormalizeFastaHeaderMode(settings.FastaHeaderMode, settings.UsePhgoHeader)
	return exportSettings{
		BaseName:              baseName,
		OutputDir:             outputDir,
		WriteReport:           settings.WriteReport,
		WriteSession:          settings.WriteSession,
		WriteText:             settings.WriteText,
		WriteConvertedFasta:   settings.WriteConvertedFasta,
		WriteAllRows:          settings.WriteAllRows,
		WriteExcel:            settings.WriteExcel,
		WriteRawExcel:         settings.WriteRawExcel,
		FastaHeaderMode:       headerMode,
		UsePhgoHeader:         headerMode == model.FastaHeaderModePhgo,
		PrependOnlyFirstQuery: settings.PrependOnlyFirstQuery,
		TreeSettings:          phylo.DefaultTreeSettings(),
	}
}

func (s exportSettings) fastaHeaderMode() model.FastaHeaderMode {
	return model.NormalizeFastaHeaderMode(s.FastaHeaderMode, s.UsePhgoHeader)
}

func fastaHeaderModeDisplay(settings exportSettings) string {
	switch settings.fastaHeaderMode() {
	case model.FastaHeaderModePhgoLite:
		return "PHgo Lite: species|ID2(symbol)"
	case model.FastaHeaderModeOriginal:
		return "original FASTA header"
	case model.FastaHeaderModeMinimal:
		return "minimal primary ID only"
	default:
		return "phgo FASTA header"
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
		if strings.TrimSpace(item.RawInput) == "" && strings.TrimSpace(item.LabelName) == "" && item.QuerySource == nil && strings.TrimSpace(item.Sequence) == "" {
			continue
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
		if fastautil.IsIgnoredPHGONoteHeader(record) {
			return blastQueryItem{}, nil
		}
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

func blastFastaHeaderLabel(item blastQueryItem, fileBaseName string) string {
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

func familyExportQueryPrependOptionForItem(item blastQueryItem) (bool, bool) {
	if !item.FamilySettings.Enabled {
		return false, false
	}
	return true, item.FamilySettings.PrependOnlyFirstQuery
}

func familyExportQueryPrependOption(runs []blastQueryRun) (bool, bool) {
	for _, run := range runs {
		if show, initial := familyExportQueryPrependOptionForItem(run.Item); show {
			return true, initial
		}
	}
	return false, false
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

func keywordSearchWorkerCountForSource(src source.DataSource, species model.SpeciesCandidate, total int) int {
	if isTAIR12ENASpecies(src, species) {
		if configured := configuredInt("PHGO_TAIR_ENA_KEYWORD_WORKERS", 0); configured > 0 {
			return boundedWorkerCount(total, configured)
		}
		return boundedWorkerCount(total, 96)
	}
	if isTAIRReleaseSpecies(src, species) {
		if configured := configuredInt("PHGO_TAIR_RELEASE_KEYWORD_WORKERS", 0); configured > 0 {
			return boundedWorkerCount(total, configured)
		}
		return boundedWorkerCount(total, 128)
	}
	return keywordSearchWorkerCount(total)
}

func isTAIR12ENASpecies(src source.DataSource, species model.SpeciesCandidate) bool {
	if !isTAIRSource(src) {
		return false
	}
	for _, value := range []string{species.JBrowseName, species.GenomeLabel, species.SearchAlias} {
		if strings.Contains(strings.ToUpper(strings.TrimSpace(value)), "TAIR12") {
			return true
		}
	}
	return false
}

func isTAIRReleaseSpecies(src source.DataSource, species model.SpeciesCandidate) bool {
	if !isTAIRSource(src) {
		return false
	}
	for _, value := range []string{species.JBrowseName, species.GenomeLabel, species.SearchAlias} {
		upper := strings.ToUpper(strings.TrimSpace(value))
		if strings.Contains(upper, "TAIR7") ||
			strings.Contains(upper, "TAIR8") ||
			strings.Contains(upper, "TAIR9") ||
			strings.Contains(upper, "TAIR10") ||
			strings.Contains(upper, "TAIR11") ||
			strings.Contains(upper, "ARAPORT11") {
			return true
		}
	}
	return false
}

func isTAIRSource(src source.DataSource) bool {
	return src != nil && strings.EqualFold(sourceDatabaseName(src), "tair")
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
				rawRecords = applyBlastHeaderMode(rawRecords, allRows, []*model.QuerySequenceSource{querySource}, len(rawRecords)-len(hitRecords), settings.fastaHeaderMode())
				steps = append(steps, keywordReportStep("Prepend query sequence record to raw FASTA", prependStart, time.Now(), "ok", blastQueryPrependStepDetails(querySource, rawRecords, hitRecords)))
				writeStart := time.Now()
				if err := export.WriteProteinSequencesText(rawTextPath, rawRecords); err != nil {
					return append(steps, keywordReportStep("Write raw BLAST peptide FASTA", writeStart, time.Now(), "failed", err.Error())), err
				}
				return append(steps, keywordReportStep("Write raw BLAST peptide FASTA", writeStart, time.Now(), "ok", fmt.Sprintf("%d sequence records written", len(rawRecords)))), nil
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
		records = applyBlastHeaderMode(records, selectedRows, []*model.QuerySequenceSource{querySource}, len(records)-len(hitRecords), settings.fastaHeaderMode())
		files.Steps = append(files.Steps, keywordReportStep("Prepend query sequence record", prependStart, time.Now(), "ok", blastQueryPrependStepDetails(querySource, records, hitRecords)))
		writeText := func() error {
			return export.WriteProteinSequencesText(textPath, records)
		}
		var err error
		stepStart := time.Now()
		if w.suppressTaskModals {
			err = writeText()
		} else {
			err = withSpinner(w.out, "Writing peptide FASTA file...", writeText)
		}
		if err != nil {
			files.Steps = append(files.Steps, keywordReportStep("Write BLAST peptide FASTA", stepStart, time.Now(), "failed", err.Error()))
			return exportFileResult{}, err
		}
		files.Steps = append(files.Steps, keywordReportStep("Write BLAST peptide FASTA", stepStart, time.Now(), "ok", fmt.Sprintf("%d sequence records written", len(records))))
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
	familySettings.PrependOnlyFirstQuery = settings.PrependOnlyFirstQuery
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
				prependedSources := familyFastaQuerySources(querySources, familySettings)
				rawRecords, prependedQueries = prependFamilyQuerySequenceRecords(rawRecords, querySources, txtHeaderLabel, familySettings)
				rawRecords = applyBlastHeaderMode(rawRecords, allRows, prependedSources, prependedQueries, settings.fastaHeaderMode())
				steps = append(steps, keywordReportStep("Prepend Family BLAST query sequence records to raw FASTA", prependStart, time.Now(), "ok", familyQueryPrependStepDetails(prependedQueries, len(querySources), familySettings.PrependOnlyFirstQuery, len(hitRecords))))
				writeStart := time.Now()
				if err := export.WriteProteinSequencesText(rawTextPath, rawRecords); err != nil {
					return append(steps, keywordReportStep("Write raw Family BLAST peptide FASTA", writeStart, time.Now(), "failed", err.Error())), err
				}
				return append(steps, keywordReportStep("Write raw Family BLAST peptide FASTA", writeStart, time.Now(), "ok", fmt.Sprintf("%d sequence records written", len(rawRecords)))), nil
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
		prependedSources := familyFastaQuerySources(querySources, familySettings)
		records, prependedQueries = prependFamilyQuerySequenceRecords(records, querySources, txtHeaderLabel, familySettings)
		records = applyBlastHeaderMode(records, selectedRows, prependedSources, prependedQueries, settings.fastaHeaderMode())
		files.Steps = append(files.Steps, keywordReportStep("Prepend Family BLAST query sequence records", prependStart, time.Now(), "ok", familyQueryPrependStepDetails(prependedQueries, len(querySources), familySettings.PrependOnlyFirstQuery, len(hitRecords))))
		writeText := func() error {
			return export.WriteProteinSequencesText(textPath, records)
		}
		var writeErr error
		stepStart := time.Now()
		if w.suppressTaskModals {
			writeErr = writeText()
		} else {
			writeErr = withSpinner(w.out, "Writing peptide FASTA file...", writeText)
		}
		if writeErr != nil {
			files.Steps = append(files.Steps, keywordReportStep("Write Family BLAST peptide FASTA", stepStart, time.Now(), "failed", writeErr.Error()))
			return exportFileResult{}, writeErr
		}
		files.Steps = append(files.Steps, keywordReportStep("Write Family BLAST peptide FASTA", stepStart, time.Now(), "ok", fmt.Sprintf("%d sequence records written", len(records))))
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

func familyFastaQueryIndexes(querySources []*model.QuerySequenceSource, settings model.FamilyBlastSettings) []int {
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

func familyFastaQuerySources(querySources []*model.QuerySequenceSource, settings model.FamilyBlastSettings) []*model.QuerySequenceSource {
	indexes := familyFastaQueryIndexes(querySources, settings)
	out := make([]*model.QuerySequenceSource, 0, len(indexes))
	for _, index := range indexes {
		if index >= 0 && index < len(querySources) && querySources[index] != nil {
			out = append(out, querySources[index])
		}
	}
	return out
}

func familyFastaHeaderLabel(source *model.QuerySequenceSource, fallback string) string {
	if source == nil {
		return strings.TrimSpace(fallback)
	}
	for _, value := range []string{
		strings.TrimSpace(source.LabelName),
		labelname.FirstAlias(source.Aliases),
		querySourceID2(source),
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
		return "family FASTA export prepends only the first family member query header; hit records append selected row label_name"
	}
	return "family FASTA export prepends all family member query headers in run order; hit records append selected row label_name"
}

func prependFamilyQuerySequenceRecords(records []model.ProteinSequenceRecord, querySources []*model.QuerySequenceSource, fallback string, familySettings model.FamilyBlastSettings) ([]model.ProteinSequenceRecord, int) {
	prependedSources := familyFastaQuerySources(querySources, familySettings)
	for i := len(prependedSources) - 1; i >= 0; i-- {
		source := prependedSources[i]
		if source == nil {
			continue
		}
		headerLabel := familyFastaHeaderLabel(source, fallback)
		records = prependQuerySequenceRecord(records, source, headerLabel)
	}
	return records, len(prependedSources)
}

func (w *BlastWizard) exportBlastExcelAndFetchRecords(ctx context.Context, rows []model.BlastResultRow, rowNumbers []int, filterFlags []bool, excelPath string, metadata *model.ExportMetadata) ([]model.ProteinSequenceRecord, error) {
	if w.suppressTaskModals {
		return w.exportBlastExcelAndFetchRecordsSilent(ctx, rows, rowNumbers, filterFlags, excelPath, metadata)
	}
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Export", "Writing files"),
		Title:       "Writing BLAST export files",
		Description: "Writing the Excel file while fetching peptide sequences for the FASTA export.",
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
		Title:       "Preparing BLAST FASTA export",
		Description: "Fetching peptide sequences for the FASTA export.",
		Initial:     "Starting FASTA export...",
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
		lines = append(lines, "FASTA\n"+files.TextPath)
	}
	if strings.TrimSpace(files.ExcelPath) != "" {
		lines = append(lines, "Excel\n"+files.ExcelPath)
	}
	if strings.TrimSpace(files.RawExcelPath) != "" {
		lines = append(lines, "Raw Excel\n"+files.RawExcelPath)
	}
	if strings.TrimSpace(files.RawTextPath) != "" {
		lines = append(lines, "Raw FASTA\n"+files.RawTextPath)
	}
	if strings.TrimSpace(files.ReportPath) != "" {
		lines = append(lines, "Data analysis report (PDF)\n"+files.ReportPath)
	}
	if strings.TrimSpace(files.SessionPath) != "" {
		lines = append(lines, "Session snapshot (.pgo)\n"+files.SessionPath)
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

func (w *BlastWizard) submitBlastWithRetry(ctx context.Context, request model.BlastRequest) (model.BlastJob, model.BlastRequest, error) {
	for {
		job, err := w.submitBlastOnce(ctx, request)
		if err == nil {
			return job, request, nil
		}
		var missingTools *blastplus.MissingToolsError
		if errors.As(err, &missingTools) {
			return model.BlastJob{}, request, err
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
		if w.suppressTaskModals {
			return model.BlastJob{}, request, err
		}
		action, actionErr := w.prompt.BlastSubmitErrorAction(fmt.Sprintf("submit BLAST job: %v", err))
		if actionErr != nil {
			return model.BlastJob{}, request, actionErr
		}
		decision, navErr := interpretRecoveryAction(action, prompt.ErrBackToQueryInput, false)
		if navErr != nil {
			return model.BlastJob{}, request, navErr
		}
		switch decision {
		case recoveryRetry:
			continue
		default:
			return model.BlastJob{}, request, prompt.ErrBackToQueryInput
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
			return lc.SubmitBlast(ctx, request)
		}
		return w.source.SubmitBlast(ctx, request)
	}
	if lc, ok := w.source.(*lemna.Client); ok {
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

func (w *BlastWizard) exportKeywordSelectionsWithRetry(ctx context.Context, selected model.SpeciesCandidate, rows []model.KeywordResultRow, allRows []model.KeywordResultRow, groups []model.KeywordSearchGroup, baseName string, outputDir string, settings exportSettings, selectedMask []bool, mode QueryMode, reportCtx *keywordReportRunContext) error {
	for {
		err := w.exportKeywordSelections(ctx, selected, rows, allRows, groups, baseName, outputDir, settings, selectedMask, mode, reportCtx)
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
	return w.prepareAndExportKeywordSelectionWithMask(ctx, selected, groups, rows, nil, ModeKeyword, reportCtx)
}

func (w *BlastWizard) prepareAndExportKeywordSelectionWithMask(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup, rows []model.KeywordResultRow, selectedMask []bool, mode QueryMode, reportCtx *keywordReportRunContext) error {
	exportRows := append([]model.KeywordResultRow(nil), rows...)
	settings, err := w.prepareExportSettings(defaultKeywordExportLabel(exportRows, groups), false, true, false)
	if err != nil {
		return err
	}
	baseName := settings.BaseName
	if err := w.exportKeywordSelectionsWithRetry(ctx, selected, exportRows, flattenKeywordSearchGroups(groups), groups, baseName, settings.OutputDir, settings, selectedMask, mode, reportCtx); err != nil {
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
		AllowHome:   isRootInstanceID(w.instanceID),
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
	return autoIdentifyKeywordLabelIdentificationsWithSourceType(groups, "symbolname database")
}

func autoIdentifyKeywordLabelIdentificationsWithSourceType(groups []model.KeywordSearchGroup, sourceType string) []keywordLabelIdentification {
	taskTimestamp := keywordLabelTaskTimestamp(groups)
	directRequests := make([]labelname.AliasRankRequest, 0, len(groups))
	directIndexes := make([]int, 0, len(groups))
	for i, group := range groups {
		request := aliasRankRequestFromKeywordSearchTerm(taskTimestamp, i, group)
		if aliasRankRequestHasLocalTerms(request) {
			directIndexes = append(directIndexes, i)
			directRequests = append(directRequests, request)
		}
	}
	results := make([]labelname.AliasRankResult, len(groups))
	for i := range results {
		results[i] = labelname.AliasRankResult{
			TaskTimestamp: taskTimestamp,
			ItemIndex:     i,
		}
	}
	for directResultIndex, result := range labelname.RankAliasBatch(directRequests) {
		results[directIndexes[directResultIndex]] = result
	}
	fallbackIndexes := make([]int, 0)
	fallbackRequests := make([]labelname.AliasRankRequest, 0)
	for i, result := range results {
		if len(result.RankedAliases) > 0 {
			continue
		}
		group := groups[i]
		request := aliasRankRequestFromKeywordRows(taskTimestamp, collectKeywordGroupAliasCandidates(group), group.Rows)
		request.ItemIndex = i
		request.SearchTerm = firstNonEmpty(request.SearchTerm, group.SearchTerm)
		fallbackIndexes = append(fallbackIndexes, i)
		fallbackRequests = append(fallbackRequests, request)
	}
	if len(fallbackRequests) > 0 {
		fallbackResults := labelname.RankAliasBatch(fallbackRequests)
		for i, result := range fallbackResults {
			results[fallbackIndexes[i]] = result
		}
	}
	identifications := make([]keywordLabelIdentification, len(results))
	for i, result := range results {
		identifications[i] = keywordLabelIdentification{
			TaskTimestamp: result.TaskTimestamp,
			ItemIndex:     result.ItemIndex,
			Aliases:       result.RankedAliases,
			SourceType:    sourceType,
		}
	}
	return identifications
}

func aliasRankRequestHasLocalTerms(request labelname.AliasRankRequest) bool {
	return strings.TrimSpace(request.SearchTerm) != "" ||
		strings.TrimSpace(request.Symbol) != "" ||
		strings.TrimSpace(request.ProteinID) != "" ||
		strings.TrimSpace(request.GeneID) != "" ||
		strings.TrimSpace(request.TranscriptID) != "" ||
		strings.TrimSpace(request.SequenceID) != "" ||
		strings.TrimSpace(request.LocusTag) != "" ||
		strings.TrimSpace(request.Description) != "" ||
		len(request.Aliases) != 0 ||
		len(request.Synonyms) != 0 ||
		len(request.DBXrefs) != 0 ||
		len(request.OtherDesignations) != 0
}

func aliasRankRequestFromKeywordSearchTerm(taskTimestamp string, itemIndex int, group model.KeywordSearchGroup) labelname.AliasRankRequest {
	searchTerm := firstNonEmpty(group.SearchTerm, group.LabelName)
	request := labelname.AliasRankRequest{
		TaskTimestamp: taskTimestamp,
		ItemIndex:     itemIndex,
		SearchTerm:    searchTerm,
		Aliases:       uniqueStrings(compactStrings(searchTerm, group.LabelName)),
	}
	for _, row := range group.Rows {
		if taxID := symbolNameTaxIDForSourceDatabase(row.SourceDatabase, row.Genome, row.SequenceHeaderLabel); taxID != "" {
			request.TaxID = firstNonEmpty(request.TaxID, taxID)
		}
	}
	return request
}

func (w *BlastWizard) autoIdentifyKeywordLabelsWithProgress(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup) ([]keywordLabelIdentification, error) {
	if !w.suppressTaskModals {
		if err := w.ensureSymbolNameDatabase(ctx, prompt.ErrBackToQueryInput); err != nil {
			return nil, err
		}
	}
	identifications, err := tui.RunTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Keyword", "Auto identify"),
		Title:       "Auto identifying symbol names",
		Description: "Inferring keyword symbol names from result rows.",
		Initial:     "Auto identifying symbol names...",
		CancelError: prompt.ErrBackToQueryInput,
	}, func(taskCtx context.Context, update func(string)) ([]keywordLabelIdentification, error) {
		taskUpdate := safeTaskUpdate(update)
		labelCtx := mergeContexts(ctx, taskCtx)
		if err := w.ensureSymbolNameDatabaseWithUpdate(labelCtx, update, true); err != nil {
			return nil, err
		}
		taskUpdate("Reviewing keyword result rows...")
		working := cloneKeywordSearchGroups(groups)
		taskUpdate("Selecting symbol names...")
		return autoIdentifyKeywordLabelIdentificationsWithSourceType(working, "symbolname database"), nil
	})
	if err == nil && len(groups) > 0 && len(identifications) == len(groups) {
		notifyaudio.PlayDone()
	}
	return identifications, err
}

func (w *BlastWizard) autoIdentifyLemnaKeywordLabelsWithProgress(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup) []keywordLabelIdentification {
	return w.autoIdentifyLemnaKeywordLabels(ctx, selected, groups, nil)
}

func (w *BlastWizard) autoIdentifyTAIRKeywordLabelsWithProgress(ctx context.Context, groups []model.KeywordSearchGroup, update func(string)) []keywordLabelIdentification {
	taskUpdate := safeTaskUpdate(update)
	taskUpdate("Selecting TAIR symbol names from the local symbol database...")
	return w.autoIdentifyTAIRKeywordLabelsWithLookup(ctx, groups, nil)
}

func (w *BlastWizard) autoIdentifyNCBIKeywordLabelsWithProgress(ctx context.Context, groups []model.KeywordSearchGroup) []keywordLabelIdentification {
	return w.autoIdentifyNCBIKeywordLabels(ctx, groups, nil)
}

func (w *BlastWizard) autoIdentifyNCBIKeywordLabels(ctx context.Context, groups []model.KeywordSearchGroup, lookupSource source.DataSource) []keywordLabelIdentification {
	return autoIdentifyKeywordLabelIdentificationsWithSourceType(groups, "symbolname database")
}

func (w *BlastWizard) autoIdentifyLemnaKeywordLabels(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup, lookupSource source.DataSource) []keywordLabelIdentification {
	return autoIdentifyKeywordLabelIdentificationsWithSourceType(groups, "symbolname database")
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

func (w *BlastWizard) autoIdentifyTAIRKeywordLabels(ctx context.Context, groups []model.KeywordSearchGroup) []keywordLabelIdentification {
	return w.autoIdentifyTAIRKeywordLabelsWithLookup(ctx, groups, nil)
}

func (w *BlastWizard) autoIdentifyTAIRKeywordLabelsWithLookup(ctx context.Context, groups []model.KeywordSearchGroup, lookupSource source.DataSource) []keywordLabelIdentification {
	return autoIdentifyKeywordLabelIdentificationsWithSourceType(groups, "symbolname database")
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
	return allKeywordRowSymbolCandidates(row)
}

func allKeywordRowSymbolCandidates(row model.KeywordResultRow) []string {
	aliases := make([]string, 0, 24)
	aliases = append(aliases, row.LabelName)
	aliases = append(aliases, labelname.SplitAliases(row.PhgoAliases)...)
	aliases = append(aliases, labelname.SplitAliases(row.Symbols)...)
	aliases = append(aliases, labelname.SplitAliases(row.Synonyms)...)
	aliases = append(aliases, labelname.SplitAliases(row.Aliases)...)
	aliases = append(aliases, labelname.SplitAliases(row.UniProt)...)
	aliases = append(aliases, labelname.AutoDefineCandidates(row.AutoDefine)...)
	aliases = append(aliases, labelname.AutoDefineCandidates(row.Description)...)
	aliases = append(aliases, labelname.AutoDefineCandidates(row.Comments)...)
	aliases = append(aliases, phytozomeUniProtAliasCandidates(row.UniProt)...)
	if row.ExtraColumns != nil {
		for _, key := range []string{
			"attr_Alias",
			"attr_alias",
			"attr_Name",
			"attr_name",
			"attr_gene_name",
			"attr_gene_symbol",
			"attr_symbol",
			"attr_gene",
			"tair_fasta_symbols",
			"tair_keyword_synonyms",
			"tair_keyword_name_exact",
			"ena_locus_tag",
			"ena_protein_id",
			"ena_accession",
			"ncbi_other_aliases",
			"ncbi_gene_name",
			"ahrd_blast_hit_accession",
		} {
			aliases = append(aliases, labelname.SplitAliases(row.ExtraColumns[key])...)
		}
	}
	return uniqueStrings(aliases)
}

func ncbiKeywordRowPhytozomeSearchTerms(row model.KeywordResultRow) []string {
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
	add(row.GeneLocus)
	if row.ExtraColumns != nil {
		for _, key := range []string{"ncbi_accession", "ncbi_gene_name", "ncbi_locus_tag", "ncbi_gene_id"} {
			add(row.ExtraColumns[key])
		}
	}
	return uniqueStrings(terms)
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
	aliases = append(aliases, splitLocalAliasText(seed.LabelName)...)
	aliases = append(aliases, splitLocalAliasText(seed.PhgoAliases)...)
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
			if fields, ok := splitWhitespaceLocalAliasList(part); ok {
				out = append(out, fields...)
			} else {
				out = append(out, part)
			}
		}
	}
	return out
}

func splitWhitespaceLocalAliasList(value string) ([]string, bool) {
	fields := strings.Fields(value)
	if len(fields) <= 1 {
		return nil, false
	}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, `"'()[]{}.,;`)
		if !looksLikeLocalAliasToken(field) {
			return nil, false
		}
		out = append(out, field)
	}
	return out, true
}

func looksLikeLocalAliasToken(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 2 || len(value) > 40 {
		return false
	}
	hasLetter := false
	hasDigit := false
	hasUpper := false
	hasLower := false
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z':
			hasLetter = true
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLetter = true
			hasLower = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == '_' || r == '-' || r == '.' || r == '\'':
		default:
			return false
		}
	}
	if !hasLetter {
		return false
	}
	if hasLower && !hasUpper && !hasDigit {
		return false
	}
	return true
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

func applyKeywordGeneLoci(groups []model.KeywordSearchGroup, loci []string, sourceType string) {
	if len(groups) != len(loci) {
		return
	}
	sourceType = strings.TrimSpace(sourceType)
	for i := range groups {
		locus := strings.TrimSpace(loci[i])
		if locus == "~" {
			locus = ""
		}
		for r := range groups[i].Rows {
			groups[i].Rows[r].GeneLocus = locus
			if groups[i].Rows[r].ExtraColumns == nil {
				groups[i].Rows[r].ExtraColumns = make(map[string]string)
			}
			if sourceType != "" {
				groups[i].Rows[r].ExtraColumns["gene_locus_type"] = sourceType
			}
		}
	}
}

func (w *BlastWizard) autoIdentifyNCBIKeywordGeneLociWithProgress(ctx context.Context, groups []model.KeywordSearchGroup) error {
	if len(groups) == 0 {
		return nil
	}
	run := func(taskCtx context.Context, update func(int, string)) error {
		_, err := w.autoIdentifyNCBIKeywordGeneLoci(mergeContexts(ctx, taskCtx), groups, update)
		return err
	}
	if w.suppressTaskModals {
		return run(ctx, nil)
	}
	_, err := tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Keyword", "NCBI Gene locus"),
		Title:       "Loading NCBI Gene locus values",
		Description: "Finding Gene locus values for NCBI keyword rows.",
		Initial:     "Loading NCBI Gene locus values...",
		Total:       len(groups),
		CancelError: prompt.ErrBackToQueryInput,
	}, func(taskCtx context.Context, update func(int, string)) (struct{}, error) {
		return struct{}{}, run(taskCtx, update)
	})
	return err
}

func (w *BlastWizard) autoIdentifyNCBIKeywordGeneLoci(ctx context.Context, groups []model.KeywordSearchGroup, update func(int, string)) ([]model.KeywordSearchGroup, error) {
	if len(groups) == 0 {
		return groups, nil
	}
	progress := safeProgress(update)
	lookupSource := phytozome.NewClient(w.httpClient)
	candidates, err := w.speciesCandidatesForSource(ctx, lookupSource, nil)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return groups, ctxErr
		}
		applyExistingNCBIGeneLoci(groups)
		progress(len(groups), "NCBI Gene locus values are ready.")
		return groups, nil
	}
	for i := range groups {
		if err := ctx.Err(); err != nil {
			return groups, err
		}
		progress(i, fmt.Sprintf("Finding Gene locus %d/%d (%s)...", i+1, len(groups), oneLinePreview(firstNonEmpty(groups[i].SearchTerm, groups[i].LabelName))))
		locus, sourceType := ncbiKeywordGroupGeneLocus(groups[i], lookupSource, candidates, w, ctx)
		if strings.TrimSpace(locus) == "" {
			continue
		}
		for r := range groups[i].Rows {
			groups[i].Rows[r].GeneLocus = locus
			if groups[i].Rows[r].ExtraColumns == nil {
				groups[i].Rows[r].ExtraColumns = make(map[string]string)
			}
			groups[i].Rows[r].ExtraColumns["gene_locus_type"] = sourceType
		}
	}
	progress(len(groups), "NCBI Gene locus values are ready.")
	return groups, nil
}

func applyExistingNCBIGeneLoci(groups []model.KeywordSearchGroup) {
	for i := range groups {
		locus := ""
		for _, row := range groups[i].Rows {
			if value := strings.TrimSpace(row.GeneLocus); value != "" {
				locus = value
				break
			}
		}
		if locus == "" {
			continue
		}
		for r := range groups[i].Rows {
			groups[i].Rows[r].GeneLocus = locus
		}
	}
}

func ncbiKeywordGroupGeneLocus(group model.KeywordSearchGroup, lookupSource source.DataSource, candidates []model.SpeciesCandidate, w *BlastWizard, ctx context.Context) (string, string) {
	for _, row := range group.Rows {
		if locus := strings.TrimSpace(row.GeneLocus); locus != "" {
			return locus, "ncbi gene locus"
		}
	}
	if lookupSource == nil || w == nil {
		return "", ""
	}
	for _, row := range group.Rows {
		species, ok := matchPhytozomeSpeciesForFastaHeader(firstNonEmpty(row.SequenceHeaderLabel, row.Genome), candidates)
		if !ok {
			continue
		}
		terms := ncbiKeywordRowPhytozomeSearchTerms(row)
		rowsByTerm := w.fetchKeywordRowsByTerms(ctx, lookupSource, species, terms)
		for _, term := range terms {
			for _, candidate := range rowsByTerm[strings.ToLower(strings.TrimSpace(term))] {
				if locus := firstNonEmpty(candidate.GeneLocus, stripTranscriptDecorations(candidate.GeneIdentifier), candidate.TranscriptID); locus != "" {
					return locus, "phytozome gene locus fallback"
				}
			}
		}
	}
	return "", ""
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
	outputDir := strings.TrimSpace(settings.OutputDir)
	if outputDir == "" {
		var err error
		outputDir, err = appfs.OutputDir()
		if err != nil {
			return err
		}
	}
	_, err := w.exportSelectedBlastFiles(ctx, rows, allRows, nil, nil, querySource, baseName, outputDir, settings, true)
	return err
}

func (w *BlastWizard) exportKeywordSelections(ctx context.Context, selected model.SpeciesCandidate, rows []model.KeywordResultRow, allRows []model.KeywordResultRow, groups []model.KeywordSearchGroup, baseName string, outputDir string, settings exportSettings, selectedMask []bool, mode QueryMode, reportCtx *keywordReportRunContext) error {
	return w.exportSelectedKeywordFiles(ctx, selected, rows, allRows, groups, baseName, outputDir, settings, selectedMask, mode, reportCtx, true)
}

func (w *BlastWizard) exportSelectedBlastFiles(ctx context.Context, rows []model.BlastResultRow, allRows []model.BlastResultRow, rowNumbers []int, filterFlags []bool, querySource *model.QuerySequenceSource, baseName string, outputDir string, settings exportSettings, showComplete bool) (exportFileResult, error) {
	return w.exportBlastSelectionsToDir(ctx, rows, allRows, rowNumbers, filterFlags, querySource, baseName, baseName, sanitizeExportName(baseName), outputDir, settings, showComplete)
}

func (w *BlastWizard) exportSelectedKeywordFiles(ctx context.Context, selected model.SpeciesCandidate, rows []model.KeywordResultRow, allRows []model.KeywordResultRow, groups []model.KeywordSearchGroup, baseName string, outputDir string, settings exportSettings, selectedMask []bool, mode QueryMode, reportCtx *keywordReportRunContext, showComplete bool) error {
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
				rawRecords = applyKeywordHeaderMode(rawRecords, allRows, settings.fastaHeaderMode())
				writeStart := time.Now()
				if err := export.WriteProteinSequencesText(rawTextPath, rawRecords); err != nil {
					return append(steps, keywordReportStep("Write raw peptide FASTA", writeStart, time.Now(), "failed", err.Error())), err
				}
				return append(steps, keywordReportStep("Write raw peptide FASTA", writeStart, time.Now(), "ok", fmt.Sprintf("%d peptide records written", len(rawRecords)))), nil
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
		steps = append(steps, keywordReportStep("Preload keyword peptide sequences", preloadStart, time.Now(), "ok", fmt.Sprintf("%d keyword rows checked before writing FASTA files", len(rows))))
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
		records = applyKeywordHeaderMode(records, rows, settings.fastaHeaderMode())
		sequenceRecords = records
		sequenceAudit = buildKeywordSequenceAudit(rows, records)
		writeStart := time.Now()
		if err := withSpinner(w.out, "Writing peptide FASTA file...", func() error {
			return export.WriteProteinSequencesText(textPath, records)
		}); err != nil {
			steps = append(steps, keywordReportStep("Write peptide FASTA", writeStart, time.Now(), "failed", err.Error()))
			return err
		}
		steps = append(steps, keywordReportStep("Write peptide FASTA", writeStart, time.Now(), "ok", fmt.Sprintf("%d peptide records written", len(records))))
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
	if settings.WriteSession {
		sessionPath, err := w.writeKeywordSessionSnapshot(selected, groups, selectedMask, mode, settings, reportCtx)
		if err != nil {
			return err
		}
		files.SessionPath = sessionPath
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
		Description: "Writing the keyword Excel file while fetching peptide sequences for the FASTA export.",
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

	if resolver, ok := w.source.(source.QueryResolver); ok {
		species := model.SpeciesCandidate{}
		for _, candidate := range candidates {
			if candidate.ProteomeID != 0 {
				species = candidate
				break
			}
		}
		if ok, resolved, err := w.tryResolveSourceQueryInput(ctx, resolver, species, input); err != nil {
			return nil, ok, err
		} else if ok {
			return resolved, true, nil
		}
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
	if !isMissingProteinSequenceError(err) {
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
	if w.source == nil {
		return model.ProteinSequenceData{}, fmt.Errorf("protein sequence source is unavailable")
	}

	value, err, _ := w.proteinSequenceGroup.Do(cacheKey, func() (any, error) {
		if sequence, ok := w.cachedProteinSequence(cacheKey); ok {
			return sequence, nil
		}
		if err := w.cachedProteinSequenceMiss(cacheKey); err != nil {
			return model.ProteinSequenceData{}, err
		}
		if w.source == nil {
			return model.ProteinSequenceData{}, fmt.Errorf("protein sequence source is unavailable")
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
		if inline := inlineKeywordProteinSequenceData(row); strings.TrimSpace(inline.Sequence) != "" {
			if strings.TrimSpace(inline.OriginalHeader) == "" {
				inline.OriginalHeader = keywordProteinSequenceHeader(row)
			}
			w.storeProteinSequence(cacheKey, inline)
			results[sequenceID] = sequenceFetchResult{data: inline}
			continue
		}
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
	return selected.ProteomeID
}

func (w *BlastWizard) loadKeywordDetailFASTA(row model.KeywordResultRow) (string, error) {
	if inline := inlineKeywordProteinSequenceData(row); strings.TrimSpace(inline.Sequence) != "" {
		header := strings.TrimSpace(inline.OriginalHeader)
		if header == "" {
			header = keywordProteinSequenceHeader(row)
		}
		return formatDetailFASTA(header, inline.Sequence), nil
	}
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

func inlineKeywordProteinSequenceData(row model.KeywordResultRow) model.ProteinSequenceData {
	header := firstNonEmpty(
		strings.TrimSpace(keywordExtraColumn(row, "plaza_fasta_header")),
		strings.TrimSpace(keywordExtraColumn(row, "ncbi_fasta_header")),
	)
	raw := firstNonEmpty(
		strings.TrimSpace(keywordExtraColumn(row, "plaza_fasta")),
		strings.TrimSpace(keywordExtraColumn(row, "ncbi_fasta")),
		strings.TrimSpace(keywordExtraColumn(row, "cyp_fasta")),
	)
	if raw != "" {
		rawHeader, sequence := splitFastaHeaderAndSequence(raw)
		if strings.TrimSpace(rawHeader) != "" {
			header = ensureFastaHeader(rawHeader)
		}
		if strings.TrimSpace(sequence) != "" {
			return model.ProteinSequenceData{
				Sequence:       strings.TrimSpace(sequence),
				OriginalHeader: firstNonEmpty(header, keywordProteinSequenceHeader(row)),
			}
		}
	}
	sequence := strings.TrimSpace(extractInlineKeywordSequence(row))
	if sequence == "" {
		sequence = strings.TrimSpace(keywordExtraColumn(row, "cyp_sequence"))
	}
	if sequence == "" {
		return model.ProteinSequenceData{}
	}
	if strings.HasPrefix(sequence, ">") {
		rawHeader, rawSequence := splitFastaHeaderAndSequence(sequence)
		if rawHeader != "" && rawSequence != "" {
			return model.ProteinSequenceData{
				Sequence:       strings.TrimSpace(rawSequence),
				OriginalHeader: ensureFastaHeader(rawHeader),
			}
		}
	}
	return model.ProteinSequenceData{
		Sequence:       sanitizeSequence(sequence),
		OriginalHeader: firstNonEmpty(header, keywordProteinSequenceHeader(row)),
	}
}

func keywordExtraColumn(row model.KeywordResultRow, key string) string {
	if row.ExtraColumns == nil {
		return ""
	}
	return strings.TrimSpace(row.ExtraColumns[key])
}

func ensureFastaHeader(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	if strings.HasPrefix(header, ">") {
		return header
	}
	return ">" + header
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
		GeneID:        strings.TrimSpace(querySource.GeneID),
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
			GeneID:            strings.TrimSpace(source.GeneID),
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

func applyKeywordHeaderMode(records []model.ProteinSequenceRecord, rows []model.KeywordResultRow, mode model.FastaHeaderMode) []model.ProteinSequenceRecord {
	switch model.NormalizeFastaHeaderMode(mode, true) {
	case model.FastaHeaderModePhgoLite:
		return applyKeywordPhgoLiteHeaders(records, rows)
	case model.FastaHeaderModeOriginal:
		return applyOriginalHeaders(records)
	case model.FastaHeaderModeMinimal:
		return applyKeywordMinimalHeaders(records, rows)
	default:
		return applyKeywordPhgoHeaders(records, rows)
	}
}

func applyBlastHeaderMode(records []model.ProteinSequenceRecord, rows []model.BlastResultRow, querySources []*model.QuerySequenceSource, prependedQueryCount int, mode model.FastaHeaderMode) []model.ProteinSequenceRecord {
	switch model.NormalizeFastaHeaderMode(mode, true) {
	case model.FastaHeaderModePhgoLite:
		return applyBlastPhgoLiteHeaders(records, rows, querySources, prependedQueryCount)
	case model.FastaHeaderModeOriginal:
		return applyOriginalHeaders(records)
	case model.FastaHeaderModeMinimal:
		return applyBlastMinimalHeaders(records, rows, querySources, prependedQueryCount)
	default:
		return applyBlastPhgoHeaders(records, rows, querySources, prependedQueryCount)
	}
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

func applyKeywordPhgoLiteHeaders(records []model.ProteinSequenceRecord, rows []model.KeywordResultRow) []model.ProteinSequenceRecord {
	out := append([]model.ProteinSequenceRecord(nil), records...)
	limit := minInt(len(out), len(rows))
	for i := 0; i < limit; i++ {
		if header := buildPhgoLiteHeader(
			firstNonEmpty(strings.TrimSpace(rows[i].SequenceHeaderLabel), strings.TrimSpace(rows[i].Genome)),
			keywordMinimalHeaderID(rows[i], out[i]),
			rowKeywordLabelName(rows[i]),
		); header != "" {
			out[i].Header = header
		}
	}
	for i := limit; i < len(out); i++ {
		if header := minimalFastaHeader(recordMinimalHeaderID(out[i])); header != "" {
			out[i].Header = header
		}
	}
	return out
}

func applyKeywordMinimalHeaders(records []model.ProteinSequenceRecord, rows []model.KeywordResultRow) []model.ProteinSequenceRecord {
	out := append([]model.ProteinSequenceRecord(nil), records...)
	limit := minInt(len(out), len(rows))
	for i := 0; i < limit; i++ {
		if header := minimalFastaHeader(keywordMinimalHeaderID(rows[i], out[i])); header != "" {
			out[i].Header = header
		}
	}
	for i := limit; i < len(out); i++ {
		if header := minimalFastaHeader(recordMinimalHeaderID(out[i])); header != "" {
			out[i].Header = header
		}
	}
	return out
}

func applyBlastPhgoHeaders(records []model.ProteinSequenceRecord, rows []model.BlastResultRow, querySources []*model.QuerySequenceSource, prependedQueryCount int) []model.ProteinSequenceRecord {
	out := append([]model.ProteinSequenceRecord(nil), records...)
	queryLimit := minInt(minInt(prependedQueryCount, len(out)), len(querySources))
	for i := 0; i < queryLimit; i++ {
		if header := querySourcePhgoHeader(querySources[i]); header != "" {
			out[i].Header = header
		}
	}
	start := minInt(prependedQueryCount, len(out))
	limit := minInt(len(out)-start, len(rows))
	for i := 0; i < limit; i++ {
		if header := blastPhgoHeader(rows[i], i+1); header != "" {
			out[start+i].Header = header
		}
	}
	return out
}

func applyBlastPhgoLiteHeaders(records []model.ProteinSequenceRecord, rows []model.BlastResultRow, querySources []*model.QuerySequenceSource, prependedQueryCount int) []model.ProteinSequenceRecord {
	out := append([]model.ProteinSequenceRecord(nil), records...)
	queryLimit := minInt(minInt(prependedQueryCount, len(out)), len(querySources))
	for i := 0; i < queryLimit; i++ {
		if source := querySources[i]; source != nil {
			if header := buildPhgoLiteHeader(
				firstNonEmpty(strings.TrimSpace(source.OrganismShort), strings.TrimSpace(source.SourceJBrowseName), strings.TrimSpace(source.SourceGenomeLabel)),
				querySourceID2(source),
				firstNonEmpty(strings.TrimSpace(source.LabelName), preferredStoredQuerySourceAlias(source)),
			); header != "" {
				out[i].Header = header
			}
		}
	}
	start := minInt(prependedQueryCount, len(out))
	limit := minInt(len(out)-start, len(rows))
	for i := 0; i < limit; i++ {
		if header := buildPhgoLiteHeader(
			strings.TrimSpace(rows[i].Species),
			blastRowID2(rows[i]),
			firstNonEmpty(strings.TrimSpace(rows[i].LabelName), strings.TrimSpace(rows[i].BlastLabelName)),
		); header != "" {
			out[start+i].Header = header
		}
	}
	for i := start + limit; i < len(out); i++ {
		if header := minimalFastaHeader(recordMinimalHeaderID(out[i])); header != "" {
			out[i].Header = header
		}
	}
	return out
}

func applyBlastMinimalHeaders(records []model.ProteinSequenceRecord, rows []model.BlastResultRow, querySources []*model.QuerySequenceSource, prependedQueryCount int) []model.ProteinSequenceRecord {
	out := append([]model.ProteinSequenceRecord(nil), records...)
	queryLimit := minInt(minInt(prependedQueryCount, len(out)), len(querySources))
	for i := 0; i < queryLimit; i++ {
		if header := minimalFastaHeader(querySourceID2(querySources[i])); header != "" {
			out[i].Header = header
		}
	}
	start := minInt(prependedQueryCount, len(out))
	limit := minInt(len(out)-start, len(rows))
	for i := 0; i < limit; i++ {
		if header := minimalFastaHeader(blastRowID2(rows[i])); header != "" {
			out[start+i].Header = header
		}
	}
	for i := start + limit; i < len(out); i++ {
		if header := minimalFastaHeader(recordMinimalHeaderID(out[i])); header != "" {
			out[i].Header = header
		}
	}
	return out
}

func keywordMinimalHeaderID(row model.KeywordResultRow, record model.ProteinSequenceRecord) string {
	return firstNonEmpty(
		strings.TrimSpace(row.TranscriptID),
		strings.TrimSpace(row.SequenceID),
		stripTranscriptDecorations(strings.TrimSpace(row.GeneIdentifier)),
		recordMinimalHeaderID(record),
	)
}

func minimalFastaHeader(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return ">" + id
}

func recordMinimalHeaderID(record model.ProteinSequenceRecord) string {
	for _, header := range []string{record.Header, record.OriginalHeader} {
		if id := primaryIDFromFastaHeader(header); id != "" {
			return id
		}
	}
	return "sequence"
}

func primaryIDFromFastaHeader(header string) string {
	if parsed, ok := parsePhgoFastaHeader(header); ok && strings.TrimSpace(parsed.GeneID) != "" {
		return strings.TrimSpace(parsed.GeneID)
	}
	header = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(header), ">"))
	if header == "" {
		return ""
	}
	if open := strings.Index(header, " ("); open >= 0 {
		header = strings.TrimSpace(header[:open])
	}
	if pipe := strings.LastIndex(header, "|"); pipe >= 0 && pipe < len(header)-1 {
		header = strings.TrimSpace(header[pipe+1:])
	}
	if fields := strings.Fields(header); len(fields) > 0 {
		header = fields[0]
	}
	return strings.TrimSpace(header)
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
	return buildBlastPhgoHeader(
		strings.TrimSpace(row.Species),
		firstNonEmpty(strings.TrimSpace(row.LabelName), strings.TrimSpace(row.BlastLabelName)),
		blastRowID2(row),
		strings.TrimSpace(row.BlastLabelName),
		strings.TrimSpace(row.BlastGeneID),
		rowNumber,
	)
}

func querySourcePhgoHeader(source *model.QuerySequenceSource) string {
	if source == nil {
		return ""
	}
	return buildPhgoHeaderWithGroups(
		firstNonEmpty(strings.TrimSpace(source.OrganismShort), strings.TrimSpace(source.SourceJBrowseName), strings.TrimSpace(source.SourceGenomeLabel)),
		firstNonEmpty(strings.TrimSpace(source.LabelName), preferredStoredQuerySourceAlias(source), querySourceID2(source)),
		querySourceID2(source),
		"h",
	)
}

func buildPhgoHeader(species string, label string, geneID string, rowNumber int) string {
	groups := []string{}
	if rowNumber > 0 {
		groups = append(groups, strconv.Itoa(rowNumber))
	}
	return buildPhgoHeaderWithGroups(species, label, geneID, groups...)
}

// buildPhgoLiteHeader keeps the portable PHgo identity fields in a compact form.
func buildPhgoLiteHeader(species string, id2 string, symbolName string) string {
	species = sanitizePhgoLiteHeaderPart(species)
	id2 = sanitizePhgoLiteHeaderPart(id2)
	if species == "" {
		species = "~"
	}
	if id2 == "" {
		id2 = "~"
	}
	header := ">" + species + "|" + id2
	if isPhgoLiteEmptyField(symbolName) {
		return header
	}
	symbolName = sanitizePhgoLiteHeaderPart(symbolName)
	if isPhgoLiteEmptyField(symbolName) {
		return header
	}
	return header + "(" + symbolName + ")"
}

func buildBlastPhgoHeader(species string, label string, geneID string, blastSourceLabel string, blastSourceGeneID string, rowNumber int) string {
	sourceLabel := sanitizePhgoHeaderPart(blastSourceLabel)
	sourceGeneID := sanitizePhgoHeaderPart(blastSourceGeneID)
	groups := []string{phgoHeaderPartOrPlaceholder(sourceLabel) + "/" + phgoHeaderPartOrPlaceholder(sourceGeneID)}
	if rowNumber > 0 {
		groups = append(groups, strconv.Itoa(rowNumber))
	}
	return buildPhgoHeaderWithGroups(species, label, geneID, groups...)
}

func buildPhgoHeaderWithGroups(species string, label string, geneID string, groups ...string) string {
	species = sanitizePhgoHeaderPart(species)
	label = sanitizePhgoHeaderPart(label)
	geneID = sanitizePhgoHeaderPart(geneID)
	species = phgoHeaderPartOrPlaceholder(species)
	label = phgoHeaderPartOrPlaceholder(label)
	geneID = phgoHeaderPartOrPlaceholder(geneID)
	header := ">phgo://" + species + "/" + label + "/" + geneID
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group != "" {
			header += "\\" + group
		}
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

func querySourceID2(source *model.QuerySequenceSource) string {
	if source == nil {
		return ""
	}
	for _, value := range []string{
		strings.TrimSpace(source.TranscriptID),
		strings.TrimSpace(source.GeneID),
		strings.TrimSpace(source.ProteinID),
		strings.TrimSpace(source.PreferredSequenceID),
	} {
		if value != "" {
			return stripTranscriptDecorations(value)
		}
	}
	return ""
}

func blastRowID2(row model.BlastResultRow) string {
	for _, value := range []string{
		strings.TrimSpace(row.Protein),
		strings.TrimSpace(row.SequenceID),
		strings.TrimSpace(row.TranscriptID),
		strings.TrimSpace(row.SubjectID),
	} {
		if value != "" {
			return stripTranscriptDecorations(value)
		}
	}
	if row.TargetID > 0 {
		return strconv.Itoa(row.TargetID)
	}
	return ""
}

func sanitizePhgoHeaderPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "~" {
		return ""
	}
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.Join(strings.Fields(value), " ")
	return strings.TrimSpace(value)
}

func sanitizePhgoLiteHeaderPart(value string) string {
	value = sanitizePhgoHeaderPart(value)
	value = strings.ReplaceAll(value, "|", "_")
	value = strings.ReplaceAll(value, "(", "_")
	value = strings.ReplaceAll(value, ")", "_")
	return value
}

func isPhgoLiteEmptyField(value string) bool {
	switch strings.TrimSpace(value) {
	case "", "~", "~~":
		return true
	default:
		return false
	}
}

func phgoHeaderPartOrPlaceholder(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "~"
	}
	return value
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
	if source.ProteinID != "" && source.ProteinID != source.TranscriptID && source.ProteinID != source.GeneID {
		lines = append(lines, "Protein ID: "+source.ProteinID)
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
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	switch host {
	case "phytozome-next.jgi.doe.gov":
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
	case "www.lemna.org", "lemna.org":
		segments := nonEmptyPathSegments(parsed.Path)
		if len(segments) != 1 || !strings.EqualFold(segments[0], "jbrowse2") {
			return "", false
		}
		values := parsed.Query()
		rootDir := strings.TrimSpace(values.Get("phgo_root"))
		geneID := strings.TrimSpace(values.Get("phgo_gene"))
		if rootDir == "" || geneID == "" {
			return "", false
		}
		parsed.Scheme = "https"
		parsed.Host = "www.lemna.org"
		parsed.RawQuery = values.Encode()
		parsed.Fragment = ""
		return parsed.String(), true
	default:
		return "", false
	}
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
	case "www.arabidopsis.org", "arabidopsis.org":
		return "tair"
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
	case "tair":
		return "TAIR"
	case "ncbi":
		return "NCBI"
	default:
		return strings.TrimSpace(name)
	}
}

func parseGeneReportURL(rawURL string) (jbrowseName string, reportType string, identifier string, err error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", fmt.Errorf("parse gene report URL: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(parsed.Host)) {
	case "phytozome-next.jgi.doe.gov":
		segments := nonEmptyPathSegments(parsed.Path)
		if len(segments) != 4 || !strings.EqualFold(segments[0], "report") {
			return "", "", "", fmt.Errorf("unsupported gene report URL path: %s", parsed.Path)
		}
		reportType = strings.ToLower(segments[1])
		jbrowseName = segments[2]
		identifier = segments[3]
	case "www.lemna.org", "lemna.org":
		segments := nonEmptyPathSegments(parsed.Path)
		if len(segments) != 1 || !strings.EqualFold(segments[0], "jbrowse2") {
			return "", "", "", fmt.Errorf("unsupported gene report URL path: %s", parsed.Path)
		}
		values := parsed.Query()
		jbrowseName = strings.TrimSpace(values.Get("phgo_root"))
		identifier = strings.TrimSpace(values.Get("phgo_gene"))
		reportType = "gene"
	default:
		return "", "", "", fmt.Errorf("unsupported gene report host: %s", parsed.Host)
	}
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
	if fastautil.IsIgnoredPHGONoteHeader(header) {
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
		source.BlastSourceLabelName = parsed.BlastSourceLabelName
		source.BlastSourceGeneID = parsed.BlastSourceGeneID
		source.PhgoRowNumber = parsed.RowNumber
		source.PhgoHasRowNumber = parsed.HasRowPart
		source.PhgoBlastQuerySource = parsed.IsBlastQuerySource
		source.PhgoCanvasRawRow = parsed.CanvasRawRowNumber
		source.PhgoCanvasHasRawRow = parsed.CanvasHasRawRow
		source.PhgoCanvasTitle = parsed.CanvasItemTitle
		source.PhgoCanvasDisplay = parsed.CanvasDisplayName
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
	if headerLine == "" {
		return "", ""
	}

	if len(remainingSequenceLines) > 0 {
		sequence := sanitizeSequence(strings.Join(remainingSequenceLines, "\n"))
		return headerLine, sequence
	}

	if strings.HasPrefix(strings.ToLower(headerLine), "phgo://") {
		header, sequence := splitSingleLinePhgoHeaderAndSequence(headerLine)
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
	labelSuffix := ""
	if strings.HasPrefix(sequencePart, "(") {
		if closeIndex := strings.Index(sequencePart, ")"); closeIndex >= 0 {
			labelSuffix = " " + strings.TrimSpace(sequencePart[:closeIndex+1])
			sequencePart = strings.TrimSpace(sequencePart[closeIndex+1:])
		}
	}
	if sequencePart == "" {
		return "", ""
	}

	header := strings.TrimSpace(headerLine[:pipeIndex+1] + idPart + labelSuffix)
	sequence := sanitizeSequence(sequencePart)
	return header, sequence
}

type phgoFastaHeader struct {
	RawHeader            string
	Species              string
	LabelName            string
	GeneID               string
	BlastSourceLabelName string
	BlastSourceGeneID    string
	RowNumber            int
	HasRowPart           bool
	IsBlastQuerySource   bool
	CanvasRawRowNumber   int
	CanvasHasRawRow      bool
	CanvasItemTitle      string
	CanvasDisplayName    string
	IsCanvasHeader       bool
	IsLiteHeader         bool
}

func parsePhgoFastaHeader(header string) (phgoFastaHeader, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return phgoFastaHeader{}, false
	}
	if !strings.HasPrefix(strings.ToLower(header), "phgo://") {
		return parsePhgoLiteFastaHeader(header)
	}
	body := strings.TrimSpace(header[len("phgo://"):])
	if body == "" {
		return phgoFastaHeader{}, false
	}
	groups := strings.Split(body, "\\")
	if len(groups) > 4 {
		return phgoFastaHeader{}, false
	}
	selfParts := strings.Split(strings.TrimSpace(groups[0]), "/")
	if len(selfParts) != 3 {
		return phgoFastaHeader{}, false
	}
	for i := range selfParts {
		selfParts[i] = strings.TrimSpace(selfParts[i])
	}
	if selfParts[0] == "" || selfParts[1] == "" || selfParts[2] == "" {
		return phgoFastaHeader{}, false
	}
	parsed := phgoFastaHeader{
		RawHeader: header,
		Species:   phgoHeaderFieldValue(selfParts[0]),
		LabelName: phgoHeaderFieldValue(selfParts[1]),
		GeneID:    phgoHeaderFieldValue(selfParts[2]),
	}
	if len(groups) == 1 {
		return parsed, true
	}
	secondGroup := strings.TrimSpace(groups[1])
	if secondGroup == "" {
		return phgoFastaHeader{}, false
	}
	switch {
	case strings.EqualFold(secondGroup, "h"):
		if len(groups) != 2 {
			return phgoFastaHeader{}, false
		}
		parsed.IsBlastQuerySource = true
		return parsed, true
	case isPositiveInteger(secondGroup):
		if len(groups) != 2 {
			return phgoFastaHeader{}, false
		}
		row, _ := strconv.Atoi(secondGroup)
		parsed.RowNumber = row
		parsed.HasRowPart = true
		return parsed, true
	default:
		sourceParts := strings.Split(secondGroup, "/")
		if len(sourceParts) != 2 {
			return phgoFastaHeader{}, false
		}
		for i := range sourceParts {
			sourceParts[i] = strings.TrimSpace(sourceParts[i])
		}
		if sourceParts[0] == "" || sourceParts[1] == "" {
			return phgoFastaHeader{}, false
		}
		parsed.BlastSourceLabelName = phgoHeaderFieldValue(sourceParts[0])
		parsed.BlastSourceGeneID = phgoHeaderFieldValue(sourceParts[1])
		if len(groups) == 4 {
			canvasParts := strings.SplitN(strings.TrimSpace(groups[2]), "/", 2)
			if len(canvasParts) != 2 {
				return phgoFastaHeader{}, false
			}
			rawPart := strings.TrimSpace(canvasParts[0])
			if !isSignedInteger(rawPart) {
				return phgoFastaHeader{}, false
			}
			raw, _ := strconv.Atoi(rawPart)
			title := strings.TrimSpace(canvasParts[1])
			displayName := strings.TrimSpace(groups[3])
			if title == "" || displayName == "" {
				return phgoFastaHeader{}, false
			}
			parsed.CanvasRawRowNumber = raw
			parsed.CanvasHasRawRow = true
			parsed.CanvasItemTitle = phgoHeaderFieldValue(title)
			parsed.CanvasDisplayName = phgoHeaderFieldValue(displayName)
			parsed.IsCanvasHeader = true
			if raw > 0 {
				parsed.RowNumber = raw
				parsed.HasRowPart = true
			}
			return parsed, true
		}
		if len(groups) == 3 {
			rowPart := strings.TrimSpace(groups[2])
			if !isPositiveInteger(rowPart) {
				return phgoFastaHeader{}, false
			}
			row, _ := strconv.Atoi(rowPart)
			parsed.RowNumber = row
			parsed.HasRowPart = true
		}
		return parsed, true
	}
}

func parsePhgoLiteFastaHeader(header string) (phgoFastaHeader, bool) {
	header = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(header), ">"))
	pipe := strings.LastIndex(header, "|")
	if pipe <= 0 || pipe >= len(header)-1 {
		return phgoFastaHeader{}, false
	}
	species := strings.TrimSpace(header[:pipe])
	idAndLabel := strings.TrimSpace(header[pipe+1:])
	id := idAndLabel
	label := ""
	hasSymbolSuffix := false
	if strings.HasSuffix(idAndLabel, ")") {
		if open := strings.LastIndex(idAndLabel, "("); open > 0 {
			if strings.TrimSpace(idAndLabel[:open]) != idAndLabel[:open] {
				return phgoFastaHeader{}, false
			}
			id = strings.TrimSpace(idAndLabel[:open])
			label = strings.TrimSpace(idAndLabel[open+1 : len(idAndLabel)-1])
			hasSymbolSuffix = true
		}
	}
	if species == "" || id == "" {
		return phgoFastaHeader{}, false
	}
	if !hasSymbolSuffix && strings.ContainsAny(species, " \t") {
		return phgoFastaHeader{}, false
	}
	return phgoFastaHeader{
		RawHeader:    ">" + header,
		Species:      phgoHeaderFieldValue(species),
		GeneID:       phgoHeaderFieldValue(id),
		IsLiteHeader: true,
		LabelName: func() string {
			if isPhgoLiteEmptyField(label) {
				return ""
			}
			return phgoHeaderFieldValue(label)
		}(),
	}, true
}

func phgoHeaderFieldValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "~" {
		return ""
	}
	return value
}

func isSignedInteger(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	digits := value
	if strings.HasPrefix(digits, "-") || strings.HasPrefix(digits, "+") {
		digits = strings.TrimSpace(digits[1:])
	}
	if digits == "" {
		return false
	}
	for _, ch := range digits {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	_, err := strconv.Atoi(value)
	return err == nil
}

func fastaHeaderPrimaryID(header string) string {
	if parsed, ok := parsePhgoFastaHeader(header); ok {
		return parsed.GeneID
	}
	return ""
}

func splitSingleLinePhgoHeaderAndSequence(headerLine string) (string, string) {
	headerLine = strings.TrimSpace(headerLine)
	if headerLine == "" {
		return "", ""
	}
	if parsed, ok := parsePhgoFastaHeader(headerLine); ok {
		_ = parsed
		return headerLine, ""
	}
	for i := len(headerLine) - 1; i >= 0; i-- {
		if headerLine[i] != ' ' && headerLine[i] != '\t' {
			continue
		}
		header := strings.TrimSpace(headerLine[:i])
		sequence := sanitizeSequence(headerLine[i+1:])
		if header == "" || sequence == "" {
			continue
		}
		if _, ok := parsePhgoFastaHeader(header); ok {
			return header, sequence
		}
	}
	return "", ""
}

func isPositiveInteger(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	row, err := strconv.Atoi(value)
	return err == nil && row > 0
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
	currentProgress := func() int {
		progressMu.Lock()
		defer progressMu.Unlock()
		return completedCount
	}
	advanceProgress := func() {
		progressMu.Lock()
		completedCount++
		current := completedCount
		progressMu.Unlock()
		progress(current, fmt.Sprintf("Searching keyword terms... %d/%d", current, len(keywords)))
	}

	for cursor := 0; cursor < len(pending); {
		remaining := len(pending) - cursor
		batchSize := keywordSearchWorkerCountForSource(w.source, species, remaining)
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
				progress(currentProgress(), fmt.Sprintf("Searching keyword term %d/%d: %s", resultIndex+1, len(keywords), strings.TrimSpace(keywords[resultIndex])))
				rows, err := w.searchKeywordRowsWithTimeout(ctx, species, keywords[resultIndex], forceWideSearch)
				if err == nil && len(rows) == 0 {
					err = keywordNoRowsError{Keyword: keywords[resultIndex]}
				}
				if err == nil {
					w.storeKeywordRowsForCurrentSource(species, keywords[resultIndex], rows)
				}
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
	if ctx == nil {
		ctx = context.Background()
	}
	searchCtx, cancel := context.WithCancel(ctx)
	timeout := configuredDurationSeconds("PHGO_KEYWORD_SEARCH_TERM_TIMEOUT_SECONDS", 0)
	if timeout > 0 {
		searchCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	if forceWideSearch {
		if wideSource, ok := w.source.(wideKeywordSearcher); ok {
			return wideSource.SearchKeywordRowsWide(searchCtx, species, keyword)
		}
	}
	return w.source.SearchKeywordRows(searchCtx, species, keyword)
}

func (w *BlastWizard) storeKeywordRowsForCurrentSource(species model.SpeciesCandidate, term string, rows []model.KeywordResultRow) {
	if w == nil || w.source == nil {
		return
	}
	cacheKey := w.keywordTermRowsCacheKey(w.source, species, term)
	w.storeKeywordTermRows(cacheKey, rows)
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

func assignKeywordGroupSearchTerm(rows []model.KeywordResultRow, searchTerm string) []model.KeywordResultRow {
	searchTerm = strings.TrimSpace(searchTerm)
	if searchTerm == "" {
		return rows
	}
	for i := range rows {
		rows[i].SearchTerm = searchTerm
	}
	return rows
}

type ncbiReplacementPlan struct {
	groupIndex   int
	oldAccession string
	newAccession string
	decision     string
}

func (w *BlastWizard) applyNCBIReplacementChoicesWithProgress(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup) ([]model.KeywordSearchGroup, error) {
	if len(groups) == 0 || !strings.EqualFold(sourceDatabaseName(w.source), "ncbi") {
		return groups, nil
	}
	out := cloneKeywordSearchGroups(groups)
	plans := make([]ncbiReplacementPlan, 0, len(out))
	needsReload := false
	for groupIndex := range out {
		oldAccession, newAccession, ok := ncbiGroupReplacement(out[groupIndex])
		if !ok {
			continue
		}
		choice := strings.TrimSpace(w.ncbiReplacementChoice)
		if choice == "" {
			if w.prompt == nil {
				choice = "old"
			} else {
				selectedChoice, err := w.prompt.NCBIReplacementAction(oldAccession, newAccession)
				if err != nil {
					return groups, err
				}
				switch selectedChoice {
				case "all_old":
					w.ncbiReplacementChoice = "old"
					choice = "old"
				case "all_new":
					w.ncbiReplacementChoice = "new"
					choice = "new"
				default:
					choice = selectedChoice
				}
			}
		}
		if choice == "new" {
			needsReload = true
		}
		plans = append(plans, ncbiReplacementPlan{
			groupIndex:   groupIndex,
			oldAccession: oldAccession,
			newAccession: newAccession,
			decision:     choice,
		})
	}
	if len(plans) == 0 {
		return out, nil
	}

	run := func(taskCtx context.Context, update func(int, string)) ([]model.KeywordSearchGroup, error) {
		runCtx := mergeContexts(ctx, taskCtx)
		progress := safeProgress(update)
		applied := cloneKeywordSearchGroups(out)
		for i, plan := range plans {
			label := firstNonEmpty(strings.TrimSpace(applied[plan.groupIndex].SearchTerm), plan.oldAccession)
			switch plan.decision {
			case "new":
				progress(i, fmt.Sprintf("Reloading updated NCBI record %d/%d (%s)...", i+1, len(plans), oneLinePreview(label)))
				rows, err := w.source.SearchKeywordRows(runCtx, selected, plan.newAccession)
				if err != nil {
					return nil, err
				}
				if len(rows) == 0 {
					return nil, keywordNoRowsError{Keyword: plan.newAccession}
				}
				rows = annotateNCBIReplacementRows(rows, plan.oldAccession, plan.newAccession, "new")
				rows = assignKeywordGroupSearchTerm(rows, applied[plan.groupIndex].SearchTerm)
				applied[plan.groupIndex].Rows = rows
				applied[plan.groupIndex].SearchType = keywordRowsSearchType(rows, applied[plan.groupIndex].SearchTerm, false)
			default:
				progress(i, fmt.Sprintf("Keeping requested NCBI record %d/%d (%s)...", i+1, len(plans), oneLinePreview(label)))
				applied[plan.groupIndex].Rows = annotateNCBIReplacementRows(applied[plan.groupIndex].Rows, plan.oldAccession, plan.newAccession, "old")
				applied[plan.groupIndex].SearchType = keywordRowsSearchType(applied[plan.groupIndex].Rows, applied[plan.groupIndex].SearchTerm, false)
			}
		}
		progress(len(plans), "NCBI updates are ready.")
		return applied, nil
	}
	if w.suppressTaskModals || !needsReload {
		applied, err := run(ctx, nil)
		if err != nil {
			return groups, err
		}
		return applied, nil
	}
	applied, err := tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Keyword", "NCBI update"),
		Title:       "Loading updated NCBI records",
		Description: "Reloading replacement records after your NCBI update choices.",
		Initial:     "Loading updated NCBI records...",
		Total:       len(plans),
		CancelError: prompt.ErrBackToQueryInput,
	}, run)
	if err != nil {
		return groups, err
	}
	return applied, nil
}

func (w *BlastWizard) applyNCBIReplacementChoices(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup) ([]model.KeywordSearchGroup, error) {
	return w.applyNCBIReplacementChoicesWithProgress(ctx, selected, groups)
}

func ncbiGroupReplacement(group model.KeywordSearchGroup) (string, string, bool) {
	for _, row := range group.Rows {
		if !strings.EqualFold(strings.TrimSpace(row.SourceDatabase), "ncbi") {
			continue
		}
		if row.ExtraColumns == nil {
			continue
		}
		replacedBy := strings.TrimSpace(row.ExtraColumns["ncbi_replaced_by"])
		if replacedBy == "" {
			continue
		}
		oldAccession := firstNonEmpty(
			strings.TrimSpace(row.ExtraColumns["ncbi_accession"]),
			strings.TrimSpace(row.ExtraColumns["ncbi_clinvar_accession"]),
			strings.TrimSpace(row.ExtraColumns["ncbi_gtr_accession"]),
			strings.TrimSpace(row.ExtraColumns["ncbi_dbvar_accession"]),
			strings.TrimSpace(row.ExtraColumns["ncbi_bioproject_accession"]),
			strings.TrimSpace(row.ExtraColumns["ncbi_biosample_accession"]),
			strings.TrimSpace(row.ExtraColumns["ncbi_assembly_accession"]),
			strings.TrimSpace(row.ExtraColumns["ncbi_omim_id"]),
			strings.TrimSpace(row.ExtraColumns["ncbi_medgen_id"]),
			strings.TrimSpace(row.ExtraColumns["ncbi_rsid"]),
			strings.TrimSpace(row.SequenceID),
			strings.TrimSpace(row.ProteinID),
			strings.TrimSpace(row.GeneIdentifier),
			strings.TrimSpace(group.SearchTerm),
		)
		if oldAccession == "" {
			continue
		}
		return oldAccession, replacedBy, true
	}
	return "", "", false
}

func annotateNCBIReplacementRows(rows []model.KeywordResultRow, oldAccession string, newAccession string, decision string) []model.KeywordResultRow {
	out := cloneKeywordResultRows(rows)
	for i := range out {
		if out[i].ExtraColumns == nil {
			out[i].ExtraColumns = make(map[string]string)
		}
		out[i].ExtraColumns["ncbi_requested_accession"] = strings.TrimSpace(oldAccession)
		out[i].ExtraColumns["ncbi_replacement_accession"] = strings.TrimSpace(newAccession)
		out[i].ExtraColumns["ncbi_replacement_decision"] = strings.TrimSpace(decision)
		baseSearchType := strings.TrimSpace(out[i].SearchType)
		if baseSearchType == "" {
			baseSearchType = classifyKeywordInputType(firstNonEmpty(out[i].SearchTerm, oldAccession))
		}
		switch strings.TrimSpace(decision) {
		case "new":
			out[i].SearchType = baseSearchType + " (accepted NCBI update)"
		case "old":
			out[i].SearchType = baseSearchType + " (kept old; NCBI update available)"
		}
	}
	return out
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

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
	"strings"
	"sync"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/ncbi"
	"github.com/KiriKirby/phytozome-go/internal/phytozome"
	"github.com/KiriKirby/phytozome-go/internal/plaza"
	"github.com/KiriKirby/phytozome-go/internal/prompt"
	"github.com/KiriKirby/phytozome-go/internal/source"
	"github.com/KiriKirby/phytozome-go/internal/tair"
	"github.com/KiriKirby/phytozome-go/internal/tui"
)

type mainInterfaceStateUpdate struct {
	state tui.MainInterfaceState
}

func (e mainInterfaceStateUpdate) Error() string {
	return "main interface state updated"
}

type mainAutoIdentifyChoice string

const (
	mainAutoIdentifyNone  mainAutoIdentifyChoice = ""
	mainAutoIdentifyClose mainAutoIdentifyChoice = "close"
	mainAutoIdentifySkip  mainAutoIdentifyChoice = "skip"
	mainAutoIdentifyAuto  mainAutoIdentifyChoice = "auto"
)

func newMainInterfaceEnabled() bool {
	return true
}

func (w *BlastWizard) runNewMainInterface(ctx context.Context) error {
	state := tui.DefaultMainInterfaceState()
	for {
		result, err := tui.RunMainInterfacePage(tui.MainInterfacePage{
			Info:          w.tuiInfo,
			State:         state,
			SpeciesLoader: w.mainInterfaceSpeciesOptionsLoader(ctx),
		})
		if err != nil {
			return err
		}
		state = tui.NormalizeMainInterfaceState(result.State)
		action := result.Action
	forAction:
		for {
			var actionErr error
			switch action {
			case tui.MainActionExit, "":
				return nil
			case tui.MainActionExploreTool:
				actionErr = w.runMainExploreAction(ctx, state.Explore.Tool)
			case tui.MainActionKeywordSearch, tui.MainActionKeywordWideSearch:
				actionErr = w.runMainKeywordAction(ctx, state, action == tui.MainActionKeywordWideSearch)
			case tui.MainActionBlastRun:
				actionErr = w.runMainBlastAction(ctx, state)
			default:
				actionErr = fmt.Errorf("unsupported main interface action %q", action)
			}
			if actionErr == nil {
				break forAction
			}
			var update mainInterfaceStateUpdate
			if errors.As(actionErr, &update) {
				state = tui.NormalizeMainInterfaceState(update.state)
				break forAction
			}
			if errors.Is(actionErr, prompt.ErrExitRequested) {
				return actionErr
			}
			if isMainInterfaceBack(actionErr) {
				break forAction
			}
			decision, recoveryErr := w.recoverUnexpectedWorkflowError(actionErr, prompt.ErrBackToDatabaseSelection)
			if recoveryErr != nil {
				if errors.Is(recoveryErr, prompt.ErrExitRequested) {
					return recoveryErr
				}
				break forAction
			}
			if decision != recoveryRetry {
				break forAction
			}
		}
	}
}

func isMainInterfaceBack(err error) bool {
	return errors.Is(err, prompt.ErrBackToDatabaseSelection) ||
		errors.Is(err, prompt.ErrBackToModeSelection) ||
		errors.Is(err, prompt.ErrBackToSpeciesSelection) ||
		errors.Is(err, prompt.ErrBackToQueryInput) ||
		errors.Is(err, prompt.ErrBackToRowSelection) ||
		errors.Is(err, prompt.ErrBackToBlastProgram) ||
		errors.Is(err, tui.ErrTaskCancelled)
}

func (w *BlastWizard) mainInterfaceSpeciesOptionsLoader(parent context.Context) tui.MainSpeciesLoader {
	return func(ctx context.Context, request tui.MainSpeciesRequest) ([]tui.MainSpeciesOption, error) {
		if parent != nil {
			ctx = mergeContexts(parent, ctx)
		}
		mode := ModeKeyword
		if strings.EqualFold(request.Mode, "blast") {
			mode = ModeBlast
		}
		src, err := w.mainInterfaceDataSource(request.DatabaseID)
		if err != nil {
			return nil, err
		}
		candidates, err := w.mainInterfaceSpeciesCandidates(ctx, src, mode)
		if err != nil {
			return nil, err
		}
		options := make([]tui.MainSpeciesOption, 0, len(candidates))
		for _, candidate := range candidates {
			options = append(options, tui.MainSpeciesOption{
				Key:         mainInterfaceSpeciesKey(candidate),
				Label:       mainInterfaceSpeciesShortLabel(candidate),
				Description: mainInterfaceSpeciesDescription(candidate),
				SearchText:  mainInterfaceSpeciesSearchText(candidate),
			})
		}
		return options, nil
	}
}

func (w *BlastWizard) mainInterfaceSpeciesCandidates(ctx context.Context, src source.DataSource, mode QueryMode) ([]model.SpeciesCandidate, error) {
	candidates, err := w.speciesCandidatesForSource(ctx, src, nil)
	if err != nil {
		return nil, err
	}
	filtered := candidates
	switch typed := src.(type) {
	case *tair.Client:
		filtered = typed.FilterCandidatesForMode(candidates, string(mode))
	}
	if len(filtered) > 0 {
		return filtered, nil
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no species candidates were returned for %s", databaseDisplayName(src.Name()))
	}
	return nil, fmt.Errorf("no %s candidates are currently usable in %s mode", strings.ToUpper(strings.TrimSpace(src.Name())), strings.ToUpper(string(mode)))
}

func (w *BlastWizard) runMainExploreAction(ctx context.Context, tool string) error {
	switch strings.TrimSpace(tool) {
	case "tair_family":
		src := tair.NewClient(w.httpClient)
		w.source = src
		w.pendingMode = ModeFamily
		w.prompt.SetDatabaseContext(databaseDisplayName(src.Name()))
		candidates, err := w.loadSpeciesCandidatesForMode(ctx, ModeFamily)
		if err != nil {
			return err
		}
		selected, err := w.selectSpecies(candidates)
		if err != nil {
			return err
		}
		return w.runFamilyMode(ctx, selected)
	default:
		return w.runStartupTool(tool)
	}
}

func (w *BlastWizard) runMainKeywordAction(ctx context.Context, state tui.MainInterfaceState, wide bool) error {
	state = tui.NormalizeMainInterfaceState(state)
	src, err := w.mainInterfaceDataSource(state.Keyword.DatabaseID)
	if err != nil {
		return err
	}
	w.source = src
	w.pendingMode = ModeKeyword
	w.prompt.SetDatabaseContext(databaseDisplayName(src.Name()))
	w.setBlastProgramContext("")
	selected, err := w.mainInterfaceSelectedSpecies(ctx, src, ModeKeyword, state.Keyword.SpeciesKey, state.Keyword.SpeciesLabel, state.Keyword.SearchTypeID)
	if err != nil {
		return err
	}
	rows, rowIndexes := mainKeywordRowsForWorkflow(state.Keyword.Rows)
	if len(rows) == 0 {
		return w.showInfo("Keyword input", "Keyword input was empty. Please enter at least one search term.", prompt.ErrBackToQueryInput)
	}
	keywords := make([]string, 0, len(rows))
	manualLabels := make([]string, 0, len(rows))
	manualGeneLoci := make([]string, 0, len(rows))
	for _, row := range rows {
		keywords = append(keywords, row.SearchTerm)
		manualLabels = append(manualLabels, row.SymbolName)
		manualGeneLoci = append(manualGeneLoci, row.GeneLocus)
	}
	needsLabelAuto := mainKeywordMissingSymbolIndexes(state.Keyword.Rows, rowIndexes)
	needsGeneLocusAuto := []int(nil)
	if _, ok := src.(*ncbi.Client); ok && ncbi.SearchTypeByID(state.Keyword.SearchTypeID).ShowsGeneLocus {
		needsGeneLocusAuto = mainKeywordMissingGeneLocusIndexes(state.Keyword.Rows, rowIndexes)
	}
	var preloadedGroups []model.KeywordSearchGroup
	var preloaded bool
	var stateChanged bool
	priorityDatabase := strings.ToLower(strings.TrimSpace(state.Keyword.GeneLocusPriorityDatabase))
	if priorityDatabase == "" && state.Keyword.PLAZAGeneLocusPriority {
		priorityDatabase = tui.GeneLocusPriorityPLAZA
	}
	if _, isNCBI := src.(*ncbi.Client); isNCBI && (priorityDatabase == tui.GeneLocusPriorityNCBI || priorityDatabase == tui.GeneLocusPriorityPLAZA) && ncbi.SearchTypeByID(state.Keyword.SearchTypeID).ShowsGeneLocus {
		preloadedGroups, err = w.searchMainKeywordGroupsWithGeneLocusPriority(ctx, selected, keywords, manualGeneLoci, wide, priorityDatabase)
		if err != nil {
			return err
		}
		preloadedGroups, err = w.applyNCBIReplacementChoicesWithProgress(ctx, selected, preloadedGroups)
		if err != nil {
			return err
		}
		preloaded = true
	}
	if len(needsLabelAuto) > 0 {
		choice, err := w.confirmMainAutoIdentify("Symbol name", true, "Some rows have a search term but no Symbol name. Auto identify the blank Symbol name cells before search?", true)
		if err != nil {
			return err
		}
		switch choice {
		case mainAutoIdentifyClose:
			return prompt.ErrBackToQueryInput
		case mainAutoIdentifyAuto:
			if !preloaded {
				preloadedGroups, err = w.loadMainKeywordGroupsForAutoIdentify(ctx, selected, keywords, wide)
				if err != nil {
					return err
				}
				preloaded = true
			}
			identifications, err := w.autoIdentifyKeywordLabelsWithProgress(ctx, selected, preloadedGroups)
			if err != nil {
				return err
			}
			mainApplyKeywordSymbolIdentifications(&state, rowIndexes, needsLabelAuto, identifications)
			stateChanged = true
		}
	}
	if len(needsGeneLocusAuto) > 0 {
		choice, err := w.confirmMainAutoIdentify("Gene locus", true, "Some rows have a search term but no Gene locus. Auto identify the blank Gene locus cells before search?", true)
		if err != nil {
			return err
		}
		switch choice {
		case mainAutoIdentifyClose:
			return prompt.ErrBackToQueryInput
		case mainAutoIdentifyAuto:
			if !preloaded {
				preloadedGroups, err = w.loadMainKeywordGroupsForAutoIdentify(ctx, selected, keywords, wide)
				if err != nil {
					return err
				}
				preloaded = true
			}
			if err := w.autoIdentifyNCBIKeywordGeneLociWithProgress(ctx, preloadedGroups); err != nil {
				return err
			}
			mainApplyKeywordGeneLoci(&state, rowIndexes, needsGeneLocusAuto, preloadedGroups)
			stateChanged = true
		}
	}
	if stateChanged {
		state.ActiveTab = "keyword"
		return mainInterfaceStateUpdate{state: state}
	}
	return w.executeMainKeywordRows(ctx, selected, keywords, manualLabels, manualGeneLoci, wide, false, false, preloadedGroups)
}

func (w *BlastWizard) loadMainKeywordGroupsForAutoIdentify(ctx context.Context, selected model.SpeciesCandidate, keywords []string, wide bool) ([]model.KeywordSearchGroup, error) {
	groups, err := w.searchKeywordGroups(ctx, selected, keywords, nil, wide)
	if err != nil {
		return nil, err
	}
	return w.applyNCBIReplacementChoicesWithProgress(ctx, selected, groups)
}

func (w *BlastWizard) searchMainKeywordGroupsWithPLAZAPriority(ctx context.Context, selected model.SpeciesCandidate, keywords []string, geneLoci []string, wide bool) ([]model.KeywordSearchGroup, error) {
	return w.searchMainKeywordGroupsWithGeneLocusPriority(ctx, selected, keywords, geneLoci, wide, tui.GeneLocusPriorityPLAZA)
}

func (w *BlastWizard) searchMainKeywordGroupsWithGeneLocusPriority(ctx context.Context, selected model.SpeciesCandidate, keywords []string, geneLoci []string, wide bool, priorityDatabase string) ([]model.KeywordSearchGroup, error) {
	priorityDatabase = strings.ToLower(strings.TrimSpace(priorityDatabase))
	if priorityDatabase != tui.GeneLocusPriorityNCBI && priorityDatabase != tui.GeneLocusPriorityPLAZA {
		return nil, fmt.Errorf("unsupported Gene locus priority database %q", priorityDatabase)
	}
	databaseLabel := "NCBI"
	if priorityDatabase == tui.GeneLocusPriorityPLAZA {
		databaseLabel = "PLAZA"
	}
	skipped := make(map[int]bool)
	for {
		groups, err := tui.RunProgressTaskValueContext(tui.TaskPage{
			Path:        w.tuiPath("Keyword", "Searching"),
			Title:       "Searching " + databaseLabel + " and NCBI keyword terms",
			Description: "Using " + databaseLabel + " Gene locus priority where a locus is available.",
			Initial:     "Searching " + databaseLabel + " and NCBI keyword terms...",
			Total:       len(keywords),
			CancelError: prompt.ErrBackToQueryInput,
		}, func(taskCtx context.Context, update func(int, string)) ([]model.KeywordSearchGroup, error) {
			return w.searchMainKeywordGroupsWithGeneLocusPriorityProgressSkipping(mergeContexts(ctx, taskCtx), selected, keywords, geneLoci, wide, priorityDatabase, skipped, update)
		})
		if err == nil {
			return groups, nil
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
		if decision == recoverySkip {
			skipped[recoverErr.Index] = true
		}
	}
}

func (w *BlastWizard) searchMainKeywordGroupsWithPLAZAPriorityProgress(ctx context.Context, selected model.SpeciesCandidate, keywords []string, geneLoci []string, wide bool, update func(int, string)) ([]model.KeywordSearchGroup, error) {
	return w.searchMainKeywordGroupsWithGeneLocusPriorityProgress(ctx, selected, keywords, geneLoci, wide, tui.GeneLocusPriorityPLAZA, update)
}

func (w *BlastWizard) searchMainKeywordGroupsWithGeneLocusPriorityProgress(ctx context.Context, selected model.SpeciesCandidate, keywords []string, geneLoci []string, wide bool, priorityDatabase string, update func(int, string)) ([]model.KeywordSearchGroup, error) {
	return w.searchMainKeywordGroupsWithGeneLocusPriorityProgressSkipping(ctx, selected, keywords, geneLoci, wide, priorityDatabase, nil, update)
}

func (w *BlastWizard) searchMainKeywordGroupsWithGeneLocusPriorityProgressSkipping(ctx context.Context, selected model.SpeciesCandidate, keywords []string, geneLoci []string, wide bool, priorityDatabase string, skipped map[int]bool, update func(int, string)) ([]model.KeywordSearchGroup, error) {
	if len(keywords) != len(geneLoci) {
		return nil, fmt.Errorf("Gene locus values (%d) do not match keyword rows (%d)", len(geneLoci), len(keywords))
	}
	progress := safeProgress(update)
	results := make([]keywordSearchResult, len(keywords))
	var lookup *plaza.Client
	if priorityDatabase == tui.GeneLocusPriorityPLAZA {
		lookup = plaza.NewClient(w.httpClient)
	}
	workers := keywordSearchWorkerCountForSource(w.source, selected, len(keywords))
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	var progressMu sync.Mutex
	completed := 0
	advance := func() {
		progressMu.Lock()
		completed++
		current := completed
		progressMu.Unlock()
		progress(current, fmt.Sprintf("Searched keyword terms... %d/%d", current, len(keywords)))
	}
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				started := time.Now()
				if skipped[index] {
					results[index] = keywordSearchResult{index: index, started: started, ended: time.Now()}
					advance()
					continue
				}
				var rows []model.KeywordResultRow
				var err error
				if locus := strings.TrimSpace(geneLoci[index]); locus != "" {
					// Priority lookup failures are deliberately non-fatal: the original
					// search term remains the documented per-row fallback.
					if priorityDatabase == tui.GeneLocusPriorityPLAZA && lookup != nil {
						rows, _ = lookup.SearchGeneLocus(ctx, locus)
					} else if priorityDatabase == tui.GeneLocusPriorityNCBI {
						rows, _ = w.searchKeywordRowsWithTimeout(ctx, selected, locus, wide)
						if len(rows) > 0 {
							for rowIndex := range rows {
								rows[rowIndex].SearchType = "NCBI Gene locus priority"
							}
						}
					}
				}
				if len(rows) == 0 {
					rows, err = w.searchKeywordRowsWithTimeout(ctx, selected, keywords[index], wide)
				}
				if err == nil && len(rows) == 0 {
					err = keywordNoRowsError{Keyword: keywords[index]}
				}
				results[index] = keywordSearchResult{index: index, started: started, ended: time.Now(), rows: rows, err: err}
				advance()
			}
		}()
	}
	for index := range keywords {
		if err := ctx.Err(); err != nil {
			break
		}
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for index, result := range results {
		if result.err != nil {
			return nil, &keywordSearchRecoveryError{Result: result, Keyword: keywords[index], Index: index, Total: len(keywords), Err: result.err}
		}
		w.storeKeywordRowsForCurrentSource(selected, keywords[index], result.rows)
	}
	return buildKeywordSearchGroups(keywords, nil, results, wide), nil
}

func (w *BlastWizard) executeMainKeywordRows(ctx context.Context, selected model.SpeciesCandidate, keywords []string, manualLabels []string, manualGeneLoci []string, wide bool, autoIdentifyLabels bool, autoIdentifyGeneLoci bool, preloadedGroups []model.KeywordSearchGroup) error {
	queryStarted := time.Now()
	identifications := manualKeywordLabelIdentifications(manualLabels, len(keywords))
	groups := cloneKeywordSearchGroups(preloadedGroups)
	var err error
	if len(groups) == 0 {
		groups, err = w.searchKeywordGroups(ctx, selected, keywords, nil, wide)
		if err != nil {
			return err
		}
		groups, err = w.applyNCBIReplacementChoicesWithProgress(ctx, selected, groups)
		if err != nil {
			return err
		}
	}
	if autoIdentifyLabels {
		identifications, err = w.autoIdentifyKeywordLabelsWithProgress(ctx, selected, groups)
		if err != nil {
			return err
		}
	}
	if len(manualGeneLoci) == len(keywords) {
		applyKeywordGeneLoci(groups, manualGeneLoci, "user input")
	}
	if autoIdentifyGeneLoci {
		if err := w.autoIdentifyNCBIKeywordGeneLociWithProgress(ctx, groups); err != nil {
			return err
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
	reportCtx := &keywordReportRunContext{
		Selected:     selected,
		QueryStarted: queryStarted,
		SearchEnded:  keywordGroupsSearchEndedAt(groups),
		LabelMode:    labelMode,
	}
	return w.reviewMainKeywordGroups(ctx, selected, groups, reportCtx)
}

func (w *BlastWizard) reviewMainKeywordGroups(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup, reportCtx *keywordReportRunContext) error {
	totalRows := countKeywordRows(groups)
	if totalRows == 0 {
		w.postRunBackTarget = prompt.ErrBackToQueryInput
		return w.showInfo("Keyword results", fmt.Sprintf("No keyword results were found in %s.\n\nThese identifiers may belong to a different species or may not exist in this proteome.", selected.DisplayLabel()), prompt.ErrBackToQueryInput)
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
			return err
		}
		if len(selection.Rows) == 0 {
			if err := w.showInfo("Keyword export", "No rows selected. Export will be skipped.", prompt.ErrBackToRowSelection); err != nil {
				if errors.Is(err, prompt.ErrBackToRowSelection) {
					continue
				}
				return err
			}
			continue
		}
		if selection.RunBlast {
			if err := w.runKeywordBlastMode(ctx, selected, groups, selection.Rows, reportCtx); err != nil {
				if errors.Is(err, prompt.ErrBackToRowSelection) {
					continue
				}
				return err
			}
			continue
		}
		if selection.CreateCanvas {
			if err := w.runKeywordRowsCanvasMode(ctx, selected, groups, selection.Rows, selection.SelectedByGroup); err != nil {
				if errors.Is(err, prompt.ErrBackToRowSelection) {
					continue
				}
				return err
			}
			continue
		}
		w.warmKeywordSequenceCache(ctx, selected, groups)
		w.postRunBackTarget = prompt.ErrBackToRowSelection
		if !selection.GenerateFile {
			continue
		}
		if err := w.prepareAndExportKeywordSelectionWithMask(ctx, selected, groups, selection.Rows, selection.Selected, ModeKeyword, reportCtx); err != nil {
			if errors.Is(err, prompt.ErrBackToRowSelection) {
				continue
			}
			return err
		}
	}
}

func (w *BlastWizard) runMainBlastAction(ctx context.Context, state tui.MainInterfaceState) error {
	state = tui.NormalizeMainInterfaceState(state)
	src, err := w.mainInterfaceDataSource(state.Blast.DatabaseID)
	if err != nil {
		return err
	}
	w.source = src
	w.pendingMode = ModeBlast
	w.prompt.SetDatabaseContext(databaseDisplayName(src.Name()))
	selected, err := w.mainInterfaceSelectedSpecies(ctx, src, ModeBlast, state.Blast.SpeciesKey, state.Blast.SpeciesLabel, "")
	if err != nil {
		return err
	}
	rows := tui.MainBlastRowsForExecution(state.Blast.Rows)
	if len(rows) == 0 {
		return w.showInfo("BLAST input", "BLAST input was empty. Please paste one or more queries.", prompt.ErrBackToQueryInput)
	}
	_, rowIndexes := mainBlastRowsForWorkflow(state.Blast.Rows)
	var rawRecords []string
	for _, row := range rows {
		rawRecords = append(rawRecords, row.FASTA)
	}
	items, err := parseBlastQueryItems(strings.Join(rawRecords, "\n\n"))
	if err != nil {
		return err
	}
	if len(items) != len(rows) {
		return fmt.Errorf("BLAST content produced %d parsed queries from %d input rows; keep each FASTA or sequence as one row", len(items), len(rows))
	}
	for i := range items {
		if i < len(rows) && strings.TrimSpace(rows[i].SymbolName) != "" {
			setBlastQueryItemLabel(&items[i], rows[i].SymbolName)
		}
		if i < len(rows) && strings.TrimSpace(rows[i].GeneLocus) != "" {
			setBlastQueryItemGeneLocus(&items[i], rows[i].GeneLocus)
		}
	}
	requestConfig, err := w.mainBlastRequestConfig(ctx, selected, state.Blast.ProgramID)
	if err != nil {
		return err
	}
	candidates, err := w.speciesCandidatesForSource(ctx, src, nil)
	if err != nil {
		return err
	}
	prepared, err := w.resolveBlastQueryItems(ctx, items, candidates)
	if err != nil {
		return err
	}
	needsLabelAuto := mainBlastMissingSymbolIndexes(state.Blast.Rows, rowIndexes)
	if len(needsLabelAuto) > 0 {
		choice, err := w.confirmMainAutoIdentify("Symbol name", true, "Some BLAST rows have input but no Symbol name. Auto identify the blank Symbol name cells before running BLAST?", len(rows) <= 1)
		if err != nil {
			return err
		}
		switch choice {
		case mainAutoIdentifyClose:
			return prompt.ErrBackToQueryInput
		case mainAutoIdentifySkip:
		case mainAutoIdentifyAuto:
			prepared, err = w.autoIdentifyBlastLabelsWithProgress(ctx, selected, prepared)
			if err != nil {
				return err
			}
			mainApplyBlastSymbolIdentifications(&state, rowIndexes, needsLabelAuto, prepared)
			state.ActiveTab = "blast"
			return mainInterfaceStateUpdate{state: state}
		}
	}
	if allLabelsPresent(prepared) {
		prepared, err = w.supplementBlastAliasesWithProgress(ctx, selected, prepared)
		if err != nil {
			return err
		}
	}
	return w.executePreparedBlast(ctx, selected, prepared, requestConfig)
}

func (w *BlastWizard) mainBlastRequestConfig(ctx context.Context, selected model.SpeciesCandidate, program string) (blastRequestConfig, error) {
	switch src := w.source.(type) {
	case *lemna.Client:
		request := mainDefaultBlastRequest(selected)
		applyBlastProgram(&request, program)
		cap, err := w.detectLemnaBlastCapabilities(ctx, src, selected, "Preparing BLAST program selection")
		if err != nil {
			return blastRequestConfig{}, err
		}
		execChoice, err := w.chooseLemnaBlastExecution(cap, selected, program)
		if err != nil {
			return blastRequestConfig{}, err
		}
		if execChoice == "local" {
			request.Program = "local:" + request.Program
		}
		w.setBlastProgramContext(blastProgramPathLabel(request.Program))
		return blastRequestConfig{Request: request, Ready: true}, nil
	case *tair.Client:
		request := mainDefaultBlastRequest(selected)
		applyBlastProgram(&request, program)
		request.Program = "local:" + request.Program
		w.setBlastProgramContext(blastProgramPathLabel(request.Program))
		return blastRequestConfig{Request: request, Ready: true}, nil
	default:
		w.setBlastProgramContext("")
		return blastRequestConfig{}, nil
	}
}

func mainDefaultBlastRequest(selected model.SpeciesCandidate) model.BlastRequest {
	return model.BlastRequest{
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
	}
}

func setBlastQueryItemGeneLocus(item *blastQueryItem, locus string) {
	if item == nil {
		return
	}
	locus = strings.TrimSpace(locus)
	if locus == "" {
		return
	}
	if item.QuerySource == nil {
		item.QuerySource = &model.QuerySequenceSource{
			Sequence:  strings.TrimSpace(firstNonEmpty(item.Sequence, item.ProteinSequence, item.NucleotideSequence)),
			LabelName: strings.TrimSpace(item.LabelName),
		}
	}
	item.QuerySource.GeneID = locus
	item.QuerySource.TranscriptID = ""
	item.QuerySource.ProteinID = ""
	item.QuerySource.PreferredSequenceID = firstNonEmpty(item.QuerySource.PreferredSequenceID, locus)
}

func allBlastGeneLociPresent(items []blastQueryItem) bool {
	for _, item := range items {
		if strings.TrimSpace(blastQueryItemGeneLocus(item)) == "" {
			return false
		}
	}
	return true
}

func blastQueryItemGeneLocus(item blastQueryItem) string {
	if item.QuerySource != nil {
		return strings.TrimSpace(item.QuerySource.GeneID)
	}
	return ""
}

func (w *BlastWizard) autoIdentifyMainBlastGeneLociWithProgress(ctx context.Context, selected model.SpeciesCandidate, items []blastQueryItem) ([]blastQueryItem, error) {
	missing := mainBlastGeneLocusMissingIndexes(items)
	if len(missing) == 0 {
		return items, nil
	}
	run := func(taskCtx context.Context, update func(string)) ([]blastQueryItem, error) {
		return w.autoIdentifyMainBlastGeneLoci(mergeContexts(ctx, taskCtx), selected, items, safeTaskUpdate(update))
	}
	if w.suppressTaskModals {
		return run(ctx, nil)
	}
	return tui.RunTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("BLAST", "Gene locus"),
		Title:       "Auto identifying BLAST Gene locus",
		Description: "Reading source-species keyword metadata for BLAST query Gene locus values.",
		Initial:     "Auto identifying BLAST Gene locus values...",
		CancelError: prompt.ErrBackToQueryInput,
	}, run)
}

func (w *BlastWizard) autoIdentifyMainBlastGeneLoci(ctx context.Context, selected model.SpeciesCandidate, items []blastQueryItem, update func(string)) ([]blastQueryItem, error) {
	out := cloneBlastQueryItems(items)
	missing := mainBlastGeneLocusMissingIndexes(out)
	if len(missing) == 0 {
		return out, nil
	}
	lookupSource := phytozome.NewClient(w.httpClient)
	for done, idx := range missing {
		if idx < 0 || idx >= len(out) {
			continue
		}
		if update != nil {
			update(fmt.Sprintf("Finding BLAST Gene locus %d/%d...", done+1, len(missing)))
		}
		if locus := w.autoIdentifyMainBlastGeneLocus(ctx, lookupSource, selected, out[idx]); locus != "" {
			setBlastQueryItemGeneLocus(&out[idx], locus)
		}
	}
	if update != nil {
		update("BLAST Gene locus values are ready.")
	}
	return out, nil
}

func mainBlastGeneLocusMissingIndexes(items []blastQueryItem) []int {
	out := make([]int, 0, len(items))
	for i, item := range items {
		if strings.TrimSpace(blastQueryItemGeneLocus(item)) == "" {
			out = append(out, i)
		}
	}
	return out
}

func (w *BlastWizard) autoIdentifyMainBlastGeneLocus(ctx context.Context, lookupSource source.DataSource, selected model.SpeciesCandidate, item blastQueryItem) string {
	if locus := strings.TrimSpace(blastQueryItemGeneLocus(item)); locus != "" {
		return locus
	}
	if item.QuerySource != nil {
		if locus := mainBlastGeneLocusFromQuerySource(item.QuerySource); locus != "" {
			return locus
		}
	}
	for _, labelSpecies := range w.mainBlastGeneLocusLookupSpecies(ctx, lookupSource, selected, item) {
		rowsByTerm := w.fetchKeywordRowsByTerms(ctx, lookupSource, labelSpecies, blastLabelSearchTerms(item))
		for _, term := range blastLabelSearchTerms(item) {
			if locus := bestMainBlastGeneLocusFromKeywordRows(rowsByTerm[strings.ToLower(strings.TrimSpace(term))]); locus != "" {
				return locus
			}
		}
	}
	return ""
}

func (w *BlastWizard) mainBlastGeneLocusLookupSpecies(ctx context.Context, lookupSource source.DataSource, selected model.SpeciesCandidate, item blastQueryItem) []model.SpeciesCandidate {
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
	candidates, err := w.speciesCandidatesForSource(ctx, lookupSource, nil)
	if err != nil {
		add(selected)
		return out
	}
	if item.QuerySource != nil {
		if item.QuerySource.SourceJBrowseName != "" {
			if species, ok := findSpeciesCandidateByJBrowseName(candidates, item.QuerySource.SourceJBrowseName); ok {
				add(species)
				return out
			}
		}
		if item.QuerySource.SourceProteomeID > 0 {
			for _, candidate := range candidates {
				if candidate.ProteomeID == item.QuerySource.SourceProteomeID {
					add(candidate)
					return out
				}
			}
		}
		for _, value := range []string{item.QuerySource.SourceGenomeLabel, item.QuerySource.OrganismShort, item.QuerySource.Annotation} {
			if species, ok := matchPhytozomeSpeciesForFastaHeader(value, candidates); ok {
				add(species)
				return out
			}
		}
		if strings.EqualFold(strings.TrimSpace(item.QuerySource.SourceDatabase), "fasta") {
			if header, _ := splitFastaHeaderAndSequence(item.RawInput); header != "" {
				if species, ok := matchPhytozomeSpeciesForFastaHeader(header, candidates); ok {
					add(species)
					return out
				}
			}
		}
	}
	if _, ok := w.source.(*lemna.Client); ok {
		if species, ok := matchPhytozomeSpeciesForLemna(selected, candidates); ok {
			add(species)
			return out
		}
	}
	add(selected)
	return out
}

func mainBlastGeneLocusFromQuerySource(source *model.QuerySequenceSource) string {
	if source == nil {
		return ""
	}
	for _, value := range []string{
		source.GeneID,
		stripTranscriptDecorations(source.TranscriptID),
		stripTranscriptDecorations(source.ProteinID),
		stripTranscriptDecorations(source.PreferredSequenceID),
	} {
		if value = mainBlastGeneLocusCandidate(value); value != "" {
			return value
		}
	}
	return ""
}

func bestMainBlastGeneLocusFromKeywordRows(rows []model.KeywordResultRow) string {
	for _, row := range rows {
		if locus := firstNonEmpty(row.GeneLocus, stripTranscriptDecorations(row.GeneIdentifier), stripTranscriptDecorations(row.TranscriptID), stripTranscriptDecorations(row.ProteinID), row.SequenceID); locus != "" {
			return mainBlastGeneLocusCandidate(locus)
		}
	}
	return ""
}

func mainBlastGeneLocusCandidate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if stripped := stripTranscriptDecorations(value); strings.TrimSpace(stripped) != "" {
		value = strings.TrimSpace(stripped)
	}
	if dot := strings.LastIndex(value, "."); dot > 0 && dot < len(value)-1 {
		suffix := value[dot+1:]
		allDigits := true
		for _, r := range suffix {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			value = value[:dot]
		}
	}
	return strings.TrimSpace(value)
}

func (w *BlastWizard) confirmMainAutoIdentify(title string, needed bool, message string, allowSkip bool) (mainAutoIdentifyChoice, error) {
	if !needed {
		return mainAutoIdentifyNone, nil
	}
	actions := []tui.Action{{Value: string(mainAutoIdentifyClose), Label: tui.ButtonClose, Shortcut: tui.ShortcutBack}}
	if allowSkip {
		actions = append(actions, tui.Action{Value: string(mainAutoIdentifySkip), Label: tui.ButtonSkip, Shortcut: "S"})
	}
	result, err := tui.RunActionModalPage(tui.ActionModalPage{
		Path:         w.tuiPath("Main", "Auto identify"),
		Title:        title,
		Message:      message,
		Actions:      actions,
		ConfirmText:  tui.ButtonAuto,
		ConfirmValue: string(mainAutoIdentifyAuto),
	})
	if err != nil {
		return mainAutoIdentifyNone, err
	}
	switch mainAutoIdentifyChoice(result.Value) {
	case mainAutoIdentifyAuto, mainAutoIdentifySkip, mainAutoIdentifyClose:
		return mainAutoIdentifyChoice(result.Value), nil
	default:
		return mainAutoIdentifyClose, nil
	}
}

func mainKeywordRowsForWorkflow(rows []tui.MainKeywordRow) ([]tui.MainKeywordRow, []int) {
	out := make([]tui.MainKeywordRow, 0, len(rows))
	indexes := make([]int, 0, len(rows))
	for i, row := range rows {
		normalized := tui.MainKeywordRowsForExecution([]tui.MainKeywordRow{row})
		if len(normalized) == 0 {
			continue
		}
		out = append(out, normalized[0])
		indexes = append(indexes, i)
	}
	return out, indexes
}

func mainBlastRowsForWorkflow(rows []tui.MainBlastRow) ([]tui.MainBlastRow, []int) {
	out := make([]tui.MainBlastRow, 0, len(rows))
	indexes := make([]int, 0, len(rows))
	for i, row := range rows {
		normalized := tui.MainBlastRowsForExecution([]tui.MainBlastRow{row})
		if len(normalized) == 0 {
			continue
		}
		out = append(out, normalized[0])
		indexes = append(indexes, i)
	}
	return out, indexes
}

func mainKeywordMissingSymbolIndexes(rows []tui.MainKeywordRow, workflowIndexes []int) []int {
	out := make([]int, 0, len(workflowIndexes))
	for workflowIndex, rowIndex := range workflowIndexes {
		if rowIndex < 0 || rowIndex >= len(rows) {
			continue
		}
		row := rows[rowIndex]
		if strings.TrimSpace(row.SearchTerm) == "" {
			continue
		}
		if mainEditableCellIsBlank(row.SymbolName) {
			out = append(out, workflowIndex)
		}
	}
	return out
}

func mainKeywordMissingGeneLocusIndexes(rows []tui.MainKeywordRow, workflowIndexes []int) []int {
	out := make([]int, 0, len(workflowIndexes))
	for workflowIndex, rowIndex := range workflowIndexes {
		if rowIndex < 0 || rowIndex >= len(rows) {
			continue
		}
		row := rows[rowIndex]
		if strings.TrimSpace(row.SearchTerm) == "" {
			continue
		}
		if mainEditableCellIsBlank(row.GeneLocus) {
			out = append(out, workflowIndex)
		}
	}
	return out
}

func mainBlastMissingSymbolIndexes(rows []tui.MainBlastRow, workflowIndexes []int) []int {
	out := make([]int, 0, len(workflowIndexes))
	for workflowIndex, rowIndex := range workflowIndexes {
		if rowIndex < 0 || rowIndex >= len(rows) {
			continue
		}
		row := rows[rowIndex]
		if strings.TrimSpace(row.FASTA) == "" {
			continue
		}
		if mainEditableCellIsBlank(row.SymbolName) {
			out = append(out, workflowIndex)
		}
	}
	return out
}

func mainEditableCellIsBlank(value string) bool {
	return strings.TrimSpace(value) == ""
}

func mainApplyKeywordSymbolIdentifications(state *tui.MainInterfaceState, workflowIndexes []int, missingWorkflowIndexes []int, identifications []keywordLabelIdentification) {
	if state == nil {
		return
	}
	for _, workflowIndex := range missingWorkflowIndexes {
		if workflowIndex < 0 || workflowIndex >= len(workflowIndexes) || workflowIndex >= len(identifications) {
			continue
		}
		rowIndex := workflowIndexes[workflowIndex]
		if rowIndex < 0 || rowIndex >= len(state.Keyword.Rows) || !mainEditableCellIsBlank(state.Keyword.Rows[rowIndex].SymbolName) {
			continue
		}
		label := ""
		if len(identifications[workflowIndex].Aliases) > 0 {
			label = strings.TrimSpace(identifications[workflowIndex].Aliases[0])
		}
		if label == "" {
			label = "~~"
		}
		state.Keyword.Rows[rowIndex].SymbolName = label
		state.Keyword.Rows[rowIndex].Aliases = append([]string(nil), identifications[workflowIndex].Aliases...)
	}
}

func mainApplyKeywordGeneLoci(state *tui.MainInterfaceState, workflowIndexes []int, missingWorkflowIndexes []int, groups []model.KeywordSearchGroup) {
	if state == nil {
		return
	}
	for _, workflowIndex := range missingWorkflowIndexes {
		if workflowIndex < 0 || workflowIndex >= len(workflowIndexes) || workflowIndex >= len(groups) {
			continue
		}
		rowIndex := workflowIndexes[workflowIndex]
		if rowIndex < 0 || rowIndex >= len(state.Keyword.Rows) || !mainEditableCellIsBlank(state.Keyword.Rows[rowIndex].GeneLocus) {
			continue
		}
		locus := ""
		for _, row := range groups[workflowIndex].Rows {
			if locus = strings.TrimSpace(row.GeneLocus); locus != "" {
				break
			}
		}
		if locus == "" {
			locus = "~~"
		}
		state.Keyword.Rows[rowIndex].GeneLocus = locus
	}
}

func mainApplyBlastSymbolIdentifications(state *tui.MainInterfaceState, workflowIndexes []int, missingWorkflowIndexes []int, items []blastQueryItem) {
	if state == nil {
		return
	}
	for _, workflowIndex := range missingWorkflowIndexes {
		if workflowIndex < 0 || workflowIndex >= len(workflowIndexes) || workflowIndex >= len(items) {
			continue
		}
		rowIndex := workflowIndexes[workflowIndex]
		if rowIndex < 0 || rowIndex >= len(state.Blast.Rows) || !mainEditableCellIsBlank(state.Blast.Rows[rowIndex].SymbolName) {
			continue
		}
		label := strings.TrimSpace(items[workflowIndex].LabelName)
		if label == "" {
			label = strings.TrimSpace(preferredStoredQuerySourceAlias(items[workflowIndex].QuerySource))
		}
		if label == "" {
			label = "~~"
		}
		state.Blast.Rows[rowIndex].SymbolName = label
		aliases := storedQuerySourceAliases(items[workflowIndex].QuerySource)
		aliases = append(aliases, items[workflowIndex].LabelName)
		state.Blast.Rows[rowIndex].Aliases = uniqueStrings(aliases)
	}
}

func (w *BlastWizard) mainInterfaceSelectedSpecies(ctx context.Context, src source.DataSource, mode QueryMode, key string, label string, searchTypeID string) (model.SpeciesCandidate, error) {
	if _, ok := src.(*ncbi.Client); ok && mode == ModeKeyword {
		return ncbi.SyntheticSpeciesCandidate(searchTypeID), nil
	}
	candidates, err := w.speciesCandidatesForSource(ctx, src, nil)
	if err != nil {
		return model.SpeciesCandidate{}, err
	}
	key = strings.TrimSpace(key)
	if key != "" {
		for _, candidate := range candidates {
			if mainInterfaceSpeciesKey(candidate) == key {
				return candidate, nil
			}
		}
	}
	label = strings.TrimSpace(label)
	if label != "" {
		for _, candidate := range candidates {
			if strings.EqualFold(candidate.DisplayLabel(), label) || strings.EqualFold(mainInterfaceSpeciesShortLabel(candidate), label) {
				return candidate, nil
			}
		}
	}
	return model.SpeciesCandidate{}, fmt.Errorf("species is not set for %s %s", databaseDisplayName(src.Name()), mode)
}

func mainInterfaceSpeciesKey(candidate model.SpeciesCandidate) string {
	return strings.Join([]string{
		strings.TrimSpace(candidate.JBrowseName),
		strings.TrimSpace(candidate.GenomeLabel),
		fmt.Sprint(candidate.ProteomeID),
		strings.TrimSpace(candidate.ReleaseDate),
	}, "\x00")
}

func mainInterfaceSpeciesShortLabel(candidate model.SpeciesCandidate) string {
	return firstNonEmpty(
		strings.TrimSpace(candidate.GenomeLabel),
		strings.TrimSpace(candidate.SearchAlias),
		strings.TrimSpace(candidate.JBrowseName),
		candidate.DisplayLabel(),
	)
}

func mainInterfaceSpeciesDescription(candidate model.SpeciesCandidate) string {
	parts := []string{}
	if value := strings.TrimSpace(candidate.JBrowseName); value != "" {
		parts = append(parts, value)
	}
	if candidate.ProteomeID > 0 {
		parts = append(parts, fmt.Sprintf("target %d", candidate.ProteomeID))
	}
	if value := strings.TrimSpace(candidate.CommonName); value != "" {
		parts = append(parts, value)
	}
	if value := strings.TrimSpace(candidate.ReleaseDate); value != "" {
		parts = append(parts, value)
	}
	if candidate.IsOfficial {
		parts = append(parts, "official")
	}
	return strings.Join(parts, " | ")
}

func mainInterfaceSpeciesSearchText(candidate model.SpeciesCandidate) string {
	return strings.Join([]string{
		candidate.DisplayLabel(),
		candidate.SearchText(),
		strings.TrimSpace(candidate.LabelName),
		strings.TrimSpace(candidate.PhgoAliases),
		fmt.Sprint(candidate.ProteomeID),
	}, " ")
}

func (w *BlastWizard) mainInterfaceDataSource(database string) (source.DataSource, error) {
	switch strings.ToLower(strings.TrimSpace(database)) {
	case "phytozome":
		return phytozome.NewClient(w.httpClient), nil
	case "lemna":
		return lemna.NewClient(w.httpClient), nil
	case "tair":
		return tair.NewClient(w.httpClient), nil
	case "ncbi":
		return ncbi.NewClient(w.httpClient), nil
	default:
		return nil, fmt.Errorf("unsupported database %q", database)
	}
}

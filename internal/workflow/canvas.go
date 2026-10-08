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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/export"
	"github.com/KiriKirby/phytozome-go/internal/fastautil"
	"github.com/KiriKirby/phytozome-go/internal/megaphgo"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/notifyaudio"
	"github.com/KiriKirby/phytozome-go/internal/phylo"
	"github.com/KiriKirby/phytozome-go/internal/progressctx"
	"github.com/KiriKirby/phytozome-go/internal/prompt"
	"github.com/KiriKirby/phytozome-go/internal/sessionsnapshot"
	"github.com/KiriKirby/phytozome-go/internal/tui"
)

var (
	canvasSequenceReadyTrue  = true
	canvasSequenceReadyFalse = false
	canvasTreeViewerClient   = &http.Client{Timeout: 10 * time.Second}
)

type canvasLaunchState struct {
	Items         []model.CanvasItem
	CurrentItem   int
	NextNumericID int
	ImportedFrom  string
	SaveBaseName  string
}

func (w *BlastWizard) runCanvasMode(ctx context.Context, state canvasLaunchState) error {
	defer w.closeCanvasTreeViewer()
	notifyaudio.PlayCanvas()
	if state.NextNumericID <= 0 {
		state.NextNumericID = nextCanvasNumericID(state.Items)
	}
	var activeTreeSettings phylo.TreeSettings
	if w.canvasTreeRefreshRun == nil {
		w.canvasTreeRefreshRun = func(runCtx context.Context, runState canvasLaunchState, runSettings phylo.TreeSettings) error {
			return w.refreshCanvasTreeWithProgress(runCtx, runState, runSettings)
		}
		defer func() {
			w.canvasTreeRefreshRun = nil
		}()
	}
	for {
		selection, err := w.prompt.SelectCanvas(state.Items, state.CurrentItem, state.NextNumericID, "canvas", prompt.ErrBackToDatabaseSelection)
		if err != nil {
			return err
		}
		state.Items = cloneCanvasItems(selection.Items)
		state.CurrentItem = selection.CurrentItem
		state.NextNumericID = selection.NextNumericID
		activeTreeSettings = selection.TreeSettings
		w.installCanvasTreeMSAApplyHandler(&state, &activeTreeSettings)
		switch strings.TrimSpace(selection.Action) {
		case "", "view":
			if !selection.GenerateFile {
				continue
			}
			if len(state.Items) == 0 {
				if infoErr := w.showInfo("Canvas", "Canvas is empty. Add items before exporting.", prompt.ErrBackToDatabaseSelection); infoErr != nil {
					return infoErr
				}
				continue
			}
			saveSettings, saveErr := w.prompt.CanvasSaveSettings("", prompt.ErrBackToDatabaseSelection)
			if saveErr != nil {
				if errors.Is(saveErr, prompt.ErrBackToDatabaseSelection) {
					continue
				}
				return saveErr
			}
			saveBaseName := strings.TrimSpace(saveSettings.BaseName)
			if saveBaseName == "" {
				saveBaseName = canvasDefaultSaveName(state.Items)
			}
			if saveBaseName == "" {
				saveBaseName = "canvas"
			}
			defaultOutputDir, outErr := appfs.OutputDir()
			if outErr != nil {
				if infoErr := w.showInfo("Canvas export", outErr.Error(), prompt.ErrBackToDatabaseSelection); infoErr != nil {
					return infoErr
				}
				continue
			}
			outputDir, outErr := w.selectExportOutputDir(defaultOutputDir)
			if outErr != nil {
				if errors.Is(outErr, prompt.ErrBackToRowSelection) {
					continue
				}
				if infoErr := w.showInfo("Canvas export", outErr.Error(), prompt.ErrBackToDatabaseSelection); infoErr != nil {
					return infoErr
				}
				continue
			}
			settings := exportSettings{
				BaseName:            saveBaseName,
				OutputDir:           outputDir,
				WriteSession:        saveSettings.WriteSession,
				WriteText:           saveSettings.WriteText,
				WriteConvertedFasta: false,
				WriteAllRows:        saveSettings.WriteAllRows,
				FastaHeaderMode:     model.NormalizeFastaHeaderMode(saveSettings.FastaHeaderMode, saveSettings.UsePhgoHeader),
				UsePhgoHeader:       model.NormalizeFastaHeaderMode(saveSettings.FastaHeaderMode, saveSettings.UsePhgoHeader) == model.FastaHeaderModePhgo,
				TreeSettings:        selection.TreeSettings,
			}
			if err := w.exportCanvasSelections(ctx, state, settings); err != nil {
				if infoErr := w.showInfo("Canvas export", err.Error(), prompt.ErrBackToDatabaseSelection); infoErr != nil {
					return infoErr
				}
				continue
			}
			if infoErr := w.showInfo("Canvas", canvasExportCompleteMessage(settings), prompt.ErrBackToDatabaseSelection); infoErr != nil {
				return infoErr
			}
		case "delete_item":
			if state.CurrentItem >= 0 && state.CurrentItem < len(state.Items) {
				state.Items = append(state.Items[:state.CurrentItem], state.Items[state.CurrentItem+1:]...)
				if state.CurrentItem >= len(state.Items) {
					state.CurrentItem = len(state.Items) - 1
				}
				if state.CurrentItem < 0 {
					state.CurrentItem = 0
				}
			}
		case "rename_item":
			if len(state.Items) == 0 || state.CurrentItem < 0 || state.CurrentItem >= len(state.Items) {
				continue
			}
			name, nameErr := w.prompt.CanvasRenameInput(state.Items[state.CurrentItem].Title)
			if nameErr != nil {
				if nameErr == prompt.ErrBackToRowSelection {
					continue
				}
				return nameErr
			}
			state.Items[state.CurrentItem].Title = strings.TrimSpace(name)
			state.SaveBaseName = canvasDefaultSaveName(state.Items)
		case "add_item":
			input, sourcePath, inputErr := w.prompt.CanvasAddItemInput()
			if inputErr != nil {
				if inputErr == prompt.ErrBackToRowSelection {
					continue
				}
				return inputErr
			}
			items, itemErr := w.canvasItemsFromInput(ctx, strings.TrimSpace(input), state.NextNumericID, sourcePath)
			if itemErr != nil {
				if infoErr := w.showInfo("Canvas", itemErr.Error(), prompt.ErrBackToDatabaseSelection); infoErr != nil {
					return infoErr
				}
				continue
			}
			applyCanvasDisplayNameSourceToItems(items, selection.TreeSettings.DisplayNameSource)
			state.Items = append(state.Items, items...)
			state.CurrentItem = len(state.Items) - 1
			state.NextNumericID = nextCanvasNumericID(state.Items)
			state.SaveBaseName = canvasDefaultSaveName(state.Items)
		case "add_rows":
			if len(state.Items) == 0 || state.CurrentItem < 0 || state.CurrentItem >= len(state.Items) {
				continue
			}
			input, inputErr := w.prompt.CanvasAddRowsInput()
			if inputErr != nil {
				if inputErr == prompt.ErrBackToRowSelection {
					continue
				}
				return inputErr
			}
			rows, rowErr := w.canvasRowsFromFastaInput(strings.TrimSpace(input), false)
			if rowErr != nil {
				if infoErr := w.showInfo("Canvas", rowErr.Error(), prompt.ErrBackToDatabaseSelection); infoErr != nil {
					return infoErr
				}
				continue
			}
			item := &state.Items[state.CurrentItem]
			rows = assignCanvasRowNumbers(rows, nextCanvasRowNumber(item.Rows))
			item.Rows = append(item.Rows, rows...)
			item.Selected = normalizeCanvasSelection(item.Selected, len(item.Rows))
			item.MSAFlags = normalizeCanvasMSAFlags(item.MSAFlags, len(item.Rows))
			for i := len(item.Rows) - len(rows); i < len(item.Rows); i++ {
				if i >= 0 && i < len(item.Selected) {
					item.Selected[i] = true
				}
				if i >= 0 && i < len(item.MSAFlags) {
					item.MSAFlags[i] = false
				}
			}
			applyCanvasDisplayNameSource(item, selection.TreeSettings.DisplayNameSource)
			updateCanvasItemSubtitle(item)
		case "delete_rows":
			if len(state.Items) == 0 || state.CurrentItem < 0 || state.CurrentItem >= len(state.Items) {
				continue
			}
			item := &state.Items[state.CurrentItem]
			selected := normalizeCanvasSelection(item.Selected, len(item.Rows))
			rowIndex := selection.ActionRow
			if rowIndex < 0 || rowIndex >= len(item.Rows) {
				continue
			}
			nextRows := make([]model.CanvasRow, 0, len(item.Rows))
			nextSelected := make([]bool, 0, len(item.Rows))
			msaFlags := normalizeCanvasMSAFlags(item.MSAFlags, len(item.Rows))
			nextMSAFlags := make([]bool, 0, len(item.Rows))
			for i, row := range item.Rows {
				if i == rowIndex {
					continue
				}
				nextRows = append(nextRows, row)
				if i < len(selected) {
					nextSelected = append(nextSelected, selected[i])
				} else {
					nextSelected = append(nextSelected, false)
				}
				if i < len(msaFlags) {
					nextMSAFlags = append(nextMSAFlags, msaFlags[i])
				} else {
					nextMSAFlags = append(nextMSAFlags, false)
				}
			}
			item.Rows = nextRows
			item.Selected = nextSelected
			item.MSAFlags = nextMSAFlags
			updateCanvasItemSubtitle(item)
		case "open_tree_tools":
			if err := w.openCanvasTreeToolsWithProgress(ctx); err != nil {
				w.collapseCanvasTreePanel()
				if infoErr := w.showInfo("System tree", err.Error(), prompt.ErrBackToDatabaseSelection); infoErr != nil {
					return infoErr
				}
				continue
			}
		case "open_tree_viewer":
			if err := w.openCanvasTreeViewerWithProgress(ctx, &state, selection.TreeSettings); err != nil {
				w.collapseCanvasTreePanel()
				if infoErr := w.showInfo("System tree", err.Error(), prompt.ErrBackToDatabaseSelection); infoErr != nil {
					return infoErr
				}
				continue
			}
		case "refresh_tree":
			if err := w.ensureCanvasTreeRuntimeWithProgress(ctx, "Preparing system tree refresh", "Checking the bundled MEGA runtime before refreshing the system tree..."); err != nil {
				if infoErr := w.showInfo("System tree", err.Error(), prompt.ErrBackToDatabaseSelection); infoErr != nil {
					return infoErr
				}
				continue
			}
			if err := w.refreshCanvasTreeInteractive(ctx, &state, selection.TreeSettings); err != nil {
				if megaphgo.IsMissingToolsError(err) {
					if runtimeErr := w.ensureCanvasTreeRuntimeWithProgress(ctx, "Retrying MEGA runtime check", "Rechecking the bundled MEGA runtime before retrying the system tree refresh..."); runtimeErr != nil {
						err = runtimeErr
					} else if retryErr := w.refreshCanvasTreeInteractive(ctx, &state, selection.TreeSettings); retryErr == nil {
						continue
					} else {
						err = retryErr
					}
				}
				if infoErr := w.showInfo("System tree", err.Error(), prompt.ErrBackToDatabaseSelection); infoErr != nil {
					return infoErr
				}
				continue
			}
		default:
			continue
		}
	}
}

func (w *BlastWizard) refreshCanvasTreeInteractive(ctx context.Context, state *canvasLaunchState, settings phylo.TreeSettings) error {
	if state == nil {
		return fmt.Errorf("canvas state is unavailable")
	}
	settings = phylo.NormalizeTreeSettings(settings)
	run := w.canvasTreeRefreshRun
	if run == nil {
		run = func(runCtx context.Context, runState canvasLaunchState, runSettings phylo.TreeSettings) error {
			return w.refreshCanvasTreeWithProgress(runCtx, runState, runSettings)
		}
	}
	return w.refreshCanvasTreeInteractiveWithRun(ctx, state, settings, run)
}

func (w *BlastWizard) refreshCanvasTreeInteractiveWithRun(ctx context.Context, state *canvasLaunchState, settings phylo.TreeSettings, run func(context.Context, canvasLaunchState, phylo.TreeSettings) error) error {
	if state == nil {
		return fmt.Errorf("canvas state is unavailable")
	}
	if run == nil {
		return fmt.Errorf("canvas tree refresh runner is unavailable")
	}
	autoSkip := false
	for {
		runState := *state
		err := run(ctx, runState, settings)
		if err == nil {
			return nil
		}
		var skippedErr *canvasTreeSkippedRowsError
		if !errors.As(err, &skippedErr) {
			return err
		}
		if autoSkip {
			selected := skippedErr.SelectedRows
			if len(selected) == 0 {
				selected = w.selectedCanvasRowsInCurrentOrder(state.Items, false)
			}
			if changed := unselectCanvasTreeSkippedRows(state.Items, resolveCanvasTreeSkippedRows(selected, skippedErr.SkippedRows)); changed == 0 {
				return err
			}
			continue
		}
		action, actionErr := w.canvasTreeRecoveryAction(canvasTreeSkipSummary(skippedErr.SkippedRows), prompt.ErrBackToRowSelection, true)
		if actionErr != nil {
			return actionErr
		}
		decision, navErr := interpretRecoveryAction(action, prompt.ErrBackToRowSelection, true)
		if navErr != nil {
			return navErr
		}
		switch decision {
		case recoveryRetry:
			continue
		case recoverySkip:
			selected := skippedErr.SelectedRows
			if len(selected) == 0 {
				selected = w.selectedCanvasRowsInCurrentOrder(state.Items, false)
			}
			if changed := unselectCanvasTreeSkippedRows(state.Items, resolveCanvasTreeSkippedRows(selected, skippedErr.SkippedRows)); changed == 0 {
				return err
			}
			autoSkip = true
			continue
		default:
			return err
		}
	}
}

func (w *BlastWizard) collapseCanvasTreePanel() {
	state := w.prompt.SnapshotCanvasTreePanelState(canvasStateKey("canvas"))
	state.Expanded = false
	state.Focused = false
	w.prompt.RestoreCanvasTreePanelState(canvasStateKey("canvas"), state)
}

func (w *BlastWizard) ensureCanvasTreeRuntimeInteractive(ctx context.Context) error {
	if w.ensureCanvasTreeRuntime != nil {
		return w.ensureCanvasTreeRuntime(ctx)
	}
	return megaphgo.EnsureRuntimeAvailable()
}

func (w *BlastWizard) ensureCanvasTreeRuntimeWithProgress(ctx context.Context, title string, description string) error {
	if w.suppressTaskModals {
		return w.ensureCanvasTreeRuntimeInteractive(ctx)
	}
	_, err := tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Startup", "Explore", "Canvas", "Tree tools"),
		Title:       title,
		Description: description,
		Initial:     "Checking bundled MEGA runtime...",
		Total:       1,
		CancelError: prompt.ErrBackToRowSelection,
	}, func(taskCtx context.Context, update func(int, string)) (struct{}, error) {
		progress := safeProgress(update)
		progress(0, "Checking bundled MEGA runtime...")
		if err := w.ensureCanvasTreeRuntimeInteractive(taskCtx); err != nil {
			return struct{}{}, err
		}
		progress(1, "Bundled MEGA runtime is ready.")
		return struct{}{}, nil
	})
	return err
}

func (w *BlastWizard) openCanvasTreeToolsWithProgress(ctx context.Context) error {
	return w.ensureCanvasTreeRuntimeWithProgress(ctx, "Opening system tree tools", "Checking the bundled MEGA runtime before opening the system tree settings panel.")
}

func (w *BlastWizard) openCanvasTreeViewerWithProgress(ctx context.Context, state *canvasLaunchState, settings phylo.TreeSettings) error {
	if w.suppressTaskModals {
		if err := w.ensureCanvasTreeRuntimeInteractive(ctx); err != nil {
			return err
		}
		return w.openCanvasTreeViewer(ctx, state, settings)
	}
	_, err := tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Startup", "Explore", "Canvas", "Tree viewer"),
		Title:       "Opening system tree preview",
		Description: "Checking the bundled MEGA runtime, preparing the local Reactree viewer, and opening the preview.",
		Initial:     "Checking bundled MEGA runtime...",
		Total:       2,
		CancelError: prompt.ErrBackToRowSelection,
	}, func(taskCtx context.Context, update func(int, string)) (struct{}, error) {
		progress := safeProgress(update)
		progress(0, "Checking bundled MEGA runtime...")
		if err := w.ensureCanvasTreeRuntimeInteractive(taskCtx); err != nil {
			return struct{}{}, err
		}
		progress(1, "Preparing local Reactree viewer...")
		if err := w.openCanvasTreeViewer(taskCtx, state, settings); err != nil {
			return struct{}{}, err
		}
		progress(2, "System tree preview opened.")
		return struct{}{}, nil
	})
	return err
}

func (w *BlastWizard) refreshCanvasTree(ctx context.Context, state canvasLaunchState, settings phylo.TreeSettings) error {
	selectedRows := w.selectedCanvasRowsInCurrentOrder(state.Items, false)
	if len(selectedRows) == 0 {
		return fmt.Errorf("no selected canvas rows are available for tree analysis")
	}
	progressctx.Report(ctx, 1, fmt.Sprintf("Preparing %d selected Canvas rows...", len(selectedRows)))
	result, err := w.buildCanvasTreeArtifacts(ctx, state, selectedRows, settings)
	if err != nil {
		return err
	}
	if result.Reused {
		progressctx.Report(ctx, 5, "Reused existing alignment and tree artifacts; updating Reactree metadata...")
	} else {
		progressctx.Report(ctx, 5, "Preparing Reactree viewer payload...")
	}
	if err := w.updateCanvasTreeViewer(ctx, result.Plan); err != nil {
		return err
	}
	progressctx.Report(ctx, 6, "Reactree viewer updated.")
	return nil
}

type canvasTreeSkippedRow struct {
	ItemTitle string
	ItemIndex int
	RowIndex  int
	TaxonID   string
	Reason    string
}

type canvasTreeSkippedRowsError struct {
	SkippedRows  []canvasTreeSkippedRow
	SelectedRows []canvasSelectedRow
}

func (e *canvasTreeSkippedRowsError) Error() string {
	if e == nil {
		return ""
	}
	return canvasTreeSkipSummary(e.SkippedRows)
}

func (w *BlastWizard) canvasTreeRecoveryAction(description string, backTarget error, allowSkip bool) (string, error) {
	if handler := w.canvasTreeRecover; handler != nil {
		return handler(description, backTarget, allowSkip)
	}
	if allowSkip {
		return w.prompt.FetchErrorAction(description, backTarget)
	}
	return w.prompt.WorkflowErrorAction(description, backTarget)
}

func (w *BlastWizard) refreshCanvasTreeWithProgress(ctx context.Context, state canvasLaunchState, settings phylo.TreeSettings) error {
	w.setCanvasTreeViewerRefreshing(true, "Refreshing tree and MSA...")
	w.prompt.UpdateOpenCanvasRefreshStatus(true, "Refreshing tree and MSA...")
	defer w.setCanvasTreeViewerRefreshing(false, "")
	defer w.prompt.UpdateOpenCanvasRefreshStatus(false, "")
	if w.suppressTaskModals {
		err := w.refreshCanvasTree(ctx, state, settings)
		if err == nil {
			notifyaudio.PlayDone()
		}
		return err
	}
	_, err := tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        w.tuiPath("Startup", "Explore", "Canvas", "Tree tools"),
		Title:       "Refreshing system tree",
		Description: "Preparing selected Canvas sequences, running the PHgo MEGA runtime when needed, and updating the Reactree viewer.",
		Initial:     "Preparing tree refresh...",
		Total:       6,
		CancelError: prompt.ErrBackToRowSelection,
	}, func(taskCtx context.Context, update func(int, string)) (struct{}, error) {
		progress := safeProgress(update)
		progress(0, "Preparing tree refresh...")
		taskCtx = progressctx.WithProgress(taskCtx, progress)
		if err := w.refreshCanvasTree(taskCtx, state, settings); err != nil {
			return struct{}{}, err
		}
		progress(6, "System tree refresh completed.")
		notifyaudio.PlayDone()
		return struct{}{}, nil
	})
	return err
}

func (w *BlastWizard) refreshCanvasTreeForMSAApply(ctx context.Context, state canvasLaunchState, settings phylo.TreeSettings) error {
	w.setCanvasTreeViewerRefreshing(true, "Refreshing tree and MSA...")
	w.prompt.UpdateOpenCanvasRefreshStatus(true, "Refreshing tree and MSA...")
	defer w.setCanvasTreeViewerRefreshing(false, "")
	defer w.prompt.UpdateOpenCanvasRefreshStatus(false, "")
	progress := func(current int, message string) {
		message = strings.TrimSpace(message)
		if message == "" {
			return
		}
		w.setCanvasTreeViewerRefreshing(true, message)
		w.prompt.UpdateOpenCanvasRefreshStatus(true, message)
	}
	err := w.refreshCanvasTree(progressctx.WithProgress(ctx, progress), state, settings)
	if err == nil {
		notifyaudio.PlayDone()
	}
	return err
}

func (w *BlastWizard) setCanvasTreeViewerRefreshing(refreshing bool, message string) {
	w.canvasTreeViewerMu.Lock()
	server := w.canvasTreeViewer
	w.canvasTreeViewerMu.Unlock()
	if server == nil {
		return
	}
	if strings.TrimSpace(message) == "" && refreshing {
		message = "Refreshing tree and MSA..."
	}
	server.SetSessionStatus(w.canvasTreeSessionID(), phylo.ViewerSessionStatus{
		Refreshing: refreshing,
		Message:    strings.TrimSpace(message),
	})
}

func (w *BlastWizard) installCanvasTreeMSAApplyHandler(state *canvasLaunchState, settings *phylo.TreeSettings) {
	w.canvasTreeViewerMu.Lock()
	server := w.canvasTreeViewer
	w.canvasTreeViewerMu.Unlock()
	if server == nil {
		return
	}
	w.installCanvasTreeMSAApplyHandlerOnServer(server, state, settings)
}

func (w *BlastWizard) installCanvasTreeMSAApplyHandlerOnServer(server *phylo.ViewerServer, state *canvasLaunchState, settings *phylo.TreeSettings) {
	server.SetMSAApplyHandler(func(applyCtx context.Context, sessionID string, req phylo.MSAApplyRequest) (phylo.MSAApplyResponse, error) {
		if state == nil {
			return phylo.MSAApplyResponse{}, fmt.Errorf("canvas state is unavailable")
		}
		w.canvasTreeMSAApplyMu.Lock()
		defer w.canvasTreeMSAApplyMu.Unlock()
		treeSettings := phylo.DefaultTreeSettings()
		if settings != nil {
			treeSettings = *settings
		}
		changed := w.applyMSASelectionToCanvasState(state, req)
		if changed {
			selected, msaFlags := canvasSelectionMasks(state.Items)
			w.prompt.UpdateOpenCanvasSelection(selected, msaFlags)
		}
		if !changed {
			return phylo.MSAApplyResponse{Accepted: true, Refreshing: false, Message: "MSA selection is already applied."}, nil
		}
		run := w.canvasTreeMSAApplyRun
		if run == nil {
			run = func(runCtx context.Context, runState canvasLaunchState, runSettings phylo.TreeSettings) error {
				return w.refreshCanvasTreeForMSAApply(runCtx, runState, runSettings)
			}
		}
		if err := run(applyCtx, *state, treeSettings); err != nil {
			return phylo.MSAApplyResponse{}, err
		}
		if !w.canvasTreeLastPayload.UpdatedAt.IsZero() || strings.TrimSpace(w.canvasTreeLastPayload.AlignedFASTA) != "" {
			w.canvasTreeLastMSAPayload = w.canvasTreeLastPayload
		}
		w.canvasTreeMSAState = canvasMSAStateFromItems(state.Items, w.canvasTreeLastMSAPayload)
		server.SetMSAState(w.canvasTreeSessionID(), w.canvasTreeMSAState)
		return phylo.MSAApplyResponse{Accepted: true, Refreshing: false, Message: "MSA selection applied."}, nil
	})
}

func (w *BlastWizard) applyMSASelectionToCanvasState(state *canvasLaunchState, req phylo.MSAApplyRequest) bool {
	if state == nil {
		return false
	}
	if len(w.canvasTreeMSARowMap) == 0 {
		w.updateCanvasTreeMSARowMap(w.canvasTreeLastPlan)
	}
	rowByTaxonID := make(map[string]phylo.InputRecord, len(w.canvasTreeMSARowMap))
	for taxonID, record := range w.canvasTreeMSARowMap {
		rowByTaxonID[taxonID] = record
	}
	for _, record := range w.canvasTreeLastPlan.Records {
		taxonID := strings.TrimSpace(record.TaxonID)
		if taxonID != "" {
			rowByTaxonID[taxonID] = record
		}
	}
	changed := false
	for _, row := range req.Rows {
		taxonID := strings.TrimSpace(row.TaxonID)
		record, ok := rowByTaxonID[taxonID]
		if !ok {
			continue
		}
		wantSelected := strings.EqualFold(strings.TrimSpace(row.State), "green")
		for itemIndex := range state.Items {
			if strings.TrimSpace(state.Items[itemIndex].Title) != strings.TrimSpace(record.CanvasItem) {
				continue
			}
			mask := normalizeCanvasSelection(state.Items[itemIndex].Selected, len(state.Items[itemIndex].Rows))
			rowIndex := record.CanvasRow
			if rowIndex < 0 || rowIndex >= len(mask) {
				continue
			}
			msaFlags := normalizeCanvasMSAFlags(state.Items[itemIndex].MSAFlags, len(state.Items[itemIndex].Rows))
			wantMSAFlag := strings.EqualFold(strings.TrimSpace(row.State), "yellow")
			if mask[rowIndex] != wantSelected || msaFlags[rowIndex] != wantMSAFlag {
				mask[rowIndex] = wantSelected
				msaFlags[rowIndex] = wantMSAFlag
				state.Items[itemIndex].Selected = mask
				state.Items[itemIndex].MSAFlags = msaFlags
				updateCanvasItemSubtitle(&state.Items[itemIndex])
				changed = true
			}
			canvasRow := &state.Items[itemIndex].Rows[rowIndex]
			if name := strings.TrimSpace(firstNonEmpty(row.DisplayName, row.Name)); name != "" && name != strings.TrimSpace(canvasRow.DisplayName) {
				canvasRow.DisplayName = name
				canvasRow.DisplayNameLocked = true
				changed = true
			}
			if sequence := normalizeMSAEditedSequence(row.Sequence); sequence != "" && sequence != strings.TrimSpace(canvasRowStoredSequence(*canvasRow)) {
				applyMSAEditedSequenceToCanvasRow(canvasRow, sequence, record.SequenceKind)
				changed = true
			}
		}
	}
	return changed
}

func normalizeMSAEditedSequence(sequence string) string {
	sequence = strings.TrimSpace(sequence)
	if sequence == "" {
		return ""
	}
	return strings.ToUpper(sequence)
}

func applyMSAEditedSequenceToCanvasRow(row *model.CanvasRow, sequence string, kind phylo.SequenceKind) {
	if row == nil {
		return
	}
	sequence = normalizeMSAEditedSequence(sequence)
	if sequence == "" {
		return
	}
	if row.SequenceData == nil {
		row.SequenceData = &model.ProteinSequenceData{}
	}
	row.SequenceData.Sequence = sequence
	ready := true
	row.SequenceReady = &ready
	switch row.Kind {
	case model.CanvasKindFasta:
		if row.FASTA == nil {
			row.FASTA = &model.QuerySequenceSource{}
		}
		row.FASTA.Sequence = sequence
		switch kind {
		case phylo.SequenceNucleotide:
			row.FASTA.NucleotideSequence = sequence
			row.FASTA.SequenceKind = model.SequenceDNA
		case phylo.SequenceProtein:
			row.FASTA.ProteinSequence = sequence
			row.FASTA.SequenceKind = model.SequenceProtein
		}
	}
}

func canvasMSAStateFromItems(items []model.CanvasItem, payload phylo.ViewerPayload) phylo.MSAState {
	stateByCanvasRow := map[string]string{}
	for itemIndex := range items {
		itemTitle := strings.TrimSpace(items[itemIndex].Title)
		selected := normalizeCanvasSelection(items[itemIndex].Selected, len(items[itemIndex].Rows))
		msaFlags := normalizeCanvasMSAFlags(items[itemIndex].MSAFlags, len(items[itemIndex].Rows))
		for rowIndex := range items[itemIndex].Rows {
			state := "red"
			if rowIndex < len(selected) && selected[rowIndex] {
				state = "green"
			} else if rowIndex < len(msaFlags) && msaFlags[rowIndex] {
				state = "yellow"
			}
			stateByCanvasRow[itemTitle+"\x00"+strconv.Itoa(rowIndex)] = state
		}
	}
	rows := make([]phylo.MSASelectionRow, 0, len(payload.Metadata.Records))
	for index, record := range payload.Metadata.Records {
		taxonID := strings.TrimSpace(record.TaxonID)
		if taxonID == "" {
			continue
		}
		state := stateByCanvasRow[strings.TrimSpace(record.CanvasItem)+"\x00"+strconv.Itoa(record.CanvasRow)]
		if state == "" {
			state = "green"
		}
		rows = append(rows, phylo.MSASelectionRow{
			TaxonID:     taxonID,
			DisplayName: strings.TrimSpace(record.DisplayName),
			Index:       index,
			State:       state,
		})
	}
	return phylo.MSAState{
		SchemaVersion: 2,
		UpdatedAt:     time.Now(),
		Rows:          rows,
	}
}

func canvasSelectionMasks(items []model.CanvasItem) ([][]bool, [][]bool) {
	selected := make([][]bool, len(items))
	msaFlags := make([][]bool, len(items))
	for i := range items {
		selected[i] = normalizeCanvasSelection(items[i].Selected, len(items[i].Rows))
		msaFlags[i] = normalizeCanvasMSAFlags(items[i].MSAFlags, len(items[i].Rows))
	}
	return selected, msaFlags
}

func (w *BlastWizard) buildCanvasTreeArtifacts(ctx context.Context, state canvasLaunchState, selectedRows []canvasSelectedRow, settings phylo.TreeSettings) (phylo.RunResult, error) {
	return w.buildCanvasTreeArtifactsWithRuntime(ctx, state, selectedRows, settings, nil)
}

func (w *BlastWizard) buildCanvasTreeArtifactsWithRuntime(ctx context.Context, state canvasLaunchState, selectedRows []canvasSelectedRow, settings phylo.TreeSettings, runtime phylo.Runtime) (phylo.RunResult, error) {
	now := time.Now()
	progressctx.Report(ctx, 2, fmt.Sprintf("Loading selected Canvas sequence payloads for MEGA %s-mode tree analysis...", canvasTreeTargetLabel(settings)))
	rowSources, err := w.canvasTreeRowSourcesWithSkippedForSettings(ctx, state, selectedRows, settings)
	if err != nil {
		return phylo.RunResult{}, err
	}
	progressctx.Report(ctx, 3, fmt.Sprintf("Writing tree input FASTA and runtime request for MEGA %s mode...", canvasTreeTargetLabel(settings)))
	records, meta, err := phylo.BuildInput(rowSources, settings.DisplayNameSource, w.canvasTreeSessionID(), now)
	if err != nil {
		return phylo.RunResult{}, err
	}
	applyCanvasCoordinateMetadata(records, &meta)
	runID := canvasTreeRunID(now)
	artifactDir, err := canvasTreeArtifactDir(w.canvasTreeSessionID(), runID)
	if err != nil {
		return phylo.RunResult{}, fmt.Errorf("prepare Canvas tree cache: %w", err)
	}
	plan, err := phylo.BuildRunPlan(w.canvasTreeSessionID(), runID, artifactDir, settings, canvasTreeTargetSequenceKind(settings), records, meta, "", "", now)
	if err != nil {
		return phylo.RunResult{}, err
	}
	if w.canvasTreeForceCompute {
		progressctx.Report(ctx, 4, "Snapshot-restored tree state requires a full PHgo recompute before reuse is allowed...")
	} else if reused, ok, err := w.reuseLastCanvasTreePlan(plan, records, meta, now); err != nil {
		return phylo.RunResult{}, err
	} else if ok {
		progressctx.Report(ctx, 4, "Only tree labels changed; reusing runtime artifacts and rerendering Reactree...")
		w.canvasTreeLastPlan = reused.Plan
		return reused, nil
	}
	progressctx.Report(ctx, 4, fmt.Sprintf("Running mega-phgo-runtime for MEGA %s alignment and tree inference...", canvasTreeTargetLabel(settings)))
	result, err := phylo.RunPlanWithRuntime(ctx, plan, phylo.RuntimeOptions{Runtime: runtime})
	if err != nil {
		if runtimeSkipped := canvasTreeSkippedRowsFromRuntime(result.SkippedRecords); len(runtimeSkipped) > 0 {
			return result, &canvasTreeSkippedRowsError{SkippedRows: runtimeSkipped, SelectedRows: cloneCanvasSelectedRows(selectedRows)}
		}
		if strings.TrimSpace(result.ArtifactDir) != "" {
			return result, fmt.Errorf("%w\n\nmega-phgo-runtime artifacts were kept at:\n%s", err, result.ArtifactDir)
		}
		return result, err
	}
	if runtimeSkipped := canvasTreeSkippedRowsFromRuntime(result.SkippedRecords); len(runtimeSkipped) > 0 {
		return result, &canvasTreeSkippedRowsError{SkippedRows: runtimeSkipped, SelectedRows: cloneCanvasSelectedRows(selectedRows)}
	}
	w.canvasTreeLastPlan = result.Plan
	w.canvasTreeForceCompute = false
	return result, nil
}

func (w *BlastWizard) reuseLastCanvasTreePlan(plan phylo.RunPlan, records []phylo.InputRecord, meta phylo.Metadata, now time.Time) (phylo.RunResult, bool, error) {
	if w.canvasTreeForceCompute {
		return phylo.RunResult{}, false, nil
	}
	last := w.canvasTreeLastPlan
	if strings.TrimSpace(last.Newick) == "" || strings.TrimSpace(last.AlignedFASTA) == "" {
		return phylo.RunResult{}, false, nil
	}
	candidate, err := phylo.BuildRunPlan(plan.SessionID, plan.RunID, plan.BaseDir, plan.Settings, plan.Kind, records, meta, last.AlignedFASTA, last.Newick, now)
	if err != nil {
		return phylo.RunResult{}, false, err
	}
	if candidate.Fingerprints.Alignment != last.Fingerprints.Alignment || candidate.Fingerprints.Tree != last.Fingerprints.Tree {
		return phylo.RunResult{}, false, nil
	}
	candidate = phylo.AttachRuntimeAudit(candidate, "mega-phgo-runtime/reused", now)
	if err := candidate.ToArtifactSet().Write(); err != nil {
		return phylo.RunResult{}, false, err
	}
	return phylo.RunResult{
		Plan:           candidate,
		ArtifactDir:    candidate.BaseDir,
		SelectedNewick: filepath.Join(candidate.BaseDir, "tree.nwk"),
		Reused:         true,
	}, true, nil
}

func (w *BlastWizard) ensureCanvasTreeViewer(ctx context.Context) (*phylo.ViewerServer, string, error) {
	w.canvasTreeViewerMu.Lock()
	defer w.canvasTreeViewerMu.Unlock()
	if w.canvasTreeViewer != nil && strings.TrimSpace(w.canvasTreeViewer.URL()) != "" {
		return w.canvasTreeViewer, w.canvasTreeViewer.URL() + "/sessions/" + w.canvasTreeSessionID() + "/tree", nil
	}
	viewerCtx, cancel := context.WithCancel(context.Background())
	server := phylo.NewViewerServer("127.0.0.1:0")
	if err := server.Start(viewerCtx); err != nil {
		cancel()
		return nil, "", err
	}
	w.canvasTreeViewer = server
	w.canvasTreeViewerCancel = cancel
	return server, server.URL() + "/sessions/" + w.canvasTreeSessionID() + "/tree", nil
}

func (w *BlastWizard) closeCanvasTreeViewer() {
	w.canvasTreeViewerMu.Lock()
	cancel := w.canvasTreeViewerCancel
	w.canvasTreeViewer = nil
	w.canvasTreeViewerCancel = nil
	w.canvasTreeViewerMu.Unlock()
	if cancel != nil {
		cancel()
	}
	w.canvasTreeLastPayload = phylo.ViewerPayload{}
	w.canvasTreeLastMSAPayload = phylo.ViewerPayload{}
	w.canvasTreeMSAState = phylo.MSAState{}
	w.canvasTreeViewerState = nil
	w.canvasTreeLastPlan = phylo.RunPlan{}
	w.canvasTreeMSARowMap = nil
	w.canvasTreeForceCompute = false
}

func (w *BlastWizard) canvasTreePreviewAvailable() bool {
	payload := w.canvasTreeLastPayload
	return strings.TrimSpace(payload.Newick) != "" && strings.TrimSpace(payload.AlignedFASTA) != ""
}

func (w *BlastWizard) openCanvasTreeViewer(ctx context.Context, state *canvasLaunchState, settings phylo.TreeSettings) error {
	server, url, err := w.ensureCanvasTreeViewer(ctx)
	if err != nil {
		return err
	}
	if state != nil {
		w.installCanvasTreeMSAApplyHandlerOnServer(server, state, &settings)
	}
	if !w.canvasTreeLastPayload.UpdatedAt.IsZero() || strings.TrimSpace(w.canvasTreeLastPayload.Newick) != "" {
		if err := w.putCanvasTreeViewerPayload(ctx, server, w.canvasTreeLastPayload); err != nil {
			return err
		}
	}
	if len(w.canvasTreeMSAState.Rows) > 0 || len(w.canvasTreeMSAState.Groups) > 0 || len(w.canvasTreeMSAState.Annotations) > 0 || len(w.canvasTreeMSAState.Settings) > 0 || len(w.canvasTreeMSAState.Markers) > 0 {
		server.SetMSAState(w.canvasTreeSessionID(), w.canvasTreeMSAState)
	}
	if len(w.canvasTreeViewerState) > 0 {
		if err := putViewerState(ctx, server, w.canvasTreeSessionID(), w.canvasTreeViewerState); err != nil {
			return err
		}
	}
	if err := openBrowserURLFunc(ctx, url); err != nil {
		return err
	}
	if err := openBrowserURLFunc(ctx, server.URL()+"/sessions/"+w.canvasTreeSessionID()+"/msa"); err != nil {
		return err
	}
	w.openCanvasTreeJalviewBestEffort()
	return nil
}

func (w *BlastWizard) updateCanvasTreeViewer(ctx context.Context, plan phylo.RunPlan) error {
	server, _, err := w.ensureCanvasTreeViewer(ctx)
	if err != nil {
		return err
	}
	payload := plan.ToArtifactSet().Payload
	if err := w.putCanvasTreeViewerPayload(ctx, server, payload); err != nil {
		return err
	}
	w.canvasTreeLastPayload = payload
	w.canvasTreeLastMSAPayload = payload
	w.canvasTreeMSAState = server.GetMSAState(w.canvasTreeSessionID())
	w.canvasTreeLastPlan = plan
	w.updateCanvasTreeMSARowMap(plan)
	return nil
}

func (w *BlastWizard) updateCanvasTreeMSARowMap(plan phylo.RunPlan) {
	if len(plan.Records) == 0 {
		return
	}
	if w.canvasTreeMSARowMap == nil {
		w.canvasTreeMSARowMap = map[string]phylo.InputRecord{}
	}
	for _, record := range plan.Records {
		taxonID := strings.TrimSpace(record.TaxonID)
		if taxonID != "" {
			w.canvasTreeMSARowMap[taxonID] = record
		}
	}
}

func (w *BlastWizard) putCanvasTreeViewerPayload(ctx context.Context, server *phylo.ViewerServer, payload phylo.ViewerPayload) error {
	payload.Title = w.canvasTreeViewerTitle()
	return putViewerPayload(ctx, server, w.canvasTreeSessionID(), payload)
}

func (w *BlastWizard) canvasTreeViewerTitle() string {
	if id := strings.TrimSpace(w.instanceID); id != "" {
		return id
	}
	if id := strings.TrimSpace(w.instanceRunID); id != "" {
		return id
	}
	return "canvas"
}

type browserCommandStarter func(name string, args ...string) error

var openBrowserURLFunc = openBrowserURL

func openBrowserURL(ctx context.Context, rawURL string) error {
	return openBrowserURLWithStarter(ctx, rawURL, startBrowserCommand)
}

func openBrowserURLWithStarter(ctx context.Context, rawURL string, starter browserCommandStarter) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("tree viewer URL is empty")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if starter == nil {
		starter = startBrowserCommand
	}
	switch runtime.GOOS {
	case "windows":
		return starter("rundll32", "url.dll,FileProtocolHandler", rawURL)
	case "darwin":
		return starter("open", rawURL)
	default:
		return starter("xdg-open", rawURL)
	}
}

func startBrowserCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Start()
}

func (w *BlastWizard) openCanvasTreeJalviewBestEffort() {
	plan := w.canvasTreeLastPlan
	alignedPath := strings.TrimSpace(filepath.Join(plan.BaseDir, "aligned.fasta"))
	if strings.TrimSpace(plan.BaseDir) == "" || strings.TrimSpace(plan.AlignedFASTA) == "" || alignedPath == "" {
		return
	}
	if _, err := os.Stat(alignedPath); err != nil {
		return
	}
	_ = startJalviewCommand(alignedPath)
}

func startJalviewCommand(alignedPath string) error {
	alignedPath = strings.TrimSpace(alignedPath)
	if alignedPath == "" {
		return fmt.Errorf("jalview alignment path is empty")
	}
	candidates := [][]string{
		{"jalview", "-open", alignedPath},
		{"Jalview", "-open", alignedPath},
		{"jalview.exe", "-open", alignedPath},
		{"Jalview.exe", "-open", alignedPath},
		{"jalview", alignedPath},
		{"Jalview", alignedPath},
		{"jalview.exe", alignedPath},
		{"Jalview.exe", alignedPath},
	}
	for _, candidate := range candidates {
		if len(candidate) == 0 {
			continue
		}
		cmd := exec.Command(candidate[0], candidate[1:]...)
		if err := cmd.Start(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("jalview executable was not found")
}

func (w *BlastWizard) canvasTreeSessionID() string {
	if id := strings.TrimSpace(w.instanceID); id != "" {
		return id
	}
	if id := strings.TrimSpace(w.instanceRunID); id != "" {
		return id
	}
	return "canvas"
}

func canvasTreeRunID(now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	return now.Format("20060102_150405_000")
}

func sanitizeCanvasTreePathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "canvas"
	}
	var b strings.Builder
	for _, ch := range value {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9':
			b.WriteRune(ch)
		case ch == '-', ch == '_', ch == '.':
			b.WriteRune(ch)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._")
	if out == "" {
		return "canvas"
	}
	return out
}

func canvasTreeArtifactDir(sessionID string, runID string) (string, error) {
	return appfs.CacheDir("tree", sanitizeCanvasTreePathPart(sessionID), sanitizeCanvasTreePathPart(runID))
}

func (w *BlastWizard) canvasTreeRowSources(ctx context.Context, state canvasLaunchState, selectedRows []canvasSelectedRow) ([]phylo.RowSource, error) {
	rowSources, err := w.canvasTreeRowSourcesWithSkipped(ctx, state, selectedRows)
	if err != nil {
		return nil, err
	}
	return rowSources, nil
}

func (w *BlastWizard) canvasTreeRowSourcesWithSkipped(ctx context.Context, state canvasLaunchState, selectedRows []canvasSelectedRow) ([]phylo.RowSource, error) {
	return w.canvasTreeRowSourcesWithSkippedForSettings(ctx, state, selectedRows, phylo.DefaultTreeSettings())
}

func (w *BlastWizard) canvasTreeRowSourcesWithSkippedForSettings(ctx context.Context, state canvasLaunchState, selectedRows []canvasSelectedRow, settings phylo.TreeSettings) ([]phylo.RowSource, error) {
	itemIndexByTitle := make(map[string]int, len(state.Items))
	for i, item := range state.Items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = fmt.Sprintf("%d", i+1)
		}
		if _, exists := itemIndexByTitle[title]; !exists {
			itemIndexByTitle[title] = i
		}
	}
	out := make([]phylo.RowSource, 0, len(selectedRows))
	for _, selected := range selectedRows {
		choice, err := w.canvasTreeSequenceForSettings(ctx, selected, settings)
		if err != nil {
			return nil, err
		}
		row := selected.Row
		values := canvasTreeTableValues(row, selected.ItemTitle)
		itemIndex := itemIndexByTitle[strings.TrimSpace(selected.ItemTitle)]
		out = append(out, phylo.RowSource{
			ItemTitle:    strings.TrimSpace(selected.ItemTitle),
			ItemIndex:    itemIndex,
			RowIndex:     selected.RowIndex,
			CanvasRow:    row,
			Sequence:     strings.TrimSpace(choice.Sequence),
			SequenceKind: choice.Kind,
			SourceType:   string(row.Kind),
			OriginalHead: canvasRowOriginalHead(row, selected.ItemTitle),
			TableValues:  values,
		})
	}
	return out, nil
}

func canvasTreeSkipSummary(skipped []canvasTreeSkippedRow) string {
	if len(skipped) == 0 {
		return ""
	}
	lines := []string{
		fmt.Sprintf("%d selected Canvas row(s) cannot be used for the current tree refresh.", len(skipped)),
		"",
		"Choose Skip to uncheck the listed row(s), then keep automatically unchecking additional rows reported by mega-phgo-runtime during this refresh until the tree succeeds.",
		"",
	}
	limit := minInt(len(skipped), 8)
	for i := 0; i < limit; i++ {
		reason := strings.TrimSpace(skipped[i].Reason)
		if taxonID := strings.TrimSpace(skipped[i].TaxonID); taxonID != "" {
			reason = firstNonEmpty(reason, "was reported by mega-phgo-runtime") + " [" + taxonID + "]"
		}
		lines = append(lines, fmt.Sprintf("- %s row %d: %s", firstNonEmpty(strings.TrimSpace(skipped[i].ItemTitle), "Canvas"), skipped[i].RowIndex+1, reason))
	}
	if len(skipped) > limit {
		lines = append(lines, fmt.Sprintf("- ...and %d more row(s)", len(skipped)-limit))
	}
	return strings.Join(lines, "\n")
}

func unselectCanvasTreeSkippedRows(items []model.CanvasItem, skipped []canvasTreeSkippedRow) int {
	if len(skipped) == 0 {
		return 0
	}
	itemIndexRows := make(map[int]map[int]struct{}, len(skipped))
	itemRowSet := make(map[string]map[int]struct{}, len(skipped))
	for _, row := range skipped {
		if row.ItemIndex >= 0 && strings.TrimSpace(row.TaxonID) != "" {
			if _, ok := itemIndexRows[row.ItemIndex]; !ok {
				itemIndexRows[row.ItemIndex] = make(map[int]struct{})
			}
			itemIndexRows[row.ItemIndex][row.RowIndex] = struct{}{}
			continue
		}
		title := strings.TrimSpace(row.ItemTitle)
		if _, ok := itemRowSet[title]; !ok {
			itemRowSet[title] = make(map[int]struct{})
		}
		itemRowSet[title][row.RowIndex] = struct{}{}
	}
	changed := 0
	for i := range items {
		title := strings.TrimSpace(items[i].Title)
		rows := itemRowSet[title]
		if indexedRows := itemIndexRows[i]; len(indexedRows) > 0 {
			if len(rows) == 0 {
				rows = indexedRows
			} else {
				for rowIndex := range indexedRows {
					rows[rowIndex] = struct{}{}
				}
			}
		}
		if len(rows) == 0 {
			continue
		}
		selected := normalizeCanvasSelection(items[i].Selected, len(items[i].Rows))
		for rowIndex := range rows {
			if rowIndex >= 0 && rowIndex < len(selected) && selected[rowIndex] {
				selected[rowIndex] = false
				changed++
			}
		}
		items[i].Selected = selected
		updateCanvasItemSubtitle(&items[i])
	}
	return changed
}

func resolveCanvasTreeSkippedRows(selected []canvasSelectedRow, skipped []canvasTreeSkippedRow) []canvasTreeSkippedRow {
	if len(skipped) == 0 {
		return nil
	}
	out := make([]canvasTreeSkippedRow, 0, len(skipped))
	for _, row := range skipped {
		resolved := row
		if index := canvasTreeSkippedTaxonIndex(row); index >= 0 && index < len(selected) {
			selectedRow := selected[index]
			resolved.ItemTitle = selectedRow.ItemTitle
			resolved.ItemIndex = selectedRow.ItemIndex
			resolved.RowIndex = selectedRow.RowIndex
		} else if resolved.ItemIndex == 0 && strings.TrimSpace(resolved.TaxonID) != "" {
			resolved.ItemIndex = -1
		}
		out = append(out, resolved)
	}
	return out
}

func canvasTreeSkippedTaxonIndex(row canvasTreeSkippedRow) int {
	taxonID := strings.ToUpper(strings.TrimSpace(row.TaxonID))
	const prefix = "PHGOT"
	if !strings.HasPrefix(taxonID, prefix) {
		return -1
	}
	value := strings.TrimLeft(taxonID[len(prefix):], "0")
	if value == "" {
		value = "0"
	}
	index, err := strconv.Atoi(value)
	if err != nil || index <= 0 {
		return -1
	}
	return index - 1
}

func canvasTreeSkippedRowsFromRuntime(records []phylo.RuntimeSkippedRecord) []canvasTreeSkippedRow {
	if len(records) == 0 {
		return nil
	}
	out := make([]canvasTreeSkippedRow, 0, len(records))
	for _, record := range records {
		out = append(out, canvasTreeSkippedRow{
			ItemTitle: strings.TrimSpace(record.ItemTitle),
			ItemIndex: -1,
			RowIndex:  record.RowIndex,
			TaxonID:   strings.TrimSpace(record.TaxonID),
			Reason:    firstNonEmpty(strings.TrimSpace(record.Reason), "was reported by mega-phgo-runtime"),
		})
	}
	return out
}

func (w *BlastWizard) canvasTreeSequence(ctx context.Context, selected canvasSelectedRow) (string, error) {
	choice, err := w.canvasTreeSequenceForSettings(ctx, selected, phylo.DefaultTreeSettings())
	if err != nil {
		return "", err
	}
	return choice.Sequence, nil
}

type canvasTreeSequenceChoice struct {
	Sequence string
	Kind     phylo.SequenceKind
}

func (w *BlastWizard) canvasTreeSequenceForSettings(ctx context.Context, selected canvasSelectedRow, settings phylo.TreeSettings) (canvasTreeSequenceChoice, error) {
	row := selected.Row
	targetKind := canvasTreeTargetSequenceKind(settings)
	if targetKind == phylo.SequenceNucleotide {
		if choice, ok, err := w.canvasTreeNucleotideSequenceChoice(ctx, selected, settings); err != nil {
			return canvasTreeSequenceChoice{}, err
		} else if ok {
			return choice, nil
		}
		return canvasTreeSequenceChoice{Kind: targetKind}, nil
	}
	switch row.Kind {
	case model.CanvasKindFasta:
		if row.FASTA == nil {
			return canvasTreeSequenceChoice{Kind: targetKind}, nil
		}
		return canvasFastaTreeSequenceChoice(*row.FASTA, targetKind), nil
	case model.CanvasKindKeyword, model.CanvasKindBlast:
		if sequence := strings.TrimSpace(canvasRowStoredSequence(row)); sequence != "" {
			return canvasTreeSequenceChoice{Sequence: sequence, Kind: phylo.SequenceProtein}, nil
		}
		if !canvasTreeProteinSequenceResolvable(row) {
			return canvasTreeSequenceChoice{Kind: targetKind}, nil
		}
		record, err := w.canvasSequenceRecord(ctx, selected)
		if err != nil {
			return canvasTreeSequenceChoice{}, err
		}
		sequence := strings.TrimSpace(record.Sequence)
		return canvasTreeSequenceChoice{Sequence: sequence, Kind: phylo.SequenceProtein}, nil
	default:
		return canvasTreeSequenceChoice{Kind: targetKind}, nil
	}
}

func canvasTreeProteinSequenceResolvable(row model.CanvasRow) bool {
	switch row.Kind {
	case model.CanvasKindKeyword:
		return row.KeywordRow != nil && strings.TrimSpace(row.KeywordRow.SequenceID) != ""
	case model.CanvasKindBlast:
		return row.BlastRow != nil && strings.TrimSpace(firstNonEmpty(row.BlastRow.SequenceID, row.BlastRow.TranscriptID, row.BlastRow.Protein)) != ""
	default:
		return false
	}
}

func (w *BlastWizard) canvasTreeNucleotideSequenceChoice(ctx context.Context, selected canvasSelectedRow, settings phylo.TreeSettings) (canvasTreeSequenceChoice, bool, error) {
	row := selected.Row
	if row.Kind == model.CanvasKindFasta && row.FASTA != nil {
		source := *row.FASTA
		if sequence := strings.TrimSpace(source.NucleotideSequence); sequence != "" {
			return canvasTreeSequenceChoice{Sequence: sequence, Kind: phylo.SequenceNucleotide}, true, nil
		}
		if sourceSequenceKind(source.SequenceKind) == phylo.SequenceNucleotide {
			if sequence := strings.TrimSpace(source.Sequence); sequence != "" {
				return canvasTreeSequenceChoice{Sequence: sequence, Kind: phylo.SequenceNucleotide}, true, nil
			}
		}
	}
	src, sourceName, targetID, sequenceID, ok, err := w.canvasTreeNucleotideResolver(row)
	if err != nil {
		return canvasTreeSequenceChoice{}, false, err
	}
	if !ok {
		return canvasTreeSequenceChoice{}, false, nil
	}
	resolver, ok := src.(nucleotideSequenceResolver)
	if !ok {
		return canvasTreeSequenceChoice{}, false, nil
	}
	program := canvasTreeNucleotideProgram(settings)
	cacheKey := canvasTreeNucleotideSequenceCacheKey(sourceName, targetID, sequenceID, program)
	if sequence, ok := w.cachedProteinSequence(cacheKey); ok {
		return canvasTreeSequenceChoice{Sequence: sequence.Sequence, Kind: phylo.SequenceNucleotide}, true, nil
	}
	value, err, _ := w.proteinSequenceGroup.Do(cacheKey, func() (any, error) {
		if sequence, ok := w.cachedProteinSequence(cacheKey); ok {
			return sequence, nil
		}
		sequence, err := resolver.FetchNucleotideSequence(ctx, targetID, sequenceID, program)
		if err != nil {
			return model.ProteinSequenceData{}, err
		}
		w.storeProteinSequence(cacheKey, sequence)
		return sequence, nil
	})
	if err != nil {
		return canvasTreeSequenceChoice{}, false, err
	}
	sequence := value.(model.ProteinSequenceData)
	if strings.TrimSpace(sequence.Sequence) == "" {
		return canvasTreeSequenceChoice{}, false, nil
	}
	return canvasTreeSequenceChoice{Sequence: sequence.Sequence, Kind: phylo.SequenceNucleotide}, true, nil
}

func (w *BlastWizard) canvasTreeNucleotideResolver(row model.CanvasRow) (any, string, int, string, bool, error) {
	database := normalizeSnapshotDatabase(canvasRowSourceDatabase(row))
	var src any
	if database == "" && w.source != nil {
		src = w.source
		database = normalizeSnapshotDatabase(w.source.Name())
	} else if database != "" && w.source != nil && strings.EqualFold(normalizeSnapshotDatabase(w.source.Name()), database) {
		src = w.source
	} else if database != "" && database != string(model.CanvasKindFasta) {
		resolved, err := w.dataSourceForDatabase(database)
		if err != nil {
			return nil, "", 0, "", false, err
		}
		src = resolved
	}
	if src == nil {
		return nil, "", 0, "", false, nil
	}
	targetID, sequenceID := canvasTreeNucleotideTarget(row, w.lastKeywordSpecies)
	sequenceID = strings.TrimSpace(sequenceID)
	if sequenceID == "" {
		return nil, "", 0, "", false, nil
	}
	sourceName := database
	if named, ok := src.(interface{ Name() string }); ok {
		sourceName = named.Name()
	}
	return src, sourceName, targetID, sequenceID, true, nil
}

func canvasTreeNucleotideTarget(row model.CanvasRow, keywordSpecies model.SpeciesCandidate) (int, string) {
	switch row.Kind {
	case model.CanvasKindFasta:
		if row.FASTA == nil {
			return 0, ""
		}
		return row.FASTA.SourceProteomeID, firstNonEmpty(
			row.FASTA.PreferredSequenceID,
			row.FASTA.TranscriptID,
			row.FASTA.ProteinID,
			row.FASTA.GeneID,
		)
	case model.CanvasKindKeyword:
		if row.KeywordRow == nil {
			return 0, ""
		}
		return keywordSpecies.ProteomeID, firstNonEmpty(
			row.KeywordRow.SequenceID,
			row.KeywordRow.TranscriptID,
			row.KeywordRow.ProteinID,
			row.KeywordRow.GeneIdentifier,
		)
	case model.CanvasKindBlast:
		if row.BlastRow == nil {
			return 0, ""
		}
		return row.BlastRow.TargetID, firstNonEmpty(
			row.BlastRow.SequenceID,
			row.BlastRow.TranscriptID,
			row.BlastRow.Protein,
			row.BlastRow.SubjectID,
		)
	default:
		return 0, ""
	}
}

func canvasTreeNucleotideProgram(settings phylo.TreeSettings) string {
	settings = phylo.NormalizeTreeSettings(settings)
	if settings.AlignmentMethod == phylo.AlignmentClustalWCodons || settings.AlignmentMethod == phylo.AlignmentMUSCLECodons {
		return "tblastn"
	}
	return "blastn"
}

func canvasTreeNucleotideSequenceCacheKey(sourceName string, targetID int, sequenceID string, program string) string {
	sourceName = databaseDisplayName(firstNonEmpty(strings.TrimSpace(sourceName), "unknown"))
	if strings.EqualFold(sourceName, "lemna") {
		targetID = 0
	}
	return "nucleotide:" + sourceName + ":" + strconv.Itoa(targetID) + ":" + strings.ToLower(strings.TrimSpace(program)) + ":" + strings.TrimSpace(sequenceID)
}

func sourceSequenceKind(kind model.SequenceKind) phylo.SequenceKind {
	switch kind {
	case model.SequenceDNA:
		return phylo.SequenceNucleotide
	case model.SequenceProtein:
		return phylo.SequenceProtein
	default:
		return phylo.SequenceUnknown
	}
}

func canvasFastaTreeSequenceChoice(source model.QuerySequenceSource, targetKind phylo.SequenceKind) canvasTreeSequenceChoice {
	generic := strings.TrimSpace(source.Sequence)
	protein := strings.TrimSpace(source.ProteinSequence)
	nucleotide := strings.TrimSpace(source.NucleotideSequence)

	if targetKind == phylo.SequenceNucleotide {
		switch {
		case nucleotide != "":
			return canvasTreeSequenceChoice{Sequence: nucleotide, Kind: phylo.SequenceNucleotide}
		case generic != "":
			return canvasTreeSequenceChoice{Sequence: generic, Kind: sourceSequenceKind(source.SequenceKind)}
		case protein != "":
			return canvasTreeSequenceChoice{Sequence: protein, Kind: phylo.SequenceProtein}
		}
		return canvasTreeSequenceChoice{}
	}

	switch {
	case protein != "":
		return canvasTreeSequenceChoice{Sequence: protein, Kind: phylo.SequenceProtein}
	case generic != "":
		return canvasTreeSequenceChoice{Sequence: generic, Kind: sourceSequenceKind(source.SequenceKind)}
	case nucleotide != "":
		return canvasTreeSequenceChoice{Sequence: nucleotide, Kind: phylo.SequenceNucleotide}
	default:
		return canvasTreeSequenceChoice{}
	}
}

func canvasRowSourceDatabase(row model.CanvasRow) string {
	switch row.Kind {
	case model.CanvasKindKeyword:
		if row.KeywordRow != nil {
			return row.KeywordRow.SourceDatabase
		}
	case model.CanvasKindBlast:
		if row.BlastRow != nil {
			return row.BlastRow.SourceDatabase
		}
	case model.CanvasKindFasta:
		if row.FASTA != nil {
			return row.FASTA.SourceDatabase
		}
	}
	return ""
}

func canvasTreeTableValues(row model.CanvasRow, fallback string) map[string]string {
	values := map[string]string{
		"source_type":     string(row.Kind),
		"head":            canvasRowOriginalHead(row, fallback),
		"display_name":    strings.TrimSpace(row.DisplayName),
		"label_name":      canvasRowColumnValue(row, "label_name", fallback),
		"geneid":          canvasRowColumnValue(row, "gene_id", fallback),
		"species":         canvasRowColumnValue(row, "species", fallback),
		"blast_labelname": canvasRowColumnValue(row, "source_label_name", fallback),
		"blast_geneid":    canvasRowColumnValue(row, "source_gene_id", fallback),
	}
	values[phylo.PHgoDisplayNameSource] = phylo.FormatPHgoLabel(values["species"], values["geneid"], values["label_name"])
	values[phylo.YTDisplayNameSource] = phylo.FormatYTLabel(values["species"], values["geneid"], values["label_name"])
	values[phylo.YTV2DisplayNameSource] = phylo.FormatYTV2Label(values["species"], values["geneid"], values["label_name"])
	if phgoAlias := canvasRowColumnValue(row, "phgo_alias", fallback); phgoAlias != "" {
		values["phgo_alias"] = phgoAlias
	}
	if transcript := canvasRowColumnValue(row, "transcript", fallback); transcript != "" {
		values["transcript"] = transcript
	}
	return values
}

func canvasRowTreeSequence(row model.CanvasRow) string {
	switch row.Kind {
	case model.CanvasKindFasta:
		if row.FASTA != nil {
			return strings.TrimSpace(firstNonEmpty(row.FASTA.Sequence, row.FASTA.ProteinSequence, row.FASTA.NucleotideSequence))
		}
	case model.CanvasKindKeyword:
		if sequence := strings.TrimSpace(canvasRowStoredSequence(row)); sequence != "" {
			return sequence
		}
		if row.KeywordRow != nil {
			return strings.TrimSpace(firstNonEmpty(row.KeywordRow.SequenceID, row.KeywordRow.ProteinID, row.KeywordRow.TranscriptID))
		}
	case model.CanvasKindBlast:
		if sequence := strings.TrimSpace(canvasRowStoredSequence(row)); sequence != "" {
			return sequence
		}
		if row.BlastRow != nil {
			return strings.TrimSpace(firstNonEmpty(row.BlastRow.SequenceID, row.BlastRow.TranscriptID, row.BlastRow.Protein, row.BlastRow.SubjectID))
		}
	}
	return ""
}

func canvasRowStoredSequence(row model.CanvasRow) string {
	if row.SequenceData != nil {
		return strings.TrimSpace(row.SequenceData.Sequence)
	}
	switch row.Kind {
	case model.CanvasKindKeyword:
		if row.KeywordRow != nil {
			return strings.TrimSpace(inlineKeywordProteinSequenceData(*row.KeywordRow).Sequence)
		}
	case model.CanvasKindBlast:
		if row.BlastRow != nil {
			return strings.TrimSpace(extractInlineBlastSequence(*row.BlastRow))
		}
	}
	return ""
}

func canvasRowDisplayName(row model.CanvasRow, fallback string) string {
	if name := strings.TrimSpace(row.DisplayName); name != "" {
		return name
	}
	if name := canvasRowColumnValue(row, "label_name", fallback); name != "" {
		return name
	}
	return canvasRowOriginalHead(row, fallback)
}

func canvasRowOriginalHead(row model.CanvasRow, fallback string) string {
	switch row.Kind {
	case model.CanvasKindFasta:
		if row.FASTA != nil {
			return canvasHeadFromSource(*row.FASTA, fallback)
		}
	case model.CanvasKindKeyword:
		if row.KeywordRow != nil {
			return strings.TrimSpace(firstNonEmpty(row.KeywordRow.SequenceHeaderLabel, row.KeywordRow.ProteinID, row.KeywordRow.TranscriptID, row.KeywordRow.GeneIdentifier, row.KeywordRow.LabelName, fallback))
		}
	case model.CanvasKindBlast:
		if row.BlastRow != nil {
			return strings.TrimSpace(firstNonEmpty(row.BlastRow.SubjectID, row.BlastRow.Protein, row.BlastRow.SequenceID, row.BlastRow.TranscriptID, row.BlastRow.LabelName, fallback))
		}
	}
	return strings.TrimSpace(fallback)
}

func applyCanvasDisplayNameSourceToItems(items []model.CanvasItem, sourceColumnID string) {
	for i := range items {
		applyCanvasDisplayNameSource(&items[i], sourceColumnID)
	}
}

func applyCanvasDisplayNameSource(item *model.CanvasItem, sourceColumnID string) {
	if item == nil {
		return
	}
	sourceColumnID = strings.TrimSpace(sourceColumnID)
	if sourceColumnID == "" {
		sourceColumnID = phylo.DefaultDisplayNameSource
	}
	for i := range item.Rows {
		if item.Rows[i].DisplayNameLocked {
			continue
		}
		value := strings.TrimSpace(canvasRowColumnValue(item.Rows[i], sourceColumnID, item.Title))
		if value == "" {
			value = canvasRowOriginalHead(item.Rows[i], item.Title)
		}
		item.Rows[i].DisplayName = value
	}
}

func canvasRowColumnValue(row model.CanvasRow, columnID string, fallback string) string {
	columnID = strings.TrimSpace(columnID)
	switch columnID {
	case phylo.PHgoDisplayNameSource:
		return phylo.FormatPHgoLabel(
			canvasRowColumnValue(row, "species", fallback),
			canvasRowColumnValue(row, "gene_id", fallback),
			canvasRowColumnValue(row, "label_name", fallback),
		)
	case phylo.YTDisplayNameSource:
		return phylo.FormatYTLabel(
			canvasRowColumnValue(row, "species", fallback),
			canvasRowColumnValue(row, "gene_id", fallback),
			canvasRowColumnValue(row, "label_name", fallback),
		)
	case phylo.YTV2DisplayNameSource:
		return phylo.FormatYTV2Label(
			canvasRowColumnValue(row, "species", fallback),
			canvasRowColumnValue(row, "gene_id", fallback),
			canvasRowColumnValue(row, "label_name", fallback),
		)
	case "source_type":
		switch row.Kind {
		case model.CanvasKindFasta:
			if row.FASTA != nil {
				return strings.TrimSpace(firstNonEmpty(row.FASTA.SourceDatabase, string(model.CanvasKindFasta)))
			}
		case model.CanvasKindKeyword:
			if row.KeywordRow != nil {
				return strings.TrimSpace(firstNonEmpty(row.KeywordRow.SourceDatabase, string(model.CanvasKindKeyword)))
			}
		case model.CanvasKindBlast:
			if row.BlastRow != nil {
				return strings.TrimSpace(firstNonEmpty(row.BlastRow.SourceDatabase, string(model.CanvasKindBlast)))
			}
		}
	case "head":
		return canvasRowOriginalHead(row, fallback)
	case "species":
		switch row.Kind {
		case model.CanvasKindFasta:
			if row.FASTA != nil {
				return strings.TrimSpace(row.FASTA.OrganismShort)
			}
		case model.CanvasKindKeyword:
			if row.KeywordRow != nil {
				return strings.TrimSpace(row.KeywordRow.Genome)
			}
		case model.CanvasKindBlast:
			if row.BlastRow != nil {
				return strings.TrimSpace(row.BlastRow.Species)
			}
		}
	case "label_name":
		switch row.Kind {
		case model.CanvasKindFasta:
			if row.FASTA != nil {
				return strings.TrimSpace(row.FASTA.LabelName)
			}
		case model.CanvasKindKeyword:
			if row.KeywordRow != nil {
				return strings.TrimSpace(row.KeywordRow.LabelName)
			}
		case model.CanvasKindBlast:
			if row.BlastRow != nil {
				return strings.TrimSpace(row.BlastRow.LabelName)
			}
		}
	case "phgo_alias":
		switch row.Kind {
		case model.CanvasKindFasta:
			if row.FASTA != nil {
				return strings.TrimSpace(row.FASTA.PhgoAliases)
			}
		case model.CanvasKindKeyword:
			if row.KeywordRow != nil {
				return strings.TrimSpace(row.KeywordRow.PhgoAliases)
			}
		case model.CanvasKindBlast:
			if row.BlastRow != nil {
				return strings.TrimSpace(row.BlastRow.PhgoAliases)
			}
		}
	case "gene_id", "geneid":
		switch row.Kind {
		case model.CanvasKindFasta:
			if row.FASTA != nil {
				return strings.TrimSpace(row.FASTA.GeneID)
			}
		case model.CanvasKindKeyword:
			if row.KeywordRow != nil {
				return strings.TrimSpace(firstNonEmpty(row.KeywordRow.GeneLocus, row.KeywordRow.GeneIdentifier))
			}
		case model.CanvasKindBlast:
			if row.BlastRow != nil {
				return strings.TrimSpace(firstNonEmpty(row.BlastRow.SequenceID, row.BlastRow.TranscriptID, row.BlastRow.SubjectID, row.BlastRow.Protein))
			}
		}
	case "transcript", "transcript_id":
		switch row.Kind {
		case model.CanvasKindFasta:
			if row.FASTA != nil {
				return strings.TrimSpace(row.FASTA.TranscriptID)
			}
		case model.CanvasKindKeyword:
			if row.KeywordRow != nil {
				return strings.TrimSpace(row.KeywordRow.TranscriptID)
			}
		case model.CanvasKindBlast:
			if row.BlastRow != nil {
				return strings.TrimSpace(row.BlastRow.TranscriptID)
			}
		}
	case "source_label_name", "blast_labelname":
		switch row.Kind {
		case model.CanvasKindFasta:
			if row.FASTA != nil {
				return strings.TrimSpace(row.FASTA.BlastSourceLabelName)
			}
		case model.CanvasKindBlast:
			if row.BlastRow != nil {
				return strings.TrimSpace(row.BlastRow.BlastLabelName)
			}
		}
	case "source_gene_id", "blast_geneid":
		switch row.Kind {
		case model.CanvasKindFasta:
			if row.FASTA != nil {
				return strings.TrimSpace(row.FASTA.BlastSourceGeneID)
			}
		case model.CanvasKindBlast:
			if row.BlastRow != nil {
				return strings.TrimSpace(row.BlastRow.BlastGeneID)
			}
		}
	case "display_name":
		return strings.TrimSpace(row.DisplayName)
	}
	return ""
}

func normalizeCanvasTreeRowSources(sources []phylo.RowSource) ([]phylo.RowSource, phylo.SequenceKind, error) {
	normalized, _, kind, err := normalizeCanvasTreeRowSourcesWithSkipped(sources, nil, phylo.DefaultTreeSettings())
	if err != nil {
		return nil, kind, err
	}
	return normalized, kind, nil
}

func normalizeCanvasTreeRowSourcesWithSkipped(sources []phylo.RowSource, skipped []canvasTreeSkippedRow, settings phylo.TreeSettings) ([]phylo.RowSource, []canvasTreeSkippedRow, phylo.SequenceKind, error) {
	if len(sources) == 0 {
		return nil, skipped, phylo.SequenceUnknown, fmt.Errorf("no tree input records were selected")
	}
	usable := make([]phylo.RowSource, 0, len(sources))
	for _, src := range sources {
		next := src
		usable = append(usable, next)
	}
	return usable, skipped, canvasTreeTargetSequenceKind(settings), nil
}

func canvasTreeTargetSequenceKind(settings phylo.TreeSettings) phylo.SequenceKind {
	settings = phylo.NormalizeTreeSettings(settings)
	if settings.ConversionTarget == phylo.ConversionTargetDNA {
		return phylo.SequenceNucleotide
	}
	return phylo.SequenceProtein
}

func canvasTreeTargetLabel(settings phylo.TreeSettings) string {
	if canvasTreeTargetSequenceKind(settings) == phylo.SequenceNucleotide {
		return "DNA"
	}
	return "Protein"
}

func canvasDefaultSaveName(items []model.CanvasItem) string {
	if len(items) == 0 {
		return "canvas"
	}
	for _, item := range items {
		if value := strings.TrimSpace(item.Title); value != "" {
			return value
		}
	}
	return "canvas"
}

func canvasImportedFileShortName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if len([]rune(name)) <= 13 {
		return name
	}
	runes := []rune(name)
	prefix := string(runes[:13])
	return fmt.Sprintf("%s~%d", prefix, len(runes)-13)
}

func nextCanvasNumericID(items []model.CanvasItem) int {
	maxID := 0
	for _, item := range items {
		name := strings.TrimSpace(item.Title)
		if name == "" {
			continue
		}
		value := 0
		for _, ch := range name {
			if ch < '0' || ch > '9' {
				value = 0
				break
			}
			value = value*10 + int(ch-'0')
		}
		if value > maxID {
			maxID = value
		}
	}
	if maxID <= 0 {
		return 1
	}
	return maxID + 1
}

func (w *BlastWizard) canvasItemsFromInput(ctx context.Context, input string, index int, sourcePath string) ([]model.CanvasItem, error) {
	if strings.TrimSpace(input) == "" {
		return nil, fmt.Errorf("canvas input cannot be empty")
	}
	_ = ctx
	if canvasInputIsSessionSnapshot(input) || canvasInputIsSessionSnapshot(sourcePath) {
		items, err := w.canvasItemsFromSnapshotInput(input, sourcePath)
		if err != nil {
			return nil, err
		}
		return items, nil
	}
	rows, err := w.canvasRowsFromFastaInput(input, true)
	if err != nil {
		return nil, err
	}
	title := fmt.Sprintf("%d", index)
	if trimmedPath := strings.TrimSpace(sourcePath); trimmedPath != "" {
		if name := strings.TrimSpace(filepath.Base(trimmedPath)); name != "" && name != "." && !strings.EqualFold(filepath.Ext(trimmedPath), ".pgo") {
			title = canvasImportedFileShortName(name)
		}
	}
	item := model.CanvasItem{
		Title:        title,
		Kind:         model.CanvasKindFasta,
		Rows:         rows,
		Selected:     make([]bool, len(rows)),
		ImportedFrom: "fasta",
	}
	updateCanvasItemSubtitle(&item)
	return []model.CanvasItem{item}, nil
}

func (w *BlastWizard) canvasItemsFromSnapshotInput(input string, sourcePath string) ([]model.CanvasItem, error) {
	path, err := canvasSnapshotInputPath(input, sourcePath)
	if err != nil {
		return nil, err
	}
	snapshot, err := sessionsnapshot.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("open canvas snapshot %s: %w", path, err)
	}
	if database := inferSnapshotDatabase(snapshot); database != "" {
		src, err := w.dataSourceForDatabase(database)
		if err != nil {
			return nil, fmt.Errorf("restore canvas snapshot source %q: %w", database, err)
		}
		w.source = src
		w.prompt.SetDatabaseContext(databaseDisplayName(src.Name()))
	}
	w.hydrateCommonSnapshotState(snapshot)
	w.hydrateRuntimeCache(snapshot.RuntimeCache)
	w.hydrateSnapshotSequenceCache(snapshot.SequenceCache)
	var items []model.CanvasItem
	importName := canvasImportedFileShortName(filepath.Base(path))
	switch {
	case snapshot.Canvas != nil:
		items = canvasItemsFromSnapshot(snapshot.Canvas.Items)
		applyCanvasSnapshotTitleFallbacks(items, importName)
	case snapshot.Keyword != nil:
		rows, selected := selectedKeywordSnapshotRows(snapshot.Keyword.Groups, snapshot.Keyword.Selected)
		items = canvasItemsFromKeywordSelection(snapshot.Keyword.Groups, rows, nil, canvasItemActiveColumnsFromKeywordRows(rows))
		for i := range items {
			items[i].Selected = normalizeCanvasSelection(selected, len(items[i].Rows))
		}
		applyCanvasNoSidebarSnapshotTitle(items, importName)
	case snapshot.Blast != nil:
		items = canvasItemsFromBlastSnapshot(snapshot.Blast)
		if !canvasBlastSnapshotHasSidebarTitles(snapshot.Context.Mode, snapshot.Blast) {
			applyCanvasNoSidebarSnapshotTitle(items, importName)
		}
	default:
		return nil, fmt.Errorf("session snapshot %s contains no Canvas, keyword, or BLAST rows to add as a canvas", path)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("session snapshot %s contains no rows to add as a canvas", path)
	}
	for i := range items {
		items[i].ImportedFrom = "snapshot"
		updateCanvasItemSubtitle(&items[i])
	}
	items = w.hydrateCanvasRowSequenceData(items)
	return items, nil
}

func canvasBlastSnapshotHasSidebarTitles(mode string, snapshot *sessionsnapshot.BlastResultV1) bool {
	if snapshot == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(mode), string(ModeFamily)) {
		return false
	}
	return snapshot.OriginalRunCount > 1 || len(snapshot.Runs) > 1
}

func applyCanvasSnapshotTitleFallbacks(items []model.CanvasItem, importName string) {
	for i := range items {
		if canvasTitleIsGeneratedFallback(items[i].Title, len(items), i) {
			items[i].Title = canvasSnapshotImportTitle(importName, i)
		}
	}
}

func applyCanvasNoSidebarSnapshotTitle(items []model.CanvasItem, importName string) {
	for i := range items {
		items[i].Title = canvasSnapshotImportTitle(importName, i)
	}
}

func canvasSnapshotImportTitle(importName string, index int) string {
	importName = strings.TrimSpace(importName)
	if importName != "" {
		return importName
	}
	return fmt.Sprintf("%d", index+1)
}

func canvasTitleIsGeneratedFallback(title string, total int, index int) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return true
	}
	if total == 1 {
		return title == "1"
	}
	return title == fmt.Sprintf("%d", index+1)
}

func canvasSnapshotInputPath(input string, sourcePath string) (string, error) {
	for _, candidate := range []string{sourcePath, input} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || strings.Contains(candidate, "\n") || strings.HasPrefix(candidate, ">") {
			continue
		}
		path, err := sessionsnapshot.ResolveOpenPath(candidate, mustOutputDir())
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("canvas snapshot input could not be resolved")
}

func selectedKeywordSnapshotRows(groups []model.KeywordSearchGroup, selected []bool) ([]model.KeywordResultRow, []bool) {
	allRows := flattenKeywordSearchGroups(groups)
	if len(selected) != len(allRows) {
		selected = nil
	}
	rows := make([]model.KeywordResultRow, 0, len(allRows))
	mask := make([]bool, 0, len(allRows))
	for i, row := range allRows {
		if len(selected) > 0 && (i >= len(selected) || !selected[i]) {
			continue
		}
		rows = append(rows, row)
		mask = append(mask, true)
	}
	return rows, mask
}

func canvasItemsFromBlastSnapshot(snapshot *sessionsnapshot.BlastResultV1) []model.CanvasItem {
	if snapshot == nil {
		return nil
	}
	items := make([]model.CanvasItem, 0, len(snapshot.Runs))
	allRows := make([]model.BlastResultRow, 0)
	for _, run := range snapshot.Runs {
		allRows = append(allRows, run.Results.Rows...)
	}
	activeColumns := canvasItemActiveColumnsFromBlastRows(allRows)
	if len(snapshot.Runs) > 1 {
		return canvasItemsFromBlastRuns(blastRunsFromSnapshot(snapshot.Runs), snapshot.SelectedByRun, activeColumns)
	}
	rows := allRows
	mask := snapshot.Selected
	var item blastQueryItem
	if len(snapshot.Runs) == 1 {
		item = blastQueryItemFromSnapshot(snapshot.Runs[0].Item)
		rows = snapshot.Runs[0].Results.Rows
		if len(snapshot.SelectedByRun) > 0 {
			mask = snapshot.SelectedByRun[0]
		}
	}
	if len(mask) == len(rows) {
		selected := make([]model.BlastResultRow, 0, len(rows))
		for i, row := range rows {
			if i < len(mask) && mask[i] {
				selected = append(selected, row)
			}
		}
		rows = selected
	}
	if len(rows) == 0 {
		return nil
	}
	if len(snapshot.Runs) == 1 {
		title := "1"
		if snapshot.OriginalRunCount > 1 {
			title = canvasBlastRunSidebarTitle(0, item)
		}
		items = append(items, canvasItemFromBlastRowsWithSource(title, item, rows, nil, activeColumns))
		return items
	}
	items = append(items, canvasItemFromBlastRows("1", "", rows, nil, activeColumns))
	return items
}

func canvasInputIsSessionSnapshot(input string) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || strings.Contains(trimmed, "\n") || strings.HasPrefix(trimmed, ">") {
		return false
	}
	if strings.EqualFold(filepath.Ext(trimmed), sessionsnapshot.FileExtension) {
		return true
	}
	path, err := sessionsnapshot.ResolveOpenPath(trimmed, mustOutputDir())
	return err == nil && strings.EqualFold(filepath.Ext(path), sessionsnapshot.FileExtension)
}

func canvasSnapshotMustOpenSessionMessage() string {
	return "Canvas session snapshots (.pgo) cannot be added to an existing canvas. Use Explore -> Open session to open the canvas snapshot instead."
}

func (w *BlastWizard) canvasRowsFromFastaInput(input string, includeBlastSource bool) ([]model.CanvasRow, error) {
	if canvasInputIsSessionSnapshot(input) {
		return nil, errors.New(canvasSnapshotMustOpenSessionMessage())
	}
	records := splitBlastInputRecords(input)
	if len(records) == 0 {
		return nil, fmt.Errorf("no FASTA records were found")
	}
	rows := make([]model.CanvasRow, 0, len(records))
	for _, record := range records {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}
		if !strings.HasPrefix(record, ">") {
			return nil, fmt.Errorf("adding rows only supports FASTA input")
		}
		source, ok := parseFastaQuerySequenceInput(record)
		if !ok || source == nil {
			header, sequence := splitCanvasFastaHeaderAndSequence(record)
			if header == "" {
				if canvasInputRecordIsIgnoredPHGONote(record) {
					continue
				}
				return nil, fmt.Errorf("invalid FASTA input near %q", oneLinePreview(record))
			}
			source = &model.QuerySequenceSource{
				Annotation:     strings.TrimSpace(header),
				Sequence:       sequence,
				SourceDatabase: "canvas",
			}
		}
		source = canvasImportSourceOnlyHeaderMetadata(source)
		if strings.TrimSpace(source.SourceDatabase) == "" || strings.EqualFold(source.SourceDatabase, "fasta") {
			source.SourceDatabase = string(model.CanvasKindFasta)
		}
		row := model.CanvasRow{
			Kind:  model.CanvasKindFasta,
			FASTA: source,
		}
		if displayName := strings.TrimSpace(source.PhgoCanvasDisplay); displayName != "" {
			row.DisplayName = displayName
			row.DisplayNameLocked = true
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no FASTA rows could be created")
	}
	if !includeBlastSource {
		return rows, nil
	}
	return assignCanvasBlastSourceRowNumbers(rows), nil
}

func canvasInputRecordIsIgnoredPHGONote(record string) bool {
	record = strings.TrimSpace(strings.ReplaceAll(record, "\r", ""))
	if record == "" {
		return false
	}
	lines := strings.Split(record, "\n")
	if len(lines) == 0 {
		return false
	}
	return fastautil.IsIgnoredPHGONoteHeader(lines[0])
}

func assignCanvasBlastSourceRowNumbers(rows []model.CanvasRow) []model.CanvasRow {
	if len(rows) == 0 {
		return nil
	}
	sources := make([]model.CanvasRow, 0, len(rows))
	regular := make([]model.CanvasRow, 0, len(rows))
	for _, row := range rows {
		if row.Kind == model.CanvasKindFasta && row.FASTA != nil && row.FASTA.PhgoBlastQuerySource {
			sources = append(sources, row)
			continue
		}
		regular = append(regular, row)
	}
	out := make([]model.CanvasRow, 0, len(rows))
	for _, row := range assignCanvasRowNumbers(sources, -len(sources)) {
		out = append(out, row)
	}
	for _, row := range assignCanvasRowNumbers(regular, 1) {
		out = append(out, row)
	}
	return out
}

func splitCanvasFastaHeaderAndSequence(input string) (string, string) {
	value := strings.TrimSpace(strings.ReplaceAll(input, "\r", ""))
	if value == "" || !strings.HasPrefix(value, ">") {
		return "", ""
	}
	lines := strings.Split(value, "\n")
	if len(lines) == 0 {
		return "", ""
	}
	header := strings.TrimSpace(strings.TrimPrefix(lines[0], ">"))
	if header == "" || fastautil.IsIgnoredPHGONoteHeader(header) {
		return "", ""
	}
	sequenceLines := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ">") {
			continue
		}
		sequenceLines = append(sequenceLines, line)
	}
	return header, sanitizeSequence(strings.Join(sequenceLines, "\n"))
}

func canvasImportSourceOnlyHeaderMetadata(source *model.QuerySequenceSource) *model.QuerySequenceSource {
	if source == nil {
		return nil
	}
	next := *source
	header := strings.TrimSpace(next.Annotation)
	if parsed, ok := parsePhgoFastaHeader(header); ok {
		next.LabelName = parsed.LabelName
		next.GeneID = parsed.GeneID
		next.OrganismShort = parsed.Species
		next.Annotation = parsed.RawHeader
		next.BlastSourceLabelName = parsed.BlastSourceLabelName
		next.BlastSourceGeneID = parsed.BlastSourceGeneID
		next.PhgoRowNumber = parsed.RowNumber
		next.PhgoHasRowNumber = parsed.HasRowPart
		next.PhgoBlastQuerySource = parsed.IsBlastQuerySource
		next.PhgoCanvasRawRow = parsed.CanvasRawRowNumber
		next.PhgoCanvasHasRawRow = parsed.CanvasHasRawRow
		next.PhgoCanvasTitle = parsed.CanvasItemTitle
		next.PhgoCanvasDisplay = parsed.CanvasDisplayName
		next.PreferredSequenceID = ""
		next.ProteinID = ""
		next.TranscriptID = ""
		next.PhgoAliases = ""
		next.Aliases = ""
		next.Symbols = ""
		next.Synonyms = ""
		next.AutoDefine = ""
		return &next
	}
	next.LabelName = ""
	next.GeneID = ""
	next.OrganismShort = ""
	next.BlastSourceLabelName = ""
	next.BlastSourceGeneID = ""
	next.PhgoRowNumber = 0
	next.PhgoHasRowNumber = false
	next.PhgoBlastQuerySource = false
	next.PhgoCanvasRawRow = 0
	next.PhgoCanvasHasRawRow = false
	next.PhgoCanvasTitle = ""
	next.PhgoCanvasDisplay = ""
	next.PreferredSequenceID = ""
	next.ProteinID = ""
	next.TranscriptID = ""
	next.PhgoAliases = ""
	next.Aliases = ""
	next.Symbols = ""
	next.Synonyms = ""
	next.AutoDefine = ""
	return &next
}

func normalizeCanvasSelection(selected []bool, size int) []bool {
	if size <= 0 {
		return nil
	}
	if len(selected) == size {
		return append([]bool(nil), selected...)
	}
	out := make([]bool, size)
	copy(out, selected)
	return out
}

func normalizeCanvasMSAFlags(flags []bool, size int) []bool {
	if size <= 0 {
		return nil
	}
	if len(flags) == size {
		return append([]bool(nil), flags...)
	}
	out := make([]bool, size)
	copy(out, flags)
	return out
}

func updateCanvasItemSubtitle(item *model.CanvasItem) {
	if item == nil {
		return
	}
	item.Subtitle = canvasSelectionSummary(len(item.Rows), normalizeCanvasSelection(item.Selected, len(item.Rows)))
}

func canvasSelectionSummary(total int, selected []bool) string {
	if total < 0 {
		total = 0
	}
	selectedCount := countTrueBools(selected)
	if selectedCount > total {
		selectedCount = total
	}
	return fmt.Sprintf("%d/%d lines", selectedCount, total)
}

func nextCanvasRowNumber(rows []model.CanvasRow) int {
	maxNumber := 0
	for i, row := range rows {
		value := row.RowNumber
		if value <= 0 {
			value = i + 1
		}
		if value > maxNumber {
			maxNumber = value
		}
	}
	return maxNumber + 1
}

func assignCanvasRowNumbers(rows []model.CanvasRow, start int) []model.CanvasRow {
	out := make([]model.CanvasRow, len(rows))
	copy(out, rows)
	next := start
	if next == 0 {
		next = 1
	}
	for i := range out {
		if out[i].RowNumber != 0 {
			if next > 0 && out[i].RowNumber >= next {
				next = out[i].RowNumber + 1
			}
			continue
		}
		out[i].RowNumber = next
		next++
		if next == 0 {
			next = 1
		}
	}
	return out
}

type canvasRowView struct {
	Number     int
	SourceType string
	Head       string
	Species    string
	LabelName  string
	GeneID     string
	SourceGene string
}

func canvasHeadFromSource(source model.QuerySequenceSource, fallback string) string {
	if label := strings.TrimSpace(source.LabelName); label != "" {
		return label
	}
	if label := strings.TrimSpace(source.PreferredSequenceID); label != "" {
		return label
	}
	if label := strings.TrimSpace(source.ProteinID); label != "" {
		return label
	}
	if label := strings.TrimSpace(source.TranscriptID); label != "" {
		return label
	}
	if label := strings.TrimSpace(source.GeneID); label != "" {
		return label
	}
	if label := strings.TrimSpace(source.Annotation); label != "" {
		fields := strings.Fields(label)
		if len(fields) > 0 {
			return fields[0]
		}
	}
	return strings.TrimSpace(fallback)
}

func canvasRowViewFromRow(row model.CanvasRow) canvasRowView {
	switch row.Kind {
	case model.CanvasKindFasta:
		if row.FASTA != nil {
			return canvasRowView{
				SourceType: strings.TrimSpace(firstNonEmpty(row.FASTA.SourceDatabase, string(model.CanvasKindFasta))),
				Head:       canvasHeadFromSource(*row.FASTA, ""),
				Species:    strings.TrimSpace(row.FASTA.OrganismShort),
				LabelName:  strings.TrimSpace(row.FASTA.LabelName),
				GeneID:     strings.TrimSpace(row.FASTA.GeneID),
				SourceGene: strings.TrimSpace(row.FASTA.BlastSourceGeneID),
			}
		}
	case model.CanvasKindKeyword:
		if row.KeywordRow != nil {
			return canvasRowView{
				SourceType: strings.TrimSpace(firstNonEmpty(row.KeywordRow.SourceDatabase, string(model.CanvasKindKeyword))),
				Head:       strings.TrimSpace(firstNonEmpty(row.KeywordRow.SequenceHeaderLabel, row.KeywordRow.ProteinID, row.KeywordRow.TranscriptID, row.KeywordRow.GeneIdentifier)),
				Species:    strings.TrimSpace(row.KeywordRow.Genome),
				LabelName:  strings.TrimSpace(row.KeywordRow.LabelName),
				GeneID:     strings.TrimSpace(row.KeywordRow.GeneIdentifier),
			}
		}
	case model.CanvasKindBlast:
		if row.BlastRow != nil {
			return canvasRowView{
				SourceType: strings.TrimSpace(firstNonEmpty(row.BlastRow.SourceDatabase, string(model.CanvasKindBlast))),
				Head:       strings.TrimSpace(firstNonEmpty(row.BlastRow.SubjectID, row.BlastRow.Protein, row.BlastRow.SequenceID)),
				Species:    strings.TrimSpace(row.BlastRow.Species),
				LabelName:  strings.TrimSpace(row.BlastRow.LabelName),
				GeneID:     strings.TrimSpace(firstNonEmpty(row.BlastRow.SequenceID, row.BlastRow.TranscriptID, row.BlastRow.Protein)),
				SourceGene: strings.TrimSpace(row.BlastRow.BlastGeneID),
			}
		}
	}
	return canvasRowView{SourceType: string(row.Kind)}
}

func canvasRowsSorted(rows []model.CanvasRow) []model.CanvasRow {
	type indexed struct {
		row  model.CanvasRow
		view canvasRowView
		idx  int
	}
	indexedRows := make([]indexed, 0, len(rows))
	for i, row := range rows {
		indexedRows = append(indexedRows, indexed{row: row, view: canvasRowViewFromRow(row), idx: i})
	}
	sort.SliceStable(indexedRows, func(i, j int) bool {
		left, right := indexedRows[i].view, indexedRows[j].view
		if cmp := strings.Compare(strings.ToLower(left.Head), strings.ToLower(right.Head)); cmp != 0 {
			return cmp < 0
		}
		if cmp := strings.Compare(strings.ToLower(left.GeneID), strings.ToLower(right.GeneID)); cmp != 0 {
			return cmp < 0
		}
		return indexedRows[i].idx < indexedRows[j].idx
	})
	out := make([]model.CanvasRow, 0, len(rows))
	for _, item := range indexedRows {
		out = append(out, item.row)
	}
	return out
}

func (w *BlastWizard) writeCanvasSessionSnapshot(state canvasLaunchState, settings exportSettings) error {
	path := sessionsnapshot.DefaultFilePath(settings.OutputDir, firstNonEmpty(settings.BaseName, "canvas"))
	return w.writeSessionSnapshotWithProgress(path, "Writing canvas session snapshot...", func(ctx context.Context, update func(int, string)) (sessionsnapshot.Snapshot, error) {
		progress := safeProgress(update)
		progress(0, "Preparing canvas snapshot data...")
		tree, treeArtifacts, treePayloads, err := w.snapshotCanvasTreeState(state.Items)
		if err != nil {
			return sessionsnapshot.Snapshot{}, err
		}
		msa := w.snapshotCanvasMSAState(state.Items)
		progress(1, "Collecting canvas sequence cache...")
		sequenceCache, err := w.snapshotCanvasSequenceCache(ctx, state.Items)
		if err != nil {
			return sessionsnapshot.Snapshot{}, err
		}
		progress(2, "Canvas snapshot data prepared.")
		return sessionsnapshot.Snapshot{
			Context: w.snapshotContext(string(ModeCanvas), "canvas-result", "Canvas"),
			Canvas: &sessionsnapshot.CanvasResultV1{
				Items:         snapshotCanvasItems(state.Items),
				CurrentItem:   state.CurrentItem,
				NextNumericID: state.NextNumericID,
				ImportedFrom:  state.ImportedFrom,
				Tree:          tree,
			},
			CanvasMSA: msa,
			CanvasReview: &sessionsnapshot.CanvasReviewStateV1{
				SelectionState: w.prompt.SnapshotCanvasReviewState(canvasStateKey("canvas")),
			},
			SequenceCache:    sequenceCache,
			ExportSettings:   snapshotExportSettings(w.prompt.SnapshotExportSettings(), settings),
			Handoff:          w.snapshotHandoffState(),
			Artifacts:        treeArtifacts,
			RuntimeCache:     w.snapshotRuntimeCache(),
			ArtifactPayloads: treePayloads,
		}, nil
	})
}

func (w *BlastWizard) snapshotCanvasMSAState(items []model.CanvasItem) *sessionsnapshot.CanvasMSAV1 {
	payload := w.canvasTreeLastMSAPayload
	if payload.UpdatedAt.IsZero() && strings.TrimSpace(payload.AlignedFASTA) == "" {
		payload = w.canvasTreeLastPayload
	}
	state := w.canvasTreeMSAState
	if server := w.canvasTreeViewer; server != nil {
		live := server.GetMSAState(w.canvasTreeSessionID())
		if len(live.Rows) > 0 || len(live.Sequences) > 0 || len(live.Groups) > 0 || len(live.Annotations) > 0 || len(live.Features) > 0 || len(live.Settings) > 0 || len(live.Colours) > 0 || len(live.Markers) > 0 {
			state = live
			w.canvasTreeMSAState = live
		}
	}
	if len(state.Rows) == 0 && (!payload.UpdatedAt.IsZero() || strings.TrimSpace(payload.AlignedFASTA) != "") {
		state = canvasMSAStateFromItems(items, payload)
	}
	if len(state.Rows) == 0 && payload.UpdatedAt.IsZero() && strings.TrimSpace(payload.AlignedFASTA) == "" {
		return nil
	}
	if state.SchemaVersion == 0 {
		state.SchemaVersion = 2
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now()
	}
	return &sessionsnapshot.CanvasMSAV1{
		State:            state,
		LastPayload:      payload,
		LastAlignedFASTA: strings.TrimSpace(payload.AlignedFASTA),
	}
}

func (w *BlastWizard) snapshotCanvasTreeState(currentItems ...[]model.CanvasItem) (*sessionsnapshot.CanvasTreeV2, *sessionsnapshot.ArtifactManifestV2, map[string][]byte, error) {
	panelState := sanitizeCanvasTreePanelStateForSnapshot(w.prompt.SnapshotCanvasTreePanelState(canvasStateKey("canvas")))
	plan := w.canvasTreeLastPlan
	payload := w.canvasTreeLastPayload
	viewerState := sanitizeCanvasTreeViewerStateForSnapshot(w.canvasTreeViewerState)
	if len(currentItems) > 0 {
		payload, plan = syncCanvasTreeSnapshotPreview(payload, plan, w.selectedCanvasRowsInCurrentOrder(currentItems[0], false), panelState)
	}
	if server := w.canvasTreeViewer; server != nil {
		if liveState, err := getViewerState(context.Background(), server, w.canvasTreeSessionID()); err == nil {
			viewerState = sanitizeCanvasTreeViewerStateForSnapshot(liveState)
			w.canvasTreeViewerState = json.RawMessage(append([]byte(nil), liveState...))
		}
	}
	if strings.TrimSpace(plan.BaseDir) == "" && !panelState.EnabledEver && strings.TrimSpace(panelState.DisplayNameSource) == "" {
		return nil, nil, nil, nil
	}
	tree := &sessionsnapshot.CanvasTreeV2{
		PanelState:       panelState,
		LastPayload:      payload,
		ViewerState:      viewerState,
		LastArtifactDir:  strings.TrimSpace(plan.BaseDir),
		LastRunID:        strings.TrimSpace(plan.RunID),
		LastAlignedFASTA: strings.TrimSpace(plan.AlignedFASTA),
		LastNewick:       strings.TrimSpace(plan.Newick),
		Fingerprints:     plan.Fingerprints,
	}
	manifest, payloads, err := snapshotCanvasTreeArtifacts(plan)
	if err != nil {
		return nil, nil, nil, err
	}
	if manifest != nil {
		tree.ArtifactPaths = make([]string, 0, len(manifest.Entries))
		for _, entry := range manifest.Entries {
			tree.ArtifactPaths = append(tree.ArtifactPaths, strings.TrimSpace(entry.Path))
		}
	}
	if strings.TrimSpace(plan.BaseDir) != "" {
		if runManifest, err := phylo.LoadRunManifest(plan.BaseDir); err == nil {
			tree.LastManifest = runManifest
		}
	}
	return tree, manifest, payloads, nil
}

func sanitizeCanvasTreePanelStateForSnapshot(state tui.CanvasTreePanelState) tui.CanvasTreePanelState {
	state.Expanded = false
	state.Focused = false
	state.CurrentControl = 0
	state.ScrollOffset = 0
	if state.AlignmentParams == nil {
		state.AlignmentParams = map[string]string{}
	}
	if state.TreeParams == nil {
		state.TreeParams = map[string]string{}
	}
	state.EnabledEver = state.EnabledEver ||
		strings.TrimSpace(state.DisplayNameSource) != "" ||
		strings.TrimSpace(state.ConversionTarget) != "" ||
		state.ConversionSkipUnselect ||
		strings.TrimSpace(state.AlignmentMethod) != "" ||
		strings.TrimSpace(state.TreeMethod) != "" ||
		len(state.AlignmentParams) > 0 ||
		len(state.TreeParams) > 0
	return state
}

func sanitizeCanvasTreeViewerStateForSnapshot(raw json.RawMessage) json.RawMessage {
	raw = json.RawMessage(append([]byte(nil), raw...))
	if len(raw) == 0 || !json.Valid(raw) {
		return nil
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		return raw
	}
	if reactree, ok := state["reactree"].(map[string]any); ok {
		for _, key := range []string{
			"search", "searchText", "searchResults", "activeMenu", "openMenu", "ribbonTab",
			"toolbarMode", "contextMenu", "hoveredNode", "selectedNode", "selectionBox",
			"rerootMode", "flipMode", "swapMode", "cladeEditor", "nodeInfo",
		} {
			delete(reactree, key)
		}
		state["reactree"] = reactree
	}
	if phgo, ok := state["phgo"].(map[string]any); ok {
		delete(phgo, "viewport")
		state["phgo"] = phgo
	}
	data, err := json.Marshal(state)
	if err != nil {
		return raw
	}
	return json.RawMessage(data)
}

func syncCanvasTreeSnapshotPreview(payload phylo.ViewerPayload, plan phylo.RunPlan, selected []canvasSelectedRow, panelState tui.CanvasTreePanelState) (phylo.ViewerPayload, phylo.RunPlan) {
	if len(selected) == 0 || len(payload.Metadata.Records) == 0 {
		return payload, plan
	}
	if len(selected) != len(payload.Metadata.Records) {
		return payload, plan
	}
	remaining := append([]phylo.InputRecord(nil), payload.Metadata.Records...)
	records := make([]phylo.InputRecord, 0, len(selected))
	displaySource := firstNonEmpty(strings.TrimSpace(panelState.DisplayNameSource), payload.Metadata.DisplayNameSource, phylo.DefaultDisplayNameSource)
	for _, current := range selected {
		match := -1
		for i := range remaining {
			if canvasTreeRecordMatchesSelectedRow(remaining[i], current) {
				match = i
				break
			}
		}
		if match < 0 {
			return payload, plan
		}
		record := remaining[match]
		row := current.Row
		record.SourceType = string(row.Kind)
		record.OriginalHead = canvasRowOriginalHead(row, current.ItemTitle)
		record.CanvasItem = strings.TrimSpace(current.ItemTitle)
		record.CanvasItemIndex = current.ItemIndex
		record.CanvasRow = current.RowIndex
		record.TableValues = canvasTreeTableValues(row, current.ItemTitle)
		record.DisplayName = canvasTreeDisplayNameFromSource(row, current.ItemTitle, displaySource, record.TableValues)
		records = append(records, record)
		remaining = append(remaining[:match], remaining[match+1:]...)
	}
	applyCanvasCoordinateMetadata(records, nil)
	payload.Metadata.DisplayNameSource = displaySource
	payload.Metadata.Records = records
	payload.Metadata.GeneratedAt = time.Now()
	if strings.TrimSpace(payload.AlignedFASTA) == "" {
		payload.AlignedFASTA = strings.TrimSpace(plan.AlignedFASTA)
	}
	if strings.TrimSpace(payload.Newick) == "" {
		payload.Newick = strings.TrimSpace(plan.Newick)
	}
	if payload.UpdatedAt.IsZero() {
		payload.UpdatedAt = time.Now()
	}
	plan.Records = records
	plan.Metadata = payload.Metadata
	if strings.TrimSpace(plan.AlignedFASTA) == "" {
		plan.AlignedFASTA = strings.TrimSpace(payload.AlignedFASTA)
	}
	if strings.TrimSpace(plan.Newick) == "" {
		plan.Newick = strings.TrimSpace(payload.Newick)
	}
	if strings.TrimSpace(plan.Settings.DisplayNameSource) == "" {
		plan.Settings = treeSettingsFromSnapshotPanel(panelState)
	}
	fingerprints := plan.Fingerprints
	preview := phylo.BuildFingerprints(records, plan.Settings, plan.AlignedFASTA, plan.Newick)
	fingerprints.Preview = preview.Preview
	plan.Fingerprints = fingerprints
	return payload, plan
}

func canvasTreeDisplayNameFromSource(row model.CanvasRow, fallback string, sourceColumn string, values map[string]string) string {
	if row.DisplayNameLocked {
		if name := strings.TrimSpace(row.DisplayName); name != "" {
			return name
		}
	}
	sourceColumn = strings.TrimSpace(sourceColumn)
	if sourceColumn == phylo.PHgoDisplayNameSource || sourceColumn == phylo.YTDisplayNameSource || sourceColumn == phylo.YTV2DisplayNameSource {
		if value := strings.TrimSpace(values[sourceColumn]); value != "" {
			return value
		}
	}
	return canvasRowDisplayName(row, fallback)
}

func applyCanvasCoordinateMetadata(records []phylo.InputRecord, meta *phylo.Metadata) {
	for i := range records {
		name := strings.TrimSpace(records[i].BaseDisplayName)
		if name == "" {
			name = strings.TrimSpace(records[i].DisplayName)
		}
		if name == "" {
			name = strings.TrimSpace(records[i].TaxonID)
		}
		records[i].BaseDisplayName = name
		records[i].DisplayPrefix = canvasCoordinatePrefix(records[i].CanvasItemIndex, records[i].CanvasRow)
		records[i].DisplayName = name
	}
	if meta != nil {
		meta.Records = records
	}
}

func canvasCoordinatePrefix(canvasItemIndex int, rowIndex int) string {
	return fmt.Sprintf("[%d,%d]", canvasItemIndex+1, rowIndex+1)
}

func canvasTreeRecordMatchesSelectedRow(record phylo.InputRecord, selected canvasSelectedRow) bool {
	if record.CanvasRow != selected.RowIndex {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(record.SourceType), string(selected.Row.Kind)) {
		return false
	}
	currentHead := canvasRowOriginalHead(selected.Row, selected.ItemTitle)
	return strings.TrimSpace(record.OriginalHead) == "" || strings.EqualFold(strings.TrimSpace(record.OriginalHead), strings.TrimSpace(currentHead))
}

func snapshotCanvasTreeArtifacts(plan phylo.RunPlan) (*sessionsnapshot.ArtifactManifestV2, map[string][]byte, error) {
	baseDir := strings.TrimSpace(plan.BaseDir)
	if baseDir == "" {
		return nil, nil, nil
	}
	files := []struct {
		name        string
		kind        string
		mediaType   string
		description string
	}{
		{"input.fasta", "tree-input", "text/x-fasta", "Tree input FASTA"},
		{"input.meta.json", "tree-metadata", "application/json", "Tree input metadata"},
		{"runtime-request.json", "tree-runtime-request", "application/json", "mega-phgo-runtime request"},
		{"runtime-response.json", "tree-runtime-response", "application/json", "mega-phgo-runtime response"},
		{"aligned.fasta", "tree-alignment", "text/x-fasta", "Aligned FASTA"},
		{"tree.nwk", "tree-newick", "text/x-newick", "Inferred Newick tree"},
		{"viewer.payload.json", "tree-viewer-payload", "application/json", "Reactree viewer payload"},
		{"run.manifest.json", "tree-run-manifest", "application/json", "Tree run manifest"},
		{"runtime.stdout.txt", "tree-runtime-log", "text/plain", "Runtime stdout log"},
		{"runtime.stderr.txt", "tree-runtime-log", "text/plain", "Runtime stderr log"},
		{"runtime-summary.txt", "tree-runtime-summary", "text/plain", "Runtime summary"},
		{"runtime.log", "tree-runtime-log", "text/plain", "Runtime log"},
	}
	entries := make([]sessionsnapshot.ArtifactEntryV2, 0, len(files))
	payloads := make(map[string][]byte, len(files))
	sessionPart := sanitizeCanvasTreePathPart(plan.SessionID)
	runPart := sanitizeCanvasTreePathPart(plan.RunID)
	if runPart == "canvas" {
		runPart = "latest"
	}
	for _, file := range files {
		sourcePath := filepath.Join(baseDir, file.name)
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, err
		}
		archivePath := filepath.ToSlash(filepath.Join("artifacts", "tree", sessionPart, runPart, file.name))
		entries = append(entries, sessionsnapshot.ArtifactEntryV2{
			ID:          "tree/" + file.name,
			Path:        archivePath,
			Kind:        file.kind,
			MediaType:   file.mediaType,
			Description: file.description,
			SourcePath:  sourcePath,
			RestorePath: sourcePath,
		})
		payloads[archivePath] = data
	}
	if len(entries) == 0 {
		return nil, nil, nil
	}
	return &sessionsnapshot.ArtifactManifestV2{Entries: entries}, payloads, nil
}

type canvasSelectedRow struct {
	ItemTitle string
	ItemIndex int
	RowIndex  int
	Row       model.CanvasRow
}

func cloneCanvasSelectedRows(rows []canvasSelectedRow) []canvasSelectedRow {
	if len(rows) == 0 {
		return nil
	}
	out := make([]canvasSelectedRow, len(rows))
	copy(out, rows)
	return out
}

func canvasExportCompleteMessage(settings exportSettings) string {
	lines := make([]string, 0, 2)
	if settings.WriteText {
		lines = append(lines, "Canvas FASTA export saved to "+settings.OutputDir+".")
	}
	if settings.WriteSession {
		lines = append(lines, "Canvas snapshot saved to "+settings.OutputDir+".")
	}
	if len(lines) == 0 {
		return "Nothing was exported."
	}
	return strings.Join(lines, "\n")
}

func (w *BlastWizard) exportCanvasSelections(ctx context.Context, state canvasLaunchState, settings exportSettings) error {
	if !settings.WriteText && !settings.WriteSession {
		return nil
	}
	if settings.WriteText {
		exportRows := w.selectedCanvasRowsInCurrentOrderForExport(state.Items, settings.WriteAllRows)
		if len(exportRows) == 0 {
			return fmt.Errorf("no selected canvas rows are available for FASTA export")
		}
		records, err := w.canvasSequenceRecordsForExport(ctx, exportRows)
		if err != nil {
			return err
		}
		records = applyCanvasHeaderMode(records, exportRows, settings.fastaHeaderMode())
		textPath := filepath.Join(settings.OutputDir, settings.BaseName+".fasta")
		if err := withSpinner(w.out, "Writing canvas FASTA file...", func() error {
			return export.WriteProteinSequencesText(textPath, records)
		}); err != nil {
			return err
		}
	}
	if settings.WriteSession {
		if err := w.writeCanvasSessionSnapshot(state, settings); err != nil {
			return err
		}
	}
	return nil
}

func (w *BlastWizard) selectedCanvasRowsInCurrentOrder(items []model.CanvasItem, includeUnchecked bool) []canvasSelectedRow {
	return selectedCanvasRowsInVisibleOrder(items, w.prompt.SnapshotCanvasReviewState(canvasStateKey("canvas")), includeUnchecked)
}

func (w *BlastWizard) selectedCanvasRowsInCurrentOrderForExport(items []model.CanvasItem, includeUnchecked bool) []canvasSelectedRow {
	return selectedCanvasRowsInVisibleOrderForExport(items, w.prompt.SnapshotCanvasReviewState(canvasStateKey("canvas")), includeUnchecked)
}

func selectedCanvasRowsInOrder(items []model.CanvasItem) []canvasSelectedRow {
	out := make([]canvasSelectedRow, 0)
	for itemIndex, item := range items {
		selected := normalizeCanvasSelection(item.Selected, len(item.Rows))
		for rowIndex, row := range item.Rows {
			if rowIndex >= len(selected) || !selected[rowIndex] {
				continue
			}
			out = append(out, canvasSelectedRow{
				ItemTitle: strings.TrimSpace(item.Title),
				ItemIndex: itemIndex,
				RowIndex:  rowIndex,
				Row:       row,
			})
		}
	}
	return out
}

func selectedCanvasRowsInOrderForExport(items []model.CanvasItem) []canvasSelectedRow {
	out := make([]canvasSelectedRow, 0)
	for _, selected := range selectedCanvasRowsInOrder(items) {
		if canvasRowHasSequenceForExport(selected.Row) {
			out = append(out, selected)
		}
	}
	return out
}

func selectedCanvasRowsInVisibleOrder(items []model.CanvasItem, reviewState tui.BlastRunSelectionState, includeUnchecked bool) []canvasSelectedRow {
	out := make([]canvasSelectedRow, 0)
	for itemIndex, item := range items {
		selected := normalizeCanvasSelection(item.Selected, len(item.Rows))
		order := prompt.CanvasVisibleRowOrder(item, reviewState.Sort)
		if len(order) != len(item.Rows) {
			order = make([]int, len(item.Rows))
			for i := range order {
				order[i] = i
			}
		}
		for _, rowIndex := range order {
			if rowIndex < 0 || rowIndex >= len(item.Rows) {
				continue
			}
			row := item.Rows[rowIndex]
			if !includeUnchecked && (rowIndex >= len(selected) || !selected[rowIndex]) {
				continue
			}
			out = append(out, canvasSelectedRow{
				ItemTitle: strings.TrimSpace(item.Title),
				ItemIndex: itemIndex,
				RowIndex:  rowIndex,
				Row:       row,
			})
		}
	}
	return out
}

func selectedCanvasRowsInVisibleOrderForExport(items []model.CanvasItem, reviewState tui.BlastRunSelectionState, includeUnchecked bool) []canvasSelectedRow {
	rows := selectedCanvasRowsInVisibleOrder(items, reviewState, includeUnchecked)
	out := make([]canvasSelectedRow, 0, len(rows))
	for _, selected := range rows {
		if canvasRowHasSequenceForExport(selected.Row) {
			out = append(out, selected)
		}
	}
	return out
}

func canvasRowHasSequenceForExport(row model.CanvasRow) bool {
	switch row.Kind {
	case model.CanvasKindFasta:
		return row.FASTA != nil && strings.TrimSpace(firstNonEmpty(row.FASTA.Sequence, row.FASTA.ProteinSequence, row.FASTA.NucleotideSequence)) != ""
	case model.CanvasKindKeyword:
		return row.KeywordRow != nil && (strings.TrimSpace(canvasRowStoredSequence(row)) != "" || strings.TrimSpace(row.KeywordRow.SequenceID) != "")
	case model.CanvasKindBlast:
		return row.BlastRow != nil && (strings.TrimSpace(canvasRowStoredSequence(row)) != "" || strings.TrimSpace(firstNonEmpty(row.BlastRow.SequenceID, row.BlastRow.TranscriptID, row.BlastRow.Protein)) != "")
	default:
		return false
	}
}

func (w *BlastWizard) canvasSequenceRecordsForExport(ctx context.Context, selectedRows []canvasSelectedRow) ([]model.ProteinSequenceRecord, error) {
	records := make([]model.ProteinSequenceRecord, 0, len(selectedRows))
	for _, item := range selectedRows {
		record, err := w.canvasSequenceRecord(ctx, item)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (w *BlastWizard) canvasSequenceRecord(ctx context.Context, selected canvasSelectedRow) (model.ProteinSequenceRecord, error) {
	row := selected.Row
	switch row.Kind {
	case model.CanvasKindFasta:
		if row.FASTA == nil {
			return model.ProteinSequenceRecord{}, fmt.Errorf("canvas FASTA row %d has no sequence payload", selected.RowIndex+1)
		}
		header := canvasOriginalHeader(*row.FASTA)
		return model.ProteinSequenceRecord{
			Header:         header,
			OriginalHeader: header,
			SourceKey:      querySequenceRecordSourceKey(row.FASTA),
			Sequence:       sanitizeSequence(firstNonEmpty(row.FASTA.Sequence, row.FASTA.ProteinSequence, row.FASTA.NucleotideSequence)),
		}, nil
	case model.CanvasKindKeyword:
		if row.KeywordRow == nil {
			return model.ProteinSequenceRecord{}, fmt.Errorf("canvas keyword row %d is unavailable", selected.RowIndex+1)
		}
		if sequence := strings.TrimSpace(canvasRowStoredSequence(row)); sequence != "" {
			header := keywordProteinSequenceHeader(*row.KeywordRow)
			originalHeader := header
			if row.SequenceData != nil && strings.TrimSpace(row.SequenceData.OriginalHeader) != "" {
				originalHeader = strings.TrimSpace(row.SequenceData.OriginalHeader)
			}
			return model.ProteinSequenceRecord{
				Header:         header,
				OriginalHeader: originalHeader,
				SourceKey:      keywordSequenceRecordSourceKey(*row.KeywordRow),
				Sequence:       sanitizeSequence(sequence),
			}, nil
		}
		targetID := keywordSequenceFetchTargetID(w.source, w.lastKeywordSpecies)
		sequenceID := strings.TrimSpace(row.KeywordRow.SequenceID)
		if sequenceID == "" {
			return model.ProteinSequenceRecord{}, fmt.Errorf("canvas keyword row %d is missing sequence id", selected.RowIndex+1)
		}
		record, err := w.fetchProteinSequenceCached(ctx, targetID, sequenceID)
		if err != nil {
			return model.ProteinSequenceRecord{}, err
		}
		originalHeader := strings.TrimSpace(record.OriginalHeader)
		if originalHeader == "" {
			originalHeader = keywordProteinSequenceHeader(*row.KeywordRow)
		}
		return model.ProteinSequenceRecord{
			Header:         keywordProteinSequenceHeader(*row.KeywordRow),
			OriginalHeader: originalHeader,
			SourceKey:      keywordSequenceRecordSourceKey(*row.KeywordRow),
			Sequence:       sanitizeSequence(record.Sequence),
		}, nil
	case model.CanvasKindBlast:
		if row.BlastRow == nil {
			return model.ProteinSequenceRecord{}, fmt.Errorf("canvas BLAST row %d is unavailable", selected.RowIndex+1)
		}
		if sequence := strings.TrimSpace(canvasRowStoredSequence(row)); sequence != "" {
			header := blastProteinSequenceHeader(*row.BlastRow)
			originalHeader := header
			if row.SequenceData != nil && strings.TrimSpace(row.SequenceData.OriginalHeader) != "" {
				originalHeader = strings.TrimSpace(row.SequenceData.OriginalHeader)
			}
			return model.ProteinSequenceRecord{
				Header:         header,
				OriginalHeader: originalHeader,
				SourceKey:      blastSequenceRecordSourceKey(*row.BlastRow),
				Sequence:       sanitizeSequence(sequence),
			}, nil
		}
		sequenceID := strings.TrimSpace(firstNonEmpty(row.BlastRow.SequenceID, row.BlastRow.TranscriptID, row.BlastRow.Protein))
		if sequenceID == "" {
			return model.ProteinSequenceRecord{}, fmt.Errorf("canvas BLAST row %d is missing sequence id", selected.RowIndex+1)
		}
		targetID := row.BlastRow.TargetID
		if targetID == 0 {
			targetID = w.phytozomeTargetIDForRow(ctx, *row.BlastRow)
		}
		record, err := w.fetchProteinSequenceCached(ctx, targetID, sequenceID)
		if err != nil {
			return model.ProteinSequenceRecord{}, err
		}
		originalHeader := strings.TrimSpace(record.OriginalHeader)
		if originalHeader == "" {
			originalHeader = blastProteinSequenceHeader(*row.BlastRow)
		}
		return model.ProteinSequenceRecord{
			Header:         blastProteinSequenceHeader(*row.BlastRow),
			OriginalHeader: originalHeader,
			SourceKey:      blastSequenceRecordSourceKey(*row.BlastRow),
			Sequence:       sanitizeSequence(record.Sequence),
		}, nil
	default:
		return model.ProteinSequenceRecord{}, fmt.Errorf("canvas row %d cannot be exported as FASTA", selected.RowIndex+1)
	}
}

func canvasOriginalHeader(source model.QuerySequenceSource) string {
	header := strings.TrimSpace(source.Annotation)
	if header == "" {
		header = buildQuerySequenceHeaderID(&source)
	}
	if header == "" {
		header = "sequence"
	}
	if !strings.HasPrefix(header, ">") {
		header = ">" + header
	}
	return header
}

func applyCanvasHeaderMode(records []model.ProteinSequenceRecord, rows []canvasSelectedRow, mode model.FastaHeaderMode) []model.ProteinSequenceRecord {
	switch model.NormalizeFastaHeaderMode(mode, true) {
	case model.FastaHeaderModePhgoLite:
		return applyCanvasPhgoLiteHeaders(records, rows)
	case model.FastaHeaderModeOriginal:
		return applyOriginalHeaders(records)
	case model.FastaHeaderModeMinimal:
		return applyCanvasMinimalHeaders(records, rows)
	case model.FastaHeaderModeDisplayName:
		return applyCanvasDisplayNameHeaders(records, rows)
	default:
		return applyCanvasPhgoHeaders(records, rows)
	}
}

func applyCanvasPhgoLiteHeaders(records []model.ProteinSequenceRecord, rows []canvasSelectedRow) []model.ProteinSequenceRecord {
	out := append([]model.ProteinSequenceRecord(nil), records...)
	limit := minInt(len(out), len(rows))
	for i := 0; i < limit; i++ {
		species, label, geneID := canvasPhgoSelfParts(rows[i], out[i])
		if header := buildPhgoLiteHeader(species, geneID, label); header != "" {
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

func applyCanvasPhgoHeaders(records []model.ProteinSequenceRecord, rows []canvasSelectedRow) []model.ProteinSequenceRecord {
	out := append([]model.ProteinSequenceRecord(nil), records...)
	limit := minInt(len(out), len(rows))
	for i := 0; i < limit; i++ {
		if header := canvasPhgoHeader(rows[i], out[i]); header != "" {
			out[i].Header = header
			continue
		}
		header := strings.TrimSpace(out[i].OriginalHeader)
		if header == "" {
			header = strings.TrimSpace(out[i].Header)
		}
		out[i].Header = header
	}
	return out
}

func applyCanvasMinimalHeaders(records []model.ProteinSequenceRecord, rows []canvasSelectedRow) []model.ProteinSequenceRecord {
	out := append([]model.ProteinSequenceRecord(nil), records...)
	limit := minInt(len(out), len(rows))
	for i := 0; i < limit; i++ {
		if header := minimalFastaHeader(canvasMinimalHeaderID(rows[i], out[i])); header != "" {
			out[i].Header = header
			continue
		}
		if header := minimalFastaHeader(recordMinimalHeaderID(out[i])); header != "" {
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

func applyCanvasDisplayNameHeaders(records []model.ProteinSequenceRecord, rows []canvasSelectedRow) []model.ProteinSequenceRecord {
	out := append([]model.ProteinSequenceRecord(nil), records...)
	limit := minInt(len(out), len(rows))
	for i := 0; i < limit; i++ {
		if header := displayNameFastaHeader(canvasRowDisplayName(rows[i].Row, rows[i].ItemTitle)); header != "" {
			out[i].Header = header
			continue
		}
		if header := minimalFastaHeader(recordMinimalHeaderID(out[i])); header != "" {
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

func displayNameFastaHeader(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(name, "\r", " "), "\n", " "))
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return ""
	}
	return ">" + name
}

func canvasPhgoHeader(selected canvasSelectedRow, record model.ProteinSequenceRecord) string {
	species, label, geneID := canvasPhgoSelfParts(selected, record)
	sourceLabel, sourceID := canvasPhgoSourceParts(selected, record)
	rawRow := selected.Row.RowNumber
	if rawRow == 0 {
		rawRow = selected.RowIndex + 1
	}
	itemTitle := strings.TrimSpace(selected.ItemTitle)
	if itemTitle == "" {
		itemTitle = "canvas"
	}
	displayName := canvasRowDisplayName(selected.Row, selected.ItemTitle)
	if displayName == "" {
		displayName = recordMinimalHeaderID(record)
	}
	return buildCanvasPhgoHeader(species, label, geneID, sourceLabel, sourceID, rawRow, itemTitle, displayName)
}

func buildCanvasPhgoHeader(species string, label string, geneID string, sourceLabel string, sourceID string, rawRow int, itemTitle string, displayName string) string {
	species = phgoHeaderPartOrPlaceholder(sanitizePhgoHeaderPart(species))
	label = phgoHeaderPartOrPlaceholder(sanitizePhgoHeaderPart(label))
	geneID = phgoHeaderPartOrPlaceholder(sanitizePhgoHeaderPart(geneID))
	sourceLabel = phgoHeaderPartOrPlaceholder(sanitizePhgoHeaderPart(sourceLabel))
	sourceID = phgoHeaderPartOrPlaceholder(sanitizePhgoHeaderPart(sourceID))
	itemTitle = phgoHeaderPartOrPlaceholder(sanitizePhgoHeaderPart(itemTitle))
	displayName = phgoHeaderPartOrPlaceholder(sanitizePhgoHeaderPart(displayName))
	return buildPhgoHeaderWithGroups(
		species,
		label,
		geneID,
		sourceLabel+"/"+sourceID,
		fmt.Sprintf("%d/%s", rawRow, itemTitle),
		displayName,
	)
}

func canvasPhgoSelfParts(selected canvasSelectedRow, record model.ProteinSequenceRecord) (string, string, string) {
	switch selected.Row.Kind {
	case model.CanvasKindKeyword:
		if selected.Row.KeywordRow != nil {
			row := *selected.Row.KeywordRow
			return firstNonEmpty(strings.TrimSpace(row.SequenceHeaderLabel), strings.TrimSpace(row.Genome)),
				rowKeywordLabelName(row),
				firstNonEmpty(strings.TrimSpace(row.TranscriptID), stripTranscriptDecorations(strings.TrimSpace(row.GeneIdentifier)), strings.TrimSpace(row.SequenceID), strings.TrimSpace(row.ProteinID))
		}
	case model.CanvasKindBlast:
		if selected.Row.BlastRow != nil {
			row := *selected.Row.BlastRow
			return strings.TrimSpace(row.Species),
				firstNonEmpty(strings.TrimSpace(row.LabelName), strings.TrimSpace(row.BlastLabelName)),
				blastRowID2(row)
		}
	case model.CanvasKindFasta:
		if selected.Row.FASTA != nil {
			source := *selected.Row.FASTA
			if parsed, ok := parsePhgoFastaHeader(source.Annotation); ok {
				return parsed.Species, parsed.LabelName, parsed.GeneID
			}
			return strings.TrimSpace(source.OrganismShort),
				strings.TrimSpace(source.LabelName),
				strings.TrimSpace(firstNonEmpty(source.TranscriptID, source.GeneID, source.ProteinID, source.PreferredSequenceID, canvasRecordFallbackID(record)))
		}
	}
	return "", "", canvasRecordFallbackID(record)
}

func canvasPhgoSourceParts(selected canvasSelectedRow, record model.ProteinSequenceRecord) (string, string) {
	switch selected.Row.Kind {
	case model.CanvasKindKeyword:
		if selected.Row.KeywordRow != nil {
			return "", ""
		}
	case model.CanvasKindBlast:
		if selected.Row.BlastRow != nil {
			row := *selected.Row.BlastRow
			return strings.TrimSpace(row.BlastLabelName), strings.TrimSpace(row.BlastGeneID)
		}
	case model.CanvasKindFasta:
		if selected.Row.FASTA != nil {
			source := *selected.Row.FASTA
			if parsed, ok := parsePhgoFastaHeader(source.Annotation); ok {
				return parsed.BlastSourceLabelName, parsed.BlastSourceGeneID
			}
			return strings.TrimSpace(source.BlastSourceLabelName), strings.TrimSpace(source.BlastSourceGeneID)
		}
	}
	return "", ""
}

func canvasRecordFallbackID(record model.ProteinSequenceRecord) string {
	for _, header := range []string{record.OriginalHeader, record.Header} {
		if id := primaryIDFromFastaHeader(header); id != "" {
			return id
		}
	}
	return "sequence"
}

func canvasMinimalHeaderID(selected canvasSelectedRow, record model.ProteinSequenceRecord) string {
	switch selected.Row.Kind {
	case model.CanvasKindKeyword:
		if selected.Row.KeywordRow != nil {
			return keywordMinimalHeaderID(*selected.Row.KeywordRow, record)
		}
	case model.CanvasKindBlast:
		if selected.Row.BlastRow != nil {
			return blastRowID2(*selected.Row.BlastRow)
		}
	case model.CanvasKindFasta:
		if selected.Row.FASTA != nil {
			return querySourceID2(selected.Row.FASTA)
		}
	}
	return recordMinimalHeaderID(record)
}

func canvasStateKey(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "canvas"
	}
	return "canvas:" + title
}

func snapshotCanvasItems(items []model.CanvasItem) []sessionsnapshot.CanvasItemV1 {
	out := make([]sessionsnapshot.CanvasItemV1, len(items))
	for i := range items {
		out[i].Title = items[i].Title
		out[i].Subtitle = items[i].Subtitle
		out[i].Kind = items[i].Kind
		out[i].Selected = append([]bool(nil), items[i].Selected...)
		out[i].MSAFlags = append([]bool(nil), items[i].MSAFlags...)
		out[i].SourceLabel = items[i].SourceLabel
		out[i].ImportedFrom = items[i].ImportedFrom
		out[i].ActiveColumns = append([]model.CanvasColumn(nil), items[i].ActiveColumns...)
		out[i].Rows = make([]sessionsnapshot.CanvasRowV1, len(items[i].Rows))
		for j := range items[i].Rows {
			out[i].Rows[j].RowNumber = items[i].Rows[j].RowNumber
			out[i].Rows[j].Kind = items[i].Rows[j].Kind
			out[i].Rows[j].DisplayName = items[i].Rows[j].DisplayName
			out[i].Rows[j].DisplayNameLocked = items[i].Rows[j].DisplayNameLocked
			if items[i].Rows[j].SequenceData != nil {
				copySequence := *items[i].Rows[j].SequenceData
				out[i].Rows[j].SequenceData = &copySequence
			}
			if items[i].Rows[j].SequenceReady != nil {
				ready := *items[i].Rows[j].SequenceReady
				out[i].Rows[j].SequenceReady = &ready
			}
			if items[i].Rows[j].KeywordRow != nil {
				copyRow := *items[i].Rows[j].KeywordRow
				out[i].Rows[j].KeywordRow = &copyRow
			}
			if items[i].Rows[j].BlastRow != nil {
				copyRow := *items[i].Rows[j].BlastRow
				out[i].Rows[j].BlastRow = &copyRow
			}
			if items[i].Rows[j].FASTA != nil {
				copySource := *items[i].Rows[j].FASTA
				out[i].Rows[j].FASTA = &copySource
			}
		}
	}
	return out
}

func mustOutputDir() string {
	dir, err := appfs.OutputDir()
	if err != nil {
		return "."
	}
	return dir
}

func canvasItemFromBlastRows(title string, sourceLabel string, rows []model.BlastResultRow, selectedMask []bool, activeColumns []model.CanvasColumn) model.CanvasItem {
	itemRows := make([]model.CanvasRow, 0, len(rows))
	mask := selectedMask
	if len(mask) != len(rows) {
		mask = make([]bool, len(rows))
		for i := range mask {
			mask[i] = true
		}
	}
	for _, row := range rows {
		copyRow := row
		itemRows = append(itemRows, model.CanvasRow{Kind: model.CanvasKindBlast, BlastRow: &copyRow, SequenceReady: canvasBlastSequenceReady(row)})
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = strings.TrimSpace(sourceLabel)
	}
	if title == "" {
		title = "1"
	}
	selected := make([]bool, len(itemRows))
	return model.CanvasItem{
		Title:         title,
		Subtitle:      canvasSelectionSummary(len(itemRows), selected),
		Kind:          model.CanvasKindBlast,
		Rows:          assignCanvasRowNumbers(itemRows, 1),
		Selected:      selected,
		SourceLabel:   strings.TrimSpace(sourceLabel),
		ActiveColumns: append([]model.CanvasColumn(nil), activeColumns...),
	}
}

func canvasItemFromBlastRowsWithSource(title string, item blastQueryItem, rows []model.BlastResultRow, selectedMask []bool, activeColumns []model.CanvasColumn) model.CanvasItem {
	if len(rows) == 0 {
		return model.CanvasItem{}
	}
	expanded := make([]model.CanvasRow, 0, len(rows)*2)
	for _, source := range canvasBlastQuerySourceRows(item) {
		expanded = append(expanded, model.CanvasRow{Kind: model.CanvasKindFasta, FASTA: source})
	}
	for _, row := range rows {
		copyRow := row
		expanded = append(expanded, model.CanvasRow{Kind: model.CanvasKindBlast, BlastRow: &copyRow, SequenceReady: canvasBlastSequenceReady(row)})
	}
	hitMask := append([]bool(nil), selectedMask...)
	if len(hitMask) != len(rows) {
		hitMask = make([]bool, len(rows))
		for i := range hitMask {
			hitMask[i] = true
		}
	}
	finalMask := make([]bool, 0, len(expanded))
	if sourceCount := len(expanded) - len(rows); sourceCount > 0 {
		finalMask = append(finalMask, make([]bool, sourceCount)...)
	}
	finalMask = append(finalMask, hitMask...)
	return model.CanvasItem{
		Title:         firstNonEmpty(strings.TrimSpace(title), strings.TrimSpace(item.LabelName), "1"),
		Subtitle:      canvasSelectionSummary(len(expanded), finalMask),
		Kind:          model.CanvasKindBlast,
		Rows:          assignCanvasBlastSourceRowNumbers(expanded),
		Selected:      finalMask,
		SourceLabel:   strings.TrimSpace(item.LabelName),
		ActiveColumns: append([]model.CanvasColumn(nil), activeColumns...),
	}
}

func canvasBlastQuerySourceRows(item blastQueryItem) []*model.QuerySequenceSource {
	sources := item.FamilySources
	if len(sources) == 0 && item.QuerySource != nil {
		sources = []*model.QuerySequenceSource{item.QuerySource}
	}
	out := make([]*model.QuerySequenceSource, 0, len(sources))
	for _, source := range sources {
		clone := canvasBlastQuerySourceRow(source, item)
		if clone != nil {
			out = append(out, clone)
		}
	}
	return out
}

func canvasBlastQuerySourceRow(source *model.QuerySequenceSource, item blastQueryItem) *model.QuerySequenceSource {
	if source == nil {
		return nil
	}
	clone := cloneQuerySource(source)
	if clone == nil {
		return nil
	}
	clone.Sequence = strings.TrimSpace(firstNonEmpty(clone.Sequence, item.Sequence, item.ProteinSequence, item.NucleotideSequence))
	clone.ProteinSequence = strings.TrimSpace(firstNonEmpty(clone.ProteinSequence, item.ProteinSequence))
	clone.NucleotideSequence = strings.TrimSpace(firstNonEmpty(clone.NucleotideSequence, item.NucleotideSequence))
	if clone.SequenceKind == "" {
		clone.SequenceKind = detectSequenceKind(clone.Sequence)
	}
	clone.PhgoBlastQuerySource = true
	return clone
}

func canvasItemsFromKeywordSelection(groups []model.KeywordSearchGroup, selectedRows []model.KeywordResultRow, selectedByGroup [][]bool, activeColumns []model.CanvasColumn) []model.CanvasItem {
	if len(selectedRows) == 0 {
		return nil
	}
	itemRows := make([]model.CanvasRow, 0, len(selectedRows))
	for _, row := range selectedRows {
		copyRow := row
		itemRows = append(itemRows, model.CanvasRow{Kind: model.CanvasKindKeyword, KeywordRow: &copyRow, SequenceReady: canvasKeywordSequenceReady(row)})
	}
	selected := make([]bool, len(itemRows))
	return []model.CanvasItem{{
		Title:         "1",
		Subtitle:      canvasSelectionSummary(len(itemRows), selected),
		Kind:          model.CanvasKindKeyword,
		Rows:          assignCanvasRowNumbers(itemRows, 1),
		Selected:      selected,
		SourceLabel:   strings.TrimSpace(firstNonEmpty(selectedRows[0].LabelName, selectedRows[0].SearchTerm)),
		ActiveColumns: append([]model.CanvasColumn(nil), activeColumns...),
	}}
}

func canvasItemsFromBlastRuns(runs []blastQueryRun, selectedByRun [][]bool, activeColumns []model.CanvasColumn) []model.CanvasItem {
	items := make([]model.CanvasItem, 0, len(runs))
	for runIndex, run := range runs {
		mask := []bool(nil)
		if runIndex < len(selectedByRun) {
			mask = selectedByRun[runIndex]
		}
		selectedRows := make([]model.BlastResultRow, 0, len(run.Results.Rows))
		selectedMask := make([]bool, 0, len(run.Results.Rows))
		for rowIndex, row := range run.Results.Rows {
			if len(mask) > 0 {
				if rowIndex >= len(mask) || !mask[rowIndex] {
					continue
				}
			}
			selectedRows = append(selectedRows, row)
			selectedMask = append(selectedMask, true)
		}
		if len(selectedRows) == 0 {
			continue
		}
		title := canvasBlastRunSidebarTitle(runIndex, run.Item)
		if len(run.Item.FamilySources) > 0 || run.Item.QuerySource != nil {
			items = append(items, canvasItemFromBlastRowsWithSource(title, run.Item, selectedRows, selectedMask, activeColumns))
			continue
		}
		sourceLabel := strings.TrimSpace(firstNonEmpty(run.Item.LabelName, run.Item.RawInput))
		items = append(items, canvasItemFromBlastRows(title, sourceLabel, selectedRows, selectedMask, activeColumns))
	}
	return items
}

func canvasBlastRunSidebarTitle(index int, item blastQueryItem) string {
	values := []string{}
	if item.QuerySource != nil {
		values = append(values, item.QuerySource.ProteinID, item.QuerySource.TranscriptID, item.QuerySource.GeneID)
	}
	values = append(values, item.RawInput, item.LabelName)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			if subtitle := canvasBlastRunSidebarSubtitle(item); subtitle != "" && !strings.EqualFold(subtitle, value) {
				return value + "[" + subtitle + "]"
			}
			return value
		}
	}
	return fmt.Sprintf("query %d", index+1)
}

func canvasBlastRunSidebarSubtitle(item blastQueryItem) string {
	if label := strings.TrimSpace(item.FamilyName); label != "" {
		return trimCanvasSidebarBracketLabel(label)
	}
	if label := firstCanvasDisplayLine(item.MemberLabel); label != "" {
		return trimCanvasSidebarBracketLabel(label)
	}
	return trimCanvasSidebarBracketLabel(strings.TrimSpace(item.LabelName))
}

func firstCanvasDisplayLine(value string) string {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(value), "\r", ""), "\n")
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

func trimCanvasSidebarBracketLabel(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	}
	return value
}

func canvasItemActiveColumnsFromKeywordRows(rows []model.KeywordResultRow) []model.CanvasColumn {
	return canvasItemActiveColumns(prompt.KeywordDisplayColumnIDs(canvasKeywordRowsDatabase(rows)))
}

func canvasItemActiveColumnsFromBlastRows(rows []model.BlastResultRow) []model.CanvasColumn {
	return canvasItemActiveColumns(prompt.BlastDisplayColumnIDs(canvasBlastRowsDatabase(rows), canvasBlastRowsProgram(rows), canvasBlastRowsHaveUniProt(rows), canvasBlastRowsHaveInterPro(rows)))
}

func canvasItemActiveColumns(columnIDs []string) []model.CanvasColumn {
	options := prompt.ColumnDisplayOptions{Multiline: true}
	out := make([]model.CanvasColumn, 0, len(columnIDs))
	for _, id := range columnIDs {
		id = strings.TrimSpace(id)
		if id == "" || strings.EqualFold(id, "label_name") {
			continue
		}
		out = append(out, model.CanvasColumn{ID: id, Header: prompt.ColumnCompactHeader(id, options)})
	}
	return out
}

func canvasKeywordRowsDatabase(rows []model.KeywordResultRow) string {
	for _, row := range rows {
		if value := strings.TrimSpace(row.SourceDatabase); value != "" {
			return value
		}
	}
	return "phytozome"
}

func canvasBlastRowsDatabase(rows []model.BlastResultRow) string {
	for _, row := range rows {
		if value := strings.TrimSpace(row.SourceDatabase); value != "" {
			return value
		}
	}
	return "phytozome"
}

func canvasBlastRowsProgram(rows []model.BlastResultRow) string {
	for _, row := range rows {
		if value := strings.TrimSpace(row.BlastProgram); value != "" {
			return strings.ToUpper(value)
		}
	}
	return ""
}

func canvasBlastRowsHaveUniProt(rows []model.BlastResultRow) bool {
	for _, row := range rows {
		if row.UniProtReferenceEnabled || strings.TrimSpace(row.UniProtAccession) != "" || strings.TrimSpace(row.UniProtEntryName) != "" {
			return true
		}
	}
	return false
}

func canvasBlastRowsHaveInterPro(rows []model.BlastResultRow) bool {
	for _, row := range rows {
		if row.InterProReferenceEnabled || strings.TrimSpace(row.InterProConservedRegionStatus) != "" || strings.TrimSpace(row.InterProAccessions) != "" || strings.TrimSpace(row.InterProEntryName) != "" {
			return true
		}
	}
	return false
}

func canvasKeywordSequenceReady(row model.KeywordResultRow) *bool {
	if strings.TrimSpace(row.SequenceID) == "" && strings.TrimSpace(row.ProteinID) == "" && strings.TrimSpace(row.TranscriptID) == "" {
		return &canvasSequenceReadyFalse
	}
	return nil
}

func canvasBlastSequenceReady(row model.BlastResultRow) *bool {
	if strings.TrimSpace(firstNonEmpty(row.SequenceID, row.TranscriptID, row.Protein, row.SubjectID)) == "" {
		return &canvasSequenceReadyFalse
	}
	return nil
}

func markKeywordCanvasSequenceAvailability(items []model.CanvasItem, results map[string]sequenceFetchResult) {
	if len(results) == 0 {
		return
	}
	for itemIndex := range items {
		for rowIndex := range items[itemIndex].Rows {
			row := &items[itemIndex].Rows[rowIndex]
			if row.Kind != model.CanvasKindKeyword || row.KeywordRow == nil {
				continue
			}
			sequenceID := strings.TrimSpace(row.KeywordRow.SequenceID)
			if sequenceID == "" {
				row.SequenceReady = &canvasSequenceReadyFalse
				continue
			}
			if fetched, ok := results[sequenceID]; ok {
				if fetched.err != nil || strings.TrimSpace(fetched.data.Sequence) == "" {
					row.SequenceReady = &canvasSequenceReadyFalse
				} else {
					copyData := fetched.data
					row.SequenceData = &copyData
					row.SequenceReady = &canvasSequenceReadyTrue
				}
			}
		}
	}
}

func markBlastCanvasSequenceAvailability(items []model.CanvasItem, results map[string]sequenceFetchResult) {
	if len(results) == 0 {
		return
	}
	for itemIndex := range items {
		for rowIndex := range items[itemIndex].Rows {
			row := &items[itemIndex].Rows[rowIndex]
			if row.Kind != model.CanvasKindBlast || row.BlastRow == nil {
				continue
			}
			sequenceID := strings.TrimSpace(firstNonEmpty(row.BlastRow.SequenceID, row.BlastRow.TranscriptID, row.BlastRow.Protein))
			if sequenceID == "" {
				row.SequenceReady = &canvasSequenceReadyFalse
				continue
			}
			key := fmt.Sprintf("%d:%s", row.BlastRow.TargetID, sequenceID)
			if fetched, ok := results[key]; ok {
				if fetched.err != nil || strings.TrimSpace(fetched.data.Sequence) == "" {
					row.SequenceReady = &canvasSequenceReadyFalse
				} else {
					copyData := fetched.data
					row.SequenceData = &copyData
					row.SequenceReady = &canvasSequenceReadyTrue
				}
			}
		}
	}
}

func canvasKeywordRowKey(row model.KeywordResultRow, index int) string {
	return strings.Join([]string{
		strings.TrimSpace(row.SourceDatabase),
		strings.TrimSpace(row.SearchTerm),
		strings.TrimSpace(row.SequenceID),
		strings.TrimSpace(row.ProteinID),
		strings.TrimSpace(row.TranscriptID),
		strings.TrimSpace(row.GeneIdentifier),
		fmt.Sprintf("%d", index),
	}, "|")
}

func countTrueBools(values []bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func (w *BlastWizard) runKeywordRowsCanvasMode(ctx context.Context, selected model.SpeciesCandidate, groups []model.KeywordSearchGroup, rows []model.KeywordResultRow, selectedByGroup [][]bool) error {
	items := canvasItemsFromKeywordSelection(groups, rows, selectedByGroup, canvasItemActiveColumnsFromKeywordRows(rows))
	if len(items) == 0 {
		return nil
	}
	markKeywordCanvasSequenceAvailability(items, w.prefetchKeywordSequences(ctx, selected, rows, nil))
	return w.openCanvasChildOrInline(ctx, items, 0, nextCanvasNumericID(items))
}

func (w *BlastWizard) runBlastRowsCanvasMode(ctx context.Context, item blastQueryItem, rows []model.BlastResultRow) error {
	if len(rows) == 0 {
		return nil
	}
	canvasItem := canvasItemFromBlastRowsWithSource("1", item, rows, nil, canvasItemActiveColumnsFromBlastRows(rows))
	items := []model.CanvasItem{canvasItem}
	markBlastCanvasSequenceAvailability(items, w.prefetchBlastSequences(ctx, rows, nil))
	return w.openCanvasChildOrInline(ctx, items, 0, nextCanvasNumericID(items))
}

func (w *BlastWizard) runBlastRunsCanvasMode(ctx context.Context, runs []blastQueryRun, selectedByRun [][]bool) error {
	allRows := make([]model.BlastResultRow, 0)
	for _, run := range runs {
		allRows = append(allRows, run.Results.Rows...)
	}
	items := canvasItemsFromBlastRuns(runs, selectedByRun, canvasItemActiveColumnsFromBlastRows(allRows))
	if len(items) == 0 {
		return nil
	}
	rows := make([]model.BlastResultRow, 0)
	for _, item := range items {
		for _, row := range item.Rows {
			if row.BlastRow != nil {
				rows = append(rows, *row.BlastRow)
			}
		}
	}
	markBlastCanvasSequenceAvailability(items, w.prefetchBlastSequences(ctx, rows, nil))
	return w.openCanvasChildOrInline(ctx, items, 0, nextCanvasNumericID(items))
}

func (w *BlastWizard) openCanvasChildOrInline(ctx context.Context, items []model.CanvasItem, currentItem int, nextNumericID int) error {
	if w.shouldSpawnChildTab() {
		handoff := w.SnapshotHandoff("", ModeCanvas, "", w.instanceID, w.instanceRunID)
		handoff.StartupSource = "canvas-transfer"
		handoff.BlastContext.PendingMode = string(ModeCanvas)
		handoff.BlastContext.TransferKind = "canvas_items"
		handoff.BlastContext.TransferCanvasItems = cloneCanvasItems(items)
		handoff.BlastContext.TransferCanvasCurrent = currentItem
		handoff.BlastContext.TransferCanvasNextID = nextNumericID
		return w.spawnChildTab(ctx, "", ModeCanvas, handoff)
	}
	return w.runCanvasMode(ctx, canvasLaunchState{
		Items:         cloneCanvasItems(items),
		CurrentItem:   currentItem,
		NextNumericID: nextNumericID,
		SaveBaseName:  canvasDefaultSaveName(items),
	})
}

func cloneCanvasItems(items []model.CanvasItem) []model.CanvasItem {
	out := make([]model.CanvasItem, len(items))
	for i := range items {
		out[i] = model.CanvasItem{
			Title:         items[i].Title,
			Subtitle:      items[i].Subtitle,
			Kind:          items[i].Kind,
			Selected:      append([]bool(nil), items[i].Selected...),
			MSAFlags:      append([]bool(nil), items[i].MSAFlags...),
			SourceLabel:   items[i].SourceLabel,
			ImportedFrom:  items[i].ImportedFrom,
			ActiveColumns: append([]model.CanvasColumn(nil), items[i].ActiveColumns...),
			Rows:          make([]model.CanvasRow, len(items[i].Rows)),
		}
		for j := range items[i].Rows {
			row := items[i].Rows[j]
			out[i].Rows[j].RowNumber = row.RowNumber
			out[i].Rows[j].Kind = row.Kind
			out[i].Rows[j].DisplayName = row.DisplayName
			out[i].Rows[j].DisplayNameLocked = row.DisplayNameLocked
			if row.SequenceData != nil {
				copySequence := *row.SequenceData
				out[i].Rows[j].SequenceData = &copySequence
			}
			if row.SequenceReady != nil {
				ready := *row.SequenceReady
				out[i].Rows[j].SequenceReady = &ready
			}
			if row.KeywordRow != nil {
				copyRow := *row.KeywordRow
				if row.KeywordRow.ExtraColumns != nil {
					copyRow.ExtraColumns = make(map[string]string, len(row.KeywordRow.ExtraColumns))
					for key, value := range row.KeywordRow.ExtraColumns {
						copyRow.ExtraColumns[key] = value
					}
				}
				out[i].Rows[j].KeywordRow = &copyRow
			}
			if row.BlastRow != nil {
				copyRow := *row.BlastRow
				out[i].Rows[j].BlastRow = &copyRow
			}
			if row.FASTA != nil {
				copySource := *row.FASTA
				out[i].Rows[j].FASTA = &copySource
			}
		}
	}
	return out
}

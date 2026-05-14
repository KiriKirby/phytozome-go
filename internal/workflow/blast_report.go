// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package workflow

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/prompt"
	"github.com/KiriKirby/phytozome-go/internal/report"
	"github.com/KiriKirby/phytozome-go/internal/tui"
)

type blastReportExportContext struct {
	Selected          model.SpeciesCandidate
	Prepared          []blastQueryItem
	InputPrepared     []blastQueryItem
	Run               blastQueryRun
	Runs              []blastQueryRun
	ExportAll         bool
	SelectedRows      []bool
	Request           model.BlastRequest
	BlastProgram      string
	UseUniProt        bool
	UseInterPro       bool
	Rows              []model.BlastResultRow
	AllRows           []model.BlastResultRow
	RowNumbers        []int
	FilterFlags       []bool
	FilterSettings    model.BlastFilterSettings
	FilterApplied     bool
	FilterCleared     bool
	BaseName          string
	OutputDir         string
	Settings          exportSettings
	Files             exportFileResult
	ExportStarted     time.Time
	ReportGeneratedAt time.Time
}

func (w *BlastWizard) renderBlastReportForExport(ctx context.Context, exportCtx blastReportExportContext) (string, error) {
	return w.renderBlastReportWithFiles(ctx, exportCtx, []exportFileResult{exportCtx.Files})
}

func (w *BlastWizard) renderBlastBatchReport(ctx context.Context, selected model.SpeciesCandidate, prepared []blastQueryItem, inputPrepared []blastQueryItem, runs []blastQueryRun, files []exportFileResult, rowsByRun [][]model.BlastResultRow, rowNumbersByRun [][]int, filterFlagsByRun [][]bool, selectedByRun [][]bool, outputDir string, settings exportSettings, request model.BlastRequest, filterSettings model.BlastFilterSettings, filterApplied bool, filterCleared bool) (string, error) {
	allRowsByRun := blastRowsByRunForReport(runs)
	flatRows, flatSelectedRows, flatSelectedMask, flatFilterFlags := flattenBlastBatchRows(allRowsByRun, rowNumbersByRun, filterFlagsByRun, selectedByRun)
	if len(flatSelectedRows) == 0 && len(rowsByRun) > 0 {
		flatSelectedRows = flattenBlastSelectedRowsByRun(rowsByRun)
		flatSelectedMask = nil
	}
	exportCtx := blastReportExportContext{
		Selected:          selected,
		Prepared:          cloneBlastQueryItems(prepared),
		InputPrepared:     cloneBlastQueryItems(inputPrepared),
		Runs:              append([]blastQueryRun(nil), runs...),
		ExportAll:         true,
		SelectedRows:      flatSelectedMask,
		Request:           request,
		Rows:              flatRows,
		AllRows:           flatRows,
		FilterFlags:       flatFilterFlags,
		FilterSettings:    filterSettings,
		FilterApplied:     filterApplied || anyBoolWorkflow(flatFilterFlags),
		FilterCleared:     filterCleared,
		BlastProgram:      request.Program,
		UseUniProt:        blastRunsUseUniProt(runs),
		UseInterPro:       blastRunsUseInterPro(runs),
		BaseName:          reportBaseNameForExport(settings.BaseName, outputDir, "Export all"),
		OutputDir:         outputDir,
		Settings:          settings,
		Files:             exportFileResult{},
		RowNumbers:        flattenIntMatrix(rowNumbersByRun),
		ExportStarted:     time.Now(),
		ReportGeneratedAt: time.Now(),
	}
	if len(flatSelectedRows) > 0 {
		exportCtx.Rows = flatSelectedRows
	}
	exportCtx.SelectedRows = flatSelectedMask
	return w.renderBlastReportWithFiles(ctx, exportCtx, files)
}

func (w *BlastWizard) renderBlastReportWithFiles(ctx context.Context, exportCtx blastReportExportContext, files []exportFileResult) (string, error) {
	metadataStart := time.Now()
	inspector := report.NewGeneratedFileInspector()
	generatedFiles, err := inspectBlastGeneratedFilesList(ctx, files, inspector)
	if err != nil {
		return "", err
	}
	steps := make([]report.GenerationStep, 0, 8)
	for _, file := range files {
		steps = append(steps, file.Steps...)
	}
	steps = append(steps, keywordReportStep("Capture file metadata and hashes", metadataStart, time.Now(), "ok", fmt.Sprintf("%d generated data files inspected", len(generatedFiles))))

	reportGeneratedAt := exportCtx.ReportGeneratedAt
	if reportGeneratedAt.IsZero() {
		reportGeneratedAt = time.Now()
	}
	reportPath := filepath.Join(exportCtx.OutputDir, report.ReportFileNameForBase(exportCtx.BaseName, reportGeneratedAt))
	generatedFiles = append(generatedFiles, report.PlannedReportFile(reportPath, reportGeneratedAt))

	renderStart := time.Now()
	reportSteps := append([]report.GenerationStep(nil), steps...)
	reportSteps = append(reportSteps, keywordReportStep("Render BLAST report PDF", renderStart, renderStart, "ok", "this step writes the PDF currently being read; final PDF self-hash is intentionally not embedded"))
	exportCtx.ReportGeneratedAt = reportGeneratedAt
	exportCtx.Files.Steps = reportSteps
	exportCtx.Files.SequenceAudit = aggregateBlastSequenceAudit(files, exportCtx.Settings.WriteText)
	data := w.buildBlastReportData(exportCtx, generatedFiles)

	if w.suppressTaskModals {
		if err := report.RenderBlastPDF(reportPath, data); err != nil {
			return "", err
		}
		return reportPath, nil
	}

	err = tui.RunTaskPageContext(tui.TaskPage{
		Path:        w.tuiPath("Export", "BLAST data analysis report"),
		Title:       "Writing BLAST Data Analysis Report",
		Description: "Rendering the BLAST-mode PDF report from already collected export data.",
		Initial:     "Writing BLAST Data Analysis Report PDF...",
		CancelError: prompt.ErrBackToRowSelection,
	}, func(taskCtx context.Context, update func(string)) error {
		_ = taskCtx
		safeTaskUpdate(update)("Writing BLAST Data Analysis Report PDF...")
		return report.RenderBlastPDF(reportPath, data)
	})
	if err != nil {
		return "", err
	}
	return reportPath, nil
}

func (w *BlastWizard) buildBlastReportData(exportCtx blastReportExportContext, files []report.GeneratedFile) report.ReportData {
	selected := exportCtx.Selected
	if selected.DisplayLabel() == "" {
		selected = exportCtx.Request.Species
	}
	rows := exportCtx.Rows
	allRows := exportCtx.AllRows
	if len(allRows) == 0 {
		allRows = rows
	}
	runs := exportCtx.Runs
	if len(runs) == 0 && len(exportCtx.Run.Results.Rows) > 0 {
		runs = []blastQueryRun{exportCtx.Run}
	}
	selectedRows := rows
	if len(selectedRows) == 0 {
		selectedRows = allRows
	}
	return report.ReportData{
		Title:       "BLAST Data Analysis Report",
		Mode:        "blast",
		GeneratedAt: exportCtx.ReportGeneratedAt,
		Software:    w.reportSoftwareInfo(),
		UserSession: reportUserSession(exportCtx.OutputDir),
		System: report.SystemInfo{
			OS:           report.PlatformDisplayName(runtime.GOOS),
			OSVersion:    "not available in this run; no OS-specific probe was performed for report generation",
			Architecture: runtime.GOARCH,
			CPUCount:     runtime.NumCPU(),
			Memory:       "not available in this run; memory details are not currently captured by the workflow",
		},
		TimeWindow: report.TimeWindow{
			QueryStart:     time.Time{},
			SearchEnd:      blastRunsSearchEndedAt(runs, exportCtx.ReportGeneratedAt),
			ReviewStart:    time.Time{},
			ExportStart:    exportCtx.ExportStarted,
			ExportEnd:      exportCtx.ReportGeneratedAt,
			ReportRendered: exportCtx.ReportGeneratedAt,
		},
		Files: files,
		Blast: report.BlastReportData{
			Database:           databaseDisplayName(w.source.Name()),
			Species:            reportSpecies(selected, w.source.Name()),
			Execution:          blastExecutionReport(exportCtx.Request),
			Inputs:             blastInputTraces(firstBlastInputPrepared(exportCtx.InputPrepared, exportCtx.Prepared), exportCtx.Run.Item),
			Runs:               blastRunReports(runs, exportCtx.Run, rows),
			Selection:          blastSelectionStats(exportCtx.Prepared, runs, selectedRows, allRows, files, len(runs)),
			ExternalReferences: blastExternalReferences(allRows, exportCtx.Request),
			Family:             blastFamilyReportBatch(runs),
			Filter:             blastFilterReport(exportCtx.Prepared, runs, allRows, selectedRows, exportCtx.SelectedRows, exportCtx.FilterFlags, exportCtx.FilterSettings, exportCtx.FilterApplied, exportCtx.FilterCleared),
			Provenance:         blastProvenance(allRows),
			ColumnCompleteness: blastColumnCompleteness(selectedRows),
			QualityChecks:      blastQualityChecks(exportCtx.Prepared, runs, selectedRows, allRows, exportCtx.Settings, exportCtx.Files.SequenceAudit),
			Columns:            blastColumnLineage(allRows, sourceDatabaseForBlastRows(allRows), firstNonEmpty(exportCtx.BlastProgram, blastRowsProgramForReport(allRows)), exportCtx.UseUniProt || blastRowsHaveUniProt(allRows), exportCtx.UseInterPro || blastRowsHaveInterPro(allRows)),
			ExportSettings:     blastExportSettings(exportCtx.BaseName, exportCtx.OutputDir, exportCtx.Settings, exportCtx.RowNumbers, exportCtx.FilterFlags),
			GenerationSteps:    exportCtx.Files.Steps,
			Sequences:          exportCtx.Files.SequenceAudit,
		},
	}
}

func inspectBlastGeneratedFilesList(ctx context.Context, files []exportFileResult, inspector *report.GeneratedFileInspector) ([]report.GeneratedFile, error) {
	type fileSpec struct {
		path string
		typ  string
		role string
	}
	specs := make([]fileSpec, 0, len(files)*4)
	for _, fileSet := range files {
		fileSpecs := []fileSpec{
			{fileSet.ExcelPath, "selected BLAST Excel", "selected BLAST rows exported as the main review workbook"},
			{fileSet.RawExcelPath, "raw BLAST Excel", "all current BLAST rows exported for audit comparison"},
			{fileSet.RawTextPath, "raw BLAST peptide text", "all current BLAST peptide sequence records exported for audit comparison"},
			{fileSet.TextPath, "BLAST peptide text", "BLAST peptide sequence records already produced by text export"},
		}
		for _, spec := range fileSpecs {
			if strings.TrimSpace(spec.path) == "" {
				continue
			}
			specs = append(specs, spec)
		}
	}
	out := make([]report.GeneratedFile, 0, len(specs))
	if len(specs) == 0 {
		return out, nil
	}
	type uniqueSpec struct {
		path string
		typ  string
		role string
	}
	uniqueByPath := make(map[string]uniqueSpec, len(specs))
	uniqueSpecs := make([]uniqueSpec, 0, len(specs))
	for _, spec := range specs {
		key := normalizeGeneratedFileSpecPath(spec.path)
		if _, ok := uniqueByPath[key]; ok {
			continue
		}
		unique := uniqueSpec{path: spec.path, typ: spec.typ, role: spec.role}
		uniqueByPath[key] = unique
		uniqueSpecs = append(uniqueSpecs, unique)
	}
	type inspectResult struct {
		file report.GeneratedFile
		err  error
	}
	results := make([]inspectResult, len(uniqueSpecs))
	if err := runParallel(ctx, len(uniqueSpecs), clampWorkers(len(uniqueSpecs), defaultDiskWorkers()), func(_ context.Context, i int) error {
		spec := uniqueSpecs[i]
		file, err := inspector.Inspect(spec.path, spec.typ, spec.role, time.Now())
		results[i] = inspectResult{file: file, err: err}
		return err
	}); err != nil {
		return nil, err
	}
	fileByPath := make(map[string]report.GeneratedFile, len(uniqueSpecs))
	for i, result := range results {
		if result.err != nil {
			return nil, result.err
		}
		fileByPath[normalizeGeneratedFileSpecPath(uniqueSpecs[i].path)] = result.file
	}
	for _, spec := range specs {
		file := fileByPath[normalizeGeneratedFileSpecPath(spec.path)]
		file.Type = spec.typ
		file.Role = spec.role
		out = append(out, file)
	}
	return out, nil
}

func normalizeGeneratedFileSpecPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func flattenBlastBatchRows(rowsByRun [][]model.BlastResultRow, rowNumbersByRun [][]int, filterFlagsByRun [][]bool, selectedByRun [][]bool) ([]model.BlastResultRow, []model.BlastResultRow, []bool, []bool) {
	totalRows := 0
	selectedCount := 0
	for runIndex, rows := range rowsByRun {
		totalRows += len(rows)
		if runIndex < len(selectedByRun) {
			for rowIndex := range rows {
				if rowIndex < len(selectedByRun[runIndex]) && selectedByRun[runIndex][rowIndex] {
					selectedCount++
				}
			}
		}
	}
	allRows := make([]model.BlastResultRow, 0, totalRows)
	selectedRows := make([]model.BlastResultRow, 0, selectedCount)
	selectedMask := make([]bool, 0, totalRows)
	filterFlags := make([]bool, 0, totalRows)
	for runIndex, rows := range rowsByRun {
		for rowIndex, row := range rows {
			allRows = append(allRows, row)
			flagged := runIndex < len(filterFlagsByRun) && rowIndex < len(filterFlagsByRun[runIndex]) && filterFlagsByRun[runIndex][rowIndex]
			filterFlags = append(filterFlags, flagged)
			selected := runIndex < len(selectedByRun) && rowIndex < len(selectedByRun[runIndex]) && selectedByRun[runIndex][rowIndex]
			selectedMask = append(selectedMask, selected)
			if selected {
				selectedRows = append(selectedRows, row)
			}
		}
		_ = rowNumbersByRun
	}
	return allRows, selectedRows, selectedMask, filterFlags
}

func appendUniqueStrings(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.EqualFold(strings.TrimSpace(existing), value) {
			return values
		}
	}
	return append(values, value)
}

func flattenIntMatrix(values [][]int) []int {
	total := 0
	for _, row := range values {
		total += len(row)
	}
	out := make([]int, 0, total)
	for _, row := range values {
		out = append(out, row...)
	}
	return out
}

func blastRowsByRunForReport(runs []blastQueryRun) [][]model.BlastResultRow {
	out := make([][]model.BlastResultRow, 0, len(runs))
	for _, run := range runs {
		out = append(out, run.Results.Rows)
	}
	return out
}

func flattenBlastSelectedRowsByRun(rowsByRun [][]model.BlastResultRow) []model.BlastResultRow {
	total := 0
	for _, rows := range rowsByRun {
		total += len(rows)
	}
	out := make([]model.BlastResultRow, 0, total)
	for _, rows := range rowsByRun {
		out = append(out, rows...)
	}
	return out
}

func aggregateBlastSequenceAudit(files []exportFileResult, requested bool) report.SequenceAudit {
	audit := report.SequenceAudit{Requested: requested}
	if !requested {
		return audit
	}
	order := make([]string, 0)
	byLabel := map[string]*report.SequenceQuerySummary{}
	for _, files := range files {
		seq := files.SequenceAudit
		if seq.Requested {
			audit.Requested = true
		}
		audit.RequestedCount += seq.RequestedCount
		audit.WrittenCount += seq.WrittenCount
		audit.SkippedCount += seq.SkippedCount
		audit.TotalCharacters += seq.TotalCharacters
		audit.Records = append(audit.Records, seq.Records...)
		if audit.TextFileType == "" && seq.TextFileType != "" {
			audit.TextFileType = seq.TextFileType
		}
		if audit.HeaderLabelMode == "" && seq.HeaderLabelMode != "" {
			audit.HeaderLabelMode = seq.HeaderLabelMode
		}
		for _, summary := range seq.QuerySummaries {
			label := firstNonEmpty(strings.TrimSpace(summary.QueryLabel), "sequence export")
			existing := byLabel[label]
			if existing == nil {
				copySummary := summary
				copySummary.QueryLabel = label
				byLabel[label] = &copySummary
				order = append(order, label)
				continue
			}
			existing.RequestedCount += summary.RequestedCount
			existing.WrittenCount += summary.WrittenCount
			existing.SkippedCount += summary.SkippedCount
			existing.TotalLength += summary.TotalLength
			if existing.QueryKind == "" {
				existing.QueryKind = summary.QueryKind
			}
			if existing.SourceSummary == "" {
				existing.SourceSummary = summary.SourceSummary
			}
			if existing.MinLength == 0 || (summary.MinLength > 0 && summary.MinLength < existing.MinLength) {
				existing.MinLength = summary.MinLength
			}
			if summary.MaxLength > existing.MaxLength {
				existing.MaxLength = summary.MaxLength
			}
		}
	}
	audit.QuerySummaries = make([]report.SequenceQuerySummary, 0, len(order))
	for _, label := range order {
		summary := *byLabel[label]
		if summary.WrittenCount > 0 {
			summary.AverageLength = summary.TotalLength / summary.WrittenCount
		}
		audit.QuerySummaries = append(audit.QuerySummaries, summary)
	}
	return audit
}

func blastExecutionReport(request model.BlastRequest) report.BlastExecutionReport {
	return report.BlastExecutionReport{
		Program:          firstNonEmpty(request.Program, "not available in this run"),
		ExecutionMode:    blastExecutionMode(request),
		QueryKind:        string(request.SequenceKind),
		TargetType:       firstNonEmpty(request.TargetType, "not available in this run"),
		EValue:           firstNonEmpty(request.EValue, "not available in this run"),
		ComparisonMatrix: firstNonEmpty(request.ComparisonMatrix, "not available in this run"),
		WordLength:       firstNonEmpty(request.WordLength, "not available in this run"),
		AlignmentsToShow: request.AlignmentsToShow,
		AllowGaps:        fmt.Sprintf("%t", request.AllowGaps),
		FilterQuery:      fmt.Sprintf("%t", request.FilterQuery),
		Notes:            "Request values were copied from the configured BLAST request already used by the workflow; the report did not rerun or probe BLAST.",
	}
}

func blastExecutionMode(request model.BlastRequest) string {
	if isLocalBlastRequest(request) {
		return "local BLAST+"
	}
	return "server BLAST"
}

func blastInputTraces(prepared []blastQueryItem, fallback blastQueryItem) []report.BlastInputTrace {
	items := prepared
	if len(items) == 0 && strings.TrimSpace(fallback.RawInput) != "" {
		items = []blastQueryItem{fallback}
	}
	out := make([]report.BlastInputTrace, 0, len(items))
	for i, item := range items {
		src := item.QuerySource
		headerPreview := blastInputRawPreview(item)
		trace := report.BlastInputTrace{
			Order:          i + 1,
			RawPreview:     headerPreview,
			InputType:      classifyBlastInputType(item),
			ParserPath:     blastParserPath(item),
			Source:         blastInputSource(item),
			SequenceLength: len(item.Sequence),
			LabelName:      firstNonEmpty(item.LabelName, item.MemberLabel),
			Outcome:        blastInputOutcome(item),
			Notes:          "Input trace was reconstructed from resolved query items already held by the workflow.",
		}
		if src != nil {
			trace.SequenceLength = len(src.Sequence)
			trace.GeneID = src.GeneID
			trace.TranscriptID = src.TranscriptID
			trace.ProteinID = src.ProteinID
			trace.OriginalURL = src.OriginalInputURL
			trace.NormalizedURL = src.NormalizedURL
			trace.Source = firstNonEmpty(src.SourceDatabase, trace.Source)
		}
		out = append(out, trace)
	}
	return out
}

func classifyBlastInputType(item blastQueryItem) string {
	raw := strings.TrimSpace(item.RawInput)
	lower := strings.ToLower(raw)
	switch {
	case item.QuerySource != nil && strings.EqualFold(item.QuerySource.SourceDatabase, "fasta"):
		return "FASTA record"
	case strings.HasPrefix(raw, ">"):
		return "FASTA record"
	case raw == "":
		return "resolved query item"
	case strings.Contains(lower, "/report/") || strings.Contains(lower, "phytozome-next.jgi.doe.gov"):
		return "report URL"
	case looksLikeSequence(raw):
		return "plain sequence"
	default:
		return "mixed or identifier-derived input"
	}
}

func blastParserPath(item blastQueryItem) string {
	switch classifyBlastInputType(item) {
	case "FASTA record":
		return "FASTA header -> sequence sanitize -> optional label"
	case "report URL":
		return "URL normalize -> report sequence resolver -> query sequence"
	case "plain sequence":
		return "inline sequence -> sequence sanitize"
	default:
		return "input parser -> resolved query item"
	}
}

func blastInputRawPreview(item blastQueryItem) string {
	raw := strings.TrimSpace(item.RawInput)
	if strings.HasPrefix(raw, ">") {
		if header := firstFastaHeaderLine(raw); strings.TrimSpace(header) != "" {
			return strings.TrimSpace(header)
		}
	}
	if item.QuerySource != nil && strings.EqualFold(item.QuerySource.SourceDatabase, "fasta") {
		if annotation := strings.TrimSpace(item.QuerySource.Annotation); annotation != "" {
			if !strings.HasPrefix(annotation, ">") {
				annotation = ">" + annotation
			}
			return annotation
		}
	}
	return firstNonEmpty(raw, item.MemberLabel, item.LabelName)
}

func firstBlastInputPrepared(primary []blastQueryItem, fallback []blastQueryItem) []blastQueryItem {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func reportBaseNameForExport(baseName string, outputDir string, fallback string) string {
	if strings.TrimSpace(baseName) != "" {
		return baseName
	}
	if dir := strings.TrimSpace(filepath.Base(outputDir)); dir != "" && dir != "." && dir != string(filepath.Separator) {
		return dir
	}
	return fallback
}

func blastInputSource(item blastQueryItem) string {
	if item.QuerySource != nil {
		return firstNonEmpty(item.QuerySource.SourceDatabase, "resolved query source")
	}
	return "user input"
}

func blastInputOutcome(item blastQueryItem) string {
	if strings.TrimSpace(item.Sequence) != "" || (item.QuerySource != nil && strings.TrimSpace(item.QuerySource.Sequence) != "") {
		return "resolved"
	}
	return "not available in this run"
}

func looksLikeSequence(value string) bool {
	cleaned := sanitizeSequence(value)
	return len(cleaned) >= 20
}

func blastRunReports(runs []blastQueryRun, fallback blastQueryRun, selectedRows []model.BlastResultRow) []report.BlastRunReport {
	if len(runs) == 0 && len(fallback.Results.Rows) > 0 {
		runs = []blastQueryRun{fallback}
	}
	out := make([]report.BlastRunReport, 0, len(runs))
	for _, run := range runs {
		rows := run.Results.Rows
		selected := len(run.SelectedRows)
		if selected == 0 && run.Index == fallback.Index {
			selected = len(selectedRows)
		}
		out = append(out, report.BlastRunReport{
			RunIndex:      run.Index,
			Label:         firstNonEmpty(run.Item.MemberLabel, run.Item.LabelName, reportQueryLabel(run.Item)),
			FamilyName:    run.Item.FamilyName,
			Program:       firstNonEmpty(run.Request.Program, fallback.Request.Program),
			ExecutionMode: blastExecutionMode(run.Request),
			JobID:         run.Results.JobID,
			RowCount:      len(rows),
			SelectedRows:  selected,
			TopHit:        blastTopHit(rows),
			BestEValue:    blastBestEValue(rows),
			BestIdentity:  blastBestIdentity(rows),
			Message:       run.Results.Message,
			ResultHash:    run.Results.Hash,
			ZUID:          run.Results.ZUID,
		})
	}
	return out
}

func blastSelectionStats(prepared []blastQueryItem, runs []blastQueryRun, selectedRows []model.BlastResultRow, allRows []model.BlastResultRow, files []report.GeneratedFile, exportedRuns int) report.BlastSelectionStats {
	parsed := len(prepared)
	resolved := 0
	for _, item := range prepared {
		if blastInputOutcome(item) == "resolved" {
			resolved++
		}
	}
	totalRuns := 0
	zeroHits := 0
	for _, run := range runs {
		if strings.TrimSpace(run.Item.FamilyName) == "" {
			totalRuns++
		}
		if len(run.Results.Rows) == 0 {
			zeroHits++
		}
	}
	if totalRuns == 0 {
		totalRuns = len(runs)
	}
	total := len(allRows)
	return report.BlastSelectionStats{
		ParsedQueries:   parsed,
		ResolvedQueries: resolved,
		ExecutedRuns:    totalRuns,
		ExportedRuns:    maxInt(1, exportedRuns),
		ZeroHitRuns:     zeroHits,
		TotalRows:       total,
		SelectedRows:    len(selectedRows),
		UnselectedRows:  maxInt(0, total-len(selectedRows)),
		RowsWithURL:     countBlastRowsWhere(allRows, func(row model.BlastResultRow) bool { return strings.TrimSpace(row.GeneReportURL) != "" }),
		RowsWithSequence: countBlastRowsWhere(allRows, func(row model.BlastResultRow) bool {
			return strings.TrimSpace(row.SequenceID) != "" || strings.TrimSpace(row.Protein) != "" || strings.TrimSpace(row.SubjectID) != ""
		}),
		RowsWithTargetLen: countBlastRowsWhere(allRows, func(row model.BlastResultRow) bool { return row.TargetLength > 0 }),
		RowsWithCoverage:  countBlastRowsWhere(allRows, func(row model.BlastResultRow) bool { return row.AlignQueryLengthPercent > 0 }),
		RowsWithEValue:    countBlastRowsWhere(allRows, func(row model.BlastResultRow) bool { return strings.TrimSpace(row.EValue) != "" }),
		RowsWithIdentity:  countBlastRowsWhere(allRows, func(row model.BlastResultRow) bool { return row.PercentIdentity > 0 }),
		GeneratedFiles:    len(files),
	}
}

func blastExternalReferences(rows []model.BlastResultRow, request model.BlastRequest) report.ExternalReferenceReport {
	uni := blastRowsHaveUniProt(rows)
	ip := blastRowsHaveInterPro(rows)
	return report.ExternalReferenceReport{
		UniProtEnabled:  uni,
		InterProEnabled: ip,
		UniProt:         blastUniProtReport(rows),
		InterPro:        blastInterProReport(rows),
	}
}

func blastRowsHaveUniProt(rows []model.BlastResultRow) bool {
	for _, row := range rows {
		if row.UniProtReferenceEnabled {
			return true
		}
	}
	return false
}

func blastRowsHaveInterPro(rows []model.BlastResultRow) bool {
	for _, row := range rows {
		if row.InterProReferenceEnabled {
			return true
		}
	}
	return false
}

func blastUniProtReport(rows []model.BlastResultRow) report.UniProtReferenceReport {
	return report.UniProtReferenceReport{
		LookupSummary: []report.NameValue{
			{Name: "Lookup source", Value: "workflow enrichment state", Explanation: "Rows were already enriched before export; the report did not call UniProt."},
			{Name: "Rows considered", Value: strconv.Itoa(len(rows)), Explanation: "Current BLAST rows available to the export."},
			{Name: "Unique accessions", Value: strconv.Itoa(len(uniqueBlastValues(rows, func(row model.BlastResultRow) string { return row.UniProtAccession }))), Explanation: "Distinct non-empty UniProt accessions present in rows."},
			{Name: "Lookup inputs", Value: "accession, protein, subject_id, sequence_id, transcript_id, defline", Explanation: "These are the identifiers the workflow already had available when it performed the real enrichment step."},
			{Name: "Fallback behaviour", Value: "no report-only lookup", Explanation: "Missing values are left blank instead of calling UniProt again during PDF rendering."},
		},
		Outcome: []report.NameValue{
			{Name: "Rows with accession", Value: fmt.Sprintf("%d of %d", countBlastRowsWhere(rows, func(row model.BlastResultRow) bool { return strings.TrimSpace(row.UniProtAccession) != "" }), len(rows)), Explanation: "Rows carrying UniProt accession after normal enrichment."},
			{Name: "Reviewed entries", Value: fmt.Sprintf("%d", countBlastRowsWhere(rows, func(row model.BlastResultRow) bool {
				return strings.EqualFold(strings.TrimSpace(row.UniProtReviewed), "reviewed")
			})), Explanation: "Rows marked reviewed in UniProt."},
			{Name: "Canonical length ratio", Value: fmt.Sprintf("%d of %d", countBlastRowsWhere(rows, func(row model.BlastResultRow) bool {
				return strings.TrimSpace(row.TargetUniProtCanonicalLengthPercent) != ""
			}), len(rows)), Explanation: "Rows with target/UniProt canonical length comparison."},
			{Name: "Sequence caution present", Value: fmt.Sprintf("%d", countBlastRowsWhere(rows, func(row model.BlastResultRow) bool { return strings.TrimSpace(row.UniProtSequenceCaution) != "" })), Explanation: "Rows carrying caution notes from the already-enriched UniProt values."},
		},
		Rows: blastUniProtRows(rows),
	}
}

func blastUniProtRows(rows []model.BlastResultRow) []report.UniProtRowSummary {
	out := make([]report.UniProtRowSummary, 0, minInt(len(rows), 8))
	for i, row := range rows {
		if strings.TrimSpace(row.UniProtAccession) == "" && strings.TrimSpace(row.UniProtProteinName) == "" {
			continue
		}
		out = append(out, report.UniProtRowSummary{
			Row:            i + 1,
			Label:          row.LabelName,
			Family:         row.FamilyName,
			Target:         firstNonEmpty(row.Protein, row.SubjectID, row.SequenceID),
			Accession:      row.UniProtAccession,
			Reviewed:       row.UniProtReviewed,
			FamilySupport:  blastFilterFamilySupportSummary(row),
			FamilySemantic: blastFilterFamilySemanticSummary(row),
			LengthRatio:    row.TargetUniProtCanonicalLengthPercent,
			Fragment:       row.UniProtFragment,
			Caution:        row.UniProtSequenceCaution,
			Annotation:     firstNonEmpty(row.UniProtProteinName, row.UniProtFunction),
		})
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func blastInterProReport(rows []model.BlastResultRow) report.InterProReferenceReport {
	settings := model.DefaultInterProConservedRegionSettings()
	return report.InterProReferenceReport{
		Settings: interProSettingsReport(settings),
		Outcome: []report.NameValue{
			{Name: "Lookup source", Value: "workflow enrichment state", Explanation: "Rows were already enriched before export; the report did not call InterPro."},
			{Name: "Rows with InterPro entry", Value: fmt.Sprintf("%d of %d", countBlastRowsWhere(rows, func(row model.BlastResultRow) bool {
				return strings.TrimSpace(row.InterProAccessions) != "" || strings.TrimSpace(row.InterProEntryName) != ""
			}), len(rows)), Explanation: "Rows with InterPro evidence fields."},
			{Name: "Rows with conserved-region status", Value: fmt.Sprintf("%d of %d", countBlastRowsWhere(rows, func(row model.BlastResultRow) bool { return strings.TrimSpace(row.InterProConservedRegionStatus) != "" }), len(rows)), Explanation: "Rows with status calculated during normal enrichment."},
		},
		StatusCounts: blastInterProStatusCounts(rows),
	}
}

func interProSettingsReport(settings model.InterProConservedRegionSettings) []report.NameValue {
	return []report.NameValue{
		{Name: "Match Pfam IDs", Value: fmt.Sprintf("%t", settings.UsePfamAccession), Explanation: "Shared Pfam IDs contribute to the InterPro status label."},
		{Name: "Match InterPro IDs", Value: fmt.Sprintf("%t", settings.UseInterProAccession), Explanation: "Shared InterPro IDs contribute to the InterPro status label."},
		{Name: "Match member-database signature IDs", Value: fmt.Sprintf("%t", settings.UseSignatureAccession), Explanation: "Shared member-database signature IDs contribute supporting evidence."},
		{Name: "Require compatible entry type", Value: fmt.Sprintf("%t", settings.UseEntryType), Explanation: "Entry types such as domain, family, repeat, and site are checked for compatibility."},
		{Name: "Also compare entry names", Value: fmt.Sprintf("%t", settings.UseEntryName), Explanation: "Entry names are used as weak supporting evidence when enabled."},
		{Name: "Use coverage cutoffs", Value: fmt.Sprintf("%t", settings.UseCoverage), Explanation: "Coverage thresholds help decide present versus partial status."},
		{Name: "Use coordinate overlap evidence", Value: fmt.Sprintf("%t", settings.UseMatchRegions), Explanation: "InterPro match-region coordinates contribute supporting evidence."},
		{Name: "present coverage >= %", Value: fmt.Sprintf("%.0f", settings.PresentMinCoverage), Explanation: "Minimum matched coverage needed for present status."},
		{Name: "partial coverage >= %", Value: fmt.Sprintf("%.0f", settings.PartialMinCoverage), Explanation: "Minimum matched coverage needed for partial status."},
		{Name: "present evidence count >=", Value: strconv.Itoa(settings.PresentMinMatchedItems), Explanation: "Minimum number of matched conserved evidence items required for present status."},
		{Name: "partial evidence count >=", Value: strconv.Itoa(settings.PartialMinMatchedItems), Explanation: "Minimum number of matched conserved evidence items required for partial status."},
	}
}

func blastFamilyReportBatch(runs []blastQueryRun) *report.FamilyBlastReport {
	groups := map[string]*report.FamilyBlastGroupReport{}
	familySettingsCaptured := false
	var familySettings model.FamilyBlastSettings
	for _, run := range runs {
		family := strings.TrimSpace(run.Item.FamilyName)
		if family == "" && len(run.Item.FamilySources) == 0 {
			continue
		}
		if !familySettingsCaptured {
			familySettings = run.Item.FamilySettings
			familySettingsCaptured = true
		}
		if family == "" {
			family = firstNonEmpty(run.Item.LabelName, "Family BLAST group")
		}
		group := groups[family]
		if group == nil {
			groupSource := firstNonEmpty(strings.TrimSpace(run.Item.FamilyGroupSource), "automatic detection")
			detectionRule := firstNonEmpty(strings.TrimSpace(run.Item.FamilyDetectionRule), "family labels already computed by workflow")
			group = &report.FamilyBlastGroupReport{
				Name:           family,
				GroupSource:    groupSource,
				DetectionRule:  detectionRule,
				OutputBaseName: family,
			}
			groups[family] = group
		}
		group.OriginalRuns++
		before := run.RowsBeforeMerge
		after := run.RowsAfterMerge
		if before <= 0 {
			before = len(run.Results.Rows)
		}
		if after <= 0 {
			after = len(run.Results.Rows)
		}
		group.RowsBefore += before
		group.RowsAfter += after
		if label := firstNonEmpty(run.Item.MemberLabel, run.Item.LabelName); label != "" {
			group.MemberLabels = appendUniqueStrings(group.MemberLabels, label)
		}
		for _, src := range run.Item.FamilySources {
			if src == nil {
				continue
			}
			group.MemberLabels = appendUniqueStrings(group.MemberLabels, firstNonEmpty(src.LabelName, src.GeneID, src.TranscriptID, src.ProteinID))
		}
	}
	if len(groups) == 0 {
		return nil
	}
	out := make([]report.FamilyBlastGroupReport, 0, len(groups))
	for _, group := range groups {
		if len(group.MemberLabels) == 0 {
			group.MemberLabels = []string{group.Name}
		}
		out = append(out, *group)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	settingsRows := []report.NameValue{
		{Name: "Group related queries as one family result", Value: "true", Explanation: "Family BLAST grouping state was present in the exported run."},
		{Name: "Used custom group editor", Value: fmt.Sprintf("%t", familySettings.CustomizeGroups), Explanation: "Whether the proposed groups were opened in the editor and confirmed or changed before running."},
		{Name: "Detect families from query names automatically", Value: fmt.Sprintf("%t", familySettings.GroupByDetectedPrefix), Explanation: "Shared query-name prefixes and source aliases were used to propose family groups."},
		{Name: "Merge rows that hit the same target gene/protein", Value: fmt.Sprintf("%t", familySettings.MergeRowsByTarget), Explanation: "Rows hitting the same normalized target inside a family are merged into one review/export row."},
		{Name: "When merged, keep the strongest member hit", Value: fmt.Sprintf("%t", familySettings.KeepBestHitPerTarget), Explanation: "When several family members hit the same target, the best-ranked row represents that target."},
		{Name: "TXT export: include only the first query sequence", Value: fmt.Sprintf("%t", familySettings.PrependOnlyFirstQuery), Explanation: "If false, every family-member query sequence is prepended in family order."},
		{Name: "minimum queries in a family", Value: strconv.Itoa(maxInt(familySettings.MinimumGroupSize, 2)), Explanation: "Minimum number of related queries required before a family review/export unit is formed."},
		{Name: "Remove leading species-style prefix", Value: fmt.Sprintf("%t", familySettings.StripLeadingSpeciesPrefix), Explanation: "Generic leading species-style prefixes are removed before family-name detection."},
		{Name: "Remove trailing member number", Value: fmt.Sprintf("%t", familySettings.StripTrailingQueryIndex), Explanation: "Trailing member numbers such as 1/2/3 are removed before grouping."},
		{Name: "Ignore suffix after a member number", Value: fmt.Sprintf("%t", familySettings.StripAfterNumberSuffix), Explanation: "Suffix text after a detected member number is ignored during family-name detection."},
		{Name: "Treat punctuation as the same separator", Value: fmt.Sprintf("%t", familySettings.NormalizeInnerPunctuation), Explanation: "Punctuation variants inside labels are normalized before grouping."},
		{Name: "Remove terminal subtype suffix", Value: fmt.Sprintf("%t", familySettings.StripTerminalSubtypeSuffix), Explanation: "Subtype markers such as -like are stripped before final family-name creation."},
		{Name: "Keep detected subgroups as separate families", Value: fmt.Sprintf("%t", familySettings.KeepDistinctQuerySubgroups), Explanation: "Detected subgroups remain separate instead of being collapsed into the broader family name."},
		{Name: "Use UniProt evidence when ranking merged rows", Value: fmt.Sprintf("%t", familySettings.UseUniProtReference), Explanation: "UniProt review, accession, annotation, and length evidence can contribute to duplicate-target ranking."},
		{Name: "Use InterPro evidence when ranking merged rows", Value: fmt.Sprintf("%t", familySettings.UseInterProReference), Explanation: "InterPro conserved-region evidence can contribute to duplicate-target ranking."},
		{Name: "best-hit ranking order", Value: familySettings.RankingTieBreakerOrder, Explanation: "Priority chain used when family rows are compared during merge and export preparation."},
	}
	return &report.FamilyBlastReport{
		Settings: settingsRows,
		Groups:   out,
	}
}

func blastFilterReport(prepared []blastQueryItem, runs []blastQueryRun, allRows []model.BlastResultRow, selectedRows []model.BlastResultRow, selectedMask []bool, flags []bool, settings model.BlastFilterSettings, applied bool, cleared bool) *report.BlastFilterReport {
	if !applied && !cleared && !anyBoolWorkflow(flags) {
		return nil
	}
	if settings == (model.BlastFilterSettings{}) {
		settings = model.DefaultBlastFilterSettings()
	}
	recommendedRemove := countBoolWorkflow(flags)
	recommendedKeep := maxInt(0, len(allRows)-recommendedRemove)
	selectedForRows := blastSelectedMaskForRows(allRows, selectedRows, selectedMask)
	userRescued := 0
	userRemovedAfterKeep := 0
	for i := range allRows {
		selected := i < len(selectedForRows) && selectedForRows[i]
		flagged := i < len(flags) && flags[i]
		if flagged && selected {
			userRescued++
		}
		if !flagged && !selected {
			userRemovedAfterKeep++
		}
	}
	return &report.BlastFilterReport{
		Applied:              applied || anyBoolWorkflow(flags),
		Cleared:              cleared,
		RecommendedKeep:      recommendedKeep,
		RecommendedRemove:    recommendedRemove,
		FinalSelected:        len(selectedRows),
		FinalUnselected:      maxInt(0, len(allRows)-len(selectedRows)),
		UserRescued:          userRescued,
		UserRemovedAfterKeep: userRemovedAfterKeep,
		Settings:             report.BlastFilterSettingDetails(settings),
		Formulas:             report.BlastFilterFormulas(blastFilterTotals(prepared, runs, allRows, selectedForRows, flags), settings),
		Totals:               blastFilterTotals(prepared, runs, allRows, selectedForRows, flags),
		QuerySummaries:       blastFilterQuerySummaries(prepared, runs, allRows, selectedForRows, flags),
		HardRuleSummaries:    blastFilterHardRuleSummaries(allRows, settings),
		Rows:                 blastFilterRows(runs, allRows, selectedForRows, flags, settings),
	}
}

func blastSelectedMaskForRows(allRows []model.BlastResultRow, selectedRows []model.BlastResultRow, selectedMask []bool) []bool {
	if len(selectedMask) == len(allRows) {
		return append([]bool(nil), selectedMask...)
	}
	selectedKeys := blastRowKeyCounts(selectedRows)
	out := make([]bool, len(allRows))
	for i, row := range allRows {
		out[i] = decrementBlastRowKey(selectedKeys, blastRowKey(row))
	}
	return out
}

func blastQueryPrependStepDetails(querySource *model.QuerySequenceSource, records []model.ProteinSequenceRecord, hitRecords []model.ProteinSequenceRecord) string {
	if querySource == nil || strings.TrimSpace(querySource.Sequence) == "" {
		return fmt.Sprintf("no query sequence record prepended; %d hit peptide records available", len(hitRecords))
	}
	return fmt.Sprintf("1 query sequence record prepended; %d hit peptide records available; %d total records", len(hitRecords), len(records))
}

func blastRunsSearchEndedAt(runs []blastQueryRun, fallback time.Time) time.Time {
	for i := len(runs) - 1; i >= 0; i-- {
		if t := runs[i].ResultsHashTime(); !t.IsZero() {
			return t
		}
	}
	if !fallback.IsZero() {
		return fallback
	}
	return time.Time{}
}

func (r blastQueryRun) ResultsHashTime() time.Time {
	return time.Time{}
}

func blastTopHit(rows []model.BlastResultRow) string {
	if len(rows) == 0 {
		return ""
	}
	return firstNonEmpty(rows[0].Protein, rows[0].SubjectID, rows[0].SequenceID)
}

func blastBestEValue(rows []model.BlastResultRow) string {
	if len(rows) == 0 {
		return ""
	}
	best := ""
	bestValue := 1e308
	for _, row := range rows {
		value := parseScientificFloatWorkflow(row.EValue, 1e308)
		if value < bestValue {
			bestValue = value
			best = row.EValue
		}
	}
	return best
}

func blastBestIdentity(rows []model.BlastResultRow) string {
	if len(rows) == 0 {
		return ""
	}
	best := ""
	bestValue := -1.0
	for _, row := range rows {
		if row.PercentIdentity > bestValue {
			bestValue = row.PercentIdentity
			best = fmt.Sprintf("%.1f%%", row.PercentIdentity)
		}
	}
	return best
}

func countBlastRowsWhere(rows []model.BlastResultRow, fn func(model.BlastResultRow) bool) int {
	count := 0
	for _, row := range rows {
		if fn(row) {
			count++
		}
	}
	return count
}

func countResolvedBlastItems(items []blastQueryItem) int {
	count := 0
	for _, item := range items {
		if blastInputOutcome(item) == "resolved" {
			count++
		}
	}
	return count
}

func countRunsWhere(runs []blastQueryRun, fn func(blastQueryRun) bool) int {
	count := 0
	for _, run := range runs {
		if fn(run) {
			count++
		}
	}
	return count
}

func blastRowKey(row model.BlastResultRow) string {
	return strings.ToLower(strings.TrimSpace(firstNonEmpty(row.Protein, row.SubjectID, row.SequenceID, row.TranscriptID, row.GeneReportURL)))
}

func blastRowKeyCounts(rows []model.BlastResultRow) map[string]int {
	counts := make(map[string]int)
	for _, row := range rows {
		key := blastRowKey(row)
		if key != "" {
			counts[key]++
		}
	}
	return counts
}

func decrementBlastRowKey(counts map[string]int, key string) bool {
	if key == "" {
		return false
	}
	if counts[key] > 0 {
		counts[key]--
		return true
	}
	return false
}

func uniqueBlastValues(rows []model.BlastResultRow, fn func(model.BlastResultRow) string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, row := range rows {
		value := strings.TrimSpace(fn(row))
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
	sort.Strings(out)
	return out
}

func blastInterProStatusCounts(rows []model.BlastResultRow) []report.NameValue {
	counts := map[string]int{}
	for _, row := range rows {
		status := strings.ToLower(strings.TrimSpace(row.InterProConservedRegionStatus))
		if status == "" {
			status = "blank"
		}
		counts[status]++
	}
	order := []string{"present", "partial", "missing", "uncertain", "blank"}
	out := make([]report.NameValue, 0, len(order))
	for _, key := range order {
		if count := counts[key]; count > 0 {
			out = append(out, report.NameValue{Name: key, Value: strconv.Itoa(count), Explanation: "captured from the already-enriched BLAST rows"})
		}
	}
	return out
}

func blastColumnSource(header string) string {
	switch header {
	case "row":
		return "generated by export workflow"
	case "source_database", "blast_program", "label_name", "labelname_type", "protein", "subject_id", "species", "e_value", "percent_identity", "align_query_length_percent", "target_length", "align_len", "strands", "query_id", "query_from", "query_to", "target_from", "target_to", "bitscore", "mismatches", "gap_openings", "identical", "positives", "gaps", "query_length", "jbrowse_name", "target_id", "sequence_id", "transcript_id", "defline", "gene_report_url":
		return "BLAST result"
	case "phgo_alias":
		return "labelname system"
	case "blast_labelname", "blast_geneid":
		return "BLAST query source"
	case "interpro_conserved_region_status", "interpro_accessions", "interpro_entry_name", "interpro_entry_type", "interpro_coverage_percent", "interpro_match_regions":
		return "InterPro enrichment"
	case "uniprot_accession", "uniprot_reviewed", "uniprot_protein_name", "uniprot_gene_names", "uniprot_function", "uniprot_fragment", "uniprot_sequence_caution", "uniprot_canonical_length", "target_uniprot_canonical_length_percent":
		return "UniProt enrichment"
	default:
		return "workflow/export"
	}
}

func blastColumnMeaning(header string) string {
	switch header {
	case "row":
		return "Stable row number in the exported BLAST table."
	case "source_database":
		return "Original target database for the hit row."
	case "blast_program":
		return "BLAST program used for the row."
	case "label_name":
		return "Readable label for this BLAST hit row."
	case "labelname_type":
		return "How this row's label_name was obtained."
	case "phgo_alias":
		return "Ranked alias list for this BLAST hit row, produced by the labelname system."
	case "protein":
		return "Target protein identifier returned by BLAST/source parser."
	case "blast_labelname":
		return "Labelname of the query gene or sequence that produced this hit."
	case "blast_geneid":
		return "Transcript-style identifier of the query sequence that produced this hit; displayed externally as blast_transcript."
	case "subject_id":
		return "BLAST subject identifier."
	case "e_value":
		return "BLAST E-value for the alignment."
	case "percent_identity":
		return "Percent identity for aligned positions."
	case "align_query_length_percent":
		return "Alignment length divided by query length percentage."
	case "target_length":
		return "Target protein length."
	case "interpro_conserved_region_status":
		return "Conserved-region status under configured InterPro evidence settings."
	case "gene_report_url":
		return "Target-side source report URL for the BLAST hit."
	default:
		return "BLAST export column derived from the current workflow state."
	}
}

func blastColumnCollection(header string) string {
	switch header {
	case "row":
		return "generated during export"
	case "source_database", "blast_program":
		return "copied from request/run state"
	case "label_name", "labelname_type":
		return "copied or inferred for the BLAST hit row"
	case "phgo_alias":
		return "computed by BLAST-hit labelname auto-identification or preserved from the hit row's stored alias ranking"
	case "blast_labelname", "blast_geneid":
		return "copied from the BLAST query source"
	case "protein", "subject_id", "species", "e_value", "percent_identity", "align_query_length_percent", "target_length", "align_len", "strands", "query_id", "query_from", "query_to", "target_from", "target_to", "bitscore", "mismatches", "gap_openings", "identical", "positives", "gaps", "query_length", "jbrowse_name", "target_id", "sequence_id", "transcript_id", "defline", "gene_report_url":
		return "parsed from BLAST result rows"
	case "uniprot_accession", "uniprot_reviewed", "uniprot_protein_name", "uniprot_gene_names", "uniprot_function", "uniprot_fragment", "uniprot_sequence_caution", "uniprot_canonical_length", "target_uniprot_canonical_length_percent":
		return "attached during UniProt enrichment"
	case "interpro_conserved_region_status", "interpro_accessions", "interpro_entry_name", "interpro_entry_type", "interpro_coverage_percent", "interpro_match_regions":
		return "attached during InterPro enrichment"
	default:
		return "derived from existing export state"
	}
}

func blastColumnBlankMeaning(header string) string {
	switch header {
	case "label_name":
		return "hit label was skipped, unavailable, or hit auto-labeling was disabled"
	case "labelname_type":
		return "label_name is blank or the old row did not record its source"
	case "phgo_alias":
		return "hit auto-labeling was disabled, skipped, or no usable hit alias candidates were available"
	case "blast_labelname", "blast_geneid":
		return "query source did not carry this metadata"
	case "gene_report_url":
		return "stable target page unavailable"
	case "uniprot_accession", "uniprot_reviewed", "uniprot_protein_name", "uniprot_gene_names", "uniprot_function", "uniprot_fragment", "uniprot_sequence_caution", "uniprot_canonical_length", "target_uniprot_canonical_length_percent":
		return "UniProt not requested, no accession, or canonical length unavailable"
	case "interpro_conserved_region_status", "interpro_accessions", "interpro_entry_name", "interpro_entry_type", "interpro_coverage_percent", "interpro_match_regions":
		return "InterPro disabled, no accession, no entry, or no status"
	default:
		return "blank or not collected in this run"
	}
}

func blastColumnStatsUse(header string) string {
	switch header {
	case "row", "label_name", "labelname_type", "phgo_alias", "protein", "blast_labelname", "blast_geneid", "subject_id", "e_value", "percent_identity", "align_query_length_percent", "target_length", "gene_report_url":
		return "traceability"
	case "uniprot_accession", "uniprot_reviewed", "uniprot_protein_name", "uniprot_function", "uniprot_fragment", "uniprot_sequence_caution", "uniprot_canonical_length", "target_uniprot_canonical_length_percent":
		return "filter/reference"
	case "interpro_conserved_region_status", "interpro_accessions", "interpro_entry_name", "interpro_entry_type", "interpro_coverage_percent", "interpro_match_regions":
		return "filter/reference"
	default:
		return "export"
	}
}

func blastFilterHardRuleSummaries(rows []model.BlastResultRow, settings model.BlastFilterSettings) []report.BlastFilterRuleSummary {
	rules := []report.BlastFilterRuleSummary{}
	add := func(name string, active bool, failed int, rule string, explanation string, source string) {
		result := "not active"
		if active {
			if failed == 0 {
				result = "pass"
			} else {
				result = "warning"
			}
		}
		rules = append(rules, report.BlastFilterRuleSummary{
			Name:        name,
			Result:      result,
			Passed:      maxInt(0, len(rows)-failed),
			Failed:      failed,
			Rule:        rule,
			Explanation: explanation,
			Source:      source,
		})
	}
	add("Identity threshold", settings.MinIdentityPercent > 0, countBlastRowsWhere(rows, func(row model.BlastResultRow) bool { return row.PercentIdentity < settings.MinIdentityPercent }), fmt.Sprintf("MinIdentityPercent > 0 and identity < %.1f", settings.MinIdentityPercent), "Identity is an optional hard rule; when disabled it still remains available for review and ranking.", "BLAST percent_identity")
	add("Query coverage threshold", settings.MinAlignQueryCoveragePercent > 0, countBlastRowsWhere(rows, func(row model.BlastResultRow) bool {
		return blastRowCoverageForReport(row) < settings.MinAlignQueryCoveragePercent
	}), fmt.Sprintf("MinAlignQueryCoveragePercent > 0 and coverage < %.1f", settings.MinAlignQueryCoveragePercent), "Query coverage is optional as a hard rule and describes how much of the query participates in the hit.", "BLAST coverage")
	add("E-value threshold", settings.MaxEValue > 0, countBlastRowsWhere(rows, func(row model.BlastResultRow) bool {
		return parseScientificFloatWorkflow(row.EValue, 1e308) > settings.MaxEValue
	}), fmt.Sprintf("MaxEValue > 0 and E-value > %s", formatScientificSettingWorkflow(settings.MaxEValue)), "E-value can be used as a strict maximum when configured.", "BLAST e_value")
	add("Canonical length ratio", settings.UseTargetCanonicalLengthRatio, countBlastRowsWhere(rows, func(row model.BlastResultRow) bool {
		ratio := parseScientificFloatWorkflow(row.TargetUniProtCanonicalLengthPercent, 0)
		if ratio == 0 {
			return settings.RequireTargetCanonicalLengthRatio
		}
		return ratio < settings.MinTargetCanonicalLengthPercent || ratio > settings.MaxTargetCanonicalLengthPercent
	}), fmt.Sprintf("ratio required=%t and %.1f <= ratio <= %.1f", settings.RequireTargetCanonicalLengthRatio, settings.MinTargetCanonicalLengthPercent, settings.MaxTargetCanonicalLengthPercent), "The target protein length is compared with UniProt canonical length to detect fragments, extensions, or isoform mismatches.", "UniProt length comparison")
	add("UniProt accession required", settings.RequireUniProtAccession, countBlastRowsWhere(rows, func(row model.BlastResultRow) bool { return strings.TrimSpace(row.UniProtAccession) == "" }), "RequireUniProtAccession=true and accession is blank", "Rows can be removed solely for missing UniProt mapping when the setting is enabled.", "UniProt accession")
	add("UniProt fragment rejection", settings.RejectUniProtFragments, countBlastRowsWhere(rows, func(row model.BlastResultRow) bool { return isTruthyWorkflow(row.UniProtFragment) }), "RejectUniProtFragments=true and fragment is truthy", "Fragment-marked reference entries are treated as lower-confidence targets.", "UniProt fragment")
	add("UniProt sequence caution rejection", settings.RejectUniProtSequenceCautions, countBlastRowsWhere(rows, func(row model.BlastResultRow) bool { return strings.TrimSpace(row.UniProtSequenceCaution) != "" }), "RejectUniProtSequenceCautions=true and caution is present", "Sequence caution indicates a reference-level warning that can remove the row when configured.", "UniProt sequence caution")
	add("InterPro conserved-region requirement", settings.RequireInterProConservedRegion, countBlastRowsWhere(rows, func(row model.BlastResultRow) bool {
		status := strings.ToLower(strings.TrimSpace(row.InterProConservedRegionStatus))
		switch status {
		case "present":
			return false
		case "partial":
			return !settings.AllowInterProPartial
		case "missing", "":
			return settings.RejectInterProMissing
		case "uncertain":
			return settings.RejectInterProUncertain
		default:
			return true
		}
	}), fmt.Sprintf("RequireInterProConservedRegion=true; partial allowed=%t; reject missing=%t; reject uncertain=%t", settings.AllowInterProPartial, settings.RejectInterProMissing, settings.RejectInterProUncertain), "Conserved-region status connects BLAST hits to optional InterPro evidence already attached during enrichment.", "InterPro status")
	add("InterPro coverage threshold", settings.MinInterProCoveragePercent > 0 || settings.RequireInterProCoverageWhenUsed, countBlastRowsWhere(rows, func(row model.BlastResultRow) bool {
		cov := parseScientificFloatWorkflow(row.InterProCoveragePercent, 0)
		if cov == 0 {
			return settings.RequireInterProCoverageWhenUsed
		}
		return settings.MinInterProCoveragePercent > 0 && cov < settings.MinInterProCoveragePercent
	}), fmt.Sprintf("Require coverage=%t and coverage >= %.1f", settings.RequireInterProCoverageWhenUsed, settings.MinInterProCoveragePercent), "InterPro coverage can act as a second conserved-region gate when configured.", "InterPro coverage")
	return rules
}

func blastFilterRows(runs []blastQueryRun, rows []model.BlastResultRow, selectedMask []bool, flags []bool, settings model.BlastFilterSettings) []report.BlastFilterRowSummary {
	out := make([]report.BlastFilterRowSummary, 0, len(rows))
	queryLabels := blastFilterRowQueryLabels(runs, len(rows))
	for i, row := range rows {
		selected := i < len(selectedMask) && selectedMask[i]
		recommended := "filter kept"
		if i < len(flags) && flags[i] {
			recommended = "filter removed"
		}
		difference := "match"
		switch {
		case selected && i < len(flags) && flags[i]:
			difference = "rescued"
		case !selected && i < len(flags) && !flags[i]:
			difference = "user removed"
		case selected && (i >= len(flags) || !flags[i]):
			difference = "kept"
		case !selected && i < len(flags) && flags[i]:
			difference = "removed"
		}
		out = append(out, report.BlastFilterRowSummary{
			Row:             i + 1,
			Query:           valueOrWorkflow(queryLabels, i, row.LabelName),
			Label:           row.LabelName,
			Family:          row.FamilyName,
			Target:          firstNonEmpty(row.Protein, row.SubjectID, row.SequenceID),
			Identity:        fmt.Sprintf("%.1f%%", row.PercentIdentity),
			Coverage:        fmt.Sprintf("%.1f%%", blastRowCoverageForReport(row)),
			EValue:          row.EValue,
			LengthRatio:     row.TargetUniProtCanonicalLengthPercent,
			FamilySupport:   blastFilterFamilySupportSummary(row),
			FamilySemantic:  blastFilterFamilySemanticSummary(row),
			UniProtEvidence: firstNonEmpty(row.UniProtReviewed, row.UniProtAccession, row.UniProtProteinName),
			InterProStatus:  row.InterProConservedRegionStatus,
			Recommended:     recommended,
			FinalSelection:  blastFilterSelectionLabel(selected),
			Difference:      difference,
			ScoreComponents: blastFilterScoreSummary(row, settings),
			HardFailures:    blastFilterFailureSummary(row, settings),
		})
	}
	return out
}

func blastFilterFamilySupportSummary(row model.BlastResultRow) string {
	if row.FamilyConsensusSupport <= 0 && strings.TrimSpace(row.FamilyConsensusCoveragePercent) == "" {
		return ""
	}
	parts := []string{}
	if row.FamilyConsensusSupport > 0 {
		parts = append(parts, fmt.Sprintf("%d", row.FamilyConsensusSupport))
	}
	if row.FamilyConsensusSize > 0 {
		parts = append(parts, fmt.Sprintf("/%d", row.FamilyConsensusSize))
	}
	if cov := strings.TrimSpace(row.FamilyConsensusCoveragePercent); cov != "" {
		parts = append(parts, fmt.Sprintf("(%s%%)", cov))
	}
	return strings.Join(parts, " ")
}

func blastFilterFamilySemanticSummary(row model.BlastResultRow) string {
	if row.FamilySemanticAnnotationMatchCount <= 0 && strings.TrimSpace(row.FamilySemanticAgreementPercent) == "" {
		return ""
	}
	parts := make([]string, 0, 3)
	if row.FamilySemanticAnnotationMatchCount > 0 {
		parts = append(parts, fmt.Sprintf("%d match", row.FamilySemanticAnnotationMatchCount))
	}
	if pct := strings.TrimSpace(row.FamilySemanticAgreementPercent); pct != "" {
		parts = append(parts, fmt.Sprintf("(%s%%)", pct))
	}
	if tokens := strings.TrimSpace(row.FamilySemanticAnnotationMatchTokens); tokens != "" {
		parts = append(parts, tokens)
	}
	return strings.Join(parts, " ")
}

func blastFilterQuerySummaries(prepared []blastQueryItem, runs []blastQueryRun, allRows []model.BlastResultRow, selectedMask []bool, flags []bool) []report.BlastFilterQuerySummary {
	if len(runs) == 0 {
		return nil
	}
	summaries := make([]report.BlastFilterQuerySummary, 0, len(runs))
	rowCursor := 0
	for _, run := range runs {
		rows := run.Results.Rows
		total := len(rows)
		end := rowCursor + total
		recommendedRemove := 0
		finalSelected := len(run.SelectedRows)
		if end <= len(selectedMask) {
			finalSelected = 0
			for j := rowCursor; j < end; j++ {
				if selectedMask[j] {
					finalSelected++
				}
			}
		}
		finalUnselected := maxInt(0, total-finalSelected)
		userRescued := 0
		userRemoved := 0
		matchedRows := 0
		if end <= len(selectedMask) && end <= len(flags) {
			for j := rowCursor; j < end; j++ {
				if flags[j] {
					recommendedRemove++
				}
				if flags[j] && selectedMask[j] {
					userRescued++
				}
				if !flags[j] && !selectedMask[j] {
					userRemoved++
				}
				if (!flags[j] && selectedMask[j]) || (flags[j] && !selectedMask[j]) {
					matchedRows++
				}
			}
		} else {
			recommendedRemove = countBoolWorkflow(flags)
		}
		recommendedKeep := maxInt(0, total-recommendedRemove)
		summaries = append(summaries, report.BlastFilterQuerySummary{
			Query:             firstNonEmpty(run.Item.MemberLabel, run.Item.LabelName, reportQueryLabel(run.Item)),
			Family:            run.Item.FamilyName,
			TotalRows:         total,
			RecommendedKeep:   recommendedKeep,
			RecommendedRemove: recommendedRemove,
			FinalSelected:     finalSelected,
			FinalUnselected:   finalUnselected,
			UserRescued:       userRescued,
			UserRemoved:       userRemoved,
			MatchedRows:       matchedRows,
			Difference:        finalSelected - recommendedKeep,
			AgreementPercent:  reportPercentText(matchedRows, total),
		})
		rowCursor = end
	}
	return summaries
}

func blastFilterTotals(prepared []blastQueryItem, runs []blastQueryRun, allRows []model.BlastResultRow, selectedMask []bool, flags []bool) report.BlastFilterTotals {
	summaries := blastFilterQuerySummaries(prepared, runs, allRows, selectedMask, flags)
	t := report.BlastFilterTotals{QueryCount: len(summaries), TotalRows: len(allRows)}
	for i := range allRows {
		flagged := i < len(flags) && flags[i]
		selected := i < len(selectedMask) && selectedMask[i]
		if flagged {
			t.RecommendedRemove++
		} else {
			t.RecommendedKeep++
		}
		if selected {
			t.FinalSelected++
		} else {
			t.FinalUnselected++
		}
		if flagged && selected {
			t.UserRescued++
			t.DifferenceRows++
		}
		if !flagged && !selected {
			t.UserRemoved++
			t.DifferenceRows++
		}
		if (!flagged && selected) || (flagged && !selected) {
			t.MatchedRows++
		}
	}
	t.AgreementPercent = reportPercentText(t.MatchedRows, t.TotalRows)
	return t
}

func blastFilterRowQueryLabels(runs []blastQueryRun, total int) []string {
	labels := make([]string, 0, total)
	for _, run := range runs {
		label := firstNonEmpty(run.Item.MemberLabel, run.Item.LabelName, reportQueryLabel(run.Item))
		for range run.Results.Rows {
			labels = append(labels, label)
		}
	}
	for len(labels) < total {
		labels = append(labels, "")
	}
	return labels
}

func blastRowCoverageForReport(row model.BlastResultRow) float64 {
	if row.AlignQueryLengthPercent > 0 {
		return row.AlignQueryLengthPercent
	}
	if row.AlignLength > 0 && row.QueryLength > 0 {
		return float64(row.AlignLength) / float64(row.QueryLength) * 100
	}
	return 0
}

func valueOrWorkflow(values []string, index int, fallback string) string {
	if index >= 0 && index < len(values) && strings.TrimSpace(values[index]) != "" {
		return values[index]
	}
	return fallback
}

func reportPercentText(value int, total int) string {
	if total <= 0 {
		return "not available"
	}
	return fmt.Sprintf("%.1f%%", float64(value)/float64(total)*100)
}

func blastFilterScoreSummary(row model.BlastResultRow, settings model.BlastFilterSettings) string {
	parts := make([]string, 0, 6)
	if settings.MinIdentityPercent > 0 {
		parts = append(parts, fmt.Sprintf("identity %.1f", row.PercentIdentity))
	}
	if settings.UseTargetCanonicalLengthRatio {
		parts = append(parts, "length ratio "+row.TargetUniProtCanonicalLengthPercent)
	}
	if row.UniProtReviewed != "" {
		parts = append(parts, "reviewed")
	}
	if row.InterProConservedRegionStatus != "" {
		parts = append(parts, "InterPro "+row.InterProConservedRegionStatus)
	}
	if settings.UseFamilySemanticAgreement && blastRowHasSemanticTokensWorkflow(row) && row.FamilySemanticAnnotationMatchCount > 0 {
		parts = append(parts, "semantic "+blastFilterFamilySemanticSummary(row))
	}
	if len(parts) == 0 {
		return "baseline BLAST evidence"
	}
	return strings.Join(parts, "; ")
}

func blastFilterFailureSummary(row model.BlastResultRow, settings model.BlastFilterSettings) string {
	failures := make([]string, 0, 5)
	if settings.UseTargetCanonicalLengthRatio && settings.RequireTargetCanonicalLengthRatio && strings.TrimSpace(row.TargetUniProtCanonicalLengthPercent) == "" {
		failures = append(failures, "missing length ratio")
	}
	if settings.RequireUniProtAccession && strings.TrimSpace(row.UniProtAccession) == "" {
		failures = append(failures, "missing UniProt accession")
	}
	if settings.RejectUniProtFragments && isTruthyWorkflow(row.UniProtFragment) {
		failures = append(failures, "fragment")
	}
	switch strings.ToLower(strings.TrimSpace(row.InterProConservedRegionStatus)) {
	case "":
		if settings.RequireInterProConservedRegion {
			failures = append(failures, "InterPro missing")
		}
	case "missing", "uncertain":
		if settings.RequireInterProConservedRegion {
			failures = append(failures, "InterPro "+strings.ToLower(strings.TrimSpace(row.InterProConservedRegionStatus)))
		}
	}
	if settings.UseFamilySemanticAgreement && settings.RequireFamilySemanticAgreement && blastRowHasSemanticTokensWorkflow(row) && blastRowHasSemanticReferenceSurfaceWorkflow(row) && !blastRowHasSemanticAgreementPrompt(row, settings) {
		failures = append(failures, "family semantic mismatch")
	}
	if len(failures) == 0 {
		return "none"
	}
	return strings.Join(failures, "; ")
}

func blastRowHasSemanticAgreementPrompt(row model.BlastResultRow, settings model.BlastFilterSettings) bool {
	if !blastRowHasSemanticTokensWorkflow(row) {
		return true
	}
	if row.FamilySemanticAnnotationMatchCount < settings.FamilySemanticMinTokenMatches {
		return false
	}
	if settings.FamilySemanticMinAgreementPercent <= 0 {
		return true
	}
	return parseScientificFloatWorkflow(row.FamilySemanticAgreementPercent, 0) >= settings.FamilySemanticMinAgreementPercent
}

func blastRowHasSemanticTokensWorkflow(row model.BlastResultRow) bool {
	return strings.TrimSpace(row.FamilySemanticTokens) != "" || strings.TrimSpace(row.FamilySemanticAliasTokens) != ""
}

func blastRowHasSemanticReferenceSurfaceWorkflow(row model.BlastResultRow) bool {
	for _, value := range []string{
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
	} {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func blastFilterRecommendation(flags []bool, index int) string {
	if index < len(flags) && flags[index] {
		return "filter removed"
	}
	return "filter kept"
}

func blastFilterSelectionLabel(selected bool) string {
	if selected {
		return "selected"
	}
	return "unselected"
}

func blastSequenceRecordKind(record model.ProteinSequenceRecord, sources []*model.QuerySequenceSource) string {
	if strings.Contains(strings.ToLower(record.Header), "query") {
		return "query sequence"
	}
	for _, src := range sources {
		if src != nil && strings.EqualFold(record.Header, firstNonEmpty(src.LabelName, src.GeneID, src.TranscriptID, src.ProteinID)) {
			return "query sequence"
		}
	}
	return "hit peptide record"
}

func blastSequenceRecordLabel(header string) string {
	return header
}

func countQuerySourcesWithSequence(sources []*model.QuerySequenceSource) int {
	count := 0
	for _, src := range sources {
		if src != nil && strings.TrimSpace(src.Sequence) != "" {
			count++
		}
	}
	return count
}

func anyBoolWorkflow(values []bool) bool {
	for _, value := range values {
		if value {
			return true
		}
	}
	return false
}

func countBoolWorkflow(values []bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func availabilityText(ok bool) string {
	if ok {
		return "available"
	}
	return "not available in this run"
}

func intString(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
}

func formatScientificSettingWorkflow(value float64) string {
	if value == 0 {
		return "off"
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func sourceDatabaseForBlastRows(rows []model.BlastResultRow) string {
	for _, row := range rows {
		if value := strings.TrimSpace(row.SourceDatabase); value != "" {
			return value
		}
	}
	return "phytozome"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func countNonEmptyStrings(values ...string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

func blastProvenance(rows []model.BlastResultRow) []report.ProvenanceSlice {
	counts := map[string]int{}
	for _, row := range rows {
		for _, value := range []string{row.BlastProgram, row.EValue, row.Protein, row.SubjectID, row.SequenceID, row.TranscriptID, row.GeneReportURL} {
			if strings.TrimSpace(value) == "" {
				counts["unavailable/missing"]++
			} else {
				counts["BLAST execution result"]++
			}
		}
		if row.UniProtReferenceEnabled {
			counts["UniProt enrichment"] += countNonEmptyStrings(row.UniProtAccession, row.UniProtReviewed, row.UniProtProteinName, row.UniProtCanonicalLength, row.TargetUniProtCanonicalLengthPercent)
		}
		if row.InterProReferenceEnabled {
			counts["InterPro enrichment"] += countNonEmptyStrings(row.InterProConservedRegionStatus, row.InterProAccessions, row.InterProEntryName, row.InterProCoveragePercent)
		}
		counts["generated/internal"] += 2
	}
	labels := []string{"BLAST execution result", "UniProt enrichment", "InterPro enrichment", "generated/internal", "unavailable/missing"}
	out := make([]report.ProvenanceSlice, 0, len(labels))
	for _, label := range labels {
		if count := counts[label]; count > 0 {
			out = append(out, report.ProvenanceSlice{Label: label, Count: count, Explanation: blastProvenanceExplanation(label)})
		}
	}
	return out
}

func blastProvenanceExplanation(label string) string {
	switch label {
	case "BLAST execution result":
		return "Values already present in parsed BLAST result rows."
	case "UniProt enrichment":
		return "Optional UniProt values already attached during normal enrichment."
	case "InterPro enrichment":
		return "Optional InterPro values already attached during normal enrichment."
	case "generated/internal":
		return "Values generated by phytozome GO for labels, row numbers, filtering, export, or reporting."
	case "unavailable/missing":
		return "Fields blank or not collected in the current run; the report did not fill them with extra lookups."
	default:
		return "Provenance category captured from completed BLAST export state."
	}
}

func blastColumnCompleteness(rows []model.BlastResultRow) []report.ColumnCompleteness {
	headers := blastReportHeaders(rows)
	out := make([]report.ColumnCompleteness, 0, len(headers))
	total := len(rows)
	for _, header := range headers {
		filled := 0
		for i, row := range rows {
			if strings.TrimSpace(blastReportCellValue(header, row, i)) != "" {
				filled++
			}
		}
		empty := total - filled
		ratio := "0.0%"
		if total > 0 {
			ratio = fmt.Sprintf("%.1f%%", float64(filled)/float64(total)*100)
		}
		out = append(out, report.ColumnCompleteness{Column: prompt.ColumnExportHeader(header, prompt.ColumnDisplayOptions{DatabaseDisplay: sourceDatabaseForBlastRows(rows)}), FilledRows: filled, EmptyRows: empty, TotalRows: total, FilledRatio: ratio, Source: blastColumnSource(header)})
	}
	return out
}

func blastQualityChecks(prepared []blastQueryItem, runs []blastQueryRun, selectedRows []model.BlastResultRow, allRows []model.BlastResultRow, settings exportSettings, sequenceAudit report.SequenceAudit) []report.QualityCheck {
	checks := []report.QualityCheck{
		qualityCheck("Resolved query sequences", countResolvedBlastItems(prepared) == len(prepared), fmt.Sprintf("%d of %d", countResolvedBlastItems(prepared), len(prepared)), "warn when parsed records fail resolution", "Only resolved query records can be submitted to BLAST.", "input trace"),
		qualityCheck("BLAST result identifiers available", countRunsWhere(runs, func(run blastQueryRun) bool {
			return strings.TrimSpace(run.Results.JobID) == "" && strings.TrimSpace(run.Results.Hash) == "" && strings.TrimSpace(run.Results.ZUID) == ""
		}) == 0, fmt.Sprintf("%d missing of %d", countRunsWhere(runs, func(run blastQueryRun) bool {
			return strings.TrimSpace(run.Results.JobID) == "" && strings.TrimSpace(run.Results.Hash) == "" && strings.TrimSpace(run.Results.ZUID) == ""
		}), len(runs)), "warn when no job/result marker is available", "Job IDs, hashes, or ZUIDs help trace a BLAST run.", "BLAST result"),
		qualityCheck("Selected rows missing E-value", countBlastRowsWhere(selectedRows, func(row model.BlastResultRow) bool { return strings.TrimSpace(row.EValue) == "" }) == 0, fmt.Sprintf("%d of %d", countBlastRowsWhere(selectedRows, func(row model.BlastResultRow) bool { return strings.TrimSpace(row.EValue) == "" }), len(selectedRows)), "warn when selected rows lack E-value", "E-value is a core BLAST metric.", "selected rows"),
		qualityCheck("Selected rows missing identity", countBlastRowsWhere(selectedRows, func(row model.BlastResultRow) bool { return row.PercentIdentity <= 0 }) == 0, fmt.Sprintf("%d of %d", countBlastRowsWhere(selectedRows, func(row model.BlastResultRow) bool { return row.PercentIdentity <= 0 }), len(selectedRows)), "warn when selected rows lack identity", "Percent identity is a core BLAST metric.", "selected rows"),
		qualityCheck("Selected rows missing target identifier", countBlastRowsWhere(selectedRows, func(row model.BlastResultRow) bool {
			return firstNonEmpty(row.Protein, row.SubjectID, row.SequenceID, row.TranscriptID) == ""
		}) == 0, fmt.Sprintf("%d of %d", countBlastRowsWhere(selectedRows, func(row model.BlastResultRow) bool {
			return firstNonEmpty(row.Protein, row.SubjectID, row.SequenceID, row.TranscriptID) == ""
		}), len(selectedRows)), "warn when selected rows lack stable target ID", "Target identifiers are needed for later review and sequence export.", "selected rows"),
	}
	if blastRowsHaveUniProt(allRows) {
		checks = append(checks, qualityCheck("UniProt accession availability", countBlastRowsWhere(selectedRows, func(row model.BlastResultRow) bool { return strings.TrimSpace(row.UniProtAccession) == "" }) == 0, fmt.Sprintf("%d missing of %d", countBlastRowsWhere(selectedRows, func(row model.BlastResultRow) bool { return strings.TrimSpace(row.UniProtAccession) == "" }), len(selectedRows)), "warn when UniProt was enabled but selected rows lack accession", "Missing accession limits external reference evidence.", "UniProt enrichment"))
	}
	if blastRowsHaveInterPro(allRows) {
		checks = append(checks, qualityCheck("InterPro status availability", countBlastRowsWhere(selectedRows, func(row model.BlastResultRow) bool { return strings.TrimSpace(row.InterProConservedRegionStatus) == "" }) == 0, fmt.Sprintf("%d missing of %d", countBlastRowsWhere(selectedRows, func(row model.BlastResultRow) bool { return strings.TrimSpace(row.InterProConservedRegionStatus) == "" }), len(selectedRows)), "warn when InterPro was enabled but selected rows lack status", "Conserved-region status is needed for reference-aware filtering.", "InterPro enrichment"))
	}
	if settings.WriteText {
		checks = append(checks, qualityCheck("BLAST peptide text completeness", sequenceAudit.SkippedCount == 0, fmt.Sprintf("%d written / %d requested", sequenceAudit.WrittenCount, sequenceAudit.RequestedCount), "warn when written < requested", "Sequence export is complete only when each requested selected row and query record produced a sequence record.", "sequence export state"))
	} else {
		checks = append(checks, report.QualityCheck{Name: "BLAST peptide text completeness", Result: "not applicable", Count: "not requested", Rule: "text export was not requested", Explanation: "No sequence fetching was performed for report generation.", Source: "export settings"})
	}
	return checks
}

func blastColumnLineage(rows []model.BlastResultRow, database string, program string, includeUniProt bool, includeInterPro bool) []report.ColumnLineage {
	headers := prompt.BlastReportColumnIDs(database, program, includeUniProt, includeInterPro)
	out := make([]report.ColumnLineage, 0, len(headers))
	for _, header := range headers {
		english := prompt.ColumnHelpEnglish(header)
		chinese := prompt.ColumnHelpChinese(header)
		japanese := prompt.ColumnHelpJapanese(header)
		meaning := firstNonEmpty(english, blastColumnMeaning(header))
		if blastColumnNeedsContextMeaning(header) {
			meaning = blastColumnMeaning(header)
		}
		out = append(out, report.ColumnLineage{
			ID:               header,
			Column:           prompt.ColumnExportHeader(header, prompt.ColumnDisplayOptions{DatabaseDisplay: database}),
			Meaning:          meaning,
			EnglishDetail:    english,
			ChineseDetail:    chinese,
			JapaneseDetail:   japanese,
			Source:           blastColumnSource(header),
			CollectionMethod: blastColumnCollection(header),
			BlankMeaning:     blastColumnBlankMeaning(header),
			UsedInStats:      blastColumnStatsUse(header),
		})
	}
	return out
}

func blastColumnNeedsContextMeaning(header string) bool {
	switch header {
	case "label_name", "labelname_type", "phgo_alias", "blast_labelname", "blast_geneid":
		return true
	default:
		return false
	}
}

func blastExportSettings(baseName string, outputDir string, settings exportSettings, rowNumbers []int, filterFlags []bool) []report.NameValue {
	return []report.NameValue{
		{Name: "File base name", Value: baseName, Explanation: "Base name used for selected Excel, raw Excel, peptide text, raw peptide text, and report outputs."},
		{Name: "Output folder", Value: outputDir, Explanation: "Destination directory for this BLAST export action."},
		{Name: "Family TXT prepend mode", Value: "first family query only / all family queries when disabled", Explanation: "Family BLAST controls whether TXT prepends only the first family-member query sequence or every family-member query sequence in family order."},
		{Name: "Write selected Excel", Value: fmt.Sprintf("%t", settings.WriteExcel), Explanation: "Selected BLAST rows are written to the main workbook when true."},
		{Name: "Write raw Excel and raw text", Value: fmt.Sprintf("%t", settings.WriteRawExcel), Explanation: "All current BLAST rows are written to _raw.xlsx, and _raw.txt is also written when text export is enabled."},
		{Name: "Write peptide text", Value: fmt.Sprintf("%t", settings.WriteText), Explanation: "Peptide sequences are fetched during normal export and written only when true."},
		{Name: "Write report PDF", Value: fmt.Sprintf("%t", settings.WriteReport), Explanation: "One PDF report is written for the current export action when true."},
		{Name: "rowNumbers", Value: availabilityText(len(rowNumbers) > 0), Explanation: "Selected workbook can preserve original review table row identities."},
		{Name: "filterFlags", Value: availabilityText(len(filterFlags) > 0), Explanation: "Excel row coloring mirrors final filter suggestion flags where supported."},
	}
}

func blastReportHeaders(rows []model.BlastResultRow) []string {
	return prompt.BlastExportColumnIDs(sourceDatabaseForBlastRows(rows), blastRowsHaveUniProt(rows), blastRowsHaveInterPro(rows))
}

func blastRowsProgramForReport(rows []model.BlastResultRow) string {
	for _, row := range rows {
		if value := strings.TrimSpace(row.BlastProgram); value != "" {
			return value
		}
	}
	return ""
}

func blastRunsUseUniProt(runs []blastQueryRun) bool {
	for _, run := range runs {
		for _, row := range run.Results.Rows {
			if row.UniProtReferenceEnabled {
				return true
			}
		}
	}
	return false
}

func blastRunsUseInterPro(runs []blastQueryRun) bool {
	for _, run := range runs {
		for _, row := range run.Results.Rows {
			if row.InterProReferenceEnabled {
				return true
			}
		}
	}
	return false
}

func blastReportCellValue(header string, row model.BlastResultRow, index int) string {
	switch header {
	case "row":
		return strconv.Itoa(index + 1)
	case "source_database":
		return row.SourceDatabase
	case "blast_program":
		return row.BlastProgram
	case "label_name":
		return row.LabelName
	case "labelname_type":
		return row.LabelNameType
	case "phgo_alias":
		return row.PhgoAliases
	case "hit_number":
		return strconv.Itoa(row.HitNumber)
	case "hsp_number":
		return strconv.Itoa(row.HSPNumber)
	case "protein":
		return row.Protein
	case "blast_labelname":
		return row.BlastLabelName
	case "blast_geneid":
		return row.BlastGeneID
	case "subject_id":
		return row.SubjectID
	case "species":
		return row.Species
	case "e_value":
		return row.EValue
	case "percent_identity":
		return fmt.Sprintf("%.2f", row.PercentIdentity)
	case "align_query_length_percent":
		return fmt.Sprintf("%.2f", row.AlignQueryLengthPercent)
	case "interpro_conserved_region_status":
		return row.InterProConservedRegionStatus
	case "target_length":
		return intString(row.TargetLength)
	case "align_len":
		return intString(row.AlignLength)
	case "strands":
		return row.Strands
	case "query_id":
		return row.QueryID
	case "query_from":
		return intString(row.QueryFrom)
	case "query_to":
		return intString(row.QueryTo)
	case "target_from":
		return intString(row.TargetFrom)
	case "target_to":
		return intString(row.TargetTo)
	case "bitscore":
		return fmt.Sprintf("%.2f", row.Bitscore)
	case "mismatches":
		return intString(row.Mismatches)
	case "gap_openings":
		return intString(row.GapOpenings)
	case "identical":
		return intString(row.Identical)
	case "positives":
		return intString(row.Positives)
	case "gaps":
		return intString(row.Gaps)
	case "query_length":
		return intString(row.QueryLength)
	case "target_uniprot_canonical_length_percent":
		return row.TargetUniProtCanonicalLengthPercent
	case "uniprot_canonical_length":
		return row.UniProtCanonicalLength
	case "jbrowse_name":
		return row.JBrowseName
	case "target_id":
		return intString(row.TargetID)
	case "sequence_id":
		return row.SequenceID
	case "transcript_id":
		return row.TranscriptID
	case "defline":
		return row.Defline
	case "gene_report_url":
		return row.GeneReportURL
	case "uniprot_accession":
		return row.UniProtAccession
	case "uniprot_reviewed":
		return row.UniProtReviewed
	case "uniprot_protein_name":
		return row.UniProtProteinName
	case "uniprot_gene_names":
		return row.UniProtGeneNames
	case "uniprot_function":
		return row.UniProtFunction
	case "uniprot_fragment":
		return row.UniProtFragment
	case "uniprot_sequence_caution":
		return row.UniProtSequenceCaution
	case "interpro_accessions":
		return row.InterProAccessions
	case "interpro_entry_name":
		return row.InterProEntryName
	case "interpro_entry_type":
		return row.InterProEntryType
	case "interpro_coverage_percent":
		return row.InterProCoveragePercent
	case "interpro_match_regions":
		return row.InterProMatchRegions
	default:
		return ""
	}
}

func buildBlastSequenceAudit(rows []model.BlastResultRow, records []model.ProteinSequenceRecord, querySources []*model.QuerySequenceSource, requested bool) report.SequenceAudit {
	audit := report.SequenceAudit{
		Requested:       requested,
		RequestedCount:  len(rows) + countQuerySourcesWithSequence(querySources),
		WrittenCount:    len(records),
		SkippedCount:    maxInt(0, len(rows)+countQuerySourcesWithSequence(querySources)-len(records)),
		TextFileType:    "BLAST peptide text export with query sequence records when available",
		HeaderLabelMode: "query records use the best available query label; hit records use selected BLAST row identifiers and label_name when available",
		TotalCharacters: 0,
		Records:         make([]report.SequenceRecord, 0, len(records)),
	}
	type summaryState struct {
		kind      string
		requested int
		written   int
		skipped   int
		totalLen  int
		minLen    int
		maxLen    int
		source    string
	}
	order := make([]string, 0)
	byQuery := map[string]*summaryState{}
	addState := func(label string, kind string, source string) *summaryState {
		label = firstNonEmpty(strings.TrimSpace(label), "sequence export")
		state := byQuery[label]
		if state == nil {
			state = &summaryState{kind: kind, minLen: -1, source: source}
			byQuery[label] = state
			order = append(order, label)
		}
		if state.kind == "" {
			state.kind = kind
		}
		if state.source == "" {
			state.source = source
		}
		return state
	}
	for _, source := range querySources {
		if source == nil || strings.TrimSpace(source.Sequence) == "" {
			continue
		}
		label := firstNonEmpty(source.LabelName, source.GeneID, source.TranscriptID, source.ProteinID, "query sequence")
		state := addState(label, "query sequence record", "prepended query sequence export")
		state.requested++
	}
	for _, row := range rows {
		label := firstNonEmpty(strings.TrimSpace(row.LabelName), firstNonEmpty(row.Protein, row.SubjectID, row.SequenceID), "selected BLAST hit")
		state := addState(label, "selected hit peptide records", "selected BLAST hit peptide export")
		state.requested++
		state.skipped++
	}
	for i, record := range records {
		audit.TotalCharacters += len(record.Sequence)
		label := blastSequenceRecordLabel(record.Header)
		kind := blastSequenceRecordKind(record, querySources)
		source := "sequence record produced during normal BLAST text export"
		state := addState(label, kind, source)
		state.written++
		state.totalLen += len(record.Sequence)
		if state.skipped > 0 {
			state.skipped--
		}
		if state.minLen < 0 || len(record.Sequence) < state.minLen {
			state.minLen = len(record.Sequence)
		}
		if len(record.Sequence) > state.maxLen {
			state.maxLen = len(record.Sequence)
		}
		audit.Records = append(audit.Records, report.SequenceRecord{
			Row:        i + 1,
			SearchTerm: kind,
			Label:      label,
			SequenceID: record.Header,
			Status:     "written",
			Length:     len(record.Sequence),
			Source:     source,
		})
	}
	audit.QuerySummaries = make([]report.SequenceQuerySummary, 0, len(order))
	for _, label := range order {
		state := byQuery[label]
		avg := 0
		minLen := maxInt(0, state.minLen)
		if state.written > 0 {
			avg = state.totalLen / state.written
		}
		audit.QuerySummaries = append(audit.QuerySummaries, report.SequenceQuerySummary{
			QueryLabel:     label,
			QueryKind:      state.kind,
			RequestedCount: state.requested,
			WrittenCount:   state.written,
			SkippedCount:   state.skipped,
			AverageLength:  avg,
			MinLength:      minLen,
			MaxLength:      state.maxLen,
			TotalLength:    state.totalLen,
			SourceSummary:  state.source,
		})
	}
	return audit
}

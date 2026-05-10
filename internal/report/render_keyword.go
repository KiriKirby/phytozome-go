package report

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func RenderKeywordPDF(path string, data ReportData) error {
	if data.GeneratedAt.IsZero() {
		data.GeneratedAt = time.Now()
	}
	if strings.TrimSpace(data.Title) == "" {
		data.Title = "Keyword Data Analysis Report"
	}
	r := newPDFReport(data.Title, data.GeneratedAt)
	renderKeywordReport(r, data)
	return r.save(path)
}

func renderKeywordReport(r *pdfReportRenderer, data ReportData) {
	k := data.Keyword

	r.title(data.Title, "Audit report for the current keyword-mode export action. The report describes only the data already collected during this run and does not perform any extra database, API, BLAST, sequence-fetch, cache-refresh, or download operation.")
	r.cards([]NameValue{
		{Name: "mode", Value: valueOr(data.Mode, "keyword"), Explanation: "current workflow"},
		{Name: "generated files", Value: strconv.Itoa(len(data.Files)), Explanation: "current export action"},
		{Name: "selected rows", Value: strconv.Itoa(k.Selection.SelectedRows), Explanation: "user export selection"},
		{Name: "total rows", Value: strconv.Itoa(k.Selection.TotalRows), Explanation: "current result table"},
		{Name: "database", Value: valueOr(k.Database, "not available"), Explanation: "selected source"},
		{Name: "report time", Value: data.GeneratedAt.Local().Format("15:04:05"), Explanation: data.GeneratedAt.Local().Format("2006-01-02")},
	})
	r.paragraph("This report is written as a reproducibility and review artifact. File names, row counts, selected/unselected status, source context, export settings, timing observations, quality checks, and technical file details are recorded from structured report data. Missing fields are reported explicitly rather than filled by a later lookup.", 9.5, colorText, pageWidth-marginLeft-marginRight)
	r.note("Data source: The report is written only from the completed workflow/export state. No external biological data acquisition is performed solely to prepare the PDF.")

	renderGeneratedFileIndex(r, data.Files)
	renderRuntimeContext(r, data)
	renderTiming(r, data)
	renderDataSource(r, data)
	renderMatching(r, k)
	renderLabels(r, k)
	renderSelection(r, k)
	renderProvenanceAndQuality(r, k)
	renderColumns(r, k)
	renderExportLog(r, k)
	renderSequenceAudit(r, k)
	renderFileAppendix(r, data.Files)
}

func renderGeneratedFileIndex(r *pdfReportRenderer, files []GeneratedFile) {
	r.chapterHeading("Chapter 2. Generated File Index")
	r.paragraph("The generated-file index lists the local artifacts produced by the current export action. Long technical identifiers are deliberately moved to the file details appendix so the opening index remains readable while still preserving full audit traceability.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	if len(files) == 0 {
		r.note("No generated data files were recorded for this export action. A completed export normally records at least the report PDF and any requested data files.")
		return
	}
	rows := make([][]string, 0, len(files))
	for _, file := range files {
		rows = append(rows, []string{
			file.Name,
			file.Type,
			fileSizeText(file.SizeBytes),
			file.Role,
			file.Path,
			"See Chapter 13",
		})
	}
	r.table([]string{"File name", "Type", "Size", "Role", "Location", "Technical details"}, rows, []float64{86, 62, 48, 104, 160, 47})
	r.note("Hash values are intentionally omitted from this index because they are long technical values. Full hashes and file attributes appear in Chapter 13.")
}

func renderRuntimeContext(r *pdfReportRenderer, data ReportData) {
	r.chapterHeading("Chapter 3. Software, User, Runtime, And System Context")
	r.paragraph("This chapter records the software identity and the local execution context available to the program. These values help a reviewer distinguish the exact tool build, user session, executable location, output location, operating system, architecture, and terminal environment used for the export.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	sw := data.Software
	r.subheading("Software")
	r.paragraph("Software fields are runtime identity values embedded in the executable or supplied by the Go runtime.", 8.6, colorMuted, pageWidth-marginLeft-marginRight)
	r.table([]string{"Field", "Value"}, availableValueRows([][]string{
		{"Software name", sw.Name},
		{"Author", sw.Author},
		{"Repository", sw.Repository},
		{"Version", sw.Version},
		{"Go runtime", sw.GoVersion},
	}), []float64{118, 389})
	r.subheading("User session and runtime paths")
	session := data.UserSession
	r.paragraph("Session fields identify the operating-system account, host, executable, working location, output location, cache location, and terminal environment visible to the process.", 8.6, colorMuted, pageWidth-marginLeft-marginRight)
	r.table([]string{"Field", "Value"}, availableValueRows([][]string{
		{"User name", session.UserName},
		{"Home directory", session.HomeDir},
		{"Session name", session.SessionName},
		{"Host name", session.HostName},
		{"Process ID", fmt.Sprintf("%d", session.ProcessID)},
		{"Executable", session.ExecutablePath},
		{"Working directory", session.WorkingDir},
		{"Application directory", session.AppDir},
		{"Output directory", session.OutputDir},
		{"Cache directory", session.CacheDir},
		{"Terminal", session.Terminal},
		{"Terminal detail", session.TerminalDetail},
	}), []float64{118, 389})
	r.subheading("System")
	sys := data.System
	r.paragraph("System fields are local runtime facts captured without launching extra database or enrichment work.", 8.6, colorMuted, pageWidth-marginLeft-marginRight)
	r.table([]string{"Field", "Value"}, availableValueRows([][]string{
		{"Operating system", sys.OS},
		{"OS version", sys.OSVersion},
		{"Architecture", sys.Architecture},
		{"CPU count", fmt.Sprintf("%d", sys.CPUCount)},
		{"Memory", sys.Memory},
	}), []float64{118, 389})
	r.note("System context shows only fields captured by the workflow/runtime without report-only probing. Uncaptured operating-system details are omitted from the table rather than counted as data quality problems.")
}

func renderTiming(r *pdfReportRenderer, data ReportData) {
	r.chapterHeading("Chapter 4. Time Window And Performance Overview")
	r.paragraph("The report time window starts at the keyword query and ends when the report PDF is rendered. Search, review, export, and report-rendering moments are separated when the workflow has already measured them. Unmeasured phases are not guessed.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	tw := data.TimeWindow
	rows := [][]string{
		{"Query started", formatTime(tw.QueryStart), "Beginning of current keyword query scope."},
		{"Search ended", formatTime(tw.SearchEnd), "Keyword rows were available to the workflow."},
		{"Review started", formatTime(tw.ReviewStart), "User review and row selection period began."},
		{"Export started", formatTime(tw.ExportStart), "File generation subflow began."},
		{"Export ended", formatTime(tw.ExportEnd), "Requested data files were complete."},
		{"Report rendered", formatTime(tw.ReportRendered), "PDF rendering completed or was scheduled to complete."},
	}
	r.table([]string{"Event", "Timestamp", "Meaning"}, rows, []float64{120, 170, 217})
	r.durationBars(data.Keyword.GenerationSteps)
	r.paragraph(performanceInterpretation(data.Keyword.GenerationSteps), 8.8, colorText, pageWidth-marginLeft-marginRight)
	r.paragraph("The duration visualization is based only on measured generation steps. It is intended to expose performance characteristics such as Excel writing, peptide text handling, hashing, and PDF rendering without mixing those measurements with earlier unrelated application activity.", 8.8, colorMuted, pageWidth-marginLeft-marginRight)
}

func renderDataSource(r *pdfReportRenderer, data ReportData) {
	r.chapterHeading("Chapter 5. Data Source And Species Context")
	r.paragraph("This chapter documents the selected data source and species context for the keyword result table. It records database and species values already present in the export state; it does not attempt to improve or complete taxonomy or release metadata after the export.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	s := data.Keyword.Species
	r.table([]string{"Field", "Value", "Source/meaning"}, availableValueRows([][]string{
		{"Database", data.Keyword.Database, "Selected keyword source."},
		{"Mode", valueOr(data.Mode, "keyword"), "Workflow mode."},
		{"Display label", s.DisplayLabel, "Species display label already used by the program."},
		{"Genome label", s.GenomeLabel, "Genome label from species candidate state."},
		{"Common name", s.CommonName, "Common-name value when available."},
		{"Search alias", s.SearchAlias, "Alternate scientific/search label when available."},
		{"JBrowse name", s.JBrowseName, "Source genome/browser identifier."},
		{"Proteome ID", s.ProteomeID, "Phytozome proteome/target ID when present."},
		{"Release date", s.ReleaseDate, "Release date already known to the workflow."},
		{"Official marker", s.IsOfficial, "lemna official clone marker or not applicable."},
		{"Source notes", s.SourceNotes, "Additional already-known source context."},
	}), []float64{105, 190, 212})
	r.note("Species context shows available fields already present in the selected species/result state. No database query was performed solely to fill omitted fields.")
}

func renderMatching(r *pdfReportRenderer, k KeywordReportData) {
	r.chapterHeading("Chapter 6. Keyword Input And Matching Method")
	r.paragraph("Search terms are listed in the order supplied to the keyword workflow. The matching method text is a method audit: it explains how rows are grouped, how input types were classified when that state is known, and how selected rows become export inputs.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	if len(k.SearchTerms) == 0 {
		r.note("No keyword search terms were available in the completed export state. The report did not reconstruct or infer terms from output files.")
		return
	}
	rows := make([][]string, 0, len(k.SearchTerms))
	for _, term := range k.SearchTerms {
		rows = append(rows, []string{
			term.SearchTerm,
			term.InputType,
			valueOr(term.SearchType, "not recorded"),
			fmt.Sprintf("%d", term.QueryOrder),
			fmt.Sprintf("%d", term.TotalRows),
			fmt.Sprintf("%d", term.SelectedRows),
			term.LabelName,
			term.MatchingNotes,
		})
	}
	r.table([]string{"Search term", "Input type", "Search type", "Order", "Rows", "Selected", "Label", "Notes"}, rows, []float64{70, 48, 62, 28, 30, 38, 48, 183})
	r.subheading("Matching method log")
	method := []string{
		"Input parsing preserved the user's term order and kept each result row traceable to its original search term.",
		"Species binding used the selected database/species context already active in the workflow.",
		"Search execution produced rows before report generation. The report uses the rows already present in the completed export state.",
		"Rows remain grouped by search term. Sorting and display decisions do not move rows across term groups.",
		"The user's row selection determines the selected export set; the raw export, when requested, represents all current rows.",
	}
	for i, step := range method {
		r.paragraph(fmt.Sprintf("Step %d - %s", i+1, step), 8.8, colorText, pageWidth-marginLeft-marginRight)
	}
	r.paragraph("When a term produces no rows, it remains part of the audit trail rather than disappearing from the report. This makes failed, misspelled, overly narrow, or release-specific searches visible during later review.", 8.8, colorMuted, pageWidth-marginLeft-marginRight)
}

func renderLabels(r *pdfReportRenderer, k KeywordReportData) {
	r.chapterHeading("Chapter 7. Label Handling And Traceability")
	r.paragraph("Labels make exported files, FASTA headers, and result rows easier to read. This chapter records the final label and the observed source of that label when the completed export state provides it. If the exact source is unavailable, the report states that directly.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	if len(k.LabelTraces) == 0 {
		r.note("No label trace records were supplied. The report therefore omits per-term label provenance instead of inventing a label source.")
		return
	}
	rows := make([][]string, 0, len(k.LabelTraces))
	for _, trace := range k.LabelTraces {
		rows = append(rows, []string{trace.SearchTerm, trace.FinalLabel, trace.SourceField, trace.SourceValue, trace.Method})
	}
	r.table([]string{"Search term", "Final label", "Source field", "Source value", "Method"}, rows, []float64{92, 68, 74, 145, 128})
	r.note("Label method note: manual labels come from the user's label-name input. Auto-identify checks existing row label_name, then the best alias candidate, then gene/transcript/sequence identifiers. In lemna mode, the Arabidopsis/Phytozome label fallback is reported only if it already ran during the actual label step; the report never runs it.")
	r.paragraph("This label trace is deliberately conservative. It records the final label that affected exported rows and the best already-known source for that label. It does not treat a polished label as proof of biological identity; biological interpretation remains tied to the result rows, source annotations, and user selection.", 8.8, colorMuted, pageWidth-marginLeft-marginRight)
}

func renderSelection(r *pdfReportRenderer, k KeywordReportData) {
	r.chapterHeading("Chapter 8. Result Set And User Selection Analytics")
	r.paragraph("This is the main quantity overview for the keyword result table. It separates all current rows from rows selected for export and then shows whether selection was concentrated in specific search terms.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	s := k.Selection
	r.cards([]NameValue{
		{Name: "total rows", Value: strconv.Itoa(s.TotalRows), Explanation: "all current rows"},
		{Name: "selected", Value: strconv.Itoa(s.SelectedRows), Explanation: "exported rows"},
		{Name: "unselected", Value: strconv.Itoa(s.UnselectedRows), Explanation: "left out"},
		{Name: "search terms", Value: strconv.Itoa(s.SearchTerms), Explanation: "input groups"},
		{Name: "terms with hits", Value: strconv.Itoa(s.TermsWithHits), Explanation: "non-empty groups"},
		{Name: "zero-hit terms", Value: strconv.Itoa(s.TermsZeroHits), Explanation: "empty groups"},
	})
	r.selectionChart(s)
	r.termHitChart(s)
	r.termBars(k.SearchTerms)
	r.paragraph(selectionInterpretation(s), 8.8, colorText, pageWidth-marginLeft-marginRight)
	rows := make([][]string, 0, len(k.SearchTerms))
	for _, term := range k.SearchTerms {
		unselected := term.TotalRows - term.SelectedRows
		percent := "0.0%"
		if term.TotalRows > 0 {
			percent = fmt.Sprintf("%.1f%%", float64(term.SelectedRows)/float64(term.TotalRows)*100)
		}
		rows = append(rows, []string{term.SearchTerm, fmt.Sprintf("%d", term.TotalRows), fmt.Sprintf("%d", term.SelectedRows), fmt.Sprintf("%d", unselected), percent})
	}
	r.table([]string{"Search term", "Total rows", "Selected", "Unselected", "Selected %"}, rows, []float64{150, 70, 70, 78, 70})
}

func renderProvenanceAndQuality(r *pdfReportRenderer, k KeywordReportData) {
	r.chapterHeading("Chapter 9. Data Provenance And Quality Checks")
	r.paragraph("This chapter makes the data origin and quality checks inspectable. It does not create a hidden global score. Each check shows its own rule, status, count, and source so a reviewer can see exactly what was evaluated.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	r.paragraph("The provenance chart is a field-level accounting of values already present in the export state. A fallback value is reported only when the actual keyword workflow already used or produced it; report generation never runs a database query to improve this distribution.", 8.8, colorText, pageWidth-marginLeft-marginRight)
	r.provenanceChart(k.Provenance)
	r.table([]string{"Provenance category", "Count", "Explanation"}, sortedProvenanceRows(k.Provenance), []float64{132, 55, 320})
	r.subheading("Generated table completeness")
	r.tableCompletenessChart(k.ColumnCompleteness)
	columnRows := make([][]string, 0, len(k.ColumnCompleteness))
	for _, col := range k.ColumnCompleteness {
		columnRows = append(columnRows, []string{col.Column, fmt.Sprintf("%d", col.FilledRows), fmt.Sprintf("%d", col.EmptyRows), col.FilledRatio, col.Source})
	}
	r.table([]string{"Column", "Filled rows", "Empty rows", "Filled %", "Source"}, columnRows, []float64{124, 65, 65, 60, 193})
	r.paragraph(completenessInterpretation(k.ColumnCompleteness), 8.8, colorText, pageWidth-marginLeft-marginRight)
	r.subheading("Quality checks")
	r.paragraph("Quality checks below are derived from the generated keyword table rows and export settings. Repeated explanations are consolidated here: warning means a reviewer should inspect the corresponding column or export step; pass means the check's visible rule was satisfied.", 8.6, colorMuted, pageWidth-marginLeft-marginRight)
	rows := make([][]string, 0, len(k.QualityChecks))
	for _, check := range k.QualityChecks {
		rows = append(rows, []string{check.Name, check.Result, check.Count, check.Rule, check.Source})
	}
	r.table([]string{"Check", "Result", "Count", "Rule", "Source"}, rows, []float64{128, 56, 70, 150, 103})
}

func renderColumns(r *pdfReportRenderer, k KeywordReportData) {
	r.chapterHeading("Chapter 10. Column Dictionary And Data Lineage")
	r.paragraph("This chapter explains the columns of the generated keyword workbook in two layers. The first layer uses the same English-facing column guidance already shown by the program UI. The second layer records technical lineage, blank-value meaning, and whether a column participates in report statistics.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	if len(k.Columns) == 0 {
		r.note("No keyword workbook column metadata was available in the export state, so the report cannot explain generated columns for this run.")
		return
	}
	r.subheading("English column descriptions")
	r.paragraph("These descriptions are aligned with the program's existing column-help text so the PDF uses the same field meaning that the interactive table already exposes to the user.", 8.6, colorMuted, pageWidth-marginLeft-marginRight)
	descriptionRows := make([][]string, 0, len(k.Columns))
	for _, col := range k.Columns {
		descriptionRows = append(descriptionRows, []string{col.Column, valueOr(col.EnglishDetail, col.Meaning)})
	}
	r.table([]string{"Column", "English explanation"}, descriptionRows, []float64{105, 402})
	r.subheading("Chinese and Japanese column descriptions")
	triRows := make([][]string, 0, len(k.Columns))
	for _, col := range k.Columns {
		triRows = append(triRows, []string{
			col.Column,
			valueOr(col.ChineseDetail, "not available in this run"),
			valueOr(col.JapaneseDetail, "not available in this run"),
		})
	}
	r.table([]string{"Column", "中文说明", "日本語説明"}, triRows, []float64{92, 208, 207})
}

func renderExportLog(r *pdfReportRenderer, k KeywordReportData) {
	r.chapterHeading("Chapter 11. Export Settings And Generation Log")
	r.paragraph("Export settings show what the user requested for the current file-generation action. The generation log then records measured file-writing and verification steps with timestamps, duration, status, and details.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	r.paragraph("A single export action can generate several artifacts, such as a selected workbook, raw workbook, peptide text file, and this report. The report treats that whole action as one audited event instead of creating separate reports for each file.", 8.8, colorText, pageWidth-marginLeft-marginRight)
	r.subheading("Export settings")
	r.table([]string{"Setting", "Value", "Effect"}, sortedRowsFromNameValues(k.ExportSettings), []float64{140, 122, 245})
	r.subheading("Generation log")
	rows := make([][]string, 0, len(k.GenerationSteps))
	for _, step := range k.GenerationSteps {
		rows = append(rows, []string{
			step.Name,
			formatTime(step.Start),
			formatTime(step.End),
			formatDurationMS(stepDuration(step)),
			step.Status,
			step.Details,
		})
	}
	r.table([]string{"Step", "Start", "End", "Duration", "Status", "Details"}, rows, []float64{89, 91, 91, 55, 46, 135})
	r.durationBars(k.GenerationSteps)
	r.paragraph(performanceInterpretation(k.GenerationSteps), 8.8, colorMuted, pageWidth-marginLeft-marginRight)
}

func renderSequenceAudit(r *pdfReportRenderer, k KeywordReportData) {
	r.chapterHeading("Chapter 12. Sequence Export Audit")
	if !k.Sequences.Requested {
		r.paragraph("Peptide text export was not requested for this export action, so sequence-export details are not applicable. The report does not fetch sequences merely to populate this chapter.", 9.2, colorText, pageWidth-marginLeft-marginRight)
		return
	}
	r.paragraph("The sequence audit records peptide records already fetched or available during the export. Completeness is stated as written, skipped, or unavailable based on the sequence status captured by the workflow.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	seq := k.Sequences
	r.subheading("Text file format")
	r.table([]string{"Field", "Value"}, [][]string{
		{"Text file type", valueOr(seq.TextFileType, "FASTA-style peptide sequence text export")},
		{"Header label mode", valueOr(seq.HeaderLabelMode, "not available in this run")},
	}, []float64{118, 389})
	r.paragraph("The header-label mode states whether the exported FASTA headers carried label_name values, transcript IDs, sequence IDs, or a mixture of those forms. This is especially important when different keyword searches produce different header readability.", 8.8, colorMuted, pageWidth-marginLeft-marginRight)
	r.cards([]NameValue{
		{Name: "requested", Value: strconv.Itoa(seq.RequestedCount), Explanation: "selected rows needing sequence"},
		{Name: "written", Value: strconv.Itoa(seq.WrittenCount), Explanation: "records in text file"},
		{Name: "skipped", Value: strconv.Itoa(seq.SkippedCount), Explanation: "missing or failed"},
		{Name: "aa characters", Value: strconv.Itoa(seq.TotalCharacters), Explanation: "sum of written lengths"},
	})
	r.sequenceChart(seq)
	estimatedRows := math.Max(1, float64(len(seq.QuerySummaries)))
	if r.y+110+estimatedRows*26 > pageHeight-marginBottom {
		r.addPage(r.chapter)
	}
	r.subheading("Per-query sequence summary")
	rows := make([][]string, 0, len(seq.QuerySummaries))
	for _, summary := range seq.QuerySummaries {
		rows = append(rows, []string{
			summary.QueryLabel,
			summary.QueryKind,
			fmt.Sprintf("%d", summary.RequestedCount),
			fmt.Sprintf("%d", summary.WrittenCount),
			fmt.Sprintf("%d", summary.SkippedCount),
			fmt.Sprintf("%d aa", summary.AverageLength),
			fmt.Sprintf("%d-%d aa", summary.MinLength, summary.MaxLength),
			summary.SourceSummary,
		})
	}
	r.table([]string{"Query", "Record class", "Requested", "Written", "Skipped", "Average", "Range", "Source summary"}, rows, []float64{60, 82, 40, 40, 40, 46, 56, 191})
	r.sequenceLengthDotPlot(seq.QuerySummaries, "Per-query length ranges")
}

func renderFileAppendix(r *pdfReportRenderer, files []GeneratedFile) {
	r.chapterHeading("Chapter 13. File Technical Details Appendix")
	r.paragraph("This appendix contains full file metadata and hash values for generated artifacts. Hashes identify byte content at the time the report inspected the files; editing a file after report generation changes its hash.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	for _, file := range files {
		r.subheading(file.Name)
		r.table([]string{"Field", "Value"}, [][]string{
			{"File name", file.Name},
			{"Full path", file.Path},
			{"Type", file.Type},
			{"Role", file.Role},
			{"Size", fmt.Sprintf("%d bytes (%s)", file.SizeBytes, fileSizeText(file.SizeBytes))},
			{"Created time", formatTime(file.CreatedAt)},
			{"Modified time", formatTime(file.ModifiedAt)},
			{"Accessed time", formatTime(file.AccessedAt)},
			{"Permissions", file.Permissions},
			{"Owner", file.Owner},
			{"SHA-256", file.SHA256},
			{"SHA-1", file.SHA1},
			{"MD5", file.MD5},
			{"Hash captured at", formatTime(file.HashCaptured)},
		}, []float64{120, 387})
	}
	r.note("A PDF cannot reliably contain its own final hash inside itself without changing its byte content. A final PDF self-hash should be stored in an external manifest if that audit requirement is added later.")
}

func valueOr(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func availableValueRows(rows [][]string) [][]string {
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		if len(row) < 2 || reportValueAvailable(row[1]) {
			out = append(out, row)
		}
	}
	return out
}

func reportValueAvailable(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	return !strings.HasPrefix(value, "not available") && !strings.HasPrefix(value, "not applicable")
}

func performanceInterpretation(steps []GenerationStep) string {
	if len(steps) == 0 {
		return "No measured generation steps were supplied, so the report does not describe performance distribution for this run."
	}
	total := int64(0)
	longest := GenerationStep{}
	for _, step := range steps {
		duration := stepDuration(step)
		total += duration
		if duration > stepDuration(longest) {
			longest = step
		}
	}
	if total <= 0 {
		return "Generation steps were present, but their measured durations were zero or unavailable. The report records the step log without inventing timing values."
	}
	share := float64(stepDuration(longest)) / float64(total) * 100
	return fmt.Sprintf("The longest measured generation step was %q at %s, accounting for %.1f%% of measured generation time. This is an operational timing observation only; it does not imply a biological data-quality issue.", longest.Name, formatDurationMS(stepDuration(longest)), share)
}

func selectionInterpretation(s KeywordSelectionStats) string {
	if s.TotalRows <= 0 {
		return "No result rows were present in the current keyword table, so selection statistics are limited to the recorded search-term counts."
	}
	selectedPct := float64(s.SelectedRows) / float64(s.TotalRows) * 100
	if s.SearchTerms > 0 && s.TermsZeroHits > 0 {
		return fmt.Sprintf("%.1f%% of current result rows were selected for export. %d of %d search terms had no returned rows, which is preserved in the report as a search outcome rather than treated as missing report data.", selectedPct, s.TermsZeroHits, s.SearchTerms)
	}
	return fmt.Sprintf("%.1f%% of current result rows were selected for export. All recorded search terms that reached the result table had at least one returned row.", selectedPct)
}

func completenessInterpretation(columns []ColumnCompleteness) string {
	if len(columns) == 0 {
		return "No generated table columns were supplied, so table completeness was not calculated."
	}
	totalCells := 0
	emptyCells := 0
	mostEmpty := ColumnCompleteness{}
	for _, col := range columns {
		totalCells += col.FilledRows + col.EmptyRows
		emptyCells += col.EmptyRows
		if col.EmptyRows > mostEmpty.EmptyRows {
			mostEmpty = col
		}
	}
	if totalCells <= 0 {
		return "Generated table columns were present, but no row-level cells were counted."
	}
	emptyPct := float64(emptyCells) / float64(totalCells) * 100
	if mostEmpty.EmptyRows > 0 {
		return fmt.Sprintf("Across generated keyword-table cells, %.1f%% were empty. The column with the highest empty count was %q (%d empty cells), so reviewers should interpret blanks according to that column's source and blank-meaning rules.", emptyPct, mostEmpty.Column, mostEmpty.EmptyRows)
	}
	return "Every counted generated keyword-table cell contained data. This statement applies only to the exported keyword table columns, not to unrelated runtime metadata."
}

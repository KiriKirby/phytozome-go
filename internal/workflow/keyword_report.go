package workflow

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/perf"
	"github.com/KiriKirby/phytozome-go/internal/prompt"
	"github.com/KiriKirby/phytozome-go/internal/report"
	"github.com/KiriKirby/phytozome-go/internal/tui"
)

func (w *BlastWizard) renderKeywordReportForExport(ctx context.Context, rows []model.KeywordResultRow, allRows []model.KeywordResultRow, groups []model.KeywordSearchGroup, files exportFileResult, baseName string, outputDir string, settings exportSettings, runCtx *keywordReportRunContext, exportStarted time.Time, steps []report.GenerationStep, sequenceAudit report.SequenceAudit, sequenceRecords []model.ProteinSequenceRecord) (string, error) {
	_ = ctx
	_ = sequenceRecords

	metadataStart := time.Now()
	generatedFiles, err := inspectKeywordGeneratedFiles(ctx, files, sequenceAudit)
	if err != nil {
		return "", err
	}
	steps = append(steps, keywordReportStep("Capture file metadata and hashes", metadataStart, time.Now(), "ok", fmt.Sprintf("%d generated data files inspected", len(generatedFiles))))

	reportGeneratedAt := time.Now()
	reportBaseName := reportBaseNameForExport(baseName, outputDir, baseName)
	reportPath := filepath.Join(outputDir, report.ReportFileNameForBase(reportBaseName, reportGeneratedAt))
	generatedFiles = append(generatedFiles, report.PlannedReportFile(reportPath, reportGeneratedAt))

	renderStart := time.Now()
	reportSteps := append([]report.GenerationStep(nil), steps...)
	reportSteps = append(reportSteps, keywordReportStep("Render report PDF", renderStart, renderStart, "ok", "this step writes the PDF currently being read; final PDF self-hash is intentionally not embedded"))
	data := w.buildKeywordReportData(rows, allRows, groups, generatedFiles, reportBaseName, outputDir, settings, runCtx, exportStarted, reportGeneratedAt, reportSteps, sequenceAudit)

	err = tui.RunTaskPageContext(tui.TaskPage{
		Path:        w.tuiPath("Export", "Data analysis report"),
		Title:       "Writing Data Analysis Report",
		Description: "Rendering the keyword-mode PDF report from already collected export data.",
		Initial:     "Writing Data Analysis Report PDF...",
		CancelError: prompt.ErrBackToRowSelection,
	}, func(taskCtx context.Context, update func(string)) error {
		_ = taskCtx
		safeTaskUpdate(update)("Writing Data Analysis Report PDF...")
		return report.RenderKeywordPDF(reportPath, data)
	})
	if err != nil {
		return "", err
	}
	return reportPath, nil
}

func (w *BlastWizard) buildKeywordReportData(rows []model.KeywordResultRow, allRows []model.KeywordResultRow, groups []model.KeywordSearchGroup, files []report.GeneratedFile, baseName string, outputDir string, settings exportSettings, runCtx *keywordReportRunContext, exportStarted time.Time, reportGeneratedAt time.Time, steps []report.GenerationStep, sequenceAudit report.SequenceAudit) report.ReportData {
	selected := model.SpeciesCandidate{}
	queryStarted := time.Time{}
	searchEnded := keywordGroupsSearchEndedAt(groups)
	reviewStarted := time.Time{}
	labelMode := keywordGroupLabelMode(groups)
	if runCtx != nil {
		selected = runCtx.Selected
		queryStarted = runCtx.QueryStarted
		if !runCtx.SearchEnded.IsZero() {
			searchEnded = runCtx.SearchEnded
		}
		reviewStarted = runCtx.ReviewStarted
		if strings.TrimSpace(runCtx.LabelMode) != "" {
			labelMode = runCtx.LabelMode
		}
	}
	if selected.DisplayLabel() == "" {
		selected = inferKeywordSpeciesFromRows(allRows)
	}
	return report.ReportData{
		Title:       "Keyword Data Analysis Report",
		Mode:        "keyword",
		GeneratedAt: reportGeneratedAt,
		Software:    w.reportSoftwareInfo(),
		UserSession: reportUserSession(outputDir),
		System: report.SystemInfo{
			OS:           report.PlatformDisplayName(runtime.GOOS),
			OSVersion:    "not available in this run; no OS-specific probe was performed for report generation",
			Architecture: runtime.GOARCH,
			CPUCount:     runtime.NumCPU(),
			Memory:       "not available in this run; memory details are not currently captured by the workflow",
		},
		TimeWindow: report.TimeWindow{
			QueryStart:     queryStarted,
			SearchEnd:      searchEnded,
			ReviewStart:    reviewStarted,
			ExportStart:    exportStarted,
			ExportEnd:      reportGeneratedAt,
			ReportRendered: reportGeneratedAt,
		},
		Files: files,
		Keyword: report.KeywordReportData{
			Database:           databaseDisplayName(w.source.Name()),
			Species:            reportSpecies(selected, w.source.Name()),
			SearchTerms:        keywordTermReports(groups, rows),
			LabelTraces:        keywordLabelReports(groups, labelMode),
			Selection:          keywordSelectionStats(groups, rows, len(files)),
			Provenance:         keywordProvenance(allRows),
			ColumnCompleteness: keywordColumnCompleteness(rows),
			QualityChecks:      keywordQualityChecks(groups, rows, allRows, settings, sequenceAudit),
			Columns:            keywordColumnLineage(allRows, sourceDatabaseForKeywordRows(allRows), keywordRowsHaveProteinIDForReport(allRows), keywordReportExtraHeaders(allRows)),
			ExportSettings:     keywordExportSettings(baseName, outputDir, settings),
			GenerationSteps:    steps,
			Sequences:          sequenceAudit,
		},
	}
}

func inspectKeywordGeneratedFiles(ctx context.Context, files exportFileResult, sequenceAudit report.SequenceAudit) ([]report.GeneratedFile, error) {
	type fileSpec struct {
		path string
		typ  string
		role string
	}
	specs := []fileSpec{
		{files.ExcelPath, "selected Excel", "selected keyword rows exported as the main workbook"},
		{files.RawExcelPath, "raw Excel", "all current keyword rows exported for audit comparison"},
		{files.RawTextPath, "raw peptide text", "all current keyword peptide sequence records exported for audit comparison"},
		{files.TextPath, "peptide text", keywordTextFileRole(sequenceAudit)},
	}
	out := make([]report.GeneratedFile, 0, len(specs))
	filtered := make([]fileSpec, 0, len(specs))
	for _, spec := range specs {
		if strings.TrimSpace(spec.path) == "" {
			continue
		}
		filtered = append(filtered, spec)
	}
	if len(filtered) == 0 {
		return out, nil
	}
	type inspectResult struct {
		file report.GeneratedFile
		err  error
	}
	results := make([]inspectResult, len(filtered))
	if err := perf.ParallelFor(ctx, perf.WorkDisk, len(filtered), func(_ context.Context, i int) error {
		spec := filtered[i]
		file, err := report.InspectGeneratedFile(spec.path, spec.typ, spec.role, time.Now())
		results[i] = inspectResult{file: file, err: err}
		return err
	}); err != nil {
		return nil, err
	}
	for _, result := range results {
		if result.err != nil {
			return nil, result.err
		}
		out = append(out, result.file)
	}
	return out, nil
}

func keywordTextFileRole(sequenceAudit report.SequenceAudit) string {
	role := "FASTA-style peptide sequence records for selected keyword rows"
	if strings.TrimSpace(sequenceAudit.HeaderLabelMode) != "" {
		role += "; header mode: " + sequenceAudit.HeaderLabelMode
	}
	return role
}

func keywordReportStep(name string, start time.Time, end time.Time, status string, details string) report.GenerationStep {
	return report.GenerationStep{
		Name:       name,
		Start:      start,
		End:        end,
		Status:     status,
		Details:    details,
		DurationMS: end.Sub(start).Milliseconds(),
	}
}

func (w *BlastWizard) reportSoftwareInfo() report.SoftwareInfo {
	return report.SoftwareInfo{
		Name:       firstNonEmpty(w.tuiInfo.DisplayName, "phytozome GO"),
		Author:     firstNonEmpty(w.tuiInfo.Author, "wangsychn"),
		Repository: firstNonEmpty(w.tuiInfo.RepoURL, "https://github.com/KiriKirby/phytozome-go"),
		Version:    firstNonEmpty(w.tuiInfo.Version, "dev"),
		GoVersion:  runtime.Version(),
	}
}

func reportUserSession(outputDir string) report.UserSessionInfo {
	wd, _ := os.Getwd()
	exe, _ := os.Executable()
	usr, _ := user.Current()
	host, _ := os.Hostname()
	appDir, appErr := appfs.ApplicationDir()
	if appErr != nil {
		appDir = "not available in this run"
	}
	cacheDir, cacheErr := appfs.CacheDir()
	if cacheErr != nil {
		cacheDir = "not available in this run"
	}
	userName := firstNonEmpty(os.Getenv("USERNAME"), os.Getenv("USER"))
	homeDir := ""
	if usr != nil {
		if userName == "" {
			userName = usr.Username
		}
		homeDir = usr.HomeDir
	}
	return report.UserSessionInfo{
		UserName:       firstNonEmpty(userName, "not available in this run"),
		HomeDir:        firstNonEmpty(homeDir, "not available in this run"),
		SessionName:    firstEnvForReport("WT_SESSION", "TERM_SESSION_ID", "SESSIONNAME"),
		HostName:       firstNonEmpty(host, "not available in this run"),
		ProcessID:      os.Getpid(),
		ExecutablePath: firstNonEmpty(exe, "not available in this run"),
		WorkingDir:     firstNonEmpty(wd, "not available in this run"),
		AppDir:         appDir,
		OutputDir:      outputDir,
		CacheDir:       cacheDir,
		Terminal:       firstEnvForReport("TERM_PROGRAM", "WT_SESSION", "TERM", "ComSpec"),
		TerminalDetail: strings.Join(nonEmptyReportValues([]string{
			envPairForReport("TERM_PROGRAM_VERSION"),
			envPairForReport("WT_SESSION"),
			envPairForReport("TERM"),
			envPairForReport("SESSIONNAME"),
		}), "; "),
	}
}

func reportSpecies(species model.SpeciesCandidate, sourceName string) report.SpeciesReport {
	return report.SpeciesReport{
		DisplayLabel: firstNonEmpty(species.DisplayLabel(), "not available in this run"),
		GenomeLabel:  firstNonEmpty(species.GenomeLabel, "not available in this run"),
		CommonName:   firstNonEmpty(species.CommonName, "not available in this run"),
		SearchAlias:  firstNonEmpty(species.SearchAlias, "not available in this run"),
		JBrowseName:  firstNonEmpty(species.JBrowseName, "not available in this run"),
		ProteomeID:   reportProteomeID(species),
		ReleaseDate:  firstNonEmpty(species.ReleaseDate, "not available in this run"),
		IsOfficial:   reportBoolApplicability(species.IsOfficial, sourceName),
		SourceNotes:  speciesSourceNotes(species, sourceName),
	}
}

func reportProteomeID(species model.SpeciesCandidate) string {
	if species.ProteomeID == 0 {
		return "not available in this run"
	}
	return fmt.Sprintf("%d", species.ProteomeID)
}

func reportBoolApplicability(value bool, sourceName string) string {
	if strings.EqualFold(strings.TrimSpace(sourceName), "lemna") {
		return fmt.Sprintf("%t", value)
	}
	return "not applicable to this source"
}

func speciesSourceNotes(species model.SpeciesCandidate, sourceName string) string {
	switch strings.ToLower(strings.TrimSpace(sourceName)) {
	case "lemna":
		return "lemna.org keyword rows are generated by resolving the selected release from the download directory, opening the release GFF3 file, scanning searchable GFF3 features, and enriching matched rows with already-loaded AHRD records when available. Typical source addresses are lemna.org download release URLs stored in lemna_gff_url and gene_report_url."
	case "phytozome":
		return "Phytozome keyword rows are generated through the normal source client path. Report URLs are parsed as /report/gene, /report/transcript, or /report/protein; specific identifiers are tried through gene, transcript, and protein lookup paths; otherwise the Elasticsearch-backed Phytozome keyword endpoint is used with the selected proteome ID. Gene report links use https://phytozome-next.jgi.doe.gov/report/gene/<JBrowse>/<gene>."
	default:
		return "source-specific notes were not available in this run"
	}
}

func inferKeywordSpeciesFromRows(rows []model.KeywordResultRow) model.SpeciesCandidate {
	for _, row := range rows {
		if strings.TrimSpace(row.Genome) != "" {
			return model.SpeciesCandidate{GenomeLabel: row.Genome}
		}
	}
	return model.SpeciesCandidate{}
}

func keywordTermReports(groups []model.KeywordSearchGroup, selectedRows []model.KeywordResultRow) []report.KeywordTermReport {
	selectedCounts := keywordSelectedCountsByTerm(selectedRows)
	out := make([]report.KeywordTermReport, 0, len(groups))
	for i, group := range groups {
		selected := selectedCounts[group.SearchTerm]
		out = append(out, report.KeywordTermReport{
			SearchTerm:     group.SearchTerm,
			InputType:      classifyKeywordInputType(group.SearchTerm),
			QueryOrder:     i + 1,
			TotalRows:      len(group.Rows),
			SelectedRows:   selected,
			LabelName:      firstNonEmpty(group.LabelName, "not available in this run"),
			MatchingNotes:  keywordMatchingNotes(group),
			DurationMillis: group.SearchDurationMS,
		})
	}
	return out
}

func keywordSelectedCountsByTerm(rows []model.KeywordResultRow) map[string]int {
	counts := make(map[string]int)
	for _, row := range rows {
		counts[row.SearchTerm]++
	}
	return counts
}

func classifyKeywordInputType(term string) string {
	value := strings.TrimSpace(term)
	lower := strings.ToLower(value)
	switch {
	case value == "":
		return "unknown/unclassified"
	case strings.Contains(lower, "phytozome-next.jgi.doe.gov/report/gene/"):
		return "report URL: gene"
	case strings.Contains(lower, "phytozome-next.jgi.doe.gov/report/transcript/"):
		return "report URL: transcript"
	case strings.Contains(lower, "phytozome-next.jgi.doe.gov/report/protein/"):
		return "report URL: protein"
	case strings.Contains(lower, "://") || strings.Contains(lower, "/report/"):
		return "report URL"
	case looksLikeTranscriptID(value):
		return "transcript ID"
	case looksLikeProteinOrGeneID(value):
		return "identifier-like term"
	default:
		return "plain keyword"
	}
}

func looksLikeTranscriptID(value string) bool {
	return strings.Contains(value, ".") && looksLikeProteinOrGeneID(strings.ReplaceAll(value, ".", ""))
}

func looksLikeProteinOrGeneID(value string) bool {
	if len(value) < 3 || strings.ContainsAny(value, " \t\r\n") {
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
		case r == '_' || r == '-' || r == ':':
		default:
			return false
		}
	}
	return hasLetter && hasDigit
}

func keywordMatchingNotes(group model.KeywordSearchGroup) string {
	source := keywordRowsSourceDatabase(group.Rows)
	duration := "not separately timed"
	if group.SearchDurationMS > 0 {
		duration = fmt.Sprintf("%d ms", group.SearchDurationMS)
	}
	switch strings.ToLower(source) {
	case "lemna":
		return "lemna workflow: selected species -> releaseForSpecies -> release GFF3 URL -> scan searchable GFF3 rows -> rowMatchesTerms -> optional AHRD enrichment from the same release when already available -> cache write; term duration: " + duration
	case "phytozome":
		return "Phytozome workflow: parse report URL if present -> try gene/transcript/protein identifier variants for specific IDs -> fallback to SearchGenesByKeyword(proteomeID, term, limit=20) only when no specific match was found -> build rows from returned gene/transcript records -> cache write; term duration: " + duration
	default:
		return "rows came from current keyword result state; term duration: " + duration
	}
}

func keywordRowsSourceDatabase(rows []model.KeywordResultRow) string {
	for _, row := range rows {
		if strings.TrimSpace(row.SourceDatabase) != "" {
			return strings.TrimSpace(row.SourceDatabase)
		}
	}
	return ""
}

func keywordLabelReports(groups []model.KeywordSearchGroup, defaultMethod string) []report.KeywordLabelReport {
	out := make([]report.KeywordLabelReport, 0, len(groups))
	for _, group := range groups {
		method := firstNonEmpty(group.LabelMethod, defaultMethod, "not recorded in this run")
		sourceField, sourceValue := labelSourceForGroup(group, method)
		out = append(out, report.KeywordLabelReport{
			SearchTerm:  group.SearchTerm,
			FinalLabel:  firstNonEmpty(group.LabelName, "not available in this run"),
			SourceField: sourceField,
			SourceValue: sourceValue,
			Method:      method,
			Explanation: labelTraceExplanation(method),
		})
	}
	return out
}

func labelSourceForGroup(group model.KeywordSearchGroup, method string) (string, string) {
	if strings.TrimSpace(group.LabelSourceField) != "" || strings.TrimSpace(group.LabelSourceValue) != "" {
		return firstNonEmpty(group.LabelSourceField, "not available in this run"), firstNonEmpty(group.LabelSourceValue, "not available in this run")
	}
	if strings.Contains(strings.ToLower(method), "manual") {
		return "user input", firstNonEmpty(group.LabelName, "blank label intentionally allowed")
	}
	for _, row := range group.Rows {
		if label := rowKeywordLabelName(row); label != "" {
			return "label_name", label
		}
	}
	for _, row := range group.Rows {
		if label := bestAlias(row.Aliases); label != "" {
			return "alias", label
		}
	}
	for _, row := range group.Rows {
		if label := firstNonEmpty(row.GeneIdentifier, row.TranscriptID, row.SequenceID); label != "" {
			return "identifier", label
		}
	}
	return "not available in this run", "not available in this run"
}

func labelTraceExplanation(method string) string {
	if strings.Contains(strings.ToLower(method), "auto") {
		return "The final label was produced during the real label-identification step before export; the report did not run fallback searches."
	}
	if strings.Contains(strings.ToLower(method), "manual") {
		return "The final label came from the user's label-name input and was applied to result rows before export."
	}
	return "The workflow did not record detailed label provenance for this run."
}

func keywordSelectionStats(groups []model.KeywordSearchGroup, selectedRows []model.KeywordResultRow, generatedFiles int) report.KeywordSelectionStats {
	total := countKeywordRows(groups)
	selected := len(selectedRows)
	withHits := 0
	zeroHits := 0
	for _, group := range groups {
		if len(group.Rows) == 0 {
			zeroHits++
		} else {
			withHits++
		}
	}
	return report.KeywordSelectionStats{
		TotalRows:      total,
		SelectedRows:   selected,
		UnselectedRows: total - selected,
		SearchTerms:    len(groups),
		TermsWithHits:  withHits,
		TermsZeroHits:  zeroHits,
		GeneratedFiles: generatedFiles,
	}
}

func keywordProvenance(rows []model.KeywordResultRow) []report.ProvenanceSlice {
	counts := map[string]int{}
	for _, row := range rows {
		source := strings.ToLower(strings.TrimSpace(row.SourceDatabase))
		for _, value := range keywordReportSourceValues(row) {
			if strings.TrimSpace(value) == "" {
				counts["unavailable/missing"]++
				continue
			}
			switch source {
			case "lemna":
				counts["local release parsed"]++
			case "phytozome":
				counts["direct source result"]++
			default:
				counts["direct source result"]++
			}
		}
		counts["generated/internal"] += 2
		for _, value := range row.ExtraColumns {
			if strings.TrimSpace(value) == "" {
				counts["unavailable/missing"]++
			} else if source == "lemna" {
				counts["local release parsed"]++
			} else {
				counts["direct source result"]++
			}
		}
	}
	labels := []string{"direct source result", "local release parsed", "generated/internal", "unavailable/missing"}
	out := make([]report.ProvenanceSlice, 0, len(labels))
	for _, label := range labels {
		count := counts[label]
		if count == 0 {
			continue
		}
		out = append(out, report.ProvenanceSlice{Label: label, Count: count, Explanation: provenanceExplanation(label)})
	}
	return out
}

func keywordReportSourceValues(row model.KeywordResultRow) []string {
	return []string{
		row.ProteinID,
		row.TranscriptID,
		row.GeneIdentifier,
		row.Genome,
		row.Location,
		row.Aliases,
		row.UniProt,
		row.Description,
		row.Comments,
		row.AutoDefine,
		row.GeneReportURL,
		row.SequenceID,
	}
}

func provenanceExplanation(label string) string {
	switch label {
	case "direct source result":
		return "Values present in current result rows returned by the selected source workflow."
	case "local release parsed":
		return "Values present in current rows parsed from already-loaded lemna GFF3/AHRD/release assets."
	case "generated/internal":
		return "Values generated by phytozome GO, such as row numbers, search-term grouping, and labels."
	case "unavailable/missing":
		return "Fields blank or not collected in the current run; the report did not fill them with extra lookups."
	default:
		return "Provenance category captured from the completed export state."
	}
}

func keywordQualityChecks(groups []model.KeywordSearchGroup, rows []model.KeywordResultRow, allRows []model.KeywordResultRow, settings exportSettings, sequenceAudit report.SequenceAudit) []report.QualityCheck {
	tableRows := rows
	totalRows := len(tableRows)
	selectedRows := len(rows)
	zeroHitTerms := 0
	for _, group := range groups {
		if len(group.Rows) == 0 {
			zeroHitTerms++
		}
	}
	checks := []report.QualityCheck{
		qualityCheck("Search terms with zero hits", zeroHitTerms == 0, fmt.Sprintf("%d of %d", zeroHitTerms, len(groups)), "warn when count > 0", "Zero-hit terms are retained for traceability.", "search groups"),
		qualityCheck("Generated table rows missing label", countKeywordRowsWhere(tableRows, func(row model.KeywordResultRow) bool { return strings.TrimSpace(row.LabelName) == "" }) == 0, fmt.Sprintf("%d of %d", countKeywordRowsWhere(tableRows, func(row model.KeywordResultRow) bool { return strings.TrimSpace(row.LabelName) == "" }), selectedRows), "warn when any exported table row lacks label", "Labels affect row readability and sequence headers.", "generated keyword table"),
		qualityCheck("Generated table rows missing transcript ID", countKeywordRowsWhere(tableRows, func(row model.KeywordResultRow) bool { return strings.TrimSpace(row.TranscriptID) == "" }) == 0, fmt.Sprintf("%d of %d", countKeywordRowsWhere(tableRows, func(row model.KeywordResultRow) bool { return strings.TrimSpace(row.TranscriptID) == "" }), totalRows), "warn when any exported table row lacks transcript ID", "Transcript IDs support biological traceability when the source provides them.", "generated keyword table"),
		qualityCheck("Generated table rows missing gene identifier", countKeywordRowsWhere(tableRows, func(row model.KeywordResultRow) bool { return strings.TrimSpace(row.GeneIdentifier) == "" }) == 0, fmt.Sprintf("%d of %d", countKeywordRowsWhere(tableRows, func(row model.KeywordResultRow) bool { return strings.TrimSpace(row.GeneIdentifier) == "" }), totalRows), "warn when any exported table row lacks gene identifier", "Gene identifiers support source-level traceability.", "generated keyword table"),
		qualityCheck("Generated table rows missing description", countKeywordRowsWhere(tableRows, func(row model.KeywordResultRow) bool { return strings.TrimSpace(row.Description) == "" }) == 0, fmt.Sprintf("%d of %d", countKeywordRowsWhere(tableRows, func(row model.KeywordResultRow) bool { return strings.TrimSpace(row.Description) == "" }), totalRows), "warn when any exported table row lacks description", "Descriptions are the main annotation text for keyword-mode review.", "generated keyword table"),
		qualityCheck("Generated table rows missing report URL", countKeywordRowsWhere(tableRows, func(row model.KeywordResultRow) bool { return strings.TrimSpace(row.GeneReportURL) == "" }) == 0, fmt.Sprintf("%d of %d", countKeywordRowsWhere(tableRows, func(row model.KeywordResultRow) bool { return strings.TrimSpace(row.GeneReportURL) == "" }), totalRows), "warn when any exported table row lacks report URL", "Stable source URLs are not always available, especially for release-backed lemna rows.", "generated keyword table"),
		qualityCheck("Duplicate sequence IDs in generated table", duplicateKeywordSequenceIDs(tableRows) == 0, fmt.Sprintf("%d duplicate IDs", duplicateKeywordSequenceIDs(tableRows)), "warn when duplicate non-empty sequence IDs exist", "Duplicates can represent isoforms, repeated annotations, or duplicate source hits.", "generated keyword table"),
	}
	if settings.WriteText {
		checks = append(checks, qualityCheck("Sequence export completeness", sequenceAudit.SkippedCount == 0, fmt.Sprintf("%d written / %d requested", sequenceAudit.WrittenCount, sequenceAudit.RequestedCount), "warn when written < requested", "Sequence export is complete only when each requested selected row produced a sequence record.", "sequence export state"))
	} else {
		checks = append(checks, report.QualityCheck{Name: "Sequence export completeness", Result: "not applicable", Count: "not requested", Rule: "text export was not requested", Explanation: "No sequence fetching was performed for report generation.", Source: "export settings"})
	}
	if settings.WriteRawExcel {
		checks = append(checks, qualityCheck("Raw Excel completeness", true, fmt.Sprintf("%d rows", len(allRows)), "pass when raw export requested and current rows are written", "Raw Excel captures all current keyword rows.", "export settings"))
	} else {
		checks = append(checks, report.QualityCheck{Name: "Raw Excel completeness", Result: "not requested", Count: "not requested", Rule: "raw Excel option disabled", Explanation: "The report still summarizes all current rows available in memory.", Source: "export settings"})
	}
	return checks
}

func keywordColumnCompleteness(rows []model.KeywordResultRow) []report.ColumnCompleteness {
	headers := keywordReportHeaders(rows)
	out := make([]report.ColumnCompleteness, 0, len(headers))
	total := len(rows)
	for _, header := range headers {
		filled := 0
		for i, row := range rows {
			if strings.TrimSpace(keywordReportCellValue(header, row, i)) != "" {
				filled++
			}
		}
		empty := total - filled
		ratio := "0.0%"
		if total > 0 {
			ratio = fmt.Sprintf("%.1f%%", float64(filled)/float64(total)*100)
		}
		source := keywordColumnLineageForHeader(header, rows).Source
		display := prompt.ColumnExportHeader(header, prompt.ColumnDisplayOptions{})
		out = append(out, report.ColumnCompleteness{Column: display, FilledRows: filled, EmptyRows: empty, TotalRows: total, FilledRatio: ratio, Source: source})
	}
	return out
}

func keywordReportCellValue(header string, row model.KeywordResultRow, index int) string {
	switch header {
	case "row":
		return fmt.Sprintf("%d", index+1)
	case "search_term":
		return row.SearchTerm
	case "label_name":
		return row.LabelName
	case "protein_id":
		return row.ProteinID
	case "transcript":
		return row.TranscriptID
	case "gene_identifier":
		return row.GeneIdentifier
	case "genome":
		return row.Genome
	case "location":
		return row.Location
	case "alias":
		return row.Aliases
	case "uniprot":
		return row.UniProt
	case "description":
		return row.Description
	case "comments":
		return row.Comments
	case "auto_define":
		return row.AutoDefine
	case "gene_report_url":
		return row.GeneReportURL
	default:
		if row.ExtraColumns == nil {
			return ""
		}
		return row.ExtraColumns[header]
	}
}

func qualityCheck(name string, pass bool, count string, rule string, explanation string, source string) report.QualityCheck {
	result := "warning"
	if pass {
		result = "pass"
	}
	return report.QualityCheck{Name: name, Result: result, Count: count, Rule: rule, Explanation: explanation, Source: source}
}

func countKeywordRowsWhere(rows []model.KeywordResultRow, fn func(model.KeywordResultRow) bool) int {
	count := 0
	for _, row := range rows {
		if fn(row) {
			count++
		}
	}
	return count
}

func duplicateKeywordSequenceIDs(rows []model.KeywordResultRow) int {
	seen := map[string]int{}
	for _, row := range rows {
		id := strings.TrimSpace(row.SequenceID)
		if id != "" {
			seen[id]++
		}
	}
	duplicates := 0
	for _, count := range seen {
		if count > 1 {
			duplicates += count - 1
		}
	}
	return duplicates
}

func keywordColumnLineage(rows []model.KeywordResultRow, database string, includeProteinID bool, extraHeaders []string) []report.ColumnLineage {
	headers := prompt.KeywordReportColumnIDs(database, includeProteinID, extraHeaders)
	out := make([]report.ColumnLineage, 0, len(headers))
	for _, header := range headers {
		out = append(out, keywordColumnLineageForHeader(header, rows))
	}
	return out
}

func keywordColumnEnglishDetail(header string) string {
	return prompt.ColumnHelpEnglish(strings.TrimSpace(header))
}

func keywordReportHeaders(rows []model.KeywordResultRow) []string {
	return prompt.KeywordReportColumnIDs(sourceDatabaseForKeywordRows(rows), keywordRowsHaveProteinIDForReport(rows), keywordReportExtraHeaders(rows))
}

func sourceDatabaseForKeywordRows(rows []model.KeywordResultRow) string {
	for _, row := range rows {
		if value := strings.TrimSpace(row.SourceDatabase); value != "" {
			return value
		}
	}
	return "phytozome"
}

func keywordRowsHaveProteinIDForReport(rows []model.KeywordResultRow) bool {
	for _, row := range rows {
		if strings.TrimSpace(row.ProteinID) != "" {
			return true
		}
	}
	return false
}

func keywordReportExtraHeaders(rows []model.KeywordResultRow) []string {
	seen := map[string]struct{}{}
	for _, row := range rows {
		for key := range row.ExtraColumns {
			if strings.TrimSpace(key) != "" {
				seen[key] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func keywordColumnLineageForHeader(header string, rows []model.KeywordResultRow) report.ColumnLineage {
	display := prompt.ColumnExportHeader(header, prompt.ColumnDisplayOptions{})
	english := keywordColumnEnglishDetail(header)
	chinese := prompt.ColumnHelpChinese(header)
	japanese := prompt.ColumnHelpJapanese(header)
	switch header {
	case "row":
		return report.ColumnLineage{ID: header, Column: display, Meaning: firstNonEmpty(english, "Stable row number in the exported table."), EnglishDetail: english, ChineseDetail: chinese, JapaneseDetail: japanese, Source: "export workflow", CollectionMethod: "generated during export", BlankMeaning: "not applicable", UsedInStats: "yes"}
	case "search_term":
		return report.ColumnLineage{ID: header, Column: display, Meaning: firstNonEmpty(english, "Original keyword that produced the row."), EnglishDetail: english, ChineseDetail: chinese, JapaneseDetail: japanese, Source: "user input", CollectionMethod: "preserved from keyword query", BlankMeaning: "not expected", UsedInStats: "yes"}
	case "label_name":
		return report.ColumnLineage{ID: header, Column: display, Meaning: firstNonEmpty(english, "Readable label used for row grouping, exports, and sequence headers."), EnglishDetail: english, ChineseDetail: chinese, JapaneseDetail: japanese, Source: "user input or label workflow", CollectionMethod: "captured before or during keyword workflow", BlankMeaning: "user skipped or label unavailable", UsedInStats: "yes"}
	case "protein_id":
		return report.ColumnLineage{ID: header, Column: display, Meaning: firstNonEmpty(english, "Original protein identifier when the source naturally provides one."), EnglishDetail: english, ChineseDetail: chinese, JapaneseDetail: japanese, Source: "selected database", CollectionMethod: "from current result rows", BlankMeaning: "not provided by this source", UsedInStats: "quality"}
	case "transcript":
		return report.ColumnLineage{ID: header, Column: display, Meaning: firstNonEmpty(english, "Transcript identifier for source traceability."), EnglishDetail: english, ChineseDetail: chinese, JapaneseDetail: japanese, Source: "selected database", CollectionMethod: "from current result rows", BlankMeaning: "not provided by source row", UsedInStats: "quality"}
	case "gene_identifier":
		return report.ColumnLineage{ID: header, Column: display, Meaning: firstNonEmpty(english, "Gene identifier or source gene key."), EnglishDetail: english, ChineseDetail: chinese, JapaneseDetail: japanese, Source: "selected database", CollectionMethod: "from current result rows", BlankMeaning: "not provided by source row", UsedInStats: "quality"}
	case "genome":
		return report.ColumnLineage{ID: header, Column: display, Meaning: firstNonEmpty(english, "Genome or release label stored with the row."), EnglishDetail: english, ChineseDetail: chinese, JapaneseDetail: japanese, Source: "species/result row", CollectionMethod: "from current result rows", BlankMeaning: "not collected", UsedInStats: "no"}
	case "location":
		return report.ColumnLineage{ID: header, Column: display, Meaning: firstNonEmpty(english, "Genomic coordinate or location text."), EnglishDetail: english, ChineseDetail: chinese, JapaneseDetail: japanese, Source: "selected database", CollectionMethod: "from current result rows", BlankMeaning: "not provided by source row", UsedInStats: "no"}
	case "alias":
		return report.ColumnLineage{ID: header, Column: display, Meaning: firstNonEmpty(english, "Alias or gene-symbol list."), EnglishDetail: english, ChineseDetail: chinese, JapaneseDetail: japanese, Source: "source annotation", CollectionMethod: "from current result rows", BlankMeaning: "source has no alias", UsedInStats: "label/quality"}
	case "uniprot":
		return report.ColumnLineage{ID: header, Column: display, Meaning: firstNonEmpty(english, "UniProt cross-reference text already present in the row."), EnglishDetail: english, ChineseDetail: chinese, JapaneseDetail: japanese, Source: "source annotation", CollectionMethod: "from current result rows", BlankMeaning: "not provided or not requested", UsedInStats: "quality"}
	case "description":
		return report.ColumnLineage{ID: header, Column: display, Meaning: firstNonEmpty(english, "Annotation description."), EnglishDetail: english, ChineseDetail: chinese, JapaneseDetail: japanese, Source: "source annotation", CollectionMethod: "from current result rows", BlankMeaning: "description unavailable", UsedInStats: "quality"}
	case "comments":
		return report.ColumnLineage{ID: header, Column: display, Meaning: firstNonEmpty(english, "Source comment or parsed annotation notes."), EnglishDetail: english, ChineseDetail: chinese, JapaneseDetail: japanese, Source: "source annotation", CollectionMethod: "from current result rows", BlankMeaning: "no comment available", UsedInStats: "no"}
	case "auto_define":
		return report.ColumnLineage{ID: header, Column: display, Meaning: firstNonEmpty(english, "Automatic definition or parsed name already present."), EnglishDetail: english, ChineseDetail: chinese, JapaneseDetail: japanese, Source: "source/internal", CollectionMethod: "from current result rows", BlankMeaning: "not collected", UsedInStats: "label"}
	case "gene_report_url":
		return report.ColumnLineage{ID: header, Column: display, Meaning: firstNonEmpty(english, "Source report or release URL known to the workflow."), EnglishDetail: english, ChineseDetail: chinese, JapaneseDetail: japanese, Source: "selected database", CollectionMethod: "from current result rows", BlankMeaning: "no stable URL available", UsedInStats: "quality"}
	default:
		return dynamicKeywordColumnLineage(header)
	}
}

func dynamicKeywordColumnLineage(header string) report.ColumnLineage {
	source := "dynamic source column"
	method := "from current result row ExtraColumns"
	blank := "not present for this row or not provided by source"
	lower := strings.ToLower(header)
	switch {
	case strings.HasPrefix(lower, "gff_"):
		source = "lemna GFF3"
		method = "parsed from already-loaded GFF3 rows during normal keyword search"
		blank = "GFF3 field was missing, not applicable, or not selected for this source"
	case strings.HasPrefix(lower, "attr_"):
		source = "lemna GFF3 attributes"
		method = "parsed from the current row's GFF3 attribute string"
		blank = "attribute was absent from the current row"
	case strings.HasPrefix(lower, "ahrd_"):
		source = "lemna AHRD"
		method = "loaded during normal lemna keyword search when AHRD data was available"
		blank = "AHRD did not provide this value for the row"
	case strings.HasPrefix(lower, "lemna_"):
		source = "lemna release metadata"
		method = "captured from already-known lemna release state"
		blank = "release metadata was not available in this run"
	}
	english := keywordColumnEnglishDetail(header)
	chinese := prompt.ColumnHelpChinese(header)
	japanese := prompt.ColumnHelpJapanese(header)
	if strings.TrimSpace(english) == "" {
		english = "This dynamic column came directly from the current keyword result rows. Its exact meaning depends on the source release field or parsed annotation key named in the column header."
	}
	display := prompt.ColumnExportHeader(header, prompt.ColumnDisplayOptions{})
	return report.ColumnLineage{ID: header, Column: display, Meaning: firstNonEmpty(english, "Dynamic keyword-mode source column: "+header), EnglishDetail: english, ChineseDetail: chinese, JapaneseDetail: japanese, Source: source, CollectionMethod: method, BlankMeaning: blank, UsedInStats: dynamicColumnStatsUse(header)}
}

func dynamicColumnStatsUse(header string) string {
	lower := strings.ToLower(header)
	if strings.Contains(lower, "quality") || strings.Contains(lower, "description") || strings.Contains(lower, "go") || strings.Contains(lower, "interpro") {
		return "quality"
	}
	return "no"
}

func keywordExportSettings(baseName string, outputDir string, settings exportSettings) []report.NameValue {
	return []report.NameValue{
		{Name: "File base name", Value: baseName, Explanation: "Base name used for selected Excel, raw Excel, peptide text, and raw peptide text outputs."},
		{Name: "Output folder", Value: outputDir, Explanation: "Destination directory for the current export action."},
		{Name: "Write selected Excel", Value: fmt.Sprintf("%t", settings.WriteExcel), Explanation: "Selected rows are written to the main workbook when true."},
		{Name: "Write raw Excel and raw text", Value: fmt.Sprintf("%t", settings.WriteRawExcel), Explanation: "All current rows are written to _raw.xlsx, and _raw.txt is also written when text export is enabled."},
		{Name: "Write peptide text", Value: fmt.Sprintf("%t", settings.WriteText), Explanation: "Peptide sequences are fetched and written only when true."},
		{Name: "Write report PDF", Value: fmt.Sprintf("%t", settings.WriteReport), Explanation: "One PDF report is written for the current export action when true."},
	}
}

func buildKeywordSequenceAudit(rows []model.KeywordResultRow, records []model.ProteinSequenceRecord) report.SequenceAudit {
	audit := report.SequenceAudit{
		Requested:       true,
		RequestedCount:  len(rows),
		WrittenCount:    len(records),
		TextFileType:    "FASTA-style peptide sequence text export for selected keyword rows",
		HeaderLabelMode: keywordSequenceHeaderLabelMode(rows),
	}
	recordByHeader := make(map[string]model.ProteinSequenceRecord, len(records))
	for _, record := range records {
		recordByHeader[record.Header] = record
		audit.TotalCharacters += len(record.Sequence)
	}
	audit.Records = make([]report.SequenceRecord, 0, len(rows))
	type summaryState struct {
		requested int
		written   int
		skipped   int
		totalLen  int
		minLen    int
		maxLen    int
		source    string
	}
	order := make([]string, 0, len(rows))
	byQuery := make(map[string]*summaryState, len(rows))
	for idx, row := range rows {
		header := keywordProteinSequenceHeader(row)
		record, ok := recordByHeader[header]
		status := "written"
		length := len(record.Sequence)
		source := "sequence fetched during the normal text-export workflow"
		if !ok {
			status = "skipped"
			source = "sequence record was not available in the current export state"
			audit.SkippedCount++
		}
		queryLabel := firstNonEmpty(row.LabelName, row.SearchTerm, "keyword query")
		state := byQuery[queryLabel]
		if state == nil {
			state = &summaryState{minLen: -1}
			byQuery[queryLabel] = state
			order = append(order, queryLabel)
		}
		state.requested++
		state.source = source
		if ok {
			state.written++
			state.totalLen += length
			if state.minLen < 0 || length < state.minLen {
				state.minLen = length
			}
			if length > state.maxLen {
				state.maxLen = length
			}
		} else {
			state.skipped++
		}
		audit.Records = append(audit.Records, report.SequenceRecord{
			Row:        idx + 1,
			SearchTerm: row.SearchTerm,
			Label:      row.LabelName,
			SequenceID: firstNonEmpty(row.SequenceID, "not available in this run"),
			Transcript: firstNonEmpty(row.TranscriptID, "not available in this run"),
			Status:     status,
			Length:     length,
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
			QueryKind:      "selected keyword peptide rows",
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

func keywordSequenceHeaderLabelMode(rows []model.KeywordResultRow) string {
	if len(rows) == 0 {
		return "no peptide sequence rows were selected"
	}
	labelled := 0
	plain := 0
	withSequenceHeaderLabel := 0
	withTranscript := 0
	withSequenceOnly := 0
	for _, row := range rows {
		if keywordRowLabelName(row) != "" {
			labelled++
		} else {
			plain++
		}
		if strings.TrimSpace(row.SequenceHeaderLabel) != "" {
			withSequenceHeaderLabel++
		}
		if strings.TrimSpace(row.TranscriptID) != "" {
			withTranscript++
		} else if strings.TrimSpace(row.SequenceID) != "" {
			withSequenceOnly++
		}
	}
	labelText := ""
	switch {
	case labelled == len(rows):
		labelText = "all headers append label_name in parentheses"
	case labelled == 0:
		labelText = "headers contain identifiers only because no label_name was available"
	default:
		labelText = fmt.Sprintf("%d/%d headers append label_name in parentheses", labelled, len(rows))
	}
	idText := ""
	switch {
	case withSequenceHeaderLabel == len(rows) && withTranscript == len(rows):
		idText = "identifier block uses sequence-header label plus transcript ID"
	case withTranscript == len(rows):
		idText = "identifier block uses transcript ID"
	case withSequenceOnly == len(rows):
		idText = "identifier block uses sequence ID"
	default:
		idText = "identifier block mixes sequence-header labels, transcript IDs, and sequence IDs according to row availability"
	}
	if plain > 0 && labelled > 0 {
		return labelText + "; " + idText + "; unlabeled rows keep plain FASTA identifiers"
	}
	return labelText + "; " + idText
}

func keywordGroupLabelMode(groups []model.KeywordSearchGroup) string {
	for _, group := range groups {
		if strings.TrimSpace(group.LabelMethod) != "" {
			return group.LabelMethod
		}
	}
	return "not recorded in this run"
}

func firstEnvForReport(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return name + "=" + value
		}
	}
	return "not available in this run"
}

func envPairForReport(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return ""
	}
	return name + "=" + value
}

func nonEmptyReportValues(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return []string{"not available in this run"}
	}
	return out
}

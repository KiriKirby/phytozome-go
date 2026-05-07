package report

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func RenderBlastPDF(path string, data ReportData) error {
	if data.GeneratedAt.IsZero() {
		data.GeneratedAt = time.Now()
	}
	if strings.TrimSpace(data.Title) == "" {
		data.Title = "BLAST Data Analysis Report"
	}
	if strings.TrimSpace(data.Mode) == "" {
		data.Mode = "blast"
	}
	r := newPDFReport(data.Title, data.GeneratedAt)
	renderBlastReport(r, data)
	return r.save(path)
}

func renderBlastReport(r *pdfReportRenderer, data ReportData) {
	b := data.Blast
	r.title(data.Title, "Audit report for the current BLAST-mode export action. The report describes query parsing, BLAST execution, optional reference enrichment, Family BLAST grouping, filter recommendations, final user selection, and generated files using only data already present in this run.")
	r.cards([]NameValue{
		{Name: "database", Value: valueOr(b.Database, "not available"), Explanation: "selected source"},
		{Name: "program", Value: valueOr(b.Execution.Program, "not available"), Explanation: b.Execution.ExecutionMode},
		{Name: "queries", Value: fmt.Sprintf("%d/%d", b.Selection.ResolvedQueries, b.Selection.ParsedQueries), Explanation: "resolved / parsed"},
		{Name: "selected rows", Value: strconv.Itoa(b.Selection.SelectedRows), Explanation: "final export"},
		{Name: "total rows", Value: strconv.Itoa(b.Selection.TotalRows), Explanation: "current export scope"},
		{Name: "references", Value: blastReferenceSummary(b.ExternalReferences), Explanation: "external evidence"},
		{Name: "Family BLAST", Value: blastFamilySummary(b.Family), Explanation: "grouping mode"},
		{Name: "filter", Value: blastFilterSummary(b.Filter), Explanation: "selection assistant"},
		{Name: "files", Value: strconv.Itoa(len(data.Files)), Explanation: "generated artifacts"},
	})
	r.paragraph(blastExecutiveParagraph(data), 9.5, colorText, pageWidth-marginLeft-marginRight)
	r.note("No additional BLAST run, source query, UniProt lookup, InterPro lookup, sequence fetch, cache refresh, download, BLAST+ probe, or OS command was performed to prepare this PDF.")

	renderBlastGeneratedFileIndex(r, data.Files, b.Runs)
	renderBlastRuntimeContext(r, data)
	renderBlastTiming(r, data)
	renderBlastSourceContext(r, data)
	renderBlastInputResolution(r, b)
	renderBlastExecution(r, b)
	renderBlastSelection(r, b)
	renderBlastExternalReferences(r, b)
	renderBlastFamily(r, b)
	renderBlastFilter(r, b)
	renderBlastColumns(r, b)
	renderBlastExportLog(r, b)
	renderBlastSequenceAudit(r, b)
	renderBlastFileAppendix(r, data.Files, b.Runs)
}

func renderBlastGeneratedFileIndex(r *pdfReportRenderer, files []GeneratedFile, runs []BlastRunReport) {
	r.chapterHeading("Chapter 2. Generated File Index")
	r.paragraph("The generated-file index maps each artifact back to the BLAST export action. BLAST exports can represent a single query, a selected run from a batch, a merged Family BLAST group, or Export all. Full hashes are kept in the technical appendix so the opening index remains readable.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	if len(files) == 0 {
		r.note("No generated data files were recorded for this BLAST export action. A complete report normally records selected Excel, raw Excel, peptide text, and the report PDF when requested.")
		return
	}
	rows := make([][]string, 0, len(files))
	for _, file := range files {
		label := blastFileRunLabel(file, runs)
		rows = append(rows, []string{file.Name, file.Type, fileSizeText(file.SizeBytes), label, file.Role, file.Path, "Appendix"})
	}
	r.table([]string{"File", "Type", "Size", "Run/family", "Role", "Location", "Details"}, rows, []float64{78, 60, 46, 58, 118, 118, 29})
	r.note("For Family BLAST, the family name is the primary artifact label. Member query labels and sequence records are documented in the Family BLAST and Sequence Export Audit chapters.")
}

func renderBlastRuntimeContext(r *pdfReportRenderer, data ReportData) {
	r.chapterHeading("Chapter 3. Software, User, Runtime, And System Context")
	r.paragraph("Runtime context identifies the executable, process, user session, output/cache paths, and local system facts that produced the BLAST export. In local BLAST+ mode these values also explain where local FASTA/index state and generated files belonged during execution, but the report does not probe BLAST+ again.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	sw := data.Software
	r.subheading("Software identity")
	r.table([]string{"Field", "Value"}, availableValueRows([][]string{
		{"Software name", sw.Name},
		{"Author", sw.Author},
		{"Repository", sw.Repository},
		{"Version", sw.Version},
		{"Go runtime", sw.GoVersion},
	}), []float64{118, 389})
	session := data.UserSession
	r.subheading("Runtime paths and session")
	r.table([]string{"Field", "Value"}, availableValueRows([][]string{
		{"User name", session.UserName},
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
	sys := data.System
	r.subheading("System")
	r.table([]string{"Field", "Value"}, availableValueRows([][]string{
		{"Operating system", sys.OS},
		{"OS version", sys.OSVersion},
		{"Architecture", sys.Architecture},
		{"CPU count", fmt.Sprintf("%d", sys.CPUCount)},
		{"Memory", sys.Memory},
	}), []float64{118, 389})
	r.note("The report did not run BLAST+ version commands, PATH checks, FASTA scans, terminal version commands, or OS probing commands. Missing local capability fields mean the workflow did not preserve them for report rendering.")
}

func renderBlastTiming(r *pdfReportRenderer, data ReportData) {
	r.chapterHeading("Chapter 4. Time Window And Performance Overview")
	r.paragraph("BLAST timing is split into preparation, execution, optional references, review, export, and report rendering when those phases were captured. Unmeasured phases are not guessed; export-generation timing is shown separately because it is the part already instrumented by file generation.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	tw := data.TimeWindow
	r.table([]string{"Event", "Timestamp", "Meaning"}, [][]string{
		{"Input started", formatTime(tw.QueryStart), "BLAST query scope began."},
		{"BLAST/results ended", formatTime(tw.SearchEnd), "Result rows were available for review."},
		{"Review started", formatTime(tw.ReviewStart), "User review, Family BLAST, and filter-driven row selection period began."},
		{"Export started", formatTime(tw.ExportStart), "File generation subflow began."},
		{"Export ended", formatTime(tw.ExportEnd), "Requested data files were complete."},
		{"Report rendered", formatTime(tw.ReportRendered), "PDF rendering completed or was scheduled to complete."},
	}, []float64{100, 155, 252})
	r.durationBars(data.Blast.GenerationSteps)
	r.table([]string{"Timing group", "What it covers"}, [][]string{
		{"Input preparation", "raw input, parsing, label collection, query resolution"},
		{"BLAST execution", "server submit/poll or local BLAST+ result loading"},
		{"Reference enrichment", "UniProt and InterPro enrichment when enabled"},
		{"Family/filter review", "family grouping, merge, filter application, user selection"},
		{"Export generation", "Excel, raw Excel, TXT, metadata, hashes, PDF rendering"},
	}, []float64{130, 377})
}

func renderBlastSourceContext(r *pdfReportRenderer, data ReportData) {
	r.chapterHeading("Chapter 5. Data Source, Species, And BLAST Target Context")
	b := data.Blast
	r.paragraph("This chapter separates the selected BLAST target source from query source metadata. A query may come from FASTA text, a Phytozome report URL, or direct sequence input while the target database/species is selected independently.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	s := b.Species
	r.subheading("Target species and source")
	r.table([]string{"Field", "Value", "Meaning"}, availableValueRows([][]string{
		{"Database", b.Database, "selected BLAST target source"},
		{"Display label", s.DisplayLabel, "species label used during execution"},
		{"Genome label", s.GenomeLabel, "genome/release label"},
		{"Common name", s.CommonName, "common name when known"},
		{"Search alias", s.SearchAlias, "scientific/search label"},
		{"JBrowse name", s.JBrowseName, "source genome/browser identifier"},
		{"Proteome ID", s.ProteomeID, "target/proteome ID when present"},
		{"Release date", s.ReleaseDate, "already-known release date"},
		{"Official marker", s.IsOfficial, "lemna official-clone marker when applicable"},
		{"Source notes", s.SourceNotes, "source context preserved by workflow/sample"},
	}), []float64{92, 162, 253})
	r.subheading("Execution capability")
	ex := b.Execution
	r.table([]string{"Field", "Value"}, [][]string{
		{"Program", ex.Program},
		{"Execution mode", ex.ExecutionMode},
		{"Query kind", ex.QueryKind},
		{"Target type", ex.TargetType},
		{"Server capability", valueOr(ex.ServerCapability, "not available in this run")},
		{"Local capability", valueOr(ex.LocalCapability, "not available in this run")},
		{"Fallback", valueOr(ex.Fallback, "not applicable or not captured")},
		{"Notes", valueOr(ex.Notes, "not available in this run")},
	}, []float64{126, 381})
	r.subheading("Query source versus target source")
	rows := make([][]string, 0, len(b.Inputs))
	for _, input := range b.Inputs {
		rows = append(rows, []string{fmt.Sprintf("%d", input.Order), input.LabelName, input.InputType, input.Source, input.OriginalURL, input.NormalizedURL, b.Database})
	}
	r.table([]string{"#", "Label", "Type", "Query source", "Original URL", "Normalized URL", "Target"}, rows, []float64{22, 50, 72, 75, 110, 110, 68})
	r.note("Target-side result-row gene_report_url values are different from query-side OriginalInputURL/NormalizedURL values. The report keeps them separate to avoid hiding cross-source or cross-species BLAST behavior.")
}

func renderBlastInputResolution(r *pdfReportRenderer, b BlastReportData) {
	r.chapterHeading("Chapter 6. BLAST Input Parsing And Query Resolution")
	r.paragraph("BLAST mode uses one input surface for several query forms. The program normalizes line endings, optionally expands a load command, splits text into records, classifies each record, resolves metadata and sequence content when possible, assigns labels, and submits only records with usable sequence content.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	r.inputTypeMosaic(b.Inputs)
	r.inputFunnel(b.Selection)
	r.subheading("Accepted input forms and parser branches")
	r.table([]string{"Input form", "Recognition rule", "Parser branch", "Report effect"}, [][]string{
		{"FASTA record", "record begins with > and contains sequence lines", "split header/sequence, sanitize sequence, infer label and identifiers", "query sequence length, header label, gene/transcript/protein IDs are shown when captured"},
		{"Phytozome report URL", "normalized /report/gene, /report/transcript, or /report/protein URL", "normalize URL, resolve report sequence through normal workflow resolver", "original and normalized URL are shown separately from target hit URLs"},
		{"Plain sequence", "single sequence-like token or pasted sequence block", "sanitize letters, infer nucleotide/protein kind later", "sequence length is shown; source identifiers remain blank"},
		{"Inline mixed line", "space-separated tokens where each token is URL-like or sequence-like", "split into independent query records before resolution", "each token receives its own row in the input trace"},
		{"load \"file.txt\"", "load command naming a .txt file in the application directory", "read file first, then apply the same normal parser", "loaded-file state is shown only when the workflow preserved it"},
	}, []float64{88, 130, 157, 132})
	rows := make([][]string, 0, len(b.Inputs))
	for _, input := range b.Inputs {
		rows = append(rows, []string{fmt.Sprintf("%d", input.Order), input.RawPreview, input.InputType, input.ParserPath, fmt.Sprintf("%d", input.SequenceLength), input.LabelName, input.Outcome, input.Notes})
	}
	r.table([]string{"#", "Raw preview", "Type", "Parser path", "Len", "Label", "Outcome", "Notes"}, rows, []float64{20, 82, 62, 105, 28, 48, 52, 110})
	r.subheading("Parser flow")
	r.flow([]string{"Raw input", "Optional load", "Normalize", "Split records", "Classify", "Resolve sequence", "Assign label", "Submit query"})
	r.note("Mixed input is accepted only when every token on a line can be independently recognized as a report URL or sequence-like token. The split trace must remain visible when captured.")
}

func renderBlastExecution(r *pdfReportRenderer, b BlastReportData) {
	r.chapterHeading("Chapter 7. BLAST Execution Method And Request Parameters")
	r.paragraph("This chapter records the BLAST request parameters and per-run result identifiers. It explains how query kind and target type selected the BLAST program and whether execution used server BLAST or local BLAST+.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	ex := b.Execution
	r.table([]string{"Parameter", "Value", "Meaning"}, [][]string{
		{"Program", ex.Program, "BLAST program used for result rows"},
		{"Execution mode", ex.ExecutionMode, "server/local/fallback path"},
		{"Query kind", ex.QueryKind, "DNA/RNA or protein query"},
		{"Target type", ex.TargetType, "genome/proteome target"},
		{"E-value", ex.EValue, "request E-value setting"},
		{"Comparison matrix", ex.ComparisonMatrix, "protein scoring matrix when applicable"},
		{"Word length", ex.WordLength, "word length setting"},
		{"Alignments to show", fmt.Sprintf("%d", ex.AlignmentsToShow), "requested alignment display limit"},
		{"Allow gaps", ex.AllowGaps, "gapped alignment option"},
		{"Filter query", ex.FilterQuery, "low complexity/query filtering option"},
	}, []float64{110, 120, 277})
	r.subheading("Program mapping")
	r.table([]string{"Query kind", "Target type", "Program"}, [][]string{
		{"nucleotide", "genome/nucleotide", "BLASTN"},
		{"nucleotide", "proteome/protein", "BLASTX"},
		{"protein", "genome/nucleotide", "TBLASTN"},
		{"protein", "proteome/protein", "BLASTP"},
	}, []float64{140, 170, 120})
	r.subheading("Executed runs")
	rows := make([][]string, 0, len(b.Runs))
	for _, run := range b.Runs {
		rows = append(rows, []string{fmt.Sprintf("%d", run.RunIndex), run.Label, valueOr(run.FamilyName, "-"), run.Program, run.ExecutionMode, run.JobID, fmt.Sprintf("%d", run.RowCount), fmt.Sprintf("%d", run.SelectedRows), run.BestEValue, run.BestIdentity})
	}
	r.table([]string{"#", "Label", "Family", "Program", "Mode", "Job/result", "Rows", "Selected", "Best E", "Best ID"}, rows, []float64{20, 46, 44, 52, 45, 95, 36, 44, 58, 58})
}

func renderBlastSelection(r *pdfReportRenderer, b BlastReportData) {
	r.chapterHeading("Chapter 8. Result Set, Review, Selection, Provenance, And Quality")
	r.paragraph("BLAST selection is per query, per batch run, or per Family BLAST group. This chapter summarizes current rows, final exported rows, zero-hit visibility, alignment metric availability, provenance layers, and BLAST-specific quality checks before the deeper reference/family/filter chapters.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	s := b.Selection
	r.cards([]NameValue{
		{Name: "parsed", Value: strconv.Itoa(s.ParsedQueries), Explanation: "input records"},
		{Name: "resolved", Value: strconv.Itoa(s.ResolvedQueries), Explanation: "usable queries"},
		{Name: "runs", Value: strconv.Itoa(s.ExecutedRuns), Explanation: "executed"},
		{Name: "exported", Value: strconv.Itoa(s.ExportedRuns), Explanation: "runs/groups"},
		{Name: "selected", Value: strconv.Itoa(s.SelectedRows), Explanation: "rows"},
		{Name: "unselected", Value: strconv.Itoa(s.UnselectedRows), Explanation: "rows"},
	})
	r.blastSelectionChart(s)
	r.blastRunBars(b.Runs)
	r.blastMetricAvailability(s)
	rows := make([][]string, 0, len(b.Runs))
	for _, run := range b.Runs {
		rows = append(rows, []string{run.Label, valueOr(run.FamilyName, "-"), fmt.Sprintf("%d", run.RowCount), fmt.Sprintf("%d", run.SelectedRows), run.TopHit, run.BestEValue, run.BestIdentity})
	}
	r.table([]string{"Run/group", "Family", "Rows", "Selected", "Top hit", "Best E-value", "Highest identity"}, rows, []float64{75, 55, 40, 50, 105, 86, 96})
	r.subheading("Layered provenance")
	r.provenanceChart(b.Provenance)
	r.table([]string{"Provenance layer", "Count", "Explanation"}, sortedProvenanceRows(b.Provenance), []float64{128, 50, 329})
	r.subheading("BLAST-specific quality checks")
	r.qualitySeverityChart(b.QualityChecks)
	qrows := make([][]string, 0, len(b.QualityChecks))
	for _, check := range b.QualityChecks {
		qrows = append(qrows, []string{check.Name, check.Result, check.Count, check.Rule, check.Source, check.Explanation})
	}
	r.table([]string{"Check", "Result", "Count", "Rule", "Source", "Interpretation"}, qrows, []float64{100, 54, 62, 104, 76, 111})
}

func renderBlastExternalReferences(r *pdfReportRenderer, b BlastReportData) {
	if !b.ExternalReferences.UniProtEnabled && !b.ExternalReferences.InterProEnabled {
		return
	}
	r.chapterHeading("Chapter 9. External Reference Analysis")
	r.paragraph("External references are optional evidence layers added after BLAST rows are available. They do not create the hit list; they add annotation, canonical-length comparison, protein existence, sequence caution, and conserved-region evidence that can support review.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	if b.ExternalReferences.UniProtEnabled {
		renderBlastUniProt(r, b.ExternalReferences.UniProt)
	}
	if b.ExternalReferences.InterProEnabled {
		renderBlastInterPro(r, b.ExternalReferences.InterPro)
	}
}

func renderBlastUniProt(r *pdfReportRenderer, u UniProtReferenceReport) {
	r.subheading("UniProt Reference Layer")
	r.paragraph("UniProt enrichment uses the UniProtKB search endpoint during the real workflow. Candidate identifiers include accessions, protein IDs, subject IDs, sequence IDs, transcript IDs, and accessions extracted from deflines or report URLs. Rows sharing a lookup key are grouped to avoid duplicate checks.", 8.9, colorText, pageWidth-marginLeft-marginRight)
	r.flow([]string{"BLAST target row", "Candidate keys", "UniProtKB search", "Best entry", "Row enrichment", "Filter evidence"})
	r.table([]string{"Lookup detail", "Value", "Explanation"}, sortedRowsFromNameValues(u.LookupSummary), []float64{120, 145, 242})
	r.table([]string{"Outcome", "Value", "Explanation"}, sortedRowsFromNameValues(u.Outcome), []float64{120, 95, 292})
	r.uniProtOutcomeChart(u.Outcome)
	rows := make([][]string, 0, len(u.Rows))
	for _, row := range u.Rows {
		rows = append(rows, []string{fmt.Sprintf("%d", row.Row), row.Label, row.Target, row.Accession, row.Reviewed, row.LengthRatio, row.Fragment, row.Caution, row.Annotation})
	}
	r.table([]string{"Row", "Label", "Target", "Accession", "Reviewed", "Ratio", "Fragment", "Caution", "Annotation"}, rows, []float64{25, 42, 72, 62, 58, 45, 45, 56, 102})
	r.note("Length ratio formula: target_length / UniProt canonical length (%) = target_length / UniProt entry length * 100. The report shows the distribution and available row values already captured by the workflow, without selecting individual rows as illustrations.")
}

func renderBlastInterPro(r *pdfReportRenderer, ip InterProReferenceReport) {
	r.subheading("InterPro Conserved-Region Layer")
	r.paragraph("InterPro enrichment looks up UniProt accessions through the InterPro protein endpoint during the real workflow. When query-side evidence is available, the report explains query-versus-hit conserved-region matching; otherwise hit self-evidence is used.", 8.9, colorText, pageWidth-marginLeft-marginRight)
	r.flow([]string{"Query entry", "Hit entry", "Evidence switches", "Coverage thresholds", "Status", "Filter/family score"})
	r.table([]string{"Parameter", "Value", "Meaning"}, sortedRowsFromNameValues(ip.Settings), []float64{118, 62, 327})
	r.interProStatusChart(ip.StatusCounts)
	r.table([]string{"Outcome", "Value", "Explanation"}, sortedRowsFromNameValues(ip.Outcome), []float64{122, 90, 295})
	r.subheading("Decision formula")
	r.flow([]string{"Query InterPro", "Hit InterPro", "Evidence score", "Matched coverage", "Status"})
	r.note("Evidence score = 5*PfamMatch + 4*InterProAccessionMatch + 3*SignatureMatch + 1*EntryTypeMatch + 1*EntryNameMatch + 1*RegionEvidence.")
}

func renderBlastFamily(r *pdfReportRenderer, b BlastReportData) {
	if b.Family == nil {
		return
	}
	f := b.Family
	r.chapterHeading("Chapter 10. Family BLAST Analysis")
	r.paragraph("Family BLAST keeps BLAST execution per query, then groups review/export units after individual results exist. Automatic family detection proposes groups from labels/source IDs without rewriting original label_name values, and the optional group editor can confirm or change those groups before execution. After grouping, duplicate targets can be merged by keeping the best-ranked row for each normalized target key.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	r.flow([]string{"Resolved queries", "Detect family name", "Group members", "Run BLAST per member", "Merge target rows", "Review/export family"})
	r.table([]string{"Setting", "Value", "Meaning"}, sortedRowsFromNameValues(f.Settings), []float64{132, 62, 313})
	r.subheading("Final family groups")
	rows := make([][]string, 0, len(f.Groups))
	for _, group := range f.Groups {
		rows = append(rows, []string{group.Name, strings.Join(group.MemberLabels, ", "), valueOr(group.GroupSource, "-"), group.DetectionRule, fmt.Sprintf("%d", group.OriginalRuns), fmt.Sprintf("%d -> %d", group.RowsBefore, group.RowsAfter), group.OutputBaseName})
	}
	r.table([]string{"Family", "Members", "Group source", "Grouping rule", "Runs", "Rows", "Output"}, rows, []float64{44, 98, 72, 132, 28, 46, 62})
	r.familyMergeChart(f.Groups)
	r.subheading("Merge audit")
	mergeRows := make([][]string, 0, len(f.MergeRecords))
	for _, m := range f.MergeRecords {
		mergeRows = append(mergeRows, []string{m.Family, m.TargetKey, m.MemberRows, m.ChosenRow, m.Reason, m.ScoreDetails})
	}
	r.table([]string{"Family", "Target key", "Member rows", "Chosen", "Reason", "Score details"}, mergeRows, []float64{45, 82, 96, 66, 98, 120})
	r.note("Family reference score can include InterPro present/partial/missing evidence, InterPro coverage, UniProt accession/reviewed status, fragment/caution penalties, and distance from 100% target/canonical length ratio.")
}

func renderBlastFilter(r *pdfReportRenderer, b BlastReportData) {
	if b.Filter == nil {
		return
	}
	f := b.Filter
	r.chapterHeading("Chapter 11. BLAST Filter Analysis")
	r.paragraph("The BLAST filter is a row-selection assistant, not a destructive transform. It rebuilds checkbox recommendations and marks suggested removals, while the user can still change final export selection. This chapter separates the original BLAST result rows, the automatic recommendation, and the final user-selected export rows using all query scopes captured in this export.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	r.cards([]NameValue{
		{Name: "total rows", Value: strconv.Itoa(f.Totals.TotalRows), Explanation: "filter scope"},
		{Name: "queries", Value: strconv.Itoa(f.Totals.QueryCount), Explanation: "scoped"},
		{Name: "recommended keep", Value: strconv.Itoa(f.RecommendedKeep), Explanation: "automatic"},
		{Name: "recommended remove", Value: strconv.Itoa(f.RecommendedRemove), Explanation: "automatic"},
		{Name: "final selected", Value: strconv.Itoa(f.FinalSelected), Explanation: "user export"},
		{Name: "rescued", Value: strconv.Itoa(f.UserRescued), Explanation: "user selected despite removal"},
		{Name: "user removed", Value: strconv.Itoa(f.UserRemovedAfterKeep), Explanation: "kept by filter, unselected by user"},
		{Name: "agreement", Value: f.Totals.AgreementPercent, Explanation: "auto vs final"},
		{Name: "cleared", Value: fmt.Sprintf("%t", f.Cleared), Explanation: "filter marks"},
	})
	if f.Cleared {
		r.note("The filter clear action was captured. A clear action removes filter marks and reselects rows at that moment; any later final user selection differences are shown in the same matrices and per-query tables below when captured.")
	}
	r.subheading("Decision flow")
	r.flow([]string{"All BLAST rows", "Hard rules", "Soft score", "Isoform and top-hit limits", "Filter recommendation", "User final selection"})
	r.paragraph("The filter first evaluates each row against active hard rules. Optional soft scoring can then remove rows below the configured score. Ranking rules are used when duplicate isoforms or per-query top-hit limits are enabled. The final export is still controlled by the user's checkbox state, so the report always keeps automatic recommendation and final selection as separate audit states.", 9.0, colorText, pageWidth-marginLeft-marginRight)
	r.subheading("Global filter totals")
	r.table([]string{"Metric", "Value", "Meaning"}, [][]string{
		{"Total rows evaluated", strconv.Itoa(f.Totals.TotalRows), "All rows available to the filter in this export scope."},
		{"Queries summarized", strconv.Itoa(f.Totals.QueryCount), "Every query/run scope represented in the per-query table."},
		{"Automatic keep", strconv.Itoa(f.Totals.RecommendedKeep), "Rows not marked for removal by the filter."},
		{"Automatic remove", strconv.Itoa(f.Totals.RecommendedRemove), "Rows marked for removal by the filter."},
		{"Final selected", strconv.Itoa(f.Totals.FinalSelected), "Rows included by the user in the final export."},
		{"Final unselected", strconv.Itoa(f.Totals.FinalUnselected), "Rows left out of the final export."},
		{"User rescued", strconv.Itoa(f.Totals.UserRescued), "Rows the filter suggested removing but the user selected."},
		{"User removed after keep", strconv.Itoa(f.Totals.UserRemoved), "Rows the filter suggested keeping but the user left unselected."},
		{"Agreement", f.Totals.AgreementPercent, "Rows where automatic recommendation and final user choice match."},
		{"Difference rows", strconv.Itoa(f.Totals.DifferenceRows), "All rows where automatic recommendation and final user choice differ."},
	}, []float64{142, 72, 293})
	r.filterRecommendationChart(f)
	r.filterMatrix(f)
	r.filterDifferenceChart(f)
	r.filterQueryBars(f.QuerySummaries)
	r.subheading("Per-query filter and user-selection statistics")
	queryRows := make([][]string, 0, len(f.QuerySummaries))
	for _, summary := range f.QuerySummaries {
		queryRows = append(queryRows, []string{
			valueOr(summary.Query, "query"),
			valueOr(summary.Family, "-"),
			strconv.Itoa(summary.TotalRows),
			fmt.Sprintf("%d / %d", summary.RecommendedKeep, summary.RecommendedRemove),
			fmt.Sprintf("%d / %d", summary.FinalSelected, summary.FinalUnselected),
			strconv.Itoa(summary.UserRescued),
			strconv.Itoa(summary.UserRemoved),
			summary.AgreementPercent,
			fmt.Sprintf("%+d", summary.Difference),
		})
	}
	r.table([]string{"Query", "Family", "Rows", "Auto keep/remove", "Final selected/unselected", "Rescued", "User removed", "Agreement", "Final-auto"}, queryRows, []float64{68, 42, 32, 68, 82, 45, 56, 56, 58})
	r.subheading("Filter formulas")
	r.table([]string{"Formula", "Expression", "Explanation"}, sortedRowsFromNameValues(f.Formulas), []float64{118, 160, 229})
	r.subheading("Hard-rule failure totals")
	rules := make([][]string, 0, len(f.HardRuleSummaries))
	for _, check := range f.HardRuleSummaries {
		rules = append(rules, []string{check.Name, check.Result, strconv.Itoa(check.Passed), strconv.Itoa(check.Failed), check.Rule, check.Source, check.Explanation})
	}
	r.filterRuleFailureBars(f.HardRuleSummaries)
	r.table([]string{"Rule", "Result", "Passed", "Failed", "Configured behavior", "Source", "Interpretation"}, rules, []float64{82, 44, 38, 38, 112, 62, 131})
	r.subheading("Parameter dictionary")
	r.paragraph("Every BLAST filter parameter used by the current run is shown below with the value used, the program default, its meaning, and its direct effect on filtering or ranking. Numeric values are not interpreted as biological truth; they are the configured audit logic that produced the automatic recommendation state.", 8.9, colorText, pageWidth-marginLeft-marginRight)
	for _, group := range BlastFilterSettingGroups(f.Settings) {
		r.subheading(group)
		rows := make([][]string, 0)
		for _, setting := range BlastFilterSettingDetailsByGroup(f.Settings, group) {
			rows = append(rows, []string{setting.Name, setting.Value, setting.Default, setting.Meaning, setting.Effect})
		}
		r.table([]string{"Parameter", "Value", "Default", "Meaning", "Effect"}, rows, []float64{118, 54, 54, 140, 141})
	}
	r.subheading("State definitions")
	r.table([]string{"State", "Definition"}, [][]string{
		{"Filter recommended keep", "The filter did not mark the row for removal after hard rules, optional soft score, and ranking limits."},
		{"Filter recommended remove", "The filter marked the row for removal through a hard-rule failure, soft-score failure, isoform limit, or top-hit limit."},
		{"Final selected", "The user left the row selected for export."},
		{"Final unselected", "The user left the row out of the export."},
		{"User rescued", "The filter suggested removal, but the user selected the row."},
		{"User removed after keep", "The filter suggested keeping the row, but the user unselected it."},
		{"Agreement", "Automatic recommendation and final user selection point in the same keep/remove direction."},
	}, []float64{145, 362})
}

func renderBlastColumns(r *pdfReportRenderer, b BlastReportData) {
	r.chapterHeading("Chapter 12. Column Dictionary And Data Lineage")
	r.paragraph("BLAST workbook columns change with database, BLAST program, UniProt, and InterPro settings. The report groups them by audit/export, source/run, hit identity, alignment, target length, and optional reference evidence.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	r.subheading("Column group map")
	r.table([]string{"Group", "Columns", "Meaning"}, [][]string{
		{"audit/export", "row, filter presentation state", "generated values for traceability"},
		{"source/run", "source_database, blast_program, label_name", "workflow and query labeling"},
		{"hit identity", "protein, subject_id, sequence_id, transcript_id, gene_report_url", "target-side identifiers"},
		{"alignment", "e_value, identity, coverage, coordinates, bitscore", "BLAST alignment evidence"},
		{"UniProt", "accession, reviewed, canonical length, GO, EC, function", "optional UniProt enrichment"},
		{"InterPro", "conserved-region status, accessions, coverage, regions", "optional InterPro enrichment"},
	}, []float64{82, 172, 253})
	r.blastCompletenessChart(b.ColumnCompleteness)
	rows := make([][]string, 0, len(b.ColumnCompleteness))
	for _, col := range b.ColumnCompleteness {
		rows = append(rows, []string{col.Column, fmt.Sprintf("%d", col.FilledRows), fmt.Sprintf("%d", col.EmptyRows), col.FilledRatio, col.Source})
	}
	r.table([]string{"Column", "Filled", "Empty", "Filled %", "Source"}, rows, []float64{134, 48, 48, 52, 225})
	r.subheading("English column descriptions")
	englishRows := make([][]string, 0, len(b.Columns))
	for _, col := range b.Columns {
		englishRows = append(englishRows, []string{col.Column, valueOr(col.EnglishDetail, col.Meaning)})
	}
	r.table([]string{"Column", "English explanation"}, englishRows, []float64{105, 402})
	r.subheading("Chinese and Japanese column descriptions")
	triRows := make([][]string, 0, len(b.Columns))
	for _, col := range b.Columns {
		triRows = append(triRows, []string{
			col.Column,
			valueOr(col.ChineseDetail, "not available in this run"),
			valueOr(col.JapaneseDetail, "not available in this run"),
		})
	}
	r.table([]string{"Column", "中文说明", "日本語説明"}, triRows, []float64{92, 208, 207})
}

func renderBlastExportLog(r *pdfReportRenderer, b BlastReportData) {
	r.chapterHeading("Chapter 13. Export Settings And Generation Log")
	r.paragraph("The export log records requested file types, output naming, row scope, TXT header behavior, row-number preservation, filter-flag availability, and measured file-generation steps. It describes file generation, not the whole interactive BLAST session.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	r.table([]string{"Setting", "Value", "Effect"}, sortedRowsFromNameValues(b.ExportSettings), []float64{132, 118, 257})
	rows := make([][]string, 0, len(b.GenerationSteps))
	for _, step := range b.GenerationSteps {
		rows = append(rows, []string{step.Name, formatTime(step.Start), formatTime(step.End), formatDurationMS(stepDuration(step)), step.Status, step.Details})
	}
	r.table([]string{"Step", "Start", "End", "Duration", "Status", "Details"}, rows, []float64{100, 88, 88, 54, 48, 129})
	r.durationBars(b.GenerationSteps)
}

func renderBlastSequenceAudit(r *pdfReportRenderer, b BlastReportData) {
	r.chapterHeading("Chapter 14. Sequence Export Audit")
	seq := b.Sequences
	if !seq.Requested {
		r.paragraph("Peptide text export was not requested, so sequence-export details are not applicable. The report did not fetch sequences to populate this chapter.", 9.2, colorText, pageWidth-marginLeft-marginRight)
		return
	}
	r.paragraph("BLAST text export can contain two distinct record classes: query sequence records prepended for reference and selected hit peptide records exported from BLAST rows. In Family BLAST, the prepended query block follows the family TXT setting: either only the first family member query is written at the top, or all family-member queries are written in family order.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	r.cards([]NameValue{
		{Name: "requested", Value: strconv.Itoa(seq.RequestedCount), Explanation: "records"},
		{Name: "written", Value: strconv.Itoa(seq.WrittenCount), Explanation: "records"},
		{Name: "skipped", Value: strconv.Itoa(seq.SkippedCount), Explanation: "records"},
		{Name: "aa chars", Value: strconv.Itoa(seq.TotalCharacters), Explanation: "written"},
	})
	r.sequenceChart(seq)
	r.table([]string{"Field", "Value"}, [][]string{
		{"Text file type", valueOr(seq.TextFileType, "BLAST peptide text export")},
		{"Header label mode", valueOr(seq.HeaderLabelMode, "not available in this run")},
	}, []float64{118, 389})
	estimatedRows := math.Max(1, float64(len(seq.QuerySummaries)))
	if r.y+120+estimatedRows*28 > pageHeight-marginBottom {
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
			fmt.Sprintf("%d aa", summary.TotalLength),
			summary.SourceSummary,
		})
	}
	r.table([]string{"Query", "Record class", "Requested", "Written", "Skipped", "Average", "Range", "Total aa", "Source summary"}, rows, []float64{56, 72, 34, 34, 34, 40, 48, 42, 147})
	r.sequenceLengthDotPlot(seq.QuerySummaries, "Per-query sequence length ranges")
}

func renderBlastFileAppendix(r *pdfReportRenderer, files []GeneratedFile, runs []BlastRunReport) {
	r.chapterHeading("Chapter 15. File Technical Details Appendix")
	r.paragraph("This appendix preserves full technical metadata and hashes for every generated artifact. Earlier chapters keep file summaries readable; this chapter keeps byte-level traceability.", 9.2, colorText, pageWidth-marginLeft-marginRight)
	for _, file := range files {
		label := blastFileRunLabel(file, runs)
		r.subheading(file.Name)
		r.table([]string{"Field", "Value"}, [][]string{
			{"File name", file.Name},
			{"Full path", file.Path},
			{"Type", file.Type},
			{"Role", file.Role},
			{"Associated query/run/family", label},
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
		}, []float64{128, 379})
	}
	r.note("The hash identifies exact file bytes at inspection time. The report PDF does not embed its own final hash because writing that hash into the PDF would change the PDF bytes.")
}

func blastFileRunLabel(file GeneratedFile, runs []BlastRunReport) string {
	if len(runs) == 1 {
		return valueOr(runs[0].FamilyName, runs[0].Label)
	}
	base := strings.TrimSuffix(filepath.Base(file.Name), filepath.Ext(file.Name))
	base = strings.TrimSuffix(base, "_raw")
	baseKey := normalizeReportMatchKey(base)
	for _, run := range runs {
		for _, candidate := range []string{run.FamilyName, run.Label} {
			if normalizeReportMatchKey(candidate) == baseKey {
				return valueOr(run.FamilyName, run.Label)
			}
		}
	}
	if strings.HasSuffix(strings.ToLower(file.Name), "_rpt.pdf") {
		return "current BLAST export"
	}
	return "current BLAST export"
}

func normalizeReportMatchKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.Join(strings.Fields(value), " ")
	value = strings.TrimSuffix(value, "_raw")
	return value
}

func blastReferenceSummary(ref ExternalReferenceReport) string {
	switch {
	case ref.UniProtEnabled && ref.InterProEnabled:
		return "UniProt + InterPro"
	case ref.UniProtEnabled:
		return "UniProt"
	case ref.InterProEnabled:
		return "InterPro"
	default:
		return "none"
	}
}

func blastFamilySummary(f *FamilyBlastReport) string {
	if f == nil {
		return "not used"
	}
	if len(f.Groups) == 0 {
		return "enabled"
	}
	return fmt.Sprintf("%d group(s)", len(f.Groups))
}

func blastFilterSummary(f *BlastFilterReport) string {
	if f == nil {
		return "not used"
	}
	if f.Cleared {
		return "cleared"
	}
	if f.Applied {
		if f.UserRescued > 0 || f.UserRemovedAfterKeep > 0 {
			return "applied + overrides"
		}
		return "applied"
	}
	return "available"
}

func blastExecutiveParagraph(data ReportData) string {
	b := data.Blast
	return fmt.Sprintf("This report documents a BLAST-mode export generated by phytozome GO. The current run parsed %d input record(s), resolved %d usable query sequence(s), executed %d BLAST run(s) with %s in %s mode against %s, exported %d run/group scope(s), and wrote %d generated artifact(s). External references were %s, Family BLAST was %s, and the BLAST filter was %s. The report covers only the current generated file set and the data already known to the workflow during export.",
		b.Selection.ParsedQueries,
		b.Selection.ResolvedQueries,
		b.Selection.ExecutedRuns,
		valueOr(b.Execution.Program, "an unavailable BLAST program"),
		valueOr(b.Execution.ExecutionMode, "an unavailable execution"),
		valueOr(b.Species.DisplayLabel, "the selected target species"),
		b.Selection.ExportedRuns,
		len(data.Files),
		blastReferenceSummary(b.ExternalReferences),
		blastFamilySummary(b.Family),
		blastFilterSummary(b.Filter),
	)
}

func (r *pdfReportRenderer) flow(labels []string) {
	if len(labels) == 0 {
		return
	}
	w := (pageWidth - marginLeft - marginRight - float64(len(labels)-1)*8) / float64(len(labels))
	wrapped := make([][]string, len(labels))
	maxLines := 1
	for i, label := range labels {
		wrapped[i] = wrapText(label, w-8, 7.0)
		if len(wrapped[i]) > maxLines {
			maxLines = len(wrapped[i])
		}
	}
	h := math.Max(28, float64(maxLines)*8.6+13)
	r.ensure(h + 14)
	x := marginLeft
	y := r.y
	for i, label := range labels {
		_ = label
		r.rect(x, y, w, h, pdfColor{0.95, 0.98, 0.99}, colorPrimary, 0.45)
		lineY := y + 12
		for _, line := range wrapped[i] {
			r.text(x+4, lineY, 7.0, fontBold, colorPrimary, line)
			lineY += 8.6
		}
		if i < len(labels)-1 {
			r.line(x+w+1, y+h/2, x+w+7, y+h/2, colorSecondary, 0.8)
			r.text(x+w+2, y+h/2-3, 7, fontBold, colorSecondary, ">")
		}
		x += w + 8
	}
	r.y += h + 14
}

func (r *pdfReportRenderer) inputTypeMosaic(inputs []BlastInputTrace) {
	counts := map[string]int{}
	order := []string{"FASTA record", "report URL", "plain sequence", "loaded file", "inline mixed line", "skipped/unresolved"}
	for _, input := range inputs {
		key := input.InputType
		if strings.TrimSpace(key) == "" {
			key = "skipped/unresolved"
		}
		counts[key]++
	}
	total := 0
	for _, count := range counts {
		total += count
	}
	if total <= 0 {
		r.note("Input type mosaic was not rendered because no input trace records were available.")
		return
	}
	colors := []pdfColor{colorPrimary, colorSecondary, colorSuccess, colorPurple, colorWarning, colorMissing}
	segments := make([]chartSegment, 0, len(order))
	for i, label := range order {
		if counts[label] > 0 {
			segments = append(segments, chartSegment{Label: label, Value: float64(counts[label]), Color: colors[i%len(colors)]})
		}
	}
	r.ensure(96)
	r.text(marginLeft, r.y, 10, fontBold, colorText, "Input type mosaic")
	r.y += 16
	x := marginLeft
	barW := pageWidth - marginLeft - marginRight
	for _, segment := range segments {
		w := barW * segment.Value / float64(total)
		r.fillRect(x, r.y, w, 18, segment.Color)
		x += w
	}
	r.strokeRect(marginLeft, r.y, barW, 18, colorRule, 0.3)
	r.y += 30
	r.legend(marginLeft, r.y, segments)
	r.y += float64(len(segments))*13 + 8
}

func (r *pdfReportRenderer) inputFunnel(sel BlastSelectionStats) {
	steps := []NameValue{
		{Name: "parsed records", Value: strconv.Itoa(sel.ParsedQueries), Explanation: "records after splitting"},
		{Name: "resolved queries", Value: strconv.Itoa(sel.ResolvedQueries), Explanation: "usable sequences"},
		{Name: "executed runs", Value: strconv.Itoa(sel.ExecutedRuns), Explanation: "BLAST jobs"},
		{Name: "exported scopes", Value: strconv.Itoa(sel.ExportedRuns), Explanation: "runs/groups"},
	}
	r.subheading("Input resolution funnel")
	r.cards(steps)
}

func (r *pdfReportRenderer) blastSelectionChart(sel BlastSelectionStats) {
	if sel.TotalRows <= 0 {
		r.note("Selection chart was not rendered because no BLAST result rows were present.")
		return
	}
	r.ensure(178)
	top := r.y
	segments := []chartSegment{
		{Label: "final selected", Value: float64(sel.SelectedRows), Color: colorPrimary},
		{Label: "final unselected", Value: float64(sel.UnselectedRows), Color: colorSecondary},
	}
	r.text(marginLeft, top, 10, fontBold, colorText, "Final BLAST row selection")
	r.donut(marginLeft+76, top+78, 54, 26, segments)
	r.text(marginLeft+59, top+82, 12, fontBold, colorText, strconv.Itoa(sel.TotalRows))
	r.text(marginLeft+48, top+96, 7.3, fontRegular, colorMuted, "result rows")
	legendX := marginLeft + 160
	r.legend(legendX, top+34, segments)
	r.y = math.Max(top+142, top+34+r.legendHeight(pageWidth-marginRight-legendX, segments)+12)
	r.paragraph("This chart describes final export selection, not biological truth. The filter chapter separately compares automatic recommendations with user choices when filter state is present.", 8.8, colorMuted, pageWidth-marginLeft-marginRight)
}

func (r *pdfReportRenderer) blastRunBars(runs []BlastRunReport) {
	if len(runs) == 0 {
		return
	}
	r.ensure(38)
	r.text(marginLeft, r.y, 10, fontBold, colorText, "Per-run rows and selected rows")
	r.y += 18
	maxRows := 1
	for _, run := range runs {
		if run.RowCount > maxRows {
			maxRows = run.RowCount
		}
	}
	labelW := 120.0
	barW := pageWidth - marginLeft - marginRight - labelW - 22
	for _, run := range runs {
		labelLines := wrapText(valueOr(run.Label, run.FamilyName), labelW-4, 7.8)
		rowH := math.Max(23, float64(len(labelLines))*9.6+5)
		r.ensure(rowH)
		y := r.y
		lineY := y + 9
		for _, line := range labelLines {
			r.text(marginLeft, lineY, 7.8, fontRegular, colorText, line)
			lineY += 9.6
		}
		x := marginLeft + labelW
		r.rect(x, y, barW, 11, pdfColor{0.94, 0.95, 0.96}, colorRule, 0.2)
		selectedW := barW * float64(run.SelectedRows) / float64(maxRows)
		totalW := barW * float64(run.RowCount) / float64(maxRows)
		if totalW > 0 {
			r.fillRect(x, y, totalW, 11, colorSecondary)
		}
		if selectedW > 0 {
			r.fillRect(x, y, selectedW, 11, colorPrimary)
		}
		r.text(x+barW+5, y+9, 7.2, fontRegular, colorMuted, fmt.Sprintf("%d/%d", run.SelectedRows, run.RowCount))
		r.y += rowH
	}
	r.y += 8
}

func (r *pdfReportRenderer) blastMetricAvailability(sel BlastSelectionStats) {
	total := sel.TotalRows
	if total <= 0 {
		return
	}
	r.subheading("Selected evidence availability")
	rows := [][]string{
		{"Report URL", fmt.Sprintf("%d of %d", sel.RowsWithURL, total), percentText(sel.RowsWithURL, total)},
		{"Sequence ID", fmt.Sprintf("%d of %d", sel.RowsWithSequence, total), percentText(sel.RowsWithSequence, total)},
		{"Target length", fmt.Sprintf("%d of %d", sel.RowsWithTargetLen, total), percentText(sel.RowsWithTargetLen, total)},
		{"Query coverage", fmt.Sprintf("%d of %d", sel.RowsWithCoverage, total), percentText(sel.RowsWithCoverage, total)},
		{"E-value", fmt.Sprintf("%d of %d", sel.RowsWithEValue, total), percentText(sel.RowsWithEValue, total)},
		{"Identity", fmt.Sprintf("%d of %d", sel.RowsWithIdentity, total), percentText(sel.RowsWithIdentity, total)},
	}
	r.table([]string{"Evidence field", "Rows", "Completeness"}, rows, []float64{150, 100, 100})
}

func (r *pdfReportRenderer) qualitySeverityChart(checks []QualityCheck) {
	if len(checks) == 0 {
		r.note("Quality severity chart was not rendered because no BLAST quality checks were supplied.")
		return
	}
	counts := map[string]int{}
	for _, check := range checks {
		counts[strings.ToLower(strings.TrimSpace(check.Result))]++
	}
	segments := []chartSegment{
		{Label: "pass", Value: float64(counts["pass"]), Color: colorSuccess},
		{Label: "warning", Value: float64(counts["warning"]), Color: colorWarning},
		{Label: "not requested", Value: float64(counts["not requested"]), Color: colorSecondary},
		{Label: "not available", Value: float64(counts["not available in this run"]), Color: colorMissing},
	}
	r.ensure(150)
	top := r.y
	r.text(marginLeft, top, 10, fontBold, colorText, "BLAST quality check severity")
	r.donut(marginLeft+74, top+74, 50, 24, segments)
	legendX := marginLeft + 155
	r.legend(legendX, top+28, segments)
	r.y = math.Max(top+128, top+28+r.legendHeight(pageWidth-marginRight-legendX, segments)+12)
	r.paragraph("Quality checks evaluate traceability, completeness, and audit readiness. They do not declare a BLAST hit biologically correct or incorrect.", 8.8, colorMuted, pageWidth-marginLeft-marginRight)
}

func (r *pdfReportRenderer) uniProtOutcomeChart(outcomes []NameValue) {
	r.subheading("UniProt outcome overview")
	r.cards(outcomes)
}

func (r *pdfReportRenderer) interProStatusChart(status []NameValue) {
	if len(status) == 0 {
		return
	}
	colors := []pdfColor{colorSuccess, colorPrimary, colorWarning, colorSecondary, colorMissing}
	segments := make([]chartSegment, 0, len(status))
	for i, value := range status {
		segments = append(segments, chartSegment{Label: value.Name, Value: floatValue(value.Value), Color: colors[i%len(colors)]})
	}
	r.ensure(160)
	top := r.y
	r.text(marginLeft, top, 10, fontBold, colorText, "InterPro conserved-region status")
	r.donut(marginLeft+74, top+74, 50, 24, segments)
	legendX := marginLeft + 155
	r.legend(legendX, top+20, segments)
	r.y = math.Max(top+132, top+20+r.legendHeight(pageWidth-marginRight-legendX, segments)+12)
}

func (r *pdfReportRenderer) familyMergeChart(groups []FamilyBlastGroupReport) {
	if len(groups) == 0 {
		return
	}
	r.ensure(42 + float64(len(groups))*24)
	r.text(marginLeft, r.y, 10, fontBold, colorText, "Family rows before and after target merge")
	r.y += 18
	maxRows := 1
	for _, group := range groups {
		if group.RowsBefore > maxRows {
			maxRows = group.RowsBefore
		}
	}
	labelW := 70.0
	barW := pageWidth - marginLeft - marginRight - labelW - 20
	for _, group := range groups {
		y := r.y
		r.text(marginLeft, y+9, 7.8, fontRegular, colorText, group.Name)
		x := marginLeft + labelW
		beforeW := barW * float64(group.RowsBefore) / float64(maxRows)
		afterW := barW * float64(group.RowsAfter) / float64(maxRows)
		r.fillRect(x, y, beforeW, 10, colorSecondary)
		r.fillRect(x, y+11, afterW, 10, colorPrimary)
		r.text(x+barW+5, y+14, 7.2, fontRegular, colorMuted, fmt.Sprintf("%d -> %d", group.RowsBefore, group.RowsAfter))
		r.y += 28
	}
	r.y += 8
}

func (r *pdfReportRenderer) filterMatrix(f *BlastFilterReport) {
	r.subheading("Filter recommendation versus final user selection")
	r.table([]string{"", "Final selected", "Final unselected"}, [][]string{
		{"Filter recommended keep", strconv.Itoa(maxIntReport(0, f.Totals.RecommendedKeep-f.Totals.UserRemoved)), strconv.Itoa(f.Totals.UserRemoved)},
		{"Filter recommended remove", strconv.Itoa(f.Totals.UserRescued), strconv.Itoa(maxIntReport(0, f.Totals.RecommendedRemove-f.Totals.UserRescued))},
	}, []float64{170, 120, 130})
}

func (r *pdfReportRenderer) filterRecommendationChart(f *BlastFilterReport) {
	if f.Totals.TotalRows <= 0 {
		return
	}
	r.ensure(150)
	top := r.y
	segments := []chartSegment{
		{Label: "recommended keep", Value: float64(f.Totals.RecommendedKeep), Color: colorSuccess},
		{Label: "recommended remove", Value: float64(f.Totals.RecommendedRemove), Color: colorWarning},
	}
	r.text(marginLeft, top, 10, fontBold, colorText, "Automatic filter recommendation")
	r.donut(marginLeft+74, top+74, 50, 24, segments)
	legendX := marginLeft + 155
	r.legend(legendX, top+28, segments)
	r.y = math.Max(top+128, top+28+r.legendHeight(pageWidth-marginRight-legendX, segments)+12)
}

func (r *pdfReportRenderer) filterQueryBars(summaries []BlastFilterQuerySummary) {
	if len(summaries) == 0 {
		return
	}
	r.subheading("Per-query recommendation and final selection")
	type item struct {
		label string
		reco  int
		final int
		diff  int
	}
	items := make([]item, 0, len(summaries))
	for _, summary := range summaries {
		label := valueOr(summary.Query, "query")
		if summary.Family != "" {
			label += " / " + summary.Family
		}
		items = append(items, item{
			label: label,
			reco:  summary.RecommendedKeep,
			final: summary.FinalSelected,
			diff:  summary.Difference,
		})
	}
	maxValue := 1
	for _, item := range items {
		if item.reco > maxValue {
			maxValue = item.reco
		}
		if item.final > maxValue {
			maxValue = item.final
		}
	}
	labelW := 112.0
	valueW := 108.0
	barW := pageWidth - marginLeft - marginRight - labelW - valueW - 18
	if barW < 180 {
		barW = 180
	}
	totalH := 18.0
	rowHeights := make([]float64, len(items))
	valueLines := make([][]string, len(items))
	labelLineSets := make([][]string, len(items))
	for i, item := range items {
		labelLineSets[i] = wrapText(item.label, labelW-4, 7.4)
		valueText := fmt.Sprintf("auto %d / final %d / diff %+d", item.reco, item.final, item.diff)
		valueLines[i] = wrapText(valueText, valueW-4, 7.2)
		rowHeights[i] = math.Max(35, math.Max(float64(len(labelLineSets[i]))*8.8+5, float64(len(valueLines[i]))*8.6+25))
		totalH += rowHeights[i]
	}
	r.ensure(totalH + 8)
	for idx, item := range items {
		y := r.y
		labelLines := labelLineSets[idx]
		lineY := y + 9
		for _, line := range labelLines {
			r.text(marginLeft, lineY, 7.4, fontRegular, colorText, line)
			lineY += 8.8
		}
		x := marginLeft + labelW
		r.rect(x, y, barW, 10, pdfColor{0.94, 0.95, 0.96}, colorRule, 0.2)
		recoW := barW * float64(item.reco) / float64(maxValue)
		finalW := barW * float64(item.final) / float64(maxValue)
		if recoW > 0 {
			r.fillRect(x, y, recoW, 10, colorWarning)
		}
		if finalW > 0 {
			r.fillRect(x, y+12, finalW, 10, colorPrimary)
		}
		valueY := y + 8
		for _, line := range valueLines[idx] {
			r.text(x+barW+8, valueY, 7.2, fontRegular, colorMuted, line)
			valueY += 8.6
		}
		r.y += rowHeights[idx]
	}
	r.y += 8
}

func (r *pdfReportRenderer) filterDifferenceChart(f *BlastFilterReport) {
	if f.Totals.TotalRows <= 0 {
		return
	}
	r.ensure(150)
	top := r.y
	segments := []chartSegment{
		{Label: "agreement", Value: float64(f.Totals.MatchedRows), Color: colorSuccess},
		{Label: "user rescued", Value: float64(f.Totals.UserRescued), Color: colorWarning},
		{Label: "user removed after keep", Value: float64(f.Totals.UserRemoved), Color: colorSecondary},
	}
	r.text(marginLeft, top, 10, fontBold, colorText, "Filter recommendation versus final user choice")
	r.donut(marginLeft+74, top+74, 50, 24, segments)
	legendX := marginLeft + 155
	r.legend(legendX, top+28, segments)
	r.y = math.Max(top+128, top+28+r.legendHeight(pageWidth-marginRight-legendX, segments)+12)
	r.paragraph("This chart compares the automatic filter suggestion with the user's final selection for every row present in this export scope.", 8.8, colorMuted, pageWidth-marginLeft-marginRight)
}

func (r *pdfReportRenderer) filterRuleFailureBars(rules []BlastFilterRuleSummary) {
	if len(rules) == 0 {
		return
	}
	r.ensure(42 + float64(len(rules))*28)
	r.text(marginLeft, r.y, 10, fontBold, colorText, "Hard-rule pass and failure totals")
	r.y += 18
	maxTotal := 1
	for _, rule := range rules {
		if rule.Passed+rule.Failed > maxTotal {
			maxTotal = rule.Passed + rule.Failed
		}
	}
	labelW := 132.0
	barW := pageWidth - marginLeft - marginRight - labelW - 28
	for _, rule := range rules {
		y := r.y
		labelLines := wrapText(rule.Name, labelW-4, 7.5)
		lineY := y + 9
		for _, line := range labelLines {
			r.text(marginLeft, lineY, 7.5, fontRegular, colorText, line)
			lineY += 8.8
		}
		x := marginLeft + labelW
		passW := barW * float64(rule.Passed) / float64(maxTotal)
		failW := barW * float64(rule.Failed) / float64(maxTotal)
		if passW > 0 {
			r.fillRect(x, y, passW, 10, colorSuccess)
		}
		if failW > 0 {
			r.fillRect(x+passW, y, failW, 10, colorWarning)
		}
		r.strokeRect(x, y, barW, 10, colorRule, 0.2)
		r.text(x+barW+5, y+9, 7.2, fontRegular, colorMuted, fmt.Sprintf("%d pass / %d fail", rule.Passed, rule.Failed))
		r.y += math.Max(24, float64(len(labelLines))*8.8+5)
	}
	r.y += 8
}

func (r *pdfReportRenderer) blastCompletenessChart(columns []ColumnCompleteness) {
	filled := 0
	empty := 0
	for _, col := range columns {
		filled += col.FilledRows
		empty += col.EmptyRows
	}
	total := filled + empty
	if total <= 0 {
		r.note("BLAST column completeness chart was not rendered because no generated table cells were available.")
		return
	}
	r.ensure(160)
	top := r.y
	segments := []chartSegment{
		{Label: "cells with data", Value: float64(filled), Color: colorSuccess},
		{Label: "empty cells", Value: float64(empty), Color: colorMissing},
	}
	r.text(marginLeft, top, 10, fontBold, colorText, "BLAST generated table cell completeness")
	r.donut(marginLeft+76, top+74, 50, 24, segments)
	legendX := marginLeft + 155
	r.legend(legendX, top+35, segments)
	r.y = math.Max(top+132, top+35+r.legendHeight(pageWidth-marginRight-legendX, segments)+12)
	r.paragraph("This chart is computed from exported BLAST workbook columns only. It excludes report metadata, generated-file properties, and external-reference features that were not enabled.", 8.8, colorMuted, pageWidth-marginLeft-marginRight)
}

func (r *pdfReportRenderer) sequenceChart(seq SequenceAudit) {
	segments := []chartSegment{
		{Label: "written records", Value: float64(seq.WrittenCount), Color: colorSuccess},
		{Label: "skipped records", Value: float64(seq.SkippedCount), Color: colorWarning},
	}
	r.ensure(150)
	top := r.y
	r.text(marginLeft, top, 10, fontBold, colorText, "Sequence export completeness")
	r.donut(marginLeft+74, top+74, 50, 24, segments)
	legendX := marginLeft + 155
	r.legend(legendX, top+35, segments)
	r.y = math.Max(top+126, top+35+r.legendHeight(pageWidth-marginRight-legendX, segments)+12)
}

func (r *pdfReportRenderer) sequenceLengthDotPlot(summaries []SequenceQuerySummary, title string) {
	if len(summaries) == 0 {
		return
	}
	maxLen := 0
	for _, summary := range summaries {
		if summary.MaxLength > maxLen {
			maxLen = summary.MaxLength
		}
	}
	if maxLen <= 0 {
		return
	}
	labelW := 120.0
	plotW := pageWidth - marginLeft - marginRight - labelW - 54
	if plotW < 180 {
		plotW = 180
	}
	rowHeights := make([]float64, len(summaries))
	labelLines := make([][]string, len(summaries))
	totalH := 28.0
	for i, summary := range summaries {
		label := strings.TrimSpace(summary.QueryLabel)
		if label == "" {
			label = "query"
		}
		labelLines[i] = wrapText(label, labelW-4, 7.4)
		rowHeights[i] = math.Max(24, float64(len(labelLines[i]))*8.6+8)
		totalH += rowHeights[i]
	}
	r.ensure(totalH + 16)
	r.subheading(title)
	top := r.y
	axisX := marginLeft + labelW
	r.text(axisX, top, 7.6, fontRegular, colorMuted, "0 aa")
	r.text(axisX+plotW-28, top, 7.6, fontRegular, colorMuted, fmt.Sprintf("%d aa", maxLen))
	r.y += 16
	for i, summary := range summaries {
		y := r.y
		lineY := y + 8
		for _, line := range labelLines[i] {
			r.text(marginLeft, lineY, 7.4, fontRegular, colorText, line)
			lineY += 8.6
		}
		centerY := y + rowHeights[i]/2
		r.line(axisX, centerY, axisX+plotW, centerY, pdfColor{0.88, 0.9, 0.92}, 0.4)
		if summary.MaxLength > 0 {
			minX := axisX + plotW*float64(summary.MinLength)/float64(maxLen)
			maxX := axisX + plotW*float64(summary.MaxLength)/float64(maxLen)
			avgX := axisX + plotW*float64(summary.AverageLength)/float64(maxLen)
			r.line(minX, centerY, maxX, centerY, colorPrimary, 1.8)
			r.circle(minX, centerY, 2.2, colorSecondary)
			r.circle(maxX, centerY, 2.2, colorSecondary)
			r.circle(avgX, centerY, 3.1, colorWarning)
			r.text(axisX+plotW+8, y+8, 7.2, fontRegular, colorMuted, fmt.Sprintf("%d-%d aa", summary.MinLength, summary.MaxLength))
			r.text(axisX+plotW+8, y+16, 7.2, fontRegular, colorMuted, fmt.Sprintf("avg %d", summary.AverageLength))
		}
		r.y += rowHeights[i]
	}
	r.note("Each row shows one query or sequence-record class. The horizontal segment marks the observed minimum-to-maximum sequence length, and the dot marks the average written length.")
}

func percentText(value int, total int) string {
	if total <= 0 {
		return "not available"
	}
	return fmt.Sprintf("%.1f%%", float64(value)/float64(total)*100)
}

func maxIntReport(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func minIntReport(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func floatValue(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	var out float64
	if _, err := fmt.Sscanf(value, "%f", &out); err != nil {
		return 0
	}
	return out
}

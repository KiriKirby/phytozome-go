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
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/export"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/phytozome"
	"github.com/KiriKirby/phytozome-go/internal/prompt"
	"github.com/KiriKirby/phytozome-go/internal/ui"
)

type BlastWizard struct {
	phytozome *phytozome.Client
	prompt    *prompt.Prompter
	out       io.Writer
}

type QueryMode string

const (
	ModeBlast   QueryMode = "blast"
	ModeKeyword QueryMode = "keyword"
)

func NewBlastWizard(out io.Writer) *BlastWizard {
	httpClient := &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          32,
			MaxIdleConnsPerHost:   16,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	return &BlastWizard{
		phytozome: phytozome.NewClient(httpClient),
		prompt:    prompt.New(os.Stdin, out),
		out:       out,
	}
}

func (w *BlastWizard) Run(ctx context.Context) error {
	candidates, err := w.loadSpeciesCandidates(ctx)
	if err != nil {
		return err
	}

	mode, err := w.chooseMode()
	if err != nil {
		return err
	}

	var selected model.SpeciesCandidate
	needSelect := true

	for {
		if needSelect || selected.JBrowseName == "" {
			selected, err = w.selectSpecies(candidates)
			if err != nil {
				return err
			}
			if err := w.printSelection(selected); err != nil {
				return err
			}
		}

		switch mode {
		case ModeBlast:
			sequence, querySource, err := w.collectQuerySequence(ctx, candidates)
			if err != nil {
				return err
			}

			request := buildBlastRequest(selected, sequence)
			job, err := w.submitBlastWithRetry(ctx, request)
			if err != nil {
				return err
			}
			fmt.Fprintf(w.out, "Job submitted: %s\n", job.JobID)

			results, err := w.waitForBlastResultsWithRetry(ctx, job.JobID)
			if err != nil {
				return err
			}

			if err := w.printResults(results); err != nil {
				return err
			}

			selectedRows, err := w.selectBlastRows(results.Rows)
			if err != nil {
				if errors.Is(err, prompt.ErrBackToModeSelection) {
					selected = model.SpeciesCandidate{}
					needSelect = true
					mode, err = w.chooseMode()
					if err != nil {
						return err
					}
					continue
				}
				return err
			}

			if err := w.exportSelectionsWithRetry(ctx, selectedRows, querySource); err != nil {
				return err
			}
		case ModeKeyword:
			if err := w.runKeywordMode(ctx, selected); err != nil {
				if errors.Is(err, prompt.ErrBackToModeSelection) {
					selected = model.SpeciesCandidate{}
					needSelect = true
					mode, err = w.chooseMode()
					if err != nil {
						return err
					}
					continue
				}
				return err
			}
		default:
			return fmt.Errorf("unsupported mode %q", mode)
		}

		action, err := w.prompt.PostRunAction(string(mode))
		if err != nil {
			return err
		}

		switch action {
		case "repeat":
			// Run another BLAST with the same species
			needSelect = false
			continue
		case "change_species":
			// Force re-selection of species next loop
			selected = model.SpeciesCandidate{}
			needSelect = true
			continue
		case "change_mode":
			mode, err = w.chooseMode()
			if err != nil {
				return err
			}
			needSelect = false
			continue
		case "exit":
			return nil
		default:
			// Unknown action: exit to be safe
			return nil
		}
	}
}

func (w *BlastWizard) chooseMode() (QueryMode, error) {
	for {
		mode, err := w.prompt.ChooseMode()
		if err == nil {
			return QueryMode(mode), nil
		}
		if !w.retryWorkflowStep(fmt.Sprintf("choose mode: %v", err)) {
			return "", err
		}
	}
}

func (w *BlastWizard) loadSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {
	for {
		candidates, err := withSpinnerValue(w.out, "Loading species candidates from Phytozome...", func() ([]model.SpeciesCandidate, error) {
			return w.phytozome.FetchSpeciesCandidates(ctx)
		})
		if err == nil {
			return candidates, nil
		}
		if !w.retryWorkflowStep(fmt.Sprintf("load species candidates: %v", err)) {
			return nil, err
		}
	}
}

func (w *BlastWizard) selectSpecies(candidates []model.SpeciesCandidate) (model.SpeciesCandidate, error) {
	for {
		selected, err := w.prompt.SearchAndSelectSpecies(candidates, func(keyword string) []model.SpeciesCandidate {
			return phytozome.FilterSpeciesCandidates(candidates, keyword)
		})
		if err == nil {
			return selected, nil
		}
		if !w.retryWorkflowStep(fmt.Sprintf("select species: %v", err)) {
			return model.SpeciesCandidate{}, err
		}
	}
}

func (w *BlastWizard) runKeywordMode(ctx context.Context, selected model.SpeciesCandidate) error {
	for {
		keywordInput, err := w.prompt.KeywordInput()
		if err != nil {
			if !w.retryWorkflowStep(fmt.Sprintf("read keyword input: %v", err)) {
				return err
			}
			continue
		}
		keywords := parseKeywordTerms(keywordInput)
		if len(keywords) == 0 {
			fmt.Fprintln(w.out, "Keyword input was empty. Please enter a keyword query.")
			continue
		}

		groups, err := w.searchKeywordGroups(ctx, selected, keywords)
		if err != nil {
			if !w.retryWorkflowStep(fmt.Sprintf("search keyword results: %v", err)) {
				return err
			}
			continue
		}

		totalRows := countKeywordRows(groups)
		fmt.Fprintln(w.out)
		fmt.Fprintf(w.out, "Keyword mode selected for %s.\n", selected.DisplayLabel())
		fmt.Fprintf(w.out, "Search terms: %d\n", len(keywords))
		fmt.Fprintf(w.out, "Matched rows: %d\n", totalRows)
		fmt.Fprintln(w.out)
		if totalRows == 0 {
			fmt.Fprintln(w.out, "No keyword results were found in the selected species.")
			fmt.Fprintln(w.out, "These identifiers may belong to a different species or may not exist in this proteome.")
			fmt.Fprintln(w.out)
			return nil
		}

		selectedRows, err := w.selectKeywordRows(groups)
		if err != nil {
			if errors.Is(err, prompt.ErrBackToModeSelection) {
				return err
			}
			if !w.retryWorkflowStep(fmt.Sprintf("select keyword rows: %v", err)) {
				return err
			}
			continue
		}

		if err := w.exportKeywordSelectionsWithRetry(ctx, selectedRows); err != nil {
			return err
		}
		return nil
	}
}

func (w *BlastWizard) collectQuerySequence(ctx context.Context, candidates []model.SpeciesCandidate) (string, *model.QuerySequenceSource, error) {
	for {
		sequenceInput, err := w.prompt.SequenceInput()
		if err != nil {
			if !w.retryWorkflowStep(fmt.Sprintf("read query input: %v", err)) {
				return "", nil, err
			}
			continue
		}
		if strings.TrimSpace(sequenceInput) == "" {
			fmt.Fprintln(w.out, "Sequence input was empty. Please paste a sequence, FASTA entry, or Phytozome URL.")
			continue
		}

		sequence := sequenceInput
		var querySource *model.QuerySequenceSource
		if source, ok, err := w.resolveQuerySequenceInput(ctx, candidates, sequenceInput); err != nil {
			if !w.retryWorkflowStep(fmt.Sprintf("resolve query input: %v", err)) {
				return "", nil, err
			}
			continue
		} else if ok {
			querySource = source
			sequence = source.Sequence
			fmt.Fprintln(w.out, describeQuerySource(source))
			if source.GeneID != "" {
				fmt.Fprintf(w.out, "  Gene ID: %s\n", source.GeneID)
			}
			if source.TranscriptID != "" && source.TranscriptID != source.GeneID {
				fmt.Fprintf(w.out, "  Transcript ID: %s\n", source.TranscriptID)
			}
			if source.NormalizedURL != "" {
				fmt.Fprintf(w.out, "  URL: %s\n", source.NormalizedURL)
			}
			fmt.Fprintln(w.out)
		}

		return sequence, querySource, nil
	}
}

func (w *BlastWizard) submitBlastWithRetry(ctx context.Context, request model.BlastRequest) (model.BlastJob, error) {
	for {
		job, err := withSpinnerValue(w.out, "Submitting BLAST job...", func() (model.BlastJob, error) {
			return w.phytozome.SubmitBlast(ctx, request)
		})
		if err == nil {
			return job, nil
		}
		if !w.retryWorkflowStep(fmt.Sprintf("submit BLAST job: %v", err)) {
			return model.BlastJob{}, err
		}
	}
}

func (w *BlastWizard) waitForBlastResultsWithRetry(ctx context.Context, jobID string) (model.BlastResult, error) {
	for {
		results, err := w.waitForBlastResultsWithProgress(ctx, jobID, 3*time.Second, 5*time.Minute)
		if err == nil {
			return results, nil
		}
		if !w.retryWorkflowStep(fmt.Sprintf("wait for BLAST results for job %s: %v", jobID, err)) {
			return model.BlastResult{}, err
		}
	}
}

func (w *BlastWizard) selectBlastRows(rows []model.BlastResultRow) ([]model.BlastResultRow, error) {
	for {
		selectedRows, err := w.prompt.SelectBlastRows(rows)
		if err == nil {
			return selectedRows, nil
		}
		if !w.retryWorkflowStep(fmt.Sprintf("select BLAST rows: %v", err)) {
			return nil, err
		}
	}
}

func (w *BlastWizard) selectKeywordRows(groups []model.KeywordSearchGroup) ([]model.KeywordResultRow, error) {
	for {
		selectedRows, err := w.prompt.SelectKeywordRows(groups)
		if err == nil {
			return selectedRows, nil
		}
		if !w.retryWorkflowStep(fmt.Sprintf("select keyword rows: %v", err)) {
			return nil, err
		}
	}
}

func (w *BlastWizard) exportSelectionsWithRetry(ctx context.Context, rows []model.BlastResultRow, querySource *model.QuerySequenceSource) error {
	for {
		err := w.exportSelections(ctx, rows, querySource)
		if err == nil {
			return nil
		}
		if !w.retryWorkflowStep(fmt.Sprintf("export selections: %v", err)) {
			return err
		}
	}
}

func (w *BlastWizard) exportKeywordSelectionsWithRetry(ctx context.Context, rows []model.KeywordResultRow) error {
	for {
		err := w.exportKeywordSelections(ctx, rows)
		if err == nil {
			return nil
		}
		if !w.retryWorkflowStep(fmt.Sprintf("export keyword selections: %v", err)) {
			return err
		}
	}
}

func (w *BlastWizard) retryWorkflowStep(description string) bool {
	action, err := w.prompt.WorkflowErrorAction(description)
	if err != nil {
		return false
	}
	return action == "retry"
}

func (w *BlastWizard) printSelection(candidate model.SpeciesCandidate) error {
	fmt.Fprintln(w.out)
	fmt.Fprintln(w.out, "Selected species:")
	fmt.Fprintf(w.out, "  Label: %s\n", candidate.GenomeLabel)
	if candidate.CommonName != "" {
		fmt.Fprintf(w.out, "  Common name: %s\n", candidate.CommonName)
	}
	fmt.Fprintf(w.out, "  JBrowse name: %s\n", candidate.JBrowseName)
	fmt.Fprintf(w.out, "  Proteome ID: %d\n", candidate.ProteomeID)
	if candidate.ReleaseDate != "" {
		fmt.Fprintf(w.out, "  Release date: %s\n", candidate.ReleaseDate)
	}
	fmt.Fprintln(w.out)
	return nil
}

func (w *BlastWizard) printResults(results model.BlastResult) error {
	fmt.Fprintln(w.out)
	fmt.Fprintf(w.out, "BLAST completed: %s\n", results.Message)
	fmt.Fprintf(w.out, "Rows: %d\n", len(results.Rows))
	fmt.Fprintln(w.out)

	if len(results.Rows) == 0 {
		fmt.Fprintln(w.out, "No hits returned.")
		return nil
	}

	writer := tabwriter.NewWriter(w.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "row\tprotein\tspecies\te_value\tpercent_identity\talign_len\tstrands\tquery_id\tquery_from\tquery_to\ttarget_from\ttarget_to\tbitscore\tidentical\tpositives\tgaps\tquery_length\ttarget_length\tgene_report_url")
	for i, row := range results.Rows {
		fmt.Fprintf(
			writer,
			"%d\t%s\t%s\t%s\t%.2f\t%d\t%s\t%s\t%d\t%d\t%d\t%d\t%.2f\t%d\t%d\t%d\t%d\t%d\t%s\n",
			i+1,
			row.Protein,
			row.Species,
			row.EValue,
			row.PercentIdentity,
			row.AlignLength,
			row.Strands,
			row.QueryID,
			row.QueryFrom,
			row.QueryTo,
			row.TargetFrom,
			row.TargetTo,
			row.Bitscore,
			row.Identical,
			row.Positives,
			row.Gaps,
			row.QueryLength,
			row.TargetLength,
			row.GeneReportURL,
		)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush results table: %w", err)
	}

	return nil
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

func (w *BlastWizard) exportSelections(ctx context.Context, rows []model.BlastResultRow, querySource *model.QuerySequenceSource) error {
	fmt.Fprintln(w.out)
	fmt.Fprintf(w.out, "Preparing export for %d selected rows...\n", len(rows))

	baseName, err := w.prompt.ExportBaseName()
	if err != nil {
		return err
	}

	outputDir, err := applicationDir()
	if err != nil {
		return err
	}

	excelPath := filepath.Join(outputDir, baseName+".xlsx")
	textPath := filepath.Join(outputDir, baseName+".txt")

	exportMetadata := buildExportMetadata(baseName, querySource)
	if err := withSpinner(w.out, "Writing Excel file...", func() error {
		return export.WriteBlastResultsExcelWithMetadata(excelPath, rows, exportMetadata)
	}); err != nil {
		return err
	}

	records, err := w.fetchProteinSequenceRecords(ctx, rows)
	if err != nil {
		return err
	}
	records = prependQuerySequenceRecord(records, querySource, baseName)
	if err := withSpinner(w.out, "Writing peptide text file...", func() error {
		return export.WriteProteinSequencesText(textPath, records)
	}); err != nil {
		return err
	}

	fmt.Fprintf(w.out, "Excel: %s\n", excelPath)
	fmt.Fprintf(w.out, "Peptides: %s\n", textPath)
	return nil
}

func (w *BlastWizard) exportKeywordSelections(ctx context.Context, rows []model.KeywordResultRow) error {
	fmt.Fprintln(w.out)
	fmt.Fprintf(w.out, "Preparing keyword export for %d selected rows...\n", len(rows))

	baseName, err := w.prompt.ExportBaseName()
	if err != nil {
		return err
	}

	outputDir, err := applicationDir()
	if err != nil {
		return err
	}

	excelPath := filepath.Join(outputDir, baseName+".xlsx")
	textPath := filepath.Join(outputDir, baseName+".txt")

	if err := withSpinner(w.out, "Writing keyword Excel file...", func() error {
		return export.WriteKeywordResultsExcel(excelPath, rows)
	}); err != nil {
		return err
	}

	records, err := w.fetchKeywordProteinSequenceRecords(ctx, rows)
	if err != nil {
		return err
	}
	if err := withSpinner(w.out, "Writing peptide text file...", func() error {
		return export.WriteProteinSequencesText(textPath, records)
	}); err != nil {
		return err
	}

	fmt.Fprintf(w.out, "Excel: %s\n", excelPath)
	fmt.Fprintf(w.out, "Peptides: %s\n", textPath)
	return nil
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

func (w *BlastWizard) resolveURLQuerySequenceInput(ctx context.Context, candidates []model.SpeciesCandidate, input string, normalizedURL string) (*model.QuerySequenceSource, bool, error) {
	jbrowseName, reportType, identifier, err := parseGeneReportURL(normalizedURL)
	if err != nil {
		return nil, false, err
	}

	species, ok := findSpeciesCandidateByJBrowseName(candidates, jbrowseName)
	if !ok {
		return nil, false, fmt.Errorf("could not match gene report species %s to a known proteome", jbrowseName)
	}

	gene, err := withSpinnerValue(w.out, "Resolving gene report URL...", func() (*model.QuerySequenceSource, error) {
		var gene model.QuerySequenceSource
		gene.OriginalInputURL = strings.TrimSpace(input)
		gene.NormalizedURL = normalizedURL

		switch reportType {
		case "gene":
			rawGene, err := w.phytozome.FetchGeneByGeneID(ctx, species.ProteomeID, identifier)
			if err != nil {
				return nil, err
			}
			gene.GeneID = rawGene.PrimaryIdentifier
			transcript, err := rawGene.PrimaryTranscript("")
			if err != nil {
				return nil, err
			}
			sequence, err := w.phytozome.FetchProteinSequence(ctx, transcript.SecondaryIdentifier)
			if err != nil {
				return nil, err
			}
			gene.Sequence = sequence
			gene.TranscriptID = transcript.PrimaryIdentifier
			gene.ProteinID = transcript.Protein
			gene.OrganismShort = rawGene.OrganismShortName()
			gene.Annotation = rawGene.AnnotationVersion()
		case "transcript":
			rawGene, err := w.phytozome.FetchGeneByTranscript(ctx, species.ProteomeID, identifier)
			if err != nil {
				return nil, err
			}
			gene.GeneID = rawGene.PrimaryIdentifier
			transcript, err := rawGene.PrimaryTranscript(identifier)
			if err != nil {
				return nil, err
			}
			sequence, err := w.phytozome.FetchProteinSequence(ctx, transcript.SecondaryIdentifier)
			if err != nil {
				return nil, err
			}
			gene.Sequence = sequence
			gene.TranscriptID = transcript.PrimaryIdentifier
			gene.ProteinID = transcript.Protein
			gene.OrganismShort = rawGene.OrganismShortName()
			gene.Annotation = rawGene.AnnotationVersion()
		default:
			return nil, fmt.Errorf("unsupported report URL type %q", reportType)
		}
		if gene.GeneID == "" {
			gene.GeneID = identifier
		}

		return &gene, nil
	})
	if err != nil {
		return nil, false, err
	}

	return gene, true, nil
}

func (w *BlastWizard) fetchProteinSequenceRecords(ctx context.Context, rows []model.BlastResultRow) ([]model.ProteinSequenceRecord, error) {
	cache := make(map[string]string, len(rows))
	records := make([]model.ProteinSequenceRecord, 0, len(rows))
	progress := ui.NewProgressBar(w.out, "Fetching peptide sequences...", len(rows))
	completed := false
	defer func() {
		if completed {
			progress.Finish("Fetched peptide sequences.")
			return
		}
		progress.Finish("")
	}()

	for i, row := range rows {
		progress.Set(i)
		cacheKey := fmt.Sprintf("%d:%s", row.TargetID, row.Protein)

		sequence, ok := cache[cacheKey]
		if !ok {
			// If the parsed row contains an invalid TargetID (0), prompt the user before attempting any fetch.
			if row.TargetID == 0 {
				action, aerr := w.prompt.FetchErrorAction(fmt.Sprintf("row has invalid TargetID=0 for protein %s", row.Protein))
				if aerr != nil {
					return nil, aerr
				}
				switch action {
				case "retry":
					// User chose to retry: fall through to the normal fetch loop (it will likely fail and re-prompt).
				case "skip":
					// Skip this row entirely.
					goto SKIP_ROW
				case "abort":
					return nil, fmt.Errorf("aborted by user due to invalid TargetID for protein %s", row.Protein)
				default:
					// Defensive: treat unknown as abort.
					return nil, fmt.Errorf("aborted due to unknown action for invalid TargetID for protein %s", row.Protein)
				}
			}

			// Interactive fetch loop: allow retry/skip/abort when remote fetch fails.
			for {
				gene, err := w.phytozome.FetchGeneByProtein(ctx, row.TargetID, row.Protein)
				if err != nil {
					// Ask user what to do on gene fetch error
					action, aerr := w.prompt.FetchErrorAction(fmt.Sprintf("gene for protein %s in proteome %d: %v", row.Protein, row.TargetID, err))
					if aerr != nil {
						return nil, aerr
					}
					switch action {
					case "retry":
						// try again
						continue
					case "skip":
						// do not include this row
						goto SKIP_ROW
					case "abort":
						return nil, fmt.Errorf("aborted by user after fetch gene error: %w", err)
					default:
						// defensive: treat unknown as abort
						return nil, fmt.Errorf("aborted due to unknown action after fetch gene error: %w", err)
					}
				}

				// fetch protein sequence for the transcript id
				sequence, err = w.phytozome.FetchProteinSequence(ctx, gene.ID)
				if err != nil {
					action, aerr := w.prompt.FetchErrorAction(fmt.Sprintf("protein sequence for transcript id %s: %v", gene.ID, err))
					if aerr != nil {
						return nil, aerr
					}
					switch action {
					case "retry":
						// try again (this will re-run gene fetch + sequence fetch)
						continue
					case "skip":
						goto SKIP_ROW
					case "abort":
						return nil, fmt.Errorf("aborted by user after fetch sequence error: %w", err)
					default:
						return nil, fmt.Errorf("aborted due to unknown action after fetch sequence error: %w", err)
					}
				}

				// success: cache and proceed
				cache[cacheKey] = sequence
				break
			}
		}

		records = append(records, model.ProteinSequenceRecord{
			Header:   fmt.Sprintf(">%s|%s", row.Species, row.Protein),
			Sequence: sequence,
		})
		// continue to next row
		continue

	SKIP_ROW:
		// user chose to skip this record; do not append anything and continue
		continue
	}

	progress.Set(len(rows))
	completed = true
	return records, nil
}

func (w *BlastWizard) fetchKeywordProteinSequenceRecords(ctx context.Context, rows []model.KeywordResultRow) ([]model.ProteinSequenceRecord, error) {
	cache := make(map[string]string, len(rows))
	records := make([]model.ProteinSequenceRecord, 0, len(rows))
	progress := ui.NewProgressBar(w.out, "Fetching keyword peptide sequences...", len(rows))
	completed := false
	defer func() {
		if completed {
			progress.Finish("Fetched keyword peptide sequences.")
			return
		}
		progress.Finish("")
	}()

	for i, row := range rows {
		progress.Set(i)
		sequenceID := strings.TrimSpace(row.SequenceID)
		if sequenceID == "" {
			action, err := w.prompt.FetchErrorAction(fmt.Sprintf("keyword row %s is missing sequence id", row.TranscriptID))
			if err != nil {
				return nil, err
			}
			switch action {
			case "retry":
				continue
			case "skip":
				continue
			default:
				return nil, fmt.Errorf("aborted by user because keyword row %s is missing sequence id", row.TranscriptID)
			}
		}

		sequence, ok := cache[sequenceID]
		if !ok {
			for {
				var err error
				sequence, err = w.phytozome.FetchProteinSequence(ctx, sequenceID)
				if err == nil {
					cache[sequenceID] = sequence
					break
				}

				action, aerr := w.prompt.FetchErrorAction(fmt.Sprintf("protein sequence for keyword row %s: %v", row.TranscriptID, err))
				if aerr != nil {
					return nil, aerr
				}
				switch action {
				case "retry":
					continue
				case "skip":
					goto NEXT_KEYWORD_ROW
				default:
					return nil, fmt.Errorf("aborted by user after keyword sequence fetch error: %w", err)
				}
			}
		}

		records = append(records, model.ProteinSequenceRecord{
			Header:   fmt.Sprintf(">%s|%s", row.SequenceHeaderLabel, row.TranscriptID),
			Sequence: sequence,
		})

	NEXT_KEYWORD_ROW:
	}

	progress.Set(len(rows))
	completed = true
	return records, nil
}

func applicationDir() (string, error) {
	executablePath, err := os.Executable()
	if err == nil {
		executableDir := filepath.Dir(executablePath)
		if !strings.Contains(strings.ToLower(executableDir), strings.ToLower(os.TempDir())) {
			return executableDir, nil
		}
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve application directory: %w", err)
	}
	return workingDir, nil
}

func buildExportMetadata(baseName string, querySource *model.QuerySequenceSource) *model.ExportMetadata {
	if querySource == nil {
		return nil
	}

	return &model.ExportMetadata{
		GeneName:      baseName,
		GeneID:        querySource.GeneID,
		GeneReportURL: querySource.NormalizedURL,
	}
}

func prependQuerySequenceRecord(records []model.ProteinSequenceRecord, querySource *model.QuerySequenceSource, baseName string) []model.ProteinSequenceRecord {
	if querySource == nil {
		return records
	}

	header := ">" + buildQuerySequenceHeaderID(querySource)
	label := buildQuerySequenceLabel(querySource.OrganismShort, baseName)
	if label != "" {
		header += " (" + label + ")"
	}

	queryRecord := model.ProteinSequenceRecord{
		Header:   header,
		Sequence: querySource.Sequence,
	}

	return append([]model.ProteinSequenceRecord{queryRecord}, records...)
}

func buildQuerySequenceLabel(organismShort string, baseName string) string {
	baseName = strings.TrimSpace(baseName)
	if baseName == "" {
		return ""
	}
	if organismShort == "A.thaliana" && !strings.HasPrefix(strings.ToLower(baseName), "at") {
		return "At" + baseName
	}
	return baseName
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

func describeQuerySource(source *model.QuerySequenceSource) string {
	switch {
	case source.NormalizedURL != "":
		return "Resolved peptide sequence from gene report URL."
	case source.TranscriptID != "" || source.ProteinID != "" || source.GeneID != "":
		return "Resolved query metadata from FASTA header."
	default:
		return "Resolved query sequence metadata."
	}
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
	if !slices.Contains([]string{"gene", "transcript"}, strings.ToLower(segments[1])) {
		return "", false
	}

	parsed.Scheme = "https"
	parsed.Host = "phytozome-next.jgi.doe.gov"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), true
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

	pipeIndex := strings.LastIndex(header, "|")
	if pipeIndex < 0 {
		return nil, false
	}

	left := strings.TrimSpace(header[:pipeIndex])
	right := strings.TrimSpace(header[pipeIndex+1:])
	if right == "" {
		return nil, false
	}

	source := &model.QuerySequenceSource{
		Sequence: sequence,
	}
	source.ProteinID = right
	source.TranscriptID = right
	source.GeneID = stripTranscriptSuffix(right)

	fields := strings.Fields(left)
	switch len(fields) {
	case 0:
	case 1:
		source.OrganismShort = fields[0]
	default:
		source.OrganismShort = strings.Join(fields[:len(fields)-1], " ")
		source.Annotation = fields[len(fields)-1]
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

func stripTrailingParentheticalLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasSuffix(value, ")") {
		return value
	}

	open := strings.LastIndex(value, " (")
	if open < 0 {
		return value
	}

	label := value[open+2 : len(value)-1]
	if label == "" {
		return value
	}
	for _, ch := range label {
		if ch == ' ' || ch == '\t' {
			return value
		}
	}
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

func (w *BlastWizard) searchKeywordGroups(ctx context.Context, species model.SpeciesCandidate, keywords []string) ([]model.KeywordSearchGroup, error) {
	groups := make([]model.KeywordSearchGroup, 0, len(keywords))
	progress := ui.NewProgressBar(w.out, "Searching keyword terms...", len(keywords))
	completed := false
	defer func() {
		if completed {
			progress.Finish("Keyword search completed.")
			return
		}
		progress.Finish("")
	}()
	for i, keyword := range keywords {
		progress.Set(i)
		rows, err := w.phytozome.SearchKeywordRows(ctx, species, keyword)
		if err != nil {
			return nil, err
		}
		groups = append(groups, model.KeywordSearchGroup{
			SearchTerm: keyword,
			Rows:       rows,
		})
	}
	progress.Set(len(keywords))
	completed = true
	return groups, nil
}

func (w *BlastWizard) waitForBlastResultsWithProgress(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, error) {
	type resultPayload struct {
		result model.BlastResult
		err    error
	}

	done := make(chan resultPayload, 1)
	go func() {
		result, err := w.phytozome.WaitForBlastResults(ctx, jobID, pollInterval, timeout)
		done <- resultPayload{result: result, err: err}
	}()

	if timeout <= 0 {
		return withSpinnerValue(w.out, "Waiting for BLAST results...", func() (model.BlastResult, error) {
			payload := <-done
			return payload.result, payload.err
		})
	}

	progress := ui.NewProgressBar(w.out, "Waiting for BLAST results...", int(timeout/time.Second))

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	start := time.Now()

	for {
		select {
		case payload := <-done:
			if payload.err != nil {
				progress.Finish("")
				return model.BlastResult{}, payload.err
			}
			progress.Set(int(timeout / time.Second))
			progress.Finish("BLAST results are ready.")
			return payload.result, nil
		case <-ticker.C:
			progress.Set(int(time.Since(start) / time.Second))
		case <-ctx.Done():
			progress.Finish("")
			return model.BlastResult{}, ctx.Err()
		}
	}
}

func withSpinner(out io.Writer, label string, fn func() error) error {
	spinner := ui.NewSpinner(out, label)
	spinner.Start()
	err := fn()
	if err != nil {
		spinner.Stop("")
		return err
	}
	spinner.Stop(label + " done.")
	return nil
}

func withSpinnerValue[T any](out io.Writer, label string, fn func() (T, error)) (T, error) {
	spinner := ui.NewSpinner(out, label)
	spinner.Start()
	value, err := fn()
	if err != nil {
		spinner.Stop("")
		var zero T
		return zero, err
	}
	spinner.Stop(label + " done.")
	return value, nil
}

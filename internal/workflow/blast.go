package workflow

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/wangsychn/phytozome-batch-cli/internal/model"
	"github.com/wangsychn/phytozome-batch-cli/internal/phytozome"
	"github.com/wangsychn/phytozome-batch-cli/internal/prompt"
)

type BlastWizard struct {
	phytozome *phytozome.Client
	prompt    *prompt.Prompter
	out       io.Writer
}

func NewBlastWizard(out io.Writer) *BlastWizard {
	httpClient := &http.Client{Timeout: 60 * time.Second}

	return &BlastWizard{
		phytozome: phytozome.NewClient(httpClient),
		prompt:    prompt.New(os.Stdin, out),
		out:       out,
	}
}

func (w *BlastWizard) Run(ctx context.Context) error {
	fmt.Fprintln(w.out, "Loading species candidates from Phytozome...")

	candidates, err := w.phytozome.FetchSpeciesCandidates(ctx)
	if err != nil {
		return err
	}

	keyword, err := w.prompt.SpeciesKeyword()
	if err != nil {
		return err
	}

	filtered := phytozome.FilterSpeciesCandidates(candidates, keyword)
	if len(filtered) == 0 {
		return fmt.Errorf("no species candidates matched %q", keyword)
	}

	if len(filtered) > 30 {
		filtered = filtered[:30]
		fmt.Fprintf(w.out, "Showing the first %d matches.\n", len(filtered))
	}

	selected, err := w.prompt.SelectSpecies(filtered)
	if err != nil {
		return err
	}

	if err := w.printSelection(selected); err != nil {
		return err
	}

	sequence, err := w.prompt.SequenceInput()
	if err != nil {
		return err
	}
	if strings.TrimSpace(sequence) == "" {
		return fmt.Errorf("sequence input was empty")
	}

	request := buildBlastRequest(selected, sequence)
	fmt.Fprintln(w.out, "Submitting BLAST job...")
	job, err := w.phytozome.SubmitBlast(ctx, request)
	if err != nil {
		return err
	}
	fmt.Fprintf(w.out, "Job submitted: %s\n", job.JobID)
	fmt.Fprintln(w.out, "Waiting for BLAST results...")

	results, err := w.phytozome.WaitForBlastResults(ctx, job.JobID, 3*time.Second, 5*time.Minute)
	if err != nil {
		return err
	}

	return w.printResults(results)
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
	request := model.BlastRequest{
		Species:          species,
		Sequence:         sequence,
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
	return cleaned
}

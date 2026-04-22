package workflow

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
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

	return w.printSelection(selected)
}

func (w *BlastWizard) printSelection(candidate model.SpeciesCandidate) error {
	fmt.Fprintln(w.out)
	fmt.Fprintln(w.out, "Selected species:")
	fmt.Fprintf(w.out, "  Label: %s\n", candidate.GenomeLabel)
	if candidate.CommonName != "" {
		fmt.Fprintf(w.out, "  Common name: %s\n", candidate.CommonName)
	}
	fmt.Fprintf(w.out, "  JBrowse name: %s\n", candidate.JBrowseName)
	if candidate.ReleaseDate != "" {
		fmt.Fprintf(w.out, "  Release date: %s\n", candidate.ReleaseDate)
	}
	fmt.Fprintln(w.out)
	fmt.Fprintln(w.out, "Next step is BLAST submission. That part is not implemented yet.")
	return nil
}

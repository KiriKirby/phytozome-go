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
	"runtime"
	"slices"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/blastplus"
	"github.com/KiriKirby/phytozome-go/internal/export"
	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/locale"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/phytozome"
	"github.com/KiriKirby/phytozome-go/internal/prompt"
	"github.com/KiriKirby/phytozome-go/internal/source"
	"github.com/KiriKirby/phytozome-go/internal/ui"
)

type BlastWizard struct {
	httpClient *http.Client
	source     source.DataSource
	prompt     *prompt.Prompter
	out        io.Writer

	speciesCandidatesMu    sync.Mutex
	speciesCandidatesCache map[string][]model.SpeciesCandidate
}

const (
	keywordSearchTimeout   = 30 * time.Second
	queryResolveTimeout    = 30 * time.Second
	proteinFetchTimeout    = 30 * time.Second
	maxParallelKeywordJobs = 6
	maxParallelQueryJobs   = 6
	maxParallelFetchJobs   = 8
)

type QueryMode string

const (
	ModeBlast   QueryMode = "blast"
	ModeKeyword QueryMode = "keyword"
)

type blastQueryItem struct {
	RawInput              string
	ProteinIdentification string
	Sequence              string
	QuerySource           *model.QuerySequenceSource
}

type blastBatchSettings struct {
	OutputDir      string
	ApproveAll     bool
	ReportPath     string
	AutoMode       bool
	AutoSelections bool
}

type blastQueryRun struct {
	Index        int
	Item         blastQueryItem
	Request      model.BlastRequest
	Results      model.BlastResult
	SelectedRows []model.BlastResultRow
	ExcelPath    string
	TextPath     string
}

type sequenceFetchResult struct {
	sequence string
	err      error
}

func NewBlastWizard(out io.Writer, lang locale.Language) *BlastWizard {
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
		httpClient:             httpClient,
		prompt:                 prompt.New(os.Stdin, out, lang),
		out:                    out,
		speciesCandidatesCache: make(map[string][]model.SpeciesCandidate),
	}
}

func (w *BlastWizard) Run(ctx context.Context) error {
databaseLoop:
	for {
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

		candidates, err := w.loadSpeciesCandidates(ctx)
		if errors.Is(err, prompt.ErrExitRequested) {
			return nil
		}
		if errors.Is(err, prompt.ErrBackToDatabaseSelection) {
			continue databaseLoop
		}
		if err != nil {
			return err
		}

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

			selected := model.SpeciesCandidate{}
			needSelect := true

		speciesLoop:
			for {
				if needSelect || selected.JBrowseName == "" {
					selected, err = w.selectSpecies(candidates)
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
					if err := w.printSelection(selected); err != nil {
						return err
					}
				}

				switch mode {
				case ModeBlast:
					if err := w.runBlastMode(ctx, selected, candidates); err != nil {
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
						return err
					}
				case ModeKeyword:
					if err := w.runKeywordMode(ctx, selected); err != nil {
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
						return err
					}
				default:
					return fmt.Errorf("unsupported mode %q", mode)
				}

				action, err := w.prompt.PostRunAction(string(mode))
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
				if err != nil {
					return err
				}

				switch action {
				case "repeat":
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
					return nil
				}
			}
		}
	}
}

func (w *BlastWizard) chooseMode() (QueryMode, error) {
	for {
		mode, err := w.prompt.ChooseMode()
		if err == nil {
			return QueryMode(mode), nil
		}
		if errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
			return "", err
		}
		retry, navErr := w.retryWorkflowStep(fmt.Sprintf("choose mode: %v", err), prompt.ErrBackToDatabaseSelection)
		if navErr != nil {
			return "", navErr
		}
		if !retry {
			return "", err
		}
	}
}

func (w *BlastWizard) chooseDataSource() (source.DataSource, error) {
	for {
		name, err := w.prompt.ChooseDatabase()
		if err != nil {
			if errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
				return nil, err
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("choose database: %v", err), prompt.ErrBackToDatabaseSelection)
			if navErr != nil {
				return nil, navErr
			}
			if !retry {
				return nil, err
			}
			continue
		}
		switch name {
		case "phytozome":
			return phytozome.NewClient(w.httpClient), nil
		case "lemna":
			return lemna.NewClient(w.httpClient), nil
		default:
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("choose database: unsupported database %q", name), prompt.ErrBackToDatabaseSelection)
			if navErr != nil {
				return nil, navErr
			}
			if !retry {
				return nil, fmt.Errorf("unsupported database %q", name)
			}
		}
	}
}

func (w *BlastWizard) configureBlastRequest(ctx context.Context, selected model.SpeciesCandidate, baseRequest model.BlastRequest) (model.BlastRequest, error) {
	request := baseRequest
	lc, ok := w.source.(*lemna.Client)
	if !ok {
		return request, nil
	}

	progs := lc.AvailableBlastPrograms(selected)
	if len(progs) == 0 {
		return model.BlastRequest{}, fmt.Errorf("no BLAST programs are available for %s based on detected lemna.org capabilities", selected.DisplayLabel())
	}
	chosenProg, err := w.prompt.ChooseBlastProgram(progs)
	if err != nil {
		return model.BlastRequest{}, err
	}

	applyBlastProgram(&request, chosenProg)
	execChoice, err := w.chooseLemnaBlastExecution(ctx, lc, selected, chosenProg)
	if err != nil {
		return model.BlastRequest{}, err
	}
	if execChoice == "local" {
		request.Program = "local:" + request.Program
	}
	return request, nil
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

func (w *BlastWizard) chooseLemnaBlastExecution(ctx context.Context, lc *lemna.Client, selected model.SpeciesCandidate, program string) (string, error) {
	cap, err := lc.DetectBlastCapabilities(ctx, selected)
	if err != nil {
		return "", err
	}
	serverOK := false
	localOK := false
	switch strings.ToLower(strings.TrimSpace(program)) {
	case "blastn", "tblastn":
		serverOK = cap.HasServerNucleotideDB
		localOK = cap.HasNucleotideFasta
	case "blastx", "blastp":
		serverOK = cap.HasServerProteinDB
		localOK = cap.HasProteinFasta
	}

	if serverOK && !localOK {
		fmt.Fprintln(w.out, "Execution target: server (local fallback not detected for this program).")
		return "server", nil
	}
	if !serverOK && localOK {
		fmt.Fprintln(w.out, "Execution target: local (lemna.org does not expose the required server DB for this program).")
		return "local", nil
	}
	if !serverOK && !localOK {
		return "", fmt.Errorf("no server or local execution target is available for %s on %s", program, selected.DisplayLabel())
	}
	return w.prompt.ChooseBlastExecution()
}

func (w *BlastWizard) loadSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {
	for {
		candidates, err := withSpinnerValue(w.out, fmt.Sprintf("Loading species candidates from %s...", w.source.Name()), func() ([]model.SpeciesCandidate, error) {
			return w.source.FetchSpeciesCandidates(ctx)
		})
		if err == nil {
			w.cacheSpeciesCandidates(w.source.Name(), candidates)
			return candidates, nil
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

func (w *BlastWizard) speciesCandidatesForSource(ctx context.Context, src source.DataSource, current []model.SpeciesCandidate) ([]model.SpeciesCandidate, error) {
	key := strings.ToLower(strings.TrimSpace(src.Name()))
	if key == "" {
		return nil, fmt.Errorf("source name is empty")
	}
	if key == strings.ToLower(strings.TrimSpace(w.source.Name())) && len(current) > 0 {
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
	// If lemna source and the candidate list is small, present the full list directly.
	const smallListThreshold = 16

	for {
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

func (w *BlastWizard) runKeywordMode(ctx context.Context, selected model.SpeciesCandidate) error {
	for {
		keywordInput, err := w.prompt.KeywordInput()
		if err != nil {
			if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
				return err
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read keyword input: %v", err), prompt.ErrBackToSpeciesSelection)
			if navErr != nil {
				return navErr
			}
			if !retry {
				return err
			}
			continue
		}
		keywords := parseKeywordTerms(keywordInput)
		if len(keywords) == 0 {
			fmt.Fprintln(w.out, "Keyword input was empty. Please enter a keyword query.")
			continue
		}

		proteinIdentifications, err := w.prompt.KeywordProteinIdentifications(len(keywords))
		if err != nil {
			if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
				return err
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read keyword protein identifications: %v", err), prompt.ErrBackToSpeciesSelection)
			if navErr != nil {
				return navErr
			}
			if !retry {
				return err
			}
			continue
		}

		baseName, err := w.prompt.ExportBaseName("File name", prompt.ErrBackToQueryInput)
		if err != nil {
			if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
				return err
			}
			if errors.Is(err, prompt.ErrBackToQueryInput) {
				continue
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read keyword file name: %v", err), prompt.ErrBackToSpeciesSelection)
			if navErr != nil {
				return navErr
			}
			if !retry {
				return err
			}
			continue
		}

		groups, err := w.searchKeywordGroups(ctx, selected, keywords, proteinIdentifications)
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

		totalRows := countKeywordRows(groups)
		fmt.Fprintln(w.out)
		fmt.Fprintf(w.out, "Keyword results for %s.\n", selected.DisplayLabel())
		fmt.Fprintf(w.out, "Search terms: %d\n", len(keywords))
		fmt.Fprintf(w.out, "Matched rows: %d\n", totalRows)
		fmt.Fprintln(w.out)
		if totalRows == 0 {
			fmt.Fprintln(w.out, "No keyword results were found in the selected species.")
			fmt.Fprintln(w.out, "These identifiers may belong to a different species or may not exist in this proteome.")
			fmt.Fprintln(w.out)
			return nil
		}

	keywordRowLoop:
		for {
			outputDir, err := appfs.OutputDir()
			if err != nil {
				return err
			}
			listPath := filepath.Join(outputDir, baseName+"_list.txt")

			selectedRows, err := w.selectKeywordRows(groups, listPath)
			if err != nil {
				if errors.Is(err, prompt.ErrBackToQueryInput) || errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
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

			if err := w.exportKeywordSelectionsWithRetry(ctx, selectedRows, baseName); err != nil {
				if errors.Is(err, prompt.ErrBackToRowSelection) {
					continue keywordRowLoop
				}
				return err
			}
			return nil
		}
	}
}

func (w *BlastWizard) runBlastMode(ctx context.Context, selected model.SpeciesCandidate, candidates []model.SpeciesCandidate) error {
	for {
		items, outputDir, err := w.collectBlastQueryItems()
		if err != nil {
			if errors.Is(err, prompt.ErrBackToQueryInput) {
				continue
			}
			if errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
				return err
			}
			retry, navErr := w.retryWorkflowStep(fmt.Sprintf("read BLAST input: %v", err), prompt.ErrBackToSpeciesSelection)
			if navErr != nil {
				return navErr
			}
			if !retry {
				return err
			}
			continue
		}
		if len(items) == 0 {
			fmt.Fprintln(w.out, "BLAST input was empty. Please paste one or more queries.")
			continue
		}

		prepared, err := w.resolveBlastQueryItems(ctx, items, candidates)
		if err != nil {
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
		if len(prepared) == 0 {
			return nil
		}

		rawOutputFolder := outputDir
	batchConfigLoop:
		for {
			baseRequest := buildBlastRequest(selected, prepared[0].Sequence)
			configuredRequest, err := w.configureBlastRequest(ctx, selected, baseRequest)
			if err != nil {
				if errors.Is(err, prompt.ErrBackToQueryInput) {
					continue
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

			resolvedOutputDir := ""
			if rawOutputFolder == "" {
				resolvedOutputDir, err = appfs.OutputDir()
				if err != nil {
					return err
				}
			} else {
				appDir, err := appfs.OutputDir()
				if err != nil {
					return err
				}
				resolvedOutputDir = filepath.Join(appDir, sanitizeExportName(rawOutputFolder))
				if err := os.MkdirAll(resolvedOutputDir, 0o755); err != nil {
					return fmt.Errorf("create output folder: %w", err)
				}
			}

			preview := w.buildBlastRunPreview(selected, prepared, configuredRequest, resolvedOutputDir)
			fmt.Fprintln(w.out)
			fmt.Fprintln(w.out, preview)
			if err := w.prompt.ConfirmReportPreview(); err != nil {
				return err
			}

			autoApproveRemaining := false
			queryRuns := make([]blastQueryRun, 0, len(prepared))
			for i, item := range prepared {
				if autoApproveRemaining {
					fmt.Fprintf(w.out, "Auto-approving query %d/%d with default selection because done all is active.\n", i+1, len(prepared))
				}
				request := configuredRequest
				request.Sequence = item.Sequence
				if item.QuerySource != nil {
					request.Sequence = item.QuerySource.Sequence
				}

			submitLoop:
				for {
					job, err := w.submitBlastWithRetry(ctx, request)
					if errors.Is(err, prompt.ErrBackToBlastProgram) {
						continue batchConfigLoop
					}
					if errors.Is(err, prompt.ErrExitRequested) {
						return err
					}
					if err != nil {
						action, actionErr := w.prompt.FetchErrorAction(fmt.Sprintf("BLAST query %d/%d (%s): submit BLAST job failed: %v", i+1, len(prepared), oneLinePreview(reportQueryLabel(item)), err), prompt.ErrBackToQueryInput)
						if actionErr != nil {
							return actionErr
						}
						switch action {
						case "retry":
							continue submitLoop
						case "skip":
							fmt.Fprintf(w.out, "Skipped BLAST query %d/%d after submission failed.\n", i+1, len(prepared))
							queryRuns = append(queryRuns, blastQueryRun{
								Index:   i + 1,
								Item:    item,
								Request: request,
							})
							break submitLoop
						case "exit":
							return prompt.ErrExitRequested
						default:
							return fmt.Errorf("unsupported submission recovery action %q", action)
						}
					}
					fmt.Fprintf(w.out, "Job submitted: %s\n", job.JobID)

				waitLoop:
					for {
						results, err := w.waitForBlastResultsWithRetry(ctx, job.JobID)
						if errors.Is(err, prompt.ErrExitRequested) || errors.Is(err, prompt.ErrBackToQueryInput) || errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) {
							return err
						}
						if err != nil {
							action, actionErr := w.prompt.FetchErrorAction(fmt.Sprintf("BLAST query %d/%d (%s): wait for results failed: %v", i+1, len(prepared), oneLinePreview(reportQueryLabel(item)), err), prompt.ErrBackToQueryInput)
							if actionErr != nil {
								return actionErr
							}
							switch action {
							case "retry":
								continue waitLoop
							case "skip":
								fmt.Fprintf(w.out, "Skipped BLAST query %d/%d after result retrieval failed.\n", i+1, len(prepared))
								queryRuns = append(queryRuns, blastQueryRun{
									Index:   i + 1,
									Item:    item,
									Request: request,
								})
								break submitLoop
							case "exit":
								return prompt.ErrExitRequested
							default:
								return fmt.Errorf("unsupported result recovery action %q", action)
							}
						}
						if err := w.printResults(results); err != nil {
							return err
						}
						if len(results.Rows) == 0 {
							fmt.Fprintln(w.out, "No BLAST results were returned for this query.")
							queryRuns = append(queryRuns, blastQueryRun{
								Index:   i + 1,
								Item:    item,
								Request: request,
								Results: results,
							})
							break submitLoop
						}

						var selectedRows []model.BlastResultRow
						if autoApproveRemaining {
							selectedRows = append(selectedRows, results.Rows...)
						} else {
							for {
								rows, doneAll, err := w.prompt.SelectBlastRowsBatch(results.Rows)
								if err != nil {
									if errors.Is(err, prompt.ErrBackToQueryInput) || errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
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
								selectedRows = rows
								autoApproveRemaining = doneAll
								break
							}
						}

						if len(selectedRows) == 0 {
							fmt.Fprintln(w.out, "No rows selected for this query. Skipping export.")
							queryRuns = append(queryRuns, blastQueryRun{
								Index:        i + 1,
								Item:         item,
								Request:      request,
								Results:      results,
								SelectedRows: nil,
							})
							break submitLoop
						}

					exportLoop:
						for {
							displayName := buildBlastOutputDisplayName(item)
							filePrefix := sanitizeExportName(displayName)
							excelPath := filepath.Join(resolvedOutputDir, filePrefix+".xlsx")
							textPath := filepath.Join(resolvedOutputDir, filePrefix+".txt")
							if err := w.exportBlastSelectionsToDir(ctx, selectedRows, item.QuerySource, displayName, filePrefix, resolvedOutputDir); err != nil {
								action, actionErr := w.prompt.FetchErrorAction(fmt.Sprintf("BLAST query %d/%d (%s): export failed: %v", i+1, len(prepared), oneLinePreview(reportQueryLabel(item)), err), prompt.ErrBackToRowSelection)
								if actionErr != nil {
									return actionErr
								}
								switch action {
								case "retry":
									continue exportLoop
								case "skip":
									fmt.Fprintf(w.out, "Skipped export for BLAST query %d/%d.\n", i+1, len(prepared))
									queryRuns = append(queryRuns, blastQueryRun{
										Index:        i + 1,
										Item:         item,
										Request:      request,
										Results:      results,
										SelectedRows: selectedRows,
									})
									break submitLoop
								case "exit":
									return prompt.ErrExitRequested
								default:
									return fmt.Errorf("unsupported export recovery action %q", action)
								}
							}
							queryRuns = append(queryRuns, blastQueryRun{
								Index:        i + 1,
								Item:         item,
								Request:      request,
								Results:      results,
								SelectedRows: selectedRows,
								ExcelPath:    excelPath,
								TextPath:     textPath,
							})
							break submitLoop
						}
					}
				}
			}

			action, err := w.prompt.DetailedReportAction()
			if err != nil {
				return err
			}
			if action == "yes" {
				reportPath := filepath.Join(resolvedOutputDir, time.Now().Format("20060102_150405")+"_blast_log.txt")
				if err := withSpinner(w.out, "Writing detailed run log...", func() error {
					return w.writeBlastDetailedReport(reportPath, selected, prepared, queryRuns, configuredRequest, resolvedOutputDir)
				}); err != nil {
					return err
				}
				fmt.Fprintf(w.out, "Detailed report written: %s\n", reportPath)
			}

			return nil
		}
	}
}

func (w *BlastWizard) collectBlastQueryItems() ([]blastQueryItem, string, error) {
	for {
		rawInput, err := w.prompt.SequenceInput()
		if err != nil {
			return nil, "", err
		}
		rawInput = strings.TrimSpace(rawInput)
		if rawInput == "" {
			return nil, "", nil
		}

		if loaded, ok, err := w.loadBlastInputFile(rawInput); err != nil {
			return nil, "", err
		} else if ok {
			rawInput = loaded
		}

		items, err := parseBlastQueryItems(rawInput)
		if err != nil {
			return nil, "", err
		}
		if len(items) == 0 {
			return nil, "", nil
		}

		if len(items) > 1 && !allLabelsBlank(items) && !allLabelsPresent(items) {
			fmt.Fprintln(w.out, "When you paste multiple BLAST queries, Protein Identification values must either be provided for all items or omitted for all items.")
			fmt.Fprintln(w.out, "Please re-enter the batch input.")
			continue
		}

		if allLabelsBlank(items) {
			labels, err := w.prompt.BlastProteinIdentifications(len(items), len(items) > 1)
			if err != nil {
				return nil, "", err
			}
			for i := range items {
				items[i].ProteinIdentification = labels[i]
			}
		}

		if len(items) > 1 {
			outputFolder, err := w.prompt.OutputFolderName()
			if err != nil {
				return nil, "", err
			}
			if strings.TrimSpace(outputFolder) != "" {
				return items, outputFolder, nil
			}
			return items, "", nil
		}

		return items, "", nil
	}
}

func allLabelsPresent(items []blastQueryItem) bool {
	for _, item := range items {
		if strings.TrimSpace(item.ProteinIdentification) == "" {
			return false
		}
	}
	return true
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
	data, err := withSpinnerValue(w.out, "Loading BLAST input file...", func() ([]byte, error) {
		return os.ReadFile(path)
	})
	if err != nil {
		return "", false, fmt.Errorf("load BLAST input file %q: %w", filename, err)
	}
	fmt.Fprintf(w.out, "Loaded BLAST input from %s.\n", path)
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
	if !strings.HasSuffix(strings.ToLower(rest), ".txt") {
		return "", false
	}
	return rest, true
}

func parseBlastQueryItems(rawInput string) ([]blastQueryItem, error) {
	text := strings.ReplaceAll(strings.TrimSpace(rawInput), "\r", "")
	if text == "" {
		return nil, nil
	}

	if items, ok, err := parseBlastClipboardItems(text); err != nil {
		return nil, err
	} else if ok {
		return items, nil
	}

	if source, ok := parseFastaQuerySequenceInput(text); ok {
		return []blastQueryItem{{RawInput: text, Sequence: source.Sequence, QuerySource: source}}, nil
	}

	lines := make([]string, 0, 8)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return nil, nil
	}
	items := make([]blastQueryItem, 0, len(lines))
	for _, line := range lines {
		items = append(items, blastQueryItem{RawInput: line})
	}
	return items, nil
}

func parseBlastClipboardItems(text string) ([]blastQueryItem, bool, error) {
	lines := make([]string, 0, 8)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	marker := -1
	for i, line := range lines {
		if strings.Trim(line, "~") == "" && len(strings.TrimSpace(line)) >= 2 {
			marker = i
			break
		}
	}
	if marker < 0 {
		return nil, false, nil
	}

	left := lines[:marker]
	right := lines[marker+1:]
	if len(left) == 0 || len(right) == 0 {
		return nil, true, fmt.Errorf("clipboard list format must contain labels above ~~ and links below ~~")
	}
	if len(left) != len(right) {
		return nil, true, fmt.Errorf("clipboard list format mismatch: %d labels vs %d links", len(left), len(right))
	}

	items := make([]blastQueryItem, 0, len(right))
	for i := range right {
		items = append(items, blastQueryItem{
			ProteinIdentification: strings.TrimSpace(strings.Trim(left[i], " ")),
			RawInput:              right[i],
		})
	}
	return items, true, nil
}

func allLabelsBlank(items []blastQueryItem) bool {
	for _, item := range items {
		if strings.TrimSpace(item.ProteinIdentification) != "" {
			return false
		}
	}
	return true
}

func buildBlastOutputDisplayName(item blastQueryItem) string {
	label := strings.TrimSpace(item.ProteinIdentification)
	if label == "" && item.QuerySource != nil {
		label = firstNonEmpty(item.QuerySource.GeneID, item.QuerySource.TranscriptID, item.QuerySource.ProteinID)
	}
	if label == "" {
		label = "query"
	}
	return label
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
	label := strings.TrimSpace(item.ProteinIdentification)
	if label != "" {
		return label
	}
	if item.QuerySource != nil {
		return firstNonEmpty(item.QuerySource.GeneID, item.QuerySource.TranscriptID, item.QuerySource.ProteinID, "query")
	}
	return "query"
}

func reportQuerySource(source *model.QuerySequenceSource) string {
	if source == nil {
		return "raw sequence input"
	}
	if source.NormalizedURL != "" {
		return source.NormalizedURL
	}
	if source.ProteinID != "" || source.TranscriptID != "" || source.GeneID != "" {
		return firstNonEmpty(source.ProteinID, source.TranscriptID, source.GeneID)
	}
	return "raw sequence input"
}

func blastExecutionLabel(program string) string {
	if strings.HasPrefix(strings.ToLower(program), "local:") {
		return "local"
	}
	return "server"
}

func (w *BlastWizard) resolveBlastQueryItems(ctx context.Context, items []blastQueryItem, candidates []model.SpeciesCandidate) ([]blastQueryItem, error) {
	type queryResolveResult struct {
		index       int
		querySource *model.QuerySequenceSource
		ok          bool
		err         error
	}

	prepared := make([]blastQueryItem, 0, len(items))
	progress := ui.NewProgressBar(w.out, "Resolving BLAST query inputs...", len(items))
	completed := false
	defer func() {
		if completed {
			progress.Finish("Resolved BLAST query inputs.")
			return
		}
		progress.Finish("")
	}()

	results := make([]queryResolveResult, len(items))
	jobs := make(chan int)
	outcomes := make(chan queryResolveResult, len(items))
	workerCount := parallelismFor(len(items), maxParallelQueryJobs)

	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for idx := range jobs {
				querySource, ok, err := w.resolveQuerySequenceInputBatchWithTimeout(ctx, candidates, items[idx].RawInput)
				outcomes <- queryResolveResult{index: idx, querySource: querySource, ok: ok, err: err}
			}
		}()
	}

	go func() {
		for i := range items {
			jobs <- i
		}
		close(jobs)
		workers.Wait()
		close(outcomes)
	}()

	doneCount := 0
	for result := range outcomes {
		results[result.index] = result
		doneCount++
		progress.Set(doneCount)
	}

	for i, item := range items {
		querySource := results[i].querySource
		ok := results[i].ok
		err := results[i].err
		for err != nil {
			action, actionErr := w.prompt.FetchErrorAction(fmt.Sprintf("BLAST query %d/%d (%s): %v", i+1, len(items), oneLinePreview(reportQueryLabel(item)), err), prompt.ErrBackToQueryInput)
			if actionErr != nil {
				return nil, actionErr
			}
			switch action {
			case "retry":
				querySource, ok, err = w.resolveQuerySequenceInputBatchWithTimeout(ctx, candidates, item.RawInput)
				continue
			case "skip":
				err = nil
				ok = false
				querySource = nil
			case "exit":
				return nil, prompt.ErrExitRequested
			default:
				return nil, fmt.Errorf("unsupported recovery action %q while resolving BLAST query input", action)
			}
		}
		sequence := item.RawInput
		if ok {
			sequence = querySource.Sequence
		}
		if strings.TrimSpace(sanitizeSequence(sequence)) == "" {
			fmt.Fprintf(w.out, "Skipped BLAST query %d/%d because no usable sequence could be resolved.\n", i+1, len(items))
			continue
		}
		if querySource == nil {
			querySource = &model.QuerySequenceSource{
				Sequence:       sequence,
				SourceDatabase: w.source.Name(),
			}
		}
		item.Sequence = sequence
		item.QuerySource = querySource
		prepared = append(prepared, item)
	}
	progress.Set(len(items))
	completed = true
	return prepared, nil
}

func (w *BlastWizard) resolveQuerySequenceInputBatchWithTimeout(ctx context.Context, candidates []model.SpeciesCandidate, input string) (*model.QuerySequenceSource, bool, error) {
	resolveCtx, cancel := context.WithTimeout(ctx, queryResolveTimeout)
	defer cancel()
	return w.resolveQuerySequenceInputBatch(resolveCtx, candidates, input)
}

func (w *BlastWizard) buildBlastRunPreview(selected model.SpeciesCandidate, items []blastQueryItem, request model.BlastRequest, outputDir string) string {
	lines := make([]string, 0, len(items)+8)
	lines = append(lines, "BLAST preview")
	lines = append(lines, fmt.Sprintf("db=%s | species=%s | mode=blast | program=%s | exec=%s", databaseDisplayName(w.source.Name()), selected.DisplayLabel(), request.Program, blastExecutionLabel(request.Program)))
	lines = append(lines, fmt.Sprintf("queries=%d | out=%s", len(items), outputDir))
	lines = append(lines, "Review the summary and press Enter to continue.")
	for i, item := range items {
		lines = append(lines, fmt.Sprintf("[%d] id=%s | len=%d | src=%s", i+1, displayBlankAsTilde(item.ProteinIdentification), len(sanitizeSequence(item.Sequence)), oneLinePreview(reportQuerySource(item.QuerySource))))
	}
	return strings.Join(lines, "\n")
}

func (w *BlastWizard) writeBlastDetailedReport(reportPath string, selected model.SpeciesCandidate, items []blastQueryItem, runs []blastQueryRun, request model.BlastRequest, outputDir string) error {
	lines := make([]string, 0, 1024)
	lines = append(lines, "Blast detailed log report")
	lines = append(lines, fmt.Sprintf("Generated: %s", time.Now().Format(time.RFC3339)))
	lines = append(lines, fmt.Sprintf("Database: %s", databaseDisplayName(w.source.Name())))
	lines = append(lines, fmt.Sprintf("Species: %s", selected.DisplayLabel()))
	lines = append(lines, fmt.Sprintf("Mode: blast"))
	lines = append(lines, fmt.Sprintf("Program: %s", request.Program))
	lines = append(lines, fmt.Sprintf("Sequence kind: %s", request.SequenceKind))
	lines = append(lines, fmt.Sprintf("Target type: %s", request.TargetType))
	lines = append(lines, fmt.Sprintf("E-value: %s", request.EValue))
	lines = append(lines, fmt.Sprintf("Matrix: %s", request.ComparisonMatrix))
	lines = append(lines, fmt.Sprintf("Word length: %s", request.WordLength))
	lines = append(lines, fmt.Sprintf("Alignments to show: %d", request.AlignmentsToShow))
	lines = append(lines, fmt.Sprintf("Allow gaps: %t", request.AllowGaps))
	lines = append(lines, fmt.Sprintf("Filter query: %t", request.FilterQuery))
	lines = append(lines, fmt.Sprintf("Output directory: %s", outputDir))
	lines = append(lines, fmt.Sprintf("Query count: %d", len(items)))
	lines = append(lines, fmt.Sprintf("Recorded runs: %d", len(runs)))
	lines = append(lines, "")
	for i, item := range items {
		lines = append(lines, fmt.Sprintf("=== Query %d ===", i+1))
		lines = append(lines, fmt.Sprintf("Label: %s", displayBlankAsTilde(item.ProteinIdentification)))
		lines = append(lines, fmt.Sprintf("Raw input: %s", oneLinePreview(item.RawInput)))
		lines = append(lines, fmt.Sprintf("Sequence length: %d", len(sanitizeSequence(item.Sequence))))
		queryDatabase := w.source.Name()
		if item.QuerySource != nil && strings.TrimSpace(item.QuerySource.SourceDatabase) != "" {
			queryDatabase = item.QuerySource.SourceDatabase
		}
		lines = append(lines, fmt.Sprintf("Query source database: %s", databaseDisplayName(queryDatabase)))
		lines = append(lines, fmt.Sprintf("Resolved source: %s", reportQuerySource(item.QuerySource)))
		if i < len(runs) {
			run := runs[i]
			lines = append(lines, fmt.Sprintf("Submitted program: %s", run.Request.Program))
			lines = append(lines, fmt.Sprintf("Selected rows: %d", len(run.SelectedRows)))
			lines = append(lines, fmt.Sprintf("Result rows: %d", len(run.Results.Rows)))
			lines = append(lines, fmt.Sprintf("Excel: %s", run.ExcelPath))
			lines = append(lines, fmt.Sprintf("Text: %s", run.TextPath))
			lines = append(lines, "Result table:")
			lines = append(lines, "row\tprotein\tspecies\te_value\tpercent_identity\talign_len\tstrands\tquery_id\tquery_from\tquery_to\ttarget_from\ttarget_to\tbitscore\tidentical\tpositives\tgaps\tquery_length\ttarget_length\tgene_report_url")
			for idx, row := range run.Results.Rows {
				lines = append(lines, fmt.Sprintf("%d\t%s\t%s\t%s\t%.2f\t%d\t%s\t%s\t%d\t%d\t%d\t%d\t%.2f\t%d\t%d\t%d\t%d\t%d\t%s",
					idx+1,
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
				))
			}
			lines = append(lines, "Selected rows:")
			for _, row := range run.SelectedRows {
				lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%s", row.Protein, row.Species, row.EValue, row.GeneReportURL))
			}
		}
		lines = append(lines, "")
	}
	report := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(reportPath, []byte(report), 0o644)
}

func displayBlankAsTilde(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "~"
	}
	return value
}

func oneLinePreview(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if len(value) > 120 {
		return value[:117] + "..."
	}
	return value
}

func parallelismFor(total int, maxWorkers int) int {
	if total <= 1 {
		return total
	}
	workers := runtime.GOMAXPROCS(0)
	if workers < 2 {
		workers = 2
	}
	if maxWorkers > 0 && workers > maxWorkers {
		workers = maxWorkers
	}
	if workers > total {
		workers = total
	}
	return workers
}

func (w *BlastWizard) exportBlastSelectionsToDir(ctx context.Context, rows []model.BlastResultRow, querySource *model.QuerySequenceSource, displayName string, fileBaseName string, outputDir string) error {
	fmt.Fprintln(w.out)
	fmt.Fprintf(w.out, "Preparing export for %d selected rows...\n", len(rows))

	excelPath := filepath.Join(outputDir, fileBaseName+".xlsx")
	textPath := filepath.Join(outputDir, fileBaseName+".txt")

	exportMetadata := buildExportMetadata(displayName, querySource)
	if err := withSpinner(w.out, "Writing Excel file...", func() error {
		return export.WriteBlastResultsExcelWithMetadata(excelPath, rows, exportMetadata)
	}); err != nil {
		return err
	}

	records, err := w.fetchProteinSequenceRecords(ctx, rows)
	if err != nil {
		return err
	}
	records = prependQuerySequenceRecord(records, querySource, displayName)
	if err := withSpinner(w.out, "Writing peptide text file...", func() error {
		return export.WriteProteinSequencesText(textPath, records)
	}); err != nil {
		return err
	}

	fmt.Fprintf(w.out, "Excel: %s\n", excelPath)
	fmt.Fprintf(w.out, "Peptides: %s\n", textPath)
	return nil
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
			fmt.Fprintln(w.out, "Sequence input was empty. Please paste a sequence, FASTA entry, or Phytozome URL.")
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
			fmt.Fprintln(w.out, describeQuerySource(source, w.source.Name()))
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
			return w.source.SubmitBlast(ctx, request)
		})
		if err == nil {
			return job, nil
		}
		var missingTools *blastplus.MissingToolsError
		if errors.As(err, &missingTools) {
			action, actionErr := w.prompt.BlastPlusInstallAction(missingTools.Error())
			if actionErr != nil {
				return model.BlastJob{}, actionErr
			}
			switch action {
			case "install":
				if _, installErr := withSpinnerValue(w.out, "Installing BLAST+...", func() (string, error) {
					return blastplus.InstallManaged(ctx, w.httpClient)
				}); installErr != nil {
					err = fmt.Errorf("install BLAST+: %w", installErr)
				} else {
					continue
				}
			case "back":
				return model.BlastJob{}, prompt.ErrBackToBlastProgram
			default:
				return model.BlastJob{}, prompt.ErrExitRequested
			}
		}
		action, actionErr := w.prompt.BlastSubmitErrorAction(fmt.Sprintf("submit BLAST job: %v", err))
		if actionErr != nil {
			return model.BlastJob{}, actionErr
		}
		switch action {
		case "retry":
			continue
		case "back":
			return model.BlastJob{}, prompt.ErrBackToBlastProgram
		default:
			return model.BlastJob{}, prompt.ErrExitRequested
		}
	}
}

func (w *BlastWizard) waitForBlastResultsWithRetry(ctx context.Context, jobID string) (model.BlastResult, error) {
	for {
		results, err := w.waitForBlastResultsWithProgress(ctx, jobID, 3*time.Second, 5*time.Minute)
		if err == nil {
			return results, nil
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

func (w *BlastWizard) selectBlastRows(rows []model.BlastResultRow) ([]model.BlastResultRow, error) {
	for {
		selectedRows, err := w.prompt.SelectBlastRows(rows)
		if err == nil {
			return selectedRows, nil
		}
		if errors.Is(err, prompt.ErrBackToQueryInput) || errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
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

func (w *BlastWizard) selectKeywordRows(groups []model.KeywordSearchGroup, listPath string) ([]model.KeywordResultRow, error) {
	for {
		selectedRows, err := w.prompt.SelectKeywordRows(groups, listPath, export.WriteKeywordListText)
		if err == nil {
			return selectedRows, nil
		}
		if errors.Is(err, prompt.ErrBackToQueryInput) || errors.Is(err, prompt.ErrBackToSpeciesSelection) || errors.Is(err, prompt.ErrBackToModeSelection) || errors.Is(err, prompt.ErrBackToDatabaseSelection) || errors.Is(err, prompt.ErrExitRequested) {
			return nil, err
		}
		retry, navErr := w.retryWorkflowStep(fmt.Sprintf("select keyword rows: %v", err), prompt.ErrBackToQueryInput)
		if navErr != nil {
			return nil, navErr
		}
		if !retry {
			return nil, err
		}
	}
}

func (w *BlastWizard) exportSelectionsWithRetry(ctx context.Context, rows []model.BlastResultRow, querySource *model.QuerySequenceSource, baseName string) error {
	for {
		err := w.exportSelections(ctx, rows, querySource, baseName)
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

func (w *BlastWizard) exportKeywordSelectionsWithRetry(ctx context.Context, rows []model.KeywordResultRow, baseName string) error {
	for {
		err := w.exportKeywordSelections(ctx, rows, baseName)
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

func (w *BlastWizard) retryWorkflowStep(description string, backTarget error) (bool, error) {
	action, err := w.prompt.WorkflowErrorAction(description, backTarget)
	if err != nil {
		return false, err
	}
	switch action {
	case "retry":
		return true, nil
	case "back":
		if backTarget != nil {
			return false, backTarget
		}
		return false, prompt.ErrBackToQueryInput
	case "exit":
		return false, prompt.ErrExitRequested
	default:
		return false, nil
	}
}

func (w *BlastWizard) printSelection(candidate model.SpeciesCandidate) error {
	fmt.Fprintln(w.out)
	fmt.Fprintln(w.out, "Selected species:")
	fmt.Fprintf(w.out, "  Label: %s\n", candidate.GenomeLabel)
	if candidate.CommonName != "" {
		fmt.Fprintf(w.out, "  Common name: %s\n", candidate.CommonName)
	}
	fmt.Fprintf(w.out, "  JBrowse name: %s\n", candidate.JBrowseName)
	if candidate.ProteomeID != 0 {
		fmt.Fprintf(w.out, "  Target ID: %d\n", candidate.ProteomeID)
	}
	if candidate.ReleaseDate != "" {
		fmt.Fprintf(w.out, "  Release date: %s\n", candidate.ReleaseDate)
	}

	// If the selected data source is lemna, detect and print a concise capability summary
	if c, ok := w.source.(*lemna.Client); ok {
		// Best-effort detection; DetectBlastCapabilities is conservative and may rely
		// on cached release metadata rather than fragile page parsing.
		cap, err := c.DetectBlastCapabilities(context.Background(), candidate)
		fmt.Fprintln(w.out)
		fmt.Fprintln(w.out, "lemna.org capability summary:")
		if err != nil {
			fmt.Fprintf(w.out, "  Could not detect capabilities: %v\n", err)
		} else {
			// Show available programs as reported by the client helper.
			progs := c.AvailableBlastPrograms(candidate)
			if len(progs) > 0 {
				fmt.Fprintf(w.out, "  Available programs: %s\n", strings.Join(progs, ", "))
			} else {
				fmt.Fprintln(w.out, "  Available programs: (none detected)")
			}

			// Server nucleotide DB capability
			if cap.HasServerNucleotideDB {
				fmt.Fprintf(w.out, "  Server BLASTn: available (DB id %d)\n", cap.BlastNDBID)
			} else {
				fmt.Fprintln(w.out, "  Server BLASTn: unavailable or no DB id exposed")
			}
			if cap.HasNucleotideFasta {
				fmt.Fprintf(w.out, "  Nucleotide FASTA (local fallback): %s\n", cap.NucleotideFastaURL)
			}

			// Protein DB / FASTA capability
			if cap.HasServerProteinDB {
				fmt.Fprintf(w.out, "  Server protein DB: available (DB id %d)\n", cap.ProteinDBID)
			} else if cap.HasProteinFasta {
				fmt.Fprintf(w.out, "  Protein FASTA (local fallback): %s\n", cap.ProteinFastaURL)
			} else {
				fmt.Fprintln(w.out, "  Protein DB / FASTA: unavailable")
			}
		}
		fmt.Fprintln(w.out)
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

func (w *BlastWizard) exportSelections(ctx context.Context, rows []model.BlastResultRow, querySource *model.QuerySequenceSource, baseName string) error {
	fmt.Fprintln(w.out)
	fmt.Fprintf(w.out, "Preparing export for %d selected rows...\n", len(rows))

	outputDir, err := appfs.OutputDir()
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

func (w *BlastWizard) exportKeywordSelections(ctx context.Context, rows []model.KeywordResultRow, baseName string) error {
	fmt.Fprintln(w.out)
	fmt.Fprintf(w.out, "Preparing keyword export for %d selected rows...\n", len(rows))

	outputDir, err := appfs.OutputDir()
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
	jbrowseName, reportType, identifier, err := parseGeneReportURL(normalizedURL)
	if err != nil {
		return nil, false, err
	}

	resolverSource := w.source
	resolverCandidates, err := w.speciesCandidatesForSource(ctx, resolverSource, candidates)
	if err != nil {
		return nil, false, fmt.Errorf("load %s species list for URL resolution: %w", resolverSource.Name(), err)
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
		return nil, false, fmt.Errorf("could not match gene report species %s to a known species in %s or phytozome", jbrowseName, w.source.Name())
	}

	resolveLabel := databaseDisplayName(resolverSource.Name())
	gene, err := withSpinnerValue(w.out, "Resolving "+resolveLabel+" gene report URL...", func() (*model.QuerySequenceSource, error) {
		return w.resolveGeneReportSequence(ctx, resolverSource, species, reportType, identifier, input, normalizedURL)
	})
	if err != nil {
		return nil, false, err
	}

	return gene, true, nil
}

func (w *BlastWizard) resolveURLQuerySequenceInputBatch(ctx context.Context, candidates []model.SpeciesCandidate, input string, normalizedURL string) (*model.QuerySequenceSource, bool, error) {
	jbrowseName, reportType, identifier, err := parseGeneReportURL(normalizedURL)
	if err != nil {
		return nil, false, err
	}

	resolverSource := w.source
	resolverCandidates, err := w.speciesCandidatesForSource(ctx, resolverSource, candidates)
	if err != nil {
		return nil, false, fmt.Errorf("load %s species list for URL resolution: %w", resolverSource.Name(), err)
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
		return nil, false, fmt.Errorf("could not match gene report species %s to a known species in %s or phytozome", jbrowseName, w.source.Name())
	}

	gene, err := w.resolveGeneReportSequence(ctx, resolverSource, species, reportType, identifier, input, normalizedURL)
	if err != nil {
		return nil, false, err
	}
	return gene, true, nil
}

func (w *BlastWizard) resolveGeneReportSequence(ctx context.Context, resolverSource source.DataSource, species model.SpeciesCandidate, reportType, identifier, input, normalizedURL string) (*model.QuerySequenceSource, error) {
	switch reportType {
	case "gene", "transcript":
		resolved, err := resolverSource.FetchGeneQuerySequence(ctx, species, reportType, identifier)
		if err != nil {
			return nil, err
		}
		gene := *resolved
		gene.OriginalInputURL = strings.TrimSpace(input)
		gene.NormalizedURL = normalizedURL
		if gene.GeneID == "" {
			gene.GeneID = identifier
		}
		return &gene, nil
	default:
		return nil, fmt.Errorf("unsupported report URL type %q", reportType)
	}
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

	results := w.prefetchBlastSequences(ctx, rows, progress)

	for _, row := range rows {
		sequenceID := firstNonEmpty(row.SequenceID, row.TranscriptID, row.Protein)
		cacheKey := fmt.Sprintf("%d:%s", row.TargetID, sequenceID)

		sequence, ok := cache[cacheKey]
		if !ok {
			if prefetched, exists := results[cacheKey]; exists && prefetched.err == nil {
				sequence = prefetched.sequence
				cache[cacheKey] = sequence
				ok = true
			}
		}

		if !ok {
			// Interactive fetch loop: allow retry/skip/back/exit when remote fetch fails.
			for {
				var fetchErr error
				fetchCtx, cancel := context.WithTimeout(ctx, proteinFetchTimeout)
				sequence, fetchErr = w.source.FetchProteinSequence(fetchCtx, row.TargetID, sequenceID)
				cancel()
				if fetchErr != nil {
					action, aerr := w.prompt.FetchErrorAction(fmt.Sprintf("protein sequence for %s: %v", sequenceID, fetchErr), prompt.ErrBackToRowSelection)
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
					case "back":
						return nil, prompt.ErrBackToRowSelection
					case "exit":
						return nil, prompt.ErrExitRequested
					default:
						return nil, fmt.Errorf("exited due to unknown recovery action after fetch sequence error: %w", fetchErr)
					}
				}
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

	results := w.prefetchKeywordSequences(ctx, rows, progress)

	for _, row := range rows {
		sequenceID := strings.TrimSpace(row.SequenceID)
		if sequenceID == "" {
			action, err := w.prompt.FetchErrorAction(fmt.Sprintf("keyword row %s is missing sequence id", row.TranscriptID), prompt.ErrBackToRowSelection)
			if err != nil {
				return nil, err
			}
			switch action {
			case "retry":
				continue
			case "skip":
				continue
			case "back":
				return nil, prompt.ErrBackToRowSelection
			case "exit":
				return nil, prompt.ErrExitRequested
			default:
				return nil, fmt.Errorf("exited because keyword row %s is missing sequence id", row.TranscriptID)
			}
		}

		sequence, ok := cache[sequenceID]
		if !ok {
			if prefetched, exists := results[sequenceID]; exists && prefetched.err == nil {
				sequence = prefetched.sequence
				cache[sequenceID] = sequence
				ok = true
			}
		}

		if !ok {
			for {
				var err error
				fetchCtx, cancel := context.WithTimeout(ctx, proteinFetchTimeout)
				sequence, err = w.source.FetchProteinSequence(fetchCtx, 0, sequenceID)
				cancel()
				if err == nil {
					cache[sequenceID] = sequence
					break
				}

				action, aerr := w.prompt.FetchErrorAction(fmt.Sprintf("protein sequence for keyword row %s: %v", row.TranscriptID, err), prompt.ErrBackToRowSelection)
				if aerr != nil {
					return nil, aerr
				}
				switch action {
				case "retry":
					continue
				case "skip":
					goto NEXT_KEYWORD_ROW
				case "back":
					return nil, prompt.ErrBackToRowSelection
				case "exit":
					return nil, prompt.ErrExitRequested
				default:
					return nil, fmt.Errorf("exited after keyword sequence fetch error: %w", err)
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

func (w *BlastWizard) prefetchBlastSequences(ctx context.Context, rows []model.BlastResultRow, progress *ui.ProgressBar) map[string]sequenceFetchResult {
	type fetchTask struct {
		key      string
		targetID int
		id       string
	}

	taskByKey := make(map[string]fetchTask, len(rows))
	for _, row := range rows {
		sequenceID := firstNonEmpty(row.SequenceID, row.TranscriptID, row.Protein)
		if sequenceID == "" {
			continue
		}
		key := fmt.Sprintf("%d:%s", row.TargetID, sequenceID)
		taskByKey[key] = fetchTask{key: key, targetID: row.TargetID, id: sequenceID}
	}

	results := make(map[string]sequenceFetchResult, len(taskByKey))
	if len(taskByKey) == 0 {
		return results
	}

	tasks := make([]fetchTask, 0, len(taskByKey))
	for _, task := range taskByKey {
		tasks = append(tasks, task)
	}

	var mu sync.Mutex
	jobs := make(chan fetchTask)
	done := make(chan struct{}, len(tasks))
	workerCount := parallelismFor(len(tasks), maxParallelFetchJobs)

	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for task := range jobs {
				fetchCtx, cancel := context.WithTimeout(ctx, proteinFetchTimeout)
				sequence, err := w.source.FetchProteinSequence(fetchCtx, task.targetID, task.id)
				cancel()
				mu.Lock()
				results[task.key] = sequenceFetchResult{sequence: sequence, err: err}
				mu.Unlock()
				done <- struct{}{}
			}
		}()
	}

	go func() {
		for _, task := range tasks {
			jobs <- task
		}
		close(jobs)
		workers.Wait()
		close(done)
	}()

	completedCount := 0
	for range done {
		completedCount++
		progress.Set(completedCount)
	}
	return results
}

func (w *BlastWizard) prefetchKeywordSequences(ctx context.Context, rows []model.KeywordResultRow, progress *ui.ProgressBar) map[string]sequenceFetchResult {
	taskIDs := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		sequenceID := strings.TrimSpace(row.SequenceID)
		if sequenceID == "" {
			continue
		}
		if _, ok := seen[sequenceID]; ok {
			continue
		}
		seen[sequenceID] = struct{}{}
		taskIDs = append(taskIDs, sequenceID)
	}

	results := make(map[string]sequenceFetchResult, len(taskIDs))
	if len(taskIDs) == 0 {
		return results
	}

	var mu sync.Mutex
	jobs := make(chan string)
	done := make(chan struct{}, len(taskIDs))
	workerCount := parallelismFor(len(taskIDs), maxParallelFetchJobs)

	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for sequenceID := range jobs {
				fetchCtx, cancel := context.WithTimeout(ctx, proteinFetchTimeout)
				sequence, err := w.source.FetchProteinSequence(fetchCtx, 0, sequenceID)
				cancel()
				mu.Lock()
				results[sequenceID] = sequenceFetchResult{sequence: sequence, err: err}
				mu.Unlock()
				done <- struct{}{}
			}
		}()
	}

	go func() {
		for _, sequenceID := range taskIDs {
			jobs <- sequenceID
		}
		close(jobs)
		workers.Wait()
		close(done)
	}()

	completedCount := 0
	for range done {
		completedCount++
		progress.Set(completedCount)
	}
	return results
}

func buildExportMetadata(baseName string, querySource *model.QuerySequenceSource) *model.ExportMetadata {
	if querySource == nil {
		return nil
	}

	return &model.ExportMetadata{
		GeneName:      baseName,
		GeneID:        querySource.GeneID,
		GeneReportURL: firstNonEmpty(querySource.OriginalInputURL, querySource.NormalizedURL),
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
	if strings.EqualFold(strings.TrimSpace(organismShort), "A.thaliana") && !strings.HasPrefix(strings.ToLower(baseName), "at") {
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
	default:
		return strings.TrimSpace(name)
	}
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (w *BlastWizard) searchKeywordGroups(ctx context.Context, species model.SpeciesCandidate, keywords []string, identifications []string) ([]model.KeywordSearchGroup, error) {
	if len(identifications) != 0 && len(identifications) != len(keywords) {
		return nil, fmt.Errorf("keyword Protein Identification count %d does not match keyword count %d", len(identifications), len(keywords))
	}

	type keywordSearchResult struct {
		index int
		rows  []model.KeywordResultRow
		err   error
	}

	groups := make([]model.KeywordSearchGroup, len(keywords))
	progress := ui.NewProgressBar(w.out, "Searching keyword terms...", len(keywords))
	completed := false
	defer func() {
		if completed {
			progress.Finish("Keyword search completed.")
			return
		}
		progress.Finish("")
	}()

	results := make([]keywordSearchResult, len(keywords))
	jobs := make(chan int)
	outcomes := make(chan keywordSearchResult, len(keywords))
	workerCount := parallelismFor(len(keywords), maxParallelKeywordJobs)

	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for idx := range jobs {
				rows, err := w.searchKeywordRowsWithTimeout(ctx, species, keywords[idx])
				outcomes <- keywordSearchResult{index: idx, rows: rows, err: err}
			}
		}()
	}

	go func() {
		for i := range keywords {
			jobs <- i
		}
		close(jobs)
		workers.Wait()
		close(outcomes)
	}()

	doneCount := 0
	for result := range outcomes {
		results[result.index] = result
		doneCount++
		progress.Set(doneCount)
	}

	for i, keyword := range keywords {
		rows := results[i].rows
		err := results[i].err
		for err != nil {
			action, actionErr := w.prompt.FetchErrorAction(fmt.Sprintf("keyword %d/%d (%s): %v", i+1, len(keywords), keyword, err), prompt.ErrBackToQueryInput)
			if actionErr != nil {
				return nil, actionErr
			}
			switch action {
			case "retry":
				rows, err = w.searchKeywordRowsWithTimeout(ctx, species, keyword)
				continue
			case "skip":
				rows = nil
				err = nil
			case "exit":
				return nil, prompt.ErrExitRequested
			default:
				return nil, fmt.Errorf("unsupported keyword recovery action %q", action)
			}
		}
		proteinIdentification := ""
		if len(identifications) == len(keywords) {
			proteinIdentification = identifications[i]
		}
		for idx := range rows {
			rows[idx].SearchTerm = keyword
			rows[idx].ProteinIdentification = proteinIdentification
		}
		groups[i] = model.KeywordSearchGroup{
			SearchTerm:            keyword,
			ProteinIdentification: proteinIdentification,
			Rows:                  rows,
		}
	}
	progress.Set(len(keywords))
	completed = true
	return groups, nil
}

func (w *BlastWizard) searchKeywordRowsWithTimeout(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	searchCtx, cancel := context.WithTimeout(ctx, keywordSearchTimeout)
	defer cancel()
	return w.source.SearchKeywordRows(searchCtx, species, keyword)
}

func (w *BlastWizard) waitForBlastResultsWithProgress(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, error) {
	type resultPayload struct {
		result model.BlastResult
		err    error
	}

	done := make(chan resultPayload, 1)
	go func() {
		result, err := w.source.WaitForBlastResults(ctx, jobID, pollInterval, timeout)
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

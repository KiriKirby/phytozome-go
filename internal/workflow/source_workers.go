package workflow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/KiriKirby/phytozome-go/internal/source"
	"github.com/goccy/go-json"
)

const (
	sourceFetchSpeciesCandidatesWorker = "workflow.source.fetch_species_candidates"
	sourceSubmitBlastWorker            = "workflow.source.submit_blast"
	sourceRunBlastWorker               = "workflow.source.run_blast"
	sourcePrepareLocalBlastWorker      = "workflow.source.prepare_local_blast"
	sourceWaitBlastResultsWorker       = "workflow.source.wait_blast_results"
	sourceFetchProteinSequenceWorker   = "workflow.source.fetch_protein_sequence"
	sourceFetchUniProtAccessionsWorker = "workflow.source.fetch_uniprot_accessions"
	sourceFetchGeneQueryWorker         = "workflow.source.fetch_gene_query"
	sourceFetchProteinQueryWorker      = "workflow.source.fetch_protein_query"
	sourceSearchKeywordRowsWorker      = "workflow.source.search_keyword_rows"
)

var (
	workerSourceCacheMu sync.Mutex
	workerSourceCache   = make(map[string]source.DataSource)
)

type sourceFetchSpeciesCandidatesInput struct {
	Database string `json:"database"`
}

type sourceFetchSpeciesCandidatesOutput struct {
	Candidates []model.SpeciesCandidate `json:"candidates"`
}

type sourceSubmitBlastInput struct {
	Database string             `json:"database"`
	Request  model.BlastRequest `json:"request"`
	Local    bool               `json:"local"`
	Server   bool               `json:"server"`
}

type sourceSubmitBlastOutput struct {
	Job model.BlastJob `json:"job"`
}

type sourceRunBlastInput struct {
	Database     string             `json:"database"`
	Request      model.BlastRequest `json:"request"`
	Server       bool               `json:"server"`
	PollInterval time.Duration      `json:"poll_interval"`
	Timeout      time.Duration      `json:"timeout"`
}

type sourceRunBlastOutput struct {
	Job    model.BlastJob    `json:"job"`
	Result model.BlastResult `json:"result"`
}

type sourcePrepareLocalBlastInput struct {
	Database string             `json:"database"`
	Request  model.BlastRequest `json:"request"`
}

type sourcePrepareLocalBlastOutput struct{}

type sourceWaitBlastResultsInput struct {
	Database     string        `json:"database"`
	JobID        string        `json:"job_id"`
	PollInterval time.Duration `json:"poll_interval"`
	Timeout      time.Duration `json:"timeout"`
}

type sourceWaitBlastResultsOutput struct {
	Result model.BlastResult `json:"result"`
}

type sourceFetchProteinSequenceInput struct {
	Database   string `json:"database"`
	TargetID   int    `json:"target_id"`
	SequenceID string `json:"sequence_id"`
}

type sourceFetchProteinSequenceOutput struct {
	Sequence string `json:"sequence"`
}

type sourceFetchUniProtAccessionsInput struct {
	Database  string `json:"database"`
	TargetID  int    `json:"target_id"`
	ProteinID string `json:"protein_id"`
}

type sourceFetchUniProtAccessionsOutput struct {
	Accessions []string `json:"accessions"`
}

type sourceFetchGeneQueryInput struct {
	Database   string                 `json:"database"`
	Species    model.SpeciesCandidate `json:"species"`
	ReportType string                 `json:"report_type"`
	Identifier string                 `json:"identifier"`
}

type sourceFetchGeneQueryOutput struct {
	Query *model.QuerySequenceSource `json:"query"`
}

type sourceFetchProteinQueryInput struct {
	Database  string                 `json:"database"`
	Species   model.SpeciesCandidate `json:"species"`
	ProteinID string                 `json:"protein_id"`
}

type sourceFetchProteinQueryOutput struct {
	Query *model.QuerySequenceSource `json:"query"`
}

type sourceSearchKeywordRowsInput struct {
	Database string                 `json:"database"`
	Species  model.SpeciesCandidate `json:"species"`
	Keyword  string                 `json:"keyword"`
	Wide     bool                   `json:"wide"`
	Broad    bool                   `json:"broad"`
}

type sourceSearchKeywordRowsOutput struct {
	Rows []model.KeywordResultRow `json:"rows"`
}

func init() {
	phygoboost.Register(sourceFetchSpeciesCandidatesWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input sourceFetchSpeciesCandidatesInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode source species input: %w", err)
		}
		src, err := workerSourceForDatabase(input.Database)
		if err != nil {
			return nil, err
		}
		candidates, err := src.FetchSpeciesCandidates(ctx)
		if err != nil {
			return nil, err
		}
		return json.Marshal(sourceFetchSpeciesCandidatesOutput{Candidates: candidates})
	})
	phygoboost.Register(sourceSubmitBlastWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input sourceSubmitBlastInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode source BLAST submit input: %w", err)
		}
		src, err := workerSourceForDatabase(input.Database)
		if err != nil {
			return nil, err
		}
		var job model.BlastJob
		if lc, ok := src.(*lemna.Client); ok {
			switch {
			case input.Local:
				job, err = lc.SubmitBlast(ctx, input.Request)
			case input.Server:
				job, err = lc.SubmitBlastServerOnly(ctx, input.Request)
			default:
				job, err = lc.SubmitBlast(ctx, input.Request)
			}
		} else {
			job, err = src.SubmitBlast(ctx, input.Request)
		}
		if err != nil {
			return nil, err
		}
		return json.Marshal(sourceSubmitBlastOutput{Job: job})
	})
	phygoboost.Register(sourceRunBlastWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input sourceRunBlastInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode source BLAST run input: %w", err)
		}
		src, err := workerSourceForDatabase(input.Database)
		if err != nil {
			return nil, err
		}
		var job model.BlastJob
		if lc, ok := src.(*lemna.Client); ok && input.Server {
			job, err = lc.SubmitBlastServerOnly(ctx, input.Request)
		} else {
			job, err = src.SubmitBlast(ctx, input.Request)
		}
		if err != nil {
			return nil, err
		}
		result, err := src.WaitForBlastResults(ctx, job.JobID, input.PollInterval, input.Timeout)
		if err != nil {
			return nil, err
		}
		return json.Marshal(sourceRunBlastOutput{Job: job, Result: result})
	})
	phygoboost.Register(sourcePrepareLocalBlastWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input sourcePrepareLocalBlastInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode source local BLAST prepare input: %w", err)
		}
		src, err := workerSourceForDatabase(input.Database)
		if err != nil {
			return nil, err
		}
		lc, ok := src.(*lemna.Client)
		if !ok {
			return nil, fmt.Errorf("%s does not support local BLAST preparation", input.Database)
		}
		if err := lemna.PrepareLocalBlast(ctx, lc, input.Request); err != nil {
			return nil, err
		}
		return nil, nil
	})
	phygoboost.Register(sourceWaitBlastResultsWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input sourceWaitBlastResultsInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode source BLAST wait input: %w", err)
		}
		src, err := workerSourceForDatabase(input.Database)
		if err != nil {
			return nil, err
		}
		result, err := src.WaitForBlastResults(ctx, input.JobID, input.PollInterval, input.Timeout)
		if err != nil {
			return nil, err
		}
		return json.Marshal(sourceWaitBlastResultsOutput{Result: result})
	})
	phygoboost.Register(sourceFetchProteinSequenceWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input sourceFetchProteinSequenceInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode source protein sequence input: %w", err)
		}
		src, err := workerSourceForDatabase(input.Database)
		if err != nil {
			return nil, err
		}
		sequence, err := src.FetchProteinSequence(ctx, input.TargetID, input.SequenceID)
		if err != nil {
			return nil, err
		}
		return json.Marshal(sourceFetchProteinSequenceOutput{Sequence: sequence})
	})
	phygoboost.Register(sourceFetchUniProtAccessionsWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input sourceFetchUniProtAccessionsInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode source UniProt input: %w", err)
		}
		src, err := workerSourceForDatabase(input.Database)
		if err != nil {
			return nil, err
		}
		resolver, ok := src.(source.UniProtResolver)
		if !ok {
			return nil, fmt.Errorf("%s does not support UniProt accession lookup", src.Name())
		}
		accessions, err := resolver.FetchUniProtAccessions(ctx, input.TargetID, input.ProteinID)
		if err != nil {
			return nil, err
		}
		return json.Marshal(sourceFetchUniProtAccessionsOutput{Accessions: accessions})
	})
	phygoboost.Register(sourceFetchGeneQueryWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input sourceFetchGeneQueryInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode source gene query input: %w", err)
		}
		src, err := workerSourceForDatabase(input.Database)
		if err != nil {
			return nil, err
		}
		query, err := src.FetchGeneQuerySequence(ctx, input.Species, input.ReportType, input.Identifier)
		if err != nil {
			return nil, err
		}
		return json.Marshal(sourceFetchGeneQueryOutput{Query: query})
	})
	phygoboost.Register(sourceFetchProteinQueryWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input sourceFetchProteinQueryInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode source protein query input: %w", err)
		}
		src, err := workerSourceForDatabase(input.Database)
		if err != nil {
			return nil, err
		}
		resolver, ok := src.(source.ProteinReportResolver)
		if !ok {
			return nil, fmt.Errorf("%s does not support protein report URL resolution", databaseDisplayName(src.Name()))
		}
		query, err := resolver.FetchProteinQuerySequence(ctx, input.Species, input.ProteinID)
		if err != nil {
			return nil, err
		}
		return json.Marshal(sourceFetchProteinQueryOutput{Query: query})
	})
	phygoboost.Register(sourceSearchKeywordRowsWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input sourceSearchKeywordRowsInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode source keyword input: %w", err)
		}
		src, err := workerSourceForDatabase(input.Database)
		if err != nil {
			return nil, err
		}
		var rows []model.KeywordResultRow
		if input.Broad {
			searcher, ok := src.(source.BroadKeywordSearcher)
			if !ok {
				return nil, fmt.Errorf("%s does not support broad keyword search", src.Name())
			}
			rows, err = searcher.SearchKeywordRowsBroad(ctx, input.Species, input.Keyword)
		} else if input.Wide {
			searcher, ok := src.(source.WideKeywordSearcher)
			if !ok {
				return nil, fmt.Errorf("%s does not support wide keyword search", src.Name())
			}
			rows, err = searcher.SearchKeywordRowsWide(ctx, input.Species, input.Keyword)
		} else {
			rows, err = src.SearchKeywordRows(ctx, input.Species, input.Keyword)
		}
		if err != nil {
			return nil, err
		}
		return json.Marshal(sourceSearchKeywordRowsOutput{Rows: rows})
	})
}

func workerSourceForDatabase(database string) (source.DataSource, error) {
	database = strings.ToLower(strings.TrimSpace(database))
	if database == "" {
		return nil, fmt.Errorf("database is empty")
	}
	workerSourceCacheMu.Lock()
	defer workerSourceCacheMu.Unlock()
	if src, ok := workerSourceCache[database]; ok && src != nil {
		return src, nil
	}
	src, err := dataSourceForName(database, workerHTTPClient)
	if err != nil {
		return nil, err
	}
	workerSourceCache[database] = src
	return src, nil
}

func sourceProcessDatabase(src source.DataSource) string {
	if src == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(src.Name())) {
	case "phytozome":
		return "phytozome"
	case "lemna", "lemna.org":
		return "lemna"
	default:
		return ""
	}
}

func sourceProcessDomain(database string) string {
	switch strings.ToLower(strings.TrimSpace(database)) {
	case "phytozome":
		return "phytozome-next.jgi.doe.gov"
	case "lemna", "lemna.org":
		return "www.lemna.org"
	default:
		return ""
	}
}

func sourceWorkerTaskSpec(database string, description string) phygoboost.TaskSpec {
	return phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      sourceProcessDomain(database),
		Description: description,
	}
}

func sourceLocalTaskSpec(description string) phygoboost.TaskSpec {
	return phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		LocalSlots:  1,
		Description: description,
	}
}

func sourceSubmitBlastTaskSpec(database string, request model.BlastRequest) phygoboost.TaskSpec {
	spec := sourceWorkerTaskSpec(database, "submit blast job")
	if isLocalBlastRequest(request) {
		spec.LocalSlots = 1
	}
	return spec
}

func sourceWaitBlastTaskSpec(database string, jobID string) phygoboost.TaskSpec {
	if isLocalBlastJobID(jobID) {
		return sourceLocalTaskSpec("wait blast results")
	}
	return sourceWorkerTaskSpec(database, "wait blast results")
}

func isLocalBlastJobID(jobID string) bool {
	jobID = strings.ToLower(strings.TrimSpace(jobID))
	return strings.HasPrefix(jobID, "local-") || strings.HasPrefix(jobID, "local:")
}

func (w *BlastWizard) fetchSpeciesCandidatesProcess(ctx context.Context, src source.DataSource) ([]model.SpeciesCandidate, bool, error) {
	database := sourceProcessDatabase(src)
	if database == "" || phygoboost.InWorker() {
		return nil, false, nil
	}
	var output sourceFetchSpeciesCandidatesOutput
	err := phygoboost.RunTaskJSON(ctx, sourceWorkerTaskSpec(database, "fetch species candidates"), sourceFetchSpeciesCandidatesWorker, sourceFetchSpeciesCandidatesInput{
		Database: database,
	}, &output)
	return output.Candidates, true, err
}

func (w *BlastWizard) submitBlastProcess(ctx context.Context, request model.BlastRequest) (model.BlastJob, bool, error) {
	database := sourceProcessDatabase(w.source)
	if database == "" || phygoboost.InWorker() {
		return model.BlastJob{}, false, nil
	}
	var output sourceSubmitBlastOutput
	input := sourceSubmitBlastInput{
		Database: database,
		Request:  request,
		Local:    isLocalBlastRequest(request),
		Server:   database == "lemna" && !isLocalBlastRequest(request),
	}
	err := phygoboost.RunTaskJSON(ctx, sourceSubmitBlastTaskSpec(database, request), sourceSubmitBlastWorker, input, &output)
	return output.Job, true, err
}

func (w *BlastWizard) runBlastProcess(ctx context.Context, request model.BlastRequest, pollInterval time.Duration, timeout time.Duration) (model.BlastJob, model.BlastResult, bool, error) {
	database := sourceProcessDatabase(w.source)
	if database == "" || phygoboost.InWorker() || isLocalBlastRequest(request) {
		return model.BlastJob{}, model.BlastResult{}, false, nil
	}
	var output sourceRunBlastOutput
	err := phygoboost.RunTaskJSON(ctx, sourceWorkerTaskSpec(database, "run blast job"), sourceRunBlastWorker, sourceRunBlastInput{
		Database:     database,
		Request:      request,
		Server:       true,
		PollInterval: pollInterval,
		Timeout:      timeout,
	}, &output)
	return output.Job, output.Result, true, err
}

func (w *BlastWizard) prepareLocalBlastProcess(ctx context.Context, request model.BlastRequest) (bool, error) {
	database := sourceProcessDatabase(w.source)
	if database == "" || phygoboost.InWorker() {
		return false, nil
	}
	var output sourcePrepareLocalBlastOutput
	err := phygoboost.RunTaskJSON(ctx, sourceLocalTaskSpec("prepare local blast"), sourcePrepareLocalBlastWorker, sourcePrepareLocalBlastInput{
		Database: database,
		Request:  request,
	}, &output)
	return true, err
}

func (w *BlastWizard) waitBlastResultsProcess(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, bool, error) {
	database := sourceProcessDatabase(w.source)
	if database == "" || phygoboost.InWorker() {
		return model.BlastResult{}, false, nil
	}
	var output sourceWaitBlastResultsOutput
	err := phygoboost.RunTaskJSON(ctx, sourceWaitBlastTaskSpec(database, jobID), sourceWaitBlastResultsWorker, sourceWaitBlastResultsInput{
		Database:     database,
		JobID:        jobID,
		PollInterval: pollInterval,
		Timeout:      timeout,
	}, &output)
	return output.Result, true, err
}

func (w *BlastWizard) fetchProteinSequenceProcess(ctx context.Context, targetID int, sequenceID string) (string, bool, error) {
	database := sourceProcessDatabase(w.source)
	if database == "" || phygoboost.InWorker() {
		return "", false, nil
	}
	var output sourceFetchProteinSequenceOutput
	err := phygoboost.RunTaskJSON(ctx, sourceWorkerTaskSpec(database, "fetch protein sequence"), sourceFetchProteinSequenceWorker, sourceFetchProteinSequenceInput{
		Database:   database,
		TargetID:   targetID,
		SequenceID: sequenceID,
	}, &output)
	return output.Sequence, true, err
}

func (w *BlastWizard) fetchUniProtAccessionsProcess(ctx context.Context, targetID int, proteinID string) ([]string, bool, error) {
	return fetchUniProtAccessionsForSourceProcess(ctx, w.source, targetID, proteinID)
}

func fetchUniProtAccessionsForSourceProcess(ctx context.Context, src source.DataSource, targetID int, proteinID string) ([]string, bool, error) {
	return fetchUniProtAccessionsForDatabaseProcess(ctx, sourceProcessDatabase(src), targetID, proteinID)
}

func fetchUniProtAccessionsForDatabaseProcess(ctx context.Context, database string, targetID int, proteinID string) ([]string, bool, error) {
	database = strings.ToLower(strings.TrimSpace(database))
	if database == "" || phygoboost.InWorker() {
		return nil, false, nil
	}
	var output sourceFetchUniProtAccessionsOutput
	err := phygoboost.RunTaskJSON(ctx, sourceWorkerTaskSpec(database, "fetch uniprot accessions"), sourceFetchUniProtAccessionsWorker, sourceFetchUniProtAccessionsInput{
		Database:  database,
		TargetID:  targetID,
		ProteinID: proteinID,
	}, &output)
	return output.Accessions, true, err
}

func fetchGeneQuerySequenceProcess(ctx context.Context, src source.DataSource, species model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, bool, error) {
	database := sourceProcessDatabase(src)
	if database == "" || phygoboost.InWorker() {
		return nil, false, nil
	}
	var output sourceFetchGeneQueryOutput
	err := phygoboost.RunTaskJSON(ctx, sourceWorkerTaskSpec(database, "fetch gene query sequence"), sourceFetchGeneQueryWorker, sourceFetchGeneQueryInput{
		Database:   database,
		Species:    species,
		ReportType: reportType,
		Identifier: identifier,
	}, &output)
	return output.Query, true, err
}

func fetchProteinQuerySequenceProcess(ctx context.Context, src source.DataSource, species model.SpeciesCandidate, proteinID string) (*model.QuerySequenceSource, bool, error) {
	database := sourceProcessDatabase(src)
	if database == "" || phygoboost.InWorker() {
		return nil, false, nil
	}
	var output sourceFetchProteinQueryOutput
	err := phygoboost.RunTaskJSON(ctx, sourceWorkerTaskSpec(database, "fetch protein query sequence"), sourceFetchProteinQueryWorker, sourceFetchProteinQueryInput{
		Database:  database,
		Species:   species,
		ProteinID: proteinID,
	}, &output)
	return output.Query, true, err
}

func (w *BlastWizard) searchKeywordRowsProcess(ctx context.Context, species model.SpeciesCandidate, keyword string, wide bool) ([]model.KeywordResultRow, bool, error) {
	return searchKeywordRowsForSourceProcess(ctx, w.source, species, keyword, wide, false)
}

func searchKeywordRowsForSourceProcess(ctx context.Context, src source.DataSource, species model.SpeciesCandidate, keyword string, wide bool, broad bool) ([]model.KeywordResultRow, bool, error) {
	database := sourceProcessDatabase(src)
	if database == "" || phygoboost.InWorker() {
		return nil, false, nil
	}
	var output sourceSearchKeywordRowsOutput
	err := phygoboost.RunTaskJSON(ctx, sourceWorkerTaskSpec(database, "search keyword rows"), sourceSearchKeywordRowsWorker, sourceSearchKeywordRowsInput{
		Database: database,
		Species:  species,
		Keyword:  keyword,
		Wide:     wide,
		Broad:    broad,
	}, &output)
	return output.Rows, true, err
}

package workflow

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/KiriKirby/phytozome-go/internal/phytozome"
	"github.com/KiriKirby/phytozome-go/internal/source"
	"github.com/KiriKirby/phytozome-go/internal/tair"
)

func sourceDatabaseName(src source.DataSource) string {
	if src == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(src.Name())) {
	case "phytozome":
		return "phytozome"
	case "lemna", "lemna.org":
		return "lemna"
	case "tair":
		return "tair"
	default:
		return ""
	}
}

func sourceDomain(database string) string {
	switch strings.ToLower(strings.TrimSpace(database)) {
	case "phytozome":
		return "phytozome-next.jgi.doe.gov"
	case "lemna", "lemna.org":
		return "www.lemna.org"
	case "tair":
		return "www.arabidopsis.org"
	default:
		return ""
	}
}

func sourceManagedTaskSpec(database string, description string) phygoboost.TaskSpec {
	return phygoboost.TaskSpec{
		Level:       phygoboost.ExecManaged,
		Domain:      sourceDomain(database),
		Description: description,
	}
}

func sourceLocalTaskSpec(description string) phygoboost.TaskSpec {
	return phygoboost.TaskSpec{
		Level:       phygoboost.ExecManaged,
		Description: description,
	}
}

func sourceSubmitBlastTaskSpec(database string, request model.BlastRequest) phygoboost.TaskSpec {
	spec := sourceManagedTaskSpec(database, "submit blast job")
	if isLocalBlastRequest(request) {
		spec.Domain = ""
	}
	return spec
}

func sourceWaitBlastTaskSpec(database string, jobID string) phygoboost.TaskSpec {
	if isLocalBlastJobID(jobID) {
		return sourceLocalTaskSpec("wait blast results")
	}
	return sourceManagedTaskSpec(database, "wait blast results")
}

func isLocalBlastJobID(jobID string) bool {
	jobID = strings.ToLower(strings.TrimSpace(jobID))
	return strings.HasPrefix(jobID, "local-") || strings.HasPrefix(jobID, "local:")
}

func (w *BlastWizard) submitBlastManaged(ctx context.Context, request model.BlastRequest) (model.BlastJob, bool, error) {
	database := sourceDatabaseName(w.source)
	if database == "" {
		return model.BlastJob{}, false, nil
	}
	var job model.BlastJob
	err := phygoboost.RunTaskSpec(ctx, sourceSubmitBlastTaskSpec(database, request), func(runCtx context.Context) error {
		var err error
		if lc, ok := w.source.(*lemna.Client); ok {
			switch {
			case isLocalBlastRequest(request):
				job, err = lc.SubmitBlast(runCtx, request)
			default:
				job, err = lc.SubmitBlastServerOnly(runCtx, request)
			}
			return err
		}
		job, err = w.source.SubmitBlast(runCtx, request)
		return err
	})
	return job, true, err
}

func (w *BlastWizard) runBlastManaged(ctx context.Context, request model.BlastRequest, pollInterval time.Duration, timeout time.Duration) (model.BlastJob, model.BlastResult, bool, error) {
	database := sourceDatabaseName(w.source)
	if database == "" || isLocalBlastRequest(request) {
		return model.BlastJob{}, model.BlastResult{}, false, nil
	}
	var (
		job    model.BlastJob
		result model.BlastResult
	)
	err := phygoboost.RunTaskSpec(ctx, sourceManagedTaskSpec(database, "run blast job"), func(runCtx context.Context) error {
		var err error
		if lc, ok := w.source.(*lemna.Client); ok {
			job, err = lc.SubmitBlastServerOnly(runCtx, request)
		} else {
			job, err = w.source.SubmitBlast(runCtx, request)
		}
		if err != nil {
			return err
		}
		result, err = w.source.WaitForBlastResults(runCtx, job.JobID, pollInterval, timeout)
		return err
	})
	return job, result, true, err
}

func (w *BlastWizard) prepareLocalBlastManaged(ctx context.Context, request model.BlastRequest) (bool, error) {
	database := sourceDatabaseName(w.source)
	if database == "" {
		return false, nil
	}
	lc, ok := w.source.(*lemna.Client)
	if !ok {
		return false, nil
	}
	err := phygoboost.RunTaskSpec(ctx, sourceLocalTaskSpec("prepare local blast"), func(runCtx context.Context) error {
		_, err := lc.RunLocalBlast(runCtx, request)
		return err
	})
	return true, err
}

func (w *BlastWizard) waitBlastResultsManaged(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, bool, error) {
	database := sourceDatabaseName(w.source)
	if database == "" {
		return model.BlastResult{}, false, nil
	}
	var result model.BlastResult
	err := phygoboost.RunTaskSpec(ctx, sourceWaitBlastTaskSpec(database, jobID), func(runCtx context.Context) error {
		var err error
		result, err = w.source.WaitForBlastResults(runCtx, jobID, pollInterval, timeout)
		return err
	})
	return result, true, err
}

func (w *BlastWizard) fetchProteinSequenceManaged(ctx context.Context, targetID int, sequenceID string) (string, bool, error) {
	database := sourceDatabaseName(w.source)
	if database == "" {
		return "", false, nil
	}
	var sequence string
	err := phygoboost.RunTaskSpec(ctx, sourceManagedTaskSpec(database, "fetch protein sequence"), func(runCtx context.Context) error {
		var err error
		data, err := w.source.FetchProteinSequence(runCtx, targetID, sequenceID)
		if err == nil {
			sequence = data.Sequence
		}
		return err
	})
	return sequence, true, err
}

func (w *BlastWizard) fetchUniProtAccessionsManaged(ctx context.Context, targetID int, proteinID string) ([]string, bool, error) {
	return fetchUniProtAccessionsForSourceManaged(ctx, w.source, targetID, proteinID)
}

func fetchUniProtAccessionsForSourceManaged(ctx context.Context, src source.DataSource, targetID int, proteinID string) ([]string, bool, error) {
	return fetchUniProtAccessionsForDatabaseManaged(ctx, sourceDatabaseName(src), src, targetID, proteinID)
}

func fetchUniProtAccessionsForDatabaseManaged(ctx context.Context, database string, args ...any) ([]string, bool, error) {
	var (
		src       source.DataSource
		targetID  int
		proteinID string
	)
	switch len(args) {
	case 3:
		var ok bool
		src, ok = args[0].(source.DataSource)
		if !ok || src == nil {
			return nil, false, nil
		}
		targetID, _ = args[1].(int)
		proteinID, _ = args[2].(string)
	case 2:
		targetID, _ = args[0].(int)
		proteinID, _ = args[1].(string)
		if strings.EqualFold(strings.TrimSpace(database), "phytozome") {
			src = phytozomeHelperSourceManaged()
		} else if strings.EqualFold(strings.TrimSpace(database), "tair") {
			src = tair.NewClient(phygoboost.HTTPClient())
		}
	default:
		return nil, false, nil
	}
	database = strings.ToLower(strings.TrimSpace(database))
	if database == "" || src == nil {
		return nil, false, nil
	}
	resolver, ok := src.(source.UniProtResolver)
	if !ok {
		return nil, true, nil
	}
	var accessions []string
	err := phygoboost.RunTaskSpec(ctx, sourceManagedTaskSpec(database, "fetch uniprot accessions"), func(runCtx context.Context) error {
		var err error
		accessions, err = resolver.FetchUniProtAccessions(runCtx, targetID, proteinID)
		return err
	})
	return accessions, true, err
}

func phytozomeHelperSourceManaged() source.DataSource {
	return phytozome.NewClient(phygoboost.HTTPClient())
}

func fetchGeneQuerySequenceManaged(ctx context.Context, src source.DataSource, species model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, bool, error) {
	database := sourceDatabaseName(src)
	if database == "" {
		return nil, false, nil
	}
	var query *model.QuerySequenceSource
	err := phygoboost.RunTaskSpec(ctx, sourceManagedTaskSpec(database, "fetch gene query sequence"), func(runCtx context.Context) error {
		var err error
		query, err = src.FetchGeneQuerySequence(runCtx, species, reportType, identifier)
		return err
	})
	return query, true, err
}

func fetchProteinQuerySequenceManaged(ctx context.Context, src source.DataSource, species model.SpeciesCandidate, proteinID string) (*model.QuerySequenceSource, bool, error) {
	database := sourceDatabaseName(src)
	if database == "" {
		return nil, false, nil
	}
	resolver, ok := src.(source.ProteinReportResolver)
	if !ok {
		return nil, false, nil
	}
	var query *model.QuerySequenceSource
	err := phygoboost.RunTaskSpec(ctx, sourceManagedTaskSpec(database, "fetch protein query sequence"), func(runCtx context.Context) error {
		var err error
		query, err = resolver.FetchProteinQuerySequence(runCtx, species, proteinID)
		return err
	})
	return query, true, err
}

func resolverForQuerySource(query *model.QuerySequenceSource, httpClient *http.Client) (source.DataSource, string, bool) {
	if query == nil {
		return nil, "", false
	}
	database := strings.ToLower(strings.TrimSpace(query.SourceDatabase))
	if database == "" {
		return nil, "", false
	}
	w := &BlastWizard{httpClient: httpClient}
	src, err := w.dataSourceForDatabase(database)
	if err != nil || src == nil {
		return nil, "", false
	}
	return src, database, true
}

func (w *BlastWizard) searchKeywordRowsManaged(ctx context.Context, species model.SpeciesCandidate, keyword string, wide bool) ([]model.KeywordResultRow, bool, error) {
	return searchKeywordRowsForSourceManaged(ctx, w.source, species, keyword, wide, false)
}

func searchKeywordRowsForSourceManaged(ctx context.Context, src source.DataSource, species model.SpeciesCandidate, keyword string, wide bool, broad bool) ([]model.KeywordResultRow, bool, error) {
	database := sourceDatabaseName(src)
	if database == "" {
		return nil, false, nil
	}
	var rows []model.KeywordResultRow
	err := phygoboost.RunTaskSpec(ctx, sourceManagedTaskSpec(database, "search keyword rows"), func(runCtx context.Context) error {
		var err error
		if broad {
			searcher, ok := src.(source.BroadKeywordSearcher)
			if !ok {
				return nil
			}
			rows, err = searcher.SearchKeywordRowsBroad(runCtx, species, keyword)
			return err
		}
		if wide {
			searcher, ok := src.(source.WideKeywordSearcher)
			if !ok {
				return nil
			}
			rows, err = searcher.SearchKeywordRowsWide(runCtx, species, keyword)
			return err
		}
		rows, err = src.SearchKeywordRows(runCtx, species, keyword)
		return err
	})
	return rows, true, err
}

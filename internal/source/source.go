package source

import (
	"context"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

type DataSource interface {
	Name() string
	FetchSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error)
	SubmitBlast(ctx context.Context, req model.BlastRequest) (model.BlastJob, error)
	WaitForBlastResults(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, error)
	SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error)
	FetchProteinSequence(ctx context.Context, targetID int, sequenceID string) (string, error)
	FetchGeneQuerySequence(ctx context.Context, species model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, error)
}

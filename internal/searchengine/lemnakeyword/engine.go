package lemnakeyword

import (
	"context"

	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
)

// Engine is the new dedicated lemna keyword-search entry point.
// The full search implementation is migrated incrementally so callers no longer
// need to bind directly to the lemna client for keyword orchestration.
type Engine struct {
	searcher Searcher
}

type Searcher interface {
	SearchKeywordRowsEngine(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error)
	SearchKeywordRowsWideEngine(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error)
	SearchKeywordRowsBroadEngine(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error)
}

func New(searcher Searcher) *Engine {
	return &Engine{searcher: searcher}
}

func (e *Engine) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	if e == nil || e.searcher == nil {
		return nil, nil
	}
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "search lemna keyword engine rows",
	}, func(runCtx context.Context) ([]model.KeywordResultRow, error) {
		return e.searcher.SearchKeywordRowsEngine(runCtx, species, keyword)
	})
}

func (e *Engine) SearchKeywordRowsWide(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	if e == nil || e.searcher == nil {
		return nil, nil
	}
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "search lemna keyword engine rows wide",
	}, func(runCtx context.Context) ([]model.KeywordResultRow, error) {
		return e.searcher.SearchKeywordRowsWideEngine(runCtx, species, keyword)
	})
}

func (e *Engine) SearchKeywordRowsBroad(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	if e == nil || e.searcher == nil {
		return nil, nil
	}
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "search lemna keyword engine rows broad",
	}, func(runCtx context.Context) ([]model.KeywordResultRow, error) {
		return e.searcher.SearchKeywordRowsBroadEngine(runCtx, species, keyword)
	})
}

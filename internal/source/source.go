// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

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
	FetchProteinSequence(ctx context.Context, targetID int, sequenceID string) (model.ProteinSequenceData, error)
	FetchGeneQuerySequence(ctx context.Context, species model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, error)
}

type UniProtResolver interface {
	FetchUniProtAccessions(ctx context.Context, targetID int, proteinID string) ([]string, error)
}

type WideKeywordSearcher interface {
	SearchKeywordRowsWide(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error)
}

type BroadKeywordSearcher interface {
	SearchKeywordRowsBroad(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error)
}

type FamilyCandidateFetcher interface {
	FetchFamilyCandidates(ctx context.Context, species model.SpeciesCandidate) ([]model.SpeciesCandidate, error)
}

type FamilyCandidateFilter interface {
	FilterFamilyCandidates(candidates []model.SpeciesCandidate, keyword string) []model.SpeciesCandidate
}

type FamilyKeywordSearcher interface {
	SearchFamilyKeywordRows(ctx context.Context, species model.SpeciesCandidate, family string) ([]model.KeywordResultRow, error)
}

type ProteinReportResolver interface {
	FetchProteinQuerySequence(ctx context.Context, species model.SpeciesCandidate, proteinID string) (*model.QuerySequenceSource, error)
}

type QueryResolver interface {
	ResolveQuerySequence(ctx context.Context, species model.SpeciesCandidate, input string) (*model.QuerySequenceSource, bool, error)
}

type TAIRLabelNameResolver interface {
	ResolveTAIRKeywordRowLabelCandidates(ctx context.Context, row model.KeywordResultRow) ([]string, string)
	ResolveTAIRFamilyCandidateLabelCandidates(ctx context.Context, version model.SpeciesCandidate, candidate model.SpeciesCandidate) ([]string, string)
}

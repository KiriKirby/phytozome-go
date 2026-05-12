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
	FetchProteinSequence(ctx context.Context, targetID int, sequenceID string) (string, error)
	FetchGeneQuerySequence(ctx context.Context, species model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, error)
}

type UniProtResolver interface {
	FetchUniProtAccessions(ctx context.Context, targetID int, proteinID string) ([]string, error)
}

type ProteinReportResolver interface {
	FetchProteinQuerySequence(ctx context.Context, species model.SpeciesCandidate, proteinID string) (*model.QuerySequenceSource, error)
}

type QueryResolver interface {
	ResolveQuerySequence(ctx context.Context, species model.SpeciesCandidate, input string) (*model.QuerySequenceSource, bool, error)
}

package export

import (
	"context"

	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
)

func WriteBlastResultsExcelWithMetadataManaged(ctx context.Context, path string, rows []model.BlastResultRow, metadata *model.ExportMetadata, options *BlastExcelExportOptions) error {
	return phygoboost.RunDisk(ctx, func(context.Context) error {
		return WriteBlastResultsExcelWithMetadata(path, rows, metadata, options)
	})
}

func WriteKeywordResultsExcelManaged(ctx context.Context, path string, rows []model.KeywordResultRow) error {
	return phygoboost.RunDisk(ctx, func(context.Context) error {
		return WriteKeywordResultsExcel(path, rows)
	})
}

func WriteProteinSequencesTextManaged(ctx context.Context, path string, records []model.ProteinSequenceRecord) error {
	return phygoboost.RunDisk(ctx, func(context.Context) error {
		return WriteProteinSequencesText(path, records)
	})
}

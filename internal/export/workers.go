package export

import (
	"context"
	"fmt"

	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/goccy/go-json"
)

const (
	WriteBlastExcelWorker   = "export.write_blast_excel"
	WriteKeywordExcelWorker = "export.write_keyword_excel"
	WriteProteinTextWorker  = "export.write_protein_text"
)

type writeBlastExcelInput struct {
	Path     string                   `json:"path"`
	Rows     []model.BlastResultRow   `json:"rows"`
	Metadata *model.ExportMetadata    `json:"metadata,omitempty"`
	Options  *BlastExcelExportOptions `json:"options,omitempty"`
}

type writeKeywordExcelInput struct {
	Path string                   `json:"path"`
	Rows []model.KeywordResultRow `json:"rows"`
}

type writeProteinTextInput struct {
	Path    string                        `json:"path"`
	Records []model.ProteinSequenceRecord `json:"records"`
}

func init() {
	phygoboost.Register(WriteBlastExcelWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input writeBlastExcelInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode BLAST Excel worker input: %w", err)
		}
		return nil, phygoboost.RunDisk(ctx, func(ctx context.Context) error {
			return WriteBlastResultsExcelWithMetadata(input.Path, input.Rows, input.Metadata, input.Options)
		})
	})
	phygoboost.Register(WriteKeywordExcelWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input writeKeywordExcelInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode keyword Excel worker input: %w", err)
		}
		return nil, phygoboost.RunDisk(ctx, func(ctx context.Context) error {
			return WriteKeywordResultsExcel(input.Path, input.Rows)
		})
	})
	phygoboost.Register(WriteProteinTextWorker, func(ctx context.Context, payload []byte) ([]byte, error) {
		var input writeProteinTextInput
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("decode protein text worker input: %w", err)
		}
		return nil, phygoboost.RunDisk(ctx, func(ctx context.Context) error {
			return WriteProteinSequencesText(input.Path, input.Records)
		})
	})
}

func WriteBlastResultsExcelWithMetadataProcess(ctx context.Context, path string, rows []model.BlastResultRow, metadata *model.ExportMetadata, options *BlastExcelExportOptions) error {
	if phygoboost.InWorker() {
		return phygoboost.RunDisk(ctx, func(ctx context.Context) error {
			return WriteBlastResultsExcelWithMetadata(path, rows, metadata, options)
		})
	}
	return phygoboost.RunTaskJSON(ctx, phygoboost.TaskSpec{Level: phygoboost.ExecHeavy, LocalSlots: 1, Description: "write blast excel"}, WriteBlastExcelWorker, writeBlastExcelInput{
		Path:     path,
		Rows:     rows,
		Metadata: metadata,
		Options:  options,
	}, (*struct{})(nil))
}

func WriteKeywordResultsExcelProcess(ctx context.Context, path string, rows []model.KeywordResultRow) error {
	if phygoboost.InWorker() {
		return phygoboost.RunDisk(ctx, func(ctx context.Context) error {
			return WriteKeywordResultsExcel(path, rows)
		})
	}
	return phygoboost.RunTaskJSON(ctx, phygoboost.TaskSpec{Level: phygoboost.ExecHeavy, LocalSlots: 1, Description: "write keyword excel"}, WriteKeywordExcelWorker, writeKeywordExcelInput{Path: path, Rows: rows}, (*struct{})(nil))
}

func WriteProteinSequencesTextProcess(ctx context.Context, path string, records []model.ProteinSequenceRecord) error {
	if phygoboost.InWorker() {
		return phygoboost.RunDisk(ctx, func(ctx context.Context) error {
			return WriteProteinSequencesText(path, records)
		})
	}
	return phygoboost.RunTaskJSON(ctx, phygoboost.TaskSpec{Level: phygoboost.ExecHeavy, LocalSlots: 1, Description: "write protein text"}, WriteProteinTextWorker, writeProteinTextInput{Path: path, Records: records}, (*struct{})(nil))
}


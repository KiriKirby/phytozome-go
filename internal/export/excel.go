package export

import (
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/wangsychn/phytozome-batch-cli/internal/model"
)

var blastResultHeaders = []string{
	"row",
	"hit_number",
	"hsp_number",
	"protein",
	"species",
	"e_value",
	"percent_identity",
	"align_len",
	"strands",
	"query_id",
	"query_from",
	"query_to",
	"target_from",
	"target_to",
	"bitscore",
	"identical",
	"positives",
	"gaps",
	"query_length",
	"target_length",
	"jbrowse_name",
	"target_id",
	"sequence_id",
	"transcript_id",
	"defline",
	"gene_report_url",
}

func WriteBlastResultsExcel(path string, rows []model.BlastResultRow) error {
	file := excelize.NewFile()
	defer func() {
		_ = file.Close()
	}()

	const sheet = "BLAST Results"
	file.SetSheetName(file.GetSheetName(0), sheet)

	for col, header := range blastResultHeaders {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		if err != nil {
			return fmt.Errorf("build header cell: %w", err)
		}
		if err := file.SetCellValue(sheet, cell, header); err != nil {
			return fmt.Errorf("write header %q: %w", header, err)
		}
	}

	for idx, row := range rows {
		values := []any{
			idx + 1,
			row.HitNumber,
			row.HSPNumber,
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
			row.JBrowseName,
			row.TargetID,
			row.SequenceID,
			row.TranscriptID,
			row.Defline,
			row.GeneReportURL,
		}

		for col, value := range values {
			cell, err := excelize.CoordinatesToCellName(col+1, idx+2)
			if err != nil {
				return fmt.Errorf("build data cell: %w", err)
			}
			if err := file.SetCellValue(sheet, cell, value); err != nil {
				return fmt.Errorf("write row %d col %d: %w", idx+2, col+1, err)
			}
		}
	}

	if err := file.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return fmt.Errorf("freeze header row: %w", err)
	}

	if err := file.SaveAs(path); err != nil {
		return fmt.Errorf("save excel file: %w", err)
	}

	return nil
}

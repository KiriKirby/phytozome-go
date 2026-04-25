package export

import (
	"fmt"
	"sort"

	"github.com/xuri/excelize/v2"

	"github.com/KiriKirby/phytozome-go/internal/model"
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

var keywordResultHeaders = []string{
	"row",
	"search_term",
	"protein_identification",
	"transcript",
	"gene_identifier",
	"genome",
	"location",
	"alias",
	"uniprot",
	"description",
	"comments",
	"auto_define",
	"gene_report_url",
}

func WriteBlastResultsExcel(path string, rows []model.BlastResultRow) error {
	return WriteBlastResultsExcelWithMetadata(path, rows, nil)
}

func WriteBlastResultsExcelWithMetadata(path string, rows []model.BlastResultRow, metadata *model.ExportMetadata) error {
	file := excelize.NewFile()
	defer func() {
		_ = file.Close()
	}()

	const sheet = "BLAST Results"
	file.SetSheetName(file.GetSheetName(0), sheet)

	headerRow := 1
	dataStartRow := 2

	if metadata != nil && (metadata.GeneName != "" || metadata.GeneID != "" || metadata.GeneReportURL != "") {
		metaValues := []any{
			"gene_name", metadata.GeneName,
			"gene_id", metadata.GeneID,
			"gene_report_url", metadata.GeneReportURL,
		}
		for col, value := range metaValues {
			cell, err := excelize.CoordinatesToCellName(col+1, 1)
			if err != nil {
				return fmt.Errorf("build metadata cell: %w", err)
			}
			if err := file.SetCellValue(sheet, cell, value); err != nil {
				return fmt.Errorf("write metadata col %d: %w", col+1, err)
			}
		}
		headerRow = 2
		dataStartRow = 3
	}

	for col, header := range blastResultHeaders {
		cell, err := excelize.CoordinatesToCellName(col+1, headerRow)
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
		cell, err := excelize.CoordinatesToCellName(1, idx+dataStartRow)
		if err != nil {
			return fmt.Errorf("build data row start cell: %w", err)
		}
		if err := file.SetSheetRow(sheet, cell, &values); err != nil {
			return fmt.Errorf("write data row %d: %w", idx+dataStartRow, err)
		}
	}

	if err := file.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      headerRow,
		TopLeftCell: fmt.Sprintf("A%d", dataStartRow),
		ActivePane:  "bottomLeft",
	}); err != nil {
		return fmt.Errorf("freeze header row: %w", err)
	}

	if err := file.SaveAs(path); err != nil {
		return fmt.Errorf("save excel file: %w", err)
	}

	return nil
}

func WriteKeywordResultsExcel(path string, rows []model.KeywordResultRow) error {
	file := excelize.NewFile()
	defer func() {
		_ = file.Close()
	}()

	const sheet = "Keyword Results"
	file.SetSheetName(file.GetSheetName(0), sheet)

	headers := append([]string(nil), keywordResultHeaders...)
	headers = append(headers, keywordExtraHeaders(rows)...)

	for col, header := range headers {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		if err != nil {
			return fmt.Errorf("build keyword header cell: %w", err)
		}
		if err := file.SetCellValue(sheet, cell, header); err != nil {
			return fmt.Errorf("write keyword header %q: %w", header, err)
		}
	}

	for idx, row := range rows {
		values := []any{
			idx + 1,
			row.SearchTerm,
			row.ProteinIdentification,
			row.TranscriptID,
			row.GeneIdentifier,
			row.Genome,
			row.Location,
			row.Aliases,
			row.UniProt,
			row.Description,
			row.Comments,
			row.AutoDefine,
			row.GeneReportURL,
		}
		for _, header := range headers[len(keywordResultHeaders):] {
			values = append(values, row.ExtraColumns[header])
		}
		cell, err := excelize.CoordinatesToCellName(1, idx+2)
		if err != nil {
			return fmt.Errorf("build keyword data row start cell: %w", err)
		}
		if err := file.SetSheetRow(sheet, cell, &values); err != nil {
			return fmt.Errorf("write keyword row %d: %w", idx+2, err)
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
		return fmt.Errorf("freeze keyword header row: %w", err)
	}

	if err := file.SaveAs(path); err != nil {
		return fmt.Errorf("save keyword excel file: %w", err)
	}

	return nil
}

func keywordExtraHeaders(rows []model.KeywordResultRow) []string {
	seen := make(map[string]struct{})
	for _, row := range rows {
		for key := range row.ExtraColumns {
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
	}
	headers := make([]string, 0, len(seen))
	for key := range seen {
		headers = append(headers, key)
	}
	sort.Strings(headers)
	return headers
}

// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package export

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/prompt"
)

const (
	excelColorDefault      = "000000"
	excelColorAction       = "00BFFF"
	excelColorMuted        = "FFA500"
	excelColorAccent       = "008000"
	excelColorSelectionOn  = "008000"
	excelColorSelectionOff = "FF0000"
)

type BlastExcelExportOptions struct {
	SelectedRows []bool
	FilterFlags  []bool
	RowNumbers   []int
}

func WriteBlastResultsExcel(path string, rows []model.BlastResultRow) error {
	return WriteBlastResultsExcelWithMetadata(path, rows, nil, nil)
}

func blastAlignQueryLengthPercent(row model.BlastResultRow) string {
	if row.AlignQueryLengthPercent != 0 {
		return fmt.Sprintf("%.2f", row.AlignQueryLengthPercent)
	}
	if row.QueryLength <= 0 {
		return ""
	}
	return fmt.Sprintf("%.2f", float64(row.AlignLength)/float64(row.QueryLength)*100)
}

func firstNonEmptyText(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func blastRowsHaveUniProtReference(rows []model.BlastResultRow) bool {
	for _, row := range rows {
		if row.UniProtReferenceEnabled {
			return true
		}
	}
	return false
}

func blastRowsHaveInterProReference(rows []model.BlastResultRow) bool {
	for _, row := range rows {
		if row.InterProReferenceEnabled {
			return true
		}
	}
	return false
}

func blastHeadersForRows(rows []model.BlastResultRow) []string {
	includeUniProt := blastRowsHaveUniProtReference(rows)
	includeInterPro := blastRowsHaveInterProReference(rows)
	headerIDs := prompt.BlastExportColumnIDs(sourceDatabaseForBlastRows(rows), includeUniProt, includeInterPro)
	headers := make([]string, 0, len(headerIDs))
	options := prompt.ColumnDisplayOptions{DatabaseDisplay: databaseDisplayNameForRows(rows)}
	for _, id := range headerIDs {
		headers = append(headers, prompt.ColumnExportHeader(id, options))
	}
	return headers
}

func blastHeaderPlanForRows(rows []model.BlastResultRow) ([]string, []string) {
	includeUniProt := blastRowsHaveUniProtReference(rows)
	includeInterPro := blastRowsHaveInterProReference(rows)
	headerIDs := prompt.BlastExportColumnIDs(sourceDatabaseForBlastRows(rows), includeUniProt, includeInterPro)
	headers := make([]string, 0, len(headerIDs))
	options := prompt.ColumnDisplayOptions{DatabaseDisplay: databaseDisplayNameForRows(rows)}
	for _, id := range headerIDs {
		headers = append(headers, prompt.ColumnExportHeader(id, options))
	}
	return headerIDs, headers
}

func blastTargetUniProtCanonicalLengthHeader(rows []model.BlastResultRow) string {
	return databaseDisplayNameForRows(rows) + " target_length / UniProt canonical length (%)"
}

func databaseDisplayNameForRows(rows []model.BlastResultRow) string {
	for _, row := range rows {
		switch strings.ToLower(strings.TrimSpace(row.SourceDatabase)) {
		case "lemna":
			return "lemna"
		case "phytozome":
			return "Phytozome"
		case "":
			continue
		default:
			return strings.TrimSpace(row.SourceDatabase)
		}
	}
	return "target"
}

func blastRowValues(row model.BlastResultRow, index int, includeUniProt bool, includeInterPro bool) []any {
	rowNumber := index + 1
	headerIDs := prompt.BlastExportColumnIDs(strings.TrimSpace(row.SourceDatabase), includeUniProt, includeInterPro)
	values := make([]any, 0, len(headerIDs))
	for _, id := range headerIDs {
		values = append(values, blastExportValue(id, row, rowNumber))
	}
	return values
}

func blastExportValue(id string, row model.BlastResultRow, rowNumber int) any {
	switch id {
	case "row":
		return rowNumber
	case "source_database":
		return row.SourceDatabase
	case "blast_program":
		return row.BlastProgram
	case "label_name":
		return row.LabelName
	case "labelname_type":
		return row.LabelNameType
	case "phgo_alias":
		return row.PhgoAliases
	case "protein":
		return row.Protein
	case "blast_labelname":
		return row.BlastLabelName
	case "blast_geneid":
		return row.BlastGeneID
	case "subject_id":
		return firstNonEmptyText(row.SubjectID, row.Protein)
	case "percent_identity":
		return row.PercentIdentity
	case "align_query_length_percent":
		return blastAlignQueryLengthPercent(row)
	case "interpro_conserved_region_status":
		return blankIfNoInterPro(row, row.InterProConservedRegionStatus)
	case "target_length":
		return row.TargetLength
	case "target_uniprot_canonical_length_percent":
		return blankIfNoUniProt(row, row.TargetUniProtCanonicalLengthPercent)
	case "align_len":
		return row.AlignLength
	case "query_length":
		return row.QueryLength
	case "uniprot_canonical_length":
		return blankIfNoUniProt(row, row.UniProtCanonicalLength)
	case "species":
		return row.Species
	case "hit_number":
		return row.HitNumber
	case "hsp_number":
		return row.HSPNumber
	case "e_value":
		return row.EValue
	case "strands":
		return row.Strands
	case "query_id":
		return row.QueryID
	case "query_from":
		return row.QueryFrom
	case "query_to":
		return row.QueryTo
	case "target_from":
		return row.TargetFrom
	case "target_to":
		return row.TargetTo
	case "bitscore":
		return row.Bitscore
	case "mismatches":
		return row.Mismatches
	case "gap_openings":
		return row.GapOpenings
	case "identical":
		return row.Identical
	case "positives":
		return row.Positives
	case "gaps":
		return row.Gaps
	case "jbrowse_name":
		return row.JBrowseName
	case "target_id":
		return row.TargetID
	case "sequence_id":
		return row.SequenceID
	case "transcript_id":
		return row.TranscriptID
	case "defline":
		return row.Defline
	case "gene_report_url":
		return row.GeneReportURL
	case "uniprot_accession":
		return blankIfNoUniProt(row, row.UniProtAccession)
	case "uniprot_entry_name":
		return blankIfNoUniProt(row, row.UniProtEntryName)
	case "uniprot_reviewed":
		return blankIfNoUniProt(row, row.UniProtReviewed)
	case "uniprot_protein_name":
		return blankIfNoUniProt(row, row.UniProtProteinName)
	case "uniprot_gene_names":
		return blankIfNoUniProt(row, row.UniProtGeneNames)
	case "uniprot_organism":
		return blankIfNoUniProt(row, row.UniProtOrganism)
	case "uniprot_organism_id":
		return blankIfNoUniProt(row, row.UniProtOrganismID)
	case "uniprot_keywords":
		return blankIfNoUniProt(row, row.UniProtKeywords)
	case "uniprot_ec":
		return blankIfNoUniProt(row, row.UniProtEC)
	case "uniprot_go":
		return blankIfNoUniProt(row, row.UniProtGO)
	case "uniprot_go_ids":
		return blankIfNoUniProt(row, row.UniProtGOIDs)
	case "uniprot_function":
		return blankIfNoUniProt(row, row.UniProtFunction)
	case "uniprot_catalytic_activity":
		return blankIfNoUniProt(row, row.UniProtCatalyticActivity)
	case "uniprot_pathway":
		return blankIfNoUniProt(row, row.UniProtPathway)
	case "uniprot_subcellular_location":
		return blankIfNoUniProt(row, row.UniProtSubcellularLocation)
	case "uniprot_protein_existence":
		return blankIfNoUniProt(row, row.UniProtProteinExistence)
	case "uniprot_annotation_score":
		return blankIfNoUniProt(row, row.UniProtAnnotationScore)
	case "uniprot_fragment":
		return blankIfNoUniProt(row, row.UniProtFragment)
	case "uniprot_sequence_caution":
		return blankIfNoUniProt(row, row.UniProtSequenceCaution)
	case "uniprot_pfam":
		return blankIfNoUniProt(row, row.UniProtPfam)
	case "uniprot_interpro":
		return blankIfNoUniProt(row, row.UniProtInterPro)
	case "uniprot_domain":
		return blankIfNoUniProt(row, row.UniProtDomain)
	case "uniprot_region":
		return blankIfNoUniProt(row, row.UniProtRegion)
	case "uniprot_motif":
		return blankIfNoUniProt(row, row.UniProtMotif)
	case "uniprot_active_site":
		return blankIfNoUniProt(row, row.UniProtActiveSite)
	case "uniprot_binding_site":
		return blankIfNoUniProt(row, row.UniProtBindingSite)
	case "uniprot_alphafolddb":
		return blankIfNoUniProt(row, row.UniProtAlphaFoldDB)
	case "uniprot_pdb":
		return blankIfNoUniProt(row, row.UniProtPDB)
	case "interpro_entry_name":
		return blankIfNoInterPro(row, row.InterProEntryName)
	case "interpro_entry_type":
		return blankIfNoInterPro(row, row.InterProEntryType)
	case "interpro_coverage_percent":
		return blankIfNoInterPro(row, row.InterProCoveragePercent)
	case "interpro_match_regions":
		return blankIfNoInterPro(row, row.InterProMatchRegions)
	case "interpro_accessions":
		return blankIfNoInterPro(row, row.InterProAccessions)
	case "interpro_signature_accessions":
		return blankIfNoInterPro(row, row.InterProSignatureAccessions)
	case "interpro_pfam_accessions":
		return blankIfNoInterPro(row, row.InterProPfamAccessions)
	default:
		return ""
	}
}

func blankIfNoUniProt(row model.BlastResultRow, value string) string {
	if strings.TrimSpace(row.UniProtAccession) == "" {
		return ""
	}
	return value
}

func blankIfNoInterPro(row model.BlastResultRow, value string) string {
	if !row.InterProReferenceEnabled || (strings.TrimSpace(row.InterProAccessions) == "" && strings.TrimSpace(row.InterProConservedRegionStatus) == "") {
		return ""
	}
	return value
}

func WriteBlastResultsExcelWithMetadata(path string, rows []model.BlastResultRow, metadata *model.ExportMetadata, options *BlastExcelExportOptions) error {
	file := excelize.NewFile()
	defer func() {
		_ = file.Close()
	}()

	const sheet = "BLAST Results"
	file.SetSheetName(file.GetSheetName(0), sheet)
	if err := writeBlastMetadataSheet(file, metadata); err != nil {
		return err
	}

	headerRow := 1
	dataStartRow := 2

	headerIDs, headers := blastHeaderPlanForRows(rows)
	styleCache := map[string]int{}
	stream, err := file.NewStreamWriter(sheet)
	if err != nil {
		return fmt.Errorf("create blast stream writer: %w", err)
	}
	if err := stream.SetPanes(&excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      headerRow,
		TopLeftCell: fmt.Sprintf("A%d", dataStartRow),
		ActivePane:  "bottomLeft",
	}); err != nil {
		return fmt.Errorf("freeze header row: %w", err)
	}

	headerCells := make([]interface{}, 0, len(headers))
	for _, header := range headers {
		headerCells = append(headerCells, excelize.Cell{
			StyleID: blastExcelFontStyle(file, styleCache, blastExcelHeaderColor(header), true),
			Value:   header,
		})
	}
	if err := stream.SetRow("A1", headerCells); err != nil {
		return fmt.Errorf("write blast header row: %w", err)
	}

	for idx, row := range rows {
		rowCells := blastRowCellsForHeaderIDs(file, styleCache, row, idx, headerIDs, options)
		cell, err := excelize.CoordinatesToCellName(1, idx+dataStartRow)
		if err != nil {
			return fmt.Errorf("build data row start cell: %w", err)
		}
		if err := stream.SetRow(cell, rowCells); err != nil {
			return fmt.Errorf("write data row %d: %w", idx+dataStartRow, err)
		}
	}
	if err := stream.Flush(); err != nil {
		return fmt.Errorf("flush blast excel stream: %w", err)
	}

	if err := file.SaveAs(path); err != nil {
		return fmt.Errorf("save excel file: %w", err)
	}

	return nil
}

func blastRowValuesForHeaderIDs(row model.BlastResultRow, index int, headerIDs []string) []any {
	rowNumber := index + 1
	values := make([]any, 0, len(headerIDs))
	for _, id := range headerIDs {
		values = append(values, blastExportValue(id, row, rowNumber))
	}
	return values
}

func blastRowCellsForHeaderIDs(file *excelize.File, styleCache map[string]int, row model.BlastResultRow, dataIndex int, headerIDs []string, options *BlastExcelExportOptions) []interface{} {
	rowNumber := dataIndex + 1
	if options != nil && dataIndex < len(options.RowNumbers) && options.RowNumbers[dataIndex] > 0 {
		rowNumber = options.RowNumbers[dataIndex]
	}
	originalRowIndex := originalRowIndexForExcel(options, dataIndex)
	cells := make([]interface{}, 0, len(headerIDs))
	for _, id := range headerIDs {
		value := blastExportValue(id, row, rowNumber)
		color := blastExcelCellColorByID(id, row, originalRowIndex, options)
		if color == "" || color == excelColorDefault {
			cells = append(cells, value)
			continue
		}
		cells = append(cells, excelize.Cell{
			StyleID: blastExcelFontStyle(file, styleCache, color, false),
			Value:   value,
		})
	}
	return cells
}

func writeBlastMetadataSheet(file *excelize.File, metadata *model.ExportMetadata) error {
	if file == nil || metadata == nil {
		return nil
	}
	queries := exportMetadataQueries(metadata)
	if len(queries) == 0 && strings.TrimSpace(metadata.GeneName) == "" && strings.TrimSpace(metadata.GeneID) == "" && strings.TrimSpace(metadata.GeneReportURL) == "" {
		return nil
	}
	const sheet = "Query Metadata"
	if _, err := file.NewSheet(sheet); err != nil {
		return fmt.Errorf("create metadata sheet: %w", err)
	}
	stream, err := file.NewStreamWriter(sheet)
	if err != nil {
		return fmt.Errorf("create metadata stream writer: %w", err)
	}
	if len(queries) == 0 {
		values := []struct {
			key   string
			value string
		}{
			{key: "gene_name", value: strings.TrimSpace(metadata.GeneName)},
			{key: "gene_id", value: strings.TrimSpace(metadata.GeneID)},
			{key: "gene_report_url", value: strings.TrimSpace(metadata.GeneReportURL)},
		}
		if err := stream.SetRow("A1", []interface{}{"field", "value"}); err != nil {
			return fmt.Errorf("write metadata header row: %w", err)
		}
		for i, item := range values {
			cell, err := excelize.CoordinatesToCellName(1, i+2)
			if err != nil {
				return fmt.Errorf("build metadata row cell %d: %w", i+2, err)
			}
			if err := stream.SetRow(cell, []interface{}{item.key, item.value}); err != nil {
				return fmt.Errorf("write metadata row %d: %w", i+2, err)
			}
		}
		if err := stream.Flush(); err != nil {
			return fmt.Errorf("flush metadata stream: %w", err)
		}
		return nil
	}
	headers := []string{
		"query_index",
		"blast_labelname",
		"blast_geneid",
		"protein_id",
		"transcript_id",
		"source_database",
		"source_proteome_id",
		"source_jbrowse_name",
		"source_genome_label",
		"original_input_url",
		"normalized_url",
		"organism_short",
		"annotation",
		"sequence_length",
	}
	headerCells := make([]interface{}, 0, len(headers))
	for _, header := range headers {
		headerCells = append(headerCells, header)
	}
	if err := stream.SetRow("A1", headerCells); err != nil {
		return fmt.Errorf("write metadata header row: %w", err)
	}
	for rowIndex, query := range queries {
		values := []any{
			query.Index,
			query.LabelName,
			query.GeneID,
			query.ProteinID,
			query.TranscriptID,
			query.SourceDatabase,
			query.SourceProteomeID,
			query.SourceJBrowseName,
			query.SourceGenomeLabel,
			query.OriginalInputURL,
			query.NormalizedURL,
			query.OrganismShort,
			query.Annotation,
			query.SequenceLength,
		}
		rowCells := make([]interface{}, 0, len(values))
		for _, value := range values {
			rowCells = append(rowCells, value)
		}
		cell, err := excelize.CoordinatesToCellName(1, rowIndex+2)
		if err != nil {
			return fmt.Errorf("build metadata row cell %d: %w", rowIndex+2, err)
		}
		if err := stream.SetRow(cell, rowCells); err != nil {
			return fmt.Errorf("write metadata query row %d: %w", rowIndex+1, err)
		}
	}
	if err := stream.Flush(); err != nil {
		return fmt.Errorf("flush metadata stream: %w", err)
	}
	return nil
}

func exportMetadataQueries(metadata *model.ExportMetadata) []model.ExportQueryMetadata {
	if metadata == nil {
		return nil
	}
	if len(metadata.Queries) > 0 {
		out := append([]model.ExportQueryMetadata(nil), metadata.Queries...)
		for i := range out {
			if out[i].Index == 0 {
				out[i].Index = i + 1
			}
		}
		return out
	}
	if strings.TrimSpace(metadata.GeneName) == "" && strings.TrimSpace(metadata.GeneID) == "" && strings.TrimSpace(metadata.GeneReportURL) == "" {
		return nil
	}
	return []model.ExportQueryMetadata{{
		Index:            1,
		LabelName:        strings.TrimSpace(metadata.GeneName),
		GeneID:           strings.TrimSpace(metadata.GeneID),
		OriginalInputURL: strings.TrimSpace(metadata.GeneReportURL),
	}}
}

func blastExcelCellColor(header string, row model.BlastResultRow, originalRowIndex int, options *BlastExcelExportOptions) string {
	return blastExcelCellColorByID(blastExcelColumnID(header), row, originalRowIndex, options)
}

func blastExcelCellColorByID(id string, row model.BlastResultRow, originalRowIndex int, options *BlastExcelExportOptions) string {
	if id == "row" && originalRowIndex >= 0 && options != nil && originalRowIndex < len(options.FilterFlags) && options.FilterFlags[originalRowIndex] {
		return excelColorSelectionOff
	}
	switch id {
	case "uniprot_reviewed":
		switch strings.ToLower(strings.TrimSpace(row.UniProtReviewed)) {
		case "reviewed":
			return excelColorSelectionOn
		case "unreviewed":
			return excelColorMuted
		}
	case "interpro_conserved_region_status":
		switch strings.ToLower(strings.TrimSpace(row.InterProConservedRegionStatus)) {
		case "present":
			return excelColorSelectionOn
		case "partial":
			return excelColorMuted
		case "missing":
			return excelColorSelectionOff
		case "uncertain":
			return excelColorAction
		}
	}
	return excelColorDefault
}

func blastExcelHeaderColor(header string) string {
	switch blastExcelColumnReference(header) {
	case "source":
		return excelColorAction
	case "uniprot":
		return excelColorMuted
	case "interpro":
		return excelColorAccent
	default:
		return excelColorDefault
	}
}

func blastExcelColumnReference(header string) string {
	id := blastExcelColumnID(header)
	switch {
	case id == "source_database":
		return "source"
	case blastExcelColumnIsUniProtReference(id):
		return "uniprot"
	case blastExcelColumnIsInterProReference(id):
		return "interpro"
	default:
		return ""
	}
}

func blastExcelColumnIsUniProtReference(id string) bool {
	return strings.HasPrefix(id, "uniprot_") || id == "target_uniprot_canonical_length_percent"
}

func blastExcelColumnIsInterProReference(id string) bool {
	return strings.HasPrefix(id, "interpro_")
}

func blastExcelColumnID(header string) string {
	normalized := normalizeBlastExcelHeader(header)
	if id, ok := blastExcelHeaderIDs[normalized]; ok {
		return id
	}
	if strings.Contains(normalized, "target_length / uniprot canonical length") {
		return "target_uniprot_canonical_length_percent"
	}
	return normalized
}

func normalizeBlastExcelHeader(header string) string {
	header = strings.ToLower(strings.TrimSpace(header))
	header = strings.ReplaceAll(header, "\r\n", " ")
	header = strings.ReplaceAll(header, "\n", " ")
	header = strings.Join(strings.Fields(header), " ")
	return header
}

var blastExcelHeaderIDs = map[string]string{
	"row":                              "row",
	"source_database":                  "source_database",
	"blast_program":                    "blast_program",
	"label_name":                       "label_name",
	"labelname_type":                   "labelname_type",
	"protein":                          "protein",
	"geneid":                           "protein",
	"blast_labelname":                  "blast_labelname",
	"blast_geneid":                     "blast_geneid",
	"subject_id":                       "subject_id",
	"identity (%)":                     "percent_identity",
	"align_len / query_length (%)":     "align_query_length_percent",
	"interpro conserved region status": "interpro_conserved_region_status",
	"target_length":                    "target_length",
	"align_len":                        "align_len",
	"query_length":                     "query_length",
	"species":                          "species",
	"hit_number":                       "hit_number",
	"hsp_number":                       "hsp_number",
	"e_value":                          "e_value",
	"strands":                          "strands",
	"query_id":                         "query_id",
	"query_from":                       "query_from",
	"query_to":                         "query_to",
	"target_from":                      "target_from",
	"target_to":                        "target_to",
	"bitscore":                         "bitscore",
	"mismatches":                       "mismatches",
	"gap_openings":                     "gap_openings",
	"identical":                        "identical",
	"positives":                        "positives",
	"gaps":                             "gaps",
	"jbrowse_name":                     "jbrowse_name",
	"target_id":                        "target_id",
	"sequence_id":                      "sequence_id",
	"transcript_id":                    "transcript_id",
	"defline":                          "defline",
	"gene_report_url":                  "gene_report_url",
	"uniprot canonical length":         "uniprot_canonical_length",
	"uniprot accession":                "uniprot_accession",
	"uniprot entry name":               "uniprot_entry_name",
	"uniprot reviewed":                 "uniprot_reviewed",
	"uniprot protein name":             "uniprot_protein_name",
	"uniprot gene names":               "uniprot_gene_names",
	"uniprot organism":                 "uniprot_organism",
	"uniprot organism id":              "uniprot_organism_id",
	"uniprot keywords":                 "uniprot_keywords",
	"uniprot ec":                       "uniprot_ec",
	"uniprot go":                       "uniprot_go",
	"uniprot go ids":                   "uniprot_go_ids",
	"uniprot function":                 "uniprot_function",
	"uniprot catalytic activity":       "uniprot_catalytic_activity",
	"uniprot pathway":                  "uniprot_pathway",
	"uniprot subcellular location":     "uniprot_subcellular_location",
	"uniprot protein existence":        "uniprot_protein_existence",
	"uniprot annotation score":         "uniprot_annotation_score",
	"uniprot fragment":                 "uniprot_fragment",
	"uniprot sequence caution":         "uniprot_sequence_caution",
	"uniprot pfam":                     "uniprot_pfam",
	"uniprot interpro":                 "uniprot_interpro",
	"uniprot domain":                   "uniprot_domain",
	"uniprot region":                   "uniprot_region",
	"uniprot motif":                    "uniprot_motif",
	"uniprot active site":              "uniprot_active_site",
	"uniprot binding site":             "uniprot_binding_site",
	"uniprot alphafolddb":              "uniprot_alphafolddb",
	"uniprot pdb":                      "uniprot_pdb",
	"interpro entry name":              "interpro_entry_name",
	"interpro entry type":              "interpro_entry_type",
	"interpro coverage (%)":            "interpro_coverage_percent",
	"interpro match regions":           "interpro_match_regions",
	"interpro accessions":              "interpro_accessions",
	"interpro signature accessions":    "interpro_signature_accessions",
	"interpro pfam accessions":         "interpro_pfam_accessions",
}

func blastExcelFontStyle(file *excelize.File, styleCache map[string]int, color string, bold bool) int {
	if color == "" {
		color = excelColorDefault
	}
	key := fmt.Sprintf("font:%s:bold:%t", color, bold)
	if styleID, ok := styleCache[key]; ok {
		return styleID
	}
	styleID, err := file.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: bold, Color: color}})
	if err != nil {
		return 0
	}
	styleCache[key] = styleID
	return styleID
}

func originalRowIndexForExcel(options *BlastExcelExportOptions, rowIndex int) int {
	if options == nil || rowIndex < 0 {
		return -1
	}
	if len(options.RowNumbers) > 0 && rowIndex < len(options.RowNumbers) {
		if options.RowNumbers[rowIndex] <= 0 {
			return -1
		}
		return options.RowNumbers[rowIndex] - 1
	}
	return rowIndex
}

func WriteKeywordResultsExcel(path string, rows []model.KeywordResultRow) error {
	file := excelize.NewFile()
	defer func() {
		_ = file.Close()
	}()

	const sheet = "Keyword Results"
	file.SetSheetName(file.GetSheetName(0), sheet)

	includeProteinID := keywordRowsHaveProteinID(rows)
	extraHeaders := keywordExtraHeaders(rows)
	headerIDs := prompt.KeywordExportColumnIDs(sourceDatabaseForKeywordRows(rows), includeProteinID, extraHeaders)
	headers := make([]string, 0, len(headerIDs))
	for _, id := range headerIDs {
		headers = append(headers, prompt.ColumnExportHeader(id, prompt.ColumnDisplayOptions{}))
	}

	stream, err := file.NewStreamWriter(sheet)
	if err != nil {
		return fmt.Errorf("create keyword stream writer: %w", err)
	}
	if err := stream.SetPanes(&excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return fmt.Errorf("freeze keyword header row: %w", err)
	}
	headerCells := make([]interface{}, 0, len(headers))
	for _, header := range headers {
		headerCells = append(headerCells, header)
	}
	if err := stream.SetRow("A1", headerCells); err != nil {
		return fmt.Errorf("write keyword header row: %w", err)
	}

	for idx, row := range rows {
		values := make([]interface{}, 0, len(headerIDs))
		for _, id := range headerIDs {
			values = append(values, keywordExportValue(id, row, idx+1))
		}
		cell, err := excelize.CoordinatesToCellName(1, idx+2)
		if err != nil {
			return fmt.Errorf("build keyword data row start cell: %w", err)
		}
		if err := stream.SetRow(cell, values); err != nil {
			return fmt.Errorf("write keyword row %d: %w", idx+2, err)
		}
	}
	if err := stream.Flush(); err != nil {
		return fmt.Errorf("flush keyword excel stream: %w", err)
	}

	if err := file.SaveAs(path); err != nil {
		return fmt.Errorf("save keyword excel file: %w", err)
	}

	return nil
}

func keywordExportValue(id string, row model.KeywordResultRow, rowNumber int) any {
	switch id {
	case "row":
		return rowNumber
	case "search_term":
		return row.SearchTerm
	case "search_type":
		return row.SearchType
	case "label_name":
		return keywordLabelName(row)
	case "labelname_type":
		return strings.TrimSpace(row.LabelNameType)
	case "phgo_alias":
		return keywordPhgoAliases(row)
	case "protein_id":
		return row.ProteinID
	case "transcript":
		return row.TranscriptID
	case "gene_identifier":
		return row.GeneIdentifier
	case "genome":
		return row.Genome
	case "location":
		return row.Location
	case "alias":
		return row.Aliases
	case "symbols":
		return row.Symbols
	case "synonyms":
		return row.Synonyms
	case "uniprot":
		return row.UniProt
	case "description":
		return row.Description
	case "comments":
		return row.Comments
	case "auto_define":
		return row.AutoDefine
	case "gene_report_url":
		return row.GeneReportURL
	default:
		if row.ExtraColumns == nil {
			return ""
		}
		return row.ExtraColumns[id]
	}
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

func keywordLabelName(row model.KeywordResultRow) string {
	return row.LabelName
}

func keywordPhgoAliases(row model.KeywordResultRow) string {
	return row.PhgoAliases
}

func keywordRowsHaveProteinID(rows []model.KeywordResultRow) bool {
	for _, row := range rows {
		if row.ProteinID != "" {
			return true
		}
	}
	return false
}

func sourceDatabaseForBlastRows(rows []model.BlastResultRow) string {
	for _, row := range rows {
		if value := strings.TrimSpace(row.SourceDatabase); value != "" {
			return value
		}
	}
	return "phytozome"
}

func sourceDatabaseForKeywordRows(rows []model.KeywordResultRow) string {
	for _, row := range rows {
		if value := strings.TrimSpace(row.SourceDatabase); value != "" {
			return value
		}
	}
	return "phytozome"
}

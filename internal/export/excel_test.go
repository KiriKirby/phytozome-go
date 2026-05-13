// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package export

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/prompt"
	"github.com/xuri/excelize/v2"
)

func TestBlastExportKeepsReservedUniProtLengthColumnsInOriginalPositions(t *testing.T) {
	rows := []model.BlastResultRow{{
		SourceDatabase:                      "phytozome",
		UniProtReferenceEnabled:             true,
		TargetUniProtCanonicalLengthPercent: "101.20",
		UniProtCanonicalLength:              "532",
		UniProtAccession:                    "Q9S7C9",
		UniProtKeywords:                     "Lignin biosynthesis",
	}}

	headers := blastHeadersForRows(rows)
	targetIdx := indexOfAny(headers, "target_length")
	ratioIdx := indexOfAny(headers, "Phytozome target_length / UniProt canonical length (%)")
	alignIdx := indexOfAny(headers, "align_len")
	queryIdx := indexOfAny(headers, "query_length")
	canonicalIdx := indexOfAny(headers, "UniProt canonical length")
	speciesIdx := indexOfAny(headers, "species")
	accessionIdx := indexOfAny(headers, "UniProt accession")
	keywordsIdx := indexOfAny(headers, "UniProt keywords")

	if !(targetIdx >= 0 && ratioIdx == targetIdx+1 && alignIdx == ratioIdx+1) {
		t.Fatalf("comparison header should remain beside target_length: %v", headers)
	}
	if !(queryIdx >= 0 && canonicalIdx == queryIdx+1 && speciesIdx == canonicalIdx+1) {
		t.Fatalf("canonical length header should remain after query_length: %v", headers)
	}
	if accessionIdx <= speciesIdx || keywordsIdx <= accessionIdx {
		t.Fatalf("remaining UniProt headers should be appended after original headers: %v", headers)
	}
}

func TestKeywordExcelIncludesSearchTypeForPhytozome(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keyword.xlsx")
	rows := []model.KeywordResultRow{{
		SourceDatabase: "phytozome",
		SearchTerm:     "XP_015639656",
		SearchType:     "RefSeq XP protein",
		TranscriptID:   "LOC_Os05g25640.1",
	}}
	if err := WriteKeywordResultsExcel(path, rows); err != nil {
		t.Fatalf("WriteKeywordResultsExcel returned error: %v", err)
	}
	file, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open keyword excel: %v", err)
	}
	defer file.Close()
	headers, err := file.GetRows("Keyword Results")
	if err != nil {
		t.Fatalf("read keyword excel rows: %v", err)
	}
	if len(headers) < 2 {
		t.Fatalf("expected header and row, got %d rows", len(headers))
	}
	searchTypeIdx := indexOfAny(headers[0], "search_type")
	if searchTypeIdx < 0 {
		t.Fatalf("keyword export header missing search_type: %v", headers[0])
	}
	if got := headers[1][searchTypeIdx]; got != "RefSeq XP protein" {
		t.Fatalf("search_type value = %q, want RefSeq XP protein", got)
	}
}

func TestBlastExportLeavesUniProtCellsBlankWhenRowHasNoUniProtAccession(t *testing.T) {
	row := model.BlastResultRow{
		SourceDatabase:                      "lemna",
		UniProtReferenceEnabled:             true,
		TargetUniProtCanonicalLengthPercent: "98.50",
		UniProtCanonicalLength:              "480",
		UniProtProteinName:                  "Should stay blank",
	}

	values := blastRowValues(row, 0, true, false)
	headerIDs := prompt.BlastExportColumnIDs("lemna", true, false)
	for _, header := range []string{
		"target_uniprot_canonical_length_percent",
		"uniprot_canonical_length",
		"uniprot_protein_name",
	} {
		index := indexOfAny(headerIDs, header)
		if index < 0 {
			t.Fatalf("header %q not found in %v", header, headerIDs)
		}
		if values[index] != "" {
			t.Fatalf("value for %s = %#v, want blank for unmapped UniProt row", header, values[index])
		}
	}
}

func TestBlastExcelExportMirrorsFinalFilterColorsForSelectedAndRawRows(t *testing.T) {
	rows := []model.BlastResultRow{
		{SourceDatabase: "phytozome", Protein: "a"},
		{SourceDatabase: "phytozome", Protein: "b"},
		{SourceDatabase: "phytozome", Protein: "c"},
	}
	filterFlags := []bool{false, true, true}

	selectedPath := filepath.Join(t.TempDir(), "selected.xlsx")
	err := WriteBlastResultsExcelWithMetadata(selectedPath, []model.BlastResultRow{rows[0], rows[2]}, nil, &BlastExcelExportOptions{
		RowNumbers:  []int{1, 3},
		FilterFlags: filterFlags,
	})
	if err != nil {
		t.Fatalf("write selected excel: %v", err)
	}
	assertExcelFontColor(t, selectedPath, "BLAST Results", "A2", excelColorDefault)
	assertExcelFontColor(t, selectedPath, "BLAST Results", "A3", excelColorSelectionOff)

	rawPath := filepath.Join(t.TempDir(), "raw.xlsx")
	err = WriteBlastResultsExcelWithMetadata(rawPath, rows, nil, &BlastExcelExportOptions{FilterFlags: filterFlags})
	if err != nil {
		t.Fatalf("write raw excel: %v", err)
	}
	assertExcelFontColor(t, rawPath, "BLAST Results", "A2", excelColorDefault)
	assertExcelFontColor(t, rawPath, "BLAST Results", "A3", excelColorSelectionOff)
	assertExcelFontColor(t, rawPath, "BLAST Results", "A4", excelColorSelectionOff)
}

func TestBlastExcelExportMirrorsHeaderAndStatusColors(t *testing.T) {
	rows := []model.BlastResultRow{{
		SourceDatabase:                      "phytozome",
		Protein:                             "protein-1",
		UniProtReferenceEnabled:             true,
		UniProtAccession:                    "P12345",
		UniProtReviewed:                     "unreviewed",
		InterProReferenceEnabled:            true,
		InterProConservedRegionStatus:       "missing",
		InterProEntryName:                   "domain",
		InterProAccessions:                  "IPR000001",
		InterProSignatureAccessions:         "PF00001",
		TargetUniProtCanonicalLengthPercent: "95.00",
	}}
	path := filepath.Join(t.TempDir(), "colors.xlsx")
	if err := WriteBlastResultsExcelWithMetadata(path, rows, nil, nil); err != nil {
		t.Fatalf("write excel: %v", err)
	}

	assertExcelFontColor(t, path, "BLAST Results", "A1", excelColorDefault)
	assertExcelFontColor(t, path, "BLAST Results", cellForHeader(t, path, "source_database"), excelColorAction)
	assertExcelFontColor(t, path, "BLAST Results", cellForHeader(t, path, "UniProt reviewed"), excelColorMuted)
	assertExcelFontColor(t, path, "BLAST Results", cellForHeader(t, path, "InterPro conserved region status"), excelColorAccent)

	assertExcelFontColor(t, path, "BLAST Results", dataCellForHeader(t, path, "UniProt reviewed"), excelColorMuted)
	assertExcelFontColor(t, path, "BLAST Results", dataCellForHeader(t, path, "InterPro conserved region status"), excelColorSelectionOff)
}

func TestBlastExcelExportKeepsResultsSheetHeaderOnFirstRowWhenMetadataPresent(t *testing.T) {
	rows := []model.BlastResultRow{{
		SourceDatabase: "phytozome",
		Protein:        "AT2G30490.1",
	}}
	path := filepath.Join(t.TempDir(), "with_metadata.xlsx")
	err := WriteBlastResultsExcelWithMetadata(path, rows, &model.ExportMetadata{
		GeneName:      "C4H",
		GeneID:        "AT2G30490",
		GeneReportURL: "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
		Queries: []model.ExportQueryMetadata{{
			Index:            1,
			LabelName:        "C4H",
			GeneID:           "AT2G30490",
			OriginalInputURL: "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
		}},
	}, nil)
	if err != nil {
		t.Fatalf("write excel: %v", err)
	}

	file, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open excel %s: %v", path, err)
	}
	defer func() { _ = file.Close() }()

	if got, err := file.GetCellValue("BLAST Results", "A1"); err != nil || got != "row" {
		t.Fatalf("BLAST Results!A1 = %q, err=%v, want row", got, err)
	}
	if got, err := file.GetCellValue("BLAST Results", "A2"); err != nil || got != "1" {
		t.Fatalf("BLAST Results!A2 = %q, err=%v, want 1", got, err)
	}
	if got, err := file.GetCellValue("Query Metadata", "A1"); err != nil || got != "query_index" {
		t.Fatalf("Query Metadata!A1 = %q, err=%v, want query_index", got, err)
	}
	if got, err := file.GetCellValue("Query Metadata", "B2"); err != nil || got != "C4H" {
		t.Fatalf("Query Metadata!B2 = %q, err=%v, want C4H", got, err)
	}
}

func assertExcelFontColor(t *testing.T, path string, sheet string, cell string, want string) {
	t.Helper()
	file, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open excel %s: %v", path, err)
	}
	defer func() {
		_ = file.Close()
	}()
	styleID, err := file.GetCellStyle(sheet, cell)
	if err != nil {
		t.Fatalf("get style for %s: %v", cell, err)
	}
	style, err := file.GetStyle(styleID)
	if err != nil {
		t.Fatalf("get style %d for %s: %v", styleID, cell, err)
	}
	got := excelColorDefault
	if style != nil && style.Font != nil && style.Font.Color != "" {
		got = strings.ToUpper(style.Font.Color)
		if len(got) == 8 && strings.HasPrefix(got, "FF") {
			got = strings.TrimPrefix(got, "FF")
		}
	}
	if got != want {
		t.Fatalf("%s font color = %s, want %s", cell, got, want)
	}
}

func cellForHeader(t *testing.T, path string, header string) string {
	t.Helper()
	col := columnForHeader(t, path, header)
	cell, err := excelize.CoordinatesToCellName(col, 1)
	if err != nil {
		t.Fatalf("header cell for col %d: %v", col, err)
	}
	return cell
}

func dataCellForHeader(t *testing.T, path string, header string) string {
	t.Helper()
	col := columnForHeader(t, path, header)
	cell, err := excelize.CoordinatesToCellName(col, 2)
	if err != nil {
		t.Fatalf("data cell for col %d: %v", col, err)
	}
	return cell
}

func columnForHeader(t *testing.T, path string, header string) int {
	t.Helper()
	file, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open excel %s: %v", path, err)
	}
	defer func() {
		_ = file.Close()
	}()
	cols, err := file.GetCols("BLAST Results")
	if err != nil {
		t.Fatalf("read columns: %v", err)
	}
	for i, col := range cols {
		if len(col) > 0 && col[0] == header {
			return i + 1
		}
	}
	t.Fatalf("header %q not found", header)
	return 0
}

func indexOfAny(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}

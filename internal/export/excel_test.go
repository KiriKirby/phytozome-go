package export

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/model"
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

func TestBlastExportLeavesUniProtCellsBlankWhenRowHasNoUniProtAccession(t *testing.T) {
	row := model.BlastResultRow{
		SourceDatabase:                      "lemna",
		UniProtReferenceEnabled:             true,
		TargetUniProtCanonicalLengthPercent: "98.50",
		UniProtCanonicalLength:              "480",
		UniProtProteinName:                  "Should stay blank",
	}

	values := blastRowValues(row, 0, true, false)
	for _, index := range []int{10, 13, 36, 39} {
		if values[index] != "" {
			t.Fatalf("value at index %d = %#v, want blank for unmapped UniProt row", index, values[index])
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
	if got, err := file.GetCellValue("Query Metadata", "A1"); err != nil || got != "field" {
		t.Fatalf("Query Metadata!A1 = %q, err=%v, want field", got, err)
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

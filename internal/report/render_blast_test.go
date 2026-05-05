package report

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderBlastPDFCreatesPDF(t *testing.T) {
	data := SampleBlastReportData()
	data.Title = "BLAST Data Analysis Report 中文 日本語 한국어"
	data.UserSession.UserName = "测试用户"
	path := filepath.Join(t.TempDir(), ReportFileName(data.GeneratedAt))
	if err := RenderBlastPDF(path, data); err != nil {
		t.Fatalf("RenderBlastPDF() error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered PDF: %v", err)
	}
	if !bytes.HasPrefix(content, []byte("%PDF-")) {
		t.Fatalf("rendered file does not look like a PDF")
	}
	if !bytes.Contains(content, []byte("%%EOF")) {
		t.Fatalf("rendered PDF is missing EOF marker")
	}
	if len(content) < 20_000 {
		t.Fatalf("rendered PDF is unexpectedly small: %d bytes", len(content))
	}
}

func TestRenderBlastPDFAllowsLongWrappedContent(t *testing.T) {
	data := SampleBlastReportData()
	long := "very-long-blast-token-for-layout-validation-ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-abcdefghijklmnopqrstuvwxyz " +
		"https://phytozome-next.jgi.doe.gov/report/protein/Athaliana/AT3G53260.1?with=a-very-long-query-string-that-must-wrap-inside-the-cell"
	data.Title = "BLAST Data Analysis Report With A Very Long Wrapped Title 中文 日本語 한국어 " + long
	data.Blast.Inputs = append(data.Blast.Inputs, BlastInputTrace{
		Order:         99,
		RawPreview:    long,
		InputType:     "inline mixed line with a deliberately long description",
		ParserPath:    long,
		Source:        long,
		LabelName:     long,
		OriginalURL:   long,
		NormalizedURL: long,
		Outcome:       "resolved after wrapping validation",
		Notes:         long,
	})
	data.Blast.ExportSettings = append(data.Blast.ExportSettings, NameValue{
		Name:        "Long BLAST export setting " + long,
		Value:       long,
		Explanation: "This explanation intentionally contains a long URL/token and must expand the report container instead of being clipped or abbreviated. " + long,
	})
	if data.Blast.Filter != nil {
		data.Blast.Filter.Settings = append(data.Blast.Filter.Settings, BlastFilterSettingDetail{
			Group:   "Long wrapping validation",
			Name:    "Long filter parameter " + long,
			Value:   long,
			Default: long,
			Meaning: "Filter documentation must also wrap long parameter meanings without clipping. " + long,
			Effect:  "Filter documentation must also wrap long parameter effects without clipping. " + long,
		})
	}
	path := filepath.Join(t.TempDir(), "blast-long-wrap.pdf")
	if err := RenderBlastPDF(path, data); err != nil {
		t.Fatalf("RenderBlastPDF() long content error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered PDF: %v", err)
	}
	if !bytes.HasPrefix(content, []byte("%PDF-")) || !bytes.Contains(content, []byte("%%EOF")) {
		t.Fatalf("long-content PDF is malformed")
	}
}

func TestBlastFileRunLabelMatchesEachBatchFile(t *testing.T) {
	runs := []BlastRunReport{
		{Label: "4CL1\n4CL2", FamilyName: "4CL"},
		{Label: "PAL1\nPAL2", FamilyName: "PAL"},
		{Label: "HCALDH"},
	}
	cases := map[string]string{
		"4CL.xlsx":                        "4CL",
		"4CL_raw.xlsx":                    "4CL",
		"PAL.txt":                         "PAL",
		"HCALDH.xlsx":                     "HCALDH",
		"Monolignol_Biosynthesis_rpt.pdf": "current BLAST export",
	}
	for name, want := range cases {
		got := blastFileRunLabel(GeneratedFile{Name: name}, runs)
		if got != want {
			t.Fatalf("blastFileRunLabel(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestSequenceLengthDotPlotRendersWithQuerySummaries(t *testing.T) {
	data := SampleBlastReportData()
	data.Blast.Sequences.QuerySummaries = []SequenceQuerySummary{
		{QueryLabel: "PAL1", QueryKind: "query sequence record", RequestedCount: 1, WrittenCount: 1, AverageLength: 711, MinLength: 711, MaxLength: 711},
		{QueryLabel: "PAL family export", QueryKind: "selected hit peptide records", RequestedCount: 12, WrittenCount: 11, SkippedCount: 1, AverageLength: 680, MinLength: 521, MaxLength: 718},
	}
	path := filepath.Join(t.TempDir(), "blast-sequence-dot-plot.pdf")
	if err := RenderBlastPDF(path, data); err != nil {
		t.Fatalf("RenderBlastPDF() dot plot error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat rendered PDF: %v", err)
	}
	if info.Size() < 20_000 {
		t.Fatalf("expected rendered PDF with dot plot to be non-trivially sized, got %d bytes", info.Size())
	}
}

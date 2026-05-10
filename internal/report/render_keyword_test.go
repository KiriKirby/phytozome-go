package report

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderKeywordPDFCreatesPDF(t *testing.T) {
	data := SampleKeywordReportData()
	data.Title = "Keyword Data Analysis Report 中文 日本語 한국어"
	data.UserSession.UserName = "测试用户"
	path := filepath.Join(t.TempDir(), ReportFileName(data.GeneratedAt))
	if err := RenderKeywordPDF(path, data); err != nil {
		t.Fatalf("RenderKeywordPDF() error = %v", err)
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
	if len(content) < 10_000 {
		t.Fatalf("rendered PDF is unexpectedly small: %d bytes", len(content))
	}
}

func TestRenderKeywordPDFAllowsLongWrappedContent(t *testing.T) {
	data := SampleKeywordReportData()
	long := "very-long-unbroken-token-for-layout-validation-ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-abcdefghijklmnopqrstuvwxyz " +
		"https://phytozome-next.jgi.doe.gov/report/gene/Spirodela_polyrhiza/Sp7498v3_T000381?with=a-very-long-query-string-that-must-wrap-inside-the-cell"
	data.Title = "Keyword Data Analysis Report With A Very Long Wrapped Title 中文 日本語 한국어 " + long
	data.Files[0].Path = filepath.Join(t.TempDir(), long, "selected.xlsx")
	data.Keyword.ExportSettings = append(data.Keyword.ExportSettings, NameValue{
		Name:        "Long wrapping setting " + long,
		Value:       long,
		Explanation: "This explanation intentionally contains a long URL/token and must expand the card/table container instead of being clipped or abbreviated. " + long,
	})
	data.Keyword.Columns = append(data.Keyword.Columns, ColumnLineage{
		Column:           "long_column_" + long,
		Meaning:          "A deliberately long meaning that should wrap across many lines.",
		EnglishDetail:    long,
		ChineseDetail:    long,
		JapaneseDetail:   long,
		Source:           long,
		CollectionMethod: long,
		BlankMeaning:     long,
		UsedInStats:      long,
	})
	path := filepath.Join(t.TempDir(), "keyword-long-wrap.pdf")
	if err := RenderKeywordPDF(path, data); err != nil {
		t.Fatalf("RenderKeywordPDF() long content error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered PDF: %v", err)
	}
	if !bytes.HasPrefix(content, []byte("%PDF-")) || !bytes.Contains(content, []byte("%%EOF")) {
		t.Fatalf("long-content PDF is malformed")
	}
}

func TestRenderKeywordPDFProcessCreatesPDF(t *testing.T) {
	data := SampleKeywordReportData()
	path := filepath.Join(t.TempDir(), "keyword-process.pdf")
	if err := RenderKeywordPDFProcess(context.Background(), path, data); err != nil {
		t.Fatalf("RenderKeywordPDFProcess() error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered PDF: %v", err)
	}
	if !bytes.HasPrefix(content, []byte("%PDF-")) || !bytes.Contains(content, []byte("%%EOF")) {
		t.Fatalf("process-rendered keyword PDF is malformed")
	}
}

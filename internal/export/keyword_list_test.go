package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

func TestBuildKeywordListLines(t *testing.T) {
	rows := []model.KeywordResultRow{
		{SearchTerm: "abc", ProteinIdentification: "ID1", GeneReportURL: "https://example.com/1"},
		{SearchTerm: "abc", ProteinIdentification: "", GeneReportURL: "https://example.com/2"},
		{SearchTerm: "def", ProteinIdentification: "ID3", GeneReportURL: "https://example.com/3"},
	}

	lines := buildKeywordListLines(rows)
	want := []string{
		"ID1 (abc)",
		"~ (abc)",
		"ID3",
		"~~",
		"https://example.com/1",
		"https://example.com/2",
		"https://example.com/3",
	}

	if len(lines) != len(want) {
		t.Fatalf("unexpected lines length: got %d want %d", len(lines), len(want))
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("unexpected line %d: got %q want %q", i, lines[i], want[i])
		}
	}
}

func TestWriteKeywordListText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "list.txt")
	rows := []model.KeywordResultRow{
		{SearchTerm: "abc", ProteinIdentification: "ID1", GeneReportURL: "https://example.com/1"},
	}
	if err := WriteKeywordListText(path, rows); err != nil {
		t.Fatalf("WriteKeywordListText returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read list text: %v", err)
	}
	got := strings.TrimSpace(string(data))
	want := "ID1\n~~\nhttps://example.com/1"
	if got != want {
		t.Fatalf("unexpected file contents: got %q want %q", got, want)
	}
}

func TestBuildKeywordListLinesWithoutProteinIdentification(t *testing.T) {
	rows := []model.KeywordResultRow{
		{SearchTerm: "abc", ProteinIdentification: "", GeneReportURL: "https://example.com/1"},
		{SearchTerm: "def", ProteinIdentification: "", GeneReportURL: "https://example.com/2"},
	}

	lines := buildKeywordListLines(rows)
	want := []string{
		"https://example.com/1",
		"https://example.com/2",
	}

	if len(lines) != len(want) {
		t.Fatalf("unexpected lines length: got %d want %d", len(lines), len(want))
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("unexpected line %d: got %q want %q", i, lines[i], want[i])
		}
	}
}

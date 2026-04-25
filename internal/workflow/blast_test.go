package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

func TestNormalizeGeneReportURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{
			input: "phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
			want:  "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
			ok:    true,
		},
		{
			input: "http://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490?x=1#frag",
			want:  "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
			ok:    true,
		},
		{
			input: "https://example.com/report/gene/Athaliana_TAIR10/AT2G30490",
			ok:    false,
		},
	}

	for _, tc := range tests {
		got, ok := normalizeGeneReportURL(tc.input)
		if ok != tc.ok {
			t.Fatalf("normalizeGeneReportURL(%q) ok=%v want %v", tc.input, ok, tc.ok)
		}
		if got != tc.want {
			t.Fatalf("normalizeGeneReportURL(%q)=%q want %q", tc.input, got, tc.want)
		}
	}
}

func TestBuildQuerySequenceLabel(t *testing.T) {
	if got := buildQuerySequenceLabel("A.thaliana", "C4H"); got != "AtC4H" {
		t.Fatalf("unexpected arabidopsis label: %q", got)
	}
	if got := buildQuerySequenceLabel("A.thaliana", "AtCESA1"); got != "AtCESA1" {
		t.Fatalf("unexpected prefixed arabidopsis label: %q", got)
	}
	if got := buildQuerySequenceLabel("S.polyrhiza", "Spipo1"); got != "Spipo1" {
		t.Fatalf("unexpected non-arabidopsis label: %q", got)
	}
}

func TestBuildBlastOutputDisplayNamePreservesLabel(t *testing.T) {
	item := blastQueryItem{ProteinIdentification: "AtCESA4"}
	if got := buildBlastOutputDisplayName(item); got != "AtCESA4" {
		t.Fatalf("unexpected display label: %q", got)
	}
}

func TestSanitizeExportNameDoesNotAffectDisplayLabel(t *testing.T) {
	item := blastQueryItem{ProteinIdentification: "AtCESA4"}
	display := buildBlastOutputDisplayName(item)
	fileName := sanitizeExportName(display)
	if display != "AtCESA4" {
		t.Fatalf("unexpected display label: %q", display)
	}
	if fileName != "AtCESA4" {
		t.Fatalf("unexpected file name: %q", fileName)
	}
}

func TestParseFastaQuerySequenceInput(t *testing.T) {
	source, ok := parseFastaQuerySequenceInput(">A.thaliana TAIR10|AT5G44030.1\nMEPNTMASFDDEH\n")
	if !ok {
		t.Fatalf("expected FASTA header to parse")
	}
	if source.GeneID != "AT5G44030" {
		t.Fatalf("unexpected gene id: %q", source.GeneID)
	}
	if source.TranscriptID != "AT5G44030.1" {
		t.Fatalf("unexpected transcript id: %q", source.TranscriptID)
	}
	if source.ProteinID != "AT5G44030.1" {
		t.Fatalf("unexpected protein id: %q", source.ProteinID)
	}
	if source.OrganismShort != "A.thaliana" {
		t.Fatalf("unexpected organism short: %q", source.OrganismShort)
	}
	if source.Annotation != "TAIR10" {
		t.Fatalf("unexpected annotation: %q", source.Annotation)
	}
	if source.Sequence != "MEPNTMASFDDEH" {
		t.Fatalf("unexpected sequence: %q", source.Sequence)
	}
}

func TestParseFastaQuerySequenceInputSingleLineWithTrailingLabel(t *testing.T) {
	input := ">A.thaliana TAIR10|AT5G44030.1 (AtCESA4) MEPNTMASFDDEHRHSSFSAKIC"
	source, ok := parseFastaQuerySequenceInput(input)
	if !ok {
		t.Fatalf("expected single-line FASTA header to parse")
	}
	if source.GeneID != "AT5G44030" {
		t.Fatalf("unexpected gene id: %q", source.GeneID)
	}
	if source.TranscriptID != "AT5G44030.1" {
		t.Fatalf("unexpected transcript id: %q", source.TranscriptID)
	}
	if source.Sequence != "MEPNTMASFDDEHRHSSFSAKIC" {
		t.Fatalf("unexpected sequence: %q", source.Sequence)
	}
}

func TestStripTrailingParentheticalLabel(t *testing.T) {
	got := stripTrailingParentheticalLabel("A.thaliana TAIR10|AT5G44030.1 (AtCESA4)")
	if got != "A.thaliana TAIR10|AT5G44030.1" {
		t.Fatalf("unexpected stripped label: %q", got)
	}
}

func TestParseFastaQuerySequenceInputPlainSequence(t *testing.T) {
	if source, ok := parseFastaQuerySequenceInput("MEPNTMASFDDEH\n"); ok || source != nil {
		t.Fatalf("plain sequence should not produce query metadata")
	}
}

func TestBuildQuerySequenceHeaderID(t *testing.T) {
	source := &model.QuerySequenceSource{
		OrganismShort: "A.thaliana",
		Annotation:    "TAIR10",
		ProteinID:     "AT5G44030.1",
	}
	if got := buildQuerySequenceHeaderID(source); got != "A.thaliana TAIR10|AT5G44030.1" {
		t.Fatalf("unexpected query header id: %q", got)
	}
}

func TestDescribeQuerySourceCrossDatabaseURL(t *testing.T) {
	source := &model.QuerySequenceSource{
		NormalizedURL:  "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
		SourceDatabase: "phytozome",
	}
	got := describeQuerySource(source, "lemna")
	want := "Resolved peptide sequence from a Phytozome gene report URL. The sequence will be fetched from Phytozome and searched against the selected lemna.org species."
	if got != want {
		t.Fatalf("unexpected description: %q", got)
	}
}

func TestDescribeQuerySourceSameDatabaseURL(t *testing.T) {
	source := &model.QuerySequenceSource{
		NormalizedURL:  "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
		SourceDatabase: "phytozome",
	}
	got := describeQuerySource(source, "phytozome")
	want := "Resolved peptide sequence from a Phytozome gene report URL."
	if got != want {
		t.Fatalf("unexpected description: %q", got)
	}
}

func TestBuildExportMetadataPrefersOriginalInputURL(t *testing.T) {
	source := &model.QuerySequenceSource{
		OriginalInputURL: "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490?copied=1",
		NormalizedURL:    "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
		GeneID:           "AT2G30490",
	}

	metadata := buildExportMetadata("C4H", source)
	if metadata == nil {
		t.Fatal("expected export metadata")
	}
	if metadata.GeneReportURL != source.OriginalInputURL {
		t.Fatalf("unexpected metadata URL: %q", metadata.GeneReportURL)
	}
}

type fakeSource struct {
	query *model.QuerySequenceSource
	err   error
}

func (f fakeSource) Name() string { return "fake" }
func (f fakeSource) FetchSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {
	return nil, nil
}
func (f fakeSource) SubmitBlast(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {
	return model.BlastJob{}, nil
}
func (f fakeSource) WaitForBlastResults(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, error) {
	return model.BlastResult{}, nil
}
func (f fakeSource) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return nil, nil
}
func (f fakeSource) FetchProteinSequence(ctx context.Context, targetID int, sequenceID string) (string, error) {
	return "", nil
}
func (f fakeSource) FetchGeneQuerySequence(ctx context.Context, species model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.query, nil
}

func TestResolveGeneReportSequencePreservesInputURLs(t *testing.T) {
	w := &BlastWizard{}
	src := fakeSource{
		query: &model.QuerySequenceSource{
			Sequence:       "MPEPTIDE",
			SourceDatabase: "phytozome",
			GeneID:         "AT2G30490",
		},
	}

	got, err := w.resolveGeneReportSequence(
		context.Background(),
		src,
		model.SpeciesCandidate{JBrowseName: "Athaliana_TAIR10"},
		"gene",
		"AT2G30490",
		"https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490?copied=1",
		"https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
	)
	if err != nil {
		t.Fatalf("resolveGeneReportSequence returned error: %v", err)
	}
	if got.OriginalInputURL != "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490?copied=1" {
		t.Fatalf("unexpected original input URL: %q", got.OriginalInputURL)
	}
	if got.NormalizedURL != "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490" {
		t.Fatalf("unexpected normalized URL: %q", got.NormalizedURL)
	}
}

func TestParseBlastLoadCommand(t *testing.T) {
	filename, ok := parseBlastLoadCommand(`load "queries.txt"`)
	if !ok {
		t.Fatalf("expected load command to parse")
	}
	if filename != "queries.txt" {
		t.Fatalf("unexpected filename: %q", filename)
	}
}

func TestParseBlastClipboardItems(t *testing.T) {
	raw := "PAL1\nPAL2\n~~\nhttps://example.com/1\nhttps://example.com/2\n"
	items, err := parseBlastQueryItems(raw)
	if err != nil {
		t.Fatalf("parseBlastQueryItems returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("unexpected item count: %d", len(items))
	}
	if items[0].ProteinIdentification != "PAL1" || items[1].ProteinIdentification != "PAL2" {
		t.Fatalf("unexpected labels: %#v", items)
	}
	if items[0].RawInput != "https://example.com/1" || items[1].RawInput != "https://example.com/2" {
		t.Fatalf("unexpected raw inputs: %#v", items)
	}
}

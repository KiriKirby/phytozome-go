package workflow

import (
	"testing"

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
	if got := buildQuerySequenceLabel("S.polyrhiza", "Spipo1"); got != "Spipo1" {
		t.Fatalf("unexpected non-arabidopsis label: %q", got)
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

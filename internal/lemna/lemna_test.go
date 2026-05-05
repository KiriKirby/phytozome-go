package lemna

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

func TestFindBlastDBIDMatchesOfficialCloneOption(t *testing.T) {
	body := `<select name="SELECT_DB">
<option value="">Select a Dataset</option>
<option value="11">Le. gibba 7742a REF-CSHL-1.0 Genome</option>
</select>`
	rel := releaseInfo{
		RootDir:      "Le_gibba_7742a",
		ReleaseDir:   "Le_gibba_7742a-REF-CSHL-1.0",
		DisplayLabel: "Lemna gibba 7742a REF-CSHL-1.0",
	}

	id, ok := findBlastDBID(body, rel)
	if !ok {
		t.Fatal("expected DB id match")
	}
	if id != 11 {
		t.Fatalf("unexpected DB id: got %d want 11", id)
	}
}

func TestLocalBlastDatabaseSelectsDBType(t *testing.T) {
	rel := releaseInfo{
		ProteinURL:    "https://example.test/proteins.fasta.gz",
		NucleotideURL: "https://example.test/genome.fasta.gz",
	}

	fastaURL, dbType, err := localBlastDatabase(rel, "blastp")
	if err != nil {
		t.Fatalf("blastp database failed: %v", err)
	}
	if fastaURL != rel.ProteinURL || dbType != "prot" {
		t.Fatalf("blastp got %q/%q, want protein/prot", fastaURL, dbType)
	}

	fastaURL, dbType, err = localBlastDatabase(rel, "tblastn")
	if err != nil {
		t.Fatalf("tblastn database failed: %v", err)
	}
	if fastaURL != rel.NucleotideURL || dbType != "nucl" {
		t.Fatalf("tblastn got %q/%q, want nucleotide/nucl", fastaURL, dbType)
	}
}

func TestWaitForLocalBlastResultsReturnsCachedResultImmediately(t *testing.T) {
	client := NewClient(nil)
	client.localResultsCache["local-test"] = model.BlastResult{
		JobID: "local-test",
		Rows:  []model.BlastResultRow{{Protein: "Spipo1G0000100"}},
	}

	result, err := client.WaitForBlastResults(context.Background(), "local-test", time.Hour, time.Hour)
	if err != nil {
		t.Fatalf("WaitForBlastResults returned error: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0].Protein != "Spipo1G0000100" {
		t.Fatalf("unexpected cached result: %#v", result)
	}
}

func TestWaitForLocalBlastResultsDoesNotPollMissingCache(t *testing.T) {
	client := NewClient(nil)
	start := time.Now()
	_, err := client.WaitForBlastResults(context.Background(), "local-missing", time.Hour, time.Hour)
	if err == nil {
		t.Fatal("expected missing local cache error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("missing local cache should fail immediately, took %s", elapsed)
	}
	if !strings.Contains(err.Error(), "cached result") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNucleotideFileScorePrefersGeneLevelTargets(t *testing.T) {
	genome := nucleotideFileScore("Le_gibba_7742a-REF-CSHL-1.0.fasta.gz")
	transcripts := nucleotideFileScore("Le_gibba_7742a_CSHL2022v1.genes.transcripts.primary.fasta.gz")
	cds := nucleotideFileScore("Le_gibba_7742a_CSHL2022v1.genes.cds.primary.fasta.gz")

	if !(cds > transcripts && transcripts > genome) {
		t.Fatalf("unexpected nucleotide FASTA priority: cds=%d transcripts=%d genome=%d", cds, transcripts, genome)
	}
}

func TestEnrichBlastRowsWithAHRDDoesNotWriteDescriptionIntoGeneReportURL(t *testing.T) {
	rows := []model.BlastResultRow{
		{Protein: "Sp9509d020g000340_T001"},
	}
	ahrd := map[string]ahrdRecord{
		"Sp9509d020g000340_T001": {
			BlastHitAccession:        "sp|Q43158|PER_SPIOL",
			HumanReadableDescription: "cinnamyl alcohol dehydrogenase",
		},
	}

	enrichBlastRowsWithAHRD(rows, ahrd)

	if rows[0].TranscriptID != "Sp9509d020g000340_T001" {
		t.Fatalf("unexpected transcript id: %q", rows[0].TranscriptID)
	}
	if rows[0].Defline != "cinnamyl alcohol dehydrogenase" {
		t.Fatalf("unexpected defline: %q", rows[0].Defline)
	}
	if rows[0].GeneReportURL != "" {
		t.Fatalf("expected GeneReportURL to stay empty, got %q", rows[0].GeneReportURL)
	}
	if rows[0].UniProtAccession != "Q43158" {
		t.Fatalf("unexpected UniProt accession: %q", rows[0].UniProtAccession)
	}
}

func TestKeywordShortLabelFromGFFPrefersAliasSymbol(t *testing.T) {
	label := keywordShortLabelFromGFF("cinnamate", map[string]string{
		"Alias":      "C4H; cinnamate 4-hydroxylase",
		"protein_id": "Sp9509d020g000340_T001",
	}, "")
	if label != "C4H" {
		t.Fatalf("unexpected label: %q", label)
	}
}

func TestKeywordShortLabelFromDescriptionParentheses(t *testing.T) {
	label := keywordShortLabelFromGFF("cinnamate", map[string]string{
		"Name": "Sp9509d020g000340_T001",
	}, "cinnamate 4-hydroxylase (C4H)")
	if label != "" {
		t.Fatalf("unexpected label: %q", label)
	}
}

func TestKeywordShortLabelFromAHRDUsesDescriptionSymbol(t *testing.T) {
	label := keywordShortLabelFromAHRD("cinnamate", ahrdRecord{
		HumanReadableDescription: "cinnamate 4-hydroxylase (C4H)",
	})
	if label != "C4H" {
		t.Fatalf("unexpected label: %q", label)
	}
}

func TestBuildKeywordRowFromGFFKeepsProteinIDSeparateFromLabel(t *testing.T) {
	row := buildKeywordRowFromGFF(
		model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza", JBrowseName: "Sp_test"},
		releaseInfo{ReleaseDir: "test-release", ReleaseURL: "https://example.test/release", GFFURL: "https://example.test/test.gff3"},
		"C4H",
		gffRow{
			SeqID:      "chr1",
			Source:     "test",
			Type:       "mRNA",
			Start:      "1",
			End:        "10",
			Strand:     "+",
			Attributes: "ID=Sp9509d020g000340_T001;protein_id=prot123;Alias=C4H",
			AttrMap: map[string]string{
				"ID":         "Sp9509d020g000340_T001",
				"protein_id": "prot123",
				"Alias":      "C4H",
			},
		},
	)

	if row.LabelName != "C4H" {
		t.Fatalf("unexpected label name: %q", row.LabelName)
	}
	if row.ProteinID != "prot123" {
		t.Fatalf("unexpected protein id: %q", row.ProteinID)
	}
}

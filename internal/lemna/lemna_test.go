package lemna

import (
	"testing"

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
}

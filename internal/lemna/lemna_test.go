// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package lemna

import (
	"context"
	"net/http"
	"os"
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

func TestFetchUniProtAccessionsReplaySamples(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-backed lemna replay sample test in short mode")
	}
	client := NewClient(http.DefaultClient)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := client.FetchSpeciesCandidates(ctx); err != nil {
		t.Fatalf("fetch species candidates: %v", err)
	}

	ids := []string{
		"Sp9509d011g008180_T004",
		"Sp9509d006g004400_T001",
		"Sp9509d012g006190_T001",
		"Sp9509d012g006280_T001",
		"Sp9509d002g009930_T001",
		"Sp9509d018g003250_T001",
		"Sp9509d006g004900_T001",
	}
	for _, id := range ids {
		accs, err := client.FetchUniProtAccessions(ctx, 18, id)
		t.Logf("%s acc=%v err=%v", id, accs, err)
	}
}

func TestLemnaKeywordReplayLiveBySearchType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-backed lemna keyword replay in short mode")
	}
	if os.Getenv("PHYTOZOME_LIVE_REPLAY") == "" {
		t.Skip("set PHYTOZOME_LIVE_REPLAY=1 to run live lemna keyword replay")
	}

	client := NewClient(http.DefaultClient)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	species := model.SpeciesCandidate{
		ProteomeID:  18,
		JBrowseName: "Sp_polyrhiza_9509",
		GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
		SearchAlias: "Spirodela polyrhiza",
		IsOfficial:  true,
	}

	tests := []struct {
		name            string
		term            string
		wantSearchType  string
		wantLabelPrefix string
		minRows         int
	}{
		{
			name:           "report-url-gene",
			term:           "https://www.lemna.org/report/Sp_polyrhiza_9509/Sp9509d020g000340",
			wantSearchType: "report URL: gene",
			minRows:        1,
		},
		{
			name:           "transcript-id",
			term:           "Sp9509d020g000340_T001",
			wantSearchType: "lemna transcript identifier",
			minRows:        1,
		},
		{
			name:           "gene-id",
			term:           "Sp9509d020g000340",
			wantSearchType: "lemna gene identifier",
			minRows:        1,
		},
		{
			name:           "transcript-id-case-insensitive",
			term:           "sp9509d020g000340_t001",
			wantSearchType: "lemna transcript identifier",
			minRows:        1,
		},
		{
			name:            "rice-locus-curated-fallback",
			term:            "LOC_Os05g25640",
			wantSearchType:  "rice LOC_Os locus",
			wantLabelPrefix: "C4H",
			minRows:         1,
		},
		{
			name:            "refseq-curated-fallback",
			term:            "XP_015639656",
			wantSearchType:  "RefSeq XP protein",
			wantLabelPrefix: "C4H",
			minRows:         1,
		},
		{
			name:            "rice-alias-curated-fallback",
			term:            "OsC4H1",
			wantSearchType:  "gene alias / symbol",
			wantLabelPrefix: "C4H",
			minRows:         1,
		},
		{
			name:            "cytochrome-family-curated-fallback",
			term:            "CYP73A38",
			wantSearchType:  "CYP73 family symbol",
			wantLabelPrefix: "C4H",
			minRows:         1,
		},
		{
			name:           "keyword",
			term:           "phenylalanine ammonia lyase",
			wantSearchType: "keyword",
			minRows:        1,
		},
		{
			name:           "keyword-case-insensitive",
			term:           "PHENYLALANINE AMMONIA LYASE",
			wantSearchType: "keyword",
			minRows:        1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			rows, err := client.SearchKeywordRows(ctx, species, tt.term)
			if err != nil {
				t.Fatalf("SearchKeywordRows(%q): %v", tt.term, err)
			}
			if len(rows) < tt.minRows {
				t.Fatalf("SearchKeywordRows(%q) rows=%d, want >= %d", tt.term, len(rows), tt.minRows)
			}
			if rows[0].SearchType != tt.wantSearchType {
				t.Fatalf("SearchKeywordRows(%q) searchType=%q, want %q", tt.term, rows[0].SearchType, tt.wantSearchType)
			}
			if tt.wantLabelPrefix != "" && !strings.EqualFold(strings.TrimSpace(rows[0].LabelName), tt.wantLabelPrefix) {
				t.Fatalf("SearchKeywordRows(%q) label=%q, want %q", tt.term, rows[0].LabelName, tt.wantLabelPrefix)
			}
		})
	}
}

func TestLemnaKeywordWideReplayLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-backed lemna wide replay in short mode")
	}
	if os.Getenv("PHYTOZOME_LIVE_REPLAY") == "" {
		t.Skip("set PHYTOZOME_LIVE_REPLAY=1 to run live lemna wide replay")
	}

	client := NewClient(http.DefaultClient)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	species := model.SpeciesCandidate{
		ProteomeID:  18,
		JBrowseName: "Sp_polyrhiza_9509",
		GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
		SearchAlias: "Spirodela polyrhiza",
		IsOfficial:  true,
	}

	rows, err := client.SearchKeywordRowsWide(ctx, species, "4cl web style")
	if err != nil {
		t.Fatalf("SearchKeywordRowsWide: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("SearchKeywordRowsWide returned no rows")
	}
	if rows[0].SearchType != "wide search" {
		t.Fatalf("wide search type = %q, want wide search", rows[0].SearchType)
	}

	mixed, err := client.SearchKeywordRows(ctx, species, "4cl web style")
	if err != nil {
		t.Fatalf("SearchKeywordRows mixed wide case: %v", err)
	}
	if len(mixed) == 0 {
		t.Fatal("SearchKeywordRows returned no wide-fallback rows")
	}
	if !strings.Contains(strings.ToLower(mixed[0].SearchType), "wide search") {
		t.Fatalf("mixed wide replay should record wide search in search type, got %q", mixed[0].SearchType)
	}
}

func TestLemnaKeywordReplayLiveRice4CLMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-backed lemna 4CL replay in short mode")
	}
	if os.Getenv("PHYTOZOME_LIVE_REPLAY") == "" {
		t.Skip("set PHYTOZOME_LIVE_REPLAY=1 to run live lemna 4CL replay")
	}

	client := NewClient(http.DefaultClient)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	species := model.SpeciesCandidate{
		ProteomeID:  18,
		JBrowseName: "Sp_polyrhiza_9509",
		GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
		SearchAlias: "Spirodela polyrhiza",
		IsOfficial:  true,
	}

	aliasTests := []struct {
		name  string
		term  string
		label string
	}{
		{"alias-1", "Os4CL1", "Os4CL1"},
		{"alias-1-lower", "os4cl1", "Os4CL1"},
		{"alias-2", "Os4CL2", "Os4CL2"},
		{"alias-3", "Os4CL3", "Os4CL3"},
		{"alias-4", "Os4CL4", "Os4CL4"},
		{"alias-5", "Os4CL5", "Os4CL5"},
	}
	for _, tt := range aliasTests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			rows, err := client.SearchKeywordRows(ctx, species, tt.term)
			if err != nil {
				t.Fatalf("SearchKeywordRows(%q): %v", tt.term, err)
			}
			if len(rows) == 0 {
				t.Fatalf("SearchKeywordRows(%q) returned no rows", tt.term)
			}
			if rows[0].SearchType != "gene alias / symbol" {
				t.Fatalf("SearchKeywordRows(%q) searchType=%q, want %q", tt.term, rows[0].SearchType, "gene alias / symbol")
			}
			if rows[0].LabelName != tt.label {
				t.Fatalf("SearchKeywordRows(%q) label=%q, want %q", tt.term, rows[0].LabelName, tt.label)
			}
			if !strings.Contains(strings.ToLower(rows[0].Description), "4-coumarate") {
				t.Fatalf("SearchKeywordRows(%q) description=%q, want 4-coumarate hit", tt.term, rows[0].Description)
			}
		})
	}

	zeroTests := []string{
		"Os08g14760.1",
		"os08g14760.1",
		"Os02g46970.1",
		"Os02g08100.1",
		"Os06g44620.1",
		"Os08g34790.1",
	}
	for _, term := range zeroTests {
		term := term
		t.Run("controlled-zero-"+strings.ReplaceAll(strings.ToLower(term), ".", "_"), func(t *testing.T) {
			rows, err := client.SearchKeywordRows(ctx, species, term)
			if err != nil {
				t.Fatalf("SearchKeywordRows(%q): %v", term, err)
			}
			if len(rows) != 0 {
				t.Fatalf("SearchKeywordRows(%q) rows=%d, want 0 to avoid false-positive remaps; first=%#v", term, len(rows), rows[0])
			}
		})
	}

	xpCases := []struct {
		name  string
		term  string
		label string
	}{
		{"xp-1", "XP_015650724.1", "Os4CL1"},
		{"xp-1-lower", "xp_015650724.1", "Os4CL1"},
		{"xp-2", "XP_015624111.1", "Os4CL2"},
		{"xp-3", "XP_015625716.1", "Os4CL3"},
		{"xp-4", "XP_015643415.1", "Os4CL4"},
		{"xp-5", "XP_015650830.1", "Os4CL5"},
	}
	for _, tt := range xpCases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			rows, err := client.SearchKeywordRows(ctx, species, tt.term)
			if err != nil {
				t.Fatalf("SearchKeywordRows(%q): %v", tt.term, err)
			}
			if len(rows) == 0 {
				t.Fatalf("SearchKeywordRows(%q) returned no rows", tt.term)
			}
			if rows[0].SearchType != "RefSeq XP protein" {
				t.Fatalf("SearchKeywordRows(%q) searchType=%q, want %q", tt.term, rows[0].SearchType, "RefSeq XP protein")
			}
			if rows[0].LabelName != tt.label {
				t.Fatalf("SearchKeywordRows(%q) label=%q, want %q", tt.term, rows[0].LabelName, tt.label)
			}
			if !strings.Contains(strings.ToLower(rows[0].Description), "4-coumarate") {
				t.Fatalf("SearchKeywordRows(%q) description=%q, want 4-coumarate hit", tt.term, rows[0].Description)
			}
		})
	}
}

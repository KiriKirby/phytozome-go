// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package lemna

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/model"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

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

func TestEnrichServerBlastCapabilityTracksProgramSpecificAvailability(t *testing.T) {
	client := NewClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := `<select name="SELECT_DB"><option value="">Select a Dataset</option>`
			switch req.URL.Path {
			case "/blast/nucleotide/nucleotide":
				body += `<option value="18">Spirodela polyrhiza 9509 REF-OXFORD-3.0 Genome</option>`
			case "/blast/protein/nucleotide":
				body += `<option value="18">Spirodela polyrhiza 9509 REF-OXFORD-3.0 Genome</option>`
			default:
				body += ``
			}
			body += `</select>`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	})
	rel := releaseInfo{
		RootDir:      "Sp_polyrhiza_9509",
		ReleaseDir:   "Sp_polyrhiza_9509-REF-OXFORD-3.0",
		DisplayLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
		BlastNDBID:   18,
	}
	cap := BlastCapability{}
	client.enrichServerBlastCapability(context.Background(), rel, &cap)

	if !cap.ServerBlastNAvailable || !cap.ServerTBlastNAvailable {
		t.Fatalf("expected nucleotide server programs to be available: %#v", cap)
	}
	if cap.ServerBlastXAvailable || cap.ServerBlastPAvailable {
		t.Fatalf("expected protein server programs to remain unavailable: %#v", cap)
	}
}

func TestLocalBlastDatabaseSelectsDBType(t *testing.T) {
	rel := releaseInfo{
		ProteinURL:    "https://example.test/proteins.fasta.gz",
		NucleotideURL: "https://example.test/genome.fasta.gz",
		AvailableFiles: []downloadFile{
			{Name: "species.genes.transcripts.primary.fasta.gz", URL: "https://example.test/transcripts.fasta.gz"},
			{Name: "species.genes.cds.primary.fasta.gz", URL: "https://example.test/cds.fasta.gz"},
			{Name: "species.fasta.gz", URL: "https://example.test/genome.fasta.gz"},
		},
	}

	fastaURL, dbType, dbKey, err := localBlastDatabase(rel, "blastp")
	if err != nil {
		t.Fatalf("blastp database failed: %v", err)
	}
	if fastaURL != rel.ProteinURL || dbType != "prot" {
		t.Fatalf("blastp got %q/%q, want protein/prot", fastaURL, dbType)
	}
	if !strings.Contains(dbKey, "blastp") {
		t.Fatalf("blastp dbKey=%q should include program name", dbKey)
	}

	fastaURL, dbType, dbKey, err = localBlastDatabase(rel, "tblastn")
	if err != nil {
		t.Fatalf("tblastn database failed: %v", err)
	}
	if fastaURL != "https://example.test/cds.fasta.gz" || dbType != "nucl" {
		t.Fatalf("tblastn got %q/%q, want cds/nucl", fastaURL, dbType)
	}
	if !strings.Contains(dbKey, "tblastn") {
		t.Fatalf("tblastn dbKey=%q should include program name", dbKey)
	}

	fastaURL, dbType, _, err = localBlastDatabase(rel, "blastn")
	if err != nil {
		t.Fatalf("blastn database failed: %v", err)
	}
	if fastaURL != "https://example.test/transcripts.fasta.gz" || dbType != "nucl" {
		t.Fatalf("blastn got %q/%q, want transcripts/nucl", fastaURL, dbType)
	}
}

func TestNormalizeProgramKeepsTBlastnDistinctFromBlastn(t *testing.T) {
	got, err := normalizeProgram("local:TBLASTN")
	if err != nil {
		t.Fatalf("normalizeProgram returned error: %v", err)
	}
	if got != "tblastn" {
		t.Fatalf("normalizeProgram(local:TBLASTN) = %q, want tblastn", got)
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

func TestLocalBlastRequestCacheKeyAndHitRequireCachedResult(t *testing.T) {
	client := NewClient(nil)
	req := model.BlastRequest{
		Species:          model.SpeciesCandidate{JBrowseName: "Sp9509", GenomeLabel: "Spirodela", ProteomeID: 18},
		Program:          "local:BLASTP",
		Sequence:         "MPEPTIDE",
		EValue:           "1e-5",
		AlignmentsToShow: 25,
	}
	key := localBlastRequestCacheKey(req, "blastp")
	if key == "" {
		t.Fatal("empty local BLAST request cache key")
	}
	client.mu.Lock()
	client.localBlastJobCache[key] = model.BlastJob{JobID: "local-cached"}
	client.mu.Unlock()
	if _, ok := client.cachedLocalBlastJob(key); ok {
		t.Fatal("job cache must not hit without the matching result cache")
	}
	client.mu.Lock()
	client.localResultsCache["local-cached"] = model.BlastResult{JobID: "local-cached"}
	client.mu.Unlock()
	if job, ok := client.cachedLocalBlastJob(key); !ok || job.JobID != "local-cached" {
		t.Fatalf("cachedLocalBlastJob()=%#v/%v, want local-cached/true", job, ok)
	}

	changed := req
	changed.AlignmentsToShow = 50
	if localBlastRequestCacheKey(changed, "blastp") == key {
		t.Fatal("local BLAST cache key should include max hit setting")
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

func TestLocalNucleotideFileScorePrefersProgramSpecificTargets(t *testing.T) {
	transcripts := "species.genes.transcripts.primary.fasta.gz"
	cds := "species.genes.cds.primary.fasta.gz"
	genome := "species.fasta.gz"

	if localNucleotideFileScore(cds, "tblastn") <= localNucleotideFileScore(transcripts, "tblastn") {
		t.Fatalf("tblastn should prefer cds over transcripts")
	}
	if localNucleotideFileScore(transcripts, "blastn") <= localNucleotideFileScore(cds, "blastn") {
		t.Fatalf("blastn should prefer transcripts over cds")
	}
	if localNucleotideFileScore(genome, "blastn") >= localNucleotideFileScore(transcripts, "blastn") {
		t.Fatalf("blastn should prefer transcripts over genome")
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

func TestEnrichBlastRowsWithMappingsUsesTranscriptDirectlyForNucleotideHits(t *testing.T) {
	rows := []model.BlastResultRow{
		{
			Protein:    "Sp9509d020g000340_T001",
			SubjectID:  "Sp9509d020g000340_T001",
			SequenceID: "Sp9509d020g000340_T001",
		},
	}
	rel := releaseInfo{
		RootDir:      "Sp_polyrhiza_9509",
		ReleaseURL:   "https://example.test/release",
		DisplayLabel: "Spirodela polyrhiza 9509",
		BlastNDBID:   18,
	}
	transToGene := map[string]string{
		"Sp9509d020g000340_T001": "Sp9509d020g000340",
	}

	enrichBlastRowsWithMappings(rel, &rows, nil, nil, transToGene, nil)

	if rows[0].TranscriptID != "Sp9509d020g000340_T001" {
		t.Fatalf("TranscriptID = %q, want direct subject transcript", rows[0].TranscriptID)
	}
	wantURL := "https://www.lemna.org/report/Sp_polyrhiza_9509/Sp9509d020g000340"
	if rows[0].GeneReportURL != wantURL {
		t.Fatalf("GeneReportURL = %q, want %q", rows[0].GeneReportURL, wantURL)
	}
	if rows[0].JBrowseName != "Sp_polyrhiza_9509" {
		t.Fatalf("JBrowseName = %q, want Sp_polyrhiza_9509", rows[0].JBrowseName)
	}
	if rows[0].TargetID != 18 {
		t.Fatalf("TargetID = %d, want 18", rows[0].TargetID)
	}
}

func TestRunBlastAndParseProgramsWithRealBlastPlus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real BLAST+ execution test in short mode")
	}
	if err := ensureBlastTools("blastp"); err != nil {
		t.Skipf("BLAST+ blastp/makeblastdb not available: %v", err)
	}
	if err := ensureBlastTools("blastx"); err != nil {
		t.Skipf("BLAST+ blastx/makeblastdb not available: %v", err)
	}
	if err := ensureBlastTools("blastn"); err != nil {
		t.Skipf("BLAST+ blastn/makeblastdb not available: %v", err)
	}
	if err := ensureBlastTools("tblastn"); err != nil {
		t.Skipf("BLAST+ tblastn/makeblastdb not available: %v", err)
	}

	tmpDir := t.TempDir()
	protein1 := "MAMAPRTEINSTRINGMAMAPRTEINSTRING"
	nucleotide1 := "ATGGCTATGGCTCCTCGTACTGAAATTAATTCTACTCGTATTAATGGTATGGCTATGGCTCCTCGTACTGAAATTAATTCTACTCGTATTAATGGT"

	proteinFASTA := filepath.Join(tmpDir, "protein.fasta")
	if err := os.WriteFile(proteinFASTA, []byte(
		">prot1 protein one\n"+protein1+"\n>prot2 protein two\nGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG\n",
	), 0o644); err != nil {
		t.Fatalf("write protein FASTA: %v", err)
	}
	nucleotideFASTA := filepath.Join(tmpDir, "nucleotide.fasta")
	if err := os.WriteFile(nucleotideFASTA, []byte(
		">transcript1 transcript one\n"+nucleotide1+"\n>transcript2 transcript two\nGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG\n",
	), 0o644); err != nil {
		t.Fatalf("write nucleotide FASTA: %v", err)
	}

	if err := ensureBlastDB(context.Background(), proteinFASTA, filepath.Join(tmpDir, "prot_db"), "prot"); err != nil {
		t.Fatalf("make protein blast db: %v", err)
	}
	if err := ensureBlastDB(context.Background(), nucleotideFASTA, filepath.Join(tmpDir, "nucl_db"), "nucl"); err != nil {
		t.Fatalf("make nucleotide blast db: %v", err)
	}

	protIdx, err := buildFastaIndex(proteinFASTA)
	if err != nil {
		t.Fatalf("build protein index: %v", err)
	}
	nuclIdx, err := buildFastaIndex(nucleotideFASTA)
	if err != nil {
		t.Fatalf("build nucleotide index: %v", err)
	}

	tests := []struct {
		name                  string
		program               string
		dbPrefix              string
		index                 map[string]fastaEntry
		query                 string
		wantSubject           string
		wantTargetLen         int
		wantQueryFrom         int
		wantTargetFrom        int
		wantQueryLength       int
		wantCoveragePositive  bool
		wantPositivesPositive bool
		wantStrandsSet        bool
	}{
		{
			name:                  "blastp",
			program:               "blastp",
			dbPrefix:              filepath.Join(tmpDir, "prot_db"),
			index:                 protIdx,
			query:                 protein1,
			wantSubject:           "prot1",
			wantTargetLen:         len(protein1),
			wantQueryFrom:         1,
			wantTargetFrom:        1,
			wantQueryLength:       len(protein1),
			wantCoveragePositive:  true,
			wantPositivesPositive: true,
		},
		{
			name:                  "blastx",
			program:               "blastx",
			dbPrefix:              filepath.Join(tmpDir, "prot_db"),
			index:                 protIdx,
			query:                 nucleotide1,
			wantSubject:           "prot1",
			wantTargetLen:         len(protein1),
			wantQueryFrom:         1,
			wantTargetFrom:        1,
			wantQueryLength:       len(nucleotide1),
			wantCoveragePositive:  true,
			wantPositivesPositive: true,
			wantStrandsSet:        true,
		},
		{
			name:                 "blastn",
			program:              "blastn",
			dbPrefix:             filepath.Join(tmpDir, "nucl_db"),
			index:                nuclIdx,
			query:                nucleotide1,
			wantSubject:          "transcript1",
			wantTargetLen:        len(nucleotide1),
			wantQueryFrom:        1,
			wantTargetFrom:       1,
			wantQueryLength:      len(nucleotide1),
			wantCoveragePositive: true,
		},
		{
			name:                  "tblastn",
			program:               "tblastn",
			dbPrefix:              filepath.Join(tmpDir, "nucl_db"),
			index:                 nuclIdx,
			query:                 protein1,
			wantSubject:           "transcript1",
			wantTargetLen:         len(nucleotide1),
			wantQueryFrom:         1,
			wantTargetFrom:        1,
			wantQueryLength:       len(protein1),
			wantCoveragePositive:  true,
			wantPositivesPositive: true,
			wantStrandsSet:        true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			res, err := runBlastAndParse(context.Background(), tt.program, tt.dbPrefix, tt.index, model.BlastRequest{
				Program:          strings.ToUpper(tt.program),
				Sequence:         tt.query,
				AlignmentsToShow: 5,
				EValue:           "10",
			})
			if err != nil {
				t.Fatalf("runBlastAndParse(%s): %v", tt.program, err)
			}
			if len(res.Rows) == 0 {
				t.Fatalf("runBlastAndParse(%s) returned no rows", tt.program)
			}
			row := res.Rows[0]
			if row.BlastProgram != strings.ToUpper(tt.program) {
				t.Fatalf("BlastProgram = %q, want %q", row.BlastProgram, strings.ToUpper(tt.program))
			}
			if row.SubjectID != tt.wantSubject {
				t.Fatalf("SubjectID = %q, want %q", row.SubjectID, tt.wantSubject)
			}
			if row.SequenceID != tt.wantSubject {
				t.Fatalf("SequenceID = %q, want %q", row.SequenceID, tt.wantSubject)
			}
			if row.TargetLength != tt.wantTargetLen {
				t.Fatalf("TargetLength = %d, want %d", row.TargetLength, tt.wantTargetLen)
			}
			if row.QueryFrom != tt.wantQueryFrom {
				t.Fatalf("QueryFrom = %d, want %d", row.QueryFrom, tt.wantQueryFrom)
			}
			if row.TargetFrom != tt.wantTargetFrom {
				t.Fatalf("TargetFrom = %d, want %d", row.TargetFrom, tt.wantTargetFrom)
			}
			if row.QueryLength != tt.wantQueryLength {
				t.Fatalf("QueryLength = %d, want %d", row.QueryLength, tt.wantQueryLength)
			}
			if tt.wantCoveragePositive && row.AlignQueryLengthPercent <= 0 {
				t.Fatalf("AlignQueryLengthPercent = %.2f, want positive", row.AlignQueryLengthPercent)
			}
			if tt.wantPositivesPositive && row.Positives <= 0 {
				t.Fatalf("Positives = %d, want positive", row.Positives)
			}
			if tt.wantStrandsSet && strings.TrimSpace(row.Strands) == "" {
				t.Fatalf("Strands = %q, want non-empty", row.Strands)
			}
		})
	}
}

func TestLocalBlastPWithUserProvidedFASTA(t *testing.T) {
	queryPath := strings.TrimSpace(os.Getenv("PHYTO_LEMNA_USER_FASTA"))
	if queryPath == "" {
		t.Skip("set PHYTO_LEMNA_USER_FASTA to run a real lemna local BLASTP with a user FASTA")
	}
	if testing.Short() {
		t.Skip("skipping real lemna local BLASTP in short mode")
	}
	if err := ensureBlastTools("blastp"); err != nil {
		t.Skipf("BLAST+ blastp/makeblastdb not available: %v", err)
	}
	queryBytes, err := os.ReadFile(queryPath)
	if err != nil {
		t.Fatalf("read query FASTA %s: %v", queryPath, err)
	}
	queryFASTA, queryLengths, _, err := localBlastQueryFASTA(string(queryBytes))
	if err != nil {
		t.Fatalf("normalize query FASTA %s: %v", queryPath, err)
	}
	if len(queryLengths) < 2 {
		t.Fatalf("expected multi-entry query FASTA, got lengths %#v", queryLengths)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	client := NewClient(nil)
	candidates, err := client.FetchSpeciesCandidates(ctx)
	if err != nil {
		t.Fatalf("fetch lemna species candidates: %v", err)
	}
	targetName := strings.TrimSpace(os.Getenv("PHYTO_LEMNA_USER_FASTA_SPECIES"))
	if targetName == "" {
		targetName = "Sp_polyrhiza_9509"
	}
	species, ok := findTestSpeciesCandidate(candidates, targetName)
	if !ok {
		t.Fatalf("species %q not found in lemna candidates", targetName)
	}
	if os.Getenv("PHYTO_LEMNA_USER_FASTA_RESET_CACHE") != "" {
		release, err := client.releaseForSpecies(ctx, species)
		if err != nil {
			t.Fatalf("resolve release before cache reset: %v", err)
		}
		if err := resetLocalBlastCache(release.RootDir, release.ReleaseDir); err != nil {
			t.Fatalf("reset local blast cache: %v", err)
		}
	}

	job, err := client.SubmitBlast(ctx, model.BlastRequest{
		Species:          species,
		Program:          "local:blastp",
		Sequence:         queryFASTA,
		EValue:           "1e-5",
		AlignmentsToShow: 5,
		AllowGaps:        true,
	})
	if err != nil {
		t.Fatalf("SubmitBlast local blastp with %s: %v", queryPath, err)
	}
	result, err := client.WaitForBlastResults(ctx, job.JobID, time.Second, 30*time.Minute)
	if err != nil {
		t.Fatalf("WaitForBlastResults(%s): %v", job.JobID, err)
	}
	if len(result.Rows) == 0 {
		t.Fatalf("local BLASTP completed but returned no hits for %s against %s", queryPath, species.DisplayLabel())
	}
	for _, row := range result.Rows {
		if row.QueryID == "" {
			t.Fatalf("row has empty QueryID: %#v", row)
		}
		if row.QueryLength <= 0 {
			t.Fatalf("row has invalid QueryLength: %#v", row)
		}
	}
	t.Logf("local BLASTP with %s against %s returned %d rows", queryPath, species.DisplayLabel(), len(result.Rows))
}

func findTestSpeciesCandidate(candidates []model.SpeciesCandidate, target string) (model.SpeciesCandidate, bool) {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, candidate := range candidates {
		if strings.ToLower(strings.TrimSpace(candidate.JBrowseName)) == target {
			return candidate, true
		}
	}
	for _, candidate := range candidates {
		if strings.Contains(strings.ToLower(candidate.DisplayLabel()), target) {
			return candidate, true
		}
	}
	return model.SpeciesCandidate{}, false
}

func TestCompactLocalBlastDBPrefixStaysShortAndStable(t *testing.T) {
	key := "blastp_Sp_polyrhiza_9509-REF-OXFORD-3.0_CSHL2022v1.genes.filt.proteins.with.extra.very.long.name.for.windows.path.behavior"
	prefixA := compactLocalBlastDBPrefix("prot", key)
	prefixB := compactLocalBlastDBPrefix("prot", key)
	if prefixA != prefixB {
		t.Fatalf("compactLocalBlastDBPrefix not stable: %q vs %q", prefixA, prefixB)
	}
	if len(prefixA) > 80 {
		t.Fatalf("compactLocalBlastDBPrefix too long: %d %q", len(prefixA), prefixA)
	}
	if !strings.HasPrefix(prefixA, "lemna_prot_") || !strings.HasSuffix(prefixA, "_db") {
		t.Fatalf("unexpected compact prefix shape: %q", prefixA)
	}
}

func TestLocalBlastDBPrefixAndArgsNeverProduceEmptyOut(t *testing.T) {
	tmpDir := t.TempDir()
	prefix, err := localBlastDBPrefix(tmpDir, "prot", "blastp:weird/name\\with*chars")
	if err != nil {
		t.Fatalf("localBlastDBPrefix: %v", err)
	}
	if strings.TrimSpace(prefix) == "" {
		t.Fatal("localBlastDBPrefix returned empty prefix")
	}
	if filepath.Dir(prefix) != tmpDir {
		t.Fatalf("prefix dir = %q, want %q", filepath.Dir(prefix), tmpDir)
	}
	args := makeBlastDBArgs(filepath.Join(tmpDir, "input.fasta"), prefix, "prot", true)
	for i, arg := range args {
		if arg == "-out" {
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				t.Fatalf("makeblastdb args have empty -out value: %#v", args)
			}
			return
		}
	}
	t.Fatalf("makeblastdb args missing -out: %#v", args)
}

func TestLocalBlastDBPrefixUsesShortSharedCacheForLongPaths(t *testing.T) {
	tmpDir := filepath.Join(t.TempDir(), strings.Repeat("very-long-cache-component-", 6))
	prefix, err := localBlastDBPrefix(tmpDir, "prot", "blastp_Sp_polyrhiza_9509-REF-OXFORD-3.0_CSHL2022v1.genes.filt.proteins")
	if err != nil {
		t.Fatalf("localBlastDBPrefix: %v", err)
	}
	if len(prefix) > maxBlastDBPrefixPathLen {
		t.Fatalf("short fallback prefix is still too long: len=%d prefix=%s", len(prefix), prefix)
	}
	if !strings.Contains(filepath.Clean(prefix), filepath.Clean(filepath.Join(".cache", "lemna", "localblastdb"))) {
		t.Fatalf("long cache path should fall back to shared localblastdb cache, got %s", prefix)
	}
}

func TestTemporaryBlastDBPrefixUsesShortBuildDirectory(t *testing.T) {
	longPrefix := filepath.Join(t.TempDir(), strings.Repeat("release-", 20), "lemna_prot_long_name_db")
	tmpDir, tmpPrefix, err := temporaryBlastDBPrefix(longPrefix)
	if err != nil {
		t.Fatalf("temporaryBlastDBPrefix: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	if tmpDir == "" || tmpPrefix == "" {
		t.Fatalf("empty temporary DB path: dir=%q prefix=%q", tmpDir, tmpPrefix)
	}
	if filepath.Dir(tmpPrefix) != tmpDir {
		t.Fatalf("temporary prefix dir = %q, want %q", filepath.Dir(tmpPrefix), tmpDir)
	}
	if strings.Contains(tmpPrefix, filepath.Base(longPrefix)) {
		t.Fatalf("temporary prefix should not inherit long final prefix name: %s", tmpPrefix)
	}
	if len(tmpPrefix) > maxBlastDBPrefixPathLen {
		t.Fatalf("temporary prefix too long: len=%d prefix=%s", len(tmpPrefix), tmpPrefix)
	}
}

func TestPrepareBlastDBSpecRejectsEmptyOutPrefixBeforeMakeblastdb(t *testing.T) {
	tmpDir := t.TempDir()
	fasta := filepath.Join(tmpDir, "query.fasta")
	if err := os.WriteFile(fasta, []byte(">q\nMPEPTIDE\n"), 0o644); err != nil {
		t.Fatalf("write fasta: %v", err)
	}
	_, err := prepareBlastDBSpec(fasta, "", "prot")
	if err == nil {
		t.Fatal("expected empty db prefix to be rejected")
	}
	if !strings.Contains(err.Error(), "without -out") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDownloadAndPrepareFastaRedownloadsInvalidCache(t *testing.T) {
	cacheDir := t.TempDir()
	rawURL := "https://example.test/proteins.fasta"
	destPath, err := localFastaCachePath(cacheDir, rawURL)
	if err != nil {
		t.Fatalf("localFastaCachePath: %v", err)
	}
	if err := os.WriteFile(destPath, []byte("<html>not fasta</html>"), 0o644); err != nil {
		t.Fatalf("write invalid cache: %v", err)
	}
	client := NewClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(">prot1\nMPEPTIDE\n")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	})
	path, err := downloadAndPrepareFasta(context.Background(), client, rawURL, cacheDir)
	if err != nil {
		t.Fatalf("downloadAndPrepareFasta: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read prepared FASTA: %v", err)
	}
	if !strings.HasPrefix(string(data), ">prot1\n") {
		t.Fatalf("invalid cache was not replaced, got %q", string(data))
	}
}

func TestLocalBlastQueryFASTANormalizesPhgoMultiEntryHeaders(t *testing.T) {
	input := strings.Join([]string{
		">phgo://Oryza sativa v7.0/4CL1/LOC_Os08g14760.1\\1",
		"MAAA*",
		"",
		">phgo://Oryza sativa v7.0/4CL2/LOC_Os02g46970.1\\2",
		"MBBBB",
		"",
	}, "\n")
	fasta, lengths, total, err := localBlastQueryFASTA(input)
	if err != nil {
		t.Fatalf("localBlastQueryFASTA: %v", err)
	}
	if !strings.Contains(fasta, ">LOC_Os08g14760.1 phgo://Oryza sativa v7.0/4CL1/LOC_Os08g14760.1\\1") {
		t.Fatalf("first normalized header missing from FASTA:\n%s", fasta)
	}
	if !strings.Contains(fasta, ">LOC_Os02g46970.1 phgo://Oryza sativa v7.0/4CL2/LOC_Os02g46970.1\\2") {
		t.Fatalf("second normalized header missing from FASTA:\n%s", fasta)
	}
	if lengths["LOC_Os08g14760.1"] != 5 || lengths["LOC_Os02g46970.1"] != 5 {
		t.Fatalf("unexpected query lengths: %#v", lengths)
	}
	if total != 10 {
		t.Fatalf("total length = %d, want 10", total)
	}
}

func TestBlastDBCompleteRequiresCoreFilesByType(t *testing.T) {
	tmpDir := t.TempDir()
	protPrefix := filepath.Join(tmpDir, "prot_db")
	if err := os.WriteFile(protPrefix+".pin", []byte("x"), 0o644); err != nil {
		t.Fatalf("write .pin: %v", err)
	}
	if blastDBComplete(protPrefix, "prot") {
		t.Fatal("protein db should not be complete with only .pin")
	}
	for _, ext := range []string{".phr", ".psq"} {
		if err := os.WriteFile(protPrefix+ext, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", ext, err)
		}
	}
	if !blastDBComplete(protPrefix, "prot") {
		t.Fatal("protein db should be complete with .pin/.phr/.psq")
	}

	nuclPrefix := filepath.Join(tmpDir, "nucl_db")
	if err := os.WriteFile(nuclPrefix+".nin", []byte("x"), 0o644); err != nil {
		t.Fatalf("write .nin: %v", err)
	}
	if blastDBComplete(nuclPrefix, "nucl") {
		t.Fatal("nucleotide db should not be complete with only .nin")
	}
	for _, ext := range []string{".nhr", ".nsq"} {
		if err := os.WriteFile(nuclPrefix+ext, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", ext, err)
		}
	}
	if !blastDBComplete(nuclPrefix, "nucl") {
		t.Fatal("nucleotide db should be complete with .nin/.nhr/.nsq")
	}
}

func TestRunBlastAndParseRejectsMissingDatabaseBeforeRunningBlast(t *testing.T) {
	_, err := runBlastAndParse(context.Background(), "blastp", filepath.Join(t.TempDir(), "missing_db"), nil, model.BlastRequest{
		Program:          "BLASTP",
		Sequence:         "MPEPTIDE",
		AlignmentsToShow: 1,
	})
	if err == nil {
		t.Fatal("expected missing database error")
	}
	if !strings.Contains(err.Error(), "database is incomplete or missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveBlastDBFilesClearsPartialDatabaseArtifacts(t *testing.T) {
	tmpDir := t.TempDir()
	prefix := filepath.Join(tmpDir, "lemna_prot_test_db")
	for _, ext := range []string{".pin", ".phr", ".psq", ".pdb", ".ptf", ".pto"} {
		if err := os.WriteFile(prefix+ext, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", ext, err)
		}
	}
	if err := removeBlastDBFiles(prefix); err != nil {
		t.Fatalf("removeBlastDBFiles: %v", err)
	}
	matches, err := filepath.Glob(prefix + ".*")
	if err != nil {
		t.Fatalf("glob leftovers: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no db artifacts left, got %v", matches)
	}
}

func TestResetLocalBlastCacheRemovesTargetReleaseOnly(t *testing.T) {
	cacheRoot, err := appfs.CacheDir("lemna", "localblast")
	if err != nil {
		t.Fatalf("cache root: %v", err)
	}
	targetA := filepath.Join(cacheRoot, "Sp_a", "release-a")
	targetB := filepath.Join(cacheRoot, "Sp_b", "release-b")
	if err := os.MkdirAll(targetA, 0o755); err != nil {
		t.Fatalf("mkdir targetA: %v", err)
	}
	if err := os.MkdirAll(targetB, 0o755); err != nil {
		t.Fatalf("mkdir targetB: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetA, "db.pin"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write targetA file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetB, "db.pin"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write targetB file: %v", err)
	}
	if err := resetLocalBlastCache("Sp_a", "release-a"); err != nil {
		t.Fatalf("resetLocalBlastCache: %v", err)
	}
	if _, err := os.Stat(targetA); !os.IsNotExist(err) {
		t.Fatalf("targetA should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(targetB); err != nil {
		t.Fatalf("targetB should remain, stat err=%v", err)
	}
}

func TestInstallManagedBlastPlusWhenMissing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping managed BLAST+ install test in short mode")
	}
	toolsDir, err := exec.LookPath("makeblastdb")
	if err == nil && strings.TrimSpace(toolsDir) != "" {
		t.Skip("BLAST+ already on PATH; managed install path is not needed for this environment")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	dir, err := installManagedBlastPlusForTest(ctx)
	if err != nil {
		t.Skipf("managed BLAST+ install unavailable in this environment: %v", err)
	}
	if strings.TrimSpace(dir) == "" {
		t.Fatal("managed BLAST+ install returned empty directory")
	}
	if _, err := exec.LookPath("makeblastdb"); err != nil {
		t.Fatalf("makeblastdb still unavailable after managed install: %v", err)
	}
}

func installManagedBlastPlusForTest(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "test", "./internal/blastplus", "-run", "^$", "-count=1")
	cmd.Env = append(os.Environ(), "GOFLAGS=")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, "ok") {
			continue
		}
	}
	// The package test itself does not install tools; call the binary through go test helper.
	helperDir := filepath.Join(".", ".cache", "test-blastplus")
	if err := os.MkdirAll(helperDir, 0o755); err != nil {
		return "", err
	}
	helper := filepath.Join(helperDir, "install_managed_helper_testmain.go")
	content := strings.Join([]string{
		"package main",
		"",
		"import (",
		"\t\"context\"",
		"\t\"fmt\"",
		"\t\"net/http\"",
		"\t\"os\"",
		"\t\"time\"",
		"\t\"github.com/KiriKirby/phytozome-go/internal/blastplus\"",
		")",
		"",
		"func main() {",
		"\tctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)",
		"\tdefer cancel()",
		"\tdir, err := blastplus.InstallManaged(ctx, http.DefaultClient)",
		"\tif err != nil {",
		"\t\tfmt.Println(err)",
		"\t\tos.Exit(1)",
		"\t}",
		"\tfmt.Println(dir)",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(helper, []byte(content), 0o644); err != nil {
		return "", err
	}
	defer os.Remove(helper)
	run := exec.CommandContext(ctx, "go", "run", helper)
	run.Env = append(os.Environ(), "GOFLAGS=")
	out, err := run.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func TestFetchProteinSequenceResolvesTranscriptThroughProteinMappings(t *testing.T) {
	client := NewClient(nil)
	tmpDir := t.TempDir()
	proteinFASTA := filepath.Join(tmpDir, "test.proteins.fasta")
	if err := os.WriteFile(proteinFASTA, []byte(">Sp9509d011g001470_P001 protein\nMPEPTIDE\n"), 0o644); err != nil {
		t.Fatalf("write protein FASTA: %v", err)
	}
	release := releaseInfo{
		RootDir:      "Sp_polyrhiza_9509",
		ReleaseDir:   "test-release",
		ReleaseURL:   "https://example.test/release/",
		GFFURL:       "https://example.test/release/test.gff3",
		ProteinURL:   "https://example.test/release/test.proteins.fasta",
		BlastNDBID:   18,
		DisplayLabel: "Spirodela polyrhiza 9509",
	}
	client.releasesByJBrowseName = map[string]releaseInfo{
		"Sp_polyrhiza_9509": release,
	}
	client.speciesCandidates = []model.SpeciesCandidate{{
		JBrowseName: "Sp_polyrhiza_9509",
		GenomeLabel: "Spirodela polyrhiza 9509",
		ProteomeID:  18,
	}}
	client.proteinTranscriptCache[release.GFFURL] = proteinTranscriptMaps{
		protToTrans: map[string]string{
			"Sp9509d011g001470_P001": "Sp9509d011g001470_T001",
		},
		transToGene: map[string]string{
			"Sp9509d011g001470_T001": "Sp9509d011g001470",
			"Sp9509d011g001470":      "Sp9509d011g001470",
		},
	}
	client.proteinReleaseCache[release.ProteinURL] = map[string]string{
		"Sp9509d011g001470_P001": "MPEPTIDE",
	}
	client.fastaIndexCache[proteinFASTA] = map[string]fastaEntry{
		"Sp9509d011g001470_P001": {Defline: "Sp9509d011g001470_P001 protein", Length: 8},
	}
	cacheDir, err := ensureCacheDir(release.RootDir, release.ReleaseDir)
	if err != nil {
		t.Fatalf("ensureCacheDir: %v", err)
	}
	destPath := filepath.Join(cacheDir, filepath.Base(release.ProteinURL))
	src, err := os.Open(proteinFASTA)
	if err != nil {
		t.Fatalf("open temp protein FASTA: %v", err)
	}
	defer src.Close()
	dst, err := os.Create(destPath)
	if err != nil {
		t.Fatalf("create cached protein FASTA: %v", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		t.Fatalf("copy cached protein FASTA: %v", err)
	}
	if err := dst.Close(); err != nil {
		t.Fatalf("close cached protein FASTA: %v", err)
	}

	record, err := client.FetchProteinSequence(context.Background(), 18, "Sp9509d011g001470_T001")
	if err != nil {
		t.Fatalf("FetchProteinSequence returned error: %v", err)
	}
	if record.Sequence != "MPEPTIDE" {
		t.Fatalf("Sequence = %q, want MPEPTIDE", record.Sequence)
	}
	if record.OriginalHeader != ">Sp9509d011g001470_P001 protein" {
		t.Fatalf("OriginalHeader = %q, want mapped protein header", record.OriginalHeader)
	}
}

func TestLemnaLocalBlastProgramPerformanceMatrix(t *testing.T) {
	if os.Getenv("PHYTO_LEMNA_LOCAL_BLAST_PERF") == "" {
		t.Skip("set PHYTO_LEMNA_LOCAL_BLAST_PERF=1 to run lemna local BLAST four-program performance matrix")
	}
	if err := ensureBlastTools("blastp"); err != nil {
		t.Skipf("BLAST+ not available: %v", err)
	}
	if err := ensureBlastTools("blastx"); err != nil {
		t.Skipf("BLAST+ not available: %v", err)
	}
	if err := ensureBlastTools("blastn"); err != nil {
		t.Skipf("BLAST+ not available: %v", err)
	}
	if err := ensureBlastTools("tblastn"); err != nil {
		t.Skipf("BLAST+ not available: %v", err)
	}

	tmpDir := t.TempDir()
	protein1 := "MAMAPRTEINSTRINGMAMAPRTEINSTRING"
	nucleotide1 := "ATGGCTATGGCTCCTCGTACTGAAATTAATTCTACTCGTATTAATGGTATGGCTATGGCTCCTCGTACTGAAATTAATTCTACTCGTATTAATGGT"

	proteinFASTA := filepath.Join(tmpDir, "protein.fasta")
	if err := os.WriteFile(proteinFASTA, []byte(
		">prot1 protein one\n"+protein1+"\n>prot2 protein two\nGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG\n",
	), 0o644); err != nil {
		t.Fatalf("write protein FASTA: %v", err)
	}
	nucleotideFASTA := filepath.Join(tmpDir, "nucleotide.fasta")
	if err := os.WriteFile(nucleotideFASTA, []byte(
		">transcript1 transcript one\n"+nucleotide1+"\n>transcript2 transcript two\nGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG\n",
	), 0o644); err != nil {
		t.Fatalf("write nucleotide FASTA: %v", err)
	}

	protDB := filepath.Join(tmpDir, "prot_db")
	nuclDB := filepath.Join(tmpDir, "nucl_db")
	if err := ensureBlastDB(context.Background(), proteinFASTA, protDB, "prot"); err != nil {
		t.Fatalf("make protein blast db: %v", err)
	}
	if err := ensureBlastDB(context.Background(), nucleotideFASTA, nuclDB, "nucl"); err != nil {
		t.Fatalf("make nucleotide blast db: %v", err)
	}
	protIdx, err := buildFastaIndex(proteinFASTA)
	if err != nil {
		t.Fatalf("build protein index: %v", err)
	}
	nuclIdx, err := buildFastaIndex(nucleotideFASTA)
	if err != nil {
		t.Fatalf("build nucleotide index: %v", err)
	}

	type perfCase struct {
		program  string
		dbPrefix string
		index    map[string]fastaEntry
		query    string
	}
	cases := []perfCase{
		{program: "blastp", dbPrefix: protDB, index: protIdx, query: protein1},
		{program: "blastx", dbPrefix: protDB, index: protIdx, query: nucleotide1},
		{program: "blastn", dbPrefix: nuclDB, index: nuclIdx, query: nucleotide1},
		{program: "tblastn", dbPrefix: nuclDB, index: nuclIdx, query: protein1},
	}

	workersToTry := []int{1, 2, 4, 8}
	threadsToTry := []int{1, 2, 4, 8, 16}
	if only := strings.TrimSpace(os.Getenv("PHYTO_LEMNA_LOCAL_BLAST_PERF_PROGRAM")); only != "" {
		filtered := cases[:0]
		for _, c := range cases {
			if strings.EqualFold(c.program, only) {
				filtered = append(filtered, c)
			}
		}
		cases = filtered
	}
	if len(cases) == 0 {
		t.Fatal("no lemna local blast perf program selected")
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.program, func(t *testing.T) {
			for _, workers := range workersToTry {
				for _, threads := range threadsToTry {
					if workers*threads > currentCPUCount()*2 {
						continue
					}
					ctx := WithLocalBlastThreads(context.Background(), threads)
					started := time.Now()
					totalRows := 0
					for i := 0; i < workers; i++ {
						res, err := runBlastAndParse(ctx, tc.program, tc.dbPrefix, tc.index, model.BlastRequest{
							Program:          strings.ToUpper(tc.program),
							Sequence:         tc.query,
							AlignmentsToShow: 5,
							EValue:           "10",
						})
						if err != nil {
							t.Fatalf("program=%s workers=%d threads=%d runBlastAndParse failed: %v", tc.program, workers, threads, err)
						}
						totalRows += len(res.Rows)
					}
					elapsed := time.Since(started)
					t.Logf("program=%s batch_workers=%d blast_threads=%d runs=%d rows=%d total_ms=%d", tc.program, workers, threads, workers, totalRows, elapsed.Milliseconds())
				}
			}
		})
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

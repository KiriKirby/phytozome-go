package lemna

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
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

func TestFindBlastDBIDIgnoresNonDatasetSelectOptions(t *testing.T) {
	body := `<form>
<select name="maxTarget">
<option value="10">10</option>
<option value="500">500</option>
</select>
<select name="SELECT_DB">
<option value="">Select a Dataset</option>
</select>
</form>`
	rel := releaseInfo{
		RootDir:      "Le_gibba_7742a",
		ReleaseDir:   "Le_gibba_7742a-REF-CSHL-1.0",
		DisplayLabel: "Lemna gibba 7742a REF-CSHL-1.0",
	}

	if id, ok := findBlastDBID(body, rel); ok || id != 0 {
		t.Fatalf("expected no dataset match, got id=%d ok=%v", id, ok)
	}
}

func TestHasBlastDatasetOptionsOnlyCountsSelectDBOptions(t *testing.T) {
	body := `<form>
<select name="maxTarget">
<option value="10">10</option>
</select>
<select name="SELECT_DB">
<option value="">Select a Dataset</option>
</select>
</form>`
	if hasBlastDatasetOptions(body) {
		t.Fatal("expected empty SELECT_DB dataset list to be treated as unavailable")
	}

	body = `<form><select name="SELECT_DB"><option value="18">Spirodela polyrhiza 9509</option></select></form>`
	if !hasBlastDatasetOptions(body) {
		t.Fatal("expected numeric SELECT_DB options to be treated as available datasets")
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

func TestSearchKeywordRowsDoesNotReuseEmptyMemoryCache(t *testing.T) {
	client := NewClient(nil)
	species := model.SpeciesCandidate{JBrowseName: "Sp_polyrhiza_9509"}
	cacheKey := species.JBrowseName + "|pal"
	client.keywordRowsCache[cacheKey] = []model.KeywordResultRow{}
	client.speciesCandidates = []model.SpeciesCandidate{species}
	client.releasesByJBrowseName[species.JBrowseName] = releaseInfo{}

	_, err := client.SearchKeywordRows(context.Background(), species, "PAL")
	if err == nil {
		t.Fatal("expected search to ignore empty cache and reach release validation")
	}
	if !strings.Contains(err.Error(), "no GFF3") {
		t.Fatalf("unexpected error after empty cache was ignored: %v", err)
	}
}

func TestLemnaKeywordProgramsUseIndexedRows(t *testing.T) {
	client := NewClient(nil)
	species := model.SpeciesCandidate{JBrowseName: "Sp_polyrhiza_9509", GenomeLabel: "Spirodela polyrhiza", ProteomeID: 18}
	release := releaseInfo{RootDir: "Sp_polyrhiza_9509", ReleaseDir: "test-release", GFFURL: "https://example.test/test.gff3.gz"}
	index := lemnaKeywordIndex{
		Release: release,
		Species: species,
		Rows: []model.KeywordResultRow{
			buildKeywordRowFromGFF(species, release, "", gffRow{
				SeqID:      "chr1",
				Source:     "test",
				Type:       "mRNA",
				Start:      "1",
				End:        "9",
				Strand:     "+",
				Attributes: "ID=Sp9509d020g000340_T001;Parent=Sp9509d020g000340;Alias=C4H;product=cinnamate 4-hydroxylase",
				AttrMap: map[string]string{
					"ID":      "Sp9509d020g000340_T001",
					"Parent":  "Sp9509d020g000340",
					"Alias":   "C4H",
					"product": "cinnamate 4-hydroxylase",
				},
			}),
		},
		ByIdentifier:     map[string][]int{},
		BySearchToken:    map[string][]int{},
		ByNormalizedText: map[string][]int{},
	}
	for i := range index.Rows {
		for _, id := range keywordRowIdentifiers(index.Rows[i]) {
			for _, candidate := range normalizedIdentifierCandidates(id) {
				addKeywordIndexHit(index.ByIdentifier, normalizeIdentifierKey(candidate), i)
			}
		}
		for _, token := range keywordRowSearchTokens(index.Rows[i]) {
			addKeywordIndexHit(index.BySearchToken, normalizeIdentifierKey(token), i)
		}
		for _, token := range strings.Fields(normalizeSearchLoose(keywordRowSearchText(index.Rows[i]))) {
			addKeywordIndexHit(index.ByNormalizedText, token, i)
		}
	}

	identifierRows, err := (lemnaIdentifierProgram{}).Search(context.Background(), client, index, species, release, "Sp9509d020g000340_T001", 20)
	if err != nil {
		t.Fatalf("identifier search returned error: %v", err)
	}
	if len(identifierRows) != 1 {
		t.Fatalf("identifier rows = %d, want 1", len(identifierRows))
	}
	wideRows, err := (lemnaWideKeywordProgram{}).Search(context.Background(), client, index, species, release, "cinnamate hydroxylase", 20)
	if err != nil {
		t.Fatalf("wide search returned error: %v", err)
	}
	if len(wideRows) != 1 || wideRows[0].LabelName != "C4H" {
		t.Fatalf("unexpected wide rows: %#v", wideRows)
	}
	broadRows, err := (lemnaBroadKeywordProgram{}).Search(context.Background(), client, index, species, release, "hydroxylase", 20)
	if err != nil {
		t.Fatalf("broad search returned error: %v", err)
	}
	if len(broadRows) != 1 {
		t.Fatalf("broad rows = %d, want 1", len(broadRows))
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

func TestParseAHRDOutputUsesDashedHeaders(t *testing.T) {
	input := strings.Join([]string{
		"Protein-Accession\tBlast-Hit-Accession\tAHRD-Quality-Code\tHuman-Readable-Description\tInterpro\tGene-Ontology-Term",
		"Sp9509d020g000340_T001\tsp|Q43158|PER_SPIOL\t***\tcinnamyl alcohol dehydrogenase\tIPR002085\tGO:0004022",
	}, "\n")
	records := map[string]ahrdRecord{}
	if err := parseAHRDOutput(strings.NewReader(input), records); err != nil {
		t.Fatalf("parseAHRDOutput returned error: %v", err)
	}
	got := records["Sp9509d020g000340_T001"]
	if got.BlastHitAccession != "sp|Q43158|PER_SPIOL" || got.QualityCode != "***" || got.GeneOntologyTerm != "GO:0004022" {
		t.Fatalf("unexpected AHRD record: %#v", got)
	}
}

func TestExtractBlastPageMessage(t *testing.T) {
	body := `<div id="messages"><div class="messages error"><h2 class="element-invisible">Error message</h2>You need to provide a valid <em class="placeholder">nucleotides</em> FASTA sequence for the query.</div></div>`
	got := extractBlastPageMessage(body)
	if !strings.Contains(got, "valid nucleotides FASTA sequence") {
		t.Fatalf("unexpected blast page message: %q", got)
	}
}

func TestExtractBlastReportURL(t *testing.T) {
	body := `<a href="/blast/report/abc123-secret">See Results</a>`
	got := extractBlastReportURL(body, "https://www.lemna.org/blast/nucleotide/nucleotide")
	want := "https://www.lemna.org/blast/report/abc123-secret"
	if got != want {
		t.Fatalf("report url = %q, want %q", got, want)
	}
}

func TestBlastPendingAndCompletedPageDetection(t *testing.T) {
	pending := `<title>BLAST Job in Progress</title><p>Your BLAST job is currently running. The results will be listed here as soon as it completes. This page will automatically refresh.</p>`
	if !isBlastPendingPage(pending) {
		t.Fatal("expected pending page to be detected")
	}
	completed := `<div class="blast-download-info"><strong>Download</strong>: <a href="../../files/example.tsv">Tab-Delimited</a></div><div class="num-results"><strong>Number of Results</strong>: 12</div>`
	if !isBlastCompletedPage(completed) {
		t.Fatal("expected completed page to be detected")
	}
}

func TestParseServerBlastTSVAssignsReleaseMetadata(t *testing.T) {
	data := []byte(strings.Join([]string{
		"# BLASTN 2.6.0+",
		"# Query: q1",
		"# Database: db",
		"# Fields: query acc.ver, subject acc.ver, % identity, alignment length, mismatches, gap opens, q. start, q. end, s. start, s. end, evalue, bit score",
		"q1\tSp9509d020g000340_T001\t99.0\t10\t0\t0\t1\t10\t3\t12\t1e-12\t42",
	}, "\n"))
	rows, err := parseServerBlastTSV(data, releaseInfo{
		RootDir:      "Sp_polyrhiza_9509",
		ReleaseDir:   "Sp_polyrhiza_9509-REF-OXFORD-3.0",
		ReleaseURL:   "https://www.lemna.org/download/Sp_polyrhiza_9509/",
		DisplayLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
		BlastNDBID:   18,
	})
	if err != nil {
		t.Fatalf("parseServerBlastTSV returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Species != "Spirodela polyrhiza 9509 REF-OXFORD-3.0" || row.JBrowseName != "Sp_polyrhiza_9509" || row.TargetID != 18 {
		t.Fatalf("unexpected row metadata: %#v", row)
	}
}

func TestParseCachedBlastResultTSVSupportsCurrentAndLegacyHeaders(t *testing.T) {
	current := strings.Join([]string{
		"hit\tprotein\tsubject_id\tqseqid\tqstart\tqend\tsstart\tsend\tevalue\tpident\talign_len\tmismatch\tgapopen\tbitscore\ttarget_length\tsequence_id\ttranscript_id\ttarget_id\tjbrowse_name\tgene_report_url\tdefline",
		"1\tprot1\tsubj1\tquery1\t2\t8\t10\t16\t1e-20\t98.5\t42\t1\t0\t77.25\t120\tseq1\ttx1\t18\tSp_test\thttps://example.test/gene\tdefline text",
	}, "\n")
	result, err := parseCachedBlastResultTSV(strings.NewReader(current), "job-current")
	if err != nil {
		t.Fatalf("parse current cache returned error: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0].SubjectID != "subj1" || result.Rows[0].TargetLength != 120 {
		t.Fatalf("unexpected current cache result: %#v", result.Rows)
	}

	legacy := strings.Join([]string{
		"hit\tprotein\tqseqid\tqstart\tqend\tevalue\tpident\talign_len\tbitscore",
		"2\tprot2\tquery2\t3\t9\t2e-10\t87.5\t24\t55.5",
	}, "\n")
	result, err = parseCachedBlastResultTSV(strings.NewReader(legacy), "job-legacy")
	if err != nil {
		t.Fatalf("parse legacy cache returned error: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0].SubjectID != "prot2" || result.Rows[0].Bitscore != 55.5 {
		t.Fatalf("unexpected legacy cache result: %#v", result.Rows)
	}
}

func TestLocalBlastJobIDsAreUnique(t *testing.T) {
	const total = 512
	ids := make(chan string, total)
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids <- newLocalBlastJobID("Sp:test?bad")
		}()
	}
	wg.Wait()
	close(ids)

	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate local job id: %s", id)
		}
		seen[id] = true
		if strings.ContainsAny(id, `<>:"/\|?*`) {
			t.Fatalf("job id contains unsafe filename characters: %q", id)
		}
	}
	if len(seen) != total {
		t.Fatalf("got %d ids, want %d", len(seen), total)
	}
}

func TestExistsBlastDBFilesRequiresReadyMarkerAndCompleteCore(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "lemna_prot_db")
	for _, ext := range []string{".pin", ".phr", ".psq"} {
		if err := os.WriteFile(prefix+ext, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if existsBlastDBFiles(prefix, "prot") {
		t.Fatal("DB without ready marker should not be treated as complete")
	}
	if err := os.WriteFile(blastDBReadyPath(prefix), []byte("ready"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !existsBlastDBFiles(prefix, "prot") {
		t.Fatal("complete protein DB with ready marker should be ready")
	}
	if err := os.Remove(prefix + ".psq"); err != nil {
		t.Fatal(err)
	}
	if existsBlastDBFiles(prefix, "prot") {
		t.Fatal("partial protein DB should not be treated as complete")
	}
}

func TestMakeBlastDBArgsForceNonLMDBDatabaseVersion(t *testing.T) {
	args := makeBlastDBArgs("input.fasta", "db-prefix", "prot")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-blastdb_version 4") {
		t.Fatalf("makeblastdb args should force v4 non-LMDB database, got %q", joined)
	}
	if strings.Contains(joined, "BLASTDB_LMDB_MAP_SIZE") {
		t.Fatalf("makeblastdb args should not rely on LMDB env tuning: %q", joined)
	}
}

func TestLocalBlastDBPrefixIncludesDatabaseVersion(t *testing.T) {
	prefix := localBlastDBPrefix(filepath.Join("cache", "release"), "prot")
	if !strings.HasSuffix(prefix, "lemna_prot_db_v4") {
		t.Fatalf("unexpected local BLAST DB prefix: %q", prefix)
	}
}

func TestLocalBlastQueryCachePathReusesSequenceFingerprint(t *testing.T) {
	dir := t.TempDir()
	first := localBlastQueryCachePath(dir, "AAA\nBBB")
	second := localBlastQueryCachePath(dir, "AAABBB")
	third := localBlastQueryCachePath(dir, "AAABBC")

	if first != second {
		t.Fatalf("equivalent sanitized sequences should reuse the same query cache path: %q != %q", first, second)
	}
	if first == third {
		t.Fatalf("different sequences should not reuse the same query cache path: %q", first)
	}
}

func TestParseBlastTabularBufferParsesRowsWithoutFilesystemRoundTrip(t *testing.T) {
	data := []byte("query\tSp9509d020g000340_T001\t99.0\t10\t0\t0\t1\t10\t3\t12\t1e-12\t42\n")
	rows, err := parseBlastTabularBuffer(data, map[string]fastaEntry{
		"Sp9509d020g000340_T001": {Defline: "test defline", Length: 123},
	})
	if err != nil {
		t.Fatalf("parseBlastTabularBuffer returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Protein != "Sp9509d020g000340_T001" || rows[0].TargetLength != 123 || rows[0].Defline != "test defline" {
		t.Fatalf("unexpected parsed row: %#v", rows[0])
	}
}

func TestSaveBlastResultToCacheWritesAtomicallyLoadableFile(t *testing.T) {
	dir := t.TempDir()
	jobID := newLocalBlastJobID("cache-test")
	want := model.BlastResult{
		JobID: jobID,
		Rows: []model.BlastResultRow{{
			Protein:         "prot1",
			SubjectID:       "subj1",
			QueryID:         "query1",
			QueryFrom:       1,
			QueryTo:         9,
			TargetFrom:      3,
			TargetTo:        11,
			EValue:          "1e-9",
			PercentIdentity: 99,
			AlignLength:     10,
			Bitscore:        42,
		}},
	}
	if err := saveBlastResultToCache(context.Background(), dir, jobID, want); err != nil {
		t.Fatalf("saveBlastResultToCache returned error: %v", err)
	}
	data, err := os.Open(filepath.Join(dir, jobID+".tsv"))
	if err != nil {
		t.Fatalf("open cached result: %v", err)
	}
	defer data.Close()
	got, err := parseCachedBlastResultTSV(data, jobID)
	if err != nil {
		t.Fatalf("parse cached result: %v", err)
	}
	if len(got.Rows) != 1 || got.Rows[0].SubjectID != "subj1" || got.Rows[0].QueryTo != 9 {
		t.Fatalf("unexpected cached rows: %#v", got.Rows)
	}
	matches, err := filepath.Glob(filepath.Join(dir, jobID+".tsv.*.part"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("left stale cache temp files: %v", matches)
	}
}

func TestLoadBlastResultFromCacheTreatsWalkEOFAsNonFatal(t *testing.T) {
	cacheDir, err := appfs.CacheDir("lemna", "localblast", "Sp_test", "release")
	if err != nil {
		t.Fatal(err)
	}
	jobID := newLocalBlastJobID("walk-eof")
	result := model.BlastResult{
		JobID: jobID,
		Rows:  []model.BlastResultRow{{Protein: "prot1", SubjectID: "subj1", QueryID: "query1"}},
	}
	if err := saveBlastResultToCache(context.Background(), cacheDir, jobID, result); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(filepath.Join(cacheDir, jobID+".tsv"))
	})
	client := NewClient(nil)
	got, err := client.loadBlastResultFromCache(jobID)
	if err != nil {
		t.Fatalf("loadBlastResultFromCache returned error: %v", err)
	}
	if len(got.Rows) != 1 || got.Rows[0].SubjectID != "subj1" {
		t.Fatalf("unexpected loaded result: %#v", got.Rows)
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

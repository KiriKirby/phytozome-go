package tair

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func stringResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestFetchSpeciesCandidatesReturnsTAIRVersions(t *testing.T) {
	c := NewClient(nil)
	candidates, err := c.FetchSpeciesCandidates(context.Background())
	if err != nil {
		t.Fatalf("FetchSpeciesCandidates: %v", err)
	}
	if len(candidates) < 2 {
		t.Fatalf("expected TAIR versions, got %d", len(candidates))
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		seen[candidate.JBrowseName] = true
	}
	if !seen["Araport11"] || !seen["TAIR10"] || !seen["TAIR11"] || !seen["TAIR12"] || !seen["TAIR9"] || !seen["TAIR8"] || !seen["TAIR7"] || !seen["TAIR6"] {
		t.Fatalf("missing expected versions: %#v", seen)
	}
}

func TestDefaultReleasesCarryVersionSpecificAssets(t *testing.T) {
	releases := defaultReleases()
	byName := map[string]releaseInfo{}
	for _, rel := range releases {
		byName[rel.Name] = rel
	}
	if byName["TAIR12"].Source != tairReleaseSourceENA || byName["TAIR12"].ENAStudyAccession != tairENAStudyPRJEB100887 {
		t.Fatalf("TAIR12 should use ENA PRJEB100887 source: %#v", byName["TAIR12"])
	}
	if byName["TAIR12"].GFFURL != "" || byName["TAIR12"].ProteinURL != "" || byName["TAIR12"].NucleotideURL != "" {
		t.Fatalf("TAIR12 should not use obsolete TAIR download assets: %#v", byName["TAIR12"])
	}
	if byName["TAIR10"].ProteinURL == "" || !strings.Contains(byName["TAIR10"].ProteinURL, "TAIR10_pep_20110103_representative_gene_model_updated") {
		t.Fatalf("TAIR10 protein url = %q", byName["TAIR10"].ProteinURL)
	}
	if !strings.Contains(byName["TAIR10"].ProteinURL, "Sequences/blast_datasets/TAIR10_blastsets") {
		t.Fatalf("TAIR10 protein url should use the concrete blastsets path, got %q", byName["TAIR10"].ProteinURL)
	}
	if byName["TAIR10"].DescriptionURL == "" || !strings.Contains(byName["TAIR10"].DescriptionURL, "TAIR10_functional_descriptions_20130831.txt") {
		t.Fatalf("TAIR10 description url = %q", byName["TAIR10"].DescriptionURL)
	}
	if byName["TAIR9"].ProteinURL == "" || !strings.Contains(byName["TAIR9"].ProteinURL, "TAIR9_pep_20090619") {
		t.Fatalf("TAIR9 protein url = %q", byName["TAIR9"].ProteinURL)
	}
	if !strings.Contains(byName["TAIR9"].GFFURL, "Maps/gbrowse_data/TAIR9") || !strings.Contains(byName["TAIR9"].ProteinURL, "Sequences/blast_datasets/TAIR9_blastsets") {
		t.Fatalf("TAIR9 urls should use concrete FTP mirror paths: gff=%q protein=%q", byName["TAIR9"].GFFURL, byName["TAIR9"].ProteinURL)
	}
	if byName["TAIR8"].ProteinURL == "" || !strings.Contains(byName["TAIR8"].ProteinURL, "TAIR8_pep_20080412") {
		t.Fatalf("TAIR8 protein url = %q", byName["TAIR8"].ProteinURL)
	}
	if !strings.Contains(byName["TAIR8"].GFFURL, "Maps/gbrowse_data/TAIR8") || !strings.Contains(byName["TAIR8"].ProteinURL, "Sequences/blast_datasets/TAIR8_blastsets") {
		t.Fatalf("TAIR8 urls should use concrete FTP mirror paths: gff=%q protein=%q", byName["TAIR8"].GFFURL, byName["TAIR8"].ProteinURL)
	}
	if byName["TAIR7"].ProteinURL == "" || !strings.Contains(byName["TAIR7"].ProteinURL, "TAIR7_pep_20070425") {
		t.Fatalf("TAIR7 protein url = %q", byName["TAIR7"].ProteinURL)
	}
	if !strings.Contains(byName["TAIR7"].ProteinURL, "Sequences/blast_datasets/TAIR7_blastsets") {
		t.Fatalf("TAIR7 protein url should use the concrete blastsets path, got %q", byName["TAIR7"].ProteinURL)
	}
	if byName["TAIR9"].NucleotideURL == "" || !strings.Contains(byName["TAIR9"].NucleotideURL, "TAIR9_chr_all.fas") {
		t.Fatalf("TAIR9 nucleotide url = %q", byName["TAIR9"].NucleotideURL)
	}
	if !strings.Contains(byName["TAIR11"].SourceURL, "zenodo.org/records/17371665") || !strings.Contains(byName["TAIR11"].GFFURL, "Araport11_GFF3_genes_transposons.20241001.gff.gz") {
		t.Fatalf("TAIR11 should use Zenodo record 17371665 annotation assets: %#v", byName["TAIR11"])
	}
	if !strings.Contains(byName["TAIR11"].GeneAliasURL, "gene_aliases_20241001.txt.gz") {
		t.Fatalf("TAIR11 should use Zenodo gene alias asset: %#v", byName["TAIR11"])
	}
	if byName["TAIR11"].ProteinURL != "" || byName["TAIR11"].NucleotideURL != "" {
		t.Fatalf("TAIR11 Zenodo record should not invent FASTA assets: %#v", byName["TAIR11"])
	}
}

func TestTAIRDownloadCandidatesPreferOfficialFTPMirror(t *testing.T) {
	raw := downloadURL("Sequences/blast_datasets/TAIR10_blastsets/TAIR10_pep_20110103_representative_gene_model_updated")
	candidates := tairDownloadCandidates(raw)
	if len(candidates) < 2 {
		t.Fatalf("expected FTP mirror plus API URL, got %#v", candidates)
	}
	if want := tairFTPBase + "Sequences/blast_datasets/TAIR10_blastsets/TAIR10_pep_20110103_representative_gene_model_updated"; candidates[0] != want {
		t.Fatalf("first candidate = %q, want %q", candidates[0], want)
	}
	if candidates[1] != raw {
		t.Fatalf("second candidate = %q, want original API URL %q", candidates[1], raw)
	}

	zenodo := "https://zenodo.org/api/records/17371665/files/Araport11_GFF3_genes_transposons.20241001.gff.gz/content"
	if got := tairDownloadCandidates(zenodo); len(got) != 1 || got[0] != zenodo {
		t.Fatalf("Zenodo TAIR11 assets should not try historical FTP mirror candidates, got %#v", got)
	}
}

func TestValidateDownloadedTAIRAssetRejectsHTML(t *testing.T) {
	dir := t.TempDir()
	tmp := filepath.Join(dir, "TAIR12_1Feb26.gff3.zip.part")
	if err := os.WriteFile(tmp, []byte("<!doctype html><html><body><div id=\"app\"></div></body></html>"), 0o600); err != nil {
		t.Fatalf("WriteFile html: %v", err)
	}
	err := validateDownloadedTAIRAsset(tmp, filepath.Join(dir, "TAIR12_1Feb26.gff3.zip"))
	if err == nil {
		t.Fatal("expected HTML asset validation error")
	}
	if !strings.Contains(err.Error(), "HTML/error page") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseENACodingTSVAndKeywordRow(t *testing.T) {
	tsv := strings.Join([]string{
		"accession\tdescription\tgene\tproduct\tlocus_tag\tprotein_id\tsequence_version\tstudy_accession",
		"CAO7037755\tArabidopsis thaliana phenylalanine ammonia-lyase[PHE ammonia lyase 1]\t\tphenylalanine ammonia-lyase[PHE ammonia lyase 1]\tTAIR12_TAIR12_AT2G37040\tCAO7037755.1\t1\tPRJEB100887",
		"",
	}, "\n")
	rows, err := parseENACodingTSV(strings.NewReader(tsv))
	if err != nil {
		t.Fatalf("parseENACodingTSV: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	rel := releaseInfo{Name: "TAIR12", Label: "TAIR12", Source: tairReleaseSourceENA, SourceURL: "https://www.ebi.ac.uk/ena/browser/view/PRJEB100887", ENAStudyAccession: tairENAStudyPRJEB100887}
	row := keywordRowFromENACoding(model.SpeciesCandidate{JBrowseName: "TAIR12", GenomeLabel: "TAIR12"}, rel, rows[0], "AT2G37040")
	if row.GeneIdentifier != "AT2G37040" || row.SequenceID != "CAO7037755" || row.ProteinID != "CAO7037755.1" {
		t.Fatalf("unexpected ENA keyword row identifiers: %#v", row)
	}
	if row.ExtraColumns["ena_study_accession"] != tairENAStudyPRJEB100887 || row.ExtraColumns["ena_accession"] != "CAO7037755" {
		t.Fatalf("unexpected ENA metadata: %#v", row.ExtraColumns)
	}
	if row.GeneReportURL != "" {
		t.Fatalf("GeneReportURL = %q, want empty because ENA coding rows do not carry a source page URL", row.GeneReportURL)
	}
	if row.LabelName == "" || !strings.Contains(row.Description, "phenylalanine") {
		t.Fatalf("expected label/description from ENA product: %#v", row)
	}
}

func TestParseGeneAliasTable(t *testing.T) {
	tsv := strings.Join([]string{
		"locus_name\tsymbol\tfull_name",
		"AT1G01010\tANAC001\tNAC domain containing protein 1",
		"AT1G01010\tNAC001\tNAC domain containing protein 1",
		"AT2G30490\tC4H\tcinnamate-4-hydroxylase",
		"AT2G30490\tREF3\tREDUCED EPRDERMAL FLUORESCENCE 3",
		"",
	}, "\n")
	index, err := parseGeneAliasTable(strings.NewReader(tsv))
	if err != nil {
		t.Fatalf("parseGeneAliasTable: %v", err)
	}
	first := index["AT1G01010"]
	if first.LocusName != "AT1G01010" || !strings.Contains(strings.Join(first.Symbols, "; "), "ANAC001") || !strings.Contains(strings.Join(first.Symbols, "; "), "NAC001") {
		t.Fatalf("unexpected first alias entry: %#v", first)
	}
	second := index["AT2G30490"]
	if !strings.Contains(strings.Join(second.Symbols, "; "), "C4H") || !strings.Contains(strings.Join(second.Symbols, "; "), "REF3") {
		t.Fatalf("unexpected second alias entry: %#v", second)
	}
}

func TestFetchENACodingRowsUsesSingleflightAndMemoryCache(t *testing.T) {
	var requests int32
	study := fmt.Sprintf("PRJEB100887_TEST_CACHE_%d", time.Now().UnixNano())
	release := releaseInfo{Name: "TAIR12-test", Source: tairReleaseSourceENA, ENAStudyAccession: study}
	client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "www.ebi.ac.uk" || req.URL.Path != "/ena/portal/api/search" {
			t.Fatalf("unexpected request URL: %s", req.URL.String())
		}
		atomic.AddInt32(&requests, 1)
		if got := req.URL.Query().Get("result"); got != "coding" {
			t.Fatalf("result query = %q, want coding", got)
		}
		query := req.URL.Query().Get("query")
		if !strings.Contains(query, `study_accession="`+study+`"`) || !strings.Contains(query, `locus_tag="TAIR12_TAIR12_AT2G37040"`) {
			t.Fatalf("unexpected ENA exact query: %q", query)
		}
		body := strings.Join([]string{
			"accession\tdescription\tgene\tproduct\tlocus_tag\tprotein_id\tsequence_version\tstudy_accession",
			"CAO9999001\tcache test\t\tcache protein\tTAIR12_TAIR12_AT2G37040\tCAO9999001.1\t1\t" + study,
			"",
		}, "\n")
		return stringResponse(http.StatusOK, body), nil
	})})

	const goroutines = 12
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := client.fetchENACodingRowsByLocus(context.Background(), release, "AT2G37040")
			if err != nil {
				errs <- err
				return
			}
			if len(rows) != 1 || rows[0].Accession != "CAO9999001" {
				errs <- io.ErrUnexpectedEOF
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("fetchENACodingRowsByLocus concurrent: %v", err)
		}
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("ENA coding requests = %d, want singleflight 1", got)
	}

	rows, err := client.fetchENACodingRowsByLocus(context.Background(), release, "AT2G37040")
	if err != nil {
		t.Fatalf("fetchENACodingRowsByLocus cached: %v", err)
	}
	if len(rows) != 1 || rows[0].Accession != "CAO9999001" {
		t.Fatalf("unexpected cached rows: %#v", rows)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("ENA coding requests after memory cache = %d, want 1", got)
	}
}

func TestFetchENAFastaEntryUsesSingleflightAndMemoryCache(t *testing.T) {
	var requests int32
	accession := fmt.Sprintf("CAO_TEST_%d_%d", time.Now().UnixNano(), os.Getpid())
	client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "www.ebi.ac.uk" || req.URL.Path != "/ena/browser/api/fasta/"+accession {
			t.Fatalf("unexpected request URL: %s", req.URL.String())
		}
		atomic.AddInt32(&requests, 1)
		return stringResponse(http.StatusOK, ">"+accession+".1 cache fasta\nMPEPTIDE\n"), nil
	})})

	const goroutines = 12
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entry, err := client.fetchENAFastaEntry(context.Background(), accession)
			if err != nil {
				errs <- err
				return
			}
			if entry.Sequence != "MPEPTIDE" {
				errs <- io.ErrUnexpectedEOF
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("fetchENAFastaEntry concurrent: %v", err)
		}
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("ENA FASTA requests = %d, want singleflight 1", got)
	}

	entry, err := client.fetchENAFastaEntry(context.Background(), accession)
	if err != nil {
		t.Fatalf("fetchENAFastaEntry cached: %v", err)
	}
	if entry.Sequence != "MPEPTIDE" {
		t.Fatalf("unexpected cached ENA FASTA: %#v", entry)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("ENA FASTA requests after memory cache = %d, want 1", got)
	}
}

func TestFindRowForENASourceDoesNotUseLiveTAIRFallback(t *testing.T) {
	var requestPaths []string
	study := fmt.Sprintf("PRJEB100887_TEST_FIND_%d", time.Now().UnixNano())
	client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestPaths = append(requestPaths, req.URL.Path)
		if req.URL.Host != "www.ebi.ac.uk" || req.URL.Path != "/ena/portal/api/search" {
			t.Fatalf("ENA findRow should not call live TAIR fallback, got %s", req.URL.String())
		}
		body := strings.Join([]string{
			"accession\tdescription\tgene\tproduct\tlocus_tag\tprotein_id\tsequence_version\tstudy_accession",
			"CAO9999003\tfind row\t\tfind row protein\tTAIR12_TAIR12_AT2G37040\tCAO9999003.1\t1\t" + study,
			"",
		}, "\n")
		return stringResponse(http.StatusOK, body), nil
	})})
	client.releases["tair12-strict-find"] = releaseInfo{
		Name:              "TAIR12-strict-find",
		Label:             "TAIR12-strict-find",
		Source:            tairReleaseSourceENA,
		ENAStudyAccession: study,
	}
	version := model.SpeciesCandidate{ProteomeID: 991212, JBrowseName: "TAIR12-strict-find", GenomeLabel: "TAIR12-strict-find"}
	row, err := client.findRow(context.Background(), version, "AT2G37040")
	if err != nil {
		t.Fatalf("findRow ENA: %v", err)
	}
	if row.GeneIdentifier != "AT2G37040" || row.SequenceID != "CAO9999003" {
		t.Fatalf("unexpected ENA row: %#v", row)
	}
	if len(requestPaths) != 1 || requestPaths[0] != "/ena/portal/api/search" {
		t.Fatalf("unexpected request paths: %#v", requestPaths)
	}
}

func TestFindRowForGFFReleaseDoesNotUseLiveTAIRFallback(t *testing.T) {
	dir := t.TempDir()
	gffPath := filepath.Join(dir, "TAIR10.gff3")
	gff := strings.Join([]string{
		"##gff-version 3",
		"Chr1\tTAIR10\tmRNA\t3631\t5899\t.\t+\t.\tID=AT1G01010.1;Parent=AT1G01010;Name=AT1G01010.1;Note=NAC domain containing protein 1",
		"",
	}, "\n")
	if err := os.WriteFile(gffPath, []byte(gff), 0o600); err != nil {
		t.Fatalf("WriteFile gff: %v", err)
	}

	client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("GFF-backed TAIR findRow must not call live fallback, got %s", req.URL.String())
		return nil, nil
	})})
	rel := releaseInfo{
		Name:   "TAIR10-strict-find",
		Label:  "TAIR10-strict-find",
		GFFURL: gffPath,
	}
	client.releases[strings.ToLower(rel.Name)] = rel
	version := model.SpeciesCandidate{ProteomeID: 991010, JBrowseName: rel.Name, GenomeLabel: rel.Label}

	if _, err := client.findRow(context.Background(), version, "AT9G99999"); err == nil {
		t.Fatal("expected local release index miss")
	} else if !strings.Contains(err.Error(), "local release index") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLocalBlastDBRejectsENABulkFallback(t *testing.T) {
	rel := releaseInfo{Name: "TAIR12", Source: tairReleaseSourceENA, ENAStudyAccession: tairENAStudyPRJEB100887}
	_, _, err := localBlastDB(rel, "blastp")
	if err == nil {
		t.Fatal("expected ENA bulk BLAST error")
	}
	if !strings.Contains(err.Error(), "does not expose a single official bulk FASTA") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFilterCandidatesForModeHidesUnavailableReleases(t *testing.T) {
	c := NewClient(nil)
	candidates, err := c.FetchSpeciesCandidates(context.Background())
	if err != nil {
		t.Fatalf("FetchSpeciesCandidates: %v", err)
	}
	blastCandidates := c.FilterCandidatesForMode(candidates, "blast")
	keywordCandidates := c.FilterCandidatesForMode(candidates, "keyword")
	familyCandidates := c.FilterCandidatesForMode(candidates, "family")

	seenBlast := map[string]bool{}
	for _, candidate := range blastCandidates {
		seenBlast[candidate.JBrowseName] = true
	}
	if seenBlast["TAIR11"] || seenBlast["TAIR12"] {
		t.Fatalf("blast list should hide releases without bulk FASTA assets, got %#v", seenBlast)
	}
	if !seenBlast["TAIR10"] || !seenBlast["Araport11"] || !seenBlast["TAIR9"] || !seenBlast["TAIR8"] || !seenBlast["TAIR7"] || !seenBlast["TAIR6"] {
		t.Fatalf("blast list missing usable releases: %#v", seenBlast)
	}

	seenKeyword := map[string]bool{}
	for _, candidate := range keywordCandidates {
		seenKeyword[candidate.JBrowseName] = true
	}
	if !seenKeyword["TAIR11"] || !seenKeyword["TAIR12"] || !seenKeyword["TAIR7"] || !seenKeyword["TAIR10"] {
		t.Fatalf("keyword list missing GFF-backed releases: %#v", seenKeyword)
	}

	seenFamily := map[string]bool{}
	for _, candidate := range familyCandidates {
		seenFamily[candidate.JBrowseName] = true
	}
	if seenFamily["TAIR12"] || !seenFamily["TAIR11"] {
		t.Fatalf("family list unexpected: %#v", seenFamily)
	}
}

func TestLiveTAIRKeywordSearch(t *testing.T) {
	if os.Getenv("PHGO_TAIR_LIVE") != "1" {
		t.Skip("set PHGO_TAIR_LIVE=1 to run live TAIR keyword search")
	}
	c := NewClient(nil)
	candidates, err := c.FetchSpeciesCandidates(context.Background())
	if err != nil {
		t.Fatalf("FetchSpeciesCandidates: %v", err)
	}
	var tair10 model.SpeciesCandidate
	for _, candidate := range candidates {
		if candidate.JBrowseName == "TAIR10" {
			tair10 = candidate
			break
		}
	}
	if tair10.JBrowseName == "" {
		t.Fatal("TAIR10 candidate not found")
	}
	rows, err := c.SearchKeywordRows(context.Background(), tair10, "AT1G01010")
	if err != nil {
		t.Fatalf("SearchKeywordRows: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected AT1G01010 rows")
	}
	if _, err := c.FetchProteinSequence(context.Background(), tair10.ProteomeID, rows[0].SequenceID); err != nil {
		t.Fatalf("FetchProteinSequence: %v", err)
	}
}

func TestLiveTAIR12KeywordAndProteinSequence(t *testing.T) {
	if os.Getenv("PHGO_TAIR_LIVE") != "1" {
		t.Skip("set PHGO_TAIR_LIVE=1 to run live TAIR12 keyword search")
	}
	c := NewClient(nil)
	candidates, err := c.FetchSpeciesCandidates(context.Background())
	if err != nil {
		t.Fatalf("FetchSpeciesCandidates: %v", err)
	}
	var tair12 model.SpeciesCandidate
	for _, candidate := range candidates {
		if candidate.JBrowseName == "TAIR12" {
			tair12 = candidate
			break
		}
	}
	if tair12.JBrowseName == "" {
		t.Fatal("TAIR12 candidate not found")
	}
	rows, err := c.SearchKeywordRows(context.Background(), tair12, "AT1G01010")
	if err != nil {
		t.Fatalf("SearchKeywordRows TAIR12: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected TAIR12 AT1G01010 rows")
	}
	var matched model.KeywordResultRow
	for _, row := range rows {
		if strings.EqualFold(strings.TrimSpace(row.GeneIdentifier), "AT1G01010") || strings.EqualFold(strings.TrimSpace(stripTranscriptSuffix(row.TranscriptID)), "AT1G01010") {
			matched = row
			break
		}
	}
	if strings.TrimSpace(matched.SequenceID) == "" {
		matched = rows[0]
	}
	if strings.TrimSpace(matched.SequenceID) == "" {
		t.Fatalf("TAIR12 keyword row has no sequence id: %#v", rows[0])
	}
	seq, err := c.FetchProteinSequence(context.Background(), tair12.ProteomeID, matched.SequenceID)
	if err != nil {
		t.Fatalf("FetchProteinSequence TAIR12: %v", err)
	}
	if strings.TrimSpace(seq.Sequence) == "" {
		t.Fatal("expected TAIR12 protein sequence")
	}
}

func TestLiveTAIR12FamilyCandidatesAndRows(t *testing.T) {
	if os.Getenv("PHGO_TAIR_LIVE") != "1" {
		t.Skip("set PHGO_TAIR_LIVE=1 to run live TAIR12 family search")
	}
	c := NewClient(nil)
	candidates, err := c.FetchSpeciesCandidates(context.Background())
	if err != nil {
		t.Fatalf("FetchSpeciesCandidates: %v", err)
	}
	var tair12 model.SpeciesCandidate
	for _, candidate := range candidates {
		if candidate.JBrowseName == "TAIR12" {
			tair12 = candidate
			break
		}
	}
	if tair12.JBrowseName == "" {
		t.Fatal("TAIR12 candidate not found")
	}
	families, err := c.FetchFamilyCandidates(context.Background(), tair12)
	if err != nil {
		t.Fatalf("FetchFamilyCandidates TAIR12: %v", err)
	}
	if len(families) == 0 {
		t.Fatal("expected TAIR12 family candidates")
	}
	var chosen model.SpeciesCandidate
	for _, fam := range families {
		if !fam.HasChildren && strings.TrimSpace(fam.JBrowseName) != "" {
			chosen = fam
			break
		}
	}
	if chosen.JBrowseName == "" {
		chosen = families[0]
	}
	rows, err := c.SearchFamilyKeywordRows(context.Background(), tair12, chosen.JBrowseName)
	if err != nil {
		t.Fatalf("SearchFamilyKeywordRows TAIR12: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected family rows for %q", chosen.JBrowseName)
	}
	first := rows[0]
	if strings.TrimSpace(first.SequenceID) == "" {
		t.Fatalf("family row missing sequence id: %#v", first)
	}
	seq, err := c.FetchProteinSequence(context.Background(), tair12.ProteomeID, first.SequenceID)
	if err != nil {
		t.Fatalf("FetchProteinSequence for TAIR12 family row: %v", err)
	}
	if strings.TrimSpace(seq.Sequence) == "" {
		t.Fatal("expected TAIR12 family protein sequence")
	}
}

func TestParseTAIRGFFAndFASTA(t *testing.T) {
	gff, ok := parseGFF3Line("Chr1\tTAIR10\tmRNA\t3631\t5899\t.\t+\t.\tID=AT1G01010.1;Parent=AT1G01010;Name=AT1G01010.1;Note=protein_coding_gene")
	if !ok {
		t.Fatal("expected GFF row")
	}
	row := buildKeywordRowFromGFF(defaultVersionForTest(), defaultReleases()[1], gff)
	if row.GeneIdentifier != "AT1G01010" || row.TranscriptID != "AT1G01010.1" {
		t.Fatalf("unexpected row identifiers: gene=%q transcript=%q", row.GeneIdentifier, row.TranscriptID)
	}

	entries, err := parseFASTA(strings.NewReader(">AT1G01010.1 | Symbols: NAC001 | NAC domain containing protein 1 | chr1:1-10\nMSTNPKPQR\n"))
	if err != nil {
		t.Fatalf("parseFASTA: %v", err)
	}
	entry, ok := lookupProteinEntry(entries, "AT1G01010.1")
	if !ok {
		t.Fatal("expected protein entry lookup")
	}
	enrichRowWithProtein(&row, entry)
	if row.Symbols != "NAC001" || !strings.Contains(row.Description, "NAC domain") {
		t.Fatalf("row not enriched: symbols=%q description=%q", row.Symbols, row.Description)
	}
}

func TestBuildIndexMergesTAIR12GeneAndTranscriptRows(t *testing.T) {
	dir := t.TempDir()
	gffPath := filepath.Join(dir, "TAIR12.gff3")
	fastaPath := filepath.Join(dir, "TAIR12_pep.fasta")
	gff := strings.Join([]string{
		"##gff-version 3",
		"Chr1\tGnomon\tgene\t7395\t9666\t.\t+\t.\tID=AT1G01010;locus_tag=TAIR12_AT1G01010;locus_biotype=protein_coding;gene=NAC001;description=NAC domain containing protein 1;gene_synonym=\"ANAC001, NTL10\";Note=NAC domain containing protein 1;Name=AT1G01010;Dbxref=TAIR:AT1G01010_TAIR12;",
		"Chr1\tGnomon\tmRNA\t7395\t9666\t.\t+\t.\tID=AT1G01010.1;Parent=AT1G01010;Name=AT1G01010.1;product=putative DNA-binding transcription factor[NAC domain containing protein 1];",
		"",
	}, "\n")
	if err := os.WriteFile(gffPath, []byte(gff), 0o600); err != nil {
		t.Fatalf("WriteFile gff: %v", err)
	}
	if err := os.WriteFile(fastaPath, []byte(">AT1G01010.1 | Chr1:7395-9666\nMEDQVG\n"), 0o600); err != nil {
		t.Fatalf("WriteFile fasta: %v", err)
	}
	client := NewClient(nil)
	rel := releaseInfo{
		Name:       "TAIR12-test",
		Label:      "TAIR12-test",
		GFFURL:     gffPath,
		ProteinURL: fastaPath,
	}
	version := model.SpeciesCandidate{ProteomeID: 370201, JBrowseName: rel.Name, GenomeLabel: rel.Label}
	idx, err := client.buildIndex(context.Background(), rel, version)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if len(idx.Rows) != 1 {
		t.Fatalf("expected merged single row, got %d: %#v", len(idx.Rows), idx.Rows)
	}
	row := idx.Rows[0]
	if row.GeneIdentifier != "AT1G01010" || row.TranscriptID != "AT1G01010.1" || row.SequenceID != "AT1G01010.1" {
		t.Fatalf("unexpected merged identifiers: %#v", row)
	}
	if row.LabelName != "NAC001" || !strings.Contains(row.Synonyms, "ANAC001") || !strings.Contains(row.Aliases, "TAIR12_AT1G01010") {
		t.Fatalf("gene-level metadata was not merged: label=%q aliases=%q synonyms=%q", row.LabelName, row.Aliases, row.Synonyms)
	}
	if !strings.Contains(row.Description, "NAC domain") {
		t.Fatalf("gene/transcript metadata was not merged: desc=%q extras=%#v", row.Description, row.ExtraColumns)
	}
}

func TestLiveTAIR12KeywordOnly(t *testing.T) {
	if os.Getenv("PHGO_TAIR_LIVE") != "1" {
		t.Skip("set PHGO_TAIR_LIVE=1 to run live TAIR12 keyword search")
	}
	c := NewClient(nil)
	candidates, err := c.FetchSpeciesCandidates(context.Background())
	if err != nil {
		t.Fatalf("FetchSpeciesCandidates: %v", err)
	}
	var tair12 model.SpeciesCandidate
	for _, candidate := range candidates {
		if candidate.JBrowseName == "TAIR12" {
			tair12 = candidate
			break
		}
	}
	if tair12.JBrowseName == "" {
		t.Fatal("TAIR12 candidate not found")
	}
	rows, err := c.SearchKeywordRows(context.Background(), tair12, "AT1G01010")
	if err != nil {
		t.Fatalf("SearchKeywordRows TAIR12: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected TAIR12 AT1G01010 rows")
	}
}

func TestBuildIndexDoesNotRequireProteinFASTAAvailability(t *testing.T) {
	dir := t.TempDir()
	gffPath := filepath.Join(dir, "TAIR12.gff3")
	gff := strings.Join([]string{
		"##gff-version 3",
		"Chr1\tGnomon\tgene\t7395\t9666\t.\t+\t.\tID=AT1G01010;locus_tag=TAIR12_AT1G01010;locus_biotype=protein_coding;gene=NAC001;description=NAC domain containing protein 1;gene_synonym=\"ANAC001, NTL10\";Note=NAC domain containing protein 1;Name=AT1G01010;Dbxref=TAIR:AT1G01010_TAIR12;",
		"Chr1\tGnomon\tmRNA\t7395\t9666\t.\t+\t.\tID=AT1G01010.1;Parent=AT1G01010;Name=AT1G01010.1;product=putative DNA-binding transcription factor[NAC domain containing protein 1];",
		"",
	}, "\n")
	if err := os.WriteFile(gffPath, []byte(gff), 0o600); err != nil {
		t.Fatalf("WriteFile gff: %v", err)
	}

	stallServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer stallServer.Close()

	client := NewClient(&http.Client{})
	rel := releaseInfo{
		Name:       "TAIR12-no-protein",
		Label:      "TAIR12-no-protein",
		GFFURL:     gffPath,
		ProteinURL: stallServer.URL + "/TAIR12_pep.fasta",
	}
	version := model.SpeciesCandidate{ProteomeID: 370201, JBrowseName: rel.Name, GenomeLabel: rel.Label}

	idx, err := client.buildIndex(context.Background(), rel, version)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if len(idx.Rows) != 1 {
		t.Fatalf("expected single row without protein FASTA, got %d", len(idx.Rows))
	}
	row := idx.Rows[0]
	if row.GeneIdentifier != "AT1G01010" || row.TranscriptID != "AT1G01010.1" {
		t.Fatalf("unexpected row identifiers: %#v", row)
	}
	if row.LabelName != "NAC001" || !strings.Contains(row.Description, "NAC domain") {
		t.Fatalf("expected GFF metadata to remain searchable without protein FASTA: %#v", row)
	}
}

func TestBuildIndexMergesTAIR11GeneAliasAsset(t *testing.T) {
	dir := t.TempDir()
	gffPath := filepath.Join(dir, "TAIR11.gff3")
	descPath := filepath.Join(dir, "TAIR11_desc.txt")
	aliasPath := filepath.Join(dir, "TAIR11_alias.tsv")
	gff := strings.Join([]string{
		"##gff-version 3",
		"Chr2\tAraport11\tgene\t12993625\t12996012\t.\t-\t.\tID=AT2G30490;Name=AT2G30490;Note=cinnamate-4-hydroxylase;Dbxref=TAIR:2064402;locus_type=protein_coding",
		"Chr2\tAraport11\tmRNA\t12993625\t12996012\t.\t-\t.\tID=AT2G30490.1;Name=AT2G30490.1;Parent=AT2G30490;Dbxref=TAIR:2064401,UniProt:P92994",
		"",
	}, "\n")
	desc := strings.Join([]string{
		"name\tgene_model_type\tshort_description\tCurator_summary\tComputational_description",
		"AT2G30490.1\tprotein_coding\tcinnamate-4-hydroxylase\tEncodes a cinnamate-4-hydroxylase.\tcinnamate-4-hydroxylase;(source:Araport11)",
		"",
	}, "\n")
	alias := strings.Join([]string{
		"locus_name\tsymbol\tfull_name",
		"AT2G30490\tATC4H\tCINNAMATE 4-HYDROXYLASE",
		"AT2G30490\tC4H\tcinnamate-4-hydroxylase",
		"AT2G30490\tCYP73A5\tNULL",
		"AT2G30490\tREF3\tREDUCED EPRDERMAL FLUORESCENCE 3",
		"",
	}, "\n")
	if err := os.WriteFile(gffPath, []byte(gff), 0o600); err != nil {
		t.Fatalf("WriteFile gff: %v", err)
	}
	if err := os.WriteFile(descPath, []byte(desc), 0o600); err != nil {
		t.Fatalf("WriteFile desc: %v", err)
	}
	if err := os.WriteFile(aliasPath, []byte(alias), 0o600); err != nil {
		t.Fatalf("WriteFile alias: %v", err)
	}

	client := NewClient(nil)
	rel := releaseInfo{
		Name:           "TAIR11-local",
		Label:          "TAIR11-local",
		GFFURL:         gffPath,
		DescriptionURL: descPath,
		GeneAliasURL:   aliasPath,
	}
	version := model.SpeciesCandidate{ProteomeID: 371111, JBrowseName: rel.Name, GenomeLabel: rel.Label}
	idx, err := client.buildIndex(context.Background(), rel, version)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if len(idx.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(idx.Rows))
	}
	row := idx.Rows[0]
	if row.LabelName != "C4H" {
		t.Fatalf("LabelName = %q, want C4H", row.LabelName)
	}
	if !strings.Contains(row.Symbols, "REF3") || !strings.Contains(row.Symbols, "CYP73A5") {
		t.Fatalf("Symbols = %q, want merged TAIR11 alias symbols", row.Symbols)
	}
	if !strings.Contains(row.Aliases, "CINNAMATE 4-HYDROXYLASE") {
		t.Fatalf("Aliases = %q, want gene alias full name", row.Aliases)
	}
	if row.ExtraColumns["tair_gene_alias_symbols"] == "" {
		t.Fatalf("expected alias metadata in extra columns: %#v", row.ExtraColumns)
	}
}

func TestFamilyHelpers(t *testing.T) {
	name := "ABC transporter subfamily B protein"
	if got := familyNameFromDescription(name); got == "" {
		t.Fatal("expected family name")
	}
	short := familyShortName(name)
	if short == "" {
		t.Fatal("expected short family name")
	}
	parentName, parentKey := familyParentName(name)
	if parentName == "" {
		t.Fatalf("expected parent family metadata, got %q %q", parentName, parentKey)
	}
}

func TestParseFamilyBrowseCandidates(t *testing.T) {
	html := `
	<tr>
	  <td><a href="/browse/gene_family/p450">Cytochrome P450</a></td>
	  <td>69 families<br>256 members</td>
	</tr>
	<tr>
	  <td><a href="/browse/gene_family/CAMTA">CAMTA Transcription Factor Family</a></td>
	  <td>1 family<br>6 members</td>
	</tr>`
	candidates := parseFamilyBrowseCandidates(html)
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].Key == "" || candidates[0].ShortName == "" {
		t.Fatalf("expected key/short name, got %#v", candidates[0])
	}
}

func TestFilterFamilyCandidatesUsesRankedFuzzyMatching(t *testing.T) {
	candidates := []model.SpeciesCandidate{
		{GenomeLabel: "Cytochrome P450", JBrowseName: "Cytochrome P450 family", SearchAlias: "P450 CYP family", GroupKey: "p450"},
		{GenomeLabel: "ABC transporter", JBrowseName: "ABC transporter family", SearchAlias: "ABC family", GroupKey: "abc"},
		{GenomeLabel: "CAMTA", JBrowseName: "CAMTA transcription factor family", SearchAlias: "camta tf", GroupKey: "camta"},
	}
	filtered := filterFamilyCandidates(candidates, "p450")
	if len(filtered) == 0 {
		t.Fatal("expected p450 match")
	}
	if filtered[0].GroupKey != "p450" {
		t.Fatalf("top fuzzy match = %q, want p450", filtered[0].GroupKey)
	}
	filtered = filterFamilyCandidates(candidates, "cytp450")
	if len(filtered) == 0 || filtered[0].GroupKey != "p450" {
		t.Fatalf("subsequence fuzzy match failed: %#v", filtered)
	}
}

func TestParseFamilyDetailRows(t *testing.T) {
	html := `
<h2><A NAME="P450"><B><i>Arabidopsis</i> P450 Gene Family</B></A></h2>
<table>
<tr><th>Sub Family</th><th>Gene Name</th><th>Genomic Locus Tag</th><th>Refseq ID</th><th>Protein Function</th></tr>
<tr><td rowspan="2">CYP51G</td><td>CYP51G2</td><td>AT2G17330</td><td>NM_127288</td><td>putative obtusifoliol 14-alpha demethylase</td></tr>
<tr><td>CYP51G1</td><td>AT1G11680</td><td>NM_101040</td><td>putative obtusifoliol 14-alpha demethylase</td></tr>
</table>`
	version := defaultVersionForTest()
	familyName, shortName, rows := parseFamilyDetailRows(version, "p450", html)
	if familyName == "" || shortName == "" {
		t.Fatalf("expected family names, got %q %q", familyName, shortName)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].GeneIdentifier == "" || rows[0].LabelName == "" {
		t.Fatalf("expected populated row, got %#v", rows[0])
	}
}

func TestKeywordRowsFromSearchDoc(t *testing.T) {
	doc := tairSearchDoc{
		ID:            "doc1",
		GeneName:      []string{"AT1G01010"},
		GeneModelIDs:  []string{"AT1G01010.1"},
		Description:   []string{"NAC domain containing protein 1"},
		OtherNames:    []string{"NAC001"},
		UniProtIDs:    []string{"Q9LZ76"},
		Keywords:      []string{"transcription factor"},
		KeywordTypes:  []string{"GO Biological Process"},
		GeneModelType: []string{"protein_coding"},
		Chromosome:    "1",
		MapType:       "AGI",
	}
	rows := keywordRowsFromSearchDoc(defaultVersionForTest(), doc)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].GeneIdentifier != "AT1G01010" || rows[0].TranscriptID != "AT1G01010.1" {
		t.Fatalf("unexpected row ids: %#v", rows[0])
	}
	if rows[0].UniProt != "Q9LZ76" {
		t.Fatalf("unexpected uniprot: %q", rows[0].UniProt)
	}
}

func TestParseRepresentativeModels(t *testing.T) {
	content := "# comment\nAT1G01010.1\nAT1G01020.2\n"
	index, err := parseRepresentativeModels(strings.NewReader(content))
	if err != nil {
		t.Fatalf("parseRepresentativeModels: %v", err)
	}
	if index["AT1G01010"] != "AT1G01010.1" {
		t.Fatalf("representative model mismatch: %#v", index)
	}
	if index["AT1G01020.2"] != "AT1G01020.2" {
		t.Fatalf("representative model exact mismatch: %#v", index)
	}
}

func TestParseDescriptionTable(t *testing.T) {
	content := "Model_name\tType\tShort_description\tCurator_summary\tComputational_description\nAT1G01010.1\tprotein_coding\tANAC001\tCurator text\tLong text\n"
	index, err := parseDescriptionTable(strings.NewReader(content))
	if err != nil {
		t.Fatalf("parseDescriptionTable: %v", err)
	}
	entry, ok := index["AT1G01010.1"]
	if !ok {
		t.Fatalf("description index missing transcript: %#v", index)
	}
	if entry.ShortDescription != "ANAC001" || entry.CuratorSummary != "Curator text" {
		t.Fatalf("unexpected description entry: %#v", entry)
	}
}

func TestBestMatchingRowPrefersExactTranscriptThenGene(t *testing.T) {
	rows := []model.KeywordResultRow{
		{GeneIdentifier: "AT1G01010", TranscriptID: "AT1G01010.2"},
		{GeneIdentifier: "AT1G01010", TranscriptID: "AT1G01010.1"},
	}
	row, ok := bestMatchingRow("AT1G01010.1", rows)
	if !ok {
		t.Fatal("expected best matching row")
	}
	if row.TranscriptID != "AT1G01010.1" {
		t.Fatalf("best match transcript = %q, want AT1G01010.1", row.TranscriptID)
	}
	row, ok = bestMatchingRow("AT1G01010", rows)
	if !ok || row.GeneIdentifier != "AT1G01010" {
		t.Fatalf("best gene match = %#v, ok=%v", row, ok)
	}
}

func TestParseFastaHeaderIndexTracksHeaderAndLength(t *testing.T) {
	index, err := parseFastaHeaderIndex(strings.NewReader(">gi|legacy|AT1G01010.1 desc\nMPEP\nTIDE\n"))
	if err != nil {
		t.Fatalf("parseFastaHeaderIndex: %v", err)
	}
	entry, ok := lookupFastaEntry(index, "AT1G01010.1")
	if !ok {
		t.Fatal("expected fasta header entry via exact token alias")
	}
	if entry.Defline != "gi|legacy|AT1G01010.1 desc" || entry.Length != 8 {
		t.Fatalf("fasta header entry = %#v", entry)
	}
}

func TestLoadFastaHeadersCachesParsedIndex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.fa")
	if err := os.WriteFile(path, []byte(">AT1G01010.1 desc\nMPEPTIDE\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	client := NewClient(nil)
	index1, err := client.loadFastaHeaders(context.Background(), path)
	if err != nil {
		t.Fatalf("loadFastaHeaders first: %v", err)
	}
	index2, err := client.loadFastaHeaders(context.Background(), path)
	if err != nil {
		t.Fatalf("loadFastaHeaders second: %v", err)
	}
	entry1, ok1 := lookupFastaEntry(index1, "AT1G01010.1")
	entry2, ok2 := lookupFastaEntry(index2, "AT1G01010.1")
	if !ok1 || !ok2 || entry1.Length != 8 || entry2.Defline == "" {
		t.Fatalf("cached fasta headers missing entries: first=%#v second=%#v", index1, index2)
	}
}

func TestLoadFastaHeadersReturnsCopyWhileSharedCacheStaysStable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.fa")
	if err := os.WriteFile(path, []byte(">AT1G01010.1 desc\nMPEPTIDE\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	client := NewClient(nil)

	shared, err := client.loadFastaHeadersShared(context.Background(), path)
	if err != nil {
		t.Fatalf("loadFastaHeadersShared: %v", err)
	}
	shared["MUTATED"] = fastaEntry{Defline: "mutated", Length: 99}

	public1, err := client.loadFastaHeaders(context.Background(), path)
	if err != nil {
		t.Fatalf("loadFastaHeaders first: %v", err)
	}
	public1["MUTATED_PUBLIC"] = fastaEntry{Defline: "public", Length: 88}

	public2, err := client.loadFastaHeaders(context.Background(), path)
	if err != nil {
		t.Fatalf("loadFastaHeaders second: %v", err)
	}
	if _, ok := public2["MUTATED_PUBLIC"]; ok {
		t.Fatal("public loadFastaHeaders result leaked caller mutation into cache")
	}
	if _, ok := public2["MUTATED"]; !ok {
		t.Fatal("shared cache mutation should remain visible to internal shared readers")
	}
}

func TestLoadFASTASequencesReturnsCopyWhileSharedCacheStaysStable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.fa")
	if err := os.WriteFile(path, []byte(">AT1G01010.1 desc\nMPEPTIDE\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	client := NewClient(nil)

	shared, err := client.loadFASTASequencesShared(context.Background(), path, client.proteinSeqs)
	if err != nil {
		t.Fatalf("loadFASTASequencesShared: %v", err)
	}
	shared["MUTATED"] = proteinEntry{ID: "MUTATED", Header: "mutated", Sequence: "AAAA"}

	public1, err := client.loadFASTASequences(context.Background(), path, client.proteinSeqs)
	if err != nil {
		t.Fatalf("loadFASTASequences first: %v", err)
	}
	public1["MUTATED_PUBLIC"] = proteinEntry{ID: "MUTATED_PUBLIC", Header: "public", Sequence: "BBBB"}

	public2, err := client.loadFASTASequences(context.Background(), path, client.proteinSeqs)
	if err != nil {
		t.Fatalf("loadFASTASequences second: %v", err)
	}
	if _, ok := public2["MUTATED_PUBLIC"]; ok {
		t.Fatal("public loadFASTASequences result leaked caller mutation into cache")
	}
	if entry, ok := public2["MUTATED"]; !ok || entry.Sequence != "AAAA" {
		t.Fatalf("shared cache mutation should remain visible to internal shared readers: %#v", public2["MUTATED"])
	}
}

func TestFetchProteinSequenceScansSingleEntryWithoutFullIndexWarmup(t *testing.T) {
	dir := t.TempDir()
	fastaPath := filepath.Join(dir, "TAIR10_pep.fa")
	if err := os.WriteFile(fastaPath, []byte(strings.Join([]string{
		">AT1G01010.1 desc",
		"MPEPTIDE",
		">AT2G37040.1 Symbols: PAL1 | phenylalanine ammonia-lyase 1",
		"MSTNPKPQR",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile fasta: %v", err)
	}
	client := NewClient(nil)
	rel := releaseInfo{
		Name:       "TAIR10-fetch-one",
		Label:      "TAIR10-fetch-one",
		ProteinURL: fastaPath,
	}
	client.releases[strings.ToLower(rel.Name)] = rel
	data, err := func() (model.ProteinSequenceData, error) {
		entry, err := client.lookupFASTASequenceEntry(context.Background(), rel.ProteinURL, client.proteinSeqs, "AT2G37040.1")
		if err != nil {
			return model.ProteinSequenceData{}, err
		}
		return model.ProteinSequenceData{Sequence: entry.Sequence, OriginalHeader: entry.Header}, nil
	}()
	if err != nil {
		t.Fatalf("FetchProteinSequence: %v", err)
	}
	if data.Sequence != "MSTNPKPQR" || !strings.Contains(data.OriginalHeader, "PAL1") {
		t.Fatalf("unexpected fetched sequence: %#v", data)
	}
}

func TestTAIR11FetchProteinSequenceReportsOfficialFastaAbsence(t *testing.T) {
	client := NewClient(nil)
	_, err := client.FetchProteinSequence(context.Background(), 370202, "AT1G01010.1")
	if err == nil {
		t.Fatal("expected TAIR11 FASTA absence error")
	}
	if !strings.Contains(err.Error(), "official protein FASTA") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTAIR11LocalBlastReportsOfficialFastaAbsence(t *testing.T) {
	client := NewClient(nil)
	_, err := client.RunLocalBlast(context.Background(), model.BlastRequest{
		Species:  model.SpeciesCandidate{ProteomeID: 370201, JBrowseName: "TAIR11", GenomeLabel: "TAIR11"},
		Program:  "blastp",
		Sequence: ">query\nMPEPTIDE\n",
	})
	if err == nil {
		t.Fatal("expected TAIR11 BLAST FASTA absence error")
	}
	if !strings.Contains(err.Error(), "official protein FASTA") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func defaultVersionForTest() model.SpeciesCandidate {
	return model.SpeciesCandidate{
		ProteomeID:  370202,
		JBrowseName: "TAIR10",
		GenomeLabel: "TAIR10",
		CommonName:  "Arabidopsis thaliana",
	}
}

func TestReleaseForTargetIDRejectsZero(t *testing.T) {
	client := NewClient(nil)
	if _, err := client.releaseForTargetID(0); err == nil {
		t.Fatal("expected missing target id error")
	}
}

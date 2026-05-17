package tair

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

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
	if !strings.Contains(byName["TAIR12"].GFFURL, "TAIR12_1Feb26.gff3.zip") {
		t.Fatalf("TAIR12 gff url = %q", byName["TAIR12"].GFFURL)
	}
	if byName["TAIR10"].ProteinURL == "" || !strings.Contains(byName["TAIR10"].ProteinURL, "TAIR10_pep_20110103_representative_gene_model_updated") {
		t.Fatalf("TAIR10 protein url = %q", byName["TAIR10"].ProteinURL)
	}
	if byName["TAIR10"].DescriptionURL == "" || !strings.Contains(byName["TAIR10"].DescriptionURL, "TAIR10_functional_descriptions_20130831.txt") {
		t.Fatalf("TAIR10 description url = %q", byName["TAIR10"].DescriptionURL)
	}
	if byName["TAIR9"].ProteinURL == "" || !strings.Contains(byName["TAIR9"].ProteinURL, "TAIR9_pep_20090619") {
		t.Fatalf("TAIR9 protein url = %q", byName["TAIR9"].ProteinURL)
	}
	if byName["TAIR8"].ProteinURL == "" || !strings.Contains(byName["TAIR8"].ProteinURL, "TAIR8_pep_20080412") {
		t.Fatalf("TAIR8 protein url = %q", byName["TAIR8"].ProteinURL)
	}
	if byName["TAIR7"].ProteinURL == "" || !strings.Contains(byName["TAIR7"].ProteinURL, "TAIR7_pep_20070425") {
		t.Fatalf("TAIR7 protein url = %q", byName["TAIR7"].ProteinURL)
	}
	if byName["TAIR9"].NucleotideURL == "" || !strings.Contains(byName["TAIR9"].NucleotideURL, "TAIR9_chr_all.fas") {
		t.Fatalf("TAIR9 nucleotide url = %q", byName["TAIR9"].NucleotideURL)
	}
	if byName["TAIR11"].GFFURL != "" || byName["TAIR11"].ProteinURL != "" {
		t.Fatalf("TAIR11 should remain empty until public assets are exposed: %#v", byName["TAIR11"])
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
		t.Fatalf("blast list should hide unavailable releases, got %#v", seenBlast)
	}
	if !seenBlast["TAIR10"] || !seenBlast["Araport11"] || !seenBlast["TAIR9"] || !seenBlast["TAIR8"] || !seenBlast["TAIR7"] || !seenBlast["TAIR6"] {
		t.Fatalf("blast list missing usable releases: %#v", seenBlast)
	}

	seenKeyword := map[string]bool{}
	for _, candidate := range keywordCandidates {
		seenKeyword[candidate.JBrowseName] = true
	}
	if seenKeyword["TAIR11"] {
		t.Fatalf("keyword list should hide TAIR11: %#v", seenKeyword)
	}
	if !seenKeyword["TAIR12"] || !seenKeyword["TAIR7"] || !seenKeyword["TAIR10"] {
		t.Fatalf("keyword list missing GFF-backed releases: %#v", seenKeyword)
	}

	seenFamily := map[string]bool{}
	for _, candidate := range familyCandidates {
		seenFamily[candidate.JBrowseName] = true
	}
	if !seenFamily["TAIR12"] || seenFamily["TAIR11"] == true {
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

func defaultVersionForTest() model.SpeciesCandidate {
	return model.SpeciesCandidate{
		ProteomeID:  370202,
		JBrowseName: "TAIR10",
		GenomeLabel: "TAIR10",
		CommonName:  "Arabidopsis thaliana",
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFetchProteinSequenceFallsBackToExternalSources(t *testing.T) {
	client := NewClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.Host, "rest.ensembl.org") && strings.Contains(req.URL.Path, "/sequence/id/AT1G01010.1"):
				body := `{"id":"AT1G01010.1","query":"AT1G01010.1","desc":"NAC domain containing protein 1","seq":"MPEPTIDE","molecule":"protein"}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader(`{"error":"not found"}`)),
					Header:     make(http.Header),
				}, nil
			}
		}),
	})
	data, err := client.FetchProteinSequence(context.Background(), 370201, "AT1G01010.1")
	if err != nil {
		t.Fatalf("FetchProteinSequence external fallback: %v", err)
	}
	if got := strings.TrimSpace(data.Sequence); got != "MPEPTIDE" {
		t.Fatalf("external fallback sequence = %q, want %q", got, "MPEPTIDE")
	}
	if !strings.Contains(data.OriginalHeader, "AT1G01010.1") {
		t.Fatalf("external fallback header = %q", data.OriginalHeader)
	}
}

func TestFetchProteinSequenceExternalFallbackSupportsTAIR12Target(t *testing.T) {
	client := NewClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.Host, "rest.ensembl.org") && strings.Contains(req.URL.Path, "/sequence/id/AT1G01010.1"):
				body := `{"id":"AT1G01010.1","query":"AT1G01010.1","desc":"NAC domain containing protein 1","seq":"MTAIR12SEQ","molecule":"protein"}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader(`{"error":"not found"}`)),
					Header:     make(http.Header),
				}, nil
			}
		}),
	})
	data, err := client.FetchProteinSequence(context.Background(), 370201, "AT1G01010.1")
	if err != nil {
		t.Fatalf("FetchProteinSequence TAIR12 fallback: %v", err)
	}
	if got := strings.TrimSpace(data.Sequence); got == "" {
		t.Fatal("expected TAIR12 fallback sequence")
	}
	if !strings.Contains(strings.TrimSpace(data.OriginalHeader), "AT1G01010.1") {
		t.Fatalf("unexpected TAIR12 fallback header: %q", data.OriginalHeader)
	}
}

func TestReleaseForTargetIDRejectsZero(t *testing.T) {
	client := NewClient(nil)
	if _, err := client.releaseForTargetID(0); err == nil {
		t.Fatal("expected missing target id error")
	}
}

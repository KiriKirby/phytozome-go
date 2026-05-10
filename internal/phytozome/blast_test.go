package phytozome

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

func TestSpecificIdentifierVariants(t *testing.T) {
	got := specificIdentifierVariants("At2g37040")
	want := []string{"AT2G37040", "At2g37040", "at2g37040"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected variants: got %v want %v", got, want)
	}
}

func TestSpecificIdentifierVariantsDeduplicates(t *testing.T) {
	got := specificIdentifierVariants("AT2G37040")
	want := []string{"AT2G37040", "at2g37040"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected variants: got %v want %v", got, want)
	}
}

func TestApplyPhytozomeQueryLabelsUsesFirstAlias(t *testing.T) {
	var source model.QuerySequenceSource
	applyPhytozomeQueryLabels(&source, geneRecord{
		Symbols:  []string{"PAL4", "PAL4"},
		Synonyms: []string{"ATPAL4"},
	})
	if source.LabelName != "PAL4" {
		t.Fatalf("unexpected label: %q", source.LabelName)
	}
	if source.Aliases != "PAL4; ATPAL4" {
		t.Fatalf("unexpected aliases: %q", source.Aliases)
	}
}

func TestPhytozomeGeneReportKeywordParsesURL(t *testing.T) {
	reportType, identifier, ok := phytozomeGeneReportKeyword("https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT3G10340")
	if !ok {
		t.Fatal("expected Phytozome gene report URL to parse")
	}
	if reportType != "gene" || identifier != "AT3G10340" {
		t.Fatalf("unexpected parsed values: reportType=%q identifier=%q", reportType, identifier)
	}
}

func TestPhytozomeGeneReportKeywordParsesURLWithoutScheme(t *testing.T) {
	reportType, identifier, ok := phytozomeGeneReportKeyword("phytozome-next.jgi.doe.gov/report/transcript/Athaliana_TAIR10/AT3G10340.1")
	if !ok {
		t.Fatal("expected Phytozome transcript report URL without scheme to parse")
	}
	if reportType != "transcript" || identifier != "AT3G10340.1" {
		t.Fatalf("unexpected parsed values: reportType=%q identifier=%q", reportType, identifier)
	}
}

func TestPhytozomeGeneReportKeywordParsesProteinURL(t *testing.T) {
	reportType, identifier, ok := phytozomeGeneReportKeyword("https://phytozome-next.jgi.doe.gov/report/protein/S_polyrhiza_v2/Spipo15G0028500")
	if !ok {
		t.Fatal("expected Phytozome protein report URL to parse")
	}
	if reportType != "protein" || identifier != "Spipo15G0028500" {
		t.Fatalf("unexpected parsed values: reportType=%q identifier=%q", reportType, identifier)
	}
}

func TestSearchKeywordRowsBroadMatchesWebSearchFor4CL(t *testing.T) {
	if testing.Short() {
		t.Skip("live Phytozome keyword search")
	}
	if os.Getenv("PHYTOZOME_LIVE_TESTS") != "1" {
		t.Skip("set PHYTOZOME_LIVE_TESTS=1 to run live Phytozome keyword search")
	}
	client := NewClient(nil)
	rows, err := client.SearchKeywordRowsBroad(context.Background(), model.SpeciesCandidate{
		ProteomeID:  167,
		JBrowseName: "Athaliana_TAIR10",
		GenomeLabel: "Arabidopsis thaliana TAIR10",
		SearchAlias: "A.thaliana TAIR10",
		CommonName:  "thale cress",
	}, "4CL")
	if err != nil {
		t.Fatalf("SearchKeywordRowsBroad returned error: %v", err)
	}
	want := []string{"AT4G05160", "AT5G38120", "AT3G21240", "AT1G62940", "AT4G19010", "AT3G21230", "AT1G51680", "AT1G65060"}
	if len(rows) != len(want) {
		t.Fatalf("broad 4CL rows = %d, want %d: %#v", len(rows), len(want), rows)
	}
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, strings.TrimSpace(strings.Split(row.GeneIdentifier, " ")[0]))
	}
	slices.Sort(got)
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("broad 4CL genes = %v, want %v", got, want)
	}
}

func TestSearchKeywordRowsMatchesArabidopsisIdentifiers(t *testing.T) {
	if testing.Short() {
		t.Skip("live Phytozome keyword search")
	}
	if os.Getenv("PHYTOZOME_LIVE_TESTS") != "1" {
		t.Skip("set PHYTOZOME_LIVE_TESTS=1 to run live Phytozome keyword search")
	}
	client := NewClient(nil)
	species := model.SpeciesCandidate{
		ProteomeID:  167,
		JBrowseName: "Athaliana_TAIR10",
		GenomeLabel: "Arabidopsis thaliana TAIR10",
		SearchAlias: "A.thaliana TAIR10",
		CommonName:  "thale cress",
	}
	keywords := []string{
		"At2g37040",
		"At3g53260",
		"At5g04230",
		"At3g10340",
		"At2g30490",
		"At1g51680",
		"At3g21240",
		"At1g15950",
		"At1g80820",
		"At5g48930",
		"At2g40890",
		"At1g52760",
		"At4g34050",
		"At4g36220",
		"At5g54160",
		"At4g34230",
		"At4g37970",
		"At1g07890",
		"At3g24503",
	}
	for _, keyword := range keywords {
		rows, err := client.SearchKeywordRows(context.Background(), species, keyword)
		if err != nil {
			t.Fatalf("%s returned error: %v", keyword, err)
		}
		if len(rows) != 1 {
			t.Fatalf("%s rows = %d, want 1: %#v", keyword, len(rows), rows)
		}
		if got := strings.ToUpper(strings.Fields(rows[0].GeneIdentifier)[0]); got != strings.ToUpper(keyword) {
			t.Fatalf("%s gene identifier = %q, want %s", keyword, rows[0].GeneIdentifier, strings.ToUpper(keyword))
		}
	}
}

func TestKeywordSearchEngineLazyInitIsConcurrentSafe(t *testing.T) {
	client := &Client{}
	const workers = 32
	var wg sync.WaitGroup
	engines := make(chan uintptr, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			engine := client.keywordSearchEngine()
			if engine == nil {
				t.Error("keywordSearchEngine returned nil")
				return
			}
			engines <- reflect.ValueOf(engine).Pointer()
		}()
	}
	wg.Wait()
	close(engines)

	var first uintptr
	for engine := range engines {
		if first == 0 {
			first = engine
			continue
		}
		if engine != first {
			t.Fatalf("keywordSearchEngine initialized multiple engines: %x and %x", first, engine)
		}
	}
}

func TestFetchUniProtAccessionsFallsBackToTranscriptLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/db/gene_167") {
			http.NotFound(w, r)
			return
		}
		switch r.URL.RawQuery {
		case "protein=AT2G30490.1":
			http.Error(w, "no gene record", http.StatusNoContent)
			return
		case "transcript=AT2G30490.1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"_id":"gene-1","primaryidentifier":"AT2G30490","transcripts":[{"primaryidentifier":"AT2G30490.1","protein":"AT2G30490.1","secondaryidentifier":"PAC:27150491","uniprot":["UniProtKB:Q9ZNS6"]}]}`)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	client := NewClient(nil)
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	client.baseHTTP = &http.Client{
		Transport: rewriteHostTransport{
			base:       server.Client().Transport,
			scheme:     serverURL.Scheme,
			host:       serverURL.Host,
			targetHost: "phytozome-next.jgi.doe.gov",
		},
	}

	got, err := client.FetchUniProtAccessions(context.Background(), 167, "AT2G30490.1")
	if err != nil {
		t.Fatalf("FetchUniProtAccessions returned error: %v", err)
	}
	want := []string{"Q9ZNS6"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FetchUniProtAccessions = %v, want %v", got, want)
	}
}

type rewriteHostTransport struct {
	base       http.RoundTripper
	scheme     string
	host       string
	targetHost string
}

func (r rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if strings.EqualFold(clone.URL.Host, r.targetHost) {
		clone.URL.Scheme = r.scheme
		clone.URL.Host = r.host
		clone.Host = r.host
	}
	return r.base.RoundTrip(clone)
}

func TestExtractUniProtAccessionsFromGeneMatchesTranscriptAndGeneIdentifiers(t *testing.T) {
	gene := geneRecord{
		PrimaryIdentifier: "AT2G30490",
		Transcripts: []geneTranscript{{
			PrimaryIdentifier:   "AT2G30490.1",
			SecondaryIdentifier: "PAC:27150491",
			Protein:             "AT2G30490.1",
			Uniprot:             []string{"UniProtKB:Q9ZNS6", "Q9ZNS6"},
		}},
	}
	if got := extractUniProtAccessionsFromGene(gene, "AT2G30490.1"); !reflect.DeepEqual(got, []string{"Q9ZNS6"}) {
		t.Fatalf("extractUniProtAccessionsFromGene transcript match = %v, want [Q9ZNS6]", got)
	}
	if got := extractUniProtAccessionsFromGene(gene, "AT2G30490"); !reflect.DeepEqual(got, []string{"Q9ZNS6"}) {
		t.Fatalf("extractUniProtAccessionsFromGene gene match = %v, want [Q9ZNS6]", got)
	}
	if got := extractUniProtAccessionsFromGene(gene, "PAC:27150491"); !reflect.DeepEqual(got, []string{"Q9ZNS6"}) {
		t.Fatalf("extractUniProtAccessionsFromGene secondary id match = %v, want [Q9ZNS6]", got)
	}
}

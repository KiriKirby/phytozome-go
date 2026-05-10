package uniprot

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/KiriKirby/phytozome-go/internal/cachex"
	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/goccy/go-json"
	"github.com/jszwec/csvutil"
	"golang.org/x/sync/singleflight"
)

const (
	uniprotBaseURL = "https://rest.uniprot.org/uniprotkb/"
	searchFields   = "accession,id,reviewed,protein_name,gene_names,organism_name,organism_id,length,cc_function,cc_catalytic_activity,go,go_id,ec,keyword,xref_pfam,xref_interpro,cc_pathway,cc_subcellular_location,protein_existence,annotation_score,fragment,cc_sequence_caution,ft_domain,ft_region,ft_motif,ft_act_site,ft_binding,xref_alphafolddb,xref_pdb"
)

var diskCache = cachex.MustOpen("uniprot", "search")

type Client struct {
	httpClient *http.Client
	mu         sync.RWMutex
	cache      map[string]Entry
	sf         singleflight.Group
}

type Entry struct {
	Accession           string
	EntryName           string
	Reviewed            string
	ProteinName         string
	GeneNames           string
	Organism            string
	OrganismID          string
	Length              int
	Function            string
	CatalyticActivity   string
	GO                  string
	GOIDs               string
	EC                  string
	Keywords            string
	Pfam                string
	InterPro            string
	Pathway             string
	SubcellularLocation string
	ProteinExistence    string
	AnnotationScore     string
	Fragment            string
	SequenceCaution     string
	Domain              string
	Region              string
	Motif               string
	ActiveSite          string
	BindingSite         string
	AlphaFoldDB         string
	PDB                 string
}

type diskEntry struct {
	Entry Entry `json:"entry"`
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = phygoboost.HTTPClient()
	}
	return &Client{
		httpClient: httpClient,
		cache:      make(map[string]Entry),
	}
}

func (c *Client) Lookup(ctx context.Context, accession string, row model.BlastResultRow) (Entry, bool, error) {
	result, err := phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "rest.uniprot.org",
		Description: "lookup uniprot entry",
	}, func(runCtx context.Context) (struct {
		Entry Entry
		OK    bool
	}, error) {
		entry, ok, err := c.lookup(runCtx, accession, row)
		return struct {
			Entry Entry
			OK    bool
		}{Entry: entry, OK: ok}, err
	})
	if err != nil {
		return Entry{}, false, err
	}
	return result.Entry, result.OK, nil
}

func (c *Client) lookup(ctx context.Context, accession string, row model.BlastResultRow) (Entry, bool, error) {
	accession = normalizeAccession(accession)
	if accession != "" {
		entry, ok, err := c.lookupByQuery(ctx, "accession:"+accession, "accession:"+accession)
		if err != nil {
			return Entry{}, false, err
		}
		if ok {
			return entry, true, nil
		}
	}
	for _, query := range candidateQueries(row) {
		entry, ok, err := c.lookupByQuery(ctx, query, query)
		if err != nil {
			continue
		}
		if ok {
			return entry, true, nil
		}
	}
	return Entry{}, false, nil
}

func (c *Client) lookupByQuery(ctx context.Context, cacheKey string, query string) (Entry, bool, error) {
	cacheKey = strings.TrimSpace(cacheKey)
	query = strings.TrimSpace(query)
	if cacheKey == "" || query == "" {
		return Entry{}, false, nil
	}
	c.mu.RLock()
	if cached, ok := c.cache[cacheKey]; ok {
		c.mu.RUnlock()
		return cached, cached.Accession != "", nil
	}
	c.mu.RUnlock()
	if cached, ok := readDiskEntry(cacheKey); ok {
		c.mu.Lock()
		c.cache[cacheKey] = cached
		c.mu.Unlock()
		return cached, cached.Accession != "", nil
	}

	value, err, _ := c.sf.Do(cacheKey, func() (any, error) {
		c.mu.RLock()
		if cached, ok := c.cache[cacheKey]; ok {
			c.mu.RUnlock()
			return cached, nil
		}
		c.mu.RUnlock()
		if cached, ok := readDiskEntry(cacheKey); ok {
			c.mu.Lock()
			c.cache[cacheKey] = cached
			c.mu.Unlock()
			return cached, nil
		}

		requestURL := uniprotBaseURL + "search?query=" + url.QueryEscape(query) + "&fields=" + url.QueryEscape(searchFields) + "&format=tsv&size=5"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return Entry{}, fmt.Errorf("create UniProt request: %w", err)
		}
		req.Header.Set("Accept", "text/tab-separated-values")
		req.Header.Set("Accept-Encoding", "identity")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return Entry{}, fmt.Errorf("fetch UniProt: %w", err)
		}
		defer phygoboost.DrainAndClose(resp.Body)
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return Entry{}, fmt.Errorf("fetch UniProt: status %s body %s", resp.Status, strings.TrimSpace(string(body)))
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return Entry{}, fmt.Errorf("read UniProt response: %w", err)
		}
		entries, err := parseTSV(string(body))
		if err != nil {
			return Entry{}, err
		}
		entry := chooseEntry(entries)
		c.mu.Lock()
		c.cache[cacheKey] = entry
		c.mu.Unlock()
		writeDiskEntry(cacheKey, entry)
		return entry, nil
	})
	if err != nil {
		return Entry{}, false, err
	}
	entry := value.(Entry)
	return entry, entry.Accession != "", nil
}

func candidateQueries(row model.BlastResultRow) []string {
	terms := []string{
		row.UniProtAccession,
		row.Protein,
		row.SubjectID,
		row.SequenceID,
		row.TranscriptID,
	}
	terms = append(terms, extractUniProtAccessions(row.Defline)...)
	terms = append(terms, extractUniProtAccessions(row.GeneReportURL)...)
	organism := cleanOrganism(row.Species)
	queries := make([]string, 0, len(terms)*8)
	seen := make(map[string]struct{})
	add := func(query string) {
		query = strings.TrimSpace(query)
		if query == "" {
			return
		}
		key := strings.ToLower(query)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		queries = append(queries, query)
	}
	for _, rawTerm := range terms {
		term := cleanIdentifier(rawTerm)
		if term == "" {
			continue
		}
		add("accession:" + term)
		add("id:" + term)
		add("xref:" + term)
		add("gene:" + term)
		add(term)
		if organism != "" {
			add(term + " AND organism_name:\"" + organism + "\"")
			add("gene:" + term + " AND organism_name:\"" + organism + "\"")
			add("xref:" + term + " AND organism_name:\"" + organism + "\"")
		}
		if base := stripIsoform(term); base != term {
			add("accession:" + base)
			add("id:" + base)
			add("xref:" + base)
			add("gene:" + base)
			add(base)
			if organism != "" {
				add(base + " AND organism_name:\"" + organism + "\"")
				add("gene:" + base + " AND organism_name:\"" + organism + "\"")
				add("xref:" + base + " AND organism_name:\"" + organism + "\"")
			}
		}
	}
	return queries
}

func readDiskEntry(cacheKey string) (Entry, bool) {
	var stored diskEntry
	if !diskCache.ReadJSON("entry:"+cacheKey, &stored) {
		return Entry{}, false
	}
	return stored.Entry, true
}

func writeDiskEntry(cacheKey string, entry Entry) {
	diskCache.WriteJSON("entry:"+cacheKey, diskEntry{Entry: entry})
}

func parseTSV(raw string) ([]Entry, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	reader := csv.NewReader(strings.NewReader(raw))
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	decoder, err := csvutil.NewDecoder(reader)
	if err != nil {
		return nil, fmt.Errorf("parse UniProt TSV: %w", err)
	}
	decoder.AlignRecord = true
	decoder.Map = func(field, col string, v any) string {
		return cleanValue(field)
	}

	var entries []Entry
	for {
		var row uniprotTSVRow
		if err := decoder.Decode(&row); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("parse UniProt TSV: %w", err)
		}
		length, _ := strconv.Atoi(row.Length)
		entries = append(entries, Entry{
			Accession:           row.Entry,
			EntryName:           row.EntryName,
			Reviewed:            row.Reviewed,
			ProteinName:         row.ProteinName,
			GeneNames:           row.GeneNames,
			Organism:            row.Organism,
			OrganismID:          row.OrganismID,
			Length:              length,
			Function:            row.Function,
			CatalyticActivity:   row.CatalyticActivity,
			GO:                  row.GO,
			GOIDs:               row.GOIDs,
			EC:                  row.EC,
			Keywords:            row.Keywords,
			Pfam:                row.Pfam,
			InterPro:            row.InterPro,
			Pathway:             row.Pathway,
			SubcellularLocation: row.SubcellularLocation,
			ProteinExistence:    row.ProteinExistence,
			AnnotationScore:     row.AnnotationScore,
			Fragment:            row.Fragment,
			SequenceCaution:     row.SequenceCaution,
			Domain:              row.Domain,
			Region:              row.Region,
			Motif:               row.Motif,
			ActiveSite:          row.ActiveSite,
			BindingSite:         row.BindingSite,
			AlphaFoldDB:         row.AlphaFoldDB,
			PDB:                 row.PDB,
		})
	}
	return entries, nil
}

type uniprotTSVRow struct {
	Entry               string `csv:"Entry"`
	EntryName           string `csv:"Entry Name"`
	Reviewed            string `csv:"Reviewed"`
	ProteinName         string `csv:"Protein names"`
	GeneNames           string `csv:"Gene Names"`
	Organism            string `csv:"Organism"`
	OrganismID          string `csv:"Organism (ID)"`
	Length              string `csv:"Length"`
	Function            string `csv:"Function [CC]"`
	CatalyticActivity   string `csv:"Catalytic activity"`
	GO                  string `csv:"Gene Ontology (GO)"`
	GOIDs               string `csv:"Gene Ontology IDs"`
	EC                  string `csv:"EC number"`
	Keywords            string `csv:"Keywords"`
	Pfam                string `csv:"Pfam"`
	InterPro            string `csv:"InterPro"`
	Pathway             string `csv:"Pathway"`
	SubcellularLocation string `csv:"Subcellular location [CC]"`
	ProteinExistence    string `csv:"Protein existence"`
	AnnotationScore     string `csv:"Annotation"`
	Fragment            string `csv:"Fragment"`
	SequenceCaution     string `csv:"Sequence caution"`
	Domain              string `csv:"Domain [FT]"`
	Region              string `csv:"Region"`
	Motif               string `csv:"Motif"`
	ActiveSite          string `csv:"Active site"`
	BindingSite         string `csv:"Binding site"`
	AlphaFoldDB         string `csv:"AlphaFoldDB"`
	PDB                 string `csv:"PDB"`
}

func chooseEntry(entries []Entry) Entry {
	if len(entries) == 0 {
		return Entry{}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		leftReviewed := strings.EqualFold(entries[i].Reviewed, "reviewed")
		rightReviewed := strings.EqualFold(entries[j].Reviewed, "reviewed")
		if leftReviewed != rightReviewed {
			return leftReviewed
		}
		leftScore := populatedScore(entries[i])
		rightScore := populatedScore(entries[j])
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		return entries[i].Length > entries[j].Length
	})
	return entries[0]
}

func populatedScore(entry Entry) int {
	values := []string{
		entry.ProteinName,
		entry.GeneNames,
		entry.Function,
		entry.CatalyticActivity,
		entry.GO,
		entry.EC,
		entry.Keywords,
		entry.Pfam,
		entry.InterPro,
		entry.Pathway,
		entry.SubcellularLocation,
	}
	score := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			score++
		}
	}
	return score
}

func normalizeAccession(value string) string {
	value = strings.TrimSpace(value)
	if strings.Contains(value, ":") {
		parts := strings.Split(value, ":")
		value = parts[len(parts)-1]
	}
	value = strings.Trim(value, ";, ")
	if strings.ContainsAny(value, " \t\r\n") {
		return ""
	}
	return value
}

func cleanIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "|") {
		parts := strings.Split(value, "|")
		value = parts[len(parts)-1]
	}
	value = strings.Fields(value)[0]
	value = strings.Trim(value, ";, ")
	return value
}

func extractUniProtAccessions(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ';' || r == ',' || r == '|' || r == '(' || r == ')' || r == '[' || r == ']'
	})
	out := make([]string, 0, 2)
	seen := map[string]struct{}{}
	for _, field := range fields {
		field = strings.Trim(field, `"'=:`)
		if !looksLikeUniProtAccession(field) {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

func looksLikeUniProtAccession(value string) bool {
	if len(value) < 6 || len(value) > 10 {
		return false
	}
	hasDigit := false
	for _, ch := range value {
		switch {
		case ch >= 'A' && ch <= 'Z':
		case ch >= '0' && ch <= '9':
			hasDigit = true
		default:
			return false
		}
	}
	if !hasDigit {
		return false
	}
	first := value[0]
	return (first >= 'A' && first <= 'Z') || (first >= '0' && first <= '9')
}

func stripIsoform(value string) string {
	if idx := strings.LastIndex(value, "."); idx > 0 {
		return value[:idx]
	}
	if idx := strings.LastIndex(value, "-"); idx > 0 {
		suffix := value[idx+1:]
		allDigits := suffix != ""
		for _, ch := range suffix {
			if ch < '0' || ch > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return value[:idx]
		}
	}
	return value
}

func cleanOrganism(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.Index(value, "("); idx > 0 {
		value = strings.TrimSpace(value[:idx])
	}
	return value
}

func cleanValue(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func ToJSON(entry Entry) string {
	data, err := json.Marshal(entry)
	if err != nil {
		return ""
	}
	return string(data)
}


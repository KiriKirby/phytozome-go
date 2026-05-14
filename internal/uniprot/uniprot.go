// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package uniprot

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"golang.org/x/sync/singleflight"
)

const (
	uniprotBaseURL   = "https://rest.uniprot.org/uniprotkb/"
	searchFields     = "accession,id,reviewed,protein_name,gene_names,organism_name,organism_id,length,cc_function,cc_catalytic_activity,go,go_id,ec,keyword,xref_pfam,xref_interpro,cc_pathway,cc_subcellular_location,protein_existence,annotation_score,fragment,cc_sequence_caution,ft_domain,ft_region,ft_motif,ft_act_site,ft_binding,xref_alphafolddb,xref_pdb"
	maxResponseBytes = 16 << 20
)

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
		httpClient = defaultHTTPClient()
	}
	return &Client{
		httpClient: httpClient,
		cache:      make(map[string]Entry),
	}
}

func (c *Client) Lookup(ctx context.Context, accession string, row model.BlastResultRow) (Entry, bool, error) {
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
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			return Entry{}, fmt.Errorf("fetch UniProt: status %s body %s", resp.Status, strings.TrimSpace(string(body)))
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
		if err != nil {
			return Entry{}, fmt.Errorf("read UniProt response: %w", err)
		}
		if len(body) > maxResponseBytes {
			return Entry{}, fmt.Errorf("read UniProt response: response exceeds %d bytes", maxResponseBytes)
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
	path, err := diskEntryPath(cacheKey)
	if err != nil {
		return Entry{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, false
	}
	var stored diskEntry
	if err := json.Unmarshal(data, &stored); err != nil {
		return Entry{}, false
	}
	return stored.Entry, true
}

func writeDiskEntry(cacheKey string, entry Entry) {
	path, err := diskEntryPath(cacheKey)
	if err != nil {
		return
	}
	data, err := json.MarshalIndent(diskEntry{Entry: entry}, "", "  ")
	if err != nil {
		return
	}
	_ = appfs.WriteFileAtomic(path, data, 0o644)
}

func diskEntryPath(cacheKey string) (string, error) {
	dir, err := appfs.CacheDir("uniprot", "search")
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(cacheKey))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".json"), nil
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
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse UniProt TSV: %w", err)
	}
	if len(records) < 2 {
		return nil, nil
	}
	index := make(map[string]int, len(records[0]))
	for i, header := range records[0] {
		index[strings.TrimSpace(header)] = i
	}
	entries := make([]Entry, 0, len(records)-1)
	for _, fields := range records[1:] {
		get := func(name string) string {
			i, ok := index[name]
			if !ok || i >= len(fields) {
				return ""
			}
			return cleanValue(fields[i])
		}
		length, _ := strconv.Atoi(get("Length"))
		entries = append(entries, Entry{
			Accession:           get("Entry"),
			EntryName:           get("Entry Name"),
			Reviewed:            get("Reviewed"),
			ProteinName:         get("Protein names"),
			GeneNames:           get("Gene Names"),
			Organism:            get("Organism"),
			OrganismID:          get("Organism (ID)"),
			Length:              length,
			Function:            get("Function [CC]"),
			CatalyticActivity:   get("Catalytic activity"),
			GO:                  get("Gene Ontology (GO)"),
			GOIDs:               get("Gene Ontology IDs"),
			EC:                  get("EC number"),
			Keywords:            get("Keywords"),
			Pfam:                get("Pfam"),
			InterPro:            get("InterPro"),
			Pathway:             get("Pathway"),
			SubcellularLocation: get("Subcellular location [CC]"),
			ProteinExistence:    get("Protein existence"),
			AnnotationScore:     get("Annotation"),
			Fragment:            get("Fragment"),
			SequenceCaution:     get("Sequence caution"),
			Domain:              get("Domain [FT]"),
			Region:              get("Region"),
			Motif:               get("Motif"),
			ActiveSite:          get("Active site"),
			BindingSite:         get("Binding site"),
			AlphaFoldDB:         get("AlphaFoldDB"),
			PDB:                 get("PDB"),
		})
	}
	return entries, nil
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

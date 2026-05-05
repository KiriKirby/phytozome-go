package interpro

import (
	"context"
	"crypto/sha256"
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
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"golang.org/x/sync/singleflight"
)

const interProBaseURL = "https://www.ebi.ac.uk/interpro/api/entry/all/protein/uniprot/"

type Client struct {
	httpClient *http.Client
	mu         sync.RWMutex
	cache      map[string]Entry
	sf         singleflight.Group
}

type Entry struct {
	Accession           string
	ProteinLength       int
	Matches             []Match
	Accessions          string
	EntryNames          string
	EntryTypes          string
	CoveragePercent     string
	MatchRegions        string
	SignatureAccessions string
	PfamAccessions      string
}

type Match struct {
	Accession            string
	Name                 string
	SourceDatabase       string
	Type                 string
	IntegratedAccession  string
	SignatureAccessions  []string
	PfamAccessions       []string
	Regions              []Region
	CoveragePercent      float64
	CoverageLength       int
}

type Region struct {
	Start  int
	End    int
	Status string
}

type diskEntry struct {
	Entry Entry `json:"entry"`
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 45 * time.Second}
	}
	return &Client{
		httpClient: httpClient,
		cache:      make(map[string]Entry),
	}
}

func (c *Client) Lookup(ctx context.Context, accession string) (Entry, bool, error) {
	accession = normalizeAccession(accession)
	if accession == "" {
		return Entry{}, false, nil
	}
	c.mu.RLock()
	if cached, ok := c.cache[accession]; ok {
		c.mu.RUnlock()
		return cached, cached.Accession != "", nil
	}
	c.mu.RUnlock()
	if cached, ok := readDiskEntry(accession); ok {
		c.mu.Lock()
		c.cache[accession] = cached
		c.mu.Unlock()
		return cached, cached.Accession != "", nil
	}

	value, err, _ := c.sf.Do(accession, func() (any, error) {
		c.mu.RLock()
		if cached, ok := c.cache[accession]; ok {
			c.mu.RUnlock()
			return cached, nil
		}
		c.mu.RUnlock()
		if cached, ok := readDiskEntry(accession); ok {
			c.mu.Lock()
			c.cache[accession] = cached
			c.mu.Unlock()
			return cached, nil
		}
		entry, err := c.fetchAllPages(ctx, accession)
		if err != nil {
			return Entry{}, err
		}
		c.mu.Lock()
		c.cache[accession] = entry
		c.mu.Unlock()
		writeDiskEntry(accession, entry)
		return entry, nil
	})
	if err != nil {
		return Entry{}, false, err
	}
	entry := value.(Entry)
	return entry, entry.Accession != "", nil
}

func (c *Client) fetchAllPages(ctx context.Context, accession string) (Entry, error) {
	requestURL := interProBaseURL + url.PathEscape(accession) + "/?format=json&page_size=200"
	entry := Entry{Accession: accession}
	for requestURL != "" {
		page, err := c.fetchPage(ctx, requestURL)
		if err != nil {
			return Entry{}, err
		}
		if page.Metadata.Accession != "" {
			entry.Accession = page.Metadata.Accession
		}
		if page.Metadata.Length > 0 {
			entry.ProteinLength = page.Metadata.Length
		}
		for _, result := range page.Results {
			match := result.toMatch()
			if match.Accession == "" {
				continue
			}
			if entry.ProteinLength <= 0 && result.firstProteinLength() > 0 {
				entry.ProteinLength = result.firstProteinLength()
			}
			if entry.ProteinLength > 0 {
				match.CoveragePercent = float64(match.CoverageLength) / float64(entry.ProteinLength) * 100
			}
			entry.Matches = append(entry.Matches, match)
		}
		requestURL = strings.TrimSpace(page.Next)
	}
	entry.finalize()
	return entry, nil
}

func (c *Client) fetchPage(ctx context.Context, requestURL string) (apiPage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return apiPage{}, fmt.Errorf("create InterPro request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return apiPage{}, fmt.Errorf("fetch InterPro: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return apiPage{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return apiPage{}, fmt.Errorf("fetch InterPro: status %s body %s", resp.Status, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return apiPage{}, fmt.Errorf("read InterPro response: %w", err)
	}
	var page apiPage
	if err := json.Unmarshal(body, &page); err != nil {
		return apiPage{}, fmt.Errorf("decode InterPro response: %w", err)
	}
	return page, nil
}

func (entry *Entry) finalize() {
	sort.SliceStable(entry.Matches, func(i, j int) bool {
		left := entry.Matches[i]
		right := entry.Matches[j]
		if left.SourceDatabase == "pfam" && right.SourceDatabase != "pfam" {
			return true
		}
		if left.SourceDatabase != "pfam" && right.SourceDatabase == "pfam" {
			return false
		}
		if left.CoverageLength != right.CoverageLength {
			return left.CoverageLength > right.CoverageLength
		}
		return left.Accession < right.Accession
	})
	accessions := make([]string, 0, len(entry.Matches))
	names := make([]string, 0, len(entry.Matches))
	types := make([]string, 0, len(entry.Matches))
	regions := make([]string, 0, len(entry.Matches))
	signatures := make([]string, 0, len(entry.Matches))
	pfams := make([]string, 0, len(entry.Matches))
	totalCovered := 0
	for _, match := range entry.Matches {
		accessions = append(accessions, match.Accession)
		if match.Name != "" {
			names = append(names, match.Name)
		}
		if match.Type != "" {
			types = append(types, match.Type)
		}
		if regionText := matchRegionsText(match); regionText != "" {
			regions = append(regions, match.Accession+":"+regionText)
		}
		signatures = append(signatures, match.SignatureAccessions...)
		pfams = append(pfams, match.PfamAccessions...)
		totalCovered += match.CoverageLength
	}
	entry.Accessions = strings.Join(uniqueSorted(accessions), "; ")
	entry.EntryNames = strings.Join(uniquePreserveOrder(names), "; ")
	entry.EntryTypes = strings.Join(uniqueSorted(types), "; ")
	entry.MatchRegions = strings.Join(uniquePreserveOrder(regions), "; ")
	entry.SignatureAccessions = strings.Join(uniqueSorted(signatures), "; ")
	entry.PfamAccessions = strings.Join(uniqueSorted(pfams), "; ")
	if entry.ProteinLength > 0 && totalCovered > 0 {
		if totalCovered > entry.ProteinLength {
			totalCovered = entry.ProteinLength
		}
		entry.CoveragePercent = fmt.Sprintf("%.2f", float64(totalCovered)/float64(entry.ProteinLength)*100)
	}
}

func matchRegionsText(match Match) string {
	values := make([]string, 0, len(match.Regions))
	for _, region := range match.Regions {
		if region.Start <= 0 || region.End <= 0 {
			continue
		}
		value := strconv.Itoa(region.Start) + "-" + strconv.Itoa(region.End)
		if region.Status != "" {
			value += "(" + region.Status + ")"
		}
		values = append(values, value)
	}
	return strings.Join(values, ",")
}

type apiPage struct {
	Next     string      `json:"next"`
	Metadata apiMetadata `json:"metadata"`
	Results  []apiResult `json:"results"`
}

type apiMetadata struct {
	Accession string `json:"accession"`
	Length    int    `json:"length"`
}

type apiResult struct {
	Metadata apiEntryMetadata `json:"metadata"`
	Proteins []apiProtein     `json:"proteins"`
}

type apiEntryMetadata struct {
	Accession       string                         `json:"accession"`
	Name            string                         `json:"name"`
	SourceDatabase  string                         `json:"source_database"`
	Type            string                         `json:"type"`
	Integrated      string                         `json:"integrated"`
	MemberDatabases map[string]map[string]string  `json:"member_databases"`
}

type apiProtein struct {
	ProteinLength int           `json:"protein_length"`
	Locations     []apiLocation `json:"entry_protein_locations"`
}

type apiLocation struct {
	Fragments []apiFragment `json:"fragments"`
	Model     string        `json:"model"`
}

type apiFragment struct {
	Start    int    `json:"start"`
	End      int    `json:"end"`
	DCStatus string `json:"dc-status"`
}

func (result apiResult) toMatch() Match {
	match := Match{
		Accession:           strings.TrimSpace(result.Metadata.Accession),
		Name:                strings.TrimSpace(result.Metadata.Name),
		SourceDatabase:      strings.ToLower(strings.TrimSpace(result.Metadata.SourceDatabase)),
		Type:                strings.TrimSpace(result.Metadata.Type),
		IntegratedAccession: strings.TrimSpace(result.Metadata.Integrated),
	}
	signatures, pfams := memberAccessions(result.Metadata.MemberDatabases)
	match.SignatureAccessions = signatures
	match.PfamAccessions = pfams
	if strings.EqualFold(match.SourceDatabase, "pfam") && match.Accession != "" {
		match.PfamAccessions = append(match.PfamAccessions, match.Accession)
	}
	if match.IntegratedAccession != "" {
		match.SignatureAccessions = append(match.SignatureAccessions, match.IntegratedAccession)
	}
	for _, protein := range result.Proteins {
		for _, location := range protein.Locations {
			if location.Model != "" {
				match.SignatureAccessions = append(match.SignatureAccessions, location.Model)
				if strings.HasPrefix(strings.ToUpper(location.Model), "PF") {
					match.PfamAccessions = append(match.PfamAccessions, location.Model)
				}
			}
			for _, fragment := range location.Fragments {
				if fragment.Start <= 0 || fragment.End <= 0 {
					continue
				}
				start, end := fragment.Start, fragment.End
				if end < start {
					start, end = end, start
				}
				match.Regions = append(match.Regions, Region{Start: start, End: end, Status: strings.TrimSpace(fragment.DCStatus)})
				match.CoverageLength += end - start + 1
			}
		}
	}
	match.SignatureAccessions = uniqueSorted(match.SignatureAccessions)
	match.PfamAccessions = uniqueSorted(match.PfamAccessions)
	return match
}

func (result apiResult) firstProteinLength() int {
	for _, protein := range result.Proteins {
		if protein.ProteinLength > 0 {
			return protein.ProteinLength
		}
	}
	return 0
}

func memberAccessions(values map[string]map[string]string) ([]string, []string) {
	signatures := make([]string, 0)
	pfams := make([]string, 0)
	for source, entries := range values {
		for accession := range entries {
			accession = strings.TrimSpace(accession)
			if accession == "" {
				continue
			}
			signatures = append(signatures, accession)
			if strings.EqualFold(source, "pfam") || strings.HasPrefix(strings.ToUpper(accession), "PF") {
				pfams = append(pfams, accession)
			}
		}
	}
	return uniqueSorted(signatures), uniqueSorted(pfams)
}

func readDiskEntry(accession string) (Entry, bool) {
	path, err := diskEntryPath(accession)
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

func writeDiskEntry(accession string, entry Entry) {
	path, err := diskEntryPath(accession)
	if err != nil {
		return
	}
	data, err := json.MarshalIndent(diskEntry{Entry: entry}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

func diskEntryPath(accession string) (string, error) {
	dir, err := appfs.CacheDir("interpro", "protein")
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(accession))))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".json"), nil
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

func uniqueSorted(values []string) []string {
	out := uniquePreserveOrder(values)
	sort.Strings(out)
	return out
}

func uniquePreserveOrder(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

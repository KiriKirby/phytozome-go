package phytozome

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/KiriKirby/phytozome-go/internal/model"
	"golang.org/x/sync/singleflight"
)

const projectOverviewURL = "https://phytozome-next.jgi.doe.gov/api/content/project/phytozome"
const homePageURL = "https://phytozome-next.jgi.doe.gov/"

var (
	rowPattern          = regexp.MustCompile(`(?is)<tr>\s*<td><a href="/info/([^"]+)">(.+?)</a></td>\s*<td>(.*?)</td>\s*<td[^>]*>(.*?)</td>\s*</tr>`)
	tagPattern          = regexp.MustCompile(`(?is)<[^>]+>`)
	spacePattern        = regexp.MustCompile(`\s+`)
	searchNoisePattern  = regexp.MustCompile(`[^a-z0-9]+`)
	scriptSrcPattern    = regexp.MustCompile(`(?is)<script[^>]+src="([^"]+\.js)"`)
	targetRecordPattern = regexp.MustCompile(`\{"attributes":\{.*?\},"name":".*?","proteomeId":\d+,"taxId":".*?"\}`)
)

type projectSection struct {
	TypeName string `json:"typeName"`
	HTML     string `json:"html"`
}

type Client struct {
	baseHTTP *http.Client

	mu                     sync.RWMutex
	speciesCandidatesCache []model.SpeciesCandidate
	geneRecordCache        map[string]geneRecord
	proteinSequenceCache   map[string]string
	keywordRowsCache       map[string][]model.KeywordResultRow
	sf                     singleflight.Group
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseHTTP:             httpClient,
		geneRecordCache:      make(map[string]geneRecord),
		proteinSequenceCache: make(map[string]string),
		keywordRowsCache:     make(map[string][]model.KeywordResultRow),
	}
}

func (c *Client) Name() string {
	return "Phytozome"
}

func (c *Client) FetchSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {
	c.mu.RLock()
	if len(c.speciesCandidatesCache) > 0 {
		cached := append([]model.SpeciesCandidate(nil), c.speciesCandidatesCache...)
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	value, err, _ := c.sf.Do("species-candidates", func() (any, error) {
		c.mu.RLock()
		if len(c.speciesCandidatesCache) > 0 {
			cached := append([]model.SpeciesCandidate(nil), c.speciesCandidatesCache...)
			c.mu.RUnlock()
			return cached, nil
		}
		c.mu.RUnlock()

		type targetResult struct {
			targets map[string]targetRecord
			err     error
		}
		type releaseResult struct {
			releaseDates map[string]string
			err          error
		}

		targetCh := make(chan targetResult, 1)
		releaseCh := make(chan releaseResult, 1)

		go func() {
			targets, err := c.fetchTargetRecords(ctx)
			targetCh <- targetResult{targets: targets, err: err}
		}()
		go func() {
			releaseDates, err := c.fetchReleaseDates(ctx)
			releaseCh <- releaseResult{releaseDates: releaseDates, err: err}
		}()

		targetsResult := <-targetCh
		if targetsResult.err != nil {
			return nil, targetsResult.err
		}
		releaseResultValue := <-releaseCh
		if releaseResultValue.err != nil {
			return nil, releaseResultValue.err
		}

		candidates := candidatesFromTargets(targetsResult.targets, releaseResultValue.releaseDates)
		if len(candidates) == 0 {
			return nil, fmt.Errorf("no species candidates found in target records")
		}

		c.mu.Lock()
		c.speciesCandidatesCache = append([]model.SpeciesCandidate(nil), candidates...)
		c.mu.Unlock()

		return append([]model.SpeciesCandidate(nil), candidates...), nil
	})
	if err != nil {
		return nil, err
	}
	return value.([]model.SpeciesCandidate), nil
}

func (c *Client) fetchReleaseDates(ctx context.Context) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, projectOverviewURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.baseHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch project overview: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch project overview: unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read project overview: %w", err)
	}

	var sections []projectSection
	if err := json.Unmarshal(body, &sections); err != nil {
		return nil, fmt.Errorf("decode project overview: %w", err)
	}

	overviewHTML := ""
	for _, section := range sections {
		if section.TypeName == "overview" {
			overviewHTML = section.HTML
			break
		}
	}
	if overviewHTML == "" {
		return map[string]string{}, nil
	}

	return parseReleaseDates(overviewHTML), nil
}

func FilterSpeciesCandidates(candidates []model.SpeciesCandidate, keyword string) []model.SpeciesCandidate {
	keyword = strings.TrimSpace(strings.ToLower(keyword))
	if keyword == "" {
		return append([]model.SpeciesCandidate(nil), candidates...)
	}

	needleLoose := normalizeSearchLoose(keyword)
	needleTight := normalizeSearchTight(keyword)

	type scoredCandidate struct {
		candidate model.SpeciesCandidate
		score     int
	}

	filtered := make([]scoredCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		score, ok := speciesMatchScore(candidate, keyword, needleLoose, needleTight)
		if ok {
			filtered = append(filtered, scoredCandidate{candidate: candidate, score: score})
		}
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].score != filtered[j].score {
			return filtered[i].score < filtered[j].score
		}
		return strings.ToLower(filtered[i].candidate.DisplayLabel()) < strings.ToLower(filtered[j].candidate.DisplayLabel())
	})

	result := make([]model.SpeciesCandidate, 0, len(filtered))
	for _, item := range filtered {
		result = append(result, item.candidate)
	}
	return result
}

func speciesMatchScore(candidate model.SpeciesCandidate, rawKeyword, looseKeyword, tightKeyword string) (int, bool) {
	searchText := candidate.SearchText()
	if strings.Contains(searchText, rawKeyword) {
		return 0, true
	}

	parts := candidateSearchParts(candidate)
	best := 0
	found := false

	for _, part := range parts {
		loosePart := normalizeSearchLoose(part)
		tightPart := normalizeSearchTight(part)

		switch {
		case looseKeyword != "" && strings.HasPrefix(loosePart, looseKeyword):
			if !found || 1 < best {
				best = 1
			}
			found = true
		case tightKeyword != "" && strings.HasPrefix(tightPart, tightKeyword):
			if !found || 2 < best {
				best = 2
			}
			found = true
		case looseKeyword != "" && strings.Contains(loosePart, looseKeyword):
			if !found || 3 < best {
				best = 3
			}
			found = true
		case tightKeyword != "" && strings.Contains(tightPart, tightKeyword):
			if !found || 4 < best {
				best = 4
			}
			found = true
		}
	}

	return best, found
}

func candidateSearchParts(candidate model.SpeciesCandidate) []string {
	return []string{
		candidate.JBrowseName,
		candidate.GenomeLabel,
		candidate.CommonName,
		candidate.SearchAlias,
	}
}

func normalizeSearchLoose(value string) string {
	value = strings.ToLower(value)
	value = searchNoisePattern.ReplaceAllString(value, " ")
	value = spacePattern.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func normalizeSearchTight(value string) string {
	return strings.ReplaceAll(normalizeSearchLoose(value), " ", "")
}

func parseSpeciesCandidates(overviewHTML string, targets map[string]targetRecord) []model.SpeciesCandidate {
	return candidatesFromTargets(targets, parseReleaseDates(overviewHTML))
}

func parseReleaseDates(overviewHTML string) map[string]string {
	matches := rowPattern.FindAllStringSubmatch(overviewHTML, -1)
	releaseDates := make(map[string]string, len(matches))

	for _, match := range matches {
		if len(match) < 5 {
			continue
		}
		jbrowseName := cleanText(match[1])
		releaseDate := cleanText(match[4])
		if jbrowseName == "" || releaseDate == "" {
			continue
		}
		releaseDates[jbrowseName] = releaseDate
	}

	return releaseDates
}

func candidatesFromTargets(targets map[string]targetRecord, releaseDates map[string]string) []model.SpeciesCandidate {
	candidates := make([]model.SpeciesCandidate, 0, len(targets))

	for _, target := range targets {
		jbrowseName := cleanText(target.Attributes.JBrowseName)
		if jbrowseName == "" || target.ProteomeID == 0 {
			continue
		}

		searchAlias := strings.TrimSpace(target.Attributes.DisplayName + " " + target.Attributes.DisplayVersion)
		genomeLabel := searchAlias
		if genomeLabel == "" {
			genomeLabel = cleanText(target.Name)
		}

		candidate := model.SpeciesCandidate{
			ProteomeID:  target.ProteomeID,
			JBrowseName: jbrowseName,
			GenomeLabel: genomeLabel,
			CommonName:  cleanText(target.Attributes.CommonName),
			ReleaseDate: cleanText(releaseDates[jbrowseName]),
			SearchAlias: searchAlias,
		}
		if candidate.GenomeLabel == "" {
			continue
		}
		candidates = append(candidates, candidate)
	}

	sort.Slice(candidates, func(i, j int) bool {
		left := strings.ToLower(candidates[i].DisplayLabel())
		right := strings.ToLower(candidates[j].DisplayLabel())
		if left != right {
			return left < right
		}
		return candidates[i].ProteomeID < candidates[j].ProteomeID
	})

	return candidates
}

func cleanText(raw string) string {
	raw = tagPattern.ReplaceAllString(raw, " ")
	raw = html.UnescapeString(raw)
	raw = strings.ReplaceAll(raw, "\u00a0", " ")
	raw = spacePattern.ReplaceAllString(raw, " ")
	return strings.TrimSpace(raw)
}

type targetRecord struct {
	Attributes struct {
		CommonName     string `json:"commonName"`
		DisplayName    string `json:"displayName"`
		DisplayVersion string `json:"displayVersion"`
		JBrowseName    string `json:"jbrowseName"`
	} `json:"attributes"`
	Name       string `json:"name"`
	ProteomeID int    `json:"proteomeId"`
	TaxID      string `json:"taxId"`
}

func (c *Client) fetchTargetRecords(ctx context.Context) (map[string]targetRecord, error) {
	homePage, err := c.fetchHomePage(ctx)
	if err != nil {
		return nil, err
	}

	scriptURLs, err := extractBundleScriptURLs(homePage)
	if err != nil {
		return nil, err
	}

	var failures []string
	for _, scriptURL := range scriptURLs {
		bundle, err := c.fetchBundle(ctx, scriptURL)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}

		targets, err := extractTargetRecords(bundle)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", scriptURL, err))
			continue
		}
		if len(targets) > 0 {
			return targets, nil
		}
	}

	if len(failures) == 0 {
		return nil, fmt.Errorf("did not find target records in any homepage bundle")
	}
	return nil, fmt.Errorf("did not find target records in homepage bundles: %s", strings.Join(failures, "; "))
}

func (c *Client) fetchHomePage(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, homePageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create homepage request: %w", err)
	}

	resp, err := c.baseHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch homepage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch homepage: unexpected status %s", resp.Status)
	}

	homePage, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read homepage: %w", err)
	}
	return homePage, nil
}

func (c *Client) fetchBundle(ctx context.Context, scriptURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scriptURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create main bundle request: %w", err)
	}

	resp, err := c.baseHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch bundle %s: %w", scriptURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch bundle %s: unexpected status %s", scriptURL, resp.Status)
	}

	bundle, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read bundle %s: %w", scriptURL, err)
	}
	return bundle, nil
}

func extractBundleScriptURLs(homePage []byte) ([]string, error) {
	matches := scriptSrcPattern.FindAllSubmatch(homePage, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("could not find any JavaScript bundle paths in homepage")
	}

	seen := make(map[string]struct{}, len(matches))
	urls := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		path := strings.TrimSpace(string(match[1]))
		if path == "" || !strings.HasSuffix(path, ".js") {
			continue
		}
		if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
			path = "https://phytozome-next.jgi.doe.gov" + path
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		urls = append(urls, path)
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("could not build JavaScript bundle URLs from homepage")
	}

	slices.SortStableFunc(urls, compareBundlePriority)
	return urls, nil
}

func compareBundlePriority(a, b string) int {
	aMain := strings.Contains(strings.ToLower(a), "/main-")
	bMain := strings.Contains(strings.ToLower(b), "/main-")
	switch {
	case aMain && !bMain:
		return -1
	case !aMain && bMain:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

func extractTargetRecords(bundle []byte) (map[string]targetRecord, error) {
	targets := decodeTargetRecords(targetRecordPattern.FindAll(bundle, -1))
	if len(targets) > 0 {
		return targets, nil
	}

	targets = decodeTargetRecords(extractJSONObjectCandidates(bundle))
	if len(targets) > 0 {
		return targets, nil
	}

	return nil, fmt.Errorf("no valid target records parsed from bundle")
}

func decodeTargetRecords(chunks [][]byte) map[string]targetRecord {
	targets := make(map[string]targetRecord, len(chunks))
	for _, chunk := range chunks {
		var target targetRecord
		if err := json.Unmarshal(chunk, &target); err != nil {
			continue
		}
		if target.Attributes.JBrowseName == "" || target.ProteomeID == 0 {
			continue
		}
		targets[target.Attributes.JBrowseName] = target
	}
	return targets
}

func extractJSONObjectCandidates(bundle []byte) [][]byte {
	results := make([][]byte, 0, 64)
	for i := 0; i < len(bundle); i++ {
		if bundle[i] != '{' {
			continue
		}
		end, ok := findMatchingJSONObjectEnd(bundle, i)
		if !ok {
			continue
		}
		candidate := bundle[i : end+1]
		if !bytes.Contains(candidate, []byte(`"proteomeId"`)) || !bytes.Contains(candidate, []byte(`"jbrowseName"`)) {
			continue
		}
		results = append(results, candidate)
		i = end
	}
	return results
}

func findMatchingJSONObjectEnd(bundle []byte, start int) (int, bool) {
	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(bundle); i++ {
		ch := bundle[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
			if depth < 0 {
				return 0, false
			}
		}
	}
	return 0, false
}

func ParseProteomeID(value string) (int, error) {
	id, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("parse proteome id: %w", err)
	}
	if id <= 0 {
		return 0, fmt.Errorf("parse proteome id: invalid id %d", id)
	}
	return id, nil
}

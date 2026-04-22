package phytozome

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/wangsychn/phytozome-batch-cli/internal/model"
)

const projectOverviewURL = "https://phytozome-next.jgi.doe.gov/api/content/project/phytozome"

var (
	rowPattern   = regexp.MustCompile(`(?is)<tr>\s*<td><a href="/info/([^"]+)">(.+?)</a></td>\s*<td>(.*?)</td>\s*<td[^>]*>(.*?)</td>\s*</tr>`)
	tagPattern   = regexp.MustCompile(`(?is)<[^>]+>`)
	spacePattern = regexp.MustCompile(`\s+`)
)

type projectSection struct {
	TypeName string `json:"typeName"`
	HTML     string `json:"html"`
}

type Client struct {
	baseHTTP *http.Client
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseHTTP: httpClient}
}

func (c *Client) FetchSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {
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
		return nil, fmt.Errorf("project overview payload did not contain overview HTML")
	}

	candidates := parseSpeciesCandidates(overviewHTML)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no species candidates found in overview HTML")
	}

	return candidates, nil
}

func FilterSpeciesCandidates(candidates []model.SpeciesCandidate, keyword string) []model.SpeciesCandidate {
	keyword = strings.TrimSpace(strings.ToLower(keyword))
	if keyword == "" {
		return append([]model.SpeciesCandidate(nil), candidates...)
	}

	filtered := make([]model.SpeciesCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.Contains(candidate.SearchText(), keyword) {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func parseSpeciesCandidates(overviewHTML string) []model.SpeciesCandidate {
	matches := rowPattern.FindAllStringSubmatch(overviewHTML, -1)
	seen := make(map[string]struct{}, len(matches))
	candidates := make([]model.SpeciesCandidate, 0, len(matches))

	for _, match := range matches {
		if len(match) < 5 {
			continue
		}

		candidate := model.SpeciesCandidate{
			JBrowseName: cleanText(match[1]),
			GenomeLabel: cleanText(match[2]),
			CommonName:  cleanText(match[3]),
			ReleaseDate: cleanText(match[4]),
		}
		if candidate.JBrowseName == "" || candidate.GenomeLabel == "" {
			continue
		}
		if _, exists := seen[candidate.JBrowseName]; exists {
			continue
		}
		seen[candidate.JBrowseName] = struct{}{}
		candidates = append(candidates, candidate)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return strings.ToLower(candidates[i].GenomeLabel) < strings.ToLower(candidates[j].GenomeLabel)
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

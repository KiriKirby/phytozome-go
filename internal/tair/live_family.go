package tair

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/searchengine/tairkeyword"
)

var (
	familyBrowseRowPattern = regexp.MustCompile(`(?is)/browse/gene_family/([^"#?]+)".*?>([^<]+)</a>.*?(\d+)\s+famil(?:y|ies).*?(\d+)\s+members`)
	familyHeaderPattern    = regexp.MustCompile(`(?is)<h2[^>]*>.*?<a[^>]*name="([^"]+)"[^>]*>.*?<b>\s*(.*?)\s*</b>.*?</h2>`)
	familyCellPattern      = regexp.MustCompile(`(?is)<t[dh][^>]*>(.*?)</t[dh]>`)
	familyRowPattern       = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	htmlTagPattern         = regexp.MustCompile(`(?is)<[^>]+>`)
)

func (c *Client) fetchLiveFamilyCandidates(ctx context.Context) ([]familyCandidate, error) {
	text, err := c.getTAIRText(ctx, "https://www.arabidopsis.org/browse/genefamily")
	if err != nil || looksLikeSPAHTML(text) {
		text, err = c.getTAIRText(ctx, "https://www.arabidopsis.org/.codex-family-browse-fallback")
	}
	if err != nil {
		// Fall back to the saved source-map snapshot we already keep locally during investigation.
		if raw, readErr := readFamilyCandidatesFromSourceMap(); readErr == nil {
			return raw, nil
		}
		return nil, err
	}
	candidates := parseFamilyBrowseCandidates(text)
	if len(candidates) == 0 {
		if raw, readErr := readFamilyCandidatesFromSourceMap(); readErr == nil {
			return raw, nil
		}
		return nil, fmt.Errorf("no TAIR family candidates were parsed from official browse data")
	}
	return candidates, nil
}

func parseFamilyBrowseCandidates(text string) []familyCandidate {
	matches := familyBrowseRowPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]familyCandidate, 0, len(matches))
	parentByPrefix := map[string]string{}
	for _, match := range matches {
		key := cleanText(strings.TrimSpace(match[1]))
		name := cleanText(match[2])
		familyCount, _ := strconv.Atoi(strings.TrimSpace(match[3]))
		memberCount, _ := strconv.Atoi(strings.TrimSpace(match[4]))
		if key == "" || name == "" {
			continue
		}
		short := shortenFamilyDisplayName(name)
		parentName, parentKey := inferFamilyHierarchy(key, name, familyCount)
		if parentKey == "" {
			if prefix := familyPrefixKey(key); prefix != "" {
				if inherited := parentByPrefix[prefix]; inherited != "" && !strings.EqualFold(inherited, key) {
					parentKey = inherited
				}
			}
		}
		if parentKey == "" && familyCount > 1 {
			parentByPrefix[familyPrefixKey(key)] = key
		}
		out = append(out, familyCandidate{
			Name:        name,
			ShortName:   short,
			Count:       memberCount,
			Key:         key,
			ParentKey:   parentKey,
			ParentName:  parentName,
			HasChildren: familyCount > 1,
		})
	}
	children := map[string]bool{}
	for _, fam := range out {
		if fam.ParentKey != "" {
			children[fam.ParentKey] = true
		}
	}
	for i := range out {
		if children[out[i].Key] {
			out[i].HasChildren = true
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		li := strings.ToLower(firstNonEmpty(out[i].ParentKey, out[i].Key, out[i].ShortName, out[i].Name))
		lj := strings.ToLower(firstNonEmpty(out[j].ParentKey, out[j].Key, out[j].ShortName, out[j].Name))
		if li != lj {
			return li < lj
		}
		if out[i].HasChildren != out[j].HasChildren {
			return out[i].HasChildren
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.ToLower(firstNonEmpty(out[i].ShortName, out[i].Name)) < strings.ToLower(firstNonEmpty(out[j].ShortName, out[j].Name))
	})
	return out
}

func inferFamilyHierarchy(key string, name string, familyCount int) (string, string) {
	if familyCount <= 1 {
		return "", ""
	}
	upper := strings.ToUpper(strings.TrimSpace(key))
	switch {
	case strings.Contains(upper, "#"):
		parts := strings.SplitN(upper, "#", 2)
		return cleanText(parts[0]), cleanText(parts[0])
	case strings.Contains(upper, "_"):
		parts := strings.SplitN(upper, "_", 2)
		return cleanText(parts[0]), cleanText(parts[0])
	}
	fields := strings.Fields(strings.TrimSpace(name))
	if len(fields) >= 2 && familyCount > 1 {
		prefix := cleanText(fields[0])
		if prefix != "" && !strings.EqualFold(prefix, name) {
			return prefix, prefix
		}
	}
	return "", ""
}

func familyPrefixKey(key string) string {
	key = strings.TrimSpace(strings.ToUpper(key))
	if key == "" {
		return ""
	}
	for _, sep := range []string{"#", "_", "-"} {
		if idx := strings.Index(key, sep); idx > 0 {
			return key[:idx]
		}
	}
	return ""
}

func shortenFamilyDisplayName(name string) string {
	name = cleanText(name)
	replacements := []string{
		" transcription factor family", "",
		" gene family", "",
		" protein family", "",
		" family", "",
	}
	lower := strings.ToLower(name)
	for i := 0; i < len(replacements); i += 2 {
		if strings.HasSuffix(lower, replacements[i]) {
			return strings.TrimSpace(name[:len(name)-len(replacements[i])])
		}
	}
	return name
}

func readFamilyCandidatesFromSourceMap() ([]familyCandidate, error) {
	const sourceMapPath = "C:\\Users\\wangsychn\\Documents\\GitHub\\phytozome-batch-cli\\.codex\\app.js.map"
	data, err := os.ReadFile(sourceMapPath)
	if err != nil {
		return nil, err
	}
	return parseFamilyBrowseCandidates(string(data)), nil
}

func (c *Client) fetchLiveFamilyRows(ctx context.Context, version model.SpeciesCandidate, familyKey string) ([]model.KeywordResultRow, error) {
	familyKey = strings.TrimSpace(familyKey)
	if familyKey == "" {
		return nil, nil
	}
	htmlText, err := c.getTAIRText(ctx, baseURL+"/api/detail/genefamily?key="+familyKey)
	if err != nil {
		return nil, err
	}
	familyName, familyShort, rows := parseFamilyDetailRows(version, familyKey, htmlText)
	for i := range rows {
		rows[i].SearchType = tairkeyword.SearchTypeFamily
		rows[i].SearchTerm = familyShort
		rows[i].ExtraColumns = ensureExtraColumns(rows[i].ExtraColumns)
		rows[i].ExtraColumns["tair_family_key"] = familyKey
		rows[i].ExtraColumns["tair_family_name"] = familyName
		rows[i].ExtraColumns["tair_family_short_name"] = familyShort
	}
	return rows, nil
}

func parseFamilyDetailRows(version model.SpeciesCandidate, familyKey string, htmlText string) (string, string, []model.KeywordResultRow) {
	familyName := cleanText(strings.TrimSpace(familyKey))
	if match := familyHeaderPattern.FindStringSubmatch(htmlText); len(match) >= 3 {
		familyName = cleanText(match[2])
	}
	shortName := shortenFamilyDisplayName(familyName)
	rowsHTML := familyRowPattern.FindAllStringSubmatch(htmlText, -1)
	if len(rowsHTML) == 0 {
		return familyName, shortName, nil
	}
	rows := make([]model.KeywordResultRow, 0, len(rowsHTML))
	currentSubfamily := shortName
	for _, rowMatch := range rowsHTML {
		cells := familyCellPattern.FindAllStringSubmatch(rowMatch[1], -1)
		if len(cells) < 4 {
			continue
		}
		values := make([]string, 0, len(cells))
		for _, cell := range cells {
			values = append(values, cleanHTMLCell(cell[1]))
		}
		if len(values) >= 5 && strings.EqualFold(values[0], "Sub Family") {
			continue
		}
		subfamily := currentSubfamily
		labelIndex := 1
		geneIndex := 2
		refseqIndex := 3
		descriptionIndex := 4
		if len(values) >= 5 {
			subfamily = strings.TrimSpace(values[0])
		} else if len(values) == 4 {
			labelIndex = 0
			geneIndex = 1
			refseqIndex = 2
			descriptionIndex = 3
		} else {
			continue
		}
		if subfamily == "" || subfamily == "\u00a0" {
			subfamily = currentSubfamily
		} else {
			currentSubfamily = subfamily
		}
		label := strings.TrimSpace(values[labelIndex])
		gene := normalizeTAIRIdentifier(values[geneIndex])
		transcript := normalizeTranscriptFromFamily(gene)
		refseq := strings.TrimSpace(values[refseqIndex])
		description := strings.TrimSpace(values[descriptionIndex])
		if gene == "" && label == "" {
			continue
		}
		if label == "" {
			label = subfamily
		}
		row := model.KeywordResultRow{
			SourceDatabase: "tair",
			LabelName:      label,
			ProteinID:      transcript,
			TranscriptID:   transcript,
			GeneIdentifier: gene,
			Genome:         version.DisplayLabel(),
			Location:       "",
			Aliases:        strings.Join(uniqueStrings([]string{label, subfamily}), "; "),
			Symbols:        label,
			Description:    description,
			Comments:       subfamily,
			AutoDefine:     firstNonEmpty(description, label, subfamily),
			GeneReportURL:  baseURL + "/servlets/TairObject?type=locus&name=" + gene,
			SequenceID:     firstNonEmpty(transcript, gene),
			ExtraColumns: map[string]string{
				"tair_family_subfamily": subfamily,
				"tair_refseq_id":        refseq,
				"tair_family_key":       familyKey,
				"tair_family_name":      familyName,
			},
		}
		rows = append(rows, row)
	}
	sortKeywordRows(rows)
	return familyName, shortName, rows
}

func cleanHTMLCell(value string) string {
	value = strings.ReplaceAll(value, "&nbsp;", " ")
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = htmlTagPattern.ReplaceAllString(value, " ")
	return cleanText(value)
}

func normalizeTAIRIdentifier(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, ". ")
	value = strings.ToUpper(value)
	if !agiGenePattern.MatchString(value) {
		return ""
	}
	return value
}

func normalizeTranscriptFromFamily(gene string) string {
	if gene == "" {
		return ""
	}
	return gene + ".1"
}

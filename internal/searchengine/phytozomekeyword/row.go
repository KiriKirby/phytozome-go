package phytozomekeyword

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

var (
	ecNumberLikeLabelPattern      = regexp.MustCompile(`^(?:EC[:\-]?)?[A-Za-z]?\d+(?:\.\d+){2,3}$`)
	arabidopsisGeneIDLabelPattern = regexp.MustCompile(`(?i)^AT[1-5MC]G\d{5}(?:\.\d+)?$`)
	lemnaGeneIDLabelPattern       = regexp.MustCompile(`(?i)^SP\d{4}D\d{3}G\d{6}(?:_T\d+)?$`)
)

func buildKeywordResultRow(searchTerm string, searchType string, species model.SpeciesCandidate, gene GeneRecord) (model.KeywordResultRow, error) {
	transcript, err := gene.PrimaryTranscript("")
	if err != nil {
		return model.KeywordResultRow{}, err
	}

	geneID := strings.TrimSpace(gene.PrimaryIdentifier)
	internalID := strings.TrimSpace(gene.ID)
	geneIdentifier := geneID
	if internalID != "" {
		geneIdentifier += " (" + internalID + ")"
	}

	aliases := dedupePreserveOrder(append(copyStringSlice(gene.Symbols), gene.Synonyms...))
	labelName := bestAlias(strings.Join(aliases, "; "))
	uniprotValues := make([]string, 0, len(transcript.Uniprot))
	for _, value := range transcript.Uniprot {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		uniprotValues = append(uniprotValues, transcript.PrimaryIdentifier+": "+value)
	}

	return model.KeywordResultRow{
		SourceDatabase:      "phytozome",
		SearchTerm:          searchTerm,
		SearchType:          searchType,
		LabelName:           labelName,
		TranscriptID:        strings.TrimSpace(transcript.PrimaryIdentifier),
		GeneIdentifier:      geneIdentifier,
		Genome:              formatKeywordGenome(gene),
		Location:            formatKeywordLocation(gene),
		Aliases:             strings.Join(aliases, "; "),
		UniProt:             strings.Join(uniprotValues, "; "),
		Description:         firstNonEmpty(gene.Deflines...),
		Comments:            strings.Join(gene.Comments, "\n"),
		AutoDefine:          strings.TrimSpace(gene.AutoDefline),
		GeneReportURL:       fmt.Sprintf("https://phytozome-next.jgi.doe.gov/report/gene/%s/%s", species.JBrowseName, geneID),
		SequenceHeaderLabel: strings.TrimSpace(gene.Organism.OrganismName + " " + gene.Organism.AnnotationVersion),
		SequenceID:          strings.TrimSpace(transcript.SecondaryIdentifier),
	}, nil
}

func formatKeywordGenome(gene GeneRecord) string {
	organism := strings.TrimSpace(gene.Organism.OrganismName)
	annotation := strings.TrimSpace(gene.Organism.AnnotationVersion)
	proteome := gene.Organism.Proteome
	taxID := strings.TrimSpace(gene.Organism.TaxID)

	parts := make([]string, 0, 2)
	if organism != "" || annotation != "" {
		parts = append(parts, strings.TrimSpace(organism+" "+annotation))
	}
	details := make([]string, 0, 2)
	if proteome != 0 {
		details = append(details, fmt.Sprintf("Phytozome genome ID: %d", proteome))
	}
	if taxID != "" {
		details = append(details, "NCBI taxonomy ID: "+taxID)
	}
	if len(details) > 0 {
		parts = append(parts, "("+strings.Join(details, "; ")+")")
	}
	return strings.Join(parts, " ")
}

func formatKeywordLocation(gene GeneRecord) string {
	strand := "forward"
	if strings.TrimSpace(gene.Strand) == "-1" {
		strand = "reverse"
	}
	return fmt.Sprintf("%s:%s..%s %s", strings.TrimSpace(gene.Scaffold), strings.TrimSpace(gene.Start), strings.TrimSpace(gene.End), strand)
}

func dedupePreserveOrder(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
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
		result = append(result, value)
	}
	return result
}

func bestAlias(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ';' || r == ',' || r == '|' || r == '\t' || r == '\n' || r == '\r'
	})
	best := ""
	bestScore := -1
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		score := aliasPreferenceScore(part)
		if score > bestScore || (score == bestScore && len(part) < len(best)) {
			best = part
			bestScore = score
		}
	}
	return best
}

func BestQuerySourceLabel(aliases string, autoDefine string) string {
	candidates := querySourceAliasCandidates(aliases)
	if label := labelFromAutoDefine(autoDefine); label != "" {
		candidates = append(candidates, label)
	}
	best := ""
	bestScore := -1
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || looksLikeECNumberLabel(candidate) || looksLikeDatabaseIdentifierLabel(candidate) {
			continue
		}
		score := aliasPreferenceScore(candidate) + querySourceLabelPreferenceBonus(candidate)
		score -= lowercaseCount(candidate) * 6
		if strings.Contains(candidate, ".") {
			score -= strings.Count(candidate, ".") * 8
		}
		if score > bestScore || (score == bestScore && len(candidate) < len(best)) {
			best = candidate
			bestScore = score
		}
	}
	if bestScore < 22 {
		return ""
	}
	return best
}

func querySourceAliasCandidates(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ';' || r == ',' || r == '|' || r == '\t' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := strings.ToUpper(part)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, part)
	}
	return out
}

func querySourceLabelPreferenceBonus(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	score := 0
	upper := strings.ToUpper(value)
	if looksLikePrimaryFamilySymbol(upper) {
		score += 30
	}
	if strings.HasPrefix(upper, "AT") && len(value) > 4 {
		score -= 8
	}
	return score
}

func looksLikePrimaryFamilySymbol(value string) bool {
	if value == "" {
		return false
	}
	hasLetter := false
	hasDigit := false
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			return false
		}
	}
	return hasLetter && hasDigit
}

func aliasPreferenceScore(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return -1
	}
	score := 0
	hasLetter := false
	hasDigit := false
	upperCount := 0
	lowerCount := 0
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z':
			hasLetter = true
			upperCount++
			score += 2
		case r >= 'a' && r <= 'z':
			hasLetter = true
			lowerCount++
			score += 1
		case r >= '0' && r <= '9':
			hasDigit = true
			score += 1
		case r == '-' || r == '\'':
			score += 1
		case r == '_' || r == '/' || r == '.':
			score -= 2
		case r == ' ' || r == '\t':
			score -= 8
		default:
			score -= 4
		}
	}
	upper := strings.ToUpper(value)
	switch {
	case strings.HasPrefix(upper, "AT") && hasDigit:
		score -= 12
	case strings.HasPrefix(upper, "CYP") && hasDigit:
		score -= 10
	case strings.HasPrefix(upper, "REF") && hasDigit:
		score -= 6
	}
	if hasLetter && hasDigit {
		score += 8
	}
	if noLowercase(value) && len(value) <= 4 {
		score += 8
	}
	if aliasHasInternalDigitPattern(value) {
		score += 2
	}
	if upperCount > 0 && lowerCount == 0 {
		score += 4
	}
	if len(value) <= 8 {
		score += 6
	} else if len(value) <= 12 {
		score += 2
	} else {
		score -= len(value) - 12
	}
	return score
}

func noLowercase(value string) bool {
	for _, r := range value {
		if r >= 'a' && r <= 'z' {
			return false
		}
	}
	return true
}

func aliasHasInternalDigitPattern(value string) bool {
	seenDigit := false
	seenLetterAfterDigit := false
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			seenDigit = true
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'):
			if seenDigit {
				seenLetterAfterDigit = true
			}
		}
	}
	if !seenLetterAfterDigit {
		return false
	}
	last := rune(value[len(value)-1])
	return last >= '0' && last <= '9'
}

func lowercaseCount(value string) int {
	count := 0
	for _, r := range value {
		if r >= 'a' && r <= 'z' {
			count++
		}
	}
	return count
}

func looksLikeECNumberLabel(value string) bool {
	return ecNumberLikeLabelPattern.MatchString(strings.TrimSpace(value))
}

func looksLikeDatabaseIdentifierLabel(value string) bool {
	value = strings.TrimSpace(value)
	switch {
	case value == "":
		return false
	case strings.HasPrefix(strings.ToUpper(value), "PAC:"):
		return true
	case arabidopsisGeneIDLabelPattern.MatchString(value):
		return true
	case lemnaGeneIDLabelPattern.MatchString(value):
		return true
	default:
		return false
	}
}

func labelFromAutoDefine(value string) string {
	best := ""
	bestScore := -1
	for _, candidate := range autoDefineCandidates(value) {
		score := autoDefineLabelScore(candidate)
		if score > bestScore || (score == bestScore && len(candidate) < len(best)) {
			best = candidate
			bestScore = score
		}
	}
	return best
}

func autoDefineCandidates(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '(' || r == ')' || r == ',' || r == ';' || r == '/' || r == '\t' || r == '\r' || r == '\n'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if !looksLikeAliasToken(part) {
			continue
		}
		key := strings.ToUpper(part)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, part)
	}
	return out
}

func autoDefineLabelScore(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return -1
	}
	score := aliasPreferenceScore(value)
	if strings.Contains(value, "'") {
		score += 10
	}
	if len(value) <= 4 {
		score += 12
	} else if len(value) <= 6 {
		score += 8
	} else if len(value) <= 8 {
		score += 4
	} else {
		score -= len(value) - 8
	}
	upper := strings.ToUpper(value)
	if strings.HasPrefix(upper, "CYP") && len(value) > 5 {
		score -= 8
	}
	return score
}

func looksLikeAliasToken(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 16 {
		return false
	}
	hasLetter := false
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
			hasLetter = true
		case r >= '0' && r <= '9', r == '-', r == '\'', r == '.':
		default:
			return false
		}
	}
	return hasLetter
}

func CopyStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func copyStringSlice(values []string) []string {
	return CopyStringSlice(values)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

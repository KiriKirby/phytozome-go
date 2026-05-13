// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package labelname

import (
	"regexp"
	"sort"
	"strings"
)

var (
	ecNumberLikePattern = regexp.MustCompile(`^(?:EC[:\-]?)?[A-Za-z]?\d+(?:\.\d+){2,3}$`)
	lemnaGeneIDPattern  = regexp.MustCompile(`(?i)^SP\d{4}D\d{3}G\d{6}(?:_T\d+)?$`)
)

type AliasRankRequest struct {
	TaskTimestamp string
	ItemIndex     int
	SearchTerm    string
	ProteinID     string
	GeneID        string
	TranscriptID  string
	SequenceID    string
	Aliases       []string
}

type AliasRankResult struct {
	TaskTimestamp string
	ItemIndex     int
	RankedAliases []string
}

func RankAliases(request AliasRankRequest) AliasRankResult {
	return AliasRankResult{
		TaskTimestamp: request.TaskTimestamp,
		ItemIndex:     request.ItemIndex,
		RankedAliases: rankAliasRequest(request),
	}
}

func RankAliasBatch(requests []AliasRankRequest) []AliasRankResult {
	if len(requests) == 0 {
		return nil
	}
	results := make([]AliasRankResult, len(requests))
	cache := make(map[string][]string, len(requests))
	for i, request := range requests {
		key := aliasRankCacheKey(request)
		if ranked, ok := cache[key]; ok {
			results[i] = AliasRankResult{
				TaskTimestamp: request.TaskTimestamp,
				ItemIndex:     request.ItemIndex,
				RankedAliases: append([]string(nil), ranked...),
			}
			continue
		}
		result := AliasRankResult{
			TaskTimestamp: request.TaskTimestamp,
			ItemIndex:     request.ItemIndex,
			RankedAliases: rankAliasRequest(request),
		}
		cache[key] = append([]string(nil), result.RankedAliases...)
		results[i] = result
	}
	return results
}

func rankAliasRequest(request AliasRankRequest) []string {
	fallback := uniqueStrings([]string{
		request.ProteinID,
		request.TranscriptID,
		request.GeneID,
		request.SequenceID,
	})
	return rankAliasCandidates(request.Aliases, fallback)
}

func aliasRankCacheKey(request AliasRankRequest) string {
	values := make([]string, 0, len(request.Aliases)+4)
	for _, value := range request.Aliases {
		if normalized := normalizeAliasKey(value); normalized != "" {
			values = append(values, normalized)
		}
	}
	for _, value := range []string{request.ProteinID, request.TranscriptID, request.GeneID, request.SequenceID} {
		if normalized := normalizeAliasKey(value); normalized != "" {
			values = append(values, normalized)
		}
	}
	return strings.Join(values, "\x00")
}

func RankedAliases(aliases []string) []string {
	return rankAliasCandidates(aliases, nil)
}

func rankAliasCandidates(aliases []string, fallback []string) []string {
	aliases = uniqueStrings(aliases)
	trusted := make([]string, 0, len(aliases))
	untrusted := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		if IsTrustedCandidate(alias) {
			trusted = append(trusted, alias)
		} else {
			untrusted = append(untrusted, alias)
		}
	}
	sortAliasRank(trusted)
	sortAliasRank(untrusted)
	out := make([]string, 0, len(trusted)+len(fallback)+len(untrusted))
	out = append(out, trusted...)
	out = append(out, fallback...)
	out = append(out, untrusted...)
	return uniqueStrings(out)
}

func sortAliasRank(aliases []string) {
	type aliasRankItem struct {
		text  string
		key   string
		score int
	}
	items := make([]aliasRankItem, 0, len(aliases))
	peers := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		trimmed := strings.TrimSpace(alias)
		if trimmed == "" {
			continue
		}
		items = append(items, aliasRankItem{text: trimmed, key: normalizeAliasKey(trimmed)})
		peers = append(peers, trimmed)
	}
	for i := range items {
		items[i].score = AliasPreferenceScore(items[i].text) +
			QueryAliasPrimarySymbolBonus(items[i].text) +
			aliasRedundantLongFormPenalty(items[i].text, peers)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		return len(items[i].text) < len(items[j].text)
	})
	for i := range items {
		aliases[i] = items[i].text
	}
}

func aliasScores(aliases []string) map[string]int {
	scores := make(map[string]int, len(aliases))
	for _, alias := range aliases {
		trimmed := strings.TrimSpace(alias)
		if trimmed == "" {
			continue
		}
		key := normalizeAliasKey(trimmed)
		if _, ok := scores[key]; ok {
			continue
		}
		scores[key] = AliasPreferenceScore(trimmed) + QueryAliasPrimarySymbolBonus(trimmed) + aliasRedundantLongFormPenalty(trimmed, aliases)
	}
	return scores
}

func FastaHeaderLabelNameFromInput(input string) string {
	return FastaHeaderLabelName(firstFastaHeaderLine(input))
}

func FastaHeaderLabelName(header string) string {
	return ParentheticalHeaderLabel(header)
}

func ParentheticalHeaderLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	open := strings.LastIndex(value, " (")
	if open < 0 {
		return ""
	}
	rest := value[open+2:]
	closeIndex := strings.Index(rest, ")")
	if closeIndex < 0 {
		return ""
	}
	label := strings.TrimSpace(rest[:closeIndex])
	if label == "" {
		return ""
	}
	for _, ch := range label {
		if ch == ' ' || ch == '\t' {
			return ""
		}
	}
	return label
}

func SplitAliases(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ';' || r == ',' || r == '|' || r == '\t' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func FirstAlias(value string) string {
	for _, part := range SplitAliases(value) {
		return part
	}
	return ""
}

func BestAlias(value string) string {
	best := ""
	bestScore := -1
	parts := SplitAliases(value)
	scores := aliasScores(parts)
	for _, part := range parts {
		if part == "" || !IsTrustedCandidate(part) {
			continue
		}
		score := scores[normalizeAliasKey(part)]
		if score > bestScore || (score == bestScore && len(part) < len(best)) {
			best = part
			bestScore = score
		}
	}
	return best
}

func TrustedLabel(candidates ...string) string {
	best := ""
	bestScore := -1
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || !IsTrustedCandidate(candidate) {
			continue
		}
		score := TrustedLabelScore(candidate)
		if score > bestScore || (score == bestScore && len(candidate) < len(best)) {
			best = candidate
			bestScore = score
		}
	}
	return best
}

func aliasRedundantLongFormPenalty(candidate string, peers []string) int {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return 0
	}
	candidateUpper := strings.ToUpper(candidate)
	for _, peer := range peers {
		peer = strings.TrimSpace(peer)
		if peer == "" || strings.EqualFold(peer, candidate) {
			continue
		}
		peerUpper := strings.ToUpper(peer)
		if len(candidateUpper) > len(peerUpper)+1 &&
			strings.HasSuffix(candidateUpper, peerUpper) &&
			strings.TrimSpace(peer) != "" &&
			looksLikePrimaryAliasSymbol(peerUpper) &&
			looksLikePrimaryAliasSymbol(candidateUpper) {
			return -6
		}
	}
	return 0
}

func IsTrustedCandidate(value string) bool {
	return TrustedLabelScore(value) >= 12
}

func TrustedLabelScore(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return -1
	}
	if LooksLikeECNumber(value) {
		return -100
	}
	if LooksLikeDatabaseIdentifier(value) {
		return -80
	}
	score := AliasPreferenceScore(value)
	hasDigit := false
	letterCount := 0
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'):
			letterCount++
		}
	}
	switch lowerCount := lowercaseCount(value); {
	case lowerCount >= 3:
		score -= 16
	case lowerCount == 2:
		score -= 8
	case lowerCount == 1:
		score -= 2
	}
	if strings.ContainsAny(value, "._:/") {
		score -= 4
	}
	if strings.Contains(value, ".") {
		score -= strings.Count(value, ".") * 8
	}
	if strings.Contains(value, "'") {
		score += 6
	}
	if !hasDigit {
		switch {
		case letterCount > 8:
			score -= 24
		case letterCount > 6:
			score -= 14
		case letterCount > 4:
			score -= 6
		}
	}
	return score
}

func AliasPreferenceScore(value string) int {
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
	case strings.HasPrefix(upper, "CYP") && hasDigit:
		score -= 6
	case strings.HasPrefix(upper, "REF") && hasDigit:
		score -= 6
	}
	if hasLetter && hasDigit {
		score += 8
	}
	if strings.Contains(value, "'") {
		score += 8
	}
	if aliasHasInternalDigitPattern(value) {
		score += 6
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

func QueryAliasPrimarySymbolBonus(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	upper := strings.ToUpper(value)
	score := 0
	if looksLikePrimaryAliasSymbol(upper) {
		score += 30
	}
	return score
}

func LabelFromAutoDefine(value string) string {
	best := ""
	bestScore := -1
	for _, candidate := range AutoDefineCandidates(value) {
		score := AutoDefineLabelScore(candidate)
		if score > bestScore || (score == bestScore && len(candidate) < len(best)) {
			best = candidate
			bestScore = score
		}
	}
	return best
}

func AutoDefineCandidates(value string) []string {
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
	sort.SliceStable(out, func(i, j int) bool {
		left := AliasPreferenceScore(out[i])
		right := AliasPreferenceScore(out[j])
		if left != right {
			return left > right
		}
		return len(out[i]) < len(out[j])
	})
	return out
}

func AutoDefineLabelScore(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return -1
	}
	score := AliasPreferenceScore(value)
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
	if strings.HasPrefix(upper, "CYP") && len(value) > 6 {
		score -= 8
	}
	return score
}

func LooksLikeECNumber(value string) bool {
	return ecNumberLikePattern.MatchString(strings.TrimSpace(value))
}

func LooksLikeDatabaseIdentifier(value string) bool {
	value = strings.TrimSpace(value)
	switch {
	case value == "":
		return false
	case strings.HasPrefix(strings.ToUpper(value), "PAC:"):
		return true
	case lemnaGeneIDPattern.MatchString(value):
		return true
	default:
		return false
	}
}

func firstFastaHeaderLine(input string) string {
	value := strings.TrimSpace(input)
	if value == "" || !strings.HasPrefix(value, ">") {
		return ""
	}
	value = strings.ReplaceAll(value, "\r", "")
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ">") {
			return strings.TrimSpace(strings.TrimPrefix(line, ">"))
		}
		return ""
	}
	return ""
}

func querySourceLabelPreferenceBonus(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	score := 0
	upper := strings.ToUpper(value)
	if looksLikePrimaryAliasSymbol(upper) {
		score += 30
	}
	return score
}

func looksLikePrimaryAliasSymbol(value string) bool {
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

func aliasHasInternalDigitPattern(value string) bool {
	if value == "" {
		return false
	}
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

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := normalizeAliasKey(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeAliasKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
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

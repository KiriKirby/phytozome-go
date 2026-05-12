// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package lemnakeyword

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/KiriKirby/phytozome-go/internal/model"
	"golang.org/x/sync/singleflight"
)

const (
	SearchTypeReportURL        = "report URL: gene"
	SearchTypeGeneID           = "lemna gene identifier"
	SearchTypeTranscriptID     = "lemna transcript identifier"
	SearchTypeLabelSymbol      = "lemna label symbol"
	SearchTypeRiceLocus        = "rice LOC_Os locus"
	SearchTypeRefSeqProtein    = "RefSeq XP protein"
	SearchTypeGeneAlias        = "gene alias / symbol"
	SearchTypeCytochromeFamily = "CYP73 family symbol"
	SearchTypeIdentifier       = "lemna identifier"
	SearchTypeKeyword          = "keyword"
	SearchTypeWide             = "wide search"
	SearchTypeBroad            = "broad search"
)

const fallbackSuffix = " (fallback to wide search)"

const cacheSchemaVersion = "lemnakeyword-v4"

const (
	identifierKindAny        = "any"
	identifierKindGene       = "gene"
	identifierKindTranscript = "transcript"
)

var (
	lemnaTranscriptPattern = regexp.MustCompile(`(?i)^[A-Z]{2}\d{4}D\d{3}G\d{6}_T\d+$`)
	lemnaGenePattern       = regexp.MustCompile(`(?i)^[A-Z]{2}\d{4}D\d{3}G\d{6}$`)
	labelSymbolPattern     = regexp.MustCompile(`\b[A-Z][A-Z0-9-]{1,14}\b`)
	riceLocusPattern       = regexp.MustCompile(`(?i)^(?:LOC_)?(?:OS)?\d{2}G\d{5}(?:\.\d+)?$`)
	refSeqProteinPattern   = regexp.MustCompile(`(?i)^(?:XP_?)\d+(?:\.\d+)?$`)
	cytochromeP450Pattern  = regexp.MustCompile(`(?i)^CYP\d+[A-Z]\d+$`)
)

type KeywordFinder interface {
	SearchKeywordRowsByReportURL(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error)
	SearchKeywordRowsByIdentifier(ctx context.Context, species model.SpeciesCandidate, term string, kind string, limit int) ([]model.KeywordResultRow, error)
	SearchKeywordRowsByLabel(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error)
	SearchKeywordRowsByKeywordText(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error)
	SearchKeywordRowsByWideText(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error)
	SearchKeywordRowsByBroadText(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error)
}

type Engine struct {
	finder KeywordFinder
	mu     sync.RWMutex
	cache  map[string][]model.KeywordResultRow
	sf     singleflight.Group
}

type searchProgram interface {
	Name() string
	Match(term string) bool
	Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error)
}

func New(finder KeywordFinder) *Engine {
	return &Engine{
		finder: finder,
		cache:  make(map[string][]model.KeywordResultRow),
	}
}

func (e *Engine) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return e.searchKeywordRowsWithProgram(ctx, species, keyword, e.selectProgram(keyword), true, "normal")
}

func (e *Engine) SearchKeywordRowsWide(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return e.searchKeywordRowsWithProgram(ctx, species, keyword, wideSearchProgram{}, false, "forced-wide")
}

func (e *Engine) SearchKeywordRowsBroad(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return e.searchKeywordRowsWithProgram(ctx, species, keyword, broadSearchProgram{}, false, "forced-broad")
}

func (e *Engine) searchKeywordRowsWithProgram(ctx context.Context, species model.SpeciesCandidate, keyword string, program searchProgram, allowWideFallback bool, mode string) ([]model.KeywordResultRow, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}
	cacheKey := e.cacheKey(species, keyword, program.Name(), mode)

	e.mu.RLock()
	if cached, ok := e.cache[cacheKey]; ok && cacheableResult(cached, keyword) {
		rows := cloneRows(cached)
		e.mu.RUnlock()
		return rows, nil
	}
	e.mu.RUnlock()
	if cached, ok := readCachedJSON[[]model.KeywordResultRow]("rows", cacheKey); ok && cacheableResult(cached, keyword) {
		rows := cloneRows(cached)
		e.mu.Lock()
		e.cache[cacheKey] = cloneRows(cached)
		e.mu.Unlock()
		return rows, nil
	}

	value, err, _ := e.sf.Do("keyword-rows:"+cacheKey, func() (any, error) {
		e.mu.RLock()
		if cached, ok := e.cache[cacheKey]; ok && cacheableResult(cached, keyword) {
			rows := cloneRows(cached)
			e.mu.RUnlock()
			return rows, nil
		}
		e.mu.RUnlock()
		if cached, ok := readCachedJSON[[]model.KeywordResultRow]("rows", cacheKey); ok && cacheableResult(cached, keyword) {
			rows := cloneRows(cached)
			e.mu.Lock()
			e.cache[cacheKey] = cloneRows(cached)
			e.mu.Unlock()
			return rows, nil
		}

		rows, err := program.Search(ctx, e, species, keyword)
		if err != nil {
			return nil, err
		}
		searchType := program.Name()
		if len(rows) == 0 && allowWideFallback && program.Name() != SearchTypeWide {
			wide := wideSearchProgram{}
			rows, err = wide.Search(ctx, e, species, keyword)
			if err != nil {
				return nil, err
			}
			if len(rows) > 0 {
				searchType = program.Name() + fallbackSuffix
			}
		}
		rows = decorateRows(rows, keyword, searchType)
		e.mu.Lock()
		e.cache[cacheKey] = cloneRows(rows)
		e.mu.Unlock()
		writeCachedJSON("rows", cacheKey, rows)
		return rows, nil
	})
	if err != nil {
		return nil, err
	}
	return value.([]model.KeywordResultRow), nil
}

func (e *Engine) selectProgram(term string) searchProgram {
	programs := []searchProgram{
		reportURLProgram{},
		riceLocusProgram{},
		refSeqProteinProgram{},
		cytochromeFamilyProgram{},
		transcriptIDProgram{},
		geneIDProgram{},
		labelSymbolProgram{},
		geneAliasProgram{},
		identifierProgram{},
		keywordProgram{},
	}
	for _, program := range programs {
		if program.Match(term) {
			return program
		}
	}
	return keywordProgram{}
}

func (e *Engine) cacheKey(species model.SpeciesCandidate, term string, program string, mode string) string {
	return strings.Join([]string{
		cacheSchemaVersion,
		strconv.Itoa(species.ProteomeID),
		strings.TrimSpace(species.JBrowseName),
		strings.TrimSpace(species.GenomeLabel),
		normalizeTermKey(term),
		program,
		mode,
	}, "|")
}

func cacheableResult(rows []model.KeywordResultRow, keyword string) bool {
	if len(rows) == 0 {
		return true
	}
	for _, row := range rows {
		if strings.TrimSpace(row.SearchType) == "" {
			return false
		}
		if strings.TrimSpace(row.SearchTerm) == "" && strings.TrimSpace(keyword) != "" {
			return false
		}
	}
	return true
}

type reportURLProgram struct{}

func (reportURLProgram) Name() string { return SearchTypeReportURL }

func (reportURLProgram) Match(term string) bool {
	_, _, ok := LemnaGeneReportKeyword(term)
	return ok
}

func (reportURLProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByReportURL(ctx, species, term, 20)
}

type transcriptIDProgram struct{}

func (transcriptIDProgram) Name() string { return SearchTypeTranscriptID }

func (transcriptIDProgram) Match(term string) bool {
	return lemnaTranscriptPattern.MatchString(strings.TrimSpace(term))
}

func (transcriptIDProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByIdentifier(ctx, species, term, identifierKindTranscript, 20)
}

type geneIDProgram struct{}

func (geneIDProgram) Name() string { return SearchTypeGeneID }

func (geneIDProgram) Match(term string) bool {
	return lemnaGenePattern.MatchString(strings.TrimSpace(term))
}

func (geneIDProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByIdentifier(ctx, species, term, identifierKindGene, 20)
}

type riceLocusProgram struct{}

func (riceLocusProgram) Name() string { return SearchTypeRiceLocus }

func (riceLocusProgram) Match(term string) bool {
	return riceLocusPattern.MatchString(normalizeRiceLocusCandidate(term))
}

func (riceLocusProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByLabel(ctx, species, term, 20)
}

type refSeqProteinProgram struct{}

func (refSeqProteinProgram) Name() string { return SearchTypeRefSeqProtein }

func (refSeqProteinProgram) Match(term string) bool {
	return refSeqProteinPattern.MatchString(strings.TrimSpace(term))
}

func (refSeqProteinProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByLabel(ctx, species, term, 20)
}

type geneAliasProgram struct{}

func (geneAliasProgram) Name() string { return SearchTypeGeneAlias }

func (geneAliasProgram) Match(term string) bool {
	term = strings.TrimSpace(term)
	if term == "" || strings.ContainsAny(term, " \t") {
		return false
	}
	if riceLocusPattern.MatchString(normalizeRiceLocusCandidate(term)) || refSeqProteinPattern.MatchString(term) || cytochromeP450Pattern.MatchString(term) {
		return false
	}
	return isGeneAliasLike(term)
}

func (geneAliasProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByLabel(ctx, species, term, 20)
}

type cytochromeFamilyProgram struct{}

func (cytochromeFamilyProgram) Name() string { return SearchTypeCytochromeFamily }

func (cytochromeFamilyProgram) Match(term string) bool {
	return cytochromeP450Pattern.MatchString(strings.TrimSpace(term))
}

func (cytochromeFamilyProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByLabel(ctx, species, term, 20)
}

type labelSymbolProgram struct{}

func (labelSymbolProgram) Name() string { return SearchTypeLabelSymbol }

func (labelSymbolProgram) Match(term string) bool {
	term = strings.TrimSpace(term)
	if term == "" || strings.ContainsAny(term, " \t") {
		return false
	}
	return labelSymbolPattern.MatchString(term)
}

func (labelSymbolProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByLabel(ctx, species, term, 20)
}

type identifierProgram struct{}

func (identifierProgram) Name() string { return SearchTypeIdentifier }

func (identifierProgram) Match(term string) bool {
	return looksLikeSpecificIdentifier(term)
}

func (identifierProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByIdentifier(ctx, species, term, identifierKindAny, 20)
}

type keywordProgram struct{}

func (keywordProgram) Name() string { return SearchTypeKeyword }

func (keywordProgram) Match(term string) bool {
	return strings.TrimSpace(term) != ""
}

func (keywordProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByKeywordText(ctx, species, term, 50)
}

type wideSearchProgram struct{}
type broadSearchProgram struct{}

func (wideSearchProgram) Name() string { return SearchTypeWide }

func (wideSearchProgram) Match(term string) bool {
	return strings.TrimSpace(term) != ""
}

func (wideSearchProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	controlledStructuredTerm := riceLocusPattern.MatchString(normalizeRiceLocusCandidate(term)) ||
		refSeqProteinPattern.MatchString(strings.TrimSpace(term)) ||
		cytochromeP450Pattern.MatchString(strings.TrimSpace(term)) ||
		(labelSymbolPattern.MatchString(strings.TrimSpace(term)) && !strings.ContainsAny(strings.TrimSpace(term), " \t"))

	steps := []func(context.Context, *Engine, model.SpeciesCandidate, string) ([]model.KeywordResultRow, error){
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if _, _, ok := LemnaGeneReportKeyword(term); !ok {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByReportURL(ctx, species, term, 20)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if !lemnaTranscriptPattern.MatchString(strings.TrimSpace(term)) {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByIdentifier(ctx, species, term, identifierKindTranscript, 20)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if !lemnaGenePattern.MatchString(strings.TrimSpace(term)) {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByIdentifier(ctx, species, term, identifierKindGene, 20)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if !riceLocusPattern.MatchString(normalizeRiceLocusCandidate(term)) {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByLabel(ctx, species, term, 20)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if !refSeqProteinPattern.MatchString(strings.TrimSpace(term)) {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByLabel(ctx, species, term, 20)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if !(labelSymbolPattern.MatchString(strings.TrimSpace(term)) && !strings.ContainsAny(strings.TrimSpace(term), " \t")) {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByLabel(ctx, species, term, 20)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if !cytochromeP450Pattern.MatchString(strings.TrimSpace(term)) {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByLabel(ctx, species, term, 20)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if controlledStructuredTerm {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByLabel(ctx, species, term, 20)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if !looksLikeSpecificIdentifier(term) {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByIdentifier(ctx, species, term, identifierKindAny, 20)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if controlledStructuredTerm {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByKeywordText(ctx, species, term, 50)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if controlledStructuredTerm {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByWideText(ctx, species, term, 50)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if controlledStructuredTerm {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByBroadText(ctx, species, term, 10000)
		},
	}
	seen := make(map[string]struct{})
	rows := make([]model.KeywordResultRow, 0, 8)
	for _, step := range steps {
		found, err := step(ctx, engine, species, term)
		if err != nil {
			return nil, err
		}
		for _, row := range found {
			key := rowKey(row)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			rows = append(rows, row)
		}
		if len(rows) > 0 {
			return rows, nil
		}
	}
	return rows, nil
}

func (broadSearchProgram) Name() string { return SearchTypeBroad }

func (broadSearchProgram) Match(term string) bool {
	return strings.TrimSpace(term) != ""
}

func (broadSearchProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByBroadText(ctx, species, term, 10000)
}

func LemnaGeneReportKeyword(value string) (rootDir string, identifier string, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}
	if !strings.Contains(value, "://") {
		value = "https://" + strings.TrimPrefix(value, "//")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", "", false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	if host != "www.lemna.org" && host != "lemna.org" {
		return "", "", false
	}
	segments := nonEmptyPathSegments(parsed.Path)
	if len(segments) != 3 || !strings.EqualFold(segments[0], "report") {
		return "", "", false
	}
	rootDir = strings.TrimSpace(segments[1])
	identifier = strings.TrimSpace(segments[2])
	if rootDir == "" || identifier == "" {
		return "", "", false
	}
	return rootDir, identifier, true
}

func looksLikeSpecificIdentifier(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t") {
		return false
	}
	hasLetter := false
	hasDigit := false
	for _, ch := range value {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z':
			hasLetter = true
		case ch >= '0' && ch <= '9':
			hasDigit = true
		case ch == '.' || ch == '_' || ch == '-' || ch == ':':
		default:
			return false
		}
	}
	return hasLetter && hasDigit
}

func specificIdentifierVariants(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	variants := make([]string, 0, 8)
	seen := make(map[string]struct{})
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		key := strings.ToLower(candidate)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		variants = append(variants, candidate)
	}
	add(value)
	add(strings.ToUpper(value))
	add(strings.ToLower(value))
	if normalized := normalizeRiceLocusCandidate(value); normalized != "" {
		add(normalized)
		add("LOC_" + normalized)
	}
	return variants
}

func riceLocusVariants(term string) []string {
	normalized := normalizeRiceLocusCandidate(term)
	if normalized == "" || !riceLocusPattern.MatchString(normalized) {
		return specificIdentifierVariants(term)
	}
	return specificIdentifierVariants("LOC_" + normalized)
}

func normalizeRiceLocusCandidate(term string) string {
	value := strings.TrimSpace(term)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "")
	upper := strings.ToUpper(value)
	upper = strings.TrimPrefix(upper, "LOC_")
	if strings.HasPrefix(upper, "OS") && len(upper) >= 8 {
		upper = upper[2:]
	}
	parts := regexp.MustCompile(`(?i)^(\d{2})G(\d{5})(\.\d+)?$`).FindStringSubmatch(upper)
	if len(parts) == 0 {
		return ""
	}
	return "Os" + parts[1] + "g" + parts[2] + parts[3]
}

func normalizeAliasKey(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	replacer := strings.NewReplacer("_", "", "-", "", " ", "", ".", "")
	return replacer.Replace(value)
}

func isGeneAliasLike(term string) bool {
	term = strings.TrimSpace(term)
	if term == "" {
		return false
	}
	normalized := normalizeAliasKey(term)
	if len(normalized) < 5 || len(normalized) > 15 {
		return false
	}
	seenDigit := false
	seenLetterAfterDigit := false
	for _, r := range normalized {
		switch {
		case r >= '0' && r <= '9':
			seenDigit = true
		case r >= 'A' && r <= 'Z':
			if seenDigit {
				seenLetterAfterDigit = true
			}
		default:
			return false
		}
	}
	return seenDigit && seenLetterAfterDigit
}

func rowKey(row model.KeywordResultRow) string {
	for _, value := range []string{
		strings.TrimSpace(row.TranscriptID),
		strings.TrimSpace(row.GeneIdentifier),
		strings.TrimSpace(row.ProteinID),
		strings.TrimSpace(row.SequenceID),
		strings.TrimSpace(row.Location),
	} {
		if value != "" {
			return value
		}
	}
	return strings.TrimSpace(row.Genome + "|" + row.Description)
}

func decorateRows(rows []model.KeywordResultRow, searchTerm string, searchType string) []model.KeywordResultRow {
	if len(rows) == 0 {
		return nil
	}
	out := cloneRows(rows)
	for i := range out {
		out[i].SearchTerm = searchTerm
		out[i].SearchType = searchType
	}
	return out
}

func cloneRows(rows []model.KeywordResultRow) []model.KeywordResultRow {
	if len(rows) == 0 {
		return nil
	}
	out := make([]model.KeywordResultRow, len(rows))
	copy(out, rows)
	return out
}

func normalizeTermKey(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func nonEmptyPathSegments(path string) []string {
	parts := strings.Split(path, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

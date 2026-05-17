// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package tairkeyword

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"golang.org/x/sync/singleflight"
)

const (
	SearchTypeReportURL   = "TAIR report URL"
	SearchTypeLocusID     = "TAIR locus"
	SearchTypeGeneModelID = "TAIR gene model"
	SearchTypeLabelSymbol = "TAIR symbol / alias"
	SearchTypeIdentifier  = "TAIR identifier"
	SearchTypeKeyword     = "keyword"
	SearchTypeWide        = "wide search"
	SearchTypeBroad       = "broad search"
	SearchTypeFamily      = "TAIR family"
)

const (
	fallbackSuffix     = " (fallback to wide search)"
	cacheSchemaVersion = "tairkeyword-v1"

	identifierKindAny   = "any"
	identifierKindGene  = "gene"
	identifierKindModel = "model"
)

var (
	agiGenePattern  = regexp.MustCompile(`(?i)^AT[1-5CM]G\d{5}$`)
	agiModelPattern = regexp.MustCompile(`(?i)^AT[1-5CM]G\d{5}\.\d+$`)
	symbolPattern   = regexp.MustCompile(`\b[A-Z][A-Z0-9-]{1,14}\b`)
)

type KeywordFinder interface {
	SearchKeywordRowsByReportURL(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error)
	SearchKeywordRowsByIdentifier(ctx context.Context, species model.SpeciesCandidate, term string, kind string, limit int) ([]model.KeywordResultRow, error)
	SearchKeywordRowsByLabel(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error)
	SearchKeywordRowsByKeywordText(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error)
	SearchKeywordRowsByWideText(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error)
	SearchKeywordRowsByBroadText(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error)
	SearchKeywordRowsByFamily(ctx context.Context, species model.SpeciesCandidate, family string, limit int) ([]model.KeywordResultRow, error)
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
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecManaged,
		Domain:      "www.arabidopsis.org",
		Description: "search tair keyword engine rows",
	}, func(runCtx context.Context) ([]model.KeywordResultRow, error) {
		return e.searchKeywordRowsWithProgram(runCtx, species, keyword, e.selectProgram(keyword), true, "normal")
	})
}

func (e *Engine) SearchKeywordRowsWide(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecManaged,
		Domain:      "www.arabidopsis.org",
		Description: "search tair keyword engine rows wide",
	}, func(runCtx context.Context) ([]model.KeywordResultRow, error) {
		return e.searchKeywordRowsWithProgram(runCtx, species, keyword, wideSearchProgram{}, false, "forced-wide")
	})
}

func (e *Engine) SearchKeywordRowsBroad(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecManaged,
		Domain:      "www.arabidopsis.org",
		Description: "search tair keyword engine rows broad",
	}, func(runCtx context.Context) ([]model.KeywordResultRow, error) {
		return e.searchKeywordRowsWithProgram(runCtx, species, keyword, broadSearchProgram{}, false, "forced-broad")
	})
}

func (e *Engine) SearchFamilyKeywordRows(ctx context.Context, species model.SpeciesCandidate, family string) ([]model.KeywordResultRow, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecManaged,
		Domain:      "www.arabidopsis.org",
		Description: "search tair family keyword rows",
	}, func(runCtx context.Context) ([]model.KeywordResultRow, error) {
		return e.searchKeywordRowsWithProgram(runCtx, species, family, familySearchProgram{}, false, "family")
	})
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
		geneModelProgram{},
		locusIDProgram{},
		identifierProgram{},
		labelSymbolProgram{},
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
	_, _, ok := TAIRReportKeyword(term)
	return ok
}
func (reportURLProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByReportURL(ctx, species, term, 20)
}

type locusIDProgram struct{}

func (locusIDProgram) Name() string { return SearchTypeLocusID }
func (locusIDProgram) Match(term string) bool {
	return agiGenePattern.MatchString(strings.TrimSpace(term))
}
func (locusIDProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByIdentifier(ctx, species, term, identifierKindGene, 20)
}

type geneModelProgram struct{}

func (geneModelProgram) Name() string { return SearchTypeGeneModelID }
func (geneModelProgram) Match(term string) bool {
	return agiModelPattern.MatchString(strings.TrimSpace(term))
}
func (geneModelProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByIdentifier(ctx, species, term, identifierKindModel, 20)
}

type labelSymbolProgram struct{}

func (labelSymbolProgram) Name() string { return SearchTypeLabelSymbol }
func (labelSymbolProgram) Match(term string) bool {
	term = strings.TrimSpace(term)
	if term == "" || strings.ContainsAny(term, " \t") {
		 return false
	}
	return symbolPattern.MatchString(term)
}
func (labelSymbolProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByLabel(ctx, species, term, 20)
}

type identifierProgram struct{}

func (identifierProgram) Name() string { return SearchTypeIdentifier }
func (identifierProgram) Match(term string) bool {
	term = strings.TrimSpace(term)
	if term == "" {
		return false
	}
	if agiGenePattern.MatchString(term) || agiModelPattern.MatchString(term) {
		return true
	}
	if strings.ContainsAny(term, ".:-") {
		return true
	}
	if !strings.HasPrefix(strings.ToUpper(term), "AT") {
		return false
	}
	if len(term) < 8 {
		return false
	}
	return LooksLikeSpecificIdentifier(term)
}
func (identifierProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByIdentifier(ctx, species, term, identifierKindAny, 20)
}

type keywordProgram struct{}

func (keywordProgram) Name() string { return SearchTypeKeyword }
func (keywordProgram) Match(term string) bool { return strings.TrimSpace(term) != "" }
func (keywordProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByKeywordText(ctx, species, term, 50)
}

type familySearchProgram struct{}

func (familySearchProgram) Name() string { return SearchTypeFamily }
func (familySearchProgram) Match(term string) bool { return strings.TrimSpace(term) != "" }
func (familySearchProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByFamily(ctx, species, term, 10000)
}

type wideSearchProgram struct{}
type broadSearchProgram struct{}

func (wideSearchProgram) Name() string { return SearchTypeWide }
func (wideSearchProgram) Match(term string) bool { return strings.TrimSpace(term) != "" }
func (wideSearchProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	term = strings.TrimSpace(term)
	locus := agiGenePattern.MatchString(term)
	modelID := agiModelPattern.MatchString(term)
	labelSymbol := symbolPattern.MatchString(term) && !strings.ContainsAny(term, " \t")
	identifier := LooksLikeSpecificIdentifier(term)
	reportURL := false
	if _, _, ok := TAIRReportKeyword(term); ok {
		reportURL = true
	}

	steps := []func(context.Context, *Engine, model.SpeciesCandidate, string) ([]model.KeywordResultRow, error){
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if !reportURL {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByReportURL(ctx, species, term, 20)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if !modelID {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByIdentifier(ctx, species, term, identifierKindModel, 20)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if !locus {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByIdentifier(ctx, species, term, identifierKindGene, 20)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if !labelSymbol {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByLabel(ctx, species, term, 20)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			if !identifier {
				return nil, nil
			}
			return engine.finder.SearchKeywordRowsByIdentifier(ctx, species, term, identifierKindAny, 20)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			return engine.finder.SearchKeywordRowsByKeywordText(ctx, species, term, 50)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
			return engine.finder.SearchKeywordRowsByWideText(ctx, species, term, 1000)
		},
		func(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
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
func (broadSearchProgram) Match(term string) bool { return strings.TrimSpace(term) != "" }
func (broadSearchProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	return engine.finder.SearchKeywordRowsByBroadText(ctx, species, term, 10000)
}

func TAIRReportKeyword(value string) (reportType string, identifier string, ok bool) {
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
	if host != "www.arabidopsis.org" && host != "arabidopsis.org" {
		return "", "", false
	}
	query := parsed.Query()
	name := strings.TrimSpace(firstNonEmpty(query.Get("name"), query.Get("accession")))
	if strings.Contains(name, ":") {
		parts := strings.Split(name, ":")
		name = strings.TrimSpace(parts[len(parts)-1])
	}
	if name == "" {
		return "", "", false
	}
	reportType = strings.ToLower(firstNonEmpty(query.Get("type"), "gene"))
	return reportType, name, true
}

func LooksLikeSpecificIdentifier(value string) bool {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

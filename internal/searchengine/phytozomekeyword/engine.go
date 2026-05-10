package phytozomekeyword

import (
	"context"
	"errors"
	"fmt"
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
	SearchTypeReportURL        = "report URL"
	SearchTypePhytozomeID      = "Phytozome identifier"
	SearchTypeRiceLocusID      = "rice LOC_Os locus"
	SearchTypeRefSeqProtein    = "RefSeq XP protein"
	SearchTypeRiceGeneAlias    = "rice gene alias"
	SearchTypeCytochromeFamily = "CYP73 family symbol"
	SearchTypeKeyword          = "keyword"
	SearchTypeWide             = "wide search"
)

const fallbackSuffix = " (fallback to wide search)"

var (
	riceLocusPattern        = regexp.MustCompile(`(?i)^(?:LOC_)?(?:OS)?\d{2}G\d{5}(?:\.\d+)?$`)
	refSeqProteinPattern    = regexp.MustCompile(`(?i)^(?:XP_?)\d+(?:\.\d+)?$`)
	cytochromeP450Pattern   = regexp.MustCompile(`(?i)^CYP\d+[A-Z]\d+$`)
	osC4HPattern            = regexp.MustCompile(`(?i)^OS[-_ ]?C4H\d+[A-Z]?$`)
	reportURLHost           = "phytozome-next.jgi.doe.gov"
	riceAliasByNormalized   = curatedRiceAliasMap()
	refSeqAliasByNormalized = curatedRiceRefSeqAliasMap()
)

type GeneRecord struct {
	ID                string           `json:"_id"`
	Proteome          string           `json:"proteome"`
	PrimaryIdentifier string           `json:"primaryidentifier"`
	Start             string           `json:"start"`
	End               string           `json:"end"`
	Strand            string           `json:"strand"`
	Scaffold          string           `json:"scaffold"`
	Symbols           []string         `json:"symbols"`
	Synonyms          []string         `json:"synonyms"`
	Comments          []string         `json:"comments"`
	Deflines          []string         `json:"deflines"`
	AutoDefline       string           `json:"auto_defline"`
	Organism          GeneOrganismInfo `json:"organism"`
	Transcripts       []GeneTranscript `json:"transcripts"`
}

type GeneOrganismInfo struct {
	TaxID             string `json:"tax_id"`
	OrganismName      string `json:"organism_name"`
	ShortName         string `json:"organism_shortname"`
	AnnotationVersion string `json:"annotation_version"`
	Proteome          int    `json:"proteome"`
}

type GeneTranscript struct {
	Protein             string   `json:"protein"`
	PrimaryIdentifier   string   `json:"primaryidentifier"`
	SecondaryIdentifier string   `json:"secondaryidentifier"`
	IsPrimary           string   `json:"is_primary"`
	Uniprot             []string `json:"uniprot"`
}

type GeneFinder interface {
	FetchGeneByGeneID(ctx context.Context, proteomeID int, geneID string) (GeneRecord, error)
	FetchGeneByTranscript(ctx context.Context, proteomeID int, transcriptID string) (GeneRecord, error)
	FetchGeneByProtein(ctx context.Context, proteomeID int, proteinID string) (GeneRecord, error)
	SearchGenesByKeyword(ctx context.Context, proteomeID int, keyword string, limit int) ([]GeneRecord, error)
}

type BroadGeneKeywordFinder interface {
	SearchGenesByKeywordBroad(ctx context.Context, proteomeID int, keyword string, limit int) ([]GeneRecord, error)
}

type Engine struct {
	finder GeneFinder
	mu     sync.RWMutex
	cache  map[string][]model.KeywordResultRow
	sf     singleflight.Group
}

type searchProgram interface {
	Name() string
	Match(term string) bool
	Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]GeneRecord, error)
}

func New(finder GeneFinder) *Engine {
	return &Engine{
		finder: finder,
		cache:  make(map[string][]model.KeywordResultRow),
	}
}

func (e *Engine) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "phytozome-next.jgi.doe.gov",
		Description: "search phytozome keyword engine rows",
	}, func(runCtx context.Context) ([]model.KeywordResultRow, error) {
		return e.searchKeywordRowsWithProgram(runCtx, species, keyword, e.selectProgram(keyword), true, "normal")
	})
}

func (e *Engine) SearchKeywordRowsWide(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "phytozome-next.jgi.doe.gov",
		Description: "search phytozome keyword engine rows wide",
	}, func(runCtx context.Context) ([]model.KeywordResultRow, error) {
		return e.searchKeywordRowsWithProgram(runCtx, species, keyword, wideSearchProgram{}, false, "forced-wide")
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
		rows := append([]model.KeywordResultRow(nil), cached...)
		e.mu.RUnlock()
		return rows, nil
	}
	e.mu.RUnlock()
	if cached, ok := readCachedJSON[[]model.KeywordResultRow]("rows", cacheKey); ok && cacheableResult(cached, keyword) {
		rows := append([]model.KeywordResultRow(nil), cached...)
		e.mu.Lock()
		e.cache[cacheKey] = append([]model.KeywordResultRow(nil), cached...)
		e.mu.Unlock()
		return rows, nil
	}

	value, err, _ := e.sf.Do("keyword-rows:"+cacheKey, func() (any, error) {
		e.mu.RLock()
		if cached, ok := e.cache[cacheKey]; ok && cacheableResult(cached, keyword) {
			rows := append([]model.KeywordResultRow(nil), cached...)
			e.mu.RUnlock()
			return rows, nil
		}
		e.mu.RUnlock()
		if cached, ok := readCachedJSON[[]model.KeywordResultRow]("rows", cacheKey); ok && cacheableResult(cached, keyword) {
			rows := append([]model.KeywordResultRow(nil), cached...)
			e.mu.Lock()
			e.cache[cacheKey] = append([]model.KeywordResultRow(nil), cached...)
			e.mu.Unlock()
			return rows, nil
		}

		genes, err := program.Search(ctx, e, species, keyword)
		if err != nil {
			return nil, err
		}
		searchType := program.Name()
		if len(genes) == 0 && allowWideFallback && program.Name() != SearchTypeWide {
			wide := wideSearchProgram{}
			genes, err = wide.Search(ctx, e, species, keyword)
			if err != nil {
				return nil, err
			}
			if len(genes) > 0 {
				searchType = program.Name() + fallbackSuffix
			}
		}
		rows := e.buildRows(keyword, searchType, species, genes)
		if cacheableResult(rows, keyword) {
			e.mu.Lock()
			e.cache[cacheKey] = append([]model.KeywordResultRow(nil), rows...)
			e.mu.Unlock()
			writeCachedJSON("rows", cacheKey, rows)
		}
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
		riceAliasProgram{},
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
		strconv.Itoa(species.ProteomeID),
		strings.TrimSpace(species.JBrowseName),
		normalizeTermKey(term),
		program,
		mode,
	}, "|")
}

func (e *Engine) buildRows(searchTerm string, searchType string, species model.SpeciesCandidate, genes []GeneRecord) []model.KeywordResultRow {
	rows := make([]model.KeywordResultRow, 0, len(genes))
	for _, gene := range genes {
		row, err := buildKeywordResultRow(searchTerm, searchType, species, gene)
		if err != nil {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func cacheableResult(rows []model.KeywordResultRow, keyword string) bool {
	if len(rows) == 0 {
		return false
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
	_, _, ok := PhytozomeGeneReportKeyword(term)
	return ok
}

func (reportURLProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]GeneRecord, error) {
	reportType, identifier, ok := PhytozomeGeneReportKeyword(term)
	if !ok {
		return nil, nil
	}
	return engine.searchSpecificIdentifier(ctx, species, reportType, identifier)
}

type riceLocusProgram struct{}

func (riceLocusProgram) Name() string { return SearchTypeRiceLocusID }

func (riceLocusProgram) Match(term string) bool {
	return riceLocusPattern.MatchString(normalizeRiceLocusCandidate(term))
}

func (riceLocusProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]GeneRecord, error) {
	return engine.searchAliasesAsGenes(ctx, species, riceLocusVariants(term))
}

type refSeqProteinProgram struct{}

func (refSeqProteinProgram) Name() string { return SearchTypeRefSeqProtein }

func (refSeqProteinProgram) Match(term string) bool {
	return refSeqProteinPattern.MatchString(strings.TrimSpace(term))
}

func (refSeqProteinProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]GeneRecord, error) {
	aliases := aliasesForNormalizedTerm(refSeqAliasByNormalized, term)
	if len(aliases) > 0 {
		if genes, err := engine.searchAliasesAsGenes(ctx, species, aliases); err != nil || len(genes) > 0 {
			return genes, err
		}
	}
	return engine.searchSpecificIdentifier(ctx, species, "", term)
}

type riceAliasProgram struct{}

func (riceAliasProgram) Name() string { return SearchTypeRiceGeneAlias }

func (riceAliasProgram) Match(term string) bool {
	return len(aliasesForNormalizedTerm(riceAliasByNormalized, term)) > 0 || osC4HPattern.MatchString(strings.TrimSpace(term))
}

func (riceAliasProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]GeneRecord, error) {
	return engine.searchAliasesAsGenes(ctx, species, aliasesForNormalizedTerm(riceAliasByNormalized, term))
}

type cytochromeFamilyProgram struct{}

func (cytochromeFamilyProgram) Name() string { return SearchTypeCytochromeFamily }

func (cytochromeFamilyProgram) Match(term string) bool {
	return cytochromeP450Pattern.MatchString(strings.TrimSpace(term))
}

func (cytochromeFamilyProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]GeneRecord, error) {
	aliases := aliasesForNormalizedTerm(riceAliasByNormalized, term)
	if len(aliases) > 0 {
		return engine.searchAliasesAsGenes(ctx, species, aliases)
	}
	return nil, nil
}

type identifierProgram struct{}

func (identifierProgram) Name() string { return SearchTypePhytozomeID }

func (identifierProgram) Match(term string) bool {
	return LooksLikeSpecificGeneIdentifier(term)
}

func (identifierProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]GeneRecord, error) {
	return engine.searchSpecificIdentifier(ctx, species, "", term)
}

type keywordProgram struct{}

func (keywordProgram) Name() string { return SearchTypeKeyword }

func (keywordProgram) Match(term string) bool {
	return strings.TrimSpace(term) != ""
}

func (keywordProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]GeneRecord, error) {
	return engine.searchKeyword(ctx, species, term, 20)
}

type wideSearchProgram struct{}

func (wideSearchProgram) Name() string { return SearchTypeWide }

func (wideSearchProgram) Match(term string) bool {
	return strings.TrimSpace(term) != ""
}

func (wideSearchProgram) Search(ctx context.Context, engine *Engine, species model.SpeciesCandidate, term string) ([]GeneRecord, error) {
	seen := make(map[string]struct{})
	genes := make([]GeneRecord, 0, 8)
	add := func(values []GeneRecord) {
		for _, gene := range values {
			addGene(&genes, seen, gene)
		}
	}
	for _, aliases := range [][]string{
		aliasesForNormalizedTerm(riceAliasByNormalized, term),
		aliasesForNormalizedTerm(refSeqAliasByNormalized, term),
		riceLocusVariants(term),
	} {
		if len(aliases) == 0 {
			continue
		}
		found, err := engine.searchAliasesAsGenes(ctx, species, aliases)
		if err != nil {
			return nil, err
		}
		add(found)
		if len(genes) > 0 {
			return genes, nil
		}
	}
	found, err := engine.searchSpecificIdentifier(ctx, species, "", term)
	if err != nil {
		return nil, err
	}
	add(found)
	if len(genes) > 0 {
		return genes, nil
	}
	if broadFinder, ok := engine.finder.(BroadGeneKeywordFinder); ok {
		found, err = broadFinder.SearchGenesByKeywordBroad(ctx, species.ProteomeID, term, 10000)
		if err != nil {
			return nil, err
		}
		for _, gene := range found {
			geneID := strings.TrimSpace(gene.PrimaryIdentifier)
			if geneID != "" {
				if fullGene, err := engine.finder.FetchGeneByGeneID(ctx, species.ProteomeID, geneID); err == nil {
					gene = fullGene
				}
			}
			addGene(&genes, seen, gene)
		}
		if len(genes) > 0 {
			return genes, nil
		}
	}
	found, err = engine.searchKeyword(ctx, species, wideKeywordQuery(term), 50)
	if err != nil {
		return nil, err
	}
	add(found)
	if len(genes) > 0 {
		return genes, nil
	}
	for _, query := range relaxedKeywordQueries(term) {
		found, err = engine.searchKeyword(ctx, species, query, 50)
		if err != nil {
			return nil, err
		}
		add(found)
		if len(genes) > 0 {
			return genes, nil
		}
	}
	return genes, nil
}

func (e *Engine) searchSpecificIdentifier(ctx context.Context, species model.SpeciesCandidate, reportType string, identifier string) ([]GeneRecord, error) {
	seen := make(map[string]struct{})
	genes := make([]GeneRecord, 0, 3)
	variants := SpecificIdentifierVariants(identifier)
	for _, variant := range variants {
		switch reportType {
		case "gene":
			gene, err := e.finder.FetchGeneByGeneID(ctx, species.ProteomeID, variant)
			if err == nil {
				addGene(&genes, seen, gene)
			} else if shouldStopSpecificIdentifierSearch(ctx, err) {
				return nil, err
			}
		case "transcript":
			gene, err := e.finder.FetchGeneByTranscript(ctx, species.ProteomeID, variant)
			if err == nil {
				addGene(&genes, seen, gene)
			} else if shouldStopSpecificIdentifierSearch(ctx, err) {
				return nil, err
			}
		case "protein":
			gene, err := e.finder.FetchGeneByProtein(ctx, species.ProteomeID, variant)
			if err == nil {
				addGene(&genes, seen, gene)
			} else if shouldStopSpecificIdentifierSearch(ctx, err) {
				return nil, err
			}
		default:
			gene, err := e.finder.FetchGeneByGeneID(ctx, species.ProteomeID, variant)
			if err == nil {
				addGene(&genes, seen, gene)
				continue
			} else if shouldStopSpecificIdentifierSearch(ctx, err) {
				return nil, err
			}
			gene, err = e.finder.FetchGeneByTranscript(ctx, species.ProteomeID, variant)
			if err == nil {
				addGene(&genes, seen, gene)
				continue
			} else if shouldStopSpecificIdentifierSearch(ctx, err) {
				return nil, err
			}
			gene, err = e.finder.FetchGeneByProtein(ctx, species.ProteomeID, variant)
			if err == nil {
				addGene(&genes, seen, gene)
			} else if shouldStopSpecificIdentifierSearch(ctx, err) {
				return nil, err
			}
		}
		if len(genes) > 0 && reportType != "" {
			break
		}
	}
	return genes, nil
}

func shouldStopSpecificIdentifierSearch(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}
	if strings.Contains(message, "context deadline exceeded") || strings.Contains(message, "operation was canceled") || strings.Contains(message, "client.timeout") || strings.Contains(message, "timeout awaiting response headers") {
		return true
	}
	if strings.Contains(message, "unexpected eof") || strings.Contains(message, "connection reset") || strings.Contains(message, "connection refused") || strings.Contains(message, "tls handshake timeout") || strings.Contains(message, "no such host") {
		return true
	}
	if strings.Contains(message, "status 429") || strings.Contains(message, "too many requests") || strings.Contains(message, "status 5") {
		return true
	}
	return false
}

func (e *Engine) searchAliasesAsGenes(ctx context.Context, species model.SpeciesCandidate, aliases []string) ([]GeneRecord, error) {
	seen := make(map[string]struct{})
	genes := make([]GeneRecord, 0, len(aliases))
	for _, alias := range aliases {
		if strings.TrimSpace(alias) == "" {
			continue
		}
		found, err := e.searchSpecificIdentifier(ctx, species, "", alias)
		if err != nil {
			return nil, err
		}
		for _, gene := range found {
			addGene(&genes, seen, gene)
		}
		if len(found) > 0 {
			continue
		}
		keywordHits, err := e.searchKeyword(ctx, species, alias, 20)
		if err != nil {
			return nil, err
		}
		for _, gene := range keywordHits {
			addGene(&genes, seen, gene)
		}
	}
	return genes, nil
}

func (e *Engine) searchKeyword(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]GeneRecord, error) {
	if strings.TrimSpace(term) == "" {
		return nil, nil
	}
	matches, err := e.finder.SearchGenesByKeyword(ctx, species.ProteomeID, term, limit)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	genes := make([]GeneRecord, 0, len(matches))
	for _, gene := range matches {
		addGene(&genes, seen, gene)
	}
	return genes, nil
}

func addGene(genes *[]GeneRecord, seen map[string]struct{}, gene GeneRecord) {
	key := strings.TrimSpace(gene.PrimaryIdentifier) + "|" + strconv.Itoa(gene.ProteomeID())
	if key == "|" {
		return
	}
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*genes = append(*genes, gene)
}

func PhytozomeGeneReportKeyword(value string) (reportType string, identifier string, ok bool) {
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
	if !strings.EqualFold(parsed.Host, reportURLHost) {
		return "", "", false
	}
	segments := nonEmptyPathSegments(parsed.Path)
	if len(segments) != 4 || !strings.EqualFold(segments[0], "report") {
		return "", "", false
	}
	reportType = strings.ToLower(strings.TrimSpace(segments[1]))
	if reportType != "gene" && reportType != "transcript" && reportType != "protein" {
		return "", "", false
	}
	identifier = strings.TrimSpace(segments[3])
	if identifier == "" {
		return "", "", false
	}
	return reportType, identifier, true
}

func LooksLikeSpecificGeneIdentifier(value string) bool {
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

func SpecificIdentifierVariants(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	variants := make([]string, 0, 8)
	seen := make(map[string]struct{}, 8)
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		variants = append(variants, candidate)
	}
	upper := strings.ToUpper(value)
	if arabidopsisGeneIDLabelPattern.MatchString(value) || strings.HasPrefix(upper, "AT") {
		add(upper)
		add(value)
		add(strings.ToLower(value))
	} else {
		add(value)
		add(upper)
		add(strings.ToLower(value))
	}
	if normalized := normalizeRiceLocusCandidate(value); normalized != "" {
		add(normalized)
		add("LOC_" + normalized)
	}
	return variants
}

func riceLocusVariants(term string) []string {
	normalized := normalizeRiceLocusCandidate(term)
	if normalized == "" || !riceLocusPattern.MatchString(normalized) {
		return SpecificIdentifierVariants(term)
	}
	return SpecificIdentifierVariants("LOC_" + normalized)
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

func aliasesForNormalizedTerm(catalog map[string][]string, term string) []string {
	key := normalizeAliasKey(term)
	values := catalog[key]
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func curatedRiceRefSeqAliasMap() map[string][]string {
	return map[string][]string{
		normalizeAliasKey("XP_015639656"): {"LOC_Os05g25640"},
		normalizeAliasKey("XP_015635394"): {"LOC_Os01g60450"},
		normalizeAliasKey("XP_015623447"): {"LOC_Os02g26770"},
		normalizeAliasKey("XP_015626579"): {"LOC_Os02g26810"},
	}
}

func curatedRiceAliasMap() map[string][]string {
	return map[string][]string{
		normalizeAliasKey("OsC4H1"):    {"LOC_Os05g25640"},
		normalizeAliasKey("CYP73A35p"): {"LOC_Os01g60450"},
		normalizeAliasKey("OsC4H2a"):   {"LOC_Os02g26770"},
		normalizeAliasKey("OsC4H2"):    {"LOC_Os02g26810"},
		normalizeAliasKey("CYP73A38"):  {"LOC_Os05g25640"},
		normalizeAliasKey("CYP73A39"):  {"LOC_Os01g60450"},
		normalizeAliasKey("CYP73A40"):  {"LOC_Os02g26770"},
	}
}

func wideKeywordQuery(term string) string {
	term = strings.TrimSpace(term)
	if term == "" {
		return term
	}
	return strings.ReplaceAll(term, "_", " ")
}

func relaxedKeywordQueries(term string) []string {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil
	}
	queries := make([]string, 0, 4)
	add := func(query string) {
		query = strings.TrimSpace(query)
		if query == "" || strings.EqualFold(query, term) {
			return
		}
		for _, existing := range queries {
			if strings.EqualFold(existing, query) {
				return
			}
		}
		queries = append(queries, query)
	}
	add(strings.ReplaceAll(term, "_", " "))
	add(strings.ReplaceAll(term, "-", " "))
	if refSeqProteinPattern.MatchString(term) {
		add(strings.TrimSuffix(strings.ReplaceAll(term, "_", ""), ".1"))
	}
	if cytochromeP450Pattern.MatchString(term) {
		add(strings.TrimSuffix(strings.ToUpper(term), "P"))
	}
	return queries
}

func normalizeAliasKey(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	replacer := strings.NewReplacer("_", "", "-", "", " ", "", ".", "")
	return replacer.Replace(value)
}

func normalizeTermKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func nonEmptyPathSegments(path string) []string {
	parts := strings.Split(path, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			segments = append(segments, part)
		}
	}
	return segments
}

func (g GeneRecord) PrimaryTranscript(preferredID string) (GeneTranscript, error) {
	preferredID = strings.TrimSpace(preferredID)
	for _, transcript := range g.Transcripts {
		if preferredID != "" && transcript.PrimaryIdentifier == preferredID {
			return transcript, nil
		}
	}
	for _, transcript := range g.Transcripts {
		if transcript.IsPrimary == "1" {
			return transcript, nil
		}
	}
	if len(g.Transcripts) == 0 {
		return GeneTranscript{}, fmt.Errorf("gene record %s has no transcripts", g.PrimaryIdentifier)
	}
	return g.Transcripts[0], nil
}

func (g GeneRecord) PrimaryTranscriptByProtein(proteinID string) (GeneTranscript, error) {
	proteinID = strings.TrimSpace(proteinID)
	for _, transcript := range g.Transcripts {
		if proteinID != "" && strings.EqualFold(strings.TrimSpace(transcript.Protein), proteinID) {
			return transcript, nil
		}
		if proteinID != "" && strings.EqualFold(strings.TrimSpace(transcript.PrimaryIdentifier), proteinID) {
			return transcript, nil
		}
	}
	return g.PrimaryTranscript("")
}

func (g GeneRecord) OrganismShortName() string {
	return strings.TrimSpace(g.Organism.ShortName)
}

func (g GeneRecord) AnnotationVersion() string {
	return strings.TrimSpace(g.Organism.AnnotationVersion)
}

func (g GeneRecord) ProteomeID() int {
	return g.Organism.Proteome
}

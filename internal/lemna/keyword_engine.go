// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package lemna

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/searchengine/lemnakeyword"
)

type lemnaKeywordProgram interface {
	Name() string
	Match(term string) bool
	Search(ctx context.Context, session *lemnaKeywordSearchSession, term string, limit int) ([]model.KeywordResultRow, error)
}

type lemnaKeywordSearchSession struct {
	client  *Client
	index   lemnaKeywordIndex
	species model.SpeciesCandidate
	release releaseInfo

	identifierCache map[string][]model.KeywordResultRow
	aliasCache      map[string][]model.KeywordResultRow
	termCache       map[string][]model.KeywordResultRow
}

type lemnaReportURLProgram struct{}
type lemnaIdentifierProgram struct{}
type lemnaRiceLocusProgram struct{}
type lemnaRefSeqProteinProgram struct{}
type lemnaRiceAliasProgram struct{}
type lemnaCytochromeFamilyProgram struct{}
type lemnaKeywordProgramDefault struct{}
type lemnaWideKeywordProgram struct{}
type lemnaBroadKeywordProgram struct{}

const (
	lemnaSearchTypeReportURL        = "report URL"
	lemnaSearchTypeIdentifier       = "Lemna identifier"
	lemnaSearchTypeRiceLocus        = "rice LOC_Os locus"
	lemnaSearchTypeRefSeqProtein    = "RefSeq XP protein"
	lemnaSearchTypeGeneAlias        = "gene alias / symbol"
	lemnaSearchTypeCytochromeFamily = "CYP73 family symbol"
	lemnaSearchTypeKeyword          = "keyword"
	lemnaSearchTypeWide             = "wide search"
	lemnaSearchTypeBroad            = "broad search"
	lemnaKeywordIndexSchemaVersion  = "v2"
)

var (
	lemnaTranscriptPattern         = regexp.MustCompile(`(?i)^[A-Z]{2}\d{4}D\d{3}G\d{6}_T\d+$`)
	lemnaGenePattern               = regexp.MustCompile(`(?i)^[A-Z]{2}\d{4}D\d{3}G\d{6}$`)
	labelSymbolPattern             = regexp.MustCompile(`\b[A-Z][A-Z0-9-]{1,14}\b`)
	riceLocusPattern             = regexp.MustCompile(`(?i)^(?:LOC_)?(?:OS)?\d{2}G\d{5}(?:\.\d+)?$`)
	refSeqProteinPattern         = regexp.MustCompile(`(?i)^(?:XP_?)\d+(?:\.\d+)?$`)
	cytochromeP450Pattern        = regexp.MustCompile(`(?i)^CYP\d+[A-Z]\d+$`)
	curatedRiceRefSeqAliasLookup = map[string][]string{
		normalizeAliasKey("XP_015639656"):   {"LOC_Os05g25640"},
		normalizeAliasKey("XP_015639656.1"): {"LOC_Os05g25640"},
		normalizeAliasKey("XP_015635394"):   {"LOC_Os01g60450"},
		normalizeAliasKey("XP_015635394.1"): {"LOC_Os01g60450"},
		normalizeAliasKey("XP_015623447"):   {"LOC_Os02g26770"},
		normalizeAliasKey("XP_015623447.1"): {"LOC_Os02g26770"},
		normalizeAliasKey("XP_015626579"):   {"LOC_Os02g26810"},
		normalizeAliasKey("XP_015626579.1"): {"LOC_Os02g26810"},
		normalizeAliasKey("XP_015650724"):   {"LOC_Os08g14760"},
		normalizeAliasKey("XP_015650724.1"): {"LOC_Os08g14760"},
		normalizeAliasKey("XP_015624111"):   {"LOC_Os02g46970"},
		normalizeAliasKey("XP_015624111.1"): {"LOC_Os02g46970"},
		normalizeAliasKey("XP_015625716"):   {"LOC_Os02g08100"},
		normalizeAliasKey("XP_015625716.1"): {"LOC_Os02g08100"},
		normalizeAliasKey("XP_015643415"):   {"LOC_Os06g44620"},
		normalizeAliasKey("XP_015643415.1"): {"LOC_Os06g44620"},
		normalizeAliasKey("XP_015650830"):   {"LOC_Os08g34790"},
		normalizeAliasKey("XP_015650830.1"): {"LOC_Os08g34790"},
	}
	curatedRiceLocusAliasLookup = map[string][]string{
		normalizeAliasKey("LOC_Os05g25640"): {"C4H"},
		normalizeAliasKey("LOC_Os01g60450"): {"CYP73A35p"},
		normalizeAliasKey("LOC_Os02g26770"): {"OsC4H2a"},
		normalizeAliasKey("LOC_Os02g26810"): {"OsC4H2"},
		normalizeAliasKey("LOC_Os08g14760"): {"Os4CL1"},
		normalizeAliasKey("LOC_Os02g46970"): {"Os4CL2"},
		normalizeAliasKey("LOC_Os02g08100"): {"Os4CL3"},
		normalizeAliasKey("LOC_Os06g44620"): {"Os4CL4"},
		normalizeAliasKey("LOC_Os08g34790"): {"Os4CL5"},
	}
	curatedRiceAliasLookup = map[string][]string{
		normalizeAliasKey("Os4CL1"):    {"LOC_Os08g14760"},
		normalizeAliasKey("Os4CL2"):    {"LOC_Os02g46970"},
		normalizeAliasKey("Os4CL3"):    {"LOC_Os02g08100"},
		normalizeAliasKey("Os4CL4"):    {"LOC_Os06g44620"},
		normalizeAliasKey("Os4CL5"):    {"LOC_Os08g34790"},
		normalizeAliasKey("OsC4H1"):    {"LOC_Os05g25640"},
		normalizeAliasKey("CYP73A35p"): {"LOC_Os01g60450"},
		normalizeAliasKey("OsC4H2a"):   {"LOC_Os02g26770"},
		normalizeAliasKey("OsC4H2"):    {"LOC_Os02g26810"},
		normalizeAliasKey("CYP73A38"):  {"LOC_Os05g25640"},
		normalizeAliasKey("CYP73A39"):  {"LOC_Os01g60450"},
		normalizeAliasKey("CYP73A40"):  {"LOC_Os02g26770"},
	}
	curatedRiceKeywordTargetLookup = map[string]lemnaCuratedKeywordTarget{
		normalizeAliasKey("LOC_Os08g14760"): {Label: "Os4CL1", Queries: []string{"4CL", "4-coumarate"}},
		normalizeAliasKey("LOC_Os02g46970"): {Label: "Os4CL2", Queries: []string{"4CL", "4-coumarate"}},
		normalizeAliasKey("LOC_Os02g08100"): {Label: "Os4CL3", Queries: []string{"4CL", "4-coumarate"}},
		normalizeAliasKey("LOC_Os06g44620"): {Label: "Os4CL4", Queries: []string{"4CL", "4-coumarate"}},
		normalizeAliasKey("LOC_Os08g34790"): {Label: "Os4CL5", Queries: []string{"4CL", "4-coumarate"}},
		normalizeAliasKey("XP_015650724"):   {Label: "Os4CL1", Queries: []string{"4CL", "4-coumarate"}},
		normalizeAliasKey("XP_015624111"):   {Label: "Os4CL2", Queries: []string{"4CL", "4-coumarate"}},
		normalizeAliasKey("XP_015625716"):   {Label: "Os4CL3", Queries: []string{"4CL", "4-coumarate"}},
		normalizeAliasKey("XP_015643415"):   {Label: "Os4CL4", Queries: []string{"4CL", "4-coumarate"}},
		normalizeAliasKey("XP_015650830"):   {Label: "Os4CL5", Queries: []string{"4CL", "4-coumarate"}},
		normalizeAliasKey("Os4CL1"):         {Label: "Os4CL1", Queries: []string{"4CL", "4-coumarate"}},
		normalizeAliasKey("Os4CL2"):         {Label: "Os4CL2", Queries: []string{"4CL", "4-coumarate"}},
		normalizeAliasKey("Os4CL3"):         {Label: "Os4CL3", Queries: []string{"4CL", "4-coumarate"}},
		normalizeAliasKey("Os4CL4"):         {Label: "Os4CL4", Queries: []string{"4CL", "4-coumarate"}},
		normalizeAliasKey("Os4CL5"):         {Label: "Os4CL5", Queries: []string{"4CL", "4-coumarate"}},
		normalizeAliasKey("LOC_Os05g25640"): {Label: "C4H", Queries: []string{"trans-cinnamate 4-monooxygenase", "cinnamate 4-hydroxylase", "P48522", "Q43240"}},
		normalizeAliasKey("XP_015639656"):   {Label: "C4H", Queries: []string{"trans-cinnamate 4-monooxygenase", "cinnamate 4-hydroxylase", "P48522", "Q43240"}},
		normalizeAliasKey("OsC4H1"):         {Label: "C4H", Queries: []string{"trans-cinnamate 4-monooxygenase", "cinnamate 4-hydroxylase", "P48522", "Q43240"}},
		normalizeAliasKey("CYP73A38"):       {Label: "C4H", Queries: []string{"trans-cinnamate 4-monooxygenase", "cinnamate 4-hydroxylase", "P48522", "Q43240"}},
	}
)

func (c *Client) SearchKeywordRowsByReportURL(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error) {
	rootDir, identifier, ok := lemnakeyword.LemnaGeneReportKeyword(term)
	if !ok {
		return nil, nil
	}
	release, err := c.keywordReleaseForSpecies(ctx, species)
	if err != nil {
		return nil, err
	}
	if rootDir != "" && !strings.EqualFold(strings.TrimSpace(release.RootDir), rootDir) {
		return nil, nil
	}
	session, err := c.keywordSearchSession(ctx, release, species)
	if err != nil {
		return nil, err
	}
	kind := "gene"
	if strings.Contains(strings.ToUpper(identifier), "_T") {
		kind = "transcript"
	}
	if kind == "transcript" {
		return session.searchIdentifiers([]string{identifier}, limit), nil
	}
	return session.searchIdentifiers([]string{identifier, stripTranscriptSuffix(identifier)}, limit), nil
}

func (c *Client) SearchKeywordRowsByIdentifier(ctx context.Context, species model.SpeciesCandidate, term string, kind string, limit int) ([]model.KeywordResultRow, error) {
	release, err := c.keywordReleaseForSpecies(ctx, species)
	if err != nil {
		return nil, err
	}
	session, err := c.keywordSearchSession(ctx, release, species)
	if err != nil {
		return nil, err
	}
	return session.searchIdentifiers(identifierCandidatesByKind(term, kind), limit), nil
}

func (c *Client) SearchKeywordRowsByLabel(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error) {
	release, err := c.keywordReleaseForSpecies(ctx, species)
	if err != nil {
		return nil, err
	}
	session, err := c.keywordSearchSession(ctx, release, species)
	if err != nil {
		return nil, err
	}
	queries := labelSearchQueries(term)
	if len(queries) == 0 {
		return nil, nil
	}
	if rows := session.searchAliases(expandCuratedLemnaAliases(queries), term, limit); len(rows) > 0 {
		return applyLemnaCuratedLabels(term, rows), nil
	}
	if rows := searchCuratedRiceKeywordTargets(session, term, limit); len(rows) > 0 {
		return rows, nil
	}
	return nil, nil
}

func (c *Client) SearchKeywordRowsByKeywordText(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error) {
	release, err := c.keywordReleaseForSpecies(ctx, species)
	if err != nil {
		return nil, err
	}
	session, err := c.keywordSearchSession(ctx, release, species)
	if err != nil {
		return nil, err
	}
	return session.searchTerms(keywordTerms(term), false, limit), nil
}

func (c *Client) SearchKeywordRowsByWideText(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error) {
	release, err := c.keywordReleaseForSpecies(ctx, species)
	if err != nil {
		return nil, err
	}
	session, err := c.keywordSearchSession(ctx, release, species)
	if err != nil {
		return nil, err
	}
	return (lemnaWideKeywordProgram{}).Search(ctx, session, term, limit)
}

func (c *Client) SearchKeywordRowsByBroadText(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error) {
	release, err := c.keywordReleaseForSpecies(ctx, species)
	if err != nil {
		return nil, err
	}
	session, err := c.keywordSearchSession(ctx, release, species)
	if err != nil {
		return nil, err
	}
	return (lemnaBroadKeywordProgram{}).Search(ctx, session, term, limit)
}

func (c *Client) keywordReleaseForSpecies(ctx context.Context, species model.SpeciesCandidate) (releaseInfo, error) {
	release, err := c.releaseForSpecies(ctx, species)
	if err != nil {
		return releaseInfo{}, err
	}
	if release.GFFURL == "" {
		return releaseInfo{}, fmt.Errorf("no GFF3 file found for %s", species.DisplayLabel())
	}
	return release, nil
}

func (c *Client) keywordSearchSession(ctx context.Context, release releaseInfo, species model.SpeciesCandidate) (*lemnaKeywordSearchSession, error) {
	index, err := c.cachedKeywordIndex(ctx, release, species)
	if err != nil {
		return nil, err
	}
	return &lemnaKeywordSearchSession{
		client:          c,
		index:           index,
		species:         species,
		release:         release,
		identifierCache: make(map[string][]model.KeywordResultRow),
		aliasCache:      make(map[string][]model.KeywordResultRow),
		termCache:       make(map[string][]model.KeywordResultRow),
	}, nil
}

func (s *lemnaKeywordSearchSession) searchIdentifiers(identifiers []string, limit int) []model.KeywordResultRow {
	key := "id|" + strconv.Itoa(limit) + "|" + strings.Join(normalizedIdentifierCandidates(strings.Join(identifiers, "|")), "|")
	if cached, ok := s.identifierCache[key]; ok {
		return cloneKeywordRows(cached)
	}
	rows := searchKeywordIndexIdentifiers(s.index, identifiers, limit)
	s.identifierCache[key] = cloneKeywordRows(rows)
	return cloneKeywordRows(rows)
}

func (s *lemnaKeywordSearchSession) searchAliases(aliases []string, term string, limit int) []model.KeywordResultRow {
	normalized := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		normalized = append(normalized, normalizeAliasKey(alias))
	}
	sort.Strings(normalized)
	key := strings.Join([]string{"alias", strconv.Itoa(limit), normalizeAliasKey(term), strings.Join(normalized, "|")}, "|")
	if cached, ok := s.aliasCache[key]; ok {
		return cloneKeywordRows(cached)
	}
	rows := searchKeywordIndexAliases(s.index, aliases, term, limit)
	s.aliasCache[key] = cloneKeywordRows(rows)
	return cloneKeywordRows(rows)
}

func (s *lemnaKeywordSearchSession) searchTerms(terms []string, loose bool, limit int) []model.KeywordResultRow {
	normalized := normalizeKeywordTerms(terms)
	key := strings.Join([]string{"term", strconv.Itoa(limit), strconv.FormatBool(loose), strings.Join(normalized, "|")}, "|")
	if cached, ok := s.termCache[key]; ok {
		return cloneKeywordRows(cached)
	}
	rows := searchKeywordIndexTerms(s.index, normalized, loose, limit)
	s.termCache[key] = cloneKeywordRows(rows)
	return cloneKeywordRows(rows)
}
func (c *Client) cachedKeywordIndex(ctx context.Context, release releaseInfo, species model.SpeciesCandidate) (lemnaKeywordIndex, error) {
	cacheKey := keywordIndexCacheKey(release, species)
	c.mu.RLock()
	if cached, ok := c.keywordIndexCache[cacheKey]; ok && len(cached.Rows) > 0 {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()
	if cached, ok := readCachedJSON[lemnaKeywordIndex]("keyword-index", cacheKey); ok && len(cached.Rows) > 0 {
		c.mu.Lock()
		c.keywordIndexCache[cacheKey] = cached
		c.mu.Unlock()
		return cached, nil
	}

	value, err, _ := c.sf.Do("keyword-index:"+cacheKey, func() (any, error) {
		c.mu.RLock()
		if cached, ok := c.keywordIndexCache[cacheKey]; ok && len(cached.Rows) > 0 {
			c.mu.RUnlock()
			return cached, nil
		}
		c.mu.RUnlock()
		if cached, ok := readCachedJSON[lemnaKeywordIndex]("keyword-index", cacheKey); ok && len(cached.Rows) > 0 {
			c.mu.Lock()
			c.keywordIndexCache[cacheKey] = cached
			c.mu.Unlock()
			return cached, nil
		}
		index, err := c.buildKeywordIndex(ctx, release, species)
		if err != nil {
			return lemnaKeywordIndex{}, err
		}
		if len(index.Rows) > 0 {
			c.mu.Lock()
			c.keywordIndexCache[cacheKey] = index
			c.mu.Unlock()
			writeCachedJSON("keyword-index", cacheKey, index)
		}
		return index, nil
	})
	if err != nil {
		return lemnaKeywordIndex{}, err
	}
	return value.(lemnaKeywordIndex), nil
}

func (c *Client) buildKeywordIndex(ctx context.Context, release releaseInfo, species model.SpeciesCandidate) (lemnaKeywordIndex, error) {
	rows, err := c.loadKeywordRowsForRelease(ctx, release, species)
	if err != nil {
		return lemnaKeywordIndex{}, err
	}
	index := lemnaKeywordIndex{
		Release:          release,
		Species:          species,
		Rows:             rows,
		ByIdentifier:     make(map[string][]int),
		ByAlias:          make(map[string][]int),
		BySearchToken:    make(map[string][]int),
		ByNormalizedText: make(map[string][]int),
	}
	for i := range index.Rows {
		row := index.Rows[i]
		for _, id := range keywordRowIdentifiers(row) {
			for _, candidate := range normalizedIdentifierCandidates(id) {
				addKeywordIndexHit(index.ByIdentifier, normalizeIdentifierKey(candidate), i)
			}
		}
		for _, alias := range keywordRowAliasCandidates(row) {
			addKeywordIndexHit(index.ByAlias, normalizeAliasKey(alias), i)
		}
		for _, token := range keywordRowSearchTokens(row) {
			addKeywordIndexHit(index.BySearchToken, normalizeIdentifierKey(token), i)
		}
		loose := normalizeSearchLoose(keywordRowSearchText(row))
		tight := normalizeSearchTight(keywordRowSearchText(row))
		for _, token := range strings.Fields(loose) {
			addKeywordIndexHit(index.ByNormalizedText, token, i)
		}
		if tight != "" {
			addKeywordIndexHit(index.ByNormalizedText, tight, i)
		}
	}
	c.ensureKeywordDerivedTranscriptCache(release, index.Rows)
	return index, nil
}

func (c *Client) loadKeywordRowsForRelease(ctx context.Context, release releaseInfo, species model.SpeciesCandidate) ([]model.KeywordResultRow, error) {
	cacheKey := keywordIndexCacheKey(release, species)
	c.mu.RLock()
	if cached, ok := c.keywordRowsCache[cacheKey]; ok && len(cached) > 0 {
		c.mu.RUnlock()
		return cloneKeywordRows(cached), nil
	}
	c.mu.RUnlock()

	reader, closeFn, err := c.openMaybeGzip(ctx, release.GFFURL)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	rows := make([]model.KeywordResultRow, 0, 4096)
	rowByTranscript := make(map[string]int)
	rowByGene := make(map[string]int)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		gff, ok := parseGFF3Line(line)
		if !ok || !isSearchableFeatureType(gff.Type) {
			continue
		}
		row := buildKeywordRowFromGFF(species, release, "", gff)
		addIndexedKeywordRow(&rows, rowByTranscript, rowByGene, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan GFF3 %s: %w", release.GFFURL, err)
	}

	if ahrd, err := c.loadAHRDRecords(ctx, release); err == nil && len(ahrd) > 0 {
		for transcriptID, record := range ahrd {
			if idx, ok := findKeywordRowIndexForAHRD(rowByTranscript, rowByGene, transcriptID); ok {
				enrichKeywordRowWithAHRD(&rows[idx], "", record)
				continue
			}
			row := keywordRowFromAHRD(species, release, "", transcriptID, record)
			addIndexedKeywordRow(&rows, rowByTranscript, rowByGene, row)
		}
	}
	if len(rows) > 0 {
		stored := cloneKeywordRows(rows)
		c.mu.Lock()
		if c.keywordRowsCache == nil {
			c.keywordRowsCache = make(map[string][]model.KeywordResultRow)
		}
		c.keywordRowsCache[cacheKey] = stored
		if c.keywordRowsByGFFCache == nil {
			c.keywordRowsByGFFCache = make(map[string][]model.KeywordResultRow)
		}
		gffKey := strings.TrimSpace(release.GFFURL)
		if gffKey != "" {
			c.keywordRowsByGFFCache[gffKey] = cloneKeywordRows(stored)
		}
		c.mu.Unlock()
	}
	return cloneKeywordRows(rows), nil
}

func (c *Client) ensureKeywordDerivedTranscriptCache(release releaseInfo, rows []model.KeywordResultRow) {
	cacheKey := strings.TrimSpace(release.GFFURL)
	if cacheKey == "" || len(rows) == 0 {
		return
	}
	c.mu.RLock()
	if _, ok := c.proteinTranscriptCache[cacheKey]; ok {
		c.mu.RUnlock()
		return
	}
	c.mu.RUnlock()
	if cached, ok := readCachedJSON[proteinTranscriptDisk]("protein-transcript", cacheKey); ok && len(cached.ProtToTrans) > 0 {
		return
	}

	protToTrans, transToGene := deriveProteinTranscriptMapsFromRows(rows)
	if len(protToTrans) == 0 && len(transToGene) == 0 {
		return
	}
	c.mu.Lock()
	if c.proteinTranscriptCache == nil {
		c.proteinTranscriptCache = make(map[string]proteinTranscriptMaps)
	}
	if _, ok := c.proteinTranscriptCache[cacheKey]; !ok {
		c.proteinTranscriptCache[cacheKey] = proteinTranscriptMaps{
			protToTrans: protToTrans,
			transToGene: transToGene,
		}
	}
	c.mu.Unlock()
	writeCachedJSON("protein-transcript", cacheKey, proteinTranscriptDisk{
		ProtToTrans: protToTrans,
		TransToGene: transToGene,
	})
}

func keywordIndexCacheKey(release releaseInfo, species model.SpeciesCandidate) string {
	return strings.Join([]string{lemnaKeywordIndexSchemaVersion, strings.TrimSpace(species.JBrowseName), strconv.Itoa(species.ProteomeID), strings.TrimSpace(release.ReleaseDir), strings.TrimSpace(release.GFFURL), strings.TrimSpace(release.AHRDURL)}, "|")
}

func addKeywordIndexHit(index map[string][]int, key string, rowIndex int) {
	key = strings.TrimSpace(strings.ToLower(key))
	if key == "" {
		return
	}
	hits := index[key]
	if len(hits) > 0 && hits[len(hits)-1] == rowIndex {
		return
	}
	index[key] = append(hits, rowIndex)
}

func addIndexedKeywordRow(rows *[]model.KeywordResultRow, rowByTranscript map[string]int, rowByGene map[string]int, row model.KeywordResultRow) {
	key := firstNonEmpty(row.TranscriptID, row.GeneIdentifier, row.SequenceID, row.Location)
	if key != "" {
		for _, candidate := range normalizedIdentifierCandidates(key) {
			if idx, ok := rowByTranscript[normalizeIdentifierKey(candidate)]; ok {
				mergeKeywordRow(&(*rows)[idx], row)
				return
			}
			if idx, ok := rowByGene[normalizeIdentifierKey(candidate)]; ok {
				mergeKeywordRow(&(*rows)[idx], row)
				return
			}
		}
	}
	idx := len(*rows)
	*rows = append(*rows, row)
	for _, id := range []string{row.TranscriptID, row.SequenceID, row.ProteinID} {
		for _, candidate := range normalizedIdentifierCandidates(id) {
			rowByTranscript[normalizeIdentifierKey(candidate)] = idx
		}
	}
	for _, id := range []string{row.GeneIdentifier, stripTranscriptSuffix(row.TranscriptID)} {
		for _, candidate := range normalizedIdentifierCandidates(id) {
			rowByGene[normalizeIdentifierKey(candidate)] = idx
		}
	}
}

func findKeywordRowIndexForAHRD(rowByTranscript map[string]int, rowByGene map[string]int, transcriptID string) (int, bool) {
	for _, candidate := range normalizedIdentifierCandidates(transcriptID) {
		if idx, ok := rowByTranscript[normalizeIdentifierKey(candidate)]; ok {
			return idx, true
		}
	}
	for _, candidate := range normalizedIdentifierCandidates(stripTranscriptSuffix(transcriptID)) {
		if idx, ok := rowByGene[normalizeIdentifierKey(candidate)]; ok {
			return idx, true
		}
	}
	return 0, false
}

func mergeKeywordRow(dst *model.KeywordResultRow, src model.KeywordResultRow) {
	if dst == nil {
		return
	}
	dst.LabelName = firstNonEmpty(dst.LabelName, src.LabelName)
	dst.ProteinID = firstNonEmpty(dst.ProteinID, src.ProteinID)
	dst.TranscriptID = firstNonEmpty(dst.TranscriptID, src.TranscriptID)
	dst.GeneIdentifier = firstNonEmpty(dst.GeneIdentifier, src.GeneIdentifier)
	dst.Genome = firstNonEmpty(dst.Genome, src.Genome)
	dst.Location = firstNonEmpty(dst.Location, src.Location)
	dst.Aliases = mergeDelimitedValues(dst.Aliases, src.Aliases)
	dst.UniProt = mergeDelimitedValues(dst.UniProt, src.UniProt)
	dst.Description = firstNonEmpty(dst.Description, src.Description)
	dst.Comments = mergeDelimitedValues(dst.Comments, src.Comments)
	dst.AutoDefine = firstNonEmpty(dst.AutoDefine, src.AutoDefine)
	dst.GeneReportURL = firstNonEmpty(dst.GeneReportURL, src.GeneReportURL)
	dst.SequenceHeaderLabel = firstNonEmpty(dst.SequenceHeaderLabel, src.SequenceHeaderLabel)
	dst.SequenceID = firstNonEmpty(dst.SequenceID, src.SequenceID)
	if src.ExtraColumns != nil {
		dst.ExtraColumns = ensureExtraColumns(dst.ExtraColumns)
		for k, v := range src.ExtraColumns {
			if _, ok := dst.ExtraColumns[k]; !ok || strings.TrimSpace(dst.ExtraColumns[k]) == "" {
				dst.ExtraColumns[k] = v
			}
		}
	}
}

func keywordRowFromAHRD(species model.SpeciesCandidate, release releaseInfo, searchTerm string, transcriptID string, record ahrdRecord) model.KeywordResultRow {
	row := model.KeywordResultRow{
		SourceDatabase:      "lemna",
		SearchTerm:          searchTerm,
		LabelName:           keywordShortLabelFromAHRD(searchTerm, record),
		ProteinID:           record.ProteinAccession,
		TranscriptID:        transcriptID,
		GeneIdentifier:      stripTranscriptSuffix(transcriptID),
		Genome:              species.DisplayLabel(),
		Description:         record.HumanReadableDescription,
		SequenceHeaderLabel: species.DisplayLabel(),
		SequenceID:          firstNonEmpty(record.ProteinAccession, transcriptID),
		GeneReportURL:       lemnaGeneReportURL(release.RootDir, stripTranscriptSuffix(transcriptID)),
	}
	enrichKeywordRowWithAHRD(&row, searchTerm, record)
	return row
}

func enrichKeywordRowWithAHRD(row *model.KeywordResultRow, searchTerm string, record ahrdRecord) {
	if row == nil {
		return
	}
	row.LabelName = firstNonEmpty(row.LabelName, keywordShortLabelFromAHRD(searchTerm, record))
	row.ProteinID = firstNonEmpty(row.ProteinID, record.ProteinAccession)
	row.Description = firstNonEmpty(row.Description, record.HumanReadableDescription)
	row.UniProt = mergeDelimitedValues(row.UniProt, uniprotAccessionFromAHRD(record))
	row.ExtraColumns = ensureExtraColumns(row.ExtraColumns)
	row.ExtraColumns["ahrd_protein_accession"] = record.ProteinAccession
	row.ExtraColumns["ahrd_blast_hit_accession"] = record.BlastHitAccession
	row.ExtraColumns["ahrd_quality_code"] = record.QualityCode
	row.ExtraColumns["ahrd_human_readable_description"] = record.HumanReadableDescription
	row.ExtraColumns["ahrd_interpro"] = record.Interpro
	row.ExtraColumns["ahrd_gene_ontology_term"] = record.GeneOntologyTerm
}
func (lemnaReportURLProgram) Name() string { return lemnaSearchTypeReportURL }
func (lemnaReportURLProgram) Match(term string) bool {
	_, _, ok := lemnakeyword.LemnaGeneReportKeyword(term)
	return ok
}
func (lemnaReportURLProgram) Search(ctx context.Context, session *lemnaKeywordSearchSession, term string, limit int) ([]model.KeywordResultRow, error) {
	_, identifier, ok := lemnakeyword.LemnaGeneReportKeyword(term)
	if !ok {
		return nil, nil
	}
	return session.searchIdentifiers([]string{identifier}, limit), nil
}

func (lemnaIdentifierProgram) Name() string { return lemnaSearchTypeIdentifier }
func (lemnaIdentifierProgram) Match(term string) bool {
	return looksLikeSpecificKeywordIdentifier(term)
}
func (lemnaIdentifierProgram) Search(ctx context.Context, session *lemnaKeywordSearchSession, term string, limit int) ([]model.KeywordResultRow, error) {
	return session.searchIdentifiers(specificKeywordIdentifierVariants(term), limit), nil
}

func (lemnaRiceLocusProgram) Name() string { return lemnaSearchTypeRiceLocus }
func (lemnaRiceLocusProgram) Match(term string) bool {
	return riceLocusPattern.MatchString(normalizeRiceLocusCandidate(term))
}
func (lemnaRiceLocusProgram) Search(ctx context.Context, session *lemnaKeywordSearchSession, term string, limit int) ([]model.KeywordResultRow, error) {
	if aliases := expandCuratedLemnaAliases(aliasesForNormalizedTerm(curatedRiceLocusAliasMap(), term)); len(aliases) > 0 {
		if rows := session.searchAliases(aliases, term, limit); len(rows) > 0 {
			return applyLemnaCuratedLabels(term, rows), nil
		}
	}
	rows := session.searchAliases(expandCuratedLemnaAliases(riceLocusVariants(term)), term, limit)
	if len(rows) > 0 {
		return applyLemnaCuratedLabels(term, rows), nil
	}
	return searchCuratedRiceKeywordTargets(session, term, limit), nil
}

func (lemnaRefSeqProteinProgram) Name() string { return lemnaSearchTypeRefSeqProtein }
func (lemnaRefSeqProteinProgram) Match(term string) bool {
	return refSeqProteinPattern.MatchString(strings.TrimSpace(term))
}
func (lemnaRefSeqProteinProgram) Search(ctx context.Context, session *lemnaKeywordSearchSession, term string, limit int) ([]model.KeywordResultRow, error) {
	if aliases := expandCuratedLemnaAliases(aliasesForNormalizedTerm(curatedRiceRefSeqAliasMap(), term)); len(aliases) > 0 {
		if rows := session.searchAliases(aliases, term, limit); len(rows) > 0 {
			return applyLemnaCuratedLabels(term, rows), nil
		}
	}
	if rows := session.searchIdentifiers(specificKeywordIdentifierVariants(term), limit); len(rows) > 0 {
		return applyLemnaCuratedLabels(term, rows), nil
	}
	return searchCuratedRiceKeywordTargets(session, term, limit), nil
}

func (lemnaRiceAliasProgram) Name() string { return lemnaSearchTypeGeneAlias }
func (lemnaRiceAliasProgram) Match(term string) bool {
	term = strings.TrimSpace(term)
	if term == "" || strings.ContainsAny(term, " \t") {
		return false
	}
	if lemnaTranscriptPattern.MatchString(term) || lemnaGenePattern.MatchString(term) ||
		riceLocusPattern.MatchString(normalizeRiceLocusCandidate(term)) ||
		refSeqProteinPattern.MatchString(term) ||
		cytochromeP450Pattern.MatchString(term) {
		return false
	}
	return isGeneAliasLike(term)
}
func (lemnaRiceAliasProgram) Search(ctx context.Context, session *lemnaKeywordSearchSession, term string, limit int) ([]model.KeywordResultRow, error) {
	aliases := expandCuratedLemnaAliases([]string{term})
	if rows := session.searchAliases(aliases, term, limit); len(rows) > 0 {
		return applyLemnaCuratedLabels(term, rows), nil
	}
	return searchCuratedRiceKeywordTargets(session, term, limit), nil
}

func (lemnaCytochromeFamilyProgram) Name() string { return lemnaSearchTypeCytochromeFamily }
func (lemnaCytochromeFamilyProgram) Match(term string) bool {
	return cytochromeP450Pattern.MatchString(strings.TrimSpace(term))
}
func (lemnaCytochromeFamilyProgram) Search(ctx context.Context, session *lemnaKeywordSearchSession, term string, limit int) ([]model.KeywordResultRow, error) {
	queries := []string{term, strings.TrimSuffix(strings.ToUpper(strings.TrimSpace(term)), "P")}
	queries = expandCuratedLemnaAliases(queries)
	if rows := session.searchAliases(queries, term, limit); len(rows) > 0 {
		return applyLemnaCuratedLabels(term, rows), nil
	}
	return searchCuratedRiceKeywordTargets(session, term, limit), nil
}

func (lemnaKeywordProgramDefault) Name() string           { return lemnaSearchTypeKeyword }
func (lemnaKeywordProgramDefault) Match(term string) bool { return strings.TrimSpace(term) != "" }
func (lemnaKeywordProgramDefault) Search(ctx context.Context, session *lemnaKeywordSearchSession, term string, limit int) ([]model.KeywordResultRow, error) {
	return session.searchTerms(keywordTerms(term), false, limit), nil
}

func (lemnaWideKeywordProgram) Name() string           { return lemnaSearchTypeWide }
func (lemnaWideKeywordProgram) Match(term string) bool { return strings.TrimSpace(term) != "" }
func (lemnaWideKeywordProgram) Search(ctx context.Context, session *lemnaKeywordSearchSession, term string, limit int) ([]model.KeywordResultRow, error) {
	seen := make(map[string]struct{})
	rows := make([]model.KeywordResultRow, 0, 16)
	add := func(found []model.KeywordResultRow) {
		for _, row := range found {
			if addKeywordRow(&rows, seen, row, limit) {
				return
			}
		}
	}
	for _, aliases := range [][]string{
		expandCuratedLemnaAliases([]string{term}),
		expandCuratedLemnaAliases(aliasesForNormalizedTerm(curatedRiceRefSeqAliasMap(), term)),
		expandCuratedLemnaAliases(riceLocusVariants(term)),
		specificKeywordIdentifierVariants(term),
	} {
		if len(aliases) == 0 {
			continue
		}
		add(session.searchAliases(aliases, term, limit))
		if len(rows) > 0 {
			return applyLemnaCuratedLabels(term, rows), nil
		}
	}
	add(searchCuratedRiceKeywordTargets(session, term, limit))
	if len(rows) > 0 {
		return applyLemnaCuratedLabels(term, rows), nil
	}
	add(session.searchTerms(keywordTerms(wideKeywordQuery(term)), true, limit))
	if len(rows) > 0 {
		return applyLemnaCuratedLabels(term, rows), nil
	}
	for _, query := range relaxedKeywordQueries(term) {
		add(session.searchTerms(keywordTerms(query), true, limit))
		if len(rows) > 0 {
			return applyLemnaCuratedLabels(term, rows), nil
		}
	}
	return applyLemnaCuratedLabels(term, rows), nil
}

func (lemnaBroadKeywordProgram) Name() string           { return lemnaSearchTypeBroad }
func (lemnaBroadKeywordProgram) Match(term string) bool { return strings.TrimSpace(term) != "" }
func (lemnaBroadKeywordProgram) Search(ctx context.Context, session *lemnaKeywordSearchSession, term string, limit int) ([]model.KeywordResultRow, error) {
	if limit <= 0 {
		limit = 10000
	}
	rows, err := (lemnaWideKeywordProgram{}).Search(ctx, session, term, limit)
	if err != nil || len(rows) > 0 {
		return rows, err
	}
	return session.searchTerms(keywordTerms(term), true, limit), nil
}

func searchKeywordIndexIdentifiers(index lemnaKeywordIndex, identifiers []string, limit int) []model.KeywordResultRow {
	seen := make(map[string]struct{})
	rows := make([]model.KeywordResultRow, 0, len(identifiers))
	for _, identifier := range identifiers {
		for _, candidate := range normalizedIdentifierCandidates(identifier) {
			for _, rowIndex := range index.ByIdentifier[normalizeIdentifierKey(candidate)] {
				if rowIndex < 0 || rowIndex >= len(index.Rows) {
					continue
				}
				if addKeywordRow(&rows, seen, index.Rows[rowIndex], limit) {
					return rows
				}
			}
		}
	}
	return rows
}

func searchKeywordIndexAliases(index lemnaKeywordIndex, aliases []string, term string, limit int) []model.KeywordResultRow {
	if len(aliases) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	rows := make([]model.KeywordResultRow, 0, len(aliases))
	for _, alias := range aliases {
		for _, rowIndex := range index.ByAlias[normalizeAliasKey(alias)] {
			if rowIndex < 0 || rowIndex >= len(index.Rows) {
				continue
			}
			if addKeywordRow(&rows, seen, index.Rows[rowIndex], limit) {
				return rows
			}
		}
		if len(rows) > 0 {
			continue
		}
		for _, row := range searchKeywordIndexIdentifiers(index, []string{alias}, limit) {
			if addKeywordRow(&rows, seen, row, limit) {
				return rows
			}
		}
		if len(rows) > 0 {
			continue
		}
		for _, row := range searchKeywordIndexTerms(index, keywordTerms(alias), true, limit) {
			if addKeywordRow(&rows, seen, row, limit) {
				return rows
			}
		}
	}
	return rows
}

func searchKeywordIndexTerms(index lemnaKeywordIndex, terms []string, loose bool, limit int) []model.KeywordResultRow {
	terms = normalizeKeywordTerms(terms)
	if len(terms) == 0 {
		return nil
	}
	candidateCounts := make(map[int]int)
	for _, term := range terms {
		keys := []string{normalizeSearchLoose(term), normalizeSearchTight(term), normalizeIdentifierKey(term)}
		seenForTerm := make(map[int]struct{})
		for _, key := range keys {
			if key == "" {
				continue
			}
			for _, rowIndex := range index.BySearchToken[key] {
				seenForTerm[rowIndex] = struct{}{}
			}
			for _, rowIndex := range index.ByNormalizedText[key] {
				seenForTerm[rowIndex] = struct{}{}
			}
		}
		for rowIndex := range seenForTerm {
			candidateCounts[rowIndex]++
		}
	}
	rows := make([]model.KeywordResultRow, 0, 16)
	seen := make(map[string]struct{})
	for rowIndex, matched := range candidateCounts {
		if rowIndex < 0 || rowIndex >= len(index.Rows) {
			continue
		}
		if !loose && matched < len(terms) {
			continue
		}
		row := index.Rows[rowIndex]
		if loose {
			if !rowMatchesAnyTerm(row, terms) {
				continue
			}
		} else if !rowMatchesTerms(row, terms) {
			continue
		}
		if addKeywordRow(&rows, seen, row, limit) {
			return rows
		}
	}
	sortKeywordRows(rows)
	return rows
}
func cloneKeywordRows(rows []model.KeywordResultRow) []model.KeywordResultRow {
	out := append([]model.KeywordResultRow(nil), rows...)
	for i := range out {
		if out[i].ExtraColumns != nil {
			extra := make(map[string]string, len(out[i].ExtraColumns))
			for k, v := range out[i].ExtraColumns {
				extra[k] = v
			}
			out[i].ExtraColumns = extra
		}
	}
	return out
}

func identifierCandidatesByKind(term string, kind string) []string {
	candidates := specificKeywordIdentifierVariants(term)
	if strings.EqualFold(strings.TrimSpace(kind), "gene") {
		geneID := stripTranscriptSuffix(term)
		if geneID != "" {
			candidates = append(candidates, specificKeywordIdentifierVariants(geneID)...)
		}
	}
	return uniqueNormalizedStrings(candidates)
}

func looksLikeSpecificKeywordIdentifier(value string) bool {
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

func specificKeywordIdentifierVariants(value string) []string {
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
	for _, candidate := range normalizedIdentifierCandidates(value) {
		add(candidate)
	}
	return variants
}

func riceLocusVariants(term string) []string {
	normalized := normalizeRiceLocusCandidate(term)
	if normalized == "" || !riceLocusPattern.MatchString(normalized) {
		return specificKeywordIdentifierVariants(term)
	}
	return specificKeywordIdentifierVariants("LOC_" + normalized)
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

func osC4HLike(term string) bool {
	normalized := strings.ToUpper(strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.TrimSpace(term)))
	return strings.HasPrefix(normalized, "OSC4H") && len(normalized) > len("OSC4H")
}

func aliasesForNormalizedTerm(catalog map[string][]string, term string) []string {
	key := normalizeAliasKey(term)
	values := catalog[key]
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func expandCuratedLemnaAliases(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values)*2)
	seen := make(map[string]struct{}, len(values)*2)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := normalizeAliasKey(value)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	for _, value := range values {
		add(value)
		if aliases := aliasesForNormalizedTerm(curatedRiceLocusAliasMap(), value); len(aliases) > 0 {
			for _, alias := range aliases {
				add(alias)
			}
		}
	}
	return out
}

func labelSearchQueries(term string) []string {
	queries := []string{term}
	if aliases := aliasesForNormalizedTerm(curatedRiceRefSeqAliasMap(), term); len(aliases) > 0 {
		queries = append(queries, aliases...)
	}
	if aliases := aliasesForNormalizedTerm(curatedRiceLocusAliasMap(), term); len(aliases) > 0 {
		queries = append(queries, aliases...)
	}
	if riceLocusPattern.MatchString(normalizeRiceLocusCandidate(term)) {
		queries = append(queries, riceLocusVariants(term)...)
	}
	if refSeqProteinPattern.MatchString(strings.TrimSpace(term)) {
		queries = append(queries, specificKeywordIdentifierVariants(term)...)
	}
	if cytochromeP450Pattern.MatchString(strings.TrimSpace(term)) {
		queries = append(queries, strings.TrimSuffix(strings.ToUpper(strings.TrimSpace(term)), "P"))
	}
	return uniqueNormalizedStrings(queries)
}

func curatedRiceRefSeqAliasMap() map[string][]string {
	return curatedRiceRefSeqAliasLookup
}

func curatedRiceLocusAliasMap() map[string][]string {
	return curatedRiceLocusAliasLookup
}

func curatedRiceKeywordTargets() map[string]lemnaCuratedKeywordTarget {
	return curatedRiceKeywordTargetLookup
}

func wideKeywordQuery(term string) string {
	return strings.ReplaceAll(strings.TrimSpace(term), "_", " ")
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
	return strings.NewReplacer("_", "", "-", "", " ", "", ".", "").Replace(value)
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

type lemnaCuratedKeywordTarget struct {
	Label   string
	Queries []string
}

func curatedRiceKeywordTarget(term string) (lemnaCuratedKeywordTarget, bool) {
	keys := []string{normalizeAliasKey(term)}
	keys = append(keys, normalizedIdentifierCandidates(term)...)
	if aliases := aliasesForNormalizedTerm(curatedRiceRefSeqAliasMap(), term); len(aliases) > 0 {
		keys = append(keys, aliases...)
	}
	if aliases := aliasesForNormalizedTerm(curatedRiceLocusAliasMap(), term); len(aliases) > 0 {
		keys = append(keys, aliases...)
	}
	if normalized := normalizeRiceLocusCandidate(term); normalized != "" {
		keys = append(keys, normalized, "LOC_"+normalized)
	}
	targets := curatedRiceKeywordTargets()
	for _, key := range keys {
		if target, ok := targets[normalizeAliasKey(key)]; ok {
			return target, true
		}
	}
	return lemnaCuratedKeywordTarget{}, false
}

func searchCuratedRiceKeywordTargets(session *lemnaKeywordSearchSession, term string, limit int) []model.KeywordResultRow {
	target, ok := curatedRiceKeywordTarget(term)
	if !ok {
		return nil
	}
	seen := make(map[string]struct{})
	rows := make([]model.KeywordResultRow, 0, 8)
	for _, query := range target.Queries {
		for _, row := range session.searchTerms(keywordTerms(query), true, limit) {
			if addKeywordRow(&rows, seen, row, limit) {
				return applyLemnaCuratedLabel(target.Label, rows)
			}
		}
		if len(rows) > 0 {
			break
		}
	}
	return applyLemnaCuratedLabel(target.Label, rows)
}

func applyLemnaCuratedLabels(term string, rows []model.KeywordResultRow) []model.KeywordResultRow {
	target, ok := curatedRiceKeywordTarget(term)
	if !ok {
		return rows
	}
	return applyLemnaCuratedLabel(target.Label, rows)
}

func applyLemnaCuratedLabel(label string, rows []model.KeywordResultRow) []model.KeywordResultRow {
	label = strings.TrimSpace(label)
	if label == "" || len(rows) == 0 {
		return rows
	}
	out := cloneKeywordRows(rows)
	for i := range out {
		out[i].LabelName = label
	}
	return out
}

func normalizeIdentifierKey(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func normalizeKeywordTerms(terms []string) []string {
	out := make([]string, 0, len(terms))
	seen := make(map[string]struct{})
	for _, term := range terms {
		term = strings.TrimSpace(strings.ToLower(term))
		if term == "" {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		out = append(out, term)
	}
	return out
}

func rowMatchesAnyTerm(row model.KeywordResultRow, terms []string) bool {
	values := keywordRowSearchValues(row)
	for _, term := range terms {
		if textValuesMatchTerms(values, []string{term}) {
			return true
		}
	}
	return false
}

func keywordRowIdentifiers(row model.KeywordResultRow) []string {
	values := []string{row.ProteinID, row.TranscriptID, row.GeneIdentifier, row.SequenceID}
	if row.ExtraColumns != nil {
		for _, key := range []string{"attr_ID", "attr_Name", "attr_Parent", "attr_protein_id", "attr_protein", "attr_protein_accession", "ahrd_protein_accession", "ahrd_blast_hit_accession"} {
			values = append(values, row.ExtraColumns[key])
		}
	}
	return values
}

func keywordRowSearchTokens(row model.KeywordResultRow) []string {
	values := keywordRowIdentifiers(row)
	values = append(values, keywordRowAliasCandidates(row)...)
	values = append(values, strings.FieldsFunc(row.Aliases, splitKeywordToken)...)
	values = append(values, strings.FieldsFunc(row.UniProt, splitKeywordToken)...)
	if row.ExtraColumns != nil {
		for _, key := range []string{"attr_Alias", "attr_Dbxref", "ahrd_interpro", "ahrd_gene_ontology_term"} {
			values = append(values, strings.FieldsFunc(row.ExtraColumns[key], splitKeywordToken)...)
		}
	}
	return values
}

func keywordRowAliasCandidates(row model.KeywordResultRow) []string {
	values := []string{row.LabelName, row.Aliases, row.UniProt}
	if row.ExtraColumns != nil {
		for _, key := range []string{
			"attr_Alias",
			"attr_alias",
			"attr_Name",
			"attr_gene_name",
			"attr_gene_symbol",
			"attr_symbol",
			"attr_gene",
			"attr_Dbxref",
			"ahrd_blast_hit_accession",
		} {
			values = append(values, row.ExtraColumns[key])
		}
	}
	out := make([]string, 0, len(values)*2)
	seen := make(map[string]struct{}, len(values)*2)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := normalizeAliasKey(value)
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	for _, value := range values {
		add(value)
		for _, token := range strings.FieldsFunc(value, splitKeywordToken) {
			add(token)
		}
	}
	return out
}

func splitKeywordToken(r rune) bool {
	switch r {
	case ';', ',', '|', '\t', '\n', '\r', ' ', ':':
		return true
	default:
		return false
	}
}

func keywordRowSearchValues(row model.KeywordResultRow) []string {
	values := []string{row.LabelName, row.ProteinID, row.TranscriptID, row.GeneIdentifier, row.Aliases, row.UniProt, row.Description, row.Comments, row.AutoDefine, row.SequenceID, row.GeneReportURL}
	if row.ExtraColumns != nil {
		for _, key := range []string{"attr_ID", "attr_Name", "attr_Parent", "attr_Alias", "attr_Dbxref", "attr_product", "attr_description", "attr_Note", "attr_note", "ahrd_protein_accession", "ahrd_blast_hit_accession", "ahrd_human_readable_description", "ahrd_interpro", "ahrd_gene_ontology_term"} {
			values = append(values, row.ExtraColumns[key])
		}
	}
	return values
}

func keywordRowSearchText(row model.KeywordResultRow) string {
	return strings.Join(keywordRowSearchValues(row), " ")
}

func mergeDelimitedValues(left string, right string) string {
	values := make([]string, 0, 4)
	values = append(values, strings.FieldsFunc(left, splitKeywordToken)...)
	values = append(values, strings.FieldsFunc(right, splitKeywordToken)...)
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
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
	return strings.Join(out, "; ")
}

func sortKeywordRows(rows []model.KeywordResultRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		left := rows[i]
		right := rows[j]
		for _, pair := range [][2]string{{left.GeneIdentifier, right.GeneIdentifier}, {left.TranscriptID, right.TranscriptID}, {left.ProteinID, right.ProteinID}, {left.Location, right.Location}} {
			if pair[0] == pair[1] {
				continue
			}
			return strings.ToLower(pair[0]) < strings.ToLower(pair[1])
		}
		return false
	})
}

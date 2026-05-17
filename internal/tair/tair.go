package tair

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/searchengine/tairkeyword"
	"golang.org/x/sync/singleflight"
)

const (
	baseURL     = "https://www.arabidopsis.org"
	downloadAPI = "https://www.arabidopsis.org/api/download-files/download?filePath="
)

type Client struct {
	httpClient    *http.Client
	keywordEngine *tairkeyword.Engine
	mu            sync.RWMutex
	sf            singleflight.Group

	releases       map[string]releaseInfo
	rowIndex       map[string]tairIndex
	familyLists    map[string][]familyCandidate
	proteinSeqs    map[string]map[string]proteinEntry
	nucleotideSeqs map[string]map[string]proteinEntry
	externalSeqs   map[string]proteinEntry
	localResults   map[string]model.BlastResult
}

type releaseInfo struct {
	Name                   string `json:"name"`
	Label                  string `json:"label"`
	ReleaseDate            string `json:"release_date"`
	GFFURL                 string `json:"gff_url"`
	ProteinURL             string `json:"protein_url"`
	NucleotideURL          string `json:"nucleotide_url"`
	CDNAURL                string `json:"cdna_url"`
	CDSURL                 string `json:"cds_url"`
	DescriptionURL         string `json:"description_url"`
	RepresentativeModelURL string `json:"representative_model_url"`
	ReportURLBase          string `json:"report_url_base"`
}

type tairIndex struct {
	Release          releaseInfo              `json:"release"`
	Version          model.SpeciesCandidate   `json:"version"`
	Rows             []model.KeywordResultRow `json:"rows"`
	ByIdentifier     map[string][]int         `json:"by_identifier"`
	ByAlias          map[string][]int         `json:"by_alias"`
	ByToken          map[string][]int         `json:"by_token"`
	ByFamily         map[string][]int         `json:"by_family"`
	FamilyCandidates []familyCandidate        `json:"family_candidates"`
}

type familyCandidate struct {
	Name        string `json:"name"`
	ShortName   string `json:"short_name"`
	Count       int    `json:"count"`
	Key         string `json:"key"`
	ParentKey   string `json:"parent_key"`
	ParentName  string `json:"parent_name"`
	HasChildren bool   `json:"has_children"`
}

type gffRow struct {
	SeqID      string
	Source     string
	Type       string
	Start      string
	End        string
	Score      string
	Strand     string
	Phase      string
	Attributes string
	AttrMap    map[string]string
}

type proteinEntry struct {
	ID          string `json:"id"`
	Header      string `json:"header"`
	Sequence    string `json:"sequence"`
	Description string `json:"description"`
	Symbols     string `json:"symbols"`
}

var (
	spacePattern       = regexp.MustCompile(`\s+`)
	searchNoisePattern = regexp.MustCompile(`[^a-z0-9]+`)
	agiGenePattern     = regexp.MustCompile(`(?i)^AT[1-5CM]G\d{5}$`)
	agiModelPattern    = regexp.MustCompile(`(?i)^AT[1-5CM]G\d{5}\.\d+$`)
	symbolPattern      = regexp.MustCompile(`\b[A-Z][A-Z0-9-]{1,14}\b`)
	familyNoisePattern = regexp.MustCompile(`(?i)\s+superfamily\s+protein$|\s+family\s+protein$|\s+protein$`)
	uniprotPattern     = regexp.MustCompile(`(?i)(?:UniProtKB|UniProt|Swiss-Prot|TrEMBL)[:= ]+([A-NR-Z][0-9][A-Z0-9]{3}[0-9](?:-[0-9]+)?|[OPQ][0-9][A-Z0-9]{3}[0-9](?:-[0-9]+)?)`)
)

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	c := &Client{
		httpClient:     httpClient,
		releases:       make(map[string]releaseInfo),
		rowIndex:       make(map[string]tairIndex),
		familyLists:    make(map[string][]familyCandidate),
		proteinSeqs:    make(map[string]map[string]proteinEntry),
		nucleotideSeqs: make(map[string]map[string]proteinEntry),
		externalSeqs:   make(map[string]proteinEntry),
		localResults:   make(map[string]model.BlastResult),
	}
	for _, rel := range defaultReleases() {
		c.releases[strings.ToLower(rel.Name)] = rel
	}
	return c
}

func (c *Client) Name() string { return "tair" }

func defaultReleases() []releaseInfo {
	return []releaseInfo{
		{
			Name:                   "TAIR12",
			Label:                  "TAIR12",
			ReleaseDate:            "2026-02-01",
			GFFURL:                 downloadURL("Genes/TAIR12_genome_release/TAIR12_1Feb26.gff3.zip"),
			DescriptionURL:         "",
			RepresentativeModelURL: "",
			ReportURLBase:          baseURL + "/servlets/TairObject?type=locus&name=",
		},
		{
			Name:                   "TAIR11",
			Label:                  "TAIR11",
			ReleaseDate:            "unpublished in current public download tree",
			GFFURL:                 "",
			ProteinURL:             "",
			NucleotideURL:          "",
			CDNAURL:                "",
			CDSURL:                 "",
			DescriptionURL:         "",
			RepresentativeModelURL: "",
			ReportURLBase:          baseURL + "/servlets/TairObject?type=locus&name=",
		},
		{
			Name:                   "Araport11",
			Label:                  "Araport11",
			ReleaseDate:            "2025-08-13 annotation; 2022-09-14 blastsets",
			GFFURL:                 downloadURL("Genes/Araport11_genome_release/Araport11_GFF3_genes_transposons.20250813.gff.gz"),
			ProteinURL:             downloadURL("Genes/Araport11_genome_release/Araport11_blastsets/Araport11_pep_20220914_representative_gene_model.gz"),
			NucleotideURL:          downloadURL("Genes/Araport11_genome_release/Araport11_blastsets/Araport11_seq_20220914_representative_gene_model.gz"),
			CDNAURL:                downloadURL("Genes/Araport11_genome_release/Araport11_blastsets/Araport11_cdna_20220914_representative_gene_model.gz"),
			CDSURL:                 downloadURL("Genes/Araport11_genome_release/Araport11_blastsets/Araport11_cds_20220914_representative_gene_model.gz"),
			DescriptionURL:         "",
			RepresentativeModelURL: downloadURL("Genes/Araport11_genome_release/Araport11_TAIRAccessionID_AGI_mapping.txt"),
			ReportURLBase:          baseURL + "/servlets/TairObject?type=locus&name=",
		},
		{
			Name:                   "TAIR10",
			Label:                  "TAIR10",
			ReleaseDate:            "2010-12",
			GFFURL:                 downloadURL("Genes/TAIR10_genome_release/TAIR10_gff3/TAIR10_GFF3_genes_transposons.gff"),
			ProteinURL:             downloadURL("Genes/TAIR10_genome_release/TAIR10_blastsets/TAIR10_pep_20110103_representative_gene_model_updated"),
			NucleotideURL:          downloadURL("Genes/TAIR10_genome_release/TAIR10_chromosome_files/TAIR10_chr_all.fas.gz"),
			CDNAURL:                downloadURL("Genes/TAIR10_genome_release/TAIR10_blastsets/TAIR10_cdna_20110103_representative_gene_model_updated"),
			CDSURL:                 downloadURL("Genes/TAIR10_genome_release/TAIR10_blastsets/TAIR10_cds_20110103_representative_gene_model_updated"),
			DescriptionURL:         downloadURL("Genes/TAIR10_genome_release/TAIR10_functional_descriptions_20130831.txt"),
			RepresentativeModelURL: downloadURL("Genes/TAIR10_genome_release/TAIR10_gene_lists/TAIR10_representative_gene_models"),
			ReportURLBase:          baseURL + "/servlets/TairObject?type=locus&name=",
		},
		{
			Name:                   "TAIR9",
			Label:                  "TAIR9",
			ReleaseDate:            "2009",
			GFFURL:                 downloadURL("Genes/TAIR9_genome_release/tair9_gff3/TAIR9_GFF3_genes_transposons.gff"),
			ProteinURL:             downloadURL("Genes/TAIR9_genome_release/TAIR9_pep_20090619"),
			NucleotideURL:          downloadURL("Genes/TAIR9_genome_release/TAIR9_chr_all.fas"),
			CDNAURL:                "",
			CDSURL:                 "",
			DescriptionURL:         downloadURL("Genes/TAIR9_genome_release/TAIR9_functional_descriptions"),
			RepresentativeModelURL: downloadURL("Genes/TAIR9_genome_release/TAIR9_representative_gene_model.txt"),
			ReportURLBase:          baseURL + "/servlets/TairObject?type=locus&name=",
		},
		{
			Name:                   "TAIR8",
			Label:                  "TAIR8",
			ReleaseDate:            "2008",
			GFFURL:                 downloadURL("Genes/TAIR8_genome_release/tair8_gff3/TAIR8_GFF3_genes_transposons.gff"),
			ProteinURL:             downloadURL("Genes/TAIR8_genome_release/TAIR8_pep_20080412"),
			NucleotideURL:          downloadURL("Genes/TAIR8_genome_release/tair8.at.chromosomes.fas"),
			CDNAURL:                "",
			CDSURL:                 "",
			DescriptionURL:         downloadURL("Genes/TAIR8_genome_release/TAIR8_functional_descriptions"),
			RepresentativeModelURL: downloadURL("Genes/TAIR8_genome_release/TAIR8_representative_gene_model"),
			ReportURLBase:          baseURL + "/servlets/TairObject?type=locus&name=",
		},
		{
			Name:                   "TAIR7",
			Label:                  "TAIR7",
			ReleaseDate:            "2007",
			GFFURL:                 downloadURL("Genes/TAIR7_genome_release/TAIR7_gff3/TAIR7_GFF3_genes.gff"),
			ProteinURL:             downloadURL("Genes/TAIR7_genome_release/TAIR7_pep_20070425"),
			NucleotideURL:          "",
			CDNAURL:                "",
			CDSURL:                 "",
			DescriptionURL:         downloadURL("Genes/TAIR7_genome_release/TAIR7_functional_descriptions"),
			RepresentativeModelURL: "",
			ReportURLBase:          baseURL + "/servlets/TairObject?type=locus&name=",
		},
		{
			Name:                   "TAIR6",
			Label:                  "TAIR6",
			ReleaseDate:            "2006-09",
			GFFURL:                 downloadURL("Genes/TAIR6_genome_release/TAIR6_GFF3_genes.gff"),
			ProteinURL:             downloadURL("Genes/TAIR6_genome_release/TAIR6_pep_20060907"),
			NucleotideURL:          downloadURL("Genes/TAIR6_genome_release/TAIR6_seq_20060907"),
			CDNAURL:                downloadURL("Genes/TAIR6_genome_release/TAIR6_cdna_20060907"),
			CDSURL:                 downloadURL("Genes/TAIR6_genome_release/TAIR6_cds_20060907"),
			DescriptionURL:         "",
			RepresentativeModelURL: "",
			ReportURLBase:          baseURL + "/servlets/TairObject?type=locus&name=",
		},
	}
}

func downloadURL(filePath string) string {
	return downloadAPI + strings.TrimLeft(filePath, "/")
}

func (c *Client) FetchSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {
	_ = ctx
	return defaultVersionCandidates(), nil
}

func (c *Client) FilterCandidatesForMode(candidates []model.SpeciesCandidate, mode string) []model.SpeciesCandidate {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return append([]model.SpeciesCandidate(nil), candidates...)
	}
	out := make([]model.SpeciesCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		rel, err := c.releaseForVersion(candidate)
		if err != nil {
			continue
		}
		if tairReleaseUsableInMode(rel, mode) {
			out = append(out, candidate)
		}
	}
	return out
}

func tairReleaseUsableInMode(rel releaseInfo, mode string) bool {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "keyword", "family":
		return strings.TrimSpace(rel.GFFURL) != ""
	case "blast":
		return strings.TrimSpace(rel.ProteinURL) != "" || strings.TrimSpace(rel.NucleotideURL) != ""
	default:
		return strings.TrimSpace(rel.GFFURL) != "" ||
			strings.TrimSpace(rel.ProteinURL) != "" ||
			strings.TrimSpace(rel.NucleotideURL) != ""
	}
}

func defaultVersionCandidates() []model.SpeciesCandidate {
	releases := defaultReleases()
	out := make([]model.SpeciesCandidate, 0, len(releases))
	for i, rel := range releases {
		out = append(out, model.SpeciesCandidate{
			ProteomeID:  370200 + i + 1,
			JBrowseName: rel.Name,
			GenomeLabel: rel.Label,
			CommonName:  "Arabidopsis thaliana",
			ReleaseDate: rel.ReleaseDate,
			SearchAlias: rel.Name + " Arabidopsis thaliana TAIR",
			IsOfficial:  true,
		})
	}
	return out
}

func FilterSpeciesCandidates(candidates []model.SpeciesCandidate, keyword string) []model.SpeciesCandidate {
	keyword = strings.TrimSpace(strings.ToLower(keyword))
	if keyword == "" {
		return append([]model.SpeciesCandidate(nil), candidates...)
	}
	loose := normalizeSearchLoose(keyword)
	tight := normalizeSearchTight(keyword)
	out := make([]model.SpeciesCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		text := candidate.SearchText()
		if strings.Contains(text, keyword) ||
			(loose != "" && strings.Contains(normalizeSearchLoose(text), loose)) ||
			(tight != "" && strings.Contains(normalizeSearchTight(text), tight)) {
			out = append(out, candidate)
		}
	}
	return out
}

func (c *Client) releaseForVersion(version model.SpeciesCandidate) (releaseInfo, error) {
	name := strings.ToLower(strings.TrimSpace(firstNonEmpty(version.JBrowseName, version.GenomeLabel)))
	if name == "" {
		return releaseInfo{}, fmt.Errorf("empty TAIR version")
	}
	c.mu.RLock()
	rel, ok := c.releases[name]
	c.mu.RUnlock()
	if ok {
		return rel, nil
	}
	for _, rel := range defaultReleases() {
		if strings.EqualFold(rel.Name, name) || strings.EqualFold(rel.Label, name) {
			return rel, nil
		}
	}
	return releaseInfo{}, fmt.Errorf("unsupported TAIR version %q", version.DisplayLabel())
}

func (c *Client) SearchKeywordRows(ctx context.Context, version model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	if c.keywordEngine == nil {
		c.keywordEngine = tairkeyword.New(c)
	}
	return c.keywordEngine.SearchKeywordRows(ctx, version, keyword)
}

func (c *Client) SearchKeywordRowsWide(ctx context.Context, version model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	if c.keywordEngine == nil {
		c.keywordEngine = tairkeyword.New(c)
	}
	return c.keywordEngine.SearchKeywordRowsWide(ctx, version, keyword)
}

func (c *Client) SearchKeywordRowsBroad(ctx context.Context, version model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	if c.keywordEngine == nil {
		c.keywordEngine = tairkeyword.New(c)
	}
	return c.keywordEngine.SearchKeywordRowsBroad(ctx, version, keyword)
}

func (c *Client) SearchFamilyKeywordRows(ctx context.Context, version model.SpeciesCandidate, family string) ([]model.KeywordResultRow, error) {
	if c.keywordEngine == nil {
		c.keywordEngine = tairkeyword.New(c)
	}
	return c.keywordEngine.SearchFamilyKeywordRows(ctx, version, family)
}

func (c *Client) SearchKeywordRowsByReportURL(ctx context.Context, version model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error) {
	_, id, ok := parseTAIRReportKeyword(term)
	if !ok {
		return nil, nil
	}
	return c.SearchKeywordRowsByIdentifier(ctx, version, id, "gene", limit)
}

func (c *Client) SearchKeywordRowsByIdentifier(ctx context.Context, version model.SpeciesCandidate, term string, kind string, limit int) ([]model.KeywordResultRow, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, nil
	}
	if rel, err := c.releaseForVersion(version); err == nil && rel.GFFURL != "" {
		if idx, idxErr := c.cachedIndex(ctx, version); idxErr == nil && len(idx.Rows) > 0 {
			var keys []string
			switch strings.ToLower(strings.TrimSpace(kind)) {
			case "gene":
				keys = identifierKeys(stripTranscriptSuffix(term))
			case "model":
				keys = identifierKeys(term)
			default:
				keys = identifierKeys(term)
				keys = append(keys, identifierKeys(stripTranscriptSuffix(term))...)
				keys = uniqueStrings(keys)
			}
			rows := c.searchKeywordRowsByIdentifiers(idx, keys, limit)
			if len(rows) > 0 {
				for i := range rows {
					rows[i].SearchTerm = term
					rows[i].SearchType = classifySearchType(term)
				}
				return rows, nil
			}
		}
	}
	rows, err := c.searchLiveIdentifierRows(ctx, version, term, kind)
	if err != nil {
		return nil, err
	}
	return limitKeywordRows(rows, limit), nil
}

func (c *Client) SearchKeywordRowsByLabel(ctx context.Context, version model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error) {
	if rel, err := c.releaseForVersion(version); err == nil && rel.GFFURL != "" {
		if idx, idxErr := c.cachedIndex(ctx, version); idxErr == nil && len(idx.Rows) > 0 {
			rows := c.searchKeywordRowsByAliases(idx, term, limit)
			if len(rows) > 0 {
				for i := range rows {
					rows[i].SearchTerm = term
					rows[i].SearchType = tairkeyword.SearchTypeLabelSymbol
				}
				return rows, nil
			}
		}
	}
	rows, err := c.searchLiveLabelRows(ctx, version, term)
	if err != nil {
		return nil, err
	}
	return limitKeywordRows(rows, limit), nil
}

func (c *Client) SearchKeywordRowsByKeywordText(ctx context.Context, version model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error) {
	if rel, err := c.releaseForVersion(version); err == nil && rel.GFFURL != "" {
		if idx, idxErr := c.cachedIndex(ctx, version); idxErr == nil && len(idx.Rows) > 0 {
			rows := c.searchIndex(idx, term, false, limit)
			if len(rows) > 0 {
				return rows, nil
			}
		}
	}
	rows, err := c.searchLiveKeywordRows(ctx, version, term, false)
	if err != nil {
		return nil, err
	}
	return limitKeywordRows(rows, limit), nil
}

func (c *Client) SearchKeywordRowsByWideText(ctx context.Context, version model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error) {
	if rel, err := c.releaseForVersion(version); err == nil && rel.GFFURL != "" {
		if idx, idxErr := c.cachedIndex(ctx, version); idxErr == nil && len(idx.Rows) > 0 {
			rows := c.searchIndex(idx, term, true, limit)
			if len(rows) > 0 {
				return rows, nil
			}
		}
	}
	rows, err := c.searchLiveKeywordRows(ctx, version, term, true)
	if err != nil {
		return nil, err
	}
	return limitKeywordRows(rows, limit), nil
}

func (c *Client) SearchKeywordRowsByBroadText(ctx context.Context, version model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error) {
	if rel, err := c.releaseForVersion(version); err == nil && rel.GFFURL != "" {
		if idx, idxErr := c.cachedIndex(ctx, version); idxErr == nil && len(idx.Rows) > 0 {
			rows := c.searchIndex(idx, term, true, limit)
			if len(rows) > 0 {
				for i := range rows {
					rows[i].SearchType = tairkeyword.SearchTypeBroad
				}
				return rows, nil
			}
		}
	}
	rows, err := c.searchLiveKeywordRows(ctx, version, term, true)
	if err != nil {
		return nil, err
	}
	return limitKeywordRows(rows, limit), nil
}

func (c *Client) SearchKeywordRowsByFamily(ctx context.Context, version model.SpeciesCandidate, family string, limit int) ([]model.KeywordResultRow, error) {
	key := strings.TrimSpace(family)
	if key == "" {
		return nil, nil
	}
	if rel, err := c.releaseForVersion(version); err == nil && rel.GFFURL != "" {
		if idx, idxErr := c.cachedIndex(ctx, version); idxErr == nil && len(idx.Rows) > 0 {
			rows := c.searchKeywordRowsByFamilyIndex(idx, key, limit)
			if len(rows) > 0 {
				for i := range rows {
					rows[i].SearchTerm = key
					rows[i].SearchType = tairkeyword.SearchTypeFamily
				}
				return rows, nil
			}
		}
	}
	candidates, err := c.FetchFamilyCandidates(ctx, version)
	if err != nil {
		return nil, err
	}
	familyKey := key
	for _, candidate := range candidates {
		if strings.EqualFold(candidate.JBrowseName, key) || strings.EqualFold(candidate.GenomeLabel, key) || strings.EqualFold(candidate.GroupKey, key) {
			familyKey = firstNonEmpty(candidate.GroupKey, candidate.JBrowseName)
			break
		}
	}
	rows, err := c.fetchLiveFamilyRows(ctx, version, familyKey)
	if err != nil {
		return nil, err
	}
	return limitKeywordRows(rows, limit), nil
}

func (c *Client) FetchFamilyCandidates(ctx context.Context, version model.SpeciesCandidate) ([]model.SpeciesCandidate, error) {
	if rel, err := c.releaseForVersion(version); err == nil && rel.GFFURL != "" {
		if idx, idxErr := c.cachedIndex(ctx, version); idxErr == nil && len(idx.FamilyCandidates) > 0 {
			return familyCandidatesToSpecies(idx.FamilyCandidates), nil
		}
	}
	families, err := c.cachedLiveFamilyCandidates(ctx, version)
	if err != nil {
		return nil, err
	}
	return familyCandidatesToSpecies(families), nil
}

func (c *Client) cachedLiveFamilyCandidates(ctx context.Context, version model.SpeciesCandidate) ([]familyCandidate, error) {
	cacheKey := strings.ToLower(strings.TrimSpace(firstNonEmpty(version.JBrowseName, version.GenomeLabel, "tair")))
	c.mu.RLock()
	if cached := c.familyLists[cacheKey]; len(cached) > 0 {
		out := append([]familyCandidate(nil), cached...)
		c.mu.RUnlock()
		return out, nil
	}
	c.mu.RUnlock()
	if cached, ok := readCachedJSON[[]familyCandidate]("family-candidates", cacheKey); ok && len(cached) > 0 {
		c.mu.Lock()
		c.familyLists[cacheKey] = append([]familyCandidate(nil), cached...)
		c.mu.Unlock()
		return append([]familyCandidate(nil), cached...), nil
	}
	value, err, _ := c.sf.Do("tair-family-candidates:"+cacheKey, func() (any, error) {
		if cached, ok := readCachedJSON[[]familyCandidate]("family-candidates", cacheKey); ok && len(cached) > 0 {
			return cached, nil
		}
		families, err := c.fetchLiveFamilyCandidates(ctx)
		if err != nil {
			return nil, err
		}
		sortFamilyCandidatesAlpha(families)
		writeCachedJSON("family-candidates", cacheKey, families)
		c.mu.Lock()
		c.familyLists[cacheKey] = append([]familyCandidate(nil), families...)
		c.mu.Unlock()
		return families, nil
	})
	if err != nil {
		return nil, err
	}
	families, _ := value.([]familyCandidate)
	return append([]familyCandidate(nil), families...), nil
}

func familyCandidatesToSpecies(families []familyCandidate) []model.SpeciesCandidate {
	out := make([]model.SpeciesCandidate, 0, len(families))
	for i, fam := range families {
		label := firstNonEmpty(fam.ShortName, fam.Name)
		common := fmt.Sprintf("%d genes", fam.Count)
		if fam.HasChildren {
			common += "; has subfamilies"
		}
		searchAlias := strings.TrimSpace(strings.Join([]string{fam.Name, fam.Key, fam.ParentName, fam.ParentKey}, " "))
		out = append(out, model.SpeciesCandidate{
			ProteomeID:  990000 + i + 1,
			JBrowseName: fam.Name,
			GenomeLabel: label,
			CommonName:  common,
			SearchAlias: searchAlias,
			IsOfficial:  true,
			GroupKey:    fam.Key,
			ParentKey:   fam.ParentKey,
			HasChildren: fam.HasChildren,
		})
	}
	return out
}

func FilterFamilyCandidates(candidates []model.SpeciesCandidate, keyword string) []model.SpeciesCandidate {
	return FilterSpeciesCandidates(candidates, keyword)
}

func (c *Client) FilterFamilyCandidates(candidates []model.SpeciesCandidate, keyword string) []model.SpeciesCandidate {
	return filterFamilyCandidates(candidates, keyword)
}

func (c *Client) searchLiveIdentifierRows(ctx context.Context, version model.SpeciesCandidate, term string, kind string) ([]model.KeywordResultRow, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, nil
	}
	seen := make(map[string]struct{})
	rows := make([]model.KeywordResultRow, 0, 8)
	addRows := func(docs []tairSearchDoc) {
		for _, doc := range docs {
			for _, row := range keywordRowsFromSearchDoc(version, doc) {
				key := rowIdentityKey(row)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				rows = append(rows, row)
			}
		}
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "gene":
		docs, err := c.searchTAIRGeneDocs(ctx, stripTranscriptSuffix(term))
		if err != nil {
			return nil, err
		}
		addRows(filterDocsByIdentifier(docs, stripTranscriptSuffix(term), "gene"))
	case "model":
		docs, err := c.searchTAIRGeneDocs(ctx, stripTranscriptSuffix(term))
		if err != nil {
			return nil, err
		}
		addRows(filterDocsByIdentifier(docs, term, "model"))
	default:
		geneDocs, err := c.searchTAIRGeneDocs(ctx, stripTranscriptSuffix(term))
		if err == nil {
			addRows(filterDocsByIdentifier(geneDocs, term, "any"))
		}
		proteinDocs, err := c.searchTAIRProteinDocs(ctx, term)
		if err == nil {
			addRows(filterDocsByIdentifier(proteinDocs, term, "any"))
		}
		if len(rows) == 0 {
			docs, err := c.searchTAIRGeneDocs(ctx, term)
			if err != nil {
				return nil, err
			}
			addRows(filterDocsByIdentifier(docs, term, "any"))
		}
	}
	sortKeywordRows(rows)
	return rows, nil
}

func (c *Client) searchLiveLabelRows(ctx context.Context, version model.SpeciesCandidate, term string) ([]model.KeywordResultRow, error) {
	geneDocs, err := c.searchTAIRGeneDocs(ctx, term)
	if err != nil {
		return nil, err
	}
	rows := keywordRowsFromSearchDocs(version, filterDocsByLabel(geneDocs, term))
	sortKeywordRows(rows)
	return rows, nil
}

func (c *Client) searchLiveKeywordRows(ctx context.Context, version model.SpeciesCandidate, term string, wide bool) ([]model.KeywordResultRow, error) {
	seen := make(map[string]struct{})
	rows := make([]model.KeywordResultRow, 0, 32)
	addRows := func(next []model.KeywordResultRow) {
		for _, row := range next {
			key := rowIdentityKey(row)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			rows = append(rows, row)
		}
	}
	geneDocs, err := c.searchTAIRGeneDocs(ctx, term)
	if err != nil {
		return nil, err
	}
	addRows(keywordRowsFromSearchDocs(version, rankGeneDocsForKeyword(geneDocs, term)))
	if wide {
		proteinDocs, err := c.searchTAIRProteinDocs(ctx, term)
		if err == nil {
			addRows(keywordRowsFromSearchDocs(version, rankGeneDocsForKeyword(proteinDocs, term)))
		}
	}
	if len(rows) == 0 || wide {
		keywordDocs, err := c.searchTAIRKeywordDocs(ctx, term)
		if err == nil {
			addRows(keywordRowsFromKeywordDocs(version, keywordDocs))
		}
	}
	sortKeywordRows(rows)
	return rows, nil
}

func (c *Client) searchTAIRGeneDocs(ctx context.Context, term string) ([]tairSearchDoc, error) {
	var res tairSearchResponse
	if err := c.postTAIRJSON(ctx, "/api/search/gene", map[string]string{"key": strings.TrimSpace(term)}, &res); err != nil {
		return nil, err
	}
	return res.Docs, nil
}

func (c *Client) searchTAIRProteinDocs(ctx context.Context, term string) ([]tairSearchDoc, error) {
	var res tairSearchResponse
	if err := c.postTAIRJSON(ctx, "/api/search/protein", map[string]string{"key": strings.TrimSpace(term)}, &res); err != nil {
		return nil, err
	}
	return res.Docs, nil
}

func (c *Client) searchTAIRKeywordDocs(ctx context.Context, term string) ([]tairKeywordSearchDoc, error) {
	var res tairKeywordSearchResponse
	if err := c.postTAIRJSON(ctx, "/api/search/keyword", map[string]string{"key": strings.TrimSpace(term)}, &res); err != nil {
		return nil, err
	}
	return res.Docs, nil
}

func filterDocsByIdentifier(docs []tairSearchDoc, term string, kind string) []tairSearchDoc {
	termGene := strings.ToUpper(stripTranscriptSuffix(term))
	termModel := strings.ToUpper(strings.TrimSpace(term))
	out := make([]tairSearchDoc, 0, len(docs))
	for _, doc := range docs {
		match := false
		for _, gene := range doc.GeneName {
			if strings.EqualFold(strings.TrimSpace(gene), termGene) {
				match = true
				break
			}
		}
		if !match {
			for _, model := range doc.GeneModelIDs {
				model = strings.TrimSpace(model)
				switch kind {
				case "gene":
					match = strings.EqualFold(stripTranscriptSuffix(model), termGene)
				case "model":
					match = strings.EqualFold(model, termModel)
				default:
					match = strings.EqualFold(model, termModel) || strings.EqualFold(stripTranscriptSuffix(model), termGene)
				}
				if match {
					break
				}
			}
		}
		if match {
			out = append(out, doc)
		}
	}
	if len(out) == 0 {
		return docs
	}
	return out
}

func filterDocsByLabel(docs []tairSearchDoc, term string) []tairSearchDoc {
	target := normalizeAliasKey(term)
	out := make([]tairSearchDoc, 0, len(docs))
	for _, doc := range docs {
		for _, value := range append(append([]string{}, doc.OtherNames...), doc.GeneName...) {
			if normalizeAliasKey(value) == target {
				out = append(out, doc)
				break
			}
		}
	}
	if len(out) == 0 {
		return docs
	}
	return out
}

func rankGeneDocsForKeyword(docs []tairSearchDoc, term string) []tairSearchDoc {
	termLoose := normalizeSearchLoose(term)
	type scoredDoc struct {
		doc   tairSearchDoc
		score int
	}
	scored := make([]scoredDoc, 0, len(docs))
	for _, doc := range docs {
		score := 0
		for _, gene := range doc.GeneName {
			if strings.EqualFold(strings.TrimSpace(gene), strings.TrimSpace(term)) {
				score += 100
			}
		}
		for _, model := range doc.GeneModelIDs {
			if strings.EqualFold(strings.TrimSpace(model), strings.TrimSpace(term)) {
				score += 90
			}
		}
		for _, name := range doc.OtherNames {
			if normalizeAliasKey(name) == normalizeAliasKey(term) {
				score += 70
			}
		}
		text := normalizeSearchLoose(strings.Join([]string{
			strings.Join(doc.GeneName, " "),
			strings.Join(doc.GeneModelIDs, " "),
			strings.Join(doc.OtherNames, " "),
			strings.Join(doc.Description, " "),
			strings.Join(doc.Keywords, " "),
		}, " "))
		if strings.Contains(text, termLoose) {
			score += 20
		}
		if score > 0 || termLoose == "" {
			scored = append(scored, scoredDoc{doc: doc, score: score})
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return strings.ToUpper(firstNonEmpty(strings.Join(scored[i].doc.GeneName, " "), strings.Join(scored[i].doc.GeneModelIDs, " "))) <
			strings.ToUpper(firstNonEmpty(strings.Join(scored[j].doc.GeneName, " "), strings.Join(scored[j].doc.GeneModelIDs, " ")))
	})
	out := make([]tairSearchDoc, 0, len(scored))
	for _, item := range scored {
		out = append(out, item.doc)
	}
	if len(out) == 0 {
		return docs
	}
	return out
}

func keywordRowsFromSearchDocs(version model.SpeciesCandidate, docs []tairSearchDoc) []model.KeywordResultRow {
	rows := make([]model.KeywordResultRow, 0, len(docs)*2)
	seen := make(map[string]struct{})
	for _, doc := range docs {
		for _, row := range keywordRowsFromSearchDoc(version, doc) {
			key := rowIdentityKey(row)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			rows = append(rows, row)
		}
	}
	return rows
}

func keywordRowsFromSearchDoc(version model.SpeciesCandidate, doc tairSearchDoc) []model.KeywordResultRow {
	geneID := normalizeTAIRIdentifier(firstNonEmpty(doc.GeneName...))
	modelIDs := uniqueStrings(doc.GeneModelIDs)
	if len(modelIDs) == 0 && geneID != "" {
		modelIDs = []string{geneID + ".1"}
	}
	label := firstNonEmpty(doc.OtherNames...)
	if label == "" {
		label = firstNonEmpty(doc.GeneName...)
	}
	description := firstNonEmpty(doc.Description...)
	if description == "" {
		description = strings.Join(uniqueStrings(doc.Keywords), "; ")
	}
	aliases := uniqueStrings(append(append([]string{}, doc.OtherNames...), doc.GeneName...))
	rows := make([]model.KeywordResultRow, 0, len(modelIDs))
	for _, transcript := range modelIDs {
		transcript = strings.TrimSpace(transcript)
		if transcript == "" {
			continue
		}
		row := model.KeywordResultRow{
			SourceDatabase: "tair",
			LabelName:      label,
			ProteinID:      transcript,
			TranscriptID:   transcript,
			GeneIdentifier: firstNonEmpty(geneID, stripTranscriptSuffix(transcript)),
			Genome:         version.DisplayLabel(),
			Aliases:        strings.Join(aliases, "; "),
			Symbols:        label,
			Synonyms:       strings.Join(uniqueStrings(doc.OtherNames), "; "),
			UniProt:        strings.Join(uniqueStrings(doc.UniProtIDs), "; "),
			Description:    description,
			Comments:       strings.Join(uniqueStrings(doc.Phenotypes), "; "),
			AutoDefine:     firstNonEmpty(description, label),
			GeneReportURL:  baseURL + "/servlets/TairObject?type=locus&name=" + firstNonEmpty(geneID, stripTranscriptSuffix(transcript)),
			SequenceID:     transcript,
			ExtraColumns: map[string]string{
				"tair_object_id":        doc.ID,
				"tair_locus_object_id":  doc.LocusTAIRObjectID,
				"tair_gene_object_id":   doc.GeneTAIRObjectID,
				"tair_gene_model_type":  strings.Join(uniqueStrings(doc.GeneModelType), "; "),
				"tair_chromosome":       strings.TrimSpace(doc.Chromosome),
				"tair_map_type":         strings.TrimSpace(doc.MapType),
				"tair_keyword_types":    strings.Join(uniqueStrings(doc.KeywordTypes), "; "),
				"tair_keywords":         strings.Join(uniqueStrings(doc.Keywords), "; "),
				"tair_evidence_codes":   strings.Join(uniqueStrings(doc.EvidenceCodes), "; "),
				"tair_has_publications": strconv.FormatBool(doc.HasPublications),
				"tair_is_obselete":      strconv.FormatBool(doc.IsObselete),
				"tair_is_sequenced":     strconv.FormatBool(doc.IsSequenced),
			},
		}
		rows = append(rows, row)
	}
	return rows
}

func keywordRowsFromKeywordDocs(version model.SpeciesCandidate, docs []tairKeywordSearchDoc) []model.KeywordResultRow {
	rows := make([]model.KeywordResultRow, 0, len(docs))
	for _, doc := range docs {
		label := firstNonEmpty(doc.KwNameExact, firstNonEmpty(doc.KwName...))
		row := model.KeywordResultRow{
			SourceDatabase: "tair",
			LabelName:      label,
			GeneIdentifier: doc.KwID,
			Genome:         version.DisplayLabel(),
			Aliases:        strings.Join(uniqueStrings(doc.Synonyms), "; "),
			Symbols:        label,
			Description:    strings.Join(uniqueStrings(doc.KwCategory), "; "),
			Comments:       strings.Join(uniqueStrings(doc.KwChildNames), "; "),
			AutoDefine:     firstNonEmpty(label, strings.Join(uniqueStrings(doc.Synonyms), "; ")),
			GeneReportURL:  "",
			SequenceID:     doc.KwID,
			ExtraColumns: map[string]string{
				"tair_keyword_id":             doc.KwID,
				"tair_keyword_name_exact":     doc.KwNameExact,
				"tair_keyword_gopo_id":        strings.Join(uniqueStrings(doc.GOPOID), "; "),
				"tair_keyword_category":       strings.Join(uniqueStrings(doc.KwCategory), "; "),
				"tair_keyword_synonyms":       strings.Join(uniqueStrings(doc.Synonyms), "; "),
				"tair_keyword_child_names":    strings.Join(uniqueStrings(doc.KwChildNames), "; "),
				"tair_keyword_loci_count":     strconv.Itoa(doc.LociCount),
				"tair_keyword_loci_count_all": strconv.Itoa(doc.LociCount + doc.LociCountChild),
			},
		}
		rows = append(rows, row)
	}
	return rows
}

func rowIdentityKey(row model.KeywordResultRow) string {
	return strings.ToUpper(firstNonEmpty(row.TranscriptID, row.GeneIdentifier, row.ProteinID, row.SequenceID, row.LabelName))
}

func limitKeywordRows(rows []model.KeywordResultRow, limit int) []model.KeywordResultRow {
	if limit <= 0 || len(rows) <= limit {
		return rows
	}
	return append([]model.KeywordResultRow(nil), rows[:limit]...)
}

func filterFamilyCandidates(candidates []model.SpeciesCandidate, keyword string) []model.SpeciesCandidate {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		out := append([]model.SpeciesCandidate(nil), candidates...)
		sort.SliceStable(out, func(i, j int) bool {
			return strings.ToLower(firstNonEmpty(out[i].GenomeLabel, out[i].JBrowseName)) <
				strings.ToLower(firstNonEmpty(out[j].GenomeLabel, out[j].JBrowseName))
		})
		return out
	}
	type scoredCandidate struct {
		candidate model.SpeciesCandidate
		score     int
	}
	keywordLower := strings.ToLower(keyword)
	keywordLoose := normalizeSearchLoose(keyword)
	keywordTight := normalizeSearchTight(keyword)
	scored := make([]scoredCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		score := scoreFamilyCandidateMatch(candidate, keywordLower, keywordLoose, keywordTight)
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredCandidate{candidate: candidate, score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		left := strings.ToLower(firstNonEmpty(scored[i].candidate.GenomeLabel, scored[i].candidate.JBrowseName))
		right := strings.ToLower(firstNonEmpty(scored[j].candidate.GenomeLabel, scored[j].candidate.JBrowseName))
		if left != right {
			return left < right
		}
		return strings.ToLower(scored[i].candidate.GroupKey) < strings.ToLower(scored[j].candidate.GroupKey)
	})
	out := make([]model.SpeciesCandidate, 0, len(scored))
	for _, item := range scored {
		out = append(out, item.candidate)
	}
	return out
}

func scoreFamilyCandidateMatch(candidate model.SpeciesCandidate, keywordLower string, keywordLoose string, keywordTight string) int {
	parts := []string{
		candidate.GenomeLabel,
		candidate.JBrowseName,
		candidate.CommonName,
		candidate.SearchAlias,
		candidate.GroupKey,
		candidate.ParentKey,
		candidate.LabelName,
	}
	score := 0
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		text := strings.ToLower(part)
		loose := normalizeSearchLoose(part)
		tight := normalizeSearchTight(part)
		weight := len(parts) - index
		switch {
		case text == keywordLower:
			score += 200 * weight
		case loose != "" && loose == keywordLoose:
			score += 180 * weight
		case strings.HasPrefix(text, keywordLower):
			score += 140 * weight
		case loose != "" && keywordLoose != "" && strings.HasPrefix(loose, keywordLoose):
			score += 120 * weight
		case strings.Contains(text, keywordLower):
			score += 90 * weight
		case loose != "" && keywordLoose != "" && strings.Contains(loose, keywordLoose):
			score += 75 * weight
		case tight != "" && keywordTight != "" && strings.Contains(tight, keywordTight):
			score += 55 * weight
		default:
			score += familyTokenSubsequenceScore(tight, keywordTight) * weight
		}
	}
	return score
}

func familyTokenSubsequenceScore(text string, keyword string) int {
	text = strings.TrimSpace(text)
	keyword = strings.TrimSpace(keyword)
	if text == "" || keyword == "" {
		return 0
	}
	pos := 0
	matched := 0
	spanStart := -1
	spanEnd := -1
	for _, r := range keyword {
		found := false
		for pos < len(text) {
			if rune(text[pos]) == r {
				if spanStart < 0 {
					spanStart = pos
				}
				spanEnd = pos
				matched++
				pos++
				found = true
				break
			}
			pos++
		}
		if !found {
			break
		}
	}
	if matched == 0 {
		return 0
	}
	score := matched * 6
	if matched == len(keyword) {
		score += 20
	}
	if spanStart == 0 {
		score += 10
	}
	if spanStart >= 0 && spanEnd >= spanStart {
		span := spanEnd - spanStart + 1
		score += max(0, 8-(span-len(keyword)))
	}
	return score
}

func sortFamilyCandidatesAlpha(candidates []familyCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		leftParent := strings.ToLower(strings.TrimSpace(candidates[i].ParentKey))
		rightParent := strings.ToLower(strings.TrimSpace(candidates[j].ParentKey))
		if leftParent != rightParent {
			return leftParent < rightParent
		}
		left := strings.ToLower(firstNonEmpty(candidates[i].ShortName, candidates[i].Name, candidates[i].Key))
		right := strings.ToLower(firstNonEmpty(candidates[j].ShortName, candidates[j].Name, candidates[j].Key))
		if left != right {
			return left < right
		}
		return strings.ToLower(candidates[i].Key) < strings.ToLower(candidates[j].Key)
	})
}

func (c *Client) searchKeywordRowsByIdentifiers(index tairIndex, keys []string, limit int) []model.KeywordResultRow {
	rows := make([]model.KeywordResultRow, 0, 8)
	seen := make(map[int]struct{})
	for _, key := range keys {
		for _, idx := range index.ByIdentifier[key] {
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			rows = append(rows, index.Rows[idx])
			if limit > 0 && len(rows) >= limit {
				sortKeywordRows(rows)
				return rows
			}
		}
	}
	sortKeywordRows(rows)
	return rows
}

func (c *Client) searchKeywordRowsByAliases(index tairIndex, keyword string, limit int) []model.KeywordResultRow {
	keywords := []string{normalizeAliasKey(keyword)}
	terms := normalizeKeywordTerms(keyword)
	for _, term := range terms {
		keywords = append(keywords, normalizeAliasKey(term))
	}
	rows := make([]model.KeywordResultRow, 0, 16)
	seen := make(map[int]struct{})
	for _, key := range uniqueStrings(keywords) {
		for _, idx := range index.ByAlias[key] {
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			rows = append(rows, index.Rows[idx])
			if limit > 0 && len(rows) >= limit {
				sortKeywordRows(rows)
				return rows
			}
		}
	}
	sortKeywordRows(rows)
	return rows
}

func (c *Client) searchKeywordRowsByFamilyIndex(index tairIndex, family string, limit int) []model.KeywordResultRow {
	keys := []string{normalizeFamilyKey(family)}
	for _, candidate := range index.FamilyCandidates {
		if strings.EqualFold(candidate.Key, family) ||
			strings.EqualFold(candidate.Name, family) ||
			strings.EqualFold(candidate.ShortName, family) {
			keys = append(keys, normalizeFamilyKey(candidate.Key), normalizeFamilyKey(candidate.Name), normalizeFamilyKey(candidate.ShortName))
		}
	}
	rows := make([]model.KeywordResultRow, 0, 32)
	seen := make(map[int]struct{})
	for _, key := range uniqueStrings(keys) {
		for _, idx := range index.ByFamily[key] {
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			rows = append(rows, index.Rows[idx])
			if limit > 0 && len(rows) >= limit {
				sortKeywordRows(rows)
				return rows
			}
		}
	}
	sortKeywordRows(rows)
	return rows
}

func (c *Client) searchKeywordRowsByTerms(index tairIndex, keyword string, limit int) []model.KeywordResultRow {
	terms := normalizeKeywordTerms(keyword)
	if len(terms) == 0 {
		return nil
	}
	candidates := make(map[int]int)
	for _, term := range terms {
		for _, idx := range index.ByToken[term] {
			candidates[idx]++
		}
	}
	rows := make([]model.KeywordResultRow, 0, 16)
	for idx, count := range candidates {
		if count != len(terms) {
			continue
		}
		rows = append(rows, index.Rows[idx])
		if limit > 0 && len(rows) >= limit {
			break
		}
	}
	sortKeywordRows(rows)
	return rows
}

func (c *Client) searchIndex(index tairIndex, keyword string, wide bool, limit int) []model.KeywordResultRow {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil
	}
	seen := make(map[int]struct{})
	addFrom := func(bucket map[string][]int, keys []string, out *[]model.KeywordResultRow) {
		for _, key := range keys {
			for _, idx := range bucket[key] {
				if _, ok := seen[idx]; ok {
					continue
				}
				seen[idx] = struct{}{}
				*out = append(*out, index.Rows[idx])
				if limit > 0 && len(*out) >= limit {
					return
				}
			}
		}
	}
	rows := make([]model.KeywordResultRow, 0, 16)
	searchType := classifySearchType(keyword)
	if _, _, ok := parseTAIRReportKeyword(keyword); ok {
		_, id, _ := parseTAIRReportKeyword(keyword)
		addFrom(index.ByIdentifier, identifierKeys(id), &rows)
	}
	if len(rows) == 0 && (agiGenePattern.MatchString(keyword) || agiModelPattern.MatchString(keyword) || looksLikeSpecificIdentifier(keyword)) {
		addFrom(index.ByIdentifier, identifierKeys(keyword), &rows)
	}
	if len(rows) == 0 && !strings.ContainsAny(keyword, " \t") {
		addFrom(index.ByAlias, []string{normalizeAliasKey(keyword)}, &rows)
	}
	if len(rows) == 0 || wide {
		terms := normalizeKeywordTerms(keyword)
		candidates := make(map[int]int)
		for _, term := range terms {
			for _, idx := range index.ByToken[term] {
				candidates[idx]++
			}
		}
		for idx, count := range candidates {
			if count != len(terms) {
				continue
			}
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			rows = append(rows, index.Rows[idx])
			if limit > 0 && len(rows) >= limit {
				break
			}
		}
		if len(rows) > 0 && searchType != "wide search" && wide {
			searchType = "wide search"
		}
	}
	sortKeywordRows(rows)
	for i := range rows {
		rows[i].SearchTerm = keyword
		rows[i].SearchType = searchType
	}
	return rows
}

func (c *Client) cachedIndex(ctx context.Context, version model.SpeciesCandidate) (tairIndex, error) {
	rel, err := c.releaseForVersion(version)
	if err != nil {
		return tairIndex{}, err
	}
	cacheKey := rel.Name + "|" + rel.GFFURL + "|" + rel.ProteinURL
	c.mu.RLock()
	if idx, ok := c.rowIndex[cacheKey]; ok && len(idx.Rows) > 0 {
		c.mu.RUnlock()
		return idx, nil
	}
	c.mu.RUnlock()
	if idx, ok := readCachedJSON[tairIndex]("keyword-index", cacheKey); ok && len(idx.Rows) > 0 {
		c.mu.Lock()
		c.rowIndex[cacheKey] = idx
		c.mu.Unlock()
		return idx, nil
	}
	value, err, _ := c.sf.Do("tair-index:"+cacheKey, func() (any, error) {
		if idx, ok := readCachedJSON[tairIndex]("keyword-index", cacheKey); ok && len(idx.Rows) > 0 {
			return idx, nil
		}
		idx, err := c.buildIndex(ctx, rel, version)
		if err != nil {
			return tairIndex{}, err
		}
		writeCachedJSON("keyword-index", cacheKey, idx)
		c.mu.Lock()
		c.rowIndex[cacheKey] = idx
		c.mu.Unlock()
		return idx, nil
	})
	if err != nil {
		return tairIndex{}, err
	}
	return value.(tairIndex), nil
}

func (c *Client) buildIndex(ctx context.Context, rel releaseInfo, version model.SpeciesCandidate) (tairIndex, error) {
	proteins, _ := c.loadProteinSequences(ctx, rel)
	reader, closeFn, err := c.openMaybeGzip(ctx, rel.GFFURL)
	if err != nil {
		return tairIndex{}, err
	}
	defer closeFn()

	rows := make([]model.KeywordResultRow, 0, 36000)
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
		row := buildKeywordRowFromGFF(version, rel, gff)
		if protein, ok := lookupProteinEntry(proteins, row.SequenceID); ok {
			enrichRowWithProtein(&row, protein)
		} else if protein, ok := lookupProteinEntry(proteins, row.TranscriptID); ok {
			enrichRowWithProtein(&row, protein)
		} else if protein, ok := lookupProteinEntry(proteins, row.GeneIdentifier); ok {
			enrichRowWithProtein(&row, protein)
		}
		if row.GeneIdentifier == "" && row.TranscriptID == "" {
			continue
		}
		key := strings.ToUpper(firstNonEmpty(row.TranscriptID, row.GeneIdentifier))
		if idx, ok := rowByGene[key]; ok {
			if rows[idx].ProteinID == "" && row.ProteinID != "" {
				rows[idx].ProteinID = row.ProteinID
			}
			if rows[idx].Description == "" && row.Description != "" {
				rows[idx].Description = row.Description
			}
			continue
		}
		rowByGene[key] = len(rows)
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return tairIndex{}, fmt.Errorf("scan TAIR GFF3: %w", err)
	}

	idx := tairIndex{
		Release:      rel,
		Version:      version,
		Rows:         rows,
		ByIdentifier: make(map[string][]int),
		ByAlias:      make(map[string][]int),
		ByToken:      make(map[string][]int),
		ByFamily:     make(map[string][]int),
	}
	descriptions := c.loadDescriptionIndex(ctx, rel)
	representative := c.loadRepresentativeModels(ctx, rel)
	familyCounts := make(map[string]int)
	familyNames := make(map[string]string)
	parentCounts := make(map[string]int)
	parentNames := make(map[string]string)
	for i, row := range rows {
		mergeDescriptionIntoRow(&row, descriptions)
		applyRepresentativeModelHint(&row, representative)
		if fam := familyNameFromDescription(firstNonEmpty(row.Description, row.AutoDefine, row.Comments)); fam != "" {
			row.ExtraColumns = ensureExtraColumns(row.ExtraColumns)
			row.ExtraColumns["tair_family_name"] = fam
			row.ExtraColumns["tair_family_short_name"] = familyShortName(fam)
			if parentName, parentKey := familyParentName(fam); parentKey != "" {
				row.ExtraColumns["tair_family_parent_name"] = parentName
				row.ExtraColumns["tair_family_parent_key"] = parentKey
			}
		}
		rows[i] = row
		for _, id := range rowIdentifiers(row) {
			for _, key := range identifierKeys(id) {
				addIndexHit(idx.ByIdentifier, key, i)
			}
		}
		for _, alias := range rowAliases(row) {
			addIndexHit(idx.ByAlias, normalizeAliasKey(alias), i)
		}
		for _, token := range normalizeKeywordTerms(rowSearchText(row)) {
			addIndexHit(idx.ByToken, token, i)
		}
		if fam := firstNonEmpty(row.ExtraColumns["tair_family_name"], familyNameFromDescription(row.Description)); fam != "" {
			key := normalizeFamilyKey(fam)
			familyCounts[key]++
			if familyNames[key] == "" || len(fam) < len(familyNames[key]) {
				familyNames[key] = fam
			}
			addIndexHit(idx.ByFamily, key, i)
			addIndexHit(idx.ByFamily, normalizeFamilyKey(familyShortName(fam)), i)
			parentName, parentKey := familyParentName(fam)
			if parentKey != "" {
				parentCounts[parentKey]++
				if parentNames[parentKey] == "" || len(parentName) < len(parentNames[parentKey]) {
					parentNames[parentKey] = parentName
				}
				addIndexHit(idx.ByFamily, parentKey, i)
			}
		}
	}
	parentHasChildren := make(map[string]bool)
	for key, count := range familyCounts {
		if count < 2 {
			continue
		}
		parentName, parentKey := familyParentName(familyNames[key])
		if parentKey != "" && parentKey != key {
			parentHasChildren[parentKey] = true
		}
		idx.FamilyCandidates = append(idx.FamilyCandidates, familyCandidate{
			Name:       familyNames[key],
			ShortName:  familyShortName(familyNames[key]),
			Count:      count,
			Key:        key,
			ParentKey:  parentKey,
			ParentName: parentName,
		})
	}
	for key, count := range parentCounts {
		if count < 2 || familyCounts[key] > 0 {
			continue
		}
		idx.FamilyCandidates = append(idx.FamilyCandidates, familyCandidate{
			Name:        parentNames[key],
			ShortName:   familyShortName(parentNames[key]),
			Count:       count,
			Key:         key,
			HasChildren: true,
		})
		parentHasChildren[key] = true
	}
	for i := range idx.FamilyCandidates {
		if parentHasChildren[idx.FamilyCandidates[i].Key] {
			idx.FamilyCandidates[i].HasChildren = true
		}
	}
	sort.Slice(idx.FamilyCandidates, func(i, j int) bool {
		if idx.FamilyCandidates[i].ParentKey == "" && idx.FamilyCandidates[j].ParentKey != "" {
			return true
		}
		if idx.FamilyCandidates[i].ParentKey != "" && idx.FamilyCandidates[j].ParentKey == "" {
			return false
		}
		if idx.FamilyCandidates[i].Count != idx.FamilyCandidates[j].Count {
			return idx.FamilyCandidates[i].Count > idx.FamilyCandidates[j].Count
		}
		return strings.ToLower(firstNonEmpty(idx.FamilyCandidates[i].ShortName, idx.FamilyCandidates[i].Name)) <
			strings.ToLower(firstNonEmpty(idx.FamilyCandidates[j].ShortName, idx.FamilyCandidates[j].Name))
	})
	return idx, nil
}

func buildKeywordRowFromGFF(version model.SpeciesCandidate, rel releaseInfo, gff gffRow) model.KeywordResultRow {
	attrs := gff.AttrMap
	id := cleanIdentifier(firstNonEmpty(attrs["ID"], attrs["Name"], attrs["locus"], attrs["gene_id"], attrs["transcript_id"]))
	parent := cleanIdentifier(firstNonEmpty(attrs["Parent"], attrs["gene"], attrs["gene_id"]))
	transcript := cleanIdentifier(firstNonEmpty(attrs["transcript_id"], attrs["Name"], id))
	if strings.EqualFold(gff.Type, "gene") {
		parent = id
		transcript = ""
	}
	gene := cleanIdentifier(firstNonEmpty(parent, stripTranscriptSuffix(id), id))
	proteinID := cleanIdentifier(firstNonEmpty(attrs["protein_id"], attrs["protein"], attrs["Derives_from"], transcript))
	sequenceID := cleanIdentifier(firstNonEmpty(proteinID, transcript, id))
	description := cleanText(firstNonEmpty(attrs["description"], attrs["Note"], attrs["note"], attrs["product"]))
	if strings.EqualFold(description, "protein_coding_gene") || strings.EqualFold(description, "protein coding gene") {
		description = ""
	}
	extra := map[string]string{
		"tair_version":   rel.Name,
		"tair_gff_url":   rel.GFFURL,
		"gff_seqid":      gff.SeqID,
		"gff_source":     gff.Source,
		"gff_type":       gff.Type,
		"gff_start":      gff.Start,
		"gff_end":        gff.End,
		"gff_score":      gff.Score,
		"gff_strand":     gff.Strand,
		"gff_phase":      gff.Phase,
		"gff_attributes": gff.Attributes,
	}
	for key, value := range attrs {
		extra["attr_"+key] = value
	}
	return model.KeywordResultRow{
		SourceDatabase:      "tair",
		SearchType:          "TAIR keyword",
		LabelName:           firstSymbolFromText(firstNonEmpty(attrs["symbol"], attrs["Alias"], attrs["Name"])),
		ProteinID:           proteinID,
		TranscriptID:        transcript,
		GeneIdentifier:      gene,
		Genome:              version.DisplayLabel(),
		Location:            fmt.Sprintf("%s:%s..%s %s", gff.SeqID, gff.Start, gff.End, gff.Strand),
		Aliases:             firstNonEmpty(attrs["Alias"], attrs["Dbxref"]),
		Symbols:             firstNonEmpty(attrs["symbol"], attrs["gene_symbol"]),
		UniProt:             firstNonEmpty(strings.Join(extractUniProtAccessions(attrs["Dbxref"], attrs["dbxref"], gff.Attributes), "; "), attrs["UniProtKB"]),
		Description:         description,
		Comments:            firstNonEmpty(attrs["Note"], attrs["comment"]),
		AutoDefine:          firstNonEmpty(description, attrs["Name"]),
		GeneReportURL:       rel.ReportURLBase + url.QueryEscape(gene),
		SequenceHeaderLabel: version.DisplayLabel(),
		SequenceID:          sequenceID,
		ExtraColumns:        extra,
	}
}

func enrichRowWithProtein(row *model.KeywordResultRow, protein proteinEntry) {
	if row == nil {
		return
	}
	row.SequenceID = firstNonEmpty(row.SequenceID, protein.ID)
	row.ProteinID = firstNonEmpty(row.ProteinID, protein.ID)
	row.Description = firstNonEmpty(row.Description, protein.Description)
	row.AutoDefine = firstNonEmpty(row.AutoDefine, protein.Description)
	row.Symbols = firstNonEmpty(row.Symbols, protein.Symbols)
	row.LabelName = firstNonEmpty(row.LabelName, firstSymbolFromText(protein.Symbols))
	row.ExtraColumns = ensureExtraColumns(row.ExtraColumns)
	row.ExtraColumns["tair_fasta_header"] = protein.Header
	row.ExtraColumns["tair_fasta_description"] = protein.Description
	row.ExtraColumns["tair_fasta_symbols"] = protein.Symbols
}

func (c *Client) FetchProteinSequence(ctx context.Context, targetID int, sequenceID string) (model.ProteinSequenceData, error) {
	sequenceID = cleanIdentifier(sequenceID)
	if sequenceID == "" {
		return model.ProteinSequenceData{}, fmt.Errorf("empty TAIR protein sequence id")
	}
	rel, err := c.releaseForTargetID(targetID)
	if err != nil {
		return model.ProteinSequenceData{}, err
	}
	proteins, err := c.loadProteinSequences(ctx, rel)
	if err == nil {
		for _, candidate := range uniqueStrings([]string{sequenceID, stripTranscriptSuffix(sequenceID)}) {
			if entry, ok := lookupProteinEntry(proteins, candidate); ok {
				return model.ProteinSequenceData{Sequence: entry.Sequence, OriginalHeader: entry.Header}, nil
			}
		}
	}
	entry, externalErr := c.fetchExternalProteinSequence(ctx, rel, sequenceID)
	if externalErr == nil {
		return model.ProteinSequenceData{Sequence: entry.Sequence, OriginalHeader: entry.Header}, nil
	}
	if err != nil {
		return model.ProteinSequenceData{}, err
	}
	if externalErr != nil {
		return model.ProteinSequenceData{}, externalErr
	}
	return model.ProteinSequenceData{}, fmt.Errorf("no TAIR protein sequence matched %s in %s", sequenceID, rel.Name)
}

func (c *Client) FetchNucleotideSequence(ctx context.Context, targetID int, sequenceID string, program string) (model.ProteinSequenceData, error) {
	rel, err := c.releaseForTargetID(targetID)
	if err != nil {
		return model.ProteinSequenceData{}, err
	}
	_ = program
	seqs, err := c.loadNucleotideSequences(ctx, rel)
	if err != nil {
		return model.ProteinSequenceData{}, err
	}
	entry, ok := lookupProteinEntry(seqs, sequenceID)
	if !ok {
		return model.ProteinSequenceData{}, fmt.Errorf("no TAIR nucleotide sequence matched %s in %s", sequenceID, rel.Name)
	}
	return model.ProteinSequenceData{Sequence: entry.Sequence, OriginalHeader: entry.Header}, nil
}

func (c *Client) FetchUniProtAccessions(ctx context.Context, targetID int, proteinID string) ([]string, error) {
	rel, err := c.releaseForTargetID(targetID)
	if err != nil {
		return nil, err
	}
	version, err := c.releaseVersion(rel)
	if err != nil {
		return nil, err
	}
	row, err := c.findRow(ctx, version, proteinID)
	if err != nil {
		return nil, nil
	}
	return extractUniProtAccessions(
		row.UniProt,
		row.Aliases,
		row.Symbols,
		row.Description,
		row.Comments,
		row.AutoDefine,
		row.ExtraColumns["attr_Dbxref"],
		row.ExtraColumns["attr_dbxref"],
		row.ExtraColumns["gff_attributes"],
	), nil
}

func (c *Client) FetchGeneQuerySequence(ctx context.Context, version model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, error) {
	row, err := c.findRow(ctx, version, identifier)
	if err != nil {
		return nil, err
	}
	seq, err := c.FetchProteinSequence(ctx, version.ProteomeID, firstNonEmpty(row.SequenceID, row.ProteinID, row.TranscriptID))
	if err != nil {
		return nil, err
	}
	return querySourceFromRow(version, row, seq.Sequence), nil
}

func (c *Client) FetchProteinQuerySequence(ctx context.Context, version model.SpeciesCandidate, proteinID string) (*model.QuerySequenceSource, error) {
	return c.FetchGeneQuerySequence(ctx, version, "protein", proteinID)
}

func (c *Client) ResolveQuerySequence(ctx context.Context, version model.SpeciesCandidate, input string) (*model.QuerySequenceSource, bool, error) {
	reportType, identifier, ok := parseTAIRReportKeyword(input)
	if !ok && (agiGenePattern.MatchString(input) || agiModelPattern.MatchString(input)) {
		reportType, identifier, ok = "gene", strings.TrimSpace(input), true
	}
	if !ok {
		return nil, false, nil
	}
	source, err := c.FetchGeneQuerySequence(ctx, version, reportType, identifier)
	if err != nil {
		return nil, true, err
	}
	source.OriginalInputURL = strings.TrimSpace(input)
	if _, _, isURL := parseTAIRReportKeyword(input); isURL {
		source.NormalizedURL = normalizeTAIRReportURL(input)
	}
	return source, true, nil
}

func querySourceFromRow(version model.SpeciesCandidate, row model.KeywordResultRow, proteinSeq string) *model.QuerySequenceSource {
	return &model.QuerySequenceSource{
		Sequence:            proteinSeq,
		ProteinSequence:     proteinSeq,
		SequenceKind:        model.SequenceProtein,
		PreferredSequenceID: firstNonEmpty(row.SequenceID, row.ProteinID, row.TranscriptID, row.GeneIdentifier),
		SourceDatabase:      "tair",
		SourceProteomeID:    version.ProteomeID,
		SourceJBrowseName:   version.JBrowseName,
		SourceGenomeLabel:   version.GenomeLabel,
		LabelName:           row.LabelName,
		PhgoAliases:         row.PhgoAliases,
		Aliases:             row.Aliases,
		Symbols:             row.Symbols,
		Synonyms:            row.Synonyms,
		AutoDefine:          row.AutoDefine,
		GeneID:              row.GeneIdentifier,
		TranscriptID:        row.TranscriptID,
		ProteinID:           row.ProteinID,
		OrganismShort:       "A. thaliana",
		Annotation:          row.Description,
	}
}

func (c *Client) findRow(ctx context.Context, version model.SpeciesCandidate, identifier string) (model.KeywordResultRow, error) {
	rows, err := c.searchLiveIdentifierRows(ctx, version, identifier, "any")
	if err != nil {
		return model.KeywordResultRow{}, err
	}
	normalizedGene := strings.ToUpper(stripTranscriptSuffix(identifier))
	normalizedModel := strings.ToUpper(strings.TrimSpace(identifier))
	for _, row := range rows {
		if strings.EqualFold(strings.TrimSpace(row.TranscriptID), normalizedModel) || strings.EqualFold(strings.TrimSpace(row.SequenceID), normalizedModel) {
			return row, nil
		}
		if strings.EqualFold(strings.TrimSpace(row.GeneIdentifier), normalizedGene) {
			return row, nil
		}
	}
	if len(rows) > 0 {
		return rows[0], nil
	}
	return model.KeywordResultRow{}, fmt.Errorf("TAIR identifier %s was not found in %s", identifier, version.DisplayLabel())
}

func (c *Client) releaseForTargetID(targetID int) (releaseInfo, error) {
	candidates, _ := c.FetchSpeciesCandidates(context.Background())
	for _, candidate := range candidates {
		if candidate.ProteomeID == targetID {
			return c.releaseForVersion(candidate)
		}
	}
	if targetID == 0 {
		return releaseInfo{}, fmt.Errorf("missing TAIR target id")
	}
	for _, rel := range defaultReleases() {
		if strings.EqualFold(rel.Name, "Araport11") {
			return rel, nil
		}
	}
	return releaseInfo{}, fmt.Errorf("unknown TAIR target id %d", targetID)
}

func (c *Client) releaseVersion(rel releaseInfo) (model.SpeciesCandidate, error) {
	for _, candidate := range defaultVersionCandidates() {
		if strings.EqualFold(candidate.JBrowseName, rel.Name) || strings.EqualFold(candidate.GenomeLabel, rel.Label) {
			return candidate, nil
		}
	}
	return model.SpeciesCandidate{}, fmt.Errorf("no TAIR version candidate matched release %s", rel.Name)
}

func (c *Client) loadProteinSequences(ctx context.Context, rel releaseInfo) (map[string]proteinEntry, error) {
	return c.loadFASTASequences(ctx, rel.ProteinURL, c.proteinSeqs)
}

func (c *Client) loadNucleotideSequences(ctx context.Context, rel releaseInfo) (map[string]proteinEntry, error) {
	return c.loadFASTASequences(ctx, rel.NucleotideURL, c.nucleotideSeqs)
}

func (c *Client) loadFASTASequences(ctx context.Context, requestURL string, memory map[string]map[string]proteinEntry) (map[string]proteinEntry, error) {
	requestURL = strings.TrimSpace(requestURL)
	if requestURL == "" {
		return nil, fmt.Errorf("empty TAIR FASTA URL")
	}
	c.mu.RLock()
	if cached, ok := memory[requestURL]; ok && len(cached) > 0 {
		out := cloneProteinMap(cached)
		c.mu.RUnlock()
		return out, nil
	}
	c.mu.RUnlock()
	if cached, ok := readCachedJSON[map[string]proteinEntry]("fasta-index", requestURL); ok && len(cached) > 0 {
		c.mu.Lock()
		memory[requestURL] = cached
		c.mu.Unlock()
		return cloneProteinMap(cached), nil
	}
	value, err, _ := c.sf.Do("tair-fasta:"+requestURL, func() (any, error) {
		reader, closeFn, err := c.openMaybeCompressed(ctx, requestURL)
		if err != nil {
			return nil, err
		}
		defer closeFn()
		seqs, err := parseFASTA(reader)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		memory[requestURL] = seqs
		c.mu.Unlock()
		writeCachedJSON("fasta-index", requestURL, seqs)
		return seqs, nil
	})
	if err != nil {
		return nil, err
	}
	return cloneProteinMap(value.(map[string]proteinEntry)), nil
}

type descriptionEntry struct {
	Identifier               string
	GeneModelType            string
	ShortDescription         string
	CuratorSummary           string
	ComputationalDescription string
}

func (c *Client) loadDescriptionIndex(ctx context.Context, rel releaseInfo) map[string]descriptionEntry {
	requestURL := strings.TrimSpace(rel.DescriptionURL)
	if requestURL == "" {
		return nil
	}
	if cached, ok := readCachedJSON[map[string]descriptionEntry]("description-index", requestURL); ok && len(cached) > 0 {
		return cached
	}
	value, err, _ := c.sf.Do("tair-description:"+requestURL, func() (any, error) {
		reader, closeFn, err := c.openMaybeCompressed(ctx, requestURL)
		if err != nil {
			return map[string]descriptionEntry{}, err
		}
		defer closeFn()
		entries, err := parseDescriptionTable(reader)
		if err != nil {
			return map[string]descriptionEntry{}, err
		}
		writeCachedJSON("description-index", requestURL, entries)
		return entries, nil
	})
	if err != nil {
		return nil
	}
	return value.(map[string]descriptionEntry)
}

func (c *Client) loadRepresentativeModels(ctx context.Context, rel releaseInfo) map[string]string {
	requestURL := strings.TrimSpace(rel.RepresentativeModelURL)
	if requestURL == "" {
		return nil
	}
	if cached, ok := readCachedJSON[map[string]string]("representative-models", requestURL); ok && len(cached) > 0 {
		return cached
	}
	value, err, _ := c.sf.Do("tair-representative:"+requestURL, func() (any, error) {
		reader, closeFn, err := c.openMaybeCompressed(ctx, requestURL)
		if err != nil {
			return map[string]string{}, err
		}
		defer closeFn()
		entries, err := parseRepresentativeModels(reader)
		if err != nil {
			return map[string]string{}, err
		}
		writeCachedJSON("representative-models", requestURL, entries)
		return entries, nil
	})
	if err != nil {
		return nil
	}
	return value.(map[string]string)
}

func parseFASTA(reader io.Reader) (map[string]proteinEntry, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 32*1024*1024)
	seqs := make(map[string]proteinEntry)
	var header string
	var seq strings.Builder
	flush := func() {
		header = strings.TrimSpace(header)
		sequence := strings.TrimSpace(seq.String())
		if header == "" || sequence == "" {
			return
		}
		id := fastaHeaderID(header)
		entry := proteinEntry{ID: id, Header: header, Sequence: sequence, Description: fastaDescription(header), Symbols: fastaSymbols(header)}
		for _, alias := range identifierVariants(id) {
			seqs[normalizeIdentifierKey(alias)] = entry
		}
		for _, token := range strings.FieldsFunc(header, func(r rune) bool {
			return r == '|' || r == ';' || r == ',' || r == ' ' || r == '\t'
		}) {
			for _, alias := range identifierVariants(token) {
				seqs[normalizeIdentifierKey(alias)] = entry
			}
		}
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, ">") {
			flush()
			header = strings.TrimPrefix(line, ">")
			seq.Reset()
			continue
		}
		seq.WriteString(line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	flush()
	return seqs, nil
}

type ensemblProteinSequenceResponse struct {
	ID       string `json:"id"`
	Query    string `json:"query"`
	Desc     string `json:"desc"`
	Sequence string `json:"seq"`
	Molecule string `json:"molecule"`
}

type uniProtSequenceSearchResponse struct {
	Results []struct {
		PrimaryAccession string `json:"primaryAccession"`
		UniProtkbID      string `json:"uniProtkbId"`
		Sequence         struct {
			Value string `json:"value"`
		} `json:"sequence"`
		Genes []struct {
			GeneName struct {
				Value string `json:"value"`
			} `json:"geneName"`
			OrderedLocusNames []struct {
				Value string `json:"value"`
			} `json:"orderedLocusNames"`
		} `json:"genes"`
		ProteinDescription struct {
			RecommendedName struct {
				FullName struct {
					Value string `json:"value"`
				} `json:"fullName"`
			} `json:"recommendedName"`
		} `json:"proteinDescription"`
	} `json:"results"`
}

func (c *Client) fetchExternalProteinSequence(ctx context.Context, rel releaseInfo, sequenceID string) (proteinEntry, error) {
	sequenceID = cleanIdentifier(sequenceID)
	if sequenceID == "" {
		return proteinEntry{}, fmt.Errorf("empty TAIR protein sequence id")
	}
	cacheKey := strings.ToLower(rel.Name) + "|" + strings.ToUpper(sequenceID)
	c.mu.RLock()
	if cached, ok := c.externalSeqs[cacheKey]; ok && strings.TrimSpace(cached.Sequence) != "" {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()
	if cached, ok := readCachedJSON[proteinEntry]("external-protein-sequence", cacheKey); ok && strings.TrimSpace(cached.Sequence) != "" {
		c.mu.Lock()
		c.externalSeqs[cacheKey] = cached
		c.mu.Unlock()
		return cached, nil
	}
	value, err, _ := c.sf.Do("tair-external-protein:"+cacheKey, func() (any, error) {
		if cached, ok := readCachedJSON[proteinEntry]("external-protein-sequence", cacheKey); ok && strings.TrimSpace(cached.Sequence) != "" {
			return cached, nil
		}
		entry, err := c.fetchExternalProteinSequenceUncached(ctx, sequenceID)
		if err != nil {
			return proteinEntry{}, err
		}
		c.mu.Lock()
		c.externalSeqs[cacheKey] = entry
		c.mu.Unlock()
		writeCachedJSON("external-protein-sequence", cacheKey, entry)
		return entry, nil
	})
	if err != nil {
		return proteinEntry{}, err
	}
	return value.(proteinEntry), nil
}

func (c *Client) fetchExternalProteinSequenceUncached(ctx context.Context, sequenceID string) (proteinEntry, error) {
	for _, candidate := range uniqueStrings([]string{sequenceID, stripTranscriptSuffix(sequenceID)}) {
		if entry, err := c.fetchProteinSequenceFromEnsembl(ctx, candidate); err == nil {
			return entry, nil
		}
	}
	for _, candidate := range uniqueStrings([]string{sequenceID, stripTranscriptSuffix(sequenceID)}) {
		if entry, err := c.fetchProteinSequenceFromUniProt(ctx, candidate); err == nil {
			return entry, nil
		}
	}
	return proteinEntry{}, fmt.Errorf("no external protein sequence source matched %s", sequenceID)
}

func (c *Client) fetchProteinSequenceFromEnsembl(ctx context.Context, sequenceID string) (proteinEntry, error) {
	sequenceID = cleanIdentifier(sequenceID)
	if sequenceID == "" {
		return proteinEntry{}, fmt.Errorf("empty Ensembl sequence id")
	}
	rawURL := "https://rest.ensembl.org/sequence/id/" + url.PathEscape(sequenceID) + "?type=protein"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return proteinEntry{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return proteinEntry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return proteinEntry{}, fmt.Errorf("Ensembl protein lookup returned %d for %s", resp.StatusCode, sequenceID)
	}
	var payload ensemblProteinSequenceResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return proteinEntry{}, err
	}
	seq := strings.TrimSpace(payload.Sequence)
	if seq == "" {
		return proteinEntry{}, fmt.Errorf("Ensembl returned empty protein sequence for %s", sequenceID)
	}
	id := cleanIdentifier(firstNonEmpty(payload.ID, payload.Query, sequenceID))
	desc := cleanText(payload.Desc)
	header := ">" + id
	if desc != "" {
		header += " " + desc
	}
	return proteinEntry{
		ID:          id,
		Header:      header,
		Sequence:    seq,
		Description: desc,
		Symbols:     firstSymbolFromText(desc),
	}, nil
}

func (c *Client) fetchProteinSequenceFromUniProt(ctx context.Context, sequenceID string) (proteinEntry, error) {
	sequenceID = cleanIdentifier(sequenceID)
	if sequenceID == "" {
		return proteinEntry{}, fmt.Errorf("empty UniProt sequence id")
	}
	query := fmt.Sprintf("(gene:%s OR gene:%s) AND organism_id:3702", sequenceID, strings.ToUpper(sequenceID))
	rawURL := "https://rest.uniprot.org/uniprotkb/search?format=json&size=3&query=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return proteinEntry{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return proteinEntry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return proteinEntry{}, fmt.Errorf("UniProt protein lookup returned %d for %s", resp.StatusCode, sequenceID)
	}
	var payload uniProtSequenceSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return proteinEntry{}, err
	}
	for _, result := range payload.Results {
		seq := strings.TrimSpace(result.Sequence.Value)
		if seq == "" {
			continue
		}
		desc := cleanText(result.ProteinDescription.RecommendedName.FullName.Value)
		symbols := ""
		for _, gene := range result.Genes {
			if symbols == "" {
				symbols = cleanText(gene.GeneName.Value)
			}
			for _, locus := range gene.OrderedLocusNames {
				if strings.EqualFold(cleanIdentifier(locus.Value), stripTranscriptSuffix(sequenceID)) && symbols == "" {
					symbols = cleanText(gene.GeneName.Value)
				}
			}
		}
		id := cleanIdentifier(sequenceID)
		header := ">" + id
		if accession := strings.TrimSpace(result.PrimaryAccession); accession != "" {
			header += " | UniProt: " + accession
		}
		if desc != "" {
			header += " | " + desc
		}
		return proteinEntry{
			ID:          id,
			Header:      header,
			Sequence:    seq,
			Description: desc,
			Symbols:     symbols,
		}, nil
	}
	return proteinEntry{}, fmt.Errorf("UniProt returned no protein sequence for %s", sequenceID)
}

func (c *Client) openMaybeCompressed(ctx context.Context, requestURL string) (io.Reader, func(), error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, nil, fmt.Errorf("fetch %s: unexpected status %s", requestURL, resp.Status)
	}
	if strings.HasSuffix(strings.ToLower(requestURL), ".gz") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			_ = resp.Body.Close()
			return nil, nil, err
		}
		return gz, func() { _ = gz.Close(); _ = resp.Body.Close() }, nil
	}
	if strings.HasSuffix(strings.ToLower(requestURL), ".zip") {
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, nil, err
		}
		zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			return nil, nil, err
		}
		for _, file := range zr.File {
			name := strings.ToLower(file.Name)
			if strings.HasSuffix(name, ".gff") || strings.HasSuffix(name, ".gff3") || strings.HasSuffix(name, ".txt") || strings.HasSuffix(name, ".tsv") || strings.HasSuffix(name, ".fa") || strings.HasSuffix(name, ".fasta") {
				rc, openErr := file.Open()
				if openErr != nil {
					continue
				}
				return rc, func() { _ = rc.Close() }, nil
			}
		}
		return nil, nil, fmt.Errorf("zip %s did not contain a supported text payload", requestURL)
	}
	return resp.Body, func() { _ = resp.Body.Close() }, nil
}

func (c *Client) openMaybeGzip(ctx context.Context, requestURL string) (io.Reader, func(), error) {
	return c.openMaybeCompressed(ctx, requestURL)
}

func parseGFF3Line(line string) (gffRow, bool) {
	cols := strings.Split(line, "\t")
	if len(cols) != 9 {
		return gffRow{}, false
	}
	return gffRow{SeqID: cols[0], Source: cols[1], Type: cols[2], Start: cols[3], End: cols[4], Score: cols[5], Strand: cols[6], Phase: cols[7], Attributes: cols[8], AttrMap: parseGFFAttributes(cols[8])}, true
}

func parseGFFAttributes(value string) map[string]string {
	attrs := make(map[string]string)
	for _, part := range strings.Split(value, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			key, val, ok = strings.Cut(part, " ")
		}
		if !ok {
			attrs[part] = ""
			continue
		}
		if decoded, err := url.QueryUnescape(strings.TrimSpace(val)); err == nil {
			val = decoded
		}
		attrs[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return attrs
}

func parseDescriptionTable(reader io.Reader) (map[string]descriptionEntry, error) {
	csvReader := csv.NewReader(reader)
	csvReader.Comma = '\t'
	csvReader.FieldsPerRecord = -1
	csvReader.LazyQuotes = true
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}
	out := make(map[string]descriptionEntry)
	if len(records) == 0 {
		return out, nil
	}
	start := 0
	header := normalizeTSVHeader(records[0])
	if strings.Contains(strings.Join(header, " "), "short_description") || strings.Contains(strings.Join(header, " "), "computational_description") {
		start = 1
	}
	for _, record := range records[start:] {
		if len(record) == 0 {
			continue
		}
		id := cleanIdentifier(recordAt(record, 0))
		if id == "" {
			continue
		}
		entry := descriptionEntry{
			Identifier:               id,
			GeneModelType:            recordAt(record, 1),
			ShortDescription:         recordAt(record, 2),
			CuratorSummary:           recordAt(record, 3),
			ComputationalDescription: recordAt(record, 4),
		}
		for _, key := range identifierKeys(id) {
			out[key] = entry
		}
	}
	return out, nil
}

func parseRepresentativeModels(reader io.Reader) (map[string]string, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 8*1024*1024)
	out := make(map[string]string)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "AT") {
			continue
		}
		fields := strings.FieldsFunc(line, func(r rune) bool {
			return r == '\t' || r == ' ' || r == ',' || r == ';'
		})
		for _, field := range fields {
			id := cleanIdentifier(field)
			if !agiGenePattern.MatchString(stripTranscriptSuffix(id)) && !agiModelPattern.MatchString(id) {
				continue
			}
			gene := stripTranscriptSuffix(id)
			if gene != "" {
				out[normalizeIdentifierKey(gene)] = id
			}
			if id != "" {
				out[normalizeIdentifierKey(id)] = id
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func normalizeTSVHeader(row []string) []string {
	out := make([]string, 0, len(row))
	for _, value := range row {
		value = strings.ToLower(strings.TrimSpace(value))
		value = strings.ReplaceAll(value, " ", "_")
		out = append(out, value)
	}
	return out
}

func recordAt(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func mergeDescriptionIntoRow(row *model.KeywordResultRow, descriptions map[string]descriptionEntry) {
	if row == nil || len(descriptions) == 0 {
		return
	}
	for _, key := range rowIdentifiers(*row) {
		if entry, ok := descriptions[normalizeIdentifierKey(key)]; ok {
			row.Description = firstNonEmpty(row.Description, entry.ShortDescription, entry.ComputationalDescription)
			row.Comments = firstNonEmpty(row.Comments, entry.CuratorSummary)
			row.AutoDefine = firstNonEmpty(row.AutoDefine, entry.ComputationalDescription, entry.ShortDescription, entry.CuratorSummary)
			row.ExtraColumns = ensureExtraColumns(row.ExtraColumns)
			row.ExtraColumns["tair_gene_model_type"] = firstNonEmpty(row.ExtraColumns["tair_gene_model_type"], entry.GeneModelType)
			row.ExtraColumns["tair_short_description"] = entry.ShortDescription
			row.ExtraColumns["tair_curator_summary"] = entry.CuratorSummary
			row.ExtraColumns["tair_computational_description"] = entry.ComputationalDescription
			if row.LabelName == "" {
				row.LabelName = firstSymbolFromText(entry.ShortDescription)
			}
			return
		}
	}
}

func applyRepresentativeModelHint(row *model.KeywordResultRow, representative map[string]string) {
	if row == nil || len(representative) == 0 {
		return
	}
	gene := normalizeIdentifierKey(row.GeneIdentifier)
	model := normalizeIdentifierKey(firstNonEmpty(row.TranscriptID, row.ProteinID, row.SequenceID))
	rep := representative[gene]
	row.ExtraColumns = ensureExtraColumns(row.ExtraColumns)
	row.ExtraColumns["tair_representative_model"] = rep
	if rep != "" && model != "" {
		row.ExtraColumns["tair_is_representative_model"] = strconv.FormatBool(strings.EqualFold(rep, firstNonEmpty(row.TranscriptID, row.ProteinID, row.SequenceID)))
	}
}

func isSearchableFeatureType(featureType string) bool {
	switch strings.ToLower(strings.TrimSpace(featureType)) {
	case "gene", "mrna", "transcript":
		return true
	default:
		return false
	}
}

func parseTAIRReportKeyword(value string) (reportType string, identifier string, ok bool) {
	return tairkeyword.TAIRReportKeyword(value)
}

func normalizeTAIRReportURL(value string) string {
	_, identifier, ok := parseTAIRReportKeyword(value)
	if !ok {
		return strings.TrimSpace(value)
	}
	return baseURL + "/servlets/TairObject?type=locus&name=" + url.QueryEscape(identifier)
}

func classifySearchType(term string) string {
	switch {
	case func() bool { _, _, ok := parseTAIRReportKeyword(term); return ok }():
		return "TAIR report URL"
	case agiGenePattern.MatchString(term):
		return "TAIR locus"
	case agiModelPattern.MatchString(term):
		return "TAIR gene model"
	case !strings.ContainsAny(term, " \t") && symbolPattern.MatchString(term):
		return "TAIR symbol / alias"
	default:
		return "keyword"
	}
}

func rowIdentifiers(row model.KeywordResultRow) []string {
	return uniqueStrings([]string{row.GeneIdentifier, row.TranscriptID, row.ProteinID, row.SequenceID})
}

func rowAliases(row model.KeywordResultRow) []string {
	values := []string{row.LabelName, row.Aliases, row.Symbols, row.Synonyms}
	for _, value := range values {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == ';' || r == ',' || r == '|' || r == '\t' }) {
			values = append(values, strings.TrimSpace(part))
		}
	}
	return uniqueStrings(values)
}

func rowSearchText(row model.KeywordResultRow) string {
	values := []string{row.LabelName, row.ProteinID, row.TranscriptID, row.GeneIdentifier, row.Aliases, row.Symbols, row.Synonyms, row.Description, row.Comments, row.AutoDefine}
	for _, v := range row.ExtraColumns {
		values = append(values, v)
	}
	return strings.Join(values, " ")
}

func addIndexHit(bucket map[string][]int, key string, idx int) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	for _, existing := range bucket[key] {
		if existing == idx {
			return
		}
	}
	bucket[key] = append(bucket[key], idx)
}

func identifierKeys(value string) []string {
	out := make([]string, 0, 8)
	for _, candidate := range identifierVariants(value) {
		out = append(out, normalizeIdentifierKey(candidate))
	}
	return uniqueStrings(out)
}

func identifierVariants(value string) []string {
	value = cleanIdentifier(value)
	if value == "" {
		return nil
	}
	out := []string{value, strings.ToUpper(value), strings.ToLower(value)}
	if strings.Contains(value, ".") {
		out = append(out, strings.Split(value, ".")[0])
	}
	if strings.HasSuffix(strings.ToLower(value), "-protein") {
		out = append(out, value[:len(value)-len("-Protein")])
	}
	if strings.Contains(value, "|") {
		parts := strings.Split(value, "|")
		out = append(out, parts[0], parts[len(parts)-1])
	}
	return uniqueStrings(out)
}

func normalizeIdentifierKey(value string) string {
	return strings.ToUpper(cleanIdentifier(value))
}

func normalizeAliasKey(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	replacer := strings.NewReplacer("_", "", "-", "", " ", "", ".", "")
	return replacer.Replace(value)
}

func normalizeKeywordTerms(value string) []string {
	value = normalizeSearchLoose(value)
	if value == "" {
		return nil
	}
	return uniqueStrings(strings.Fields(value))
}

func normalizeSearchLoose(value string) string {
	value = strings.ToLower(html.UnescapeString(value))
	value = searchNoisePattern.ReplaceAllString(value, " ")
	return strings.TrimSpace(spacePattern.ReplaceAllString(value, " "))
}

func normalizeSearchTight(value string) string {
	return strings.ReplaceAll(normalizeSearchLoose(value), " ", "")
}

func cleanIdentifier(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "gene:")
	value = strings.TrimPrefix(value, "transcript:")
	value = strings.TrimPrefix(value, "protein:")
	value = strings.TrimSuffix(value, "-Protein")
	return strings.Trim(value, `"' `)
}

func stripTranscriptSuffix(value string) string {
	value = cleanIdentifier(value)
	if idx := strings.Index(value, "."); idx > 0 {
		return value[:idx]
	}
	return value
}

func fastaHeaderID(header string) string {
	if fields := strings.Fields(header); len(fields) > 0 {
		return cleanIdentifier(fields[0])
	}
	return cleanIdentifier(header)
}

func fastaDescription(header string) string {
	parts := strings.Split(header, "|")
	if len(parts) >= 3 {
		return cleanText(parts[2])
	}
	if fields := strings.Fields(header); len(fields) > 1 {
		return cleanText(strings.Join(fields[1:], " "))
	}
	return ""
}

func fastaSymbols(header string) string {
	lower := strings.ToLower(header)
	idx := strings.Index(lower, "symbols:")
	if idx < 0 {
		return ""
	}
	value := header[idx+len("symbols:"):]
	if cut := strings.Index(value, "|"); cut >= 0 {
		value = value[:cut]
	}
	return cleanText(value)
}

func lookupProteinEntry(values map[string]proteinEntry, key string) (proteinEntry, bool) {
	for _, candidate := range identifierKeys(key) {
		if entry, ok := values[candidate]; ok {
			return entry, true
		}
	}
	return proteinEntry{}, false
}

func familyNameFromDescription(description string) string {
	description = cleanText(description)
	if description == "" || !strings.Contains(strings.ToLower(description), "family") {
		return ""
	}
	value := familyNoisePattern.ReplaceAllString(description, "")
	value = strings.Trim(value, " ;,.")
	if len(value) < 4 {
		return ""
	}
	return value
}

func normalizeFamilyKey(value string) string {
	value = familyNameFromDescription(value)
	if value == "" {
		value = strings.TrimSpace(value)
	}
	return normalizeSearchTight(value)
}

func familyShortName(value string) string {
	value = cleanText(value)
	value = strings.TrimSpace(familyNoisePattern.ReplaceAllString(value, ""))
	value = strings.Trim(value, " ;,.")
	if value == "" {
		return ""
	}
	fields := strings.Fields(value)
	switch len(fields) {
	case 0:
		return ""
	case 1:
		return fields[0]
	case 2:
		return strings.Join(fields, " ")
	default:
		return fields[0] + " " + fields[1]
	}
}

func familyParentName(value string) (string, string) {
	value = familyNameFromDescription(value)
	if value == "" {
		return "", ""
	}
	lower := strings.ToLower(value)
	separators := []string{
		" subfamily ",
		" subgroup ",
		" class ",
		" clade ",
		" type ",
	}
	for _, separator := range separators {
		if idx := strings.Index(lower, separator); idx > 0 {
			parent := strings.TrimSpace(value[:idx])
			return parent, normalizeFamilyKey(parent)
		}
	}
	fields := strings.Fields(value)
	if len(fields) >= 3 {
		parent := strings.Join(fields[:2], " ")
		return parent, normalizeFamilyKey(parent + " family")
	}
	if len(fields) == 2 {
		parent := fields[0]
		return parent, normalizeFamilyKey(parent + " family")
	}
	return "", ""
}

func extractUniProtAccessions(values ...string) []string {
	out := make([]string, 0, 2)
	seen := make(map[string]struct{}, 4)
	for _, value := range values {
		for _, match := range uniprotPattern.FindAllStringSubmatch(value, -1) {
			if len(match) < 2 {
				continue
			}
			accession := strings.ToUpper(strings.TrimSpace(match[1]))
			if accession == "" {
				continue
			}
			if _, ok := seen[accession]; ok {
				continue
			}
			seen[accession] = struct{}{}
			out = append(out, accession)
		}
	}
	return out
}

func firstSymbolFromText(value string) string {
	for _, token := range symbolPattern.FindAllString(value, -1) {
		lower := strings.ToLower(token)
		switch lower {
		case "gene", "protein", "family", "domain", "like":
			continue
		}
		return token
	}
	return ""
}

func sortKeywordRows(rows []model.KeywordResultRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		return strings.ToUpper(firstNonEmpty(rows[i].GeneIdentifier, rows[i].TranscriptID, rows[i].ProteinID)) <
			strings.ToUpper(firstNonEmpty(rows[j].GeneIdentifier, rows[j].TranscriptID, rows[j].ProteinID))
	})
}

func ensureExtraColumns(values map[string]string) map[string]string {
	if values != nil {
		return values
	}
	return make(map[string]string)
}

func cleanText(raw string) string {
	raw = html.UnescapeString(raw)
	raw = strings.ReplaceAll(raw, "\u00a0", " ")
	return strings.TrimSpace(spacePattern.ReplaceAllString(raw, " "))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueStrings(values []string) []string {
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

func cloneProteinMap(values map[string]proteinEntry) map[string]proteinEntry {
	out := make(map[string]proteinEntry, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func looksLikeSpecificIdentifier(value string) bool {
	return tairkeyword.LooksLikeSpecificIdentifier(value)
}

func cacheDir(parts ...string) (string, error) {
	all := append([]string{"tair"}, parts...)
	return appfs.CacheDir(all...)
}

func localFileName(rawURL string) string {
	if parsed, err := url.Parse(rawURL); err == nil {
		if file := path.Base(parsed.Query().Get("filePath")); file != "." && file != "/" {
			return sanitizeFileName(file)
		}
		if file := path.Base(parsed.Path); file != "." && file != "/" {
			return sanitizeFileName(file)
		}
	}
	sum := sha256.Sum256([]byte(rawURL))
	return "tair_" + hex.EncodeToString(sum[:8]) + ".fasta"
}

func sanitizeFileName(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	last := false
	for _, r := range value {
		if r < 32 || strings.ContainsRune(`/\:*?"<>|`, r) {
			if !last {
				b.WriteByte('_')
				last = true
			}
			continue
		}
		b.WriteRune(r)
		last = r == '_'
	}
	out := strings.Trim(b.String(), " ._-")
	if out == "" {
		return "file"
	}
	return out
}

func downloadToCache(ctx context.Context, httpClient *http.Client, rawURL string, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, localFileName(rawURL))
	if info, err := os.Stat(dest); err == nil && !info.IsDir() && info.Size() > 0 {
		if strings.HasSuffix(strings.ToLower(dest), ".gz") {
			return decompressCached(dest)
		}
		return dest, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: unexpected status %s", rawURL, resp.Status)
	}
	tmp := dest + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	_, copyErr := io.CopyBuffer(out, resp.Body, make([]byte, 1024*1024))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return "", closeErr
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if strings.HasSuffix(strings.ToLower(dest), ".gz") {
		return decompressCached(dest)
	}
	return dest, nil
}

func decompressCached(gzPath string) (string, error) {
	target := strings.TrimSuffix(gzPath, ".gz")
	if info, err := os.Stat(target); err == nil && !info.IsDir() && info.Size() > 0 {
		return target, nil
	}
	in, err := os.Open(gzPath)
	if err != nil {
		return "", err
	}
	defer in.Close()
	gz, err := gzip.NewReader(in)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tmp := target + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	_, copyErr := io.CopyBuffer(out, gz, make([]byte, 1024*1024))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return "", closeErr
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return target, nil
}

func atoi(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

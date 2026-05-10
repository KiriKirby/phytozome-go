package lemna

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
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
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/KiriKirby/phytozome-go/internal/searchengine/lemnakeyword"
	"github.com/jszwec/csvutil"
	"github.com/karrick/godirwalk"
	"github.com/klauspost/compress/gzip"
	"golang.org/x/sync/singleflight"
)

const (
	baseURL     = "https://www.lemna.org"
	downloadURL = "https://www.lemna.org/download/"
)

var (
	linkPattern           = regexp.MustCompile(`(?is)<a\s+href="([^"]+)">([^<]+)</a>`)
	spacePattern          = regexp.MustCompile(`\s+`)
	searchNoisePattern    = regexp.MustCompile(`[^a-z0-9]+`)
	symbolTokenPattern    = regexp.MustCompile(`\b[A-Z][A-Z0-9]{1,9}(?:-[A-Z0-9]{1,8})?\d*\b`)
	riceLocusPattern      = regexp.MustCompile(`(?i)^(?:LOC_)?(?:OS)?\d{2}G\d{5}(?:\.\d+)?$`)
	refSeqProteinPattern  = regexp.MustCompile(`(?i)^(?:XP_?)\d+(?:\.\d+)?$`)
	cytochromeP450Pattern = regexp.MustCompile(`(?i)^CYP\d+[A-Z]\d+[A-Z]?$`)
	lemnaReportURLPattern = regexp.MustCompile(`(?i)^https?://(?:www\.)?lemna\.org/report/([^/\s]+)/([^?\s#]+)`)
)

type Client struct {
	baseHTTP *http.Client

	mu                     sync.RWMutex
	speciesCandidates      []model.SpeciesCandidate
	releasesByJBrowseName  map[string]releaseInfo
	ahrdCache              map[string]map[string]ahrdRecord
	proteinTranscriptCache map[string]proteinTranscriptMaps
	fastaIndexCache        map[string]map[string]fastaEntry
	proteinReleaseCache    map[string]map[string]string
	proteinSequenceCache   map[string]string
	keywordEngine          *lemnakeyword.Engine
	keywordIndexCache      map[string]lemnaKeywordIndex
	keywordRowsCache       map[string][]model.KeywordResultRow
	blastCapabilitiesCache map[string]BlastCapability
	localResultsCache      map[string]model.BlastResult
	textCache              map[string]string
	sf                     singleflight.Group
}

type proteinTranscriptMaps struct {
	protToTrans map[string]string
	transToGene map[string]string
}

type speciesMetaResult struct {
	candidate model.SpeciesCandidate
	release   releaseInfo
	ok        bool
}

type speciesCandidatesDisk struct {
	Candidates []model.SpeciesCandidate `json:"candidates"`
	Releases   map[string]releaseInfo   `json:"releases"`
}

type releaseInfo struct {
	RootDir        string
	ReleaseDir     string
	ReleaseURL     string
	DisplayLabel   string
	BlastNDBID     int
	GFFURL         string
	ProteinURL     string
	NucleotideURL  string
	AHRDURL        string
	LastModified   string
	AvailableFiles []downloadFile
}

type downloadFile struct {
	Name string
	URL  string
}

type downloadDir struct {
	Name string
	URL  string
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
	RawColumns []string
}

type ahrdRecord struct {
	ProteinAccession         string `csv:"Protein-Accession"`
	BlastHitAccession        string `csv:"Blast-Hit-Accession"`
	QualityCode              string `csv:"AHRD-Quality-Code"`
	HumanReadableDescription string `csv:"Human-Readable-Description"`
	Interpro                 string `csv:"Interpro"`
	GeneOntologyTerm         string `csv:"Gene-Ontology-Term"`
}

const (
	lemnaSearchTypeReportURL        = "report URL"
	lemnaSearchTypeIdentifier       = "Lemna identifier"
	lemnaSearchTypeRiceLocus        = "rice LOC_Os locus"
	lemnaSearchTypeRefSeqProtein    = "RefSeq XP protein"
	lemnaSearchTypeRiceGeneAlias    = "rice gene alias"
	lemnaSearchTypeCytochromeFamily = "CYP73 family symbol"
	lemnaSearchTypeKeyword          = "keyword"
	lemnaSearchTypeWide             = "wide search"
	lemnaSearchTypeBroad            = "broad search"
)

type lemnaKeywordIndex struct {
	Release          releaseInfo              `json:"release"`
	Species          model.SpeciesCandidate   `json:"species"`
	Rows             []model.KeywordResultRow `json:"rows"`
	ByIdentifier     map[string][]int         `json:"by_identifier"`
	BySearchToken    map[string][]int         `json:"by_search_token"`
	ByNormalizedText map[string][]int         `json:"by_normalized_text"`
}

type lemnaKeywordProgram interface {
	Name() string
	Match(term string) bool
	Search(ctx context.Context, c *Client, index lemnaKeywordIndex, species model.SpeciesCandidate, release releaseInfo, term string, limit int) ([]model.KeywordResultRow, error)
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

// BlastCapability describes detected BLAST capabilities for a release/species.
type BlastCapability struct {
	HasServerNucleotideDB bool
	BlastNDBID            int
	HasServerProteinDB    bool
	ProteinDBID           int
	HasProteinFasta       bool
	ProteinFastaURL       string
	HasNucleotideFasta    bool
	NucleotideFastaURL    string
}

type lemnaBlastSession struct {
	client *http.Client
}

type lemnaBlastSubmission struct {
	JobID     string
	ReportURL string
	Message   string
}

// DetectBlastCapabilities inspects cached release metadata and returns a best-effort
// capability summary for the given species. This function prefers the download
// metadata cache and only attempts lightweight page parsing elsewhere (parsing
// not implemented here - this is a conservative detection that enables CLI UX).
func (c *Client) DetectBlastCapabilities(ctx context.Context, species model.SpeciesCandidate) (BlastCapability, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "detect lemna blast capabilities",
	}, func(runCtx context.Context) (BlastCapability, error) {
		// Return cached capabilities if present.
		c.mu.RLock()
		if c.blastCapabilitiesCache != nil {
			if cached, ok := c.blastCapabilitiesCache[species.JBrowseName]; ok {
				c.mu.RUnlock()
				return cached, nil
			}
		}
		rel, ok := c.releasesByJBrowseName[species.JBrowseName]
		c.mu.RUnlock()
		if !ok {
			return BlastCapability{}, fmt.Errorf("no lemna.org release metadata for %s", species.JBrowseName)
		}

		cap := BlastCapability{
			HasProteinFasta:    rel.ProteinURL != "",
			ProteinFastaURL:    rel.ProteinURL,
			HasNucleotideFasta: rel.NucleotideURL != "",
			NucleotideFastaURL: rel.NucleotideURL,
		}

		c.enrichServerBlastCapability(runCtx, rel, &cap)

		// Persist capability to cache for future quick lookups.
		c.mu.Lock()
		if c.blastCapabilitiesCache == nil {
			c.blastCapabilitiesCache = make(map[string]BlastCapability)
		}
		c.blastCapabilitiesCache[species.JBrowseName] = cap
		c.mu.Unlock()

		return cap, nil
	})
}

func (c *Client) cachedProteinTranscriptMaps(ctx context.Context, release releaseInfo) (map[string]string, map[string]string, error) {
	cacheKey := release.GFFURL
	if cacheKey == "" {
		return map[string]string{}, map[string]string{}, nil
	}

	c.mu.RLock()
	if c.proteinTranscriptCache != nil {
		if cached, ok := c.proteinTranscriptCache[cacheKey]; ok {
			protCopy := make(map[string]string, len(cached.protToTrans))
			for k, v := range cached.protToTrans {
				protCopy[k] = v
			}
			transCopy := make(map[string]string, len(cached.transToGene))
			for k, v := range cached.transToGene {
				transCopy[k] = v
			}
			c.mu.RUnlock()
			return protCopy, transCopy, nil
		}
	}
	c.mu.RUnlock()
	if cached, ok := readCachedJSON[proteinTranscriptDisk]("protein-transcript", cacheKey); ok {
		protCopy := make(map[string]string, len(cached.ProtToTrans))
		for k, v := range cached.ProtToTrans {
			protCopy[k] = v
		}
		transCopy := make(map[string]string, len(cached.TransToGene))
		for k, v := range cached.TransToGene {
			transCopy[k] = v
		}
		c.mu.Lock()
		if c.proteinTranscriptCache == nil {
			c.proteinTranscriptCache = make(map[string]proteinTranscriptMaps)
		}
		c.proteinTranscriptCache[cacheKey] = proteinTranscriptMaps{protToTrans: protCopy, transToGene: transCopy}
		c.mu.Unlock()
		return protCopy, transCopy, nil
	}

	value, err, _ := c.sf.Do("protein-transcript-map:"+cacheKey, func() (any, error) {
		c.mu.RLock()
		if c.proteinTranscriptCache != nil {
			if cached, ok := c.proteinTranscriptCache[cacheKey]; ok {
				protCopy := make(map[string]string, len(cached.protToTrans))
				for k, v := range cached.protToTrans {
					protCopy[k] = v
				}
				transCopy := make(map[string]string, len(cached.transToGene))
				for k, v := range cached.transToGene {
					transCopy[k] = v
				}
				c.mu.RUnlock()
				return proteinTranscriptMaps{protToTrans: protCopy, transToGene: transCopy}, nil
			}
		}
		c.mu.RUnlock()
		if cached, ok := readCachedJSON[proteinTranscriptDisk]("protein-transcript", cacheKey); ok {
			protCopy := make(map[string]string, len(cached.ProtToTrans))
			for k, v := range cached.ProtToTrans {
				protCopy[k] = v
			}
			transCopy := make(map[string]string, len(cached.TransToGene))
			for k, v := range cached.TransToGene {
				transCopy[k] = v
			}
			c.mu.Lock()
			if c.proteinTranscriptCache == nil {
				c.proteinTranscriptCache = make(map[string]proteinTranscriptMaps)
			}
			c.proteinTranscriptCache[cacheKey] = proteinTranscriptMaps{protToTrans: protCopy, transToGene: transCopy}
			c.mu.Unlock()
			return proteinTranscriptMaps{protToTrans: protCopy, transToGene: transCopy}, nil
		}

		protToTrans, transToGene, err := c.buildProteinTranscriptMap(ctx, release)
		if err != nil {
			return proteinTranscriptMaps{}, err
		}
		storedProt := make(map[string]string, len(protToTrans))
		for k, v := range protToTrans {
			storedProt[k] = v
		}
		storedTrans := make(map[string]string, len(transToGene))
		for k, v := range transToGene {
			storedTrans[k] = v
		}
		c.mu.Lock()
		if c.proteinTranscriptCache == nil {
			c.proteinTranscriptCache = make(map[string]proteinTranscriptMaps)
		}
		c.proteinTranscriptCache[cacheKey] = proteinTranscriptMaps{
			protToTrans: storedProt,
			transToGene: storedTrans,
		}
		c.mu.Unlock()
		writeCachedJSON("protein-transcript", cacheKey, proteinTranscriptDisk{
			ProtToTrans: storedProt,
			TransToGene: storedTrans,
		})
		return proteinTranscriptMaps{protToTrans: storedProt, transToGene: storedTrans}, nil
	})
	if err != nil {
		return nil, nil, err
	}
	cached := value.(proteinTranscriptMaps)
	protCopy := make(map[string]string, len(cached.protToTrans))
	for k, v := range cached.protToTrans {
		protCopy[k] = v
	}
	transCopy := make(map[string]string, len(cached.transToGene))
	for k, v := range cached.transToGene {
		transCopy[k] = v
	}
	return protCopy, transCopy, nil
}

func (c *Client) cachedFastaIndex(ctx context.Context, fastaPath string) (map[string]fastaEntry, error) {
	key := strings.TrimSpace(fastaPath)
	if key == "" {
		return nil, nil
	}

	c.mu.RLock()
	if c.fastaIndexCache != nil {
		if cached, ok := c.fastaIndexCache[key]; ok {
			copyIndex := make(map[string]fastaEntry, len(cached))
			for k, v := range cached {
				copyIndex[k] = v
			}
			c.mu.RUnlock()
			return copyIndex, nil
		}
	}
	c.mu.RUnlock()
	if cached, ok := readCachedJSON[map[string]fastaIndexDiskEntry]("fasta-index", key); ok {
		copyIndex := make(map[string]fastaEntry, len(cached))
		for k, v := range cached {
			copyIndex[k] = fastaEntry{Defline: v.Defline, Length: v.Length}
		}
		c.mu.Lock()
		if c.fastaIndexCache == nil {
			c.fastaIndexCache = make(map[string]map[string]fastaEntry)
		}
		c.fastaIndexCache[key] = copyIndex
		c.mu.Unlock()
		return copyIndex, nil
	}

	value, err, _ := c.sf.Do("fasta-index:"+key, func() (any, error) {
		c.mu.RLock()
		if c.fastaIndexCache != nil {
			if cached, ok := c.fastaIndexCache[key]; ok {
				copyIndex := make(map[string]fastaEntry, len(cached))
				for k, v := range cached {
					copyIndex[k] = v
				}
				c.mu.RUnlock()
				return copyIndex, nil
			}
		}
		c.mu.RUnlock()
		if cached, ok := readCachedJSON[map[string]fastaIndexDiskEntry]("fasta-index", key); ok {
			copyIndex := make(map[string]fastaEntry, len(cached))
			for k, v := range cached {
				copyIndex[k] = fastaEntry{Defline: v.Defline, Length: v.Length}
			}
			c.mu.Lock()
			if c.fastaIndexCache == nil {
				c.fastaIndexCache = make(map[string]map[string]fastaEntry)
			}
			c.fastaIndexCache[key] = copyIndex
			c.mu.Unlock()
			return copyIndex, nil
		}

		index, err := buildFastaIndex(ctx, fastaPath)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		if c.fastaIndexCache == nil {
			c.fastaIndexCache = make(map[string]map[string]fastaEntry)
		}
		c.fastaIndexCache[key] = index
		c.mu.Unlock()
		diskIndex := make(map[string]fastaIndexDiskEntry, len(index))
		for k, v := range index {
			diskIndex[k] = fastaIndexDiskEntry{Defline: v.Defline, Length: v.Length}
		}
		writeCachedJSON("fasta-index", key, diskIndex)

		copyIndex := make(map[string]fastaEntry, len(index))
		for k, v := range index {
			copyIndex[k] = v
		}
		return copyIndex, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(map[string]fastaEntry), nil
}

// AvailableBlastPrograms returns allowed BLAST programs for the species based on
// detected capabilities. The returned slice contains program names like:
//   - "blastn", "blastx", "tblastn", "blastp"
func (c *Client) AvailableBlastPrograms(ctx context.Context, species model.SpeciesCandidate) []string {
	progs, err := phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "list lemna blast programs",
	}, func(runCtx context.Context) ([]string, error) {
		if runCtx == nil {
			runCtx = context.Background()
		}
		cap, err := c.DetectBlastCapabilities(runCtx, species)
		if err != nil {
			return nil, err
		}

		progs := make([]string, 0, 4)
		if cap.HasServerNucleotideDB || cap.HasNucleotideFasta {
			progs = append(progs, "blastn")
		}
		if cap.HasServerProteinDB || cap.HasProteinFasta {
			progs = append(progs, "blastx")
		}
		if cap.HasServerNucleotideDB || cap.HasNucleotideFasta {
			progs = append(progs, "tblastn")
		}
		if cap.HasServerProteinDB || cap.HasProteinFasta {
			progs = append(progs, "blastp")
		}
		return progs, nil
	})
	if err != nil {
		return nil
	}
	return progs
}

func (c *Client) enrichServerBlastCapability(ctx context.Context, rel releaseInfo, cap *BlastCapability) {
	type capabilityResult struct {
		program string
		dbID    int
		ok      bool
	}
	programs := []string{"blastn", "tblastn", "blastx", "blastp"}
	results := make([]capabilityResult, len(programs))
	spec := phygoboost.ParallelSpec{Level: phygoboost.ExecHeavy, Domain: "www.lemna.org", Workers: phygoboost.NetworkWorkers(len(programs)), Description: "inspect lemna blast capability"}
	_ = phygoboost.ParallelForSpec(ctx, spec, len(programs), func(ctx context.Context, i int) error {
		program := programs[i]
		pageURL, err := blastFormURL(program)
		if err != nil {
			results[i] = capabilityResult{program: program}
			return nil
		}
		body, err := c.fetchText(ctx, pageURL)
		if err != nil || body == "" {
			results[i] = capabilityResult{program: program}
			return nil
		}
		dbID, ok := findBlastDBID(body, rel)
		results[i] = capabilityResult{program: program, dbID: dbID, ok: ok}
		return nil
	})
	for _, result := range results {
		if !result.ok {
			continue
		}
		switch result.program {
		case "blastn", "tblastn":
			cap.HasServerNucleotideDB = true
			if cap.BlastNDBID == 0 {
				cap.BlastNDBID = result.dbID
			}
		case "blastx", "blastp":
			cap.HasServerProteinDB = true
			cap.ProteinDBID = result.dbID
		}
	}
}

func blastFormURL(program string) (string, error) {
	switch normalizeBlastProgramName(program) {
	case "blastn":
		return baseURL + "/blast/nucleotide/nucleotide", nil
	case "blastx":
		return baseURL + "/blast/nucleotide/protein", nil
	case "tblastn":
		return baseURL + "/blast/protein/nucleotide", nil
	case "blastp":
		return baseURL + "/blast/protein/protein", nil
	default:
		return "", fmt.Errorf("unsupported BLAST program %q", program)
	}
}

func normalizeBlastProgramName(program string) string {
	p := strings.ToLower(strings.TrimSpace(program))
	p = strings.TrimPrefix(p, "local:")
	switch {
	case strings.Contains(p, "tblastn"):
		return "tblastn"
	case strings.Contains(p, "blastx"):
		return "blastx"
	case strings.Contains(p, "blastp"):
		return "blastp"
	case strings.Contains(p, "blastn"):
		return "blastn"
	default:
		return p
	}
}

func findBlastDBID(body string, rel releaseInfo) (int, bool) {
	for _, option := range parseBlastDatasetOptions(body) {
		text := normalizeSearchLoose(option.Text)
		if text == "" {
			continue
		}
		if strings.Contains(text, normalizeSearchLoose(rel.RootDir)) ||
			strings.Contains(text, normalizeSearchLoose(rel.ReleaseDir)) ||
			strings.Contains(text, normalizeSearchLoose(rel.DisplayLabel)) ||
			blastOptionMatchesRoot(text, rel.RootDir) {
			return option.Value, true
		}
	}
	return 0, false
}

func hasBlastDatasetOptions(body string) bool {
	return len(parseBlastDatasetOptions(body)) > 0
}

type blastOption struct {
	Value int
	Text  string
}

func parseBlastOptions(body string) []blastOption {
	re := regexp.MustCompile(`(?is)<option[^>]*\bvalue=["']?(\d+)["']?[^>]*>([^<]+)</option>`)
	matches := re.FindAllStringSubmatch(body, -1)
	options := make([]blastOption, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(match[1]))
		if err != nil {
			continue
		}
		text := cleanText(match[2])
		if strings.EqualFold(text, "Select a Dataset") {
			continue
		}
		options = append(options, blastOption{Value: value, Text: text})
	}
	return options
}

func parseBlastDatasetOptions(body string) []blastOption {
	selectRe := regexp.MustCompile(`(?is)<select\b[^>]*\bname=["']?SELECT_DB["']?[^>]*>(.*?)</select>`)
	match := selectRe.FindStringSubmatch(body)
	if len(match) < 2 {
		return nil
	}
	return parseBlastOptions(match[1])
}

func blastOptionMatchesRoot(optionText string, rootDir string) bool {
	parts := strings.Split(rootDir, "_")
	if len(parts) < 3 {
		return false
	}
	genus := strings.ToLower(parts[0])
	species := strings.ToLower(parts[1])
	clone := strings.ToLower(parts[2])
	return strings.Contains(optionText, genus) &&
		strings.Contains(optionText, species) &&
		strings.Contains(optionText, clone)
}

func blastFormHasDB(body string, dbID int) bool {
	for _, option := range parseBlastDatasetOptions(body) {
		if option.Value == dbID {
			return true
		}
	}
	return false
}

func parseBlastFormDefaults(body string) url.Values {
	form := url.Values{}
	inputRe := regexp.MustCompile(`(?is)<input\b[^>]*>`)
	for _, input := range inputRe.FindAllString(body, -1) {
		name := htmlAttr(input, "name")
		if name == "" {
			continue
		}
		form.Set(name, htmlAttr(input, "value"))
	}
	selectRe := regexp.MustCompile(`(?is)<select\b[^>]*\bname=["']?([^"'\s>]+)["']?[^>]*>(.*?)</select>`)
	for _, match := range selectRe.FindAllStringSubmatch(body, -1) {
		if len(match) < 3 {
			continue
		}
		name := html.UnescapeString(match[1])
		if form.Get(name) != "" {
			continue
		}
		if value := selectedOptionValue(match[2]); value != "" {
			form.Set(name, value)
		}
	}
	return form
}

func htmlAttr(tag string, attr string) string {
	re := regexp.MustCompile(`(?is)\b` + regexp.QuoteMeta(attr) + `=["']([^"']*)["']`)
	if match := re.FindStringSubmatch(tag); len(match) >= 2 {
		return html.UnescapeString(match[1])
	}
	return ""
}

func selectedOptionValue(selectBody string) string {
	selectedRe := regexp.MustCompile(`(?is)<option[^>]*\bselected=["']?selected["']?[^>]*\bvalue=["']?([^"'\s>]*)["']?`)
	if match := selectedRe.FindStringSubmatch(selectBody); len(match) >= 2 {
		return html.UnescapeString(match[1])
	}
	optionRe := regexp.MustCompile(`(?is)<option[^>]*\bvalue=["']?([^"'\s>]*)["']?[^>]*>`)
	if match := optionRe.FindStringSubmatch(selectBody); len(match) >= 2 {
		return html.UnescapeString(match[1])
	}
	return ""
}

func ensureFASTA(sequence string) string {
	sequence = strings.TrimSpace(sequence)
	if strings.HasPrefix(sequence, ">") {
		return sequence
	}
	return ">query\n" + sequence + "\n"
}

func sanitizeFASTAForLemna(sequence string) string {
	sequence = strings.ReplaceAll(sequence, "\r\n", "\n")
	sequence = strings.ReplaceAll(sequence, "\r", "\n")
	return ensureFASTA(sequence)
}

func extractBlastJobID(value string) string {
	for _, pattern := range []string{
		`(?i)job(?:\s|-)?id[:=\s]*([0-9a-zA-Z_-]+)`,
		`(?i)/blast/(?:results|report|job)/([0-9a-zA-Z._~-]+)`,
		`(?i)rid=([0-9a-zA-Z_-]+)`,
	} {
		if match := regexp.MustCompile(pattern).FindStringSubmatch(value); len(match) >= 2 {
			return match[1]
		}
	}
	return ""
}

func extractBlastReportURL(body string, requestURL string) string {
	patterns := []string{
		`(?is)href=["']([^"']*/blast/report/[^"']+)["']`,
		`(?is)action=["']([^"']*/blast/report/[^"']+)["']`,
		`(?i)(https?://[^"'\\s>]+/blast/report/[^"'\\s<]+)`,
		`(?i)(/blast/report/[^"'\\s<]+)`,
	}
	for _, pattern := range patterns {
		matches := regexp.MustCompile(pattern).FindAllStringSubmatch(body, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			candidate := strings.TrimSpace(html.UnescapeString(match[1]))
			if candidate == "" {
				continue
			}
			return resolveURL(requestURL, candidate)
		}
	}
	return ""
}

func extractBlastPageMessage(body string) string {
	messageRe := regexp.MustCompile(`(?is)<div[^>]*class=["'][^"']*messages(?:\s+[a-z]+)?[^"']*["'][^>]*>(.*?)</div>`)
	match := messageRe.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	text := cleanText(stripHTMLTags(match[1]))
	return strings.TrimSpace(text)
}

func stripHTMLTags(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}
	tagRe := regexp.MustCompile(`(?is)<[^>]+>`)
	return html.UnescapeString(tagRe.ReplaceAllString(input, " "))
}

func isBlastPendingPage(body string) bool {
	text := strings.ToLower(cleanText(stripHTMLTags(body)))
	return strings.Contains(text, "blast job in queue") ||
		strings.Contains(text, "blast job in progress") ||
		strings.Contains(text, "your blast has been registered and will be started shortly") ||
		strings.Contains(text, "your blast job is currently running") ||
		strings.Contains(text, "this page will automatically refresh")
}

func isBlastCompletedPage(body string) bool {
	lower := strings.ToLower(body)
	if strings.Contains(lower, "tab-delimited") && strings.Contains(lower, "/blast/") {
		return true
	}
	text := strings.ToLower(cleanText(stripHTMLTags(body)))
	return strings.Contains(text, "resulting blast hits") ||
		strings.Contains(text, "number of results") ||
		strings.Contains(text, "submission date")
}

func lemnaReportURLForJob(jobID string) string {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return ""
	}
	return baseURL + "/blast/report/" + strings.TrimLeft(jobID, "/")
}

func extractBlastDownloadURL(body string, reportURL string, linkText string, suffix string) string {
	linkText = strings.ToLower(strings.TrimSpace(linkText))
	suffix = strings.ToLower(strings.TrimSpace(suffix))
	linkRe := regexp.MustCompile(`(?is)<a[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	matches := linkRe.FindAllStringSubmatch(body, -1)
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		href := strings.TrimSpace(html.UnescapeString(match[1]))
		label := strings.ToLower(cleanText(stripHTMLTags(match[2])))
		if href == "" {
			continue
		}
		if linkText != "" && strings.Contains(label, linkText) {
			return resolveURL(reportURL, href)
		}
		if suffix != "" && strings.HasSuffix(strings.ToLower(href), suffix) {
			return resolveURL(reportURL, href)
		}
	}
	return ""
}

func extractBlastTargetLabelFromReport(body string) string {
	patterns := []string{
		`(?is)<strong>\s*Search Target\s*</strong>\s*:\s*([^<]+)</div>`,
		`(?is)Search Target\s*</strong>\s*:\s*([^<]+)<`,
	}
	for _, pattern := range patterns {
		match := regexp.MustCompile(pattern).FindStringSubmatch(body)
		if len(match) < 2 {
			continue
		}
		label := strings.TrimSpace(cleanText(stripHTMLTags(match[1])))
		if label != "" {
			return label
		}
	}
	return ""
}

type officialClone struct {
	RootDir        string
	ScientificName string
	ShortName      string
	CloneID        string
	DisplayName    string
	CommonName     string
}

var officialClones = []officialClone{
	{RootDir: "Sp_polyrhiza_9509", ScientificName: "Spirodela polyrhiza", ShortName: "Sp. polyrhiza", CloneID: "9509", DisplayName: "Spirodela polyrhiza 9509 REF-OXFORD-3.0", CommonName: "giant duckweed"},
	{RootDir: "Le_gibba_7742a", ScientificName: "Lemna gibba", ShortName: "Le. gibba", CloneID: "7742a", DisplayName: "Lemna gibba 7742a REF-CSHL-1.0", CommonName: "duckweed"},
	{RootDir: "Le_japonica_7182", ScientificName: "Lemna japonica", ShortName: "Le. japonica", CloneID: "7182", DisplayName: "Lemna japonica 7182 REF-CSHL-1.0", CommonName: "duckweed"},
	{RootDir: "Le_japonica_8627", ScientificName: "Lemna japonica", ShortName: "Le. japonica", CloneID: "8627", DisplayName: "Lemna japonica 8627 REF-CSHL-1.0", CommonName: "duckweed"},
	{RootDir: "Le_japonica_9421", ScientificName: "Lemna japonica", ShortName: "Le. japonica", CloneID: "9421", DisplayName: "Lemna japonica 9421 REF-CSHL-1.0", CommonName: "duckweed"},
	{RootDir: "Le_minor_7210", ScientificName: "Lemna minor", ShortName: "Le. minor", CloneID: "7210", DisplayName: "Lemna minor 7210 REF-CSHL-1.0", CommonName: "duckweed"},
	{RootDir: "Le_minor_9252", ScientificName: "Lemna minor", ShortName: "Le. minor", CloneID: "9252", DisplayName: "Lemna minor 9252 REF-CSHL-1.0", CommonName: "duckweed"},
	{RootDir: "Le_turionifera_9434", ScientificName: "Lemna turionifera", ShortName: "Le. turionifera", CloneID: "9434", DisplayName: "Lemna turionifera 9434 REF-CSHL-1.0", CommonName: "duckweed"},
	{RootDir: "Wo_australiana_8730", ScientificName: "Wolffia australiana", ShortName: "Wo. australiana", CloneID: "8730", DisplayName: "Wolffia australiana 8730 REF-CSHL-1.0", CommonName: "watermeal"},
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = phygoboost.HTTPClient()
	}
	return &Client{
		baseHTTP:               httpClient,
		releasesByJBrowseName:  make(map[string]releaseInfo),
		ahrdCache:              make(map[string]map[string]ahrdRecord),
		proteinTranscriptCache: make(map[string]proteinTranscriptMaps),
		fastaIndexCache:        make(map[string]map[string]fastaEntry),
		proteinReleaseCache:    make(map[string]map[string]string),
		proteinSequenceCache:   make(map[string]string),
		keywordIndexCache:      make(map[string]lemnaKeywordIndex),
		keywordRowsCache:       make(map[string][]model.KeywordResultRow),
		blastCapabilitiesCache: make(map[string]BlastCapability),
		localResultsCache:      make(map[string]model.BlastResult),
		textCache:              make(map[string]string),
	}
}

func (c *Client) Name() string {
	return "lemna.org"
}

func (c *Client) keywordSearchEngine() *lemnakeyword.Engine {
	c.mu.RLock()
	engine := c.keywordEngine
	c.mu.RUnlock()
	if engine != nil {
		return engine
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.keywordEngine == nil {
		c.keywordEngine = lemnakeyword.New(c)
	}
	return c.keywordEngine
}

func (c *Client) FetchSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "fetch lemna species candidates",
	}, c.fetchSpeciesCandidates)
}

func (c *Client) fetchSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {
	c.mu.RLock()
	if len(c.speciesCandidates) > 0 {
		cached := append([]model.SpeciesCandidate(nil), c.speciesCandidates...)
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()
	if cached, ok := readCachedJSON[speciesCandidatesDisk]("species-candidates", "download"); ok && len(cached.Candidates) > 0 {
		c.mu.Lock()
		c.speciesCandidates = append([]model.SpeciesCandidate(nil), cached.Candidates...)
		c.releasesByJBrowseName = cloneReleaseMap(cached.Releases)
		c.mu.Unlock()
		return append([]model.SpeciesCandidate(nil), cached.Candidates...), nil
	}

	value, err, _ := c.sf.Do("species-candidates", func() (any, error) {
		c.mu.RLock()
		if len(c.speciesCandidates) > 0 {
			cached := append([]model.SpeciesCandidate(nil), c.speciesCandidates...)
			c.mu.RUnlock()
			return cached, nil
		}
		c.mu.RUnlock()
		if cached, ok := readCachedJSON[speciesCandidatesDisk]("species-candidates", "download"); ok && len(cached.Candidates) > 0 {
			c.mu.Lock()
			c.speciesCandidates = append([]model.SpeciesCandidate(nil), cached.Candidates...)
			c.releasesByJBrowseName = cloneReleaseMap(cached.Releases)
			c.mu.Unlock()
			return append([]model.SpeciesCandidate(nil), cached.Candidates...), nil
		}

		rootDirs, err := c.listDownloadDirs(ctx, downloadURL)
		if err != nil {
			return nil, err
		}

		candidates := make([]model.SpeciesCandidate, 0, len(rootDirs))
		releases := make(map[string]releaseInfo, len(rootDirs))
		var resultsMu sync.Mutex
		spec := phygoboost.ParallelSpec{Level: phygoboost.ExecHeavy, Domain: "www.lemna.org", Workers: phygoboost.NetworkWorkers(len(rootDirs)), Description: "inspect lemna species downloads"}
		if err := phygoboost.ParallelForSpec(ctx, spec, len(rootDirs), func(ctx context.Context, i int) error {
			result := c.inspectRootDownloadDir(ctx, rootDirs[i])
			if result.ok {
				resultsMu.Lock()
				candidates = append(candidates, result.candidate)
				releases[result.candidate.JBrowseName] = result.release
				resultsMu.Unlock()
			}
			return nil
		}); err != nil {
			return nil, err
		}

		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].IsOfficial != candidates[j].IsOfficial {
				return candidates[i].IsOfficial && !candidates[j].IsOfficial
			}
			return strings.ToLower(candidates[i].DisplayLabel()) < strings.ToLower(candidates[j].DisplayLabel())
		})

		if len(candidates) == 0 {
			return nil, fmt.Errorf("no lemna.org download species found")
		}

		c.mu.Lock()
		c.speciesCandidates = append([]model.SpeciesCandidate(nil), candidates...)
		c.releasesByJBrowseName = releases
		c.mu.Unlock()
		writeCachedJSON("species-candidates", "download", speciesCandidatesDisk{
			Candidates: append([]model.SpeciesCandidate(nil), candidates...),
			Releases:   cloneReleaseMap(releases),
		})

		return append([]model.SpeciesCandidate(nil), candidates...), nil
	})
	if err != nil {
		return nil, err
	}
	return value.([]model.SpeciesCandidate), nil
}

func cloneReleaseMap(values map[string]releaseInfo) map[string]releaseInfo {
	out := make(map[string]releaseInfo, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func (c *Client) inspectRootDownloadDir(ctx context.Context, root downloadDir) speciesMetaResult {
	if !looksLikeSpeciesDir(root.Name) {
		return speciesMetaResult{}
	}
	rootName := strings.TrimSuffix(root.Name, "/")
	clone, ok := officialCloneByRootDir(rootName)

	releaseDirs, err := c.listDownloadDirs(ctx, root.URL)
	if err != nil || len(releaseDirs) == 0 {
		return speciesMetaResult{}
	}
	release := choosePreferredRelease(root.Name, releaseDirs)
	release.RootDir = rootName
	release.BlastNDBID = blastNDBID(release.RootDir, release.ReleaseDir)
	if err := c.populateReleaseFiles(ctx, &release); err != nil {
		return speciesMetaResult{}
	}

	label := ""
	if ok {
		label = clone.DisplayName
	}
	if label == "" {
		label = formatSpeciesLabel(rootName, release.ReleaseDir)
	}
	release.DisplayLabel = label

	return speciesMetaResult{
		ok:      true,
		release: release,
		candidate: model.SpeciesCandidate{
			ProteomeID:  release.BlastNDBID,
			JBrowseName: rootName,
			GenomeLabel: label,
			CommonName: func() string {
				if ok {
					return clone.CommonName
				}
				return ""
			}(),
			ReleaseDate: release.LastModified,
			SearchAlias: "",
			IsOfficial:  ok,
		},
	}
}

func FilterSpeciesCandidates(candidates []model.SpeciesCandidate, keyword string) []model.SpeciesCandidate {
	keyword = strings.TrimSpace(strings.ToLower(keyword))
	if keyword == "" {
		return append([]model.SpeciesCandidate(nil), candidates...)
	}
	needleLoose := normalizeSearchLoose(keyword)
	needleTight := normalizeSearchTight(keyword)

	filtered := make([]model.SpeciesCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		searchText := candidate.SearchText()
		loose := normalizeSearchLoose(searchText)
		tight := normalizeSearchTight(searchText)
		if strings.Contains(searchText, keyword) ||
			(needleLoose != "" && strings.Contains(loose, needleLoose)) ||
			(needleTight != "" && strings.Contains(tight, needleTight)) {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func (c *Client) SubmitBlast(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "submit lemna blast",
	}, func(runCtx context.Context) (model.BlastJob, error) {
		return c.submitBlast(runCtx, req)
	})
}

func (c *Client) submitBlast(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(req.Program)), "local:") {
		prog := strings.TrimSpace(req.Program[len("local:"):])
		req.Program = prog
		return c.RunLocalBlast(ctx, req)
	}

	program := normalizeBlastProgramName(req.Program)
	cap, capErr := c.DetectBlastCapabilities(ctx, req.Species)
	if capErr != nil {
		return model.BlastJob{}, capErr
	}

	switch program {
	case "blastn", "tblastn":
		if cap.HasServerNucleotideDB {
			job, serr := c.submitBlastToServer(ctx, req)
			if serr == nil {
				return job, nil
			}
			if cap.HasNucleotideFasta {
				job, lerr := c.RunLocalBlast(ctx, req)
				if lerr == nil {
					job.Message = fmt.Sprintf("%s (server submit failed: %v; ran local BLAST instead)", job.Message, serr)
					return job, nil
				}
				return model.BlastJob{}, fmt.Errorf("server submit failed: %v; local nucleotide BLAST fallback failed: %v", serr, lerr)
			}
			return model.BlastJob{}, fmt.Errorf("server submit failed: %v; no local nucleotide FASTA fallback is available for %s", serr, req.Species.DisplayLabel())
		}
		if cap.HasNucleotideFasta {
			return c.RunLocalBlast(ctx, req)
		}
		return model.BlastJob{}, fmt.Errorf("%s requires a nucleotide BLAST database, but no server DB id or local nucleotide FASTA was detected for %s", program, req.Species.DisplayLabel())
	case "blastx", "blastp":
		if cap.HasServerProteinDB {
			job, serr := c.submitBlastToServer(ctx, req)
			if serr == nil {
				return job, nil
			}
			if cap.HasProteinFasta {
				job, lerr := c.RunLocalBlast(ctx, req)
				if lerr == nil {
					job.Message = fmt.Sprintf("%s (server submit failed: %v; ran local BLAST instead)", job.Message, serr)
					return job, nil
				}
				return model.BlastJob{}, fmt.Errorf("server submit failed: %v; local protein BLAST fallback failed: %v", serr, lerr)
			}
			return model.BlastJob{}, fmt.Errorf("server submit failed: %v; no local protein FASTA fallback is available for %s", serr, req.Species.DisplayLabel())
		}
		if cap.HasProteinFasta {
			return c.RunLocalBlast(ctx, req)
		}
		return model.BlastJob{}, fmt.Errorf("%s requires a protein BLAST database, but no server protein DB id or local protein FASTA was detected for %s", program, req.Species.DisplayLabel())
	default:
		return model.BlastJob{}, fmt.Errorf("unsupported lemna.org BLAST program %q", req.Program)
	}
}

// SubmitBlastServerOnly submits a BLAST job to the lemna.org server path without
// silently falling back to local BLAST. The TUI workflow owns the local fallback
// decision so users can see and approve the transition explicitly.
func (c *Client) SubmitBlastServerOnly(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "submit lemna blast server only",
	}, func(runCtx context.Context) (model.BlastJob, error) {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(req.Program)), "local:") {
			return model.BlastJob{}, fmt.Errorf("server-only BLAST cannot run local program %q", req.Program)
		}
		return c.submitBlastToServer(runCtx, req)
	})
}

func (c *Client) newBlastSession() (*lemnaBlastSession, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create lemna session cookie jar: %w", err)
	}
	client := *c.baseHTTP
	client.Jar = jar
	return &lemnaBlastSession{client: &client}, nil
}

func (s *lemnaBlastSession) do(req *http.Request) (*http.Response, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("lemna blast session is not initialized")
	}
	return s.client.Do(req)
}

func setBlastAdvancedOptions(form url.Values, req model.BlastRequest, program string) {
	if req.AlignmentsToShow > 0 {
		form.Set("maxTarget", strconv.Itoa(req.AlignmentsToShow))
	}
	if strings.TrimSpace(req.EValue) != "" && req.EValue != "-1" {
		form.Set("eVal", strings.TrimSpace(req.EValue))
	}
	if form.Get("eVal") == "" {
		form.Set("eVal", "0.001")
	}
	wordLength := strings.TrimSpace(req.WordLength)
	if wordLength != "" && !strings.EqualFold(wordLength, "default") {
		form.Set("wordSize", wordLength)
	}
	switch program {
	case "blastn":
		matrixChoice := "0"
		if strings.Contains(strings.ToUpper(strings.TrimSpace(req.ComparisonMatrix)), "4,-5") {
			matrixChoice = "4"
		}
		form.Set("M&MScores", matrixChoice)
	case "blastx", "blastp", "tblastn":
		matrix := strings.TrimSpace(req.ComparisonMatrix)
		if matrix == "" || strings.EqualFold(matrix, "default") {
			matrix = "BLOSUM62"
		}
		form.Set("Matrix", matrix)
	}
}

func buildMultipartBlastBody(form url.Values, fasta string) (*bytes.Buffer, string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, values := range form {
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				return nil, "", fmt.Errorf("write multipart field %s: %w", key, err)
			}
		}
	}
	if err := writer.WriteField("UPLOAD[fid]", firstNonEmpty(form.Get("UPLOAD[fid]"), "0")); err != nil {
		return nil, "", fmt.Errorf("write upload fid: %w", err)
	}
	if err := writer.WriteField("FASTA", fasta); err != nil {
		return nil, "", fmt.Errorf("write FASTA field: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("finalize multipart body: %w", err)
	}
	return body, writer.FormDataContentType(), nil
}

func (c *Client) fetchBlastFormPage(ctx context.Context, session *lemnaBlastSession, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", fmt.Errorf("create server BLAST form request: %w", err)
	}
	req.Header.Set("User-Agent", "phytozome-go/lemna")
	resp, err := session.do(req)
	if err != nil {
		return "", fmt.Errorf("fetch server BLAST form: %w", err)
	}
	defer phygoboost.DrainAndClose(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch server BLAST form: unexpected status %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read server BLAST form: %w", err)
	}
	return string(body), nil
}

func (c *Client) submitMultipartBlastForm(ctx context.Context, session *lemnaBlastSession, pageURL string, form url.Values, fasta string) (string, string, error) {
	body, contentType, err := buildMultipartBlastBody(form, fasta)
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pageURL, body)
	if err != nil {
		return "", "", fmt.Errorf("create server submit request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Referer", pageURL)
	req.Header.Set("User-Agent", "phytozome-go/lemna")
	resp, err := session.do(req)
	if err != nil {
		return "", "", fmt.Errorf("submit to lemna.org failed: %w", err)
	}
	defer phygoboost.DrainAndClose(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("lemna.org submit returned status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read lemna.org submit response: %w", err)
	}
	finalURL := pageURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return string(respBody), finalURL, nil
}

// submitBlastToServer is a conservative, best-effort server submission helper.
// It attempts to POST a BLAST job to the lemna.org Tripal BLAST endpoints using
// the same multipart/session semantics as the public form. When the site returns
// an immediate validation or transport failure, this method returns a descriptive
// error so callers can fall back to local BLAST quickly instead of hanging.
func (c *Client) submitBlastToServer(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {
	if _, err := c.releaseForSpecies(ctx, req.Species); err != nil {
		return model.BlastJob{}, fmt.Errorf("no release metadata for species: %w", err)
	}
	program := normalizeBlastProgramName(req.Program)
	cap, err := c.DetectBlastCapabilities(ctx, req.Species)
	if err != nil {
		return model.BlastJob{}, err
	}
	dbID := cap.BlastNDBID
	if program == "blastx" || program == "blastp" {
		dbID = cap.ProteinDBID
	}
	if dbID == 0 {
		return model.BlastJob{}, fmt.Errorf("no server DB id for %s on selected species", program)
	}
	pageURL, err := blastFormURL(program)
	if err != nil {
		return model.BlastJob{}, err
	}
	session, err := c.newBlastSession()
	if err != nil {
		return model.BlastJob{}, err
	}
	pageBody, err := c.fetchBlastFormPage(ctx, session, pageURL)
	if err != nil {
		return model.BlastJob{}, err
	}
	if !hasBlastDatasetOptions(pageBody) {
		return model.BlastJob{}, fmt.Errorf("lemna.org public %s BLAST form does not currently expose any selectable datasets", program)
	}
	if !blastFormHasDB(pageBody, dbID) {
		return model.BlastJob{}, fmt.Errorf("lemna.org server form for %s does not expose DB id %d", program, dbID)
	}

	form := parseBlastFormDefaults(pageBody)
	form.Set("SELECT_DB", strconv.Itoa(dbID))
	form.Set("op", " BLAST ")
	form.Set("blast_program", program)
	setBlastAdvancedOptions(form, req, program)

	respText, finalURL, err := c.submitMultipartBlastForm(ctx, session, pageURL, form, sanitizeFASTAForLemna(req.Sequence))
	if err != nil {
		return model.BlastJob{}, err
	}
	if message := strings.TrimSpace(extractBlastPageMessage(respText)); message != "" {
		return model.BlastJob{}, fmt.Errorf("lemna.org server BLAST rejected the request: %s", message)
	}

	reportURL := extractBlastReportURL(respText, finalURL)
	jobID := extractBlastJobID(finalURL)
	if jobID == "" {
		jobID = extractBlastJobID(reportURL)
	}
	if jobID == "" && reportURL != "" {
		jobID = strings.TrimPrefix(reportURL, baseURL+"/blast/report/")
	}

	if reportURL == "" && jobID != "" {
		reportURL = lemnaReportURLForJob(jobID)
	}
	if reportURL == "" {
		return model.BlastJob{}, fmt.Errorf("lemna.org server BLAST accepted the form but did not expose a report URL for follow-up")
	}
	if jobID == "" {
		jobID = reportURL
	}
	message := "submitted to lemna.org BLAST server"
	if isBlastPendingPage(respText) {
		message = "lemna.org BLAST job submitted and queued"
	}
	return model.BlastJob{
		JobID:   jobID,
		Message: message,
	}, nil
}

func (c *Client) WaitForBlastResults(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "wait lemna blast results",
	}, func(runCtx context.Context) (model.BlastResult, error) {
		return c.waitForBlastResults(runCtx, jobID, pollInterval, timeout)
	})
}

func (c *Client) fetchBlastReportPage(ctx context.Context, session *lemnaBlastSession, reportURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reportURL, nil)
	if err != nil {
		return "", fmt.Errorf("create lemna blast report request: %w", err)
	}
	req.Header.Set("User-Agent", "phytozome-go/lemna")
	resp, err := session.do(req)
	if err != nil {
		return "", fmt.Errorf("fetch lemna blast report: %w", err)
	}
	defer phygoboost.DrainAndClose(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch lemna blast report: unexpected status %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read lemna blast report: %w", err)
	}
	return string(body), nil
}

func (c *Client) downloadBlastResultsTSV(ctx context.Context, session *lemnaBlastSession, resultsURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resultsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create lemna blast TSV request: %w", err)
	}
	req.Header.Set("User-Agent", "phytozome-go/lemna")
	resp, err := session.do(req)
	if err != nil {
		return nil, fmt.Errorf("download lemna blast TSV: %w", err)
	}
	defer phygoboost.DrainAndClose(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download lemna blast TSV: unexpected status %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read lemna blast TSV: %w", err)
	}
	return data, nil
}

func reportURLForJobID(jobID string) string {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(jobID), "http://") || strings.HasPrefix(strings.ToLower(jobID), "https://") {
		return jobID
	}
	if strings.HasPrefix(jobID, "/blast/report/") {
		return resolveURL(baseURL, jobID)
	}
	return lemnaReportURLForJob(jobID)
}

func (c *Client) releaseForTargetLabel(ctx context.Context, label string) (releaseInfo, error) {
	label = normalizeSearchLoose(label)
	if label == "" {
		return releaseInfo{}, fmt.Errorf("empty lemna target label")
	}
	if _, err := c.FetchSpeciesCandidates(ctx); err != nil {
		return releaseInfo{}, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, release := range c.releasesByJBrowseName {
		candidates := []string{release.DisplayLabel, release.RootDir, release.ReleaseDir}
		for _, candidate := range candidates {
			if normalizeSearchLoose(candidate) == label {
				return release, nil
			}
		}
	}
	for _, release := range c.releasesByJBrowseName {
		candidates := []string{release.DisplayLabel, release.RootDir, release.ReleaseDir}
		for _, candidate := range candidates {
			cleaned := normalizeSearchLoose(candidate)
			if cleaned != "" && (strings.Contains(cleaned, label) || strings.Contains(label, cleaned)) {
				return release, nil
			}
		}
	}
	return releaseInfo{}, fmt.Errorf("no lemna release matched target label %q", label)
}

func parseServerBlastTSV(data []byte, release releaseInfo) ([]model.BlastResultRow, error) {
	if len(data) == 0 {
		return nil, nil
	}
	rows, err := parseBlastTabularBuffer(data, nil)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].Species == "" {
			rows[i].Species = release.DisplayLabel
		}
		if rows[i].JBrowseName == "" {
			rows[i].JBrowseName = release.RootDir
		}
		if rows[i].TargetID == 0 {
			rows[i].TargetID = release.BlastNDBID
		}
		if rows[i].GeneReportURL == "" {
			rows[i].GeneReportURL = release.ReleaseURL
		}
	}
	return rows, nil
}

func (c *Client) waitForBlastResults(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, error) {
	// Support returning local-run results cached by LocalBlastRun/LocalBlastRunFull.
	// For local jobs we search the cache directory for a cached TSV result and load it.
	if strings.HasPrefix(jobID, "local-") || strings.HasPrefix(jobID, "local:") {
		if err := ctx.Err(); err != nil {
			return model.BlastResult{}, fmt.Errorf("wait for local blast results canceled: %w", err)
		}
		c.mu.RLock()
		res, ok := c.localResultsCache[jobID]
		c.mu.RUnlock()
		if ok {
			return res, nil
		}
		res, err := c.loadBlastResultFromCache(jobID)
		if err != nil {
			return model.BlastResult{}, fmt.Errorf("local BLAST finished but cached result %q was not found: %w", jobID, err)
		}
		c.mu.Lock()
		c.localResultsCache[jobID] = res
		c.mu.Unlock()
		return res, nil
	}

	reportURL := reportURLForJobID(jobID)
	if reportURL == "" {
		return model.BlastResult{}, fmt.Errorf("lemna.org BLAST report URL is empty for job %q", jobID)
	}
	if pollInterval <= 0 {
		pollInterval = 3 * time.Second
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	session, err := c.newBlastSession()
	if err != nil {
		return model.BlastResult{}, err
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return model.BlastResult{}, fmt.Errorf("wait for lemna blast results canceled: %w", err)
		}
		if time.Now().After(deadline) {
			return model.BlastResult{}, fmt.Errorf("lemna.org BLAST report did not complete within %s", timeout)
		}

		pageBody, err := c.fetchBlastReportPage(ctx, session, reportURL)
		if err != nil {
			return model.BlastResult{}, err
		}
		if message := strings.TrimSpace(extractBlastPageMessage(pageBody)); message != "" {
			lower := strings.ToLower(message)
			if strings.Contains(lower, "cancelled") {
				return model.BlastResult{}, fmt.Errorf("lemna.org BLAST job was cancelled: %s", message)
			}
			if strings.Contains(lower, "unable to load your blast results") {
				return model.BlastResult{}, fmt.Errorf("lemna.org BLAST report failed: %s", message)
			}
		}
		if isBlastPendingPage(pageBody) {
			timer := time.NewTimer(pollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return model.BlastResult{}, fmt.Errorf("wait for lemna blast results canceled: %w", ctx.Err())
			case <-timer.C:
			}
			continue
		}
		if !isBlastCompletedPage(pageBody) {
			return model.BlastResult{}, fmt.Errorf("lemna.org BLAST report page did not expose a completed result view")
		}

		tsvURL := extractBlastDownloadURL(pageBody, reportURL, "tab-delimited", ".tsv")
		if tsvURL == "" {
			return model.BlastResult{}, fmt.Errorf("lemna.org BLAST report did not expose a tab-delimited results download")
		}
		data, err := c.downloadBlastResultsTSV(ctx, session, tsvURL)
		if err != nil {
			return model.BlastResult{}, err
		}
		release, relErr := c.releaseForTargetLabel(ctx, extractBlastTargetLabelFromReport(pageBody))
		if relErr != nil {
			release = releaseInfo{DisplayLabel: "lemna.org", ReleaseURL: reportURL}
		}
		rows, err := parseServerBlastTSV(data, release)
		if err != nil {
			return model.BlastResult{}, fmt.Errorf("parse lemna server BLAST results: %w", err)
		}
		return model.BlastResult{
			JobID:   jobID,
			Message: "lemna.org BLAST result loaded from server report",
			Rows:    rows,
		}, nil
	}
}

// RunLocalBlast executes a local BLAST workflow using available FASTA downloads.
// This dispatches to the LocalBlastRun helper defined in localblast.go which
// performs download, makeblastdb, run blast+, and caches results to disk.
func (c *Client) RunLocalBlast(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		LocalSlots:  1,
		Description: "run lemna local blast wrapper",
	}, func(runCtx context.Context) (model.BlastJob, error) {
		return LocalBlastRun(runCtx, c, req)
	})
}

// loadBlastResultFromCache searches the lemna cache tree for jobID.tsv and
// parses it into a model.BlastResult when found.
func (c *Client) loadBlastResultFromCache(jobID string) (model.BlastResult, error) {
	if strings.TrimSpace(jobID) == "" {
		return model.BlastResult{}, fmt.Errorf("empty job id")
	}
	cacheRoot, err := appfs.CacheDir("lemna")
	if err != nil {
		return model.BlastResult{}, err
	}
	found, err := findBlastResultCacheFile(cacheRoot, jobID)
	if err != nil {
		return model.BlastResult{}, err
	}
	if found == "" {
		return model.BlastResult{}, fmt.Errorf("no cached result file for job %s", jobID)
	}

	f, err := os.Open(found)
	if err != nil {
		return model.BlastResult{}, fmt.Errorf("open cached result: %w", err)
	}
	defer f.Close()

	return parseCachedBlastResultTSV(f, jobID)
}

func findBlastResultCacheFile(cacheRoot string, jobID string) (string, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return "", fmt.Errorf("empty job id")
	}
	if found := findDirectBlastResultCacheFile(filepath.Join(cacheRoot, "localblast"), jobID); found != "" {
		return found, nil
	}
	var found string
	foundErr := errors.New("found cached BLAST result")
	err := godirwalk.Walk(cacheRoot, &godirwalk.Options{
		Unsorted: true,
		Callback: func(pathStr string, entry *godirwalk.Dirent) error {
			if entry == nil || entry.IsDir() {
				return nil
			}
			if entry.Name() == jobID+".tsv" {
				found = pathStr
				return foundErr
			}
			return nil
		},
		ErrorCallback: func(string, error) godirwalk.ErrorAction {
			return godirwalk.SkipNode
		},
	})
	if err != nil && !errors.Is(err, foundErr) && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("walk lemna cache: %w", err)
	}
	return found, nil
}

func findDirectBlastResultCacheFile(localBlastRoot string, jobID string) string {
	speciesDirs, err := os.ReadDir(localBlastRoot)
	if err != nil {
		return ""
	}
	fileName := jobID + ".tsv"
	for _, speciesDir := range speciesDirs {
		if !speciesDir.IsDir() {
			continue
		}
		releaseRoot := filepath.Join(localBlastRoot, speciesDir.Name())
		releaseDirs, err := os.ReadDir(releaseRoot)
		if err != nil {
			continue
		}
		for _, releaseDir := range releaseDirs {
			if !releaseDir.IsDir() {
				continue
			}
			candidate := filepath.Join(releaseRoot, releaseDir.Name(), fileName)
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
				return candidate
			}
		}
	}
	return ""
}

func parseCachedBlastResultTSV(input io.Reader, jobID string) (model.BlastResult, error) {
	reader := csv.NewReader(input)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return model.BlastResult{
				JobID:   jobID,
				Message: "local BLAST result loaded from cache",
				Rows:    []model.BlastResultRow{},
			}, nil
		}
		return model.BlastResult{}, fmt.Errorf("read cached result header: %w", err)
	}
	result := model.BlastResult{
		JobID:   jobID,
		Message: "local BLAST result loaded from cache",
		Rows:    []model.BlastResultRow{},
	}
	if !tsvHeaderContains(header, "subject_id") {
		decoder, err := csvutil.NewDecoder(reader, header...)
		if err != nil {
			return model.BlastResult{}, fmt.Errorf("open legacy cached result decoder: %w", err)
		}
		decoder.AlignRecord = true
		for {
			var record legacyLocalBlastCacheRecord
			if err := decoder.Decode(&record); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return model.BlastResult{}, fmt.Errorf("decode legacy cached result: %w", err)
			}
			if record.Hit == 0 && strings.TrimSpace(record.Protein) == "" {
				continue
			}
			result.Rows = append(result.Rows, model.BlastResultRow{
				SourceDatabase:  "lemna",
				HitNumber:       record.Hit,
				Protein:         record.Protein,
				SubjectID:       record.Protein,
				QueryID:         record.QueryID,
				QueryFrom:       record.QueryFrom,
				QueryTo:         record.QueryTo,
				EValue:          record.EValue,
				PercentIdentity: record.PercentIdentity,
				AlignLength:     record.AlignLength,
				Bitscore:        record.Bitscore,
				Identical:       int(record.PercentIdentity * float64(record.AlignLength) / 100),
			})
		}
		return result, nil
	}

	decoder, err := csvutil.NewDecoder(reader, header...)
	if err != nil {
		return model.BlastResult{}, fmt.Errorf("open cached result decoder: %w", err)
	}
	decoder.AlignRecord = true
	for {
		var record localBlastCacheRecord
		if err := decoder.Decode(&record); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return model.BlastResult{}, fmt.Errorf("decode cached result: %w", err)
		}
		if record.Hit == 0 && strings.TrimSpace(record.Protein) == "" {
			continue
		}
		subjectID := firstNonEmpty(record.SubjectID, record.Protein)
		row := model.BlastResultRow{
			SourceDatabase:  "lemna",
			HitNumber:       record.Hit,
			Protein:         record.Protein,
			SubjectID:       subjectID,
			QueryID:         record.QueryID,
			QueryFrom:       record.QueryFrom,
			QueryTo:         record.QueryTo,
			TargetFrom:      record.TargetFrom,
			TargetTo:        record.TargetTo,
			EValue:          record.EValue,
			PercentIdentity: record.PercentIdentity,
			AlignLength:     record.AlignLength,
			Mismatches:      record.Mismatches,
			GapOpenings:     record.GapOpenings,
			Bitscore:        record.Bitscore,
			Identical:       int(record.PercentIdentity * float64(record.AlignLength) / 100),
			Gaps:            record.GapOpenings,
			TargetLength:    record.TargetLength,
			SequenceID:      record.SequenceID,
			TranscriptID:    record.TranscriptID,
			TargetID:        record.TargetID,
			JBrowseName:     record.JBrowseName,
			GeneReportURL:   record.GeneReportURL,
			Defline:         record.Defline,
		}
		result.Rows = append(result.Rows, row)
	}
	return result, nil
}

type legacyLocalBlastCacheRecord struct {
	Hit             int     `csv:"hit"`
	Protein         string  `csv:"protein"`
	QueryID         string  `csv:"qseqid"`
	QueryFrom       int     `csv:"qstart"`
	QueryTo         int     `csv:"qend"`
	EValue          string  `csv:"evalue"`
	PercentIdentity float64 `csv:"pident"`
	AlignLength     int     `csv:"align_len"`
	Bitscore        float64 `csv:"bitscore"`
}

func tsvHeaderContains(header []string, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, value := range header {
		if strings.EqualFold(strings.TrimSpace(value), name) {
			return true
		}
	}
	return false
}

func (c *Client) FetchGeneQuerySequence(ctx context.Context, species model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "fetch lemna gene query sequence",
	}, func(runCtx context.Context) (*model.QuerySequenceSource, error) {
		return c.fetchGeneQuerySequence(runCtx, species, reportType, identifier)
	})
}

func (c *Client) fetchGeneQuerySequence(ctx context.Context, species model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, error) {
	rows, err := c.SearchKeywordRows(ctx, species, identifier)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if strings.EqualFold(row.GeneIdentifier, identifier) || strings.EqualFold(row.TranscriptID, identifier) {
			sequence, err := c.FetchProteinSequence(ctx, 0, row.SequenceID)
			if err != nil {
				return nil, err
			}
			return &model.QuerySequenceSource{
				Sequence:          sequence,
				SourceDatabase:    c.Name(),
				SourceProteomeID:  species.ProteomeID,
				SourceJBrowseName: species.JBrowseName,
				SourceGenomeLabel: species.GenomeLabel,
				GeneID:            row.GeneIdentifier,
				TranscriptID:      row.TranscriptID,
				ProteinID:         row.SequenceID,
				OrganismShort:     species.JBrowseName,
				Annotation:        species.GenomeLabel,
			}, nil
		}
	}
	return nil, fmt.Errorf("no lemna.org gene or transcript matched %q", identifier)
}

func (c *Client) FetchUniProtAccessions(ctx context.Context, targetID int, proteinID string) ([]string, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "fetch lemna uniprot accessions",
	}, func(runCtx context.Context) ([]string, error) {
		return c.fetchUniProtAccessions(runCtx, targetID, proteinID)
	})
}

func (c *Client) fetchUniProtAccessions(ctx context.Context, targetID int, proteinID string) ([]string, error) {
	proteinID = strings.TrimSpace(proteinID)
	if proteinID == "" {
		return nil, nil
	}
	release, err := c.releaseForTargetID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	ahrd, err := c.loadAHRDRecords(ctx, release)
	if err != nil {
		return nil, err
	}
	if len(ahrd) == 0 {
		return nil, nil
	}

	candidates := normalizedIdentifierCandidates(proteinID)
	protToTrans, transToGene, mapErr := c.cachedProteinTranscriptMaps(ctx, release)
	if mapErr == nil {
		seed := append([]string(nil), candidates...)
		for _, candidate := range seed {
			if transcriptID, ok := lookupNormalizedMapValue(protToTrans, candidate); ok {
				candidates = append(candidates, normalizedIdentifierCandidates(transcriptID)...)
				if geneID, ok := lookupNormalizedMapValue(transToGene, transcriptID); ok {
					candidates = append(candidates, normalizedIdentifierCandidates(geneID)...)
				}
			}
			if geneID, ok := lookupNormalizedMapValue(transToGene, candidate); ok {
				candidates = append(candidates, normalizedIdentifierCandidates(geneID)...)
			}
		}
	}
	candidates = uniqueNormalizedStrings(candidates)

	accessions := make([]string, 0, 2)
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		rec, ok := lookupAHRDRecord(ahrd, candidate)
		if !ok {
			continue
		}
		if accession := uniprotAccessionFromAHRD(rec); accession != "" {
			key := strings.ToUpper(accession)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			accessions = append(accessions, accession)
		}
	}
	return accessions, nil
}

func (c *Client) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "search lemna keyword rows",
	}, func(runCtx context.Context) ([]model.KeywordResultRow, error) {
		return c.keywordSearchEngine().SearchKeywordRows(runCtx, species, keyword)
	})
}

func (c *Client) SearchKeywordRowsWide(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "search lemna keyword rows wide",
	}, func(runCtx context.Context) ([]model.KeywordResultRow, error) {
		return c.keywordSearchEngine().SearchKeywordRowsWide(runCtx, species, keyword)
	})
}

func (c *Client) SearchKeywordRowsBroad(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "search lemna keyword rows broad",
	}, func(runCtx context.Context) ([]model.KeywordResultRow, error) {
		return c.keywordSearchEngine().SearchKeywordRowsBroad(runCtx, species, keyword)
	})
}

func (c *Client) SearchKeywordRowsEngine(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "search lemna keyword rows engine",
	}, func(runCtx context.Context) ([]model.KeywordResultRow, error) {
		return c.searchKeywordRowsWithProgram(runCtx, species, keyword, c.selectKeywordProgram(keyword), true, "normal")
	})
}

func (c *Client) SearchKeywordRowsWideEngine(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "search lemna keyword rows wide engine",
	}, func(runCtx context.Context) ([]model.KeywordResultRow, error) {
		return c.searchKeywordRowsWithProgram(runCtx, species, keyword, lemnaWideKeywordProgram{}, false, "forced-wide")
	})
}

func (c *Client) SearchKeywordRowsBroadEngine(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "search lemna keyword rows broad engine",
	}, func(runCtx context.Context) ([]model.KeywordResultRow, error) {
		return c.searchKeywordRowsWithProgram(runCtx, species, keyword, lemnaBroadKeywordProgram{}, false, "forced-broad")
	})
}

func (c *Client) searchKeywordRowsWithProgram(ctx context.Context, species model.SpeciesCandidate, keyword string, program lemnaKeywordProgram, allowWideFallback bool, mode string) ([]model.KeywordResultRow, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}
	cacheKey := c.keywordRowsCacheKey(species, keyword, program.Name(), mode)
	c.mu.RLock()
	if cached, ok := c.keywordRowsCache[cacheKey]; ok && len(cached) > 0 {
		rows := cloneKeywordRows(cached)
		c.mu.RUnlock()
		return rows, nil
	}
	c.mu.RUnlock()
	if cached, ok := readCachedJSON[[]model.KeywordResultRow]("keyword-rows", cacheKey); ok && len(cached) > 0 {
		rows := cloneKeywordRows(cached)
		c.mu.Lock()
		c.keywordRowsCache[cacheKey] = cloneKeywordRows(cached)
		c.mu.Unlock()
		return rows, nil
	}

	value, err, _ := c.sf.Do("keyword-rows:"+cacheKey, func() (any, error) {
		c.mu.RLock()
		if cached, ok := c.keywordRowsCache[cacheKey]; ok && len(cached) > 0 {
			rows := cloneKeywordRows(cached)
			c.mu.RUnlock()
			return rows, nil
		}
		c.mu.RUnlock()
		if cached, ok := readCachedJSON[[]model.KeywordResultRow]("keyword-rows", cacheKey); ok && len(cached) > 0 {
			rows := cloneKeywordRows(cached)
			c.mu.Lock()
			c.keywordRowsCache[cacheKey] = cloneKeywordRows(cached)
			c.mu.Unlock()
			return rows, nil
		}

		release, err := c.releaseForSpecies(ctx, species)
		if err != nil {
			return nil, err
		}
		if release.GFFURL == "" {
			return nil, fmt.Errorf("no GFF3 file found for %s", species.DisplayLabel())
		}
		index, err := c.cachedKeywordIndex(ctx, release, species)
		if err != nil {
			return nil, err
		}
		rows, err := program.Search(ctx, c, index, species, release, keyword, 200)
		if err != nil {
			return nil, err
		}
		searchType := program.Name()
		if len(rows) == 0 && allowWideFallback && program.Name() != lemnaSearchTypeWide {
			wide := lemnaWideKeywordProgram{}
			rows, err = wide.Search(ctx, c, index, species, release, keyword, 200)
			if err != nil {
				return nil, err
			}
			if len(rows) > 0 {
				searchType = program.Name() + " (fallback to wide search)"
			}
		}
		rows = finalizeKeywordRows(rows, keyword, searchType)
		if len(rows) > 0 {
			c.mu.Lock()
			c.keywordRowsCache[cacheKey] = cloneKeywordRows(rows)
			c.mu.Unlock()
			writeCachedJSON("keyword-rows", cacheKey, rows)
		}
		return rows, nil
	})
	if err != nil {
		return nil, err
	}
	return cloneKeywordRows(value.([]model.KeywordResultRow)), nil
}

func (c *Client) keywordRowsCacheKey(species model.SpeciesCandidate, keyword string, program string, mode string) string {
	if legacy := legacyKeywordRowsCacheKey(species, keyword, program, mode); legacy != "" {
		return legacy
	}
	return strings.Join([]string{
		strings.TrimSpace(species.JBrowseName),
		strconv.Itoa(species.ProteomeID),
		strings.ToLower(strings.TrimSpace(keyword)),
		program,
		mode,
	}, "|")
}

func legacyKeywordRowsCacheKey(species model.SpeciesCandidate, keyword string, program string, mode string) string {
	if program == lemnaSearchTypeKeyword && mode == "normal" {
		return species.JBrowseName + "|" + strings.ToLower(strings.TrimSpace(keyword))
	}
	return ""
}

func (c *Client) selectKeywordProgram(term string) lemnaKeywordProgram {
	programs := []lemnaKeywordProgram{
		lemnaReportURLProgram{},
		lemnaRiceLocusProgram{},
		lemnaRefSeqProteinProgram{},
		lemnaCytochromeFamilyProgram{},
		lemnaRiceAliasProgram{},
		lemnaIdentifierProgram{},
		lemnaKeywordProgramDefault{},
	}
	for _, program := range programs {
		if program.Match(term) {
			return program
		}
	}
	return lemnaKeywordProgramDefault{}
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
		if c.keywordIndexCache == nil {
			c.keywordIndexCache = make(map[string]lemnaKeywordIndex)
		}
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
			if c.keywordIndexCache == nil {
				c.keywordIndexCache = make(map[string]lemnaKeywordIndex)
			}
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
			if c.keywordIndexCache == nil {
				c.keywordIndexCache = make(map[string]lemnaKeywordIndex)
			}
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
	return index, nil
}

func (c *Client) loadKeywordRowsForRelease(ctx context.Context, release releaseInfo, species model.SpeciesCandidate) ([]model.KeywordResultRow, error) {
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
		if addIndexedKeywordRow(&rows, rowByTranscript, rowByGene, row) {
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan GFF3 %s: %w", release.GFFURL, err)
	}

	ahrd, err := c.loadAHRDRecords(ctx, release)
	if err == nil && len(ahrd) > 0 {
		for transcriptID, record := range ahrd {
			idx, ok := findKeywordRowIndexForAHRD(rowByTranscript, rowByGene, transcriptID)
			if !ok {
				row := keywordRowFromAHRD(species, release, "", transcriptID, record)
				addIndexedKeywordRow(&rows, rowByTranscript, rowByGene, row)
				continue
			}
			enrichKeywordRowWithAHRD(&rows[idx], "", record)
		}
	}
	return rows, nil
}

func keywordIndexCacheKey(release releaseInfo, species model.SpeciesCandidate) string {
	return strings.Join([]string{
		strings.TrimSpace(species.JBrowseName),
		strconv.Itoa(species.ProteomeID),
		strings.TrimSpace(release.ReleaseDir),
		strings.TrimSpace(release.GFFURL),
		strings.TrimSpace(release.AHRDURL),
	}, "|")
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

func addIndexedKeywordRow(rows *[]model.KeywordResultRow, rowByTranscript map[string]int, rowByGene map[string]int, row model.KeywordResultRow) bool {
	key := firstNonEmpty(row.TranscriptID, row.GeneIdentifier, row.SequenceID, row.Location)
	if key != "" {
		for _, candidate := range normalizedIdentifierCandidates(key) {
			if idx, ok := rowByTranscript[normalizeIdentifierKey(candidate)]; ok {
				mergeKeywordRow(&(*rows)[idx], row)
				return false
			}
			if idx, ok := rowByGene[normalizeIdentifierKey(candidate)]; ok {
				mergeKeywordRow(&(*rows)[idx], row)
				return false
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
	return true
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
	_, _, ok := lemnaReportKeyword(term)
	return ok
}
func (lemnaReportURLProgram) Search(ctx context.Context, c *Client, index lemnaKeywordIndex, species model.SpeciesCandidate, release releaseInfo, term string, limit int) ([]model.KeywordResultRow, error) {
	_, identifier, ok := lemnaReportKeyword(term)
	if !ok {
		return nil, nil
	}
	return searchKeywordIndexIdentifiers(index, []string{identifier}, limit), nil
}

func (lemnaIdentifierProgram) Name() string { return lemnaSearchTypeIdentifier }
func (lemnaIdentifierProgram) Match(term string) bool {
	return looksLikeSpecificKeywordIdentifier(term)
}
func (lemnaIdentifierProgram) Search(ctx context.Context, c *Client, index lemnaKeywordIndex, species model.SpeciesCandidate, release releaseInfo, term string, limit int) ([]model.KeywordResultRow, error) {
	return searchKeywordIndexIdentifiers(index, specificKeywordIdentifierVariants(term), limit), nil
}

func (lemnaRiceLocusProgram) Name() string { return lemnaSearchTypeRiceLocus }
func (lemnaRiceLocusProgram) Match(term string) bool {
	return riceLocusPattern.MatchString(normalizeRiceLocusCandidate(term))
}
func (lemnaRiceLocusProgram) Search(ctx context.Context, c *Client, index lemnaKeywordIndex, species model.SpeciesCandidate, release releaseInfo, term string, limit int) ([]model.KeywordResultRow, error) {
	return searchKeywordIndexAliases(index, riceLocusVariants(term), term, limit), nil
}

func (lemnaRefSeqProteinProgram) Name() string { return lemnaSearchTypeRefSeqProtein }
func (lemnaRefSeqProteinProgram) Match(term string) bool {
	return refSeqProteinPattern.MatchString(strings.TrimSpace(term))
}
func (lemnaRefSeqProteinProgram) Search(ctx context.Context, c *Client, index lemnaKeywordIndex, species model.SpeciesCandidate, release releaseInfo, term string, limit int) ([]model.KeywordResultRow, error) {
	if aliases := aliasesForNormalizedTerm(curatedRiceRefSeqAliasMap(), term); len(aliases) > 0 {
		if rows := searchKeywordIndexAliases(index, aliases, term, limit); len(rows) > 0 {
			return rows, nil
		}
	}
	return searchKeywordIndexIdentifiers(index, specificKeywordIdentifierVariants(term), limit), nil
}

func (lemnaRiceAliasProgram) Name() string { return lemnaSearchTypeRiceGeneAlias }
func (lemnaRiceAliasProgram) Match(term string) bool {
	return len(aliasesForNormalizedTerm(curatedRiceAliasMap(), term)) > 0 || osC4HLike(term)
}
func (lemnaRiceAliasProgram) Search(ctx context.Context, c *Client, index lemnaKeywordIndex, species model.SpeciesCandidate, release releaseInfo, term string, limit int) ([]model.KeywordResultRow, error) {
	aliases := aliasesForNormalizedTerm(curatedRiceAliasMap(), term)
	if len(aliases) == 0 {
		aliases = []string{term}
	}
	return searchKeywordIndexAliases(index, aliases, term, limit), nil
}

func (lemnaCytochromeFamilyProgram) Name() string { return lemnaSearchTypeCytochromeFamily }
func (lemnaCytochromeFamilyProgram) Match(term string) bool {
	return cytochromeP450Pattern.MatchString(strings.TrimSpace(term))
}
func (lemnaCytochromeFamilyProgram) Search(ctx context.Context, c *Client, index lemnaKeywordIndex, species model.SpeciesCandidate, release releaseInfo, term string, limit int) ([]model.KeywordResultRow, error) {
	queries := []string{term, strings.TrimSuffix(strings.ToUpper(strings.TrimSpace(term)), "P")}
	if aliases := aliasesForNormalizedTerm(curatedRiceAliasMap(), term); len(aliases) > 0 {
		queries = append(aliases, queries...)
	}
	return searchKeywordIndexAliases(index, queries, term, limit), nil
}

func (lemnaKeywordProgramDefault) Name() string { return lemnaSearchTypeKeyword }
func (lemnaKeywordProgramDefault) Match(term string) bool {
	return strings.TrimSpace(term) != ""
}
func (lemnaKeywordProgramDefault) Search(ctx context.Context, c *Client, index lemnaKeywordIndex, species model.SpeciesCandidate, release releaseInfo, term string, limit int) ([]model.KeywordResultRow, error) {
	return searchKeywordIndexTerms(index, keywordTerms(term), false, limit), nil
}

func (lemnaWideKeywordProgram) Name() string { return lemnaSearchTypeWide }
func (lemnaWideKeywordProgram) Match(term string) bool {
	return strings.TrimSpace(term) != ""
}
func (lemnaWideKeywordProgram) Search(ctx context.Context, c *Client, index lemnaKeywordIndex, species model.SpeciesCandidate, release releaseInfo, term string, limit int) ([]model.KeywordResultRow, error) {
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
		aliasesForNormalizedTerm(curatedRiceAliasMap(), term),
		aliasesForNormalizedTerm(curatedRiceRefSeqAliasMap(), term),
		riceLocusVariants(term),
		specificKeywordIdentifierVariants(term),
	} {
		if len(aliases) == 0 {
			continue
		}
		add(searchKeywordIndexAliases(index, aliases, term, limit))
		if len(rows) > 0 {
			return rows, nil
		}
	}
	add(searchKeywordIndexTerms(index, keywordTerms(wideKeywordQuery(term)), true, limit))
	if len(rows) > 0 {
		return rows, nil
	}
	for _, query := range relaxedKeywordQueries(term) {
		add(searchKeywordIndexTerms(index, keywordTerms(query), true, limit))
		if len(rows) > 0 {
			return rows, nil
		}
	}
	return rows, nil
}

func (lemnaBroadKeywordProgram) Name() string { return lemnaSearchTypeBroad }
func (lemnaBroadKeywordProgram) Match(term string) bool {
	return strings.TrimSpace(term) != ""
}
func (lemnaBroadKeywordProgram) Search(ctx context.Context, c *Client, index lemnaKeywordIndex, species model.SpeciesCandidate, release releaseInfo, term string, limit int) ([]model.KeywordResultRow, error) {
	if limit <= 0 {
		limit = 10000
	}
	wideRows, err := (lemnaWideKeywordProgram{}).Search(ctx, c, index, species, release, term, limit)
	if err != nil || len(wideRows) > 0 {
		return wideRows, err
	}
	return searchKeywordIndexTerms(index, keywordTerms(term), true, limit), nil
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

func finalizeKeywordRows(rows []model.KeywordResultRow, searchTerm string, searchType string) []model.KeywordResultRow {
	out := cloneKeywordRows(rows)
	for i := range out {
		out[i].SearchTerm = searchTerm
		if strings.TrimSpace(out[i].SearchType) == "" {
			out[i].SearchType = searchType
		}
	}
	sortKeywordRows(out)
	return out
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

func (c *Client) FetchProteinSequence(ctx context.Context, targetID int, sequenceID string) (string, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      "www.lemna.org",
		Description: "fetch lemna protein sequence",
	}, func(runCtx context.Context) (string, error) {
		return c.fetchProteinSequence(runCtx, targetID, sequenceID)
	})
}

func (c *Client) fetchProteinSequence(ctx context.Context, targetID int, sequenceID string) (string, error) {
	sequenceID = strings.TrimSpace(sequenceID)
	if sequenceID == "" {
		return "", fmt.Errorf("empty lemna.org sequence id")
	}
	c.mu.RLock()
	if cached, ok := c.proteinSequenceCache[sequenceID]; ok {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	value, err, _ := c.sf.Do("protein-seq:"+sequenceID, func() (any, error) {
		c.mu.RLock()
		if cached, ok := c.proteinSequenceCache[sequenceID]; ok {
			c.mu.RUnlock()
			return cached, nil
		}
		c.mu.RUnlock()

		if release, err := c.releaseForTargetID(ctx, targetID); err == nil && release.ProteinURL != "" {
			if sequence, ok, err := c.findProteinSequenceInRelease(ctx, release, sequenceID); err == nil && ok {
				c.mu.Lock()
				c.proteinSequenceCache[sequenceID] = sequence
				c.mu.Unlock()
				return sequence, nil
			}
		}
		candidates, err := c.FetchSpeciesCandidates(ctx)
		if err != nil {
			return "", err
		}
		for _, species := range candidates {
			release, err := c.releaseForSpecies(ctx, species)
			if err != nil || release.ProteinURL == "" {
				continue
			}
			sequence, ok, err := c.findProteinSequenceInRelease(ctx, release, sequenceID)
			if err != nil {
				continue
			}
			if ok {
				c.mu.Lock()
				c.proteinSequenceCache[sequenceID] = sequence
				c.mu.Unlock()
				return sequence, nil
			}
		}
		return "", fmt.Errorf("no lemna.org protein sequence matched %s", sequenceID)
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

func (c *Client) listDownloadDirs(ctx context.Context, requestURL string) ([]downloadDir, error) {
	body, err := c.fetchText(ctx, requestURL)
	if err != nil {
		return nil, err
	}
	links := parseLinks(body, requestURL)
	dirs := make([]downloadDir, 0, len(links))
	for _, file := range links {
		name := strings.TrimSpace(file.Name)
		if name == "" || name == "Parent Directory" || !strings.HasSuffix(name, "/") {
			continue
		}
		dirs = append(dirs, downloadDir{Name: name, URL: file.URL})
	}
	return dirs, nil
}

func (c *Client) populateReleaseFiles(ctx context.Context, release *releaseInfo) error {
	body, err := c.fetchText(ctx, release.ReleaseURL)
	if err != nil {
		return err
	}
	files := parseLinks(body, release.ReleaseURL)
	release.AvailableFiles = files
	bestNucleotideScore := 0
	for _, file := range files {
		name := strings.ToLower(file.Name)
		switch {
		case release.GFFURL == "" && strings.HasSuffix(name, ".genes.gff3.gz"):
			release.GFFURL = file.URL
		case release.GFFURL == "" && strings.HasSuffix(name, ".gff3.gz") && strings.Contains(name, "genes"):
			release.GFFURL = file.URL
		}
		switch {
		case release.ProteinURL == "" && strings.HasSuffix(name, ".genes.proteins.primary.fasta.gz"):
			release.ProteinURL = file.URL
		case release.ProteinURL == "" && strings.HasSuffix(name, ".proteins.primary.fasta.gz"):
			release.ProteinURL = file.URL
		case release.ProteinURL == "" && strings.HasSuffix(name, ".proteins.fasta.gz"):
			release.ProteinURL = file.URL
		}
		if score := nucleotideFileScore(name); score > bestNucleotideScore {
			bestNucleotideScore = score
			release.NucleotideURL = file.URL
		}
		if release.AHRDURL == "" && strings.HasSuffix(name, ".ahrd.tar.gz") {
			release.AHRDURL = file.URL
		}
	}
	return nil
}

func nucleotideFileScore(name string) int {
	switch {
	case strings.HasSuffix(name, ".genes.cds.primary.fasta.gz"):
		return 50
	case strings.HasSuffix(name, ".genes.filt.cds.primary.fasta.gz"):
		return 45
	case strings.HasSuffix(name, ".genes.transcripts.primary.fasta.gz"):
		return 40
	case strings.HasSuffix(name, ".genes.filt.transcripts.primary.fasta.gz"):
		return 35
	case strings.HasSuffix(name, ".genes.cds.fasta.gz"):
		return 30
	case strings.HasSuffix(name, ".genes.transcripts.fasta.gz"):
		return 25
	case strings.HasSuffix(name, ".fasta.gz") && !strings.Contains(name, ".genes."):
		return 10
	default:
		return 0
	}
}

func (c *Client) releaseForSpecies(ctx context.Context, species model.SpeciesCandidate) (releaseInfo, error) {
	if _, err := c.FetchSpeciesCandidates(ctx); err != nil {
		return releaseInfo{}, err
	}
	c.mu.RLock()
	release, ok := c.releasesByJBrowseName[species.JBrowseName]
	c.mu.RUnlock()
	if !ok {
		return releaseInfo{}, fmt.Errorf("no lemna.org release metadata for %s", species.JBrowseName)
	}
	return release, nil
}

func (c *Client) searchGFFRows(ctx context.Context, release releaseInfo, species model.SpeciesCandidate, keyword string, limit int) ([]model.KeywordResultRow, error) {
	reader, closeFn, err := c.openMaybeGzip(ctx, release.GFFURL)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	terms := keywordTerms(keyword)
	rows := make([]model.KeywordResultRow, 0, 16)
	rowsByTranscript := make(map[string]model.KeywordResultRow)
	rowsByGene := make(map[string]model.KeywordResultRow)
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		gff, ok := parseGFF3Line(line)
		if !ok || !isSearchableFeatureType(gff.Type) {
			continue
		}
		row := buildKeywordRowFromGFF(species, release, keyword, gff)
		if row.TranscriptID != "" {
			rowsByTranscript[row.TranscriptID] = row
		}
		if row.GeneIdentifier != "" {
			rowsByGene[row.GeneIdentifier] = row
		}
		if !rowMatchesTerms(row, terms) {
			continue
		}
		if addKeywordRow(&rows, seen, row, limit) {
			return rows, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan GFF3 %s: %w", release.GFFURL, err)
	}

	ahrd, err := c.loadAHRDRecords(ctx, release)
	if err == nil {
		for transcriptID, record := range ahrd {
			if !ahrdRecordMatchesTerms(record, terms) {
				continue
			}
			row, ok := rowsByTranscript[transcriptID]
			if !ok {
				row, ok = rowsByGene[stripTranscriptSuffix(transcriptID)]
			}
			if !ok {
				row = model.KeywordResultRow{
					SourceDatabase:      "lemna",
					SearchTerm:          keyword,
					LabelName:           keywordShortLabelFromAHRD(keyword, record),
					ProteinID:           record.ProteinAccession,
					TranscriptID:        transcriptID,
					GeneIdentifier:      stripTranscriptSuffix(transcriptID),
					Genome:              species.DisplayLabel(),
					Description:         record.HumanReadableDescription,
					SequenceHeaderLabel: species.DisplayLabel(),
					SequenceID:          transcriptID,
				}
			}
			row.Description = firstNonEmpty(row.Description, record.HumanReadableDescription)
			row.LabelName = firstNonEmpty(row.LabelName, keywordShortLabelFromAHRD(keyword, record))
			row.ProteinID = firstNonEmpty(row.ProteinID, record.ProteinAccession)
			row.ExtraColumns = ensureExtraColumns(row.ExtraColumns)
			row.ExtraColumns["ahrd_protein_accession"] = record.ProteinAccession
			row.ExtraColumns["ahrd_blast_hit_accession"] = record.BlastHitAccession
			row.ExtraColumns["ahrd_quality_code"] = record.QualityCode
			row.ExtraColumns["ahrd_human_readable_description"] = record.HumanReadableDescription
			row.ExtraColumns["ahrd_interpro"] = record.Interpro
			row.ExtraColumns["ahrd_gene_ontology_term"] = record.GeneOntologyTerm
			if addKeywordRow(&rows, seen, row, limit) {
				return rows, nil
			}
		}
	}
	return rows, nil
}

func (c *Client) loadAHRDRecords(ctx context.Context, release releaseInfo) (map[string]ahrdRecord, error) {
	if release.AHRDURL == "" {
		return nil, fmt.Errorf("no AHRD archive for %s", release.ReleaseDir)
	}
	c.mu.RLock()
	if cached, ok := c.ahrdCache[release.AHRDURL]; ok {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()
	if cached, ok := readCachedJSON[map[string]ahrdRecord]("ahrd", release.AHRDURL); ok {
		c.mu.Lock()
		c.ahrdCache[release.AHRDURL] = cached
		c.mu.Unlock()
		return cached, nil
	}

	value, err, _ := c.sf.Do("ahrd:"+release.AHRDURL, func() (any, error) {
		c.mu.RLock()
		if cached, ok := c.ahrdCache[release.AHRDURL]; ok {
			copyRecords := make(map[string]ahrdRecord, len(cached))
			for k, v := range cached {
				copyRecords[k] = v
			}
			c.mu.RUnlock()
			return copyRecords, nil
		}
		c.mu.RUnlock()
		if cached, ok := readCachedJSON[map[string]ahrdRecord]("ahrd", release.AHRDURL); ok {
			c.mu.Lock()
			c.ahrdCache[release.AHRDURL] = cached
			c.mu.Unlock()
			return cached, nil
		}

		reader, closeFn, err := c.openMaybeGzip(ctx, release.AHRDURL)
		if err != nil {
			return nil, err
		}
		defer closeFn()

		records := make(map[string]ahrdRecord)
		tr := tar.NewReader(reader)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("read AHRD archive %s: %w", release.AHRDURL, err)
			}
			if header == nil || !strings.HasSuffix(header.Name, "ahrd_output.tsv") {
				continue
			}
			if err := parseAHRDOutput(tr, records); err != nil {
				return nil, err
			}
			break
		}
		c.mu.Lock()
		c.ahrdCache[release.AHRDURL] = records
		c.mu.Unlock()
		writeCachedJSON("ahrd", release.AHRDURL, records)

		copyRecords := make(map[string]ahrdRecord, len(records))
		for k, v := range records {
			copyRecords[k] = v
		}
		return copyRecords, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(map[string]ahrdRecord), nil
}

func parseAHRDOutput(reader io.Reader, records map[string]ahrdRecord) error {
	csvReader := csv.NewReader(reader)
	csvReader.Comma = '\t'
	csvReader.FieldsPerRecord = -1
	csvReader.LazyQuotes = true
	decoder, err := csvutil.NewDecoder(csvReader)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("open AHRD TSV decoder: %w", err)
	}
	decoder.AlignRecord = true
	for {
		var record ahrdRecord
		if err := decoder.Decode(&record); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("decode AHRD output: %w", err)
		}
		if strings.TrimSpace(record.ProteinAccession) == "" || strings.HasPrefix(record.ProteinAccession, "#") {
			continue
		}
		records[record.ProteinAccession] = record
	}
	return nil
}

func (c *Client) releaseForTargetID(ctx context.Context, targetID int) (releaseInfo, error) {
	if targetID == 0 {
		return releaseInfo{}, fmt.Errorf("missing lemna target id")
	}
	if _, err := c.FetchSpeciesCandidates(ctx); err != nil {
		return releaseInfo{}, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, release := range c.releasesByJBrowseName {
		if release.BlastNDBID == targetID {
			return release, nil
		}
	}
	return releaseInfo{}, fmt.Errorf("no lemna release for target id %d", targetID)
}

// buildProteinTranscriptMap scans the release GFF3 (if present) and builds mappings
// that help relate protein accessions (or protein identifiers found in FASTA deflines)
// back to transcript IDs and gene IDs. It returns two maps:
//
//	proteinToTranscript[proteinToken] = transcriptID
//	transcriptToGene[transcriptID] = geneID
//
// This is best-effort and intentionally tolerant: it looks for common GFF3 attributes
// such as "protein_id", "transcript_id", "Parent", "ID", and "gene" to construct mappings.
func (c *Client) buildProteinTranscriptMap(ctx context.Context, release releaseInfo) (map[string]string, map[string]string, error) {
	proteinToTranscript := make(map[string]string)
	transcriptToGene := make(map[string]string)

	if release.GFFURL == "" {
		// No GFF available for this release; return empty maps.
		return proteinToTranscript, transcriptToGene, nil
	}

	reader, closeFn, err := c.openMaybeGzip(ctx, release.GFFURL)
	if err != nil {
		return nil, nil, fmt.Errorf("open GFF3 %s: %w", release.GFFURL, err)
	}
	defer closeFn()

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		gff, ok := parseGFF3Line(line)
		if !ok {
			continue
		}
		// Try to discover transcript / protein relationships from attributes.
		attr := gff.AttrMap
		// Candidate transcript identifiers
		transcriptID := firstNonEmpty(attr["transcript_id"], attr["ID"], attr["Name"], attr["protein_id"])
		// Candidate gene identifier (Parent, gene, gene_id)
		geneID := firstNonEmpty(attr["Parent"], attr["gene"], attr["gene_id"])
		// Candidate protein accession/token
		protID := firstNonEmpty(attr["protein_id"], attr["protein"], attr["translation"], attr["protein_accession"])
		// Normalize and store mappings when reasonable.
		if transcriptID != "" && geneID != "" {
			// ensure transcript -> gene mapping
			transcriptToGene[transcriptID] = geneID
			for _, alias := range normalizedIdentifierCandidates(transcriptID) {
				transcriptToGene[alias] = geneID
			}
		}
		if protID != "" {
			// Map protein token(s) to transcript, including normalized aliases.
			for _, alias := range normalizedIdentifierCandidates(protID) {
				proteinToTranscript[alias] = transcriptID
			}
		}
		// For CDS/mRNA lines where protein_id not present but ID/Name exists, attempt to map:
		if protID == "" && transcriptID != "" {
			// Record transcript->gene if possible (already done above).
			// If the feature has a protein-like ID in a different attribute, try common keys.
			if val := firstNonEmpty(attr["Dbxref"], attr["Alias"]); val != "" {
				// take last token as protein alias possibility
				token := val
				if strings.Contains(token, ":") {
					parts := strings.Split(token, ":")
					token = parts[len(parts)-1]
				}
				if token != "" {
					for _, alias := range normalizedIdentifierCandidates(token) {
						proteinToTranscript[alias] = transcriptID
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan GFF3 %s: %w", release.GFFURL, err)
	}
	return proteinToTranscript, transcriptToGene, nil
}

func (c *Client) findProteinSequenceInRelease(ctx context.Context, release releaseInfo, sequenceID string) (string, bool, error) {
	aliases := sequenceAliases(sequenceID)
	if len(aliases) == 0 {
		return "", false, nil
	}
	sequences, err := c.cachedProteinReleaseSequences(ctx, release)
	if err != nil {
		return "", false, err
	}
	for _, alias := range aliases {
		if sequence := strings.TrimSpace(sequences[alias]); sequence != "" {
			return sequence, true, nil
		}
	}
	return "", false, nil
}

func (c *Client) cachedProteinReleaseSequences(ctx context.Context, release releaseInfo) (map[string]string, error) {
	key := strings.TrimSpace(release.ProteinURL)
	if key == "" {
		return nil, fmt.Errorf("missing protein FASTA URL for %s", release.ReleaseDir)
	}
	c.mu.RLock()
	if cached, ok := c.proteinReleaseCache[key]; ok {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	value, err, _ := c.sf.Do("protein-release-seq:"+key, func() (any, error) {
		c.mu.RLock()
		if cached, ok := c.proteinReleaseCache[key]; ok {
			c.mu.RUnlock()
			return cached, nil
		}
		c.mu.RUnlock()

		sequences, err := c.loadProteinReleaseSequences(ctx, release)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		if c.proteinReleaseCache == nil {
			c.proteinReleaseCache = make(map[string]map[string]string)
		}
		c.proteinReleaseCache[key] = sequences
		c.mu.Unlock()
		return sequences, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(map[string]string), nil
}

func (c *Client) loadProteinReleaseSequences(ctx context.Context, release releaseInfo) (map[string]string, error) {
	reader, closeFn, err := c.openMaybeGzip(ctx, release.ProteinURL)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 16*1024*1024)
	sequences := make(map[string]string)
	var header string
	var seq strings.Builder
	flush := func() {
		header = strings.TrimSpace(header)
		sequence := strings.TrimSpace(seq.String())
		if header == "" || sequence == "" {
			return
		}
		token := header
		if fields := strings.Fields(header); len(fields) > 0 {
			token = fields[0]
		}
		for _, alias := range normalizedIdentifierCandidates(token) {
			if alias != "" {
				sequences[alias] = sequence
			}
		}
		for _, value := range strings.FieldsFunc(header, func(r rune) bool {
			return r == '|' || r == ';' || r == ',' || r == ' ' || r == '\t'
		}) {
			for _, alias := range normalizedIdentifierCandidates(value) {
				if alias != "" {
					sequences[alias] = sequence
				}
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
		return nil, fmt.Errorf("scan protein FASTA %s: %w", release.ProteinURL, err)
	}
	flush()
	return sequences, nil
}

func (c *Client) fetchText(ctx context.Context, requestURL string) (string, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      hostForRemoteFile(requestURL),
		Description: "fetch lemna text",
	}, func(runCtx context.Context) (string, error) {
		c.mu.RLock()
		if cached, ok := c.textCache[requestURL]; ok {
			c.mu.RUnlock()
			return cached, nil
		}
		c.mu.RUnlock()

		value, err, _ := c.sf.Do("text:"+requestURL, func() (any, error) {
			c.mu.RLock()
			if cached, ok := c.textCache[requestURL]; ok {
				c.mu.RUnlock()
				return cached, nil
			}
			c.mu.RUnlock()

			req, err := http.NewRequestWithContext(runCtx, http.MethodGet, requestURL, nil)
			if err != nil {
				return "", fmt.Errorf("create lemna.org request: %w", err)
			}
			resp, err := c.baseHTTP.Do(req)
			if err != nil {
				return "", fmt.Errorf("fetch %s: %w", requestURL, err)
			}
			defer phygoboost.DrainAndClose(resp.Body)
			if resp.StatusCode != http.StatusOK {
				return "", fmt.Errorf("fetch %s: unexpected status %s", requestURL, resp.Status)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return "", fmt.Errorf("read %s: %w", requestURL, err)
			}
			text := string(body)
			c.mu.Lock()
			if c.textCache == nil {
				c.textCache = make(map[string]string)
			}
			c.textCache[requestURL] = text
			c.mu.Unlock()
			return text, nil
		})
		if err != nil {
			return "", err
		}
		return value.(string), nil
	})
}

func (c *Client) openMaybeGzip(ctx context.Context, requestURL string) (io.Reader, func(), error) {
	handle, runCtx, err := phygoboost.BindTaskSpec(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		Domain:      hostForRemoteFile(requestURL),
		Description: "open lemna remote data stream",
	})
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(runCtx, http.MethodGet, requestURL, nil)
	if err != nil {
		if handle != nil {
			handle.Release()
		}
		return nil, nil, fmt.Errorf("create lemna.org data request: %w", err)
	}
	resp, err := c.baseHTTP.Do(req)
	if err != nil {
		if handle != nil {
			handle.Release()
		}
		return nil, nil, fmt.Errorf("fetch %s: %w", requestURL, err)
	}
	release := func() {
		phygoboost.DrainAndClose(resp.Body)
		if handle != nil {
			handle.Release()
		}
	}
	if resp.StatusCode != http.StatusOK {
		release()
		return nil, nil, fmt.Errorf("fetch %s: unexpected status %s", requestURL, resp.Status)
	}
	if strings.HasSuffix(strings.ToLower(requestURL), ".gz") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			release()
			return nil, nil, fmt.Errorf("open gzip stream %s: %w", requestURL, err)
		}
		return gz, func() { _ = gz.Close(); release() }, nil
	}
	return resp.Body, release, nil
}

func parseLinks(body string, base string) []downloadFile {
	matches := linkPattern.FindAllStringSubmatch(body, -1)
	files := make([]downloadFile, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		href := html.UnescapeString(strings.TrimSpace(match[1]))
		name := cleanText(match[2])
		if href == "" || name == "" || strings.HasPrefix(href, "?") {
			continue
		}
		files = append(files, downloadFile{Name: name, URL: resolveURL(base, href)})
	}
	return files
}

func choosePreferredRelease(rootName string, dirs []downloadDir) releaseInfo {
	sort.SliceStable(dirs, func(i, j int) bool {
		return releaseScore(strings.TrimSuffix(dirs[i].Name, "/")) > releaseScore(strings.TrimSuffix(dirs[j].Name, "/"))
	})
	return releaseInfo{
		RootDir:    strings.TrimSuffix(rootName, "/"),
		ReleaseDir: strings.TrimSuffix(dirs[0].Name, "/"),
		ReleaseURL: dirs[0].URL,
	}
}

func releaseScore(name string) int {
	lower := strings.ToLower(name)
	score := 0
	if strings.Contains(lower, "ref") {
		score += 1_000_000
	}
	if strings.Contains(lower, "draft") {
		score -= 100_000
	}
	if strings.Contains(lower, "primary") {
		score += 100
	}
	parts := regexp.MustCompile(`\d+`).FindAllString(lower, -1)
	for _, part := range parts {
		n, _ := strconv.Atoi(part)
		score += n
	}
	return score
}

func parseGFF3Line(line string) (gffRow, bool) {
	cols := strings.Split(line, "\t")
	if len(cols) != 9 {
		return gffRow{}, false
	}
	return gffRow{
		SeqID:      cols[0],
		Source:     cols[1],
		Type:       cols[2],
		Start:      cols[3],
		End:        cols[4],
		Score:      cols[5],
		Strand:     cols[6],
		Phase:      cols[7],
		Attributes: cols[8],
		AttrMap:    parseGFFAttributes(cols[8]),
		RawColumns: cols,
	}, true
}

func buildKeywordRowFromGFF(species model.SpeciesCandidate, release releaseInfo, searchTerm string, gff gffRow) model.KeywordResultRow {
	attrs := gff.AttrMap
	id := firstNonEmpty(attrs["ID"], attrs["Name"], attrs["locus"], attrs["gene_id"], attrs["transcript_id"])
	parent := firstNonEmpty(attrs["Parent"], attrs["gene"], attrs["gene_id"])
	transcript := firstNonEmpty(attrs["transcript_id"], attrs["protein_id"], attrs["Name"], id)
	proteinID := firstNonEmpty(attrs["protein_id"], attrs["protein"], attrs["protein_accession"])
	sequenceID := firstNonEmpty(proteinID, transcript, id)
	description := firstNonEmpty(attrs["product"], attrs["description"], attrs["Note"], attrs["note"])
	labelName := keywordShortLabelFromGFF(searchTerm, attrs, description)

	extra := map[string]string{
		"gff_seqid":      gff.SeqID,
		"gff_source":     gff.Source,
		"gff_type":       gff.Type,
		"gff_start":      gff.Start,
		"gff_end":        gff.End,
		"gff_score":      gff.Score,
		"gff_strand":     gff.Strand,
		"gff_phase":      gff.Phase,
		"gff_attributes": gff.Attributes,
		"lemna_release":  release.ReleaseDir,
		"lemna_gff_url":  release.GFFURL,
	}
	for key, value := range attrs {
		extra["attr_"+key] = value
	}

	return model.KeywordResultRow{
		SourceDatabase:      "lemna",
		SearchTerm:          searchTerm,
		LabelName:           labelName,
		ProteinID:           proteinID,
		TranscriptID:        transcript,
		GeneIdentifier:      firstNonEmpty(parent, id),
		Genome:              species.DisplayLabel(),
		Location:            fmt.Sprintf("%s:%s..%s %s", gff.SeqID, gff.Start, gff.End, gff.Strand),
		Aliases:             firstNonEmpty(attrs["Alias"], attrs["Dbxref"]),
		Description:         description,
		Comments:            firstNonEmpty(attrs["Note"], attrs["comment"]),
		AutoDefine:          firstNonEmpty(attrs["product"], attrs["Name"]),
		GeneReportURL:       lemnaGeneReportURL(release.RootDir, firstNonEmpty(parent, id)),
		SequenceHeaderLabel: species.DisplayLabel(),
		SequenceID:          sequenceID,
		ExtraColumns:        extra,
	}
}

func officialCloneByRootDir(rootDir string) (officialClone, bool) {
	for _, clone := range officialClones {
		if clone.RootDir == rootDir {
			return clone, true
		}
	}
	return officialClone{}, false
}

func isSearchableFeatureType(featureType string) bool {
	switch strings.ToLower(strings.TrimSpace(featureType)) {
	case "gene", "mrna", "transcript":
		return true
	default:
		return false
	}
}

func keywordTerms(keyword string) []string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(keyword)))
	if len(fields) == 0 && strings.TrimSpace(keyword) != "" {
		return []string{strings.ToLower(strings.TrimSpace(keyword))}
	}
	return fields
}

func rowMatchesTerms(row model.KeywordResultRow, terms []string) bool {
	if len(terms) == 0 {
		return false
	}
	return textValuesMatchTerms(keywordRowSearchValues(row), terms)
}

func ahrdRecordMatchesTerms(record ahrdRecord, terms []string) bool {
	return textValuesMatchTerms([]string{
		record.ProteinAccession,
		record.BlastHitAccession,
		record.HumanReadableDescription,
		record.Interpro,
		record.GeneOntologyTerm,
	}, terms)
}

func textValuesMatchTerms(values []string, terms []string) bool {
	haystack := strings.ToLower(strings.Join(values, " "))
	for _, term := range terms {
		if term == "" {
			continue
		}
		if !strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}

func addKeywordRow(rows *[]model.KeywordResultRow, seen map[string]struct{}, row model.KeywordResultRow, limit int) bool {
	key := firstNonEmpty(row.TranscriptID, row.GeneIdentifier, row.Location)
	if key == "" {
		key = fmt.Sprintf("%s:%s", row.Genome, row.Description)
	}
	if _, ok := seen[key]; ok {
		return false
	}
	seen[key] = struct{}{}
	*rows = append(*rows, row)
	return limit > 0 && len(*rows) >= limit
}

func ensureExtraColumns(values map[string]string) map[string]string {
	if values != nil {
		return values
	}
	return make(map[string]string)
}

func keywordShortLabelFromGFF(_ string, attrs map[string]string, _ string) string {
	for _, value := range []string{
		attrs["Alias"],
		attrs["alias"],
		attrs["gene_name"],
		attrs["gene_symbol"],
		attrs["symbol"],
		attrs["Name"],
		attrs["gene"],
	} {
		if label := firstSymbolFromDelimited(value); label != "" {
			return label
		}
	}
	return ""
}

func keywordShortLabelFromAHRD(keyword string, record ahrdRecord) string {
	for _, value := range []string{
		record.HumanReadableDescription,
		record.BlastHitAccession,
		record.Interpro,
		record.GeneOntologyTerm,
		keyword,
	} {
		if label := firstSymbolFromDelimited(value); label != "" {
			return label
		}
	}
	return ""
}

func firstSymbolFromDelimited(value string) string {
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ';' || r == ',' || r == '|' || r == '\t' || r == '\n' || r == '\r'
	}) {
		part = strings.TrimSpace(part)
		if label := firstSymbolFromText(part); label != "" {
			return label
		}
	}
	return ""
}

func firstSymbolFromText(value string) string {
	value = cleanText(value)
	if value == "" {
		return ""
	}
	for _, token := range symbolTokenPattern.FindAllString(value, -1) {
		if isLikelyShortLabel(token) {
			return token
		}
	}
	fields := strings.Fields(value)
	if len(fields) == 1 && isLikelyShortLabel(fields[0]) {
		return fields[0]
	}
	return ""
}

func isLikelyShortLabel(value string) bool {
	value = strings.Trim(strings.TrimSpace(value), ".,:;()[]{}")
	if len(value) < 2 || len(value) > 15 {
		return false
	}
	lower := strings.ToLower(value)
	switch lower {
	case "go", "ipr", "pfam", "kegg", "ec", "gene", "mrna", "cds", "rna", "dna", "protein":
		return false
	}
	if strings.Contains(lower, "http") || strings.Contains(lower, "spipo") || strings.Contains(lower, "lem") {
		return false
	}
	hasUpper := false
	hasDigit := false
	hasLower := false
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	if !hasUpper {
		return false
	}
	if strings.Count(value, "_") > 0 {
		return false
	}
	return hasDigit || !hasLower || len(value) <= 6
}

func stripTranscriptSuffix(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, suffix := range []string{"_T001", "_T002", "_T003", ".1", ".2", ".3"} {
		if strings.HasSuffix(value, suffix) {
			return strings.TrimSuffix(value, suffix)
		}
	}
	return value
}

func parseGFFAttributes(value string) map[string]string {
	attrs := make(map[string]string)
	for _, part := range strings.Split(value, ";") {
		if part = strings.TrimSpace(part); part == "" {
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
		key = strings.TrimSpace(key)
		val, _ = url.QueryUnescape(strings.TrimSpace(val))
		if key != "" {
			attrs[key] = val
		}
	}
	return attrs
}

func sequenceAliases(sequenceID string) []string {
	return normalizedIdentifierCandidates(strings.TrimSpace(strings.TrimPrefix(sequenceID, ">")))
}

func fastaHeaderMatches(header string, aliases []string) bool {
	header = strings.TrimPrefix(strings.TrimSpace(header), ">")
	token := header
	if fields := strings.Fields(header); len(fields) > 0 {
		token = fields[0]
	}
	for _, alias := range aliases {
		if alias == "" {
			continue
		}
		if token == alias || strings.Contains(header, alias) {
			return true
		}
	}
	return false
}

func looksLikeSpeciesDir(name string) bool {
	name = strings.TrimSuffix(name, "/")
	return strings.HasPrefix(name, "Le_") || strings.HasPrefix(name, "Sp_") || strings.HasPrefix(name, "Wo_")
}

func blastNDBID(rootDir string, releaseDir string) int {
	switch rootDir {
	case "Le_gibba_7742a":
		return 11
	case "Le_japonica_8627":
		return 12
	case "Le_japonica_7182":
		return 13
	case "Le_japonica_9421":
		return 14
	case "Le_minor_7210":
		return 15
	case "Le_minor_9252":
		return 16
	case "Le_turionifera_9434":
		return 17
	case "Sp_polyrhiza_9509":
		return 18
	case "Wo_australiana_8730":
		return 19
	default:
		return 0
	}
}

func formatSpeciesLabel(rootDir string, releaseDir string) string {
	label := strings.ReplaceAll(rootDir, "_", " ")
	if releaseDir != "" {
		label += " " + releaseDir
	}
	return cleanText(label)
}

func commonName(rootDir string) string {
	switch {
	case strings.HasPrefix(rootDir, "Le_"):
		return "duckweed"
	case strings.HasPrefix(rootDir, "Sp_"):
		return "giant duckweed"
	case strings.HasPrefix(rootDir, "Wo_"):
		return "watermeal"
	default:
		return ""
	}
}

func resolveURL(baseValue string, href string) string {
	parsedBase, err := url.Parse(baseValue)
	if err != nil {
		return href
	}
	parsedHref, err := url.Parse(href)
	if err != nil {
		return href
	}
	if parsedHref.IsAbs() {
		return parsedHref.String()
	}
	if strings.HasPrefix(href, "/") {
		return baseURL + href
	}
	if !strings.HasSuffix(parsedBase.Path, "/") {
		parsedBase.Path = path.Dir(parsedBase.Path) + "/"
	}
	return parsedBase.ResolveReference(parsedHref).String()
}

func cleanText(raw string) string {
	raw = html.UnescapeString(raw)
	raw = strings.ReplaceAll(raw, "\u00a0", " ")
	raw = spacePattern.ReplaceAllString(raw, " ")
	return strings.TrimSpace(raw)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizedIdentifierCandidates(value string) []string {
	value = strings.TrimSpace(strings.TrimPrefix(value, ">"))
	if value == "" {
		return nil
	}
	out := []string{value}
	if fields := strings.Fields(value); len(fields) > 0 {
		out = append(out, fields[0])
	}
	if strings.Contains(value, "|") {
		parts := strings.Split(value, "|")
		out = append(out, parts[len(parts)-1])
		if len(parts) > 1 {
			out = append(out, parts[0])
		}
	}
	if strings.Contains(value, ":") {
		parts := strings.Split(value, ":")
		out = append(out, parts[len(parts)-1])
	}
	seed := append([]string(nil), out...)
	for _, candidate := range seed {
		if strings.Contains(candidate, ".") {
			out = append(out, strings.Split(candidate, ".")[0])
		}
	}
	return uniqueNormalizedStrings(out)
}

func uniqueNormalizedStrings(values []string) []string {
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

func lookupNormalizedMapValue(values map[string]string, key string) (string, bool) {
	for _, candidate := range normalizedIdentifierCandidates(key) {
		if value, ok := values[candidate]; ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

func lookupAHRDRecord(records map[string]ahrdRecord, key string) (ahrdRecord, bool) {
	for _, candidate := range normalizedIdentifierCandidates(key) {
		if record, ok := records[candidate]; ok {
			return record, true
		}
	}
	return ahrdRecord{}, false
}

func lemnaReportKeyword(value string) (rootDir string, identifier string, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}
	match := lemnaReportURLPattern.FindStringSubmatch(value)
	if len(match) < 3 {
		return "", "", false
	}
	return strings.TrimSpace(match[1]), strings.TrimSpace(match[2]), strings.TrimSpace(match[2]) != ""
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
	replacer := strings.NewReplacer("_", "", "-", "", " ", "", ".", "")
	return replacer.Replace(value)
}

func normalizeIdentifierKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

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
	values := []string{
		row.ProteinID,
		row.TranscriptID,
		row.GeneIdentifier,
		row.SequenceID,
	}
	if row.ExtraColumns != nil {
		for _, key := range []string{
			"attr_ID",
			"attr_Name",
			"attr_Parent",
			"attr_protein_id",
			"attr_protein",
			"attr_protein_accession",
			"ahrd_protein_accession",
			"ahrd_blast_hit_accession",
		} {
			values = append(values, row.ExtraColumns[key])
		}
	}
	return values
}

func keywordRowSearchTokens(row model.KeywordResultRow) []string {
	values := keywordRowIdentifiers(row)
	values = append(values, strings.FieldsFunc(row.Aliases, splitKeywordToken)...)
	values = append(values, strings.FieldsFunc(row.UniProt, splitKeywordToken)...)
	if row.ExtraColumns != nil {
		for _, key := range []string{"attr_Alias", "attr_Dbxref", "ahrd_interpro", "ahrd_gene_ontology_term"} {
			values = append(values, strings.FieldsFunc(row.ExtraColumns[key], splitKeywordToken)...)
		}
	}
	return values
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
	values := []string{
		row.LabelName,
		row.ProteinID,
		row.TranscriptID,
		row.GeneIdentifier,
		row.Aliases,
		row.UniProt,
		row.Description,
		row.Comments,
		row.AutoDefine,
		row.SequenceID,
		row.GeneReportURL,
	}
	if row.ExtraColumns != nil {
		for _, key := range []string{
			"attr_ID",
			"attr_Name",
			"attr_Parent",
			"attr_Alias",
			"attr_Dbxref",
			"attr_product",
			"attr_description",
			"attr_Note",
			"attr_note",
			"ahrd_protein_accession",
			"ahrd_blast_hit_accession",
			"ahrd_human_readable_description",
			"ahrd_interpro",
			"ahrd_gene_ontology_term",
		} {
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
		for _, pair := range [][2]string{
			{left.GeneIdentifier, right.GeneIdentifier},
			{left.TranscriptID, right.TranscriptID},
			{left.ProteinID, right.ProteinID},
			{left.Location, right.Location},
		} {
			if pair[0] == pair[1] {
				continue
			}
			return strings.ToLower(pair[0]) < strings.ToLower(pair[1])
		}
		return false
	})
}

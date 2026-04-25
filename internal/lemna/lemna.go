package lemna

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
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
	"golang.org/x/sync/singleflight"
)

const (
	baseURL     = "https://www.lemna.org"
	downloadURL = "https://www.lemna.org/download/"
)

var (
	linkPattern        = regexp.MustCompile(`(?is)<a\s+href="([^"]+)">([^<]+)</a>`)
	spacePattern       = regexp.MustCompile(`\s+`)
	searchNoisePattern = regexp.MustCompile(`[^a-z0-9]+`)
)

type Client struct {
	baseHTTP *http.Client

	mu                     sync.RWMutex
	speciesCandidates      []model.SpeciesCandidate
	releasesByJBrowseName  map[string]releaseInfo
	ahrdCache              map[string]map[string]ahrdRecord
	proteinTranscriptCache map[string]proteinTranscriptMaps
	fastaIndexCache        map[string]map[string]fastaEntry
	proteinSequenceCache   map[string]string
	keywordRowsCache       map[string][]model.KeywordResultRow
	blastCapabilitiesCache map[string]BlastCapability
	localResultsCache      map[string]model.BlastResult
	textCache              map[string]string
	sf                     singleflight.Group
}

const maxParallelReleaseMetadataJobs = 6

type proteinTranscriptMaps struct {
	protToTrans map[string]string
	transToGene map[string]string
}

type speciesMetaResult struct {
	candidate model.SpeciesCandidate
	release   releaseInfo
	ok        bool
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
	ProteinAccession         string
	BlastHitAccession        string
	QualityCode              string
	HumanReadableDescription string
	Interpro                 string
	GeneOntologyTerm         string
}

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

// DetectBlastCapabilities inspects cached release metadata and returns a best-effort
// capability summary for the given species. This function prefers the download
// metadata cache and only attempts lightweight page parsing elsewhere (parsing
// not implemented here - this is a conservative detection that enables CLI UX).
func (c *Client) DetectBlastCapabilities(ctx context.Context, species model.SpeciesCandidate) (BlastCapability, error) {
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
		HasServerNucleotideDB: rel.BlastNDBID != 0,
		BlastNDBID:            rel.BlastNDBID,
		HasProteinFasta:       rel.ProteinURL != "",
		ProteinFastaURL:       rel.ProteinURL,
		HasNucleotideFasta:    rel.NucleotideURL != "",
		NucleotideFastaURL:    rel.NucleotideURL,
	}

	c.enrichServerBlastCapability(ctx, rel, &cap)

	// Persist capability to cache for future quick lookups.
	c.mu.Lock()
	if c.blastCapabilitiesCache == nil {
		c.blastCapabilitiesCache = make(map[string]BlastCapability)
	}
	c.blastCapabilitiesCache[species.JBrowseName] = cap
	c.mu.Unlock()

	return cap, nil
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

func (c *Client) cachedFastaIndex(fastaPath string) (map[string]fastaEntry, error) {
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

		index, err := buildFastaIndex(fastaPath)
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
func (c *Client) AvailableBlastPrograms(species model.SpeciesCandidate) []string {
	cap, err := c.DetectBlastCapabilities(context.Background(), species)
	if err != nil {
		// On error, return no programs so callers can handle the missing capability.
		return nil
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
	return progs
}

func (c *Client) enrichServerBlastCapability(ctx context.Context, rel releaseInfo, cap *BlastCapability) {
	for _, program := range []string{"blastn", "tblastn", "blastx", "blastp"} {
		pageURL, err := blastFormURL(program)
		if err != nil {
			continue
		}
		body, err := c.fetchText(ctx, pageURL)
		if err != nil || body == "" {
			continue
		}
		dbID, ok := findBlastDBID(body, rel)
		if !ok {
			continue
		}
		switch program {
		case "blastn", "tblastn":
			cap.HasServerNucleotideDB = true
			if cap.BlastNDBID == 0 {
				cap.BlastNDBID = dbID
			}
		case "blastx", "blastp":
			cap.HasServerProteinDB = true
			cap.ProteinDBID = dbID
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
	for _, option := range parseBlastOptions(body) {
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
	for _, option := range parseBlastOptions(body) {
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

func extractBlastJobID(value string) string {
	for _, pattern := range []string{
		`(?i)job(?:\s|-)?id[:=\s]*([0-9a-zA-Z_-]+)`,
		`(?i)/blast/(?:results|report|job)/([0-9a-zA-Z_-]+)`,
		`(?i)rid=([0-9a-zA-Z_-]+)`,
	} {
		if match := regexp.MustCompile(pattern).FindStringSubmatch(value); len(match) >= 2 {
			return match[1]
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
		httpClient = http.DefaultClient
	}
	return &Client{
		baseHTTP:               httpClient,
		releasesByJBrowseName:  make(map[string]releaseInfo),
		ahrdCache:              make(map[string]map[string]ahrdRecord),
		proteinTranscriptCache: make(map[string]proteinTranscriptMaps),
		fastaIndexCache:        make(map[string]map[string]fastaEntry),
		proteinSequenceCache:   make(map[string]string),
		keywordRowsCache:       make(map[string][]model.KeywordResultRow),
		blastCapabilitiesCache: make(map[string]BlastCapability),
		localResultsCache:      make(map[string]model.BlastResult),
		textCache:              make(map[string]string),
	}
}

func (c *Client) Name() string {
	return "lemna.org"
}

func (c *Client) FetchSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {
	c.mu.RLock()
	if len(c.speciesCandidates) > 0 {
		cached := append([]model.SpeciesCandidate(nil), c.speciesCandidates...)
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	value, err, _ := c.sf.Do("species-candidates", func() (any, error) {
		c.mu.RLock()
		if len(c.speciesCandidates) > 0 {
			cached := append([]model.SpeciesCandidate(nil), c.speciesCandidates...)
			c.mu.RUnlock()
			return cached, nil
		}
		c.mu.RUnlock()

		rootDirs, err := c.listDownloadDirs(ctx, downloadURL)
		if err != nil {
			return nil, err
		}

		rootJobs := make(chan downloadDir)
		rootResults := make(chan speciesMetaResult, len(rootDirs))
		workerCount := min(len(rootDirs), maxParallelReleaseMetadataJobs)
		if workerCount < 1 {
			workerCount = 1
		}

		var workers sync.WaitGroup
		for range workerCount {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for root := range rootJobs {
					result := c.inspectRootDownloadDir(ctx, root)
					if result.ok {
						rootResults <- result
					}
				}
			}()
		}

		go func() {
			for _, root := range rootDirs {
				rootJobs <- root
			}
			close(rootJobs)
			workers.Wait()
			close(rootResults)
		}()

		candidates := make([]model.SpeciesCandidate, 0, len(rootDirs))
		releases := make(map[string]releaseInfo, len(rootDirs))
		for result := range rootResults {
			candidates = append(candidates, result.candidate)
			releases[result.candidate.JBrowseName] = result.release
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

		return append([]model.SpeciesCandidate(nil), candidates...), nil
	})
	if err != nil {
		return nil, err
	}
	return value.([]model.SpeciesCandidate), nil
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

// submitBlastToServer is a conservative, best-effort server submission helper.
// It attempts to POST a nucleotide BLAST job to the lemna.org endpoints using the
// detected ProteomeID. This implementation is intentionally defensive: if the
// site's submission form or tokens are not available, it returns a descriptive
// error so callers can fall back to local BLAST.
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
	pageBody, err := c.fetchText(ctx, pageURL)
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("fetch server BLAST form: %w", err)
	}
	if !blastFormHasDB(pageBody, dbID) {
		return model.BlastJob{}, fmt.Errorf("lemna.org server form for %s does not expose DB id %d", program, dbID)
	}

	form := parseBlastFormDefaults(pageBody)
	form.Set("FASTA", ensureFASTA(req.Sequence))
	form.Set("SELECT_DB", strconv.Itoa(dbID))
	form.Set("maxTarget", strconv.Itoa(req.AlignmentsToShow))
	if strings.TrimSpace(req.EValue) != "" && req.EValue != "-1" {
		form.Set("eVal", req.EValue)
	}
	if form.Get("eVal") == "" {
		form.Set("eVal", "0.001")
	}
	form.Set("op", " BLAST ")
	form.Set("blast_program", program)

	submitURL := pageURL
	httpReq, err := http.NewRequestWithContext(ctx, "POST", submitURL, strings.NewReader(form.Encode()))
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("create server submit request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Referer", pageURL)
	httpReq.Header.Set("User-Agent", "phytozome-go/lemna")
	resp, err := c.baseHTTP.Do(httpReq)
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("submit to lemna.org failed: %w", err)
	}
	defer resp.Body.Close()

	// If site returned a non-200, treat as failure for robust behavior.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return model.BlastJob{}, fmt.Errorf("lemna.org submit returned status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	// Try to extract a job id or results URL from the response body.
	bbody, _ := io.ReadAll(resp.Body)
	respText := string(bbody)
	jobID := extractBlastJobID(respText)
	if jobID == "" && resp.Request != nil && resp.Request.URL != nil {
		jobID = extractBlastJobID(resp.Request.URL.String())
	}

	if jobID == "" {
		return model.BlastJob{}, fmt.Errorf("could not parse server job id from lemna.org response")
	}

	return model.BlastJob{}, fmt.Errorf("lemna.org accepted server job %s, but automated result retrieval is not implemented for this server response", jobID)
}

func (c *Client) WaitForBlastResults(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, error) {
	// Support returning local-run results cached by LocalBlastRun/LocalBlastRunFull.
	// For local jobs we search the cache directory for a cached TSV result and load it.
	if strings.HasPrefix(jobID, "local-") || strings.HasPrefix(jobID, "local:") {
		deadline := time.Now().Add(timeout)
		if pollInterval <= 0 {
			pollInterval = 1 * time.Second
		}
		for {
			// Check in-memory cache first.
			c.mu.RLock()
			if res, ok := c.localResultsCache[jobID]; ok {
				c.mu.RUnlock()
				return res, nil
			}
			c.mu.RUnlock()

			// Search cache directory for a matching job file.
			res, err := c.loadBlastResultFromCache(jobID)
			if err == nil {
				// store in memory cache for faster subsequent access
				c.mu.Lock()
				c.localResultsCache[jobID] = res
				c.mu.Unlock()
				return res, nil
			}

			// Respect context cancellation.
			select {
			case <-ctx.Done():
				return model.BlastResult{}, fmt.Errorf("wait for local blast results canceled: %w", ctx.Err())
			case <-time.After(pollInterval):
				// continue and re-check deadline
			}

			if time.Now().After(deadline) {
				return model.BlastResult{}, fmt.Errorf("timeout waiting for local BLAST results for job %s", jobID)
			}
		}
	}

	// Non-local jobs: server-side parsing not implemented yet.
	return model.BlastResult{}, fmt.Errorf("lemna.org BLAST result parsing is not enabled yet")
}

// RunLocalBlast executes a local BLAST workflow using available FASTA downloads.
// This dispatches to the LocalBlastRun helper defined in localblast.go which
// performs download, makeblastdb, run blast+, and caches results to disk.
func (c *Client) RunLocalBlast(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {
	// Delegate to LocalBlastRun which implements the full local BLAST workflow.
	job, err := LocalBlastRun(ctx, c, req)
	if err != nil {
		return model.BlastJob{}, err
	}
	return job, nil
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
	var found string
	// Walk cache tree to find jobID+".tsv"
	_ = filepath.Walk(cacheRoot, func(pathStr string, info os.FileInfo, err error) error {
		if err != nil {
			// ignore and continue walking
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if info.Name() == jobID+".tsv" {
			found = pathStr
			return fmt.Errorf("found") // short-circuit
		}
		return nil
	})
	if found == "" {
		return model.BlastResult{}, fmt.Errorf("no cached result file for job %s", jobID)
	}

	// parse simple TSV saved by saveBlastResultToCache
	f, err := os.Open(found)
	if err != nil {
		return model.BlastResult{}, fmt.Errorf("open cached result: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	result := model.BlastResult{
		JobID:   jobID,
		Message: "local BLAST result loaded from cache",
		Rows:    []model.BlastResultRow{},
	}
	lineNo := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineNo++
		// skip header line
		if lineNo == 1 {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 9 {
			continue
		}
		hit, _ := strconv.Atoi(fields[0])
		protein := fields[1]
		qseqid := fields[2]
		qstart, _ := strconv.Atoi(fields[3])
		qend, _ := strconv.Atoi(fields[4])
		evalue := fields[5]
		pident, _ := strconv.ParseFloat(fields[6], 64)
		alignLen, _ := strconv.Atoi(fields[7])
		bitscore, _ := strconv.ParseFloat(fields[8], 64)

		row := model.BlastResultRow{
			HitNumber:       hit,
			Protein:         protein,
			QueryID:         qseqid,
			QueryFrom:       qstart,
			QueryTo:         qend,
			EValue:          evalue,
			PercentIdentity: pident,
			AlignLength:     alignLen,
			Bitscore:        bitscore,
		}
		result.Rows = append(result.Rows, row)
	}
	if err := scanner.Err(); err != nil {
		return model.BlastResult{}, fmt.Errorf("scan cached result: %w", err)
	}
	return result, nil
}

func (c *Client) FetchGeneQuerySequence(ctx context.Context, species model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, error) {
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
				Sequence:       sequence,
				SourceDatabase: c.Name(),
				GeneID:         row.GeneIdentifier,
				TranscriptID:   row.TranscriptID,
				ProteinID:      row.SequenceID,
				OrganismShort:  species.JBrowseName,
				Annotation:     species.GenomeLabel,
			}, nil
		}
	}
	return nil, fmt.Errorf("no lemna.org gene or transcript matched %q", identifier)
}

func (c *Client) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}
	cacheKey := species.JBrowseName + "|" + strings.ToLower(keyword)
	c.mu.RLock()
	if cached, ok := c.keywordRowsCache[cacheKey]; ok {
		rows := append([]model.KeywordResultRow(nil), cached...)
		c.mu.RUnlock()
		return rows, nil
	}
	c.mu.RUnlock()
	if cached, ok := readCachedJSON[[]model.KeywordResultRow]("keyword-rows", cacheKey); ok {
		rows := append([]model.KeywordResultRow(nil), cached...)
		c.mu.Lock()
		c.keywordRowsCache[cacheKey] = append([]model.KeywordResultRow(nil), cached...)
		c.mu.Unlock()
		return rows, nil
	}

	value, err, _ := c.sf.Do("keyword-rows:"+cacheKey, func() (any, error) {
		c.mu.RLock()
		if cached, ok := c.keywordRowsCache[cacheKey]; ok {
			rows := append([]model.KeywordResultRow(nil), cached...)
			c.mu.RUnlock()
			return rows, nil
		}
		c.mu.RUnlock()
		if cached, ok := readCachedJSON[[]model.KeywordResultRow]("keyword-rows", cacheKey); ok {
			rows := append([]model.KeywordResultRow(nil), cached...)
			c.mu.Lock()
			c.keywordRowsCache[cacheKey] = append([]model.KeywordResultRow(nil), cached...)
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

		rows, err := c.searchGFFRows(ctx, release, species, keyword, 200)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.keywordRowsCache[cacheKey] = append([]model.KeywordResultRow(nil), rows...)
		c.mu.Unlock()
		writeCachedJSON("keyword-rows", cacheKey, rows)
		return rows, nil
	})
	if err != nil {
		return nil, err
	}
	return value.([]model.KeywordResultRow), nil
}

func (c *Client) FetchProteinSequence(ctx context.Context, _ int, sequenceID string) (string, error) {
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
					SearchTerm:          keyword,
					TranscriptID:        transcriptID,
					GeneIdentifier:      stripTranscriptSuffix(transcriptID),
					Genome:              species.DisplayLabel(),
					Description:         record.HumanReadableDescription,
					SequenceHeaderLabel: species.DisplayLabel(),
					SequenceID:          transcriptID,
				}
			}
			row.Description = firstNonEmpty(row.Description, record.HumanReadableDescription)
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
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "Protein-Accession\t") {
			continue
		}
		cols := strings.Split(line, "\t")
		for len(cols) < 6 {
			cols = append(cols, "")
		}
		record := ahrdRecord{
			ProteinAccession:         cols[0],
			BlastHitAccession:        cols[1],
			QualityCode:              cols[2],
			HumanReadableDescription: cols[3],
			Interpro:                 cols[4],
			GeneOntologyTerm:         cols[5],
		}
		if record.ProteinAccession != "" {
			records[record.ProteinAccession] = record
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan AHRD output: %w", err)
	}
	return nil
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
		}
		if protID != "" {
			// Map protein token(s) to transcript. Use both raw and simplified tokens.
			proteinToTranscript[protID] = transcriptID
			// also record variants: if protID contains pipe or dot, index tokens.
			if strings.Contains(protID, "|") {
				parts := strings.Split(protID, "|")
				token := parts[len(parts)-1]
				if token != "" {
					proteinToTranscript[token] = transcriptID
				}
			}
			if strings.Contains(protID, ".") {
				base := strings.Split(protID, ".")[0]
				if base != "" {
					proteinToTranscript[base] = transcriptID
				}
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
					proteinToTranscript[token] = transcriptID
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
	reader, closeFn, err := c.openMaybeGzip(ctx, release.ProteinURL)
	if err != nil {
		return "", false, err
	}
	defer closeFn()

	aliases := sequenceAliases(sequenceID)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 16*1024*1024)
	var matched bool
	var seq strings.Builder
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, ">") {
			if matched {
				return seq.String(), true, nil
			}
			matched = fastaHeaderMatches(line, aliases)
			seq.Reset()
			continue
		}
		if matched {
			seq.WriteString(line)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", false, fmt.Errorf("scan protein FASTA %s: %w", release.ProteinURL, err)
	}
	if matched && seq.Len() > 0 {
		return seq.String(), true, nil
	}
	return "", false, nil
}

func (c *Client) fetchText(ctx context.Context, requestURL string) (string, error) {
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

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return "", fmt.Errorf("create lemna.org request: %w", err)
		}
		resp, err := c.baseHTTP.Do(req)
		if err != nil {
			return "", fmt.Errorf("fetch %s: %w", requestURL, err)
		}
		defer resp.Body.Close()
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
}

func (c *Client) openMaybeGzip(ctx context.Context, requestURL string) (io.Reader, func(), error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create lemna.org data request: %w", err)
	}
	resp, err := c.baseHTTP.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch %s: %w", requestURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, nil, fmt.Errorf("fetch %s: unexpected status %s", requestURL, resp.Status)
	}
	if strings.HasSuffix(strings.ToLower(requestURL), ".gz") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			_ = resp.Body.Close()
			return nil, nil, fmt.Errorf("open gzip stream %s: %w", requestURL, err)
		}
		return gz, func() { _ = gz.Close(); _ = resp.Body.Close() }, nil
	}
	return resp.Body, func() { _ = resp.Body.Close() }, nil
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
	sequenceID := firstNonEmpty(attrs["protein_id"], transcript, id)
	description := firstNonEmpty(attrs["product"], attrs["description"], attrs["Note"], attrs["note"])

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
		SearchTerm:          searchTerm,
		TranscriptID:        transcript,
		GeneIdentifier:      firstNonEmpty(parent, id),
		Genome:              species.DisplayLabel(),
		Location:            fmt.Sprintf("%s:%s..%s %s", gff.SeqID, gff.Start, gff.End, gff.Strand),
		Aliases:             firstNonEmpty(attrs["Alias"], attrs["Dbxref"]),
		Description:         description,
		Comments:            firstNonEmpty(attrs["Note"], attrs["comment"]),
		AutoDefine:          firstNonEmpty(attrs["product"], attrs["Name"]),
		GeneReportURL:       release.ReleaseURL,
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
	values := []string{
		row.TranscriptID,
		row.GeneIdentifier,
		row.Aliases,
		row.UniProt,
		row.Description,
		row.Comments,
		row.AutoDefine,
	}
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
	} {
		if row.ExtraColumns != nil {
			values = append(values, row.ExtraColumns[key])
		}
	}
	return textValuesMatchTerms(values, terms)
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
	sequenceID = strings.TrimSpace(strings.TrimPrefix(sequenceID, ">"))
	aliases := []string{sequenceID}
	if strings.Contains(sequenceID, ".") {
		aliases = append(aliases, strings.Split(sequenceID, ".")[0])
	}
	if strings.Contains(sequenceID, ":") {
		parts := strings.Split(sequenceID, ":")
		aliases = append(aliases, parts[len(parts)-1])
	}
	return aliases
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

package phytozome

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

const (
	blastSubmitURL   = "https://phytozome-next.jgi.doe.gov/api/blast/submit/sequence"
	blastResultsBase = "https://phytozome-next.jgi.doe.gov/api/blast/results/"
)

type submitResponse struct {
	Code    int    `json:"code"`
	JobID   int    `json:"job_id"`
	Message string `json:"message"`
}

type resultsResponse struct {
	Code    int    `json:"code"`
	JobID   string `json:"job_id"`
	Message string `json:"message"`
	Data    struct {
		UserOptions string `json:"userOptions"`
		Results     string `json:"results"`
		Hash        string `json:"hash"`
		ZUID        string `json:"zuid"`
	} `json:"data"`
}

type blastOutput struct {
	Program    string             `xml:"BlastOutput_program"`
	QueryID    string             `xml:"BlastOutput_query-def"`
	QueryLen   int                `xml:"BlastOutput_query-len"`
	Iterations blastIterationsXML `xml:"BlastOutput_iterations"`
}

type blastIterationsXML struct {
	Items []blastIterationXML `xml:"Iteration"`
}

type blastIterationXML struct {
	IterNum  int          `xml:"Iteration_iter-num"`
	QueryID  string       `xml:"Iteration_query-def"`
	QueryLen int          `xml:"Iteration_query-len"`
	Hits     blastHitsXML `xml:"Iteration_hits"`
	Message  string       `xml:"Iteration_message"`
}

type blastHitsXML struct {
	Items []blastHitXML `xml:"Hit"`
}

type blastHitXML struct {
	Num       int          `xml:"Hit_num"`
	Def       string       `xml:"Hit_def"`
	Accession string       `xml:"Hit_accession"`
	Length    int          `xml:"Hit_len"`
	HSPs      blastHSPsXML `xml:"Hit_hsps"`
}

type blastHSPsXML struct {
	Items []blastHSPXML `xml:"Hsp"`
}

type blastHSPXML struct {
	Num        int     `xml:"Hsp_num"`
	Bitscore   float64 `xml:"Hsp_bit-score"`
	EValue     string  `xml:"Hsp_evalue"`
	QueryFrom  int     `xml:"Hsp_query-from"`
	QueryTo    int     `xml:"Hsp_query-to"`
	HitFrom    int     `xml:"Hsp_hit-from"`
	HitTo      int     `xml:"Hsp_hit-to"`
	QueryFrame int     `xml:"Hsp_query-frame"`
	HitFrame   int     `xml:"Hsp_hit-frame"`
	Identity   int     `xml:"Hsp_identity"`
	Positive   int     `xml:"Hsp_positive"`
	Gaps       int     `xml:"Hsp_gaps"`
	AlignLen   int     `xml:"Hsp_align-len"`
}

type geneRecord struct {
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
	Organism          geneOrganismInfo `json:"organism"`
	Transcripts       []geneTranscript `json:"transcripts"`
}

type geneOrganismInfo struct {
	TaxID             string `json:"tax_id"`
	OrganismName      string `json:"organism_name"`
	ShortName         string `json:"organism_shortname"`
	AnnotationVersion string `json:"annotation_version"`
	Proteome          int    `json:"proteome"`
}

type geneTranscript struct {
	Protein             string   `json:"protein"`
	PrimaryIdentifier   string   `json:"primaryidentifier"`
	SecondaryIdentifier string   `json:"secondaryidentifier"`
	IsPrimary           string   `json:"is_primary"`
	Uniprot             []string `json:"uniprot"`
}

type proteinSequenceResponse []struct {
	Uniquename string `json:"uniquename"`
	Name       string `json:"name"`
	Organism   string `json:"organism"`
	GenomeID   string `json:"phytozome_genome_id"`
	Residues   string `json:"residues"`
}

type esSearchResponse struct {
	Hits struct {
		Hits []struct {
			Source geneRecord `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

func (g geneRecord) PrimaryTranscript(preferredID string) (geneTranscript, error) {
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
		return geneTranscript{}, fmt.Errorf("gene record %s has no transcripts", g.PrimaryIdentifier)
	}
	return g.Transcripts[0], nil
}

func (g geneRecord) OrganismShortName() string {
	return strings.TrimSpace(g.Organism.ShortName)
}

func (g geneRecord) AnnotationVersion() string {
	return strings.TrimSpace(g.Organism.AnnotationVersion)
}

func (g geneRecord) ProteomeID() int {
	return g.Organism.Proteome
}

func (c *Client) SubmitBlast(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	fields := map[string]string{
		"targets":          strconv.Itoa(req.Species.ProteomeID),
		"targetType":       req.TargetType,
		"program":          req.Program,
		"eValue":           req.EValue,
		"comparisonMatrix": req.ComparisonMatrix,
		"wordLength":       normalizeWordLength(req.WordLength),
		"alignmentsToShow": strconv.Itoa(req.AlignmentsToShow),
		"allowGaps":        boolString(req.AllowGaps),
		"filterQuery":      boolString(req.FilterQuery),
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return model.BlastJob{}, fmt.Errorf("write blast field %s: %w", key, err)
		}
	}
	if err := writeSequenceField(writer, req.Sequence); err != nil {
		return model.BlastJob{}, err
	}
	if err := writer.Close(); err != nil {
		return model.BlastJob{}, fmt.Errorf("close multipart writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, blastSubmitURL, body)
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("create blast submit request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.baseHTTP.Do(httpReq)
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("submit blast: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("read blast submit response: %w", err)
	}

	var submitted submitResponse
	if err := json.Unmarshal(payload, &submitted); err != nil {
		return model.BlastJob{}, fmt.Errorf("decode blast submit response: %w", err)
	}
	if submitted.Code != http.StatusCreated {
		return model.BlastJob{}, fmt.Errorf("submit blast: code %d message %s", submitted.Code, submitted.Message)
	}

	return model.BlastJob{
		JobID:   strconv.Itoa(submitted.JobID),
		Message: submitted.Message,
	}, nil
}

func (c *Client) WaitForBlastResults(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, error) {
	if pollInterval <= 0 {
		pollInterval = 3 * time.Second
	}
	waitCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		result, ready, err := c.fetchBlastResult(waitCtx, jobID)
		if err != nil {
			return model.BlastResult{}, err
		}
		if ready {
			return result, nil
		}

		select {
		case <-waitCtx.Done():
			return model.BlastResult{}, fmt.Errorf("wait for blast results: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func (c *Client) fetchBlastResult(ctx context.Context, jobID string) (model.BlastResult, bool, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, blastResultsBase+jobID, nil)
	if err != nil {
		return model.BlastResult{}, false, fmt.Errorf("create blast results request: %w", err)
	}

	resp, err := c.baseHTTP.Do(httpReq)
	if err != nil {
		return model.BlastResult{}, false, fmt.Errorf("get blast results: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return model.BlastResult{}, false, fmt.Errorf("read blast results response: %w", err)
	}

	var results resultsResponse
	if err := json.Unmarshal(payload, &results); err != nil {
		return model.BlastResult{}, false, fmt.Errorf("decode blast results response: %w", err)
	}

	if results.Code == http.StatusAccepted {
		return model.BlastResult{JobID: jobID, Message: results.Message}, false, nil
	}
	if blastResultsPending(results.Code, results.Message) {
		return model.BlastResult{JobID: jobID, Message: results.Message}, false, nil
	}
	if results.Code != http.StatusOK {
		return model.BlastResult{}, false, fmt.Errorf("blast results: code %d message %s", results.Code, results.Message)
	}

	parsedRows, err := parseBlastRows(results.Data.Results)
	if err != nil {
		return model.BlastResult{}, false, err
	}

	return model.BlastResult{
		JobID:       jobID,
		Message:     results.Message,
		UserOptions: results.Data.UserOptions,
		RawXML:      results.Data.Results,
		Hash:        results.Data.Hash,
		ZUID:        results.Data.ZUID,
		Rows:        parsedRows,
	}, true, nil
}

func blastResultsPending(code int, message string) bool {
	if code == http.StatusAccepted {
		return true
	}
	if code != http.StatusInternalServerError {
		return false
	}

	message = strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(message, "incomplete") || strings.Contains(message, "couldn't find those results")
}

func parseBlastRows(rawXML string) ([]model.BlastResultRow, error) {
	var output blastOutput
	if err := xml.Unmarshal([]byte(rawXML), &output); err != nil {
		return nil, fmt.Errorf("parse blast xml: %w", err)
	}

	rows := make([]model.BlastResultRow, 0, 32)
	for _, iteration := range output.Iterations.Items {
		queryID := strings.TrimSpace(iteration.QueryID)
		if queryID == "" {
			queryID = strings.TrimSpace(output.QueryID)
		}
		queryLen := iteration.QueryLen
		if queryLen == 0 {
			queryLen = output.QueryLen
		}

		for _, hit := range iteration.Hits.Items {
			meta := parseHitDefinition(hit.Def)
			proteinID := meta.SequenceID
			if proteinID == "" {
				proteinID = hit.Accession
			}

			for _, hsp := range hit.HSPs.Items {
				row := model.BlastResultRow{
					HitNumber:       hit.Num,
					HSPNumber:       hsp.Num,
					Protein:         proteinID,
					Species:         meta.SpeciesLabel,
					EValue:          hsp.EValue,
					PercentIdentity: percentIdentity(hsp.Identity, hsp.AlignLen),
					AlignLength:     hsp.AlignLen,
					Strands:         strandText(hsp.QueryFrame, hsp.HitFrame),
					QueryID:         queryID,
					QueryFrom:       hsp.QueryFrom,
					QueryTo:         hsp.QueryTo,
					TargetFrom:      hsp.HitFrom,
					TargetTo:        hsp.HitTo,
					Bitscore:        hsp.Bitscore,
					Identical:       hsp.Identity,
					Positives:       hsp.Positive,
					Gaps:            hsp.Gaps,
					QueryLength:     queryLen,
					TargetLength:    hit.Length,
					GeneReportURL:   meta.GeneReportURL,
					JBrowseName:     meta.JBrowseName,
					TargetID:        meta.TargetID,
					SequenceID:      meta.SequenceID,
					TranscriptID:    meta.TranscriptID,
					Defline:         meta.Defline,
				}
				rows = append(rows, row)
			}
		}
	}

	return rows, nil
}

type hitMeta struct {
	SpeciesLabel  string
	JBrowseName   string
	TargetID      int
	SequenceID    string
	TranscriptID  string
	Defline       string
	GeneReportURL string
}

func parseHitDefinition(hitDef string) hitMeta {
	parts := strings.Split(hitDef, "|")
	meta := hitMeta{SpeciesLabel: hitDef, Defline: hitDef}
	if len(parts) >= 5 {
		meta.JBrowseName = parts[0]
		meta.TargetID, _ = strconv.Atoi(parts[1])
		meta.SequenceID = parts[2]
		meta.TranscriptID = parts[3]
		meta.Defline = parts[4]
		meta.SpeciesLabel = strings.ReplaceAll(parts[0], "__", ".")
		meta.GeneReportURL = fmt.Sprintf("https://phytozome-next.jgi.doe.gov/report/protein/%s/%s", parts[0], parts[2])
	}
	return meta
}

func percentIdentity(identity int, alignLen int) float64 {
	if alignLen == 0 {
		return 0
	}
	return float64(identity) * 100 / float64(alignLen)
}

func strandText(queryFrame int, hitFrame int) string {
	return frameDirection(queryFrame) + "/" + frameDirection(hitFrame)
}

func frameDirection(frame int) string {
	switch {
	case frame < 0:
		return "-"
	case frame > 0:
		return "+"
	default:
		return "0"
	}
}

func normalizeWordLength(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "default") {
		return "0"
	}
	return strings.TrimSpace(value)
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func writeSequenceField(writer *multipart.Writer, sequence string) error {
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="sequence"`)
	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("create sequence field: %w", err)
	}
	if _, err := io.Copy(part, strings.NewReader(sequence)); err != nil {
		return fmt.Errorf("write sequence field: %w", err)
	}
	return nil
}

func (c *Client) FetchGeneByProtein(ctx context.Context, proteomeID int, proteinID string) (geneRecord, error) {
	requestURL := fmt.Sprintf("https://phytozome-next.jgi.doe.gov/api/db/gene_%d?protein=%s", proteomeID, url.QueryEscape(strings.TrimSpace(proteinID)))
	return c.fetchGeneRecord(ctx, requestURL, fmt.Sprintf("protein %s in proteome %d", proteinID, proteomeID))
}

func (c *Client) FetchGeneByGeneID(ctx context.Context, proteomeID int, geneID string) (geneRecord, error) {
	requestURL := fmt.Sprintf("https://phytozome-next.jgi.doe.gov/api/db/gene_%d?gene=%s", proteomeID, url.QueryEscape(strings.TrimSpace(geneID)))
	return c.fetchGeneRecord(ctx, requestURL, fmt.Sprintf("gene %s in proteome %d", geneID, proteomeID))
}

func (c *Client) FetchGeneByTranscript(ctx context.Context, proteomeID int, transcriptID string) (geneRecord, error) {
	requestURL := fmt.Sprintf("https://phytozome-next.jgi.doe.gov/api/db/gene_%d?transcript=%s", proteomeID, url.QueryEscape(strings.TrimSpace(transcriptID)))
	return c.fetchGeneRecord(ctx, requestURL, fmt.Sprintf("transcript %s in proteome %d", transcriptID, proteomeID))
}

func (c *Client) fetchGeneRecord(ctx context.Context, requestURL string, description string) (geneRecord, error) {
	c.mu.RLock()
	if cached, ok := c.geneRecordCache[requestURL]; ok {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return geneRecord{}, fmt.Errorf("create gene request: %w", err)
	}

	resp, err := c.baseHTTP.Do(req)
	if err != nil {
		return geneRecord{}, fmt.Errorf("fetch gene record for %s: %w", description, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return geneRecord{}, fmt.Errorf("no gene record for %s", description)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return geneRecord{}, fmt.Errorf("fetch gene record for %s: status %s body %s", description, resp.Status, strings.TrimSpace(string(body)))
	}

	var gene geneRecord
	if err := json.NewDecoder(resp.Body).Decode(&gene); err != nil {
		return geneRecord{}, fmt.Errorf("decode gene response for %s: %w", description, err)
	}
	if gene.ID == "" {
		return geneRecord{}, fmt.Errorf("gene response missing _id for %s", description)
	}

	c.mu.Lock()
	c.geneRecordCache[requestURL] = gene
	c.mu.Unlock()

	return gene, nil
}

func (c *Client) FetchProteinSequence(ctx context.Context, transcriptInternalID string) (string, error) {
	transcriptInternalID = strings.TrimSpace(transcriptInternalID)
	c.mu.RLock()
	if cached, ok := c.proteinSequenceCache[transcriptInternalID]; ok {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	requestURL := "https://phytozome-next.jgi.doe.gov/api/db/sequence/protein/" + url.PathEscape(transcriptInternalID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("create protein sequence request: %w", err)
	}

	resp, err := c.baseHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch protein sequence: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return "", fmt.Errorf("no protein sequence for transcript id %s", transcriptInternalID)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("fetch protein sequence: status %s body %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var payload proteinSequenceResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode protein sequence response: %w", err)
	}
	if len(payload) == 0 || strings.TrimSpace(payload[0].Residues) == "" {
		return "", fmt.Errorf("protein sequence response empty for transcript id %s", transcriptInternalID)
	}
	residues := strings.TrimSpace(payload[0].Residues)

	c.mu.Lock()
	c.proteinSequenceCache[transcriptInternalID] = residues
	c.mu.Unlock()

	return residues, nil
}

func (c *Client) SearchGenesByKeyword(ctx context.Context, proteomeID int, keyword string, limit int) ([]geneRecord, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	body, err := json.Marshal(map[string]any{
		"size": limit,
		"query": map[string]any{
			"bool": map[string]any{
				"must": []any{
					map[string]any{
						"query_string": map[string]any{
							"query":            keyword,
							"default_operator": "AND",
							"fields": []string{
								"primaryidentifier^5",
								"transcripts.primaryidentifier^5",
								"transcripts.protein^4",
								"symbols^4",
								"synonyms^3",
								"deflines^2",
								"auto_defline^2",
								"comments",
							},
						},
					},
				},
				"filter": []any{
					map[string]any{
						"term": map[string]any{
							"organism.proteome": proteomeID,
						},
					},
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode keyword search body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://phytozome-next.jgi.doe.gov/api/essearch/gene/_search/", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create keyword search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.baseHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search genes by keyword: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("keyword search: status %s body %s", resp.Status, strings.TrimSpace(string(payload)))
	}

	var payload esSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode keyword search response: %w", err)
	}

	results := make([]geneRecord, 0, len(payload.Hits.Hits))
	for _, hit := range payload.Hits.Hits {
		if strings.TrimSpace(hit.Source.PrimaryIdentifier) == "" {
			continue
		}
		results = append(results, hit.Source)
	}
	return results, nil
}

func (c *Client) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	keyword = strings.TrimSpace(keyword)
	cacheKey := strconv.Itoa(species.ProteomeID) + "|" + keyword
	c.mu.RLock()
	if cached, ok := c.keywordRowsCache[cacheKey]; ok {
		rows := append([]model.KeywordResultRow(nil), cached...)
		c.mu.RUnlock()
		return rows, nil
	}
	c.mu.RUnlock()

	seen := make(map[string]struct{})
	genes := make([]geneRecord, 0, 8)

	addGene := func(gene geneRecord) {
		key := strings.TrimSpace(gene.PrimaryIdentifier) + "|" + strconv.Itoa(gene.ProteomeID())
		if key == "|" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		genes = append(genes, gene)
	}

	if looksLikeSpecificGeneIdentifier(keyword) {
		for _, variant := range specificIdentifierVariants(keyword) {
			if gene, err := c.FetchGeneByGeneID(ctx, species.ProteomeID, variant); err == nil {
				addGene(gene)
			}
			if gene, err := c.FetchGeneByTranscript(ctx, species.ProteomeID, variant); err == nil {
				addGene(gene)
			}
			if gene, err := c.FetchGeneByProtein(ctx, species.ProteomeID, variant); err == nil {
				addGene(gene)
			}
		}
	}

	if len(genes) == 0 {
		matches, err := c.SearchGenesByKeyword(ctx, species.ProteomeID, keyword, 20)
		if err != nil {
			return nil, err
		}
		for _, gene := range matches {
			addGene(gene)
		}
	}

	rows := make([]model.KeywordResultRow, 0, len(genes))
	for _, gene := range genes {
		row, err := buildKeywordResultRow(keyword, species, gene)
		if err != nil {
			continue
		}
		rows = append(rows, row)
	}
	c.mu.Lock()
	c.keywordRowsCache[cacheKey] = append([]model.KeywordResultRow(nil), rows...)
	c.mu.Unlock()
	return rows, nil
}

func buildKeywordResultRow(searchTerm string, species model.SpeciesCandidate, gene geneRecord) (model.KeywordResultRow, error) {
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
	uniprotValues := make([]string, 0, len(transcript.Uniprot))
	for _, value := range transcript.Uniprot {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		uniprotValues = append(uniprotValues, transcript.PrimaryIdentifier+": "+value)
	}

	return model.KeywordResultRow{
		SearchTerm:          searchTerm,
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

func formatKeywordGenome(gene geneRecord) string {
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
		parts = append(parts, "("+strings.Join(details, " · ")+")")
	}
	return strings.Join(parts, " ")
}

func formatKeywordLocation(gene geneRecord) string {
	strand := "forward"
	if strings.TrimSpace(gene.Strand) == "-1" {
		strand = "reverse"
	}
	return fmt.Sprintf("%s:%s..%s %s", strings.TrimSpace(gene.Scaffold), strings.TrimSpace(gene.Start), strings.TrimSpace(gene.End), strand)
}

func looksLikeSpecificGeneIdentifier(value string) bool {
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

	variants := make([]string, 0, 3)
	seen := make(map[string]struct{}, 3)
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

	add(value)
	add(strings.ToUpper(value))
	add(strings.ToLower(value))
	return variants
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

func copyStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, len(values))
	copy(result, values)
	return result
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

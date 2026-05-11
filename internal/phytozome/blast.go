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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/searchengine/phytozomekeyword"
)

const (
	blastSubmitURL   = "https://phytozome-next.jgi.doe.gov/api/blast/submit/sequence"
	blastResultsBase = "https://phytozome-next.jgi.doe.gov/api/blast/results/"
)

var (
	ecNumberLikeLabelPattern      = regexp.MustCompile(`^(?:EC[:\-]?)?[A-Za-z]?\d+(?:\.\d+){2,3}$`)
	arabidopsisGeneIDLabelPattern = regexp.MustCompile(`(?i)^AT[1-5MC]G\d{5}(?:\.\d+)?$`)
	lemnaGeneIDLabelPattern       = regexp.MustCompile(`(?i)^SP\d{4}D\d{3}G\d{6}(?:_T\d+)?$`)
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

type geneRecord = phytozomekeyword.GeneRecord
type geneOrganismInfo = phytozomekeyword.GeneOrganismInfo
type geneTranscript = phytozomekeyword.GeneTranscript

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
					SourceDatabase:  "phytozome",
					HitNumber:       hit.Num,
					HSPNumber:       hsp.Num,
					Protein:         proteinID,
					SubjectID:       proteinID,
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

func (c *Client) FetchUniProtAccessions(ctx context.Context, targetID int, proteinID string) ([]string, error) {
	gene, err := c.FetchGeneByProtein(ctx, targetID, proteinID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, 2)
	seen := make(map[string]struct{})
	for _, transcript := range gene.Transcripts {
		if strings.TrimSpace(proteinID) != "" && !strings.EqualFold(strings.TrimSpace(transcript.Protein), strings.TrimSpace(proteinID)) && !strings.EqualFold(strings.TrimSpace(transcript.PrimaryIdentifier), strings.TrimSpace(proteinID)) {
			continue
		}
		for _, value := range transcript.Uniprot {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if idx := strings.LastIndex(value, ":"); idx >= 0 {
				value = strings.TrimSpace(value[idx+1:])
			}
			value = strings.Trim(value, ";, ")
			if value == "" {
				continue
			}
			key := strings.ToUpper(value)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		for _, transcript := range gene.Transcripts {
			for _, value := range transcript.Uniprot {
				value = strings.TrimSpace(value)
				if value == "" {
					continue
				}
				if idx := strings.LastIndex(value, ":"); idx >= 0 {
					value = strings.TrimSpace(value[idx+1:])
				}
				value = strings.Trim(value, ";, ")
				if value == "" {
					continue
				}
				key := strings.ToUpper(value)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				out = append(out, value)
			}
		}
	}
	return out, nil
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
	if cached, ok := readCachedJSON[geneRecord]("gene-records", requestURL); ok && cached.ID != "" {
		c.mu.Lock()
		c.geneRecordCache[requestURL] = cached
		c.mu.Unlock()
		return cached, nil
	}

	value, err, _ := c.sf.Do("gene:"+requestURL, func() (any, error) {
		c.mu.RLock()
		if cached, ok := c.geneRecordCache[requestURL]; ok {
			c.mu.RUnlock()
			return cached, nil
		}
		c.mu.RUnlock()
		if cached, ok := readCachedJSON[geneRecord]("gene-records", requestURL); ok && cached.ID != "" {
			c.mu.Lock()
			c.geneRecordCache[requestURL] = cached
			c.mu.Unlock()
			return cached, nil
		}

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
		writeCachedJSON("gene-records", requestURL, gene)

		return gene, nil
	})
	if err != nil {
		return geneRecord{}, err
	}
	return value.(geneRecord), nil
}

func (c *Client) FetchProteinSequence(ctx context.Context, targetID int, sequenceID string) (string, error) {
	sequenceID = strings.TrimSpace(sequenceID)
	if sequenceID == "" {
		return "", fmt.Errorf("empty protein sequence id")
	}
	if sequence, err := c.fetchProteinSequenceByTranscript(ctx, sequenceID); err == nil {
		return sequence, nil
	}
	if targetID == 0 {
		return "", fmt.Errorf("no target proteome id available to resolve protein %s", sequenceID)
	}
	gene, err := c.FetchGeneByProtein(ctx, targetID, sequenceID)
	if err != nil {
		return "", err
	}
	return c.fetchProteinSequenceByTranscript(ctx, gene.ID)
}

func (c *Client) FetchProteinQuerySequence(ctx context.Context, species model.SpeciesCandidate, proteinID string) (*model.QuerySequenceSource, error) {
	gene, err := c.FetchGeneByProtein(ctx, species.ProteomeID, proteinID)
	if err != nil {
		return nil, err
	}
	transcript, err := gene.PrimaryTranscriptByProtein(proteinID)
	if err != nil {
		return nil, err
	}
	sequence, err := c.fetchProteinSequenceByTranscript(ctx, transcript.SecondaryIdentifier)
	if err != nil {
		return nil, err
	}
	source := &model.QuerySequenceSource{
		Sequence:          sequence,
		SourceDatabase:    c.Name(),
		SourceProteomeID:  species.ProteomeID,
		SourceJBrowseName: species.JBrowseName,
		SourceGenomeLabel: species.GenomeLabel,
		GeneID:            strings.TrimSpace(gene.PrimaryIdentifier),
		TranscriptID:      strings.TrimSpace(transcript.PrimaryIdentifier),
		ProteinID:         strings.TrimSpace(transcript.Protein),
		OrganismShort:     gene.OrganismShortName(),
		Annotation:        gene.AnnotationVersion(),
	}
	applyPhytozomeQueryLabels(source, gene)
	return source, nil
}

func (c *Client) fetchProteinSequenceByTranscript(ctx context.Context, transcriptInternalID string) (string, error) {
	transcriptInternalID = strings.TrimSpace(transcriptInternalID)
	c.mu.RLock()
	if cached, ok := c.proteinSequenceCache[transcriptInternalID]; ok {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()
	if cached, ok := readCachedText("protein-sequences", transcriptInternalID); ok && strings.TrimSpace(cached) != "" {
		c.mu.Lock()
		c.proteinSequenceCache[transcriptInternalID] = cached
		c.mu.Unlock()
		return cached, nil
	}

	value, err, _ := c.sf.Do("protein-seq:"+transcriptInternalID, func() (any, error) {
		c.mu.RLock()
		if cached, ok := c.proteinSequenceCache[transcriptInternalID]; ok {
			c.mu.RUnlock()
			return cached, nil
		}
		c.mu.RUnlock()
		if cached, ok := readCachedText("protein-sequences", transcriptInternalID); ok && strings.TrimSpace(cached) != "" {
			c.mu.Lock()
			c.proteinSequenceCache[transcriptInternalID] = cached
			c.mu.Unlock()
			return cached, nil
		}

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
		writeCachedText("protein-sequences", transcriptInternalID, residues)

		return residues, nil
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

func (c *Client) FetchGeneQuerySequence(ctx context.Context, species model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, error) {
	var gene model.QuerySequenceSource
	gene.SourceDatabase = c.Name()
	gene.SourceProteomeID = species.ProteomeID
	gene.SourceJBrowseName = species.JBrowseName
	gene.SourceGenomeLabel = species.GenomeLabel

	switch reportType {
	case "gene":
		rawGene, err := c.FetchGeneByGeneID(ctx, species.ProteomeID, identifier)
		if err != nil {
			return nil, err
		}
		applyPhytozomeQueryLabels(&gene, rawGene)
		gene.GeneID = rawGene.PrimaryIdentifier
		transcript, err := rawGene.PrimaryTranscript("")
		if err != nil {
			return nil, err
		}
		sequence, err := c.fetchProteinSequenceByTranscript(ctx, transcript.SecondaryIdentifier)
		if err != nil {
			return nil, err
		}
		gene.Sequence = sequence
		gene.TranscriptID = transcript.PrimaryIdentifier
		gene.ProteinID = transcript.Protein
		gene.OrganismShort = rawGene.OrganismShortName()
		gene.Annotation = rawGene.AnnotationVersion()
	case "transcript":
		rawGene, err := c.FetchGeneByTranscript(ctx, species.ProteomeID, identifier)
		if err != nil {
			return nil, err
		}
		applyPhytozomeQueryLabels(&gene, rawGene)
		gene.GeneID = rawGene.PrimaryIdentifier
		transcript, err := rawGene.PrimaryTranscript(identifier)
		if err != nil {
			return nil, err
		}
		sequence, err := c.fetchProteinSequenceByTranscript(ctx, transcript.SecondaryIdentifier)
		if err != nil {
			return nil, err
		}
		gene.Sequence = sequence
		gene.TranscriptID = transcript.PrimaryIdentifier
		gene.ProteinID = transcript.Protein
		gene.OrganismShort = rawGene.OrganismShortName()
		gene.Annotation = rawGene.AnnotationVersion()
	default:
		return nil, fmt.Errorf("unsupported report URL type %q", reportType)
	}
	if gene.GeneID == "" {
		gene.GeneID = identifier
	}

	return &gene, nil
}

func applyPhytozomeQueryLabels(source *model.QuerySequenceSource, gene geneRecord) {
	aliases := dedupePreserveOrder(append(phytozomekeyword.CopyStringSlice(gene.Symbols), gene.Synonyms...))
	source.Aliases = strings.Join(aliases, "; ")
	source.AutoDefine = strings.TrimSpace(gene.AutoDefline)
	source.LabelName = phytozomekeyword.BestQuerySourceLabel(source.Aliases, gene.AutoDefline)
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
	if c.keywordEngine == nil {
		c.keywordEngine = phytozomekeyword.New(c)
	}
	return c.keywordEngine.SearchKeywordRows(ctx, species, keyword)
}

func (c *Client) SearchKeywordRowsWide(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	if c.keywordEngine == nil {
		c.keywordEngine = phytozomekeyword.New(c)
	}
	return c.keywordEngine.SearchKeywordRowsWide(ctx, species, keyword)
}

func looksLikeSpecificGeneIdentifier(value string) bool {
	return phytozomekeyword.LooksLikeSpecificGeneIdentifier(value)
}

func phytozomeGeneReportKeyword(value string) (reportType string, identifier string, ok bool) {
	return phytozomekeyword.PhytozomeGeneReportKeyword(value)
}

func specificIdentifierVariants(value string) []string {
	return phytozomekeyword.SpecificIdentifierVariants(value)
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

func firstAlias(value string) string {
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ';' || r == ',' || r == '|' || r == '\t' || r == '\n' || r == '\r'
	}) {
		part = strings.TrimSpace(part)
		if part != "" {
			return part
		}
	}
	return ""
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

func bestQuerySourceLabel(aliases string, autoDefine string) string {
	return phytozomekeyword.BestQuerySourceLabel(aliases, autoDefine)
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

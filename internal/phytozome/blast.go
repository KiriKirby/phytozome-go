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
	"strconv"
	"strings"
	"time"

	"github.com/wangsychn/phytozome-batch-cli/internal/model"
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
	IterNum  int            `xml:"Iteration_iter-num"`
	QueryID  string         `xml:"Iteration_query-def"`
	QueryLen int            `xml:"Iteration_query-len"`
	Hits     blastHitsXML   `xml:"Iteration_hits"`
	Message  string         `xml:"Iteration_message"`
}

type blastHitsXML struct {
	Items []blastHitXML `xml:"Hit"`
}

type blastHitXML struct {
	Num       int           `xml:"Hit_num"`
	Def       string        `xml:"Hit_def"`
	Accession string        `xml:"Hit_accession"`
	Length    int           `xml:"Hit_len"`
	HSPs      blastHSPsXML  `xml:"Hit_hsps"`
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

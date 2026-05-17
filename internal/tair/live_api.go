package tair

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"

	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
)

type tairSearchResponse struct {
	Total int              `json:"total"`
	Docs  []tairSearchDoc  `json:"docs"`
}

type tairSearchDoc struct {
	ID               string   `json:"id"`
	GeneName         []string `json:"gene_name"`
	GeneModelIDs     []string `json:"gene_model_ids"`
	Description      []string `json:"description"`
	OtherNames       []string `json:"other_names"`
	Keywords         []string `json:"keywords"`
	KeywordTypes     []string `json:"keyword_types"`
	UniProtIDs       []string `json:"uniprot_ids"`
	EvidenceCodes    []string `json:"evidence_codes"`
	Phenotypes       []string `json:"phenotypes"`
	GeneModelType    []string `json:"gene_model_type"`
	Chromosome       string   `json:"chromosome"`
	MapType          string   `json:"map_type"`
	LocusTAIRObjectID string  `json:"locus_tairObjectId"`
	GeneTAIRObjectID string   `json:"gene_tairObjectId"`
	HasPublications  bool     `json:"has_publications"`
	IsObselete       bool     `json:"is_obselete"`
	IsSequenced      bool     `json:"is_sequenced"`
}

type tairKeywordSearchResponse struct {
	Total int                   `json:"total"`
	Docs  []tairKeywordSearchDoc `json:"docs"`
}

type tairKeywordSearchDoc struct {
	ID            string   `json:"id"`
	KwID          string   `json:"kwId"`
	KwName        []string `json:"kwName"`
	KwNameExact   string   `json:"kwNameExact"`
	KwCategory    []string `json:"kwCategory"`
	GOPOID        []string `json:"gopoId"`
	Synonyms      []string `json:"synonyms"`
	KwChildNames  []string `json:"kwChildNames"`
	LociCount     int      `json:"lociCount"`
	LociCountChild int     `json:"lociCount_child"`
	AnnotCount    int      `json:"annotCount"`
	AnnotCountChild int    `json:"annotCount_child"`
	PubCount      int      `json:"pubCount"`
	PubCountChild int      `json:"pubCount_child"`
}

func (c *Client) postTAIRJSON(ctx context.Context, endpoint string, requestBody any, out any) error {
	cacheKey := endpoint + "|" + jsonCacheKey(requestBody)
	if cached, ok := readCachedJSON[json.RawMessage]("live-json", cacheKey); ok && len(cached) > 0 {
		if err := json.Unmarshal(cached, out); err == nil {
			return nil
		}
	}
	value, err, _ := c.sf.Do("tair-live-json:"+cacheKey, func() (any, error) {
		if cached, ok := readCachedJSON[json.RawMessage]("live-json", cacheKey); ok && len(cached) > 0 {
			var tmp any = out
			if err := json.Unmarshal(cached, tmp); err == nil {
				return cached, nil
			}
		}
		payload, err := json.Marshal(requestBody)
		if err != nil {
			return nil, err
		}
		raw, err := c.postTAIRJSONRaw(ctx, endpoint, payload)
		if err != nil {
			return nil, err
		}
		writeCachedJSON("live-json", cacheKey, json.RawMessage(raw))
		return json.RawMessage(raw), nil
	})
	if err != nil {
		return err
	}
	return json.Unmarshal(value.(json.RawMessage), out)
}

func (c *Client) getTAIRText(ctx context.Context, rawURL string) (string, error) {
	cacheKey := "text|" + rawURL
	if cached, ok := readCachedJSON[string]("live-text", cacheKey); ok && strings.TrimSpace(cached) != "" {
		return cached, nil
	}
	value, err, _ := c.sf.Do("tair-live-text:"+cacheKey, func() (any, error) {
		if cached, ok := readCachedJSON[string]("live-text", cacheKey); ok && strings.TrimSpace(cached) != "" {
			return cached, nil
		}
		text, err := c.fetchTAIRText(ctx, rawURL)
		if err != nil {
			return "", err
		}
		writeCachedJSON("live-text", cacheKey, text)
		return text, nil
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

func (c *Client) postTAIRJSONRaw(ctx context.Context, endpoint string, payload []byte) ([]byte, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecManaged,
		Domain:      "www.arabidopsis.org",
		Description: "post tair json",
	}, func(runCtx context.Context) ([]byte, error) {
		url := strings.TrimRight(baseURL, "/") + endpoint
		req, err := http.NewRequestWithContext(runCtx, http.MethodPost, url, bytes.NewReader(payload))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
			resp, httpErr := c.httpClient.Do(req)
			if httpErr == nil && resp != nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					body, readErr := io.ReadAll(resp.Body)
					if readErr == nil && json.Valid(body) {
						return body, nil
					}
				}
			}
			_ = err
		}
		return c.postTAIRViaPowerShell(runCtx, url, payload)
	})
}

func (c *Client) fetchTAIRText(ctx context.Context, rawURL string) (string, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecManaged,
		Domain:      "www.arabidopsis.org",
		Description: "get tair html text",
	}, func(runCtx context.Context) (string, error) {
		req, err := http.NewRequestWithContext(runCtx, http.MethodGet, rawURL, nil)
		if err == nil {
			req.Header.Set("Accept", "text/html,application/xhtml+xml")
			resp, httpErr := c.httpClient.Do(req)
			if httpErr == nil && resp != nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					body, readErr := io.ReadAll(resp.Body)
					if readErr == nil && len(body) > 0 {
						text := string(body)
						if !looksLikeSPAHTML(text) {
							return text, nil
						}
					}
				}
			}
			_ = err
		}
		return c.getTAIRTextViaPowerShell(runCtx, rawURL)
	})
}

func (c *Client) postTAIRViaPowerShell(ctx context.Context, rawURL string, payload []byte) ([]byte, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("TAIR endpoint blocked and PowerShell fallback is only available on Windows")
	}
	encoded := base64.StdEncoding.EncodeToString(payload)
	script := strings.Join([]string{
		"$ErrorActionPreference='Stop'",
		"$body=[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('" + encoded + "'))",
		"$resp=Invoke-WebRequest -UseBasicParsing '" + psSingleQuote(rawURL) + "' -Method POST -ContentType 'application/json' -Body $body",
		"[Console]::OutputEncoding=[System.Text.Encoding]::UTF8",
		"Write-Output $resp.Content",
	}, "; ")
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Env = append(cmd.Env, "POWERSHELL_TELEMETRY_OPTOUT=1")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("PowerShell TAIR POST failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}
	return bytes.TrimSpace(out), nil
}

func (c *Client) getTAIRTextViaPowerShell(ctx context.Context, rawURL string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("TAIR endpoint blocked and PowerShell fallback is only available on Windows")
	}
	script := strings.Join([]string{
		"$ErrorActionPreference='Stop'",
		"$resp=Invoke-WebRequest -UseBasicParsing '" + psSingleQuote(rawURL) + "'",
		"[Console]::OutputEncoding=[System.Text.Encoding]::UTF8",
		"Write-Output $resp.Content",
	}, "; ")
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Env = append(cmd.Env, "POWERSHELL_TELEMETRY_OPTOUT=1")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("PowerShell TAIR GET failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

func jsonCacheKey(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("marshal-error-%T", v)
	}
	return string(data)
}

func psSingleQuote(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func looksLikeSPAHTML(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "<div id=\"app\">") && strings.Contains(lower, "/js/app.")
}

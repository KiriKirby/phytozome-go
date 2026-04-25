package lemna

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/blastplus"
	"github.com/KiriKirby/phytozome-go/internal/model"
)

// NOTE:
// This file provides a self-contained local BLAST helper implementation.
// High level flow implemented here:
// 1. Resolve release metadata and cache paths.
// 2. Download the matching nucleotide or protein FASTA from the lemna release.
// 3. Decompress FASTA if necessary.
// 4. Ensure blast+ tools are available (makeblastdb + selected program).
// 5. Build the correct BLAST DB type (nucl/prot), cached by species and release.
// 6. Run BLAST with sensible outfmt (tabular) and parse results into model.Bl​astResult.
// 7. Return model.BlastJob with an id and the parsed results (or an error).
//
// The implementation attempts to be defensive and to fail with clear errors
// so callers can present informative fallback choices to users.

// LocalBlastRun orchestrates a local BLAST execution.
// It returns a populated model.BlastJob on success or an error otherwise.
func LocalBlastRun(ctx context.Context, c *Client, req model.BlastRequest) (model.BlastJob, error) {
	// Validate
	if req.Species.JBrowseName == "" {
		return model.BlastJob{}, fmt.Errorf("missing species in BlastRequest")
	}
	if strings.TrimSpace(req.Sequence) == "" {
		return model.BlastJob{}, fmt.Errorf("empty query sequence")
	}

	blastProg, err := normalizeProgram(req.Program)
	if err != nil {
		return model.BlastJob{}, err
	}

	rel, err := c.releaseForSpecies(ctx, req.Species)
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("resolve release metadata: %w", err)
	}
	fastaURL, dbType, err := localBlastDatabase(rel, blastProg)
	if err != nil {
		return model.BlastJob{}, err
	}

	cacheDir, err := ensureCacheDir(req.Species.JBrowseName, rel.ReleaseDir)
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("ensure cache dir: %w", err)
	}

	if err := ensureBlastTools(blastProg); err != nil {
		return model.BlastJob{}, err
	}

	fastaPath, err := downloadAndPrepareFasta(ctx, c, fastaURL, cacheDir)
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("download FASTA: %w", err)
	}

	dbPrefix := filepath.Join(cacheDir, "lemna_"+dbType+"_db")
	if err := ensureBlastDBOnce(ctx, c, fastaPath, dbPrefix, dbType); err != nil {
		return model.BlastJob{}, fmt.Errorf("makeblastdb: %w", err)
	}

	fastaIdx, err := c.cachedFastaIndex(fastaPath)
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("build FASTA index: %w", err)
	}

	// Run BLAST
	result, err := runBlastAndParse(ctx, blastProg, dbPrefix, fastaIdx, req.Sequence)
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("run blast: %w", err)
	}

	// Enrich rows using GFF3-derived mappings and AHRD when available.
	// First attempt to build protein->transcript and transcript->gene maps from the GFF.
	if protToTrans, transToGene, perr := c.cachedProteinTranscriptMaps(ctx, rel); perr == nil {
		// best-effort load AHRD map
		ahrdMap := map[string]ahrdRecord{}
		if recs, aerr := c.loadAHRDRecords(ctx, rel); aerr == nil {
			ahrdMap = recs
		}
		// Use combined mappings to enrich parsed rows
		enrichBlastRowsWithMappings(rel, &result.Rows, ahrdMap, protToTrans, transToGene, fastaIdx)
	} else {
		// Fall back to AHRD-only enrichment if available.
		if ahrdMap, aerr := c.loadAHRDRecords(ctx, rel); aerr == nil && len(ahrdMap) > 0 {
			enrichBlastRowsWithAHRD(result.Rows, ahrdMap)
		}
	}

	// Construct a Job id and message
	job := model.BlastJob{
		JobID:   fmt.Sprintf("local-%s-%d", req.Species.JBrowseName, time.Now().Unix()),
		Message: "local BLAST completed",
	}

	// Attach results into the returned job via a side channel: we can't return
	// BlastResult here in the Job type, but callers of LocalBlastRun are expected
	// to call a parser or use the returned model.BlastResult (we also return it
	// via a side-effect by saving to a cache file). To be practical, we'll save
	// results to a result file in cacheDir and return job.JobID so caller can
	// subsequently load or WaitForBlastResults can pick it up.
	if err := saveBlastResultToCache(cacheDir, job.JobID, result); err != nil {
		// Non-fatal: return job with warning message
		job.Message = fmt.Sprintf("local BLAST completed; failed to cache results: %v", err)
	}
	c.mu.Lock()
	c.localResultsCache[job.JobID] = result
	c.mu.Unlock()

	// Return job (results cached)
	return job, nil
}

func localBlastDatabase(rel releaseInfo, program string) (fastaURL string, dbType string, err error) {
	switch program {
	case "blastp", "blastx":
		if rel.ProteinURL == "" {
			return "", "", fmt.Errorf("no protein FASTA available for local %s", program)
		}
		return rel.ProteinURL, "prot", nil
	case "blastn", "tblastn":
		if rel.NucleotideURL == "" {
			return "", "", fmt.Errorf("no nucleotide FASTA available for local %s", program)
		}
		return rel.NucleotideURL, "nucl", nil
	default:
		return "", "", fmt.Errorf("unsupported local BLAST program %q", program)
	}
}

// ensureCacheDir returns (and creates) a cache directory for species and release.
func ensureCacheDir(jbrowseName string, releaseDir string) (string, error) {
	return appfs.CacheDir("lemna", "localblast", jbrowseName, sanitizeFileName(releaseDir))
}

// downloadAndPrepareFasta downloads the FASTA (possibly gzipped) and ensures an
// uncompressed FASTA file path is returned.
func downloadAndPrepareFasta(ctx context.Context, c *Client, url string, cacheDir string) (string, error) {
	// derive file names
	fileName := filepath.Base(url)
	destPath := filepath.Join(cacheDir, fileName)

	// If file already exists on disk, skip download
	if _, err := os.Stat(destPath); err == nil {
		// If gz, ensure decompressed version exists
		if strings.HasSuffix(strings.ToLower(destPath), ".gz") {
			return ensureDecompressed(c, destPath)
		}
		return destPath, nil
	}

	value, err, _ := c.sf.Do("download-fasta:"+destPath, func() (any, error) {
		if _, err := os.Stat(destPath); err == nil {
			if strings.HasSuffix(strings.ToLower(destPath), ".gz") {
				return ensureDecompressed(c, destPath)
			}
			return destPath, nil
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		resp, err := c.baseHTTP.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
		}

		tmpPath := destPath + ".part"
		out, err := os.Create(tmpPath)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, resp.Body); err != nil {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			return "", err
		}
		if err := out.Close(); err != nil {
			_ = os.Remove(tmpPath)
			return "", err
		}
		if err := os.Rename(tmpPath, destPath); err != nil {
			_ = os.Remove(tmpPath)
			return "", err
		}

		if strings.HasSuffix(strings.ToLower(destPath), ".gz") {
			return ensureDecompressed(c, destPath)
		}
		return destPath, nil
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

// ensureDecompressed returns path to .fasta decompressed from gz, creating it if needed.
func ensureDecompressed(c *Client, gzPath string) (string, error) {
	// target path: remove .gz suffix
	target := strings.TrimSuffix(gzPath, ".gz")
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}

	value, err, _ := c.sf.Do("decompress-fasta:"+gzPath, func() (any, error) {
		if _, err := os.Stat(target); err == nil {
			return target, nil
		}

		gzFile, err := os.Open(gzPath)
		if err != nil {
			return "", err
		}
		defer gzFile.Close()

		gzReader, err := gzip.NewReader(gzFile)
		if err != nil {
			return "", err
		}
		defer gzReader.Close()

		tmpPath := target + ".part"
		out, err := os.Create(tmpPath)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, gzReader); err != nil {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			return "", err
		}
		if err := out.Close(); err != nil {
			_ = os.Remove(tmpPath)
			return "", err
		}
		if err := os.Rename(tmpPath, target); err != nil {
			_ = os.Remove(tmpPath)
			return "", err
		}
		return target, nil
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

// ensureBlastTools checks that makeblastdb and the requested program are available.
func ensureBlastTools(program string) error {
	return blastplus.EnsureToolsOnPath("makeblastdb", program)
}

// ensureBlastDB runs makeblastdb if the db files are not already present.
func ensureBlastDB(ctx context.Context, fastaPath string, dbPrefix string, dbType string) error {
	// Check for sentinel file (.pin) to detect built DB quickly
	// makeblastdb creates files like dbPrefix.phr/.pin/.psq for protein DB
	// We'll check for dbPrefix+".pin" or dbPrefix+".nsq" depending on type.
	if existsBlastDBFiles(dbPrefix) {
		return nil
	}

	if dbType != "prot" && dbType != "nucl" {
		return fmt.Errorf("unsupported makeblastdb dbtype %q", dbType)
	}

	cmd := exec.CommandContext(ctx, "makeblastdb", "-in", fastaPath, "-dbtype", dbType, "-parse_seqids", "-out", dbPrefix)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("makeblastdb failed: %w%s", err, formatCapturedOutput(stderr.String()))
	}
	if !existsBlastDBFiles(dbPrefix) {
		return fmt.Errorf("makeblastdb completed but DB files not found for %s", dbPrefix)
	}
	return nil
}

func ensureBlastDBOnce(ctx context.Context, c *Client, fastaPath string, dbPrefix string, dbType string) error {
	if existsBlastDBFiles(dbPrefix) {
		return nil
	}
	_, err, _ := c.sf.Do("makeblastdb:"+dbPrefix, func() (any, error) {
		if existsBlastDBFiles(dbPrefix) {
			return nil, nil
		}
		return nil, ensureBlastDB(ctx, fastaPath, dbPrefix, dbType)
	})
	return err
}

func existsBlastDBFiles(dbPrefix string) bool {
	// common extensions for protein DB
	exts := []string{".pin", ".phr", ".psq", ".nsq", ".nin", ".nhr"}
	for _, ex := range exts {
		if _, err := os.Stat(dbPrefix + ex); err == nil {
			return true
		}
	}
	return false
}

// normalizeProgram returns the executable program name for a requested program.
func normalizeProgram(requestProg string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(requestProg))
	// Accept values like "BLASTP", "blastp", "blastp (local)", "local:BLASTP"
	p = strings.TrimPrefix(p, "local:")
	p = strings.TrimSpace(strings.ReplaceAll(p, "(local)", ""))
	switch {
	case strings.Contains(p, "blastn"):
		return "blastn", nil
	case strings.Contains(p, "blastp"):
		return "blastp", nil
	case strings.Contains(p, "blastx"):
		return "blastx", nil
	case strings.Contains(p, "tblastn"):
		return "tblastn", nil
	default:
		// Fallback: choose blastp for protein-like, else blastn
		if strings.Contains(p, "protein") || strings.Contains(p, "prot") {
			return "blastp", nil
		}
		return "blastn", nil
	}
}

// runBlastAndParse runs the blast program against dbPrefix and parses tabular output.
// The function uses outfmt 6 with extended columns to capture needed stats.
func runBlastAndParse(ctx context.Context, prog string, dbPrefix string, fastaIndex map[string]fastaEntry, querySequence string) (model.BlastResult, error) {
	// Create a temp FASTA file for query
	tmpDir, err := os.MkdirTemp("", "lemna-blast-query-*")
	if err != nil {
		return model.BlastResult{}, err
	}
	defer os.RemoveAll(tmpDir)

	queryPath := filepath.Join(tmpDir, "query.fasta")
	if err := os.WriteFile(queryPath, []byte(">query\n"+querySequence+"\n"), 0o644); err != nil {
		return model.BlastResult{}, err
	}

	// Prepare output file
	outPath := filepath.Join(tmpDir, "blast.tsv")

	// outfmt fields:
	// qseqid sseqid pident length mismatch gapopen qstart qend sstart send evalue bitscore
	outfmt := "6 qseqid sseqid pident length mismatch gapopen qstart qend sstart send evalue bitscore"

	// Build command
	args := []string{"-query", queryPath, "-db", dbPrefix, "-outfmt", outfmt, "-out", outPath, "-max_target_seqs", "500", "-evalue", "1e-5"}
	cmd := exec.CommandContext(ctx, prog, args...)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return model.BlastResult{}, fmt.Errorf("%s failed: %w%s", prog, err, formatCapturedOutput(stderr.String()))
	}

	// Parse output TSV and enrich rows using the provided FASTA (deflines + lengths).
	rows, err := parseBlastTabular(outPath, fastaIndex)
	if err != nil {
		return model.BlastResult{}, err
	}

	result := model.BlastResult{
		JobID:   fmt.Sprintf("local-%d", time.Now().Unix()),
		Message: fmt.Sprintf("local %s completed; %d hits", prog, len(rows)),
		Rows:    rows,
	}
	return result, nil
}

func formatCapturedOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	lines := strings.Split(output, "\n")
	if len(lines) > 8 {
		lines = append(lines[:8], "...")
	}
	return "\n" + strings.Join(lines, "\n")
}

// parseBlastTabular parses the outfmt 6 TSV into model.BlastResultRow slice,
// and enriches rows using a FASTA index built from fastaPath when available.
func parseBlastTabular(path string, fastaIndex map[string]fastaEntry) ([]model.BlastResultRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	rows := make([]model.BlastResultRow, 0, 32)
	i := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		// Expect at least 12 fields per outfmt declaration
		if len(fields) < 12 {
			continue
		}
		i++
		// Parse numeric fields carefully
		pident, _ := strconv.ParseFloat(fields[2], 64)
		alignLen, _ := strconv.Atoi(fields[3])
		qstart, _ := strconv.Atoi(fields[6])
		qend, _ := strconv.Atoi(fields[7])
		sstart, _ := strconv.Atoi(fields[8])
		send, _ := strconv.Atoi(fields[9])
		evalue := fields[10]
		bitscore, _ := strconv.ParseFloat(fields[11], 64)

		proteinID := fields[1]
		queryID := fields[0]

		row := model.BlastResultRow{
			HitNumber:       i,
			Protein:         proteinID,
			Species:         "", // species label may be filled by caller if known
			EValue:          evalue,
			PercentIdentity: pident,
			AlignLength:     alignLen,
			Strands:         "",
			QueryID:         queryID,
			QueryFrom:       qstart,
			QueryTo:         qend,
			TargetFrom:      sstart,
			TargetTo:        send,
			Bitscore:        bitscore,
			Identical:       0,
			Positives:       0,
			Gaps:            0,
		}

		// Enrich from fastaIndex if available
		if fastaIndex != nil {
			// Try direct lookup
			if ent, ok := fastaIndex[proteinID]; ok {
				row.SequenceID = proteinID
				row.Defline = ent.Defline
				row.TargetLength = ent.Length
			} else {
				// Try token heuristics: first token or last pipe-separated field, or base before dot
				token := proteinID
				if strings.Contains(token, "|") {
					parts := strings.Split(token, "|")
					token = parts[len(parts)-1]
				}
				token = strings.Fields(token)[0]
				if ent, ok := fastaIndex[token]; ok {
					row.SequenceID = token
					row.Defline = ent.Defline
					row.TargetLength = ent.Length
				} else if strings.Contains(token, ".") {
					base := strings.Split(token, ".")[0]
					if ent, ok := fastaIndex[base]; ok {
						row.SequenceID = base
						row.Defline = ent.Defline
						row.TargetLength = ent.Length
					}
				}
			}
		}

		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

// fastaEntry holds minimal FASTA header info used to enrich BLAST rows.
type fastaEntry struct {
	Defline string
	Length  int
}

// buildFastaIndex parses the FASTA file and returns a map from header token -> fastaEntry.
func buildFastaIndex(fastaPath string) (map[string]fastaEntry, error) {
	f, err := os.Open(fastaPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024), 16*1024*1024)
	index := make(map[string]fastaEntry)
	var curHeader string
	var curLen int
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, ">") {
			// flush previous
			if curHeader != "" {
				token := headerToken(curHeader)
				index[token] = fastaEntry{Defline: curHeader, Length: curLen}
			}
			curHeader = strings.TrimPrefix(line, ">")
			curLen = 0
		} else {
			curLen += len(strings.TrimSpace(line))
		}
	}
	if curHeader != "" {
		token := headerToken(curHeader)
		index[token] = fastaEntry{Defline: curHeader, Length: curLen}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return index, nil
}

func headerToken(header string) string {
	h := strings.TrimSpace(header)
	// token heuristics: first whitespace-separated token, or last after pipe
	if fields := strings.Fields(h); len(fields) > 0 {
		token := fields[0]
		if strings.Contains(token, "|") {
			parts := strings.Split(token, "|")
			return parts[len(parts)-1]
		}
		return token
	}
	return h
}

// saveBlastResultToCache writes a serialized (simple TSV) result to cacheDir with jobID as name.
func saveBlastResultToCache(cacheDir string, jobID string, result model.BlastResult) error {
	outPath := filepath.Join(cacheDir, jobID+".tsv")
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	// header
	if _, err := w.WriteString("hit\tprotein\tqseqid\tqstart\tqend\tevalue\tpident\talign_len\tbitscore\n"); err != nil {
		return err
	}
	for i, r := range result.Rows {
		line := fmt.Sprintf("%d\t%s\t%s\t%d\t%d\t%s\t%.2f\t%d\t%.2f\n",
			i+1,
			r.Protein,
			r.QueryID,
			r.QueryFrom,
			r.QueryTo,
			r.EValue,
			r.PercentIdentity,
			r.AlignLength,
			r.Bitscore,
		)
		if _, err := w.WriteString(line); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return nil
}

// enrichBlastRowsWithAHRD attempts to map parsed BLAST rows to AHRD records and
// populate TranscriptID and Defline fields when matches are found.
// AHRD records do not provide a stable gene-report URL, so URL fields must be
// filled by release/mapping logic elsewhere rather than by annotation text.
func enrichBlastRowsWithAHRD(rows []model.BlastResultRow, ahrd map[string]ahrdRecord) {
	if rows == nil || len(rows) == 0 || len(ahrd) == 0 {
		return
	}
	for i := range rows {
		r := &rows[i]
		// Candidate keys to try: SequenceID (if set), Protein id, and token variants.
		candidates := []string{strings.TrimSpace(r.SequenceID), strings.TrimSpace(r.Protein)}
		found := false
		for _, key := range candidates {
			if key == "" {
				continue
			}
			// direct match
			if rec, ok := ahrd[key]; ok {
				// populate useful fields
				if r.TranscriptID == "" {
					r.TranscriptID = key
				}
				if r.Defline == "" {
					r.Defline = rec.HumanReadableDescription
				}
				found = true
				break
			}
			// try base token (strip version like .1)
			if strings.Contains(key, ".") {
				base := strings.Split(key, ".")[0]
				if rec, ok := ahrd[base]; ok {
					if r.TranscriptID == "" {
						r.TranscriptID = base
					}
					if r.Defline == "" {
						r.Defline = rec.HumanReadableDescription
					}
					found = true
					break
				}
			}
			// try last pipe-separated token
			if strings.Contains(key, "|") {
				parts := strings.Split(key, "|")
				token := parts[len(parts)-1]
				if rec, ok := ahrd[token]; ok {
					if r.TranscriptID == "" {
						r.TranscriptID = token
					}
					if r.Defline == "" {
						r.Defline = rec.HumanReadableDescription
					}
					found = true
					break
				}
			}
		}
		// If still not found, leave row as-is; future mapping via GFF could be attempted.
		if found {
			// nothing further for this row
		}
	}
}

// enrichBlastRowsWithMappings enriches BLAST rows using multiple sources:
//   - AHRD records (human-readable descriptions and protein accessions)
//   - GFF-derived protein->transcript and transcript->gene mappings
//   - FASTA defline index (to extract defline and sequence length)
//
// The function is best-effort: it will populate TranscriptID, SequenceID, Defline,
// TargetLength, GeneReportURL, JBrowseName, and TargetID when mappings are found.
func enrichBlastRowsWithMappings(rel releaseInfo, rows *[]model.BlastResultRow, ahrd map[string]ahrdRecord, protToTrans map[string]string, transToGene map[string]string, fastaIdx map[string]fastaEntry) {
	if rows == nil || len(*rows) == 0 {
		return
	}

	for i := range *rows {
		r := &(*rows)[i]
		if r.Species == "" {
			r.Species = rel.DisplayLabel
		}

		// Collect candidate tokens to try matching against maps.
		cands := make([]string, 0, 4)
		if r.SequenceID != "" {
			cands = append(cands, r.SequenceID)
		}
		if r.Protein != "" {
			cands = append(cands, r.Protein)
		}
		// Expand tokens: last pipe-part, base before dot, and first whitespace token.
		expanded := make([]string, 0, 8)
		for _, k := range cands {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			expanded = append(expanded, k)
			if strings.Contains(k, "|") {
				parts := strings.Split(k, "|")
				token := parts[len(parts)-1]
				expanded = append(expanded, token)
			}
			if strings.Contains(k, ".") {
				base := strings.Split(k, ".")[0]
				expanded = append(expanded, base)
			}
			fields := strings.Fields(k)
			if len(fields) > 0 {
				expanded = append(expanded, fields[0])
			}
		}

		// Try AHRD mapping first (gives human-readable description).
		found := false
		for _, tok := range expanded {
			if tok == "" {
				continue
			}
			if rec, ok := ahrd[tok]; ok {
				// Populate description/defline fields from AHRD.
				if r.Defline == "" {
					r.Defline = rec.HumanReadableDescription
				}
				if r.TranscriptID == "" {
					r.TranscriptID = tok
				}
				if r.SequenceID == "" {
					r.SequenceID = tok
				}
				if r.GeneReportURL == "" {
					r.GeneReportURL = rel.ReleaseURL
				}
				if r.JBrowseName == "" {
					r.JBrowseName = rel.RootDir
				}
				if r.TargetID == 0 {
					r.TargetID = rel.BlastNDBID
				}
				found = true
				break
			}
		}
		if found {
			continue
		}

		// Try GFF-derived protein->transcript and transcript->gene mapping.
		for _, tok := range expanded {
			if tok == "" {
				continue
			}
			if tid, ok := protToTrans[tok]; ok && tid != "" {
				// fill transcript and gene fields where possible
				if r.TranscriptID == "" {
					r.TranscriptID = tid
				}
				if gid, ok2 := transToGene[tid]; ok2 && gid != "" {
					r.GeneReportURL = rel.ReleaseURL
					// Set TargetID to release proteome id as identifier for export convenience.
					if r.TargetID == 0 {
						r.TargetID = rel.BlastNDBID
					}
					r.JBrowseName = rel.RootDir
					_ = gid // gene id is available; could be used to build more precise GeneReportURL if desired
				}
				found = true
				break
			}
		}
		if found {
			continue
		}

		// Try FASTA index enrichment (defline, length)
		if fastaIdx != nil {
			for _, tok := range expanded {
				if tok == "" {
					continue
				}
				if ent, ok := fastaIdx[tok]; ok {
					if r.SequenceID == "" {
						r.SequenceID = tok
					}
					if r.Defline == "" {
						r.Defline = ent.Defline
					}
					if r.TargetLength == 0 {
						r.TargetLength = ent.Length
					}
					if r.JBrowseName == "" {
						r.JBrowseName = rel.RootDir
					}
					if r.TargetID == 0 {
						r.TargetID = rel.BlastNDBID
					}
					found = true
					break
				}
			}
			if found {
				continue
			}
		}

		// Fallback: ensure rows have traceability to release
		if r.GeneReportURL == "" {
			r.GeneReportURL = rel.ReleaseURL
		}
		if r.JBrowseName == "" {
			r.JBrowseName = rel.RootDir
		}
		// If possible, set TargetID to proteome id for downstream export identification.
		if r.TargetID == 0 {
			r.TargetID = rel.BlastNDBID
		}
	}
}

// sanitizeFileName replaces characters unsuitable for file names.
func sanitizeFileName(s string) string {
	// minimal sanitization: replace path separators
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	return s
}

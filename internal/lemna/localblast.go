// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package lemna

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/blastplus"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/progressctx"
)

type localBlastThreadsContextKey struct{}

const maxBlastDBPrefixPathLen = 220

func WithLocalBlastThreads(ctx context.Context, threads int) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if threads < 1 {
		threads = 1
	}
	return context.WithValue(ctx, localBlastThreadsContextKey{}, threads)
}

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
	cacheKey := localBlastRequestCacheKey(req, blastProg)
	if job, ok := c.cachedLocalBlastJob(cacheKey); ok {
		return job, nil
	}

	rel, err := c.releaseForSpecies(ctx, req.Species)
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("resolve release metadata: %w", err)
	}
	fastaURL, dbType, dbKey, err := localBlastDatabase(rel, blastProg)
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
	progressctx.Report(ctx, 1, fmt.Sprintf("Preparing local %s FASTA and BLAST database...", strings.ToUpper(blastProg)))

	fastaPath, err := downloadAndPrepareFasta(ctx, c, fastaURL, cacheDir)
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("download FASTA: %w", err)
	}

	dbPrefix, err := localBlastDBPrefix(cacheDir, dbType, dbKey)
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("prepare BLAST database path: %w", err)
	}
	progressctx.Report(ctx, 60, fmt.Sprintf("Preparing local %s database...", strings.ToUpper(blastProg)))
	if err := ensureBlastDBOnce(ctx, c, fastaPath, dbPrefix, dbType); err != nil {
		return model.BlastJob{}, fmt.Errorf("makeblastdb: %w", err)
	}

	fastaIdx, err := c.cachedFastaIndex(fastaPath)
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("build FASTA index: %w", err)
	}

	progressctx.Report(ctx, 80, fmt.Sprintf("Running local %s...", strings.ToUpper(blastProg)))
	result, err := runBlastAndParse(ctx, blastProg, dbPrefix, fastaIdx, req)
	if err != nil {
		return model.BlastJob{}, fmt.Errorf("run blast: %w", err)
	}
	progressctx.Report(ctx, 90, fmt.Sprintf("Parsed local %s results with %d hits.", strings.ToUpper(blastProg), len(result.Rows)))

	if err := ctx.Err(); err != nil {
		return model.BlastJob{}, err
	}
	progressctx.Report(ctx, 92, "Enriching local BLAST hits with Lemna release metadata...")
	if protToTrans, transToGene, perr := c.cachedProteinTranscriptMaps(ctx, rel); perr == nil {
		ahrdMap := map[string]ahrdRecord{}
		progressctx.Report(ctx, 95, "Loading Lemna AHRD annotations for local BLAST hits...")
		if recs, aerr := c.loadAHRDRecords(ctx, rel); aerr == nil {
			ahrdMap = recs
		}
		progressctx.Report(ctx, 98, "Applying Lemna gene, transcript, and annotation mappings...")
		enrichBlastRowsWithMappings(rel, &result.Rows, ahrdMap, protToTrans, transToGene, fastaIdx)
	} else {
		progressctx.Report(ctx, 96, "Loading fallback Lemna annotations for local BLAST hits...")
		if ahrdMap, aerr := c.loadAHRDRecords(ctx, rel); aerr == nil && len(ahrdMap) > 0 {
			enrichBlastRowsWithAHRD(result.Rows, ahrdMap)
		}
	}
	if err := ctx.Err(); err != nil {
		return model.BlastJob{}, err
	}
	progressctx.Report(ctx, 100, fmt.Sprintf("Local %s completed with %d hits.", strings.ToUpper(blastProg), len(result.Rows)))

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
	if c.localBlastJobCache == nil {
		c.localBlastJobCache = make(map[string]model.BlastJob)
	}
	c.localBlastJobCache[cacheKey] = job
	c.localResultsCache[job.JobID] = result
	c.mu.Unlock()

	// Return job (results cached)
	return job, nil
}

func (c *Client) cachedLocalBlastJob(cacheKey string) (model.BlastJob, bool) {
	cacheKey = strings.TrimSpace(cacheKey)
	if cacheKey == "" {
		return model.BlastJob{}, false
	}
	c.mu.RLock()
	job, ok := c.localBlastJobCache[cacheKey]
	if ok {
		_, ok = c.localResultsCache[job.JobID]
	}
	c.mu.RUnlock()
	return job, ok
}

func localBlastRequestCacheKey(req model.BlastRequest, blastProg string) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(req.Species.JBrowseName)),
		strings.ToLower(strings.TrimSpace(req.Species.GenomeLabel)),
		strconv.Itoa(req.Species.ProteomeID),
		strings.ToLower(strings.TrimSpace(blastProg)),
		strings.TrimSpace(req.Sequence),
		strings.TrimSpace(req.EValue),
		strconv.Itoa(req.AlignmentsToShow),
	}
	return strings.Join(parts, "\x00")
}

func localBlastDatabase(rel releaseInfo, program string) (fastaURL string, dbType string, dbKey string, err error) {
	switch program {
	case "blastp", "blastx":
		if rel.ProteinURL == "" {
			return "", "", "", fmt.Errorf("no protein FASTA available for local %s", program)
		}
		return rel.ProteinURL, "prot", localBlastDBKey(rel.ProteinURL, program), nil
	case "blastn", "tblastn":
		fastaURL = bestLocalNucleotideURL(rel, program)
		if fastaURL == "" {
			return "", "", "", fmt.Errorf("no nucleotide FASTA available for local %s", program)
		}
		return fastaURL, "nucl", localBlastDBKey(fastaURL, program), nil
	default:
		return "", "", "", fmt.Errorf("unsupported local BLAST program %q", program)
	}
}

func localBlastDBTypeForProgram(program string) string {
	switch strings.ToLower(strings.TrimSpace(program)) {
	case "blastp", "blastx":
		return "prot"
	case "blastn", "tblastn":
		return "nucl"
	default:
		return ""
	}
}

func bestLocalNucleotideURL(rel releaseInfo, program string) string {
	bestURL := ""
	bestScore := 0
	for _, file := range rel.AvailableFiles {
		name := strings.ToLower(strings.TrimSpace(file.Name))
		if name == "" {
			continue
		}
		score := localNucleotideFileScore(name, program)
		if score <= bestScore {
			continue
		}
		bestScore = score
		bestURL = file.URL
	}
	if bestURL != "" {
		return bestURL
	}
	return strings.TrimSpace(rel.NucleotideURL)
}

func localNucleotideFileScore(name string, program string) int {
	base := nucleotideFileScore(name)
	if base == 0 {
		return 0
	}
	switch program {
	case "tblastn":
		switch {
		case strings.HasSuffix(name, ".genes.cds.primary.fasta.gz"):
			return 300
		case strings.HasSuffix(name, ".genes.filt.cds.primary.fasta.gz"):
			return 280
		case strings.HasSuffix(name, ".genes.cds.fasta.gz"):
			return 260
		case strings.HasSuffix(name, ".genes.transcripts.primary.fasta.gz"):
			return 220
		case strings.HasSuffix(name, ".genes.filt.transcripts.primary.fasta.gz"):
			return 200
		case strings.HasSuffix(name, ".genes.transcripts.fasta.gz"):
			return 180
		default:
			return 100 + base
		}
	case "blastn":
		switch {
		case strings.HasSuffix(name, ".genes.transcripts.primary.fasta.gz"):
			return 300
		case strings.HasSuffix(name, ".genes.filt.transcripts.primary.fasta.gz"):
			return 280
		case strings.HasSuffix(name, ".genes.transcripts.fasta.gz"):
			return 260
		case strings.HasSuffix(name, ".genes.cds.primary.fasta.gz"):
			return 220
		case strings.HasSuffix(name, ".genes.filt.cds.primary.fasta.gz"):
			return 200
		case strings.HasSuffix(name, ".genes.cds.fasta.gz"):
			return 180
		default:
			return 100 + base
		}
	default:
		return base
	}
}

func localBlastDBKey(fastaURL string, program string) string {
	base := strings.TrimSpace(filepath.Base(fastaURL))
	base = strings.TrimSuffix(base, ".gz")
	base = strings.TrimSuffix(base, ".fasta")
	base = strings.TrimSuffix(base, ".fa")
	base = sanitizeFileName(base)
	if base == "" {
		base = "default"
	}
	return sanitizeFileName(strings.ToLower(program) + "_" + base)
}

func compactLocalBlastDBPrefix(dbType string, dbKey string) string {
	base := sanitizeFileName(strings.TrimSpace(dbKey))
	if base == "" {
		base = "default"
	}
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(dbType)) + "\x00" + base))
	shortHash := hex.EncodeToString(sum[:6])
	if len(base) > 48 {
		base = strings.TrimRight(base[:48], "._- ")
	}
	base = strings.TrimSpace(base)
	if base == "" {
		base = "default"
	}
	return fmt.Sprintf("lemna_%s_%s_%s_db", sanitizeFileName(strings.ToLower(strings.TrimSpace(dbType))), base, shortHash)
}

func localBlastDBPrefix(cacheDir string, dbType string, dbKey string) (string, error) {
	cacheDir = strings.TrimSpace(cacheDir)
	if cacheDir == "" {
		return "", fmt.Errorf("empty BLAST database cache directory")
	}
	prefixName := compactLocalBlastDBPrefix(dbType, dbKey)
	if strings.TrimSpace(prefixName) == "" {
		return "", fmt.Errorf("empty BLAST database output name")
	}
	if strings.ContainsAny(prefixName, `/\`) {
		return "", fmt.Errorf("invalid BLAST database output name %q", prefixName)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create BLAST database cache directory: %w", err)
	}
	prefix := filepath.Join(cacheDir, prefixName)
	prefixLen := len(prefix)
	if absPrefix, err := filepath.Abs(prefix); err == nil {
		prefixLen = len(absPrefix)
	}
	if prefixLen > maxBlastDBPrefixPathLen {
		shortDir, err := shortLocalBlastDBCacheDir(cacheDir, dbType, dbKey)
		if err != nil {
			return "", err
		}
		prefix = filepath.Join(shortDir, "db")
	}
	abs, err := filepath.Abs(prefix)
	if err != nil {
		return "", fmt.Errorf("resolve BLAST database output path: %w", err)
	}
	if strings.TrimSpace(abs) == "" || filepath.Base(abs) == "." || filepath.Base(abs) == string(filepath.Separator) {
		return "", fmt.Errorf("invalid empty BLAST database output path")
	}
	return abs, nil
}

func shortLocalBlastDBCacheDir(cacheDir string, dbType string, dbKey string) (string, error) {
	sum := sha256.Sum256([]byte(filepath.Clean(cacheDir) + "\x00" + strings.ToLower(strings.TrimSpace(dbType)) + "\x00" + strings.TrimSpace(dbKey)))
	name := sanitizeFileName(strings.ToLower(strings.TrimSpace(dbType))) + "_" + hex.EncodeToString(sum[:10])
	return appfs.CacheDir("lemna", "localblastdb", name)
}

// ensureCacheDir returns (and creates) a cache directory for species and release.
func ensureCacheDir(jbrowseName string, releaseDir string) (string, error) {
	return appfs.CacheDir("lemna", "localblast", jbrowseName, sanitizeFileName(releaseDir))
}

func resetLocalBlastCache(jbrowseName string, releaseDir string) error {
	jbrowseName = strings.TrimSpace(jbrowseName)
	releaseDir = sanitizeFileName(releaseDir)
	if jbrowseName == "" && releaseDir == "" {
		_ = appfs.RemoveCacheSubtree("lemna", "localblastdb")
		return appfs.RemoveCacheSubtree("lemna", "localblast")
	}
	if jbrowseName == "" {
		_ = appfs.RemoveCacheSubtree("lemna", "localblastdb")
		return appfs.RemoveCacheSubtree("lemna", "localblast")
	}
	if releaseDir == "" {
		_ = appfs.RemoveCacheSubtree("lemna", "localblastdb")
		return appfs.RemoveCacheSubtree("lemna", "localblast", jbrowseName)
	}
	_ = appfs.RemoveCacheSubtree("lemna", "localblastdb")
	return appfs.RemoveCacheSubtree("lemna", "localblast", jbrowseName, releaseDir)
}

// downloadAndPrepareFasta downloads the FASTA (possibly gzipped) and ensures an
// uncompressed FASTA file path is returned.
func downloadAndPrepareFasta(ctx context.Context, c *Client, url string, cacheDir string) (string, error) {
	destPath, err := localFastaCachePath(cacheDir, url)
	if err != nil {
		return "", err
	}

	if path, ok, err := cachedPreparedFasta(ctx, c, destPath); err != nil {
		return "", err
	} else if ok {
		return path, nil
	}

	value, err, _ := c.sf.Do("download-fasta:"+destPath, func() (any, error) {
		if path, ok, err := cachedPreparedFasta(ctx, c, destPath); err != nil {
			return "", err
		} else if ok {
			return path, nil
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
		progressctx.Report(ctx, 1, fmt.Sprintf("Downloading FASTA: %s", filepath.Base(destPath)))
		if _, err := io.CopyBuffer(&localProgressWriter{
			ctx:     ctx,
			sink:    out,
			total:   resp.ContentLength,
			base:    1,
			span:    39,
			prefix:  fmt.Sprintf("Downloading FASTA %s", filepath.Base(destPath)),
			lastPct: -1,
		}, resp.Body, make([]byte, 1024*1024)); err != nil {
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
			path, err := ensureDecompressed(ctx, c, destPath)
			if err != nil {
				return "", err
			}
			if !fastaFileLooksUsable(path) {
				_ = os.Remove(path)
				_ = os.Remove(destPath)
				return "", fmt.Errorf("downloaded FASTA archive %s did not produce a valid FASTA file", filepath.Base(destPath))
			}
			return path, nil
		}
		if !fastaFileLooksUsable(destPath) {
			_ = os.Remove(destPath)
			return "", fmt.Errorf("downloaded FASTA %s is not a valid FASTA file", filepath.Base(destPath))
		}
		progressctx.Report(ctx, 40, fmt.Sprintf("Downloaded FASTA: %s", filepath.Base(destPath)))
		return destPath, nil
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

func cachedPreparedFasta(ctx context.Context, c *Client, destPath string) (string, bool, error) {
	info, err := os.Stat(destPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("stat cached FASTA %s: %w", destPath, err)
	}
	if info.IsDir() || info.Size() <= 0 {
		_ = os.Remove(destPath)
		return "", false, nil
	}
	if strings.HasSuffix(strings.ToLower(destPath), ".gz") {
		progressctx.Report(ctx, 10, fmt.Sprintf("Using cached FASTA archive: %s", filepath.Base(destPath)))
		path, err := ensureDecompressed(ctx, c, destPath)
		if err != nil {
			_ = os.Remove(decompressedFastaPath(destPath))
			_ = os.Remove(destPath)
			return "", false, nil
		}
		if !fastaFileLooksUsable(path) {
			_ = os.Remove(path)
			_ = os.Remove(destPath)
			return "", false, nil
		}
		return path, true, nil
	}
	if !fastaFileLooksUsable(destPath) {
		_ = os.Remove(destPath)
		return "", false, nil
	}
	progressctx.Report(ctx, 20, fmt.Sprintf("Using cached FASTA: %s", filepath.Base(destPath)))
	return destPath, true, nil
}

func localFastaCachePath(cacheDir string, rawURL string) (string, error) {
	cacheDir = strings.TrimSpace(cacheDir)
	rawURL = strings.TrimSpace(rawURL)
	if cacheDir == "" {
		return "", fmt.Errorf("empty FASTA cache directory")
	}
	if rawURL == "" {
		return "", fmt.Errorf("empty FASTA URL")
	}
	fileName := ""
	if parsed, err := url.Parse(rawURL); err == nil && parsed.Path != "" {
		fileName = pathBaseURL(parsed.Path)
	}
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = filepath.Base(rawURL)
	}
	fileName = sanitizeFileName(fileName)
	if fileName == "" || fileName == "." {
		sum := sha256.Sum256([]byte(rawURL))
		fileName = "fasta_" + hex.EncodeToString(sum[:8]) + ".fasta"
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create FASTA cache directory: %w", err)
	}
	return filepath.Join(cacheDir, fileName), nil
}

func pathBaseURL(pathValue string) string {
	pathValue = strings.TrimSpace(pathValue)
	if pathValue == "" {
		return ""
	}
	parts := strings.Split(strings.TrimRight(pathValue, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// ensureDecompressed returns path to .fasta decompressed from gz, creating it if needed.
func ensureDecompressed(ctx context.Context, c *Client, gzPath string) (string, error) {
	// target path: remove .gz suffix
	gzPath = strings.TrimSpace(gzPath)
	if gzPath == "" {
		return "", fmt.Errorf("empty gzip FASTA path")
	}
	if !strings.HasSuffix(strings.ToLower(gzPath), ".gz") {
		return gzPath, nil
	}
	target := decompressedFastaPath(gzPath)
	if target == "" || target == gzPath {
		return "", fmt.Errorf("invalid gzip FASTA target for %s", gzPath)
	}
	if info, err := os.Stat(target); err == nil && !info.IsDir() && info.Size() > 0 && fastaFileLooksUsable(target) {
		progressctx.Report(ctx, 59, fmt.Sprintf("Using cached decompressed FASTA: %s", filepath.Base(target)))
		return target, nil
	}

	value, err, _ := c.sf.Do("decompress-fasta:"+gzPath, func() (any, error) {
		if info, err := os.Stat(target); err == nil && !info.IsDir() && info.Size() > 0 && fastaFileLooksUsable(target) {
			progressctx.Report(ctx, 59, fmt.Sprintf("Using cached decompressed FASTA: %s", filepath.Base(target)))
			return target, nil
		}
		_ = os.Remove(target)

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
		_ = os.Remove(tmpPath)
		out, err := os.Create(tmpPath)
		if err != nil {
			return "", err
		}
		progressctx.Report(ctx, 41, fmt.Sprintf("Decompressing FASTA: %s", filepath.Base(gzPath)))
		sourceSize := int64(0)
		if info, err := os.Stat(gzPath); err == nil {
			sourceSize = info.Size()
		}
		if _, err := io.CopyBuffer(&localProgressWriter{
			ctx:     ctx,
			sink:    out,
			total:   sourceSize,
			base:    41,
			span:    18,
			prefix:  fmt.Sprintf("Decompressing FASTA %s", filepath.Base(gzPath)),
			lastPct: -1,
		}, gzReader, make([]byte, 1024*1024)); err != nil {
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
		if !fastaFileLooksUsable(target) {
			_ = os.Remove(target)
			return "", fmt.Errorf("decompressed FASTA %s is not a valid FASTA file", filepath.Base(target))
		}
		progressctx.Report(ctx, 59, fmt.Sprintf("Decompressed FASTA: %s", filepath.Base(target)))
		return target, nil
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

func decompressedFastaPath(gzPath string) string {
	if strings.HasSuffix(strings.ToLower(gzPath), ".gz") {
		return gzPath[:len(gzPath)-3]
	}
	return gzPath
}

func fastaFileLooksUsable(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 64*1024)
	n, err := f.Read(buf)
	if n <= 0 {
		return false
	}
	if err != nil && err != io.EOF {
		return false
	}
	for _, b := range buf[:n] {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case '>':
			return true
		default:
			return false
		}
	}
	return false
}

// ensureBlastTools checks that makeblastdb and the requested program are available.
func ensureBlastTools(program string) error {
	return blastplus.EnsureToolsOnPath("makeblastdb", program)
}

// ensureBlastDB runs makeblastdb if the db files are not already present.
func ensureBlastDB(ctx context.Context, fastaPath string, dbPrefix string, dbType string) error {
	spec, err := prepareBlastDBSpec(fastaPath, dbPrefix, dbType)
	if err != nil {
		return err
	}
	if blastDBComplete(spec.dbPrefix, spec.dbType) {
		return nil
	}

	if err := removeBlastDBFiles(spec.dbPrefix); err != nil {
		return err
	}

	tmpDir, tmpPrefix, err := temporaryBlastDBPrefix(spec.dbPrefix)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	err = runMakeBlastDB(ctx, spec.fastaPath, tmpPrefix, spec.dbType, true)
	if err != nil {
		parseSeqIDErr := err
		_ = removeBlastDBFiles(tmpPrefix)
		err = runMakeBlastDB(ctx, spec.fastaPath, tmpPrefix, spec.dbType, false)
		if err != nil {
			return fmt.Errorf("makeblastdb failed with parse_seqids: %v; retry without parse_seqids failed: %w", parseSeqIDErr, err)
		}
	}
	if !blastDBComplete(tmpPrefix, spec.dbType) {
		return fmt.Errorf("makeblastdb completed but temporary DB files not found for %s", tmpPrefix)
	}
	if err := moveBlastDBFiles(tmpPrefix, spec.dbPrefix); err != nil {
		_ = removeBlastDBFiles(spec.dbPrefix)
		return err
	}
	progressctx.Report(ctx, 79, fmt.Sprintf("Prepared BLAST database: %s", filepath.Base(spec.dbPrefix)))
	if !blastDBComplete(spec.dbPrefix, spec.dbType) {
		_ = removeBlastDBFiles(spec.dbPrefix)
		return fmt.Errorf("makeblastdb completed but DB files not found for %s", spec.dbPrefix)
	}
	return nil
}

func ensureBlastDBOnce(ctx context.Context, c *Client, fastaPath string, dbPrefix string, dbType string) error {
	spec, err := prepareBlastDBSpec(fastaPath, dbPrefix, dbType)
	if err != nil {
		return err
	}
	if blastDBComplete(spec.dbPrefix, spec.dbType) {
		return nil
	}
	_, err, _ = c.sf.Do("makeblastdb:"+spec.dbPrefix, func() (any, error) {
		if blastDBComplete(spec.dbPrefix, spec.dbType) {
			return nil, nil
		}
		return nil, ensureBlastDB(ctx, spec.fastaPath, spec.dbPrefix, spec.dbType)
	})
	return err
}

type localBlastDBSpec struct {
	fastaPath string
	dbPrefix  string
	dbType    string
}

func prepareBlastDBSpec(fastaPath string, dbPrefix string, dbType string) (localBlastDBSpec, error) {
	dbType = strings.ToLower(strings.TrimSpace(dbType))
	if dbType != "prot" && dbType != "nucl" {
		return localBlastDBSpec{}, fmt.Errorf("unsupported makeblastdb dbtype %q", dbType)
	}
	fastaPath = strings.TrimSpace(fastaPath)
	if fastaPath == "" {
		return localBlastDBSpec{}, fmt.Errorf("empty makeblastdb FASTA input path")
	}
	fastaAbs, err := filepath.Abs(filepath.Clean(fastaPath))
	if err != nil {
		return localBlastDBSpec{}, fmt.Errorf("resolve makeblastdb FASTA input path: %w", err)
	}
	info, err := os.Stat(fastaAbs)
	if err != nil {
		return localBlastDBSpec{}, fmt.Errorf("stat makeblastdb FASTA input %s: %w", fastaAbs, err)
	}
	if info.IsDir() || info.Size() <= 0 {
		return localBlastDBSpec{}, fmt.Errorf("makeblastdb FASTA input %s is not a non-empty file", fastaAbs)
	}
	dbPrefix = strings.TrimSpace(dbPrefix)
	if dbPrefix == "" {
		return localBlastDBSpec{}, fmt.Errorf("empty makeblastdb output prefix; refusing to run without -out value")
	}
	dbAbs, err := filepath.Abs(filepath.Clean(dbPrefix))
	if err != nil {
		return localBlastDBSpec{}, fmt.Errorf("resolve makeblastdb output prefix: %w", err)
	}
	base := filepath.Base(dbAbs)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return localBlastDBSpec{}, fmt.Errorf("invalid makeblastdb output prefix %q", dbPrefix)
	}
	if err := os.MkdirAll(filepath.Dir(dbAbs), 0o755); err != nil {
		return localBlastDBSpec{}, fmt.Errorf("create makeblastdb output directory: %w", err)
	}
	return localBlastDBSpec{fastaPath: fastaAbs, dbPrefix: dbAbs, dbType: dbType}, nil
}

func makeBlastDBArgs(fastaPath string, dbPrefix string, dbType string, parseSeqIDs bool) []string {
	args := []string{"-in", fastaPath, "-dbtype", dbType}
	if parseSeqIDs {
		args = append(args, "-parse_seqids")
	}
	args = append(args, "-blastdb_version", "4", "-out", dbPrefix)
	return args
}

func runMakeBlastDB(ctx context.Context, fastaPath string, dbPrefix string, dbType string, parseSeqIDs bool) error {
	args := makeBlastDBArgs(fastaPath, dbPrefix, dbType, parseSeqIDs)
	for i, arg := range args {
		if arg == "-out" && (i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "") {
			return fmt.Errorf("internal error: makeblastdb -out argument is empty")
		}
	}
	cmd := exec.CommandContext(ctx, "makeblastdb", args...)
	cmd.Env = append(os.Environ(), "BLASTDB_VERSION=4")
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w%s", err, formatCapturedOutput(stderr.String()))
	}
	return nil
}

func temporaryBlastDBPrefix(dbPrefix string) (string, string, error) {
	sum := sha256.Sum256([]byte(dbPrefix + "\x00" + strconv.FormatInt(time.Now().UnixNano(), 10)))
	dir, err := os.MkdirTemp("", "phgo-blastdb-"+hex.EncodeToString(sum[:4])+"-*")
	if err != nil {
		return "", "", fmt.Errorf("create short temporary BLAST database directory: %w", err)
	}
	return dir, filepath.Join(dir, "db"), nil
}

func moveBlastDBFiles(tmpPrefix string, finalPrefix string) error {
	if strings.TrimSpace(tmpPrefix) == "" || strings.TrimSpace(finalPrefix) == "" {
		return fmt.Errorf("empty BLAST database prefix while moving files")
	}
	if err := removeBlastDBFiles(finalPrefix); err != nil {
		return err
	}
	matches, err := filepath.Glob(tmpPrefix + ".*")
	if err != nil {
		return fmt.Errorf("scan temporary BLAST database files: %w", err)
	}
	if len(matches) == 0 {
		return fmt.Errorf("no temporary BLAST database files found for %s", tmpPrefix)
	}
	for _, match := range matches {
		suffix := strings.TrimPrefix(match, tmpPrefix)
		if suffix == "" || suffix == match {
			continue
		}
		target := finalPrefix + suffix
		if err := os.Rename(match, target); err != nil {
			if copyErr := copyFile(match, target); copyErr != nil {
				return fmt.Errorf("move BLAST database file %s to %s: rename failed: %v; copy failed: %w", match, target, err, copyErr)
			}
			_ = os.Remove(match)
		}
	}
	return nil
}

func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, copyErr := io.CopyBuffer(out, in, make([]byte, 1024*1024))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return closeErr
	}
	return nil
}

func blastDBExtensions(dbType string) []string {
	switch dbType {
	case "prot":
		return []string{".pin", ".phr", ".psq"}
	case "nucl":
		return []string{".nin", ".nhr", ".nsq"}
	default:
		return nil
	}
}

func blastDBComplete(dbPrefix string, dbType string) bool {
	dbPrefix = strings.TrimSpace(dbPrefix)
	if dbPrefix == "" {
		return false
	}
	exts := blastDBExtensions(dbType)
	if len(exts) == 0 {
		return false
	}
	for _, ex := range exts {
		info, err := os.Stat(dbPrefix + ex)
		if err != nil || info.IsDir() || info.Size() <= 0 {
			return false
		}
	}
	return true
}

func removeBlastDBFiles(dbPrefix string) error {
	dbPrefix = strings.TrimSpace(dbPrefix)
	if dbPrefix == "" {
		return fmt.Errorf("empty BLAST database prefix; refusing broad cleanup")
	}
	base := filepath.Base(filepath.Clean(dbPrefix))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return fmt.Errorf("invalid BLAST database prefix %q; refusing broad cleanup", dbPrefix)
	}
	patterns := []string{
		dbPrefix + ".*",
		dbPrefix + ".p*",
		dbPrefix + ".n*",
	}
	seen := make(map[string]struct{})
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, match := range matches {
			if _, ok := seen[match]; ok {
				continue
			}
			seen[match] = struct{}{}
			_ = os.Remove(match)
		}
	}
	return nil
}

// normalizeProgram returns the executable program name for a requested program.
func normalizeProgram(requestProg string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(requestProg))
	// Accept values like "BLASTP", "blastp", "blastp (local)", "local:BLASTP"
	p = strings.TrimPrefix(p, "local:")
	p = strings.TrimSpace(strings.ReplaceAll(p, "(local)", ""))
	switch {
	case strings.Contains(p, "tblastn"):
		return "tblastn", nil
	case strings.Contains(p, "blastx"):
		return "blastx", nil
	case strings.Contains(p, "blastp"):
		return "blastp", nil
	case strings.Contains(p, "blastn"):
		return "blastn", nil
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
func runBlastAndParse(ctx context.Context, prog string, dbPrefix string, fastaIndex map[string]fastaEntry, req model.BlastRequest) (model.BlastResult, error) {
	dbPrefix = strings.TrimSpace(dbPrefix)
	if dbPrefix == "" {
		return model.BlastResult{}, fmt.Errorf("empty BLAST database prefix")
	}
	prog, err := normalizeProgram(prog)
	if err != nil {
		return model.BlastResult{}, err
	}
	if dbType := localBlastDBTypeForProgram(prog); dbType != "" && !blastDBComplete(dbPrefix, dbType) {
		return model.BlastResult{}, fmt.Errorf("local %s database is incomplete or missing at %s; rebuild the local BLAST cache", strings.ToUpper(prog), dbPrefix)
	}
	queryFASTA, queryLengths, fallbackQueryLength, err := localBlastQueryFASTA(req.Sequence)
	if err != nil {
		return model.BlastResult{}, err
	}
	// Create a temp FASTA file for query
	tmpDir, err := os.MkdirTemp("", "lemna-blast-query-*")
	if err != nil {
		return model.BlastResult{}, err
	}
	defer os.RemoveAll(tmpDir)

	queryPath := filepath.Join(tmpDir, "query.fasta")
	if err := os.WriteFile(queryPath, []byte(queryFASTA), 0o644); err != nil {
		return model.BlastResult{}, err
	}

	// Prepare output file
	outPath := filepath.Join(tmpDir, "blast.tsv")

	// Include frames for translated programs so downstream rows can expose strand-like context.
	outfmtFields := []string{
		"qseqid", "sseqid", "pident", "length", "mismatch", "gapopen",
		"qstart", "qend", "sstart", "send", "evalue", "bitscore",
	}
	switch prog {
	case "blastx", "tblastn", "blastp":
		outfmtFields = append(outfmtFields, "positive")
	}
	switch prog {
	case "blastx", "tblastn":
		outfmtFields = append(outfmtFields, "qframe", "sframe")
	}
	outfmt := "6 " + strings.Join(outfmtFields, " ")

	// Build command
	args := []string{"-query", queryPath, "-db", dbPrefix, "-outfmt", outfmt, "-out", outPath}
	if threads := localBlastThreads(ctx); threads > 1 {
		args = append(args, "-num_threads", strconv.Itoa(threads))
	}
	if n := strings.TrimSpace(req.EValue); n != "" && n != "-1" {
		args = append(args, "-evalue", n)
	}
	if req.AlignmentsToShow > 0 {
		args = append(args, "-max_target_seqs", strconv.Itoa(req.AlignmentsToShow))
	}
	if wordSize := normalizedLocalBlastWordSize(req.WordLength); wordSize != "" {
		args = append(args, "-word_size", wordSize)
	}
	if !req.AllowGaps {
		switch prog {
		case "blastn":
			args = append(args, "-ungapped")
		case "blastp", "blastx", "tblastn":
			args = append(args, "-ungapped", "-comp_based_stats", "F")
		}
	}
	if filter := localBlastSegArg(req.FilterQuery, prog); filter != "" {
		args = append(args, "-seg", filter)
	}
	if filter := localBlastDustArg(req.FilterQuery, prog); filter != "" {
		args = append(args, "-dust", filter)
	}
	if matrix := normalizedLocalBlastMatrix(req.ComparisonMatrix, prog); matrix != "" {
		args = append(args, "-matrix", matrix)
	}
	cmd := exec.CommandContext(ctx, prog, args...)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return model.BlastResult{}, fmt.Errorf("%s failed: %w%s", prog, err, formatCapturedOutput(stderr.String()))
	}

	// Parse output TSV and enrich rows using the provided FASTA (deflines + lengths).
	rows, err := parseBlastTabular(outPath, fastaIndex, prog, queryLengths, fallbackQueryLength)
	if err != nil {
		return model.BlastResult{}, err
	}
	for i := range rows {
		if rows[i].BlastProgram == "" {
			rows[i].BlastProgram = strings.ToUpper(prog)
		}
	}

	result := model.BlastResult{
		JobID:   fmt.Sprintf("local-%d", time.Now().Unix()),
		Message: fmt.Sprintf("local %s completed; %d hits", prog, len(rows)),
		Rows:    rows,
	}
	return result, nil
}

func localBlastQueryFASTA(input string) (string, map[string]int, int, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", nil, 0, fmt.Errorf("empty BLAST query sequence")
	}
	if !strings.HasPrefix(input, ">") {
		seq := localBlastSanitizeSequence(input)
		if seq == "" {
			return "", nil, 0, fmt.Errorf("empty BLAST query sequence after cleanup")
		}
		return ">query\n" + seq + "\n", map[string]int{"query": len(seq)}, len(seq), nil
	}
	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Buffer(make([]byte, 1024), 16*1024*1024)
	var out strings.Builder
	var seq strings.Builder
	queryLengths := make(map[string]int)
	seenIDs := make(map[string]int)
	totalLen := 0
	entryCount := 0
	currentHeader := ""
	currentID := ""
	flush := func() error {
		if currentHeader == "" {
			return nil
		}
		cleanSeq := localBlastSanitizeSequence(seq.String())
		if cleanSeq == "" {
			return fmt.Errorf("FASTA query %q has no sequence", currentHeader)
		}
		entryCount++
		totalLen += len(cleanSeq)
		queryLengths[currentID] = len(cleanSeq)
		out.WriteString(">")
		out.WriteString(currentID)
		if currentHeader != "" && currentHeader != currentID {
			out.WriteString(" ")
			out.WriteString(currentHeader)
		}
		out.WriteString("\n")
		out.WriteString(cleanSeq)
		out.WriteString("\n")
		seq.Reset()
		return nil
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ">") {
			if err := flush(); err != nil {
				return "", nil, 0, err
			}
			currentHeader = sanitizeFastaHeader(strings.TrimSpace(strings.TrimPrefix(line, ">")))
			currentID = uniqueFastaQueryID(currentHeader, entryCount+1, seenIDs)
			continue
		}
		if currentHeader == "" {
			return "", nil, 0, fmt.Errorf("FASTA sequence data appears before a header")
		}
		seq.WriteString(line)
	}
	if err := scanner.Err(); err != nil {
		return "", nil, 0, err
	}
	if err := flush(); err != nil {
		return "", nil, 0, err
	}
	if entryCount == 0 || totalLen == 0 {
		return "", nil, 0, fmt.Errorf("empty BLAST query FASTA")
	}
	return out.String(), queryLengths, totalLen, nil
}

func sanitizeFastaHeader(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	header = strings.ReplaceAll(header, "\t", " ")
	header = spacePattern.ReplaceAllString(header, " ")
	return header
}

func uniqueFastaQueryID(header string, index int, seen map[string]int) string {
	id := fastaQueryID(header, index)
	key := strings.ToLower(id)
	if seen[key] == 0 {
		seen[key] = 1
		return id
	}
	seen[key]++
	return fmt.Sprintf("%s_%d", id, seen[key])
}

func fastaQueryID(header string, index int) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return fmt.Sprintf("query_%d", index)
	}
	if strings.HasPrefix(strings.ToLower(header), "phgo://") {
		main := strings.TrimSpace(strings.SplitN(header[len("phgo://"):], "\\", 2)[0])
		parts := strings.Split(main, "/")
		candidates := make([]string, 0, 2)
		if len(parts) == 3 {
			candidates = append(candidates, parts[2], parts[1])
		}
		for _, candidate := range candidates {
			if id := safeQueryID(candidate); id != "" {
				return id
			}
		}
	}
	if fields := strings.Fields(header); len(fields) > 0 {
		if id := safeQueryID(fields[0]); id != "" {
			return id
		}
	}
	if id := safeQueryID(header); id != "" {
		return id
	}
	return fmt.Sprintf("query_%d", index)
}

func safeQueryID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		valid := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if !valid {
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
			continue
		}
		b.WriteRune(r)
		lastUnderscore = r == '_'
	}
	id := strings.Trim(b.String(), "._-")
	if id == "" {
		return ""
	}
	if len(id) > 80 {
		id = strings.Trim(id[:80], "._-")
	}
	return id
}

func normalizedLocalBlastWordSize(wordLength string) string {
	wordLength = strings.TrimSpace(wordLength)
	if wordLength == "" || strings.EqualFold(wordLength, "default") {
		return ""
	}
	if _, err := strconv.Atoi(wordLength); err != nil {
		return ""
	}
	return wordLength
}

func normalizedLocalBlastMatrix(matrix string, prog string) string {
	if prog != "blastp" && prog != "blastx" && prog != "tblastn" {
		return ""
	}
	matrix = strings.TrimSpace(matrix)
	if matrix == "" || strings.EqualFold(matrix, "default") {
		return ""
	}
	return matrix
}

func localBlastSegArg(enabled bool, prog string) string {
	switch prog {
	case "blastp", "blastx", "tblastn":
		if enabled {
			return "yes"
		}
		return "no"
	default:
		return ""
	}
}

func localBlastDustArg(enabled bool, prog string) string {
	if prog != "blastn" {
		return ""
	}
	if enabled {
		return "yes"
	}
	return "no"
}

func localBlastThreads(ctx context.Context) int {
	if ctx != nil {
		if threads, ok := ctx.Value(localBlastThreadsContextKey{}).(int); ok && threads > 0 {
			return threads
		}
	}
	threads := defaultLocalBlastThreads()
	if threads < 1 {
		return 1
	}
	return threads
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

type localProgressWriter struct {
	ctx     context.Context
	sink    io.Writer
	total   int64
	written int64
	base    int
	span    int
	prefix  string
	lastPct int
}

func (w *localProgressWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := w.sink.Write(p)
	if n > 0 {
		w.written += int64(n)
		w.report()
	}
	return n, err
}

func (w *localProgressWriter) report() {
	if w.total > 0 {
		pct := int((w.written * 100) / w.total)
		if pct == w.lastPct {
			return
		}
		w.lastPct = pct
		progressctx.Report(w.ctx, w.base+(w.span*pct)/100, fmt.Sprintf("%s... %d%%", w.prefix, pct))
		return
	}
	progressctx.Report(w.ctx, w.base, fmt.Sprintf("%s... %d bytes", w.prefix, w.written))
}

// parseBlastTabular parses the outfmt 6 TSV into model.BlastResultRow slice,
// and enriches rows using a FASTA index built from fastaPath when available.
func parseBlastTabular(path string, fastaIndex map[string]fastaEntry, prog string, queryLengths map[string]int, fallbackQueryLength int) ([]model.BlastResultRow, error) {
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
		// Expect at least the base fields per outfmt declaration.
		if len(fields) < 12 {
			continue
		}
		i++
		// Parse numeric fields carefully
		pident, _ := strconv.ParseFloat(fields[2], 64)
		alignLen, _ := strconv.Atoi(fields[3])
		mismatch, _ := strconv.Atoi(fields[4])
		gapOpen, _ := strconv.Atoi(fields[5])
		qstart, _ := strconv.Atoi(fields[6])
		qend, _ := strconv.Atoi(fields[7])
		sstart, _ := strconv.Atoi(fields[8])
		send, _ := strconv.Atoi(fields[9])
		evalue := fields[10]
		bitscore, _ := strconv.ParseFloat(fields[11], 64)

		proteinID := fields[1]
		queryID := fields[0]
		queryLength := fallbackQueryLength
		if queryLengths != nil {
			if n := queryLengths[queryID]; n > 0 {
				queryLength = n
			}
		}

		row := model.BlastResultRow{
			SourceDatabase:  "lemna",
			HitNumber:       i,
			Protein:         proteinID,
			SubjectID:       proteinID,
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
			Mismatches:      mismatch,
			GapOpenings:     gapOpen,
			Identical:       int(pident * float64(alignLen) / 100),
			Positives:       0,
			Gaps:            gapOpen,
			QueryLength:     queryLength,
		}
		next := 12
		if prog == "blastx" || prog == "tblastn" || prog == "blastp" {
			if len(fields) > next {
				row.Positives, _ = strconv.Atoi(fields[next])
				next++
			}
		}
		if prog == "blastx" || prog == "tblastn" {
			qframe, sframe := 0, 0
			if len(fields) > next {
				qframe, _ = strconv.Atoi(fields[next])
				next++
			}
			if len(fields) > next {
				sframe, _ = strconv.Atoi(fields[next])
				next++
			}
			row.Strands = localBlastStrandText(qframe, sframe)
		}
		if row.AlignLength > 0 && row.QueryLength > 0 {
			row.AlignQueryLengthPercent = float64(row.AlignLength) / float64(row.QueryLength) * 100
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

func localBlastStrandText(queryFrame int, subjectFrame int) string {
	if queryFrame == 0 && subjectFrame == 0 {
		return ""
	}
	return localBlastFrameDirection(queryFrame) + "/" + localBlastFrameDirection(subjectFrame)
}

func localBlastFrameDirection(frame int) string {
	switch {
	case frame < 0:
		return "-"
	case frame > 0:
		return "+"
	default:
		return "0"
	}
}

func localBlastSanitizeSequence(sequence string) string {
	sequence = strings.TrimSpace(sequence)
	if sequence == "" {
		return ""
	}
	fields := strings.Fields(sequence)
	return strings.ToUpper(strings.Join(fields, ""))
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
	if _, err := w.WriteString("hit\tprotein\tsubject_id\tqseqid\tqstart\tqend\tsstart\tsend\tevalue\tpident\talign_len\tmismatch\tgapopen\tbitscore\ttarget_length\tsequence_id\ttranscript_id\ttarget_id\tjbrowse_name\tgene_report_url\tdefline\n"); err != nil {
		return err
	}
	for i, r := range result.Rows {
		line := fmt.Sprintf("%d\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t%s\t%.2f\t%d\t%d\t%d\t%.2f\t%d\t%s\t%s\t%d\t%s\t%s\t%s\n",
			i+1,
			r.Protein,
			r.SubjectID,
			r.QueryID,
			r.QueryFrom,
			r.QueryTo,
			r.TargetFrom,
			r.TargetTo,
			r.EValue,
			r.PercentIdentity,
			r.AlignLength,
			r.Mismatches,
			r.GapOpenings,
			r.Bitscore,
			r.TargetLength,
			r.SequenceID,
			r.TranscriptID,
			r.TargetID,
			r.JBrowseName,
			r.GeneReportURL,
			strings.ReplaceAll(r.Defline, "\t", " "),
		)
		if _, err := w.WriteString(line); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if indexPath, err := localBlastResultIndexPath(jobID); err == nil {
		_ = writeAtomically(indexPath, []byte(outPath))
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
		candidates := normalizedIdentifierCandidates(firstNonEmpty(strings.TrimSpace(r.SequenceID), strings.TrimSpace(r.Protein)))
		if len(candidates) == 0 {
			candidates = append(candidates, normalizedIdentifierCandidates(strings.TrimSpace(r.Protein))...)
		}
		for _, key := range candidates {
			if key == "" {
				continue
			}
			rec, ok := lookupAHRDRecord(ahrd, key)
			if !ok {
				continue
			}
			if r.UniProtAccession == "" {
				r.UniProtAccession = uniprotAccessionFromAHRD(rec)
			}
			if r.TranscriptID == "" {
				r.TranscriptID = key
			}
			if r.Defline == "" {
				r.Defline = rec.HumanReadableDescription
			}
			break
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
		expanded := make([]string, 0, 12)
		for _, k := range cands {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			expanded = append(expanded, normalizedIdentifierCandidates(k)...)
		}
		expanded = uniqueNormalizedStrings(expanded)

		// Try AHRD mapping first (gives human-readable description).
		for _, tok := range expanded {
			if tok == "" {
				continue
			}
			if rec, ok := lookupAHRDRecord(ahrd, tok); ok {
				if r.UniProtAccession == "" {
					r.UniProtAccession = uniprotAccessionFromAHRD(rec)
				}
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
				break
			}
		}

		// Try GFF-derived protein->transcript and transcript->gene mapping.
		gffMatched := false
		for _, tok := range expanded {
			if tok == "" {
				continue
			}
			if gid, ok := lookupNormalizedMapValue(transToGene, tok); ok && gid != "" {
				if r.TranscriptID == "" {
					r.TranscriptID = tok
				}
				if r.GeneReportURL == "" || r.GeneReportURL == rel.ReleaseURL {
					r.GeneReportURL = lemnaGeneReportURL(rel.RootDir, gid)
				}
				if r.JBrowseName == "" {
					r.JBrowseName = rel.RootDir
				}
				if r.TargetID == 0 {
					r.TargetID = rel.BlastNDBID
				}
				gffMatched = true
				break
			}
			if tid, ok := lookupNormalizedMapValue(protToTrans, tok); ok && tid != "" {
				// fill transcript and gene fields where possible
				if r.TranscriptID == "" {
					r.TranscriptID = tid
				}
				if gid, ok2 := lookupNormalizedMapValue(transToGene, tid); ok2 && gid != "" {
					if r.GeneReportURL == "" || r.GeneReportURL == rel.ReleaseURL {
						r.GeneReportURL = lemnaGeneReportURL(rel.RootDir, gid)
					}
					// Set TargetID to release proteome id as identifier for export convenience.
					if r.TargetID == 0 {
						r.TargetID = rel.BlastNDBID
					}
					if r.JBrowseName == "" {
						r.JBrowseName = rel.RootDir
					}
				}
				gffMatched = true
				break
			}
		}
		if !gffMatched && strings.TrimSpace(r.TranscriptID) != "" {
			if gid, ok := lookupNormalizedMapValue(transToGene, r.TranscriptID); ok && gid != "" {
				if r.GeneReportURL == "" || r.GeneReportURL == rel.ReleaseURL {
					r.GeneReportURL = lemnaGeneReportURL(rel.RootDir, gid)
				}
				if r.JBrowseName == "" {
					r.JBrowseName = rel.RootDir
				}
				if r.TargetID == 0 {
					r.TargetID = rel.BlastNDBID
				}
			}
		}

		// Try FASTA index enrichment (defline, length)
		fastaMatched := false
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
					fastaMatched = true
					break
				}
			}
			_ = fastaMatched
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

func lemnaGeneReportURL(rootDir string, geneID string) string {
	rootDir = strings.TrimSpace(rootDir)
	geneID = strings.TrimSpace(geneID)
	if rootDir == "" || geneID == "" {
		return ""
	}
	return fmt.Sprintf("https://www.lemna.org/report/%s/%s", rootDir, geneID)
}

func uniprotAccessionFromAHRD(record ahrdRecord) string {
	for _, value := range []string{record.BlastHitAccession, record.ProteinAccession} {
		for _, token := range strings.FieldsFunc(value, func(r rune) bool {
			return r == '|' || r == ';' || r == ',' || r == ' ' || r == '\t'
		}) {
			token = strings.Trim(token, `"'=:`)
			if looksLikeUniProtAccession(token) {
				return token
			}
		}
	}
	return ""
}

func looksLikeUniProtAccession(value string) bool {
	if len(value) < 6 || len(value) > 10 {
		return false
	}
	hasDigit := false
	for _, ch := range value {
		switch {
		case ch >= 'A' && ch <= 'Z':
		case ch >= '0' && ch <= '9':
			hasDigit = true
		default:
			return false
		}
	}
	return hasDigit
}

// sanitizeFileName replaces characters unsuitable for file names.
func sanitizeFileName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range s {
		invalid := r < 32 || strings.ContainsRune(`/\:*?"<>|`, r)
		if invalid {
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
			continue
		}
		b.WriteRune(r)
		lastUnderscore = r == '_'
	}
	out := strings.Trim(b.String(), " ._-")
	if out == "" {
		return ""
	}
	return out
}

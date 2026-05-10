package lemna

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/blastplus"
	"github.com/KiriKirby/phytozome-go/internal/model"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/cavaliergopher/grab/v3"
	"github.com/cespare/xxhash/v2"
	"github.com/gofrs/flock"
	"github.com/jszwec/csvutil"
	"github.com/klauspost/compress/gzip"
)

type localBlastThreadsContextKey struct{}

var localBlastUniqueCounter atomic.Uint64

const localBlastDBVersion = "4"

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
// 6. Run BLAST with sensible outfmt (tabular) and parse results into model.Bl鈥媋stResult.
// 7. Return model.BlastJob with an id and the parsed results (or an error).
//
// The implementation attempts to be defensive and to fail with clear errors
// so callers can present informative fallback choices to users.

// LocalBlastRun orchestrates a local BLAST execution.
// It returns a populated model.BlastJob on success or an error otherwise.
func LocalBlastRun(ctx context.Context, c *Client, req model.BlastRequest) (model.BlastJob, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		LocalSlots:  1,
		Description: "run lemna local blast",
	}, func(runCtx context.Context) (model.BlastJob, error) {
		if strings.TrimSpace(req.Sequence) == "" {
			return model.BlastJob{}, fmt.Errorf("empty query sequence")
		}
		runner, err := NewLocalBlastRunner(runCtx, c, req)
		if err != nil {
			return model.BlastJob{}, err
		}
		return runner.Run(runCtx, req)
	})
}

// LocalBlastRunner holds per-release local BLAST resources that can be reused
// across a batch of query sequences.
type LocalBlastRunner struct {
	c           *Client
	prepared    localBlastPreparedResources
	protToTrans map[string]string
	transToGene map[string]string
	ahrdMap     map[string]ahrdRecord
}

func NewLocalBlastRunner(ctx context.Context, c *Client, req model.BlastRequest) (*LocalBlastRunner, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		LocalSlots:  1,
		Description: "create lemna local blast runner",
	}, func(runCtx context.Context) (*LocalBlastRunner, error) {
		prepared, err := prepareLocalBlastResources(runCtx, c, req, false)
		if err != nil {
			return nil, err
		}
		runner := &LocalBlastRunner{
			c:        c,
			prepared: prepared,
		}
		type referenceResult struct {
			protToTrans map[string]string
			transToGene map[string]string
			ahrdMap     map[string]ahrdRecord
		}
		results := make([]referenceResult, 2)
		workers := phygoboost.Budgets().NetworkMain
		if len(results) < workers {
			workers = len(results)
		}
		if workers < 1 {
			workers = 1
		}
		spec := phygoboost.ParallelSpec{Level: phygoboost.ExecHeavy, Domain: "www.lemna.org", Workers: workers, Description: "warm local blast references"}
		_ = phygoboost.ParallelForSpec(runCtx, spec, len(results), func(ctx context.Context, i int) error {
			switch i {
			case 0:
				protToTrans, transToGene, err := c.cachedProteinTranscriptMaps(ctx, prepared.Release)
				if err == nil {
					results[i] = referenceResult{protToTrans: protToTrans, transToGene: transToGene}
				}
			case 1:
				ahrdMap, err := c.loadAHRDRecords(ctx, prepared.Release)
				if err == nil {
					results[i] = referenceResult{ahrdMap: ahrdMap}
				}
			}
			return ctx.Err()
		})
		runner.protToTrans = results[0].protToTrans
		runner.transToGene = results[0].transToGene
		runner.ahrdMap = results[1].ahrdMap
		if runner.protToTrans == nil {
			runner.protToTrans = map[string]string{}
		}
		if runner.transToGene == nil {
			runner.transToGene = map[string]string{}
		}
		if runner.ahrdMap == nil {
			runner.ahrdMap = map[string]ahrdRecord{}
		}
		return runner, nil
	})
}

func (r *LocalBlastRunner) Run(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {
	return phygoboost.RunTaskSpecValue(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		LocalSlots:  1,
		Description: "run lemna local blast runner",
	}, func(runCtx context.Context) (model.BlastJob, error) {
		if r == nil || r.c == nil {
			return model.BlastJob{}, fmt.Errorf("local BLAST runner is not initialized")
		}
		if strings.TrimSpace(req.Sequence) == "" {
			return model.BlastJob{}, fmt.Errorf("empty query sequence")
		}
		prepared := r.prepared

		result, err := runBlastAndParse(runCtx, prepared.CacheDir, prepared.Program, prepared.DBPrefix, prepared.FastaIndex, req)
		if err != nil {
			return model.BlastJob{}, fmt.Errorf("run blast: %w", err)
		}

		if len(r.protToTrans) > 0 || len(r.transToGene) > 0 {
			enrichBlastRowsWithMappings(prepared.Release, &result.Rows, r.ahrdMap, r.protToTrans, r.transToGene, prepared.FastaIndex)
		} else if len(r.ahrdMap) > 0 {
			enrichBlastRowsWithAHRD(result.Rows, r.ahrdMap)
		}

		job := model.BlastJob{
			JobID:   newLocalBlastJobID(req.Species.JBrowseName),
			Message: "local BLAST completed",
		}
		result.JobID = job.JobID

		if err := saveBlastResultToCache(runCtx, prepared.CacheDir, job.JobID, result); err != nil {
			return model.BlastJob{}, fmt.Errorf("cache local BLAST result: %w", err)
		}
		r.c.mu.Lock()
		r.c.localResultsCache[job.JobID] = result
		r.c.mu.Unlock()

		return job, nil
	})
}

// PrepareLocalBlast downloads the FASTA, builds the local BLAST database, and
// warms the FASTA index cache without running a query.
func PrepareLocalBlast(ctx context.Context, c *Client, req model.BlastRequest) error {
	return phygoboost.RunTaskSpec(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecHeavy,
		LocalSlots:  1,
		Description: "prepare lemna local blast",
	}, func(runCtx context.Context) error {
		_, err := prepareLocalBlastResources(runCtx, c, req, false)
		return err
	})
}

type localBlastPreparedResources struct {
	Release    releaseInfo
	CacheDir   string
	FastaPath  string
	DBPrefix   string
	DBType     string
	Program    string
	FastaIndex map[string]fastaEntry
}

func prepareLocalBlastResources(ctx context.Context, c *Client, req model.BlastRequest, requireSequence bool) (localBlastPreparedResources, error) {
	if req.Species.JBrowseName == "" {
		return localBlastPreparedResources{}, fmt.Errorf("missing species in BlastRequest")
	}
	if requireSequence && strings.TrimSpace(req.Sequence) == "" {
		return localBlastPreparedResources{}, fmt.Errorf("empty query sequence")
	}

	blastProg, err := normalizeProgram(req.Program)
	if err != nil {
		return localBlastPreparedResources{}, err
	}

	rel, err := c.releaseForSpecies(ctx, req.Species)
	if err != nil {
		return localBlastPreparedResources{}, fmt.Errorf("resolve release metadata: %w", err)
	}
	fastaURL, dbType, err := localBlastDatabase(rel, blastProg)
	if err != nil {
		return localBlastPreparedResources{}, err
	}

	cacheDir, err := ensureCacheDir(req.Species.JBrowseName, rel.ReleaseDir)
	if err != nil {
		return localBlastPreparedResources{}, fmt.Errorf("ensure cache dir: %w", err)
	}

	if err := ensureBlastTools(blastProg); err != nil {
		return localBlastPreparedResources{}, err
	}

	fastaPath, err := downloadAndPrepareFasta(ctx, c, fastaURL, cacheDir)
	if err != nil {
		return localBlastPreparedResources{}, fmt.Errorf("download FASTA: %w", err)
	}

	dbPrefix := localBlastDBPrefix(cacheDir, dbType)
	if err := ensureBlastDBOnce(ctx, c, fastaPath, dbPrefix, dbType); err != nil {
		return localBlastPreparedResources{}, fmt.Errorf("makeblastdb: %w", err)
	}

	fastaIdx, err := c.cachedFastaIndex(ctx, fastaPath)
	if err != nil {
		return localBlastPreparedResources{}, fmt.Errorf("build FASTA index: %w", err)
	}

	return localBlastPreparedResources{
		Release:    rel,
		CacheDir:   cacheDir,
		FastaPath:  fastaPath,
		DBPrefix:   dbPrefix,
		DBType:     dbType,
		Program:    blastProg,
		FastaIndex: fastaIdx,
	}, nil
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

func localBlastDBPrefix(cacheDir string, dbType string) string {
	return filepath.Join(cacheDir, "lemna_"+dbType+"_db_v"+localBlastDBVersion)
}

func newLocalBlastJobID(scope string) string {
	scope = sanitizeFileName(strings.TrimSpace(scope))
	if scope == "" {
		scope = "blast"
	}
	return fmt.Sprintf("local-%s-%d-%d-%d", scope, time.Now().UnixNano(), os.Getpid(), localBlastUniqueCounter.Add(1))
}

func withLocalResourceLock(ctx context.Context, dir string, name string, key string, fn func() error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	lockDir := filepath.Join(dir, ".locks")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return err
	}
	lockPath := filepath.Join(lockDir, sanitizeFileName(name)+"-"+strconv.FormatUint(xxhash.Sum64String(key), 16)+".lock")
	lock := flock.New(lockPath)
	locked, err := lock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		_ = lock.Close()
		return fmt.Errorf("lock %s: %w", name, err)
	}
	if !locked {
		_ = lock.Close()
		if err := ctx.Err(); err != nil {
			return err
		}
		return fmt.Errorf("lock %s was not acquired", name)
	}
	defer func() {
		_ = lock.Close()
	}()
	return fn()
}

func uniqueTempPath(path string, suffix string) string {
	if suffix == "" {
		suffix = ".tmp"
	}
	return fmt.Sprintf("%s.%d.%d.%d%s", path, os.Getpid(), time.Now().UnixNano(), localBlastUniqueCounter.Add(1), suffix)
}

func removeStaleTempFiles(path string) {
	_ = phygoboost.RunDisk(context.Background(), func(ctx context.Context) error {
		return removeStaleTempFilesLocked(path)
	})
}

func removeStaleTempFilesLocked(path string) error {
	_ = os.Remove(path + ".part")
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, base+".") && strings.HasSuffix(name, ".part") {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
	return nil
}

func fileExistsNonEmpty(path string) bool {
	info, err := statLocalFile(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func fileExistsNonEmptyLocked(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func regularFileExists(path string) bool {
	info, err := statLocalFile(path)
	return err == nil && info.Mode().IsRegular()
}

func regularFileExistsLocked(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func statLocalFile(path string) (os.FileInfo, error) {
	var info os.FileInfo
	err := phygoboost.RunDisk(context.Background(), func(ctx context.Context) error {
		var statErr error
		info, statErr = os.Stat(path)
		return statErr
	})
	return info, err
}

func moveTempIntoPlace(ctx context.Context, tmpPath string, destPath string, allowExisting bool) error {
	return phygoboost.RunDisk(ctx, func(ctx context.Context) error {
		return moveTempIntoPlaceLocked(ctx, tmpPath, destPath, allowExisting)
	})
}

func moveTempIntoPlaceLocked(ctx context.Context, tmpPath string, destPath string, allowExisting bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var lastErr error
	for attempt := 0; attempt < 12; attempt++ {
		if err := ctx.Err(); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		if err := os.Rename(tmpPath, destPath); err == nil {
			return nil
		} else {
			lastErr = err
			if allowExisting && fileExistsNonEmptyLocked(destPath) {
				_ = os.Remove(tmpPath)
				return nil
			}
		}
		if err := phygoboost.SleepContext(ctx, time.Duration(25*(attempt+1))*time.Millisecond); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
	}
	_ = os.Remove(tmpPath)
	return lastErr
}

func writeReadySentinel(ctx context.Context, path string) error {
	tmpPath := uniqueTempPath(path, ".tmp")
	return phygoboost.RunDisk(ctx, func(context.Context) error {
		if err := os.WriteFile(tmpPath, []byte(time.Now().Format(time.RFC3339Nano)+"\n"), 0o644); err != nil {
			return err
		}
		return moveTempIntoPlaceLocked(ctx, tmpPath, path, false)
	})
}

// downloadAndPrepareFasta downloads the FASTA (possibly gzipped) and ensures an
// uncompressed FASTA file path is returned.
func downloadAndPrepareFasta(ctx context.Context, c *Client, url string, cacheDir string) (string, error) {
	// derive file names
	fileName := filepath.Base(url)
	destPath := filepath.Join(cacheDir, fileName)

	// If file already exists on disk, skip download
	if fileExistsNonEmpty(destPath) {
		// If gz, ensure decompressed version exists
		if strings.HasSuffix(strings.ToLower(destPath), ".gz") {
			return ensureDecompressed(ctx, c, destPath)
		}
		return destPath, nil
	}

	value, err, _ := c.sf.Do("download-fasta:"+destPath, func() (any, error) {
		if fileExistsNonEmpty(destPath) {
			if strings.HasSuffix(strings.ToLower(destPath), ".gz") {
				return ensureDecompressed(ctx, c, destPath)
			}
			return destPath, nil
		}

		return downloadAndPrepareFastaLocked(ctx, c, url, cacheDir, destPath)
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

func downloadAndPrepareFastaLocked(ctx context.Context, c *Client, url string, cacheDir string, destPath string) (string, error) {
	err := withLocalResourceLock(ctx, cacheDir, "download-fasta", destPath+"|"+url, func() error {
		if fileExistsNonEmpty(destPath) {
			return nil
		}
		tmpPath := uniqueTempPath(destPath, ".part")
		if err := phygoboost.RunDisk(ctx, func(ctx context.Context) error {
			_ = os.Remove(destPath)
			return removeStaleTempFilesLocked(destPath)
		}); err != nil {
			return err
		}
		if err := downloadGrabFile(ctx, c.baseHTTP, url, tmpPath); err != nil {
			_ = phygoboost.RunDisk(context.Background(), func(ctx context.Context) error {
				return os.Remove(tmpPath)
			})
			return err
		}
		if err := moveTempIntoPlace(ctx, tmpPath, destPath, true); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if strings.HasSuffix(strings.ToLower(destPath), ".gz") {
		return ensureDecompressed(ctx, c, destPath)
	}
	return destPath, nil
}

func downloadGrabFile(ctx context.Context, httpClient *http.Client, url string, dst string) error {
	if httpClient == nil {
		httpClient = phygoboost.HTTPClient()
	}
	return phygoboost.RunTaskSpec(ctx, phygoboost.TaskSpec{Level: phygoboost.ExecHeavy, Domain: hostForRemoteFile(url), Description: "download lemna local blast file"}, func(ctx context.Context) error {
		client := grab.NewClient()
		client.HTTPClient = httpClient
		req, err := grab.NewRequest(dst, url)
		if err != nil {
			return err
		}
		req = req.WithContext(ctx)
		resp := client.Do(req)
		<-resp.Done
		if err := resp.Err(); err != nil {
			_ = os.Remove(dst)
			return err
		}
		return nil
	})
}

func hostForRemoteFile(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return parsed.Host
}

// ensureDecompressed returns path to .fasta decompressed from gz, creating it if needed.
func ensureDecompressed(ctx context.Context, c *Client, gzPath string) (string, error) {
	// target path: remove .gz suffix
	target := strings.TrimSuffix(gzPath, ".gz")
	if fileExistsNonEmpty(target) {
		return target, nil
	}

	value, err, _ := c.sf.Do("decompress-fasta:"+gzPath, func() (any, error) {
		if fileExistsNonEmpty(target) {
			return target, nil
		}

		if err := decompressFastaLocked(ctx, gzPath, target); err != nil {
			return "", err
		}
		return target, nil
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

func decompressFastaLocked(ctx context.Context, gzPath string, target string) error {
	return withLocalResourceLock(ctx, filepath.Dir(target), "decompress-fasta", gzPath+"|"+target, func() error {
		return phygoboost.RunDisk(ctx, func(ctx context.Context) error {
			if fileExistsNonEmptyLocked(target) {
				return nil
			}
			_ = os.Remove(target)
			if err := removeStaleTempFilesLocked(target); err != nil {
				return err
			}
			gzFile, err := os.Open(gzPath)
			if err != nil {
				return err
			}
			defer gzFile.Close()

			gzReader, err := gzip.NewReader(gzFile)
			if err != nil {
				return err
			}
			defer gzReader.Close()

			tmpPath := uniqueTempPath(target, ".part")
			out, err := os.Create(tmpPath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, gzReader); err != nil {
				_ = out.Close()
				_ = os.Remove(tmpPath)
				return err
			}
			if err := out.Close(); err != nil {
				_ = os.Remove(tmpPath)
				return err
			}
			return moveTempIntoPlaceLocked(ctx, tmpPath, target, true)
		})
	})
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
	if existsBlastDBFiles(dbPrefix, dbType) {
		return nil
	}

	if dbType != "prot" && dbType != "nucl" {
		return fmt.Errorf("unsupported makeblastdb dbtype %q", dbType)
	}

	removeBlastDBFiles(dbPrefix)
	var stderr bytes.Buffer
	if err := phygoboost.RunProcess(ctx, phygoboost.ProcessSpec{
		Name:   "makeblastdb",
		Args:   makeBlastDBArgs(fastaPath, dbPrefix, dbType),
		Task:   phygoboost.TaskSpec{Level: phygoboost.ExecHeavy, LocalSlots: 1, Description: "build local blast database"},
		Stdout: io.Discard,
		Stderr: &stderr,
	}); err != nil {
		removeBlastDBFiles(dbPrefix)
		return fmt.Errorf("makeblastdb failed for %s database at %s using FASTA %s: %w%s", dbType, dbPrefix, fastaPath, err, phygoboost.FormatCapturedOutput(stderr.String()))
	}
	if !hasBlastDBCoreFiles(dbPrefix, dbType) {
		removeBlastDBFiles(dbPrefix)
		return fmt.Errorf("makeblastdb completed but DB files not found for %s", dbPrefix)
	}
	if err := writeReadySentinel(ctx, blastDBReadyPath(dbPrefix)); err != nil {
		removeBlastDBFiles(dbPrefix)
		return fmt.Errorf("write makeblastdb ready marker: %w", err)
	}
	return nil
}

func makeBlastDBArgs(fastaPath string, dbPrefix string, dbType string) []string {
	return []string{
		"-in", fastaPath,
		"-dbtype", dbType,
		"-parse_seqids",
		"-blastdb_version", localBlastDBVersion,
		"-out", dbPrefix,
	}
}

func ensureBlastDBOnce(ctx context.Context, c *Client, fastaPath string, dbPrefix string, dbType string) error {
	if existsBlastDBFiles(dbPrefix, dbType) {
		return nil
	}
	_, err, _ := c.sf.Do("makeblastdb:"+dbPrefix, func() (any, error) {
		if existsBlastDBFiles(dbPrefix, dbType) {
			return nil, nil
		}
		err := withLocalResourceLock(ctx, filepath.Dir(dbPrefix), "makeblastdb", dbPrefix+"|"+dbType, func() error {
			if existsBlastDBFiles(dbPrefix, dbType) {
				return nil
			}
			return ensureBlastDB(ctx, fastaPath, dbPrefix, dbType)
		})
		return nil, err
	})
	return err
}

func existsBlastDBFiles(dbPrefix string, dbType string) bool {
	var exists bool
	err := phygoboost.RunDisk(context.Background(), func(ctx context.Context) error {
		exists = existsBlastDBFilesLocked(dbPrefix, dbType)
		return nil
	})
	return err == nil && exists
}

func existsBlastDBFilesLocked(dbPrefix string, dbType string) bool {
	return regularFileExistsLocked(blastDBReadyPath(dbPrefix)) && hasBlastDBCoreFilesLocked(dbPrefix, dbType)
}

func hasBlastDBCoreFiles(dbPrefix string, dbType string) bool {
	var exists bool
	err := phygoboost.RunDisk(context.Background(), func(ctx context.Context) error {
		exists = hasBlastDBCoreFilesLocked(dbPrefix, dbType)
		return nil
	})
	return err == nil && exists
}

func hasBlastDBCoreFilesLocked(dbPrefix string, dbType string) bool {
	var extGroups [][]string
	switch dbType {
	case "prot":
		extGroups = [][]string{
			{".pin", ".phr", ".psq"},
			{".pal"},
		}
	case "nucl":
		extGroups = [][]string{
			{".nin", ".nhr", ".nsq"},
			{".nal"},
		}
	default:
		return false
	}
	for _, exts := range extGroups {
		ready := true
		for _, ex := range exts {
			if !regularFileExistsLocked(dbPrefix + ex) {
				ready = false
				break
			}
		}
		if ready {
			return true
		}
	}
	return false
}

func blastDBReadyPath(dbPrefix string) string {
	return dbPrefix + ".ready"
}

func removeBlastDBFiles(dbPrefix string) {
	matches, err := filepath.Glob(dbPrefix + ".*")
	if err != nil {
		return
	}
	for _, match := range matches {
		_ = os.Remove(match)
	}
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
func runBlastAndParse(ctx context.Context, cacheDir string, prog string, dbPrefix string, fastaIndex map[string]fastaEntry, req model.BlastRequest) (model.BlastResult, error) {
	queryPath, cleanupQuery, err := writeBlastQueryFile(ctx, cacheDir, req.Sequence)
	if err != nil {
		return model.BlastResult{}, err
	}
	defer cleanupQuery()

	outfmt := "6 qseqid sseqid pident length mismatch gapopen qstart qend sstart send evalue bitscore"
	args := []string{"-query", queryPath, "-db", dbPrefix, "-outfmt", outfmt}
	if threads := localBlastThreads(ctx); threads > 1 {
		args = append(args, "-num_threads", strconv.Itoa(threads))
	}
	if n := strings.TrimSpace(req.EValue); n != "" && n != "-1" {
		args = append(args, "-evalue", n)
	}
	if req.AlignmentsToShow > 0 {
		args = append(args, "-max_target_seqs", strconv.Itoa(req.AlignmentsToShow))
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := phygoboost.RunProcess(ctx, phygoboost.ProcessSpec{
		Name:   prog,
		Args:   args,
		Task:   phygoboost.TaskSpec{Level: phygoboost.ExecHeavy, LocalSlots: 1, Description: "run local blast process"},
		Stdout: &stdout,
		Stderr: &stderr,
	}); err != nil {
		return model.BlastResult{}, fmt.Errorf("%s failed: %w%s", prog, err, phygoboost.FormatCapturedOutput(stderr.String()))
	}

	rows, err := parseBlastTabularBuffer(stdout.Bytes(), fastaIndex)
	if err != nil {
		return model.BlastResult{}, err
	}

	result := model.BlastResult{
		JobID:   newLocalBlastJobID("blast"),
		Message: fmt.Sprintf("local %s completed; %d hits", prog, len(rows)),
		Rows:    rows,
	}
	return result, nil
}

func writeBlastQueryFile(ctx context.Context, cacheDir string, sequence string) (string, func(), error) {
	queryPath := localBlastQueryCachePath(cacheDir, sequence)
	if fileExistsNonEmpty(queryPath) {
		return queryPath, func() {}, nil
	}
	err := withLocalResourceLock(ctx, cacheDir, "blast-query", queryPath, func() error {
		if fileExistsNonEmpty(queryPath) {
			return nil
		}
		tmpPath := uniqueTempPath(queryPath, ".part")
		if err := phygoboost.RunDisk(ctx, func(ctx context.Context) error {
			return os.WriteFile(tmpPath, []byte(">query\n"+sequence+"\n"), 0o644)
		}); err != nil {
			_ = phygoboost.RunDisk(context.Background(), func(ctx context.Context) error {
				return os.Remove(tmpPath)
			})
			return err
		}
		return moveTempIntoPlace(ctx, tmpPath, queryPath, true)
	})
	if err != nil {
		return "", func() {}, err
	}
	return queryPath, func() {}, nil
}

func localBlastQueryCachePath(cacheDir string, sequence string) string {
	normalized := normalizeLocalBlastQuerySequence(sequence)
	if normalized == "" {
		normalized = strings.TrimSpace(sequence)
	}
	hash := xxhash.Sum64String(normalized)
	return filepath.Join(cacheDir, fmt.Sprintf("query-%016x.fasta", hash))
}

func normalizeLocalBlastQuerySequence(sequence string) string {
	sequence = strings.TrimSpace(sequence)
	if sequence == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(sequence))
	for _, r := range sequence {
		switch r {
		case '\r', '\n', '\t', ' ':
			continue
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func parseBlastTabularBuffer(data []byte, fastaIndex map[string]fastaEntry) ([]model.BlastResultRow, error) {
	if len(data) == 0 {
		return nil, nil
	}
	return parseBlastTabularReader(bytes.NewReader(data), "stdout", fastaIndex)
}

func parseBlastTabularReader(r io.Reader, source string, fastaIndex map[string]fastaEntry) ([]model.BlastResultRow, error) {
	reader := csv.NewReader(r)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	decoder, err := csvutil.NewDecoder(reader, "qseqid", "sseqid", "pident", "length", "mismatch", "gapopen", "qstart", "qend", "sstart", "send", "evalue", "bitscore")
	if err != nil {
		return nil, fmt.Errorf("open BLAST TSV decoder: %w", err)
	}
	decoder.AlignRecord = true

	rows := make([]model.BlastResultRow, 0, 32)
	i := 0
	for {
		var record blastTabularRecord
		if err := decoder.Decode(&record); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode BLAST TSV %s: %w", source, err)
		}
		i++
		pident, _ := strconv.ParseFloat(record.PercentIdentity, 64)
		alignLen, _ := strconv.Atoi(record.AlignLength)
		mismatch, _ := strconv.Atoi(record.Mismatches)
		gapOpen, _ := strconv.Atoi(record.GapOpenings)
		qstart, _ := strconv.Atoi(record.QueryFrom)
		qend, _ := strconv.Atoi(record.QueryTo)
		sstart, _ := strconv.Atoi(record.TargetFrom)
		send, _ := strconv.Atoi(record.TargetTo)
		bitscore, _ := strconv.ParseFloat(record.Bitscore, 64)

		proteinID := strings.TrimSpace(record.SubjectID)
		queryID := strings.TrimSpace(record.QueryID)
		if proteinID == "" || queryID == "" {
			continue
		}

		row := model.BlastResultRow{
			SourceDatabase:  "lemna",
			HitNumber:       i,
			Protein:         proteinID,
			SubjectID:       proteinID,
			Species:         "",
			EValue:          record.EValue,
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
		}

		if fastaIndex != nil {
			if ent, ok := fastaIndex[proteinID]; ok {
				row.SequenceID = proteinID
				row.Defline = ent.Defline
				row.TargetLength = ent.Length
			} else {
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
	return rows, nil
}

func localBlastThreads(ctx context.Context) int {
	if ctx != nil {
		if threads, ok := ctx.Value(localBlastThreadsContextKey{}).(int); ok && threads > 0 {
			return threads
		}
	}
	threads := phygoboost.Budgets().WorkerGOMAXPROCS
	if threads < 1 {
		return 1
	}
	return threads
}

// parseBlastTabular parses the outfmt 6 TSV into model.BlastResultRow slice,
// and enriches rows using a FASTA index built from fastaPath when available.
func parseBlastTabular(ctx context.Context, path string, fastaIndex map[string]fastaEntry) ([]model.BlastResultRow, error) {
	var rows []model.BlastResultRow
	err := phygoboost.RunDisk(ctx, func(ctx context.Context) error {
		var err error
		rows, err = parseBlastTabularLocked(path, fastaIndex)
		return err
	})
	return rows, err
}

func parseBlastTabularLocked(path string, fastaIndex map[string]fastaEntry) ([]model.BlastResultRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseBlastTabularReader(f, path, fastaIndex)
}

type blastTabularRecord struct {
	QueryID         string `csv:"qseqid"`
	SubjectID       string `csv:"sseqid"`
	PercentIdentity string `csv:"pident"`
	AlignLength     string `csv:"length"`
	Mismatches      string `csv:"mismatch"`
	GapOpenings     string `csv:"gapopen"`
	QueryFrom       string `csv:"qstart"`
	QueryTo         string `csv:"qend"`
	TargetFrom      string `csv:"sstart"`
	TargetTo        string `csv:"send"`
	EValue          string `csv:"evalue"`
	Bitscore        string `csv:"bitscore"`
}

// fastaEntry holds minimal FASTA header info used to enrich BLAST rows.
type fastaEntry struct {
	Defline string
	Length  int
}

// buildFastaIndex parses the FASTA file and returns a map from header token -> fastaEntry.
func buildFastaIndex(ctx context.Context, fastaPath string) (map[string]fastaEntry, error) {
	var index map[string]fastaEntry
	err := phygoboost.RunDisk(ctx, func(ctx context.Context) error {
		var err error
		index, err = buildFastaIndexLocked(fastaPath)
		return err
	})
	return index, err
}

func buildFastaIndexLocked(fastaPath string) (map[string]fastaEntry, error) {
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
func saveBlastResultToCache(ctx context.Context, cacheDir string, jobID string, result model.BlastResult) error {
	return phygoboost.RunDisk(ctx, func(ctx context.Context) error {
		return saveBlastResultToCacheLocked(ctx, cacheDir, jobID, result)
	})
}

func saveBlastResultToCacheLocked(ctx context.Context, cacheDir string, jobID string, result model.BlastResult) error {
	outPath := filepath.Join(cacheDir, jobID+".tsv")
	tmpPath := uniqueTempPath(outPath, ".part")
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	writer := csv.NewWriter(f)
	writer.Comma = '\t'
	encoder := csvutil.NewEncoder(writer)
	for i, r := range result.Rows {
		if err := encoder.Encode(localBlastCacheRecord{
			Hit:             i + 1,
			Protein:         r.Protein,
			SubjectID:       r.SubjectID,
			QueryID:         r.QueryID,
			QueryFrom:       r.QueryFrom,
			QueryTo:         r.QueryTo,
			TargetFrom:      r.TargetFrom,
			TargetTo:        r.TargetTo,
			EValue:          r.EValue,
			PercentIdentity: r.PercentIdentity,
			AlignLength:     r.AlignLength,
			Mismatches:      r.Mismatches,
			GapOpenings:     r.GapOpenings,
			Bitscore:        r.Bitscore,
			TargetLength:    r.TargetLength,
			SequenceID:      r.SequenceID,
			TranscriptID:    r.TranscriptID,
			TargetID:        r.TargetID,
			JBrowseName:     r.JBrowseName,
			GeneReportURL:   r.GeneReportURL,
			Defline:         r.Defline,
		}); err != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)
			return err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return moveTempIntoPlaceLocked(ctx, tmpPath, outPath, false)
}

type localBlastCacheRecord struct {
	Hit             int     `csv:"hit"`
	Protein         string  `csv:"protein"`
	SubjectID       string  `csv:"subject_id"`
	QueryID         string  `csv:"qseqid"`
	QueryFrom       int     `csv:"qstart"`
	QueryTo         int     `csv:"qend"`
	TargetFrom      int     `csv:"sstart"`
	TargetTo        int     `csv:"send"`
	EValue          string  `csv:"evalue"`
	PercentIdentity float64 `csv:"pident"`
	AlignLength     int     `csv:"align_len"`
	Mismatches      int     `csv:"mismatch"`
	GapOpenings     int     `csv:"gapopen"`
	Bitscore        float64 `csv:"bitscore"`
	TargetLength    int     `csv:"target_length"`
	SequenceID      string  `csv:"sequence_id"`
	TranscriptID    string  `csv:"transcript_id"`
	TargetID        int     `csv:"target_id"`
	JBrowseName     string  `csv:"jbrowse_name"`
	GeneReportURL   string  `csv:"gene_report_url"`
	Defline         string  `csv:"defline"`
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
	s = strings.Map(func(r rune) rune {
		switch {
		case r < 32:
			return '_'
		case strings.ContainsRune(`<>:"/\|?*`, r):
			return '_'
		default:
			return r
		}
	}, strings.TrimSpace(s))
	s = strings.Trim(s, ". ")
	if s == "" {
		return "item"
	}
	return s
}

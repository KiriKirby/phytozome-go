// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package blastplus

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/KiriKirby/phytozome-go/internal/progressctx"
)

const latestURL = "https://ftp.ncbi.nlm.nih.gov/blast/executables/blast+/LATEST/"

var (
	toolsOnPathMu    sync.RWMutex
	toolsOnPathCache = make(map[string]struct{})
)

type MissingToolsError struct {
	Tools []string
}

func (e *MissingToolsError) Error() string {
	if len(e.Tools) == 0 {
		return "BLAST+ tools are missing"
	}
	return fmt.Sprintf("%s not found in PATH; BLAST+ is required", strings.Join(e.Tools, ", "))
}

func IsMissingToolsError(err error) bool {
	var target *MissingToolsError
	return errors.As(err, &target)
}

func AsMissingToolsError(err error, target **MissingToolsError) bool {
	return errors.As(err, target)
}

func ToolsDir() (string, error) {
	base, err := applicationDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "blastplus")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create blast+ tools dir: %w", err)
	}
	return dir, nil
}

func EnsureToolsOnPath(required ...string) error {
	cacheKey := toolsOnPathCacheKey(required)
	if cacheKey != "" {
		toolsOnPathMu.RLock()
		_, ok := toolsOnPathCache[cacheKey]
		toolsOnPathMu.RUnlock()
		if ok {
			return nil
		}
	}
	missing := make([]string, 0, len(required))
	for _, tool := range required {
		if tool == "" {
			continue
		}
		if _, err := execLookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	if len(missing) == 0 {
		storeToolsOnPathCache(cacheKey)
		return nil
	}

	toolsDir, err := ToolsDir()
	if err != nil {
		return err
	}
	binDir, found, err := findManagedBinDir(toolsDir)
	if err != nil {
		return err
	}
	if !found {
		return &MissingToolsError{Tools: missing}
	}
	prependPath(binDir)
	stillMissing := make([]string, 0, len(required))
	for _, tool := range required {
		if tool == "" {
			continue
		}
		if _, err := execLookPath(tool); err != nil {
			stillMissing = append(stillMissing, tool)
		}
	}
	if len(stillMissing) > 0 {
		return &MissingToolsError{Tools: stillMissing}
	}
	storeToolsOnPathCache(cacheKey)
	return nil
}

func storeToolsOnPathCache(key string) {
	if strings.TrimSpace(key) == "" {
		return
	}
	toolsOnPathMu.Lock()
	toolsOnPathCache[key] = struct{}{}
	toolsOnPathMu.Unlock()
}

func toolsOnPathCacheKey(required []string) string {
	tools := make([]string, 0, len(required))
	seen := make(map[string]struct{}, len(required))
	for _, tool := range required {
		tool = strings.ToLower(strings.TrimSpace(tool))
		if tool == "" {
			continue
		}
		if _, ok := seen[tool]; ok {
			continue
		}
		seen[tool] = struct{}{}
		tools = append(tools, tool)
	}
	if len(tools) == 0 {
		return ""
	}
	sort.Strings(tools)
	return os.Getenv("PATH") + "\x00" + strings.Join(tools, "\x00")
}

func InstallManaged(ctx context.Context, httpClient *http.Client) (string, error) {
	if httpClient == nil {
		httpClient = defaultHTTPClient()
	}
	archiveName, err := archiveNameForPlatform()
	if err != nil {
		return "", err
	}
	toolsDir, err := ToolsDir()
	if err != nil {
		return "", err
	}
	if binDir, found, err := findManagedBinDir(toolsDir); err == nil && found {
		prependPath(binDir)
		return binDir, nil
	}
	downloadURL := latestURL + archiveName
	archivePath := filepath.Join(toolsDir, archiveName)
	targetDir := filepath.Join(toolsDir, strings.TrimSuffix(archiveName, ".tar.gz"))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("create managed BLAST+ dir: %w", err)
	}
	if err := downloadArchive(ctx, httpClient, downloadURL, archivePath); err != nil {
		return "", err
	}
	if err := extractTarGz(ctx, archivePath, targetDir); err != nil {
		return "", err
	}
	binDir, found, err := findManagedBinDir(toolsDir)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("BLAST+ was downloaded but no usable bin directory was found")
	}
	prependPath(binDir)
	return binDir, nil
}

func archiveNameForPlatform() (string, error) {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "amd64" {
			return "ncbi-blast-2.17.0+-x64-win64.tar.gz", nil
		}
	case "linux":
		if runtime.GOARCH == "amd64" {
			return "ncbi-blast-2.17.0+-x64-linux.tar.gz", nil
		}
	case "darwin":
		if runtime.GOARCH == "amd64" {
			return "ncbi-blast-2.17.0+-x64-macosx.tar.gz", nil
		}
		if runtime.GOARCH == "arm64" {
			return "ncbi-blast-2.17.0+-aarch64-macosx.tar.gz", nil
		}
	}
	return "", fmt.Errorf("automatic BLAST+ install is not supported on %s/%s", runtime.GOOS, runtime.GOARCH)
}

func downloadArchive(ctx context.Context, httpClient *http.Client, url string, archivePath string) error {
	if _, err := os.Stat(archivePath); err == nil {
		progressctx.Report(ctx, 40, fmt.Sprintf("Using cached BLAST+ archive: %s", filepath.Base(archivePath)))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return fmt.Errorf("create BLAST+ archive dir: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create BLAST+ download request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}
	tmpPath := archivePath + ".part"
	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create BLAST+ archive file: %w", err)
	}
	total := resp.ContentLength
	progressctx.Report(ctx, 1, "Starting BLAST+ download...")
	counter := &progressWriter{
		ctx:     ctx,
		total:   total,
		base:    1,
		span:    39,
		prefix:  "Downloading BLAST+",
		sink:    out,
		lastPct: -1,
	}
	if _, err := io.CopyBuffer(counter, resp.Body, make([]byte, 1024*1024)); err != nil {
		_ = out.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write BLAST+ archive: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close BLAST+ archive: %w", err)
	}
	if err := os.Rename(tmpPath, archivePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("finalize BLAST+ archive: %w", err)
	}
	progressctx.Report(ctx, 40, fmt.Sprintf("Downloaded BLAST+ archive: %s", filepath.Base(archivePath)))
	return nil
}

func extractTarGz(ctx context.Context, archivePath string, targetDir string) error {
	progressctx.Report(ctx, 41, "Opening BLAST+ archive...")
	targetDir, err := filepath.Abs(filepath.Clean(targetDir))
	if err != nil {
		return fmt.Errorf("resolve BLAST+ target dir: %w", err)
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open BLAST+ archive: %w", err)
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open BLAST+ archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	entryCount := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("extract BLAST+ archive: %w", err)
		}
		if header == nil {
			continue
		}
		path, err := safeArchivePath(targetDir, header.Name)
		if err != nil {
			return err
		}
		if path == "" {
			continue
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return fmt.Errorf("create BLAST+ dir %s: %w", path, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("create BLAST+ parent dir %s: %w", filepath.Dir(path), err)
			}
			file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
			if err != nil {
				return fmt.Errorf("create BLAST+ file %s: %w", path, err)
			}
			if _, err := io.Copy(file, tr); err != nil {
				_ = file.Close()
				return fmt.Errorf("write BLAST+ file %s: %w", path, err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close BLAST+ file %s: %w", path, err)
			}
		}
		entryCount++
		progressctx.Report(ctx, minInt(99, 41+entryCount), fmt.Sprintf("Extracting BLAST+ archive... %d files", entryCount))
	}
	progressctx.Report(ctx, 100, "BLAST+ extraction completed.")
	return nil
}

func safeArchivePath(targetDir string, entryName string) (string, error) {
	entryName = strings.TrimSpace(entryName)
	if entryName == "" {
		return "", nil
	}
	if strings.HasPrefix(entryName, "/") || strings.HasPrefix(entryName, `\`) {
		return "", fmt.Errorf("refusing to extract unexpected path %s", entryName)
	}
	name := filepath.Clean(entryName)
	if name == "." || name == string(filepath.Separator) {
		return "", nil
	}
	if filepath.IsAbs(name) || strings.Contains(filepath.ToSlash(name), ":") {
		return "", fmt.Errorf("refusing to extract unexpected path %s", entryName)
	}
	targetPath := filepath.Join(targetDir, name)
	rel, err := filepath.Rel(targetDir, targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve BLAST+ archive path %s: %w", entryName, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("refusing to extract unexpected path %s", entryName)
	}
	return targetPath, nil
}

type progressWriter struct {
	ctx     context.Context
	total   int64
	written int64
	base    int
	span    int
	prefix  string
	sink    io.Writer
	lastPct int
}

func (w *progressWriter) Write(p []byte) (int, error) {
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

func (w *progressWriter) report() {
	if w.total > 0 {
		pct := int((w.written * 100) / w.total)
		if pct == w.lastPct {
			return
		}
		w.lastPct = pct
		progressctx.Report(w.ctx, w.base+(w.span*pct)/100, fmt.Sprintf("%s... %d%% (%s/%s)", w.prefix, pct, humanBytes(w.written), humanBytes(w.total)))
		return
	}
	progressctx.Report(w.ctx, w.base, fmt.Sprintf("%s... %s", w.prefix, humanBytes(w.written)))
}

func humanBytes(v int64) string {
	const unit = 1024
	if v < unit {
		return fmt.Sprintf("%d B", v)
	}
	div, exp := int64(unit), 0
	for n := v / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(v)/float64(div), "KMGTPE"[exp])
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func findManagedBinDir(root string) (string, bool, error) {
	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		if matched, _ := regexp.MatchString(`(?i)[\\/]bin$`, path); matched {
			if hasTool(path, "makeblastdb") {
				found = path
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return "", false, fmt.Errorf("scan managed BLAST+ dir: %w", err)
	}
	return found, found != "", nil
}

func hasTool(dir string, tool string) bool {
	_, err := os.Stat(filepath.Join(dir, executableName(tool)))
	return err == nil
}

func prependPath(dir string) {
	current := os.Getenv("PATH")
	for _, part := range filepath.SplitList(current) {
		if strings.EqualFold(filepath.Clean(part), filepath.Clean(dir)) {
			return
		}
	}
	if current == "" {
		_ = os.Setenv("PATH", dir)
		return
	}
	_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+current)
}

func executableName(tool string) string {
	if runtime.GOOS == "windows" {
		return tool + ".exe"
	}
	return tool
}

func applicationDir() (string, error) {
	executablePath, err := executableFn()
	if err == nil {
		executableDir := filepath.Dir(executablePath)
		if !strings.Contains(strings.ToLower(executableDir), strings.ToLower(tempDirFn())) {
			return executableDir, nil
		}
	}

	workingDir, err := getwdFn()
	if err != nil {
		return "", fmt.Errorf("resolve application directory: %w", err)
	}
	return workingDir, nil
}

var executableFn = os.Executable
var getwdFn = os.Getwd
var tempDirFn = os.TempDir

var execLookPath = func(file string) (string, error) {
	return exec.LookPath(file)
}

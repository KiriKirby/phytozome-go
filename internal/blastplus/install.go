package blastplus

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/cavaliergopher/grab/v3"
	"github.com/gofrs/flock"
	"github.com/karrick/godirwalk"
	"github.com/klauspost/compress/gzip"
)

const latestURL = "https://ftp.ncbi.nlm.nih.gov/blast/executables/blast+/LATEST/"

var latestDownloadURLs = []string{
	latestURL,
	"https://ftp.ncbi.nih.gov/blast/executables/blast+/LATEST/",
}

var errFoundManagedBin = errors.New("found managed BLAST+ bin")

var installTempCounter atomic.Uint64

type ProgressFunc func(current int, message string)

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
	if err := phygoboost.RunDisk(context.Background(), func(ctx context.Context) error {
		return os.MkdirAll(dir, 0o755)
	}); err != nil {
		return "", fmt.Errorf("create blast+ tools dir: %w", err)
	}
	return dir, nil
}

func EnsureToolsOnPath(required ...string) error {
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
	return nil
}

func InstallManaged(ctx context.Context, httpClient *http.Client) (string, error) {
	return InstallManagedWithProgress(ctx, httpClient, nil)
}

func AddToolsDirToPath(dir string) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return
	}
	prependPath(dir)
}

func InstallManagedWithProgress(ctx context.Context, httpClient *http.Client, progress ProgressFunc) (string, error) {
	if httpClient == nil {
		httpClient = phygoboost.HTTPClient()
	}
	fallbackArchiveName, err := archiveNameForPlatform()
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
	lock := flock.New(filepath.Join(toolsDir, ".install.lock"))
	locked, err := lock.TryLockContext(ctx, 250*time.Millisecond)
	if err != nil {
		_ = lock.Close()
		return "", fmt.Errorf("lock managed BLAST+ installer: %w", err)
	}
	if !locked {
		_ = lock.Close()
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("managed BLAST+ installer lock was not acquired")
	}
	defer func() {
		_ = lock.Close()
	}()
	if binDir, found, err := findManagedBinDir(toolsDir); err == nil && found {
		prependPath(binDir)
		return binDir, nil
	}

	archiveNames := discoverArchiveNames(ctx, httpClient, fallbackArchiveName)
	candidates := downloadCandidates(archiveNames)

	var errs []error
	for i, candidate := range candidates {
		targetDir := filepath.Join(toolsDir, strings.TrimSuffix(candidate.ArchiveName, ".tar.gz"))
		archivePath := filepath.Join(toolsDir, candidate.ArchiveName)
		if err := resetManagedInstallTarget(ctx, targetDir, archivePath); err != nil {
			return "", err
		}

		reportProgress(progress, 0, 100, fmt.Sprintf("Trying BLAST+ source %d/%d...", i+1, len(candidates)))
		if err := downloadAndExtractTarGz(ctx, httpClient, candidate.URL, targetDir, archivePath, progress); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", candidate.URL, err))
			continue
		}

		binDir, found, err := findManagedBinDir(toolsDir)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", candidate.URL, err))
			continue
		}
		if !found {
			errs = append(errs, fmt.Errorf("%s: BLAST+ was extracted but no usable bin directory was found", candidate.URL))
			continue
		}
		prependPath(binDir)
		reportProgress(progress, 100, 100, "BLAST+ installation complete.")
		return binDir, nil
	}
	if len(errs) == 0 {
		return "", fmt.Errorf("unable to install managed BLAST+")
	}
	return "", errors.Join(errs...)
}

func resetManagedInstallTarget(ctx context.Context, targetDir string, archivePath string) error {
	return phygoboost.RunDisk(ctx, func(ctx context.Context) error {
		if err := os.RemoveAll(targetDir); err != nil {
			return fmt.Errorf("reset managed BLAST+ dir: %w", err)
		}
		if err := os.Remove(archivePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("reset managed BLAST+ archive: %w", err)
		}
		return nil
	})
}

type downloadCandidate struct {
	ArchiveName string
	URL         string
}

func downloadCandidates(archiveNames []string) []downloadCandidate {
	seen := make(map[string]bool)
	candidates := make([]downloadCandidate, 0, len(archiveNames)*len(latestDownloadURLs))
	for _, archiveName := range archiveNames {
		for _, baseURL := range latestDownloadURLs {
			url := baseURL + archiveName
			if seen[url] {
				continue
			}
			seen[url] = true
			candidates = append(candidates, downloadCandidate{ArchiveName: archiveName, URL: url})
		}
	}
	return candidates
}

func discoverArchiveNames(ctx context.Context, httpClient *http.Client, fallbackArchiveName string) []string {
	suffix, err := archiveSuffixForPlatform()
	if err != nil {
		return []string{fallbackArchiveName}
	}
	seen := make(map[string]bool)
	var names []string
	for _, baseURL := range latestDownloadURLs {
		body, err := fetchText(ctx, httpClient, baseURL)
		if err != nil {
			continue
		}
		for _, name := range archiveLinks(body) {
			if strings.HasSuffix(name, suffix) && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	if !seen[fallbackArchiveName] {
		names = append(names, fallbackArchiveName)
	}
	if len(names) == 0 {
		return []string{fallbackArchiveName}
	}
	return names
}

func archiveNameForPlatform() (string, error) {
	suffix, err := archiveSuffixForPlatform()
	if err != nil {
		return "", err
	}
	return "ncbi-blast-2.17.0+" + suffix, nil
}

func archiveSuffixForPlatform() (string, error) {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "amd64" {
			return "-x64-win64.tar.gz", nil
		}
	case "linux":
		if runtime.GOARCH == "amd64" {
			return "-x64-linux.tar.gz", nil
		}
	case "darwin":
		if runtime.GOARCH == "amd64" {
			return "-x64-macosx.tar.gz", nil
		}
		if runtime.GOARCH == "arm64" {
			return "-aarch64-macosx.tar.gz", nil
		}
	}
	return "", fmt.Errorf("automatic BLAST+ install is not supported on %s/%s", runtime.GOOS, runtime.GOARCH)
}

func archiveLinks(body string) []string {
	re := regexp.MustCompile(`href=["']([^"']+\.tar\.gz)["']`)
	matches := re.FindAllStringSubmatch(body, -1)
	links := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" || strings.Contains(name, "/") {
			continue
		}
		links = append(links, name)
	}
	return links
}

func fetchText(ctx context.Context, httpClient *http.Client, url string) (string, error) {
	body, err := fetchGrabText(ctx, httpClient, url)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func downloadAndExtractTarGz(ctx context.Context, httpClient *http.Client, url string, targetDir string, archivePath string, progress ProgressFunc) error {
	if err := phygoboost.RunDisk(ctx, func(ctx context.Context) error {
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return fmt.Errorf("create managed BLAST+ dir: %w", err)
		}
		removeStalePartFilesLocked(archivePath)
		return nil
	}); err != nil {
		return fmt.Errorf("create managed BLAST+ dir: %w", err)
	}

	tmpPath := uniquePartPath(archivePath)
	if err := downloadGrabFile(ctx, httpClient, url, tmpPath, progress, "Downloading BLAST+ archive", 0, 70); err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	if err := renameWithRetry(ctx, tmpPath, archivePath); err != nil {
		return fmt.Errorf("save BLAST+ archive: %w", err)
	}

	totalExtractBytes, err := tarGzUncompressedSize(ctx, archivePath)
	if err != nil {
		return fmt.Errorf("inspect BLAST+ archive: %w", err)
	}
	if err := extractTarGzArchive(ctx, archivePath, targetDir, totalExtractBytes, progress); err != nil {
		return err
	}
	return nil
}

func uniquePartPath(path string) string {
	return fmt.Sprintf("%s.%d.%d.%d.part", path, os.Getpid(), time.Now().UnixNano(), installTempCounter.Add(1))
}

func removeStalePartFiles(path string) {
	_ = phygoboost.RunDisk(context.Background(), func(ctx context.Context) error {
		removeStalePartFilesLocked(path)
		return nil
	})
}

func removeStalePartFilesLocked(path string) {
	_ = os.Remove(path + ".part")
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, base+".") && strings.HasSuffix(name, ".part") {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

func renameWithRetry(ctx context.Context, tmpPath string, destPath string) error {
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
		}
		if err := phygoboost.SleepContext(ctx, time.Duration(25*(attempt+1))*time.Millisecond); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
	}
	_ = os.Remove(tmpPath)
	return lastErr
}

func fetchGrabText(ctx context.Context, httpClient *http.Client, url string) ([]byte, error) {
	if httpClient == nil {
		httpClient = phygoboost.HTTPClient()
	}
	var data []byte
	err := phygoboost.RunTaskSpec(ctx, phygoboost.TaskSpec{Level: phygoboost.ExecHeavy, Domain: domainForDownloadURL(url), Description: "fetch blastplus text"}, func(ctx context.Context) error {
		client := grab.NewClient()
		client.HTTPClient = httpClient
		req, reqErr := grab.NewRequest("", url)
		if reqErr != nil {
			return fmt.Errorf("create request: %w", reqErr)
		}
		req = req.WithContext(ctx)
		req.NoStore = true
		resp := client.Do(req)
		<-resp.Done
		if err := resp.Err(); err != nil {
			return err
		}
		bytes, bytesErr := resp.Bytes()
		if bytesErr != nil {
			return bytesErr
		}
		data = bytes
		return nil
	})
	return data, err
}

func downloadGrabFile(ctx context.Context, httpClient *http.Client, url string, dst string, progress ProgressFunc, prefix string, startPercent int, endPercent int) error {
	if httpClient == nil {
		httpClient = phygoboost.HTTPClient()
	}
	return phygoboost.RunTaskSpec(ctx, phygoboost.TaskSpec{Level: phygoboost.ExecHeavy, Domain: domainForDownloadURL(url), Description: "download blastplus archive"}, func(ctx context.Context) error {
		client := grab.NewClient()
		client.HTTPClient = httpClient
		req, err := grab.NewRequest(dst, url)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req = req.WithContext(ctx)
		resp := client.Do(req)
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		defer func() {
			if resp != nil && resp.Err() != nil {
				_ = os.Remove(dst)
			}
		}()
		for {
			select {
			case <-resp.Done:
				if err := resp.Err(); err != nil {
					return err
				}
				return nil
			case <-ticker.C:
				reportProgress(progress, stagePercent(resp.BytesComplete(), resp.Size(), startPercent, endPercent), 100, fmt.Sprintf("%s... %s / %s", prefix, formatBytes(resp.BytesComplete()), formatBytes(resp.Size())))
			case <-ctx.Done():
				_ = resp.Cancel()
				return ctx.Err()
			}
		}
	})
}

func domainForDownloadURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return parsed.Host
}

func extractTarGzArchive(ctx context.Context, archivePath string, targetDir string, totalExtractBytes int64, progress ProgressFunc) error {
	return phygoboost.RunDisk(ctx, func(ctx context.Context) error {
		return extractTarGzArchiveLocked(ctx, archivePath, targetDir, totalExtractBytes, progress)
	})
}

func extractTarGzArchiveLocked(ctx context.Context, archivePath string, targetDir string, totalExtractBytes int64, progress ProgressFunc) error {
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
	var extractedBytes int64
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
		name := filepath.Clean(header.Name)
		if name == "." || name == string(filepath.Separator) {
			continue
		}
		path := filepath.Join(targetDir, name)
		rel, err := filepath.Rel(targetDir, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("refusing to extract unexpected path %s", header.Name)
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
			if _, err := io.Copy(&archiveCopyProgressWriter{
				File:          file,
				Progress:      progress,
				Current:       &extractedBytes,
				Total:         totalExtractBytes,
				StartPercent:  70,
				EndPercent:    95,
				MessagePrefix: "Extracting BLAST+ files",
			}, tr); err != nil {
				_ = file.Close()
				return fmt.Errorf("write BLAST+ file %s: %w", path, err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close BLAST+ file %s: %w", path, err)
			}
			reportProgress(progress, stagePercent(extractedBytes, totalExtractBytes, 70, 95), 100, fmt.Sprintf("Extracting BLAST+ files... %s / %s", formatBytes(extractedBytes), formatBytes(totalExtractBytes)))
		}
	}
	reportProgress(progress, 95, 100, "Verifying BLAST+ installation...")
	return nil
}

type archiveCopyProgressWriter struct {
	File          *os.File
	Progress      ProgressFunc
	Current       *int64
	Total         int64
	StartPercent  int
	EndPercent    int
	MessagePrefix string
}

func (w *archiveCopyProgressWriter) Write(p []byte) (int, error) {
	n, err := w.File.Write(p)
	if n > 0 && w.Current != nil {
		*w.Current += int64(n)
		reportProgress(w.Progress, stagePercent(*w.Current, w.Total, w.StartPercent, w.EndPercent), 100, fmt.Sprintf("%s... %s / %s", w.MessagePrefix, formatBytes(*w.Current), formatBytes(w.Total)))
	}
	return n, err
}

func tarGzUncompressedSize(ctx context.Context, archivePath string) (int64, error) {
	var size int64
	err := phygoboost.RunDisk(ctx, func(ctx context.Context) error {
		var err error
		size, err = tarGzUncompressedSizeLocked(ctx, archivePath)
		return err
	})
	return size, err
}

func tarGzUncompressedSizeLocked(ctx context.Context, archivePath string) (int64, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return 0, fmt.Errorf("open BLAST+ archive: %w", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return 0, fmt.Errorf("open BLAST+ archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("scan BLAST+ archive: %w", err)
		}
		if header != nil && header.Typeflag == tar.TypeReg {
			total += header.Size
		}
	}
	return total, nil
}

func stagePercent(current int64, total int64, startPercent int, endPercent int) int {
	if endPercent < startPercent {
		startPercent, endPercent = endPercent, startPercent
	}
	if total <= 0 {
		return startPercent
	}
	span := endPercent - startPercent
	if span <= 0 {
		return startPercent
	}
	percent := startPercent + int((current*int64(span))/total)
	if percent < startPercent {
		return startPercent
	}
	if percent > endPercent {
		return endPercent
	}
	return percent
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	for _, unit := range units {
		value /= 1024
		if value < 1024 {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%.1f EB", value/1024)
}

func reportProgress(progress ProgressFunc, current int, total int, message string) {
	if progress == nil {
		return
	}
	progress(current, message)
}

func findManagedBinDir(root string) (string, bool, error) {
	var found string
	err := godirwalk.Walk(root, &godirwalk.Options{
		Unsorted: true,
		Callback: func(path string, entry *godirwalk.Dirent) error {
			if entry == nil || !entry.IsDir() {
				return nil
			}
			if matched, _ := regexp.MatchString(`(?i)[\\/]bin$`, path); matched {
				if hasTool(path, "makeblastdb") {
					found = path
					return errFoundManagedBin
				}
			}
			return nil
		},
		ErrorCallback: func(_ string, err error) godirwalk.ErrorAction {
			if errors.Is(err, errFoundManagedBin) {
				return godirwalk.Halt
			}
			return godirwalk.SkipNode
		},
	})
	if err != nil {
		if errors.Is(err, errFoundManagedBin) && found != "" {
			return found, true, nil
		}
		if errors.Is(err, io.EOF) {
			return found, found != "", nil
		}
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


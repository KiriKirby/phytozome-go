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
	"strings"
)

const latestURL = "https://ftp.ncbi.nlm.nih.gov/blast/executables/blast+/LATEST/"

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
	if httpClient == nil {
		httpClient = http.DefaultClient
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

	body, err := fetchText(ctx, httpClient, latestURL)
	if err != nil {
		return "", err
	}
	if !strings.Contains(body, archiveName) {
		return "", fmt.Errorf("official BLAST+ archive %s was not found in %s", archiveName, latestURL)
	}
	downloadURL := latestURL + archiveName
	targetDir := filepath.Join(toolsDir, strings.TrimSuffix(archiveName, ".tar.gz"))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("create managed BLAST+ dir: %w", err)
	}
	if err := downloadAndExtractTarGz(ctx, httpClient, downloadURL, targetDir); err != nil {
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

func fetchText(ctx context.Context, httpClient *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create BLAST+ request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: unexpected status %s", url, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", url, err)
	}
	return string(body), nil
}

func downloadAndExtractTarGz(ctx context.Context, httpClient *http.Client, url string, targetDir string) error {
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
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("open BLAST+ archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
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
		if !strings.HasPrefix(path, targetDir) {
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
			if _, err := io.Copy(file, tr); err != nil {
				_ = file.Close()
				return fmt.Errorf("write BLAST+ file %s: %w", path, err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close BLAST+ file %s: %w", path, err)
			}
		}
	}
	return nil
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

package megaphgo

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/KiriKirby/phytozome-go/internal/progressctx"
)

const (
	releaseDownloadBaseURL = "https://github.com/KiriKirby/phytozome-go/releases/download"
	envDownloadURL         = "PHYTOZOME_GO_MEGAPHGO_RUNTIME_URL"
	RuntimeExecutable      = "mega-phgo-runtime"
	MuscleExecutable       = "runtime-owned MUSCLE"
	RuntimeProbeArgument   = "--phgo-runtime-probe"
	RuntimeProbeToken      = "mega-phgo-runtime"
	runtimeProbeTimeout    = 60 * time.Second
)

type runtimeReleaseManifest struct {
	ReleaseTag string            `json:"release_tag"`
	Assets     map[string]string `json:"assets"`
}

//go:embed runtime-release.json
var runtimeReleaseManifestJSON []byte

var currentRuntimeReleaseManifest, runtimeReleaseManifestErr = loadRuntimeReleaseManifest()

type MissingToolsError struct {
	Tools      []string
	RuntimeDir string
}

func (e *MissingToolsError) Error() string {
	dir := strings.TrimSpace(e.RuntimeDir)
	if dir == "" {
		if runtimeDir, err := ToolsDir(); err == nil {
			dir = runtimeDir
		}
	}
	if len(e.Tools) == 0 {
		if dir == "" {
			return "PHgo tree runtime is missing; this Windows release requires the bundled application-local PHgo runtime files"
		}
		return fmt.Sprintf("PHgo tree runtime is missing; expected the bundled application-local PHgo runtime files at %s", dir)
	}
	if dir == "" {
		return fmt.Sprintf("%s not found; re-extract or reinstall the Windows bundle so the bundled PHgo runtime files are restored", strings.Join(e.Tools, ", "))
	}
	return fmt.Sprintf("%s not found in %s; re-extract or reinstall the Windows bundle so the bundled PHgo runtime files are restored", strings.Join(e.Tools, ", "), dir)
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
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		return base, nil
	}
	return filepath.Join(base, "mega-phgo-runtime"), nil
}

func bundledRuntimeSupportError() error {
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		return nil
	}
	return fmt.Errorf("PHgo system-tree is currently supported only in the Windows amd64 release because mega-phgo-runtime is bundled only there; %s/%s is not supported", runtime.GOOS, runtime.GOARCH)
}

func EnsureRuntimeAvailable(required ...string) error {
	if err := bundledRuntimeSupportError(); err != nil {
		return err
	}
	if len(required) == 0 {
		toolsDir, err := ToolsDir()
		if err != nil {
			return err
		}
		if _, found, err := ManagedExecutable(); err != nil {
			return err
		} else if found {
			return nil
		}
		return &MissingToolsError{Tools: []string{RuntimeExecutable, MuscleExecutable}, RuntimeDir: toolsDir}
	}
	toolsDir, err := ToolsDir()
	if err != nil {
		return err
	}
	missing := make([]string, 0, len(required))
	for _, tool := range required {
		if strings.TrimSpace(tool) == "" {
			continue
		}
		path := filepath.Join(toolsDir, executableName(tool))
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			missing = append(missing, tool)
		}
	}
	if len(missing) > 0 {
		return &MissingToolsError{Tools: missing, RuntimeDir: toolsDir}
	}
	for _, tool := range required {
		if isRuntimeExecutableName(tool) {
			if err := ProbeExecutable(filepath.Join(toolsDir, executableName(tool))); err != nil {
				return &MissingToolsError{Tools: []string{RuntimeExecutable}, RuntimeDir: toolsDir}
			}
		}
	}
	return nil
}

func ManagedExecutable() (string, bool, error) {
	if err := bundledRuntimeSupportError(); err != nil {
		return "", false, err
	}
	toolsDir, err := ToolsDir()
	if err != nil {
		return "", false, err
	}
	for _, name := range executableCandidates() {
		path := filepath.Join(toolsDir, executableName(name))
		if info, err := os.Stat(path); err == nil && !info.IsDir() && runtimeOwnedMuscleAvailable(toolsDir) && ProbeExecutable(path) == nil {
			return path, true, nil
		}
	}
	return "", false, nil
}

func PrepareExecution(path string) (string, func(), error) {
	cleanup := func() {}
	if runtime.GOOS != "windows" {
		return path, cleanup, nil
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", cleanup, fmt.Errorf("PHgo tree runtime executable path is empty")
	}
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", cleanup, err
	}
	if !strings.HasSuffix(strings.ToLower(absPath), ".bin") {
		return absPath, cleanup, nil
	}
	toolsDir, err := ToolsDir()
	if err != nil {
		return "", cleanup, err
	}
	runtimeSource := filepath.Join(toolsDir, executableName(RuntimeExecutable))
	if !samePath(absPath, runtimeSource) {
		return absPath, cleanup, nil
	}
	muscleSource := filepath.Join(toolsDir, muscleExecutableName())
	if info, err := os.Stat(muscleSource); err != nil || info.IsDir() {
		return "", cleanup, &MissingToolsError{Tools: []string{MuscleExecutable}, RuntimeDir: toolsDir}
	}
	tempDir, err := os.MkdirTemp("", "phytozome-go-megaphgo-runtime-")
	if err != nil {
		return "", cleanup, fmt.Errorf("create PHgo runtime temp directory: %w", err)
	}
	cleanup = func() {
		_ = os.RemoveAll(tempDir)
	}
	tempRuntime := filepath.Join(tempDir, RuntimeExecutable+".exe")
	tempMuscle := filepath.Join(tempDir, "muscleWin64.bin")
	if err := copyExecutionFile(runtimeSource, tempRuntime); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if err := copyExecutionFile(muscleSource, tempMuscle); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return tempRuntime, cleanup, nil
}

func ProbeExecutable(path string) error {
	return probeExecutableFn(path)
}

func probeExecutable(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return &MissingToolsError{Tools: []string{RuntimeExecutable}}
	}
	exe, cleanup, err := PrepareExecution(path)
	if err != nil {
		return err
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), runtimeProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, RuntimeProbeArgument)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if ctx.Err() != nil {
		return fmt.Errorf("probe PHgo tree runtime %s: %w", path, ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("probe PHgo tree runtime %s: %w: %s", path, err, text)
	}
	if !strings.Contains(strings.ToLower(text), RuntimeProbeToken) {
		return fmt.Errorf("probe PHgo tree runtime %s: unexpected response %q", path, text)
	}
	return nil
}

func InstallManaged(ctx context.Context, httpClient *http.Client) (string, error) {
	if err := bundledRuntimeSupportError(); err != nil {
		return "", err
	}
	toolsDir, err := ToolsDir()
	if err != nil {
		return "", err
	}
	_ = ctx
	_ = httpClient
	if err := EnsureRuntimeAvailable(RuntimeExecutable, MuscleExecutable); err != nil {
		return "", err
	}
	exe, found, err := ManagedExecutable()
	if err != nil {
		return "", err
	}
	if !found {
		return "", &MissingToolsError{Tools: []string{RuntimeExecutable, MuscleExecutable}, RuntimeDir: toolsDir}
	}
	return filepath.Dir(exe), nil
}

type DownloadAsset struct {
	URL      string
	FileName string
}

func ResolveDownload() (DownloadAsset, error) {
	assetName, err := assetNameForPlatform()
	if err != nil {
		return DownloadAsset{}, err
	}
	rawURL := strings.TrimSpace(os.Getenv(envDownloadURL))
	if rawURL == "" {
		rawURL, err = releaseAssetURL(assetName)
		if err != nil {
			return DownloadAsset{}, err
		}
	}
	return DownloadAsset{URL: rawURL, FileName: assetName}, nil
}

func downloadArchive(ctx context.Context, httpClient *http.Client, rawURL string, archivePath string) error {
	httpClient = noTimeoutHTTPClient(httpClient)
	if _, err := os.Stat(archivePath); err == nil {
		progressctx.Report(ctx, 40, fmt.Sprintf("Using cached PHgo tree runtime archive: %s", filepath.Base(archivePath)))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return fmt.Errorf("create PHgo tree runtime archive dir: %w", err)
	}
	if handled, err := copyLocalArchive(ctx, rawURL, archivePath); handled {
		return err
	}
	return phygoboost.RunTaskSpec(ctx, phygoboost.TaskSpec{
		Level:       phygoboost.ExecManaged,
		Domain:      domainForDownloadURL(rawURL),
		Description: "download PHgo tree runtime archive",
	}, func(runCtx context.Context) error {
		req, err := http.NewRequestWithContext(runCtx, http.MethodGet, rawURL, nil)
		if err != nil {
			return fmt.Errorf("create PHgo tree runtime download request: %w", err)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("download %s: %w", rawURL, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("download %s: unexpected status %s", rawURL, resp.Status)
		}
		tmpPath := archivePath + ".part"
		out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("create PHgo tree runtime archive file: %w", err)
		}
		progressctx.Report(runCtx, 1, "Starting PHgo tree runtime download...")
		counter := &progressWriter{
			ctx:     runCtx,
			total:   resp.ContentLength,
			base:    1,
			span:    39,
			prefix:  "Downloading PHgo tree runtime",
			sink:    out,
			lastPct: -1,
		}
		if _, err := io.CopyBuffer(counter, resp.Body, make([]byte, 1024*1024)); err != nil {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			return fmt.Errorf("write PHgo tree runtime archive: %w", err)
		}
		if err := out.Close(); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("close PHgo tree runtime archive: %w", err)
		}
		if err := os.Rename(tmpPath, archivePath); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("finalize PHgo tree runtime archive: %w", err)
		}
		progressctx.Report(runCtx, 40, fmt.Sprintf("Downloaded PHgo tree runtime archive: %s", filepath.Base(archivePath)))
		return nil
	})
}

func copyLocalArchive(ctx context.Context, rawURL string, archivePath string) (bool, error) {
	localPath, ok, err := localFilesystemPath(rawURL)
	if err != nil || !ok {
		return ok, err
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return true, err
	}
	if info.IsDir() {
		return false, nil
	}
	src, err := os.Open(localPath)
	if err != nil {
		return true, err
	}
	defer src.Close()
	tmpPath := archivePath + ".part"
	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return true, err
	}
	progressctx.Report(ctx, 1, "Copying local PHgo tree runtime archive...")
	if _, err := io.CopyBuffer(out, src, make([]byte, 1024*1024)); err != nil {
		_ = out.Close()
		_ = os.Remove(tmpPath)
		return true, err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return true, err
	}
	if err := os.Rename(tmpPath, archivePath); err != nil {
		_ = os.Remove(tmpPath)
		return true, err
	}
	progressctx.Report(ctx, 40, fmt.Sprintf("Copied PHgo tree runtime archive: %s", filepath.Base(archivePath)))
	return true, nil
}

func installLocalRuntimeDir(ctx context.Context, rawURL string, toolsDir string) (bool, error) {
	localPath, ok, err := localFilesystemPath(rawURL)
	if err != nil || !ok {
		return ok, err
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return true, err
	}
	if !info.IsDir() {
		return false, nil
	}
	progressctx.Report(ctx, 1, "Copying local PHgo tree runtime directory...")
	if err := copyRuntimeDir(ctx, localPath, toolsDir); err != nil {
		return true, err
	}
	progressctx.Report(ctx, 100, "PHgo tree runtime directory copied.")
	return true, nil
}

func localFilesystemPath(rawURL string) (string, bool, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", false, nil
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "file://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", true, err
		}
		localPath := parsed.Path
		if runtime.GOOS == "windows" {
			localPath = strings.TrimPrefix(localPath, "/")
		}
		return localPath, true, nil
	}
	if filepath.IsAbs(trimmed) {
		return trimmed, true, nil
	}
	return "", false, nil
}

func copyRuntimeDir(ctx context.Context, sourceDir string, targetDir string) error {
	sourceDir, err := filepath.Abs(filepath.Clean(sourceDir))
	if err != nil {
		return fmt.Errorf("resolve PHgo tree runtime source dir: %w", err)
	}
	targetDir, err = filepath.Abs(filepath.Clean(targetDir))
	if err != nil {
		return fmt.Errorf("resolve PHgo tree runtime target dir: %w", err)
	}
	if strings.EqualFold(sourceDir, targetDir) {
		return nil
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create PHgo tree runtime target dir: %w", err)
	}
	copied := 0
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		targetPath := filepath.Join(targetDir, rel)
		if info.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			_ = src.Close()
			return err
		}
		if _, err := io.CopyBuffer(dst, src, make([]byte, 1024*1024)); err != nil {
			_ = src.Close()
			_ = dst.Close()
			return err
		}
		if err := src.Close(); err != nil {
			_ = dst.Close()
			return err
		}
		if err := dst.Close(); err != nil {
			return err
		}
		copied++
		progressctx.Report(ctx, minInt(99, 1+copied), fmt.Sprintf("Copying PHgo tree runtime... %d files", copied))
		return nil
	})
}

func extractArchive(ctx context.Context, archivePath string, targetDir string) error {
	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return extractTarGz(ctx, archivePath, targetDir)
	case strings.HasSuffix(lower, ".zip"):
		return extractZip(ctx, archivePath, targetDir)
	default:
		return fmt.Errorf("unsupported PHgo tree runtime archive format: %s; use a pre-extracted runtime .zip or .tar.gz asset", filepath.Base(archivePath))
	}
}

func extractTarGz(ctx context.Context, archivePath string, targetDir string) error {
	progressctx.Report(ctx, 41, "Opening PHgo tree runtime archive...")
	targetDir, err := filepath.Abs(filepath.Clean(targetDir))
	if err != nil {
		return fmt.Errorf("resolve PHgo tree runtime target dir: %w", err)
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open PHgo tree runtime archive: %w", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open PHgo tree runtime archive: %w", err)
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
			return fmt.Errorf("extract PHgo tree runtime archive: %w", err)
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
				return fmt.Errorf("create PHgo tree runtime dir %s: %w", path, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("create PHgo tree runtime parent dir %s: %w", filepath.Dir(path), err)
			}
			file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
			if err != nil {
				return fmt.Errorf("create PHgo tree runtime file %s: %w", path, err)
			}
			if _, err := io.Copy(file, tr); err != nil {
				_ = file.Close()
				return fmt.Errorf("write PHgo tree runtime file %s: %w", path, err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close PHgo tree runtime file %s: %w", path, err)
			}
		}
		entryCount++
		progressctx.Report(ctx, minInt(99, 41+entryCount), fmt.Sprintf("Extracting PHgo tree runtime archive... %d files", entryCount))
	}
	progressctx.Report(ctx, 100, "PHgo tree runtime extraction completed.")
	return nil
}

func extractZip(ctx context.Context, archivePath string, targetDir string) error {
	progressctx.Report(ctx, 41, "Opening PHgo tree runtime archive...")
	targetDir, err := filepath.Abs(filepath.Clean(targetDir))
	if err != nil {
		return fmt.Errorf("resolve PHgo tree runtime target dir: %w", err)
	}
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open PHgo tree runtime archive: %w", err)
	}
	defer zr.Close()
	for i, entry := range zr.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		path, err := safeArchivePath(targetDir, entry.Name)
		if err != nil {
			return err
		}
		if path == "" {
			continue
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return fmt.Errorf("create PHgo tree runtime dir %s: %w", path, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create PHgo tree runtime parent dir %s: %w", filepath.Dir(path), err)
		}
		src, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open PHgo tree runtime zip entry %s: %w", entry.Name, err)
		}
		dst, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			_ = src.Close()
			return fmt.Errorf("create PHgo tree runtime file %s: %w", path, err)
		}
		if _, err := io.Copy(dst, src); err != nil {
			_ = src.Close()
			_ = dst.Close()
			return fmt.Errorf("write PHgo tree runtime file %s: %w", path, err)
		}
		if err := src.Close(); err != nil {
			_ = dst.Close()
			return fmt.Errorf("close PHgo tree runtime zip entry %s: %w", entry.Name, err)
		}
		if err := dst.Close(); err != nil {
			return fmt.Errorf("close PHgo tree runtime file %s: %w", path, err)
		}
		progressctx.Report(ctx, minInt(99, 41+i), fmt.Sprintf("Extracting PHgo tree runtime archive... %d files", i+1))
	}
	progressctx.Report(ctx, 100, "PHgo tree runtime extraction completed.")
	return nil
}

func FindManagedBinDir(root string) (string, bool, error) {
	for _, name := range executableCandidates() {
		path := filepath.Join(root, executableName(name))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return root, true, nil
		} else if err != nil && !os.IsNotExist(err) {
			return "", false, fmt.Errorf("scan managed PHgo tree runtime dir: %w", err)
		}
	}
	return "", false, nil
}

func isRuntimeExecutableName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.TrimSuffix(name, ".exe")
	return name == RuntimeExecutable
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
		return "", fmt.Errorf("resolve PHgo tree runtime archive path %s: %w", entryName, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("refusing to extract unexpected path %s", entryName)
	}
	return targetPath, nil
}

func assetNameForPlatform() (string, error) {
	if runtimeReleaseManifestErr != nil {
		return "", runtimeReleaseManifestErr
	}
	platform, err := runtimeAssetPlatform()
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(currentRuntimeReleaseManifest.Assets[platform])
	if name == "" {
		return "", fmt.Errorf("PHgo tree runtime asset name is not configured for %s", platform)
	}
	return name, nil
}

func runtimeAssetPlatform() (string, error) {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "amd64" {
			return "windows-amd64", nil
		}
	}
	return "", fmt.Errorf("PHgo tree runtime package is currently published only for Windows amd64; %s/%s is not supported", runtime.GOOS, runtime.GOARCH)
}

func sanitizeArchiveName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "mega-phgo-runtime-download"
	}
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, `\`, "_")
	if !strings.Contains(strings.ToLower(name), "mega-phgo-runtime") {
		name = "mega-phgo-runtime-" + name
	}
	return name
}

func executableCandidates() []string {
	return []string{RuntimeExecutable}
}

func executableName(tool string) string {
	if tool == MuscleExecutable {
		return muscleExecutableName()
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(tool), ".bin") {
		return tool + ".bin"
	}
	return tool
}

func copyExecutionFile(source string, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open bundled runtime file %s: %w", source, err)
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create runtime temp directory %s: %w", filepath.Dir(target), err)
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("create runtime temp file %s: %w", target, err)
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return fmt.Errorf("copy bundled runtime file %s: %w", source, err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close runtime temp file %s: %w", target, err)
	}
	return nil
}

func runtimeOwnedMuscleAvailable(toolsDir string) bool {
	info, err := os.Stat(filepath.Join(toolsDir, muscleExecutableName()))
	return err == nil && !info.IsDir()
}

func muscleExecutableName() string {
	switch runtime.GOOS {
	case "windows":
		return "muscleWin64.bin"
	case "darwin":
		return "muscledarwin64"
	default:
		return "muscleUnix64.exe"
	}
}

func samePath(a string, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func domainForDownloadURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return parsed.Host
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

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func defaultHTTPClient() *http.Client {
	return phygoboost.HTTPClient()
}

func noTimeoutHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = defaultHTTPClient()
	}
	clone := *client
	clone.Timeout = 0
	return &clone
}

func releaseAssetURL(assetName string) (string, error) {
	if runtimeReleaseManifestErr != nil {
		return "", runtimeReleaseManifestErr
	}
	releaseVersion := strings.TrimSpace(currentRuntimeReleaseManifest.ReleaseTag)
	return strings.TrimRight(releaseDownloadBaseURL, "/") + "/" + releaseVersion + "/" + strings.TrimSpace(assetName), nil
}

func loadRuntimeReleaseManifest() (runtimeReleaseManifest, error) {
	var manifest runtimeReleaseManifest
	if err := json.Unmarshal(runtimeReleaseManifestJSON, &manifest); err != nil {
		return runtimeReleaseManifest{}, fmt.Errorf("parse PHgo tree runtime release manifest: %w", err)
	}
	manifest.ReleaseTag = strings.TrimSpace(manifest.ReleaseTag)
	if manifest.ReleaseTag == "" {
		return runtimeReleaseManifest{}, fmt.Errorf("PHgo tree runtime release manifest is missing release_tag")
	}
	if len(manifest.Assets) == 0 {
		return runtimeReleaseManifest{}, fmt.Errorf("PHgo tree runtime release manifest is missing assets")
	}
	for platform, name := range manifest.Assets {
		platform = strings.TrimSpace(platform)
		name = strings.TrimSpace(name)
		if platform == "" || name == "" {
			return runtimeReleaseManifest{}, fmt.Errorf("PHgo tree runtime release manifest contains an empty platform or asset name")
		}
	}
	return manifest, nil
}

var executableFn = os.Executable
var getwdFn = os.Getwd
var tempDirFn = os.TempDir
var probeExecutableFn = probeExecutable

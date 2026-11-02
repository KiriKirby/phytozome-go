package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/labelname"
	"github.com/KiriKirby/phytozome-go/internal/notifyaudio"
	"github.com/KiriKirby/phytozome-go/internal/startupstate"
)

const mainProgramName = "core.bin"
const windowsWezTermCLIName = "wezterm-cli.bin"
const displayName = "phytozome GO"
const author = "wangsychn"
const repoURL = "https://github.com/KiriKirby/phytozome-go"
const licenseName = "Common Public Attribution License 1.0"
const licenseID = "CPAL-1.0"
const skipBundlePreflightEnv = "PHGO_SKIP_BUNDLE_PREFLIGHT"
const autoAcceptUpdateEnv = "PHGO_AUTO_ACCEPT_UPDATE"
const updateDebugLogEnv = "PHGO_UPDATE_DEBUG_LOG"
const lastUpdateErrorEnv = "PHGO_LAST_UPDATE_ERROR"
const windowsErrorSharingViolation syscall.Errno = 32
const windowsErrorLockViolation syscall.Errno = 33
const windowsErrorAccessDenied syscall.Errno = 5
const startupASCIIArt = `          _____                    _____                    _____                   _______
         /\    \                  /\    \                  /\    \                 /::\    \
        /::\    \                /::\____\                /::\    \               /::::\    \
       /::::\    \              /:::/    /               /::::\    \             /::::::\    \
      /::::::\    \            /:::/    /               /::::::\    \           /::::::::\    \
     /:::/\:::\    \          /:::/    /               /:::/\:::\    \         /:::/~~\:::\    \
    /:::/__\:::\    \        /:::/____/               /:::/  \:::\    \       /:::/    \:::\    \
   /::::\   \:::\    \      /::::\    \              /:::/    \:::\    \     /:::/    / \:::\    \
  /::::::\   \:::\    \    /::::::\    \   _____    /:::/    / \:::\    \   /:::/____/   \:::\____\
 /:::/\:::\   \:::\____\  /:::/\:::\    \ /\    \  /:::/    /   \:::\ ___\ |:::|    |     |:::|    |
/:::/  \:::\   \:::|    |/:::/  \:::\    /::\____\/:::/____/  ___\:::|    ||:::|____|     |:::|    |
\::/    \:::\  /:::|____|\::/    \:::\  /:::/    /\:::\    \ /\  /:::|____| \:::\    \   /:::/    /
 \/_____/\:::\/:::/    /  \/____/ \:::\/:::/    /  \:::\    /::\ \::/    /   \:::\    \ /:::/    /
          \::::::/    /            \::::::/    /    \:::\   \:::\ \/____/     \:::\    /:::/    /
           \::::/    /              \::::/    /      \:::\   \:::\____\        \:::\__/:::/    /
            \::/____/               /:::/    /        \:::\  /:::/    /         \::::::::/    /
             ~~                    /:::/    /          \:::\/:::/    /           \::::::/    /
                                  /:::/    /            \::::::/    /             \::::/    /
                                 /:::/    /              \::::/    /               \::/____/
                                 \::/    /                \::/____/                 ~~
                                  \/____/
`

var version = "dev"

func main() {
	if err := run(); err != nil {
		fatal(err)
	}
}

func run() error {
	printStartupNotice()
	printLastUpdateErrorNotice()
	appDir, err := applicationDir()
	if err != nil {
		return err
	}
	if err := waitForPendingBundleUpdate(appDir); err != nil {
		return err
	}
	_ = startupstate.Write(appDir, startupstate.State{Status: startupstate.StatusInitializing, Message: "Startup helper is preparing phytozome GO."})
	mainAlreadyLaunched := false
	if shouldSkipBundleMaintenancePreflight() {
		_, _ = fmt.Fprintln(os.Stdout)
		_, _ = fmt.Fprintln(os.Stdout, "Bundle cache cleanup and release update check skipped for relaunch.")
	} else {
		cacheTargets, err := resolveCacheTargets()
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintln(os.Stdout)
		_, _ = fmt.Fprintln(os.Stdout, "Cache cleanup targets:")
		for _, target := range cacheTargets {
			_, _ = fmt.Fprintf(os.Stdout, "  - %s\n", target)
		}

		writeStartupStatus(appDir, startupstate.StatusInitializing, false, "Deleting startup cache directories.", "")
		if err := runSpinner("Deleting .cache directories", func() error {
			return removeCacheTargets(cacheTargets)
		}); err != nil {
			return err
		}

		_, _ = fmt.Fprintln(os.Stdout, "Cache cleanup complete.")
		writeStartupStatus(appDir, startupstate.StatusInitializing, false, "Checking for application updates.", "")
		if updateLaunched := maybeHandleReleaseUpdate(appDir, os.Args[1:]); updateLaunched {
			_ = startupstate.Complete(appDir)
			return nil
		}
	}
	writeStartupStatus(appDir, startupstate.StatusInitializing, false, "Checking symbol name database.", "")
	symbolLaunched, err := maybeEnsureSymbolNameDatabase(appDir, os.Args[1:])
	if err != nil {
		_ = startupstate.Complete(appDir)
		return err
	}
	mainAlreadyLaunched = mainAlreadyLaunched || symbolLaunched
	_ = startupstate.Complete(appDir)
	if mainAlreadyLaunched {
		return nil
	}
	return launchMainProgram(os.Args[1:])
}

func writeStartupStatus(appDir string, status string, allowUse bool, message string, dbPath string) {
	_ = startupstate.Write(appDir, startupstate.State{
		Status:   status,
		AllowUse: allowUse,
		Message:  message,
		DBPath:   dbPath,
	})
}

func printStartupNotice() {
	_, _ = fmt.Fprint(os.Stdout, startupASCIIArt)
	_, _ = fmt.Fprintf(os.Stdout, "%s %s\n", displayName, version)
	_, _ = fmt.Fprintf(os.Stdout, "Author: %s\n", author)
	_, _ = fmt.Fprintf(os.Stdout, "Repo:   %s\n", repoURL)
	_, _ = fmt.Fprintf(os.Stdout, "License: %s (%s)\n", licenseName, licenseID)
}

func printLastUpdateErrorNotice() {
	message := strings.TrimSpace(os.Getenv(lastUpdateErrorEnv))
	if message == "" {
		return
	}
	_, _ = fmt.Fprintln(os.Stdout)
	_, _ = fmt.Fprintln(os.Stdout, "Previous update attempt did not fully apply.")
	_, _ = fmt.Fprintf(os.Stdout, "Reason: %s\n", message)
}

func shouldSkipBundleMaintenancePreflight() bool {
	return strings.TrimSpace(os.Getenv(skipBundlePreflightEnv)) != ""
}

func runSpinner(label string, fn func() error) error {
	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()

	frames := []rune{'|', '/', '-', '\\'}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	index := 0
	for {
		select {
		case err := <-done:
			_, _ = fmt.Fprint(os.Stdout, "\r\033[2K")
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(os.Stdout, "\r%s... done.\n", label)
			return nil
		case <-ticker.C:
			_, _ = fmt.Fprintf(os.Stdout, "\r%c %s...", frames[index%len(frames)], label)
			index++
		}
	}
}

func launchMainProgramDirect(args []string) error {
	mainPath, err := resolveMainProgramPath()
	if err != nil {
		return err
	}

	cmd := exec.Command(mainPath, args...)
	cmd.Dir = filepath.Dir(mainPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("start %s: %w", filepath.Base(mainPath), err)
	}
	return nil
}

func launchMainProgramInNewTab(args []string) error {
	cleanerPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate helper executable: %w", err)
	}
	mainPath, err := resolveMainProgramPathFrom(cleanerPath)
	if err != nil {
		return err
	}
	weztermCLIPath, err := resolveWezTermCLIPathFrom(cleanerPath)
	if err != nil {
		return err
	}

	cmd := exec.Command(weztermCLIPath, buildWezTermSpawnArgs(mainPath, args)...)
	cmd.Dir = filepath.Dir(weztermCLIPath)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open %s in new tab: %w", filepath.Base(mainPath), err)
	}
	return nil
}

func launchMainProgram(args []string) error {
	if shouldSpawnMainProgramInNewTab() {
		_, _ = fmt.Fprintln(os.Stdout, "Opening phytozome GO in a new tab...")
		return launchMainProgramInNewTab(args)
	}
	_, _ = fmt.Fprintln(os.Stdout, "Starting phytozome GO...")
	return launchMainProgramDirect(args)
}

func resolveMainProgramPath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate helper executable: %w", err)
	}
	return resolveMainProgramPathFrom(exePath)
}

func resolveCacheTargets() ([]string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate helper executable: %w", err)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve current working directory: %w", err)
	}
	return resolveCacheTargetsFrom(exePath, workingDir), nil
}

func resolveWezTermCLIPathFrom(cleanerPath string) (string, error) {
	cleanerPath = strings.TrimSpace(cleanerPath)
	if cleanerPath == "" {
		return "", fmt.Errorf("helper path is empty")
	}
	dir := filepath.Dir(cleanerPath)
	candidates := []string{}
	switch runtime.GOOS {
	case "windows":
		candidates = append(candidates,
			filepath.Join(dir, windowsWezTermCLIName),
			filepath.Join(dir, "wezterm.exe"),
			filepath.Join(dir, "wezterm.bin"),
		)
	default:
		candidates = append(candidates,
			filepath.Join(dir, "wezterm-cli"),
			filepath.Join(dir, "wezterm"),
			filepath.Join(dir, "wezterm-gui"),
			filepath.Join(dir, "wezterm.AppImage"),
		)
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not locate WezTerm CLI next to the helper:\n%s", dir)
}

func resolveCacheTargetsFrom(cleanerPath string, workingDir string) []string {
	seen := make(map[string]struct{}, 4)
	targets := make([]string, 0, 4)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		key := strings.ToLower(clean)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		targets = append(targets, clean)
	}

	cleanerDir := filepath.Dir(strings.TrimSpace(cleanerPath))
	if cleanerDir != "" && cleanerDir != "." {
		add(filepath.Join(cleanerDir, ".cache"))
		if repoRoot, ok := detectDevRepoRootFromBundleDir(cleanerDir); ok {
			add(filepath.Join(repoRoot, ".cache"))
		}
	}
	if strings.TrimSpace(workingDir) != "" {
		add(filepath.Join(workingDir, ".cache"))
	}

	sort.Strings(targets)
	return targets
}

func detectDevRepoRootFromBundleDir(bundleDir string) (string, bool) {
	bundleDir = filepath.Clean(strings.TrimSpace(bundleDir))
	if bundleDir == "" {
		return "", false
	}
	parent := filepath.Dir(bundleDir)
	if !strings.EqualFold(filepath.Base(parent), "bin") {
		return "", false
	}
	repoRoot := filepath.Dir(parent)
	if repoRoot == "" || repoRoot == "." || samePath(repoRoot, parent) {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err != nil {
		return "", false
	}
	return repoRoot, true
}

func removeCacheTargets(targets []string) error {
	for _, target := range targets {
		if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
			if isLockedCacheCleanupError(err) {
				_, _ = fmt.Fprintf(os.Stdout, "\nWarning: cache directory %s is still in use; leaving locked files in place and continuing startup.\n", target)
				continue
			}
			return fmt.Errorf("delete cache directory %s: %w", target, err)
		}
	}
	return nil
}

func isLockedCacheCleanupError(err error) bool {
	return errors.Is(err, windowsErrorSharingViolation) ||
		errors.Is(err, windowsErrorLockViolation) ||
		errors.Is(err, windowsErrorAccessDenied)
}

func maybeEnsureSymbolNameDatabase(appDir string, args []string) (bool, error) {
	dbPath := labelname.DefaultGeneInfoDatabasePath(appDir)
	labelname.SetDefaultGeneInfoDatabasePath(dbPath)
	writeStartupStatus(appDir, startupstate.StatusInitializing, false, "Checking remote symbol name database manifest.", dbPath)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	plan, err := labelname.PreferredGeneInfoInstallPlan(ctx)
	if err != nil {
		return false, err
	}
	writeStartupStatus(appDir, startupstate.StatusInitializing, false, "Inspecting local symbol name database.", dbPath)
	remote := plan.Remote
	local, localErr := labelname.InspectGeneInfoDatabase(dbPath)
	if localErr != nil {
		_, _ = fmt.Fprintln(os.Stdout)
		_, _ = fmt.Fprintln(os.Stdout, "Symbol name library is missing.")
		_, _ = fmt.Fprintf(os.Stdout, "Database path: %s\n", dbPath)
		_, _ = fmt.Fprintf(os.Stdout, "Source: %s\n", plan.SourceLabel())
		_, _ = fmt.Fprintf(os.Stdout, "Download URL: %s\n", plan.SourceURL())
		_, _ = fmt.Fprintf(os.Stdout, "Remote size: %s\n", humanBytes(plan.DownloadSize()))
		_, _ = fmt.Fprintf(os.Stdout, "Last modified: %s\n", firstNonEmptyText(remote.LastModifiedRaw, "unknown"))
		if !canPromptForUpdateConsent() {
			return false, fmt.Errorf("symbol name library is missing and confirmation is not interactive")
		}
		if !promptYesNo(os.Stdin, os.Stdout, "Download and build the symbol name library now? [y/N]: ") {
			_, _ = fmt.Fprintln(os.Stdout, "Skipping symbol name library download. It will be required when symbol names are used.")
			return false, nil
		}
		// Symbol-name data is always installed completely before the main UI starts.
		// The former "use while downloading" branch is intentionally retired.
		return false, downloadSymbolNameDatabaseStartupWithRetry(appDir, "Downloading and preparing symbol name library", dbPath, plan, false)
	}
	if remote.LastModified.IsZero() || local.LastModified.IsZero() || !remote.LastModified.After(local.LastModified) {
		writeStartupStatus(appDir, startupstate.StatusInitializing, false, "Symbol name database is current.", dbPath)
		_, _ = fmt.Fprintf(os.Stdout, "Symbol name library: current (%d records).\n", local.RecordCount)
		return false, nil
	}
	expiredDays := int(remote.LastModified.Sub(local.LastModified).Hours() / 24)
	if expiredDays < 0 {
		expiredDays = 0
	}
	_, _ = fmt.Fprintln(os.Stdout)
	_, _ = fmt.Fprintln(os.Stdout, "Symbol name library update available.")
	_, _ = fmt.Fprintf(os.Stdout, "Database path: %s\n", dbPath)
	_, _ = fmt.Fprintf(os.Stdout, "Local Last-Modified:  %s\n", firstNonEmptyText(local.LastModifiedRaw, local.LastModified.Format(time.RFC1123)))
	_, _ = fmt.Fprintf(os.Stdout, "Remote Last-Modified: %s\n", firstNonEmptyText(remote.LastModifiedRaw, remote.LastModified.Format(time.RFC1123)))
	_, _ = fmt.Fprintf(os.Stdout, "Expired by: %d days\n", expiredDays)
	_, _ = fmt.Fprintf(os.Stdout, "Source: %s\n", plan.SourceLabel())
	_, _ = fmt.Fprintf(os.Stdout, "Download URL: %s\n", plan.SourceURL())
	_, _ = fmt.Fprintf(os.Stdout, "Remote size: %s\n", humanBytes(plan.DownloadSize()))
	if !canPromptForUpdateConsent() {
		_, _ = fmt.Fprintln(os.Stdout, "Skipping symbol name library update because confirmation is not interactive.")
		return false, nil
	}
	if !promptYesNo(os.Stdin, os.Stdout, "Download updated symbol name library now? [y/N]: ") {
		_, _ = fmt.Fprintln(os.Stdout, "Keeping existing symbol name library.")
		return false, nil
	}
	return false, downloadSymbolNameDatabaseStartupWithRetry(appDir, "Downloading and preparing updated symbol name library", dbPath, plan, false)
}

func downloadSymbolNameDatabaseStartupWithRetry(appDir string, label string, dbPath string, plan labelname.GeneInfoInstallPlan, allowUse bool) error {
	for {
		writeStartupStatus(appDir, startupstate.StatusDownloading, allowUse, label, dbPath)
		err := downloadSymbolNameDatabaseWithProgress(appDir, label, dbPath, plan, allowUse)
		if err == nil {
			return nil
		}
		_, _ = fmt.Fprintln(os.Stdout)
		_, _ = fmt.Fprintf(os.Stdout, "Symbol name library download/build failed: %v\n", err)
		if !canPromptForUpdateConsent() {
			return err
		}
		switch promptRetrySkip(os.Stdin, os.Stdout, "Retry download/build, or skip startup install? [r/S]: ") {
		case "retry":
			continue
		default:
			_, _ = fmt.Fprintln(os.Stdout, "Skipping startup symbol name library install. It will be required when symbol names are used.")
			return nil
		}
	}
}

func downloadSymbolNameDatabaseInteractiveFirstAttemptWithRetry(appDir string, args []string, dbPath string, plan labelname.GeneInfoInstallPlan, label string, prompt string) (bool, error) {
	launchNow, err := chooseLaunchWhileDownloading(appDir, args, dbPath, plan, label, prompt)
	if err == nil {
		return launchNow, nil
	}
	_, _ = fmt.Fprintln(os.Stdout)
	_, _ = fmt.Fprintf(os.Stdout, "Symbol name library download/build failed: %v\n", err)
	if !canPromptForUpdateConsent() {
		return launchNow, err
	}
	for {
		switch promptRetrySkip(os.Stdin, os.Stdout, "Retry download/build, or skip startup install? [r/S]: ") {
		case "retry":
			retryErr := downloadSymbolNameDatabaseStartupWithRetry(appDir, label, dbPath, plan, launchNow)
			return launchNow, retryErr
		default:
			_, _ = fmt.Fprintln(os.Stdout, "Skipping startup symbol name library install. It will be required when symbol names are used.")
			return launchNow, nil
		}
	}
}

func chooseLaunchWhileDownloading(appDir string, args []string, dbPath string, plan labelname.GeneInfoInstallPlan, label string, prompt string) (bool, error) {
	type progressState struct {
		line string
		done bool
		err  error
	}
	progress := newConsoleProgress(os.Stdout)
	reader := bufio.NewReader(os.Stdin)
	downloadCtx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	stateCh := make(chan progressState, 64)
	doneCh := make(chan error, 1)
	var latest progressState
	progressVisible := false
	setState := func(state progressState) {
		select {
		case stateCh <- state:
		default:
		}
	}
	go func() {
		err := plan.Install(downloadCtx, dbPath, labelname.DownloadOptions{
			Workers: labelname.DefaultDownloadWorkers(),
			Stdout:  os.Stdout,
			Progress: func(event labelname.GeneInfoProgress) {
				message := labelname.FormatGeneInfoProgress(event)
				setState(progressState{line: message, done: event.Done})
				writeStartupStatus(appDir, startupstate.StatusDownloading, false, message, dbPath)
			},
		})
		doneCh <- err
	}()

	writeStartupStatus(appDir, startupstate.StatusDownloading, false, label, dbPath)
	_, _ = fmt.Fprintf(os.Stdout, "%s...\n", label)
	_, _ = fmt.Fprintf(os.Stdout, "Writing database to: %s\n", dbPath)
	showPrompt := func() {
		_, _ = fmt.Fprintf(os.Stdout, "%s ", startupDownloadPromptText(prompt))
	}
	showPrompt()

	answerCh := make(chan string, 1)
	go func() {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			answerCh <- ""
			return
		}
		answerCh <- strings.TrimSpace(strings.ToLower(line))
	}()

	launchNow := false
	for {
		select {
		case state := <-stateCh:
			latest = state
			if progressVisible {
				progress.Update(state.line, state.done)
			}
		case answer := <-answerCh:
			switch answer {
			case "y", "yes":
				launchNow = true
				progressVisible = true
				writeStartupStatus(appDir, startupstate.StatusDownloading, true, "Symbol name library is downloading.", dbPath)
				_, _ = fmt.Fprintln(os.Stdout)
				if strings.TrimSpace(latest.line) != "" {
					progress.Update(latest.line, latest.done)
				}
				_, _ = fmt.Fprintln(os.Stdout, "Opening phytozome GO in a new tab while download continues in tab 0...")
				if err := launchMainProgramInNewTab(args); err != nil {
					return false, err
				}
			case "", "n", "no":
				progressVisible = true
				_, _ = fmt.Fprintln(os.Stdout)
				if strings.TrimSpace(latest.line) != "" {
					progress.Update(latest.line, latest.done)
				}
			default:
				_, _ = fmt.Fprintln(os.Stdout)
				_, _ = fmt.Fprintln(os.Stdout, "Please enter y or n.")
				showPrompt()
				go func() {
					line, err := reader.ReadString('\n')
					if err != nil && !errors.Is(err, io.EOF) {
						answerCh <- ""
						return
					}
					answerCh <- strings.TrimSpace(strings.ToLower(line))
				}()
			}
		case err := <-doneCh:
			progress.Finish("Symbol name library download/build complete.", err == nil)
			if err != nil {
				return launchNow, err
			}
			notifyaudio.PlayStartup()
			return launchNow, nil
		}
	}
}

func startupDownloadPromptText(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "Downloading in background."
	}
	return "Downloading in background. " + prompt
}

func downloadSymbolNameDatabaseWithProgress(appDir string, label string, dbPath string, plan labelname.GeneInfoInstallPlan, allowUse bool) error {
	_, _ = fmt.Fprintf(os.Stdout, "%s...\n", label)
	_, _ = fmt.Fprintf(os.Stdout, "Writing database to: %s\n", dbPath)
	progress := newConsoleProgress(os.Stdout)
	downloadCtx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	err := plan.Install(downloadCtx, dbPath, labelname.DownloadOptions{
		Workers: labelname.DefaultDownloadWorkers(),
		Stdout:  os.Stdout,
		Progress: func(event labelname.GeneInfoProgress) {
			message := labelname.FormatGeneInfoProgress(event)
			progress.Update(message, event.Done)
			writeStartupStatus(appDir, startupstate.StatusDownloading, allowUse, message, dbPath)
		},
	})
	progress.Finish("Symbol name library download/build complete.", err == nil)
	if err == nil {
		notifyaudio.PlayStartup()
	}
	return err
}

func applicationDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate helper executable: %w", err)
	}
	return filepath.Dir(exePath), nil
}

func promptRetrySkip(input io.Reader, output io.Writer, label string) string {
	reader := bufio.NewReader(input)
	for {
		_, _ = fmt.Fprint(output, label)
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "skip"
		}
		answer := strings.TrimSpace(strings.ToLower(line))
		switch answer {
		case "r", "retry":
			return "retry"
		case "", "s", "skip":
			return "skip"
		}
		_, _ = fmt.Fprintln(output, "Please enter r or s.")
		if err == io.EOF {
			return "skip"
		}
	}
}

type consoleProgress struct {
	output io.Writer
	last   string
	open   bool
}

func newConsoleProgress(output io.Writer) *consoleProgress {
	return &consoleProgress{output: output}
}

func (p *consoleProgress) Update(line string, done bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	p.last = line
	_, _ = fmt.Fprintf(p.output, "\r\033[2K%s", line)
	if done {
		_, _ = fmt.Fprintln(p.output)
		p.open = false
		return
	}
	p.open = true
}

func (p *consoleProgress) Finish(successMessage string, ok bool) {
	if p.open {
		_, _ = fmt.Fprintln(p.output)
	}
	p.open = false
	if ok && strings.TrimSpace(successMessage) != "" {
		_, _ = fmt.Fprintln(p.output, successMessage)
	}
	p.last = ""
}

func humanBytes(size int64) string {
	if size <= 0 {
		return "unknown"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	value := float64(size)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", size, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

func firstNonEmptyText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func resolveMainProgramPathFrom(cleanerPath string) (string, error) {
	cleanerPath = strings.TrimSpace(cleanerPath)
	if cleanerPath == "" {
		return "", fmt.Errorf("helper path is empty")
	}
	mainPath := filepath.Join(filepath.Dir(cleanerPath), mainProgramName)
	info, err := os.Stat(mainPath)
	if err != nil {
		return "", fmt.Errorf("could not locate phytozome GO core program next to the helper:\n%s", mainPath)
	}
	if info.IsDir() {
		return "", fmt.Errorf("phytozome GO main program path is a directory:\n%s", mainPath)
	}
	return mainPath, nil
}

func shouldSpawnMainProgramInNewTab() bool {
	return strings.TrimSpace(os.Getenv("WEZTERM_PANE")) != ""
}

func buildWezTermSpawnArgs(mainPath string, args []string) []string {
	mainPath = strings.TrimSpace(mainPath)
	spawnArgs := []string{
		"cli",
		"spawn",
		"--cwd", filepath.Dir(mainPath),
		"--",
		mainPath,
	}
	for _, arg := range args {
		if strings.TrimSpace(arg) == "" {
			continue
		}
		spawnArgs = append(spawnArgs, arg)
	}
	return spawnArgs
}

func samePath(a string, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func fatal(err error) {
	_, _ = os.Stderr.WriteString(err.Error() + "\n")
	os.Exit(1)
}

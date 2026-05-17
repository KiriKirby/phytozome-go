package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const mainProgramName = "phytozome-go.bin"
const windowsWezTermCLIName = "wezterm-cli.bin"
const displayName = "phytozome GO"
const author = "wangsychn"
const repoURL = "https://github.com/KiriKirby/phytozome-go"
const licenseName = "Common Public Attribution License 1.0"
const licenseID = "CPAL-1.0"

var version = "dev"

func main() {
	if err := run(); err != nil {
		fatal(err)
	}
}

func run() error {
	printStartupNotice()

	cacheTargets, err := resolveCacheTargets()
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(os.Stdout)
	_, _ = fmt.Fprintln(os.Stdout, "Cache cleanup targets:")
	for _, target := range cacheTargets {
		_, _ = fmt.Fprintf(os.Stdout, "  - %s\n", target)
	}

	if err := runSpinner("Deleting .cache directories", func() error {
		return removeCacheTargets(cacheTargets)
	}); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(os.Stdout, "Cache cleanup complete.")
	if shouldSpawnMainProgramInNewTab() {
		_, _ = fmt.Fprintln(os.Stdout, "Opening phytozome GO in a new tab...")
		return launchMainProgramInNewTab(os.Args[1:])
	}
	_, _ = fmt.Fprintln(os.Stdout, "Starting phytozome GO...")
	return launchMainProgramDirect(os.Args[1:])
}

func printStartupNotice() {
	_, _ = fmt.Fprintf(os.Stdout, "%s %s\n", displayName, version)
	_, _ = fmt.Fprintf(os.Stdout, "Author: %s\n", author)
	_, _ = fmt.Fprintf(os.Stdout, "Repo:   %s\n", repoURL)
	_, _ = fmt.Fprintf(os.Stdout, "License: %s (%s)\n", licenseName, licenseID)
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
		return fmt.Errorf("locate cache cleaner executable: %w", err)
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

func resolveMainProgramPath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate cache cleaner executable: %w", err)
	}
	return resolveMainProgramPathFrom(exePath)
}

func resolveCacheTargets() ([]string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate cache cleaner executable: %w", err)
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
		return "", fmt.Errorf("cache cleaner path is empty")
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
	return "", fmt.Errorf("could not locate WezTerm CLI next to the cache cleaner:\n%s", dir)
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
			return fmt.Errorf("delete cache directory %s: %w", target, err)
		}
	}
	return nil
}

func resolveMainProgramPathFrom(cleanerPath string) (string, error) {
	cleanerPath = strings.TrimSpace(cleanerPath)
	if cleanerPath == "" {
		return "", fmt.Errorf("cache cleaner path is empty")
	}
	mainPath := filepath.Join(filepath.Dir(cleanerPath), mainProgramName)
	info, err := os.Stat(mainPath)
	if err != nil {
		return "", fmt.Errorf("could not locate phytozome GO main program next to the cache cleaner:\n%s", mainPath)
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

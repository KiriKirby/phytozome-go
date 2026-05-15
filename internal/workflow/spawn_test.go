package workflow

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWeztermSpawnArgsIncludeCLICommandAndMainPath(t *testing.T) {
	t.Setenv("WEZTERM_PANE", "1")

	args := weztermSpawnArgs("run-1", "2.3", filepath.Join("cache", "handoff.json"))
	if len(args) < 5 {
		t.Fatalf("weztermSpawnArgs returned too few args: %v", args)
	}
	if args[0] != "cli" || args[1] != "spawn" {
		t.Fatalf("weztermSpawnArgs prefix = %v, want cli spawn", args[:2])
	}
	if args[2] != "--cwd" {
		t.Fatalf("weztermSpawnArgs missing --cwd: %v", args)
	}
	if args[4] != "--" {
		t.Fatalf("weztermSpawnArgs missing command separator: %v", args)
	}
	foundRunID := false
	foundInstanceID := false
	foundHandoff := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--instance-run-id" && args[i+1] == "run-1" {
			foundRunID = true
		}
		if args[i] == "--instance-id" && args[i+1] == "2.3" {
			foundInstanceID = true
		}
		if args[i] == "--handoff" && args[i+1] == filepath.Join("cache", "handoff.json") {
			foundHandoff = true
		}
	}
	if !foundRunID || !foundInstanceID || !foundHandoff {
		t.Fatalf("weztermSpawnArgs missing expected metadata: %v", args)
	}
}

func TestWeztermCLIPathPrefersBundledCandidates(t *testing.T) {
	tempDir := t.TempDir()
	exeName := "phytozome-go.bin"
	if runtime.GOOS == "windows" {
		exeName = "phytozome-go.exe"
	}
	exePath := filepath.Join(tempDir, exeName)
	if err := os.WriteFile(exePath, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write executable stub: %v", err)
	}

	var expected string
	if runtime.GOOS == "windows" {
		expected = filepath.Join(tempDir, "wezterm-cli.bin")
	} else {
		expected = filepath.Join(tempDir, "wezterm")
	}
	if err := os.WriteFile(expected, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write wezterm stub: %v", err)
	}

	got, err := resolveWezTermCLIPathFromExecutable(exePath)
	if err != nil {
		t.Fatalf("resolveWezTermCLIPathFromExecutable returned error: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(expected) {
		t.Fatalf("resolveWezTermCLIPathFromExecutable = %q, want %q", got, expected)
	}
}

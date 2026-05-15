package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolveMainProgramPathFromFindsSiblingBinary(t *testing.T) {
	tmp := t.TempDir()
	cleanerPath := filepath.Join(tmp, "phytozome-go-cleancache.bin")
	mainPath := filepath.Join(tmp, mainProgramName)

	if err := os.WriteFile(cleanerPath, []byte("cleaner"), 0o644); err != nil {
		t.Fatalf("write cleaner: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte("main"), 0o644); err != nil {
		t.Fatalf("write main binary: %v", err)
	}

	got, err := resolveMainProgramPathFrom(cleanerPath)
	if err != nil {
		t.Fatalf("resolveMainProgramPathFrom returned error: %v", err)
	}
	if got != mainPath {
		t.Fatalf("resolveMainProgramPathFrom = %q, want %q", got, mainPath)
	}
}

func TestResolveMainProgramPathFromErrorsWhenSiblingBinaryMissing(t *testing.T) {
	tmp := t.TempDir()
	cleanerPath := filepath.Join(tmp, "phytozome-go-cleancache.bin")

	if err := os.WriteFile(cleanerPath, []byte("cleaner"), 0o644); err != nil {
		t.Fatalf("write cleaner: %v", err)
	}

	_, err := resolveMainProgramPathFrom(cleanerPath)
	if err == nil {
		t.Fatal("resolveMainProgramPathFrom returned nil error, want missing binary error")
	}
}

func TestResolveCacheTargetsFromIncludesBundleAndWorkingDirectory(t *testing.T) {
	tmp := t.TempDir()
	repoRoot := filepath.Join(tmp, "repo")
	bundleDir := filepath.Join(repoRoot, "bin", "phytozome-go_windows_amd64_wezterm")
	cleanerPath := filepath.Join(bundleDir, "phytozome-go-cleancache.bin")
	workingDir := filepath.Join(tmp, "separate-workdir")

	for _, dir := range []string{repoRoot, bundleDir, workingDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	got := resolveCacheTargetsFrom(cleanerPath, workingDir)
	want := []string{
		filepath.Join(repoRoot, ".cache"),
		filepath.Join(bundleDir, ".cache"),
		filepath.Join(workingDir, ".cache"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveCacheTargetsFrom = %#v, want %#v", got, want)
	}
}

func TestResolveCacheTargetsFromDeduplicatesSameDirectory(t *testing.T) {
	tmp := t.TempDir()
	cleanerPath := filepath.Join(tmp, "phytozome-go-cleancache.bin")

	got := resolveCacheTargetsFrom(cleanerPath, tmp)
	want := []string{filepath.Join(tmp, ".cache")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveCacheTargetsFrom = %#v, want %#v", got, want)
	}
}

func TestResolveWezTermCLIPathFromFindsSiblingCLI(t *testing.T) {
	tmp := t.TempDir()
	cleanerPath := filepath.Join(tmp, "phytozome-go-cleancache.bin")
	cliPath := filepath.Join(tmp, windowsWezTermCLIName)

	if err := os.WriteFile(cleanerPath, []byte("cleaner"), 0o644); err != nil {
		t.Fatalf("write cleaner: %v", err)
	}
	if err := os.WriteFile(cliPath, []byte("cli"), 0o644); err != nil {
		t.Fatalf("write cli: %v", err)
	}

	got, err := resolveWezTermCLIPathFrom(cleanerPath)
	if err != nil {
		t.Fatalf("resolveWezTermCLIPathFrom returned error: %v", err)
	}
	if got != cliPath {
		t.Fatalf("resolveWezTermCLIPathFrom = %q, want %q", got, cliPath)
	}
}

func TestBuildWezTermSpawnArgsUsesMainPathAndAppArgs(t *testing.T) {
	mainPath := filepath.Join(`C:\bundle`, mainProgramName)
	got := buildWezTermSpawnArgs(mainPath, []string{"--instance-id", "1", "", "blast"})
	want := []string{
		"cli",
		"spawn",
		"--cwd", filepath.Dir(mainPath),
		"--",
		mainPath,
		"--instance-id",
		"1",
		"blast",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildWezTermSpawnArgs = %#v, want %#v", got, want)
	}
}

// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package blastplus

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplicationDirUsesExecutableDirWhenStable(t *testing.T) {
	oldExecutableFn := executableFn
	oldGetwdFn := getwdFn
	oldTempDirFn := tempDirFn
	t.Cleanup(func() {
		executableFn = oldExecutableFn
		getwdFn = oldGetwdFn
		tempDirFn = oldTempDirFn
	})

	executableFn = func() (string, error) { return `C:\tools\phytozome-go\phytozome-go.exe`, nil }
	getwdFn = func() (string, error) { return `C:\workspace`, nil }
	tempDirFn = func() string { return `C:\Users\me\AppData\Local\Temp` }

	dir, err := applicationDir()
	if err != nil {
		t.Fatalf("applicationDir returned error: %v", err)
	}
	if dir != `C:\tools\phytozome-go` {
		t.Fatalf("unexpected application dir: got %q", dir)
	}
}

func TestApplicationDirFallsBackToWorkingDirForTempExecutable(t *testing.T) {
	oldExecutableFn := executableFn
	oldGetwdFn := getwdFn
	oldTempDirFn := tempDirFn
	t.Cleanup(func() {
		executableFn = oldExecutableFn
		getwdFn = oldGetwdFn
		tempDirFn = oldTempDirFn
	})

	executableFn = func() (string, error) { return `C:\Users\me\AppData\Local\Temp\go-build123\phytozome-go.exe`, nil }
	getwdFn = func() (string, error) { return `C:\workspace`, nil }
	tempDirFn = func() string { return `C:\Users\me\AppData\Local\Temp` }

	dir, err := applicationDir()
	if err != nil {
		t.Fatalf("applicationDir returned error: %v", err)
	}
	if dir != `C:\workspace` {
		t.Fatalf("unexpected fallback application dir: got %q", dir)
	}
}

func TestEnsureToolsOnPathCachesSuccessfulLookups(t *testing.T) {
	oldExecLookPath := execLookPath
	oldCache := toolsOnPathCache
	t.Cleanup(func() {
		execLookPath = oldExecLookPath
		toolsOnPathMu.Lock()
		toolsOnPathCache = oldCache
		toolsOnPathMu.Unlock()
	})

	t.Setenv("PATH", "test-path-a")
	toolsOnPathMu.Lock()
	toolsOnPathCache = make(map[string]struct{})
	toolsOnPathMu.Unlock()
	calls := 0
	execLookPath = func(file string) (string, error) {
		calls++
		return file, nil
	}

	if err := EnsureToolsOnPath("makeblastdb", "blastp"); err != nil {
		t.Fatalf("first EnsureToolsOnPath: %v", err)
	}
	if err := EnsureToolsOnPath("blastp", "makeblastdb"); err != nil {
		t.Fatalf("cached EnsureToolsOnPath: %v", err)
	}
	if calls != 2 {
		t.Fatalf("lookup calls = %d, want 2 after cached repeat", calls)
	}

	t.Setenv("PATH", "test-path-b")
	if err := EnsureToolsOnPath("makeblastdb", "blastp"); err != nil {
		t.Fatalf("PATH-changed EnsureToolsOnPath: %v", err)
	}
	if calls != 4 {
		t.Fatalf("lookup calls after PATH change = %d, want 4", calls)
	}
}

func TestEnsureToolsOnPathDoesNotCacheFailures(t *testing.T) {
	oldExecLookPath := execLookPath
	oldCache := toolsOnPathCache
	t.Cleanup(func() {
		execLookPath = oldExecLookPath
		toolsOnPathMu.Lock()
		toolsOnPathCache = oldCache
		toolsOnPathMu.Unlock()
	})

	t.Setenv("PATH", "test-path-fail")
	toolsOnPathMu.Lock()
	toolsOnPathCache = make(map[string]struct{})
	toolsOnPathMu.Unlock()
	calls := 0
	execLookPath = func(file string) (string, error) {
		calls++
		return "", fmt.Errorf("missing %s", file)
	}

	if err := EnsureToolsOnPath("makeblastdb"); err == nil {
		t.Fatal("expected missing tool error")
	}
	if err := EnsureToolsOnPath("makeblastdb"); err == nil {
		t.Fatal("expected repeated missing tool error")
	}
	if calls != 2 {
		t.Fatalf("failed lookup calls = %d, want 2 because failures are not cached", calls)
	}
}

func TestSafeArchivePathRejectsEscapes(t *testing.T) {
	target := filepath.Clean(t.TempDir())
	unsafeNames := []string{"../escape.txt", "/absolute/escape.txt", "C:/escape.txt"}
	for _, name := range unsafeNames {
		if got, err := safeArchivePath(target, name); err == nil {
			t.Fatalf("safeArchivePath(%q) = %q, nil error", name, got)
		}
	}
	got, err := safeArchivePath(target, "ncbi-blast/bin/makeblastdb.exe")
	if err != nil {
		t.Fatalf("safeArchivePath rejected valid entry: %v", err)
	}
	if !strings.HasPrefix(filepath.Clean(got), target) {
		t.Fatalf("safeArchivePath returned path outside target: %q", got)
	}
}

func TestExtractTarGzRejectsPathTraversal(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "bad.tar.gz")
	if err := writeTestTarGz(archive, "../escape.txt", []byte("bad")); err != nil {
		t.Fatalf("write test archive: %v", err)
	}
	target := t.TempDir()
	err := extractTarGz(context.Background(), archive, target)
	if err == nil {
		t.Fatal("expected path traversal archive to be rejected")
	}
	if !strings.Contains(err.Error(), "refusing to extract") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(target, "..", "escape.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("archive wrote outside target, stat err=%v", statErr)
	}
}

func writeTestTarGz(path string, name string, data []byte) error {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
		return err
	}
	if _, err := tw.Write(data); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

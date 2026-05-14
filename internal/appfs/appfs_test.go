package appfs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestWriteFileAtomicConcurrentWritersLeaveCompleteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	values := [][]byte{
		[]byte(`{"value":"first"}`),
		[]byte(`{"value":"second"}`),
		[]byte(`{"value":"third"}`),
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		data := values[i%len(values)]
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := WriteFileAtomic(path, data, 0o644); err != nil {
				t.Errorf("WriteFileAtomic: %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read final file: %v", err)
	}
	for _, value := range values {
		if string(got) == string(value) {
			return
		}
	}
	t.Fatalf("final file is not one complete write: %q", got)
}

func TestCachePathRejectsEscapingComponents(t *testing.T) {
	root := t.TempDir()
	unsafeParts := []string{"..", "../escape", filepath.Join("safe", "..", "..", "escape")}
	if runtime.GOOS == "windows" {
		unsafeParts = append(unsafeParts, `..\escape`)
	}
	for _, part := range unsafeParts {
		if _, err := cachePath(root, part); err == nil {
			t.Fatalf("cachePath accepted unsafe component %q", part)
		}
	}
	if _, err := cachePath(root, filepath.Join("safe", "child")); err != nil {
		t.Fatalf("cachePath rejected safe nested component: %v", err)
	}
}

func TestRemoveCacheSubtreeRejectsRootDeletion(t *testing.T) {
	err := RemoveCacheSubtree("")
	if err == nil {
		t.Fatal("expected root deletion to be rejected")
	}
	if !strings.Contains(err.Error(), "entire cache root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

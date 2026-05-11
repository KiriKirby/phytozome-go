package perf_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionCodeUsesPerformanceEntryPoints(t *testing.T) {
	root := moduleRoot(t)
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "bin", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		bodyBytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		body := string(bodyBytes)
		if rel != "internal/perf/perf.go" {
			if strings.Contains(body, "http.DefaultClient") || strings.Contains(body, "http.Client{") || strings.Contains(body, "&http.Client{") {
				violations = append(violations, rel+": use perf.HTTPClient instead of direct HTTP clients")
			}
		}
		if strings.Contains(body, "sync.WaitGroup") && !allowedManualParallelFile(rel) {
			violations = append(violations, rel+": use perf.ParallelFor or document a recovery-aware exception")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("performance guardrail violations:\n%s", strings.Join(violations, "\n"))
	}
}

func allowedManualParallelFile(rel string) bool {
	switch rel {
	case "internal/perf/parallel.go":
		return true
	case "internal/lemna/lemna.go":
		return true
	case "internal/phytozome/species.go":
		return true
	case "internal/workflow/blast.go":
		return true
	case "internal/tui/templates.go":
		return true
	case "internal/tui/console_windows.go":
		return true
	default:
		return false
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func mustRel(t *testing.T, base string, path string) string {
	t.Helper()
	rel, err := filepath.Rel(base, path)
	if err != nil {
		t.Fatal(err)
	}
	return rel
}

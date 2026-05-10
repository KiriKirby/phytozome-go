package blastplus

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/require"
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

func TestFetchGrabTextUsesRegisteredHTTPResponder(t *testing.T) {
	httpmock.Activate()
	t.Cleanup(httpmock.DeactivateAndReset)
	httpmock.RegisterResponder("GET", "https://example.test/latest/",
		httpmock.NewStringResponder(200, `<a href="ncbi-blast-2.17.0+-x64-win64.tar.gz">archive</a>`))

	body, err := fetchGrabText(context.Background(), &http.Client{Transport: httpmock.DefaultTransport}, "https://example.test/latest/")
	require.NoError(t, err)
	require.Empty(t, cmp.Diff(`<a href="ncbi-blast-2.17.0+-x64-win64.tar.gz">archive</a>`, string(body)))
}

func TestUniquePartPathAvoidsSharedPartFile(t *testing.T) {
	base := filepath.Join(t.TempDir(), "archive.tar.gz")
	first := uniquePartPath(base)
	second := uniquePartPath(base)
	if first == second {
		t.Fatalf("uniquePartPath returned duplicate path %q", first)
	}
	if !strings.HasPrefix(first, base+".") || !strings.HasSuffix(first, ".part") {
		t.Fatalf("unexpected part path: %q", first)
	}
}

func TestRenameWithRetryRemovesTempOnFailure(t *testing.T) {
	dir := t.TempDir()
	tmp := filepath.Join(dir, "tmp.part")
	if err := os.WriteFile(tmp, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := renameWithRetry(context.Background(), tmp, filepath.Join(dir, "missing", "archive.tar.gz"))
	if err == nil {
		t.Fatal("expected rename error")
	}
	if _, statErr := os.Stat(tmp); !os.IsNotExist(statErr) {
		t.Fatalf("temp file should be removed after failed rename, stat err=%v", statErr)
	}
}

package phygoboost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBusinessCodeDoesNotCallLegacyKindBasedPhygoboostEntrypoints(t *testing.T) {
	root := repoRootFromPhygoboostTest(t)
	targets := []string{
		filepath.Join(root, "internal", "workflow"),
		filepath.Join(root, "internal", "lemna"),
		filepath.Join(root, "internal", "phytozome"),
		filepath.Join(root, "internal", "export"),
		filepath.Join(root, "internal", "report"),
	}
	banned := []string{
		"phygoboost.Go(",
		"phygoboost.ParallelFor(",
		"phygoboost.ParallelForWithWorkers(",
		"phygoboost.RunJSON(",
		"phygoboost.RunSingletonJSON(",
		"phygoboost.Run(",
		"phygoboost.RunSingleton(",
		"phygoboost.RunInteractiveJSON(",
		"phygoboost.RunInteractive(",
		"phygoboost.StartInteractive(",
	}
	for _, dir := range targets {
		visitSourceFiles(t, dir, func(path string, body string) {
			for _, marker := range banned {
				if strings.Contains(body, marker) {
					t.Fatalf("legacy phygoboost entrypoint %q found in %s", marker, path)
				}
			}
		})
	}
}

func TestProductionCodeDoesNotUseHTTPDefaultClientDirectly(t *testing.T) {
	root := repoRootFromPhygoboostTest(t)
	targets := []string{
		filepath.Join(root, "internal"),
	}
	for _, dir := range targets {
		visitSourceFiles(t, dir, func(path string, body string) {
			if strings.HasSuffix(path, "_test.go") {
				return
			}
			if strings.Contains(body, "http.DefaultClient") {
				t.Fatalf("http.DefaultClient found in production code: %s", path)
			}
		})
	}
}

func TestRunProcessCallersAlwaysProvideTaskSpec(t *testing.T) {
	root := repoRootFromPhygoboostTest(t)
	targets := []string{
		filepath.Join(root, "internal"),
		filepath.Join(root, "cmd"),
	}
	required := []string{
		"phygoboost.ProcessSpec{",
		"Task:",
	}
	for _, dir := range targets {
		visitSourceFiles(t, dir, func(path string, body string) {
			if strings.HasSuffix(path, "_test.go") {
				return
			}
			if !strings.Contains(body, "phygoboost.RunProcess(") {
				return
			}
			start := 0
			for {
				index := strings.Index(body[start:], "phygoboost.ProcessSpec{")
				if index < 0 {
					break
				}
				index += start
				end := strings.Index(body[index:], "})")
				if end < 0 {
					end = len(body) - index
				}
				block := body[index : index+end]
				for _, marker := range required {
					if !strings.Contains(block, marker) {
						t.Fatalf("RunProcess ProcessSpec in %s is missing %q", path, marker)
					}
				}
				start = index + len("phygoboost.ProcessSpec{")
			}
		})
	}
}

func TestBusinessCodeDoesNotCallObserveTaskSpecDirectly(t *testing.T) {
	root := repoRootFromPhygoboostTest(t)
	targets := []string{
		filepath.Join(root, "internal", "workflow"),
		filepath.Join(root, "internal", "lemna"),
		filepath.Join(root, "internal", "phytozome"),
		filepath.Join(root, "internal", "blastplus"),
		filepath.Join(root, "internal", "export"),
		filepath.Join(root, "internal", "report"),
		filepath.Join(root, "internal", "tui"),
	}
	for _, dir := range targets {
		visitSourceFiles(t, dir, func(path string, body string) {
			if strings.Contains(body, "phygoboost.ObserveTaskSpec(") {
				t.Fatalf("business code should use explicit phygoboost runtime entrypoints instead of ObserveTaskSpec directly: %s", path)
			}
		})
	}
}

func TestLocalBlastExportedEntrypointsUseTaskSpecs(t *testing.T) {
	root := repoRootFromPhygoboostTest(t)
	path := filepath.Join(root, "internal", "lemna", "localblast.go")
	bodyBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	body := string(bodyBytes)
	requiredMarkers := []string{
		`func LocalBlastRun(ctx context.Context, c *Client, req model.BlastRequest) (model.BlastJob, error) {`,
		`Description: "run lemna local blast"`,
		`func NewLocalBlastRunner(ctx context.Context, c *Client, req model.BlastRequest) (*LocalBlastRunner, error) {`,
		`Description: "create lemna local blast runner"`,
		`func PrepareLocalBlast(ctx context.Context, c *Client, req model.BlastRequest) error {`,
		`Description: "prepare lemna local blast"`,
	}
	for _, marker := range requiredMarkers {
		if !strings.Contains(body, marker) {
			t.Fatalf("expected local blast exported entrypoint marker %q in %s", marker, path)
		}
	}
}

func TestKeywordEngineExportedEntrypointsUseTaskSpecs(t *testing.T) {
	root := repoRootFromPhygoboostTest(t)
	checks := []struct {
		path    string
		markers []string
	}{
		{
			path: filepath.Join(root, "internal", "searchengine", "lemnakeyword", "engine.go"),
			markers: []string{
				`func (e *Engine) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {`,
				`Description: "search lemna keyword engine rows"`,
				`func (e *Engine) SearchKeywordRowsWide(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {`,
				`Description: "search lemna keyword engine rows wide"`,
				`func (e *Engine) SearchKeywordRowsBroad(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {`,
				`Description: "search lemna keyword engine rows broad"`,
			},
		},
		{
			path: filepath.Join(root, "internal", "searchengine", "phytozomekeyword", "engine.go"),
			markers: []string{
				`func (e *Engine) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {`,
				`Description: "search phytozome keyword engine rows"`,
				`func (e *Engine) SearchKeywordRowsWide(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {`,
				`Description: "search phytozome keyword engine rows wide"`,
			},
		},
	}
	for _, check := range checks {
		bodyBytes, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatalf("read %s: %v", check.path, err)
		}
		body := string(bodyBytes)
		for _, marker := range check.markers {
			if !strings.Contains(body, marker) {
				t.Fatalf("expected keyword engine entrypoint marker %q in %s", marker, check.path)
			}
		}
	}
}

func TestSourceAndReferenceExportedEntrypointsUseExpectedTaskSpecs(t *testing.T) {
	root := repoRootFromPhygoboostTest(t)
	checks := []struct {
		path    string
		markers []string
	}{
		{
			path: filepath.Join(root, "internal", "phytozome", "species.go"),
			markers: []string{
				`func (c *Client) FetchSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {`,
				`Level:       phygoboost.ExecHeavy,`,
				`Description: "fetch phytozome species candidates"`,
				`Description: "load phytozome species metadata"`,
				`Description: "scan phytozome homepage bundles"`,
			},
		},
		{
			path: filepath.Join(root, "internal", "lemna", "lemna.go"),
			markers: []string{
				`func (c *Client) DetectBlastCapabilities(ctx context.Context, species model.SpeciesCandidate) (BlastCapability, error) {`,
				`Description: "detect lemna blast capabilities"`,
				`func (c *Client) AvailableBlastPrograms(ctx context.Context, species model.SpeciesCandidate) []string {`,
				`Description: "list lemna blast programs"`,
				`func (c *Client) FetchSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {`,
				`Description: "fetch lemna species candidates"`,
				`func (c *Client) SubmitBlast(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {`,
				`Description: "submit lemna blast"`,
				`func (c *Client) SubmitBlastServerOnly(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {`,
				`Description: "submit lemna blast server only"`,
				`func (c *Client) WaitForBlastResults(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, error) {`,
				`Description: "wait lemna blast results"`,
				`Description: "inspect lemna blast capability"`,
				`Description: "inspect lemna species downloads"`,
				`Description: "fetch lemna text"`,
				`Description: "open lemna remote data stream"`,
				`Level:       phygoboost.ExecHeavy,`,
			},
		},
		{
			path: filepath.Join(root, "internal", "lemna", "localblast.go"),
			markers: []string{
				`Description: "warm local blast references"`,
				`Description: "download lemna local blast file"`,
				`Level: phygoboost.ExecHeavy`,
			},
		},
		{
			path: filepath.Join(root, "internal", "blastplus", "install.go"),
			markers: []string{
				`Description: "fetch blastplus text"`,
				`Description: "download blastplus archive"`,
				`Level: phygoboost.ExecHeavy`,
			},
		},
		{
			path: filepath.Join(root, "internal", "searchengine", "lemnakeyword", "engine.go"),
			markers: []string{
				`func (e *Engine) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {`,
				`func (e *Engine) SearchKeywordRowsWide(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {`,
				`func (e *Engine) SearchKeywordRowsBroad(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {`,
				`Level:       phygoboost.ExecHeavy,`,
			},
		},
		{
			path: filepath.Join(root, "internal", "searchengine", "phytozomekeyword", "engine.go"),
			markers: []string{
				`func (e *Engine) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {`,
				`func (e *Engine) SearchKeywordRowsWide(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {`,
				`Level:       phygoboost.ExecHeavy,`,
			},
		},
		{
			path: filepath.Join(root, "internal", "uniprot", "uniprot.go"),
			markers: []string{
				`func (c *Client) Lookup(ctx context.Context, accession string, row model.BlastResultRow) (Entry, bool, error) {`,
				`Level:       phygoboost.ExecHeavy,`,
				`Description: "lookup uniprot entry"`,
			},
		},
		{
			path: filepath.Join(root, "internal", "interpro", "interpro.go"),
			markers: []string{
				`func (c *Client) Lookup(ctx context.Context, accession string) (Entry, bool, error) {`,
				`Level:       phygoboost.ExecHeavy,`,
				`Description: "lookup interpro entry"`,
			},
		},
	}
	for _, check := range checks {
		bodyBytes, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatalf("read %s: %v", check.path, err)
		}
		body := string(bodyBytes)
		for _, marker := range check.markers {
			if !strings.Contains(body, marker) {
				t.Fatalf("expected source/reference entrypoint marker %q in %s", marker, check.path)
			}
		}
	}
}

func repoRootFromPhygoboostTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolve repo root from %s: %v", wd, err)
	}
	return root
}

func visitSourceFiles(t *testing.T, root string, fn func(path string, body string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		fn(path, string(body))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

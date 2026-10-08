package cyp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func testPGD(t *testing.T, path string) []byte {
	t.Helper()
	db, err := bolt.Open(path, 0o644, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, e := tx.CreateBucket(bucket)
		if e != nil {
			return e
		}
		v, _ := json.Marshal(record{ID: "CYP73A5", Category: "plants", Species: "Arabidopsis thaliana", Symbol: "CYP73A5", Description: "cinnamate 4-hydroxylase", Sequence: "MABCDEFGHIJKLMNOPQRSTVWY"})
		return b.Put([]byte("00000000"), v)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDownloadValidateSpeciesAndCategorySearch(t *testing.T) {
	dir := t.TempDir()
	data := testPGD(t, filepath.Join(dir, "source.pgd"))
	sum := fmt.Sprintf("%x", sha256.Sum256(data))
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			fmt.Fprintf(w, `{"database_url":%q,"sha256":%q,"size":%d}`, server.URL+"/p450phgo.pgd", sum, len(data))
		case "/p450phgo.pgd":
			_, _ = w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("PHGO_CYP_PGD_MANIFEST_URL", server.URL+"/manifest.json")
	c := NewClient(server.Client())
	c.dbPath = filepath.Join(dir, "installed.pgd")
	species, err := c.FetchSpeciesCandidates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(species) != 1 || species[0].GroupKey != "plants" {
		t.Fatalf("species=%#v", species)
	}
	rows, err := c.SearchKeywordRows(context.Background(), species[0], "p: CYP73")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].LabelName != "CYP73A5" {
		t.Fatalf("rows=%#v", rows)
	}
	rows, err = c.SearchKeywordRows(context.Background(), species[0], "a: CYP73")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("animal rows=%#v", rows)
	}
}

func TestManifestFailureUsesValidLocalDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "installed.pgd")
	testPGD(t, path)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "offline", 503) }))
	defer server.Close()
	t.Setenv("PHGO_CYP_PGD_MANIFEST_URL", server.URL)
	c := NewClient(server.Client())
	c.dbPath = path
	if _, err := c.FetchSpeciesCandidates(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPublishedTableS2RecordsSearchByEverySpecies(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "p450phgo.pgd", "p450phgo.pgd"))
	if err != nil {
		t.Skip("published PGD repository checkout is not available")
	}
	path := filepath.Join(t.TempDir(), "installed.pgd")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewClient(http.DefaultClient)
	c.dbPath = path
	want := map[string][]string{
		"Oryza sativa": {"CYP84A6", "CYP84A7"}, "Arabidopsis thaliana": {"CYP84A1"},
		"Liquidambar styraciflua": {"CYP84A3"}, "Populus trichocarpa": {"CYP84A10", "CYP84A11"},
		"Eucalyptus globulus": {"CYP84A-like"}, "Medicago sativa": {"CYP84A20"},
		"Setaria italica": {"CYP84A-like"}, "Zea mays": {"CYP84A33", "CYP84A34"},
		"Sorghum bicolor": {"CYP84A-like1"}, "Brachypodium distachyon": {"CYP84A5"},
		"Panicum virgatum": {"CYP84A-like1", "CYP84A-like2"},
	}
	for species, symbols := range want {
		rows, err := c.SearchKeywordRows(context.Background(), model.SpeciesCandidate{JBrowseName: species}, "CYP84")
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, row := range rows {
			got[row.LabelName] = true
		}
		for _, symbol := range symbols {
			if !got[symbol] {
				t.Errorf("%s missing %s in %#v", species, symbol, got)
			}
		}
	}
}

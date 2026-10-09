package cyp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/source"
	bolt "go.etcd.io/bbolt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const DatabaseFileName = "p450phgo.pgd"
const DefaultManifestURL = "https://raw.githubusercontent.com/KiriKirby/phytozome-go-p450phgo/main/manifest.json"

type manifest struct {
	DatabaseURL string `json:"database_url"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
}

var _ source.DataSource = (*Client)(nil)

type record struct{ ID, RecordKey, Category, Species, Symbol, Description, Sequence, SourceURL string }
type speciesRecord struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Selectable  bool   `json:"selectable"`
	Description string `json:"description"`
}
type Client struct {
	httpClient   *http.Client
	mu           sync.Mutex
	dbPath       string
	cacheMu      sync.RWMutex
	cacheStamp   string
	cacheRows    []record
	cacheSpecies []speciesRecord
}

func NewClient(h *http.Client) *Client {
	if h == nil {
		h = &http.Client{Timeout: 60 * time.Second}
	}
	r, _ := appfs.ApplicationDir()
	return &Client{httpClient: h, dbPath: filepath.Join(r, DatabaseFileName)}
}
func (c *Client) Name() string         { return "cyp" }
func (c *Client) DatabasePath() string { return c.dbPath }

var bucket = []byte("records")

func (c *Client) ensureDB(x context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	u := strings.TrimSpace(os.Getenv("PHGO_CYP_PGD_URL"))
	expected := ""
	expectedSize := int64(0)
	if u == "" {
		murl := strings.TrimSpace(os.Getenv("PHGO_CYP_PGD_MANIFEST_URL"))
		if murl == "" {
			murl = DefaultManifestURL
		}
		m, e := c.fetchManifest(x, murl)
		if e != nil {
			if _, statErr := os.Stat(c.dbPath); statErr == nil {
				return validate(c.dbPath)
			}
			return e
		}
		u = m.DatabaseURL
		expected = m.SHA256
		expectedSize = m.Size
	}
	if st, e := os.Stat(c.dbPath); e == nil {
		if expected == "" {
			return validate(c.dbPath)
		}
		if st.Size() == expectedSize {
			if sum, e := fileSHA256(c.dbPath); e == nil && strings.EqualFold(sum, expected) {
				return validate(c.dbPath)
			}
		}
	}
	q, e := http.NewRequestWithContext(x, http.MethodGet, u, nil)
	if e != nil {
		return e
	}
	r, e := c.httpClient.Do(q)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode >= 400 {
		return fmt.Errorf("download CYP database: %s", r.Status)
	}
	t, e := os.CreateTemp(filepath.Dir(c.dbPath), "p450phgo-*.pgd")
	if e != nil {
		return e
	}
	p := t.Name()
	_, e = io.Copy(t, io.LimitReader(r.Body, 4<<30))
	if e == nil {
		e = t.Close()
	}
	if e == nil {
		e = validate(p)
	}
	if e == nil && expectedSize > 0 {
		if st, se := os.Stat(p); se != nil || st.Size() != expectedSize {
			e = fmt.Errorf("CYP database size mismatch")
		}
	}
	if e == nil && expected != "" {
		var sum string
		sum, e = fileSHA256(p)
		if e == nil && !strings.EqualFold(sum, expected) {
			e = fmt.Errorf("CYP database checksum mismatch")
		}
	}
	if e == nil {
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			e = readErr
		} else {
			e = appfs.WriteFileAtomic(c.dbPath, data, 0o644)
		}
	}
	if e != nil {
		_ = os.Remove(p)
	} else {
		c.cacheMu.Lock()
		c.cacheStamp = ""
		c.cacheRows = nil
		c.cacheSpecies = nil
		c.cacheMu.Unlock()
	}
	return e
}

func (c *Client) cachedRecords() ([]record, error) {
	st, err := os.Stat(c.dbPath)
	if err != nil {
		return nil, err
	}
	stamp := fmt.Sprintf("%d:%d", st.Size(), st.ModTime().UnixNano())
	c.cacheMu.RLock()
	if c.cacheStamp == stamp && c.cacheRows != nil {
		rows := append([]record(nil), c.cacheRows...)
		c.cacheMu.RUnlock()
		return rows, nil
	}
	c.cacheMu.RUnlock()
	var rows []record
	if err := read(c.dbPath, func(v []record) error { rows = v; return nil }); err != nil {
		return nil, err
	}
	c.cacheMu.Lock()
	if c.cacheStamp != stamp {
		c.cacheStamp, c.cacheRows = stamp, append([]record(nil), rows...)
	}
	rows = append([]record(nil), c.cacheRows...)
	c.cacheMu.Unlock()
	return rows, nil
}

func (c *Client) cachedSpecies() ([]speciesRecord, error) {
	st, err := os.Stat(c.dbPath)
	if err != nil {
		return nil, err
	}
	stamp := fmt.Sprintf("%d:%d", st.Size(), st.ModTime().UnixNano())
	c.cacheMu.RLock()
	if c.cacheStamp == stamp && c.cacheSpecies != nil {
		items := append([]speciesRecord(nil), c.cacheSpecies...)
		c.cacheMu.RUnlock()
		return items, nil
	}
	c.cacheMu.RUnlock()
	var items []speciesRecord
	if err := readSpecies(c.dbPath, func(v []speciesRecord) error { items = v; return nil }); err != nil {
		return nil, err
	}
	c.cacheMu.Lock()
	if c.cacheStamp != stamp {
		c.cacheStamp, c.cacheSpecies = stamp, append([]speciesRecord(nil), items...)
	}
	items = append([]speciesRecord(nil), c.cacheSpecies...)
	c.cacheMu.Unlock()
	return items, nil
}
func (c *Client) fetchManifest(x context.Context, u string) (manifest, error) {
	var m manifest
	q, e := http.NewRequestWithContext(x, http.MethodGet, u, nil)
	if e != nil {
		return m, e
	}
	r, e := c.httpClient.Do(q)
	if e != nil {
		return m, e
	}
	defer r.Body.Close()
	if r.StatusCode >= 400 {
		return m, fmt.Errorf("check CYP database update: %s", r.Status)
	}
	e = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&m)
	if e != nil {
		return m, e
	}
	if strings.TrimSpace(m.DatabaseURL) == "" || strings.TrimSpace(m.SHA256) == "" {
		return m, errors.New("invalid CYP database manifest")
	}
	return m, nil
}
func fileSHA256(p string) (string, error) {
	f, e := os.Open(p)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
func validate(p string) error {
	d, e := bolt.Open(p, 0444, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if e != nil {
		return e
	}
	defer d.Close()
	return d.View(func(t *bolt.Tx) error {
		if t.Bucket(bucket) == nil {
			return errors.New("missing records bucket")
		}
		return nil
	})
}
func read(p string, fn func([]record) error) error {
	d, e := bolt.Open(p, 0444, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if e != nil {
		return e
	}
	defer d.Close()
	var a []record
	e = d.View(func(t *bolt.Tx) error {
		b := t.Bucket(bucket)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var r record
			if json.Unmarshal(v, &r) == nil {
				a = append(a, r)
			}
			return nil
		})
	})
	if e != nil {
		return e
	}
	return fn(a)
}
func (c *Client) FetchSpeciesCandidates(x context.Context) ([]model.SpeciesCandidate, error) {
	if e := c.ensureDB(x); e != nil {
		return nil, e
	}
	var a []model.SpeciesCandidate
	if ss, e := c.cachedSpecies(); e == nil {
		for _, s := range ss {
			if strings.TrimSpace(s.Name) != "" {
				a = append(a, model.SpeciesCandidate{JBrowseName: s.Name, GenomeLabel: s.Name, SearchAlias: s.Description, GroupKey: s.Category, Disabled: !s.Selectable})
			}
		}
		if len(a) > 0 {
			return a, nil
		}
	}
	if len(a) > 0 {
		return a, nil
	}
	rs, e := c.cachedRecords()
	if e == nil {
		s := map[string]bool{}
		for _, r := range rs {
			if r.Species != "" && !s[r.Species] {
				s[r.Species] = true
				a = append(a, model.SpeciesCandidate{JBrowseName: r.Species, GenomeLabel: r.Species, SearchAlias: r.Species, GroupKey: r.Category})
			}
		}
	}
	return a, e
}
func readSpecies(p string, fn func([]speciesRecord) error) error {
	d, e := bolt.Open(p, 0o444, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if e != nil {
		return e
	}
	defer d.Close()
	var out []speciesRecord
	e = d.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("species"))
		if b == nil {
			return errors.New("missing species bucket")
		}
		return b.ForEach(func(_, v []byte) error {
			var s speciesRecord
			if json.Unmarshal(v, &s) == nil {
				out = append(out, s)
			}
			return nil
		})
	})
	if e != nil {
		return e
	}
	return fn(out)
}
func (c *Client) SearchKeywordRows(x context.Context, s model.SpeciesCandidate, k string) ([]model.KeywordResultRow, error) {
	if e := c.ensureDB(x); e != nil {
		return nil, e
	}
	term := strings.ToLower(strings.TrimSpace(k))
	category := ""
	parts := strings.Fields(term)
	if len(parts) > 1 {
		switch strings.TrimSuffix(parts[0], ":") {
		case "a", "animals":
			category = "animals"
		case "p", "plants":
			category = "plants"
		case "f", "fungi", "fungal":
			category = "fungi"
		case "b", "bacteria":
			category = "bacteria"
		}
		if category != "" {
			term = strings.Join(parts[1:], " ")
		}
	}
	var a []model.KeywordResultRow
	rs, e := c.cachedRecords()
	if e != nil {
		return nil, e
	}
	for _, r := range rs {
		if category != "" && !strings.EqualFold(category, r.Category) {
			continue
		}
		if s.JBrowseName != "" && !strings.EqualFold(s.JBrowseName, "all") && !strings.EqualFold(s.JBrowseName, r.Species) {
			continue
		}
		if !strings.Contains(strings.ToLower(r.ID+" "+r.Species+" "+r.Symbol+" "+r.Description), term) {
			continue
		}
		extra := map[string]string{"cyp_category": r.Category}
		if strings.TrimSpace(r.Sequence) != "" {
			extra["cyp_sequence"] = r.Sequence
			extra["cyp_fasta"] = ">" + r.ID + "\n" + r.Sequence
		}
		sequenceID := strings.TrimSpace(r.RecordKey)
		if sequenceID == "" {
			sequenceID = r.ID
		}
		a = append(a, model.KeywordResultRow{SourceDatabase: "cyp", SearchTerm: k, SearchType: "CYP keyword", LabelName: r.Symbol, GeneIdentifier: r.ID, Genome: r.Species, Description: r.Description, GeneReportURL: r.SourceURL, SequenceID: sequenceID, ExtraColumns: extra})
	}
	return a, e
}

// FetchProteinSequence resolves a sequence from the local PGD only.  CYP has
// no online sequence fallback: the published database is the authoritative
// source at runtime.
func (c *Client) FetchProteinSequence(ctx context.Context, _ int, sequenceID string) (model.ProteinSequenceData, error) {
	if err := c.ensureDB(ctx); err != nil {
		return model.ProteinSequenceData{}, err
	}
	want := strings.TrimSpace(sequenceID)
	if want == "" {
		return model.ProteinSequenceData{}, errors.New("CYP sequence unavailable")
	}
	var found model.ProteinSequenceData
	rs, err := c.cachedRecords()
	if err == nil {
		for _, r := range rs {
			if !strings.EqualFold(strings.TrimSpace(r.RecordKey), want) && !strings.EqualFold(strings.TrimSpace(r.ID), want) && !strings.EqualFold(strings.TrimSpace(r.Symbol), want) {
				continue
			}
			if strings.TrimSpace(r.Sequence) == "" {
				continue
			}
			found = model.ProteinSequenceData{Sequence: strings.TrimSpace(r.Sequence), OriginalHeader: ">" + r.ID}
			break
		}
	}
	if err != nil {
		return model.ProteinSequenceData{}, err
	}
	if strings.TrimSpace(found.Sequence) == "" {
		return model.ProteinSequenceData{}, errors.New("CYP sequence unavailable")
	}
	return found, nil
}
func (c *Client) FetchGeneQuerySequence(context.Context, model.SpeciesCandidate, string, string) (*model.QuerySequenceSource, error) {
	return nil, errors.New("CYP query unavailable")
}
func (c *Client) SubmitBlast(context.Context, model.BlastRequest) (model.BlastJob, error) {
	return model.BlastJob{}, errors.New("CYP BLAST unavailable")
}
func (c *Client) WaitForBlastResults(context.Context, string, time.Duration, time.Duration) (model.BlastResult, error) {
	return model.BlastResult{}, errors.New("CYP BLAST unavailable")
}

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

const DatabaseFileName = "cypgo.pgd"
const DefaultManifestURL = "https://raw.githubusercontent.com/KiriKirby/cypgo/main/manifest.json"

const legacyDatabaseFileName = "p450phgo.pgd"
const legacyManifestEnv = "PHGO_CYP_PGD_MANIFEST_URL"
const legacyDatabaseEnv = "PHGO_CYP_PGD_URL"

type manifest struct {
	DatabaseURL string `json:"database_url"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
}

// DatabaseStatus describes the locally installed CYP PGD and the current
// published manifest. It is intentionally small so the workflow can decide
// whether an explicit download/update prompt is needed before querying.
type DatabaseStatus struct {
	LocalPath       string
	Ready           bool
	UpdateAvailable bool
	ExpectedSize    int64
	LocalSize       int64
	ManifestURL     string
	DatabaseURL     string
}

type DownloadProgress struct {
	Current int64
	Total   int64
	Message string
}

var _ source.DataSource = (*Client)(nil)

type record struct{ ID, RecordKey, Category, Species, Symbol, Description, Sequence, SourceURL string }

func recordSequenceID(r record) string {
	key := strings.TrimSpace(r.RecordKey)
	// Older PGD rows may have used a category name as a shared RecordKey. That
	// is not a row identity and can make an empty legacy row resolve to another
	// record. Use the biological ID for those legacy rows.
	if key == "" || isCYPCategoryKey(key) {
		return strings.TrimSpace(r.ID)
	}
	return key
}

func isCYPCategoryKey(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "plants", "animals", "fungi", "bacteria", "protists":
		return true
	default:
		return false
	}
}

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
	dbPath := filepath.Join(r, DatabaseFileName)
	legacyPath := filepath.Join(r, legacyDatabaseFileName)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		// Preserve an already-downloaded database across the cypgo rename.
		_ = os.Rename(legacyPath, dbPath)
	}
	return &Client{httpClient: h, dbPath: dbPath}
}
func (c *Client) Name() string         { return "cyp" }
func (c *Client) DatabasePath() string { return c.dbPath }

var bucket = []byte("records")

func (c *Client) manifestURL() string {
	murl := strings.TrimSpace(os.Getenv("PHGO_CYPGO_MANIFEST_URL"))
	if murl == "" {
		murl = strings.TrimSpace(os.Getenv(legacyManifestEnv))
	}
	if murl == "" {
		murl = DefaultManifestURL
	}
	return murl
}

func (c *Client) CheckDatabase(x context.Context) (DatabaseStatus, error) {
	status := DatabaseStatus{LocalPath: c.dbPath, ManifestURL: c.manifestURL()}
	u := databaseURLOverride()
	m := manifest{}
	if u == "" {
		var e error
		m, e = c.fetchManifest(x, status.ManifestURL)
		if e != nil {
			if st, statErr := os.Stat(c.dbPath); statErr == nil {
				status.LocalSize = st.Size()
				status.Ready = validate(c.dbPath) == nil
				return status, nil
			}
			return status, e
		}
		u = m.DatabaseURL
	}
	status.DatabaseURL, status.ExpectedSize = u, m.Size
	if st, e := os.Stat(c.dbPath); e == nil {
		status.LocalSize = st.Size()
		valid := validate(c.dbPath) == nil
		if strings.TrimSpace(m.SHA256) == "" {
			status.Ready = valid
		} else if st.Size() == m.Size {
			sum, sumErr := fileSHA256(c.dbPath)
			status.Ready = valid && sumErr == nil && strings.EqualFold(sum, m.SHA256)
		}
	}
	status.UpdateAvailable = !status.Ready
	return status, nil
}

func (c *Client) DownloadDatabase(x context.Context, progress func(DownloadProgress)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	status, err := c.CheckDatabase(x)
	if err != nil {
		return err
	}
	if status.Ready {
		return nil
	}
	if strings.TrimSpace(status.DatabaseURL) == "" {
		return errors.New("CYP database URL is unavailable")
	}
	q, e := http.NewRequestWithContext(x, http.MethodGet, status.DatabaseURL, nil)
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
	t, e := os.CreateTemp(filepath.Dir(c.dbPath), "cypgo-*.part")
	if e != nil {
		return e
	}
	p := t.Name()
	defer func() { _ = os.Remove(p) }()
	total := status.ExpectedSize
	if total <= 0 {
		total = r.ContentLength
	}
	if progress != nil {
		progress(DownloadProgress{Total: total, Message: "Downloading CYP database..."})
	}
	var current int64
	buf := make([]byte, 256*1024)
	for {
		n, readErr := r.Body.Read(buf)
		if n > 0 {
			if _, e = t.Write(buf[:n]); e != nil {
				return e
			}
			current += int64(n)
			if progress != nil {
				progress(DownloadProgress{Current: current, Total: total, Message: "Downloading CYP database..."})
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if e = t.Close(); e != nil {
		return e
	}
	if e = validate(p); e != nil {
		return e
	}
	if status.ExpectedSize > 0 && current != status.ExpectedSize {
		return fmt.Errorf("CYP database size mismatch: got %d bytes, want %d", current, status.ExpectedSize)
	}
	if databaseURLOverride() == "" && status.ExpectedSize > 0 {
		sum, sumErr := fileSHA256(p)
		if sumErr != nil {
			return sumErr
		}
		m, mErr := c.fetchManifest(x, status.ManifestURL)
		if mErr != nil || !strings.EqualFold(sum, m.SHA256) {
			return errors.New("CYP database checksum mismatch")
		}
	}
	data, e := os.ReadFile(p)
	if e != nil {
		return e
	}
	if e = appfs.WriteFileAtomic(c.dbPath, data, 0o644); e != nil {
		return e
	}
	c.cacheMu.Lock()
	c.cacheStamp, c.cacheRows, c.cacheSpecies = "", nil, nil
	c.cacheMu.Unlock()
	if progress != nil {
		progress(DownloadProgress{Current: current, Total: total, Message: "CYP database is ready."})
	}
	return nil
}

func databaseURLOverride() string {
	u := strings.TrimSpace(os.Getenv("PHGO_CYPGO_URL"))
	if u == "" {
		u = strings.TrimSpace(os.Getenv(legacyDatabaseEnv))
	}
	return u
}

// EnsureDatabase is retained for non-interactive callers and tests. UI code
// should call CheckDatabase/DownloadDatabase so installation is explicit.
func (c *Client) EnsureDatabase(x context.Context) error {
	status, err := c.CheckDatabase(x)
	if err != nil {
		return err
	}
	if status.Ready {
		return nil
	}
	return c.DownloadDatabase(x, nil)
}

func (c *Client) ensureDB(x context.Context) error {
	return c.EnsureDatabase(x)
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
	category, term := splitCYPCategoryQuery(k)
	queryRunes := []rune(term)
	if len(queryRunes) == 0 {
		return nil, nil
	}
	rs, e := c.cachedRecords()
	if e != nil {
		return nil, e
	}

	// Match the static CYPgo website: search only the displayed CYP name
	// (Symbol, falling back to ID), try the complete trimmed query first, then
	// remove one Unicode character from the right until the first non-empty
	// result level is found. Compute the best level in one database scan rather
	// than rescanning all records once per shortened query.
	bestLength := 0
	matchedRecords := make([]record, 0)
	for _, r := range rs {
		if !cypRecordInScope(r, s, category) {
			continue
		}
		matchLength := longestCYPQueryPrefixMatch(displayedCYPName(r), queryRunes, bestLength)
		if matchLength == 0 || matchLength < bestLength {
			continue
		}
		if matchLength > bestLength {
			bestLength = matchLength
			matchedRecords = matchedRecords[:0]
		}
		matchedRecords = append(matchedRecords, r)
	}
	if bestLength == 0 {
		return nil, nil
	}
	matchedTerm := string(queryRunes[:bestLength])
	searchType := "CYP name full-query match"
	matchMethod := "full CYP name query"
	if bestLength < len(queryRunes) {
		searchType = "CYP name right-truncation fallback"
		matchMethod = "right-truncated CYP name fallback"
	}
	return buildCYPKeywordRows(matchedRecords, k, matchedTerm, searchType, matchMethod), nil
}

// SearchKeywordRowsByGeneLocus keeps PHgo's Gene locus priority separate from
// website-style CYP-name fallback. A missed locus must return no rows so the
// workflow can safely try the row's ordinary CYP search term next.
func (c *Client) SearchKeywordRowsByGeneLocus(x context.Context, s model.SpeciesCandidate, locus string) ([]model.KeywordResultRow, error) {
	if e := c.ensureDB(x); e != nil {
		return nil, e
	}
	term := strings.TrimSpace(locus)
	if term == "" {
		return nil, nil
	}
	rs, err := c.cachedRecords()
	if err != nil {
		return nil, err
	}
	matched := make([]record, 0)
	for _, r := range rs {
		if cypRecordInScope(r, s, "") && strings.EqualFold(strings.TrimSpace(r.ID), term) {
			matched = append(matched, r)
		}
	}
	return buildCYPKeywordRows(matched, locus, term, "CYP Gene locus exact", "exact Gene locus"), nil
}

func splitCYPCategoryQuery(query string) (string, string) {
	term := strings.TrimSpace(query)
	parts := strings.Fields(term)
	if len(parts) <= 1 {
		return "", term
	}
	category := ""
	switch strings.ToLower(strings.TrimSuffix(parts[0], ":")) {
	case "a", "animals":
		category = "animals"
	case "p", "plants":
		category = "plants"
	case "f", "fungi", "fungal":
		category = "fungi"
	case "b", "bacteria":
		category = "bacteria"
	case "r", "protist", "protists":
		category = "protists"
	}
	if category == "" {
		return "", term
	}
	return category, strings.Join(parts[1:], " ")
}

func cypRecordInScope(r record, species model.SpeciesCandidate, category string) bool {
	if category != "" && !strings.EqualFold(category, r.Category) {
		return false
	}
	return species.JBrowseName == "" || strings.EqualFold(species.JBrowseName, "all") || strings.EqualFold(species.JBrowseName, r.Species)
}

func displayedCYPName(r record) string {
	if symbol := strings.TrimSpace(r.Symbol); symbol != "" {
		return symbol
	}
	return strings.TrimSpace(r.ID)
}

func longestCYPQueryPrefixMatch(name string, queryRunes []rune, minimum int) int {
	lowerName := strings.ToLower(name)
	lowest := minimum
	if lowest < 1 {
		lowest = 1
	}
	for length := len(queryRunes); length >= lowest; length-- {
		if strings.Contains(lowerName, strings.ToLower(string(queryRunes[:length]))) {
			return length
		}
	}
	return 0
}

func buildCYPKeywordRows(records []record, originalTerm string, matchedTerm string, searchType string, matchMethod string) []model.KeywordResultRow {
	rows := make([]model.KeywordResultRow, 0, len(records))
	for _, r := range records {
		extra := map[string]string{
			"cyp_category":            r.Category,
			"cyp_match_method":        matchMethod,
			"cyp_matched_search_term": matchedTerm,
		}
		if strings.TrimSpace(r.Sequence) != "" {
			extra["cyp_sequence"] = r.Sequence
			extra["cyp_fasta"] = ">" + r.ID + "\n" + r.Sequence
		}
		rows = append(rows, model.KeywordResultRow{
			SourceDatabase: "cyp", SearchTerm: originalTerm, SearchType: searchType,
			LabelName: displayedCYPName(r), GeneLocus: r.ID, GeneIdentifier: r.ID,
			Genome: r.Species, Description: r.Description, GeneReportURL: r.SourceURL,
			SequenceID: recordSequenceID(r), ExtraColumns: extra,
		})
	}
	return rows
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
		return model.ProteinSequenceData{}, errors.New("CYP sequence unavailable: the reviewed source does not contain a protein sequence for this record")
	}
	var found model.ProteinSequenceData
	rs, err := c.cachedRecords()
	if err == nil {
		for _, r := range rs {
			if !strings.EqualFold(recordSequenceID(r), want) && !strings.EqualFold(strings.TrimSpace(r.RecordKey), want) && !strings.EqualFold(strings.TrimSpace(r.ID), want) && !strings.EqualFold(strings.TrimSpace(r.Symbol), want) {
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
		return model.ProteinSequenceData{}, errors.New("CYP sequence unavailable: the reviewed source does not contain a protein sequence for this record")
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

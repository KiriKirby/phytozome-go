package phylo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

type RowSource struct {
	TaxonID      string
	ItemTitle    string
	ItemIndex    int
	RowIndex     int
	CanvasRow    model.CanvasRow
	Sequence     string
	SequenceKind SequenceKind
	SourceType   string
	OriginalHead string
	TableValues  map[string]string
}

type RunPlan struct {
	SessionID       string
	RunID           string
	BaseDir         string
	Settings        TreeSettings
	Kind            SequenceKind
	Records         []InputRecord
	Metadata        Metadata
	InputFASTA      string
	RuntimeRequest  string
	RuntimeResponse string
	AlignedFASTA    string
	Newick          string
	Fingerprints    Fingerprints
	UpdatedAt       time.Time
}

func BuildInput(records []RowSource, sourceColumn string, sessionID string, now time.Time) ([]InputRecord, Metadata, error) {
	if len(records) == 0 {
		return nil, Metadata{}, fmt.Errorf("no tree input records were selected")
	}
	sourceColumn = strings.TrimSpace(sourceColumn)
	if sourceColumn == "" {
		sourceColumn = DefaultDisplayNameSource
	}
	out := make([]InputRecord, 0, len(records))
	meta := Metadata{
		SchemaVersion:     1,
		GeneratedAt:       now,
		DisplayNameSource: sourceColumn,
		Records:           make([]InputRecord, 0, len(records)),
	}
	reservedTaxonIDs := make(map[string]struct{}, len(records))
	for _, src := range records {
		if taxonID := strings.TrimSpace(src.TaxonID); taxonID != "" {
			reservedTaxonIDs[taxonID] = struct{}{}
		}
	}
	nextTaxonNumber := 1
	for _, src := range records {
		taxonID := strings.TrimSpace(src.TaxonID)
		if taxonID == "" {
			for {
				candidate := fmt.Sprintf("PHGOT%06d", nextTaxonNumber)
				nextTaxonNumber++
				if _, exists := reservedTaxonIDs[candidate]; exists {
					continue
				}
				taxonID = candidate
				reservedTaxonIDs[taxonID] = struct{}{}
				break
			}
		}
		name := ""
		if src.CanvasRow.DisplayNameLocked {
			name = strings.TrimSpace(src.CanvasRow.DisplayName)
		}
		if name == "" && (sourceColumn == PHgoDisplayNameSource || sourceColumn == YTDisplayNameSource || sourceColumn == YTV2DisplayNameSource) {
			name = strings.TrimSpace(src.TableValues[sourceColumn])
		}
		if name == "" {
			name = strings.TrimSpace(src.CanvasRow.DisplayName)
		}
		if name == "" {
			name = strings.TrimSpace(src.TableValues[sourceColumn])
		}
		if name == "" && sourceColumn != "head" {
			name = strings.TrimSpace(src.TableValues["head"])
		}
		if name == "" {
			name = strings.TrimSpace(src.OriginalHead)
		}
		if name == "" {
			name = taxonID
		}
		record := InputRecord{
			TaxonID:         taxonID,
			DisplayName:     name,
			SourceType:      strings.TrimSpace(src.SourceType),
			OriginalHead:    strings.TrimSpace(src.OriginalHead),
			Sequence:        strings.TrimSpace(src.Sequence),
			SequenceKind:    src.SequenceKind,
			CanvasItem:      strings.TrimSpace(src.ItemTitle),
			CanvasItemIndex: src.ItemIndex,
			CanvasRow:       src.RowIndex,
			TableValues:     cloneTreeTableValues(src.TableValues),
			RowFingerprint:  fingerprintRow(src, sourceColumn),
		}
		out = append(out, record)
		meta.Records = append(meta.Records, record)
	}
	return out, meta, nil
}

func BuildPayload(sessionID string, records []InputRecord, metadata Metadata, alignedFASTA string, newick string, updatedAt time.Time) ViewerPayload {
	return ViewerPayload{
		SchemaVersion: 1,
		SessionID:     strings.TrimSpace(sessionID),
		Title:         viewerPayloadTitle(records, sessionID),
		UpdatedAt:     updatedAt,
		Newick:        strings.TrimSpace(newick),
		AlignedFASTA:  strings.TrimSpace(alignedFASTA),
		Metadata:      metadata,
	}
}

func viewerPayloadTitle(records []InputRecord, fallback string) string {
	seen := make(map[string]struct{})
	for _, record := range records {
		title := strings.TrimSpace(record.CanvasItem)
		if title == "" {
			continue
		}
		seen[title] = struct{}{}
	}
	if len(seen) == 1 {
		for title := range seen {
			return title
		}
	}
	if len(seen) > 1 {
		return "System tree"
	}
	return strings.TrimSpace(fallback)
}

func BuildRunPlan(sessionID string, runID string, baseDir string, settings TreeSettings, kind SequenceKind, records []InputRecord, metadata Metadata, alignedFASTA string, newick string, updatedAt time.Time) (RunPlan, error) {
	settings = NormalizeTreeSettingsForKind(settings, kind)
	inputFASTA := runtimeInputFASTA(records, settings)
	fingerprints := BuildFingerprints(records, settings, alignedFASTA, newick)
	return RunPlan{
		SessionID:       strings.TrimSpace(sessionID),
		RunID:           strings.TrimSpace(runID),
		BaseDir:         strings.TrimSpace(baseDir),
		Settings:        settings,
		Kind:            kind,
		Records:         append([]InputRecord(nil), records...),
		Metadata:        metadata,
		InputFASTA:      inputFASTA,
		RuntimeRequest:  "",
		RuntimeResponse: "",
		AlignedFASTA:    strings.TrimSpace(alignedFASTA),
		Newick:          strings.TrimSpace(newick),
		Fingerprints:    fingerprints,
		UpdatedAt:       updatedAt,
	}, nil
}

func (p RunPlan) ToArtifactSet() ArtifactSet {
	metadata := p.Metadata
	metadata.TreeComputationSource = "mega-phgo-runtime"
	metadata.SequenceKind = p.Kind
	metadata.ConversionTarget = p.Settings.ConversionTarget
	metadata.AlignmentMethod = p.Settings.AlignmentMethod
	metadata.TreeMethod = p.Settings.TreeMethod
	metadata.AlignmentParams = cloneStringMap(p.Settings.AlignmentParams)
	metadata.TreeParams = cloneStringMap(p.Settings.TreeParams)
	metadata.TreeCount = 1
	manifest := RunManifest{
		SchemaVersion:   1,
		CreatedAt:       p.UpdatedAt,
		Settings:        p.Settings,
		Fingerprints:    p.Fingerprints,
		InputFASTA:      "input.fasta",
		MetadataJSON:    "input.meta.json",
		RuntimeRequest:  "runtime-request.json",
		RuntimeResponse: "runtime-response.json",
	}
	if strings.TrimSpace(p.AlignedFASTA) != "" {
		manifest.AlignedFASTA = "aligned.fasta"
	}
	if strings.TrimSpace(p.Newick) != "" {
		manifest.NewickPath = "tree.nwk"
	}
	return ArtifactSet{
		BaseDir:         p.BaseDir,
		Manifest:        manifest,
		Metadata:        metadata,
		InputFASTA:      p.InputFASTA,
		RuntimeRequest:  p.RuntimeRequest,
		RuntimeResponse: p.RuntimeResponse,
		AlignedFASTA:    p.AlignedFASTA,
		Newick:          p.Newick,
		Payload:         BuildPayload(p.SessionID, p.Records, metadata, p.AlignedFASTA, p.Newick, p.UpdatedAt),
	}
}

func BuildFingerprints(records []InputRecord, settings TreeSettings, alignedFASTA string, newick string) Fingerprints {
	settings = NormalizeTreeSettings(settings)
	normalizedRecords := normalizeRecordsForRuntime(records, settings)
	inputDigest := sha256.New()
	alignmentDigest := sha256.New()
	treeDigest := sha256.New()
	previewDigest := sha256.New()

	writeComputeRecordFingerprint(inputDigest, normalizedRecords)
	writeComputeRecordFingerprint(alignmentDigest, normalizedRecords)
	alignmentDigest.Write([]byte("\nconversion_target=" + string(settings.ConversionTarget)))
	alignmentDigest.Write([]byte("\nmethod=" + string(settings.AlignmentMethod)))
	writeSortedMap(alignmentDigest, settings.AlignmentParams)

	treeDigest.Write([]byte(hex.EncodeToString(alignmentDigest.Sum(nil))))
	treeDigest.Write([]byte("\nmethod=" + string(settings.TreeMethod)))
	writeSortedMap(treeDigest, settings.TreeParams)

	writeString(previewDigest, settings.DisplayNameSource)
	for _, record := range records {
		previewDigest.Write([]byte(record.TaxonID))
		previewDigest.Write([]byte("\n"))
		previewDigest.Write([]byte(record.DisplayName))
		previewDigest.Write([]byte("\n"))
		previewDigest.Write([]byte(record.DisplayPrefix))
		previewDigest.Write([]byte("\n"))
	}

	return Fingerprints{
		Input:     hex.EncodeToString(inputDigest.Sum(nil)),
		Alignment: hex.EncodeToString(alignmentDigest.Sum(nil)),
		Tree:      hex.EncodeToString(treeDigest.Sum(nil)),
		Preview:   hex.EncodeToString(previewDigest.Sum(nil)),
	}
}

func normalizeRecordsForRuntime(records []InputRecord, settings TreeSettings) []InputRecord {
	normalize := runtimeSequenceNormalizer(settings)
	if normalize == nil {
		return records
	}
	out := make([]InputRecord, len(records))
	for i, record := range records {
		out[i] = record
		out[i].Sequence = normalize(sanitizeFASTASequence(record.Sequence))
		out[i].RowFingerprint = fingerprintInputRecord(out[i])
	}
	return out
}

func LoadRunManifest(dir string) (RunManifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "run.manifest.json"))
	if err != nil {
		return RunManifest{}, err
	}
	var manifest RunManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return RunManifest{}, err
	}
	return manifest, nil
}

func ManifestMatchesPlan(manifest RunManifest, plan RunPlan) bool {
	return manifest.SchemaVersion == 1 &&
		computationSettingsEqual(manifest.Settings, plan.Settings) &&
		manifest.Fingerprints.Input == plan.Fingerprints.Input &&
		manifest.Fingerprints.Alignment == plan.Fingerprints.Alignment &&
		manifest.Fingerprints.Tree == plan.Fingerprints.Tree &&
		strings.TrimSpace(manifest.InputFASTA) == "input.fasta" &&
		strings.TrimSpace(manifest.MetadataJSON) == "input.meta.json" &&
		strings.TrimSpace(manifest.RuntimeRequest) == "runtime-request.json" &&
		strings.TrimSpace(manifest.RuntimeResponse) == "runtime-response.json" &&
		strings.TrimSpace(manifest.AlignedFASTA) == "aligned.fasta" &&
		strings.TrimSpace(manifest.NewickPath) == "tree.nwk"
}

func computationSettingsEqual(a TreeSettings, b TreeSettings) bool {
	a = NormalizeTreeSettings(a)
	b = NormalizeTreeSettings(b)
	return a.AlignmentMethod == b.AlignmentMethod &&
		a.ConversionTarget == b.ConversionTarget &&
		a.TreeMethod == b.TreeMethod &&
		reflect.DeepEqual(a.AlignmentParams, b.AlignmentParams) &&
		reflect.DeepEqual(a.TreeParams, b.TreeParams)
}

func fingerprintRow(src RowSource, sourceColumn string) string {
	h := sha256.New()
	writeString(h, src.ItemTitle)
	writeString(h, strconv.Itoa(src.ItemIndex))
	writeString(h, strconv.Itoa(src.RowIndex))
	writeString(h, src.SourceType)
	writeString(h, src.OriginalHead)
	writeString(h, src.Sequence)
	writeSortedMapExcept(h, src.TableValues, "display_name")
	return hex.EncodeToString(h.Sum(nil))
}

func fingerprintInputRecord(record InputRecord) string {
	h := sha256.New()
	writeString(h, record.CanvasItem)
	writeString(h, strconv.Itoa(record.CanvasItemIndex))
	writeString(h, strconv.Itoa(record.CanvasRow))
	writeString(h, record.SourceType)
	writeString(h, record.OriginalHead)
	writeString(h, record.Sequence)
	writeSortedMapExcept(h, record.TableValues, "display_name")
	return hex.EncodeToString(h.Sum(nil))
}

func cloneTreeTableValues(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	return cloneStringMap(values)
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func writeComputeRecordFingerprint(h interface{ Write([]byte) (int, error) }, records []InputRecord) {
	for _, record := range records {
		writeString(h, record.TaxonID)
		writeString(h, record.RowFingerprint)
		writeString(h, string(record.SequenceKind))
		writeString(h, record.Sequence)
	}
}

func writeSortedMap(h interface{ Write([]byte) (int, error) }, values map[string]string) {
	if len(values) == 0 {
		return
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		writeString(h, key)
		writeString(h, values[key])
	}
}

func writeSortedMapExcept(h interface{ Write([]byte) (int, error) }, values map[string]string, excluded ...string) {
	if len(values) == 0 {
		return
	}
	skip := make(map[string]struct{}, len(excluded))
	for _, key := range excluded {
		skip[strings.ToLower(strings.TrimSpace(key))] = struct{}{}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if _, ok := skip[strings.ToLower(strings.TrimSpace(key))]; ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		writeString(h, key)
		writeString(h, values[key])
	}
}

func writeString(h interface{ Write([]byte) (int, error) }, value string) {
	_, _ = h.Write([]byte(value))
	_, _ = h.Write([]byte{0})
}

func MarshalMetadata(meta Metadata) ([]byte, error) {
	return json.MarshalIndent(meta, "", "  ")
}

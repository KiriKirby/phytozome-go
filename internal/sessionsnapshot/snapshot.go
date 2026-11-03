// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package sessionsnapshot

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/interpro"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/phylo"
	"github.com/KiriKirby/phytozome-go/internal/tui"
	"github.com/KiriKirby/phytozome-go/internal/uniprot"
)

const (
	FileExtension = ".pgo"
	FormatName    = "phgo-session-snapshot"
	FormatVersion = "2.8"

	contextModuleName       = "context"
	keywordModuleName       = "keyword-result"
	keywordSourceName       = "keyword-source-state"
	blastModuleName         = "blast-result"
	canvasModuleName        = "canvas-result"
	keywordReviewName       = "keyword-review-state"
	blastReviewName         = "blast-review-state"
	canvasReviewName        = "canvas-review-state"
	canvasMSAName           = "canvas-msa-state"
	sequenceCacheName       = "sequence-cache"
	exportSettingsName      = "export-settings"
	externalReferencesName  = "external-references"
	handoffStateName        = "handoff-state"
	artifactManifestName    = "artifact-manifest"
	runtimeCacheName        = "runtime-cache"
	contextModulePath       = "modules/context-v2.xml"
	keywordModulePath       = "modules/keyword-result-v2.xml"
	keywordSourceModulePath = "modules/keyword-source-state-v4.xml"
	blastModulePath         = "modules/blast-result-v2.xml"
	canvasModulePath        = "modules/canvas-result-v2.xml"
	keywordReviewModulePath = "modules/keyword-review-state-v2.xml"
	blastReviewModulePath   = "modules/blast-review-state-v2.xml"
	canvasReviewModulePath  = "modules/canvas-review-state-v2.xml"
	canvasMSAModulePath     = "modules/canvas-msa-state-v1.xml"
	sequenceCacheModulePath = "modules/sequence-cache-v2.xml"
	exportModulePath        = "modules/export-settings-v2.xml"
	referenceModulePath     = "modules/external-references-v2.xml"
	handoffModulePath       = "modules/handoff-state-v2.xml"
	artifactModulePath      = "modules/artifact-manifest-v2.xml"
	runtimeCacheModulePath  = "modules/runtime-cache-v2.xml"
)

type Snapshot struct {
	Context            ContextV2
	Keyword            *KeywordResultV2
	KeywordSource      *KeywordSourceStateV4
	Blast              *BlastResultV2
	Canvas             *CanvasResultV2
	CanvasMSA          *CanvasMSAV1
	KeywordReview      *KeywordReviewStateV2
	BlastReview        *BlastReviewStateV2
	CanvasReview       *CanvasReviewStateV2
	SequenceCache      *SequenceCacheV2
	ExportSettings     *ExportSettingsV2
	ExternalReferences *ExternalReferenceSettingsV2
	Handoff            *HandoffStateV2
	Artifacts          *ArtifactManifestV2
	RuntimeCache       *RuntimeCacheV2
	ArtifactPayloads   map[string][]byte
}

type ContextV2 struct {
	SnapshotID         string    `json:"snapshot_id"`
	CreatedAt          time.Time `json:"created_at"`
	ApplicationName    string    `json:"application_name"`
	ApplicationVersion string    `json:"application_version"`
	FormatName         string    `json:"format_name"`
	FormatVersion      string    `json:"format_version"`
	Database           string    `json:"database"`
	Mode               string    `json:"mode"`
	ResultKind         string    `json:"result_kind"`
	Title              string    `json:"title"`
}

type KeywordResultV2 struct {
	SelectedSpecies model.SpeciesCandidate     `json:"selected_species"`
	Groups          []model.KeywordSearchGroup `json:"groups"`
	Selected        []bool                     `json:"selected"`
	ReportContext   ReportContextV2            `json:"report_context"`
}

type KeywordSourceStateV4 struct {
	Database     string               `json:"database"`
	SourceKind   string               `json:"source_kind"`
	Engine       string               `json:"engine"`
	ResultDomain string               `json:"result_domain"`
	SearchTypes  []string             `json:"search_types,omitempty"`
	Terms        []string             `json:"terms,omitempty"`
	Extra        map[string]string    `json:"extra,omitempty"`
	NCBI         *NCBIKeywordSourceV4 `json:"ncbi,omitempty"`
}

type NCBIKeywordSourceV4 struct {
	EntrezDatabase       string   `json:"entrez_database"`
	RecordType           string   `json:"record_type"`
	EUtilitiesBaseURL    string   `json:"eutilities_base_url"`
	EngineSchema         string   `json:"engine_schema"`
	Accessions           []string `json:"accessions,omitempty"`
	UIDs                 []string `json:"uids,omitempty"`
	RequestedIDs         []string `json:"requested_ids,omitempty"`
	ReplacementTargets   []string `json:"replacement_targets,omitempty"`
	ReplacementDecisions []string `json:"replacement_decisions,omitempty"`
	LinkResolution       string   `json:"link_resolution,omitempty"`
	LinkedFromDB         string   `json:"linked_from_db,omitempty"`
	LinkedToDB           string   `json:"linked_to_db,omitempty"`
	LinkedFromTypes      []string `json:"linked_from_search_types,omitempty"`
	LinkedToTypes        []string `json:"linked_to_search_types,omitempty"`
	LinkNames            []string `json:"link_names,omitempty"`
	LinkSourceIDs        []string `json:"link_source_ids,omitempty"`
	LinkTargetIDs        []string `json:"link_target_ids,omitempty"`
	SearchTypeIDs        []string `json:"search_type_ids,omitempty"`
	ResultDomains        []string `json:"result_domains,omitempty"`
	GroupSearchTypes     []string `json:"group_search_types,omitempty"`
}

type KeywordSourceStateV3 = KeywordSourceStateV4
type NCBIKeywordSourceV3 = NCBIKeywordSourceV4

type ReportContextV2 struct {
	QueryStarted  time.Time `json:"query_started"`
	SearchEnded   time.Time `json:"search_ended"`
	ReviewStarted time.Time `json:"review_started"`
	LabelMode     string    `json:"label_mode"`
}

type BlastResultV2 struct {
	SelectedSpecies   model.SpeciesCandidate    `json:"selected_species"`
	Prepared          []BlastQueryItemV2        `json:"prepared"`
	Runs              []BlastRunV2              `json:"runs"`
	ConfiguredRequest model.BlastRequest        `json:"configured_request"`
	OriginalRunCount  int                       `json:"original_run_count"`
	Selected          []bool                    `json:"selected"`
	SelectedByRun     [][]bool                  `json:"selected_by_run"`
	FilterFlags       []bool                    `json:"filter_flags"`
	FilterFlagsByRun  [][]bool                  `json:"filter_flags_by_run"`
	FilterSettings    model.BlastFilterSettings `json:"filter_settings"`
	FilterApplied     bool                      `json:"filter_applied"`
	FilterCleared     bool                      `json:"filter_cleared"`
}

type CanvasResultV2 struct {
	Items         []CanvasItemV2 `json:"items"`
	CurrentItem   int            `json:"current_item"`
	NextNumericID int            `json:"next_numeric_id"`
	ImportedFrom  string         `json:"imported_from"`
	Tree          *CanvasTreeV2  `json:"tree,omitempty"`
}

type CanvasTreeV2 struct {
	PanelState       tui.CanvasTreePanelState `json:"panel_state"`
	LastPayload      phylo.ViewerPayload      `json:"last_payload"`
	ViewerState      json.RawMessage          `json:"viewer_state,omitempty"`
	LastManifest     phylo.RunManifest        `json:"last_manifest"`
	LastArtifactDir  string                   `json:"last_artifact_dir,omitempty"`
	LastRunID        string                   `json:"last_run_id,omitempty"`
	LastAlignedFASTA string                   `json:"last_aligned_fasta,omitempty"`
	LastNewick       string                   `json:"last_newick,omitempty"`
	Fingerprints     phylo.Fingerprints       `json:"fingerprints"`
	ArtifactPaths    []string                 `json:"artifact_paths,omitempty"`
}

type CanvasMSAV1 struct {
	State            phylo.MSAState      `json:"state"`
	LastPayload      phylo.ViewerPayload `json:"last_payload,omitempty"`
	LastAlignedFASTA string              `json:"last_aligned_fasta,omitempty"`
}

func IsLegacyTreeSnapshot(snapshot Snapshot) bool {
	return compareFormatVersion(snapshot.Context.FormatVersion, FormatVersion) < 0
}

func compareFormatVersion(left string, right string) int {
	leftParts := strings.Split(strings.TrimSpace(left), ".")
	rightParts := strings.Split(strings.TrimSpace(right), ".")
	maxLen := len(leftParts)
	if len(rightParts) > maxLen {
		maxLen = len(rightParts)
	}
	for i := 0; i < maxLen; i++ {
		li := 0
		ri := 0
		if i < len(leftParts) {
			li = parseVersionPart(leftParts[i])
		}
		if i < len(rightParts) {
			ri = parseVersionPart(rightParts[i])
		}
		if li < ri {
			return -1
		}
		if li > ri {
			return 1
		}
	}
	return 0
}

func parseVersionPart(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}

type CanvasItemV2 struct {
	Title         string               `json:"title"`
	Subtitle      string               `json:"subtitle"`
	Kind          model.CanvasKind     `json:"kind"`
	Rows          []CanvasRowV2        `json:"rows"`
	Selected      []bool               `json:"selected"`
	MSAFlags      []bool               `json:"msa_flags,omitempty"`
	SourceLabel   string               `json:"source_label,omitempty"`
	ImportedFrom  string               `json:"imported_from,omitempty"`
	ActiveColumns []model.CanvasColumn `json:"active_columns,omitempty"`
}

type CanvasRowV2 struct {
	RowNumber         int                        `json:"row_number"`
	Kind              model.CanvasKind           `json:"kind"`
	DisplayName       string                     `json:"display_name,omitempty"`
	DisplayNameLocked bool                       `json:"display_name_locked,omitempty"`
	KeywordRow        *model.KeywordResultRow    `json:"keyword_row,omitempty"`
	BlastRow          *model.BlastResultRow      `json:"blast_row,omitempty"`
	FASTA             *model.QuerySequenceSource `json:"fasta,omitempty"`
	SequenceData      *model.ProteinSequenceData `json:"sequence_data,omitempty"`
	SequenceReady     *bool                      `json:"sequence_ready,omitempty"`
}

type BlastRunV2 struct {
	Index           int                `json:"index"`
	Item            BlastQueryItemV2   `json:"item"`
	Request         model.BlastRequest `json:"request"`
	Results         model.BlastResult  `json:"results"`
	RowsBeforeMerge int                `json:"rows_before_merge"`
	RowsAfterMerge  int                `json:"rows_after_merge"`
}

type BlastQueryItemV2 struct {
	RawInput            string                       `json:"raw_input"`
	LabelName           string                       `json:"label_name"`
	Sequence            string                       `json:"sequence"`
	ProteinSequence     string                       `json:"protein_sequence"`
	NucleotideSequence  string                       `json:"nucleotide_sequence"`
	QuerySource         *model.QuerySequenceSource   `json:"query_source,omitempty"`
	FromKeyword         bool                         `json:"from_keyword"`
	FamilyName          string                       `json:"family_name"`
	MemberLabel         string                       `json:"member_label"`
	FamilyGroupSource   string                       `json:"family_group_source"`
	FamilyDetectionRule string                       `json:"family_detection_rule"`
	FamilySources       []*model.QuerySequenceSource `json:"family_sources,omitempty"`
	FamilySettings      model.FamilyBlastSettings    `json:"family_settings"`
}

type KeywordReviewStateV2 struct {
	SelectionState tui.RowSelectionState `json:"selection_state"`
}

type BlastReviewStateV2 struct {
	SingleSelectionState tui.RowSelectionState      `json:"single_selection_state"`
	MultiSelectionState  tui.BlastRunSelectionState `json:"multi_selection_state"`
}

type CanvasReviewStateV2 struct {
	SelectionState tui.BlastRunSelectionState `json:"selection_state"`
}

type SequenceCacheV2 struct {
	Entries []SequenceCacheEntryV2 `json:"entries"`
}

type SequenceCacheEntryV2 struct {
	TargetID       int    `json:"target_id"`
	SequenceID     string `json:"sequence_id"`
	Sequence       string `json:"sequence"`
	OriginalHeader string `json:"original_header"`
}

type ExportSettingsV2 struct {
	BaseName  string                 `json:"base_name"`
	OutputDir string                 `json:"output_dir"`
	Prompt    PromptExportSettingsV2 `json:"prompt"`
}

type PromptExportSettingsV2 struct {
	BaseName              string                `json:"base_name"`
	FolderName            string                `json:"folder_name"`
	WriteReport           bool                  `json:"write_report"`
	WriteSession          bool                  `json:"write_session"`
	WriteText             bool                  `json:"write_text"`
	WriteConvertedFasta   bool                  `json:"write_converted_fasta"`
	WriteAllRows          bool                  `json:"write_all_rows"`
	WriteExcel            bool                  `json:"write_excel"`
	WriteRawExcel         bool                  `json:"write_raw_excel"`
	FastaHeaderMode       model.FastaHeaderMode `json:"fasta_header_mode"`
	UsePhgoHeader         bool                  `json:"use_phgo_header"`
	PrependOnlyFirstQuery bool                  `json:"prepend_only_first_query"`
}

type ExternalReferenceSettingsV2 struct {
	AutoLabelBlastHits bool                                  `json:"auto_label_blast_hits"`
	UseUniProt         bool                                  `json:"use_uniprot"`
	UseInterPro        bool                                  `json:"use_interpro"`
	InterProSettings   model.InterProConservedRegionSettings `json:"interpro_settings"`
	MotifChecker       model.MotifCheckerSettings            `json:"motif_checker"`
}

type HandoffStateV2 struct {
	PendingMode           string                     `json:"pending_mode"`
	TransferKind          string                     `json:"transfer_kind"`
	TransferTargetDB      string                     `json:"transfer_target_database"`
	BlastProgramPath      string                     `json:"blast_program_path"`
	ReuseLastBlastInput   bool                       `json:"reuse_last_blast_input"`
	ReuseLastBlastRows    bool                       `json:"reuse_last_blast_rows"`
	ReuseLastKeywordRows  bool                       `json:"reuse_last_keyword_rows"`
	RewindBlastToInput    bool                       `json:"rewind_blast_to_input"`
	RewindKeywordToInput  bool                       `json:"rewind_keyword_to_input"`
	TransferSourceSpecies model.SpeciesCandidate     `json:"transfer_source_species"`
	TransferKeywordRows   []model.KeywordResultRow   `json:"transfer_keyword_rows"`
	TransferBlastRows     []model.BlastResultRow     `json:"transfer_blast_rows"`
	TransferPreparedItems []BlastQueryItemV2         `json:"transfer_prepared_items,omitempty"`
	TransferCanvasItems   []model.CanvasItem         `json:"transfer_canvas_items"`
	TransferCanvasCurrent int                        `json:"transfer_canvas_current"`
	TransferCanvasNextID  int                        `json:"transfer_canvas_next_id"`
	LastBlastItems        []BlastQueryItemV2         `json:"last_blast_items"`
	LastKeywordGroups     []model.KeywordSearchGroup `json:"last_keyword_groups"`
	LastKeywordReport     *ReportContextV2           `json:"last_keyword_report,omitempty"`
	LastKeywordSpecies    model.SpeciesCandidate     `json:"last_keyword_species"`
	LastBlastRowContext   *BlastRowContextV2         `json:"last_blast_row_context,omitempty"`
	LastBlastReview       *BlastReviewContextV2      `json:"last_blast_review_context,omitempty"`
}

type BlastRowContextV2 struct {
	Rows             []model.BlastResultRow    `json:"rows"`
	AllRows          []model.BlastResultRow    `json:"all_rows"`
	Numbers          []int                     `json:"numbers"`
	Flags            []bool                    `json:"flags"`
	SelectedRowsMask []bool                    `json:"selected_rows_mask"`
	Item             BlastQueryItemV2          `json:"item"`
	Selected         model.SpeciesCandidate    `json:"selected"`
	Request          model.BlastRequest        `json:"request"`
	Results          model.BlastResult         `json:"results"`
	Index            int                       `json:"index"`
	FilterSettings   model.BlastFilterSettings `json:"filter_settings"`
	FilterApplied    bool                      `json:"filter_applied"`
	FilterCleared    bool                      `json:"filter_cleared"`
	FamilySettings   model.FamilyBlastSettings `json:"family_settings"`
}

type BlastReviewContextV2 struct {
	Selected          model.SpeciesCandidate `json:"selected"`
	Prepared          []BlastQueryItemV2     `json:"prepared"`
	OriginalPrepared  []BlastQueryItemV2     `json:"original_prepared"`
	Runs              []BlastRunV2           `json:"runs"`
	OriginalRuns      []BlastRunV2           `json:"original_runs"`
	ConfiguredRequest model.BlastRequest     `json:"configured_request"`
	OriginalRunCount  int                    `json:"original_run_count"`
}

type CanvasOpenContextV2 struct {
	Items         []CanvasItemV2 `json:"items"`
	CurrentItem   int            `json:"current_item"`
	NextNumericID int            `json:"next_numeric_id"`
}

type ArtifactManifestV2 struct {
	Entries []ArtifactEntryV2 `json:"entries"`
}

type ArtifactEntryV2 struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	MediaType   string `json:"media_type"`
	Description string `json:"description"`
	SourcePath  string `json:"source_path,omitempty"`
	RestorePath string `json:"restore_path,omitempty"`
}

type RuntimeCacheV2 struct {
	BlastLabelLookups      []BlastLabelLookupCacheEntryV2      `json:"blast_label_lookups"`
	BlastHitLabelLookups   []BlastHitLabelLookupCacheEntryV2   `json:"blast_hit_label_lookups"`
	RowUniProtAccessions   []RowUniProtAccessionsCacheEntryV2  `json:"row_uniprot_accessions"`
	UniProtLookups         []UniProtLookupCacheEntryV2         `json:"uniprot_lookups"`
	InterProLookups        []InterProLookupCacheEntryV2        `json:"interpro_lookups"`
	KeywordBlastItems      []KeywordBlastItemCacheEntryV2      `json:"keyword_blast_items"`
	QuerySourceResolutions []QuerySourceResolutionCacheEntryV2 `json:"query_source_resolutions"`
	KeywordTermRows        []KeywordTermRowsCacheEntryV2       `json:"keyword_term_rows"`
	ProteinSequences       []ProteinSequenceCacheEntryV2       `json:"protein_sequences"`
	ProteinSequenceMisses  []ProteinSequenceMissCacheEntryV2   `json:"protein_sequence_misses"`
	SpeciesCandidates      []SpeciesCandidatesCacheEntryV2     `json:"species_candidates"`
}

type BlastLabelLookupCacheEntryV2 struct {
	Key           string   `json:"key"`
	Label         string   `json:"label"`
	Aliases       []string `json:"aliases"`
	TaskTimestamp string   `json:"task_timestamp"`
	ItemIndex     int      `json:"item_index"`
}

type BlastHitLabelLookupCacheEntryV2 struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	LabelType string   `json:"label_type"`
	Aliases   []string `json:"aliases"`
}

type RowUniProtAccessionsCacheEntryV2 struct {
	Key        string   `json:"key"`
	Known      bool     `json:"known"`
	Accessions []string `json:"accessions"`
}

type UniProtLookupCacheEntryV2 struct {
	Key   string        `json:"key"`
	Entry uniprot.Entry `json:"entry"`
	OK    bool          `json:"ok"`
	Error string        `json:"error,omitempty"`
}

type InterProLookupCacheEntryV2 struct {
	Key   string         `json:"key"`
	Entry interpro.Entry `json:"entry"`
	OK    bool           `json:"ok"`
	Error string         `json:"error,omitempty"`
}

type KeywordBlastItemCacheEntryV2 struct {
	Key  string           `json:"key"`
	Item BlastQueryItemV2 `json:"item"`
}

type QuerySourceResolutionCacheEntryV2 struct {
	Key    string                    `json:"key"`
	Source model.QuerySequenceSource `json:"source"`
}

type KeywordTermRowsCacheEntryV2 struct {
	Key  string                   `json:"key"`
	Rows []model.KeywordResultRow `json:"rows"`
}

type ProteinSequenceCacheEntryV2 struct {
	Key      string                    `json:"key"`
	Sequence model.ProteinSequenceData `json:"sequence"`
}

type ProteinSequenceMissCacheEntryV2 struct {
	Key   string `json:"key"`
	Error string `json:"error"`
}

type SpeciesCandidatesCacheEntryV2 struct {
	Key        string                   `json:"key"`
	Candidates []model.SpeciesCandidate `json:"candidates"`
}

type manifestXML struct {
	XMLName xml.Name         `xml:"phgoSessionSnapshot"`
	Format  string           `xml:"format,attr"`
	Version string           `xml:"version,attr"`
	Created string           `xml:"created,attr"`
	Modules []manifestModule `xml:"modules>module"`
}

type manifestModule struct {
	Name    string `xml:"name,attr"`
	Version string `xml:"version,attr"`
	Path    string `xml:"path,attr"`
}

type moduleXML struct {
	XMLName xml.Name `xml:"module"`
	Name    string   `xml:"name,attr"`
	Version string   `xml:"version,attr"`
	Payload string   `xml:"payload"`
}

type ContextV1 = ContextV2
type KeywordResultV1 = KeywordResultV2
type ReportContextV1 = ReportContextV2
type BlastResultV1 = BlastResultV2
type CanvasResultV1 = CanvasResultV2
type CanvasItemV1 = CanvasItemV2
type CanvasRowV1 = CanvasRowV2
type BlastRunV1 = BlastRunV2
type BlastQueryItemV1 = BlastQueryItemV2
type KeywordReviewStateV1 = KeywordReviewStateV2
type BlastReviewStateV1 = BlastReviewStateV2
type CanvasReviewStateV1 = CanvasReviewStateV2
type SequenceCacheV1 = SequenceCacheV2
type SequenceCacheEntryV1 = SequenceCacheEntryV2

func WriteFile(path string, snapshot Snapshot) error {
	path = ensureExtension(strings.TrimSpace(path))
	if path == "" {
		return fmt.Errorf("empty session snapshot path")
	}
	if snapshot.Context.FormatName == "" {
		snapshot.Context.FormatName = FormatName
	}
	if snapshot.Context.FormatVersion == "" {
		snapshot.Context.FormatVersion = FormatVersion
	}
	if snapshot.Context.CreatedAt.IsZero() {
		snapshot.Context.CreatedAt = time.Now()
	}
	sanitizeSnapshotSourceWebURLs(&snapshot)

	modules := []manifestModule{}
	payloads := map[string]any{}
	addModule := func(name string, version string, path string, payload any) {
		modules = append(modules, manifestModule{Name: name, Version: version, Path: path})
		payloads[path] = payload
	}

	addModule(contextModuleName, "2", contextModulePath, snapshot.Context)
	if snapshot.Keyword != nil {
		addModule(keywordModuleName, "2", keywordModulePath, snapshot.Keyword)
	}
	if snapshot.KeywordSource != nil {
		addModule(keywordSourceName, "4", keywordSourceModulePath, snapshot.KeywordSource)
	}
	if snapshot.Blast != nil {
		addModule(blastModuleName, "2", blastModulePath, snapshot.Blast)
	}
	if snapshot.Canvas != nil {
		addModule(canvasModuleName, "2", canvasModulePath, snapshot.Canvas)
	}
	if snapshot.CanvasMSA != nil {
		addModule(canvasMSAName, "1", canvasMSAModulePath, snapshot.CanvasMSA)
	}
	if snapshot.KeywordReview != nil {
		addModule(keywordReviewName, "2", keywordReviewModulePath, snapshot.KeywordReview)
	}
	if snapshot.BlastReview != nil {
		addModule(blastReviewName, "2", blastReviewModulePath, snapshot.BlastReview)
	}
	if snapshot.CanvasReview != nil {
		addModule(canvasReviewName, "2", canvasReviewModulePath, snapshot.CanvasReview)
	}
	if snapshot.SequenceCache != nil {
		addModule(sequenceCacheName, "2", sequenceCacheModulePath, snapshot.SequenceCache)
	}
	if snapshot.ExportSettings != nil {
		addModule(exportSettingsName, "2", exportModulePath, snapshot.ExportSettings)
	}
	if snapshot.ExternalReferences != nil {
		addModule(externalReferencesName, "2", referenceModulePath, snapshot.ExternalReferences)
	}
	if snapshot.Handoff != nil {
		addModule(handoffStateName, "2", handoffModulePath, snapshot.Handoff)
	}
	if snapshot.Artifacts != nil {
		addModule(artifactManifestName, "2", artifactModulePath, snapshot.Artifacts)
	}
	if snapshot.RuntimeCache != nil {
		addModule(runtimeCacheName, "2", runtimeCacheModulePath, snapshot.RuntimeCache)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	manifestData, err := xml.MarshalIndent(manifestXML{
		Format:  FormatName,
		Version: FormatVersion,
		Created: snapshot.Context.CreatedAt.Format(time.RFC3339Nano),
		Modules: modules,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot manifest: %w", err)
	}
	if err := writeZipFile(zw, "manifest.xml", append([]byte(xml.Header), manifestData...)); err != nil {
		_ = zw.Close()
		return err
	}
	for _, module := range modules {
		payload, ok := payloads[module.Path]
		if !ok {
			continue
		}
		data, err := marshalModule(module.Name, module.Version, payload)
		if err != nil {
			_ = zw.Close()
			return err
		}
		if err := writeZipFile(zw, module.Path, data); err != nil {
			_ = zw.Close()
			return err
		}
	}
	if snapshot.Artifacts != nil {
		for _, artifact := range snapshot.Artifacts.Entries {
			artifactPath := strings.TrimSpace(artifact.Path)
			if artifactPath == "" {
				_ = zw.Close()
				return fmt.Errorf("artifact entry %q has empty archive path", artifact.ID)
			}
			payload, ok := snapshot.ArtifactPayloads[artifactPath]
			if !ok {
				_ = zw.Close()
				return fmt.Errorf("artifact payload %q is missing", artifactPath)
			}
			if err := writeZipFile(zw, artifactPath, payload); err != nil {
				_ = zw.Close()
				return err
			}
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("close snapshot archive: %w", err)
	}
	return appfs.WriteFileAtomic(path, buf.Bytes(), 0o600)
}

func ReadFile(path string) (Snapshot, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Snapshot{}, fmt.Errorf("empty session snapshot path")
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("open session snapshot: %w", err)
	}
	defer reader.Close()

	files := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		files[strings.TrimSpace(file.Name)] = file
	}
	manifestFile := files["manifest.xml"]
	if manifestFile == nil {
		return Snapshot{}, fmt.Errorf("session snapshot has no manifest.xml")
	}
	var manifest manifestXML
	if err := readXMLFile(manifestFile, &manifest); err != nil {
		return Snapshot{}, fmt.Errorf("read snapshot manifest: %w", err)
	}
	if manifest.Format != "" && manifest.Format != FormatName {
		return Snapshot{}, fmt.Errorf("unsupported snapshot format %q", manifest.Format)
	}
	var snapshot Snapshot
	for _, module := range manifest.Modules {
		file := files[strings.TrimSpace(module.Path)]
		if file == nil {
			continue
		}
		switch module.Name {
		case contextModuleName:
			var context ContextV2
			if err := readModule(file, &context); err != nil {
				return Snapshot{}, err
			}
			snapshot.Context = context
		case keywordModuleName:
			var keyword KeywordResultV2
			if err := readModule(file, &keyword); err != nil {
				return Snapshot{}, err
			}
			snapshot.Keyword = &keyword
		case keywordSourceName:
			var source KeywordSourceStateV4
			if err := readModule(file, &source); err != nil {
				return Snapshot{}, err
			}
			snapshot.KeywordSource = &source
		case blastModuleName:
			var blast BlastResultV2
			if err := readModule(file, &blast); err != nil {
				return Snapshot{}, err
			}
			snapshot.Blast = &blast
		case canvasModuleName:
			var canvas CanvasResultV2
			if err := readModule(file, &canvas); err != nil {
				return Snapshot{}, err
			}
			snapshot.Canvas = &canvas
		case canvasMSAName:
			var msa CanvasMSAV1
			if err := readModule(file, &msa); err != nil {
				return Snapshot{}, err
			}
			snapshot.CanvasMSA = &msa
		case keywordReviewName:
			var review KeywordReviewStateV2
			if err := readModule(file, &review); err != nil {
				return Snapshot{}, err
			}
			snapshot.KeywordReview = &review
		case blastReviewName:
			var review BlastReviewStateV2
			if err := readModule(file, &review); err != nil {
				return Snapshot{}, err
			}
			snapshot.BlastReview = &review
		case canvasReviewName:
			var review CanvasReviewStateV2
			if err := readModule(file, &review); err != nil {
				return Snapshot{}, err
			}
			snapshot.CanvasReview = &review
		case sequenceCacheName:
			var cache SequenceCacheV2
			if err := readModule(file, &cache); err != nil {
				return Snapshot{}, err
			}
			snapshot.SequenceCache = &cache
		case exportSettingsName:
			var exportSettings ExportSettingsV2
			if err := readModule(file, &exportSettings); err != nil {
				return Snapshot{}, err
			}
			snapshot.ExportSettings = &exportSettings
		case externalReferencesName:
			var references ExternalReferenceSettingsV2
			if err := readModule(file, &references); err != nil {
				return Snapshot{}, err
			}
			snapshot.ExternalReferences = &references
		case handoffStateName:
			var handoff HandoffStateV2
			if err := readModule(file, &handoff); err != nil {
				return Snapshot{}, err
			}
			snapshot.Handoff = &handoff
		case artifactManifestName:
			var artifacts ArtifactManifestV2
			if err := readModule(file, &artifacts); err != nil {
				return Snapshot{}, err
			}
			snapshot.Artifacts = &artifacts
		case runtimeCacheName:
			var runtimeCache RuntimeCacheV2
			if err := readModule(file, &runtimeCache); err != nil {
				return Snapshot{}, err
			}
			snapshot.RuntimeCache = &runtimeCache
		}
	}
	if snapshot.Artifacts != nil && len(snapshot.Artifacts.Entries) > 0 {
		snapshot.ArtifactPayloads = make(map[string][]byte, len(snapshot.Artifacts.Entries))
		for _, artifact := range snapshot.Artifacts.Entries {
			artifactPath := strings.TrimSpace(artifact.Path)
			if artifactPath == "" {
				continue
			}
			file := files[artifactPath]
			if file == nil {
				return Snapshot{}, fmt.Errorf("artifact payload %q is missing from snapshot archive", artifactPath)
			}
			data, err := readRawZipFile(file)
			if err != nil {
				return Snapshot{}, fmt.Errorf("read artifact payload %q: %w", artifactPath, err)
			}
			snapshot.ArtifactPayloads[artifactPath] = data
		}
	}
	if snapshot.Context.FormatName == "" {
		snapshot.Context.FormatName = FormatName
	}
	if snapshot.Context.FormatVersion == "" {
		snapshot.Context.FormatVersion = strings.TrimSpace(manifest.Version)
		if snapshot.Context.FormatVersion == "" {
			snapshot.Context.FormatVersion = FormatVersion
		}
	}
	sanitizeSnapshotSourceWebURLs(&snapshot)
	return snapshot, nil
}

func sanitizeSnapshotSourceWebURLs(snapshot *Snapshot) {
	if snapshot == nil {
		return
	}
	if snapshot.Keyword != nil {
		for groupIndex := range snapshot.Keyword.Groups {
			model.SanitizeKeywordResultRowsSourceWebURLs(snapshot.Keyword.Groups[groupIndex].Rows)
		}
	}
	if snapshot.Blast != nil {
		for runIndex := range snapshot.Blast.Runs {
			model.SanitizeBlastResultRowsSourceWebURLs(snapshot.Blast.Runs[runIndex].Results.Rows)
		}
	}
	if snapshot.Canvas != nil {
		sanitizeSnapshotCanvasItemsSourceWebURLs(snapshot.Canvas.Items)
	}
	if snapshot.RuntimeCache != nil {
		for entryIndex := range snapshot.RuntimeCache.KeywordTermRows {
			model.SanitizeKeywordResultRowsSourceWebURLs(snapshot.RuntimeCache.KeywordTermRows[entryIndex].Rows)
		}
	}
	if snapshot.Handoff != nil {
		model.SanitizeKeywordResultRowsSourceWebURLs(snapshot.Handoff.TransferKeywordRows)
		model.SanitizeBlastResultRowsSourceWebURLs(snapshot.Handoff.TransferBlastRows)
		sanitizeModelCanvasItemsSourceWebURLs(snapshot.Handoff.TransferCanvasItems)
		if snapshot.Handoff.LastBlastRowContext != nil {
			model.SanitizeBlastResultRowsSourceWebURLs(snapshot.Handoff.LastBlastRowContext.Rows)
			model.SanitizeBlastResultRowsSourceWebURLs(snapshot.Handoff.LastBlastRowContext.AllRows)
		}
		if snapshot.Handoff.LastKeywordGroups != nil {
			for groupIndex := range snapshot.Handoff.LastKeywordGroups {
				model.SanitizeKeywordResultRowsSourceWebURLs(snapshot.Handoff.LastKeywordGroups[groupIndex].Rows)
			}
		}
		if snapshot.Handoff.LastBlastReview != nil {
			for runIndex := range snapshot.Handoff.LastBlastReview.Runs {
				model.SanitizeBlastResultRowsSourceWebURLs(snapshot.Handoff.LastBlastReview.Runs[runIndex].Results.Rows)
			}
			for runIndex := range snapshot.Handoff.LastBlastReview.OriginalRuns {
				model.SanitizeBlastResultRowsSourceWebURLs(snapshot.Handoff.LastBlastReview.OriginalRuns[runIndex].Results.Rows)
			}
		}
	}
}

func sanitizeSnapshotCanvasItemsSourceWebURLs(items []CanvasItemV2) {
	for itemIndex := range items {
		for rowIndex := range items[itemIndex].Rows {
			row := &items[itemIndex].Rows[rowIndex]
			if row.KeywordRow != nil {
				model.SanitizeKeywordResultRowSourceWebURL(row.KeywordRow)
			}
			if row.BlastRow != nil {
				row.BlastRow.GeneReportURL = ""
			}
		}
	}
}

func sanitizeModelCanvasItemsSourceWebURLs(items []model.CanvasItem) {
	for itemIndex := range items {
		for rowIndex := range items[itemIndex].Rows {
			row := &items[itemIndex].Rows[rowIndex]
			if row.KeywordRow != nil {
				model.SanitizeKeywordResultRowSourceWebURL(row.KeywordRow)
			}
			if row.BlastRow != nil {
				row.BlastRow.GeneReportURL = ""
			}
		}
	}
}

func ResolveOpenPath(input string, outputDir string) (string, error) {
	input = strings.TrimSpace(strings.Trim(input, `"'`))
	if input == "" {
		return "", fmt.Errorf("session snapshot path cannot be empty")
	}
	candidates := []string{input}
	if filepath.Ext(input) == "" {
		candidates = append(candidates, input+FileExtension)
	}
	if !filepath.IsAbs(input) {
		base := strings.TrimSpace(outputDir)
		if base == "" {
			if dir, err := appfs.OutputDir(); err == nil {
				base = dir
			}
		}
		if base != "" {
			candidates = append(candidates, filepath.Join(base, input))
			if filepath.Ext(input) == "" {
				candidates = append(candidates, filepath.Join(base, input+FileExtension))
			}
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return filepath.Abs(candidate)
		}
	}
	return "", fmt.Errorf("session snapshot %q was not found; absolute paths are read directly, relative names are resolved from output", input)
}

func DefaultFilePath(outputDir string, baseName string) string {
	baseName = strings.TrimSpace(baseName)
	if baseName == "" {
		baseName = strings.TrimSpace(filepath.Base(outputDir))
	}
	if baseName == "" || strings.EqualFold(baseName, "output") || baseName == "." || baseName == string(filepath.Separator) {
		baseName = time.Now().Format("20060102_150405") + "_session"
	}
	return filepath.Join(outputDir, ensureExtension(sanitizeFileName(baseName)))
}

func ensureExtension(path string) string {
	if path == "" {
		return ""
	}
	if strings.EqualFold(filepath.Ext(path), FileExtension) {
		return path
	}
	return path + FileExtension
}

func marshalModule(name string, version string, payload any) ([]byte, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal %s module payload: %w", name, err)
	}
	data, err := xml.MarshalIndent(moduleXML{Name: name, Version: version, Payload: string(jsonData)}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal %s module XML: %w", name, err)
	}
	return append([]byte(xml.Header), data...), nil
}

func readModule(file *zip.File, out any) error {
	var module moduleXML
	if err := readXMLFile(file, &module); err != nil {
		return fmt.Errorf("read snapshot module %s: %w", file.Name, err)
	}
	if err := json.Unmarshal([]byte(module.Payload), out); err != nil {
		return fmt.Errorf("decode snapshot module %s payload: %w", file.Name, err)
	}
	return nil
}

func readXMLFile(file *zip.File, out any) error {
	data, err := readRawZipFile(file)
	if err != nil {
		return err
	}
	return xml.Unmarshal(data, out)
}

func readRawZipFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	writer, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("create snapshot archive entry %s: %w", name, err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("write snapshot archive entry %s: %w", name, err)
	}
	return nil
}

func sanitizeFileName(value string) string {
	value = strings.TrimSpace(value)
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	value = replacer.Replace(value)
	value = strings.Join(strings.Fields(value), "_")
	value = strings.Trim(value, " ._")
	if value == "" {
		return "session"
	}
	return value
}

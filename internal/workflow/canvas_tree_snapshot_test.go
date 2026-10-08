package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/phylo"
	"github.com/KiriKirby/phytozome-go/internal/sessionsnapshot"
	"github.com/KiriKirby/phytozome-go/internal/source"
	"github.com/KiriKirby/phytozome-go/internal/tui"
)

func TestSnapshotCanvasTreeArtifactsPacksRuntimeFiles(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"input.fasta":           ">PHGOT000001\nMPEPTIDE\n",
		"input.meta.json":       `{"schema_version":1}`,
		"runtime-request.json":  `{"schema_version":1}`,
		"runtime-response.json": `{"schema_version":1}`,
		"aligned.fasta":         ">PHGOT000001\nMPEPTIDE\n",
		"tree.nwk":              "(PHGOT000001);",
		"viewer.payload.json":   `{"schema_version":1}`,
		"run.manifest.json":     `{"schema_version":1}`,
		"runtime.stdout.txt":    "ok",
		"runtime.stderr.txt":    "",
		"runtime-summary.txt":   "summary",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	manifest, payloads, err := snapshotCanvasTreeArtifacts(phylo.RunPlan{
		SessionID: "canvas session",
		RunID:     "run:1",
		BaseDir:   dir,
	})
	if err != nil {
		t.Fatalf("snapshotCanvasTreeArtifacts returned error: %v", err)
	}
	if manifest == nil || len(manifest.Entries) != len(files) {
		t.Fatalf("manifest entries = %#v, want %d", manifest, len(files))
	}
	for _, entry := range manifest.Entries {
		if !strings.HasPrefix(entry.Path, "artifacts/tree/canvas_session/run_1/") {
			t.Fatalf("unexpected archive path: %s", entry.Path)
		}
		if len(payloads[entry.Path]) == 0 && filepath.Base(entry.Path) != "runtime.stderr.txt" {
			t.Fatalf("missing payload for %s", entry.Path)
		}
		if entry.RestorePath == "" {
			t.Fatalf("missing restore path for %s", entry.Path)
		}
	}
}

func TestSnapshotCanvasTreeStateKeepsPanelWithoutRun(t *testing.T) {
	w := NewBlastWizard(nil)
	panel := treePanelForSnapshotTest()
	panel.Expanded = true
	panel.Focused = true
	panel.CurrentControl = 2
	panel.ScrollOffset = 7
	w.prompt.RestoreCanvasTreePanelState(canvasStateKey("canvas"), panel)
	tree, manifest, payloads, err := w.snapshotCanvasTreeState()
	if err != nil {
		t.Fatalf("snapshotCanvasTreeState returned error: %v", err)
	}
	if tree == nil || !tree.PanelState.EnabledEver || tree.PanelState.AlignmentMethod != string(phylo.AlignmentMUSCLE) {
		t.Fatalf("tree panel state not captured: %#v", tree)
	}
	if tree.PanelState.Expanded || tree.PanelState.Focused || tree.PanelState.CurrentControl != 0 || tree.PanelState.ScrollOffset != 0 {
		t.Fatalf("tree panel snapshot should drop UI-open state: %#v", tree.PanelState)
	}
	if manifest != nil || payloads != nil {
		t.Fatalf("unexpected artifact bundle without a run: %#v %#v", manifest, payloads)
	}
}

func TestSnapshotCanvasTreeStateCapturesViewerState(t *testing.T) {
	w := NewBlastWizard(nil)
	w.prompt.RestoreCanvasTreePanelState(canvasStateKey("canvas"), treePanelForSnapshotTest())
	ctx := context.Background()
	server, _, err := w.ensureCanvasTreeViewer(ctx)
	if err != nil {
		t.Fatalf("ensureCanvasTreeViewer returned error: %v", err)
	}
	defer w.closeCanvasTreeViewer()

	viewerState := json.RawMessage(`{"schema_version":2,"reactree":{"layout":"rectangular","fontFamily":"Georgia","searchText":"PAL","openMenu":"file"},"phgo":{"split_percent":42,"viewport":{"inner_width":1200}}}`)
	if err := putViewerState(ctx, server, w.canvasTreeSessionID(), viewerState); err != nil {
		t.Fatalf("putViewerState returned error: %v", err)
	}

	tree, _, _, err := w.snapshotCanvasTreeState()
	if err != nil {
		t.Fatalf("snapshotCanvasTreeState returned error: %v", err)
	}
	if tree == nil {
		t.Fatal("snapshotCanvasTreeState returned nil tree")
	}
	if strings.Contains(string(tree.ViewerState), "searchText") || strings.Contains(string(tree.ViewerState), "openMenu") || strings.Contains(string(tree.ViewerState), "viewport") {
		t.Fatalf("viewer state should drop UI-only fields: %s", tree.ViewerState)
	}
	if !strings.Contains(string(tree.ViewerState), `"layout":"rectangular"`) || !strings.Contains(string(tree.ViewerState), `"split_percent":42`) {
		t.Fatalf("viewer state should keep durable viewer settings: %s", tree.ViewerState)
	}
	if string(w.canvasTreeViewerState) != string(viewerState) {
		t.Fatalf("wizard viewer state cache not updated: %s", w.canvasTreeViewerState)
	}
}

func TestSnapshotCanvasMSAStateCapturesFlagsAndJalviewState(t *testing.T) {
	w := NewBlastWizard(nil)
	now := time.Now()
	payload := phylo.ViewerPayload{
		SchemaVersion: 1,
		SessionID:     "canvas",
		UpdatedAt:     now,
		AlignedFASTA:  ">PHGOT000001\nAAA\n",
		Metadata: phylo.Metadata{Records: []phylo.InputRecord{
			{TaxonID: "PHGOT000001", DisplayName: "green", CanvasItem: "msa", CanvasRow: 0},
		}},
	}
	w.canvasTreeLastMSAPayload = payload
	items := []model.CanvasItem{{
		Title:    "msa",
		Selected: []bool{true, false, false},
		MSAFlags: []bool{false, true, false},
		Rows: []model.CanvasRow{
			{Kind: model.CanvasKindFasta, DisplayName: "green"},
			{Kind: model.CanvasKindFasta, DisplayName: "yellow"},
			{Kind: model.CanvasKindFasta, DisplayName: "red"},
		},
	}}
	server := phylo.NewViewerServer("127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("viewer server start: %v", err)
	}
	w.canvasTreeViewer = server
	server.SetMSAPayload("canvas", payload)
	server.SetMSAState("canvas", phylo.MSAState{
		SchemaVersion: 1,
		Rows: []phylo.MSASelectionRow{
			{TaxonID: "PHGOT000001", State: "green"},
		},
		Groups: []map[string]any{{"name": "manual"}},
	})

	msa := w.snapshotCanvasMSAState(items)
	if msa == nil || len(msa.State.Rows) != 1 {
		t.Fatalf("MSA snapshot missing rows: %#v", msa)
	}
	if msa.State.Rows[0].State != "green" || msa.State.Rows[0].TaxonID != "PHGOT000001" {
		t.Fatalf("MSA snapshot row states should follow shared selected payload only: %#v", msa.State.Rows)
	}
	if len(msa.State.Groups) != 1 || msa.LastPayload.AlignedFASTA == "" {
		t.Fatalf("MSA snapshot should keep Jalview groups and payload: %#v", msa)
	}
}

func TestSnapshotCanvasTreeStateSyncsCurrentDisplayNamesWithoutRecompute(t *testing.T) {
	w := NewBlastWizard(nil)
	now := time.Now()
	panel := treePanelForSnapshotTest()
	w.prompt.RestoreCanvasTreePanelState(canvasStateKey("canvas"), panel)
	record := phylo.InputRecord{
		TaxonID:      "PHGOT000001",
		DisplayName:  "Old PAL",
		SourceType:   string(model.CanvasKindFasta),
		OriginalHead: "Old PAL",
		CanvasItem:   "canvas 1",
		CanvasRow:    0,
		TableValues:  map[string]string{"label_name": "Old PAL", "head": "Old PAL"},
	}
	meta := phylo.Metadata{SchemaVersion: 1, GeneratedAt: now, DisplayNameSource: "label_name", Records: []phylo.InputRecord{record}}
	w.canvasTreeLastPayload = phylo.ViewerPayload{SchemaVersion: 1, SessionID: "canvas", UpdatedAt: now, Newick: "(PHGOT000001);", AlignedFASTA: ">PHGOT000001\nMPEPTIDE\n", Metadata: meta}
	w.canvasTreeLastPlan = phylo.RunPlan{
		SessionID:    "canvas",
		RunID:        "run1",
		BaseDir:      t.TempDir(),
		Settings:     treeSettingsFromSnapshotPanel(panel),
		Records:      []phylo.InputRecord{record},
		Metadata:     meta,
		AlignedFASTA: ">PHGOT000001\nMPEPTIDE\n",
		Newick:       "(PHGOT000001);",
		Fingerprints: phylo.Fingerprints{Alignment: "align", Tree: "tree", Preview: "old-preview"},
	}
	items := []model.CanvasItem{{
		Title:    "canvas 1",
		Selected: []bool{true},
		Rows: []model.CanvasRow{{
			Kind:              model.CanvasKindFasta,
			DisplayName:       "New PAL",
			DisplayNameLocked: true,
			FASTA:             &model.QuerySequenceSource{Annotation: "raw head", LabelName: "Old PAL", Sequence: "MPEPTIDE", SequenceKind: model.SequenceProtein},
		}},
	}}
	tree, _, _, err := w.snapshotCanvasTreeState(items)
	if err != nil {
		t.Fatalf("snapshotCanvasTreeState returned error: %v", err)
	}
	if tree.LastPayload.Metadata.Records[0].DisplayName != "New PAL" || tree.LastPayload.Metadata.Records[0].DisplayPrefix != "[1,1]" {
		t.Fatalf("snapshot payload did not sync latest display name: %#v", tree.LastPayload.Metadata.Records)
	}
	if tree.Fingerprints.Alignment != "align" || tree.Fingerprints.Tree != "tree" {
		t.Fatalf("compute fingerprints should not change after display-name sync: %#v", tree.Fingerprints)
	}
	if tree.Fingerprints.Preview == "old-preview" || tree.Fingerprints.Preview == "" {
		t.Fatalf("preview fingerprint should update after display-name sync: %#v", tree.Fingerprints)
	}
}

func TestLegacyShowCanvasCoordinatesDoesNotChangeFingerprints(t *testing.T) {
	now := time.Now()
	records := []phylo.InputRecord{{
		TaxonID:         "PHGOT000001",
		DisplayName:     "PAL1",
		Sequence:        "MPEPTIDE",
		SequenceKind:    phylo.SequenceProtein,
		CanvasItemIndex: 0,
		CanvasRow:       0,
		RowFingerprint:  "row-1",
	}}
	settings := phylo.NormalizeTreeSettingsForKind(phylo.DefaultTreeSettings(), phylo.SequenceProtein)
	meta := phylo.Metadata{SchemaVersion: 1, GeneratedAt: now, DisplayNameSource: "label_name", Records: records}
	planWithCoords, err := phylo.BuildRunPlan("canvas", "coords", t.TempDir(), settings, phylo.SequenceProtein, records, meta, "", "", now)
	if err != nil {
		t.Fatalf("BuildRunPlan coords: %v", err)
	}
	settingsNoCoords := settings
	settingsNoCoords.ShowCanvasCoordinates = false
	settingsNoCoords.ShowCanvasCoordinatesSet = true
	planWithoutCoords, err := phylo.BuildRunPlan("canvas", "no-coords", t.TempDir(), settingsNoCoords, phylo.SequenceProtein, records, meta, "", "", now)
	if err != nil {
		t.Fatalf("BuildRunPlan no coords: %v", err)
	}
	if planWithCoords.Fingerprints.Alignment != planWithoutCoords.Fingerprints.Alignment || planWithCoords.Fingerprints.Tree != planWithoutCoords.Fingerprints.Tree {
		t.Fatalf("legacy coordinate setting should not affect compute fingerprints: with=%#v without=%#v", planWithCoords.Fingerprints, planWithoutCoords.Fingerprints)
	}
	if planWithCoords.Fingerprints.Preview != planWithoutCoords.Fingerprints.Preview {
		t.Fatalf("legacy coordinate display option should not affect preview fingerprint")
	}
}

func TestApplyCanvasCoordinateMetadataDoesNotMutateCanvasDisplayNameColumn(t *testing.T) {
	item := model.CanvasItem{
		Title:    "family A",
		Selected: []bool{true},
		Rows: []model.CanvasRow{{
			Kind:        model.CanvasKindFasta,
			DisplayName: "PAL display",
			FASTA:       &model.QuerySequenceSource{Sequence: "MPEPTIDE", SequenceKind: model.SequenceProtein},
		}},
	}
	w := NewBlastWizard(nil)
	state := canvasLaunchState{Items: []model.CanvasItem{item}}
	selected := selectedCanvasRowsInOrder(state.Items)
	settings := phylo.DefaultTreeSettings()
	settings.DisplayNameSource = "display_name"
	result, err := w.buildCanvasTreeArtifactsWithRuntime(context.Background(), state, selected, settings, workflowStaticRuntime{})
	if err != nil {
		t.Fatalf("buildCanvasTreeArtifactsWithRuntime returned error: %v", err)
	}
	if state.Items[0].Rows[0].DisplayName != "PAL display" {
		t.Fatalf("Canvas display_name column was mutated: %#v", state.Items[0].Rows[0])
	}
	if got := result.Plan.Metadata.Records[0].DisplayName; got != "PAL display" {
		t.Fatalf("metadata display name = %q, want raw display name", got)
	}
	if got := result.Plan.Metadata.Records[0].DisplayPrefix; got != "[1,1]" {
		t.Fatalf("metadata display prefix = %q, want coordinate prefix", got)
	}

	settings.ShowCanvasCoordinates = false
	settings.ShowCanvasCoordinatesSet = true
	result, err = w.buildCanvasTreeArtifactsWithRuntime(context.Background(), state, selected, settings, workflowStaticRuntime{})
	if err != nil {
		t.Fatalf("buildCanvasTreeArtifactsWithRuntime without coords returned error: %v", err)
	}
	if got := result.Plan.Metadata.Records[0].DisplayName; got != "PAL display" {
		t.Fatalf("metadata display name with legacy coordinates off = %q, want raw display name", got)
	}
	if got := result.Plan.Metadata.Records[0].DisplayPrefix; got != "[1,1]" {
		t.Fatalf("metadata display prefix with legacy coordinates off = %q, want coordinate prefix", got)
	}
}

func TestSnapshotCanvasTreeStateUsesVisibleCanvasSortOrder(t *testing.T) {
	w := NewBlastWizard(nil)
	now := time.Now()
	panel := treePanelForSnapshotTest()
	w.prompt.RestoreCanvasTreePanelState(canvasStateKey("canvas"), panel)
	w.prompt.RestoreCanvasReviewState(canvasStateKey("canvas"), tui.BlastRunSelectionState{
		Valid: true,
		Sort:  tui.TableSort{Column: 1, Direction: tui.SortAscending},
	})
	records := []phylo.InputRecord{
		{
			TaxonID:      "PHGOT000001",
			DisplayName:  "beta",
			SourceType:   string(model.CanvasKindFasta),
			OriginalHead: "beta",
			CanvasItem:   "canvas 1",
			CanvasRow:    0,
			TableValues:  map[string]string{"label_name": "beta", "head": "beta"},
		},
		{
			TaxonID:      "PHGOT000002",
			DisplayName:  "alpha",
			SourceType:   string(model.CanvasKindFasta),
			OriginalHead: "alpha",
			CanvasItem:   "canvas 1",
			CanvasRow:    1,
			TableValues:  map[string]string{"label_name": "alpha", "head": "alpha"},
		},
	}
	meta := phylo.Metadata{SchemaVersion: 1, GeneratedAt: now, DisplayNameSource: "label_name", Records: records}
	w.canvasTreeLastPayload = phylo.ViewerPayload{
		SchemaVersion: 1,
		SessionID:     "canvas",
		UpdatedAt:     now,
		Newick:        "(PHGOT000001,PHGOT000002);",
		AlignedFASTA:  ">PHGOT000001\nBBB\n>PHGOT000002\nAAA\n",
		Metadata:      meta,
	}
	w.canvasTreeLastPlan = phylo.RunPlan{
		SessionID:    "canvas",
		RunID:        "run1",
		BaseDir:      t.TempDir(),
		Settings:     treeSettingsFromSnapshotPanel(panel),
		Records:      records,
		Metadata:     meta,
		AlignedFASTA: ">PHGOT000001\nBBB\n>PHGOT000002\nAAA\n",
		Newick:       "(PHGOT000001,PHGOT000002);",
		Fingerprints: phylo.Fingerprints{Alignment: "align", Tree: "tree", Preview: "old-preview"},
	}
	items := []model.CanvasItem{{
		Title:    "canvas 1",
		Selected: []bool{true, true},
		Rows: []model.CanvasRow{
			{
				RowNumber: 7,
				Kind:      model.CanvasKindFasta,
				FASTA:     &model.QuerySequenceSource{Annotation: "beta", LabelName: "beta", Sequence: "BBB", SequenceKind: model.SequenceProtein},
			},
			{
				RowNumber: 3,
				Kind:      model.CanvasKindFasta,
				FASTA:     &model.QuerySequenceSource{Annotation: "alpha", LabelName: "alpha", Sequence: "AAA", SequenceKind: model.SequenceProtein},
			},
		},
	}}
	tree, _, _, err := w.snapshotCanvasTreeState(items)
	if err != nil {
		t.Fatalf("snapshotCanvasTreeState returned error: %v", err)
	}
	if len(tree.LastPayload.Metadata.Records) != 2 {
		t.Fatalf("snapshot payload records = %#v, want 2", tree.LastPayload.Metadata.Records)
	}
	if tree.LastPayload.Metadata.Records[0].DisplayName != "alpha" || tree.LastPayload.Metadata.Records[0].DisplayPrefix != "[1,2]" ||
		tree.LastPayload.Metadata.Records[1].DisplayName != "beta" || tree.LastPayload.Metadata.Records[1].DisplayPrefix != "[1,1]" {
		t.Fatalf("snapshot payload order should follow visible canvas sort: %#v", tree.LastPayload.Metadata.Records)
	}
}

func TestCloseCanvasTreeViewerStopsCanvasScopedServer(t *testing.T) {
	w := NewBlastWizard(nil)
	w.instanceID = "canvas-viewer-close-test"

	server, url, err := w.ensureCanvasTreeViewer(context.Background())
	if err != nil {
		t.Fatalf("ensureCanvasTreeViewer returned error: %v", err)
	}
	if server == nil || strings.TrimSpace(url) == "" {
		t.Fatalf("viewer server not initialized: %#v %q", server, url)
	}
	if _, err := http.Get(server.URL() + "/health"); err != nil {
		t.Fatalf("viewer health before close failed: %v", err)
	}

	w.closeCanvasTreeViewer()

	if w.canvasTreeViewer != nil || w.canvasTreeViewerCancel != nil {
		t.Fatalf("viewer fields should be cleared after close")
	}
	if !w.canvasTreeLastPayload.UpdatedAt.IsZero() || strings.TrimSpace(w.canvasTreeLastPayload.Newick) != "" {
		t.Fatalf("viewer payload should be cleared after close: %#v", w.canvasTreeLastPayload)
	}
	if len(w.canvasTreeViewerState) != 0 {
		t.Fatalf("viewer state should be cleared after close: %s", w.canvasTreeViewerState)
	}
	if strings.TrimSpace(w.canvasTreeLastPlan.BaseDir) != "" || strings.TrimSpace(w.canvasTreeLastPlan.SessionID) != "" {
		t.Fatalf("viewer plan should be cleared after close: %#v", w.canvasTreeLastPlan)
	}

	var lastErr error
	for range 20 {
		_, lastErr = http.Get(server.URL() + "/health")
		if lastErr != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("viewer health still responded after close; lastErr=%v", lastErr)
}

func TestCanvasTreePreviewAvailableRequiresTreeAndMSAContent(t *testing.T) {
	w := NewBlastWizard(nil)
	if w.canvasTreePreviewAvailable() {
		t.Fatal("preview should be unavailable before any tree refresh")
	}
	w.canvasTreeLastPayload = phylo.ViewerPayload{Newick: "(PHGOT000001);"}
	if w.canvasTreePreviewAvailable() {
		t.Fatal("preview should require aligned FASTA for the MSA page")
	}
	w.canvasTreeLastPayload = phylo.ViewerPayload{AlignedFASTA: ">PHGOT000001\nMPEPTIDE\n"}
	if w.canvasTreePreviewAvailable() {
		t.Fatal("preview should require Newick for the tree page")
	}
	w.canvasTreeLastPayload = phylo.ViewerPayload{
		Newick:       "(PHGOT000001);",
		AlignedFASTA: ">PHGOT000001\nMPEPTIDE\n",
	}
	if !w.canvasTreePreviewAvailable() {
		t.Fatal("preview should be available only after both tree and MSA content exist")
	}
}

func TestReuseLastCanvasTreePlanDoesNotBiologicallyValidateAlignment(t *testing.T) {
	w := NewBlastWizard(nil)
	now := time.Now()
	settings := phylo.NormalizeTreeSettingsForKind(phylo.DefaultTreeSettings(), phylo.SequenceProtein)
	records := []phylo.InputRecord{
		{TaxonID: "PHGOT000001", DisplayName: "DNA row", Sequence: "ATGCGTATGCGT", SequenceKind: phylo.SequenceNucleotide, RowFingerprint: "dna"},
		{TaxonID: "PHGOT000002", DisplayName: "Protein row", Sequence: "MPEPTIDE", SequenceKind: phylo.SequenceProtein, RowFingerprint: "protein"},
	}
	meta := phylo.Metadata{SchemaVersion: 1, GeneratedAt: now, DisplayNameSource: "label_name", Records: records}
	staleAligned := ">PHGOT000001\nATGCGTATGCGT\n>PHGOT000002\nMPEPTIDE\n"
	last, err := phylo.BuildRunPlan("canvas", "old", t.TempDir(), settings, phylo.SequenceProtein, records, meta, staleAligned, "(PHGOT000001,PHGOT000002);", now)
	if err != nil {
		t.Fatalf("BuildRunPlan last: %v", err)
	}
	w.canvasTreeLastPlan = last
	plan, err := phylo.BuildRunPlan("canvas", "new", t.TempDir(), settings, phylo.SequenceProtein, records, meta, "", "", now)
	if err != nil {
		t.Fatalf("BuildRunPlan candidate: %v", err)
	}
	reused, ok, err := w.reuseLastCanvasTreePlan(plan, records, meta, now)
	if err != nil {
		t.Fatalf("reuseLastCanvasTreePlan returned error: %v", err)
	}
	if !ok {
		t.Fatalf("matching runtime artifacts should be reused without Go-side biological validation")
	}
	if !strings.Contains(reused.Plan.AlignedFASTA, "ATGCGTATGCGT") {
		t.Fatalf("reused alignment was unexpectedly changed: %#v", reused.Plan.AlignedFASTA)
	}
}

func TestReuseLastCanvasTreePlanRejectsComputeInputChanges(t *testing.T) {
	w := NewBlastWizard(nil)
	now := time.Now()
	settings := phylo.NormalizeTreeSettingsForKind(phylo.DefaultTreeSettings(), phylo.SequenceProtein)
	records := []phylo.InputRecord{
		{TaxonID: "PHGOT000001", DisplayName: "PAL1", Sequence: "MPEPTIDE", SequenceKind: phylo.SequenceProtein, RowFingerprint: "row-1"},
		{TaxonID: "PHGOT000002", DisplayName: "PAL2", Sequence: "MSECOND", SequenceKind: phylo.SequenceProtein, RowFingerprint: "row-2"},
	}
	meta := phylo.Metadata{SchemaVersion: 1, GeneratedAt: now, DisplayNameSource: "label_name", Records: records}
	aligned := ">PHGOT000001\nMPEPTIDE\n>PHGOT000002\nMSECOND\n"
	newick := "(PHGOT000001,PHGOT000002);"
	last, err := phylo.BuildRunPlan("canvas", "old", t.TempDir(), settings, phylo.SequenceProtein, records, meta, aligned, newick, now)
	if err != nil {
		t.Fatalf("BuildRunPlan last: %v", err)
	}
	w.canvasTreeLastPlan = last

	renamed := append([]phylo.InputRecord(nil), records...)
	renamed[0].DisplayName = "PAL display"
	renamedMeta := meta
	renamedMeta.Records = renamed
	renamedPlan, err := phylo.BuildRunPlan("canvas", "renamed", t.TempDir(), settings, phylo.SequenceProtein, renamed, renamedMeta, "", "", now)
	if err != nil {
		t.Fatalf("BuildRunPlan renamed: %v", err)
	}
	if _, ok, err := w.reuseLastCanvasTreePlan(renamedPlan, renamed, renamedMeta, now); err != nil {
		t.Fatalf("reuse renamed returned error: %v", err)
	} else if !ok {
		t.Fatalf("display-name-only change should reuse runtime artifacts")
	}

	noCoordsSettings := settings
	noCoordsSettings.ShowCanvasCoordinates = false
	noCoordsSettings.ShowCanvasCoordinatesSet = true
	noCoordsPlan, err := phylo.BuildRunPlan("canvas", "no-coords", t.TempDir(), noCoordsSettings, phylo.SequenceProtein, records, meta, "", "", now)
	if err != nil {
		t.Fatalf("BuildRunPlan no coords: %v", err)
	}
	if reused, ok, err := w.reuseLastCanvasTreePlan(noCoordsPlan, records, meta, now); err != nil {
		t.Fatalf("reuse no-coords returned error: %v", err)
	} else if !ok {
		t.Fatalf("legacy coordinate display toggle should reuse runtime artifacts")
	} else if reused.Plan.Fingerprints.Alignment != last.Fingerprints.Alignment || reused.Plan.Fingerprints.Tree != last.Fingerprints.Tree {
		t.Fatalf("legacy coordinate display toggle should keep compute fingerprints: %#v", reused.Plan.Fingerprints)
	}

	paramSettings := settings
	paramSettings.AlignmentParams = make(map[string]string, len(settings.AlignmentParams))
	for key, value := range settings.AlignmentParams {
		paramSettings.AlignmentParams[key] = value
	}
	paramSettings.AlignmentParams["multiple_gap_opening_penalty"] = "12"
	if got := paramSettings.AlignmentParams["multiple_gap_opening_penalty"]; got != "12" {
		t.Fatalf("test setup alignment param = %q, want 12", got)
	}
	paramPlan, err := phylo.BuildRunPlan("canvas", "param", t.TempDir(), paramSettings, phylo.SequenceProtein, records, meta, "", "", now)
	if err != nil {
		t.Fatalf("BuildRunPlan param: %v", err)
	}
	if got := paramPlan.Settings.AlignmentParams["multiple_gap_opening_penalty"]; got != "12" {
		t.Fatalf("param plan alignment param = %q, want 12", got)
	}
	if reused, ok, err := w.reuseLastCanvasTreePlan(paramPlan, records, meta, now); err != nil {
		t.Fatalf("reuse param returned error: %v", err)
	} else if ok {
		t.Fatalf("alignment parameter change reused artifacts instead of full compute: %#v", reused)
	}

	selectionChanged := records[:1]
	selectionMeta := meta
	selectionMeta.Records = selectionChanged
	selectionPlan, err := phylo.BuildRunPlan("canvas", "selection", t.TempDir(), settings, phylo.SequenceProtein, selectionChanged, selectionMeta, "", "", now)
	if err != nil {
		t.Fatalf("BuildRunPlan selection: %v", err)
	}
	if reused, ok, err := w.reuseLastCanvasTreePlan(selectionPlan, selectionChanged, selectionMeta, now); err != nil {
		t.Fatalf("reuse selection returned error: %v", err)
	} else if ok {
		t.Fatalf("selection change reused artifacts instead of full compute: %#v", reused)
	}
}

func TestSnapshotCanvasItemsRestoresDisplayNameLock(t *testing.T) {
	items := canvasItemsFromSnapshot([]sessionsnapshot.CanvasItemV1{{
		Title: "canvas 1",
		Rows: []sessionsnapshot.CanvasRowV1{{
			Kind:              model.CanvasKindFasta,
			DisplayName:       "Locked PAL",
			DisplayNameLocked: true,
			FASTA: &model.QuerySequenceSource{
				LabelName: "PAL1",
				Sequence:  "MPEPTIDE",
			},
		}},
	}})
	if len(items) != 1 || len(items[0].Rows) != 1 {
		t.Fatalf("snapshot canvas items mismatch: %#v", items)
	}
	if !items[0].Rows[0].DisplayNameLocked {
		t.Fatalf("snapshot restore did not preserve display-name lock: %#v", items[0].Rows[0])
	}
}

func TestCanvasItemsFromSnapshotRecomputesSubtitleFromSelectedState(t *testing.T) {
	items := canvasItemsFromSnapshot([]sessionsnapshot.CanvasItemV1{{
		Title:    "canvas 1",
		Subtitle: "0/2 lines",
		Selected: []bool{true, false},
		Rows: []sessionsnapshot.CanvasRowV1{
			{
				Kind:  model.CanvasKindFasta,
				FASTA: &model.QuerySequenceSource{Sequence: "MPEPTIDE", SequenceKind: model.SequenceProtein},
			},
			{
				Kind:  model.CanvasKindFasta,
				FASTA: &model.QuerySequenceSource{Sequence: "MSECOND", SequenceKind: model.SequenceProtein},
			},
		},
	}})
	if len(items) != 1 {
		t.Fatalf("snapshot item count = %d, want 1", len(items))
	}
	if got := items[0].Subtitle; got != "1/2 lines" {
		t.Fatalf("snapshot subtitle = %q, want recomputed selection summary", got)
	}
}

func TestSnapshotCanvasSequenceCacheCollectsKeywordAndBlastRows(t *testing.T) {
	w := NewBlastWizard(nil)
	w.lastKeywordSpecies = model.SpeciesCandidate{ProteomeID: 370201, GenomeLabel: "TAIR12"}
	w.proteinSequenceCache[w.proteinSequenceCacheKey(370201, "AT2G37040.1")] = model.ProteinSequenceData{
		Sequence:       "MKEYWORD",
		OriginalHeader: ">AT2G37040.1",
	}
	w.proteinSequenceCache[w.proteinSequenceCacheKey(456, "BLASTSEQ1")] = model.ProteinSequenceData{
		Sequence:       "MBLAST",
		OriginalHeader: ">BLASTSEQ1",
	}

	cache, err := w.snapshotCanvasSequenceCache(context.Background(), []model.CanvasItem{{
		Title: "canvas 1",
		Rows: []model.CanvasRow{
			{
				Kind: model.CanvasKindKeyword,
				KeywordRow: &model.KeywordResultRow{
					LabelName:  "PAL1",
					ProteinID:  "AT2G37040.1",
					SequenceID: "AT2G37040.1",
				},
			},
			{
				Kind: model.CanvasKindBlast,
				BlastRow: &model.BlastResultRow{
					TargetID:   456,
					Protein:    "BLASTSEQ1",
					SequenceID: "BLASTSEQ1",
				},
			},
			{
				Kind: model.CanvasKindKeyword,
				KeywordRow: &model.KeywordResultRow{
					LabelName:  "INLINE1",
					ProteinID:  "INLINE1",
					SequenceID: "INLINE1",
					ExtraColumns: map[string]string{
						"protein_sequence": "MINLINE",
					},
				},
			},
		},
	}})
	if err != nil {
		t.Fatalf("snapshotCanvasSequenceCache returned error: %v", err)
	}
	if cache == nil {
		t.Fatal("expected canvas sequence cache")
	}
	gotIDs := make([]string, 0, len(cache.Entries))
	for _, entry := range cache.Entries {
		gotIDs = append(gotIDs, entry.SequenceID)
	}
	for _, want := range []string{"AT2G37040.1", "BLASTSEQ1", "INLINE1"} {
		if !slices.Contains(gotIDs, want) {
			t.Fatalf("canvas sequence cache missing %q: %#v", want, cache.Entries)
		}
	}
}

func TestCanvasTreeTableValuesUseGeneLocusAndPHgoLabelForNCBIKeywordRows(t *testing.T) {
	row := model.CanvasRow{
		Kind: model.CanvasKindKeyword,
		KeywordRow: &model.KeywordResultRow{
			SourceDatabase: "ncbi",
			Genome:         "Oryza sativa Japonica Group",
			GeneLocus:      "Os08g14760",
			GeneIdentifier: "GeneID:4335555",
			LabelName:      "Os4CL1",
			SequenceID:     "XP_015650724.1",
		},
	}
	values := canvasTreeTableValues(row, "1")
	if values["geneid"] != "Os08g14760" {
		t.Fatalf("tree table geneid = %q, want Gene locus", values["geneid"])
	}
	if values["blast_labelname"] != "" || values["blast_geneid"] != "" {
		t.Fatalf("keyword rows should not synthesize BLAST source fields: %#v", values)
	}
	if values[phylo.PHgoDisplayNameSource] != "Os-Os08g14760 (Os4CL1)" {
		t.Fatalf("PHgo table value = %q", values[phylo.PHgoDisplayNameSource])
	}
	records, meta, err := phylo.BuildInput([]phylo.RowSource{{
		ItemTitle:    "1",
		RowIndex:     0,
		CanvasRow:    row,
		Sequence:     "MPEPTIDE",
		SequenceKind: phylo.SequenceProtein,
		OriginalHead: "XP_015650724.1",
		TableValues:  values,
	}}, phylo.PHgoDisplayNameSource, "session", time.Now())
	if err != nil {
		t.Fatalf("BuildInput returned error: %v", err)
	}
	if meta.DisplayNameSource != phylo.PHgoDisplayNameSource || records[0].DisplayName != "Os-Os08g14760 (Os4CL1)" {
		t.Fatalf("PHgo display source did not carry into tree input: records=%#v meta=%#v", records, meta)
	}
}

func TestWriteCanvasSessionSnapshotIncludesSequenceCache(t *testing.T) {
	w := NewBlastWizard(nil)
	w.suppressTaskModals = true
	w.lastKeywordSpecies = model.SpeciesCandidate{ProteomeID: 370201, GenomeLabel: "TAIR12"}
	outputDir := mustOutputDir()
	path := filepath.Join(outputDir, "canvas-sequence-cache-test"+sessionsnapshot.FileExtension)
	_ = os.Remove(path)
	t.Cleanup(func() { _ = os.Remove(path) })

	w.proteinSequenceCache[w.proteinSequenceCacheKey(370201, "AT2G37040.1")] = model.ProteinSequenceData{
		Sequence:       "MKEYWORD",
		OriginalHeader: ">AT2G37040.1",
	}
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title: "canvas 1",
			Rows: []model.CanvasRow{{
				Kind: model.CanvasKindKeyword,
				KeywordRow: &model.KeywordResultRow{
					LabelName:  "PAL1",
					ProteinID:  "AT2G37040.1",
					SequenceID: "AT2G37040.1",
				},
			}},
			Selected: []bool{true},
		}},
	}
	err := w.writeCanvasSessionSnapshot(state, exportSettings{
		BaseName:     "canvas-sequence-cache-test",
		OutputDir:    outputDir,
		WriteSession: true,
	})
	if err != nil {
		t.Fatalf("writeCanvasSessionSnapshot returned error: %v", err)
	}

	snapshot, err := sessionsnapshot.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if snapshot.SequenceCache == nil || len(snapshot.SequenceCache.Entries) != 1 {
		t.Fatalf("expected canvas snapshot sequence cache, got %#v", snapshot.SequenceCache)
	}
	if snapshot.SequenceCache.Entries[0].Sequence != "MKEYWORD" {
		t.Fatalf("canvas snapshot sequence cache mismatch: %#v", snapshot.SequenceCache.Entries[0])
	}
}

func TestCanvasRowHasSequenceForExportUsesAllFastaSequenceFields(t *testing.T) {
	if !canvasRowHasSequenceForExport(model.CanvasRow{Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{ProteinSequence: "MPEPTIDE"}}) {
		t.Fatal("FASTA rows with ProteinSequence should be export-ready")
	}
	if !canvasRowHasSequenceForExport(model.CanvasRow{Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{NucleotideSequence: "ATGC"}}) {
		t.Fatal("FASTA rows with NucleotideSequence should be export-ready")
	}
}

func TestRestoreCanvasTreeSnapshotRestoresPayloadAndPlan(t *testing.T) {
	w := NewBlastWizard(nil)
	now := time.Now()
	dir := t.TempDir()
	snapshot := treeSnapshotForTest(dir, now)
	writeTreeSnapshotRestoreArtifacts(t, dir, snapshot)
	w.restoreCanvasTreeSnapshot(snapshot, false)
	if w.canvasTreeLastPayload.Newick != "(PHGOT000001);" {
		t.Fatalf("payload not restored: %#v", w.canvasTreeLastPayload)
	}
	if w.canvasTreeLastPlan.BaseDir == "" || w.canvasTreeLastPlan.RunID != "run1" {
		t.Fatalf("plan not restored: %#v", w.canvasTreeLastPlan)
	}
	if !strings.Contains(string(w.canvasTreeViewerState), `"fontFamily":"Georgia"`) {
		t.Fatalf("viewer state not restored: %s", w.canvasTreeViewerState)
	}
	if len(w.canvasTreeLastPlan.Records) != 1 || w.canvasTreeLastPlan.Records[0].DisplayName != "PAL display" {
		t.Fatalf("plan records/metadata not restored: %#v", w.canvasTreeLastPlan.Records)
	}
	if !strings.Contains(w.canvasTreeLastPlan.RuntimeRequest, `"session_id"`) || !strings.Contains(w.canvasTreeLastPlan.RuntimeResponse, `"mega-phgo-runtime"`) {
		t.Fatalf("runtime audit files not restored into plan: request=%q response=%q", w.canvasTreeLastPlan.RuntimeRequest, w.canvasTreeLastPlan.RuntimeResponse)
	}
	if w.canvasTreeLastPlan.Settings.AlignmentMethod != phylo.AlignmentMUSCLE || w.canvasTreeLastPlan.Fingerprints.Tree != "manifest-tree" {
		t.Fatalf("manifest settings/fingerprints not restored: %#v", w.canvasTreeLastPlan)
	}
	if !w.canvasTreeForceCompute {
		t.Fatalf("restored snapshot tree state must force the next refresh to recompute")
	}
	panel := w.prompt.SnapshotCanvasTreePanelState(canvasStateKey("canvas"))
	if panel.AlignmentMethod != string(phylo.AlignmentMUSCLE) || !panel.EnabledEver {
		t.Fatalf("panel not restored: %#v", panel)
	}
}

func TestRestoreCanvasTreeSnapshotBlocksFirstArtifactReuse(t *testing.T) {
	w := NewBlastWizard(nil)
	now := time.Now()
	dir := t.TempDir()
	settings := phylo.DefaultTreeSettings()
	settings.AlignmentMethod = phylo.AlignmentMUSCLE
	records := []phylo.InputRecord{{
		TaxonID:        "PHGOT000001",
		DisplayName:    "PAL display",
		Sequence:       "MPEPTIDE",
		SequenceKind:   phylo.SequenceProtein,
		RowFingerprint: "row-fingerprint",
	}}
	meta := phylo.Metadata{SchemaVersion: 1, GeneratedAt: now, DisplayNameSource: "label_name", Records: records}
	aligned := ">PHGOT000001\nMPEPTIDE\n"
	newick := "(PHGOT000001);"
	last, err := phylo.BuildRunPlan("canvas", "run1", dir, settings, phylo.SequenceProtein, records, meta, aligned, newick, now)
	if err != nil {
		t.Fatalf("BuildRunPlan last: %v", err)
	}
	snapshot := sessionsnapshot.CanvasTreeV2{
		PanelState:       treePanelForSnapshotTest(),
		LastPayload:      phylo.BuildPayload("canvas", records, meta, aligned, newick, now),
		LastManifest:     last.ToArtifactSet().Manifest,
		LastArtifactDir:  dir,
		LastRunID:        "run1",
		LastAlignedFASTA: aligned,
		LastNewick:       newick,
		Fingerprints:     last.Fingerprints,
	}
	writeTreeSnapshotRestoreArtifacts(t, dir, snapshot)
	w.restoreCanvasTreeSnapshot(snapshot, false)
	if !w.canvasTreeForceCompute {
		t.Fatalf("snapshot restore did not mark the next refresh as full-compute")
	}
	candidate, err := phylo.BuildRunPlan("canvas", "run2", t.TempDir(), settings, phylo.SequenceProtein, records, meta, "", "", now)
	if err != nil {
		t.Fatalf("BuildRunPlan candidate: %v", err)
	}
	if reused, ok, err := w.reuseLastCanvasTreePlan(candidate, records, meta, now); err != nil {
		t.Fatalf("reuse with force flag returned error: %v", err)
	} else if ok {
		t.Fatalf("first refresh after snapshot restore reused artifacts instead of recomputing: %#v", reused)
	}
	w.canvasTreeForceCompute = false
	if _, ok, err := w.reuseLastCanvasTreePlan(candidate, records, meta, now); err != nil {
		t.Fatalf("reuse after clearing force flag returned error: %v", err)
	} else if !ok {
		t.Fatalf("matching non-snapshot artifacts should remain reusable after the forced refresh has completed")
	}
}

func TestRestoreCanvasTreeSnapshotDoesNotBiologicallyValidatePayload(t *testing.T) {
	w := NewBlastWizard(nil)
	now := time.Now()
	dir := t.TempDir()
	snapshot := treeSnapshotForTest(dir, now)
	snapshot.LastPayload.AlignedFASTA = ">PHGOT000001\nATGCGTATGCGT\n"
	snapshot.LastAlignedFASTA = snapshot.LastPayload.AlignedFASTA
	writeTreeSnapshotRestoreArtifacts(t, dir, snapshot)
	w.restoreCanvasTreeSnapshot(snapshot, false)
	if strings.TrimSpace(w.canvasTreeLastPayload.Newick) != "(PHGOT000001);" {
		t.Fatalf("snapshot payload should be restored without Go-side biological validation: %#v", w.canvasTreeLastPayload)
	}
	if strings.TrimSpace(w.canvasTreeLastPlan.AlignedFASTA) != ">PHGOT000001\nATGCGTATGCGT" {
		t.Fatalf("snapshot plan should keep restored runtime alignment unchanged: %#v", w.canvasTreeLastPlan)
	}
}

func TestRestoreCanvasTreeSnapshotCanRecoverPayloadFromArtifact(t *testing.T) {
	w := NewBlastWizard(nil)
	now := time.Now()
	dir := t.TempDir()
	snapshot := treeSnapshotForTest(dir, now)
	writeTreeSnapshotRestoreArtifacts(t, dir, snapshot)
	payloadData, err := json.Marshal(snapshot.LastPayload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "viewer.payload.json"), payloadData, 0o644); err != nil {
		t.Fatalf("write viewer payload: %v", err)
	}
	snapshot.LastPayload = phylo.ViewerPayload{}
	w.restoreCanvasTreeSnapshot(snapshot, false)
	if w.canvasTreeLastPayload.Newick != "(PHGOT000001);" || len(w.canvasTreeLastPlan.Records) != 1 {
		t.Fatalf("payload artifact fallback did not restore tree state: payload=%#v plan=%#v", w.canvasTreeLastPayload, w.canvasTreeLastPlan)
	}
}

func TestRestoreCanvasTreeSnapshotRemapsLegacyOutputTreeDirToCache(t *testing.T) {
	w := NewBlastWizard(nil)
	now := time.Now()
	outputDir, err := appfs.OutputDir()
	if err != nil {
		t.Fatalf("OutputDir returned error: %v", err)
	}
	legacyDir := filepath.Join(outputDir, "tree", "legacy-session", "run1")
	cacheDir, err := canvasTreeArtifactDir("legacy-session", "run1")
	if err != nil {
		t.Fatalf("resolve Canvas tree cache: %v", err)
	}
	snapshot := treeSnapshotForTest(legacyDir, now)
	writeTreeSnapshotRestoreArtifacts(t, cacheDir, snapshot)

	w.restoreCanvasTreeSnapshot(snapshot, false)

	if !samePath(w.canvasTreeLastPlan.BaseDir, cacheDir) {
		t.Fatalf("restored BaseDir = %q, want cache dir %q", w.canvasTreeLastPlan.BaseDir, cacheDir)
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Fatalf("legacy output tree dir should not be required during restore, stat err=%v", err)
	}
}

func TestRestoreLegacyCanvasTreeSnapshotResetsTreeParamsToCurrentDefaults(t *testing.T) {
	w := NewBlastWizard(nil)
	now := time.Now()
	dir := t.TempDir()
	snapshot := treeSnapshotForTest(dir, now)
	snapshot.PanelState.AlignmentParams = map[string]string{"max_iterations": "8"}
	snapshot.PanelState.TreeParams = map[string]string{"distance_model": "p-distance"}
	snapshot.LastManifest.Settings.AlignmentParams = map[string]string{"max_iterations": "8"}
	snapshot.LastManifest.Settings.TreeParams = map[string]string{"distance_model": "p-distance"}
	writeTreeSnapshotRestoreArtifacts(t, dir, snapshot)

	w.restoreCanvasTreeSnapshot(snapshot, true)

	if got := w.canvasTreeLastPlan.Settings.AlignmentParams["multiple_gap_opening_penalty"]; got != "10" {
		t.Fatalf("legacy alignment params should reset to current protein defaults, got multiple_gap_opening_penalty=%q", got)
	}
	if got := w.canvasTreeLastPlan.Settings.TreeParams["model_method"]; got != "Poisson model" {
		t.Fatalf("tree defaults should still be restored from current definition, got model_method=%q", got)
	}
	panel := w.prompt.SnapshotCanvasTreePanelState(canvasStateKey("canvas"))
	if got := panel.AlignmentParams["multiple_gap_opening_penalty"]; got != "10" {
		t.Fatalf("legacy panel alignment params should reset to current protein defaults, got multiple_gap_opening_penalty=%q", got)
	}
}

func TestTreeSettingsFromSnapshotPanelNormalizesEmptyDraftState(t *testing.T) {
	settings := treeSettingsFromSnapshotPanel(tui.CanvasTreePanelState{})
	if settings.DisplayNameSource != phylo.DefaultDisplayNameSource {
		t.Fatalf("display source = %q", settings.DisplayNameSource)
	}
	if settings.AlignmentMethod != phylo.DefaultAlignmentMethod {
		t.Fatalf("alignment method = %q", settings.AlignmentMethod)
	}
	if settings.ConversionTarget != phylo.DefaultConversionTarget || !settings.ConversionSkipUnselect {
		t.Fatalf("mode/recovery defaults = target %q unselect %v", settings.ConversionTarget, settings.ConversionSkipUnselect)
	}
	if settings.TreeMethod != phylo.DefaultTreeMethod {
		t.Fatalf("tree method = %q", settings.TreeMethod)
	}
	if len(settings.AlignmentParams) == 0 || len(settings.TreeParams) == 0 {
		t.Fatalf("expected default params to be populated: %#v", settings)
	}
}

func TestEnsureCanvasTreeRuntimeInteractiveUsesInjectedChecker(t *testing.T) {
	w := NewBlastWizard(nil)
	want := errors.New("runtime missing")
	called := false
	w.ensureCanvasTreeRuntime = func(context.Context) error {
		called = true
		return want
	}
	err := w.ensureCanvasTreeRuntimeInteractive(context.Background())
	if !called {
		t.Fatal("runtime checker was not called")
	}
	if !errors.Is(err, want) {
		t.Fatalf("ensureCanvasTreeRuntimeInteractive error = %v, want %v", err, want)
	}
}

func TestOpenCanvasTreeToolsWithProgressChecksRuntime(t *testing.T) {
	w := NewBlastWizard(nil)
	w.suppressTaskModals = true
	want := errors.New("runtime missing")
	calls := 0
	w.ensureCanvasTreeRuntime = func(context.Context) error {
		calls++
		return want
	}
	err := w.openCanvasTreeToolsWithProgress(context.Background())
	if calls != 1 {
		t.Fatalf("runtime checker calls = %d, want 1", calls)
	}
	if !errors.Is(err, want) {
		t.Fatalf("openCanvasTreeToolsWithProgress error = %v, want %v", err, want)
	}
}

func TestOpenBrowserURLStartsDetachedFromLaterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	called := false
	err := openBrowserURLWithStarter(ctx, "http://127.0.0.1:12345/sessions/canvas", func(name string, args ...string) error {
		called = true
		cancel()
		if strings.TrimSpace(name) == "" {
			t.Fatalf("browser command name is empty")
		}
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "http://127.0.0.1:12345/sessions/canvas") {
			t.Fatalf("browser command args do not contain viewer URL: %q", joined)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("openBrowserURLWithStarter returned error after starter cancellation: %v", err)
	}
	if !called {
		t.Fatal("browser starter was not called")
	}
}

func TestStartJalviewCommandRejectsEmptyAlignmentPath(t *testing.T) {
	if err := startJalviewCommand("  "); err == nil || !strings.Contains(strings.ToLower(err.Error()), "empty") {
		t.Fatalf("startJalviewCommand error = %v, want empty path error", err)
	}
}

func TestOpenCanvasTreeViewerOpensTreeAndMSAPages(t *testing.T) {
	w := NewBlastWizard(nil)
	oldOpen := openBrowserURLFunc
	defer func() { openBrowserURLFunc = oldOpen }()

	var opened []string
	openBrowserURLFunc = func(ctx context.Context, rawURL string) error {
		opened = append(opened, rawURL)
		return nil
	}

	if err := w.openCanvasTreeViewer(context.Background(), nil, phylo.DefaultTreeSettings()); err != nil {
		t.Fatalf("openCanvasTreeViewer returned error: %v", err)
	}
	if len(opened) != 2 {
		t.Fatalf("opened URLs = %#v, want 2", opened)
	}
	if !strings.Contains(opened[0], "/sessions/canvas/tree") {
		t.Fatalf("first opened URL = %q, want tree session page", opened[0])
	}
	if !strings.Contains(opened[1], "/sessions/canvas/msa") {
		t.Fatalf("second opened URL = %q, want msa page", opened[1])
	}
}

func TestOpenCanvasTreeViewerInstallsLiveMSAApplyHandler(t *testing.T) {
	w := NewBlastWizard(nil)
	w.suppressTaskModals = true
	oldOpen := openBrowserURLFunc
	defer func() { openBrowserURLFunc = oldOpen }()
	openBrowserURLFunc = func(ctx context.Context, rawURL string) error { return nil }

	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "msa rows",
			Selected: []bool{true, true},
			Rows: []model.CanvasRow{
				{Kind: model.CanvasKindFasta},
				{Kind: model.CanvasKindFasta},
			},
		}},
	}
	w.canvasTreeLastPlan = phylo.RunPlan{
		Records: []phylo.InputRecord{
			{TaxonID: "PHGOT000001", CanvasItem: "msa rows", CanvasRow: 0},
			{TaxonID: "PHGOT000002", CanvasItem: "msa rows", CanvasRow: 1},
		},
	}
	refreshCalled := false
	w.canvasTreeMSAApplyRun = func(ctx context.Context, runState canvasLaunchState, settings phylo.TreeSettings) error {
		refreshCalled = true
		selected := selectedCanvasRowsInOrder(runState.Items)
		if len(selected) != 1 || selected[0].RowIndex != 0 {
			t.Fatalf("Apply refresh selected rows = %#v, want only green row", selected)
		}
		return nil
	}

	if err := w.openCanvasTreeViewer(context.Background(), &state, phylo.DefaultTreeSettings()); err != nil {
		t.Fatalf("openCanvasTreeViewer returned error: %v", err)
	}
	if w.canvasTreeViewer == nil {
		t.Fatal("viewer server was not created")
	}
	resp, err := http.Post(w.canvasTreeViewer.URL()+"/sessions/canvas/msa/apply", "application/json", strings.NewReader(`{"rows":[{"taxon_id":"PHGOT000001","state":"green"},{"taxon_id":"PHGOT000002","state":"red"}]}`))
	if err != nil {
		t.Fatalf("MSA apply post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("MSA apply status = %d body = %s", resp.StatusCode, body)
	}
	if !refreshCalled {
		t.Fatal("MSA apply did not trigger a tree refresh")
	}
	if got, want := state.Items[0].Selected, []bool{true, false}; !slices.Equal(got, want) {
		t.Fatalf("canvas selection = %#v, want %#v", got, want)
	}
}

func TestMSAApplyDoesNotUseInteractiveTreeRefreshRunner(t *testing.T) {
	w := NewBlastWizard(nil)
	server := phylo.NewViewerServer("127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("viewer server start: %v", err)
	}
	w.canvasTreeViewer = server
	w.canvasTreeLastPlan = phylo.RunPlan{
		Records: []phylo.InputRecord{
			{TaxonID: "PHGOT000001", CanvasItem: "msa rows", CanvasRow: 0},
			{TaxonID: "PHGOT000002", CanvasItem: "msa rows", CanvasRow: 1},
		},
	}
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "msa rows",
			Selected: []bool{true, true},
			Rows: []model.CanvasRow{
				{Kind: model.CanvasKindFasta, DisplayName: "green"},
				{Kind: model.CanvasKindFasta, DisplayName: "red"},
			},
		}},
	}
	w.canvasTreeRefreshRun = func(ctx context.Context, runState canvasLaunchState, settings phylo.TreeSettings) error {
		t.Fatal("MSA Apply must not start the interactive Canvas tree refresh runner")
		return nil
	}
	msaApplyCalled := false
	w.canvasTreeMSAApplyRun = func(ctx context.Context, runState canvasLaunchState, settings phylo.TreeSettings) error {
		msaApplyCalled = true
		selected := selectedCanvasRowsInOrder(runState.Items)
		if len(selected) != 1 || selected[0].Row.DisplayName != "green" {
			t.Fatalf("MSA Apply refresh rows = %#v, want green row only", selected)
		}
		w.canvasTreeLastPayload = phylo.ViewerPayload{
			SchemaVersion: 1,
			SessionID:     "canvas",
			UpdatedAt:     time.Now(),
			AlignedFASTA:  ">PHGOT000001\nAAA\n",
			Metadata: phylo.Metadata{Records: []phylo.InputRecord{
				{TaxonID: "PHGOT000001", DisplayName: "green", CanvasItem: "msa rows", CanvasRow: 0},
			}},
		}
		return nil
	}
	w.installCanvasTreeMSAApplyHandlerOnServer(server, &state, nil)

	resp, err := http.Post(server.URL()+"/sessions/canvas/msa/apply", "application/json", strings.NewReader(`{"rows":[{"taxon_id":"PHGOT000001","state":"green"},{"taxon_id":"PHGOT000002","state":"red"}]}`))
	if err != nil {
		t.Fatalf("MSA apply post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("MSA apply status = %d body = %s", resp.StatusCode, body)
	}
	if !msaApplyCalled {
		t.Fatal("MSA Apply background refresh runner was not called")
	}
}

func TestOpenBrowserURLRespectsCancellationBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := openBrowserURLWithStarter(ctx, "http://127.0.0.1:12345/sessions/canvas", func(name string, args ...string) error {
		called = true
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("openBrowserURLWithStarter error = %v, want context.Canceled", err)
	}
	if called {
		t.Fatal("browser starter should not run after pre-start cancellation")
	}
}

func TestOpenBrowserURLRejectsEmptyURL(t *testing.T) {
	err := openBrowserURLWithStarter(context.Background(), "  ", func(name string, args ...string) error {
		t.Fatal("browser starter should not run for an empty URL")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("openBrowserURLWithStarter error = %v, want empty URL error", err)
	}
}

func TestOpenBrowserURLRealLauncherProbe(t *testing.T) {
	if strings.TrimSpace(os.Getenv("PHYTOZOME_OPEN_BROWSER_PROBE")) == "" {
		t.Skip("set PHYTOZOME_OPEN_BROWSER_PROBE=1 to open the real system browser for a local viewer probe")
	}
	visited := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case visited <- struct{}{}:
		default:
		}
		_, _ = w.Write([]byte("<html><title>PHgo browser probe</title><body>ok</body></html>"))
	}))
	defer server.Close()
	if err := openBrowserURL(context.Background(), server.URL); err != nil {
		t.Fatalf("openBrowserURL returned error: %v", err)
	}
	select {
	case <-visited:
	case <-time.After(10 * time.Second):
		t.Fatalf("system browser did not request the local probe URL %s", server.URL)
	}
}

func TestRefreshCanvasTreeReturnsClearErrorWithoutSequenceSource(t *testing.T) {
	w := NewBlastWizard(nil)
	w.canvasTreeRefreshRun = func(ctx context.Context, runState canvasLaunchState, settings phylo.TreeSettings) error {
		selected := selectedCanvasRowsInOrder(runState.Items)
		if len(selected) != 2 {
			t.Fatalf("selected rows = %d, want 2", len(selected))
		}
		return fmt.Errorf("mega-phgo-runtime: protein sequence source is unavailable")
	}
	state := canvasLaunchState{
		Items: []model.CanvasItem{
			{
				Title: "canvas 1",
				Rows: []model.CanvasRow{{
					Kind: model.CanvasKindKeyword,
					KeywordRow: &model.KeywordResultRow{
						LabelName:  "PAL1",
						ProteinID:  "AT2G37040.1",
						SequenceID: "AT2G37040.1",
					},
				}},
				Selected: []bool{true},
			},
			{
				Title: "canvas 2",
				Rows: []model.CanvasRow{{
					Kind: model.CanvasKindFasta,
					FASTA: &model.QuerySequenceSource{
						Annotation: "query1",
						Sequence:   "MPEPTIDE",
					},
				}},
				Selected: []bool{true},
			},
		},
	}
	err := w.refreshCanvasTreeInteractive(context.Background(), &state, phylo.DefaultTreeSettings())
	if err == nil {
		t.Fatal("expected refreshCanvasTree to fail when source is unavailable")
	}
	if strings.Contains(strings.ToLower(err.Error()), "nil pointer") {
		t.Fatalf("refreshCanvasTree should not panic-wrap nil pointer errors: %v", err)
	}
	if !strings.Contains(err.Error(), "protein sequence source is unavailable") {
		t.Fatalf("unexpected refresh error: %v", err)
	}
}

func TestHydrateCanvasRowSequenceDataUsesSnapshotCacheForLegacyRows(t *testing.T) {
	w := NewBlastWizard(nil)
	w.lastKeywordSpecies = model.SpeciesCandidate{ProteomeID: 370201, GenomeLabel: "TAIR12"}
	w.hydrateSnapshotSequenceCache(&sessionsnapshot.SequenceCacheV1{
		Entries: []sessionsnapshot.SequenceCacheEntryV1{
			{TargetID: 370201, SequenceID: "AT2G37040.1", Sequence: "MKEYWORD", OriginalHeader: ">AT2G37040.1"},
			{TargetID: 456, SequenceID: "BLASTSEQ1", Sequence: "MBLAST", OriginalHeader: ">BLASTSEQ1"},
		},
	})
	items := w.hydrateCanvasRowSequenceData([]model.CanvasItem{{
		Title: "canvas 1",
		Rows: []model.CanvasRow{
			{
				Kind: model.CanvasKindKeyword,
				KeywordRow: &model.KeywordResultRow{
					LabelName:  "PAL1",
					SequenceID: "AT2G37040.1",
				},
			},
			{
				Kind: model.CanvasKindBlast,
				BlastRow: &model.BlastResultRow{
					TargetID:   456,
					Protein:    "BLASTSEQ1",
					SequenceID: "BLASTSEQ1",
				},
			},
		},
	}})
	if got := items[0].Rows[0].SequenceData; got == nil || got.Sequence != "MKEYWORD" {
		t.Fatalf("keyword row did not hydrate sequence cache: %#v", got)
	}
	if got := items[0].Rows[1].SequenceData; got == nil || got.Sequence != "MBLAST" {
		t.Fatalf("blast row did not hydrate sequence cache: %#v", got)
	}
	if items[0].Rows[0].SequenceReady == nil || !*items[0].Rows[0].SequenceReady {
		t.Fatalf("keyword row should be marked sequence-ready: %#v", items[0].Rows[0].SequenceReady)
	}
	if items[0].Rows[1].SequenceReady == nil || !*items[0].Rows[1].SequenceReady {
		t.Fatalf("blast row should be marked sequence-ready: %#v", items[0].Rows[1].SequenceReady)
	}
}

func TestCanvasTreeRowSourcesUsesStoredSequenceDataForMultiCanvasSelection(t *testing.T) {
	w := NewBlastWizard(nil)
	state := canvasLaunchState{
		Items: []model.CanvasItem{
			{
				Title: "canvas 1",
				Rows: []model.CanvasRow{
					{
						Kind: model.CanvasKindKeyword,
						KeywordRow: &model.KeywordResultRow{
							LabelName:  "PAL1",
							SequenceID: "AT2G37040.1",
						},
						SequenceData: &model.ProteinSequenceData{Sequence: "MKEYWORD", OriginalHeader: ">AT2G37040.1"},
					},
					{
						Kind: model.CanvasKindKeyword,
						KeywordRow: &model.KeywordResultRow{
							LabelName:  "SKIP",
							SequenceID: "ATSKIP.1",
						},
						SequenceData: &model.ProteinSequenceData{Sequence: "MSKIP", OriginalHeader: ">ATSKIP.1"},
					},
				},
				Selected: []bool{true, false},
			},
			{
				Title: "canvas 2",
				Rows: []model.CanvasRow{
					{
						Kind: model.CanvasKindFasta,
						FASTA: &model.QuerySequenceSource{
							Annotation: "plain fasta",
							Sequence:   "MFASTA",
						},
					},
					{
						Kind: model.CanvasKindBlast,
						BlastRow: &model.BlastResultRow{
							TargetID:   456,
							Protein:    "BLASTSEQ1",
							SequenceID: "BLASTSEQ1",
							LabelName:  "C4H",
						},
						SequenceData: &model.ProteinSequenceData{Sequence: "MBLAST", OriginalHeader: ">BLASTSEQ1"},
					},
				},
				Selected: []bool{false, true},
			},
		},
	}
	selected := selectedCanvasRowsInOrder(state.Items)
	if len(selected) != 2 {
		t.Fatalf("selected rows = %d, want 2", len(selected))
	}
	sources, err := w.canvasTreeRowSources(context.Background(), state, selected)
	if err != nil {
		t.Fatalf("canvasTreeRowSources returned error: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("row source count = %d, want 2", len(sources))
	}
	if sources[0].Sequence != "MKEYWORD" || sources[1].Sequence != "MBLAST" {
		t.Fatalf("row sources should use stored sequences in selected order: %#v", sources)
	}
	if sources[0].ItemTitle != "canvas 1" || sources[1].ItemTitle != "canvas 2" {
		t.Fatalf("row source canvas order mismatch: %#v", sources)
	}
}

func TestCanvasTreeRowSourcesDoesNotInferMixedKindFasta(t *testing.T) {
	w := NewBlastWizard(nil)
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "mixed",
			Selected: []bool{true},
			Rows: []model.CanvasRow{{
				Kind: model.CanvasKindFasta,
				FASTA: &model.QuerySequenceSource{
					Sequence:     "ACGTMPEPTIDE",
					SequenceKind: "",
				},
			}},
		}},
	}
	selected := selectedCanvasRowsInOrder(state.Items)
	if len(selected) != 1 {
		t.Fatalf("selected rows = %d, want 1", len(selected))
	}
	sources, err := w.canvasTreeRowSources(context.Background(), state, selected)
	if err != nil {
		t.Fatalf("canvasTreeRowSources returned error: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("row source count = %d, want 1", len(sources))
	}
	if sources[0].SequenceKind != phylo.SequenceUnknown {
		t.Fatalf("sequence kind = %s, want unknown", sources[0].SequenceKind)
	}
}

func TestCanvasTreeRowSourcesChoosesFastaSequenceForConversionTarget(t *testing.T) {
	w := NewBlastWizard(nil)
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "dual-sequence",
			Selected: []bool{true},
			Rows: []model.CanvasRow{{
				Kind: model.CanvasKindFasta,
				FASTA: &model.QuerySequenceSource{
					Sequence:           "MGENERIC",
					ProteinSequence:    "MPROTEIN",
					NucleotideSequence: "ATGCCCGGG",
					SequenceKind:       model.SequenceProtein,
				},
			}},
		}},
	}
	selected := selectedCanvasRowsInOrder(state.Items)

	proteinSettings := phylo.DefaultTreeSettings()
	proteinSources, err := w.canvasTreeRowSourcesWithSkippedForSettings(context.Background(), state, selected, proteinSettings)
	if err != nil {
		t.Fatalf("protein canvasTreeRowSourcesWithSkippedForSettings returned error: %v", err)
	}
	if len(proteinSources) != 1 || proteinSources[0].Sequence != "MPROTEIN" || proteinSources[0].SequenceKind != phylo.SequenceProtein {
		t.Fatalf("protein target row source = %#v, want protein sequence", proteinSources)
	}

	dnaSettings := phylo.DefaultTreeSettings()
	dnaSettings.ConversionTarget = phylo.ConversionTargetDNA
	dnaSources, err := w.canvasTreeRowSourcesWithSkippedForSettings(context.Background(), state, selected, dnaSettings)
	if err != nil {
		t.Fatalf("DNA canvasTreeRowSourcesWithSkippedForSettings returned error: %v", err)
	}
	if len(dnaSources) != 1 || dnaSources[0].Sequence != "ATGCCCGGG" || dnaSources[0].SequenceKind != phylo.SequenceNucleotide {
		t.Fatalf("DNA target row source = %#v, want nucleotide sequence", dnaSources)
	}
}

func TestCanvasTreeRowSourcesResolvesNucleotideSequenceForDNAConversion(t *testing.T) {
	w := NewBlastWizard(nil)
	w.source = fakeSource{
		name: "lemna",
		nucleotideSeqs: map[string]string{
			"blastn|SEQ1": "ATGCCCGGG",
		},
	}
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "blast-row",
			Selected: []bool{true},
			Rows: []model.CanvasRow{{
				Kind: model.CanvasKindBlast,
				BlastRow: &model.BlastResultRow{
					TargetID:     18,
					SequenceID:   "SEQ1",
					TranscriptID: "SEQ1",
					Protein:      "SEQ1",
				},
				SequenceData: &model.ProteinSequenceData{Sequence: "MPEPTIDE"},
			}},
		}},
	}
	settings := phylo.DefaultTreeSettings()
	settings.ConversionTarget = phylo.ConversionTargetDNA
	selected := selectedCanvasRowsInOrder(state.Items)
	sources, err := w.canvasTreeRowSourcesWithSkippedForSettings(context.Background(), state, selected, settings)
	if err != nil {
		t.Fatalf("canvasTreeRowSourcesWithSkippedForSettings returned error: %v", err)
	}
	if len(sources) != 1 || sources[0].Sequence != "ATGCCCGGG" || sources[0].SequenceKind != phylo.SequenceNucleotide {
		t.Fatalf("DNA resolver row source = %#v, want resolved nucleotide sequence", sources)
	}
}

func TestCanvasTreeRowSourcesResolvesNucleotideUsingRowSourceDatabase(t *testing.T) {
	w := NewBlastWizard(nil)
	w.sourceFactory = func(database string) source.DataSource {
		if database == "lemna" {
			return fakeSource{
				name: "lemna",
				nucleotideSeqs: map[string]string{
					"blastn|SEQ1": "ATGCCCGGG",
				},
			}
		}
		return nil
	}
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "snapshot-blast-row",
			Selected: []bool{true},
			Rows: []model.CanvasRow{{
				Kind: model.CanvasKindBlast,
				BlastRow: &model.BlastResultRow{
					SourceDatabase: "lemna",
					TargetID:       18,
					SequenceID:     "SEQ1",
					TranscriptID:   "SEQ1",
					Protein:        "SEQ1",
				},
				SequenceData: &model.ProteinSequenceData{Sequence: "MPEPTIDE"},
			}},
		}},
	}
	settings := phylo.DefaultTreeSettings()
	settings.ConversionTarget = phylo.ConversionTargetDNA
	selected := selectedCanvasRowsInOrder(state.Items)
	sources, err := w.canvasTreeRowSourcesWithSkippedForSettings(context.Background(), state, selected, settings)
	if err != nil {
		t.Fatalf("canvasTreeRowSourcesWithSkippedForSettings returned error: %v", err)
	}
	if len(sources) != 1 || sources[0].Sequence != "ATGCCCGGG" || sources[0].SequenceKind != phylo.SequenceNucleotide {
		t.Fatalf("row-source resolver source = %#v, want resolved nucleotide sequence", sources)
	}
}

func TestCanvasTreeRowSourcesResolvesFastaProteinToDNAUsingPhytozomeHeader(t *testing.T) {
	w := NewBlastWizard(nil)
	w.sourceFactory = func(database string) source.DataSource {
		if database == "phytozome" {
			return fakeSource{
				name:    "phytozome",
				species: []model.SpeciesCandidate{{ProteomeID: 167, SearchAlias: "Arabidopsis thaliana", GenomeLabel: "Arabidopsis thaliana TAIR10"}},
				nucleotideSeqs: map[string]string{
					"blastn|AT1G51680.1": "ATGCCCGGG",
				},
			}
		}
		return nil
	}
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "snapshot-fasta-row",
			Selected: []bool{true},
			Rows: []model.CanvasRow{{
				Kind: model.CanvasKindFasta,
				FASTA: &model.QuerySequenceSource{
					Sequence:            "MPEPTIDE",
					ProteinSequence:     "MPEPTIDE",
					SequenceKind:        model.SequenceProtein,
					PreferredSequenceID: "AT1G51680.1",
					GeneID:              "AT1G51680.1",
					SourceDatabase:      "fasta",
					OrganismShort:       "Arabidopsis thaliana TAIR10",
					Annotation:          "phgo://Arabidopsis thaliana TAIR10/4CL1/AT1G51680.1\\6",
				},
			}},
		}},
	}
	settings := phylo.DefaultTreeSettings()
	settings.ConversionTarget = phylo.ConversionTargetDNA
	selected := selectedCanvasRowsInOrder(state.Items)
	sources, err := w.canvasTreeRowSourcesWithSkippedForSettings(context.Background(), state, selected, settings)
	if err != nil {
		t.Fatalf("canvasTreeRowSourcesWithSkippedForSettings returned error: %v", err)
	}
	if len(sources) != 1 || sources[0].Sequence != "" || sources[0].SequenceKind != phylo.SequenceNucleotide {
		t.Fatalf("FASTA row source = %#v, want selected empty nucleotide record when no real DNA sequence is embedded or resolved", sources)
	}
}

func TestCanvasItemsFromSnapshotInputRestoresCanvasSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "canvas-source.pgo")
	if err := sessionsnapshot.WriteFile(path, sessionsnapshot.Snapshot{
		Context: sessionsnapshot.ContextV1{CreatedAt: time.Now(), Mode: "canvas"},
		Canvas: &sessionsnapshot.CanvasResultV1{
			Items: []sessionsnapshot.CanvasItemV1{{
				Rows: []sessionsnapshot.CanvasRowV1{{
					Kind: model.CanvasKindBlast,
					BlastRow: &model.BlastResultRow{
						SourceDatabase: "lemna",
						SequenceID:     "SEQ1",
					},
				}},
				Selected: []bool{true},
			}},
		},
	}); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	w := NewBlastWizard(nil)
	w.sourceFactory = func(database string) source.DataSource {
		if database == "lemna" {
			return fakeSource{name: "lemna"}
		}
		return nil
	}
	items, err := w.canvasItemsFromSnapshotInput(path, path)
	if err != nil {
		t.Fatalf("canvasItemsFromSnapshotInput returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if w.source == nil || w.source.Name() != "lemna" {
		t.Fatalf("snapshot source = %#v, want lemna", w.source)
	}
}

func TestInferSnapshotDatabaseUsesCanvasRows(t *testing.T) {
	snapshot := sessionsnapshot.Snapshot{
		Canvas: &sessionsnapshot.CanvasResultV1{
			Items: []sessionsnapshot.CanvasItemV1{{
				Rows: []sessionsnapshot.CanvasRowV1{
					{Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{SourceDatabase: "fasta"}},
					{Kind: model.CanvasKindBlast, BlastRow: &model.BlastResultRow{SourceDatabase: "lemna"}},
				},
			}},
		},
	}
	if got := inferSnapshotDatabase(snapshot); got != "lemna" {
		t.Fatalf("inferSnapshotDatabase = %q, want lemna", got)
	}
}

func TestCanvasTreeRowSourcesPassesAmbiguousFastaToMEGARuntime(t *testing.T) {
	w := NewBlastWizard(nil)
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "mixed",
			Selected: []bool{true},
			Rows: []model.CanvasRow{{
				Kind: model.CanvasKindFasta,
				FASTA: &model.QuerySequenceSource{
					Sequence:     "???",
					SequenceKind: "",
				},
			}},
		}},
	}
	selected := selectedCanvasRowsInOrder(state.Items)
	sources, err := w.canvasTreeRowSourcesWithSkipped(context.Background(), state, selected)
	if err != nil {
		t.Fatalf("canvasTreeRowSourcesWithSkipped returned error: %v", err)
	}
	if len(sources) != 1 || sources[0].Sequence != "???" {
		t.Fatalf("row source = %#v, want ambiguous FASTA passed through", sources)
	}
}

func TestSelectedCanvasRowsInOrderKeepsSelectedRowsWithoutExportReadySequence(t *testing.T) {
	rows := selectedCanvasRowsInOrder([]model.CanvasItem{{
		Title:    "tree-input",
		Selected: []bool{true, true, false},
		Rows: []model.CanvasRow{
			{Kind: model.CanvasKindKeyword, KeywordRow: &model.KeywordResultRow{LabelName: "missing-sequence"}},
			{Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Annotation: "empty-fasta"}},
			{Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Sequence: "MPEPTIDE"}},
		},
	}})
	if len(rows) != 2 {
		t.Fatalf("selected rows = %#v, want both checked rows even when sequence payloads are empty", rows)
	}
}

func TestNormalizeCanvasTreeRowSourcesPassesMixedDNAProteinToMEGARuntime(t *testing.T) {
	sources := []phylo.RowSource{
		{ItemTitle: "protein", RowIndex: 0, Sequence: "MPEPTIDE", SequenceKind: phylo.SequenceProtein},
		{ItemTitle: "dna", RowIndex: 0, Sequence: "ATGCGTATGCGT", SequenceKind: phylo.SequenceNucleotide},
	}
	normalized, skipped, kind, err := normalizeCanvasTreeRowSourcesWithSkipped(sources, nil, phylo.DefaultTreeSettings())
	if err != nil {
		t.Fatalf("normalizeCanvasTreeRowSourcesWithSkipped returned error: %v", err)
	}
	if kind != phylo.SequenceProtein {
		t.Fatalf("normalized kind = %s, want protein", kind)
	}
	if len(normalized) != 2 {
		t.Fatalf("normalized rows = %#v, want both rows for MEGA runtime conversion", normalized)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped rows = %#v, want none", skipped)
	}
}

func TestNormalizeCanvasTreeRowSourcesDoesNotSkipMismatchedRowsWhenConfigured(t *testing.T) {
	sources := []phylo.RowSource{
		{ItemTitle: "protein", RowIndex: 0, Sequence: "MPEPTIDE", SequenceKind: phylo.SequenceProtein},
		{ItemTitle: "dna", RowIndex: 1, Sequence: "ATGCNNNNATGC", SequenceKind: phylo.SequenceNucleotide},
	}
	settings := phylo.DefaultTreeSettings()
	normalized, skipped, kind, err := normalizeCanvasTreeRowSourcesWithSkipped(sources, nil, settings)
	if err != nil {
		t.Fatalf("normalizeCanvasTreeRowSourcesWithSkipped returned error: %v", err)
	}
	if kind != phylo.SequenceProtein {
		t.Fatalf("normalized kind = %s, want protein", kind)
	}
	if len(normalized) != 2 {
		t.Fatalf("normalized rows = %#v, want all rows passed to MEGA runtime", normalized)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped rows = %#v, want none", skipped)
	}
}

func TestNormalizeCanvasTreeRowSourcesPassesEmptyRowsToMEGARuntime(t *testing.T) {
	sources := []phylo.RowSource{
		{ItemTitle: "protein", RowIndex: 0, Sequence: "MPEPTIDE", SequenceKind: phylo.SequenceProtein},
		{ItemTitle: "dna", RowIndex: 1, Sequence: "", SequenceKind: phylo.SequenceNucleotide},
	}
	normalized, skipped, kind, err := normalizeCanvasTreeRowSourcesWithSkipped(sources, nil, phylo.DefaultTreeSettings())
	if err != nil {
		t.Fatalf("normalizeCanvasTreeRowSourcesWithSkipped returned error: %v", err)
	}
	if kind != phylo.SequenceProtein {
		t.Fatalf("normalized kind = %s, want protein", kind)
	}
	if len(normalized) != 2 {
		t.Fatalf("normalized rows = %#v, want all rows passed to MEGA runtime", normalized)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped rows = %#v, want none", skipped)
	}
}

func TestNormalizeCanvasTreeRowSourcesPreservesUnknownRowKind(t *testing.T) {
	sources := []phylo.RowSource{
		{ItemTitle: "ambiguous", RowIndex: 0, Sequence: "ACGTMPEPTIDE", SequenceKind: phylo.SequenceUnknown},
	}
	normalized, _, kind, err := normalizeCanvasTreeRowSourcesWithSkipped(sources, nil, phylo.DefaultTreeSettings())
	if err != nil {
		t.Fatalf("normalizeCanvasTreeRowSourcesWithSkipped returned error: %v", err)
	}
	if kind != phylo.SequenceProtein {
		t.Fatalf("global target kind = %s, want protein", kind)
	}
	if normalized[0].SequenceKind != phylo.SequenceUnknown {
		t.Fatalf("row sequence kind = %s, want unknown metadata preserved", normalized[0].SequenceKind)
	}
}

func TestCanvasTreeDNAModeResolvesKeywordNucleotideBeforeStoredProtein(t *testing.T) {
	w := NewBlastWizard(nil)
	w.source = fakeSource{
		name:           "phytozome",
		nucleotideSeqs: map[string]string{"blastn|tx1": "ATGAAATGA"},
	}
	w.lastKeywordSpecies = model.SpeciesCandidate{ProteomeID: 42}
	selected := canvasSelectedRow{
		ItemTitle: "keyword",
		RowIndex:  0,
		Row: model.CanvasRow{
			Kind: model.CanvasKindKeyword,
			KeywordRow: &model.KeywordResultRow{
				SourceDatabase: "phytozome",
				SequenceID:     "tx1",
			},
			SequenceData: &model.ProteinSequenceData{Sequence: "MPEPTIDE"},
		},
	}
	settings := phylo.DefaultTreeSettings()
	settings.ConversionTarget = phylo.ConversionTargetDNA
	settings.AlignmentMethod = phylo.AlignmentClustalW

	choice, err := w.canvasTreeSequenceForSettings(context.Background(), selected, settings)
	if err != nil {
		t.Fatalf("canvasTreeSequenceForSettings returned error: %v", err)
	}
	if choice.Kind != phylo.SequenceNucleotide || choice.Sequence != "ATGAAATGA" {
		t.Fatalf("DNA mode should resolve real nucleotide sequence, got %#v", choice)
	}
}

func TestCanvasTreeDNAModeDoesNotInventNucleotideWhenResolverMisses(t *testing.T) {
	w := NewBlastWizard(nil)
	w.source = fakeSource{name: "phytozome"}
	w.lastKeywordSpecies = model.SpeciesCandidate{ProteomeID: 42}
	selected := canvasSelectedRow{
		ItemTitle: "keyword",
		RowIndex:  0,
		Row: model.CanvasRow{
			Kind: model.CanvasKindKeyword,
			KeywordRow: &model.KeywordResultRow{
				SourceDatabase: "phytozome",
				SequenceID:     "tx1",
			},
			SequenceData: &model.ProteinSequenceData{Sequence: "MPEPTIDE"},
		},
	}
	settings := phylo.DefaultTreeSettings()
	settings.ConversionTarget = phylo.ConversionTargetDNA

	choice, err := w.canvasTreeSequenceForSettings(context.Background(), selected, settings)
	if err == nil {
		t.Fatalf("canvasTreeSequenceForSettings returned %#v, want resolver error", choice)
	}
	if !strings.Contains(err.Error(), "no nucleotide sequence") {
		t.Fatalf("resolver error = %v, want original nucleotide fetch error", err)
	}
}

func TestCanvasTreeRowSourcesReturnsNucleotideResolverErrors(t *testing.T) {
	w := NewBlastWizard(nil)
	w.source = fakeSource{name: "phytozome"}
	w.lastKeywordSpecies = model.SpeciesCandidate{ProteomeID: 42}
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "keyword",
			Selected: []bool{true},
			Rows: []model.CanvasRow{{
				Kind: model.CanvasKindKeyword,
				KeywordRow: &model.KeywordResultRow{
					SourceDatabase: "phytozome",
					SequenceID:     "tx1",
				},
				SequenceData: &model.ProteinSequenceData{Sequence: "MPEPTIDE"},
			}},
		}},
	}
	settings := phylo.DefaultTreeSettings()
	settings.ConversionTarget = phylo.ConversionTargetDNA

	selected := selectedCanvasRowsInOrder(state.Items)
	sources, err := w.canvasTreeRowSourcesWithSkippedForSettings(context.Background(), state, selected, settings)
	if err == nil {
		t.Fatalf("canvasTreeRowSourcesWithSkippedForSettings returned sources %#v, want resolver error", sources)
	}
	if !strings.Contains(err.Error(), "no nucleotide sequence") {
		t.Fatalf("row-source error = %v, want original nucleotide fetch error", err)
	}
}

func TestCanvasTreeDNAModeReturnsSourceConstructionErrors(t *testing.T) {
	w := NewBlastWizard(nil)
	selected := canvasSelectedRow{
		ItemTitle: "blast",
		RowIndex:  0,
		Row: model.CanvasRow{
			Kind: model.CanvasKindBlast,
			BlastRow: &model.BlastResultRow{
				SourceDatabase: "missingdb",
				TargetID:       42,
				SequenceID:     "tx1",
			},
			SequenceData: &model.ProteinSequenceData{Sequence: "MPEPTIDE"},
		},
	}
	settings := phylo.DefaultTreeSettings()
	settings.ConversionTarget = phylo.ConversionTargetDNA

	choice, err := w.canvasTreeSequenceForSettings(context.Background(), selected, settings)
	if err == nil {
		t.Fatalf("canvasTreeSequenceForSettings returned %#v, want source-construction error", choice)
	}
	if !strings.Contains(err.Error(), "unsupported BLAST target database") {
		t.Fatalf("source-construction error = %v", err)
	}
}

func TestRefreshCanvasTreeInteractiveSkipUnchecksRowsAndRetries(t *testing.T) {
	w := NewBlastWizard(nil)
	state := canvasLaunchState{
		Items: []model.CanvasItem{
			{
				Title:    "protein",
				Selected: []bool{true},
				Rows: []model.CanvasRow{{
					Kind:  model.CanvasKindFasta,
					FASTA: &model.QuerySequenceSource{Sequence: "MPEPTIDE", SequenceKind: model.SequenceProtein},
				}},
			},
			{
				Title:    "dna",
				Selected: []bool{true},
				Rows: []model.CanvasRow{{
					Kind:  model.CanvasKindFasta,
					FASTA: &model.QuerySequenceSource{Sequence: "NN", SequenceKind: model.SequenceDNA},
				}},
			},
		},
	}
	callCount := 0
	recoveryCount := 0
	w.canvasTreeRefreshRun = func(ctx context.Context, runState canvasLaunchState, settings phylo.TreeSettings) error {
		callCount++
		selected := selectedCanvasRowsInOrder(runState.Items)
		if callCount == 1 {
			if len(selected) != 2 {
				t.Fatalf("first refresh selected rows = %d, want 2", len(selected))
			}
			return &canvasTreeSkippedRowsError{SkippedRows: []canvasTreeSkippedRow{{
				ItemTitle: "dna",
				RowIndex:  0,
				Reason:    "skipped by mega-phgo-runtime",
			}}}
		}
		if len(selected) != 1 || selected[0].ItemTitle != "protein" {
			t.Fatalf("retry selected rows = %#v, want only protein row", selected)
		}
		return nil
	}
	w.canvasTreeRecover = func(description string, backTarget error, allowSkip bool) (string, error) {
		recoveryCount++
		if !allowSkip {
			t.Fatalf("allowSkip = false, want true")
		}
		if !strings.Contains(description, "cannot be used for the current tree refresh") {
			t.Fatalf("unexpected recovery description: %q", description)
		}
		if !strings.Contains(description, "dna row 1") {
			t.Fatalf("recovery description missing skipped row detail: %q", description)
		}
		return "skip", nil
	}
	settings := phylo.DefaultTreeSettings()
	err := w.refreshCanvasTreeInteractive(context.Background(), &state, settings)
	if err != nil {
		t.Fatalf("refreshCanvasTreeInteractive returned error: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("refresh call count = %d, want 2", callCount)
	}
	if recoveryCount != 1 {
		t.Fatalf("recovery call count = %d, want 1", recoveryCount)
	}
	if !state.Items[0].Selected[0] {
		t.Fatalf("protein row should remain selected: %#v", state.Items[0].Selected)
	}
	if state.Items[1].Selected[0] {
		t.Fatalf("skipped dna row should be unselected: %#v", state.Items[1].Selected)
	}
	if got := state.Items[1].Subtitle; got != "0/1 lines" {
		t.Fatalf("dna canvas subtitle = %q, want selection summary updated", got)
	}
}

func TestRefreshCanvasTreeInteractiveSkipResolvesRuntimeTaxonIDsAndRetriesRepeatedly(t *testing.T) {
	w := NewBlastWizard(nil)
	state := canvasLaunchState{
		Items: []model.CanvasItem{
			{
				Title:    "duplicate",
				Selected: []bool{true, true},
				Rows: []model.CanvasRow{
					{Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Sequence: "MAAAA", SequenceKind: model.SequenceProtein}},
					{Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Sequence: "MBBBB", SequenceKind: model.SequenceProtein}},
				},
			},
			{
				Title:    "duplicate",
				Selected: []bool{true, true},
				Rows: []model.CanvasRow{
					{Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Sequence: "MCCCC", SequenceKind: model.SequenceProtein}},
					{Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Sequence: "MDDDD", SequenceKind: model.SequenceProtein}},
				},
			},
		},
	}
	callCount := 0
	w.canvasTreeRefreshRun = func(ctx context.Context, runState canvasLaunchState, settings phylo.TreeSettings) error {
		callCount++
		selected := selectedCanvasRowsInOrder(runState.Items)
		switch callCount {
		case 1:
			if len(selected) != 4 {
				t.Fatalf("first refresh selected rows = %#v, want 4", selected)
			}
			return &canvasTreeSkippedRowsError{SkippedRows: []canvasTreeSkippedRow{{
				ItemTitle: "duplicate",
				RowIndex:  0,
				TaxonID:   "PHGOT000003",
				Reason:    "first runtime pair failure",
			}}}
		case 2:
			if len(selected) != 3 {
				t.Fatalf("second refresh selected rows = %#v, want 3", selected)
			}
			for _, row := range selected {
				if row.ItemIndex == 1 && row.RowIndex == 0 {
					t.Fatalf("runtime taxon PHGOT000003 row was not unchecked: %#v", selected)
				}
			}
			return &canvasTreeSkippedRowsError{SkippedRows: []canvasTreeSkippedRow{{
				ItemTitle: "duplicate",
				RowIndex:  1,
				TaxonID:   "PHGOT000002",
				Reason:    "second runtime pair failure",
			}}}
		case 3:
			if len(selected) != 2 {
				t.Fatalf("third refresh selected rows = %#v, want 2", selected)
			}
			for _, row := range selected {
				if (row.ItemIndex == 1 && row.RowIndex == 0) || (row.ItemIndex == 0 && row.RowIndex == 1) {
					t.Fatalf("previous runtime skipped rows should stay unchecked: %#v", selected)
				}
			}
			return nil
		default:
			t.Fatalf("unexpected refresh call %d", callCount)
			return nil
		}
	}
	recoveryCount := 0
	w.canvasTreeRecover = func(description string, backTarget error, allowSkip bool) (string, error) {
		recoveryCount++
		if !strings.Contains(description, "keep automatically unchecking additional rows") {
			t.Fatalf("recovery description should explain automatic skip retry behavior: %q", description)
		}
		return "skip", nil
	}

	if err := w.refreshCanvasTreeInteractive(context.Background(), &state, phylo.DefaultTreeSettings()); err != nil {
		t.Fatalf("refreshCanvasTreeInteractive returned error: %v", err)
	}
	if callCount != 3 || recoveryCount != 1 {
		t.Fatalf("callCount=%d recoveryCount=%d, want 3/1", callCount, recoveryCount)
	}
	if state.Items[1].Selected[0] || state.Items[0].Selected[1] {
		t.Fatalf("runtime skipped rows should be unchecked in persistent state: %#v %#v", state.Items[0].Selected, state.Items[1].Selected)
	}
	if !state.Items[0].Selected[0] || !state.Items[1].Selected[1] {
		t.Fatalf("non-skipped rows should remain selected: %#v %#v", state.Items[0].Selected, state.Items[1].Selected)
	}
}

func TestRefreshCanvasTreeInteractiveSkipUsesFailedRunSelectedRows(t *testing.T) {
	w := NewBlastWizard(nil)
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "runtime-order",
			Selected: []bool{true, true, true},
			Rows: []model.CanvasRow{
				{Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Sequence: "MAAAA", SequenceKind: model.SequenceProtein}},
				{Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Sequence: "MBBBB", SequenceKind: model.SequenceProtein}},
				{Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Sequence: "MCCCC", SequenceKind: model.SequenceProtein}},
			},
		}},
	}
	failedRunRows := []canvasSelectedRow{
		{ItemTitle: "runtime-order", ItemIndex: 0, RowIndex: 1},
		{ItemTitle: "runtime-order", ItemIndex: 0, RowIndex: 0},
		{ItemTitle: "runtime-order", ItemIndex: 0, RowIndex: 2},
	}
	callCount := 0
	w.canvasTreeRefreshRun = func(ctx context.Context, runState canvasLaunchState, settings phylo.TreeSettings) error {
		callCount++
		selected := selectedCanvasRowsInOrder(runState.Items)
		if callCount == 1 {
			return &canvasTreeSkippedRowsError{
				SkippedRows: []canvasTreeSkippedRow{{
					ItemTitle: "runtime-order",
					RowIndex:  0,
					TaxonID:   "PHGOT000001",
					Reason:    "runtime failure in failed-run order",
				}},
				SelectedRows: failedRunRows,
			}
		}
		if len(selected) != 2 {
			t.Fatalf("retry selected rows = %#v, want 2 rows after skip", selected)
		}
		for _, row := range selected {
			if row.RowIndex == 1 {
				t.Fatalf("skip used current UI order instead of failed runtime order: %#v", selected)
			}
		}
		return nil
	}
	w.canvasTreeRecover = func(description string, backTarget error, allowSkip bool) (string, error) {
		return "skip", nil
	}

	if err := w.refreshCanvasTreeInteractive(context.Background(), &state, phylo.DefaultTreeSettings()); err != nil {
		t.Fatalf("refreshCanvasTreeInteractive returned error: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("refresh call count = %d, want 2", callCount)
	}
	if state.Items[0].Selected[1] {
		t.Fatalf("failed-run PHGOT000001 row should be unchecked: %#v", state.Items[0].Selected)
	}
	if !state.Items[0].Selected[0] || !state.Items[0].Selected[2] {
		t.Fatalf("non-skipped rows should remain checked: %#v", state.Items[0].Selected)
	}
}

func TestApplyMSASelectionToCanvasStateUpdatesCanvasSelection(t *testing.T) {
	w := NewBlastWizard(nil)
	w.canvasTreeLastPlan = phylo.RunPlan{
		Records: []phylo.InputRecord{
			{TaxonID: "PHGOT000001", CanvasItem: "msa rows", CanvasRow: 0},
			{TaxonID: "PHGOT000002", CanvasItem: "msa rows", CanvasRow: 1},
			{TaxonID: "PHGOT000003", CanvasItem: "msa rows", CanvasRow: 2},
		},
	}
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "msa rows",
			Selected: []bool{true, true, false},
			Rows: []model.CanvasRow{
				{Kind: model.CanvasKindFasta},
				{Kind: model.CanvasKindFasta},
				{Kind: model.CanvasKindFasta},
			},
		}},
	}

	changed := w.applyMSASelectionToCanvasState(&state, phylo.MSAApplyRequest{Rows: []phylo.MSAApplyRow{
		{TaxonID: "PHGOT000001", State: "yellow"},
		{TaxonID: "PHGOT000002", State: "red"},
		{TaxonID: "PHGOT000003", State: "green"},
	}})
	if !changed {
		t.Fatal("applyMSASelectionToCanvasState reported no change")
	}
	if got, want := state.Items[0].Selected, []bool{false, false, true}; !slices.Equal(got, want) {
		t.Fatalf("canvas selected mask = %#v, want %#v", got, want)
	}
	if got, want := state.Items[0].MSAFlags, []bool{true, false, false}; !slices.Equal(got, want) {
		t.Fatalf("canvas MSA flags = %#v, want %#v", got, want)
	}
	if got := state.Items[0].Subtitle; got != "1/3 lines" {
		t.Fatalf("canvas subtitle = %q, want selection summary updated", got)
	}
}

func TestApplyMSASelectionToCanvasStateEditsDisplayNameAndSequence(t *testing.T) {
	w := NewBlastWizard(nil)
	w.canvasTreeLastPlan = phylo.RunPlan{
		Records: []phylo.InputRecord{{
			TaxonID:      "PHGOT000001",
			DisplayName:  "old name",
			SequenceKind: phylo.SequenceProtein,
			CanvasItem:   "msa rows",
			CanvasRow:    0,
		}},
	}
	state := canvasLaunchState{Items: []model.CanvasItem{{
		Title:    "msa rows",
		Selected: []bool{true},
		Rows: []model.CanvasRow{{
			Kind:        model.CanvasKindFasta,
			DisplayName: "old name",
			FASTA:       &model.QuerySequenceSource{Sequence: "MPEPTIDE", SequenceKind: model.SequenceProtein},
		}},
	}}}

	changed := w.applyMSASelectionToCanvasState(&state, phylo.MSAApplyRequest{Rows: []phylo.MSAApplyRow{{
		TaxonID:     "PHGOT000001",
		DisplayName: "new name with spaces",
		Description: "MSA-only description",
		Sequence:    "m pep tide",
		State:       "green",
	}}})
	if !changed {
		t.Fatal("applyMSASelectionToCanvasState reported no change for MSA edits")
	}
	row := state.Items[0].Rows[0]
	if row.DisplayName != "new name with spaces" || !row.DisplayNameLocked {
		t.Fatalf("MSA name edit should lock Canvas display name, got %#v", row)
	}
	if row.SequenceData == nil || row.SequenceData.Sequence != "M PEP TIDE" {
		t.Fatalf("MSA sequence edit should uppercase and save to Canvas sequence data, got %#v", row.SequenceData)
	}
	if row.FASTA == nil || row.FASTA.Sequence != "M PEP TIDE" || row.FASTA.ProteinSequence != "M PEP TIDE" {
		t.Fatalf("MSA sequence edit should update FASTA row sequence fields, got %#v", row.FASTA)
	}

	applyCanvasDisplayNameSource(&state.Items[0], "gene_id")
	if got := state.Items[0].Rows[0].DisplayName; got != "new name with spaces" {
		t.Fatalf("MSA-locked display name was overwritten by display-name source change: %q", got)
	}
}

func TestCanvasMSAStateUsesSharedSelectedPayloadOnly(t *testing.T) {
	items := []model.CanvasItem{{
		Title:    "msa rows",
		Selected: []bool{true, false, false},
		MSAFlags: []bool{false, true, false},
		Rows: []model.CanvasRow{
			{Kind: model.CanvasKindFasta, DisplayName: "green"},
			{Kind: model.CanvasKindFasta, DisplayName: "yellow"},
			{Kind: model.CanvasKindFasta, DisplayName: "red"},
		},
	}}
	payload := phylo.ViewerPayload{
		SchemaVersion: 1,
		SessionID:     "canvas",
		UpdatedAt:     time.Now(),
		AlignedFASTA:  ">PHGOT000001\nAAA\n",
		Metadata: phylo.Metadata{Records: []phylo.InputRecord{
			{TaxonID: "PHGOT000001", DisplayName: "green", CanvasItem: "msa rows", CanvasRow: 0},
		}},
	}

	state := canvasMSAStateFromItems(items, payload)
	if len(state.Rows) != 1 || state.Rows[0].TaxonID != "PHGOT000001" || state.Rows[0].State != "green" {
		t.Fatalf("MSA state = %#v, want only shared selected green row", state.Rows)
	}
}

func TestUpdateCanvasTreeViewerRefreshesMSAStateFromSharedPayload(t *testing.T) {
	w := NewBlastWizard(nil)
	server := phylo.NewViewerServer("127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("viewer server start: %v", err)
	}
	w.canvasTreeViewer = server
	server.SetMSAState("canvas", phylo.MSAState{
		SchemaVersion: 1,
		Rows: []phylo.MSASelectionRow{
			{TaxonID: "OLD", DisplayName: "old", Index: 0, State: "yellow"},
		},
	})
	plan := phylo.RunPlan{
		SessionID:    "canvas",
		RunID:        "run1",
		Settings:     phylo.DefaultTreeSettings(),
		AlignedFASTA: ">NEW\nAAAA\n",
		Newick:       "(NEW);",
		UpdatedAt:    time.Now(),
		Metadata: phylo.Metadata{
			SchemaVersion: 1,
			Records: []phylo.InputRecord{
				{TaxonID: "NEW", DisplayName: "new", CanvasItem: "msa rows", CanvasRow: 0},
			},
		},
		Records: []phylo.InputRecord{
			{TaxonID: "NEW", DisplayName: "new", CanvasItem: "msa rows", CanvasRow: 0},
		},
	}

	if err := w.updateCanvasTreeViewer(context.Background(), plan); err != nil {
		t.Fatalf("updateCanvasTreeViewer returned error: %v", err)
	}
	msaState := server.GetMSAState("canvas")
	if len(msaState.Rows) != 1 || msaState.Rows[0].TaxonID != "NEW" || msaState.Rows[0].State != "green" {
		t.Fatalf("MSA state should refresh from shared payload: %#v", msaState.Rows)
	}
	if len(w.canvasTreeMSAState.Rows) != 1 || w.canvasTreeMSAState.Rows[0].TaxonID != "NEW" {
		t.Fatalf("wizard MSA state not synchronized: %#v", w.canvasTreeMSAState.Rows)
	}
}

func TestMSAApplyYellowMarksCanvasAndUsesSharedPayload(t *testing.T) {
	w := NewBlastWizard(nil)
	server := phylo.NewViewerServer("127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("viewer server start: %v", err)
	}
	w.canvasTreeViewer = server
	w.canvasTreeLastPlan = phylo.RunPlan{
		Records: []phylo.InputRecord{
			{TaxonID: "PHGOT000001", CanvasItem: "msa rows", CanvasRow: 0},
			{TaxonID: "PHGOT000002", CanvasItem: "msa rows", CanvasRow: 1},
		},
	}
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "msa rows",
			Selected: []bool{true, false},
			Rows: []model.CanvasRow{
				{Kind: model.CanvasKindFasta, DisplayName: "green"},
				{Kind: model.CanvasKindFasta, DisplayName: "yellow"},
			},
		}},
	}
	refreshCalled := false
	w.canvasTreeMSAApplyRun = func(ctx context.Context, runState canvasLaunchState, settings phylo.TreeSettings) error {
		refreshCalled = true
		selected := selectedCanvasRowsInOrder(runState.Items)
		if len(selected) != 1 || selected[0].Row.DisplayName != "green" {
			t.Fatalf("tree refresh rows = %#v, want green row only", selected)
		}
		w.canvasTreeLastPayload = phylo.ViewerPayload{
			SchemaVersion: 1,
			SessionID:     "canvas",
			UpdatedAt:     time.Now(),
			AlignedFASTA:  ">PHGOT000001\nAAA\n",
			Metadata: phylo.Metadata{Records: []phylo.InputRecord{
				{TaxonID: "PHGOT000001", DisplayName: "green", CanvasItem: "msa rows", CanvasRow: 0},
			}},
		}
		return nil
	}
	w.installCanvasTreeMSAApplyHandlerOnServer(server, &state, nil)

	resp, err := http.Post(server.URL()+"/sessions/canvas/msa/apply", "application/json", strings.NewReader(`{"rows":[{"taxon_id":"PHGOT000001","state":"green"},{"taxon_id":"PHGOT000002","state":"yellow"}]}`))
	if err != nil {
		t.Fatalf("MSA apply post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("MSA apply status = %d body = %s", resp.StatusCode, body)
	}
	if !refreshCalled {
		t.Fatal("yellow MSA apply should refresh after marking the Canvas row")
	}
	if got, want := state.Items[0].Selected, []bool{true, false}; !slices.Equal(got, want) {
		t.Fatalf("canvas selection = %#v, want %#v", got, want)
	}
	if got, want := state.Items[0].MSAFlags, []bool{false, true}; !slices.Equal(got, want) {
		t.Fatalf("canvas MSA flags = %#v, want %#v", got, want)
	}
	if len(w.canvasTreeLastMSAPayload.Metadata.Records) != 1 || w.canvasTreeLastMSAPayload.Metadata.Records[0].TaxonID != "PHGOT000001" {
		t.Fatalf("MSA payload should match shared tree payload: %#v", w.canvasTreeLastMSAPayload.Metadata.Records)
	}
	msaState := server.GetMSAState("canvas")
	if len(msaState.Rows) != 1 || msaState.Rows[0].TaxonID != "PHGOT000001" || msaState.Rows[0].State != "green" {
		t.Fatalf("MSA state should contain only shared selected row: %#v", msaState.Rows)
	}
}

func TestMegaPHGORuntimeCanvasSnapshot123Probe(t *testing.T) {
	if strings.TrimSpace(os.Getenv("PHYTOZOME_RUN_MEGAPHGO_RUNTIME")) == "" {
		t.Skip("set PHYTOZOME_RUN_MEGAPHGO_RUNTIME=1 to run the real mega-phgo-runtime probe")
	}
	path := strings.TrimSpace(os.Getenv("PHYTOZOME_MEGAPHGO_PGO"))
	explicitPath := path != ""
	if path == "" {
		path = `C:\Users\wangsychn\Desktop\output\123.pgo`
	}
	if _, err := os.Stat(path); err != nil {
		if !explicitPath {
			t.Skipf("real Canvas snapshot %s is not available: %v", path, err)
		}
		t.Fatalf("real Canvas snapshot %s is not available: %v", path, err)
	}
	appRoot := repoRootForWorkflowRuntimeProbeTest(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working dir: %v", err)
	}
	if err := os.Chdir(appRoot); err != nil {
		t.Fatalf("switch to runtime app root %s: %v", appRoot, err)
	}
	defer func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore working dir %s: %v", oldWD, err)
		}
	}()

	snapshot, err := sessionsnapshot.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) returned error: %v", path, err)
	}
	if snapshot.Canvas == nil || len(snapshot.Canvas.Items) == 0 {
		t.Fatalf("snapshot %s does not contain Canvas items", path)
	}
	w := NewBlastWizard(nil)
	w.instanceID = "123-pgo-runtime-probe"
	w.hydrateSnapshotSequenceCache(snapshot.SequenceCache)
	items := w.hydrateCanvasRowSequenceData(canvasItemsFromSnapshot(snapshot.Canvas.Items))
	state := canvasLaunchState{Items: items}
	selected := selectedCanvasRowsInOrder(state.Items)
	if len(selected) < 2 {
		t.Fatalf("snapshot %s selected rows = %d, want at least two sequence-ready rows", path, len(selected))
	}

	proteinSettings := phylo.DefaultTreeSettings()
	proteinSettings.ConversionTarget = phylo.ConversionTargetProtein
	proteinSettings.AlignmentMethod = phylo.AlignmentClustalW
	result, err := w.buildCanvasTreeArtifacts(context.Background(), state, selected, proteinSettings)
	if err != nil {
		t.Fatalf("Protein runtime probe failed: %v\nartifacts: %s", err, result.ArtifactDir)
	}
	logText := readRuntimeLogForWorkflowProbe(t, result.ArtifactDir)
	if strings.Contains(logText, "conversion.applied") || strings.Contains(logText, "converted_dna_to_protein=") {
		t.Fatalf("Protein runtime probe should not perform PHgo DNA-to-protein conversion, got:\n%s", logText)
	}

	lemnaBlastState := canvasSnapshotSelectedRowsByDatabase(items, model.CanvasKindBlast, "lemna")
	lemnaSelected := selectedCanvasRowsInOrder(lemnaBlastState.Items)
	if len(lemnaSelected) < 2 {
		t.Fatalf("snapshot %s selected Lemna BLAST rows = %d, want at least two", path, len(lemnaSelected))
	}
	dnaResolveSettings := phylo.DefaultTreeSettings()
	dnaResolveSettings.ConversionTarget = phylo.ConversionTargetDNA
	dnaResolveSettings.AlignmentMethod = phylo.AlignmentClustalW
	result, err = w.buildCanvasTreeArtifacts(context.Background(), lemnaBlastState, lemnaSelected, dnaResolveSettings)
	if err != nil {
		t.Fatalf("DNA mode should resolve selected Lemna BLAST rows from snapshot source metadata: %v\nartifacts: %s", err, result.ArtifactDir)
	}
	for _, seq := range alignedSequencesForWorkflowTest(t, result.Plan.AlignedFASTA) {
		if !looksNucleotideOnlyForWorkflowTest(strings.ReplaceAll(seq, "-", "")) {
			t.Fatalf("resolved Lemna BLAST DNA alignment should stay nucleotide:\n%s", result.Plan.AlignedFASTA)
		}
	}

	dnaState := canvasSnapshotDNAOnlyState(items)
	dnaSelected := selectedCanvasRowsInOrder(dnaState.Items)
	if len(dnaSelected) < 2 {
		t.Fatalf("snapshot %s DNA-capable selected rows = %d, want at least two", path, len(dnaSelected))
	}
	for _, method := range []phylo.AlignmentMethod{phylo.AlignmentClustalW, phylo.AlignmentMUSCLE, phylo.AlignmentClustalWCodons, phylo.AlignmentMUSCLECodons} {
		t.Run("DNA_"+string(method), func(t *testing.T) {
			settings := phylo.DefaultTreeSettings()
			settings.ConversionTarget = phylo.ConversionTargetDNA
			settings.AlignmentMethod = method
			result, err := w.buildCanvasTreeArtifacts(context.Background(), dnaState, dnaSelected, settings)
			if err != nil {
				t.Fatalf("DNA runtime probe for %s failed: %v\nartifacts: %s", method, err, result.ArtifactDir)
			}
			for _, seq := range alignedSequencesForWorkflowTest(t, result.Plan.AlignedFASTA) {
				if !looksNucleotideOnlyForWorkflowTest(strings.ReplaceAll(seq, "-", "")) {
					t.Fatalf("DNA aligned FASTA for %s should stay nucleotide:\n%s", method, result.Plan.AlignedFASTA)
				}
			}
			summary, err := os.ReadFile(filepath.Join(result.ArtifactDir, "runtime-summary.txt"))
			if err != nil {
				t.Fatalf("read runtime summary: %v", err)
			}
			if !strings.Contains(string(summary), "alignment_method="+string(method)) {
				t.Fatalf("runtime summary did not record %s:\n%s", method, summary)
			}
		})
	}
}

func TestMegaPHGORuntimeDesktop4CLSnapshotSourceExeProbe(t *testing.T) {
	if strings.TrimSpace(os.Getenv("PHYTOZOME_RUN_MEGAPHGO_RUNTIME")) == "" || strings.TrimSpace(os.Getenv("PHYTOZOME_MEGAPHGO_4CL_PGO")) == "" {
		t.Skip("set PHYTOZOME_RUN_MEGAPHGO_RUNTIME=1 and PHYTOZOME_MEGAPHGO_4CL_PGO=1 to run the desktop 4CL .pgo source-runtime probe")
	}
	path, sourceExe := desktop4CLRuntimeProbePaths(t)

	snapshot, err := sessionsnapshot.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) returned error: %v", path, err)
	}
	if snapshot.Canvas == nil || len(snapshot.Canvas.Items) == 0 {
		t.Fatalf("snapshot %s does not contain Canvas items", path)
	}
	w := NewBlastWizard(nil)
	w.instanceID = "desktop-4cl-pgo-source-runtime-probe"
	w.hydrateSnapshotSequenceCache(snapshot.SequenceCache)
	items := w.hydrateCanvasRowSequenceData(canvasItemsFromSnapshot(snapshot.Canvas.Items))
	state := canvasLaunchState{Items: items}
	selected := selectedCanvasRowsInOrder(state.Items)
	if len(selected) < 2 {
		t.Fatalf("snapshot %s selected rows = %d, want at least two sequence-ready rows", path, len(selected))
	}

	now := time.Now()
	settings := phylo.DefaultTreeSettings()
	settings.ConversionTarget = phylo.ConversionTargetProtein
	settings.AlignmentMethod = phylo.AlignmentClustalW
	settings.TreeMethod = phylo.TreeNeighborJoining
	rowSources, err := w.canvasTreeRowSourcesWithSkippedForSettings(context.Background(), state, selected, settings)
	if err != nil {
		t.Fatalf("canvas row source hydration failed: %v", err)
	}
	records, meta, err := phylo.BuildInput(rowSources, settings.DisplayNameSource, w.canvasTreeSessionID(), now)
	if err != nil {
		t.Fatalf("BuildInput returned error: %v", err)
	}
	artifactDir := filepath.Join(os.TempDir(), "phgo-megaphgo-4cl-pgo-"+now.Format("20060102-150405.000000000"))
	plan, err := phylo.BuildRunPlan(w.canvasTreeSessionID(), "source-exe-probe", artifactDir, settings, phylo.SequenceProtein, records, meta, "", "", now)
	if err != nil {
		t.Fatalf("BuildRunPlan returned error: %v", err)
	}
	if err := plan.ToArtifactSet().Write(); err != nil {
		t.Fatalf("write plan artifacts: %v", err)
	}
	requestPath, err := phylo.WriteMegaPHGORuntimeRequest(plan)
	if err != nil {
		t.Fatalf("WriteMegaPHGORuntimeRequest returned error: %v", err)
	}
	cmd := exec.CommandContext(context.Background(), sourceExe, requestPath)
	cmd.Dir = artifactDir
	output, runErr := cmd.CombinedOutput()
	t.Logf("snapshot=%s", path)
	t.Logf("source_exe=%s", sourceExe)
	t.Logf("artifact_dir=%s", artifactDir)
	if len(output) > 0 {
		t.Logf("runtime_output=%s", strings.TrimSpace(string(output)))
	}

	response, responseErr := readMegaPHGORuntimeResponseForWorkflowProbe(filepath.Join(artifactDir, phylo.RuntimeResponseFile))
	if responseErr != nil {
		t.Fatalf("runtime did not write a readable response: %v\nrunErr=%v\n%s", responseErr, runErr, string(output))
	}
	logText := readRuntimeLogForWorkflowProbe(t, artifactDir)
	if runErr == nil && strings.TrimSpace(response.ErrorText) == "" {
		if !strings.Contains(logText, "clustalw.complete canceled=false") {
			t.Fatalf("ClustalW did not complete normally; runtime log:\n%s", logText)
		}
		if strings.TrimSpace(response.Artifacts.Newick) == "" {
			t.Fatalf("successful runtime response should include Newick artifact: %#v", response.Artifacts)
		}
		return
	}
	if len(response.SkippedRecords) == 0 {
		t.Fatalf("runtime failed without mapping MEGA skipped records; runErr=%v error_text=%q\nlog:\n%s", runErr, response.ErrorText, logText)
	}
	t.Logf("runtime_error=%s", response.ErrorText)
	for _, skipped := range response.SkippedRecords {
		t.Logf("skipped taxon_id=%s item=%s row=%d reason=%s", skipped.TaxonID, skipped.ItemTitle, skipped.RowIndex, skipped.Reason)
	}
}

func TestMegaPHGORuntimeDesktop4CLSkipRetriesWithoutSamePair(t *testing.T) {
	if strings.TrimSpace(os.Getenv("PHYTOZOME_RUN_MEGAPHGO_RUNTIME")) == "" || strings.TrimSpace(os.Getenv("PHYTOZOME_MEGAPHGO_4CL_PGO")) == "" {
		t.Skip("set PHYTOZOME_RUN_MEGAPHGO_RUNTIME=1 and PHYTOZOME_MEGAPHGO_4CL_PGO=1 to run the desktop 4CL .pgo source-runtime skip probe")
	}
	path, sourceExe := desktop4CLRuntimeProbePaths(t)
	snapshot, err := sessionsnapshot.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) returned error: %v", path, err)
	}
	if snapshot.Canvas == nil || len(snapshot.Canvas.Items) == 0 {
		t.Fatalf("snapshot %s does not contain Canvas items", path)
	}

	w := NewBlastWizard(nil)
	w.instanceID = "desktop-4cl-pgo-source-runtime-skip-probe"
	w.hydrateSnapshotSequenceCache(snapshot.SequenceCache)
	state := canvasLaunchState{Items: w.hydrateCanvasRowSequenceData(canvasItemsFromSnapshot(snapshot.Canvas.Items))}
	settings := phylo.DefaultTreeSettings()
	settings.ConversionTarget = phylo.ConversionTargetProtein
	settings.AlignmentMethod = phylo.AlignmentClustalW
	settings.TreeMethod = phylo.TreeNeighborJoining
	sourceRuntime := workflowSourceExeRuntime{exe: sourceExe}
	var firstSkipped []canvasTreeSkippedRow
	var firstSelected []canvasSelectedRow
	callCount := 0
	skipCount := 0
	const maxRuntimeRefreshes = 8
	w.canvasTreeRefreshRun = func(ctx context.Context, runState canvasLaunchState, settings phylo.TreeSettings) error {
		callCount++
		if callCount > maxRuntimeRefreshes {
			t.Fatalf("automatic Skip exceeded %d runtime refreshes", maxRuntimeRefreshes)
		}
		selected := selectedCanvasRowsInOrder(runState.Items)
		result, err := w.buildCanvasTreeArtifactsWithRuntime(ctx, runState, selected, settings, sourceRuntime)
		t.Logf("refresh_call=%d selected=%d artifact_dir=%s", callCount, len(selected), result.ArtifactDir)
		if err == nil {
			return nil
		}
		var skippedErr *canvasTreeSkippedRowsError
		if !errors.As(err, &skippedErr) {
			t.Fatalf("runtime refresh failed without skip mapping on call %d: %v\nartifacts: %s", callCount, err, result.ArtifactDir)
		}
		t.Logf("refresh_call=%d skipped=%s", callCount, canvasTreeSkipSummary(skippedErr.SkippedRows))
		if callCount == 1 {
			firstSkipped = append([]canvasTreeSkippedRow(nil), skippedErr.SkippedRows...)
			firstSelected = append([]canvasSelectedRow(nil), selected...)
			if !canvasTreeSkippedRowsContainTaxa(firstSkipped, "PHGOT000089", "PHGOT000085") {
				t.Fatalf("first 4CL skip did not expose expected runtime pair: %#v", firstSkipped)
			}
			return err
		}
		if canvasTreeSkippedRowsContainTaxa(skippedErr.SkippedRows, "PHGOT000089", "PHGOT000085") {
			t.Fatalf("Skip did not remove the first runtime pair; second skipped rows: %#v", skippedErr.SkippedRows)
		}
		return err
	}
	w.canvasTreeRecover = func(description string, backTarget error, allowSkip bool) (string, error) {
		skipCount++
		if !allowSkip {
			t.Fatalf("runtime recovery should allow Skip")
		}
		if callCount == 1 && (!strings.Contains(description, "PHGOT000089") || !strings.Contains(description, "PHGOT000085")) {
			t.Fatalf("first recovery dialog did not name expected runtime taxa: %q", description)
		}
		return "skip", nil
	}
	if err := w.refreshCanvasTreeInteractive(context.Background(), &state, settings); err != nil {
		t.Fatalf("interactive runtime refresh returned error: %v", err)
	}
	if callCount < 2 {
		t.Fatalf("Skip probe only ran %d refresh call(s), want at least 2", callCount)
	}
	if skipCount != 1 {
		t.Fatalf("automatic Skip should prompt once, got %d prompts", skipCount)
	}
	if len(firstSkipped) == 0 {
		t.Fatal("Skip probe did not capture the first runtime skipped rows")
	}
	for _, skipped := range resolveCanvasTreeSkippedRows(firstSelected, firstSkipped) {
		if skipped.ItemIndex < 0 || skipped.ItemIndex >= len(state.Items) {
			t.Fatalf("resolved skipped item index is invalid: %#v", skipped)
		}
		selected := normalizeCanvasSelection(state.Items[skipped.ItemIndex].Selected, len(state.Items[skipped.ItemIndex].Rows))
		if skipped.RowIndex < 0 || skipped.RowIndex >= len(selected) {
			t.Fatalf("resolved skipped row index is invalid: %#v", skipped)
		}
		if selected[skipped.RowIndex] {
			t.Fatalf("4CL Skip should uncheck %s row %d after first runtime failure: %#v", skipped.ItemTitle, skipped.RowIndex+1, selected)
		}
	}
}

func canvasSnapshotDNAOnlyState(items []model.CanvasItem) canvasLaunchState {
	out := cloneCanvasItems(items)
	for itemIndex := range out {
		selected := make([]bool, len(out[itemIndex].Rows))
		for rowIndex, row := range out[itemIndex].Rows {
			if row.Kind != model.CanvasKindFasta || row.FASTA == nil {
				continue
			}
			choice := canvasFastaTreeSequenceChoice(*row.FASTA, phylo.SequenceNucleotide)
			if strings.TrimSpace(choice.Sequence) != "" && choice.Kind == phylo.SequenceNucleotide {
				selected[rowIndex] = true
			}
		}
		out[itemIndex].Selected = selected
		updateCanvasItemSubtitle(&out[itemIndex])
	}
	return canvasLaunchState{Items: out}
}

func canvasSnapshotSelectedRowsByDatabase(items []model.CanvasItem, kind model.CanvasKind, database string) canvasLaunchState {
	out := cloneCanvasItems(items)
	database = strings.ToLower(strings.TrimSpace(database))
	for itemIndex := range out {
		selected := make([]bool, len(out[itemIndex].Rows))
		for rowIndex, row := range out[itemIndex].Rows {
			if row.Kind != kind {
				continue
			}
			if strings.EqualFold(normalizeSnapshotDatabase(canvasRowSourceDatabase(row)), database) {
				selected[rowIndex] = true
			}
		}
		out[itemIndex].Selected = selected
		updateCanvasItemSubtitle(&out[itemIndex])
	}
	return canvasLaunchState{Items: out}
}

func desktop4CLRuntimeProbePaths(t *testing.T) (string, string) {
	t.Helper()
	path := strings.TrimSpace(os.Getenv("PHYTOZOME_MEGAPHGO_PGO"))
	if path == "" {
		path = `C:\Users\wangsychn\Desktop\4CLtree.pgo`
	}
	sourceExe := strings.TrimSpace(os.Getenv("PHYTOZOME_MEGAPHGO_SOURCE_EXE"))
	appRoot := repoRootForWorkflowRuntimeProbeTest(t)
	if sourceExe == "" {
		sourceExe = filepath.Join(appRoot, `_mega_source\MEGA12.1-source\PHgoRuntime\lib\x86_64-win64\mega-phgo-runtime.exe`)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("desktop Canvas snapshot %s is not available: %v", path, err)
	}
	if _, err := os.Stat(sourceExe); err != nil {
		t.Fatalf("MEGA PHgo source runtime exe %s is not available: %v", sourceExe, err)
	}
	if !strings.EqualFold(filepath.Ext(sourceExe), ".exe") {
		t.Fatalf("MEGA PHgo source runtime must be an .exe for this probe, got %s", sourceExe)
	}
	return path, sourceExe
}

type workflowSourceExeRuntime struct {
	exe string
}

func (r workflowSourceExeRuntime) Name() string {
	return "mega-phgo-runtime-source-exe"
}

func (r workflowSourceExeRuntime) Run(ctx context.Context, plan phylo.RunPlan) (phylo.RunResult, error) {
	if err := plan.ToArtifactSet().Write(); err != nil {
		return phylo.RunResult{}, err
	}
	requestPath, err := phylo.WriteMegaPHGORuntimeRequest(plan)
	if err != nil {
		return phylo.RunResult{}, err
	}
	cmd := exec.CommandContext(ctx, r.exe, requestPath)
	cmd.Dir = plan.BaseDir
	output, runErr := cmd.CombinedOutput()
	responsePath := filepath.Join(plan.BaseDir, phylo.RuntimeResponseFile)
	response, readErr := readMegaPHGORuntimeResponseForWorkflowProbe(responsePath)
	if readErr != nil {
		if runErr != nil {
			return phylo.RunResult{Plan: plan, ArtifactDir: plan.BaseDir, ErrorText: strings.TrimSpace(string(output))}, fmt.Errorf("%v: %w", runErr, readErr)
		}
		return phylo.RunResult{Plan: plan, ArtifactDir: plan.BaseDir, ErrorText: readErr.Error()}, readErr
	}
	response.SkippedRecords = phylo.SkippedRecordsForRuntimeError(plan, response.ErrorText, response.SkippedRecords)
	result := phylo.RunResult{
		Plan:           plan,
		ArtifactDir:    plan.BaseDir,
		ErrorText:      strings.TrimSpace(firstNonEmpty(response.ErrorText, string(output))),
		SkippedRecords: append([]phylo.RuntimeSkippedRecord(nil), response.SkippedRecords...),
	}
	if responseErr := strings.TrimSpace(response.ErrorText); responseErr != "" {
		return result, fmt.Errorf("%s", responseErr)
	}
	if runErr != nil {
		return result, runErr
	}
	alignedFASTA, err := os.ReadFile(filepath.Join(plan.BaseDir, "aligned.fasta"))
	if err != nil {
		return result, err
	}
	newick, err := os.ReadFile(filepath.Join(plan.BaseDir, "tree.nwk"))
	if err != nil {
		return result, err
	}
	finalPlan, err := phylo.BuildRunPlan(plan.SessionID, plan.RunID, plan.BaseDir, plan.Settings, plan.Kind, plan.Records, plan.Metadata, string(alignedFASTA), string(newick), time.Now())
	if err != nil {
		return result, err
	}
	if err := finalPlan.ToArtifactSet().Write(); err != nil {
		return result, err
	}
	result.Plan = finalPlan
	result.SelectedNewick = filepath.Join(plan.BaseDir, "tree.nwk")
	result.SummaryPath = filepath.Join(plan.BaseDir, "runtime-summary.txt")
	return result, nil
}

type workflowStaticRuntime struct{}

func (workflowStaticRuntime) Name() string {
	return "static-runtime"
}

func (workflowStaticRuntime) Run(ctx context.Context, plan phylo.RunPlan) (phylo.RunResult, error) {
	aligned := phylo.InputFASTA(plan.Records)
	names := make([]string, 0, len(plan.Records))
	for _, record := range plan.Records {
		names = append(names, record.TaxonID)
	}
	newick := "(" + strings.Join(names, ",") + ");"
	finalPlan, err := phylo.BuildRunPlan(plan.SessionID, plan.RunID, plan.BaseDir, plan.Settings, plan.Kind, plan.Records, plan.Metadata, aligned, newick, time.Now())
	if err != nil {
		return phylo.RunResult{}, err
	}
	if err := finalPlan.ToArtifactSet().Write(); err != nil {
		return phylo.RunResult{}, err
	}
	return phylo.RunResult{
		Plan:           finalPlan,
		ArtifactDir:    finalPlan.BaseDir,
		SelectedNewick: filepath.Join(finalPlan.BaseDir, "tree.nwk"),
	}, nil
}

func canvasTreeSkippedRowsContainTaxa(rows []canvasTreeSkippedRow, taxa ...string) bool {
	if len(taxa) == 0 {
		return false
	}
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		seen[strings.ToUpper(strings.TrimSpace(row.TaxonID))] = true
	}
	for _, taxon := range taxa {
		if !seen[strings.ToUpper(strings.TrimSpace(taxon))] {
			return false
		}
	}
	return true
}

func repoRootForWorkflowRuntimeProbeTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working dir: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %s", dir)
		}
		dir = parent
	}
}

func readRuntimeLogForWorkflowProbe(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "runtime.log"))
	if err != nil {
		t.Fatalf("read runtime log: %v", err)
	}
	return string(data)
}

func readMegaPHGORuntimeResponseForWorkflowProbe(path string) (phylo.MegaPHGORuntimeResponse, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return phylo.MegaPHGORuntimeResponse{}, err
	}
	var response phylo.MegaPHGORuntimeResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return phylo.MegaPHGORuntimeResponse{}, err
	}
	return response, nil
}

func alignedSequencesForWorkflowTest(t *testing.T, fasta string) []string {
	t.Helper()
	var seqs []string
	var seq strings.Builder
	flush := func() {
		if seq.Len() == 0 {
			return
		}
		seqs = append(seqs, seq.String())
		seq.Reset()
	}
	for _, line := range strings.Split(strings.ReplaceAll(fasta, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ">") {
			flush()
			continue
		}
		seq.WriteString(line)
	}
	flush()
	if len(seqs) == 0 {
		t.Fatalf("expected aligned FASTA sequences")
	}
	return seqs
}

func looksNucleotideOnlyForWorkflowTest(sequence string) bool {
	sequence = strings.TrimSpace(strings.ToUpper(sequence))
	if sequence == "" {
		return false
	}
	for _, ch := range sequence {
		if !strings.ContainsRune("ACGTUNRYKMSWBDHV", ch) {
			return false
		}
	}
	return true
}

func treePanelForSnapshotTest() tui.CanvasTreePanelState {
	return tui.CanvasTreePanelState{
		EnabledEver:            true,
		CurrentControl:         0,
		DisplayNameSource:      "label_name",
		ConversionTarget:       string(phylo.ConversionTargetProtein),
		ConversionSkipUnselect: true,
		AlignmentMethod:        string(phylo.AlignmentMUSCLE),
		TreeMethod:             string(phylo.TreeNeighborJoining),
		AlignmentParams:        map[string]string{"max_iterations": "8"},
		TreeParams:             map[string]string{"distance_model": "p-distance"},
	}
}

func treeSnapshotForTest(dir string, now time.Time) sessionsnapshot.CanvasTreeV2 {
	panel := treePanelForSnapshotTest()
	record := phylo.InputRecord{TaxonID: "PHGOT000001", DisplayName: "PAL display", SequenceKind: phylo.SequenceProtein}
	meta := phylo.Metadata{SchemaVersion: 1, GeneratedAt: now, DisplayNameSource: "label_name", Records: []phylo.InputRecord{record}}
	return sessionsnapshot.CanvasTreeV2{
		PanelState:       panel,
		LastPayload:      phylo.ViewerPayload{SchemaVersion: 1, SessionID: "canvas", UpdatedAt: now, Newick: "(PHGOT000001);", AlignedFASTA: ">PHGOT000001\nMPEPTIDE\n", Metadata: meta},
		ViewerState:      json.RawMessage(`{"schema_version":2,"reactree":{"layout":"rectangular","fontFamily":"Georgia"}}`),
		LastManifest:     treeRunManifestForSnapshotTest(now),
		LastArtifactDir:  dir,
		LastRunID:        "run1",
		LastAlignedFASTA: ">PHGOT000001\nMPEPTIDE\n",
		LastNewick:       "(PHGOT000001);",
		Fingerprints:     phylo.Fingerprints{Alignment: "a", Tree: "t"},
	}
}

func treeRunManifestForSnapshotTest(now time.Time) phylo.RunManifest {
	return phylo.RunManifest{
		SchemaVersion:   1,
		CreatedAt:       now,
		Settings:        phylo.TreeSettings{DisplayNameSource: "label_name", AlignmentMethod: phylo.AlignmentMUSCLE, AlignmentParams: map[string]string{"max_iterations": "8"}, TreeMethod: phylo.TreeNeighborJoining, TreeParams: map[string]string{"distance_model": "p-distance"}},
		Fingerprints:    phylo.Fingerprints{Alignment: "manifest-align", Tree: "manifest-tree", Preview: "manifest-preview"},
		InputFASTA:      "input.fasta",
		MetadataJSON:    "input.meta.json",
		RuntimeRequest:  "runtime-request.json",
		RuntimeResponse: "runtime-response.json",
		AlignedFASTA:    "aligned.fasta",
		NewickPath:      "tree.nwk",
	}
}

func writeTreeSnapshotRestoreArtifacts(t *testing.T, dir string, snapshot sessionsnapshot.CanvasTreeV2) {
	t.Helper()
	files := map[string]string{
		"input.fasta":           ">PHGOT000001\nMPEPTIDE\n",
		"runtime-request.json":  `{"schema_version":1,"session_id":"canvas"}`,
		"runtime-response.json": `{"schema_version":1,"runtime":"mega-phgo-runtime"}`,
		"aligned.fasta":         ">PHGOT000001\nMPEPTIDE\n",
		"tree.nwk":              "(PHGOT000001);",
	}
	meta, err := json.Marshal(snapshot.LastPayload.Metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	files["input.meta.json"] = string(meta)
	manifest, err := json.Marshal(snapshot.LastManifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	files["run.manifest.json"] = string(manifest)
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

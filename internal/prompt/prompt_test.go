// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package prompt

import (
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/phylo"
	"github.com/KiriKirby/phytozome-go/internal/tui"
	"github.com/gdamore/tcell/v2"
)

func TestIdentityRowOrder(t *testing.T) {
	rows := []model.BlastResultRow{
		{Protein: "a", PercentIdentity: 71},
		{Protein: "b", PercentIdentity: 88},
		{Protein: "c", PercentIdentity: 88},
		{Protein: "d", PercentIdentity: 63},
	}

	order := identityRowOrder(rows)
	want := []int{1, 2, 0, 3}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("unexpected order at %d: got %d want %d", i, order[i], want[i])
		}
	}
}

func TestKeywordRowAliasSelectionPinsAliasForDownstreamUse(t *testing.T) {
	row := model.KeywordResultRow{
		LabelName:   "OLD",
		PhgoAliases: "OLD; C4H; C4H; CYP73A",
	}

	choices := keywordRowAliasChoices(row)
	if got, want := strings.Join(choices.Aliases, "|"), "OLD|C4H|CYP73A"; got != want {
		t.Fatalf("unexpected alias choices: got %q want %q", got, want)
	}

	row = keywordRowWithSelectedAlias(row, "C4H")
	if row.LabelName != "C4H" {
		t.Fatalf("label name was not updated: %q", row.LabelName)
	}
	if row.LabelNameType != "user selected alias" {
		t.Fatalf("unexpected label type: %q", row.LabelNameType)
	}
	if got, want := row.PhgoAliases, "C4H; OLD; CYP73A"; got != want {
		t.Fatalf("selected alias should be pinned in phgo_alias: got %q want %q", got, want)
	}
}

func TestBlastRowAliasSelectionPinsAliasForDownstreamUse(t *testing.T) {
	row := model.BlastResultRow{
		LabelName:        "OLD",
		PhgoAliases:      "OLD; PAL1; PAL1; PAL2",
		UniProtGeneNames: "UPAL",
		Protein:          "prot1",
	}

	choices := blastRowAliasChoices(row)
	if got, want := strings.Join(choices.Aliases, "|"), "OLD|PAL1|PAL2"; got != want {
		t.Fatalf("unexpected BLAST alias choices: got %q want %q", got, want)
	}

	row = blastRowWithSelectedAlias(row, "PAL1")
	if row.LabelName != "PAL1" {
		t.Fatalf("label name was not updated: %q", row.LabelName)
	}
	if row.LabelNameType != "user selected alias" {
		t.Fatalf("unexpected label type: %q", row.LabelNameType)
	}
	if got, want := row.PhgoAliases, "PAL1; OLD; PAL2"; got != want {
		t.Fatalf("selected alias should be pinned in phgo_alias: got %q want %q", got, want)
	}
}

func TestBlastRowAliasChoicesFallBackToRowIdentifiers(t *testing.T) {
	row := model.BlastResultRow{
		LabelName:        "HIT",
		UniProtGeneNames: "PAL1 PAL2",
		Protein:          "prot1",
		SequenceID:       "seq1",
	}
	choices := blastRowAliasChoices(row)
	got := strings.Join(choices.Aliases, "|")
	for _, want := range []string{"HIT", "PAL1 PAL2", "prot1", "seq1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("fallback aliases missing %q from %q", want, got)
		}
	}
}

func TestBuildCanvasSelectionTableForFastaUsesFixedCanvasColumns(t *testing.T) {
	item := model.CanvasItem{
		Title: "1",
		Kind:  model.CanvasKindFasta,
		Rows: []model.CanvasRow{{
			Kind: model.CanvasKindFasta,
			FASTA: &model.QuerySequenceSource{
				Annotation:    ">seq1 some header",
				Sequence:      "MPEPTIDE",
				ProteinID:     "P1",
				LabelName:     "PAL1",
				OrganismShort: "Athaliana",
			},
		}},
	}
	columns, rows := buildCanvasSelectionTable(item)
	gotIDs := make([]string, 0, len(columns))
	for _, column := range columns {
		gotIDs = append(gotIDs, column.ID)
	}
	wantIDs := []string{"source_type", "head", "species", "label_name", "gene_id", "source_label_name", "source_gene_id", "display_name"}
	if strings.Join(gotIDs, "|") != strings.Join(wantIDs, "|") {
		t.Fatalf("canvas FASTA columns = %#v, want %v", gotIDs, wantIDs)
	}
	if len(rows) != 1 || len(rows[0].Cells) != len(wantIDs) {
		t.Fatalf("canvas FASTA rows = %#v, want one fixed-column row", rows)
	}
	if rows[0].Cells[1] != "seq1 some header" || rows[0].Cells[2] != "Athaliana" || rows[0].Cells[3] != "PAL1" || rows[0].Cells[4] != "" || rows[0].Cells[5] != "" || rows[0].Cells[6] != "" || rows[0].Cells[7] != "PAL1" {
		t.Fatalf("canvas FASTA row cells = %#v", rows[0].Cells)
	}
	if !rows[0].SelectableSet || !rows[0].Selectable {
		t.Fatalf("canvas FASTA row should be selectable when sequence is present: %#v", rows[0])
	}
}

func TestBuildCanvasSelectionTableForPhgoFastaKeepsHeadAndMetadata(t *testing.T) {
	item := model.CanvasItem{
		Title: "1",
		Kind:  model.CanvasKindFasta,
		Rows: []model.CanvasRow{{
			Kind: model.CanvasKindFasta,
			FASTA: &model.QuerySequenceSource{
				Annotation:           "phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\7",
				Sequence:             "MPEPTIDE",
				LabelName:            "C4H",
				GeneID:               "Sp7498_C4H_001",
				OrganismShort:        "Sp7498",
				BlastSourceLabelName: "PAL1",
				BlastSourceGeneID:    "AT2G37040",
				PhgoRowNumber:        7,
				PhgoHasRowNumber:     true,
			},
		}},
	}
	_, rows := buildCanvasSelectionTable(item)
	if len(rows) != 1 {
		t.Fatalf("canvas rows = %#v", rows)
	}
	want := []string{"fasta", "phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\7", "Sp7498", "C4H", "Sp7498_C4H_001", "PAL1", "AT2G37040", "C4H"}
	if strings.Join(rows[0].Cells, "|") != strings.Join(want, "|") {
		t.Fatalf("phgo canvas cells = %#v, want %#v", rows[0].Cells, want)
	}
}

func TestCanvasItemRowNumbersPreservesNegativeValues(t *testing.T) {
	item := model.CanvasItem{
		Rows: []model.CanvasRow{
			{RowNumber: -2},
			{RowNumber: -1},
			{RowNumber: 1},
			{RowNumber: 2},
		},
	}
	got := canvasItemRowNumbers(item)
	want := []int{-2, -1, 1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("canvasItemRowNumbers = %#v, want %#v", got, want)
	}
}

func TestCanvasDisplayNameSourceExcludesDisplayNameAndRemovedSourceRow(t *testing.T) {
	item := model.CanvasItem{
		Kind: model.CanvasKindFasta,
		Rows: []model.CanvasRow{{
			Kind: model.CanvasKindFasta,
			FASTA: &model.QuerySequenceSource{
				Annotation:    "seq1 some header",
				Sequence:      "MPEPTIDE",
				LabelName:     "PAL1",
				GeneID:        "AT2G37040",
				OrganismShort: "Athaliana",
			},
		}},
	}
	columns, _ := buildCanvasSelectionTable(item)
	got := canvasDisplayNameSourceColumnIDs(columns)
	if slices.Contains(got, "display_name") || slices.Contains(got, "source_row") {
		t.Fatalf("display-name source choices include forbidden columns: %v", got)
	}
	if !slices.Contains(got, "label_name") {
		t.Fatalf("display-name source choices should include label_name: %v", got)
	}
}

func TestApplyCanvasDisplayNameSourceFallsBackToOriginalHead(t *testing.T) {
	item := model.CanvasItem{
		Title: "fallback",
		Rows: []model.CanvasRow{
			{
				Kind:       model.CanvasKindKeyword,
				KeywordRow: &model.KeywordResultRow{LabelName: "PAL1", SequenceHeaderLabel: "ATPAL1.1"},
			},
			{
				Kind:  model.CanvasKindFasta,
				FASTA: &model.QuerySequenceSource{Annotation: ">seq2 original head", Sequence: "MPEPTIDE"},
			},
		},
	}
	applyCanvasDisplayNameSource(&item, "label_name")
	if got := item.Rows[0].DisplayName; got != "PAL1" {
		t.Fatalf("keyword display name = %q, want PAL1", got)
	}
	if got := item.Rows[1].DisplayName; got != "seq2 original head" {
		t.Fatalf("FASTA display name fallback = %q, want original head", got)
	}
}

func TestApplyCanvasDisplayNameSourceSkipsLockedRows(t *testing.T) {
	item := model.CanvasItem{
		Title: "fallback",
		Rows: []model.CanvasRow{
			{
				Kind:              model.CanvasKindKeyword,
				DisplayName:       "Locked Name",
				DisplayNameLocked: true,
				KeywordRow:        &model.KeywordResultRow{LabelName: "PAL1", SequenceHeaderLabel: "ATPAL1.1"},
			},
			{
				Kind:       model.CanvasKindKeyword,
				KeywordRow: &model.KeywordResultRow{LabelName: "PAL2", SequenceHeaderLabel: "ATPAL2.1"},
			},
		},
	}
	applyCanvasDisplayNameSource(&item, "label_name")
	if got := item.Rows[0].DisplayName; got != "Locked Name" {
		t.Fatalf("locked display name = %q, want Locked Name", got)
	}
	if got := item.Rows[1].DisplayName; got != "PAL2" {
		t.Fatalf("unlocked display name = %q, want PAL2", got)
	}
}

func TestApplyCanvasDisplayNameSourceUsesPHgoLabelFormat(t *testing.T) {
	item := model.CanvasItem{
		Title: "fallback",
		Rows: []model.CanvasRow{{
			Kind: model.CanvasKindKeyword,
			KeywordRow: &model.KeywordResultRow{
				SourceDatabase: "ncbi",
				Genome:         "Oryza sativa Japonica Group",
				GeneLocus:      "Os08g14760",
				GeneIdentifier: "GeneID:4335555",
				LabelName:      "Os4CL1",
			},
		}},
	}
	applyCanvasDisplayNameSource(&item, phylo.PHgoDisplayNameSource)
	if got := item.Rows[0].DisplayName; got != "Os-Os08g14760 (Os4CL1)" {
		t.Fatalf("PHgo display name = %q", got)
	}
}

func TestApplyCanvasDisplayNameSourceUsesYTLabelFormat(t *testing.T) {
	item := model.CanvasItem{
		Title: "fallback",
		Rows: []model.CanvasRow{{
			Kind: model.CanvasKindKeyword,
			KeywordRow: &model.KeywordResultRow{
				SourceDatabase: "ncbi",
				Genome:         "Arabidopsis thaliana",
				GeneLocus:      "At1G51680",
				GeneIdentifier: "GeneID:841572",
				LabelName:      "At4CL1",
			},
		}, {
			Kind: model.CanvasKindKeyword,
			KeywordRow: &model.KeywordResultRow{
				SourceDatabase: "ncbi",
				Genome:         "Panicum virgatum",
				GeneLocus:      "Pavir.6KG154400",
			},
		}},
	}
	applyCanvasDisplayNameSource(&item, phylo.YTDisplayNameSource)
	if got := item.Rows[0].DisplayName; got != "At1G51680_At4CL1" {
		t.Fatalf("YT display name = %q", got)
	}
	if got := item.Rows[1].DisplayName; got != "Pavir.6KG154400" {
		t.Fatalf("YT display name without label = %q", got)
	}
}

func TestApplyCanvasDisplayNameSourceUsesYTV2LabelFormat(t *testing.T) {
	item := model.CanvasItem{
		Title: "fallback",
		Rows: []model.CanvasRow{{
			Kind: model.CanvasKindKeyword,
			KeywordRow: &model.KeywordResultRow{
				Genome:    "Arabidopsis thaliana",
				GeneLocus: "At1G51680",
				LabelName: "At4CL1",
			},
		}, {
			Kind: model.CanvasKindKeyword,
			KeywordRow: &model.KeywordResultRow{
				Genome:    "Panicum virgatum",
				GeneLocus: "Pavir.6KG154400",
			},
		}},
	}
	applyCanvasDisplayNameSource(&item, phylo.YTV2DisplayNameSource)
	if got := item.Rows[0].DisplayName; got != "At1G51680_4CL1" {
		t.Fatalf("YT v2 display name = %q", got)
	}
	if got := item.Rows[1].DisplayName; got != "Pavir.6KG154400" {
		t.Fatalf("YT v2 display name without label = %q", got)
	}
}

func TestCanvasKeywordFixedGeneIDPrefersGeneLocus(t *testing.T) {
	item := model.CanvasItem{
		Title: "1",
		Kind:  model.CanvasKindKeyword,
		Rows: []model.CanvasRow{{
			Kind: model.CanvasKindKeyword,
			KeywordRow: &model.KeywordResultRow{
				SourceDatabase:      "ncbi",
				SequenceID:          "XP_015650724.1",
				ProteinID:           "XP_015650724.1",
				GeneLocus:           "Os08g14760",
				GeneIdentifier:      "GeneID:4335555",
				LabelName:           "Os4CL1",
				Genome:              "Oryza sativa Japonica Group",
				ExtraColumns:        map[string]string{"ncbi_gene_id": "4335555"},
				SequenceHeaderLabel: "XP_015650724.1",
			},
		}},
	}
	_, rows := buildCanvasSelectionTable(item)
	if len(rows) != 1 {
		t.Fatalf("canvas rows = %#v", rows)
	}
	if got := rows[0].Cells[4]; got != "Os08g14760" {
		t.Fatalf("canvas fixed geneid = %q, want Gene locus", got)
	}
	if rows[0].Cells[5] != "" || rows[0].Cells[6] != "" {
		t.Fatalf("keyword-origin canvas should not synthesize BLAST source fields: %#v", rows[0].Cells)
	}
}

func TestBuildCanvasTreePanelFiltersCodonMethodsByConversionTarget(t *testing.T) {
	proteinItem := model.CanvasItem{
		Title:    "protein",
		Kind:     model.CanvasKindFasta,
		Selected: []bool{true},
		Rows: []model.CanvasRow{{
			Kind:  model.CanvasKindFasta,
			FASTA: &model.QuerySequenceSource{Sequence: "MPEPTIDE"},
		}},
	}
	proteinViewColumns, proteinViewRows := buildCanvasSelectionTable(proteinItem)
	proteinPanel := buildCanvasTreePanel([]tui.BlastRunItem{{
		Columns:  proteinViewColumns,
		Rows:     proteinViewRows,
		Selected: []bool{true},
	}}, []model.CanvasItem{proteinItem}, phylo.DefaultTreeSettings(), tui.CanvasTreePanelState{})
	for _, method := range proteinPanel.AlignmentMethods {
		if strings.Contains(method.ID, "codons") {
			t.Fatalf("protein panel should hide codon method: %#v", proteinPanel.AlignmentMethods)
		}
	}
	foundProteinClustal := false
	foundProteinMuscle := false
	for _, method := range proteinPanel.AlignmentMethods {
		if method.ID == "clustalw_protein" {
			foundProteinClustal = true
		}
		if method.ID == "muscle_protein" {
			foundProteinMuscle = true
		}
	}
	if !foundProteinClustal || !foundProteinMuscle {
		t.Fatalf("protein panel should expose protein-specific methods: %#v", proteinPanel.AlignmentMethods)
	}
	if got := len(proteinPanel.AlignmentMethods); got != 2 {
		t.Fatalf("protein mode alignment method count = %d, want 2: %#v", got, proteinPanel.AlignmentMethods)
	}

	nucleotideItem := model.CanvasItem{
		Title:    "nucleotide",
		Kind:     model.CanvasKindFasta,
		Selected: []bool{true},
		Rows: []model.CanvasRow{{
			Kind:  model.CanvasKindFasta,
			FASTA: &model.QuerySequenceSource{Sequence: "ATGGCCATGGCC"},
		}},
	}
	nucViewColumns, nucViewRows := buildCanvasSelectionTable(nucleotideItem)
	nucleotidePanel := buildCanvasTreePanel([]tui.BlastRunItem{{
		Columns:  nucViewColumns,
		Rows:     nucViewRows,
		Selected: []bool{true},
	}}, []model.CanvasItem{nucleotideItem}, phylo.DefaultTreeSettings(), tui.CanvasTreePanelState{})
	for _, method := range nucleotidePanel.AlignmentMethods {
		if strings.Contains(method.ID, "codons") {
			t.Fatalf("default protein conversion target should hide codon method: %#v", nucleotidePanel.AlignmentMethods)
		}
	}

	dnaPanel := buildCanvasTreePanel([]tui.BlastRunItem{{
		Columns:  nucViewColumns,
		Rows:     nucViewRows,
		Selected: []bool{true},
	}}, []model.CanvasItem{nucleotideItem}, phylo.DefaultTreeSettings(), tui.CanvasTreePanelState{ConversionTarget: string(phylo.ConversionTargetDNA)})
	hasCodon := false
	hasNucleotideClustal := false
	hasNucleotideMuscle := false
	for _, method := range dnaPanel.AlignmentMethods {
		if strings.Contains(method.ID, "codons") {
			hasCodon = true
		}
		if method.ID == "clustalw" {
			hasNucleotideClustal = true
		}
		if method.ID == "muscle" {
			hasNucleotideMuscle = true
		}
	}
	if !hasCodon {
		t.Fatalf("DNA mode panel should expose codon-capable methods: %#v", dnaPanel.AlignmentMethods)
	}
	if !hasNucleotideClustal || !hasNucleotideMuscle {
		t.Fatalf("DNA mode panel should expose nucleotide-specific methods: %#v", dnaPanel.AlignmentMethods)
	}
	if got := len(dnaPanel.AlignmentMethods); got != 4 {
		t.Fatalf("DNA mode alignment method count = %d, want 4: %#v", got, dnaPanel.AlignmentMethods)
	}
}

func TestBuildCanvasTreePanelPreviewAvailabilityIsExplicit(t *testing.T) {
	item := model.CanvasItem{
		Title:    "protein",
		Kind:     model.CanvasKindFasta,
		Selected: []bool{true},
		Rows: []model.CanvasRow{{
			Kind:  model.CanvasKindFasta,
			FASTA: &model.QuerySequenceSource{Sequence: "MPEPTIDE"},
		}},
	}
	columns, rows := buildCanvasSelectionTable(item)
	viewItems := []tui.BlastRunItem{{
		Columns:  columns,
		Rows:     rows,
		Selected: []bool{true},
	}}
	if panel := buildCanvasTreePanel(viewItems, []model.CanvasItem{item}, phylo.DefaultTreeSettings(), tui.CanvasTreePanelState{}); panel.PreviewAvailable {
		t.Fatal("preview should be unavailable before a shared tree/MSA payload exists")
	}
	if panel := buildCanvasTreePanel(viewItems, []model.CanvasItem{item}, phylo.DefaultTreeSettings(), tui.CanvasTreePanelState{}, true); !panel.PreviewAvailable {
		t.Fatal("preview should be available after the workflow reports a complete shared payload")
	}
}

func TestBuildCanvasTreePanelIncludesFormattedDisplayNameSources(t *testing.T) {
	item := model.CanvasItem{
		Title:    "protein",
		Kind:     model.CanvasKindKeyword,
		Selected: []bool{true},
		Rows: []model.CanvasRow{{
			Kind:       model.CanvasKindKeyword,
			KeywordRow: &model.KeywordResultRow{SequenceID: "XP_015650724.1", LabelName: "Os4CL1"},
		}},
	}
	columns, rows := buildCanvasSelectionTable(item)
	panel := buildCanvasTreePanel([]tui.BlastRunItem{{
		Columns:  columns,
		Rows:     rows,
		Selected: []bool{true},
	}}, []model.CanvasItem{item}, phylo.DefaultTreeSettings(), tui.CanvasTreePanelState{})
	foundPHgo := false
	foundYT := false
	foundYTV2 := false
	for _, choice := range panel.DisplayNameSources {
		if choice.Value == phylo.PHgoDisplayNameSource && choice.Label == "PHgo label format" {
			foundPHgo = true
		}
		if choice.Value == phylo.YTDisplayNameSource && choice.Label == "YT label format" {
			foundYT = true
		}
		if choice.Value == phylo.YTV2DisplayNameSource && choice.Label == "YT v2 label format" {
			foundYTV2 = true
		}
	}
	if !foundPHgo || !foundYT || !foundYTV2 {
		t.Fatalf("formatted display sources missing: %#v", panel.DisplayNameSources)
	}
}

func TestCanvasRowWithDisplayNameDoesNotChangeLabelName(t *testing.T) {
	row := model.CanvasRow{
		Kind:       model.CanvasKindKeyword,
		KeywordRow: &model.KeywordResultRow{LabelName: "PAL1"},
	}
	row = canvasRowWithDisplayName(row, "Tree PAL")
	if row.DisplayName != "Tree PAL" {
		t.Fatalf("display name = %q, want Tree PAL", row.DisplayName)
	}
	if row.KeywordRow == nil || row.KeywordRow.LabelName != "PAL1" {
		t.Fatalf("display-name edit should not rewrite label_name: %#v", row.KeywordRow)
	}
	if !row.DisplayNameLocked {
		t.Fatal("manual display-name edit should lock the row")
	}
}

func TestBuildCanvasSelectionTableDisablesRowsWithoutSequence(t *testing.T) {
	item := model.CanvasItem{
		Title: "1",
		Kind:  model.CanvasKindFasta,
		Rows: []model.CanvasRow{{
			Kind:  model.CanvasKindFasta,
			FASTA: &model.QuerySequenceSource{Annotation: "seq1 header"},
		}},
	}
	_, rows := buildCanvasSelectionTable(item)
	if len(rows) != 1 || !rows[0].SelectableSet || rows[0].Selectable {
		t.Fatalf("row without FASTA sequence should be non-selectable: %#v", rows)
	}
}

func TestBuildCanvasSelectionTableDisablesRowsMarkedSequenceUnavailable(t *testing.T) {
	ready := false
	item := model.CanvasItem{
		Title: "1",
		Kind:  model.CanvasKindKeyword,
		Rows: []model.CanvasRow{{
			Kind:          model.CanvasKindKeyword,
			KeywordRow:    &model.KeywordResultRow{SequenceID: "AT1G01010.1", TranscriptID: "AT1G01010.1"},
			SequenceReady: &ready,
		}},
	}
	_, rows := buildCanvasSelectionTable(item)
	if len(rows) != 1 || !rows[0].SelectableSet || rows[0].Selectable {
		t.Fatalf("row marked without sequence should be non-selectable: %#v", rows)
	}
}

func TestCloneCanvasItemsPreservesMetadata(t *testing.T) {
	ready := false
	items := []model.CanvasItem{{
		Title:        "group 1",
		Subtitle:     "0/1 lines",
		Kind:         model.CanvasKindKeyword,
		Selected:     []bool{true},
		SourceLabel:  "PAL1",
		ImportedFrom: "snapshot",
		Rows: []model.CanvasRow{{
			RowNumber:         9,
			Kind:              model.CanvasKindKeyword,
			DisplayName:       "PAL display",
			DisplayNameLocked: true,
			KeywordRow:        &model.KeywordResultRow{LabelName: "PAL1"},
			SequenceReady:     &ready,
		}},
	}}
	cloned := cloneCanvasItems(items)
	if len(cloned) != 1 || cloned[0].SourceLabel != "PAL1" || cloned[0].ImportedFrom != "snapshot" {
		t.Fatalf("clone did not preserve canvas metadata: %#v", cloned)
	}
	if len(cloned[0].Rows) != 1 || cloned[0].Rows[0].RowNumber != 9 {
		t.Fatalf("clone did not preserve row number: %#v", cloned[0].Rows)
	}
	if cloned[0].Rows[0].DisplayName != "PAL display" {
		t.Fatalf("clone did not preserve display name: %#v", cloned[0].Rows[0])
	}
	if !cloned[0].Rows[0].DisplayNameLocked {
		t.Fatalf("clone did not preserve display-name lock: %#v", cloned[0].Rows[0])
	}
	if cloned[0].Rows[0].SequenceReady == nil || *cloned[0].Rows[0].SequenceReady {
		t.Fatalf("clone did not preserve sequence availability: %#v", cloned[0].Rows[0])
	}
}

func TestCanvasSelectionStructKeepsSaveFields(t *testing.T) {
	got := CanvasSelection{
		SaveBaseName: "canvas1",
		WriteSession: true,
	}
	if got.SaveBaseName != "canvas1" || !got.WriteSession {
		t.Fatalf("canvas save fields not preserved: %#v", got)
	}
}

func TestCanvasSaveSettingsStructKeepsFastaExportFields(t *testing.T) {
	got := CanvasSaveSettings{
		BaseName:        "canvas1",
		WriteSession:    true,
		WriteText:       true,
		WriteAllRows:    true,
		FastaHeaderMode: model.FastaHeaderModePhgo,
		UsePhgoHeader:   true,
	}
	if got.BaseName != "canvas1" || !got.WriteSession || !got.WriteText || !got.WriteAllRows || got.FastaHeaderMode != model.FastaHeaderModePhgo || !got.UsePhgoHeader {
		t.Fatalf("canvas save settings fields not preserved: %#v", got)
	}
}

func TestCanvasDefaultFixedColumnsUseDisplaynameHeader(t *testing.T) {
	columns := canvasDefaultFixedColumns()
	found := false
	for _, column := range columns {
		if column.ID != "display_name" {
			continue
		}
		found = true
		if column.Header != "displayname" {
			t.Fatalf("display_name header = %q, want displayname", column.Header)
		}
	}
	if !found {
		t.Fatal("display_name column missing from canvas defaults")
	}
}

func TestCanvasKeywordStoredDetailFASTAPrefersNCBIInlineFASTA(t *testing.T) {
	row := model.CanvasRow{
		Kind: model.CanvasKindKeyword,
		KeywordRow: &model.KeywordResultRow{
			LabelName:  "PAL1",
			SequenceID: "XP_001",
			ExtraColumns: map[string]string{
				"ncbi_fasta": ">XP_001 NCBI protein\nMPEPTIDE",
			},
		},
	}
	got := canvasKeywordStoredDetailFASTA(row)
	if got != ">XP_001 NCBI protein\nMPEPTIDE" {
		t.Fatalf("stored NCBI FASTA detail = %q", got)
	}
}

func TestBuildCanvasSelectionTableReturnsTableRows(t *testing.T) {
	item := model.CanvasItem{
		Title: "PAL",
		Kind:  model.CanvasKindKeyword,
		Rows: []model.CanvasRow{{
			Kind:       model.CanvasKindKeyword,
			KeywordRow: &model.KeywordResultRow{LabelName: "PAL1", ProteinID: "AT2G37040.1"},
		}},
	}
	columns, rows := buildCanvasSelectionTable(item)
	if len(columns) == 0 || len(rows) != 1 {
		t.Fatalf("canvas keyword table not built: columns=%#v rows=%#v", columns, rows)
	}
	if len(rows[0].DetailPages) == 0 && strings.TrimSpace(rows[0].Detail) == "" {
		t.Fatalf("canvas keyword detail should be preserved: %#v", rows[0])
	}
}

func TestAliasLikeTableColumnsShowFirstFiveAliasesOnly(t *testing.T) {
	full := "A1; A2; A3; A4; A5; A6; A7"
	want := "A1; A2; A3; A4; A5; ..."

	_, keywordRows := buildKeywordSelectionTable([]model.KeywordResultRow{{
		SourceDatabase: "phytozome",
		SearchTerm:     "PAL",
		LabelName:      "PAL1",
		PhgoAliases:    full,
	}})
	if got := tableCellByColumn(t, keywordDisplayColumns([]model.KeywordResultRow{{SourceDatabase: "phytozome", LabelName: "PAL1", PhgoAliases: full}}), keywordRows[0], "phgo_alias"); got != want {
		t.Fatalf("keyword phgo_alias display = %q, want %q", got, want)
	}
	if !strings.Contains(keywordRows[0].Detail, full) {
		t.Fatalf("keyword detail should keep full aliases: %q", keywordRows[0].Detail)
	}

	_, blastRows := buildBlastSelectionTable([]model.BlastResultRow{{
		SourceDatabase: "phytozome",
		BlastProgram:   "BLASTP",
		LabelName:      "PAL1",
		PhgoAliases:    full,
	}})
	if got := tableCellByColumn(t, blastDisplayColumns([]model.BlastResultRow{{SourceDatabase: "phytozome", BlastProgram: "BLASTP", LabelName: "PAL1", PhgoAliases: full}}), blastRows[0], "phgo_alias"); got != want {
		t.Fatalf("blast phgo_alias display = %q, want %q", got, want)
	}
	if !strings.Contains(blastRows[0].Detail, full) {
		t.Fatalf("blast detail should keep full aliases: %q", blastRows[0].Detail)
	}

	item := model.CanvasItem{
		Title: "PAL",
		Kind:  model.CanvasKindKeyword,
		ActiveColumns: []model.CanvasColumn{
			{ID: "phgo_alias", Header: "phgo_alias"},
		},
		Rows: []model.CanvasRow{{
			Kind:       model.CanvasKindKeyword,
			KeywordRow: &model.KeywordResultRow{LabelName: "PAL1", PhgoAliases: full},
		}},
	}
	canvasColumns, canvasRows := buildCanvasSelectionTable(item)
	if got := tableCellByColumnID(t, canvasColumns, canvasRows[0], "phgo_alias"); got != want {
		t.Fatalf("canvas phgo_alias display = %q, want %q", got, want)
	}
}

func tableCellByColumn[T any](t testing.TB, defs []tableColumnValue[T], row tui.TableRow, id string) string {
	t.Helper()
	columns := make([]tui.TableColumn, 0, len(defs))
	for _, def := range defs {
		columns = append(columns, tui.TableColumn{ID: def.ID})
	}
	return tableCellByColumnID(t, columns, row, id)
}

func tableCellByColumnID(t testing.TB, columns []tui.TableColumn, row tui.TableRow, id string) string {
	t.Helper()
	for i, column := range columns {
		if column.ID == id {
			if i >= len(row.Cells) {
				t.Fatalf("row has %d cells, missing column %q at %d", len(row.Cells), id, i)
			}
			return row.Cells[i]
		}
	}
	t.Fatalf("missing column %q in %#v", id, columns)
	return ""
}

func TestApplySelectionCommandUpDownAndRange(t *testing.T) {
	selected := []bool{false, false, false, false, false}
	order := []int{0, 1, 2, 3, 4}

	if err := applySelectionCommand(selected, order, []string{"up", "3"}, true); err != nil {
		t.Fatalf("apply up failed: %v", err)
	}
	if !selected[0] || !selected[1] || !selected[2] || selected[3] || selected[4] {
		t.Fatalf("unexpected selection after up: %v", selected)
	}

	if err := applySelectionCommand(selected, order, []string{"down", "4"}, false); err != nil {
		t.Fatalf("apply down failed: %v", err)
	}
	if selected[3] || selected[4] {
		t.Fatalf("down should include row 4 and below: %v", selected)
	}

	if err := applySelectionCommand(selected, order, []string{"2~4"}, true); err != nil {
		t.Fatalf("apply range failed: %v", err)
	}
	if !selected[1] || !selected[2] || !selected[3] {
		t.Fatalf("range 2~4 should be selected: %v", selected)
	}
}

func TestParseRowSpecRangeReversed(t *testing.T) {
	got, err := parseRowSpec("5~3", 8)
	if err != nil {
		t.Fatalf("parse reversed range failed: %v", err)
	}
	want := []int{3, 4, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected parsed range at %d: got %d want %d", i, got[i], want[i])
		}
	}
}

func TestParseKeywordIdentityValues(t *testing.T) {
	got := parseKeywordIdentityValues([]string{"ID1 ~", "ID3"})
	want := []string{"ID1", "", "ID3"}
	if len(got) != len(want) {
		t.Fatalf("unexpected length: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected value at %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestKeywordWideSearchContextSupportsPhytozomeAndLemna(t *testing.T) {
	tests := []struct {
		name string
		path []string
		want bool
	}{
		{name: "phytozome", path: []string{"phytozome", "Startup"}, want: true},
		{name: "lemna", path: []string{"lemna", "Startup"}, want: true},
		{name: "blast", path: []string{"blast", "Startup"}, want: false},
	}

	for _, tt := range tests {
		p := &Prompter{sessionPath: tt.path}
		if got := p.keywordWideSearchContext(); got != tt.want {
			t.Fatalf("%s context = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestUpdateOpenCanvasRefreshStatusUsesRegisteredCallback(t *testing.T) {
	p := New(nil, io.Discard)
	type call struct {
		refreshing bool
		message    string
	}
	var calls []call
	p.canvasExternalRefresh = func(refreshing bool, message string) {
		calls = append(calls, call{refreshing: refreshing, message: message})
	}

	p.UpdateOpenCanvasRefreshStatus(true, "  Refreshing tree and MSA...  ")
	p.UpdateOpenCanvasRefreshStatus(false, "")

	want := []call{
		{refreshing: true, message: "Refreshing tree and MSA..."},
		{refreshing: false, message: ""},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("refresh status calls = %#v, want %#v", calls, want)
	}
}

func TestParseBlastIdentityValues(t *testing.T) {
	got := parseBlastIdentityValues([]string{"A", "~", "C"})
	want := []string{"A", "", "C"}
	if len(got) != len(want) {
		t.Fatalf("unexpected length: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected value at %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestBlastRowsDefaultBackTargetReturnsToQueryInput(t *testing.T) {
	if got := blastRowsBackTarget(); got != ErrBackToQueryInput {
		t.Fatalf("BLAST row table Back should return to BLAST input, got %v", got)
	}
}

func TestExportSettingsStructSupportsDefaultPhgoHeaderPreference(t *testing.T) {
	settings := ExportSettings{
		WriteText:       true,
		WriteExcel:      true,
		WriteRawExcel:   false,
		FastaHeaderMode: model.FastaHeaderModePhgo,
		UsePhgoHeader:   true,
	}
	if !settings.UsePhgoHeader {
		t.Fatalf("expected phgo header preference to be enabled in export settings test fixture: %#v", settings)
	}
	if settings.FastaHeaderMode != model.FastaHeaderModePhgo {
		t.Fatalf("expected phgo header mode in export settings test fixture: %#v", settings)
	}
}

func TestBuildKeywordSelectionTableKeepsRealRowsOnly(t *testing.T) {
	rows := []model.KeywordResultRow{{SearchTerm: "alpha", LabelName: "C4H", TranscriptID: "AT1G01010.1"}}
	columns, tableRows := buildKeywordSelectionTable(rows)
	if len(columns) == 0 {
		t.Fatal("expected keyword columns")
	}
	if len(tableRows) != 1 {
		t.Fatalf("table rows = %d, want 1 real row", len(tableRows))
	}
	if tableRows[0].Group != "alpha" {
		t.Fatalf("unexpected row group: %q", tableRows[0].Group)
	}
	if columns[1].ID != "search_type" {
		t.Fatalf("expected search_type column after search_term, got %#v", columns)
	}
	if columns[2].ID != "label_name" {
		t.Fatalf("expected label_name column, got %#v", columns)
	}
}

func TestSelectKeywordRowsDefaultSelectionPicksFirstRowPerGroup(t *testing.T) {
	groups := []model.KeywordSearchGroup{
		{SearchTerm: "alpha", Rows: []model.KeywordResultRow{{SearchTerm: "alpha"}, {SearchTerm: "alpha"}}},
		{SearchTerm: "beta", Rows: []model.KeywordResultRow{{SearchTerm: "beta"}, {SearchTerm: "beta"}}},
	}
	totalRows := countKeywordResultRows(groups)
	selected := make([]bool, totalRows)
	offset := 0
	for _, group := range groups {
		if len(group.Rows) > 0 {
			selected[offset] = true
		}
		offset += len(group.Rows)
	}
	want := []bool{true, false, true, false}
	if !slices.Equal(selected, want) {
		t.Fatalf("default keyword selection = %v, want %v", selected, want)
	}
}

func TestFamilyContextDefaultSelectionStartsEmpty(t *testing.T) {
	p := New(strings.NewReader(""), io.Discard)
	p.SetDatabaseContext("TAIR")
	restore := p.PushSessionContext("Family", "TAIR family")
	defer restore()
	if !p.familyContext() {
		t.Fatal("expected family context")
	}
	if got := p.keywordRowSelectionDescription(); !strings.Contains(strings.ToLower(got), "no rows are selected by default") {
		t.Fatalf("unexpected family selection description: %q", got)
	}
}

func TestKeywordRowDetailIncludesAllAvailableColumns(t *testing.T) {
	row := model.KeywordResultRow{
		SearchTerm:     "alpha",
		SearchType:     "keyword",
		LabelName:      "C4H",
		ProteinID:      "prot123",
		TranscriptID:   "AT1G01010.1",
		GeneIdentifier: "AT1G01010",
		Genome:         "Arabidopsis",
		Description:    "desc",
	}
	detail := keywordRowDetail(row)
	for _, want := range []string{"search_type: keyword", "label_name: C4H", "protein_id: prot123", "geneid: AT1G01010", "transcript: AT1G01010.1"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("keyword detail missing %q: %s", want, detail)
		}
	}
	pages := keywordRowDetailPages(row)
	if len(pages) != 3 {
		t.Fatalf("detail pages = %d, want 3", len(pages))
	}
	if pages[0].Title != "Source" || pages[1].Title != "Annotation" || pages[2].Title != "FASTA" {
		t.Fatalf("unexpected page titles: %#v", pages)
	}
	if len(pages[0].Items) == 0 {
		t.Fatal("expected detail items on first page")
	}
	if got := pages[2].Items[0].Value; got != "Sequence not loaded yet." {
		t.Fatalf("unexpected FASTA placeholder: %q", got)
	}
	if !pages[2].Items[0].AutoLoad {
		t.Fatal("expected FASTA page to auto-load")
	}
}

func TestBlastDisplayKeepsLengthComparisonColumnsInOriginalPosition(t *testing.T) {
	rows := []model.BlastResultRow{{
		SourceDatabase:                      "lemna",
		Protein:                             "Spipo11G0031600",
		UniProtReferenceEnabled:             true,
		UniProtAccession:                    "A0A1234567",
		TargetUniProtCanonicalLengthPercent: "98.50",
		UniProtCanonicalLength:              "480",
	}}
	columns, tableRows := buildBlastSelectionTable(rows)
	ids := make([]string, 0, len(columns))
	headers := make(map[string]string, len(columns))
	for _, column := range columns {
		ids = append(ids, column.ID)
		headers[column.ID] = column.Header
	}
	joined := strings.Join(ids, ",")
	for _, unexpected := range []string{"uniprot_keywords", "uniprot_ec", "uniprot_go"} {
		if strings.Contains(joined, unexpected) {
			t.Fatalf("display columns should not include %s: %v", unexpected, ids)
		}
	}
	targetIdx := indexOfString(ids, "target_length")
	ratioIdx := indexOfString(ids, "target_uniprot_canonical_length_percent")
	alignIdx := indexOfString(ids, "align_len")
	canonicalIdx := indexOfString(ids, "uniprot_canonical_length")
	speciesIdx := indexOfString(ids, "species")
	accessionIdx := indexOfString(ids, "uniprot_accession")
	if !(targetIdx >= 0 && ratioIdx == targetIdx+1 && alignIdx == ratioIdx+1) {
		t.Fatalf("length comparison column should remain beside target_length: %v", ids)
	}
	if !(canonicalIdx >= 0 && speciesIdx == canonicalIdx+1) {
		t.Fatalf("canonical length should remain before species: %v", ids)
	}
	if accessionIdx <= speciesIdx {
		t.Fatalf("other UniProt display columns should be after original columns: %v", ids)
	}
	if headers["target_uniprot_canonical_length_percent"] != "lemna target_length /\nUniProt canonical length (%)" {
		t.Fatalf("unexpected dynamic header: %q", headers["target_uniprot_canonical_length_percent"])
	}
	if got := tableRows[0].Cells[ratioIdx]; got != "98.50" {
		t.Fatalf("ratio cell = %q", got)
	}
	if got := tableRows[0].Cells[canonicalIdx]; got != "480" {
		t.Fatalf("canonical length cell = %q", got)
	}
}

func TestBlastDisplayLeavesUniProtCellsBlankWithoutUniProtData(t *testing.T) {
	rows := []model.BlastResultRow{{
		SourceDatabase:                      "lemna",
		Protein:                             "Spipo11G0031600",
		UniProtReferenceEnabled:             true,
		TargetUniProtCanonicalLengthPercent: "98.50",
		UniProtCanonicalLength:              "480",
		UniProtProteinName:                  "Should stay blank",
	}}
	columns, tableRows := buildBlastSelectionTable(rows)
	for index, column := range columns {
		if strings.HasPrefix(column.ID, "uniprot_") || column.ID == "target_uniprot_canonical_length_percent" {
			if got := tableRows[0].Cells[index]; got != "" {
				t.Fatalf("%s cell = %q, want blank", column.ID, got)
			}
		}
	}

	detail := blastRowDetail(rows[0])
	if !strings.Contains(detail, "lemna target_length / UniProt canonical length (%): ") {
		t.Fatalf("detail missing dynamic ratio label: %s", detail)
	}
	if strings.Contains(detail, "Should stay blank") || strings.Contains(detail, "UniProt canonical length: 480") {
		t.Fatalf("detail should blank UniProt values without accession: %s", detail)
	}
	pages := blastRowDetailPages(rows[0])
	if len(pages) != 4 {
		t.Fatalf("detail pages = %d, want 4", len(pages))
	}
	if pages[0].Title != "Source" || pages[1].Title != "Query Metadata" || pages[2].Title != "UniProt" || pages[3].Title != "FASTA" {
		t.Fatalf("unexpected page titles: %#v", pages)
	}
	if got := pages[2].Items[0].Value; got != "" {
		t.Fatalf("expected blank UniProt detail cell without accession, got %q", got)
	}
	if !pages[3].Items[0].AutoLoad {
		t.Fatal("expected blast FASTA page to auto-load")
	}
}

func TestBlastDisplayAppliesInterProQueryComparisonColors(t *testing.T) {
	rows := []model.BlastResultRow{{
		SourceDatabase:                 "phytozome",
		InterProReferenceEnabled:       true,
		InterProEntryName:              "Domain A",
		InterProMatchRegions:           "IPR1:1-10",
		InterProAccessions:             "IPR1",
		InterProEntryNameComparison:    "green",
		InterProMatchRegionsComparison: "yellow",
		InterProAccessionsComparison:   "red",
	}}
	columns, tableRows := buildBlastSelectionTable(rows)
	if len(tableRows) != 1 || len(tableRows[0].CellColors) != len(columns) {
		t.Fatalf("unexpected InterPro color row: %#v", tableRows)
	}
	wants := map[string]tcell.Color{
		"interpro_entry_name":    tcell.ColorGreen,
		"interpro_match_regions": tcell.ColorYellow,
		"interpro_accessions":    tcell.ColorRed,
	}
	for i, column := range columns {
		if want, ok := wants[column.ID]; ok && tableRows[0].CellColors[i] != want {
			t.Fatalf("%s color = %v, want %v", column.ID, tableRows[0].CellColors[i], want)
		}
	}
}

func TestKeywordRowDetailPagesExposeExtraColumnsAsSeparateTab(t *testing.T) {
	row := model.KeywordResultRow{
		SourceDatabase: "lemna",
		SearchTerm:     "alpha",
		LabelName:      "PAL1",
		TranscriptID:   "TR1",
		Description:    "desc",
		Genome:         "Spirodela",
		ExtraColumns: map[string]string{
			"attr_Alias": "alias1",
			"gff_source": "maker",
		},
	}
	pages := keywordRowDetailPages(row)
	if len(pages) != 4 {
		t.Fatalf("detail pages = %d, want 4", len(pages))
	}
	if pages[2].Title != "Extra" {
		t.Fatalf("third page title = %q, want Extra", pages[2].Title)
	}
	if len(pages[2].Items) != 2 {
		t.Fatalf("extra page items = %d, want 2", len(pages[2].Items))
	}
}

func TestKeywordRowDetailPagesPutNCBIInlineFASTAOnlyOnFASTATab(t *testing.T) {
	row := model.KeywordResultRow{
		SourceDatabase:      "ncbi",
		SearchTerm:          "XP_015650724.1",
		LabelName:           "Os4CL1",
		ProteinID:           "XP_015650724.1",
		SequenceID:          "XP_015650724.1",
		SequenceHeaderLabel: "Oryza sativa Japonica Group",
		ExtraColumns: map[string]string{
			"ncbi_accession":        "XP_015650724.1",
			"ncbi_fasta_header":     ">XP_015650724.1 probable 4-coumarate--CoA ligase 1",
			"ncbi_protein_sequence": "MNCBISEQ",
			"ncbi_fasta":            ">XP_015650724.1 probable 4-coumarate--CoA ligase 1\nMNCBISEQ\n",
		},
	}
	pages := keywordRowDetailPages(row)
	if len(pages) != 4 {
		t.Fatalf("detail pages = %d, want 4", len(pages))
	}
	if pages[2].Title != "Extra" || pages[3].Title != "FASTA" {
		t.Fatalf("unexpected page titles: %#v", pages)
	}
	for _, item := range pages[2].Items {
		if strings.Contains(strings.ToLower(item.Label), "fasta") || strings.Contains(strings.ToLower(item.Label), "sequence") {
			t.Fatalf("FASTA/sequence payload leaked into Extra tab: %#v", item)
		}
	}
	if pages[3].Items[0].AutoLoad {
		t.Fatal("inline NCBI FASTA should be shown immediately, not loaded asynchronously")
	}
	if got := pages[3].Items[0].Value; !strings.Contains(got, ">XP_015650724.1 probable 4-coumarate--CoA ligase 1") || !strings.Contains(got, "MNCBISEQ") {
		t.Fatalf("FASTA tab missing inline NCBI sequence: %q", got)
	}
}

func TestKeywordDetailColumnIDsForRowUsesNCBISearchTypeSpecificSchema(t *testing.T) {
	geneRow := model.KeywordResultRow{
		SourceDatabase: "ncbi",
		ExtraColumns: map[string]string{
			"ncbi_search_type_id": "gene",
			"ncbi_result_domain":  "gene-record",
		},
	}
	nuccoreRow := model.KeywordResultRow{
		SourceDatabase: "ncbi",
		ExtraColumns: map[string]string{
			"ncbi_search_type_id": "nuccore",
			"ncbi_result_domain":  "sequence-record",
		},
	}
	geneIDs := keywordDetailColumnIDsForRow(geneRow)
	nuccoreIDs := keywordDetailColumnIDsForRow(nuccoreRow)
	if !slices.Contains(geneIDs, "gene_locus") || !slices.Contains(geneIDs, "symbols") {
		t.Fatalf("gene detail ids missing expected fields: %#v", geneIDs)
	}
	if slices.Contains(nuccoreIDs, "gene_locus") || !slices.Contains(nuccoreIDs, "transcript") {
		t.Fatalf("nuccore detail ids should be nucleotide-oriented: %#v", nuccoreIDs)
	}
}

func TestKeywordDisplayColumnsMixedPLAZAAndNCBIPreserveSharedFields(t *testing.T) {
	plazaRow := model.KeywordResultRow{
		SourceDatabase: "plaza",
		SearchTerm:     "4CL1",
		SearchType:     "PLAZA Gene locus priority",
		GeneLocus:      "AT1G51680",
		ProteinID:      "AT1G51680.1",
		TranscriptID:   "AT1G51680.1",
		GeneIdentifier: "AT1G51680",
		Genome:         "Arabidopsis thaliana",
		UniProt:        "Q42524",
		Description:    "4-coumarate--CoA ligase 1",
	}
	ncbiRow := model.KeywordResultRow{
		SourceDatabase: "ncbi",
		SearchTerm:     "PAL1",
		SearchType:     "protein",
		ProteinID:      "NP_001000001.1",
		GeneIdentifier: "12345",
		Genome:         "Arabidopsis thaliana",
		Description:    "phenylalanine ammonia-lyase",
		ExtraColumns: map[string]string{
			"ncbi_search_type_id": "protein",
			"ncbi_result_domain":  "sequence-record",
		},
	}
	defs := keywordDisplayColumns([]model.KeywordResultRow{plazaRow, ncbiRow})
	values := make(map[string]string, len(defs))
	for _, def := range defs {
		if def.Value == nil {
			t.Fatalf("column %q has no value function", def.ID)
		}
		values[def.ID] = def.Value(plazaRow)
	}
	for _, required := range []string{
		"source_database", "search_term", "search_type", "gene_locus", "protein_id",
		"transcript", "gene_identifier", "genome", "uniprot", "description",
	} {
		if _, ok := values[required]; !ok {
			t.Fatalf("mixed PLAZA/NCBI table missing %q: %#v", required, values)
		}
	}
	if got := values["source_database"]; got != "plaza" {
		t.Fatalf("PLAZA source_database = %q, want plaza", got)
	}
	if got := values["search_type"]; got != "PLAZA Gene locus priority" {
		t.Fatalf("PLAZA search_type = %q", got)
	}
	if !strings.Contains(keywordRowDetail(plazaRow), "source_database: plaza") {
		t.Fatalf("PLAZA detail did not retain source database: %q", keywordRowDetail(plazaRow))
	}
}

func TestDetailPageIsFASTADetectsLastTabOnly(t *testing.T) {
	if !detailPageIsFASTA(2, 0, 3) {
		t.Fatal("expected last page first item to be FASTA")
	}
	if detailPageIsFASTA(1, 0, 3) {
		t.Fatal("non-last page must not be treated as FASTA")
	}
	if detailPageIsFASTA(2, 1, 3) {
		t.Fatal("non-first item must not be treated as FASTA loader target")
	}
}

func TestDetailFASTACacheKeyIsStableAndCacheable(t *testing.T) {
	row := model.BlastResultRow{
		SourceDatabase: "lemna",
		TargetID:       42,
		SequenceID:     "SEQ1",
		TranscriptID:   "TR1",
		Protein:        "PROT1",
	}
	key := blastDetailFASTACacheKey(row)
	if key == "" {
		t.Fatal("expected non-empty cache key")
	}
	p := New(nil, nil)
	p.detailFASTACache[key] = ">PROT1\nMPEPTIDE"
	got := p.detailFASTACache[blastDetailFASTACacheKey(row)]
	if !strings.Contains(got, "MPEPTIDE") {
		t.Fatalf("cache lookup failed: %q", got)
	}
}

func TestCanvasFastaDetailPagesUseLoadedSequence(t *testing.T) {
	source := model.QuerySequenceSource{
		Annotation: "phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\7",
		Sequence:   "MPEPTIDE",
	}
	pages := canvasFastaDetailPages(source, "canvas")
	if len(pages) == 0 {
		t.Fatal("expected detail pages")
	}
	last := pages[len(pages)-1]
	if last.Title != "FASTA" {
		t.Fatalf("last page title = %q, want FASTA", last.Title)
	}
	if len(last.Items) != 1 || last.Items[0].AutoLoad {
		t.Fatalf("expected loaded FASTA item, got %#v", last.Items)
	}
	if !strings.Contains(last.Items[0].Value, "MPEPTIDE") {
		t.Fatalf("loaded FASTA value = %q", last.Items[0].Value)
	}
}

func TestDefaultBlastFilterSuggestionUsesReferenceEvidenceHardRules(t *testing.T) {
	rows := []model.BlastResultRow{
		{
			Protein:                             "Spipo1_T001",
			PercentIdentity:                     42,
			AlignLength:                         220,
			QueryLength:                         300,
			EValue:                              "1e-80",
			UniProtReferenceEnabled:             true,
			UniProtAccession:                    "A0A1",
			TargetUniProtCanonicalLengthPercent: "98.0",
			InterProReferenceEnabled:            true,
			InterProConservedRegionStatus:       "present",
		},
		{
			Protein:                             "Spipo2_T001",
			PercentIdentity:                     45,
			AlignLength:                         240,
			QueryLength:                         300,
			EValue:                              "1e-80",
			UniProtReferenceEnabled:             true,
			UniProtAccession:                    "A0A2",
			TargetUniProtCanonicalLengthPercent: "100.0",
			InterProReferenceEnabled:            true,
			InterProConservedRegionStatus:       "missing",
		},
		{
			Protein:                             "Spipo3_T001",
			PercentIdentity:                     25,
			AlignLength:                         280,
			QueryLength:                         300,
			EValue:                              "1e-80",
			UniProtReferenceEnabled:             true,
			UniProtAccession:                    "A0A3",
			TargetUniProtCanonicalLengthPercent: "100.0",
			InterProReferenceEnabled:            true,
			InterProConservedRegionStatus:       "partial",
		},
		{
			Protein:                             "Spipo4_T001",
			PercentIdentity:                     55,
			AlignLength:                         90,
			QueryLength:                         300,
			EValue:                              "1e-80",
			UniProtReferenceEnabled:             true,
			UniProtAccession:                    "A0A4",
			TargetUniProtCanonicalLengthPercent: "100.0",
			InterProReferenceEnabled:            true,
			InterProConservedRegionStatus:       "partial",
		},
		{
			Protein:                             "Spipo5_T001",
			PercentIdentity:                     60,
			AlignLength:                         260,
			QueryLength:                         300,
			EValue:                              "1e-80",
			UniProtReferenceEnabled:             true,
			UniProtAccession:                    "A0A5",
			TargetUniProtCanonicalLengthPercent: "40.0",
			InterProReferenceEnabled:            true,
			InterProConservedRegionStatus:       "present",
		},
	}
	got := defaultBlastFilterSuggestion(BlastFilterRequest{Rows: rows, Settings: model.DefaultBlastFilterSettings()})
	wantSelected := []bool{true, false, true, true, false}
	wantFlags := []bool{false, true, false, false, true}
	for i := range wantSelected {
		if got.Selected[i] != wantSelected[i] || got.Flags[i] != wantFlags[i] {
			t.Fatalf("row %d selected/flag = %v/%v, want %v/%v", i, got.Selected[i], got.Flags[i], wantSelected[i], wantFlags[i])
		}
	}
}

func TestDefaultBlastFilterSuggestionRebuildsSelection(t *testing.T) {
	rows := []model.BlastResultRow{{
		Protein:                             "Spipo1_T001",
		PercentIdentity:                     42,
		AlignLength:                         220,
		QueryLength:                         300,
		EValue:                              "1e-80",
		UniProtReferenceEnabled:             true,
		UniProtAccession:                    "A0A1",
		TargetUniProtCanonicalLengthPercent: "98.0",
		InterProReferenceEnabled:            true,
		InterProConservedRegionStatus:       "present",
	}}
	got := defaultBlastFilterSuggestion(BlastFilterRequest{
		Rows:     rows,
		Selected: []bool{false},
		Settings: model.DefaultBlastFilterSettings(),
	})
	if len(got.Selected) != 1 || !got.Selected[0] || got.Flags[0] {
		t.Fatalf("filter should restore its own keep recommendation, got selected=%v flags=%v", got.Selected, got.Flags)
	}
}

func TestDefaultBlastFilterSuggestionDoesNotUseBlastMetricsAsDefaultHardRules(t *testing.T) {
	rows := []model.BlastResultRow{{
		Protein:                             "Spipo1_T001",
		PercentIdentity:                     10,
		AlignLength:                         20,
		QueryLength:                         300,
		EValue:                              "1e-2",
		UniProtReferenceEnabled:             true,
		UniProtAccession:                    "A0A1",
		TargetUniProtCanonicalLengthPercent: "100.0",
		InterProReferenceEnabled:            true,
		InterProConservedRegionStatus:       "present",
	}}
	got := defaultBlastFilterSuggestion(BlastFilterRequest{Rows: rows, Settings: model.DefaultBlastFilterSettings()})
	if len(got.Selected) != 1 || !got.Selected[0] || got.Flags[0] {
		t.Fatalf("default should keep rows with length and conserved-region support even when BLAST metrics are weak, got selected=%v flags=%v", got.Selected, got.Flags)
	}
}

func TestBlastFilterSuggestionCanUseBlastMetricsAsOptionalHardRules(t *testing.T) {
	rows := []model.BlastResultRow{{
		Protein:                             "Spipo1_T001",
		PercentIdentity:                     80,
		AlignLength:                         280,
		QueryLength:                         300,
		EValue:                              "1e-5",
		UniProtReferenceEnabled:             true,
		UniProtAccession:                    "A0A1",
		TargetUniProtCanonicalLengthPercent: "100.0",
		InterProReferenceEnabled:            true,
		InterProConservedRegionStatus:       "present",
	}}
	settings := model.DefaultBlastFilterSettings()
	settings.MaxEValue = 1e-30
	got := defaultBlastFilterSuggestion(BlastFilterRequest{Rows: rows, Settings: settings})
	if len(got.Selected) != 1 || got.Selected[0] || !got.Flags[0] {
		t.Fatalf("weak E-value row should be removable when optional E-value hard rule is enabled, got selected=%v flags=%v", got.Selected, got.Flags)
	}
}

func TestDefaultBlastFilterSuggestionRequiresLengthRatioByDefault(t *testing.T) {
	rows := []model.BlastResultRow{{
		Protein:                       "Spipo1_T001",
		PercentIdentity:               80,
		AlignLength:                   280,
		QueryLength:                   300,
		EValue:                        "1e-80",
		UniProtReferenceEnabled:       true,
		UniProtAccession:              "A0A1",
		InterProReferenceEnabled:      true,
		InterProConservedRegionStatus: "present",
	}}
	got := defaultBlastFilterSuggestion(BlastFilterRequest{Rows: rows, Settings: model.DefaultBlastFilterSettings()})
	if len(got.Selected) != 1 || got.Selected[0] || !got.Flags[0] {
		t.Fatalf("missing length ratio should be suggested for removal, got selected=%v flags=%v", got.Selected, got.Flags)
	}
}

func TestDefaultBlastFilterSuggestionRequiresInterProEvidenceByDefault(t *testing.T) {
	rows := []model.BlastResultRow{{
		Protein:                             "Spipo1_T001",
		PercentIdentity:                     80,
		AlignLength:                         280,
		QueryLength:                         300,
		EValue:                              "1e-80",
		UniProtReferenceEnabled:             true,
		UniProtAccession:                    "A0A1",
		TargetUniProtCanonicalLengthPercent: "100.0",
		InterProReferenceEnabled:            true,
	}}
	got := defaultBlastFilterSuggestion(BlastFilterRequest{Rows: rows, Settings: model.DefaultBlastFilterSettings()})
	if len(got.Selected) != 1 || got.Selected[0] || !got.Flags[0] {
		t.Fatalf("missing InterPro conserved evidence should be suggested for removal, got selected=%v flags=%v", got.Selected, got.Flags)
	}
}

func TestDefaultBlastFilterSuggestionAllowsStrongBlastFallbackWithoutReferences(t *testing.T) {
	rows := []model.BlastResultRow{{
		Protein:                        "Spipo1_T001",
		PercentIdentity:                62,
		AlignLength:                    320,
		QueryLength:                    340,
		TargetLength:                   330,
		EValue:                         "1e-120",
		FamilyConsensusSupport:         3,
		FamilyConsensusSize:            4,
		FamilyConsensusCoveragePercent: "75",
	}}
	got := defaultBlastFilterSuggestion(BlastFilterRequest{Rows: rows, Settings: model.DefaultBlastFilterSettings()})
	if len(got.Selected) != 1 || !got.Selected[0] || got.Flags[0] {
		t.Fatalf("strong BLAST fallback row should be kept, got selected=%v flags=%v", got.Selected, got.Flags)
	}
}

func TestDefaultBlastFilterSuggestionRejectsStrongFallbackWithoutFamilyConsensus(t *testing.T) {
	rows := []model.BlastResultRow{{
		Protein:                        "Spipo1_T001",
		PercentIdentity:                62,
		AlignLength:                    320,
		QueryLength:                    340,
		TargetLength:                   330,
		EValue:                         "1e-120",
		FamilyConsensusSupport:         1,
		FamilyConsensusSize:            4,
		FamilyConsensusCoveragePercent: "25",
	}}
	settings := model.DefaultBlastFilterSettings()
	settings.RequireFamilyConsensusForStrongFallback = true
	got := defaultBlastFilterSuggestion(BlastFilterRequest{Rows: rows, Settings: settings})
	if len(got.Selected) != 1 || got.Selected[0] || !got.Flags[0] {
		t.Fatalf("strong fallback row without family consensus should be removed, got selected=%v flags=%v", got.Selected, got.Flags)
	}
}

func TestDefaultBlastFilterSuggestionStillRejectsWeakNoReferenceRows(t *testing.T) {
	rows := []model.BlastResultRow{{
		Protein:         "Spipo1_T001",
		PercentIdentity: 28,
		AlignLength:     120,
		QueryLength:     340,
		TargetLength:    330,
		EValue:          "1e-20",
	}}
	got := defaultBlastFilterSuggestion(BlastFilterRequest{Rows: rows, Settings: model.DefaultBlastFilterSettings()})
	if len(got.Selected) != 1 || got.Selected[0] || !got.Flags[0] {
		t.Fatalf("weak no-reference row should still be removed, got selected=%v flags=%v", got.Selected, got.Flags)
	}
}

func TestDefaultBlastFilterSuggestionKeepsBestIsoformPerTargetGene(t *testing.T) {
	rows := []model.BlastResultRow{
		{
			Protein:                             "Sp9509d006g001100_T002",
			PercentIdentity:                     80,
			AlignLength:                         280,
			QueryLength:                         300,
			EValue:                              "1e-90",
			UniProtReferenceEnabled:             true,
			UniProtAccession:                    "A0A1",
			TargetUniProtCanonicalLengthPercent: "100.0",
			InterProReferenceEnabled:            true,
			InterProConservedRegionStatus:       "present",
		},
		{
			Protein:                             "Sp9509d006g001100_T001",
			PercentIdentity:                     80,
			AlignLength:                         280,
			QueryLength:                         300,
			EValue:                              "1e-80",
			UniProtReferenceEnabled:             true,
			UniProtAccession:                    "A0A1",
			TargetUniProtCanonicalLengthPercent: "100.0",
			InterProReferenceEnabled:            true,
			InterProConservedRegionStatus:       "present",
		},
	}
	got := defaultBlastFilterSuggestion(BlastFilterRequest{Rows: rows, Settings: model.DefaultBlastFilterSettings()})
	if !got.Selected[0] || got.Flags[0] || got.Selected[1] || !got.Flags[1] {
		t.Fatalf("best isoform by reference evidence should be kept, got selected=%v flags=%v", got.Selected, got.Flags)
	}
}

func TestDefaultBlastFilterSuggestionProcessesRowsByRun(t *testing.T) {
	runA := []model.BlastResultRow{{
		Protein:                             "RunA1",
		PercentIdentity:                     80,
		AlignLength:                         280,
		QueryLength:                         300,
		EValue:                              "1e-90",
		UniProtReferenceEnabled:             true,
		UniProtAccession:                    "A0A1",
		TargetUniProtCanonicalLengthPercent: "100.0",
		InterProReferenceEnabled:            true,
		InterProConservedRegionStatus:       "present",
	}}
	runB := []model.BlastResultRow{{
		Protein:                             "RunB1",
		PercentIdentity:                     80,
		AlignLength:                         280,
		QueryLength:                         300,
		EValue:                              "1e-90",
		UniProtReferenceEnabled:             true,
		UniProtAccession:                    "A0A2",
		TargetUniProtCanonicalLengthPercent: "45.0",
		InterProReferenceEnabled:            true,
		InterProConservedRegionStatus:       "present",
	}}
	got := defaultBlastFilterSuggestion(BlastFilterRequest{
		RowsByRun:     [][]model.BlastResultRow{runA, runB},
		SelectedByRun: [][]bool{{true}, {true}},
		Settings:      model.DefaultBlastFilterSettings(),
		CurrentRun:    1,
	})
	if len(got.SelectedByRun) != 2 || len(got.FlagsByRun) != 2 {
		t.Fatalf("rows-by-run output lengths = %d/%d, want 2/2", len(got.SelectedByRun), len(got.FlagsByRun))
	}
	if !got.SelectedByRun[0][0] || got.FlagsByRun[0][0] {
		t.Fatalf("run A should be kept, got selected=%v flags=%v", got.SelectedByRun[0], got.FlagsByRun[0])
	}
	if got.SelectedByRun[1][0] || !got.FlagsByRun[1][0] {
		t.Fatalf("run B should be filtered, got selected=%v flags=%v", got.SelectedByRun[1], got.FlagsByRun[1])
	}
	if len(got.Selected) != 1 || got.Selected[0] || len(got.Flags) != 1 || !got.Flags[0] {
		t.Fatalf("current run projection wrong: selected=%v flags=%v", got.Selected, got.Flags)
	}
}

func indexOfString(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}

func TestBackNavigationSentinelRemainsStable(t *testing.T) {
	if ErrBackToQueryInput == nil {
		t.Fatal("ErrBackToQueryInput should remain a stable non-nil navigation sentinel")
	}
}

func BenchmarkDefaultBlastFilterSuggestionRowsByRun(b *testing.B) {
	settings := model.DefaultBlastFilterSettings()
	runs := make([][]model.BlastResultRow, 6)
	selectedByRun := make([][]bool, len(runs))
	for runIndex := range runs {
		rows := make([]model.BlastResultRow, 900)
		selected := make([]bool, len(rows))
		for i := range rows {
			selected[i] = true
			rows[i] = model.BlastResultRow{
				Protein:                             fmt.Sprintf("Sp9509d%06dg%06d_T%03d", runIndex, i/3, i%3+1),
				QueryID:                             fmt.Sprintf("query_%d", i%75),
				QueryLength:                         300 + i%31,
				AlignLength:                         210 + i%43,
				TargetLength:                        295 + i%37,
				PercentIdentity:                     65 + float64(i%25),
				Bitscore:                            200 + float64(i%50),
				EValue:                              fmt.Sprintf("1e-%d", 40+i%30),
				UniProtReferenceEnabled:             true,
				UniProtAccession:                    fmt.Sprintf("Q%05d", i%300),
				UniProtReviewed:                     []string{"reviewed", "unreviewed"}[i%2],
				TargetUniProtCanonicalLengthPercent: []string{"98.0", "105.0", "72.0", "135.0"}[i%4],
				InterProReferenceEnabled:            true,
				InterProConservedRegionStatus:       []string{"present", "partial", "missing", "uncertain"}[i%4],
				InterProCoveragePercent:             []string{"82.0", "67.0", "45.0", ""}[i%4],
			}
		}
		runs[runIndex] = rows
		selectedByRun[runIndex] = selected
	}
	request := BlastFilterRequest{
		RowsByRun:     runs,
		SelectedByRun: selectedByRun,
		Settings:      settings,
		CurrentRun:    0,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got := defaultBlastFilterSuggestion(request)
		if len(got.SelectedByRun) != len(runs) {
			b.Fatalf("unexpected run count: got %d want %d", len(got.SelectedByRun), len(runs))
		}
	}
}

// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package prompt

import (
	"strings"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/model"
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
	for _, want := range []string{"search_type: keyword", "label_name: C4H", "protein_id: prot123", "transcript: AT1G01010.1"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("keyword detail missing %q: %s", want, detail)
		}
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

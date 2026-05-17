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
	"slices"
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

func TestExportSettingsStructSupportsDefaultPhgoHeaderPreference(t *testing.T) {
	settings := ExportSettings{
		WriteText:     true,
		WriteExcel:    true,
		WriteRawExcel: false,
		UsePhgoHeader: true,
	}
	if !settings.UsePhgoHeader {
		t.Fatalf("expected phgo header preference to be enabled in export settings test fixture: %#v", settings)
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
	if len(pages) != 3 {
		t.Fatalf("detail pages = %d, want 3", len(pages))
	}
	if pages[0].Title != "Source" || pages[1].Title != "UniProt" || pages[2].Title != "FASTA" {
		t.Fatalf("unexpected page titles: %#v", pages)
	}
	if got := pages[1].Items[0].Value; got != "" {
		t.Fatalf("expected blank UniProt detail cell without accession, got %q", got)
	}
	if !pages[2].Items[0].AutoLoad {
		t.Fatal("expected blast FASTA page to auto-load")
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

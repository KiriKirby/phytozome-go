// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/ncbi"
	"github.com/KiriKirby/phytozome-go/internal/tui"
)

func TestNewMainInterfaceEnabled(t *testing.T) {
	if !newMainInterfaceEnabled() {
		t.Fatal("new main interface should be enabled in the current build")
	}
}

func TestMainInterfaceSpeciesKeyAndShortLabel(t *testing.T) {
	candidate := model.SpeciesCandidate{
		ProteomeID:  42,
		JBrowseName: "Sp42",
		GenomeLabel: "Arabidopsis thaliana",
		CommonName:  "thale cress",
		ReleaseDate: "TAIR10",
		SearchAlias: "A. thaliana",
	}
	if got := mainInterfaceSpeciesShortLabel(candidate); got != "Arabidopsis thaliana" {
		t.Fatalf("unexpected short label: %q", got)
	}
	key := mainInterfaceSpeciesKey(candidate)
	if key == "" || key == candidate.DisplayLabel() {
		t.Fatalf("species key should be stable and separate from display label, got %q", key)
	}
	if desc := mainInterfaceSpeciesDescription(candidate); desc == "" || !containsAll(desc, []string{"Sp42", "target 42", "thale cress", "TAIR10"}) {
		t.Fatalf("species description missing fields: %q", desc)
	}
	if search := mainInterfaceSpeciesSearchText(candidate); !containsAll(strings.ToLower(search), []string{"arabidopsis thaliana", "sp42", "42"}) {
		t.Fatalf("species search text missing fields: %q", search)
	}
}

func TestNCBIGeneLocusPrioritySearchesLocusThenSearchTerm(t *testing.T) {
	w := NewBlastWizard(nil)
	w.source = keywordMapSource{rowsByKeyword: map[string][]model.KeywordResultRow{
		"AT1G01010": {{SourceDatabase: "ncbi", GeneLocus: "AT1G01010", ProteinID: "NP_locus"}},
		"PAL1":      {{SourceDatabase: "ncbi", ProteinID: "NP_fallback"}},
	}}
	groups, err := w.searchMainKeywordGroupsWithGeneLocusPriorityProgress(
		context.Background(),
		model.SpeciesCandidate{GenomeLabel: "Arabidopsis thaliana"},
		[]string{"C4H", "PAL1"},
		[]string{"AT1G01010", "AT1G99999"},
		false,
		tui.GeneLocusPriorityNCBI,
		nil,
	)
	if err != nil {
		t.Fatalf("NCBI Gene locus priority search: %v", err)
	}
	if got := groups[0].Rows[0].SearchType; got != "NCBI Gene locus priority" {
		t.Fatalf("locus-hit search type = %q", got)
	}
	if got := groups[0].Rows[0].SearchTerm; got != "C4H" {
		t.Fatalf("locus-hit search term = %q, want original term C4H", got)
	}
	if got := groups[1].Rows[0].ProteinID; got != "NP_fallback" {
		t.Fatalf("fallback protein id = %q, want NP_fallback", got)
	}
	if got := groups[1].Rows[0].SearchType; got == "NCBI Gene locus priority" {
		t.Fatalf("fallback row must retain its ordinary NCBI search type: %#v", groups[1].Rows[0])
	}
}

func TestCYPRowSearchUsesSpeciesLabelNotCompositeSelectorKey(t *testing.T) {
	w := NewBlastWizard(nil)
	src := keywordMapSource{name: "cyp", rowsByKeyword: map[string][]model.KeywordResultRow{"CYP84A6": {{LabelName: "CYP84A6"}}}}
	key := mainInterfaceSpeciesKey(model.SpeciesCandidate{JBrowseName: "Oryza sativa", GenomeLabel: "Oryza sativa", ReleaseDate: "local"})
	groups, err := w.searchCYPKeywordRows(context.Background(), src, []tui.MainKeywordRow{{SearchTerm: "CYP84A6", SpeciesKey: key, SpeciesLabel: "Oryza sativa"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Rows) != 1 || groups[0].Rows[0].LabelName != "CYP84A6" {
		t.Fatalf("CYP row search did not resolve selected species: %#v", groups)
	}
	if got := groups[0].Rows[0].Genome; got != "Oryza sativa" {
		t.Fatalf("CYP connector received composite key instead of species label: %q", got)
	}
}

func TestCYPRowSearchUsesGeneLocusBeforeSearchTerm(t *testing.T) {
	w := NewBlastWizard(nil)
	src := keywordMapSource{name: "cyp", rowsByKeyword: map[string][]model.KeywordResultRow{
		"ACU45738.1": {{SourceDatabase: "cyp", LabelName: "CYP84A-like", GeneLocus: "ACU45738.1"}},
		"wrong-term": nil,
	}}
	groups, err := w.searchCYPKeywordRows(context.Background(), src, []tui.MainKeywordRow{{SearchTerm: "wrong-term", GeneLocus: "ACU45738.1", SpeciesLabel: "Eucalyptus globulus"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Rows) != 1 || groups[0].Rows[0].SearchType != "CYP Gene locus priority" {
		t.Fatalf("gene locus was not preferred: %#v", groups)
	}
	if groups[0].Rows[0].SearchTerm != "wrong-term" {
		t.Fatalf("original search term was not preserved: %#v", groups[0].Rows[0])
	}
}

func TestNCBIGeneLocusPrioritySkipKeepsOtherKeywordResults(t *testing.T) {
	w := NewBlastWizard(nil)
	w.source = keywordMapSource{rowsByKeyword: map[string][]model.KeywordResultRow{
		"PAL1": {{SourceDatabase: "ncbi", ProteinID: "NP_fallback"}},
	}}
	groups, err := w.searchMainKeywordGroupsWithGeneLocusPriorityProgressSkipping(
		context.Background(),
		model.SpeciesCandidate{GenomeLabel: "Arabidopsis thaliana"},
		[]string{"CYP84A1", "PAL1"},
		[]string{"", ""},
		false,
		tui.GeneLocusPriorityNCBI,
		map[int]bool{0: true},
		nil,
	)
	if err != nil {
		t.Fatalf("priority search after skip: %v", err)
	}
	if len(groups) != 2 || len(groups[0].Rows) != 0 {
		t.Fatalf("skipped group = %#v, want an empty preserved group", groups)
	}
	if len(groups[1].Rows) != 1 || groups[1].Rows[0].ProteinID != "NP_fallback" {
		t.Fatalf("later keyword result was not preserved: %#v", groups[1].Rows)
	}
}

func TestSetBlastQueryItemGeneLocusSeedsQuerySource(t *testing.T) {
	item := blastQueryItem{
		RawInput:  ">q1\nAAAA",
		LabelName: "PAL1",
		Sequence:  "AAAA",
		QuerySource: &model.QuerySequenceSource{
			TranscriptID: "AT2G37040.1",
			ProteinID:    "AT2G37040.1",
		},
	}
	setBlastQueryItemGeneLocus(&item, "AT2G37040")
	if item.QuerySource == nil {
		t.Fatal("expected query source")
	}
	if item.QuerySource.GeneID != "AT2G37040" {
		t.Fatalf("GeneID = %q, want AT2G37040", item.QuerySource.GeneID)
	}
	if got := blastQueryItemID2(item); got != "AT2G37040" {
		t.Fatalf("blastQueryItemID2 = %q, want AT2G37040", got)
	}
}

func TestBestMainBlastGeneLocusFromKeywordRowsPrefersGeneLocus(t *testing.T) {
	rows := []model.KeywordResultRow{{
		GeneLocus:      "AT2G37040",
		GeneIdentifier: "AT2G37040.1",
		TranscriptID:   "AT2G37040.1",
		ProteinID:      "AT2G37040.1",
	}}
	if got := bestMainBlastGeneLocusFromKeywordRows(rows); got != "AT2G37040" {
		t.Fatalf("gene locus = %q, want AT2G37040", got)
	}
}

func TestAutoIdentifyMainBlastGeneLocusUsesQuerySourceFallback(t *testing.T) {
	w := NewBlastWizard(nil)
	item := blastQueryItem{QuerySource: &model.QuerySequenceSource{TranscriptID: "AT2G37040.1"}}
	got := w.autoIdentifyMainBlastGeneLocus(nil, nil, model.SpeciesCandidate{}, item)
	if got != "AT2G37040" {
		t.Fatalf("gene locus = %q, want AT2G37040", got)
	}
}

func TestMainAutoIdentifyMissingCellsOnlyTreatsEmptyAsBlank(t *testing.T) {
	rows := []tui.MainKeywordRow{
		{SearchTerm: "PAL", SymbolName: ""},
		{SearchTerm: "C4H", SymbolName: "~"},
		{SearchTerm: "REF8", SymbolName: "~~"},
	}
	_, indexes := mainKeywordRowsForWorkflow(rows)
	missing := mainKeywordMissingSymbolIndexes(rows, indexes)
	if len(missing) != 1 || missing[0] != 0 {
		t.Fatalf("expected only the truly empty Symbol name cell to be auto-fillable, got %#v", missing)
	}
}

func TestMainApplyKeywordSymbolIdentificationsLeavesTildeCellsAloneAndFillsMissingAsDoubleTilde(t *testing.T) {
	state := tui.MainInterfaceState{Keyword: tui.MainKeywordState{Rows: []tui.MainKeywordRow{
		{SearchTerm: "PAL", SymbolName: ""},
		{SearchTerm: "C4H", SymbolName: "~"},
		{SearchTerm: "REF8", SymbolName: ""},
	}}}
	mainApplyKeywordSymbolIdentifications(&state, []int{0, 1, 2}, []int{0, 1, 2}, []keywordLabelIdentification{
		{Aliases: []string{"PAL1"}},
		{Aliases: []string{"C4H"}},
		{},
	})
	if state.Keyword.Rows[0].SymbolName != "PAL1" {
		t.Fatalf("row 0 SymbolName = %q, want PAL1", state.Keyword.Rows[0].SymbolName)
	}
	if strings.Join(state.Keyword.Rows[0].Aliases, ",") != "PAL1" {
		t.Fatalf("row 0 aliases = %#v, want PAL1", state.Keyword.Rows[0].Aliases)
	}
	if state.Keyword.Rows[1].SymbolName != "~" {
		t.Fatalf("row 1 SymbolName should preserve explicit tilde, got %q", state.Keyword.Rows[1].SymbolName)
	}
	if state.Keyword.Rows[2].SymbolName != "~~" {
		t.Fatalf("row 2 SymbolName = %q, want ~~ for no result", state.Keyword.Rows[2].SymbolName)
	}
}

func TestMainApplyBlastSymbolIdentificationsStoresAliasChoices(t *testing.T) {
	state := tui.MainInterfaceState{Blast: tui.MainBlastState{Rows: []tui.MainBlastRow{
		{FASTA: ">q1\nAAAA", SymbolName: ""},
	}}}
	items := []blastQueryItem{{
		LabelName: "PAL1",
		QuerySource: &model.QuerySequenceSource{
			LabelName:   "PAL1",
			PhgoAliases: "PAL1; PAL2",
		},
	}}
	mainApplyBlastSymbolIdentifications(&state, []int{0}, []int{0}, items)
	if state.Blast.Rows[0].SymbolName != "PAL1" {
		t.Fatalf("row 0 SymbolName = %q, want PAL1", state.Blast.Rows[0].SymbolName)
	}
	joined := strings.Join(state.Blast.Rows[0].Aliases, ",")
	if !containsAll(joined, []string{"PAL1", "PAL2"}) {
		t.Fatalf("row 0 aliases = %#v, want PAL1/PAL2", state.Blast.Rows[0].Aliases)
	}
}

func TestMainBlastMissingSymbolIndexesOnlyEmptyCells(t *testing.T) {
	rows := []tui.MainBlastRow{
		{FASTA: ">q1\nAAAA", SymbolName: ""},
		{FASTA: ">q2\nCCCC", SymbolName: "~"},
		{FASTA: ">q3\nGGGG", SymbolName: "~~"},
	}
	_, indexes := mainBlastRowsForWorkflow(rows)
	missing := mainBlastMissingSymbolIndexes(rows, indexes)
	if len(missing) != 1 || missing[0] != 0 {
		t.Fatalf("expected only the truly empty BLAST Symbol name cell to be auto-fillable, got %#v", missing)
	}
}

func TestMainInterfaceSelectedSpeciesUsesSyntheticNCBISearchTypeCandidate(t *testing.T) {
	w := NewBlastWizard(nil)
	src := ncbi.NewClient(nil)
	selected, err := w.mainInterfaceSelectedSpecies(context.Background(), src, ModeKeyword, "", "", "gene")
	if err != nil {
		t.Fatalf("mainInterfaceSelectedSpecies returned error: %v", err)
	}
	if got := ncbi.SearchTypeIDFromSpeciesCandidate(selected); got != "gene" {
		t.Fatalf("synthetic NCBI search type = %q, want gene", got)
	}
}

func containsAll(value string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

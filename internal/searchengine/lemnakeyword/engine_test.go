// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package lemnakeyword

import (
	"context"
	"strings"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

type fakeFinder struct {
	reportRows  map[string][]model.KeywordResultRow
	idRows      map[string][]model.KeywordResultRow
	labelRows   map[string][]model.KeywordResultRow
	keywordRows map[string][]model.KeywordResultRow
	wideRows    map[string][]model.KeywordResultRow
	broadRows   map[string][]model.KeywordResultRow
}

func (f *fakeFinder) SearchKeywordRowsByReportURL(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error) {
	return cloneRows(f.reportRows[strings.ToUpper(term)]), nil
}

func (f *fakeFinder) SearchKeywordRowsByIdentifier(ctx context.Context, species model.SpeciesCandidate, term string, kind string, limit int) ([]model.KeywordResultRow, error) {
	return cloneRows(f.idRows[strings.ToUpper(kind+"|"+term)]), nil
}

func (f *fakeFinder) SearchKeywordRowsByLabel(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error) {
	return cloneRows(f.labelRows[strings.ToUpper(term)]), nil
}

func (f *fakeFinder) SearchKeywordRowsByKeywordText(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error) {
	return cloneRows(f.keywordRows[strings.ToUpper(term)]), nil
}

func (f *fakeFinder) SearchKeywordRowsByWideText(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error) {
	return cloneRows(f.wideRows[strings.ToUpper(term)]), nil
}

func (f *fakeFinder) SearchKeywordRowsByBroadText(ctx context.Context, species model.SpeciesCandidate, term string, limit int) ([]model.KeywordResultRow, error) {
	return cloneRows(f.broadRows[strings.ToUpper(term)]), nil
}

func TestEngineMapsLemnaPrograms(t *testing.T) {
	finder := &fakeFinder{
		reportRows: map[string][]model.KeywordResultRow{
			"HTTPS://WWW.LEMNA.ORG/REPORT/SP_POLYRHIZA_9509/SP9509D020G000340": {{TranscriptID: "Sp9509d020g000340_T001", LabelName: "C4H"}},
		},
		idRows: map[string][]model.KeywordResultRow{
			"TRANSCRIPT|SP9509D020G000340_T001": {{TranscriptID: "Sp9509d020g000340_T001", LabelName: "C4H"}},
			"GENE|SP9509D020G000340":            {{GeneIdentifier: "Sp9509d020g000340", LabelName: "C4H"}},
			"ANY|PROT123":                       {{ProteinID: "prot123", LabelName: "C4H"}},
		},
		labelRows: map[string][]model.KeywordResultRow{
			"C4H":            {{TranscriptID: "Sp9509d020g000340_T001", LabelName: "C4H"}},
			"LOC_OS05G25640": {{TranscriptID: "Sp9509d020g000340_T001", LabelName: "C4H"}},
			"XP_015639656":   {{TranscriptID: "Sp9509d020g000340_T001", LabelName: "C4H"}},
			"OSC4H1":         {{TranscriptID: "Sp9509d020g000340_T001", LabelName: "C4H"}},
			"CYP73A38":       {{TranscriptID: "Sp9509d020g000340_T001", LabelName: "C4H"}},
		},
		keywordRows: map[string][]model.KeywordResultRow{
			"PHENYLALANINE AMMONIA LYASE": {{TranscriptID: "Sp9509d011g008180_T004", LabelName: "PAL1"}},
		},
	}
	engine := New(finder)
	species := model.SpeciesCandidate{JBrowseName: "Sp_polyrhiza_9509"}

	tests := []struct {
		term       string
		searchType string
		label      string
	}{
		{"https://www.lemna.org/report/Sp_polyrhiza_9509/Sp9509d020g000340", SearchTypeReportURL, "C4H"},
		{"Sp9509d020g000340_T001", SearchTypeTranscriptID, "C4H"},
		{"Sp9509d020g000340", SearchTypeGeneID, "C4H"},
		{"LOC_Os05g25640", SearchTypeRiceLocus, "C4H"},
		{"XP_015639656", SearchTypeRefSeqProtein, "C4H"},
		{"OsC4H1", SearchTypeGeneAlias, "C4H"},
		{"CYP73A38", SearchTypeCytochromeFamily, "C4H"},
		{"prot123", SearchTypeIdentifier, "C4H"},
		{"C4H", SearchTypeLabelSymbol, "C4H"},
		{"phenylalanine ammonia lyase", SearchTypeKeyword, "PAL1"},
	}
	for _, tt := range tests {
		rows, err := engine.SearchKeywordRows(context.Background(), species, tt.term)
		if err != nil {
			t.Fatalf("%s returned error: %v", tt.term, err)
		}
		if len(rows) != 1 {
			t.Fatalf("%s rows = %d, want 1", tt.term, len(rows))
		}
		if rows[0].SearchType != tt.searchType {
			t.Fatalf("%s search type = %q, want %q", tt.term, rows[0].SearchType, tt.searchType)
		}
		if rows[0].LabelName != tt.label {
			t.Fatalf("%s label = %q, want %q", tt.term, rows[0].LabelName, tt.label)
		}
	}
}

func TestEngineRecordsWideSearchFallback(t *testing.T) {
	finder := &fakeFinder{
		wideRows: map[string][]model.KeywordResultRow{
			"PHENYLALANINE BROAD": {{TranscriptID: "Sp9509d011g008180_T004", LabelName: "PAL1"}},
		},
	}
	engine := New(finder)

	rows, err := engine.SearchKeywordRows(context.Background(), model.SpeciesCandidate{ProteomeID: 2026051201, JBrowseName: "test-wide"}, "phenylalanine broad")
	if err != nil {
		t.Fatalf("SearchKeywordRows returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if !strings.Contains(rows[0].SearchType, "fallback to wide search") {
		t.Fatalf("search type should record wide fallback, got %q", rows[0].SearchType)
	}
}

func TestEngineCanForceWideSearch(t *testing.T) {
	finder := &fakeFinder{
		wideRows: map[string][]model.KeywordResultRow{
			"PHENYLALANINE BROAD": {{TranscriptID: "Sp9509d011g008180_T004", LabelName: "PAL1"}},
		},
	}
	engine := New(finder)

	rows, err := engine.SearchKeywordRowsWide(context.Background(), model.SpeciesCandidate{ProteomeID: 2026051201, JBrowseName: "test-wide"}, "phenylalanine broad")
	if err != nil {
		t.Fatalf("SearchKeywordRowsWide returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].SearchType != SearchTypeWide {
		t.Fatalf("forced wide search type = %q, want %q", rows[0].SearchType, SearchTypeWide)
	}
	if rows[0].LabelName != "PAL1" {
		t.Fatalf("forced wide search should preserve label name, got %q", rows[0].LabelName)
	}
}

func TestEngineWideSearchCanUseBroadRows(t *testing.T) {
	finder := &fakeFinder{
		broadRows: map[string][]model.KeywordResultRow{
			"4CL WEB STYLE": {{TranscriptID: "Sp9509d011g008180_T004", LabelName: "4CL"}},
		},
	}
	engine := New(finder)

	rows, err := engine.SearchKeywordRowsWide(context.Background(), model.SpeciesCandidate{ProteomeID: 2026051201, JBrowseName: "test-wide"}, "4cl web style")
	if err != nil {
		t.Fatalf("SearchKeywordRowsWide returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].SearchType != SearchTypeWide {
		t.Fatalf("forced wide search type = %q, want %q", rows[0].SearchType, SearchTypeWide)
	}
	if rows[0].LabelName != "4CL" {
		t.Fatalf("forced wide search should use broad rows, got %q", rows[0].LabelName)
	}
}

func TestEngineMatchesLemnaProgramsCaseInsensitive(t *testing.T) {
	finder := &fakeFinder{
		idRows: map[string][]model.KeywordResultRow{
			"TRANSCRIPT|SP9509D020G000340_T001": {{TranscriptID: "Sp9509d020g000340_T001", LabelName: "C4H"}},
		},
		labelRows: map[string][]model.KeywordResultRow{
			"LOC_OS05G25640": {{TranscriptID: "Sp9509d020g000340_T001", LabelName: "C4H"}},
			"XP_015639656":   {{TranscriptID: "Sp9509d020g000340_T001", LabelName: "C4H"}},
			"OSC4H1":         {{TranscriptID: "Sp9509d020g000340_T001", LabelName: "C4H"}},
			"CYP73A38":       {{TranscriptID: "Sp9509d020g000340_T001", LabelName: "C4H"}},
		},
	}
	engine := New(finder)

	tests := []struct {
		term       string
		searchType string
	}{
		{"sp9509d020g000340_t001", SearchTypeTranscriptID},
		{"loc_os05g25640", SearchTypeRiceLocus},
		{"xp_015639656", SearchTypeRefSeqProtein},
		{"osc4h1", SearchTypeGeneAlias},
		{"cyp73a38", SearchTypeCytochromeFamily},
	}
	for _, tt := range tests {
		rows, err := engine.SearchKeywordRows(context.Background(), model.SpeciesCandidate{ProteomeID: 2026051201, JBrowseName: "test-wide"}, tt.term)
		if err != nil {
			t.Fatalf("%s returned error: %v", tt.term, err)
		}
		if len(rows) != 1 {
			t.Fatalf("%s rows = %d, want 1", tt.term, len(rows))
		}
		if rows[0].SearchType != tt.searchType {
			t.Fatalf("%s search type = %q, want %q", tt.term, rows[0].SearchType, tt.searchType)
		}
	}
}

func TestEngineSupportsVersionedRiceRefSeqProteinAccessions(t *testing.T) {
	finder := &fakeFinder{
		labelRows: map[string][]model.KeywordResultRow{
			"XP_015650724.1": {{TranscriptID: "Sp9509d008g014760_T001", GeneIdentifier: "Sp9509d008g014760", LabelName: "Os4CL1"}},
			"XP_015624111.1": {{TranscriptID: "Sp9509d002g046970_T001", GeneIdentifier: "Sp9509d002g046970", LabelName: "Os4CL2"}},
			"XP_015625716.1": {{TranscriptID: "Sp9509d002g008100_T001", GeneIdentifier: "Sp9509d002g008100", LabelName: "Os4CL3"}},
			"XP_015643415.1": {{TranscriptID: "Sp9509d006g044620_T001", GeneIdentifier: "Sp9509d006g044620", LabelName: "Os4CL4"}},
			"XP_015650830.1": {{TranscriptID: "Sp9509d008g034790_T001", GeneIdentifier: "Sp9509d008g034790", LabelName: "Os4CL5"}},
		},
	}
	engine := New(finder)

	tests := []struct {
		term  string
		label string
	}{
		{"XP_015650724.1", "Os4CL1"},
		{"XP_015624111.1", "Os4CL2"},
		{"XP_015625716.1", "Os4CL3"},
		{"XP_015643415.1", "Os4CL4"},
		{"XP_015650830.1", "Os4CL5"},
	}
	for _, tt := range tests {
		rows, err := engine.SearchKeywordRows(context.Background(), model.SpeciesCandidate{ProteomeID: 2026051201, JBrowseName: "test-wide"}, tt.term)
		if err != nil {
			t.Fatalf("%s returned error: %v", tt.term, err)
		}
		if len(rows) != 1 {
			t.Fatalf("%s rows = %d, want 1", tt.term, len(rows))
		}
		if rows[0].SearchType != SearchTypeRefSeqProtein {
			t.Fatalf("%s search type = %q, want %q", tt.term, rows[0].SearchType, SearchTypeRefSeqProtein)
		}
		if rows[0].LabelName != tt.label {
			t.Fatalf("%s label = %q, want %q", tt.term, rows[0].LabelName, tt.label)
		}
	}
}

func TestEngineSupportsRice4CLAliasSeries(t *testing.T) {
	finder := &fakeFinder{
		labelRows: map[string][]model.KeywordResultRow{
			"OS4CL1": {{TranscriptID: "Sp9509d008g014760_T001", GeneIdentifier: "Sp9509d008g014760", LabelName: "Os4CL1"}},
			"OS4CL2": {{TranscriptID: "Sp9509d002g046970_T001", GeneIdentifier: "Sp9509d002g046970", LabelName: "Os4CL2"}},
			"OS4CL3": {{TranscriptID: "Sp9509d002g008100_T001", GeneIdentifier: "Sp9509d002g008100", LabelName: "Os4CL3"}},
			"OS4CL4": {{TranscriptID: "Sp9509d006g044620_T001", GeneIdentifier: "Sp9509d006g044620", LabelName: "Os4CL4"}},
			"OS4CL5": {{TranscriptID: "Sp9509d008g034790_T001", GeneIdentifier: "Sp9509d008g034790", LabelName: "Os4CL5"}},
		},
	}
	engine := New(finder)

	tests := []struct {
		term  string
		label string
	}{
		{"Os4CL1", "Os4CL1"},
		{"Os4CL2", "Os4CL2"},
		{"Os4CL3", "Os4CL3"},
		{"Os4CL4", "Os4CL4"},
		{"Os4CL5", "Os4CL5"},
	}
	for _, tt := range tests {
		rows, err := engine.SearchKeywordRows(context.Background(), model.SpeciesCandidate{ProteomeID: 2026051201, JBrowseName: "test-wide"}, tt.term)
		if err != nil {
			t.Fatalf("%s returned error: %v", tt.term, err)
		}
		if len(rows) != 1 {
			t.Fatalf("%s rows = %d, want 1", tt.term, len(rows))
		}
		if rows[0].SearchType != SearchTypeGeneAlias {
			t.Fatalf("%s search type = %q, want %q", tt.term, rows[0].SearchType, SearchTypeGeneAlias)
		}
		if rows[0].LabelName != tt.label {
			t.Fatalf("%s label = %q, want %q", tt.term, rows[0].LabelName, tt.label)
		}
	}
}

func TestEngineWideSearchPrefersSpecificLemnaProgramsBeforeKeyword(t *testing.T) {
	finder := &fakeFinder{
		labelRows: map[string][]model.KeywordResultRow{
			"OSC4H1": {{TranscriptID: "Sp9509d020g000340_T001", LabelName: "C4H"}},
		},
		keywordRows: map[string][]model.KeywordResultRow{
			"OSC4H1": {{TranscriptID: "keyword-only", LabelName: "KW"}},
		},
	}
	engine := New(finder)

	rows, err := engine.SearchKeywordRowsWide(context.Background(), model.SpeciesCandidate{ProteomeID: 2026051201, JBrowseName: "test-wide"}, "OsC4H1")
	if err != nil {
		t.Fatalf("SearchKeywordRowsWide returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].TranscriptID != "Sp9509d020g000340_T001" {
		t.Fatalf("wide search should prefer specific alias program, got %q", rows[0].TranscriptID)
	}
}

func TestEngineCanReturnCuratedRiceFallbackRowsWithReadableLabel(t *testing.T) {
	finder := &fakeFinder{
		keywordRows: map[string][]model.KeywordResultRow{
			"TRANS-CINNAMATE 4-MONOOXYGENASE": {
				{TranscriptID: "Sp9509d006g002010_T001", LabelName: "P48522", Description: "Trans-cinnamate 4-monooxygenase"},
			},
		},
	}
	engine := New(finder)

	rows, err := engine.SearchKeywordRows(context.Background(), model.SpeciesCandidate{ProteomeID: 2026051201, JBrowseName: "test-wide"}, "CYP73A38")
	if err != nil {
		t.Fatalf("SearchKeywordRows returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].SearchType != SearchTypeCytochromeFamily {
		t.Fatalf("search type = %q, want %q", rows[0].SearchType, SearchTypeCytochromeFamily)
	}
	if rows[0].LabelName != "C4H" {
		t.Fatalf("curated fallback should override raw accession label, got %q", rows[0].LabelName)
	}
}

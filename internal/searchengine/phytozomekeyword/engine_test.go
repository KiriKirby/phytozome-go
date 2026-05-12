// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package phytozomekeyword

import (
	"context"
	"strings"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

type fakeFinder struct {
	genesByID           map[string]GeneRecord
	genesByKeyword      map[string][]GeneRecord
	genesByBroadKeyword map[string][]GeneRecord
	keywordRequests     []string
}

func (f *fakeFinder) FetchGeneByGeneID(ctx context.Context, proteomeID int, geneID string) (GeneRecord, error) {
	if gene, ok := f.genesByID[strings.ToUpper(geneID)]; ok {
		return gene, nil
	}
	return GeneRecord{}, errNotFound{}
}

func (f *fakeFinder) FetchGeneByTranscript(ctx context.Context, proteomeID int, transcriptID string) (GeneRecord, error) {
	return f.FetchGeneByGeneID(ctx, proteomeID, transcriptID)
}

func (f *fakeFinder) FetchGeneByProtein(ctx context.Context, proteomeID int, proteinID string) (GeneRecord, error) {
	return f.FetchGeneByGeneID(ctx, proteomeID, proteinID)
}

func (f *fakeFinder) SearchGenesByKeyword(ctx context.Context, proteomeID int, keyword string, limit int) ([]GeneRecord, error) {
	f.keywordRequests = append(f.keywordRequests, keyword)
	rows := append([]GeneRecord(nil), f.genesByKeyword[strings.ToUpper(keyword)]...)
	return rows, nil
}

func (f *fakeFinder) SearchGenesByKeywordBroad(ctx context.Context, proteomeID int, keyword string, limit int) ([]GeneRecord, error) {
	rows := append([]GeneRecord(nil), f.genesByBroadKeyword[strings.ToUpper(keyword)]...)
	return rows, nil
}

type errNotFound struct{}

func (errNotFound) Error() string { return "not found" }

func TestEngineMapsRiceKeywordTypes(t *testing.T) {
	finder := &fakeFinder{genesByID: map[string]GeneRecord{
		"LOC_OS05G25640": testRiceGene("LOC_Os05g25640"),
		"LOC_OS01G60450": testRiceGene("LOC_Os01g60450"),
		"LOC_OS02G26770": testRiceGene("LOC_Os02g26770"),
		"LOC_OS02G26810": testRiceGene("LOC_Os02g26810"),
	}}
	engine := New(finder)
	species := model.SpeciesCandidate{ProteomeID: 323, JBrowseName: "Osativa_v7_0"}
	tests := []struct {
		term       string
		searchType string
		gene       string
	}{
		{"LOC_Os05g25640", SearchTypeRiceLocusID, "LOC_Os05g25640"},
		{"XP_015639656", SearchTypeRefSeqProtein, "LOC_Os05g25640"},
		{"XP_015635394", SearchTypeRefSeqProtein, "LOC_Os01g60450"},
		{"XP_015623447", SearchTypeRefSeqProtein, "LOC_Os02g26770"},
		{"XP_015626579", SearchTypeRefSeqProtein, "LOC_Os02g26810"},
		{"OsC4H1", SearchTypeRiceGeneAlias, "LOC_Os05g25640"},
		{"CYP73A35p", SearchTypeRiceGeneAlias, "LOC_Os01g60450"},
		{"OsC4H2a", SearchTypeRiceGeneAlias, "LOC_Os02g26770"},
		{"OsC4H2", SearchTypeRiceGeneAlias, "LOC_Os02g26810"},
		{"CYP73A38", SearchTypeCytochromeFamily, "LOC_Os05g25640"},
		{"CYP73A39", SearchTypeCytochromeFamily, "LOC_Os01g60450"},
		{"CYP73A40", SearchTypeCytochromeFamily, "LOC_Os02g26770"},
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
		if !strings.Contains(rows[0].GeneIdentifier, tt.gene) {
			t.Fatalf("%s gene = %q, want %s", tt.term, rows[0].GeneIdentifier, tt.gene)
		}
	}
}

func TestEngineRecordsWideSearchFallback(t *testing.T) {
	gene := testRiceGene("LOC_Os05g25640")
	finder := &fakeFinder{
		genesByID:      map[string]GeneRecord{},
		genesByKeyword: map[string][]GeneRecord{"OS-C4H-ODD": {gene}},
	}
	engine := New(finder)
	rows, err := engine.SearchKeywordRows(context.Background(), model.SpeciesCandidate{ProteomeID: 323}, "Os-C4H-odd")
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
	gene := testRiceGene("LOC_Os05g25640")
	finder := &fakeFinder{
		genesByKeyword: map[string][]GeneRecord{
			"WIDE ONLY PHRASE 20260509": {gene},
		},
	}
	engine := New(finder)

	rows, err := engine.SearchKeywordRowsWide(context.Background(), model.SpeciesCandidate{ProteomeID: 99323}, "wide only phrase 20260509")
	if err != nil {
		t.Fatalf("SearchKeywordRowsWide returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].SearchType != SearchTypeWide {
		t.Fatalf("forced wide search type = %q, want %q", rows[0].SearchType, SearchTypeWide)
	}
	if rows[0].GeneIdentifier != "LOC_Os05g25640" {
		t.Fatalf("forced wide search should use wide keyword result, got %q", rows[0].GeneIdentifier)
	}
}

func TestEngineWideSearchUsesBroadKeywordFinder(t *testing.T) {
	gene := testRiceGene("LOC_Os01g60450")
	finder := &fakeFinder{
		genesByBroadKeyword: map[string][]GeneRecord{
			"4CL WEB STYLE": {gene},
		},
	}
	engine := New(finder)

	rows, err := engine.SearchKeywordRowsWide(context.Background(), model.SpeciesCandidate{ProteomeID: 323}, "4cl web style")
	if err != nil {
		t.Fatalf("SearchKeywordRowsWide returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].SearchType != SearchTypeWide {
		t.Fatalf("forced wide search type = %q, want %q", rows[0].SearchType, SearchTypeWide)
	}
	if rows[0].GeneIdentifier != "LOC_Os01g60450" {
		t.Fatalf("wide search should use broad keyword result, got %q", rows[0].GeneIdentifier)
	}
}

func TestEngineMatchesPhytozomeProgramsCaseInsensitive(t *testing.T) {
	finder := &fakeFinder{genesByID: map[string]GeneRecord{
		"LOC_OS05G25640": testRiceGene("LOC_Os05g25640"),
	}}
	engine := New(finder)

	tests := []struct {
		term       string
		searchType string
	}{
		{"loc_os05g25640", SearchTypeRiceLocusID},
		{"xp_015639656", SearchTypeRefSeqProtein},
		{"osc4h1", SearchTypeRiceGeneAlias},
		{"cyp73a38", SearchTypeCytochromeFamily},
	}
	for _, tt := range tests {
		rows, err := engine.SearchKeywordRows(context.Background(), model.SpeciesCandidate{ProteomeID: 323}, tt.term)
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
	finder := &fakeFinder{genesByID: map[string]GeneRecord{
		"LOC_OS08G14760": testRiceGene("LOC_Os08g14760"),
		"LOC_OS02G46970": testRiceGene("LOC_Os02g46970"),
		"LOC_OS02G08100": testRiceGene("LOC_Os02g08100"),
		"LOC_OS06G44620": testRiceGene("LOC_Os06g44620"),
		"LOC_OS08G34790": testRiceGene("LOC_Os08g34790"),
	}}
	engine := New(finder)
	species := model.SpeciesCandidate{ProteomeID: 323, JBrowseName: "Osativa_v7_0"}

	tests := []struct {
		term string
		gene string
	}{
		{"XP_015650724.1", "LOC_Os08g14760"},
		{"XP_015624111.1", "LOC_Os02g46970"},
		{"XP_015625716.1", "LOC_Os02g08100"},
		{"XP_015643415.1", "LOC_Os06g44620"},
		{"XP_015650830.1", "LOC_Os08g34790"},
	}
	for _, tt := range tests {
		rows, err := engine.SearchKeywordRows(context.Background(), species, tt.term)
		if err != nil {
			t.Fatalf("%s returned error: %v", tt.term, err)
		}
		if len(rows) != 1 {
			t.Fatalf("%s rows = %d, want 1", tt.term, len(rows))
		}
		if rows[0].SearchType != SearchTypeRefSeqProtein {
			t.Fatalf("%s search type = %q, want %q", tt.term, rows[0].SearchType, SearchTypeRefSeqProtein)
		}
		if rows[0].GeneIdentifier != tt.gene {
			t.Fatalf("%s gene = %q, want %q", tt.term, rows[0].GeneIdentifier, tt.gene)
		}
	}
}

func TestEngineSupportsRice4CLAliasSeries(t *testing.T) {
	finder := &fakeFinder{genesByID: map[string]GeneRecord{
		"LOC_OS08G14760": testRiceGene("LOC_Os08g14760"),
		"LOC_OS02G46970": testRiceGene("LOC_Os02g46970"),
		"LOC_OS02G08100": testRiceGene("LOC_Os02g08100"),
		"LOC_OS06G44620": testRiceGene("LOC_Os06g44620"),
		"LOC_OS08G34790": testRiceGene("LOC_Os08g34790"),
	}}
	engine := New(finder)
	species := model.SpeciesCandidate{ProteomeID: 323, JBrowseName: "Osativa_v7_0"}

	tests := []struct {
		term string
		gene string
	}{
		{"Os4CL1", "LOC_Os08g14760"},
		{"Os4CL2", "LOC_Os02g46970"},
		{"Os4CL3", "LOC_Os02g08100"},
		{"Os4CL4", "LOC_Os06g44620"},
		{"Os4CL5", "LOC_Os08g34790"},
	}
	for _, tt := range tests {
		rows, err := engine.SearchKeywordRows(context.Background(), species, tt.term)
		if err != nil {
			t.Fatalf("%s returned error: %v", tt.term, err)
		}
		if len(rows) != 1 {
			t.Fatalf("%s rows = %d, want 1", tt.term, len(rows))
		}
		if rows[0].SearchType != SearchTypeRiceGeneAlias {
			t.Fatalf("%s search type = %q, want %q", tt.term, rows[0].SearchType, SearchTypeRiceGeneAlias)
		}
		if rows[0].GeneIdentifier != tt.gene {
			t.Fatalf("%s gene = %q, want %q", tt.term, rows[0].GeneIdentifier, tt.gene)
		}
	}
}

func TestEngineWideSearchPrefersSpecificPhytozomeProgramsBeforeKeyword(t *testing.T) {
	gene := testRiceGene("LOC_Os05g25640")
	finder := &fakeFinder{
		genesByID: map[string]GeneRecord{
			"LOC_OS05G25640": gene,
		},
		genesByKeyword: map[string][]GeneRecord{
			"OSC4H1": {testRiceGene("LOC_Os01g60450")},
		},
	}
	engine := New(finder)

	rows, err := engine.SearchKeywordRowsWide(context.Background(), model.SpeciesCandidate{ProteomeID: 323}, "OsC4H1")
	if err != nil {
		t.Fatalf("SearchKeywordRowsWide returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].GeneIdentifier != "LOC_Os05g25640" {
		t.Fatalf("wide search should prefer specific alias program, got %q", rows[0].GeneIdentifier)
	}
}

func testRiceGene(id string) GeneRecord {
	return GeneRecord{
		PrimaryIdentifier: id,
		Organism: GeneOrganismInfo{
			OrganismName:      "Oryza sativa",
			AnnotationVersion: "v7.0",
			Proteome:          323,
		},
		Transcripts: []GeneTranscript{{
			PrimaryIdentifier:   id + ".1",
			SecondaryIdentifier: "PAC:1",
			IsPrimary:           "1",
		}},
		Deflines: []string{"cytochrome P450, putative, expressed"},
	}
}

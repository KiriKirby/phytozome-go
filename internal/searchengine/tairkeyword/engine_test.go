// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package tairkeyword

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
	familyRows  map[string][]model.KeywordResultRow
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

func (f *fakeFinder) SearchKeywordRowsByFamily(ctx context.Context, species model.SpeciesCandidate, family string, limit int) ([]model.KeywordResultRow, error) {
	return cloneRows(f.familyRows[strings.ToUpper(family)]), nil
}

func TestEngineMapsTAIRPrograms(t *testing.T) {
	finder := &fakeFinder{
		reportRows: map[string][]model.KeywordResultRow{
			"HTTPS://WWW.ARABIDOPSIS.ORG/SERVLETS/TAIROBJECT?TYPE=LOCUS&NAME=AT1G01010": {{GeneIdentifier: "AT1G01010", LabelName: "NAC001"}},
		},
		idRows: map[string][]model.KeywordResultRow{
			"GENE|AT1G01010":   {{GeneIdentifier: "AT1G01010", LabelName: "NAC001"}},
			"MODEL|AT1G01010.1": {{TranscriptID: "AT1G01010.1", GeneIdentifier: "AT1G01010", LabelName: "NAC001"}},
			"ANY|AT1G01010-P1": {{ProteinID: "AT1G01010-P1", GeneIdentifier: "AT1G01010", LabelName: "NAC001"}},
		},
		labelRows: map[string][]model.KeywordResultRow{
			"NAC001": {{GeneIdentifier: "AT1G01010", LabelName: "NAC001"}},
		},
		keywordRows: map[string][]model.KeywordResultRow{
			"NAC DOMAIN TRANSCRIPTION FACTOR": {{GeneIdentifier: "AT1G01010", LabelName: "NAC001"}},
		},
	}
	engine := New(finder)
	species := model.SpeciesCandidate{JBrowseName: "Araport11"}

	tests := []struct {
		term       string
		searchType string
		label      string
	}{
		{"https://www.arabidopsis.org/servlets/TairObject?type=locus&name=AT1G01010", SearchTypeReportURL, "NAC001"},
		{"AT1G01010", SearchTypeLocusID, "NAC001"},
		{"AT1G01010.1", SearchTypeGeneModelID, "NAC001"},
		{"AT1G01010-P1", SearchTypeIdentifier, "NAC001"},
		{"NAC001", SearchTypeLabelSymbol, "NAC001"},
		{"nac domain transcription factor", SearchTypeKeyword, "NAC001"},
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
			"NAC RARE WIDE ONLY": {{GeneIdentifier: "AT1G01010", LabelName: "NAC001"}},
		},
	}
	engine := New(finder)
	rows, err := engine.SearchKeywordRows(context.Background(), model.SpeciesCandidate{ProteomeID: 1, JBrowseName: "Araport11"}, "nac rare wide only")
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

func TestEngineCanForceWideAndBroadSearch(t *testing.T) {
	finder := &fakeFinder{
		broadRows: map[string][]model.KeywordResultRow{
			"C4H WEB STYLE": {{GeneIdentifier: "AT2G30490", LabelName: "C4H"}},
		},
	}
	engine := New(finder)
	rows, err := engine.SearchKeywordRowsWide(context.Background(), model.SpeciesCandidate{ProteomeID: 1, JBrowseName: "Araport11"}, "c4h web style")
	if err != nil {
		t.Fatalf("SearchKeywordRowsWide returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].SearchType != SearchTypeWide {
		t.Fatalf("forced wide search type = %q, want %q", rows[0].SearchType, SearchTypeWide)
	}
	rows, err = engine.SearchKeywordRowsBroad(context.Background(), model.SpeciesCandidate{ProteomeID: 1, JBrowseName: "Araport11"}, "c4h web style")
	if err != nil {
		t.Fatalf("SearchKeywordRowsBroad returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("broad rows = %d, want 1", len(rows))
	}
	if rows[0].SearchType != SearchTypeBroad {
		t.Fatalf("forced broad search type = %q, want %q", rows[0].SearchType, SearchTypeBroad)
	}
}

func TestEngineSupportsFamilySearch(t *testing.T) {
	finder := &fakeFinder{
		familyRows: map[string][]model.KeywordResultRow{
			"NAC FAMILY": {
				{GeneIdentifier: "AT1G01010", LabelName: "NAC001"},
				{GeneIdentifier: "AT1G01720", LabelName: "NAC002"},
			},
		},
	}
	engine := New(finder)
	rows, err := engine.SearchFamilyKeywordRows(context.Background(), model.SpeciesCandidate{ProteomeID: 1, JBrowseName: "Araport11"}, "NAC family")
	if err != nil {
		t.Fatalf("SearchFamilyKeywordRows returned error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].SearchType != SearchTypeFamily {
		t.Fatalf("search type = %q, want %q", rows[0].SearchType, SearchTypeFamily)
	}
}

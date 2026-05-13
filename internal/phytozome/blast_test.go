// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package phytozome

import (
	"context"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

func TestSpecificIdentifierVariants(t *testing.T) {
	got := specificIdentifierVariants("At2g37040")
	want := []string{"At2g37040", "AT2G37040", "at2g37040"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected variants: got %v want %v", got, want)
	}
}

func TestSpecificIdentifierVariantsDeduplicates(t *testing.T) {
	got := specificIdentifierVariants("AT2G37040")
	want := []string{"AT2G37040", "at2g37040"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected variants: got %v want %v", got, want)
	}
}

func TestApplyPhytozomeQueryLabelsKeepsRawAliasFields(t *testing.T) {
	var source model.QuerySequenceSource
	applyPhytozomeQueryLabels(&source, geneRecord{
		Symbols:  []string{"PAL4", "PAL4"},
		Synonyms: []string{"ATPAL4"},
	})
	if source.LabelName != "" {
		t.Fatalf("unexpected precomputed label: %q", source.LabelName)
	}
	if source.Aliases != "" {
		t.Fatalf("unexpected aliases: %q", source.Aliases)
	}
	if source.Symbols != "PAL4" {
		t.Fatalf("unexpected symbols: %q", source.Symbols)
	}
	if source.Synonyms != "ATPAL4" {
		t.Fatalf("unexpected synonyms: %q", source.Synonyms)
	}
}

func TestPhytozomeGeneReportKeywordParsesURL(t *testing.T) {
	reportType, identifier, ok := phytozomeGeneReportKeyword("https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT3G10340")
	if !ok {
		t.Fatal("expected Phytozome gene report URL to parse")
	}
	if reportType != "gene" || identifier != "AT3G10340" {
		t.Fatalf("unexpected parsed values: reportType=%q identifier=%q", reportType, identifier)
	}
}

func TestPhytozomeGeneReportKeywordParsesURLWithoutScheme(t *testing.T) {
	reportType, identifier, ok := phytozomeGeneReportKeyword("phytozome-next.jgi.doe.gov/report/transcript/Athaliana_TAIR10/AT3G10340.1")
	if !ok {
		t.Fatal("expected Phytozome transcript report URL without scheme to parse")
	}
	if reportType != "transcript" || identifier != "AT3G10340.1" {
		t.Fatalf("unexpected parsed values: reportType=%q identifier=%q", reportType, identifier)
	}
}

func TestPhytozomeGeneReportKeywordParsesProteinURL(t *testing.T) {
	reportType, identifier, ok := phytozomeGeneReportKeyword("https://phytozome-next.jgi.doe.gov/report/protein/S_polyrhiza_v2/Spipo15G0028500")
	if !ok {
		t.Fatal("expected Phytozome protein report URL to parse")
	}
	if reportType != "protein" || identifier != "Spipo15G0028500" {
		t.Fatalf("unexpected parsed values: reportType=%q identifier=%q", reportType, identifier)
	}
}

func TestPhytozomeKeywordReplayLiveBySearchType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-backed phytozome keyword replay in short mode")
	}
	if os.Getenv("PHYTOZOME_LIVE_REPLAY") == "" {
		t.Skip("set PHYTOZOME_LIVE_REPLAY=1 to run live phytozome keyword replay")
	}

	client := NewClient(http.DefaultClient)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	species := model.SpeciesCandidate{
		ProteomeID:  323,
		JBrowseName: "Osativa_v7_0",
		GenomeLabel: "Oryza sativa v7.0",
	}

	tests := []struct {
		name           string
		term           string
		wantSearchType string
		minRows        int
	}{
		{
			name:           "report-url-gene",
			term:           "https://phytozome-next.jgi.doe.gov/report/gene/Osativa_v7_0/LOC_Os05g25640",
			wantSearchType: "report URL",
			minRows:        1,
		},
		{
			name:           "rice-locus",
			term:           "LOC_Os05g25640",
			wantSearchType: "rice LOC_Os locus",
			minRows:        1,
		},
		{
			name:           "rice-locus-case-insensitive",
			term:           "loc_os05g25640",
			wantSearchType: "rice LOC_Os locus",
			minRows:        1,
		},
		{
			name:           "refseq",
			term:           "XP_015639656",
			wantSearchType: "RefSeq XP protein",
			minRows:        1,
		},
		{
			name:           "refseq-case-insensitive",
			term:           "xp_015639656",
			wantSearchType: "RefSeq XP protein",
			minRows:        1,
		},
		{
			name:           "rice-alias",
			term:           "OsC4H1",
			wantSearchType: "rice gene alias",
			minRows:        1,
		},
		{
			name:           "rice-alias-case-insensitive",
			term:           "osc4h1",
			wantSearchType: "rice gene alias",
			minRows:        1,
		},
		{
			name:           "cytochrome-family",
			term:           "CYP73A38",
			wantSearchType: "CYP73 family symbol",
			minRows:        1,
		},
		{
			name:           "cytochrome-family-case-insensitive",
			term:           "cyp73a38",
			wantSearchType: "CYP73 family symbol",
			minRows:        1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			rows, err := client.SearchKeywordRows(ctx, species, tt.term)
			if err != nil {
				t.Fatalf("SearchKeywordRows(%q): %v", tt.term, err)
			}
			if len(rows) < tt.minRows {
				t.Fatalf("SearchKeywordRows(%q) rows=%d, want >= %d", tt.term, len(rows), tt.minRows)
			}
			if rows[0].SearchType != tt.wantSearchType {
				t.Fatalf("SearchKeywordRows(%q) searchType=%q, want %q", tt.term, rows[0].SearchType, tt.wantSearchType)
			}
		})
	}
}

func TestPhytozomeKeywordWideReplayLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-backed phytozome wide replay in short mode")
	}
	if os.Getenv("PHYTOZOME_LIVE_REPLAY") == "" {
		t.Skip("set PHYTOZOME_LIVE_REPLAY=1 to run live phytozome wide replay")
	}

	client := NewClient(http.DefaultClient)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	species := model.SpeciesCandidate{
		ProteomeID:  323,
		JBrowseName: "Osativa_v7_0",
		GenomeLabel: "Oryza sativa v7.0",
	}

	rows, err := client.SearchKeywordRowsWide(ctx, species, "4cl web style")
	if err != nil {
		t.Fatalf("SearchKeywordRowsWide: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("SearchKeywordRowsWide returned no rows")
	}
	if rows[0].SearchType != "wide search" {
		t.Fatalf("wide search type = %q, want wide search", rows[0].SearchType)
	}

	mixed, err := client.SearchKeywordRows(ctx, species, "4cl web style")
	if err != nil {
		t.Fatalf("SearchKeywordRows mixed wide case: %v", err)
	}
	if len(mixed) == 0 {
		t.Fatal("SearchKeywordRows returned no wide-fallback rows")
	}
	if !strings.Contains(strings.ToLower(mixed[0].SearchType), "wide search") && mixed[0].SearchType != "keyword" {
		t.Fatalf("mixed wide replay should be keyword or explicit wide-related type, got %q", mixed[0].SearchType)
	}
}

func TestPhytozomeKeywordReplayLiveRice4CLMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-backed phytozome 4CL replay in short mode")
	}
	if os.Getenv("PHYTOZOME_LIVE_REPLAY") == "" {
		t.Skip("set PHYTOZOME_LIVE_REPLAY=1 to run live phytozome 4CL replay")
	}

	client := NewClient(http.DefaultClient)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	species := model.SpeciesCandidate{
		ProteomeID:  323,
		JBrowseName: "Osativa_v7_0",
		GenomeLabel: "Oryza sativa v7.0",
	}

	tests := []struct {
		name           string
		term           string
		wantSearchType string
		wantGenePrefix string
	}{
		{"alias-1", "Os4CL1", "rice gene alias", "LOC_Os08g14760"},
		{"alias-1-lower", "os4cl1", "rice gene alias", "LOC_Os08g14760"},
		{"alias-2", "Os4CL2", "rice gene alias", "LOC_Os02g46970"},
		{"alias-3", "Os4CL3", "rice gene alias", "LOC_Os02g08100"},
		{"alias-4", "Os4CL4", "rice gene alias", "LOC_Os06g44620"},
		{"alias-5", "Os4CL5", "rice gene alias", "LOC_Os08g34790"},
		{"locus-1", "Os08g14760.1", "rice LOC_Os locus", "LOC_Os08g14760"},
		{"locus-1-lower", "os08g14760.1", "rice LOC_Os locus", "LOC_Os08g14760"},
		{"locus-2", "Os02g46970.1", "rice LOC_Os locus", "LOC_Os02g46970"},
		{"locus-3", "Os02g08100.1", "rice LOC_Os locus", "LOC_Os02g08100"},
		{"locus-4", "Os06g44620.1", "rice LOC_Os locus", "LOC_Os06g44620"},
		{"locus-5", "Os08g34790.1", "rice LOC_Os locus", "LOC_Os08g34790"},
		{"xp-1", "XP_015650724.1", "RefSeq XP protein", "LOC_Os08g14760"},
		{"xp-1-lower", "xp_015650724.1", "RefSeq XP protein", "LOC_Os08g14760"},
		{"xp-2", "XP_015624111.1", "RefSeq XP protein", "LOC_Os02g46970"},
		{"xp-3", "XP_015625716.1", "RefSeq XP protein", "LOC_Os02g08100"},
		{"xp-4", "XP_015643415.1", "RefSeq XP protein", "LOC_Os06g44620"},
		{"xp-5", "XP_015650830.1", "RefSeq XP protein", "LOC_Os08g34790"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			rows, err := client.SearchKeywordRows(ctx, species, tt.term)
			if err != nil {
				t.Fatalf("SearchKeywordRows(%q): %v", tt.term, err)
			}
			if len(rows) == 0 {
				t.Fatalf("SearchKeywordRows(%q) returned no rows", tt.term)
			}
			if rows[0].SearchType != tt.wantSearchType {
				t.Fatalf("SearchKeywordRows(%q) searchType=%q, want %q", tt.term, rows[0].SearchType, tt.wantSearchType)
			}
			if !strings.Contains(rows[0].GeneIdentifier, tt.wantGenePrefix) {
				t.Fatalf("SearchKeywordRows(%q) gene=%q, want prefix %q", tt.term, rows[0].GeneIdentifier, tt.wantGenePrefix)
			}
		})
	}
}

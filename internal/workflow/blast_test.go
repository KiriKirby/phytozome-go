// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/interpro"
	"github.com/KiriKirby/phytozome-go/internal/labelname"
	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/phylo"
	"github.com/KiriKirby/phytozome-go/internal/prompt"
	"github.com/KiriKirby/phytozome-go/internal/report"
	"github.com/KiriKirby/phytozome-go/internal/sessionsnapshot"
	"github.com/KiriKirby/phytozome-go/internal/source"
	"github.com/KiriKirby/phytozome-go/internal/tair"
	"github.com/KiriKirby/phytozome-go/internal/tui"
	"github.com/KiriKirby/phytozome-go/internal/uniprot"
)

func TestParseBlastQueryItemsIgnoresPhgoNoteHeader(t *testing.T) {
	items, err := parseBlastQueryItems(strings.Join([]string{
		">phgo://note",
		"MNOTE",
		">phgo://Sp7498/PAL1/AT2G37040\\1",
		"MPEPTIDE",
	}, "\n"))
	if err != nil {
		t.Fatalf("parseBlastQueryItems returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("parseBlastQueryItems returned %d items, want 1", len(items))
	}
	if items[0].Sequence != "MPEPTIDE" {
		t.Fatalf("unexpected remaining item: %#v", items[0])
	}
}

func TestNormalizeGeneReportURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{
			input: "phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
			want:  "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
			ok:    true,
		},
		{
			input: "http://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490?x=1#frag",
			want:  "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
			ok:    true,
		},
		{
			input: "https://example.com/report/gene/Athaliana_TAIR10/AT2G30490",
			ok:    false,
		},
		{
			input: "https://phytozome-next.jgi.doe.gov/report/protein/S_polyrhiza_v2/Spipo15G0028500",
			want:  "https://phytozome-next.jgi.doe.gov/report/protein/S_polyrhiza_v2/Spipo15G0028500",
			ok:    true,
		},
		{
			input: "https://lemna.org/jbrowse2/?assembly=Sp9509d&config=https%3A%2F%2Fwww.lemna.org%2Fjbrowse2%2Fconfig.json&filter=Sp9509d020g000340&highlight=Sp9509d020g000340&phgo_gene=Sp9509d020g000340&phgo_root=Sp_polyrhiza_9509",
			want:  "https://www.lemna.org/jbrowse2/?assembly=Sp9509d&config=https%3A%2F%2Fwww.lemna.org%2Fjbrowse2%2Fconfig.json&filter=Sp9509d020g000340&highlight=Sp9509d020g000340&phgo_gene=Sp9509d020g000340&phgo_root=Sp_polyrhiza_9509",
			ok:    true,
		},
	}

	for _, tc := range tests {
		got, ok := normalizeGeneReportURL(tc.input)
		if ok != tc.ok {
			t.Fatalf("normalizeGeneReportURL(%q) ok=%v want %v", tc.input, ok, tc.ok)
		}
		if got != tc.want {
			t.Fatalf("normalizeGeneReportURL(%q)=%q want %q", tc.input, got, tc.want)
		}
	}
}

func TestPrependQuerySequenceRecordPreservesProvidedLabel(t *testing.T) {
	source := &model.QuerySequenceSource{
		OrganismShort: "A.thaliana",
		Annotation:    "TAIR10",
		ProteinID:     "AT2G37040.1",
		Sequence:      "MSTN",
	}
	records := prependQuerySequenceRecord(nil, source, "ATPAL1")
	if len(records) != 1 {
		t.Fatalf("expected one query record, got %d", len(records))
	}
	if !strings.Contains(records[0].Header, "(ATPAL1)") {
		t.Fatalf("query header did not preserve provided label: %q", records[0].Header)
	}
}

func TestBuildBlastOutputDisplayNamePreservesLabel(t *testing.T) {
	item := blastQueryItem{LabelName: "AtCESA4"}
	if got := buildBlastOutputDisplayName(item); got != "AtCESA4" {
		t.Fatalf("unexpected display label: %q", got)
	}
}

func TestBuildBlastOutputDisplayNameDoesNotNormalizeArabidopsisLabel(t *testing.T) {
	item := blastQueryItem{LabelName: "ATPAL1"}
	if got := buildBlastOutputDisplayName(item); got != "ATPAL1" {
		t.Fatalf("unexpected display label: %q", got)
	}
}

func TestExportSettingsFromPromptKeepsFileTypeToggles(t *testing.T) {
	settings := exportSettingsFromPrompt(prompt.ExportSettings{
		WriteReport:           true,
		WriteText:             true,
		WriteExcel:            false,
		WriteRawExcel:         true,
		FastaHeaderMode:       model.FastaHeaderModePhgo,
		UsePhgoHeader:         true,
		PrependOnlyFirstQuery: true,
	}, "C4H", "out")

	if settings.BaseName != "C4H" || settings.OutputDir != "out" || !settings.WriteReport {
		t.Fatalf("basic export settings not preserved: %#v", settings)
	}
	if !settings.WriteText || settings.WriteExcel || !settings.WriteRawExcel {
		t.Fatalf("file type toggles not preserved: %#v", settings)
	}
	if !settings.UsePhgoHeader {
		t.Fatalf("phgo header toggle not preserved: %#v", settings)
	}
	if settings.FastaHeaderMode != model.FastaHeaderModePhgo {
		t.Fatalf("FASTA header mode not preserved: %#v", settings)
	}
	if !settings.PrependOnlyFirstQuery {
		t.Fatalf("family FASTA query prepend setting not preserved: %#v", settings)
	}
}

func TestFilesSummaryIncludesRawText(t *testing.T) {
	summary := filesSummary(exportFileResult{
		TextPath:     filepath.Join("out", "PAL.fasta"),
		RawExcelPath: filepath.Join("out", "PAL_raw.xlsx"),
		RawTextPath:  filepath.Join("out", "PAL_raw.fasta"),
	})

	if !strings.Contains(summary, "Raw FASTA") || !strings.Contains(summary, "PAL_raw.fasta") {
		t.Fatalf("raw FASTA missing from files summary:\n%s", summary)
	}
}

func TestInspectBlastGeneratedFilesIncludesRawText(t *testing.T) {
	dir := t.TempDir()
	rawTextPath := filepath.Join(dir, "PAL_raw.fasta")
	if err := os.WriteFile(rawTextPath, []byte(">PAL1\nMAAA\n"), 0o600); err != nil {
		t.Fatalf("write raw FASTA fixture: %v", err)
	}

	files, err := inspectBlastGeneratedFilesList(context.Background(), []exportFileResult{{RawTextPath: rawTextPath}}, report.NewGeneratedFileInspector())
	if err != nil {
		t.Fatalf("inspect generated files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("generated file count = %d, want 1", len(files))
	}
	if files[0].Name != "PAL_raw.fasta" || files[0].Type != "raw BLAST peptide FASTA" {
		t.Fatalf("raw FASTA file metadata not captured: %#v", files[0])
	}
}

func TestInspectKeywordGeneratedFilesIncludesRawText(t *testing.T) {
	dir := t.TempDir()
	rawTextPath := filepath.Join(dir, "keyword_raw.fasta")
	if err := os.WriteFile(rawTextPath, []byte(">hit\nMAAA\n"), 0o600); err != nil {
		t.Fatalf("write raw FASTA fixture: %v", err)
	}

	files, err := inspectKeywordGeneratedFiles(context.Background(), exportFileResult{RawTextPath: rawTextPath}, report.SequenceAudit{}, report.NewGeneratedFileInspector())
	if err != nil {
		t.Fatalf("inspect generated files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("generated file count = %d, want 1", len(files))
	}
	if files[0].Name != "keyword_raw.fasta" || files[0].Type != "raw peptide FASTA" {
		t.Fatalf("raw keyword FASTA file metadata not captured: %#v", files[0])
	}
}

func TestInspectGeneratedFilesReusesHashForDuplicatePath(t *testing.T) {
	dir := t.TempDir()
	rawTextPath := filepath.Join(dir, "shared.txt")
	if err := os.WriteFile(rawTextPath, []byte(">shared\nMAAA\n"), 0o600); err != nil {
		t.Fatalf("write shared text fixture: %v", err)
	}

	inspector := report.NewGeneratedFileInspector()
	files, err := inspectBlastGeneratedFilesList(context.Background(), []exportFileResult{{
		TextPath:    rawTextPath,
		RawTextPath: rawTextPath,
	}}, inspector)
	if err != nil {
		t.Fatalf("inspect duplicated generated files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("generated file count = %d, want 2", len(files))
	}
	if files[0].SHA256 != files[1].SHA256 || files[0].SHA1 != files[1].SHA1 || files[0].MD5 != files[1].MD5 {
		t.Fatalf("duplicate file hashes should match: %#v %#v", files[0], files[1])
	}
	if files[0].Type == files[1].Type {
		t.Fatalf("duplicate path metadata should still preserve distinct export roles: %#v %#v", files[0], files[1])
	}
}

func TestBuildKeywordReportDataRendersPDFForPhytozome(t *testing.T) {
	now := time.Now()
	row := model.KeywordResultRow{
		SourceDatabase:      "phytozome",
		SearchTerm:          "AT2G30490",
		LabelName:           "C4H",
		TranscriptID:        "AT2G30490.1",
		GeneIdentifier:      "AT2G30490",
		Genome:              "Athaliana_TAIR10",
		Aliases:             "C4H",
		Description:         "cinnamate 4-hydroxylase",
		GeneReportURL:       "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
		SequenceHeaderLabel: "Athaliana_TAIR10",
		SequenceID:          "AT2G30490.1",
	}
	groups := []model.KeywordSearchGroup{{
		SearchTerm:       row.SearchTerm,
		LabelName:        row.LabelName,
		LabelMethod:      "manual labels",
		SearchStartedAt:  now.Add(-2 * time.Second),
		SearchEndedAt:    now.Add(-1 * time.Second),
		SearchDurationMS: 1000,
		Rows:             []model.KeywordResultRow{row},
	}}
	w := &BlastWizard{source: keywordMapSource{name: "phytozome"}}
	data := w.buildKeywordReportData(
		[]model.KeywordResultRow{row},
		[]model.KeywordResultRow{row},
		groups,
		[]report.GeneratedFile{{
			Name:      "C4H.xlsx",
			Type:      "selected Excel",
			Role:      "test workbook",
			Path:      filepath.Join(t.TempDir(), "C4H.xlsx"),
			SizeBytes: 128,
			SHA256:    strings.Repeat("a", 64),
		}},
		"C4H",
		t.TempDir(),
		exportSettings{BaseName: "C4H", WriteExcel: true, WriteReport: true},
		&keywordReportRunContext{
			Selected:     model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis thaliana TAIR10"},
			QueryStarted: now.Add(-3 * time.Second),
			SearchEnded:  now.Add(-1 * time.Second),
			LabelMode:    "manual labels",
		},
		now.Add(-500*time.Millisecond),
		now,
		[]report.GenerationStep{keywordReportStep("Write selected Excel", now.Add(-400*time.Millisecond), now.Add(-250*time.Millisecond), "ok", "1 selected row written")},
		report.SequenceAudit{Requested: false},
	)

	if data.Keyword.Database != "Phytozome" {
		t.Fatalf("database label = %q, want Phytozome", data.Keyword.Database)
	}
	if len(data.Keyword.ColumnCompleteness) == 0 {
		t.Fatal("expected generated table column completeness stats")
	}
	for _, check := range data.Keyword.QualityChecks {
		if strings.Contains(check.Source, "report") {
			t.Fatalf("quality checks must not be based on report metadata: %#v", check)
		}
	}
	path := filepath.Join(t.TempDir(), "keyword_report.pdf")
	if err := report.RenderKeywordPDF(path, data); err != nil {
		t.Fatalf("RenderKeywordPDF() error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered PDF: %v", err)
	}
	if !bytes.HasPrefix(content, []byte("%PDF-")) {
		t.Fatalf("rendered file does not look like a PDF")
	}
}

func TestKeywordReportDataClassifiesLemnaDynamicColumns(t *testing.T) {
	row := model.KeywordResultRow{
		SourceDatabase: "lemna",
		SearchTerm:     "PAL",
		LabelName:      "PAL",
		TranscriptID:   "SpT0001",
		GeneIdentifier: "SpG0001",
		Description:    "phenylalanine ammonia lyase",
		ExtraColumns: map[string]string{
			"gff_start":         "1024",
			"ahrd_quality_code": "1",
		},
	}
	groups := []model.KeywordSearchGroup{{
		SearchTerm:  "PAL",
		LabelName:   "PAL",
		LabelMethod: "auto-identify labels",
		Rows:        []model.KeywordResultRow{row},
	}}
	w := &BlastWizard{source: keywordMapSource{name: "lemna"}}
	data := w.buildKeywordReportData(
		[]model.KeywordResultRow{row},
		[]model.KeywordResultRow{row},
		groups,
		nil,
		"PAL",
		t.TempDir(),
		exportSettings{BaseName: "PAL", WriteReport: true},
		&keywordReportRunContext{Selected: model.SpeciesCandidate{JBrowseName: "Sp7498v3", GenomeLabel: "Spirodela polyrhiza 7498", IsOfficial: true}, LabelMode: "auto-identify labels"},
		time.Now(),
		time.Now(),
		nil,
		report.SequenceAudit{Requested: false},
	)
	if data.Keyword.Database != "lemna.org" {
		t.Fatalf("database label = %q, want lemna.org", data.Keyword.Database)
	}
	sources := map[string]string{}
	for _, column := range data.Keyword.Columns {
		sources[column.Column] = column.Source
	}
	if sources["gff_start"] != "lemna GFF3" {
		t.Fatalf("gff_start source = %q, want lemna GFF3", sources["gff_start"])
	}
	if sources["ahrd_quality_code"] != "lemna AHRD" {
		t.Fatalf("ahrd_quality_code source = %q, want lemna AHRD", sources["ahrd_quality_code"])
	}
}

func TestDetectFamilyBlastGroupsStripsMemberIndex(t *testing.T) {
	items := []blastQueryItem{
		{LabelName: "PAL1"},
		{LabelName: "PAL2"},
		{LabelName: "PAL3"},
		{LabelName: "PAL4"},
		{LabelName: "ATPAL1"},
		{LabelName: "ATPAL2"},
		{LabelName: "ATCAD5"},
		{LabelName: "ATCAD6"},
		{LabelName: "4CL.1"},
		{LabelName: "4CL2"},
	}
	groups := detectFamilyBlastGroups(items, model.DefaultFamilyBlastSettings())
	got := map[string]int{}
	for _, group := range groups {
		got[group.Name] = len(group.Indexes)
	}
	if got["PAL"] != 4 {
		t.Fatalf("PAL group size = %d, want 4; groups=%#v", got["PAL"], groups)
	}
	if got["ATPAL"] != 2 {
		t.Fatalf("ATPAL group size = %d, want 2; groups=%#v", got["ATPAL"], groups)
	}
	if got["ATCAD"] != 2 {
		t.Fatalf("ATCAD group size = %d, want 2; groups=%#v", got["ATCAD"], groups)
	}
	if got["4CL"] != 2 {
		t.Fatalf("4CL group size = %d, want 2; groups=%#v", got["4CL"], groups)
	}
}

func TestDetectFamilyBlastGroupsIgnoresSuffixAfterMemberNumberByDefault(t *testing.T) {
	items := []blastQueryItem{
		{LabelName: "IRX9"},
		{LabelName: "IRX14"},
		{LabelName: "IRX10"},
		{LabelName: "IRX10-like"},
	}
	groups := detectFamilyBlastGroups(items, model.DefaultFamilyBlastSettings())
	if len(groups) != 1 {
		t.Fatalf("group count = %d, want 1: %#v", len(groups), groups)
	}
	if groups[0].Name != "IRX" {
		t.Fatalf("family name = %q, want IRX", groups[0].Name)
	}
	if len(groups[0].Indexes) != 4 {
		t.Fatalf("IRX group size = %d, want 4", len(groups[0].Indexes))
	}
}

func TestDetectFamilyBlastGroupsCanCollapseDistinctQuerySubgroupsWhenDisabled(t *testing.T) {
	items := []blastQueryItem{
		{LabelName: "IRX9"},
		{LabelName: "IRX14"},
		{LabelName: "IRX10"},
		{LabelName: "IRX10-like"},
	}
	settings := model.DefaultFamilyBlastSettings()
	settings.KeepDistinctQuerySubgroups = true
	groups := detectFamilyBlastGroups(items, settings)
	if len(groups) != 1 {
		t.Fatalf("group count = %d, want 1 subgroup for IRX10 family labels: %#v", len(groups), groups)
	}
	if groups[0].Name != "IRX" {
		t.Fatalf("family name = %q, want IRX", groups[0].Name)
	}
	if len(groups[0].Indexes) != 2 {
		t.Fatalf("IRX subgroup size = %d, want 2", len(groups[0].Indexes))
	}
}

func TestDetectFamilyBlastGroupsKeepsApostropheFamiliesDistinct(t *testing.T) {
	items := []blastQueryItem{
		{LabelName: "C3H"},
		{LabelName: "C3'H"},
	}
	settings := model.DefaultFamilyBlastSettings()
	groups := detectFamilyBlastGroups(items, settings)
	if len(groups) != 0 {
		t.Fatalf("group count = %d, want 0 because C3H and C3'H should stay distinct: %#v", len(groups), groups)
	}
	if got := detectFamilyName("C3H", settings); got != "C3H" {
		t.Fatalf("detectFamilyName(C3H)=%q want C3H", got)
	}
	if got := detectFamilyName("C3'H", settings); got != "C3'H" {
		t.Fatalf("detectFamilyName(C3'H)=%q want C3'H", got)
	}
}

func TestApplyFamilyBlastPlanMergesRunsByTarget(t *testing.T) {
	prepared := []blastQueryItem{{LabelName: "PAL1"}, {LabelName: "PAL2"}}
	runs := []blastQueryRun{
		{Index: 1, Item: prepared[0], Results: model.BlastResult{Rows: []model.BlastResultRow{{Protein: "Spipo1", EValue: "1e-20", PercentIdentity: 50, LabelName: "PAL1"}}}},
		{Index: 2, Item: prepared[1], Results: model.BlastResult{Rows: []model.BlastResultRow{{Protein: "Spipo1", EValue: "1e-40", PercentIdentity: 60, LabelName: "PAL2"}, {Protein: "Spipo2", EValue: "1e-10", PercentIdentity: 40, LabelName: "PAL2"}}}},
	}
	plan := &familyBlastPlan{
		Settings: model.DefaultFamilyBlastSettings(),
		Groups:   []familyBlastGroup{{Name: "PAL", Indexes: []int{0, 1}, Labels: []string{"PAL1", "PAL2"}}},
	}
	items, mergedRuns := applyFamilyBlastPlan(prepared, runs, plan)
	if len(items) != 1 || len(mergedRuns) != 1 {
		t.Fatalf("got %d items/%d runs, want one family run", len(items), len(mergedRuns))
	}
	if items[0].FamilyName != "PAL" || buildBlastOutputDisplayName(items[0]) != "PAL" {
		t.Fatalf("family item not named PAL: %#v", items[0])
	}
	if len(mergedRuns[0].Results.Rows) != 2 {
		t.Fatalf("merged row count = %d, want 2", len(mergedRuns[0].Results.Rows))
	}
	if mergedRuns[0].Results.Rows[0].EValue != "1e-40" {
		t.Fatalf("duplicate target did not keep best e-value row: %#v", mergedRuns[0].Results.Rows[0])
	}
}

func TestApplyFamilyBlastPlanMergesTranscriptIsoformsByTargetGene(t *testing.T) {
	prepared := []blastQueryItem{{LabelName: "C3H1"}, {LabelName: "C3H2"}}
	runs := []blastQueryRun{
		{Index: 1, Item: prepared[0], Results: model.BlastResult{Rows: []model.BlastResultRow{{Protein: "Sp9509d006g001100_T002", EValue: "1e-40", PercentIdentity: 70, LabelName: "C3H1"}}}},
		{Index: 2, Item: prepared[1], Results: model.BlastResult{Rows: []model.BlastResultRow{{Protein: "Sp9509d006g001100_T001", EValue: "1e-30", PercentIdentity: 65, LabelName: "C3H2", InterProConservedRegionStatus: "present"}}}},
	}
	settings := model.DefaultFamilyBlastSettings()
	settings.UseInterProReference = true
	plan := &familyBlastPlan{
		Settings: settings,
		Groups:   []familyBlastGroup{{Name: "C3H", Indexes: []int{0, 1}, Labels: []string{"C3H1", "C3H2"}}},
	}
	_, mergedRuns := applyFamilyBlastPlan(prepared, runs, plan)
	if len(mergedRuns) != 1 || len(mergedRuns[0].Results.Rows) != 1 {
		t.Fatalf("transcript isoforms should merge to one target gene row: %#v", mergedRuns)
	}
	if got := mergedRuns[0].Results.Rows[0].Protein; got != "Sp9509d006g001100_T001" {
		t.Fatalf("reference-supported isoform should win merge, got %q", got)
	}
}

func TestApplyFamilyBlastPlanUsesExternalReferenceEvidenceWhenMerging(t *testing.T) {
	prepared := []blastQueryItem{{LabelName: "PAL1"}, {LabelName: "PAL2"}}
	runs := []blastQueryRun{
		{Index: 1, Item: prepared[0], Results: model.BlastResult{Rows: []model.BlastResultRow{{
			Protein:                             "Spipo1",
			EValue:                              "1e-60",
			PercentIdentity:                     70,
			LabelName:                           "PAL1",
			InterProConservedRegionStatus:       "missing",
			UniProtAccession:                    "A0A000",
			UniProtReviewed:                     "unreviewed",
			TargetUniProtCanonicalLengthPercent: "40",
		}}}},
		{Index: 2, Item: prepared[1], Results: model.BlastResult{Rows: []model.BlastResultRow{{
			Protein:                             "Spipo1",
			EValue:                              "1e-20",
			PercentIdentity:                     50,
			LabelName:                           "PAL2",
			InterProConservedRegionStatus:       "present",
			UniProtAccession:                    "Q00001",
			UniProtReviewed:                     "reviewed",
			TargetUniProtCanonicalLengthPercent: "100",
		}}}},
	}
	settings := model.DefaultFamilyBlastSettings()
	settings.UseUniProtReference = true
	settings.UseInterProReference = true
	plan := &familyBlastPlan{
		Settings: settings,
		Groups:   []familyBlastGroup{{Name: "PAL", Indexes: []int{0, 1}, Labels: []string{"PAL1", "PAL2"}}},
	}
	_, mergedRuns := applyFamilyBlastPlan(prepared, runs, plan)
	if len(mergedRuns) != 1 || len(mergedRuns[0].Results.Rows) != 1 {
		t.Fatalf("unexpected merged runs: %#v", mergedRuns)
	}
	if got := mergedRuns[0].Results.Rows[0].LabelName; got != "PAL2" {
		t.Fatalf("reference-supported row should win duplicate target merge, got %q", got)
	}
}

func TestBlastSnapshotOriginalRunCountPrefersReviewContext(t *testing.T) {
	reviewCtx := &blastReviewContext{OriginalRunCount: 4}
	runs := []blastQueryRun{{Index: 1}}
	if got := blastSnapshotOriginalRunCount(reviewCtx, runs); got != 4 {
		t.Fatalf("blastSnapshotOriginalRunCount()=%d want 4", got)
	}
}

func TestBlastSnapshotOriginalRunCountFallsBackToVisibleRuns(t *testing.T) {
	runs := []blastQueryRun{{Index: 1}, {Index: 2}}
	if got := blastSnapshotOriginalRunCount(nil, runs); got != 2 {
		t.Fatalf("blastSnapshotOriginalRunCount()=%d want 2", got)
	}
	if got := blastSnapshotOriginalRunCount(&blastReviewContext{}, nil); got != 1 {
		t.Fatalf("blastSnapshotOriginalRunCount() with empty context=%d want 1", got)
	}
}

func TestCustomPromptFamilyBlastGroupsMapsLabelsBackToPreparedIndexes(t *testing.T) {
	prepared := []blastQueryItem{
		{LabelName: "PAL1"},
		{LabelName: "PAL2"},
		{LabelName: "CCR1"},
	}
	custom := []prompt.FamilyBlastGroup{{
		Name:   "PAL",
		Labels: []string{"PAL2", "PAL1"},
	}}
	groups := customPromptFamilyBlastGroups(prepared, custom)
	if len(groups) != 1 {
		t.Fatalf("group count = %d, want 1", len(groups))
	}
	if groups[0].Name != "PAL" {
		t.Fatalf("group name = %q, want PAL", groups[0].Name)
	}
	if groups[0].GroupSource != "customized groups" {
		t.Fatalf("group source = %q, want customized groups", groups[0].GroupSource)
	}
	if groups[0].DetectionRule != "customized in Family BLAST group editor" {
		t.Fatalf("detection rule = %q", groups[0].DetectionRule)
	}
	if len(groups[0].Indexes) != 2 || groups[0].Indexes[0] != 1 || groups[0].Indexes[1] != 0 {
		t.Fatalf("unexpected mapped indexes: %#v", groups[0].Indexes)
	}
}

func TestCustomPromptFamilyBlastGroupsMapsRenamedMembersByStableSourceKey(t *testing.T) {
	prepared := []blastQueryItem{
		{LabelName: "PAL1", QuerySource: &model.QuerySequenceSource{ProteinID: "PAC:1", LabelName: "PAL1", Aliases: "PAL1; ATPAL1"}},
		{LabelName: "PAL2", QuerySource: &model.QuerySequenceSource{ProteinID: "PAC:2", LabelName: "PAL2", Aliases: "PAL2; ATPAL2"}},
	}
	members := []familyBlastMember{familyBlastMemberForItem(prepared[0]), familyBlastMemberForItem(prepared[1])}
	custom := []prompt.FamilyBlastGroup{{
		Name: "PAL-renamed",
		Members: []prompt.FamilyBlastMember{
			{LabelName: "MY-PAL1", ProteinID: members[0].ProteinID, OriginalLabelName: members[0].OriginalLabelName, SourceKey: members[0].SourceKey, Aliases: members[0].Aliases},
			{LabelName: "MY-PAL2", ProteinID: members[1].ProteinID, OriginalLabelName: members[1].OriginalLabelName, SourceKey: members[1].SourceKey, Aliases: members[1].Aliases},
		},
	}}

	groups := customPromptFamilyBlastGroups(prepared, custom)
	if len(groups) != 1 {
		t.Fatalf("group count = %d, want 1", len(groups))
	}
	if got := groups[0].Labels; len(got) != 2 || got[0] != "MY-PAL1" || got[1] != "MY-PAL2" {
		t.Fatalf("labels after rename = %#v", got)
	}
	if got := prepared[0].QuerySource.LabelName; got != "MY-PAL1" {
		t.Fatalf("prepared[0] QuerySource.LabelName = %q, want MY-PAL1", got)
	}
}

func TestDetectFamilyBlastGroupsAnnotatesAutomaticSource(t *testing.T) {
	items := []blastQueryItem{{LabelName: "PAL1"}, {LabelName: "PAL2"}}
	groups := detectFamilyBlastGroups(items, model.DefaultFamilyBlastSettings())
	if len(groups) != 1 {
		t.Fatalf("group count = %d, want 1", len(groups))
	}
	if groups[0].GroupSource != "automatic detection" {
		t.Fatalf("group source = %q, want automatic detection", groups[0].GroupSource)
	}
	if !strings.Contains(groups[0].DetectionRule, "auto-detected from query labels") {
		t.Fatalf("detection rule = %q", groups[0].DetectionRule)
	}
}

func TestBlastFamilyReportBatchCapturesCustomizedGroupingMetadata(t *testing.T) {
	settings := model.DefaultFamilyBlastSettings()
	settings.CustomizeGroups = true
	runs := []blastQueryRun{{
		Index: 1,
		Item: blastQueryItem{
			LabelName:           "PAL",
			FamilyName:          "PAL",
			MemberLabel:         "PAL2\nPAL1",
			FamilyGroupSource:   "customized groups",
			FamilyDetectionRule: "customized in Family BLAST group editor",
			FamilySettings:      settings,
		},
		Results:         model.BlastResult{Rows: []model.BlastResultRow{{Protein: "Spipo1"}}},
		RowsBeforeMerge: 5,
		RowsAfterMerge:  3,
	}}

	report := blastFamilyReportBatch(runs)
	if report == nil {
		t.Fatal("expected family report")
	}
	if len(report.Groups) != 1 {
		t.Fatalf("group count = %d, want 1", len(report.Groups))
	}
	if report.Groups[0].GroupSource != "customized groups" {
		t.Fatalf("group source = %q, want customized groups", report.Groups[0].GroupSource)
	}
	if report.Groups[0].DetectionRule != "customized in Family BLAST group editor" {
		t.Fatalf("detection rule = %q", report.Groups[0].DetectionRule)
	}
	foundCustomizeSetting := false
	for _, row := range report.Settings {
		if row.Name == "Used custom group editor" {
			foundCustomizeSetting = true
			if row.Value != "true" {
				t.Fatalf("group editor setting value = %q, want true", row.Value)
			}
		}
	}
	if !foundCustomizeSetting {
		t.Fatal("group editor setting missing from family report")
	}
}

func TestBuildPromptFamilyBlastPreviewKeepsUngroupedItems(t *testing.T) {
	prepared := []blastQueryItem{
		{LabelName: "PAL1"},
		{LabelName: "PAL2"},
		{LabelName: "CCR1"},
	}
	settings := model.DefaultFamilyBlastSettings()
	groups := detectFamilyBlastGroups(prepared, settings)
	preview := buildPromptFamilyBlastPreview(prepared, groups)
	if len(preview.Groups) != 1 {
		t.Fatalf("preview groups = %d, want 1", len(preview.Groups))
	}
	if len(preview.Ungrouped) != 1 || preview.Ungrouped[0] != "CCR1" {
		t.Fatalf("unexpected ungrouped labels: %#v", preview.Ungrouped)
	}
}

func TestBlastFastaHeaderLabelPrefersLabelName(t *testing.T) {
	item := blastQueryItem{LabelName: "AtPAL1"}
	if got := blastFastaHeaderLabel(item, "CustomFileName"); got != "AtPAL1" {
		t.Fatalf("FASTA header label = %q, want label name", got)
	}
}

func TestBlastFastaHeaderLabelFallsBackToFileName(t *testing.T) {
	item := blastQueryItem{}
	if got := blastFastaHeaderLabel(item, "CustomFileName"); got != "CustomFileName" {
		t.Fatalf("FASTA header label = %q, want file name", got)
	}
}

func TestFamilyFastaHeaderLabelPrefersQueryIdentityBeforeFallback(t *testing.T) {
	source := &model.QuerySequenceSource{
		LabelName:    "",
		Aliases:      "ATPAL1; PAL1",
		GeneID:       "AT2G37040",
		TranscriptID: "AT2G37040.1",
		ProteinID:    "AT2G37040.1",
	}
	if got := familyFastaHeaderLabel(source, "PAL"); got != "ATPAL1" {
		t.Fatalf("familyFastaHeaderLabel()=%q want ATPAL1", got)
	}
}

func TestFamilyFastaQueryIndexesRespectPrependOnlyFirstQuery(t *testing.T) {
	sources := []*model.QuerySequenceSource{{LabelName: "PAL1"}, {LabelName: "PAL2"}, {LabelName: "PAL3"}}
	firstOnly := familyFastaQueryIndexes(sources, model.FamilyBlastSettings{PrependOnlyFirstQuery: true})
	if len(firstOnly) != 1 || firstOnly[0] != 0 {
		t.Fatalf("first-only indexes = %#v", firstOnly)
	}
	all := familyFastaQueryIndexes(sources, model.FamilyBlastSettings{PrependOnlyFirstQuery: false})
	if len(all) != 3 || all[0] != 0 || all[2] != 2 {
		t.Fatalf("all indexes = %#v", all)
	}
}

func TestFamilyExportQueryPrependOptionOnlyForFamilyItems(t *testing.T) {
	single := blastQueryItem{QuerySource: &model.QuerySequenceSource{LabelName: "PAL1"}}
	if show, _ := familyExportQueryPrependOptionForItem(single); show {
		t.Fatal("single-query export should not show family FASTA prepend option")
	}
	singleFamily := blastQueryItem{
		QuerySource:    &model.QuerySequenceSource{LabelName: "PAL1"},
		FamilySettings: model.FamilyBlastSettings{Enabled: true},
	}
	if show, _ := familyExportQueryPrependOptionForItem(singleFamily); !show {
		t.Fatal("family export should show family FASTA prepend option even with one query source")
	}
	family := blastQueryItem{
		FamilySources:  []*model.QuerySequenceSource{{LabelName: "PAL1"}, {LabelName: "PAL2"}},
		FamilySettings: model.FamilyBlastSettings{Enabled: true, PrependOnlyFirstQuery: true},
	}
	show, initial := familyExportQueryPrependOptionForItem(family)
	if !show || !initial {
		t.Fatalf("family export option = show %v initial %v, want true true", show, initial)
	}
}

func TestBuildBlastSequenceAuditUsesQueryLabelModeText(t *testing.T) {
	audit := buildBlastSequenceAudit(nil, nil, []*model.QuerySequenceSource{{LabelName: "PAL1"}}, true)
	if !strings.Contains(audit.HeaderLabelMode, "best available query label") {
		t.Fatalf("unexpected header label mode: %q", audit.HeaderLabelMode)
	}
}

func TestAggregateBlastSequenceAuditMergesQuerySummaries(t *testing.T) {
	files := []exportFileResult{
		{SequenceAudit: report.SequenceAudit{
			Requested:      true,
			QuerySummaries: []report.SequenceQuerySummary{{QueryLabel: "PAL1", QueryKind: "query sequence record", RequestedCount: 1, WrittenCount: 1, MinLength: 711, MaxLength: 711, TotalLength: 711, SourceSummary: "prepended query"}},
		}},
		{SequenceAudit: report.SequenceAudit{
			Requested:      true,
			QuerySummaries: []report.SequenceQuerySummary{{QueryLabel: "PAL family export", QueryKind: "selected hit peptide records", RequestedCount: 12, WrittenCount: 11, SkippedCount: 1, MinLength: 661, MaxLength: 718, TotalLength: 7528, SourceSummary: "selected BLAST hit peptide export"}},
		}},
	}
	audit := aggregateBlastSequenceAudit(files, true)
	if len(audit.QuerySummaries) != 2 {
		t.Fatalf("expected 2 query summaries, got %#v", audit.QuerySummaries)
	}
	if audit.QuerySummaries[0].QueryLabel != "PAL1" || audit.QuerySummaries[1].QueryLabel != "PAL family export" {
		t.Fatalf("unexpected query summary order: %#v", audit.QuerySummaries)
	}
	if audit.QuerySummaries[1].AverageLength != 684 {
		t.Fatalf("unexpected average length: %#v", audit.QuerySummaries[1])
	}
}

func TestAutoIdentifyKeywordLabelsUsesBestAliasCandidate(t *testing.T) {
	groups := []model.KeywordSearchGroup{{
		SearchTerm: "PAL",
		Rows: []model.KeywordResultRow{{
			Aliases:        "ATPAL1; PAL1",
			GeneIdentifier: "AT2G37040",
			TranscriptID:   "AT2G37040.1",
		}},
	}}

	labels := autoIdentifyKeywordLabels(groups)
	if len(labels) != 1 || labels[0] != "PAL1" {
		t.Fatalf("unexpected auto labels: %#v", labels)
	}
	applyKeywordIdentifications(groups, labels)
	if groups[0].LabelName != "PAL1" || groups[0].Rows[0].LabelName != "PAL1" {
		t.Fatalf("auto label was not applied to group and rows: %#v", groups)
	}
}

func TestAutoIdentifyKeywordLabelsDoesNotSpecialCaseLemnaLocalRows(t *testing.T) {
	groups := []model.KeywordSearchGroup{{
		SearchTerm: "CYP73A38",
		Rows: []model.KeywordResultRow{{
			LabelName:      "C4H",
			GeneIdentifier: "Sp9509d006g002010",
			TranscriptID:   "Sp9509d006g002010_T001",
			Description:    "Trans-cinnamate 4-monooxygenase",
			UniProt:        "P48522",
		}},
	}}

	labels := autoIdentifyKeywordLabels(groups)
	if len(labels) != 1 || labels[0] != "C4H" {
		t.Fatalf("generic keyword auto labels should resolve through gene_info: %#v", labels)
	}
}

func TestAutoIdentifyKeywordLabelsReturnsEmptyWhenPhytozomeRowMetadataHasNoDatabaseMatch(t *testing.T) {
	groups := []model.KeywordSearchGroup{{
		SearchTerm: "Os4CL1",
		Rows: []model.KeywordResultRow{{
			SourceDatabase: "phytozome",
			UniProt:        "LOC_Os08g14760.1: P17814; LOC_Os08g14760.1: 4CL1_ORYSJ",
			Description:    "AMP-binding domain containing protein, expressed",
			AutoDefine:     "(1 of 8) 6.2.1.12 - 4-coumarate--CoA ligase. / 4-coumaryl-CoA synthetase.",
			TranscriptID:   "LOC_Os08g14760.1",
			GeneIdentifier: "LOC_Os08g14760",
		}},
	}}

	labels := autoIdentifyKeywordLabels(groups)
	if len(labels) != 1 || labels[0] != "" {
		t.Fatalf("expected empty label without gene_info match, got %#v", labels)
	}
}

func TestKeywordProteinSequenceHeaderUsesLabelName(t *testing.T) {
	row := model.KeywordResultRow{
		LabelName:           "C4H",
		SequenceHeaderLabel: "Spirodela polyrhiza",
		TranscriptID:        "Sp9509d020g000340_T001",
	}
	if got := keywordProteinSequenceHeader(row); got != ">Spirodela polyrhiza|Sp9509d020g000340_T001 (C4H)" {
		t.Fatalf("unexpected keyword sequence header: %q", got)
	}
}

func TestApplyOriginalHeadersRestoresOriginalHeader(t *testing.T) {
	records := []model.ProteinSequenceRecord{{
		Header:         ">phgo://species/PAL1/AT2G37040\\1",
		OriginalHeader: ">Arabidopsis thaliana TAIR10|AT2G37040.1 (PAL1)",
		SourceKey:      "keyword|phytozome|AT2G37040.1|AT2G37040.1|AT2G37040",
		Sequence:       "MPEPTIDE",
	}}
	got := applyOriginalHeaders(records)
	if got[0].Header != records[0].OriginalHeader {
		t.Fatalf("header = %q, want original %q", got[0].Header, records[0].OriginalHeader)
	}
}

func TestApplyKeywordMinimalHeadersUsesPrimaryIDOnly(t *testing.T) {
	rows := []model.KeywordResultRow{{
		SequenceHeaderLabel: "Arabidopsis thaliana TAIR10",
		TranscriptID:        "AT2G37040.1",
		SequenceID:          "AT2G37040.1",
		GeneIdentifier:      "AT2G37040",
	}}
	records := []model.ProteinSequenceRecord{{
		Header:         ">Arabidopsis thaliana TAIR10|AT2G37040.1 (PAL1)",
		OriginalHeader: ">Arabidopsis thaliana TAIR10|AT2G37040.1 (PAL1)",
		SourceKey:      keywordSequenceRecordSourceKey(rows[0]),
		Sequence:       "MPEPTIDE",
	}}
	got := applyKeywordMinimalHeaders(records, rows)
	if got[0].Header != ">AT2G37040.1" {
		t.Fatalf("header = %q, want minimal primary ID", got[0].Header)
	}
}

func TestApplyBlastMinimalHeadersHandlesPrependedQueryAndHits(t *testing.T) {
	source := &model.QuerySequenceSource{
		TranscriptID: "AT2G37040.1",
		Sequence:     "MPEPTIDE",
	}
	rows := []model.BlastResultRow{{
		Protein:    "Sp9509d020g000340_T001",
		SequenceID: "fallback",
	}}
	records := []model.ProteinSequenceRecord{
		{Header: ">Arabidopsis thaliana TAIR10|AT2G37040.1 (PAL1)", OriginalHeader: ">Arabidopsis thaliana TAIR10|AT2G37040.1 (PAL1)", SourceKey: querySequenceRecordSourceKey(source), Sequence: "MPEPTIDE"},
		{Header: ">Spirodela polyrhiza|Sp9509d020g000340_T001 (C4H)", OriginalHeader: ">Spirodela polyrhiza|Sp9509d020g000340_T001 (C4H)", SourceKey: blastSequenceRecordSourceKey(rows[0]), Sequence: "MPEPTIDE"},
	}
	got := applyBlastMinimalHeaders(records, rows, []*model.QuerySequenceSource{source}, 1)
	if got[0].Header != ">AT2G37040.1" {
		t.Fatalf("query header = %q, want minimal query ID", got[0].Header)
	}
	if got[1].Header != ">Sp9509d020g000340_T001" {
		t.Fatalf("hit header = %q, want minimal hit ID", got[1].Header)
	}
}

func TestBlastProteinSequenceHeaderPrefersBestAvailableIdentifier(t *testing.T) {
	tests := []struct {
		name string
		row  model.BlastResultRow
		want string
	}{
		{
			name: "protein",
			row:  model.BlastResultRow{Protein: "AT2G37040.1", SequenceID: "seq", TranscriptID: "tx", SubjectID: "sub"},
			want: ">AT2G37040.1",
		},
		{
			name: "sequence id fallback",
			row:  model.BlastResultRow{SequenceID: "Sp9509d020g000340_T001", TranscriptID: "tx", SubjectID: "sub"},
			want: ">Sp9509d020g000340_T001",
		},
		{
			name: "transcript fallback",
			row:  model.BlastResultRow{TranscriptID: "LOC_Os01g01010.1", SubjectID: "sub"},
			want: ">LOC_Os01g01010.1",
		},
		{
			name: "subject fallback",
			row:  model.BlastResultRow{SubjectID: "transcript1"},
			want: ">transcript1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := blastProteinSequenceHeader(tt.row); got != tt.want {
				t.Fatalf("blastProteinSequenceHeader()=%q want %q", got, tt.want)
			}
		})
	}
}

func TestKeywordSequenceRecordSourceKeyIsStable(t *testing.T) {
	row := model.KeywordResultRow{
		SourceDatabase: "phytozome",
		SequenceID:     "AT2G37040.1",
		TranscriptID:   "AT2G37040.1",
		GeneIdentifier: "AT2G37040",
	}
	got := keywordSequenceRecordSourceKey(row)
	want := "keyword|phytozome|AT2G37040.1|AT2G37040.1|AT2G37040"
	if got != want {
		t.Fatalf("keywordSequenceRecordSourceKey()=%q want %q", got, want)
	}
}

func TestBuildKeywordSequenceAuditMatchesBySourceKeyNotHeader(t *testing.T) {
	rows := []model.KeywordResultRow{{
		SourceDatabase:      "phytozome",
		SearchTerm:          "PAL",
		LabelName:           "PAL1",
		SequenceHeaderLabel: "Arabidopsis thaliana TAIR10",
		TranscriptID:        "AT2G37040.1",
		SequenceID:          "AT2G37040.1",
		GeneIdentifier:      "AT2G37040",
	}}
	records := []model.ProteinSequenceRecord{{
		Header:         ">some-real-source-header",
		OriginalHeader: ">some-real-source-header",
		SourceKey:      keywordSequenceRecordSourceKey(rows[0]),
		Sequence:       "MPEPTIDE",
	}}
	audit := buildKeywordSequenceAudit(rows, records)
	if audit.WrittenCount != 1 || audit.SkippedCount != 0 {
		t.Fatalf("unexpected audit counts: %#v", audit)
	}
	if len(audit.Records) != 1 || audit.Records[0].Status != "written" {
		t.Fatalf("unexpected audit records: %#v", audit.Records)
	}
}

func TestBuildPhgoHeaderIncludesRowNumber(t *testing.T) {
	got := buildPhgoHeader("Sp7498", "PAL1", "AT2G37040", 7)
	want := ">phgo://Sp7498/PAL1/AT2G37040\\7"
	if got != want {
		t.Fatalf("buildPhgoHeader()=%q want %q", got, want)
	}
}

func TestBuildPhgoHeaderOmitsRowNumberWhenZero(t *testing.T) {
	got := buildPhgoHeader("Sp7498", "PAL1", "AT2G37040", 0)
	want := ">phgo://Sp7498/PAL1/AT2G37040"
	if got != want {
		t.Fatalf("buildPhgoHeader()=%q want %q", got, want)
	}
}

func TestBuildPhgoLiteHeaderUsesCompactIdentityFormat(t *testing.T) {
	got := buildPhgoLiteHeader("Brachypodium distachyon", "Bradi3g18960", "Bd4CL1")
	want := ">Brachypodium distachyon|Bradi3g18960(Bd4CL1)"
	if got != want {
		t.Fatalf("buildPhgoLiteHeader()=%q want %q", got, want)
	}
}

func TestParsePhgoLiteFastaHeader(t *testing.T) {
	parsed, ok := parsePhgoFastaHeader(">Brachypodium distachyon|Bradi3g18960(Bd4CL1)")
	if !ok || !parsed.IsLiteHeader {
		t.Fatalf("expected PHgo Lite header to parse: %#v ok=%v", parsed, ok)
	}
	if parsed.Species != "Brachypodium distachyon" || parsed.GeneID != "Bradi3g18960" || parsed.LabelName != "Bd4CL1" {
		t.Fatalf("unexpected PHgo Lite fields: %#v", parsed)
	}
}

func TestPhgoLiteHeaderOmitsAndIgnoresEmptySymbols(t *testing.T) {
	for _, symbol := range []string{"", "~", "~~"} {
		header := buildPhgoLiteHeader("Bd21-3", "Bradi3g18960", symbol)
		if header != ">Bd21-3|Bradi3g18960" {
			t.Fatalf("PHgo Lite header for symbol %q = %q", symbol, header)
		}
		parsed, ok := parsePhgoFastaHeader(header)
		if !ok || parsed.LabelName != "" {
			t.Fatalf("PHgo Lite header without symbol parsed incorrectly: %#v ok=%v", parsed, ok)
		}
	}
	for _, header := range []string{">Bd21-3|Bradi3g18960(~)", ">Bd21-3|Bradi3g18960(~~)"} {
		parsed, ok := parsePhgoFastaHeader(header)
		if !ok || parsed.LabelName != "" {
			t.Fatalf("PHgo Lite placeholder symbol should be ignored: %#v ok=%v", parsed, ok)
		}
	}
}

func TestBlastPhgoHeaderIncludesHitAndBlastSourceMetadata(t *testing.T) {
	got := blastPhgoHeader(model.BlastResultRow{
		Species:        "Sp7498",
		LabelName:      "C4H",
		BlastLabelName: "PAL1",
		BlastGeneID:    "AT2G37040",
		TranscriptID:   "AT2G37040.1",
		Protein:        "Sp7498_C4H_001",
		SequenceID:     "PAC:123456",
		SubjectID:      "PAC:123456",
	}, 7)
	want := ">phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\7"
	if got != want {
		t.Fatalf("blastPhgoHeader()=%q want %q", got, want)
	}
}

func TestParsePhgoFastaHeaderKeepsBackslashDelimitedGroups(t *testing.T) {
	parsed, ok := parsePhgoFastaHeader("phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\7")
	if !ok {
		t.Fatal("expected phgo header to parse")
	}
	if parsed.Species != "Sp7498" || parsed.LabelName != "C4H" || parsed.GeneID != "Sp7498_C4H_001" {
		t.Fatalf("self group parsed incorrectly: %#v", parsed)
	}
	if parsed.BlastSourceLabelName != "PAL1" || parsed.BlastSourceGeneID != "AT2G37040" {
		t.Fatalf("source group parsed incorrectly: %#v", parsed)
	}
	if !parsed.HasRowPart || parsed.RowNumber != 7 {
		t.Fatalf("table group parsed incorrectly: %#v", parsed)
	}
}

func TestParsePhgoFastaHeaderReadsCanvasDisplayMetadata(t *testing.T) {
	parsed, ok := parsePhgoFastaHeader("phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\-2/My canvas\\Display PAL")
	if !ok {
		t.Fatal("expected canvas phgo header to parse")
	}
	if !parsed.IsCanvasHeader || !parsed.CanvasHasRawRow || parsed.CanvasRawRowNumber != -2 {
		t.Fatalf("canvas row metadata parsed incorrectly: %#v", parsed)
	}
	if parsed.CanvasItemTitle != "My canvas" || parsed.CanvasDisplayName != "Display PAL" {
		t.Fatalf("canvas title/display parsed incorrectly: %#v", parsed)
	}
	if parsed.BlastSourceLabelName != "PAL1" || parsed.BlastSourceGeneID != "AT2G37040" {
		t.Fatalf("source group parsed incorrectly: %#v", parsed)
	}
}

func TestParsePhgoFastaHeaderTreatsTildeAsEmptyPlaceholder(t *testing.T) {
	parsed, ok := parsePhgoFastaHeader("phgo://~/~/AT1G01010\\~/~\\-1/~\\~")
	if !ok {
		t.Fatal("expected placeholder phgo header to parse")
	}
	if parsed.Species != "" || parsed.LabelName != "" || parsed.BlastSourceLabelName != "" || parsed.BlastSourceGeneID != "" || parsed.CanvasItemTitle != "" || parsed.CanvasDisplayName != "" {
		t.Fatalf("tilde placeholders should parse as empty fields: %#v", parsed)
	}
	if parsed.GeneID != "AT1G01010" || !parsed.CanvasHasRawRow || parsed.CanvasRawRowNumber != -1 {
		t.Fatalf("non-placeholder fields parsed incorrectly: %#v", parsed)
	}
}

func TestKeywordPhgoHeaderPrefersTranscriptID(t *testing.T) {
	got := keywordPhgoHeader(model.KeywordResultRow{
		SequenceHeaderLabel: "Athaliana_TAIR10",
		LabelName:           "PAL1",
		TranscriptID:        "AT2G37040.1",
		GeneIdentifier:      "AT2G37040 (PAC:123456)",
	}, 7)
	want := ">phgo://Athaliana_TAIR10/PAL1/AT2G37040.1\\7"
	if got != want {
		t.Fatalf("keywordPhgoHeader()=%q want %q", got, want)
	}
}

func TestMatchPhytozomeSpeciesForLemnaRequiresUniqueScientificName(t *testing.T) {
	lemnaSpecies := model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0"}
	candidates := []model.SpeciesCandidate{
		{SearchAlias: "Spirodela polyrhiza v2", JBrowseName: "Spolyrhiza_v2"},
	}
	got, ok := matchPhytozomeSpeciesForLemna(lemnaSpecies, candidates)
	if !ok || got.JBrowseName != "Spolyrhiza_v2" {
		t.Fatalf("unexpected match: %#v ok=%v", got, ok)
	}

	_, ok = matchPhytozomeSpeciesForLemna(lemnaSpecies, append(candidates, model.SpeciesCandidate{SearchAlias: "Spirodela polyrhiza v3"}))
	if ok {
		t.Fatal("ambiguous phytozome species should not match")
	}
}

func TestAutoIdentifyBlastLabelUsesLocalSymbolDatabaseForStructuredIDs(t *testing.T) {
	w := &BlastWizard{}
	src := &countingKeywordMapSource{keywordMapSource: keywordMapSource{rowsByKeyword: map[string][]model.KeywordResultRow{
		"AT2G37040.1": {{Aliases: "SHOULDNOTUSE", TranscriptID: "AT2G37040.1"}},
	}}}
	item := blastQueryItem{
		RawInput: ">A.thaliana TAIR10|AT2G37040.1\nMPEPTIDE",
		QuerySource: &model.QuerySequenceSource{
			SourceDatabase: "tair",
			ProteinID:      "AT2G37040.1",
			TranscriptID:   "AT2G37040.1",
			GeneID:         "AT2G37040",
		},
	}

	got := w.autoIdentifyBlastLabel(context.Background(), src, model.SpeciesCandidate{ProteomeID: 167}, item)
	if got != "PAL1" {
		t.Fatalf("unexpected label: %q", got)
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if len(src.fetchCount) != 0 {
		t.Fatalf("symbolname auto-identify should not query source rows, got %#v", src.fetchCount)
	}
}

func TestAutoIdentifyBlastLabelUsesQuerySourceAndIgnoresFastaHeaderLabel(t *testing.T) {
	w := &BlastWizard{}
	item := blastQueryItem{
		RawInput:    ">Arabidopsis thaliana TAIR10|AT5G62380.1 (AtVND6)\nMESLAHIPP",
		QuerySource: &model.QuerySequenceSource{SourceDatabase: "phytozome", Synonyms: "SOMETHINGELSE1; VND6"},
	}
	got := w.autoIdentifyBlastLabel(context.Background(), keywordMapSource{}, model.SpeciesCandidate{}, item)
	if got != "SOMETHINGELSE1" {
		t.Fatalf("unexpected label: %q", got)
	}
}

func TestAutoIdentifyBlastLabelUsesResolvedPhytozomeAliases(t *testing.T) {
	w := &BlastWizard{}
	item := blastQueryItem{QuerySource: &model.QuerySequenceSource{
		SourceDatabase: "phytozome",
		NormalizedURL:  "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT3G10340",
		Synonyms:       "ATPAL4",
		Symbols:        "PAL4",
	}}
	got := w.autoIdentifyBlastLabel(context.Background(), keywordMapSource{}, model.SpeciesCandidate{}, item)
	if got != "ATPAL4" {
		t.Fatalf("unexpected label: %q", got)
	}
}

func TestPhytozomeRawBlastAliasesAreNotReusableUntilRanked(t *testing.T) {
	items := []blastQueryItem{{QuerySource: &model.QuerySequenceSource{
		SourceDatabase: "phytozome",
		Synonyms:       "ATPAL4; PAL4",
		Symbols:        "PAL4",
		AutoDefine:     "phenylalanine ammonia-lyase 4",
	}}}
	if blastItemsHaveReusableAliases(items) {
		t.Fatalf("raw synonyms/symbols/auto_define should be sent to labelname, not treated as reusable phgo_alias")
	}
	items[0].QuerySource.PhgoAliases = "PAL4; ATPAL4"
	if !blastItemsHaveReusableAliases(items) {
		t.Fatalf("ranked phgo_alias should be reusable")
	}
}

func TestAutoIdentifyBlastLabelResultUsesLocalSymbolDatabaseAliasesAndIgnoresHeaderLabel(t *testing.T) {
	w := &BlastWizard{}
	src := keywordMapSource{rowsByKeyword: map[string][]model.KeywordResultRow{
		"AT1G01010.1": {{Synonyms: "NAC001; ANAC001", AutoDefine: "NAC domain protein", SourceDatabase: "phytozome"}},
		"AT1G01010":   {{Synonyms: "NAC001; ANAC001", AutoDefine: "NAC domain protein", SourceDatabase: "phytozome"}},
	}}
	item := blastQueryItem{
		RawInput: ">A.thaliana TAIR10|AT1G01010.1 (OldName)\nMPEPTIDE",
		QuerySource: &model.QuerySequenceSource{
			ProteinID:    "AT1G01010.1",
			TranscriptID: "AT1G01010.1",
			GeneID:       "AT1G01010",
		},
	}

	result := w.autoIdentifyBlastLabelResult(context.Background(), src, model.SpeciesCandidate{GenomeLabel: "Arabidopsis thaliana"}, item)
	if result.Label != "ANAC001" && result.Label != "NAC001" {
		t.Fatalf("label = %q, want database symbol-name candidate", result.Label)
	}
	if !containsString(result.Aliases, "ANAC001") || !containsString(result.Aliases, "NAC001") {
		t.Fatalf("aliases = %#v, want database symbol-name candidates", result.Aliases)
	}
}

func TestAutoIdentifyBlastLabelResultUsesDraggedFastaFileHeaderSpecies(t *testing.T) {
	path := `C:\Users\wangsychn\Desktop\phytozome-go_windows_amd64\output\Monolignol Polymerization.txt`
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("sample FASTA file is not available: %v", err)
	}
	items, err := parseBlastQueryItems(string(data))
	if err != nil {
		t.Fatalf("parse sample FASTA: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("sample FASTA parsed no items")
	}
	w := &BlastWizard{}
	src := keywordMapSource{rowsByKeyword: map[string][]model.KeywordResultRow{
		"AT2G29130.1": {{Aliases: "LAC2; TT10", LabelName: "LAC2", AutoDefine: "laccase 2"}},
	}}

	result := w.autoIdentifyBlastLabelResult(context.Background(), src, model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza"}, items[0])
	if len(result.Aliases) < 2 {
		t.Fatalf("aliases = %#v, want keyword aliases from FASTA protein id", result.Aliases)
	}
	found := false
	for _, alias := range result.Aliases {
		if alias == "TT10" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("aliases = %#v, want TT10 from keyword lookup", result.Aliases)
	}
}

func TestAutoIdentifyBlastLabelResultLetsLocalSymbolDatabaseAliasOverrideHeaderLabel(t *testing.T) {
	w := &BlastWizard{}
	src := keywordMapSource{rowsByKeyword: map[string][]model.KeywordResultRow{
		"AT5G62380.1": {{Synonyms: "VND6; ANAC101", AutoDefine: "vascular related NAC domain 6", SourceDatabase: "phytozome"}},
	}}
	item := blastQueryItem{
		RawInput: ">Arabidopsis thaliana TAIR10|AT5G62380.1 (HeaderName)\nMESLAHIPP",
		QuerySource: &model.QuerySequenceSource{
			SourceDatabase: "fasta",
			ProteinID:      "AT5G62380.1",
			TranscriptID:   "AT5G62380.1",
			GeneID:         "AT5G62380",
		},
	}

	result := w.autoIdentifyBlastLabelResult(context.Background(), src, model.SpeciesCandidate{GenomeLabel: "Arabidopsis thaliana"}, item)
	if result.Label != "VND6" {
		t.Fatalf("label = %q, want gene_info symbol VND6", result.Label)
	}
	if !containsString(result.Aliases, "VND6") || !containsString(result.Aliases, "ANAC101") {
		t.Fatalf("aliases = %#v, want database symbol-name candidates", result.Aliases)
	}
}

func TestSupplementBlastAliasesPreservesExistingFastaLabels(t *testing.T) {
	path := `C:\Users\wangsychn\Desktop\phytozome-go_windows_amd64\output\Monolignol Polymerization.txt`
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("sample FASTA file is not available: %v", err)
	}
	items, err := parseBlastQueryItems(string(data))
	if err != nil {
		t.Fatalf("parse sample FASTA: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("sample FASTA parsed no items")
	}
	if items[0].LabelName != "" {
		t.Fatalf("sample FASTA should not directly set parenthetical label, got %q", items[0].LabelName)
	}
	w := &BlastWizard{}
	src := keywordMapSource{
		candidates: []model.SpeciesCandidate{
			{GenomeLabel: "Arabidopsis thaliana TAIR10", JBrowseName: "Athaliana_TAIR10", SearchAlias: "Arabidopsis thaliana"},
			{GenomeLabel: "Spirodela polyrhiza", JBrowseName: "Sp7498v3", SearchAlias: "Spirodela polyrhiza"},
		},
		rowsByKeyword: map[string][]model.KeywordResultRow{
			"AT2G29130.1": {{Aliases: "LAC2; TT10", LabelName: "LAC2", AutoDefine: "laccase 2"}},
			"AT2G29130":   {{Aliases: "LAC2; TT10", LabelName: "LAC2", AutoDefine: "laccase 2"}},
		},
	}

	out, err := w.supplementBlastAliases(context.Background(), context.Background(), src, model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza"}, items[:1], nil)
	if err != nil {
		t.Fatalf("supplement aliases: %v", err)
	}
	if got := out[0].LabelName; got != "" {
		t.Fatalf("label changed to %q, want unchanged blank label", got)
	}
	if got := out[0].QuerySource.ProteinID; got != "AT2G29130.1" {
		t.Fatalf("protein id = %q, want clean FASTA id AT2G29130.1", got)
	}
	aliases := labelname.SplitAliases(out[0].QuerySource.PhgoAliases)
	found := false
	for _, alias := range aliases {
		if alias == "TT10" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("aliases = %#v, want source-species alias TT10", aliases)
	}
	member := familyBlastMemberForItem(out[0])
	found = false
	for _, alias := range member.Aliases {
		if alias == "TT10" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("family member aliases = %#v, want TT10 available in custom-group alias modal", member.Aliases)
	}
}

func TestAutoIdentifyBlastLabelResultResolvesStructuredIDsThroughGeneInfo(t *testing.T) {
	w := &BlastWizard{}
	item := blastQueryItem{QuerySource: &model.QuerySequenceSource{
		SourceDatabase: "phytozome",
		NormalizedURL:  "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT3G10340",
		GeneID:         "AT3G10340",
		TranscriptID:   "AT3G10340.1",
		ProteinID:      "PAC:19660032",
	}}
	got := w.autoIdentifyBlastLabel(context.Background(), keywordMapSource{}, model.SpeciesCandidate{}, item)
	if got != "ATPAL4" {
		t.Fatalf("unexpected symbol label: %q", got)
	}
}

func TestAutoIdentifyBlastLabelReturnsEmptyWhenStructuredIDsHaveNoDatabaseMatch(t *testing.T) {
	w := &BlastWizard{}
	item := blastQueryItem{
		RawInput: ">A.thaliana TAIR10|NO_SUCH_PROTEIN.1 (AtVND6)\nMPEPTIDE",
		QuerySource: &model.QuerySequenceSource{
			ProteinID:    "NO_SUCH_PROTEIN.1",
			TranscriptID: "NO_SUCH_TRANSCRIPT.1",
			GeneID:       "NO_SUCH_GENE",
		},
	}
	got := w.autoIdentifyBlastLabel(context.Background(), keywordMapSource{}, model.SpeciesCandidate{}, item)
	if got != "" {
		t.Fatalf("unexpected label without gene_info match: %q", got)
	}
}

func TestAutoIdentifyLemnaBlastSourceLabelUsesSymbolNameDatabaseOnly(t *testing.T) {
	src := keywordMapSource{
		name: "phytozome",
		candidates: []model.SpeciesCandidate{
			{SearchAlias: "Spirodela polyrhiza", GenomeLabel: "Spirodela polyrhiza v2", JBrowseName: "Spolyrhiza_v2"},
		},
		rowsByKeyword: map[string][]model.KeywordResultRow{
			"Sp9509d020g000340_T001": {{SourceDatabase: "phytozome", TranscriptID: "Sp9509d020g000340_T001", Synonyms: "CYP73A5; C4H"}},
		},
	}
	w := &BlastWizard{source: lemna.NewClient(nil)}
	item := blastQueryItem{
		RawInput: ">Spirodela polyrhiza|Sp9509d020g000340_T001 (HeaderName)\nMPEPTIDE",
		QuerySource: &model.QuerySequenceSource{
			SourceDatabase: "lemna",
			ProteinID:      "Sp9509d020g000340_T001",
			TranscriptID:   "Sp9509d020g000340_T001",
			LabelName:      "LOCAL_SHOULD_NOT_WIN",
		},
	}
	result := w.autoIdentifyBlastLabelResult(context.Background(), src, model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza"}, item)
	if result.Label != "CYP73A5" {
		t.Fatalf("Label = %q, want local symbol database match", result.Label)
	}
	if containsString(result.Aliases, "HeaderName") || containsString(result.Aliases, "LOCAL_SHOULD_NOT_WIN") {
		t.Fatalf("Aliases = %#v, want local symbol database aliases without header/source fallback", result.Aliases)
	}

	item.QuerySource.ProteinID = "missing"
	item.QuerySource.TranscriptID = ""
	item.QuerySource.LabelName = "C4H"
	item.RawInput = ">Spirodela polyrhiza (HeaderName)\nMPEPTIDE"
	result = w.autoIdentifyBlastLabelResult(context.Background(), src, model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza"}, item)
	if result.Label == "" || !containsString(result.Aliases, "CYP73A5") {
		t.Fatalf("local symbol database result = %#v, want gene_info alias set", result)
	}

	missingItem := blastQueryItem{
		RawInput: ">Spirodela polyrhiza|missing\nMPEPTIDE",
		QuerySource: &model.QuerySequenceSource{
			SourceDatabase: "lemna",
			ProteinID:      "missing",
		},
	}
	result = (&BlastWizard{source: lemna.NewClient(nil)}).autoIdentifyBlastLabelResult(context.Background(), src, model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza"}, missingItem)
	if result.Label != "" || len(result.Aliases) != 0 {
		t.Fatalf("ID miss result = %#v, want empty without gene_info match", result)
	}
}

func TestAutoIdentifyBlastHitLabelUsesSymbolNameDatabaseOnly(t *testing.T) {
	src := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT5G62380.1": {{SourceDatabase: "phytozome", TranscriptID: "AT5G62380.1", Synonyms: "VND6; ANAC101", Symbols: "SHOULDNOTUSE", AutoDefine: "fallback auto"}},
				"AT1G71930.1": {{SourceDatabase: "phytozome", TranscriptID: "AT1G71930.1", Symbols: "VND7", AutoDefine: "fallback auto"}},
				"AT2G00000.1": {{SourceDatabase: "phytozome", TranscriptID: "AT2G00000.1", AutoDefine: "K12345 - made up protein (AUTO1)"}},
			},
		},
	}
	w := &BlastWizard{source: src}
	rows := []model.BlastResultRow{
		{TranscriptID: "AT5G62380.1"},
		{TranscriptID: "AT1G71930.1"},
		{TranscriptID: "AT2G00000.1"},
		{TranscriptID: "AT3G00000.1"},
		{TranscriptID: "AT5G62380.1"},
	}
	got := w.autoIdentifyBlastHitLabels(context.Background(), model.SpeciesCandidate{ProteomeID: 1}, blastQueryItem{LabelName: "SOURCE1"}, rows)
	wants := []string{"VND6", "VND7", "AUTO1", "", "VND6"}
	wantTypes := []string{"symbolname database", "symbolname database", "symbolname database", "symbolname database", "symbolname database"}
	for i := range wants {
		if got[i].LabelName != wants[i] || got[i].LabelNameType != wantTypes[i] {
			t.Fatalf("row %d label/type = %q/%q, want %q/%q", i, got[i].LabelName, got[i].LabelNameType, wants[i], wantTypes[i])
		}
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.fetchCount["AT5G62380.1"] != 0 {
		t.Fatalf("duplicate hit should not trigger keyword lookup, got %d", src.fetchCount["AT5G62380.1"])
	}
}

func TestAutoIdentifyBlastHitLabelPopulatesHitPhgoAliases(t *testing.T) {
	src := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT5G62380.1": {{SourceDatabase: "phytozome", TranscriptID: "AT5G62380.1", Synonyms: "VND6; ANAC101"}},
			},
		},
	}
	w := &BlastWizard{source: src}
	got := w.autoIdentifyBlastHitLabels(
		context.Background(),
		model.SpeciesCandidate{ProteomeID: 1},
		blastQueryItem{LabelName: "SOURCE1", QuerySource: &model.QuerySequenceSource{PhgoAliases: "SOURCE1; SOURCE_ALIAS"}},
		[]model.BlastResultRow{{TranscriptID: "AT5G62380.1"}},
	)
	if got[0].LabelName == "" || got[0].PhgoAliases == "" {
		t.Fatalf("expected hit label and hit phgo aliases: %#v", got[0])
	}
	if strings.Contains(got[0].PhgoAliases, "SOURCE_ALIAS") {
		t.Fatalf("hit phgo_alias must not copy source aliases: %q", got[0].PhgoAliases)
	}
}

func TestAutoIdentifyBlastHitLabelUsesTAIRLocalSymbolDatabase(t *testing.T) {
	w := &BlastWizard{source: tair.NewClient(nil)}
	got := w.autoIdentifyBlastHitLabels(
		context.Background(),
		model.SpeciesCandidate{JBrowseName: "TAIR10", GenomeLabel: "TAIR10"},
		blastQueryItem{LabelName: "SOURCE1"},
		[]model.BlastResultRow{{
			SourceDatabase: "tair",
			Protein:        "AT2G37040.1",
			TranscriptID:   "AT2G37040.1",
			SequenceID:     "AT2G37040.1",
			Defline:        "phenylalanine ammonia-lyase protein",
		}},
	)
	if got[0].LabelName != "PAL1" {
		t.Fatalf("TAIR hit LabelName = %q, want PAL1 from local symbol database", got[0].LabelName)
	}
	if got[0].LabelNameType != "symbolname database" {
		t.Fatalf("TAIR hit LabelNameType = %q, want symbolname database", got[0].LabelNameType)
	}
	if got[0].PhgoAliases == "" || strings.Contains(got[0].PhgoAliases, "SOURCE1") {
		t.Fatalf("TAIR hit PhgoAliases = %q, want local aliases without source label", got[0].PhgoAliases)
	}
}

func TestAutoIdentifyBlastHitLabelSkipsKeywordLookupForExistingLabels(t *testing.T) {
	src := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT5G62380.1": {{SourceDatabase: "phytozome", TranscriptID: "AT5G62380.1", Synonyms: "VND6; ANAC101"}},
			},
		},
	}
	w := &BlastWizard{source: src}
	got := w.autoIdentifyBlastHitLabels(
		context.Background(),
		model.SpeciesCandidate{ProteomeID: 1},
		blastQueryItem{LabelName: "SOURCE1"},
		[]model.BlastResultRow{{TranscriptID: "AT5G62380.1", LabelName: "EXISTING"}},
	)
	if got[0].LabelName != "EXISTING" || got[0].LabelNameType != "existing row label_name" || got[0].PhgoAliases != "EXISTING" {
		t.Fatalf("existing label row changed unexpectedly: %#v", got[0])
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if len(src.fetchCount) != 0 {
		t.Fatalf("existing label row should not trigger keyword lookup, got %#v", src.fetchCount)
	}
}

func TestAutoIdentifyBlastHitLabelReusesDuplicateHitIdentification(t *testing.T) {
	src := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT5G62380.1": {{SourceDatabase: "phytozome", TranscriptID: "AT5G62380.1", Synonyms: "VND6; ANAC101"}},
			},
		},
	}
	w := &BlastWizard{source: src}
	rows := []model.BlastResultRow{
		{TranscriptID: "AT5G62380.1", Protein: "AT5G62380.1", HSPNumber: 1},
		{TranscriptID: "AT5G62380.1", Protein: "AT5G62380.1", HSPNumber: 2},
	}
	got := w.autoIdentifyBlastHitLabels(context.Background(), model.SpeciesCandidate{ProteomeID: 1}, blastQueryItem{LabelName: "SOURCE1"}, rows)
	if got[0].LabelName == "" || got[0].LabelName != got[1].LabelName || got[0].PhgoAliases != got[1].PhgoAliases {
		t.Fatalf("duplicate hits should reuse identical identification: %#v", got)
	}
	if blastHitLabelIdentificationCacheKey(got[0], "SOURCE1") != blastHitLabelIdentificationCacheKey(got[1], "SOURCE1") {
		t.Fatalf("duplicate hit cache keys should match: %q vs %q", blastHitLabelIdentificationCacheKey(got[0], "SOURCE1"), blastHitLabelIdentificationCacheKey(got[1], "SOURCE1"))
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.fetchCount["AT5G62380.1"] != 0 {
		t.Fatalf("duplicate hit should not trigger keyword lookup, got %d", src.fetchCount["AT5G62380.1"])
	}
}

func TestAutoIdentifyBlastHitLabelUsesSymbolnameWithoutSourceRowLookup(t *testing.T) {
	src := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT5G62380.1": {{SourceDatabase: "phytozome", TranscriptID: "AT5G62380.1", Synonyms: "VND6; ANAC101"}},
			},
		},
	}
	w := &BlastWizard{
		source:                   src,
		blastHitLabelLookupCache: make(map[string]blastHitLabelIdentification),
	}
	rows := []model.BlastResultRow{{TranscriptID: "AT5G62380.1", Protein: "AT5G62380.1"}}
	first := w.autoIdentifyBlastHitLabels(context.Background(), model.SpeciesCandidate{ProteomeID: 1}, blastQueryItem{LabelName: "SOURCE1"}, rows)
	second := w.autoIdentifyBlastHitLabels(context.Background(), model.SpeciesCandidate{ProteomeID: 1}, blastQueryItem{LabelName: "SOURCE1"}, rows)
	if first[0].LabelName != "VND6" || second[0].LabelName != "VND6" {
		t.Fatalf("cached hit label = %q/%q, want VND6", first[0].LabelName, second[0].LabelName)
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.fetchCount["AT5G62380.1"] != 0 {
		t.Fatalf("cross-call hit lookup count = %d, want 0", src.fetchCount["AT5G62380.1"])
	}
}

func TestAutoIdentifyLemnaBlastHitLabelFallsBackToLocalBeforeSourceLabel(t *testing.T) {
	w := &BlastWizard{source: lemna.NewClient(nil)}
	got := w.autoIdentifyBlastHitLabels(
		context.Background(),
		model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza"},
		blastQueryItem{LabelName: "SOURCE1"},
		[]model.BlastResultRow{{
			SourceDatabase: "lemna",
			Protein:        "Sp9509d020g000340_T001",
			Defline:        "cinnamate 4-hydroxylase (C4H)",
		}},
	)
	if got[0].LabelName != "C4H" && got[0].LabelName != "REF3" && got[0].LabelName != "CYP73A5" {
		t.Fatalf("LabelName = %q, want gene_info symbol-name candidate", got[0].LabelName)
	}
	if got[0].LabelNameType != "symbolname database" {
		t.Fatalf("LabelNameType = %q, want symbolname database", got[0].LabelNameType)
	}
	if got[0].PhgoAliases == "" || strings.Contains(got[0].PhgoAliases, "SOURCE1") {
		t.Fatalf("PhgoAliases = %q, want local hit aliases without source label", got[0].PhgoAliases)
	}
}

func TestAutoIdentifyLemnaBlastHitLabelSplitsWhitespaceAliasList(t *testing.T) {
	w := &BlastWizard{source: lemna.NewClient(nil)}
	got := w.autoIdentifyBlastHitLabels(
		context.Background(),
		model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza"},
		blastQueryItem{LabelName: "SOURCE1"},
		[]model.BlastResultRow{{
			SourceDatabase:   "lemna",
			Protein:          "Sp9509d008g014760_T001",
			UniProtGeneNames: "4CLL4 Os03g0132000 LOC_Os03g04000 OsJ_09299",
			Defline:          "4-coumarate--CoA ligase-like 4",
		}},
	)
	if got[0].LabelName != "4CLL4" {
		t.Fatalf("LabelName = %q, want first split alias 4CLL4", got[0].LabelName)
	}
	if got[0].LabelNameType != "symbolname database" {
		t.Fatalf("LabelNameType = %q, want symbolname database", got[0].LabelNameType)
	}
	if strings.Contains(got[0].PhgoAliases, "4CLL4 Os03g0132000") {
		t.Fatalf("PhgoAliases kept whitespace list as one alias: %q", got[0].PhgoAliases)
	}
	if !strings.HasPrefix(got[0].PhgoAliases, "4CLL4") || strings.Contains(got[0].PhgoAliases, "4-coumarate") {
		t.Fatalf("PhgoAliases = %q, want uppercase/digit aliases first without long text first", got[0].PhgoAliases)
	}
}

func TestAutoIdentifyLemnaBlastHitLabelUsesHitKeywordRowsBeforeSourceFallback(t *testing.T) {
	w := &BlastWizard{source: fakeSource{
		name: "lemna",
		keywordRows: []model.KeywordResultRow{{
			SourceDatabase: "lemna",
			LabelName:      "C4H",
			Aliases:        "C4H; CYP73A5",
			TranscriptID:   "Sp9509d020g000340_T001",
			SequenceID:     "Sp9509d020g000340_T001",
		}},
	}}
	got := w.autoIdentifyBlastHitLabels(
		context.Background(),
		model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza"},
		blastQueryItem{LabelName: "SOURCE1"},
		[]model.BlastResultRow{{
			SourceDatabase: "lemna",
			Protein:        "Sp9509d020g000340_T001",
			SubjectID:      "Sp9509d020g000340_T001",
		}},
	)
	if got[0].LabelName == "" || got[0].LabelName == "SOURCE1" {
		t.Fatalf("LabelName = %q, want Lemna hit keyword label instead of source fallback", got[0].LabelName)
	}
	if got[0].LabelNameType != "symbolname database" {
		t.Fatalf("LabelNameType = %q, want symbolname database", got[0].LabelNameType)
	}
	if got[0].PhgoAliases == "" || strings.Contains(got[0].PhgoAliases, "SOURCE1") {
		t.Fatalf("PhgoAliases = %q, want hit aliases without source fallback", got[0].PhgoAliases)
	}
	if !strings.Contains(got[0].PhgoAliases, "CYP73A5") {
		t.Fatalf("PhgoAliases = %q, want keyword-row aliases", got[0].PhgoAliases)
	}
}

func TestAutoIdentifyLemnaBlastHitLabelUsesSourceLabelLast(t *testing.T) {
	w := &BlastWizard{source: lemna.NewClient(nil)}
	got := w.autoIdentifyBlastHitLabels(
		context.Background(),
		model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza"},
		blastQueryItem{LabelName: "SOURCE1"},
		[]model.BlastResultRow{{SourceDatabase: "lemna", Protein: "Sp9509d020g000340_T001"}},
	)
	if got[0].LabelName == "SOURCE1" || got[0].LabelNameType != "symbolname database" {
		t.Fatalf("got label/type = %q/%q, want symbolname database result without source fallback", got[0].LabelName, got[0].LabelNameType)
	}
	if strings.Contains(got[0].PhgoAliases, "SOURCE1") {
		t.Fatalf("PhgoAliases = %q, want no source fallback alias", got[0].PhgoAliases)
	}
}

func TestPrepareBlastExportItemRequiresExistingSourceLabel(t *testing.T) {
	w := &BlastWizard{}
	item := blastQueryItem{QuerySource: &model.QuerySequenceSource{
		GeneID:       "AT3G10340",
		TranscriptID: "AT3G10340.1",
		ProteinID:    "PAC:19660032",
	}}
	if _, err := w.prepareBlastExportItem(item, false); err == nil {
		t.Fatalf("prepareBlastExportItem should reject missing source label")
	}
}

func TestAutoIdentifyBlastLabelDoesNotFallbackForPlainProteinSequence(t *testing.T) {
	w := &BlastWizard{}
	item := blastQueryItem{RawInput: "MPEPTIDERAWSEQ", Sequence: "MPEPTIDERAWSEQ"}
	got := w.autoIdentifyBlastLabel(context.Background(), keywordMapSource{}, model.SpeciesCandidate{}, item)
	if got != "" {
		t.Fatalf("plain protein sequence should not auto identify label, got %q", got)
	}
}

func TestSupplementBlastAliasesUsesBatchRankedAliases(t *testing.T) {
	lookupSource := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT2G30490.1": {{SourceDatabase: "phytozome", TranscriptID: "AT2G30490.1", Synonyms: "CYP73A5; REF3", Symbols: "C4H"}},
				"AT5G13930.1": {{SourceDatabase: "phytozome", TranscriptID: "AT5G13930.1", Synonyms: "PAL1; ATPAL1", Symbols: "PAL1"}},
			},
		},
	}
	w := &BlastWizard{}
	items := []blastQueryItem{
		{QuerySource: &model.QuerySequenceSource{SourceDatabase: "phytozome", SourceProteomeID: 167, TranscriptID: "AT2G30490.1", ProteinID: "AT2G30490.1"}},
		{QuerySource: &model.QuerySequenceSource{SourceDatabase: "phytozome", SourceProteomeID: 167, TranscriptID: "AT5G13930.1", ProteinID: "AT5G13930.1"}},
	}
	out, err := w.supplementBlastAliases(context.Background(), context.Background(), lookupSource, model.SpeciesCandidate{ProteomeID: 167}, items, nil)
	if err != nil {
		t.Fatalf("supplementBlastAliases returned error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("labeled items = %d, want 2", len(out))
	}
	if strings.TrimSpace(out[0].QuerySource.PhgoAliases) == "" {
		t.Fatalf("first item missing label or aliases: %#v", out[0])
	}
	if strings.TrimSpace(out[1].QuerySource.PhgoAliases) == "" {
		t.Fatalf("second item missing label or aliases: %#v", out[1])
	}
	if lookupSource.fetchCount["AT2G30490.1"] != 0 || lookupSource.fetchCount["AT5G13930.1"] != 0 {
		t.Fatalf("unexpected lookup counts: %#v", lookupSource.fetchCount)
	}
}

func TestHarmonizeAutoIdentifiedBlastLabelsPreservesOrImprovesFamilyGrouping(t *testing.T) {
	items := []blastQueryItem{
		{LabelName: "IRX5", QuerySource: &model.QuerySequenceSource{LabelName: "IRX5", Aliases: "CESA4; IRX5; NWS2"}},
		{LabelName: "IRX3", QuerySource: &model.QuerySequenceSource{LabelName: "IRX3", Aliases: "ATCESA7; CESA7; IRX3; MUR10"}},
		{LabelName: "IRX1", QuerySource: &model.QuerySequenceSource{LabelName: "IRX1", Aliases: "ATCESA8; CESA8; IRX1; LEW2"}},
		{LabelName: "GUT2", QuerySource: &model.QuerySequenceSource{LabelName: "GUT2", Aliases: "ATGUT1; GUT2; IRX10", AutoDefine: "IRX10"}},
		{LabelName: "GUT1", QuerySource: &model.QuerySequenceSource{LabelName: "GUT1", Aliases: "GUT1; IRX10-L; XYS1", AutoDefine: "IRX10-like"}},
	}

	out := harmonizeAutoIdentifiedBlastLabels(items)
	settings := model.DefaultFamilyBlastSettings()
	before := detectFamilyBlastGroups(items, settings)
	after := detectFamilyBlastGroups(out, settings)
	if len(after) < len(before) {
		t.Fatalf("family grouping regressed: before=%v after=%v", before, after)
	}
	if out[3].LabelName == "" || out[4].LabelName == "" {
		t.Fatalf("expected harmonized labels to stay populated: %#v", out)
	}
}

func TestHarmonizeAutoIdentifiedBlastLabelsRetainsCompactFunctionalCandidates(t *testing.T) {
	items := []blastQueryItem{
		{LabelName: "REF8", QuerySource: &model.QuerySequenceSource{LabelName: "REF8", Aliases: "CYP98A3; REF8", AutoDefine: "C3'H"}},
		{LabelName: "FAH1", QuerySource: &model.QuerySequenceSource{LabelName: "FAH1", Aliases: "CYP84A1; FAH1", AutoDefine: "F5H1"}},
	}

	out := harmonizeAutoIdentifiedBlastLabels(items)
	candidates0 := blastAutoLabelCandidates(items[0])
	candidates1 := blastAutoLabelCandidates(items[1])
	if !containsString(candidates0, out[0].LabelName) {
		t.Fatalf("first harmonized label=%q not in candidates=%v", out[0].LabelName, candidates0)
	}
	if !containsString(candidates1, out[1].LabelName) {
		t.Fatalf("second harmonized label=%q not in candidates=%v", out[1].LabelName, candidates1)
	}
}

func TestHarmonizeAutoIdentifiedBlastLabelsWithLocksKeepsPreexistingLabels(t *testing.T) {
	items := []blastQueryItem{
		{LabelName: "HeaderName", QuerySource: &model.QuerySequenceSource{LabelName: "HeaderName", Aliases: "VND6; ANAC101"}},
		{LabelName: "VND7", QuerySource: &model.QuerySequenceSource{LabelName: "VND7", Aliases: "ANAC030; VND7"}},
	}

	out := harmonizeAutoIdentifiedBlastLabelsWithLocks(items, []bool{true, false})
	if out[0].LabelName != "HeaderName" {
		t.Fatalf("locked label changed to %q, want HeaderName", out[0].LabelName)
	}
}

func TestApplyUniProtEntryPopulatesReferenceColumns(t *testing.T) {
	row := model.BlastResultRow{TargetLength: 329}
	applyUniProtEntry(&row, uniprot.Entry{
		Accession:   "Q43158",
		Reviewed:    "unreviewed",
		ProteinName: "Peroxidase (EC 1.11.1.7)",
		EC:          "1.11.1.7",
		GO:          "heme binding [GO:0020037]",
		Length:      329,
	})
	if row.UniProtAccession != "Q43158" || row.UniProtEC != "1.11.1.7" || row.UniProtCanonicalLength != "329" {
		t.Fatalf("unexpected UniProt row: %#v", row)
	}
	if row.TargetUniProtCanonicalLengthPercent != "100.00" {
		t.Fatalf("unexpected canonical length percent: %q", row.TargetUniProtCanonicalLengthPercent)
	}
}

func TestAnnotateFamilyBlastConsensusRowsUsesPrecomputedSemanticTokenList(t *testing.T) {
	rows := []model.BlastResultRow{{
		BlastLabelName:                "C4H1",
		Protein:                       "prot1",
		UniProtProteinName:            "cinnamate 4 hydroxylase",
		UniProtKeywords:               "phenylpropanoid",
		InterProEntryName:             "Cytochrome P450",
		InterProCoveragePercent:       "88.0",
		InterProConservedRegionStatus: "present",
	}}
	out := annotateFamilyBlastConsensusRows(rows, "C4H", []string{"C4H1", "C4H2"}, []string{"cinnamate 4-hydroxylase"})
	if len(out) != 1 {
		t.Fatalf("annotated row count = %d, want 1", len(out))
	}
	if out[0].FamilySemanticAnnotationMatchCount == 0 {
		t.Fatalf("expected semantic match evidence, got %#v", out[0])
	}
	if strings.TrimSpace(out[0].FamilySemanticAnnotationMatchTokens) == "" {
		t.Fatalf("expected semantic match tokens, got %#v", out[0])
	}
}

func TestPrioritizeFamilyBlastRowsMatchesPairwiseComparator(t *testing.T) {
	settings := model.DefaultFamilyBlastSettings()
	settings.UseUniProtReference = true
	settings.UseInterProReference = true
	rows := []model.BlastResultRow{
		{
			Protein:                             "protA",
			UniProtAccession:                    "Q1",
			UniProtReviewed:                     "reviewed",
			InterProConservedRegionStatus:       "present",
			InterProCoveragePercent:             "85",
			TargetUniProtCanonicalLengthPercent: "101",
			PercentIdentity:                     55,
			AlignLength:                         300,
			QueryLength:                         320,
			Bitscore:                            250,
			EValue:                              "1e-50",
		},
		{
			Protein:                             "protB",
			UniProtAccession:                    "Q2",
			UniProtReviewed:                     "unreviewed",
			InterProConservedRegionStatus:       "partial",
			InterProCoveragePercent:             "42",
			TargetUniProtCanonicalLengthPercent: "145",
			PercentIdentity:                     48,
			AlignLength:                         280,
			QueryLength:                         320,
			Bitscore:                            220,
			EValue:                              "1e-30",
		},
	}
	out := prioritizeFamilyBlastRows(rows, settings)
	if len(out) != 2 {
		t.Fatalf("prioritized row count = %d, want 2", len(out))
	}
	if !familyBlastRowLess(out[0], out[1], settings) {
		t.Fatalf("sorted order must still satisfy comparator: %#v", out)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestLabelnameColumnMatrixCoversBothDatabasesAndModes(t *testing.T) {
	for _, database := range []string{"phytozome", "lemna"} {
		display := prompt.KeywordDisplayColumnIDs(database)
		requireColumnsInOrder(t, database+" keyword display", display, []string{"label_name", "labelname_type", "phgo_alias"})
		if database == "phytozome" {
			rejectColumns(t, "phytozome keyword display", display, []string{"alias", "aliases", "symbols", "synonyms"})
		}
		detail := prompt.KeywordDetailColumnIDs(database)
		exportIDs := prompt.KeywordExportColumnIDs(database, true, nil)
		requireColumnsInOrder(t, database+" keyword detail", detail, []string{"label_name", "labelname_type", "phgo_alias"})
		requireColumnsInOrder(t, database+" keyword export", exportIDs, []string{"label_name", "labelname_type", "phgo_alias"})
	}

	for _, database := range []string{"phytozome", "lemna"} {
		for _, program := range []string{"BLASTN", "BLASTX", "TBLASTN", "BLASTP"} {
			display := prompt.BlastDisplayColumnIDs(database, program, true, true)
			requireColumnsInOrder(t, database+" "+program+" blast display", display, []string{"label_name", "labelname_type", "phgo_alias", "protein", "blast_labelname", "blast_geneid"})
			requireColumns(t, database+" "+program+" blast display references", display, []string{"uniprot_accession", "interpro_entry_type"})
			detail := prompt.BlastDetailColumnIDs(database, program, true, true)
			exportIDs := prompt.BlastExportColumnIDs(database, true, true)
			requireColumnsInOrder(t, database+" "+program+" blast detail", detail, []string{"label_name", "labelname_type", "phgo_alias", "protein", "blast_labelname", "blast_geneid"})
			requireColumnsInOrder(t, database+" "+program+" blast export", exportIDs, []string{"label_name", "labelname_type", "phgo_alias", "protein", "blast_labelname", "blast_geneid"})
		}
	}
}

func TestBlastReportLineageDocumentsHitPhgoAliasAndQuerySourceColumns(t *testing.T) {
	rows := []model.BlastResultRow{{
		SourceDatabase:                "lemna",
		BlastProgram:                  "BLASTP",
		LabelName:                     "C4H",
		LabelNameType:                 "symbolname database",
		PhgoAliases:                   "C4H; CYP73A5",
		BlastLabelName:                "PAL1",
		BlastGeneID:                   "Sp9509d011g001470",
		Protein:                       "Sp9509d020g000340_T001",
		UniProtReferenceEnabled:       true,
		UniProtAccession:              "Q00001",
		InterProReferenceEnabled:      true,
		InterProConservedRegionStatus: "present",
	}}
	lineage := blastColumnLineage(rows, "lemna", "BLASTP", true, true)
	phgo := findColumnLineage(lineage, "phgo_alias")
	if phgo == nil {
		t.Fatal("phgo_alias lineage missing")
	}
	if phgo.Source != "symbol name system" {
		t.Fatalf("phgo_alias source = %q, want symbol name system", phgo.Source)
	}
	if !strings.Contains(phgo.Meaning, "BLAST hit row") || !strings.Contains(phgo.CollectionMethod, "BLAST-hit symbol name") {
		t.Fatalf("phgo_alias lineage does not describe hit-level aliases: %#v", *phgo)
	}
	if phgo.UsedInStats != "traceability" {
		t.Fatalf("phgo_alias UsedInStats = %q, want traceability", phgo.UsedInStats)
	}
	for _, id := range []string{"blast_labelname", "blast_geneid"} {
		col := findColumnLineage(lineage, id)
		if col == nil {
			t.Fatalf("%s lineage missing", id)
		}
		if col.Source != "BLAST query source" {
			t.Fatalf("%s source = %q, want BLAST query source", id, col.Source)
		}
	}
}

func TestBlastFullReferenceAutoLabelSimulationKeepsHitAliasesSeparateFromSourceGrouping(t *testing.T) {
	src := keywordMapSource{
		name: "phytozome",
		rowsByKeyword: map[string][]model.KeywordResultRow{
			"AT5G62380.1": {{SourceDatabase: "phytozome", TranscriptID: "AT5G62380.1", Synonyms: "VND6; ANAC101"}},
			"AT1G71930.1": {{SourceDatabase: "phytozome", TranscriptID: "AT1G71930.1", Symbols: "VND7"}},
		},
	}
	w := &BlastWizard{source: src}
	item := blastQueryItem{
		LabelName: "PAL1",
		QuerySource: &model.QuerySequenceSource{
			SourceDatabase:    "phytozome",
			LabelName:         "PAL1",
			PhgoAliases:       "PAL1; ATPAL1",
			GeneID:            "AT2G37040",
			ProteinID:         "AT2G37040.1",
			SourceJBrowseName: "Athaliana_TAIR10",
			SourceGenomeLabel: "Arabidopsis thaliana TAIR10",
			Sequence:          "MPEPTIDE",
		},
	}
	rows := []model.BlastResultRow{
		{
			SourceDatabase:                      "phytozome",
			BlastProgram:                        "BLASTP",
			Protein:                             "AT5G62380.1",
			TranscriptID:                        "AT5G62380.1",
			EValue:                              "1e-50",
			PercentIdentity:                     80,
			UniProtReferenceEnabled:             true,
			UniProtAccession:                    "Q9SZZ8",
			UniProtReviewed:                     "reviewed",
			UniProtProteinName:                  "VND6 protein",
			UniProtGeneNames:                    "VND6 ANAC101",
			UniProtCanonicalLength:              "320",
			TargetUniProtCanonicalLengthPercent: "100.00",
			InterProReferenceEnabled:            true,
			InterProConservedRegionStatus:       "present",
			InterProEntryType:                   "family",
			InterProCoveragePercent:             "95.00",
		},
		{
			SourceDatabase:                "phytozome",
			BlastProgram:                  "BLASTP",
			Protein:                       "AT1G71930.1",
			TranscriptID:                  "AT1G71930.1",
			EValue:                        "1e-40",
			PercentIdentity:               70,
			UniProtReferenceEnabled:       true,
			UniProtAccession:              "Q9M000",
			InterProReferenceEnabled:      true,
			InterProConservedRegionStatus: "partial",
		},
	}
	rows = prepareBlastRowsForReferences(rows, item, model.BlastRequest{
		Species:      model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10"},
		Sequence:     "MPEPTIDE",
		Program:      "BLASTP",
		SequenceKind: model.SequenceProtein,
	}, "phytozome")
	rows = w.autoIdentifyBlastHitLabels(context.Background(), model.SpeciesCandidate{ProteomeID: 167}, item, rows)
	rows = annotateBlastRowsForQueryContext(rows, item)

	if rows[0].LabelName != "VND6" || rows[0].LabelNameType != "symbolname database" {
		t.Fatalf("first hit label/type = %q/%q, want local symbolname result", rows[0].LabelName, rows[0].LabelNameType)
	}
	if rows[0].PhgoAliases == "" || strings.Contains(rows[0].PhgoAliases, "ATPAL1") {
		t.Fatalf("first hit phgo_alias = %q, want hit aliases without query-source aliases", rows[0].PhgoAliases)
	}
	for i, row := range rows {
		if row.BlastLabelName != "PAL1" || row.BlastGeneID != "AT2G37040" {
			t.Fatalf("row %d query source columns = %q/%q, want PAL1/AT2G37040", i, row.BlastLabelName, row.BlastGeneID)
		}
		if !row.UniProtReferenceEnabled || !row.InterProReferenceEnabled {
			t.Fatalf("row %d reference flags lost after auto-label: %#v", i, row)
		}
	}
	plan := &familyBlastPlan{
		Settings: model.DefaultFamilyBlastSettings(),
		Groups:   []familyBlastGroup{{Name: "PAL", Indexes: []int{0}, Labels: []string{"PAL1"}}},
	}
	_, merged := applyFamilyBlastPlan([]blastQueryItem{item}, []blastQueryRun{{Index: 1, Item: item, Results: model.BlastResult{Rows: rows}}}, plan)
	if len(merged) != 1 || len(merged[0].Results.Rows) != 2 {
		t.Fatalf("unexpected family merge output: %#v", merged)
	}
	for _, row := range merged[0].Results.Rows {
		if row.BlastLabelName != "PAL1" || row.LabelName == "PAL1" {
			t.Fatalf("family grouping should use source label without overwriting hit label: %#v", row)
		}
	}
}

func TestOfflineWorkflowMatrixTwoDatabasesTwoModesWithAutoLabelsAndReferences(t *testing.T) {
	phySpecies := model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis thaliana TAIR10", SearchAlias: "Arabidopsis thaliana"}
	lemSpecies := model.SpeciesCandidate{ProteomeID: 18, JBrowseName: "Sp_polyrhiza_9509", GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0", SearchAlias: "Spirodela polyrhiza"}

	keywordCases := []struct {
		name     string
		database string
		species  model.SpeciesCandidate
		groups   []model.KeywordSearchGroup
		lookup   source.DataSource
	}{
		{
			name:     "phytozome-keyword",
			database: "phytozome",
			species:  phySpecies,
			groups: []model.KeywordSearchGroup{{
				SearchTerm: "PAL1",
				Rows: []model.KeywordResultRow{{
					SourceDatabase: "phytozome",
					SearchTerm:     "PAL1",
					Synonyms:       "PAL1; ATPAL1",
					Symbols:        "SHOULD_NOT_WIN",
					AutoDefine:     "phenylalanine ammonia-lyase",
					ProteinID:      "AT2G37040.1",
					TranscriptID:   "AT2G37040.1",
					GeneIdentifier: "AT2G37040",
					SequenceID:     "AT2G37040.1",
				}},
			}},
		},
		{
			name:     "lemna-keyword",
			database: "lemna",
			species:  lemSpecies,
			groups: []model.KeywordSearchGroup{{
				SearchTerm: "Sp9509d020g000340_T001",
				Rows: []model.KeywordResultRow{{
					SourceDatabase: "lemna",
					LabelName:      "LOCAL_SHOULD_NOT_WIN",
					ProteinID:      "Sp9509d020g000340_T001",
					TranscriptID:   "Sp9509d020g000340_T001",
					GeneIdentifier: "Sp9509d020g000340",
					SequenceID:     "Sp9509d020g000340_T001",
					Aliases:        "LOCAL_ALIAS",
				}},
			}},
			lookup: keywordMapSource{
				name: "phytozome",
				rowsByKeyword: map[string][]model.KeywordResultRow{
					"Sp9509d020g000340_T001": {{SourceDatabase: "phytozome", TranscriptID: "Sp9509d020g000340_T001", Synonyms: "C4H; CYP73A5"}},
				},
			},
		},
	}
	for _, tc := range keywordCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			groups := cloneKeywordSearchGroups(tc.groups)
			var ids []keywordLabelIdentification
			if tc.database == "lemna" {
				w := &BlastWizard{source: lemna.NewClient(nil)}
				ids = w.autoIdentifyLemnaKeywordLabels(context.Background(), tc.species, groups, tc.lookup)
			} else {
				ids = autoIdentifyKeywordLabelIdentifications(groups)
			}
			annotateKeywordLabelSources(groups, ids, "auto identify labelname")
			applyKeywordLabelIdentifications(groups, ids)
			if len(groups) == 0 || len(groups[0].Rows) == 0 {
				t.Fatal("keyword matrix fixture has no rows")
			}
			row := groups[0].Rows[0]
			if row.LabelName == "" || row.LabelNameType == "" || row.PhgoAliases == "" {
				t.Fatalf("keyword auto label incomplete: %#v", row)
			}
			display := prompt.KeywordDisplayColumnIDs(tc.database)
			requireColumnsInOrder(t, tc.name+" display", display, []string{"label_name", "labelname_type", "phgo_alias"})
			if tc.database == "phytozome" {
				rejectColumns(t, tc.name+" display", display, []string{"symbols", "synonyms", "alias"})
			}
		})
	}

	blastCases := []struct {
		name        string
		database    string
		species     model.SpeciesCandidate
		source      source.DataSource
		item        blastQueryItem
		rows        []model.BlastResultRow
		wantHitType string
	}{
		{
			name:     "phytozome-blast",
			database: "phytozome",
			species:  phySpecies,
			source: keywordMapSource{
				name: "phytozome",
				rowsByKeyword: map[string][]model.KeywordResultRow{
					"AT5G62380.1": {{SourceDatabase: "phytozome", TranscriptID: "AT5G62380.1", Synonyms: "VND6; ANAC101"}},
				},
			},
			item: blastQueryItem{LabelName: "PAL1", QuerySource: &model.QuerySequenceSource{
				SourceDatabase: "phytozome", SourceProteomeID: 167, SourceJBrowseName: "Athaliana_TAIR10", SourceGenomeLabel: "Arabidopsis thaliana TAIR10",
				LabelName: "PAL1", PhgoAliases: "PAL1; ATPAL1", GeneID: "AT2G37040", ProteinID: "AT2G37040.1", Sequence: "MPEPTIDE",
			}},
			rows:        []model.BlastResultRow{{SourceDatabase: "phytozome", BlastProgram: "BLASTP", Protein: "AT5G62380.1", TranscriptID: "AT5G62380.1", SequenceID: "AT5G62380.1", TargetLength: 320}},
			wantHitType: "symbolname database",
		},
		{
			name:     "lemna-blast",
			database: "lemna",
			species:  lemSpecies,
			source:   lemna.NewClient(nil),
			item: blastQueryItem{LabelName: "SOURCE_C4H", QuerySource: &model.QuerySequenceSource{
				SourceDatabase: "lemna", SourceProteomeID: 18, SourceJBrowseName: "Sp_polyrhiza_9509", SourceGenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
				LabelName: "SOURCE_C4H", PhgoAliases: "SOURCE_C4H; CYP73A5", GeneID: "Sp9509d020g000340", ProteinID: "Sp9509d020g000340_T001", Sequence: "MPEPTIDE",
			}},
			rows:        []model.BlastResultRow{{SourceDatabase: "lemna", BlastProgram: "BLASTP", Protein: "Sp9509d020g000340_T001", TranscriptID: "Sp9509d020g000340_T001", Defline: "cinnamate 4-hydroxylase (C4H)", TargetLength: 505}},
			wantHitType: "symbolname database",
		},
	}
	for _, tc := range blastCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := &BlastWizard{source: tc.source, suppressTaskModals: true}
			rows := prepareBlastRowsForReferences(tc.rows, tc.item, model.BlastRequest{
				Species:      tc.species,
				Sequence:     "MPEPTIDE",
				Program:      "BLASTP",
				SequenceKind: model.SequenceProtein,
			}, tc.database)
			for i := range rows {
				rows[i].UniProtReferenceEnabled = true
				rows[i].UniProtAccession = "Q00001"
				rows[i].UniProtReviewed = "reviewed"
				rows[i].UniProtProteinName = "reference protein"
				rows[i].UniProtGeneNames = "REF1"
				rows[i].InterProReferenceEnabled = true
				rows[i].InterProConservedRegionStatus = "present"
				rows[i].InterProEntryType = "family"
				rows[i].InterProCoveragePercent = "95.00"
			}
			rows = w.autoIdentifyBlastHitLabels(context.Background(), tc.species, tc.item, rows)
			rows = annotateBlastRowsForQueryContext(rows, tc.item)
			if rows[0].LabelName == "" || rows[0].LabelNameType != tc.wantHitType || rows[0].PhgoAliases == "" {
				t.Fatalf("blast hit auto label incomplete: %#v", rows[0])
			}
			if rows[0].BlastLabelName != blastQueryItemLabelName(tc.item) || rows[0].BlastGeneID != blastQueryItemID2(tc.item) {
				t.Fatalf("blast source columns not preserved: %#v", rows[0])
			}
			if !rows[0].UniProtReferenceEnabled || !rows[0].InterProReferenceEnabled {
				t.Fatalf("external reference flags lost: %#v", rows[0])
			}
			display := prompt.BlastDisplayColumnIDs(tc.database, "BLASTP", true, true)
			requireColumnsInOrder(t, tc.name+" display", display, []string{"label_name", "labelname_type", "phgo_alias", "protein", "blast_labelname", "blast_geneid"})
			requireColumns(t, tc.name+" references", display, []string{"uniprot_accession", "interpro_entry_type"})
			lineage := blastColumnLineage(rows, tc.database, "BLASTP", true, true)
			if phgo := findColumnLineage(lineage, "phgo_alias"); phgo == nil || phgo.Source != "symbol name system" {
				t.Fatalf("phgo_alias lineage missing or wrong: %#v", phgo)
			}
			metadata := buildExportMetadata(blastQueryItemLabelName(tc.item), tc.item.QuerySource)
			if metadata == nil || len(metadata.Queries) != 1 || metadata.Queries[0].LabelName == "" {
				t.Fatalf("query metadata missing source label: %#v", metadata)
			}
		})
	}
}

func requireColumns(t *testing.T, context string, got []string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if columnIndex(got, want) < 0 {
			t.Fatalf("%s missing column %q in %#v", context, want, got)
		}
	}
}

func requireColumnsInOrder(t *testing.T, context string, got []string, wants []string) {
	t.Helper()
	last := -1
	for _, want := range wants {
		idx := columnIndex(got, want)
		if idx < 0 {
			t.Fatalf("%s missing column %q in %#v", context, want, got)
		}
		if idx <= last {
			t.Fatalf("%s column %q index=%d should be after previous index=%d in %#v", context, want, idx, last, got)
		}
		last = idx
	}
}

func rejectColumns(t *testing.T, context string, got []string, rejects []string) {
	t.Helper()
	for _, reject := range rejects {
		if columnIndex(got, reject) >= 0 {
			t.Fatalf("%s should not display column %q in %#v", context, reject, got)
		}
	}
}

func columnIndex(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}

func findColumnLineage(lineage []report.ColumnLineage, id string) *report.ColumnLineage {
	for i := range lineage {
		if lineage[i].ID == id {
			return &lineage[i]
		}
	}
	return nil
}

func TestUniProtLookupGroupsDeduplicateEquivalentRows(t *testing.T) {
	rows := []model.BlastResultRow{
		{Protein: "Spipo15G0028500", SubjectID: "Spipo15G0028500", Species: "Spirodela polyrhiza"},
		{Protein: "Spipo15G0028500", SubjectID: "Spipo15G0028500", Species: "Spirodela polyrhiza"},
		{Protein: "Spipo11G0031600", SubjectID: "Spipo11G0031600", Species: "Spirodela polyrhiza"},
	}
	groups := uniProtLookupGroups(rows)
	if len(groups) != 2 {
		t.Fatalf("expected 2 lookup groups, got %#v", groups)
	}
	if len(groups[0].Rows) != 2 {
		t.Fatalf("first group should contain duplicate rows: %#v", groups)
	}
}

func TestBlastNetworkWorkerLimitsStayConservativeForSmallBatches(t *testing.T) {
	cfg := externalReferenceConfig{
		AutoLabelBlastHits: true,
		UseUniProt:         true,
		UseInterPro:        true,
		InterProSettings:   model.DefaultInterProConservedRegionSettings(),
	}
	if got := blastUniProtWorkerCountForConfig(3, cfg); got > 12 {
		t.Fatalf("small-batch UniProt workers = %d, want <= 12", got)
	}
	if got := blastUniProtAccessionWorkerCountForConfig(3, cfg); got > 16 {
		t.Fatalf("small-batch UniProt accession workers = %d, want <= 16", got)
	}
	if got := blastInterProWorkerCountForConfig(3, cfg); got > 12 {
		t.Fatalf("small-batch InterPro workers = %d, want <= 12", got)
	}
}

func TestSearchKeywordGroupsAppliesBlankManualLabels(t *testing.T) {
	w := &BlastWizard{source: fakeSource{
		keywordRows: []model.KeywordResultRow{{
			LabelName:    "PAL",
			TranscriptID: "AT2G37040.1",
		}},
	}}

	groups, err := w.searchKeywordGroupsWithProgress(context.Background(), model.SpeciesCandidate{}, []string{"PAL"}, []string{""}, false, nil)
	if err != nil {
		t.Fatalf("searchKeywordGroupsWithProgress returned error: %v", err)
	}
	if len(groups) != 1 || len(groups[0].Rows) != 1 {
		t.Fatalf("unexpected groups: %#v", groups)
	}
	if groups[0].LabelName != "" || groups[0].Rows[0].LabelName != "" {
		t.Fatalf("blank manual label should clear group and row labels: %#v", groups)
	}
}

func TestSearchKeywordGroupsCanForceWideSearch(t *testing.T) {
	source := fakeWideKeywordSource{
		normalRows: []model.KeywordResultRow{{
			SearchType:   "keyword",
			TranscriptID: "normal.1",
		}},
		wideRows: []model.KeywordResultRow{{
			TranscriptID: "wide.1",
		}},
	}
	w := &BlastWizard{source: source}

	groups, err := w.searchKeywordGroupsWithProgress(context.Background(), model.SpeciesCandidate{}, []string{"PAL"}, nil, true, nil)
	if err != nil {
		t.Fatalf("searchKeywordGroupsWithProgress returned error: %v", err)
	}
	if len(groups) != 1 || len(groups[0].Rows) != 1 {
		t.Fatalf("unexpected groups: %#v", groups)
	}
	if groups[0].Rows[0].TranscriptID != "wide.1" {
		t.Fatalf("forced wide search should use wide rows, got %#v", groups[0].Rows[0])
	}
	if groups[0].SearchType != "wide search" || groups[0].Rows[0].SearchType != "wide search" {
		t.Fatalf("forced wide search should mark group and row as wide search: %#v", groups)
	}
}

func TestSearchKeywordResultsWithProgressReturnsRecoverableError(t *testing.T) {
	w := &BlastWizard{source: keywordMapSource{
		errByKeyword: map[string]error{"bad": fmt.Errorf("network down")},
	}}

	results, err := w.searchKeywordResultsWithProgress(context.Background(), model.SpeciesCandidate{}, []string{"bad"}, make([]keywordSearchResult, 1), 0, false, nil)
	if err == nil {
		t.Fatal("expected recoverable error")
	}
	var recoverErr *keywordSearchRecoveryError
	if !errors.As(err, &recoverErr) {
		t.Fatalf("expected keywordSearchRecoveryError, got %T", err)
	}
	if recoverErr.Index != 0 || recoverErr.Keyword != "bad" {
		t.Fatalf("unexpected recoverable error payload: %#v", recoverErr)
	}
	if len(results) != 1 || results[0].err == nil {
		t.Fatalf("expected partial results to preserve failure: %#v", results)
	}
}

func TestSearchKeywordResultsWithProgressReturnsRecoverableErrorForEmptyRows(t *testing.T) {
	w := &BlastWizard{source: keywordMapSource{
		rowsByKeyword: map[string][]model.KeywordResultRow{"missing": nil},
	}}

	results, err := w.searchKeywordResultsWithProgress(context.Background(), model.SpeciesCandidate{}, []string{"missing"}, make([]keywordSearchResult, 1), 0, false, nil)
	if err == nil {
		t.Fatal("expected recoverable no-results error")
	}
	var recoverErr *keywordSearchRecoveryError
	if !errors.As(err, &recoverErr) {
		t.Fatalf("expected keywordSearchRecoveryError, got %T", err)
	}
	if recoverErr.Index != 0 || recoverErr.Keyword != "missing" {
		t.Fatalf("unexpected recoverable error payload: %#v", recoverErr)
	}
	var noRows keywordNoRowsError
	if !errors.As(err, &noRows) {
		t.Fatalf("expected keywordNoRowsError, got %T: %v", err, err)
	}
	if len(results) != 1 || results[0].err == nil {
		t.Fatalf("expected partial results to preserve no-results failure: %#v", results)
	}
}

func TestSearchKeywordResultsWithProgressPreservesCompletedResultsAcrossParallelFailure(t *testing.T) {
	w := &BlastWizard{source: keywordMapSource{
		rowsByKeyword: map[string][]model.KeywordResultRow{
			"ok-a": {{TranscriptID: "ok-a.1", SearchType: "keyword"}},
			"ok-b": {{TranscriptID: "ok-b.1", SearchType: "keyword"}},
		},
		errByKeyword: map[string]error{
			"bad": fmt.Errorf("network down"),
		},
	}}

	keywords := []string{"ok-a", "bad", "ok-b"}
	results, err := w.searchKeywordResultsWithProgress(context.Background(), model.SpeciesCandidate{}, keywords, make([]keywordSearchResult, len(keywords)), 0, false, nil)
	if err == nil {
		t.Fatal("expected recoverable error")
	}
	var recoverErr *keywordSearchRecoveryError
	if !errors.As(err, &recoverErr) {
		t.Fatalf("expected keywordSearchRecoveryError, got %T", err)
	}
	if recoverErr.Index != 1 || recoverErr.Keyword != "bad" {
		t.Fatalf("unexpected recoverable error payload: %#v", recoverErr)
	}
	if len(results[0].rows) != 1 || results[0].err != nil {
		t.Fatalf("first result should stay completed: %#v", results[0])
	}
	if results[1].err == nil {
		t.Fatalf("second result should preserve the failure: %#v", results[1])
	}
	if len(results[2].rows) != 1 || results[2].err != nil {
		t.Fatalf("third result should still be allowed to complete within the bounded batch: %#v", results[2])
	}
}

func TestBuildKeywordSearchGroupsKeepsSkippedGroupEmpty(t *testing.T) {
	now := time.Now()
	groups := buildKeywordSearchGroups([]string{"PAL"}, nil, []keywordSearchResult{{
		index:   0,
		started: now.Add(-time.Second),
		ended:   now,
		rows:    nil,
	}}, false)
	if len(groups) != 1 {
		t.Fatalf("unexpected group count: %#v", groups)
	}
	if groups[0].SearchTerm != "PAL" {
		t.Fatalf("unexpected search term: %#v", groups[0])
	}
	if len(groups[0].Rows) != 0 {
		t.Fatalf("skipped keyword should keep empty rows: %#v", groups[0])
	}
	if strings.TrimSpace(groups[0].SearchType) == "" {
		t.Fatalf("skipped keyword should still infer a search type: %#v", groups[0])
	}
}

func TestKeywordRowsToBlastItemsReusesKeywordMetadata(t *testing.T) {
	selected := model.SpeciesCandidate{
		ProteomeID:  42,
		JBrowseName: "Athaliana_TAIR10",
		GenomeLabel: "Arabidopsis thaliana TAIR10",
	}
	rows := []model.KeywordResultRow{{
		SourceDatabase:      "phytozome",
		LabelName:           "C4H",
		Aliases:             "AT2G30490;C4H",
		AutoDefine:          "cinnamate 4-hydroxylase",
		GeneIdentifier:      "AT2G30490",
		TranscriptID:        "AT2G30490.1",
		ProteinID:           "AT2G30490.1.p",
		SequenceID:          "AT2G30490.1",
		SequenceHeaderLabel: "At",
		Description:         "cinnamate 4-hydroxylase family protein",
		GeneReportURL:       "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
	}}
	items := keywordRowsToBlastItems(selected, rows, map[string]sequenceFetchResult{
		"AT2G30490.1": {data: model.ProteinSequenceData{Sequence: "MPEPTIDE"}},
	})
	if len(items) != 1 {
		t.Fatalf("blast item count = %d, want 1", len(items))
	}
	item := items[0]
	if item.LabelName != "C4H" {
		t.Fatalf("label name = %q, want C4H", item.LabelName)
	}
	if item.Sequence != "MPEPTIDE" {
		t.Fatalf("sequence = %q", item.Sequence)
	}
	if item.RawInput != rows[0].GeneReportURL {
		t.Fatalf("raw input = %q, want gene report URL", item.RawInput)
	}
	if item.QuerySource == nil {
		t.Fatal("expected query source")
	}
	if item.QuerySource.TranscriptID != rows[0].TranscriptID || item.QuerySource.GeneID != "AT2G30490" {
		t.Fatalf("query source identifiers not reused: %#v", item.QuerySource)
	}
	if item.QuerySource.LabelName != "C4H" || item.QuerySource.SourceProteomeID != 42 {
		t.Fatalf("query source metadata not reused: %#v", item.QuerySource)
	}
}

func TestKeywordRowsToBlastItemsFallsBackWhenLabelBlank(t *testing.T) {
	selected := model.SpeciesCandidate{ProteomeID: 7, JBrowseName: "S_polyrhiza_v2", GenomeLabel: "Spirodela polyrhiza"}
	rows := []model.KeywordResultRow{{
		SourceDatabase: "lemna",
		GeneIdentifier: "Sp9509d006g002010",
		TranscriptID:   "Sp9509d006g002010_T001",
		SequenceID:     "Sp9509d006g002010_T001",
	}}
	items := keywordRowsToBlastItems(selected, rows, map[string]sequenceFetchResult{
		"Sp9509d006g002010_T001": {data: model.ProteinSequenceData{Sequence: "MAAA"}},
	})
	if len(items) != 1 {
		t.Fatalf("blast item count = %d, want 1", len(items))
	}
	if items[0].LabelName != "" {
		t.Fatalf("blank keyword label should stay blank before BLAST label flow, got %q", items[0].LabelName)
	}
	if items[0].QuerySource == nil || items[0].QuerySource.LabelName != "" {
		t.Fatalf("blank keyword label should remain blank in source metadata: %#v", items[0].QuerySource)
	}
}

func TestResolveBlastQueryItemsCarriesQueryLabelIntoSourceMetadata(t *testing.T) {
	w := &BlastWizard{source: fakeSource{}}
	items := []blastQueryItem{{
		RawInput:    ">query\nMPEPTIDE",
		LabelName:   "PAL1",
		Sequence:    "MPEPTIDE",
		QuerySource: &model.QuerySequenceSource{Sequence: "MPEPTIDE"},
	}}
	prepared, err := w.resolveBlastQueryItemsWithProgress(context.Background(), items, nil, nil)
	if err != nil {
		t.Fatalf("resolveBlastQueryItemsWithProgress returned error: %v", err)
	}
	if len(prepared) != 1 || prepared[0].QuerySource == nil {
		t.Fatalf("prepared = %#v, want one item with source", prepared)
	}
	if got := prepared[0].QuerySource.LabelName; got != "PAL1" {
		t.Fatalf("QuerySource.LabelName = %q, want PAL1", got)
	}
	rows := prepareBlastRowsForReferences([]model.BlastResultRow{{Protein: "hit-1"}}, prepared[0], model.BlastRequest{
		Species:      model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10"},
		Sequence:     "MPEPTIDE",
		Program:      "BLASTP",
		SequenceKind: model.SequenceProtein,
	}, "phytozome")
	if got := rows[0].LabelName; got != "" {
		t.Fatalf("hit LabelName = %q, want query label not copied to hit label_name", got)
	}
	if got := rows[0].BlastLabelName; got != "PAL1" {
		t.Fatalf("BlastLabelName = %q, want PAL1", got)
	}
	metadata := buildExportMetadata("PAL1", prepared[0].QuerySource)
	if metadata == nil || len(metadata.Queries) != 1 || metadata.Queries[0].LabelName != "PAL1" {
		t.Fatalf("query metadata did not preserve source label: %#v", metadata)
	}
}

func TestFamilyBlastQueryLabelPrefersSourceLabelOverAliasList(t *testing.T) {
	item := blastQueryItem{
		LabelName: "PAL1",
		QuerySource: &model.QuerySequenceSource{
			LabelName:   "PAL1",
			PhgoAliases: "ATPAL1; PAL1",
			ProteinID:   "AT2G37040.1",
		},
	}
	if got := familyBlastQueryLabel(item); got != "PAL1" {
		t.Fatalf("familyBlastQueryLabel = %q, want source query label PAL1", got)
	}
	rows := prepareBlastRowsForReferences([]model.BlastResultRow{{Protein: "hit-1"}}, item, model.BlastRequest{
		Species:      model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10"},
		Sequence:     "MPEPTIDE",
		Program:      "BLASTP",
		SequenceKind: model.SequenceProtein,
	}, "phytozome")
	if got := rows[0].BlastLabelName; got != "PAL1" {
		t.Fatalf("BlastLabelName = %q, want source query label PAL1", got)
	}
	if got := rows[0].TargetID; got != 167 {
		t.Fatalf("TargetID = %d, want request species target id 167", got)
	}
	if got := rows[0].JBrowseName; got != "Athaliana_TAIR10" {
		t.Fatalf("JBrowseName = %q, want request species jbrowse name", got)
	}
}

func TestKeywordRowsSearchTypeFallsBackToClassifiedInputTypeWhenRowsEmpty(t *testing.T) {
	if got := keywordRowsSearchType(nil, "F5H1", false); got == "" {
		t.Fatal("empty keyword rows should still produce a classified search type")
	}
}

func TestAutoIdentifyLemnaKeywordLabelsUseSymbolNameDatabaseOnly(t *testing.T) {
	lookupSource := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			candidates: []model.SpeciesCandidate{
				{SearchAlias: "Spirodela polyrhiza v2", JBrowseName: "Spolyrhiza_v2", GenomeLabel: "Spirodela polyrhiza v2"},
			},
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"Sp9509d020g000340_T001": {{SourceDatabase: "phytozome", TranscriptID: "Sp9509d020g000340_T001", Synonyms: "C4H; CYP73A5", Symbols: "LOCAL_SHOULD_NOT_WIN"}},
			},
		},
	}
	w := &BlastWizard{
		source: lemna.NewClient(nil),
		speciesCandidatesCache: map[string][]model.SpeciesCandidate{
			"phytozome": {
				{SearchAlias: "Spirodela polyrhiza v2", JBrowseName: "Spolyrhiza_v2", GenomeLabel: "Spirodela polyrhiza v2"},
			},
		},
	}
	groups := []model.KeywordSearchGroup{{
		SearchTerm: "Sp9509d020g000340_T001",
		Rows: []model.KeywordResultRow{{
			SourceDatabase: "lemna",
			LabelName:      "LOCAL_SHOULD_NOT_WIN",
			ProteinID:      "Sp9509d020g000340_T001",
			TranscriptID:   "Sp9509d020g000340_T001",
		}},
	}}

	got := w.autoIdentifyLemnaKeywordLabels(context.Background(), model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0"}, groups, lookupSource)
	if len(got) != 1 || len(got[0].Aliases) == 0 {
		t.Fatalf("expected lemna keyword aliases: %#v", got)
	}
	if got[0].Aliases[0] != "CYP73A5" || got[0].SourceType != "symbolname database" {
		t.Fatalf("label aliases/type = %#v/%q, want symbolname database", got[0].Aliases, got[0].SourceType)
	}
}

func TestAutoIdentifyLemnaKeywordLabelsPrefersDirectSearchTerm(t *testing.T) {
	w := &BlastWizard{source: lemna.NewClient(nil)}
	groups := []model.KeywordSearchGroup{{
		SearchTerm: "Sp9509d020g000340_T001",
		Rows: []model.KeywordResultRow{{
			SourceDatabase: "lemna",
			LabelName:      "C4H",
			ProteinID:      "Sp9509d020g000340_T001",
			TranscriptID:   "Sp9509d020g000340_T001",
		}},
	}}

	got := w.autoIdentifyLemnaKeywordLabels(context.Background(), model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0"}, groups, nil)
	if len(got) != 1 || len(got[0].Aliases) == 0 || got[0].Aliases[0] != "CYP73A5" {
		t.Fatalf("expected lemna search term resolved first through gene_info: %#v", got)
	}
	if got[0].SourceType != "symbolname database" {
		t.Fatalf("SourceType = %q, want symbolname database", got[0].SourceType)
	}
}

func TestAutoIdentifyKeywordLabelsFallsBackToRowAliasesWhenSearchTermMisses(t *testing.T) {
	groups := []model.KeywordSearchGroup{{
		SearchTerm: "NO_SUCH_SYMBOL",
		Rows: []model.KeywordResultRow{{
			SourceDatabase: "phytozome",
			LabelName:      "C4H",
			Synonyms:       "C4H; CYP73A5",
		}},
	}}

	got := autoIdentifyKeywordLabelIdentifications(groups)
	if len(got) != 1 || len(got[0].Aliases) == 0 || got[0].Aliases[0] != "CYP73A5" {
		t.Fatalf("expected row aliases after direct search term miss: %#v", got)
	}
	if got[0].SourceType != "symbolname database" {
		t.Fatalf("SourceType = %q, want symbolname database", got[0].SourceType)
	}
}

func TestSearchKeywordRowsWithTimeoutDoesNotSetDefaultDeadline(t *testing.T) {
	source := &deadlineInspectingKeywordSource{}
	w := &BlastWizard{source: source}

	rows, err := w.searchKeywordRowsWithTimeout(context.Background(), model.SpeciesCandidate{}, "AT1G51680", false)
	if err != nil {
		t.Fatalf("searchKeywordRowsWithTimeout returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].GeneIdentifier != "AT1G51680" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	if source.lastHadDeadline {
		t.Fatal("keyword searches should not set a default hard deadline")
	}
}

func TestAutoIdentifyTAIRKeywordLabelsUseSymbolNameDatabaseOnly(t *testing.T) {
	lookupSource := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			candidates: []model.SpeciesCandidate{
				{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis thaliana TAIR10"},
			},
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT2G30490.1": {{SourceDatabase: "phytozome", TranscriptID: "AT2G30490.1", Synonyms: "C4H; CYP73A5", Symbols: "LOCAL_SHOULD_NOT_WIN"}},
			},
		},
	}
	w := &BlastWizard{
		source:               tair.NewClient(nil),
		keywordTermRowsCache: make(map[string][]model.KeywordResultRow),
		speciesCandidatesCache: map[string][]model.SpeciesCandidate{
			"phytozome": {
				{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis thaliana TAIR10"},
			},
		},
	}
	groups := []model.KeywordSearchGroup{{
		SearchTerm: "AT2G30490.1",
		Rows: []model.KeywordResultRow{
			{SourceDatabase: "tair", ProteinID: "AT2G30490.1", TranscriptID: "AT2G30490.1", Synonyms: "LOCAL_SHOULD_NOT_WIN"},
			{SourceDatabase: "tair", ProteinID: "AT2G30490.1", TranscriptID: "AT2G30490.1", Synonyms: "LOCAL_SHOULD_NOT_WIN"},
		},
	}}

	got := w.autoIdentifyTAIRKeywordLabelsWithLookup(context.Background(), groups, lookupSource)
	if len(got) != 1 || len(got[0].Aliases) == 0 {
		t.Fatalf("expected TAIR keyword aliases: %#v", got)
	}
	if got[0].Aliases[0] != "CYP73A5" || got[0].SourceType != "symbolname database" {
		t.Fatalf("label aliases/type = %#v/%q, want symbolname database", got[0].Aliases, got[0].SourceType)
	}
	lookupSource.mu.Lock()
	defer lookupSource.mu.Unlock()
	if lookupSource.fetchCount["AT2G30490.1"] != 0 {
		t.Fatalf("symbolname auto-identify should not query Phytozome, got %d", lookupSource.fetchCount["AT2G30490.1"])
	}
}

func TestAutoIdentifyTAIRKeywordLabelsFallsBackToOtherNames(t *testing.T) {
	w := &BlastWizard{source: tair.NewClient(nil)}
	groups := []model.KeywordSearchGroup{{
		SearchTerm: "AT2G30490.1",
		Rows: []model.KeywordResultRow{{
			SourceDatabase: "tair",
			ProteinID:      "AT2G30490.1",
			TranscriptID:   "AT2G30490.1",
			Synonyms:       "C4H; CYP73A5",
		}},
	}}

	got := w.autoIdentifyTAIRKeywordLabelsWithLookup(context.Background(), groups, nil)
	if len(got) != 1 || len(got[0].Aliases) == 0 || got[0].Aliases[0] != "CYP73A5" {
		t.Fatalf("expected TAIR symbolname database aliases: %#v", got)
	}
	if got[0].SourceType != "symbolname database" {
		t.Fatalf("SourceType = %q, want symbolname database", got[0].SourceType)
	}
}

func TestAutoIdentifyTAIRKeywordLabelsUsesLocalSymbolDatabaseFromAGI(t *testing.T) {
	w := &BlastWizard{source: tair.NewClient(nil)}
	groups := []model.KeywordSearchGroup{{
		SearchTerm: "PAL",
		Rows: []model.KeywordResultRow{{
			SourceDatabase: "tair",
			GeneIdentifier: "AT2G37040",
			TranscriptID:   "AT2G37040.1",
			SequenceID:     "AT2G37040.1",
			Description:    "phenylalanine ammonia-lyase",
		}},
	}}

	got := w.autoIdentifyTAIRKeywordLabelsWithLookup(context.Background(), groups, nil)
	if len(got) != 1 || len(got[0].Aliases) == 0 {
		t.Fatalf("expected TAIR AGI to resolve through local symbol database: %#v", got)
	}
	if got[0].Aliases[0] != "PAL1" {
		t.Fatalf("TAIR local symbol aliases = %#v, want PAL1 first", got[0].Aliases)
	}
}

func TestAutoIdentifyLemnaKeywordLabelsDoesNotQueryPhytozome(t *testing.T) {
	lookupSource := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			candidates: []model.SpeciesCandidate{
				{SearchAlias: "Spirodela polyrhiza v2", JBrowseName: "Spolyrhiza_v2", GenomeLabel: "Spirodela polyrhiza v2"},
			},
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT2G30490.1": {{SourceDatabase: "phytozome", TranscriptID: "AT2G30490.1", Synonyms: "C4H; CYP73A5"}},
			},
		},
	}
	w := &BlastWizard{
		source: lemna.NewClient(nil),
		speciesCandidatesCache: map[string][]model.SpeciesCandidate{
			"phytozome": {
				{SearchAlias: "Spirodela polyrhiza v2", JBrowseName: "Spolyrhiza_v2", GenomeLabel: "Spirodela polyrhiza v2"},
			},
		},
	}
	groups := []model.KeywordSearchGroup{
		{
			SearchTerm: "row-1",
			Rows: []model.KeywordResultRow{{
				ProteinID: "AT2G30490.1",
				Aliases:   "candidate alias phrase",
			}},
		},
		{
			SearchTerm: "row-2",
			Rows: []model.KeywordResultRow{{
				ProteinID: "AT2G30490.1",
				Aliases:   "candidate alias phrase",
			}},
		},
	}

	got := w.autoIdentifyLemnaKeywordLabels(context.Background(), model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0"}, groups, lookupSource)
	if len(got) != 2 || got[0].Aliases[0] != "CYP73A5" || got[1].Aliases[0] != "CYP73A5" {
		t.Fatalf("expected deduplicated lookup to populate both identifications: %#v", got)
	}
	lookupSource.mu.Lock()
	defer lookupSource.mu.Unlock()
	if lookupSource.fetchCount["AT2G30490.1"] != 0 {
		t.Fatalf("symbolname auto-identify should not query Phytozome, got %d", lookupSource.fetchCount["AT2G30490.1"])
	}
}

func TestAutoIdentifyLemnaKeywordLabelsRanksLocalAliasesWithoutPhytozome(t *testing.T) {
	lookupSource := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			candidates: []model.SpeciesCandidate{
				{SearchAlias: "Spirodela polyrhiza v2", JBrowseName: "Spolyrhiza_v2", GenomeLabel: "Spirodela polyrhiza v2"},
			},
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT2G30490.1": {{SourceDatabase: "phytozome", TranscriptID: "AT2G30490.1", Synonyms: "C4H; CYP73A5"}},
			},
		},
	}
	w := &BlastWizard{
		source: lemna.NewClient(nil),
		speciesCandidatesCache: map[string][]model.SpeciesCandidate{
			"phytozome": {
				{SearchAlias: "Spirodela polyrhiza v2", JBrowseName: "Spolyrhiza_v2", GenomeLabel: "Spirodela polyrhiza v2"},
			},
		},
	}
	groups := []model.KeywordSearchGroup{
		{
			SearchTerm: "row-1",
			Rows: []model.KeywordResultRow{{
				ProteinID:    "AT2G30490.1",
				TranscriptID: "AT2G30490.1",
				Aliases:      "C4H; CYP73A5",
			}},
		},
	}

	got := w.autoIdentifyLemnaKeywordLabels(context.Background(), model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0"}, groups, lookupSource)
	if len(got) != 1 || got[0].Aliases[0] != "CYP73A5" {
		t.Fatalf("expected local fields ranked by symbolname database: %#v", got)
	}
	lookupSource.mu.Lock()
	defer lookupSource.mu.Unlock()
	if lookupSource.fetchCount["AT2G30490.1"] != 0 {
		t.Fatalf("symbolname auto-identify should not query Phytozome, got %d", lookupSource.fetchCount["AT2G30490.1"])
	}
}

func TestFetchKeywordRowsByTermsCachesAcrossCalls(t *testing.T) {
	lookupSource := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT2G30490.1": {{SourceDatabase: "phytozome", TranscriptID: "AT2G30490.1", Synonyms: "C4H; CYP73A5"}},
			},
		},
	}
	w := &BlastWizard{
		keywordTermRowsCache: make(map[string][]model.KeywordResultRow),
	}
	species := model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis thaliana TAIR10"}

	first := w.fetchKeywordRowsByTerms(context.Background(), lookupSource, species, []string{"AT2G30490.1", "AT2G30490.1"})
	second := w.fetchKeywordRowsByTerms(context.Background(), lookupSource, species, []string{"AT2G30490.1"})
	if len(first["at2g30490.1"]) == 0 || len(second["at2g30490.1"]) == 0 {
		t.Fatalf("expected cached keyword rows for AT2G30490.1, got first=%#v second=%#v", first, second)
	}
	lookupSource.mu.Lock()
	defer lookupSource.mu.Unlock()
	if lookupSource.fetchCount["AT2G30490.1"] != 1 {
		t.Fatalf("phytozome lookup count across repeated fetchKeywordRowsByTerms calls = %d, want 1", lookupSource.fetchCount["AT2G30490.1"])
	}
}

func TestKeywordSearchRowsAreReusedByTermLookupCache(t *testing.T) {
	lookupSource := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT2G30490.1": {{SourceDatabase: "phytozome", TranscriptID: "AT2G30490.1", Synonyms: "C4H; CYP73A5"}},
			},
		},
	}
	w := &BlastWizard{
		source:               lookupSource,
		keywordTermRowsCache: make(map[string][]model.KeywordResultRow),
	}
	species := model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis thaliana TAIR10"}

	results, err := w.searchKeywordResultsWithProgress(context.Background(), species, []string{"AT2G30490.1"}, make([]keywordSearchResult, 1), 0, false, nil)
	if err != nil {
		t.Fatalf("searchKeywordResultsWithProgress returned error: %v", err)
	}
	if len(results) != 1 || len(results[0].rows) == 0 {
		t.Fatalf("expected search rows, got %#v", results)
	}
	lookupSource.mu.Lock()
	if lookupSource.fetchCount["AT2G30490.1"] != 1 {
		t.Fatalf("initial keyword search count = %d, want 1", lookupSource.fetchCount["AT2G30490.1"])
	}
	lookupSource.mu.Unlock()

	reused := w.fetchKeywordRowsByTerms(context.Background(), lookupSource, species, []string{"AT2G30490.1"})
	if len(reused["at2g30490.1"]) == 0 {
		t.Fatalf("expected term lookup to reuse cached keyword rows, got %#v", reused)
	}
	lookupSource.mu.Lock()
	defer lookupSource.mu.Unlock()
	if lookupSource.fetchCount["AT2G30490.1"] != 1 {
		t.Fatalf("term lookup performed another source search, count = %d, want 1", lookupSource.fetchCount["AT2G30490.1"])
	}
}

func TestFetchKeywordRowsByTermsDeduplicatesConcurrentLookups(t *testing.T) {
	lookupSource := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT2G30490.1": {{SourceDatabase: "phytozome", TranscriptID: "AT2G30490.1", Synonyms: "C4H; CYP73A5"}},
			},
		},
	}
	w := &BlastWizard{
		keywordTermRowsCache: make(map[string][]model.KeywordResultRow),
	}
	species := model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis thaliana TAIR10"}
	var wg sync.WaitGroup
	results := make([]map[string][]model.KeywordResultRow, 2)
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = w.fetchKeywordRowsByTerms(context.Background(), lookupSource, species, []string{"AT2G30490.1"})
		}(i)
	}
	wg.Wait()
	for i, got := range results {
		if len(got["at2g30490.1"]) == 0 {
			t.Fatalf("concurrent result %d missing cached keyword rows: %#v", i, got)
		}
	}
	lookupSource.mu.Lock()
	defer lookupSource.mu.Unlock()
	if lookupSource.fetchCount["AT2G30490.1"] != 1 {
		t.Fatalf("phytozome lookup count across concurrent fetchKeywordRowsByTerms calls = %d, want 1", lookupSource.fetchCount["AT2G30490.1"])
	}
}

func TestKeywordBackToQueryInputClearsRowReuse(t *testing.T) {
	w := &BlastWizard{
		rewindKeywordToInput: true,
		reuseLastKeywordRows: true,
		lastKeywordGroups: []model.KeywordSearchGroup{{
			SearchTerm: "ara",
			Rows: []model.KeywordResultRow{{
				SearchTerm:   "ara",
				TranscriptID: "AT1G01010.1",
			}},
		}},
	}

	w.consumeKeywordInputRewind()

	if w.rewindKeywordToInput {
		t.Fatal("keyword rewind flag should be consumed before re-entering keyword input")
	}
	if w.reuseLastKeywordRows {
		t.Fatal("keyword row reuse must be disabled when Back targets keyword input")
	}
}

func TestKeywordRowBackRewindsOuterInputLoop(t *testing.T) {
	w := &BlastWizard{
		reuseLastKeywordRows: true,
		lastKeywordGroups: []model.KeywordSearchGroup{{
			SearchTerm: "ara",
			Rows: []model.KeywordResultRow{{
				SearchTerm:   "ara",
				TranscriptID: "AT1G01010.1",
			}},
		}},
	}

	w.rewindKeywordRowsToInput()
	w.consumeKeywordInputRewind()

	if w.rewindKeywordToInput {
		t.Fatal("keyword rewind flag should be consumed after leaving row selection")
	}
	if w.reuseLastKeywordRows {
		t.Fatal("keyword row selection Back must not reuse rows and re-open the same table")
	}
}

func TestBlastBackToQueryInputClearsInputAndRowReuse(t *testing.T) {
	w := &BlastWizard{
		rewindBlastToInput:  true,
		reuseLastBlastInput: true,
		reuseLastBlastRows:  true,
		lastBlastItems: []blastQueryItem{{
			Sequence: "MPEPTIDE",
		}},
		lastBlastRowContext: &blastRowContext{
			Rows: []model.BlastResultRow{{Protein: "AT1G01010.1"}},
		},
	}

	w.consumeBlastInputRewind()

	if w.rewindBlastToInput {
		t.Fatal("blast rewind flag should be consumed before re-entering BLAST input")
	}
	if w.reuseLastBlastInput {
		t.Fatal("BLAST input reuse must be disabled when Back targets BLAST input")
	}
	if w.reuseLastBlastRows {
		t.Fatal("BLAST row reuse must be disabled when Back targets BLAST input")
	}
}

func TestPostRunCloseRewindsToInputInsteadOfExit(t *testing.T) {
	w := &BlastWizard{}
	w.rewindModeToInput(ModeBlast)
	if !w.rewindBlastToInput {
		t.Fatal("closing the post-run dialog in BLAST mode should re-enter BLAST input")
	}
	if w.rewindKeywordToInput {
		t.Fatal("closing the BLAST post-run dialog should not rewind keyword input")
	}

	w = &BlastWizard{}
	w.rewindModeToInput(ModeKeyword)
	if !w.rewindKeywordToInput {
		t.Fatal("closing the post-run dialog in keyword mode should re-enter keyword input")
	}
	if w.rewindBlastToInput {
		t.Fatal("closing the keyword post-run dialog should not rewind BLAST input")
	}
}

func TestTableBackTargetsDoNotReuseSameTable(t *testing.T) {
	keywordWizard := &BlastWizard{
		reuseLastKeywordRows: true,
		lastKeywordGroups: []model.KeywordSearchGroup{{
			SearchTerm: "keyword",
			Rows:       []model.KeywordResultRow{{TranscriptID: "AT1G01010.1"}},
		}},
	}
	if classifyWizardBack(prompt.ErrBackToQueryInput) != wizardBackQuery {
		t.Fatal("row table Back should classify as query input navigation")
	}
	keywordWizard.rewindKeywordRowsToInput()
	keywordWizard.consumeKeywordInputRewind()
	if keywordWizard.reuseLastKeywordRows {
		t.Fatal("keyword table Back must not reopen the same row table")
	}

	blastWizard := &BlastWizard{
		rewindBlastToInput:  true,
		reuseLastBlastInput: true,
		reuseLastBlastRows:  true,
		lastBlastRowContext: &blastRowContext{Rows: []model.BlastResultRow{{Protein: "AT1G01010.1"}}},
	}
	blastWizard.consumeBlastInputRewind()
	if blastWizard.reuseLastBlastInput || blastWizard.reuseLastBlastRows {
		t.Fatal("BLAST table Back must not reopen the same row table or skip BLAST input")
	}
}

func TestClassifyWizardBackCoversNavigationTargets(t *testing.T) {
	tests := []struct {
		err  error
		want wizardBackAction
	}{
		{err: nil, want: wizardBackNone},
		{err: prompt.ErrExitRequested, want: wizardBackExit},
		{err: prompt.ErrBackToDatabaseSelection, want: wizardBackDatabase},
		{err: prompt.ErrBackToModeSelection, want: wizardBackMode},
		{err: prompt.ErrBackToSpeciesSelection, want: wizardBackSpecies},
		{err: prompt.ErrBackToQueryInput, want: wizardBackQuery},
		{err: prompt.ErrBackToBlastProgram, want: wizardBackBlastProgram},
		{err: prompt.ErrBackToRowSelection, want: wizardBackRows},
	}

	for _, tc := range tests {
		if got := classifyWizardBack(tc.err); got != tc.want {
			t.Fatalf("classifyWizardBack(%v)=%v want %v", tc.err, got, tc.want)
		}
	}
}

func TestInterpretRecoveryAction(t *testing.T) {
	tests := []struct {
		name       string
		action     string
		backTarget error
		allowSkip  bool
		want       recoveryDecision
		wantErr    error
	}{
		{name: "retry", action: "retry", want: recoveryRetry},
		{name: "skip", action: "skip", allowSkip: true, want: recoverySkip},
		{name: "back", action: "back", backTarget: prompt.ErrBackToRowSelection, want: recoveryBack, wantErr: prompt.ErrBackToRowSelection},
		{name: "close uses back target", action: "close", backTarget: prompt.ErrBackToQueryInput, want: recoveryBack, wantErr: prompt.ErrBackToQueryInput},
		{name: "exit", action: "exit", want: recoveryExit, wantErr: prompt.ErrExitRequested},
		{name: "empty falls back", action: "", backTarget: prompt.ErrBackToQueryInput, want: recoveryBack, wantErr: prompt.ErrBackToQueryInput},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := interpretRecoveryAction(tt.action, tt.backTarget, tt.allowSkip)
			if got != tt.want {
				t.Fatalf("decision=%v want %v", got, tt.want)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err=%v want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeExportNameDoesNotAffectDisplayLabel(t *testing.T) {
	item := blastQueryItem{LabelName: "AtCESA4"}
	display := buildBlastOutputDisplayName(item)
	fileName := sanitizeExportName(display)
	if display != "AtCESA4" {
		t.Fatalf("unexpected display label: %q", display)
	}
	if fileName != "AtCESA4" {
		t.Fatalf("unexpected file name: %q", fileName)
	}
}

func TestParseFastaQuerySequenceInput(t *testing.T) {
	source, ok := parseFastaQuerySequenceInput(">A.thaliana TAIR10|AT5G44030.1\nMEPNTMASFDDEH\n")
	if !ok {
		t.Fatalf("expected FASTA header to parse")
	}
	if source.GeneID != "" || source.TranscriptID != "" || source.ProteinID != "" || source.LabelName != "" {
		t.Fatalf("generic FASTA header should not directly produce structured metadata: %#v", source)
	}
	if source.Sequence != "MEPNTMASFDDEH" {
		t.Fatalf("unexpected sequence: %q", source.Sequence)
	}
}

func TestParseBlastQueryItemsMultiFasta(t *testing.T) {
	input := strings.Join([]string{
		">Arabidopsis thaliana TAIR10|AT5G62380.1 (AtVND6)",
		"MESLAHIPPGYRFHPT",
		">Arabidopsis thaliana TAIR10|AT1G71930.1 (AtVND7)",
		"MDNIMQSSMPPGFRF",
	}, "\n")

	items, err := parseBlastQueryItems(input)
	if err != nil {
		t.Fatalf("parseBlastQueryItems returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two FASTA query items, got %d: %#v", len(items), items)
	}
	if got := items[0].LabelName; got != "" {
		t.Fatalf("FASTA parser should not directly assign first label: %q", got)
	}
	if got := items[1].LabelName; got != "" {
		t.Fatalf("FASTA parser should not directly assign second label: %q", got)
	}
	if got := items[0].Sequence; got != "MESLAHIPPGYRFHPT" {
		t.Fatalf("unexpected first sequence: %q", got)
	}
	if got := items[1].Sequence; got != "MDNIMQSSMPPGFRF" {
		t.Fatalf("unexpected second sequence: %q", got)
	}
	if items[0].QuerySource == nil || items[1].QuerySource == nil {
		t.Fatalf("expected FASTA query sources to be preserved")
	}
	if got := items[0].QuerySource.GeneID; got != "" {
		t.Fatalf("generic FASTA should not directly assign first gene id: %q", got)
	}
	if got := items[1].QuerySource.GeneID; got != "" {
		t.Fatalf("generic FASTA should not directly assign second gene id: %q", got)
	}
}

func TestParseBlastQueryItemsSingleLineMultiFasta(t *testing.T) {
	input := strings.Join([]string{
		">Arabidopsis thaliana TAIR10|AT5G62380.1 (AtVND6) MESL",
		">Arabidopsis thaliana TAIR10|AT1G71930.1 (VND7) MDNI",
	}, "\n")

	items, err := parseBlastQueryItems(input)
	if err != nil {
		t.Fatalf("parseBlastQueryItems returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two FASTA query items, got %d: %#v", len(items), items)
	}
	if got := items[0].LabelName; got != "" {
		t.Fatalf("FASTA parser should not directly assign first label: %q", got)
	}
	if got := items[1].LabelName; got != "" {
		t.Fatalf("FASTA parser should not directly assign second label: %q", got)
	}
	if got := items[0].Sequence; got != "MESL" {
		t.Fatalf("unexpected first sequence: %q", got)
	}
	if got := items[1].Sequence; got != "MDNI" {
		t.Fatalf("unexpected second sequence: %q", got)
	}
}

func TestParseBlastQueryItemsMixedFastaURLAndPlainSequence(t *testing.T) {
	input := strings.Join([]string{
		">Arabidopsis thaliana TAIR10|AT5G62380.1 (AtVND6)",
		"MESL*",
		"",
		"https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT1G71930",
		"",
		">plain_header_no_label",
		"MDNI*",
		"",
		"MPEPTIDE*",
	}, "\n")

	items, err := parseBlastQueryItems(input)
	if err != nil {
		t.Fatalf("parseBlastQueryItems returned error: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("expected four query items, got %d: %#v", len(items), items)
	}
	if got := items[0].LabelName; got != "" {
		t.Fatalf("FASTA parser should not directly assign first label: %q", got)
	}
	if got := items[0].Sequence; got != "MESL" {
		t.Fatalf("unexpected first sequence: %q", got)
	}
	if got := items[1].RawInput; got != "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT1G71930" {
		t.Fatalf("unexpected URL item: %q", got)
	}
	if items[1].QuerySource != nil {
		t.Fatalf("URL item should resolve later, got query source: %#v", items[1].QuerySource)
	}
	if got := items[2].LabelName; got != "" {
		t.Fatalf("plain FASTA without parenthetical label should not invent label: %q", got)
	}
	if got := items[2].Sequence; got != "MDNI" {
		t.Fatalf("unexpected plain FASTA sequence: %q", got)
	}
	if items[2].QuerySource == nil {
		t.Fatalf("expected plain FASTA query source to be preserved")
	}
	if items[2].QuerySource.ProteinID != "" || items[2].QuerySource.GeneID != "" || items[2].QuerySource.LabelName != "" {
		t.Fatalf("plain FASTA header should not directly produce metadata, got %#v", items[2].QuerySource)
	}
	if got := items[3].RawInput; got != "MPEPTIDE*" {
		t.Fatalf("unexpected plain sequence item: %q", got)
	}
}

func TestParseBlastQueryItemsWhitespaceSeparatedURLs(t *testing.T) {
	input := "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT5G62380 https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT1G71930"

	items, err := parseBlastQueryItems(input)
	if err != nil {
		t.Fatalf("parseBlastQueryItems returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two URL query items, got %d: %#v", len(items), items)
	}
	if !strings.Contains(items[0].RawInput, "AT5G62380") || !strings.Contains(items[1].RawInput, "AT1G71930") {
		t.Fatalf("unexpected URL items: %#v", items)
	}
}

func TestParseBlastQueryItemsPlainSequencesSeparatedByBlankLines(t *testing.T) {
	input := strings.Join([]string{
		"MPEPTIDE*",
		"",
		"MSECONDSEQ*",
	}, "\n")

	items, err := parseBlastQueryItems(input)
	if err != nil {
		t.Fatalf("parseBlastQueryItems returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two plain sequence items, got %d: %#v", len(items), items)
	}
	if items[0].RawInput != "MPEPTIDE*" || items[1].RawInput != "MSECONDSEQ*" {
		t.Fatalf("unexpected plain sequence items: %#v", items)
	}
}

func TestParseBlastQueryItemsPlainSequencesSeparatedBySpaces(t *testing.T) {
	items, err := parseBlastQueryItems("MPEPTIDE* MSECONDSEQ*")
	if err != nil {
		t.Fatalf("parseBlastQueryItems returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two plain sequence items, got %d: %#v", len(items), items)
	}
	if items[0].RawInput != "MPEPTIDE*" || items[1].RawInput != "MSECONDSEQ*" {
		t.Fatalf("unexpected plain sequence items: %#v", items)
	}
}

func TestParseFastaQuerySequenceInputSingleLineWithTrailingLabel(t *testing.T) {
	input := ">A.thaliana TAIR10|AT5G44030.1 (AtCESA4) MEPNTMASFDDEHRHSSFSAKIC"
	source, ok := parseFastaQuerySequenceInput(input)
	if !ok {
		t.Fatalf("expected single-line FASTA header to parse")
	}
	if source.Sequence != "MEPNTMASFDDEHRHSSFSAKIC" {
		t.Fatalf("unexpected sequence: %q", source.Sequence)
	}
	if source.GeneID != "" || source.TranscriptID != "" || source.ProteinID != "" || source.LabelName != "" {
		t.Fatalf("generic single-line FASTA should not directly assign metadata, got %#v", source)
	}
}

func TestParseFastaQuerySequenceInputPhgoHeaderWithRowNumber(t *testing.T) {
	source, ok := parseFastaQuerySequenceInput(">phgo://Sp7498/PAL1/AT2G37040\\7\nMEPNTMASFDDEH\n")
	if !ok {
		t.Fatalf("expected phgo FASTA header to parse")
	}
	if source.LabelName != "PAL1" {
		t.Fatalf("unexpected label: %q", source.LabelName)
	}
	if source.GeneID != "AT2G37040" {
		t.Fatalf("unexpected gene id: %q", source.GeneID)
	}
	if source.OrganismShort != "Sp7498" {
		t.Fatalf("unexpected species: %q", source.OrganismShort)
	}
	if source.Sequence != "MEPNTMASFDDEH" {
		t.Fatalf("unexpected sequence: %q", source.Sequence)
	}
	if strings.TrimSpace(source.PhgoAliases) != "" {
		t.Fatalf("phgo FASTA parse should not prefill ranked aliases: %#v", source)
	}
}

func TestParseFastaQuerySequenceInputPhgoHeaderWithoutRowNumber(t *testing.T) {
	source, ok := parseFastaQuerySequenceInput(">phgo://Sp7498/PAL1/AT2G37040\nMEPNTMASFDDEH\n")
	if !ok {
		t.Fatalf("expected phgo FASTA header without row number to parse")
	}
	if source.LabelName != "PAL1" || source.GeneID != "AT2G37040" || source.OrganismShort != "Sp7498" {
		t.Fatalf("unexpected phgo FASTA metadata: %#v", source)
	}
}

func TestParseFastaQuerySequenceInputPhgoLiteHeader(t *testing.T) {
	source, ok := parseFastaQuerySequenceInput(">Bd21-3|Bradi3g18960(Bd4CL1)\nMEPNTMASFDDEH\n")
	if !ok {
		t.Fatal("expected PHgo Lite FASTA header to parse")
	}
	if source.LabelName != "Bd4CL1" || source.GeneID != "Bradi3g18960" || source.OrganismShort != "Bd21-3" {
		t.Fatalf("unexpected PHgo Lite FASTA metadata: %#v", source)
	}
}

func TestParseFastaQuerySequenceInputSingleLinePhgoHeader(t *testing.T) {
	source, ok := parseFastaQuerySequenceInput(">phgo://Sp7498/PAL1/AT2G37040\\7 MEPNTMASFDDEH")
	if !ok {
		t.Fatalf("expected single-line phgo FASTA header to parse")
	}
	if source.LabelName != "PAL1" || source.GeneID != "AT2G37040" {
		t.Fatalf("unexpected phgo single-line metadata: %#v", source)
	}
	if source.Sequence != "MEPNTMASFDDEH" {
		t.Fatalf("unexpected single-line phgo sequence: %q", source.Sequence)
	}
}

func TestParseFastaQuerySequenceInputBlastPhgoHeader(t *testing.T) {
	source, ok := parseFastaQuerySequenceInput(">phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\7\nMEPNTMASFDDEH\n")
	if !ok {
		t.Fatalf("expected BLAST phgo FASTA header to parse")
	}
	if source.LabelName != "C4H" || source.GeneID != "Sp7498_C4H_001" || source.OrganismShort != "Sp7498" {
		t.Fatalf("unexpected BLAST phgo metadata: %#v", source)
	}
	if source.Sequence != "MEPNTMASFDDEH" {
		t.Fatalf("unexpected BLAST phgo sequence: %q", source.Sequence)
	}
}

func TestParseFastaQuerySequenceInputPhgoSourceHeaderWithSpaces(t *testing.T) {
	source, ok := parseFastaQuerySequenceInput(">phgo://Oryza sativa v7.0/4CL1/LOC_Os08g14760.1\\h MEPNTMASFDDEH")
	if !ok {
		t.Fatalf("expected source phgo FASTA header with spaces to parse")
	}
	if source.LabelName != "4CL1" || source.GeneID != "LOC_Os08g14760.1" || source.OrganismShort != "Oryza sativa v7.0" {
		t.Fatalf("unexpected source phgo metadata: %#v", source)
	}
	if source.Sequence != "MEPNTMASFDDEH" {
		t.Fatalf("unexpected source phgo sequence: %q", source.Sequence)
	}
}

func TestStripTrailingParentheticalLabel(t *testing.T) {
	got := stripTrailingParentheticalLabel("A.thaliana TAIR10|AT5G44030.1 (AtCESA4)")
	if got != "A.thaliana TAIR10|AT5G44030.1" {
		t.Fatalf("unexpected stripped label: %q", got)
	}
}

func TestParseFastaQuerySequenceInputPlainSequence(t *testing.T) {
	if source, ok := parseFastaQuerySequenceInput("MEPNTMASFDDEH\n"); ok || source != nil {
		t.Fatalf("plain sequence should not produce query metadata")
	}
}

func TestParseFastaQuerySequenceInputIgnoresPhgoNoteHeader(t *testing.T) {
	if source, ok := parseFastaQuerySequenceInput(">phgo://note\nMNOTE\n"); ok || source != nil {
		t.Fatalf("phgo note header should be ignored")
	}
}

func TestBuildQuerySequenceHeaderID(t *testing.T) {
	source := &model.QuerySequenceSource{
		OrganismShort: "A.thaliana",
		Annotation:    "TAIR10",
		ProteinID:     "AT5G44030.1",
	}
	if got := buildQuerySequenceHeaderID(source); got != "A.thaliana TAIR10|AT5G44030.1" {
		t.Fatalf("unexpected query header id: %q", got)
	}
}

func TestDescribeQuerySourceCrossDatabaseURL(t *testing.T) {
	source := &model.QuerySequenceSource{
		NormalizedURL:  "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
		SourceDatabase: "phytozome",
	}
	got := describeQuerySource(source, "lemna")
	want := "Resolved peptide sequence from a Phytozome gene report URL. The sequence will be fetched from Phytozome and searched against the selected lemna.org species."
	if got != want {
		t.Fatalf("unexpected description: %q", got)
	}
}

func TestDescribeQuerySourceSameDatabaseURL(t *testing.T) {
	source := &model.QuerySequenceSource{
		NormalizedURL:  "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
		SourceDatabase: "phytozome",
	}
	got := describeQuerySource(source, "phytozome")
	want := "Resolved peptide sequence from a Phytozome gene report URL."
	if got != want {
		t.Fatalf("unexpected description: %q", got)
	}
}

func TestBuildExportMetadataPrefersOriginalInputURL(t *testing.T) {
	source := &model.QuerySequenceSource{
		OriginalInputURL: "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490?copied=1",
		NormalizedURL:    "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
		GeneID:           "AT2G30490",
	}

	metadata := buildExportMetadata("C4H", source)
	if metadata == nil {
		t.Fatal("expected export metadata")
	}
	if metadata.GeneReportURL != source.OriginalInputURL {
		t.Fatalf("unexpected metadata URL: %q", metadata.GeneReportURL)
	}
}

func TestBlastRowToBlastQueryItemUsesHitFASTAAndLabelMetadata(t *testing.T) {
	w := &BlastWizard{
		source: fakeSource{
			sequences: map[string]string{"seq1": "MPEPTIDE"},
			headers:   map[string]string{"seq1": ">seq1 source header"},
		},
		proteinSequenceCache: make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:  make(map[string]error),
	}
	row := model.BlastResultRow{
		SourceDatabase:   "phytozome",
		LabelName:        "PAL1",
		PhgoAliases:      "PAL1; PAL2",
		BlastGeneID:      "GeneA.1",
		Protein:          "prot1",
		SequenceID:       "seq1",
		TranscriptID:     "tx1",
		Species:          "Arabidopsis",
		TargetID:         42,
		UniProtAccession: "P12345",
		Defline:          "phenylalanine ammonia-lyase",
	}

	item, err := w.blastRowToBlastQueryItem(context.Background(), model.SpeciesCandidate{ProteomeID: 42, GenomeLabel: "TAIR10"}, row)
	if err != nil {
		t.Fatalf("blast row conversion failed: %v", err)
	}
	if !strings.HasPrefix(item.RawInput, ">seq1 source header\n") {
		t.Fatalf("raw FASTA was not preserved: %q", item.RawInput)
	}
	if item.Sequence != "MPEPTIDE" || item.ProteinSequence != "MPEPTIDE" {
		t.Fatalf("unexpected query sequence: %#v", item)
	}
	if item.LabelName != "PAL1" || item.QuerySource == nil || item.QuerySource.LabelName != "PAL1" {
		t.Fatalf("label metadata not preserved: %#v", item)
	}
	if item.QuerySource.PhgoAliases != "PAL1; PAL2" || item.QuerySource.UniProtAccession != "P12345" {
		t.Fatalf("source aliases/uniprot not preserved: %#v", item.QuerySource)
	}
	if item.QuerySource.SourceProteomeID != 42 || item.QuerySource.ProteinID != "prot1" || item.QuerySource.TranscriptID != "tx1" {
		t.Fatalf("source identifiers not preserved: %#v", item.QuerySource)
	}
}

type fakeSource struct {
	name           string
	query          *model.QuerySequenceSource
	species        []model.SpeciesCandidate
	keywordRows    []model.KeywordResultRow
	sequences      map[string]string
	nucleotideSeqs map[string]string
	headers        map[string]string
	sequenceErrors map[string]error
	fetchCount     map[string]int
	err            error
}

var fakeSourceFetchMu sync.Mutex

func (f fakeSource) Name() string {
	if strings.TrimSpace(f.name) != "" {
		return strings.TrimSpace(f.name)
	}
	return "fake"
}
func (f fakeSource) FetchSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {
	return append([]model.SpeciesCandidate(nil), f.species...), nil
}
func (f fakeSource) SubmitBlast(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {
	return model.BlastJob{}, nil
}
func (f fakeSource) WaitForBlastResults(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, error) {
	return model.BlastResult{}, nil
}
func (f fakeSource) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return append([]model.KeywordResultRow(nil), f.keywordRows...), nil
}
func (f fakeSource) FetchProteinSequence(ctx context.Context, targetID int, sequenceID string) (model.ProteinSequenceData, error) {
	if f.fetchCount != nil {
		fakeSourceFetchMu.Lock()
		f.fetchCount[sequenceID]++
		fakeSourceFetchMu.Unlock()
	}
	if err, ok := f.sequenceErrors[sequenceID]; ok {
		return model.ProteinSequenceData{}, err
	}
	if sequence, ok := f.sequences[sequenceID]; ok {
		return model.ProteinSequenceData{
			Sequence:       sequence,
			OriginalHeader: strings.TrimSpace(f.headers[sequenceID]),
		}, nil
	}
	return model.ProteinSequenceData{}, fmt.Errorf("no protein sequence for transcript id %s", sequenceID)
}
func (f fakeSource) FetchGeneQuerySequence(ctx context.Context, species model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.query, nil
}
func (f fakeSource) FetchProteinQuerySequence(ctx context.Context, species model.SpeciesCandidate, proteinID string) (*model.QuerySequenceSource, error) {
	if f.err != nil {
		return nil, f.err
	}
	source := *f.query
	source.ProteinID = proteinID
	return &source, nil
}
func (f fakeSource) FetchNucleotideSequence(ctx context.Context, targetID int, sequenceID string, program string) (model.ProteinSequenceData, error) {
	key := strings.ToLower(strings.TrimSpace(program)) + "|" + sequenceID
	if f.fetchCount != nil {
		fakeSourceFetchMu.Lock()
		f.fetchCount[key]++
		fakeSourceFetchMu.Unlock()
	}
	if err, ok := f.sequenceErrors[key]; ok {
		return model.ProteinSequenceData{}, err
	}
	if sequence, ok := f.nucleotideSeqs[key]; ok {
		return model.ProteinSequenceData{
			Sequence:       sequence,
			OriginalHeader: strings.TrimSpace(f.headers[key]),
		}, nil
	}
	if sequence, ok := f.nucleotideSeqs[sequenceID]; ok {
		return model.ProteinSequenceData{
			Sequence:       sequence,
			OriginalHeader: strings.TrimSpace(f.headers[sequenceID]),
		}, nil
	}
	return model.ProteinSequenceData{}, fmt.Errorf("no nucleotide sequence for %s (%s)", sequenceID, program)
}

type fakeWideKeywordSource struct {
	fakeSource
	normalRows []model.KeywordResultRow
	wideRows   []model.KeywordResultRow
}

func (f fakeWideKeywordSource) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return append([]model.KeywordResultRow(nil), f.normalRows...), nil
}

func (f fakeWideKeywordSource) SearchKeywordRowsWide(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	return append([]model.KeywordResultRow(nil), f.wideRows...), nil
}

type keywordMapSource struct {
	name          string
	candidates    []model.SpeciesCandidate
	rowsByKeyword map[string][]model.KeywordResultRow
	errByKeyword  map[string]error
}

func (f keywordMapSource) Name() string { return firstNonEmpty(f.name, "fake") }
func (f keywordMapSource) FetchSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {
	if len(f.candidates) > 0 {
		return append([]model.SpeciesCandidate(nil), f.candidates...), nil
	}
	return []model.SpeciesCandidate{
		{GenomeLabel: "Arabidopsis thaliana TAIR10", JBrowseName: "Athaliana_TAIR10", SearchAlias: "Arabidopsis thaliana"},
	}, nil
}
func (f keywordMapSource) SubmitBlast(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {
	return model.BlastJob{}, nil
}
func (f keywordMapSource) WaitForBlastResults(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, error) {
	return model.BlastResult{}, nil
}
func (f keywordMapSource) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	if err, ok := f.errByKeyword[keyword]; ok {
		return nil, err
	}
	rows := append([]model.KeywordResultRow(nil), f.rowsByKeyword[keyword]...)
	for i := range rows {
		if rows[i].Genome == "" {
			rows[i].Genome = species.GenomeLabel
		}
	}
	return rows, nil
}
func (f keywordMapSource) FetchProteinSequence(ctx context.Context, targetID int, sequenceID string) (model.ProteinSequenceData, error) {
	return model.ProteinSequenceData{}, nil
}
func (f keywordMapSource) FetchGeneQuerySequence(ctx context.Context, species model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, error) {
	return nil, nil
}

type deadlineInspectingKeywordSource struct {
	lastHadDeadline bool
}

func (f *deadlineInspectingKeywordSource) Name() string { return "deadline-inspecting" }
func (f *deadlineInspectingKeywordSource) FetchSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {
	return nil, nil
}
func (f *deadlineInspectingKeywordSource) SubmitBlast(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {
	return model.BlastJob{}, nil
}
func (f *deadlineInspectingKeywordSource) WaitForBlastResults(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, error) {
	return model.BlastResult{}, nil
}
func (f *deadlineInspectingKeywordSource) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	_, f.lastHadDeadline = ctx.Deadline()
	return []model.KeywordResultRow{{GeneIdentifier: keyword}}, nil
}
func (f *deadlineInspectingKeywordSource) FetchProteinSequence(ctx context.Context, targetID int, sequenceID string) (model.ProteinSequenceData, error) {
	return model.ProteinSequenceData{}, nil
}
func (f *deadlineInspectingKeywordSource) FetchGeneQuerySequence(ctx context.Context, species model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, error) {
	return nil, nil
}

type countingKeywordMapSource struct {
	keywordMapSource
	mu         sync.Mutex
	fetchCount map[string]int
}

func (f *countingKeywordMapSource) Name() string { return f.keywordMapSource.Name() }
func (f *countingKeywordMapSource) FetchSpeciesCandidates(ctx context.Context) ([]model.SpeciesCandidate, error) {
	return f.keywordMapSource.FetchSpeciesCandidates(ctx)
}
func (f *countingKeywordMapSource) SubmitBlast(ctx context.Context, req model.BlastRequest) (model.BlastJob, error) {
	return f.keywordMapSource.SubmitBlast(ctx, req)
}
func (f *countingKeywordMapSource) WaitForBlastResults(ctx context.Context, jobID string, pollInterval time.Duration, timeout time.Duration) (model.BlastResult, error) {
	return f.keywordMapSource.WaitForBlastResults(ctx, jobID, pollInterval, timeout)
}
func (f *countingKeywordMapSource) SearchKeywordRows(ctx context.Context, species model.SpeciesCandidate, keyword string) ([]model.KeywordResultRow, error) {
	f.mu.Lock()
	if f.fetchCount == nil {
		f.fetchCount = make(map[string]int)
	}
	f.fetchCount[keyword]++
	f.mu.Unlock()
	return f.keywordMapSource.SearchKeywordRows(ctx, species, keyword)
}
func (f *countingKeywordMapSource) FetchProteinSequence(ctx context.Context, targetID int, sequenceID string) (model.ProteinSequenceData, error) {
	return f.keywordMapSource.FetchProteinSequence(ctx, targetID, sequenceID)
}
func (f *countingKeywordMapSource) FetchGeneQuerySequence(ctx context.Context, species model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, error) {
	return f.keywordMapSource.FetchGeneQuerySequence(ctx, species, reportType, identifier)
}

type countingUniProtResolverSource struct {
	fakeSource
	mu               sync.Mutex
	accessionFetches map[string]int
	accessionsByID   map[string][]string
}

func (f *countingUniProtResolverSource) FetchUniProtAccessions(ctx context.Context, targetID int, proteinID string) ([]string, error) {
	f.mu.Lock()
	if f.accessionFetches == nil {
		f.accessionFetches = make(map[string]int)
	}
	f.accessionFetches[proteinID]++
	f.mu.Unlock()
	return append([]string(nil), f.accessionsByID[proteinID]...), nil
}

func TestFetchProteinSequenceRecordsSkipsMissingSequencesAndCachesMisses(t *testing.T) {
	fetchCount := map[string]int{}
	w := &BlastWizard{
		source:               fakeSource{sequences: map[string]string{"ok": "MPEPTIDE"}, fetchCount: fetchCount},
		proteinSequenceCache: make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:  make(map[string]error),
	}
	rows := []model.BlastResultRow{
		{Protein: "ok", SequenceID: "ok", Species: "sp"},
		{Protein: "missing", SequenceID: "missing", Species: "sp"},
		{Protein: "missing", SequenceID: "missing", Species: "sp"},
	}
	records, err := w.fetchProteinSequenceRecordsWithProgress(context.Background(), rows, nil)
	if err != nil {
		t.Fatalf("fetchProteinSequenceRecordsWithProgress returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if fetchCount["missing"] != 1 {
		t.Fatalf("missing sequence fetch count = %d, want 1", fetchCount["missing"])
	}
}

func TestLoadKeywordDetailFASTAReturnsFetchedSequenceForTAIRLikeRows(t *testing.T) {
	w := &BlastWizard{
		source: fakeSource{
			name:      "tair",
			sequences: map[string]string{"AT1G01010.1": "MTAIRSEQ"},
			headers:   map[string]string{"AT1G01010.1": ">AT1G01010.1 NAC domain containing protein 1"},
		},
		lastKeywordSpecies:   model.SpeciesCandidate{ProteomeID: 370201, JBrowseName: "TAIR12", GenomeLabel: "TAIR12"},
		proteinSequenceCache: make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:  make(map[string]error),
	}
	row := model.KeywordResultRow{
		SourceDatabase:      "tair",
		SequenceID:          "AT1G01010.1",
		TranscriptID:        "AT1G01010.1",
		GeneIdentifier:      "AT1G01010",
		SequenceHeaderLabel: "TAIR12",
		LabelName:           "NAC001",
	}
	fasta, err := w.loadKeywordDetailFASTA(row)
	if err != nil {
		t.Fatalf("loadKeywordDetailFASTA returned error: %v", err)
	}
	if !strings.Contains(fasta, ">AT1G01010.1 NAC domain containing protein 1") {
		t.Fatalf("detail FASTA header mismatch: %q", fasta)
	}
	if !strings.Contains(fasta, "MTAIRSEQ") {
		t.Fatalf("detail FASTA sequence mismatch: %q", fasta)
	}
}

func TestLoadKeywordDetailFASTAUsesInlineNCBISequence(t *testing.T) {
	fetchCount := map[string]int{}
	w := &BlastWizard{
		source: fakeSource{
			name:       "ncbi",
			fetchCount: fetchCount,
			sequenceErrors: map[string]error{
				"XP_015650724.1": fmt.Errorf("network should not be used"),
			},
		},
		proteinSequenceCache: make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:  make(map[string]error),
	}
	row := model.KeywordResultRow{
		SourceDatabase: "ncbi",
		SequenceID:     "XP_015650724.1",
		ProteinID:      "XP_015650724.1",
		LabelName:      "Os4CL1",
		ExtraColumns: map[string]string{
			"ncbi_fasta_header":     ">XP_015650724.1 probable 4-coumarate--CoA ligase 1",
			"ncbi_protein_sequence": "MNCBISEQ",
		},
	}
	fasta, err := w.loadKeywordDetailFASTA(row)
	if err != nil {
		t.Fatalf("loadKeywordDetailFASTA returned error: %v", err)
	}
	if !strings.Contains(fasta, ">XP_015650724.1 probable 4-coumarate--CoA ligase 1") || !strings.Contains(fasta, "MNCBISEQ") {
		t.Fatalf("detail FASTA did not use inline NCBI payload: %q", fasta)
	}
	if fetchCount["XP_015650724.1"] != 0 {
		t.Fatalf("inline NCBI FASTA should not fetch from network, fetch count = %d", fetchCount["XP_015650724.1"])
	}
}

func TestPrefetchKeywordSequencesSeedsInlineNCBISequence(t *testing.T) {
	fetchCount := map[string]int{}
	w := &BlastWizard{
		source: fakeSource{
			name:       "ncbi",
			fetchCount: fetchCount,
			sequenceErrors: map[string]error{
				"XP_015650724.1": fmt.Errorf("network should not be used"),
			},
		},
		proteinSequenceCache: make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:  make(map[string]error),
	}
	selected := model.SpeciesCandidate{JBrowseName: "ncbi_protein", GenomeLabel: "NCBI Protein"}
	rows := []model.KeywordResultRow{{
		SourceDatabase: "ncbi",
		SequenceID:     "XP_015650724.1",
		ExtraColumns: map[string]string{
			"ncbi_fasta": ">XP_015650724.1 probable protein\nMNCBISEQ\n",
		},
	}}
	results := w.prefetchKeywordSequences(context.Background(), selected, rows, nil)
	got := results["XP_015650724.1"].data.Sequence
	if got != "MNCBISEQ" {
		t.Fatalf("prefetched inline sequence = %q, want MNCBISEQ", got)
	}
	if fetchCount["XP_015650724.1"] != 0 {
		t.Fatalf("inline prefetch should not fetch from network, fetch count = %d", fetchCount["XP_015650724.1"])
	}
	if cached, ok := w.cachedProteinSequence(w.proteinSequenceCacheKey(0, "XP_015650724.1")); !ok || cached.Sequence != "MNCBISEQ" {
		t.Fatalf("inline NCBI sequence was not seeded into cache: %#v ok=%v", cached, ok)
	}
}

func TestNCBIKeywordSnapshotSourceStateAndSequenceCache(t *testing.T) {
	w := &BlastWizard{
		source:               fakeSource{name: "ncbi"},
		proteinSequenceCache: make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:  make(map[string]error),
	}
	selected := model.SpeciesCandidate{JBrowseName: "ncbi_protein", GenomeLabel: "NCBI Protein"}
	groups := []model.KeywordSearchGroup{{
		SearchTerm: "XP_015650724.1",
		SearchType: "NCBI protein accession",
		Rows: []model.KeywordResultRow{{
			SourceDatabase: "ncbi",
			SearchTerm:     "XP_015650724.1",
			SearchType:     "NCBI protein accession",
			SequenceID:     "XP_015650724.1",
			ExtraColumns: map[string]string{
				"ncbi_search_type_id":   "protein",
				"ncbi_result_domain":    "sequence-record",
				"ncbi_entrez_database":  "protein",
				"ncbi_record_type":      "protein",
				"ncbi_engine_schema":    "ncbi-eutilities-keyword-v4",
				"ncbi_uid":              "1022887543",
				"ncbi_accession":        "XP_015650724.1",
				"ncbi_fasta_header":     ">XP_015650724.1 probable 4-coumarate--CoA ligase 1",
				"ncbi_protein_sequence": "MNCBISEQ",
				"ncbi_fasta":            ">XP_015650724.1 probable 4-coumarate--CoA ligase 1\nMNCBISEQ\n",
			},
		}},
	}}
	state := snapshotKeywordSourceState(w.source, groups)
	if state == nil || state.NCBI == nil {
		t.Fatalf("missing NCBI keyword source state: %#v", state)
	}
	if state.NCBI.RecordType != "protein" || state.NCBI.EntrezDatabase != "protein" {
		t.Fatalf("unexpected NCBI snapshot source metadata: %#v", state.NCBI)
	}
	if state.NCBI.EngineSchema != "ncbi-eutilities-keyword-v4" {
		t.Fatalf("engine schema = %q", state.NCBI.EngineSchema)
	}
	if len(state.NCBI.SearchTypeIDs) != 1 || state.NCBI.SearchTypeIDs[0] != "protein" {
		t.Fatalf("search type ids not preserved: %#v", state.NCBI)
	}
	if !slices.Contains(state.NCBI.Accessions, "XP_015650724.1") || !slices.Contains(state.NCBI.UIDs, "1022887543") {
		t.Fatalf("NCBI accessions/uids not preserved: %#v", state.NCBI)
	}
	cache, err := w.snapshotKeywordSequenceCache(context.Background(), selected, flattenKeywordSearchGroups(groups))
	if err != nil {
		t.Fatalf("snapshotKeywordSequenceCache returned error: %v", err)
	}
	if cache == nil || len(cache.Entries) != 1 {
		t.Fatalf("unexpected sequence cache: %#v", cache)
	}
	if cache.Entries[0].Sequence != "MNCBISEQ" || strings.Contains(cache.Entries[0].Sequence, ">") {
		t.Fatalf("NCBI sequence cache should store clean sequence only: %#v", cache.Entries[0])
	}
}

func TestHydrateKeywordSnapshotSourceStateBackfillsNCBIMetadata(t *testing.T) {
	groups := []model.KeywordSearchGroup{{
		Rows: []model.KeywordResultRow{{
			SourceDatabase: "",
			ExtraColumns:   map[string]string{},
		}},
	}}
	hydrateKeywordSnapshotSourceState(groups, &sessionsnapshot.KeywordSourceStateV4{
		Database:     "ncbi",
		SourceKind:   "keyword",
		Engine:       "ncbi-eutilities-keyword",
		ResultDomain: "variant-clinical",
		Extra:        map[string]string{"ncbi_search_type_id": "clinvar"},
		NCBI: &sessionsnapshot.NCBIKeywordSourceV4{
			EntrezDatabase:    "clinvar",
			RecordType:        "clinvar",
			EUtilitiesBaseURL: "https://eutils.ncbi.nlm.nih.gov/entrez/eutils",
			EngineSchema:      "ncbi-eutilities-keyword-v4",
			LinkResolution:    "elink",
			LinkedFromDB:      "gene",
			LinkedToDB:        "clinvar",
			SearchTypeIDs:     []string{"clinvar"},
		},
	})
	row := groups[0].Rows[0]
	if row.SourceDatabase != "ncbi" {
		t.Fatalf("SourceDatabase = %q", row.SourceDatabase)
	}
	for key, want := range map[string]string{
		"ncbi_entrez_database":     "clinvar",
		"ncbi_record_type":         "clinvar",
		"ncbi_eutilities_base_url": "https://eutils.ncbi.nlm.nih.gov/entrez/eutils",
		"ncbi_engine_schema":       "ncbi-eutilities-keyword-v4",
		"ncbi_link_resolution":     "elink",
		"ncbi_linked_from_db":      "gene",
		"ncbi_linked_to_db":        "clinvar",
		"ncbi_result_domain":       "variant-clinical",
		"ncbi_search_type_id":      "clinvar",
	} {
		if got := row.ExtraColumns[key]; got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestApplyNCBIProteinReplacementChoiceUsesNewRows(t *testing.T) {
	selected := model.SpeciesCandidate{JBrowseName: "ncbi_protein", GenomeLabel: "NCBI Protein"}
	src := keywordMapSource{
		name: "ncbi",
		rowsByKeyword: map[string][]model.KeywordResultRow{
			"NP_001409439": {{
				SourceDatabase: "ncbi",
				SearchTerm:     "NP_001409439",
				ProteinID:      "NP_001409439.1",
				SequenceID:     "NP_001409439.1",
				ExtraColumns: map[string]string{
					"ncbi_accession": "NP_001409439.1",
				},
			}},
		},
	}
	w := &BlastWizard{source: src, ncbiReplacementChoice: "new"}
	groups := []model.KeywordSearchGroup{{
		SearchTerm: "XP_015650724.1",
		Rows: []model.KeywordResultRow{{
			SourceDatabase: "ncbi",
			ProteinID:      "XP_015650724.1",
			SequenceID:     "XP_015650724.1",
			ExtraColumns: map[string]string{
				"ncbi_accession":   "XP_015650724.1",
				"ncbi_replaced_by": "NP_001409439",
			},
		}},
	}}

	out, err := w.applyNCBIReplacementChoices(context.Background(), selected, groups)
	if err != nil {
		t.Fatalf("applyNCBIReplacementChoices returned error: %v", err)
	}
	if got := out[0].Rows[0].ProteinID; got != "NP_001409439.1" {
		t.Fatalf("replacement ProteinID = %q, want NP_001409439.1", got)
	}
	if got := out[0].Rows[0].ExtraColumns["ncbi_replacement_decision"]; got != "new" {
		t.Fatalf("replacement decision = %q, want new", got)
	}
	if got := out[0].Rows[0].SearchType; !strings.Contains(got, "accepted NCBI update") {
		t.Fatalf("replacement SearchType = %q, want accepted update marker", got)
	}
	if got := out[0].Rows[0].ExtraColumns["ncbi_requested_accession"]; got != "XP_015650724.1" {
		t.Fatalf("requested accession = %q, want XP_015650724.1", got)
	}
	if got := out[0].Rows[0].SearchTerm; got != "XP_015650724.1" {
		t.Fatalf("replacement SearchTerm = %q, want original keyword XP_015650724.1", got)
	}
}

func TestApplyNCBIProteinReplacementChoiceMarksKeptOldSearchType(t *testing.T) {
	w := &BlastWizard{source: keywordMapSource{name: "ncbi"}, ncbiReplacementChoice: "old"}
	groups := []model.KeywordSearchGroup{{
		SearchTerm: "XP_015650724.1",
		Rows: []model.KeywordResultRow{{
			SourceDatabase: "ncbi",
			SearchType:     "NCBI protein accession",
			ProteinID:      "XP_015650724.1",
			SequenceID:     "XP_015650724.1",
			ExtraColumns: map[string]string{
				"ncbi_accession":   "XP_015650724.1",
				"ncbi_replaced_by": "NP_001409439",
			},
		}},
	}}

	out, err := w.applyNCBIReplacementChoices(context.Background(), model.SpeciesCandidate{}, groups)
	if err != nil {
		t.Fatalf("applyNCBIReplacementChoices returned error: %v", err)
	}
	if got := out[0].Rows[0].SearchType; !strings.Contains(got, "kept old; NCBI update available") {
		t.Fatalf("kept-old SearchType = %q, want update marker", got)
	}
	if out[0].SearchType != out[0].Rows[0].SearchType {
		t.Fatalf("group SearchType should follow row update marker: group=%q row=%q", out[0].SearchType, out[0].Rows[0].SearchType)
	}
}

func TestNCBIGroupReplacementRecognizesNonProteinNCBIAccessionFields(t *testing.T) {
	oldAccession, newAccession, ok := ncbiGroupReplacement(model.KeywordSearchGroup{
		SearchTerm: "RCV000000001",
		Rows: []model.KeywordResultRow{{
			SourceDatabase: "ncbi",
			GeneIdentifier: "8456",
			ExtraColumns: map[string]string{
				"ncbi_clinvar_accession": "RCV000000001",
				"ncbi_replaced_by":       "RCV000000999",
			},
		}},
	})
	if !ok {
		t.Fatal("expected replacement to be detected")
	}
	if oldAccession != "RCV000000001" || newAccession != "RCV000000999" {
		t.Fatalf("replacement = %q -> %q", oldAccession, newAccession)
	}
}

func TestFetchProteinSequenceCachedDoesNotMemoizeTransientErrors(t *testing.T) {
	sequenceErrors := map[string]error{"XP_1": fmt.Errorf("fetch NCBI efetch.fcgi: status 429 body rate limit")}
	fetchCount := map[string]int{}
	source := fakeSource{
		name:           "ncbi",
		sequences:      map[string]string{"XP_1": "MPEPTIDE"},
		sequenceErrors: sequenceErrors,
		fetchCount:     fetchCount,
	}
	w := &BlastWizard{
		source:               source,
		proteinSequenceCache: make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:  make(map[string]error),
	}
	if _, err := w.fetchProteinSequenceCached(context.Background(), 0, "XP_1"); err == nil {
		t.Fatal("expected first transient fetch to fail")
	}
	delete(sequenceErrors, "XP_1")
	data, err := w.fetchProteinSequenceCached(context.Background(), 0, "XP_1")
	if err != nil {
		t.Fatalf("retry should refetch after transient error, got %v", err)
	}
	if data.Sequence != "MPEPTIDE" {
		t.Fatalf("retried sequence = %q, want MPEPTIDE", data.Sequence)
	}
	if fetchCount["XP_1"] != 2 {
		t.Fatalf("fetch count = %d, want 2 real attempts", fetchCount["XP_1"])
	}
}

func TestFetchKeywordProteinSequenceRecordsWithProgressSkipsMissingAndUsesOriginalHeaders(t *testing.T) {
	fetchCount := map[string]int{}
	w := &BlastWizard{
		source: fakeSource{
			name: "tair",
			sequences: map[string]string{
				"AT1G01010.1": "MTAIRSEQ",
			},
			headers: map[string]string{
				"AT1G01010.1": ">AT1G01010.1 NAC domain containing protein 1",
			},
			fetchCount: fetchCount,
		},
		proteinSequenceCache: make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:  make(map[string]error),
	}
	selected := model.SpeciesCandidate{ProteomeID: 370201, JBrowseName: "TAIR12", GenomeLabel: "TAIR12"}
	rows := []model.KeywordResultRow{
		{
			SourceDatabase:      "tair",
			SequenceID:          "AT1G01010.1",
			TranscriptID:        "AT1G01010.1",
			GeneIdentifier:      "AT1G01010",
			SequenceHeaderLabel: "TAIR12",
			LabelName:           "NAC001",
		},
		{
			SourceDatabase:      "tair",
			SequenceID:          "missing-seq",
			TranscriptID:        "missing-seq",
			GeneIdentifier:      "AT1G99999",
			SequenceHeaderLabel: "TAIR12",
			LabelName:           "MISSING",
		},
	}
	records, err := w.fetchKeywordProteinSequenceRecordsWithProgress(context.Background(), selected, rows, nil)
	if err != nil {
		t.Fatalf("fetchKeywordProteinSequenceRecordsWithProgress returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if got := records[0].OriginalHeader; got != ">AT1G01010.1 NAC domain containing protein 1" {
		t.Fatalf("original header = %q, want fetched header", got)
	}
	if got := records[0].Sequence; got != "MTAIRSEQ" {
		t.Fatalf("sequence = %q, want MTAIRSEQ", got)
	}
	if fetchCount["missing-seq"] != 1 {
		t.Fatalf("missing sequence fetch count = %d, want 1", fetchCount["missing-seq"])
	}
}

func TestSnapshotKeywordSequenceCacheOmitsMissingTAIRFastaWithoutFailing(t *testing.T) {
	w := &BlastWizard{
		source: fakeSource{
			name: "tair",
			sequenceErrors: map[string]error{
				"AT1G01010.1": fmt.Errorf("empty TAIR FASTA URL; external fallback: no external protein sequence source matched AT1G01010.1"),
			},
		},
		proteinSequenceCache: make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:  make(map[string]error),
	}
	cache, err := w.snapshotKeywordSequenceCache(context.Background(), model.SpeciesCandidate{ProteomeID: 370201, JBrowseName: "TAIR12"}, []model.KeywordResultRow{{
		SourceDatabase: "tair",
		SequenceID:     "AT1G01010.1",
		TranscriptID:   "AT1G01010.1",
		GeneIdentifier: "AT1G01010",
	}})
	if err != nil {
		t.Fatalf("snapshotKeywordSequenceCache returned error: %v", err)
	}
	if cache == nil || len(cache.Entries) != 0 {
		t.Fatalf("missing FASTA should be omitted from snapshot cache, got %#v", cache)
	}
}

func TestResolveKeywordRowsToBlastItemsSkipsModalWrapperWhenSuppressed(t *testing.T) {
	w := &BlastWizard{
		source:                fakeSource{sequences: map[string]string{"seq1": "MPEPTIDE"}},
		suppressTaskModals:    true,
		proteinSequenceCache:  make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:   make(map[string]error),
		keywordBlastItemCache: make(map[string]blastQueryItem),
	}
	rows := []model.KeywordResultRow{{
		SourceDatabase: "phytozome",
		SequenceID:     "seq1",
		TranscriptID:   "AT1G01010.1",
		GeneIdentifier: "AT1G01010",
		LabelName:      "PAL1",
		Genome:         "Arabidopsis thaliana",
	}}
	items, err := w.resolveKeywordRowsToBlastItems(context.Background(), model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10"}, rows)
	if err != nil {
		t.Fatalf("resolveKeywordRowsToBlastItems returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("resolved items = %d, want 1", len(items))
	}
	if strings.TrimSpace(items[0].Sequence) != "MPEPTIDE" {
		t.Fatalf("resolved item sequence = %q, want MPEPTIDE", items[0].Sequence)
	}
}

func TestResolveTransferredKeywordRowsToBlastItemsUsesRowSourceDatabase(t *testing.T) {
	sourceFetchCount := make(map[string]int)
	targetFetchCount := make(map[string]int)
	w := &BlastWizard{
		source: fakeSource{name: "lemna", sequences: map[string]string{}, fetchCount: targetFetchCount},
		sourceFactory: func(name string) source.DataSource {
			switch strings.ToLower(strings.TrimSpace(name)) {
			case "phytozome":
				return fakeSource{
					name:       "phytozome",
					sequences:  map[string]string{"seq1": "MPEPTIDE"},
					fetchCount: sourceFetchCount,
				}
			case "lemna":
				return fakeSource{
					name:       "lemna",
					sequences:  map[string]string{},
					fetchCount: targetFetchCount,
				}
			default:
				return nil
			}
		},
		suppressTaskModals:    true,
		proteinSequenceCache:  make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:   make(map[string]error),
		keywordBlastItemCache: make(map[string]blastQueryItem),
	}
	rows := []model.KeywordResultRow{{
		SourceDatabase: "phytozome",
		SequenceID:     "seq1",
		TranscriptID:   "AT1G01010.1",
		GeneIdentifier: "AT1G01010",
		LabelName:      "PAL1",
	}}
	items, err := w.resolveTransferredKeywordRowsToBlastItems(context.Background(), model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10"}, rows)
	if err != nil {
		t.Fatalf("resolveTransferredKeywordRowsToBlastItems returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("resolved items = %d, want 1", len(items))
	}
	if sourceFetchCount["seq1"] != 1 {
		t.Fatalf("source fetch count = %d, want 1", sourceFetchCount["seq1"])
	}
	if targetFetchCount["seq1"] != 0 {
		t.Fatalf("target fetch count = %d, want 0", targetFetchCount["seq1"])
	}
	if w.source == nil || !strings.EqualFold(w.source.Name(), "lemna") {
		t.Fatalf("wizard source restored to %v, want lemna", w.source)
	}
}

func TestResolveTransferredBlastRowsToBlastItemsUsesRowSourceDatabase(t *testing.T) {
	sourceFetchCount := make(map[string]int)
	targetFetchCount := make(map[string]int)
	w := &BlastWizard{
		source: fakeSource{name: "lemna", sequences: map[string]string{}, fetchCount: targetFetchCount},
		sourceFactory: func(name string) source.DataSource {
			switch strings.ToLower(strings.TrimSpace(name)) {
			case "phytozome":
				return fakeSource{
					name:       "phytozome",
					sequences:  map[string]string{"seq1": "MPEPTIDE"},
					fetchCount: sourceFetchCount,
				}
			case "lemna":
				return fakeSource{
					name:       "lemna",
					sequences:  map[string]string{},
					fetchCount: targetFetchCount,
				}
			default:
				return nil
			}
		},
		suppressTaskModals:   true,
		proteinSequenceCache: make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:  make(map[string]error),
	}
	rows := []model.BlastResultRow{{
		SourceDatabase: "phytozome",
		SequenceID:     "seq1",
		TranscriptID:   "AT1G01010.1",
		Protein:        "AT1G01010.1",
		TargetID:       167,
		LabelName:      "PAL1",
	}}
	items, err := w.resolveTransferredBlastRowsToBlastItems(context.Background(), model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10"}, rows)
	if err != nil {
		t.Fatalf("resolveTransferredBlastRowsToBlastItems returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("resolved items = %d, want 1", len(items))
	}
	if sourceFetchCount["seq1"] != 1 {
		t.Fatalf("source fetch count = %d, want 1", sourceFetchCount["seq1"])
	}
	if targetFetchCount["seq1"] != 0 {
		t.Fatalf("target fetch count = %d, want 0", targetFetchCount["seq1"])
	}
	if w.source == nil || !strings.EqualFold(w.source.Name(), "lemna") {
		t.Fatalf("wizard source restored to %v, want lemna", w.source)
	}
}

func TestRunTransferredBlastModeKeepsTransferStateForBackNavigation(t *testing.T) {
	w := &BlastWizard{
		source:                 fakeSource{name: "lemna"},
		prompt:                 prompt.New(strings.NewReader(""), io.Discard),
		transferKind:           "unsupported_transfer_kind",
		transferTargetDatabase: "lemna",
		transferKeywordRows: []model.KeywordResultRow{{
			SourceDatabase: "phytozome",
			SequenceID:     "seq1",
			LabelName:      "PAL1",
		}},
	}

	err := w.runTransferredBlastMode(context.Background(), model.SpeciesCandidate{JBrowseName: "target"})
	if err == nil {
		t.Fatalf("runTransferredBlastMode returned nil, want unsupported transfer error")
	}
	if w.transferKind != "unsupported_transfer_kind" {
		t.Fatalf("transferKind = %q, want unsupported_transfer_kind", w.transferKind)
	}
	if w.transferTargetDatabase != "lemna" {
		t.Fatalf("transferTargetDatabase = %q, want lemna", w.transferTargetDatabase)
	}
	if len(w.transferKeywordRows) != 1 {
		t.Fatalf("transferKeywordRows length = %d, want 1", len(w.transferKeywordRows))
	}
}

func TestRunAgainstBlastTargetDatabaseBackReturnsToTransferDatabaseChoice(t *testing.T) {
	choices := []string{"lemna", "phytozome"}
	choiceIndex := 0
	runCount := 0
	selectedDatabases := make([]string, 0, len(choices))
	w := &BlastWizard{
		source: fakeSource{name: "source"},
		prompt: prompt.New(strings.NewReader(""), io.Discard),
		sourceFactory: func(name string) source.DataSource {
			return fakeSource{
				name: name,
				species: []model.SpeciesCandidate{{
					JBrowseName: name + "_species",
					GenomeLabel: name,
				}},
			}
		},
		chooseBlastTargetDB: func() (string, error) {
			if choiceIndex >= len(choices) {
				t.Fatalf("unexpected extra BLAST target database prompt")
			}
			choice := choices[choiceIndex]
			choiceIndex++
			selectedDatabases = append(selectedDatabases, choice)
			return choice, nil
		},
		selectSpeciesHook: func(candidates []model.SpeciesCandidate) (model.SpeciesCandidate, error) {
			if len(candidates) != 1 {
				t.Fatalf("candidate count = %d, want 1", len(candidates))
			}
			return candidates[0], nil
		},
	}

	err := w.runAgainstBlastTargetDatabase(context.Background(), "", nil, func(targetSpecies model.SpeciesCandidate) error {
		runCount++
		if runCount == 1 {
			if targetSpecies.JBrowseName != "lemna_species" {
				t.Fatalf("first target species = %q, want lemna_species", targetSpecies.JBrowseName)
			}
			return prompt.ErrBackToDatabaseSelection
		}
		if targetSpecies.JBrowseName != "phytozome_species" {
			t.Fatalf("second target species = %q, want phytozome_species", targetSpecies.JBrowseName)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("runAgainstBlastTargetDatabase returned error: %v", err)
	}
	if runCount != 2 {
		t.Fatalf("run count = %d, want 2", runCount)
	}
	if !reflect.DeepEqual(selectedDatabases, []string{"lemna", "phytozome"}) {
		t.Fatalf("database choices = %#v, want lemna then phytozome", selectedDatabases)
	}
	if w.source == nil || w.source.Name() != "source" {
		t.Fatalf("source was not restored: %#v", w.source)
	}
}

func TestUniProtAccessionsForBlastRowUsesSingleflightForConcurrentSameRow(t *testing.T) {
	src := &countingUniProtResolverSource{
		fakeSource: fakeSource{},
		accessionsByID: map[string][]string{
			"AT5G13930.1": {"Q12345"},
		},
	}
	w := &BlastWizard{
		source:                    src,
		rowUniProtAccessionsCache: make(map[string][]string),
		rowUniProtAccessionsKnown: make(map[string]bool),
		speciesCandidatesCache: map[string][]model.SpeciesCandidate{
			"fake": {{
				ProteomeID:  167,
				JBrowseName: "Athaliana_TAIR10",
			}},
		},
	}
	row := model.BlastResultRow{
		Protein:     "AT5G13930.1",
		SubjectID:   "AT5G13930.1",
		JBrowseName: "Athaliana_TAIR10",
	}
	const workers = 8
	results := make(chan []string, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- w.uniprotAccessionsForBlastRow(context.Background(), row)
		}()
	}
	wg.Wait()
	close(results)
	for accessions := range results {
		if len(accessions) != 1 || accessions[0] != "Q12345" {
			t.Fatalf("accessions = %#v, want Q12345", accessions)
		}
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.accessionFetches["AT5G13930.1"] != 1 {
		t.Fatalf("FetchUniProtAccessions count = %d, want 1", src.accessionFetches["AT5G13930.1"])
	}
}

func TestLoadBlastDetailFASTAReturnsWrappedFASTA(t *testing.T) {
	w := &BlastWizard{
		source:               fakeSource{sequences: map[string]string{"seq1": "MPEPTIDE"}},
		proteinSequenceCache: make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:  make(map[string]error),
	}
	text, err := w.loadBlastDetailFASTA(model.BlastResultRow{
		Protein:    "prot1",
		SequenceID: "seq1",
		TargetID:   123,
	})
	if err != nil {
		t.Fatalf("loadBlastDetailFASTA returned error: %v", err)
	}
	if !strings.Contains(text, ">prot1") || !strings.Contains(text, "MPEPTIDE") {
		t.Fatalf("unexpected FASTA text: %q", text)
	}
}

func TestLoadBlastDetailFASTAFallsBackToResolvedTargetID(t *testing.T) {
	w := &BlastWizard{
		source:               fakeSource{sequences: map[string]string{"seq1": "MPEPTIDE"}},
		proteinSequenceCache: make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:  make(map[string]error),
		speciesCandidatesCache: map[string][]model.SpeciesCandidate{
			"fake": {{
				ProteomeID:  167,
				JBrowseName: "Athaliana_TAIR10",
			}},
		},
	}
	text, err := w.loadBlastDetailFASTA(model.BlastResultRow{
		Protein:     "prot1",
		SequenceID:  "seq1",
		TargetID:    0,
		JBrowseName: "Athaliana_TAIR10",
	})
	if err != nil {
		t.Fatalf("loadBlastDetailFASTA returned error: %v", err)
	}
	if !strings.Contains(text, "MPEPTIDE") {
		t.Fatalf("unexpected FASTA text: %q", text)
	}
}

func TestFetchProteinSequenceRecordsReturnsNonMissingErrors(t *testing.T) {
	w := &BlastWizard{
		source: fakeSource{
			sequences:      map[string]string{"ok": "MPEPTIDE"},
			sequenceErrors: map[string]error{"net": fmt.Errorf("fetch protein sequence: unexpected status 500")},
		},
		proteinSequenceCache: make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:  make(map[string]error),
	}
	rows := []model.BlastResultRow{
		{Protein: "ok", SequenceID: "ok", Species: "sp"},
		{Protein: "net", SequenceID: "net", Species: "sp"},
	}
	_, err := w.fetchProteinSequenceRecordsWithProgress(context.Background(), rows, nil)
	if err == nil {
		t.Fatal("expected non-missing sequence fetch error")
	}
	if !strings.Contains(err.Error(), "unexpected status 500") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchProteinSequenceRecordsUsesTranscriptFallbackForLemnaBlastRows(t *testing.T) {
	w := &BlastWizard{
		source: fakeSource{
			sequences: map[string]string{
				"Sp9509d011g001470_P001": "MPEPTIDE",
			},
			headers: map[string]string{
				"Sp9509d011g001470_P001": ">Sp9509d011g001470_P001 protein",
			},
		},
		proteinSequenceCache: make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:  make(map[string]error),
	}
	rows := []model.BlastResultRow{
		{
			SourceDatabase: "lemna",
			BlastProgram:   "TBLASTN",
			Protein:        "Sp9509d011g001470_P001",
			SequenceID:     "Sp9509d011g001470_P001",
			TranscriptID:   "Sp9509d011g001470_T001",
			Species:        "Spirodela polyrhiza 9509",
			TargetID:       18,
		},
	}

	records, err := w.fetchProteinSequenceRecordsWithProgress(context.Background(), rows, nil)
	if err != nil {
		t.Fatalf("fetchProteinSequenceRecordsWithProgress returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].Sequence != "MPEPTIDE" {
		t.Fatalf("Sequence = %q, want MPEPTIDE", records[0].Sequence)
	}
	if records[0].OriginalHeader != ">Sp9509d011g001470_P001 protein" {
		t.Fatalf("OriginalHeader = %q, want mapped protein header", records[0].OriginalHeader)
	}
}

func TestFetchProteinSequenceRecordsSupportsAllLemnaBlastPrograms(t *testing.T) {
	w := &BlastWizard{
		source: fakeSource{
			sequences: map[string]string{
				"Sp9509d011g001470_P001":       "MPEPTIDE",
				"Sp9509d020g000340_T001":       "ATGGCC",
				"LOC_Os01g01010.1":             "MSPQQQ",
				"AT2G37040.1":                  "MSTPAL",
				"Sp9509d011g001470_T001":       "ATGAAA",
				"Sp9509d011g001470_subject_id": "ATGCCC",
			},
			headers: map[string]string{
				"Sp9509d011g001470_P001": ">Sp9509d011g001470_P001 protein",
				"LOC_Os01g01010.1":       ">LOC_Os01g01010.1 source header",
			},
		},
		proteinSequenceCache: make(map[string]model.ProteinSequenceData),
		proteinSequenceMiss:  make(map[string]error),
	}

	tests := []struct {
		name               string
		row                model.BlastResultRow
		wantHeader         string
		wantOriginalHeader string
		wantSequence       string
	}{
		{
			name: "blastp protein header fallback",
			row: model.BlastResultRow{
				SourceDatabase: "lemna",
				BlastProgram:   "BLASTP",
				Protein:        "AT2G37040.1",
				SequenceID:     "AT2G37040.1",
				Species:        "Arabidopsis thaliana",
				TargetID:       18,
			},
			wantHeader:         ">AT2G37040.1",
			wantOriginalHeader: ">AT2G37040.1",
			wantSequence:       "MSTPAL",
		},
		{
			name: "blastx original source header preserved",
			row: model.BlastResultRow{
				SourceDatabase: "lemna",
				BlastProgram:   "BLASTX",
				Protein:        "LOC_Os01g01010.1",
				SequenceID:     "LOC_Os01g01010.1",
				Species:        "Oryza sativa",
				TargetID:       18,
			},
			wantHeader:         ">LOC_Os01g01010.1",
			wantOriginalHeader: ">LOC_Os01g01010.1 source header",
			wantSequence:       "MSPQQQ",
		},
		{
			name: "blastn transcript fallback header",
			row: model.BlastResultRow{
				SourceDatabase: "lemna",
				BlastProgram:   "BLASTN",
				SequenceID:     "Sp9509d020g000340_T001",
				TranscriptID:   "Sp9509d020g000340_T001",
				SubjectID:      "Sp9509d020g000340_T001",
				Species:        "Spirodela polyrhiza 9509",
				TargetID:       18,
			},
			wantHeader:         ">Sp9509d020g000340_T001",
			wantOriginalHeader: ">Sp9509d020g000340_T001",
			wantSequence:       "ATGGCC",
		},
		{
			name: "tblastn mapped protein original header preserved",
			row: model.BlastResultRow{
				SourceDatabase: "lemna",
				BlastProgram:   "TBLASTN",
				Protein:        "Sp9509d011g001470_P001",
				SequenceID:     "Sp9509d011g001470_P001",
				TranscriptID:   "Sp9509d011g001470_T001",
				Species:        "Spirodela polyrhiza 9509",
				TargetID:       18,
			},
			wantHeader:         ">Sp9509d011g001470_P001",
			wantOriginalHeader: ">Sp9509d011g001470_P001 protein",
			wantSequence:       "MPEPTIDE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records, err := w.fetchProteinSequenceRecordsWithProgress(context.Background(), []model.BlastResultRow{tt.row}, nil)
			if err != nil {
				t.Fatalf("fetchProteinSequenceRecordsWithProgress returned error: %v", err)
			}
			if len(records) != 1 {
				t.Fatalf("records = %d, want 1", len(records))
			}
			if records[0].Header != tt.wantHeader {
				t.Fatalf("Header = %q, want %q", records[0].Header, tt.wantHeader)
			}
			if records[0].OriginalHeader != tt.wantOriginalHeader {
				t.Fatalf("OriginalHeader = %q, want %q", records[0].OriginalHeader, tt.wantOriginalHeader)
			}
			if records[0].Sequence != tt.wantSequence {
				t.Fatalf("Sequence = %q, want %q", records[0].Sequence, tt.wantSequence)
			}
		})
	}
}

func TestKeywordRowsToBlastItemsCachedReusesBuiltItemWithinCall(t *testing.T) {
	w := &BlastWizard{
		keywordBlastItemCache: make(map[string]blastQueryItem),
	}
	selected := model.SpeciesCandidate{
		ProteomeID:  323,
		JBrowseName: "Osativa_v7_0",
		GenomeLabel: "Oryza sativa v7.0",
	}
	rows := []model.KeywordResultRow{
		{
			SourceDatabase: "phytozome",
			SequenceID:     "Os06g44620.1",
			TranscriptID:   "Os06g44620.1",
			ProteinID:      "Os06g44620.1",
			GeneIdentifier: "Os06g44620",
			GeneReportURL:  "https://example.test/Os06g44620",
			LabelName:      "PAL1",
			PhgoAliases:    "PAL1; ATPAL1",
		},
		{
			SourceDatabase: "phytozome",
			SequenceID:     "Os06g44620.1",
			TranscriptID:   "Os06g44620.1",
			ProteinID:      "Os06g44620.1",
			GeneIdentifier: "Os06g44620",
			GeneReportURL:  "https://example.test/Os06g44620",
			LabelName:      "PAL1",
			PhgoAliases:    "PAL1; ATPAL1",
		},
	}
	sequences := map[string]sequenceFetchResult{
		"Os06g44620.1": {data: model.ProteinSequenceData{Sequence: "MPEPTIDE"}},
	}

	items, converted := w.keywordRowsToBlastItemsCached(context.Background(), selected, rows, sequences)
	if converted != 2 {
		t.Fatalf("converted = %d, want 2", converted)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].QuerySource == nil || items[1].QuerySource == nil {
		t.Fatalf("expected query sources on both items: %#v", items)
	}
	if items[0].QuerySource.ProteinSequence != "MPEPTIDE" || items[1].QuerySource.ProteinSequence != "MPEPTIDE" {
		t.Fatalf("unexpected sequences on cached items: %#v", items)
	}
}

func TestLocalBlastBatchWorkerBudgetDoesNotOversubscribeCPU(t *testing.T) {
	previous := runtime.GOMAXPROCS(8)
	defer runtime.GOMAXPROCS(previous)
	t.Setenv("PHYTOZOME_GO_LOCAL_BLAST_BATCH_WORKERS", "")
	t.Setenv("PHYTOZOME_GO_LOCAL_BLAST_THREADS", "")
	t.Setenv("PHYTOZOME_GO_REMOTE_BLAST_BATCH_WORKERS", "")
	t.Setenv("PHYTOZOME_GO_MAX_WORKERS", "")

	request := model.BlastRequest{Program: "local:BLASTP"}
	workers := batchBlastWorkerCount(65, request)
	if workers <= 0 {
		t.Fatalf("workers = %d, want positive", workers)
	}
	threads := localBlastThreadsPerWorker(workers, request)
	if threads <= 0 {
		t.Fatalf("threads = %d, want positive", threads)
	}
	if workers*threads > 8 {
		t.Fatalf("local BLAST oversubscribed CPU budget: workers=%d threads=%d cpu=8", workers, threads)
	}

	networkWorkers := batchBlastWorkerCount(65, model.BlastRequest{Program: "BLASTP"})
	if networkWorkers != 2 {
		t.Fatalf("remote BLAST workers = %d, want conservative default 2", networkWorkers)
	}
}

func TestLocalBlastDefaultsFavorThreadsOverWorkerFanout(t *testing.T) {
	previous := runtime.GOMAXPROCS(16)
	defer runtime.GOMAXPROCS(previous)
	t.Setenv("PHYTOZOME_GO_LOCAL_BLAST_BATCH_WORKERS", "")
	t.Setenv("PHYTOZOME_GO_LOCAL_BLAST_THREADS", "")

	workers := batchBlastWorkerCount(99, model.BlastRequest{Program: "local:BLASTP"})
	if workers != 2 {
		t.Fatalf("workers = %d, want 2 on 16 CPU budget", workers)
	}

	blastpThreads := localBlastThreadsPerWorker(workers, model.BlastRequest{Program: "local:BLASTP"})
	if blastpThreads != 8 {
		t.Fatalf("blastp threads = %d, want 8", blastpThreads)
	}

	tblastnThreads := localBlastThreadsPerWorker(workers, model.BlastRequest{Program: "local:TBLASTN"})
	if tblastnThreads != 2 {
		t.Fatalf("tblastn threads = %d, want 2", tblastnThreads)
	}
}

func TestLocalBlastDefaultsUseProgramSpecificBatchStrategy(t *testing.T) {
	previous := runtime.GOMAXPROCS(16)
	defer runtime.GOMAXPROCS(previous)
	t.Setenv("PHYTOZOME_GO_LOCAL_BLAST_BATCH_WORKERS", "")
	t.Setenv("PHYTOZOME_GO_LOCAL_BLAST_THREADS", "")

	if got := batchBlastWorkerCount(3, model.BlastRequest{Program: "local:BLASTX"}); got != 1 {
		t.Fatalf("blastx workers = %d, want 1", got)
	}
	if got := batchBlastWorkerCount(3, model.BlastRequest{Program: "local:BLASTN"}); got != 2 {
		t.Fatalf("blastn workers = %d, want 2", got)
	}
	if got := batchBlastWorkerCount(3, model.BlastRequest{Program: "local:TBLASTN"}); got != 2 {
		t.Fatalf("tblastn workers = %d, want 2", got)
	}
	if got := localBlastThreadsPerWorker(2, model.BlastRequest{Program: "local:BLASTN"}); got != 2 {
		t.Fatalf("blastn threads with 2 workers = %d, want 2", got)
	}
	if got := localBlastThreadsPerWorker(2, model.BlastRequest{Program: "local:TBLASTN"}); got != 2 {
		t.Fatalf("tblastn threads with 2 workers = %d, want 2", got)
	}
}

func TestBlastAuxWorkerBudgetsStayBoundedByPhase(t *testing.T) {
	previous := runtime.GOMAXPROCS(8)
	defer runtime.GOMAXPROCS(previous)
	t.Setenv("PHYTOZOME_GO_MAX_WORKERS", "")
	t.Setenv("PHYTOZOME_GO_BLAST_UNIPROT_WORKERS", "")
	t.Setenv("PHYTOZOME_GO_BLAST_UNIPROT_ACCESSION_WORKERS", "")
	t.Setenv("PHYTOZOME_GO_BLAST_INTERPRO_WORKERS", "")
	t.Setenv("PHYTOZOME_GO_BLAST_LABEL_WORKERS", "")
	t.Setenv("PHYTOZOME_GO_BLAST_KEYWORD_TERM_WORKERS", "")
	t.Setenv("PHYTOZOME_GO_BLAST_SEQUENCE_FETCH_WORKERS", "")

	if got := blastUniProtWorkerCount(500); got != 12 {
		t.Fatalf("blastUniProtWorkerCount = %d, want 12", got)
	}
	if got := blastUniProtAccessionWorkerCount(500); got != 16 {
		t.Fatalf("blastUniProtAccessionWorkerCount = %d, want 16", got)
	}
	if got := blastInterProWorkerCount(500); got != 12 {
		t.Fatalf("blastInterProWorkerCount = %d, want 12", got)
	}
	if got := blastLabelWorkerCount(500); got != 16 {
		t.Fatalf("blastLabelWorkerCount = %d, want 16 on 8 CPU budget", got)
	}
	if got := blastKeywordTermWorkerCount(500); got != 16 {
		t.Fatalf("blastKeywordTermWorkerCount = %d, want 16 on 8 CPU budget", got)
	}
	if got := blastSequenceFetchWorkerCount(500); got != 16 {
		t.Fatalf("blastSequenceFetchWorkerCount = %d, want 16 on 8 CPU budget", got)
	}
}

func TestBlastAuxWorkerBudgetsScaleWithReferenceLoad(t *testing.T) {
	previous := runtime.GOMAXPROCS(8)
	defer runtime.GOMAXPROCS(previous)
	t.Setenv("PHYTOZOME_GO_MAX_WORKERS", "")
	t.Setenv("PHYTOZOME_GO_BLAST_UNIPROT_WORKERS", "")
	t.Setenv("PHYTOZOME_GO_BLAST_UNIPROT_ACCESSION_WORKERS", "")
	t.Setenv("PHYTOZOME_GO_BLAST_INTERPRO_WORKERS", "")

	none := externalReferenceConfig{}
	full := externalReferenceConfig{
		AutoLabelBlastHits: true,
		UseUniProt:         true,
		UseInterPro:        true,
	}

	if got := blastUniProtWorkerCountForConfig(500, none); got != 12 {
		t.Fatalf("blastUniProtWorkerCountForConfig(none) = %d, want 12", got)
	}
	if got := blastUniProtWorkerCountForConfig(500, full); got != 18 {
		t.Fatalf("blastUniProtWorkerCountForConfig(full) = %d, want 18", got)
	}
	if got := blastUniProtAccessionWorkerCountForConfig(500, none); got != 16 {
		t.Fatalf("blastUniProtAccessionWorkerCountForConfig(none) = %d, want 16", got)
	}
	if got := blastUniProtAccessionWorkerCountForConfig(500, full); got != 22 {
		t.Fatalf("blastUniProtAccessionWorkerCountForConfig(full) = %d, want 22", got)
	}
	if got := blastInterProWorkerCountForConfig(500, none); got != 12 {
		t.Fatalf("blastInterProWorkerCountForConfig(none) = %d, want 12", got)
	}
	if got := blastInterProWorkerCountForConfig(500, full); got != 18 {
		t.Fatalf("blastInterProWorkerCountForConfig(full) = %d, want 18", got)
	}
}

func TestBlastAuxWorkerBudgetsHonorEnvOverrides(t *testing.T) {
	t.Setenv("PHYTOZOME_GO_BLAST_UNIPROT_WORKERS", "7")
	t.Setenv("PHYTOZOME_GO_BLAST_UNIPROT_ACCESSION_WORKERS", "9")
	t.Setenv("PHYTOZOME_GO_BLAST_INTERPRO_WORKERS", "5")
	t.Setenv("PHYTOZOME_GO_BLAST_LABEL_WORKERS", "11")
	t.Setenv("PHYTOZOME_GO_BLAST_KEYWORD_TERM_WORKERS", "13")
	t.Setenv("PHYTOZOME_GO_BLAST_SEQUENCE_FETCH_WORKERS", "15")

	if got := blastUniProtWorkerCount(100); got != 7 {
		t.Fatalf("blastUniProtWorkerCount override = %d, want 7", got)
	}
	if got := blastUniProtAccessionWorkerCount(100); got != 9 {
		t.Fatalf("blastUniProtAccessionWorkerCount override = %d, want 9", got)
	}
	if got := blastInterProWorkerCount(100); got != 5 {
		t.Fatalf("blastInterProWorkerCount override = %d, want 5", got)
	}
	if got := blastLabelWorkerCount(100); got != 11 {
		t.Fatalf("blastLabelWorkerCount override = %d, want 11", got)
	}
	if got := blastKeywordTermWorkerCount(100); got != 13 {
		t.Fatalf("blastKeywordTermWorkerCount override = %d, want 13", got)
	}
	if got := blastSequenceFetchWorkerCount(100); got != 15 {
		t.Fatalf("blastSequenceFetchWorkerCount override = %d, want 15", got)
	}
}

func TestTAIR12ENAKeywordWorkerBudgetUsesMeasuredHighConcurrency(t *testing.T) {
	t.Setenv("PHGO_TAIR_ENA_KEYWORD_WORKERS", "")
	src := fakeSource{name: "tair"}
	tair12 := model.SpeciesCandidate{JBrowseName: "TAIR12", GenomeLabel: "TAIR12"}
	if got := keywordSearchWorkerCountForSource(src, tair12, 200); got != 96 {
		t.Fatalf("TAIR12 ENA keyword workers = %d, want measured default 96", got)
	}
	if got := keywordSearchWorkerCountForSource(src, model.SpeciesCandidate{JBrowseName: "TAIR10"}, 200); got != keywordSearchWorkerCount(200) {
		t.Fatalf("non-ENA TAIR worker budget changed unexpectedly: %d", got)
	}

	t.Setenv("PHGO_TAIR_ENA_KEYWORD_WORKERS", "32")
	if got := keywordSearchWorkerCountForSource(src, tair12, 200); got != 32 {
		t.Fatalf("TAIR12 ENA keyword env override = %d, want 32", got)
	}
}

func TestAlignPreparedBlastItemsToRequestResolvesProgramSpecificSequenceKinds(t *testing.T) {
	w := &BlastWizard{
		source: fakeSource{
			sequences: map[string]string{
				"protA": "MPEPTIDE",
			},
			nucleotideSeqs: map[string]string{
				"blastn|txA":  "ATGGCCATGGCC",
				"blastx|txA":  "ATGGCCATGGCC",
				"tblastn|txA": "ATGGCCATGGCC",
			},
		},
	}
	baseItem := blastQueryItem{
		LabelName: "C4H",
		Sequence:  "MPEPTIDE",
		QuerySource: &model.QuerySequenceSource{
			Sequence:         "MPEPTIDE",
			SourceDatabase:   "lemna.org",
			SourceProteomeID: 18,
			TranscriptID:     "txA",
			ProteinID:        "protA",
			GeneID:           "geneA",
		},
	}

	tests := []struct {
		name         string
		request      model.BlastRequest
		wantSequence string
		wantKind     model.SequenceKind
	}{
		{
			name:         "blastn-uses-dna",
			request:      model.BlastRequest{Program: "local:BLASTN", SequenceKind: model.SequenceDNA},
			wantSequence: "ATGGCCATGGCC",
			wantKind:     model.SequenceDNA,
		},
		{
			name:         "blastx-uses-dna",
			request:      model.BlastRequest{Program: "local:BLASTX", SequenceKind: model.SequenceDNA},
			wantSequence: "ATGGCCATGGCC",
			wantKind:     model.SequenceDNA,
		},
		{
			name:         "tblastn-keeps-protein",
			request:      model.BlastRequest{Program: "local:TBLASTN", SequenceKind: model.SequenceProtein},
			wantSequence: "MPEPTIDE",
			wantKind:     model.SequenceProtein,
		},
		{
			name:         "blastp-keeps-protein",
			request:      model.BlastRequest{Program: "local:BLASTP", SequenceKind: model.SequenceProtein},
			wantSequence: "MPEPTIDE",
			wantKind:     model.SequenceProtein,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := w.alignPreparedBlastItemsToRequest(context.Background(), []blastQueryItem{baseItem}, tt.request)
			if err != nil {
				t.Fatalf("alignPreparedBlastItemsToRequest returned error: %v", err)
			}
			if len(out) != 1 {
				t.Fatalf("aligned items = %d, want 1", len(out))
			}
			if out[0].Sequence != tt.wantSequence {
				t.Fatalf("Sequence = %q, want %q", out[0].Sequence, tt.wantSequence)
			}
			if out[0].QuerySource == nil || out[0].QuerySource.Sequence != tt.wantSequence {
				t.Fatalf("QuerySource.Sequence = %q, want %q", out[0].QuerySource.Sequence, tt.wantSequence)
			}
			if got := detectSequenceKind(out[0].Sequence); got != tt.wantKind {
				t.Fatalf("detectSequenceKind(%q) = %s, want %s", out[0].Sequence, got, tt.wantKind)
			}
		})
	}
}

func TestAlignPreparedBlastItemsToRequestDeduplicatesSequenceFetches(t *testing.T) {
	fetchCount := map[string]int{}
	w := &BlastWizard{
		source: fakeSource{
			nucleotideSeqs: map[string]string{
				"blastn|txA": "ATGGCCATGGCC",
			},
			fetchCount: fetchCount,
		},
	}
	items := []blastQueryItem{
		{
			LabelName: "C4H-1",
			QuerySource: &model.QuerySequenceSource{
				Sequence:         "MPEPTIDE",
				SourceDatabase:   "lemna.org",
				SourceProteomeID: 18,
				TranscriptID:     "txA",
				ProteinID:        "protA",
			},
		},
		{
			LabelName: "C4H-2",
			QuerySource: &model.QuerySequenceSource{
				Sequence:         "MPEPTIDE",
				SourceDatabase:   "lemna.org",
				SourceProteomeID: 18,
				TranscriptID:     "txA",
				ProteinID:        "protA",
			},
		},
	}

	out, err := w.alignPreparedBlastItemsToRequest(context.Background(), items, model.BlastRequest{
		Program:      "local:BLASTN",
		SequenceKind: model.SequenceDNA,
	})
	if err != nil {
		t.Fatalf("alignPreparedBlastItemsToRequest returned error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("aligned items = %d, want 2", len(out))
	}
	for i := range out {
		if out[i].Sequence != "ATGGCCATGGCC" {
			t.Fatalf("item %d sequence = %q, want DNA sequence", i, out[i].Sequence)
		}
	}
	if fetchCount["blastn|txA"] != 1 {
		t.Fatalf("FetchNucleotideSequence count = %d, want 1 deduped fetch", fetchCount["blastn|txA"])
	}
}

func TestAlignPreparedBlastItemsToRequestReusesStoredSequenceVariants(t *testing.T) {
	w := &BlastWizard{
		source: fakeSource{
			fetchCount: map[string]int{},
		},
	}
	items := []blastQueryItem{
		{
			LabelName:          "C4H-1",
			ProteinSequence:    "MPEPTIDE",
			NucleotideSequence: "ATGGCCATGGCC",
			QuerySource: &model.QuerySequenceSource{
				ProteinSequence:     "MPEPTIDE",
				NucleotideSequence:  "ATGGCCATGGCC",
				PreferredSequenceID: "txA",
				SourceProteomeID:    18,
				TranscriptID:        "txA",
				ProteinID:           "protA",
			},
		},
	}

	out, err := w.alignPreparedBlastItemsToRequest(context.Background(), items, model.BlastRequest{
		Program:      "local:BLASTN",
		SequenceKind: model.SequenceDNA,
	})
	if err != nil {
		t.Fatalf("alignPreparedBlastItemsToRequest returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("aligned items = %d, want 1", len(out))
	}
	if out[0].Sequence != "ATGGCCATGGCC" {
		t.Fatalf("Sequence = %q, want cached DNA", out[0].Sequence)
	}
	if out[0].QuerySource == nil || out[0].QuerySource.Sequence != "ATGGCCATGGCC" {
		t.Fatalf("QuerySource.Sequence = %q, want cached DNA", out[0].QuerySource.Sequence)
	}
}

func TestResolveGeneReportSequencePreservesInputURLs(t *testing.T) {
	w := &BlastWizard{}
	src := fakeSource{
		query: &model.QuerySequenceSource{
			Sequence:       "MPEPTIDE",
			SourceDatabase: "phytozome",
			GeneID:         "AT2G30490",
		},
	}

	got, err := w.resolveGeneReportSequence(
		context.Background(),
		src,
		model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10"},
		"gene",
		"AT2G30490",
		"https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490?copied=1",
		"https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
	)
	if err != nil {
		t.Fatalf("resolveGeneReportSequence returned error: %v", err)
	}
	if got.OriginalInputURL != "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490?copied=1" {
		t.Fatalf("unexpected original input URL: %q", got.OriginalInputURL)
	}
	if got.NormalizedURL != "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490" {
		t.Fatalf("unexpected normalized URL: %q", got.NormalizedURL)
	}
	if got.SourceProteomeID != 167 || got.SourceJBrowseName != "Athaliana_TAIR10" {
		t.Fatalf("unexpected source species metadata: %#v", got)
	}
}

func TestResolveProteinReportSequencePreservesInputURLs(t *testing.T) {
	w := &BlastWizard{}
	src := fakeSource{
		query: &model.QuerySequenceSource{
			Sequence:       "MPEPTIDE",
			SourceDatabase: "phytozome",
			GeneID:         "Spipo15G0028500",
			TranscriptID:   "Spipo15G0028500",
		},
	}

	got, err := w.resolveGeneReportSequence(
		context.Background(),
		src,
		model.SpeciesCandidate{ProteomeID: 290, JBrowseName: "S_polyrhiza_v2"},
		"protein",
		"Spipo15G0028500",
		"https://phytozome-next.jgi.doe.gov/report/protein/S_polyrhiza_v2/Spipo15G0028500?copied=1",
		"https://phytozome-next.jgi.doe.gov/report/protein/S_polyrhiza_v2/Spipo15G0028500",
	)
	if err != nil {
		t.Fatalf("resolveGeneReportSequence returned error: %v", err)
	}
	if got.ProteinID != "Spipo15G0028500" {
		t.Fatalf("unexpected protein ID: %q", got.ProteinID)
	}
	if got.OriginalInputURL != "https://phytozome-next.jgi.doe.gov/report/protein/S_polyrhiza_v2/Spipo15G0028500?copied=1" {
		t.Fatalf("unexpected original input URL: %q", got.OriginalInputURL)
	}
	if got.NormalizedURL != "https://phytozome-next.jgi.doe.gov/report/protein/S_polyrhiza_v2/Spipo15G0028500" {
		t.Fatalf("unexpected normalized URL: %q", got.NormalizedURL)
	}
	if got.SourceProteomeID != 290 || got.SourceJBrowseName != "S_polyrhiza_v2" {
		t.Fatalf("unexpected source species metadata: %#v", got)
	}
}

func TestInterProQueryLookupRowCarriesQuerySourceMetadata(t *testing.T) {
	w := &BlastWizard{}
	item := blastQueryItem{
		QuerySource: &model.QuerySequenceSource{
			SourceDatabase:    "phytozome",
			SourceProteomeID:  167,
			SourceJBrowseName: "Athaliana_TAIR10",
			ProteinID:         "AT2G30490.1",
			TranscriptID:      "AT2G30490.1",
			NormalizedURL:     "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
			Annotation:        "Cytochrome P450",
			OrganismShort:     "A.thaliana",
			UniProtAccession:  "Q43158",
		},
	}

	row := w.interProQueryLookupRow(item, context.Background())
	if row.TargetID != 167 {
		t.Fatalf("TargetID = %d, want 167", row.TargetID)
	}
	if row.JBrowseName != "Athaliana_TAIR10" {
		t.Fatalf("JBrowseName = %q, want Athaliana_TAIR10", row.JBrowseName)
	}
	if row.Protein != "AT2G30490.1" {
		t.Fatalf("Protein = %q, want query protein id", row.Protein)
	}
	if strings.TrimSpace(row.UniProtAccession) == "" {
		t.Fatalf("UniProtAccession = %q, want non-empty accession from query source or resolver", row.UniProtAccession)
	}
}

func TestEnrichBlastRowsWithUniProtProgressReportsPrefetchPhase(t *testing.T) {
	w := &BlastWizard{}
	rows := []model.BlastResultRow{{UniProtAccession: "Q43158", Protein: "Q43158"}}
	messages := make([]string, 0, 4)
	got, err := w.enrichBlastRowsWithUniProtProgress(context.Background(), uniprot.NewClient(defaultHTTPClient()), rows, func(current int, message string) {
		messages = append(messages, message)
	})
	if err != nil {
		t.Fatalf("enrichBlastRowsWithUniProtProgress returned error: %v", err)
	}
	if len(got) != 1 || !got[0].UniProtReferenceEnabled {
		t.Fatalf("unexpected UniProt enrichment result: %#v", got)
	}
	foundPrefetch := false
	foundResolve := false
	for _, message := range messages {
		if strings.Contains(message, "Prefetching UniProt accessions") {
			foundPrefetch = true
		}
		if strings.Contains(message, "Resolving UniProt references") {
			foundResolve = true
		}
	}
	if !foundPrefetch || !foundResolve {
		t.Fatalf("progress messages = %#v, want prefetch and resolve phases", messages)
	}
}

func TestKeywordRowsToBlastItemsPreservesKeywordMetadata(t *testing.T) {
	rows := []model.KeywordResultRow{{
		SourceDatabase:      "lemna",
		LabelName:           "Os4CL1",
		PhgoAliases:         "Os4CL1; 4CL1",
		Aliases:             "4CL1; Os4CL1",
		AutoDefine:          "4CL1",
		UniProt:             "P41636",
		GeneIdentifier:      "Sp9509d011g001470",
		TranscriptID:        "Sp9509d011g001470_T001",
		ProteinID:           "Sp9509d011g001470_T001",
		SequenceID:          "Sp9509d011g001470_T001",
		SequenceHeaderLabel: "Spirodela polyrhiza",
		Genome:              "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
		Description:         "4-coumarate--CoA ligase",
		Comments:            "AHDR note",
		GeneReportURL:       "https://www.lemna.org/jbrowse2/?assembly=Sp9509d&config=https%3A%2F%2Fwww.lemna.org%2Fjbrowse2%2Fconfig.json&filter=Sp9509d011g001470&highlight=Sp9509d011g001470&phgo_gene=Sp9509d011g001470&phgo_root=Sp_polyrhiza_9509",
	}}
	items := keywordRowsToBlastItems(model.SpeciesCandidate{
		ProteomeID:  18,
		JBrowseName: "Sp_polyrhiza_9509",
		GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
		SearchAlias: "Spirodela polyrhiza",
	}, rows, map[string]sequenceFetchResult{
		"Sp9509d011g001470_T001": {data: model.ProteinSequenceData{Sequence: "MPEPTIDE"}},
	})
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].QuerySource == nil {
		t.Fatal("expected query source metadata")
	}
	if !items[0].FromKeyword {
		t.Fatal("expected keyword-origin marker")
	}
	source := items[0].QuerySource
	if source.LabelName != "Os4CL1" {
		t.Fatalf("LabelName = %q, want keyword label", source.LabelName)
	}
	if source.PhgoAliases != "Os4CL1; 4CL1" {
		t.Fatalf("PhgoAliases = %q, want keyword phgo aliases", source.PhgoAliases)
	}
	if source.Aliases != "" {
		t.Fatalf("Aliases = %q, want no source alias transfer into BLAST", source.Aliases)
	}
	if source.AutoDefine != "" {
		t.Fatalf("AutoDefine = %q, want no auto_define transfer into BLAST", source.AutoDefine)
	}
	if source.UniProtAccession != "P41636" {
		t.Fatalf("UniProtAccession = %q, want P41636", source.UniProtAccession)
	}
	if source.OriginalInputURL != rows[0].GeneReportURL || source.NormalizedURL != rows[0].GeneReportURL {
		t.Fatalf("expected gene report URL to be preserved, got %#v", source)
	}
	if source.OrganismShort != "Spirodela polyrhiza" {
		t.Fatalf("OrganismShort = %q, want sequence header label", source.OrganismShort)
	}
	if source.Annotation != "4-coumarate--CoA ligase" {
		t.Fatalf("Annotation = %q, want description", source.Annotation)
	}
}

func TestKeywordRowsToBlastItemsDoesNotMergePhytozomeSymbolsOrSynonyms(t *testing.T) {
	rows := []model.KeywordResultRow{{
		SourceDatabase: "phytozome",
		LabelName:      "PAL1",
		PhgoAliases:    "PAL1; ATPAL1",
		Symbols:        "ATPAL1",
		Synonyms:       "PAL1; PAL2",
		AutoDefine:     "phenylalanine ammonia-lyase",
		TranscriptID:   "AT2G37040.1",
		GeneIdentifier: "AT2G37040",
		SequenceID:     "AT2G37040.1",
		GeneReportURL:  "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G37040",
	}}
	items := keywordRowsToBlastItems(model.SpeciesCandidate{
		ProteomeID:  167,
		JBrowseName: "Athaliana_TAIR10",
		GenomeLabel: "Arabidopsis thaliana TAIR10",
	}, rows, map[string]sequenceFetchResult{
		"AT2G37040.1": {data: model.ProteinSequenceData{Sequence: "MPEPTIDE"}},
	})
	if len(items) != 1 || items[0].QuerySource == nil {
		t.Fatalf("items = %#v, want one query source", items)
	}
	source := items[0].QuerySource
	if source.PhgoAliases != "PAL1; ATPAL1" {
		t.Fatalf("PhgoAliases = %q, want stored labelname aliases", source.PhgoAliases)
	}
	if source.Aliases != "" || source.AutoDefine != "" {
		t.Fatalf("unexpected source alias transfer: aliases=%q auto_define=%q", source.Aliases, source.AutoDefine)
	}
	rowsWithSource := prepareBlastRowsForReferences([]model.BlastResultRow{{Protein: "hit-1"}}, items[0], model.BlastRequest{
		Species:      model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10"},
		Sequence:     "MPEPTIDE",
		Program:      "BLASTP",
		SequenceKind: model.SequenceProtein,
	}, "phytozome")
	if got := rowsWithSource[0].LabelName; got != "" {
		t.Fatalf("hit LabelName = %q, want keyword label not copied to BLAST hit label_name", got)
	}
	if got := rowsWithSource[0].BlastLabelName; got != "PAL1" {
		t.Fatalf("BlastLabelName = %q, want keyword query label", got)
	}
	if got := rowsWithSource[0].BlastGeneID; got != "AT2G37040.1" {
		t.Fatalf("BlastGeneID = %q, want keyword query transcript id", got)
	}
	if !keywordBlastItemsHaveReusableAliases(items) {
		t.Fatal("expected keyword phgo_alias to be reusable")
	}
}

func TestCachedKeywordBlastItemDropsLegacySourceAliases(t *testing.T) {
	w := &BlastWizard{
		keywordBlastItemCache: make(map[string]blastQueryItem),
	}
	cacheKey := "legacy"
	w.keywordBlastItemCache[cacheKey] = blastQueryItem{
		Sequence: "OLD",
		QuerySource: &model.QuerySequenceSource{
			Sequence:    "OLD",
			LabelName:   "PAL1",
			PhgoAliases: "PAL1; ATPAL1",
			Aliases:     "RAW_ALIAS",
			Symbols:     "RAW_SYMBOL",
			Synonyms:    "RAW_SYNONYM",
			AutoDefine:  "raw auto define",
		},
	}

	item, ok := w.cachedKeywordBlastItem(cacheKey, "MPEPTIDE")
	if !ok || item.QuerySource == nil {
		t.Fatalf("cachedKeywordBlastItem returned %#v, %v", item, ok)
	}
	source := item.QuerySource
	if source.Sequence != "MPEPTIDE" || item.Sequence != "MPEPTIDE" {
		t.Fatalf("cached sequence was not refreshed: item=%q source=%q", item.Sequence, source.Sequence)
	}
	if source.PhgoAliases != "PAL1; ATPAL1" {
		t.Fatalf("PhgoAliases = %q, want preserved symbol-name aliases", source.PhgoAliases)
	}
	if source.Aliases != "" || source.Symbols != "" || source.Synonyms != "" || source.AutoDefine != "" {
		t.Fatalf("legacy source aliases were not cleaned: %#v", source)
	}
}

func TestSupplementBlastAliasesPreservesKeywordQueryLabels(t *testing.T) {
	w := &BlastWizard{
		blastLabelLookupCache: make(map[string]blastAutoLabelResult),
	}
	items := []blastQueryItem{
		{
			LabelName:   "PAL1",
			FromKeyword: true,
			QuerySource: &model.QuerySequenceSource{
				LabelName:   "PAL1",
				PhgoAliases: "PAL1; ATPAL1",
				ProteinID:   "AT2G37040.1",
			},
		},
		{
			QuerySource: &model.QuerySequenceSource{
				ProteinID: "AT2G30490.1",
			},
		},
	}
	src := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT2G30490.1": {{SourceDatabase: "phytozome", Synonyms: "C4H; CYP73A5"}},
			},
		},
	}
	out := cloneBlastQueryItems(items)
	result := w.autoIdentifyBlastLabelResultForTask(context.Background(), src, model.SpeciesCandidate{
		ProteomeID:  167,
		JBrowseName: "Athaliana_TAIR10",
		GenomeLabel: "Arabidopsis thaliana TAIR10",
	}, out[1], time.Now().UTC().Format(time.RFC3339Nano), 1)
	setBlastQueryItemLabel(&out[1], result.Label)
	mergeBlastQueryItemAliases(&out[1], result.Aliases)
	if out[0].LabelName != "PAL1" || out[0].QuerySource.LabelName != "PAL1" {
		t.Fatalf("locked keyword label changed: %#v", out[0])
	}
	if out[0].QuerySource.PhgoAliases != "PAL1; ATPAL1" {
		t.Fatalf("locked keyword phgo aliases changed: %q", out[0].QuerySource.PhgoAliases)
	}
	if out[1].LabelName == "" {
		t.Fatalf("missing-label item was not auto identified: %#v", out[1])
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.fetchCount["AT2G37040.1"] != 0 {
		t.Fatalf("keyword item with reusable aliases triggered label lookup %d times", src.fetchCount["AT2G37040.1"])
	}
	if src.fetchCount["AT2G30490.1"] != 0 {
		t.Fatalf("missing-label item lookup count = %d, want 0", src.fetchCount["AT2G30490.1"])
	}
}

func TestAutoIdentifyBlastLabelResultForPhgoFastaKeepsPinnedLabelAndRanksAliases(t *testing.T) {
	w := &BlastWizard{
		blastLabelLookupCache: make(map[string]blastAutoLabelResult),
	}
	src := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT2G30490": {{SourceDatabase: "phytozome", Synonyms: "C4H; CYP73A5"}},
			},
		},
	}
	item, err := parseBlastQueryRecord(">phgo://Sp7498/PAL1/AT2G30490\nMPEPTIDE\n")
	if err != nil {
		t.Fatalf("parseBlastQueryRecord returned error: %v", err)
	}
	if item.QuerySource == nil {
		t.Fatal("expected FASTA query source")
	}
	if strings.TrimSpace(item.QuerySource.PhgoAliases) != "" {
		t.Fatalf("phgo parse should not prefill aliases: %#v", item.QuerySource)
	}
	result := w.autoIdentifyBlastLabelResultForTask(context.Background(), src, model.SpeciesCandidate{
		ProteomeID:  167,
		JBrowseName: "Athaliana_TAIR10",
		GenomeLabel: "Arabidopsis thaliana TAIR10",
	}, item, time.Now().UTC().Format(time.RFC3339Nano), 0)
	if result.Label != "PAL1" {
		t.Fatalf("pinned phgo label changed: %#v", result)
	}
	if len(result.Aliases) == 0 {
		t.Fatalf("expected ranked aliases for phgo FASTA item: %#v", result)
	}
	if result.Aliases[0] != "PAL1" {
		t.Fatalf("expected pinned label to stay first in ranked aliases: %#v", result)
	}
	if !containsString(result.Aliases, "CYP73A5") {
		t.Fatalf("expected alias ranking to include gene_info symbol aliases: %#v", result)
	}
}

func TestAutoIdentifyBlastLabelResultDoesNotQueryPhytozomeRows(t *testing.T) {
	lookupSource := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT2G30490.1": {{LabelName: "C4H", Aliases: "C4H; CYP73A5"}},
			},
		},
	}
	w := &BlastWizard{
		blastLabelLookupCache: make(map[string]blastAutoLabelResult),
	}
	item := blastQueryItem{
		QuerySource: &model.QuerySequenceSource{
			SourceDatabase: "tair",
			ProteinID:      "AT2G30490.1",
			TranscriptID:   "AT2G30490.1",
		},
	}
	species := model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis thaliana TAIR10"}

	first := w.autoIdentifyBlastLabelResult(context.Background(), lookupSource, species, item)
	second := w.autoIdentifyBlastLabelResult(context.Background(), lookupSource, species, item)
	if strings.TrimSpace(first.Label) == "" || strings.TrimSpace(second.Label) == "" {
		t.Fatalf("expected non-empty cached label results: first=%#v second=%#v", first, second)
	}
	if first.Label != second.Label {
		t.Fatalf("cached labels should match: first=%#v second=%#v", first, second)
	}
	lookupSource.mu.Lock()
	defer lookupSource.mu.Unlock()
	if lookupSource.fetchCount["AT2G30490.1"] != 0 {
		t.Fatalf("symbolname auto-identify should not query Phytozome, got %d", lookupSource.fetchCount["AT2G30490.1"])
	}
}

func TestAutoIdentifyBlastLabelsStoresStableSourceLabelCacheKey(t *testing.T) {
	w := &BlastWizard{
		source:                keywordMapSource{name: "tair"},
		suppressTaskModals:    true,
		blastLabelLookupCache: make(map[string]blastAutoLabelResult),
	}
	species := model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis thaliana TAIR10"}
	item := blastQueryItem{
		QuerySource: &model.QuerySequenceSource{
			SourceDatabase: "tair",
			ProteinID:      "AT2G37040.1",
			TranscriptID:   "AT2G37040.1",
			GeneID:         "AT2G37040",
		},
	}
	stableKey := w.blastLabelLookupKey(w.source, species, item)
	out, err := w.autoIdentifyBlastLabelsWithProgress(context.Background(), species, []blastQueryItem{item})
	if err != nil {
		t.Fatalf("autoIdentifyBlastLabelsWithProgress returned error: %v", err)
	}
	if len(out) != 1 || out[0].LabelName != "PAL1" {
		t.Fatalf("auto-identified items = %#v, want PAL1", out)
	}
	if _, ok := w.cachedBlastLabelLookupByKey(stableKey); !ok {
		t.Fatalf("stable pre-mutation cache key was not stored")
	}
	mutatedKey := w.blastLabelLookupKey(w.source, species, out[0])
	if mutatedKey != stableKey {
		t.Fatalf("blast source label key changed after applying label/aliases: before=%q after=%q", stableKey, mutatedKey)
	}
}

func TestSupplementBlastAliasesReusesBlastSourceLabelCache(t *testing.T) {
	w := &BlastWizard{
		source:                keywordMapSource{name: "tair"},
		suppressTaskModals:    true,
		blastLabelLookupCache: make(map[string]blastAutoLabelResult),
	}
	species := model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis thaliana TAIR10"}
	item := blastQueryItem{
		LabelName: "manual-label",
		QuerySource: &model.QuerySequenceSource{
			SourceDatabase: "tair",
			ProteinID:      "AT2G37040.1",
			TranscriptID:   "AT2G37040.1",
			GeneID:         "AT2G37040",
		},
	}
	key := w.blastLabelLookupKey(w.source, species, item)
	w.storeBlastLabelLookupByKey(key, blastAutoLabelResult{
		Label:   "PAL1",
		Aliases: []string{"PAL1", "ATPAL1"},
	})
	out, err := w.supplementBlastAliases(context.Background(), context.Background(), nil, species, []blastQueryItem{item}, nil)
	if err != nil {
		t.Fatalf("supplementBlastAliases returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("supplemented item count = %d, want 1", len(out))
	}
	if out[0].LabelName != "manual-label" {
		t.Fatalf("supplement aliases changed existing label to %q", out[0].LabelName)
	}
	aliases := labelname.SplitAliases(out[0].QuerySource.PhgoAliases)
	if !containsString(aliases, "PAL1") || !containsString(aliases, "ATPAL1") {
		t.Fatalf("PhgoAliases = %#v, want cached symbol-name aliases", aliases)
	}
}

func TestAutoIdentifyBlastLabelResultConcurrentCallsDoNotQueryPhytozomeRows(t *testing.T) {
	lookupSource := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT2G30490.1": {{LabelName: "C4H", Aliases: "C4H; CYP73A5"}},
			},
		},
	}
	w := &BlastWizard{
		blastLabelLookupCache: make(map[string]blastAutoLabelResult),
		keywordTermRowsCache:  make(map[string][]model.KeywordResultRow),
	}
	item := blastQueryItem{
		QuerySource: &model.QuerySequenceSource{
			SourceDatabase: "tair",
			ProteinID:      "AT2G30490.1",
			TranscriptID:   "AT2G30490.1",
		},
	}
	species := model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis thaliana TAIR10"}
	var wg sync.WaitGroup
	results := make([]blastAutoLabelResult, 2)
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = w.autoIdentifyBlastLabelResult(context.Background(), lookupSource, species, item)
		}(i)
	}
	wg.Wait()
	for i, got := range results {
		if strings.TrimSpace(got.Label) == "" {
			t.Fatalf("concurrent blast label result %d missing label: %#v", i, got)
		}
	}
	lookupSource.mu.Lock()
	defer lookupSource.mu.Unlock()
	if lookupSource.fetchCount["AT2G30490.1"] != 0 {
		t.Fatalf("symbolname auto-identify should not query Phytozome, got %d", lookupSource.fetchCount["AT2G30490.1"])
	}
}

func TestParseBlastLoadCommand(t *testing.T) {
	filename, ok := parseBlastLoadCommand(`load "queries.txt"`)
	if !ok {
		t.Fatalf("expected load command to parse")
	}
	if filename != "queries.txt" {
		t.Fatalf("unexpected filename: %q", filename)
	}
}

func TestParseBlastLoadCommandAcceptsFastaExtensions(t *testing.T) {
	filename, ok := parseBlastLoadCommand(`load "queries.fasta"`)
	if !ok || filename != "queries.fasta" {
		t.Fatalf("unexpected fasta filename parse: %q ok=%v", filename, ok)
	}
	filename, ok = parseBlastLoadCommand(`load "queries.fa"`)
	if !ok || filename != "queries.fa" {
		t.Fatalf("unexpected fa filename parse: %q ok=%v", filename, ok)
	}
}

func TestAvailableBlastProgramsRequireLocalCapabilities(t *testing.T) {
	serverOnly := lemna.BlastCapability{
		HasServerNucleotideDB:  true,
		BlastNDBID:             12,
		HasServerProteinDB:     true,
		ProteinDBID:            34,
		ServerBlastNAvailable:  true,
		ServerBlastXAvailable:  true,
		ServerTBlastNAvailable: true,
		ServerBlastPAvailable:  true,
	}
	got := availableBlastProgramsFromCapability(serverOnly)
	if len(got) != 0 {
		t.Fatalf("server-only capability should not expose local programs: got %#v", got)
	}

	localOnly := lemna.BlastCapability{
		HasNucleotideFasta: true,
		HasProteinFasta:    true,
	}
	got = availableBlastProgramsFromCapability(localOnly)
	want := []string{"blastn", "blastx", "tblastn", "blastp"}
	if len(got) != len(want) {
		t.Fatalf("unexpected program count: got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected program at %d: got %q want %q", i, got[i], want[i])
		}
	}

	mixed := lemna.BlastCapability{
		HasServerNucleotideDB:  true,
		BlastNDBID:             12,
		ServerBlastNAvailable:  true,
		ServerTBlastNAvailable: true,
		HasProteinFasta:        true,
	}
	got = availableBlastProgramsFromCapability(mixed)
	want = []string{"blastx", "blastp"}
	if len(got) != len(want) {
		t.Fatalf("unexpected mixed program count: got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected mixed program at %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestChooseLemnaBlastExecutionRequiresLocalData(t *testing.T) {
	w := &BlastWizard{}
	selected := model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza 9509"}
	tests := []struct {
		name      string
		program   string
		serverCap lemna.BlastCapability
		localCap  lemna.BlastCapability
	}{
		{
			name:      "blastn",
			program:   "blastn",
			serverCap: lemna.BlastCapability{ServerBlastNAvailable: true},
			localCap:  lemna.BlastCapability{HasNucleotideFasta: true},
		},
		{
			name:      "blastx",
			program:   "blastx",
			serverCap: lemna.BlastCapability{ServerBlastXAvailable: true},
			localCap:  lemna.BlastCapability{HasProteinFasta: true},
		},
		{
			name:      "tblastn",
			program:   "tblastn",
			serverCap: lemna.BlastCapability{ServerTBlastNAvailable: true},
			localCap:  lemna.BlastCapability{HasNucleotideFasta: true},
		},
		{
			name:      "blastp",
			program:   "blastp",
			serverCap: lemna.BlastCapability{ServerBlastPAvailable: true},
			localCap:  lemna.BlastCapability{HasProteinFasta: true},
		},
	}
	merge := func(left, right lemna.BlastCapability) lemna.BlastCapability {
		return lemna.BlastCapability{
			ServerBlastNAvailable:  left.ServerBlastNAvailable || right.ServerBlastNAvailable,
			ServerBlastXAvailable:  left.ServerBlastXAvailable || right.ServerBlastXAvailable,
			ServerTBlastNAvailable: left.ServerTBlastNAvailable || right.ServerTBlastNAvailable,
			ServerBlastPAvailable:  left.ServerBlastPAvailable || right.ServerBlastPAvailable,
			HasNucleotideFasta:     left.HasNucleotideFasta || right.HasNucleotideFasta,
			HasProteinFasta:        left.HasProteinFasta || right.HasProteinFasta,
		}
	}
	for _, tt := range tests {
		if got, err := w.chooseLemnaBlastExecution(merge(tt.serverCap, tt.localCap), selected, tt.program); err != nil || got != "local" {
			t.Fatalf("%s both = %q/%v, want local/nil", tt.name, got, err)
		}
		if got, err := w.chooseLemnaBlastExecution(tt.serverCap, selected, tt.program); err == nil || got != "" {
			t.Fatalf("%s server-only = %q/%v, want empty/error", tt.name, got, err)
		}
		if got, err := w.chooseLemnaBlastExecution(tt.localCap, selected, tt.program); err != nil || got != "local" {
			t.Fatalf("%s local-only = %q/%v, want local/nil", tt.name, got, err)
		}
		if got, err := w.chooseLemnaBlastExecution(lemna.BlastCapability{}, selected, tt.program); err == nil || got != "" {
			t.Fatalf("%s unavailable = %q/%v, want empty/error", tt.name, got, err)
		}
	}
}

func TestUseSingleBlastRunReviewDependsOnOriginalQueryCount(t *testing.T) {
	oneRun := []blastQueryRun{{Index: 1, Results: model.BlastResult{Rows: []model.BlastResultRow{{Protein: "hit1"}}}}}
	if !useSingleBlastRunReview(1, oneRun) {
		t.Fatal("single original query with one run should use single-run review")
	}
	if useSingleBlastRunReview(2, oneRun) {
		t.Fatal("multi-query input with one surviving run must remain in multi-run review")
	}
	if useSingleBlastRunReview(19, oneRun) {
		t.Fatal("large multi-query input with one surviving run must remain in multi-run review")
	}
}

func TestHydrateKeywordSnapshotRestoresExportDefaults(t *testing.T) {
	w := NewBlastWizardWithTUIInfo(&bytes.Buffer{}, TUIInfo{})
	snapshot := sessionsnapshot.Snapshot{
		Context: sessionsnapshot.ContextV1{Mode: string(ModeKeyword)},
		Keyword: &sessionsnapshot.KeywordResultV1{
			SelectedSpecies: model.SpeciesCandidate{JBrowseName: "TAIR10", GenomeLabel: "TAIR10"},
			Groups: []model.KeywordSearchGroup{{
				SearchTerm: "PAL",
				Rows: []model.KeywordResultRow{{
					SourceDatabase: "tair",
					LabelName:      "PAL1",
					ProteinID:      "AT2G37040.1",
				}},
			}},
			Selected: []bool{true},
		},
		ExportSettings: &sessionsnapshot.ExportSettingsV2{
			BaseName:  "keyword",
			OutputDir: "out",
			Prompt: sessionsnapshot.PromptExportSettingsV2{
				BaseName:        "keyword",
				WriteExcel:      true,
				WriteSession:    true,
				FastaHeaderMode: model.FastaHeaderModePhgo,
				UsePhgoHeader:   true,
			},
		},
	}
	_, _, _, _, err := w.hydrateKeywordSnapshot(snapshot)
	if err != nil {
		t.Fatalf("hydrateKeywordSnapshot returned error: %v", err)
	}
	got := w.prompt.SnapshotExportSettings()
	if got.BaseName != "keyword" || !got.WriteExcel || !got.WriteSession {
		t.Fatalf("export defaults not restored: %#v", got)
	}
}

func TestHydrateBlastSnapshotRestoresExternalReferencesAndHandoff(t *testing.T) {
	w := NewBlastWizardWithTUIInfo(&bytes.Buffer{}, TUIInfo{})
	snapshot := sessionsnapshot.Snapshot{
		Context: sessionsnapshot.ContextV1{Mode: string(ModeBlast)},
		Blast: &sessionsnapshot.BlastResultV1{
			SelectedSpecies:  model.SpeciesCandidate{JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis"},
			OriginalRunCount: 2,
			Prepared: []sessionsnapshot.BlastQueryItemV1{{
				LabelName: "PAL1",
				Sequence:  "MPEPTIDE",
			}},
			Runs: []sessionsnapshot.BlastRunV1{{
				Index: 1,
				Item:  sessionsnapshot.BlastQueryItemV1{LabelName: "PAL1", Sequence: "MPEPTIDE"},
				Results: model.BlastResult{Rows: []model.BlastResultRow{{
					SourceDatabase: "phytozome",
					Protein:        "AT2G37040.1",
				}}},
			}},
		},
		ExternalReferences: &sessionsnapshot.ExternalReferenceSettingsV2{
			AutoLabelBlastHits: true,
			UseUniProt:         true,
			UseInterPro:        true,
			InterProSettings:   model.DefaultInterProConservedRegionSettings(),
		},
		Handoff: &sessionsnapshot.HandoffStateV2{
			PendingMode:          string(ModeBlast),
			TransferKind:         "blast-row-to-blast",
			TransferTargetDB:     "lemna",
			ReuseLastBlastInput:  true,
			ReuseLastBlastRows:   true,
			ReuseLastKeywordRows: true,
			LastBlastItems: []sessionsnapshot.BlastQueryItemV2{{
				LabelName: "PAL1",
				Sequence:  "MPEPTIDE",
			}},
		},
	}
	_, _, _, _, err := w.hydrateBlastSnapshot(snapshot)
	if err != nil {
		t.Fatalf("hydrateBlastSnapshot returned error: %v", err)
	}
	if !w.lastExternalRefs.UseUniProt || !w.lastExternalRefs.UseInterPro || !w.lastExternalRefs.AutoLabelBlastHits {
		t.Fatalf("external references not restored: %#v", w.lastExternalRefs)
	}
	if w.transferKind != "blast-row-to-blast" || w.transferTargetDatabase != "lemna" {
		t.Fatalf("handoff transfer state not restored: transferKind=%q target=%q", w.transferKind, w.transferTargetDatabase)
	}
	if !w.reuseLastBlastInput || !w.reuseLastBlastRows || !w.reuseLastKeywordRows {
		t.Fatalf("handoff reuse flags not restored")
	}
	if len(w.lastBlastItems) != 1 || w.lastBlastItems[0].LabelName != "PAL1" {
		t.Fatalf("last blast items not restored: %#v", w.lastBlastItems)
	}
	gotRefs := w.prompt.SnapshotExternalReferenceSettings()
	if !gotRefs.UseUniProt || !gotRefs.UseInterPro || !gotRefs.AutoLabelBlastHits {
		t.Fatalf("prompt external reference defaults not restored: %#v", gotRefs)
	}
}

func TestBlastSnapshotRoundTripRestoresRuntimeCaches(t *testing.T) {
	dir := t.TempDir()
	w := NewBlastWizardWithTUIInfo(&bytes.Buffer{}, TUIInfo{})
	w.source = keywordMapSource{name: "phytozome"}
	w.blastLabelLookupCache["label-key"] = blastAutoLabelResult{
		Label:         "PAL1",
		Aliases:       []string{"PAL1", "ATPAL1"},
		TaskTimestamp: "ts-1",
		ItemIndex:     2,
	}
	w.blastHitLabelLookupCache["hit-key"] = blastHitLabelIdentification{
		Label:     "C4H",
		LabelType: "phgo_alias",
		Aliases:   []string{"C4H", "CYP73A5"},
	}
	w.rowUniProtAccessionsCache["acc-key"] = []string{"Q43158"}
	w.rowUniProtAccessionsKnown["acc-key"] = true
	w.uniProtLookupCache["uni-key"] = uniProtLookupResult{
		entry: uniprot.Entry{Accession: "Q43158", ProteinName: "PAL"},
		ok:    true,
	}
	w.interProLookupCache["ip-key"] = interProLookupResult{
		entry: interpro.Entry{Accession: "Q43158", Accessions: "IPR000001"},
		ok:    true,
	}
	w.keywordBlastItemCache["item-key"] = blastQueryItem{LabelName: "PAL1", Sequence: "MPEPTIDE"}
	w.querySourceResolveCache["qs-key"] = model.QuerySequenceSource{LabelName: "PAL1", ProteinID: "AT2G37040.1"}
	w.keywordTermRowsCache["term-key"] = []model.KeywordResultRow{{LabelName: "PAL1", ProteinID: "AT2G37040.1"}}
	w.proteinSequenceCache["seq-key"] = model.ProteinSequenceData{Sequence: "MPEPTIDE", OriginalHeader: ">PAL1"}
	w.proteinSequenceMiss["miss-key"] = fmt.Errorf("no protein sequence for missing")
	w.speciesCandidatesCache["species-key"] = []model.SpeciesCandidate{{JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis"}}

	path := filepath.Join(dir, "runtime-cache")
	err := sessionsnapshot.WriteFile(path, sessionsnapshot.Snapshot{
		Context:      sessionsnapshot.ContextV1{CreatedAt: time.Now()},
		RuntimeCache: w.snapshotRuntimeCache(),
	})
	if err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	snapshot, err := sessionsnapshot.ReadFile(path + sessionsnapshot.FileExtension)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	restored := NewBlastWizardWithTUIInfo(&bytes.Buffer{}, TUIInfo{})
	restored.source = keywordMapSource{name: "phytozome"}
	restored.hydrateRuntimeCache(snapshot.RuntimeCache)

	if got := restored.blastLabelLookupCache["label-key"]; got.Label != "PAL1" || len(got.Aliases) != 2 {
		t.Fatalf("blast label cache not restored: %#v", got)
	}
	if got := restored.blastHitLabelLookupCache["hit-key"]; got.LabelType != "phgo_alias" || got.Label != "C4H" {
		t.Fatalf("blast hit label cache not restored: %#v", got)
	}
	if got := restored.rowUniProtAccessionsCache["acc-key"]; len(got) != 1 || got[0] != "Q43158" {
		t.Fatalf("row uniprot accession cache not restored: %#v", got)
	}
	if got := restored.uniProtLookupCache["uni-key"]; !got.ok || got.entry.Accession != "Q43158" {
		t.Fatalf("uniprot lookup cache not restored: %#v", got)
	}
	if got := restored.interProLookupCache["ip-key"]; !got.ok || got.entry.Accessions != "IPR000001" {
		t.Fatalf("interpro lookup cache not restored: %#v", got)
	}
	if got := restored.keywordBlastItemCache["item-key"]; got.LabelName != "PAL1" {
		t.Fatalf("keyword blast item cache not restored: %#v", got)
	}
	if got := restored.querySourceResolveCache["qs-key"]; got.ProteinID != "AT2G37040.1" {
		t.Fatalf("query source cache not restored: %#v", got)
	}
	if got := restored.keywordTermRowsCache["term-key"]; len(got) != 1 || got[0].LabelName != "PAL1" {
		t.Fatalf("keyword term rows cache not restored: %#v", got)
	}
	if got := restored.proteinSequenceCache["seq-key"]; got.Sequence != "MPEPTIDE" {
		t.Fatalf("protein sequence cache not restored: %#v", got)
	}
	if err := restored.proteinSequenceMiss["miss-key"]; err == nil || !strings.Contains(err.Error(), "no protein sequence") {
		t.Fatalf("protein sequence miss cache not restored: %v", err)
	}
	if got := restored.speciesCandidatesCache["species-key"]; len(got) != 1 || got[0].JBrowseName != "Athaliana_TAIR10" {
		t.Fatalf("species candidates cache not restored: %#v", got)
	}
}

func TestSnapshotArtifactBundleReturnsNilByDefault(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	outputDir, err := appfs.OutputDir()
	if err != nil {
		t.Fatalf("OutputDir: %v", err)
	}
	manifest, payloads, err := snapshotArtifactBundle(filepath.Join(outputDir, "session.pgo"), outputDir)
	if err != nil {
		t.Fatalf("snapshotArtifactBundle: %v", err)
	}
	if manifest != nil {
		t.Fatalf("snapshotArtifactBundle manifest = %#v, want nil", manifest)
	}
	if payloads != nil {
		t.Fatalf("snapshotArtifactBundle payloads = %#v, want nil", payloads)
	}
}

func TestCanvasItemFromBlastRowsUsesNumericTitleAndSequentialRowNumbers(t *testing.T) {
	item := canvasItemFromBlastRows("ignored", "PAL1", []model.BlastResultRow{
		{LabelName: "PAL1", Protein: "AT2G37040.1"},
		{LabelName: "PAL2", Protein: "AT3G53260.1"},
	}, nil, nil)
	if item.Title != "ignored" {
		t.Fatalf("canvas item title = %q, want preserved title", item.Title)
	}
	if len(item.Rows) != 2 {
		t.Fatalf("canvas row count = %d, want 2", len(item.Rows))
	}
	if item.Rows[0].RowNumber != 1 || item.Rows[1].RowNumber != 2 {
		t.Fatalf("canvas row numbers = %#v", item.Rows)
	}
}

func TestCanvasItemFromBlastRowsWithSourcePrependsNegativeSourceRow(t *testing.T) {
	item := canvasItemFromBlastRowsWithSource("PAL1", blastQueryItem{
		LabelName: "PAL1",
		Sequence:  "MPEPTIDE",
		QuerySource: &model.QuerySequenceSource{
			Sequence:            "MPEPTIDE",
			ProteinSequence:     "MPEPTIDE",
			SequenceKind:        model.SequenceProtein,
			LabelName:           "PAL1",
			GeneID:              "AT2G37040",
			SourceGenomeLabel:   "Athaliana_TAIR10",
			PreferredSequenceID: "AT2G37040.1",
		},
	}, []model.BlastResultRow{
		{LabelName: "C4H", BlastLabelName: "PAL1", BlastGeneID: "AT2G37040", Protein: "Sp7498_C4H_001"},
	}, nil, nil)
	if len(item.Rows) != 2 {
		t.Fatalf("canvas row count = %d, want source + hit", len(item.Rows))
	}
	if item.Rows[0].RowNumber != -1 || item.Rows[1].RowNumber != 1 {
		t.Fatalf("canvas source row numbers = %#v, want -1 then 1", item.Rows)
	}
	if item.Rows[0].FASTA == nil || item.Rows[0].FASTA.Sequence != "MPEPTIDE" || !item.Rows[0].FASTA.PhgoBlastQuerySource {
		t.Fatalf("source row not preserved for tree selection: %#v", item.Rows[0])
	}
}

func TestCanvasItemFromBlastRowsWithSourcePrependsAllFamilySourceRows(t *testing.T) {
	item := canvasItemFromBlastRowsWithSource("family", blastQueryItem{
		LabelName: "family",
		Sequence:  "MPEPTIDE",
		FamilySources: []*model.QuerySequenceSource{
			{
				Sequence:            "AAA",
				ProteinSequence:     "AAA",
				SequenceKind:        model.SequenceProtein,
				LabelName:           "PAL1",
				GeneID:              "AT2G37040",
				PreferredSequenceID: "AT2G37040.1",
			},
			{
				Sequence:            "BBB",
				ProteinSequence:     "BBB",
				SequenceKind:        model.SequenceProtein,
				LabelName:           "PAL2",
				GeneID:              "AT3G53260",
				PreferredSequenceID: "AT3G53260.1",
			},
		},
		QuerySource: &model.QuerySequenceSource{
			Sequence:            "AAA",
			ProteinSequence:     "AAA",
			SequenceKind:        model.SequenceProtein,
			LabelName:           "PAL1",
			GeneID:              "AT2G37040",
			PreferredSequenceID: "AT2G37040.1",
		},
	}, []model.BlastResultRow{
		{LabelName: "C4H", BlastLabelName: "PAL1", BlastGeneID: "AT2G37040", Protein: "Sp7498_C4H_001"},
	}, nil, nil)
	if len(item.Rows) != 3 {
		t.Fatalf("canvas row count = %d, want 2 sources + hit", len(item.Rows))
	}
	if item.Rows[0].RowNumber != -2 || item.Rows[1].RowNumber != -1 || item.Rows[2].RowNumber != 1 {
		t.Fatalf("canvas row numbers = %#v, want -2, -1, 1", item.Rows)
	}
	if item.Rows[0].FASTA == nil || item.Rows[0].FASTA.LabelName != "PAL1" {
		t.Fatalf("first source row missing or wrong: %#v", item.Rows[0])
	}
	if item.Rows[1].FASTA == nil || item.Rows[1].FASTA.LabelName != "PAL2" {
		t.Fatalf("second source row missing or wrong: %#v", item.Rows[1])
	}
}

func TestCanvasItemsFromBlastSnapshotSingleRunPrependsNegativeSourceRow(t *testing.T) {
	items := canvasItemsFromBlastSnapshot(&sessionsnapshot.BlastResultV1{
		Runs: []sessionsnapshot.BlastRunV1{{
			Index: 1,
			Item: sessionsnapshot.BlastQueryItemV1{
				LabelName: "PAL1",
				Sequence:  "MPEPTIDE",
				QuerySource: &model.QuerySequenceSource{
					LabelName:           "PAL1",
					GeneID:              "AT2G37040",
					Sequence:            "MPEPTIDE",
					ProteinSequence:     "MPEPTIDE",
					SequenceKind:        model.SequenceProtein,
					PreferredSequenceID: "AT2G37040.1",
				},
				FamilySources: []*model.QuerySequenceSource{
					{
						LabelName:           "PAL1",
						GeneID:              "AT2G37040",
						Sequence:            "MPEPTIDE",
						ProteinSequence:     "MPEPTIDE",
						SequenceKind:        model.SequenceProtein,
						PreferredSequenceID: "AT2G37040.1",
					},
					{
						LabelName:           "PAL2",
						GeneID:              "AT3G53260",
						Sequence:            "MPEPTIDER",
						ProteinSequence:     "MPEPTIDER",
						SequenceKind:        model.SequenceProtein,
						PreferredSequenceID: "AT3G53260.1",
					},
				},
			},
			Results: model.BlastResult{Rows: []model.BlastResultRow{
				{LabelName: "C4H", BlastLabelName: "PAL1", BlastGeneID: "AT2G37040", Protein: "Sp7498_C4H_001"},
			}},
		}},
		SelectedByRun: [][]bool{{true}},
	})
	if len(items) != 1 {
		t.Fatalf("canvas item count = %d, want 1", len(items))
	}
	if len(items[0].Rows) != 3 {
		t.Fatalf("canvas row count = %d, want 2 sources + hit", len(items[0].Rows))
	}
	if items[0].Rows[0].RowNumber != -2 || items[0].Rows[1].RowNumber != -1 || items[0].Rows[2].RowNumber != 1 {
		t.Fatalf("canvas row numbers = %#v, want -2, -1 then 1", items[0].Rows)
	}
	if items[0].Rows[0].FASTA == nil || !items[0].Rows[0].FASTA.PhgoBlastQuerySource {
		t.Fatalf("source row missing from snapshot import: %#v", items[0].Rows[0])
	}
}

func TestCanvasItemsFromBlastRunsPreserveRunTitlesAndSelectedRowsOnly(t *testing.T) {
	runs := []blastQueryRun{
		{
			Item: blastQueryItem{LabelName: "group A", RawInput: "raw A"},
			Results: model.BlastResult{Rows: []model.BlastResultRow{
				{LabelName: "A1", Protein: "P1"},
				{LabelName: "A2", Protein: "P2"},
			}},
		},
		{
			Item: blastQueryItem{LabelName: "group B", RawInput: "raw B"},
			Results: model.BlastResult{Rows: []model.BlastResultRow{
				{LabelName: "B1", Protein: "P3"},
			}},
		},
	}
	items := canvasItemsFromBlastRuns(runs, [][]bool{
		{true, false},
		{false},
	}, nil)
	if len(items) != 1 {
		t.Fatalf("canvas item count = %d, want 1", len(items))
	}
	if items[0].Title != "raw A[group A]" {
		t.Fatalf("canvas title = %q, want preserved sidebar title", items[0].Title)
	}
	if len(items[0].Rows) != 1 || items[0].Rows[0].RowNumber != 1 {
		t.Fatalf("canvas rows = %#v", items[0].Rows)
	}
}

func TestCanvasItemsFromBlastRunsUseQuerySourceSidebarTitle(t *testing.T) {
	runs := []blastQueryRun{{
		Item: blastQueryItem{
			LabelName: "PAL1",
			RawInput:  "raw input",
			QuerySource: &model.QuerySequenceSource{
				ProteinID: "AT2G37040.1",
			},
		},
		Results: model.BlastResult{Rows: []model.BlastResultRow{{LabelName: "hit", Protein: "P1"}}},
	}}
	items := canvasItemsFromBlastRuns(runs, [][]bool{{true}}, nil)
	if len(items) != 1 {
		t.Fatalf("canvas item count = %d, want 1", len(items))
	}
	if items[0].Title != "AT2G37040.1[PAL1]" {
		t.Fatalf("canvas title = %q, want original sidebar title", items[0].Title)
	}
}

func TestCanvasItemsFromKeywordSelectionKeepsSelectedRowsOnly(t *testing.T) {
	groups := []model.KeywordSearchGroup{{
		SearchTerm: "group A",
		LabelName:  "A",
		Rows: []model.KeywordResultRow{
			{LabelName: "A1", ProteinID: "P1"},
			{LabelName: "A2", ProteinID: "P2"},
		},
	}, {
		SearchTerm: "group B",
		LabelName:  "B",
		Rows: []model.KeywordResultRow{
			{LabelName: "B1", ProteinID: "P3"},
		},
	}}
	items := canvasItemsFromKeywordSelection(groups, []model.KeywordResultRow{
		groups[0].Rows[1],
	}, nil, nil)
	if len(items) != 1 {
		t.Fatalf("canvas item count = %d, want 1", len(items))
	}
	if items[0].Title != "1" {
		t.Fatalf("canvas title = %q, want numeric single-table title", items[0].Title)
	}
	if len(items[0].Rows) != 1 || items[0].Rows[0].KeywordRow == nil || items[0].Rows[0].KeywordRow.LabelName != "A2" {
		t.Fatalf("canvas rows = %#v", items[0].Rows)
	}
	if len(items[0].Selected) != 1 || items[0].Selected[0] {
		t.Fatalf("canvas selected = %#v, want default unselected row", items[0].Selected)
	}
}

func TestCanvasItemsFromKeywordSelectionCombinesSelectedRowsIntoOneCanvas(t *testing.T) {
	groups := []model.KeywordSearchGroup{{
		SearchTerm: "keyword A",
		LabelName:  "A",
		Rows: []model.KeywordResultRow{
			{LabelName: "A1", ProteinID: "P1"},
			{LabelName: "A2", ProteinID: "P2"},
		},
	}, {
		SearchTerm: "keyword B",
		LabelName:  "B",
		Rows: []model.KeywordResultRow{
			{LabelName: "B1", ProteinID: "P3"},
		},
	}}
	items := canvasItemsFromKeywordSelection(groups, []model.KeywordResultRow{
		groups[0].Rows[1],
		groups[1].Rows[0],
	}, [][]bool{
		{false, true},
		{true},
	}, nil)
	if len(items) != 1 {
		t.Fatalf("canvas item count = %d, want 1", len(items))
	}
	if items[0].Title != "1" {
		t.Fatalf("canvas title = %q, want single combined canvas title", items[0].Title)
	}
	if len(items[0].Rows) != 2 || items[0].Rows[0].KeywordRow == nil || items[0].Rows[0].KeywordRow.LabelName != "A2" || items[0].Rows[1].KeywordRow == nil || items[0].Rows[1].KeywordRow.LabelName != "B1" {
		t.Fatalf("canvas rows = %#v", items[0].Rows)
	}
	if got := items[0].Subtitle; got != "0/2 lines" {
		t.Fatalf("canvas subtitle = %q, want single-line unselected summary", got)
	}
}

func TestMarkKeywordCanvasSequenceAvailabilityDisablesMissingRows(t *testing.T) {
	items := canvasItemsFromKeywordSelection([]model.KeywordSearchGroup{{
		SearchTerm: "keyword A",
		Rows: []model.KeywordResultRow{
			{LabelName: "A1", SequenceID: "ok"},
			{LabelName: "A2", SequenceID: "missing"},
		},
	}}, []model.KeywordResultRow{
		{LabelName: "A1", SequenceID: "ok"},
		{LabelName: "A2", SequenceID: "missing"},
	}, [][]bool{{true, true}}, nil)
	markKeywordCanvasSequenceAvailability(items, map[string]sequenceFetchResult{
		"ok":      {data: model.ProteinSequenceData{Sequence: "MPEPTIDE"}},
		"missing": {err: fmt.Errorf("empty TAIR FASTA URL")},
	})
	if len(items) != 1 || len(items[0].Rows) != 2 {
		t.Fatalf("canvas items = %#v", items)
	}
	if items[0].Rows[0].SequenceReady == nil || !*items[0].Rows[0].SequenceReady {
		t.Fatalf("available row should be marked selectable: %#v", items[0].Rows[0])
	}
	if items[0].Rows[1].SequenceReady == nil || *items[0].Rows[1].SequenceReady {
		t.Fatalf("missing row should be marked non-selectable: %#v", items[0].Rows[1])
	}
}

func TestMissingProteinSequenceErrorIncludesTAIRFastaURLFailures(t *testing.T) {
	for _, err := range []error{
		fmt.Errorf("empty TAIR FASTA URL"),
		fmt.Errorf("missing protein FASTA URL"),
		fmt.Errorf("no external protein sequence source matched AT1G01010"),
		fmt.Errorf("no TAIR protein sequence matched AT1G01010"),
	} {
		if !isMissingProteinSequenceError(err) {
			t.Fatalf("error should be classified as missing sequence: %v", err)
		}
	}
}

func TestCanvasRowsFromFastaInputKeepsPhgoHeadAndSequence(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	rows, err := w.canvasRowsFromFastaInput(">phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\7\nMPEPTIDE\n", false)
	if err != nil {
		t.Fatalf("canvasRowsFromFastaInput returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].FASTA == nil {
		t.Fatalf("canvas rows = %#v", rows)
	}
	source := rows[0].FASTA
	if source.Annotation != "phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\7" || source.Sequence != "MPEPTIDE" {
		t.Fatalf("phgo FASTA head/sequence not preserved: %#v", source)
	}
	if source.OrganismShort != "Sp7498" || source.LabelName != "C4H" || source.GeneID != "Sp7498_C4H_001" || source.BlastSourceLabelName != "PAL1" || source.BlastSourceGeneID != "AT2G37040" || source.PhgoRowNumber != 7 {
		t.Fatalf("phgo FASTA metadata not extracted: %#v", source)
	}
}

func TestCanvasRowsFromFastaInputLocksCanvasPhgoDisplayName(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	rows, err := w.canvasRowsFromFastaInput(">phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\-2/My canvas\\Display PAL\nMPEPTIDE\n", false)
	if err != nil {
		t.Fatalf("canvasRowsFromFastaInput returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].FASTA == nil {
		t.Fatalf("canvas rows = %#v", rows)
	}
	if rows[0].DisplayName != "Display PAL" || !rows[0].DisplayNameLocked {
		t.Fatalf("canvas phgo display name was not locked: %#v", rows[0])
	}
	if rows[0].FASTA.PhgoCanvasRawRow != -2 || !rows[0].FASTA.PhgoCanvasHasRawRow || rows[0].FASTA.PhgoCanvasTitle != "My canvas" {
		t.Fatalf("canvas phgo metadata not extracted: %#v", rows[0].FASTA)
	}
}

func TestApplyCanvasDisplayNameSourceUsesCurrentColumnForUnlockedRows(t *testing.T) {
	item := model.CanvasItem{
		Title: "Canvas 7",
		Rows: []model.CanvasRow{
			{
				Kind: model.CanvasKindFasta,
				FASTA: &model.QuerySequenceSource{
					Annotation:           ">seq1",
					Sequence:             "MPEPTIDE",
					LabelName:            "PAL1",
					GeneID:               "AT2G37040",
					BlastSourceLabelName: "SRC1",
				},
			},
			{
				Kind:              model.CanvasKindFasta,
				DisplayName:       "Locked display",
				DisplayNameLocked: true,
				FASTA: &model.QuerySequenceSource{
					Annotation: ">seq2",
					Sequence:   "MPEPTIDE",
					LabelName:  "PAL2",
					GeneID:     "AT3G53260",
				},
			},
		},
	}

	applyCanvasDisplayNameSource(&item, "gene_id")
	if got := item.Rows[0].DisplayName; got != "AT2G37040" {
		t.Fatalf("unlocked display name = %q, want gene_id value", got)
	}
	if got := item.Rows[1].DisplayName; got != "Locked display" {
		t.Fatalf("locked display name = %q, want preserved lock", got)
	}

	applyCanvasDisplayNameSource(&item, phylo.PHgoDisplayNameSource)
	if got := item.Rows[0].DisplayName; got != "~-AT2G37040 (PAL1)" {
		t.Fatalf("PHgo display name = %q", got)
	}
	if got := item.Rows[1].DisplayName; got != "Locked display" {
		t.Fatalf("locked display name after PHgo apply = %q, want preserved lock", got)
	}
}

func TestApplyCanvasDisplayNameSourceToItemsUpdatesEveryImportedCanvas(t *testing.T) {
	items := []model.CanvasItem{
		{
			Title: "A",
			Rows: []model.CanvasRow{{
				Kind: model.CanvasKindFasta,
				FASTA: &model.QuerySequenceSource{
					Annotation: ">alpha",
					Sequence:   "MPEPTIDE",
					LabelName:  "PALA",
				},
			}},
		},
		{
			Title: "B",
			Rows: []model.CanvasRow{{
				Kind: model.CanvasKindFasta,
				FASTA: &model.QuerySequenceSource{
					Annotation: ">beta",
					Sequence:   "MPEPTIDE",
					LabelName:  "PALB",
				},
			}},
		},
	}

	applyCanvasDisplayNameSourceToItems(items, "label_name")

	if got := items[0].Rows[0].DisplayName; got != "PALA" {
		t.Fatalf("first canvas display name = %q, want PALA", got)
	}
	if got := items[1].Rows[0].DisplayName; got != "PALB" {
		t.Fatalf("second canvas display name = %q, want PALB", got)
	}
}

func TestCanvasRowsFromFastaInputUsesNegativeNumbersForPhgoBlastSources(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	rows, err := w.canvasRowsFromFastaInput(strings.Join([]string{
		">phgo://Athaliana_TAIR10/PAL1/AT2G37040.1\\h",
		"MPEPTIDE",
		">phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\1",
		"GGGTT",
	}, "\n"), true)
	if err != nil {
		t.Fatalf("canvasRowsFromFastaInput returned error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("canvas rows = %#v, want source and hit", rows)
	}
	if rows[0].RowNumber != -1 || rows[1].RowNumber != 1 {
		t.Fatalf("canvas row numbers = %#v, want -1 then 1", rows)
	}
	if rows[0].FASTA == nil || !rows[0].FASTA.PhgoBlastQuerySource {
		t.Fatalf("source row marker missing: %#v", rows[0])
	}
}

func TestCanvasRowsFromFastaInputIgnoresPhgoNoteEntries(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	rows, err := w.canvasRowsFromFastaInput(strings.Join([]string{
		">phgo://note",
		"xxxxx",
		"xxxxxx",
		">query1",
		"MPEPTIDE",
	}, "\n"), false)
	if err != nil {
		t.Fatalf("canvasRowsFromFastaInput returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].FASTA == nil {
		t.Fatalf("canvas rows = %#v, want one non-note row", rows)
	}
	if rows[0].FASTA.Annotation != "query1" || rows[0].FASTA.Sequence != "MPEPTIDE" {
		t.Fatalf("unexpected surviving FASTA row: %#v", rows[0].FASTA)
	}
}

func TestCanvasItemsFromInputPreservesNegativeBlastSourceNumbersForPhgoFasta(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	items, err := w.canvasItemsFromInput(context.Background(), strings.Join([]string{
		">phgo://Athaliana_TAIR10/PAL1/AT2G37040.1\\h",
		"MPEPTIDE",
		">phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\1",
		"GGGTT",
	}, "\n"), 1, "")
	if err != nil {
		t.Fatalf("canvasItemsFromInput returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("canvas item count = %d, want 1", len(items))
	}
	if len(items[0].Rows) != 2 {
		t.Fatalf("canvas row count = %d, want 2", len(items[0].Rows))
	}
	if items[0].Rows[0].RowNumber != -1 || items[0].Rows[1].RowNumber != 1 {
		t.Fatalf("canvas row numbers = %#v, want -1 then 1", items[0].Rows)
	}
	if items[0].Rows[0].FASTA == nil || !items[0].Rows[0].FASTA.PhgoBlastQuerySource {
		t.Fatalf("source row marker missing after add canvas: %#v", items[0].Rows[0])
	}
}

func TestCanvasItemsFromInputUsesNegativeSequenceForMultipleBlastSources(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	items, err := w.canvasItemsFromInput(context.Background(), strings.Join([]string{
		">phgo://Arabidopsis_thaliana_TAIR10/4CL1/AT1G51680.1\\h",
		"MPEPTIDE",
		">phgo://Arabidopsis_thaliana_TAIR10/4CL2/AT3G21240.1\\h",
		"MPEPTIDER",
		">phgo://Spirodela_polyrhiza_9509_REF-OXFORD-3.0/024540/Sp9509d012g007020_T001\\4CL1/AT1G51680.1\\1",
		"AAAA",
		">phgo://Spirodela_polyrhiza_9509_REF-OXFORD-3.0/P41636/Sp9509d011g001470_T001\\4CL2/AT3G21240.1\\2",
		"BBBB",
	}, "\n"), 1, "")
	if err != nil {
		t.Fatalf("canvasItemsFromInput returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("canvas item count = %d, want 1", len(items))
	}
	got := []int{
		items[0].Rows[0].RowNumber,
		items[0].Rows[1].RowNumber,
		items[0].Rows[2].RowNumber,
		items[0].Rows[3].RowNumber,
	}
	want := []int{-2, -1, 1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("canvas row numbers = %#v, want %#v", got, want)
	}
}

func TestCanvasItemsFromInputUsesFastaFilenameTitle(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	dir := t.TempDir()
	path := filepath.Join(dir, "Transcriptional Factors for Lignin_Cell Wall Biosynthesis.fasta")
	if err := os.WriteFile(path, []byte(">query1\nMPEPTIDE\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	items, err := w.canvasItemsFromInput(context.Background(), ">query1\nMPEPTIDE\n", 3, path)
	if err != nil {
		t.Fatalf("canvasItemsFromInput returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("canvas item count = %d, want 1", len(items))
	}
	if items[0].Title != "Transcription~50" {
		t.Fatalf("canvas title = %q, want shortened import filename", items[0].Title)
	}
}

func TestCanvasItemsFromInputUsesNumericTitleWhenSourcePathBlank(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	items, err := w.canvasItemsFromInput(context.Background(), ">query1\nMPEPTIDE\n", 3, "")
	if err != nil {
		t.Fatalf("canvasItemsFromInput returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("canvas item count = %d, want 1", len(items))
	}
	if items[0].Title != "3" {
		t.Fatalf("canvas title = %q, want numeric title 3", items[0].Title)
	}
}

func TestCanvasItemsFromInputImportsSessionSnapshot(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	path := filepath.Join(t.TempDir(), "canvas-session.pgo")
	if err := os.WriteFile(path, []byte("PK\x03\x04snapshot"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	items, err := w.canvasItemsFromInput(context.Background(), path, 3, path)
	if err == nil {
		t.Fatalf("canvasItemsFromInput returned items %#v, want snapshot parse error", items)
	}
	if !strings.Contains(err.Error(), "open canvas snapshot") && !strings.Contains(err.Error(), "session snapshot") {
		t.Fatalf("error = %q, want snapshot import guidance", err.Error())
	}
}

func TestCanvasItemsFromSnapshotInputUsesFilenameWhenSnapshotHasNoTitle(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	dir := t.TempDir()
	path := filepath.Join(dir, "keyword_session_without_title.pgo")
	if err := sessionsnapshot.WriteFile(path, sessionsnapshot.Snapshot{
		Context: sessionsnapshot.ContextV1{CreatedAt: time.Now(), Mode: "keyword"},
		Keyword: &sessionsnapshot.KeywordResultV1{
			SelectedSpecies: model.SpeciesCandidate{JBrowseName: "TAIR12", GenomeLabel: "TAIR12"},
			Groups: []model.KeywordSearchGroup{{
				SearchTerm: "PAL",
				Rows: []model.KeywordResultRow{{
					SourceDatabase: "tair",
					LabelName:      "PAL1",
					ProteinID:      "AT2G37040.1",
					SequenceID:     "AT2G37040.1",
				}},
			}},
			Selected: []bool{true},
		},
	}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	items, err := w.canvasItemsFromSnapshotInput(path, path)
	if err != nil {
		t.Fatalf("canvasItemsFromSnapshotInput returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("canvas item count = %d, want 1", len(items))
	}
	if items[0].Title != "keyword_sessi~20" {
		t.Fatalf("canvas title = %q, want shortened filename fallback", items[0].Title)
	}
}

func TestCanvasItemsFromCanvasSnapshotInputPreservesSidebarTitles(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	dir := t.TempDir()
	path := filepath.Join(dir, "canvas_sidebar_titles.pgo")
	if err := sessionsnapshot.WriteFile(path, sessionsnapshot.Snapshot{
		Context: sessionsnapshot.ContextV1{CreatedAt: time.Now(), Mode: "canvas"},
		Canvas: &sessionsnapshot.CanvasResultV1{
			Items: []sessionsnapshot.CanvasItemV1{{
				Title:    "A-thaliana",
				Selected: []bool{true},
				Rows: []sessionsnapshot.CanvasRowV1{{
					Kind:  model.CanvasKindFasta,
					FASTA: &model.QuerySequenceSource{LabelName: "PAL1", Sequence: "MPEPTIDE", SequenceKind: model.SequenceProtein},
				}},
			}, {
				Title:    "o-sativa",
				Selected: []bool{true},
				Rows: []sessionsnapshot.CanvasRowV1{{
					Kind:  model.CanvasKindFasta,
					FASTA: &model.QuerySequenceSource{LabelName: "PAL2", Sequence: "MSECOND", SequenceKind: model.SequenceProtein},
				}},
			}},
		},
	}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	items, err := w.canvasItemsFromSnapshotInput(path, path)
	if err != nil {
		t.Fatalf("canvasItemsFromSnapshotInput returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("canvas item count = %d, want 2", len(items))
	}
	if items[0].Title != "A-thaliana" || items[1].Title != "o-sativa" {
		t.Fatalf("canvas titles = %q/%q, want restored sidebar titles", items[0].Title, items[1].Title)
	}
}

func TestCanvasItemsFromCanvasSnapshotInputUsesFilenameForGeneratedTitles(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	dir := t.TempDir()
	path := filepath.Join(dir, "numeric_canvas_snapshot.pgo")
	if err := sessionsnapshot.WriteFile(path, sessionsnapshot.Snapshot{
		Context: sessionsnapshot.ContextV1{CreatedAt: time.Now(), Mode: "canvas"},
		Canvas: &sessionsnapshot.CanvasResultV1{
			Items: []sessionsnapshot.CanvasItemV1{{
				Title:    "1",
				Selected: []bool{true},
				Rows: []sessionsnapshot.CanvasRowV1{{
					Kind:  model.CanvasKindFasta,
					FASTA: &model.QuerySequenceSource{LabelName: "PAL1", Sequence: "MPEPTIDE", SequenceKind: model.SequenceProtein},
				}},
			}},
		},
	}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	items, err := w.canvasItemsFromSnapshotInput(path, path)
	if err != nil {
		t.Fatalf("canvasItemsFromSnapshotInput returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("canvas item count = %d, want 1", len(items))
	}
	if items[0].Title != "numeric_canva~14" {
		t.Fatalf("canvas title = %q, want shortened filename for generated title", items[0].Title)
	}
}

func TestCanvasItemsFromKeywordSnapshotInputAlwaysUseFilenameTitle(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	dir := t.TempDir()
	path := filepath.Join(dir, "keyword_named_snapshot.pgo")
	if err := sessionsnapshot.WriteFile(path, sessionsnapshot.Snapshot{
		Context: sessionsnapshot.ContextV1{CreatedAt: time.Now(), Mode: "keyword"},
		Keyword: &sessionsnapshot.KeywordResultV1{
			SelectedSpecies: model.SpeciesCandidate{JBrowseName: "TAIR12", GenomeLabel: "TAIR12"},
			Groups: []model.KeywordSearchGroup{{
				SearchTerm: "PAL",
				LabelName:  "PAL group",
				Rows: []model.KeywordResultRow{{
					SourceDatabase: "tair",
					LabelName:      "PAL1",
					ProteinID:      "AT2G37040.1",
					SequenceID:     "AT2G37040.1",
				}},
			}},
			Selected: []bool{true},
		},
	}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	items, err := w.canvasItemsFromSnapshotInput(path, path)
	if err != nil {
		t.Fatalf("canvasItemsFromSnapshotInput returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("canvas item count = %d, want 1", len(items))
	}
	if items[0].Title != "keyword_named~13" {
		t.Fatalf("canvas title = %q, want filename title for keyword snapshot", items[0].Title)
	}
}

func TestCanvasItemsFromFamilyBlastSnapshotInputUseFilenameTitle(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	dir := t.TempDir()
	path := filepath.Join(dir, "family_blast_snapshot.pgo")
	if err := sessionsnapshot.WriteFile(path, sessionsnapshot.Snapshot{
		Context: sessionsnapshot.ContextV1{CreatedAt: time.Now(), Mode: "blast"},
		Blast: &sessionsnapshot.BlastResultV1{
			Prepared: []sessionsnapshot.BlastQueryItemV1{{
				LabelName:  "PAL1",
				Sequence:   "MPEPTIDE",
				FamilyName: "PAL",
				FamilySources: []*model.QuerySequenceSource{{
					LabelName:           "PAL1",
					Sequence:            "MPEPTIDE",
					ProteinSequence:     "MPEPTIDE",
					SequenceKind:        model.SequenceProtein,
					PreferredSequenceID: "AT2G37040.1",
				}},
			}},
			Runs: []sessionsnapshot.BlastRunV1{{
				Index: 1,
				Item: sessionsnapshot.BlastQueryItemV1{
					LabelName:  "PAL1",
					Sequence:   "MPEPTIDE",
					FamilyName: "PAL",
					FamilySources: []*model.QuerySequenceSource{{
						LabelName:           "PAL1",
						Sequence:            "MPEPTIDE",
						ProteinSequence:     "MPEPTIDE",
						SequenceKind:        model.SequenceProtein,
						PreferredSequenceID: "AT2G37040.1",
					}},
				},
				Results: model.BlastResult{
					Rows: []model.BlastResultRow{{
						SourceDatabase: "phytozome",
						LabelName:      "PALHIT",
						Protein:        "AT3G53260.1",
						SequenceID:     "AT3G53260.1",
					}},
				},
			}},
			SelectedByRun: [][]bool{{true}},
		},
	}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	items, err := w.canvasItemsFromSnapshotInput(path, path)
	if err != nil {
		t.Fatalf("canvasItemsFromSnapshotInput returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("canvas item count = %d, want 1", len(items))
	}
	if items[0].Title != "family_blast_~12" {
		t.Fatalf("canvas title = %q, want filename title for family snapshot", items[0].Title)
	}
}

func TestCanvasItemsFromMultiRunFamilyBlastSnapshotInputUseFilenameTitle(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	dir := t.TempDir()
	path := filepath.Join(dir, "family_multi_run_snapshot.pgo")
	familyItem := func(label, sequence string) sessionsnapshot.BlastQueryItemV1 {
		return sessionsnapshot.BlastQueryItemV1{
			LabelName:  label,
			Sequence:   sequence,
			FamilyName: "PAL",
			FamilySources: []*model.QuerySequenceSource{{
				LabelName:           label,
				Sequence:            sequence,
				ProteinSequence:     sequence,
				SequenceKind:        model.SequenceProtein,
				PreferredSequenceID: label + ".1",
			}},
		}
	}
	if err := sessionsnapshot.WriteFile(path, sessionsnapshot.Snapshot{
		Context: sessionsnapshot.ContextV1{CreatedAt: time.Now(), Mode: string(ModeFamily)},
		Blast: &sessionsnapshot.BlastResultV1{
			Runs: []sessionsnapshot.BlastRunV1{{
				Index: 1,
				Item:  familyItem("PAL1", "MPEPTIDE"),
				Results: model.BlastResult{Rows: []model.BlastResultRow{{
					SourceDatabase: "phytozome",
					LabelName:      "PALHIT1",
					Protein:        "AT3G53260.1",
					SequenceID:     "AT3G53260.1",
					FamilyName:     "PAL",
				}}},
			}, {
				Index: 2,
				Item:  familyItem("PAL2", "MPEPTIDER"),
				Results: model.BlastResult{Rows: []model.BlastResultRow{{
					SourceDatabase: "phytozome",
					LabelName:      "PALHIT2",
					Protein:        "AT5G04230.1",
					SequenceID:     "AT5G04230.1",
					FamilyName:     "PAL",
				}}},
			}},
			SelectedByRun: [][]bool{{true}, {true}},
		},
	}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	items, err := w.canvasItemsFromSnapshotInput(path, path)
	if err != nil {
		t.Fatalf("canvasItemsFromSnapshotInput returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("canvas item count = %d, want separate no-sidebar snapshot items", len(items))
	}
	for i, item := range items {
		if item.Title != "family_multi_~16" {
			t.Fatalf("canvas item %d title = %q, want filename title for no-sidebar family snapshot", i, item.Title)
		}
		if len(item.Rows) != 2 || item.Rows[0].RowNumber != -1 || item.Rows[1].RowNumber != 1 {
			t.Fatalf("canvas item %d rows = %#v, want source row then hit row", i, item.Rows)
		}
	}
}

func TestCanvasItemsFromMultiFileBlastFamilySnapshotInputPreservesSidebarTitle(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	dir := t.TempDir()
	path := filepath.Join(dir, "family_multi_file_blast_snapshot.pgo")
	familyItem := func(mainTitle, member, sequence string) sessionsnapshot.BlastQueryItemV1 {
		return sessionsnapshot.BlastQueryItemV1{
			LabelName:  member,
			Sequence:   sequence,
			FamilyName: "PAL",
			QuerySource: &model.QuerySequenceSource{
				GeneID:          mainTitle,
				Sequence:        sequence,
				ProteinSequence: sequence,
				SequenceKind:    model.SequenceProtein,
				LabelName:       member,
			},
			FamilySources: []*model.QuerySequenceSource{{
				LabelName:           member,
				GeneID:              mainTitle,
				Sequence:            sequence,
				ProteinSequence:     sequence,
				SequenceKind:        model.SequenceProtein,
				PreferredSequenceID: mainTitle,
			}},
		}
	}
	if err := sessionsnapshot.WriteFile(path, sessionsnapshot.Snapshot{
		Context: sessionsnapshot.ContextV1{CreatedAt: time.Now(), Mode: string(ModeBlast)},
		Blast: &sessionsnapshot.BlastResultV1{
			OriginalRunCount: 2,
			Runs: []sessionsnapshot.BlastRunV1{{
				Index: 1,
				Item:  familyItem("AT2G37040.1", "PAL1", "MPEPTIDE"),
				Results: model.BlastResult{Rows: []model.BlastResultRow{{
					SourceDatabase: "phytozome",
					LabelName:      "PALHIT1",
					Protein:        "AT3G53260.1",
					SequenceID:     "AT3G53260.1",
					FamilyName:     "PAL",
				}}},
			}, {
				Index: 2,
				Item:  familyItem("AT1G51680.1", "4CL1", "MPEPTIDER"),
				Results: model.BlastResult{Rows: []model.BlastResultRow{{
					SourceDatabase: "phytozome",
					LabelName:      "PALHIT2",
					Protein:        "AT5G04230.1",
					SequenceID:     "AT5G04230.1",
					FamilyName:     "PAL",
				}}},
			}},
			SelectedByRun: [][]bool{{true}, {true}},
		},
	}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	items, err := w.canvasItemsFromSnapshotInput(path, path)
	if err != nil {
		t.Fatalf("canvasItemsFromSnapshotInput returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("canvas item count = %d, want 2", len(items))
	}
	if items[0].Title != "AT2G37040.1[PAL]" || items[1].Title != "AT1G51680.1[PAL]" {
		t.Fatalf("canvas titles = %q, %q; want original sidebar main title plus family subtitle", items[0].Title, items[1].Title)
	}
}

func TestCanvasItemsFromMergedMultiFileBlastSnapshotInputPreservesSidebarTitle(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	dir := t.TempDir()
	path := filepath.Join(dir, "merged_multi_file_blast_snapshot.pgo")
	if err := sessionsnapshot.WriteFile(path, sessionsnapshot.Snapshot{
		Context: sessionsnapshot.ContextV1{CreatedAt: time.Now(), Mode: string(ModeBlast)},
		Blast: &sessionsnapshot.BlastResultV1{
			OriginalRunCount: 2,
			Runs: []sessionsnapshot.BlastRunV1{{
				Index: 1,
				Item: sessionsnapshot.BlastQueryItemV1{
					LabelName:  "PAL1",
					Sequence:   "MPEPTIDE",
					FamilyName: "PAL",
					QuerySource: &model.QuerySequenceSource{
						GeneID:          "AT2G37040.1",
						Sequence:        "MPEPTIDE",
						ProteinSequence: "MPEPTIDE",
						SequenceKind:    model.SequenceProtein,
						LabelName:       "PAL1",
					},
					FamilySources: []*model.QuerySequenceSource{{
						LabelName:           "PAL1",
						GeneID:              "AT2G37040.1",
						Sequence:            "MPEPTIDE",
						ProteinSequence:     "MPEPTIDE",
						SequenceKind:        model.SequenceProtein,
						PreferredSequenceID: "AT2G37040.1",
					}},
				},
				Results: model.BlastResult{Rows: []model.BlastResultRow{{
					SourceDatabase: "phytozome",
					LabelName:      "PALHIT1",
					Protein:        "AT3G53260.1",
					SequenceID:     "AT3G53260.1",
					FamilyName:     "PAL",
				}}},
			}},
			SelectedByRun: [][]bool{{true}},
		},
	}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	items, err := w.canvasItemsFromSnapshotInput(path, path)
	if err != nil {
		t.Fatalf("canvasItemsFromSnapshotInput returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("canvas item count = %d, want 1", len(items))
	}
	if items[0].Title != "AT2G37040.1[PAL]" {
		t.Fatalf("canvas title = %q; want original multi-file sidebar title after merge", items[0].Title)
	}
}

func TestCanvasRowsFromFastaInputRejectsSessionSnapshot(t *testing.T) {
	w := NewBlastWizard(io.Discard)
	path := filepath.Join(t.TempDir(), "canvas-session.pgo")
	if err := os.WriteFile(path, []byte("PK\x03\x04snapshot"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	rows, err := w.canvasRowsFromFastaInput(path, false)
	if err == nil {
		t.Fatalf("canvasRowsFromFastaInput returned rows %#v, want .pgo rejection", rows)
	}
	if !strings.Contains(err.Error(), "Open session") {
		t.Fatalf("error = %q, want Open session guidance", err.Error())
	}
}

func TestCanvasImportedFileShortNameShortensOnlyLongImportNames(t *testing.T) {
	got := canvasImportedFileShortName("Monolignol Polymerization.fasta")
	if got != "Monolignol Po~18" {
		t.Fatalf("canvasImportedFileShortName = %q, want Monolignol Po~18", got)
	}
	short := canvasImportedFileShortName("queries.fasta")
	if short != "queries.fasta" {
		t.Fatalf("short filename = %q, want unchanged filename", short)
	}
}

func TestSelectedCanvasRowsInOrderKeepsCanvasAndRowOrder(t *testing.T) {
	rows := selectedCanvasRowsInOrder([]model.CanvasItem{
		{
			Title: "2",
			Rows: []model.CanvasRow{
				{RowNumber: 2, Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Annotation: "h2", Sequence: "BBB"}},
				{RowNumber: 1, Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Annotation: "h1", Sequence: "AAA"}},
			},
			Selected: []bool{true, false},
		},
		{
			Title: "3",
			Rows: []model.CanvasRow{
				{RowNumber: 3, Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Annotation: "h3", Sequence: "CCC"}},
			},
			Selected: []bool{true},
		},
	})
	if len(rows) != 2 {
		t.Fatalf("selected canvas rows = %d, want 2", len(rows))
	}
	if rows[0].ItemTitle != "2" || rows[0].Row.RowNumber != 2 || rows[1].ItemTitle != "3" || rows[1].Row.RowNumber != 3 {
		t.Fatalf("selected canvas rows order = %#v", rows)
	}
}

func TestSelectedCanvasRowsInVisibleOrderUsesCanvasSortState(t *testing.T) {
	rows := selectedCanvasRowsInVisibleOrder([]model.CanvasItem{{
		Title: "canvas 1",
		Rows: []model.CanvasRow{
			{RowNumber: 7, Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Annotation: "beta", Sequence: "BBB"}},
			{RowNumber: 3, Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Annotation: "alpha", Sequence: "AAA"}},
		},
		Selected: []bool{true, true},
	}}, tui.BlastRunSelectionState{
		Valid: true,
		Sort:  tui.TableSort{Column: 1, Direction: tui.SortAscending},
	}, false)
	if len(rows) != 2 {
		t.Fatalf("selected visible rows = %d, want 2", len(rows))
	}
	if rows[0].Row.RowNumber != 3 || rows[1].Row.RowNumber != 7 {
		t.Fatalf("selected visible row order = %#v, want row numbers 3 then 7", rows)
	}
}

func TestSelectedCanvasRowsForExportFiltersRowsWithoutExportSequence(t *testing.T) {
	rows := selectedCanvasRowsInOrderForExport([]model.CanvasItem{{
		Title:    "canvas 1",
		Selected: []bool{true, true, true},
		Rows: []model.CanvasRow{
			{RowNumber: 1, Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Annotation: "empty"}},
			{RowNumber: 2, Kind: model.CanvasKindFasta, FASTA: &model.QuerySequenceSource{Annotation: "protein", ProteinSequence: "MPEPTIDE"}},
			{RowNumber: 3, Kind: model.CanvasKindKeyword, KeywordRow: &model.KeywordResultRow{LabelName: "missing"}},
		},
	}})
	if len(rows) != 1 {
		t.Fatalf("export rows = %#v, want only sequence-ready row", rows)
	}
	if rows[0].Row.RowNumber != 2 {
		t.Fatalf("export row number = %d, want 2", rows[0].Row.RowNumber)
	}
}

func TestApplyCanvasHeaderModePhgoAlwaysBuildsCanvasHeader(t *testing.T) {
	selected := []canvasSelectedRow{{
		ItemTitle: "1",
		Row: model.CanvasRow{
			RowNumber: 4,
			Kind:      model.CanvasKindFasta,
			FASTA: &model.QuerySequenceSource{
				Annotation: "plain original header",
				Sequence:   "MPEPTIDE",
			},
		},
	}}
	records := []model.ProteinSequenceRecord{{
		Header:         ">temporary",
		OriginalHeader: ">plain original header",
		Sequence:       "MPEPTIDE",
	}}
	got := applyCanvasHeaderMode(records, selected, model.FastaHeaderModePhgo)
	want := ">phgo://~/~/plain\\~/~\\4/1\\plain"
	if len(got) != 1 || got[0].Header != want {
		t.Fatalf("phgo canvas header = %#v, want %q", got, want)
	}
}

func TestApplyCanvasHeaderModePhgoConvertsStoredPhgoHeaderToCanvasHeader(t *testing.T) {
	selected := []canvasSelectedRow{{
		ItemTitle: "1",
		Row: model.CanvasRow{
			RowNumber: 7,
			Kind:      model.CanvasKindFasta,
			FASTA: &model.QuerySequenceSource{
				Annotation:           "phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\7",
				Sequence:             "MPEPTIDE",
				OrganismShort:        "Sp7498",
				LabelName:            "C4H",
				GeneID:               "Sp7498_C4H_001",
				BlastSourceLabelName: "PAL1",
				BlastSourceGeneID:    "AT2G37040",
			},
		},
	}}
	records := []model.ProteinSequenceRecord{{
		Header:         ">temporary",
		OriginalHeader: ">plain original header",
		Sequence:       "MPEPTIDE",
	}}
	got := applyCanvasHeaderMode(records, selected, model.FastaHeaderModePhgo)
	if len(got) != 1 || got[0].Header != ">phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\7/1\\C4H" {
		t.Fatalf("phgo canvas header = %#v", got)
	}
}

func TestApplyCanvasHeaderModePhgoLiteUsesSpeciesIDAndSymbol(t *testing.T) {
	selected := []canvasSelectedRow{{
		ItemTitle: "1",
		Row: model.CanvasRow{
			RowNumber: 7,
			Kind:      model.CanvasKindFasta,
			FASTA: &model.QuerySequenceSource{
				OrganismShort: "Bd21-3",
				LabelName:     "Bd4CL1",
				GeneID:        "Bradi3g18960",
				Sequence:      "MPEPTIDE",
			},
		},
	}}
	records := []model.ProteinSequenceRecord{{Header: ">temporary", Sequence: "MPEPTIDE"}}
	got := applyCanvasHeaderMode(records, selected, model.FastaHeaderModePhgoLite)
	want := ">Bd21-3|Bradi3g18960(Bd4CL1)"
	if len(got) != 1 || got[0].Header != want {
		t.Fatalf("PHgo Lite canvas header = %#v, want %q", got, want)
	}
}

func TestApplyCanvasHeaderModeDisplayNameUsesDisplayColumn(t *testing.T) {
	selected := []canvasSelectedRow{{
		ItemTitle: "1",
		Row: model.CanvasRow{
			RowNumber:   5,
			Kind:        model.CanvasKindFasta,
			DisplayName: "Tree PAL",
			FASTA: &model.QuerySequenceSource{
				Annotation: "row1",
				Sequence:   "MPEPTIDE",
			},
		},
	}}
	records := []model.ProteinSequenceRecord{{
		Header:         ">temporary",
		OriginalHeader: ">original",
		Sequence:       "MPEPTIDE",
	}}
	got := applyCanvasHeaderMode(records, selected, model.FastaHeaderModeDisplayName)
	if len(got) != 1 || got[0].Header != ">Tree PAL" {
		t.Fatalf("display-name canvas header = %#v", got)
	}
}

func TestApplyCanvasHeaderModePhgoUsesTildeForEmptyBlastSourceFields(t *testing.T) {
	selected := []canvasSelectedRow{{
		ItemTitle: "tree",
		Row: model.CanvasRow{
			RowNumber: 1,
			Kind:      model.CanvasKindBlast,
			BlastRow: &model.BlastResultRow{
				Species:    "Sp7498",
				LabelName:  "C4H",
				Protein:    "Sp7498_C4H_001",
				SequenceID: "PAC:123456",
			},
		},
	}}
	records := []model.ProteinSequenceRecord{{
		Header:         ">temporary",
		OriginalHeader: ">orig",
		Sequence:       "MPEPTIDE",
	}}
	got := applyCanvasHeaderMode(records, selected, model.FastaHeaderModePhgo)
	want := ">phgo://Sp7498/C4H/Sp7498_C4H_001\\~/~\\1/tree\\C4H"
	if len(got) != 1 || got[0].Header != want {
		t.Fatalf("empty-source canvas phgo header = %#v, want %q", got, want)
	}
}

func TestApplyCanvasHeaderModeMinimalUsesAvailableIDs(t *testing.T) {
	selected := []canvasSelectedRow{{
		ItemTitle: "1",
		Row: model.CanvasRow{
			RowNumber: 5,
			Kind:      model.CanvasKindFasta,
			FASTA: &model.QuerySequenceSource{
				TranscriptID: "AT2G37040.1",
				Sequence:     "MPEPTIDE",
			},
		},
	}}
	records := []model.ProteinSequenceRecord{{
		Header:         ">temporary",
		OriginalHeader: ">original",
		Sequence:       "MPEPTIDE",
	}}
	got := applyCanvasHeaderMode(records, selected, model.FastaHeaderModeMinimal)
	if len(got) != 1 || got[0].Header != ">AT2G37040.1" {
		t.Fatalf("minimal canvas header = %#v", got)
	}
}

func TestExportCanvasSelectionsWritesMixedCanvasRowsAsOnePhgoFasta(t *testing.T) {
	outputDir := t.TempDir()

	w := NewBlastWizard(io.Discard)
	w.source = fakeSource{
		name: "phytozome",
		sequences: map[string]string{
			"AT2G37040.1": "MKWAA",
			"PAC:123456":  "GGGTT",
		},
		headers: map[string]string{
			"AT2G37040.1": ">orig_keyword_header",
			"PAC:123456":  ">orig_blast_header",
		},
	}
	w.proteinSequenceCache = make(map[string]model.ProteinSequenceData)
	w.proteinSequenceMiss = make(map[string]error)
	w.lastKeywordSpecies = model.SpeciesCandidate{ProteomeID: 123, GenomeLabel: "Arabidopsis thaliana TAIR10"}

	state := canvasLaunchState{
		Items: []model.CanvasItem{
			{
				Title: "2",
				Rows: []model.CanvasRow{
					{
						RowNumber: 2,
						Kind:      model.CanvasKindFasta,
						FASTA: &model.QuerySequenceSource{
							Annotation:           "phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\2",
							Sequence:             "MPEPTIDE",
							OrganismShort:        "Sp7498",
							LabelName:            "C4H",
							GeneID:               "Sp7498_C4H_001",
							BlastSourceLabelName: "PAL1",
							BlastSourceGeneID:    "AT2G37040",
						},
					},
					{
						RowNumber: 3,
						Kind:      model.CanvasKindKeyword,
						KeywordRow: &model.KeywordResultRow{
							SourceDatabase:      "phytozome",
							SequenceHeaderLabel: "Athaliana_TAIR10",
							LabelName:           "CESA4",
							TranscriptID:        "AT2G37040.1",
							SequenceID:          "AT2G37040.1",
							GeneIdentifier:      "AT2G37040",
						},
					},
				},
				Selected: []bool{true, true},
			},
			{
				Title: "3",
				Rows: []model.CanvasRow{
					{
						RowNumber: 7,
						Kind:      model.CanvasKindBlast,
						BlastRow: &model.BlastResultRow{
							SourceDatabase: "phytozome",
							Species:        "Sp7498",
							LabelName:      "C4H",
							BlastLabelName: "PAL1",
							BlastGeneID:    "AT2G37040",
							Protein:        "Sp7498_C4H_001",
							SequenceID:     "PAC:123456",
							SubjectID:      "PAC:123456",
							TargetID:       456,
						},
					},
				},
				Selected: []bool{true},
			},
		},
	}

	err := w.exportCanvasSelections(context.Background(), state, exportSettings{
		BaseName:        "canvas_mixed",
		OutputDir:       outputDir,
		WriteText:       true,
		FastaHeaderMode: model.FastaHeaderModePhgo,
		UsePhgoHeader:   true,
	})
	if err != nil {
		t.Fatalf("exportCanvasSelections returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outputDir, "canvas_mixed.fasta"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := strings.ReplaceAll(strings.TrimSpace(string(data)), "\r\n", "\n")
	want := strings.Join([]string{
		">phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\2/2\\C4H",
		"MPEPTIDE",
		"",
		">phgo://Athaliana_TAIR10/CESA4/AT2G37040.1\\~/~\\3/2\\CESA4",
		"MKWAA",
		"",
		">phgo://Sp7498/C4H/Sp7498_C4H_001\\PAL1/AT2G37040\\7/3\\C4H",
		"GGGTT",
	}, "\n")
	if got != want {
		t.Fatalf("mixed canvas FASTA = %q\nwant %q", got, want)
	}
}

func TestExportCanvasSelectionsOriginalHeaderFallsBackPerRow(t *testing.T) {
	outputDir := t.TempDir()

	w := NewBlastWizard(io.Discard)
	w.source = fakeSource{
		name: "phytozome",
		sequences: map[string]string{
			"AT2G37040.1": "MKWAA",
		},
		headers: map[string]string{
			"AT2G37040.1": ">orig_keyword_header",
		},
	}
	w.proteinSequenceCache = make(map[string]model.ProteinSequenceData)
	w.proteinSequenceMiss = make(map[string]error)
	w.lastKeywordSpecies = model.SpeciesCandidate{ProteomeID: 123, GenomeLabel: "Arabidopsis thaliana TAIR10"}

	state := canvasLaunchState{
		Items: []model.CanvasItem{
			{
				Title: "1",
				Rows: []model.CanvasRow{
					{
						RowNumber: 1,
						Kind:      model.CanvasKindFasta,
						FASTA: &model.QuerySequenceSource{
							Annotation: "plain fasta header",
							Sequence:   "MPEPTIDE",
						},
					},
					{
						RowNumber: 2,
						Kind:      model.CanvasKindKeyword,
						KeywordRow: &model.KeywordResultRow{
							SourceDatabase:      "phytozome",
							SequenceHeaderLabel: "Athaliana_TAIR10",
							LabelName:           "CESA4",
							TranscriptID:        "AT2G37040.1",
							SequenceID:          "AT2G37040.1",
							GeneIdentifier:      "AT2G37040",
						},
					},
				},
				Selected: []bool{true, true},
			},
		},
	}

	err := w.exportCanvasSelections(context.Background(), state, exportSettings{
		BaseName:        "canvas_original",
		OutputDir:       outputDir,
		WriteText:       true,
		FastaHeaderMode: model.FastaHeaderModeOriginal,
		UsePhgoHeader:   false,
	})
	if err != nil {
		t.Fatalf("exportCanvasSelections returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outputDir, "canvas_original.fasta"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := strings.ReplaceAll(strings.TrimSpace(string(data)), "\r\n", "\n")
	want := strings.Join([]string{
		">plain_fasta_header",
		"MPEPTIDE",
		"",
		">orig_keyword_header",
		"MKWAA",
	}, "\n")
	if got != want {
		t.Fatalf("original-header canvas FASTA = %q\nwant %q", got, want)
	}
}

func TestExportCanvasSelectionsAllRowsIncludesUncheckedRows(t *testing.T) {
	outputDir := t.TempDir()
	w := NewBlastWizard(io.Discard)
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "1",
			Selected: []bool{true, false},
			Rows: []model.CanvasRow{
				{
					RowNumber: 1,
					Kind:      model.CanvasKindFasta,
					FASTA: &model.QuerySequenceSource{
						Annotation: "row1",
						Sequence:   "AAAA",
					},
				},
				{
					RowNumber:   2,
					Kind:        model.CanvasKindFasta,
					DisplayName: "Unchecked",
					FASTA: &model.QuerySequenceSource{
						Annotation: "row2",
						Sequence:   "BBBB",
					},
				},
			},
		}},
	}
	err := w.exportCanvasSelections(context.Background(), state, exportSettings{
		BaseName:        "canvas_all_rows",
		OutputDir:       outputDir,
		WriteText:       true,
		WriteAllRows:    true,
		FastaHeaderMode: model.FastaHeaderModeDisplayName,
	})
	if err != nil {
		t.Fatalf("exportCanvasSelections returned error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputDir, "canvas_all_rows.fasta"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := strings.ReplaceAll(strings.TrimSpace(string(data)), "\r\n", "\n")
	want := strings.Join([]string{
		">row1",
		"AAAA",
		"",
		">Unchecked",
		"BBBB",
	}, "\n")
	if got != want {
		t.Fatalf("all-row canvas FASTA = %q\nwant %q", got, want)
	}
}

func TestExportCanvasSelectionsPlainFastaSkipsRowsWithoutExportSequence(t *testing.T) {
	outputDir := t.TempDir()
	w := NewBlastWizard(io.Discard)
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "1",
			Selected: []bool{true, true},
			Rows: []model.CanvasRow{
				{
					RowNumber: 1,
					Kind:      model.CanvasKindFasta,
					FASTA: &model.QuerySequenceSource{
						Annotation: "empty",
					},
				},
				{
					RowNumber: 2,
					Kind:      model.CanvasKindFasta,
					FASTA: &model.QuerySequenceSource{
						Annotation: "ready",
						Sequence:   "MPEPTIDE",
					},
				},
			},
		}},
	}
	err := w.exportCanvasSelections(context.Background(), state, exportSettings{
		BaseName:        "canvas_ready_only",
		OutputDir:       outputDir,
		WriteText:       true,
		FastaHeaderMode: model.FastaHeaderModeOriginal,
	})
	if err != nil {
		t.Fatalf("exportCanvasSelections returned error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputDir, "canvas_ready_only.fasta"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := strings.ReplaceAll(strings.TrimSpace(string(data)), "\r\n", "\n")
	want := strings.Join([]string{
		">ready",
		"MPEPTIDE",
	}, "\n")
	if got != want {
		t.Fatalf("plain canvas FASTA = %q\nwant %q", got, want)
	}
}

func TestExportCanvasSelectionsIgnoresRemovedConvertedFastaOption(t *testing.T) {
	outputDir := t.TempDir()
	w := NewBlastWizard(io.Discard)
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "1",
			Selected: []bool{true},
			Rows: []model.CanvasRow{{
				RowNumber: 1,
				Kind:      model.CanvasKindFasta,
				FASTA: &model.QuerySequenceSource{
					Annotation: "ready",
					Sequence:   "MPEPTIDE",
				},
			}},
		}},
	}
	err := w.exportCanvasSelections(context.Background(), state, exportSettings{
		BaseName:            "canvas_removed_converted",
		OutputDir:           outputDir,
		WriteConvertedFasta: true,
	})
	if err != nil {
		t.Fatalf("exportCanvasSelections returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "canvas_removed_converted_converted.fasta")); !os.IsNotExist(err) {
		t.Fatalf("removed converted FASTA export should not write a file, stat err=%v", err)
	}
}

func TestExportCanvasSelectionsUsesAllFASTASequenceFields(t *testing.T) {
	outputDir := t.TempDir()
	w := NewBlastWizard(io.Discard)
	state := canvasLaunchState{
		Items: []model.CanvasItem{{
			Title:    "1",
			Selected: []bool{true},
			Rows: []model.CanvasRow{{
				RowNumber: 1,
				Kind:      model.CanvasKindFasta,
				FASTA: &model.QuerySequenceSource{
					Annotation:      ">row1",
					ProteinSequence: "MPEPTIDE",
				},
			}},
		}},
	}
	err := w.exportCanvasSelections(context.Background(), state, exportSettings{
		BaseName:        "canvas_allseq",
		OutputDir:       outputDir,
		WriteText:       true,
		FastaHeaderMode: model.FastaHeaderModeOriginal,
		UsePhgoHeader:   false,
	})
	if err != nil {
		t.Fatalf("exportCanvasSelections returned error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputDir, "canvas_allseq.fasta"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "MPEPTIDE") {
		t.Fatalf("FASTA export should include protein-sequence fallback: %q", string(data))
	}
}

func TestAssignCanvasRowNumbersContinuesFromExistingMaximum(t *testing.T) {
	existing := []model.CanvasRow{
		{RowNumber: 1, Kind: model.CanvasKindFasta},
		{RowNumber: 4, Kind: model.CanvasKindFasta},
	}
	next := assignCanvasRowNumbers([]model.CanvasRow{
		{Kind: model.CanvasKindFasta},
		{Kind: model.CanvasKindFasta},
	}, nextCanvasRowNumber(existing))
	if next[0].RowNumber != 5 || next[1].RowNumber != 6 {
		t.Fatalf("assigned row numbers = %#v, want 5 and 6", next)
	}
}

func TestAssignCanvasRowNumbersSupportsNegativeSequenceWithoutZero(t *testing.T) {
	rows := assignCanvasRowNumbers([]model.CanvasRow{
		{Kind: model.CanvasKindFasta},
		{Kind: model.CanvasKindFasta},
		{Kind: model.CanvasKindFasta},
	}, -3)
	if rows[0].RowNumber != -3 || rows[1].RowNumber != -2 || rows[2].RowNumber != -1 {
		t.Fatalf("assigned negative row numbers = %#v, want -3, -2, -1", rows)
	}
}

func TestHydrateSnapshotArtifactsReportsMissingPayload(t *testing.T) {
	err := hydrateSnapshotArtifacts(sessionsnapshot.Snapshot{
		Artifacts: &sessionsnapshot.ArtifactManifestV2{
			Entries: []sessionsnapshot.ArtifactEntryV2{{
				ID:          "artifacts/cache/uniprot/search/entry.json",
				Path:        "artifacts/cache/uniprot/search/entry.json",
				RestorePath: filepath.Join("C:\\", "missing", "entry.json"),
			}},
		},
		ArtifactPayloads: map[string][]byte{},
	})
	if err == nil || !strings.Contains(err.Error(), "missing packed payload") {
		t.Fatalf("expected missing payload error, got %v", err)
	}
}

func TestHydrateSnapshotArtifactsRemapsLegacyOutputTreeRestorePathToCache(t *testing.T) {
	outputDir, err := appfs.OutputDir()
	if err != nil {
		t.Fatalf("OutputDir returned error: %v", err)
	}
	legacyPath := filepath.Join(outputDir, "tree", "legacy-session", "run1", "tree.nwk")
	cacheDir, err := canvasTreeArtifactDir("legacy-session", "run1")
	if err != nil {
		t.Fatalf("resolve Canvas tree cache: %v", err)
	}
	cachePath := filepath.Join(cacheDir, "tree.nwk")
	_ = os.Remove(cachePath)
	t.Cleanup(func() { _ = os.Remove(cachePath) })

	err = hydrateSnapshotArtifacts(sessionsnapshot.Snapshot{
		Artifacts: &sessionsnapshot.ArtifactManifestV2{
			Entries: []sessionsnapshot.ArtifactEntryV2{{
				ID:          "tree/tree.nwk",
				Path:        "artifacts/tree/legacy-session/run1/tree.nwk",
				RestorePath: legacyPath,
			}},
		},
		ArtifactPayloads: map[string][]byte{
			"artifacts/tree/legacy-session/run1/tree.nwk": []byte("(PHGOT000001);"),
		},
	})
	if err != nil {
		t.Fatalf("hydrateSnapshotArtifacts returned error: %v", err)
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("expected remapped cache artifact at %s: %v", cachePath, err)
	}
	if strings.TrimSpace(string(data)) != "(PHGOT000001);" {
		t.Fatalf("restored cache artifact = %q", string(data))
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy output tree path should stay absent, stat err=%v", err)
	}
}

func TestLoadSessionSnapshotWithProgressSuppressTaskModals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session")
	if err := sessionsnapshot.WriteFile(path, sessionsnapshot.Snapshot{
		Context: sessionsnapshot.ContextV1{
			CreatedAt: time.Now(),
			Mode:      string(ModeKeyword),
		},
		Keyword: &sessionsnapshot.KeywordResultV1{
			SelectedSpecies: model.SpeciesCandidate{JBrowseName: "TAIR10", GenomeLabel: "TAIR10"},
			Groups: []model.KeywordSearchGroup{{
				SearchTerm: "PAL",
				Rows:       []model.KeywordResultRow{{LabelName: "PAL1", SequenceID: "AT2G37040.1"}},
			}},
		},
	}); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	w := NewBlastWizardWithTUIInfo(&bytes.Buffer{}, TUIInfo{})
	w.suppressTaskModals = true
	load, err := w.loadSessionSnapshotWithProgress(context.Background(), path+sessionsnapshot.FileExtension)
	if err != nil {
		t.Fatalf("loadSessionSnapshotWithProgress returned error: %v", err)
	}
	if load.snapshot.Keyword == nil || len(load.snapshot.Keyword.Groups) != 1 {
		t.Fatalf("loaded snapshot keyword module = %#v", load.snapshot.Keyword)
	}
	if load.restoreErr != nil {
		t.Fatalf("restoreErr = %v, want nil", load.restoreErr)
	}
}

func TestHydrateKeywordSnapshotRestoresFullRowSelectionState(t *testing.T) {
	w := NewBlastWizardWithTUIInfo(&bytes.Buffer{}, TUIInfo{})
	state := tui.RowSelectionState{
		Valid:          true,
		SelectedRow:    7,
		SelectedColumn: 5,
		RowOffset:      11,
		ColumnOffset:   3,
		Sort:           tui.TableSort{Column: 4, Direction: tui.SortDescending},
		ControlHeaders: true,
		HeaderColumn:   4,
	}
	snapshot := sessionsnapshot.Snapshot{
		Context: sessionsnapshot.ContextV1{Mode: string(ModeKeyword)},
		Keyword: &sessionsnapshot.KeywordResultV1{
			SelectedSpecies: model.SpeciesCandidate{JBrowseName: "TAIR10", GenomeLabel: "TAIR10"},
			Groups: []model.KeywordSearchGroup{{
				SearchTerm: "PAL",
				Rows: []model.KeywordResultRow{
					{SourceDatabase: "tair", LabelName: "PAL1", ProteinID: "AT2G37040.1", SequenceID: "AT2G37040.1"},
					{SourceDatabase: "tair", LabelName: "PAL2", ProteinID: "AT3G53260.1", SequenceID: "AT3G53260.1"},
				},
			}},
			Selected: []bool{false, true},
		},
		KeywordReview: &sessionsnapshot.KeywordReviewStateV1{
			SelectionState: state,
		},
	}
	_, groups, _, _, err := w.hydrateKeywordSnapshot(snapshot)
	if err != nil {
		t.Fatalf("hydrateKeywordSnapshot returned error: %v", err)
	}
	got := w.prompt.SnapshotKeywordReviewState(groups)
	if got != state {
		t.Fatalf("keyword review state = %#v, want %#v", got, state)
	}
}

func TestHydrateBlastSnapshotRestoresSingleTableSelectionState(t *testing.T) {
	w := NewBlastWizardWithTUIInfo(&bytes.Buffer{}, TUIInfo{})
	state := tui.RowSelectionState{
		Valid:          true,
		SelectedRow:    4,
		SelectedColumn: 8,
		RowOffset:      9,
		ColumnOffset:   2,
		Sort:           tui.TableSort{Column: 6, Direction: tui.SortDescending},
		ControlHeaders: true,
		HeaderColumn:   6,
	}
	rows := []model.BlastResultRow{
		{SourceDatabase: "phytozome", Protein: "AT2G37040.1"},
		{SourceDatabase: "phytozome", Protein: "AT3G53260.1"},
	}
	snapshot := sessionsnapshot.Snapshot{
		Context: sessionsnapshot.ContextV1{Mode: string(ModeBlast)},
		Blast: &sessionsnapshot.BlastResultV1{
			SelectedSpecies:  model.SpeciesCandidate{JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis"},
			OriginalRunCount: 1,
			Prepared: []sessionsnapshot.BlastQueryItemV1{{
				LabelName: "PAL1",
				Sequence:  "MPEPTIDE",
			}},
			Runs: []sessionsnapshot.BlastRunV1{{
				Index:   1,
				Item:    sessionsnapshot.BlastQueryItemV1{LabelName: "PAL1", Sequence: "MPEPTIDE"},
				Results: model.BlastResult{Rows: rows},
			}},
			Selected:    []bool{true, false},
			FilterFlags: []bool{false, false},
		},
		BlastReview: &sessionsnapshot.BlastReviewStateV1{
			SingleSelectionState: state,
		},
	}
	_, _, runs, _, err := w.hydrateBlastSnapshot(snapshot)
	if err != nil {
		t.Fatalf("hydrateBlastSnapshot returned error: %v", err)
	}
	got := w.prompt.SnapshotBlastRowReviewState(runs[0].Results.Rows)
	if got != state {
		t.Fatalf("blast single-table state = %#v, want %#v", got, state)
	}
}

func TestHydrateBlastSnapshotRestoresMultiRunSelectionState(t *testing.T) {
	w := NewBlastWizardWithTUIInfo(&bytes.Buffer{}, TUIInfo{})
	state := tui.BlastRunSelectionState{
		Valid:        true,
		CurrentRun:   1,
		ControlMode:  1,
		ListOffset:   5,
		Sort:         tui.TableSort{Column: 3, Direction: tui.SortDescending},
		HeaderColumn: 3,
		Tables: []tui.BlastRunTableState{
			{Valid: true, SelectedRow: 2, SelectedColumn: 4, RowOffset: 3, ColumnOffset: 1},
			{Valid: true, SelectedRow: 6, SelectedColumn: 5, RowOffset: 7, ColumnOffset: 2},
		},
	}
	runs := []sessionsnapshot.BlastRunV1{
		{
			Index:   1,
			Item:    sessionsnapshot.BlastQueryItemV1{LabelName: "PAL1", Sequence: "MPEPTIDE"},
			Results: model.BlastResult{Rows: []model.BlastResultRow{{SourceDatabase: "phytozome", Protein: "AT2G37040.1"}}},
		},
		{
			Index:   2,
			Item:    sessionsnapshot.BlastQueryItemV1{LabelName: "PAL2", Sequence: "MPEPTIDE"},
			Results: model.BlastResult{Rows: []model.BlastResultRow{{SourceDatabase: "phytozome", Protein: "AT3G53260.1"}}},
		},
	}
	snapshot := sessionsnapshot.Snapshot{
		Context: sessionsnapshot.ContextV1{Mode: string(ModeBlast)},
		Blast: &sessionsnapshot.BlastResultV1{
			SelectedSpecies:  model.SpeciesCandidate{JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis"},
			OriginalRunCount: 2,
			Prepared: []sessionsnapshot.BlastQueryItemV1{
				{LabelName: "PAL1", Sequence: "MPEPTIDE"},
				{LabelName: "PAL2", Sequence: "MPEPTIDE"},
			},
			Runs:             runs,
			SelectedByRun:    [][]bool{{true}, {false}},
			FilterFlagsByRun: [][]bool{{false}, {true}},
		},
		BlastReview: &sessionsnapshot.BlastReviewStateV1{
			MultiSelectionState: state,
		},
	}
	_, _, restoredRuns, _, err := w.hydrateBlastSnapshot(snapshot)
	if err != nil {
		t.Fatalf("hydrateBlastSnapshot returned error: %v", err)
	}
	got := w.prompt.SnapshotBlastRunsReviewState(blastRunViews(restoredRuns))
	if got.Valid != state.Valid || got.CurrentRun != state.CurrentRun || got.ControlMode != state.ControlMode || got.ListOffset != state.ListOffset || got.Sort != state.Sort || got.HeaderColumn != state.HeaderColumn {
		t.Fatalf("blast multi-run state mismatch: got=%#v want=%#v", got, state)
	}
	if len(got.Tables) != len(state.Tables) {
		t.Fatalf("blast multi-run table state length = %d, want %d", len(got.Tables), len(state.Tables))
	}
	for i := range state.Tables {
		if got.Tables[i] != state.Tables[i] {
			t.Fatalf("blast multi-run table state[%d] = %#v, want %#v", i, got.Tables[i], state.Tables[i])
		}
	}
}

func TestSnapshotKeywordSequenceCacheSkipsMissingTAIRSequenceFailures(t *testing.T) {
	w := NewBlastWizardWithTUIInfo(&bytes.Buffer{}, TUIInfo{})
	w.source = fakeSource{
		name: "tair",
		sequences: map[string]string{
			"AT2G37040.1": "MPEPTIDE",
		},
		sequenceErrors: map[string]error{
			"AT5G04230": fmt.Errorf("empty TAIR FASTA URL"),
		},
	}
	selected := model.SpeciesCandidate{ProteomeID: 370201, JBrowseName: "TAIR10", GenomeLabel: "TAIR10"}
	rows := []model.KeywordResultRow{
		{SourceDatabase: "tair", SequenceID: "AT2G37040.1", TranscriptID: "AT2G37040.1", LabelName: "PAL1"},
		{SourceDatabase: "tair", SequenceID: "AT5G04230", TranscriptID: "AT5G04230", LabelName: "BAD"},
	}
	cache, err := w.snapshotKeywordSequenceCache(context.Background(), selected, rows)
	if err != nil {
		t.Fatalf("snapshotKeywordSequenceCache returned error: %v", err)
	}
	if cache == nil || len(cache.Entries) != 1 {
		t.Fatalf("sequence cache entries = %#v, want 1 good entry", cache)
	}
	if cache.Entries[0].SequenceID != "AT2G37040.1" {
		t.Fatalf("sequence cache first entry = %#v", cache.Entries[0])
	}
}

func TestAutoIdentifyBlastLabelsWithProgressSkipsTaskModalWhenSuppressed(t *testing.T) {
	w := &BlastWizard{
		httpClient:         defaultHTTPClient(),
		suppressTaskModals: true,
	}
	items := []blastQueryItem{
		{
			LabelName: "PAL1",
			QuerySource: &model.QuerySequenceSource{
				PhgoAliases: "PAL1; PHENYLALANINE AMMONIA-LYASE 1",
			},
		},
	}

	out, err := w.autoIdentifyBlastLabelsWithProgress(context.Background(), model.SpeciesCandidate{}, items)
	if err != nil {
		t.Fatalf("autoIdentifyBlastLabelsWithProgress returned error: %v", err)
	}
	if len(out) != 1 || out[0].LabelName != "PAL1" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestAutoIdentifyNCBIKeywordGeneLociStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	groups := []model.KeywordSearchGroup{
		{SearchTerm: "XP_1", Rows: []model.KeywordResultRow{{SequenceID: "XP_1"}}},
	}
	w := &BlastWizard{httpClient: defaultHTTPClient()}
	_, err := w.autoIdentifyNCBIKeywordGeneLoci(ctx, groups, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("autoIdentifyNCBIKeywordGeneLoci error = %v, want context.Canceled", err)
	}
}

func TestReplayExportFilterSettingsRelaxesGenomeTargetPrograms(t *testing.T) {
	tblastn := replayExportFilterSettings(model.BlastRequest{Program: "local:TBLASTN", TargetType: "genome"})
	if tblastn.UseTargetCanonicalLengthRatio || tblastn.RequireTargetCanonicalLengthRatio {
		t.Fatalf("tblastn canonical ratio should be disabled: %#v", tblastn)
	}
	if tblastn.InterProDomainMode != "off" || tblastn.RequireInterProConservedRegion {
		t.Fatalf("tblastn interpro hard rules should be disabled: %#v", tblastn)
	}

	blastp := replayExportFilterSettings(model.BlastRequest{Program: "local:BLASTP", TargetType: "proteome"})
	if !blastp.UseTargetCanonicalLengthRatio || !blastp.RequireTargetCanonicalLengthRatio {
		t.Fatalf("blastp canonical ratio should stay enabled: %#v", blastp)
	}
	if blastp.InterProDomainMode != "conserved_region" || !blastp.RequireInterProConservedRegion {
		t.Fatalf("blastp interpro defaults should stay enabled: %#v", blastp)
	}
}

func TestKeywordRowLabelnameCandidatesIncludeENAIdentifiers(t *testing.T) {
	row := model.KeywordResultRow{ExtraColumns: map[string]string{
		"ena_locus_tag":  "TAIR12_TAIR12_AT2G37040",
		"ena_protein_id": "CAO7037755.1",
		"ena_accession":  "CAO7037755",
	}}
	candidates := allKeywordRowSymbolCandidates(row)
	for _, want := range []string{"TAIR12_TAIR12_AT2G37040", "CAO7037755.1", "CAO7037755"} {
		if !slices.Contains(candidates, want) {
			t.Fatalf("ENA labelname candidate %q missing from %#v", want, candidates)
		}
	}
}

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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/labelname"
	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/prompt"
	"github.com/KiriKirby/phytozome-go/internal/report"
	"github.com/KiriKirby/phytozome-go/internal/source"
	"github.com/KiriKirby/phytozome-go/internal/uniprot"
)

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
		WriteReport:   true,
		WriteText:     true,
		WriteExcel:    false,
		WriteRawExcel: true,
		UsePhgoHeader: true,
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
}

func TestFilesSummaryIncludesRawText(t *testing.T) {
	summary := filesSummary(exportFileResult{
		TextPath:     filepath.Join("out", "PAL.fasta"),
		RawExcelPath: filepath.Join("out", "PAL_raw.xlsx"),
		RawTextPath:  filepath.Join("out", "PAL_raw.fasta"),
	})

	if !strings.Contains(summary, "Raw text") || !strings.Contains(summary, "PAL_raw.fasta") {
		t.Fatalf("raw text missing from files summary:\n%s", summary)
	}
}

func TestInspectBlastGeneratedFilesIncludesRawText(t *testing.T) {
	dir := t.TempDir()
	rawTextPath := filepath.Join(dir, "PAL_raw.fasta")
	if err := os.WriteFile(rawTextPath, []byte(">PAL1\nMAAA\n"), 0o600); err != nil {
		t.Fatalf("write raw text fixture: %v", err)
	}

	files, err := inspectBlastGeneratedFilesList(context.Background(), []exportFileResult{{RawTextPath: rawTextPath}}, report.NewGeneratedFileInspector())
	if err != nil {
		t.Fatalf("inspect generated files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("generated file count = %d, want 1", len(files))
	}
	if files[0].Name != "PAL_raw.fasta" || files[0].Type != "raw BLAST peptide text" {
		t.Fatalf("raw text file metadata not captured: %#v", files[0])
	}
}

func TestInspectKeywordGeneratedFilesIncludesRawText(t *testing.T) {
	dir := t.TempDir()
	rawTextPath := filepath.Join(dir, "keyword_raw.fasta")
	if err := os.WriteFile(rawTextPath, []byte(">hit\nMAAA\n"), 0o600); err != nil {
		t.Fatalf("write raw text fixture: %v", err)
	}

	files, err := inspectKeywordGeneratedFiles(context.Background(), exportFileResult{RawTextPath: rawTextPath}, report.SequenceAudit{}, report.NewGeneratedFileInspector())
	if err != nil {
		t.Fatalf("inspect generated files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("generated file count = %d, want 1", len(files))
	}
	if files[0].Name != "keyword_raw.fasta" || files[0].Type != "raw peptide text" {
		t.Fatalf("raw keyword text file metadata not captured: %#v", files[0])
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

func TestBlastTXTHeaderLabelPrefersLabelName(t *testing.T) {
	item := blastQueryItem{LabelName: "AtPAL1"}
	if got := blastTXTHeaderLabel(item, "CustomFileName"); got != "AtPAL1" {
		t.Fatalf("txt header label = %q, want label name", got)
	}
}

func TestBlastTXTHeaderLabelFallsBackToFileName(t *testing.T) {
	item := blastQueryItem{}
	if got := blastTXTHeaderLabel(item, "CustomFileName"); got != "CustomFileName" {
		t.Fatalf("txt header label = %q, want file name", got)
	}
}

func TestFamilyTXTHeaderLabelPrefersQueryIdentityBeforeFallback(t *testing.T) {
	source := &model.QuerySequenceSource{
		LabelName:    "",
		Aliases:      "ATPAL1; PAL1",
		GeneID:       "AT2G37040",
		TranscriptID: "AT2G37040.1",
		ProteinID:    "AT2G37040.1",
	}
	if got := familyTXTHeaderLabel(source, "PAL"); got != "ATPAL1" {
		t.Fatalf("familyTXTHeaderLabel()=%q want ATPAL1", got)
	}
}

func TestFamilyTXTQueryIndexesRespectPrependOnlyFirstQuery(t *testing.T) {
	sources := []*model.QuerySequenceSource{{LabelName: "PAL1"}, {LabelName: "PAL2"}, {LabelName: "PAL3"}}
	firstOnly := familyTXTQueryIndexes(sources, model.FamilyBlastSettings{PrependOnlyFirstQuery: true})
	if len(firstOnly) != 1 || firstOnly[0] != 0 {
		t.Fatalf("first-only indexes = %#v", firstOnly)
	}
	all := familyTXTQueryIndexes(sources, model.FamilyBlastSettings{PrependOnlyFirstQuery: false})
	if len(all) != 3 || all[0] != 0 || all[2] != 2 {
		t.Fatalf("all indexes = %#v", all)
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
	if len(labels) != 1 || labels[0] != "P48522" {
		t.Fatalf("generic keyword auto labels should not apply Lemna-specific fallback ordering: %#v", labels)
	}
}

func TestAutoIdentifyKeywordLabelsFallsBackToPhytozomeRowMetadataWhenSynonymsMissing(t *testing.T) {
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
	if len(labels) != 1 || labels[0] != "4CL1" {
		t.Fatalf("expected phytozome keyword fallback to preserve row metadata label, got %#v", labels)
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

func TestAutoIdentifyBlastLabelFromPhytozomeUsesBestAliasCandidate(t *testing.T) {
	w := &BlastWizard{}
	src := keywordMapSource{rowsByKeyword: map[string][]model.KeywordResultRow{
		"AT2G37040.1": {{
			Aliases:      "ATPAL1; PAL1",
			TranscriptID: "AT2G37040.1",
		}},
	}}
	item := blastQueryItem{
		RawInput: ">A.thaliana TAIR10|AT2G37040.1\nMPEPTIDE",
		QuerySource: &model.QuerySequenceSource{
			ProteinID:    "AT2G37040.1",
			TranscriptID: "AT2G37040.1",
			GeneID:       "AT2G37040",
		},
	}

	got := w.autoIdentifyBlastLabelFromPhytozome(context.Background(), src, model.SpeciesCandidate{ProteomeID: 167}, item)
	if got != "PAL1" {
		t.Fatalf("unexpected label: %q", got)
	}
}

func TestAutoIdentifyBlastLabelUsesQuerySourceBeforeFastaHeaderFallback(t *testing.T) {
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

func TestAutoIdentifyBlastLabelResultUsesDatabaseAliasesBeforeFastaHeaderFallback(t *testing.T) {
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
	if result.Label != "ANAC001" {
		t.Fatalf("label = %q, want database synonym ANAC001", result.Label)
	}
	if containsString(result.Aliases, "OldName") {
		t.Fatalf("aliases = %#v, want FASTA header ignored when database candidates exist", result.Aliases)
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

func TestAutoIdentifyBlastLabelResultLetsDatabaseAliasOverrideFastaHeaderFallback(t *testing.T) {
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
	if result.Label != "ANAC101" {
		t.Fatalf("label = %q, want database synonym ANAC101", result.Label)
	}
	if containsString(result.Aliases, "HeaderName") {
		t.Fatalf("aliases = %#v, want FASTA header ignored when database candidates exist", result.Aliases)
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

func TestAutoIdentifyBlastLabelResultFallsBackToResolvedIDs(t *testing.T) {
	w := &BlastWizard{}
	item := blastQueryItem{QuerySource: &model.QuerySequenceSource{
		SourceDatabase: "phytozome",
		NormalizedURL:  "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT3G10340",
		GeneID:         "AT3G10340",
		TranscriptID:   "AT3G10340.1",
		ProteinID:      "PAC:19660032",
	}}
	got := w.autoIdentifyBlastLabel(context.Background(), keywordMapSource{}, model.SpeciesCandidate{}, item)
	if got != "PAC:19660032" {
		t.Fatalf("unexpected fallback label: %q", got)
	}
}

func TestAutoIdentifyBlastLabelFallsBackToStructuredIDsWhenNoDatabaseCandidates(t *testing.T) {
	w := &BlastWizard{}
	item := blastQueryItem{
		RawInput: ">A.thaliana TAIR10|AT5G62380.1 (AtVND6)\nMPEPTIDE",
		QuerySource: &model.QuerySequenceSource{
			ProteinID:    "AT5G62380.1",
			TranscriptID: "AT5G62380.1",
			GeneID:       "AT5G62380",
		},
	}
	got := w.autoIdentifyBlastLabel(context.Background(), keywordMapSource{}, model.SpeciesCandidate{}, item)
	if got != "AT5G62380.1" {
		t.Fatalf("unexpected ID fallback label: %q", got)
	}
}

func TestAutoIdentifyLemnaBlastSourceLabelPrefersPhytozomeThenLocalThenStructuredIDFallback(t *testing.T) {
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
		t.Fatalf("Label = %q, want ranked Phytozome synonym", result.Label)
	}
	if containsString(result.Aliases, "HeaderName") || containsString(result.Aliases, "LOCAL_SHOULD_NOT_WIN") {
		t.Fatalf("Aliases = %#v, want Phytozome aliases before Lemna local/header fallback", result.Aliases)
	}

	item.QuerySource.ProteinID = "missing"
	item.QuerySource.TranscriptID = ""
	item.QuerySource.LabelName = "C4H"
	item.RawInput = ">Spirodela polyrhiza (HeaderName)\nMPEPTIDE"
	result = w.autoIdentifyBlastLabelResult(context.Background(), src, model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza"}, item)
	if result.Label != "C4H" || !containsString(result.Aliases, "C4H") {
		t.Fatalf("local fallback result = %#v, want Lemna local aliases", result)
	}

	item.QuerySource.LabelName = ""
	item.QuerySource.PhgoAliases = ""
	item.QuerySource.Aliases = ""
	item.QuerySource.AutoDefine = ""
	result = w.autoIdentifyBlastLabelResult(context.Background(), src, model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza"}, item)
	if result.Label != "missing" || !containsString(result.Aliases, "missing") {
		t.Fatalf("ID fallback result = %#v, want structured ID fallback", result)
	}
}

func TestAutoIdentifyBlastHitLabelUsesPhytozomeFallbackAndSourceLabelLast(t *testing.T) {
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
	wants := []string{"ANAC101", "VND7", "AUTO1", "SOURCE1", "ANAC101"}
	wantTypes := []string{"phytozome synonyms", "phytozome symbols", "phytozome auto_define", "blast source labelname fallback", "phytozome synonyms"}
	for i := range wants {
		if got[i].LabelName != wants[i] || got[i].LabelNameType != wantTypes[i] {
			t.Fatalf("row %d label/type = %q/%q, want %q/%q", i, got[i].LabelName, got[i].LabelNameType, wants[i], wantTypes[i])
		}
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.fetchCount["AT5G62380.1"] != 1 {
		t.Fatalf("duplicate hit lookup count = %d, want 1", src.fetchCount["AT5G62380.1"])
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
	if src.fetchCount["AT5G62380.1"] != 1 {
		t.Fatalf("duplicate hit lookup count = %d, want 1", src.fetchCount["AT5G62380.1"])
	}
}

func TestAutoIdentifyBlastHitLabelReusesCachedIdentificationAcrossCalls(t *testing.T) {
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
	if first[0].LabelName != "ANAC101" || second[0].LabelName != "ANAC101" {
		t.Fatalf("cached hit label = %q/%q, want ANAC101", first[0].LabelName, second[0].LabelName)
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.fetchCount["AT5G62380.1"] != 1 {
		t.Fatalf("cross-call hit lookup count = %d, want 1", src.fetchCount["AT5G62380.1"])
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
	if got[0].LabelName != "C4H" {
		t.Fatalf("LabelName = %q, want Lemna local hit alias C4H", got[0].LabelName)
	}
	if got[0].LabelNameType != "lemna local aliases" {
		t.Fatalf("LabelNameType = %q, want lemna local aliases", got[0].LabelNameType)
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
	if got[0].LabelNameType != "lemna local aliases" {
		t.Fatalf("LabelNameType = %q, want lemna local aliases", got[0].LabelNameType)
	}
	if strings.Contains(got[0].PhgoAliases, "4CLL4 Os03g0132000") {
		t.Fatalf("PhgoAliases kept whitespace list as one alias: %q", got[0].PhgoAliases)
	}
	aliases := labelname.SplitAliases(got[0].PhgoAliases)
	for _, alias := range []string{"4CLL4", "Os03g0132000", "LOC_Os03g04000", "OsJ_09299"} {
		found := false
		for _, gotAlias := range aliases {
			if strings.EqualFold(gotAlias, alias) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("PhgoAliases = %q, missing split alias %q", got[0].PhgoAliases, alias)
		}
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
	if got[0].LabelNameType != "lemna local aliases" {
		t.Fatalf("LabelNameType = %q, want lemna local aliases", got[0].LabelNameType)
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
	if got[0].LabelName != "SOURCE1" || got[0].LabelNameType != "blast source labelname fallback" {
		t.Fatalf("got label/type = %q/%q, want source label fallback", got[0].LabelName, got[0].LabelNameType)
	}
	if got[0].PhgoAliases != "SOURCE1" {
		t.Fatalf("PhgoAliases = %q, want source label as last-resort hit alias", got[0].PhgoAliases)
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
	if lookupSource.fetchCount["AT2G30490.1"] != 1 || lookupSource.fetchCount["AT5G13930.1"] != 1 {
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
		LabelNameType:                 "lemna local aliases",
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
	if phgo.Source != "labelname system" {
		t.Fatalf("phgo_alias source = %q, want labelname system", phgo.Source)
	}
	if !strings.Contains(phgo.Meaning, "BLAST hit row") || !strings.Contains(phgo.CollectionMethod, "BLAST-hit labelname") {
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

	if rows[0].LabelName != "ANAC101" || rows[0].LabelNameType != "phytozome synonyms" {
		t.Fatalf("first hit label/type = %q/%q, want hit-level phytozome synonyms", rows[0].LabelName, rows[0].LabelNameType)
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
			wantHitType: "phytozome synonyms",
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
			wantHitType: "lemna local aliases",
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
			if phgo := findColumnLineage(lineage, "phgo_alias"); phgo == nil || phgo.Source != "labelname system" {
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

func TestAutoIdentifyLemnaKeywordLabelsPrefersPhytozomeCandidates(t *testing.T) {
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
	if got[0].Aliases[0] != "CYP73A5" || got[0].SourceType != "phytozome synonyms" {
		t.Fatalf("label aliases/type = %#v/%q, want ranked phytozome synonyms", got[0].Aliases, got[0].SourceType)
	}
}

func TestAutoIdentifyLemnaKeywordLabelsFallsBackToLocalAliases(t *testing.T) {
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
	if len(got) != 1 || len(got[0].Aliases) == 0 || got[0].Aliases[0] != "C4H" {
		t.Fatalf("expected lemna local label fallback: %#v", got)
	}
	if got[0].SourceType != "lemna local aliases" {
		t.Fatalf("SourceType = %q, want lemna local aliases", got[0].SourceType)
	}
}

func TestAutoIdentifyLemnaKeywordLabelsDeduplicatesPhytozomeLookups(t *testing.T) {
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
	if lookupSource.fetchCount["AT2G30490.1"] != 1 {
		t.Fatalf("phytozome lookup count = %d, want 1", lookupSource.fetchCount["AT2G30490.1"])
	}
}

func TestAutoIdentifyLemnaKeywordLabelsStillQueriesPhytozomeWhenLocalAliasesExist(t *testing.T) {
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
		t.Fatalf("expected phytozome alias-derived label before local fallback: %#v", got)
	}
	lookupSource.mu.Lock()
	defer lookupSource.mu.Unlock()
	if lookupSource.fetchCount["AT2G30490.1"] != 1 {
		t.Fatalf("phytozome lookup count = %d, want 1", lookupSource.fetchCount["AT2G30490.1"])
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
	return nil, nil
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
		GeneReportURL:       "https://www.lemna.org/report/Sp_polyrhiza_9509/Sp9509d011g001470",
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
	if src.fetchCount["AT2G30490.1"] != 1 {
		t.Fatalf("missing-label item lookup count = %d, want 1", src.fetchCount["AT2G30490.1"])
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
	if !containsString(result.Aliases, "C4H") {
		t.Fatalf("expected alias ranking to still include phytozome aliases: %#v", result)
	}
}

func TestAutoIdentifyBlastLabelResultFromPhytozomeReusesWizardCache(t *testing.T) {
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
			ProteinID: "AT2G30490.1",
		},
	}
	species := model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis thaliana TAIR10"}

	first := w.autoIdentifyBlastLabelResultFromPhytozome(context.Background(), lookupSource, species, item)
	second := w.autoIdentifyBlastLabelResultFromPhytozome(context.Background(), lookupSource, species, item)
	if strings.TrimSpace(first.Label) == "" || strings.TrimSpace(second.Label) == "" {
		t.Fatalf("expected non-empty cached label results: first=%#v second=%#v", first, second)
	}
	if first.Label != second.Label {
		t.Fatalf("cached labels should match: first=%#v second=%#v", first, second)
	}
	lookupSource.mu.Lock()
	defer lookupSource.mu.Unlock()
	if lookupSource.fetchCount["AT2G30490.1"] != 1 {
		t.Fatalf("phytozome label lookup count = %d, want 1", lookupSource.fetchCount["AT2G30490.1"])
	}
}

func TestAutoIdentifyBlastLabelResultFromPhytozomeDeduplicatesConcurrentLookups(t *testing.T) {
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
			ProteinID: "AT2G30490.1",
		},
	}
	species := model.SpeciesCandidate{ProteomeID: 167, JBrowseName: "Athaliana_TAIR10", GenomeLabel: "Arabidopsis thaliana TAIR10"}
	var wg sync.WaitGroup
	results := make([]blastAutoLabelResult, 2)
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = w.autoIdentifyBlastLabelResultFromPhytozome(context.Background(), lookupSource, species, item)
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
	if lookupSource.fetchCount["AT2G30490.1"] != 1 {
		t.Fatalf("phytozome concurrent label lookup count = %d, want 1", lookupSource.fetchCount["AT2G30490.1"])
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

func TestAvailableBlastProgramsIncludeServerAndLocalCapabilities(t *testing.T) {
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
	want := []string{"blastn", "blastx", "tblastn", "blastp"}
	if len(got) != len(want) {
		t.Fatalf("unexpected server-only program count: got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected server-only program at %d: got %q want %q", i, got[i], want[i])
		}
	}

	localOnly := lemna.BlastCapability{
		HasNucleotideFasta: true,
		HasProteinFasta:    true,
	}
	got = availableBlastProgramsFromCapability(localOnly)
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
	want = []string{"blastn", "blastx", "tblastn", "blastp"}
	if len(got) != len(want) {
		t.Fatalf("unexpected mixed program count: got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected mixed program at %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestChooseLemnaBlastExecutionUsesProgramSpecificServerFlags(t *testing.T) {
	w := &BlastWizard{}
	selected := model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza 9509"}
	tests := []struct {
		name       string
		program    string
		serverCap  lemna.BlastCapability
		localCap   lemna.BlastCapability
		wantBoth   string
		wantServer string
		wantLocal  string
	}{
		{
			name:       "blastn",
			program:    "blastn",
			serverCap:  lemna.BlastCapability{ServerBlastNAvailable: true},
			localCap:   lemna.BlastCapability{HasNucleotideFasta: true},
			wantBoth:   "server",
			wantServer: "server",
			wantLocal:  "local",
		},
		{
			name:       "blastx",
			program:    "blastx",
			serverCap:  lemna.BlastCapability{ServerBlastXAvailable: true},
			localCap:   lemna.BlastCapability{HasProteinFasta: true},
			wantBoth:   "server",
			wantServer: "server",
			wantLocal:  "local",
		},
		{
			name:       "tblastn",
			program:    "tblastn",
			serverCap:  lemna.BlastCapability{ServerTBlastNAvailable: true},
			localCap:   lemna.BlastCapability{HasNucleotideFasta: true},
			wantBoth:   "server",
			wantServer: "server",
			wantLocal:  "local",
		},
		{
			name:       "blastp",
			program:    "blastp",
			serverCap:  lemna.BlastCapability{ServerBlastPAvailable: true},
			localCap:   lemna.BlastCapability{HasProteinFasta: true},
			wantBoth:   "server",
			wantServer: "server",
			wantLocal:  "local",
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
		if got, err := w.chooseLemnaBlastExecution(merge(tt.serverCap, tt.localCap), selected, tt.program); err != nil || got != tt.wantBoth {
			t.Fatalf("%s both = %q/%v, want %q/nil", tt.name, got, err, tt.wantBoth)
		}
		if got, err := w.chooseLemnaBlastExecution(tt.serverCap, selected, tt.program); err != nil || got != tt.wantServer {
			t.Fatalf("%s server-only = %q/%v, want %q/nil", tt.name, got, err, tt.wantServer)
		}
		if got, err := w.chooseLemnaBlastExecution(tt.localCap, selected, tt.program); err != nil || got != tt.wantLocal {
			t.Fatalf("%s local-only = %q/%v, want %q/nil", tt.name, got, err, tt.wantLocal)
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

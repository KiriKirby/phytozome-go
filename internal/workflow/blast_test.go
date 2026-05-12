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

	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/prompt"
	"github.com/KiriKirby/phytozome-go/internal/report"
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

func TestBuildQuerySequenceLabel(t *testing.T) {
	if got := buildQuerySequenceLabel("A.thaliana", "C4H"); got != "AtC4H" {
		t.Fatalf("unexpected arabidopsis label: %q", got)
	}
	if got := buildQuerySequenceLabel("A.thaliana", "AtCESA1"); got != "AtCESA1" {
		t.Fatalf("unexpected prefixed arabidopsis label: %q", got)
	}
	if got := buildQuerySequenceLabel("A.thaliana", "ATPAL1"); got != "AtPAL1" {
		t.Fatalf("unexpected uppercase arabidopsis label: %q", got)
	}
	if got := buildQuerySequenceLabel("S.polyrhiza", "Spipo1"); got != "Spipo1" {
		t.Fatalf("unexpected non-arabidopsis label: %q", got)
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
	}, "C4H", "out")

	if settings.BaseName != "C4H" || settings.OutputDir != "out" || !settings.WriteReport {
		t.Fatalf("basic export settings not preserved: %#v", settings)
	}
	if !settings.WriteText || settings.WriteExcel || !settings.WriteRawExcel {
		t.Fatalf("file type toggles not preserved: %#v", settings)
	}
}

func TestFilesSummaryIncludesRawText(t *testing.T) {
	summary := filesSummary(exportFileResult{
		TextPath:     filepath.Join("out", "PAL.txt"),
		RawExcelPath: filepath.Join("out", "PAL_raw.xlsx"),
		RawTextPath:  filepath.Join("out", "PAL_raw.txt"),
	})

	if !strings.Contains(summary, "Raw text") || !strings.Contains(summary, "PAL_raw.txt") {
		t.Fatalf("raw text missing from files summary:\n%s", summary)
	}
}

func TestInspectBlastGeneratedFilesIncludesRawText(t *testing.T) {
	dir := t.TempDir()
	rawTextPath := filepath.Join(dir, "PAL_raw.txt")
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
	if files[0].Name != "PAL_raw.txt" || files[0].Type != "raw BLAST peptide text" {
		t.Fatalf("raw text file metadata not captured: %#v", files[0])
	}
}

func TestInspectKeywordGeneratedFilesIncludesRawText(t *testing.T) {
	dir := t.TempDir()
	rawTextPath := filepath.Join(dir, "keyword_raw.txt")
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
	if files[0].Name != "keyword_raw.txt" || files[0].Type != "raw peptide text" {
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

func TestDetectFamilyBlastGroupsCanStripArabidopsisPrefixWhenEnabled(t *testing.T) {
	items := []blastQueryItem{{LabelName: "ATPAL1"}, {LabelName: "ATPAL2"}}
	settings := model.DefaultFamilyBlastSettings()
	settings.StripArabidopsisPrefix = true
	groups := detectFamilyBlastGroups(items, settings)
	if len(groups) != 1 {
		t.Fatalf("group count = %d, want 1: %#v", len(groups), groups)
	}
	if groups[0].Name != "PAL" {
		t.Fatalf("family name = %q, want PAL", groups[0].Name)
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

func TestPrependQuerySequenceRecordUsesProvidedHeaderLabel(t *testing.T) {
	source := &model.QuerySequenceSource{
		OrganismShort: "A.thaliana",
		Annotation:    "TAIR10",
		ProteinID:     "AT2G37040.1",
		Sequence:      "MSTN",
	}
	records := prependQuerySequenceRecord(nil, source, "AtPAL1")
	if len(records) != 1 {
		t.Fatalf("expected one query record, got %d", len(records))
	}
	if !strings.Contains(records[0].Header, "(AtPAL1)") {
		t.Fatalf("query header did not use label: %q", records[0].Header)
	}
	if strings.Contains(records[0].Header, "ATPAL1_export") {
		t.Fatalf("query header was polluted by file name: %q", records[0].Header)
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

func TestAutoIdentifyKeywordLabelsPrefersReadableCuratedLemnaLabel(t *testing.T) {
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
		t.Fatalf("unexpected curated lemna auto labels: %#v", labels)
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

func TestArabidopsisGeneSearchTermNormalizesIDs(t *testing.T) {
	tests := map[string]string{
		"At1g80820":          "At1g80820",
		"gene:AT1G80820.1":   "At1g80820",
		"foo|AT1G80820.1 xx": "At1g80820",
		"Sp9509d005g008190":  "",
	}
	for input, want := range tests {
		if got := arabidopsisGeneSearchTerm(input); got != want {
			t.Fatalf("arabidopsisGeneSearchTerm(%q)=%q want %q", input, got, want)
		}
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
	item := blastQueryItem{RawInput: ">A.thaliana TAIR10|AT2G37040.1\nMPEPTIDE"}

	got := w.autoIdentifyBlastLabelFromPhytozome(context.Background(), src, model.SpeciesCandidate{ProteomeID: 167}, item)
	if got != "PAL1" {
		t.Fatalf("unexpected label: %q", got)
	}
}

func TestBestAliasPrefersCanonicalFamilyStyleOverInternalPrefix(t *testing.T) {
	if got := bestAlias("ATPAL1; PAL1"); got != "PAL1" {
		t.Fatalf("bestAlias()=%q want PAL1", got)
	}
	if got := bestAlias("CYP84A1; FAH1; F5H1"); got != "F5H1" {
		t.Fatalf("bestAlias()=%q want F5H1", got)
	}
	if got := bestAlias("CYP98A3; REF8"); got != "CYP98A3" {
		t.Fatalf("bestAlias()=%q want CYP98A3", got)
	}
}

func TestLabelFromAutoDefineFindsCompactFunctionalAlias(t *testing.T) {
	if got := labelFromAutoDefine("(1 of 2) K09755 - ferulate-5-hydroxylase (CYP84A, F5H)"); got != "F5H" {
		t.Fatalf("labelFromAutoDefine()=%q want F5H", got)
	}
	if got := labelFromAutoDefine("(1 of 1) K09754 - coumaroylquinate(coumaroylshikimate) 3'-monooxygenase (CYP98A3, C3'H)"); got != "C3'H" {
		t.Fatalf("labelFromAutoDefine()=%q want C3'H", got)
	}
}

func TestAutoIdentifyBlastLabelPrefersFastaParentheticalLabel(t *testing.T) {
	w := &BlastWizard{}
	item := blastQueryItem{
		RawInput:    ">Arabidopsis thaliana TAIR10|AT5G62380.1 (AtVND6)\nMESLAHIPP",
		QuerySource: &model.QuerySequenceSource{Aliases: "SOMETHINGELSE; VND6", LabelName: "SOMETHINGELSE"},
	}
	got := w.autoIdentifyBlastLabel(context.Background(), keywordMapSource{}, model.SpeciesCandidate{}, item)
	if got != "VND6" {
		t.Fatalf("unexpected label: %q", got)
	}
}

func TestFastaHeaderLabelNameOnlyStripsMixedCaseAt(t *testing.T) {
	tests := map[string]string{
		"Arabidopsis thaliana TAIR10|AT5G62380.1 (AtVND6)": "VND6",
		"Arabidopsis thaliana TAIR10|AT5G62380.1 (VND6)":   "VND6",
		"Arabidopsis thaliana TAIR10|AT5G62380.1 (ATVND6)": "ATVND6",
	}
	for input, want := range tests {
		if got := fastaHeaderLabelName(input); got != want {
			t.Fatalf("fastaHeaderLabelName(%q)=%q want %q", input, got, want)
		}
	}
}

func TestAutoIdentifyBlastLabelUsesResolvedPhytozomeAliases(t *testing.T) {
	w := &BlastWizard{}
	item := blastQueryItem{QuerySource: &model.QuerySequenceSource{
		SourceDatabase: "phytozome",
		NormalizedURL:  "https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT3G10340",
		Aliases:        "PAL4; ATPAL4",
		LabelName:      "PAL4",
	}}
	got := w.autoIdentifyBlastLabel(context.Background(), keywordMapSource{}, model.SpeciesCandidate{}, item)
	if got != "PAL4" {
		t.Fatalf("unexpected label: %q", got)
	}
}

func TestAutoIdentifyBlastLabelResultKeepsFastaHeaderLabelAndRetainsKeywordAliases(t *testing.T) {
	w := &BlastWizard{}
	src := keywordMapSource{rowsByKeyword: map[string][]model.KeywordResultRow{
		"AT1G01010.1": {{Aliases: "NAC001; ANAC001", LabelName: "NAC001", AutoDefine: "NAC domain protein"}},
		"AT1G01010":   {{Aliases: "NAC001; ANAC001", LabelName: "NAC001", AutoDefine: "NAC domain protein"}},
	}}
	item := blastQueryItem{
		RawInput: ">A.thaliana TAIR10|AT1G01010.1 (OldName)\nMPEPTIDE",
		QuerySource: &model.QuerySequenceSource{
			ProteinID:    "AT1G01010.1",
			TranscriptID: "AT1G01010.1",
			GeneID:       "AT1G01010",
			LabelName:    "OldName",
		},
	}

	result := w.autoIdentifyBlastLabelResult(context.Background(), src, model.SpeciesCandidate{GenomeLabel: "Arabidopsis thaliana"}, item)
	if result.Label != "OldName" {
		t.Fatalf("label = %q, want FASTA header label OldName", result.Label)
	}
	found := false
	for _, alias := range result.Aliases {
		if alias == "ANAC001" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("aliases = %#v, want ANAC001 retained", result.Aliases)
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

func TestAutoIdentifyBlastLabelResultDoesNotLetDatabaseAliasOverrideFastaHeaderLabel(t *testing.T) {
	w := &BlastWizard{}
	src := keywordMapSource{rowsByKeyword: map[string][]model.KeywordResultRow{
		"AT5G62380.1": {{Aliases: "VND6; ANAC101", LabelName: "VND6", AutoDefine: "vascular related NAC domain 6"}},
	}}
	item := blastQueryItem{
		RawInput: ">Arabidopsis thaliana TAIR10|AT5G62380.1 (HeaderName)\nMESLAHIPP",
		QuerySource: &model.QuerySequenceSource{
			SourceDatabase: "fasta",
			ProteinID:      "AT5G62380.1",
			TranscriptID:   "AT5G62380.1",
			GeneID:         "AT5G62380",
			LabelName:      "HeaderName",
		},
		LabelName: "HeaderName",
	}

	result := w.autoIdentifyBlastLabelResult(context.Background(), src, model.SpeciesCandidate{GenomeLabel: "Arabidopsis thaliana"}, item)
	if result.Label != "HeaderName" {
		t.Fatalf("label = %q, want FASTA header label HeaderName", result.Label)
	}
	if !containsString(result.Aliases, "VND6") {
		t.Fatalf("aliases = %#v, want database alias retained without overriding label", result.Aliases)
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
	originalLabel := items[0].LabelName
	if originalLabel == "" {
		t.Fatal("sample FASTA should already contain a parenthetical label")
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
	if got := out[0].LabelName; got != originalLabel {
		t.Fatalf("label changed to %q, want preserved %q", got, originalLabel)
	}
	if got := out[0].QuerySource.ProteinID; got != "AT2G29130.1" {
		t.Fatalf("protein id = %q, want clean FASTA id AT2G29130.1", got)
	}
	aliases := splitAliasText(out[0].QuerySource.Aliases)
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

func TestAutoIdentifyBlastLabelFallsBackToResolvedIDs(t *testing.T) {
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

func TestAutoIdentifyBlastLabelKeepsECStyleFastaHeaderLabel(t *testing.T) {
	w := &BlastWizard{}
	item := blastQueryItem{
		RawInput: ">A.thaliana TAIR10|AT5G48930.1 (AtE2.3.1.133)\nMPEPTIDE",
		QuerySource: &model.QuerySequenceSource{
			ProteinID:    "AT5G48930.1",
			TranscriptID: "AT5G48930.1",
			GeneID:       "AT5G48930",
			LabelName:    "E2.3.1.133",
			Aliases:      "E2.3.1.133",
		},
	}
	got := w.autoIdentifyBlastLabel(context.Background(), keywordMapSource{}, model.SpeciesCandidate{}, item)
	if got != "E2.3.1.133" {
		t.Fatalf("unexpected FASTA header label: %q", got)
	}
}

func TestAutoIdentifyBlastLabelKeepsLowercaseFastaHeaderLabel(t *testing.T) {
	w := &BlastWizard{}
	item := blastQueryItem{
		RawInput: ">A.thaliana TAIR10|AT1G52760.1 (AtLysoPL2)\nMPEPTIDE",
		QuerySource: &model.QuerySequenceSource{
			ProteinID:    "AT1G52760.1",
			TranscriptID: "AT1G52760.1",
			GeneID:       "AT1G52760",
			LabelName:    "LysoPL2",
			Aliases:      "LysoPL2",
		},
	}
	got := w.autoIdentifyBlastLabel(context.Background(), keywordMapSource{}, model.SpeciesCandidate{}, item)
	if got != "LysoPL2" {
		t.Fatalf("unexpected FASTA header label: %q", got)
	}
}

func TestBestTrustedAutoLabelPrefersCanonicalCompactSymbol(t *testing.T) {
	if got := bestTrustedAutoLabel("CYP84A1", "F5H1", "LysoPL2"); got != "F5H1" {
		t.Fatalf("bestTrustedAutoLabel()=%q want F5H1", got)
	}
}

func TestBestTrustedAutoLabelRejectsUntrustedCandidates(t *testing.T) {
	if got := bestTrustedAutoLabel("E2.3.1.133", "LysoPL2"); got != "" {
		t.Fatalf("bestTrustedAutoLabel()=%q want empty", got)
	}
}

func TestPrepareBlastExportItemAutoIdentifiesFallbackLabel(t *testing.T) {
	w := &BlastWizard{}
	item := blastQueryItem{QuerySource: &model.QuerySequenceSource{
		GeneID:       "AT3G10340",
		TranscriptID: "AT3G10340.1",
		ProteinID:    "PAC:19660032",
	}}
	got, err := w.prepareBlastExportItem(item, false)
	if err != nil {
		t.Fatalf("prepareBlastExportItem returned error: %v", err)
	}
	if got.LabelName != "PAC:19660032" {
		t.Fatalf("unexpected label: %q", got.LabelName)
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
		"AT2G30490.1": {sequence: "MPEPTIDE"},
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
	if item.QuerySource.TranscriptID != rows[0].TranscriptID || item.QuerySource.GeneID != rows[0].GeneIdentifier {
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
		"Sp9509d006g002010_T001": {sequence: "MAAA"},
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

func TestKeywordRowsSearchTypeFallsBackToClassifiedInputTypeWhenRowsEmpty(t *testing.T) {
	if got := keywordRowsSearchType(nil, "F5H1", false); got == "" {
		t.Fatal("empty keyword rows should still produce a classified search type")
	}
}

func TestEnrichKeywordLabelsFromPhytozomeKeepsExistingLemnaLabels(t *testing.T) {
	w := &BlastWizard{
		source: lemna.NewClient(nil),
		speciesCandidatesCache: map[string][]model.SpeciesCandidate{
			"phytozome": {
				{SearchAlias: "Spirodela polyrhiza v2", JBrowseName: "Spolyrhiza_v2"},
			},
		},
	}
	groups := []model.KeywordSearchGroup{{
		SearchTerm: "Sp9509d020g000340_T001",
		Rows: []model.KeywordResultRow{{
			LabelName:    "C4H",
			ProteinID:    "Sp9509d020g000340_T001",
			TranscriptID: "Sp9509d020g000340_T001",
		}},
	}}

	got := w.enrichKeywordLabelsFromPhytozome(context.Background(), model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0"}, groups)
	if got[0].LabelName != "C4H" || got[0].Rows[0].LabelName != "C4H" {
		t.Fatalf("existing lemna label should be preserved: %#v", got)
	}
}

func TestEnrichKeywordLabelsFromSourceDeduplicatesPhytozomeLookups(t *testing.T) {
	lookupSource := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			candidates: []model.SpeciesCandidate{
				{SearchAlias: "Spirodela polyrhiza v2", JBrowseName: "Spolyrhiza_v2", GenomeLabel: "Spirodela polyrhiza v2"},
			},
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT2G30490.1": {{LabelName: "C4H", Aliases: "C4H; CYP73A5"}},
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

	got := w.enrichKeywordLabelsFromSource(context.Background(), model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0"}, groups, lookupSource)
	if got[0].LabelName == "" || got[0].LabelName != got[1].LabelName {
		t.Fatalf("expected deduplicated lookup to populate both groups: %#v", got)
	}
	if got[0].Rows[0].LabelName == "" || got[0].Rows[0].LabelName != got[1].Rows[0].LabelName {
		t.Fatalf("expected deduplicated lookup to populate both rows: %#v", got)
	}
	lookupSource.mu.Lock()
	defer lookupSource.mu.Unlock()
	if lookupSource.fetchCount["AT2G30490.1"] != 1 {
		t.Fatalf("phytozome lookup count = %d, want 1", lookupSource.fetchCount["AT2G30490.1"])
	}
}

func TestEnrichKeywordLabelsFromSourceReusesWizardLookupCacheAcrossRuns(t *testing.T) {
	lookupSource := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			candidates: []model.SpeciesCandidate{
				{SearchAlias: "Spirodela polyrhiza v2", JBrowseName: "Spolyrhiza_v2", GenomeLabel: "Spirodela polyrhiza v2"},
			},
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT2G30490.1": {{LabelName: "C4H", Aliases: "C4H; CYP73A5"}},
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
		keywordLabelLookupCache: make(map[string]string),
	}
	groups := []model.KeywordSearchGroup{
		{
			SearchTerm: "row-1",
			Rows: []model.KeywordResultRow{{
				ProteinID: "AT2G30490.1",
				Aliases:   "candidate alias phrase",
			}},
		},
	}

	first := w.enrichKeywordLabelsFromSource(context.Background(), model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0"}, groups, lookupSource)
	second := w.enrichKeywordLabelsFromSource(context.Background(), model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0"}, groups, lookupSource)
	if strings.TrimSpace(first[0].LabelName) == "" || strings.TrimSpace(second[0].LabelName) == "" {
		t.Fatalf("expected cached lookup to populate labels across runs: first=%#v second=%#v", first, second)
	}
	if first[0].LabelName != second[0].LabelName || first[0].Rows[0].LabelName != second[0].Rows[0].LabelName {
		t.Fatalf("expected cached lookup to populate labels across runs: first=%#v second=%#v", first, second)
	}
	lookupSource.mu.Lock()
	defer lookupSource.mu.Unlock()
	if lookupSource.fetchCount["AT2G30490.1"] != 1 {
		t.Fatalf("phytozome lookup count across runs = %d, want 1", lookupSource.fetchCount["AT2G30490.1"])
	}
}

func TestEnrichKeywordLabelsFromSourceSkipsFallbackWhenLocalAliasesAlreadySuffice(t *testing.T) {
	lookupSource := &countingKeywordMapSource{
		keywordMapSource: keywordMapSource{
			name: "phytozome",
			candidates: []model.SpeciesCandidate{
				{SearchAlias: "Spirodela polyrhiza v2", JBrowseName: "Spolyrhiza_v2", GenomeLabel: "Spirodela polyrhiza v2"},
			},
			rowsByKeyword: map[string][]model.KeywordResultRow{
				"AT2G30490.1": {{LabelName: "C4H", Aliases: "C4H; CYP73A5"}},
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

	got := w.enrichKeywordLabelsFromSource(context.Background(), model.SpeciesCandidate{GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0"}, groups, lookupSource)
	if strings.TrimSpace(got[0].LabelName) == "" {
		t.Fatalf("expected local alias-derived label without fallback lookup: %#v", got)
	}
	lookupSource.mu.Lock()
	defer lookupSource.mu.Unlock()
	if lookupSource.fetchCount["AT2G30490.1"] != 0 {
		t.Fatalf("phytozome fallback lookup count = %d, want 0", lookupSource.fetchCount["AT2G30490.1"])
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
	if source.GeneID != "AT5G44030" {
		t.Fatalf("unexpected gene id: %q", source.GeneID)
	}
	if source.TranscriptID != "AT5G44030.1" {
		t.Fatalf("unexpected transcript id: %q", source.TranscriptID)
	}
	if source.ProteinID != "AT5G44030.1" {
		t.Fatalf("unexpected protein id: %q", source.ProteinID)
	}
	if source.OrganismShort != "A.thaliana" {
		t.Fatalf("unexpected organism short: %q", source.OrganismShort)
	}
	if source.Annotation != "TAIR10" {
		t.Fatalf("unexpected annotation: %q", source.Annotation)
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
	if got := items[0].LabelName; got != "VND6" {
		t.Fatalf("unexpected first label: %q", got)
	}
	if got := items[1].LabelName; got != "VND7" {
		t.Fatalf("unexpected second label: %q", got)
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
	if got := items[0].QuerySource.GeneID; got != "AT5G62380" {
		t.Fatalf("unexpected first gene id: %q", got)
	}
	if got := items[1].QuerySource.GeneID; got != "AT1G71930" {
		t.Fatalf("unexpected second gene id: %q", got)
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
	if got := items[0].LabelName; got != "VND6" {
		t.Fatalf("unexpected first label: %q", got)
	}
	if got := items[1].LabelName; got != "VND7" {
		t.Fatalf("unexpected second label: %q", got)
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
	if got := items[0].LabelName; got != "VND6" {
		t.Fatalf("unexpected first label: %q", got)
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
	if items[2].QuerySource == nil || items[2].QuerySource.ProteinID != "plain_header_no_label" {
		t.Fatalf("expected primary FASTA header ID, got %#v", items[2].QuerySource)
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
	if source.GeneID != "AT5G44030" {
		t.Fatalf("unexpected gene id: %q", source.GeneID)
	}
	if source.TranscriptID != "AT5G44030.1" {
		t.Fatalf("unexpected transcript id: %q", source.TranscriptID)
	}
	if source.Sequence != "MEPNTMASFDDEHRHSSFSAKIC" {
		t.Fatalf("unexpected sequence: %q", source.Sequence)
	}
	if source.LabelName != "CESA4" {
		t.Fatalf("unexpected label name: %q", source.LabelName)
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

type fakeSource struct {
	query          *model.QuerySequenceSource
	keywordRows    []model.KeywordResultRow
	sequences      map[string]string
	sequenceErrors map[string]error
	fetchCount     map[string]int
	err            error
}

var fakeSourceFetchMu sync.Mutex

func (f fakeSource) Name() string { return "fake" }
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
func (f fakeSource) FetchProteinSequence(ctx context.Context, targetID int, sequenceID string) (string, error) {
	if f.fetchCount != nil {
		fakeSourceFetchMu.Lock()
		f.fetchCount[sequenceID]++
		fakeSourceFetchMu.Unlock()
	}
	if err, ok := f.sequenceErrors[sequenceID]; ok {
		return "", err
	}
	if sequence, ok := f.sequences[sequenceID]; ok {
		return sequence, nil
	}
	return "", fmt.Errorf("no protein sequence for transcript id %s", sequenceID)
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
func (f keywordMapSource) FetchProteinSequence(ctx context.Context, targetID int, sequenceID string) (string, error) {
	return "", nil
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
func (f *countingKeywordMapSource) FetchProteinSequence(ctx context.Context, targetID int, sequenceID string) (string, error) {
	return f.keywordMapSource.FetchProteinSequence(ctx, targetID, sequenceID)
}
func (f *countingKeywordMapSource) FetchGeneQuerySequence(ctx context.Context, species model.SpeciesCandidate, reportType string, identifier string) (*model.QuerySequenceSource, error) {
	return f.keywordMapSource.FetchGeneQuerySequence(ctx, species, reportType, identifier)
}

func TestFetchProteinSequenceRecordsSkipsMissingSequencesAndCachesMisses(t *testing.T) {
	fetchCount := map[string]int{}
	w := &BlastWizard{
		source:               fakeSource{sequences: map[string]string{"ok": "MPEPTIDE"}, fetchCount: fetchCount},
		proteinSequenceCache: make(map[string]string),
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

func TestFetchProteinSequenceRecordsReturnsNonMissingErrors(t *testing.T) {
	w := &BlastWizard{
		source: fakeSource{
			sequences:      map[string]string{"ok": "MPEPTIDE"},
			sequenceErrors: map[string]error{"net": fmt.Errorf("fetch protein sequence: unexpected status 500")},
		},
		proteinSequenceCache: make(map[string]string),
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

func TestLocalBlastBatchWorkerBudgetDoesNotOversubscribeCPU(t *testing.T) {
	previous := runtime.GOMAXPROCS(8)
	defer runtime.GOMAXPROCS(previous)

	request := model.BlastRequest{Program: "local:BLASTP"}
	workers := batchBlastWorkerCount(65, request)
	if workers <= 0 {
		t.Fatalf("workers = %d, want positive", workers)
	}
	threads := localBlastThreadsPerWorker(workers)
	if threads <= 0 {
		t.Fatalf("threads = %d, want positive", threads)
	}
	if workers*threads > 8 {
		t.Fatalf("local BLAST oversubscribed CPU budget: workers=%d threads=%d cpu=8", workers, threads)
	}

	networkWorkers := batchBlastWorkerCount(65, model.BlastRequest{Program: "BLASTP"})
	if networkWorkers <= workers {
		t.Fatalf("remote BLAST workers = %d, want more than local budget %d", networkWorkers, workers)
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

func TestKeywordRowsToBlastItemsPreservesKeywordMetadata(t *testing.T) {
	rows := []model.KeywordResultRow{{
		SourceDatabase:      "lemna",
		LabelName:           "Os4CL1",
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
		"Sp9509d011g001470_T001": {sequence: "MPEPTIDE"},
	})
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].QuerySource == nil {
		t.Fatal("expected query source metadata")
	}
	source := items[0].QuerySource
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

func TestParseBlastLoadCommand(t *testing.T) {
	filename, ok := parseBlastLoadCommand(`load "queries.txt"`)
	if !ok {
		t.Fatalf("expected load command to parse")
	}
	if filename != "queries.txt" {
		t.Fatalf("unexpected filename: %q", filename)
	}
}

func TestAvailableBlastProgramsIncludeServerAndLocalCapabilities(t *testing.T) {
	serverOnly := lemna.BlastCapability{
		HasServerNucleotideDB: true,
		BlastNDBID:            12,
		HasServerProteinDB:    true,
		ProteinDBID:           34,
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
}

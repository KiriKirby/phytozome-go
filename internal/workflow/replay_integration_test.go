// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package workflow

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/phytozome"
	"github.com/KiriKirby/phytozome-go/internal/prompt"
	"github.com/xuri/excelize/v2"
)

func TestReplaySpirodelaBlastExportRawAndPDF(t *testing.T) {
	if os.Getenv("PHYTO_REPLAY_EXPORT") == "" {
		t.Skip("set PHYTO_REPLAY_EXPORT=1 to run BLAST export replay")
	}
	inputRoot := os.Getenv("PHYTO_REPLAY_INPUT_DIR")
	if strings.TrimSpace(inputRoot) == "" {
		inputRoot = `C:\Users\wangsychn\Desktop\phytozome-go_windows_amd64\output`
	}
	outputRoot := os.Getenv("PHYTO_REPLAY_EXPORT_DIR")
	if strings.TrimSpace(outputRoot) == "" {
		outputRoot = filepath.Join(os.TempDir(), "phytozome-go-replay-export")
	}
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		t.Fatalf("create replay output dir: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(inputRoot, "*.txt"))
	if err != nil {
		t.Fatalf("list replay txt files: %v", err)
	}
	sort.Strings(files)
	if limit := replayEnvInt("PHYTO_REPLAY_FILE_LIMIT"); limit > 0 && limit < len(files) {
		files = files[:limit]
	}
	if len(files) == 0 {
		t.Fatalf("no txt files found in %s", inputRoot)
	}

	cases := []struct {
		name     string
		source   string
		selected model.SpeciesCandidate
		request  model.BlastRequest
	}{
		{
			name:   "lemna-spirodela-9509",
			source: "lemna",
			selected: model.SpeciesCandidate{
				GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
				SearchAlias: "Spirodela polyrhiza",
				JBrowseName: "Sp_polyrhiza_9509",
				ProteomeID:  18,
				IsOfficial:  true,
				CommonName:  "giant duckweed",
			},
			request: replayBlastPRequest(model.SpeciesCandidate{
				GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
				SearchAlias: "Spirodela polyrhiza",
				JBrowseName: "Sp_polyrhiza_9509",
				ProteomeID:  18,
				IsOfficial:  true,
				CommonName:  "giant duckweed",
			}, "local:BLASTP"),
		},
		{
			name:   "phytozome-spirodela-v2",
			source: "phytozome",
			selected: model.SpeciesCandidate{
				ProteomeID:  290,
				JBrowseName: "S_polyrhiza_v2",
				GenomeLabel: "Spirodela polyrhiza v2",
				SearchAlias: "Spirodela polyrhiza v2",
				CommonName:  "greater duckweed",
			},
			request: replayBlastPRequest(model.SpeciesCandidate{
				ProteomeID:  290,
				JBrowseName: "S_polyrhiza_v2",
				GenomeLabel: "Spirodela polyrhiza v2",
				SearchAlias: "Spirodela polyrhiza v2",
				CommonName:  "greater duckweed",
			}, "BLASTP"),
		},
	}
	if only := strings.ToLower(strings.TrimSpace(os.Getenv("PHYTO_REPLAY_SOURCE"))); only != "" {
		filtered := cases[:0]
		for _, tc := range cases {
			if strings.Contains(strings.ToLower(tc.name), only) || strings.EqualFold(tc.source, only) {
				filtered = append(filtered, tc)
			}
		}
		cases = filtered
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range files {
				t.Run(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), func(t *testing.T) {
					replayOneBlastExport(t, tc.source, tc.selected, tc.request, path, filepath.Join(outputRoot, tc.name))
				})
			}
		})
	}
}

func replayOneBlastExport(t *testing.T, sourceName string, selected model.SpeciesCandidate, request model.BlastRequest, inputPath string, outputRoot string) {
	t.Helper()
	replayEnsureBlastPlusPath(t)
	items, err := loadReplayFastaItems(inputPath)
	if err != nil {
		t.Fatalf("load replay FASTA %s: %v", inputPath, err)
	}
	if len(items) == 0 {
		t.Fatalf("no FASTA items in %s", inputPath)
	}
	ctx, cancel := context.WithTimeout(context.Background(), replayEnvDuration("PHYTO_REPLAY_TIMEOUT_MINUTES", 90*time.Minute))
	defer cancel()
	w := NewBlastWizard(os.Stdout)
	switch sourceName {
	case "lemna":
		w.source = lemna.NewClient(w.httpClient)
	case "phytozome":
		w.source = phytozome.NewClient(w.httpClient)
	default:
		t.Fatalf("unsupported replay source %q", sourceName)
	}
	w.suppressTaskModals = true
	references := externalReferenceConfig{
		UseUniProt:       true,
		UseInterPro:      true,
		InterProSettings: model.DefaultInterProConservedRegionSettings(),
	}
	start := time.Now()
	runs, err := w.executeConfiguredBlastBatchRuns(ctx, items, request, references)
	if err != nil {
		t.Fatalf("run BLAST batch: %v", err)
	}
	blastDuration := time.Since(start)
	filterStart := time.Now()
	filterSettings := model.DefaultBlastFilterSettings()
	rowsByRun, rowNumbersByRun, filterFlagsByRun, selectedByRun := replayDefaultFilteredRows(runs, filterSettings)
	filterDuration := time.Since(filterStart)

	outputDir := filepath.Join(outputRoot, sanitizeExportName(strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))))
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("create export output dir: %v", err)
	}
	settings := exportSettings{
		BaseName:      sanitizeExportName(strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))) + "_all",
		OutputDir:     outputDir,
		WriteReport:   true,
		WriteText:     true,
		WriteExcel:    true,
		WriteRawExcel: true,
	}
	exportStart := time.Now()
	batchResult, err := w.exportAllBlastRunsWithProgress(ctx, selected, items, runs, rowsByRun, rowNumbersByRun, filterFlagsByRun, selectedByRun, request, settings)
	if err != nil {
		t.Fatalf("export all BLAST runs: %v", err)
	}
	exportDuration := time.Since(exportStart)
	reportStart := time.Now()
	reportPath, err := w.renderBlastBatchReport(ctx, selected, items, items, cloneBlastQueryRuns(runs), batchResult.Files, rowsByRun, rowNumbersByRun, filterFlagsByRun, selectedByRun, outputDir, settings, request, filterSettings, true, false)
	if err != nil {
		t.Fatalf("render BLAST batch report: %v", err)
	}
	reportDuration := time.Since(reportStart)
	if strings.TrimSpace(reportPath) == "" {
		t.Fatal("report path is empty")
	}
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("report was not written: %v", err)
	}
	rawFiles := 0
	for _, file := range batchResult.Files {
		if strings.TrimSpace(file.RawExcelPath) != "" {
			rawFiles++
		}
	}
	if rawFiles == 0 {
		t.Fatal("no raw Excel files were written")
	}
	t.Logf("replay source=%s file=%s queries=%d runs=%d selected=%d raw_files=%d blast=%s filter=%s export=%s report=%s output=%s",
		sourceName,
		filepath.Base(inputPath),
		len(items),
		len(runs),
		replayCountSelectedRows(rowsByRun),
		rawFiles,
		blastDuration.Round(time.Millisecond),
		filterDuration.Round(time.Millisecond),
		exportDuration.Round(time.Millisecond),
		reportDuration.Round(time.Millisecond),
		outputDir,
	)
}

func replayBlastPRequest(selected model.SpeciesCandidate, program string) model.BlastRequest {
	return model.BlastRequest{
		Species:          selected,
		SequenceKind:     model.SequenceProtein,
		TargetType:       "proteome",
		Program:          program,
		EValue:           "1e-30",
		ComparisonMatrix: "BLOSUM62",
		WordLength:       "default",
		AlignmentsToShow: 100,
		AllowGaps:        true,
		FilterQuery:      true,
	}
}

func replayDefaultFilteredRows(runs []blastQueryRun, settings model.BlastFilterSettings) ([][]model.BlastResultRow, [][]int, [][]bool, [][]bool) {
	rowsByRun := make([][]model.BlastResultRow, len(runs))
	rowNumbersByRun := make([][]int, len(runs))
	filterFlagsByRun := make([][]bool, len(runs))
	selectedByRun := make([][]bool, len(runs))
	for runIndex, run := range runs {
		suggestion := prompt.DefaultBlastFilterSuggestion(prompt.BlastFilterRequest{
			Rows:     run.Results.Rows,
			Settings: settings,
		})
		selectedByRun[runIndex] = append([]bool(nil), suggestion.Selected...)
		filterFlagsByRun[runIndex] = append([]bool(nil), suggestion.Flags...)
		for i, selected := range suggestion.Selected {
			if selected && i < len(run.Results.Rows) {
				rowsByRun[runIndex] = append(rowsByRun[runIndex], run.Results.Rows[i])
				rowNumbersByRun[runIndex] = append(rowNumbersByRun[runIndex], i+1)
			}
		}
	}
	return rowsByRun, rowNumbersByRun, filterFlagsByRun, selectedByRun
}

func replayCountSelectedRows(rowsByRun [][]model.BlastResultRow) int {
	total := 0
	for _, rows := range rowsByRun {
		total += len(rows)
	}
	return total
}

func replayEnvInt(name string) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func replayEnvDuration(name string, fallback time.Duration) time.Duration {
	value := replayEnvInt(name)
	if value <= 0 {
		return fallback
	}
	return time.Duration(value) * time.Minute
}

func replayEnsureBlastPlusPath(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(".", "bin", "blastplus")); err == nil {
		replayPrependFirstBlastBin(filepath.Join(".", "bin", "blastplus"))
	}
	replayPrependFirstBlastBin(`C:\Users\wangsychn\Desktop\phytozome-go_windows_amd64\blastplus`)
}

func replayPrependFirstBlastBin(root string) {
	root = strings.TrimSpace(root)
	if root == "" {
		return
	}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry == nil || !entry.IsDir() {
			return nil
		}
		blastp := filepath.Join(path, "blastp.exe")
		makeblastdb := filepath.Join(path, "makeblastdb.exe")
		if _, err := os.Stat(blastp); err == nil {
			if _, err := os.Stat(makeblastdb); err == nil {
				current := os.Getenv("PATH")
				_ = os.Setenv("PATH", path+string(os.PathListSeparator)+current)
				return filepath.SkipAll
			}
		}
		return nil
	})
}

func TestReplayMonolignolTable17_LemnaLocalBlastP(t *testing.T) {
	if os.Getenv("PHYTO_REPLAY") == "" {
		t.Skip("set PHYTO_REPLAY=1 to run integration replay")
	}

	txtPath := `C:\Users\wangsychn\Desktop\新建文件夹\Monolignol Biosynthesis.txt`
	xlsxPath := `C:\Users\wangsychn\Desktop\新建文件夹\Monolignol Biosynthesis.xlsx`

	manualItems, err := loadReplayFastaItems(txtPath)
	if err != nil {
		t.Fatalf("load manual replay FASTA: %v", err)
	}
	if len(manualItems) == 0 {
		t.Fatal("no manual replay items loaded")
	}

	w := NewBlastWizard(os.Stdout)
	lc := lemna.NewClient(nil)
	w.source = lc
	selected := model.SpeciesCandidate{
		GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
		SearchAlias: "Spirodela polyrhiza",
		JBrowseName: "Sp_polyrhiza_9509",
		ProteomeID:  18,
		IsOfficial:  true,
		CommonName:  "giant duckweed",
	}

	arabidopsis := model.SpeciesCandidate{
		ProteomeID:  167,
		JBrowseName: "Athaliana_TAIR10",
		GenomeLabel: "Arabidopsis thaliana TAIR10",
		SearchAlias: "Arabidopsis thaliana",
	}

	req := model.BlastRequest{
		Species:          selected,
		SequenceKind:     model.SequenceProtein,
		TargetType:       "proteome",
		Program:          "local:BLASTP",
		EValue:           "1e-30",
		ComparisonMatrix: "BLOSUM62",
		WordLength:       "default",
		AlignmentsToShow: 100,
		AllowGaps:        true,
		FilterQuery:      true,
	}
	references := externalReferenceConfig{
		UseUniProt:       true,
		UseInterPro:      true,
		InterProSettings: model.DefaultInterProConservedRegionSettings(),
	}
	filterSettings := model.DefaultBlastFilterSettings()
	targets := map[string]int{
		"4CL":    9,
		"C3H":    1,
		"C4H":    3,
		"CAD":    4,
		"CCOAMT": 1,
		"CCR":    21,
		"COMT":   5,
		"F5H":    3,
		"HCT":    20,
		"PAL":    3,
	}

	manualResult, err := runReplayScenario(context.Background(), w, selected, arabidopsis, req, references, filterSettings, manualItems, false)
	if err != nil {
		t.Fatalf("manual replay: %v", err)
	}
	t.Log("Manual labels from TXT")
	logReplayResult(t, manualResult, targets)

	autoItems, err := loadReplayItemsFromExcel(context.Background(), xlsxPath, arabidopsis, w.httpClient)
	if err != nil {
		t.Fatalf("load auto-label replay items: %v", err)
	}
	if len(autoItems) == 0 {
		t.Fatal("no auto-label replay items loaded")
	}
	labeledAutoItems := cloneBlastQueryItems(autoItems)
	phytozomeSource := phytozome.NewClient(w.httpClient)
	for i := range labeledAutoItems {
		labeledAutoItems[i].LabelName = w.autoIdentifyBlastLabel(context.Background(), phytozomeSource, arabidopsis, labeledAutoItems[i])
	}
	logReplayLabels(t, "Auto-label assignments", labeledAutoItems, model.DefaultFamilyBlastSettings())
	autoResult, err := runReplayScenario(context.Background(), w, selected, arabidopsis, req, references, filterSettings, autoItems, true)
	if err != nil {
		t.Fatalf("auto-label replay: %v", err)
	}
	t.Log("Auto labels from Excel protein IDs / Phytozome metadata")
	logReplayResult(t, autoResult, targets)
}

type replayScenarioResult struct {
	RawByFamily    map[string]int
	MergedByFamily map[string]int
	KeptByFamily   map[string]int
}

func runReplayScenario(ctx context.Context, w *BlastWizard, selected model.SpeciesCandidate, labelSpecies model.SpeciesCandidate, req model.BlastRequest, references externalReferenceConfig, filterSettings model.BlastFilterSettings, items []blastQueryItem, autoLabel bool) (replayScenarioResult, error) {
	items = cloneBlastQueryItems(items)
	if autoLabel {
		labeled, err := w.autoIdentifyBlastLabelsWithProgress(ctx, labelSpecies, items)
		if err != nil {
			return replayScenarioResult{}, err
		}
		items = labeled
	}
	runs, err := w.executeConfiguredBlastBatchRuns(ctx, items, req, references)
	if err != nil {
		return replayScenarioResult{}, err
	}
	result := replayScenarioResult{
		RawByFamily:    map[string]int{},
		MergedByFamily: map[string]int{},
		KeptByFamily:   map[string]int{},
	}
	for _, run := range runs {
		family := replayRunFamily(run, model.DefaultFamilyBlastSettings())
		result.RawByFamily[family] += len(run.Results.Rows)
	}
	familySettings := model.DefaultFamilyBlastSettings()
	plan := &familyBlastPlan{
		Settings: familySettings,
		Groups:   detectFamilyBlastGroups(items, familySettings),
	}
	_, mergedRuns := applyFamilyBlastPlan(items, runs, plan)
	for _, run := range mergedRuns {
		family := replayRunFamily(run, familySettings)
		result.MergedByFamily[family] += len(run.Results.Rows)
		suggestion := prompt.DefaultBlastFilterSuggestion(prompt.BlastFilterRequest{
			Rows:     run.Results.Rows,
			Settings: filterSettings,
		})
		for _, keep := range suggestion.Selected {
			if keep {
				result.KeptByFamily[family]++
			}
		}
	}
	return result, nil
}

func TestReplayMonolignolVariants_LemnaLocalBlastP(t *testing.T) {
	if os.Getenv("PHYTO_REPLAY") == "" {
		t.Skip("set PHYTO_REPLAY=1 to run integration replay")
	}

	txtPath := `C:\Users\wangsychn\Desktop\新建文件夹\Monolignol Biosynthesis.txt`
	manualItems, err := loadReplayFastaItems(txtPath)
	if err != nil {
		t.Fatalf("load manual replay FASTA: %v", err)
	}
	w := NewBlastWizard(os.Stdout)
	w.source = lemna.NewClient(nil)
	selected := model.SpeciesCandidate{
		GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
		SearchAlias: "Spirodela polyrhiza",
		JBrowseName: "Sp_polyrhiza_9509",
		ProteomeID:  18,
		IsOfficial:  true,
		CommonName:  "giant duckweed",
	}
	arabidopsis := model.SpeciesCandidate{
		ProteomeID:  167,
		JBrowseName: "Athaliana_TAIR10",
		GenomeLabel: "Arabidopsis thaliana TAIR10",
		SearchAlias: "Arabidopsis thaliana",
	}
	req := model.BlastRequest{
		Species:          selected,
		SequenceKind:     model.SequenceProtein,
		TargetType:       "proteome",
		Program:          "local:BLASTP",
		EValue:           "1e-30",
		ComparisonMatrix: "BLOSUM62",
		WordLength:       "default",
		AlignmentsToShow: 100,
		AllowGaps:        true,
		FilterQuery:      true,
	}
	references := externalReferenceConfig{
		UseUniProt:       true,
		UseInterPro:      true,
		InterProSettings: model.DefaultInterProConservedRegionSettings(),
	}
	baseFamily := model.DefaultFamilyBlastSettings()
	baseFilter := model.DefaultBlastFilterSettings()

	cases := []struct {
		name   string
		family model.FamilyBlastSettings
		filter model.BlastFilterSettings
	}{
		{name: "baseline", family: baseFamily, filter: baseFilter},
		func() struct {
			name   string
			family model.FamilyBlastSettings
			filter model.BlastFilterSettings
		} {
			f := baseFilter
			f.RequireInterProConservedRegion = false
			f.RejectInterProMissing = false
			f.RejectInterProUncertain = false
			return struct {
				name   string
				family model.FamilyBlastSettings
				filter model.BlastFilterSettings
			}{name: "no-interpro-hard-reject", family: baseFamily, filter: f}
		}(),
		func() struct {
			name   string
			family model.FamilyBlastSettings
			filter model.BlastFilterSettings
		} {
			f := baseFilter
			f.RequireTargetCanonicalLengthRatio = false
			return struct {
				name   string
				family model.FamilyBlastSettings
				filter model.BlastFilterSettings
			}{name: "allow-missing-canonical-ratio", family: baseFamily, filter: f}
		}(),
		func() struct {
			name   string
			family model.FamilyBlastSettings
			filter model.BlastFilterSettings
		} {
			f := baseFilter
			f.RequireInterProConservedRegion = false
			f.RejectInterProMissing = false
			f.RejectInterProUncertain = false
			f.RequireTargetCanonicalLengthRatio = false
			return struct {
				name   string
				family model.FamilyBlastSettings
				filter model.BlastFilterSettings
			}{name: "relaxed-interpro-and-ratio", family: baseFamily, filter: f}
		}(),
		func() struct {
			name   string
			family model.FamilyBlastSettings
			filter model.BlastFilterSettings
		} {
			fam := baseFamily
			fam.RankingTieBreakerOrder = "evalue,identity,coverage,reference,bitscore"
			return struct {
				name   string
				family model.FamilyBlastSettings
				filter model.BlastFilterSettings
			}{name: "family-evalue-first", family: fam, filter: baseFilter}
		}(),
		func() struct {
			name   string
			family model.FamilyBlastSettings
			filter model.BlastFilterSettings
		} {
			f := baseFilter
			f.InterProDomainMode = "any_domain"
			f.RequireInterProConservedRegion = false
			f.RejectInterProMissing = false
			f.RejectInterProUncertain = false
			return struct {
				name   string
				family model.FamilyBlastSettings
				filter model.BlastFilterSettings
			}{name: "interpro-any-domain", family: baseFamily, filter: f}
		}(),
		func() struct {
			name   string
			family model.FamilyBlastSettings
			filter model.BlastFilterSettings
		} {
			f := baseFilter
			f.InterProDomainMode = "any_domain"
			f.RequireInterProConservedRegion = false
			f.RejectInterProMissing = false
			f.RejectInterProUncertain = false
			f.UseTargetQueryLengthRatio = true
			f.RequireTargetQueryLengthRatio = true
			f.MinTargetQueryLengthPercent = 70
			f.MaxTargetQueryLengthPercent = 150
			f.RequireTargetCanonicalLengthRatio = false
			return struct {
				name   string
				family model.FamilyBlastSettings
				filter model.BlastFilterSettings
			}{name: "any-domain-plus-target-query-ratio", family: baseFamily, filter: f}
		}(),
		func() struct {
			name   string
			family model.FamilyBlastSettings
			filter model.BlastFilterSettings
		} {
			f := baseFilter
			f.InterProDomainMode = "any_domain"
			f.RequireInterProConservedRegion = false
			f.RejectInterProMissing = false
			f.RejectInterProUncertain = false
			f.MinIdentityPercent = 30
			f.MinAlignQueryCoveragePercent = 50
			f.MaxEValue = 1e-30
			return struct {
				name   string
				family model.FamilyBlastSettings
				filter model.BlastFilterSettings
			}{name: "any-domain-plus-paper-metrics", family: baseFamily, filter: f}
		}(),
		func() struct {
			name   string
			family model.FamilyBlastSettings
			filter model.BlastFilterSettings
		} {
			f := baseFilter
			f.RequireFamilySemanticAgreement = false
			return struct {
				name   string
				family model.FamilyBlastSettings
				filter model.BlastFilterSettings
			}{name: "semantic-soft-only", family: baseFamily, filter: f}
		}(),
		func() struct {
			name   string
			family model.FamilyBlastSettings
			filter model.BlastFilterSettings
		} {
			f := baseFilter
			f.RequireFamilySemanticAgreement = false
			f.StrongBlastFallbackMinIdentityPercent = 30
			f.StrongBlastFallbackMaxEValue = 1e-50
			return struct {
				name   string
				family model.FamilyBlastSettings
				filter model.BlastFilterSettings
			}{name: "semantic-soft-plus-relaxed-fallback-30-1e50", family: baseFamily, filter: f}
		}(),
		func() struct {
			name   string
			family model.FamilyBlastSettings
			filter model.BlastFilterSettings
		} {
			f := baseFilter
			f.RequireFamilySemanticAgreement = false
			f.StrongBlastFallbackMinIdentityPercent = 28
			f.StrongBlastFallbackMaxEValue = 1e-45
			return struct {
				name   string
				family model.FamilyBlastSettings
				filter model.BlastFilterSettings
			}{name: "semantic-soft-plus-relaxed-fallback-28-1e45", family: baseFamily, filter: f}
		}(),
		func() struct {
			name   string
			family model.FamilyBlastSettings
			filter model.BlastFilterSettings
		} {
			f := baseFilter
			f.RequireFamilySemanticAgreement = false
			f.StrongBlastFallbackMinIdentityPercent = 30
			f.StrongBlastFallbackMaxEValue = 1e-50
			f.UseTargetQueryLengthRatio = true
			f.RequireTargetQueryLengthRatio = true
			f.MinTargetQueryLengthPercent = 80
			f.MaxTargetQueryLengthPercent = 120
			return struct {
				name   string
				family model.FamilyBlastSettings
				filter model.BlastFilterSettings
			}{name: "semantic-soft-relaxed-fallback-plus-target-query", family: baseFamily, filter: f}
		}(),
	}

	targets := map[string]int{"CCR": 21, "HCT": 20, "C3H": 1, "F5H": 3, "C4H": 3, "CAD": 4, "PAL": 3, "4CL": 9, "COMT": 5, "CCOAMT": 1}
	for _, tc := range cases {
		result, err := runReplayScenarioWithSettings(context.Background(), w, selected, arabidopsis, req, references, tc.filter, tc.family, manualItems, false)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		t.Logf("variant=%s", tc.name)
		logReplayResult(t, result, targets)
	}

	debugRuns, err := replayMergedRuns(context.Background(), w, selected, arabidopsis, req, references, baseFamily, manualItems, false)
	if err != nil {
		t.Fatalf("debug merged runs: %v", err)
	}
	for _, family := range []string{"CCR", "HCT", "C3H", "F5H"} {
		logReplayFamilyDiagnostics(t, family, debugRuns, baseFilter, baseFamily)
	}
}

func TestReplayCelluloseTable16_LemnaLocalBlastP(t *testing.T) {
	if os.Getenv("PHYTO_REPLAY") == "" {
		t.Skip("set PHYTO_REPLAY=1 to run integration replay")
	}
	txtPath := `C:\Users\wangsychn\Desktop\新建文件夹\Cellulose.txt`
	xlsxPath := `C:\Users\wangsychn\Desktop\新建文件夹\Cellulose.xlsx`
	items, err := loadReplayFastaItems(txtPath)
	if err != nil {
		t.Fatalf("load cellulose replay FASTA: %v", err)
	}
	w := NewBlastWizard(os.Stdout)
	w.source = lemna.NewClient(nil)
	selected := model.SpeciesCandidate{
		GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
		SearchAlias: "Spirodela polyrhiza",
		JBrowseName: "Sp_polyrhiza_9509",
		ProteomeID:  18,
		IsOfficial:  true,
		CommonName:  "giant duckweed",
	}
	arabidopsis := model.SpeciesCandidate{
		ProteomeID:  167,
		JBrowseName: "Athaliana_TAIR10",
		GenomeLabel: "Arabidopsis thaliana TAIR10",
		SearchAlias: "Arabidopsis thaliana",
	}
	req := model.BlastRequest{
		Species:          selected,
		SequenceKind:     model.SequenceProtein,
		TargetType:       "proteome",
		Program:          "local:BLASTP",
		EValue:           "1e-30",
		ComparisonMatrix: "BLOSUM62",
		WordLength:       "default",
		AlignmentsToShow: 100,
		AllowGaps:        true,
		FilterQuery:      true,
	}
	references := externalReferenceConfig{
		UseUniProt:       true,
		UseInterPro:      true,
		InterProSettings: model.DefaultInterProConservedRegionSettings(),
	}
	result, err := runReplayScenario(context.Background(), w, selected, arabidopsis, req, references, model.DefaultBlastFilterSettings(), items, false)
	if err != nil {
		t.Fatalf("cellulose replay: %v", err)
	}
	t.Log("Cellulose manual labels from TXT")
	logReplayResult(t, result, map[string]int{"CESA": 10})

	autoItems, err := loadReplayItemsFromExcel(context.Background(), xlsxPath, arabidopsis, w.httpClient)
	if err != nil {
		t.Fatalf("load cellulose auto-label replay items: %v", err)
	}
	labeledAutoItems := cloneBlastQueryItems(autoItems)
	phytozomeSource := phytozome.NewClient(w.httpClient)
	for i := range labeledAutoItems {
		labeledAutoItems[i].LabelName = w.autoIdentifyBlastLabel(context.Background(), phytozomeSource, arabidopsis, labeledAutoItems[i])
	}
	logReplayLabels(t, "Cellulose auto-label assignments", labeledAutoItems, model.DefaultFamilyBlastSettings())
	autoResult, err := runReplayScenario(context.Background(), w, selected, arabidopsis, req, references, model.DefaultBlastFilterSettings(), autoItems, true)
	if err != nil {
		t.Fatalf("cellulose auto replay: %v", err)
	}
	t.Log("Cellulose auto labels from Excel protein IDs / Phytozome metadata")
	logReplayResult(t, autoResult, map[string]int{"CESA": 10})
}

func TestReplayHemicellulosesTable16_LemnaLocalBlastP(t *testing.T) {
	if os.Getenv("PHYTO_REPLAY") == "" {
		t.Skip("set PHYTO_REPLAY=1 to run integration replay")
	}
	txtPath := `C:\Users\wangsychn\Desktop\新建文件夹\Hemicelluloses.txt`
	xlsxPath := `C:\Users\wangsychn\Desktop\新建文件夹\Hemicelluloses.xlsx`
	items, err := loadReplayFastaItems(txtPath)
	if err != nil {
		t.Fatalf("load hemicelluloses replay FASTA: %v", err)
	}
	w := NewBlastWizard(os.Stdout)
	w.source = lemna.NewClient(nil)
	selected := model.SpeciesCandidate{
		GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
		SearchAlias: "Spirodela polyrhiza",
		JBrowseName: "Sp_polyrhiza_9509",
		ProteomeID:  18,
		IsOfficial:  true,
		CommonName:  "giant duckweed",
	}
	arabidopsis := model.SpeciesCandidate{
		ProteomeID:  167,
		JBrowseName: "Athaliana_TAIR10",
		GenomeLabel: "Arabidopsis thaliana TAIR10",
		SearchAlias: "Arabidopsis thaliana",
	}
	req := model.BlastRequest{
		Species:          selected,
		SequenceKind:     model.SequenceProtein,
		TargetType:       "proteome",
		Program:          "local:BLASTP",
		EValue:           "1e-30",
		ComparisonMatrix: "BLOSUM62",
		WordLength:       "default",
		AlignmentsToShow: 100,
		AllowGaps:        true,
		FilterQuery:      true,
	}
	references := externalReferenceConfig{
		UseUniProt:       true,
		UseInterPro:      true,
		InterProSettings: model.DefaultInterProConservedRegionSettings(),
	}
	result, err := runReplayScenario(context.Background(), w, selected, arabidopsis, req, references, model.DefaultBlastFilterSettings(), items, false)
	if err != nil {
		t.Fatalf("hemicelluloses replay: %v", err)
	}
	t.Log("Hemicelluloses manual labels from TXT")
	logReplayResult(t, result, map[string]int{"IRX": 21})

	autoItems, err := loadReplayItemsFromExcel(context.Background(), xlsxPath, arabidopsis, w.httpClient)
	if err != nil {
		t.Fatalf("load hemicelluloses auto-label replay items: %v", err)
	}
	labeledAutoItems := cloneBlastQueryItems(autoItems)
	phytozomeSource := phytozome.NewClient(w.httpClient)
	for i := range labeledAutoItems {
		labeledAutoItems[i].LabelName = w.autoIdentifyBlastLabel(context.Background(), phytozomeSource, arabidopsis, labeledAutoItems[i])
	}
	logReplayLabels(t, "Hemicelluloses auto-label assignments", labeledAutoItems, model.DefaultFamilyBlastSettings())
	autoResult, err := runReplayScenario(context.Background(), w, selected, arabidopsis, req, references, model.DefaultBlastFilterSettings(), autoItems, true)
	if err != nil {
		t.Fatalf("hemicelluloses auto replay: %v", err)
	}
	t.Log("Hemicelluloses auto labels from Excel protein IDs / Phytozome metadata")
	logReplayResult(t, autoResult, map[string]int{"IRX": 21})
}

func runReplayScenarioWithSettings(ctx context.Context, w *BlastWizard, selected model.SpeciesCandidate, labelSpecies model.SpeciesCandidate, req model.BlastRequest, references externalReferenceConfig, filterSettings model.BlastFilterSettings, familySettings model.FamilyBlastSettings, items []blastQueryItem, autoLabel bool) (replayScenarioResult, error) {
	items = cloneBlastQueryItems(items)
	if autoLabel {
		labeled, err := w.autoIdentifyBlastLabelsWithProgress(ctx, labelSpecies, items)
		if err != nil {
			return replayScenarioResult{}, err
		}
		items = labeled
	}
	runs, err := w.executeConfiguredBlastBatchRuns(ctx, items, req, references)
	if err != nil {
		return replayScenarioResult{}, err
	}
	result := replayScenarioResult{
		RawByFamily:    map[string]int{},
		MergedByFamily: map[string]int{},
		KeptByFamily:   map[string]int{},
	}
	for _, run := range runs {
		family := replayRunFamily(run, familySettings)
		result.RawByFamily[family] += len(run.Results.Rows)
	}
	plan := &familyBlastPlan{
		Settings: familySettings,
		Groups:   detectFamilyBlastGroups(items, familySettings),
	}
	_, mergedRuns := applyFamilyBlastPlan(items, runs, plan)
	for _, run := range mergedRuns {
		family := replayRunFamily(run, familySettings)
		result.MergedByFamily[family] += len(run.Results.Rows)
		suggestion := prompt.DefaultBlastFilterSuggestion(prompt.BlastFilterRequest{
			Rows:     run.Results.Rows,
			Settings: filterSettings,
		})
		for _, keep := range suggestion.Selected {
			if keep {
				result.KeptByFamily[family]++
			}
		}
	}
	return result, nil
}

func replayMergedRuns(ctx context.Context, w *BlastWizard, selected model.SpeciesCandidate, labelSpecies model.SpeciesCandidate, req model.BlastRequest, references externalReferenceConfig, familySettings model.FamilyBlastSettings, items []blastQueryItem, autoLabel bool) ([]blastQueryRun, error) {
	items = cloneBlastQueryItems(items)
	if autoLabel {
		labeled, err := w.autoIdentifyBlastLabelsWithProgress(ctx, labelSpecies, items)
		if err != nil {
			return nil, err
		}
		items = labeled
	}
	runs, err := w.executeConfiguredBlastBatchRuns(ctx, items, req, references)
	if err != nil {
		return nil, err
	}
	plan := &familyBlastPlan{
		Settings: familySettings,
		Groups:   detectFamilyBlastGroups(items, familySettings),
	}
	_, mergedRuns := applyFamilyBlastPlan(items, runs, plan)
	return mergedRuns, nil
}

func logReplayFamilyDiagnostics(t *testing.T, family string, runs []blastQueryRun, filterSettings model.BlastFilterSettings, familySettings model.FamilyBlastSettings) {
	t.Helper()
	for _, run := range runs {
		if replayRunFamily(run, familySettings) != family {
			continue
		}
		statusCounts := map[string]int{}
		inRangeCanonical := 0
		missingCanonical := 0
		selected := 0
		suggestion := prompt.DefaultBlastFilterSuggestion(prompt.BlastFilterRequest{
			Rows:     run.Results.Rows,
			Settings: filterSettings,
		})
		for i, row := range run.Results.Rows {
			status := strings.ToLower(strings.TrimSpace(row.InterProConservedRegionStatus))
			if status == "" {
				status = "<blank>"
			}
			statusCounts[status]++
			ratio := replayParseFloat(row.TargetUniProtCanonicalLengthPercent)
			if ratio == 0 {
				missingCanonical++
			} else if ratio >= filterSettings.MinTargetCanonicalLengthPercent && ratio <= filterSettings.MaxTargetCanonicalLengthPercent {
				inRangeCanonical++
			}
			if i < len(suggestion.Selected) && suggestion.Selected[i] {
				selected++
			}
		}
		t.Logf("diagnostic family=%s merged=%d selected=%d interpro=%s canonical_in_range=%d canonical_missing=%d",
			family,
			len(run.Results.Rows),
			selected,
			replayStatusSummary(statusCounts),
			inRangeCanonical,
			missingCanonical,
		)
		for i, row := range run.Results.Rows {
			keep := i < len(suggestion.Selected) && suggestion.Selected[i]
			targetQueryRatio := 0.0
			if row.QueryLength > 0 && row.TargetLength > 0 {
				targetQueryRatio = float64(row.TargetLength) / float64(row.QueryLength) * 100
			}
			if i < 12 {
				t.Logf("family=%s row=%02d keep=%t label=%q target=%q e=%s id=%.1f cov=%.1f target_len=%d query_len=%d tq_ratio=%.1f canon=%q interpro=%q uniprot=%q defline=%q url=%q",
					family,
					i+1,
					keep,
					row.LabelName,
					firstNonEmpty(row.Protein, row.SubjectID, row.SequenceID, row.TranscriptID),
					row.EValue,
					row.PercentIdentity,
					row.AlignQueryLengthPercent,
					row.TargetLength,
					row.QueryLength,
					targetQueryRatio,
					row.TargetUniProtCanonicalLengthPercent,
					row.InterProConservedRegionStatus,
					row.UniProtAccession,
					row.Defline,
					row.GeneReportURL,
				)
			}
		}
		return
	}
	t.Logf("diagnostic family=%s merged=0 selected=0", family)
}

func replayStatusSummary(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+strconv.Itoa(counts[key]))
	}
	return strings.Join(parts, ",")
}

func replayParseFloat(value string) float64 {
	value = strings.TrimSpace(strings.TrimSuffix(value, "%"))
	if value == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(value, 64)
	return v
}

func replayRunFamily(run blastQueryRun, settings model.FamilyBlastSettings) string {
	return replayFamilyFromStrings(settings,
		run.Item.FamilyName,
		run.Item.LabelName,
		run.Item.MemberLabel,
	)
}

func replayFamilyFromStrings(settings model.FamilyBlastSettings, values ...string) string {
	replaySettings := settings
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		for _, piece := range strings.FieldsFunc(value, func(r rune) bool { return r == '\n' || r == '\r' || r == '\t' }) {
			piece = strings.TrimSpace(piece)
			if piece == "" {
				continue
			}
			if family := detectFamilyName(piece, replaySettings); family != "" {
				return canonicalReplayFamily(family)
			}
			if family := canonicalReplayFamily(piece); family != "" {
				return family
			}
		}
	}
	return ""
}

func TestLiveKeywordRice4CLWorkflowAutoLabels(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live keyword workflow replay in short mode")
	}
	if os.Getenv("PHYTOZOME_LIVE_REPLAY") == "" {
		t.Skip("set PHYTOZOME_LIVE_REPLAY=1 to run live keyword workflow replay")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	phySpecies := model.SpeciesCandidate{
		ProteomeID:  323,
		JBrowseName: "Osativa_v7_0",
		GenomeLabel: "Oryza sativa v7.0",
	}
	lemSpecies := model.SpeciesCandidate{
		ProteomeID:  18,
		JBrowseName: "Sp_polyrhiza_9509",
		GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
		SearchAlias: "Spirodela polyrhiza",
		IsOfficial:  true,
	}

	phyCases := []struct {
		name      string
		nameType  string
		keywords  []string
		forceWide bool
		minRows   int
	}{
		{"phy-aliases", "alias", []string{"Os4CL1", "Os4CL2", "Os4CL3", "Os4CL4", "Os4CL5"}, false, 5},
		{"phy-locuses", "locus", []string{"Os08g14760.1", "Os02g46970.1", "Os02g08100.1", "Os06g44620.1", "Os08g34790.1"}, false, 5},
		{"phy-xp", "accession", []string{"XP_015650724.1", "XP_015624111.1", "XP_015625716.1", "XP_015643415.1", "XP_015650830.1"}, false, 5},
		{"phy-mixed", "mixed", []string{"Os4CL1", "Os02g46970.1", "XP_015625716.1", "Os4CL4", "XP_015650830.1"}, false, 5},
		{"phy-wide", "keyword", []string{"4CL"}, true, 1},
	}
	for _, tc := range phyCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := NewBlastWizard(os.Stdout)
			w.source = phytozome.NewClient(w.httpClient)
			groups, err := w.searchKeywordGroupsWithProgress(ctx, phySpecies, tc.keywords, nil, tc.forceWide, nil)
			if err != nil {
				t.Fatalf("searchKeywordGroupsWithProgress: %v", err)
			}
			labels, err := w.autoIdentifyKeywordLabelsWithProgress(ctx, phySpecies, groups)
			if err != nil {
				t.Fatalf("autoIdentifyKeywordLabelsWithProgress: %v", err)
			}
			if len(labels) != len(tc.keywords) {
				t.Fatalf("labels=%d, want %d", len(labels), len(tc.keywords))
			}
			labelNames := keywordIdentificationLabels(labels)
			totalRows := 0
			for i, group := range groups {
				totalRows += len(group.Rows)
				if len(group.Rows) == 0 {
					t.Fatalf("group %d keyword=%q returned no rows", i, group.SearchTerm)
				}
				if strings.TrimSpace(labelNames[i]) == "" {
					t.Fatalf("group %d keyword=%q auto label is empty", i, group.SearchTerm)
				}
			}
			if totalRows < tc.minRows {
				t.Fatalf("total rows=%d, want >= %d", totalRows, tc.minRows)
			}
			t.Logf("live keyword matrix source=phytozome name_type=%s queries=%d total_rows=%d labels=%v", tc.nameType, len(tc.keywords), totalRows, labelNames)
		})
	}

	lemCases := []struct {
		name           string
		nameType       string
		keywords       []string
		forceWide      bool
		expectNonEmpty bool
		allowEmpty     map[string]bool
	}{
		{"lem-aliases", "alias", []string{"Os4CL1", "Os4CL2", "Os4CL3", "Os4CL4", "Os4CL5"}, false, true, nil},
		{"lem-locus-controlled-zero", "locus", []string{"Os08g14760.1", "Os02g46970.1", "Os02g08100.1", "Os06g44620.1", "Os08g34790.1"}, false, false, nil},
		{"lem-xp", "accession", []string{"XP_015650724.1", "XP_015624111.1", "XP_015625716.1", "XP_015643415.1", "XP_015650830.1"}, false, true, nil},
		{"lem-mixed", "mixed", []string{"Os4CL1", "Os02g46970.1", "XP_015625716.1", "Os4CL4", "XP_015650830.1"}, false, true, map[string]bool{"Os02g46970.1": true}},
		{"lem-wide-4cl", "keyword", []string{"4CL"}, true, true, nil},
	}
	for _, tc := range lemCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := NewBlastWizard(os.Stdout)
			w.source = lemna.NewClient(w.httpClient)
			groups, err := w.searchKeywordGroupsWithProgress(ctx, lemSpecies, tc.keywords, nil, tc.forceWide, nil)
			if err != nil {
				t.Fatalf("searchKeywordGroupsWithProgress: %v", err)
			}
			labels, err := w.autoIdentifyKeywordLabelsWithProgress(ctx, lemSpecies, groups)
			if err != nil {
				t.Fatalf("autoIdentifyKeywordLabelsWithProgress: %v", err)
			}
			if len(labels) != len(tc.keywords) {
				t.Fatalf("labels=%d, want %d", len(labels), len(tc.keywords))
			}
			labelNames := keywordIdentificationLabels(labels)
			totalRows := 0
			for i, group := range groups {
				totalRows += len(group.Rows)
				if tc.expectNonEmpty {
					if tc.allowEmpty != nil && tc.allowEmpty[group.SearchTerm] && len(group.Rows) == 0 {
						continue
					}
					if len(group.Rows) == 0 {
						t.Fatalf("group %d keyword=%q returned no rows", i, group.SearchTerm)
					}
					if strings.TrimSpace(labelNames[i]) == "" {
						t.Fatalf("group %d keyword=%q auto label is empty", i, group.SearchTerm)
					}
				} else if len(group.Rows) != 0 {
					t.Fatalf("group %d keyword=%q rows=%d, want controlled zero results to avoid false-positive remaps", i, group.SearchTerm, len(group.Rows))
				}
			}
			t.Logf("live keyword matrix source=lemna name_type=%s queries=%d total_rows=%d labels=%v", tc.nameType, len(tc.keywords), totalRows, labelNames)
		})
	}
}

func logReplayResult(t *testing.T, result replayScenarioResult, targets map[string]int) {
	t.Helper()
	keys := make([]string, 0, len(targets))
	for key := range targets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		t.Logf("%s raw=%d merged=%d kept=%d target=%d diff=%d", key, result.RawByFamily[key], result.MergedByFamily[key], result.KeptByFamily[key], targets[key], result.KeptByFamily[key]-targets[key])
	}
}

func logReplayLabels(t *testing.T, title string, items []blastQueryItem, settings model.FamilyBlastSettings) {
	t.Helper()
	t.Log(title)
	for i, item := range items {
		t.Logf("query %02d label=%q aliases=%q source_label=%q auto_define_family=%q family=%q gene=%q protein=%q",
			i+1,
			item.LabelName,
			func() string {
				if item.QuerySource == nil {
					return ""
				}
				return item.QuerySource.Aliases
			}(),
			func() string {
				if item.QuerySource == nil {
					return ""
				}
				return item.QuerySource.LabelName
			}(),
			func() string {
				if item.QuerySource == nil {
					return ""
				}
				return replayFamilyFromStrings(settings, item.QuerySource.LabelName)
			}(),
			replayFamilyFromStrings(settings, item.LabelName, func() string {
				if item.QuerySource == nil {
					return ""
				}
				return item.QuerySource.LabelName
			}()),
			func() string {
				if item.QuerySource == nil {
					return ""
				}
				return item.QuerySource.GeneID
			}(),
			func() string {
				if item.QuerySource == nil {
					return ""
				}
				return item.QuerySource.ProteinID
			}(),
		)
	}
}

func loadReplayItemsFromExcel(ctx context.Context, path string, species model.SpeciesCandidate, httpClient *http.Client) ([]blastQueryItem, error) {
	file, err := excelize.OpenFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	rows, err := file.GetRows(file.GetSheetName(0))
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, nil
	}
	headers := map[string]int{}
	for i, value := range rows[0] {
		headers[strings.ToLower(strings.TrimSpace(value))] = i
	}
	get := func(row []string, key string) string {
		idx, ok := headers[strings.ToLower(key)]
		if !ok || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	client := phytozome.NewClient(httpClient)
	items := make([]blastQueryItem, 0, len(rows)-1)
	for _, row := range rows[1:] {
		geneURL := get(row, "gene_report_url")
		if geneURL == "" {
			continue
		}
		reportType, identifier, ok := replayGeneReportKeyword(geneURL)
		if !ok {
			continue
		}
		query, err := client.FetchGeneQuerySequence(ctx, species, reportType, identifier)
		if err != nil || query == nil || strings.TrimSpace(query.Sequence) == "" {
			continue
		}
		query.OriginalInputURL = geneURL
		query.NormalizedURL = geneURL
		query.SourceDatabase = "phytozome"
		if query.SourceProteomeID == 0 {
			query.SourceProteomeID = species.ProteomeID
		}
		if query.SourceJBrowseName == "" {
			query.SourceJBrowseName = species.JBrowseName
		}
		excelAliases := strings.TrimSpace(get(row, "alias"))
		if strings.TrimSpace(query.Aliases) == "" {
			query.Aliases = excelAliases
		} else if excelAliases != "" {
			query.Aliases = strings.TrimSpace(excelAliases + "; " + query.Aliases)
		}
		items = append(items, blastQueryItem{
			RawInput:    geneURL,
			Sequence:    query.Sequence,
			QuerySource: query,
			LabelName:   "",
		})
	}
	return items, nil
}

func replayGeneReportKeyword(value string) (reportType string, identifier string, ok bool) {
	normalized, ok := normalizeGeneReportURL(value)
	if !ok {
		return "", "", false
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", "", false
	}
	segments := nonEmptyPathSegments(parsed.Path)
	if len(segments) != 4 || !strings.EqualFold(segments[0], "report") {
		return "", "", false
	}
	switch strings.ToLower(strings.TrimSpace(segments[1])) {
	case "gene", "transcript", "protein":
		return strings.ToLower(strings.TrimSpace(segments[1])), strings.TrimSpace(segments[3]), strings.TrimSpace(segments[3]) != ""
	default:
		return "", "", false
	}
}

func canonicalReplayFamily(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "")
	value = strings.ReplaceAll(value, "'", "")
	value = strings.ReplaceAll(value, "_", "")
	switch {
	case strings.HasPrefix(value, "CAD"):
		return "CAD"
	case strings.HasPrefix(value, "CCOA"):
		return "CCOAMT"
	case strings.HasPrefix(value, "4CL"):
		return "4CL"
	case strings.HasPrefix(value, "CCR"):
		return "CCR"
	case strings.HasPrefix(value, "PAL"):
		return "PAL"
	case strings.HasPrefix(value, "C4H"):
		return "C4H"
	case strings.HasPrefix(value, "HCT"):
		return "HCT"
	case strings.HasPrefix(value, "COMT"), strings.HasPrefix(value, "OMT"):
		return "COMT"
	case strings.HasPrefix(value, "C3H"):
		return "C3H"
	case strings.HasPrefix(value, "F5H"), strings.HasPrefix(value, "FAH"):
		return "F5H"
	default:
		return value
	}
}

func loadReplayFastaItems(path string) ([]blastQueryItem, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var (
		items        []blastQueryItem
		header       string
		seqBuilder   strings.Builder
		flushCurrent = func() {
			h := strings.TrimSpace(header)
			seq := strings.TrimSpace(seqBuilder.String())
			if h == "" || seq == "" {
				return
			}
			label := replayLabelFromHeader(h)
			items = append(items, blastQueryItem{
				RawInput:  ">" + h + "\n" + seq,
				LabelName: label,
				Sequence:  seq,
				QuerySource: &model.QuerySequenceSource{
					Sequence:          seq,
					LabelName:         label,
					GeneID:            replayGeneIDFromHeader(h),
					ProteinID:         replayProteinIDFromHeader(h),
					TranscriptID:      replayProteinIDFromHeader(h),
					Aliases:           label,
					Annotation:        h,
					OrganismShort:     "A.thaliana",
					SourceDatabase:    "phytozome",
					SourceProteomeID:  167,
					SourceJBrowseName: "Athaliana_TAIR10",
					NormalizedURL:     replayGeneURLFromHeader(h),
					OriginalInputURL:  replayGeneURLFromHeader(h),
				},
			})
		}
	)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ">") {
			flushCurrent()
			header = strings.TrimPrefix(line, ">")
			seqBuilder.Reset()
			continue
		}
		seqBuilder.WriteString(line)
	}
	flushCurrent()
	return items, scanner.Err()
}

func replayLabelFromHeader(header string) string {
	if start := strings.LastIndex(header, "("); start >= 0 && strings.HasSuffix(header, ")") {
		return strings.TrimSpace(header[start+1 : len(header)-1])
	}
	return replayProteinIDFromHeader(header)
}

func replayProteinIDFromHeader(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	parts := strings.Split(header, "|")
	if len(parts) >= 2 {
		fields := strings.Fields(parts[1])
		if len(fields) > 0 {
			return strings.TrimSpace(fields[0])
		}
	}
	fields := strings.Fields(header)
	if len(fields) > 0 {
		return strings.TrimSpace(fields[0])
	}
	return header
}

func replayGeneIDFromHeader(header string) string {
	proteinID := replayProteinIDFromHeader(header)
	proteinID = strings.TrimSpace(proteinID)
	if proteinID == "" {
		return ""
	}
	if idx := strings.Index(proteinID, "."); idx > 0 {
		return proteinID[:idx]
	}
	return proteinID
}

func replayGeneURLFromHeader(header string) string {
	geneID := replayGeneIDFromHeader(header)
	if geneID == "" {
		return ""
	}
	return fmt.Sprintf("https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/%s", geneID)
}

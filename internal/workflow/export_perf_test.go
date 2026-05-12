// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/phytozome"
	"github.com/KiriKirby/phytozome-go/internal/report"
)

func TestKeywordExportPerformanceMatrixLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live keyword export performance matrix in short mode")
	}
	if os.Getenv("PHYTOZOME_LIVE_REPLAY") == "" {
		t.Skip("set PHYTOZOME_LIVE_REPLAY=1 to run live keyword export performance matrix")
	}
	if os.Getenv("PHYTO_EXPORT_PERF") == "" {
		t.Skip("set PHYTO_EXPORT_PERF=1 to run live export performance matrix")
	}

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

	type combo struct {
		name     string
		source   string
		species  model.SpeciesCandidate
		keywords []string
		wide     bool
		settings exportSettings
	}

	combos := []combo{
		{
			name:     "keyword-phy-excel-text-raw-report",
			source:   "phytozome",
			species:  phySpecies,
			keywords: []string{"Os4CL1", "XP_015624111.1", "Os06g44620.1"},
			settings: exportSettings{WriteExcel: true, WriteText: true, WriteRawExcel: true, WriteReport: true},
		},
		{
			name:     "keyword-lem-excel-text-raw-report",
			source:   "lemna",
			species:  lemSpecies,
			keywords: []string{"Os4CL1", "XP_015624111.1", "Os4CL5"},
			settings: exportSettings{WriteExcel: true, WriteText: true, WriteRawExcel: true, WriteReport: true},
		},
		{
			name:     "keyword-phy-excel-only",
			source:   "phytozome",
			species:  phySpecies,
			keywords: []string{"Os4CL1", "Os4CL2", "Os4CL3"},
			settings: exportSettings{WriteExcel: true},
		},
		{
			name:     "keyword-lem-wide-excel-text",
			source:   "lemna",
			species:  lemSpecies,
			keywords: []string{"4CL"},
			wide:     true,
			settings: exportSettings{WriteExcel: true, WriteText: true},
		},
	}

	if only := strings.ToLower(strings.TrimSpace(os.Getenv("PHYTO_EXPORT_COMBO"))); only != "" {
		filtered := combos[:0]
		for _, c := range combos {
			if strings.Contains(strings.ToLower(c.name), only) {
				filtered = append(filtered, c)
			}
		}
		combos = filtered
	}
	if len(combos) == 0 {
		t.Fatal("no keyword export combos selected")
	}

	for _, tc := range combos {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()

			w := NewBlastWizard(os.Stdout)
			w.suppressTaskModals = true
			switch tc.source {
			case "phytozome":
				w.source = phytozome.NewClient(w.httpClient)
			case "lemna":
				w.source = lemna.NewClient(w.httpClient)
			default:
				t.Fatalf("unsupported source %q", tc.source)
			}

			searchStarted := time.Now()
			groups, err := w.searchKeywordGroupsWithProgress(ctx, tc.species, tc.keywords, nil, tc.wide, nil)
			if err != nil {
				t.Fatalf("searchKeywordGroupsWithProgress: %v", err)
			}
			searchDuration := time.Since(searchStarted)

			labelStarted := time.Now()
			labels, err := w.autoIdentifyKeywordLabelsWithProgress(ctx, tc.species, groups)
			if err != nil {
				t.Fatalf("autoIdentifyKeywordLabelsWithProgress: %v", err)
			}
			applyKeywordIdentifications(groups, labels)
			labelDuration := time.Since(labelStarted)

			var selectedRows []model.KeywordResultRow
			var allRows []model.KeywordResultRow
			for _, group := range groups {
				allRows = append(allRows, group.Rows...)
				if len(group.Rows) > 0 {
					selectedRows = append(selectedRows, group.Rows[0])
				}
			}
			if len(selectedRows) == 0 {
				t.Fatal("no keyword rows selected for export replay")
			}

			outputDir := filepath.Join(os.TempDir(), "phytozome-go-export-perf", tc.name)
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				t.Fatalf("create export output dir: %v", err)
			}

			settings := tc.settings
			settings.BaseName = sanitizeExportName(tc.name)
			settings.OutputDir = outputDir
			reportCtx := &keywordReportRunContext{
				Selected:      tc.species,
				QueryStarted:  searchStarted,
				SearchEnded:   searchStarted.Add(searchDuration),
				ReviewStarted: time.Now(),
				LabelMode:     "auto identify labelname",
			}

			exportStarted := time.Now()
			err = w.exportSelectedKeywordFiles(ctx, tc.species, selectedRows, allRows, groups, settings.BaseName, outputDir, settings, reportCtx, false)
			exportDuration := time.Since(exportStarted)
			if err != nil {
				t.Fatalf("exportSelectedKeywordFiles: %v", err)
			}

			t.Logf(
				"combo=%s source=%s groups=%d selected=%d all_rows=%d wide=%t excel=%t text=%t raw=%t report=%t keyword_ms=%d autolabel_ms=%d export_total_ms=%d output=%s",
				tc.name,
				tc.source,
				len(groups),
				len(selectedRows),
				len(allRows),
				tc.wide,
				settings.WriteExcel,
				settings.WriteText,
				settings.WriteRawExcel,
				settings.WriteReport,
				searchDuration.Milliseconds(),
				labelDuration.Milliseconds(),
				exportDuration.Milliseconds(),
				outputDir,
			)
			logExportDirArtifacts(t, outputDir)
		})
	}
}

func TestBlastExportPerformanceMatrixLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live blast export performance matrix in short mode")
	}
	if os.Getenv("PHYTOZOME_LIVE_REPLAY") == "" {
		t.Skip("set PHYTOZOME_LIVE_REPLAY=1 to run live blast export performance matrix")
	}
	if os.Getenv("PHYTO_EXPORT_PERF") == "" {
		t.Skip("set PHYTO_EXPORT_PERF=1 to run live export performance matrix")
	}

	replayEnsureBlastPlusPath(t)
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

	type combo struct {
		name        string
		source      string
		species     model.SpeciesCandidate
		items       []blastQueryItem
		request     model.BlastRequest
		references  externalReferenceConfig
		settings    exportSettings
	}

	combos := []combo{
		{
			name:    "blast-phy-refs-excel-text-raw-report",
			source:  "phytozome",
			species: phySpecies,
			request: replayBlastPRequest(phySpecies, "BLASTP"),
			references: externalReferenceConfig{
				UseUniProt:       true,
				UseInterPro:      true,
				InterProSettings: model.DefaultInterProConservedRegionSettings(),
			},
			settings: exportSettings{WriteExcel: true, WriteText: true, WriteRawExcel: true, WriteReport: true},
		},
		{
			name:    "blast-lem-refs-excel-text-raw-report",
			source:  "lemna",
			species: lemSpecies,
			request: replayBlastPRequest(lemSpecies, "local:BLASTP"),
			references: externalReferenceConfig{
				UseUniProt:       true,
				UseInterPro:      true,
				InterProSettings: model.DefaultInterProConservedRegionSettings(),
			},
			settings: exportSettings{WriteExcel: true, WriteText: true, WriteRawExcel: true, WriteReport: true},
		},
		{
			name:    "blast-phy-excel-only",
			source:  "phytozome",
			species: phySpecies,
			request: replayBlastPRequest(phySpecies, "BLASTP"),
			settings: exportSettings{WriteExcel: true},
		},
		{
			name:    "blast-lem-excel-text",
			source:  "lemna",
			species: lemSpecies,
			request: replayBlastPRequest(lemSpecies, "local:BLASTP"),
			settings: exportSettings{WriteExcel: true, WriteText: true},
		},
	}

	if only := strings.ToLower(strings.TrimSpace(os.Getenv("PHYTO_EXPORT_COMBO"))); only != "" {
		filtered := combos[:0]
		for _, c := range combos {
			if strings.Contains(strings.ToLower(c.name), only) {
				filtered = append(filtered, c)
			}
		}
		combos = filtered
	}
	if len(combos) == 0 {
		t.Fatal("no blast export combos selected")
	}

	for _, tc := range combos {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
			defer cancel()

			w := NewBlastWizard(os.Stdout)
			w.suppressTaskModals = true
			switch tc.source {
			case "phytozome":
				w.source = phytozome.NewClient(w.httpClient)
			case "lemna":
				w.source = lemna.NewClient(w.httpClient)
			default:
				t.Fatalf("unsupported source %q", tc.source)
			}

			keywordSeed := []string{"Os4CL1", "XP_015624111.1", "Os06g44620.1"}
			if tc.source == "lemna" {
				keywordSeed = []string{"Os4CL1", "XP_015624111.1", "Os4CL5"}
			}
			prepareStarted := time.Now()
			groups, err := w.searchKeywordGroupsWithProgress(ctx, tc.species, keywordSeed, nil, false, nil)
			if err != nil {
				t.Fatalf("searchKeywordGroupsWithProgress: %v", err)
			}
			labels, err := w.autoIdentifyKeywordLabelsWithProgress(ctx, tc.species, groups)
			if err != nil {
				t.Fatalf("autoIdentifyKeywordLabelsWithProgress: %v", err)
			}
			applyKeywordIdentifications(groups, labels)
			selectedKeywordRows := make([]model.KeywordResultRow, 0, len(groups))
			for _, group := range groups {
				if len(group.Rows) == 0 {
					continue
				}
				selectedKeywordRows = append(selectedKeywordRows, group.Rows[0])
			}
			if len(selectedKeywordRows) == 0 {
				t.Fatal("no keyword rows resolved for blast export replay")
			}
			items, err := w.resolveKeywordRowsToBlastItems(ctx, tc.species, selectedKeywordRows)
			if err != nil {
				t.Fatalf("resolveKeywordRowsToBlastItems: %v", err)
			}
			items, err = w.prepareKeywordBlastItems(ctx, tc.species, items)
			if err != nil {
				t.Fatalf("prepareKeywordBlastItems: %v", err)
			}
			prepareDuration := time.Since(prepareStarted)
			if len(items) == 0 {
				t.Fatal("no blast items prepared")
			}

			blastStarted := time.Now()
			runs, err := w.executeConfiguredBlastBatchRuns(ctx, items, tc.request, tc.references)
			if err != nil {
				t.Fatalf("executeConfiguredBlastBatchRuns: %v", err)
			}
			blastDuration := time.Since(blastStarted)

			filterSettings := model.DefaultBlastFilterSettings()
			rowsByRun, rowNumbersByRun, filterFlagsByRun, selectedByRun := replayDefaultFilteredRows(runs, filterSettings)
			selectedRows := replayCountSelectedRows(rowsByRun)
			if selectedRows == 0 {
				t.Fatal("no selected blast rows for export replay")
			}

			outputDir := filepath.Join(os.TempDir(), "phytozome-go-export-perf", tc.name)
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				t.Fatalf("create export output dir: %v", err)
			}

			settings := tc.settings
			settings.BaseName = sanitizeExportName(tc.name)
			settings.OutputDir = outputDir

			exportStarted := time.Now()
			batchResult, err := w.exportAllBlastRunsWithProgress(ctx, tc.species, items, runs, rowsByRun, rowNumbersByRun, filterFlagsByRun, selectedByRun, tc.request, settings)
			if err != nil {
				t.Fatalf("exportAllBlastRunsWithProgress: %v", err)
			}
			exportDuration := time.Since(exportStarted)

			reportDuration := time.Duration(0)
			if settings.WriteReport {
				reportStarted := time.Now()
				reportPath, err := w.renderBlastBatchReport(ctx, tc.species, items, items, cloneBlastQueryRuns(runs), batchResult.Files, rowsByRun, rowNumbersByRun, filterFlagsByRun, selectedByRun, outputDir, settings, tc.request, filterSettings, true, false)
				if err != nil {
					t.Fatalf("renderBlastBatchReport: %v", err)
				}
				if strings.TrimSpace(reportPath) == "" {
					t.Fatal("renderBlastBatchReport returned empty report path")
				}
				reportDuration = time.Since(reportStarted)
			}

			totalRows := 0
			for _, run := range runs {
				totalRows += len(run.Results.Rows)
			}

			t.Logf(
				"combo=%s source=%s queries=%d runs=%d rows=%d selected=%d refs_uniprot=%t refs_interpro=%t excel=%t text=%t raw=%t report=%t prepare_ms=%d blast_ms=%d export_ms=%d report_ms=%d total_ms=%d output=%s",
				tc.name,
				tc.source,
				len(items),
				len(runs),
				totalRows,
				selectedRows,
				tc.references.UseUniProt,
				tc.references.UseInterPro,
				settings.WriteExcel,
				settings.WriteText,
				settings.WriteRawExcel,
				settings.WriteReport,
				prepareDuration.Milliseconds(),
				blastDuration.Milliseconds(),
				exportDuration.Milliseconds(),
				reportDuration.Milliseconds(),
				(prepareDuration + blastDuration + exportDuration + reportDuration).Milliseconds(),
				outputDir,
			)
			logBlastExportSteps(t, batchResult.Files)
			logExportDirArtifacts(t, outputDir)
		})
	}
}

func TestExportPerformanceSweepLog(t *testing.T) {
	if os.Getenv("PHYTO_EXPORT_PERF_SWEEP") == "" {
		t.Skip("set PHYTO_EXPORT_PERF_SWEEP=1 to log current export worker settings")
	}
	t.Logf(
		"workers max=%s disk=%s http_idle=%s http_host=%s blast_batch_local=%s blast_batch_remote=%s blast_threads=%s blast_poll=%s",
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_MAX_WORKERS")),
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_DISK_WORKERS")),
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_MAX_IDLE_CONNS")),
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_MAX_IDLE_CONNS_PER_HOST")),
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_LOCAL_BLAST_BATCH_WORKERS")),
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_REMOTE_BLAST_BATCH_WORKERS")),
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_LOCAL_BLAST_THREADS")),
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_BLAST_POLL_MS")),
	)
	fmt.Println("")
}

func logBlastExportSteps(t *testing.T, files []exportFileResult) {
	t.Helper()
	for i, file := range files {
		for _, step := range file.Steps {
			t.Logf("blast_export_step run=%d name=%q status=%s ms=%d details=%q", i+1, step.Name, step.Status, step.DurationMS, step.Details)
		}
	}
}

func logExportDirArtifacts(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Logf("export_dir_scan_failed dir=%s err=%v", dir, err)
		return
	}
	type artifact struct {
		name string
		size int64
	}
	artifacts := make([]artifact, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		artifacts = append(artifacts, artifact{name: entry.Name(), size: info.Size()})
	}
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].name < artifacts[j].name
	})
	for _, item := range artifacts {
		t.Logf("export_artifact dir=%s file=%s size_bytes=%d", dir, item.name, item.size)
	}
}

func logKeywordExportSteps(t *testing.T, data report.ReportData) {
	t.Helper()
	for _, step := range data.Keyword.GenerationSteps {
		t.Logf("keyword_export_step name=%q status=%s ms=%d details=%q", step.Name, step.Status, step.DurationMS, step.Details)
	}
}

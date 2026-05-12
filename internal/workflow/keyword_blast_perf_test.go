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
	"strings"
	"testing"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/phytozome"
)

func TestKeywordBlastPerformanceMatrixLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live keyword->blast performance matrix in short mode")
	}
	if os.Getenv("PHYTOZOME_LIVE_REPLAY") == "" {
		t.Skip("set PHYTOZOME_LIVE_REPLAY=1 to run live keyword->blast performance matrix")
	}
	if os.Getenv("PHYTO_KEYWORD_BLAST_PERF") == "" {
		t.Skip("set PHYTO_KEYWORD_BLAST_PERF=1 to run keyword->blast performance matrix")
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
		name          string
		keywordSource string
		blastSource   string
		keywordSpecies model.SpeciesCandidate
		blastSpecies   model.SpeciesCandidate
		keywords      []string
		forceWide     bool
		blastProgram  string
	}

	combos := []combo{
		{
			name:           "phy-keyword_to_phy-blast",
			keywordSource:  "phytozome",
			blastSource:    "phytozome",
			keywordSpecies: phySpecies,
			blastSpecies:   phySpecies,
			keywords:       []string{"Os4CL1", "XP_015624111.1", "Os06g44620.1"},
			blastProgram:   "BLASTP",
		},
		{
			name:           "phy-keyword_to_lem-blast",
			keywordSource:  "phytozome",
			blastSource:    "lemna",
			keywordSpecies: phySpecies,
			blastSpecies:   lemSpecies,
			keywords:       []string{"Os4CL1", "XP_015624111.1", "Os06g44620.1"},
			blastProgram:   "local:BLASTP",
		},
		{
			name:           "lem-keyword_to_phy-blast",
			keywordSource:  "lemna",
			blastSource:    "phytozome",
			keywordSpecies: lemSpecies,
			blastSpecies:   phySpecies,
			keywords:       []string{"Os4CL1", "XP_015624111.1", "Os4CL5"},
			blastProgram:   "BLASTP",
		},
		{
			name:           "lem-keyword_to_lem-blast",
			keywordSource:  "lemna",
			blastSource:    "lemna",
			keywordSpecies: lemSpecies,
			blastSpecies:   lemSpecies,
			keywords:       []string{"Os4CL1", "XP_015624111.1", "Os4CL5"},
			blastProgram:   "local:BLASTP",
		},
	}

	if only := strings.ToLower(strings.TrimSpace(os.Getenv("PHYTO_KEYWORD_BLAST_COMBO"))); only != "" {
		filtered := combos[:0]
		for _, c := range combos {
			if strings.Contains(strings.ToLower(c.name), only) {
				filtered = append(filtered, c)
			}
		}
		combos = filtered
	}
	if len(combos) == 0 {
		t.Fatal("no performance combos selected")
	}

	references := externalReferenceConfig{
		UseUniProt:       true,
		UseInterPro:      true,
		InterProSettings: model.DefaultInterProConservedRegionSettings(),
	}

	for _, tc := range combos {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			replayEnsureKeywordBlastPerfBlastPlusPath()
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
			defer cancel()

			w := NewBlastWizard(os.Stdout)
			w.suppressTaskModals = true
			switch tc.keywordSource {
			case "phytozome":
				w.source = phytozome.NewClient(w.httpClient)
			case "lemna":
				w.source = lemna.NewClient(w.httpClient)
			default:
				t.Fatalf("unsupported keyword source %q", tc.keywordSource)
			}

			searchStarted := time.Now()
			groups, err := w.searchKeywordGroupsWithProgress(ctx, tc.keywordSpecies, tc.keywords, nil, tc.forceWide, nil)
			if err != nil {
				t.Fatalf("searchKeywordGroupsWithProgress: %v", err)
			}
			searchDuration := time.Since(searchStarted)

			labelStarted := time.Now()
			labels, err := w.autoIdentifyKeywordLabelsWithProgress(ctx, tc.keywordSpecies, groups)
			if err != nil {
				t.Fatalf("autoIdentifyKeywordLabelsWithProgress: %v", err)
			}
			labelDuration := time.Since(labelStarted)
			applyKeywordIdentifications(groups, labels)

			selectedRows := make([]model.KeywordResultRow, 0, len(groups))
			for _, group := range groups {
				if len(group.Rows) == 0 {
					continue
				}
				selectedRows = append(selectedRows, group.Rows[0])
			}
			if len(selectedRows) == 0 {
				t.Fatal("no keyword rows selected for BLAST replay")
			}

			prepareStarted := time.Now()
			items, err := w.resolveKeywordRowsToBlastItems(ctx, tc.keywordSpecies, selectedRows)
			if err != nil {
				t.Fatalf("resolveKeywordRowsToBlastItems: %v", err)
			}
			items, err = w.prepareKeywordBlastItems(ctx, tc.keywordSpecies, items)
			if err != nil {
				t.Fatalf("prepareKeywordBlastItems: %v", err)
			}
			prepareDuration := time.Since(prepareStarted)
			if len(items) == 0 {
				t.Fatal("no BLAST items prepared from keyword rows")
			}

			switch tc.blastSource {
			case "phytozome":
				w.source = phytozome.NewClient(w.httpClient)
			case "lemna":
				w.source = lemna.NewClient(w.httpClient)
			default:
				t.Fatalf("unsupported BLAST source %q", tc.blastSource)
			}

			request := model.BlastRequest{
				Species:          tc.blastSpecies,
				SequenceKind:     model.SequenceProtein,
				TargetType:       "proteome",
				Program:          tc.blastProgram,
				EValue:           "1e-10",
				ComparisonMatrix: "BLOSUM62",
				WordLength:       "default",
				AlignmentsToShow: 20,
				AllowGaps:        true,
				FilterQuery:      true,
			}
			blastStarted := time.Now()
			runs, err := w.executeConfiguredBlastBatchRuns(ctx, items, request, references)
			if err != nil {
				t.Fatalf("executeConfiguredBlastBatchRuns: %v", err)
			}
			blastDuration := time.Since(blastStarted)

			totalRows := 0
			for _, run := range runs {
				totalRows += len(run.Results.Rows)
			}
			t.Logf(
				"combo=%s keyword_source=%s blast_source=%s max_workers=%s queries=%d selected=%d items=%d runs=%d blast_rows=%d keyword_ms=%d autolabel_ms=%d prepare_ms=%d blast_ms=%d total_ms=%d",
				tc.name,
				tc.keywordSource,
				tc.blastSource,
				strings.TrimSpace(os.Getenv("PHYTOZOME_GO_MAX_WORKERS")),
				len(tc.keywords),
				len(selectedRows),
				len(items),
				len(runs),
				totalRows,
				searchDuration.Milliseconds(),
				labelDuration.Milliseconds(),
				prepareDuration.Milliseconds(),
				blastDuration.Milliseconds(),
				(searchDuration + labelDuration + prepareDuration + blastDuration).Milliseconds(),
			)
			if totalRows == 0 {
				t.Fatalf("combo=%s produced no BLAST rows", tc.name)
			}
		})
	}
}

func TestKeywordBlastPerformanceSweepLog(t *testing.T) {
	if os.Getenv("PHYTO_KEYWORD_BLAST_SWEEP") == "" {
		t.Skip("set PHYTO_KEYWORD_BLAST_SWEEP=1 to log current worker settings")
	}
	t.Logf(
		"workers max=%s disk=%s http_idle=%s http_host=%s",
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_MAX_WORKERS")),
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_DISK_WORKERS")),
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_MAX_IDLE_CONNS")),
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_MAX_IDLE_CONNS_PER_HOST")),
	)
	fmt.Println("")
}

func replayEnsureKeywordBlastPerfBlastPlusPath() {
	for _, root := range []string{
		filepath.Join(".", "bin", "blastplus"),
		`C:\Users\wangsychn\Desktop\phytozome-go_windows_amd64\blastplus`,
	} {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
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
					if !strings.Contains(strings.ToLower(current), strings.ToLower(path)) {
						_ = os.Setenv("PATH", path+string(os.PathListSeparator)+current)
					}
					return filepath.SkipAll
				}
			}
			return nil
		})
	}
}

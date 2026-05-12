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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/lemna"
	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/phytozome"
)

func TestBlastPerformanceMatrixLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live BLAST performance matrix in short mode")
	}
	if os.Getenv("PHYTOZOME_LIVE_REPLAY") == "" {
		t.Skip("set PHYTOZOME_LIVE_REPLAY=1 to run live BLAST performance matrix")
	}
	if os.Getenv("PHYTO_BLAST_PERF") == "" {
		t.Skip("set PHYTO_BLAST_PERF=1 to run live BLAST performance matrix")
	}

	phySpecies := model.SpeciesCandidate{
		ProteomeID:  167,
		JBrowseName: "Athaliana_TAIR10",
		GenomeLabel: "Arabidopsis thaliana TAIR10",
		SearchAlias: "Arabidopsis thaliana",
		CommonName:  "thale cress",
	}
	lemSpecies := model.SpeciesCandidate{
		ProteomeID:  18,
		JBrowseName: "Sp_polyrhiza_9509",
		GenomeLabel: "Spirodela polyrhiza 9509 REF-OXFORD-3.0",
		SearchAlias: "Spirodela polyrhiza",
		IsOfficial:  true,
	}

	type combo struct {
		name         string
		sourceName   string
		selected     model.SpeciesCandidate
		program      string
		rawInputs    []string
		fallbackItem *model.QuerySequenceSource
	}

	combos := []combo{
		{
			name:       "phytozome-remote",
			sourceName: "phytozome",
			selected:   phySpecies,
			program:    "BLASTP",
			rawInputs: []string{
				"https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G30490",
				"https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT5G13930",
				"https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT2G37040",
			},
		},
		{
			name:       "lemna-local",
			sourceName: "lemna",
			selected:   lemSpecies,
			program:    "local:BLASTP",
			rawInputs: []string{
				"XP_015650724.1",
				"XP_015624111.1",
				"XP_015643415.1",
			},
		},
	}

	if only := strings.ToLower(strings.TrimSpace(os.Getenv("PHYTO_BLAST_PERF_SOURCE"))); only != "" {
		filtered := combos[:0]
		for _, c := range combos {
			if strings.Contains(strings.ToLower(c.name), only) || strings.Contains(strings.ToLower(c.sourceName), only) {
				filtered = append(filtered, c)
			}
		}
		combos = filtered
	}
	if len(combos) == 0 {
		t.Fatal("no BLAST performance combos selected")
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
			switch tc.sourceName {
			case "phytozome":
				w.source = phytozome.NewClient(w.httpClient)
			case "lemna":
				w.source = lemna.NewClient(w.httpClient)
			default:
				t.Fatalf("unsupported source %q", tc.sourceName)
			}

			resolveStarted := time.Now()
			prepared, err := blastPerfResolveItems(ctx, w, tc.selected, tc.rawInputs)
			if err != nil {
				t.Fatalf("resolve blast inputs: %v", err)
			}
			resolveDuration := time.Since(resolveStarted)
			if len(prepared) == 0 {
				t.Fatal("no BLAST items resolved")
			}

			labelStarted := time.Now()
			prepared, err = w.autoIdentifyBlastLabelsWithProgress(ctx, tc.selected, prepared)
			if err != nil {
				t.Fatalf("autoIdentifyBlastLabelsWithProgress: %v", err)
			}
			if !keywordBlastItemsHaveReusableAliases(prepared) {
				prepared, err = w.supplementBlastAliasesWithProgress(ctx, tc.selected, prepared)
				if err != nil {
					t.Fatalf("supplementBlastAliasesWithProgress: %v", err)
				}
			}
			labelDuration := time.Since(labelStarted)

			request := model.BlastRequest{
				Species:          tc.selected,
				SequenceKind:     model.SequenceProtein,
				TargetType:       "proteome",
				Program:          tc.program,
				EValue:           "1e-10",
				ComparisonMatrix: "BLOSUM62",
				WordLength:       "default",
				AlignmentsToShow: 20,
				AllowGaps:        true,
				FilterQuery:      true,
			}

			blastStarted := time.Now()
			runs, err := w.executeConfiguredBlastBatchRuns(ctx, prepared, request, references)
			if err != nil {
				t.Fatalf("executeConfiguredBlastBatchRuns: %v", err)
			}
			blastDuration := time.Since(blastStarted)

			totalRows := 0
			for _, run := range runs {
				totalRows += len(run.Results.Rows)
			}
			if totalRows == 0 {
				t.Fatalf("combo=%s produced no BLAST rows", tc.name)
			}

			t.Logf(
				"combo=%s source=%s program=%s workers=%s local_threads=%s queries=%d runs=%d rows=%d resolve_ms=%d autolabel_ms=%d blast_ms=%d total_ms=%d",
				tc.name,
				tc.sourceName,
				tc.program,
				strings.TrimSpace(os.Getenv("PHYTOZOME_GO_MAX_WORKERS")),
				strings.TrimSpace(os.Getenv("PHYTOZOME_GO_LOCAL_BLAST_THREADS")),
				len(prepared),
				len(runs),
				totalRows,
				resolveDuration.Milliseconds(),
				labelDuration.Milliseconds(),
				blastDuration.Milliseconds(),
				(resolveDuration + labelDuration + blastDuration).Milliseconds(),
			)
		})
	}
}

func TestBlastPerformanceSweepLog(t *testing.T) {
	if os.Getenv("PHYTO_BLAST_PERF_SWEEP") == "" {
		t.Skip("set PHYTO_BLAST_PERF_SWEEP=1 to log current BLAST worker settings")
	}
	t.Logf(
		"workers max=%s disk=%s http_idle=%s http_host=%s local_threads=%s gomaxprocs=%d",
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_MAX_WORKERS")),
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_DISK_WORKERS")),
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_MAX_IDLE_CONNS")),
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_MAX_IDLE_CONNS_PER_HOST")),
		strings.TrimSpace(os.Getenv("PHYTOZOME_GO_LOCAL_BLAST_THREADS")),
		currentCPUCount(),
	)
}

func blastPerfResolveItems(ctx context.Context, w *BlastWizard, selected model.SpeciesCandidate, rawInputs []string) ([]blastQueryItem, error) {
	items := make([]blastQueryItem, 0, len(rawInputs))
	if _, ok := w.source.(*lemna.Client); ok {
		for _, raw := range rawInputs {
			rows, err := w.source.SearchKeywordRows(ctx, selected, raw)
			if err != nil {
				return nil, err
			}
			if len(rows) == 0 {
				return nil, fmt.Errorf("no lemna keyword rows matched %q", raw)
			}
			row := rows[0]
			sequenceID := strings.TrimSpace(row.SequenceID)
			if sequenceID == "" {
				sequenceID = firstNonEmpty(row.ProteinID, row.TranscriptID, row.GeneIdentifier)
			}
			if sequenceID == "" {
				return nil, fmt.Errorf("lemna row has no usable sequence identifier for %q", raw)
			}
			sequence, err := w.source.FetchProteinSequence(ctx, selected.ProteomeID, sequenceID)
			if err != nil {
				return nil, err
			}
			source := &model.QuerySequenceSource{
				Sequence:          sequence,
				SourceDatabase:    w.source.Name(),
				SourceProteomeID:  selected.ProteomeID,
				SourceJBrowseName: selected.JBrowseName,
				SourceGenomeLabel: selected.GenomeLabel,
				LabelName:         strings.TrimSpace(row.LabelName),
				Aliases:           strings.TrimSpace(row.Aliases),
				AutoDefine:        strings.TrimSpace(row.AutoDefine),
				UniProtAccession:  strings.TrimSpace(row.UniProt),
				GeneID:            strings.TrimSpace(row.GeneIdentifier),
				TranscriptID:      strings.TrimSpace(row.TranscriptID),
				ProteinID:         firstNonEmpty(row.ProteinID, row.SequenceID, row.TranscriptID),
				OrganismShort:     firstNonEmpty(strings.TrimSpace(row.SequenceHeaderLabel), selected.SearchAlias, selected.GenomeLabel),
				Annotation:        firstNonEmpty(strings.TrimSpace(row.Description), strings.TrimSpace(row.Comments), selected.GenomeLabel),
				OriginalInputURL:  strings.TrimSpace(raw),
				NormalizedURL:     strings.TrimSpace(raw),
			}
			items = append(items, blastQueryItem{
				RawInput:    raw,
				Sequence:    sequence,
				QuerySource: source,
			})
		}
		return items, nil
	}
	candidates, err := w.speciesCandidatesForSource(ctx, w.source, nil)
	if err != nil {
		return nil, err
	}
	for _, raw := range rawInputs {
		source, ok, err := w.resolveQuerySequenceInputBatchWithTimeout(ctx, candidates, raw)
		if err != nil {
			return nil, err
		}
		if !ok || source == nil {
			return nil, strconv.ErrSyntax
		}
		items = append(items, blastQueryItem{
			RawInput:    raw,
			Sequence:    source.Sequence,
			QuerySource: source,
		})
	}
	return items, nil
}

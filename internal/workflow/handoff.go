// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	"github.com/KiriKirby/phytozome-go/internal/model"
)

type InstanceHandoff struct {
	Kind          string                     `json:"kind"`
	RunID         string                     `json:"run_id"`
	ParentID      string                     `json:"parent_id"`
	InstanceID    string                     `json:"instance_id"`
	Database      string                     `json:"database"`
	Mode          string                     `json:"mode"`
	BlastContext  blastHandoffContext        `json:"blast_context"`
	StartupSource string                     `json:"startup_source"`
}

type blastHandoffContext struct {
	PendingMode            string                  `json:"pending_mode"`
	TransferKind           string                  `json:"transfer_kind"`
	BlastProgramPath       string                  `json:"blast_program_path"`
	ReuseLastBlastInput    bool                    `json:"reuse_last_blast_input"`
	ReuseLastBlastRows     bool                    `json:"reuse_last_blast_rows"`
	ReuseLastKeywordRows   bool                    `json:"reuse_last_keyword_rows"`
	RewindBlastToInput     bool                    `json:"rewind_blast_to_input"`
	RewindKeywordToInput   bool                    `json:"rewind_keyword_to_input"`
	TransferSourceSpecies  model.SpeciesCandidate  `json:"transfer_source_species"`
	TransferKeywordRows    []model.KeywordResultRow `json:"transfer_keyword_rows"`
	TransferBlastRows      []model.BlastResultRow  `json:"transfer_blast_rows"`
	LastBlastItems         []blastQueryItem        `json:"last_blast_items"`
	LastKeywordGroups      []model.KeywordSearchGroup `json:"last_keyword_groups"`
	LastKeywordReport      *keywordReportRunContext `json:"last_keyword_report,omitempty"`
	LastKeywordSpecies     model.SpeciesCandidate  `json:"last_keyword_species"`
	LastBlastRowContext    *blastRowContext        `json:"last_blast_row_context,omitempty"`
	LastBlastReviewContext *blastReviewContext     `json:"last_blast_review_context,omitempty"`
}

func (w *BlastWizard) SnapshotHandoff(database string, mode QueryMode, instanceID string, parentID string, runID string) InstanceHandoff {
	handoff := InstanceHandoff{
		Kind:          "blast-session",
		RunID:         strings.TrimSpace(runID),
		ParentID:      strings.TrimSpace(parentID),
		InstanceID:    strings.TrimSpace(instanceID),
		Database:      strings.TrimSpace(database),
		Mode:          string(mode),
		StartupSource:  "database-selection",
	}
	handoff.BlastContext = blastHandoffContext{
		PendingMode:          string(w.pendingMode),
		BlastProgramPath:     strings.TrimSpace(w.blastProgramPath),
		ReuseLastBlastInput:  w.reuseLastBlastInput,
		ReuseLastBlastRows:   w.reuseLastBlastRows,
		ReuseLastKeywordRows: w.reuseLastKeywordRows,
		RewindBlastToInput:   w.rewindBlastToInput,
		RewindKeywordToInput: w.rewindKeywordToInput,
		LastBlastItems:       cloneBlastQueryItems(w.lastBlastItems),
		LastKeywordGroups:    cloneKeywordSearchGroups(w.lastKeywordGroups),
		LastKeywordSpecies:   w.lastKeywordSpecies,
	}
	if w.lastKeywordReport != nil {
		reportCopy := *w.lastKeywordReport
		handoff.BlastContext.LastKeywordReport = &reportCopy
	}
	if w.lastBlastRowContext != nil {
		rowCopy := *w.lastBlastRowContext
		rowCopy.Rows = append([]model.BlastResultRow(nil), w.lastBlastRowContext.Rows...)
		rowCopy.AllRows = append([]model.BlastResultRow(nil), w.lastBlastRowContext.AllRows...)
		rowCopy.Numbers = append([]int(nil), w.lastBlastRowContext.Numbers...)
		rowCopy.Flags = append([]bool(nil), w.lastBlastRowContext.Flags...)
		rowCopy.SelectedRowsMask = append([]bool(nil), w.lastBlastRowContext.SelectedRowsMask...)
		handoff.BlastContext.LastBlastRowContext = &rowCopy
	}
	if w.lastBlastReviewContext != nil {
		reviewCopy := *w.lastBlastReviewContext
		reviewCopy.Prepared = cloneBlastQueryItems(w.lastBlastReviewContext.Prepared)
		reviewCopy.Runs = cloneBlastQueryRuns(w.lastBlastReviewContext.Runs)
		handoff.BlastContext.LastBlastReviewContext = &reviewCopy
	}
	return handoff
}

func HandoffDir(runID string) (string, error) {
	root, err := appfs.CacheDir("session", strings.TrimSpace(runID), "handoff")
	if err != nil {
		return "", err
	}
	return root, nil
}

func SaveInstanceHandoff(runID string, handoff InstanceHandoff) (string, error) {
	dir, err := HandoffDir(runID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("ensure handoff directory: %w", err)
	}
	name := strings.TrimSpace(handoff.InstanceID)
	if name == "" {
		name = "instance"
	}
	path := filepath.Join(dir, sanitizeExportName(name)+".json")
	data, err := json.MarshalIndent(handoff, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal handoff: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write handoff: %w", err)
	}
	return path, nil
}

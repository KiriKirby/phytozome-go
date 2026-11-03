// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package model

import "time"

type FastaHeaderMode string

const (
	FastaHeaderModePhgo        FastaHeaderMode = "phgo"
	FastaHeaderModePhgoLite    FastaHeaderMode = "phgo_lite"
	FastaHeaderModeOriginal    FastaHeaderMode = "original"
	FastaHeaderModeMinimal     FastaHeaderMode = "minimal"
	FastaHeaderModeDisplayName FastaHeaderMode = "display_name"
)

func (m FastaHeaderMode) String() string {
	return string(m)
}

func NormalizeFastaHeaderMode(mode FastaHeaderMode, legacyUsePhgo bool) FastaHeaderMode {
	switch mode {
	case FastaHeaderModePhgo, FastaHeaderModePhgoLite, FastaHeaderModeOriginal, FastaHeaderModeMinimal, FastaHeaderModeDisplayName:
		return mode
	}
	if legacyUsePhgo {
		return FastaHeaderModePhgo
	}
	return FastaHeaderModeOriginal
}

type ProteinSequenceRecord struct {
	Header         string
	OriginalHeader string
	SourceKey      string
	Sequence       string
}

type ProteinSequenceData struct {
	Sequence       string
	OriginalHeader string
}

type ExportMetadata struct {
	GeneName      string
	GeneID        string
	GeneReportURL string
	Queries       []ExportQueryMetadata
	Motifs        string
}

type ExportQueryMetadata struct {
	Index                 int
	LabelName             string
	PhgoAliases           string
	GeneID                string
	ProteinID             string
	TranscriptID          string
	SourceDatabase        string
	SourceProteomeID      int
	SourceJBrowseName     string
	SourceGenomeLabel     string
	OriginalInputURL      string
	NormalizedURL         string
	OrganismShort         string
	Annotation            string
	SequenceLength        int
	Sequence              string
	ExternalReferenceData *BlastResultRow `json:"external_reference_data,omitempty"`
}

type KeywordResultRow struct {
	SourceDatabase      string
	SearchTerm          string
	SearchType          string
	LabelName           string
	LabelNameType       string
	PhgoAliases         string
	ProteinID           string
	TranscriptID        string
	GeneLocus           string
	GeneIdentifier      string
	Genome              string
	Location            string
	Aliases             string
	Symbols             string
	Synonyms            string
	UniProt             string
	Description         string
	Comments            string
	AutoDefine          string
	GeneReportURL       string
	SequenceHeaderLabel string
	SequenceID          string
	ExtraColumns        map[string]string
}

type KeywordSearchGroup struct {
	SearchTerm       string
	SearchType       string
	LabelName        string
	LabelMethod      string
	LabelSourceField string
	LabelSourceValue string
	SearchStartedAt  time.Time
	SearchEndedAt    time.Time
	SearchDurationMS int64
	Rows             []KeywordResultRow
}

type QuerySequenceSource struct {
	Sequence              string
	ProteinSequence       string
	NucleotideSequence    string
	SequenceKind          SequenceKind
	PreferredSequenceID   string
	OriginalInputURL      string
	NormalizedURL         string
	SourceDatabase        string
	SourceProteomeID      int
	SourceJBrowseName     string
	SourceGenomeLabel     string
	LabelName             string
	PhgoAliases           string
	Aliases               string
	Symbols               string
	Synonyms              string
	AutoDefine            string
	UniProtAccession      string
	GeneID                string
	TranscriptID          string
	ProteinID             string
	OrganismShort         string
	Annotation            string
	BlastSourceLabelName  string
	BlastSourceGeneID     string
	PhgoRowNumber         int
	PhgoHasRowNumber      bool
	PhgoBlastQuerySource  bool
	PhgoCanvasRawRow      int
	PhgoCanvasHasRawRow   bool
	PhgoCanvasTitle       string
	PhgoCanvasDisplay     string
	ExternalReferenceData *BlastResultRow `json:"external_reference_data,omitempty"`
}

type CanvasKind string

const (
	CanvasKindKeyword CanvasKind = "keyword"
	CanvasKindBlast   CanvasKind = "blast"
	CanvasKindFasta   CanvasKind = "fasta"
)

type CanvasRow struct {
	RowNumber         int                  `json:"row_number"`
	Kind              CanvasKind           `json:"kind"`
	DisplayName       string               `json:"display_name,omitempty"`
	DisplayNameLocked bool                 `json:"display_name_locked,omitempty"`
	KeywordRow        *KeywordResultRow    `json:"keyword_row,omitempty"`
	BlastRow          *BlastResultRow      `json:"blast_row,omitempty"`
	FASTA             *QuerySequenceSource `json:"fasta,omitempty"`
	SequenceData      *ProteinSequenceData `json:"sequence_data,omitempty"`
	SequenceReady     *bool                `json:"sequence_ready,omitempty"`
}

type CanvasColumn struct {
	ID     string `json:"id"`
	Header string `json:"header"`
}

type CanvasItem struct {
	Title         string         `json:"title"`
	Subtitle      string         `json:"subtitle"`
	Kind          CanvasKind     `json:"kind"`
	Rows          []CanvasRow    `json:"rows"`
	Selected      []bool         `json:"selected,omitempty"`
	MSAFlags      []bool         `json:"msa_flags,omitempty"`
	SourceLabel   string         `json:"source_label,omitempty"`
	ImportedFrom  string         `json:"imported_from,omitempty"`
	ActiveColumns []CanvasColumn `json:"active_columns,omitempty"`
}

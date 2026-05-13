// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package model

import "time"

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
}

type ExportQueryMetadata struct {
	Index             int
	LabelName         string
	GeneID            string
	ProteinID         string
	TranscriptID      string
	SourceDatabase    string
	SourceProteomeID  int
	SourceJBrowseName string
	SourceGenomeLabel string
	OriginalInputURL  string
	NormalizedURL     string
	OrganismShort     string
	Annotation        string
	SequenceLength    int
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
	Sequence            string
	ProteinSequence     string
	NucleotideSequence  string
	SequenceKind        SequenceKind
	PreferredSequenceID string
	OriginalInputURL    string
	NormalizedURL       string
	SourceDatabase      string
	SourceProteomeID    int
	SourceJBrowseName   string
	SourceGenomeLabel   string
	LabelName           string
	PhgoAliases         string
	Aliases             string
	Symbols             string
	Synonyms            string
	AutoDefine          string
	UniProtAccession    string
	GeneID              string
	TranscriptID        string
	ProteinID           string
	OrganismShort       string
	Annotation          string
}

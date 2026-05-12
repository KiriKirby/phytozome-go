// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package model

import "time"

type ProteinSequenceRecord struct {
	Header   string
	Sequence string
}

type ExportMetadata struct {
	GeneName      string
	GeneID        string
	GeneReportURL string
}

type KeywordResultRow struct {
	SourceDatabase      string
	SearchTerm          string
	SearchType          string
	LabelName           string
	ProteinID           string
	TranscriptID        string
	GeneIdentifier      string
	Genome              string
	Location            string
	Aliases             string
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
	Sequence          string
	OriginalInputURL  string
	NormalizedURL     string
	SourceDatabase    string
	SourceProteomeID  int
	SourceJBrowseName string
	SourceGenomeLabel string
	LabelName         string
	Aliases           string
	AutoDefine        string
	UniProtAccession  string
	GeneID            string
	TranscriptID      string
	ProteinID         string
	OrganismShort     string
	Annotation        string
}

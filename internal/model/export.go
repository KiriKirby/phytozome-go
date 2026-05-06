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
	GeneID            string
	TranscriptID      string
	ProteinID         string
	OrganismShort     string
	Annotation        string
}

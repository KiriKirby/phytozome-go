package model

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
	SearchTerm          string
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
}

type KeywordSearchGroup struct {
	SearchTerm string
	Rows       []KeywordResultRow
}

type QuerySequenceSource struct {
	Sequence         string
	OriginalInputURL string
	NormalizedURL    string
	GeneID           string
	TranscriptID     string
	ProteinID        string
	OrganismShort    string
	Annotation       string
}

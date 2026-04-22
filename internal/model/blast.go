package model

type SequenceKind string

const (
	SequenceDNA     SequenceKind = "dna"
	SequenceProtein SequenceKind = "protein"
)

type BlastRequest struct {
	Species          SpeciesCandidate
	Sequence         string
	SequenceKind     SequenceKind
	TargetType       string
	Program          string
	EValue           string
	ComparisonMatrix string
	WordLength       string
	AlignmentsToShow int
	AllowGaps        bool
	FilterQuery      bool
}

type BlastJob struct {
	JobID   string
	Message string
}

type BlastResult struct {
	JobID       string
	Message     string
	UserOptions string
	RawXML      string
	Hash        string
	ZUID        string
	Rows        []BlastResultRow
}

type BlastResultRow struct {
	HitNumber       int
	HSPNumber       int
	Protein         string
	Species         string
	EValue          string
	PercentIdentity float64
	AlignLength     int
	Strands         string
	QueryID         string
	QueryFrom       int
	QueryTo         int
	TargetFrom      int
	TargetTo        int
	Bitscore        float64
	Identical       int
	Positives       int
	Gaps            int
	QueryLength     int
	TargetLength    int
	GeneReportURL   string
	JBrowseName     string
	TargetID        int
	SequenceID      string
	TranscriptID    string
	Defline         string
}

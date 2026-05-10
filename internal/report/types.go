package report

import (
	"fmt"
	"strings"
	"time"
)

type ReportData struct {
	Title       string
	Mode        string
	GeneratedAt time.Time
	Software    SoftwareInfo
	UserSession UserSessionInfo
	System      SystemInfo
	TimeWindow  TimeWindow
	Files       []GeneratedFile
	Keyword     KeywordReportData
	Blast       BlastReportData
}

type SoftwareInfo struct {
	Name       string
	Author     string
	Repository string
	Version    string
	GoVersion  string
}

type UserSessionInfo struct {
	UserName       string
	HomeDir        string
	SessionName    string
	HostName       string
	ProcessID      int
	ExecutablePath string
	WorkingDir     string
	AppDir         string
	OutputDir      string
	CacheDir       string
	Terminal       string
	TerminalDetail string
}

type SystemInfo struct {
	OS           string
	OSVersion    string
	Architecture string
	CPUCount     int
	Memory       string
}

type TimeWindow struct {
	QueryStart     time.Time
	SearchEnd      time.Time
	ReviewStart    time.Time
	ExportStart    time.Time
	ExportEnd      time.Time
	ReportRendered time.Time
}

type GeneratedFile struct {
	Name         string
	Type         string
	Role         string
	Path         string
	SizeBytes    int64
	CreatedAt    time.Time
	ModifiedAt   time.Time
	AccessedAt   time.Time
	Permissions  string
	Owner        string
	SHA256       string
	SHA1         string
	MD5          string
	HashCaptured time.Time
}

type KeywordReportData struct {
	Database           string
	Species            SpeciesReport
	SearchTerms        []KeywordTermReport
	LabelTraces        []KeywordLabelReport
	Selection          KeywordSelectionStats
	Provenance         []ProvenanceSlice
	ColumnCompleteness []ColumnCompleteness
	QualityChecks      []QualityCheck
	Columns            []ColumnLineage
	ExportSettings     []NameValue
	GenerationSteps    []GenerationStep
	Sequences          SequenceAudit
}

type SpeciesReport struct {
	DisplayLabel string
	GenomeLabel  string
	CommonName   string
	SearchAlias  string
	JBrowseName  string
	ProteomeID   string
	ReleaseDate  string
	IsOfficial   string
	SourceNotes  string
}

type KeywordTermReport struct {
	SearchTerm     string
	InputType      string
	SearchType     string
	QueryOrder     int
	TotalRows      int
	SelectedRows   int
	LabelName      string
	MatchingNotes  string
	DurationMillis int64
}

type KeywordLabelReport struct {
	SearchTerm  string
	FinalLabel  string
	SourceField string
	SourceValue string
	Method      string
	Explanation string
}

type KeywordSelectionStats struct {
	TotalRows      int
	SelectedRows   int
	UnselectedRows int
	SearchTerms    int
	TermsWithHits  int
	TermsZeroHits  int
	GeneratedFiles int
}

type BlastReportData struct {
	Database           string
	Species            SpeciesReport
	Execution          BlastExecutionReport
	Inputs             []BlastInputTrace
	Runs               []BlastRunReport
	Selection          BlastSelectionStats
	ExternalReferences ExternalReferenceReport
	Family             *FamilyBlastReport
	Filter             *BlastFilterReport
	Provenance         []ProvenanceSlice
	ColumnCompleteness []ColumnCompleteness
	QualityChecks      []QualityCheck
	Columns            []ColumnLineage
	ExportSettings     []NameValue
	GenerationSteps    []GenerationStep
	Sequences          SequenceAudit
}

type BlastExecutionReport struct {
	Program          string
	ExecutionMode    string
	QueryKind        string
	TargetType       string
	EValue           string
	ComparisonMatrix string
	WordLength       string
	AlignmentsToShow int
	AllowGaps        string
	FilterQuery      string
	ServerCapability string
	LocalCapability  string
	Fallback         string
	Notes            string
}

type BlastInputTrace struct {
	Order          int
	RawPreview     string
	InputType      string
	ParserPath     string
	Source         string
	SequenceLength int
	LabelName      string
	GeneID         string
	TranscriptID   string
	ProteinID      string
	OriginalURL    string
	NormalizedURL  string
	Outcome        string
	Notes          string
}

type BlastRunReport struct {
	RunIndex      int
	Label         string
	FamilyName    string
	Program       string
	ExecutionMode string
	JobID         string
	RowCount      int
	SelectedRows  int
	TopHit        string
	BestEValue    string
	BestIdentity  string
	Message       string
	ResultHash    string
	ZUID          string
}

type BlastSelectionStats struct {
	ParsedQueries     int
	ResolvedQueries   int
	ExecutedRuns      int
	ExportedRuns      int
	ZeroHitRuns       int
	TotalRows         int
	SelectedRows      int
	UnselectedRows    int
	RowsWithURL       int
	RowsWithSequence  int
	RowsWithTargetLen int
	RowsWithCoverage  int
	RowsWithEValue    int
	RowsWithIdentity  int
	GeneratedFiles    int
}

type ExternalReferenceReport struct {
	UniProtEnabled  bool
	InterProEnabled bool
	UniProt         UniProtReferenceReport
	InterPro        InterProReferenceReport
}

type UniProtReferenceReport struct {
	LookupSummary []NameValue
	Outcome       []NameValue
	Rows          []UniProtRowSummary
}

type UniProtRowSummary struct {
	Row            int
	Label          string
	Family         string
	Target         string
	Accession      string
	Reviewed       string
	FamilySupport  string
	FamilySemantic string
	LengthRatio    string
	Fragment       string
	Caution        string
	Annotation     string
}

type InterProReferenceReport struct {
	Settings     []NameValue
	Outcome      []NameValue
	StatusCounts []NameValue
}

type FamilyBlastReport struct {
	Settings     []NameValue
	Groups       []FamilyBlastGroupReport
	MergeRecords []FamilyMergeRecord
}

type FamilyBlastGroupReport struct {
	Name           string
	MemberLabels   []string
	GroupSource    string
	DetectionRule  string
	OriginalRuns   int
	RowsBefore     int
	RowsAfter      int
	OutputBaseName string
}

type FamilyMergeRecord struct {
	Family       string
	TargetKey    string
	MemberRows   string
	ChosenRow    string
	Reason       string
	ScoreDetails string
}

type BlastFilterReport struct {
	Applied              bool
	Cleared              bool
	RecommendedKeep      int
	RecommendedRemove    int
	FinalSelected        int
	FinalUnselected      int
	UserRescued          int
	UserRemovedAfterKeep int
	Settings             []BlastFilterSettingDetail
	Formulas             []NameValue
	Totals               BlastFilterTotals
	QuerySummaries       []BlastFilterQuerySummary
	HardRuleSummaries    []BlastFilterRuleSummary
	Rows                 []BlastFilterRowSummary
}

type BlastFilterTotals struct {
	QueryCount        int
	TotalRows         int
	RecommendedKeep   int
	RecommendedRemove int
	FinalSelected     int
	FinalUnselected   int
	UserRescued       int
	UserRemoved       int
	MatchedRows       int
	AgreementPercent  string
	DifferenceRows    int
}

type BlastFilterRowSummary struct {
	Row             int
	Query           string
	Label           string
	Family          string
	Target          string
	Identity        string
	Coverage        string
	EValue          string
	LengthRatio     string
	FamilySupport   string
	FamilySemantic  string
	UniProtEvidence string
	InterProStatus  string
	Recommended     string
	FinalSelection  string
	Difference      string
	ScoreComponents string
	HardFailures    string
}

type BlastFilterQuerySummary struct {
	Query             string
	Family            string
	TotalRows         int
	RecommendedKeep   int
	RecommendedRemove int
	FinalSelected     int
	FinalUnselected   int
	UserRescued       int
	UserRemoved       int
	MatchedRows       int
	Difference        int
	AgreementPercent  string
}

type BlastFilterSettingDetail struct {
	Name    string
	Value   string
	Default string
	Meaning string
	Effect  string
	Group   string
}

type BlastFilterRuleSummary struct {
	Name        string
	Result      string
	Passed      int
	Failed      int
	Rule        string
	Explanation string
	Source      string
}

type ProvenanceSlice struct {
	Label       string
	Count       int
	Explanation string
}

type QualityCheck struct {
	Name        string
	Result      string
	Count       string
	Rule        string
	Explanation string
	Source      string
}

type ColumnCompleteness struct {
	Column      string
	FilledRows  int
	EmptyRows   int
	TotalRows   int
	FilledRatio string
	Source      string
}

type ColumnLineage struct {
	ID               string
	Column           string
	Meaning          string
	EnglishDetail    string
	ChineseDetail    string
	JapaneseDetail   string
	Source           string
	CollectionMethod string
	BlankMeaning     string
	UsedInStats      string
}

type NameValue struct {
	Name        string
	Value       string
	Explanation string
}

type GenerationStep struct {
	Name       string
	Start      time.Time
	End        time.Time
	Status     string
	Details    string
	DurationMS int64
}

type SequenceAudit struct {
	Requested       bool
	RequestedCount  int
	WrittenCount    int
	SkippedCount    int
	TotalCharacters int
	TextFileType    string
	HeaderLabelMode string
	Records         []SequenceRecord
	QuerySummaries  []SequenceQuerySummary
}

type SequenceRecord struct {
	Row        int
	SearchTerm string
	Label      string
	SequenceID string
	Transcript string
	Status     string
	Length     int
	Source     string
}

type SequenceQuerySummary struct {
	QueryLabel     string
	QueryKind      string
	RequestedCount int
	WrittenCount   int
	SkippedCount   int
	AverageLength  int
	MinLength      int
	MaxLength      int
	TotalLength    int
	SourceSummary  string
}

func ReportFileName(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return fmt.Sprintf("%s_rpt.pdf", t.Local().Format("20060102_150405"))
}

func ReportFileNameForBase(baseName string, t time.Time) string {
	baseName = sanitizeReportFileBase(baseName)
	if baseName == "" {
		return ReportFileName(t)
	}
	return fmt.Sprintf("%s_rpt.pdf", baseName)
}

func sanitizeReportFileBase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"<", "_", ">", "_", ":", "_", `"`, "_", "/", "_", "\\", "_", "|", "_", "?", "_", "*", "_",
	)
	value = replacer.Replace(value)
	value = strings.Trim(value, ". ")
	if value == "" {
		return ""
	}
	return value
}

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
	RequestProfile   string
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
	SourceDatabase                      string
	BlastProgram                        string
	LabelName                           string
	FamilyName                          string
	FamilyConsensusSupport              int
	FamilyConsensusSize                 int
	FamilyConsensusCoveragePercent      string
	FamilyConsensusPrimaryLabel         string
	FamilyMemberLabels                  string
	FamilySemanticTokens                string
	FamilySemanticAliasTokens           string
	FamilySemanticAnnotationMatchCount  int
	FamilySemanticAnnotationMatchTokens string
	FamilySemanticAgreementPercent      string
	HitNumber                           int
	HSPNumber                           int
	Protein                             string
	SubjectID                           string
	Species                             string
	EValue                              string
	PercentIdentity                     float64
	UniProtReferenceEnabled             bool
	UniProtAccession                    string
	UniProtReviewed                     string
	UniProtProteinName                  string
	UniProtGeneNames                    string
	UniProtKeywords                     string
	UniProtEC                           string
	UniProtGO                           string
	TargetUniProtCanonicalLengthPercent string
	AlignQueryLengthPercent             float64
	InterProReferenceEnabled            bool
	InterProConservedRegionStatus       string
	PfamDomain                          string
	AlignLength                         int
	Strands                             string
	QueryID                             string
	QueryFrom                           int
	QueryTo                             int
	TargetFrom                          int
	TargetTo                            int
	Bitscore                            float64
	Mismatches                          int
	GapOpenings                         int
	Identical                           int
	Positives                           int
	Gaps                                int
	QueryLength                         int
	TargetLength                        int
	UniProtCanonicalLength              string
	UniProtEntryName                    string
	UniProtOrganism                     string
	UniProtOrganismID                   string
	UniProtFunction                     string
	UniProtCatalyticActivity            string
	UniProtGOIDs                        string
	UniProtPathway                      string
	UniProtSubcellularLocation          string
	UniProtProteinExistence             string
	UniProtAnnotationScore              string
	UniProtFragment                     string
	UniProtSequenceCaution              string
	UniProtPfam                         string
	UniProtInterPro                     string
	UniProtDomain                       string
	UniProtRegion                       string
	UniProtMotif                        string
	UniProtActiveSite                   string
	UniProtBindingSite                  string
	UniProtAlphaFoldDB                  string
	UniProtPDB                          string
	InterProEntryName                   string
	InterProEntryType                   string
	InterProCoveragePercent             string
	InterProMatchRegions                string
	InterProAccessions                  string
	InterProSignatureAccessions         string
	InterProPfamAccessions              string
	GeneReportURL                       string
	JBrowseName                         string
	TargetID                            int
	SequenceID                          string
	TranscriptID                        string
	Defline                             string
}

type InterProConservedRegionSettings struct {
	UsePfamAccession       bool
	UseInterProAccession   bool
	UseSignatureAccession  bool
	UseEntryType           bool
	UseEntryName           bool
	UseCoverage            bool
	UseMatchRegions        bool
	PresentMinCoverage     float64
	PartialMinCoverage     float64
	PresentMinMatchedItems int
	PartialMinMatchedItems int
}

func DefaultInterProConservedRegionSettings() InterProConservedRegionSettings {
	return InterProConservedRegionSettings{
		UsePfamAccession:       true,
		UseInterProAccession:   true,
		UseSignatureAccession:  true,
		UseEntryType:           true,
		UseEntryName:           false,
		UseCoverage:            true,
		UseMatchRegions:        true,
		PresentMinCoverage:     70,
		PartialMinCoverage:     25,
		PresentMinMatchedItems: 1,
		PartialMinMatchedItems: 1,
	}
}

type BlastFilterSettings struct {
	MinIdentityPercent                        float64
	MinAlignQueryCoveragePercent              float64
	MaxEValue                                 float64
	UseTargetCanonicalLengthRatio             bool
	RequireTargetCanonicalLengthRatio         bool
	MinTargetCanonicalLengthPercent           float64
	MaxTargetCanonicalLengthPercent           float64
	UseTargetQueryLengthRatio                 bool
	RequireTargetQueryLengthRatio             bool
	MinTargetQueryLengthPercent               float64
	MaxTargetQueryLengthPercent               float64
	RequireUniProtAccession                   bool
	PreferUniProtReviewed                     bool
	RejectUniProtFragments                    bool
	RejectUniProtSequenceCautions             bool
	InterProDomainMode                        string
	RequireInterProConservedRegion            bool
	AllowInterProPartial                      bool
	RejectInterProMissing                     bool
	RejectInterProUncertain                   bool
	MinInterProCoveragePercent                float64
	RequireInterProCoverageWhenUsed           bool
	AllowStrongBlastFallbackWithoutReferences bool
	StrongBlastFallbackMinIdentityPercent     float64
	StrongBlastFallbackMaxEValue              float64
	StrongBlastFallbackMinTargetQueryPercent  float64
	StrongBlastFallbackMaxTargetQueryPercent  float64
	RequireFamilyConsensusForStrongFallback   bool
	StrongFallbackMinFamilyConsensusSupport   int
	StrongFallbackMinFamilyConsensusPercent   float64
	UseFamilySemanticAgreement                bool
	RequireFamilySemanticAgreement            bool
	FamilySemanticMinTokenMatches             int
	FamilySemanticMinAgreementPercent         float64
	FamilySemanticAllowStrongReferenceBypass  bool
	KeepBestIsoformPerTargetGene              bool
	KeepTopHitsPerQuery                       bool
	TopHitsPerQuery                           int
	RankingTieBreakerOrder                    string
	PreferHigherFilterScoreWhenRanking        bool
	PreferLowerEValueWhenTies                 bool
	PreferHigherIdentityWhenTies              bool
	PreferHigherCoverageWhenTies              bool
	PreferHigherReferenceScoreWhenTies        bool
	PreferHigherBitscoreWhenTies              bool
	RejectIfAnyHardRuleFails                  bool
	EnableSoftScore                           bool
	MinSoftScore                              int
	IdentityWeight                            int
	CoverageWeight                            int
	LengthRatioWeight                         int
	TargetQueryLengthWeight                   int
	InterProWeight                            int
	InterProPartialWeight                     int
	InterProCoverageWeight                    int
	UniProtReviewedWeight                     int
	UniProtAnnotationWeight                   int
	FamilySemanticAgreementWeight             int
	PenaltySequenceCaution                    int
	PenaltyFragment                           int
	InterProPresentReferenceScore             int
	InterProPartialReferenceScore             int
	InterProUncertainReferenceScore           int
	InterProMissingReferencePenalty           int
	InterProCoverageReferenceDivisor          int
	UniProtAccessionReferenceScore            int
	UniProtReviewedReferenceScore             int
	UniProtAnnotationReferenceScore           int
	FamilySemanticReferenceScore              int
	FragmentReferencePenaltyMultiplier        int
	SequenceCautionReferencePenaltyMultiplier int
	LengthNearDistancePercent                 float64
	LengthNearReferenceScore                  int
	LengthAcceptableDistancePercent           float64
	LengthAcceptableReferenceScore            int
	LengthFarDistancePercent                  float64
	LengthFarReferencePenalty                 int
}

type FamilyBlastSettings struct {
	Enabled                    bool
	GroupByDetectedPrefix      bool
	MergeRowsByTarget          bool
	KeepBestHitPerTarget       bool
	PrependOnlyFirstQuery      bool
	CustomizeGroups            bool
	MinimumGroupSize           int
	StripArabidopsisPrefix     bool
	StripLeadingSpeciesPrefix  bool
	StripTrailingQueryIndex    bool
	StripAfterNumberSuffix     bool
	NormalizeInnerPunctuation  bool
	StripTerminalSubtypeSuffix bool
	KeepDistinctQuerySubgroups bool
	UseUniProtReference        bool
	UseInterProReference       bool
	RankingTieBreakerOrder     string
}

func DefaultFamilyBlastSettings() FamilyBlastSettings {
	return FamilyBlastSettings{
		Enabled:                    true,
		GroupByDetectedPrefix:      true,
		MergeRowsByTarget:          true,
		KeepBestHitPerTarget:       true,
		PrependOnlyFirstQuery:      true,
		CustomizeGroups:            false,
		MinimumGroupSize:           2,
		StripArabidopsisPrefix:     false,
		StripLeadingSpeciesPrefix:  true,
		StripTrailingQueryIndex:    true,
		StripAfterNumberSuffix:     true,
		NormalizeInnerPunctuation:  true,
		StripTerminalSubtypeSuffix: true,
		KeepDistinctQuerySubgroups: false,
		RankingTieBreakerOrder:     "reference,evalue,identity,coverage,bitscore",
	}
}

func DefaultBlastFilterSettings() BlastFilterSettings {
	return BlastFilterSettings{
		MinIdentityPercent:                        0,
		MinAlignQueryCoveragePercent:              0,
		MaxEValue:                                 0,
		UseTargetCanonicalLengthRatio:             true,
		RequireTargetCanonicalLengthRatio:         true,
		MinTargetCanonicalLengthPercent:           70,
		MaxTargetCanonicalLengthPercent:           130,
		UseTargetQueryLengthRatio:                 false,
		RequireTargetQueryLengthRatio:             false,
		MinTargetQueryLengthPercent:               60,
		MaxTargetQueryLengthPercent:               160,
		RequireUniProtAccession:                   false,
		PreferUniProtReviewed:                     true,
		RejectUniProtFragments:                    false,
		RejectUniProtSequenceCautions:             false,
		InterProDomainMode:                        "conserved_region",
		RequireInterProConservedRegion:            true,
		AllowInterProPartial:                      true,
		RejectInterProMissing:                     true,
		RejectInterProUncertain:                   true,
		MinInterProCoveragePercent:                0,
		RequireInterProCoverageWhenUsed:           false,
		AllowStrongBlastFallbackWithoutReferences: true,
		StrongBlastFallbackMinIdentityPercent:     40,
		StrongBlastFallbackMaxEValue:              1e-80,
		StrongBlastFallbackMinTargetQueryPercent:  80,
		StrongBlastFallbackMaxTargetQueryPercent:  120,
		RequireFamilyConsensusForStrongFallback:   false,
		StrongFallbackMinFamilyConsensusSupport:   2,
		StrongFallbackMinFamilyConsensusPercent:   35,
		UseFamilySemanticAgreement:                true,
		RequireFamilySemanticAgreement:            false,
		FamilySemanticMinTokenMatches:             1,
		FamilySemanticMinAgreementPercent:         20,
		FamilySemanticAllowStrongReferenceBypass:  true,
		KeepBestIsoformPerTargetGene:              true,
		KeepTopHitsPerQuery:                       false,
		TopHitsPerQuery:                           10,
		RankingTieBreakerOrder:                    "score,identity,coverage,reference,evalue,bitscore",
		PreferHigherFilterScoreWhenRanking:        true,
		PreferLowerEValueWhenTies:                 true,
		PreferHigherIdentityWhenTies:              true,
		PreferHigherCoverageWhenTies:              true,
		PreferHigherReferenceScoreWhenTies:        true,
		PreferHigherBitscoreWhenTies:              true,
		RejectIfAnyHardRuleFails:                  true,
		EnableSoftScore:                           false,
		MinSoftScore:                              5,
		IdentityWeight:                            2,
		CoverageWeight:                            2,
		LengthRatioWeight:                         2,
		TargetQueryLengthWeight:                   2,
		InterProWeight:                            3,
		InterProPartialWeight:                     1,
		InterProCoverageWeight:                    1,
		UniProtReviewedWeight:                     1,
		UniProtAnnotationWeight:                   1,
		FamilySemanticAgreementWeight:             2,
		PenaltySequenceCaution:                    2,
		PenaltyFragment:                           3,
		InterProPresentReferenceScore:             80,
		InterProPartialReferenceScore:             35,
		InterProUncertainReferenceScore:           5,
		InterProMissingReferencePenalty:           80,
		InterProCoverageReferenceDivisor:          10,
		UniProtAccessionReferenceScore:            25,
		UniProtReviewedReferenceScore:             25,
		UniProtAnnotationReferenceScore:           10,
		FamilySemanticReferenceScore:              20,
		FragmentReferencePenaltyMultiplier:        10,
		SequenceCautionReferencePenaltyMultiplier: 5,
		LengthNearDistancePercent:                 10,
		LengthNearReferenceScore:                  20,
		LengthAcceptableDistancePercent:           30,
		LengthAcceptableReferenceScore:            8,
		LengthFarDistancePercent:                  60,
		LengthFarReferencePenalty:                 20,
	}
}

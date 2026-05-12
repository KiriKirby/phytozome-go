// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package report

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

func BlastFilterSettingDetails(settings model.BlastFilterSettings) []BlastFilterSettingDetail {
	defaults := model.DefaultBlastFilterSettings()
	out := make([]BlastFilterSettingDetail, 0, 40)
	add := func(group, name, value, def, meaning, effect string) {
		out = append(out, BlastFilterSettingDetail{
			Group:   group,
			Name:    name,
			Value:   value,
			Default: def,
			Meaning: meaning,
			Effect:  effect,
		})
	}
	fv := formatBlastFilterFloat
	fi := strconv.Itoa
	fb := func(v bool) string { return fmt.Sprintf("%t", v) }

	add("Alignment and length rules", "minimum identity (%)", fv(settings.MinIdentityPercent), fv(defaults.MinIdentityPercent), "Minimum BLAST identity required when the identity cutoff is active.", "Rows below this value are suggested for removal when the value is greater than zero.")
	add("Alignment and length rules", "minimum query coverage (%)", fv(settings.MinAlignQueryCoveragePercent), fv(defaults.MinAlignQueryCoveragePercent), "Minimum query coverage required when the query-coverage cutoff is active.", "Rows below this value are suggested for removal when the value is greater than zero.")
	add("Alignment and length rules", "maximum E-value", formatBlastFilterEValue(settings.MaxEValue), formatBlastFilterEValue(defaults.MaxEValue), "Maximum BLAST E-value accepted when the E-value cutoff is active.", "Rows above this value are suggested for removal when the value is greater than zero.")
	add("Alignment and length rules", "Check target length against UniProt canonical length", fb(settings.UseTargetCanonicalLengthRatio), fb(defaults.UseTargetCanonicalLengthRatio), "Whether source target length is compared with UniProt canonical length.", "Enables the UniProt length rule when canonical length data is available.")
	add("Alignment and length rules", "Reject rows missing canonical-length data", fb(settings.RequireTargetCanonicalLengthRatio), fb(defaults.RequireTargetCanonicalLengthRatio), "Whether missing UniProt canonical-length ratio fails the rule.", "Missing ratio can suggest removal instead of being treated as unknown.")
	add("Alignment and length rules", "min target/UniProt length (%)", fv(settings.MinTargetCanonicalLengthPercent), fv(defaults.MinTargetCanonicalLengthPercent), "Lower bound for target length divided by UniProt canonical length.", "Rows shorter than this bound are suggested for removal when the rule is active.")
	add("Alignment and length rules", "max target/UniProt length (%)", fv(settings.MaxTargetCanonicalLengthPercent), fv(defaults.MaxTargetCanonicalLengthPercent), "Upper bound for target length divided by UniProt canonical length.", "Rows longer than this bound are suggested for removal when the rule is active.")
	add("Alignment and length rules", "Check target length against query length", fb(settings.UseTargetQueryLengthRatio), fb(defaults.UseTargetQueryLengthRatio), "Whether target length is compared with query length.", "Enables a length sanity rule that does not require UniProt.")
	add("Alignment and length rules", "Reject rows missing target/query length data", fb(settings.RequireTargetQueryLengthRatio), fb(defaults.RequireTargetQueryLengthRatio), "Whether missing target/query length data fails the rule.", "Missing target or query length can suggest removal when enabled.")
	add("Alignment and length rules", "min target/query length (%)", fv(settings.MinTargetQueryLengthPercent), fv(defaults.MinTargetQueryLengthPercent), "Lower bound for target length divided by query length.", "Rows shorter than this bound are suggested for removal when the rule is active.")
	add("Alignment and length rules", "max target/query length (%)", fv(settings.MaxTargetQueryLengthPercent), fv(defaults.MaxTargetQueryLengthPercent), "Upper bound for target length divided by query length.", "Rows longer than this bound are suggested for removal when the rule is active.")

	add("External-reference rules", "Reject rows without a UniProt accession", fb(settings.RequireUniProtAccession), fb(defaults.RequireUniProtAccession), "Whether UniProt mapping is mandatory.", "Rows without a UniProt accession can be suggested for removal.")
	add("External-reference rules", "Prefer reviewed UniProt records", fb(settings.PreferUniProtReviewed), fb(defaults.PreferUniProtReviewed), "Whether reviewed UniProt records receive ranking support.", "Reviewed rows rank higher but are not required solely by this setting.")
	add("External-reference rules", "Reject UniProt fragment records", fb(settings.RejectUniProtFragments), fb(defaults.RejectUniProtFragments), "Whether UniProt fragment records are rejected.", "Fragment-marked rows are suggested for removal when enabled.")
	add("External-reference rules", "Reject UniProt sequence cautions", fb(settings.RejectUniProtSequenceCautions), fb(defaults.RejectUniProtSequenceCautions), "Whether UniProt sequence-caution rows are rejected.", "Rows with sequence-caution annotations are suggested for removal when enabled.")
	add("External-reference rules", "InterPro rule", interProDomainModeLabel(settings.InterProDomainMode), interProDomainModeLabel(defaults.InterProDomainMode), "How the filter uses InterPro domain evidence.", "Chooses conserved-region status, any domain evidence, family consensus domain, or no InterPro domain gate.")
	add("External-reference rules", "Require InterPro conserved-region support", fb(settings.RequireInterProConservedRegion), fb(defaults.RequireInterProConservedRegion), "Whether acceptable InterPro status is required.", "Rows without acceptable InterPro evidence can be suggested for removal.")
	add("External-reference rules", "Allow InterPro partial status", fb(settings.AllowInterProPartial), fb(defaults.AllowInterProPartial), "Whether partial InterPro status can pass.", "Partial rows can remain checked instead of being removed outright.")
	add("External-reference rules", "Reject InterPro status: missing", fb(settings.RejectInterProMissing), fb(defaults.RejectInterProMissing), "Whether missing InterPro status fails.", "Rows with missing conserved-region evidence can be suggested for removal.")
	add("External-reference rules", "Reject InterPro status: uncertain", fb(settings.RejectInterProUncertain), fb(defaults.RejectInterProUncertain), "Whether uncertain InterPro status fails.", "Rows with uncertain conserved-region evidence can be suggested for removal.")
	add("External-reference rules", "minimum InterPro coverage (%)", fv(settings.MinInterProCoveragePercent), fv(defaults.MinInterProCoveragePercent), "Minimum InterPro coverage when the coverage cutoff is active.", "Rows below this coverage can be suggested for removal.")
	add("External-reference rules", "Reject rows missing InterPro coverage when cutoff is used", fb(settings.RequireInterProCoverageWhenUsed), fb(defaults.RequireInterProCoverageWhenUsed), "Whether coverage becomes mandatory once the coverage cutoff is used.", "Missing coverage can suggest removal when coverage checking is active.")
	add("External-reference rules", "Let very strong BLAST evidence rescue weak references", fb(settings.AllowStrongBlastFallbackWithoutReferences), fb(defaults.AllowStrongBlastFallbackWithoutReferences), "Whether strong BLAST rows may survive weak or missing references.", "Protects globally strong rows from being removed only because reference anchors are absent.")
	add("External-reference rules", "fallback minimum identity (%)", fv(settings.StrongBlastFallbackMinIdentityPercent), fv(defaults.StrongBlastFallbackMinIdentityPercent), "Minimum identity needed for fallback rescue.", "Rows below this identity cannot use fallback rescue.")
	add("External-reference rules", "fallback maximum E-value", formatBlastFilterEValue(settings.StrongBlastFallbackMaxEValue), formatBlastFilterEValue(defaults.StrongBlastFallbackMaxEValue), "Maximum E-value allowed for fallback rescue.", "Rows above this E-value cannot use fallback rescue.")
	add("External-reference rules", "fallback min target/query (%)", fv(settings.StrongBlastFallbackMinTargetQueryPercent), fv(defaults.StrongBlastFallbackMinTargetQueryPercent), "Minimum target/query length ratio for fallback rescue.", "Rows shorter than this ratio cannot use fallback rescue.")
	add("External-reference rules", "fallback max target/query (%)", fv(settings.StrongBlastFallbackMaxTargetQueryPercent), fv(defaults.StrongBlastFallbackMaxTargetQueryPercent), "Maximum target/query length ratio for fallback rescue.", "Rows longer than this ratio cannot use fallback rescue.")
	add("External-reference rules", "Require family-member support for fallback rescue", fb(settings.RequireFamilyConsensusForStrongFallback), fb(defaults.RequireFamilyConsensusForStrongFallback), "Whether fallback rescue also needs repeated family-member support.", "Reduces retention of one-off neighbor-family hits.")
	add("External-reference rules", "fallback family members >=", fi(settings.StrongFallbackMinFamilyConsensusSupport), fi(defaults.StrongFallbackMinFamilyConsensusSupport), "Minimum number of family members that must hit the same target for fallback rescue.", "Rows below this support count cannot use fallback rescue when support is required.")
	add("External-reference rules", "fallback family support (%)", fv(settings.StrongFallbackMinFamilyConsensusPercent), fv(defaults.StrongFallbackMinFamilyConsensusPercent), "Minimum percentage of family members that must support the same target for fallback rescue.", "Rows below this support percentage cannot use fallback rescue when support is required.")

	add("Family-name agreement", "Compare query family name with annotations", fb(settings.UseFamilySemanticAgreement), fb(defaults.UseFamilySemanticAgreement), "Whether family/query labels are compared with enrichment annotation text.", "Adds a reusable name-agreement signal for separating neighboring families.")
	add("Family-name agreement", "Reject rows with poor family-name agreement", fb(settings.RequireFamilySemanticAgreement), fb(defaults.RequireFamilySemanticAgreement), "Whether name agreement is a hard requirement.", "Rows with poor family-vs-annotation agreement can be suggested for removal.")
	add("Family-name agreement", "minimum token matches", fi(settings.FamilySemanticMinTokenMatches), fi(defaults.FamilySemanticMinTokenMatches), "Minimum number of normalized family tokens that must appear in annotation text.", "Rows below this token count fail the name-agreement rule.")
	add("Family-name agreement", "minimum agreement (%)", fv(settings.FamilySemanticMinAgreementPercent), fv(defaults.FamilySemanticMinAgreementPercent), "Minimum percentage of family tokens that must match annotation text.", "Rows below this agreement percentage fail when the percentage is used.")
	add("Family-name agreement", "Let strong reference evidence override name mismatch", fb(settings.FamilySemanticAllowStrongReferenceBypass), fb(defaults.FamilySemanticAllowStrongReferenceBypass), "Whether strong reference evidence can bypass text mismatch.", "Protects synonym-heavy families from over-pruning by text matching alone.")
	add("Family-name agreement", "Reject when any enabled hard rule fails", fb(settings.RejectIfAnyHardRuleFails), fb(defaults.RejectIfAnyHardRuleFails), "Whether any active hard-rule failure causes removal.", "This is the main hard-rule gate for automatic row suggestions.")

	add("Ranking, row limits, and scoring", "Keep only the best isoform per target gene", fb(settings.KeepBestIsoformPerTargetGene), fb(defaults.KeepBestIsoformPerTargetGene), "Whether duplicate isoforms per target gene are collapsed to the best-ranked row.", "Prevents multiple isoforms from occupying the export set when one representative is enough.")
	add("Ranking, row limits, and scoring", "Keep only the top-ranked rows per query", fb(settings.KeepTopHitsPerQuery), fb(defaults.KeepTopHitsPerQuery), "Whether only the top N hits are kept per query.", "Limits exported rows per query when enabled.")
	add("Ranking, row limits, and scoring", "rows to keep per query", fi(settings.TopHitsPerQuery), fi(defaults.TopHitsPerQuery), "Maximum number of hits retained for each query when top-hit limiting is enabled.", "Rows beyond this limit are unchecked during ranking.")
	add("Ranking, row limits, and scoring", "ranking priority list", settings.RankingTieBreakerOrder, defaults.RankingTieBreakerOrder, "Priority list used when rows need to be ordered.", "Defines the preference chain for isoform and top-hit limiting.")
	add("Ranking, row limits, and scoring", "Ranking: use the soft filter score first", fb(settings.PreferHigherFilterScoreWhenRanking), fb(defaults.PreferHigherFilterScoreWhenRanking), "Whether higher soft filter score outranks lower scores.", "Rows with stronger scores are ranked ahead of weaker ones.")
	add("Ranking, row limits, and scoring", "Tie break: lower E-value wins", fb(settings.PreferLowerEValueWhenTies), fb(defaults.PreferLowerEValueWhenTies), "Whether lower E-value wins a tie.", "Lower E-value rows rank first when earlier criteria tie.")
	add("Ranking, row limits, and scoring", "Tie break: higher identity wins", fb(settings.PreferHigherIdentityWhenTies), fb(defaults.PreferHigherIdentityWhenTies), "Whether higher identity wins a tie.", "Higher identity rows rank first when earlier criteria tie.")
	add("Ranking, row limits, and scoring", "Tie break: higher query coverage wins", fb(settings.PreferHigherCoverageWhenTies), fb(defaults.PreferHigherCoverageWhenTies), "Whether higher query coverage wins a tie.", "Higher coverage rows rank first when earlier criteria tie.")
	add("Ranking, row limits, and scoring", "Tie break: stronger reference evidence wins", fb(settings.PreferHigherReferenceScoreWhenTies), fb(defaults.PreferHigherReferenceScoreWhenTies), "Whether higher reference score wins a tie.", "Rows with stronger external-reference evidence rank first.")
	add("Ranking, row limits, and scoring", "Tie break: higher BLAST bitscore wins", fb(settings.PreferHigherBitscoreWhenTies), fb(defaults.PreferHigherBitscoreWhenTies), "Whether higher bitscore wins a tie.", "Higher BLAST bitscore is used as a final tie-break.")

	add("Soft-score values", "Enable soft evidence score", fb(settings.EnableSoftScore), fb(defaults.EnableSoftScore), "Whether soft scoring is used as an additional removal criterion.", "When enabled, rows under the minimum soft score are suggested for removal.")
	add("Soft-score values", "minimum soft score", fi(settings.MinSoftScore), fi(defaults.MinSoftScore), "Minimum soft score accepted when soft scoring is active.", "Rows below this threshold are suggested for removal.")
	add("Soft-score values", "identity score weight", fi(settings.IdentityWeight), fi(defaults.IdentityWeight), "Soft-score value for satisfying the identity rule.", "Adds score when identity is strong enough.")
	add("Soft-score values", "query coverage score weight", fi(settings.CoverageWeight), fi(defaults.CoverageWeight), "Soft-score value for satisfying the query-coverage rule.", "Adds score when query coverage is strong enough.")
	add("Soft-score values", "UniProt length score weight", fi(settings.LengthRatioWeight), fi(defaults.LengthRatioWeight), "Soft-score value for satisfying the target/UniProt length rule.", "Adds score when length ratio is inside range.")
	add("Soft-score values", "target/query length score weight", fi(settings.TargetQueryLengthWeight), fi(defaults.TargetQueryLengthWeight), "Soft-score value for satisfying the target/query length rule.", "Adds score when source-side length ratio is inside range.")
	add("Soft-score values", "InterPro present score", fi(settings.InterProWeight), fi(defaults.InterProWeight), "Soft-score value for present InterPro evidence.", "Adds score when conserved-region evidence is present.")
	add("Soft-score values", "InterPro partial score", fi(settings.InterProPartialWeight), fi(defaults.InterProPartialWeight), "Soft-score value for partial InterPro evidence.", "Adds a smaller score when partial evidence is allowed.")
	add("Soft-score values", "InterPro coverage score", fi(settings.InterProCoverageWeight), fi(defaults.InterProCoverageWeight), "Soft-score value for sufficient InterPro coverage.", "Adds score when InterPro coverage reaches the configured threshold.")
	add("Soft-score values", "reviewed UniProt score", fi(settings.UniProtReviewedWeight), fi(defaults.UniProtReviewedWeight), "Soft-score value for reviewed UniProt records.", "Adds score when the UniProt row is reviewed.")
	add("Soft-score values", "annotation richness score", fi(settings.UniProtAnnotationWeight), fi(defaults.UniProtAnnotationWeight), "Soft-score value for usable UniProt annotation text.", "Adds score when enrichment annotations are present.")
	add("Soft-score values", "semantic agreement score", fi(settings.FamilySemanticAgreementWeight), fi(defaults.FamilySemanticAgreementWeight), "Soft-score value for annotation text that agrees with the family/query identity.", "Adds score when family-derived tokens are found in annotation text.")
	add("Soft-score values", "sequence caution penalty", fi(settings.PenaltySequenceCaution), fi(defaults.PenaltySequenceCaution), "Soft-score penalty for UniProt sequence cautions.", "Lowers the score when the sequence is flagged.")
	add("Soft-score values", "fragment record penalty", fi(settings.PenaltyFragment), fi(defaults.PenaltyFragment), "Soft-score penalty for UniProt fragment records.", "Lowers the score for fragment-marked rows.")

	add("Reference-ranking values", "InterPro present rank score", fi(settings.InterProPresentReferenceScore), fi(defaults.InterProPresentReferenceScore), "Reference-ranking score assigned to present InterPro evidence.", "Higher scores strengthen reference-ranked rows.")
	add("Reference-ranking values", "InterPro partial rank score", fi(settings.InterProPartialReferenceScore), fi(defaults.InterProPartialReferenceScore), "Reference-ranking score assigned to partial InterPro evidence.", "Partial evidence contributes less than present evidence.")
	add("Reference-ranking values", "InterPro uncertain rank score", fi(settings.InterProUncertainReferenceScore), fi(defaults.InterProUncertainReferenceScore), "Reference-ranking score assigned to uncertain InterPro evidence.", "Uncertain evidence contributes only a small score.")
	add("Reference-ranking values", "InterPro missing rank penalty", fi(settings.InterProMissingReferencePenalty), fi(defaults.InterProMissingReferencePenalty), "Reference-ranking penalty for missing InterPro evidence.", "Missing evidence lowers the reference score.")
	add("Reference-ranking values", "InterPro coverage score divisor", fi(settings.InterProCoverageReferenceDivisor), fi(defaults.InterProCoverageReferenceDivisor), "Divisor used to convert InterPro coverage into ranking score.", "Larger divisors make coverage contribute more gently.")
	add("Reference-ranking values", "UniProt accession rank score", fi(settings.UniProtAccessionReferenceScore), fi(defaults.UniProtAccessionReferenceScore), "Reference-ranking score assigned when a UniProt accession is available.", "Accessions strengthen the reference score.")
	add("Reference-ranking values", "reviewed UniProt rank score", fi(settings.UniProtReviewedReferenceScore), fi(defaults.UniProtReviewedReferenceScore), "Reference-ranking score assigned to reviewed UniProt records.", "Reviewed accessions strengthen ranking more than unreviewed ones.")
	add("Reference-ranking values", "annotation rank score", fi(settings.UniProtAnnotationReferenceScore), fi(defaults.UniProtAnnotationReferenceScore), "Reference-ranking score assigned for usable UniProt annotation text.", "Annotation text strengthens the reference score.")
	add("Reference-ranking values", "semantic rank score", fi(settings.FamilySemanticReferenceScore), fi(defaults.FamilySemanticReferenceScore), "Reference-ranking score assigned when annotation text agrees with the family.", "Semantic agreement strengthens ranking among otherwise similar rows.")
	add("Reference-ranking values", "fragment rank penalty multiplier", fi(settings.FragmentReferencePenaltyMultiplier), fi(defaults.FragmentReferencePenaltyMultiplier), "Multiplier applied to fragment ranking penalties.", "Fragment evidence reduces the reference score more strongly.")
	add("Reference-ranking values", "caution rank penalty multiplier", fi(settings.SequenceCautionReferencePenaltyMultiplier), fi(defaults.SequenceCautionReferencePenaltyMultiplier), "Multiplier applied to sequence-caution ranking penalties.", "Sequence-caution evidence reduces the reference score.")
	add("Reference-ranking values", "near length distance (%)", fv(settings.LengthNearDistancePercent), fv(defaults.LengthNearDistancePercent), "Distance from 100% length ratio treated as near.", "Near values earn the strongest length-reference score.")
	add("Reference-ranking values", "near length rank score", fi(settings.LengthNearReferenceScore), fi(defaults.LengthNearReferenceScore), "Reference-ranking score when length ratio is near 100%.", "Near-ratio rows receive the highest length-reference score.")
	add("Reference-ranking values", "acceptable length distance (%)", fv(settings.LengthAcceptableDistancePercent), fv(defaults.LengthAcceptableDistancePercent), "Distance from 100% length ratio treated as acceptable.", "Acceptable values receive a moderate length-reference score.")
	add("Reference-ranking values", "acceptable length rank score", fi(settings.LengthAcceptableReferenceScore), fi(defaults.LengthAcceptableReferenceScore), "Reference-ranking score when length ratio is acceptable but not near.", "Acceptable-ratio rows receive a smaller length-reference score.")
	add("Reference-ranking values", "far length distance (%)", fv(settings.LengthFarDistancePercent), fv(defaults.LengthFarDistancePercent), "Distance from 100% length ratio treated as far.", "Far values are penalized rather than rewarded.")
	add("Reference-ranking values", "far length rank penalty", fi(settings.LengthFarReferencePenalty), fi(defaults.LengthFarReferencePenalty), "Reference-ranking penalty when length ratio is far from canonical length.", "Far-ratio rows lose reference score.")

	return out
}

func BlastFilterFormulas(totals BlastFilterTotals, settings model.BlastFilterSettings) []NameValue {
	_ = settings
	matched := totals.MatchedRows
	total := totals.TotalRows
	agreement := totals.AgreementPercent
	if agreement == "" {
		agreement = formatPercent(float64(matched), float64(total))
	}
	return []NameValue{
		{Name: "query_coverage_percent", Value: "AlignQueryLengthPercent if present, otherwise AlignLength / QueryLength * 100", Explanation: "The report uses the captured BLAST coverage field first and only reconstructs it from alignment length and query length when the cached field is missing."},
		{Name: "target_canonical_length_ratio", Value: "TargetLength / UniProtCanonicalLength * 100", Explanation: "This ratio is only meaningful when UniProt canonical length is available in the row."},
		{Name: "target_query_length_ratio", Value: "TargetLength / QueryLength * 100", Explanation: "This ratio compares source target length with query length and does not depend on UniProt enrichment."},
		{Name: "strong_blast_fallback", Value: "AllowStrongBlastFallbackWithoutReferences AND no UniProt accession AND no InterPro status AND identity/E-value/target_query_length_ratio all inside fallback thresholds AND optional family-consensus thresholds satisfied", Explanation: "This fallback is the global escape hatch for strong BLAST rows that lack external-reference anchors, with optional family-level support to avoid over-retaining neighbor-family hits."},
		{Name: "family_semantic_agreement", Value: "normalized family/query tokens found in UniProt/InterPro/annotation text; compared by token count and token-match percentage", Explanation: "This approximates the paper's family/phylogenetic review step using reusable text-based agreement signals instead of family-specific rules."},
		{Name: "remove_by_hard_rules", Value: "RejectIfAnyHardRuleFails AND any hard rule fails", Explanation: "A row is removed automatically when at least one active hard rule fails and the hard-rule gate is enabled."},
		{Name: "remove_by_soft_score", Value: "EnableSoftScore AND score < MinSoftScore", Explanation: "Soft scoring can remove rows only when it is enabled in the settings."},
		{Name: "final_filter_recommendation", Value: "remove_by_hard_rules OR remove_by_soft_score OR removed_by_best_isoform_limit OR removed_by_top_hit_limit", Explanation: "The final recommendation combines hard rules, optional soft scoring, and selection-limiting rules."},
		{Name: "agreement_rate", Value: fmt.Sprintf("%d / %d = %s", matched, total, agreement), Explanation: "This is the share of rows where the automatic recommendation and the final user selection agree."},
		{Name: "difference_rows", Value: fmt.Sprintf("%d", totals.DifferenceRows), Explanation: "Rows that differ between the automatic recommendation and final selection are rescued or manually removed rows."},
	}
}

func formatBlastFilterFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func formatBlastFilterEValue(value float64) string {
	if value == 0 {
		return "0"
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func interProDomainModeLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "any_domain":
		return "accept any domain evidence"
	case "family_consensus_domain":
		return "require family consensus domain"
	case "off":
		return "ignore InterPro domain evidence"
	default:
		return "use conserved-region status"
	}
}

func formatPercent(value float64, total float64) string {
	if total <= 0 {
		return "not available"
	}
	return fmt.Sprintf("%.1f%%", value/total*100)
}

func BlastFilterSettingGroups(details []BlastFilterSettingDetail) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 5)
	for _, detail := range details {
		if !seen[detail.Group] {
			seen[detail.Group] = true
			out = append(out, detail.Group)
		}
	}
	return out
}

func BlastFilterSettingDetailsByGroup(details []BlastFilterSettingDetail, group string) []BlastFilterSettingDetail {
	out := make([]BlastFilterSettingDetail, 0, len(details))
	for _, detail := range details {
		if strings.EqualFold(detail.Group, group) {
			out = append(out, detail)
		}
	}
	return out
}

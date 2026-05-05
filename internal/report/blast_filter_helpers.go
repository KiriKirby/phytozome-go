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

	add("Hard rules", "MinIdentityPercent", fv(settings.MinIdentityPercent), fv(defaults.MinIdentityPercent), "Minimum percent identity that can satisfy the filter without being removed.", "When positive, rows below this threshold are suggested for removal before ranking.")
	add("Hard rules", "MinAlignQueryCoveragePercent", fv(settings.MinAlignQueryCoveragePercent), fv(defaults.MinAlignQueryCoveragePercent), "Minimum query coverage that can satisfy the filter without being removed.", "When positive, rows below this threshold are suggested for removal before ranking.")
	add("Hard rules", "MaxEValue", formatBlastFilterEValue(settings.MaxEValue), formatBlastFilterEValue(defaults.MaxEValue), "Maximum BLAST expectation value accepted by the filter.", "When positive, rows above this threshold are suggested for removal.")
	add("Hard rules", "UseTargetCanonicalLengthRatio", fb(settings.UseTargetCanonicalLengthRatio), fb(defaults.UseTargetCanonicalLengthRatio), "Whether target length is compared with UniProt canonical length.", "Enables the length-ratio rule whenever UniProt canonical length is available.")
	add("Hard rules", "RequireTargetCanonicalLengthRatio", fb(settings.RequireTargetCanonicalLengthRatio), fb(defaults.RequireTargetCanonicalLengthRatio), "Whether missing canonical-length ratio is treated as a failure.", "Missing ratio can remove a row instead of merely hiding the length score.")
	add("Hard rules", "MinTargetCanonicalLengthPercent", fv(settings.MinTargetCanonicalLengthPercent), fv(defaults.MinTargetCanonicalLengthPercent), "Lower bound of the acceptable target/canonical length range.", "Rows shorter than this bound are suggested for removal when the ratio rule is active.")
	add("Hard rules", "MaxTargetCanonicalLengthPercent", fv(settings.MaxTargetCanonicalLengthPercent), fv(defaults.MaxTargetCanonicalLengthPercent), "Upper bound of the acceptable target/canonical length range.", "Rows longer than this bound are suggested for removal when the ratio rule is active.")
	add("Hard rules", "RequireUniProtAccession", fb(settings.RequireUniProtAccession), fb(defaults.RequireUniProtAccession), "Whether a UniProt accession is mandatory for the row.", "Rows without a UniProt accession can be removed when this is enabled.")
	add("Hard rules", "PreferUniProtReviewed", fb(settings.PreferUniProtReviewed), fb(defaults.PreferUniProtReviewed), "Whether reviewed UniProt entries are preferred during ranking.", "Reviewed rows receive extra ranking support but are not forced on their own.")
	add("Hard rules", "RejectUniProtFragments", fb(settings.RejectUniProtFragments), fb(defaults.RejectUniProtFragments), "Whether fragment-marked UniProt entries are rejected.", "Fragment rows are suggested for removal when this is enabled.")
	add("Hard rules", "RejectUniProtSequenceCautions", fb(settings.RejectUniProtSequenceCautions), fb(defaults.RejectUniProtSequenceCautions), "Whether UniProt sequence-caution rows are rejected.", "Rows with sequence-caution annotations are suggested for removal when this is enabled.")
	add("Hard rules", "RequireInterProConservedRegion", fb(settings.RequireInterProConservedRegion), fb(defaults.RequireInterProConservedRegion), "Whether InterPro conserved-region evidence is mandatory.", "Rows without acceptable InterPro evidence can be removed when this is enabled.")
	add("Hard rules", "AllowInterProPartial", fb(settings.AllowInterProPartial), fb(defaults.AllowInterProPartial), "Whether partial conserved-region status may stay in the selection.", "Partial rows can remain selectable instead of being removed outright.")
	add("Hard rules", "RejectInterProMissing", fb(settings.RejectInterProMissing), fb(defaults.RejectInterProMissing), "Whether missing InterPro conserved-region evidence is rejected.", "Missing conserved-region evidence can remove a row when this is enabled.")
	add("Hard rules", "RejectInterProUncertain", fb(settings.RejectInterProUncertain), fb(defaults.RejectInterProUncertain), "Whether uncertain InterPro conserved-region evidence is rejected.", "Uncertain conserved-region evidence can remove a row when this is enabled.")
	add("Hard rules", "MinInterProCoveragePercent", fv(settings.MinInterProCoveragePercent), fv(defaults.MinInterProCoveragePercent), "Minimum InterPro coverage needed when coverage is used as a rule.", "Rows below this coverage can be removed when coverage checking is active.")
	add("Hard rules", "RequireInterProCoverageWhenUsed", fb(settings.RequireInterProCoverageWhenUsed), fb(defaults.RequireInterProCoverageWhenUsed), "Whether coverage becomes mandatory whenever InterPro is consulted.", "Missing coverage can remove a row when the coverage rule is active.")
	add("Hard rules", "RejectIfAnyHardRuleFails", fb(settings.RejectIfAnyHardRuleFails), fb(defaults.RejectIfAnyHardRuleFails), "Whether any hard-rule failure causes automatic removal.", "This is the main hard-rule gate for the filter.")

	add("Selection limits", "KeepBestIsoformPerTargetGene", fb(settings.KeepBestIsoformPerTargetGene), fb(defaults.KeepBestIsoformPerTargetGene), "Whether duplicate isoforms per target gene are collapsed to the best-ranked row.", "Prevents multiple isoforms from occupying the export set when one best row is enough.")
	add("Selection limits", "KeepTopHitsPerQuery", fb(settings.KeepTopHitsPerQuery), fb(defaults.KeepTopHitsPerQuery), "Whether only the top N hits are kept per query.", "Limits the number of exported rows per query when enabled.")
	add("Selection limits", "TopHitsPerQuery", fi(settings.TopHitsPerQuery), fi(defaults.TopHitsPerQuery), "Maximum number of hits retained for each query when top-hit limiting is enabled.", "Rows beyond this limit are removed during ranking.")

	add("Ranking and tie-breaks", "RankingTieBreakerOrder", settings.RankingTieBreakerOrder, defaults.RankingTieBreakerOrder, "Ranking priority list used when rows need to be ordered.", "Defines the preference chain for isoform and top-hit limiting.")
	add("Ranking and tie-breaks", "PreferHigherFilterScoreWhenRanking", fb(settings.PreferHigherFilterScoreWhenRanking), fb(defaults.PreferHigherFilterScoreWhenRanking), "Whether higher filter score should outrank lower scores.", "Rows with stronger scores are ranked ahead of weaker ones.")
	add("Ranking and tie-breaks", "PreferLowerEValueWhenTies", fb(settings.PreferLowerEValueWhenTies), fb(defaults.PreferLowerEValueWhenTies), "Whether lower E-value wins a tie.", "Lower E-value rows are ranked first when scores are tied.")
	add("Ranking and tie-breaks", "PreferHigherIdentityWhenTies", fb(settings.PreferHigherIdentityWhenTies), fb(defaults.PreferHigherIdentityWhenTies), "Whether higher identity wins a tie.", "Higher identity rows are ranked first when earlier criteria tie.")
	add("Ranking and tie-breaks", "PreferHigherCoverageWhenTies", fb(settings.PreferHigherCoverageWhenTies), fb(defaults.PreferHigherCoverageWhenTies), "Whether higher query coverage wins a tie.", "Higher coverage rows are ranked first when earlier criteria tie.")
	add("Ranking and tie-breaks", "PreferHigherReferenceScoreWhenTies", fb(settings.PreferHigherReferenceScoreWhenTies), fb(defaults.PreferHigherReferenceScoreWhenTies), "Whether higher reference score wins a tie.", "Rows with stronger external-reference evidence are ranked first.")
	add("Ranking and tie-breaks", "PreferHigherBitscoreWhenTies", fb(settings.PreferHigherBitscoreWhenTies), fb(defaults.PreferHigherBitscoreWhenTies), "Whether higher bitscore wins a tie.", "Higher BLAST score is used as the final tie-break.")

	add("Soft score", "EnableSoftScore", fb(settings.EnableSoftScore), fb(defaults.EnableSoftScore), "Whether soft scoring is used as an additional removal criterion.", "When enabled, rows under the minimum soft score are removed.")
	add("Soft score", "MinSoftScore", fi(settings.MinSoftScore), fi(defaults.MinSoftScore), "Minimum soft score accepted when soft scoring is active.", "Rows below this threshold are suggested for removal.")
	add("Soft score", "IdentityWeight", fi(settings.IdentityWeight), fi(defaults.IdentityWeight), "Score weight for satisfying the identity rule.", "Adds score when identity is strong enough.")
	add("Soft score", "CoverageWeight", fi(settings.CoverageWeight), fi(defaults.CoverageWeight), "Score weight for satisfying the query coverage rule.", "Adds score when query coverage is strong enough.")
	add("Soft score", "LengthRatioWeight", fi(settings.LengthRatioWeight), fi(defaults.LengthRatioWeight), "Score weight for satisfying the target/canonical length rule.", "Adds score when length ratio is inside range.")
	add("Soft score", "InterProWeight", fi(settings.InterProWeight), fi(defaults.InterProWeight), "Score weight for strong InterPro conserved-region evidence.", "Adds score when conserved-region evidence is present.")
	add("Soft score", "InterProPartialWeight", fi(settings.InterProPartialWeight), fi(defaults.InterProPartialWeight), "Score weight for partial InterPro conserved-region evidence.", "Adds a smaller score when partial evidence is allowed.")
	add("Soft score", "InterProCoverageWeight", fi(settings.InterProCoverageWeight), fi(defaults.InterProCoverageWeight), "Score weight for sufficient InterPro coverage.", "Adds score when InterPro coverage reaches the configured threshold.")
	add("Soft score", "UniProtReviewedWeight", fi(settings.UniProtReviewedWeight), fi(defaults.UniProtReviewedWeight), "Score weight for reviewed UniProt entries.", "Adds score when the UniProt row is reviewed.")
	add("Soft score", "UniProtAnnotationWeight", fi(settings.UniProtAnnotationWeight), fi(defaults.UniProtAnnotationWeight), "Score weight for usable UniProt annotation text.", "Adds score when the row carries enrichment annotations.")
	add("Soft score", "PenaltySequenceCaution", fi(settings.PenaltySequenceCaution), fi(defaults.PenaltySequenceCaution), "Penalty applied when UniProt sequence caution is present.", "Lowers the score when the sequence is flagged.")
	add("Soft score", "PenaltyFragment", fi(settings.PenaltyFragment), fi(defaults.PenaltyFragment), "Penalty applied when UniProt fragment evidence is present.", "Lowers the score for fragment-marked rows.")

	add("Reference scoring", "InterProPresentReferenceScore", fi(settings.InterProPresentReferenceScore), fi(defaults.InterProPresentReferenceScore), "Reference score assigned to present InterPro evidence.", "Higher scores strengthen reference-ranked rows.")
	add("Reference scoring", "InterProPartialReferenceScore", fi(settings.InterProPartialReferenceScore), fi(defaults.InterProPartialReferenceScore), "Reference score assigned to partial InterPro evidence.", "Partial evidence contributes less than present evidence.")
	add("Reference scoring", "InterProUncertainReferenceScore", fi(settings.InterProUncertainReferenceScore), fi(defaults.InterProUncertainReferenceScore), "Reference score assigned to uncertain InterPro evidence.", "Uncertain evidence contributes only a small score.")
	add("Reference scoring", "InterProMissingReferencePenalty", fi(settings.InterProMissingReferencePenalty), fi(defaults.InterProMissingReferencePenalty), "Penalty assigned when InterPro evidence is missing.", "Missing evidence lowers the reference score.")
	add("Reference scoring", "InterProCoverageReferenceDivisor", fi(settings.InterProCoverageReferenceDivisor), fi(defaults.InterProCoverageReferenceDivisor), "Divisor used to convert InterPro coverage into a score component.", "Larger divisors make coverage contribute more gently.")
	add("Reference scoring", "UniProtAccessionReferenceScore", fi(settings.UniProtAccessionReferenceScore), fi(defaults.UniProtAccessionReferenceScore), "Reference score assigned when a UniProt accession is available.", "Accessions strengthen the reference score.")
	add("Reference scoring", "UniProtReviewedReferenceScore", fi(settings.UniProtReviewedReferenceScore), fi(defaults.UniProtReviewedReferenceScore), "Reference score assigned to reviewed UniProt entries.", "Reviewed accessions strengthen ranking more than unreviewed ones.")
	add("Reference scoring", "UniProtAnnotationReferenceScore", fi(settings.UniProtAnnotationReferenceScore), fi(defaults.UniProtAnnotationReferenceScore), "Reference score assigned for usable UniProt annotation text.", "Annotation text strengthens the reference score.")
	add("Reference scoring", "FragmentReferencePenaltyMultiplier", fi(settings.FragmentReferencePenaltyMultiplier), fi(defaults.FragmentReferencePenaltyMultiplier), "Multiplier applied when a fragment row is scored.", "Fragment evidence reduces the reference score more strongly.")
	add("Reference scoring", "SequenceCautionReferencePenaltyMultiplier", fi(settings.SequenceCautionReferencePenaltyMultiplier), fi(defaults.SequenceCautionReferencePenaltyMultiplier), "Multiplier applied when sequence caution is scored.", "Sequence-caution evidence reduces the reference score.")
	add("Reference scoring", "LengthNearDistancePercent", fv(settings.LengthNearDistancePercent), fv(defaults.LengthNearDistancePercent), "Distance from 100% length ratio treated as near.", "Near values earn the strongest length-reference score.")
	add("Reference scoring", "LengthNearReferenceScore", fi(settings.LengthNearReferenceScore), fi(defaults.LengthNearReferenceScore), "Reference score used when length ratio is near 100%.", "Near-ratio rows receive the highest length-reference score.")
	add("Reference scoring", "LengthAcceptableDistancePercent", fv(settings.LengthAcceptableDistancePercent), fv(defaults.LengthAcceptableDistancePercent), "Distance from 100% length ratio treated as acceptable.", "Acceptable values receive a moderate length-reference score.")
	add("Reference scoring", "LengthAcceptableReferenceScore", fi(settings.LengthAcceptableReferenceScore), fi(defaults.LengthAcceptableReferenceScore), "Reference score used when length ratio is acceptable but not near.", "Acceptable-ratio rows receive a smaller length-reference score.")
	add("Reference scoring", "LengthFarDistancePercent", fv(settings.LengthFarDistancePercent), fv(defaults.LengthFarDistancePercent), "Distance from 100% length ratio treated as far.", "Far values are penalized rather than rewarded.")
	add("Reference scoring", "LengthFarReferencePenalty", fi(settings.LengthFarReferencePenalty), fi(defaults.LengthFarReferencePenalty), "Penalty applied when the length ratio is far from canonical length.", "Far-ratio rows lose reference score.")

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

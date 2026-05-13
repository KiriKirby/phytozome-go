package labelname

import "testing"

func TestBestAliasPrefersCanonicalFamilyStyleOverInternalPrefix(t *testing.T) {
	if got := BestAlias("ATPAL1; PAL1"); got != "PAL1" {
		t.Fatalf("BestAlias()=%q want PAL1", got)
	}
	if got := BestAlias("CYP84A1; FAH1; F5H1"); got != "F5H1" {
		t.Fatalf("BestAlias()=%q want F5H1", got)
	}
	if got := BestAlias("CYP98A3; REF8"); got != "CYP98A3" {
		t.Fatalf("BestAlias()=%q want CYP98A3", got)
	}
}

func TestLabelFromAutoDefineFindsCompactFunctionalAlias(t *testing.T) {
	if got := LabelFromAutoDefine("(1 of 2) K09755 - ferulate-5-hydroxylase (CYP84A, F5H)"); got != "F5H" {
		t.Fatalf("LabelFromAutoDefine()=%q want F5H", got)
	}
	if got := LabelFromAutoDefine("(1 of 1) K09754 - coumaroylquinate(coumaroylshikimate) 3'-monooxygenase (CYP98A3, C3'H)"); got != "C3'H" {
		t.Fatalf("LabelFromAutoDefine()=%q want C3'H", got)
	}
}

func TestFastaHeaderLabelNamePreservesParentheticalLabel(t *testing.T) {
	tests := map[string]string{
		"Arabidopsis thaliana TAIR10|AT5G62380.1 (AtVND6)": "AtVND6",
		"Arabidopsis thaliana TAIR10|AT5G62380.1 (VND6)":   "VND6",
		"Arabidopsis thaliana TAIR10|AT5G62380.1 (ATVND6)": "ATVND6",
	}
	for input, want := range tests {
		if got := FastaHeaderLabelName(input); got != want {
			t.Fatalf("FastaHeaderLabelName(%q)=%q want %q", input, got, want)
		}
	}
}

func TestTrustedLabelPrefersCanonicalCompactSymbol(t *testing.T) {
	if got := TrustedLabel("CYP84A1", "F5H1", "LysoPL2"); got != "F5H1" {
		t.Fatalf("TrustedLabel()=%q want F5H1", got)
	}
}

func TestTrustedLabelRejectsUntrustedCandidates(t *testing.T) {
	if got := TrustedLabel("E2.3.1.133", "LysoPL2"); got != "" {
		t.Fatalf("TrustedLabel()=%q want empty", got)
	}
}

func TestAliasPreferenceScoreDoesNotPenalizeATSpeciesIdentifiers(t *testing.T) {
	if gotAt, gotOs := AliasPreferenceScore("AT1G51680"), AliasPreferenceScore("OS1G51680"); gotAt != gotOs {
		t.Fatalf("AliasPreferenceScore should not special-case AT species ids: AT=%d OS=%d", gotAt, gotOs)
	}
	if gotAt, gotZm := QueryAliasPrimarySymbolBonus("AT4CL1"), QueryAliasPrimarySymbolBonus("ZM4CL1"); gotAt != gotZm {
		t.Fatalf("QueryAliasPrimarySymbolBonus should not special-case AT species labels: AT=%d ZM=%d", gotAt, gotZm)
	}
}

func TestRankAliasBatchMatchesSingleRankingWithTrimmedDuplicateInputs(t *testing.T) {
	request := AliasRankRequest{
		TaskTimestamp: "t1",
		ItemIndex:     7,
		ProteinID:     " AT5G13930.1 ",
		GeneID:        "AT5G13930",
		Aliases:       []string{" PAL1 ", "ATPAL1", "pal1", " PAL1"},
	}
	got := RankAliasBatch([]AliasRankRequest{request, request})
	if len(got) != 2 {
		t.Fatalf("RankAliasBatch returned %d results, want 2", len(got))
	}
	want := RankAliases(request)
	for i := range got {
		if len(got[i].RankedAliases) != len(want.RankedAliases) {
			t.Fatalf("result %d aliases = %#v, want %#v", i, got[i].RankedAliases, want.RankedAliases)
		}
		for j := range want.RankedAliases {
			if got[i].RankedAliases[j] != want.RankedAliases[j] {
				t.Fatalf("result %d alias %d = %q, want %q", i, j, got[i].RankedAliases[j], want.RankedAliases[j])
			}
		}
	}
}

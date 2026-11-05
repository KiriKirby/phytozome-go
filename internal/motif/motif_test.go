package motif

import (
	"reflect"
	"testing"
)

func TestMatchingIndexesUsesMSAFinderSyntax(t *testing.T) {
	motifs := SplitList("[N/S/G], AXC，ABC-DE,(P/I)PGPX(P/G)XP")
	got := MatchingIndexes("ZZIPGPQGRPYYABC.DE", motifs)
	want := []int{1, 2, 3, 4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestXDoesNotMatchGap(t *testing.T) {
	if got := MatchingIndexes("A-C", []string{"AXC"}); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestEllipsisMatchesAnyLengthSequence(t *testing.T) {
	tests := []struct {
		sequence string
		motif    string
		want     bool
	}{
		{sequence: "AAEXXRQQPERGGGR", motif: "EXXR...P(E/F)R...R", want: true},
		{sequence: "EXXRPERR", motif: "EXXR...P(E/F)R...R", want: true},
		{sequence: "EXXRQQPFRR", motif: "EXXR...P(E/F)R...R", want: true},
		{sequence: "EXXRQQPDRR", motif: "EXXR...P(E/F)R...R", want: false},
		{sequence: "ABC---XYZ", motif: "ABC...XYZ", want: true},
	}
	for _, test := range tests {
		got := MatchingIndexes(test.sequence, []string{test.motif})
		if matched := len(got) == 1; matched != test.want {
			t.Fatalf("MatchingIndexes(%q, %q) matched = %v, want %v", test.sequence, test.motif, matched, test.want)
		}
	}
}

func TestAllMatchedRequiresEveryMotifIndex(t *testing.T) {
	tests := []struct {
		matches string
		total   int
		want    bool
	}{
		{matches: "1,2", total: 2, want: true},
		{matches: "1", total: 2, want: false},
		{matches: "1,3", total: 2, want: false},
		{matches: "1,1", total: 2, want: false},
		{matches: "", total: 0, want: false},
	}
	for _, test := range tests {
		if got := AllMatched(test.matches, test.total); got != test.want {
			t.Fatalf("AllMatched(%q, %d) = %v, want %v", test.matches, test.total, got, test.want)
		}
	}
}

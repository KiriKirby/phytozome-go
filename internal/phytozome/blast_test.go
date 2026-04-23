package phytozome

import (
	"reflect"
	"testing"
)

func TestSpecificIdentifierVariants(t *testing.T) {
	got := specificIdentifierVariants("At2g37040")
	want := []string{"At2g37040", "AT2G37040", "at2g37040"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected variants: got %v want %v", got, want)
	}
}

func TestSpecificIdentifierVariantsDeduplicates(t *testing.T) {
	got := specificIdentifierVariants("AT2G37040")
	want := []string{"AT2G37040", "at2g37040"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected variants: got %v want %v", got, want)
	}
}

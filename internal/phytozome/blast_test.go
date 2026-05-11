package phytozome

import (
	"reflect"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/model"
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

func TestApplyPhytozomeQueryLabelsUsesFirstAlias(t *testing.T) {
	var source model.QuerySequenceSource
	applyPhytozomeQueryLabels(&source, geneRecord{
		Symbols:  []string{"PAL4", "PAL4"},
		Synonyms: []string{"ATPAL4"},
	})
	if source.LabelName != "PAL4" {
		t.Fatalf("unexpected label: %q", source.LabelName)
	}
	if source.Aliases != "PAL4; ATPAL4" {
		t.Fatalf("unexpected aliases: %q", source.Aliases)
	}
}

func TestPhytozomeGeneReportKeywordParsesURL(t *testing.T) {
	reportType, identifier, ok := phytozomeGeneReportKeyword("https://phytozome-next.jgi.doe.gov/report/gene/Athaliana_TAIR10/AT3G10340")
	if !ok {
		t.Fatal("expected Phytozome gene report URL to parse")
	}
	if reportType != "gene" || identifier != "AT3G10340" {
		t.Fatalf("unexpected parsed values: reportType=%q identifier=%q", reportType, identifier)
	}
}

func TestPhytozomeGeneReportKeywordParsesURLWithoutScheme(t *testing.T) {
	reportType, identifier, ok := phytozomeGeneReportKeyword("phytozome-next.jgi.doe.gov/report/transcript/Athaliana_TAIR10/AT3G10340.1")
	if !ok {
		t.Fatal("expected Phytozome transcript report URL without scheme to parse")
	}
	if reportType != "transcript" || identifier != "AT3G10340.1" {
		t.Fatalf("unexpected parsed values: reportType=%q identifier=%q", reportType, identifier)
	}
}

func TestPhytozomeGeneReportKeywordParsesProteinURL(t *testing.T) {
	reportType, identifier, ok := phytozomeGeneReportKeyword("https://phytozome-next.jgi.doe.gov/report/protein/S_polyrhiza_v2/Spipo15G0028500")
	if !ok {
		t.Fatal("expected Phytozome protein report URL to parse")
	}
	if reportType != "protein" || identifier != "Spipo15G0028500" {
		t.Fatalf("unexpected parsed values: reportType=%q identifier=%q", reportType, identifier)
	}
}

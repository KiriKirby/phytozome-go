package phytozomekeyword

import (
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

func TestBuildKeywordResultRowPopulatesProteinID(t *testing.T) {
	gene := GeneRecord{
		PrimaryIdentifier: "LOC_Os08g14760",
		Symbols:           []string{"Os4CL1"},
		Organism: GeneOrganismInfo{
			OrganismName:      "Oryza sativa",
			AnnotationVersion: "v7.0",
			Proteome:          323,
		},
		Transcripts: []GeneTranscript{{
			Protein:             "XP_015650724.1",
			PrimaryIdentifier:   "LOC_Os08g14760.1",
			SecondaryIdentifier: "PAC:1",
			IsPrimary:           "1",
		}},
	}

	row, err := buildKeywordResultRow("Os4CL1", SearchTypeRiceGeneAlias, model.SpeciesCandidate{ProteomeID: 323, JBrowseName: "Osativa_v7_0"}, gene)
	if err != nil {
		t.Fatalf("buildKeywordResultRow returned error: %v", err)
	}
	if row.ProteinID != "XP_015650724.1" {
		t.Fatalf("protein id = %q, want XP_015650724.1", row.ProteinID)
	}
}

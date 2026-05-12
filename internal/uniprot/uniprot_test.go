// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package uniprot

import (
	"strings"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

func TestParseTSVHandlesUniProtCommentsAndQuotes(t *testing.T) {
	raw := "Entry\tEntry Name\tReviewed\tProtein names\tGene Names\tOrganism\tOrganism (ID)\tLength\tFunction [CC]\tCatalytic activity\tGene Ontology (GO)\tGene Ontology IDs\tEC number\tKeywords\tPfam\tInterPro\tPathway\tSubcellular location [CC]\tProtein existence\tAnnotation\tFragment\tSequence caution\tDomain [FT]\tRegion\tMotif\tActive site\tBinding site\tAlphaFoldDB\tPDB\n" +
		"Q43158\tQ43158_SPIPO\tunreviewed\tPeroxidase (EC 1.11.1.7)\t\tSpirodela polyrhiza (Giant duckweed)\t29656\t329\tFUNCTION: Removal of H(2)O(2).\tCATALYTIC ACTIVITY: /ligand=\"Ca(2+)\"; EC=1.11.1.7\textracellular region [GO:0005576]\tGO:0005576\t1.11.1.7\tCalcium;Peroxidase\tPF00141;\tIPR002016;\t\tSUBCELLULAR LOCATION: Secreted.\tEvidence at transcript level\t3.0\t\t\tDOMAIN 24..329\t\t\tACT_SITE 65\tBINDING 66; /ligand=\"Ca(2+)\"\tQ43158;\t\n"

	entries, err := parseTSV(raw)
	if err != nil {
		t.Fatalf("parseTSV returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("unexpected entry count: %d", len(entries))
	}
	got := entries[0]
	if got.Accession != "Q43158" || got.EC != "1.11.1.7" || got.Length != 329 {
		t.Fatalf("unexpected parsed entry: %#v", got)
	}
	if !strings.Contains(got.BindingSite, `ligand="Ca(2+)"`) {
		t.Fatalf("binding site quotes were not preserved: %q", got.BindingSite)
	}
}

func TestCandidateQueriesIncludeGeneAndOrganismFallbacks(t *testing.T) {
	row := model.BlastResultRow{
		UniProtAccession: "Q43158",
		Protein:          "Spipo15G0028500",
		SubjectID:        "Spipo15G0028500",
		SequenceID:       "PAC:31507400",
		TranscriptID:     "Spipo15G0028500",
		Species:          "Spirodela polyrhiza (greater duckweed)",
		Defline:          "Cytochrome P450",
	}
	queries := strings.Join(candidateQueries(row), "\n")
	for _, want := range []string{
		"accession:Q43158",
		"accession:Spipo15G0028500",
		"gene:Spipo15G0028500",
		`gene:Spipo15G0028500 AND organism_name:"Spirodela polyrhiza"`,
	} {
		if !strings.Contains(queries, want) {
			t.Fatalf("candidate queries missing %q in:\n%s", want, queries)
		}
	}
	if strings.Contains(queries, "protein_name:") {
		t.Fatalf("candidate queries should not use weak protein_name matching:\n%s", queries)
	}
}

func TestExtractUniProtAccessionsFromAHRDLikeText(t *testing.T) {
	got := extractUniProtAccessions("sp|Q43158|PER_SPIOL; tr|A0A123ABC1|foo")
	joined := strings.Join(got, ",")
	for _, want := range []string{"Q43158", "A0A123ABC1"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing accession %s in %#v", want, got)
		}
	}
}

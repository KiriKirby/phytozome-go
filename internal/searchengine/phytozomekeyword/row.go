// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package phytozomekeyword

import (
	"fmt"
	"strings"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

func buildKeywordResultRow(searchTerm string, searchType string, species model.SpeciesCandidate, gene GeneRecord) (model.KeywordResultRow, error) {
	transcript, err := gene.PrimaryTranscript("")
	if err != nil {
		return model.KeywordResultRow{}, err
	}

	geneID := strings.TrimSpace(gene.PrimaryIdentifier)
	internalID := strings.TrimSpace(gene.ID)
	geneIdentifier := geneID
	if internalID != "" {
		geneIdentifier += " (" + internalID + ")"
	}

	uniprotValues := make([]string, 0, len(transcript.Uniprot))
	for _, value := range transcript.Uniprot {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		uniprotValues = append(uniprotValues, transcript.PrimaryIdentifier+": "+value)
	}

	row := model.KeywordResultRow{
		SourceDatabase:      "phytozome",
		SearchTerm:          searchTerm,
		SearchType:          searchType,
		ProteinID:           strings.TrimSpace(transcript.Protein),
		TranscriptID:        strings.TrimSpace(transcript.PrimaryIdentifier),
		GeneIdentifier:      geneIdentifier,
		Genome:              formatKeywordGenome(gene),
		Location:            formatKeywordLocation(gene),
		Symbols:             strings.Join(dedupePreserveOrder(copyStringSlice(gene.Symbols)), "; "),
		Synonyms:            strings.Join(dedupePreserveOrder(copyStringSlice(gene.Synonyms)), "; "),
		UniProt:             strings.Join(uniprotValues, "; "),
		Description:         firstNonEmpty(gene.Deflines...),
		Comments:            strings.Join(gene.Comments, "\n"),
		AutoDefine:          strings.TrimSpace(gene.AutoDefline),
		GeneReportURL:       fmt.Sprintf("https://phytozome-next.jgi.doe.gov/report/gene/%s/%s", species.JBrowseName, geneID),
		SequenceHeaderLabel: strings.TrimSpace(gene.Organism.OrganismName + " " + gene.Organism.AnnotationVersion),
		SequenceID:          strings.TrimSpace(transcript.SecondaryIdentifier),
	}
	return row, nil
}

func formatKeywordGenome(gene GeneRecord) string {
	organism := strings.TrimSpace(gene.Organism.OrganismName)
	annotation := strings.TrimSpace(gene.Organism.AnnotationVersion)
	proteome := gene.Organism.Proteome
	taxID := strings.TrimSpace(gene.Organism.TaxID)

	parts := make([]string, 0, 2)
	if organism != "" || annotation != "" {
		parts = append(parts, strings.TrimSpace(organism+" "+annotation))
	}
	details := make([]string, 0, 2)
	if proteome != 0 {
		details = append(details, fmt.Sprintf("Phytozome genome ID: %d", proteome))
	}
	if taxID != "" {
		details = append(details, "NCBI taxonomy ID: "+taxID)
	}
	if len(details) > 0 {
		parts = append(parts, "("+strings.Join(details, "; ")+")")
	}
	return strings.Join(parts, " ")
}

func formatKeywordLocation(gene GeneRecord) string {
	strand := "forward"
	if strings.TrimSpace(gene.Strand) == "-1" {
		strand = "reverse"
	}
	return fmt.Sprintf("%s:%s..%s %s", strings.TrimSpace(gene.Scaffold), strings.TrimSpace(gene.Start), strings.TrimSpace(gene.End), strand)
}

func dedupePreserveOrder(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func CopyStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func copyStringSlice(values []string) []string {
	return CopyStringSlice(values)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

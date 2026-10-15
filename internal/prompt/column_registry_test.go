// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package prompt

import (
	"slices"
	"strings"
	"testing"
)

func TestKnownColumnHelpIDsAllResolve(t *testing.T) {
	ids := KnownColumnHelpIDs()
	if len(ids) == 0 {
		t.Fatal("expected known column ids")
	}
	for _, id := range ids {
		help := ColumnHelpText(id)
		if strings.TrimSpace(help) == "" {
			t.Fatalf("missing help text for %q", id)
		}
		if strings.TrimSpace(ColumnHelpEnglish(id)) == "" {
			t.Fatalf("missing English report help for %q", id)
		}
		if !strings.Contains(help, "EN:") || !strings.Contains(help, "中文：") || !strings.Contains(help, "日本語：") {
			t.Fatalf("help text for %q is not tri-lingual: %q", id, help)
		}
	}
}

func TestKnownColumnHelpIDsHaveDocumentedEnglishDescriptions(t *testing.T) {
	ids := KnownColumnHelpIDs()
	for _, id := range ids {
		english := strings.TrimSpace(ColumnHelpEnglish(id))
		if got := len(strings.Fields(english)); got < 18 {
			t.Fatalf("English column help for %q is too thin: %d words: %q", id, got, english)
		}
		lower := strings.ToLower(english)
		for _, weak := range []string{
			"exact meaning depends",
			"dynamic column",
			"column derived from",
			"workflow state",
		} {
			if strings.Contains(lower, weak) {
				t.Fatalf("English column help for %q is generic, contains %q: %q", id, weak, english)
			}
		}
	}
}

func TestColumnHelpAliasesResolveCanonicalReportIDs(t *testing.T) {
	for _, id := range []string{"search_term", "description", "alias"} {
		if got := strings.TrimSpace(ColumnHelpEnglish(id)); got == "" {
			t.Fatalf("expected English help for alias id %q", id)
		}
	}
}

func TestColumnHelpDynamicFallbacksResolveUnknownStructuredColumns(t *testing.T) {
	for _, id := range []string{"attr_custom_flag", "gff_custom_score", "ahrd_extra_note", "lemna_release_name"} {
		if got := strings.TrimSpace(ColumnHelpText(id)); got == "" {
			t.Fatalf("expected generated help text for %q", id)
		}
	}
}

func TestColumnSchemasExistPerDatabaseModeAndView(t *testing.T) {
	if ids := KeywordDisplayColumnIDs("phytozome"); len(ids) == 0 {
		t.Fatal("expected phytozome keyword display schema")
	}
	if ids := KeywordDisplayColumnIDs("tair"); len(ids) == 0 {
		t.Fatal("expected tair keyword display schema")
	}
	if ids := KeywordDetailColumnIDs("lemna"); len(ids) == 0 {
		t.Fatal("expected lemna keyword detail schema")
	}
	if ids := KeywordDetailColumnIDs("tair"); len(ids) == 0 {
		t.Fatal("expected tair keyword detail schema")
	}
	if ids := KeywordExportColumnIDs("phytozome", true, nil); len(ids) == 0 {
		t.Fatal("expected phytozome keyword export schema")
	}
	if ids := KeywordDisplayColumnIDs("ncbi:gene"); len(ids) == 0 {
		t.Fatal("expected ncbi gene keyword display schema")
	}
	if ids := KeywordDetailColumnIDs("ncbi:nuccore"); len(ids) == 0 {
		t.Fatal("expected ncbi nuccore keyword detail schema")
	}
	if ids := KeywordDetailColumnIDs("ncbi:assembly"); len(ids) == 0 {
		t.Fatal("expected ncbi assembly keyword detail schema")
	}
	if ids := KeywordDetailColumnIDs("ncbi:clinvar"); len(ids) == 0 {
		t.Fatal("expected ncbi clinvar keyword detail schema")
	}
	if ids := KeywordDetailColumnIDs("ncbi:pubmed"); len(ids) == 0 {
		t.Fatal("expected ncbi pubmed keyword detail schema")
	}
	if ids := KeywordDetailColumnIDs("ncbi:omim"); len(ids) == 0 {
		t.Fatal("expected ncbi omim keyword detail schema")
	}
	if ids := KeywordExportColumnIDs("tair", true, nil); len(ids) == 0 {
		t.Fatal("expected tair keyword export schema")
	}
	if ids := BlastDisplayColumnIDs("phytozome", "", true, true); len(ids) == 0 {
		t.Fatal("expected phytozome blast display schema")
	}
	if ids := BlastDisplayColumnIDs("lemna", "BLASTP", true, true); len(ids) == 0 {
		t.Fatal("expected lemna blast display schema")
	}
	if ids := BlastDetailColumnIDs("lemna", "BLASTX", true, true); len(ids) == 0 {
		t.Fatal("expected lemna blast detail schema")
	}
	if ids := BlastExportColumnIDs("lemna", true, true); len(ids) == 0 {
		t.Fatal("expected lemna blast export schema")
	}
}

func TestPLAZAKeywordSchemasRetainNCBICompatibleFields(t *testing.T) {
	for name, ids := range map[string][]string{
		"display": KeywordDisplayColumnIDs("plaza"),
		"detail":  KeywordDetailColumnIDs("plaza"),
		"export":  KeywordExportColumnIDs("plaza", true, nil),
	} {
		for _, required := range []string{
			"source_database", "search_term", "search_type", "gene_locus", "protein_id",
			"transcript", "gene_identifier", "genome", "uniprot", "description",
		} {
			if !slices.Contains(ids, required) {
				t.Fatalf("PLAZA %s schema missing %q: %#v", name, required, ids)
			}
		}
	}
}

func TestCYPKeywordSchemasExposeMatchProvenance(t *testing.T) {
	for name, ids := range map[string][]string{
		"display": KeywordDisplayColumnIDs("cyp"),
		"detail":  KeywordDetailColumnIDs("cyp"),
		"export":  KeywordExportColumnIDs("cyp", true, []string{"cyp_match_method", "cyp_matched_search_term"}),
	} {
		for _, required := range []string{"cyp_match_method", "cyp_matched_search_term"} {
			if !slices.Contains(ids, required) {
				t.Fatalf("CYP %s schema missing %q: %#v", name, required, ids)
			}
			if strings.Count(strings.Join(ids, ","), required) != 1 {
				t.Fatalf("CYP %s schema duplicates %q: %#v", name, required, ids)
			}
		}
	}
}

func TestNCBIKeywordDatabaseKeyForSearchTypePrefersSpecializedBuckets(t *testing.T) {
	for _, tc := range []struct {
		searchType string
		domain     string
		want       string
	}{
		{searchType: "gene", domain: "gene-record", want: "ncbi:gene"},
		{searchType: "nuccore", domain: "sequence-record", want: "ncbi:nuccore"},
		{searchType: "assembly", domain: "genome-resource", want: "ncbi:assembly"},
		{searchType: "biosample", domain: "sample-project", want: "ncbi:biosample"},
		{searchType: "taxonomy", domain: "taxonomy-reference", want: "ncbi:taxonomy"},
		{searchType: "pubmed", domain: "literature-reference", want: "ncbi:pubmed"},
		{searchType: "pmc", domain: "literature-reference", want: "ncbi:pmc"},
		{searchType: "clinvar", domain: "variant-clinical", want: "ncbi:clinvar"},
		{searchType: "omim", domain: "variant-clinical", want: "ncbi:omim"},
	} {
		if got := NCBIKeywordDatabaseKeyForSearchType(tc.searchType, tc.domain); got != tc.want {
			t.Fatalf("searchType=%q domain=%q => %q, want %q", tc.searchType, tc.domain, got, tc.want)
		}
	}
}

func TestNCBISpecializedDisplaySchemasExposeNewVisibleMetadataColumns(t *testing.T) {
	for _, tc := range []struct {
		database string
		want     []string
	}{
		{database: "ncbi:assembly", want: []string{"ncbi_assembly_accession", "ncbi_assembly_level", "ncbi_assembly_status"}},
		{database: "ncbi:biosample", want: []string{"ncbi_biosample_accession", "ncbi_isolation_source", "ncbi_geo_loc_name"}},
		{database: "ncbi:sra", want: []string{"ncbi_sra_accession", "ncbi_library_strategy", "ncbi_bioproject_accession"}},
		{database: "ncbi:clinvar", want: []string{"ncbi_clinvar_accession", "ncbi_clinical_significance", "ncbi_review_status"}},
		{database: "ncbi:gtr", want: []string{"ncbi_gtr_accession", "ncbi_condition", "ncbi_lab"}},
		{database: "ncbi:taxonomy", want: []string{"ncbi_taxonomy_id", "ncbi_rank", "ncbi_lineage_summary"}},
		{database: "ncbi:snp", want: []string{"ncbi_rsid", "ncbi_variant_type", "ncbi_clinical_significance"}},
		{database: "ncbi:dbvar", want: []string{"ncbi_dbvar_accession", "ncbi_phenotype", "ncbi_clinical_assertion"}},
		{database: "ncbi:medgen", want: []string{"ncbi_medgen_id", "ncbi_condition_summary", "ncbi_related_gene_summary"}},
		{database: "ncbi:omim", want: []string{"ncbi_omim_id", "ncbi_condition_summary", "ncbi_related_gene_summary"}},
	} {
		ids := KeywordDisplayColumnIDs(tc.database)
		for _, required := range tc.want {
			if !slices.Contains(ids, required) {
				t.Fatalf("%s display schema missing %q: %#v", tc.database, required, ids)
			}
		}
	}
}

func TestNCBISpecializedDetailSchemasExposeReplacementAndDeepFields(t *testing.T) {
	for _, tc := range []struct {
		database string
		want     []string
	}{
		{database: "ncbi:assembly", want: []string{"ncbi_taxonomy_id", "ncbi_replaced_by", "ncbi_replacement_decision"}},
		{database: "ncbi:taxonomy", want: []string{"ncbi_scientific_name", "ncbi_replacement_accession"}},
		{database: "ncbi:snp", want: []string{"ncbi_taxonomy_id", "ncbi_chromosome", "ncbi_chrpos", "ncbi_replacement_decision"}},
		{database: "ncbi:dbvar", want: []string{"ncbi_bioproject_accession", "ncbi_replacement_accession"}},
		{database: "ncbi:medgen", want: []string{"ncbi_definition", "ncbi_source", "ncbi_replacement_decision"}},
		{database: "ncbi:omim", want: []string{"ncbi_omim_text", "ncbi_replacement_accession"}},
	} {
		ids := KeywordDetailColumnIDs(tc.database)
		for _, required := range tc.want {
			if !slices.Contains(ids, required) {
				t.Fatalf("%s detail schema missing %q: %#v", tc.database, required, ids)
			}
		}
	}
}

func TestAllSchemaColumnsResolveToExplicitHelpEntries(t *testing.T) {
	seen := map[string]struct{}{}
	add := func(ids []string) {
		for _, id := range ids {
			canonical := ColumnCanonicalID(id)
			if canonical != "" {
				seen[canonical] = struct{}{}
			}
		}
	}
	for _, db := range []string{"phytozome", "lemna", "tair"} {
		add(KeywordDisplayColumnIDs(db))
		add(KeywordDetailColumnIDs(db))
		add(KeywordExportColumnIDs(db, true, nil))
	}
	for _, db := range []string{"phytozome", "lemna"} {
		for _, program := range []string{"", "BLASTN", "BLASTX", "TBLASTN", "BLASTP"} {
			add(BlastDisplayColumnIDs(db, program, true, true))
			add(BlastDetailColumnIDs(db, program, true, true))
		}
		add(BlastExportColumnIDs(db, true, true))
	}
	for id := range seen {
		help := strings.TrimSpace(ColumnHelpText(id))
		if help == "" {
			t.Fatalf("missing help text for schema column %q", id)
		}
		lower := strings.ToLower(help)
		if strings.Contains(lower, "parsed lemna gff3 attribute") || strings.Contains(lower, "raw lemna gff3 field") || strings.Contains(lower, "ahrd-derived field") || strings.Contains(lower, "lemna release-context field") {
			t.Fatalf("schema column %q still relies on generic dynamic help: %q", id, help)
		}
	}
}

func TestColumnSchemaCallsReturnCopies(t *testing.T) {
	first := KeywordDisplayColumnIDs("phytozome")
	if len(first) == 0 {
		t.Fatal("expected keyword display ids")
	}
	first[0] = "mutated"
	second := KeywordDisplayColumnIDs("phytozome")
	if second[0] == "mutated" {
		t.Fatal("keyword display schema should return a defensive copy")
	}
}

func TestKeywordReportColumnIDsIncludeFormalNonDisplayColumns(t *testing.T) {
	ids := KeywordReportColumnIDs("phytozome", true, nil)
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for _, required := range []string{"row", "sequence_header_label", "sequence_id", "gene_report_url", "protein_id"} {
		if !seen[required] {
			t.Fatalf("keyword report schema missing %q", required)
		}
	}
}

func TestKeywordReportColumnIDsIncludeNCBILinkProvenanceColumns(t *testing.T) {
	ids := KeywordReportColumnIDs("ncbi:clinvar", true, nil)
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for _, required := range []string{
		"ncbi_link_resolution",
		"ncbi_linked_from_db",
		"ncbi_linked_to_db",
		"ncbi_linkname",
		"ncbi_link_source_ids",
		"ncbi_link_target_ids",
		"ncbi_jump_targets",
	} {
		if !seen[required] {
			t.Fatalf("ncbi keyword report schema missing %q", required)
		}
	}
}

func TestIdentifierHeadersRemainBiologicallyExplicit(t *testing.T) {
	if got := ColumnCompactHeader("protein_id", ColumnDisplayOptions{}); got != "protein_id" {
		t.Fatalf("protein_id compact header = %q, want protein_id", got)
	}
	if got := ColumnDetailLabel("gene_identifier", ColumnDisplayOptions{}); got != "geneid" {
		t.Fatalf("gene_identifier detail label = %q, want geneid", got)
	}
	if got := ColumnExportHeader("blast_geneid", ColumnDisplayOptions{}); got != "blast_transcript" {
		t.Fatalf("blast_geneid export header = %q, want blast_transcript", got)
	}
	for _, id := range []string{"protein", "blast_geneid", "protein_id", "gene_identifier"} {
		for _, label := range []string{
			ColumnCompactHeader(id, ColumnDisplayOptions{}),
			ColumnDetailLabel(id, ColumnDisplayOptions{}),
			ColumnExportHeader(id, ColumnDisplayOptions{}),
		} {
			if strings.Contains(strings.ToLower(label), "id2") {
				t.Fatalf("%s leaked internal ID2 label %q", id, label)
			}
		}
	}
}

func TestKeywordReportColumnIDsIncludeLemnaFormalExtras(t *testing.T) {
	ids := KeywordReportColumnIDs("lemna", true, nil)
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for _, required := range []string{"gff_seqid", "attr_ID", "ahrd_human_readable_description"} {
		if !seen[required] {
			t.Fatalf("lemna keyword report schema missing %q", required)
		}
	}
}

func TestKeywordReportColumnIDsIncludeTAIRFormalExtras(t *testing.T) {
	ids := KeywordReportColumnIDs("tair", true, nil)
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for _, required := range []string{"tair_version", "tair_fasta_header", "gff_attributes", "attr_Dbxref"} {
		if !seen[required] {
			t.Fatalf("tair keyword report schema missing %q", required)
		}
	}
}

func TestBlastReportColumnIDsExcludeReferenceColumnsWhenDisabled(t *testing.T) {
	ids := BlastReportColumnIDs("phytozome", "BLASTP", false, false)
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for _, forbidden := range []string{
		"uniprot_accession",
		"uniprot_canonical_length",
		"target_uniprot_canonical_length_percent",
		"interpro_entry_name",
		"interpro_conserved_region_status",
	} {
		if seen[forbidden] {
			t.Fatalf("blast no-ref report schema should omit %q", forbidden)
		}
	}
}

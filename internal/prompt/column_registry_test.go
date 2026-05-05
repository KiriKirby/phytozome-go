package prompt

import (
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
	if ids := KeywordDetailColumnIDs("lemna"); len(ids) == 0 {
		t.Fatal("expected lemna keyword detail schema")
	}
	if ids := KeywordExportColumnIDs("phytozome", true, nil); len(ids) == 0 {
		t.Fatal("expected phytozome keyword export schema")
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
	for _, db := range []string{"phytozome", "lemna"} {
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

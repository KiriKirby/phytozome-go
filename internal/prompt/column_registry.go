package prompt

import (
	"sort"
	"strings"
)

type ColumnDisplayOptions struct {
	DatabaseDisplay string
	Multiline       bool
}

type columnMetadata struct {
	CompactHeader string
	DetailLabel   string
	ExportHeader  string
}

var keywordDisplayColumnIDsByDatabase = map[string][]string{
	"phytozome": {"search_term", "search_type", "label_name", "transcript", "description", "genome", "alias"},
	"lemna":     {"search_term", "label_name", "transcript", "description", "genome", "alias"},
}

var keywordDetailColumnIDsByDatabase = map[string][]string{
	"phytozome": {"search_term", "search_type", "label_name", "protein_id", "transcript", "gene_identifier", "genome", "location", "alias", "uniprot", "description", "comments", "auto_define", "gene_report_url", "sequence_header_label", "sequence_id"},
	"lemna":     {"search_term", "label_name", "transcript", "description", "genome", "protein_id", "gene_identifier", "location", "alias", "uniprot", "comments", "auto_define", "gene_report_url", "sequence_header_label", "sequence_id"},
}

var keywordExportColumnIDsByDatabase = map[string][]string{
	"phytozome": {"row", "search_term", "search_type", "label_name", "protein_id", "transcript", "gene_identifier", "genome", "location", "alias", "uniprot", "description", "comments", "auto_define", "gene_report_url"},
	"lemna":     {"row", "search_term", "label_name", "protein_id", "transcript", "gene_identifier", "genome", "location", "alias", "uniprot", "description", "comments", "auto_define", "gene_report_url"},
}

var keywordReportExtraColumnIDsByDatabase = map[string][]string{
	"phytozome": nil,
	"lemna": {
		"gff_seqid",
		"gff_source",
		"gff_type",
		"gff_start",
		"gff_end",
		"gff_score",
		"gff_strand",
		"gff_phase",
		"gff_attributes",
		"lemna_release",
		"lemna_gff_url",
		"attr_ID",
		"attr_Name",
		"attr_Parent",
		"attr_Alias",
		"attr_Dbxref",
		"attr_product",
		"attr_description",
		"attr_Note",
		"attr_note",
		"attr_gene_id",
		"attr_transcript_id",
		"attr_protein_id",
		"attr_protein",
		"attr_protein_accession",
		"attr_locus",
		"attr_comment",
		"ahrd_protein_accession",
		"ahrd_blast_hit_accession",
		"ahrd_quality_code",
		"ahrd_human_readable_description",
		"ahrd_interpro",
		"ahrd_gene_ontology_term",
	},
}

var blastDisplayBaseColumnIDsByDatabaseProgram = map[string]map[string][]string{
	"phytozome": {
		"default": {"label_name", "protein", "percent_identity", "e_value", "align_query_length_percent", "interpro_conserved_region_status", "target_length", "target_uniprot_canonical_length_percent", "align_len", "query_length", "uniprot_canonical_length", "species"},
	},
	"lemna": {
		"BLASTN":  {"label_name", "protein", "percent_identity", "align_query_length_percent", "interpro_conserved_region_status", "target_length", "target_uniprot_canonical_length_percent", "align_len", "query_length", "uniprot_canonical_length", "species"},
		"BLASTX":  {"label_name", "protein", "percent_identity", "align_query_length_percent", "interpro_conserved_region_status", "target_length", "target_uniprot_canonical_length_percent", "align_len", "query_length", "uniprot_canonical_length", "species"},
		"TBLASTN": {"label_name", "protein", "percent_identity", "align_query_length_percent", "interpro_conserved_region_status", "target_length", "target_uniprot_canonical_length_percent", "align_len", "query_length", "uniprot_canonical_length", "species"},
		"BLASTP":  {"label_name", "protein", "percent_identity", "align_query_length_percent", "interpro_conserved_region_status", "target_length", "target_uniprot_canonical_length_percent", "align_len", "query_length", "uniprot_canonical_length", "species"},
	},
}

var blastDisplayUniProtColumnIDs = []string{"uniprot_accession", "uniprot_reviewed", "uniprot_gene_names"}
var blastDisplayInterProColumnIDs = []string{"interpro_entry_type", "interpro_coverage_percent"}

var blastDetailBaseColumnIDsByDatabaseProgram = map[string]map[string][]string{
	"phytozome": {
		"default": {"label_name", "source_database", "blast_program", "hit_number", "hsp_number", "protein", "subject_id", "species", "e_value", "percent_identity", "align_query_length_percent", "interpro_conserved_region_status", "target_length", "align_len", "strands", "query_id", "query_from", "query_to", "target_from", "target_to", "bitscore", "identical", "positives", "gaps", "query_length", "target_length", "target_uniprot_canonical_length_percent", "jbrowse_name", "target_id", "sequence_id", "transcript_id", "defline", "gene_report_url", "uniprot_canonical_length"},
	},
	"lemna": {
		"BLASTN":  {"label_name", "source_database", "blast_program", "hit_number", "protein", "subject_id", "species", "e_value", "percent_identity", "align_query_length_percent", "interpro_conserved_region_status", "align_len", "mismatches", "gap_openings", "gaps", "query_id", "query_from", "query_to", "target_from", "target_to", "bitscore", "identical", "positives", "query_length", "target_length", "target_uniprot_canonical_length_percent", "jbrowse_name", "target_id", "sequence_id", "transcript_id", "defline", "gene_report_url", "uniprot_canonical_length"},
		"BLASTX":  {"label_name", "source_database", "blast_program", "hit_number", "protein", "subject_id", "species", "e_value", "percent_identity", "align_query_length_percent", "interpro_conserved_region_status", "align_len", "mismatches", "gap_openings", "gaps", "query_id", "query_from", "query_to", "target_from", "target_to", "bitscore", "identical", "positives", "query_length", "target_length", "target_uniprot_canonical_length_percent", "jbrowse_name", "target_id", "sequence_id", "transcript_id", "defline", "gene_report_url", "uniprot_canonical_length"},
		"TBLASTN": {"label_name", "source_database", "blast_program", "hit_number", "protein", "subject_id", "species", "e_value", "percent_identity", "align_query_length_percent", "interpro_conserved_region_status", "align_len", "mismatches", "gap_openings", "gaps", "query_id", "query_from", "query_to", "target_from", "target_to", "bitscore", "identical", "positives", "query_length", "target_length", "target_uniprot_canonical_length_percent", "jbrowse_name", "target_id", "sequence_id", "transcript_id", "defline", "gene_report_url", "uniprot_canonical_length"},
		"BLASTP":  {"label_name", "source_database", "blast_program", "hit_number", "protein", "subject_id", "species", "e_value", "percent_identity", "align_query_length_percent", "interpro_conserved_region_status", "align_len", "mismatches", "gap_openings", "gaps", "query_id", "query_from", "query_to", "target_from", "target_to", "bitscore", "identical", "positives", "query_length", "target_length", "target_uniprot_canonical_length_percent", "jbrowse_name", "target_id", "sequence_id", "transcript_id", "defline", "gene_report_url", "uniprot_canonical_length"},
	},
}

var blastDetailUniProtColumnIDs = []string{
	"uniprot_accession", "uniprot_entry_name", "uniprot_reviewed", "uniprot_protein_name", "uniprot_gene_names", "uniprot_organism",
	"uniprot_organism_id", "uniprot_keywords", "uniprot_ec", "uniprot_go", "uniprot_go_ids", "uniprot_function", "uniprot_catalytic_activity",
	"uniprot_pathway", "uniprot_subcellular_location", "uniprot_protein_existence", "uniprot_annotation_score", "uniprot_fragment",
	"uniprot_sequence_caution", "uniprot_pfam", "uniprot_interpro", "uniprot_domain", "uniprot_region", "uniprot_motif", "uniprot_active_site",
	"uniprot_binding_site", "uniprot_alphafolddb", "uniprot_pdb",
}

var blastDetailInterProColumnIDs = []string{
	"interpro_entry_name", "interpro_entry_type", "interpro_coverage_percent", "interpro_match_regions", "interpro_accessions",
	"interpro_signature_accessions", "interpro_pfam_accessions",
}

var blastExportBaseColumnIDsByDatabase = map[string][]string{
	"phytozome": {"row", "source_database", "blast_program", "label_name", "protein", "subject_id", "percent_identity", "align_query_length_percent", "interpro_conserved_region_status", "target_length", "align_len", "query_length", "species", "hit_number", "hsp_number", "e_value", "strands", "query_id", "query_from", "query_to", "target_from", "target_to", "bitscore", "mismatches", "gap_openings", "identical", "positives", "gaps", "jbrowse_name", "target_id", "sequence_id", "transcript_id", "defline", "gene_report_url"},
	"lemna":     {"row", "source_database", "blast_program", "label_name", "protein", "subject_id", "percent_identity", "align_query_length_percent", "interpro_conserved_region_status", "target_length", "align_len", "query_length", "species", "hit_number", "hsp_number", "e_value", "strands", "query_id", "query_from", "query_to", "target_from", "target_to", "bitscore", "mismatches", "gap_openings", "identical", "positives", "gaps", "jbrowse_name", "target_id", "sequence_id", "transcript_id", "defline", "gene_report_url"},
}

var blastExportUniProtColumnIDs = []string{
	"uniprot_accession", "uniprot_entry_name", "uniprot_reviewed", "uniprot_protein_name", "uniprot_gene_names", "uniprot_organism",
	"uniprot_organism_id", "uniprot_keywords", "uniprot_ec", "uniprot_go", "uniprot_go_ids", "uniprot_function", "uniprot_catalytic_activity",
	"uniprot_pathway", "uniprot_subcellular_location", "uniprot_protein_existence", "uniprot_annotation_score", "uniprot_fragment",
	"uniprot_sequence_caution", "uniprot_pfam", "uniprot_interpro", "uniprot_domain", "uniprot_region", "uniprot_motif", "uniprot_active_site",
	"uniprot_binding_site", "uniprot_alphafolddb", "uniprot_pdb",
}

var blastExportInterProColumnIDs = []string{
	"interpro_entry_name", "interpro_entry_type", "interpro_coverage_percent", "interpro_match_regions", "interpro_accessions",
	"interpro_signature_accessions", "interpro_pfam_accessions",
}

var columnHelpAliases = map[string]string{
	"search_term": "search_tern",
	"description": "discripition",
	"alias":       "aliases",
}

var columnMetadataByID = map[string]columnMetadata{
	"row":                              {CompactHeader: "row", DetailLabel: "row", ExportHeader: "row"},
	"search_tern":                      {CompactHeader: "search_tern", DetailLabel: "search_term", ExportHeader: "search_term"},
	"search_type":                      {CompactHeader: "search_type", DetailLabel: "search_type", ExportHeader: "search_type"},
	"label_name":                       {CompactHeader: "label_name", DetailLabel: "label_name", ExportHeader: "label_name"},
	"transcript":                       {CompactHeader: "transcript", DetailLabel: "transcript", ExportHeader: "transcript"},
	"discripition":                     {CompactHeader: "discripition", DetailLabel: "description", ExportHeader: "description"},
	"gnome":                            {CompactHeader: "gnome", DetailLabel: "genome", ExportHeader: "genome"},
	"protein_id":                       {CompactHeader: "protein_id", DetailLabel: "protein_id", ExportHeader: "protein_id"},
	"gene_identifier":                  {CompactHeader: "gene_identifier", DetailLabel: "gene_identifier", ExportHeader: "gene_identifier"},
	"genome":                           {CompactHeader: "gnome", DetailLabel: "genome", ExportHeader: "genome"},
	"location":                         {CompactHeader: "location", DetailLabel: "location", ExportHeader: "location"},
	"aliases":                          {CompactHeader: "alias", DetailLabel: "alias", ExportHeader: "alias"},
	"uniprot":                          {CompactHeader: "uniprot", DetailLabel: "uniprot", ExportHeader: "uniprot"},
	"comments":                         {CompactHeader: "comments", DetailLabel: "comments", ExportHeader: "comments"},
	"auto_define":                      {CompactHeader: "auto_define", DetailLabel: "auto_define", ExportHeader: "auto_define"},
	"gene_report_url":                  {CompactHeader: "gene_report_url", DetailLabel: "gene_report_url", ExportHeader: "gene_report_url"},
	"sequence_header_label":            {CompactHeader: "sequence_header_label", DetailLabel: "sequence_header_label", ExportHeader: "sequence_header_label"},
	"sequence_id":                      {CompactHeader: "sequence_id", DetailLabel: "sequence_id", ExportHeader: "sequence_id"},
	"source_database":                  {CompactHeader: "source_database", DetailLabel: "source_database", ExportHeader: "source_database"},
	"blast_program":                    {CompactHeader: "blast_program", DetailLabel: "blast_program", ExportHeader: "blast_program"},
	"hit_number":                       {CompactHeader: "hit_number", DetailLabel: "hit_number", ExportHeader: "hit_number"},
	"hsp_number":                       {CompactHeader: "hsp_number", DetailLabel: "hsp_number", ExportHeader: "hsp_number"},
	"protein":                          {CompactHeader: "protein", DetailLabel: "protein", ExportHeader: "protein"},
	"subject_id":                       {CompactHeader: "subject_id", DetailLabel: "subject_id", ExportHeader: "subject_id"},
	"species":                          {CompactHeader: "species", DetailLabel: "species", ExportHeader: "species"},
	"e_value":                          {CompactHeader: "e_value", DetailLabel: "e_value", ExportHeader: "e_value"},
	"percent_identity":                 {CompactHeader: "identity (%)", DetailLabel: "identity (%)", ExportHeader: "identity (%)"},
	"align_query_length_percent":       {CompactHeader: "align_len /\nquery_length (%)", DetailLabel: "align_len / query_length (%)", ExportHeader: "align_len / query_length (%)"},
	"interpro_conserved_region_status": {CompactHeader: "InterPro conserved\nregion status", DetailLabel: "InterPro conserved region status", ExportHeader: "InterPro conserved region status"},
	"target_length":                    {CompactHeader: "target_length", DetailLabel: "target_length", ExportHeader: "target_length"},
	"align_len":                        {CompactHeader: "align_len", DetailLabel: "align_len", ExportHeader: "align_len"},
	"query_length":                     {CompactHeader: "query_length", DetailLabel: "query_length", ExportHeader: "query_length"},
	"strands":                          {CompactHeader: "strands", DetailLabel: "strands", ExportHeader: "strands"},
	"query_id":                         {CompactHeader: "query_id", DetailLabel: "query_id", ExportHeader: "query_id"},
	"query_from":                       {CompactHeader: "query_from", DetailLabel: "query_from", ExportHeader: "query_from"},
	"query_to":                         {CompactHeader: "query_to", DetailLabel: "query_to", ExportHeader: "query_to"},
	"target_from":                      {CompactHeader: "target_from", DetailLabel: "target_from", ExportHeader: "target_from"},
	"target_to":                        {CompactHeader: "target_to", DetailLabel: "target_to", ExportHeader: "target_to"},
	"bitscore":                         {CompactHeader: "bitscore", DetailLabel: "bitscore", ExportHeader: "bitscore"},
	"mismatches":                       {CompactHeader: "mismatches", DetailLabel: "mismatches", ExportHeader: "mismatches"},
	"gap_openings":                     {CompactHeader: "gap_openings", DetailLabel: "gap_openings", ExportHeader: "gap_openings"},
	"identical":                        {CompactHeader: "identical", DetailLabel: "identical", ExportHeader: "identical"},
	"positives":                        {CompactHeader: "positives", DetailLabel: "positives", ExportHeader: "positives"},
	"gaps":                             {CompactHeader: "gaps", DetailLabel: "gaps", ExportHeader: "gaps"},
	"jbrowse_name":                     {CompactHeader: "jbrowse_name", DetailLabel: "jbrowse_name", ExportHeader: "jbrowse_name"},
	"target_id":                        {CompactHeader: "target_id", DetailLabel: "target_id", ExportHeader: "target_id"},
	"transcript_id":                    {CompactHeader: "transcript_id", DetailLabel: "transcript_id", ExportHeader: "transcript_id"},
	"defline":                          {CompactHeader: "defline", DetailLabel: "defline", ExportHeader: "defline"},
	"uniprot_accession":                {CompactHeader: "UniProt accession", DetailLabel: "UniProt accession", ExportHeader: "UniProt accession"},
	"uniprot_entry_name":               {CompactHeader: "UniProt entry name", DetailLabel: "UniProt entry name", ExportHeader: "UniProt entry name"},
	"uniprot_reviewed":                 {CompactHeader: "UniProt reviewed", DetailLabel: "UniProt reviewed", ExportHeader: "UniProt reviewed"},
	"uniprot_protein_name":             {CompactHeader: "UniProt protein name", DetailLabel: "UniProt protein name", ExportHeader: "UniProt protein name"},
	"uniprot_gene_names":               {CompactHeader: "UniProt gene names", DetailLabel: "UniProt gene names", ExportHeader: "UniProt gene names"},
	"uniprot_organism":                 {CompactHeader: "UniProt organism", DetailLabel: "UniProt organism", ExportHeader: "UniProt organism"},
	"uniprot_organism_id":              {CompactHeader: "UniProt organism ID", DetailLabel: "UniProt organism ID", ExportHeader: "UniProt organism ID"},
	"uniprot_keywords":                 {CompactHeader: "UniProt keywords", DetailLabel: "UniProt keywords", ExportHeader: "UniProt keywords"},
	"uniprot_ec":                       {CompactHeader: "UniProt EC", DetailLabel: "UniProt EC", ExportHeader: "UniProt EC"},
	"uniprot_go":                       {CompactHeader: "UniProt GO", DetailLabel: "UniProt GO", ExportHeader: "UniProt GO"},
	"uniprot_go_ids":                   {CompactHeader: "UniProt GO IDs", DetailLabel: "UniProt GO IDs", ExportHeader: "UniProt GO IDs"},
	"uniprot_function":                 {CompactHeader: "UniProt function", DetailLabel: "UniProt function", ExportHeader: "UniProt function"},
	"uniprot_catalytic_activity":       {CompactHeader: "UniProt catalytic activity", DetailLabel: "UniProt catalytic activity", ExportHeader: "UniProt catalytic activity"},
	"uniprot_pathway":                  {CompactHeader: "UniProt pathway", DetailLabel: "UniProt pathway", ExportHeader: "UniProt pathway"},
	"uniprot_subcellular_location":     {CompactHeader: "UniProt subcellular location", DetailLabel: "UniProt subcellular location", ExportHeader: "UniProt subcellular location"},
	"uniprot_protein_existence":        {CompactHeader: "UniProt protein existence", DetailLabel: "UniProt protein existence", ExportHeader: "UniProt protein existence"},
	"uniprot_annotation_score":         {CompactHeader: "UniProt annotation score", DetailLabel: "UniProt annotation score", ExportHeader: "UniProt annotation score"},
	"uniprot_fragment":                 {CompactHeader: "UniProt fragment", DetailLabel: "UniProt fragment", ExportHeader: "UniProt fragment"},
	"uniprot_sequence_caution":         {CompactHeader: "UniProt sequence caution", DetailLabel: "UniProt sequence caution", ExportHeader: "UniProt sequence caution"},
	"uniprot_pfam":                     {CompactHeader: "UniProt Pfam", DetailLabel: "UniProt Pfam", ExportHeader: "UniProt Pfam"},
	"uniprot_interpro":                 {CompactHeader: "UniProt InterPro", DetailLabel: "UniProt InterPro", ExportHeader: "UniProt InterPro"},
	"uniprot_domain":                   {CompactHeader: "UniProt domain", DetailLabel: "UniProt domain", ExportHeader: "UniProt domain"},
	"uniprot_region":                   {CompactHeader: "UniProt region", DetailLabel: "UniProt region", ExportHeader: "UniProt region"},
	"uniprot_motif":                    {CompactHeader: "UniProt motif", DetailLabel: "UniProt motif", ExportHeader: "UniProt motif"},
	"uniprot_active_site":              {CompactHeader: "UniProt active site", DetailLabel: "UniProt active site", ExportHeader: "UniProt active site"},
	"uniprot_binding_site":             {CompactHeader: "UniProt binding site", DetailLabel: "UniProt binding site", ExportHeader: "UniProt binding site"},
	"uniprot_alphafolddb":              {CompactHeader: "UniProt AlphaFoldDB", DetailLabel: "UniProt AlphaFoldDB", ExportHeader: "UniProt AlphaFoldDB"},
	"uniprot_pdb":                      {CompactHeader: "UniProt PDB", DetailLabel: "UniProt PDB", ExportHeader: "UniProt PDB"},
	"uniprot_canonical_length":         {CompactHeader: "UniProt canonical\nlength", DetailLabel: "UniProt canonical length", ExportHeader: "UniProt canonical length"},
	"interpro_entry_name":              {CompactHeader: "InterPro entry name", DetailLabel: "InterPro entry name", ExportHeader: "InterPro entry name"},
	"interpro_entry_type":              {CompactHeader: "InterPro entry type", DetailLabel: "InterPro entry type", ExportHeader: "InterPro entry type"},
	"interpro_coverage_percent":        {CompactHeader: "InterPro coverage (%)", DetailLabel: "InterPro coverage (%)", ExportHeader: "InterPro coverage (%)"},
	"interpro_match_regions":           {CompactHeader: "InterPro match regions", DetailLabel: "InterPro match regions", ExportHeader: "InterPro match regions"},
	"interpro_accessions":              {CompactHeader: "InterPro accessions", DetailLabel: "InterPro accessions", ExportHeader: "InterPro accessions"},
	"interpro_signature_accessions":    {CompactHeader: "InterPro signature accessions", DetailLabel: "InterPro signature accessions", ExportHeader: "InterPro signature accessions"},
	"interpro_pfam_accessions":         {CompactHeader: "InterPro Pfam accessions", DetailLabel: "InterPro Pfam accessions", ExportHeader: "InterPro Pfam accessions"},
}

var supplementalColumnHelpText = map[string]string{
	"row": columnHelp(
		"Stable 1-based row number generated when the table is exported or rendered. It is not source-database biology; it is an audit handle that lets the UI, exported files, and PDF report refer to the same row unambiguously.",
		"导出或渲染表格时生成的稳定 1-based 行号。它不是源数据库中的生物学字段，而是一个审计定位编号，用来让界面、导出文件和 PDF report 无歧义地指向同一行。",
		"テーブルの出力または描画時に生成される安定した 1-based 行番号です。元データベース由来の生物学項目ではなく、UI、出力ファイル、PDF report が同じ行を曖昧さなく参照するための監査用ハンドルです。",
	),
	"gff_seqid": columnHelp(
		"Sequence-region identifier from the matched lemna GFF3 feature. It tells you which reference sequence, chromosome, scaffold, or contig the row came from, and is the first coordinate anchor for checking the original release annotation.",
		"匹配到的 lemna GFF3 特征所在的序列区域编号。它说明这一行来自哪个参考序列、染色体、scaffold 或 contig，是回查原始发布注释时最基础的坐标锚点。",
		"一致した lemna GFF3 feature の sequence-region ID です。どの参照配列、染色体、scaffold、contig 由来かを示し、元リリース注釈へ戻るときの最初の座標アンカーになります。",
	),
	"gff_source": columnHelp(
		"Source field from the matched lemna GFF3 feature. It records which annotation source or pipeline emitted the feature in the release file and can help separate different annotation origins inside the same genome release.",
		"匹配到的 lemna GFF3 特征中的 source 字段。它记录该特征在发布文件中来自哪个注释来源或流程，有助于区分同一基因组发布中的不同注释来源。",
		"一致した lemna GFF3 feature の source 欄です。どの注釈ソースやパイプラインがその feature を生成したかを示し、同じ genome release 内の注釈由来の違いを見分ける助けになります。",
	),
	"gff_type": columnHelp(
		"Feature type from the matched lemna GFF3 row, such as gene, mRNA, or transcript. It shows what biological annotation unit actually matched the keyword search and is important when one search term can hit both gene and transcript features.",
		"匹配到的 lemna GFF3 行中的 feature type，例如 gene、mRNA 或 transcript。它说明真正与关键词匹配的生物学注释单元是什么，尤其在一个搜索词同时命中 gene 和 transcript 时很重要。",
		"一致した lemna GFF3 行の feature type です。gene、mRNA、transcript などを示し、実際にどの生物学的注釈単位がキーワード検索に一致したかを表します。",
	),
	"gff_start": columnHelp(
		"Start coordinate of the matched lemna GFF3 feature in the source release file. Use it with gff_end and gff_strand when checking the original genomic interval.",
		"匹配到的 lemna GFF3 特征在源发布文件中的起始坐标。与 gff_end 和 gff_strand 结合使用，可回查原始基因组区间。",
		"一致した lemna GFF3 feature の開始座標です。gff_end と gff_strand と合わせて、元の genomic interval を確認するために使います。",
	),
	"gff_end": columnHelp(
		"End coordinate of the matched lemna GFF3 feature in the source release file. Together with gff_start it defines the span of the feature that produced the row.",
		"匹配到的 lemna GFF3 特征在源发布文件中的终止坐标。它与 gff_start 一起定义生成这一行结果的特征跨度。",
		"一致した lemna GFF3 feature の終了座標です。gff_start と合わせて、この行を生んだ feature の範囲を定義します。",
	),
	"gff_score": columnHelp(
		"GFF3 score field from the matched lemna feature. Many releases leave it empty or use a source-specific numeric value, so it should be interpreted only in the context of that release's annotation conventions.",
		"匹配到的 lemna 特征中的 GFF3 score 字段。很多发布会将其留空，或填入来源特定的数值，因此只能结合该发布版本的注释约定来解释。",
		"一致した lemna feature の GFF3 score 欄です。多くのリリースでは空欄だったりソース固有の数値だったりするため、その release の注釈慣習の中でのみ解釈すべきです。",
	),
	"gff_strand": columnHelp(
		"Strand field from the matched lemna GFF3 feature. It indicates whether the annotated feature lies on the forward or reverse strand of the reference sequence.",
		"匹配到的 lemna GFF3 特征中的 strand 字段。它表示该注释特征位于参考序列的正链还是负链。",
		"一致した lemna GFF3 feature の strand 欄です。注釈 feature が参照配列の順鎖か逆鎖かを示します。",
	),
	"gff_phase": columnHelp(
		"Phase frame field from the matched lemna GFF3 feature. It is mainly meaningful for CDS-like features and may be empty for gene or transcript rows.",
		"匹配到的 lemna GFF3 特征中的 phase 字段。它主要对 CDS 类特征有意义，而对 gene 或 transcript 行通常可能为空。",
		"一致した lemna GFF3 feature の phase 欄です。主に CDS 系 feature で意味を持ち、gene や transcript 行では空のことがあります。",
	),
	"gff_attributes": columnHelp(
		"Original raw GFF3 attribute string from the matched lemna feature. It preserves the un-simplified source annotation text so that parsed attr_* columns can always be traced back to the exact release-file content.",
		"匹配到的 lemna 特征中的原始 GFF3 attributes 字符串。它保留未简化的源注释文本，使解析后的 attr_* 列始终可以追溯到发布文件中的原始内容。",
		"一致した lemna feature の生の GFF3 attribute 文字列です。整形前の注釈テキストを保持し、解析された attr_* 列を常に元の release file 内容へ追跡できるようにします。",
	),
	"lemna_release": columnHelp(
		"lemna release directory or release identifier used to generate this row. It tells you exactly which locally selected release snapshot supplied the GFF3 and related release-backed metadata.",
		"生成这一行时使用的 lemna release 目录或发布标识。它说明究竟是哪一个本地选定的发布快照提供了 GFF3 及相关发布版元数据。",
		"この行の生成に使われた lemna release directory または release ID です。どのローカル選択済み release snapshot が GFF3 と関連 metadata を供給したかを正確に示します。",
	),
	"lemna_gff_url": columnHelp(
		"Direct lemna GFF3 download URL already associated with the chosen release. It records the exact release-file address the workflow used or cached when building the keyword rows.",
		"与所选 release 关联的 lemna GFF3 直接下载 URL。它记录工作流在构建关键词结果行时使用或缓存的确切发布文件地址。",
		"選択された release に対応する lemna GFF3 の直接ダウンロード URL です。キーワード行を構築するときにワークフローが使用またはキャッシュした正確な release file アドレスを記録します。",
	),
	"attr_ID": columnHelp(
		"Parsed ID attribute from the matched lemna GFF3 feature. This is the raw source-level feature identifier before the program chooses display labels, transcript IDs, or sequence IDs for easier downstream use.",
		"匹配到的 lemna GFF3 特征中解析出的 ID 属性。这是程序在选择更易用的 display label、transcript ID 或 sequence ID 之前的原始源级特征编号。",
		"一致した lemna GFF3 feature から解析した ID 属性です。表示用 label、transcript ID、sequence ID をプログラムが選ぶ前の、生の source-level feature ID を表します。",
	),
	"attr_Name": columnHelp(
		"Parsed Name attribute from the matched lemna GFF3 feature. It often contains a human-readable feature name or transcript/protein-style label supplied by the release annotation.",
		"匹配到的 lemna GFF3 特征中解析出的 Name 属性。它通常包含由发布版注释提供的人类可读特征名称，或 transcript/protein 风格的标签。",
		"一致した lemna GFF3 feature から解析した Name 属性です。release annotation が付けた、人間が読みやすい feature 名や transcript/protein 風のラベルを含むことがよくあります。",
	),
	"attr_Parent": columnHelp(
		"Parsed Parent attribute from the matched lemna GFF3 feature. It links the current feature back to its parent annotation object and is useful for reconstructing gene-transcript relationships from the raw release data.",
		"匹配到的 lemna GFF3 特征中解析出的 Parent 属性。它把当前特征连接回其父级注释对象，有助于从原始发布数据中重建 gene-transcript 关系。",
		"一致した lemna GFF3 feature から解析した Parent 属性です。現在の feature を親注釈オブジェクトへ結び付け、生の release data から gene-transcript 関係を再構築するのに役立ちます。",
	),
	"attr_Alias": columnHelp(
		"Parsed Alias attribute from the matched lemna GFF3 feature. It often stores alternate names, symbols, or legacy identifiers that help match user keywords against non-primary labels.",
		"匹配到的 lemna GFF3 特征中解析出的 Alias 属性。它常用于保存别名、符号或旧编号，有助于把用户关键词匹配到非主名称。",
		"一致した lemna GFF3 feature から解析した Alias 属性です。別名、記号、旧 ID を保持することが多く、ユーザーのキーワードを primary ではないラベルへマッチさせる助けになります。",
	),
	"attr_Dbxref": columnHelp(
		"Parsed Dbxref attribute from the matched lemna GFF3 feature. It preserves source cross-references that can point to external identifiers or release-internal linked records.",
		"匹配到的 lemna GFF3 特征中解析出的 Dbxref 属性。它保留源交叉引用，可指向外部编号或发布版内部的关联记录。",
		"一致した lemna GFF3 feature から解析した Dbxref 属性です。外部 ID や release 内部の関連レコードを指しうる source cross-reference を保持します。",
	),
	"attr_product": columnHelp(
		"Parsed product attribute from the matched lemna GFF3 feature. It is usually one of the most important raw annotation texts because it often carries the source's primary product or protein-name description.",
		"匹配到的 lemna GFF3 特征中解析出的 product 属性。它通常是最重要的原始注释文本之一，因为其中常带有源数据库的主要产物或蛋白名称描述。",
		"一致した lemna GFF3 feature から解析した product 属性です。source の主要な product 名や protein 名の説明を含むことが多く、最も重要な生注釈テキストの一つです。",
	),
	"attr_description": columnHelp(
		"Parsed description attribute from the matched lemna GFF3 feature. It preserves the release-file description text before the program condenses or prioritizes it into shorter display fields.",
		"匹配到的 lemna GFF3 特征中解析出的 description 属性。它保留发布文件中的描述文本，优先级整理或缩短成更简洁显示字段之前的原貌。",
		"一致した lemna GFF3 feature から解析した description 属性です。プログラムが短い表示欄へ要約・優先付けする前の、release file 由来の説明テキストを保持します。",
	),
	"attr_Note": columnHelp(
		"Parsed Note attribute from the matched lemna GFF3 feature. It preserves free-text annotation notes exactly as written in the release file when the original key used capitalized Note.",
		"匹配到的 lemna GFF3 特征中解析出的 Note 属性。它保留发布文件中以大写 Note 键写入的自由文本注释说明。",
		"一致した lemna GFF3 feature から解析した Note 属性です。元のキーが大文字 Note だった場合の自由記述注釈メモを、そのまま保持します。",
	),
	"attr_note": columnHelp(
		"Parsed note attribute from the matched lemna GFF3 feature. It preserves free-text annotation notes exactly as written in the release file when the original key used lowercase note.",
		"匹配到的 lemna GFF3 特征中解析出的 note 属性。它保留发布文件中以小写 note 键写入的自由文本注释说明。",
		"一致した lemna GFF3 feature から解析した note 属性です。元のキーが小文字 note だった場合の自由記述注釈メモを、そのまま保持します。",
	),
	"attr_gene_id": columnHelp(
		"Parsed gene_id attribute from the matched lemna GFF3 feature. It is the raw gene-level identifier as written by the release annotation.",
		"匹配到的 lemna GFF3 特征中解析出的 gene_id 属性。它是发布版注释原样写出的基因层级编号。",
		"一致した lemna GFF3 feature から解析した gene_id 属性です。release annotation がそのまま記録した gene-level ID を表します。",
	),
	"attr_transcript_id": columnHelp(
		"Parsed transcript_id attribute from the matched lemna GFF3 feature. It records the source transcript identifier before the program decides which identifier to promote into the main transcript column.",
		"匹配到的 lemna GFF3 特征中解析出的 transcript_id 属性。它记录源转录本编号，在程序决定把哪个编号提升为主 transcript 列之前保留原始值。",
		"一致した lemna GFF3 feature から解析した transcript_id 属性です。どの ID を主要 transcript 列へ昇格させるかをプログラムが決める前の、生の source transcript ID を保持します。",
	),
	"attr_protein_id": columnHelp(
		"Parsed protein_id attribute from the matched lemna GFF3 feature. It preserves the source protein identifier exactly as stated in the release file.",
		"匹配到的 lemna GFF3 特征中解析出的 protein_id 属性。它按发布文件原样保留源蛋白编号。",
		"一致した lemna GFF3 feature から解析した protein_id 属性です。release file に書かれた source protein ID をそのまま保持します。",
	),
	"attr_protein": columnHelp(
		"Parsed protein attribute from the matched lemna GFF3 feature. Some releases use this instead of protein_id for the protein-style identifier associated with the feature.",
		"匹配到的 lemna GFF3 特征中解析出的 protein 属性。某些发布版本会用它而不是 protein_id 来记录与该特征关联的蛋白风格编号。",
		"一致した lemna GFF3 feature から解析した protein 属性です。release によっては protein_id の代わりに、feature に対応する protein 風 ID をここへ書きます。",
	),
	"attr_protein_accession": columnHelp(
		"Parsed protein_accession attribute from the matched lemna GFF3 feature. It preserves the source accession-style protein identifier when the release provides one separately from transcript-like IDs.",
		"匹配到的 lemna GFF3 特征中解析出的 protein_accession 属性。当发布版把 accession 风格蛋白编号与 transcript 风格编号分开提供时，它保留该源 accession。",
		"一致した lemna GFF3 feature から解析した protein_accession 属性です。release が accession 形式の protein ID を transcript 風 ID と分けて提供する場合に、その source accession を保持します。",
	),
	"attr_locus": columnHelp(
		"Parsed locus attribute from the matched lemna GFF3 feature. It preserves a release-supplied locus-style identifier that may be more stable than display labels in some releases.",
		"匹配到的 lemna GFF3 特征中解析出的 locus 属性。它保留发布版提供的 locus 风格编号，在某些发布中这类编号可能比显示标签更稳定。",
		"一致した lemna GFF3 feature から解析した locus 属性です。release が与えた locus 形式 ID を保持し、表示ラベルより安定している場合があります。",
	),
	"attr_comment": columnHelp(
		"Parsed comment attribute from the matched lemna GFF3 feature. It preserves source-side comment text without forcing it into the shorter main comment field.",
		"匹配到的 lemna GFF3 特征中解析出的 comment 属性。它保留源侧的备注文本，而不会强行压缩进较短的主 comment 字段。",
		"一致した lemna GFF3 feature から解析した comment 属性です。短い主 comment 欄へ無理に詰め込まず、source 側の注記テキストを保持します。",
	),
	"ahrd_protein_accession": columnHelp(
		"AHRD protein accession associated with the matched lemna row. It records the accession key used inside the already-loaded AHRD annotation table.",
		"与匹配到的 lemna 行关联的 AHRD protein accession。它记录已加载 AHRD 注释表内部使用的 accession 键。",
		"一致した lemna 行に関連付けられた AHRD protein accession です。すでに読み込まれた AHRD 注釈表の中で使われた accession key を記録します。",
	),
	"ahrd_blast_hit_accession": columnHelp(
		"AHRD BLAST-hit accession associated with the matched lemna row. It indicates which external hit or mapped accession the AHRD annotation record was built around.",
		"与匹配到的 lemna 行关联的 AHRD BLAST hit accession。它说明 AHRD 注释记录是围绕哪个外部命中或映射 accession 构建的。",
		"一致した lemna 行に関連付けられた AHRD BLAST-hit accession です。AHRD 注釈レコードがどの外部ヒットまたは accession マッピングを基準にしているかを示します。",
	),
	"ahrd_quality_code": columnHelp(
		"AHRD quality code associated with the matched lemna row. It is a compact confidence/quality indicator from the AHRD annotation output and should be interpreted according to the AHRD release conventions used by the source data.",
		"与匹配到的 lemna 行关联的 AHRD quality code。它是 AHRD 注释输出中的紧凑质量/置信度标记，应结合源数据所用 AHRD 发布约定来解释。",
		"一致した lemna 行に関連付けられた AHRD quality code です。AHRD 注釈出力の簡潔な品質・信頼度指標であり、source data が使う AHRD release の慣習に沿って解釈します。",
	),
	"ahrd_human_readable_description": columnHelp(
		"Human-readable AHRD description associated with the matched lemna row. It is often the most interpretable AHRD annotation sentence and can summarize likely protein function more clearly than raw accession fields.",
		"与匹配到的 lemna 行关联的 AHRD 人类可读描述。它通常是 AHRD 中最容易理解的说明句，比原始 accession 字段更清楚地概括可能的蛋白功能。",
		"一致した lemna 行に関連付けられた、人が読みやすい AHRD description です。生の accession 欄より分かりやすく、推定されるタンパク質機能を要約することが多いです。",
	),
	"ahrd_interpro": columnHelp(
		"InterPro-related annotation text carried by the already-loaded AHRD record. It is not a fresh InterPro lookup performed for the report; it is whatever InterPro-linked text the AHRD table already contained for this row.",
		"已加载 AHRD 记录中携带的 InterPro 相关注释文本。它不是为了 report 新跑的 InterPro 查询，而是该 AHRD 表本来就已有的 InterPro 关联文本。",
		"すでに読み込まれた AHRD レコードに含まれていた InterPro 関連テキストです。report のために新しく InterPro を引いたものではなく、その AHRD 表が元から持っていた内容です。",
	),
	"ahrd_gene_ontology_term": columnHelp(
		"Gene Ontology term text carried by the already-loaded AHRD record. It preserves GO-style annotation text already present in the AHRD table for the matched row.",
		"已加载 AHRD 记录中携带的 Gene Ontology 术语文本。它保留的是匹配行在 AHRD 表里本来就已有的 GO 风格注释文本。",
		"すでに読み込まれた AHRD レコードに含まれていた Gene Ontology term テキストです。一致行について AHRD 表が元から持っていた GO 風注釈を保持します。",
	),
}

func ColumnHelpText(id string) string {
	key := normalizeColumnHelpID(id)
	if key == "" {
		return ""
	}
	if help := strings.TrimSpace(columnHelpText[key]); help != "" {
		return help
	}
	if help := strings.TrimSpace(supplementalColumnHelpText[key]); help != "" {
		return help
	}
	return strings.TrimSpace(dynamicColumnHelpText(key))
}

func ColumnCanonicalID(id string) string {
	key := normalizeColumnHelpID(id)
	switch key {
	case "search_tern":
		return "search_term"
	case "discripition":
		return "description"
	case "gnome":
		return "genome"
	case "aliases":
		return "alias"
	default:
		return key
	}
}

func ColumnCompactHeader(id string, options ColumnDisplayOptions) string {
	return columnLabel(id, options, "compact")
}

func ColumnDetailLabel(id string, options ColumnDisplayOptions) string {
	return columnLabel(id, options, "detail")
}

func ColumnExportHeader(id string, options ColumnDisplayOptions) string {
	return columnLabel(id, options, "export")
}

func ColumnHelpChinese(id string) string {
	return extractColumnHelpSection(ColumnHelpText(id), "中文：", "\n日本語：")
}

func ColumnHelpJapanese(id string) string {
	return extractColumnHelpSection(ColumnHelpText(id), "日本語：", "")
}

func KnownColumnHelpIDs() []string {
	seen := map[string]struct{}{}
	add := func(values []string) {
		for _, value := range values {
			key := normalizeColumnHelpID(value)
			if key != "" {
				seen[key] = struct{}{}
			}
		}
	}

	for _, ids := range keywordDisplayColumnIDsByDatabase {
		add(ids)
	}
	for _, ids := range keywordDetailColumnIDsByDatabase {
		add(ids)
	}
	for _, ids := range keywordExportColumnIDsByDatabase {
		add(ids)
	}
	for _, ids := range keywordReportExtraColumnIDsByDatabase {
		add(ids)
	}
	for _, byProgram := range blastDisplayBaseColumnIDsByDatabaseProgram {
		for _, ids := range byProgram {
			add(ids)
		}
	}
	add(blastDisplayUniProtColumnIDs)
	add(blastDisplayInterProColumnIDs)
	for _, byProgram := range blastDetailBaseColumnIDsByDatabaseProgram {
		for _, ids := range byProgram {
			add(ids)
		}
	}
	add(blastDetailUniProtColumnIDs)
	add(blastDetailInterProColumnIDs)
	for _, ids := range blastExportBaseColumnIDsByDatabase {
		add(ids)
	}
	add(blastExportUniProtColumnIDs)
	add(blastExportInterProColumnIDs)

	for key := range supplementalColumnHelpText {
		seen[key] = struct{}{}
	}
	for key := range columnHelpText {
		seen[normalizeColumnHelpID(key)] = struct{}{}
	}
	for key := range columnHelpAliases {
		seen[normalizeColumnHelpID(key)] = struct{}{}
	}

	out := make([]string, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func KeywordDisplayColumnIDs(database string) []string {
	return copyColumnIDs(keywordDisplayColumnIDsByDatabase[normalizedDatabaseKey(database)])
}

func KeywordDetailColumnIDs(database string) []string {
	return copyColumnIDs(keywordDetailColumnIDsByDatabase[normalizedDatabaseKey(database)])
}

func KeywordExportColumnIDs(database string, includeProteinID bool, extraHeaders []string) []string {
	base := copyColumnIDs(keywordExportColumnIDsByDatabase[normalizedDatabaseKey(database)])
	if !includeProteinID {
		base = filteredColumnIDs(base, "protein_id")
	}
	return append(base, copyColumnIDs(extraHeaders)...)
}

func KeywordReportColumnIDs(database string, includeProteinID bool, observedExtraHeaders []string) []string {
	base := mergeUniqueColumnIDs(
		KeywordExportColumnIDs(database, includeProteinID, nil),
		KeywordDetailColumnIDs(database),
	)
	if !includeProteinID {
		base = filteredColumnIDs(base, "protein_id")
	}
	extras := mergeUniqueColumnIDs(
		copyColumnIDs(keywordReportExtraColumnIDsByDatabase[normalizedDatabaseKey(database)]),
		copyColumnIDs(observedExtraHeaders),
	)
	return append(base, extras...)
}

func BlastDisplayColumnIDs(database string, program string, includeUniProt bool, includeInterPro bool) []string {
	base := blastBaseColumnIDs(blastDisplayBaseColumnIDsByDatabaseProgram, database, program)
	return appendBlastReferenceColumns(base, includeUniProt, includeInterPro, blastDisplayUniProtColumnIDs, blastDisplayInterProColumnIDs)
}

func BlastDetailColumnIDs(database string, program string, includeUniProt bool, includeInterPro bool) []string {
	base := blastBaseColumnIDs(blastDetailBaseColumnIDsByDatabaseProgram, database, program)
	return appendBlastReferenceColumns(base, includeUniProt, includeInterPro, blastDetailUniProtColumnIDs, blastDetailInterProColumnIDs)
}

func BlastExportColumnIDs(database string, includeUniProt bool, includeInterPro bool) []string {
	base := copyColumnIDs(blastExportBaseColumnIDsByDatabase[normalizedDatabaseKey(database)])
	withRefs := make([]string, 0, len(base)+len(blastExportUniProtColumnIDs)+len(blastExportInterProColumnIDs)+2)
	for _, id := range base {
		withRefs = append(withRefs, id)
		if includeUniProt && id == "target_length" {
			withRefs = append(withRefs, "target_uniprot_canonical_length_percent")
		}
		if includeUniProt && id == "query_length" {
			withRefs = append(withRefs, "uniprot_canonical_length")
		}
	}
	if includeUniProt {
		withRefs = append(withRefs, copyColumnIDs(blastExportUniProtColumnIDs)...)
	}
	if includeInterPro {
		withRefs = append(withRefs, copyColumnIDs(blastExportInterProColumnIDs)...)
	}
	return withRefs
}

func BlastReportColumnIDs(database string, program string, includeUniProt bool, includeInterPro bool) []string {
	_ = program
	ids := BlastExportColumnIDs(database, includeUniProt, includeInterPro)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		key := strings.TrimSpace(id)
		if !includeUniProt && blastColumnIsUniProtLike(key) {
			continue
		}
		if !includeInterPro && (blastColumnIsInterProLike(key) || key == "interpro_conserved_region_status") {
			continue
		}
		out = append(out, key)
	}
	return out
}

func normalizeColumnHelpID(id string) string {
	key := strings.TrimSpace(id)
	if key == "" {
		return ""
	}
	if alias, ok := columnHelpAliases[key]; ok {
		return alias
	}
	return key
}

func columnLabel(id string, options ColumnDisplayOptions, kind string) string {
	key := normalizeColumnHelpID(id)
	if key == "" {
		return ""
	}
	if key == "target_uniprot_canonical_length_percent" {
		base := strings.TrimSpace(options.DatabaseDisplay)
		if base == "" {
			base = "target"
		}
		label := base + " target_length / UniProt canonical length (%)"
		if kind == "compact" && options.Multiline {
			return wrapColumnLabel(label)
		}
		return label
	}
	if meta, ok := columnMetadataByID[key]; ok {
		switch kind {
		case "compact":
			if meta.CompactHeader != "" {
				return meta.CompactHeader
			}
		case "detail":
			if meta.DetailLabel != "" {
				return meta.DetailLabel
			}
		case "export":
			if meta.ExportHeader != "" {
				return meta.ExportHeader
			}
		}
	}
	if kind == "compact" && options.Multiline {
		return wrapColumnLabel(key)
	}
	return key
}

func wrapColumnLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return label
	}
	if strings.Contains(label, "/") {
		parts := strings.SplitN(label, "/", 2)
		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])
		if left != "" && right != "" {
			return left + " /\n" + right
		}
	}
	return label
}

func normalizedDatabaseKey(database string) string {
	key := strings.ToLower(strings.TrimSpace(database))
	if key == "" {
		return "phytozome"
	}
	return key
}

func copyColumnIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			out = append(out, id)
		}
	}
	return out
}

func filteredColumnIDs(ids []string, omit string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != omit {
			out = append(out, id)
		}
	}
	return out
}

func mergeUniqueColumnIDs(groups ...[]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, group := range groups {
		for _, id := range group {
			key := strings.TrimSpace(id)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, key)
		}
	}
	return out
}

func blastBaseColumnIDs(catalog map[string]map[string][]string, database string, program string) []string {
	dbKey := normalizedDatabaseKey(database)
	byProgram := catalog[dbKey]
	if len(byProgram) == 0 {
		byProgram = catalog["phytozome"]
	}
	programKey := strings.ToUpper(strings.TrimSpace(program))
	if ids := byProgram[programKey]; len(ids) > 0 {
		return copyColumnIDs(ids)
	}
	if ids := byProgram["default"]; len(ids) > 0 {
		return copyColumnIDs(ids)
	}
	for _, ids := range byProgram {
		return copyColumnIDs(ids)
	}
	return nil
}

func appendBlastReferenceColumns(base []string, includeUniProt bool, includeInterPro bool, uniProtIDs []string, interProIDs []string) []string {
	out := make([]string, 0, len(base)+len(uniProtIDs)+len(interProIDs))
	for _, id := range base {
		if blastColumnIsUniProtLike(id) {
			if includeUniProt {
				out = append(out, id)
			}
			continue
		}
		if blastColumnIsInterProLike(id) && !includeInterPro {
			continue
		}
		out = append(out, id)
	}
	if includeUniProt {
		out = append(out, copyColumnIDs(uniProtIDs)...)
	}
	if includeInterPro {
		out = append(out, copyColumnIDs(interProIDs)...)
	}
	return out
}

func blastColumnIsUniProtLike(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), "uniprot_") || strings.TrimSpace(id) == "target_uniprot_canonical_length_percent"
}

func blastColumnIsInterProLike(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), "interpro_")
}

func dynamicColumnHelpText(id string) string {
	switch {
	case id == "search_type":
		return columnHelp(
			"Search program selected by the new keyword search engine for this row. If the selected program found nothing and wide search produced the hit, the value explicitly records that fallback.",
			"新 keyword 搜索引擎为这一行选择的搜索程序。如果原本选择的程序没有命中，而宽搜索产生了结果，这里会明确记录这个回退。",
			"新しい keyword 検索エンジンがこの行に選んだ検索プログラムです。最初のプログラムで命中せず wide search が結果を返した場合、その fallback も明示します。",
		)
	case strings.HasPrefix(id, "attr_"):
		name := humanizeColumnSuffix(strings.TrimPrefix(id, "attr_"))
		return columnHelp(
			"Parsed lemna GFF3 attribute \""+name+"\" from the matched feature. This column preserves the exact attribute value already present in the current release file so the workflow can expose source-specific annotation detail without inventing a normalized replacement.",
			"从匹配到的 lemna GFF3 特征中解析出的属性 \""+name+"\"。这一列保留当前发布文件中本来就存在的原始属性值，使工作流能够暴露源数据特有的注释细节，而不是编造一个标准化替代值。",
			"一致した lemna GFF3 feature から解析した属性 \""+name+"\" です。この列は現在の release file に元から存在する属性値をそのまま保持し、source 固有の注釈詳細を、勝手に正規化した代替値へ置き換えずに公開します。",
		)
	case strings.HasPrefix(id, "gff_"):
		name := humanizeColumnSuffix(strings.TrimPrefix(id, "gff_"))
		return columnHelp(
			"Raw lemna GFF3 field \""+name+"\" from the matched feature. It comes directly from the already-loaded release annotation and is included so reviewers can inspect the original structural annotation context behind the keyword row.",
			"匹配到的特征中的原始 lemna GFF3 字段 \""+name+"\"。它直接来自已加载的发布版注释，保留这一列是为了让审阅者检查关键词结果行背后的原始结构注释上下文。",
			"一致した feature の生の lemna GFF3 欄 \""+name+"\" です。すでに読み込まれた release annotation から直接来ており、キーワード行の背後にある元の構造注釈コンテキストを確認できるように含めています。",
		)
	case strings.HasPrefix(id, "ahrd_"):
		name := humanizeColumnSuffix(strings.TrimPrefix(id, "ahrd_"))
		return columnHelp(
			"AHRD-derived field \""+name+"\" already attached to the matched lemna row. It is copied from the AHRD data that the workflow had already loaded during the real search and is never refreshed only for UI display or report writing.",
			"已附加到匹配 lemna 行上的 AHRD 派生字段 \""+name+"\"。它来自工作流在真实搜索过程中已经加载好的 AHRD 数据，不会为了界面显示或 report 写入而重新刷新。",
			"一致した lemna 行へすでに付与されていた AHRD 由来フィールド \""+name+"\" です。実際の検索中にワークフローがすでに読み込んだ AHRD data からコピーされるもので、UI 表示や report 作成のためだけに再取得されることはありません。",
		)
	case strings.HasPrefix(id, "lemna_"):
		name := humanizeColumnSuffix(strings.TrimPrefix(id, "lemna_"))
		return columnHelp(
			"lemna release-context field \""+name+"\" captured from the selected release state. It records already-known release metadata used while constructing the keyword rows.",
			"从所选 release 状态中捕获的 lemna 发布上下文字段 \""+name+"\"。它记录的是构建关键词结果行时已经已知的发布版元数据。",
			"選択された release 状態から取得した lemna release context フィールド \""+name+"\" です。キーワード行を構築する際に、すでに分かっていた release metadata を記録します。",
		)
	default:
		return ""
	}
}

func extractColumnHelpSection(text string, marker string, endMarker string) string {
	text = strings.TrimSpace(text)
	if text == "" || marker == "" {
		return ""
	}
	start := strings.Index(text, marker)
	if start < 0 {
		return ""
	}
	text = text[start+len(marker):]
	if endMarker != "" {
		if end := strings.Index(text, endMarker); end >= 0 {
			text = text[:end]
		}
	}
	return strings.TrimSpace(text)
}

func humanizeColumnSuffix(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	value = strings.ReplaceAll(value, "_", " ")
	return value
}

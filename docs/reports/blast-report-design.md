# BLAST Data Analysis Report Specification

This document defines the PDF `Data Analysis Report` for BLAST-mode exports in `phytozome GO`.
It is an implementation specification, not a marketing document. The report must be generated from a structured report data model and then rendered to PDF.

The BLAST report shares the same audit foundation as the keyword report, but it must not be limited by keyword-mode chapter logic. BLAST has a much richer state surface: one input box accepts many query formats, execution may be server-side or local, rows may be enriched by UniProt and InterPro, family mode may merge multiple query runs, the filter may rebuild checkbox recommendations, and export may target one reviewed query or every reviewed query. The report must explain that complexity clearly enough for a reviewer who did not watch the terminal session.

## Non-Negotiable Rules

1. The report is passive and observational.
   It must only describe data that the current BLAST workflow already collected for normal execution, review, and export.

2. The report must never trigger extra data acquisition.
   Do not query Phytozome, lemna.org, UniProt, InterPro, BLAST services, local BLAST databases, release download indexes, cache refreshers, sequence fetchers, or any other database/API/download path solely to make the report more complete.

3. Missing information is reported honestly.
   If a desired field is not present in the current workflow state, the report must say `Not available in this run` and explain that no extra lookup was performed for report generation.

4. One explicit export action produces at most one report.
   A normal BLAST export that writes selected Excel, raw Excel, peptide text, and a PDF still gets one report. `Export all` also gets one report for the whole batch export action, not one report per generated query file.

5. Report file names use the export file or export folder base name.
   Single-query exports write `<export-file-base>_rpt.pdf`. `Export all` writes `<output-folder-name>_rpt.pdf`. The fallback timestamp form is reserved only for states where no usable base name or folder name exists.

6. The report is an audit artifact.
   It must be readable by a reviewer who did not watch the program run. Use serious explanatory prose, traceability tables, measured timings, parameter dictionaries, formulas, flow diagrams, result summaries, file hashes, and section-level interpretation. Charts and tables support the writing; they must never replace the writing.

7. BLAST dynamic sections are required.
   Sections for UniProt, InterPro, Family BLAST, and the BLAST filter are rendered only when the current export action used or retained state for those features. If a major feature was not used but its absence affects interpretation, write a short `Not used in this run` note in the nearest relevant chapter instead of rendering a full empty chapter.

8. Parameter explanations must be documentation-grade.
   Every numeric threshold, boolean switch, score weight, penalty, ranking rule, evidence requirement, matching rule, and missing-data behavior used by InterPro conserved-region status, Family BLAST grouping/merging, or the BLAST filter must be visible in the report when that feature is used. The report must include both the value used in the run and a plain-language explanation of what the parameter means.

9. Formula disclosure is mandatory for calculated decisions.
   If the report describes InterPro conserved-region status, Family BLAST best-hit merging, or BLAST filter recommendations, it must show the decision formula or decision tree plus full-scope statistics. The PDF must not select individual rows as illustrative picked-row displays.

10. The report must distinguish automatic recommendations from final user selection.
    Filter suggestion flags and user-selected exported rows are related but not identical. The report must show the filter recommendation, final checkbox/export state, and differences between them when filter state is present.

## User Requirement Coverage

This blueprint covers the BLAST-mode report requirements stated for the current implementation cycle.

| Requirement | Applied design |
| --- | --- |
| Use keyword report as the base | Common audit chapters, runtime context, generated-file index, file appendix, passive-data rule, timestamp naming, typography, and file hashing mirror the keyword report. |
| Do not be constrained by keyword content | BLAST has dedicated chapters for input parsing, BLAST execution, external references, Family BLAST, filter analysis, and query/run-level result analytics. |
| Explain versatile BLAST input deeply | Chapter 6 is a major chapter with input-type classification, tokenization/record splitting, FASTA parsing, URL normalization, file-load handling, mixed-input handling, query resolution, skipped/failure outcomes, and natural-language explanation. |
| Show how each input was parsed | The report includes an input trace table and an input flow diagram for every exported query or query group. |
| External references get major coverage when used | Chapter 9 is conditional. It contains one subsection per enabled external database, including purpose, API/source, lookup keys, cache behavior, fallback/matching strategy, output columns, matching outcomes, and charts. |
| InterPro conserved-region parameter details are critical | The InterPro subsection includes every `InterProConservedRegionSettings` value, documentation text, decision flow, scoring evidence, formulas, and status distribution. |
| Family BLAST gets major coverage when used | Chapter 10 is conditional. It covers detection, group naming, parameters, recognized family groups, merge behavior, best-row selection, family source sequences, output naming, and visual summaries. |
| Filter gets the strongest coverage when used | Chapter 11 is conditional and high-detail. It documents every `BlastFilterSettings` parameter, hard rules, soft score, ranking/tie-break rules, filter flags, final user selection, differences, global totals, and per-query distributions. |
| Dynamic report content | Chapters and subsections render based on source database, execution mode, query count, family mode, external references, filter flags, raw export, text export, and `Export all`. |
| Suitable for explaining the program to a teacher/reviewer | The report uses explanatory prose, formulas, flow diagrams, parameter dictionaries, all-query statistics, and source/lineage tables rather than only raw log-style output. |

## Current Program Data Inventory

The report is designed around the data the program already has or can preserve from normal BLAST execution. The implementation should extend workflow state capture where necessary, but only by recording values already used by the workflow. It must not perform report-only lookups.

### Common Software, Runtime, And File Data

These fields mirror the keyword report:

| Data | Current source |
| --- | --- |
| Software display name | `cmd/phytozome-go/main.go` constant `displayName = "phytozome GO"` |
| Author | `cmd/phytozome-go/main.go` constant `author = "wangsychn"` |
| Repository URL | `cmd/phytozome-go/main.go` constant `repoURL = "https://github.com/KiriKirby/phytozome-go"` |
| Version | `cmd/phytozome-go/main.go` variable `version`, defaulting to `dev` unless replaced at build time |
| Go runtime version | `runtime.Version()` |
| Executable path/name | `os.Executable()` |
| Working directory | `os.Getwd()` |
| Application directory | `internal/appfs.ApplicationDir()` |
| Output directory | resolved export output directory |
| Cache directory | `internal/appfs.CacheDir()` |
| Generated files | local file paths after selected Excel, raw Excel, text, and report writing |
| File hashes | SHA-256, SHA-1, and MD5 from local generated artifacts |

### Species And Database Data

Current `model.SpeciesCandidate` fields:

| Field | Meaning in report |
| --- | --- |
| `ProteomeID` | Phytozome proteome/target ID when present |
| `JBrowseName` | source genome/browser identifier |
| `GenomeLabel` | primary genome/species label |
| `CommonName` | common-name display value |
| `ReleaseDate` | release date when source provides it |
| `SearchAlias` | alternate display/search label, often scientific/version label |
| `IsOfficial` | official lemna clone marker when true |
| `DisplayLabel()` | combined human-readable species label already used by the workflow |

Source-specific interpretation:

| Source | BLAST interpretation |
| --- | --- |
| `Phytozome` | Server BLAST and source-specific query URL resolution use the Phytozome client. Target rows may carry Phytozome target IDs, JBrowse names, transcript/sequence IDs, and report URLs. |
| `lemna.org` | BLAST can use detected online server capability or local BLAST+ fallback. Local mode depends on already-discovered/downloaded FASTA paths and BLAST+ tooling. Rows may come from lemna server results or local BLAST result parsing. |

No additional species fields are fetched for the report. If taxonomy ID, genome version, release URL, capability detail, FASTA URL, or local BLAST index path is not already present in workflow state, write it as unavailable.

### BLAST Request Data

Current `model.BlastRequest` fields:

| Field | Meaning in report |
| --- | --- |
| `Species` | selected target species |
| `Sequence` | actual query sequence used for BLAST execution |
| `SequenceKind` | `dna` or `protein` query type |
| `TargetType` | `genome` or `proteome` target type |
| `Program` | `BLASTN`, `BLASTX`, `TBLASTN`, `BLASTP`, or `local:<program>` |
| `EValue` | E-value setting passed to the source/local BLAST path |
| `ComparisonMatrix` | comparison matrix setting such as `BLOSUM62` |
| `WordLength` | word-length setting or `default` |
| `AlignmentsToShow` | requested maximum displayed alignments |
| `AllowGaps` | whether gapped alignments are enabled |
| `FilterQuery` | whether low-complexity/query filtering is enabled |

Program mapping to explain:

| Query kind | Target type | BLAST program |
| --- | --- | --- |
| nucleotide | genome/nucleotide | `BLASTN` |
| nucleotide | proteome/protein | `BLASTX` |
| protein | genome/nucleotide | `TBLASTN` |
| protein | proteome/protein | `BLASTP` |

If `Program` starts with `local:`, the execution mode is local BLAST+ and the displayed program should strip the local prefix for biological interpretation while preserving the execution-mode fact.

### BLAST Input Data

Current `blastQueryItem` fields in workflow:

| Field | Meaning in report |
| --- | --- |
| `RawInput` | raw input record after initial splitting; may be a plain sequence, FASTA record, report URL, loaded file record, or family group synthetic record |
| `LabelName` | user-facing label assigned manually or by auto-identify |
| `Sequence` | resolved query sequence used for BLAST |
| `QuerySource` | structured sequence source metadata when resolved |
| `FamilyName` | derived family group name when Family BLAST grouped this item |
| `MemberLabel` | newline-separated member labels in a family group |
| `FamilySources` | all query sequence sources used by a family export |

Current `model.QuerySequenceSource` fields:

| Field | Meaning in report |
| --- | --- |
| `Sequence` | resolved sequence used for BLAST |
| `OriginalInputURL` | original pasted report URL when input came from a URL |
| `NormalizedURL` | canonical normalized URL used for source resolution |
| `SourceDatabase` | source that resolved the query sequence, such as `phytozome`, `lemna`, or `fasta` |
| `SourceProteomeID` | source proteome ID when known |
| `SourceJBrowseName` | source JBrowse name when known |
| `SourceGenomeLabel` | source genome label when known |
| `LabelName` | label from FASTA header/source metadata when available |
| `Aliases` | alias text from source metadata when available |
| `GeneID` | resolved source gene ID |
| `TranscriptID` | resolved transcript ID |
| `ProteinID` | resolved protein ID |
| `OrganismShort` | source organism short label |
| `Annotation` | parsed annotation/defline text |

The report must treat BLAST input parsing as a first-class audit subject, not a small appendix.

### BLAST Result Data

Current `model.BlastResult` fields:

| Field | Meaning in report |
| --- | --- |
| `JobID` | server job ID or local job identifier/path marker |
| `Message` | source/local message, including Family BLAST merge notes when added |
| `UserOptions` | source-provided option summary when available |
| `RawXML` | raw XML if available from source |
| `Hash` | result hash if set by source/local path |
| `ZUID` | Phytozome-style identifier when available |
| `Rows` | BLAST result rows used for review/export |

Current `model.BlastResultRow` fields include:

| Category | Fields |
| --- | --- |
| Source and run | `SourceDatabase`, `BlastProgram`, `LabelName`, `HitNumber`, `HSPNumber` |
| Hit identity | `Protein`, `SubjectID`, `Species`, `GeneReportURL`, `JBrowseName`, `TargetID`, `SequenceID`, `TranscriptID`, `Defline` |
| Alignment metrics | `EValue`, `PercentIdentity`, `AlignQueryLengthPercent`, `AlignLength`, `Strands`, `QueryID`, `QueryFrom`, `QueryTo`, `TargetFrom`, `TargetTo`, `Bitscore`, `Mismatches`, `GapOpenings`, `Identical`, `Positives`, `Gaps`, `QueryLength`, `TargetLength` |
| UniProt reference | `UniProtReferenceEnabled`, `UniProtAccession`, `UniProtReviewed`, `UniProtProteinName`, `UniProtGeneNames`, `UniProtKeywords`, `UniProtEC`, `UniProtGO`, `TargetUniProtCanonicalLengthPercent`, `UniProtCanonicalLength`, `UniProtEntryName`, `UniProtOrganism`, `UniProtOrganismID`, `UniProtFunction`, `UniProtCatalyticActivity`, `UniProtGOIDs`, `UniProtPathway`, `UniProtSubcellularLocation`, `UniProtProteinExistence`, `UniProtAnnotationScore`, `UniProtFragment`, `UniProtSequenceCaution`, `UniProtPfam`, `UniProtInterPro`, `UniProtDomain`, `UniProtRegion`, `UniProtMotif`, `UniProtActiveSite`, `UniProtBindingSite`, `UniProtAlphaFoldDB`, `UniProtPDB` |
| InterPro reference | `InterProReferenceEnabled`, `InterProConservedRegionStatus`, `InterProEntryName`, `InterProEntryType`, `InterProCoveragePercent`, `InterProMatchRegions`, `InterProAccessions`, `InterProSignatureAccessions`, `InterProPfamAccessions`, `PfamDomain` |

### Export Settings And Output Behavior

Current BLAST export settings and file behavior:

| Setting/behavior | Report interpretation |
| --- | --- |
| `BaseName` | base name used for single-query selected Excel, raw Excel, and peptide text outputs |
| `OutputDir` | resolved output directory |
| `WriteExcel` | selected rows written to `<base>.xlsx` |
| `WriteRawExcel` | all current rows written to `<base>_raw.xlsx` |
| `WriteText` | selected hit peptide records written to `<base>.txt`; query sequence records are prepended when known |
| `WriteReport` | report PDF requested |
| single export | one selected query/family group is exported |
| `Export all` | multiple reviewed runs are exported into one output directory; the report describes the whole export action |
| selected Excel | produced by `export.WriteBlastResultsExcelWithMetadata` using selected rows and row/filter options |
| raw Excel | produced by `export.WriteBlastResultsExcelWithMetadata` using all current rows and filter options |
| peptide text | produced by fetching/using BLAST hit protein sequence records; query-source records are prepended for traceability |

Current BLAST Excel columns are controlled by `prompt.BlastExportColumnIDs(sourceDatabase, includeUniProt, includeInterPro)`.
The report must use the same column registry, dynamic UniProt/InterPro inclusion rules, source-specific display headers, and two-line ratio-header semantics as the export.

## Implemented Workflow Integration Target

The BLAST PDF report should be generated by the real BLAST export subflow for both `Phytozome` and `lemna.org` when the user enables `WriteReport`.

Target integration behavior:

- Add a renderer entry such as `internal/report.RenderBlastPDF`.
- Add a workflow builder such as `internal/workflow/blast_report.go`.
- Extend `internal/report/types.go` with `BlastReportData` while preserving shared common report structs.
- Populate the report data model after requested BLAST export files are written and before the export-complete page is shown.
- Use one report path per explicit export action: `<file-or-folder-base>_rpt.pdf`.
- For single export, describe one selected query or one Family BLAST group.
- For `Export all`, describe every exported run/group and every generated file in one report.
- Inspect selected Excel, raw Excel, and peptide text files after writing; append a planned report-file entry before rendering.
- Do not call `source.DataSource`, Phytozome clients, lemna clients, BLAST clients, sequence fetchers, cache refreshers, download helpers, UniProt, or InterPro from the renderer or report builder for report-only data.
- If the implementation needs data such as input parsing traces, family grouping traces, external reference settings, or filter settings, capture them during the real workflow step that already computed them.

### Minimum New Workflow State To Preserve

The current implementation already carries most row-level data but does not yet retain every explanatory trace needed by the report. The BLAST report implementation should preserve the following during normal operation:

| Needed state | Capture point | Reason |
| --- | --- | --- |
| raw input text or loaded file name/path | `collectBlastQueryItems` / `loadBlastInputFile` | input chapter needs to explain source and record splitting |
| per-record input classification | `parseBlastQueryItems` / `parseBlastQueryRecord` | report should show plain sequence, FASTA, URL, loaded file, mixed line, skipped item |
| split/tokenization trace | `splitBlastInputRecords` | report should explain mixed input and one-query-per-record behavior |
| query resolution outcome | `resolveBlastQueryItemsWithProgress` | report should show resolved source, sequence length, identifiers, failures/skips |
| label method | label collection and auto-identify steps | report should distinguish manual, FASTA label, query-source-derived, auto-identify, export filename fallback |
| BLAST request settings per run | before `submitBlastWithRetry` | report should show actual sequence kind, program, target type, and parameters |
| execution mode and fallback outcome | configure/submit local vs server flow | report should show server/local/local fallback details without inferring later |
| external reference settings | `collectExternalReferenceConfig` | UniProt/InterPro chapters need enabled/disabled status and parameters |
| external reference lookup summaries | UniProt/InterPro enrichment functions | report should show lookup counts, matches, misses, cache/source status when already known |
| InterPro conserved-region settings and formulas | InterPro enrichment functions | report must disclose parameter values and formulas |
| family settings and detected groups | `collectFamilyBlastPlan` / `applyFamilyBlastPlan` | family chapter needs detection and merge audit |
| family merge replaced rows | `mergeFamilyBlastRowsByTarget` | report should quantify duplicate target merging and chosen best-ranked rows |
| filter settings, suggestions, and clear actions | `SelectBlastRows*` and filter callbacks | filter chapter needs parameter disclosure and user-vs-filter comparison |
| selected row numbers and filter flags | row selection result | export traceability, selected/raw Excel matching |
| sequence export records including prepended query records | BLAST text export | sequence chapter should show hit/query sequence status |

If any of these are not captured in the first implementation, the report must state `Not available in this run` rather than recompute by running new lookups. Pure deterministic recomputation from already-captured rows and settings is allowed only when it is guaranteed not to call external systems or mutate state.

## Dynamic Section Policy

The BLAST report uses a stable high-level order with conditional chapters.

| Section | Render condition | If absent |
| --- | --- | --- |
| Cover and executive summary | always | not applicable |
| Generated file index | always | list report PDF and any generated data files; note if no data files were written |
| Runtime and reproducibility context | always | not applicable |
| Time and performance overview | always | unmeasured phases marked unavailable |
| Data source, species, and BLAST target | always | unavailable source fields omitted or explained |
| BLAST input parsing and query resolution | always | if traces missing, still summarize available query-source rows and state trace not captured |
| BLAST execution method and request parameters | always | if job/user options missing, write unavailable |
| Result set and selection analytics | always | if no rows, render no-hit explanation |
| External reference analysis | only if UniProt or InterPro was enabled, or rows carry reference-enabled flags | otherwise short note in result chapter |
| Family BLAST analysis | only if Family BLAST was enabled or exported item has `FamilyName`/`FamilySources` | otherwise short note in input/result chapter when batch had no detected family groups |
| BLAST filter analysis | only if filter settings, filter flags, or filter-clear state is present | otherwise short note that no automatic filter was applied |
| Column dictionary and data lineage | always | use selected/exported columns actually generated |
| Export settings and generation log | always | not applicable |
| Sequence export audit | only if peptide text was requested or query records were prepended | if not requested, one short non-applicable paragraph |
| File technical details appendix | always | not applicable |
| Limitations | always | not applicable |

## Visual And Layout Rules

Use the keyword report's visual system unless this file overrides it.

Shared rules:

- A4 portrait pages with consistent margins, running header, running footer, generation timestamp, and page numbers.
- OS system sans/CJK fonts must be used. Do not silently fall back to ASCII-only PDF core fonts.
- Use restrained scientific/audit styling. Do not make a colorful dashboard.
- Long hashes and long raw identifiers belong in appendix tables or wrapped technical tables, not in the opening summary.
- Every chart must be backed by structured report data and accompanied by prose explaining what the chart means.

BLAST-specific layout additions:

- Use flow diagrams for input parsing, InterPro conserved-region status, Family BLAST grouping/merging, and filter decision logic.
- Use parameter dictionaries with compact grouped sections. Each row has `Parameter`, `Value used`, `Default`, `Meaning`, `Effect in this run`, and `Where used`.
- Do not use picked-row panels, selected rows, or best-ranked rows. Use global totals, every-query summaries, full parameter dictionaries, formulas, and decision flows.
- Use small multiples when comparing multiple BLAST queries in `Export all`: one compact row per query, plus one aggregate chart.
- Prefer `status distribution` charts for UniProt/InterPro/filter outcomes: stacked bars, compact donut charts, or horizontal bars depending on row count.

## Chart Blueprint

Charts must be deterministic, drawn internally by the PDF renderer, and based only on `BlastReportData`.

### Required BLAST Charts

| Chart | Chapter | Data | Purpose |
| --- | --- | --- | --- |
| Export artifact count cards | Chapter 1/2 | generated files | show what this export action produced |
| Time-window bars | Chapter 4 | measured steps | separate input/resolve/BLAST/reference/review/export/report durations where captured |
| Input type mosaic | Chapter 6 | input records by type | show the mixed-input nature of the BLAST input box |
| Input resolution funnel | Chapter 6 | raw records -> parsed records -> resolved queries -> executed queries -> exported queries | explain how pasted content became BLAST jobs |
| Query/run summary bars | Chapter 8 | per-run total and selected rows | show which queries produced hits and which were exported |
| Selection donut | Chapter 8 | selected vs unselected rows in export scope | mirror keyword selection analytics at BLAST row level |
| Alignment metric distributions | Chapter 8 | percent identity, query coverage, E-value buckets, target length ratio when available | summarize biological/technical result strength without judging final truth |
| External reference coverage bars | Chapter 9 | UniProt/InterPro matches, misses, reviewed/unreviewed, status categories | quantify enrichment outcomes |
| InterPro status distribution | Chapter 9 | present/partial/missing/uncertain/blank | expose conserved-region evidence state |
| Family grouping map | Chapter 10 | detected families and member labels | show which queries became each family group |
| Family merge reduction chart | Chapter 10 | pre-merge vs post-merge rows by family | show duplicate target collapse |
| Filter recommendation outcome | Chapter 11 | filter keep/remove flags | show automatic filter recommendations |
| Filter vs final selection matrix | Chapter 11 | filter flags and final selected rows | reveal user overrides and final export decision |
| BLAST quality check severity bars | Chapter 8/12 | BLAST-specific quality checks | separate pass, warning, not requested, and unavailable checks |
| Column completeness chart | Chapter 12 | exported column cells | explain blank/reference-column coverage |
| Sequence export completeness chart | Chapter 14 | sequence records | show hit records, prepended query records, skipped/unavailable records |

### Input Type Mosaic

Use a horizontal segmented bar or tile mosaic:

| Type | Color group | Meaning |
| --- | --- | --- |
| FASTA record | primary | input started with `>` and parsed into header + sequence |
| report URL | secondary | normalized Phytozome report URL resolved to a query sequence |
| plain sequence | success | direct sequence tokens/lines used as sequence |
| inline mixed line | warning | one line split into multiple URL/sequence tokens |
| loaded file | purple/slate | input came from `load "file.txt"` |
| skipped/unresolved | error/missing | record had no usable sequence or failed resolution |

The chart must be followed by prose:

`The BLAST input box is intentionally permissive. This run's input was first normalized for line endings, then split into records, then each record was classified and resolved. The report records that interpretation so the generated files can be traced back to the exact user input form.`

### Input Resolution Funnel

Render a funnel with counts:

`Raw input blocks -> parsed records -> valid FASTA/URL/plain records -> resolved query sequences -> submitted/executed BLAST jobs -> exported query groups`

If Family BLAST is enabled, add:

`executed member jobs -> family groups -> exported family files`

The funnel must explicitly count failures/skips when known.

### External Reference Coverage Charts

UniProt:

- rows with accession vs rows without accession
- reviewed vs unreviewed vs unknown
- canonical length ratio present vs absent
- fragment/caution flags
- annotation fields present: protein name, function, EC, GO, pathway, subcellular location

InterPro:

- rows with InterPro entry vs without entry
- conserved-region status: present, partial, missing, uncertain, blank
- coverage percent buckets: 0/missing, 1-24, 25-69, 70-100
- evidence type presence: Pfam accession, InterPro accession, signature accession, entry type, match regions

### Filter Vs Final Selection Matrix

Use a 2x2 matrix:

| | Final selected | Final unselected |
| --- | --- | --- |
| Filter recommended keep | kept by both | user removed after keep recommendation |
| Filter recommended remove | user rescued after remove recommendation | removed by both |

This matrix is mandatory when filter flags are present. It directly answers whether the final export followed or overrode the automatic filter.

## Table Blueprint

### General Table Rules

- Tables must have concise column headers and wrapped cell text.
- Large technical values such as hashes, raw URLs, long FASTA headers, and long function text should be wrapped or moved to appendix/detail tables.
- Do not show empty tables. If a section has no data, write a note explaining why it is unavailable or not applicable.
- Repeated explanations should be moved into prose above the table.
- Parameter tables must never omit settings that affected the decision logic.

### Required BLAST Tables

| Table | Chapter | Notes |
| --- | --- | --- |
| Generated file index | 2 | BLAST artifact list with query/run/family labels, selected/raw/text/report roles, and appendix links |
| Species/source table | 5 | Database, species, target, execution mode, local/server status |
| BLAST request table | 7 | Program, query kind, target type, E-value, matrix, gaps, filter query, alignments |
| Input trace table | 6 | One row per parsed record, with raw preview, type, parser path, resolved sequence length, source identifiers, outcome |
| Query-source metadata table | 6 | Source database, original URL, normalized URL, gene/transcript/protein, label, organism, annotation |
| Execution run table | 7 | One row per BLAST job/run: run index, label, job ID, mode, result row count, message/hash |
| Row-selection table | 8 | Total/selected/unselected per run or family group |
| External reference parameter table | 9 | UniProt and InterPro settings/lookup rules |
| UniProt outcome table | 9 | accession, reviewed, canonical length, ratio, fragment/caution, annotation fields |
| InterPro conserved-region parameter table | 9 | every `InterProConservedRegionSettings` field |
| InterPro decision formula table | 9 | query evidence, hit evidence, matched items, matched coverage, status |
| Family settings table | 10 | every `FamilyBlastSettings` field |
| Family detected group table | 10 | family name, member labels, detected prefix, original indexes, output file base |
| Family merge table | 10 | target key, member rows, chosen best-ranked row, reason |
| Filter parameter table | 11 | every `BlastFilterSettings` field |
| Filter hard-rule result table | 11 | per-rule pass/fail counts |
| Filter per-query and total statistics | 11 | rows, automatic keep/remove, final selected/unselected, rescued, user removed, agreement |
| BLAST quality-check table | 8/12 | BLAST-specific pass/warning/not-applicable checks with count, rule, source, and interpretation |
| Column dictionary | 12 | generated BLAST columns and lineage |
| Export settings and generation log | 13 | BLAST export settings, selected/raw/TXT/report choices, query/family naming, row/filter options, and generation steps |
| Sequence export audit | 14 | query records and hit records, status, length, source |
| File technical appendix | final | hashes and file metadata |

### Status Vocabulary

Use the keyword report vocabulary plus BLAST-specific status values:

| Status | Meaning |
| --- | --- |
| `pass` | check passed |
| `warning` | reviewer should inspect |
| `not applicable` | feature was not used in this run |
| `not requested` | export/reference/filter option was disabled |
| `not available in this run` | data was not present and report did not fetch it |
| `server` | online BLAST execution path |
| `local` | local BLAST+ execution path |
| `present` | InterPro conserved-region evidence meets present thresholds |
| `partial` | InterPro evidence exists but meets partial thresholds only |
| `missing` | required InterPro-like evidence was not found |
| `uncertain` | evidence exists but does not meet configured thresholds |
| `filter kept` | automatic filter did not mark row for removal |
| `filter removed` | automatic filter marked row for removal |
| `user rescued` | filter marked removal but user selected row for export |
| `user removed` | filter kept row but user left it unselected |

## Narrative Style

The BLAST report should read like a method-and-result audit written by a careful scientific software engineer.

Good report prose:

- explains what happened in chronological order
- names the exact program mechanisms used
- distinguishes query input, target database, hit rows, external reference evidence, automatic suggestions, and final user choices
- uses `not available in this run` instead of pretending data exists
- states when no additional lookup was performed
- treats filter and family mode as transparent, parameterized workflows, not as hidden magic

Avoid:

- raw debug-log dumping
- implying that a BLAST hit is biologically validated solely because it passed a filter
- implying that missing UniProt/InterPro data means a protein lacks biology
- using external-reference results when external references were not enabled
- hiding parameter values behind vague phrases such as `default filter was applied`

Core paragraph shape:

`This report documents a BLAST-mode export generated by phytozome GO. The input records were parsed from the BLAST input box, resolved into query sequences, searched against the selected target species, optionally enriched with external reference evidence, reviewed by the user, and exported as the current generated file set. The PDF describes only data already present in the workflow state during this export. No additional BLAST, database, UniProt, InterPro, sequence-fetch, cache-refresh, or download operation was performed for report generation.`

## Data Availability And Instrumentation Policy

The BLAST report must be transparent about instrumentation gaps.

Rules:

- Report rendering may inspect local files it just generated for metadata and hashes.
- Report rendering may compute deterministic summaries from already-present rows, settings, flags, and generated-file metadata.
- Report rendering may not execute network calls, local BLAST runs, sequence fetches, source refreshes, package manager checks, or OS probing commands.
- If a useful trace was not captured, write `Not available in this run; this value was not captured during the BLAST workflow and no additional lookup was performed for report generation`.
- If a feature was disabled, write `Not requested` or `Not applicable` instead of treating the missing section as a quality warning.

## Final Chapter Order

The final PDF uses this order. Chapter numbers are logical and may shift when conditional chapters are omitted; rendered headings should remain readable even if a conditional chapter is absent.

1. Cover And Executive Summary
2. Generated File Index
3. Software, User, Runtime, And System Context
4. Time Window And Performance Overview
5. Data Source, Species, And BLAST Target Context
6. BLAST Input Parsing And Query Resolution
7. BLAST Execution Method And Request Parameters
8. Result Set, Review, And Selection Analytics
9. External Reference Analysis, if UniProt or InterPro was enabled
10. Family BLAST Analysis, if Family BLAST was enabled or exported
11. BLAST Filter Analysis, if filter settings/flags are present
12. Column Dictionary And Data Lineage
13. Export Settings And Generation Log
14. Sequence Export Audit, if text export was requested; otherwise short not-requested section
15. File Technical Details Appendix
16. Report Limitations And Data Availability Notes

## Chapter Content Contracts

### Chapter 1 Contract: Cover And Executive Summary

Purpose:

- State this is a BLAST-mode export report generated by `phytozome GO`.
- Include database, species, execution mode, BLAST program, query count, exported run/group count, selected row count, total current row count, generated-file count, external-reference status, Family BLAST status, filter status, and export completion timestamp.
- State that no additional lookup was performed for report generation.
- Give the reader enough context to understand whether this report describes a simple one-sequence BLAST export, a batch run, a local BLAST fallback, a Family BLAST merged export, a reference-enriched review, a filter-assisted review, or `Export all`.

Required cards:

- mode: `BLAST`
- database
- target species
- BLAST program
- execution mode: server/local
- query source mode: direct sequence, FASTA, report URL, mixed batch, loaded file, or mixed
- query records parsed
- query records resolved
- BLAST runs executed
- exported runs/groups
- selected rows
- total rows in export scope
- external references: none, UniProt, InterPro, both
- Family BLAST: not used, detected but disabled, enabled
- filter: not used, applied, cleared, applied with user overrides
- generated files
- report time

Required prose:

- first paragraph: describe the exact BLAST path in plain language, with this shape: `The user supplied N query records, the program resolved them into M usable sequences, ran BLASTP against the selected lemna.org species through local BLAST+, enriched rows with UniProt and InterPro, grouped two labels into Family BLAST group PAL, applied a filter suggestion, and exported selected rows to Excel/TXT/PDF.`
- second paragraph: explain report scope and passive-data rules.
- third paragraph: warn that BLAST alignment, external reference matches, Family BLAST grouping, and filter recommendations support review but do not by themselves prove biological function.

Required summary table:

| Field | BLAST-specific content |
| --- | --- |
| Query source summary | count by parsed input type and sequence source |
| Target search summary | database, species, program, server/local mode |
| Evidence summary | alignment rows, UniProt status, InterPro status, filter/family status |
| Export summary | generated files and selected/raw/text/report settings |

Do not use keyword-style phrases such as `search terms` in this chapter. Use `query records`, `query sequences`, `BLAST runs`, `result rows`, and `exported runs/groups`.

### Chapter 2 Contract: Generated File Index

Purpose:

- List files produced by the current export action.
- Keep full hashes in appendix.
- For `Export all`, list every generated selected Excel/raw Excel/text file plus the single report PDF.
- Explain how each generated artifact maps back to a BLAST query, Family BLAST group, or batch export action.

Required columns:

- file name
- type
- size
- role
- query/family/run label if applicable
- BLAST program/execution mode if applicable
- selected/raw row scope
- location
- technical details appendix pointer

BLAST file roles:

| File type | Role text |
| --- | --- |
| selected Excel | selected BLAST rows exported after user review |
| raw Excel | all current BLAST rows for the exported query/run/group |
| peptide text | selected hit peptide records plus prepended query sequence records when available |
| report PDF | Data Analysis Report for the current BLAST export action |

Single export behavior:

- selected Excel, raw Excel, and TXT share one file base name.
- the file index must show the reviewed query label or family name.
- if the export filename was used as fallback label in TXT headers, the index must say so.

`Export all` behavior:

- group the index by exported query/family label.
- each exported run may have its own selected Excel, raw Excel, and TXT.
- the report PDF appears once at the end as the audit artifact for the whole `Export all` action.
- if a run had zero selected rows and produced no files, list it in Chapter 8 rather than pretending it generated an artifact.

Family BLAST behavior:

- list the family name as the primary artifact label.
- show member query labels in a secondary detail row or footnote.
- if TXT prepended multiple member query sequences, mark the TXT role as `family query sequences + selected hit peptides`.

### Chapter 3 Contract: Software, User, Runtime, And System Context

Purpose:

- Record the exact local program context that produced the BLAST export.
- Make local BLAST+ and file-output reproducibility inspectable without running any report-only probes.
- Separate general runtime facts from BLAST-specific operational facts.

Required subsections:

1. Software identity:
   - software name
   - author
   - repository
   - version/build metadata
   - Go runtime

2. User and process:
   - user name
   - home directory
   - host name
   - process ID
   - session/terminal environment values already available

3. Runtime paths:
   - executable path
   - working directory
   - application directory
   - output directory
   - cache directory
   - local BLAST working/cache path when already captured by the workflow
   - generated file directory

4. System facts:
   - OS display name
   - architecture
   - CPU count
   - memory status if already captured, otherwise unavailable

BLAST-specific prose:

- If execution mode is server BLAST, explain that runtime context mainly supports reproducibility of local output files, report rendering, cache location, and environment.
- If execution mode is local BLAST+, explain that runtime context also affects local FASTA/index handling and BLAST+ execution, but the report did not re-run `blastp`, `blastn`, `makeblastdb`, version checks, or PATH probes.
- If local fallback happened after server capability or submission problems, state that the fallback transition must be recorded from workflow state, not inferred after the export.

Forbidden:

- Do not run terminal version commands.
- Do not run BLAST+ version commands.
- Do not scan local FASTA directories only for the report.
- Do not infer local BLAST availability from the current system after the export.

### Chapter 4 Contract: Time Window And Performance Overview

Purpose:

- Show available timings from BLAST input through report rendering.
- Separate user time from active processing time where state is available.
- Reveal whether export time was dominated by file writing, sequence fetching, external references, BLAST execution, report rendering, or user review.

Recommended events:

- BLAST input started
- input parsed
- query resolution started/ended
- labels assigned
- BLAST request configured
- BLAST execution started/ended
- UniProt enrichment started/ended, if enabled
- InterPro enrichment started/ended, if enabled
- Family BLAST grouping/merge completed, if enabled
- review/selection started
- export started
- export ended
- report rendered

If only export-generation steps are currently captured in the first implementation, render those steps and explicitly state earlier timings were not instrumented.

Required chart:

- duration bars for measured steps

Required BLAST timing groups:

| Timing group | Events | Meaning |
| --- | --- | --- |
| Input preparation | raw input, parsing, label collection, query resolution | User text became executable query sequences. |
| BLAST execution | request configuration, submit/run, wait/load results | Server or local BLAST produced hit rows. |
| Reference enrichment | UniProt and InterPro enrichment | Optional external annotations were added after BLAST rows existed. |
| Family/filter review | family grouping/merge, filter application/clear, user selection | Review state and row-selection recommendations were prepared. |
| Export generation | Excel, raw Excel, sequence fetching, TXT, file metadata, PDF rendering | Files for the current export action were written. |

Presentation rules:

- Use one horizontal duration bar section for high-level groups and one table for exact steps.
- If user review time is not measured, do not mix it with processing time.
- If batch execution timing is available per query, show a small multiple by query/run.
- If `Export all` suppresses nested progress modals and only aggregate timing is available, state that per-run export timings were not instrumented.
- If external reference enrichment failed softly and rows continued without references, show the attempted phase only if that state was captured.

### Chapter 5 Contract: Data Source, Species, And BLAST Target Context

Purpose:

- Describe selected target database/species and execution capability.
- Separate the selected target source from the query sequence source. This is essential because a query can come from a FASTA header, a Phytozome report URL, or another source while the target species is selected separately.

Required content:

- database/source table
- selected species table
- target interpretation table: query kind, target type, BLAST program
- server/local execution status
- local BLAST+ and FASTA capability details when already present
- source notes for Phytozome and lemna.org
- query source vs target source comparison
- execution capability/fallback table when the workflow already recorded capability details

Source-specific notes:

- Phytozome: describe selected species, proteome/JBrowse context, server BLAST/source resolver path when known.
- lemna.org: describe capability detection, online server program availability, local fallback possibility, and local FASTA/BLAST+ requirements when already used.

Required tables:

1. Target species table:
   - database
   - display label
   - genome label
   - common name
   - search alias
   - JBrowse name
   - proteome ID
   - release date
   - lemna official marker
   - source notes

2. Query source comparison table:
   - query/run label
   - query source type: direct sequence, FASTA, Phytozome URL, source resolver, family member
   - query source database
   - original URL
   - normalized URL
   - source gene/transcript/protein IDs
   - target database/species
   - whether query source and target source differ

3. Execution capability table:
   - requested program
   - resolved program
   - server capability status
   - local capability status
   - local FASTA/protein/nucleotide availability when captured
   - fallback decision if any

Required prose:

- Explain that `gene_report_url` in result rows is target-side hit provenance, while `OriginalInputURL`/`NormalizedURL` in query source metadata describe query-side input provenance.
- If query source and target source differ, explicitly state that this is a cross-source/cross-species BLAST query and is expected behavior.
- If source capability details are absent, say they were not preserved in the current workflow state and were not checked again for report generation.

### Chapter 6 Contract: BLAST Input Parsing And Query Resolution

This is a major chapter. It must be more detailed than keyword input documentation.

Purpose:

- Explain the BLAST input box as a permissive parser.
- Show exactly how raw user input became one or more query sequences.
- Document mixed input cases, FASTA parsing, report URL handling, loaded files, label assignment, resolution failures, and skipped records.

Required natural-language introduction:

`BLAST mode uses one input surface for several query forms. The program first treats the text as user input, then normalizes line endings, optionally expands a load command, splits the text into records, classifies each record, resolves metadata and sequence content when possible, and finally submits only records with usable sequence content. This chapter records those parser decisions so the exported BLAST rows can be traced back to the user's original input.`

Current parser behavior to document:

1. `load "file.txt"` command:
   - recognized only when the trimmed input starts with `load`
   - filename is stripped to `filepath.Base`
   - only `.txt` files are accepted
   - file is read from the application directory
   - loaded file content then enters normal BLAST parsing

2. Line-ending normalization:
   - carriage returns are removed
   - outer whitespace is trimmed

3. Record splitting:
   - blank lines flush plain or FASTA records
   - a line beginning with `>` starts a FASTA record
   - FASTA records continue until the next `>` or blank-line flush
   - a line containing only Phytozome report URLs separated by whitespace is split into separate URL records
   - a line containing multiple inline sequence tokens and/or report URLs can be split into separate records when every token is recognizable
   - otherwise lines are accumulated as a plain record

4. FASTA parsing:
   - header begins with `>`
   - sequence lines are sanitized
   - label may be extracted from a trailing parenthetical label
   - pipe-delimited identifiers can populate protein/transcript/gene fields
   - if sequence content is absent but the header embeds a sequence after an identifier, that inline sequence form may be parsed

5. Report URL recognition:
   - currently normalized only for `phytozome-next.jgi.doe.gov/report/{gene|transcript|protein}/{JBrowse}/{identifier}`
   - scheme is normalized to `https`
   - query string and fragment are removed
   - host is normalized to `phytozome-next.jgi.doe.gov`
   - URL resolution fetches the query sequence through the appropriate source resolver during the real workflow, not during report generation

6. Plain sequence handling:
   - if no structured source is resolved, the raw record can be sanitized and used as direct sequence input
   - sequence kind is controlled by selected BLAST program/request configuration

7. Batch behavior:
   - multiple records become multiple `blastQueryItem` values
   - query resolution runs with controlled parallelism
   - failures may be retried, skipped, or abort the workflow according to user recovery choice
   - records without usable sequence are skipped

Required tables:

- Raw input source table: pasted input vs loaded file, load filename/path if used, raw line count, raw character count.
- Input trace table with one row per parsed record.
- Query resolution table with sequence length, source database, original URL, normalized URL, gene/transcript/protein IDs, label, and outcome.
- Label trace table for BLAST labels: manual, FASTA parenthetical, query-source identifier, auto-identify, skipped, or export filename fallback.

Required diagrams/charts:

- input parser flow diagram
- input type mosaic
- input resolution funnel

Required parser-flow diagram nodes:

`Raw input -> optional load command -> normalize line endings -> split into records -> classify record -> resolve URL/FASTA/source metadata -> sanitize sequence -> assign label -> submit usable query`

If mixed input occurs, add a side note:

`Mixed input was accepted because every token on the line was independently recognized as a report URL or sequence-like token. The program split that line into separate query records before resolution.`

### Chapter 7 Contract: BLAST Execution Method And Request Parameters

Purpose:

- Explain how BLAST was configured and executed.

Required content:

- request parameter table using actual `model.BlastRequest` values
- one row per executed BLAST run with job ID, run label, mode, row count, message/hash, and result status
- server/local explanation
- if local BLAST was used, explain that BLAST+ and downloaded FASTA/index availability were required during actual execution
- if server BLAST was used, explain submit/poll lifecycle when job IDs/results are available

Required request formula:

`BLAST program = f(query sequence kind, target database type)`

Show the mapping table from BLAST request data.

### Chapter 8 Contract: Result Set, Review, And Selection Analytics

Purpose:

- Summarize BLAST rows and final user selection before external-reference/family/filter deep dives.
- Explain how the current export scope was formed: one reviewed run, one Family BLAST group, one selected run from a batch, or all exported runs from `Export all`.
- Preserve zero-hit and unexported query visibility without mixing them into generated-file counts.

Required cards:

- total result rows
- selected/exported rows
- unselected rows
- BLAST runs/groups
- zero-hit runs
- exported runs/groups
- skipped or unresolved query records if already captured
- rows with gene/report URL
- rows with sequence ID
- rows with target length
- rows with query coverage
- rows with E-value
- rows with percent identity
- rows with external reference evidence when enabled

Required charts:

- selected/unselected donut
- per-run row bars
- percent identity histogram or buckets
- query coverage buckets
- E-value buckets
- target length ratio buckets when UniProt length ratio is available
- row traceability completeness stacked bar
- zero-hit/with-hit query status chart for batch or `Export all`

Required row status table:

- run/group label
- raw rows
- selected rows
- unselected rows
- zero-hit status
- top hit preview
- best E-value
- highest identity
- generated files for that run/group

No external-reference judgments should be presented here beyond simple availability counts. Detailed reference interpretation belongs in Chapter 9.

Required provenance subsection:

The BLAST report must not reuse keyword provenance categories without changes. BLAST provenance should be row-field and workflow-layer based:

| Provenance category | Meaning | Column Set |
| --- | --- | --- |
| user query input | values originally supplied or loaded by the user | raw sequence, FASTA header, report URL, manual label |
| query resolver metadata | values produced while resolving input into a query sequence | normalized URL, query gene/transcript/protein ID, source genome |
| BLAST execution result | values returned by server BLAST or local BLAST result parsing | hit number, E-value, identity, bitscore, alignment coordinates |
| target source metadata | values attached from the selected source during BLAST/result parsing | target IDs, JBrowse name, gene report URL, defline |
| UniProt enrichment | values added only when UniProt reference was enabled | accession, reviewed status, canonical length, GO, EC, function |
| InterPro enrichment | values added only when InterPro reference was enabled | conserved-region status, accessions, coverage, match regions |
| family-generated/internal | values created by Family BLAST grouping/merge | family name, member labels, merged target key |
| filter-generated/internal | values created by filter suggestion or clear action | filter flags, recommended keep/remove, hard-rule summaries |
| export-generated/internal | values generated during export/report | row number, file name, hashes, report metadata |
| unavailable/missing | fields blank or not captured in this run | missing URL, missing reference accession, uninstrumented timing |

Required provenance charts:

- provenance donut or stacked bar across all exported table cells
- separate enrichment coverage chart for UniProt/InterPro when enabled
- generated/internal breakdown showing row numbers, family/filter/export values separately when present

Required provenance prose:

`BLAST provenance is layered. A selected row begins as a user query, becomes a BLAST hit through server or local execution, may receive target-source identifiers, may receive UniProt and InterPro enrichment, may be grouped or filtered by internal review tools, and is finally written to files. The report separates these layers so a reviewer can see which values came from biological sources and which values were generated by phytozome GO for traceability.`

#### BLAST-Specific Quality Check Design

Keyword-mode quality checks focus on search-term hit/miss behavior, labels, transcript IDs, gene IDs, descriptions, report URLs, raw export, and sequence export. BLAST-mode quality checks must be redesigned around sequence-query interpretation, alignment evidence, target traceability, reference evidence availability, filter transparency, family grouping, and export integrity.

Render a BLAST quality-check subsection in Chapter 8 for run/result checks, and mirror column-level checks in Chapter 12. Do not create one hidden global score. Each check must show:

- check name
- result: `pass`, `warning`, `not requested`, `not applicable`, or `not available in this run`
- count
- visible rule
- source data used
- explanation for reviewer interpretation
- whether the check is biological evidence, technical traceability, or export integrity

Required BLAST quality-check groups:

| Group | Check | Warning rule | Source data | Interpretation |
| --- | --- | --- | --- | --- |
| Input parsing | Unresolved or skipped query records | warn when any parsed record failed or was skipped | input trace / query resolution | Shows whether the exported BLAST scope differs from pasted input. |
| Input parsing | Mixed input split audit available | warn when mixed input was detected but split trace is unavailable | parser trace | Mixed lines are allowed, but the split must be reviewable. |
| Query sequence | Query sequence length available | warn when exported run lacks known query length | `BlastRequest.Sequence`, rows `QueryLength` | Query coverage and alignment interpretation depend on length. |
| Query sequence | Query source identifiers available | warn when URL/FASTA-derived query lacks gene/transcript/protein/source label | `QuerySequenceSource` | Traceability from exported hits back to input source is weaker. |
| Execution | BLAST job/result identifier available | warn when no job ID, local result marker, hash, or source message is available | `BlastResult` | Supports reproducibility of server/local result retrieval. |
| Execution | Result rows returned | warn when an exported run has zero rows | run rows | Zero-hit runs are valid outcomes but must stay visible. |
| Alignment | Query coverage present | warn when selected rows lack `AlignQueryLengthPercent` and cannot compute it from `AlignLength/QueryLength` | result rows | Coverage is one of the main alignment-review measures. |
| Alignment | E-value present | warn when selected rows lack E-value | result rows | E-value is a core BLAST significance field. |
| Alignment | Identity present | warn when selected rows have zero/missing identity where source should provide it | result rows | Identity helps compare candidate strength. |
| Alignment | Target length present | warn when selected rows lack target length | result rows | Length checks, family merging, and reference comparison depend on it. |
| Target traceability | Stable target identifier available | warn when selected rows lack protein, subject ID, sequence ID, transcript ID, and report URL | result rows | Exported hits need stable handles for follow-up. |
| Target traceability | Source report URL available | warn when selected rows lack report URL and the source normally provides one | result rows/source database | Source inspection may require alternate identifiers. |
| External reference | UniProt coverage | warn when UniProt was requested but many selected rows lack accession | rows with `UniProtReferenceEnabled` | Missing UniProt does not disprove a hit, but limits reference evidence. |
| External reference | Canonical length ratio availability | warn when UniProt was requested but selected rows lack target/canonical ratio | UniProt fields | Filter and length-review evidence may be incomplete. |
| External reference | InterPro conserved-region status availability | warn when InterPro was requested but status is blank for selected rows | InterPro fields | Conserved-region interpretation is unavailable for those rows. |
| Family BLAST | Family detection trace available | warn when Family BLAST was used but detected group trace is missing | family plan/report data | Family grouping must be explainable from labels and settings. |
| Family BLAST | Family merge audit available | warn when rows were merged but chosen/removed best-rankeds are not recorded | merge records | Reviewers need to know which member hit survived and why. |
| Filter | Filter settings captured | warn when filter flags exist but settings are unavailable | filter report state | Recommendations cannot be audited without parameters. |
| Filter | Filter/final selection difference visible | warn when filter flags exist but final selected state is unavailable | filter flags + row selection | The report must distinguish automatic suggestion from user export choice. |
| Export | Selected Excel written when requested | warn when requested output path is missing | generated files/settings | Export integrity check. |
| Export | Raw Excel written when requested | warn when raw export requested but missing | generated files/settings | Raw rows preserve review context. |
| Export | Peptide text sequence completeness | warn when text export requested and hit records were skipped | sequence audit | Sequence export may be partial. |
| Export | Report generated once for action | warn if more than one report path is recorded for one export action | generated files | BLAST `Export all` still gets one report. |

Recommended BLAST quality charts:

- severity bar: count of pass/warning/not requested/not applicable/unavailable
- quality group heatmap: input, execution, alignment, traceability, references, family, filter, export
- selected-row evidence completeness stacked bar: alignment metrics, target IDs, source URLs, UniProt, InterPro
- per-run quality strip for `Export all`: one compact strip per query/family group

Quality prose must explicitly avoid overclaiming:

`These checks evaluate traceability, completeness, and audit readiness of the exported BLAST result set. They do not declare a hit biologically correct or incorrect. A warning means the reviewer should inspect the relevant field or workflow setting before relying on that evidence layer.`

### Chapter 9 Contract: External Reference Analysis

Render this chapter only when UniProt or InterPro was enabled for the run, or rows carry `UniProtReferenceEnabled`/`InterProReferenceEnabled`.

Purpose:

- Explain the external reference workflows in enough detail that a reviewer can understand what extra evidence was added, how it was matched, and how reliable/complete it was.

Opening prose:

`External references are optional evidence layers added after BLAST rows are available. They do not change the fact that BLAST alignment produced the original hit list. They add annotation, canonical-length comparison, protein existence, sequence caution, and conserved-region evidence that can help review whether a hit is plausible. The report describes only reference lookups that already ran during the BLAST workflow.`

#### UniProt Subsection

Render when UniProt was enabled.

Required explanation:

- UniProt client base endpoint: `https://rest.uniprot.org/uniprotkb/search`
- output format: TSV
- requested fields: accession, entry name, reviewed status, protein name, gene names, organism, organism ID, length, function, catalytic activity, GO, GO IDs, EC, keywords, Pfam, InterPro, pathway, subcellular location, protein existence, annotation score, fragment, sequence caution, domains, regions, motifs, active/binding sites, AlphaFoldDB, PDB
- rows are grouped by lookup key to avoid duplicate lookups
- candidate terms include existing UniProt accession, protein, subject ID, sequence ID, transcript ID, extracted accessions from defline and report URL
- source-specific resolver may supply accessions when supported
- candidate queries include accession, id, xref, gene, raw term, and organism-constrained forms; isoform base accessions are also tried
- cache is memory + disk via the UniProt client during normal workflow
- the report does not repeat UniProt API calls

Required UniProt charts:

- accession matched vs not matched
- reviewed/unreviewed/unknown
- canonical length available vs missing
- target length / UniProt canonical length ratio buckets
- fragment/caution flags
- annotation-field coverage

Required UniProt tables:

- lookup strategy summary
- outcome summary
- full-scope statistics
- column lineage for UniProt columns

Important ratio formula:

`target_length / UniProt canonical length (%) = target_length / UniProt entry length * 100`

Formula disclosure:

When captured data are available, show:

`target_length = 360 aa`, `UniProt canonical length = 352 aa`, `ratio = 360 / 352 * 100 = 102.27%`

Use aggregate captured values only; do not choose individual rows for illustration.

#### InterPro Subsection

Render when InterPro was enabled.

Required explanation:

- InterPro client base endpoint: `https://www.ebi.ac.uk/interpro/api/entry/all/protein/uniprot/{accession}/?format=json&page_size=200`
- InterPro requires UniProt accessions for lookup; accessions can come from source resolver or already enriched row data
- query-side InterPro entry is looked up when the query source provides enough protein/accession metadata
- hit rows are grouped by lookup key to avoid duplicate lookups
- API pagination is followed during normal workflow
- cached entries are memory + disk via the InterPro client
- the report does not repeat InterPro API calls

Required InterPro settings table:

Every `model.InterProConservedRegionSettings` field must be shown:

| Field | Meaning |
| --- | --- |
| `UsePfamAccession` | match query and hit evidence through shared Pfam accessions |
| `UseInterProAccession` | match exact InterPro entry accession |
| `UseSignatureAccession` | match member signature accessions |
| `UseEntryType` | restrict or score by entry type, and treat domain/family/homologous_superfamily/repeat/site as conserved candidates |
| `UseEntryName` | allow exact entry-name match as evidence |
| `UseCoverage` | require coverage thresholds for present/partial status |
| `UseMatchRegions` | count region-level overlap as evidence |
| `PresentMinCoverage` | minimum matched coverage percent for `present` |
| `PartialMinCoverage` | minimum matched coverage percent for `partial` |
| `PresentMinMatchedItems` | minimum matched evidence items for `present` |
| `PartialMinMatchedItems` | minimum matched evidence items for `partial` |

Required InterPro decision flow:

1. If hit has no matches, status is blank.
2. If query entry is unavailable or query has no matches, evaluate hit by self evidence:
   - count conserved candidate hit matches
   - compute best coverage
   - `present` if count >= `PresentMinMatchedItems` and coverage rule passes
   - `partial` if count >= `PartialMinMatchedItems` and partial coverage rule passes
   - `missing` if count is zero
   - otherwise `uncertain`
3. If query entry is available:
   - iterate query matches that pass conserved-candidate rules
   - find best hit match for each query match using the evidence score
   - count matched items
   - sum matched coverage length, capped by query match coverage length
   - compute matched coverage percent
   - `present` if matched items and coverage meet present thresholds
   - `partial` if matched items and coverage meet partial thresholds, or if any matched item exists
   - otherwise `missing`

Required formulas:

Evidence score for one query/hit match:

`score = 5*PfamMatch + 4*InterProAccessionMatch + 3*SignatureMatch + 1*EntryTypeMatch + 1*EntryNameMatch + 1*RegionEvidence`

Matched coverage when query evidence is available:

`matched_coverage_percent = sum(min(hit_coverage_length, query_coverage_length) for best matched query evidence) / sum(query_coverage_length for conserved query evidence) * 100`

Self-evidence coverage:

`best_coverage_percent = max(hit match coverage percent among conserved candidate matches)`

Required InterPro charts:

- conserved-region status distribution
- coverage bucket chart
- evidence source contribution chart
- query-entry available vs unavailable
- lookup matched vs missing

Required formula and aggregate evidence:

Use complete captured summary data to show:

- query InterPro/Pfam/signature evidence
- hit evidence
- which settings were active
- evidence score components
- matched item count
- matched coverage calculation
- final status

If row-level query evidence was not captured, state which aggregate fields are unavailable without selecting fallback rows.

### Chapter 10 Contract: Family BLAST Analysis

Render this chapter only when Family BLAST was enabled or the exported item has family group data.

Purpose:

- Explain how multiple query runs were grouped and how duplicate targets were handled.

Required explanation:

- Family BLAST does not combine queries before execution; each query still runs BLAST separately.
- Family BLAST groups review/export units after individual query results exist.
- Group names are derived from labels/source IDs, not by rewriting original `label_name` values.
- Family grouping is independent from external references, but best-hit merge ranking can use UniProt/InterPro evidence when those reference layers were enabled.

Required Family settings table:

Every `model.FamilyBlastSettings` field must be shown:

| Field | Meaning |
| --- | --- |
| `Enabled` | whether Family BLAST grouping was applied |
| `GroupByDetectedPrefix` | whether the program detects common family prefixes |
| `MergeRowsByTarget` | whether rows from member queries are merged by target key |
| `KeepBestHitPerTarget` | whether one best-ranked row is kept when a target appears multiple times |
| `MinimumGroupSize` | minimum number of member queries needed for a family group |
| `StripTrailingQueryIndex` | whether trailing numeric member indexes are stripped |
| `StripAfterNumberSuffix` | whether suffixes after the first member number are stripped |
| `UseUniProtReference` | whether UniProt evidence contributes to best-hit merge scoring |
| `UseInterProReference` | whether InterPro evidence contributes to best-hit merge scoring |

Required detection explanation:

Family label source priority:

`LabelName -> query source alias label -> query source gene/transcript/protein -> auto label from input/header/URL -> RawInput`

Family-name derivation:

1. take first whitespace-delimited field
2. trim separators
3. optionally strip text after number suffix such as `GENE10-like -> GENE10`
4. optionally strip trailing numeric member index such as `GENE10 -> GENE`
5. trim trailing punctuation
6. uppercase final family name
7. keep group only if member count >= `MinimumGroupSize`

Required diagrams/charts:

- family detection flow
- family group map: family name connected to member labels
- pre-merge vs post-merge row count bars
- duplicate-target merge reason distribution

Required merge explanation:

Target key priority:

`Protein -> SubjectID -> SequenceID -> TranscriptID -> GeneReportURL`

Target key normalization:

- lower case
- trim trailing slash
- for URLs, use final path segment
- remove transcript suffix patterns such as `_t1`, `.t1`, `-t1`, `.1`

Representative selection order:

1. higher family reference score
2. lower E-value
3. higher percent identity
4. higher align-query-length percent
5. higher bitscore
6. earlier row if still tied

Family reference score formula:

InterPro contribution, when enabled:

`present +80`, `partial +40`, `uncertain +5`, `missing -80`, plus `InterProCoveragePercent / 10`

UniProt contribution, when enabled:

`UniProtAccession present +20`, `Reviewed +30`, `Fragment -30`, `SequenceCaution -10`, and length-ratio distance from 100%:

- distance <= 10: `+25`
- distance <= 30: `+10`
- distance >= 60: `-20`

Required formula and aggregate evidence:

For at least one merged target when available, show:

- member rows that hit the same target
- reference score components
- E-value/identity/coverage/bitscore tie-break values
- chosen row and reason

### Chapter 11 Contract: BLAST Filter Analysis

Render this chapter only when filter settings, filter suggestion flags, clear-filter state, or filter-vs-final selection data are present.

This is the most detailed BLAST report chapter.

Purpose:

- Fully disclose how the automatic filter made row-selection recommendations.
- Show parameter values, hard rules, soft scoring, ranking/tie-breaks, isoform/top-hit limiting, and the final relationship between filter recommendations and user export selection.

Opening prose:

`The BLAST filter is a row-selection assistant, not a destructive data transform. Applying it rebuilds checkbox recommendations and marks suggested removals, but the user can still change checkboxes before export. This chapter separates three states: the original BLAST result rows, the automatic filter recommendation, and the final user-selected export rows.`

Required filter settings table:

Every `model.BlastFilterSettings` field must be shown with value, default, meaning, and effect:

Hard-rule settings:

- `MinIdentityPercent`
- `MinAlignQueryCoveragePercent`
- `MaxEValue`
- `UseTargetCanonicalLengthRatio`
- `RequireTargetCanonicalLengthRatio`
- `MinTargetCanonicalLengthPercent`
- `MaxTargetCanonicalLengthPercent`
- `RequireUniProtAccession`
- `RejectUniProtFragments`
- `RejectUniProtSequenceCautions`
- `RequireInterProConservedRegion`
- `AllowInterProPartial`
- `RejectInterProMissing`
- `RejectInterProUncertain`
- `MinInterProCoveragePercent`
- `RequireInterProCoverageWhenUsed`
- `RejectIfAnyHardRuleFails`

Selection-limiting settings:

- `KeepBestIsoformPerTargetGene`
- `KeepTopHitsPerQuery`
- `TopHitsPerQuery`

Ranking/tie-break settings:

- `RankingTieBreakerOrder`
- `PreferHigherFilterScoreWhenRanking`
- `PreferLowerEValueWhenTies`
- `PreferHigherIdentityWhenTies`
- `PreferHigherCoverageWhenTies`
- `PreferHigherReferenceScoreWhenTies`
- `PreferHigherBitscoreWhenTies`

Soft-score settings:

- `EnableSoftScore`
- `MinSoftScore`
- `IdentityWeight`
- `CoverageWeight`
- `LengthRatioWeight`
- `InterProWeight`
- `InterProPartialWeight`
- `InterProCoverageWeight`
- `UniProtReviewedWeight`
- `UniProtAnnotationWeight`
- `PenaltySequenceCaution`
- `PenaltyFragment`

Reference-score settings:

- `InterProPresentReferenceScore`
- `InterProPartialReferenceScore`
- `InterProUncertainReferenceScore`
- `InterProMissingReferencePenalty`
- `InterProCoverageReferenceDivisor`
- `UniProtAccessionReferenceScore`
- `UniProtReviewedReferenceScore`
- `UniProtAnnotationReferenceScore`
- `FragmentReferencePenaltyMultiplier`
- `SequenceCautionReferencePenaltyMultiplier`
- `LengthNearDistancePercent`
- `LengthNearReferenceScore`
- `LengthAcceptableDistancePercent`
- `LengthAcceptableReferenceScore`
- `LengthFarDistancePercent`
- `LengthFarReferencePenalty`

Required hard-rule explanation:

For each row:

1. Start with `hardFailed = false` and `score = 0`.
2. If `MinIdentityPercent > 0`, identity below threshold sets `hardFailed`; otherwise identity can add `IdentityWeight`.
3. If `MinAlignQueryCoveragePercent > 0`, coverage below threshold sets `hardFailed`; otherwise coverage can add `CoverageWeight`.
4. If `MaxEValue > 0`, E-value above threshold sets `hardFailed`.
5. If target/canonical length ratio is used, missing ratio can fail when required; out-of-range ratio fails; in-range ratio adds `LengthRatioWeight`.
6. Required UniProt accession can fail missing accession.
7. Reviewed UniProt can add weight; available annotation can add weight.
8. Fragment and sequence caution can fail or penalize according to settings.
9. InterPro status is evaluated as present/partial/missing/uncertain/blank according to the configured required/reject rules.
10. InterPro coverage threshold can fail or add weight.
11. If `RejectIfAnyHardRuleFails` is true and any hard rule failed, the row is recommended for removal.
12. If soft score is enabled and `score < MinSoftScore`, the row is recommended for removal.

Required filter formulas:

Query coverage:

`query_coverage_percent = AlignQueryLengthPercent if present, otherwise AlignLength / QueryLength * 100`

Length ratio:

`target_canonical_length_ratio = TargetLength / UniProtCanonicalLength * 100`

Hard-rule removal:

`remove_by_hard_rules = RejectIfAnyHardRuleFails AND any(hard rule failed)`

Soft-score removal:

`remove_by_soft_score = EnableSoftScore AND score < MinSoftScore`

Final recommendation:

`filter_recommended_remove = remove_by_hard_rules OR remove_by_soft_score OR removed_by_best_isoform_limit OR removed_by_top_hit_limit`

Required ranking/tie-break explanation:

When the filter limits isoforms or top hits, rows are sorted by configured order. The default order is:

`score, identity, coverage, reference, evalue, bitscore`

Each enabled preference determines whether higher or lower values are preferred.

Required charts:

- filter recommendation donut: keep vs remove
- hard-rule failure stacked bars
- soft-score distribution when enabled
- reference-score distribution when reference score is used
- filter vs final selection matrix
- per-query removed/kept bars for batch/Export all

Required tables:

- parameter dictionary
- hard-rule pass/fail summary
- per-query table with total rows, automatic keep/remove, final selected/unselected, rescued, user removed, and agreement
- filter vs final selection table
- user override table listing rescued rows and manually removed rows

Required full-scope filter statistics:

At minimum:

- one row recommended keep
- one row recommended remove
- if user overrode filter, one rescued or manually removed row

The filter statistics must show:

- row number
- query/run label
- target identifier
- identity
- coverage
- E-value
- length ratio
- UniProt fragment/caution/reviewed/accession
- InterPro status/coverage
- score components
- hard-failure reasons
- filter recommendation
- final user selection

If the filter was cleared:

- state that filter marks were cleared and all rows were reselected by the clear action at that time
- show any final user changes if available

### Chapter 12 Contract: Column Dictionary And Data Lineage

Purpose:

- Explain generated BLAST workbook columns using the same registry as the UI/export.
- Explain why the column set changes across database, BLAST program, UniProt, and InterPro settings.
- Connect columns to the chapters that use them for charts, quality checks, Family BLAST, or filter decisions.

Required content:

- selected Excel columns
- raw Excel columns when raw export was requested
- source/database-specific display names
- UniProt columns only when UniProt reference rows were present/enabled
- InterPro columns only when InterPro reference rows were present/enabled
- `target_length / UniProt canonical length (%)` header must name the original source database
- `InterPro conserved region status` owns the old Pfam-domain position

For each column:

- column display header
- technical ID
- meaning
- source
- collection method
- blank meaning
- whether used in report statistics, filter, family merge, or external-reference analysis

Required BLAST column groups:

| Group | Columns | Source/meaning |
| --- | --- | --- |
| audit/export | `row`, selection/filter-related presentation state when captured | generated by export/report for traceability |
| source/run | `source_database`, `blast_program`, `label_name` | workflow annotation and user/query label state |
| hit identity | `protein`, `subject_id`, `species`, `jbrowse_name`, `target_id`, `sequence_id`, `transcript_id`, `defline`, `gene_report_url` | target-side BLAST/source identifiers |
| alignment ranking | `hit_number`, `hsp_number`, `e_value`, `bitscore`, `percent_identity` | BLAST result evidence |
| alignment geometry | `align_len`, `query_length`, `align_query_length_percent`, `query_from`, `query_to`, `target_from`, `target_to`, `strands`, `mismatches`, `gap_openings`, `identical`, `positives`, `gaps` | alignment span/quality interpretation |
| target length | `target_length` | source/result target sequence length |
| UniProt reference | all `uniprot_*`, `target_uniprot_canonical_length_percent`, `uniprot_canonical_length` | optional UniProt enrichment |
| InterPro reference | `interpro_conserved_region_status`, `interpro_*` | optional InterPro enrichment |

Column dictionary must include these BLAST-specific blank meanings:

- UniProt columns blank because UniProt was not requested.
- UniProt columns blank because lookup was requested but no entry was matched.
- InterPro columns blank because InterPro was not requested.
- InterPro columns blank because no UniProt accession was available for InterPro lookup.
- InterPro conserved-region status blank because hit had no InterPro matches or query/hit evidence was not captured.
- alignment percentage blank because query length was unavailable and could not be computed.
- gene report URL blank because the target source/result did not provide a stable report page.

Required visual presentation:

- a compact column-group map before the full dictionary
- a column completeness chart grouped by BLAST column group
- a reference-column availability note when UniProt/InterPro were disabled

Implementation rule:

The report must use `prompt.BlastExportColumnIDs` and `prompt.ColumnExportHeader` for column order and display names. It must not invent an order that differs from Excel export.

### Chapter 13 Contract: Export Settings And Generation Log

Purpose:

- Record what files the user requested and how the export action proceeded.
- Explain how BLAST export settings affected file naming, row scope, TXT headers, row-number preservation, and filter-color mirroring.

Required settings:

- output directory
- file base name or folder name
- selected Excel on/off
- raw Excel on/off
- peptide text on/off
- report PDF on/off
- single export vs `Export all`
- query/family display names
- TXT header label behavior
- selected row count and all-row count for each exported run/group
- rowNumbers availability
- filterFlags availability
- export metadata gene name/gene ID/report URL when query source provided it

Required generation steps:

- write selected BLAST Excel
- write raw BLAST Excel
- fetch/use peptide sequences for text export
- prepend query sequence records when available
- write peptide text
- capture file metadata and hashes
- render report PDF

For `Export all`, steps may be summarized per exported run plus aggregate timing. If per-run timing is not instrumented, state that per-run timing is not available.

BLAST-specific export branches:

| Branch | Required report behavior |
| --- | --- |
| selected Excel only | explain that no peptide sequence fetching was requested unless TXT was also enabled |
| selected Excel + TXT | explain that Excel writing and sequence fetching may run together in the workflow |
| raw Excel enabled | explain raw rows preserve current review context, not only exported rows |
| TXT enabled | explain hit sequences are fetched for selected rows and query sequences are prepended when available |
| Family BLAST TXT | explain all available family member query sequences are prepended |
| filter flags present | explain Excel cell colors/row states mirror final filter suggestion flags where supported |
| `Export all` | explain each exported run/group may produce its own files, while the report PDF is single |
| empty default file name | explain fallback naming behavior and TXT header fallback if label was missing |

Required generation log table columns:

- step name
- run/group label, or `all` for aggregate steps
- file path if a file was written
- start time
- end time
- duration
- status
- detail message
- row/sequence/file count

Required prose:

`The export log describes file generation, not the whole interactive session. BLAST searching, reference enrichment, filtering, and review may have occurred earlier. If those phases were not instrumented as generation steps, they are summarized in earlier chapters and marked unavailable in this log rather than guessed.`

### Chapter 14 Contract: Sequence Export Audit

Render full chapter when `WriteText` was true. If text export was not requested, render a short not-requested section.

Purpose:

- Explain peptide records written to TXT, including hit records and prepended query records.
- Separate exported hit sequences from query sequences used as references at the top of the TXT.

Required content:

- text file format
- selected hit count
- written hit sequence records
- skipped/unavailable hit sequence records
- prepended query sequence records
- query source labels used in headers
- family mode: all available query sequence headers for the family, not only first member
- sequence length summaries

Important rule:

The report must not fetch sequences to fill this chapter. It may only describe records already fetched/written for the actual text export.

Required sequence categories:

| Category | Meaning | Report behavior |
| --- | --- | --- |
| query sequence record | sequence originally used as BLAST query and prepended to TXT | list before hit records; mark source as query input/query resolver |
| family member query sequence record | query sequence from a Family BLAST member | list each member when available; show member label |
| selected hit peptide record | peptide sequence fetched for a selected BLAST hit | list row number, target ID, sequence ID, status, length |
| skipped hit peptide record | selected row whose sequence could not be fetched/written during actual export | include status and reason if captured |

Required charts:

- query records vs hit records count
- written vs skipped hit records
- sequence length distribution for written hit records
- family member query sequence completeness when Family BLAST was used

Required table columns:

- record order
- record category
- run/group label
- row number when applicable
- header label
- sequence ID
- transcript ID
- target/source identifier
- status
- length
- source

TXT header explanation:

- If `LabelName` was missing and export filename became the TXT header label fallback, state this explicitly.
- If family member query labels override the group TXT header label, show each member label and source.

### Chapter 15 Contract: File Technical Details Appendix

Purpose:

- Preserve full technical metadata and hashes for every generated artifact.
- Connect each file back to the BLAST query/run/family group it represents.
- Avoid crowding earlier chapters with long hashes, paths, and technical identifiers.

For each file:

- file name
- full path
- type
- role
- associated query/run/family label
- associated BLAST program and execution mode when applicable
- associated row scope: selected rows, all rows, peptide records, report
- related generated file group for `Export all`
- size
- created/modified/accessed time when available
- permissions
- owner when available
- SHA-256
- SHA-1
- MD5
- hash captured at

The report PDF self-hash limitation must be explained.

Appendix grouping:

- For single export, show selected Excel/raw Excel/TXT/report in one artifact group.
- For `Export all`, show one artifact group per exported query/family plus the single report artifact group.
- For Family BLAST, show family name first and member query labels underneath.

Hash prose:

`The hash identifies the exact byte content of this generated file at the time the report inspected it. If the file is edited after report generation, the hash will no longer match. The report PDF does not include its own final hash because writing that hash into the PDF would change the PDF bytes.`

### Chapter 16 Contract: Report Limitations And Data Availability Notes

Purpose:

- Make audit boundaries explicit.
- Explain BLAST-specific interpretation risks in a way that is useful to a scientific reviewer.

Required notes:

- report did not perform extra BLAST, UniProt, InterPro, sequence-fetch, cache-refresh, download, or OS-probe work
- missing fields are unavailable because they were not captured, not because they were proven absent biologically
- BLAST hits require scientific interpretation beyond alignment statistics
- external references can be incomplete or absent even for real homologs
- filter recommendations are generic and parameter-driven; they do not replace expert review
- Family BLAST grouping is label/prefix based and should be checked by the user when names are ambiguous

Required BLAST-specific limitation categories:

| Category | Required note |
| --- | --- |
| Input interpretation | The parser records how input was split and resolved, but ambiguous labels or pasted sequences may still require user review. |
| BLAST alignment | BLAST similarity supports candidate discovery; it does not prove orthology, function, pathway role, or copy-number completeness. |
| Server/local execution | Server and local BLAST paths may differ in database contents, availability, and parsing detail; the report documents the path used in this run. |
| External references | Missing UniProt or InterPro data can mean no match was found, accession was unavailable, API/cache data were incomplete, or the feature was disabled; it is not proof of absent protein function. |
| InterPro conserved-region status | Status depends on configured evidence switches and thresholds; present/partial/missing/uncertain are review aids, not final biological labels. |
| Family BLAST | Family grouping is derived from labels/source IDs and parameterized string handling; ambiguous family names require human inspection. |
| Filter | Filter defaults are conservative generic heuristics. They are intended to suggest rows for review/export selection, not to certify homologs. |
| Export scope | The report covers only the current generated file set and selected/exported rows unless raw Excel or all-row summaries are explicitly included. |
| Instrumentation | Some useful details may be unavailable if the current workflow did not capture them before report generation. |

Required final statement:

`This report is an audit explanation of what phytozome GO did during this BLAST export action. It is not a biological conclusion document. Final interpretation should combine the exported rows, source database pages, sequence/domain evidence, experimental context, and expert review.`

## Implementation Data Model Hints

Add a `BlastReportData` field to `report.ReportData`.

Suggested structs:

```go
type BlastReportData struct {
    Database           string
    Species            SpeciesReport
    Execution          BlastExecutionReport
    Inputs             []BlastInputTrace
    Queries            []BlastQueryReport
    Runs               []BlastRunReport
    Selection          BlastSelectionStats
    ExternalReferences ExternalReferenceReport
    Family             *FamilyBlastReport
    Filter             *BlastFilterReport
    Provenance         []ProvenanceSlice
    ColumnCompleteness []ColumnCompleteness
    QualityChecks      []QualityCheck
    Columns            []ColumnLineage
    ExportSettings     []NameValue
    GenerationSteps    []GenerationStep
    Sequences          SequenceAudit
}
```

Suggested supporting structs:

```go
type BlastInputTrace struct {
    Order          int
    RawPreview     string
    InputType      string
    ParserPath     string
    Source         string
    SequenceLength int
    LabelName      string
    GeneID         string
    TranscriptID   string
    ProteinID      string
    OriginalURL    string
    NormalizedURL  string
    Outcome        string
    Notes          string
}

type BlastRunReport struct {
    RunIndex       int
    Label          string
    FamilyName     string
    Program        string
    ExecutionMode  string
    JobID          string
    RowCount       int
    SelectedRows   int
    Message        string
    ResultHash     string
    ZUID           string
}

type ExternalReferenceReport struct {
    UniProtEnabled  bool
    InterProEnabled bool
    UniProt         UniProtReferenceReport
    InterPro        InterProReferenceReport
}

type FamilyBlastReport struct {
    Settings     model.FamilyBlastSettings
    Groups       []FamilyBlastGroupReport
    MergeRecords []FamilyMergeRecord
}

type BlastFilterReport struct {
    Settings              model.BlastFilterSettings
    Applied               bool
    Cleared               bool
    RecommendedKeep       int
    RecommendedRemove     int
    FinalSelected         int
    FinalUnselected       int
    UserRescued           int
    UserRemovedAfterKeep  int
    HardRuleSummaries     []BlastFilterRuleSummary
    Totals                BlastFilterTotals
}
```

Keep the real implementation idiomatic and compact; the exact names may differ. The important rule is that report data must be structured and captured from normal workflow state before rendering.

## Renderer Acceptance Criteria

### Content Completeness

- Report can be generated for single BLAST export.
- Report can be generated for `Export all` as one PDF.
- Report can be generated for Family BLAST grouped export.
- Report can be generated when no external references were used.
- Report can be generated when UniProt only, InterPro only, or both were used.
- Report can be generated when filter was never applied, applied, cleared, or overridden by the user.
- Input chapter explains FASTA, URL, plain sequence, inline mixed tokens, loaded file, skipped/unresolved records when present.
- InterPro chapter includes all conserved-region settings, formulas, and aggregate status distributions when InterPro was used.
- Family chapter includes all settings, detected groups, and merge explanation when Family BLAST was used.
- Filter chapter includes every filter setting, formulas, charts, and user-vs-filter comparison when filter state is present.
- Generated file index and appendix include all files produced by the current export action.

### Visual Quality

- Text wraps cleanly in all tables.
- Long URLs, hashes, FASTA headers, and annotations do not overflow page margins.
- Conditional chapters do not leave blank pages or empty tables.
- Charts have legends, labels, and explanatory prose.
- Dense parameter chapters are readable through grouping, subheadings, and consistent table formatting.

### Audit Safety

- Renderer and report builder do not call external systems.
- Report-only code does not run BLAST, sequence fetching, UniProt, InterPro, source searches, downloads, or cache refreshes.
- Every missing field has an honest explanation.
- File hashes reflect generated files at inspection time.
- PDF self-hash is omitted or explained.

### Professional Tone

- The report explains method and evidence without overstating conclusions.
- It is suitable for a teacher, lab reviewer, or collaborator to understand what the program did.
- It presents filter/family/reference logic as transparent parameterized computation.

## Future Enhancements

These are not required for the first BLAST report implementation:

- external manifest containing final report PDF hash
- embedded raw BLAST XML appendix excerpts
- optional CSV/JSON companion report data export
- richer per-row filter reason codes captured directly during filter evaluation
- full cache provenance table distinguishing memory cache, disk cache, API hit, and source resolver hit
- optional interactive HTML report sharing the same structured data model
- pathway-aware interpretation layer after pathway search is implemented

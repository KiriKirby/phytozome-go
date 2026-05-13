# Keyword Data Analysis Report Specification

This document defines the PDF `Data Analysis Report` for keyword-mode exports in `phytozome GO`.
It is an implementation specification, not a marketing description. The report must be generated from a structured report data model and then rendered to PDF.

## Non-Negotiable Rules

1. The report is passive and observational.
   It must only describe data that the current keyword query/export workflow already collected for normal operation.

2. The report must never trigger extra data acquisition.
   Do not query Phytozome, lemna.org, UniProt, InterPro, BLAST services, local BLAST databases, release download indexes, cache refreshers, sequence fetchers, or any other database/API/download path solely to make the report more complete.

3. Missing information is reported honestly.
   If a desired field is not present in the current workflow state, the report must say `Not available in this run` and explain that no extra lookup was performed for report generation.

4. One export action produces one report.
   A normal keyword export that writes both Excel and peptide text still gets one report. A report describes the current generated file set, not every previous screen, search, or navigation action in the session.

5. Report file names use the export file or export folder base name.
   Single-file exports write `<export-file-base>_rpt.pdf`. Folder-level exports write `<output-folder-name>_rpt.pdf`. The fallback timestamp form is reserved only for states where no usable base name or folder name exists.

6. The report is an audit artifact.
   It must be readable by a reviewer who did not watch the program run. Use serious, professional explanatory prose, traceability tables, measured timings, file hashes, and section-level interpretation. Charts and tables support the writing; they must never replace the writing.

## User Requirement Coverage

This blueprint covers all currently stated requirements for keyword-mode reports.

| Requirement | Applied design |
| --- | --- |
| Report only uses information already known by the program | Enforced in non-negotiable rules, chapter contracts, and audit safety criteria. Missing values are written as unavailable instead of fetched. |
| Report file name is file/folder base + `_rpt` | File naming rule uses the export file base or export folder name, for example `PAL_rpt.pdf`. |
| Common opening content for all modes | Chapter 1 and Chapter 3 contain software, user/session, time, system, runtime location, executable, terminal, and environment context. |
| Software name, author, repository, version | Chapter 3 uses `phytozome GO`, `wangsychn`, repository URL, and `version`/build metadata when embedded. |
| User name/session and detailed system info | Chapter 3 records OS/user/env information, host, process ID, working directory, executable metadata, OS/architecture/CPU/memory/terminal details where available. |
| Usage time from query to generated file completion | Chapter 4 records query/search start through export completion with detailed local and UTC timestamps. |
| Keyword mode begins with generated file list | Chapter 2 is the generated-file index with name, type, size, path, role explanation, and jump target to technical details. |
| Hashes are too long and belong at the end | Chapter 2 links to Chapter 13; Chapter 13 contains full hashes and file metadata. |
| Database, species, search terms, matching mode, matching flow, label flow | Chapters 5, 6, and 7 document database/species, search-term types, matching method, method log, and label traceability. |
| Table statistics and selected/unselected statistics | Chapter 8 contains statistic cards, selection donut chart, per-term selection chart, and row status table. |
| Detailed column source/API information | Chapter 10 is the column dictionary and lineage chapter, with source, collection method, blank meaning, and report-stat usage for every column. |
| Export options and generation process with log-level detail | Chapter 11 records export settings and chronological generation log. |
| Final file details with attributes and hashes | Chapter 13 records full file metadata, size, times, permissions, owner where available, and hashes. |
| Smart professional layout and chart choices | Layout, typography, chart, table, and chapter composition rules are fully applied in this file. |
| Detailed explanatory writing throughout the PDF | Narrative style and chapter contracts require serious professional prose in every chapter. |
| Selection/all-items relationship chart | Chapter 8 uses a selected vs unselected donut chart and per-term stacked bars. |
| Performance and flow timing charts | Chapter 4 and Chapter 11 use measured workflow and export-generation duration charts. |
| Quality checks and direct/fallback proportions | Chapter 9 uses provenance donut chart, quality summary bar, and detailed quality-check table. |
| Dedicated report design file referenced by AGENT | This file is the keyword report source of truth; AGENT only points here. |

## Current Program Data Inventory

The report is designed around the data the current program actually has. The following fields and behaviors were identified from the current Go code and are first-class report inputs when available.

### Software And Build Data

Current constants and runtime values:

| Data | Current source |
| --- | --- |
| Software display name | `cmd/phytozome-go/main.go` constant `displayName = "phytozome GO"` |
| Author | `cmd/phytozome-go/main.go` constant `author = "wangsychn"` |
| Repository URL | `cmd/phytozome-go/main.go` constant `repoURL = "https://github.com/KiriKirby/phytozome-go"` |
| Version | `cmd/phytozome-go/main.go` variable `version`, currently defaulting to `dev` unless replaced at build time |
| Go runtime version | `runtime.Version()` |
| Executable path/name | `os.Executable()` |
| Working directory | `os.Getwd()` |
| Application directory | `internal/appfs.ApplicationDir()` |
| Output directory | `internal/appfs.OutputDir()` |
| Cache directory | `internal/appfs.CacheDir()` |

### Species Data

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

No additional species fields are fetched for the report. If a field such as taxonomy ID, genome version, release URL, or capability detail is not already present in workflow state or result rows, write it as unavailable.

### Keyword Group And Row Data

Current `model.KeywordSearchGroup` fields:

| Field | Meaning in report |
| --- | --- |
| `SearchTerm` | original keyword/search term group |
| `LabelName` | group label assigned or inferred during workflow |
| `LabelMethod` | label path captured by the workflow, such as manual labels or auto-identify labels |
| `SearchStartedAt` | per-term keyword search start timestamp captured during normal search execution |
| `SearchEndedAt` | per-term keyword search end timestamp captured during normal search execution |
| `SearchDurationMS` | measured per-term keyword search duration |
| `Rows` | result rows for that search term |

Current `model.KeywordResultRow` fields:

| Field | Meaning in report |
| --- | --- |
| `SourceDatabase` | `phytozome`, `lemna`, or source name set by workflow |
| `SearchTerm` | original term that produced this row |
| `LabelName` | user-facing label used for readability/export naming |
| `ProteinID` | source protein identifier when present |
| `TranscriptID` | source transcript identifier |
| `GeneIdentifier` | source gene identifier or combined source/internal gene ID |
| `Genome` | genome/species/release text stored in row |
| `Location` | genomic coordinate/location text when known |
| `Aliases` | alias/symbol list |
| `UniProt` | source-provided UniProt cross-reference text when present |
| `Description` | source annotation/description |
| `Comments` | source comments/notes |
| `AutoDefine` | source or parsed automatic definition/name |
| `GeneReportURL` | source report/release URL known to workflow |
| `SequenceHeaderLabel` | label used in sequence export headers |
| `SequenceID` | identifier used for peptide sequence fetching/export |
| `ExtraColumns` | dynamic source-specific columns, especially lemna GFF3/AHRD fields |

### Keyword Search Behavior

Current Phytozome keyword behavior:

- If the keyword is a Phytozome report URL, the workflow parses report type and identifier and searches variants through gene/transcript/protein lookup paths.
- If the keyword looks like a specific gene/transcript/protein identifier, the workflow tries specific identifier lookups.
- Otherwise it performs keyword search through the Phytozome keyword search path.
- Keyword rows are cached in memory and persistent JSON cache during normal workflow.
- Phytozome keyword rows include aliases, UniProt text, descriptions, comments, automatic definition, report URL, sequence header label, and sequence ID when available from returned gene/transcript records.

Current lemna keyword behavior:

- The workflow resolves a lemna release and searches release-backed GFF3 rows.
- GFF3 rows populate dynamic `ExtraColumns` such as `gff_seqid`, `gff_source`, `gff_type`, `gff_start`, `gff_end`, `gff_score`, `gff_strand`, `gff_phase`, `gff_attributes`, `lemna_release`, `lemna_gff_url`, and `attr_*` attributes.
- AHRD records are loaded during normal workflow when available and add dynamic fields such as `ahrd_protein_accession`, `ahrd_blast_hit_accession`, `ahrd_quality_code`, `ahrd_human_readable_description`, `ahrd_interpro`, and `ahrd_gene_ontology_term`.
- lemna rows use release URL as `GeneReportURL` because AHRD records do not provide stable gene report URLs.

The report describes these mechanisms only as mechanisms that already ran. It must not call them.

### Export Settings And Output Behavior

Current keyword export settings and file behavior:

| Setting/behavior | Report interpretation |
| --- | --- |
| `BaseName` | base name used for `.xlsx`, `_raw.xlsx`, and `.txt` outputs |
| `OutputDir` | resolved output directory |
| `WriteExcel` | selected rows written to `<base>.xlsx` |
| `WriteRawExcel` | all current rows written to `<base>_raw.xlsx` |
| `WriteText` | peptide records written to `<base>.txt` |
| `WriteReport` | report PDF requested |
| selected Excel | produced by `export.WriteKeywordResultsExcel` using selected rows |
| raw Excel | produced by `export.WriteKeywordResultsExcel` using all current rows |
| peptide text | produced by fetching/using keyword protein sequence records and `export.WriteProteinSequencesText` |

Current keyword Excel columns:

- `row`
- `search_term`
- `label_name`
- `protein_id` only when at least one exported row has `ProteinID`
- `transcript`
- `gene_identifier`
- `genome`
- `location`
- `alias`
- `uniprot`
- `description`
- `comments`
- `auto_define`
- `gene_report_url`
- sorted dynamic `ExtraColumns`

The report must use this exact current behavior when describing selected Excel and raw Excel contents.

## Implemented Workflow Integration

The keyword PDF report is generated by the real keyword export subflow for both `Phytozome` and `lemna.org` when the user enables `WriteReport`.

Applied integration behavior:

- The renderer entry is `internal/report.RenderKeywordPDF`.
- The workflow builder is `internal/workflow/keyword_report.go`.
- The report data model is populated after the user starts file generation and before the export-complete page is shown.
- The report path uses the export-completion local timestamp and the file name shape `YYYYMMDD_HHMMSS_report.pdf`.
- The report is generated once per explicit export action, even when selected Excel, raw Excel, and peptide text are all generated.
- The report renderer receives only structured workflow/export state and local generated-file metadata.
- The report renderer does not call `source.DataSource`, Phytozome clients, lemna clients, BLAST clients, sequence fetchers, cache refreshers, download helpers, UniProt, or InterPro.
- Selected Excel writing no longer triggers peptide sequence fetching by itself. Peptide sequence fetching occurs only when peptide text export is requested.

Implemented source-specific handling:

| Source | Applied report behavior |
| --- | --- |
| `Phytozome` | Species context uses proteome ID, JBrowse name, genome label, common name, release date, and search alias already selected by the user. Provenance counts classify present row values as direct source result values. Keyword input type is classified from the term text as report URL, transcript-like ID, identifier-like term, or plain keyword. |
| `lemna.org` | Species context uses release/species fields already selected by the user, including official-clone marker where present. Dynamic `gff_*`, `attr_*`, `ahrd_*`, and `lemna_*` columns are described as release-backed parsed data. Provenance counts classify present row values as local release parsed values. |

Implemented generated-file handling:

- Selected Excel, raw Excel, and peptide text files are inspected after they are written.
- File size, modified time, permissions, SHA-256, SHA-1, and MD5 are recorded from local generated artifacts.
- The report PDF itself is listed as the report artifact for the current export action.
- The report PDF does not embed its own final hash because writing that hash into the PDF would change the PDF bytes. An external manifest is the correct future mechanism if final report self-hashing becomes required.

Implemented quality handling:

- Quality checks are based on the keyword table rows and columns that the export writes.
- Report metadata, report file properties, system fields, missing OS/memory instrumentation, and the report PDF itself must never affect data-quality results.
- The report includes a generated-table cell-completeness chart with `cells with data` versus `empty cells`.
- The report includes a search-term hit chart with `terms with data` versus `zero-hit terms`.
- Column-level completeness is shown as filled rows, empty rows, filled percentage, and column source.
- Repeated explanatory text is consolidated into section prose and notes; tables should not repeat identical `Explanation` cells row after row.

Implemented timing behavior:

- Query start is captured after keyword input has been accepted.
- Per-term search start/end/duration are captured inside the normal parallel keyword search workers.
- Review start is captured when the keyword result table selection loop begins.
- Export start is captured when file generation begins.
- File writing, sequence handling, metadata/hash capture, and report rendering are recorded as generation steps.
- Uninstrumented values remain explicit `not available in this run` values rather than being inferred.

Implemented label trace handling:

- Manual label mode records `user input` as the label source.
- Auto-identify mode records the first source that produced the final label when available: row `label_name`, first alias, gene/transcript/sequence identifier, or the final auto-identify result.
- In lemna mode, Phytozome fallback labels are reported only when the fallback already ran during the actual label step. The report never runs fallback searches while rendering.
- The label chapter uses one method note for the shared precedence rules instead of repeating the same explanation in every table row.

## Visual And Layout Rules

- Page size: A4 or Letter is acceptable, but choose one globally and keep it consistent.
- Margins: use professional document margins, with room for page numbers and section headers.
- Typography:
  - title: large, restrained, non-decorative
  - section headings: clear hierarchy, no excessive styling
  - body text: readable paragraph width
  - code/path/hash text: monospace
- Use a running footer with report title, generation timestamp, and page number.
- Use internal PDF bookmarks for major sections when the renderer supports them.
- In tables, keep long paths and hashes wrapped or moved to the technical appendix.
- Use charts only when they communicate a concrete audit relationship. Avoid decorative charts.
- Every chart must have:
  - a title
  - values and percentages visible in labels or an adjacent table
  - a one-paragraph interpretation below it
  - a note explaining which already-known report data produced it

## Document Design System

The report uses a restrained scientific audit style, not a colorful dashboard and not a raw log dump.
The visual language is clean, quiet, and evidence-focused.

### Layout Grid

- Use a single-column narrative grid for text-heavy pages.
- Use a two-column grid only for compact summary blocks, paired charts, or side-by-side small tables.
- Do not use more than two columns in the main body; wide scientific tables need horizontal space.
- Keep each major section starting near the top of a page unless the previous section has enough space for the heading and at least one complete content block.
- Avoid orphan headings at the bottom of a page.
- Keep chart + interpretation + source note together on the same page whenever possible.

Applied layout zones:

| Zone | Purpose | Notes |
| --- | --- | --- |
| Header | section title or report title | subtle, small, repeated |
| Body | prose, tables, charts | main content |
| Footer | page number, generation timestamp | repeated |
| Appendix marker | technical appendix chapters | small label shown on appendix pages |

### Color Palette

Use a restrained palette that prints well in grayscale.

Applied semantic colors:

| Semantic use | Color intent | Notes |
| --- | --- | --- |
| Primary text | near black | main prose and table text |
| Secondary text | dark gray | explanatory notes and source lines |
| Primary accent | deep blue or teal | headings, selected chart slice |
| Neutral accent | medium gray | unselected/user-time/background categories |
| Success/pass | green | quality checks that passed |
| Warning | amber | missing non-required data, incomplete instrumentation |
| Error/fail | red | failed export step or required data missing |
| Internal/generated | muted purple or slate | generated metadata/provenance category |
| Missing/unavailable | light gray | missing values in charts |

Rules:

- Never rely on color alone. Status must also be written as text: `pass`, `warn`, `not requested`, `not available`, or `failed`.
- Use the same color for the same concept across all charts.
- Do not use saturated colors for large backgrounds.
- Pie/donut charts use no more than six slices. If more categories exist, combine low-count categories into `other` and explain it in the adjacent table.

### Typography

Applied typography hierarchy:

| Element | Style intent |
| --- | --- |
| Report title | largest, bold, no decoration |
| Section heading | bold, medium-large |
| Subsection heading | bold, body-plus size |
| Body text | regular, readable line height |
| Table header | bold, small caps not required |
| Source note | smaller, gray, starts with `Data source:` |
| Limitation note | smaller, gray or amber label |
| Paths/hashes/IDs | monospace |

Keep body prose concise but explanatory. A report chapter reads like a formal scientific/commercial audit narrative, not like a console log pasted into PDF.

### Font Implementation

The renderer uses the operating system's available sans/CJK fonts for report text.

Applied implementation rules:

- Body text, headings, tables, chart labels, footers, paths, hashes, and notes use a resolved system sans/CJK font family.
- The renderer does not require bundled report fonts, font downloads, project-local font folders, or a report-font environment variable.
- Runtime discovery maps to platform sans/CJK fonts in quality-first order: Windows starts with Microsoft YaHei, then DengXian, Malgun Gothic, Meiryo, and only then older fallback fonts such as SimHei; macOS starts with PingFang and Hiragino Sans GB before broader fallbacks such as Arial Unicode; Linux starts with Noto CJK or Source Han Sans before WenQuanYi fallback fonts.
- The renderer must not silently fall back to Helvetica or another ASCII-only PDF core font for multilingual report text.
- If no usable system sans/CJK font can be found, report generation fails with an explicit font error instead of producing a PDF with missing-glyph question marks.

### Applied Report Elements

The following elements are used throughout the report. They define how facts, sources, missing values, checks, method steps, paths, and hashes are rendered.

#### Summary Card

Purpose:

Show one important numeric or textual fact.

Layout:

- small label
- large value
- subtitle/source line when that value exists

Rendered value examples:

- `Selected rows: 14`
- `Search terms: 5`
- `Generated files: 3`
- `Active processing time: 18.4 s`

#### Source Note

Purpose:

Explain where the section's data came from.

Text shape:

`Data source: This section uses keyword result rows already loaded in the workflow and row-selection state captured when the user confirmed export. No additional database lookup was performed for this section.`

Every major section must include either a section-level source note or source columns inside tables.

#### Availability Badge

Purpose:

Represent availability without hiding missing data.

Allowed labels:

- `available`
- `not available in this run`
- `not requested`
- `not applicable`
- `not instrumented`
- `failed`
- `warning`

Do not use empty cells when an explanation is possible. Empty biological/source fields can remain blank in exported data tables, but report tables explain why the value is blank when the reason is known.

#### Quality Check Row

Purpose:

Show transparent checks without inventing a black-box score.

Required fields:

- check name
- result badge
- count/value
- threshold or rule
- explanation
- data source

#### Method Step

Purpose:

Describe workflow in a human-readable audit trail.

Layout:

- step number
- short title
- one paragraph of explanation
- timing/status line when the timestamp or status exists

Example:

`Step 3 - Keyword matching. The workflow searched the selected source using the parsed search terms in their original order. Result rows were tagged with search_tern so exported rows can be traced back to the exact input term.`

#### Long Technical Value

Purpose:

Handle paths, URLs, IDs, and hashes.

Rules:

- Use monospace.
- Wrap long values at safe boundaries where possible.
- For hashes, show a short prefix in summary sections and the full value only in the technical appendix.
- Never truncate a value without indicating truncation.

## Chart Blueprint

Charts are part of the report's explanation layer. They must not create facts that are not present in `ReportData`.

### Chart Rendering Rules

- Every chart must be reproducible from tables in the same section.
- If the chart cannot be produced because instrumentation is missing, render a small `not instrumented` note instead of an empty chart.
- Use exact counts in adjacent tables; percentages are rounded consistently to one decimal place.
- If a denominator is zero, omit the chart and explain that there were no rows/files/events to chart.
- Sort bars by workflow order when order matters, such as search terms. Sort by value only when the chart is explicitly comparative and order is not meaningful.

### Required Keyword Charts

#### 1. Selection Donut Chart

Question answered:

`How much of the current result table did the user export?`

Data:

- selected rows
- unselected rows

Formula:

- `selected_percent = selected_rows / total_rows * 100`
- `unselected_percent = unselected_rows / total_rows * 100`

Design:

- donut chart, not full pie chart, because the center shows total rows
- selected slice uses primary accent
- unselected slice uses neutral gray
- adjacent table gives counts and percentages

Interpretation rules:

- High selected percentage: phrase as "most result rows were included"
- Low selected percentage: phrase as "the user applied stronger manual selection"
- Do not describe biological quality from selection percentage alone

#### 2. Per-Term Selection Bar Chart

Question answered:

`Which search terms contributed selected rows, and where did the user filter more strongly?`

Data:

- for each search term: selected row count, unselected row count

Design:

- horizontal stacked bar chart
- y-axis: search terms in original input order
- x-axis: row count
- selected segment first, unselected segment second
- include terms with zero hits as zero-length rows in the adjacent table, not necessarily as visible bars

Render condition:

- more than one search term exists

Omission condition:

- only one search term exists; the selection donut is enough

#### 3. Workflow Duration Timeline

Question answered:

`Where did time go from query to export completion?`

Data:

- measured phase start/end timestamps
- phase durations

Design:

- horizontal stacked duration bar
- active software phases use blue/teal shades
- user decision/review phases use gray
- report/file verification phases use slate/purple
- unmeasured phases are not drawn

Minimum phases to support when instrumented:

- query input/search
- keyword search
- label handling
- row review
- export setup
- selected Excel generation
- raw Excel generation
- peptide sequence handling
- peptide text generation
- file metadata capture
- hash calculation
- report rendering

Interpretation rules:

- Distinguish active processing time from user decision time.
- Do not blame the database or filesystem unless timing categories actually support that conclusion.
- Use neutral wording: "The longest measured phase was..." rather than "the bottleneck was..." unless the data are clear.

#### 4. Export Generation Duration Chart

Question answered:

`Which file-generation step took the most time?`

Data:

- export/file step timings only

Design:

- stacked bar or simple sorted horizontal bars
- include only requested file types and actually measured report steps

Use:

- always when at least two export generation steps have timing

Omit:

- when only one generation step is measured

#### 5. Data Provenance Donut Chart

Question answered:

`How direct or fallback-dependent was the data used in the exported table?`

Preferred granularity:

1. row-level provenance if the workflow records it
2. field-level provenance by exported column/value if row-level provenance is not available
3. section-level note if neither is instrumented

Categories:

- direct source result
- local release parsed
- cache-backed
- fallback already performed
- generated/internal
- missing/unavailable
- other

Design:

- donut chart with adjacent category table
- direct source/local parsed categories use strong but distinct colors
- fallback uses amber
- missing uses light gray
- generated/internal uses slate/purple

Interpretation rules:

- Fallback does not automatically mean low quality; it means secondary logic was used during the real workflow.
- Missing values are presented as a traceability limitation, not as a biological conclusion.
- Do not infer fallback use after the fact.

#### 6. Quality Check Summary Bar

Question answered:

`How many audit checks passed, warned, failed, or were not applicable?`

Data:

- quality check result statuses

Design:

- small stacked bar or status strip:
  - pass
  - warn
  - fail
  - not applicable
  - not instrumented

Interpretation:

This gives a quick audit overview while the detailed table explains each check.

#### 7. Sequence Export Completeness Chart

Question answered:

`If peptide text was requested, how complete was the sequence export?`

Data:

- sequence records requested
- written
- skipped
- failed
- unavailable

Design:

- donut chart or stacked bar
- render only when text export was requested or sequence records were already fetched

Important:

If the workflow currently cannot distinguish skipped/failed/unavailable, state that sequence export status was not separately instrumented.

## Table Blueprint

### General Table Rules

- Tables must have clear names and a one-sentence purpose before or after the table.
- Use `Source` or `Data source` columns for audit tables.
- Use `Meaning` and `Blank meaning` columns for dictionaries.
- Do not force very wide tables into tiny text. Split wide tables into logical groups when needed.
- Repeated long strings are summarized once and referenced by label.

### Applied Table Styles

| Table type | Visual style | Notes |
| --- | --- | --- |
| Key-value facts | two or three columns | compact; good for software/system info |
| Traceability | many columns, small font | use for search terms, labels, files |
| Quality checks | badge column + explanation | keep explanations readable |
| Technical appendix | key-value blocks per file | avoid giant all-files table when values are long |
| Column dictionary | multi-page table | group by source family if large |

### Status Vocabulary

Use these exact status labels:

| Status | Meaning |
| --- | --- |
| `ok` | step completed successfully |
| `pass` | quality check passed |
| `warn` | usable but requires review |
| `failed` | step failed and affected output |
| `not requested` | user did not request this output or process |
| `not applicable` | concept does not apply to this mode/data |
| `not instrumented` | useful metric cannot yet be reported |
| `not available in this run` | data was not present and was not fetched for report |

## Narrative Style

The report explains what happened without sounding like a debug log.

Every chapter must contain professional natural-language explanation in addition to any chart or table.
The renderer generates these paragraphs from structured data, and the tone remains formal, precise, and suitable for a research/commercial audit report.

Minimum prose blocks for every chapter:

1. Opening explanation.
   State what this chapter covers, why it matters, and which part of the export action it documents.

2. Method or evidence explanation.
   Explain how the values in the chapter were produced or collected, using only already-known workflow state.

3. Interpretation.
   Explain what the table or chart means in neutral language. Do not make unsupported biological claims.

4. Data-source or limitation note.
   State the source of the information and whether anything is unavailable, not requested, not applicable, or not instrumented.

Short chapters combine these into two paragraphs when appropriate, but no chapter consists only of headings, tables, charts, or bullet lists.

Use this style:

- "The workflow searched [database] for [N] keyword term(s) in the selected species."
- "Rows were grouped by the original search term so each exported row can be traced back to its input."
- "No additional database lookup was performed for this report."
- "This field was not available in the current workflow state."

Avoid:

- speculative claims about biological correctness
- "probably", "maybe", or "seems" in audit statements
- unexplained abbreviations
- long raw logs when a method table would be clearer

## Data Availability And Instrumentation Policy

The first implementation does not have to capture every useful metric. The report remains honest and polished by marking unavailable values explicitly.

When data is unavailable, show one of these:

| Situation | Report text |
| --- | --- |
| the workflow did not store the value | `Not instrumented in this build` |
| the source did not provide the value | `Not provided by source data` |
| the user did not request the related output | `Not requested` |
| the mode cannot produce the value | `Not applicable to keyword mode` |
| the value would require an extra lookup | `Not available in this run; no additional lookup was performed for report generation` |

Do not hide a whole section only because one field is missing. Keep the section and mark missing fields clearly, unless the section is conditional, such as sequence export audit when text export was not requested.

## Final Chapter Order

The keyword report is rendered in the following chapter order. Chapter length is data-dependent; do not tie chapter numbers to fixed PDF page numbers.

## Chapter Composition Plan

This section defines the chapter flow. Actual pagination changes with data volume, but the visual composition remains stable.

### Chapter 1: Cover And Executive Summary

Layout:

- top: report title and subtitle
- below title: one-paragraph executive summary in normal prose
- middle: 6 to 8 summary cards in a two-column or four-column grid
- lower: run facts table
- footer: report generation timestamp and page number

Purpose:

The reader knows within the opening chapter:

- what was exported
- from which database/species
- how many rows/files were involved
- when the run happened
- that the report did not perform extra database lookups

### Chapter 2: Generated File Index

Layout:

- short paragraph explaining the output package
- file index table
- file size bar chart if more than one generated file exists
- small note pointing to the technical appendix for hashes

Purpose:

The file index is an entry map. It makes paths and file roles easy to find.

### Chapter 3: Runtime And Reproducibility Context

Layout:

- software table
- user/session table
- runtime location table
- system/terminal table

If these tables become long, split system/terminal to a continuation page.

Purpose:

This section supports reproducibility and audit review. Keep it dense but readable.

### Chapter 4: Time And Performance Overview

Layout:

- time-window paragraph
- timeline table
- workflow duration stacked bar
- performance metrics table

Purpose:

Separate user time from active processing time. This prevents the report from implying that slow manual review is software latency.

### Chapter 5: Data Source And Species

Layout:

- database/source method paragraph
- database table
- species table
- no-extra-lookup note

Purpose:

This chapter establishes biological/data context without fetching anything new.

### Chapter 6: Keyword Inputs And Matching Method

Layout:

- search term summary table
- matching method steps
- per-term result mini-summary when there are many terms

Purpose:

Every exported row is traceable to its input term and source workflow.

### Chapter 7: Label Handling

Layout:

- label method paragraph
- label precedence block
- label traceability table

Purpose:

Make label handling transparent because labels affect file names, headers, and human interpretation.

### Chapter 8: Result And Selection Analytics

Layout:

- statistic cards
- selection donut chart with adjacent values table
- per-term stacked bar chart when applicable
- row status table

Purpose:

This is the most user-facing analysis chapter. It is clear, visual, and scientifically cautious.

### Chapter 9: Provenance And Quality Checks

Layout:

- provenance summary paragraph
- provenance donut chart with category table
- quality check summary bar
- detailed quality check table

Purpose:

Show direct/fallback/generated/missing proportions and transparent checks without inventing a hidden quality score.

### Chapter 10: Column Dictionary

Layout:

- grouped dictionary tables
- group by original source columns, generated/internal columns, dynamic lemna columns, export metadata columns

Purpose:

This can span pages. It is reference material, so clarity beats compactness.

### Chapter 11: Export Settings And Generation Log

Layout:

- export settings table
- generation log table
- export generation duration chart when enough timing exists

Purpose:

This section explains exactly how the files were produced.

### Conditional Chapter: Sequence Export Audit

Render only when peptide text export was requested or sequence records already exist in workflow state.

Layout:

- sequence cards
- completeness chart
- sequence status table

Purpose:

Show whether the peptide text file is complete and where sequence data came from, without refetching.

### Final Chapters: File Technical Details And Limitations

Layout:

- one technical subsection per file
- full hashes
- metadata
- limitations and reproducibility notes

Purpose:

The final chapters support verification. They are more technical than earlier chapters.

## Chapter Content Contracts

This section is the executable writing contract for each chapter. The renderer treats these as required content blocks.
If a data block cannot be populated, render the chapter with an explicit unavailable/not-instrumented explanation rather than deleting the chapter silently, except for the conditional sequence export chapter when peptide text was not requested.

### Chapter 1 Contract: Cover And Executive Summary

Required prose:

1. Executive summary paragraph.
   State that this is a keyword-mode export report generated by `phytozome GO`. Include database, species, search-term count, total result-row count, selected/exported row count, generated-file count, and export completion timestamp.

2. Scope paragraph.
   State that the report covers only the current export action and the data already known to the program during that action. Explicitly state that no additional database/API/search operation was performed for the report.

3. Reading guide paragraph.
   Tell the reader where to find the output files, method traceability, selection/provenance analysis, and technical hashes.

Required visual/data blocks:

- report title and subtitle
- summary cards for database, species, selected rows, total rows, search terms, generated files, and export completion time
- run facts table with values and data sources

Required fallback behavior:

- If software version/build metadata is absent, write `Version metadata was not embedded in this build` rather than leaving the value blank.

### Chapter 2 Contract: Generated File Index

Required prose:

1. Output package paragraph.
   Explain that the files listed here are the complete output package produced by this keyword export action.

2. File-role paragraph.
   Explain the role of selected Excel, raw Excel, peptide text, and report PDF. Mention only file types that were generated or explicitly not requested.

3. Verification paragraph.
   Explain that hashes and filesystem metadata are in the technical appendix because full hashes are long and intended for byte-level verification.

Required visual/data blocks:

- generated file index table
- file size chart when at least two generated files exist
- technical details links or textual references to appendix subsections

Required fallback behavior:

- If a file type was not requested, do not include it as a generated file row. Mention it in prose only if useful for explaining the export settings.

### Chapter 3 Contract: Software, User, Runtime, And System Context

Required prose:

1. Reproducibility paragraph.
   Explain that this chapter identifies the software build, user/session context, runtime location, and system environment used to create the export.

2. Collection-method paragraph.
   Explain that these values come from Go runtime APIs, OS/user APIs, environment variables, executable file metadata, and app filesystem helpers.

3. Privacy/availability paragraph.
   State that some user/system fields can be unavailable depending on OS permissions and platform APIs. Unavailable fields are reported as unavailable rather than inferred.

Required visual/data blocks:

- software table
- user/session table
- runtime location table
- system/terminal table

Required fallback behavior:

- Terminal version must be marked unavailable unless already present in environment variables or cheap local metadata.
- Do not run shell commands to discover terminal versions for report generation.

### Chapter 4 Contract: Time Window And Performance Overview

Required prose:

1. Time-window paragraph.
   State the measured interval from keyword query/search start to export completion, with local timezone and UTC equivalent when available.

2. Performance-method paragraph.
   Explain which phases were measured and how durations were calculated from recorded timestamps.

3. Interpretation paragraph.
   Identify the longest measured phases and separate active software processing from user review/decision time.

4. Limitation paragraph.
   State which timing phases are not instrumented in the current build, if any.

Required visual/data blocks:

- timeline table
- workflow duration stacked bar when at least two measured phases exist
- performance metrics table with only computable metrics

Required fallback behavior:

- Do not estimate missing durations.
- Do not compute throughput metrics unless both numerator and denominator are known.

### Chapter 5 Contract: Data Source And Species Context

Required prose:

1. Data-source paragraph.
   State which database/source was selected and how keyword data were obtained during the real workflow.

2. Species paragraph.
   Identify the selected species using all already-known model fields useful for biological traceability.

3. No-extra-lookup paragraph.
   Explicitly state that no database lookup, release lookup, or capability refresh was performed for report writing.

Required visual/data blocks:

- database/source table
- species detail table
- availability notes for missing species fields

Required fallback behavior:

- If release/capability/download details are not already present, mark them unavailable in this run.

### Chapter 6 Contract: Keyword Input And Matching Method

Required prose:

1. Input paragraph.
   State the exact number of search terms and explain that original order was preserved.

2. Matching-method paragraph.
   Describe how the selected source matched keyword terms, using source-specific language for Phytozome or lemna.

3. Grouping paragraph.
   Explain that rows are grouped by `search_tern` so each row is traceable to the originating input term.

4. Selection handoff paragraph.
   Explain that the result table was presented for user review and that only selected rows entered selected-output writers.

Required visual/data blocks:

- search term summary table
- per-term traceability table if more detailed than summary table
- numbered method steps

Required fallback behavior:

- Input type must be `unknown/unclassified` if parser state cannot classify it.

### Chapter 7 Contract: Label Handling And Traceability

Required prose:

1. Label-purpose paragraph.
   Explain that labels affect exported file names, FASTA/text headers, grouping readability, and downstream human interpretation.

2. Label-method paragraph.
   State whether labels were manual, skipped, or auto-identified if the workflow recorded that state.

3. Label-precedence paragraph.
   Describe the auto-identification precedence and clearly state that fallback searches are only described if they already occurred during the real label step.

4. Missing-instrumentation paragraph.
   If label source/method is not tracked, state that the final label is available but its exact inference source was not instrumented.

Required visual/data blocks:

- label precedence block
- label traceability table

Required fallback behavior:

- Do not run fallback label searches for the report.

### Chapter 8 Contract: Result Set And User Selection Analytics

Required prose:

1. Result-set paragraph.
   State the total rows loaded in the current keyword result table and the number/percentage selected for export.

2. Selection interpretation paragraph.
   Explain what selected versus unselected proportions mean operationally. Do not claim that selected rows are biologically superior solely because they were selected.

3. Per-term interpretation paragraph.
   If multiple terms exist, explain which terms contributed most rows and whether selection was concentrated or broadly distributed.

4. Limitation paragraph.
   State whether unselected rows were preserved in raw Excel or only summarized in the report, depending on user settings.

Required visual/data blocks:

- statistic cards
- selection donut chart
- selected/unselected values table
- per-term stacked bar chart when more than one term exists
- row status table

Required fallback behavior:

- If total rows are zero, omit selection donut and explain that no result rows were available for selection.

### Chapter 9 Contract: Data Provenance And Quality Checks

Required prose:

1. Provenance purpose paragraph.
   Explain that provenance identifies whether values came directly from the selected source, local parsed release data, cache-backed workflow state, fallback logic, generated/internal metadata, or missing/unavailable fields.

2. Provenance interpretation paragraph.
   Explain the observed proportions. Make clear that fallback is not automatically a quality failure; it is traceability information.

3. Quality-check paragraph.
   Explain that checks are transparent audit checks, not a hidden global quality score.

4. Limitations paragraph.
   State whether provenance is row-level, field-level, or not instrumented.

Required visual/data blocks:

- provenance donut chart when supported by data
- provenance category table
- quality check summary bar
- detailed quality check table

Required fallback behavior:

- If provenance is not instrumented, render a source/method note and still render available quality checks.
- Do not infer fallback categories from blank values.

### Chapter 10 Contract: Column Dictionary And Data Lineage

Required prose:

1. Dictionary purpose paragraph.
   Explain that this chapter defines each exported/detail column and how the reader interprets it.

2. Source-family paragraph.
   Explain how columns are grouped by source family: source database fields, generated/internal fields, dynamic lemna/GFF3/AHRD fields, and export metadata fields.

3. Blank-value paragraph.
   Explain the difference between missing source values, not applicable fields, not requested outputs, and not instrumented fields.

Required visual/data blocks:

- grouped column dictionary table(s)
- source family labels
- blank meaning for every column

Required fallback behavior:

- Dynamic columns must use their collected names and best-known source family. If detailed meaning is unknown, say `Meaning not instrumented; column was present in source/export data`.

### Chapter 11 Contract: Export Settings And Generation Log

Required prose:

1. Export-settings paragraph.
   Explain which file types the user requested and how the base name/output folder affected generated files.

2. Generation-method paragraph.
   Explain the chronological file-generation process from output directory resolution to report rendering.

3. Performance interpretation paragraph.
   Explain which generation steps took the most time when timings are available.

4. Error/recovery paragraph.
   Summarize only errors, retries, skips, or warnings that affected the current export action.

Required visual/data blocks:

- export settings table
- generation log table
- export generation duration chart when enough timing exists

Required fallback behavior:

- If a step was not requested, mark it `not requested`; do not omit it from the generation log if the setting table makes it relevant.

### Conditional Chapter Contract: Sequence Export Audit

Required prose:

1. Applicability paragraph.
   State whether peptide text export was requested. If not requested, this chapter is omitted.

2. Completeness paragraph.
   If requested, state how many sequence records were requested, written, skipped, failed, or unavailable when that state is known.

3. Source paragraph.
   Explain whether sequences came from already-used workflow sequence fetching, cache-backed values, or source fetches that were necessary for text export.

4. Limitation paragraph.
   If per-row fetch status is not tracked, state that explicitly and do not reconstruct it.

Required visual/data blocks:

- sequence summary cards
- completeness chart when enough status categories are known
- sequence status table

Required fallback behavior:

- Never fetch sequences only for this chapter.

### Final Chapter Contract: File Technical Details Appendix

Required prose:

1. Verification paragraph.
   Explain that this appendix provides byte-level and filesystem-level details for verifying generated artifacts.

2. Hash paragraph.
   Explain SHA-256 and any secondary hashes that are implemented, including that the hash represents file bytes at report inspection time.

3. Report-self-hash paragraph.
   Explain why the report PDF usually cannot include its own final hash inside itself without changing the file bytes.

Required visual/data blocks:

- one technical metadata block per generated file
- full SHA-256 hash for every generated non-report file when possible
- report PDF self-hash limitation or external manifest note

Required fallback behavior:

- If owner/access time/creation time is unavailable on the OS, mark it unavailable and keep other metadata.

## 1. Cover And Executive Summary

### Presentation

First page. Use a clean title block, followed by a summary panel and a compact run facts table.

### Content

Title:

`Data Analysis Report`

Subtitle:

`Keyword Export Report`

Summary paragraph:

Describe the export action in natural language:

- database
- species
- number of search terms
- number of result rows
- number of selected/exported rows
- files generated
- export completion timestamp

Example text shape:

`This report documents a keyword-mode export generated by phytozome GO. The exported data were produced from [database] keyword searches for [species display label]. The current export action wrote [N] file(s), including [file types]. The report describes only the data and processing state already known to the program during this export. No additional database lookup was performed for report generation.`

Run facts table:

| Field | Value | Source |
| --- | --- | --- |
| Report file | `YYYYMMDD_HHMMSS_report.pdf` | report writer |
| Mode | Keyword | workflow |
| Database | Phytozome or lemna | selected data source |
| Species | display label | `model.SpeciesCandidate` |
| Search terms | count | keyword input state |
| Result rows | count | current keyword result rows |
| Selected rows | count | row-selection state |
| Exported files | count | generated file metadata |
| Query started | timestamp | workflow timestamp |
| Export completed | timestamp | workflow timestamp |

## 2. Generated File Index

### Presentation

Place immediately after the executive summary. This section is the user's map to the output package.
Use a table plus a short explanatory paragraph. Use internal links from each file row to the final `File Technical Details` appendix when supported.

### Content

Intro paragraph:

Explain that the listed files are the complete output set for this export action. Mention that file hashes are available in the appendix because they are long.

File index table:

| File | Type | Purpose | Size | Location | Technical details |
| --- | --- | --- | --- | --- | --- |
| `name.xlsx` | selected Excel | selected rows only | human + bytes | full path | link |
| `name_raw.xlsx` | raw Excel | all current table rows | human + bytes | full path | link |
| `name.txt` | peptide text | exported peptide records | human + bytes | full path | link |
| `YYYYMMDD_HHMMSS_report.pdf` | report PDF | this audit report | human + bytes | full path | link |

Purpose prose:

- selected Excel: explain that it contains the selected/exported keyword rows and export metadata when available
- raw Excel: explain that it preserves the full current table rows, including unselected rows, if the user requested it
- peptide text: explain that it contains peptide sequence records only if the user requested text export and sequence fetching already occurred
- report PDF: explain that it documents the export action and generated files

Chart:

Use a small horizontal bar chart showing generated file sizes by file type. This is useful when multiple files are generated.

Interpretation:

`The chart compares the byte size of the files generated in this export action. Large Excel files usually indicate many result rows or many dynamic annotation columns; large text files usually indicate many or long peptide sequences.`

Data source note:

`File sizes and timestamps were read from filesystem metadata after file generation.`

## 3. Software, User, Runtime, And System Context

### Presentation

Use four compact tables:

1. Software
2. User/session
3. Runtime location
4. System/terminal

This section reads as a reproducibility block in commercial scientific software.

### Software Table

| Field | Value | Source |
| --- | --- | --- |
| Software name | phytozome GO | constant |
| Executable name | value | `os.Executable` |
| Author | value or not embedded | project/build metadata |
| Repository | value or not embedded | project/build metadata |
| Version | value or not embedded | build metadata |
| Commit | value or not embedded | build metadata |
| Build date | value or not embedded | build metadata |
| Go version | value | `runtime.Version()` |
| Module path | value | Go build info |

### User And Session Table

| Field | Value | Source |
| --- | --- | --- |
| Username | value | `os/user` or env |
| User ID | value if available | `os/user` |
| Home directory | value if available | `os/user` |
| Host name | value | OS hostname |
| Process ID | value | `os.Getpid()` |
| Windows domain | value if available | `USERDOMAIN` |
| Windows session | value if available | `SESSIONNAME` |
| Login-related env | value if available | `USER`, `USERNAME`, `LOGNAME` |

### Runtime Location Table

| Field | Value | Source |
| --- | --- | --- |
| Working directory | value | `os.Getwd()` |
| Executable path | value | `os.Executable()` |
| Executable file size | value | filesystem metadata |
| Executable modified time | value | filesystem metadata |
| Output directory | value | app filesystem helper |
| Cache directory | value | app filesystem helper |

### System And Terminal Table

| Field | Value | Source |
| --- | --- | --- |
| OS | `runtime.GOOS` | Go runtime |
| Architecture | `runtime.GOARCH` | Go runtime |
| CPU count | value | `runtime.NumCPU()` |
| OS version/build | value or not available | platform helper |
| Kernel/release | value or not available | platform helper |
| Total memory | value or not available | platform helper |
| Available memory | value or not available | platform helper |
| Terminal env | `TERM`, `TERM_PROGRAM`, etc. | environment |
| Terminal version | value or not available | environment/process metadata |
| Parent process or shell | value or not available | platform helper/env |

Important rule:

Terminal versions must only come from environment variables or local process/file metadata. Do not run terminal update checks, package managers, network calls, or terminal-specific version commands for the report.

## 4. Time Window And Performance Overview

### Presentation

Use a timeline table, a stacked duration bar, and a short interpretation paragraph.

### Content

Time table:

| Phase | Start | End | Duration | Data source |
| --- | --- | --- | --- | --- |
| Query input/search | timestamp | timestamp | duration | workflow timestamps |
| Keyword search | timestamp | timestamp | duration | workflow timestamps |
| Label handling | timestamp | timestamp | duration | workflow timestamps |
| User row review | timestamp | timestamp | duration | workflow timestamps |
| Export setup | timestamp | timestamp | duration | workflow timestamps |
| Excel generation | timestamp | timestamp | duration | export step timestamps |
| Raw Excel generation | timestamp | timestamp | duration | export step timestamps |
| Peptide text generation | timestamp | timestamp | duration | export step timestamps |
| File metadata/hash capture | timestamp | timestamp | duration | report step timestamps |
| PDF rendering | timestamp | timestamp | duration | report step timestamps |

If the current implementation has not yet instrumented a phase, display:

`Not available in this run; this phase was not timestamped separately.`

Performance chart:

Use a horizontal stacked bar chart:

- each segment is a measured phase duration
- active compute/network/file phases use one color family
- user decision/review phases use a distinct neutral color if measured
- unmeasured phases must not be guessed

Interpretation paragraph:

Explain which phases dominated the run. For example:

`Most measured time was spent in keyword search and peptide sequence export. User review time is shown separately from active processing time because it reflects interactive decision time rather than software throughput.`

Additional performance metrics table:

| Metric | Value | Meaning |
| --- | --- | --- |
| Search terms per second | computed if timing exists | throughput of keyword search |
| Rows found per second | computed if timing exists | result production rate |
| Exported rows per second | computed if timing exists | file writing throughput |
| Bytes written per second | computed if timing exists | output I/O throughput |
| Hashing throughput | computed if timing exists | file verification throughput |

Important rule:

Do not invent performance metrics. Only compute from measured timestamps and known counts/byte sizes.

## 5. Data Source And Species Context

### Presentation

Use a prose method block followed by two tables.

### Content

Method paragraph:

Describe the selected database and species. Explicitly state that no extra lookup was performed for report writing.

Database table:

| Field | Value | Source |
| --- | --- | --- |
| Database | Phytozome or lemna | selected source |
| Mode | Keyword | workflow |
| Source strategy | API/search or local release parsing | workflow/source state |
| Cache usage | value if known | workflow/cache state |
| Fallback usage | value if known | workflow state |

Species table:

Include every known field from `model.SpeciesCandidate` and related already-collected workflow state:

- display label
- genome label
- common name
- scientific/search name
- JBrowse name
- proteome ID
- target ID
- release date
- release/version label
- official clone marker
- source/download URLs already known
- capability flags already known

Missing data note:

`Fields marked "not available in this run" were not present in the species model or current workflow state. The report did not perform additional database lookups to fill them.`

## 6. Keyword Input And Matching Method

### Presentation

Use one summary table, one per-term traceability table, and a detailed prose method log.

### Search Term Summary Table

| Search term | Input type | Query order | Result rows | Selected rows | Label name | Notes |
| --- | --- | --- | --- | --- | --- | --- |

Input type classification is based only on existing parser state:

- plain keyword
- identifier-like term
- report URL
- protein ID
- transcript ID
- gene ID
- loaded batch input
- unknown/unclassified

### Matching Method Log

Write this as numbered prose steps, not only bullets:

1. Input parsing.
   Describe how the input was split into terms while preserving order.

2. Species binding.
   State which selected species/database context was used.

3. Search execution.
   For Phytozome, describe that result rows came from the keyword search mechanism already used by the workflow.
   For lemna, describe that rows came from release-backed GFF3/AHRD/search state already loaded for the run.

4. Row grouping.
   Explain that result rows are grouped by `search_tern` and that sorting occurs only inside each term group.

5. Row display and selection.
   Explain that the user selected rows from the current table and the export uses that selection.

6. Export.
   Explain which file writers consumed the selected rows and which file writers consumed all current rows.

## 7. Label Handling And Traceability

### Presentation

Use a label-method prose block and a label traceability table.

### Content

Label method paragraph:

Explain whether labels were manually entered, skipped, or auto-identified when known.

Auto-identification precedence:

1. explicit row `label_name`
2. first alias
3. gene/transcript/sequence identifier

Label traceability table:

| Search term | Final label | Source field | Source value | Method | Explanation |
| --- | --- | --- | --- | --- | --- |

Rules:

- Do not run new fallback searches for the report.
- If source field/method is not tracked, say it is not available and recommend future instrumentation.
- Explain how labels affect exported file names, FASTA headers, and row readability.

## 8. Result Set And User Selection Analytics

### Presentation

This is the main data overview section. It uses statistic cards, a selection pie chart, grouped bars when multiple terms exist, and explanatory text.

### Statistic Cards

Display compact cards for:

- total result rows
- selected/exported rows
- unselected rows
- search terms
- search terms with at least one hit
- search terms with zero hits
- generated files

### Selection Pie Chart

Purpose:

Show the quantity relationship between all result rows and the rows the user selected for export.

Chart design:

- donut or pie chart
- slices:
  - selected/exported rows
  - unselected rows
- center label for total rows if using donut
- adjacent values table with counts and percentages

Interpretation paragraph:

`This chart shows how much of the current keyword result table was included in the export. A high selected percentage indicates that most returned rows were kept; a low selected percentage indicates stronger manual filtering by the user.`

Data source note:

`Selection counts come from the current row-selection state and the result table already loaded in the workflow.`

### Per-Term Result Chart

Use a stacked bar chart when multiple search terms exist.

For each search term:

- selected rows
- unselected rows

Interpretation:

This shows whether selection behavior was consistent across terms or concentrated in particular terms.

Fallback:

If there is only one search term, omit this chart and keep the summary table.

### Row Status Table

| Search term | Total rows | Selected rows | Unselected rows | Selected % |
| --- | --- | --- | --- | --- |

## 9. Data Provenance And Quality Checks

### Presentation

Use a quality summary paragraph, a provenance pie chart, and a check table.

This section answers: "Where did the data come from, and how direct or fallback-dependent was it?"

### Provenance Pie Chart

Purpose:

Show the proportion of row values or rows by provenance category. Implement whichever granularity is actually trackable first; do not guess.

Preferred row-level categories:

- direct source result: row came directly from the selected database/search mechanism
- local release parsed: row came from already-loaded lemna GFF3/AHRD/release assets
- cache-backed: row came from an existing cache used during normal workflow
- fallback label/enrichment already performed: value was filled by a fallback that already ran before report generation
- generated/internal: value was generated by the program, such as row number, `search_tern`, generated report URL, export header, or label assignment
- unavailable/missing: expected field was blank or not collected

If row-level provenance is not available, use field-level provenance for exported columns.

Chart design:

- pie or donut chart with counts and percentages
- include a table below because provenance labels can be long

Interpretation paragraph:

`This chart summarizes how much of the exported dataset came directly from the selected source versus local parsed data, cache-backed data, fallback-derived values, or internally generated metadata. Fallback-derived values are not bad by themselves; they identify values that required secondary logic during the actual workflow.`

Important rule:

Only include fallback categories if fallback use was already recorded. Do not retroactively infer fallback use from blank fields.

### Quality Check Table

| Check | Result | Count | Explanation | Data source |
| --- | --- | --- | --- | --- |
| Search terms with zero hits | pass/warn | count | term had no rows | search groups |
| Rows missing label | pass/warn | count | label can be blank if skipped | row data |
| Rows missing transcript ID | pass/warn | count | affects traceability | row data |
| Rows missing gene identifier | pass/warn | count | source did not provide gene ID | row data |
| Rows missing description | pass/warn | count | annotation unavailable | row data |
| Rows missing report URL | pass/warn | count | direct source page unavailable | row data |
| Duplicate transcript/sequence IDs | pass/warn | count | can represent duplicate hits/isoforms | row data |
| Sequence export completeness | pass/warn/not applicable | fetched vs requested | only for text export | sequence export state |
| Raw Excel completeness | pass/not requested | rows written | raw file option | export settings |

Quality scoring:

Do not invent a single global quality score unless all component rules are explicit and shown. Prefer transparent checks over a black-box score.

## 10. Column Dictionary And Data Lineage

### Presentation

Use a detailed table. This can span multiple pages.

### Content

For every exported/detail column, include:

| Column | Meaning | Source | Collection method | Blank meaning | Used in report stats |
| --- | --- | --- | --- | --- | --- |

Examples:

- `row`: generated by phytozome GO to preserve table order
- `search_tern`: original search term that produced the row
- `label_name`: user-supplied or auto-identified label
- `protein_id`: source protein identifier when available
- `transcript`: source transcript identifier
- `gene_identifier`: source gene identifier
- `genome`: source genome/release label
- `location`: genomic location from source data when available
- `alias`: source aliases
- `uniprot`: source-provided UniProt cross-reference if present in current data
- `description`: source annotation/description
- `comments`: source comments or parsed annotation notes
- `auto_define`: source or internal annotation field already present
- `gene_report_url`: source report URL generated or provided during the workflow
- dynamic lemna/GFF3/AHRD columns: describe based on collected column name and source family

Blank meaning must be specific:

- not provided by source
- not applicable to this database/mode
- not collected in this run
- user skipped
- sequence export not requested

## 11. Export Settings And Generation Log

### Presentation

Use a settings table and a chronological generation log table.

### Export Settings Table

| Setting | Value | Effect |
| --- | --- | --- |
| File base name | value | base for Excel/text outputs |
| Output folder | value | destination directory |
| Write selected Excel | true/false | selected rows are written |
| Write raw Excel | true/false | all current rows are written |
| Write peptide text | true/false | peptide sequences are written |
| Write report PDF | true/false | this report is written |

### Generation Log Table

| Step | Start | End | Duration | Status | Details |
| --- | --- | --- | --- | --- | --- |
| Resolve output directory | timestamp | timestamp | duration | ok/warn/error | path |
| Write selected Excel | timestamp | timestamp | duration | ok/not requested | rows written |
| Write raw Excel | timestamp | timestamp | duration | ok/not requested | rows written |
| Fetch/use peptide sequences | timestamp | timestamp | duration | ok/not requested/warn | counts |
| Write peptide text | timestamp | timestamp | duration | ok/not requested | records written |
| Capture file metadata | timestamp | timestamp | duration | ok/warn | files inspected |
| Compute hashes | timestamp | timestamp | duration | ok/warn | algorithms |
| Render report PDF | timestamp | timestamp | duration | ok/warn | renderer |

Performance chart:

Use a second stacked duration bar focused only on file generation phases.

Interpretation:

`This chart separates file-writing and verification time from earlier keyword search and review time. It helps identify whether export time was dominated by Excel writing, peptide sequence handling, hashing, or PDF rendering.`

## 12. Sequence Export Audit

Render this section only when peptide text export was requested or sequence records were otherwise already fetched for the current export.

### Presentation

Use a summary card set, a completeness chart, and a sequence table.

### Content

Cards:

- sequence records requested
- sequence records written
- cache hits if known
- direct fetches if known
- failed/skipped records if known
- total amino acid characters written

Completeness pie chart:

- written
- skipped/failed
- not available

Sequence table:

| Row | Search term | Label | Sequence ID | Transcript | Status | Length | Source |
| --- | --- | --- | --- | --- | --- | --- | --- |

Rule:

If sequence fetch status is not currently recorded, do not reconstruct it by re-fetching. State that sequence status was not separately instrumented.

## 13. File Technical Details Appendix

### Presentation

This appendix is the target of "technical details" links from the file index.
Use one subsection per generated file.

### Per-File Content

For each generated file:

| Field | Value |
| --- | --- |
| File name | value |
| Full path | value |
| Type | selected Excel/raw Excel/text/report |
| Size | bytes and human-readable |
| Created time | value if available |
| Modified time | value |
| Accessed time | value if available |
| Permissions/mode | value |
| Owner | value if cheaply available |
| SHA-256 | full hash |
| SHA-1 | secondary hash when implemented |
| MD5 | secondary compatibility hash when implemented |
| Hash calculated at | timestamp |

Hash explanation:

`The hash identifies the exact byte content of the file at the time the report inspected it. If any file is edited after report generation, the hash will no longer match.`

Report PDF self-hash rule:

The PDF cannot reliably include its own final SHA-256 inside itself without changing its bytes. Acceptable options:

- omit the report PDF self-hash and explain why
- include the report PDF hash in an external manifest
- render the report, compute the hash, and store it in PDF metadata only if the renderer can do so without invalidating the hash, which is uncommon

## Implementation Data Model Hints

These are not required PDF sections, but they guide implementation.

Applied top-level structure names:

- `ReportData`
- `ReportSoftwareInfo`
- `ReportUserSession`
- `ReportSystemInfo`
- `ReportTiming`
- `ReportGeneratedFile`
- `KeywordReportData`
- `KeywordSearchTermReport`
- `KeywordLabelReport`
- `KeywordSelectionStats`
- `KeywordProvenanceStats`
- `KeywordQualityCheck`
- `ReportGenerationStep`

Implementation rule:

Capture timing and provenance at the moment the workflow performs the work. Do not try to infer everything after export completion.

## Renderer Acceptance Criteria

A generated keyword report is acceptable only if it satisfies all of the following.

### Content Completeness

- The report has a cover/title section.
- The report identifies software, user/session, runtime location, system, and terminal context where available.
- The report includes query/export time windows.
- The report lists all files generated by the current export action.
- The report includes full file technical details and at least SHA-256 hashes where possible.
- The report states that no extra lookup was performed for report generation.
- The report includes keyword input, matching, label, selection, provenance, quality, and export sections.
- Conditional sections clearly state `not requested`, `not applicable`, or `not instrumented` instead of silently disappearing, except for sequence export audit when text export is not requested.

### Visual Quality

- The report is readable when printed in grayscale.
- Tables do not overlap text or clip important values.
- Long paths and hashes wrap or move to appendix sections.
- Charts include counts/percentages and are backed by adjacent tables.
- No chapter or rendered page contains only a heading with no meaningful content.
- Footer page numbers are present.
- Section titles and source notes are visually consistent.

### Audit Safety

- No renderer path performs database, API, BLAST, sequence-fetch, download, or cache-refresh work.
- Missing data is labeled honestly.
- Performance metrics are only computed from measured timestamps and known counts.
- Provenance categories are only used when the workflow recorded enough information to support them.
- Quality checks expose their rule and count; no hidden global quality score is used.

### Professional Tone

- Prose explains the workflow in neutral, precise language.
- The report avoids speculative biological claims.
- The report distinguishes data-source limitations from software failures.
- The report uses concise explanations rather than raw debug logs.

## Future Enhancements

These additions are allowed later only under the same no-extra-lookup rule.

- Add richer provenance instrumentation during normal workflow execution.
- Record per-term search timing.
- Record cache hit/miss counts for keyword searches and sequence fetches.
- Record label inference source and confidence at the time labels are assigned.
- Record sequence export status per row.
- Add an external manifest for the report PDF's own hash when report self-hashing is required.
- Add a small glossary chapter for users who are less familiar with terms such as GFF3, AHRD, hash, provenance, and fallback.

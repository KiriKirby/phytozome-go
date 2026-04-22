# BLAST Workflow Agent

This file is the working spec for the first usable version of `phytozome-batch-cli`.

## Product goal

Replace the repetitive browser workflow on `phytozome-next.jgi.doe.gov` with one terminal workflow that:

1. finds the target species
2. runs one BLAST search
3. shows the full result table
4. lets the user choose rows interactively
5. exports the selected rows to Excel and peptide text

## Non-goals for v1

- support every Phytozome feature
- cover non-BLAST flows such as keyword gene search
- implement a full-screen TUI
- optimize for headless bulk mode before the interactive path works

## Workflow contract

### Step 1: species search

User input:

- species keyword, free text

System behavior:

- query Phytozome for matching species/genomes
- display a numbered candidate list
- keep enough metadata to identify the exact selected genome later

Output object:

- `SpeciesCandidate`
  - display label
  - internal species/genome identifier
  - optional version label

### Step 2: species selection

User action:

- choose exactly one candidate

System behavior:

- persist the chosen target genome in workflow state

### Step 3: BLAST query input

User input:

- sequence text
- optional BLAST mode, if Phytozome requires it later

System behavior:

- submit the BLAST request against the chosen genome
- wait for the result page or result payload

### Step 4: collect result table

System behavior:

- fetch every row and every visible column from the BLAST result table
- preserve column order from the site as much as possible
- add one derived column:
  - `gene_report_url`

Expected row fields, based on the current screenshots:

- protein
- species
- e-value
- percent identity
- align len
- strands
- query id
- query start/end or equivalent query-side columns
- subject-side columns if present
- gene report URL derived from the row

Output object:

- `BlastResultRow`
  - `columns map[string]string`
  - `ordered_values []Cell`
  - `protein_id string`
  - `species_label string`
  - `gene_report_url string`

### Step 5: row selection

System behavior:

- display the result rows in an interactive multi-select list
- include:
  - `select all`
  - `select none`
- selected rows become the only export source

CLI simplification rule:

- do not build a complex TUI first
- use a prompt-based list with pagination if the result set is large

### Step 6: export files

Generate two files from selected rows only.

#### 6.1 Excel export

Requirements:

- one row per selected BLAST result row
- all columns from the BLAST table
- append `gene_report_url` as the last column

Recommended file name:

- `blast_results_<timestamp>.xlsx`

#### 6.2 Peptide text export

Requirements:

- visit each selected `gene_report_url`
- extract `Peptide sequence`
- write one FASTA-like block per selected row
- separate blocks with a blank line

Header format:

- `>{species_label}|{protein_id}`

Body format:

- raw peptide sequence as displayed by the gene report page

Recommended file name:

- `blast_peptides_<timestamp>.txt`

## Proposed code layout

```text
cmd/phytozome-batch-cli/
internal/app/
internal/model/
internal/phytozome/
internal/prompt/
internal/export/
internal/workflow/
```

Module roles:

- `internal/model`
  - shared structs such as `SpeciesCandidate`, `BlastResultRow`, `PeptideRecord`
- `internal/phytozome`
  - all site-specific requests, parsing, selectors, URL derivation
- `internal/prompt`
  - single-select and multi-select prompts
- `internal/export`
  - Excel and text writers
- `internal/workflow`
  - the orchestration for the interactive BLAST command
- `internal/app`
  - command dispatch and shared runtime setup

## Technical decisions

### Why Go

- cross-platform single binary
- easy distribution for Windows, macOS, Linux
- good enough HTTP and HTML tooling
- simple concurrency for fetching many gene report pages

### Why avoid heavy architecture

- the hardest part is site integration, not local application complexity
- keep Phytozome interaction behind one adapter
- keep the CLI flow linear and stateful

### Excel dependency

The standard library cannot write `.xlsx`. Use one focused dependency for that task when implementation starts.

Preferred candidate:

- `github.com/xuri/excelize/v2`

### Interactive selection dependency

A minimal prompt library is acceptable if it saves time and stays cross-platform.

Preferred candidate:

- `github.com/AlecAivazis/survey/v2`

If `survey` cannot cleanly support `select all` and `select none`, wrap those as synthetic options or fall back to a numbered text prompt.

## Risks and unknowns

### Authentication

Phytozome may require a login or session before BLAST access is fully available. Keep auth/session handling isolated.

### Dynamic UI

The site is JavaScript-heavy. We should first look for stable request endpoints behind the UI instead of scraping the rendered screen directly.

### URL derivation

The final `gene_report_url` must be derived from stable row identifiers. Do not hardcode the sample URL pattern until the live row payload is verified.

Current finding:

- the general frontend contains transcript-report links in other flows
- BLAST protein rows resolve to:
  - `/report/protein/{jbrowseName}/{sequenceId}`
- the export column should therefore use the protein-report URL for BLAST protein hits

### Large result sets

BLAST can return many rows. Avoid rendering the entire table in one unpaged terminal view if usability suffers.

## Confirmed endpoints

These were verified from the live `phytozome-next` frontend bundle on April 22, 2026.

### BLAST submit and poll

- submit pasted sequence:
  - `POST /api/blast/submit/sequence`
- submit uploaded sequence file:
  - `POST /api/blast/submit/sequence-in-file`
- poll result payload:
  - `GET /api/blast/results/{jobId}`

Observed submit payload fields:

- `targets`
- `targetType`
- `program`
- `eValue`
- `comparisonMatrix`
- `wordLength`
- `alignmentsToShow`
- `allowGaps`
- `filterQuery`
- optional `userEmail`
- optional `emailResults`
- one of:
  - `sequence`
  - `sequenceInFile`

### Sequence retrieval

- peptide sequence:
  - `GET /api/db/sequence/protein/{transcriptId}`

### Proteome and info pages

- proteome properties:
  - `GET /api/db/properties/proteome/{proteomeId}`
  - `GET /api/db/properties/proteome/{proteomeId}?format=verbose`
- info page data:
  - `GET /api/content/info/{proteomeId}`

### Home/project content

- project/home content:
  - `GET /api/content/project/{groupId}`

Confirmed examples:

- `GET /api/content/project/phytozome`
- `GET /api/content/project/home`
- `GET /api/content/project/main`

The `phytozome` project overview HTML includes many `/info/{jbrowseName}` links and can serve as a fallback source for candidate genomes if no cleaner search endpoint is found.

## Implementation order

1. Lock this workflow spec
2. Build command skeleton and data models
3. Inspect live Phytozome requests for species search and BLAST submission
4. Implement species candidate sourcing and local filtering
5. Implement BLAST submit and result polling
6. Implement result-table parsing
7. Implement row selection UX
8. Implement Excel export
9. Implement peptide fetch and text export
10. Add non-interactive flags later

## Current implementation state

Implemented:

- candidate species fetch from project overview plus frontend target metadata
- keyword filter and single-species selection
- sequence paste input
- automatic DNA/protein detection
- BLAST submit via `/api/blast/submit/sequence`
- polling via `/api/blast/results/{jobId}`
- BLAST XML parsing into structured rows
- terminal-table rendering for parsed BLAST rows
- derived protein report URLs for parsed rows
- row selection commands: `all`, `none`, `toggle`, `done`
- `.xlsx` export for selected rows
- protein sequence export to FASTA-like `.txt`
- live protein-sequence retrieval via:
  - `/api/db/gene_{proteome}?protein={proteinId}`
  - `/api/db/sequence/protein/{internalTranscriptId}`

Pending:

- selector pagination or a denser row-selection UX for very large result sets
- output-path flags and non-interactive batch mode

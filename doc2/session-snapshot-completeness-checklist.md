# Session Snapshot Completeness Checklist

This document defines the snapshot-completeness contract for every current database, mode, transition, and artifact class in `phytozome GO`.

The governing rule is strict:

- if the program has already generated it, the snapshot should preserve it
- reopening a snapshot should continue from saved state, not rebuild from upstream inputs
- raw cached payloads may be packed in their original formats, including binary files, when that is the simplest and most reliable preservation path
- file size is not a reason to omit saved workflow state

## Global Contract

Every snapshot must preserve:

- startup database selection context
- current workflow mode
- current result kind and review path
- selected species, release, version, or database-specific context
- original user inputs exactly as entered or normalized for execution
- all generated result rows currently known to the workflow
- all user edits, selections, aliases, labels, grouping decisions, filters, and display state
- all downstream-action prerequisites already generated during the workflow
- all generated caches or blobs required to avoid rebuilding saved work

At review time this includes, explicitly, external-reference settings, filter settings, family/group settings, export settings, transfer/handoff context, and table-state context. They are not optional nice-to-haves; they are part of workflow continuity.

Every snapshot must avoid depending on later reconstruction of:

- search results
- BLAST jobs or BLAST polling
- auto-label decisions
- UniProt enrichment
- InterPro enrichment
- family-group detection or custom grouping decisions
- generated FASTA content when already produced
- generated report-side summaries when already produced and needed for continuity
- explicitly recorded cache payloads or generated files that the workflow needs for later continuity

## Artifact Classes

These artifact classes must be considered during snapshot design for every feature:

- structured workflow state
  - mode, species, release, settings, rows, selections, filters, sort, cursor, scroll, aliases
- generated text artifacts
  - normalized inputs, FASTA text, raw FASTA text, raw BLAST XML text, report source text
- generated non-text artifacts
  - binary caches, database-specific cache payloads, local BLAST support files, downloaded archives, serialized helper blobs
  - explicitly recorded cache payloads and generated files when they contain already-generated workflow continuity state
- transfer state
  - handoff state between tabs, keyword-to-BLAST transfer state, BLAST-row-to-BLAST transfer state
- external-reference state
  - UniProt-enriched fields, InterPro-enriched fields, auto-label ranked aliases, hit-level label decisions
- review-state and filter-state
  - row-selection state, blast-run-selection state, filter applied/cleared flags, per-row filter flags, row numbers, focus positions
- export-state
  - every toggle and parameter shown in export settings
- execution-context state
  - configured request parameters, prepared items, original run count, transfer origin metadata, tab handoff context

## Field-Level Rule

If a feature has a struct, form, prompt result, cache record, or handoff payload that affects later user-visible behavior, that object must be considered a snapshot candidate field-by-field.

The checklist is not complete if it mentions only a feature name but ignores the feature's concrete parameters.

## Database Matrix

Current primary databases and startup entrypoints:

- `phytozome`
- `lemna`
- `ncbi`
- `tair`
- `Explore -> Open session`
- `TAIR database family index`

Every one of these must follow the same completeness contract.

## Mode Matrix

Current user-visible workflow families:

- keyword search and review
- BLAST single-query review
- BLAST multi-query review
- Family BLAST merged review
- TAIR family index search and review
- Canvas review, system-tree preview, and MSA preview
- keyword-to-BLAST transfer
- BLAST-row-to-BLAST transfer
- session open / session reopen
- export flows

## Canvas Tree and MSA Workflows

Must preserve:

- Canvas items, row order, row data, display names, selected checkboxes, and MSA-origin exclusion flags
- tree target mode, display-name source, skip-unselect setting, alignment method/parameters, and tree method/parameters
- last tree payload, run manifest, computation fingerprints, aligned FASTA, Newick, runtime request/response, input metadata, and runtime logs when present
- MSA row states: `green` for rows selected and sent to both tree and MSA, `yellow` for rows unchecked for both tree and MSA because MSA Apply excluded them, and `red` for ordinary unchecked rows
- last shared tree/MSA payload and aligned FASTA plus durable Jalview state such as groups, sequence features, annotations, markers, settings, sequence descriptions, MSA-local saved sequence names, and MSA sequence text edits when available
- MSA coordinate display metadata split into raw/base display name and optional `[canvas,row]` display prefix, so the prefix can be redrawn without becoming Jalview's true sequence name

Must not preserve as `.pgo` workflow state:

- tree panel expanded/focused state, current settings page, scroll offsets, transient search text, open menus/ribbons, hover state, or browser viewport metadata
- UI state that only describes how a menu/dialog was open, unless it changes the tree, MSA, row selection, or exported result

Must not rebuild on open:

- runtime tree artifacts already packed into the snapshot
- MSA yellow/red/green row state
- Jalview groups/features/annotations/settings already captured by the MSA bridge

## Keyword Workflows

Applies to:

- phytozome keyword
- lemna keyword
- ncbi keyword
- tair keyword
- tair family search result review

Must preserve:

- selected database and selected species/release/version
- exact original keyword inputs and their input order
- search classification or search type chosen/performed for each input
- all generated keyword groups
- all generated keyword rows
- all row fields already present after search and post-processing
- all user-edited `label_name`, `labelname_type`, `phgo_alias`, or row-affecting edits
- selected checkboxes
- table cursor, focused row, sort state, offsets, and scroll state
- report/run timing metadata already captured by the workflow
- sequence cache and any fetched sequence payloads already resolved for keyword rows
- any release-backed metadata already materialized into rows
- NCBI source-engine metadata already known for keyword rows, including Entrez database, record type, engine schema, accessions, UIDs, FASTA headers, clean protein sequences, gene locus values, and alias candidate source fields
- any raw downloaded metadata or cache payloads already needed for exact continuity

Must not rebuild on open:

- keyword search results
- symbol name ranking already applied to rows
- release-file parsing already performed into saved rows

## BLAST Input and Execution Workflows

Applies to:

- phytozome BLAST
- lemna BLAST
- tair BLAST
- local BLAST fallback paths
- online-submission paths

Must preserve:

- selected target database and species
- exact original query/input set
- original input count before any merge, skip, or collapse
- normalized sequence/query payload used for execution
- configured BLAST request parameters
  - `Species`
  - `Sequence`
  - `SequenceKind`
  - `TargetType`
  - `Program`
  - `EValue`
  - `ComparisonMatrix`
  - `WordLength`
  - `AlignmentsToShow`
  - `AllowGaps`
  - `FilterQuery`
  - `RequestProfile`
- external-reference settings
  - `AutoLabelBlastHits`
  - `UseUniProt`
  - `UseInterPro`
  - `InterProSettings.UsePfamAccession`
  - `InterProSettings.UseInterProAccession`
  - `InterProSettings.UseSignatureAccession`
  - `InterProSettings.UseEntryType`
  - `InterProSettings.UseEntryName`
  - `InterProSettings.UseCoverage`
  - `InterProSettings.UseMatchRegions`
  - `InterProSettings.PresentMinCoverage`
  - `InterProSettings.PartialMinCoverage`
  - `InterProSettings.PresentMinMatchedItems`
  - `InterProSettings.PartialMinMatchedItems`
  - `MotifChecker.Enabled`
  - `MotifChecker.Motifs`
- family BLAST settings
  - `Enabled`
  - `GroupByDetectedPrefix`
  - `MergeRowsByTarget`
  - `KeepBestHitPerTarget`
  - `PrependOnlyFirstQuery`
  - `CustomizeGroups`
  - `MinimumGroupSize`
  - `StripLeadingSpeciesPrefix`
  - `StripTrailingQueryIndex`
  - `StripAfterNumberSuffix`
  - `NormalizeInnerPunctuation`
  - `StripTerminalSubtypeSuffix`
  - `KeepDistinctQuerySubgroups`
  - `UseUniProtReference`
  - `UseInterProReference`
  - `RankingTieBreakerOrder`
- prepared query items
  - `RawInput`
  - `LabelName`
  - `Sequence`
  - `ProteinSequence`
  - `NucleotideSequence`
  - `QuerySource`
  - `FromKeyword`
  - `FamilyName`
  - `MemberLabel`
  - `FamilyGroupSource`
  - `FamilyDetectionRule`
  - `FamilySources`
  - `FamilySettings`
- query-source metadata
  - `Sequence`
  - `ProteinSequence`
  - `NucleotideSequence`
  - `SequenceKind`
  - `PreferredSequenceID`
  - `OriginalInputURL`
  - `NormalizedURL`
  - `SourceDatabase`
  - `SourceProteomeID`
  - `SourceJBrowseName`
  - `SourceGenomeLabel`
  - `LabelName`
  - `PhgoAliases`
  - `Aliases`
  - `Symbols`
  - `Synonyms`
  - `AutoDefine`
  - `UniProtAccession`
  - `GeneID`
  - `TranscriptID`
  - `ProteinID`
  - `OrganismShort`
  - `Annotation`
- all BLAST runs already generated
- all BLAST result rows already generated
- raw BLAST payloads already captured from source systems
- all post-processing already applied to rows
  - query-context annotations
  - label assignments
  - UniProt fields
  - InterPro fields
  - family merge outputs

Must not rebuild on open:

- BLAST submission
- BLAST result polling
- local BLAST execution
- auto-label hit labeling
- UniProt enrichment
- InterPro enrichment

## BLAST Review Workflows

Applies to:

- single-query BLAST review
- multi-query BLAST review
- Family BLAST merged review

Must preserve:

- whether the review is logically single-query or multi-query
- original query/input count exactly
- visible run list exactly as saved
- original prepared query list
- original run list when needed to preserve multi-input semantics
- per-run selection masks
- per-run filter flags
- filter settings and whether filter was applied or cleared
- row numbering as seen by the user
- alias edits made in review
- current run focus
- current row focus
- table cursor, sort, offset, and scroll state for both single-table and multi-table review
- single-table review state fields
  - `RowSelectionState.Valid`
  - `RowSelectionState.SelectedRow`
  - `RowSelectionState.SelectedColumn`
  - `RowSelectionState.RowOffset`
  - `RowSelectionState.ColumnOffset`
  - `RowSelectionState.Sort`
  - `RowSelectionState.ControlHeaders`
  - `RowSelectionState.HeaderColumn`
- multi-table review state fields
  - `BlastRunSelectionState.Valid`
  - `BlastRunSelectionState.CurrentRun`
  - `BlastRunSelectionState.ControlMode`
  - `BlastRunSelectionState.ListOffset`
  - `BlastRunSelectionState.Sort`
  - `BlastRunSelectionState.HeaderColumn`
  - every `BlastRunTableState` entry:
    - `Valid`
    - `SelectedRow`
    - `SelectedColumn`
    - `RowOffset`
    - `ColumnOffset`
- filter-state fields
  - every `BlastFilterSettings` field
  - `FilterApplied`
  - `FilterCleared`
  - row-level `FilterFlags`
  - run-level `FilterFlagsByRun`

Special rule:

- Family BLAST merging must never rewrite a multi-input workflow into single-input review semantics. If the workflow started with more than one query item, reopened review must still behave as multi-file mode even when the visible merged run list has one table.

## Family BLAST Workflows

Must preserve:

- whether family BLAST was enabled
- detected family groups
- customized family groups
- group source and detection rule text
- member ordering
- member label renames
- member alias lists already stored for grouping
- family-level merge decisions
- family-level ranking/filter settings
- family FASTA prepend behavior
- whether grouped review/export was using first-query-only prepend or all-query prepend
- member-level metadata used by custom grouping dialogs
  - `LabelName`
  - `ProteinID`
  - `Aliases`
  - `OriginalLabelName`
  - `SourceKey`

Must not rebuild on open:

- family auto-detection already confirmed by the user
- custom grouping edits
- stored alias ranking used by grouping

## Keyword-to-BLAST Transfer

Must preserve:

- source database and source species
- selected keyword rows transferred into BLAST
- resolved sequence payloads used for transfer
- source `label_name`
- source `phgo_alias`
- source identifiers and metadata required for later BLAST export/report continuity
- reusable query-source fields prepared during transfer, including sequence,
  source URLs, source database/species IDs, UniProt accession, and
  gene/transcript/protein identifiers

Must not preserve as fresh BLAST label candidates:

- raw keyword source `Aliases`
- raw keyword source `Symbols`
- raw keyword source `Synonyms`
- keyword `AutoDefine` state

Must not rebuild on open:

- sequence resolution already done for transferred rows
- query-source labels already resolved for transferred rows
- target BLAST database choice while the user is still navigating inside the
  transferred BLAST target flow; Back from target species or BLAST execution
  must return to the transfer database choice instead of discarding transfer
  rows

## BLAST-Row-to-BLAST Transfer

Must preserve:

- source database and source species
- selected BLAST rows used for transfer
- resolved sequence payloads prepared from those rows
- hit/source metadata copied into new query items
- any already-generated query-source information required for follow-up BLAST continuity

Must not rebuild on open:

- sequence resolution from selected BLAST rows
- source-label and source-alias material already attached to the transferred query items

## TAIR Family Index Workflow

Must preserve:

- selected TAIR release/version
- selected family candidate or browse path
- hierarchy position or selected family context
- generated family result rows
- row selections
- review state
- sequence caches
- all row fields already materialized from the TAIR release data

Must not rebuild on open:

- family search result generation
- resolved row metadata already present in saved rows

## Export State

If export settings or export-side generated artifacts are already known at snapshot time, preserve:

- base name
- output directory intent if part of workflow continuity
- write toggles
  - `WriteReport`
  - `WriteSession`
  - `WriteText`
  - `WriteExcel`
  - `WriteRawExcel`
- `FastaHeaderMode`
- `UsePhgoHeader`
- `PrependOnlyFirstQuery`
- any report-side context already materialized for the active review state

If generated export artifacts already exist and are needed to avoid rebuilding, preserve:

- generated FASTA payloads
- generated raw FASTA payloads
- generated raw BLAST XML payloads
- generated workbook-side raw/support data
- generated report-support payloads

## Caches and Raw Payloads

When these have already been generated and later workflow continuity would otherwise depend on rebuilding them, snapshots should preserve them directly:

- sequence caches
- raw BLAST XML
- downloaded release metadata already tied to saved rows
- local BLAST helper outputs
- cached external-reference payloads
- cached alias-ranking payloads
- explicitly recorded artifact files and cache payloads needed for workflow continuity
- in-memory runtime caches already populated during the workflow
  - BLAST query/source auto-label caches
  - BLAST hit label-identification caches
  - row-to-UniProt-accession caches
  - UniProt lookup-result caches
  - InterPro lookup-result caches
  - query-source resolution caches
  - keyword-term row caches
  - keyword-row-to-BLAST-item caches
  - species-candidate caches
  - protein-sequence success caches
  - protein-sequence miss caches
- tab handoff state
- tab handoff fields
  - `PendingMode`
  - `TransferKind`
  - `BlastProgramPath`
  - `ReuseLastBlastInput`
  - `ReuseLastBlastRows`
  - `ReuseLastKeywordRows`
  - `RewindBlastToInput`
  - `RewindKeywordToInput`
  - `TransferSourceSpecies`
  - `TransferKeywordRows`
  - `TransferBlastRows`
  - `LastBlastItems`
  - `LastKeywordGroups`
  - `LastKeywordReport`
  - `LastKeywordSpecies`
  - `LastBlastRowContext`
  - `LastBlastReviewContext`

Storage rule:

- if the artifact is already textual and stable, storing text is fine
- if the artifact is already binary or non-text, store it in the original format instead of translating it unless translation is simpler and lossless

## Open Session / Reopen Guarantees

Opening a snapshot must restore enough state that the user can continue:

- review
- alias editing
- filtering
- export
- export all
- follow-up BLAST
- transfer to another database
- detail/FASTA viewing

without any silent rebuild of already-generated workflow outputs.

Open-session restore order should prefer:

- restore explicitly recorded artifact files to their original paths
- then hydrate in-memory snapshot modules and review state
- only if a restore target is missing or unusable should later workflow logic fall back, and that fallback should be explicit rather than silent

## Future Features Rule

Every new feature must answer these questions before its snapshot support is considered complete:

- what exactly has already been generated by the time the user can save a session?
- which of those generated artifacts would be annoying, fragile, or expensive to rebuild?
- what UI state would the user expect to see exactly as before?
- what transfer/export/follow-up actions depend on generated material rather than original input alone?
- what raw cache or blob payload should be preserved in original format instead of translated?

If the answer is "the user would notice if it were recomputed", that state belongs in the snapshot.

## Development Gate

New features are not complete unless snapshot integration is updated in the same change.

That means every new feature must ship with:

- snapshot write-path updates
- snapshot read/hydration updates
- completeness-checklist updates
- `AGENT.md` updates when the user-visible workflow contract changes
- tests proving the new state survives save/open without hidden rebuilding



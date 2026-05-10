# Phygoboost Work Classification Standard

## Purpose

This document defines the official classification standard for all runtime work in `phytozome GO`.

It exists for three reasons:

1. architecture consistency
2. easier future rewrites
3. fast temporary classification when new work appears during development

This document is normative.

When code, comments, plans, or review notes disagree with this document, this document wins unless it is updated first.

## Mandatory companion index

This standard is paired with:

- [Work Inventory](./work-inventory.md)

The inventory is the archive and index of concrete classified work.

This standard defines **how** to classify.

The inventory records **what is currently classified**.

Both documents must be kept in sync.

## Required maintenance rule

Whenever a new work unit is introduced, changed, split, merged, or moved:

1. classify it using this document
2. add or update its entry in `docs/phygoboost/work-inventory.md`
3. if the new case does not fit the current rules cleanly, update this document first
4. then update the inventory entry

This is not optional.

New code is not considered fully integrated until its classification has been archived.

## What counts as a work unit

A work unit is any code path that does one or more of the following:

- runs CPU work
- runs local file work
- launches a subprocess
- performs network I/O
- coordinates worker threads
- coordinates between `main` and `heavy`
- drives user-visible loading or progress behavior
- acts as a thin orchestration wrapper for one of the above

Important:

- thin wrappers still count
- dispatch functions still count
- worker registration and worker bridge functions still count
- helper functions that only shape strings or parse values usually do not leave class `0`
- but helper functions that belong tightly to a threaded, searched, or worker-bound pipeline may inherit a higher class by coupling

## Process model behind the classification

The intended runtime has exactly two processes:

- `main`
- `heavy`

Classification is not only about "what the code does in isolation".

It is also about:

- where that work should live
- what it communicates with
- what pipeline it belongs to
- whether it is UI-heavy
- whether splitting it across processes would make the system worse

## The five classes

### `0`

Direct local synchronous helper logic.

Use `0` only when all of the following are true:

- no multithreading
- no background worker behavior
- no batch orchestration
- no process launch
- no remote I/O
- no worker-bound bridge behavior
- no strong UI loading/progress role
- no search/lookup/species-loading responsibility

Typical `0` work:

- parsing helper
- string normalization
- pure data shaping
- row scoring helper
- local comparison helper
- local formatting helper
- path-token helper that does not perform I/O

Hard exclusion rule:

- if it is multithreaded, it is not `0`
- `0` is outside `phygoboost` resource control
- `0` does not enter any local pool, network pool, or process-pool accounting
- `0` must run directly in the owning process without scheduler negotiation

### `1A`

Main-process local work with strong UI affinity.

Use `1A` when work belongs in `main` because it is strongly tied to the interactive experience or to lightweight main-process local coordination.

Typical `1A` work:

- table loading orchestration
- progress overlay orchestration
- modal flow orchestration
- row-selection rendering support
- UI-facing report assembly
- UI-facing export orchestration
- TUI task wrappers
- local main-process state coordination for visible loading behavior
- lightweight local disk work that is directly part of a UI-facing step

Key idea:

`1A` is not "all local work".

It is local work that should remain in `main` because moving it away would hurt UI coherence or make progress/loading behavior harder to control.

Process rule:

- if work is tightly coupled to visible UI state, table loading, table rendering, progress overlays, modal orchestration, or user-facing recovery flow, it must stay in `main`
- such work belongs to `0` or `1A`, never `2`

### `1B`

Main-process network work with weak coupling to the heavy/search side.

Use `1B` sparingly.

This class only applies when network work is truly better kept in `main` and is not part of a tightly-coupled search, lookup, source-loading, or worker pipeline.

Typical `1B` work:

- a small direct network helper used by `main`
- runtime-level network utility with no heavy/search affinity

Bias rule:

- when in doubt between `1B` and `2B`, prefer `2B`

### `2A`

Heavy-process local work.

Use `2A` for local work that is heavy, threaded, subprocess-based, worker-bound, or strongly coupled to heavy-side orchestration.

Typical `2A` work:

- local BLAST preparation
- local BLAST execution
- `makeblastdb`
- managed tool installation
- archive extraction for heavy pipelines
- worker registration for heavy-bound flows
- subprocess wrappers
- non-UI multithreaded local batching
- heavy-side cache/file transforms
- PDF generation
- Excel generation
- large export artifact creation
- IPC/heavy host/runtime internals

Hard rule:

- non-UI multithreaded local work should generally land in `2A`, not `1A`

### `2B`

Heavy-process network/search/reference/source work.

Use `2B` for remote work that belongs to tightly-coupled source/search/lookup pipelines.

Typical `2B` work:

- keyword search
- wide search
- identifier search
- species loading
- source-side BLAST submission
- BLAST result polling when treated as part of the remote pipeline
- UniProt lookup
- InterPro lookup
- remote source fetches
- worker-side capability detection
- worker-side search orchestration
- search-engine entry points
- source-worker bridge functions

Hard rule:

- search belongs to class `2`

This includes:

- search itself
- search wrappers
- search dispatchers
- search program selectors
- remote search pipeline bridges

## Global hard rules

These rules override local intuition.

### Rule 1: search is class `2`

Anything that is meaningfully part of search, lookup, species loading, remote source discovery, or reference enrichment should be classified as `2A` or `2B`, usually `2B`.

Do not leave search in `1`.

### Rule 2: multithreaded work is never `0`

If a function:

- launches goroutines
- uses controlled parallel loops
- dispatches to workers
- batches background tasks
- orchestrates async result channels

then it cannot be `0`.

### Rule 3: strong communication should stay together

If several work units:

- communicate heavily
- belong to one pipeline
- exchange many intermediate states
- retry together
- cancel together
- share caches closely

then they should be grouped into one process.

Default bias:

- prefer grouping them under `heavy`

### Rule 4: UI-heavy loading stays in `1`

If the main value of a function is:

- loading overlay control
- progress presentation
- table-related load/render coordination
- modal/report/export flow visible to the user
- UI-coupled state reuse or incremental rendering support

then it stays in `main`, usually `1A`, or `0` when it is direct synchronous helper logic.

## Performance preservation rule

Classification and runtime refactors must preserve and strengthen existing performance behavior.

This is a hard maintenance rule.

When moving work between `0`, `1A`, `1B`, `2A`, and `2B`, do not casually delete:

- useful concurrency
- bounded parallel batching
- caching
- singleflight or duplicate suppression
- data reuse
- warm-path prefetch
- chunking tuned for throughput
- shared transport reuse
- cancellation-aware short-circuiting

If an optimization must change because the architecture changed:

- replace it with an equivalent or stronger mechanism
- document the new mechanism in code and inventory updates
- validate that throughput, latency, or stability does not regress silently

then it should usually be `1A`, even if it triggers other work under the hood.

### Rule 5: wrappers inherit the dominant pipeline

A thin wrapper should usually inherit the class of the work it primarily exists to drive.

Example:

- a tiny `PrepareLocalBlast(...)` wrapper is still `2A`
- a tiny `SearchKeywordGroups(...)` wrapper is still `2B`
- a tiny `RunTaskValueContext(...)` wrapper is still `1A`

Do not incorrectly downgrade wrappers just because their own body is short.

## Decision procedure

When classifying a new work unit, apply this order:

1. Is it a pure synchronous helper with no threading, no network, no subprocess, no search role, and no UI loading role?
   - yes: `0`
2. Is it mainly UI-heavy loading/progress/table/render orchestration in `main`?
   - yes: `1A`
3. Is it part of search, lookup, species loading, remote source work, or reference enrichment?
   - yes: usually `2B`
4. Is it local heavy work, subprocess work, worker-bound local orchestration, or non-UI multithreaded local work?
   - yes: `2A`
5. Is it a rare network helper that truly belongs in `main` and is not tightly coupled to heavy/search pipelines?
   - yes: `1B`
6. If still unclear, classify by dominant coupling:
   - UI-dominant: `1A`
   - heavy local dominant: `2A`
   - heavy remote/search dominant: `2B`

## Coupling inheritance rules

Sometimes the function itself is simple, but its classification should be inherited from its pipeline.

Use inheritance when the function exists mainly to:

- enter a heavy-side stage
- bridge into a worker
- wrap a search stage
- wrap a BLAST stage
- wrap a reference-enrichment stage
- expose a UI task boundary for progress/report/export interaction

Inheritance examples:

- BLAST prep wrapper -> `2A`
- search wrapper -> `2B`
- UI loading wrapper -> `1A`

## UI affinity test

Ask these questions:

- if this moves out of `main`, does the visible loading/progress/table experience get harder to control?
- is this function mostly about a user-facing screen, modal, table, report, or export flow?
- does this function exist primarily to mediate visible interaction rather than raw computation?

If yes, it leans toward `1A`.

## Heavy affinity test

Ask these questions:

- does this function launch or coordinate subprocesses?
- does it belong to local BLAST/tool setup/export/PDF generation?
- does it use worker bridges or communicate strongly with heavy-side runtime pieces?
- is it multithreaded but not strongly UI-facing?

If yes, it leans toward `2A`.

## Search affinity test

Ask these questions:

- does this function search?
- does it lookup?
- does it fetch species candidates?
- does it fetch remote source records?
- does it enrich from external scientific references?
- does it select or route search programs?
- is it a search-engine helper that only exists because search exists?

If yes, it leans toward `2B`.

## Inventory archiving requirements

The inventory file must remain:

- exhaustive for current non-test production work
- indexed by file
- line-referenced where possible
- updated when functions are added, removed, renamed, split, or merged

Minimum required update cases:

- new function added
- function deleted
- function moved to another file
- function reclassified
- wrapper split into multiple work units
- a previously pure helper becomes threaded, remote, or UI-heavy

## Review checklist for future changes

Before finishing a change that adds work:

1. Did new code introduce a new work unit?
2. Did any existing `0` helper become threaded or async?
3. Did any main-process work become tightly coupled to heavy/search work?
4. Did any search-related helper remain in `1` by mistake?
5. Did any UI-heavy loading/orchestration helper get pushed into `2` by mistake?
6. Was `docs/phygoboost/work-inventory.md` updated?
7. If the case was new or ambiguous, was this standard updated first?

## Source of truth rule

For classification questions, use these documents in this order:

1. this file
2. `docs/phygoboost/work-inventory.md`
3. `docs/phygoboost/architecture.md`
4. `docs/phygoboost/workflow-contracts.md`

If those documents conflict, update them explicitly instead of letting code drift silently.

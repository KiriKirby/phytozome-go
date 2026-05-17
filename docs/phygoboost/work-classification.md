# Phygoboost Work Classification Standard

## Purpose

This document defines the only valid `phygoboost` work classes in the current architecture.

The process split is gone. The old `1A`, `1B`, `2A`, and `2B` system is retired.

The runtime contract now has only:

- `0`: unmanaged direct work
- `1`: managed work under `phygoboost`

Inside `1`, resource control distinguishes only:

- shared managed execution
- managed per-domain network execution

## Maintenance rule

Whenever a new runtime-controlled work unit is introduced, removed, renamed, merged, split, or moved:

1. classify it with this document
2. update [Work Inventory](./work-inventory.md) in the same change
3. if the current rules do not fit, update this document first

## What counts as a work unit

A work unit is any code path that does one or more of the following:

- runs CPU work
- performs local disk I/O
- launches a subprocess
- performs network I/O
- launches goroutines or controlled parallel loops
- owns a user-visible long-running loading stage
- wraps one of the above as a meaningful execution boundary

Thin wrappers still count when they define a real execution boundary.

## Class `0`

`0` is for only the smallest direct local synchronous helper logic.

Use `0` only when all of the following are true:

- truly tiny direct helper logic
- one synchronous thread of execution
- no goroutines or controlled parallelism
- no subprocess launch
- no remote I/O
- no managed disk stage
- no managed-slot acquisition
- no managed network-slot acquisition
- no long-running user-visible stage boundary

Typical `0` work:

- string normalization
- URL or identifier parsing
- pure data shaping
- small scoring helpers
- row formatting helpers
- static table/header helpers
- cheap synchronous state glue with no resource negotiation
- search-program selection helpers that only inspect already-available input text

Hard rules:

- if it is not tiny direct helper logic, it is not `0`
- if it is multithreaded, it is not `0`
- if it performs network I/O, it is not `0`
- if it launches a subprocess, it is not `0`
- if it should be counted or throttled by `phygoboost`, it is not `0`

## Class `1`

`1` is for every work unit that `phygoboost` must count, throttle, or coordinate.

This includes both local and network work.

If it is not clearly `0`, classify it as `1`.

### `1` shared managed

Use shared managed execution without a domain declaration for work such as:

- bounded CPU batches
- disk-heavy reads or writes
- archive extraction
- BLAST+ installation
- `makeblastdb`
- local BLAST execution
- PDF generation
- Excel generation
- sequence text export generation
- file inspection and hashing stages
- local prefetch or enrichment batches that use controlled parallelism

### `1` network

Use managed per-domain network execution for work such as:

- keyword search
- wide search
- species discovery
- source record fetches
- remote BLAST submit/wait flows
- UniProt lookup
- InterPro lookup
- Phytozome or lemna remote fetches
- release capability detection
- any other remote source/search/reference pipeline

## Global hard rules

### Rule 1: no process distinction inside `1`

Managed work must not be split into old level `1` versus old level `2` semantics.

If two tasks are both managed, they use the same runtime model. The remaining difference is only whether they need local resources or domain network resources.

### Rule 2: search is just managed work

Search, lookup, source fetch, enrichment, submit, and poll logic are no longer a separate class family.

They are `1` network unless the specific step is purely local.

For keyword search specifically:

- query text normalization or search-program selection may stay `0` when they are truly tiny synchronous helpers
- keyword-index loading, cache fill, alias expansion, identifier probing, wide fallback, and any batched or parallel search branch execution are all `1`

### Rule 3: multithreaded work is never `0`

If a function:

- launches goroutines
- uses controlled parallel loops
- runs a managed batch
- coordinates asynchronous results

then it must be `1`.

This rule is absolute. Anything beyond a tiny single-thread direct helper belongs in `1`.

### Rule 4: subprocess work is shared managed work without a domain

Anything that launches or coordinates subprocesses must be `1` and must not invent a fake network domain.

### Rule 5: UI ownership does not create a new class

UI-facing orchestration can still be either:

- `0` when it is direct synchronous helper logic
- `1` when it performs long-running local or network work that must be counted

There is no separate UI process class.

## Decision procedure

When classifying a work unit, use this order:

1. Is it a pure local synchronous helper with no batching, no subprocess, no network, and no managed execution boundary?
   - yes: `0`
2. Does it need `phygoboost` accounting, throttling, or cancellation-aware managed execution?
   - yes: `1`
3. If it is `1`, which resource does it primarily need?
   - shared managed slots only
   - per-domain network slots

## Inventory expectations

The inventory must record the maintained classification of current work families.

It should focus on real managed execution boundaries and the module families that own them, not on preserving removed split-runtime categories.

## Source of truth order

For classification questions, use these documents in this order:

1. this file
2. `docs/phygoboost/work-inventory.md`
3. `docs/phygoboost/architecture.md`
4. `docs/phygoboost/workflow-contracts.md`

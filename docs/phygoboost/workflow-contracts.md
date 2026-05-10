# Workflow Contracts For Phygoboost

## Purpose

This document defines the contract between workflow code and `phygoboost`.

It is critical because the old design blurred the line between:

- workflow planning
- runtime scheduling
- transport

The rewrite restores that boundary.

Work classification is part of that boundary.

For classification rules and archival duties, also read:

- [Work Classification Standard](./work-classification.md)
- [Work Inventory](./work-inventory.md)

## Workflow owns planning

Every workflow owns:

- stage definitions
- stage transitions
- progress totals
- user-visible labels
- retry and recovery semantics

Examples:

- BLAST decides how many query-resolution steps exist
- BLAST decides how many remote submit or wait stages exist
- keyword search decides how many term-search stages exist
- export decides how many file-write stages exist

`phygoboost` must not guess these values.

## Workflow reports resources, not business meaning

When a workflow enters a stage, it requests the resources needed for that stage.

Examples:

- "I need 3 local slots in `main`"
- "I need 5 local slots in `heavy`"
- "I need 2 slots for `rest.uniprot.org`"

It does not say:

- "I am doing BLAST plan phase 2"
- "I am a high-priority external reference lookup"

Those semantics belong to the workflow, not the scheduler.

## No planner inside phygoboost

A dedicated file like the old `blast_plan.go` must not exist inside `phygoboost`.

If BLAST has special counting or batching logic, that logic must live in BLAST workflow code.

This rule exists because:

- business rules change often
- performance systems should stay generic
- hiding planner logic in the runtime makes debugging much harder

## Progress and loading responsibilities

Workflow code owns progress semantics.

That includes:

- total item counts
- completed item counts
- pipeline labels
- multi-line loading overlays
- stage-specific messages

`phygoboost` may expose observability data, but it does not render or define progress UI.

This boundary is important for fixing progress bugs correctly. Broken counts are usually a workflow accounting issue, not a scheduler issue.

## Execution-level choice

Workflow code must explicitly choose where work runs:

- `ExecInline`
- `ExecMain`
- `ExecHeavy`

The runtime must not silently rewrite this choice behind the workflow's back.

If a fallback path is needed, the workflow defines that fallback.

That execution-level choice must also match the documented work classification.

If a workflow step changes from UI-local to heavy-local, or from main-network to heavy-network, both the code and the classification documents must be updated together.

## Domain declaration

Workflow code must declare the actual remote domain being used.

Bad:

- "network work"

Good:

- `blast.ncbi.nlm.nih.gov`
- `rest.uniprot.org`
- `www.ebi.ac.uk`
- `phytozome-next.jgi.doe.gov`

This is necessary for correct pool isolation.

## Stage-based resource use

Workflows should request resources at stage boundaries, not for every tiny step.

Good pattern:

1. enter stage
2. acquire needed local or domain slots
3. do the batch or chunk of work
4. release
5. advance stage

Bad pattern:

- constantly re-reporting desired thread counts every few milliseconds
- treating the scheduler like a chatty telemetry sink

## Cancellation contract

Every long-running workflow path must:

- accept a cancellable context
- stop promptly when cancelled
- release grants
- return to the correct UI recovery target

The contract is incomplete if a workflow acquires resources but does not reliably release them on cancellation.

## Retry contract

Workflow code decides:

- whether a failed stage can retry
- whether retry happens per item or for the whole stage
- whether partial results are preserved

`phygoboost` may provide transport or HTTP helpers, but it does not decide the workflow retry model.

## Testing expectations

Workflow tests should verify:

- correct domain declaration
- correct execution-level choice
- release on success
- release on failure
- release on cancellation
- progress totals derived from workflow counts, not runtime guesses

`phygoboost` tests should verify scheduling correctness separately.

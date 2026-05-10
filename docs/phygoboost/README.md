# Phygoboost Design Index

This directory is the source of truth for the `phygoboost` rewrite.

`phygoboost` replaces the old split between:

- `internal/perf`
- `internal/procworker`
- `internal/pipeline`

The rewrite is not a compatibility layer and not a thin rename. It is a full redesign of runtime control, worker execution, and network scheduling for `phytozome GO`.

## Goals

- Keep the application responsive under heavy local and remote work.
- Reduce architecture complexity instead of adding more adaptive branches.
- Move from many loosely controlled worker processes to exactly two processes:
  - `main`
  - `heavy`
- Make resource control explicit, observable, and easy to reason about.
- Remove business-specific planning from the performance system.
- Replace the old global network bucket with per-domain network pools.

## Non-goals

- Do not preserve old `perf` semantics for compatibility.
- Do not keep BLAST-specific planning logic inside the runtime controller.
- Do not maintain a general "network work kind" bucket as the main network abstraction.
- Do not keep historical tuning branches whose behavior is hard to predict or explain.

## Document map

- [Work Classification Standard](./work-classification.md)
  The official rulebook for classifying every work unit into `0`, `1A`, `1B`, `2A`, or `2B`, including maintenance rules for future additions.
- [Work Inventory](./work-inventory.md)
  The indexed archive of current classified non-test work units. New work must be added here when introduced.
- [Architecture](./architecture.md)
  Overall system structure, process model, module boundaries, and source-of-truth rules.
- [Scheduler](./scheduler.md)
  Shared total-pool control, local slot allocation, fairness model, and lifecycle rules.
- [Network Pools](./network-pools.md)
  Per-domain concurrency, HTTP client ownership, rate adaptation, and failure handling.
- [IPC And Heavy Process](./ipc-heavy.md)
  `main` to `heavy` communication, task execution, cancellation, and host lifecycle.
- [Workflow Contracts](./workflow-contracts.md)
  What workflow code must do, what it must stop doing, and how it reports work.
- [Rewrite Plan](./rewrite-plan.md)
  Deletion targets, replacement mapping, code layout, and rollout structure for the rewrite.

## Core statement

`phygoboost` is a runtime coordination system, not a workflow planner.

It also owns the official work-classification vocabulary used by the rewrite:

- `0`
- `1A`
- `1B`
- `2A`
- `2B`

Classification policy lives in:

- `docs/phygoboost/work-classification.md`

Classification archive lives in:

- `docs/phygoboost/work-inventory.md`

Any new runtime, workflow, source, UI-loading, search, worker, export, or report work must be classified and added to the inventory as part of the same change.

It owns:

- total local resource limits
- local slot grants
- per-domain network slot grants
- `main` and `heavy` process coordination
- HTTP transport policy
- cancellation propagation
- runtime observation

It does not own:

- BLAST stage counting
- keyword-stage planning
- table progress semantics
- export business logic
- biological workflow decisions

Each workflow owns its own stage model and progress accounting. `phygoboost` only grants and reclaims execution resources.

# Phygoboost Design Index

This directory is the source of truth for the current `phygoboost` design.

`phygoboost` is the current runtime-control system.

The current design is deliberately smaller:

- one program process
- no extra runtime layer
- no `2`-level work class

## Goals

- Keep the application responsive under large local and remote workloads.
- Keep execution control explainable.
- Use one managed execution model for all runtime-controlled work.
- Distinguish only between shared managed work and per-domain network work.
- Remove process-specific architecture and budgeting.

## Non-goals

- Do not keep BLAST-specific planning logic inside the runtime controller.
- Do not reintroduce a generic global network bucket as the main abstraction.
- Do not reintroduce removed split-runtime structure.

## Document map

- [Work Classification Standard](./work-classification.md)
  The official rulebook for classifying work into `0` or `1`.
- [Work Inventory](./work-inventory.md)
  The indexed archive of current classified non-test work units.
- [Architecture](./architecture.md)
  Runtime structure, module boundaries, and execution model.
- [Scheduler](./scheduler.md)
  Shared managed total-slot control, fairness, and lifecycle rules.
- [Network Pools](./network-pools.md)
  Per-domain concurrency, HTTP client ownership, rate adaptation, and failure handling.
- [Workflow Contracts](./workflow-contracts.md)
  What workflow code must do, what it must stop doing, and how it reports work.
- [Rewrite Plan](./rewrite-plan.md)
  What was deleted, what remains, and what future cleanup should follow the simplification.

## Core statement

`phygoboost` is a runtime coordination system, not a workflow planner.

It owns the official work-classification vocabulary:

- `0`: unmanaged direct work
- `1`: managed work under `phygoboost`

Inside `1`, resource control only distinguishes:

- shared managed execution
- per-domain network execution

It owns:

- shared managed total-capacity limits
- shared managed total-slot grants
- per-domain network slot grants
- shared HTTP transport policy
- cancellation propagation
- runtime observation

It does not own:

- BLAST stage counting
- keyword-stage planning
- table progress semantics
- export business logic
- biological workflow decisions

Workflow code must submit complete `1`-level stages and let `phygoboost` decide actual concurrency through the shared managed total pool, plus per-domain network pool control for network work.

The current control model is intentionally compact:

- one shared managed total pool derived from active CPU, UI reserve, memory pressure, and whole-system CPU saturation
- one independent domain pool per remote origin
- no second cross-domain network cap
- real HTTP latency, `429`, `5xx`, and `Retry-After` signals feed back into the matching domain pool

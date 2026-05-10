# Phygoboost Rewrite Plan

## Purpose

This document records the intended full rewrite target for the runtime system.

The rewrite is allowed to delete and replace old subsystems directly. It is not constrained by compatibility with the old `perf`, `procworker`, or `pipeline` package boundaries.

## Packages to replace

The following legacy areas are replaced by `internal/phygoboost/`:

- `internal/perf`
- `internal/procworker`
- `internal/pipeline`

## Deletion intent

### Delete as concepts

- old global network-worker budgeting as the primary network abstraction
- workflow-specific planner logic inside the runtime system
- sprawling process-pool heuristics
- policy logic split across `perf` and `procworker`
- hidden worker-environment tuning as a major control plane

### Delete as files or directories after rewrite landing

- `internal/perf/blast_plan.go`
- `internal/pipeline/process.go`
- old `internal/procworker` transport and pool management code
- large sections of the old `internal/perf/perf.go` that mixed unrelated responsibilities

## New package responsibilities

### `internal/phygoboost/core`

Replaces:

- worker-budget control logic from old `perf`
- runtime pressure decisions
- central state tracking

### `internal/phygoboost/local`

Replaces:

- general local parallel helpers
- slot permits
- local execution wrappers

### `internal/phygoboost/network`

Replaces:

- old shared network worker budgeting
- old shared HTTP transport policy
- old global rate-limit logic

### `internal/phygoboost/ipc`

Replaces:

- process transport portions of `procworker`
- ad-hoc worker startup message paths

### `internal/phygoboost/heavy`

Replaces:

- process-hosting portions of `procworker`
- `pipeline/process.go` execution host role

## Rewrite constraints

These are the architectural constraints that should survive implementation details:

1. two-process model only
2. no business planner in runtime control
3. per-domain network pools
4. shared local total pool across `main` and `heavy`
5. FIFO fairness
6. explicit acquire and release lifecycles
7. workflow-owned progress counts
8. small IPC surface

## Code layout target

```text
internal/phygoboost/
  api.go
  types.go
  core/
  local/
  network/
  ipc/
  heavy/
  observe/
```

## Main risks to watch during rewrite

- accidentally re-introducing workflow semantics into scheduler code
- accidentally creating another global network bucket under a different name
- routing too much short work into `heavy`
- failing to release grants on cancellation
- making domain identification too loose or inconsistent
- allowing ad-hoc HTTP clients back into the codebase

## Review checklist

Every rewrite review should ask:

- Does this code belong to workflow or to runtime control?
- Is a domain explicitly identified?
- Is work running in the correct process?
- Is acquire and release explicit?
- Is progress being counted by workflow code instead of inferred by the runtime?
- Is this adding hidden policy to IPC?

## End state

When the rewrite is complete:

- `phygoboost` is the only runtime-control subsystem
- the old performance stack is gone
- `main` stays responsive under long work
- remote sources no longer interfere through one coarse network bucket
- workflow code becomes easier to reason about because planning and scheduling are no longer mixed

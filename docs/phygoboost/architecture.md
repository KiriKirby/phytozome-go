# Phygoboost Architecture

## Purpose

`phygoboost` is the runtime execution and resource-control subsystem for `phytozome GO`.

It replaces the old architecture where:

- `perf` mixed together runtime tuning, network policy, worker counting, and some workflow-shaped logic
- `procworker` mixed together process transport and execution policy
- `pipeline` added another execution layer that no longer justifies its complexity

The rewrite separates concerns sharply:

- workflow packages define work
- `phygoboost` controls execution resources
- `heavy` executes heavyweight tasks outside the UI process

## Process model

The program has exactly two processes.

### `main`

Responsibilities:

- all TUI interaction
- workflow orchestration
- lightweight local work
- short or interactive remote work
- user-facing progress state
- owning the central `phygoboost` core

### `heavy`

Responsibilities:

- heavy local computation
- long-running batch work
- local BLAST database preparation
- local BLAST execution
- expensive parsing or bulk file transforms when they would hurt UI responsiveness in `main`

The `heavy` process is not a second application mode. It is an execution host managed by `main`.

## Boundary rules

### Workflow code

Workflow code may:

- define stages
- request local slots
- request network slots by domain
- dispatch a task to `heavy`
- report user-facing progress

Workflow code must not:

- calculate machine-wide worker budgets
- create ad-hoc HTTP clients
- create hidden process trees outside `phygoboost`
- invent its own retry scheduler
- treat all network work as one shared bucket

### `phygoboost`

`phygoboost` may:

- measure runtime pressure
- set safe total local budgets
- grant or deny slot requests
- maintain domain pools
- own shared HTTP transports
- coordinate `main` and `heavy`
- propagate cancellation

`phygoboost` must not:

- know what a BLAST phase means
- plan workflow stage counts
- infer export semantics
- guess progress totals from business data

## Module layout

The end-state package structure is:

```text
internal/phygoboost/
  api.go
  types.go

  core/
    core.go
    scheduler.go
    state.go
    runtime.go
    guardrail.go

  local/
    executor.go

  network/
    manager.go

  ipc/
    bus.go
    message.go
    codec.go
    cancel.go

  heavy/
    host.go
    client.go
    registry.go
    worker_main.go

  observe/
    snapshot.go
    metrics.go
    trace.go
```

The current repository is in the middle of that rewrite and already uses a flattened top-level runtime with the same ownership split:

```text
internal/phygoboost/
  api.go
  types.go
  budgets.go
  observation.go
  env.go
  runtime.go
  runtime_state.go
  network_context.go
  http_client.go
  process.go
  procworker.go
  core_parallel.go
  worker_parallel.go
  heavy_host_runtime.go
  profile.go

  core/
  network/
```

Files such as the old `perf.go`, `permit.go`, and `blast_plan.go` are no longer part of the architecture and must not be referenced as active design units.

## Ownership map

### `api.go`

Public entry points used by workflow packages. This file exposes the runtime contract in simple terms:

- acquire local resources
- acquire network resources
- run controlled local parallel work
- dispatch a heavy task
- get domain-specific HTTP clients

### `core/`

The central scheduler and runtime-state owner.

This layer is the only place that may decide:

- total local capacity
- how many local slots are currently grantable
- whether a request must wait
- whether current runtime pressure requires shrinking grants

### `network/`

All domain-specific remote execution control lives here.

This layer owns:

- per-domain slot pools
- per-domain backoff
- per-domain latency feedback
- shared domain HTTP client construction

It does not own a single global network bucket. Domain pools are independent peers, constrained only by the global local-resource safety budget.

### `ipc/`

The inter-process transport layer between `main` and `heavy`.

This layer must stay small. It is not a planner and not a scheduler. It serializes requests, results, cancellations, and limited runtime-control messages.

Important runtime rule:

- when `main` has already declared domain grants for a heavy-bound task, the active grant snapshot is propagated over IPC and rebound onto the heavy task context
- this prevents the same workflow-stage network reservation from being charged twice just because execution crossed the `main`/`heavy` boundary
- the heavy worker still performs real per-domain control for any new network work it starts itself

### `heavy/`

The execution shell for the `heavy` process.

This layer owns:

- host startup
- host liveness
- task registry
- task execution loop
- result return

It does not own global scheduling policy.

### `observe/`

Runtime observation only.

This layer provides measurements, not policy. It feeds `core/runtime.go` but does not make final decisions.

## Resource model

There are only two scheduling dimensions:

1. local execution slots
2. per-domain network slots

There is no generic priority matrix, no workflow-specific scheduler tree, and no hidden cross-package worker counting scheme.

### Local execution slots

Local slots cover:

- CPU work
- blocking file work
- subprocess-hosted local work

The total local slot budget is computed from current machine pressure and bounded by safety guardrails.

### Per-domain network slots

Every network origin gets its own pool, for example:

- `blast.ncbi.nlm.nih.gov`
- `rest.uniprot.org`
- `www.ebi.ac.uk`
- `phytozome-next.jgi.doe.gov`
- a `lemna.org` origin when applicable

Each domain pool adapts based on its own behavior. One slow or rate-limited source must not directly collapse another healthy source.

## Lifecycle model

All granted resources follow a strict lifecycle:

1. request
2. grant
3. use
4. release

If a task is cancelled or errors early, the same release rule applies.

The architecture must make leaks hard to write and easy to detect.

## Why the old design must be removed

The old system became hard to trust because:

- network control was too global and too opaque
- business planning leaked into runtime control
- too many tasks were routed through process boundaries without enough payoff
- the code grew multiple adaptive branches that were hard to explain during debugging

The rewrite rejects "smart-looking but difficult to predict" behavior. The new design aims for directness, transparency, and stable responsiveness.

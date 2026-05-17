# Phygoboost Architecture

## Purpose

`phygoboost` is the runtime execution and resource-control subsystem for `phytozome GO`.

The current architecture is intentionally single-process.

- all workflow code runs in the main program process
- there is no second runtime process
- there is no runtime bridge

## Execution model

`phygoboost` recognizes only two execution classes:

- `0`: unmanaged direct work
- `1`: managed work under runtime control

Within managed work, only two resource dimensions matter:

1. shared managed total slots
2. per-domain network slots for network work

There is no longer any process-class distinction inside managed work. Old `1` and old `2` work now run through the same implementation model.

## Boundary rules

### Workflow code

Workflow code may:

- define stages
- request shared managed slots
- request managed network slots by domain
- run subprocesses through managed execution
- report user-facing progress

Workflow code must not:

- calculate machine-wide worker budgets
- calculate stage thread budgets
- calculate stage chunk budgets for throttling
- create ad-hoc HTTP clients
- create hidden process trees outside `phygoboost`
- invent its own retry scheduler
- treat all network work as one shared bucket

### `phygoboost`

`phygoboost` may:

- measure runtime pressure
- set safe total managed budgets
- grant or deny managed work requests
- maintain per-domain pools
- own shared HTTP transports
- propagate cancellation

`phygoboost` must not:

- know what a BLAST phase means
- plan workflow stage counts
- infer export semantics
- guess progress totals from business data

## Module layout

The effective package shape is:

```text
internal/phygoboost/
  api.go
  types.go
  budgets.go
  runtime.go
  runtime_helpers.go
  observation.go
  network_context.go
  http_client.go
  process.go
  core_parallel.go
  profile.go

  core/
  network/
  local/
  observe/
```

Deleted from the architecture are the removed split-runtime files.

## Ownership map

### `api.go`

Public entry points used by workflow packages:

- acquire local resources
- acquire network resources
- run controlled local parallel work
- get domain-specific HTTP clients

### `core/`

The central managed scheduler and runtime-state owner.

This layer is the only place that may decide:

- total managed capacity
- how many managed slots are currently grantable
- whether a request must wait
- whether runtime pressure requires shrinking future grants

The current shared-capacity model is intentionally compact:

- start from active CPU count
- reserve a small UI share when needed
- shrink under memory pressure
- shrink again under near-saturated whole-system CPU usage

### `network/`

All domain-specific remote execution control lives here.

This layer owns:

- per-domain slot pools
- per-domain backoff
- per-domain latency feedback
- shared domain HTTP client construction

The domain controller is single-layered:

- there is no second cross-domain network scheduler
- each domain starts small, grows slowly on healthy traffic, and cuts back quickly on overload
- `Retry-After`, `429`, `5xx`, transport failure, and bad latency all belong to the same domain-local feedback path

It does not own a single global network bucket.

### `observe/`

Runtime observation only.

This layer provides measurements, not workflow policy.

## Resource model

### Shared managed total execution

Every `1`-level task first consumes shared managed total slots.

That includes:

- CPU work
- blocking file work
- network work
- subprocess-hosted local work

All managed work now competes in the same total pool before any network-specific per-domain control is applied.

### Per-domain network execution

Every network origin gets its own pool, for example:

- `blast.ncbi.nlm.nih.gov`
- `rest.uniprot.org`
- `www.ebi.ac.uk`
- `phytozome-next.jgi.doe.gov`
- `www.lemna.org`

Each domain pool adapts based on that domain's own behavior, but network work still consumes shared managed total slots first.

The domain pool limit is also capped by the shared managed total capacity, so the application still has one real global ceiling even though domains do not throttle each other directly.

## Lifecycle model

All managed resources follow a strict lifecycle:

1. request
2. grant
3. use
4. release

If a task is cancelled or errors early, the same release rule applies.

## Why the old design was removed

The removed design became hard to trust because:

- it split one runtime into multiple execution classes that behaved differently
- transport-layer duplication repeated control concepts
- network control became harder to reason about when mixed with process routing
- debugging required understanding both resource control and transport behavior

The current design keeps the same performance intentions while removing the process boundary entirely.

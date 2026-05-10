# Phygoboost Network Pools

## Purpose

This document defines the new network model for `phygoboost`.

The old model used a broad "network work kind" budget. That is no longer acceptable as the primary abstraction because it causes unrelated remote sources to interfere with each other and makes debugging too coarse.

The new rule is:

- every network origin has its own pool
- there is no single global network bucket used as the central scheduling abstraction
- total network activity is still bounded by the shared runtime safety budget

## Domain pool model

Each remote origin is represented by a `DomainPool`.

Illustrative structure:

```go
type DomainPool struct {
    Domain            string
    Limit             int
    Active            int
    Waiting           int
    CooldownUntil     time.Time
    LastLatency       time.Duration
    SuccessWindow     int
    FailureWindow     int
    RateLimitedWindow int
}
```

The exact fields may differ, but the semantics must remain:

- active count
- waiting count
- current limit
- cooldown state
- recent health signals

## Domain identity

Pools are keyed by remote origin, not by workflow name.

Examples:

- `blast.ncbi.nlm.nih.gov`
- `rest.uniprot.org`
- `www.ebi.ac.uk`
- `phytozome-next.jgi.doe.gov`
- a specific `lemna.org` origin

Different workflows that hit the same origin share the same pool. This is necessary to avoid one workflow unknowingly spamming the same site that another workflow is already using.

## Why no single network bucket

A single network bucket is too blunt because:

- different sites have different tolerance and rate limits
- one bad site should not automatically throttle all others
- latency and failure signals need to stay attached to their actual source
- debugging "the network is slow" is useless if the program cannot identify which remote origin is the problem

## Pool lifecycle

For a domain request:

1. workflow asks for `N` slots for domain `D`
2. manager locates or creates pool `D`
3. pool decides whether to grant immediately or queue
4. workflow uses the slots
5. workflow releases the slots
6. pool updates health feedback after the work unit finishes

## Slot limits

Each domain pool has its own independent limit.

That limit should adapt using:

- latency trend
- timeout rate
- transport error rate
- HTTP `429`
- server-side `5xx`
- success recovery after cooldown

The adaptation should be conservative and explainable.

Good defaults:

- rise slowly on healthy traffic
- fall quickly on `429`, transport failure bursts, or repeated timeouts
- temporarily cool down when the source clearly asks for relief

## Total safety bound

Although there is no global network bucket abstraction, all network work still consumes real machine resources:

- sockets
- memory
- local CPU
- timer churn

So domain limits must also respect an application-wide safety bound maintained by `core`.

This is a safety ceiling, not a global network scheduler policy.

## HTTP client ownership

All production network calls must use `phygoboost`-owned HTTP clients.

Rules:

- no `http.DefaultClient`
- no ad-hoc per-call `http.Client`
- no workflow-managed transport tuning

The owning API should be something like:

```go
func HTTPClientForDomain(domain string) *http.Client
```

The client is responsible for:

- shared transport reuse
- keep-alive policy
- request timeout defaults
- retry policy where explicitly allowed
- domain-aware rate adaptation hooks

## Feedback model

Feedback must be attached to the actual domain pool that observed it.

Useful signals:

- request latency
- context deadline exceeded
- connect/reset errors
- HTTP `429`
- HTTP `5xx`
- explicit `Retry-After`

Bad signals should reduce that domain's concurrency. Healthy sustained signals may gradually allow small increases.

## Cooldown handling

When a server indicates overload, the domain pool may enter cooldown.

During cooldown:

- new grants may pause
- waiting requests remain queued
- the pool reopens after the cooldown expires or after a conservative probe succeeds

Cooldown is local to the domain. It must not freeze unrelated domains.

## Workflow responsibilities

Workflows must identify the actual domain they are about to hit. They must not just say "this is network work."

For example:

- NCBI BLAST submit and poll should both target the NCBI BLAST domain pool
- UniProt accession fetches should use the UniProt pool
- InterPro lookups should use the EBI pool

## What this system is not

- not a workflow planner
- not a general-purpose web crawler
- not a cross-domain fairness engine
- not a progress tracker

It is a per-domain remote resource controller.

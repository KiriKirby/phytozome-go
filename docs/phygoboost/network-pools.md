# Phygoboost Network Pools

## Purpose

This document defines the new network model for `phygoboost`.

The old model used a broad "network work kind" budget. That is no longer acceptable as the primary abstraction because it causes unrelated remote sources to interfere with each other and makes debugging too coarse.

The new rule is:

- every network origin has its own pool
- there is no single global network bucket used as the central scheduling abstraction
- every managed network action still consumes the shared managed total pool first
- there is no second shared network limit across different domains

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

1. workflow asks for managed work
2. shared managed total pool grants or queues it
3. if the work is network-bound, the domain manager locates or creates pool `D`
4. pool `D` grants or queues the network side
5. workflow uses the slots
6. workflow releases the slots
7. pool updates health feedback after the work unit finishes

## Slot limits

Each domain pool has its own independent limit.

That limit now adapts from a single domain-local control loop using:

- latency trend
- timeout rate
- transport error rate
- HTTP `429`
- server-side `5xx`
- explicit `Retry-After`
- healthy completion recovery after cooldown

The adaptation is intentionally simple and explainable.

Current policy:

- start every domain at `1`
- never exceed the current shared managed total pool
- increase by `+1` only after several healthy completions
- allow growth only when recent latency is still healthy
- cut quickly on `429`, `5xx`, transport failure bursts, or obviously bad latency
- honor `Retry-After` when the server provides it, otherwise use a short local cooldown

This is a domain-local additive-increase / multiplicative-decrease style controller, not a second layered scheduler.

## Relationship To The Shared Total Pool

Different domains do not constrain each other through a second network-wide limit.

The only shared cross-domain resource control is the managed total pool that every `1`-level action already consumes before network work starts.

After that, each domain pool is independent and adjusts only from that domain's own latency, failures, `429`, `5xx`, `Retry-After`, and cooldown state.

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
- observing real request latency and HTTP overload signals
- feeding those signals back into the matching domain pool

## Feedback model

Feedback must be attached to the actual domain pool that observed it.

Useful signals:

- request latency
- context deadline exceeded
- connect/reset errors
- HTTP `429`
- HTTP `5xx`
- explicit `Retry-After`

Bad signals reduce only that domain's concurrency. Healthy sustained signals may gradually allow small increases.

Important implementation rule:

- domain feedback must be recorded from the actual HTTP result path, not only from stage-level acquire/release bookkeeping
- when a workflow already holds a domain grant before issuing the request, the shared transport must still record latency and overload signals for that same domain

## Cooldown handling

When a server indicates overload, the domain pool may enter cooldown.

During cooldown:

- new grants may pause
- waiting requests remain queued
- the pool reopens after the cooldown expires and healthy requests can slowly rebuild the limit

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

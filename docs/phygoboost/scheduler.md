# Phygoboost Scheduler

## Scope

This document defines how `phygoboost` grants and reclaims managed execution capacity.

It covers:

- shared managed total-slot budgeting
- fairness
- lifecycle rules
- waiting and wake-up behavior
- runtime resizing

It does not cover:

- workflow stage planning
- UI progress bars
- report generation logic

## Scheduling philosophy

The scheduler is intentionally simple.

Rules:

- first come, first served
- no priority classes
- no workflow-type privilege
- grant only what currently fits
- release immediately on finish, failure, or cancellation

## Shared managed total pool

The whole application shares one managed execution pool.

All shared managed and managed network actions contend for this same total pool in arrival order.

Network work may also need a second grant from its per-domain pool, but it does not skip the shared total pool.

### Inputs

The scheduler uses a deliberately small capacity function.

Current inputs are:

- active CPU count
- UI CPU reserve
- global memory-pressure level
- current whole-system CPU usage

It does not use workflow-type heuristics, task-family weights, or per-workflow feedback loops.

### Guardrails

The scheduler enforces safety bounds derived from machine limits, such as:

- minimum slot floor
- maximum slot ceiling from active CPU count
- emergency shrink under high or critical memory pressure
- additional shrink under near-saturated system CPU use

### Current capacity rule

The current shared managed total capacity follows this model:

1. start from active CPU count
2. reserve a small UI share when the machine is busy enough that the interface would otherwise stutter
3. clamp to at least one managed slot
4. shrink future grants when memory pressure is `moderate`, `high`, or `critical`
5. shrink future grants again when whole-system CPU usage is already very high

This produces one runtime-sized `ManagedTotal` value.

That single value is the ceiling for:

- shared managed local work
- subprocess-hosted work
- network work before domain-specific gating
- helper pool sizing such as shared HTTP idle-connection defaults

## Request model

Workflow code makes explicit requests when entering a stage:

- acquire `N` shared managed slots
- acquire `N` network slots for a domain

The scheduler reacts to meaningful lifecycle events, not noisy micro-updates.

## Fairness

Fairness is simple FIFO within each resource queue.

- shared managed-slot requests wait in arrival order
- each domain pool waits in arrival order

## Resizing behavior

The runtime may resize the shared managed pool while work is already running.

Rules:

- never revoke an already granted slot in the middle of a unit of work
- shrinking only affects future grants
- growth wakes queued requests in FIFO order
- all managed work classes see the same resized total pool because there is no longer a separate managed subpool family

## Cancellation

When a waiting request is cancelled:

- remove it from the queue
- do not grant it later

When a running task is cancelled:

- the task code must stop
- the held grant must be released

## Slot ownership

Every grant has an owner token recording:

- request id
- amount
- resource type
- acquisition time

This makes leak detection possible and keeps accounting honest.

## API shape

The shared managed scheduling API is conceptually:

```go
type ExecLevel int

const (
    ExecUnmanaged ExecLevel = iota
    ExecManaged
)

type ManagedGrant struct {
    ID       string
    Level    ExecLevel
    Slots    int
    Acquired time.Time
}

func AcquireManaged(ctx context.Context, level ExecLevel, slots int) (*ManagedGrant, error)
func ReleaseManaged(grant *ManagedGrant)
```

## What the scheduler must not do

- It must not inspect BLAST task structure.
- It must not guess query counts.
- It must not calculate progress totals for tables or overlays.
- It must not own report-stage sequencing.

The scheduler is a resource arbiter, not a business-logic planner.

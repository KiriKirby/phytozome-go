# Phygoboost Scheduler

## Scope

This document defines how `phygoboost` grants and reclaims execution capacity.

It only covers:

- shared local slot budgeting
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
- no "external tool" versus "internal tool" split
- grant only what currently fits
- release immediately on finish, failure, or cancellation

This simplicity is deliberate. Complexity in resource schedulers becomes a maintenance burden very quickly and is difficult to validate under all workflow combinations.

## Shared total local pool

The whole application shares one local execution pool.

This pool spans both processes:

- `main`
- `heavy`

The total local pool is not a static machine profile. It is determined from runtime conditions, with hard safety guardrails.

### Inputs

The scheduler may consider:

- runnable CPU pressure
- recent local task queue depth
- memory pressure
- heavy process health
- sustained blocking behavior

### Guardrails

The scheduler must still enforce safety bounds derived from machine limits, such as:

- minimum slot floor
- maximum slot ceiling
- emergency shrink under severe memory pressure

Static hardware information is a guardrail, not the main policy engine.

## Request model

Workflow code does not continuously stream "desired thread counts."

Instead, workflow code makes explicit requests when entering a stage:

- acquire `N` local slots in `main`
- acquire `N` local slots in `heavy`
- acquire `N` network slots for a domain

This is important. The scheduler should react to meaningful lifecycle events, not noisy micro-updates.

## Grant model

Each request is one of:

- granted immediately
- queued until capacity is available
- rejected only if the request is invalid or the context is already cancelled

Partial grant behavior should be explicit in the API. A request either:

- asks for exactly `N`
- or asks for "up to `N`"

The default should prefer exactness so workflow behavior is predictable.

## Fairness

Fairness is simple FIFO within each resource queue.

This means:

- local slot requests wait in arrival order
- each domain pool waits in arrival order
- no special-case bypass for specific workflows

The design favors predictability over aggressive optimization tricks.

## Local request queues

There are two local execution destinations:

- `main`
- `heavy`

But both draw from the same total local capacity.

Example:

- total local capacity currently allows `8`
- `main` holds `3`
- `heavy` holds `4`
- only `1` additional local slot is currently free

The scheduler therefore reasons about:

- global remaining capacity
- destination-specific outstanding requests

It does not create two unrelated pools that can oversubscribe the machine independently.

## Resizing behavior

The runtime may resize the total local pool while work is already running.

Rules:

- never revoke an already granted slot in the middle of a unit of work
- shrinking only affects future grants
- growth wakes queued requests in FIFO order

This avoids chaotic behavior where tasks are constantly being interrupted by scheduler churn.

## Cancellation

Cancellation must be cheap and decisive.

When a waiting request is cancelled:

- remove it from the queue
- do not grant it later
- wake the next eligible waiter if relevant

When a running task is cancelled:

- the task code must stop
- the held grant must be released

The scheduler should assume cancellations are normal, especially in TUI-driven flows where the user may back out of a long action.

## Slot ownership

Every grant must have an owner token.

The token records:

- request id
- process destination
- amount
- resource type
- acquisition time
- release state

This makes leak detection possible and keeps accounting honest.

## Leak detection

The scheduler should be able to answer:

- who currently holds local slots
- for how long
- which domain pools are occupied
- which requests are waiting

Leak detection is a first-class requirement because the program has a history of "stuck but still running" behavior.

## API shape

The local scheduling API should look roughly like this:

```go
type ExecLevel int

const (
    ExecInline ExecLevel = iota
    ExecMain
    ExecHeavy
)

type LocalGrant struct {
    ID        string
    Level     ExecLevel
    Slots     int
    Acquired  time.Time
}

func AcquireLocal(ctx context.Context, level ExecLevel, slots int) (*LocalGrant, error)
func ReleaseLocal(grant *LocalGrant)
```

## What the scheduler must not do

- It must not inspect BLAST task structure.
- It must not guess query counts.
- It must not calculate progress totals for tables or overlays.
- It must not own report-stage sequencing.
- It must not silently route work to another destination without the workflow explicitly choosing that destination.

The scheduler is a resource arbiter. It is not an execution strategist for business logic.

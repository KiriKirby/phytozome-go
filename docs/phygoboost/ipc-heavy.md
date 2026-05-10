# IPC And Heavy Process Design

## Purpose

This document defines how `main` and `heavy` communicate under `phygoboost`.

The goal is to keep this part narrow and robust.

`procworker` in the old design accumulated too much responsibility. In the rewrite, IPC is only transport and lifecycle plumbing. Scheduling policy belongs to `core`, and workflow semantics belong to workflow packages.

## Heavy process role

The `heavy` process exists to protect `main` from long or blocking work that would damage interactivity.

Typical `heavy` work includes:

- local BLAST database preparation
- local BLAST execution
- large parsing or indexing jobs
- file-heavy transforms that are safe to detach from the UI process

The `heavy` process should not absorb every non-trivial task. Short interactive work stays in `main` unless there is a clear reason to move it.

## Main-side ownership

`main` owns:

- startup of `heavy`
- liveness checks
- restart policy
- task submission
- task cancellation
- result collection
- user-facing error handling

The central scheduler also lives in `main`.

## Message types

The message model must stay small and explicit.

Suggested message families:

- run task
- task result
- task failure
- cancel task
- shutdown
- heartbeat
- heavy status snapshot

Optional resource-control messages may exist if needed, but IPC should not become a second scheduler.

## Task model

Each heavy task should carry:

- task id
- task type
- payload
- cancellation identity
- requested local slot count already granted by `core`

The important part is that scheduling happens before dispatch when possible. `heavy` is an execution host, not the place where global policy is invented.

## Registry model

The `heavy` process exposes a registry of supported task handlers.

Illustrative shape:

```go
type TaskHandler func(ctx context.Context, payload []byte) ([]byte, error)

func Register(taskType string, handler TaskHandler)
```

The registry is local to `heavy`. Workflow packages should not talk to the registry directly. They should go through `phygoboost/heavy/client.go`.

## Cancellation

Cancellation must propagate across the process boundary with low latency.

Required behavior:

- user cancels in TUI
- workflow context cancels
- `main` sends cancel message to `heavy`
- heavy task context cancels
- task stops and releases resources
- `main` updates UI and queue state

Cancellation is not a best-effort bonus feature. It is part of correctness.

## Host lifecycle

The `heavy` host should be long-lived.

Rules:

- start once when first needed or during app startup if policy chooses
- keep warm during active sessions
- detect dead or hung host
- restart when necessary

Do not spawn a fresh worker process for every medium-sized task. That pattern caused too much overhead and too much complexity before.

## Failure behavior

If `heavy` crashes:

- outstanding tasks fail clearly
- held grants are released
- the host state becomes unhealthy
- later work may trigger restart

Silent hanging is unacceptable. The host layer must expose enough observability to distinguish:

- waiting on queue
- executing
- cancelled
- failed
- transport lost

## IPC transport expectations

The transport should be:

- binary or efficient structured messages
- bounded in memory
- safe under cancellation
- robust on Windows

The exact transport mechanism can vary, but the design must not rely on interactive shell behavior or fragile text parsing.

## What must be deleted from the old idea

The rewrite should remove:

- overgrown generic process-pool logic
- hidden fallback execution paths that are hard to reason about
- transport code that also mutates policy
- worker-side copies of central scheduling logic

The new design intentionally keeps `heavy` simpler than the old `procworker` world.

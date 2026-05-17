# Phygoboost Rewrite Plan

## Purpose

This document records the current simplification target for the runtime system.

The decisive change is that the split runtime was removed entirely.

## Removed in this rewrite

- split-runtime code
- separate old level `2` runtime behavior
- distinct process-specific local budgets

## What remains

`internal/phygoboost/` remains responsible for:

- shared managed execution control
- per-domain network control
- shared HTTP transports
- runtime observation
- bounded local parallel helpers

## Constraints that survive

1. no business planner in runtime control
2. per-domain network pools
3. one shared managed total pool
4. FIFO fairness
5. explicit acquire and release lifecycles
6. workflow-owned progress counts

## Future cleanup direction

- continue shrinking leftover legacy names that no longer describe the code
- keep removing dead comments, dead tests, and dead docs that assume the removed split runtime

## Review checklist

Every runtime review should ask:

- Does this code belong to workflow or to runtime control?
- Is a domain explicitly identified?
- Does this change preserve the single managed execution model?
- Is acquire and release explicit?
- Is progress being counted by workflow code instead of inferred by the runtime?

# PHgo tview fork

This directory is based on `github.com/kivattt/tview v1.0.6` and is replaced from the root `go.mod`.

PHgo-local changes:

- application lifetime channel exposed through `Application.Done()`;
- cancellation-aware `QueueUpdateContext` and `QueueUpdateDrawContext`;
- shutdown-safe event/update queues that release blocked producers;
- resize-event coalescing through the event queue;
- pointer-only mouse motion skips full redraw while preserving click, wheel, release, and drag behavior;
- logical mouse actions do not share a capture-mutated event pointer;
- lifecycle tests for cancellation, saturated queues, shutdown, wheel, click, and pointer motion.

Keep the fork narrow. Do not add custom Windows screens or TTY wrappers here.

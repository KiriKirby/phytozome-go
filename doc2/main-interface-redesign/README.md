# Main Interface Redesign Design Index

This directory is the source of truth for the planned main-interface redesign.

The redesign is not an in-place replacement yet. The existing multi-screen workflow code remains available, but the current normal startup path is hard-coded to open the new workflow.

## Confirmed Direction

- Build a new main interface path instead of rewriting the existing screens directly.
- Keep the current screens and workflows intact until the new path is complete and intentionally replaces them.
- The new path should allow the user to configure the whole workflow in one interface before starting execution.
- The current implementation is enabled in code for normal startup; it does not depend on a runtime environment flag.
- Implementation details that change behavior must stay recorded in this document library before or alongside code changes.

## Current Implementation

- TUI entry: `internal/tui/main_interface.go`.
- Workflow adapter: `internal/workflow/main_interface.go`.
- Normal startup uses the new page. Handoff/transfer paths keep their existing flow.
- Keyword, BLAST, and Explore all remain backed by existing execution/review/export flows after the new page collects settings.
- Species selection opens an in-page large modal in the new main interface. The legacy selector pages remain available for the old workflow path; the new path uses its own option model and callback.
- Shared interaction, theme, shortcut, focus, hint-row, and background-lifetime rules are defined in [`../ui/interaction-and-theme.md`](../ui/interaction-and-theme.md) and [`../ui/shortcuts.md`](../ui/shortcuts.md). The homepage search grid retains its specialized interaction model.

## Document Map

- [Requirements Intake](./requirements-intake.md)
  Working log for user-provided design requirements, questions, and unresolved decisions.
- [Startup Home](./startup-home.md)
  Confirmed design notes for the new tabbed startup/home interface.
- [Keyword Tab](./keyword-tab.md)
  Confirmed design notes and implementation-level model for the Keyword tab.
- [BLAST Tab](./blast-tab.md)
  Confirmed design notes and implementation-level model for the BLAST tab.
- [Explore Tab](./explore-tab.md)
  Confirmed design notes for keeping Explore as the current menu-style interaction.
- [Execution Auto-Identification Flow](./execution-auto-identification.md)
  Shared pre-execution behavior for missing Symbol name and Gene locus values.
- [Implementation Boundary](./implementation-boundary.md)
  Guardrails for separating the new interface from the existing screens.

## Update Rule

When the redesign direction changes, update this documentation before or alongside implementation work.

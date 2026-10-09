# BLAST Tab Design Notes

This document records confirmed requirements and implementation-level design for the new BLAST tab.

## Source Sketch

The user's sketch is a conceptual reminder only. Do not copy its temporary labels, typography, sizes, spacing, border weights, or exact proportions. The real implementation must follow the current application's established TUI design language and shared tview patterns.

Reference sketch:

![BLAST tab conceptual sketch](./assets/blast-tab-sketch.png)

## Page Structure

The BLAST tab follows the same overall pattern as the Keyword tab:

- BLAST Options
- BLAST Content

The contextual bottom button bar belongs to the whole active tab.

## BLAST Options Module

BLAST Options contains a left-to-right row of controls whose visibility depends on the selected database.

Controls:

- database selector
- species selector button
- BLAST program selector

The database selector is a dropdown over all supported BLAST-capable databases.

Species selection:

- Shown only when the selected database/workflow requires species selection.
- Uses the same in-page large-modal behavior as the Keyword tab species selector.
- The modal loads candidates through the new-main callback and returns the selected species key/label directly into BLAST Options.
- Before configuration, show a concise "set species..." style label.
- After configuration, show the selected species using the shortest available display form, truncated with ellipsis when needed.

BLAST program selection:

- Shown only for databases/workflows that currently require the user to choose a BLAST program.
- Hidden entirely for databases where program selection is fixed, inferred, or not needed.
- Hidden controls do not leave holes; later visible controls shift left.
- When visible, the program selector is a dropdown opened with `Space`.
- Program order must preserve the order supplied by the existing capability layer. Do not promote a special default unless the existing workflow already does so.
- Program labels include the query/database direction, such as nucleotide-to-nucleotide or protein-to-translated-nucleotide, while preserving the program identifier used by execution.
- Visible BLAST Options controls are laid out horizontally and share the available width equally, whether there are two controls or three.
- The BLAST Options and BLAST Content module titles are right-aligned inside their borders.
- Every BLAST Options control uses the same vertical field form: title on the upper line and the actual dropdown/button below it.

## BLAST Content Module

BLAST Content is a spreadsheet-like multi-column text editor using the same interaction model as Keyword Search Content.

It should visually match the application's other table views, not merely approximate them: use the same two-space cell padding, bold header text, muted table separators, header divider line, active-cell blue highlight, fixed Row column, and no separate placeholder/hint row inside the editor.

Initial conceptual columns:

- FASTA
- symbol name
- gene locus as a future capability column

Column visibility depends on database/workflow capability:

- FASTA is always required.
- Current implementation generates columns from BLAST capability.
- Current BLAST modes show FASTA and Symbol name only.
- Current BLAST modes do not show Gene locus because the present BLAST entry chain does not expose that manual input as a current visible capability.
- The capability model keeps a Gene locus column flag for future BLAST workflows, but it must remain hidden until an actual workflow consumes it.
- Execution maps visible FASTA and Symbol name values into the existing BLAST query-item flow.
- Hidden columns do not create blank spacing.

Rows are synchronized across all visible columns:

- A row represents one complete BLAST input item.
- The visible column cells in the same row always move together as one row unit.
- Adding, deleting, pasting, wrapping, or scrolling must preserve row alignment across columns.

Input/navigation rules:

- Literal spaces are not allowed inside this editor.
- `Space` moves between visible columns in the current row and must not insert a character.
- Empty input and `~` both mean an empty value.
- Left/Right move inside the current cell text when possible.
- At the far left/right cell boundary, Left/Right moves to the neighboring visible column in the same row.
- Up/Down move inside the current wrapped cell when the current cell has multiple visual lines.
- When the caret is already on the top visual line of the current cell, Up moves to the same visible column in the previous logical row.
- When the caret is already on the bottom visual line of the current cell, Down moves to the same visible column in the next logical row.
- Top-level module focus movement is through `Tab`, not arrow keys.
- `Enter` inside the BLAST Content editor inserts an in-cell newline only when the active column is the FASTA column.
- Non-FASTA BLAST columns remain single-line cells. They stretch vertically with a wrapped FASTA row, but their own text is vertically centered and does not become multiline.
- Outside the BLAST Content editor, `Enter` activates the focused dropdown or module button.

Column width and visibility:

- FASTA is a wrapping column fixed to half of the current grid input width because FASTA headers and sequences are long.
- FASTA headers keep their natural line break from sequence content; pasted multi-line FASTA records stay in one logical row.
- Other columns keep the result-table sizing principle: each non-FASTA column width is derived from the longest visible value/header for that column.
- Initial non-FASTA column width is the header width itself. Do not impose a wider minimum before the user enters content.
- The grid has a fixed left Row column. Row numbers are shown only for created/populated rows, and the Row column stays visible when data columns scroll horizontally.
- Visible column separators are drawn between Row and data columns and between data columns.
- When a column is focused, horizontal viewport handling must make the whole focused column visible when possible.
- Avoid half-visible trailing columns when the viewport is wide enough to show whole columns.
- Mouse wheel scrolls rows vertically. Horizontal wheel events scroll columns; Shift+wheel also scrolls columns, matching the result-table interaction.
- Row height is the maximum wrapped visual-line count among cells in that logical row.
- Text inside shorter cells is vertically centered against the row height so Symbol name stays visually aligned with wrapped FASTA.
- Mouse click hit-testing must map screen rows through the visual row-height table, not through physical row index arithmetic, because wrapped FASTA rows occupy multiple terminal rows.
- Caret placement must use the same padded content width as wrapping and mouse hit-testing. The rendered caret should remain visible even in terminal themes where the native cursor is thin.

Current implementation:

- The FASTA column is fixed to half of the current grid input width and is excluded from leftover-width expansion.
- FASTA cells keep embedded newlines and wrap by visual width.
- Pasting into the FASTA column groups input by BLAST query record, not by physical clipboard line.
- A `>` FASTA header starts a FASTA record; following non-empty sequence lines remain in that record until the next `>` header or a blank-line boundary.
- Headerless consecutive sequence lines are grouped into one logical row so wrapped plain sequences pasted from another editor are not split into separate queries.
- Blank lines split records for both FASTA and headerless sequence blocks.
- A single headerless line containing multiple whitespace-separated tokens is split into multiple records. This keeps copied URL, identifier, or one-line sequence lists usable without distributing text across other columns.
- Mixed FASTA/headerless input is supported when the boundary is either a blank line or the next `>` header. A plain sequence line immediately after a FASTA header cannot be distinguished from normal FASTA continuation and is intentionally kept in that FASTA record.
- Pasting into Symbol name remains line-oriented and fills downward in that current column.
- Up/Down use wrapped visual-line movement first, then move to the previous/next logical row only from the top or bottom visual line.
- Manual FASTA editing supports explicit in-cell newlines with plain `Enter`.
- `Ctrl+R` is reserved for the tab's main action and must not be consumed by the FASTA editor.
- The drawn caret cell is reverse-highlighted in addition to using the terminal cursor, so the insertion point remains visible in wrapped FASTA cells.

## Button Bar

The BLAST tab contextual button bar contains:

- Clear Content on the left.
- Paste immediately to the right of Clear Content.
- Aliases on the left, shown only when the BLAST Content grid focus is on a Symbol name cell.
- Run BLAST on the right.

Clear Content:

- Clears only the BLAST Content grid/editor in the current BLAST tab.
- It does not reset BLAST Options.
- It does not reset other top-level tabs.

Paste:

- Pastes clipboard text into the currently selected cell's visible column in the BLAST Content editor.
- Paste does not distribute input across other columns.
- In the FASTA column, pasted text is first recognized and split into BLAST query records, then those records fill rows downward in the FASTA column from the active row.
- In non-FASTA columns, multi-line pasted text fills rows downward in the current column from the active row.
- Pasted `~` values normalize to empty values.
- Pasted literal spaces are rejected or normalized according to the final paste-format decision, but spaces must not become in-cell content.

Run BLAST:

- Usable only when BLAST options and BLAST content are valid enough to start the action pipeline.
- If required inputs are invalid, show a modal explaining what is still wrong.
- Missing optional Symbol name / Gene locus values use the shared auto-identification prompt flow before execution when the current workflow supports auto-identification.
- Current BLAST new-main columns show FASTA and Symbol name only, so current pre-execution auto-identification applies to Symbol name.

Button shortcuts:

- Button shortcuts should reuse the current application's existing button shortcuts wherever possible.
- Do not invent new shortcuts when an equivalent legacy action already has a stable shortcut.
- The main Run BLAST action uses `Ctrl+R` in the new main interface.
- Shortcut help remains reserved for shortcuts without visible button equivalents.

## Auto-Identification Before Execution

The BLAST tab must keep the same behavior as the current workflow for missing Symbol name and Gene locus values.

When the user invokes Run BLAST:

- If Symbol name values are truly blank and the selected workflow supports automatic symbol-name identification, show the prompt asking whether to auto-identify.
- `~` and `~~` are explicit user/set values and are not auto-filled.
- In BLAST multi-search mode, defined as more than one BLAST input row, Symbol name is required and the prompt does not offer Skip.
- If a future visible Gene locus column has missing values, show the Gene locus prompt after Symbol name.
- If both Symbol name and Gene locus need auto-identification, run Symbol name first, then Gene locus.
- Reuse the existing Symbol name identification modules and source-species keyword metadata cache where possible.
- Gene locus auto-identification uses the same BLAST query source metadata and keyword-row lookup terms as Symbol name identification, then writes the best available `GeneLocus`, stripped `GeneIdentifier`, transcript, protein, or sequence ID into the query source Gene ID path.
- The new tab is responsible for inserting those modules into the action flow and then updating the BLAST Content grid. After auto-identification, return to the BLAST tab so the user can edit the filled values before invoking Run BLAST again.
- Button enablement and button labels/states must stay synchronized with this flow so the user does not see stale availability after auto-identification changes the content.

Auto-identification prompts are not the same as validation errors:

- Required missing FASTA, missing required database, missing required species, or missing required BLAST program blocks execution with an invalid-state modal.
- Missing auto-fillable Symbol name or Gene locus should prompt for automatic identification when supported, matching current behavior.
- If the user skips automatic identification where Skip is allowed, continue or stop according to the existing workflow's semantics for that field.

## Suggested Code-Level Model

Use the same grid-editor architecture as the Keyword tab, but with BLAST-specific row data and capabilities.

```go
type BlastTabState struct {
    DatabaseID      string
    Species         *SpeciesSelection
    ProgramID       string
    Rows            []BlastInputRow
    Focus           BlastFocus
    LastValidation  BlastValidationResult
}

type BlastInputRow struct {
    FASTA      string
    SymbolName string
    GeneLocus  string
}
```

Define capability data instead of hard-coding layout branches:

```go
type BlastDatabaseCapability struct {
    DatabaseID            string
    RequiresSpecies       bool
    ShowsSpecies          bool
    RequiresProgramChoice bool
    Programs              []BlastProgramCapability
    ShowsSymbolName       bool
    ShowsGeneLocus        bool
    SupportsSymbolAutoID  bool
    SupportsGeneLocusAutoID bool
}
```

Generate visible options and content columns from current capabilities:

```go
func (s BlastTabState) VisibleOptionControls(c BlastDatabaseCapability) []OptionControl
func (s BlastTabState) VisibleInputColumns(c BlastDatabaseCapability) []BlastInputColumn
```

The content editor should share the same underlying implementation as Keyword where possible:

```go
type MultiColumnGridEditor[T any] struct {
    rows          []T
    columns       []GridColumn[T]
    activeRow     int
    activeCol     int
    caretByCell   map[CellCoord]int
    rowOffset     int
    colOffset     int
}
```

Implementation direction:

- Prefer one reusable grid-editor primitive parameterized by row/column adapters for Keyword and BLAST.
- Keep row editing, paste, caret movement, and column viewport behavior inside the editor primitive.
- Keep database capability decisions outside the primitive.
- Translate `BlastTabState` into the existing BLAST workflow request only when Run BLAST starts.
- Insert the existing Symbol name and Gene locus auto-identification flows before final BLAST execution.

## Open Decisions

- Exact final visible labels for database, species, BLAST program, content columns, and buttons.
- Whether Clear Content asks for confirmation.

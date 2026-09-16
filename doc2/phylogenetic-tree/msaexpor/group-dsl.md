# `msaexpor` Advanced Group DSL

## Purpose

The advanced group DSL lets users script exactly which PHgo rows and which MSA column spans appear in the exported figure, and how each span is split into major visual blocks.

The first implementation uses a multiline text box. Later structured controls must generate exactly this DSL and must not introduce a second layout grammar.

## Line Shape

Each non-empty line has this shape:

```text
>items\range[/allocation]
```

The backslash separates row selection from column range and optional allocation.

Example:

```text
>1,4/1,3/1,6/1,5/1,8/1,7\10,100/~,~,~
```

Parsing rules:

- each physical line is trimmed before parsing
- empty trimmed lines are ignored
- comments are not supported
- internal spaces inside tokens are invalid
- the first non-space character of a parsed line must be `>`
- every parsed line must contain exactly one row/range separator `\`
- every parsed line must contain at most one allocation separator after the range
- parsing is case-sensitive

## Row Item List

The left side after `>` is a `/`-separated list of PHgo coordinates.

Example:

```text
>1,4/1,3/1,6
```

Meaning:

- first exported row is PHgo coordinate `1,4`
- second exported row is PHgo coordinate `1,3`
- third exported row is PHgo coordinate `1,6`

PHgo coordinates are always the one-based coordinates shown in the MSA ID list. The `/msa/selection` API exposes matching one-based `canvas_item_index,canvas_row` values, while internal Canvas and legacy `metadata.records` indexes can remain zero-based. When both numeric fields and the canonical visible `display_prefix` exist, the exporter treats the prefix (for example `[1,4]`) as authoritative, so the script can always use the displayed coordinate verbatim.

Rows are rendered in the exact order listed. Current Jalview visual sort order is ignored while advanced layout is enabled.

### All-Row Shorthand

The row item side may be exactly:

```text
~
```

This means all current MSA rows in their normalized PHgo/MSA order.

Example:

```text
>~\10,100/~,~,~
```

Meaning:

- use every current MSA row
- export MSA boundary range `[10,100)`
- split the 90 columns into three 30-column major blocks

Rules:

- `~` is valid only when it is the complete row item side.
- `>~/1,3\10,100` is invalid.
- `>1,3/~\10,100` is invalid.
- the all-row shorthand does not filter by PHgo green/yellow/red row state.

Rules:

- each PHgo coordinate token is exactly `item,row`
- `item` and `row` are positive base-10 integers
- the coordinate must resolve to exactly one current MSA row
- duplicate coordinates inside one DSL line are invalid
- the same coordinate may appear on different DSL lines to export different MSA column ranges

## Column Range

The range is written:

```text
start,end
```

Endpoints are MSA boundary coordinates, not inclusive residue positions and not ungapped residue positions.

Definitions:

- `alignmentWidth` is the number of columns in the aligned sequence strings.
- Boundary `0` is the position before the first MSA column.
- Boundary `alignmentWidth` is the position after the last MSA column.
- MSA column `1` occupies boundary interval `[0,1)`.
- MSA column `N` occupies boundary interval `[N-1,N)`.
- A range always resolves to the half-open boundary interval `[start,end)`.

Example:

```text
10,100
```

The implementation must treat this as:

```text
[start, end)
```

That means `10,100` contains 90 alignment columns and is equivalent to the shorthand allocation `10,100/90`.

This boundary-coordinate rule is required to keep the user-stated equivalence `10,100 == 10,100/90` mathematically consistent.

Rules:

- `start` and `end` are integer boundary coordinates or `~`
- numeric endpoints must be in `[0, alignmentWidth]`
- `start` must be less than `end`
- `end - start` is the number of exported alignment columns
- column positions are alignment columns, not ungapped residue positions
- ranges are validated after `~` endpoints are resolved

### Range Endpoint `~`

In a range endpoint, `~` means the open edge of the current MSA:

- `10,~` resolves to `[10, alignmentWidth)` and exports from boundary `10` to the end.
- `~,10` resolves to `[0,10)` and exports from the beginning through boundary `10`.
- `~,~` resolves to `[0, alignmentWidth)` and exports the full aligned width.

`~` in a range endpoint is different from `~` in allocation counts. Range `~` resolves to a concrete boundary before allocation is evaluated.

## Allocation

Allocation appears after the range:

```text
start,end/counts
```

Counts are comma-separated. Each count creates one major block.

Example:

```text
10,100/30,20,40
```

Meaning:

- export columns `[10,100)`
- first major block has 30 columns
- second major block has 20 columns
- third major block has 40 columns

Resolved block ranges are allocated sequentially from left to right:

- `10,100/30,20,40` creates `[10,40)`, `[40,60)`, and `[60,100)`
- `10,100/~,30,~` resolves to `30,30,30` and creates `[10,40)`, `[40,70)`, and `[70,100)`
- allocation order controls the top-to-bottom order of major blocks generated from that DSL line

All major blocks are rendered left-aligned. If a set of major blocks from one line has different widths, shorter blocks leave blank space to match the longest block when right-side numbering and top numbering align to the longest block.

## Range Shorthand

If allocation is omitted:

```text
10,100
```

It is treated as:

```text
10,100/90
```

because `100 - 10 = 90`.

For ranges containing endpoint `~`, the shorthand is evaluated after resolving the endpoint:

- if `alignmentWidth` is `250`, `10,~` is treated as `10,250/240`
- if `alignmentWidth` is `250`, `~,10` is treated as `0,10/10`
- if `alignmentWidth` is `250`, `~,~` is treated as `0,250/250`

## `~` Allocation Placeholder

`~` means "fill from the remaining unallocated column count".

Examples:

```text
10,100/~,~,~
```

Total columns: `90`. There are three placeholders, so each gets `30`.

```text
10,100/30,~,~
```

Total columns: `90`. Fixed columns: `30`. Remaining columns: `60`. Each placeholder gets `30`.

```text
10,100/~,30,~
```

Equivalent to the previous example. Placeholders receive equal shares of the remaining columns regardless of position.

```text
10,100/30,~,30
```

Total columns: `90`. Fixed columns: `60`. The only placeholder receives `30`.

## Placeholder Validation

Rules:

- fixed counts must be positive integers
- placeholder-resolved counts must be positive integers
- fixed counts must not exceed total range length
- remaining columns must divide evenly across all `~` placeholders
- if the remaining columns cannot be evenly divided, the line is invalid
- if there is no `~`, fixed counts must sum exactly to range length
- if there is at least one `~`, fixed counts plus resolved placeholder counts must sum exactly to range length

## Multiple Lines

Multiple DSL lines are rendered top to bottom.

Example:

```text
>1,4/1,3/1,6/1,5/1,8/1,7\10,100/~,~,~
>1,4/1,3/1,6\10,200/30,~,~
```

All generated major blocks from all lines enter one normal vertical flow. Source DSL lines do not create extra visual separation.

The gap between major blocks is uniform whether those blocks came from the same DSL line or different DSL lines.

## Right Residue Number Alignment

For each major block, right-side residue numbers are aligned to the longest block width in that DSL line's allocation set.

If the allocation for one line is `30,20,40`, all right-side residue numbers and top column-number spans use the 40-column width as the alignment width for that set.

The exported residues themselves remain left-aligned.

## Error Reporting

The parser must report:

- line number
- original line text
- failing token
- short actionable message

Examples:

- `Line 2: expected row/range separator "\".`
- `Line 4: range start must be less than range end.`
- `Line 5: remaining 37 columns cannot be divided by 2 placeholders.`
- `Line 7: PHgo row coordinate 3,9 was not found in the current MSA payload.`

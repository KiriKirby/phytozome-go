# phytozome GO Agent Notes

This file tracks the intended shape of `phytozome GO` and its release packaging, with an expanded, actionable plan for fully supporting `lemna.org` mode (download-backed and BLAST interactions). It documents product identity, workflows, reliability constraints, design decisions for `lemna.org`, API surfaces, CLI UX, implementation tasks, tests, and risks.

## Product identity

- Product name: `phytozome GO`
- CLI command: `phytozome-go`
- GitHub repository and Go module: `phytozome-go`

## Top-level goals for lemna.org mode

- Provide a first-class `lemna` mode with the same user workflows as `phytozome` where feasible:
  - species search and selection
  - keyword mode (GFF3 + AHRD downloads)
  - BLAST mode (server submission where robust, and local BLAST fallbacks)
  - peptide FASTA export and resolving sequences from release FASTA
- Keep all site-specific scraping, download parsing, and fragile logic isolated under `internal/lemna`.
- Prefer reliable data sources: treat the `download/` release directory as the authoritative source of available databases/releases; use the public homepage only for labeling and "official" marking.
- Expose capabilities to the CLI so the workflow can choose between server BLAST and local BLAST fallbacks transparently.

## Long-term database and workflow direction

- Treat `Phytozome` and `lemna.org` as primary sequence/genome sources. They provide species selection, keyword search targets, BLAST targets, source FASTA/GFF-style assets, gene/protein identifiers, and peptide export data.
- Keep search, BLAST, and labelname identification as three separate systems:
  - Search is only the keyword/search-engine path for the selected primary database.
  - BLAST is only BLAST execution, parsing, review, filtering, and export of BLAST hits.
- Labelname identification is a standalone ranking system under `internal/labelname`. Workflows collect alias candidates from their own search terms, protein/gene identifiers, source aliases, FASTA headers, and database-specific metadata, then send each item to labelname with the current task timestamp and task-local item index. Labelname returns the same timestamp/index plus aliases sorted by priority. The workflow stores that ranked list, uses the first alias as the default `label_name`, and passes the stored list forward.
  - BLAST query/source auto labelname and BLAST-hit auto labelname should prefer two-phase execution for performance: parallel candidate collection first, then batch `labelname` ranking over the de-duplicated requests.
  - BLAST-hit auto labelname should cache hit-level identification decisions by a stable row/source-label key across repeated runs so reopening/re-filtering/export chains do not redo the same keyword-label resolution work.
  - Auto labelname stages must expose visible progress states instead of a silent wait; at minimum distinguish candidate prefetch/collection from ranking/application so users do not interpret the phase as a hang.
  - Table selection performance is part of workflow performance, not only UI polish:
    - large row-selection tables and BLAST run tables must keep `SetEvaluateAllRows(false)` so off-screen rows are not eagerly re-evaluated during draw
    - repeated selection-only actions such as `Ctrl+A`, `Ctrl+N`, header checkbox toggle, or single-row checkbox toggle should update only visible checkbox/title/list state when the row order and table contents did not change; do not rebuild the whole table for those actions
    - grouped row-selection tables should keep a direct original-row -> display-row index cache after each display-row rebuild so selection restore and current-row lookup do not rescan the full visible row slice on every interaction
    - grouped display-row construction must not rescan the sorted order once per group; build ordered/group lookup structures once and reuse them
- `phgo_alias` is the phytozome GO authoritative alias column produced by the labelname system. Result table views show `phgo_alias`; the original source `alias` column remains available in row details, exports, and reports immediately after `phgo_alias`.
- Phytozome keyword rows keep source `symbols` and `synonyms` as separate fields instead of merging them into `alias`. They are not table-display columns. For each Phytozome keyword item, labelname candidate collection uses `synonyms` first, falls back to `symbols` only when synonyms are empty, and falls back to candidates parsed from source `auto_define` only when both are empty.
- Phytozome BLAST query/source labelname collection follows the same per-item fallback as Phytozome keyword: `synonyms` first, then `symbols`, then `auto_define`. FASTA parenthetical/header labels are never direct query labels; they are only the final fallback alias candidate when the three database-derived layers are empty. BLAST query/source `label_name` is required before BLAST execution can continue.
- Optional BLAST-hit labelname identification also follows the Phytozome keyword fallback for each hit (`synonyms` -> `symbols` -> `auto_define`) using de-duplicated, parallel Phytozome keyword lookups for hit identifiers. If those three layers produce nothing, the hit falls back to the BLAST source item's `label_name`; if the source item has no label, the hit label stays blank.
- Lemna keyword and Lemna BLAST labelname candidate collection share the same source ordering: first look up usable gene/protein/transcript IDs in Phytozome and apply the Phytozome three-layer fallback (`synonyms` -> `symbols` -> `auto_define`), then fall back to Lemna local alias candidates from the row/source metadata. For BLAST source/query items only, FASTA header aliases are the final fallback after Phytozome and Lemna local candidates fail, and the whole ranked alias list is stored for grouping/custom-group reuse. For BLAST hit rows, the final fallback after Phytozome and Lemna local candidates fail is the BLAST source/query `label_name`.
- Keyword-to-BLAST transfer keeps only reusable workflow outputs plus BLAST execution/fallback material: stored query-source `label_name`, stored `phgo_alias`, resolved peptide sequence, source URLs, source database/species identifiers, target-compatible gene/transcript/protein identifiers, and other sequence-resolution metadata needed by downstream BLAST fallbacks. It must not forward source `alias`, `symbols`, `synonyms`, or `auto_define` as fresh BLAST label candidates.
  - Keyword-to-BLAST preparation should treat the selected-row transfer as its own performance-critical stage:
    - de-duplicate sequence fetches by sequence id through the shared protein-sequence cache/singleflight path
    - de-duplicate BLAST query-item construction by stable keyword-row cache key within the same transfer pass, not only across later re-entry
    - expose at least two visible progress phases: fetching keyword peptide sequences, then building cached BLAST query items
- A `label_name` selected from a keyword result table is the BLAST query/source label, not the label for every BLAST hit. It is preserved as query metadata, written to hit rows through `blast_labelname`, used in TXT query-sequence headers, and included in the Excel query metadata sheet. BLAST hit rows have their own independent `label_name`, with `labelname_type` recording how that row label was obtained.
- In keyword modes, `phgo_alias` is each keyword result item's ranked alias list. In BLAST modes, `phgo_alias` is each BLAST hit/result row's ranked alias list, not the BLAST source/query item's alias list. Source/query aliases stay on `QuerySequenceSource.PhgoAliases` for metadata, family BLAST, and custom grouping.
- Keyword-to-BLAST skips the BLAST query label input page only when every selected keyword row already carries a non-empty query-source `label_name`; otherwise the normal BLAST label input flow still runs for the remaining unlabeled query items. This query label flow is separate from optional automatic BLAST-hit label identification.
- Family BLAST, the custom grouping editor, and multi-file BLAST grouping define groups by the genes/sequences used as BLAST queries (`blast_labelname`/query-source gene identity), not by each BLAST hit row's own `label_name`. Hit-level labels may differ inside one family table and must not change the query-family grouping definition.
- Family BLAST and the custom grouping editor must consume the ranked alias lists already stored by the workflow during auto-identification for grouping, member alias dialogs, semantic alias tokens, and default member names. They must not query labelname again or independently rebuild alias ranking.
- Treat `UniProt`, `InterPro`, `Plant Reactome`, `PlantCyc`, and `MetaCyc` as external knowledge layers, not as peer primary databases in the startup selector:
  - `UniProt`: protein names, reviewed/unreviewed status, canonical length, GO, EC/catalytic activity, pathway notes, subcellular location, keywords, sequence cautions, and cross-references.
  - `InterPro`: protein families, domains, motifs, signatures, and Pfam-backed domain evidence through InterPro rather than direct Pfam-only coupling.
  - `Plant Reactome` / `PlantCyc` / `MetaCyc`: pathway skeletons, reactions, compounds, enzyme steps, and pathway-guided candidate discovery.
- The long-term biological workflow target is pathway-guided protein discovery: a user can choose a species, search a function or pathway such as `Monolignol Biosynthesis`, view a pathway diagram with enzyme steps annotated by candidate proteins, then select proteins for keyword review, BLAST, cross-species BLAST, export, or comparison.
- Pathway discovery should combine pathway skeletons from Plant Reactome/PlantCyc/MetaCyc with candidate proteins from Phytozome/lemna and evidence from UniProt/InterPro. Candidate confidence should be based on multiple signals such as keyword match, BLAST homology, EC/GO match, domain/family match, sequence length, reviewed status, and sequence caution flags.

## CLI UX standards

- Treat the terminal UI like a commercial wizard: every major step must be easy to scan, self-explanatory, and safe to recover from.
- The product is pure TUI for all interactive workflows. All interactive workflow design and implementation must target the shared tview template layer; do not preserve, add, or keep a traditional text wizard fallback.
- Delete traditional interactive paths instead of hiding them. The interactive wizard must not contain stdin `readLine` prompts, `Press Enter to continue` pauses, stdout banners, stdout status logs, stdout tables, stdout spinners, or stdout progress bars.
- Use mature Go TUI libraries as the primary implementation path. The default stack for interactive screens is now `github.com/rivo/tview` plus `github.com/gdamore/tcell/v2` so the app can rely on mature widgets, focus handling, mouse handling, forms, lists, tables, buttons, pages, and layout primitives instead of custom drawing.
- Do not hand-roll custom terminal widgets when tview already covers the need. Custom code should be limited to app-specific screen composition, workflow state, and small adapters between workflow data and tview primitives.
- UI reference baseline: `letientai299/7guis`, especially `tui/`, is the visual/structural reference for simple module composition, restrained widgets, and clear page structure.
- Technical reference baseline: `kivattt/fen` is the input/backend reference for Windows-compatible tview behavior. Treat its application setup as the source of truth for keyboard, mouse, paste, resize, and terminal reliability.
- Follow the `7guis/tui` style and architecture:
  - configure a global `tview.Styles` theme once
  - follow `kivattt/fen` for terminal backend initialization: use a plain `tview.NewApplication()`, the default tcell screen, `EnableMouse(true)`, and `EnablePaste(true)`
  - use `github.com/kivattt/tcell-naively-faster/v2` and `github.com/kivattt/tview` through `go.mod` replace directives, matching `fen`'s dependency strategy
  - compose screens with `Frame`, `Flex`, `Pages`, `List`, `Table`, `Form`, `InputField`, `Button`, and `TextView`
  - prefer a sidebar/page or framed-panel structure over hand-drawn ASCII layout
  - use bordered tview widgets for modules and selection areas
  - use tview button widgets for actions and keep mouse enabled; mouse must remain optional, not required
  - keep keyboard behavior available, but let tview manage focus traversal and widget events wherever practical
- Classify interactive pages by behavior and reuse templates instead of building each screen separately:
  - startup/multi-module pages: `Frame + Flex` with ordered modules; Enter advances modules, final Enter confirms
  - single-choice pages: `List` plus action buttons
  - text input pages: `InputField` plus action buttons
  - multi-line input pages: `TextArea` plus action buttons
  - row-selection pages: shared `Table` result template plus Back/Home/Copy/View/Export buttons only
  - recovery/error pages: `List` or `Modal` with retry/skip/back/exit-style choices
  - progress pages: tview modal progress/status primitives
- Row-selection checkbox state is semantic and consistent across the app: selected/on checkboxes are green, and unselected/off checkboxes are red. This belongs in the shared TUI layer, not in each workflow.
- Row-selection tables use a shared Excel-like interaction model:
  - The checkbox column and row-number column are fixed on the left; content columns can scroll horizontally, and result rows can scroll vertically.
  - The first header cell is a checkbox that toggles all rows. Do not render separate Select all, Clear, or Toggle buttons. Keep `Ctrl+A`, `Ctrl+N`, and `Space` shortcuts.
  - The default sort is row-number ascending. The row-number header must show the initial sort indicator.
  - Selection is a single cell, not an entire row. Do not add Excel-style row/column crosshair background fills unless explicitly requested again; they were tried and removed for readability.
  - Press `Tab` to switch between table-cell control and column-header control. Header control must visibly focus the current sortable header cell, keep the original header text readable, and keep Left/Right navigation locked within sortable headers. Up/Down changes only the current header's sort direction and must not move focus. Do not add a whole-header-row background unless explicitly requested again.
  - Mouse-clicking a sortable header changes/toggles sort but does not switch into header-control mode.
  - Non-sortable columns are skipped by header-control Left/Right navigation and ignored by mouse sort clicks.
  - Columns are auto-sized from the longest displayed value/header so cell text is not truncated by fixed column widths.
  - Columns should have comfortable horizontal padding and low-contrast vertical separators between them so dense tables remain readable without making grid lines visually dominant. The header row and table body should also be separated by a small gap plus a matching low-contrast horizontal divider.
  - Data columns should be displayed as whole columns whenever possible. Avoid showing a half-visible trailing column; hide that column until horizontal navigation can bring it fully into view, except when the viewport is too narrow to fit even the fixed columns plus one data column.
  - BLAST result table headers with ratio-style `/` labels must render as two physical header rows only in BLAST tables. Keep the numerator and trailing slash on the first row, for example `align_len /`, and put the denominator on the second row, for example `query_length (%)`. Non-BLAST tables must not split headers just because a `/` appears.
  - Do not draw scrollbars or hidden row/column status bars in table modules. The row-number header shows the total row count, for example `38 lines`, and the table itself handles vertical and horizontal navigation.
  - `View` in table-cell control means a large modal showing all available details for the currently focused normal data row, not a summary of all selected rows or the whole table. The row-number column is a valid row focus for row View. In column-header control, `View` shows a column explanation modal for the currently focused header, with English, Chinese, and Japanese explanations. On group/header rows outside header-control, on the checkbox column, or when no data row/column can be viewed, invalid view attempts should show a small OK-only modal instead of returning a workflow error.
  - `Copy` in row-selection tables copies only the currently focused normal data cell to the clipboard. The row-number column is copyable and copies the row number. It is exposed as a button plus `Ctrl+Shift+C`, and is disabled with a small OK-only modal in header control, on checkbox/group/header cells, or when no copyable cell is selected.
  - Copy success should be silent. Do not open a success modal, do not flash the Copy button, and do not call `Application.Draw()` directly from table/button callbacks; only failure or invalid-copy cases should open a small OK-only modal.
  - Display-column lists affect only the table view. Exports, generated files, and View row-detail dialogs must continue to use the full underlying model data.
  - Every table type must keep explicit column lists for display columns and full-detail/export columns. Manage column order and column names through those lists rather than scattering ad-hoc header order in render/export code.
  - BLAST UniProt display-column ordering rule: the two pre-existing comparison columns, original-database `target_length / UniProt canonical length (%)` and `UniProt canonical length`, stay in their original length-reference positions in the table and details. The comparison header must name the current original database, for example `Phytozome target_length / UniProt canonical length (%)` or `lemna target_length / UniProt canonical length (%)`. Other UniProt display columns belong after the original database columns. Table view must not show `UniProt keywords`, `UniProt EC`, or `UniProt GO`, although details and exports may include them.
  - `UniProt reviewed` table cells should be color-coded by status while remaining readable as text. Empty UniProt cells stay blank; do not use placeholders for rows without UniProt data.
  - BLAST InterPro display-column ordering rule: the old `Pfam_domain` position is owned by `InterPro conserved region status`, displayed as a two-line header and color-coded by `present`, `partial`, `missing`, and `uncertain`. Other InterPro columns are appended after original database columns and UniProt columns, with every InterPro-sourced header prefixed by `InterPro`. Details and exports should preserve the same original, UniProt, InterPro grouping, and InterPro Excel headers use the InterPro header color. The external-reference dialog owns the InterPro enable switch and detailed `InterPro conserved region status` thresholds/evidence checkboxes.
  - InterPro enrichment should use the official EBI InterPro API by UniProt accession, with caching, de-duplication, and parallel lookup. Blank is preferred over weak invented mappings; when query InterPro evidence is unavailable, status may be conservatively derived from the hit protein's own conserved region evidence rather than forcing all rows to `uncertain`.
  - BLAST filter behavior: the filter is a row-selection-table action, not a destructive data transform. It is available only when BLAST results have all external references enabled, meaning both UniProt and InterPro reference columns are present for the run. The filter opens a large parameter modal with detailed hard-rule and optional soft-score controls, plus the standard trilingual Help window. Applying the filter automatically rebuilds the suggested checkbox state: rows judged plausible are checked, rows suggested for removal are unchecked, and the suggested-removal row numbers are colored red. Users may manually change checkboxes afterward, and running the filter again restores the filter's own current recommendation.
  - BLAST filter default intent: conservatively remove hits that are unlikely to be true homologs before export or follow-up BLAST, using only generic evidence columns and thresholds. Default hard rules emphasize the two main biological failure modes for homolog pickup: abnormal amino-acid length and lost conserved-region evidence. By default, original target_length / UniProt canonical length must be present and within 70-130%, UniProt fragment records are rejected, and `InterPro conserved region status` must be `present` or allowed `partial`; `missing`, `uncertain`, and blank InterPro status are rejected. Identity, align_len/query_length, and E-value thresholds default to 0/off and are available as optional stricter hard rules for follow-up passes. Do not reject all unreviewed UniProt rows by default and do not require UniProt accession by default. By default the filter also collapses transcript isoforms to the best row per target gene. Do not add family-specific, species-specific, pathway-specific, paper-specific, copy-number-specific, whitelist, or special-name matching behavior to the filter; papers may only be used to calibrate generic thresholds. Optional generic top-hit limiting and soft scoring exist for stricter later passes, but should remain off by default.
  - BLAST filter parameter-surface rule: every numeric threshold, score weight, penalty, multiplier, distance band, evidence requirement, missing-data behavior, row-limiting mechanism, and ranking/tie-break preference used by the filter must be exposed in the filter parameter modal and round-tripped through model, prompt, and TUI settings. Do not add hidden filter constants or hard-coded ordering rules that a user cannot inspect and change from the modal.
  - BLAST filter execution rule: applying a filter, recalculating filter suggestions, clearing filter marks, and resetting selected rows from filter state must run behind a TUI loading/progress modal over the current result table. Never perform these actions synchronously in a table callback in a way that freezes the terminal. The modal must include `Cancel (Esc)` and cancellation returns to the same result table without showing a workflow error.
  - BLAST filter performance rule:
    - when suggesting rows for `RowsByRun`, process runs in parallel with a bounded worker pool instead of serially walking each run
    - ranking/sorting helpers for top-hit and best-isoform limits must reuse precomputed per-row metrics such as query coverage, parsed E-value, and reference score instead of recomputing them inside sort comparison hot loops
- Scrollable module status:
  - Do not add hidden-row/hidden-column status lines to scrollable modules. This design was removed because it made table and modal layouts heavier without improving navigation.
  - Do not draw custom scrollbar tracks or thumbs.
- Keyword result tables for both `phytozome` and `lemna` preserve search-term grouping:
  - Each search term appears as a centered, non-selectable group title row above its result rows, matching the old non-TUI table grouping intent.
  - Search-term group order is fixed to the query order.
  - Sorting in keyword tables is performed only inside each search-term group; sorting must never move rows between groups.
  - The `search_tern` display column is not sortable because group headers already define the search-term partition.
  - Keyword result short labels such as `C4H`, `CCR2`, or `ATPAL1` are `label_name`, not `geneid`. Phytozome keyword results expose this naming label as `label_name`; source protein/gene identifiers are shown as `geneid` only when the selected database naturally provides them independently of the label feature.
  - Keyword display columns are checkbox, row count, `search_tern`, conditional `label_name`, `labelname_type` where available, `phgo_alias`, conditional original `geneid` only when the source naturally provides it, `transcript`, and `discripition`; `search_tern` is not sortable, while `label_name`, `labelname_type`, `phgo_alias`, original `geneid`, `transcript`, and `discripition` are sortable.
  - Keyword-mode label names are part of the main keyword review flow, not export-only data. Ask for them immediately after keyword input and before running keyword searches for both databases, so table display, View dialogs, file naming, and FASTA labels can use `label_name`.
  - Keyword and BLAST label prompts use `Auto identify (Enter)` as the primary action when the input is empty. As soon as any text is present, the same primary button becomes `Apply (Enter)`. `Skip` is shown immediately to its left in the right-side action group and uses a non-Enter Ctrl shortcut. Auto identify must produce the same label list as manual input would: for keyword mode, label trust order is user-entered `label_name`, then labels inferred from database/result data; within automatic keyword inference, prefer an explicit row `label_name`, then the first alias (for example `ATPAL1` from `ATPAL1; PAL1`), and only then gene/transcript/sequence identifiers. For BLAST query/source automatic label inference, user-entered `label_name` is authoritative; otherwise Phytozome database candidates use the `synonyms` -> `symbols` -> `auto_define` fallback, and FASTA header labels are only the last fallback alias when database candidates are absent. User-edited family/custom grouping after automatic identification is authoritative and must not be rewritten by these automatic label priority rules.
- Treat the application as query-first. Search/query workflows should end at selectable result tables by default; file generation is a subordinate table action, never the default meaning of completing a table.
- In every result table, the default primary action is `View` (`Enter`): it opens a small details dialog for the current/selected rows. Export is exposed as `Export` (`Ctrl+G`) and batch export as `Export all` (`Ctrl+D`) where applicable.
- Do not ask for export filenames, output folders, label names that are only needed for exported files, data analysis reports, peptide fetching, Excel writing, list-file writing, or any other export-only data until the user explicitly chooses `Export` or `Export all`.
- This separation must be enforced in workflow/business logic, not only in button labels. Before the generation subflow is opened, no export path should create output directories, derive export file names, write list files, fetch peptides only for exports, or run export-specific preparation.
- Export settings are a shared contract across all generation paths. When adding or changing export options in the TUI, propagate the returned values through `prompt.ExportSettings`, workflow `exportSettings`, and every writer branch before considering the work complete. This must cover BLAST single export, BLAST `Export all`, keyword export, text output, normal Excel, raw Excel, and the data analysis report option. A missing propagation step can make all `Write*` flags false and surface as `No files were written`; add regression tests around any new export setting.
- Data analysis reports replace the old detailed TXT log concept. Do not revive timestamped `*_blast_log.txt` or `*_keyword_log.txt` writers.
- Reports must be generated from structured report data first, then rendered to PDF by an internal Go implementation or an internal HTML-to-PDF backend. Do not use Carbone for this feature.
- Reports must never trigger database searches, API calls, BLAST runs, sequence fetches, enrichment requests, cache refreshes, downloads, or any other data-gathering work solely for report writing. A report may only use data already known to the program because the current export action needed it. If desired report content is missing, write an explicit "not available in this run" explanation instead of fetching it.
- A report covers only the current generated file set and the process that produced it: query/source input, input resolution, selected database/species, search or BLAST parameters, result/selection summary, generated files, and relevant row-level evidence for the current export. Do not include earlier session navigation, earlier searches, unrelated tools, future pathway actions, or any operation outside the current export action.
- Each explicit generation action creates at most one report. A normal export that writes both `.xlsx` and `.txt` still gets one report. BLAST multi-file/export-all mode also gets one report for that action, not one report per generated file.
- Report file names use the generation completion timestamp followed by `_report`, for example `20260505_143012_report.pdf`. The timestamp must be local time, sortable, filesystem-safe, and independent of the exported data file base name.
- Export settings modals must size themselves from actual content. Do not hard-code a height that can clip checkboxes or leave large dead space after adding options. The file-type section must always show all available options, and its copy must explain that normal Excel exports selected rows while raw Excel writes every table row, including unchecked rows, to a separate `_raw.xlsx` file.

## Data Analysis Report Design

- Mode-specific report design belongs in dedicated files under `docs/reports/`.
- Keyword-mode report design is specified in `docs/reports/keyword-report-design.md`; treat that file as the source of truth for keyword report content, order, chart choices, layout rules, provenance rules, performance sections, quality checks, and implementation data model hints.
- Future BLAST, pathway, or other report types should get their own files in `docs/reports/` instead of expanding this agent note.
- Do not keep text fallback paths for interactive workflows, including tests. Tests should cover pure parsing/business helpers or TUI primitives, not simulated stdin/readLine wizard flows.
- The interactive terminal UI should be implemented as a conservative TUI, with Windows 10 and Windows 11 `conhost.exe` as the baseline compatibility target.
- Keep keyboard navigation as the primary interaction path in every TUI screen:
  - arrow keys move selection
  - Enter confirms
  - Esc performs Back on full-screen pages when a safe back target exists
  - Esc closes the topmost modal/overlay when a modal/overlay is open; modal input must be captured by the topmost modal/overlay before any underlying full-screen page receives keys
  - Ctrl+B is not a navigation shortcut; reserve it for the explicit `Run BLAST` result-table action only
  - Back has one absolute meaning: return to the full-screen page the user saw immediately before the current full-screen page
  - Close has one absolute meaning: dismiss the topmost modal/overlay and return to the underlying full-screen page
  - Back must never mean "exit the program", "close only the current modal", or "return to an arbitrary earlier workflow state"
  - Modal dismissal is `Close`; full-screen navigation is `Back`
- Mouse support is enabled, matching `kivattt/fen`'s Windows-tested tview setup. It must remain optional and must never be required to complete a workflow.
- Avoid terminal features that are fragile in Windows `conhost.exe`:
  - do not depend on emoji, Powerline glyphs, ligatures, gradients, or 24-bit truecolor
  - prefer ASCII borders and conservative symbols for core UI structure
  - keep color as an aid only; every status must also be readable as text
  - avoid drag interactions, complex animations, and layout assumptions that fail under IME input or narrow windows
- TUI output must fail clearly when no interactive terminal is available:
  - do not silently fall back to the old text prompt wizard; the old text prompt wizard is not supported
  - non-interactive commands such as `version` and `blast plan` may remain plain text
  - interactive wizard commands should require a real TTY and return a clear error otherwise
- Keep the TUI layer separate from workflow/business packages. The TUI should collect user choices and display state; BLAST, species, export, cache, and download behavior should stay in the existing domain packages.
- Use one consistent `7guis/tui`-style shell:
  - a top `Frame` area shows only the left-aligned breadcrumb/layer line; do not add a separate divider line there
  - breadcrumb/layer data should be modeled as a real path, not a hardcoded two-item label
  - when breadcrumb text is too long, collapse from the left with `...`, preserving the deepest/current page labels on the right
  - page titles should live in the current module/frame title, not as a separate top title
  - the main area uses tview layout primitives, not manual string drawing
  - startup page: show a natural short program introduction, author, repository, and license, then one `Startup` module with two stacked tview `List` selection boxes for `Function` and `Sub-option`
  - startup page behavior: function and sub-option live on one page; Enter moves through modules in order, and Enter on the final module starts the chosen workflow
  - startup page number shortcuts are global across the whole page: `1`, `2`, and `3` choose the fixed function module (`Keyword`, `Blast`, `Tools`); sub-option shortcuts always start at `4` and proceed downward based on the currently selected function. Sub-option numbering is dynamic and should not reserve permanent numbers for hidden options.
  - current startup sub-options: `Keyword` offers `Phytozome keyword` and `lemna keyword`; `Blast` offers `Phytozome blast` and `lemna blast`; `Tools` currently offers `Pathway search` as an entry placeholder only.
  - `Pathway search` must remain an inert/placeholder entry until its pathway implementation is added. It should not silently run keyword or BLAST behavior.
  - mouse clicks select/focus widgets only; mouse clicks on selection lists must not advance to the next module or enter the next page
  - bottom hints use `TextView` lines for keyboard shortcuts and explanations not already visible as buttons
- Resize behavior:
  - let tview/tcell own resize handling through the default application and screen
  - on Windows, keep a lightweight resize sync fallback because maximizing, minimizing, or restoring `conhost.exe` can miss the normal resize event; the fallback should ask the active tcell screen to resync instead of rebuilding page state
  - keep every interactive template full-screen by calling `SetRoot(root, true)`
  - avoid manual viewport padding, custom resize clearing, or hand-rendered frame strings unless a tview primitive cannot express the layout
  - apply conservative background colors through tview styles and primitives, not through raw ANSI string rows
- Button rules:
  - render only mouse-clickable actions as buttons
  - every button label must include its shortcut in parentheses, for example `Search (Enter)` or `Paste (Ctrl+V)`
  - letter shortcuts must use Ctrl combinations; never bind plain letters such as `q`, `b`, `h`, `a`, or `d` in TUI pages because plain letters must remain safe for text input
  - never bind `Ctrl+H`; many terminals encode Backspace as Ctrl+H, so using it for Home/Help/Back breaks text editing
  - use `tview.Button` for buttons whenever possible
  - button rows must wrap by whole button units when horizontal space is insufficient; never truncate or partially draw a button just to keep a one-line row
  - button rows occupy one layout line by default; wrapped extra lines expand upward into the flexible content area only when the current width actually requires wrapping
  - never reserve worst-case multi-line button height on wide layouts, because that creates dead space between the main content and hints
  - left-side buttons wrap with other left-side buttons, right-side primary buttons wrap with other right-side primary buttons, and the two groups must not push, overlap, or hide each other
  - every page and modal must add buttons through the shared button-row helper so wrapping behavior stays global instead of hard-coding per-page button heights
  - global navigation buttons are limited to Back and Home
  - do not show Quit/Exit as a global quick button; closing the terminal/window is the supported program-level exit path
  - do not expose a Mode/Spawn button in the TUI
  - navigation buttons such as Back/Home and progression buttons such as Next/Start/Confirm should be real focusable/clickable controls
  - progression buttons such as Start, Select, Search, Run BLAST, Apply, Save, View, Export, and Export all belong at the far right of the button row
  - progression buttons must use the same blue as the focused module border/title, distinct from normal dark-blue buttons
  - button text should be concise and verb-led; avoid vague labels like `Go`, `Use`, `Choose`, `Next`, `Continue`, `Confirm`, `Next / Start`, or verbose `Use ...`
  - button labels and shortcuts must come from the shared button dictionary in `internal/tui/buttons.go` whenever a standard action exists
  - standard button dictionary:
    - navigation: `Back (Esc)`, `Home (Ctrl+O)`
    - modal dismissal: `Close (Esc)`, `OK (Enter)`
    - selection/progression: `Start (Enter)`, `Select (Enter)`, `Search (Enter)`, `Run BLAST (Ctrl+B)` when launched from a result table and `Run BLAST (Enter)` when it is the primary action of a dedicated BLAST start page, `Apply (Enter)`, `Auto identify (Enter)` when it is the primary empty-input action, `Save (Enter)`, `View (Enter)`
    - export/results: `Export (Ctrl+G)`, `Export all (Ctrl+D)`
    - table selection shortcuts only, not visible buttons: `Ctrl+A` select all, `Ctrl+N` clear all, `Space` toggle current row
    - editing/recovery: `Paste (Ctrl+V)`, `Retry (Ctrl+R)`, `Skip (Ctrl+R)` when it is a secondary input-page action, `Install (Enter)`, `Yes (Enter)`, `No`
  - identical or similar actions must use identical labels across all pages; for example export actions are always `Export` / `Export all`, not `Generate file`, `Write file`, or `Create`
  - every interactive page after the startup page should expose a Home button that returns to database/startup selection
  - every full-screen page that exposes Back must map Back to the immediately previous full-screen page in the user-visible flow
  - do not emulate buttons with plain colored text unless a tview widget cannot represent the interaction
  - non-button keyboard hints such as `Tab switch selection box` or `Up/Down choose item` go in hint `TextView` lines, not inside the button row
- Selection page behavior:
  - multi-module pages must have an explicit module order
  - `Enter` advances through modules in that order; on the final actionable module it confirms the page
  - `Tab` switches modules in order without confirming and must wrap from the last module back to the first
  - intercept `Tab` at the app/template layer before tview `List` receives it, because `List` treats Tab as item navigation by default
  - arrow keys move within the focused section
  - number keys select within the focused section
  - mouse click may select/focus but must not advance modules or confirm the page unless the clicked widget is an explicit button
- Search page behavior:
  - search pages combine the search input and search results on one screen, like a modern search interface
  - editing the search input refreshes the result list in the same page; do not split search input and result selection into separate pages
  - search pages use one keyboard model, not separate active regions: Left/Right always move the search-box cursor, Up/Down always move the highlighted candidate
  - search result highlighting must use exactly one visual system; do not combine tview's default selected-row background with custom blue text highlighting
  - when the highlighted result is at the top and the user presses Up, move to the previous page and highlight that page's last result; when it is at the bottom and the user presses Down, move to the next page and highlight that page's first result
  - search results do not use number shortcuts or visible numeric prefixes; plain digits remain editable search text, and keyboard selection is Up/Down plus Enter
  - use pagination for result sets larger than 10; `Tab` and `PgDn` move forward, `Shift+Tab` and `PgUp` move backward
  - show a dedicated centered page selector below the results, make every page number mouse-clickable, and wrap page numbers onto additional lines instead of truncating after a fixed count
  - each result item should use two display rows: the name/key on the first row and secondary details on the second row, rather than squeezing name and details into separate columns
  - Back from search pages returns to the Startup page that contains both database and mode selection, not to a separate mode-only page
- Focus styling:
  - the active module's border and title must be bright blue
  - inactive module borders and titles use the normal theme colors
  - focus styling must be attached to real tview focus/blur events, not set once during construction
  - apply the same focus styling rule consistently across lists, tables, text inputs, and multi-line inputs
- Prefer modal dialogs for small decisions:
  - use centered modal overlays for every loading state, progress state, error reminder, warning, confirmation, validation message, and small status notice
  - hard rule for every workflow: before opening any modal, prompt, info page, recovery page, confirmation, or any other user-facing overlay, the currently running loading/progress/task page must already be stopped or unwound. Never open a prompt or modal from inside an active task/progress callback. If a running operation needs user input, return a structured recoverable result to the outer workflow, stop the task page first, and only then show the prompt.
  - keep large working contexts such as search, table selection, and multi-line editing as full-screen pages
  - modal overlays should sit on top of the current tview shell/pages instead of dumping traditional text or switching to a plain terminal prompt
  - modal overlays must reuse the most recent real full-screen page as their background; never use an empty placeholder page behind a modal unless no previous page exists, including loading/progress/status task modals
  - loading/progress/status modal descriptions must live inside the modal frame while still overlaying the previous full-screen page. Do not render `TaskPage.Description`, "Fetching...", "Loading...", or similar explanatory text outside or behind the modal box; the whole modal content belongs inside the bordered dialog, and an empty background must never be used to hide misplaced content.
  - all modal explanatory text, prompts, choice lists, form fields, and buttons must be inside the modal's bordered dialog. Never render a modal `Message` or prompt sentence above/outside the modal frame.
  - modal overlays must capture keyboard and mouse before lower layers; lower-layer buttons must not receive clicks while a modal is open
  - design the modal layer as a stack even if most flows only show one modal at a time; the topmost modal owns input
  - implement modal layering through `tview.Pages` plus a full-screen overlay primitive that draws only the centered dialog but consumes all mouse events outside and inside the dialog unless the dialog itself handles them
  - modal dialogs must not show global Back/Home/Quit navigation buttons by default
  - simple information/validation modals should show only the primary action on the far right, usually `OK (Enter)`
  - action/recovery modals should put this modal's own actions on the left, for example Retry, Skip, Back, Install, or Exit, and keep the primary/default action on the far right
  - modal dismiss actions must be labeled `Close`, not `Back`; `Close` only dismisses the current overlay and returns to the underlying page/state
  - Esc inside a modal must close only the topmost modal and return to the underlying full-screen page
  - do not turn a modal close into a workflow error page; the parent workflow may decide to stay on the current page, retry the same step, or explicitly navigate, but that decision must not be surfaced as an error
  - short action sets should use buttons; long final-operation prompts whose choices need explanatory secondary text should stay as a single-choice list with an added `Close` button
  - modal primary buttons must map to their own explicit action value; never let the far-right primary button accidentally trigger the first left-side action
  - BLAST loading states must describe the actual step, such as capability detection, online BLAST submission, local fallback check, BLAST+ installation, local BLAST execution, or result polling
  - when a loading task has no known total, use a modal spinner/status dialog; when a meaningful total exists, use a modal progress dialog
  - every loading/progress/status task modal must include a left-side `Cancel (Esc)` button. Cancel means abandon the running task, cancel its context when possible, close the modal, and return to the relevant previous workflow page without showing a workflow error modal.
  - BLAST fallback flow is TUI-owned: try online BLAST explicitly when available, show a modal decision if online BLAST fails or cannot produce a usable automated result, then only run local BLAST+ after the user chooses the local fallback
  - if local BLAST+ tools are missing, close the fallback decision and show a dedicated BLAST+ install modal; installation itself runs in a loading modal, then local BLAST resumes in its own progress/status modal
  - use a long single-choice list only when there are many choices or each choice needs long explanatory secondary text
  - use tview widgets such as `Pages`, `Modal`, `TextView`, and shared buttons for dialogs; do not print ad-hoc messages to stdout during interactive workflows
  - `internal/ui` stdout spinner/progress helpers are forbidden for interactive workflow code; remove the package if it exists only for stdout UI
- Export settings:
  - entering the file-generation/export subflow must open a modal dialog over the current result table, not a new full-screen page
  - collect export file name, optional output folder, and data analysis report choice in one larger modal when export begins
  - omit the output-folder module when that mode does not use a folder; when present, leaving it blank means write files directly to the program output directory
  - checkbox-style choices, including the data analysis report choice, use the shared `[x]` / `[ ]` selection style: selected/on is green, unselected/off is red, and Space toggles the focused checkbox
  - the export settings modal uses stacked modules like the startup page: Tab switches modules, Enter advances modules, and Enter on the final module activates the primary export action
- Keyboard capture rules:
  - do not solve duplicate input by timing-based event dropping or "eat repeated key" filters
  - do not add custom Windows screen selection, custom TTY wrappers, or outgoing escape-sequence filtering unless a focused reproduction proves the default tview/tcell path cannot work
  - prefer the `fen` pattern: default tview application, default tcell screen, mouse enabled, paste enabled, and no custom screen/TTY wrappers
  - do not attach ad-hoc global key capture to text input or multi-line input pages unless absolutely necessary
  - ordinary text entry must be handled only by the focused tview input widget
  - text input pages should use widget-local handlers such as `InputField.SetDoneFunc` or widget `SetInputCapture` for widget-specific behavior, following `fen`; app-level capture is only for global Ctrl navigation
  - do not install no-op Tab captures; only intercept Tab on pages with multiple focusable modules where Tab has an explicit app-level focus-switching meaning
  - navigation on input pages should primarily use real buttons and Ctrl shortcuts, avoiding duplicate key processing
  - in multi-line text areas, Enter inserts a new line; the main/progression action must only use a key event that is explicitly `KeyEnter` with `ModCtrl`
  - do not treat `KeyCtrlJ` or `KeyCtrlM` as multi-line confirmation because many terminals encode ordinary Enter or pasted newlines as those values
  - never bind plain letter shortcuts on input pages; use Ctrl shortcuts for navigation and secondary actions, and Enter for the page's primary action, including multi-line input pages
- Paste behavior:
  - keep the default tview/tcell paste path provided by `EnablePaste(true)` and the focused input widget
  - input, multi-line input, and search pages may also expose a Paste button plus `Ctrl+V` as an explicit app action that reads the system clipboard and inserts through the focused widget's paste handler
  - the app-level Paste action must not use a modal loading overlay, must not swap the page root, and must not move focus away from the active input widget while reading the system clipboard
  - input, multi-line input, and search pages should reserve one inline status line directly below the input area for Paste feedback; show reading/failure text there, clear it after a successful paste, and refresh stale failure text on the next paste attempt
  - pasted text must continue to pass through the shared clipboard read and sanitization path before insertion, including UTF-8 validation, newline normalization, ANSI escape stripping, control-character filtering, and empty-content validation
  - the app-level Paste action must not replace or disable normal terminal paste behavior
  - do not fix Paste focus problems with page-level focus locks or broad app-level mouse/key capture; keep focus behavior close to tview defaults and fix the Paste flow itself
  - `Ctrl+Shift+C` is reserved for row-selection table cell copy. Do not bind `Ctrl+Shift+V` because terminals often reserve it for paste.
- Color rules:
  - centralize all TUI colors in the shared theme constants
  - canvas/background: black
  - normal panel/select background: dark blue
  - normal text: ghost white
  - secondary/help text: yellow
  - accent/status text: green
  - inactive borders and titles: white
  - focused borders, focused titles, selected text, and primary/progression buttons: the same bright blue
  - primary/progression button text: black for contrast
- Use one consistent page pattern for interactive steps:
  - a short page title that names the step
  - one or two lines that explain what the user is doing
  - the available choices or table
  - one prompt line that matches the page title
  - the global navigation hint on every page
- Use one consistent help-page pattern:
  - prefix help pages with `Help - <topic>`
  - keep help text short, concrete, and action-oriented
  - explain command meaning instead of repeating the prompt verbatim
- Never show bare command lists when a short explanation fits. If a page offers commands, each command must include a one-line meaning.
- Keep wording consistent across similar pages:
  - use `Function selection`, `Sub-option selection`, `BLAST program selection`, `BLAST execution target`, `Keyword input`, and `BLAST input` as the canonical page titles
  - use `Select ...` for choice prompts and `Enter ...` for free-form text prompts
  - prefer `Review the summary and press Enter to continue.` for confirmation pages
  - prefer sentence-case labels and avoid mixing labels like `Choose ...`, `Pick ...`, and `Enter ...` on the same type of page
- Keep the global navigation controls visible on every interactive page:
  - `back` returns to the previous page
  - `home` returns to database/startup selection
  - `exit` quits the wizard
- Use clear TUI section boundaries between sessions and major phases:
  - show session identity and outcomes inside tview pages or modal overlays
  - never print wizard session banners, separators, or completion messages to stdout
  - keep repeated prompts grouped so the user can immediately tell where they are in the flow
- The program UI is English-only. Do not add runtime language switching or executable-name locale behavior. Use explicit multi-language text only in deliberately scoped scientific help surfaces, such as column details or InterPro parameter help.
- For blank/Enter-only branches, return an explicit placeholder value when the caller expects a slice or list, rather than returning `nil` and relying on downstream indexing
- Prefer descriptive labels over terse jargon:
  - `File name` is preferred over localized or ambiguous labels
  - query input prompts should say exactly what kinds of inputs are accepted
  - list/selection pages should say what the commands do before asking for input
- BLAST batch input rules:
  - accept one query per line, FASTA entries, Phytozome report URLs, or a keyword list copied from `list`
  - accept `load "file.txt"` from the program directory as a batch input source
  - when more than one query is supplied, validate that pasted label names are either present for all items or omitted for all items
  - allow `~` as the explicit blank placeholder for per-query labels
  - after external-reference settings, detect Family BLAST/query-group patterns generically, such as `NAME1/NAME2/NAME3`, `PREFIX10/PREFIX10-like`, or `GROUP.1/GROUP2`; if at least two queries share a detected family prefix, open the Family BLAST settings modal with Family BLAST enabled by default
  - Family BLAST does not require external references to run; it groups the review/export unit by gene family while still executing BLAST per query. External references still control evidence columns, and the automatic filter remains gated on full external-reference availability
  - Family BLAST must not rewrite or normalize hit-row `label_name` values. It groups by BLAST query/source labels and query gene identity, then derives a separate `family_name` for the grouped review/export unit. Remove trailing member numbering by default. Before that, default-on suffix handling should treat `prefix + number + suffix` query labels such as `GENE10-like` or `GENE10_like` as `GENE10`, so `GENE9/GENE14/GENE10/GENE10-like` group as `GENE`.
  - in Family BLAST, grouped output filenames use the derived family name without member numbering, for example `GENE` instead of `GENE1`, `GENE2`, `GENE3`, `GENE4`; rows are merged by target protein/gene by default, keeping the best hit when one target is hit by multiple member queries
  - Family BLAST export should preserve the grouped family file while including all available query sequence headers for the family, not only the first member query
  - ask for missing per-query source `label_name` values before BLAST execution when they are needed for multi-query review, Family BLAST grouping, query-source metadata, and TXT query headers; this source-label flow is separate from hit-row labelname auto-identification in the external-reference modal
  - ask for an optional output folder name only inside the file-generation subflow for batch BLAST export
  - process and confirm BLAST result tables one query at a time
  - keep the BLAST selection table visible during row confirmation, including checkbox markers and row numbers
  - in multi-file BLAST result tables, the left query sidebar shows the gene ID as the primary text. Its secondary yellow text is two lines: first the `label_name`, then the result count such as `5 lines`.
  - support `Export all` / `Ctrl+D` to generate files for the current selection and auto-generate remaining batch queries with default selections
- Keyword mode ordering:
  - ask for keyword `label_name` values in the main keyword review flow, before running the keyword search
  - preserve one-to-one ordering between keyword terms and `label_name` values
  - allow `~` to mark an intentionally blank `label_name`
- Batch query resolution:
  - use a progress bar for batch URL resolution instead of repeating per-item spinner messages
  - keep single-query resolution feedback concise and non-duplicative
- Confirmation wording:
  - use concise confirmation prompts like `Review the summary and press Enter to continue.`
  - avoid wording that implies files are being created before the user confirms
  - confirmation pages should not say `generate files` or similar before the user presses Enter
- Output naming:
  - do not add artificial numeric prefixes to export filenames or query FASTA headers
  - preserve the user-entered query label as the primary visible name
  - when a query comes from a report URL, keep the original pasted URL visible in export metadata
  - do not overload `gene_report_url` in result rows with query-source URLs or annotation text; row-level `gene_report_url` is reserved for real target-side links only
- Keep fallback paths explicit:
  - tell the user when an automatic fallback is happening
  - tell the user when a path requires local BLAST+ or downloaded data
  - always provide a retry or back/exit route when a step can fail
  - when a workflow step can be recovered from safely, surface `back` explicitly in the prompt text rather than relying only on the global hint
- Error handling:
  - use `retry`/`skip`/`back`/`exit` patterns whenever the underlying action can support them safely
  - fetch-style errors should always offer `retry`, `skip`, `back`, and `exit` with clear page-specific back targets
  - keep fetch errors skippable where possible
  - batch query resolution and batch per-query execution should fail one item at a time, not fail the whole batch by default, when a safe `skip` path exists
  - if a skip path is unsafe for a step, document that in the prompt and keep the recovery path explicit
- Loading feedback:
  - use a TUI modal progress bar whenever the code knows the number of items being processed
  - use a TUI modal status/progress overlay for single-item or unknown-duration work
  - do not leave long network/file operations without visible TUI feedback
- Modal navigation for external-reference-like scientific parameter windows:
  - top-level modules are switched with `Tab`/`Backtab`
  - inside a module, use `Up`/`Down` only; reserve `Left`/`Right` for text-input cursor movement
  - `Enter` moves to the next top-level module, and only the final module's `Enter` performs the primary action
  - clicking the primary action button performs the action immediately
  - every such modal should include a `Help (F1)` button using the shared three-language help template; remember the last help language for the current session
  - text and controls must wrap or use vertical module layout rather than overflowing horizontally; modal height should fit actual content without unnecessary dead space
- Performance and concurrency:
  - all new network clients must use `internal/netconfig.DefaultHTTPClient()` or receive the shared workflow client; do not use `http.DefaultClient` or construct ad-hoc `http.Client` values in production code
  - global network fanout must come from `internal/netconfig.DefaultNetworkWorkers()` / `NetworkWorkerCount(total)`, which currently defaults to `max(GOMAXPROCS*16, 96)` and can be raised with `PHYTOZOME_GO_MAX_WORKERS`; HTTP idle pools use the same shared defaults and `PHYTOZOME_GO_MAX_IDLE_CONNS*` overrides
  - all new ordinary batch-safe parallel loops should reuse the existing shared/cancellable worker helpers where available, or the package's `netconfig`-backed worker count plus first-error cancellation, so dynamic worker counts, context cancellation, and first-error cancellation stay consistent
  - recovery-aware workflow pipelines such as BLAST batch execution or export-all may keep specialized worker loops, but they must use shared `netconfig`/workflow worker counts, propagate `context.Context`, cancel sibling workers on first fatal error, preserve completed results, and return a recovery error whose index resumes from the first incomplete item
  - every long-running loading/generation/analysis path must accept or derive a cancellable `context.Context`; cancellation must pause/stop the current action and return to the expected TUI recovery/back target without leaking background workers
  - prefer controlled parallelism for batch-safe stages such as keyword searches, batch query resolution, and sequence fetch preloading
  - keep all interactive prompts serialized even when background work is parallelized
  - apply per-item timeouts to remote work units so a single slow request cannot stall the whole batch indefinitely
  - preserve input order in final tables and exports even when work completes out of order
  - deduplicate identical in-flight work with `singleflight` or equivalent guards so concurrent workers do not download, parse, or build the same artifact twice
  - size auto-label and alias-supplement worker pools by the number of items that actually need work, not by the total row/query count when many items already carry labels or reusable aliases
  - for BLAST downstream auxiliary stages, do not blindly reuse the global `96+` network worker budget. Keep phase-specific bounded defaults and only raise them with fresh measurements:
    - UniProt row enrichment default worker cap: `12`
    - UniProt accession prefetch default worker cap: `16`
    - InterPro row enrichment default worker cap: `10`
    - BLAST query/source auto-label and alias-supplement default worker cap: `min(cpu*2, 24)` with a floor of `4`
    - BLAST keyword-term prefetch default worker cap: `min(cpu*2, 24)` with a floor of `4`
    - BLAST sequence fetch default worker cap: `min(cpu*2, 20)` with a floor of `4`
  - keep environment overrides for each BLAST auxiliary phase (`PHYTOZOME_GO_BLAST_UNIPROT_WORKERS`, `PHYTOZOME_GO_BLAST_UNIPROT_ACCESSION_WORKERS`, `PHYTOZOME_GO_BLAST_INTERPRO_WORKERS`, `PHYTOZOME_GO_BLAST_LABEL_WORKERS`, `PHYTOZOME_GO_BLAST_KEYWORD_TERM_WORKERS`, `PHYTOZOME_GO_BLAST_SEQUENCE_FETCH_WORKERS`) so live sweeps can still push higher where evidence supports it
  - reuse BLAST-hit labelname identification results by stable hit/source keys within a run so duplicate HSPs, duplicate target rows, and family-merged tables do not repeat local labelname ranking or fallback work
  - cache UniProt and InterPro lookup results across BLAST runs in the same wizard session, including negative lookups, so repeated hits from related query sources do not repeat external-reference calls
  - remote Phytozome BLAST should default to a small bounded batch concurrency of 2; live replay checks showed 2 workers modestly improves throughput while 3 workers slows down the remote target
  - inside Phytozome keyword lookups, run independent identifier variants and alias fallbacks with bounded network parallelism, but merge results in the original priority order so faster requests never change biological/search semantics
  - cache duplicate labelname batch-ranking requests within a run so repeated alias lists are sorted once while preserving each request's task timestamp and item index in the returned result
  - live keyword -> BLAST throughput is still not monotonic with larger worker caps, but the best setting changes when the full downstream pipeline is enabled. Old single-path sweeps that stopped at BLAST-only stages are no longer sufficient after enabling automatic query `labelname`, BLAST-hit `labelname`, UniProt, and InterPro together.
  - for keyword-result-table -> BLAST full-chain live sweeps, always test with all expensive downstream stages enabled together:
    - keyword auto `labelname`
    - BLAST query/source auto `labelname`
    - BLAST-hit auto `labelname`
    - UniProt enrichment
    - InterPro enrichment
  - current full-chain live matrix evidence for the four keyword/blast combinations (`phytozome->phytozome`, `phytozome->lemna`, `lemna->phytozome`, `lemna->lemna`) showed:
    - `PHYTOZOME_GO_MAX_WORKERS=32` total about `62633 ms`
    - `PHYTOZOME_GO_MAX_WORKERS=64` total about `58564 ms`
    - `PHYTOZOME_GO_MAX_WORKERS=96` total about `53129 ms`
    - `PHYTOZOME_GO_MAX_WORKERS=128` total about `49124 ms` in one full pass, but later reruns varied upward
    - `PHYTOZOME_GO_MAX_WORKERS=192` gave very fast `phytozome->lemna` and `lemna->lemna` runs in one pass, but could regress badly on the remote-Phytozome combinations on rerun
    - `PHYTOZOME_GO_MAX_WORKERS=224` and `256` stayed competitive, but the best point still drifted run-to-run because the remote services dominate variance
  - practical rule from the latest full-chain sweeps: do not trust old BLAST-only worker conclusions for the complete keyword->BLAST workflow. Re-measure on the exact stage mix being optimized, and treat roughly the `96-256` band as the current search space for full-chain runs with all downstream enrichments enabled.
  - keep `internal/labelname` independent from workflow/search row model types; workflows collect candidates and metadata, while labelname only ranks generic alias requests and returns ranked aliases
  - remove obsolete helper paths after moving behavior into labelname or workflow-owned candidate collection; do not keep compatibility wrappers that only reintroduce old ownership boundaries
  - use persistent local caches for stable remote payloads when that improves later sessions without changing user-visible behavior
  - prefer atomic file writes for shared cache artifacts so background workers cannot observe partial files
  - avoid deep-copying large BLAST/keyword row slices unless crossing a mutable ownership boundary; report and export assembly should pre-size result buffers and reuse already-stable slices where behavior stays unchanged
  - keep `internal/netconfig` tests as the baseline guardrail for shared HTTP and worker defaults; if a broader `internal/perf` layer is later added, migrate clients/loops deliberately instead of reintroducing scattered constants
- Metadata and result-link semantics:
  - preserve `OriginalInputURL` and `NormalizedURL` when resolving report URLs; do not overwrite them with fetched source structs
  - keep export metadata for query-source URLs separate from row-level target links
- Lemna local BLAST performance:
  - cache release-level AHRD, protein->transcript maps, and FASTA indexes in memory for the current client
  - reuse cached BLAST-ready FASTA artifacts and build the index only once per path
  - avoid duplicate scans of the same release assets within one run
  - production networked BLAST/keyword/reference paths must not impose hard client/request timeouts for slow links; cancellation must come from the task context so ESC/close can always stop the underlying work cleanly
  - managed `blast+` bootstrap and local lemna FASTA preparation must expose real download/decompress progress in task pages, not a fake spinner-only status
  - cache the managed `blast+` archive itself under the app-local tools directory so retries and repeated installs can skip the network when the archive is already present
  - for multi-query local BLAST, prefer a small number of BLAST processes plus higher `-num_threads` per process instead of opening one process per CPU core by default
  - current default local BLAST scheduler rule is evidence-based from four-program real BLAST+ sweeps and later batch/export replay:
    - `blastx` default local batch workers: keep `1` unless the user explicitly overrides; the current machine's replay showed `2 x 8` regressed versus `1 x 8`
    - `blastn` default local batch workers: prefer `2` when CPU >= 8 and batch size >= 2; current replay improved from about `44 s` at `1 x 4` to about `8 s` at `2 x 2` / `2 x 4`
    - `tblastn` default local batch workers: prefer `2` when CPU >= 8 and batch size >= 2; current replay improved from about `75 s` at `1 x 4` to about `27-30 s` at `2 x 2` / `2 x 4`
    - `blastp` default local batch workers: prefer `2` on larger CPU budgets and multi-query batches, but re-measure because gains are smaller and can drift with remote enrichment variance
    - current thread caps by program/worker shape:
      - `blastx`: cap `8`
      - `blastp`: cap `8`
      - `blastn`: cap `4` with one worker, cap `2` when batch workers >= 2
      - `tblastn`: cap `4` with one worker, cap `2` when batch workers >= 2
  - every shared BLAST execution entry point must align prepared query sequences to the configured BLAST program before submission, not only the top-level interactive wizard path:
    - `blastn` / `blastx` must convert query items to DNA using the selected source's nucleotide resolver/cache path
    - `tblastn` / `blastp` must convert query items to protein using the selected source's protein resolver/cache path
    - direct callers such as replay/performance tests must get the same sequence-kind correction as interactive runs
  - `lemna` nucleotide query preparation must use the release FASTA chosen for the current local program and cache parsed nucleotide sequences on disk/in memory just like protein FASTA
  - `lemna` capability detection must self-initialize species/release metadata before reading cached release maps; do not require callers to fetch species candidates first
  - local `makeblastdb` reliability on Windows must be defensive:
    - local cached DB prefixes should stay short and stable instead of embedding the full FASTA filename verbatim
    - build local cached databases with BLAST DB version 4 to avoid fragile LMDB/v5 runtime failures in app-local cache paths
    - only treat a cached DB as complete when the full core file set exists (`.pin/.phr/.psq` for protein, `.nin/.nhr/.nsq` for nucleotide); a single leftover artifact is not a valid cache hit
    - if `makeblastdb` fails, remove partial DB artifacts before retry so half-built local caches cannot poison later BLAST runs
  - current live replay evidence for `lemna` single-query local BLAST with the repaired workflow path:
    - `local:BLASTN` succeeded on `Sp9509d006g004400_T001` with `1` batch worker and `4` BLAST threads
    - `local:BLASTX` succeeded on `Sp9509d006g004400_T001` with `1` batch worker and `8` BLAST threads
    - `local:BLASTP` succeeded on `Sp9509d006g004400_T001` with `1` batch worker and `8` BLAST threads
    - `local:TBLASTN` succeeded on `Sp9509d006g004400_T001` with `1` batch worker and `4` BLAST threads
  - current live replay evidence for `lemna` three-query batch local BLAST with query/source auto label, BLAST-hit auto label, UniProt, and InterPro enabled together:
    - `local:BLASTN` batch succeeded with exports/reports at `2` batch workers and `2` BLAST threads, about `9 rows selected` and about `8 s` full export replay
    - `local:BLASTX` batch succeeded with exports/reports at `1` batch worker and `8` BLAST threads, about `24 rows selected` and about `8 s` full export replay
    - `local:BLASTP` batch succeeded in live batch replay at `2` batch workers and `8` BLAST threads, about `25 s`; export chain also succeeded on the earlier `1 x 8` replay
    - `local:TBLASTN` batch succeeded with exports/reports at `2` batch workers and `2` BLAST threads, about `54 rows selected` and about `7 s` full export replay
  - BLAST external-reference enrichment workers should scale by enabled feature set instead of one fixed budget:
    - UniProt accession prefetch, UniProt lookup, and InterPro lookup may use more aggressive network fanout when auto BLAST-hit labelname, UniProt, and InterPro are enabled together
    - CPU/disk-heavy phases should keep their own more conservative worker caps and must not blindly inherit the network enrichment fanout
    - small keyword->BLAST replay batches should keep more conservative default network worker ceilings than large batches; live replay evidence showed that pushing UniProt/InterPro worker counts higher for 3-query / about 60-row runs made the total wall time slightly worse instead of better
  - UniProt and InterPro enrichment progress must expose explicit stage text before the row-level loop begins so long external-reference runs do not look frozen:
    - UniProt should show an accession-prefetch phase before lookup resolution
    - InterPro should distinguish query-reference resolution from hit-reference resolution
  - BLAST query/source labelname worker strategy:
    - keep the expensive remote Phytozome keyword-row fetch stage parallel and de-duplicated by term/species cache key
    - after candidate collection, batch-rank aliases instead of calling single-item rank repeatedly
    - progress text should explicitly say `Collecting BLAST source label candidates...` and then `Ranking BLAST source labels...`
  - Keyword-to-BLAST real-flow performance work should be measured separately from BLAST execution:
    - keyword search
    - keyword auto labelname
    - selected-row to BLAST-item resolution
    - BLAST query/source auto labelname + alias supplement
    - only after that, BLAST execution and external references
    - keep a dedicated live breakdown test so worker tuning can target the real pre-BLAST bottleneck instead of only the final BLAST stage
    - when `suppressTaskModals` is true, keyword-row to BLAST-item resolution should also run headless without wrapping an extra progress modal so replay/perf runs do not spend extra TUI orchestration overhead on the already-instrumented fast path
  - BLAST-hit labelname worker strategy:
    - prefetch keyword rows for all unresolved hits in one shared term batch
    - de-duplicate identical hits by stable identification cache key before ranking
    - batch-rank unresolved hit alias requests and then fan the decision back onto all duplicate rows/HSPs
    - progress text should explicitly say `Prefetching BLAST hit label candidates...`, `Ranking BLAST hit label aliases...`, and `Resolved BLAST hit label names... x/y`
  - `internal/labelname` batch ranking should avoid repeated batch-internal normalization work where behavior does not depend on the repeated recomputation; preserve exact ranking output while reducing redundant `TrimSpace`/`ToLower`/split passes
  - family BLAST post-processing must avoid recomputing the same semantic-token list and reference-score/coverage/target-key tuple inside sort/comparison hot loops; precompute once per row when ranking large result sets
  - when running replay/live export tests for `blastn` or `tblastn`, do not blindly reuse the protein-target default filter settings:
    - genome/nucleotide-target programs do not have a biologically equivalent `target_length / UniProt canonical length` surface
    - replay/export validation should derive filter settings from the BLAST program and disable canonical-length hard rejection plus InterPro conserved-region hard rejection for `blastn` / `tblastn`
    - this rule is for replay/export validation fidelity; it does not silently change the interactive user defaults
  - when `suppressTaskModals` is true, BLAST auto-label and alias-supplement paths must run headless and must not open tview task modals; otherwise long replay/live tests can deadlock the Windows tcell/tview loop
  - current stable `lemna` live replay transcript seed set for local nucleotide/protein BLAST performance tests:
    - `Sp9509d006g004400_T001`
    - `Sp9509d012g006190_T001`
    - `Sp9509d012g006280_T001`
  - keep environment overrides available for large local machines or unusually large databases, but do not raise defaults without rerunning a four-program sweep
- When adding a new command or prompt, update the prompt text, help text, and this file together so the behavior stays discoverable and consistent.

## Documentation and release standards

- `README.md` must function as a real operator guide, not a short feature list.
- The README should explain the product through step-by-step usage, realistic examples, and user-facing outcomes.
- Document both `phytozome` and `lemna` workflows in plain language, including:
  - what each step asks
  - why the step exists
  - what kind of input is expected
  - what files are generated at the end
- Prefer concrete examples such as rice or soybean gene IDs, Phytozome report URLs, copied `list` output blocks, and batch BLAST usage instead of abstract placeholders.
- Do not reuse user-provided identifiers, species combinations, pathway panels, or research-specific examples in public documentation. When writing README examples, use fresh public examples that did not come from the user's prior workflow.
- Keep screenshots and README images in a stable repository location such as `docs/images/`.
- Whenever output paths, cache paths, batch behavior, or recovery commands change, update the README and this file in the same change.
- Release packaging rules:
  - clear `bin/` before rebuilding release artifacts
  - rebuild all supported platform binaries into `bin/`
  - keep release assets aligned with the actual executable names documented in the README
  - Windows WezTerm bundles must apply `docs/logo2.png` to both the launcher executable icon and the embedded WezTerm window executable icon; keep the bundled `phytozome-go-window-icon.png` beside `wezterm.lua` as the stable source image
  - the README lead image must use `docs/logo.png`
  - prefer publishing GitHub releases with explicit release notes that summarize user-visible changes and supported platforms

## Current workflows

The wizard asks users to choose a database at startup:

- `phytozome`: original Phytozome-backed behavior
- `lemna`: lemna.org download-backed behavior

### BLAST mode (desired behavior)

- species search and single-species selection
- sequence / FASTA / report URL input
- batch query input from multiple newline-separated items, FASTA entries, or supported report URLs
- server BLAST submission and polling when server-side submission is available and parsed safely
- automatic fallback to a local BLAST workflow when server-side submission isn't available or lacks required DBs
- interactive row selection
- Excel export
- peptide text export

Notes specific to lemna:
- `lemna` downloads and keyword export are already supported.
- The public lemna.org BLAST form exposes Query Type, Database Type, and BLAST Program combinations (blastn, blastx, tblastn, blastp). Some DB selectors may not expose protein DBs publicly; treat that as a server capability constraint and offer local BLAST fallback.

### Keyword mode

- species search and single-species selection
- multi-keyword input
- grouped search results in input order
- interactive row selection
- Excel export with full result metadata
- peptide text export for selected rows

In lemna mode, keyword search reads the selected lemna.org release GFF3 and AHRD annotation archive from the public download directory, exports standard columns plus all discovered GFF3 columns, attributes, and AHRD fields. Peptide export resolves sequences from the release protein FASTA when available.

## Reliability requirements and isolation

- Keep Phytozome-specific scraping and parsing isolated in `internal/phytozome`.
- Keep lemna.org download/GFF3/FASTA parsing and BLAST form handling isolated in `internal/lemna`.
- Keep workflow code dependent on a shared `internal/source.DataSource` interface where practical so higher-level flows remain unchanged.
- Prefer stable endpoints (the download directory) over fragile homepage or JS-bundled form parsing.
- Any homepage or frontend-bundle scraping is fallback-only and must be isolated and explicit about fragility and caching.
- Avoid changing user-visible workflows unless explicitly requested.

## Design decisions for lemna.org mode

1. Authoritative source
   - The `download/` directory and release metadata are authoritative for available species/releases and downloadable assets (GFF3, AHRD, protein FASTA).
   - The public homepage is used only to mark "official clones" (the small set advertised). Do not remove releases that exist in the download directory simply because the homepage doesn't list them.

2. Species candidate listing
   - If the total candidate count is small (configurable threshold, default <= 16), present the full list directly to the user (no search required).
   - If the list is longer, present a searchable/filterable UI.
   - Mark candidates that are "official" (present in the official clone list) and optionally pin them to the top.

3. BLAST capabilities and mapping
   - Map Query Type + Database Type into BLAST program:
     - Query Nucleotide + DB Nucleotide => `blastn`
     - Query Nucleotide + DB Protein => `blastx`
     - Query Protein + DB Nucleotide => `tblastn`
     - Query Protein + DB Protein => `blastp`
   - Determine capability for each species/release:
     - `HasServerNucleotideDB` / `BlastNDBID` (server-side nucleotide DB exposed with DB id)
     - `HasServerProteinDB` / `ProteinDBID` (server-side protein DB id exposed)
     - `HasProteinFasta` / `ProteinFastaURL` (protein FASTA available in download assets)
     - `HasNucleotideFasta` / `NucleotideFastaURL` (genome/transcript/CDS FASTA available in download assets)
   - Use capability discovery to present only valid choices to users and offer fallbacks:
     - If server-side program is unavailable, suggest local BLAST using downloaded FASTA.
     - If local BLAST is requested, download the matching FASTA, run `makeblastdb` with the correct DB type, run `blastn`/`blastx`/`tblastn`/`blastp`, parse, and return unified results.

4. BLAST submission strategy
   - Prefer server submission when:
     - The server exposes the required DB id(s).
     - The server form can be parsed reliably and submission can be polled.
   - Use local BLAST fallback when:
     - Server DB is not exposed for the requested program (particularly protein DB may be absent).
     - Server form parsing/submission/result retrieval is fragile/unreliable.
   - Current implementation treats server BLAST as best-effort probing only; if automated result retrieval cannot be guaranteed, it falls back to local BLAST when matching FASTA + BLAST+ are available.
   - Always surface the capability and the chosen path to the user (explicit prompt).

5. Graceful failure and caching
   - Cache release metadata and capability detection to avoid repeated fragile parsing.
   - On submission failure, present clear diagnostic and a choice to retry via local BLAST or switch species/program.

## API surface (internal/lemna)

Add the following logical APIs (signatures adjusted to existing code style and `model` types):

- FetchSpeciesCandidates(ctx) ([]model.SpeciesCandidate, error)
  - Already implemented; ensure it:
    - Uses download/ directory as authoritative.
    - Marks `IsOfficial` on candidates matching the official clone list.
    - Populates `ProteomeID` / `BlastNDBID` if detectable.

- DetectBlastCapabilities(ctx, species model.SpeciesCandidate) (BlastCapability, error)
  - BlastCapability {
      HasServerNucleotideDB bool
      BlastNDBID int
      HasServerProteinDB bool
      ProteinDBID int
      HasProteinFasta bool
      ProteinFastaURL string
      HasNucleotideFasta bool
      NucleotideFastaURL string
    }
  - Use cached `releasesByJBrowseName` where possible.
  - Parse the concrete BLAST selector pages only if necessary to find DB ids.

- AvailableBlastPrograms(species model.SpeciesCandidate) ([]string)
  - Returns allowed programs based on capability and valid Query/DB combos.

- SubmitBlast(ctx, req model.BlastRequest) (model.BlastJob, error)
  - If server submission is chosen:
    - Build form payload and POST to lemna endpoints.
    - Handle Drupal/Tripal hidden fields, tokens, and possible redirects.
    - Persist job id and return standardized `model.BlastJob`.
  - If chosen local submission:
    - Download FASTA (if needed), build BLAST DB with `makeblastdb`, run appropriate `blast+` command, parse and return results.

- WaitForBlastResults(ctx, jobID, pollInterval, timeout) (model.BlastResult, error)
  - For server jobs: poll the site endpoints until results are available or fail safely.
  - For local DB runs: return the parsed result when command completes.

- SearchKeywordRows(ctx, species, query) ([]model.KeywordResultRow, error)
  - Already present; ensure it supports resolving sequences using the downloaded protein FASTA.

Notes:
- Keep all fragile HTTP parsing and form submissions inside `internal/lemna`; expose stable capability and job APIs to the rest of the app.
- If server BLAST requires JS or heavy dynamic behavior and can't be parsed, mark capability as unavailable and prefer local BLAST.

## CLI UX and interactive flow (lemna mode)

1. Mode selection
   - User selects `lemna` at startup.

2. Species selection
   - Call `FetchSpeciesCandidates()`.
   - If candidates <= threshold (default 16): list candidates numbered with label, common name, release date, and tags: `[official] [has-protein-fasta] [server-blastn]`.
   - If larger: present search prompt (support fuzzy search via `FilterSpeciesCandidates`).

3. Display capability summary for chosen species
   - Show available BLAST programs (server / local) and recommended defaults.
   - Example:
     - Available: `blastn (server)`, `blastx (server)`, `blastp (local only; protein FASTA available)`

4. BLAST configuration prompts
   - Query Type (Nucleotide / Protein) — prefilled based on input sequence if possible.
   - Database Type (Nucleotide / Protein) — only allowed combos shown.
   - Program — inferred or chosen from available programs.
   - Advanced options (optional): e-value, max hits, filter, matrix, word size.
   - If the selected program requires a capability not provided by the server, prompt:
     - "Server does not expose required DB. Run local BLAST using downloaded FASTA? (yes/no)"

5. Submission and results
   - If server submission chosen: try the concrete Tripal/Drupal BLAST form, but fall back to local BLAST unless job id and result retrieval are both reliable.
   - If local BLAST chosen or used as fallback: download FASTA, run `makeblastdb` + `blast+`, show progress.
   - Present results in the same UI and allow export.

## Implementation tasks and priorities

Priority 0 (must do before release)
- Update `AGENT.md` (this file) with the lemna design and plan.
- Ensure `internal/lemna.FetchSpeciesCandidates` continues to use download releases as authoritative and marks official clones.
- Add `IsOfficial` marker to `model.SpeciesCandidate` or use `SearchAlias` consistently to surface official status in CLI displays.

Priority 1 (high)
- Implement `DetectBlastCapabilities` and `AvailableBlastPrograms`.
- Update CLI species selection to:
  - Present full list when small.
  - Pin/mark official clones.
  - Display capability summary after species selection.

Priority 2 (medium)
- Implement robust server-side `SubmitBlast`:
  - Parse required form elements, tokens, and database selectors where stable.
  - Provide robust error handling and fallbacks.
- Implement `WaitForBlastResults` for server jobs where possible.

Priority 3 (medium)
- Implement local BLAST fallback utilities:
  - Download FASTA asset.
  - Run `makeblastdb` and `blast+` commands where available.
  - Parse blast output into `model.BlastResult`.
  - Provide informative prompts if `blast+` is missing.

Priority 4 (low / optional)
- Integration tests that exercise both server submission and local fallback.
- CI steps to verify behavior when `blast+` isn't available (skip or simulate).
- Documentation updates with screenshots and example CLI sessions.

## Testing and validation

- Unit tests:
  - `FetchSpeciesCandidates` using sample HTML of `download/` structure.
  - `DetectBlastCapabilities` with mocked release metadata.
  - `AvailableBlastPrograms` for capability matrix permutations.
- Integration tests:
  - Simulate BLAST form submission via recorded HTML if server parsing is fragile.
  - Local BLAST end-to-end run when `blast+` available in test environment.
- Before release:
  1. run `gofmt`
  2. run `go test ./...`
  3. run `go vet ./...`
  4. build release artifacts
  5. verify expected CLI behavior (species listing, BLAST choices, file exports)

## Risks and mitigation

- Fragile server form parsing (Drupal/Tripal/JS-driven UI)
  - Mitigation: cache capabilities from download metadata; treat form parsing as fallback; provide local BLAST fallback.
- Server DB selectors may not expose protein DBs
  - Mitigation: rely on protein FASTA downloads for local BLAST.
- External site changes
  - Mitigation: keep parsing logic isolated and short-lived caches; provide clear error messages and fallbacks.
- Local BLAST dependencies missing on user machine
  - Mitigation: detect presence and offer "download only" option or instruct user to install `blast+`.

## Output behavior

- Exported `.xlsx`, `.txt`, and PDF data analysis reports are written under an `output/` directory next to the executable being run.
- If the user requests an extra output folder, create it inside `output/`, not beside the executable.
- Runtime caches must live under a single hidden-capable `.cache/` directory next to the executable, not scattered across OS temp or user cache roots.
- Cache layout rule:
  - `appfs.CacheRoot()` is the single source of truth for the app-local cache root
  - startup cache reset clears the whole app-local `.cache` tree for the new run; do not add side caches outside that root
  - modules may also expose narrower subtree cleanup for known-bad artifacts, but that is a supplement to the startup reset, not a replacement
- Lemna local BLAST assets should live under `.cache/lemna/localblast/<jbrowseName>/<release>/`.
- Persistent Phytozome caches should live under `.cache/phytozome/...`.

## Current implementation status

- Done:
  - Startup database selection.
  - `source.DataSource` abstraction.
  - lemna species discovery from `download/`, with official clone marking.
  - Small lemna species lists are shown directly instead of forcing search.
  - lemna keyword search from GFF3 + AHRD with dynamic export columns.
  - lemna BLAST capability detection from download assets and concrete BLAST selector pages.
  - local BLAST fallback for `blastn`, `blastx`, `tblastn`, and `blastp` using the matching nucleotide/protein FASTA.
  - BLAST submission errors support retry, back to BLAST program selection, or exit.
- Remaining:
  - Robust server-side lemna result retrieval. Server form probing exists, but local BLAST remains the dependable execution path.
  - Optional integration tests against live lemna.org and local BLAST+ when available.

---
End of agent notes.

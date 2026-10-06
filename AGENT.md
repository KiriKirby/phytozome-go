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

## PLAZA Gene locus priority for NCBI keyword mode

- In the main NCBI keyword interface, show a Gene locus priority dropdown immediately after `Search type` only for NCBI search types that expose the Gene locus column. Its default is `None`, followed by `Use NCBI database` and `Use PLAZA database`.
- With `Use NCBI database`, each input row with a non-empty Gene locus queries the selected NCBI search type by Gene locus first; with `Use PLAZA database`, it queries PLAZA first. In either mode, a miss or unavailable priority lookup falls back to that row's ordinary NCBI search term. Rows without a Gene locus always use the ordinary NCBI search term.
- Use PLAZA's public versioned FTP data (`plaza_public_dicots_05`, `plaza_public_monocots_05`, `plaza_diatoms_01`, and `plaza_pico_03`) rather than browser-page scraping. The primary website is Cloudflare-protected and must not be treated as a machine API.
- Resolve Gene locus values through `IdConversion` records, enrich matches from `Descriptions`, and load the selected-transcript protein FASTA on demand. Cache downloaded source files and per-locus lookup results under `.cache/plaza/`; coalesce duplicate lookups and use a bounded remote worker pool.
- PLAZA-derived rows must identify `SourceDatabase` as `plaza`, `SearchType` as `PLAZA Gene locus priority`, preserve provenance in `plaza_*` extra columns, include the source FASTA inline for downstream exports/Canvas, and leave `gene_report_url` empty unless PLAZA returned an exact row-level page URL.
- A keyword result set that contains one or more PLAZA-derived rows must use the shared PLAZA/NCBI Gene schema in its table, details, exports, and reports. This schema retains the standard NCBI-compatible fields (`search_type`, Gene locus, identifiers, genome, description) plus PLAZA protein, transcript, UniProt, source-database, and provenance data, so a mixed PLAZA/NCBI fallback batch never loses columns based on row ordering.

## Top-level goals for phylogenetic tree mode

- Add a first-class phylogenetic tree workflow centered on Canvas mode.
- Treat the custom `mega-phgo-runtime` built from the MEGA 12.1 source tree as the only in-product computation engine for tree work:
  - sequence alignment must run through the custom runtime only
  - tree inference must run through the custom runtime only
  - supported alignment choices should mirror the runtime-backed MEGA surface exposed by the product: Protein mode exposes ClustalW and MUSCLE amino-acid alignment; DNA mode exposes ClustalW (DNA), MUSCLE (DNA), ClustalW (Codons), and MUSCLE (Codons)
  - ClustalW is linked from the MEGA 12.1 source tree; MUSCLE is the MEGA-distributed MUSCLE binary bundled inside the PHgo runtime folder and launched only by `mega-phgo-runtime`
  - Runtime discovery for tree work is strict: only the application-local bundled PHgo runtime files next to the running app are valid on Windows amd64. Do not search `PATH`, do not use installed MEGA/MEGA-CC folders, and do not accept a system-managed runtime location. Expanding the Canvas tree tool panel and refreshing a tree must both verify `<application-dir>/mega-phgo-runtime.bin` by PHgo probe plus the runtime-owned MUSCLE binary `<application-dir>/muscleWin64.bin`.
  - Windows amd64 release bundles must ship with the PHgo runtime files already present at the application root, next to the main executables, just like the bundled WezTerm runtime model. The application must not download or self-install the PHgo runtime from GitHub Releases at runtime.
  - Linux and macOS must report that PHgo system-tree is currently unsupported instead of attempting any runtime download, install, placeholder lookup, or manifest-driven recovery flow.
  - Runtime freshness is defined by the files bundled in the current Windows release package. Do not add or depend on a separate runtime marker/version file for the PHgo runtime; the bundled files are acceptable only when the PHgo probe and runtime-owned MUSCLE check pass.
  - FASTA selected for tree analysis must be handed to `mega-phgo-runtime` with only narrow runtime-format normalization: PHgo strips one or more terminal `*` markers from every sequence before writing the MEGA runtime FASTA because a final `*` is a terminator marker and has no alignment-site meaning for PHgo's tree computation. PHgo must not strip internal `*`, replace residues, pad sequence content, reverse-translate, infer biological targets, or otherwise repair protein/nucleotide data; let MEGA alignment/distance/tree components accept the data or emit their own runtime error.
  - Canvas tree target mode is a first-class tree setting. Protein mode should select/routinely request protein alignment methods, and DNA mode should select/routinely request nucleotide or codon alignment methods, but PHgo must not biologically infer, convert, skip, or repair sequence contents before runtime execution. Let the MEGA runtime alignment/distance/tree path accept the selected data or emit its own error.
  - The Canvas tree panel target is authoritative. Choosing Protein mode must show and run only the protein ClustalW/MUSCLE methods; choosing DNA mode must show and run only ClustalW (DNA), MUSCLE (DNA), ClustalW (Codons), and MUSCLE (Codons). Refresh must normalize stale/snapshot methods to the selected target before launching the runtime.
  - Protein-to-DNA tree input is only valid when PHgo already has a real nucleotide sequence for that row, either embedded in the row or resolved through the current source cache/resolver. Do not invent reverse translations from protein FASTA, do not classify protein-only rows by sequence content, and do not skip them before MEGA runtime execution; MEGA runtime errors should be surfaced directly.
  - Tree execution must treat `mega-phgo-runtime` as authoritative for correctness and failure. PHgo should forward the exact runtime error text and artifacts, avoid local biological validation or correction, and only manage orchestration, snapshots, and viewer refresh around the runtime output.
  - `mega-phgo-runtime` tree inference must call MEGA 12.1 source components for distance calculation, Neighbor-Joining, Minimum Evolution, UPGMA, Maximum Likelihood, Maximum Parsimony, bootstrap, and Newick export. Do not keep PHgo-local pairwise-distance, tree-search, bootstrap, or Newick-recursion implementations.
  - Tree artifact reuse is allowed only when the runtime artifacts are present and the runtime manifest/fingerprint state indicates the same compute inputs. Do not perform Go-side biological-target validation or kind correction before reuse; rely on the runtime outputs and manifest consistency instead.
  - In DNA mode, FASTA rows may use a real nucleotide/CDS sequence only when it is already embedded or resolved from row metadata/cache. PHgo must never reverse-translate, must not infer sequence kind from letters, and must not pre-skip rows based on biological compatibility; MEGA runtime is the authority for alignment/tree failure.
  - If `mega-phgo-runtime` writes `error_text` in `runtime-response.json`, surface that exact runtime error before looking for aligned/Newick artifacts so users see the real computation failure instead of a secondary missing-file message.
  - When `mega-phgo-runtime` reports skipped records, such as no-common-sites sequence pairs, the recovery dialog's Skip action must uncheck the exact runtime-mapped Canvas rows and immediately retry the refresh with the remaining selection. Runtime taxon ids such as `PHGOT000089` must be resolved against the selected-row order that generated the failed runtime request before unchecking so duplicate Canvas titles or UI order changes do not cause the wrong row to be skipped. After the user chooses Skip once, the same refresh should keep automatically unchecking additional rows explicitly reported by `mega-phgo-runtime` until the tree succeeds or no newly selected row can be unchecked; do not repeatedly prompt for each subsequent runtime-reported bad pair. Runtime errors that identify pairs with either `sequence pair (X,Y)` or `between sequences X and Y` must both be mapped back to the two PHgo records in that failed request.
  - Windows release assets keep the PHgo runtime executable as `mega-phgo-runtime.bin` for packaging, but PHgo must never directly execute/open that `.bin` as the final runnable. Runtime probes and tree runs must pass through `megaphgo.PrepareExecution`, which copies the bundled `.bin` to a temporary `mega-phgo-runtime.exe` next to the runtime-owned `muscleWin64.bin` and then launches that `.exe`. Source/runtime development probes may use `_mega_source/MEGA12.1-source/PHgoRuntime/lib/x86_64-win64/mega-phgo-runtime.exe` directly.
  - The desktop regression snapshot `C:\Users\wangsychn\Desktop\4CLtree.pgo` must remain a real-runtime probe. It previously exposed a ClustalW failure caused by terminal `*` markers in the runtime FASTA. PHgo must strip terminal `*` markers before launching MEGA, keep internal `*` characters runtime-visible, and must not auto-rerun with MUSCLE or another tree method on the user's behalf.
- Treat tree rendering, preview, styling, and export as a separate browser-based subsystem with its own documentation set:
  - do not attempt to render phylogenetic trees directly inside the terminal UI
  - do not embed or depend on MEGA GUI widgets/components inside the Go TUI application
  - the TUI should launch and control a local browser-facing tree viewer/editor backed by generated tree/alignment artifacts
  - each Canvas page/session owns its own local tree viewer service; leaving that Canvas page must stop its service so multiple Canvas pages can run independently without sharing one renderer process
  - startup Explore tools may also launch the same local browser-facing viewer for standalone `.nwk` inspection; each requested `.nwk` should open in its own viewer session, and leaving that standalone browser-input screen must stop the local viewer service
  - Reactree's browser page owns tree layout, search, visual editing, preview interaction, and graphical export actions
  - use the Vite/React `tree-viewer` package with the `reactreejs` npm package for the viewer; do not use the repository-local `reactree-0.1.0.tgz`, because that archive is an unrelated old generic React tree-view package
  - the tree-viewer build must patch the vendored `reactreejs` parser/exporter so quoted Newick leaf labels round-trip correctly; Canvas/Explore display names must be allowed to contain arbitrary visible characters, including underscores and punctuation such as parentheses, commas, semicolons, colons, and single quotes, without being stripped, normalized, or shown with literal quote wrappers in the UI
  - tree workflow docs should live outside the main BLAST/keyword workflow docs so the tree system can evolve independently
  - source-backed system-tree audit docs live under `doc2/phylogenetic-tree/reference/`; update that library whenever MEGA source anchors, parameter surfaces, runtime boundaries, rendering/export behavior, artifacts, snapshots, or final audit tests change
  - Reactree node-edit controls that enter an interaction mode (Reroot, Flip, Swap, and Color) remain active across completed node edits. A second click on the same toolbar command exits the mode; choosing a different mode or pressing Escape may also leave the current mode. Midpoint and ladderize are immediate one-shot commands, not modes.
- Keep the computation pipeline and the viewer pipeline loosely coupled by stable artifact formats such as FASTA, aligned FASTA, Newick, runtime request/response JSON, and structured metadata JSON.
- Canvas tree snapshots must preserve durable tree settings, the last Reactree viewer payload, last run manifest, computation fingerprints, and the core run-directory artifacts (`input.fasta`, `input.meta.json`, `runtime-request.json`, `runtime-response.json`, `aligned.fasta`, `tree.nwk`, `viewer.payload.json`, `run.manifest.json`, and runtime logs when present). Snapshot open restores those files and pushes the preserved payload back to the local Reactree service when the tree panel opens, without rerunning `mega-phgo-runtime` unless the user explicitly refreshes. Snapshots must not preserve pure UI-open state such as expanded/focused panels, current settings page, scroll offsets, transient search/menu/ribbon state, or browser viewport metadata. Canvas MSA snapshots must separately preserve MSA row states (`green`, `yellow`, `red`), the last shared tree/MSA payload and aligned FASTA, and durable Jalview state such as groups, sequence features, annotations, markers, and settings when available; the Jalview bridge synchronizes those durable fields with PHgo through `GET`/`PUT /sessions/<id>/msa/state`; PHgo must maintain exactly one selected-row set for tree and MSA. Green rows are selected and sent to both tree and MSA, yellow rows are unchecked for both tree and MSA but shown with a yellow checkbox in PHgo because the exclusion came from MSA Apply, and red rows are ordinary unchecked rows.
  - Saving a Canvas snapshot after tree analysis must synchronize the last Reactree payload metadata from the current Canvas table before writing the `.pgo`: current `display_name` values, display-name source, and per-row PHgo coordinate prefixes are preserved even if the user has not pressed `Refresh tree` again. This is a preview-only snapshot update; it must update only the preview fingerprint and must not change alignment/tree fingerprints or rerun `mega-phgo-runtime`.
  - Canvas tree/MSA display names must not be made unique by appending hidden taxon suffixes such as `[PHGOT000123]`. The former duplicate-label disambiguation mechanism is removed. PHgo always sends each selected row's raw/base display name plus a separate `display_prefix` coordinate value such as `[1,13]` to the shared tree/MSA payload. MSA always draws that coordinate prefix in front of the raw display name in Jalview's left ID list; there is no PHgo-side MSA coordinate display switch. Tree labels default to the raw display name without the coordinate prefix. The Reactree viewer's View tab, Labels group exposes a default-off `Coord` toggle between `Align` and the branch-label toggle; when enabled, Reactree renders tree leaf labels with the coordinate prefix. This viewer toggle is browser/viewer state only and must not trigger PHgo refresh, runtime recompute, or payload regeneration. Jalview's real sequence name remains the raw/base display name, while the left ID list draws the coordinate prefix in front of it. PHgo's Jalview left ID list must always draw this display text left-aligned immediately after the PHgo checkbox column, ignoring Jalview's right-align-id setting, and it must use the raw sequence name instead of Jalview's `getDisplayId(showJVSuffix)` so Jalview-generated sequence-limit suffixes such as `/1-564` are not displayed or used for PHgo name editing. The MSA left ID list also always appends a display-only per-row residue ratio suffix such as `60/64 93.8%`, calculated as non-gap/non-blank/non-terminal-marker characters over the current aligned row length; this suffix is never part of Jalview's sequence name, right-click menus, rename dialogs, search, Apply payloads, Canvas `display_name`, or snapshots except as pixels in the rendered left list. Right-click menus, rename dialogs, search, and Apply name edits must use the raw display name without the coordinate prefix or residue-ratio suffix. This prefix and residue-ratio suffix are display-only and must not write back into the Canvas `display_name` column. The Canvas left sidebar always prefixes each item primary title with `[item number]` independently of this option.
  - Canvas PHgo FASTA export uses the Canvas-specific header form `phgo://species/label/id2\source_label/source_id\raw_row_number/canvas_left_title\display_name`. PHgo headers use `~` as the canonical placeholder for an empty field; parsers must treat `~` as empty, and generators must write `~` instead of guessing missing source fields. Canvas import must read the trailing `display_name` into the row `display_name` column and lock it so tree display-name source changes do not overwrite it. Non-Canvas import paths may ignore the Canvas-only fields after reading the common PHgo species/label/id/source fields.
  - Canvas FASTA export header modes are: original headers, PHgo Canvas headers, PHgo Lite headers, minimal ID-only headers, and display-name-only headers. PHgo Lite uses `species|ID2(symbol)` when a real symbol is available, otherwise `species|ID2`; empty, `~`, and `~~` symbols must neither generate nor be restored from a parenthetical suffix. PHgo Lite must be available in every regular FASTA export surface that offers full PHgo headers, and the common PHgo-header parser must recognize both its optional-symbol forms. The optional all-rows FASTA export toggle includes unchecked rows for the normal FASTA export; when the toggle is off, FASTA export remains selected-rows only. The old Canvas "converted FASTA" export is removed from the UI because PHgo no longer performs pre-alignment biological conversion outside the MEGA runtime.
- Opening a Canvas snapshot must rebuild enough in-memory tree plan state from the saved payload, run manifest, and restored artifact files for the next tree-panel open, snapshot save, or preview-only metadata sync to continue without recomputing. If `last_payload` is empty but `viewer.payload.json` exists in artifacts, use the artifact payload as a recovery fallback. However, the first user-triggered `Refresh tree` after opening a Canvas snapshot is always a full compute refresh through `mega-phgo-runtime`; it must not reuse snapshot-restored alignment/tree artifacts even when fingerprints match.
  - Canvas tree refresh has two explicit phases: compute and render. The first refresh in a live Canvas session is full compute plus render. Later refreshes run render-only only when the sole change since the last successful compute is a display label change (`display_name` edits or the display-name source). Any other right-panel setting change, including target mode, alignment method/parameters, or tree method/parameters, is a computation change and must rerun `mega-phgo-runtime`. Any Canvas table selection change is also a computation change. Reactree-only viewer controls such as the View > Labels > Coord toggle are not PHgo refresh inputs.
  - Every successful Canvas tree refresh must update the single shared viewer payload and MSA state through the common viewer update path so already-open tree and MSA browser pages observe the same alignment/tree generation. The MSA page must listen for viewer update events and reload itself only when the shared payload version changes, not for ordinary MSA state saves.
  - Opening the Canvas system-tree tools panel, opening the Reactree preview, refreshing the tree, and retrying after a runtime-tool check must run behind task/progress modals unless task modals are explicitly suppressed by tests. No tree runtime probe, viewer startup, or refresh preparation should leave the TUI sitting on an unlabelled black wait screen.
  - The Canvas `Open preview` action is available only after a successful shared refresh has produced both a Newick tree and aligned FASTA in the single viewer payload. Before that point the preview button and shortcut should not open empty tree/MSA browser pages.
  - MSA Apply is triggered by the browser/Jalview server callback while the PHgo Canvas TUI may still be drawing. It must refresh the shared PHgo/MSA/tree artifacts once, report progress through the local viewer session status for the tree and MSA browser pages, and show the normal PHgo loading modal through the already-open Canvas TUI app. It must not start a second PHgo TUI task/progress page or any interactive terminal prompt from the HTTP handler. Jalview File > Save is different from Apply: Save only persists the current MSA state to `/sessions/<id>/msa/state` for later `.pgo` snapshot capture and must not update PHgo Canvas rows or refresh tree/MSA artifacts. Save is manual, not automatic, but its scope is complete durable MSA state: row states, edited sequences/names/descriptions, render settings, colours, groups, annotations, markers, and Jalview-owned sequence features using Jalview's project-save data model: feature type, description, begin/end, score, feature group, links, and `otherDetails`. Pure UI state such as menus, scroll offsets, window geometry, cursor/selection rectangles, export-window controls, and browser viewport state must not be saved. Reopening the MSA page restores this saved state unless PHgo has refreshed the shared tree/MSA payload.
  - System browser launching for Canvas/Explore tree viewers must be detached from short-lived task-modal contexts. The task context may cancel as soon as the progress modal closes; do not use it to own the OS browser launcher process after the URL has been prepared.
  - Tree recomputation must be driven by computation inputs: selected row identity/order, sequence content, target mode, alignment method/parameters, and tree method/parameters. `display_name` edits and display-name source changes are preview-only and must update Reactree/Jalview metadata without rerunning `mega-phgo-runtime` when the compute fingerprints are unchanged and the session is not in the first-refresh-after-snapshot state. Reactree viewer coordinate-label toggling is browser state only and must not call PHgo refresh.
  - Tree alignment method surfaces must follow the target mode. Protein mode shows the two amino-acid aligners; DNA mode shows the two base aligners plus the two codon aligners. Runtime plan building should normalize the surfaced method set to the current mode and then hand the selected plan to `mega-phgo-runtime` without adding extra local biological validation.
  - Opening the Canvas system-tree preview must open two browser pages for the same session: the Reactree tree preview page at `/sessions/<session>/tree` and a separate JalviewJS host page at `/sessions/<session>/msa` that opens the current aligned FASTA through official JalviewJS bootstrap arguments. Do not replace that MSA page with a PHgo-made alignment viewer. When aligned FASTA artifacts already exist, PHgo should also best-effort launch the local Jalview desktop application with that alignment file, preferably via Jalview's `-open` argument, but failure to find or start Jalview must not block the two browser previews.
  - The `/sessions/<session>/msa` JalviewJS page must use vendored local JalviewJS static assets under `internal/phylo/viewer_assets/assets/jalviewjs/`, not remote CDN or GitHub-hosted URLs, so the app remains fully offline-capable and future JalviewJS page customization can be developed against repository-owned assets. Tree-viewer rebuild scripts must preserve that vendored JalviewJS subtree when refreshing Reactree assets.
  - The local JalviewJS bootstrap should follow JalviewJS's official embedded-demo container contract (`jalview-desktop-div`, `jalview-alignment-div`, and related embedded target divs) instead of injecting SwingJS-generated `_code` into an arbitrary wrapper div. Keep PHgo diagnostics around that official embedded startup path so future blank pages expose the exact failing phase immediately.
  - JalviewJS command-line opening must use the official `Info.args` path as a JavaScript string array, e.g. `["open", "<aligned.fasta URL>"]`. Although the upstream README shows a string form, the vendored JalviewJS runtime passes `Info.args` directly to `main$SA`, and `jalview.bin.ArgsParser` expects an array. The visible MSA document URL should remain `/sessions/<session>/msa`; serve the query-free bootstrap HTML directly on that route instead of redirecting users to a long `/jalview-bootstrap/...#phgo=...` URL. Keep `/jalview-bootstrap/<stamp>.html` query-free as a compatibility/debug route, because JalviewJS `getURLCommandArguments$` reads the page query string and overwrites `Info.args`.
  - PHgo-specific JalviewJS bootstrap glue belongs in the vendored `assets/jalviewjs/phgo-bridge.js` bridge layer, not in a custom MSA renderer. The bridge owns PHgo state parsing, Jalview `Info.args` construction, post-startup SwingJS DOM adaptation, MSA state save/apply payload collection, and robust outside-click/Escape menu closing. Menu closing should first go through SwingJS/j2sMenu state paths such as `collapseAll`, `J2S.Swing.hideMenus`, and `JSPopupMenuUI.closeAllMenus`; any DOM fallback may hide popup menu nodes with `display:none` plus aria state, but must remove any inline `visibility` override and must never write `visibility:hidden`, because SwingJS `_show` restores display but not a forced hidden visibility state. Parser behavior, display-name/taxon-id handling, DPI behavior, core SwingJS/Jalview rendering changes, and PHgo MSA row selection UI must be patched directly in the vendored JalviewJS/SwingJS source and runtime core files. For the MSA row selection column, draw and handle clicks in Jalview's native `IdCanvas`/`IdPanel` and add File > Save plus File > Apply through Jalview menu code; do not overlay DOM checkboxes/buttons on top of Jalview. Checkbox clicks must repaint through the bridge's single `requestMSARepaint()` path, clear the cached ID-canvas image and height together, force `fastPaint=false`, and keep `IdCanvas` null-safe so toggling a checkbox cannot leave the left ID list as a white panel; `IdCanvas.paintComponent` must not take its fast-paint early return unless the cached image is non-null. Because `/sessions/<session>/msa` is the visible bootstrap route, root-relative PHgo paths such as `/sessions/<id>/aligned.fasta` should be passed to Jalview as same-origin absolute URLs so Jalview's resolver reaches the viewer server root.
  - JalviewJS scrollbars are SwingJS `JSScrollBarUI`/jQuery slider components, not browser-native scrollbars. PHgo scrollbar shape/color changes must patch the vendored loose `swingjs/plaf/JSScrollBarUI.js` and the readable runtime core bundles that PHgo executes, while leaving control dimensions and layout calculations unchanged.
  - JalviewJS/SwingJS DPI fixes must cover both the visible root canvas and Jalview's offscreen `java.awt.image.BufferedImage` caches. Alignment rows, sequence IDs, annotation panels, and scroll blits render through cached `BufferedImage` canvases before being drawn to the root canvas, so `BufferedImage.getImageGraphic`, `JSGraphics2D.drawImage` source-rectangle draws, direct pixel writes, and `copyArea` must stay device-pixel-ratio aware in both loose source files and the readable core bundles.
  - Jalview preview FASTA served from `/sessions/<session>/aligned.fasta` is preview-only and must not replace the MEGA/runtime `aligned.fasta` artifact. PHgo should encode metadata display names as `phgo_name64:<base64url-utf8>` headers, and vendored JalviewJS `AlignFile.parseId` must decode that prefix directly into the sequence name so spaces, slashes, coordinate-looking suffixes, punctuation, and non-ASCII labels render without Jalview's normal FASTA header splitting or `/from-to` parsing.
  - Local JalviewJS startup should load repository-owned readable core bundles such as `corejvexamplefile.js` instead of `.z.js` bundles so direct source patches remain the executed code during PHgo development and offline testing.
  - The visible MSA page should present the opened Jalview alignment window as the whole page. Implement this in vendored JalviewJS/SwingJS itself: Jalview `Desktop.addInternalFrame` marks the first opened `AlignFrame` as the PHgo main alignment frame, and SwingJS `JSFrameUI` embeds and undecorates only that marked frame into `jalview-alignment-div`. Keep the Jalview root `Desktop` JFrame as an invisible full-page window-manager layer so search/dialog child windows can still render above the alignment; the root desktop container must remain `display:block`/visible, must not be `aria-hidden`, and SwingJS component rendering must not short-circuit the root desktop frame because child dialogs depend on that component tree. Hide only the root desktop's own canvas/menu chrome, globally stretch no `.swingjs-window`, and do not strip title bars/resizers from child windows through the PHgo bridge. Do not hide or replace Jalview's actual alignment content.
  - The PHgo JalviewJS MSA view has exactly one parent alignment window: the opened PHgo alignment. Do not expose file-open/add-sequence/paste/delete/remove/translate/fetch actions, and do not allow Jalview actions to create additional sibling alignment/split/input parent windows. Output-to-textbox is allowed only as a read-only utility child window for copying/export inspection, never as an input/new-alignment path. Do not let double-clicking sequence IDs launch the default EMBL-EBI search. Search/find, manual grouping/annotation actions, output textboxes, and other small utility dialogs may still open above the single alignment parent when they do not mutate PHgo-provided FASTA/alignment content. Allowed MSA edits are sequence name, description, and sequence text. Name edits Apply back to Canvas as locked `display_name`; description edits are MSA-owned state for snapshots unless a PHgo field is deliberately added later; sequence edits Apply back to Canvas sequence data and are uppercased on confirmation while preserving spaces/other characters instead of replacing them with underscores or gaps. Do not expose 3D Structure Data or Toggle Case in the PHgo MSA menus.
  - Notification sounds use the repository `assets/sounds/*.mid` files directly. Do not add bundled MP3/WAV audio blobs or embedded binary sound assets back into the app. Keep playback minimal and native per platform, allow overlapping independent plays, and make each platform player invocation close/exit when playback ends. On Windows, keep the native `winmm.dll` path robust by trying MCI sequencer playback first and falling back to a small Standard MIDI File parser that schedules channel messages through `midiOutShortMsg`; this avoids total silence on systems where MCI sequencer playback is unavailable while still using the same `.mid` assets. Current cues are: startup/helper download success -> `startup.mid`; BLAST results -> `blast.mid`; keyword and family results -> `keyword.mid`; entering Canvas -> `canvas.mid`; successful tree refresh and successful automatic labelname completion -> `done.mid`.
  - PHgo fully replaces the current MSA image-export workflow with the PHgo-owned `msaexpor` module. In the visible MSA `File` menu, the item text is `Export image...` and it must open the PHgo export-settings window instead of Jalview's built-in simple image-export chooser/path.
  - The `msaexpor` export-settings window lives inside the existing `/sessions/<session>/msa` page and must register through Jalview/SwingJS's native child-window framework so it behaves like the other MSA child windows: it participates in the same layer stack, is draggable, is resizable, and can coexist with interaction on the underlying alignment page when that child-window class is non-modal.
  - `msaexpor` window content is entirely PHgo-owned Fluent-style browser UI, not a Jalview-native Swing form. PHgo must style the controls to fit the existing MSA window environment: color tokens and corner radii are adapted toward the current Jalview/MSA child-window look so the export window feels native to the page instead of a foreign generic web dialog. A later React + Fluent rewrite is allowed only if it preserves the same child-window, settings, renderer, and export contracts.
  - `msaexpor` is a full workflow replacement, not a second parallel export path. PHgo owns the detailed export settings UI, preview logic, and final file generation for MSA image export. Exporting from `msaexpor` must not modify alignment content, must not act like `Apply`, and must not trigger tree recomputation.
  - `msaexpor` must not export by screenshotting the current MSA page, current SwingJS canvas, child window, or Jalview `AlignmentPanel`, because those paths can include temporary UI state. It must build a clean data-driven export scene from aligned FASTA, PHgo metadata, MSA selection/state, groups, and features. Export output must exclude current selection/cursor rectangles, PHgo row checkboxes, menus, scrollbars, child-window chrome, and annotations.
  - `msaexpor` writes SVG, PNG, and PDF. SVG is the canonical generated scene; PNG and PDF are derived from that same scene. PDF export uses the bundled browser-global `PHGOmsaexporPDF.exportPDFBlob` asset, not ESM module loading from the MSA page. The initial scale choices are 1x, 2x, 5x, and 10x, with 2x as the default. The scale changes output resolution only; element sizes are controlled by explicit cell width/cell height settings and remain stable for the same data/settings. The scene must honor current Jalview Format semantics for every residue: `Boxes` gates background fills, `Text` gates glyphs, and `Colour Text` selects the active residue scheme colour for glyphs (using Jalview's darker contrast colour when boxes are also present). A sequence group's own display-boxes/display-text/colour-text configuration takes precedence over the viewport Format settings.
  - `msaexpor` PNG export must use `canvas.toBlob()` and must not use `canvas.toDataURL()` for generated MSA image output, because large alignments would otherwise duplicate the raster as base64 text in memory. If PNG dimensions exceed the explicit safety limit, fail with a clear error instead of silently downscaling or crashing the page.
  - `msaexpor` model building must remain tolerant of incomplete payload metadata. When `payload.metadata.records` is absent or empty, build export rows from saved/live MSA state sequences first and then from `payload.aligned_fasta`; if no rows can be produced, the renderer must fail with the explicit `No MSA rows to export.` message.
  - Opening the export window from the Jalview menu must call the safe bridge entry (`openMSAExportImageWindowSafe`) so failures are surfaced through the in-page PHgo toast as `MSA export window failed: ...` and also recorded in bridge debug state. Do not call the unsafe window-opening function directly from vendored menu code.
  - Save flows for `msaexpor`, tree image/PDF export, and tree viewer snapshots must sanitize suggested names, default to the numbered payload/page-title prefix when available, and keep writable-handle cleanup in `finally` blocks. A cancelled save picker is not an export error; non-cancel picker/write failures may fall back to browser download only when that fallback is explicitly enabled.
  - `msaexpor` initial settings must include: optional PHgo coordinate prefix display default off, optional length ratio default off, optional length percent default off, alignment column numbers default on with interval default 20, optional right-side per-row residue end numbers default off, group rendering default on, feature rendering default on, cell width input default 9 logical px, cell height input default 13 logical px, format choices SVG/PNG/PDF, scale choices 1x/2x/5x/10x with 2x default, and an advanced layout-script switch. Enabling the advanced layout script must also enable right-side residue numbers. Automatic layout without the advanced script wraps at 100 MSA columns per major block. The export window action buttons are `Generate` and `Cancel`; `Generate` must open the save-location picker when the runtime supports it and fall back to browser download only when no picker is available.
- `msaexpor` advanced layout scripting is documented under `doc2/phylogenetic-tree/msaexpor/`. The text format uses `>rows\range[/allocation]`, PHgo row coordinates separated by `/`, and half-open MSA boundary ranges. A row item side of exactly `~`, such as `>~\10,100/~,~,~`, means all current MSA rows; `~` mixed with explicit row coordinates is invalid. Boundary `0` is before the first MSA column, boundary `alignmentWidth` is after the last MSA column, `10,100` means `[10,100)` and therefore 90 columns, `10,~` means `[10,alignmentWidth)`, `~,10` means `[0,10)`, and `~,~` means the full aligned width. Allocation placeholders `~` evenly consume remaining columns after fixed allocation counts are subtracted. Multiple script lines render in one uniform top-to-bottom major-block flow. MSA export, tree image/PDF export, and tree viewer snapshot export should default file names to the numbered prefix parsed from the payload/page title, such as `1` or `1.1`.
  - Advanced-layout coordinates are always the one-based coordinates shown in the MSA ID list. The public `/msa/selection` fields must use those same one-based values even though Canvas stores indexes internally from zero. When a record also has the canonical `display_prefix` form `[item,row]`, that visible prefix is authoritative so a script copied from the MSA always resolves, including older or restored payloads with zero-based or omitted numeric fields.
  - JalviewJS MSA resize must be debounced and applied after browser size changes settle, not continuously during resize. Do not use high-frequency bridge polling, recursive external Swing component relayout, or visible root Desktop canvases/menubars to force sizing; fix resize in the vendored Jalview/SwingJS source and keep the bridge as a low-frequency bootstrap helper. If a Jalview child dialog fails to appear, fix the SwingJS window-manager/CSS visibility or source action path; do not hide or delete otherwise valid utility/dialog/export menu items just to avoid a broken popup.
  - The visible `/sessions/<session>/msa` page should not add PHgo-specific chrome such as titles, status bars, notices, or log panels around JalviewJS. Keep any PHgo bootstrap diagnostics internal to JavaScript state or console logging so the browser surface is effectively full-screen JalviewJS.
  - Reactree viewer labels are sanitized only at the viewer adapter boundary. The adapter must preserve PHgo-provided display labels without adding duplicate-disambiguation suffixes such as `[PHGOT...]`; PHgo's default-on Canvas coordinate prefix is the normal disambiguation surface. The same taxon-id-to-label map must be applied to Newick leaves and aligned FASTA headers so the tree and alignment panel stay synchronized.
  - Reactree live Canvas previews must adapt raw MEGA runtime Newick into MEGA Tree Explorer's default display order before rendering, but only for payloads explicitly marked as `tree_computation_source=mega-phgo-runtime`. Standalone/imported Newick and `.pgv` snapshots without runtime inference metadata must not be silently rerooted or sorted as inferred trees. For ordinary single-tree NJ/ME/ML/MP runtime results, the viewer restores MEGA's `TTreeList`/TreeBox binary structure from Newick, uses MEGA's `SetMaxLength` -> `SearchMidPoint` -> `ChangeRoot` midpoint-root behavior exactly once when branch lengths exist, including binary-root-shaped runtime Newick that still represents an inferred unrooted MEGA tree, and then arranges clusters with `SortBranchByFigure`. Topology-only MP cannot be midpoint-rooted because MEGA `MakeRootOnMidPoint` exits without branch lengths; rooted UPGMA keeps its root because the method is rooted and MEGA exits when `isRooted` is true. Multi-tree runtime payloads must disable midpoint rooting and use `SortBranchByOrder`/input order, matching MEGA `InitForm`. This is MEGA's source-defined Tree Explorer display behavior, not a generic farthest-leaf diameter midpoint algorithm and not a screenshot-specific correction. It is viewer-only and must not alter `tree.nwk`, runtime manifests, computation fingerprints, or MEGA-owned calculation artifacts. Canvas live sessions must not restore stale Reactree `treeData/history` over a fresh payload; standalone `.pgv` sessions may restore saved tree state because that is an explicit snapshot-open action.
  - Reactree MEGA-style rendering is display-only: it uses the same sans-serif font family as PHgo style, hides node dots, and adds a short rectangular terminal connector so leaf labels remain visually attached without changing the Newick or computation artifacts. Clade/color palettes in the tree viewer use the standard Okabe-Ito color-blind-friendly scientific palette rather than a dark-only contrast palette, so exported figures remain readable for common color-vision deficiencies and do not look overly heavy.
  - PHgo viewer `.pgv` snapshots currently write PGV schema v3 and viewer-state schema v3. Readers must keep v1/v2 compatibility. The v3 viewer state covers Office-style ribbon tab/search/menu state, PHgo/MEGA render style, font family, export long-edge size, alignment split width, viewport metadata, zoom transform, and non-negative width/height/circular-size scale values where `0` is valid and no schema-level maximum is imposed.
  - System-tree parameter-setting documentation must use the newest MEGA 12.1 source authority and ignore MEGA option JSON files. ClustalW alignment parameters use the registered MEGA 12.1 HTML dialogs. MUSCLE parameters use real MEGA screenshots plus MEGA 12.1 source/runtime anchors when no registered MUSCLE HTML dialog exists. Tree-type/tree-inference parameters use `MegaDlgs/manalysisprefdlg.pas`, `Common/megaanalysisprefstrings.pas`, and real MEGA screenshots when no registered tree analysis HTML dialog exists. JSON must not override controls, selected options, checkbox states, option lists, dynamic visibility, or default input values.
  - For MEGA behavior that has no parameter-setting UI surface, such as runtime orchestration, artifact production, algorithm/thread ownership, error reporting, and snapshot/viewer handoff, use the MEGA 12.1 source code and the PHgo runtime implementation as the authority and record the confidence level in the tree reference docs.

## Long-term database and workflow direction

- Treat `Phytozome` and `lemna.org` as primary sequence/genome sources. They provide species selection, keyword search targets, BLAST targets, source FASTA/GFF-style assets, gene/protein identifiers, and peptide export data.
- Treat `NCBI` as a first-class Entrez/E-utilities source family, but not as a BLAST target. The current implementation already includes a broad multi-search-type Entrez catalog plus first-wave runtime support across sequence, genome/resource, sample/project, variant/clinical, and literature families; it is no longer protein-only. NCBI retrieval code should stay isolated under `internal/ncbi` plus search-type-specific engines under `internal/searchengine/ncbi*`; do not scatter ad hoc NCBI HTTP logic through unrelated packages. Use official E-utilities surfaces (`EInfo`, `ESearch`, `ESummary`, `EFetch`, `ELink`, and `EPost` when batching explicit ID sets) rather than scraping normal NCBI HTML pages as the primary data path. NCBI E-utilities must obey the official rate limits globally across goroutines: no more than 3 requests/second without an API key, and no more than 10 requests/second with a valid API key; if an API key becomes invalid, automatically fall back to the no-key request rate. Support both GET and POST centrally because official NCBI guidance recommends POST for long queries and large ID lists. Future NCBI work must treat `EInfo` as the authority for live database names, searchable fields, truncatable/rangeable field metadata, and available `ELink` link names; do not hardcode all NCBI field/link assumptions when they can be discovered. For large or multi-step result sets, prefer History-server pipelines (`usehistory`, `WebEnv`, `query_key`) plus batched `ESummary`/`EFetch`/`ELink` retrieval instead of one request per row. The NCBI protein keyword search must always try `db=protein` first; if that yields no usable protein FASTA, retry the same keyword through official Nucleotide/nuccore E-utilities, parse GenBank CDS `/translation`, and, when only `/protein_id` is present, fetch that linked protein record for FASTA. Nucleotide fallback must never invent reverse translations. NCBI API keys must be normalized to lowercase before live requests because the current default key works lowercase but live NCBI rejects the uppercase form as malformed. NCBI protein rows expose a dedicated `gene_locus` column before `label_name`; NCBI Gene aliases/designations and row metadata are sent directly to the symbol-name system for labels, while Gene locus recovery may still use a documented Phytozome secondary lookup when NCBI lacks a usable locus. NCBI keyword empty results are not cacheable successes: the NCBI engine must pause conservatively and retry once, then the shared keyword recovery dialog must offer Retry/Skip/Close if there are still no rows. If an NCBI row reports a replacement/update target such as `replacedby`, review must never silently overwrite the original requested identifier; ask whether to keep the old record or use the replacement, preserve the original request/accession and the chosen decision in row metadata, and make the accepted/kept update decision visible in `search_type`. Future NCBI search types must not all inherit protein assumptions: define explicit result domains such as `sequence-record`, `genome-resource`, `sample-project`, `variant-clinical`, and `literature-reference`, and use those domains to control columns, detail views, exportability, FASTA/Canvas eligibility, and snapshot metadata instead of deriving everything from `database == ncbi` alone. Automatic `ELink` fallback chains are now a first-class part of the NCBI design; whenever linked fallback materially changes the visible result set, persist executed-link provenance in row metadata and snapshot source-state. NCBI design and rollout documentation belongs under `doc2/ncbi/`; update that library whenever new NCBI search types, linked workflows, row contracts, or snapshot requirements change.
- Session snapshots are a standalone system under `internal/sessionsnapshot` with the `.pgo` extension. A `.pgo` file is a compressed archive with XML manifest/modules and per-module versioning. Snapshot semantics are "freeze the user-visible workflow state", not "save only enough to redraw one page" and not "dump raw memory". Every database, mode, result page, and future feature must define its snapshot contract as preserving the workflow inputs, settings, selections, review mode, derived rows, and downstream-export prerequisites needed to reopen in the same working state without re-running the original job.
- Snapshot routing must stay flat and future-ready: database, mode, and result kind are string identifiers, and new databases or result pages should add modules/handlers without turning the loader into a rigid current-workflow tree.
- Opening a `.pgo` snapshot must rehydrate directly into the saved result review page using stored rows, grouping, selection masks, filter state, query metadata, user-edited labels, table cursor/sort/scroll state, and the sequence data needed by later export actions. Loading a snapshot must not re-run searches, BLAST jobs, enrichment, or label inference unless the user later chooses an action that normally performs that work.
- Snapshot completeness is judged by workflow continuity: after opening a snapshot, the user should be able to continue review, filtering, export, follow-up BLAST, and other saved downstream actions as if the original workflow instance had just been paused and resumed. If a future feature needs any extra precondition to make that true, that precondition belongs in the snapshot contract.
- Snapshot preservation is strict: once the program has generated a result, cache, enrichment, alias decision, sequence payload, or other workflow artifact, the snapshot should preserve that generated artifact directly instead of planning to rebuild it later. Prefer complete preservation over compactness.
- Snapshot storage may mix formats when that keeps preservation simple and faithful: structured workflow state may stay in XML modules, while raw cached payloads or non-text artifacts may be packed in their original text or binary formats if that avoids fragile translation or reconstruction logic.
- The current unreleased `v2.3` contract should preserve only the explicitly recorded workflow artifacts and cache payloads that are needed for later continuity, not blanket export directories or `.cache/` trees; snapshot open should restore only those recorded paths back to their original locations before later workflow actions continue. NCBI keyword snapshots must also preserve the versioned `keyword-source-state` module so future NCBI search result types can reopen without guessing the Entrez database, record type, engine schema, accessions, or UIDs. If a recorded file cannot be restored, the fallback must be explicit rather than silent.
- New feature work is not complete until snapshot integration is updated in the same change. Any new user-visible workflow, parameter set, cache artifact, transfer path, review state, or generated payload must be assessed field-by-field for snapshot preservation, with matching write/read logic, tests, and checklist/docs updates.
- Keep search, BLAST, and symbol name identification as three separate systems:
  - Search is only the keyword/search-engine path for the selected primary database.
  - BLAST is only BLAST execution, parsing, review, filtering, and export of BLAST hits.
- Symbol name identification is a standalone system under `internal/labelname`; keep the package/import name for compatibility, but user-facing text should say `symbol name`. Workflows send known structured fields from source rows, BLAST rows, query sources, and FASTA/phgo headers as `AliasRankRequest` data. The symbol name system owns matching and ranking.
  - The primary dictionary is the full NCBI Gene `GENE_INFO/` split source directory from `https://ftp.ncbi.nlm.nih.gov/gene/DATA/GENE_INFO/`, covering all taxa through the `All_*` category files plus top-level organelle/plasmid files. The app converts it into the application-local `symbolname.pgd` database. The `.pgd` metadata must record the source URL, newest NCBI split-source `Last-Modified`, download/build time, total compressed source length, schema version, record count, and checksum.
  - Runtime install/update checks must use the GitHub-hosted prebuilt symbol-name database manifest and archive from the dedicated repository `KiriKirby/phytozome-go-symbolname-db`. The runtime application must not download NCBI `gene_info.gz` / `GENE_INFO/` sources or build `symbolname.pgd` locally as a fallback. The prebuilt branch stores only `symbolname/manifest.json` and small documentation files; database archive files are distributed through that repository's GitHub Release assets and referenced by `browser_download_url` values in the manifest. Runtime downloads those assets, expands them locally to `symbolname.pgd`, and uses the manifest's NCBI split-source `Last-Modified` metadata for staleness checks. If the GitHub manifest/archive path is unavailable or invalid, startup may skip after user choice, but normal symbol-name use must retry/report the GitHub prebuilt install failure rather than continuing without a database or building from NCBI.
  - The prebuilt symbol-name database branch is maintained by GitHub Actions in `KiriKirby/phytozome-go-symbolname-db` on a fixed monthly schedule plus manual dispatch. Current policy is one scheduled rebuild each month on day 18 at 02:18 UTC, with the workflow force-publishing the latest manifest branch snapshot rather than appending database artifacts to the main application repository.
  - Prebuilt database workflow changes must be made and validated in the dedicated `KiriKirby/phytozome-go-symbolname-db` repository before a full GitHub run: first run local unit tests plus the local `cmd/build-symbolname-db -sample` small-sample build there, then run a local `-simulate-archive-size` large-archive split/publish dry-run that does not build the database, then dispatch that repository's GitHub workflow in `sample` mode, then dispatch GitHub `simulate` mode to exercise large multipart publishing without NCBI/database build cost, and only after all of those succeed dispatch or wait for the `full` build. Do not use the full GitHub build as the first test because it can take hours and consumes limited Actions quota.
  - GitHub publishing for the prebuilt symbol-name database must happen in `KiriKirby/phytozome-go-symbolname-db` and follow GitHub's documented large-file guidance: normal git branches hold only small manifest/docs files, while large binary archive parts live in that repository's Release assets. Release asset parts must stay below the documented per-asset limit with a safety margin; the full workflow currently uses 1 GiB parts. The publish tool must fail during validation when any branch file exceeds the configured Git blob limit or any Release asset exceeds the configured asset limit, before attempting network uploads. The manifest branch is published only after all referenced Release assets have uploaded successfully, and old matching assets are cleaned up only after the branch update succeeds. Do not add symbol-name database build workflows or database Release assets back to the main `phytozome-go` repository.
  - Any prebuilt database split/archive change must test the reverse path: manifest-ordered part download, reassembly into the original compressed stream, decompression, sha256 validation against the decompressed `.pgd`, `.pgd` inspection, and one real symbol-name ranking query. Keep a gated live test for the GitHub `symbolname-db-sample` branch in `KiriKirby/phytozome-go-symbolname-db` so real raw.githubusercontent.com multipart downloads can be verified without running the full database build.
  - The current `.pgd` schema is v2. It stores compact binary records for the fields needed at query time and keeps the full `gene_info.gz` field set only for build-time indexing. Do not reintroduce JSON record storage in the hot path.
  - `.pgd` builds are owned by the dedicated database repository and its GitHub Actions workflow, not the runtime application. Keep runtime code focused on manifest fetch, Release asset download, stream decompression, validation, installation, and query-time ranking.
  - Supported index inputs mirror `gene_info.gz`: tax_id, GeneID, Symbol, LocusTag, Synonyms, dbXrefs, chromosome, map_location, description, type_of_gene, Symbol_from_nomenclature_authority, Full_name_from_nomenclature_authority, Nomenclature_status, Other_designations, Modification_date, and Feature_type. Callers should provide any fields they know and leave unknown fields blank.
  - Database namespace tokens from dbXrefs, such as `TAIR`, `GeneID`, `NCBI`, `Ensembl`, `UniProt`, and similar prefixes, are stop keys. Keep matching the full dbxref and the identifier after the colon, but do not index the namespace prefix alone because it creates massive low-value posting scans.
  - Ranking first requires a local database match. `symbolname.pgd` is mandatory for symbol-name use: startup may offer a y/n download prompt when it is missing and allow skipping, but normal TUI workflows must download/build it in a task modal when symbol names are actually used. Download/build failures must present retry recovery; startup may skip after a failure, but TUI symbol-name use must not continue in a no-database fallback state. Suppressed/noninteractive paths must fail clearly unless tests provide a temporary `.pgd`. When no candidate matches the database, return no aliases instead of inventing a fallback. Do not keep a no-database heuristic compatibility path.
  - Querying should collect matching record ids first and decode each record at most once per request. Batch ranking should de-duplicate equivalent requests before work starts, reuse the open `.pgd` handle, and run independent lookups through a bounded local worker pool.
  - Every prebuilt database download path, including startup helper and in-app TUI installation, must show visible progress with downloaded bytes, total size when known, download speed, and installation status. Runtime must support any manifest part count: download Release asset parts with bounded parallel staging to disk, reassemble strictly in manifest order, stream-decompress, validate the decompressed `.pgd` sha256/record count/source `Last-Modified`, and clean temporary part files after success or failure.
  - After matching, batch ranking should promote candidates that form the largest symbol families in the current batch, ordered by mergeable family size descending; non-family candidates come after family-forming candidates.
  - BLAST query/source auto symbol name and BLAST-hit auto symbol name should prefer two-phase execution for performance: parallel data/candidate collection first, then batch symbol-name ranking over de-duplicated requests.
  - All automatic symbol-name paths for every supported database and mode must construct `labelname.AliasRankRequest` values from all known structured local fields, then batch-rank through the local `symbolname.pgd` database. This includes Phytozome, Lemna, TAIR, NCBI keyword rows, BLAST query/source items, and BLAST-hit rows.
  - Keyword-mode automatic symbol-name identification must first batch-rank each original search term directly against `symbolname.pgd` with only search-term/group-label/tax context. Only groups with no direct database aliases may fall back to already-fetched result-row metadata, aliases, descriptions, and dbxrefs. This keeps TAIR/Lemna/NCBI/Phytozome keyword labels from being dominated by noisy broad search rows.
  - Automatic symbol-name code must not perform workflow-level fallback lookup chains. Do not do secondary Phytozome keyword searches for Lemna/TAIR/NCBI symbol names, do not use source-label or FASTA/header text as an invented fallback label, and do not synthesize gene/transcript/protein identifiers as labels when the symbol-name database returns no aliases.
  - Labelname/symbol-name matching must not query online database sources. It may only use the local `symbolname.pgd` plus structured data already present in the current query/source/result rows. Cache and reuse both hits and misses so repeated automatic label paths do not re-scan the local database for equivalent requests.
  - BLAST-hit auto symbol name should cache hit-level identification decisions by a stable hit/source-symbol key across repeated runs so reopening/re-filtering/export chains do not redo the same local ranking work.
  - Auto symbol name stages must expose visible progress states instead of a silent wait; at minimum distinguish candidate prefetch/collection from ranking/application so users do not interpret the phase as a hang.
  - Table selection performance is part of workflow performance, not only UI polish:
    - large row-selection tables and BLAST run tables must keep `SetEvaluateAllRows(false)` so off-screen rows are not eagerly re-evaluated during draw
    - repeated selection-only actions such as `Ctrl+A`, `Ctrl+N`, header checkbox toggle, or single-row checkbox toggle should update only visible checkbox/title/list state when the row order and table contents did not change; do not rebuild the whole table for those actions
    - grouped row-selection tables should keep a direct original-row -> display-row index cache after each display-row rebuild so selection restore and current-row lookup do not rescan the full visible row slice on every interaction
    - grouped display-row construction must not rescan the sorted order once per group; build ordered/group lookup structures once and reuse them
- `phgo_alias` is the phytozome GO authoritative alias column produced by the symbol name system. Result table views show `phgo_alias`; the original source `alias` column remains available in row details, exports, and reports immediately after `phgo_alias`.
- Alias-like table columns, including `phgo_alias`, source `alias`, `symbols`, `synonyms`, `uniprot_gene_names`, and dynamic `*_alias*` fields, should show only the first five aliases in result/Canvas table cells followed by `...` when more are present. Detail panes, exports, reports, snapshots, and alias chooser dialogs must keep the full stored alias list.
- Phytozome keyword rows keep source `symbols` and `synonyms` as separate fields instead of merging them into `alias`. They are not table-display columns. Automatic symbol-name collection sends all available row fields and metadata to the symbol-name system together; workflow code must not impose a source-field fallback order.
- BLAST query/source and BLAST-hit symbol-name collection use the same local symbol-name database path as keyword mode. Known source metadata, identifiers, aliases, descriptions, dbxrefs, and tax IDs are structured into the request; if `symbolname.pgd` produces no match, the label stays blank unless the user already supplied or pinned a label.
- Lemna, TAIR, and NCBI keyword/BLAST symbol-name collection must not route through Phytozome as a secondary symbol source. Their local row/source metadata is structured and submitted directly to `internal/labelname`, which performs the only matching and ranking step.
- TAIR release support is intentionally maintained as an explicit per-release asset table in `internal/tair`; this is acceptable and preferred over guessing across TAIR's heterogeneous historical directory layouts. Verify paths against TAIR's JSON download listing endpoint (`https://www.arabidopsis.org/api/download-files/list?dir=...`) and download concrete files through `https://www.arabidopsis.org/api/download-files/download?filePath=...`; for historical releases present in the official FTP tree, prefer the equivalent concrete mirror path under `ftp://ftp.arabidopsis.org/home/tair/` so old GFF/FASTA assets do not depend on the slow current download proxy. For versioned releases that expose GFF/FASTA assets, keyword rows, peptide export, nucleotide export, BLAST databases, and row enrichment must come only from that selected TAIR release's official download files. Do not fall back from a selected TAIR release to Ensembl, UniProt, Phytozome, or TAIR live search APIs for sequence data or version-specific row enrichment. TAIR live APIs may be used only for TAIR workflows with no versioned release assets available, and such releases should remain hidden from modes that require unavailable assets. If a selected release has versioned assets listed but the official asset endpoint currently returns HTML/error content or times out, surface that asset-download error instead of returning an empty search result.
- TAIR keyword search semantics are strict and non-fallback. In versioned-release workflows, `SearchKeywordRows` must use only the exact program selected for the user's query class within the selected TAIR release data. Do not automatically fall back from locus/model/identifier/symbol/keyword search into TAIR wide search, broad search, family search, or live TAIR APIs just because the first pass returned zero rows. Zero rows is the correct result when the requested precise search path has no match. Wide and broad TAIR search remain explicit user-invoked modes only.
- TAIR11 is backed by the official TAIR/Phoenix Zenodo record `https://zenodo.org/records/17371665` for Araport11/TAIR functional annotation assets. Use only the concrete Zenodo record files wired in `internal/tair`; if that record does not include protein/CDS/nucleotide FASTA, detail FASTA and BLAST must report that absence instead of using Araport11/TAIR10/live API fallback. TAIR12 is backed by ENA project `PRJEB100887`; exact AGI identifier searches should query ENA Portal records inside that project, and per-record FASTA should use ENA Browser FASTA for the matched accession. TAIR12 ENA coding searches and FASTA fetches must use shared in-process caches, persistent disk caches, and `singleflight` request coalescing so concurrent keyword/detail lookups for the same accession do not create duplicate remote calls. Live ENA probes on 2026-06-25 succeeded at 96 concurrent requests for the user's duplicated 38-keyword set with zero coding/FASTA failures, so TAIR12 keyword workflow defaults to `PHGO_TAIR_ENA_KEYWORD_WORKERS=96` unless the environment overrides it. TAIR12 BLAST must remain unavailable until an official bulk FASTA asset for that same ENA project is wired; do not synthesize a BLAST database by cross-release fallback.
- TAIR GFF indexing must merge complementary feature rows for the same locus/gene, especially TAIR12-style `gene` plus `mRNA` records where gene rows carry symbols/synonyms/descriptions and transcript rows carry FASTA-compatible sequence IDs. Search/family results should not show duplicate rows for one locus solely because metadata is split across feature types. Family candidates derived from local GFF descriptions must be conservative: avoid sentence-like descriptions such as "Encodes..." and exclude rows without selected-release sequence IDs from family FASTA-oriented flows.
- Keyword-to-BLAST transfer keeps only reusable workflow outputs plus BLAST execution/fallback material: stored query-source `label_name`, stored `phgo_alias`, resolved peptide sequence, source URLs, source database/species identifiers, target-compatible gene/transcript/protein identifiers, and other sequence-resolution metadata needed by downstream BLAST fallbacks. It must not forward source `alias`, `symbols`, `synonyms`, or `auto_define` as fresh BLAST label candidates.
  - Keyword-to-BLAST preparation should treat the selected-row transfer as its own performance-critical stage:
    - de-duplicate sequence fetches by sequence id through the shared protein-sequence cache/singleflight path
    - de-duplicate BLAST query-item construction by stable keyword-row cache key within the same transfer pass, not only across later re-entry
    - expose at least two visible progress phases: fetching keyword peptide sequences, then building cached BLAST query items
- A `label_name` selected from a keyword result table is the BLAST query/source label, not the label for every BLAST hit. It is preserved as query metadata, written to hit rows through `blast_labelname`, used in FASTA query-sequence headers, and included in the Excel query metadata sheet. BLAST hit rows have their own independent `label_name`, with `labelname_type` recording how that row label was obtained.
- In keyword modes, `phgo_alias` is each keyword result item's ranked alias list. In BLAST modes, `phgo_alias` is each BLAST hit/result row's ranked alias list, not the BLAST source/query item's alias list. Source/query aliases stay on `QuerySequenceSource.PhgoAliases` for metadata, family BLAST, and custom grouping.
- Identifier naming must stay biologically explicit in every user-facing table, detail view, Excel sheet, FASTA/PDF report, and help text: use `geneid`, `protein_id`, and `transcript`/`transcript_id`/`blast_transcript` according to the biological identifier actually being shown. Preserve established user-facing mappings unless the stored value is clearly a different identifier class; for example, the historical internal `BlastGeneID` field stores query transcript-like IDs in BLAST rows and should be displayed/exported as `blast_transcript`. Internal helper names may use `ID2` only for generic workflow tracking identifiers that deliberately span gene/protein/transcript/sequence IDs; `ID2`, `id2`, or `blast_id2` must never appear in user-facing output.
- Any BLAST workflow that starts from more than one query item must remain in multi-file/multi-run review mode for all databases and BLAST programs, even if Family BLAST merging, failures, skips, filtering, or no-hit queries leave only one visible table or one query with visible result rows. Single-file review is only for workflows that originally had one query item. The row-selection Export button must use that original query count when deciding whether to show the Current table / All tables scope modal; do not decide from the post-merge visible table count.
- BLAST session snapshots must store and restore that original query/input count exactly. When a multi-query workflow is exported as a session after Family BLAST merging, reopened review/export must still behave as multi-file mode even if the saved visible run list contains only one merged family table.
- BLAST program selection must preserve the order supplied by the source capability layer. Do not promote `blastp` or any other program to a special default, do not add `(default)` markers, and start the cursor on the first available program like every normal choice list.
- Keyword-to-BLAST skips the BLAST query label input page only when every selected keyword row already carries a non-empty query-source `label_name`; otherwise the normal BLAST label input flow still runs for the remaining unlabeled query items. This query label flow is separate from optional automatic BLAST-hit label identification.
- Family BLAST, the custom grouping editor, and multi-file BLAST grouping define groups by the genes/sequences used as BLAST queries (`blast_labelname`/query-source gene identity), not by each BLAST hit row's own `label_name`. Hit-level labels may differ inside one family table and must not change the query-family grouping definition.
- Family BLAST and the custom grouping editor must consume the ranked alias lists already stored by the workflow during auto-identification for grouping, member alias dialogs, semantic alias tokens, and default member names. They must not query the symbol name system again or independently rebuild alias ranking.
- phytozome GO FASTA exports offer three header modes in the export settings modal. The default is `phgo` format, using the current group-delimited `phgo://` schema. Backslash (`\`) separates semantic groups; slash (`/`) separates fields inside one group. Keyword records use `phgo://species_short/label_name/id2\row` when a row number exists, which means self group plus table group. BLAST hit records use `phgo://species_short/hit_label_name/hit_id2\blast_source_label_name/blast_source_id2\row`, which means self group, source/query group, and table group. Prepended BLAST query/source records also use phgo headers in this mode, with `phgo://species_short/source_label_name/source_id2\h`. The second mode writes the source-original FASTA header directly from the database/cache. The third mode is a minimal primary-ID-only header such as `>AT2G37040.1`, using the row/query primary transcript, protein, sequence, or subject identifier and no labels, species, row numbers, or phgo metadata. BLAST input parsing must first split phgo headers by backslash into groups, then split each group's slash-delimited fields; it must recognize these current forms, including species or labels containing spaces, and must treat the primary `label_name/id2` in the self group as the pasted query sequence identity. Old slash-row phgo variants do not need compatibility work, but accidental generic FASTA parsing does not need to be blocked.
- Family BLAST FASTA export defaults to prepending all family-member BLAST query/source records. The "only first query sequence" option belongs only in the generation/export settings modal, is shown only for active Family BLAST exports, and must be unchecked by default; it must not appear in the Family BLAST parameter modal.
- Treat `UniProt`, `InterPro`, `Plant Reactome`, `PlantCyc`, and `MetaCyc` as external knowledge layers, not as peer primary databases in the startup selector:
  - `UniProt`: protein names, reviewed/unreviewed status, canonical length, GO, EC/catalytic activity, pathway notes, subcellular location, keywords, sequence cautions, and cross-references.
  - `InterPro`: protein families, domains, motifs, signatures, and Pfam-backed domain evidence through InterPro rather than direct Pfam-only coupling.
  - `Plant Reactome` / `PlantCyc` / `MetaCyc`: pathway skeletons, reactions, compounds, enzyme steps, and pathway-guided candidate discovery.
- The long-term biological workflow target is pathway-guided protein discovery: a user can choose a species, search a function or pathway such as `Monolignol Biosynthesis`, view a pathway diagram with enzyme steps annotated by candidate proteins, then select proteins for keyword review, BLAST, cross-species BLAST, export, or comparison.
- Pathway discovery should combine pathway skeletons from Plant Reactome/PlantCyc/MetaCyc with candidate proteins from Phytozome/lemna and evidence from UniProt/InterPro. Candidate confidence should be based on multiple signals such as keyword match, BLAST homology, EC/GO match, domain/family match, sequence length, reviewed status, and sequence caution flags.

## CLI UX standards

- Treat the terminal UI like a commercial wizard: every major step must be easy to scan, self-explanatory, and safe to recover from.
- The product is pure TUI for all interactive workflows. All interactive workflow design and implementation must target the shared tview template layer; do not preserve, add, or keep a traditional text wizard fallback.
- Main-interface redesign documentation lives under `doc2/main-interface-redesign/`. The redesigned single-interface setup-then-execute path is the hard-coded normal startup path while the old multi-screen workflow code remains available for handoff, transfer, recovery, and future rollback work. Do not invent interaction details before they are recorded from confirmed user requirements, and update that documentation before or alongside implementation work that changes the redesign direction.
- Bundled WezTerm startup has one special packaging-only preflight: when the packaged app itself is opened, tab `[0]` runs `phgohelper.bin` with visible text/spinner feedback for cache cleanup, release update check, and symbol-name database check/update. Only after that full preflight finishes should it open the main wizard through `core.bin` in tab `[1]`. It must not spawn the main tab early during normal startup. User-created new tabs inside that same WezTerm window bypass cache cleaning and launch `core.bin` directly, but if the startup state says preflight is still running they must wait in the existing TUI modal. The startup helper must keep `.phgo-init-state.json` messages current during each potentially slow phase so waiting TUI tabs show useful progress instead of appearing unresponsive.
- The bundled startup helper is also the release self-update entrypoint for packaged builds:
  - after cache cleanup, query the repository GitHub `releases/latest` endpoint with a short timeout; if the network or GitHub is unavailable, print a skip message and continue the remaining startup preflight
  - only prompt when the latest release tag differs from the bundled build version and the current build version is a real release tag, not `dev`
  - the confirmation prompt is plain `y` / `n` text in the preflight CLI; declining continues into the current build without error
  - accepted updates must download the current platform's packaged release asset, extract it into a sibling staging directory, spawn a detached updater helper, then let `phgohelper.bin` exit
  - the detached updater helper owns waiting for the helper PID to exit, copying the previous `output/` tree and `symbolname.pgd` into the staged bundle, replacing the packaged files in place, and relaunching via the normal launcher with an environment flag that skips a second cache-clean/update preflight on that relaunch
  - self-update must preserve only `output/` and the application-local `symbolname.pgd`; application-local managed tool folders such as `blastplus/` and bundled PHgo runtime files such as `mega-phgo-runtime.bin` / `muscleWin64.bin` must be replaced by the new bundle or recreated later on demand instead of being carried forward from the old install
  - after the release self-update check, the startup helper `phgohelper.bin` checks only the GitHub-hosted prebuilt symbol name library manifest from `KiriKirby/phytozome-go-symbolname-db`; runtime must not fall back to NCBI Gene source downloads or local `.pgd` builds. If `symbolname.pgd` is missing, it shows the source, remote size, Last-Modified, and target path, then asks y/n whether to download now; declining skips only startup preflight, and the TUI must ask/download when symbol names are later used. If the remote source `Last-Modified` is newer than the stored value, it shows the expired day count and remote size before asking whether to update. If an application update was accepted and staged, skip the symbol library check until the next launch. Startup downloads and update downloads must show progress, speed, and install status.
  - packaged startup uses tab `[0]` for `phgohelper.bin`, then opens `core.bin` in the main TUI tab only after preflight completes. If symbol-name database download/update is running with "use while downloading" enabled, the main tab may open while tab `[0]` continues; any symbol-name operation must wait for tab `[0]` to finish the database download/build before continuing.
  - the updater must never try to overwrite the running helper binary in-place from the live process itself
  - Windows packaged updates run inside a WezTerm-hosted startup process, so the updater must not rely on renaming the bundle root directory; mirror staged files into the existing bundle root instead
  - Windows detached updater launch must survive the closing startup WezTerm window; prefer a launcher wrapper such as `wscript.exe` spawning hidden `powershell.exe -File <script>` instead of a plain child process that dies with the terminal
- Window titles are fixed to `PHgo (version)` and must not be rewritten to tab labels; tab titles keep the existing instance naming / hierarchy logic.
- Only root tabs may expose Home. Child tabs created after clicking `+` must not show Home buttons or accept `Ctrl+O`.
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
- Keyword and BLAST result row-selection tables expose an `Aliases` action with `Ctrl+L` only when a normal `label_name` data cell is focused. It opens the same alias-labelname chooser semantics as the Family BLAST custom-group editor, including adaptive height from alias count, a `Rename`/`F2` single-input custom labelname dialog, and row updates to `label_name`, `labelname_type`, and pinned `phgo_alias` so later BLAST transfer, FASTA headers, Excel export, and reports consume the chosen label.
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
- Keyword and BLAST label prompts use `Auto identify (Enter)` as the primary action when the input is empty. As soon as any text is present, the same primary button becomes `Apply (Enter)`. `Skip` is shown immediately to its left in the right-side action group and uses a non-Enter Ctrl shortcut. Auto identify must call the symbol-name system for every database/mode instead of applying workflow-local priority fallbacks. User-entered or pinned `label_name` remains authoritative, and user-edited family/custom grouping after automatic identification must not be rewritten by automatic label rules.
- Treat the application as query-first. Search/query workflows should end at selectable result tables by default; file generation is a subordinate table action, never the default meaning of completing a table.
- In every result table, the default primary action is `View` (`Enter`): it opens a small details dialog for the current/selected rows. Export is exposed as `Export` (`Ctrl+G`). In multi-table result pages, `Export` first opens an OK/Close scope modal with vertically stacked `Current table` and `All tables` choices; do not show a separate `Export all` button.
- Do not ask for export filenames, output folders, label names that are only needed for exported files, data analysis reports, peptide fetching, Excel writing, list-file writing, or any other export-only data until the user explicitly chooses `Export` and confirms the export scope when a scope modal is shown.
- This separation must be enforced in workflow/business logic, not only in button labels. Before the generation subflow is opened, no export path should create output directories, derive export file names, write list files, fetch peptides only for exports, or run export-specific preparation.
- Export settings are a shared contract across all generation paths. When adding or changing export options in the TUI, propagate the returned values through `prompt.ExportSettings`, workflow `exportSettings`, and every writer branch before considering the work complete. This must cover BLAST single export, BLAST `Export all`, keyword export, FASTA output, normal Excel, raw Excel, and the data analysis report option. A missing propagation step can make all `Write*` flags false and surface as `No files were written`; add regression tests around any new export setting.
- Data analysis reports replace the old detailed plain-text log concept. Do not revive timestamped `*_blast_log.txt` or `*_keyword_log.txt` writers.
- Reports must be generated from structured report data first, then rendered to PDF by an internal Go implementation or an internal HTML-to-PDF backend. Do not use Carbone for this feature.
- Reports must never trigger database searches, API calls, BLAST runs, sequence fetches, enrichment requests, cache refreshes, downloads, or any other data-gathering work solely for report writing. A report may only use data already known to the program because the current export action needed it. If desired report content is missing, write an explicit "not available in this run" explanation instead of fetching it.
- A report covers only the current generated file set and the process that produced it: query/source input, input resolution, selected database/species, search or BLAST parameters, result/selection summary, generated files, and relevant row-level evidence for the current export. Do not include earlier session navigation, earlier searches, unrelated tools, future pathway actions, or any operation outside the current export action.
- Each explicit generation action creates at most one report. A normal export that writes both `.xlsx` and `.fasta` still gets one report. BLAST multi-file/export-all mode also gets one report for that action, not one report per generated file.
- Report file names use the generation completion timestamp followed by `_report`, for example `20260505_143012_report.pdf`. The timestamp must be local time, sortable, filesystem-safe, and independent of the exported data file base name.
- Export settings modals must size themselves from actual content. Do not hard-code a height that can clip checkboxes or leave large dead space after adding options. The file-type section must always show all available options, and its copy must explain that normal Excel exports selected rows while raw Excel writes every table row, including unchecked rows, to a separate `_raw.xlsx` file.
- The export settings modal must expose FASTA header format as a dropdown, not a checkbox. Options are `Use phgo FASTA headers` (default), `Use original FASTA headers`, and `Use minimal ID-only headers`; the selected mode must propagate through prompt settings, workflow settings, BLAST single export, BLAST all-table export, Family BLAST export, keyword export, normal FASTA, and raw FASTA. The same modal must expose the Family BLAST FASTA prepend-only-first-query option only when the export target is a Family BLAST result.
- Canvas mode preserves sequence payloads as first-class data. Every canvas row imported from FASTA, keyword rows, BLAST rows, or snapshots must keep the available FASTA/sequence material so later tree-building and the detail FASTA tab can work without re-fetching. Rows that genuinely have no sequence remain visible but cannot be selected.
- Canvas-driven tree building must operate only on already selected, sequence-ready canvas rows in visible canvas order. The tree workflow should preserve the exact input FASTA used, the `mega-phgo-runtime` request/response pair used, the aligned output, the inferred tree output, and Reactree viewer payload needed to reopen the same tree review state from a snapshot without recomputation. Full Reactree visual-edit UI state is owned by the browser viewer and is exported/restored through the independent text `.pgv` PHgo Viewer Snapshot format, not by faking viewer internals in the TUI.
- Canvas tables use a split schema:
  - default fixed columns are row number, source type, head, species, label_name, geneid, blast_labelname, blast_geneid, and display name
  - NCBI keyword-origin Canvas rows map fixed `geneid` from `Gene locus` first, while BLAST source label/gene columns stay blank because keyword rows are not BLAST-derived
  - the system-tree display-column selector includes `PHgo label format`, defined as `speciesInitials-geneid (label_name)` with missing parts rendered as `~`; `YT label format`, defined as `gene_locus_speciesInitialsLabelName` without duplicating an existing species-initial prefix in `label_name`; and `YT v2 label format`, defined as `gene_locus_labelName` after removing that species-initial prefix from `label_name`. When `label_name` is empty, both YT formats are only `gene_locus` with no trailing underscore. The PHgo label format is the default display-name source; species initials are the first letters of the first two species words, preserving source casing
  - canvas items imported from result tables or snapshot-backed whole-workflow additions may append the source table's full visible columns after the fixed columns
  - repeated columns are acceptable except for `label_name`, which must stay only in the fixed section
  - the system-tree display-column selector and tree computation must continue to use only the fixed column set and must ignore appended activity columns entirely
- Add canvas is the full import entry: it accepts FASTA/phgo FASTA plus `.pgo` session snapshots and turns snapshot result tables into complete new canvas items. Add rows is FASTA/phgo FASTA only; if a `.pgo` path, dropped `.pgo` file, or resolvable output snapshot name reaches Add rows, show a clear error telling the user to use Add canvas instead. Add rows appends FASTA rows to the active canvas table with row numbers continuing from the existing maximum.
- Canvas item titles follow the source contract exactly. Creating a canvas from result tables uses only rows selected at creation time: sources with a left sidebar preserve each original left-sidebar title, while no-sidebar sources such as keyword, family, and single-file BLAST use numeric titles. Multi-file BLAST remains a left-sidebar source even after family merging leaves a single visible result table; its Canvas title is the original sidebar main title plus the merged family subtitle first line, e.g. `AT2G37040.1[PAL]`. Adding or creating a canvas from a `.pgo` snapshot preserves saved left-sidebar titles when present; no-sidebar snapshot sources use the shortened snapshot filename. Adding or creating a canvas from a FASTA/text file uses the shortened source filename; pasted FASTA text uses a numeric title.
- Canvas export settings must support both snapshot save and FASTA export. Canvas FASTA export writes one `.fasta` file containing every selected canvas row in visible canvas order and row order, without prepended BLAST query/source records. Header mode follows the shared export modal options: prefer phgo headers when enough metadata exists, but fall back to the stored original FASTA header when a phgo header cannot be built.
- Canvas edit buttons after Back/Home/Copy are always Add, Delete, Rename in that order with the same shortcuts in list and table modes. List mode targets canvases; table mode targets rows/cells, with Rename becoming Alias only on `label_name` cells. Header mode must hide those edit buttons and disable their shortcuts. These edit buttons are normal-color left-side buttons, not right-side primary buttons.
- Detail FASTA tabs across the program should use already stored sequence data directly and only invoke loader callbacks when sequence text is absent. Snapshot-opened and canvas-imported rows must preserve any available sequence payloads for this path.

## Data Analysis Report Design

- Mode-specific report design belongs in dedicated files under `doc2/reports/`.
- Keyword-mode report design is specified in `doc2/reports/keyword-report-design.md`; treat that file as the source of truth for keyword report content, order, chart choices, layout rules, provenance rules, performance sections, quality checks, and implementation data model hints.
- Future BLAST, pathway, or other report types should get their own files in `doc2/reports/` instead of expanding this agent note.
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
  - progression buttons such as Start, Select, Search, Run BLAST, Apply, Save, View, and Export belong at the far right of the button row
  - progression buttons must use the same blue as the focused module border/title, distinct from normal dark-blue buttons
  - button text should be concise and verb-led; avoid vague labels like `Go`, `Use`, `Choose`, `Next`, `Continue`, `Confirm`, `Next / Start`, or verbose `Use ...`
  - button labels and shortcuts must come from the shared button dictionary in `internal/tui/buttons.go` whenever a standard action exists
  - standard button dictionary:
    - navigation: `Back (Esc)`, `Home (Ctrl+O)`
    - modal dismissal: `Close (Esc)`, `OK (Enter)`
    - selection/progression: `Start (Enter)`, `Select (Enter)`, `Search (Enter)`, `Run BLAST (Ctrl+B)` when launched from a result table and `Run BLAST (Enter)` when it is the primary action of a dedicated BLAST start page, `Apply (Enter)`, `Auto identify (Enter)` when it is the primary empty-input action, `Save (Enter)`, `View (Enter)`
    - export/results: `Export (Ctrl+G)`; multi-table pages choose current table versus all tables inside the export scope modal instead of showing `Export all`
    - table selection shortcuts only, not visible buttons: `Ctrl+A` select all, `Ctrl+N` clear all, `Space` toggle current row
    - editing/recovery: `Paste (Ctrl+V)`, `Retry (Ctrl+R)`, `Skip (Ctrl+R)` when it is a secondary input-page action, `Install (Enter)`, `Yes (Enter)`, `No`
  - identical or similar actions must use identical labels across all pages; export actions are always `Export`, not `Generate file`, `Write file`, or `Create`
  - every interactive page after the startup page should expose a Home button that returns to database/startup selection
  - every full-screen page that exposes Back must map Back to the immediately previous full-screen page in the user-visible flow
  - modal close semantics are strict: `Close (Esc)` and close-only `OK (Enter)` in informational or overlay dialogs dismiss only the current dialog/overlay and must not emit `NavBack`/`NavHome` or send the user to the startup/home page; use explicit `Back`/`Home` buttons only for real page navigation
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
  - BLAST fallback flow is TUI-owned: for lemna `blastn`, `blastx`, `tblastn`, and `blastp`, try online BLAST first whenever the server capability exists; if online submission/result automation fails or server capability is absent, continue to the matching local BLAST+ fallback when local FASTA is available
  - if local BLAST+ tools are missing, close the fallback decision and show a dedicated BLAST+ install modal; installation itself runs in a loading modal, then local BLAST resumes in its own progress/status modal
  - use a long single-choice list only when there are many choices or each choice needs long explanatory secondary text
  - use tview widgets such as `Pages`, `Modal`, `TextView`, and shared buttons for dialogs; do not print ad-hoc messages to stdout during interactive workflows
  - `internal/ui` stdout spinner/progress helpers are forbidden for interactive workflow code; remove the package if it exists only for stdout UI
- Export settings:
  - entering the file-generation/export subflow must open a modal dialog over the current result table, not a new full-screen page
  - collect export file name, optional output folder, and data analysis report choice in one larger modal when export begins
  - omit the output-folder module when that mode does not use a folder; when present, leaving it blank means write files directly to the program output directory
  - checkbox-style choices, including the data analysis report choice, use the shared `[x]` / `[ ]` selection style: selected/on is green, unselected/off is red, and Space toggles the focused checkbox
  - the export settings modal uses stacked modules like the startup page: Tab switches modules, Enter advances modules, and Enter on the final module activates the primary export action; key events must be trapped by the modal so arrows cannot leak into the result table behind it. Inside grouped modules, Up/Down moves between controls, Space toggles checkboxes or opens dropdowns, and dropdowns must support both Up and Down selection while open.
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
  - accept one query per line, FASTA entries, supported source report/browser URLs such as Phytozome report URLs and lemna.org JBrowse gene links, or a keyword list copied from `list`
  - accept `load "file.txt"` from the program directory as a batch input source
  - when more than one query is supplied, validate that pasted label names are either present for all items or omitted for all items
  - allow `~` as the explicit blank placeholder for per-query labels
  - after external-reference settings, detect Family BLAST/query-group patterns generically, such as `NAME1/NAME2/NAME3`, `PREFIX10/PREFIX10-like`, or `GROUP.1/GROUP2`; if at least two queries share a detected family prefix, open the Family BLAST settings modal with Family BLAST enabled by default
  - Family BLAST does not require external references to run; it groups the review/export unit by gene family while still executing BLAST per query. External references still control evidence columns, and the automatic filter remains gated on full external-reference availability
  - Family BLAST settings must present grouping and merging as two separate concepts. Grouping combines related query members into one review/export unit; merging is a separate default-on switch in the merging section that collapses duplicate/same-target rows by keeping the preferred row. Turning merging off must still keep the family grouped while preserving all member hit rows.
  - Family BLAST must not rewrite or normalize hit-row `label_name` values. It groups by BLAST query/source labels and query gene identity, then derives a separate `family_name` for the grouped review/export unit. Remove trailing member numbering by default. Before that, default-on suffix handling should treat `prefix + number + suffix` query labels such as `GENE10-like` or `GENE10_like` as `GENE10`, so `GENE9/GENE14/GENE10/GENE10-like` group as `GENE`.
  - in Family BLAST, grouped output filenames use the derived family name without member numbering, for example `GENE` instead of `GENE1`, `GENE2`, `GENE3`, `GENE4`; rows are merged by target protein/gene by default, keeping the best hit when one target is hit by multiple member queries
  - Family BLAST export should preserve the grouped family file while including all available query sequence headers for the family, not only the first member query
  - ask for missing per-query source `label_name` values before BLAST execution when they are needed for multi-query review, Family BLAST grouping, query-source metadata, and FASTA query headers; this source-label flow is separate from hit-row symbol name auto-identification in the external-reference modal
  - ask for an optional output folder name only inside the file-generation subflow for batch BLAST export
  - process and confirm BLAST result tables one query at a time
  - keep the BLAST selection table visible during row confirmation, including checkbox markers and row numbers
  - in multi-file BLAST result tables, the left query sidebar shows the gene ID as the primary text. Its secondary yellow text is two lines: first the `label_name`, then the result count such as `5 lines`.
  - support exporting all batch query tables through the multi-table `Export` scope modal; `Ctrl+D` may open the same scope modal with `All tables` preselected, but it must not bypass the modal or appear as a separate visible button
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
  - reuse BLAST-hit symbol name identification results by stable hit/source keys within a run so duplicate HSPs, duplicate target rows, and family-merged tables do not repeat local symbol name ranking work
  - cache UniProt and InterPro lookup results across BLAST runs in the same wizard session, including negative lookups, so repeated hits from related query sources do not repeat external-reference calls
  - remote Phytozome BLAST should default to a small bounded batch concurrency of 2; live replay checks showed 2 workers modestly improves throughput while 3 workers slows down the remote target
  - inside Phytozome keyword searches, keep network parallelism bounded and preserve search semantics; do not use Phytozome keyword lookup as a symbol-name fallback path for other databases
  - cache duplicate labelname batch-ranking requests within a run so repeated alias lists are sorted once while preserving each request's task timestamp and item index in the returned result
  - live keyword -> BLAST throughput is still not monotonic with larger worker caps, but the best setting changes when the full downstream pipeline is enabled. Old single-path sweeps that stopped at BLAST-only stages are no longer sufficient after enabling automatic query `labelname`, BLAST-hit `labelname`, UniProt, and InterPro together.
  - for keyword-result-table -> BLAST full-chain live sweeps, always test with all expensive downstream stages enabled together:
    - keyword auto `symbol name`
    - BLAST query/source auto `symbol name`
    - BLAST-hit auto `symbol name`
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
    - UniProt accession prefetch, UniProt lookup, and InterPro lookup may use more aggressive network fanout when auto BLAST-hit symbol name, UniProt, and InterPro are enabled together
    - CPU/disk-heavy phases should keep their own more conservative worker caps and must not blindly inherit the network enrichment fanout
    - small keyword->BLAST replay batches should keep more conservative default network worker ceilings than large batches; live replay evidence showed that pushing UniProt/InterPro worker counts higher for 3-query / about 60-row runs made the total wall time slightly worse instead of better
  - UniProt and InterPro enrichment progress must expose explicit stage text before the row-level loop begins so long external-reference runs do not look frozen:
    - UniProt should show an accession-prefetch phase before lookup resolution
    - InterPro should distinguish query-reference resolution from hit-reference resolution
  - BLAST query/source symbol name worker strategy:
    - collect local structured request data in parallel, de-duplicate equivalent `AliasRankRequest` inputs, then rank the batch through the local symbol-name database
    - do not call single-item `RankAliases` during batch candidate collection; construct requests only, then use one `RankAliasBatch` pass so batch de-duplication and family coordination are preserved
    - progress text should explicitly say `Collecting BLAST source label candidates...` and then `Ranking BLAST source labels...`
  - Keyword-to-BLAST real-flow performance work should be measured separately from BLAST execution:
    - keyword search
    - keyword auto symbol name
    - selected-row to BLAST-item resolution
    - BLAST query/source auto symbol name + alias supplement
    - only after that, BLAST execution and external references
    - keep a dedicated live breakdown test so worker tuning can target the real pre-BLAST bottleneck instead of only the final BLAST stage
    - when `suppressTaskModals` is true, keyword-row to BLAST-item resolution should also run headless without wrapping an extra progress modal so replay/perf runs do not spend extra TUI orchestration overhead on the already-instrumented fast path
  - BLAST-hit symbol name worker strategy:
    - collect structured hit requests from existing BLAST/reference metadata without remote keyword-row prefetch
    - de-duplicate identical hits by stable identification cache key before ranking
    - batch-rank unresolved hit alias requests and then fan the decision back onto all duplicate rows/HSPs
    - progress text should explicitly say `Collecting BLAST hit symbol requests...`, `Ranking BLAST hit label aliases...`, and `Resolved BLAST hit label names... x/y`
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

- The static FASTA Header Tools page at `docs/wt.html` is maintained under the contract in `doc2/fasta-header-tools/README.md`. Before changing that page, read this entry and the full FASTA Header Tools document library. Keep the page static and preserve its inherited FrontPage visual layout; modernization belongs only in the internal HTML semantics and browser-side behavior. Update that library whenever its UI contract, FASTA transformation rules, browser/export limits, or test coverage changes.
  - The `docs/wt.html` page header is a locked compatibility boundary: it must retain the same legacy table structure and visual geometry as `docs/index.html`. In particular, do not add a `<!doctype>` (the page must render as `document.compatMode === "BackCompat"`), and preserve the outer `760`-pixel table, `160`-pixel banner, nested `150`-pixel banner table, and `50`-pixel navigation row. Only the navigation selection may differ, with `TOOLS` rendered as the selected item.
  - Keep all tool behavior in `docs/wt.js`; do not change the shared header/body layout merely to implement or refactor tool behavior. Before publishing any `wt.html` change, run `powershell -NoProfile -ExecutionPolicy Bypass -File scripts\check-wt-header.ps1` and compare the two pages in a browser: compatibility mode, outer page width, banner height, and navigation height must match.
- `README.md` must function as a real operator guide, not a short feature list.
- The README should explain the product through step-by-step usage, realistic examples, and user-facing outcomes.
- Document both `phytozome` and `lemna` workflows in plain language, including:
  - what each step asks
  - why the step exists
  - what kind of input is expected
  - what files are generated at the end
- Prefer concrete examples such as rice or soybean gene IDs, Phytozome report URLs, copied `list` output blocks, and batch BLAST usage instead of abstract placeholders.
- Do not reuse user-provided identifiers, species combinations, pathway panels, or research-specific examples in public documentation. When writing README examples, use fresh public examples that did not come from the user's prior workflow.
- Keep screenshots and README images that are part of the published website in a stable repository location such as `docs/images/`.
- Whenever output paths, cache paths, batch behavior, or recovery commands change, update the README and this file in the same change.
- Treat repository-root `docs/` as the GitHub Pages website root only. Keep HTML, favicon/logo/image assets, sitemap/robots, `.nojekyll`, and FrontPage-style `_vti_*` website metadata there. Do not add project design notes, implementation plans, audit references, or other Markdown documentation under `docs/`; new documentation belongs under `doc2/`.
- Treat repository-root `doc2/` as the project documentation root. Keep design docs, implementation notes, audit/reference libraries, report specs, snapshot docs, and future Markdown documentation under `doc2/`, preserving topic subdirectories such as `doc2/phylogenetic-tree/`, `doc2/ncbi/`, and `doc2/reports/`.
- Do not delete, clean, sanitize, filter, or specially whitelist the `docs/` website subtree during build/release work; keep release publishes subject to the same clean-worktree requirement as the rest of the repository.
- `docs/nac.html` is the embedded website changelog page. Every release publish must sync GitHub release history into that page with the same flat paragraph format as the existing `test` sample: newest release first, every title/body line rendered with the same `<p>...</p>` style, and exactly one blank separator line between release entries.
- The website favicon should use the small product icon variant. Keep `docs/favicon.png` aligned with `docs/logo3small.png`, and keep the homepage pointing at that favicon.
- Keep GitHub Pages publishing compatible with the `docs/` website root: preserve `docs/.nojekyll`, and do not reintroduce broken git submodule/gitlink entries such as `.codex/tripal_blast` that can make Actions checkout fail before Pages builds even start.
- Release packaging rules:
  - use the fixed release build template instead of hand-running ad hoc build commands:
    - Codex must directly use `powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-codex.ps1`
    - local artifact build/test: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-codex.ps1`
    - GitHub release build after committing the intended changes: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-codex.ps1 -Publish`
    - optional explicit version format: `-BuildVersion vYYYYMMDDTHHMMSSZ`
  - `scripts\build-codex.ps1` is the only normal Codex-facing build entrypoint: without `-Publish` it must run the Windows-only development build/test flow and write artifacts only under `bin\`; with `-Publish` it may forward to the full cross-platform release build
  - the new main interface is enabled in code, not by packaging environment variables or WezTerm config. Do not reintroduce `PHGO_NEW_MAIN_INTERFACE` or `PHGO_BUNDLE_NEW_MAIN_INTERFACE_DEFAULT` as packaging-time requirements.
  - local development builds/tests must stay Windows-only; do not reintroduce Linux/macOS dev helper build entrypoints
  - do not write casual development executables into the repository root; keep generated build/test artifacts under `bin\`
  - `scripts\build-release.ps1` owns clearing `bin/`, rebuilding all supported release artifacts, validating package contents, extracting/verifying Windows icons, and writing `bin\SHA256SUMS.txt`
  - release publishing must also update `docs/nac.html` from GitHub release notes before packaging/pushing, so the embedded website changelog stays in sync with released titles and bodies
  - release publishing must not exclude or special-case the website files under `docs/`; upload the whole website subtree together with normal repository changes
  - release-time WezTerm asset downloads must tolerate transient upstream CDN/GitHub failures such as HTTP `504`; keep retry/backoff logic in the packaging path instead of treating one timeout as a hard release failure
  - keep release assets aligned with the actual executable names documented in the README
  - all desktop release assets should use the bundled `WezTerm` runtime model instead of shipping bare `linux` or `darwin` binaries
  - macOS release artifacts should be normal app bundles packaged as archives containing `phytozome GO.app`, not raw `darwin` executables
  - `assets/mega-phgo-runtime/<platform>/runtime/` remains the repository-side source of pre-extracted runtime directories. Each populated runtime directory must contain the PHgo runtime executable plus the runtime-owned MUSCLE binary (`muscleWin64.bin`, `muscleUnix64.exe`, or `muscledarwin64`); do not keep `.deb` or `.pkg` installers in the repository or use them at application runtime.
  - Windows release packaging must copy the contents of `assets/mega-phgo-runtime/windows-amd64/runtime/` into the bundle root, so the shipped app works without any runtime download step and without a separate runtime folder.
  - Linux/macOS release bundles must not claim bundled PHgo runtime support. Those platforms should report system-tree unsupported until matching native runtime builds are intentionally added.
  - If future Linux/macOS support is explicitly requested, those runtime folders must contain real PHgo runtime executables built on matching platforms or with a configured Lazarus/FPC cross toolchain; never use MUSCLE-only folders, MEGA-CC binaries, installed MEGA files, or renamed placeholders to satisfy release validation.
  - release builds must rebuild and sync the Vite/React `tree-viewer` assets before packaging so the embedded Reactree viewer is current
  - Windows WezTerm bundles must build one `.ico` stack from `docs/logo3small.png` and `docs/logo3large.png`: Windows shell small/list/details slots (`16`, `20`, `24`, `32`, `40`) must use `docs/logo3small.png`, while larger icon slots (`48`, `64`, `128`, `256`) use `docs/logo3large.png`; apply that `.ico` to both the launcher executable icon and the embedded WezTerm window executable icon as executable resources only
  - do not package source logo image files in release zips; specifically, `docs/logo.png`, `docs/logo2.png`, `docs/logo3large.png`, `docs/logo3small.png`, `logo.png`, `logo2.png`, `logo3large.png`, `logo3small.png`, and `phytozome-go-window-icon.png` must not appear in `phytozome-go_windows_amd64_wezterm.zip`
  - the README lead image must use `docs/logo3large.png`
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
- peptide FASTA export

Notes specific to lemna:
- `lemna` downloads and keyword export are already supported.
- The public lemna.org BLAST form exposes Query Type, Database Type, and BLAST Program combinations (blastn, blastx, tblastn, blastp). Some DB selectors may not expose protein DBs publicly; treat that as a server capability constraint and offer local BLAST fallback.

### Keyword mode

- species search and single-species selection
- multi-keyword input
- grouped search results in input order
- interactive row selection
- Excel export with full result metadata
- peptide FASTA export for selected rows

In lemna mode, keyword search reads the selected lemna.org release GFF3 and AHRD annotation archive from the public download directory, exports standard columns plus all discovered GFF3 columns, attributes, and AHRD fields. Peptide export resolves sequences from the release protein FASTA when available.

## Reliability requirements and isolation

- Keep Phytozome-specific scraping and parsing isolated in `internal/phytozome`.
- Keep lemna.org download/GFF3/FASTA parsing and BLAST form handling isolated in `internal/lemna`.
- Keep workflow code dependent on a shared `internal/source.DataSource` interface where practical so higher-level flows remain unchanged.
- Prefer stable endpoints (the download directory) over fragile homepage or JS-bundled form parsing.
- Any homepage or frontend-bundle scraping is fallback-only and must be isolated and explicit about fragility and caching.
- Row-level source database webpage URLs such as `gene_report_url` must be authentic. Never synthesize, infer, or template-build a source webpage URL from gene IDs, accessions, JBrowse names, coordinates, release directories, or known-looking route patterns. Save a row-level webpage URL only when that exact page URL was obtained during the workflow from a user-provided report/browser URL or an explicit source response field/link. If a mode/database path cannot obtain a real row page URL, leave the field empty. Download/API/source-file URLs may still be recorded in separate provenance fields such as `lemna_gff_url`; do not overload row-level webpage fields with release URLs or traceability fallbacks.
- Search engines must not contain hardcoded term redirects or curated mappings from particular user search terms to fixed genes, loci, labels, or fallback query strings. Search results must come from the selected source's actual identifier, label, keyword, API, or local-index lookup path for the user's submitted term, not from source-code dictionaries of special cases.
- Avoid changing user-visible workflows unless explicitly requested.
- TAIR keyword/detail behavior is strict by selected release. TAIR10, TAIR9, TAIR8, TAIR7, TAIR11, and Araport11 keyword/detail lookup must use only the selected release's configured local GFF/FASTA assets; after a local release index miss, do not fall back to live TAIR search or another TAIR version. TAIR12 uses exact ENA PRJEB100887 queries and must not fall back to TAIR bulk assets. No result means no result for that selected version.
- TAIR historical release keyword searches should reuse the release index through memory, disk cache, and singleflight. Batch keyword worker count is separately tunable with `PHGO_TAIR_RELEASE_KEYWORD_WORKERS` and defaults high because concurrent requests coalesce into one index build and then become local lookups. TAIR12 ENA keeps its own remote setting `PHGO_TAIR_ENA_KEYWORD_WORKERS`.
- TAIR BLAST for TAIR10 through TAIR7 is local BLAST+ only against the selected release's configured official protein or nucleotide FASTA. Do not invent server fallback. Prepared BLAST databases and FASTA header indexes must be cached and reused; BLAST execution threads are controlled by `PHGO_TAIR_BLAST_THREADS` when present.

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
   - For every lemna BLAST program (`blastn`, `blastx`, `tblastn`, `blastp`), if both online server capability and local FASTA capability are available, choose online first. Local BLAST+ is the fallback path, not a program-specific first choice.
   - Prefer server submission when:
     - The server exposes the required DB id(s).
     - The server form can be parsed reliably and submission can be polled.
   - Use local BLAST fallback when:
     - Server DB is not exposed for the requested program (particularly protein DB may be absent).
     - Server form parsing/submission/result retrieval is fragile/unreliable.
   - Current implementation treats server BLAST as the first execution path whenever capability detection says it is available; if automated result retrieval cannot be guaranteed, it falls back to local BLAST when matching FASTA + BLAST+ are available.
   - Do not pre-download FASTA or build local BLAST databases while the online path is still viable; defer local work until fallback is actually needed so the common online-first path stays fast.
   - Always surface the capability and the chosen path to the user.

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
   - Query Type (Nucleotide / Protein)  - prefilled based on input sequence if possible.
   - Database Type (Nucleotide / Protein)  - only allowed combos shown.
   - Program  - inferred or chosen from available programs.
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

- TUI shortcut safety: multiline bracketed paste may deliver embedded line breaks as `Ctrl+J`/line-feed events. `Ctrl+J` must never be treated as `Ctrl+Enter` or any submit action. Clear/new-group actions use `Ctrl+Shift+N`; do not introduce bare `Ctrl+N` or `Ctrl+M` shortcuts that can collide with terminal paste/edit control characters.

- The website's `docs/wt.html` FASTA Header Tools includes a Batch FASTA Record Filter task. It takes one case-insensitive header substring per line and can either remove every matching FASTA record or retain only matching records; record parsing must preserve complete records and support normal adjacent FASTA entries as well as entries separated by blank lines.

- FASTA output rule: every FASTA header generated by PHgo, including normal exports, Canvas exports, PHgo-format headers, source-original-header mode, temporary local-BLAST query FASTA, and MEGA/PHgo-runtime FASTA, must contain no whitespace. Replace every header-space, tab, or line break with `_` at the final write boundary. FASTA readers/importers must remain backward compatible with externally supplied or legacy headers containing spaces; never require callers to pre-normalize input headers.
- Exported `.xlsx`, `.fasta`, PDF data analysis reports, and `.pgo` session snapshots are written to the user-selected export directory. Export flows should open the operating system's folder picker before writing files; the picker defaults to an `output/` directory next to the executable and creates that default directory if missing. If the platform cannot show a system folder picker, fall back to the default `output/` directory instead of blocking export.
- If an export mode still asks for an extra output folder name, treat it as an optional subfolder inside the user-selected export directory, not as a folder forced under the application `output/` directory.
- Large multiline input pages that explicitly accept file paths, dropped files, or file-backed pasted content must include an `Open...` button in the right-side action button group, placed before the main action button, with a keyboard shortcut. The button must use the operating system's file picker and insert the selected file path into the input box. Do not add this button to keyword search input or keyword label/Gene-locus helper inputs.
- Runtime caches must live under a single hidden-capable `.cache/` directory next to the executable, not scattered across OS temp or user cache roots.
- Canvas tree runtime artifacts and snapshot-restored tree files must live under `.cache/tree/...`, not under `output/`.
- Opening older snapshots that still record legacy `output/tree/...` restore paths must remap those tree artifacts into `.cache/tree/...` instead of recreating the old output-side tree cache.
- Cache layout rule:
  - `appfs.CacheRoot()` is the single source of truth for the app-local cache root
  - packaged desktop startup uses a dedicated `[0]` helper tab that cleans only `.cache`, checks GitHub releases for a newer packaged bundle, checks/updates the symbol-name database, and only then spawns the main wizard through `core.bin` in a fresh tab in the same WezTerm window and exits. User-created tabs inside that same WezTerm session bypass cache cleaning and launch the main wizard directly, but must wait on startup-state initialization when tab `[0]` has not completed.
  - modules may also expose narrower subtree cleanup for known-bad artifacts, but that is a supplement to the startup cleanup path, not a replacement
  - Windows bundled WezTerm runtime extraction must use a fresh temp runtime directory per launcher start instead of reusing one fixed `%TEMP%` path, otherwise `conpty.dll`, `OpenConsole.exe`, or other runtime files can remain locked by a still-closing old terminal process and block relaunch
- Lemna local BLAST FASTA assets should live under `.cache/lemna/localblast/<jbrowseName>/<release>/`. BLAST database index files may use `.cache/lemna/localblastdb/<dbtype_hash>/` when the species/release cache path would make Windows BLAST+ output prefixes too long; `makeblastdb` itself should build in a short OS temp directory and then move complete DB files into the app-local cache.
- Persistent Phytozome caches should live under `.cache/phytozome/...`.

## Documentation Center

- The public static Documentation Center is rooted at `docs/dc.html`. Its topic
  pages live under `docs/dc/` and retain the legacy layout in
  `docs/dc/dc_sample.html`.
- `docs/dc/CONTENT_MODEL.md` defines the navigation and writing contract.
  `docs/dc/generate-docs.js` is the maintainable source for generated overview
  and topic pages; run `node docs/dc/generate-docs.js` after changing its topic
  data, then verify internal links and `git diff --check`.
- Documentation must distinguish released behavior from planned behavior. In
  particular, system-tree computation is Windows-amd64-only, the Symbol Name
  database is installed from a prebuilt manifest/release-asset path, and no
  numeric public error-code registry exists.

## Current implementation status

- Done:
  - Startup database selection.
  - `source.DataSource` abstraction.
  - lemna species discovery from `download/`, with official clone marking.
  - Small lemna species lists are shown directly instead of forcing search.
  - lemna keyword search from GFF3 + AHRD with dynamic export columns.
  - lemna BLAST capability detection from download assets and concrete BLAST selector pages.
  - local BLAST fallback for `blastn`, `blastx`, `tblastn`, and `blastp` using the matching nucleotide/protein FASTA.
- Single-query and batch BLAST execution now run behind cancellable task/progress modals; local Lemna BLAST reports FASTA preparation, database build, BLAST execution, result parsing, and release-metadata enrichment instead of silently continuing after BLAST+ exits.
- BLAST submission errors support retry, back to BLAST program selection, or exit.
- Keyword/BLAST result transfer into BLAST target selection preserves transfer state across Back navigation: Back from target species or BLAST run returns to the transfer database choice instead of dropping the user into a broken target-source state.
- Keyword-result-to-BLAST transfer sanitizes legacy raw alias/symbol fields from cached query sources while preserving `label_name`, `phgo_alias`, IDs, sequences, URLs, UniProt accession, and source metadata needed for BLAST.
- Cancellable task/progress modals must cancel their context and wait for the associated task goroutine to finish before returning control. Cancel handlers must not synchronously queue a UI update from inside the same TUI input event, because that can deadlock the modal.
- Remaining:
  - Robust server-side lemna result retrieval. Server form probing exists, but local BLAST remains the dependable execution path.
  - Optional integration tests against live lemna.org and local BLAST+ when available.

---
End of agent notes.


# Reactree Viewer Service

## Purpose

The tree viewer service is an external browser-facing subsystem. It uses Reactree.js as the tree rendering and interaction core.

The browser is the user interaction surface. The viewer service owns rendering, preview updates, and graphical export.

## Reactree.js Basis

Reactree.js is selected because it provides a React component for interactive phylogenetic visualization from Newick strings. Its documented capabilities include:

- Newick input
- rectangular and circular/radial layouts
- D3-powered zoom and pan
- reroot, flip, swap, collapse, and clade coloring
- sequence alignment panel from multi-FASTA input
- bootstrap and branch length display
- search and highlight
- SVG, PNG, PDF, and Newick export
- CSS theming
- MIT license

References:

- https://reactree.vaaloo.fr/
- https://reactree.vaaloo.fr/docs

## Service Shape

The viewer should be a standalone TypeScript/React application served by a local service.

Current implementation note:

- The first shippable viewer service is hosted by the Go tree subsystem while preserving the same HTTP/SSE contract.
- Its browser page is a bundled `tree-viewer` React application built with Vite and `reactreejs`.
- The React app accepts Newick plus metadata, applies MEGA Tree Explorer's source-defined display ordering only for MEGA PHgo runtime inferred-tree payloads, maps stable taxon IDs to `display_name`, passes aligned FASTA into Reactree's alignment panel, and relies on Reactree's own toolbar for layout, search, editing, and SVG/PNG/PDF/Newick export. The display adapter reconstructs MEGA `TTreeList` Newick folding; for ordinary single-tree NJ/ME/ML/MP runtime results it runs one `SetMaxLength`/`SearchMidPoint`/`ChangeRoot` midpoint step when branch lengths exist, even when the exported Newick root is binary-shaped, then applies `SortBranchByFigure`. Topology-only MP does not midpoint-root, rooted UPGMA keeps its root, multi-tree payloads use input order, and standalone imported Newick is not silently reordered.
- Before data enters Reactree, the adapter converts `display_name` values into Reactree-safe labels by removing Newick-breaking punctuation/whitespace and adding deterministic stable-taxon suffixes when two display names would collide after sanitization. The same collision-safe label map is applied to both Newick leaves and aligned FASTA headers so the tree and alignment panel stay synchronized.
- The Go service embeds the built static assets under `internal/phylo/viewer_assets` and serves them locally. It does not use a CDN or remote website.
- The page supports an empty initial state and updates automatically through Server-Sent Events after `Refresh tree`.
- Canvas live sessions do not expose a top-right local `.pgv` open button. `.pgv` files are opened through the standalone tree-browser workflow so Canvas preview state cannot drift away from the current PHgo payload.
- The page intentionally has no PHgo hero/header/status card, no notice strip, no rounded outer viewer card, and no extra empty-state chrome. Before the first rendered tree, the browser page may be visually blank.
- Errors and warnings are shown as modal dialogs. Informational helper text should not occupy permanent page space.
- The Reactree component fills the full browser viewport. The tree canvas uses a white background and Reactree's blue light-theme accent colors.
- Reactree's built-in vertical tree-height resize handle is hidden in PHgo because the viewer already tracks the browser viewport and should always occupy the available height.
- When Reactree's Alignment panel is open, PHgo adds a vertical splitter between the tree pane and the alignment pane so the user can drag horizontally to resize the two panes.
- The repository-local `reactree-0.1.0.tgz` is not the selected viewer package. It is an old generic tree-view package and should not be wired into the phylogenetic tree viewer.

Current layout:

```text
tree-viewer/
  package.json
  index.html
  src/
    main.jsx
    style.css
internal/phylo/viewer_assets/
  index.html
  assets/
```

The Go TUI launches the service and opens the browser to:

```text
http://127.0.0.1:<port>/sessions/<session_id>
```

The service must support an empty initial state. It should not require a tree before the browser page opens.

The page should show Reactree's own toolbar and controls rather than a custom mirror of those controls inside the TUI. It should otherwise stay visually minimal: Reactree's toolbar and main interaction surface are the entire page.

`scripts/build-tree-viewer.ps1` runs `npm install`, builds the Vite app, and copies `tree-viewer/dist` into `internal/phylo/viewer_assets`.

## Communication

The first implementation should support both:

- HTTP requests for explicit refresh/update
- Server-Sent Events or WebSocket for live preview updates

Recommended minimal API:

```text
GET  /health
GET  /sessions/{session_id}
PUT  /sessions/{session_id}/payload
PUT  /sessions/{session_id}/preview
GET  /events/{session_id}
```

`PUT /payload` replaces the current tree payload. The browser should update automatically through the event stream.

`PUT /preview` updates viewer-only settings without requiring new runtime artifacts.

## Payload Model

The viewer receives:

- Newick tree where leaf names are stable taxon IDs
- aligned FASTA with matching stable taxon IDs when available
- metadata mapping stable taxon IDs to display names and table values
- page-state configuration

Reactree.js receives Newick and aligned FASTA. The viewer adapter is responsible for mapping leaf IDs to display names in the rendered view.

If Reactree.js does not expose a direct label-rendering hook, the viewer service must transform the Newick leaf names from stable IDs to collision-safe display labels before passing Newick to Reactree, while retaining a reverse map for export and state recovery.

Current adapter behavior:

- Metadata and PHgo artifacts remain authoritative and keep stable taxon IDs.
- Raw `display_name` values are not written back into computation artifacts.
- Reactree-facing labels are sanitized only at the browser adapter boundary.
- If `PAL 1` and `PAL_1` both sanitize to `PAL_1`, the later duplicate receives a deterministic suffix derived from its stable taxon ID, such as `PAL_1__PHGOT000002`.

## Preview Configuration

Preview settings are controlled from the Reactree page, not primarily from the TUI panel.

Required preview fields:

- layout
- display-name mode
- show branch lengths
- show bootstrap values
- show alignment panel
- theme/accent compatibility if needed

Browser-side Reactree controls are authoritative for interactive inspection and export. TUI-pushed configuration only seeds the initial state and current data payload.

Node-edit commands use persistent modes. Reroot and Flip remain active after each
completed node edit; Swap remains active after each completed sibling pair; and
Color remains active after each color edit. Click the active command again to
leave it, select another mode to switch modes, or press Escape to cancel the
current mode. Midpoint and ladderize apply immediately and are not modes.

PHgo viewer styling should prefer Reactree's official/default light theme. Overrides are limited to removing PHgo outer chrome, keeping the preview canvas white, preserving the blue accent family, hiding Reactree's vertical resize handle, and adding the horizontal alignment splitter.

## Live Update Rule

After `Refresh tree`, the browser should update without manual reload.

Preferred path:

1. TUI sends new payload or page-state config to the viewer service.
2. Service stores it as current session state.
3. Service emits a session update event.
4. Browser receives the event.
5. React app reloads the current session payload and rerenders Reactree.

Manual browser refresh should still work and reload the latest session state.

## Export Ownership

All graphical tree export is owned by the Reactree page and the service that hosts it.

Canvas does not provide a separate tree export modal. The user exports from the Reactree page.

The viewer service may still expose export files for integration or snapshot recovery, but the browser toolbar is the primary export surface. The TUI should not manually inspect DOM output.

## State Preservation

The viewer exports complete browser-owned visual state as a text `.pgv` PHgo Viewer Snapshot file.

The snapshot includes:

- the current viewer payload
- current topology after reroot, flip, swap, and ladderize edits
- Office-style ribbon tab/search/menu state
- layout, PHgo/MEGA render style, tree type, label mode, scale, height, font, and stroke settings
- selected graphical export long-edge size
- node/clade colors, collapsed clades, and clade labels
- search, toolbar mode, and zoom state
- PHgo viewer-only state such as alignment split width and browser viewport metadata

The `.pgv` format carries its own `schema_version`. Current writers emit PGV schema v3 and viewer-state schema v3. Readers keep v1/v2 compatibility for existing saved viewer snapshots.

## Packaging

The viewer should be bundled with the application release. It must not depend on a remote website or CDN to run.

The first implementation may use a local Node runtime if packaging already includes or can bundle it. A later implementation may produce a single viewer binary if that simplifies distribution.

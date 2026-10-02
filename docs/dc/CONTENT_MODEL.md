# Documentation Center content model

The Documentation Center is a static, MSDN-style reference for **phytozome GO**.
`docs/dc/dc_sample.html` is the visual template. `generate-docs.js` is the single
source used to generate the topic pages; it deliberately preserves the legacy
layout so the site can continue to be hosted as plain files.

## Navigation model

Each sidebar group has an overview article. Its topic entries link to focused
articles. Every article repeats the complete sidebar so it is usable when opened
directly from a search result or a bookmark.

| Group | Overview | Focused articles |
| --- | --- | --- |
| Usage and Maintenance | `usage-maintenance.html` | Windows, Macintosh and Linux, updates, Symbol Name database, BLAST+ components, troubleshooting |
| Getting Started | `getting-started.html` | TUI introduction, home page, multiple tabs |
| Keyword Search | `keyword-search.html` | workflow, results table, export, connectors |
| BLAST | `blast.html` | workflow, modes, family merger, cross-database enhancer, results table, export, connectors |
| Explore | `explore.html` | sessions, canvas, phylogenetic trees, multiple sequence alignment |

## Writing contract

- Use English UI labels exactly as presented by the product where they are known.
- Describe release selection, local caches, source provenance, and failure states;
  never imply that an unavailable source or platform has a fallback that the
  product does not implement.
- Distinguish `geneid`, `protein_id`, and transcript identifiers. Do not expose
  internal `id2` terminology as a user-facing identifier.
- Treat `symbolname.pgd` as a locally installed, prebuilt database. It is not
  generated from NCBI data by an end-user installation.
- For documentation that depends on external services, name the authoritative
  endpoint or provider and explain that availability can change.
- Error messages do not have a published numeric code system. Troubleshooting
  therefore indexes diagnostic classes, the message text to retain, and the
  supported recovery actions rather than inventing unsupported codes.

## Regeneration

Run `node docs/dc/generate-docs.js` from the repository root after changing the
topic data. The generator writes only the HTML pages that it owns under
`docs/dc/`; it does not modify `dc_sample.html`.

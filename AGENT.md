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

## CLI UX standards

- Treat the terminal UI like a commercial wizard: every major step must be easy to scan, self-explanatory, and safe to recover from.
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
  - use `Database selection`, `Mode selection`, `BLAST program selection`, `BLAST execution target`, `Keyword input`, and `BLAST input` as the canonical page titles
  - use `Select ...` for choice prompts and `Enter ...` for free-form text prompts
  - prefer `Review the summary and press Enter to continue.` for confirmation pages
  - prefer sentence-case labels and avoid mixing labels like `Choose ...`, `Pick ...`, and `Enter ...` on the same type of page
- Keep the global navigation commands visible on every interactive page:
  - `back` returns to the previous page
  - `spawn` jumps to mode selection
  - `lobby` jumps to database selection
  - `exit` quits the wizard
- Use clear section boundaries between sessions and major phases:
  - print a visible banner when a wizard session starts
  - print a visible separator and outcome when a session ends
  - keep repeated prompts grouped so the user can immediately tell where they are in the flow
- Support runtime language switching and executable-name locale selection:
  - `lang=en`, `lang=cn`, and `lang=jp` must switch the following prompts immediately
  - executable suffixes `-en`, `-cn`, and `-jp` should select the default language on startup
  - if no suffix or runtime override is present, default to English
  - keep language-specific prompt text and help text aligned across the whole wizard
  - when adding or changing a prompt, update the English text and both translated variants in the same change
  - do not leave new prompt text untranslated in one locale while adding it in another
  - for blank/Enter-only branches, return an explicit placeholder value when the caller expects a slice or list, rather than returning `nil` and relying on downstream indexing
- Prefer descriptive labels over terse jargon:
  - `File name` is preferred over localized or ambiguous labels
  - query input prompts should say exactly what kinds of inputs are accepted
  - list/selection pages should say what the commands do before asking for input
- BLAST batch input rules:
  - accept one query per line, FASTA entries, Phytozome report URLs, or a keyword list copied from `list`
  - accept `load "file.txt"` from the program directory as a batch input source
  - if more than one query is supplied, require a per-query `Protein Identification` unless the pasted list already supplies labels
  - allow `~` as the explicit blank placeholder for per-query labels
  - ask for an optional output folder name when multiple BLAST queries are queued
  - process and confirm BLAST result tables one query at a time
  - keep the BLAST selection table visible during row confirmation, including checkbox markers and row numbers
  - support `done all` to approve the current result selection and auto-approve the remaining batch with default selections
  - accept `doneall` as a convenience alias for `done all`
  - print a run report before writing any export files
- Keyword mode ordering:
  - ask for `Protein Identification` before `File name`
  - preserve one-to-one ordering between keyword terms and `Protein Identification` values
  - allow `~` to mark an intentionally blank identification
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
  - use a progress bar whenever the code knows the number of items being processed
  - use a spinner for single-item or unknown-duration work
  - do not leave long network/file operations without visible feedback
- Performance and concurrency:
  - prefer controlled parallelism for batch-safe stages such as keyword searches, batch query resolution, and sequence fetch preloading
  - keep all interactive prompts serialized even when background work is parallelized
  - apply per-item timeouts to remote work units so a single slow request cannot stall the whole batch indefinitely
  - preserve input order in final tables and exports even when work completes out of order
  - deduplicate identical in-flight work with `singleflight` or equivalent guards so concurrent workers do not download, parse, or build the same artifact twice
  - use persistent local caches for stable remote payloads when that improves later sessions without changing user-visible behavior
  - prefer atomic file writes for shared cache artifacts so background workers cannot observe partial files
- Metadata and result-link semantics:
  - preserve `OriginalInputURL` and `NormalizedURL` when resolving report URLs; do not overwrite them with fetched source structs
  - keep export metadata for query-source URLs separate from row-level target links
- Lemna local BLAST performance:
  - cache release-level AHRD, protein->transcript maps, and FASTA indexes in memory for the current client
  - reuse cached BLAST-ready FASTA artifacts and build the index only once per path
  - avoid duplicate scans of the same release assets within one run
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
- Whenever output paths, cache paths, language switching, batch behavior, or recovery commands change, update the README and this file in the same change.
- Release packaging rules:
  - clear `bin/` before rebuilding release artifacts
  - rebuild all supported platform binaries into `bin/`
  - keep release assets aligned with the actual executable names documented in the README
  - prefer publishing GitHub releases with explicit release notes that summarize user-visible changes and supported platforms

## Current workflows

The wizard asks users to choose a database at startup:

- `phytozome`: original Phytozome-backed behavior
- `lemna`: lemna.org download-backed behavior

### BLAST mode (desired behavior)

- species search and single-species selection
- sequence / FASTA / report URL input
- batch query input from multiple newline-separated items or copied keyword list previews
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

- Exported `.xlsx`, `.txt`, reports, and generated list files are written under an `output/` directory next to the executable being run.
- If the user requests an extra output folder, create it inside `output/`, not beside the executable.
- Runtime caches must live under a single hidden-capable `.cache/` directory next to the executable, not scattered across OS temp or user cache roots.
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

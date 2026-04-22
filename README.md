# phytozome-batch-cli

Cross-platform command-line tool for collecting BLAST results from `phytozome-next.jgi.doe.gov` and exporting selected rows to Excel and FASTA-style text.

## Scope

The first supported workflow is the BLAST path shown in the Phytozome web UI:

1. Search species by keyword
2. Let the user choose one candidate species
3. Submit a BLAST query
4. Fetch the full BLAST results table
5. Add a derived `gene_report_url` column for each row
6. Let the user multi-select rows, with `select all` and `select none`
7. Export only selected rows to:
   - `.xlsx`
   - `.txt` containing peptide sequences in FASTA-like blocks

## Architecture

- Language: Go
- CLI shell: standard library plus thin internal command router
- Network layer: `net/http`
- Parsing layer: isolate all Phytozome-specific scraping and response parsing
- Export layer: Excel writer plus FASTA text writer
- Prompt layer: interactive single-select and multi-select

The project should stay simple, but one detail matters: Phytozome is a JavaScript-heavy site and its UI may change. The scraper should therefore be written behind a narrow adapter so the rest of the CLI does not care whether data came from:

- direct HTTP requests
- parsed HTML responses
- a future browser-automation fallback

## Planned CLI

```text
phytozome-batch-cli blast wizard
phytozome-batch-cli blast plan
phytozome-batch-cli version
```

`blast wizard` is the main interactive workflow.

## Current status

The repository currently contains:

- a workflow spec in `AGENT.md`
- live endpoint notes in `API_NOTES.md`
- a minimal CLI entry point
- interactive species search and selection for `blast wizard`
- BLAST job submission and polling
- XML parsing for BLAST result rows
- interactive row selection with `all` and `none`
- Excel export for selected rows
- peptide text export for selected rows
- derived `gene_report_url` values for result rows

Today the implemented path is:

1. fetch candidate species from `phytozome-next`
2. ask for a species keyword
3. show matching candidates
4. let the user choose one species
5. accept pasted query sequence
6. auto-detect DNA vs protein and choose `BLASTN` or `BLASTP`
7. submit the BLAST job and poll until completion
8. parse the BLAST XML into row records
9. print the returned rows as a terminal table with `gene_report_url`
10. let the user select rows with `all`, `none`, `toggle`, and `done`
11. export selected rows to `.xlsx`
12. fetch peptide sequences and export selected rows to `.txt`

Still pending:

- paging or a more compact selector for large result sets
- better formatting of species labels in BLAST row output
- optional non-interactive flags and output-path control

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
- a minimal CLI entry point
- the initial repository scaffold


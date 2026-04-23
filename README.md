# phytozome GO

`phytozome GO` is a cross-platform Go CLI for running interactive Phytozome searches from the terminal.

<img width="446" height="447" alt="image" src="https://github.com/user-attachments/assets/5ff97faf-5245-4ee8-8b47-2b3027052e53" />

It currently supports:

- BLAST workflow against a selected species
- keyword gene search within a selected species
- interactive row selection
- export to Excel
- export to peptide FASTA-style text

The command name is `phytozome-go`. The product display name is `phytozome GO`.

## Install

Download a release asset for your platform from the GitHub Releases page, extract it, and run the binary:

```text
phytozome-go blast wizard
phytozome-go blast plan
phytozome-go version
```

## Interactive workflows

### BLAST mode

1. Search species by keyword
2. Choose one species
3. Paste a sequence, FASTA entry, or Phytozome gene/transcript report URL
4. Submit the BLAST job
5. Review the returned rows
6. Select rows interactively
7. Export `.xlsx` and `.txt`

### Keyword mode

1. Search species by keyword
2. Choose one species
3. Enter one or more identifiers or keywords
4. Review grouped gene results
5. Select rows interactively
6. Export `.xlsx` and `.txt`

## Export files

The CLI writes output files next to the executable you run.

- `<name>.xlsx`
- `<name>.txt`

BLAST exports include the BLAST table plus derived URLs. Keyword exports include the gene report metadata shown in the selected result rows.

## Project notes

- Language: Go
- Network layer: `net/http`
- Excel export: `excelize`
- Target site: `https://phytozome-next.jgi.doe.gov`

The Phytozome frontend is JavaScript-heavy and subject to upstream changes. The repository keeps Phytozome-specific scraping and parsing isolated behind internal adapters so the CLI workflow remains stable as the site evolves.

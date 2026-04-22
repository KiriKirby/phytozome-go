# Phytozome API Notes

Research date: April 22, 2026

This file records live endpoint findings for the first implementation phase.

## What the JGI Data Portal API is good for

`https://files.jgi.doe.gov/apidoc/` is a Swagger page for the JGI Data Portal API.

It appears useful for:

- listing downloadable files
- searching file metadata
- downloading public data files
- restoring archived files

It does not appear to be the API behind the `phytozome-next` BLAST workflow.

That means:

- it is relevant for future bulk-download features
- it is not the right first integration target for the BLAST-driven CLI

## Confirmed `phytozome-next` endpoints

Confirmed from the live frontend JavaScript bundle.

### BLAST

- `POST /api/blast/submit/sequence`
- `POST /api/blast/submit/sequence-in-file`
- `GET /api/blast/results/{jobId}`

Observed submit fields:

- `targets`
- `targetType`
- `program`
- `eValue`
- `comparisonMatrix`
- `wordLength`
- `alignmentsToShow`
- `allowGaps`
- `filterQuery`
- optional `userEmail`
- optional `emailResults`
- `sequence` or `sequenceInFile`

## Candidate species source

Two viable approaches are visible right now.

### Option A: structured source

Find the internal data source that populates frontend target metadata such as:

- `jbrowseName`
- `displayVersion`
- `organism_abbreviation`
- `proteomeId`

This is still preferred if we can identify it cleanly.

### Option B: project overview fallback

`GET /api/content/project/phytozome` returns JSON sections.

Its `overview` HTML contains many links of the form:

- `/info/{jbrowseName}`

This can be parsed into a first-pass species candidate index if a cleaner endpoint is not found quickly.

Tradeoff:

- fast to implement
- less complete and less structured than the hidden frontend target metadata

## URL findings

The frontend currently constructs a gene-report link as:

- `/report/transcript/{jbrowseName}/{transcriptName}`

This should be verified against live BLAST result rows before the export column is finalized, because user examples used a protein-report URL shape.

## Next implementation target

The next code step should be:

1. source candidate species
2. filter candidates by user-entered keyword
3. let the user choose one candidate

Only after that should we wire BLAST submission and polling.

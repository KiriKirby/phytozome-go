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

## Confirmed live BLAST behavior

Verified against the live site on April 22, 2026.

- public BLAST submission works without a login for at least some targets
- a real protein submission against target `725` returned HTTP-style code `201` and a numeric `job_id`
- polling `GET /api/blast/results/{jobId}` returned:
  - `202` while queued or running
  - `200` with `data.results` containing BLAST XML when complete

Observed BLAST XML `Hit_def` format:

- `{jbrowseName}|{targetId}|{sequenceId}|{transcriptId}|{defline}`

Example shape:

- `T_aestivum_cv__ChineseSpring_v2_1|725|TraesCS7A03G0821400.1|TraesCS7A03G0821400.1|...`

This is enough to derive BLAST protein report URLs:

- `https://phytozome-next.jgi.doe.gov/report/protein/{jbrowseName}/{sequenceId}`

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

The frontend contains multiple report-link patterns in different parts of the site.

For the BLAST protein workflow, the current working link shape is:

- `/report/protein/{jbrowseName}/{sequenceId}`

Transcript report URLs may still matter in other flows, but they are not the correct default for the current BLAST export target.

## Next implementation target

The next code step should be:

1. render the full parsed result table, not only a preview
2. add interactive multi-select
3. export selected rows to Excel and peptide text

Only after that should we wire BLAST submission and polling.

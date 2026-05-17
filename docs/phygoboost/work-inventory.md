# Program Work Inventory

This file is the maintained companion to [Work Classification Standard](./work-classification.md).

The current inventory tracks the maintained work families that matter under the simplified runtime:

- `0`: unmanaged direct helper logic
- `1`: managed work under `phygoboost`

Inside `1`, only the resource kind matters:

- local
- per-domain network

## Inventory rules

- Update this file whenever managed execution boundaries change.
- Add new families when a module introduces new shared managed or managed network stages.
- Remove entries when a family is deleted or merged away.
- Keep descriptions architecture-focused.

## Current work families

| Path / family | Class | Resource | Scope |
| --- | --- | --- | --- |
| `cmd/phytozome-go` entry wrappers | `0` | local direct | CLI bootstrap, argument wiring, top-level wizard entry |
| `internal/appfs` | `0` | local direct | application/output/cache path resolution helpers |
| `internal/cachex` synchronous helpers | `0` | local direct | cache keying, serialization helpers, local convenience wrappers |
| `internal/phygoboost` scheduler/core | `1` | shared managed + network | shared managed grants, per-domain pools, shared HTTP clients, observation |
| `internal/blastplus` install and extraction stages | `1` | local | BLAST+ install, archive extraction, local tool setup |
| `internal/blastplus` download discovery | `1` | network | remote BLAST+ archive discovery and download |
| `internal/export` generated-file writers | `1` | local | Excel writing, sequence text writing, export artifact generation |
| `internal/report` file inspection and PDF rendering | `1` | local | generated-file inspection, metadata capture, report PDF rendering |
| `internal/phytozome` species, search, sequence, and BLAST remote stages | `1` | network | Phytozome species loading, remote fetches, BLAST submit/wait, sequence fetches |
| `internal/lemna` species, release, capability, keyword, download, and BLAST stages | `1` | network or local | lemna remote discovery/download plus local BLAST preparation/execution |
| `internal/searchengine/phytozomekeyword` input classifiers and tiny normalization helpers | `0` | local direct | tiny synchronous search-program selection and identifier/term normalization only |
| `internal/searchengine/phytozomekeyword` program execution, identifier probing, alias expansion, wide fallback, cache fill, and batched search branches | `1` | network | managed keyword search against Phytozome sources, including internal parallel search branches |
| `internal/searchengine/lemnakeyword` input classifiers and tiny normalization helpers | `0` | local direct | tiny synchronous search-program selection and token normalization only |
| `internal/searchengine/lemnakeyword` index loading, release-backed search execution, wide/broad fallback, and cache fill | `1` | network | managed keyword search against lemna release-backed sources |
| `internal/uniprot` | `1` | network | UniProt reference lookups |
| `internal/interpro` | `1` | network | InterPro reference lookups |
| `internal/workflow` input parsing, label helpers, row formatting, and report summarizers | `0` | local direct | cheap orchestration helpers with no managed execution boundary |
| `internal/workflow` disk input loading stages | `1` | local | managed file reads for batch inputs and export support |
| `internal/workflow` remote source submit/wait/search/fetch stages | `1` | network | source-backed BLAST submit/wait, keyword search, sequence fetch, species/capability fetch |
| `internal/workflow` reference enrichment batches | `1` | network | accession prefetch, UniProt batch lookup, InterPro batch lookup |
| `internal/workflow` local BLAST preparation/execution/finalization stages | `1` | shared managed | BLAST preparation, subprocess execution, and finalize work that use the shared managed total pool without a domain |
| `internal/tui` page rendering and view-state helpers | `0` | local direct | synchronous TUI composition, table layout, modal wiring |
| `internal/tui` task execution gates | `1` when executing managed tasks, otherwise `0` | local or network via delegated task | main-process task launching and progress integration without any secondary TUI process |

## Review checklist

Before finishing a `phygoboost`-related change, verify:

1. Did any new shared managed stage appear?
2. Did any new managed network stage appear?
3. Does the inventory still describe the current architecture honestly?

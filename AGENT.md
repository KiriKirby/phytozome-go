# phytozome GO Agent Notes

This file tracks the intended shape of `phytozome GO` and its release packaging.

## Product identity

- Product name: `phytozome GO`
- CLI command: `phytozome-go`
- GitHub repository and Go module: `phytozome-go`

## Current workflows

### BLAST mode

- species search and single-species selection
- sequence / FASTA / report URL input
- BLAST submission and polling
- interactive row selection
- Excel export
- peptide text export

### Keyword mode

- species search and single-species selection
- multi-keyword input
- grouped search results in input order
- interactive row selection
- Excel export with full result metadata
- peptide text export for selected rows

## Reliability requirements

- keep Phytozome-specific scraping and parsing isolated in `internal/phytozome`
- prefer stable API endpoints when available
- keep homepage and frontend-bundle scraping behind fallback logic only
- avoid changing user-visible workflow unless explicitly requested

## Release packaging

Release assets should be built under `bin/` using the `phytozome-go` base name.

Primary binaries:

- Windows amd64: `phytozome-go.exe`
- Linux amd64: `phytozome-go_linux_amd64`
- macOS amd64: `phytozome-go_darwin_amd64`
- macOS arm64: `phytozome-go_darwin_arm64`

Recommended archives:

- `phytozome-go_<version>_windows_amd64.zip`
- `phytozome-go_<version>_linux_amd64.tar.gz`
- `phytozome-go_<version>_darwin_amd64.tar.gz`
- `phytozome-go_<version>_darwin_arm64.tar.gz`

Each archive should contain:

- the platform binary
- `README.md`
- `LICENSE` if one is added later

## Validation

Before a release:

1. run `gofmt`
2. run `go test ./...`
3. run `go vet ./...`
4. build all release targets
5. verify the binaries and archives under `bin/`

## Output behavior

- exported `.xlsx` and `.txt` files are written next to the executable being run
- if the user runs `bin/phytozome-go.exe`, exports will also land in `bin/`

# phytozome-batch-cli

Cross-platform command-line tool for batch downloading and organizing data from phytozome-next.jgi.doe.gov.

## Goals

- Keep the dependency surface small
- Build a single binary for Windows, macOS, and Linux
- Make the workflow scriptable and easy to automate

## Suggested architecture

- Language: Go
- CLI parsing: standard library `flag`
- Networking: `net/http`
- Retry and rate control: small internal helper package
- Data model: plain structs and JSON
- Output: local files and a machine-readable log format

## Roadmap

1. Authenticate and manage session state
2. Discover datasets and batch targets
3. Download files with retry, resume, and progress
4. Normalize naming and output layout
5. Add config file support and dry-run mode


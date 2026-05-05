# phytozome GO Release Notes

## Version

`v20260505T194610Z`

## Highlights

- Expanded desktop workflow coverage for both keyword and BLAST modes, including the newer report-generation pipeline and richer export metadata handling.
- Improved PDF report generation with stronger layout safety, better long-text handling, and navigation-friendly structure suitable for large biological result sets.
- Extended report content around BLAST-specific workflows such as sequence input analysis, filter/family reporting, and external reference integration.
- Continued CLI/TUI integration work across export, workflow, prompt, and report modules so report generation can ship with the main application instead of living as a disconnected sample path.

## Validation

- `go test ./...`
- `go vet ./...`

## Supported Release Assets

- Windows `amd64`
- Linux `amd64`
- macOS `amd64`
- macOS `arm64`

## Packaged Files

- `phytozome-go_v20260505T194610Z_windows_amd64.zip`
- `phytozome-go_v20260505T194610Z_linux_amd64.tar.gz`
- `phytozome-go_v20260505T194610Z_darwin_amd64.tar.gz`
- `phytozome-go_v20260505T194610Z_darwin_arm64.tar.gz`
- `phytozome-go_v20260505T194610Z_all.tar.gz`
- `SHA256SUMS.txt`

## Notes

- This release was packaged locally from the current working tree without creating a git tag or GitHub release entry.
- Existing source edits in the repository were preserved; only generated caches, prior build outputs, and previous release artifacts were cleared before rebuilding.

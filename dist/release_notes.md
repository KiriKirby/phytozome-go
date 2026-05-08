# phytozome GO Release Notes

## Version

`v20260508T042023Z`

## Highlights

- Stabilized the Family BLAST custom grouping modal focus model.
- Custom grouping now opens as a fresh modal flow after closing the Family BLAST settings modal, preventing nested TUI focus leaks.
- Mouse selection in the two-pane grouping editor now switches panes only on the final click event, avoiding the left-pane/right-pane focus flash.
- Preserves stacked child modal behavior inside the custom grouping editor and restores parent selection state after child dialogs close.

## Validation

- `go test ./...`
- `go vet ./...`

## Supported Release Assets

- Windows `amd64`
- Linux `amd64`
- macOS `amd64`
- macOS `arm64`

## Packaged Files

- `phytozome-go_windows_amd64.zip`
- `phytozome-go_linux_amd64.zip`
- `phytozome-go_darwin_amd64.zip`
- `phytozome-go_darwin_arm64.zip`
- `SHA256SUMS.txt`

phytozome GO release

This release packages the current interactive wizard for both `phytozome` and `lemna` workflows.

Included in this release:

- Windows `amd64`
- Linux `amd64`
- macOS `amd64`
- macOS `arm64`

Highlights:

- added first-class `lemna` mode with download-backed species discovery, keyword search, and BLAST fallback behavior
- added runtime language switching with `lang=en`, `lang=cn`, and `lang=jp`
- added executable-name locale selection with `-en`, `-cn`, and `-jp` suffixes
- unified output and cache locations next to the executable
- expanded batch BLAST and keyword workflows, including copied `list` input support
- improved error recovery, progress reporting, local BLAST fallback, and persistent caching
- rewrote the README into a full operator guide with examples and flowcharts
- replaced README examples with fresh public sample cases so release documentation does not reuse workflow-specific research examples

Release assets:

- raw binaries in `bin/`
- packaged archives for direct download on GitHub Releases

Notes:

- local BLAST workflows may install or use `BLAST+` beside the executable
- exported files are written inside `output/`
- persistent caches are written inside `.cache/`

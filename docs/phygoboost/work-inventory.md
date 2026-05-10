# Program Work Inventory

This file is an exhaustive non-test function inventory generated from the current repository state, reclassified using the latest user rules.

This file is the indexed archive companion to:

- [Work Classification Standard](./work-classification.md)

Use the standard to decide the class.

Use this file to record the result.

Maintenance rule:

- whenever a new work unit is introduced, removed, renamed, moved, split, merged, or reclassified, this file must be updated in the same change
- if the case does not fit the standard cleanly, update `work-classification.md` first and then update this inventory
- a new work unit is not considered fully integrated until it appears here

Hard rules used in this pass:

- `0` only means direct local synchronous helper logic. Any multithreaded / background / worker / batched function is excluded from `0`.
- Search, lookup, species loading, remote fetch, and similar pipeline steps are treated as class `2` work, not class `1`.
- Threads that communicate a lot or belong to the same tightly-coupled pipeline are grouped into one process; bias is toward the heavy process.
- UI-heavy loading, table rendering, progress overlays, selection UI, and strong UI-adjacent orchestration stay in class `1`.
- `1A`: main-process local/UI-heavy work.
- `1B`: main-process network work that is not better grouped into the heavy/search side.
- `2A`: heavy-process local work, subprocess work, worker-bound orchestration, or tightly-coupled non-UI multithreaded work.
- `2B`: heavy-process network/search/reference/source work.

## Totals

- `0`: 1059
- `1A`: 451
- `1B`: 1
- `2A`: 163
- `2B`: 262

## `cmd/phytozome-go/main.go`

- `0` [main](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/phytozome-go/main.go:24): entrypoint wrapper logic
- `0` [rootCommand](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/phytozome-go/main.go:53): entrypoint wrapper logic
- `0` [configureRuntime](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/phytozome-go/main.go:89): entrypoint wrapper logic
- `0` [printBlastPlan](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/phytozome-go/main.go:109): entrypoint wrapper logic
- `0` [runInteractiveWizard](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/phytozome-go/main.go:119): entrypoint wrapper logic
- `0` [printHeader](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/phytozome-go/main.go:128): entrypoint wrapper logic
- `0` [printVersion](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/phytozome-go/main.go:134): entrypoint wrapper logic
- `0` [workflowTUIInfo](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/phytozome-go/main.go:138): entrypoint wrapper logic

## `cmd/phytozome-go-winlauncher/main_windows.go`

- `1A` [main](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/phytozome-go-winlauncher/main_windows.go:19): entry/bootstrap orchestration for runtime or UI
- `0` [prepareWezTermRuntime](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/phytozome-go-winlauncher/main_windows.go:68): entrypoint wrapper logic
- `0` [copyDir](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/phytozome-go-winlauncher/main_windows.go:112): entrypoint wrapper logic
- `0` [copyFileIfNeeded](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/phytozome-go-winlauncher/main_windows.go:136): entrypoint wrapper logic
- `0` [requireFile](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/phytozome-go-winlauncher/main_windows.go:161): entrypoint wrapper logic
- `0` [showError](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/phytozome-go-winlauncher/main_windows.go:172): entrypoint wrapper logic

## `cmd/report-sample/main.go`

- `0` [main](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/report-sample/main.go:12): entrypoint wrapper logic
- `0` [scenarioKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/report-sample/main.go:62): entrypoint wrapper logic
- `0` [sampleReportBaseName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/cmd/report-sample/main.go:70): entrypoint wrapper logic

## `internal/appfs/appfs.go`

- `1A` [ApplicationDir](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/appfs/appfs.go:12): filesystem path preparation for the running app
- `1A` [OutputDir](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/appfs/appfs.go:28): filesystem path preparation for the running app
- `1A` [CacheDir](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/appfs/appfs.go:40): filesystem path preparation for the running app

## `internal/appfs/appfs_other.go`

- `1A` [markHiddenIfSupported](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/appfs/appfs_other.go:5): filesystem path preparation for the running app

## `internal/appfs/appfs_windows.go`

- `1A` [markHiddenIfSupported](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/appfs/appfs_windows.go:5): filesystem path preparation for the running app

## `internal/blastplus/install.go`

- `0` [Error](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:44): small BLAST+ helper
- `0` [IsMissingToolsError](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:51): small BLAST+ helper
- `0` [AsMissingToolsError](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:56): small BLAST+ helper
- `2A` [ToolsDir](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:60): tool install, extraction, and subprocess preparation are heavy local work
- `0` [EnsureToolsOnPath](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:74): small BLAST+ helper
- `2A` [InstallManaged](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:115): tool install, extraction, and subprocess preparation are heavy local work
- `0` [AddToolsDirToPath](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:119): small BLAST+ helper
- `2A` [InstallManagedWithProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:127): tool install, extraction, and subprocess preparation are heavy local work
- `0` [resetManagedInstallTarget](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:200): small BLAST+ helper
- `2B` [downloadCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:217): tool download/discovery is remote setup work and stays with the heavy side
- `2B` [discoverArchiveNames](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:233): tool download/discovery is remote setup work and stays with the heavy side
- `0` [archiveNameForPlatform](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:262): small BLAST+ helper
- `0` [archiveSuffixForPlatform](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:270): small BLAST+ helper
- `0` [archiveLinks](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:291): small BLAST+ helper
- `2B` [fetchText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:308): tool download/discovery is remote setup work and stays with the heavy side
- `2B` [downloadAndExtractTarGz](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:316): tool download/discovery is remote setup work and stays with the heavy side
- `0` [uniquePartPath](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:345): small BLAST+ helper
- `0` [removeStalePartFiles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:349): small BLAST+ helper
- `0` [removeStalePartFilesLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:356): small BLAST+ helper
- `0` [renameWithRetry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:372): small BLAST+ helper
- `2B` [fetchGrabText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:397): tool download/discovery is remote setup work and stays with the heavy side
- `2B` [downloadGrabFile](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:425): tool download/discovery is remote setup work and stays with the heavy side
- `2A` [extractTarGzArchive](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:479): tool install, extraction, and subprocess preparation are heavy local work
- `2A` [extractTarGzArchiveLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:473): tool install, extraction, and subprocess preparation are heavy local work
- `0` [Write](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:556): small BLAST+ helper
- `0` [tarGzUncompressedSize](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:565): small BLAST+ helper
- `2A` [tarGzUncompressedSizeLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:575): tool install, extraction, and subprocess preparation are heavy local work
- `0` [stagePercent](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:606): small BLAST+ helper
- `0` [formatBytes](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:627): small BLAST+ helper
- `0` [reportProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:642): small BLAST+ helper
- `2A` [findManagedBinDir](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:649): tool install, extraction, and subprocess preparation are heavy local work
- `0` [hasTool](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:684): small BLAST+ helper
- `0` [prependPath](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:689): small BLAST+ helper
- `0` [executableName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:703): small BLAST+ helper
- `2A` [applicationDir](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/blastplus/install.go:710): tool install, extraction, and subprocess preparation are heavy local work

## `internal/cachex/cachex.go`

- `1A` [Open](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:46): local cache and disk-budget work in main process
- `1A` [MustOpen](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:83): local cache and disk-budget work in main process
- `1A` [ReadJSON](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:91): local cache and disk-budget work in main process
- `1A` [WriteJSON](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:99): local cache and disk-budget work in main process
- `1A` [ReadMsgpack](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:107): local cache and disk-budget work in main process
- `1A` [WriteMsgpack](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:115): local cache and disk-budget work in main process
- `1A` [ReadText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:123): local cache and disk-budget work in main process
- `1A` [WriteText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:131): local cache and disk-budget work in main process
- `1A` [ReadBytes](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:135): local cache and disk-budget work in main process
- `1A` [WriteBytes](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:173): local cache and disk-budget work in main process
- `1A` [Delete](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:199): local cache and disk-budget work in main process
- `1A` [TrimMemory](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:223): local cache and disk-budget work in main process
- `1A` [Close](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:238): local cache and disk-budget work in main process
- `1A` [CloseAll](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:260): local cache and disk-budget work in main process
- `1A` [setMemory](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:278): local cache and disk-budget work in main process
- `1A` [recordIndex](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:293): local cache and disk-budget work in main process
- `1A` [filePath](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:318): local cache and disk-budget work in main process
- `1A` [filePathLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:328): local cache and disk-budget work in main process
- `1A` [writeAtomically](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:344): local cache and disk-budget work in main process
- `1A` [lockShared](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:356): local cache and disk-budget work in main process
- `1A` [lockExclusive](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:370): local cache and disk-budget work in main process
- `1A` [newMemoryCache](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:384): local cache and disk-budget work in main process
- `1A` [memoryKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:405): local cache and disk-budget work in main process
- `1A` [hashKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:409): local cache and disk-budget work in main process
- `1A` [bitsReverse64](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:414): local cache and disk-budget work in main process
- `1A` [cleanPart](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:423): local cache and disk-budget work in main process
- `1A` [IsUnavailable](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/cachex/cachex.go:435): local cache and disk-budget work in main process

## `internal/export/excel.go`

- `0` [WriteBlastResultsExcel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:31): tiny export helper
- `0` [blastAlignQueryLengthPercent](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:35): tiny export helper
- `0` [firstNonEmptyText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:45): tiny export helper
- `0` [blastRowsHaveUniProtReference](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:55): tiny export helper
- `0` [blastRowsHaveInterProReference](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:64): tiny export helper
- `0` [blastHeadersForRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:73): tiny export helper
- `0` [blastHeaderPlanForRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:85): tiny export helper
- `0` [blastTargetUniProtCanonicalLengthHeader](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:97): tiny export helper
- `0` [databaseDisplayNameForRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:101): tiny export helper
- `0` [blastRowValues](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:117): tiny export helper
- `0` [blastExportValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:127): tiny export helper
- `0` [blankIfNoUniProt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:276): tiny export helper
- `0` [blankIfNoInterPro](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:283): tiny export helper
- `0` [WriteBlastResultsExcelWithMetadata](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:290): tiny export helper
- `0` [blastRowValuesForHeaderIDs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:353): tiny export helper
- `0` [writeBlastMetadataSheet](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:362): tiny export helper
- `0` [applyBlastRowCellStyles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:406): tiny export helper
- `0` [applyBlastHeaderStyle](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:425): tiny export helper
- `0` [blastExcelCellColor](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:433): tiny export helper
- `0` [blastExcelCellColorByID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:437): tiny export helper
- `0` [blastExcelHeaderColor](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:464): tiny export helper
- `0` [blastExcelColumnReference](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:477): tiny export helper
- `0` [blastExcelColumnIsUniProtReference](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:491): tiny export helper
- `0` [blastExcelColumnIsInterProReference](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:495): tiny export helper
- `0` [blastExcelColumnID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:499): tiny export helper
- `0` [normalizeBlastExcelHeader](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:510): tiny export helper
- `0` [blastExcelFontStyle](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:591): tiny export helper
- `0` [originalRowIndexForExcel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:607): tiny export helper
- `0` [WriteKeywordResultsExcel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:620): tiny export helper
- `2A` [saveExcelFile](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:679): export generation and file writing are heavy local artifact work
- `0` [keywordExportValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:688): tiny export helper
- `0` [keywordExtraHeaders](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:728): tiny export helper
- `0` [keywordLabelName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:749): tiny export helper
- `0` [keywordRowsHaveProteinID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:753): tiny export helper
- `0` [sourceDatabaseForBlastRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:762): tiny export helper
- `0` [sourceDatabaseForKeywordRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/excel.go:771): tiny export helper

## `internal/export/text.go`

- `0` [WriteProteinSequencesText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/text.go:13): tiny export helper
- `2A` [writeProteinSequencesTextLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/text.go:19): export generation and file writing are heavy local artifact work

## `internal/export/workers.go`

- `2A` [init](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/workers.go:35): export generation and file writing are heavy local artifact work
- `2A` [WriteBlastResultsExcelWithMetadataProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/workers.go:65): export generation and file writing are heavy local artifact work
- `2A` [WriteKeywordResultsExcelProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/workers.go:79): export generation and file writing are heavy local artifact work
- `2A` [WriteProteinSequencesTextProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/export/workers.go:88): export generation and file writing are heavy local artifact work

## `internal/interpro/interpro.go`

- `0` [NewClient](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/interpro/interpro.go:67): InterPro parsing helper
- `2B` [Lookup](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/interpro/interpro.go:77): external reference lookup/search should stay in class 2 network work
- `0` [fetchAllPages](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/interpro/interpro.go:125): InterPro parsing helper
- `2B` [fetchPage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/interpro/interpro.go:158): external reference lookup/search should stay in class 2 network work
- `0` [finalize](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/interpro/interpro.go:188): InterPro parsing helper
- `0` [matchRegionsText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/interpro/interpro.go:239): InterPro parsing helper
- `0` [toMatch](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/interpro/interpro.go:295): InterPro parsing helper
- `0` [firstProteinLength](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/interpro/interpro.go:338): InterPro parsing helper
- `0` [memberAccessions](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/interpro/interpro.go:347): InterPro parsing helper
- `0` [readDiskEntry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/interpro/interpro.go:365): InterPro parsing helper
- `0` [writeDiskEntry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/interpro/interpro.go:373): InterPro parsing helper
- `0` [normalizeAccession](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/interpro/interpro.go:377): InterPro parsing helper
- `0` [uniqueSorted](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/interpro/interpro.go:390): InterPro parsing helper
- `0` [uniquePreserveOrder](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/interpro/interpro.go:396): InterPro parsing helper

## `internal/lemna/cache.go`

- `0` [writeCachedJSON](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/cache.go:19): Lemna local helper without threading
- `2A` [writeAtomically](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/cache.go:23): threaded or tightly-coupled local cache/parsing work stays with the heavy Lemna side

## `internal/lemna/lemna.go`

- `2B` [DetectBlastCapabilities](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:186): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [cachedProteinTranscriptMaps](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:221): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [cachedFastaIndex](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:338): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [AvailableBlastPrograms](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:434): wrapper inherits remote capability-detection pipeline because cold calls may trigger Lemna network work
- `2A` [enrichServerBlastCapability](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:454): threaded or tightly-coupled local cache/parsing work stays with the heavy Lemna side
- `0` [blastFormURL](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:495): Lemna local helper without threading
- `0` [normalizeBlastProgramName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:510): Lemna local helper without threading
- `0` [findBlastDBID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:527): Lemna local helper without threading
- `0` [hasBlastDatasetOptions](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:543): Lemna local helper without threading
- `0` [parseBlastOptions](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:552): Lemna local helper without threading
- `0` [parseBlastDatasetOptions](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:573): Lemna local helper without threading
- `0` [blastOptionMatchesRoot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:582): Lemna local helper without threading
- `0` [blastFormHasDB](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:595): Lemna local helper without threading
- `0` [parseBlastFormDefaults](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:604): Lemna local helper without threading
- `0` [htmlAttr](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:630): Lemna local helper without threading
- `0` [selectedOptionValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:638): Lemna local helper without threading
- `0` [ensureFASTA](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:650): Lemna local helper without threading
- `0` [extractBlastJobID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:658): Lemna local helper without threading
- `0` [NewClient](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:692): Lemna local helper without threading
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:712): Lemna local helper without threading
- `2B` [keywordSearchEngine](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:716): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [FetchSpeciesCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:732): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [cloneReleaseMap](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:813): Lemna local helper without threading
- `0` [inspectRootDownloadDir](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:821): Lemna local helper without threading
- `0` [FilterSpeciesCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:868): Lemna local helper without threading
- `2B` [SubmitBlast](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:890): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [SubmitBlastServerOnly](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:952): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [submitBlastToServer](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:964): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [WaitForBlastResults](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1043): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2A` [RunLocalBlast](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1115): thin wrapper still belongs to heavy local BLAST orchestration and should remain inside phygoboost
- `2A` [loadBlastResultFromCache](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1084): threaded or tightly-coupled local cache/parsing work stays with the heavy Lemna side
- `0` [findBlastResultCacheFile](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1109): Lemna local helper without threading
- `0` [findDirectBlastResultCacheFile](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1141): Lemna local helper without threading
- `2A` [parseCachedBlastResultTSV](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1169): threaded or tightly-coupled local cache/parsing work stays with the heavy Lemna side
- `0` [tsvHeaderContains](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1286): Lemna local helper without threading
- `2B` [FetchGeneQuerySequence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1299): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [FetchUniProtAccessions](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1327): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [SearchKeywordRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1381): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [SearchKeywordRowsWide](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1385): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [SearchKeywordRowsBroad](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1389): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [SearchKeywordRowsEngine](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1393): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [SearchKeywordRowsWideEngine](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1397): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [SearchKeywordRowsBroadEngine](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1401): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [searchKeywordRowsWithProgram](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1405): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [keywordRowsCacheKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1483): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [legacyKeywordRowsCacheKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1496): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [selectKeywordProgram](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1503): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [cachedKeywordIndex](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1521): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [buildKeywordIndex](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1576): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [loadKeywordRowsForRelease](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1611): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [keywordIndexCacheKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1656): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [addKeywordIndexHit](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1666): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [addIndexedKeywordRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1678): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [findKeywordRowIndexForAHRD](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1707): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [mergeKeywordRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1721): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [keywordRowFromAHRD](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1749): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [enrichKeywordRowWithAHRD](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1767): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1784): Lemna local helper without threading
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1785): Lemna local helper without threading
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1789): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1797): Lemna local helper without threading
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1798): Lemna local helper without threading
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1801): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1805): Lemna local helper without threading
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1806): Lemna local helper without threading
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1809): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1813): Lemna local helper without threading
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1814): Lemna local helper without threading
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1817): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1826): Lemna local helper without threading
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1827): Lemna local helper without threading
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1830): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1838): Lemna local helper without threading
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1839): Lemna local helper without threading
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1842): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1850): Lemna local helper without threading
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1851): Lemna local helper without threading
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1854): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1858): Lemna local helper without threading
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1859): Lemna local helper without threading
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1862): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1899): Lemna local helper without threading
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1900): Lemna local helper without threading
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1903): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [searchKeywordIndexIdentifiers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1914): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [searchKeywordIndexAliases](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1932): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [searchKeywordIndexTerms](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:1956): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [finalizeKeywordRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2005): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [cloneKeywordRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2017): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [FetchProteinSequence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2031): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [listDownloadDirs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2087): Lemna local helper without threading
- `0` [populateReleaseFiles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2104): Lemna local helper without threading
- `0` [nucleotideFileScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2139): Lemna local helper without threading
- `0` [releaseForSpecies](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2160): Lemna local helper without threading
- `2B` [searchGFFRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2173): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [loadAHRDRecords](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2256): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2A` [parseAHRDOutput](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2332): threaded or tightly-coupled local cache/parsing work stays with the heavy Lemna side
- `0` [releaseForTargetID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2361): Lemna local helper without threading
- `0` [buildProteinTranscriptMap](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2387): Lemna local helper without threading
- `0` [findProteinSequenceInRelease](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2460): Lemna local helper without threading
- `2B` [cachedProteinReleaseSequences](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2477): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [loadProteinReleaseSequences](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2515): Lemna local helper without threading
- `2B` [fetchText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2569): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [openMaybeGzip](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2616): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [parseLinks](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2640): Lemna local helper without threading
- `0` [choosePreferredRelease](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2657): Lemna local helper without threading
- `0` [releaseScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2668): Lemna local helper without threading
- `0` [parseGFF3Line](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2688): Lemna local helper without threading
- `2B` [buildKeywordRowFromGFF](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2708): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [officialCloneByRootDir](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2755): Lemna local helper without threading
- `2B` [isSearchableFeatureType](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2764): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [keywordTerms](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2773): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [rowMatchesTerms](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2781): Lemna local helper without threading
- `0` [ahrdRecordMatchesTerms](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2788): Lemna local helper without threading
- `0` [textValuesMatchTerms](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2798): Lemna local helper without threading
- `2B` [addKeywordRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2811): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [ensureExtraColumns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2824): Lemna local helper without threading
- `2B` [keywordShortLabelFromGFF](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2831): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [keywordShortLabelFromAHRD](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2848): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [firstSymbolFromDelimited](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2863): Lemna local helper without threading
- `0` [firstSymbolFromText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2875): Lemna local helper without threading
- `0` [isLikelyShortLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2892): Lemna local helper without threading
- `0` [stripTranscriptSuffix](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2930): Lemna local helper without threading
- `0` [parseGFFAttributes](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2943): Lemna local helper without threading
- `0` [sequenceAliases](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2966): Lemna local helper without threading
- `0` [fastaHeaderMatches](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2970): Lemna local helper without threading
- `0` [looksLikeSpeciesDir](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2987): Lemna local helper without threading
- `0` [blastNDBID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:2992): Lemna local helper without threading
- `0` [formatSpeciesLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3017): Lemna local helper without threading
- `0` [commonName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3025): Lemna local helper without threading
- `0` [resolveURL](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3038): Lemna local helper without threading
- `0` [cleanText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3059): Lemna local helper without threading
- `2B` [normalizeSearchLoose](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3066): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [normalizeSearchTight](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3073): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [firstNonEmpty](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3077): Lemna local helper without threading
- `0` [normalizedIdentifierCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3086): Lemna local helper without threading
- `0` [uniqueNormalizedStrings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3115): Lemna local helper without threading
- `2B` [lookupNormalizedMapValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3133): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [lookupAHRDRecord](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3142): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [lemnaReportKeyword](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3151): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [looksLikeSpecificKeywordIdentifier](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3163): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [specificKeywordIdentifierVariants](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3184): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [riceLocusVariants](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3216): Lemna local helper without threading
- `0` [normalizeRiceLocusCandidate](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3224): Lemna local helper without threading
- `0` [osC4HLike](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3243): Lemna local helper without threading
- `0` [aliasesForNormalizedTerm](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3248): Lemna local helper without threading
- `0` [curatedRiceRefSeqAliasMap](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3257): Lemna local helper without threading
- `0` [curatedRiceAliasMap](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3266): Lemna local helper without threading
- `2B` [wideKeywordQuery](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3278): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [relaxedKeywordQueries](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3282): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [normalizeAliasKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3311): Lemna local helper without threading
- `0` [normalizeIdentifierKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3317): Lemna local helper without threading
- `2B` [normalizeKeywordTerms](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3321): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [rowMatchesAnyTerm](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3338): Lemna local helper without threading
- `2B` [keywordRowIdentifiers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3348): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [keywordRowSearchTokens](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3372): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [splitKeywordToken](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3384): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [keywordRowSearchValues](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3393): Lemna search, species load, and server BLAST coordination are class 2 network work
- `2B` [keywordRowSearchText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3430): Lemna search, species load, and server BLAST coordination are class 2 network work
- `0` [mergeDelimitedValues](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3434): Lemna local helper without threading
- `2B` [sortKeywordRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/lemna.go:3455): Lemna search, species load, and server BLAST coordination are class 2 network work

## `internal/lemna/localblast.go`

- `2A` [WithLocalBlastThreads](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:36): local BLAST threaded/subprocess/file work belongs in heavy local work
- `2A` [LocalBlastRun](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:62): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `2A` [NewLocalBlastRunner](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:84): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `2A` [warm local blast references](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:117): local BLAST reference prewarm is a tightly-coupled heavy-side warmup step for reusable batch state
- `2A` [Run](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:132): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `2A` [PrepareLocalBlast](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:184): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `2A` [prepareLocalBlastResources](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:199): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `0` [localBlastDatabase](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:256): local BLAST pure helper without threading
- `2A` [ensureCacheDir](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:274): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `0` [localBlastDBPrefix](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:278): local BLAST pure helper without threading
- `0` [newLocalBlastJobID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:282): local BLAST pure helper without threading
- `2A` [withLocalResourceLock](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:290): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `0` [uniqueTempPath](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:318): local BLAST pure helper without threading
- `0` [removeStaleTempFiles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:325): local BLAST pure helper without threading
- `0` [removeStaleTempFilesLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:331): local BLAST pure helper without threading
- `0` [fileExistsNonEmpty](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:348): local BLAST pure helper without threading
- `0` [fileExistsNonEmptyLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:353): local BLAST pure helper without threading
- `0` [regularFileExists](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:358): local BLAST pure helper without threading
- `0` [regularFileExistsLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:363): local BLAST pure helper without threading
- `0` [statLocalFile](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:368): local BLAST pure helper without threading
- `0` [moveTempIntoPlace](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:378): local BLAST pure helper without threading
- `0` [moveTempIntoPlaceLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:384): local BLAST pure helper without threading
- `2A` [writeReadySentinel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:412): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `2B` [downloadAndPrepareFasta](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:424): local-BLAST prerequisite download/fetch remains heavy-process network work
- `2B` [downloadAndPrepareFastaLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:454): local-BLAST prerequisite download/fetch remains heavy-process network work
- `2B` [downloadGrabFile](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:487): local-BLAST prerequisite download/fetch remains heavy-process network work
- `2B` [ensureDecompressed](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:514): local-BLAST prerequisite download/fetch remains heavy-process network work
- `2A` [decompressFastaLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:537): local BLAST threaded/subprocess/file work belongs in heavy local work
- `2A` [ensureBlastTools](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:579): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `2A` [ensureBlastDB](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:584): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `2A` [makeBlastDBArgs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:619): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `2A` [ensureBlastDBOnce](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:629): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `0` [existsBlastDBFiles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:648): local BLAST pure helper without threading
- `0` [existsBlastDBFilesLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:657): local BLAST pure helper without threading
- `0` [hasBlastDBCoreFiles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:661): local BLAST pure helper without threading
- `0` [hasBlastDBCoreFilesLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:670): local BLAST pure helper without threading
- `0` [blastDBReadyPath](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:701): local BLAST pure helper without threading
- `0` [removeBlastDBFiles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:705): local BLAST pure helper without threading
- `0` [normalizeProgram](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:716): local BLAST pure helper without threading
- `2A` [runBlastAndParse](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:741): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `2A` [writeBlastQueryFile](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:784): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `0` [localBlastQueryCachePath](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:810): local BLAST pure helper without threading
- `0` [normalizeLocalBlastQuerySequence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:819): local BLAST pure helper without threading
- `2A` [parseBlastTabularBuffer](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:837): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `2A` [parseBlastTabularReader](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:844): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `0` [localBlastThreads](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:937): local BLAST pure helper without threading
- `2A` [parseBlastTabular](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:952): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `2A` [parseBlastTabularLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:962): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `2A` [buildFastaIndex](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:993): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `2A` [buildFastaIndexLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:1003): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `0` [headerToken](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:1039): local BLAST pure helper without threading
- `2A` [saveBlastResultToCache](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:1054): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `2A` [saveBlastResultToCacheLocked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:1060): local BLAST orchestration, wrappers, subprocesses, and result files belong in heavy local work
- `0` [enrichBlastRowsWithAHRD](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:1141): local BLAST pure helper without threading
- `0` [enrichBlastRowsWithMappings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:1180): local BLAST pure helper without threading
- `0` [lemnaGeneReportURL](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:1326): local BLAST pure helper without threading
- `0` [uniprotAccessionFromAHRD](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:1335): local BLAST pure helper without threading
- `0` [looksLikeUniProtAccession](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:1349): local BLAST pure helper without threading
- `0` [sanitizeFileName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/lemna/localblast.go:1367): local BLAST pure helper without threading

## `internal/model/blast.go`

- `0` [DefaultInterProConservedRegionSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/model/blast.go:141): data model helper without threading, UI, search, or external work
- `0` [DefaultFamilyBlastSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/model/blast.go:257): data model helper without threading, UI, search, or external work
- `0` [DefaultBlastFilterSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/model/blast.go:277): data model helper without threading, UI, search, or external work

## `internal/model/species.go`

- `0` [DisplayLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/model/species.go:19): data model helper without threading, UI, search, or external work
- `0` [SearchText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/model/species.go:39): data model helper without threading, UI, search, or external work

## `internal/phygoboost/api.go`

- `0` [ClosePools](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/api.go:4): pure runtime helper without threading or external work

## `internal/phygoboost/core_parallel.go`

- `1A` [ParallelFor](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/core_parallel.go:29): main-process parallel runtime scheduling with explicit resource declaration
- `1A` [ParallelForWithWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/core_parallel.go:33): main-process parallel runtime scheduling with explicit worker override
- `1A` [ParallelForSpec](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/core_parallel.go:39): main-process parallel runtime scheduling through explicit `ParallelSpec`
- `0` [waitParallelTask](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/core_parallel.go:119): pooled-task wait helper
- `0` [specForKind](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/core_parallel.go:132): legacy compatibility helper mapping old kinds to new specs
- `0` [requestForSpec](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/core_parallel.go:147): resource-request projection helper for parallel specs
- `0` [kindForSpec](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/core_parallel.go:162): observation helper mapping explicit specs back to coarse runtime buckets
- `0` [closeParallelPools](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/core_parallel.go:174): runtime shutdown placeholder for pooled parallel resources

## `internal/phygoboost/pagesize_fallback.go`

- `0` [pageSize](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/pagesize_fallback.go:7): pure runtime helper without threading or external work

## `internal/phygoboost/pagesize_sysconf.go`

- `0` [pageSize](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/pagesize_sysconf.go:11): pure runtime helper without threading or external work

## `internal/phygoboost/budgets.go`

- `0` [Current](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:52): runtime budget snapshot helper without direct workflow semantics
- `0` [HTTPClient](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:118): shared runtime HTTP client accessor
- `0` [DynamicWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:122): runtime worker-budget helper
- `0` [CPUWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:136): runtime worker-budget helper
- `0` [DiskWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:145): runtime worker-budget helper
- `0` [NetworkWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:149): runtime worker-budget helper
- `0` [NetworkRequestWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:153): runtime worker-budget helper
- `0` [NetworkProcessWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:161): runtime worker-budget helper
- `0` [BackgroundPrefetchWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:169): runtime worker-budget helper
- `0` [ProcessWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:195): runtime worker-budget helper
- `0` [ProcessSlotWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:199): runtime worker-budget helper
- `0` [KindWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:213): runtime worker-budget helper
- `0` [MemoryCacheBudgetBytes](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:228): runtime cache-budget helper
- `0` [Budgets](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:232): summarized phygoboost budget profile helper
- `0` [UIThrottle](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:243): UI timing helper
- `0` [UIAnimationTick](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:247): UI timing helper
- `0` [SearchDebounce](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:251): UI timing helper
- `0` [clampWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:255): runtime worker-bound clamp helper
- `0` [uiReservedCPU](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:265): runtime UI CPU reservation helper
- `0` [childWorkerNetworkWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:280): heavy-child network projection helper
- `0` [childWorkerDiskWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:284): heavy-child disk projection helper
- `0` [childWorkerProcessWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:288): heavy-child process projection helper
- `0` [childWorkerBudget](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:292): heavy-child shared-budget slicing helper
- `0` [externalNativeThreads](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:304): subprocess native-thread projection helper
- `0` [adaptiveWorkerBudget](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:309): runtime adaptive worker-budget calculator
- `0` [feedbackBudgetScale](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:338): observation-feedback scale helper for worker budgets
- `0` [adaptiveNetworkRequestBudget](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:386): adaptive main-request network budget helper
- `0` [adaptiveNetworkProcessBudget](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:408): adaptive heavy-process network budget helper
- `0` [networkExternalScale](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:427): external-pressure network scaling helper
- `0` [ewmaDuration](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:449): duration EWMA helper
- `0` [ewmaFloat](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:457): float EWMA helper
- `0` [runtimeScale](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:467): runtime pressure scaling helper
- `0` [processContentionScale](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:480): process-contention scale helper
- `0` [contentionScale](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:488): worker-contention scale helper
- `0` [childScopeScale](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:496): heavy-child scope scale helper
- `0` [pressureScale](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:504): pressure-level scale helper
- `0` [cpuIdleRatio](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:517): runtime CPU-idle ratio helper
- `0` [memoryFreeRatio](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:524): runtime memory-free ratio helper
- `0` [memoryUsedPercent](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:531): runtime memory-use percent helper
- `0` [uiReserveRatio](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:538): UI reservation ratio helper
- `0` [adaptiveHTTPIdleConnections](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:550): shared HTTP idle-connection budget helper
- `0` [adaptiveHTTPIdlePerHost](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:554): shared HTTP per-host idle budget helper
- `0` [adaptiveHTTPRateLimit](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:558): shared HTTP rate-limit helper
- `0` [adaptiveMemoryCacheBudget](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:562): adaptive memory-cache budget helper
- `0` [positiveCeil](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:572): bounded positive ceil helper
- `0` [clampRatio](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:582): ratio clamp helper
- `0` [capByMaxWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:592): configured worker-cap helper
- `0` [minInt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:599): integer helper
- `0` [maxInt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:606): integer helper
- `0` [minPositive](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:613): positive minimum helper
- `0` [maxUint64](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/budgets.go:626): unsigned integer helper

## `internal/phygoboost/observation.go`

- `1B` [RuntimeState](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:73): runtime observation entrypoint feeding scheduler and network policy
- `0` [RegisterCleaner](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:83): runtime memory-pressure cleanup registration helper
- `0` [ObserveCacheBytes](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:93): runtime cache observation helper
- `0` [Pressure](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:101): runtime pressure snapshot accessor
- `0` [ChildProcesses](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:108): runtime child-process snapshot accessor
- `0` [systemCPUPercentSnapshot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:115): runtime CPU snapshot accessor
- `0` [snapshot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:122): runtime observation snapshot helper
- `0` [feedback](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:137): runtime feedback accessor by coarse work bucket
- `0` [feedbackSnapshot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:143): runtime feedback snapshot helper
- `0` [sampleChildProcessCount](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:165): runtime process-count observation helper
- `0` [AdjustWorkers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:184): observation-aware worker-budget adjustment helper
- `0` [loop](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:197): background runtime sampling loop
- `0` [sampleIfStale](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:206): staleness-checked sampler helper
- `0` [sample](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:215): runtime observation sampler
- `0` [maybeTrim](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:234): memory-pressure cleanup trigger
- `0` [classifyPressure](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:254): runtime pressure classifier
- `0` [WorkerStarted](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:276): runtime active-worker observation helper
- `0` [ObserveWork](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:284): runtime throughput and failure observation helper
- `0` [isPerformanceFailure](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:307): performance-failure filter helper
- `1A` [ObserveTaskSpec](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:330): main-process observation wrapper for TaskSpec-based work
- `0` [DrainAndClose](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/observation.go:343): HTTP body cleanup helper

## `internal/phygoboost/env.go`

- `0` [WorkerEnv](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/env.go:17): heavy worker environment projection helper
- `0` [workerGOMAXPROCS](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/env.go:45): heavy worker GOMAXPROCS helper
- `0` [workerMemoryLimitBytes](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/env.go:65): heavy worker memory-limit helper
- `0` [pressureName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/env.go:85): pressure naming helper
- `0` [configuredInt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/env.go:98): integer environment helper
- `0` [configuredInt64](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/env.go:110): int64 environment helper
- `0` [configuredDurationMS](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/env.go:122): millisecond duration environment helper
- `0` [configuredDurationSeconds](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/env.go:130): second duration environment helper
- `0` [configuredBool](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/env.go:138): boolean environment helper
- `0` [workerProcessMode](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/env.go:148): heavy worker-mode detector
- `0` [activeCPUCount](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/env.go:152): active CPU-count helper
- `0` [sampleSystemMemory](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/env.go:165): runtime system-memory sampler
- `0` [sampleSystemCPUPercent](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/env.go:181): runtime system-CPU sampler
- `0` [memoryLimits](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/env.go:195): runtime memory-limit helper
- `0` [defaultMemorySoftLimit](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/env.go:210): default soft-memory-limit helper

## `internal/phygoboost/network_context.go`

- `0` [contextWithNetworkGrants](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/network_context.go:10): runtime context helper for declared network grants
- `0` [contextHasNetworkGrant](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/network_context.go:37): runtime context helper for skipping duplicate domain acquisition
- `0` [contextLocalGrant](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/network_context.go:50): runtime context helper for reusing already-acquired local grants
- `0` [contextWithLocalGrant](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/network_context.go:61): runtime context helper for binding declared local grants
- `0` [networkGrantSnapshotFromContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/network_context.go:68): IPC-facing active-network snapshot helper
- `0` [contextWithNetworkGrantSnapshot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/network_context.go:90): heavy-side network-grant rebinding helper

## `internal/phygoboost/process.go`

- `2A` [RunProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/process.go:24): subprocess execution routed through explicit `TaskSpec` and runtime resource control
- `0` [FormatCapturedOutput](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/process.go:72): captured stderr/stdout formatting helper

## `internal/phygoboost/http_client.go`

- `1B` [newSharedHTTPClient](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/http_client.go:13): shared main-process HTTP transport construction for all production network traffic
- `0` [minDurationPositive](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/http_client.go:42): duration fallback helper
- `1B` [RoundTrip](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/http_client.go:51): domain-aware request execution with per-domain acquire/release semantics
- `1B` [Close](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/http_client.go:98): response-body close hook that releases domain reservations with observed latency and status
- `0` [contextForRequest](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/http_client.go:108): HTTP request context helper
- `0` [statusError](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/http_client.go:118): HTTP response error helper
- `0` [statusErrorCode](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/http_client.go:128): HTTP status-to-error helper

## `internal/phygoboost/procworker.go`

- `2A` [Register](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/procworker.go:31): runtime piece tied to heavy/process/worker coordination
- `2A` [Registered](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/procworker.go:23): runtime piece tied to heavy/process/worker coordination
- `2A` [InWorker](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/procworker.go:31): runtime piece tied to heavy/process/worker coordination
- `2A` [RunIfWorker](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/procworker.go:35): runtime piece tied to heavy/process/worker coordination
- `2A` [RunTaskJSON](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/procworker.go:42): heavy-worker JSON task dispatch entrypoint
- `2A` [RunTask](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/procworker.go:65): heavy-worker byte-payload dispatch entrypoint with nested resource reuse

## `internal/phygoboost/profile.go`

- `0` [StartDiagnostics](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/profile.go:21): pure runtime helper without threading or external work
- `1A` [startPProfServer](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/profile.go:36): main-process runtime scheduling, permit, observation, or disk helper
- `0` [startLocalProfile](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/profile.go:73): pure runtime helper without threading or external work
- `0` [enabledEnv](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/profile.go:99): pure runtime helper without threading or external work
- `0` [LogRuntimeSummary](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/profile.go:109): pure runtime helper without threading or external work

## `internal/phygoboost/runtime_state.go`

- `0` [coordinator](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime_state.go:24): singleton runtime-coordinator accessor
- `1A` [local](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime_state.go:31): main-process local-slot scheduler construction and access
- `1B` [httpClient](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime_state.go:48): shared production HTTP client construction and access
- `1B` [network](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime_state.go:55): per-domain network manager construction and access
- `1A` [AcquireLocal](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime_state.go:72): explicit main/heavy local-slot acquisition through the shared scheduler
- `1A` [ReleaseLocal](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime_state.go:85): shared local-slot release helper
- `1B` [AcquireNetwork](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime_state.go:97): per-domain network slot acquisition through the shared manager
- `1B` [HTTPClientForDomain](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime_state.go:112): shared HTTP client accessor for domain-bound runtime work
- `1A` [DeclareResources](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime_state.go:117): explicit runtime resource declaration for local and network grants
- `1A` [BindDeclaredResources](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime_state.go:136): context binding helper for declared network grants
- `1A` [Release](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime_state.go:143): unified release helper for declared runtime resources

## `internal/phygoboost/runtime.go`

- `0` [TimeoutContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:14): pure runtime helper without threading or external work
- `0` [RunWithTimeout](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:24): pure runtime helper without threading or external work
- `0` [RunWithTimeoutValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:34): timeout-bound value helper
- `1A` [RunTaskSpec](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:44): main-process explicit task runner with resource declaration and observation
- `1A` [RunTaskSpecValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:60): main-process explicit task runner returning a value
- `1A` [RunDisk](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:81): heavy-local disk task convenience wrapper
- `1A` [RunDiskValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:85): heavy-local disk task convenience wrapper returning a value
- `1A` [GoTask](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:104): explicit async task launch in main process
- `0` [StartAsyncResult](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:120): async result channel helper
- `0` [MergeContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:137): merged-context helper
- `0` [SleepContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:156): context-aware sleep helper
- `1A` [BindTaskSpec](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:173): task-spec binding helper that only acquires missing local and per-domain resources
- `0` [workKindForTaskSpec](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:188): observation helper mapping explicit task specs to coarse runtime buckets
- `0` [resourceRequestForTaskSpec](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:200): explicit task-to-resource projection helper
- `0` [cloneNetworkBudget](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:223): network-budget clone and normalization helper
- `0` [missingResourceRequestForTaskSpec](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:241): nested-resource reuse helper that subtracts already-held local and network grants
- `0` [resourceRequestIsEmpty](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/runtime.go:263): empty-resource-request helper

## `internal/phygoboost/worker_parallel.go`

- `2A` [RegisterIndexedJSON](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/worker_parallel.go:18): indexed heavy-worker registration helper for batch JSON tasks
- `2A` [ParallelTaskJSON](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/worker_parallel.go:32): heavy-worker parallel JSON task dispatcher
- `0` [runningTestBinary](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/worker_parallel.go:83): test-binary detection helper

## `internal/phygoboost/heavy_host_runtime.go`

- `2A` [heavyCoordinator](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy_host_runtime.go:33): heavy-process client singleton accessor
- `0` [heavyMode](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy_host_runtime.go:40): heavy-process mode detector
- `2A` [runHeavyWorkerLoop](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy_host_runtime.go:44): heavy-process worker-host loop bootstrap
- `2A` [dispatchHeavyTask](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy_host_runtime.go:56): main-to-heavy task dispatch with IPC grant propagation
- `2A` [ensureStarted](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy_host_runtime.go:84): heavy-process host startup and subprocess bootstrap
- `2A` [shutdown](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy_host_runtime.go:123): heavy host shutdown and waiter cleanup
- `2A` [closeHeavyHost](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy_host_runtime.go:159): global heavy-host shutdown helper
- `2A` [send](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy_host_runtime.go:166): IPC send helper for the heavy client
- `2A` [registerWaiter](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy_host_runtime.go:176): task waiter registration helper
- `2A` [unregisterWaiter](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy_host_runtime.go:188): task waiter removal helper
- `2A` [closedCh](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy_host_runtime.go:194): heavy-host closed-channel accessor
- `2A` [receiveLoop](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy_host_runtime.go:204): IPC receive loop for heavy task results
- `2A` [waitLoop](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy_host_runtime.go:227): heavy subprocess wait loop
- `2A` [finish](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy_host_runtime.go:237): heavy client terminal cleanup helper

## `internal/phygoboost/heavy/host.go`

- `2A` [New](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy/host.go:21): heavy worker-host construction
- `2A` [Serve](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy/host.go:32): heavy worker-host task loop, cancellation handling, and IPC replies
- `2A` [register](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy/host.go:77): running-task registration helper
- `2A` [unregister](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy/host.go:86): running-task removal helper
- `2A` [cancel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy/host.go:92): per-task cancellation helper
- `2A` [cancelAll](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/heavy/host.go:100): whole-host cancellation helper

## `internal/phygoboost/ipc/bus.go`

- `2A` [New](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/ipc/bus.go:27): IPC bus construction for main-heavy communication
- `2A` [Send](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/ipc/bus.go:34): IPC message send helper
- `2A` [Receive](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phygoboost/ipc/bus.go:49): IPC message receive helper

## `internal/phytozome/blast.go`

- `2B` [SubmitBlast](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:128): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [WaitForBlastResults](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:188): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [fetchBlastResult](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:219): species fetch, search, and BLAST-related remote work are all class 2 network work
- `0` [blastResultsPending](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:267): Phytozome parse/normalize helper without threading
- `0` [parseBlastRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:279): Phytozome parse/normalize helper without threading
- `2B` [parseHitDefinition](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:351): species fetch, search, and BLAST-related remote work are all class 2 network work
- `0` [percentIdentity](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:366): Phytozome parse/normalize helper without threading
- `0` [strandText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:373): Phytozome parse/normalize helper without threading
- `0` [frameDirection](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:377): Phytozome parse/normalize helper without threading
- `0` [normalizeWordLength](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:388): Phytozome parse/normalize helper without threading
- `0` [boolString](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:395): Phytozome parse/normalize helper without threading
- `0` [writeSequenceField](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:402): Phytozome parse/normalize helper without threading
- `2B` [FetchGeneByProtein](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:415): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [FetchUniProtAccessions](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:420): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [extractUniProtAccessionsFromGene](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:450): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [normalizeUniProtAccessionValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:478): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [FetchGeneByGeneID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:490): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [FetchGeneByTranscript](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:495): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [fetchGeneRecord](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:500): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [FetchProteinSequence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:568): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [FetchProteinQuerySequence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:586): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [fetchProteinSequenceByTranscript](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:615): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [FetchGeneQuerySequence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:686): species fetch, search, and BLAST-related remote work are all class 2 network work
- `0` [applyPhytozomeQueryLabels](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:744): Phytozome parse/normalize helper without threading
- `2B` [SearchGenesByKeyword](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:751): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [SearchGenesByKeywordBroad](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:828): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [phytozomeKeywordSearchBodies](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:877): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [keywordBroadMustClause](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:963): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [phytozomeKeywordHasPlusQualifier](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:970): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [phytozomeKeywordTerms](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:975): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [searchGenesWithBody](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:986): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [SearchKeywordRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1022): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [SearchKeywordRowsWide](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1026): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [SearchKeywordRowsBroad](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1030): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [buildKeywordResultRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1034): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [formatKeywordGenome](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1078): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [formatKeywordLocation](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1101): species fetch, search, and BLAST-related remote work are all class 2 network work
- `0` [looksLikeSpecificGeneIdentifier](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1109): Phytozome parse/normalize helper without threading
- `2B` [phytozomeGeneReportKeyword](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1113): species fetch, search, and BLAST-related remote work are all class 2 network work
- `0` [specificIdentifierVariants](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1117): Phytozome parse/normalize helper without threading
- `0` [nonEmptyPathSegments](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1121): Phytozome parse/normalize helper without threading
- `0` [dedupePreserveOrder](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1133): Phytozome parse/normalize helper without threading
- `0` [firstAlias](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1151): Phytozome parse/normalize helper without threading
- `0` [bestAlias](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1163): Phytozome parse/normalize helper without threading
- `0` [bestQuerySourceLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1183): Phytozome parse/normalize helper without threading
- `0` [querySourceAliasCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1211): Phytozome parse/normalize helper without threading
- `0` [querySourceLabelPreferenceBonus](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1232): Phytozome parse/normalize helper without threading
- `0` [looksLikePrimaryFamilySymbol](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1248): Phytozome parse/normalize helper without threading
- `0` [aliasPreferenceScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1267): Phytozome parse/normalize helper without threading
- `0` [noLowercase](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1331): Phytozome parse/normalize helper without threading
- `0` [aliasHasInternalDigitPattern](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1340): Phytozome parse/normalize helper without threading
- `0` [lowercaseCount](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1360): Phytozome parse/normalize helper without threading
- `0` [looksLikeECNumberLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1370): Phytozome parse/normalize helper without threading
- `0` [looksLikeDatabaseIdentifierLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1374): Phytozome parse/normalize helper without threading
- `0` [labelFromAutoDefine](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1390): Phytozome parse/normalize helper without threading
- `0` [autoDefineCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1403): Phytozome parse/normalize helper without threading
- `0` [autoDefineLabelScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1428): Phytozome parse/normalize helper without threading
- `0` [looksLikeAliasToken](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1453): Phytozome parse/normalize helper without threading
- `0` [copyStringSlice](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1471): Phytozome parse/normalize helper without threading
- `0` [firstNonEmpty](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/blast.go:1480): Phytozome parse/normalize helper without threading

## `internal/phytozome/cache.go`

- `0` [writeCachedJSON](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/cache.go:15): Phytozome parse/normalize helper without threading
- `0` [readCachedText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/cache.go:19): Phytozome parse/normalize helper without threading
- `0` [writeCachedText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/cache.go:23): Phytozome parse/normalize helper without threading

## `internal/phytozome/species.go`

- `0` [NewClient](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:52): Phytozome parse/normalize helper without threading
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:65): Phytozome parse/normalize helper without threading
- `2B` [keywordSearchEngine](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:69): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [FetchSpeciesCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:85): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [load phytozome species metadata](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:128): species metadata fan-out is heavy-side remote source loading work
- `2B` [fetchReleaseDates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:153): species fetch, search, and BLAST-related remote work are all class 2 network work
- `0` [FilterSpeciesCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:193): Phytozome parse/normalize helper without threading
- `0` [speciesMatchScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:229): Phytozome parse/normalize helper without threading
- `2B` [candidateSearchParts](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:270): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [normalizeSearchLoose](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:279): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [normalizeSearchTight](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:286): species fetch, search, and BLAST-related remote work are all class 2 network work
- `0` [parseSpeciesCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:290): Phytozome parse/normalize helper without threading
- `0` [parseReleaseDates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:294): Phytozome parse/normalize helper without threading
- `0` [candidatesFromTargets](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:313): Phytozome parse/normalize helper without threading
- `0` [cleanText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:354): Phytozome parse/normalize helper without threading
- `0` [fetchTargetRecords](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:374): Phytozome parse/normalize helper without threading
- `2A` [fetchTargetRecordsFromBundles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:388): threaded source-side coordination should stay with the heavy side
- `2B` [scan phytozome homepage bundles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:408): homepage bundle scan is heavy-side remote source loading work
- `0` [recordBundleFailure](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:432): Phytozome parse/normalize helper without threading
- `2B` [fetchHomePage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:441): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [fetchBundle](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:464): species fetch, search, and BLAST-related remote work are all class 2 network work
- `2B` [extractBundleScriptURLs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:487): species fetch, search, and BLAST-related remote work are all class 2 network work
- `0` [compareBundlePriority](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:520): Phytozome parse/normalize helper without threading
- `0` [extractTargetRecords](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:533): Phytozome parse/normalize helper without threading
- `0` [decodeTargetRecords](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:547): Phytozome parse/normalize helper without threading
- `0` [extractJSONObjectCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:562): Phytozome parse/normalize helper without threading
- `0` [findMatchingJSONObjectEnd](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:582): Phytozome parse/normalize helper without threading
- `0` [ParseProteomeID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/phytozome/species.go:620): Phytozome parse/normalize helper without threading

## `internal/prompt/column_registry.go`

- `0` [ColumnHelpText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:397): prompt construction and local validation only
- `0` [ColumnCanonicalID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:411): prompt construction and local validation only
- `0` [ColumnCompactHeader](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:427): prompt construction and local validation only
- `0` [ColumnDetailLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:431): prompt construction and local validation only
- `0` [ColumnExportHeader](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:435): prompt construction and local validation only
- `0` [ColumnHelpChinese](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:439): prompt construction and local validation only
- `0` [ColumnHelpJapanese](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:443): prompt construction and local validation only
- `0` [KnownColumnHelpIDs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:447): prompt construction and local validation only
- `0` [KeywordDisplayColumnIDs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:508): prompt construction and local validation only
- `0` [KeywordDetailColumnIDs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:512): prompt construction and local validation only
- `0` [KeywordExportColumnIDs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:516): prompt construction and local validation only
- `0` [KeywordReportColumnIDs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:524): prompt construction and local validation only
- `0` [BlastDisplayColumnIDs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:539): prompt construction and local validation only
- `0` [BlastDetailColumnIDs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:544): prompt construction and local validation only
- `0` [BlastExportColumnIDs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:549): prompt construction and local validation only
- `0` [BlastReportColumnIDs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:570): prompt construction and local validation only
- `0` [normalizeColumnHelpID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:587): prompt construction and local validation only
- `0` [columnLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:598): prompt construction and local validation only
- `0` [wrapColumnLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:636): prompt construction and local validation only
- `0` [normalizedDatabaseKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:652): prompt construction and local validation only
- `0` [copyColumnIDs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:660): prompt construction and local validation only
- `0` [filteredColumnIDs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:670): prompt construction and local validation only
- `0` [mergeUniqueColumnIDs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:680): prompt construction and local validation only
- `0` [blastBaseColumnIDs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:699): prompt construction and local validation only
- `0` [appendBlastReferenceColumns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:718): prompt construction and local validation only
- `0` [blastColumnIsUniProtLike](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:741): prompt construction and local validation only
- `0` [blastColumnIsInterProLike](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:745): prompt construction and local validation only
- `0` [dynamicColumnHelpText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:749): prompt construction and local validation only
- `0` [extractColumnHelpSection](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:790): prompt construction and local validation only
- `0` [humanizeColumnSuffix](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/column_registry.go:808): prompt construction and local validation only

## `internal/prompt/prompt.go`

- `0` [columnHelp](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:39): prompt construction and local validation only
- `1A` [ColumnHelpEnglish](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:43): prompt flow with UI/task orchestration
- `1A` [blastTableHeader](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:477): prompt flow with UI/task orchestration
- `1A` [blastAlignQueryLengthPercent](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:481): prompt flow with UI/task orchestration
- `0` [New](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:599): prompt construction and local validation only
- `0` [SetDatabaseContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:612): prompt construction and local validation only
- `0` [SetBlastProgramContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:621): prompt construction and local validation only
- `0` [t](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:625): prompt construction and local validation only
- `0` [tf](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:629): prompt construction and local validation only
- `0` [tuiNavError](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:633): prompt construction and local validation only
- `0` [tuiPath](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:651): prompt construction and local validation only
- `0` [blastTUIPath](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:664): prompt construction and local validation only
- `0` [firstNonEmptyText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:673): prompt construction and local validation only
- `1A` [ChooseDatabase](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:683): prompt flow with UI/task orchestration
- `1A` [ChooseBlastTargetDatabase](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:708): prompt flow with UI/task orchestration
- `1A` [ChooseMode](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:733): prompt flow with UI/task orchestration
- `1A` [ChooseBlastProgram](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:762): prompt flow with UI/task orchestration
- `0` [blastProgramDescription](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:860): prompt construction and local validation only
- `1A` [ChooseBlastExecution](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:877): prompt flow with UI/task orchestration
- `1A` [SpeciesKeyword](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:904): prompt flow with UI/task orchestration
- `1A` [SelectSpecies](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:926): prompt flow with UI/task orchestration
- `1A` [KeywordLabelNames](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:959): prompt flow with UI/task orchestration
- `1A` [BlastLabelNames](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1005): prompt flow with UI/task orchestration
- `1A` [OutputFolderName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1055): prompt flow with UI/task orchestration
- `1A` [DetailedReportAction](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1077): prompt flow with UI/task orchestration
- `1A` [ExternalReferenceSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1105): prompt flow with UI/task orchestration
- `1A` [FamilyBlastSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1145): prompt flow with UI/task orchestration
- `0` [familyBlastReferenceMessage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1294): prompt construction and local validation only
- `0` [tuiFamilyBlastGroups](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1334): prompt construction and local validation only
- `0` [tuiFamilyBlastCustomGroups](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1347): prompt construction and local validation only
- `0` [tuiFamilyBlastMembers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1359): prompt construction and local validation only
- `0` [promptFamilyBlastGroupsFromTUI](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1373): prompt construction and local validation only
- `0` [promptFamilyBlastMembersFromTUI](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1401): prompt construction and local validation only
- `1A` [BlastFilterSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1415): prompt flow with UI/task orchestration
- `1A` [tuiBlastFilterSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1443): prompt flow with UI/task orchestration
- `1A` [parseBlastFilterSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1529): prompt flow with UI/task orchestration
- `0` [blastFilterSuggestion](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1622): prompt construction and local validation only
- `1A` [blastFilterSuggestionWithProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1629): prompt flow with UI/task orchestration
- `1A` [clearBlastFilterWithProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1662): prompt flow with UI/task orchestration
- `1A` [clearBlastRunFiltersWithProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1692): prompt flow with UI/task orchestration
- `0` [blastFilterTaskCancelled](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1736): prompt construction and local validation only
- `0` [defaultBlastFilterSuggestion](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1740): prompt construction and local validation only
- `0` [DefaultBlastFilterSuggestion](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1770): prompt construction and local validation only
- `0` [blastFilterSuggestRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1774): prompt construction and local validation only
- `1A` [normalizePromptSelection](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1794): prompt flow with UI/task orchestration
- `0` [evaluateBlastFilterRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1805): prompt construction and local validation only
- `0` [blastRowMissingReferenceAnchors](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1939): prompt construction and local validation only
- `0` [blastRowMeetsStrongFallback](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1943): prompt construction and local validation only
- `0` [blastRowMeetsStrongFallbackConsensus](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1963): prompt construction and local validation only
- `0` [blastTargetQueryLengthRatio](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1975): prompt construction and local validation only
- `0` [blastRowHasAnyInterProDomain](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:1982): prompt construction and local validation only
- `0` [applyTopHitLimit](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2000): prompt construction and local validation only
- `0` [applyBestIsoformLimit](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2022): prompt construction and local validation only
- `1A` [sortBlastFilterIndexes](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2051): prompt flow with UI/task orchestration
- `0` [blastFilterIndexLess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2057): prompt construction and local validation only
- `0` [blastFilterRankingOrder](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2098): prompt construction and local validation only
- `0` [parseBlastFilterRankingOrder](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2106): prompt construction and local validation only
- `1A` [blastFilterReferenceScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2142): prompt flow with UI/task orchestration
- `0` [blastRowHasSemanticAgreement](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2192): prompt construction and local validation only
- `0` [blastRowHasSemanticTokens](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2205): prompt construction and local validation only
- `0` [blastRowHasSemanticReferenceSurface](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2209): prompt construction and local validation only
- `0` [strongReferenceBypassScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2230): prompt construction and local validation only
- `0` [blastFilterTargetGeneKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2238): prompt construction and local validation only
- `0` [blastFilterQueryKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2259): prompt construction and local validation only
- `0` [blastRowQueryCoverage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2269): prompt construction and local validation only
- `0` [hasUniProtAnnotation](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2284): prompt construction and local validation only
- `0` [isTruthyAnnotation](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2293): prompt construction and local validation only
- `0` [parseFirstFloat](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2302): prompt construction and local validation only
- `0` [parseScientificFloat](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2306): prompt construction and local validation only
- `0` [coordinateSpanPrompt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2322): prompt construction and local validation only
- `1A` [normalizeBlastFilterSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2332): prompt flow with UI/task orchestration
- `0` [blastRowsHaveAllExternalReferences](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2481): prompt construction and local validation only
- `0` [blastRunsHaveAllExternalReferences](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2493): prompt construction and local validation only
- `0` [blastRunRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2507): prompt construction and local validation only
- `0` [parseInterProSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2515): prompt construction and local validation only
- `0` [parseFloatDefault](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2533): prompt construction and local validation only
- `0` [formatScientificSetting](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2541): prompt construction and local validation only
- `0` [parseFloatDefaultAllowZero](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2551): prompt construction and local validation only
- `0` [parseIntDefault](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2559): prompt construction and local validation only
- `1A` [SearchAndSelectSpecies](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2567): prompt flow with UI/task orchestration
- `1A` [SequenceInput](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2607): prompt flow with UI/task orchestration
- `0` [targetIDLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2628): prompt construction and local validation only
- `1A` [KeywordInput](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2640): prompt flow with UI/task orchestration
- `0` [phytozomeContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2671): prompt construction and local validation only
- `1A` [SelectKeywordRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2680): prompt flow with UI/task orchestration
- `1A` [SelectBlastRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2752): prompt flow with UI/task orchestration
- `1A` [SelectBlastRowsBatch](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2757): prompt flow with UI/task orchestration
- `1A` [SelectBlastRowsBatchWithBack](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2761): prompt flow with UI/task orchestration
- `1A` [SelectBlastRowsWithOptions](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2765): prompt flow with UI/task orchestration
- `1A` [SelectBlastRuns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2769): prompt flow with UI/task orchestration
- `0` [blastRunGeneLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2944): prompt construction and local validation only
- `0` [blastRunLabelName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2954): prompt construction and local validation only
- `0` [splitPromptDisplayLines](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2970): prompt construction and local validation only
- `0` [uniquePromptDisplayLines](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2982): prompt construction and local validation only
- `0` [blastRowsBackTarget](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:2999): prompt construction and local validation only
- `1A` [selectBlastRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3003): prompt flow with UI/task orchestration
- `0` [anyPromptFilterFlagsByRun](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3140): prompt construction and local validation only
- `0` [anyPromptBool](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3149): prompt construction and local validation only
- `1A` [buildKeywordSelectionTable](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3158): prompt flow with UI/task orchestration
- `1A` [buildBlastSelectionTable](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3181): prompt flow with UI/task orchestration
- `1A` [tableStateKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3204): prompt flow with UI/task orchestration
- `0` [digestStrings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3216): prompt construction and local validation only
- `0` [cloneBoolMatrixPrompt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3225): prompt construction and local validation only
- `0` [setAllPrompt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3233): prompt construction and local validation only
- `1A` [selectedByRunFromItems](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3239): prompt flow with UI/task orchestration
- `0` [filterFlagsByRunFromItems](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3247): prompt construction and local validation only
- `1A` [blastDisplayColumns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3255): prompt flow with UI/task orchestration
- `0` [blastDisplayColumnIDsForRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3515): prompt construction and local validation only
- `0` [databaseDisplayNameForRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3521): prompt construction and local validation only
- `0` [blastDetailColumnIDsForRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3537): prompt construction and local validation only
- `0` [blastRowsHaveUniProtReference](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3541): prompt construction and local validation only
- `0` [blastRowHasUniProtData](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3550): prompt construction and local validation only
- `0` [blastColumnIsUniProtReference](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3554): prompt construction and local validation only
- `0` [blastRowsHaveInterProReference](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3558): prompt construction and local validation only
- `0` [blastRowHasInterProData](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3567): prompt construction and local validation only
- `0` [blastColumnIsInterProReference](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3573): prompt construction and local validation only
- `0` [blastRowsFromLemna](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3577): prompt construction and local validation only
- `0` [blastRowsProgram](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3581): prompt construction and local validation only
- `0` [blastRowsHaveLabelName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3590): prompt construction and local validation only
- `1A` [keywordDisplayColumns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3599): prompt flow with UI/task orchestration
- `1A` [phytozomeKeywordDisplayColumns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3675): prompt flow with UI/task orchestration
- `0` [keywordDisplayColumnIDsForRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3679): prompt construction and local validation only
- `0` [keywordDetailColumnIDsForRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3683): prompt construction and local validation only
- `0` [keywordRowsFromLemna](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3687): prompt construction and local validation only
- `0` [keywordRowsHaveLabelName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3691): prompt construction and local validation only
- `0` [keywordRowsHaveProteinID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3700): prompt construction and local validation only
- `0` [sourceDatabaseForBlastRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3709): prompt construction and local validation only
- `0` [sourceDatabaseForKeywordRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3718): prompt construction and local validation only
- `1A` [ExportBaseName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3727): prompt flow with UI/task orchestration
- `1A` [ExportSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3756): prompt flow with UI/task orchestration
- `1A` [PostRunAction](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3807): prompt flow with UI/task orchestration
- `1A` [toggleSelections](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3860): prompt flow with UI/task orchestration
- `1A` [applySelectionCommand](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3874): prompt flow with UI/task orchestration
- `0` [setDisplayIndexes](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3920): prompt construction and local validation only
- `1A` [countSelected](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3927): prompt flow with UI/task orchestration
- `0` [parseRowSpec](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3937): prompt construction and local validation only
- `0` [commandTargetValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3978): prompt construction and local validation only
- `0` [defaultRowOrder](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3989): prompt construction and local validation only
- `1A` [identityRowOrder](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:3997): prompt flow with UI/task orchestration
- `0` [FetchErrorAction](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4011): prompt construction and local validation only
- `0` [WorkflowErrorAction](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4022): prompt construction and local validation only
- `0` [BlastSubmitErrorAction](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4033): prompt construction and local validation only
- `1A` [recoveryErrorAction](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4043): prompt flow with UI/task orchestration
- `1A` [BlastPlusInstallAction](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4066): prompt flow with UI/task orchestration
- `0` [parseKeywordIdentityValues](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4093): prompt construction and local validation only
- `0` [countKeywordResultRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4107): prompt construction and local validation only
- `0` [parseBlastIdentityValues](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4115): prompt construction and local validation only
- `0` [keywordLabelName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4127): prompt construction and local validation only
- `0` [blastLabelName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4131): prompt construction and local validation only
- `0` [keywordRowDetail](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4135): prompt construction and local validation only
- `0` [blastRowDetail](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4176): prompt construction and local validation only
- `0` [blastDetailDisplayValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4257): prompt construction and local validation only
- `0` [blastDetailLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4267): prompt construction and local validation only
- `0` [displayPreviewValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4271): prompt construction and local validation only
- `0` [sanitizeFileName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4279): prompt construction and local validation only
- `0` [looksLikeURL](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/prompt/prompt.go:4286): prompt construction and local validation only

## `internal/report/blast_filter_helpers.go`

- `1A` [BlastFilterSettingDetails](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/blast_filter_helpers.go:11): UI-facing report assembly or preview-related render logic
- `0` [BlastFilterFormulas](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/blast_filter_helpers.go:114): report data shaping helper
- `0` [formatBlastFilterFloat](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/blast_filter_helpers.go:136): report data shaping helper
- `0` [formatBlastFilterEValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/blast_filter_helpers.go:140): report data shaping helper
- `0` [interProDomainModeLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/blast_filter_helpers.go:147): report data shaping helper
- `0` [formatPercent](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/blast_filter_helpers.go:160): report data shaping helper
- `0` [BlastFilterSettingGroups](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/blast_filter_helpers.go:167): report data shaping helper
- `0` [BlastFilterSettingDetailsByGroup](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/blast_filter_helpers.go:179): report data shaping helper

## `internal/report/files.go`

- `0` [InspectGeneratedFile](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/files.go:18): report data shaping helper
- `0` [PlannedReportFile](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/files.go:48): report data shaping helper
- `2A` [hashFile](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/files.go:67): report/PDF rendering and artifact generation belong in heavy local work

## `internal/report/pdf.go`

- `2A` [newPDFReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:72): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [save](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:95): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [ensureSystemFonts](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:113): report/PDF rendering and artifact generation belong in heavy local work
- `1A` [addPage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:136): UI-facing report assembly or preview-related render logic
- `1A` [drawHeader](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:142): UI-facing report assembly or preview-related render logic
- `1A` [ensure](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:161): UI-facing report assembly or preview-related render logic
- `1A` [title](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:172): UI-facing report assembly or preview-related render logic
- `1A` [chapterHeading](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:188): UI-facing report assembly or preview-related render logic
- `1A` [subheading](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:208): UI-facing report assembly or preview-related render logic
- `1A` [paragraph](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:219): UI-facing report assembly or preview-related render logic
- `1A` [note](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:230): UI-facing report assembly or preview-related render logic
- `1A` [cards](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:255): UI-facing report assembly or preview-related render logic
- `1A` [table](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:318): UI-facing report assembly or preview-related render logic
- `1A` [selectionChart](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:433): UI-facing report assembly or preview-related render logic
- `1A` [termHitChart](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:455): UI-facing report assembly or preview-related render logic
- `1A` [provenanceChart](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:477): UI-facing report assembly or preview-related render logic
- `1A` [tableCompletenessChart](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:501): UI-facing report assembly or preview-related render logic
- `1A` [termBars](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:529): UI-facing report assembly or preview-related render logic
- `1A` [durationBars](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:570): UI-facing report assembly or preview-related render logic
- `1A` [legend](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:622): UI-facing report assembly or preview-related render logic
- `1A` [legendBox](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:626): UI-facing report assembly or preview-related render logic
- `1A` [legendAt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:634): UI-facing report assembly or preview-related render logic
- `1A` [legendHeight](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:661): UI-facing report assembly or preview-related render logic
- `1A` [donut](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:675): UI-facing report assembly or preview-related render logic
- `1A` [annularSegment](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:695): UI-facing report assembly or preview-related render logic
- `1A` [circle](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:712): UI-facing report assembly or preview-related render logic
- `1A` [text](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:719): UI-facing report assembly or preview-related render logic
- `1A` [line](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:741): UI-facing report assembly or preview-related render logic
- `1A` [fillRect](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:748): UI-facing report assembly or preview-related render logic
- `1A` [rect](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:755): UI-facing report assembly or preview-related render logic
- `1A` [strokeRect](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:763): UI-facing report assembly or preview-related render logic
- `1A` [drawFooter](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:770): UI-facing report assembly or preview-related render logic
- `2A` [setFillColor](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:783): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [setDrawColor](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:787): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [setTextColor](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:791): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [colorByte](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:795): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [resolveSystemReportFont](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:811): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [firstReadableFont](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:828): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [systemSansRegularCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:850): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [systemSansBoldCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:870): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [firstFontFaceBytes](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:889): report/PDF rendering and artifact generation belong in heavy local work
- `1A` [extractFontFace](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:901): UI-facing report assembly or preview-related render logic
- `2A` [u16](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:951): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [u32](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:955): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [putU32](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:959): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [wrapText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:966): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [wrapTextLine](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:981): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [splitByVisualWidth](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:1020): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [visualWidth](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:1043): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [runeVisualWidth](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:1051): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [truncate](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:1074): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [formatTime](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:1092): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [formatDurationMS](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:1099): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [stepDuration](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:1109): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [fileSizeText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:1119): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [sortedRowsFromNameValues](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:1132): report/PDF rendering and artifact generation belong in heavy local work
- `1A` [sortedProvenanceRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:1140): UI-facing report assembly or preview-related render logic
- `2A` [pdfBytesForTest](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/pdf.go:1151): report/PDF rendering and artifact generation belong in heavy local work

## `internal/report/platform.go`

- `0` [PlatformDisplayName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/platform.go:3): report data shaping helper

## `internal/report/render_blast.go`

- `2A` [RenderBlastPDF](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:12): report/PDF rendering and artifact generation belong in heavy local work
- `1A` [renderBlastReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:27): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastGeneratedFileIndex](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:60): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastRuntimeContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:76): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastTiming](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:114): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastSourceContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:136): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastInputResolution](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:175): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastExecution](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:198): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastSelection](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:229): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastExternalReferences](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:261): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastUniProt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:275): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastInterPro](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:290): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastFamily](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:302): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastFilter](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:327): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastColumns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:415): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastExportLog](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:451): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastSequenceAudit](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:463): UI-facing report assembly or preview-related render logic
- `1A` [renderBlastFileAppendix](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:505): UI-facing report assembly or preview-related render logic
- `2A` [blastFileRunLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:532): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [normalizeReportMatchKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:552): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [blastReferenceSummary](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:561): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [blastFamilySummary](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:574): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [blastFilterSummary](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:584): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [blastExecutiveParagraph](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:600): report/PDF rendering and artifact generation belong in heavy local work
- `1A` [flow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:617): UI-facing report assembly or preview-related render logic
- `1A` [inputTypeMosaic](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:651): UI-facing report assembly or preview-related render logic
- `1A` [inputFunnel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:692): UI-facing report assembly or preview-related render logic
- `1A` [blastSelectionChart](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:703): UI-facing report assembly or preview-related render logic
- `1A` [blastRunBars](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:724): UI-facing report assembly or preview-related render logic
- `1A` [blastMetricAvailability](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:765): UI-facing report assembly or preview-related render logic
- `1A` [qualitySeverityChart](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:782): UI-facing report assembly or preview-related render logic
- `1A` [uniProtOutcomeChart](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:807): UI-facing report assembly or preview-related render logic
- `1A` [interProStatusChart](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:812): UI-facing report assembly or preview-related render logic
- `1A` [familyMergeChart](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:830): UI-facing report assembly or preview-related render logic
- `1A` [filterMatrix](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:859): UI-facing report assembly or preview-related render logic
- `1A` [filterRecommendationChart](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:867): UI-facing report assembly or preview-related render logic
- `1A` [filterQueryBars](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:884): UI-facing report assembly or preview-related render logic
- `1A` [filterDifferenceChart](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:963): UI-facing report assembly or preview-related render logic
- `1A` [filterRuleFailureBars](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:982): UI-facing report assembly or preview-related render logic
- `1A` [blastCompletenessChart](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:1021): UI-facing report assembly or preview-related render logic
- `1A` [sequenceChart](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:1047): UI-facing report assembly or preview-related render logic
- `1A` [sequenceLengthDotPlot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:1061): UI-facing report assembly or preview-related render logic
- `2A` [percentText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:1123): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [maxIntReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:1130): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [minIntReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:1137): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [floatValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_blast.go:1144): report/PDF rendering and artifact generation belong in heavy local work

## `internal/report/render_keyword.go`

- `2A` [RenderKeywordPDF](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:11): report/PDF rendering and artifact generation belong in heavy local work
- `1A` [renderKeywordReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:23): UI-facing report assembly or preview-related render logic
- `1A` [renderGeneratedFileIndex](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:52): UI-facing report assembly or preview-related render logic
- `1A` [renderRuntimeContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:74): UI-facing report assembly or preview-related render logic
- `1A` [renderTiming](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:117): UI-facing report assembly or preview-related render logic
- `1A` [renderDataSource](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:135): UI-facing report assembly or preview-related render logic
- `1A` [renderMatching](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:155): UI-facing report assembly or preview-related render logic
- `1A` [renderLabels](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:190): UI-facing report assembly or preview-related render logic
- `1A` [renderSelection](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:206): UI-facing report assembly or preview-related render logic
- `1A` [renderProvenanceAndQuality](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:234): UI-facing report assembly or preview-related render logic
- `1A` [renderColumns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:257): UI-facing report assembly or preview-related render logic
- `1A` [renderExportLog](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:283): UI-facing report assembly or preview-related render logic
- `1A` [renderSequenceAudit](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:306): UI-facing report assembly or preview-related render logic
- `1A` [renderFileAppendix](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:349): UI-facing report assembly or preview-related render logic
- `2A` [valueOr](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:374): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [availableValueRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:381): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [reportValueAvailable](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:391): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [performanceInterpretation](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:399): report/PDF rendering and artifact generation belong in heavy local work
- `1A` [selectionInterpretation](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:419): UI-facing report assembly or preview-related render logic
- `1A` [completenessInterpretation](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/render_keyword.go:430): UI-facing report assembly or preview-related render logic

## `internal/report/sample.go`

- `1A` [SampleKeywordReportData](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:20): UI-facing report assembly or preview-related render logic
- `0` [SampleKeywordPhytozomeReportData](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:149): report data shaping helper
- `2A` [SampleBlastReportData](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:180): report/PDF rendering and artifact generation belong in heavy local work
- `0` [SampleBlastPhytozomeReportData](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:426): report data shaping helper
- `0` [SampleBlastWithoutReferencesReportData](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:446): report data shaping helper
- `0` [filterCompletenessWithoutLabels](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:481): report data shaping helper
- `0` [filterProvenanceLabels](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:496): report data shaping helper
- `0` [filterQualityChecksBySource](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:511): report data shaping helper
- `1A` [sampleSession](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:526): UI-facing report assembly or preview-related render logic
- `0` [sampleFiles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:565): report data shaping helper
- `0` [sampleBlastFiles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:574): report data shaping helper
- `0` [sampleBlastFilterSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:583): report data shaping helper
- `0` [sampleBlastFilterSettingsWithoutReferences](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:587): report data shaping helper
- `0` [sampleBlastFilterTotals](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:604): report data shaping helper
- `0` [sampleBlastColumnCompleteness](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:620): report data shaping helper
- `1A` [sampleBlastQualityChecks](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:636): UI-facing report assembly or preview-related render logic
- `0` [sampleBlastFilterHardRulesWithoutReferences](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:651): report data shaping helper
- `0` [sampleBlastFilterFormulasWithoutReferences](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:659): report data shaping helper
- `0` [sampleBlastColumns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:674): report data shaping helper
- `1A` [sampleBlastSteps](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:695): UI-facing report assembly or preview-related render logic
- `0` [sampleFile](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:715): report data shaping helper
- `0` [sampleKeywordColumns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:739): report data shaping helper
- `0` [sampleColumnSource](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:760): report data shaping helper
- `0` [sampleColumnCollection](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:779): report data shaping helper
- `0` [sampleColumnBlankMeaning](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:802): report data shaping helper
- `0` [sampleColumnStatsUse](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:821): report data shaping helper
- `1A` [sampleSteps](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:832): UI-facing report assembly or preview-related render logic
- `0` [firstEnv](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:852): report data shaping helper
- `0` [envPair](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:861): report data shaping helper
- `0` [nonEmpty](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/sample.go:869): report data shaping helper

## `internal/report/types.go`

- `0` [ReportFileName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/types.go:459): report data shaping helper
- `0` [ReportFileNameForBase](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/types.go:466): report data shaping helper
- `0` [sanitizeReportFileBase](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/types.go:474): report data shaping helper

## `internal/report/workers.go`

- `2A` [init](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/workers.go:27): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [RenderKeywordPDFProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/workers.go:58): report/PDF rendering and artifact generation belong in heavy local work
- `2A` [RenderBlastPDFProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/report/workers.go:67): report/PDF rendering and artifact generation belong in heavy local work

## `internal/searchengine/lemnakeyword/cache.go`

- `0` [writeCachedJSON](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/lemnakeyword/cache.go:15): tiny search-engine glue helper

## `internal/searchengine/lemnakeyword/engine.go`

- `0` [New](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/lemnakeyword/engine.go:22): tiny search-engine glue helper
- `2B` [SearchKeywordRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/lemnakeyword/engine.go:26): exported search-engine wrapper now declares explicit phygoboost network work for Lemna keyword search
- `2B` [SearchKeywordRowsWide](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/lemnakeyword/engine.go:40): exported search-engine wrapper now declares explicit phygoboost network work for forced wide Lemna search
- `2B` [SearchKeywordRowsBroad](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/lemnakeyword/engine.go:53): exported search-engine wrapper now declares explicit phygoboost network work for forced broad Lemna search

## `internal/searchengine/phytozomekeyword/cache.go`

- `0` [writeCachedJSON](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/cache.go:15): tiny search helper

## `internal/searchengine/phytozomekeyword/engine.go`

- `0` [New](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:97): tiny search helper
- `2B` [SearchKeywordRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:104): exported search-engine wrapper now declares explicit phygoboost network work for Phytozome keyword search
- `2B` [SearchKeywordRowsWide](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:115): exported search-engine wrapper now declares explicit phygoboost network work for forced wide Phytozome search
- `2B` [searchKeywordRowsWithProgram](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:126): all search paths are forced into class 2, and this search engine is part of that pipeline
- `0` [selectProgram](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:180): tiny search helper
- `0` [cacheKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:198): tiny search helper
- `0` [buildRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:208): tiny search helper
- `0` [cacheableResult](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:220): tiny search helper
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:237): tiny search helper
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:239): tiny search helper
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:244): all search paths are forced into class 2, and this search engine is part of that pipeline
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:254): tiny search helper
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:256): tiny search helper
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:260): all search paths are forced into class 2, and this search engine is part of that pipeline
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:266): tiny search helper
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:268): tiny search helper
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:272): all search paths are forced into class 2, and this search engine is part of that pipeline
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:284): tiny search helper
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:286): tiny search helper
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:290): all search paths are forced into class 2, and this search engine is part of that pipeline
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:296): tiny search helper
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:298): tiny search helper
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:302): all search paths are forced into class 2, and this search engine is part of that pipeline
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:312): tiny search helper
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:314): tiny search helper
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:318): all search paths are forced into class 2, and this search engine is part of that pipeline
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:324): tiny search helper
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:326): tiny search helper
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:330): all search paths are forced into class 2, and this search engine is part of that pipeline
- `0` [Name](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:336): tiny search helper
- `0` [Match](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:338): tiny search helper
- `2B` [Search](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:342): all search paths are forced into class 2, and this search engine is part of that pipeline
- `2B` [searchSpecificIdentifier](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:414): all search paths are forced into class 2, and this search engine is part of that pipeline
- `2B` [shouldStopSpecificIdentifierSearch](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:470): all search paths are forced into class 2, and this search engine is part of that pipeline
- `2B` [searchAliasesAsGenes](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:496): all search paths are forced into class 2, and this search engine is part of that pipeline
- `2B` [searchKeyword](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:524): all search paths are forced into class 2, and this search engine is part of that pipeline
- `0` [addGene](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:540): tiny search helper
- `2B` [PhytozomeGeneReportKeyword](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:552): all search paths are forced into class 2, and this search engine is part of that pipeline
- `0` [LooksLikeSpecificGeneIdentifier](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:582): tiny search helper
- `0` [SpecificIdentifierVariants](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:603): tiny search helper
- `0` [riceLocusVariants](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:638): tiny search helper
- `0` [normalizeRiceLocusCandidate](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:646): tiny search helper
- `0` [aliasesForNormalizedTerm](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:665): tiny search helper
- `0` [curatedRiceRefSeqAliasMap](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:674): tiny search helper
- `0` [curatedRiceAliasMap](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:683): tiny search helper
- `2B` [wideKeywordQuery](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:695): all search paths are forced into class 2, and this search engine is part of that pipeline
- `2B` [relaxedKeywordQueries](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:703): all search paths are forced into class 2, and this search engine is part of that pipeline
- `0` [normalizeAliasKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:732): tiny search helper
- `0` [normalizeTermKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:738): tiny search helper
- `0` [nonEmptyPathSegments](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:742): tiny search helper
- `0` [PrimaryTranscript](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:754): tiny search helper
- `0` [PrimaryTranscriptByProtein](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:772): tiny search helper
- `0` [OrganismShortName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:785): tiny search helper
- `0` [AnnotationVersion](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:789): tiny search helper
- `0` [ProteomeID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/engine.go:793): tiny search helper

## `internal/searchengine/phytozomekeyword/row.go`

- `2B` [buildKeywordResultRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:17): all search paths are forced into class 2, and this search engine is part of that pipeline
- `2B` [formatKeywordGenome](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:62): all search paths are forced into class 2, and this search engine is part of that pipeline
- `2B` [formatKeywordLocation](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:85): all search paths are forced into class 2, and this search engine is part of that pipeline
- `0` [dedupePreserveOrder](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:93): tiny search helper
- `0` [bestAlias](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:111): tiny search helper
- `0` [BestQuerySourceLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:131): tiny search helper
- `0` [querySourceAliasCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:159): tiny search helper
- `0` [querySourceLabelPreferenceBonus](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:180): tiny search helper
- `0` [looksLikePrimaryFamilySymbol](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:196): tiny search helper
- `0` [aliasPreferenceScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:215): tiny search helper
- `0` [noLowercase](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:279): tiny search helper
- `0` [aliasHasInternalDigitPattern](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:288): tiny search helper
- `0` [lowercaseCount](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:308): tiny search helper
- `0` [looksLikeECNumberLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:318): tiny search helper
- `0` [looksLikeDatabaseIdentifierLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:322): tiny search helper
- `0` [labelFromAutoDefine](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:338): tiny search helper
- `0` [autoDefineCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:351): tiny search helper
- `0` [autoDefineLabelScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:376): tiny search helper
- `0` [looksLikeAliasToken](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:401): tiny search helper
- `0` [CopyStringSlice](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:419): tiny search helper
- `0` [copyStringSlice](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:428): tiny search helper
- `0` [firstNonEmpty](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/searchengine/phytozomekeyword/row.go:432): tiny search helper

## `internal/tui/clipboard.go`

- `1A` [readClipboardText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/clipboard.go:13): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [writeClipboardText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/clipboard.go:58): UI-heavy loading, table, rendering, or progress orchestration belongs in main process

## `internal/tui/console_other.go`

- `1A` [installConsoleCloseHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/console_other.go:7): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [installConsoleResizeWatcher](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/console_other.go:11): UI-heavy loading, table, rendering, or progress orchestration belongs in main process

## `internal/tui/console_windows.go`

- `1A` [installConsoleCloseHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/console_windows.go:67): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [installConsoleResizeWatcher](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/console_windows.go:92): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [currentConsoleViewportSize](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/console_windows.go:139): UI-heavy loading, table, rendering, or progress orchestration belongs in main process

## `internal/tui/helpers.go`

- `1A` [pageBreadcrumb](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:11): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [tableHeaderText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:25): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [setFocusBorder](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:29): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [attachFocusBorder](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:40): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [normalizeSelection](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:52): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [setAll](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:64): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [paddedTableCell](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:70): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [rowSelectionColumnWidths](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:74): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [compareRowOrder](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:96): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [rowSelectionGroups](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:124): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [tableCellColor](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:154): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [tableHeaderLines](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:167): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [tableHeaderStyle](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:179): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [cloneBoolMatrix](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:190): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [runTaskValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:218): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [runProgressTaskValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:226): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [actionCloseValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:215): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [actionLooksLikeClose](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:224): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [textViewLineCount](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/helpers.go:229): UI-heavy loading, table, rendering, or progress orchestration belongs in main process

## `internal/tui/task_runner_main.go`

- `1A` [runTaskValueInMainProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/task_runner_main.go:13): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [runProgressTaskValueInMainProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/task_runner_main.go:21): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [runTaskModal](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/task_runner_main.go:29): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [normalizedTaskProgressSlot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/task_runner_main.go:172): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [chooseTaskTotal](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/task_runner_main.go:180): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [normalizedTaskTotal](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/task_runner_main.go:191): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [normalizeTaskProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/task_runner_main.go:196): UI-heavy loading, table, rendering, or progress orchestration belongs in main process

## `internal/tui/overlay.go`

- `1A` [rememberBackground](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:15): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [currentBackground](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:24): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [setPageRoot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:34): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [newModalOverlay](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:51): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [SetSize](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:66): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Draw](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:78): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [InputHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:116): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Focus](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:124): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [HasFocus](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:133): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Blur](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:137): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [MouseHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:144): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [PasteHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:166): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [modalRoot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:174): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [taskModalRoot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:182): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [newTaskModalRoot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:187): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [infoModalRoot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:196): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [overlayRootOn](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/overlay.go:204): UI-heavy loading, table, rendering, or progress orchestration belongs in main process

## `internal/tui/select.go`

- `1A` [SelectStartup](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:52): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [newApp](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:207): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [runApp](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:224): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [configStyles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:237): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [installInputCapture](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:255): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [chainInputCaptures](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:259): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [remapTabCapture](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:274): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [startupRoot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:288): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [pageFrame](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:326): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [buttonRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:342): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [optionList](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:346): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [optionListWithStart](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:350): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [setOptionListItems](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:365): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [focusStartupList](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:380): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [hintView](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:384): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [currentItem](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:390): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [moveChoiceSelection](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:398): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [productName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:415): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [fallbackText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:423): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [trimColon](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:431): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [elideBreadcrumb](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/select.go:435): UI-heavy loading, table, rendering, or progress orchestration belongs in main process

## `internal/tui/templates.go`

- `1A` [SetInputFileLoader](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:32): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [currentInputFileLoader](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:44): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [newRowSelectionLayout](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:190): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [newCheckboxModule](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:397): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [newBlastRunSidebar](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:406): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [SetItems](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:414): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [SetCurrentItem](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:419): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [GetOffset](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:425): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [SetOffset](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:430): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [SetChangedFunc](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:435): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [clamp](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:439): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [keepCurrentVisible](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:460): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [itemHeight](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:475): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [itemStartRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:486): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [totalRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:500): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [rowToItem](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:508): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [choose](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:523): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Draw](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:535): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [InputHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:571): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [MouseHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:592): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [MouseHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:628): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [scrollColumns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:649): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [toggleChecked](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:668): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Draw](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:675): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [InputHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:714): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [MouseHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:725): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunChoicePage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:1127): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunGroupedChoicePage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:1202): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunTextInputPage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:1371): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunMultiLinePage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:1452): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunSearchPage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:1590): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunRowSelectionPage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:1981): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunBlastRunSelectionPage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:2868): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunTaskPage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3661): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunTaskPageContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3668): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [taskCancelError](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3675): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [initialTaskStatus](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3682): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [taskTitle](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3696): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [taskSubtitle](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3710): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [taskProgressRender](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3721): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [taskProgressSlotHeight](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3751): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [taskModalHeightForSlots](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3762): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [clampTaskProgressSlots](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3777): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [register](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3806): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [update](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3832): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [remove](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3857): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [snapshot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3863): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [renderTaskProgressSlots](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3935): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [taskChildRegistrarFromContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3967): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RegisterTaskChildSlot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3975): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunInfoPage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:3984): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunActionModalPage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:4022): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunRecoveryModalPage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:4108): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunChoiceModalPage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:4130): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [choiceModalOptions](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:4235): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunExportSettingsModal](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:4247): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunExternalReferenceModal](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:4479): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunFamilyBlastModal](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:4794): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [compactFamilyBlastGroupLabels](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:5073): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [compactFamilyBlastMembers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:5091): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [groupsToCustomGroups](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:5132): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [familyBlastMemberDisplay](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:5149): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [familyBlastMemberInlineDisplay](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:5161): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [familyBlastPreviewMemberText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:5173): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [visibleTreeNodes](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:5177): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [familyGroupUngroupedLabels](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:5198): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [familyBlastMemberKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:5217): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [sortFamilyBlastMembersStable](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:5221): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [buildFamilyBlastCustomizeModal](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:5237): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunFamilyBlastCustomizeModal](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:6186): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [splitSidebarLines](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:6203): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [RunBlastFilterModal](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:6216): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [normalizeTUIInterProSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:6815): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [normalizeTUIBlastFilterSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:6839): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [normalizeTUIFamilyBlastSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:6996): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [familyBlastHelpPages](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7009): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [blastFilterHelpPages](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7050): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [interProParameterHelpPages](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7080): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [columnHelpPages](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7106): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [columnHelpPageText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7125): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [splitTrilingualHelp](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7137): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [tableTitleWithCount](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7180): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [tableLineCountLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7189): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [countSelectedBools](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7202): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [displayModalValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7212): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [wrapPlainText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7220): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [modalFramePage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7252): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [newLocalizedHelpModal](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7260): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [normalizeLocalizedHelpPages](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7299): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [SetLanguage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7326): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Body](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7351): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Title](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7358): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [TextView](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7365): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [HandleKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7372): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [scrollTextView](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7407): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [isCopyShortcut](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7419): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [maxInt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7432): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [minInt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7439): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [modalHeightForContent](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7446): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [rowSelectionFirstSortableColumn](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7457): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [rowSelectionSortArrow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7466): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [compareTableValues](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7475): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [firstNonEmptyText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7495): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [indentSecondary](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7504): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [defaultChoiceFilter](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7516): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [searchResultOffsetForSelection](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7531): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [textBlock](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7563): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [textPanel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7571): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [sectionHeader](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7581): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [centeredPrimitive](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7593): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [addHints](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7613): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [navButtons](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7623): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [navButtonsWithShortcut](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7627): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [modalButtons](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7636): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [inputButtons](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7653): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [clipPrimitive](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7663): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Draw](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7667): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [InputHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7689): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Focus](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7700): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [HasFocus](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7708): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Blur](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7715): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [MouseHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7722): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [PasteHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7738): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Draw](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7749): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [InputHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7759): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Focus](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7770): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [HasFocus](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7784): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Blur](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7791): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [MouseHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7798): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [PasteHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7814): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [primitiveContains](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7825): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [primitiveBox](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7830): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [inputFieldEditKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7856): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [deliverInputFieldKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7868): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [SetContent](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7881): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [ShowCursor](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7888): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [newButtonFlex](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7896): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [addButtonRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7900): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Draw](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7910): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [syncButtonRowHeights](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7915): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Draw](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7937): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [newButtonRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7963): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [InputHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7970): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [setPrimaryLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:7999): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [dynamicPrimaryButton](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8008): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [hasDynamicPrimary](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8020): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [primaryButton](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8032): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [inputConfirmText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8049): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [compactButtonLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8059): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [printStyledText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8068): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [Draw](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8098): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [MouseHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8145): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [pageLines](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8200): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [printCentered](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8232): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [MouseHandler](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8255): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [visibleButtonGroups](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8295): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [buttonPositions](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8311): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [overlapsButtonPositions](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8362): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [requiredHeight](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8374): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [buttonWidth](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8384): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [navCapture](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8400): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [isCtrlEnter](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8435): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [selectionKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8450): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [newPasteStatus](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8460): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [runInlinePaste](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8467): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [resolveInputFileText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8503): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [showInputFileError](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8527): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [pastedFilePath](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8538): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [sanitizePastedText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8581): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [stripANSIEscapeSequences](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8613): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [conciseActionLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8639): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [shortcutMatchesEvent](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/templates.go:8666): UI-heavy loading, table, rendering, or progress orchestration belongs in main process

## `internal/tui/worker.go`

- `1A` [inTUIWorker](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/worker.go:24): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [tuiWorkerEnabled](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/worker.go:28): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [tuiPageWorkerEnabled](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/worker.go:32): UI-heavy loading, table, rendering, or progress orchestration belongs in main process
- `1A` [tuiTaskWorkerEnabled](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/tui/worker.go:36): UI-heavy loading, table, rendering, or progress orchestration belongs in main process

## `internal/uniprot/uniprot.go`

- `0` [NewClient](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:74): UniProt parsing helper
- `2B` [Lookup](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:84): external reference lookup/search should stay in class 2 network work
- `2B` [lookupByQuery](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:107): external reference lookup/search should stay in class 2 network work
- `0` [candidateQueries](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:178): UniProt parsing helper
- `0` [readDiskEntry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:234): UniProt parsing helper
- `0` [writeDiskEntry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:242): UniProt parsing helper
- `2A` [parseTSV](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:246): reference cache is tightly coupled to the same heavy lookup pipeline
- `0` [chooseEntry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:341): UniProt parsing helper
- `0` [populatedScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:361): UniProt parsing helper
- `0` [normalizeAccession](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:384): UniProt parsing helper
- `0` [cleanIdentifier](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:397): UniProt parsing helper
- `2B` [extractUniProtAccessions](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:411): external reference lookup/search should stay in class 2 network work
- `2B` [looksLikeUniProtAccession](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:435): external reference lookup/search should stay in class 2 network work
- `0` [stripIsoform](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:456): UniProt parsing helper
- `0` [cleanOrganism](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:476): UniProt parsing helper
- `0` [cleanValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:487): UniProt parsing helper
- `0` [ToJSON](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/uniprot/uniprot.go:493): UniProt parsing helper

## `internal/workflow/blast.go`

- `0` [isMissingProteinSequenceError](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:102): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [contextWithUpdate](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:315): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [updateFromContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:322): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [updateWithContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:332): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [safeProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:345): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [safeTaskUpdate](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:353): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [contextWithTaskSlot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:361): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [taskSlotsFromContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:374): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [registerWorkflowTaskSlot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:382): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [workflowTaskSlotUpdate](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:401): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [tuiTaskProgressSlot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:410): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [Error](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:428): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [Unwrap](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:435): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [Error](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:455): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [Unwrap](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:470): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [Error](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:483): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [Unwrap](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:490): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [NewBlastWizard](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:497): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [NewBlastWizardWithTUIInfo](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:501): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [Run](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:521): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [dataSourceForName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:710): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [chooseMode](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:724): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [chooseDataSource](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:735): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [runStartupTool](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:756): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [isLemnaSource](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:769): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [setBlastProgramContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:774): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [tuiPath](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:779): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [classifyWizardBack](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:808): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [consumeKeywordInputRewind](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:833): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [rewindKeywordRowsToInput](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:842): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [consumeBlastInputRewind](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:848): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [rewindModeToInput](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:857): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [configureBlastRequest](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:866): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [configureBlastRequestBeforeInput](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:898): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [applyBlastProgram](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:921): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [blastProgramPathLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:942): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2A` [detectLemnaBlastCapabilities](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:953): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `0` [detectLemnaBlastCapabilitiesDirect](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:970): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [availableBlastProgramsFromCapability](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:977): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [chooseLemnaBlastExecution](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:994): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2A` [loadSpeciesCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1015): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `0` [cacheSpeciesCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1049): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [speciesCandidatesForSource](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1060): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [phytozomeHelperSource](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1094): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `1A` [selectSpecies](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1106): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `2B` [runKeywordMode](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1155): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [runKeywordBlastMode](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1343): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [resolveKeywordRowsToBlastItems](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1388): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [keywordRowsToBlastItems](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1408): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [runPreparedBlastMode](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1447): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [executePreparedBlast](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1504): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [runBlastMode](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1544): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [collectExternalReferenceConfig](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1658): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [collectFamilyBlastPlan](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1670): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [applyFamilyBlastGroupLabels](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1707): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [buildPromptFamilyBlastPreview](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1718): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [promptFamilyBlastGroups](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1742): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [promptFamilyBlastMembers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1755): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [promptFamilyBlastMember](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1763): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [customPromptFamilyBlastGroups](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1773): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [promptGroupMembers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1854): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [executeConfiguredBlastBatch](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1873): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [executeConfiguredBlastBatchWithReferences](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1877): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2A` [executeConfiguredBlastBatchRuns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:1950): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `0` [parallelBlastBatchResumeError](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2091): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [newBlastBatchProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2139): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [withContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2148): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [updateRunSlot](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2157): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [baseOffset](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2164): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [runPhaseOffset](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2171): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [runPhaseSpan](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2175): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [finalizePhaseOffset](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2182): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [finalizePhaseSpan](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2186): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [referencePhaseOffset](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2193): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [referencePhaseSpan](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2197): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [finishOffset](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2204): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [referenceTaskIndex](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2211): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [startLocalPrepare](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2234): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [finishLocalPrepare](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2240): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [submitting](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2246): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [submitted](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2253): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [waiting](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2260): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [running](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2267): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [finishedRun](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2279): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [annotating](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2299): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [annotated](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2306): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [annotateBlastRowsForTargetSpecies](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4103): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [executeBlastRunPipeline](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2313): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2A` [finalizeBlastRunPipeline](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2345): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `0` [enrichBlastRunBatch](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2396): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [blastReferenceTaskShape](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2437): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2A` [primeBlastReferenceInputs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2456): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `0` [collectBlastAccessionTasks](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2511): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [collectBlastAccessionRowsByTask](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2531): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [accessionSignature](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2542): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [collectBlastInterProQueryItems](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2562): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [enrichBlastRunsWithUniProtBatch](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2579): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [enrichBlastRunsWithInterProBatch](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2682): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2A` [executeLocalBlastPipeline](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2794): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `2A` [canUseDirectLocalBlastBatchRunner](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2822): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `2A` [newLocalBlastBatchRunner](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2830): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `0` [executeLocalBlastBatchRun](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2842): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2A` [executeCombinedRemoteBlastPipeline](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2858): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `2A` [executeSplitRemoteBlastPipeline](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2876): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `0` [executeBlastRunJob](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2929): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [wrapBlastRunStageError](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2967): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [firstBlastPipelineError](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:2981): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [completedBlastRuns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3015): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [blastItemsFromRuns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3025): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [prepareLocalBlastForBatch](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3033): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [batchBlastDescription](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3048): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [batchBlastProgressTotal](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3055): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `1A` [resumeBlastRowSelection](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3069): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `0` [reviewBlastRuns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3176): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [reviewSingleBlastRun](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3187): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [reviewMultiBlastRuns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3248): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [blastRunViews](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3306): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [warmBlastRunsSequenceCache](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3325): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2A` [warmBlastSequenceCache](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3333): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `2B` [warmKeywordSequenceCache](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3354): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `1A` [exportSingleBlastRun](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3376): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `1A` [exportAllBlastRuns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3454): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `1A` [exportAllBlastRunsWithProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3535): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `0` [parallelBlastExportResumeFailure](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3724): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [isCancellationLikeError](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3744): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `1A` [prefetchBlastExportBatchSequences](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3751): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `0` [uniqueExportPrefix](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3772): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `1A` [buildBlastSelectedMaskFromSelection](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3785): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `0` [hasExportedBlastFiles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3804): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [removeExportedBlastRuns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3813): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [blastReportInputPreparedForItem](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3831): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [blastQuerySourceSame](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3858): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [applyBlastLabelToRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3866): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [annotateBlastRowsForQueryContext](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3875): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [fillBlastQueryLength](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3903): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [coordinateSpan](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3931): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [annotateBlastRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3941): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [enrichBlastRowsWithUniProt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3959): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [enrichBlastRowsWithUniProtProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:3983): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [uniProtLookupGroups](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4027): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [uniProtLookupGroupCount](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4042): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [uniProtLookupKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4050): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [lookupUniProtEntry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4066): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [lookupUniProtEntryWithAccessions](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4070): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [uniqueStrings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4095): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [blastRowLookupIdentity](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4113): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [blastRowAccessionLookupIdentity](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4124): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [cachedUniProtAccessions](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4139): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [storeUniProtAccessions](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4152): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [uniprotAccessionsForBlastRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4167): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [phytozomeTargetIDForRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4219): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [applyUniProtEntry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4271): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [enrichBlastRowsWithInterPro](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4309): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [enrichBlastRowsWithInterProProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4334): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [lookupInterProQueryEntry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4376): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [storeInterProQueryEntry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4416): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [interProQueryCacheKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4428): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [interProQueryLookupRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4443): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [lookupInterProEntry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4480): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [lookupInterProEntryWithAccessions](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4484): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [resolverForQuerySource](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4508): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [uniprotReferenceClient](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4520): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [interproReferenceClient](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4529): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [runUniProtBatchLookup](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4538): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [runInterProBatchLookup](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4590): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [interProLookupGroups](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4647): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [interProLookupGroupCount](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4662): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [interProLookupKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4670): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [applyInterProEntry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4686): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [interProConservedRegionStatus](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4696): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [interProSelfEvidenceStatus](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4716): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [interProMatchedQueryEvidence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4740): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [interProMatchIsConservedCandidate](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4769): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [interProBestHitMatch](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4777): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [interProEvidenceScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4793): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [intersects](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4816): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [normalizeInterProConservedRegionSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4833): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [canonicalBlastProgram](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4856): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [cloneBlastQueryItems](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4864): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [cloneBlastQueryRuns](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4887): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [detectFamilyBlastGroups](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4897): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [uniqueFamilyBlastMembers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4949): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familyBlastAutoDetectionRule](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4966): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familyBlastSubgroupKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:4993): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familyBlastCanonicalSubgroupLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5008): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familyBlastQueryLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5017): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familyBlastMemberForItem](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5038): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familyBlastMemberSourceKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5074): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [splitAliasText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5095): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [setBlastQueryItemLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5108): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [mergeBlastQueryItemAliases](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5122): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [detectFamilyName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5138): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familyBlastCanonicalLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5163): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [normalizeFamilyPunctuation](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5189): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [stripLeadingFamilySpeciesPrefix](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5194): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [stripFamilyTerminalSubtypeSuffix](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5216): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [stripAfterFamilyMemberNumber](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5245): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [isFamilyVariantSeparator](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5267): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [stripFamilyTrailingIndex](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5271): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [applyFamilyBlastPlan](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5285): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [buildFamilyBlastRun](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5314): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [annotateFamilyBlastConsensusRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5389): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [All](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5453): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familySemanticTokensFromMembers](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5457): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familySemanticAnnotationAgreement](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5501): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familySemanticAnnotationText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5527): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [normalizeFamilySemanticText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5550): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [normalizeFamilySemanticToken](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5559): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [splitFamilySemanticTokens](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5570): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [foldFamilySemanticAliases](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5594): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [mergeFamilyBlastRowsByTarget](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5608): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [prioritizeFamilyBlastRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5630): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familyBlastTargetKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5638): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [betterFamilyBlastRow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5655): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familyBlastRowLess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5662): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familyBlastCoverage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5700): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familyBlastRankingOrder](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5710): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [parseFamilyBlastRankingOrder](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5718): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familyBlastReferenceScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5754): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [isTruthyWorkflow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5802): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [parseScientificFloatWorkflow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5811): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [cloneKeywordSearchGroups](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5823): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [collectBlastQueryItems](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5832): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [allLabelsPresent](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5861): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [collectBlastLabelsBeforeResolve](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5870): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [keepBlastItemsWithLabels](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5893): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [collectBlastLabels](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5903): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [prepareBlastExportItem](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5937): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2A` [autoIdentifyBlastLabelsWithProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5944): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `2A` [supplementBlastAliasesWithProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:5996): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `2A` [supplementBlastAliases](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6020): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `2B` [shouldLookupBlastAliases](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6077): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [harmonizeAutoIdentifiedBlastLabels](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6096): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [harmonizeAutoIdentifiedBlastLabelsWithLocks](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6100): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [blastAutoIdentifyLockedLabels](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6143): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [blastAutoLabelCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6151): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [blastAutoLabelCoordinationScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6181): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [looksLikeFamilyMemberStyleLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6196): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [blastLabelFallbackSpecies](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6217): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [autoIdentifyBlastLabelFromPhytozome](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6227): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [autoIdentifyBlastLabelResultFromPhytozome](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6231): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [autoIdentifyBlastLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6253): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [autoIdentifyBlastLabelResult](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6257): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [phytozomeKeywordLabelSpeciesForItem](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6285): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [phytozomeKeywordLabelSpeciesFromFastaHeader](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6312): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [phytozomeKeywordLabelSpeciesFromQuerySource](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6332): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [fastaHeaderKeywordSearchTerm](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6360): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [phytozomeKeywordLabelSpecies](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6381): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [blastLabelSearchTerms](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6393): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2A` [prepareExportSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6423): non-UI local artifact work belongs in heavy local class
- `2A` [prepareBatchExportSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6449): non-UI local artifact work belongs in heavy local class
- `0` [exportSettingsFromPrompt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6468): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2A` [loadBlastInputFile](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6479): non-UI local artifact work belongs in heavy local class
- `0` [parseBlastLoadCommand](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6508): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [parseBlastQueryItems](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6528): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [splitBlastInputRecords](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6549): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [splitURLRecordTokens](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6607): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [splitInlineBlastRecordTokens](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6622): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [isLikelyInlineSequenceToken](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6642): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [parseBlastQueryRecord](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6660): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [blastQueryItemFromFastaSource](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6675): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [allLabelsBlank](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6691): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [buildBlastOutputDisplayName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6700): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [blastTXTHeaderLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6714): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [exportItemFamilySources](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6721): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [sanitizeExportName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6731): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [reportQueryLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6746): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [blastExecutionLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6760): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2A` [resolveBlastQueryItems](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6767): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `2A` [resolveBlastQueryItemsWithProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6780): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `0` [resolveQuerySequenceInputBatchWithTimeout](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6862): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [oneLinePreview](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6874): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [batchBlastWorkerCount](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6882): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [finalizeBlastWorkerCount](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6889): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [shouldUseCombinedRemoteBlastWorker](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6896): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [combinedRemoteBlastWorkerCount](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6903): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [remoteBlastSubmitWorkerCount](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6910): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [remoteBlastWaitWorkerCount](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6917): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [referenceEnrichWorkerCount](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6924): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [localBlastThreadsPerWorker](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6932): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [clampInt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6936): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `1A` [exportBlastSelectionsToDir](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:6949): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `1A` [exportFamilyBlastSelectionsToDir](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7076): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `0` [familyTXTQueryIndexes](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7211): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familyTXTHeaderLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7224): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familyQueryPrependStepDetails](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7241): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [familySequenceHeaderMode](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7252): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [prependFamilyQuerySequenceRecords](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7259): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `1A` [exportBlastExcelAndFetchRecords](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7274): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `1A` [exportBlastExcelAndFetchRecordsSilent](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7312): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `1A` [fetchBlastRecordsForExport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7333): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `0` [filesSummary](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7352): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [collectQuerySequence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7375): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [submitBlastWithRetry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7424): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2A` [promptInstallBlastPlus](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7461): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `2B` [submitBlastOnce](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7495): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2A` [canRunLocalBlastFallback](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7557): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `0` [isLocalBlastRequest](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7599): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [normalizeWorkflowBlastProgram](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7603): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [shouldAutoFallbackToLocalBlast](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7609): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [shouldUseCombinedRemoteBlastExecution](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7624): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [waitForBlastResultsWithRetry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7631): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `1A` [selectBlastRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7669): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `1A` [selectKeywordRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7688): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `1A` [exportSelectionsWithRetry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7707): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `1A` [exportKeywordSelectionsWithRetry](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7726): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `2B` [flattenKeywordSearchGroups](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7745): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `1A` [prepareAndExportKeywordSelection](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7753): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `0` [retryWorkflowStep](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7766): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `1A` [showInfo](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7786): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `1A` [showSelection](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7811): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `1A` [showBlastResults](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7863): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `0` [buildBlastRequest](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7874): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [parseKeywordTerms](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7897): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [countKeywordRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7901): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [autoIdentifyKeywordLabels](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7909): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [autoIdentifyKeywordLabelsWithProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7917): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [autoIdentifyKeywordGroupLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7938): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [enrichKeywordLabelsFromPhytozome](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:7960): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [lemnaKeywordProteinSearchTerm](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8011): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [findArabidopsisThalianaSpecies](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8021): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [keywordLabelFromPhytozomeRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8034): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [keywordAliasesFromRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8051): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [bestKeywordRowLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8067): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [labelFromAutoDefine](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8079): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [autoDefineCandidates](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8096): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [autoDefineLabelScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8128): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [looksLikeAliasToken](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8153): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [arabidopsisGeneSearchTerm](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8171): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [normalizeArabidopsisGeneID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8189): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [matchPhytozomeSpeciesForLemna](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8228): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [normalizedScientificName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8244): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [matchPhytozomeSpeciesForFastaHeader](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8258): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [normalizedFastaHeaderSpeciesName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8277): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [applyKeywordIdentifications](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8291): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [applyKeywordLabelMethod](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8304): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [annotateKeywordLabelSources](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8311): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [inferKeywordAutoLabelSource](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8327): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [keywordGroupsSearchEndedAt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8350): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [keywordRowLabelName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8360): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [rowKeywordLabelName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8364): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [defaultKeywordExportLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8368): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [keywordSearchTermLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8405): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [firstAlias](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8432): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [bestAlias](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8444): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [queryAliasPrimarySymbolBonus](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8464): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [looksLikePrimaryAliasSymbol](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8480): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [aliasPreferenceScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8496): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [aliasHasInternalDigitPattern](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8560): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [autoIdentifyBlastLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8580): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [fastaHeaderLabelNameFromInput](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8597): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [fastaHeaderLabelName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8601): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [firstFastaHeaderLine](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8612): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [querySourceAliasLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8631): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [fastaQuerySourceLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8644): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [bestTrustedAutoLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8651): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [isTrustedAutoLabelCandidate](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8668): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [trustedAutoLabelScore](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8672): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [lowercaseCount](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8725): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [looksLikeECNumberLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8735): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [looksLikeDatabaseIdentifierLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8739): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [blastLabelIdentityFallback](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8755): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [detectSequenceKind](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8773): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [sanitizeSequence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8796): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [normalizeBlastSequence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8814): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `1A` [exportSelections](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8822): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `1A` [exportKeywordSelections](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8831): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `1A` [exportSelectedBlastFiles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8835): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `1A` [exportSelectedKeywordFiles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8839): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `1A` [exportKeywordExcelAndFetchRecords](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8954): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process
- `0` [resolveQuerySequenceInput](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:8989): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [resolveQuerySequenceInputBatch](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9002): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [resolveURLQuerySequenceInput](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9015): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [resolveURLQuerySequenceInputBatch](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9054): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [resolveGeneReportSequence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9089): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [fetchProteinSequenceRecords](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9160): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [fetchProteinSequenceRecordsMaybeSilent](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9173): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [fetchProteinSequenceRecordsWithProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9180): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [fetchKeywordProteinSequenceRecords](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9213): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [fetchKeywordProteinSequenceRecordsWithProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9226): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [filterKeywordProteinSequenceRecords](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9261): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [keywordProteinSequenceHeader](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9286): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [buildKeywordSequenceLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9304): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [proteinSequenceCacheKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9312): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [cachedProteinSequence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9323): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [cachedProteinSequenceMiss](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9330): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [storeProteinSequence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9337): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [storeProteinSequenceMiss](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9348): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [fetchProteinSequenceCached](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9357): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2A` [prefetchBlastSequences](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9405): threaded local BLAST/setup or strong worker-communication logic stays in heavy local work
- `2B` [prefetchKeywordSequences](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9453): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [buildExportMetadata](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9496): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [prependQuerySequenceRecord](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9508): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [buildQuerySequenceLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9527): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [buildQuerySequenceHeaderID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9541): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [describeQuerySource](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9565): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [describeQuerySourceDetails](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9584): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [normalizeGeneReportURL](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9598): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `0` [inferSourceDatabaseFromURL](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9630): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [databaseDisplayName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9645): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [parseGeneReportURL](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9656): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [nonEmptyPathSegments](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9676): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [findSpeciesCandidateByJBrowseName](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9688): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [parseFastaQuerySequenceInput](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9697): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [splitFastaHeaderAndSequence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9743): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [fastaHeaderPrimaryID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9815): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [trailingParentheticalLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9834): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [parentheticalHeaderLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9855): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [stripTrailingParentheticalLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9881): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [findFirstWhitespace](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9890): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [stripTranscriptSuffix](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9899): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `0` [firstNonEmpty](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9919): workflow-local parsing, labeling, filtering, or shaping helper without threading
- `2B` [searchKeywordGroups](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9929): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [searchKeywordGroupsWithProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:9950): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [keywordSearchProgressMessage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:10023): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [searchKeywordRowsWithTimeout](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:10034): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [keywordRowsSearchType](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:10050): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `2B` [waitForBlastResultsWithProgress](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:10062): all search/species-fetch/lookup/remote BLAST orchestration is forced into class 2 network work
- `1A` [withSpinner](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast.go:10109): UI-heavy loading, table rendering, progress, and selection/export orchestration stay in main process

## `internal/workflow/blast_report.go`

- `1A` [renderBlastReportForExport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:47): UI-facing report assembly, loading summary, and table/export presentation stay in main process
- `1A` [renderBlastBatchReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:51): UI-facing report assembly, loading summary, and table/export presentation stay in main process
- `1A` [renderBlastReportWithFiles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:90): UI-facing report assembly, loading summary, and table/export presentation stay in main process
- `0` [buildBlastReportData](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:140): report summarization helper without threading
- `0` [inspectBlastGeneratedFilesList](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:201): report summarization helper without threading
- `0` [flattenBlastBatchRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:243): report summarization helper without threading
- `0` [appendUniqueStrings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:276): report summarization helper without threading
- `0` [flattenIntMatrix](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:289): report summarization helper without threading
- `0` [blastRowsByRunForReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:301): report summarization helper without threading
- `0` [flattenBlastSelectedRowsByRun](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:309): report summarization helper without threading
- `0` [aggregateBlastSequenceAudit](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:321): report summarization helper without threading
- `0` [blastExecutionReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:383): report summarization helper without threading
- `0` [blastExecutionMode](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:399): report summarization helper without threading
- `0` [blastInputTraces](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:406): report summarization helper without threading
- `0` [classifyBlastInputType](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:440): report summarization helper without threading
- `0` [blastParserPath](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:459): report summarization helper without threading
- `0` [blastInputRawPreview](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:472): report summarization helper without threading
- `0` [firstBlastInputPrepared](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:490): report summarization helper without threading
- `0` [reportBaseNameForExport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:497): report summarization helper without threading
- `0` [blastInputSource](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:507): report summarization helper without threading
- `0` [blastInputOutcome](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:514): report summarization helper without threading
- `0` [looksLikeSequence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:521): report summarization helper without threading
- `0` [blastRunReports](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:526): report summarization helper without threading
- `0` [blastSelectionStats](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:557): report summarization helper without threading
- `0` [blastExternalReferences](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:600): report summarization helper without threading
- `0` [blastRowsHaveUniProt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:611): report summarization helper without threading
- `0` [blastRowsHaveInterPro](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:620): report summarization helper without threading
- `0` [blastUniProtReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:629): report summarization helper without threading
- `0` [blastUniProtRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:652): report summarization helper without threading
- `0` [blastInterProReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:679): report summarization helper without threading
- `0` [interProSettingsReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:694): report summarization helper without threading
- `0` [blastFamilyReportBatch](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:710): report summarization helper without threading
- `0` [blastFilterReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:795): report summarization helper without threading
- `0` [blastSelectedMaskForRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:835): report summarization helper without threading
- `0` [blastQueryPrependStepDetails](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:847): report summarization helper without threading
- `0` [blastRunsSearchEndedAt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:854): report summarization helper without threading
- `0` [ResultsHashTime](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:866): report summarization helper without threading
- `0` [blastTopHit](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:870): report summarization helper without threading
- `0` [blastBestEValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:877): report summarization helper without threading
- `0` [blastBestIdentity](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:893): report summarization helper without threading
- `0` [countBlastRowsWhere](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:908): report summarization helper without threading
- `0` [countResolvedBlastItems](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:918): report summarization helper without threading
- `0` [countRunsWhere](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:928): report summarization helper without threading
- `0` [blastRowKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:938): report summarization helper without threading
- `0` [blastRowKeyCounts](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:942): report summarization helper without threading
- `0` [decrementBlastRowKey](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:953): report summarization helper without threading
- `0` [uniqueBlastValues](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:964): report summarization helper without threading
- `0` [blastInterProStatusCounts](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:983): report summarization helper without threading
- `0` [blastColumnSource](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1002): report summarization helper without threading
- `0` [blastColumnMeaning](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1017): report summarization helper without threading
- `0` [blastColumnCollection](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1048): report summarization helper without threading
- `0` [blastColumnBlankMeaning](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1065): report summarization helper without threading
- `0` [blastColumnStatsUse](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1078): report summarization helper without threading
- `0` [blastFilterHardRuleSummaries](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1091): report summarization helper without threading
- `0` [blastFilterRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1154): report summarization helper without threading
- `0` [blastFilterFamilySupportSummary](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1198): report summarization helper without threading
- `0` [blastFilterFamilySemanticSummary](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1215): report summarization helper without threading
- `0` [blastFilterQuerySummaries](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1232): report summarization helper without threading
- `0` [blastFilterTotals](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1294): report summarization helper without threading
- `0` [blastFilterRowQueryLabels](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1326): report summarization helper without threading
- `0` [blastRowCoverageForReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1340): report summarization helper without threading
- `0` [valueOrWorkflow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1350): report summarization helper without threading
- `0` [reportPercentText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1357): report summarization helper without threading
- `0` [blastFilterScoreSummary](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1364): report summarization helper without threading
- `0` [blastFilterFailureSummary](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1387): report summarization helper without threading
- `0` [blastRowHasSemanticAgreementPrompt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1417): report summarization helper without threading
- `0` [blastRowHasSemanticTokensWorkflow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1430): report summarization helper without threading
- `0` [blastRowHasSemanticReferenceSurfaceWorkflow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1434): report summarization helper without threading
- `0` [blastFilterRecommendation](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1455): report summarization helper without threading
- `0` [blastFilterSelectionLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1462): report summarization helper without threading
- `0` [blastSequenceRecordKind](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1469): report summarization helper without threading
- `0` [blastSequenceRecordLabel](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1481): report summarization helper without threading
- `0` [countQuerySourcesWithSequence](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1485): report summarization helper without threading
- `0` [anyBoolWorkflow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1495): report summarization helper without threading
- `0` [countBoolWorkflow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1504): report summarization helper without threading
- `0` [availabilityText](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1514): report summarization helper without threading
- `0` [intString](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1521): report summarization helper without threading
- `0` [formatScientificSettingWorkflow](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1528): report summarization helper without threading
- `0` [sourceDatabaseForBlastRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1535): report summarization helper without threading
- `0` [minInt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1544): report summarization helper without threading
- `0` [maxInt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1551): report summarization helper without threading
- `0` [countNonEmptyStrings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1558): report summarization helper without threading
- `0` [blastProvenance](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1568): report summarization helper without threading
- `0` [blastProvenanceExplanation](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1596): report summarization helper without threading
- `0` [blastColumnCompleteness](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1613): report summarization helper without threading
- `0` [blastQualityChecks](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1634): report summarization helper without threading
- `0` [blastColumnLineage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1664): report summarization helper without threading
- `0` [blastExportSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1687): report summarization helper without threading
- `0` [blastReportHeaders](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1701): report summarization helper without threading
- `0` [blastRowsProgramForReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1705): report summarization helper without threading
- `0` [blastRunsUseUniProt](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1714): report summarization helper without threading
- `0` [blastRunsUseInterPro](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1725): report summarization helper without threading
- `0` [blastReportCellValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1736): report summarization helper without threading
- `0` [buildBlastSequenceAudit](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/blast_report.go:1839): report summarization helper without threading

## `internal/workflow/io_workers.go`

- `2A` [init](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/io_workers.go:30): worker registration belongs with the heavy communicating pipeline
- `2A` [readTextFileProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/io_workers.go:53): I/O workers are heavy local helper bridges
- `2A` [installManagedBlastProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/io_workers.go:62): I/O workers are heavy local helper bridges

## `internal/workflow/keyword_report.go`

- `1A` [renderKeywordReportForExport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:22): UI-facing report assembly, loading summary, and table/export presentation stay in main process
- `0` [buildKeywordReportData](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:59): report summarization helper without threading
- `0` [inspectKeywordGeneratedFiles](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:118): report summarization helper without threading
- `0` [keywordTextFileRole](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:153): report summarization helper without threading
- `0` [keywordReportStep](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:161): report summarization helper without threading
- `0` [reportSoftwareInfo](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:172): report summarization helper without threading
- `1A` [reportUserSession](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:182): UI-facing report assembly, loading summary, and table/export presentation stay in main process
- `0` [reportSpecies](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:224): report summarization helper without threading
- `0` [reportProteomeID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:238): report summarization helper without threading
- `0` [reportBoolApplicability](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:245): report summarization helper without threading
- `0` [speciesSourceNotes](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:252): report summarization helper without threading
- `0` [inferKeywordSpeciesFromRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:263): report summarization helper without threading
- `0` [keywordTermReports](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:272): report summarization helper without threading
- `0` [keywordSelectedCountsByTerm](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:292): report summarization helper without threading
- `0` [classifyKeywordInputType](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:300): report summarization helper without threading
- `0` [looksLikeTranscriptID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:323): report summarization helper without threading
- `0` [looksLikeProteinOrGeneID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:327): report summarization helper without threading
- `0` [keywordMatchingNotes](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:347): report summarization helper without threading
- `0` [keywordRowsSourceDatabase](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:364): report summarization helper without threading
- `0` [keywordLabelReports](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:373): report summarization helper without threading
- `0` [labelSourceForGroup](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:390): report summarization helper without threading
- `0` [labelTraceExplanation](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:415): report summarization helper without threading
- `0` [keywordSelectionStats](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:425): report summarization helper without threading
- `0` [keywordProvenance](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:448): report summarization helper without threading
- `0` [keywordReportSourceValues](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:489): report summarization helper without threading
- `0` [provenanceExplanation](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:506): report summarization helper without threading
- `0` [keywordQualityChecks](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:521): report summarization helper without threading
- `0` [keywordColumnCompleteness](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:553): report summarization helper without threading
- `0` [keywordReportCellValue](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:576): report summarization helper without threading
- `0` [qualityCheck](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:614): report summarization helper without threading
- `0` [countKeywordRowsWhere](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:622): report summarization helper without threading
- `0` [duplicateKeywordSequenceIDs](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:632): report summarization helper without threading
- `0` [keywordColumnLineage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:649): report summarization helper without threading
- `0` [keywordColumnEnglishDetail](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:658): report summarization helper without threading
- `0` [keywordReportHeaders](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:662): report summarization helper without threading
- `0` [sourceDatabaseForKeywordRows](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:666): report summarization helper without threading
- `0` [keywordRowsHaveProteinIDForReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:675): report summarization helper without threading
- `0` [keywordReportExtraHeaders](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:684): report summarization helper without threading
- `0` [keywordColumnLineageForHeader](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:701): report summarization helper without threading
- `0` [dynamicKeywordColumnLineage](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:740): report summarization helper without threading
- `0` [dynamicColumnStatsUse](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:773): report summarization helper without threading
- `0` [keywordExportSettings](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:781): report summarization helper without threading
- `0` [buildKeywordSequenceAudit](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:792): report summarization helper without threading
- `0` [keywordSequenceHeaderLabelMode](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:884): report summarization helper without threading
- `0` [keywordGroupLabelMode](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:934): report summarization helper without threading
- `0` [firstEnvForReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:943): report summarization helper without threading
- `0` [envPairForReport](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:952): report summarization helper without threading
- `0` [nonEmptyReportValues](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/keyword_report.go:960): report summarization helper without threading

## `internal/workflow/label_workers.go`

- `2A` [init](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/label_workers.go:23): worker registration belongs with the heavy communicating pipeline
- `2B` [autoIdentifyBlastLabelProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/label_workers.go:38): auto label lookup is effectively search work and must be class 2 network work

## `internal/workflow/lemna_workers.go`

- `2A` [init](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/lemna_workers.go:23): worker registration belongs with the heavy communicating pipeline
- `2B` [detectLemnaBlastCapabilitiesProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/lemna_workers.go:41): Lemna capability detection is a remote search/load task, so it stays in class 2 network work

## `internal/workflow/reference_workers.go`

- `2A` [init](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/reference_workers.go:71): reference worker registration belongs with the heavy communicating pipeline
- `2A` [workerWizardForDatabase](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/reference_workers.go:146): reference worker helper stays beside the heavy lookup pipeline
- `2B` [lookupUniProtEntryProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/reference_workers.go:174): reference lookup/search is class 2 network work
- `2B` [lookupInterProEntryProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/reference_workers.go:190): reference lookup/search is class 2 network work
- `2B` [lookupUniProtEntriesProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/reference_workers.go:209): reference lookup/search is class 2 network work
- `2B` [lookupInterProEntriesProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/reference_workers.go:225): reference lookup/search is class 2 network work

## `internal/workflow/source_workers.go`

- `2A` [init](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:138): worker registration belongs with the communicating heavy pipeline
- `0` [workerSourceForDatabase](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:338): source worker helper only
- `0` [sourceProcessDatabase](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:356): source worker helper only
- `0` [sourceProcessDomain](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:367): source worker helper only
- `2A` [sourceWorkerTaskSpec](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:377): worker bridge task-spec builder belongs to the communicating heavy pipeline
- `2A` [sourceLocalTaskSpec](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:385): worker bridge task-spec builder belongs to the communicating heavy pipeline
- `2A` [sourceSubmitBlastTaskSpec](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:393): worker bridge task-spec builder belongs to the communicating heavy pipeline
- `2A` [sourceWaitBlastTaskSpec](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:401): worker bridge task-spec builder belongs to the communicating heavy pipeline
- `0` [isLocalBlastJobID](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:408): source worker helper only
- `2B` [fetchSpeciesCandidatesProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:413): source worker bridges remote/search tasks into the heavy side
- `2B` [submitBlastProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:425): source worker now bridges both remote and local source submission into the heavy side
- `2A` [runBlastProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:440): threaded worker-bound orchestration stays in heavy process
- `2A` [prepareLocalBlastProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:456): local BLAST preparation bridge belongs in heavy local work
- `2A` [waitBlastResultsProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:467): result wait bridging chooses heavy local or heavy network resources and stays in heavy process
- `2B` [fetchProteinSequenceProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:482): source worker bridges remote/search tasks into the heavy side
- `2B` [fetchUniProtAccessionsProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:496): source worker bridges remote/search tasks into the heavy side
- `2B` [fetchUniProtAccessionsForSourceProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:500): source worker bridges remote/search tasks into the heavy side
- `2B` [fetchUniProtAccessionsForDatabaseProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:504): source worker bridges remote/search tasks into the heavy side
- `2B` [fetchGeneQuerySequenceProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:516): source worker bridges remote/search tasks into the heavy side
- `2B` [fetchProteinQuerySequenceProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:531): source worker bridges remote/search tasks into the heavy side
- `2B` [searchKeywordRowsProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:545): source worker bridges remote/search tasks into the heavy side
- `2B` [searchKeywordRowsForSourceProcess](/C:/Users/wangsychn/Documents/GitHub/phytozome-go/internal/workflow/source_workers.go:549): source worker bridges remote/search tasks into the heavy side


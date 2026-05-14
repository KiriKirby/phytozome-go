module github.com/KiriKirby/phytozome-go

go 1.26.3

require (
	github.com/gdamore/tcell/v2 v2.7.4
	github.com/phpdave11/gofpdf v1.4.3
	github.com/rivo/tview v0.0.0-20241030223020-e34b54cd4c27
	github.com/xuri/excelize/v2 v2.10.1
	golang.org/x/sync v0.20.0
	golang.org/x/sys v0.43.0
)

replace github.com/gdamore/tcell/v2 => github.com/kivattt/tcell-naively-faster/v2 v2.0.1

replace github.com/rivo/tview => github.com/kivattt/tview v1.0.6

require (
	github.com/gdamore/encoding v1.0.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/richardlehane/mscfb v1.0.6 // indirect
	github.com/richardlehane/msoleps v1.0.6 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/tiendc/go-deepcopy v1.7.2 // indirect
	github.com/xuri/efp v0.0.1 // indirect
	github.com/xuri/nfp v0.0.2-0.20250530014748-2ddeb826f9a9 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/term v0.42.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)

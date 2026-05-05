//go:build !windows

package tui

import "github.com/rivo/tview"

func installConsoleCloseHandler(app *tview.Application) func() {
	return func() {}
}

func installConsoleResizeWatcher(app *tview.Application) func() {
	return func() {}
}

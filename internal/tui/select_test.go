package tui

import (
	"sync/atomic"
	"testing"

	"github.com/rivo/tview"
)

func TestInstallDeferredAppHookInstallsOnFirstAfterDrawAndRestores(t *testing.T) {
	app := tview.NewApplication()

	var installCount atomic.Int32
	var restoreCount atomic.Int32
	restore := installDeferredAppHook(app, func(app *tview.Application) func() {
		installCount.Add(1)
		return func() {
			restoreCount.Add(1)
		}
	})
	afterDraw := app.GetAfterDrawFunc()
	if afterDraw == nil {
		t.Fatal("after draw hook was not installed")
	}

	afterDraw(nil)
	restore()

	if got := installCount.Load(); got != 1 {
		t.Fatalf("install count = %d, want 1", got)
	}
	if got := restoreCount.Load(); got != 1 {
		t.Fatalf("restore count = %d, want 1", got)
	}
}

func TestInstallDeferredAppHookRestoreBeforeRunIsNoop(t *testing.T) {
	app := tview.NewApplication()
	var installCount atomic.Int32
	restore := installDeferredAppHook(app, func(app *tview.Application) func() {
		installCount.Add(1)
		return func() {}
	})

	restore()

	if got := installCount.Load(); got != 0 {
		t.Fatalf("install count = %d, want 0", got)
	}
}

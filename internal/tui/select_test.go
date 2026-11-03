package tui

import (
	"sync/atomic"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestInstallInputCaptureMapsBundledCtrlRunTransport(t *testing.T) {
	app := tview.NewApplication()
	var got *tcell.EventKey
	installInputCapture(app, func(event *tcell.EventKey) *tcell.EventKey { got = event; return nil })
	app.GetInputCapture()(tcell.NewEventKey(tcell.KeyF13, 0, tcell.ModNone))
	if got == nil || got.Key() != tcell.KeyCtrlR {
		t.Fatalf("F13 transport mapped to %#v, want Ctrl+R", got)
	}
	got = nil
	app.GetInputCapture()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModCtrl))
	if got != nil {
		t.Fatalf("Ctrl+Enter reached application handler as %#v", got)
	}
}

func TestHintLineIsSingleLineAndElided(t *testing.T) {
	if got := elideHintLine("one two\nthree", 20); got != "one two\nthree" {
		t.Fatalf("unexpected direct elision result %q", got)
	}
	if got := elideHintLine("123456789", 6); got != "123..." {
		t.Fatalf("elided hint = %q, want 123...", got)
	}
	view := hintView("one\n two")
	if view == nil {
		t.Fatal("hint view is nil")
	}
}

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

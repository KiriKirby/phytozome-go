//go:build windows

package tui

import (
	"sync/atomic"
	"testing"

	"github.com/rivo/tview"
)

func TestHandleConsoleCloseEventQueuesSingleStopRequest(t *testing.T) {
	app := tview.NewApplication()

	consoleCloseHandlerMu.Lock()
	previousApp := consoleCloseHandlerApp
	consoleCloseHandlerApp = app
	consoleCloseHandlerMu.Unlock()
	previousRequester := consoleCloseHandlerRequestStop
	consoleCloseHandlerStopQueued.Store(false)
	defer func() {
		consoleCloseHandlerMu.Lock()
		consoleCloseHandlerApp = previousApp
		consoleCloseHandlerMu.Unlock()
		consoleCloseHandlerRequestStop = previousRequester
		consoleCloseHandlerStopQueued.Store(false)
	}()

	var requests atomic.Int32
	consoleCloseHandlerRequestStop = func(app *tview.Application) {
		requests.Add(1)
	}

	if got := handleConsoleCloseEvent(ctrlCloseEvent); got != 1 {
		t.Fatalf("close event result = %d, want 1", got)
	}
	if got := handleConsoleCloseEvent(ctrlShutdownEvent); got != 1 {
		t.Fatalf("shutdown event result = %d, want 1", got)
	}
	if count := requests.Load(); count != 1 {
		t.Fatalf("stop requests = %d, want 1", count)
	}
}

func TestHandleConsoleCloseEventReturnsZeroWithoutApp(t *testing.T) {
	consoleCloseHandlerMu.Lock()
	previousApp := consoleCloseHandlerApp
	consoleCloseHandlerApp = nil
	consoleCloseHandlerMu.Unlock()
	consoleCloseHandlerStopQueued.Store(false)
	defer func() {
		consoleCloseHandlerMu.Lock()
		consoleCloseHandlerApp = previousApp
		consoleCloseHandlerMu.Unlock()
		consoleCloseHandlerStopQueued.Store(false)
	}()

	if got := handleConsoleCloseEvent(ctrlCloseEvent); got != 0 {
		t.Fatalf("close event result = %d, want 0", got)
	}
	if got := handleConsoleCloseEvent(999); got != 0 {
		t.Fatalf("unknown event result = %d, want 0", got)
	}
}

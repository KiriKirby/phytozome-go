//go:build windows

package tui

import (
	"sync/atomic"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/rivo/tview"
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleCtrlHandler   = kernel32.NewProc("SetConsoleCtrlHandler")
	procGetStdHandle            = kernel32.NewProc("GetStdHandle")
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
	consoleCloseHandlerMu       sync.Mutex
	consoleCloseHandlerRefCount int
	consoleCloseHandlerApp      *tview.Application
	consoleCloseHandlerCallback = syscall.NewCallback(func(ctrlType uintptr) uintptr {
		switch ctrlType {
		case ctrlCEvent, ctrlBreakEvent, ctrlCloseEvent, ctrlLogoffEvent, ctrlShutdownEvent:
			consoleCloseHandlerMu.Lock()
			app := consoleCloseHandlerApp
			consoleCloseHandlerMu.Unlock()
			if app != nil {
				app.Stop()
				return 1
			}
		}
		return 0
	})
)

const (
	ctrlCEvent        = 0
	ctrlBreakEvent    = 1
	ctrlCloseEvent    = 2
	ctrlLogoffEvent   = 5
	ctrlShutdownEvent = 6
	stdOutputHandle   = ^uintptr(10) + 1
)

type coord struct {
	X int16
	Y int16
}

type smallRect struct {
	Left   int16
	Top    int16
	Right  int16
	Bottom int16
}

type consoleScreenBufferInfo struct {
	Size              coord
	CursorPosition    coord
	Attributes        uint16
	Window            smallRect
	MaximumWindowSize coord
}

func installConsoleCloseHandler(app *tview.Application) func() {
	consoleCloseHandlerMu.Lock()
	consoleCloseHandlerApp = app
	if consoleCloseHandlerRefCount == 0 {
		_, _, _ = procSetConsoleCtrlHandler.Call(consoleCloseHandlerCallback, 1)
	}
	consoleCloseHandlerRefCount++
	consoleCloseHandlerMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			consoleCloseHandlerMu.Lock()
			if consoleCloseHandlerRefCount > 0 {
				consoleCloseHandlerRefCount--
			}
			if consoleCloseHandlerRefCount == 0 {
				consoleCloseHandlerApp = nil
				_, _, _ = procSetConsoleCtrlHandler.Call(consoleCloseHandlerCallback, 0)
			}
			consoleCloseHandlerMu.Unlock()
		})
	}
}

func installConsoleResizeWatcher(app *tview.Application) func() {
	handle, _, _ := procGetStdHandle.Call(stdOutputHandle)
	if handle == 0 || handle == ^uintptr(0) {
		return func() {}
	}
	initialWidth, initialHeight, ok := currentConsoleViewportSize(handle)
	if !ok {
		return func() {}
	}
	var running atomic.Bool
	running.Store(true)
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		lastWidth := initialWidth
		lastHeight := initialHeight
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				width, height, ok := currentConsoleViewportSize(handle)
				if !ok {
					continue
				}
				if width == lastWidth && height == lastHeight {
					continue
				}
				lastWidth = width
				lastHeight = height
				if !running.Load() {
					return
				}
				app.QueueUpdateDraw(func() {})
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			running.Store(false)
			close(done)
		})
	}
}

func currentConsoleViewportSize(handle uintptr) (int, int, bool) {
	var info consoleScreenBufferInfo
	ret, _, _ := procGetConsoleScreenBufferInfo.Call(handle, uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		return 0, 0, false
	}
	width := int(info.Window.Right-info.Window.Left) + 1
	height := int(info.Window.Bottom-info.Window.Top) + 1
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

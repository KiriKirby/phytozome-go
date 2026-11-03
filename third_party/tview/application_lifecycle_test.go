package tview

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func TestQueueUpdateContextReturnsBeforeRun(t *testing.T) {
	app := NewApplication()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if app.QueueUpdateContext(ctx, func() { t.Fatal("update ran without an event loop") }) {
		t.Fatal("queue update unexpectedly completed before Run")
	}
}

func TestStopReleasesSaturatedQueues(t *testing.T) {
	app := NewApplication()
	var wait sync.WaitGroup
	for index := 0; index < queueSize*4; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			app.QueueEvent(tcell.NewEventInterrupt(nil))
			app.QueueUpdate(func() {})
		}()
	}
	app.Stop()
	done := make(chan struct{})
	go func() { wait.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("producers remained blocked after Stop")
	}
}

func runTestApplication(t *testing.T, root Primitive) (*Application, tcell.SimulationScreen, <-chan error) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	app := NewApplication().SetScreen(screen).SetRoot(root, true).EnableMouse(true)
	done := make(chan error, 1)
	go func() { done <- app.Run() }()
	return app, screen, done
}

func TestQueuedProducersAreReleasedAfterStop(t *testing.T) {
	app, _, runDone := runTestApplication(t, NewBox())
	app.QueueUpdate(func() { app.Stop() })
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after Stop")
	}

	var wait sync.WaitGroup
	for index := 0; index < queueSize*3; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			app.QueueUpdate(func() { t.Error("update ran after stop") })
			app.QueueEvent(tcell.NewEventInterrupt(nil))
		}()
	}
	producersDone := make(chan struct{})
	go func() { wait.Wait(); close(producersDone) }()
	select {
	case <-producersDone:
	case <-time.After(time.Second):
		t.Fatal("queue producers remained blocked after stop")
	}
}

func TestPointerOnlyMotionDoesNotLoseWheelOrClicks(t *testing.T) {
	var scrolls atomic.Int32
	var clicks atomic.Int32
	processed := make(chan struct{})
	box := NewBox()
	box.SetMouseCapture(func(action MouseAction, event *tcell.EventMouse) (MouseAction, *tcell.EventMouse) {
		switch action {
		case MouseScrollDown:
			scrolls.Add(1)
		case MouseLeftClick:
			clicks.Add(1)
		}
		if action == MouseLeftClick {
			select {
			case <-processed:
			default:
				close(processed)
			}
		}
		return MouseConsumed, nil
	})
	app, screen, runDone := runTestApplication(t, box)
	screen.PostEventWait(tcell.NewEventMouse(3, 2, tcell.ButtonNone, tcell.ModNone))
	screen.PostEventWait(tcell.NewEventMouse(4, 3, tcell.WheelDown, tcell.ModNone))
	screen.PostEventWait(tcell.NewEventMouse(4, 3, tcell.Button1, tcell.ModNone))
	screen.PostEventWait(tcell.NewEventMouse(4, 3, tcell.ButtonNone, tcell.ModNone))
	select {
	case <-processed:
	case <-time.After(time.Second):
		app.Stop()
		t.Fatal("mouse events were not processed before stop")
	}
	app.Stop()
	<-runDone
	if scrolls.Load() != 1 {
		t.Fatalf("scroll count = %d, want 1", scrolls.Load())
	}
	if clicks.Load() != 1 {
		t.Fatalf("click count = %d, want 1", clicks.Load())
	}
}

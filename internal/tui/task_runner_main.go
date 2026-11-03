package tui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func runTaskValueInMainProcess[T any](page TaskPage, task func(ctx context.Context, update func(string)) (T, error)) (T, error) {
	return runTaskModal(page, func(ctx context.Context, update func(taskProgressSlot)) (T, error) {
		return task(ctx, func(message string) {
			update(taskProgressSlot{Status: message, Active: true})
		})
	})
}

func runProgressTaskValueInMainProcess[T any](page TaskPage, task func(ctx context.Context, update func(current int, message string)) (T, error)) (T, error) {
	return runTaskModal(page, func(ctx context.Context, update func(taskProgressSlot)) (T, error) {
		return task(ctx, func(current int, message string) {
			update(taskProgressSlot{Current: current, Status: message, Active: true})
		})
	})
}

func runTaskModal[T any](page TaskPage, task func(ctx context.Context, update func(taskProgressSlot)) (T, error)) (T, error) {
	var zero T

	app := newApp()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	statusView := textBlock("")
	statusView.SetDynamicColors(true)
	statusView.SetWrap(true)
	statusView.SetBorder(true)
	statusView.SetTitle(" Progress ")
	statusView.SetTitleAlign(tview.AlignCenter)

	body := newButtonFlex()
	body.SetBorder(true)
	body.SetTitle(" " + trimColon(taskTitle(page)) + " ")
	body.SetTitleAlign(tview.AlignCenter)
	setFocusBorder(body.Box, true)
	attachFocusBorder(body.Box)
	if description := strings.TrimSpace(page.Description); description != "" {
		body.AddItem(textBlock(description), 2, 0, false)
	}
	body.AddItem(statusView, 0, 1, true)
	addHints(body, []string{"Esc cancels"})

	root, overlay := newTaskModalRoot(page, body, 118, 18)
	setPageRoot(app, root)
	app.SetFocus(body)

	parentSlot := taskProgressSlot{
		Title:   taskSubtitle(page),
		Status:  initialTaskStatus(page),
		Current: 0,
		Total:   normalizedTaskTotal(0, page.Total),
		Active:  true,
	}
	parentID, updateParent, removeParent := sharedTaskOverlay.register(parentSlot)
	defer removeParent()

	var (
		mu              sync.Mutex
		result          T
		taskErr         error
		cancelRequested bool
		currentSlot     = parentSlot
	)

	refresh := func() {
		mu.Lock()
		slot := currentSlot
		mu.Unlock()
		slots := sharedTaskOverlay.snapshot(5, parentID, slot)
		statusView.SetText(renderTaskProgressSlots("|", slots))
		overlay.SetSize(118, taskModalHeightForSlots(page, slots))
	}

	updateCurrent := func(next taskProgressSlot) {
		mu.Lock()
		if strings.TrimSpace(next.Title) == "" {
			next.Title = currentSlot.Title
		}
		next.Current, next.Total = normalizeTaskProgress(next.Current, chooseTaskTotal(next.Total, currentSlot.Total, page.Total))
		if strings.TrimSpace(next.Status) == "" {
			next.Status = currentSlot.Status
		}
		next.Active = next.Active || currentSlot.Active
		currentSlot = next
		updateParent(currentSlot)
		mu.Unlock()
	}

	requestCancel := func() {
		mu.Lock()
		cancelRequested = true
		currentSlot.Status = "Cancelling..."
		updateParent(currentSlot)
		mu.Unlock()
		cancel()
		// Input callbacks already run on the UI thread. Never synchronously
		// queue back onto that same thread.
		refresh()
	}

	addButtonRow(body, buttonRow(buttonSpec{
		Label:    ButtonCancel,
		Shortcut: ShortcutBack,
		Action:   requestCancel,
		Visible:  true,
	}))

	refresh()

	taskCtx := context.WithValue(ctx, taskChildSlotKey{}, TaskChildSlotRegistrar(func(slot taskProgressSlot) (func(taskProgressSlot), func()) {
		_, update, remove := sharedTaskOverlay.register(normalizedTaskProgressSlot(slot))
		return func(next taskProgressSlot) {
				update(normalizedTaskProgressSlot(next))
			}, func() {
				remove()
			}
	}))

	ready := make(chan struct{})
	var readyOnce sync.Once
	afterDraw := app.GetAfterDrawFunc()
	app.SetAfterDrawFunc(func(screen tcell.Screen) {
		if afterDraw != nil {
			afterDraw(screen)
		}
		readyOnce.Do(func() { close(ready) })
	})
	// One refresh worker coalesces progress from every producer. Task update
	// frequency and child count cannot grow the UI queue or goroutine count.
	refreshDone := make(chan struct{})
	go func() {
		defer close(refreshDone)
		if !waitForTaskStart(ctx, ready) {
			return
		}
		ticker := time.NewTicker(uiAnimationDelay())
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-app.Done():
				return
			case <-ticker.C:
				if !app.QueueUpdateDrawContext(ctx, refresh) {
					return
				}
			}
		}
	}()
	done := make(chan struct{})
	go func() {
		defer close(done)
		var out T
		var err error
		if waitForTaskStart(ctx, ready) {
			out, err = task(taskCtx, updateCurrent)
		} else {
			err = ctx.Err()
		}
		mu.Lock()
		result = out
		if cancelRequested && (err == nil || errors.Is(err, context.Canceled)) {
			err = taskCancelError(page)
		}
		taskErr = err
		if taskErr == nil {
			currentSlot.Active = false
			currentSlot.Current, currentSlot.Total = normalizeTaskProgress(currentSlot.Total, currentSlot.Total)
			if strings.TrimSpace(currentSlot.Status) == "" || strings.EqualFold(strings.TrimSpace(currentSlot.Status), strings.TrimSpace(initialTaskStatus(page))) {
				currentSlot.Status = "Completed."
			}
		} else if cancelRequested && (errors.Is(taskErr, taskCancelError(page)) || errors.Is(taskErr, context.Canceled)) {
			currentSlot.Active = false
			currentSlot.Status = "Cancelled."
		} else {
			currentSlot.Active = false
			if strings.TrimSpace(currentSlot.Status) == "" {
				currentSlot.Status = "Failed."
			}
		}
		updateParent(currentSlot)
		mu.Unlock()
		app.QueueUpdateDraw(func() {
			refresh()
			app.Stop()
		})
	}()

	installInputCapture(app, func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			requestCancel()
			return nil
		}
		return event
	})

	runErr := runApp(app)
	cancel()
	// Also release queues when an initialization error/test runner never
	// entered Application.Run. Join both workers before returning.
	app.Stop()
	<-done
	<-refreshDone
	if runErr != nil {
		return zero, runErr
	}

	mu.Lock()
	defer mu.Unlock()
	if cancelRequested && taskErr == nil {
		taskErr = taskCancelError(page)
	}
	return result, taskErr
}

func normalizedTaskProgressSlot(slot taskProgressSlot) taskProgressSlot {
	slot.Current, slot.Total = normalizeTaskProgress(slot.Current, slot.Total)
	if strings.TrimSpace(slot.Status) == "" {
		slot.Status = "Working..."
	}
	return slot
}

func chooseTaskTotal(next int, current int, page int) int {
	switch {
	case next > 0:
		return next
	case current > 0:
		return current
	default:
		return page
	}
}

func normalizedTaskTotal(current int, total int) int {
	_, normalized := normalizeTaskProgress(current, total)
	return normalized
}

func normalizeTaskProgress(current int, total int) (int, int) {
	if current < 0 {
		current = 0
	}
	if total < 0 {
		total = 0
	}
	if total > 0 && current > total {
		total = current
	}
	return current, total
}

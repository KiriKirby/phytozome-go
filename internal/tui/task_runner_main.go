package tui

import (
	"context"
	"errors"
	"strings"
	"sync"

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
	addHints(body, []string{"Esc requests cancellation. Concurrent work is shown in up to 5 live progress rows."})

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
		slots := sharedTaskOverlay.snapshot(5, parentID, currentSlot)
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
		app.QueueUpdateDraw(refresh)
	}

	refresh()

	taskCtx := context.WithValue(ctx, taskChildSlotKey{}, TaskChildSlotRegistrar(func(slot taskProgressSlot) (func(taskProgressSlot), func()) {
		_, update, remove := sharedTaskOverlay.register(normalizedTaskProgressSlot(slot))
		return func(next taskProgressSlot) {
			update(normalizedTaskProgressSlot(next))
			app.QueueUpdateDraw(refresh)
		}, func() {
			remove()
			app.QueueUpdateDraw(refresh)
		}
	}))

	done := make(chan struct{})
	go func() {
		defer close(done)
		out, err := task(taskCtx, updateCurrent)
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
			mu.Lock()
			cancelRequested = true
			mu.Unlock()
			cancel()
			updateCurrent(taskProgressSlot{
				Status:  "Cancelling...",
				Current: currentSlot.Current,
				Total:   currentSlot.Total,
				Active:  true,
			})
			return nil
		}
		return event
	})

	if err := runApp(app); err != nil {
		return zero, err
	}
	<-done

	mu.Lock()
	defer mu.Unlock()
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

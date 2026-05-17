package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type taskChildSlotKey struct{}

type TaskChildSlotRegistrar func(slot taskProgressSlot) (func(taskProgressSlot), func())

type taskProgressSlot struct {
	Title   string
	Status  string
	Current int
	Total   int
	Active  bool
}

type TaskProgressSlot = taskProgressSlot

type taskOverlaySlotState struct {
	mu             sync.Mutex
	title          string
	status         string
	current        int
	total          int
	active         bool
	order          int64
	completedOrder int64
}

type taskOverlayManager struct {
	mu      sync.Mutex
	nextID  int64
	nextSeq int64
	slots   map[int64]*taskOverlaySlotState
}

func initialTaskStatus(page TaskPage) string {
	status := strings.TrimSpace(page.Initial)
	if status == "" {
		status = strings.TrimSpace(page.Description)
	}
	if status == "" {
		status = strings.TrimSpace(page.Title)
	}
	if status == "" {
		status = "Working..."
	}
	return status
}

func taskTitle(page TaskPage) string {
	title := strings.TrimSpace(page.Title)
	if title == "" {
		title = strings.TrimSpace(page.Initial)
	}
	if title == "" {
		title = "Task"
	}
	return title
}

func taskSubtitle(page TaskPage) string {
	subtitle := strings.TrimSpace(page.Title)
	if subtitle == "" {
		subtitle = strings.TrimSpace(page.Description)
	}
	if subtitle == "" {
		subtitle = "Background activity"
	}
	return subtitle
}

func taskProgressRender(current int, total int, width int) string {
	current, total = normalizeTaskProgress(current, total)
	if total <= 0 {
		return ""
	}
	if width < 8 {
		width = 8
	}
	ratio := float64(current) / float64(total)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return fmt.Sprintf("[deepskyblue]%s[white]%s  %d/%d (%3.0f%%)",
		strings.Repeat("#", filled),
		strings.Repeat("-", width-filled),
		current,
		total,
		ratio*100,
	)
}

func taskProgressSlotHeight(slot taskProgressSlot) int {
	height := 3
	if strings.TrimSpace(slot.Title) != "" {
		height++
	}
	if slot.Total > 0 {
		height++
	}
	return height
}

func taskModalHeightForSlots(page TaskPage, slots []taskProgressSlot) int {
	rows := 4
	if strings.TrimSpace(page.Description) != "" {
		rows += 2
	}
	if len(slots) == 0 {
		rows += 4
	}
	for _, slot := range slots {
		rows += taskProgressSlotHeight(slot)
	}
	rows += 3
	return modalHeightForContent(rows, 16, 34)
}

func clampTaskProgressSlots(slots []taskProgressSlot, limit int) []taskProgressSlot {
	if limit <= 0 || len(slots) <= limit {
		return append([]taskProgressSlot(nil), slots...)
	}
	out := make([]taskProgressSlot, 0, limit)
	active := make([]taskProgressSlot, 0, limit)
	idle := make([]taskProgressSlot, 0, len(slots))
	for _, slot := range slots {
		if slot.Active {
			active = append(active, slot)
		} else {
			idle = append(idle, slot)
		}
	}
	out = append(out, active...)
	if len(out) > limit {
		out = out[:limit]
	}
	for _, slot := range idle {
		if len(out) >= limit {
			break
		}
		out = append(out, slot)
	}
	return out
}

var sharedTaskOverlay taskOverlayManager

func (m *taskOverlayManager) register(slot taskProgressSlot) (int64, func(taskProgressSlot), func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.slots == nil {
		m.slots = map[int64]*taskOverlaySlotState{}
	}
	m.nextID++
	m.nextSeq++
	id := m.nextID
	m.slots[id] = &taskOverlaySlotState{
		title:   slot.Title,
		status:  slot.Status,
		current: slot.Current,
		total:   slot.Total,
		active:  slot.Active,
		order:   m.nextSeq,
	}
	update := func(next taskProgressSlot) {
		m.update(id, next)
	}
	remove := func() {
		m.remove(id)
	}
	return id, update, remove
}

func (m *taskOverlayManager) update(id int64, slot taskProgressSlot) {
	m.mu.Lock()
	state := m.slots[id]
	if state == nil {
		m.mu.Unlock()
		return
	}
	m.nextSeq++
	seq := m.nextSeq
	m.mu.Unlock()

	state.mu.Lock()
	state.title = slot.Title
	state.status = slot.Status
	state.current = slot.Current
	state.total = slot.Total
	state.active = slot.Active
	if slot.Active {
		state.order = seq
	} else {
		state.completedOrder = seq
	}
	state.mu.Unlock()
}

func (m *taskOverlayManager) remove(id int64) {
	m.mu.Lock()
	delete(m.slots, id)
	m.mu.Unlock()
}

func (m *taskOverlayManager) snapshot(limit int, currentID int64, fallback taskProgressSlot) []taskProgressSlot {
	type snapshotSlot struct {
		id             int64
		slot           taskProgressSlot
		order          int64
		completedOrder int64
	}

	m.mu.Lock()
	states := make([]snapshotSlot, 0, len(m.slots)+1)
	for id, state := range m.slots {
		if state == nil {
			continue
		}
		state.mu.Lock()
		copySlot := snapshotSlot{
			id: id,
			slot: taskProgressSlot{
				Title:   state.title,
				Status:  state.status,
				Current: state.current,
				Total:   state.total,
				Active:  state.active,
			},
			order:          state.order,
			completedOrder: state.completedOrder,
		}
		state.mu.Unlock()
		states = append(states, copySlot)
	}
	m.mu.Unlock()

	foundCurrent := false
	for _, state := range states {
		if state.id == currentID {
			foundCurrent = true
			break
		}
	}
	if !foundCurrent {
		states = append(states, snapshotSlot{id: currentID, slot: fallback, order: 1 << 60})
	}

	sort.SliceStable(states, func(i, j int) bool {
		left := states[i]
		right := states[j]
		if left.id == currentID && right.id != currentID {
			return true
		}
		if right.id == currentID && left.id != currentID {
			return false
		}
		if left.slot.Active != right.slot.Active {
			return left.slot.Active
		}
		if left.slot.Active {
			if left.order != right.order {
				return left.order > right.order
			}
		} else if left.completedOrder != right.completedOrder {
			return left.completedOrder > right.completedOrder
		}
		return left.order > right.order
	})

	out := make([]taskProgressSlot, 0, len(states))
	for _, state := range states {
		out = append(out, state.slot)
	}
	return clampTaskProgressSlots(out, limit)
}

func renderTaskProgressSlots(frame string, slots []taskProgressSlot) string {
	lines := make([]string, 0, len(slots)*5)
	for _, item := range slots {
		title := strings.TrimSpace(item.Title)
		status := strings.TrimSpace(item.Status)
		if title != "" {
			titleLine := fmt.Sprintf("[yellow]%s[white] [::b]%s[::-]", frame, title)
			if !item.Active {
				titleLine = fmt.Sprintf("[gray]%s[white] %s", frame, title)
			}
			lines = append(lines, titleLine)
		}
		if status == "" {
			status = "Working..."
		}
		lines = append(lines, status)
		if bar := taskProgressRender(item.Current, item.Total, 28); bar != "" {
			lines = append(lines, bar)
		}
		lines = append(lines, "")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func taskChildRegistrarFromContext(ctx context.Context) TaskChildSlotRegistrar {
	if ctx == nil {
		return nil
	}
	registrar, _ := ctx.Value(taskChildSlotKey{}).(TaskChildSlotRegistrar)
	return registrar
}

func RegisterTaskChildSlot(ctx context.Context, slot taskProgressSlot) (func(taskProgressSlot), func(), bool) {
	registrar := taskChildRegistrarFromContext(ctx)
	if registrar == nil {
		return nil, nil, false
	}
	update, remove := registrar(slot)
	return update, remove, true
}

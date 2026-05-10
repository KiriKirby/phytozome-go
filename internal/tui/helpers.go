package tui

import (
	"context"
	"errors"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
)

func pageBreadcrumb(breadcrumb string, path []string) string {
	if strings.TrimSpace(breadcrumb) != "" {
		return breadcrumb
	}
	parts := make([]string, 0, len(path))
	for _, item := range path {
		item = strings.TrimSpace(item)
		if item != "" {
			parts = append(parts, item)
		}
	}
	return strings.Join(parts, " > ")
}

func tableHeaderText(value string) string {
	return strings.TrimSpace(value)
}

func setFocusBorder(box *tview.Box, focused bool) {
	if box == nil {
		return
	}
	if focused {
		box.SetBorderColor(tcell.ColorGreen)
		return
	}
	box.SetBorderColor(tview.Styles.BorderColor)
}

func attachFocusBorder(box *tview.Box) {
	if box == nil {
		return
	}
	box.SetFocusFunc(func() {
		setFocusBorder(box, true)
	})
	box.SetBlurFunc(func() {
		setFocusBorder(box, false)
	})
}

func normalizeSelection(values []bool, total int, defaultValue bool) []bool {
	out := make([]bool, total)
	for i := 0; i < total; i++ {
		if i < len(values) {
			out[i] = values[i]
		} else {
			out[i] = defaultValue
		}
	}
	return out
}

func setAll(values []bool, target bool) {
	for i := range values {
		values[i] = target
	}
}

func paddedTableCell(value string) *tview.TableCell {
	return tview.NewTableCell(" " + value + " ")
}

func rowSelectionColumnWidths(columns []TableColumn, rows []TableRow, layout rowSelectionLayout, groupSort bool) []int {
	widths := make([]int, len(columns)+2)
	widths[0] = 5
	widths[1] = 6
	for i, column := range columns {
		width := len([]rune(strings.TrimSpace(firstNonEmptyText(column.Header, column.ID)))) + 2
		for _, row := range rows {
			if i < len(row.Cells) {
				cellWidth := len([]rune(strings.TrimSpace(row.Cells[i]))) + 2
				if cellWidth > width {
					width = cellWidth
				}
			}
		}
		if width < 6 {
			width = 6
		}
		widths[i+2] = width
	}
	return widths
}

func compareRowOrder(rows []TableRow, left int, right int, sort TableSort) int {
	if left < 0 || right < 0 || left >= len(rows) || right >= len(rows) {
		return left - right
	}
	if sort.Column < 0 {
		if sort.Direction == SortDescending {
			return right - left
		}
		return left - right
	}
	leftValue := ""
	rightValue := ""
	if sort.Column < len(rows[left].Cells) {
		leftValue = rows[left].Cells[sort.Column]
	}
	if sort.Column < len(rows[right].Cells) {
		rightValue = rows[right].Cells[sort.Column]
	}
	cmp := strings.Compare(strings.ToLower(strings.TrimSpace(leftValue)), strings.ToLower(strings.TrimSpace(rightValue)))
	if cmp == 0 {
		cmp = left - right
	}
	if sort.Direction == SortDescending {
		return -cmp
	}
	return cmp
}

func rowSelectionGroups(rows []TableRow, explicit []string) []rowSelectionGroup {
	groups := make([]rowSelectionGroup, 0, len(explicit))
	indexByLabel := map[string]int{}
	for _, label := range explicit {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if _, ok := indexByLabel[label]; ok {
			continue
		}
		indexByLabel[label] = len(groups)
		groups = append(groups, rowSelectionGroup{Label: label, Explicit: true})
	}
	for i, row := range rows {
		label := strings.TrimSpace(row.Group)
		if label == "" {
			label = "Ungrouped"
		}
		idx, ok := indexByLabel[label]
		if !ok {
			idx = len(groups)
			indexByLabel[label] = idx
			groups = append(groups, rowSelectionGroup{Label: label})
		}
		groups[idx].Rows = append(groups[idx].Rows, i)
	}
	return groups
}

func tableCellColor(column TableColumn, value string) tcell.Color {
	value = strings.ToLower(strings.TrimSpace(value))
	if column.ID == "uniprot_reviewed" {
		switch value {
		case "reviewed":
			return colorSelectionOn
		case "unreviewed":
			return colorMuted
		}
	}
	return tview.Styles.PrimaryTextColor
}

func tableHeaderLines(value string) (string, string) {
	value = tableHeaderText(value)
	if value == "" {
		return "", ""
	}
	parts := strings.SplitN(value, "\n", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func tableHeaderStyle(column TableColumn) tcell.Style {
	switch strings.ToLower(strings.TrimSpace(column.Reference)) {
	case "uniprot":
		return tcell.StyleDefault.Foreground(tcell.ColorLightSkyBlue).Bold(true)
	case "interpro":
		return tcell.StyleDefault.Foreground(tcell.ColorLightSeaGreen).Bold(true)
	default:
		return tcell.StyleDefault.Foreground(tview.Styles.PrimaryTextColor).Bold(true)
	}
}

func cloneBoolMatrix(values [][]bool) [][]bool {
	if values == nil {
		return nil
	}
	out := make([][]bool, len(values))
	for i := range values {
		out[i] = append([]bool(nil), values[i]...)
	}
	return out
}

func uiTaskSpec(description string) phygoboost.TaskSpec {
	return phygoboost.TaskSpec{
		Level:       phygoboost.ExecMain,
		LocalSlots:  1,
		Description: description,
	}
}

func RunTaskValue[T any](page TaskPage, task func(update func(string)) (T, error)) (T, error) {
	return RunTaskValueContext(page, func(ctx context.Context, update func(string)) (T, error) {
		return task(update)
	})
}

func runTaskValue[T any](page TaskPage, task func(ctx context.Context, update func(string)) (T, error)) (T, error) {
	result, err := runTaskInUIWorker(context.Background(), page, task)
	if !errors.Is(err, errTUIWorkerUnavailable) {
		return result, err
	}
	return runTaskValueInMainProcess(page, task)
}

func runProgressTaskValue[T any](page TaskPage, task func(ctx context.Context, update func(current int, message string)) (T, error)) (T, error) {
	result, err := runTaskInUIWorkerWithProgress(context.Background(), page, task)
	if !errors.Is(err, errTUIWorkerUnavailable) {
		return result, err
	}
	return runProgressTaskValueInMainProcess(page, task)
}

func actionCloseValue(actions []Action) string {
	for _, action := range actions {
		if actionLooksLikeClose(action.Label, action.Value) {
			return action.Value
		}
	}
	return ""
}

func actionLooksLikeClose(label string, value string) bool {
	text := strings.ToLower(strings.TrimSpace(label + " " + value))
	return strings.Contains(text, "close") || strings.Contains(text, "cancel") || strings.Contains(text, "back")
}

func textViewLineCount(value string) int {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	return len(strings.Split(value, "\n"))
}

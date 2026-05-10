package tui

import (
	"context"
	"errors"
	"testing"

	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
)

func TestTUIPageAndTaskWorkersStayInMainProcess(t *testing.T) {
	t.Setenv("PHYTOZOME_GO_ENABLE_TUI_PAGE_WORKER", "1")
	t.Setenv("PHYTOZOME_GO_ENABLE_TUI_TASK_WORKER", "1")
	t.Setenv("PHYTOZOME_GO_DISABLE_TUI_WORKER", "")

	if tuiWorkerEnabled() {
		t.Fatal("TUI workers must stay disabled so the interactive UI remains in the main process")
	}
	if tuiPageWorkerEnabled() {
		t.Fatal("TUI page workers must stay disabled so pages keep one terminal owner")
	}
	if tuiTaskWorkerEnabled() {
		t.Fatal("TUI task workers must stay disabled so task modals keep one terminal owner")
	}

	var result ChoiceResult
	handled, err := runPageInWorkerIfNeeded("tui.choice_page", ChoicePage{}, &result)
	if err != nil {
		t.Fatalf("page worker gate returned an error: %v", err)
	}
	if handled {
		t.Fatal("page worker gate handled a TUI page despite the main-process UI policy")
	}

	ran := false
	_, err = runTaskInUIWorkerWithProgress[int](context.Background(), TaskPage{}, func(ctx context.Context, update func(current int, message string)) (int, error) {
		ran = true
		return 1, nil
	})
	if !errors.Is(err, errTUIWorkerUnavailable) {
		t.Fatalf("task worker gate error = %v, want %v", err, errTUIWorkerUnavailable)
	}
	if ran {
		t.Fatal("task worker gate ran task code instead of falling back to the main-process UI")
	}
}

func TestInitialTaskStatusFallback(t *testing.T) {
	tests := []struct {
		name string
		page TaskPage
		want string
	}{
		{
			name: "initial",
			page: TaskPage{Initial: "  Loading genes  ", Description: "Description", Title: "Title"},
			want: "Loading genes",
		},
		{
			name: "description",
			page: TaskPage{Description: "  Searching species  ", Title: "Title"},
			want: "Searching species",
		},
		{
			name: "title",
			page: TaskPage{Title: "  Running task  "},
			want: "Running task",
		},
		{
			name: "default",
			page: TaskPage{},
			want: "Working...",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := initialTaskStatus(tt.page); got != tt.want {
				t.Fatalf("initialTaskStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTUIWorkersAreNotRegistered(t *testing.T) {
	tasks := []string{
		"tui.select_startup",
		"tui.choice_page",
		"tui.grouped_choice_page",
		"tui.text_input_page",
		"tui.multi_line_page",
		"tui.search_page",
		"tui.row_selection_page",
		"tui.blast_run_selection_page",
		"tui.info_page",
		"tui.action_modal_page",
		"tui.recovery_modal_page",
		"tui.choice_modal_page",
		"tui.export_settings_page",
		"tui.external_reference_page",
		"tui.family_blast_page",
		"tui.family_blast_customize_page",
		"tui.blast_filter_page",
		"tui.task_status_page",
	}
	for _, task := range tasks {
		if phygoboost.Registered(task) {
			t.Fatalf("TUI worker task %q must not be registered; the interactive UI belongs to the main process", task)
		}
	}
}


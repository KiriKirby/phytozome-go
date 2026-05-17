package tui

import (
	"context"
	"errors"
	"testing"
)

func TestTUIPageAndTaskExecutionStayInMainProcess(t *testing.T) {
	t.Setenv("PHYTOZOME_GO_ENABLE_TUI_PAGE_WORKER", "1")
	t.Setenv("PHYTOZOME_GO_ENABLE_TUI_TASK_WORKER", "1")
	t.Setenv("PHYTOZOME_GO_DISABLE_TUI_WORKER", "")

	if tuiParallelProcessEnabled() {
		t.Fatal("TUI parallel-process execution must stay disabled so the interactive UI remains in the main process")
	}
	if tuiPageParallelProcessEnabled() {
		t.Fatal("TUI page parallel-process execution must stay disabled so pages keep one terminal owner")
	}
	if tuiTaskParallelProcessEnabled() {
		t.Fatal("TUI task parallel-process execution must stay disabled so task modals keep one terminal owner")
	}

	var result ChoiceResult
	handled, err := runPageOutsideMainProcessIfNeeded("tui.choice_page", ChoicePage{}, &result)
	if err != nil {
		t.Fatalf("page main-process gate returned an error: %v", err)
	}
	if handled {
		t.Fatal("page gate handled a TUI page outside the main-process UI policy")
	}

	ran := false
	_, err = runTaskOutsideMainProcessWithProgress[int](context.Background(), TaskPage{}, func(ctx context.Context, update func(current int, message string)) (int, error) {
		ran = true
		return 1, nil
	})
	if !errors.Is(err, errTUIMustRunInMainProcess) {
		t.Fatalf("task main-process gate error = %v, want %v", err, errTUIMustRunInMainProcess)
	}
	if ran {
		t.Fatal("task gate ran task code outside the main-process UI path")
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

package prompt

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

func TestIdentityRowOrder(t *testing.T) {
	rows := []model.BlastResultRow{
		{Protein: "a", PercentIdentity: 71},
		{Protein: "b", PercentIdentity: 88},
		{Protein: "c", PercentIdentity: 88},
		{Protein: "d", PercentIdentity: 63},
	}

	order := identityRowOrder(rows)
	want := []int{1, 2, 0, 3}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("unexpected order at %d: got %d want %d", i, order[i], want[i])
		}
	}
}

func TestApplySelectionCommandUpDownAndRange(t *testing.T) {
	selected := []bool{false, false, false, false, false}
	order := []int{0, 1, 2, 3, 4}

	if err := applySelectionCommand(selected, order, []string{"up", "3"}, true); err != nil {
		t.Fatalf("apply up failed: %v", err)
	}
	if !selected[0] || !selected[1] || !selected[2] || selected[3] || selected[4] {
		t.Fatalf("unexpected selection after up: %v", selected)
	}

	if err := applySelectionCommand(selected, order, []string{"down", "4"}, false); err != nil {
		t.Fatalf("apply down failed: %v", err)
	}
	if selected[3] || selected[4] {
		t.Fatalf("down should include row 4 and below: %v", selected)
	}

	if err := applySelectionCommand(selected, order, []string{"2~4"}, true); err != nil {
		t.Fatalf("apply range failed: %v", err)
	}
	if !selected[1] || !selected[2] || !selected[3] {
		t.Fatalf("range 2~4 should be selected: %v", selected)
	}
}

func TestParseRowSpecRangeReversed(t *testing.T) {
	got, err := parseRowSpec("5~3", 8)
	if err != nil {
		t.Fatalf("parse reversed range failed: %v", err)
	}
	want := []int{3, 4, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected parsed range at %d: got %d want %d", i, got[i], want[i])
		}
	}
}

func TestPostRunActionKeywordMode(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("3\n"), &out)

	action, err := p.PostRunAction("keyword")
	if err != nil {
		t.Fatalf("PostRunAction returned error: %v", err)
	}
	if action != "change_mode" {
		t.Fatalf("unexpected action: got %q want %q", action, "change_mode")
	}

	output := out.String()
	if !strings.Contains(output, "Run keyword again with the same species") {
		t.Fatalf("keyword-specific prompt not shown: %q", output)
	}
	if strings.Contains(output, "BLAST again") {
		t.Fatalf("blast-specific prompt leaked into keyword mode: %q", output)
	}
}

func TestSelectBlastRowsBack(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("back\n"), &out)

	_, err := p.SelectBlastRows([]model.BlastResultRow{
		{Protein: "AT1G01010.1", Species: "A.thaliana TAIR10", EValue: "0", PercentIdentity: 100, GeneReportURL: "https://example.com"},
	})
	if !errors.Is(err, ErrBackToModeSelection) {
		t.Fatalf("expected ErrBackToModeSelection, got %v", err)
	}
}

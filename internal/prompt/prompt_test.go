package prompt

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/KiriKirby/phytozome-go/internal/locale"
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
	p := New(strings.NewReader("3\n"), &out, locale.English)

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

func TestPostRunActionBackShortcut(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("back\n"), &out, locale.English)

	action, err := p.PostRunAction("blast")
	if err != nil {
		t.Fatalf("PostRunAction returned error: %v", err)
	}
	if action != "change_species" {
		t.Fatalf("unexpected action: got %q want %q", action, "change_species")
	}
}

func TestPostRunActionLobbyShortcut(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("lobby\n"), &out, locale.English)

	_, err := p.PostRunAction("blast")
	if !errors.Is(err, ErrBackToDatabaseSelection) {
		t.Fatalf("expected ErrBackToDatabaseSelection, got %v", err)
	}
}

func TestSelectBlastRowsBack(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("back\n"), &out, locale.English)

	_, err := p.SelectBlastRows([]model.BlastResultRow{
		{Protein: "AT1G01010.1", Species: "A.thaliana TAIR10", EValue: "0", PercentIdentity: 100, GeneReportURL: "https://example.com"},
	})
	if !errors.Is(err, ErrBackToQueryInput) {
		t.Fatalf("expected ErrBackToQueryInput, got %v", err)
	}
}

func TestSelectBlastRowsBatchDoneAll(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("done all\n"), &out, locale.English)

	rows, doneAll, err := p.SelectBlastRowsBatch([]model.BlastResultRow{
		{Protein: "AT1G01010.1", Species: "A.thaliana TAIR10", EValue: "0", PercentIdentity: 100, GeneReportURL: "https://example.com"},
	})
	if err != nil {
		t.Fatalf("SelectBlastRowsBatch returned error: %v", err)
	}
	if !doneAll {
		t.Fatalf("expected doneAll to be true")
	}
	if len(rows) != 1 {
		t.Fatalf("unexpected selected rows count: %d", len(rows))
	}
}

func TestBlastProteinIdentificationsOptionalSkip(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("\n"), &out, locale.English)

	got, err := p.BlastProteinIdentifications(1, false)
	if err != nil {
		t.Fatalf("BlastProteinIdentifications returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil result for skipped optional label, got %v", got)
	}
}

func TestBlastProteinIdentificationsRequiredCount(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("A\nB\n\n"), &out, locale.English)

	got, err := p.BlastProteinIdentifications(2, true)
	if err != nil {
		t.Fatalf("BlastProteinIdentifications returned error: %v", err)
	}
	want := []string{"A", "B"}
	if len(got) != len(want) {
		t.Fatalf("unexpected length: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected value at %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestChooseBlastProgramShowsDescriptions(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("1\n"), &out, locale.English)

	program, err := p.ChooseBlastProgram([]string{"blastn", "blastx", "tblastn", "blastp"})
	if err != nil {
		t.Fatalf("ChooseBlastProgram returned error: %v", err)
	}
	if program != "blastn" {
		t.Fatalf("unexpected program: got %q want %q", program, "blastn")
	}
	output := out.String()
	for _, want := range []string{
		"Nucleotide query starts here:",
		"Protein query starts here:",
		"blastn - nucleotide query -> nucleotide/genome database",
		"blastx - nucleotide query -> translated protein -> protein database",
		"tblastn - protein query -> translated nucleotide/genome database",
		"blastp - protein query -> protein database",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected prompt to contain %q, got %q", want, output)
		}
	}
}

func TestChooseBlastExecutionExplainsLocalMode(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("2\n"), &out, locale.English)

	mode, err := p.ChooseBlastExecution()
	if err != nil {
		t.Fatalf("ChooseBlastExecution returned error: %v", err)
	}
	if mode != "local" {
		t.Fatalf("unexpected execution mode: got %q want %q", mode, "local")
	}
	output := out.String()
	for _, want := range []string{
		"download the lemna FASTA files automatically and run BLAST on this computer",
		"does not require you to prepare the FASTA files yourself",
		"require NCBI BLAST+ on PATH",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected prompt to contain %q, got %q", want, output)
		}
	}
}

func TestBlastSubmitErrorActionBack(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("back\n"), &out, locale.English)

	_, err := p.BlastSubmitErrorAction("submit BLAST job: failed")
	if !errors.Is(err, ErrBackToBlastProgram) {
		t.Fatalf("expected ErrBackToBlastProgram, got %v", err)
	}
	if !strings.Contains(out.String(), "choose BLAST program / execution target again") {
		t.Fatalf("back option not shown: %q", out.String())
	}
}

func TestChooseModeLobby(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("lobby\n"), &out, locale.English)

	_, err := p.ChooseMode()
	if !errors.Is(err, ErrBackToDatabaseSelection) {
		t.Fatalf("expected ErrBackToDatabaseSelection, got %v", err)
	}
}

func TestSequenceInputSpawn(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("spawn\n"), &out, locale.English)

	_, err := p.SequenceInput()
	if !errors.Is(err, ErrBackToModeSelection) {
		t.Fatalf("expected ErrBackToModeSelection, got %v", err)
	}
}

func TestKeywordProteinIdentificationsTilde(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("ID1 ~\n\n"), &out, locale.English)

	got, err := p.KeywordProteinIdentifications(2)
	if err != nil {
		t.Fatalf("KeywordProteinIdentifications returned error: %v", err)
	}
	want := []string{"ID1", ""}
	if len(got) != len(want) {
		t.Fatalf("unexpected length: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected value at %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestBlastPlusInstallActionInstall(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("install\n"), &out, locale.English)

	action, err := p.BlastPlusInstallAction("makeblastdb not found")
	if err != nil {
		t.Fatalf("BlastPlusInstallAction returned error: %v", err)
	}
	if action != "install" {
		t.Fatalf("unexpected action: got %q want %q", action, "install")
	}
	output := out.String()
	for _, want := range []string{
		"download official NCBI BLAST+ for this app now",
		"choose BLAST program / execution target again",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected prompt to contain %q, got %q", want, output)
		}
	}
}

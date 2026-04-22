package prompt

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/wangsychn/phytozome-batch-cli/internal/model"
)

type Prompter struct {
	in  *bufio.Reader
	out io.Writer
}

func New(in io.Reader, out io.Writer) *Prompter {
	return &Prompter{
		in:  bufio.NewReader(in),
		out: out,
	}
}

func (p *Prompter) SpeciesKeyword() (string, error) {
	return p.readLine("Enter species keyword: ")
}

func (p *Prompter) SelectSpecies(candidates []model.SpeciesCandidate) (model.SpeciesCandidate, error) {
	if len(candidates) == 0 {
		return model.SpeciesCandidate{}, fmt.Errorf("no candidates available")
	}

	for i, candidate := range candidates {
		fmt.Fprintf(p.out, "%d. %s\n", i+1, candidate.DisplayLabel())
		fmt.Fprintf(p.out, "   %s (proteome %d)\n", candidate.JBrowseName, candidate.ProteomeID)
	}

	for {
		value, err := p.readLine("Choose one species by number: ")
		if err != nil {
			return model.SpeciesCandidate{}, err
		}

		index, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || index < 1 || index > len(candidates) {
			fmt.Fprintln(p.out, "Invalid selection. Enter one of the numbers above.")
			continue
		}

		return candidates[index-1], nil
	}
}

func (p *Prompter) SequenceInput() (string, error) {
	fmt.Fprintln(p.out, "Paste sequence lines. Finish with an empty line.")

	lines := make([]string, 0, 8)
	for {
		line, err := p.in.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			return strings.Join(lines, "\n"), nil
		}

		lines = append(lines, line)
		if err == io.EOF {
			return strings.Join(lines, "\n"), nil
		}
	}
}

func (p *Prompter) SelectBlastRows(rows []model.BlastResultRow) ([]model.BlastResultRow, error) {
	if len(rows) == 0 {
		return nil, nil
	}

	selected := make([]bool, len(rows))
	for i := range selected {
		selected[i] = true
	}

	for {
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, "Selected BLAST rows:")
		for i, row := range rows {
			marker := " "
			if selected[i] {
				marker = "x"
			}
			fmt.Fprintf(
				p.out,
				"[%s] %d. %s | %s | e=%s | id=%.2f%% | %s\n",
				marker,
				i+1,
				row.Protein,
				row.Species,
				row.EValue,
				row.PercentIdentity,
				row.GeneReportURL,
			)
		}
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, "Commands: all | none | toggle 1 2 3 | done")

		input, err := p.readLine("Selection command: ")
		if err != nil {
			return nil, err
		}

		fields := strings.Fields(strings.ToLower(strings.TrimSpace(input)))
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "all":
			for i := range selected {
				selected[i] = true
			}
		case "none":
			for i := range selected {
				selected[i] = false
			}
		case "toggle":
			if len(fields) == 1 {
				fmt.Fprintln(p.out, "Provide one or more row numbers after 'toggle'.")
				continue
			}
			if err := toggleSelections(selected, fields[1:]); err != nil {
				fmt.Fprintf(p.out, "Invalid toggle command: %v\n", err)
			}
		case "done":
			chosen := make([]model.BlastResultRow, 0, len(rows))
			for i, ok := range selected {
				if ok {
					chosen = append(chosen, rows[i])
				}
			}
			if len(chosen) == 0 {
				fmt.Fprintln(p.out, "No rows selected.")
				continue
			}
			return chosen, nil
		default:
			fmt.Fprintln(p.out, "Unknown command.")
		}
	}
}

func toggleSelections(selected []bool, fields []string) error {
	for _, field := range fields {
		index, err := strconv.Atoi(field)
		if err != nil {
			return fmt.Errorf("invalid row number %q", field)
		}
		if index < 1 || index > len(selected) {
			return fmt.Errorf("row %d out of range", index)
		}
		selected[index-1] = !selected[index-1]
	}
	return nil
}

func (p *Prompter) readLine(label string) (string, error) {
	fmt.Fprint(p.out, label)
	line, err := p.in.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return strings.TrimSpace(line), nil
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}

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

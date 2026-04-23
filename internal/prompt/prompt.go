package prompt

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/KiriKirby/phytozome-go/internal/model"
)

type Prompter struct {
	in  *bufio.Reader
	out io.Writer
}

var invalidFileNameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)

func New(in io.Reader, out io.Writer) *Prompter {
	return &Prompter{
		in:  bufio.NewReader(in),
		out: out,
	}
}

func (p *Prompter) ChooseMode() (string, error) {
	for {
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, "Choose mode:")
		fmt.Fprintln(p.out, " 1) blast   - sequence / FASTA / URL query against one species")
		fmt.Fprintln(p.out, " 2) keyword - keyword gene search within one species")

		value, err := p.readLine("Choose 1 or 2 (or 'blast'/'keyword'): ")
		if err != nil {
			return "", err
		}
		if isHelpCommand(value) {
			p.printModeHelp()
			continue
		}

		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "blast", "b":
			return "blast", nil
		case "2", "keyword", "k":
			return "keyword", nil
		default:
			fmt.Fprintln(p.out, "Please enter one of: 1, 2, 'blast', or 'keyword'.")
		}
	}
}

func (p *Prompter) SpeciesKeyword() (string, error) {
	for {
		value, err := p.readLine("Search species keyword: ")
		if err != nil {
			return "", err
		}
		if isHelpCommand(value) {
			p.printSpeciesSearchHelp()
			continue
		}
		return value, nil
	}
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
		if isHelpCommand(value) {
			p.printSpeciesChooseHelp()
			continue
		}

		index, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || index < 1 || index > len(candidates) {
			fmt.Fprintln(p.out, "Invalid selection. Enter one of the numbers above.")
			continue
		}

		return candidates[index-1], nil
	}
}

func (p *Prompter) SearchAndSelectSpecies(candidates []model.SpeciesCandidate, searchFn func(string) []model.SpeciesCandidate) (model.SpeciesCandidate, error) {
	keyword := ""

	for {
		if keyword == "" {
			var err error
			keyword, err = p.SpeciesKeyword()
			if err != nil {
				return model.SpeciesCandidate{}, err
			}
		}

		filtered := searchFn(keyword)
		if len(filtered) == 0 {
			fmt.Fprintf(p.out, "No species candidates matched %q.\n", keyword)
			keyword = ""
			continue
		}

		if len(filtered) > 12 {
			filtered = filtered[:12]
		}

		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, "Candidate species for %q:\n", keyword)
		for i, candidate := range filtered {
			fmt.Fprintf(p.out, "%d. %s\n", i+1, candidate.DisplayLabel())
			fmt.Fprintf(p.out, "   %s (proteome %d)\n", candidate.JBrowseName, candidate.ProteomeID)
		}
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, "Enter a number to choose one candidate.")
		fmt.Fprintln(p.out, "Or type another keyword to search again.")

		value, err := p.readLine("Choose species or search again: ")
		if err != nil {
			return model.SpeciesCandidate{}, err
		}
		if isHelpCommand(value) {
			p.printSpeciesSearchHelp()
			p.printSpeciesChooseHelp()
			continue
		}

		index, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil && index >= 1 && index <= len(filtered) {
			return filtered[index-1], nil
		}

		if strings.TrimSpace(value) == "" {
			fmt.Fprintln(p.out, "Please enter a number or another keyword.")
			continue
		}

		keyword = value
	}
}

func (p *Prompter) SequenceInput() (string, error) {
	fmt.Fprintln(p.out, "Paste sequence lines, a FASTA-style header plus sequence, or a Phytozome gene report URL.")
	fmt.Fprintln(p.out, "Finish sequence input with an empty line.")

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
		if len(lines) == 0 && isHelpCommand(line) {
			p.printSequenceInputHelp()
			continue
		}

		if len(lines) == 0 && looksLikeURL(line) {
			return line, nil
		}

		lines = append(lines, line)
		if err == io.EOF {
			return strings.Join(lines, "\n"), nil
		}
	}
}

func (p *Prompter) KeywordInput() (string, error) {
	fmt.Fprintln(p.out, "Enter one or more keywords separated by spaces or new lines.")
	fmt.Fprintln(p.out, "Finish keyword input with an empty line.")

	lines := make([]string, 0, 4)
	for {
		line, err := p.in.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}

		line = strings.TrimSpace(line)
		if len(lines) == 0 && isHelpCommand(line) {
			p.printKeywordInputHelp()
			continue
		}
		if line == "" {
			return strings.Join(lines, "\n"), nil
		}

		lines = append(lines, line)
		if err == io.EOF {
			return strings.Join(lines, "\n"), nil
		}
	}
}

func (p *Prompter) SelectKeywordRows(groups []model.KeywordSearchGroup) ([]model.KeywordResultRow, error) {
	totalRows := 0
	for _, group := range groups {
		totalRows += len(group.Rows)
	}
	if totalRows == 0 {
		return nil, nil
	}

	selected := make([]bool, totalRows)
	order := defaultRowOrder(totalRows)

	rowIndex := 0
	for _, group := range groups {
		if len(group.Rows) == 0 {
			continue
		}
		selected[rowIndex] = true
		rowIndex += len(group.Rows)
	}

	for {
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, "Keyword results:")
		fmt.Fprintln(p.out, "sel\trow\ttranscript\tgene_identifier\tgenome\tlocation\talias\tuniprot\tdescription\tauto_define\tgene_report_url")

		writer := tabwriter.NewWriter(p.out, 0, 4, 2, ' ', 0)
		displayIndex := 0
		for _, group := range groups {
			fmt.Fprintf(writer, "----%s----\n", group.SearchTerm)
			if len(group.Rows) == 0 {
				fmt.Fprintln(writer, "No results")
				continue
			}
			for _, row := range group.Rows {
				marker := "[ ]"
				if selected[displayIndex] {
					marker = "[x]"
				}
				fmt.Fprintf(
					writer,
					"%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					marker,
					displayIndex+1,
					row.TranscriptID,
					row.GeneIdentifier,
					row.Genome,
					row.Location,
					row.Aliases,
					row.UniProt,
					row.Description,
					row.AutoDefine,
					row.GeneReportURL,
				)
				displayIndex++
			}
		}
		if err := writer.Flush(); err != nil {
			return nil, err
		}

		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, "Commands:")
		fmt.Fprintln(p.out, "  all | none")
		fmt.Fprintln(p.out, "  toggle 1 2 3 5~8")
		fmt.Fprintln(p.out, "  on 5~8 | off 5~8")
		fmt.Fprintln(p.out, "  on up 12 | off up 12")
		fmt.Fprintln(p.out, "  on down 12 | off down 12")
		fmt.Fprintln(p.out, "  done")

		input, err := p.readLine("Selection command: ")
		if err != nil {
			return nil, err
		}
		if isHelpCommand(input) {
			p.printKeywordSelectionHelp()
			continue
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
				fmt.Fprintln(p.out, "Provide one or more row numbers or ranges after 'toggle'.")
				continue
			}
			if err := toggleSelections(selected, order, fields[1:]); err != nil {
				fmt.Fprintf(p.out, "Invalid toggle command: %v\n", err)
			}
		case "on", "off", "select", "unselect":
			targetValue, ok := commandTargetValue(fields[0])
			if !ok {
				fmt.Fprintln(p.out, "Unknown selection command.")
				continue
			}
			if len(fields) < 2 {
				fmt.Fprintln(p.out, "Provide a range like '5~8' or a direction like 'up 12'.")
				continue
			}
			if err := applySelectionCommand(selected, order, fields[1:], targetValue); err != nil {
				fmt.Fprintf(p.out, "Invalid selection command: %v\n", err)
			}
		case "done":
			chosen := make([]model.KeywordResultRow, 0, totalRows)
			displayIndex := 0
			for _, group := range groups {
				for _, row := range group.Rows {
					if selected[displayIndex] {
						chosen = append(chosen, row)
					}
					displayIndex++
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

func (p *Prompter) SelectBlastRows(rows []model.BlastResultRow) ([]model.BlastResultRow, error) {
	if len(rows) == 0 {
		return nil, nil
	}

	selected := make([]bool, len(rows))
	for i := range selected {
		selected[i] = true
	}
	order := defaultRowOrder(len(rows))
	sortMode := "default"

	for {
		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, "Selected BLAST rows (sort: %s):\n", sortMode)
		for displayIndex, rowIndex := range order {
			row := rows[rowIndex]
			marker := " "
			if selected[rowIndex] {
				marker = "x"
			}
			fmt.Fprintf(
				p.out,
				"[%s] %d. %s | %s | e=%s | id=%.2f%% | %s\n",
				marker,
				displayIndex+1,
				row.Protein,
				row.Species,
				row.EValue,
				row.PercentIdentity,
				row.GeneReportURL,
			)
		}
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, "Commands:")
		fmt.Fprintln(p.out, "  sort default | sort identity")
		fmt.Fprintln(p.out, "  all | none")
		fmt.Fprintln(p.out, "  toggle 1 2 3 5~8")
		fmt.Fprintln(p.out, "  on 5~8 | off 5~8")
		fmt.Fprintln(p.out, "  on up 12 | off up 12")
		fmt.Fprintln(p.out, "  on down 12 | off down 12")
		fmt.Fprintln(p.out, "  done")

		input, err := p.readLine("Selection command: ")
		if err != nil {
			return nil, err
		}
		if isHelpCommand(input) {
			p.printBlastSelectionHelp()
			continue
		}

		fields := strings.Fields(strings.ToLower(strings.TrimSpace(input)))
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "sort":
			if len(fields) != 2 {
				fmt.Fprintln(p.out, "Use 'sort default' or 'sort identity'.")
				continue
			}
			switch fields[1] {
			case "default":
				order = defaultRowOrder(len(rows))
				sortMode = "default"
			case "identity":
				order = identityRowOrder(rows)
				sortMode = "identity"
			default:
				fmt.Fprintln(p.out, "Use 'sort default' or 'sort identity'.")
			}
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
				fmt.Fprintln(p.out, "Provide one or more row numbers or ranges after 'toggle'.")
				continue
			}
			if err := toggleSelections(selected, order, fields[1:]); err != nil {
				fmt.Fprintf(p.out, "Invalid toggle command: %v\n", err)
			}
		case "on", "off", "select", "unselect":
			targetValue, ok := commandTargetValue(fields[0])
			if !ok {
				fmt.Fprintln(p.out, "Unknown selection command.")
				continue
			}
			if len(fields) < 2 {
				fmt.Fprintln(p.out, "Provide a range like '5~8' or a direction like 'up 12'.")
				continue
			}
			if err := applySelectionCommand(selected, order, fields[1:], targetValue); err != nil {
				fmt.Fprintf(p.out, "Invalid selection command: %v\n", err)
			}
		case "done":
			chosen := make([]model.BlastResultRow, 0, len(rows))
			for _, rowIndex := range order {
				if selected[rowIndex] {
					chosen = append(chosen, rows[rowIndex])
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

func (p *Prompter) ExportBaseName() (string, error) {
	for {
		value, err := p.readLine("Export file name (without extension): ")
		if err != nil {
			return "", err
		}
		if isHelpCommand(value) {
			p.printExportNameHelp()
			continue
		}

		name := sanitizeFileName(value)
		if name == "" {
			fmt.Fprintln(p.out, "File name cannot be empty.")
			continue
		}

		return name, nil
	}
}

// PostRunAction prompts the user for what to do after a completed run.
// It returns one of:
//   - "repeat"         : run the current mode again with the same species
//   - "change_species" : go back to species search/selection within the current mode
//   - "change_mode"    : switch between blast and keyword mode
//   - "exit"           : quit the wizard
func (p *Prompter) PostRunAction(mode string) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	modeLabel := mode
	if modeLabel == "" {
		modeLabel = "current"
	}

	for {
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, "What would you like to do next?")
		fmt.Fprintf(p.out, " 1) Run %s again with the same species\n", modeLabel)
		fmt.Fprintf(p.out, " 2) Change species and run %s\n", modeLabel)
		fmt.Fprintln(p.out, " 3) Switch mode")
		fmt.Fprintln(p.out, " 4) Exit")

		input, err := p.readLine("Choose 1, 2, 3, or 4 (or 'repeat'/'change'/'mode'/'exit'): ")
		if err != nil {
			return "", err
		}
		if isHelpCommand(input) {
			p.printPostRunHelp(modeLabel)
			continue
		}
		val := strings.ToLower(strings.TrimSpace(input))
		switch val {
		case "1", "repeat", "r":
			return "repeat", nil
		case "2", "change", "c":
			return "change_species", nil
		case "3", "mode", "m", "switch":
			return "change_mode", nil
		case "4", "exit", "e", "q":
			return "exit", nil
		default:
			fmt.Fprintln(p.out, "Please enter one of: 1, 2, 3, 4, or 'repeat', 'change', 'mode', 'exit'.")
		}
	}
}

func toggleSelections(selected []bool, order []int, fields []string) error {
	for _, field := range fields {
		indexes, err := parseRowSpec(field, len(order))
		if err != nil {
			return err
		}
		for _, displayIndex := range indexes {
			rowIndex := order[displayIndex-1]
			selected[rowIndex] = !selected[rowIndex]
		}
	}
	return nil
}

func applySelectionCommand(selected []bool, order []int, args []string, value bool) error {
	if len(args) == 0 {
		return fmt.Errorf("missing selection arguments")
	}

	switch args[0] {
	case "up", "down":
		if len(args) != 2 {
			return fmt.Errorf("use '%s <row>' with exactly one row number", args[0])
		}
		index, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid row number %q", args[1])
		}
		if index < 1 || index > len(order) {
			return fmt.Errorf("row %d out of range", index)
		}

		var indexes []int
		if args[0] == "up" {
			indexes = make([]int, 0, index)
			for i := 1; i <= index; i++ {
				indexes = append(indexes, i)
			}
		} else {
			indexes = make([]int, 0, len(order)-index+1)
			for i := index; i <= len(order); i++ {
				indexes = append(indexes, i)
			}
		}
		setDisplayIndexes(selected, order, indexes, value)
		return nil
	default:
		indexes := make([]int, 0, len(args))
		for _, arg := range args {
			parsed, err := parseRowSpec(arg, len(order))
			if err != nil {
				return err
			}
			indexes = append(indexes, parsed...)
		}
		setDisplayIndexes(selected, order, indexes, value)
		return nil
	}
}

func setDisplayIndexes(selected []bool, order []int, indexes []int, value bool) {
	for _, displayIndex := range indexes {
		rowIndex := order[displayIndex-1]
		selected[rowIndex] = value
	}
}

func parseRowSpec(spec string, max int) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty row spec")
	}
	if strings.Contains(spec, "~") {
		parts := strings.Split(spec, "~")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range %q", spec)
		}
		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid range start %q", parts[0])
		}
		end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid range end %q", parts[1])
		}
		if start < 1 || end < 1 || start > max || end > max {
			return nil, fmt.Errorf("range %q out of bounds", spec)
		}
		if start > end {
			start, end = end, start
		}
		indexes := make([]int, 0, end-start+1)
		for i := start; i <= end; i++ {
			indexes = append(indexes, i)
		}
		return indexes, nil
	}

	index, err := strconv.Atoi(spec)
	if err != nil {
		return nil, fmt.Errorf("invalid row number %q", spec)
	}
	if index < 1 || index > max {
		return nil, fmt.Errorf("row %d out of range", index)
	}
	return []int{index}, nil
}

func commandTargetValue(command string) (bool, bool) {
	switch command {
	case "on", "select":
		return true, true
	case "off", "unselect":
		return false, true
	default:
		return false, false
	}
}

func defaultRowOrder(size int) []int {
	order := make([]int, size)
	for i := range order {
		order[i] = i
	}
	return order
}

func identityRowOrder(rows []model.BlastResultRow) []int {
	order := defaultRowOrder(len(rows))
	sort.SliceStable(order, func(i, j int) bool {
		left := rows[order[i]]
		right := rows[order[j]]
		if left.PercentIdentity != right.PercentIdentity {
			return left.PercentIdentity > right.PercentIdentity
		}
		return order[i] < order[j]
	})
	return order
}

// FetchErrorAction prompts the user when a fetch (gene/sequence) operation fails.
// It returns one of:
//   - "retry" : attempt the fetch again
//   - "skip"  : skip this record and continue
//   - "abort" : abort the wizard entirely
func (p *Prompter) FetchErrorAction(description string) (string, error) {
	for {
		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, "Failed to fetch: %s\n", description)
		fmt.Fprintln(p.out, "Options:")
		fmt.Fprintln(p.out, " 1) retry  - try again now")
		fmt.Fprintln(p.out, " 2) skip   - skip this record and continue")
		fmt.Fprintln(p.out, " 3) abort  - abort the wizard")
		input, err := p.readLine("Choose 1,2,3 (or 'retry'/'skip'/'abort'): ")
		if err != nil {
			return "", err
		}
		if isHelpCommand(input) {
			p.printFetchErrorHelp()
			continue
		}
		val := strings.ToLower(strings.TrimSpace(input))
		switch val {
		case "1", "retry", "r":
			return "retry", nil
		case "2", "skip", "s":
			return "skip", nil
		case "3", "abort", "a":
			return "abort", nil
		default:
			fmt.Fprintln(p.out, "Please enter one of: 1, 2, 3, or 'retry', 'skip', 'abort'.")
		}
	}
}

// WorkflowErrorAction prompts the user when a higher-level wizard step fails.
// It returns one of:
//   - "retry" : attempt the step again
//   - "exit"  : stop the wizard
func (p *Prompter) WorkflowErrorAction(description string) (string, error) {
	for {
		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, "Step failed: %s\n", description)
		fmt.Fprintln(p.out, "Options:")
		fmt.Fprintln(p.out, " 1) retry - try this step again")
		fmt.Fprintln(p.out, " 2) exit  - stop the wizard")
		input, err := p.readLine("Choose 1 or 2 (or 'retry'/'exit'): ")
		if err != nil {
			return "", err
		}
		if isHelpCommand(input) {
			p.printWorkflowErrorHelp()
			continue
		}
		val := strings.ToLower(strings.TrimSpace(input))
		switch val {
		case "1", "retry", "r":
			return "retry", nil
		case "2", "exit", "e", "q":
			return "exit", nil
		default:
			fmt.Fprintln(p.out, "Please enter one of: 1, 2, or 'retry', 'exit'.")
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

func sanitizeFileName(value string) string {
	value = strings.TrimSpace(value)
	value = invalidFileNameChars.ReplaceAllString(value, "_")
	value = strings.Trim(value, ". ")
	return value
}

func isHelpCommand(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "help" || value == "?"
}

func (p *Prompter) printSpeciesSearchHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, "Help: species search")
	fmt.Fprintln(p.out, " Enter a partial species keyword such as 'spiro', 'wheat', or 'arabidopsis'.")
	fmt.Fprintln(p.out, " You can search by abbreviated name, full scientific name, or common name.")
	fmt.Fprintln(p.out, " Type 'help' or '?' at this prompt to see this message again.")
}

func (p *Prompter) printModeHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, "Help: mode selection")
	fmt.Fprintln(p.out, " blast: current main workflow for sequence / FASTA / URL based search")
	fmt.Fprintln(p.out, " keyword: keyword gene search and export within one selected species")
	fmt.Fprintln(p.out, " The chosen mode stays active for the current session.")
}

func (p *Prompter) printSpeciesChooseHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, "Help: species selection")
	fmt.Fprintln(p.out, " Enter a numbered candidate to choose that species.")
	fmt.Fprintln(p.out, " Or type another keyword to run a new search.")
	fmt.Fprintln(p.out, " Type 'help' or '?' at this prompt to see this message again.")
}

func (p *Prompter) printSequenceInputHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, "Help: query input")
	fmt.Fprintln(p.out, " Accepted inputs:")
	fmt.Fprintln(p.out, "  1) plain sequence")
	fmt.Fprintln(p.out, "  2) FASTA-style header plus sequence")
	fmt.Fprintln(p.out, "  3) Phytozome gene/transcript report URL")
	fmt.Fprintln(p.out, " Finish sequence entry with an empty line.")
	fmt.Fprintln(p.out, " Single-line FASTA entries and trailing labels like '(AtC4H)' are accepted.")
}

func (p *Prompter) printKeywordInputHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, "Help: keyword input")
	fmt.Fprintln(p.out, " Enter one or more keywords within the selected species.")
	fmt.Fprintln(p.out, " You can separate them by spaces or by new lines.")
	fmt.Fprintln(p.out, " Finish the whole input with an empty line.")
}

func (p *Prompter) printBlastSelectionHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, "Help: BLAST row selection")
	fmt.Fprintln(p.out, " sort default | sort identity")
	fmt.Fprintln(p.out, " all | none")
	fmt.Fprintln(p.out, " toggle 1 2 3 5~8")
	fmt.Fprintln(p.out, " on 5~8 | off 5~8")
	fmt.Fprintln(p.out, " on up 12 | off up 12")
	fmt.Fprintln(p.out, " on down 12 | off down 12")
	fmt.Fprintln(p.out, " done")
	fmt.Fprintln(p.out, " All row numbers are based on the current visible order.")
}

func (p *Prompter) printKeywordSelectionHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, "Help: keyword row selection")
	fmt.Fprintln(p.out, " all | none")
	fmt.Fprintln(p.out, " toggle 1 2 3 5~8")
	fmt.Fprintln(p.out, " on 5~8 | off 5~8")
	fmt.Fprintln(p.out, " on up 12 | off up 12")
	fmt.Fprintln(p.out, " on down 12 | off down 12")
	fmt.Fprintln(p.out, " done")
	fmt.Fprintln(p.out, " The first result under each search term is selected by default.")
}

func (p *Prompter) printExportNameHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, "Help: export file name")
	fmt.Fprintln(p.out, " Enter one base name without extension.")
	fmt.Fprintln(p.out, " The program will create both '<name>.xlsx' and '<name>.txt'.")
	fmt.Fprintln(p.out, " Invalid Windows filename characters will be replaced automatically.")
}

func (p *Prompter) printPostRunHelp(modeLabel string) {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, "Help: next action")
	fmt.Fprintf(p.out, " repeat: run %s again with the same species\n", modeLabel)
	fmt.Fprintf(p.out, " change: go back to species search for %s\n", modeLabel)
	fmt.Fprintln(p.out, " mode: switch between blast and keyword")
	fmt.Fprintln(p.out, " exit: quit the wizard")
}

func (p *Prompter) printFetchErrorHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, "Help: fetch error options")
	fmt.Fprintln(p.out, " retry: try the same remote fetch again")
	fmt.Fprintln(p.out, " skip: omit this record and continue")
	fmt.Fprintln(p.out, " abort: stop the current export/workflow")
}

func (p *Prompter) printWorkflowErrorHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, "Help: workflow error options")
	fmt.Fprintln(p.out, " retry: try the current step again")
	fmt.Fprintln(p.out, " exit: stop the wizard")
}

func looksLikeURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		return err == nil && parsed.Host != ""
	}
	if strings.Contains(value, "phytozome-next.jgi.doe.gov/") {
		parsed, err := url.Parse("https://" + value)
		return err == nil && parsed.Host != ""
	}
	return false
}

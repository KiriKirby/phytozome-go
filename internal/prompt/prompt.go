package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/KiriKirby/phytozome-go/internal/locale"
	"github.com/KiriKirby/phytozome-go/internal/model"
)

type Prompter struct {
	in   *bufio.Reader
	out  io.Writer
	lang locale.Language
}

var activeLanguage = locale.English

var invalidFileNameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)
var errBackCommand = errors.New("global back command")
var errSpawnCommand = errors.New("global spawn command")
var errLobbyCommand = errors.New("global lobby command")
var ErrBackToDatabaseSelection = errors.New("back to database selection")
var ErrBackToModeSelection = errors.New("back to mode selection")
var ErrBackToSpeciesSelection = errors.New("back to species selection")
var ErrBackToQueryInput = errors.New("back to query input")
var ErrBackToBlastProgram = errors.New("back to BLAST program selection")
var ErrBackToRowSelection = errors.New("back to row selection")
var ErrExitRequested = errors.New("exit requested")

func New(in io.Reader, out io.Writer, lang locale.Language) *Prompter {
	activeLanguage = lang
	return &Prompter{
		in:   bufio.NewReader(in),
		out:  out,
		lang: lang,
	}
}

func (p *Prompter) Language() locale.Language {
	return p.lang
}

func (p *Prompter) SetLanguage(lang locale.Language) {
	p.lang = lang
	activeLanguage = lang
}

func (p *Prompter) t(text string) string {
	return locale.Text(p.lang, text)
}

func (p *Prompter) tf(text string, args ...any) string {
	return fmt.Sprintf(p.t(text), args...)
}

func printGlobalCommandHint(out io.Writer) {
	fmt.Fprintln(out, locale.Text(activeLanguage, "Global navigation: back - previous page | spawn - mode selection | lobby - database selection | exit - quit the wizard"))
}

func printSelectionCommands(out io.Writer, includeList bool) {
	fmt.Fprintln(out, locale.Text(activeLanguage, "Selection commands:"))
	fmt.Fprintln(out, locale.Text(activeLanguage, "  all - select every row"))
	fmt.Fprintln(out, locale.Text(activeLanguage, "  none - clear all selections"))
	fmt.Fprintln(out, locale.Text(activeLanguage, "  toggle 1 2 3 5~8 - flip the selected state for the listed rows or ranges"))
	fmt.Fprintln(out, locale.Text(activeLanguage, "  on 5~8 | off 5~8 - select or clear an explicit row range"))
	fmt.Fprintln(out, locale.Text(activeLanguage, "  on up 12 | off up 12 - select or clear rows from the start through row 12"))
	fmt.Fprintln(out, locale.Text(activeLanguage, "  on down 12 | off down 12 - select or clear rows from row 12 through the end"))
	if includeList {
		fmt.Fprintln(out, locale.Text(activeLanguage, "  list - preview the currently selected rows and optionally write a _list file"))
	}
	fmt.Fprintln(out, locale.Text(activeLanguage, "  done - confirm the current selection"))
	fmt.Fprintln(out, locale.Text(activeLanguage, "  back - return to the previous page"))
	fmt.Fprintln(out, locale.Text(activeLanguage, "  spawn - jump back to mode selection"))
	fmt.Fprintln(out, locale.Text(activeLanguage, "  lobby - jump back to database selection"))
	fmt.Fprintln(out, locale.Text(activeLanguage, "  exit - quit the program"))
}

func printListOutputCommands(out io.Writer) {
	fmt.Fprintln(out, locale.Text(activeLanguage, "List output actions:"))
	fmt.Fprintln(out, locale.Text(activeLanguage, "  back - return to the selection table without changing the current selection"))
	fmt.Fprintln(out, locale.Text(activeLanguage, "  txt - write the _list text file now"))
	fmt.Fprintln(out, locale.Text(activeLanguage, "  exit - quit the program"))
}

func printBlastSelectionTable(out io.Writer, rows []model.BlastResultRow, selected []bool) {
	writer := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "sel\trow\tprotein\tspecies\te_value\tpercent_identity\talign_len\tstrands\tquery_id\tquery_from\tquery_to\ttarget_from\ttarget_to\tbitscore\tidentical\tpositives\tgaps\tquery_length\ttarget_length\tgene_report_url")
	for i, row := range rows {
		marker := "[ ]"
		if i < len(selected) && selected[i] {
			marker = "[x]"
		}
		fmt.Fprintf(
			writer,
			"%s\t%d\t%s\t%s\t%s\t%.2f\t%d\t%s\t%s\t%d\t%d\t%d\t%d\t%.2f\t%d\t%d\t%d\t%d\t%d\t%s\n",
			marker,
			i+1,
			row.Protein,
			row.Species,
			row.EValue,
			row.PercentIdentity,
			row.AlignLength,
			row.Strands,
			row.QueryID,
			row.QueryFrom,
			row.QueryTo,
			row.TargetFrom,
			row.TargetTo,
			row.Bitscore,
			row.Identical,
			row.Positives,
			row.Gaps,
			row.QueryLength,
			row.TargetLength,
			row.GeneReportURL,
		)
	}
	_ = writer.Flush()
}

func mapNavigationError(err error, backTarget error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errBackCommand):
		if backTarget != nil {
			return backTarget
		}
		return nil
	case errors.Is(err, errSpawnCommand):
		return ErrBackToModeSelection
	case errors.Is(err, errLobbyCommand):
		return ErrBackToDatabaseSelection
	case errors.Is(err, ErrExitRequested):
		return ErrExitRequested
	default:
		return err
	}
}

func (p *Prompter) ChooseDatabase() (string, error) {
	for {
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, p.t("Database selection:"))
		fmt.Fprintln(p.out, p.t(" 1) phytozome - original Phytozome workflow"))
		fmt.Fprintln(p.out, p.t(" 2) lemna     - lemna.org download-backed workflow"))
		printGlobalCommandHint(p.out)

		value, err := p.readLine(p.t("Select 1 or 2 (or 'phytozome'/'lemna'): "))
		if err != nil {
			if mapped := mapNavigationError(err, ErrBackToDatabaseSelection); mapped != nil {
				return "", mapped
			}
			continue
		}
		if isHelpCommand(value) {
			p.printDatabaseHelp()
			continue
		}

		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "phytozome", "p":
			return "phytozome", nil
		case "2", "lemna", "l":
			return "lemna", nil
		default:
			fmt.Fprintln(p.out, p.t("Please enter one of: 1, 2, 'phytozome', or 'lemna'."))
		}
	}
}

func (p *Prompter) ChooseMode() (string, error) {
	for {
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, p.t("Mode selection:"))
		fmt.Fprintln(p.out, p.t(" 1) blast   - sequence / FASTA / URL query against one species"))
		fmt.Fprintln(p.out, p.t(" 2) keyword - keyword gene search within one species"))
		printGlobalCommandHint(p.out)

		mode, err := p.readLine(p.t("Select 1 or 2 (or 'blast'/'keyword'): "))
		if err != nil {
			if mapped := mapNavigationError(err, ErrBackToDatabaseSelection); mapped != nil {
				return "", mapped
			}
			continue
		}
		if isHelpCommand(mode) {
			p.printModeHelp()
			continue
		}

		switch strings.ToLower(strings.TrimSpace(mode)) {
		case "1", "blast", "b":
			return "blast", nil
		case "2", "keyword", "k":
			return "keyword", nil
		default:
			fmt.Fprintln(p.out, p.t("Please enter one of: 1, 2, 'blast', or 'keyword'."))
		}
	}
}

// ChooseBlastProgram prompts the user to pick one BLAST program from the
// provided list of program names. The prompt accepts either a program number
// (1-based) or the program name (case-insensitive). Returns the selected
// program string as given in the `programs` slice.
func (p *Prompter) ChooseBlastProgram(programs []string) (string, error) {
	if len(programs) == 0 {
		return "", fmt.Errorf("no BLAST programs available")
	}

	for {
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, p.t("BLAST program selection:"))
		printBlastProgramGroups(p.out, programs)
		printGlobalCommandHint(p.out)
		value, err := p.readLine(p.t("Select a program by number or name (type help for details): "))
		if err != nil {
			if mapped := mapNavigationError(err, ErrBackToQueryInput); mapped != nil {
				return "", mapped
			}
			continue
		}
		if isHelpCommand(value) {
			fmt.Fprintln(p.out, p.t("Programs are grouped by query type."))
			fmt.Fprintln(p.out, p.t("Use the nucleotide-query programs for DNA/RNA input."))
			fmt.Fprintln(p.out, p.t("Use the protein-query programs for amino-acid input."))
			fmt.Fprintln(p.out, p.t("Examples: '1', 'blastn', or 'blastp'."))
			continue
		}

		trim := strings.TrimSpace(value)
		// Try numeric selection first.
		if idx, err := strconv.Atoi(trim); err == nil {
			if idx >= 1 && idx <= len(programs) {
				return programs[idx-1], nil
			}
			fmt.Fprintln(p.out, "Number out of range. Choose one of the listed numbers.")
			continue
		}

		// Try exact name match (case-insensitive).
		lower := strings.ToLower(trim)
		for _, prog := range programs {
			if strings.ToLower(prog) == lower {
				return prog, nil
			}
		}

		// Try prefix or contains match to be user friendly.
		matches := make([]string, 0, len(programs))
		for _, prog := range programs {
			if strings.Contains(strings.ToLower(prog), lower) || strings.HasPrefix(strings.ToLower(prog), lower) {
				matches = append(matches, prog)
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			fmt.Fprintln(p.out, p.t("Ambiguous program name; multiple candidates match:"))
			for _, m := range matches {
				fmt.Fprintf(p.out, "  - %s\n", m)
			}
			fmt.Fprintln(p.out, p.t("Please enter a number or a more specific program name."))
			continue
		}

		fmt.Fprintln(p.out, p.t("Unknown program. Enter a listed number or program name."))
	}
}

func printBlastProgramGroups(out io.Writer, programs []string) {
	numbered := make([]string, 0, len(programs))
	for i, prog := range programs {
		numbered = append(numbered, fmt.Sprintf(" %d) %s - %s", i+1, prog, locale.Text(activeLanguage, blastProgramDescription(prog))))
	}

	printed := make(map[string]bool, len(programs))
	printGroup := func(title string, wanted ...string) {
		groupPrinted := false
		for i, prog := range programs {
			for _, want := range wanted {
				if strings.EqualFold(prog, want) {
					if !groupPrinted {
						fmt.Fprintf(out, " %s:\n", title)
						groupPrinted = true
					}
					fmt.Fprintln(out, numbered[i])
					printed[strings.ToLower(prog)] = true
				}
			}
		}
	}

	printGroup(locale.Text(activeLanguage, "Nucleotide query starts here"), "blastn", "blastx")
	printGroup(locale.Text(activeLanguage, "Protein query starts here"), "tblastn", "blastp")

	for i, prog := range programs {
		if !printed[strings.ToLower(prog)] {
			fmt.Fprintln(out, numbered[i])
		}
	}
}

func blastProgramDescription(program string) string {
	switch strings.ToLower(strings.TrimSpace(program)) {
	case "blastn":
		return "nucleotide query -> nucleotide/genome database"
	case "blastx":
		return "nucleotide query -> translated protein -> protein database"
	case "tblastn":
		return "protein query -> translated nucleotide/genome database"
	case "blastp":
		return "protein query -> protein database"
	default:
		return "BLAST search program"
	}
}

// ChooseBlastExecution asks whether to run the BLAST job on the server or
// locally. Returns \"server\" or \"local\". Accepts numeric choice or name.
func (p *Prompter) ChooseBlastExecution() (string, error) {
	for {
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, p.t("BLAST execution target:"))
		fmt.Fprintln(p.out, p.t(" 1) server - try the remote lemna.org BLAST service first"))
		fmt.Fprintln(p.out, p.t(" 2) local  - download the lemna FASTA files automatically and run BLAST on this computer"))
		fmt.Fprintln(p.out, p.t("      Local mode does not require you to prepare the FASTA files yourself."))
		fmt.Fprintln(p.out, p.t("      It does require NCBI BLAST+ on PATH, including makeblastdb."))
		printGlobalCommandHint(p.out)

		value, err := p.readLine(p.t("Select 1 or 2 (or 'server'/'local'): "))
		if err != nil {
			if mapped := mapNavigationError(err, ErrBackToBlastProgram); mapped != nil {
				return "", mapped
			}
			continue
		}
		if isHelpCommand(value) {
			fmt.Fprintln(p.out, p.t("server: use the lemna.org website if the needed database is exposed there."))
			fmt.Fprintln(p.out, p.t("local: the CLI downloads FASTA files from lemna.org into a local cache, builds a BLAST database, and runs BLAST on your machine."))
			fmt.Fprintln(p.out, p.t("You do not need to prepare the data files yourself, but BLAST+ must be installed."))
			continue
		}

		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "server", "s":
			return "server", nil
		case "2", "local", "l":
			return "local", nil
		default:
			fmt.Fprintln(p.out, p.t("Please enter one of: 1, 2, 'server', or 'local'."))
		}
	}
}

func (p *Prompter) SpeciesKeyword() (string, error) {
	for {
		printGlobalCommandHint(p.out)
		value, err := p.readLine(p.t("Enter a species keyword: "))
		if err != nil {
			if mapped := mapNavigationError(err, ErrBackToModeSelection); mapped != nil {
				return "", mapped
			}
			continue
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
		fmt.Fprintf(p.out, "   %s%s\n", candidate.JBrowseName, targetIDLabel(candidate.ProteomeID))
	}
	printGlobalCommandHint(p.out)

	for {
		value, err := p.readLine(p.t("Select one species by number: "))
		if err != nil {
			if mapped := mapNavigationError(err, ErrBackToModeSelection); mapped != nil {
				return model.SpeciesCandidate{}, mapped
			}
			continue
		}
		if isHelpCommand(value) {
			p.printSpeciesChooseHelp()
			continue
		}

		index, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || index < 1 || index > len(candidates) {
			fmt.Fprintln(p.out, p.t("Invalid selection. Enter one of the numbers above."))
			continue
		}

		return candidates[index-1], nil
	}
}

func (p *Prompter) KeywordProteinIdentifications(termCount int) ([]string, error) {
	if termCount <= 0 {
		return nil, nil
	}

	for {
		fmt.Fprintln(p.out, p.t("Protein identification labels:"))
		fmt.Fprintln(p.out, p.t(" Enter one label per search term."))
		fmt.Fprintln(p.out, p.t(" Use ~ for a blank label."))
		fmt.Fprintln(p.out, p.t(" Press Enter on the first line to skip all labels."))
		printGlobalCommandHint(p.out)

		lines := make([]string, 0, termCount)
		for {
			line, err := p.in.ReadString('\n')
			if err != nil && err != io.EOF {
				return nil, err
			}

			line = strings.TrimSpace(line)
			if p.applyLanguageCommand(line) {
				continue
			}
			if len(lines) == 0 && line == "" {
				return nil, nil
			}
			if line == "" {
				break
			}
			if len(lines) == 0 {
				switch strings.ToLower(line) {
				case "back":
					return nil, ErrBackToSpeciesSelection
				case "spawn":
					return nil, ErrBackToModeSelection
				case "lobby":
					return nil, ErrBackToDatabaseSelection
				case "exit":
					return nil, ErrExitRequested
				}
			}
			lines = append(lines, line)
			if err == io.EOF {
				break
			}
		}

		values := parseKeywordIdentityValues(lines)
		if len(values) != termCount {
			fmt.Fprintf(p.out, p.t("Need exactly %d Protein Identification values, got %d. Please re-enter.\n"), termCount, len(values))
			continue
		}
		return values, nil
	}
}

func (p *Prompter) BlastProteinIdentifications(itemCount int, required bool) ([]string, error) {
	if itemCount <= 0 {
		return nil, nil
	}

	for {
		fmt.Fprintln(p.out, p.t("Protein identification labels:"))
		if itemCount == 1 {
			fmt.Fprintln(p.out, p.t(" Enter one label for this BLAST query, or press Enter to skip."))
		} else {
			fmt.Fprintln(p.out, locale.Sprintf(p.lang, " Enter exactly %d labels, one per line.\n", itemCount))
			fmt.Fprintln(p.out, p.t(" Use ~ for a blank label."))
		}
		fmt.Fprintln(p.out, p.t(" Finish with an empty line."))
		printGlobalCommandHint(p.out)

		lines := make([]string, 0, itemCount)
		for {
			line, err := p.in.ReadString('\n')
			if err != nil && err != io.EOF {
				return nil, err
			}

			line = strings.TrimSpace(line)
			if p.applyLanguageCommand(line) {
				continue
			}
			if len(lines) == 0 && line == "" {
				if required {
					fmt.Fprintln(p.out, p.t("Protein Identification is required for this input. Please enter one label per query."))
					break
				}
				return nil, nil
			}
			if line == "" {
				break
			}
			if len(lines) == 0 {
				switch strings.ToLower(line) {
				case "back":
					return nil, ErrBackToSpeciesSelection
				case "spawn":
					return nil, ErrBackToModeSelection
				case "lobby":
					return nil, ErrBackToDatabaseSelection
				case "exit":
					return nil, ErrExitRequested
				}
			}
			lines = append(lines, line)
			if err == io.EOF {
				break
			}
		}

		values := parseBlastIdentityValues(lines)
		if len(values) != itemCount {
			fmt.Fprintf(p.out, p.t("Need exactly %d Protein Identification values, got %d. Please re-enter.\n"), itemCount, len(values))
			continue
		}
		return values, nil
	}
}

func (p *Prompter) OutputFolderName() (string, error) {
	for {
		fmt.Fprintln(p.out, p.t("Output folder (optional)."))
		fmt.Fprintln(p.out, p.t(" Leave blank to write next to the program."))
		printGlobalCommandHint(p.out)

		value, err := p.readLine(p.t("Enter a folder name or press Enter: "))
		if err != nil {
			if mapped := mapNavigationError(err, ErrBackToQueryInput); mapped != nil {
				return "", mapped
			}
			continue
		}
		if isHelpCommand(value) {
			fmt.Fprintln(p.out, p.t("A folder name keeps all generated files together."))
			fmt.Fprintln(p.out, p.t("Leave it blank to write files next to the program."))
			continue
		}

		return strings.TrimSpace(value), nil
	}
}

func (p *Prompter) ConfirmReportPreview() error {
	for {
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, p.t("Review the summary and press Enter to continue."))
		printGlobalCommandHint(p.out)
		value, err := p.readLine(p.t("Press Enter to continue: "))
		if err != nil {
			return err
		}
		if strings.TrimSpace(value) == "" {
			return nil
		}
		if isHelpCommand(value) {
			fmt.Fprintln(p.out, p.t("Press Enter to continue."))
			fmt.Fprintln(p.out, p.t("Or use back, spawn, lobby, or exit to navigate away."))
			continue
		}
		fmt.Fprintln(p.out, p.t("Press Enter to continue."))
	}
}

func (p *Prompter) DetailedReportAction() (string, error) {
	for {
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, p.t("Detailed run log?"))
		fmt.Fprintln(p.out, p.t(" 1) yes - write a timestamped log next to the generated files"))
		fmt.Fprintln(p.out, p.t(" 2) no  - skip the detailed log"))
		printGlobalCommandHint(p.out)
		value, err := p.readLine(p.t("Select 1 or 2 (or 'yes'/'no'): "))
		if err != nil {
			return "", err
		}
		if isHelpCommand(value) {
			fmt.Fprintln(p.out, p.t("yes writes a detailed log after file generation."))
			fmt.Fprintln(p.out, p.t("no skips the extra log."))
			continue
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "yes", "y":
			return "yes", nil
		case "2", "no", "n":
			return "no", nil
		default:
			fmt.Fprintln(p.out, p.t("Please enter one of: 1, 2, yes, or no."))
		}
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
			fmt.Fprintf(p.out, p.t("No species candidates matched %q.\n"), keyword)
			keyword = ""
			continue
		}

		if len(filtered) > 12 {
			filtered = filtered[:12]
		}

		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, p.t("Candidate species for %q:\n"), keyword)
		for i, candidate := range filtered {
			fmt.Fprintf(p.out, "%d. %s\n", i+1, candidate.DisplayLabel())
			fmt.Fprintf(p.out, "   %s%s\n", candidate.JBrowseName, targetIDLabel(candidate.ProteomeID))
		}
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, p.t("Enter a number to choose one candidate."))
		fmt.Fprintln(p.out, p.t("Or enter another keyword to search again."))
		printGlobalCommandHint(p.out)

		value, err := p.readLine(p.t("Select a species or search again: "))
		if err != nil {
			if mapped := mapNavigationError(err, ErrBackToModeSelection); mapped != nil {
				return model.SpeciesCandidate{}, mapped
			}
			continue
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
			fmt.Fprintln(p.out, p.t("Please enter a number or another keyword."))
			continue
		}

		keyword = value
	}
}

func (p *Prompter) SequenceInput() (string, error) {
	fmt.Fprintln(p.out, p.t("Paste one or more BLAST queries, one per line, or paste a FASTA entry / Phytozome gene or transcript report URL."))
	fmt.Fprintln(p.out, p.t("You can also paste a keyword-mode list preview with ~~ in the middle, or type load \"file.txt\" to read from the program directory."))
	fmt.Fprintln(p.out, p.t("Finish sequence input with an empty line."))
	printGlobalCommandHint(p.out)

	lines := make([]string, 0, 8)
	for {
		line, err := p.in.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}

		line = strings.TrimSpace(line)
		if p.applyLanguageCommand(line) {
			continue
		}
		if len(lines) == 0 {
			switch strings.ToLower(line) {
			case "back":
				return "", ErrBackToSpeciesSelection
			case "spawn":
				return "", ErrBackToModeSelection
			case "lobby":
				return "", ErrBackToDatabaseSelection
			case "exit":
				return "", ErrExitRequested
			}
		}
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

func targetIDLabel(targetID int) string {
	if targetID == 0 {
		return ""
	}
	return fmt.Sprintf(" (target id %d)", targetID)
}

func (p *Prompter) KeywordInput() (string, error) {
	fmt.Fprintln(p.out, p.t("Paste one or more keywords for the selected species."))
	fmt.Fprintln(p.out, p.t("Separate them by spaces or new lines, then finish with an empty line."))
	printGlobalCommandHint(p.out)

	lines := make([]string, 0, 4)
	for {
		line, err := p.in.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}

		line = strings.TrimSpace(line)
		if p.applyLanguageCommand(line) {
			continue
		}
		if len(lines) == 0 {
			switch strings.ToLower(line) {
			case "back":
				return "", ErrBackToSpeciesSelection
			case "spawn":
				return "", ErrBackToModeSelection
			case "lobby":
				return "", ErrBackToDatabaseSelection
			case "exit":
				return "", ErrExitRequested
			}
		}
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

func (p *Prompter) SelectKeywordRows(groups []model.KeywordSearchGroup, listPath string, writeListFn func(string, []model.KeywordResultRow) error) ([]model.KeywordResultRow, error) {
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
		fmt.Fprintln(p.out, p.t("Keyword results:"))
		fmt.Fprintln(p.out, "sel\trow\tsearch_term\tprotein_identification\ttranscript\tgene_identifier\tgenome\tlocation\talias\tuniprot\tdescription\tauto_define\tgene_report_url")

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
					"%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					marker,
					displayIndex+1,
					row.SearchTerm,
					row.ProteinIdentification,
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
		printSelectionCommands(p.out, true)
		printGlobalCommandHint(p.out)

		input, err := p.readLine(p.t("Selection command (all/none/toggle/on/off/done, plus back/spawn/lobby/exit): "))
		if err != nil {
			if mapped := mapNavigationError(err, ErrBackToQueryInput); mapped != nil {
				return nil, mapped
			}
			continue
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
		case "list":
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
			printKeywordListPreview(p.out, chosen)
			for {
				printListOutputCommands(p.out)
				printGlobalCommandHint(p.out)
				action, err := p.readLine("List action (back - return to the table, txt - write the _list file, exit - quit): ")
				if err != nil {
					if mapped := mapNavigationError(err, ErrBackToQueryInput); mapped != nil {
						return nil, mapped
					}
					continue
				}
				switch strings.ToLower(strings.TrimSpace(action)) {
				case "back":
					goto CONTINUE_SELECTION
				case "txt":
					if writeListFn != nil {
						if err := writeListFn(listPath, chosen); err != nil {
							fmt.Fprintf(p.out, "Failed to write list text file: %v\n", err)
							continue
						}
						fmt.Fprintf(p.out, "List text file written: %s\n", listPath)
					}
					goto CONTINUE_SELECTION
				case "exit":
					return nil, ErrExitRequested
				default:
					fmt.Fprintln(p.out, "Please enter 'back', 'txt', or 'exit'.")
				}
			}
		CONTINUE_SELECTION:
			continue
		case "back":
			return nil, ErrBackToQueryInput
		case "spawn":
			return nil, ErrBackToModeSelection
		case "lobby":
			return nil, ErrBackToDatabaseSelection
		case "exit":
			return nil, ErrExitRequested
		default:
			fmt.Fprintln(p.out, "Unknown command.")
		}
	}
}

func (p *Prompter) SelectBlastRows(rows []model.BlastResultRow) ([]model.BlastResultRow, error) {
	selectedRows, _, err := p.selectBlastRows(rows, false)
	return selectedRows, err
}

func (p *Prompter) SelectBlastRowsBatch(rows []model.BlastResultRow) ([]model.BlastResultRow, bool, error) {
	return p.selectBlastRows(rows, true)
}

func (p *Prompter) selectBlastRows(rows []model.BlastResultRow, allowDoneAll bool) ([]model.BlastResultRow, bool, error) {
	if len(rows) == 0 {
		return nil, false, nil
	}

	selected := make([]bool, len(rows))
	for i := range selected {
		selected[i] = true
	}
	order := defaultRowOrder(len(rows))

	for {
		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, "BLAST row selection: %d/%d rows currently selected.\n", countSelected(selected), len(rows))
		fmt.Fprintln(p.out, "Use the table below to review and change the current selection.")
		fmt.Fprintln(p.out)
		printBlastSelectionTable(p.out, rows, selected)
		fmt.Fprintln(p.out)
		printSelectionCommands(p.out, false)
		if allowDoneAll {
			fmt.Fprintln(p.out, "  done all | doneall - confirm this selection and auto-approve the remaining BLAST queries")
		}
		printGlobalCommandHint(p.out)

		promptLabel := "Selection command (all/none/toggle/on/off/done, done all, plus back/spawn/lobby/exit): "
		if allowDoneAll {
			promptLabel = "Selection command (all/none/toggle/on/off/done, done all/doneall, plus back/spawn/lobby/exit): "
		}
		input, err := p.readLine(promptLabel)
		if err != nil {
			if mapped := mapNavigationError(err, ErrBackToQueryInput); mapped != nil {
				return nil, false, mapped
			}
			continue
		}
		if isHelpCommand(input) {
			if allowDoneAll {
				p.printBlastBatchSelectionHelp()
			} else {
				p.printBlastSelectionHelp()
			}
			continue
		}

		fields := strings.Fields(strings.ToLower(strings.TrimSpace(input)))
		if len(fields) == 0 {
			continue
		}

		switch {
		case len(fields) == 2 && fields[0] == "done" && fields[1] == "all", len(fields) == 1 && fields[0] == "doneall":
			if !allowDoneAll {
				fmt.Fprintln(p.out, "Unknown command.")
				continue
			}
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
			return chosen, true, nil
		case fields[0] == "all":
			for i := range selected {
				selected[i] = true
			}
		case fields[0] == "none":
			for i := range selected {
				selected[i] = false
			}
		case fields[0] == "toggle":
			if len(fields) == 1 {
				fmt.Fprintln(p.out, "Provide one or more row numbers or ranges after 'toggle'.")
				continue
			}
			if err := toggleSelections(selected, order, fields[1:]); err != nil {
				fmt.Fprintf(p.out, "Invalid toggle command: %v\n", err)
			}
		case fields[0] == "on", fields[0] == "off", fields[0] == "select", fields[0] == "unselect":
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
		case fields[0] == "done":
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
			return chosen, false, nil
		case fields[0] == "back":
			return nil, false, ErrBackToQueryInput
		case fields[0] == "spawn":
			return nil, false, ErrBackToModeSelection
		case fields[0] == "lobby":
			return nil, false, ErrBackToDatabaseSelection
		case fields[0] == "exit":
			return nil, false, ErrExitRequested
		default:
			fmt.Fprintln(p.out, "Unknown command.")
		}
	}
}

func (p *Prompter) ExportBaseName(label string, backTarget error) (string, error) {
	for {
		promptLabel := strings.TrimSpace(label)
		if promptLabel == "" {
			promptLabel = "Export file name"
		}
		printGlobalCommandHint(p.out)
		value, err := p.readLine(promptLabel + " (without extension): ")
		if err != nil {
			if mapped := mapNavigationError(err, backTarget); mapped != nil {
				return "", mapped
			}
			continue
		}
		if isHelpCommand(value) {
			p.printExportNameHelp()
			continue
		}

		name := sanitizeFileName(value)
		if name == "" {
			fmt.Fprintln(p.out, p.t("File name cannot be empty."))
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
		fmt.Fprintln(p.out, p.t("What would you like to do next?"))
		fmt.Fprintf(p.out, p.t(" 1) Run %s again with the same species\n"), modeLabel)
		fmt.Fprintf(p.out, p.t(" 2) Change species and run %s\n"), modeLabel)
		fmt.Fprintln(p.out, p.t(" 3) Switch mode"))
		fmt.Fprintln(p.out, p.t(" 4) Exit"))

		input, err := p.readLine(p.t("Choose 1, 2, 3, or 4 (or 'repeat'/'change'/'mode'/'exit'): "))
		if err != nil {
			switch {
			case errors.Is(err, errBackCommand):
				return "change_species", nil
			case errors.Is(err, errSpawnCommand):
				return "change_mode", nil
			case errors.Is(err, errLobbyCommand):
				return "", ErrBackToDatabaseSelection
			case errors.Is(err, ErrExitRequested):
				return "exit", nil
			default:
				continue
			}
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
		case "back", "spawn", "lobby":
			switch val {
			case "back":
				return "change_species", nil
			case "spawn":
				return "change_mode", nil
			case "lobby":
				return "exit", nil
			}
		default:
			fmt.Fprintln(p.out, p.t("Please enter one of: 1, 2, 3, 4, or 'repeat', 'change', 'mode', 'exit', 'back', 'spawn', 'lobby'."))
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

func countSelected(selected []bool) int {
	total := 0
	for _, value := range selected {
		if value {
			total++
		}
	}
	return total
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
//   - "back"  : return to the caller-provided previous page
//   - "exit"  : stop the wizard entirely
func (p *Prompter) FetchErrorAction(description string, backTarget error) (string, error) {
	for {
		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, "Failed to fetch: %s\n", description)
		fmt.Fprintln(p.out, "Options:")
		fmt.Fprintln(p.out, " 1) retry  - try again now")
		fmt.Fprintln(p.out, " 2) skip   - skip this record and continue")
		fmt.Fprintln(p.out, " 3) back   - return to the previous page")
		fmt.Fprintln(p.out, " 4) exit   - stop the wizard")
		printGlobalCommandHint(p.out)
		input, err := p.readLine("Choose 1, 2, 3, or 4 (or 'retry'/'skip'/'back'/'exit'): ")
		if err != nil {
			if mapped := mapNavigationError(err, backTarget); mapped != nil {
				return "", mapped
			}
			continue
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
		case "3", "back", "b":
			return "back", nil
		case "4", "exit", "e", "q":
			return "exit", nil
		default:
			fmt.Fprintln(p.out, "Please enter one of: 1, 2, 3, 4, or 'retry', 'skip', 'back', 'exit'.")
		}
	}
}

// WorkflowErrorAction prompts the user when a higher-level wizard step fails.
// It returns one of:
//   - "retry" : attempt the step again
//   - "exit"  : stop the wizard
func (p *Prompter) WorkflowErrorAction(description string, backTarget error) (string, error) {
	for {
		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, "Step failed: %s\n", description)
		fmt.Fprintln(p.out, "Options:")
		fmt.Fprintln(p.out, " 1) retry - try this step again")
		fmt.Fprintln(p.out, " 2) back  - return to the previous page")
		fmt.Fprintln(p.out, " 3) exit  - stop the wizard")
		printGlobalCommandHint(p.out)
		input, err := p.readLine("Select 1, 2, or 3 (or 'retry'/'back'/'exit'): ")
		if err != nil {
			if mapped := mapNavigationError(err, backTarget); mapped != nil {
				return "", mapped
			}
			continue
		}
		if isHelpCommand(input) {
			p.printWorkflowErrorHelp()
			continue
		}
		val := strings.ToLower(strings.TrimSpace(input))
		switch val {
		case "1", "retry", "r":
			return "retry", nil
		case "2", "back", "b":
			return "back", nil
		case "3", "exit", "e", "q":
			return "exit", nil
		default:
			fmt.Fprintln(p.out, "Please enter one of: 1, 2, 3, 'retry', 'back', or 'exit'.")
		}
	}
}

// BlastSubmitErrorAction prompts the user after a BLAST submission failure.
// It returns:
//   - "retry" : retry the same program/execution path
//   - "back"  : return to BLAST program selection
//   - "exit"  : stop the wizard
func (p *Prompter) BlastSubmitErrorAction(description string) (string, error) {
	for {
		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, "Step failed: %s\n", description)
		fmt.Fprintln(p.out, "Options:")
		fmt.Fprintln(p.out, " 1) retry - try the same BLAST submission again")
		fmt.Fprintln(p.out, " 2) back  - choose BLAST program / execution target again")
		fmt.Fprintln(p.out, " 3) exit  - stop the wizard")
		printGlobalCommandHint(p.out)
		input, err := p.readLine("Choose 1, 2, or 3 (or 'retry'/'back'/'exit'): ")
		if err != nil {
			if mapped := mapNavigationError(err, ErrBackToBlastProgram); mapped != nil {
				return "", mapped
			}
			continue
		}
		if isHelpCommand(input) {
			fmt.Fprintln(p.out, "retry keeps the current program and execution target.")
			fmt.Fprintln(p.out, "back returns to BLAST program selection without re-entering the query sequence.")
			fmt.Fprintln(p.out, "exit stops the wizard.")
			continue
		}
		switch strings.ToLower(strings.TrimSpace(input)) {
		case "1", "retry", "r":
			return "retry", nil
		case "2", "back", "b":
			return "back", nil
		case "3", "exit", "e", "q":
			return "exit", nil
		default:
			fmt.Fprintln(p.out, "Please enter one of: 1, 2, 3, 'retry', 'back', or 'exit'.")
		}
	}
}

// BlastPlusInstallAction prompts the user when local BLAST needs BLAST+.
// It returns:
//   - "install" : download and install managed BLAST+ for this app
//   - "back"    : return to BLAST program selection
//   - "exit"    : stop the wizard
func (p *Prompter) BlastPlusInstallAction(description string) (string, error) {
	for {
		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, "BLAST+ is required for local BLAST: %s\n", description)
		fmt.Fprintln(p.out, "Options:")
		fmt.Fprintln(p.out, " 1) install - download official NCBI BLAST+ for this app now")
		fmt.Fprintln(p.out, " 2) back    - choose BLAST program / execution target again")
		fmt.Fprintln(p.out, " 3) exit    - stop the wizard")
		printGlobalCommandHint(p.out)
		input, err := p.readLine("Choose 1, 2, or 3 (or 'install'/'back'/'exit'): ")
		if err != nil {
			if mapped := mapNavigationError(err, ErrBackToBlastProgram); mapped != nil {
				return "", mapped
			}
			continue
		}
		if isHelpCommand(input) {
			fmt.Fprintln(p.out, "install downloads the official BLAST+ archive into a blastplus/ directory next to the running executable.")
			fmt.Fprintln(p.out, "You do not need to prepare FASTA files yourself; the app already downloads those from lemna.org when needed.")
			continue
		}
		switch strings.ToLower(strings.TrimSpace(input)) {
		case "1", "install", "i":
			return "install", nil
		case "2", "back", "b":
			return "back", nil
		case "3", "exit", "e", "q":
			return "exit", nil
		default:
			fmt.Fprintln(p.out, "Please enter one of: 1, 2, 3, 'install', 'back', or 'exit'.")
		}
	}
}

func (p *Prompter) readLine(label string) (string, error) {
	fmt.Fprint(p.out, label)
	line, err := p.in.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			line = strings.TrimSpace(line)
			if p.applyLanguageCommand(line) {
				return "", nil
			}
			switch strings.ToLower(line) {
			case "back":
				return "", errBackCommand
			case "spawn":
				return "", errSpawnCommand
			case "lobby":
				return "", errLobbyCommand
			case "exit":
				return "", ErrExitRequested
			}
			return line, nil
		}
		return "", err
	}
	line = strings.TrimSpace(line)
	if p.applyLanguageCommand(line) {
		return "", nil
	}
	switch strings.ToLower(line) {
	case "back":
		return "", errBackCommand
	case "spawn":
		return "", errSpawnCommand
	case "lobby":
		return "", errLobbyCommand
	case "exit":
		return "", ErrExitRequested
	}
	return line, nil
}

func (p *Prompter) applyLanguageCommand(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(strings.ToLower(line), "lang=") {
		return false
	}
	if lang, ok := locale.Parse(strings.TrimSpace(line[5:])); ok {
		p.SetLanguage(lang)
		fmt.Fprintln(p.out, p.t("Language switched for subsequent prompts."))
		return true
	}
	fmt.Fprintln(p.out, p.t("Unknown language. Use lang=en, lang=cn, or lang=jp."))
	return true
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
	fmt.Fprintln(p.out, p.t("Help - species search"))
	fmt.Fprintln(p.out, p.t(" Enter a partial species keyword such as 'spiro', 'wheat', or 'arabidopsis'."))
	fmt.Fprintln(p.out, p.t(" You can search by abbreviated name, full scientific name, or common name."))
	fmt.Fprintln(p.out, p.t(" Type 'help' or '?' at this prompt to see this message again."))
}

func (p *Prompter) printDatabaseHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, p.t("Help - database selection"))
	fmt.Fprintln(p.out, p.t(" phytozome: keeps the existing Phytozome behavior."))
	fmt.Fprintln(p.out, p.t(" lemna: uses lemna.org species releases and GFF3/FASTA files from the Download area."))
	fmt.Fprintln(p.out, p.t(" The chosen database stays active for the current session."))
	printGlobalCommandHint(p.out)
}

func (p *Prompter) printModeHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, p.t("Help - mode selection"))
	fmt.Fprintln(p.out, p.t(" blast: sequence / FASTA / URL-based search workflow"))
	fmt.Fprintln(p.out, p.t(" keyword: keyword gene search and export workflow"))
	fmt.Fprintln(p.out, p.t(" The chosen mode stays active for the current session."))
	printGlobalCommandHint(p.out)
}

func (p *Prompter) printSpeciesChooseHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, p.t("Help - species selection"))
	fmt.Fprintln(p.out, p.t(" Enter a numbered candidate to choose that species."))
	fmt.Fprintln(p.out, p.t(" Or enter another keyword to run a new search."))
	fmt.Fprintln(p.out, p.t(" Type 'help' or '?' at this prompt to see this message again."))
	printGlobalCommandHint(p.out)
}

func (p *Prompter) printSequenceInputHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, p.t("Help - BLAST input"))
	fmt.Fprintln(p.out, p.t(" Accepted inputs:"))
	fmt.Fprintln(p.out, p.t("  1) one query per line for batch BLAST runs"))
	fmt.Fprintln(p.out, p.t("  2) plain sequence"))
	fmt.Fprintln(p.out, p.t("  3) FASTA-style header plus sequence"))
	fmt.Fprintln(p.out, p.t("  4) supported Phytozome gene/transcript report URL"))
	fmt.Fprintln(p.out, p.t("  5) a keyword-mode list preview copied from the list command, with ~~ between labels and links"))
	fmt.Fprintln(p.out, p.t("  6) load \"file.txt\" to read the same kinds of inputs from the program directory"))
	fmt.Fprintln(p.out, p.t(" Finish with an empty line."))
	fmt.Fprintln(p.out, p.t(" Single-line FASTA entries and trailing labels like '(AtC4H)' are accepted."))
	printGlobalCommandHint(p.out)
}

func (p *Prompter) printKeywordInputHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, p.t("Help - keyword input"))
	fmt.Fprintln(p.out, p.t(" Enter one or more keywords for the selected species."))
	fmt.Fprintln(p.out, p.t(" You can separate them by spaces or by new lines."))
	fmt.Fprintln(p.out, p.t(" Finish with an empty line."))
	printGlobalCommandHint(p.out)
}

func (p *Prompter) printBlastSelectionHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, p.t("Help - BLAST row selection"))
	fmt.Fprintln(p.out, p.t(" all - select every row"))
	fmt.Fprintln(p.out, p.t(" none - clear all selections"))
	fmt.Fprintln(p.out, p.t(" toggle 1 2 3 5~8 - flip the selection state for the listed rows or ranges"))
	fmt.Fprintln(p.out, p.t(" on 5~8 | off 5~8 - select or clear a specific row range"))
	fmt.Fprintln(p.out, p.t(" on up 12 | off up 12 - select or clear rows from the top through row 12"))
	fmt.Fprintln(p.out, p.t(" on down 12 | off down 12 - select or clear rows from row 12 through the bottom"))
	fmt.Fprintln(p.out, p.t(" done - confirm the current row selection"))
	fmt.Fprintln(p.out, p.t(" back - return to the previous page"))
	fmt.Fprintln(p.out, p.t(" spawn - jump back to mode selection"))
	fmt.Fprintln(p.out, p.t(" lobby - jump back to database selection"))
	fmt.Fprintln(p.out, p.t(" exit - quit the program"))
	fmt.Fprintln(p.out, p.t(" All row numbers refer to the BLAST results table shown below."))
}

func (p *Prompter) printBlastBatchSelectionHelp() {
	p.printBlastSelectionHelp()
	fmt.Fprintln(p.out, p.t(" done all | doneall - confirm this selection and auto-approve the remaining BLAST queries"))
}

func (p *Prompter) printKeywordSelectionHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, p.t("Help - keyword row selection"))
	fmt.Fprintln(p.out, p.t(" all - select every row"))
	fmt.Fprintln(p.out, p.t(" none - clear all selections"))
	fmt.Fprintln(p.out, p.t(" toggle 1 2 3 5~8 - flip the selection state for the listed rows or ranges"))
	fmt.Fprintln(p.out, p.t(" on 5~8 | off 5~8 - select or clear a specific row range"))
	fmt.Fprintln(p.out, p.t(" on up 12 | off up 12 - select or clear rows from the top through row 12"))
	fmt.Fprintln(p.out, p.t(" on down 12 | off down 12 - select or clear rows from row 12 through the bottom"))
	fmt.Fprintln(p.out, p.t(" list - preview the currently selected rows and optionally write a _list file"))
	fmt.Fprintln(p.out, p.t(" done - confirm the current row selection"))
	fmt.Fprintln(p.out, p.t(" back - return to the previous page"))
	fmt.Fprintln(p.out, p.t(" spawn - jump back to mode selection"))
	fmt.Fprintln(p.out, p.t(" lobby - jump back to database selection"))
	fmt.Fprintln(p.out, p.t(" exit - quit the program"))
	fmt.Fprintln(p.out, p.t(" The first result under each search term is selected by default."))
}

func parseKeywordIdentityValues(lines []string) []string {
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		for _, token := range strings.Fields(line) {
			if token == "~" {
				values = append(values, "")
				continue
			}
			values = append(values, token)
		}
	}
	return values
}

func parseBlastIdentityValues(lines []string) []string {
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "~" {
			values = append(values, "")
			continue
		}
		values = append(values, strings.TrimSpace(line))
	}
	return values
}

func printKeywordListPreview(out io.Writer, rows []model.KeywordResultRow) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, locale.Text(activeLanguage, "Keyword list preview:"))
	for _, line := range buildKeywordPreviewLines(rows) {
		fmt.Fprintln(out, line)
	}
	fmt.Fprintln(out)
}

func buildKeywordPreviewLines(rows []model.KeywordResultRow) []string {
	countByTerm := make(map[string]int, len(rows))
	for _, row := range rows {
		countByTerm[strings.TrimSpace(row.SearchTerm)]++
	}

	lines := make([]string, 0, len(rows)*2+1)
	for _, row := range rows {
		label := strings.TrimSpace(row.ProteinIdentification)
		if label == "" {
			label = "~"
		}
		term := strings.TrimSpace(row.SearchTerm)
		if countByTerm[term] > 1 && term != "" {
			label += " (" + term + ")"
		}
		lines = append(lines, label)
	}
	lines = append(lines, "~~")
	for _, row := range rows {
		link := strings.TrimSpace(row.GeneReportURL)
		if link == "" {
			link = "~"
		}
		lines = append(lines, link)
	}
	return lines
}

func (p *Prompter) printExportNameHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, p.t("Help - export file name"))
	fmt.Fprintln(p.out, p.t(" Enter one base name without extension."))
	fmt.Fprintln(p.out, p.t(" The program will create both '<name>.xlsx' and '<name>.txt'."))
	fmt.Fprintln(p.out, p.t(" Invalid Windows filename characters will be replaced automatically."))
}

func (p *Prompter) printPostRunHelp(modeLabel string) {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, p.t("Help - next action"))
	fmt.Fprintf(p.out, p.t(" repeat - run %s again with the same species\n"), modeLabel)
	fmt.Fprintf(p.out, p.t(" change - go back to species search for %s\n"), modeLabel)
	fmt.Fprintln(p.out, p.t(" mode - switch between blast and keyword"))
	fmt.Fprintln(p.out, p.t(" exit - quit the wizard"))
	fmt.Fprintln(p.out, p.t(" back - same as change"))
	fmt.Fprintln(p.out, p.t(" spawn - same as mode"))
	fmt.Fprintln(p.out, p.t(" lobby - go back to database selection"))
	fmt.Fprintln(p.out, p.t(" Each option is also available as a global command with the same meaning."))
}

func (p *Prompter) printFetchErrorHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, p.t("Help - fetch error options"))
	fmt.Fprintln(p.out, p.t(" retry - try the same remote fetch again"))
	fmt.Fprintln(p.out, p.t(" skip - omit this record and continue"))
	fmt.Fprintln(p.out, p.t(" back - return to the previous page"))
	fmt.Fprintln(p.out, p.t(" exit - stop the current export/workflow"))
}

func (p *Prompter) printWorkflowErrorHelp() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, p.t("Help - workflow error options"))
	fmt.Fprintln(p.out, p.t(" retry - try the current step again"))
	fmt.Fprintln(p.out, p.t(" exit - stop the wizard"))
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

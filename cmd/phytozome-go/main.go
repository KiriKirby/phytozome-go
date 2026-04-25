package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/locale"
	"github.com/KiriKirby/phytozome-go/internal/workflow"
)

var version = "dev"

const displayName = "phytozome GO"
const author = "wangsychn"
const repoURL = "https://github.com/KiriKirby/phytozome-go"

func main() {
	lang := locale.DetectFromExecutable(os.Args[0])
	args := os.Args[1:]
	if argLang, found, kept := locale.DetectFromArgs(args); found {
		lang = argLang
		args = kept
	}

	if len(args) < 1 {
		runDesktopWizard(lang)
		return
	}

	switch args[0] {
	case "version", "--version", "-version":
		printVersion(lang)
	case "blast":
		runBlast(lang, args[1:])
	case "help", "--help", "-h":
		printUsage(lang)
	default:
		fmt.Fprintf(os.Stderr, "%s\n\n", locale.Text(lang, "unknown command: ")+args[0])
		printUsage(lang)
		os.Exit(1)
	}
}

func runDesktopWizard(lang locale.Language) {
	if err := runInteractiveWizard(true, lang); err != nil {
		os.Exit(1)
	}
}

func runBlast(lang locale.Language, args []string) {
	if len(args) == 0 {
		printBlastUsage(lang)
		return
	}

	switch args[0] {
	case "plan":
		printBlastPlan(lang)
	case "wizard":
		if err := runInteractiveWizard(false, lang); err != nil {
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "%s\n\n", locale.Sprintf(lang, "unknown blast subcommand: %s", args[0]))
		printBlastUsage(lang)
		os.Exit(1)
	}
}

func printBlastPlan(lang locale.Language) {
	printHeader()
	fmt.Println()
	fmt.Println(locale.Text(lang, "BLAST plan:"))
	fmt.Println(locale.Text(lang, "  1) choose a database and mode"))
	fmt.Println(locale.Text(lang, "  2) pick a species when needed"))
	fmt.Println(locale.Text(lang, "  3) paste a sequence, FASTA, URL, or keyword list"))
	fmt.Println(locale.Text(lang, "  4) review results, select rows, and export files"))
}

func runInteractiveWizard(waitForExit bool, lang locale.Language) error {
	printSessionBanner(lang, "interactive wizard")
	fmt.Println(locale.Text(lang, "Desktop mode: starting interactive wizard."))
	fmt.Println()

	wizard := workflow.NewBlastWizard(os.Stdout, lang)
	err := wizard.Run(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", locale.Sprintf(lang, "blast wizard failed: %v", err))
	}

	printSessionFooter(lang, err)
	if waitForExit {
		pauseForExit(lang)
	}
	return err
}

func printUsage(lang locale.Language) {
	printHeader()
	fmt.Println()
	fmt.Println(locale.Text(lang, "Usage:"))
	fmt.Println(locale.Text(lang, "  phytozome-go blast wizard"))
	fmt.Println(locale.Text(lang, "  phytozome-go blast plan"))
	fmt.Println(locale.Text(lang, "  phytozome-go version"))
}

func printBlastUsage(lang locale.Language) {
	fmt.Println(locale.Text(lang, "Usage:"))
	fmt.Println(locale.Text(lang, "  phytozome-go blast wizard"))
	fmt.Println(locale.Text(lang, "  phytozome-go blast plan"))
}

func pauseForExit(lang locale.Language) {
	fmt.Println()
	fmt.Print(locale.Text(lang, "Press Enter to exit..."))
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func printHeader() {
	fmt.Printf("%s %s\n", displayName, version)
	fmt.Printf("Author: %s\n", author)
	fmt.Printf("Repo:   %s\n", repoURL)
}

func printSessionBanner(lang locale.Language, label string) {
	line := strings.Repeat("=", 78)
	fmt.Println(line)
	fmt.Printf("%s\n", locale.Sprintf(lang, "%s session: %s", displayName, label))
	fmt.Printf("%s\n", locale.Sprintf(lang, "Started: %s", time.Now().Format("2006-01-02 15:04:05")))
	fmt.Printf("%s\n", locale.Sprintf(lang, "Author:  %s", author))
	fmt.Printf("%s\n", locale.Sprintf(lang, "Repo:    %s", repoURL))
	fmt.Println(line)
}

func printSessionFooter(lang locale.Language, err error) {
	line := strings.Repeat("=", 78)
	fmt.Println()
	fmt.Println(line)
	if err != nil {
		fmt.Println(locale.Text(lang, "Session ended with errors."))
	} else {
		fmt.Println(locale.Text(lang, "Session completed successfully."))
	}
	fmt.Println(line)
}

func printVersion(lang locale.Language) {
	// A compact version output for machine-friendly calls
	fmt.Printf("%s %s\n%s\n%s\n", displayName, version, locale.Sprintf(lang, "Author: %s", author), locale.Sprintf(lang, "Repo: %s", repoURL))
}

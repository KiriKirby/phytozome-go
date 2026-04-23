package main

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/KiriKirby/phytozome-go/internal/workflow"
)

var version = "dev"

const displayName = "phytozome GO"
const author = "王舒扬"
const repoURL = "https://github.com/KiriKirby/phytozome-go"

func main() {
	if len(os.Args) < 2 {
		runDesktopWizard()
		return
	}

	switch os.Args[1] {
	case "version", "--version", "-version":
		printVersion()
	case "blast":
		runBlast(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func runDesktopWizard() {
	printHeader()
	fmt.Println("Desktop mode: starting interactive wizard.")
	fmt.Println()

	wizard := workflow.NewBlastWizard(os.Stdout)
	if err := wizard.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "blast wizard failed: %v\n", err)
		pauseForExit()
		os.Exit(1)
	}

	pauseForExit()
}

func runBlast(args []string) {
	if len(args) == 0 {
		printBlastUsage()
		return
	}

	switch args[0] {
	case "plan":
		fmt.Println("BLAST workflow plan:")
		fmt.Println("1. Search species candidates")
		fmt.Println("2. Select one species")
		fmt.Println("3. Submit BLAST sequence")
		fmt.Println("4. Fetch full result table")
		fmt.Println("5. Add gene_report_url column")
		fmt.Println("6. Multi-select rows")
		fmt.Println("7. Export selected rows to .xlsx and .txt")
	case "wizard":
		wizard := workflow.NewBlastWizard(os.Stdout)
		if err := wizard.Run(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "blast wizard failed: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown blast subcommand: %s\n\n", args[0])
		printBlastUsage()
		os.Exit(1)
	}
}

func printUsage() {
	printHeader()
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  phytozome-go blast wizard")
	fmt.Println("  phytozome-go blast plan")
	fmt.Println("  phytozome-go version")
}

func printBlastUsage() {
	fmt.Println("Usage:")
	fmt.Println("  phytozome-go blast wizard")
	fmt.Println("  phytozome-go blast plan")
}

func pauseForExit() {
	fmt.Println()
	fmt.Print("Press Enter to exit...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func printHeader() {
	fmt.Printf("%s %s\n", displayName, version)
	fmt.Printf("Author: %s\n", author)
	fmt.Printf("Repo:   %s\n", repoURL)
}

func printVersion() {
	// A compact version output for machine-friendly calls
	fmt.Printf("%s %s\nAuthor: %s\nRepo: %s\n", displayName, version, author, repoURL)
}

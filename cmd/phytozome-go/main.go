package main

import (
	"context"
	"fmt"
	"os"

	"github.com/KiriKirby/phytozome-go/internal/workflow"
)

var version = "dev"

const displayName = "phytozome GO"
const author = "wangsychn"
const repoURL = "https://github.com/KiriKirby/phytozome-go"

func main() {
	args := os.Args[1:]

	if len(args) < 1 {
		runDesktopWizard()
		return
	}

	switch args[0] {
	case "version", "--version", "-version":
		printVersion()
	case "blast":
		runBlast(args[1:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

func runDesktopWizard() {
	if err := runInteractiveWizard(); err != nil {
		os.Exit(1)
	}
}

func runBlast(args []string) {
	if len(args) == 0 {
		printBlastUsage()
		return
	}

	switch args[0] {
	case "plan":
		printBlastPlan()
	case "wizard":
		if err := runInteractiveWizard(); err != nil {
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown blast subcommand: %s\n\n", args[0])
		printBlastUsage()
		os.Exit(1)
	}
}

func printBlastPlan() {
	printHeader()
	fmt.Println()
	fmt.Println("BLAST plan:")
	fmt.Println("  1) choose a database and mode")
	fmt.Println("  2) pick a species when needed")
	fmt.Println("  3) paste a sequence, FASTA, URL, or keyword list")
	fmt.Println("  4) review results, select rows, and export files")
}

func runInteractiveWizard() error {
	wizard := workflow.NewBlastWizardWithTUIInfo(os.Stdout, workflowTUIInfo())
	err := wizard.Run(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "blast wizard failed: %v\n", err)
	}
	return err
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

func printHeader() {
	fmt.Printf("%s %s\n", displayName, version)
	fmt.Printf("Author: %s\n", author)
	fmt.Printf("Repo:   %s\n", repoURL)
}

func printVersion() {
	// A compact version output for machine-friendly calls
	fmt.Printf("%s %s\nAuthor: %s\nRepo: %s\n", displayName, version, author, repoURL)
}

func workflowTUIInfo() workflow.TUIInfo {
	return workflow.TUIInfo{
		DisplayName: displayName,
		Version:     version,
		Author:      author,
		RepoURL:     repoURL,
	}
}

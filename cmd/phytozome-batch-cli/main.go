package main

import (
	"fmt"
	"os"
)

const version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	switch os.Args[1] {
	case "version", "--version", "-version":
		fmt.Println("phytozome-batch-cli", version)
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
		fmt.Println("blast wizard is not implemented yet")
		fmt.Println("See AGENT.md for the current workflow contract")
	default:
		fmt.Fprintf(os.Stderr, "unknown blast subcommand: %s\n\n", args[0])
		printBlastUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("phytozome-batch-cli")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  phytozome-batch-cli blast wizard")
	fmt.Println("  phytozome-batch-cli blast plan")
	fmt.Println("  phytozome-batch-cli version")
}

func printBlastUsage() {
	fmt.Println("Usage:")
	fmt.Println("  phytozome-batch-cli blast wizard")
	fmt.Println("  phytozome-batch-cli blast plan")
}


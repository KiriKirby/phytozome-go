package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	version := flag.Bool("version", false, "print version information")
	flag.Parse()

	if *version {
		fmt.Println("phytozome-batch-cli dev")
		return
	}

	fmt.Fprintln(os.Stdout, "phytozome-batch-cli: starter scaffold")
	fmt.Fprintln(os.Stdout, "Use --version or extend cmd/phytozome-batch-cli/main.go")
}


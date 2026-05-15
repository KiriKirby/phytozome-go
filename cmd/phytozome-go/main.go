// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/KiriKirby/phytozome-go/internal/workflow"
)

var version = "dev"

const displayName = "phytozome GO"
const author = "wangsychn"
const repoURL = "https://github.com/KiriKirby/phytozome-go"

func main() {
	launch, args, err := parseLaunchArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	if len(args) < 1 {
		runDesktopWizard(launch)
		return
	}

	switch args[0] {
	case "version", "--version", "-version":
		printVersion()
	case "blast":
		runBlast(launch, args[1:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

func runDesktopWizard(launch workflow.InstanceLaunchRequest) {
	if err := runInteractiveWizard(launch); err != nil {
		os.Exit(1)
	}
}

func runBlast(launch workflow.InstanceLaunchRequest, args []string) {
	if len(args) == 0 {
		printBlastUsage()
		return
	}

	switch args[0] {
	case "plan":
		printBlastPlan()
	case "wizard":
		if err := runInteractiveWizard(launch); err != nil {
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

func runInteractiveWizard(launch workflow.InstanceLaunchRequest) error {
	wizard := workflow.NewBlastWizardWithLaunch(os.Stdout, workflowTUIInfo(), launch)
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
	fmt.Printf("License: %s (%s)\n", licenseName, licenseID)
}

func printVersion() {
	// A compact version output for machine-friendly calls
	fmt.Printf("%s %s\nAuthor: %s\nRepo: %s\nLicense: %s (%s)\n", displayName, version, author, repoURL, licenseName, licenseID)
}

func workflowTUIInfo() workflow.TUIInfo {
	return workflow.TUIInfo{
		DisplayName: displayName,
		Version:     version,
		Author:      author,
		RepoURL:     repoURL,
		LicenseName: licenseName,
		LicenseID:   licenseID,
	}
}

func parseLaunchArgs(args []string) (workflow.InstanceLaunchRequest, []string, error) {
	fs := flag.NewFlagSet("phytozome-go", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	runID := fs.String("instance-run-id", "", "shared run id for spawned instances")
	instanceID := fs.String("instance-id", "", "instance id for spawned instances")
	parentID := fs.String("instance-parent-id", "", "parent instance id for spawned instances")
	handoffPath := fs.String("handoff", "", "path to instance handoff json")
	versionFlag := fs.Bool("version", false, "print version information")
	helpFlag := fs.Bool("help", false, "show help")
	helpShortFlag := fs.Bool("h", false, "show help")
	if err := fs.Parse(args); err != nil {
		return workflow.InstanceLaunchRequest{}, nil, err
	}

	launch := workflow.InstanceLaunchRequest{
		RunID:            strings.TrimSpace(*runID),
		InstanceID:       strings.TrimSpace(*instanceID),
		ParentInstanceID: strings.TrimSpace(*parentID),
		HandoffPath:      strings.TrimSpace(*handoffPath),
	}
	if launch.HandoffPath != "" {
		handoff, err := workflow.LoadInstanceHandoff(launch.HandoffPath)
		if err != nil {
			return workflow.InstanceLaunchRequest{}, nil, fmt.Errorf("load instance handoff: %w", err)
		}
		launch.Handoff = handoff
		if launch.RunID == "" {
			launch.RunID = strings.TrimSpace(handoff.RunID)
		}
		if launch.InstanceID == "" {
			launch.InstanceID = strings.TrimSpace(handoff.InstanceID)
		}
		if launch.ParentInstanceID == "" {
			launch.ParentInstanceID = strings.TrimSpace(handoff.ParentID)
		}
		if launch.Database == "" {
			launch.Database = strings.TrimSpace(handoff.Database)
		}
		if launch.Mode == "" {
			launch.Mode = workflow.QueryMode(strings.TrimSpace(handoff.Mode))
		}
	}
	if *versionFlag {
		return launch, []string{"--version"}, nil
	}
	if *helpFlag || *helpShortFlag {
		return launch, []string{"--help"}, nil
	}
	return launch, fs.Args(), nil
}

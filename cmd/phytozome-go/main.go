package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/KiriKirby/phytozome-go/internal/cachex"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/KiriKirby/phytozome-go/internal/workflow"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/automaxprocs/maxprocs"
)

var version = "dev"

const displayName = "phytozome GO"
const author = "wangsychn"
const repoURL = "https://github.com/KiriKirby/phytozome-go"

func main() {
	_, _ = maxprocs.Set(maxprocs.Logger(nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer phygoboost.ClosePools()
	defer phygoboost.ClosePools()
	defer cachex.CloseAll()
	handled, err := phygoboost.RunIfWorker(ctx)
	if handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	configureRuntime()
	stopDiagnostics := phygoboost.StartDiagnostics(ctx)
	defer stopDiagnostics()

	root := rootCommand(ctx)
	if len(os.Args) == 1 {
		root.SetArgs([]string{"blast", "wizard"})
	}
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCommand(ctx context.Context) *cobra.Command {
	root := &cobra.Command{
		Use:           "phytozome-go",
		Short:         displayName,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			printVersion()
		},
	})
	blast := &cobra.Command{
		Use:   "blast",
		Short: "Run BLAST and keyword workflows",
	}
	blast.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Print the BLAST workflow plan",
		Run: func(cmd *cobra.Command, args []string) {
			printBlastPlan()
		},
	})
	blast.AddCommand(&cobra.Command{
		Use:   "wizard",
		Short: "Start the interactive workflow",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInteractiveWizard(ctx)
		},
	})
	root.AddCommand(blast)
	return root
}

func configureRuntime() {
	viper.SetEnvPrefix("PHYTOZOME_GO")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()
	level := strings.ToLower(strings.TrimSpace(viper.GetString("LOG_LEVEL")))
	switch level {
	case "trace":
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	_ = phygoboost.RuntimeState()
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

func runInteractiveWizard(ctx context.Context) error {
	wizard := workflow.NewBlastWizardWithTUIInfo(os.Stdout, workflowTUIInfo())
	err := wizard.Run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "blast wizard failed: %v\n", err)
	}
	return err
}

func printHeader() {
	fmt.Printf("%s %s\n", displayName, version)
	fmt.Printf("Author: %s\n", author)
	fmt.Printf("Repo:   %s\n", repoURL)
}

func printVersion() {
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


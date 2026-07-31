package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/gopherust-io/goalign/internal/formatter"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze [path]",
	Short: "Analyze Go files for struct alignment issues",
	Long: `Analyze Go source files for struct alignment issues.

This command will scan Go files and report struct alignment problems that could
lead to memory waste and performance issues. It also suggests NATS-style field
reordering (atomics first, then density packing).`,
	Args: cobra.MaximumNArgs(1),
	Run:  runAnalyze,
}

var failOnFindings bool

func init() {
	rootCmd.AddCommand(analyzeCmd)
	addScanFlags(analyzeCmd)
	analyzeCmd.Flags().BoolVar(&failOnFindings, "fail-on-findings", false, "exit 1 if any issue meets --min-waste")
}

func runAnalyze(cmd *cobra.Command, args []string) {
	path := resolvePath(args)

	results, nFiles, fileErrs, err := collectResults(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if nFiles == 0 {
		fmt.Println("No Go files found")
		return
	}

	if err := formatter.Format(os.Stdout, results, format); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	exitCode := 0
	if fileErrs > 0 {
		exitCode = 1
	}
	if failOnFindings && countIssues(results) > 0 {
		exitCode = 1
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

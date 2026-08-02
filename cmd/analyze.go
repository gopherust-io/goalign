package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gopherust-io/goalign/internal/formatter"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze [path]",
	Short: "Analyze Go files for struct alignment issues",
	Long: `Analyze Go source files for struct alignment issues.

Scans Go files for padding waste and suggests denser field reordering
(atomics first, then density packing). Also reports false-share notes when
contended fields share a cache line (even at 0 padding waste).

Use --cacheguard to preview a Suggested layout with cache-line pads; apply
pads with "goalign fix --cacheguard".

Exit codes:
  0  success (no findings above threshold, or --fail-on-findings not set)
  1  analysis errors or findings with --fail-on-findings
  2  usage / flag errors`,
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

	results, nFiles, fileErrs, err := collectResults(cmd, path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if nFiles == 0 {
		fmt.Println("No Go files found")
		return
	}

	// Multi-arch already printed its text matrix. Skip text per-file detail
	// unless --verbose; always emit machine formats (json/sarif/table).
	if shouldFormatAfterArches(format, verbose, arches) {
		if err := formatter.Format(os.Stdout, results, format); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
			os.Exit(1)
		}
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

// shouldFormatAfterArches reports whether formatter.Format should run after a
// multi-arch matrix. Text stays matrix-only unless verbose; json/sarif/table always emit.
func shouldFormatAfterArches(format string, verbose bool, arches []string) bool {
	if len(arches) == 0 || verbose {
		return true
	}
	switch strings.ToLower(format) {
	case "json", "sarif", "table":
		return true
	default:
		return false
	}
}

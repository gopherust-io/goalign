package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gopherust-io/goalign/internal/fixer"
	"github.com/gopherust-io/goalign/internal/formatter"
)

var fixCmd = &cobra.Command{
	Use:   "fix [path]",
	Short: "Rewrite structs to the suggested field order",
	Long: `Rewrite Go structs to apply alignment-friendly field order.

Uses the same scan rules as analyze (atomics first, then density packing).
Respects // goalign:ignore. Review the resulting diffs before committing.`,
	Args: cobra.MaximumNArgs(1),
	Run:  runFix,
}

func init() {
	rootCmd.AddCommand(fixCmd)
	addScanFlags(fixCmd)
}

func runFix(cmd *cobra.Command, args []string) {
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

	var (
		filesFixed   int
		structsFixed int
		bytesSaved   int
		fixErrs      int
	)

	for _, r := range results {
		var fr fixer.FileResult
		var err error
		if len(r.Content) > 0 {
			fr, err = fixer.FixContent(r.File, r.Content, r.Issues)
		} else {
			fr, err = fixer.FixPath(r.File, r.Issues)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fixing %s: %v\n", r.File, err)
			fixErrs++
			continue
		}
		if !fr.Changed {
			continue
		}
		filesFixed++
		structsFixed += len(fr.Structs)
		bytesSaved += fr.BytesSaved
		if verbose {
			fmt.Printf("Fixed %s (%s)\n", fr.File, strings.Join(fr.Structs, ", "))
		}
	}

	if err := formatter.FormatFixSummary(os.Stdout, filesFixed, structsFixed, bytesSaved); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	if fileErrs > 0 || fixErrs > 0 {
		os.Exit(1)
	}
}

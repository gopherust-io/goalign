package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gopherust-io/goalign/internal/fixer"
	"github.com/gopherust-io/goalign/internal/formatter"
)

var (
	fixDiff      bool
	rewriteBools bool
)

var fixCmd = &cobra.Command{
	Use:   "fix [path]",
	Short: "Rewrite structs to the suggested field order",
	Long: `Rewrite Go structs to apply alignment-friendly field order.

Uses the same scan rules as analyze (atomics first, then density packing).
Respects // goalign:ignore. Review the resulting diffs before committing.

Use --diff (or --dry-run) to print a unified diff without writing files.
Use --cacheguard to insert cache-line pads that isolate contended fields.`,
	Args: cobra.MaximumNArgs(1),
	Run:  runFix,
}

func init() {
	rootCmd.AddCommand(fixCmd)
	addScanFlags(fixCmd)
	fixCmd.Flags().BoolVar(&fixDiff, "diff", false, "print unified diff without writing files")
	fixCmd.Flags().BoolVar(&fixDryRun, "dry-run", false, "alias for --diff")
	fixCmd.Flags().BoolVar(&rewriteBools, "rewrite-bools", false, "pack 3+ unexported scattered bools into a flags word (breaking; review carefully)")
}

var fixDryRun bool

func runFix(cmd *cobra.Command, args []string) {
	path := resolvePath(args)
	if fixDryRun {
		fixDiff = true
	}

	// Apply config before the arches guard so YAML arches: is rejected too.
	if err := applyConfig(cmd, path); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(arches) > 0 {
		fmt.Fprintln(os.Stderr, "Error: --arches is analyze-only")
		os.Exit(2)
	}

	results, nFiles, fileErrs, err := collectResultsConfigured(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if nFiles == 0 {
		fmt.Println("No Go files found")
		return
	}

	opts := fixer.Options{DiffOnly: fixDiff, RewriteBools: rewriteBools, Cacheguard: cacheguard}

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
			fr, err = fixer.FixContentWithOptions(r.File, r.Content, r.Issues, opts)
		} else {
			fr, err = fixer.FixPathWithOptions(r.File, r.Issues, opts)
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
		if fixDiff && fr.Diff != "" {
			fmt.Print(fr.Diff)
		} else if verbose {
			fmt.Printf("Fixed %s (%s)\n", fr.File, strings.Join(fr.Structs, ", "))
		}
	}

	if !fixDiff {
		if err := formatter.FormatFixSummary(os.Stdout, filesFixed, structsFixed, bytesSaved); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
			os.Exit(1)
		}
	} else if structsFixed == 0 {
		fmt.Println("No structs needed fixing.")
	} else {
		fmt.Fprintf(os.Stderr, "# would fix %d structs in %d files", structsFixed, filesFixed)
		if bytesSaved > 0 {
			fmt.Fprintf(os.Stderr, ", save %d bytes", bytesSaved)
		}
		fmt.Fprintln(os.Stderr)
	}

	if fileErrs > 0 || fixErrs > 0 {
		os.Exit(1)
	}
}

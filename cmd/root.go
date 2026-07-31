package cmd

import (
	"os"
	"runtime/debug"

	"github.com/gopherust-io/goalign/internal/bytesconv"
	"github.com/spf13/cobra"
)

var (
	version = moduleVersion()
	verbose bool
	format  string
)

func moduleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil || bytesconv.IsEmpty(info.Main.Version) {
		return "unknown"
	}
	return info.Main.Version
}

var rootCmd = &cobra.Command{
	Use:   "goalign",
	Short: "Check and fix Go struct memory alignment",
	Long: `GoAlign analyzes Go struct field order for padding waste and can rewrite
structs to a denser, NATS-style layout (atomics first, then density packing).

Use "analyze" to report issues and "fix" to apply suggested field order.`,
	Version: version,
}

// Execute runs the root command.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVarP(&format, "format", "f", "text", "output format (text, json, table)")
}

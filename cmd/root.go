package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "1.0.0"
	verbose bool
	format  string
)

var rootCmd = &cobra.Command{
	Use:   "goalign",
	Short: "A CLI tool for analyzing Go struct alignment",
	Long: `GoAlign is a utility for checking and viewing Golang struct alignment info.

Struct alignment is the extra space added between fields in a struct to align them 
in memory according to the CPU's word size. By understanding and managing struct 
padding (e.g., reordering fields), you can improve program performance, reduce 
memory usage, and ensure data integrity.`,
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

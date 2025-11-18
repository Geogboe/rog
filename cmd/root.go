package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Geogboe/rog/cmd/meta"
)

var rootCmd = &cobra.Command{
	Use:   "rog",
	Short: "A Git repository navigator and catalog system",
	Long: `rog is a fast, local-first Git repository navigator that helps you
find, understand, and manage the projects on your system.

rog indexes your repositories and provides fast searching, filtering,
and navigation capabilities with optional LLM enrichment.`,
}

// Execute executes the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Disable automatic completion command
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Add meta subcommand
	rootCmd.AddCommand(meta.MetaCmd)
}

// exitWithError prints an error message and exits
func exitWithError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

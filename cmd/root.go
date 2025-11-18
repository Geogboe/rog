package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Geogboe/rog/cmd/meta"
	"github.com/Geogboe/rog/internal/logger"
)

var (
	verbose bool
	debug   bool
)

var rootCmd = &cobra.Command{
	Use:   "rog",
	Short: "A Git repository navigator and catalog system",
	Long: `rog is a fast, local-first Git repository navigator that helps you
find, understand, and manage the projects on your system.

rog indexes your repositories and provides fast searching, filtering,
and navigation capabilities with optional LLM enrichment.

Environment variables:
  ROG_VERBOSE=1  Enable verbose logging
  ROG_DEBUG=1    Enable debug logging`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Initialize logger from environment first
		logger.InitFromEnv()

		// Then override with flags if set
		if debug {
			logger.SetLevel(logger.LevelDebug)
		} else if verbose {
			logger.SetLevel(logger.LevelVerbose)
		}
	},
}

// Execute executes the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Add global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging (more verbose than --verbose)")

	// Add meta subcommand
	rootCmd.AddCommand(meta.MetaCmd)
}

// exitWithError prints an error message and exits
func exitWithError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

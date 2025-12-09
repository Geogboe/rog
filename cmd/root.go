package cmd

import (
	"fmt"
	"os"
	"strings"

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
	SilenceErrors: true, // We handle errors in Execute()
	SilenceUsage:  true,
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
	// First, try to execute normally
	err := rootCmd.Execute()

	// If we get an "unknown command" error and there are args,
	// treat it as an alias for "rog list <args>"
	if err != nil && strings.Contains(err.Error(), "unknown command") {
		// Get original args (skip program name)
		args := os.Args[1:]

		// Only apply the alias if there are args and the first arg
		// doesn't look like a flag
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			// Prepend "list" to the arguments
			newArgs := append([]string{"list"}, args...)
			rootCmd.SetArgs(newArgs)

			// Execute with the list command - error already suppressed
			return rootCmd.Execute()
		}

		// If we couldn't handle it as an alias, print the error
		fmt.Fprintln(os.Stderr, err)
		return err
	}

	// For other errors, print them
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
	}

	return err
}

func init() {
	// Add global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging (more verbose than --verbose)")

	// Make completion command visible in help
	// Cobra adds it automatically but hides it by default
	rootCmd.InitDefaultCompletionCmd()
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "completion" {
			cmd.Hidden = false
			break
		}
	}

	// Add meta subcommand
	rootCmd.AddCommand(meta.MetaCmd)
}

// exitWithError prints an error message and exits
func exitWithError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/query"
)

var pathCmd = &cobra.Command{
	Use:   "path <name|path|query>",
	Short: "Print the absolute path of a repository",
	Long: `Print the absolute path of a repository.

Useful for scripting and shell integration.

Accepts exact name, absolute path, or a fuzzy query.
If the query is ambiguous, shows an error.

Examples:
  cd "$(rog path myproject)"
  code "$(rog path api)"`,
	Args: cobra.ExactArgs(1),
	Run:  runPath,
}

func init() {
	rootCmd.AddCommand(pathCmd)
}

func runPath(cmd *cobra.Command, args []string) {
	// Load index
	idx, err := index.Load()
	if err != nil {
		exitWithError("Failed to load index: %v", err)
	}

	queryStr := args[0]

	// Try to find unique match
	repo, matches, err := query.FindUnique(idx, queryStr)
	if err != nil {
		exitWithError("Query failed: %v", err)
	}

	if repo == nil {
		if len(matches) == 0 {
			exitWithError("No repository found matching '%s'", queryStr)
		} else {
			exitWithError("Ambiguous query '%s' matches %d repositories. Use 'rog info' or 'rog select'", queryStr, len(matches))
		}
	}

	// Print path only (for scripting)
	fmt.Println(repo.AbsPath)
}

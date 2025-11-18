package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/query"
)

var openCmd = &cobra.Command{
	Use:   "open <name|path|query>",
	Short: "Open a repository in your editor",
	Long: `Open a repository in your configured editor.

Editor resolution priority:
  1. ROG_EDITOR environment variable
  2. editor field in config.yml
  3. EDITOR environment variable
  4. vi (fallback)

Accepts exact name, absolute path, or a fuzzy query.

Examples:
  rog open myproject
  rog open api`,
	Args: cobra.ExactArgs(1),
	Run:  runOpen,
}

func init() {
	rootCmd.AddCommand(openCmd)
}

func runOpen(cmd *cobra.Command, args []string) {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		exitWithError("Failed to load config: %v", err)
	}

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
			exitWithError("Ambiguous query '%s' matches %d repositories. Use 'rog select'", queryStr, len(matches))
		}
	}

	// Get editor
	editor := cfg.Editor
	if editor == "" {
		editor = "vi"
	}

	// Open in editor
	fmt.Printf("Opening %s in %s...\n", repo.Name, editor)

	editorCmd := exec.Command(editor, repo.AbsPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		exitWithError("Failed to open editor: %v", err)
	}
}

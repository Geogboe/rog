package meta

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/metadata"
)

var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit metadata file in editor",
	Long: `Edit metadata file in your configured editor.

By default, edits .rogmeta.yml in the current directory.
With --global, edits the global metadata file.

Examples:
  cd /path/to/repo && rog meta edit
  rog meta edit --global`,
	Run: runMetaEdit,
}

func init() {
	editCmd.Flags().BoolVar(&globalFlag, "global", false, "Edit global metadata file")
}

func runMetaEdit(cmd *cobra.Command, args []string) {
	// Load config for editor
	cfg, err := config.Load()
	if err != nil {
		exitWithError("Failed to load config: %v", err)
	}

	editor := cfg.Editor
	if editor == "" {
		editor = "vi"
	}

	var filePath string

	if globalFlag {
		filePath = config.GetMetaPath()
		// Ensure file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			// Create default
			globalMeta := &metadata.GlobalMeta{Repos: []metadata.GlobalRepoMeta{}}
			if err := metadata.SaveGlobalMeta(globalMeta); err != nil {
				exitWithError("Failed to create global metadata: %v", err)
			}
		}
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			exitWithError("Failed to get current directory: %v", err)
		}
		filePath = fmt.Sprintf("%s/.rogmeta.yml", cwd)

		// Check if file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			exitWithError(".rogmeta.yml not found. Run 'rog meta init' first.")
		}
	}

	// Open in editor
	editorCmd := exec.Command(editor, filePath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		exitWithError("Failed to open editor: %v", err)
	}

	fmt.Printf("✓ Metadata file updated\n")
}

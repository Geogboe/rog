package meta

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Geogboe/rog/internal/metadata"
)

var (
	globalFlag bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize metadata file",
	Long: `Initialize a metadata file.

By default, creates .rogmeta.yml in the current directory.
With --global, creates or opens the global metadata file.

Examples:
  cd /path/to/repo && rog meta init
  rog meta init --global`,
	Run: runMetaInit,
}

func init() {
	initCmd.Flags().BoolVar(&globalFlag, "global", false, "Initialize global metadata file")
}

func runMetaInit(cmd *cobra.Command, args []string) {
	if globalFlag {
		initGlobalMeta()
	} else {
		initRepoMeta()
	}
}

func initRepoMeta() {
	cwd, err := os.Getwd()
	if err != nil {
		exitWithError("Failed to get current directory: %v", err)
	}

	// Check if .rogmeta.yml already exists
	metaPath := fmt.Sprintf("%s/.rogmeta.yml", cwd)
	if _, err := os.Stat(metaPath); err == nil {
		exitWithError(".rogmeta.yml already exists in current directory")
	}

	// Create default metadata
	meta := &metadata.RepoMeta{
		Description: "",
		Tags:        []string{},
	}

	if err := metadata.WriteRepoMeta(cwd, meta); err != nil {
		exitWithError("Failed to create .rogmeta.yml: %v", err)
	}

	fmt.Printf("✓ Created .rogmeta.yml\n")
	fmt.Printf("  Edit this file to add description, tags, and other metadata.\n")
}

func initGlobalMeta() {
	// Load or create global metadata
	globalMeta, err := metadata.LoadGlobalMeta()
	if err != nil {
		exitWithError("Failed to load global metadata: %v", err)
	}

	// Save it (creates file if doesn't exist)
	if err := metadata.SaveGlobalMeta(globalMeta); err != nil {
		exitWithError("Failed to save global metadata: %v", err)
	}

	fmt.Printf("✓ Global metadata file ready\n")
	fmt.Printf("  Use 'rog meta edit --global' to edit it.\n")
}

func exitWithError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

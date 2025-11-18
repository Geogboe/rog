package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Geogboe/rog/internal/config"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize rog configuration",
	Long: `Initialize rog by creating a default configuration file.

This will create ~/.config/rog/config.yml with default settings.
You can edit this file to customize your roots, editor, and LLM settings.`,
	Run: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) {
	// Create default config
	cfg := config.DefaultConfig()

	// Save config
	if err := config.Save(cfg); err != nil {
		exitWithError("Failed to save config: %v", err)
	}

	configPath := config.GetDataDir()
	fmt.Printf("✓ Configuration initialized at %s/config.yml\n", configPath)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Edit the config file to add your repository roots")
	fmt.Println("  2. Run 'rog scan' to index your repositories")
	fmt.Println("  3. Use 'rog list' to view your repositories")
}

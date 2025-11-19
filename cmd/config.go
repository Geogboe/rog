package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/Geogboe/rog/internal/config"
)

var (
	configValidate bool
	configEdit     bool
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage rog configuration",
	Long: `Manage and validate rog configuration.

By default, displays the current configuration.
Use --validate to check for configuration errors.
Use --edit to open config in your editor.

Examples:
  rog config                  Show current config
  rog config --validate       Validate config file
  rog config --edit           Edit config in $EDITOR`,
	Run: runConfig,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.Flags().BoolVar(&configValidate, "validate", false, "Validate configuration file")
	configCmd.Flags().BoolVar(&configEdit, "edit", false, "Edit configuration file in editor")
}

func runConfig(cmd *cobra.Command, args []string) {
	if configValidate {
		validateConfig()
		return
	}

	if configEdit {
		editConfig()
		return
	}

	// Default: show current config
	showConfig()
}

func validateConfig() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ Config validation failed: %v\n", err)
		os.Exit(1)
	}

	// Validation checks
	errors := []string{}
	warnings := []string{}

	// 1. Check if config has at least one root
	if len(cfg.Roots) == 0 {
		errors = append(errors, "No roots configured")
	}

	// 2. Validate each root
	for i, root := range cfg.Roots {
		rootPrefix := fmt.Sprintf("Root[%d] (%s)", i, root.Name)

		// Check name is not empty
		if root.Name == "" {
			errors = append(errors, fmt.Sprintf("%s: Name is required", rootPrefix))
		}

		// Check path is not empty
		if root.Path == "" {
			errors = append(errors, fmt.Sprintf("%s: Path is required", rootPrefix))
		} else {
			// Check if path exists and is accessible
			info, err := os.Stat(root.Path)
			if os.IsNotExist(err) {
				errors = append(errors, fmt.Sprintf("%s: Path does not exist: %s", rootPrefix, root.Path))
			} else if err != nil {
				errors = append(errors, fmt.Sprintf("%s: Cannot access path: %s (%v)", rootPrefix, root.Path, err))
			} else if !info.IsDir() {
				errors = append(errors, fmt.Sprintf("%s: Path is not a directory: %s", rootPrefix, root.Path))
			}

			// Check for permission issues
			if err := checkDirPermissions(root.Path); err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", rootPrefix, err))
			}
		}

		// Check MaxDepth is positive
		if root.MaxDepth <= 0 {
			errors = append(errors, fmt.Sprintf("%s: MaxDepth must be positive (got %d)", rootPrefix, root.MaxDepth))
		}

		// Check for suspicious excludes
		for _, exclude := range root.Exclude {
			if exclude == "" {
				warnings = append(warnings, fmt.Sprintf("%s: Empty exclude pattern", rootPrefix))
			}
			if exclude == "." || exclude == ".." {
				errors = append(errors, fmt.Sprintf("%s: Invalid exclude pattern: %s", rootPrefix, exclude))
			}
		}
	}

	// 3. Check for duplicate root names
	rootNames := make(map[string]int)
	for i, root := range cfg.Roots {
		if prevIdx, exists := rootNames[root.Name]; exists {
			errors = append(errors, fmt.Sprintf("Duplicate root name '%s' at indexes %d and %d", root.Name, prevIdx, i))
		}
		rootNames[root.Name] = i
	}

	// 4. Check global excludes
	for _, exclude := range cfg.GlobalExcludes {
		if exclude == "" {
			warnings = append(warnings, "Empty global exclude pattern")
		}
		if exclude == "." || exclude == ".." {
			errors = append(errors, fmt.Sprintf("Invalid global exclude pattern: %s", exclude))
		}
	}

	// 5. Check if global excludes conflict with root paths
	for _, root := range cfg.Roots {
		baseName := filepath.Base(root.Path)
		for _, exclude := range cfg.GlobalExcludes {
			if matched, _ := filepath.Match(exclude, baseName); matched {
				warnings = append(warnings, fmt.Sprintf("Global exclude '%s' matches root path basename '%s'", exclude, baseName))
			}
		}
	}

	// 6. Validate LLM config if present
	if cfg.LLM != nil {
		if cfg.LLM.Endpoint == "" {
			warnings = append(warnings, "LLM endpoint is empty (LLM features will not work)")
		}
		if cfg.LLM.Model == "" {
			warnings = append(warnings, "LLM model is empty (LLM features will not work)")
		}
	}

	// 7. Check editor
	if cfg.Editor != "" {
		// Only warn if it looks like a path and doesn't exist
		if filepath.IsAbs(cfg.Editor) || filepath.Base(cfg.Editor) != cfg.Editor {
			if _, err := os.Stat(cfg.Editor); os.IsNotExist(err) {
				warnings = append(warnings, fmt.Sprintf("Editor path does not exist: %s", cfg.Editor))
			}
		}
	}

	// Print results
	if len(errors) > 0 {
		fmt.Println("✗ Configuration validation failed\n")
		fmt.Println("Errors:")
		for _, err := range errors {
			fmt.Printf("  • %s\n", err)
		}
		if len(warnings) > 0 {
			fmt.Println("\nWarnings:")
			for _, warn := range warnings {
				fmt.Printf("  • %s\n", warn)
			}
		}
		os.Exit(1)
	}

	if len(warnings) > 0 {
		fmt.Println("✓ Configuration is valid (with warnings)\n")
		fmt.Println("Warnings:")
		for _, warn := range warnings {
			fmt.Printf("  • %s\n", warn)
		}
	} else {
		fmt.Println("✓ Configuration is valid")
		fmt.Printf("\nSummary:\n")
		fmt.Printf("  • %d root(s) configured\n", len(cfg.Roots))
		if len(cfg.GlobalExcludes) > 0 {
			fmt.Printf("  • %d global exclude(s)\n", len(cfg.GlobalExcludes))
		}
		if cfg.LLM != nil && cfg.LLM.Endpoint != "" {
			fmt.Printf("  • LLM configured: %s\n", cfg.LLM.Endpoint)
		}
	}
}

func showConfig() {
	cfg, err := config.Load()
	if err != nil {
		exitWithError("Failed to load config: %v", err)
	}

	// Marshal to YAML for pretty printing
	data, err := yaml.Marshal(cfg)
	if err != nil {
		exitWithError("Failed to format config: %v", err)
	}

	fmt.Print(string(data))
}

func editConfig() {
	// Get config path
	configPath := os.Getenv("ROG_CONFIG")
	if configPath == "" {
		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			homeDir, _ := os.UserHomeDir()
			configDir = filepath.Join(homeDir, ".config")
		}
		configPath = filepath.Join(configDir, "rog", "config.yml")
	}

	// Ensure config exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config
		cfg := config.DefaultConfig()
		if err := config.Save(cfg); err != nil {
			exitWithError("Failed to create config: %v", err)
		}
		fmt.Printf("Created default config at %s\n", configPath)
	}

	// Get editor
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Open in editor
	cmd := exec.Command(editor, configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		exitWithError("Failed to open editor: %v", err)
	}

	// Validate after editing
	fmt.Println("\nValidating config...")
	if cfg, err := config.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: Config has errors: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run 'rog config --validate' for details\n")
		os.Exit(1)
	} else {
		fmt.Printf("✓ Config is valid (%d roots)\n", len(cfg.Roots))
	}
}

func checkDirPermissions(path string) error {
	// Try to read the directory
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("Cannot read directory: %v", err)
	}
	defer f.Close()

	// Try to list contents
	_, err = f.Readdirnames(1)
	if err != nil && err.Error() != "EOF" {
		return fmt.Errorf("Cannot list directory contents: %v", err)
	}

	return nil
}

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the rog configuration
type Config struct {
	Roots  []Root      `yaml:"roots"`
	Editor string      `yaml:"editor"`
	LLM    *LLMConfig  `yaml:"llm,omitempty"`
	List   *ListConfig `yaml:"list,omitempty"`
}

// Root represents a search root configuration
type Root struct {
	Name      string   `yaml:"name"`
	Path      string   `yaml:"path"`
	MaxDepth  int      `yaml:"max_depth"`
	Exclude   []string `yaml:"exclude,omitempty"`
	WSL       bool     `yaml:"wsl,omitempty"`        // True if this root is in WSL
	WSLDistro string   `yaml:"wsl_distro,omitempty"` // WSL distro name (e.g., "Ubuntu")
}

// LLMConfig represents LLM configuration
type LLMConfig struct {
	Endpoint          string `yaml:"endpoint"`
	Model             string `yaml:"model"`
	APIKey            string `yaml:"api_key,omitempty"`
	ExtraInstructions string `yaml:"extra_instructions,omitempty"`
}

// ListConfig represents list command configuration
type ListConfig struct {
	DefaultFields []string `yaml:"default_fields,omitempty"`
}

// Load loads configuration from file with environment variable overrides
func Load() (*Config, error) {
	configPath := getConfigPath()

	// If config doesn't exist, return default config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply environment variable overrides
	applyEnvOverrides(&cfg)

	// Expand paths in roots
	for i := range cfg.Roots {
		expandedPath, err := expandPath(cfg.Roots[i].Path)
		if err != nil {
			return nil, fmt.Errorf("failed to expand path for root %s: %w", cfg.Roots[i].Name, err)
		}
		cfg.Roots[i].Path = expandedPath
	}

	// Apply defaults
	applyDefaults(&cfg)

	return &cfg, nil
}

// Save saves configuration to file
func Save(cfg *Config) error {
	configPath := getConfigPath()

	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write atomically: write to temp file, then rename
	tempPath := configPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp config file: %w", err)
	}

	if err := os.Rename(tempPath, configPath); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("failed to rename temp config file: %w", err)
	}

	return nil
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		Roots: []Root{
			{
				Name:     "home",
				Path:     homeDir,
				MaxDepth: 3,
				Exclude:  []string{"node_modules", "vendor", ".git"},
			},
		},
		Editor: getDefaultEditor(),
	}
}

// getConfigPath returns the path to the config file
func getConfigPath() string {
	if path := os.Getenv("ROG_CONFIG"); path != "" {
		return path
	}

	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		homeDir, _ := os.UserHomeDir()
		configDir = filepath.Join(homeDir, ".config")
	}

	return filepath.Join(configDir, "rog", "config.yml")
}

// GetDataDir returns the path to the data directory
func GetDataDir() string {
	if path := os.Getenv("ROG_DATA"); path != "" {
		return path
	}

	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		homeDir, _ := os.UserHomeDir()
		dataDir = filepath.Join(homeDir, ".local", "share")
	}

	// For simplicity, use same as config dir
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		homeDir, _ := os.UserHomeDir()
		configDir = filepath.Join(homeDir, ".config")
	}

	return filepath.Join(configDir, "rog")
}

// GetMetaPath returns the path to the global metadata file
func GetMetaPath() string {
	return filepath.Join(GetDataDir(), "meta.yml")
}

// applyEnvOverrides applies environment variable overrides
func applyEnvOverrides(cfg *Config) {
	if editor := os.Getenv("ROG_EDITOR"); editor != "" {
		cfg.Editor = editor
	}

	if cfg.LLM == nil {
		cfg.LLM = &LLMConfig{}
	}

	if endpoint := os.Getenv("ROG_LLM_ENDPOINT"); endpoint != "" {
		cfg.LLM.Endpoint = endpoint
	}

	if model := os.Getenv("ROG_LLM_MODEL"); model != "" {
		cfg.LLM.Model = model
	}

	if apiKey := os.Getenv("ROG_LLM_API_KEY"); apiKey != "" {
		cfg.LLM.APIKey = apiKey
	}

	if extra := os.Getenv("ROG_LLM_EXTRA"); extra != "" {
		cfg.LLM.ExtraInstructions = extra
	}
}

// applyDefaults applies default values
func applyDefaults(cfg *Config) {
	// Default editor
	if cfg.Editor == "" {
		cfg.Editor = getDefaultEditor()
	}

	// Default max depth for roots
	for i := range cfg.Roots {
		if cfg.Roots[i].MaxDepth == 0 {
			cfg.Roots[i].MaxDepth = 4
		}
		if cfg.Roots[i].Exclude == nil {
			cfg.Roots[i].Exclude = []string{"node_modules", "vendor"}
		}
	}
}

// getDefaultEditor returns the default editor
func getDefaultEditor() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	return "vi"
}

// expandPath expands ~ and environment variables in path
func expandPath(path string) (string, error) {
	// Expand environment variables
	path = os.ExpandEnv(path)

	// Expand ~
	if len(path) > 0 && path[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if len(path) == 1 {
			return homeDir, nil
		}
		return filepath.Join(homeDir, path[1:]), nil
	}

	// Make absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	return absPath, nil
}

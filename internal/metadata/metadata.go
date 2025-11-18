package metadata

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/Geogboe/rog/internal/config"
)

// RepoMeta represents metadata for a repository
type RepoMeta struct {
	Name            string   `yaml:"name,omitempty"`
	Description     string   `yaml:"description,omitempty"`
	Tags            []string `yaml:"tags,omitempty"`
	PrimaryLanguage string   `yaml:"primary_language,omitempty"`
}

// GlobalMeta represents global metadata configuration
type GlobalMeta struct {
	Repos []GlobalRepoMeta `yaml:"repos"`
}

// GlobalRepoMeta represents a repository entry in global metadata
type GlobalRepoMeta struct {
	Root            string   `yaml:"root"`
	Path            string   `yaml:"path"`
	Description     string   `yaml:"description,omitempty"`
	Tags            []string `yaml:"tags,omitempty"`
	PrimaryLanguage string   `yaml:"primary_language,omitempty"`
}

// ReadRepoMeta reads .rogmeta.yml from a repository directory
func ReadRepoMeta(repoPath string) (*RepoMeta, error) {
	metaPath := filepath.Join(repoPath, ".rogmeta.yml")

	// If file doesn't exist, return nil without error
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		return nil, nil
	}

	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read .rogmeta.yml: %w", err)
	}

	var meta RepoMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse .rogmeta.yml: %w", err)
	}

	return &meta, nil
}

// WriteRepoMeta writes .rogmeta.yml to a repository directory
func WriteRepoMeta(repoPath string, meta *RepoMeta) error {
	metaPath := filepath.Join(repoPath, ".rogmeta.yml")

	data, err := yaml.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(metaPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write .rogmeta.yml: %w", err)
	}

	return nil
}

// LoadGlobalMeta loads the global metadata file
func LoadGlobalMeta() (*GlobalMeta, error) {
	metaPath := config.GetMetaPath()

	// If file doesn't exist, return empty metadata
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		return &GlobalMeta{Repos: []GlobalRepoMeta{}}, nil
	}

	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read global metadata: %w", err)
	}

	var meta GlobalMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse global metadata: %w", err)
	}

	if meta.Repos == nil {
		meta.Repos = []GlobalRepoMeta{}
	}

	return &meta, nil
}

// SaveGlobalMeta saves the global metadata file
func SaveGlobalMeta(meta *GlobalMeta) error {
	metaPath := config.GetMetaPath()

	// Ensure directory exists
	metaDir := filepath.Dir(metaPath)
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		return fmt.Errorf("failed to create metadata directory: %w", err)
	}

	data, err := yaml.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal global metadata: %w", err)
	}

	if err := os.WriteFile(metaPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write global metadata: %w", err)
	}

	return nil
}

// FindGlobalMeta finds metadata for a repository in global metadata
func FindGlobalMeta(globalMeta *GlobalMeta, root, relPath string) *GlobalRepoMeta {
	for _, repo := range globalMeta.Repos {
		if repo.Root == root && repo.Path == relPath {
			return &repo
		}
	}
	return nil
}

// MergeMeta merges metadata from different sources with precedence
// Priority: repoMeta (manual) > globalMeta > existing
func MergeMeta(existing *RepoMeta, repoMeta *RepoMeta, globalMeta *GlobalRepoMeta) *RepoMeta {
	result := &RepoMeta{}

	// Start with existing
	if existing != nil {
		*result = *existing
	}

	// Apply global metadata
	if globalMeta != nil {
		if globalMeta.Description != "" {
			result.Description = globalMeta.Description
		}
		if len(globalMeta.Tags) > 0 {
			result.Tags = globalMeta.Tags
		}
		if globalMeta.PrimaryLanguage != "" {
			result.PrimaryLanguage = globalMeta.PrimaryLanguage
		}
	}

	// Apply repo metadata (highest priority)
	if repoMeta != nil {
		if repoMeta.Name != "" {
			result.Name = repoMeta.Name
		}
		if repoMeta.Description != "" {
			result.Description = repoMeta.Description
		}
		if len(repoMeta.Tags) > 0 {
			result.Tags = repoMeta.Tags
		}
		if repoMeta.PrimaryLanguage != "" {
			result.PrimaryLanguage = repoMeta.PrimaryLanguage
		}
	}

	return result
}

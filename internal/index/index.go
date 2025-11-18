package index

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Geogboe/rog/internal/config"
)

// Index represents the repository index
type Index struct {
	Repos     map[string]*Repo `json:"repos"` // Key: absolute path
	UpdatedAt time.Time        `json:"updated_at"`
	mu        sync.RWMutex
}

// Repo represents a single repository entry
type Repo struct {
	// Identity
	ID      string `json:"id"`       // Hash of absolute path
	Name    string `json:"name"`     // Directory name
	Root    string `json:"root"`     // Root identifier
	RelPath string `json:"rel_path"` // Relative to root
	AbsPath string `json:"abs_path"` // Full resolved path

	// Git Info
	RemoteURL        string    `json:"remote_url,omitempty"`
	Host             string    `json:"host,omitempty"`
	CurrentBranch    string    `json:"current_branch,omitempty"`
	LastCommitTime   time.Time `json:"last_commit_time,omitempty"`
	LastCommitAuthor string    `json:"last_commit_author,omitempty"`
	LastCommitHash   string    `json:"last_commit_hash,omitempty"`
	IsDirty          bool      `json:"is_dirty"`
	HasUntracked     bool      `json:"has_untracked"`
	Ahead            int       `json:"ahead"`
	Behind           int       `json:"behind"`
	LastGitCheckAt   time.Time `json:"last_git_check_at,omitempty"`

	// Metadata
	PrimaryLanguage string   `json:"primary_language,omitempty"`
	Description     string   `json:"description,omitempty"`
	Tags            []string `json:"tags,omitempty"`

	// Tracking
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastScanAt  time.Time `json:"last_scan_at"`

	// Metadata source tracking
	DescriptionSource string `json:"description_source,omitempty"` // "manual", "global", "llm", "auto"
	TagsSource        string `json:"tags_source,omitempty"`
}

// New creates a new empty index
func New() *Index {
	return &Index{
		Repos:     make(map[string]*Repo),
		UpdatedAt: time.Now(),
	}
}

// Load loads the index from disk
func Load() (*Index, error) {
	indexPath := getIndexPath()

	// If index doesn't exist, return new empty index
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		return New(), nil
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read index file: %w", err)
	}

	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("failed to parse index file: %w", err)
	}

	if idx.Repos == nil {
		idx.Repos = make(map[string]*Repo)
	}

	return &idx, nil
}

// Save saves the index to disk atomically
func (idx *Index) Save() error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	indexPath := getIndexPath()

	// Ensure data directory exists
	dataDir := filepath.Dir(indexPath)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	idx.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	// Write atomically: write to temp file, then rename
	tempPath := indexPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp index file: %w", err)
	}

	if err := os.Rename(tempPath, indexPath); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("failed to rename temp index file: %w", err)
	}

	return nil
}

// Upsert inserts or updates a repository in the index
func (idx *Index) Upsert(repo *Repo) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Generate ID if not set
	if repo.ID == "" {
		repo.ID = generateID(repo.AbsPath)
	}

	// Preserve FirstSeenAt if repo already exists
	if existing, ok := idx.Repos[repo.AbsPath]; ok {
		repo.FirstSeenAt = existing.FirstSeenAt
	} else {
		repo.FirstSeenAt = time.Now()
	}

	repo.LastScanAt = time.Now()

	idx.Repos[repo.AbsPath] = repo
}

// Get retrieves a repository by absolute path
func (idx *Index) Get(absPath string) (*Repo, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	repo, ok := idx.Repos[absPath]
	return repo, ok
}

// GetByName retrieves repositories by name (can be multiple)
func (idx *Index) GetByName(name string) []*Repo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var repos []*Repo
	for _, repo := range idx.Repos {
		if repo.Name == name {
			repos = append(repos, repo)
		}
	}
	return repos
}

// List returns all repositories as a slice
func (idx *Index) List() []*Repo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	repos := make([]*Repo, 0, len(idx.Repos))
	for _, repo := range idx.Repos {
		repos = append(repos, repo)
	}

	// Sort by name by default
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Name < repos[j].Name
	})

	return repos
}

// Delete removes a repository from the index
func (idx *Index) Delete(absPath string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	delete(idx.Repos, absPath)
}

// Count returns the number of repositories in the index
func (idx *Index) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return len(idx.Repos)
}

// RemoveStale removes repositories that no longer exist on disk
func (idx *Index) RemoveStale() int {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	removed := 0
	for path, repo := range idx.Repos {
		if _, err := os.Stat(filepath.Join(repo.AbsPath, ".git")); os.IsNotExist(err) {
			delete(idx.Repos, path)
			removed++
		}
	}

	return removed
}

// getIndexPath returns the path to the index file
func getIndexPath() string {
	return filepath.Join(config.GetDataDir(), "index.json")
}

// generateID generates a unique ID for a repository based on its absolute path
func generateID(absPath string) string {
	hash := sha256.Sum256([]byte(absPath))
	return hex.EncodeToString(hash[:])[:16]
}

package index

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Geogboe/rog/internal/config"
)

// Index represents the repository index
type Index struct {
	Repos           map[string]*Repo          `json:"repos"` // Key: absolute path
	RejectedMarkers map[string]RejectedMarker `json:"rejected_markers,omitempty"`
	UpdatedAt       time.Time                 `json:"updated_at"`
	mu              sync.RWMutex
}

// RejectedMarker records an invalid Git marker until it changes or ages out.
type RejectedMarker struct {
	ModTime   time.Time `json:"mod_time"`
	Size      int64     `json:"size"`
	CheckedAt time.Time `json:"checked_at"`
}

const rejectedMarkerMaxAge = 24 * time.Hour

// Repo represents a single repository entry
type Repo struct {
	// Identity
	ID        string `json:"id"`         // Hash of absolute path
	Name      string `json:"name"`       // Directory name
	Root      string `json:"root"`       // Root identifier
	RelPath   string `json:"rel_path"`   // Relative to root
	AbsPath   string `json:"abs_path"`   // Full resolved path
	IsWSL     bool   `json:"is_wsl"`     // True if repo is in WSL
	WSLDistro string `json:"wsl_distro"` // WSL distro name if IsWSL is true

	// Git Info
	RemoteURL         string    `json:"remote_url,omitempty"`
	Host              string    `json:"host,omitempty"`
	CurrentBranch     string    `json:"current_branch,omitempty"`
	LastCommitTime    time.Time `json:"last_commit_time,omitempty"`
	LastCommitAuthor  string    `json:"last_commit_author,omitempty"`
	LastCommitHash    string    `json:"last_commit_hash,omitempty"`
	IsDirty           bool      `json:"is_dirty"`
	HasUntracked      bool      `json:"has_untracked"`
	StatusUnavailable bool      `json:"status_unavailable,omitempty"`
	Ahead             int       `json:"ahead"`
	Behind            int       `json:"behind"`
	LastGitCheckAt    time.Time `json:"last_git_check_at,omitempty"`

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
		Repos:           make(map[string]*Repo),
		RejectedMarkers: make(map[string]RejectedMarker),
		UpdatedAt:       time.Now(),
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
	if idx.RejectedMarkers == nil {
		idx.RejectedMarkers = make(map[string]RejectedMarker)
	}
	if runtime.GOOS == "windows" {
		idx.canonicalizeWSLPaths()
	}

	return &idx, nil
}

// canonicalizeWSLPaths preserves cached metadata when the WSL UNC alias changes.
func (idx *Index) canonicalizeWSLPaths() {
	const oldPrefix = `\\wsl.localhost\`
	const newPrefix = `\\wsl$\`
	for key, repo := range idx.Repos {
		if repo == nil || !repo.IsWSL || !strings.HasPrefix(strings.ToLower(repo.AbsPath), oldPrefix) {
			continue
		}
		newPath := newPrefix + repo.AbsPath[len(oldPrefix):]
		delete(idx.Repos, key)
		if current, exists := idx.Repos[newPath]; exists && current.LastScanAt.After(repo.LastScanAt) {
			continue
		}
		repo.AbsPath = newPath
		repo.ID = generateID(newPath)
		idx.Repos[newPath] = repo
	}
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

// IsRejectedUnchanged reports whether a previously invalid marker can be skipped.
func (idx *Index) IsRejectedUnchanged(repoPath string) bool {
	idx.mu.RLock()
	marker, ok := idx.RejectedMarkers[repoPath]
	idx.mu.RUnlock()
	if !ok || time.Since(marker.CheckedAt) >= rejectedMarkerMaxAge {
		return false
	}
	info, err := os.Stat(filepath.Join(repoPath, ".git"))
	return err == nil && info.Size() == marker.Size && info.ModTime().Equal(marker.ModTime)
}

// RememberRejected records the current marker fingerprint after Git rejects it.
func (idx *Index) RememberRejected(repoPath string) {
	info, err := os.Stat(filepath.Join(repoPath, ".git"))
	if err != nil {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.RejectedMarkers == nil {
		idx.RejectedMarkers = make(map[string]RejectedMarker)
	}
	idx.RejectedMarkers[repoPath] = RejectedMarker{ModTime: info.ModTime(), Size: info.Size(), CheckedAt: time.Now()}
}

// ForgetRejected removes a negative cache entry when a repository becomes valid.
func (idx *Index) ForgetRejected(repoPath string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.RejectedMarkers, repoPath)
}

// RemoveStale removes repositories that no longer exist on disk.
func (idx *Index) RemoveStale() int {
	return idx.RemoveStaleExcept(nil)
}

// RemoveStaleExcept skips filesystem checks for repositories found during this scan.
func (idx *Index) RemoveStaleExcept(foundPaths map[string]struct{}) int {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	removed := 0
	for path, repo := range idx.Repos {
		if _, found := foundPaths[path]; found {
			continue
		}
		if _, err := os.Stat(filepath.Join(repo.AbsPath, ".git")); os.IsNotExist(err) {
			delete(idx.Repos, path)
			removed++
		}
	}
	if foundPaths != nil {
		for path := range idx.RejectedMarkers {
			if _, found := foundPaths[path]; !found {
				delete(idx.RejectedMarkers, path)
			}
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

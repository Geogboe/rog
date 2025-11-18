package scanner

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/git"
	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/metadata"
)

// Scanner handles repository scanning
type Scanner struct {
	cfg         *config.Config
	idx         *index.Index
	globalMeta  *metadata.GlobalMeta
	checkRemote bool
	workers     int
}

// New creates a new scanner
func New(cfg *config.Config, idx *index.Index) *Scanner {
	return &Scanner{
		cfg:     cfg,
		idx:     idx,
		workers: runtime.NumCPU() * 2,
	}
}

// WithRemoteCheck enables remote status checking
func (s *Scanner) WithRemoteCheck(enabled bool) *Scanner {
	s.checkRemote = enabled
	return s
}

// Scan scans all configured roots for git repositories
func (s *Scanner) Scan() error {
	// Load global metadata
	globalMeta, err := metadata.LoadGlobalMeta()
	if err != nil {
		log.Printf("Warning: failed to load global metadata: %v", err)
		globalMeta = &metadata.GlobalMeta{}
	}
	s.globalMeta = globalMeta

	// Channel for discovered repos
	repoChan := make(chan string, 100)
	var wg sync.WaitGroup

	// Start worker pool
	for i := 0; i < s.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for repoPath := range repoChan {
				if err := s.processRepo(repoPath); err != nil {
					log.Printf("Warning: failed to process %s: %v", repoPath, err)
				}
			}
		}()
	}

	// Walk each root
	for _, root := range s.cfg.Roots {
		if err := s.walkRoot(root, repoChan); err != nil {
			log.Printf("Warning: failed to walk root %s: %v", root.Name, err)
		}
	}

	// Close channel and wait for workers
	close(repoChan)
	wg.Wait()

	return nil
}

// walkRoot walks a single root directory
func (s *Scanner) walkRoot(root config.Root, repoChan chan<- string) error {
	// Build exclude map for fast lookup
	excludeMap := make(map[string]bool)
	for _, exclude := range root.Exclude {
		excludeMap[exclude] = true
	}

	return filepath.WalkDir(root.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip inaccessible directories
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		// Skip non-directories
		if !d.IsDir() {
			return nil
		}

		// Check if excluded
		if excludeMap[d.Name()] {
			return fs.SkipDir
		}

		// Check max depth
		relPath, err := filepath.Rel(root.Path, path)
		if err != nil {
			return nil
		}
		depth := 0
		if relPath != "." {
			// Count path separators to determine depth
			depth = strings.Count(relPath, string(filepath.Separator)) + 1
		}
		if depth > root.MaxDepth {
			return fs.SkipDir
		}

		// Check if directory contains .git (making it a git repo)
		gitPath := filepath.Join(path, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			// This is a git repo, process it
			repoChan <- path
			// Don't descend into this repo
			return fs.SkipDir
		}

		return nil
	})
}

// processRepo processes a single repository
func (s *Scanner) processRepo(repoPath string) error {
	// Verify it's actually a git repo
	if !git.IsGitRepo(repoPath) {
		return fmt.Errorf("not a git repository")
	}

	// Determine root and relative path
	root, relPath := s.findRoot(repoPath)
	if root == "" {
		return fmt.Errorf("could not determine root for %s", repoPath)
	}

	// Create repo entry
	repo := &index.Repo{
		Name:    filepath.Base(repoPath),
		Root:    root,
		RelPath: relPath,
		AbsPath: repoPath,
	}

	// Get git information
	if branch, err := git.GetBranch(repoPath); err == nil {
		repo.CurrentBranch = branch
	}

	if commit, err := git.GetLastCommit(repoPath); err == nil {
		repo.LastCommitTime = commit.Timestamp
		repo.LastCommitAuthor = commit.Author
		repo.LastCommitHash = commit.Hash
	}

	if status, err := git.GetStatus(repoPath); err == nil {
		repo.IsDirty = status.IsDirty
		repo.HasUntracked = status.HasUntracked
	}

	if remoteURL, err := git.GetRemoteURL(repoPath); err == nil {
		repo.RemoteURL = remoteURL
		repo.Host = git.ExtractHost(remoteURL)
	}

	// Check remote status if requested
	if s.checkRemote {
		if remoteStatus, err := git.GetRemoteStatus(repoPath); err == nil {
			repo.Ahead = remoteStatus.Ahead
			repo.Behind = remoteStatus.Behind
		}
	}

	// Detect language
	repo.PrimaryLanguage = DetectLanguage(repoPath)

	// Read metadata
	repoMeta, _ := metadata.ReadRepoMeta(repoPath)
	globalMeta := metadata.FindGlobalMeta(s.globalMeta, root, relPath)

	// Get existing repo data to preserve certain fields
	var existingMeta *metadata.RepoMeta
	if existing, ok := s.idx.Get(repoPath); ok {
		existingMeta = &metadata.RepoMeta{
			Description:     existing.Description,
			Tags:            existing.Tags,
			PrimaryLanguage: existing.PrimaryLanguage,
		}
	}

	// Merge metadata
	mergedMeta := metadata.MergeMeta(existingMeta, repoMeta, globalMeta)

	// Apply merged metadata
	if mergedMeta.Description != "" {
		repo.Description = mergedMeta.Description
		if repoMeta != nil && repoMeta.Description != "" {
			repo.DescriptionSource = "manual"
		} else if globalMeta != nil && globalMeta.Description != "" {
			repo.DescriptionSource = "global"
		}
	}

	if len(mergedMeta.Tags) > 0 {
		repo.Tags = mergedMeta.Tags
		if repoMeta != nil && len(repoMeta.Tags) > 0 {
			repo.TagsSource = "manual"
		} else if globalMeta != nil && len(globalMeta.Tags) > 0 {
			repo.TagsSource = "global"
		}
	}

	if mergedMeta.PrimaryLanguage != "" {
		repo.PrimaryLanguage = mergedMeta.PrimaryLanguage
	}

	// Upsert into index
	s.idx.Upsert(repo)

	return nil
}

// findRoot determines which root a repository belongs to
func (s *Scanner) findRoot(repoPath string) (string, string) {
	for _, root := range s.cfg.Roots {
		if rel, err := filepath.Rel(root.Path, repoPath); err == nil {
			if rel[0] != '.' && rel != "" {
				return root.Name, rel
			}
			if rel == "." {
				return root.Name, ""
			}
		}
	}
	return "", ""
}

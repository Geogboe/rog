package scanner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/git"
	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/logger"
	"github.com/Geogboe/rog/internal/metadata"
)

// Scanner handles repository scanning
type Scanner struct {
	cfg         *config.Config
	idx         *index.Index
	globalMeta  *metadata.GlobalMeta
	checkRemote bool
	workers     int
	dryRun      bool
	metrics     *ScanMetrics
}

// ScanMetrics tracks scanning statistics
type ScanMetrics struct {
	DirsScanned    int
	DirsExcluded   int
	DirsSkipped    int // max depth
	ReposFound     int
	TotalDirs      int
	DeepestPath    string
	DeepestDepth   int
	LargestDir     string
	LargestDirSize int // subdirs count
	StartTime      time.Time
	EndTime        time.Time
	mu             sync.Mutex
}

// New creates a new scanner
func New(cfg *config.Config, idx *index.Index) *Scanner {
	return &Scanner{
		cfg:     cfg,
		idx:     idx,
		workers: runtime.NumCPU() * 2,
		metrics: &ScanMetrics{},
	}
}

// WithRemoteCheck enables remote status checking
func (s *Scanner) WithRemoteCheck(enabled bool) *Scanner {
	s.checkRemote = enabled
	return s
}

// WithDryRun enables dry-run mode (collect metrics without processing)
func (s *Scanner) WithDryRun(enabled bool) *Scanner {
	s.dryRun = enabled
	return s
}

// GetMetrics returns scanning metrics
func (s *Scanner) GetMetrics() *ScanMetrics {
	return s.metrics
}

// Scan scans all configured roots for git repositories
func (s *Scanner) Scan() error {
	s.metrics.StartTime = time.Now()
	defer func() {
		s.metrics.EndTime = time.Now()
	}()

	// Load global metadata
	globalMeta, err := metadata.LoadGlobalMeta()
	if err != nil {
		logger.Verbose("Failed to load global metadata: %v", err)
		globalMeta = &metadata.GlobalMeta{}
	}
	s.globalMeta = globalMeta

	logger.Debug("Starting scan with %d workers across %d roots", s.workers, len(s.cfg.Roots))

	// Channel for discovered repos
	repoChan := make(chan string, 100)
	var wg sync.WaitGroup

	// Start worker pool for processing repos (only if not dry-run)
	if !s.dryRun {
		for i := 0; i < s.workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for repoPath := range repoChan {
					logger.Debug("Processing repository: %s", repoPath)
					if err := s.processRepo(repoPath); err != nil {
						logger.Verbose("Failed to process %s: %v", repoPath, err)
					}
				}
			}()
		}
	} else {
		// In dry-run mode, just count repos
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range repoChan {
				s.metrics.mu.Lock()
				s.metrics.ReposFound++
				s.metrics.mu.Unlock()
			}
		}()
	}

	// Walk each root in parallel
	var rootWg sync.WaitGroup
	for _, root := range s.cfg.Roots {
		rootWg.Add(1)
		go func(r config.Root) {
			defer rootWg.Done()
			logger.Debug("Walking root: %s (path: %s, max_depth: %d)", r.Name, r.Path, r.MaxDepth)
			if err := s.walkRootParallel(r, repoChan); err != nil {
				logger.Verbose("Failed to walk root %s: %v", r.Name, err)
			}
		}(root)
	}

	// Wait for all roots to finish walking, then close channel
	rootWg.Wait()
	close(repoChan)

	// Wait for all repo processing to finish
	wg.Wait()

	return nil
}

// walkRootParallel walks a single root directory with parallel subdirectory exploration
func (s *Scanner) walkRootParallel(root config.Root, repoChan chan<- string) error {
	// Merge global excludes with root-specific excludes
	excludes := make([]string, 0, len(s.cfg.GlobalExcludes)+len(root.Exclude))
	excludes = append(excludes, s.cfg.GlobalExcludes...)
	excludes = append(excludes, root.Exclude...)

	// Semaphore to limit concurrent directory reads
	sem := make(chan struct{}, runtime.NumCPU()*4)
	var walkWg sync.WaitGroup

	// Recursive parallel walker
	var walkDir func(path string, depth int)
	walkDir = func(path string, depth int) {
		defer walkWg.Done()

		// Track metrics
		s.metrics.mu.Lock()
		s.metrics.DirsScanned++
		s.metrics.TotalDirs++
		if depth > s.metrics.DeepestDepth {
			s.metrics.DeepestDepth = depth
			s.metrics.DeepestPath = path
		}
		s.metrics.mu.Unlock()

		// Check max depth
		if depth > root.MaxDepth {
			s.metrics.mu.Lock()
			s.metrics.DirsSkipped++
			s.metrics.mu.Unlock()
			logger.Debug("Skipping directory (max depth %d reached): %s", root.MaxDepth, path)
			return
		}

		// Check if this is a git repo
		gitPath := filepath.Join(path, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			logger.Debug("Found git repository: %s", path)
			repoChan <- path
			s.metrics.mu.Lock()
			s.metrics.ReposFound++
			s.metrics.mu.Unlock()
			// Don't descend into git repos
			return
		}

		// Read directory entries
		entries, err := os.ReadDir(path)
		if err != nil {
			// Permission denied or other error - skip this directory
			return
		}

		// Track largest directory
		subdirCount := 0
		for _, entry := range entries {
			if entry.IsDir() {
				subdirCount++
			}
		}
		if subdirCount > 0 {
			s.metrics.mu.Lock()
			if subdirCount > s.metrics.LargestDirSize {
				s.metrics.LargestDirSize = subdirCount
				s.metrics.LargestDir = path
			}
			s.metrics.mu.Unlock()
		}

		// Process subdirectories in parallel
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			dirName := entry.Name()
			fullPath := filepath.Join(path, dirName)

			// Check if excluded
			if s.isExcluded(dirName, fullPath, root.Path, excludes) {
				logger.Debug("Skipping excluded directory: %s", fullPath)
				s.metrics.mu.Lock()
				s.metrics.DirsExcluded++
				s.metrics.TotalDirs++ // Count it but don't scan it
				s.metrics.mu.Unlock()
				continue
			}

			// Acquire semaphore and spawn goroutine for subdirectory
			sem <- struct{}{}
			walkWg.Add(1)
			go func(subPath string, d int) {
				defer func() { <-sem }()
				walkDir(subPath, d)
			}(fullPath, depth+1)
		}
	}

	// Start walking from root
	walkWg.Add(1)
	go walkDir(root.Path, 0)

	// Wait for all walking to complete
	walkWg.Wait()

	return nil
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

	// Extract README description as fallback
	readmeDesc := extractReadmeDescription(repoPath)

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
	} else if readmeDesc != "" {
		// Use README description as fallback
		repo.Description = readmeDesc
		repo.DescriptionSource = "readme"
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
// When multiple roots match (nested roots), it returns the most specific one (longest path)
func (s *Scanner) findRoot(repoPath string) (string, string) {
	var bestMatch struct {
		name    string
		relPath string
		pathLen int
	}

	for _, root := range s.cfg.Roots {
		if rel, err := filepath.Rel(root.Path, repoPath); err == nil {
			// Check if this path is within the root (not outside with ..)
			if len(rel) > 0 && rel[0] != '.' {
				// This is a valid match
				rootPathLen := len(root.Path)
				if rootPathLen > bestMatch.pathLen {
					bestMatch.name = root.Name
					bestMatch.relPath = rel
					bestMatch.pathLen = rootPathLen
				}
			} else if rel == "." {
				// Repo is at the root itself
				rootPathLen := len(root.Path)
				if rootPathLen > bestMatch.pathLen {
					bestMatch.name = root.Name
					bestMatch.relPath = ""
					bestMatch.pathLen = rootPathLen
				}
			}
		}
	}

	return bestMatch.name, bestMatch.relPath
}

// isExcluded checks if a directory should be excluded based on patterns
// Supports both exact matches and glob patterns (e.g., "**/node_modules")
func (s *Scanner) isExcluded(dirName, fullPath, rootPath string, excludes []string) bool {
	// Get relative path from root for pattern matching
	relPath, err := filepath.Rel(rootPath, fullPath)
	if err != nil {
		relPath = dirName
	}

	for _, pattern := range excludes {
		// Try exact basename match first (fast path)
		if pattern == dirName {
			return true
		}

		// Try glob pattern match on basename
		if matched, _ := filepath.Match(pattern, dirName); matched {
			return true
		}

		// Try glob pattern match on relative path
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}

		// Try glob pattern match on full relative path with ** support
		// Convert ** to match any path components
		if strings.Contains(pattern, "**") {
			// Simple ** handling: "**/<name>" matches any depth
			if strings.HasPrefix(pattern, "**/") {
				suffix := pattern[3:]
				if strings.HasSuffix(relPath, suffix) || dirName == suffix {
					return true
				}
			}
		}
	}

	return false
}

// extractReadmeDescription extracts a description from README.md
// It reads the first non-header line and returns either the first sentence
// or truncates to ~140 characters
func extractReadmeDescription(repoPath string) string {
	// Try common README file names
	readmeNames := []string{"README.md", "README.MD", "Readme.md", "readme.md", "README"}

	var readmePath string
	for _, name := range readmeNames {
		path := filepath.Join(repoPath, name)
		if _, err := os.Stat(path); err == nil {
			readmePath = path
			break
		}
	}

	if readmePath == "" {
		return ""
	}

	file, err := os.Open(readmePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip markdown headers (lines starting with #)
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Skip HTML comments
		if strings.HasPrefix(line, "<!--") {
			continue
		}

		// Skip badges and images
		if strings.HasPrefix(line, "[![") || strings.HasPrefix(line, "![") {
			continue
		}

		// Found first real content line
		desc := line

		// Try to extract first sentence (up to first period followed by space or end)
		if idx := strings.Index(desc, ". "); idx != -1 {
			desc = desc[:idx+1]
		} else if idx := strings.Index(desc, ".\n"); idx != -1 {
			desc = desc[:idx+1]
		}

		// Truncate to ~140 chars if still too long
		const maxLength = 140
		if len(desc) > maxLength {
			// Try to cut at word boundary
			if idx := strings.LastIndex(desc[:maxLength], " "); idx != -1 {
				desc = desc[:idx] + "..."
			} else {
				desc = desc[:maxLength] + "..."
			}
		}

		return desc
	}

	return ""
}

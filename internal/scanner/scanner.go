package scanner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
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
	"github.com/Geogboe/rog/internal/scanner/discovery"
	"github.com/Geogboe/rog/internal/wsl"
)

var errNotGitRepo = errors.New("not a git repository")

// Scanner handles repository scanning
type Scanner struct {
	cfg           *config.Config
	idx           *index.Index
	globalMeta    *metadata.GlobalMeta
	wslScan       WSLScanFunc
	checkRemote   bool
	reuseExisting bool
	workers       int
	dryRun        bool
	metrics       *ScanMetrics
	foundPaths    map[string]struct{}
	completeRoots map[string]struct{}
	foundMu       sync.Mutex
}

// ScanMetrics tracks scanning statistics
type ScanMetrics struct {
	DirsScanned       int
	DirsExcluded      int
	DirsSkipped       int // max depth
	ReposFound        int
	ReposReused       int
	ReposRefreshed    int
	StatusUnavailable int
	StatusTimeouts    int
	DiscoveryDuration time.Duration
	GitDuration       time.Duration
	MetadataDuration  time.Duration
	TransportDuration time.Duration
	CurrentRoot       string
	CurrentRepo       string
	RootsTotal        int
	RootsCompleted    int
	TotalDirs         int
	DeepestPath       string
	DeepestDepth      int
	LargestDir        string
	LargestDirSize    int // subdirs count
	StartTime         time.Time
	EndTime           time.Time
	mu                sync.Mutex
}

// New creates a new scanner
func New(cfg *config.Config, idx *index.Index) *Scanner {
	workers := runtime.NumCPU() * 2
	if workers > 8 {
		workers = 8
	}
	return &Scanner{
		cfg:           cfg,
		idx:           idx,
		workers:       workers,
		metrics:       &ScanMetrics{},
		foundPaths:    make(map[string]struct{}),
		completeRoots: make(map[string]struct{}),
	}
}

// WithRemoteCheck enables remote status checking
func (s *Scanner) WithRemoteCheck(enabled bool) *Scanner {
	s.checkRemote = enabled
	return s
}

// WithReuseExisting keeps indexed metadata while still discovering new repositories.
func (s *Scanner) WithReuseExisting(enabled bool) *Scanner {
	s.reuseExisting = enabled
	return s
}

// WSLResult contains results produced by one Linux worker process.
type WSLResult struct {
	Repos                                                []*index.Repo
	Found                                                []string
	Candidates                                           int
	Reused, Refreshed, StatusUnavailable, StatusTimeouts int
	Discovery, Git, Metadata                             time.Duration
	CompleteRoots                                        []string
	Warnings                                             []string
}

// WSLScanFunc scans all roots in one distro and returns native Linux results.
type WSLScanFunc func(context.Context, []config.Root, []string, []*index.Repo, *metadata.GlobalMeta, bool, bool, bool, func(string, string, int, int, int)) (WSLResult, error)

// WithWSLScan supplies the Windows-to-WSL transport without linking it into the Linux worker.
func (s *Scanner) WithWSLScan(fn WSLScanFunc) *Scanner {
	s.wslScan = fn
	return s
}

// WithGlobalMeta supplies metadata from the parent CLI process to a WSL worker.
func (s *Scanner) WithGlobalMeta(meta *metadata.GlobalMeta) *Scanner {
	s.globalMeta = meta
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

// SnapshotMetrics returns a consistent copy of the current scan metrics.
func (s *Scanner) SnapshotMetrics() ScanMetrics {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()

	return ScanMetrics{
		DirsScanned:       s.metrics.DirsScanned,
		DirsExcluded:      s.metrics.DirsExcluded,
		DirsSkipped:       s.metrics.DirsSkipped,
		ReposFound:        s.metrics.ReposFound,
		ReposReused:       s.metrics.ReposReused,
		ReposRefreshed:    s.metrics.ReposRefreshed,
		StatusUnavailable: s.metrics.StatusUnavailable,
		StatusTimeouts:    s.metrics.StatusTimeouts,
		DiscoveryDuration: s.metrics.DiscoveryDuration,
		GitDuration:       s.metrics.GitDuration,
		MetadataDuration:  s.metrics.MetadataDuration,
		TransportDuration: s.metrics.TransportDuration,
		CurrentRoot:       s.metrics.CurrentRoot,
		CurrentRepo:       s.metrics.CurrentRepo,
		RootsTotal:        s.metrics.RootsTotal,
		RootsCompleted:    s.metrics.RootsCompleted,
		TotalDirs:         s.metrics.TotalDirs,
		DeepestPath:       s.metrics.DeepestPath,
		DeepestDepth:      s.metrics.DeepestDepth,
		LargestDir:        s.metrics.LargestDir,
		LargestDirSize:    s.metrics.LargestDirSize,
		StartTime:         s.metrics.StartTime,
		EndTime:           s.metrics.EndTime,
	}
}

func (s *Scanner) markFound(repoPath string) {
	s.foundMu.Lock()
	s.foundPaths[repoPath] = struct{}{}
	s.foundMu.Unlock()
}

// FoundPaths returns repository paths discovered by the last scan.
func (s *Scanner) FoundPaths() map[string]struct{} {
	s.foundMu.Lock()
	defer s.foundMu.Unlock()
	found := make(map[string]struct{}, len(s.foundPaths))
	for path := range s.foundPaths {
		found[path] = struct{}{}
	}
	return found
}

// IncompleteError means one or more roots failed; completed roots can still be saved.
type IncompleteError struct{ Errors []error }

func (e *IncompleteError) Error() string { return errors.Join(e.Errors...).Error() }

// CompletedRoots names roots whose discovery finished without an error.
func (s *Scanner) CompletedRoots() map[string]struct{} {
	s.foundMu.Lock()
	defer s.foundMu.Unlock()
	copy := make(map[string]struct{}, len(s.completeRoots))
	for name := range s.completeRoots {
		copy[name] = struct{}{}
	}
	return copy
}

func (s *Scanner) markRootComplete(name string) {
	s.foundMu.Lock()
	s.completeRoots[name] = struct{}{}
	s.foundMu.Unlock()
}

// Scan scans all configured roots for git repositories.
func (s *Scanner) Scan() error { return s.ScanContext(context.Background()) }

// ScanContext cancels discovery and WSL transport when the caller stops a scan.
func (s *Scanner) ScanContext(ctx context.Context) error {
	s.foundMu.Lock()
	s.foundPaths = make(map[string]struct{})
	s.completeRoots = make(map[string]struct{})
	s.foundMu.Unlock()
	s.metrics.StartTime = time.Now()
	defer func() {
		s.metrics.EndTime = time.Now()
	}()
	s.metrics.mu.Lock()
	s.metrics.RootsTotal = len(s.cfg.Roots)
	for i := range s.cfg.Roots {
		s.cfg.Roots[i].Path = normalizeConfiguredRootPath(s.cfg.Roots[i], runtime.GOOS == "windows")
	}
	s.metrics.mu.Unlock()

	// Load global metadata
	if s.globalMeta == nil {
		metaStart := time.Now()
		globalMeta, err := metadata.LoadGlobalMeta()
		if err != nil {
			logger.Verbose("Failed to load global metadata: %v", err)
			globalMeta = &metadata.GlobalMeta{}
		}
		s.globalMeta = globalMeta
		s.metrics.mu.Lock()
		s.metrics.MetadataDuration += time.Since(metaStart)
		s.metrics.mu.Unlock()
	}

	for i := range s.cfg.Roots {
		root := &s.cfg.Roots[i]
		if !root.WSL || runtime.GOOS != "windows" {
			continue
		}
		if root.WSLDistro == "" {
			distro, err := wsl.GetDefaultDistro()
			if err != nil {
				return fmt.Errorf("WSL root %s: %w", root.Name, err)
			}
			root.WSLDistro = distro
		}
		if s.wslScan == nil {
			return fmt.Errorf("WSL root %s requires a native WSL worker", root.Name)
		}
	}

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
					if ctx.Err() != nil {
						continue
					}
					s.metrics.mu.Lock()
					s.metrics.CurrentRepo = filepath.Base(repoPath)
					s.metrics.mu.Unlock()
					if s.reuseExisting {
						if existing, ok := s.idx.Get(repoPath); ok {
							rootName, relPath := s.findRoot(repoPath)
							if existing.Root == rootName && existing.RelPath == relPath {
								s.markFound(repoPath)
								s.metrics.mu.Lock()
								s.metrics.ReposReused++
								s.metrics.mu.Unlock()
								continue
							}
						}
						if s.idx.IsRejectedUnchanged(repoPath) {
							continue
						}
					}
					logger.Debug("Processing repository: %s", repoPath)
					if err := s.processRepo(repoPath); err != nil {
						if errors.Is(err, errNotGitRepo) {
							s.idx.RememberRejected(repoPath)
						} else {
							// An inspection failure must not erase cached metadata.
							s.markFound(repoPath)
						}
						logger.Verbose("Failed to process %s: %v", repoPath, err)
					} else {
						s.markFound(repoPath)
						s.idx.ForgetRejected(repoPath)
					}
				}
			}()
		}
	} else {
		// Discovery records the count; drain the channel without processing repos.
		wg.Add(1)
		go func() {
			defer wg.Done()
			for repoPath := range repoChan {
				s.foundMu.Lock()
				s.foundPaths[repoPath] = struct{}{}
				s.foundMu.Unlock()
			}
		}()
	}

	// Scan native roots independently and WSL roots once per distro.
	rootErrors := make(chan error, len(s.cfg.Roots))
	var rootWg sync.WaitGroup
	wslGroups := make(map[string][]config.Root)
	for _, root := range s.cfg.Roots {
		if root.WSL && runtime.GOOS == "windows" {
			wslGroups[root.WSLDistro] = append(wslGroups[root.WSLDistro], root)
			continue
		}
		rootWg.Add(1)
		go func(r config.Root) {
			defer rootWg.Done()
			defer func() { s.metrics.mu.Lock(); s.metrics.RootsCompleted++; s.metrics.mu.Unlock() }()
			s.metrics.mu.Lock()
			s.metrics.CurrentRoot = r.Name
			s.metrics.mu.Unlock()
			if err := s.walkRootNative(ctx, r, repoChan); err != nil {
				rootErrors <- fmt.Errorf("root %s: %w", r.Name, err)
			} else {
				s.markRootComplete(r.Name)
			}
		}(root)
	}
	for distro, roots := range wslGroups {
		rootWg.Add(1)
		go func(distro string, roots []config.Root) {
			defer rootWg.Done()
			defer func() { s.metrics.mu.Lock(); s.metrics.RootsCompleted += len(roots); s.metrics.mu.Unlock() }()
			s.metrics.mu.Lock()
			s.metrics.CurrentRoot = distro
			s.metrics.CurrentRepo = ""
			s.metrics.mu.Unlock()
			transportStart := time.Now()
			lastFound, lastRefreshed, lastReused := 0, 0, 0
			onProgress := func(root, repo string, found, refreshed, reused int) {
				s.metrics.mu.Lock()
				s.metrics.ReposFound += found - lastFound
				s.metrics.ReposRefreshed += refreshed - lastRefreshed
				s.metrics.ReposReused += reused - lastReused
				if root != "" {
					s.metrics.CurrentRoot = distro + "/" + root
				}
				if repo != "" {
					s.metrics.CurrentRepo = repo
				}
				lastFound, lastRefreshed, lastReused = found, refreshed, reused
				s.metrics.mu.Unlock()
			}
			result, err := s.wslScan(ctx, roots, s.cfg.GlobalExcludes, s.idx.List(), s.globalMeta, !s.reuseExisting, s.checkRemote, s.dryRun, onProgress)
			s.metrics.mu.Lock()
			s.metrics.TransportDuration += time.Since(transportStart)
			s.metrics.mu.Unlock()
			if err != nil {
				rootErrors <- fmt.Errorf("WSL distro %s: %w", distro, err)
				return
			}
			s.metrics.mu.Lock()
			s.metrics.ReposFound += result.Candidates - lastFound
			s.metrics.ReposReused += result.Reused - lastReused
			s.metrics.ReposRefreshed += result.Refreshed - lastRefreshed
			s.metrics.StatusUnavailable += result.StatusUnavailable
			s.metrics.StatusTimeouts += result.StatusTimeouts
			s.metrics.DiscoveryDuration += result.Discovery
			s.metrics.GitDuration += result.Git
			s.metrics.MetadataDuration += result.Metadata
			s.metrics.mu.Unlock()
			for _, warning := range result.Warnings {
				rootErrors <- fmt.Errorf("WSL distro %s: %s", distro, warning)
			}
			for _, name := range result.CompleteRoots {
				s.markRootComplete(name)
			}
			if s.dryRun {
				return
			}
			s.foundMu.Lock()
			for _, p := range result.Found {
				s.foundPaths[p] = struct{}{}
			}
			s.foundMu.Unlock()
			for _, repo := range result.Repos {
				s.idx.Upsert(repo)
			}
		}(distro, roots)
	}

	// Wait for all roots to finish walking, then close channel
	rootWg.Wait()
	close(rootErrors)
	close(repoChan)

	// Wait for all repo processing to finish
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	var failed []error
	for err := range rootErrors {
		failed = append(failed, err)
	}
	if len(failed) > 0 {
		return &IncompleteError{Errors: failed}
	}
	return nil
}

// normalizeConfiguredRootPath preserves Linux syntax for WSL roots in Windows configs.
func normalizeConfiguredRootPath(root config.Root, windows bool) string {
	if windows && root.WSL {
		return path.Clean(root.Path)
	}
	return normalizeScanPath(root.Path)
}

func rootScanPath(root config.Root) string {
	if root.WSL && runtime.GOOS == "windows" {
		return wsl.TranslatePathToWindows(root.WSLDistro, root.Path)
	}
	return root.Path
}

// walkRootNative uses the same discovery engine on local Windows and Linux roots.
func (s *Scanner) walkRootNative(ctx context.Context, root config.Root, repoChan chan<- string) error {
	started := time.Now()
	defer func() { s.metrics.mu.Lock(); s.metrics.DiscoveryDuration += time.Since(started); s.metrics.mu.Unlock() }()
	excludes := append(append([]string{}, s.cfg.GlobalExcludes...), root.Exclude...)
	return discovery.Walk(ctx, root.Path, root.MaxDepth,
		func(name, fullPath string) bool { return s.isExcluded(name, fullPath, root.Path, excludes) },
		func(repoPath string) error {
			repoPath = normalizeScanPath(repoPath)
			s.metrics.mu.Lock()
			s.metrics.ReposFound++
			s.metrics.mu.Unlock()
			select {
			case repoChan <- repoPath:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
}

// processRepo processes a single repository
func (s *Scanner) processRepo(repoPath string) error {
	repoPath = normalizeScanPath(repoPath)
	rootName, relPath := s.findRoot(repoPath)
	if rootName == "" {
		return fmt.Errorf("could not determine root for %s", repoPath)
	}

	gitStart := time.Now()
	info, err := git.GetRepoInfo(repoPath)
	if err != nil {
		return errNotGitRepo
	}
	repo := &index.Repo{Name: filepath.Base(repoPath), Root: rootName, RelPath: relPath, AbsPath: repoPath}
	repo.CurrentBranch = info.Branch
	if info.Commit != nil {
		repo.LastCommitTime = info.Commit.Timestamp
		repo.LastCommitAuthor = info.Commit.Author
		repo.LastCommitHash = info.Commit.Hash
	}
	repo.RemoteURL = info.RemoteURL
	repo.Host = info.Host
	if status, err := git.GetStatus(repoPath); err == nil {
		repo.IsDirty = status.IsDirty
		repo.HasUntracked = status.HasUntracked
	} else {
		logger.Verbose("Git status unavailable for %s: %v", repoPath, err)
		repo.StatusUnavailable = true
		s.metrics.mu.Lock()
		s.metrics.StatusUnavailable++
		if errors.Is(err, context.DeadlineExceeded) {
			s.metrics.StatusTimeouts++
		}
		s.metrics.mu.Unlock()
	}
	if s.checkRemote {
		if remoteStatus, err := git.GetRemoteStatus(repoPath); err == nil {
			repo.Ahead = remoteStatus.Ahead
			repo.Behind = remoteStatus.Behind
		}
	}

	s.metrics.mu.Lock()
	s.metrics.GitDuration += time.Since(gitStart)
	s.metrics.mu.Unlock()
	metaStart := time.Now()
	// Detect language
	repo.PrimaryLanguage = DetectLanguage(repoPath)

	// Extract README description as fallback
	readmeDesc := extractReadmeDescription(repoPath)

	// Read metadata
	repoMeta, _ := metadata.ReadRepoMeta(repoPath)
	globalMeta := metadata.FindGlobalMeta(s.globalMeta, rootName, relPath)

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
	s.metrics.mu.Lock()
	s.metrics.ReposRefreshed++
	s.metrics.MetadataDuration += time.Since(metaStart)
	s.metrics.mu.Unlock()

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
		scanPath := rootScanPath(root)
		if rel, ok := pathWithinRoot(scanPath, repoPath); ok {
			rootPathLen := len(normalizeScanPath(scanPath))
			if rootPathLen > bestMatch.pathLen {
				bestMatch.name = root.Name
				bestMatch.relPath = rel
				bestMatch.pathLen = rootPathLen
			}
		}
	}

	return bestMatch.name, bestMatch.relPath
}

// isExcluded checks if a directory should be excluded based on patterns
// Supports both exact matches and glob patterns (e.g., "**/node_modules")
func (s *Scanner) isExcluded(dirName, fullPath, rootPath string, excludes []string) bool {
	// Get relative path from root for pattern matching
	relPath, ok := pathWithinRoot(rootPath, fullPath)
	if !ok {
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

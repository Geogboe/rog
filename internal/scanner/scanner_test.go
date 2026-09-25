package scanner

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractReadmeDescription(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expected    string
		description string
	}{
		{
			name: "simple description",
			content: `# My Project

This is a simple project description.

More text here.`,
			expected:    "This is a simple project description.",
			description: "should extract first non-header line as first sentence",
		},
		{
			name: "multiple sentences - take first",
			content: `# Project

This is the first sentence. This is the second sentence.`,
			expected:    "This is the first sentence.",
			description: "should extract only first sentence when multiple exist",
		},
		{
			name: "long description - truncate",
			content: `# Project

This is a very long description that exceeds the maximum length limit and should be truncated at a word boundary to ensure readable output without cutting words in half.`,
			expected:    "This is a very long description that exceeds the maximum length limit and should be truncated at a word boundary to ensure readable output...",
			description: "should truncate long descriptions at word boundary",
		},
		{
			name: "skip headers",
			content: `# Main Header

## Sub Header

### Another Header

This is the actual description.`,
			expected:    "This is the actual description.",
			description: "should skip all markdown headers",
		},
		{
			name: "skip empty lines",
			content: `# Header



This is the description after empty lines.`,
			expected:    "This is the description after empty lines.",
			description: "should skip empty lines",
		},
		{
			name: "skip badges",
			content: `# Project

[![Build Status](https://img.shields.io/badge/build-passing-brightgreen)](https://example.com)
![Coverage](https://img.shields.io/badge/coverage-90%25-green)

A real project description here.`,
			expected:    "A real project description here.",
			description: "should skip badge lines",
		},
		{
			name: "skip HTML comments",
			content: `# Project

<!-- This is a comment -->

The actual description.`,
			expected:    "The actual description.",
			description: "should skip HTML comments",
		},
		{
			name: "no description - only headers",
			content: `# Header

## Another Header

### Yet Another`,
			expected:    "",
			description: "should return empty string when only headers exist",
		},
		{
			name:        "empty file",
			content:     ``,
			expected:    "",
			description: "should handle empty file",
		},
		{
			name: "description without period",
			content: `# Project

A description without a period at the end`,
			expected:    "A description without a period at the end",
			description: "should handle descriptions without periods",
		},
		{
			name: "mixed case readme variations",
			content: `# Test

Description from README.`,
			expected:    "Description from README.",
			description: "should work with various README filename cases",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir, err := os.MkdirTemp("", "rog-scanner-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			// Write README.md
			readmePath := filepath.Join(tmpDir, "README.md")
			err = os.WriteFile(readmePath, []byte(tt.content), 0644)
			require.NoError(t, err)

			// Extract description
			result := extractReadmeDescription(tmpDir)

			// Assert
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestExtractReadmeDescriptionFilenameVariations(t *testing.T) {
	filenames := []string{"README.md", "README.MD", "Readme.md", "readme.md", "README"}

	for _, filename := range filenames {
		t.Run(filename, func(t *testing.T) {
			// Create temp directory
			tmpDir, err := os.MkdirTemp("", "rog-scanner-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			// Write README with specific filename
			content := `# Test

A test description.`
			readmePath := filepath.Join(tmpDir, filename)
			err = os.WriteFile(readmePath, []byte(content), 0644)
			require.NoError(t, err)

			// Extract description
			result := extractReadmeDescription(tmpDir)

			// Assert
			assert.Equal(t, "A test description.", result)
		})
	}
}

func TestExtractReadmeDescriptionNoReadme(t *testing.T) {
	// Create temp directory without README
	tmpDir, err := os.MkdirTemp("", "rog-scanner-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Extract description
	result := extractReadmeDescription(tmpDir)

	// Should return empty string
	assert.Equal(t, "", result)
}

func TestExtractReadmeDescriptionReadmeWithOnlyWhitespace(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "rog-scanner-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Write README with only whitespace
	readmePath := filepath.Join(tmpDir, "README.md")
	err = os.WriteFile(readmePath, []byte("   \n\n\t\n   "), 0644)
	require.NoError(t, err)

	// Extract description
	result := extractReadmeDescription(tmpDir)

	// Should return empty string
	assert.Equal(t, "", result)
}

func TestExtractReadmeDescriptionMaxLength(t *testing.T) {
	// Create a description that's exactly at the boundary
	tmpDir, err := os.MkdirTemp("", "rog-scanner-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a string that's exactly 140 characters
	desc := "This is exactly one hundred and forty characters long string that should not be truncated because it fits within the maximum length limit"
	content := "# Test\n\n" + desc

	readmePath := filepath.Join(tmpDir, "README.md")
	err = os.WriteFile(readmePath, []byte(content), 0644)
	require.NoError(t, err)

	// Extract description
	result := extractReadmeDescription(tmpDir)

	// Should return the full description without truncation
	assert.Equal(t, desc, result)
}

func TestExtractReadmeDescriptionSentenceExtraction(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "period with space",
			content:  "First sentence. Second sentence.",
			expected: "First sentence.",
		},
		{
			name:     "period with newline",
			content:  "First sentence.\nSecond sentence.",
			expected: "First sentence.",
		},
		{
			name:     "no period",
			content:  "Just one long sentence without ending period",
			expected: "Just one long sentence without ending period",
		},
		{
			name:     "period at end",
			content:  "Sentence with period at end.",
			expected: "Sentence with period at end.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "rog-scanner-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			readmePath := filepath.Join(tmpDir, "README.md")
			err = os.WriteFile(readmePath, []byte("# Test\n\n"+tt.content), 0644)
			require.NoError(t, err)

			result := extractReadmeDescription(tmpDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWalkRootWithFdFindsOnlyGitMarkers(t *testing.T) {
	fdCommand := findFdCommand()
	if fdCommand == "" {
		t.Skip("fd or fdfind is not installed")
	}

	rootPath := t.TempDir()
	for _, name := range []string{"regular", "worktree", "not-a-repo", "excluded"} {
		require.NoError(t, os.Mkdir(filepath.Join(rootPath, name), 0755))
	}
	require.NoError(t, os.Mkdir(filepath.Join(rootPath, "regular", ".git"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(rootPath, "worktree", ".git"), []byte("gitdir: elsewhere"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(rootPath, "not-a-repo", ".github"), 0755))
	require.NoError(t, os.Mkdir(filepath.Join(rootPath, "excluded", ".git"), 0755))

	root := config.Root{Path: rootPath, MaxDepth: 1, Exclude: []string{"excluded"}}
	scan := New(&config.Config{GlobalExcludes: []string{".git"}}, nil)
	repos := make(chan string, 4)
	require.NoError(t, scan.walkRootWithFd(fdCommand, root, repos))
	close(repos)

	var found []string
	for repo := range repos {
		found = append(found, repo)
	}
	sort.Strings(found)
	assert.Equal(t, []string{filepath.Join(rootPath, "regular"), filepath.Join(rootPath, "worktree")}, found)
}

func TestScanDryRunCountsRepositoriesOnce(t *testing.T) {
	rootPath := t.TempDir()
	for _, name := range []string{"first", "second"} {
		require.NoError(t, os.MkdirAll(filepath.Join(rootPath, name, ".git"), 0755))
	}
	t.Setenv("ROG_DATA", t.TempDir())

	cfg := &config.Config{Roots: []config.Root{{Path: rootPath, MaxDepth: 1}}}
	scan := New(cfg, nil).WithDryRun(true)
	require.NoError(t, scan.Scan())
	assert.Equal(t, 2, scan.GetMetrics().ReposFound)
}

func TestWSLRepoPathFromGitMarker(t *testing.T) {
	got := wslRepoPathFromGitMarker("Ubuntu", "/home/user/projects/example/.git")
	assert.Equal(t, `\\wsl$\Ubuntu\home\user\projects\example`, got)
}

func TestRepoPathFromGitMarkerWithTrailingSeparator(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "project")
	marker := filepath.Join(repoPath, ".git") + string(filepath.Separator)
	assert.Equal(t, repoPath, repoPathFromGitMarker(marker))
}

func TestFindRootIncludesRootAndHiddenRepository(t *testing.T) {
	rootPath := t.TempDir()
	scan := New(&config.Config{Roots: []config.Root{{Name: "projects", Path: rootPath}}}, nil)

	for _, tt := range []struct {
		path string
		rel  string
	}{
		{rootPath, ""},
		{filepath.Join(rootPath, ".hidden"), ".hidden"},
	} {
		name, rel := scan.findRoot(tt.path)
		assert.Equal(t, "projects", name)
		assert.Equal(t, tt.rel, rel)
	}
}

func TestNormalizeConfiguredRootPathWSLOnWindows(t *testing.T) {
	got := normalizeConfiguredRootPath(config.Root{Path: "/home/user/projects/../projects", WSL: true}, true)
	if got != "/home/user/projects" {
		t.Fatalf("WSL root path = %q, want /home/user/projects", got)
	}
}

func TestScanReusesKnownReposAndDiscoversNewOnes(t *testing.T) {
	rootPath := t.TempDir()
	known := filepath.Join(rootPath, "known")
	newRepo := filepath.Join(rootPath, "new")
	for _, repoPath := range []string{known, newRepo} {
		cmd := exec.Command("git", "init", "--quiet", repoPath)
		require.NoError(t, cmd.Run())
	}
	t.Setenv("ROG_DATA", t.TempDir())
	idx := index.New()
	idx.Upsert(&index.Repo{AbsPath: known, Name: "known", Root: "projects", RelPath: "known", CurrentBranch: "cached-branch"})
	before, ok := idx.Get(known)
	require.True(t, ok)
	lastScan := before.LastScanAt

	cfg := &config.Config{Roots: []config.Root{{Name: "projects", Path: rootPath, MaxDepth: 1}}}
	scan := New(cfg, idx).WithReuseExisting(true)
	require.NoError(t, scan.Scan())
	got, ok := idx.Get(known)
	require.True(t, ok)
	assert.Equal(t, "cached-branch", got.CurrentBranch)
	assert.Equal(t, lastScan, got.LastScanAt)
	_, ok = idx.Get(newRepo)
	assert.True(t, ok, "scan should index newly discovered repositories")
	assert.Equal(t, 1, scan.GetMetrics().ReposReused)

	fullScan := New(cfg, idx).WithReuseExisting(false)
	require.NoError(t, fullScan.Scan())
	refreshed, ok := idx.Get(known)
	require.True(t, ok)
	assert.NotEqual(t, "cached-branch", refreshed.CurrentBranch)
	assert.True(t, refreshed.LastScanAt.After(lastScan))
	assert.Zero(t, fullScan.GetMetrics().ReposReused)
}

func TestScanCachesInvalidMarkerUntilFullRefresh(t *testing.T) {
	rootPath := t.TempDir()
	repoPath := filepath.Join(rootPath, "invalid")
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".git"), 0755))
	t.Setenv("ROG_DATA", t.TempDir())
	cfg := &config.Config{Roots: []config.Root{{Name: "projects", Path: rootPath, MaxDepth: 1}}}
	idx := index.New()

	require.NoError(t, New(cfg, idx).WithReuseExisting(true).Scan())
	first, ok := idx.RejectedMarkers[repoPath]
	require.True(t, ok)
	require.NoError(t, New(cfg, idx).WithReuseExisting(true).Scan())
	assert.Equal(t, first.CheckedAt, idx.RejectedMarkers[repoPath].CheckedAt)

	require.NoError(t, New(cfg, idx).WithReuseExisting(false).Scan())
	assert.True(t, idx.RejectedMarkers[repoPath].CheckedAt.After(first.CheckedAt))
}

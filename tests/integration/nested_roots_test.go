package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/scanner"
)

// TestNestedRoots tests that repos in nested roots are assigned to the most specific root
func TestNestedRoots(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested directory structure:
	// tmpDir/
	//   repo1/          <- should be in "projects" root
	//   go/
	//     repo2/        <- should be in "go" root (not "projects")
	//   rust/
	//     repo3/        <- should be in "rust" root (not "projects")

	repo1Dir := filepath.Join(tmpDir, "repo1")
	repo2Dir := filepath.Join(tmpDir, "go", "repo2")
	repo3Dir := filepath.Join(tmpDir, "rust", "repo3")

	os.MkdirAll(repo1Dir, 0755)
	os.MkdirAll(repo2Dir, 0755)
	os.MkdirAll(repo3Dir, 0755)

	initGitRepo(t, repo1Dir)
	initGitRepo(t, repo2Dir)
	initGitRepo(t, repo3Dir)

	// Create some files to make them identifiable
	os.WriteFile(filepath.Join(repo1Dir, "main.js"), []byte("console.log('repo1')"), 0644)
	os.WriteFile(filepath.Join(repo2Dir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(repo3Dir, "main.rs"), []byte("fn main() {}"), 0644)

	commitFiles(t, repo1Dir, "init repo1")
	commitFiles(t, repo2Dir, "init repo2")
	commitFiles(t, repo3Dir, "init repo3")

	// Create config with nested roots
	cfg := &config.Config{
		Roots: []config.Root{
			{
				Name:     "projects",
				Path:     tmpDir,
				MaxDepth: 3,
			},
			{
				Name:     "go",
				Path:     filepath.Join(tmpDir, "go"),
				MaxDepth: 2,
			},
			{
				Name:     "rust",
				Path:     filepath.Join(tmpDir, "rust"),
				MaxDepth: 2,
			},
		},
	}

	// Scan
	idx := index.New()
	scan := scanner.New(cfg, idx)
	require.NoError(t, scan.Scan())

	// Should find all 3 repos
	assert.Equal(t, 3, idx.Count())

	// Get repos
	repo1, ok := idx.Get(repo1Dir)
	require.True(t, ok, "repo1 should be in index")

	repo2, ok := idx.Get(repo2Dir)
	require.True(t, ok, "repo2 should be in index")

	repo3, ok := idx.Get(repo3Dir)
	require.True(t, ok, "repo3 should be in index")

	// Verify root assignments
	// repo1 should be in "projects" root
	assert.Equal(t, "projects", repo1.Root, "repo1 should be assigned to 'projects' root")
	assert.Equal(t, "repo1", repo1.RelPath, "repo1 should have correct RelPath")

	// repo2 should be in "go" root (most specific), NOT "projects"
	assert.Equal(t, "go", repo2.Root, "repo2 should be assigned to 'go' root (most specific path)")
	assert.Equal(t, "repo2", repo2.RelPath, "repo2 should have correct RelPath")

	// repo3 should be in "rust" root (most specific), NOT "projects"
	assert.Equal(t, "rust", repo3.Root, "repo3 should be assigned to 'rust' root (most specific path)")
	assert.Equal(t, "repo3", repo3.RelPath, "repo3 should have correct RelPath")
}

// TestNestedRootsFiltering tests that --root filtering works correctly with nested roots
func TestNestedRootsFiltering(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested structure
	repo1Dir := filepath.Join(tmpDir, "repo1")
	repo2Dir := filepath.Join(tmpDir, "go", "repo2")
	repo3Dir := filepath.Join(tmpDir, "go", "repo3")

	os.MkdirAll(repo1Dir, 0755)
	os.MkdirAll(repo2Dir, 0755)
	os.MkdirAll(repo3Dir, 0755)

	initGitRepo(t, repo1Dir)
	initGitRepo(t, repo2Dir)
	initGitRepo(t, repo3Dir)

	os.WriteFile(filepath.Join(repo1Dir, "main.js"), []byte("console.log('repo1')"), 0644)
	os.WriteFile(filepath.Join(repo2Dir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(repo3Dir, "main.go"), []byte("package main"), 0644)

	commitFiles(t, repo1Dir, "init")
	commitFiles(t, repo2Dir, "init")
	commitFiles(t, repo3Dir, "init")

	// Create config with nested roots
	cfg := &config.Config{
		Roots: []config.Root{
			{
				Name:     "projects",
				Path:     tmpDir,
				MaxDepth: 3,
			},
			{
				Name:     "go",
				Path:     filepath.Join(tmpDir, "go"),
				MaxDepth: 2,
			},
		},
	}

	// Scan
	idx := index.New()
	scan := scanner.New(cfg, idx)
	require.NoError(t, scan.Scan())

	assert.Equal(t, 3, idx.Count())

	// Now test filtering by root
	// Filtering by "go" should only return repo2 and repo3
	goRepos := idx.List()
	var goRootRepos []*index.Repo
	for _, r := range goRepos {
		if r.Root == "go" {
			goRootRepos = append(goRootRepos, r)
		}
	}

	assert.Equal(t, 2, len(goRootRepos), "Should have 2 repos in 'go' root")

	// Filtering by "projects" should only return repo1
	var projectsRepos []*index.Repo
	for _, r := range goRepos {
		if r.Root == "projects" {
			projectsRepos = append(projectsRepos, r)
		}
	}

	assert.Equal(t, 1, len(projectsRepos), "Should have 1 repo in 'projects' root")
	assert.Equal(t, "repo1", projectsRepos[0].Name, "projects root should only have repo1")
}

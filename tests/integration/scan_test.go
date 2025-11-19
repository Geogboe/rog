package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/scanner"
)

// initGitRepo initializes a git repo with proper config
func initGitRepo(t testing.TB, repoPath string) {
	t.Helper()

	// Init
	cmd := exec.Command("git", "init", repoPath)
	require.NoError(t, cmd.Run())

	// Configure
	cmd = exec.Command("git", "-C", repoPath, "config", "user.email", "test@test.com")
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "-C", repoPath, "config", "user.name", "Test User")
	require.NoError(t, cmd.Run())

	// Set default branch (ignore error for old git)
	exec.Command("git", "-C", repoPath, "config", "init.defaultBranch", "main").Run()

	// Disable GPG signing for tests
	exec.Command("git", "-C", repoPath, "config", "commit.gpgsign", "false").Run()
	exec.Command("git", "-C", repoPath, "config", "gpg.program", "").Run()
}

// commitFiles adds and commits all files in a repo
func commitFiles(t testing.TB, repoPath string, message string) {
	t.Helper()

	cmd := exec.Command("git", "-C", repoPath, "add", ".")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, output)
	}

	cmd = exec.Command("git", "-C", repoPath, "commit", "-m", message)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, output)
	}
}

// setupTestRepos creates a test directory with multiple git repos
func setupTestRepos(t testing.TB) string {
	tmpDir := t.TempDir()

	repos := []struct {
		name  string
		files map[string]string
	}{
		{
			name: "go-api",
			files: map[string]string{
				"go.mod":   "module github.com/test/go-api\n\ngo 1.21",
				"main.go":  "package main\n\nfunc main() {}",
				"api.go":   "package main\n\ntype API struct {}",
				"README.md": "# Go API Server",
			},
		},
		{
			name: "python-scripts",
			files: map[string]string{
				"requirements.txt": "requests==2.28.0\nclick==8.0.0",
				"main.py":          "#!/usr/bin/env python3\n\nprint('hello')",
				"utils.py":         "def helper(): pass",
				"README.md":        "# Python Scripts",
			},
		},
		{
			name: "rust-cli",
			files: map[string]string{
				"Cargo.toml": "[package]\nname = \"rust-cli\"\nversion = \"0.1.0\"",
				"main.rs":    "fn main() {\n    println!(\"Hello\");\n}",
				"README.md":  "# Rust CLI Tool",
			},
		},
	}

	for _, repo := range repos {
		repoDir := filepath.Join(tmpDir, repo.name)
		os.MkdirAll(repoDir, 0755)

		// Initialize git repo
		initGitRepo(t, repoDir)

		// Create files
		for filename, content := range repo.files {
			filePath := filepath.Join(repoDir, filename)
			require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
		}

		// Create initial commit
		commitFiles(t, repoDir, "Initial commit")
	}

	return tmpDir
}

func TestScanWorkflow(t *testing.T) {
	testDir := setupTestRepos(t)

	// Create config
	cfg := &config.Config{
		Roots: []config.Root{
			{
				Name:     "test",
				Path:     testDir,
				MaxDepth: 2,
				Exclude:  []string{"node_modules", "vendor"},
			},
		},
	}

	// Create index
	idx := index.New()

	// Create scanner
	scan := scanner.New(cfg, idx)

	// Run scan
	err := scan.Scan()
	require.NoError(t, err)

	// Verify results
	assert.Equal(t, 3, idx.Count())

	// Check Go repo
	repos := idx.GetByName("go-api")
	require.Equal(t, 1, len(repos))
	goRepo := repos[0]
	assert.Equal(t, "Go", goRepo.PrimaryLanguage)
	assert.Equal(t, "test", goRepo.Root)
	assert.Contains(t, []string{"main", "master"}, goRepo.CurrentBranch) // Support both old and new git
	assert.False(t, goRepo.IsDirty)
	assert.False(t, goRepo.LastCommitTime.IsZero())

	// Check Python repo
	repos = idx.GetByName("python-scripts")
	require.Equal(t, 1, len(repos))
	pyRepo := repos[0]
	assert.Equal(t, "Python", pyRepo.PrimaryLanguage)

	// Check Rust repo
	repos = idx.GetByName("rust-cli")
	require.Equal(t, 1, len(repos))
	rustRepo := repos[0]
	assert.Equal(t, "Rust", rustRepo.PrimaryLanguage)
}

func TestScanWithDirtyRepos(t *testing.T) {
	testDir := setupTestRepos(t)

	// Modify a file in one repo to make it dirty
	dirtyFile := filepath.Join(testDir, "go-api", "new.go")
	require.NoError(t, os.WriteFile(dirtyFile, []byte("package main"), 0644))

	// Create config
	cfg := &config.Config{
		Roots: []config.Root{
			{
				Name:     "test",
				Path:     testDir,
				MaxDepth: 2,
			},
		},
	}

	// Scan
	idx := index.New()
	scan := scanner.New(cfg, idx)
	require.NoError(t, scan.Scan())

	// Check dirty status
	repos := idx.GetByName("go-api")
	require.Equal(t, 1, len(repos))
	assert.True(t, repos[0].HasUntracked)
}

func TestScanExcludesDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create repos in various locations
	mainRepo := filepath.Join(tmpDir, "main-repo")
	nestedRepo := filepath.Join(tmpDir, "node_modules", "excluded-repo")

	for _, repoPath := range []string{mainRepo, nestedRepo} {
		os.MkdirAll(repoPath, 0755)
		initGitRepo(t, repoPath)
		os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("test"), 0644)
		commitFiles(t, repoPath, "init")
	}

	// Create config with exclusions
	cfg := &config.Config{
		Roots: []config.Root{
			{
				Name:     "test",
				Path:     tmpDir,
				MaxDepth: 3,
				Exclude:  []string{"node_modules"},
			},
		},
	}

	// Scan
	idx := index.New()
	scan := scanner.New(cfg, idx)
	require.NoError(t, scan.Scan())

	// Should only find main-repo, not the one in node_modules
	assert.Equal(t, 1, idx.Count())
	repos := idx.GetByName("main-repo")
	assert.Equal(t, 1, len(repos))
}

func TestScanMaxDepth(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested repos
	shallow := filepath.Join(tmpDir, "shallow")
	deep := filepath.Join(tmpDir, "level1", "level2", "level3", "deep")

	for _, repoPath := range []string{shallow, deep} {
		os.MkdirAll(repoPath, 0755)
		initGitRepo(t, repoPath)
		os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("test"), 0644)
		commitFiles(t, repoPath, "init")
	}

	// Create config with max depth 2
	cfg := &config.Config{
		Roots: []config.Root{
			{
				Name:     "test",
				Path:     tmpDir,
				MaxDepth: 2,
			},
		},
	}

	// Scan
	idx := index.New()
	scan := scanner.New(cfg, idx)
	require.NoError(t, scan.Scan())

	// Should only find shallow repo (deep is beyond max depth)
	assert.Equal(t, 1, idx.Count())
	repos := idx.GetByName("shallow")
	assert.Equal(t, 1, len(repos))
}

func TestIndexPersistence(t *testing.T) {
	testDir := setupTestRepos(t)
	indexDir := t.TempDir()

	// Set index location
	os.Setenv("ROG_DATA", indexDir)
	defer os.Unsetenv("ROG_DATA")

	// Create config
	cfg := &config.Config{
		Roots: []config.Root{
			{
				Name:     "test",
				Path:     testDir,
				MaxDepth: 2,
			},
		},
	}

	// Scan and save
	idx := index.New()
	scan := scanner.New(cfg, idx)
	require.NoError(t, scan.Scan())
	require.NoError(t, idx.Save())

	// Load from disk
	loadedIdx, err := index.Load()
	require.NoError(t, err)
	assert.Equal(t, 3, loadedIdx.Count())

	// Verify repos are intact
	repos := loadedIdx.GetByName("go-api")
	assert.Equal(t, 1, len(repos))
	assert.Equal(t, "Go", repos[0].PrimaryLanguage)
}

// Stress test: scan many repos
func TestScanStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	tmpDir := t.TempDir()

	// Create 50 repos
	for i := 0; i < 50; i++ {
		repoDir := filepath.Join(tmpDir, "repo-"+strconv.Itoa(i))
		os.MkdirAll(repoDir, 0755)
		initGitRepo(t, repoDir)

		// Create some files
		os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main"), 0644)
		commitFiles(t, repoDir, "init")
	}

	// Create config
	cfg := &config.Config{
		Roots: []config.Root{
			{
				Name:     "test",
				Path:     tmpDir,
				MaxDepth: 2,
			},
		},
	}

	// Scan
	idx := index.New()
	scan := scanner.New(cfg, idx)
	require.NoError(t, scan.Scan())

	assert.Equal(t, 50, idx.Count())
}

func TestGlobalExcludes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create repos in various excluded directories
	repos := map[string]string{
		"main-repo":                    "main-repo",
		"node_modules/dep":             "node_modules/dep",
		"vendor/lib":                   "vendor/lib",
		"build/artifact":               "build/artifact",
		"nested/node_modules/deep-dep": "nested/node_modules/deep-dep",
	}

	for _, repoPath := range repos {
		fullPath := filepath.Join(tmpDir, repoPath)
		os.MkdirAll(fullPath, 0755)
		initGitRepo(t, fullPath)
		os.WriteFile(filepath.Join(fullPath, "README.md"), []byte("test"), 0644)
		commitFiles(t, fullPath, "init")
	}

	// Create config with global excludes
	cfg := &config.Config{
		GlobalExcludes: []string{"node_modules", "vendor", "build"},
		Roots: []config.Root{
			{
				Name:     "test",
				Path:     tmpDir,
				MaxDepth: 5,
			},
		},
	}

	// Scan
	idx := index.New()
	scan := scanner.New(cfg, idx)
	require.NoError(t, scan.Scan())

	// Should only find main-repo (others are in excluded dirs)
	assert.Equal(t, 1, idx.Count())
	found := idx.GetByName("main-repo")
	assert.Equal(t, 1, len(found))
}

func TestGlobalExcludesWithRootOverrides(t *testing.T) {
	tmpDir := t.TempDir()

	// Create repos
	mainRepo := filepath.Join(tmpDir, "main-repo")
	customExcluded := filepath.Join(tmpDir, "custom-excluded", "repo")
	globalExcluded := filepath.Join(tmpDir, "vendor", "repo")

	for _, repoPath := range []string{mainRepo, customExcluded, globalExcluded} {
		os.MkdirAll(repoPath, 0755)
		initGitRepo(t, repoPath)
		os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("test"), 0644)
		commitFiles(t, repoPath, "init")
	}

	// Create config with both global and root-specific excludes
	cfg := &config.Config{
		GlobalExcludes: []string{"vendor"},
		Roots: []config.Root{
			{
				Name:     "test",
				Path:     tmpDir,
				MaxDepth: 5,
				Exclude:  []string{"custom-excluded"},
			},
		},
	}

	// Scan
	idx := index.New()
	scan := scanner.New(cfg, idx)
	require.NoError(t, scan.Scan())

	// Should only find main-repo (others are excluded)
	assert.Equal(t, 1, idx.Count())
	found := idx.GetByName("main-repo")
	assert.Equal(t, 1, len(found))
}

func TestGlobPatternExcludes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create repos with pattern-based names
	repos := []string{
		"main-repo",
		"test-cache/repo",
		"build-cache/repo",
		"random-cache/repo",
		"normal/repo",
	}

	for _, repoPath := range repos {
		fullPath := filepath.Join(tmpDir, repoPath)
		os.MkdirAll(fullPath, 0755)
		initGitRepo(t, fullPath)
		os.WriteFile(filepath.Join(fullPath, "README.md"), []byte("test"), 0644)
		commitFiles(t, fullPath, "init")
	}

	// Create config with glob pattern excludes
	cfg := &config.Config{
		GlobalExcludes: []string{"*-cache"},
		Roots: []config.Root{
			{
				Name:     "test",
				Path:     tmpDir,
				MaxDepth: 5,
			},
		},
	}

	// Scan
	idx := index.New()
	scan := scanner.New(cfg, idx)
	require.NoError(t, scan.Scan())

	// Should find main-repo and normal/repo (cache dirs excluded)
	assert.Equal(t, 2, idx.Count())
}

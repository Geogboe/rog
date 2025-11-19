package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/scanner"
)

// TestScanPerformanceDeepNesting tests scanning with deep directory nesting
func TestScanPerformanceDeepNesting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	tmpDir := t.TempDir()

	// Create deep nesting: 10 levels deep with 5 directories per level
	// This simulates a real workspace with nested projects
	repoCount := 0
	dirCount := 0

	// Create a function to recursively build directory tree
	var createNestedDirs func(basePath string, depth, maxDepth, branchFactor int)
	createNestedDirs = func(basePath string, depth, maxDepth, branchFactor int) {
		if depth > maxDepth {
			return
		}

		for i := 0; i < branchFactor; i++ {
			dirName := fmt.Sprintf("level%d-dir%d", depth, i)
			dirPath := filepath.Join(basePath, dirName)
			os.MkdirAll(dirPath, 0755)
			dirCount++

			// Create a git repo in some directories (every 3rd directory)
			if i%3 == 0 {
				initGitRepo(t, dirPath)
				os.WriteFile(filepath.Join(dirPath, "main.go"), []byte("package main"), 0644)
				commitFiles(t, dirPath, "init")
				repoCount++
			}

			// Recurse to next level
			createNestedDirs(dirPath, depth+1, maxDepth, branchFactor)
		}
	}

	t.Logf("Creating nested directory structure...")
	start := time.Now()
	createNestedDirs(tmpDir, 1, 10, 5)
	setupDuration := time.Since(start)
	t.Logf("Created %d directories and %d repos in %v", dirCount, repoCount, setupDuration)

	// Create config
	cfg := &config.Config{
		GlobalExcludes: []string{"node_modules", "vendor"},
		Roots: []config.Root{
			{
				Name:     "test",
				Path:     tmpDir,
				MaxDepth: 12,
			},
		},
	}

	// Benchmark scan
	t.Logf("Starting scan...")
	scanStart := time.Now()
	idx := index.New()
	scan := scanner.New(cfg, idx)
	require.NoError(t, scan.Scan())
	scanDuration := time.Since(scanStart)

	t.Logf("Scan completed in %v", scanDuration)
	t.Logf("Found %d repositories", idx.Count())
	t.Logf("Performance: %.2f repos/sec, %.2f dirs/sec",
		float64(idx.Count())/scanDuration.Seconds(),
		float64(dirCount)/scanDuration.Seconds())

	// Verify correctness
	assert.Equal(t, repoCount, idx.Count())

	// Performance assertions (adjust based on acceptable performance)
	// Target: < 2 seconds for hundreds of repos
	if scanDuration > 5*time.Second {
		t.Logf("WARNING: Scan took %v, which is slower than expected", scanDuration)
	}
}

// TestScanPerformanceWideShallow tests scanning with many directories at shallow depth
func TestScanPerformanceWideShallow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	tmpDir := t.TempDir()

	// Create 1000 directories at depth 1, with repos in every 5th directory
	t.Logf("Creating wide shallow directory structure...")
	start := time.Now()

	repoCount := 0
	for i := 0; i < 1000; i++ {
		dirPath := filepath.Join(tmpDir, fmt.Sprintf("dir-%04d", i))
		os.MkdirAll(dirPath, 0755)

		// Every 5th is a git repo
		if i%5 == 0 {
			initGitRepo(t, dirPath)
			os.WriteFile(filepath.Join(dirPath, "README.md"), []byte("test"), 0644)
			commitFiles(t, dirPath, "init")
			repoCount++
		}
	}

	setupDuration := time.Since(start)
	t.Logf("Created 1000 directories with %d repos in %v", repoCount, setupDuration)

	// Create config
	cfg := &config.Config{
		GlobalExcludes: []string{"node_modules", "vendor"},
		Roots: []config.Root{
			{
				Name:     "test",
				Path:     tmpDir,
				MaxDepth: 2,
			},
		},
	}

	// Benchmark scan
	t.Logf("Starting scan...")
	scanStart := time.Now()
	idx := index.New()
	scan := scanner.New(cfg, idx)
	require.NoError(t, scan.Scan())
	scanDuration := time.Since(scanStart)

	t.Logf("Scan completed in %v", scanDuration)
	t.Logf("Found %d repositories", idx.Count())
	t.Logf("Performance: %.2f repos/sec, %.2f dirs/sec",
		float64(idx.Count())/scanDuration.Seconds(),
		1000.0/scanDuration.Seconds())

	// Verify correctness
	assert.Equal(t, repoCount, idx.Count())

	// Target: < 2 seconds for 1000 directories
	if scanDuration > 3*time.Second {
		t.Logf("WARNING: Scan took %v, which is slower than expected", scanDuration)
	}
}

// TestScanPerformanceWithExcludes tests scanning performance with many excludes
func TestScanPerformanceWithExcludes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	tmpDir := t.TempDir()

	// Create directory structure with many node_modules, vendor, etc.
	repoCount := 0
	excludedDirCount := 0

	for i := 0; i < 100; i++ {
		projectDir := filepath.Join(tmpDir, fmt.Sprintf("project-%d", i))
		os.MkdirAll(projectDir, 0755)

		// Create main repo
		initGitRepo(t, projectDir)
		os.WriteFile(filepath.Join(projectDir, "package.json"), []byte("{}"), 0644)
		commitFiles(t, projectDir, "init")
		repoCount++

		// Create excluded directories with nested structure
		excludedDirs := []string{"node_modules", "vendor", "build", "dist"}
		for _, excluded := range excludedDirs {
			excludedPath := filepath.Join(projectDir, excluded)
			os.MkdirAll(excludedPath, 0755)
			excludedDirCount++

			// Create nested repos inside excluded dirs (should be skipped)
			for j := 0; j < 5; j++ {
				nestedRepo := filepath.Join(excludedPath, fmt.Sprintf("nested-%d", j))
				os.MkdirAll(nestedRepo, 0755)
				initGitRepo(t, nestedRepo)
				os.WriteFile(filepath.Join(nestedRepo, "README.md"), []byte("test"), 0644)
				commitFiles(t, nestedRepo, "init")
				excludedDirCount++
			}
		}
	}

	t.Logf("Created 100 projects with %d excluded directories", excludedDirCount)

	// Create config with global excludes
	cfg := &config.Config{
		GlobalExcludes: []string{"node_modules", "vendor", "build", "dist"},
		Roots: []config.Root{
			{
				Name:     "test",
				Path:     tmpDir,
				MaxDepth: 10,
			},
		},
	}

	// Benchmark scan
	t.Logf("Starting scan with excludes...")
	scanStart := time.Now()
	idx := index.New()
	scan := scanner.New(cfg, idx)
	require.NoError(t, scan.Scan())
	scanDuration := time.Since(scanStart)

	t.Logf("Scan completed in %v", scanDuration)
	t.Logf("Found %d repositories (excluded %d)", idx.Count(), excludedDirCount)

	// Should only find the 100 main repos, not the nested ones in excluded dirs
	assert.Equal(t, repoCount, idx.Count())

	// Verify no repos from excluded directories
	repos := idx.List()
	for _, repo := range repos {
		assert.NotContains(t, repo.RelPath, "node_modules")
		assert.NotContains(t, repo.RelPath, "vendor")
		assert.NotContains(t, repo.RelPath, "build")
		assert.NotContains(t, repo.RelPath, "dist")
	}

	t.Logf("Performance: %.2f repos/sec", float64(idx.Count())/scanDuration.Seconds())

	// This should be fast since we're skipping huge subtrees
	if scanDuration > 3*time.Second {
		t.Logf("WARNING: Scan took %v, excludes may not be working efficiently", scanDuration)
	}
}

// BenchmarkScanSmall benchmarks scanning a small workspace
func BenchmarkScanSmall(b *testing.B) {
	tmpDir := b.TempDir()

	// Create 10 repos
	for i := 0; i < 10; i++ {
		repoDir := filepath.Join(tmpDir, fmt.Sprintf("repo-%d", i))
		os.MkdirAll(repoDir, 0755)
		initGitRepo(b, repoDir)
		os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main"), 0644)
		commitFiles(b, repoDir, "init")
	}

	cfg := &config.Config{
		Roots: []config.Root{
			{
				Name:     "test",
				Path:     tmpDir,
				MaxDepth: 3,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := index.New()
		scan := scanner.New(cfg, idx)
		if err := scan.Scan(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkScanMedium benchmarks scanning a medium workspace
func BenchmarkScanMedium(b *testing.B) {
	tmpDir := b.TempDir()

	// Create 50 repos
	for i := 0; i < 50; i++ {
		repoDir := filepath.Join(tmpDir, fmt.Sprintf("repo-%d", i))
		os.MkdirAll(repoDir, 0755)
		initGitRepo(b, repoDir)
		os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main"), 0644)
		commitFiles(b, repoDir, "init")
	}

	cfg := &config.Config{
		GlobalExcludes: []string{"node_modules", "vendor"},
		Roots: []config.Root{
			{
				Name:     "test",
				Path:     tmpDir,
				MaxDepth: 3,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := index.New()
		scan := scanner.New(cfg, idx)
		if err := scan.Scan(); err != nil {
			b.Fatal(err)
		}
	}
}

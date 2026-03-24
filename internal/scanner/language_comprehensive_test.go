package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGoProjectWithoutModules tests Go projects that don't have go.mod
func TestGoProjectWithoutModules(t *testing.T) {
	tmpDir := t.TempDir()

	// Create Go files without go.mod (old-style Go project)
	files := []string{"main.go", "utils.go", "handler.go", "server.go"}
	for _, file := range files {
		err := os.WriteFile(filepath.Join(tmpDir, file), []byte("package main"), 0644)
		require.NoError(t, err)
	}

	lang := DetectLanguage(tmpDir)
	assert.Equal(t, "Go", lang, "Go project without go.mod should be detected as Go by extension count")
}

// TestGoProjectWithCFiles tests Go projects that have C files (cgo)
func TestGoProjectWithCFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Go project with more .go files than .c files
	goFiles := []string{"main.go", "utils.go", "handler.go", "server.go"}
	for _, file := range goFiles {
		err := os.WriteFile(filepath.Join(tmpDir, file), []byte("package main"), 0644)
		require.NoError(t, err)
	}

	// Add some C files (cgo)
	cFiles := []string{"wrapper.c", "wrapper.h"}
	for _, file := range cFiles {
		err := os.WriteFile(filepath.Join(tmpDir, file), []byte("// C code"), 0644)
		require.NoError(t, err)
	}

	lang := DetectLanguage(tmpDir)
	assert.Equal(t, "Go", lang, "Go project with C files should still be detected as Go when Go files dominate")
}

// TestGoProjectWithEqualCAndGoFiles tests tie-breaking
func TestGoProjectWithEqualCAndGoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Equal number of .go and .c files
	goFiles := []string{"main.go", "utils.go"}
	for _, file := range goFiles {
		err := os.WriteFile(filepath.Join(tmpDir, file), []byte("package main"), 0644)
		require.NoError(t, err)
	}

	cFiles := []string{"wrapper.c", "helper.c"}
	for _, file := range cFiles {
		err := os.WriteFile(filepath.Join(tmpDir, file), []byte("// C code"), 0644)
		require.NoError(t, err)
	}

	lang := DetectLanguage(tmpDir)
	// With equal counts, result depends on map iteration order (non-deterministic)
	// But it should be either Go or C, not unknown
	assert.Contains(t, []string{"Go", "C"}, lang, "Should detect as either Go or C, not unknown")
}

// TestGoProjectWithMoreCFilesThanGo tests problematic case
func TestGoProjectWithMoreCFilesThanGo(t *testing.T) {
	tmpDir := t.TempDir()

	// More C files than Go files (but has go.mod)
	goFiles := []string{"main.go"}
	for _, file := range goFiles {
		err := os.WriteFile(filepath.Join(tmpDir, file), []byte("package main"), 0644)
		require.NoError(t, err)
	}

	// Add go.mod to mark it as Go project
	err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)
	require.NoError(t, err)

	// Many C files
	cFiles := []string{"lib1.c", "lib2.c", "lib3.c", "lib1.h", "lib2.h", "lib3.h"}
	for _, file := range cFiles {
		err := os.WriteFile(filepath.Join(tmpDir, file), []byte("// C code"), 0644)
		require.NoError(t, err)
	}

	lang := DetectLanguage(tmpDir)
	assert.Equal(t, "Go", lang, "Go project with go.mod should be detected as Go regardless of C file count")
}

// TestAllLanguageMarkers tests that all language markers work correctly
func TestAllLanguageMarkers(t *testing.T) {
	tests := []struct {
		name     string
		marker   string
		expected string
		content  string
	}{
		{"Go with go.mod", "go.mod", "Go", "module test"},
		{"Go with go.sum", "go.sum", "Go", "checksums"},
		{"Rust with Cargo.toml", "Cargo.toml", "Rust", "[package]"},
		{"Rust with Cargo.lock", "Cargo.lock", "Rust", "lock"},
		{"JavaScript with package.json", "package.json", "JavaScript", "{}"},
		{"Python with requirements.txt", "requirements.txt", "Python", "flask"},
		{"Python with pyproject.toml", "pyproject.toml", "Python", "[tool]"},
		{"Python with Pipfile", "Pipfile", "Python", "packages"},
		{"Python with setup.py", "setup.py", "Python", "from setuptools"},
		{"Ruby with Gemfile", "Gemfile", "Ruby", "source"},
		{"PHP with composer.json", "composer.json", "PHP", "{}"},
		{"Swift with Package.swift", "Package.swift", "Swift", "// swift"},
		{"C++ with CMakeLists.txt", "CMakeLists.txt", "C++", "cmake"},
		{"Haskell with stack.yaml", "stack.yaml", "Haskell", "resolver"},
		{"Elixir with mix.exs", "mix.exs", "Elixir", "defmodule"},
		{"D with dub.json", "dub.json", "D", "{}"},
		{"Java with pom.xml", "pom.xml", "Java", "<project>"},
		{"Java with build.gradle", "build.gradle", "Java", "plugins"},
		{"Kotlin with build.gradle.kts", "build.gradle.kts", "Kotlin", "plugins"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			err := os.WriteFile(filepath.Join(tmpDir, tt.marker), []byte(tt.content), 0644)
			require.NoError(t, err)

			lang := DetectLanguage(tmpDir)
			assert.Equal(t, tt.expected, lang, "Marker file should correctly detect language")
		})
	}
}

// TestWildcardMarkers tests that wildcard patterns work
func TestWildcardMarkers(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		expected string
	}{
		{"C# csproj", "MyProject.csproj", "C#"},
		{"C# sln", "MySolution.sln", "C#"},
		{"Ruby gemspec", "mygem.gemspec", "Ruby"},
		{"Haskell cabal", "myproject.cabal", "Haskell"},
		{"Nim nimble", "myproject.nimble", "Nim"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			err := os.WriteFile(filepath.Join(tmpDir, tt.file), []byte("content"), 0644)
			require.NoError(t, err)

			lang := DetectLanguage(tmpDir)
			assert.Equal(t, tt.expected, lang, "Wildcard pattern should match")
		})
	}
}

// TestLanguagePrecedence tests that markers take precedence over extension counts
func TestLanguagePrecedence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create many C files
	for i := 0; i < 10; i++ {
		filename := filepath.Join(tmpDir, fmt.Sprintf("file%d.c", i))
		err := os.WriteFile(filename, []byte("// C"), 0644)
		require.NoError(t, err)
	}

	// But add go.mod
	err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)
	require.NoError(t, err)

	lang := DetectLanguage(tmpDir)
	assert.Equal(t, "Go", lang, "Language marker should take precedence over extension count")
}

// TestMixedLanguages tests repositories with multiple languages
func TestMixedLanguages(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files for multiple languages
	files := map[string]string{
		"main.go":    "Go",
		"util.go":    "Go",
		"server.go":  "Go",
		"test.py":    "Python",
		"helper.js":  "JavaScript",
		"README.md":  "",
		"config.yml": "",
	}

	for file := range files {
		err := os.WriteFile(filepath.Join(tmpDir, file), []byte("content"), 0644)
		require.NoError(t, err)
	}

	lang := DetectLanguage(tmpDir)
	assert.Equal(t, "Go", lang, "Go should win with 3 files vs 1 Python and 1 JavaScript")
}

// TestEmptyDirectory tests edge case of empty directory
func TestEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	lang := DetectLanguage(tmpDir)
	assert.Equal(t, "unknown", lang, "Empty directory should return unknown")
}

// TestOnlyNonCodeFiles tests directory with only non-code files
func TestOnlyNonCodeFiles(t *testing.T) {
	tmpDir := t.TempDir()

	files := []string{"README.md", "LICENSE", "config.yml", "data.json"}
	for _, file := range files {
		err := os.WriteFile(filepath.Join(tmpDir, file), []byte("content"), 0644)
		require.NoError(t, err)
	}

	lang := DetectLanguage(tmpDir)
	assert.Equal(t, "unknown", lang, "Directory with only non-code files should return unknown")
}

// TestSubdirectoryScanning tests that subdirectories are scanned
func TestSubdirectoryScanning(t *testing.T) {
	tmpDir := t.TempDir()

	// Create subdirectory with Go files
	subDir := filepath.Join(tmpDir, "pkg")
	err := os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	// Put most Go files in subdirectory
	for i := 0; i < 5; i++ {
		filename := filepath.Join(subDir, fmt.Sprintf("file%d.go", i))
		err := os.WriteFile(filename, []byte("package pkg"), 0644)
		require.NoError(t, err)
	}

	// One Python file in root
	err = os.WriteFile(filepath.Join(tmpDir, "test.py"), []byte("# python"), 0644)
	require.NoError(t, err)

	lang := DetectLanguage(tmpDir)
	assert.Equal(t, "Go", lang, "Should count files in subdirectories")
}

// TestVendorDirectorySkipped tests that vendor directory is skipped
func TestVendorDirectorySkipped(t *testing.T) {
	tmpDir := t.TempDir()

	// Create vendor directory with many C files
	vendorDir := filepath.Join(tmpDir, "vendor")
	err := os.MkdirAll(vendorDir, 0755)
	require.NoError(t, err)

	for i := 0; i < 20; i++ {
		filename := filepath.Join(vendorDir, fmt.Sprintf("file%d.c", i))
		err := os.WriteFile(filename, []byte("// C"), 0644)
		require.NoError(t, err)
	}

	// One Go file in root
	err = os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)
	require.NoError(t, err)

	lang := DetectLanguage(tmpDir)
	assert.Equal(t, "Go", lang, "Should skip vendor directory and detect as Go")
}

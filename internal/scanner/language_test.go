package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected string
	}{
		{
			name:     "Go project with go.mod",
			files:    []string{"go.mod", "main.go", "util.go"},
			expected: "Go",
		},
		{
			name:     "Rust project with Cargo.toml",
			files:    []string{"Cargo.toml", "main.rs", "lib.rs"},
			expected: "Rust",
		},
		{
			name:     "JavaScript project with package.json",
			files:    []string{"package.json", "index.js", "app.js"},
			expected: "JavaScript",
		},
		{
			name:     "Python project with requirements.txt",
			files:    []string{"requirements.txt", "main.py", "utils.py"},
			expected: "Python",
		},
		{
			name:     "Python project with pyproject.toml",
			files:    []string{"pyproject.toml", "app.py"},
			expected: "Python",
		},
		{
			name:     "TypeScript by extension count",
			files:    []string{"index.ts", "app.ts", "utils.ts", "types.ts"},
			expected: "TypeScript",
		},
		{
			name:     "Mixed files - Go dominates",
			files:    []string{"main.go", "util.go", "server.go", "README.md", "test.py"},
			expected: "Go",
		},
		{
			name:     "Unknown language",
			files:    []string{"README.md", "LICENSE", ".gitignore"},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()

			// Create files
			for _, file := range tt.files {
				filePath := filepath.Join(tmpDir, file)
				err := os.WriteFile(filePath, []byte("test content"), 0644)
				assert.NoError(t, err)
			}

			// Detect language
			lang := DetectLanguage(tmpDir)
			assert.Equal(t, tt.expected, lang)
		})
	}
}

func TestDetectLanguageWithDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "tests"), 0755)

	// Create Go files
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "src", "lib.go"), []byte("package lib"), 0644)

	lang := DetectLanguage(tmpDir)
	assert.Equal(t, "Go", lang)
}

func TestDetectLanguageMarkerPriority(t *testing.T) {
	tmpDir := t.TempDir()

	// Create both Go marker and lots of Python files
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)
	for i := 0; i < 10; i++ {
		filename := filepath.Join(tmpDir, "test"+string(rune(i))+".py")
		os.WriteFile(filename, []byte("# python"), 0644)
	}

	// Marker should take priority
	lang := DetectLanguage(tmpDir)
	assert.Equal(t, "Go", lang)
}

func TestDetectLanguageSkipsDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directories that should be skipped
	os.MkdirAll(filepath.Join(tmpDir, "node_modules", "package"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "vendor", "lib"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, ".git", "objects"), 0755)

	// Create Go files in main dir
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)

	// Create many JS files in node_modules (should be ignored)
	for i := 0; i < 20; i++ {
		filename := filepath.Join(tmpDir, "node_modules", "package", "file"+string(rune(i))+".js")
		os.WriteFile(filename, []byte("// js"), 0644)
	}

	// Should detect Go, not JavaScript
	lang := DetectLanguage(tmpDir)
	assert.Equal(t, "Go", lang)
}

func TestLanguageMarkers(t *testing.T) {
	// Verify marker map has expected entries
	assert.Equal(t, "Go", languageMarkers["go.mod"])
	assert.Equal(t, "Rust", languageMarkers["Cargo.toml"])
	assert.Equal(t, "JavaScript", languageMarkers["package.json"])
	assert.Equal(t, "Python", languageMarkers["requirements.txt"])
	assert.Equal(t, "Java", languageMarkers["pom.xml"])
}

func TestExtensionMap(t *testing.T) {
	// Verify extension map has expected entries
	assert.Equal(t, "Go", extensionMap[".go"])
	assert.Equal(t, "Rust", extensionMap[".rs"])
	assert.Equal(t, "JavaScript", extensionMap[".js"])
	assert.Equal(t, "TypeScript", extensionMap[".ts"])
	assert.Equal(t, "Python", extensionMap[".py"])
	assert.Equal(t, "C++", extensionMap[".cpp"])
}

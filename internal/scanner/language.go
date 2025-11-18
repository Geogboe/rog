package scanner

import (
	"os"
	"path/filepath"
	"strings"
)

// languageMarkers maps specific files to languages (highest priority)
var languageMarkers = map[string]string{
	"go.mod":          "Go",
	"go.sum":          "Go",
	"Cargo.toml":      "Rust",
	"Cargo.lock":      "Rust",
	"package.json":    "JavaScript",
	"pom.xml":         "Java",
	"build.gradle":    "Java",
	"build.gradle.kts": "Kotlin",
	"requirements.txt": "Python",
	"pyproject.toml":  "Python",
	"Pipfile":         "Python",
	"setup.py":        "Python",
	"Gemfile":         "Ruby",
	"*.gemspec":       "Ruby",
	"composer.json":   "PHP",
	"Package.swift":   "Swift",
	"*.csproj":        "C#",
	"*.sln":           "C#",
	"CMakeLists.txt":  "C++",
	"*.cabal":         "Haskell",
	"stack.yaml":      "Haskell",
	"*.nimble":        "Nim",
	"dub.json":        "D",
	"mix.exs":         "Elixir",
}

// extensionMap maps file extensions to languages
var extensionMap = map[string]string{
	".go":    "Go",
	".rs":    "Rust",
	".js":    "JavaScript",
	".ts":    "TypeScript",
	".jsx":   "JavaScript",
	".tsx":   "TypeScript",
	".py":    "Python",
	".java":  "Java",
	".kt":    "Kotlin",
	".rb":    "Ruby",
	".php":   "PHP",
	".swift": "Swift",
	".c":     "C",
	".h":     "C",
	".cpp":   "C++",
	".cc":    "C++",
	".cxx":   "C++",
	".hpp":   "C++",
	".cs":    "C#",
	".hs":    "Haskell",
	".ex":    "Elixir",
	".exs":   "Elixir",
	".erl":   "Erlang",
	".scala": "Scala",
	".clj":   "Clojure",
	".nim":   "Nim",
	".d":     "D",
	".lua":   "Lua",
	".r":     "R",
	".jl":    "Julia",
	".dart":  "Dart",
	".v":     "V",
	".zig":   "Zig",
}

// DetectLanguage detects the primary language of a repository
func DetectLanguage(repoPath string) string {
	// First, check for language marker files (highest priority)
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return "unknown"
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if lang, ok := languageMarkers[entry.Name()]; ok {
			return lang
		}
		// Check wildcard patterns
		for pattern, lang := range languageMarkers {
			if strings.Contains(pattern, "*") {
				suffix := strings.TrimPrefix(pattern, "*")
				if strings.HasSuffix(entry.Name(), suffix) {
					return lang
				}
			}
		}
	}

	// Second, count files by extension (scan up to 2 levels deep, max 100 files)
	langCounts := make(map[string]int)
	fileCount := 0
	maxFiles := 100
	maxDepth := 2

	var walkFn func(path string, depth int) error
	walkFn = func(path string, depth int) error {
		if fileCount >= maxFiles || depth > maxDepth {
			return filepath.SkipDir
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}

		for _, entry := range entries {
			if fileCount >= maxFiles {
				break
			}

			name := entry.Name()

			// Skip hidden files and common directories to exclude
			if strings.HasPrefix(name, ".") {
				continue
			}
			if entry.IsDir() {
				// Skip common build/dependency directories
				if name == "node_modules" || name == "vendor" || name == "target" ||
					name == "build" || name == "dist" || name == "__pycache__" {
					continue
				}
				// Recurse into subdirectory
				if depth < maxDepth {
					walkFn(filepath.Join(path, name), depth+1)
				}
				continue
			}

			// Check file extension
			ext := filepath.Ext(name)
			if lang, ok := extensionMap[ext]; ok {
				langCounts[lang]++
				fileCount++
			}
		}

		return nil
	}

	walkFn(repoPath, 0)

	// Find most common language
	maxCount := 0
	primaryLang := "unknown"
	for lang, count := range langCounts {
		if count > maxCount {
			maxCount = count
			primaryLang = lang
		}
	}

	return primaryLang
}

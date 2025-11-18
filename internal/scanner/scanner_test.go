package scanner

import (
	"os"
	"path/filepath"
	"testing"

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
			name: "empty file",
			content: ``,
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

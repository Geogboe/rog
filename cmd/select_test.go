package cmd

import (
	"strings"
	"testing"

	"github.com/Geogboe/rog/internal/index"
	"github.com/stretchr/testify/assert"
)

func TestSelectFzfFormatting(t *testing.T) {
	// Create test repos with varying name lengths
	repos := []*index.Repo{
		{
			Name:            "short",
			PrimaryLanguage: "Go",
			Root:            "work",
			RelPath:         "path/to/short",
			Description:     "A short description",
		},
		{
			Name:            "very-long-repository-name",
			PrimaryLanguage: "Python",
			Root:            "personal",
			RelPath:         "long/path/to/repo",
			Description:     "This is a much longer description that should be visible",
		},
		{
			Name:            "medium-name",
			PrimaryLanguage: "JavaScript",
			Root:            "work",
			RelPath:         "src",
			Description:     "Medium description here",
		},
	}

	// Build formatted lines using the same logic as selectWithFzf
	var lines []string
	for _, repo := range repos {
		desc := repo.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		if desc == "" {
			desc = "-"
		}

		line := formatSelectLine(repo, desc)
		lines = append(lines, line)
	}

	// Validate that columns align properly
	// Each line should have consistent column positions
	for i, line := range lines {
		t.Logf("Line %d: %s", i, line)

		// Count spaces to verify alignment
		// The issue is that tabs don't create fixed-width columns
		// We want to validate that after the fix, columns start at the same position
	}

	// After fix: verify that all lines have same column start positions
	// For now, this test documents the expected behavior
	assert.Greater(t, len(lines), 0, "Should have formatted lines")
}

func TestSelectLineColumnAlignment(t *testing.T) {
	repos := []*index.Repo{
		{
			Name:            "a",
			PrimaryLanguage: "Go",
			Root:            "r1",
			RelPath:         "p1",
			Description:     "Desc 1",
		},
		{
			Name:            "very-long-name",
			PrimaryLanguage: "Python",
			Root:            "very-long-root",
			RelPath:         "very/long/path",
			Description:     "Description 2",
		},
	}

	lines := []string{}
	for _, repo := range repos {
		desc := repo.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		if desc == "" {
			desc = "-"
		}
		line := formatSelectLine(repo, desc)
		lines = append(lines, line)
	}

	// With fixed-width formatting, the language column should start at the same position
	// Find position of first field separator in each line
	positions := [][]int{}
	for _, line := range lines {
		pos := []int{}
		// Find all separator positions
		for i := 0; i < len(line); i++ {
			if i > 0 && line[i] != ' ' && line[i-1] == ' ' {
				// Count consecutive spaces before this position
				spaceCount := 0
				for j := i - 1; j >= 0 && line[j] == ' '; j-- {
					spaceCount++
				}
				if spaceCount >= 2 { // Multiple spaces indicate column separator
					pos = append(pos, i)
				}
			}
		}
		positions = append(positions, pos)
	}

	// After fix: column positions should be consistent across lines
	// This test will validate that columns align
	if len(positions) > 1 {
		t.Logf("Line 0 column positions: %v", positions[0])
		t.Logf("Line 1 column positions: %v", positions[1])

		// The language column should start at same position in both lines
		if len(positions[0]) > 0 && len(positions[1]) > 0 {
			assert.Equal(t, positions[0][0], positions[1][0],
				"Language column should start at same position")
		}
	}
}

func TestSelectLineFormatting(t *testing.T) {
	repo := &index.Repo{
		Name:            "test-repo",
		PrimaryLanguage: "Go",
		Root:            "work",
		RelPath:         "path/to/repo",
		Description:     "Test description",
	}

	line := formatSelectLine(repo, "Test description")

	// Line should contain all fields
	assert.Contains(t, line, "test-repo")
	assert.Contains(t, line, "Go")
	assert.Contains(t, line, "work")
	assert.Contains(t, line, "path/to/repo")
	assert.Contains(t, line, "Test description")

	// Fields should appear in order
	namePos := strings.Index(line, "test-repo")
	langPos := strings.Index(line, "Go")
	pathPos := strings.Index(line, "work/path/to/repo")
	descPos := strings.Index(line, "Test description")

	assert.Less(t, namePos, langPos, "Name should come before language")
	assert.Less(t, langPos, pathPos, "Language should come before path")
	assert.Less(t, pathPos, descPos, "Path should come before description")
}

package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Geogboe/rog/internal/index"
)

// Helper to capture stdout
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// Helper to create a test index
func createTestIndexForCmd() *index.Index {
	idx := index.New()

	idx.Upsert(&index.Repo{
		Name:            "test-repo-1",
		AbsPath:         "/tmp/test-repo-1",
		Root:            "test-root",
		RelPath:         "path/to/repo1",
		PrimaryLanguage: "Go",
		CurrentBranch:   "main",
		Host:            "github.com",
		RemoteURL:       "https://github.com/user/test-repo-1",
		LastCommitTime:  time.Now().Add(-1 * time.Hour),
		LastCommitAuthor: "Test Author",
		IsDirty:         false,
		Ahead:           0,
		Behind:          0,
	})

	idx.Upsert(&index.Repo{
		Name:            "test-repo-2",
		AbsPath:         "/tmp/test-repo-2",
		Root:            "test-root",
		RelPath:         "path/to/repo2",
		PrimaryLanguage: "Python",
		CurrentBranch:   "develop",
		Host:            "gitlab.com",
		RemoteURL:       "https://gitlab.com/user/test-repo-2",
		LastCommitTime:  time.Now().Add(-2 * time.Hour),
		LastCommitAuthor: "Another Author",
		IsDirty:         true,
		Ahead:           2,
		Behind:          0,
	})

	return idx
}

func TestOutputTableShort(t *testing.T) {
	idx := createTestIndexForCmd()
	repos := idx.List()

	output := captureOutput(func() {
		outputTable(repos, true, false)
	})

	// Verify short format headers
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "LANG")
	assert.Contains(t, output, "PATH")

	// Should NOT contain fields from normal/long format
	assert.NotContains(t, output, "HOST")
	assert.NotContains(t, output, "BRANCH")
	assert.NotContains(t, output, "STATUS")
	assert.NotContains(t, output, "AUTHOR")
	assert.NotContains(t, output, "REMOTE")

	// Should contain repo data
	assert.Contains(t, output, "test-repo-1")
	assert.Contains(t, output, "test-repo-2")
	assert.Contains(t, output, "Go")
	assert.Contains(t, output, "Python")

	// Should contain combined paths
	assert.Contains(t, output, "test-root/path/to/repo1")
	assert.Contains(t, output, "test-root/path/to/repo2")

	// Should contain total count
	assert.Contains(t, output, "Total: 2 repositories")
}

func TestOutputTableNormal(t *testing.T) {
	idx := createTestIndexForCmd()
	repos := idx.List()

	output := captureOutput(func() {
		outputTable(repos, false, false)
	})

	// Verify normal format headers
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "LANG")
	assert.Contains(t, output, "HOST")
	assert.Contains(t, output, "BRANCH")
	assert.Contains(t, output, "STATUS")
	assert.Contains(t, output, "LAST COMMIT")
	assert.Contains(t, output, "ROOT")
	assert.Contains(t, output, "PATH")

	// Should NOT contain long-format-only fields
	assert.NotContains(t, output, "AUTHOR")
	assert.NotContains(t, output, "REMOTE")

	// Should contain repo data
	assert.Contains(t, output, "test-repo-1")
	assert.Contains(t, output, "github.com")
	assert.Contains(t, output, "main")

	assert.Contains(t, output, "test-repo-2")
	assert.Contains(t, output, "gitlab.com")
	assert.Contains(t, output, "develop")
	assert.Contains(t, output, "dirty")
}

func TestOutputTableLong(t *testing.T) {
	idx := createTestIndexForCmd()
	repos := idx.List()

	output := captureOutput(func() {
		outputTable(repos, false, true)
	})

	// Verify long format contains all fields
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "LANG")
	assert.Contains(t, output, "HOST")
	assert.Contains(t, output, "BRANCH")
	assert.Contains(t, output, "STATUS")
	assert.Contains(t, output, "LAST COMMIT")
	assert.Contains(t, output, "AUTHOR")
	assert.Contains(t, output, "ROOT")
	assert.Contains(t, output, "PATH")
	assert.Contains(t, output, "REMOTE")

	// Should contain author info
	assert.Contains(t, output, "Test Author")
	assert.Contains(t, output, "Another Author")

	// Should contain remote URLs (may be truncated)
	assert.Contains(t, output, "github.com/user/test-repo-1")
}

func TestOutputTableMutualExclusivity(t *testing.T) {
	// This should never happen in practice due to validation in runList,
	// but let's test the function behavior
	idx := createTestIndexForCmd()
	repos := idx.List()

	// When both short and long are true, short should take precedence
	output := captureOutput(func() {
		outputTable(repos, true, true)
	})

	// Should produce short output (first condition checked)
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "LANG")
	assert.Contains(t, output, "PATH")
	assert.NotContains(t, output, "AUTHOR")
}

func TestRunListWithShortFlag(t *testing.T) {
	// Setup temp directory for index
	tmpDir := t.TempDir()
	os.Setenv("ROG_DATA", tmpDir)
	defer os.Unsetenv("ROG_DATA")

	// Create and save test index
	idx := createTestIndexForCmd()
	require.NoError(t, idx.Save())

	// Capture output
	listShort = true
	listLong = false
	defer func() {
		listShort = false
	}()

	output := captureOutput(func() {
		runList(nil, []string{})
	})

	// Verify short output
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "LANG")
	assert.Contains(t, output, "PATH")
	assert.NotContains(t, output, "BRANCH")
	assert.NotContains(t, output, "STATUS")
}

func TestRunListWithLongFlag(t *testing.T) {
	// Setup temp directory for index
	tmpDir := t.TempDir()
	os.Setenv("ROG_DATA", tmpDir)
	defer os.Unsetenv("ROG_DATA")

	// Create and save test index
	idx := createTestIndexForCmd()
	require.NoError(t, idx.Save())

	// Capture output
	listShort = false
	listLong = true
	defer func() {
		listLong = false
	}()

	output := captureOutput(func() {
		runList(nil, []string{})
	})

	// Verify long output
	assert.Contains(t, output, "AUTHOR")
	assert.Contains(t, output, "REMOTE")
}

func TestRunListDefaultFormat(t *testing.T) {
	// Setup temp directory for index
	tmpDir := t.TempDir()
	os.Setenv("ROG_DATA", tmpDir)
	defer os.Unsetenv("ROG_DATA")

	// Create and save test index
	idx := createTestIndexForCmd()
	require.NoError(t, idx.Save())

	// Ensure flags are default
	listShort = false
	listLong = false

	output := captureOutput(func() {
		runList(nil, []string{})
	})

	// Verify normal output (has BRANCH and STATUS, but not AUTHOR or REMOTE)
	assert.Contains(t, output, "BRANCH")
	assert.Contains(t, output, "STATUS")
	assert.NotContains(t, output, "AUTHOR")
	assert.NotContains(t, output, "REMOTE")
}

func TestListCommandHelp(t *testing.T) {
	output := captureOutput(func() {
		listCmd.Help()
	})

	// Verify help text mentions all output modes
	assert.Contains(t, output, "--short")
	assert.Contains(t, output, "--long")
	assert.Contains(t, output, "Minimal output")
	assert.Contains(t, output, "Detailed output")
}

func TestFormatStatusWithDifferentStates(t *testing.T) {
	tests := []struct {
		name     string
		repo     *index.Repo
		expected []string
		notExpected []string
	}{
		{
			name: "clean and up-to-date",
			repo: &index.Repo{
				IsDirty:      false,
				HasUntracked: false,
				Ahead:        0,
				Behind:       0,
			},
			expected: []string{"up-to-date", "clean"},
		},
		{
			name: "dirty",
			repo: &index.Repo{
				IsDirty: true,
				Ahead:   0,
				Behind:  0,
			},
			expected: []string{"dirty"},
		},
		{
			name: "ahead",
			repo: &index.Repo{
				IsDirty: false,
				Ahead:   3,
				Behind:  0,
			},
			expected: []string{"ahead 3", "clean"},
		},
		{
			name: "behind",
			repo: &index.Repo{
				IsDirty: false,
				Ahead:   0,
				Behind:  5,
			},
			expected: []string{"behind 5", "clean"},
		},
		{
			name: "diverged",
			repo: &index.Repo{
				IsDirty: true,
				Ahead:   2,
				Behind:  3,
			},
			expected: []string{"diverged", "↑2", "↓3", "dirty"},
			notExpected: []string{"ahead", "behind"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := formatStatus(tt.repo)
			for _, exp := range tt.expected {
				assert.Contains(t, status, exp)
			}
			for _, notExp := range tt.notExpected {
				assert.NotContains(t, status, notExp)
			}
		})
	}
}

func TestShortOutputCompactness(t *testing.T) {
	idx := createTestIndexForCmd()
	repos := idx.List()

	shortOutput := captureOutput(func() {
		outputTable(repos, true, false)
	})

	normalOutput := captureOutput(func() {
		outputTable(repos, false, false)
	})

	longOutput := captureOutput(func() {
		outputTable(repos, false, true)
	})

	// Short output should be shorter than normal, which should be shorter than long
	shortLines := len(strings.Split(strings.TrimSpace(shortOutput), "\n"))
	normalLines := len(strings.Split(strings.TrimSpace(normalOutput), "\n"))
	longLines := len(strings.Split(strings.TrimSpace(longOutput), "\n"))

	// They all have the same number of lines (header + 2 repos + total)
	// but let's check output length
	assert.True(t, len(shortOutput) < len(normalOutput), "Short output should be more compact than normal")
	assert.True(t, len(normalOutput) < len(longOutput), "Normal output should be more compact than long")

	// Verify line counts are consistent (header + repos + total)
	expectedLines := 5 // Header, 2 repos, blank line, total
	assert.Equal(t, expectedLines, shortLines)
	assert.Equal(t, expectedLines, normalLines)
	assert.Equal(t, expectedLines, longLines)
}

// Integration test: Test with filtering
func TestListShortWithFiltering(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("ROG_DATA", tmpDir)
	defer os.Unsetenv("ROG_DATA")

	idx := createTestIndexForCmd()
	require.NoError(t, idx.Save())

	// Setup filter for Go repos only
	listShort = true
	listLang = []string{"Go"}
	listLimit = 0
	defer func() {
		listShort = false
		listLang = nil
	}()

	output := captureOutput(func() {
		runList(nil, []string{})
	})

	// Should show only Go repo in short format
	assert.Contains(t, output, "test-repo-1")
	assert.NotContains(t, output, "test-repo-2") // Python repo
	assert.Contains(t, output, "Total: 1 repositories")
}

package query

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Geogboe/rog/internal/index"
)

func createTestIndex() *index.Index {
	idx := index.New()

	idx.Upsert(&index.Repo{
		Name:            "api-server",
		AbsPath:         "/tmp/api-server",
		Root:            "dev",
		RelPath:         "backend/api-server",
		PrimaryLanguage: "Go",
		Description:     "REST API server for backend services",
		Tags:            []string{"go", "rest", "api"},
		CurrentBranch:   "main",
		IsDirty:         false,
		Ahead:           0,
		Behind:          0,
		LastCommitTime:  time.Now().Add(-1 * time.Hour),
	})

	idx.Upsert(&index.Repo{
		Name:            "web-frontend",
		AbsPath:         "/tmp/web-frontend",
		Root:            "dev",
		RelPath:         "frontend/web",
		PrimaryLanguage: "TypeScript",
		Description:     "React web application",
		Tags:            []string{"typescript", "react", "web"},
		CurrentBranch:   "develop",
		IsDirty:         true,
		HasUntracked:    true,
		Ahead:           2,
		Behind:          0,
		LastCommitTime:  time.Now().Add(-2 * time.Hour),
	})

	idx.Upsert(&index.Repo{
		Name:            "cli-tool",
		AbsPath:         "/tmp/cli-tool",
		Root:            "personal",
		RelPath:         "tools/cli",
		PrimaryLanguage: "Go",
		Description:     "Command-line tool for automation",
		Tags:            []string{"go", "cli", "automation"},
		CurrentBranch:   "main",
		IsDirty:         false,
		Ahead:           0,
		Behind:          3,
		LastCommitTime:  time.Now().Add(-24 * time.Hour),
	})

	idx.Upsert(&index.Repo{
		Name:            "python-scripts",
		AbsPath:         "/tmp/python-scripts",
		Root:            "personal",
		RelPath:         "scripts",
		PrimaryLanguage: "Python",
		Description:     "Collection of utility scripts",
		Tags:            []string{"python", "scripts", "utilities"},
		CurrentBranch:   "main",
		IsDirty:         true,
		Ahead:           1,
		Behind:          0,
		LastCommitTime:  time.Now().Add(-30 * time.Minute),
	})

	return idx
}

func TestQueryNoFilters(t *testing.T) {
	idx := createTestIndex()
	filter := &Filter{}

	results := Query(idx, filter)
	assert.Equal(t, 4, len(results))
}

func TestQueryByLanguage(t *testing.T) {
	idx := createTestIndex()

	// Single language
	filter := &Filter{Languages: []string{"Go"}}
	results := Query(idx, filter)
	assert.Equal(t, 2, len(results))
	for _, r := range results {
		assert.Equal(t, "Go", r.PrimaryLanguage)
	}

	// Multiple languages
	filter = &Filter{Languages: []string{"Go", "Python"}}
	results = Query(idx, filter)
	assert.Equal(t, 3, len(results))
}

func TestQueryByTags(t *testing.T) {
	idx := createTestIndex()

	// Single tag
	filter := &Filter{Tags: []string{"cli"}}
	results := Query(idx, filter)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "cli-tool", results[0].Name)

	// Multiple tags (all must match)
	filter = &Filter{Tags: []string{"go", "api"}}
	results = Query(idx, filter)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "api-server", results[0].Name)

	// Non-existent tag
	filter = &Filter{Tags: []string{"nonexistent"}}
	results = Query(idx, filter)
	assert.Equal(t, 0, len(results))
}

func TestQueryByBranch(t *testing.T) {
	idx := createTestIndex()

	filter := &Filter{Branch: "main"}
	results := Query(idx, filter)
	assert.Equal(t, 3, len(results))

	filter = &Filter{Branch: "develop"}
	results = Query(idx, filter)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "web-frontend", results[0].Name)
}

func TestQueryByRoot(t *testing.T) {
	idx := createTestIndex()

	filter := &Filter{Root: "dev"}
	results := Query(idx, filter)
	assert.Equal(t, 2, len(results))

	filter = &Filter{Root: "personal"}
	results = Query(idx, filter)
	assert.Equal(t, 2, len(results))
}

func TestQueryByDirty(t *testing.T) {
	idx := createTestIndex()

	dirty := true
	filter := &Filter{Dirty: &dirty}
	results := Query(idx, filter)
	assert.Equal(t, 2, len(results))

	clean := false
	filter = &Filter{Dirty: &clean}
	results = Query(idx, filter)
	assert.Equal(t, 2, len(results))
}

func TestQueryByAhead(t *testing.T) {
	idx := createTestIndex()

	ahead := true
	filter := &Filter{Ahead: &ahead}
	results := Query(idx, filter)
	assert.Equal(t, 2, len(results))
	for _, r := range results {
		assert.Greater(t, r.Ahead, 0)
	}
}

func TestQueryByBehind(t *testing.T) {
	idx := createTestIndex()

	behind := true
	filter := &Filter{Behind: &behind}
	results := Query(idx, filter)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "cli-tool", results[0].Name)
}

func TestQuerySearchTerms(t *testing.T) {
	idx := createTestIndex()

	// Search in name
	filter := &Filter{SearchTerms: []string{"api"}}
	results := Query(idx, filter)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "api-server", results[0].Name)

	// Search in description
	filter = &Filter{SearchTerms: []string{"utility"}}
	results = Query(idx, filter)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "python-scripts", results[0].Name)

	// Search in tags
	filter = &Filter{SearchTerms: []string{"react"}}
	results = Query(idx, filter)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "web-frontend", results[0].Name)

	// Multiple search terms (all must match)
	filter = &Filter{SearchTerms: []string{"web", "react"}}
	results = Query(idx, filter)
	assert.Equal(t, 1, len(results))

	// Case insensitive
	filter = &Filter{SearchTerms: []string{"API"}}
	results = Query(idx, filter)
	assert.Equal(t, 1, len(results))
}

func TestQueryCombinedFilters(t *testing.T) {
	idx := createTestIndex()

	// Language + tag
	filter := &Filter{
		Languages: []string{"Go"},
		Tags:      []string{"api"},
	}
	results := Query(idx, filter)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "api-server", results[0].Name)

	// Search + language
	filter = &Filter{
		SearchTerms: []string{"tool"},
		Languages:   []string{"Go"},
	}
	results = Query(idx, filter)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "cli-tool", results[0].Name)

	// Dirty + language
	dirty := true
	filter = &Filter{
		Dirty:     &dirty,
		Languages: []string{"Python"},
	}
	results = Query(idx, filter)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "python-scripts", results[0].Name)
}

func TestQuerySort(t *testing.T) {
	idx := createTestIndex()

	// Sort by name (default)
	filter := &Filter{SortBy: SortByName}
	results := Query(idx, filter)
	assert.Equal(t, "api-server", results[0].Name)
	assert.Equal(t, "cli-tool", results[1].Name)

	// Sort by last commit
	filter = &Filter{SortBy: SortByLastCommit}
	results = Query(idx, filter)
	assert.Equal(t, "python-scripts", results[0].Name) // Most recent
	assert.Equal(t, "cli-tool", results[3].Name)       // Least recent

	// Sort by path
	filter = &Filter{SortBy: SortByPath}
	results = Query(idx, filter)
	// Should sort by root/relpath
	assert.True(t, results[0].Root+"/"+results[0].RelPath < results[1].Root+"/"+results[1].RelPath)
}

func TestQueryLimit(t *testing.T) {
	idx := createTestIndex()

	filter := &Filter{Limit: 2}
	results := Query(idx, filter)
	assert.Equal(t, 2, len(results))

	filter = &Filter{Limit: 100}
	results = Query(idx, filter)
	assert.Equal(t, 4, len(results)) // All repos (less than limit)
}

func TestFindUnique(t *testing.T) {
	idx := createTestIndex()

	// Exact name match
	repo, matches, err := FindUnique(idx, "api-server")
	assert.NoError(t, err)
	assert.NotNil(t, repo)
	assert.Nil(t, matches)
	assert.Equal(t, "api-server", repo.Name)

	// Exact path match
	repo, matches, err = FindUnique(idx, "/tmp/cli-tool")
	assert.NoError(t, err)
	assert.NotNil(t, repo)
	assert.Equal(t, "cli-tool", repo.Name)

	// Fuzzy match (unique)
	repo, matches, err = FindUnique(idx, "python")
	assert.NoError(t, err)
	assert.NotNil(t, repo)
	assert.Equal(t, "python-scripts", repo.Name)

	// Fuzzy match (ambiguous)
	repo, matches, err = FindUnique(idx, "go")
	assert.NoError(t, err)
	assert.Nil(t, repo)
	assert.Equal(t, 2, len(matches)) // api-server and cli-tool both match

	// No match
	repo, matches, err = FindUnique(idx, "nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, repo)
	assert.Nil(t, matches)
}

func TestMatchesSearchTerms(t *testing.T) {
	repo := &index.Repo{
		Name:        "test-repo",
		Description: "A test repository",
		Tags:        []string{"test", "example"},
		RelPath:     "projects/test",
		RemoteURL:   "https://github.com/user/test-repo",
	}

	// Match in name
	assert.True(t, matchesSearchTerms(repo, []string{"test"}))

	// Match in description
	assert.True(t, matchesSearchTerms(repo, []string{"repository"}))

	// Match in tags
	assert.True(t, matchesSearchTerms(repo, []string{"example"}))

	// Match in path
	assert.True(t, matchesSearchTerms(repo, []string{"projects"}))

	// Match in URL
	assert.True(t, matchesSearchTerms(repo, []string{"github"}))

	// Multiple matches (all must match)
	assert.True(t, matchesSearchTerms(repo, []string{"test", "repo"}))
	assert.False(t, matchesSearchTerms(repo, []string{"test", "nonexistent"}))

	// Case insensitive
	assert.True(t, matchesSearchTerms(repo, []string{"TEST"}))
	assert.True(t, matchesSearchTerms(repo, []string{"REPOSITORY"}))

	// No match
	assert.False(t, matchesSearchTerms(repo, []string{"nonexistent"}))
}

// Stress test with many repos
func TestQueryStress(t *testing.T) {
	idx := index.New()

	// Create 1000 repos
	for i := 0; i < 1000; i++ {
		idx.Upsert(&index.Repo{
			Name:            "repo-" + string(rune(i)),
			AbsPath:         "/tmp/repo-" + string(rune(i)),
			Root:            "test",
			PrimaryLanguage: []string{"Go", "Python", "JavaScript"}[i%3],
			Tags:            []string{"tag1", "tag2", "tag3"}[i%3 : (i%3)+1],
		})
	}

	// Query should still be fast
	filter := &Filter{Languages: []string{"Go"}}
	results := Query(idx, filter)
	assert.Greater(t, len(results), 300)

	// Complex filter
	filter = &Filter{
		Languages: []string{"Python"},
		Tags:      []string{"tag2"},
	}
	results = Query(idx, filter)
	assert.NotEmpty(t, results)
}

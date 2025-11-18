package index

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	idx := New()
	assert.NotNil(t, idx)
	assert.NotNil(t, idx.Repos)
	assert.Equal(t, 0, idx.Count())
}

func TestUpsert(t *testing.T) {
	idx := New()

	repo := &Repo{
		Name:    "test-repo",
		AbsPath: "/tmp/test-repo",
		Root:    "test",
	}

	// Insert
	idx.Upsert(repo)
	assert.Equal(t, 1, idx.Count())

	// Should set ID and timestamps
	assert.NotEmpty(t, repo.ID)
	assert.False(t, repo.FirstSeenAt.IsZero())
	assert.False(t, repo.LastScanAt.IsZero())

	// Update
	firstSeen := repo.FirstSeenAt
	time.Sleep(10 * time.Millisecond)
	repo.Description = "Updated description"
	idx.Upsert(repo)

	// Should preserve FirstSeenAt
	assert.Equal(t, firstSeen, repo.FirstSeenAt)
	assert.Equal(t, 1, idx.Count())
}

func TestGet(t *testing.T) {
	idx := New()

	repo := &Repo{
		Name:    "test-repo",
		AbsPath: "/tmp/test-repo",
	}

	idx.Upsert(repo)

	// Get existing
	found, ok := idx.Get("/tmp/test-repo")
	assert.True(t, ok)
	assert.Equal(t, "test-repo", found.Name)

	// Get non-existent
	_, ok = idx.Get("/tmp/nonexistent")
	assert.False(t, ok)
}

func TestGetByName(t *testing.T) {
	idx := New()

	idx.Upsert(&Repo{Name: "test", AbsPath: "/tmp/test1"})
	idx.Upsert(&Repo{Name: "test", AbsPath: "/tmp/test2"})
	idx.Upsert(&Repo{Name: "other", AbsPath: "/tmp/other"})

	repos := idx.GetByName("test")
	assert.Equal(t, 2, len(repos))

	repos = idx.GetByName("other")
	assert.Equal(t, 1, len(repos))

	repos = idx.GetByName("nonexistent")
	assert.Equal(t, 0, len(repos))
}

func TestList(t *testing.T) {
	idx := New()

	idx.Upsert(&Repo{Name: "zebra", AbsPath: "/tmp/zebra"})
	idx.Upsert(&Repo{Name: "alpha", AbsPath: "/tmp/alpha"})
	idx.Upsert(&Repo{Name: "beta", AbsPath: "/tmp/beta"})

	repos := idx.List()
	assert.Equal(t, 3, len(repos))

	// Should be sorted by name
	assert.Equal(t, "alpha", repos[0].Name)
	assert.Equal(t, "beta", repos[1].Name)
	assert.Equal(t, "zebra", repos[2].Name)
}

func TestDelete(t *testing.T) {
	idx := New()

	idx.Upsert(&Repo{Name: "test", AbsPath: "/tmp/test"})
	assert.Equal(t, 1, idx.Count())

	idx.Delete("/tmp/test")
	assert.Equal(t, 0, idx.Count())
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.json")

	// Override index path
	os.Setenv("ROG_DATA", tmpDir)
	defer os.Unsetenv("ROG_DATA")

	// Create index
	idx := New()
	idx.Upsert(&Repo{
		Name:            "test-repo",
		AbsPath:         "/tmp/test-repo",
		Root:            "test",
		PrimaryLanguage: "Go",
		Description:     "Test repository",
		Tags:            []string{"test", "example"},
	})

	// Save
	err := idx.Save()
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(indexPath)
	require.NoError(t, err)

	// Load
	loaded, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 1, loaded.Count())

	repo, ok := loaded.Get("/tmp/test-repo")
	assert.True(t, ok)
	assert.Equal(t, "test-repo", repo.Name)
	assert.Equal(t, "Go", repo.PrimaryLanguage)
	assert.Equal(t, "Test repository", repo.Description)
	assert.Equal(t, []string{"test", "example"}, repo.Tags)
}

func TestRemoveStale(t *testing.T) {
	idx := New()

	// Create temp repos for testing
	tmpDir := t.TempDir()
	existingRepo := filepath.Join(tmpDir, "existing")
	os.MkdirAll(filepath.Join(existingRepo, ".git"), 0755)

	idx.Upsert(&Repo{Name: "existing", AbsPath: existingRepo})
	idx.Upsert(&Repo{Name: "nonexistent", AbsPath: "/nonexistent/repo"})

	assert.Equal(t, 2, idx.Count())

	removed := idx.RemoveStale()
	assert.Equal(t, 1, removed)
	assert.Equal(t, 1, idx.Count())

	// Existing repo should still be there
	_, ok := idx.Get(existingRepo)
	assert.True(t, ok)
}

func TestConcurrentAccess(t *testing.T) {
	idx := New()

	// Concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			repo := &Repo{
				Name:    "repo" + string(rune(n)),
				AbsPath: "/tmp/repo" + string(rune(n)),
			}
			idx.Upsert(repo)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	assert.Equal(t, 10, idx.Count())

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			repos := idx.List()
			assert.NotEmpty(t, repos)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID("/tmp/test")
	id2 := generateID("/tmp/test")
	id3 := generateID("/tmp/other")

	// Same path should generate same ID
	assert.Equal(t, id1, id2)

	// Different path should generate different ID
	assert.NotEqual(t, id1, id3)

	// ID should be 16 chars (hex of first 8 bytes of hash)
	assert.Equal(t, 16, len(id1))
}

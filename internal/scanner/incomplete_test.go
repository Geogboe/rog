package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
)

func TestIncompleteScanPreservesFailedRoot(t *testing.T) {
	good := t.TempDir()
	if err := os.Mkdir(filepath.Join(good, "project"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(good, "project", ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	cfg := &config.Config{Roots: []config.Root{{Name: "good", Path: good, MaxDepth: 1}, {Name: "missing", Path: missing, MaxDepth: 1}}}
	scan := New(cfg, index.New()).WithDryRun(true)
	var incomplete *IncompleteError
	if err := scan.ScanContext(context.Background()); !errors.As(err, &incomplete) {
		t.Fatalf("want incomplete scan, got %v", err)
	}
	complete := scan.CompletedRoots()
	if _, ok := complete["good"]; !ok {
		t.Fatal("good root not marked complete")
	}
	if _, ok := complete["missing"]; ok {
		t.Fatal("missing root marked complete")
	}
	if len(scan.FoundPaths()) != 1 {
		t.Fatalf("found %d repositories", len(scan.FoundPaths()))
	}
}

func TestFullScanPrunesIncompleteGitMarker(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "abandoned")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	idx := index.New()
	idx.Upsert(&index.Repo{AbsPath: repo, Root: "root", Name: "abandoned"})
	scan := New(&config.Config{Roots: []config.Root{{Name: "root", Path: root, MaxDepth: 1}}}, idx)
	if err := scan.ScanContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := idx.RemoveStaleInRoots(scan.FoundPaths(), scan.CompletedRoots()); got != 1 {
		t.Fatalf("removed %d, want 1", got)
	}
	if _, ok := idx.Get(repo); ok {
		t.Fatal("incomplete Git marker stayed indexed")
	}
	if got := scan.SnapshotMetrics().ReposFound; got != 1 {
		t.Fatalf("discovered %d markers, want 1", got)
	}
}

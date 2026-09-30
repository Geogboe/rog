package discovery

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
)

func TestWalkNestedHiddenWorktreeAndExcluded(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"outer", "outer/nested", ".hidden", "work tree", "excluded", "outer/.git/objects"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"outer/.git", "outer/nested/.git", ".hidden/.git", "work tree/.git", "excluded/.git"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("gitdir: example"), 0644); err != nil && name != "outer/.git" {
			t.Fatal(err)
		}
	}
	var found []string
	var mu sync.Mutex
	err := Walk(context.Background(), root, 2, func(name, _ string) bool { return name == "excluded" }, func(p string) error { mu.Lock(); found = append(found, p); mu.Unlock(); return nil })
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(found)
	want := []string{filepath.Join(root, ".hidden"), filepath.Join(root, "outer"), filepath.Join(root, "outer", "nested"), filepath.Join(root, "work tree")}
	sort.Strings(want)
	if len(found) != len(want) {
		t.Fatalf("found %q; want %q", found, want)
	}
	for i := range want {
		if found[i] != want[i] {
			t.Fatalf("found %q; want %q", found, want)
		}
	}
}

func TestWalkCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Walk(ctx, t.TempDir(), 2, nil, func(string) error { t.Fatal("unexpected result"); return nil }); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
}

func TestWalkWithProgressReportsVisitedDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "group", "repo"), 0755); err != nil {
		t.Fatal(err)
	}
	var visited atomic.Int64
	err := WalkWithProgress(context.Background(), root, 3, nil, func(path string, depth int) {
		if path == "" || depth < 0 {
			t.Errorf("invalid traversal progress: %q at depth %d", path, depth)
		}
		visited.Add(1)
	}, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if visited.Load() < 3 {
		t.Fatalf("reported %d visited directories, want at least root/group/repo", visited.Load())
	}
}

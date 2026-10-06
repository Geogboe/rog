package cmd

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
	"github.com/spf13/cobra"
)

func TestSetupFullScanUsesExecutingCommandContextAndRefreshesIndex(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v: %s", err, out)
	}
	t.Setenv("ROG_CONFIG", filepath.Join(t.TempDir(), "config.yml"))
	t.Setenv("ROG_DATA", t.TempDir())
	t.Setenv("ROG_PROGRESS", "off")
	if err := config.Save(&config.Config{Roots: []config.Root{{Name: "projects", Path: root, MaxDepth: 1}}}); err != nil {
		t.Fatal(err)
	}
	idx := index.New()
	idx.Repos[repo] = &index.Repo{Name: "repo", Root: "projects", AbsPath: repo, RelPath: "repo", CurrentBranch: "stale-cached-branch"}
	if err := idx.Save(); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{Use: "setup"}
	cmd.SetContext(context.Background())
	runSetupScan(cmd)
	refreshed, err := index.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(refreshed.Repos) != 1 {
		t.Fatalf("scan did not use saved roots: %d repos", len(refreshed.Repos))
	}
	r := refreshed.Repos[repo]
	if r == nil || r.CurrentBranch == "" || r.CurrentBranch == "stale-cached-branch" {
		t.Fatal("full setup scan did not refresh Git metadata")
	}
}

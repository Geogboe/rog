package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGetRepoInfoUsesNativeGit(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init", repo},
		{"-C", repo, "checkout", "-b", "main"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-C", repo, "add", "README.md"},
		{"-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "test"},
		{"-C", repo, "remote", "add", "origin", "https://example.com/team/repo.git"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	info, err := GetRepoInfo(repo)
	if err != nil {
		t.Fatal(err)
	}
	if info.Branch != "main" || info.Commit == nil || info.Host != "example.com" {
		t.Fatalf("unexpected repo info: branch=%q commit_present=%t host=%q", info.Branch, info.Commit != nil, info.Host)
	}
}

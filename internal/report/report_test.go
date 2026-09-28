package report

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestAIEvidenceBudgetAndReferenceValidation(t *testing.T) {
	d := Document{Since: time.Now().Add(-time.Hour), Until: time.Now(), Complete: true}
	for i := 0; i < 753; i++ {
		d.Projects = append(d.Projects, Project{Name: fmt.Sprintf("project-%03d", i), Metrics: Metrics{Commits: 1, ActiveDays: 1}, Commits: []Commit{{Hash: fmt.Sprintf("%040x", i+1), Subject: "feature work", AuthorTime: time.Now()}}})
	}
	input := Evidence(d)
	if len(input) > MaxAIContextBytes || !strings.Contains(input, "omitted") {
		t.Fatalf("evidence budget/omission failed: %d bytes", len(input))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Worked on feature [deadbeef]."}}]}`)
	}))
	defer server.Close()
	_, err := GenerateAI(context.Background(), d, &config.LLMConfig{Endpoint: server.URL, Model: "test"})
	if err == nil || !strings.Contains(err.Error(), "unknown commit") {
		t.Fatalf("expected unknown reference error, got %v", err)
	}
}

func TestAISummaryAcceptsCollectedReference(t *testing.T) {
	hash := strings.Repeat("a", 40)
	d := Document{Since: time.Now().Add(-time.Hour), Until: time.Now(), Projects: []Project{{Name: "demo", Commits: []Commit{{Hash: hash, AuthorTime: time.Now()}}}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"content":"Completed demo work [%s]."}}]}`, hash[:8])
	}))
	defer server.Close()
	summary, err := GenerateAI(context.Background(), d, &config.LLMConfig{Endpoint: server.URL, Model: "test"})
	if err != nil || !strings.Contains(summary, hash[:8]) {
		t.Fatalf("summary=%q err=%v", summary, err)
	}
}

func TestAuthorTimeRangeSurvivesDifferentCommitterDate(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "me@example.test")
	runGit(t, root, "config", "commit.gpgsign", "false")
	runGit(t, root, "config", "core.hooksPath", filepath.Join(root, "nohooks"))
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "file")
	command := exec.Command("git", "commit", "-m", "old authored work")
	command.Dir = root
	command.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2020-01-02T12:00:00+00:00")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v: %s", err, out)
	}
	since := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	projects, warnings := CollectLocal(context.Background(), []*index.Repo{{Name: "demo", Root: "dev", AbsPath: root}}, Options{Since: since, Until: since.Add(24 * time.Hour)})
	if len(warnings) != 0 || len(projects) != 1 || projects[0].Metrics.Commits != 1 {
		t.Fatalf("author date was lost: warnings=%v projects=%+v", warnings, projects)
	}
}

func TestSensitivePathsAndLines(t *testing.T) {
	if !sensitivePath("config/.env.production") || !sensitivePath("keys/id_ed25519") || sensitivePath("src/main.go") {
		t.Fatal("sensitive path matching failed")
	}
	if !secretLine.MatchString("+ api_key = top-secret") {
		t.Fatal("secret line should be filtered")
	}
}

func TestCollectAllLocalBranchesAndWorktrees(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "project")
	wt := filepath.Join(root, "feature worktree")
	if err := os.Mkdir(base, 0755); err != nil {
		t.Fatal(err)
	}
	runGit(t, base, "init", "-b", "main")
	runGit(t, base, "config", "user.name", "Test")
	runGit(t, base, "config", "user.email", "me@example.test")
	runGit(t, base, "config", "commit.gpgsign", "false")
	runGit(t, base, "config", "core.hooksPath", filepath.Join(root, "empty-hooks"))
	if err := os.WriteFile(filepath.Join(base, "main.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, base, "add", ".")
	runGit(t, base, "commit", "-m", "start")
	runGit(t, base, "worktree", "add", "-b", "feature", wt)
	if err := os.WriteFile(filepath.Join(wt, "unicode ü.txt"), []byte("feature\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, wt, "add", ".")
	runGit(t, wt, "commit", "-m", "feature work")
	if err := os.WriteFile(filepath.Join(wt, "unfinished.txt"), []byte("draft\n"), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	start := now.Add(-time.Hour)
	projects, warnings := CollectLocal(context.Background(), []*index.Repo{{Name: "project", Root: "dev", AbsPath: base}, {Name: "feature worktree", Root: "dev", AbsPath: wt}}, Options{Since: start, Until: now.Add(time.Hour)})
	if len(warnings) > 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if len(projects) != 1 {
		t.Fatalf("want one worktree group, got %d", len(projects))
	}
	p := projects[0]
	if p.Metrics.Commits != 2 || p.Metrics.CurrentChangedPaths != 1 || len(p.Worktrees) != 2 {
		t.Fatalf("unexpected metrics: %+v", p.Metrics)
	}
	if p.Metrics.Additions != 2 || p.Metrics.ChangedPaths != 2 {
		t.Fatalf("unexpected change volume: %+v, commits: %+v", p.Metrics, p.Commits)
	}
	if !strings.Contains(strings.Join(p.Commits[0].ChangedPaths, ","), "ü") {
		t.Fatalf("Unicode path missing: %+v", p.Commits[0].ChangedPaths)
	}
}

func TestCorruptBranchFallsBackToHeadAndWarns(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "user.name", "Test")
	runGit(t, repo, "config", "user.email", "me@example.test")
	runGit(t, repo, "config", "commit.gpgsign", "false")
	runGit(t, repo, "config", "core.hooksPath", filepath.Join(root, "nohooks"))
	if err := os.WriteFile(filepath.Join(repo, "file"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "file")
	runGit(t, repo, "commit", "-m", "working branch")
	if err := os.WriteFile(filepath.Join(repo, ".git", "refs", "heads", "broken"), []byte("invalid\n"), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	projects, warnings := CollectLocal(context.Background(), []*index.Repo{{Name: "repo", Root: "dev", AbsPath: repo}}, Options{Since: now.Add(-time.Hour), Until: now.Add(time.Hour)})
	if len(warnings) != 0 || len(projects) != 1 || projects[0].Metrics.Commits != 1 {
		t.Fatalf("unexpected result: warnings=%v projects=%+v", warnings, projects)
	}
	if len(projects[0].Warnings) == 0 || !strings.Contains(projects[0].Warnings[0], "local branch history") {
		t.Fatalf("missing partial history warning: %v", projects[0].Warnings)
	}
}

func TestBranchNotCheckedOutIsIncluded(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "user.name", "Test")
	runGit(t, repo, "config", "user.email", "me@example.test")
	runGit(t, repo, "config", "commit.gpgsign", "false")
	runGit(t, repo, "config", "core.hooksPath", filepath.Join(root, "nohooks"))
	if err := os.WriteFile(filepath.Join(repo, "base"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "base")
	runGit(t, repo, "commit", "-m", "base")
	runGit(t, repo, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(repo, "feature"), []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "feature")
	runGit(t, repo, "commit", "-m", "feature")
	runGit(t, repo, "checkout", "main")
	now := time.Now()
	projects, warnings := CollectLocal(context.Background(), []*index.Repo{{Name: "repo", Root: "dev", AbsPath: repo}}, Options{Since: now.Add(-time.Hour), Until: now.Add(time.Hour)})
	if len(warnings) > 0 || len(projects) != 1 || projects[0].Metrics.Commits != 2 {
		t.Fatalf("unchecked-out branch missing: warnings=%v projects=%+v", warnings, projects)
	}
}

func TestReportRenderEscapesHTMLAndSeparatesWorktreeChanges(t *testing.T) {
	d := Document{SchemaVersion: 1, Since: time.Now(), Until: time.Now(), Complete: true, ConfiguredRoots: []string{"dev"}, Projects: []Project{{Name: "<script>alert(1)</script>", Metrics: Metrics{CurrentChangedPaths: 1}, Worktrees: []Worktree{{Path: "/repo", ChangedPaths: []string{"draft"}}}}}}
	html, err := HTML(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>alert(1)</script>") || !strings.Contains(html, "&lt;script&gt;") {
		t.Fatal("HTML did not escape repository name")
	}
	if !strings.Contains(html, "No matching authored commits") || !strings.Contains(html, "Current working-tree changes (undated)") {
		t.Fatal("HTML attributed undated edits to the week")
	}
	md := Markdown(d, "log")
	if !strings.Contains(md, "Current working-tree changes") {
		t.Fatal("working tree changes missing from log")
	}
}

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
	"reflect"
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

func TestExplainCoverageDistinguishesReportFiltersAndDiscovery(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "projects", "group", "repo")
	cfg := &config.Config{Roots: []config.Root{{Name: "projects", Path: filepath.Join(root, "projects"), MaxDepth: 2}}}
	repo := &index.Repo{Name: "repo", AbsPath: repoPath, Root: "projects"}
	d := Document{Projects: []Project{{Name: "repo", Paths: []string{repoPath}}}}
	msg := ExplainCoverage("repo", cfg, []*index.Repo{repo}, d)
	if !strings.Contains(msg, "--since") || !strings.Contains(msg, "--author-email") {
		t.Fatalf("unexpected filter explanation: %s", msg)
	}
	tooDeep := filepath.Join(root, "projects", "a", "b", "c")
	msg = ExplainCoverage(tooDeep, cfg, nil, Document{})
	if !strings.Contains(msg, "beyond max_depth 2") {
		t.Fatalf("unexpected depth explanation: %s", msg)
	}
	msg = ExplainCoverage(filepath.Join(root, "other", "repo"), cfg, nil, Document{})
	if !strings.Contains(msg, "outside the current Configured Roots") {
		t.Fatalf("unexpected root explanation: %s", msg)
	}
}

func TestExplainCoverageReportsDashboardLimitAndGlobalExclusion(t *testing.T) {
	root := t.TempDir()
	projectsRoot := filepath.Join(root, "projects")
	cfg := &config.Config{Roots: []config.Root{{Name: "projects", Path: projectsRoot, MaxDepth: 4}}, GlobalExcludes: []string{"node_modules"}}
	var d Document
	for i := 0; i < 51; i++ {
		name := fmt.Sprintf("project-%02d", i)
		projectPath := filepath.Join(projectsRoot, name)
		d.Projects = append(d.Projects, Project{Name: name, Paths: []string{projectPath}, Metrics: Metrics{Commits: 1, ActiveDays: 1}, Commits: []Commit{{Hash: fmt.Sprintf("%040x", i+1)}}})
	}
	SortProjects(d.Projects)
	target := d.Projects[50]
	indexed := &index.Repo{Name: target.Name, AbsPath: target.Paths[0], Root: "projects"}
	message := ExplainCoverage(target.Name, cfg, []*index.Repo{indexed}, d)
	if !strings.Contains(message, "omitted from the Dashboard") {
		t.Fatalf("display-limit explanation missing: %s", message)
	}
	message = ExplainCoverage(filepath.Join(projectsRoot, "node_modules", "missing"), cfg, nil, Document{})
	if !strings.Contains(message, "excluded folder node_modules") {
		t.Fatalf("global exclusion explanation missing: %s", message)
	}
	marker := filepath.Join(projectsRoot, "old", "repo")
	if err := os.MkdirAll(filepath.Join(marker, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	message = ExplainCoverage(marker, cfg, nil, Document{})
	if !strings.Contains(message, "Git marker found") {
		t.Fatalf("invalid marker explanation missing: %s", message)
	}
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

func TestAIEvidenceIncludesOnlySafeCurrentFilenames(t *testing.T) {
	d := Document{Since: time.Now().Add(-time.Hour), Until: time.Now(), Projects: []Project{{Name: "demo", Metrics: Metrics{CurrentChangedPaths: 2}, Worktrees: []Worktree{{ChangedPaths: []string{"src/new.go", "config/.env.production"}}}}}}
	evidence := Evidence(d)
	if !strings.Contains(evidence, "src/new.go") || strings.Contains(evidence, ".env.production") || !strings.Contains(evidence, "omitted") {
		t.Fatalf("current filenames were not filtered: %s", evidence)
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
	baseCommon, baseErr := readCommonGitDir(base)
	worktreeCommon, worktreeErr := readCommonGitDir(wt)
	if baseErr != nil || worktreeErr != nil || baseCommon != worktreeCommon {
		t.Fatalf("native worktree grouping failed: base=%q (%v), worktree=%q (%v)", baseCommon, baseErr, worktreeCommon, worktreeErr)
	}
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
	grouped := 0
	projects, warnings := CollectLocal(context.Background(), []*index.Repo{{Name: "project", Root: "dev", AbsPath: base}, {Name: "feature worktree", Root: "dev", AbsPath: wt}}, Options{Since: start, Until: now.Add(time.Hour), OnGrouping: func(done, total int) {
		if total != 2 {
			t.Errorf("grouping total=%d", total)
		}
		grouped = done
	}})
	if grouped != 2 {
		t.Fatalf("grouping progress ended at %d", grouped)
	}
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

func TestBatchedCommitStatsMatchGitShow(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "user.name", "Test")
	runGit(t, repo, "config", "user.email", "me@example.test")
	runGit(t, repo, "config", "commit.gpgsign", "false")
	runGit(t, repo, "config", "core.hooksPath", filepath.Join(repo, "nohooks"))
	file := filepath.Join(repo, "unicode ü spaced.txt")
	var commits []Commit
	for i := range 17 {
		if err := os.WriteFile(file, []byte(strings.Repeat("line\n", i+1)), 0644); err != nil {
			t.Fatal(err)
		}
		runGit(t, repo, "add", ".")
		runGit(t, repo, "commit", "-m", fmt.Sprintf("commit %d", i))
		commits = append(commits, Commit{Hash: runGit(t, repo, "rev-parse", "HEAD")})
	}
	if err := os.WriteFile(filepath.Join(repo, "binary.dat"), []byte{0, 1, 2, 3}, 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "binary.dat")
	runGit(t, repo, "commit", "-m", "binary")
	commits = append(commits, Commit{Hash: runGit(t, repo, "rev-parse", "HEAD")})
	runGit(t, repo, "commit", "--allow-empty", "-m", "empty")
	commits = append(commits, Commit{Hash: runGit(t, repo, "rev-parse", "HEAD")})

	if warnings := fillCommitStats(context.Background(), repo, commits); len(warnings) != 0 {
		t.Fatalf("batch warnings: %v", warnings)
	}
	for i := range commits {
		out, err := git(context.Background(), repo, 5*time.Second, 256<<10, "show", "--format=", "--numstat", "-z", "--no-renames", "--root", commits[i].Hash)
		if err != nil {
			t.Fatal(err)
		}
		want := Commit{}
		parseNumstat(out, &want)
		got := commits[i]
		if !reflect.DeepEqual(got.ChangedPaths, want.ChangedPaths) || got.Additions != want.Additions || got.Deletions != want.Deletions || got.BinaryChanges != want.BinaryChanges {
			t.Fatalf("commit %d stats mismatch: got=%+v want=%+v", i, got, want)
		}
	}
	if commits[17].BinaryChanges != 1 || len(commits[18].ChangedPaths) != 0 {
		t.Fatalf("binary and empty commits lost: %+v %+v", commits[17], commits[18])
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

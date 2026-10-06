package setup

import (
	"context"
	"github.com/Geogboe/rog/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDiscoverLocatesMarkersAndHonorsExcludes(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "projects", "real repo")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", repo)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	bad := filepath.Join(root, "projects", "archive", "invalid")
	if err := os.MkdirAll(filepath.Join(bad, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	result := Discover(context.Background(), []SearchRoot{{Name: "test", Path: root}}, []string{"archive"}, nil)
	if len(result.Candidates) != 1 || result.Candidates[0].Path != repo {
		t.Fatalf("unexpected candidates: %+v", result.Candidates)
	}
	if result.Rejected != 0 {
		t.Fatalf("excluded invalid marker should not be visited; rejected=%d", result.Rejected)
	}
}

func TestDiscoverLeavesGitMarkerValidationToFullScan(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "not-git")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	result := Discover(context.Background(), []SearchRoot{{Name: "test", Path: root}}, nil, nil)
	if len(result.Candidates) != 1 || result.Rejected != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestDiscoverPrunesNestedCacheExclusion(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "home", "me", "go", "pkg", "mod", "cached-module")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	result := Discover(context.Background(), []SearchRoot{{Name: "all", Path: root}}, []string{"**/go/pkg/mod"}, nil)
	if len(result.Candidates) != 0 {
		t.Fatalf("nested cache exclusion was not applied: %+v", result.Candidates)
	}
}

func TestDiscoverDeduplicatesOverlappingRootsAndUsesNarrowest(t *testing.T) {
	outer := t.TempDir()
	repo := filepath.Join(outer, "dev", "projects", "repo")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", repo)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	inner := filepath.Join(outer, "dev")
	result := Discover(context.Background(), []SearchRoot{{Name: "outer", Path: outer}, {Name: "inner", Path: inner}}, nil, nil)
	if len(result.Candidates) != 1 || result.Overlaps != 1 || result.Candidates[0].Root != "inner" {
		t.Fatalf("overlap was not resolved to narrowest root: %+v", result)
	}
}

func TestDiscoverCancellationIsReportedAsPartialCoverage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := Discover(ctx, []SearchRoot{{Name: "root", Path: t.TempDir()}}, nil, nil)
	if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, " "), "cancel") {
		t.Fatalf("cancelled discovery has no incomplete warning: %+v", result)
	}
}

func TestSuggestionsUseCandidateOperatingSystemPaths(t *testing.T) {
	candidates := []Candidate{
		{Path: `C:\Users\george\dev\projects\rog`, Root: "drive-C", Windows: true},
		{Path: `/home/george/dev-linux/projects/rog`, Root: "wsl-Ubuntu", WSL: true, Distro: "Ubuntu"},
	}
	got := Suggestions(candidates, nil)
	if len(got) != 2 {
		t.Fatalf("got %d suggestions: %+v", len(got), got)
	}
	var windowsRoot, wslRoot bool
	for _, s := range got {
		if s.Root.Path == `C:\Users\george\dev\projects` && strings.HasPrefix(s.Root.Name, "projects") && s.Root.MaxDepth == 1 {
			windowsRoot = true
		}
		if s.Root.Path == "/home/george/dev-linux/projects" && s.Root.WSL && s.Root.WSLDistro == "Ubuntu" && s.Root.MaxDepth == 1 {
			wslRoot = true
		}
	}
	if !windowsRoot || !wslRoot {
		t.Fatalf("cross-OS paths were grouped incorrectly: %+v", got)
	}
}

func TestSuggestionsIncreaseExistingDepthForObservedRepository(t *testing.T) {
	existing := []config.Root{{Name: "projects", Path: "/home/me/projects", MaxDepth: 1}}
	got := Suggestions([]Candidate{{Path: "/home/me/projects/group/repo", Root: "linux"}}, existing)
	if len(got) != 1 || got[0].Root.MaxDepth != 2 || got[0].Count != 1 {
		t.Fatalf("suggestion does not cover nested repo: %+v", got)
	}
}

func TestCountCoveredCandidatesUsesDepthExclusionsAndUniqueOverlap(t *testing.T) {
	parent := "/home/me/projects"
	nested := "/home/me/projects/repos"
	candidates := []Candidate{
		{Path: "/home/me/projects/app", Root: "search"},
		{Path: "/home/me/projects/repos/tool", Root: "search"},
	}
	shallow := config.Root{Name: "projects", Path: parent, MaxDepth: 1}
	if got := CountCoveredCandidates(candidates, []config.Root{shallow}, nil); got != 1 {
		t.Fatalf("depth-1 parent covers %d discoveries, want 1", got)
	}
	deep := shallow
	deep.MaxDepth = 2
	if got := CountCoveredCandidates(candidates, []config.Root{deep}, nil); got != 2 {
		t.Fatalf("depth-2 parent covers %d discoveries, want 2", got)
	}
	inner := config.Root{Name: "repos", Path: nested, MaxDepth: 1}
	if got := CountCoveredCandidates(candidates, []config.Root{shallow, inner}, nil); got != 2 {
		t.Fatalf("overlapping roots double-counted or missed discoveries: got %d, want 2", got)
	}
	if got := CountCoveredCandidates(candidates, []config.Root{deep}, []string{"repos"}); got != 1 {
		t.Fatalf("excluded nested repository was counted as covered: got %d, want 1", got)
	}
}

func TestNativeWindowsDiscoveryIsCoveredByNativeWindowsRoot(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native Windows root semantics")
	}
	root := config.Root{Name: "rog-test", Path: `C:\Users\me\dev\rog`, MaxDepth: 1}
	candidate := Candidate{Path: root.Path, Root: root.Name, Valid: true}
	if got := CountCoveredCandidates([]Candidate{candidate}, []config.Root{root}, nil); got != 1 {
		t.Fatalf("native Windows root covered %d of 1 discovered repositories", got)
	}
}

func TestNestedWithinReportsClosestContainingSuggestion(t *testing.T) {
	suggestions := []RootSuggestion{
		{Root: config.Root{Name: "projects", Path: "/home/me/projects"}},
		{Root: config.Root{Name: "repos", Path: "/home/me/projects/repos"}},
		{Root: config.Root{Name: "source", Path: "/home/me/projects/repos/source"}},
	}
	if got := NestedWithin(suggestions[2].Root, suggestions); got != "repos" {
		t.Fatalf("closest nested parent = %q, want repos", got)
	}
}

func TestSuggestionsDoNotSelectCoveredNestedRootButKeepItAvailable(t *testing.T) {
	existing := []config.Root{{Name: "projects", Path: "/home/me/projects", MaxDepth: 4}}
	candidates := []Candidate{
		{Path: "/home/me/projects/app", Root: "search"},
		{Path: "/home/me/projects/repos/tool", Root: "search"},
	}
	got := Suggestions(candidates, existing)
	if len(got) != 2 {
		t.Fatalf("got %d suggestions, want parent and nested child: %+v", len(got), got)
	}
	var parent, child *RootSuggestion
	for i := range got {
		switch got[i].Root.Path {
		case "/home/me/projects":
			parent = &got[i]
		case "/home/me/projects/repos":
			child = &got[i]
		}
	}
	if parent == nil || !parent.Selected || child == nil || child.Selected {
		t.Fatalf("redundant nested root was not kept available but deselected: %+v", got)
	}
	if covered := CountCoveredCandidates(candidates, []config.Root{parent.Root}, nil); covered != 2 {
		t.Fatalf("parent coverage=%d, want both discoveries", covered)
	}
}

func TestDiscoverNeedsNoGitAndDoesNotReadMarkerContents(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"directory", "worktree"} {
		repo := filepath.Join(root, name)
		if err := os.MkdirAll(repo, 0755); err != nil {
			t.Fatal(err)
		}
		if name == "directory" {
			if err := os.Mkdir(filepath.Join(repo, ".git"), 0755); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("intentionally invalid git contents"), 0000); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", t.TempDir())
	result := Discover(context.Background(), []SearchRoot{{Name: "local", Path: root}}, nil, nil)
	if len(result.Candidates) != 2 || result.Rejected != 0 {
		t.Fatalf("marker-only discovery: %+v", result)
	}
	for _, c := range result.Candidates {
		if c.Valid || c.Email != "" || len(c.AuthorEmails) != 0 {
			t.Fatal("discovery returned Git metadata")
		}
	}
}

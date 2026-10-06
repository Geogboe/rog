package setupui

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/setup"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"gopkg.in/yaml.v3"
)

func key(m model, value string) model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)})
	return next.(model)
}
func enter(m model) model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return next.(model)
}
func fixture(t *testing.T) model {
	t.Helper()
	file := filepath.Join(t.TempDir(), "config.yml")
	root := filepath.ToSlash(filepath.Join(t.TempDir(), "dev"))
	cfg := &config.Config{Roots: []config.Root{{Name: "dev", Path: root, MaxDepth: 2}}, GlobalExcludes: []string{"node_modules"}, Report: &config.ReportConfig{AuthorEmails: []string{"test@example.test"}}}
	original, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	original = append(original, []byte("future_key: retained\n")...)
	if err := os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	return newModel(context.Background(), file, cfg, nil, nil, nil, original)
}

func TestWizardManualRootsTransaction(t *testing.T) {
	m := fixture(t)
	m = key(m, "a")
	m = key(m, filepath.ToSlash(filepath.Join(t.TempDir(), "new projects")))
	m = enter(m)
	if len(m.suggestions) != 2 || m.suggestions[1].Root.MaxDepth != 4 {
		t.Fatalf("manual roots: %+v", m.suggestions)
	}
	m = key(m, "+")
	m = enter(m)
	if m.step != 2 {
		t.Fatalf("review: %s", m.status)
	}
	data, _ := os.ReadFile(m.path)
	if !bytes.Equal(data, m.original) {
		t.Fatal("root edits changed file before confirmation")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(m.path), "setup-history")); !os.IsNotExist(err) {
		t.Fatal("history created before confirmation")
	}
	preview := append([]byte(nil), m.preview...)
	at := m.previewAt
	m = m.apply()().(applyMessage).model
	if m.step != 3 {
		t.Fatalf("save: %s", m.status)
	}
	data, _ = os.ReadFile(m.path)
	if !bytes.Equal(data, preview) {
		t.Fatal("saved data differs from reviewed proposal")
	}
	for _, needle := range []string{"future_key: retained", "test@example.test", "node_modules"} {
		if !strings.Contains(string(data), needle) {
			t.Fatalf("lost %s", needle)
		}
	}
	backup := filepath.Join(filepath.Dir(m.path), "setup-history", at.Local().Format("20060102-150405.000000000")+".yml")
	data, err := os.ReadFile(backup)
	if err != nil || !bytes.Equal(data, m.original) {
		t.Fatalf("backup: %v", err)
	}
	m = key(m, "n")
	if !m.finished || m.out.ScanNow || m.out.Cancelled {
		t.Fatal("decline scan must retain successful setup")
	}
}

func TestWizardCancelEveryStage(t *testing.T) {
	for _, stage := range []int{0, 1, 2} {
		t.Run(string(rune('0'+stage)), func(t *testing.T) {
			m := fixture(t)
			m.step = stage
			m = key(m, "q")
			data, _ := os.ReadFile(m.path)
			if !m.out.Cancelled || !bytes.Equal(data, m.original) {
				t.Fatal("cancellation changed state")
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(m.path), "setup-history")); !os.IsNotExist(err) {
				t.Fatal("cancel created history")
			}
		})
	}
}

func TestWizardChangedConfigRequiresReviewAgain(t *testing.T) {
	m := fixture(t)
	m = enter(m)
	changed := bytes.ReplaceAll(m.original, []byte("test@example.test"), []byte("changed@example.test"))
	changed = append(changed, []byte("another_future_key: keep\n")...)
	if err := os.WriteFile(m.path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	m = m.apply()().(applyMessage).model
	if m.step != 2 || !strings.Contains(m.status, "press y again") {
		t.Fatalf("missing conflict guard: %s", m.status)
	}
	data, _ := os.ReadFile(m.path)
	if !bytes.Equal(data, changed) {
		t.Fatal("overwrote concurrent changes")
	}
	m = m.apply()().(applyMessage).model
	data, _ = os.ReadFile(m.path)
	if m.step != 3 || !strings.Contains(string(data), "changed@example.test") || !strings.Contains(string(data), "another_future_key") {
		t.Fatalf("refresh lost settings: %s", m.status)
	}
}

func TestWSLSelectionControlsDraftAndDiscovery(t *testing.T) {
	m := fixture(t)
	m.config.Roots = append(m.config.Roots, config.Root{Name: "ubuntu", Path: "/home/me/dev", MaxDepth: 3, WSL: true, WSLDistro: "Ubuntu"})
	external := []setup.SearchRoot{{Name: "Ubuntu", Path: "/", WSL: true, Distro: "Ubuntu"}, {Name: "Debian", Path: "/", WSL: true, Distro: "Debian"}}
	m = newModel(m.ctx, m.path, m.config, []setup.SearchRoot{{Name: "local", Path: "/"}}, external, nil, m.original)
	if m.step != 0 || !m.selected["Ubuntu"] || m.selected["Debian"] {
		t.Fatal("existing distributions not preselected")
	}
	m = key(m, " ") // Deselect Ubuntu; choose Debian.
	m.cursor = 1
	m = key(m, " ")
	m = enter(m)
	if len(m.draftRoots()) != 1 {
		t.Fatal("deselected WSL root remains in draft")
	}
	m.discover = func(_ context.Context, roots []setup.SearchRoot, _ []string, progress func(string, string, int, int)) setup.DiscoveryResult {
		if len(roots) != 2 || roots[1].Distro != "Debian" {
			t.Fatalf("discovery environments: %+v", roots)
		}
		progress("Debian", "dev", 0, 0)
		return setup.DiscoveryResult{Candidates: []setup.Candidate{{Path: "/home/me/dev/repo", WSL: true, Distro: "Debian"}}, Warnings: []string{"permission denied elsewhere"}}
	}
	next, batch := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(model)
	cmds := batch().(tea.BatchMsg)
	done := cmds[1]()
	next, _ = m.Update(done)
	m = next.(model)
	if m.busy || len(m.draftRoots()) != 2 || len(m.result.Warnings) != 1 {
		t.Fatalf("merged discovery: %+v", m.suggestions)
	}
	m = enter(m)
	m.height = 0
	text := ansi.Strip(m.View())
	if !strings.Contains(text, "Partial discovery") {
		t.Fatal("missing discovery warning")
	}
}

func TestDiscoveryKeepsManualDepthAndChoice(t *testing.T) {
	m := fixture(t)
	manual := m.suggestions[0].Root
	m.suggestions[0].Selected = false
	recommended := manual
	recommended.MaxDepth = 6
	next, _ := m.Update(discoveryDone{suggestions: []setup.RootSuggestion{{Root: recommended, Selected: true, Count: 3}}})
	m = next.(model)
	if m.suggestions[0].Selected || m.suggestions[0].Root.MaxDepth != manual.MaxDepth {
		t.Fatal("discovery replaced explicit choice")
	}
}

func TestNewConfigStartsEmptyAndRequiresRoot(t *testing.T) {
	m := newModel(context.Background(), filepath.Join(t.TempDir(), "config.yml"), config.DefaultConfig(), nil, nil, nil, nil)
	m = enter(m)
	if m.step != 1 || len(m.suggestions) != 0 || !strings.Contains(m.status, "discover") {
		t.Fatal("empty config should offer discovery or manual entry")
	}
}

func TestManualWSLAndWindowsRoots(t *testing.T) {
	m := fixture(t)
	m.external = []setup.SearchRoot{{WSL: true, Distro: "Ubuntu"}}
	m.selected["Ubuntu"] = true
	m = key(m, "a")
	m.inputDistro = "Ubuntu"
	m = key(m, "/home/me/dev")
	m = enter(m)
	last := m.suggestions[len(m.suggestions)-1].Root
	if !last.WSL || last.WSLDistro != "Ubuntu" || last.MaxDepth != 4 {
		t.Fatal("WSL root ownership lost")
	}
	m = key(m, "a")
	m = key(m, `C:\Users\me\dev`)
	m = enter(m)
	last = m.suggestions[len(m.suggestions)-1].Root
	if !setup.IsWindowsDrivePath(last.Path) {
		t.Fatal("Windows path not retained")
	}
	if err := setup.ValidateSetupConfig(&config.Config{Roots: m.draftRoots()}); err != nil {
		t.Fatal(err)
	}
}

func TestWizardEditingAndInvalidInput(t *testing.T) {
	m := fixture(t)
	m = key(m, "e")
	m.locationInput = "relative/path"
	m = enter(m)
	if !m.addingLocation || !strings.Contains(m.status, "absolute") {
		t.Fatal("invalid input must stay editable")
	}
	m.locationInput = filepath.ToSlash(filepath.Join(t.TempDir(), "renamed"))
	m = enter(m)
	if m.suggestions[0].Root.Name != "dev" || m.suggestions[0].Root.MaxDepth != 2 {
		t.Fatal("edit lost name or depth")
	}
}

func TestWizardLayoutKeepsActionsVisible(t *testing.T) {
	m := fixture(t)
	for i := 0; i < 20; i++ {
		m.suggestions = append(m.suggestions, setup.RootSuggestion{Root: config.Root{Name: "long project name", Path: "/home/me/dev/long/project/path", MaxDepth: 4}, Selected: true})
	}
	m.width, m.height, m.cursor = 60, 24, 15
	text := ansi.Strip(m.View())
	if !strings.Contains(text, "NEXT") || !strings.Contains(text, "> [✓]") {
		t.Fatalf("focused row/actions hidden:\n%s", text)
	}
	for _, line := range strings.Split(text, "\n") {
		if ansi.StringWidth(line) > m.width {
			t.Fatal("line exceeds width")
		}
	}
	if len(strings.Split(text, "\n")) > m.height {
		t.Fatal("view exceeds height")
	}
}

func TestSavedScanOptInAndContextCancellation(t *testing.T) {
	for _, choice := range []string{"s", "y", "n", "ctrl+c"} {
		m := fixture(t)
		m = enter(m)
		m = m.apply()().(applyMessage).model
		if choice == "ctrl+c" {
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
			m = next.(model)
		} else {
			m = key(m, choice)
		}
		if !m.finished || m.out.Cancelled || m.out.ScanNow != (choice == "s" || choice == "y") {
			t.Fatalf("scan choice %s", choice)
		}
	}
	m := fixture(t)
	m.step = 3
	next, _ := m.Update(setupContextDone{})
	if next.(model).out.Cancelled {
		t.Fatal("post-save context cancellation must not report setup cancelled")
	}
}

func TestSetupColorRespectsNoColorAndDumbTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm-256color")
	if supportsColor() {
		t.Fatal("NO_COLOR should disable colors")
	}
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	if supportsColor() {
		t.Fatal("dumb terminal should disable colors")
	}
}

func TestWizardBackupFailureLeavesConfigUnchanged(t *testing.T) {
	m := fixture(t)
	m = enter(m)
	backup := filepath.Join(filepath.Dir(m.path), "setup-history", m.previewAt.Local().Format("20060102-150405.000000000")+".yml")
	if err := os.MkdirAll(filepath.Dir(backup), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("existing revision"), 0600); err != nil {
		t.Fatal(err)
	}
	m = m.apply()().(applyMessage).model
	data, _ := os.ReadFile(m.path)
	if m.step != 2 || !bytes.Equal(data, m.original) || !strings.Contains(m.status, "not applied") {
		t.Fatal("backup failure changed config or advanced")
	}
}

func TestWizardPreservesRawRootPathsAndUnknownFields(t *testing.T) {
	m := fixture(t)
	m.original = []byte("roots:\n  - name: dev\n    path: ~/dev\n    max_depth: 2\n    future_root: keep\nreport:\n  author_emails: [raw@example.test]\nglobal_excludes: []\n")
	if err := os.WriteFile(m.path, m.original, 0600); err != nil {
		t.Fatal(err)
	}
	// Loaded defaults and overrides must not replace the user's YAML settings.
	m.suggestions[0].Root.MaxDepth = 5
	m = enter(m)
	m = m.apply()().(applyMessage).model
	data, _ := os.ReadFile(m.path)
	for _, expected := range []string{"path: ~/dev", "future_root: keep", "raw@example.test", "global_excludes: []", "max_depth: 5"} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("lost %s: %s", expected, data)
		}
	}
}

func TestFailedConflictRefreshCannotReuseStalePreview(t *testing.T) {
	m := fixture(t)
	m = enter(m)
	changed := bytes.ReplaceAll(m.original, []byte("node_modules"), []byte("\"\""))
	if err := os.WriteFile(m.path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	m = m.apply()().(applyMessage).model
	m = m.apply()().(applyMessage).model
	data, _ := os.ReadFile(m.path)
	if !bytes.Equal(data, changed) || m.step == 3 {
		t.Fatal("second confirmation saved a stale preview after failed conflict refresh")
	}
}

func TestManualEntryVisibleWithManyRoots(t *testing.T) {
	m := fixture(t)
	for i := 0; i < 12; i++ {
		m.suggestions = append(m.suggestions, setup.RootSuggestion{Root: config.Root{Name: "other", Path: "/home/me/other", MaxDepth: 4}, Selected: true})
	}
	m.width, m.height = 80, 24
	m = key(m, "a")
	text := ansi.Strip(m.View())
	if !strings.Contains(text, "Root path:") || !strings.Contains(text, "Environment:") {
		t.Fatalf("entry is hidden below the root list:\n%s", text)
	}
}

func TestManualRootIdentityNormalizesPathsWithinEnvironment(t *testing.T) {
	pairs := [][2]config.Root{
		{{Path: "/home/me/dev/"}, {Path: "/home/me/dev"}},
		{{Path: `C:\Dev\`, Windows: true}, {Path: "c:/dev", Windows: true}},
		{{Path: "/home/me/dev/./", WSL: true, WSLDistro: "Ubuntu"}, {Path: "/home/me/dev", WSL: true, WSLDistro: "Ubuntu"}},
	}
	for _, pair := range pairs {
		if rootKey(pair[0]) != rootKey(pair[1]) {
			t.Errorf("equivalent paths have different identity: %+v", pair)
		}
	}
	if rootKey(config.Root{Path: "/home/me/dev", WSL: true, WSLDistro: "Ubuntu"}) == rootKey(config.Root{Path: "/home/me/dev", WSL: true, WSLDistro: "Debian"}) {
		t.Fatal("different environments collapsed")
	}
}

func TestUnrunDiscoveryDoesNotClaimZeroLocations(t *testing.T) {
	m := fixture(t)
	m.height = 0
	text := ansi.Strip(m.View())
	if !strings.Contains(text, "Discovery has not checked") || strings.Contains(text, "0 repository locations") {
		t.Fatal("unrun discovery displayed a false zero")
	}
}

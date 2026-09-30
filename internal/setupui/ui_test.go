package setupui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/setup"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestWizardBackAndCancelLeavesConfigAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	original := []byte("roots: []\neditor: vi\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	m := model{ctx: context.Background(), path: path, config: config.DefaultConfig()}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.step != 1 {
		t.Fatal("Enter should advance from review to environment selection")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = next.(model)
	if m.step != 0 {
		t.Fatal("Back should return to review")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = next.(model)
	if !m.out.Cancelled {
		t.Fatal("q should cancel setup")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatalf("cancel changed config: %s", after)
	}
}

func TestReviewPageStartsConciseAndCanShowConfigurationDetails(t *testing.T) {
	m := model{
		step: 0, width: 44, path: "/home/george/.config/rog/config.yml",
		config:       &config.Config{Roots: []config.Root{{Name: "projects", Path: "/home/george/dev/projects", MaxDepth: 4}}},
		excludes:     []string{"node_modules", "vendor", "dist"},
		indexSummary: IndexSummary{Count: 753, BridgeStatus: "available"},
	}
	initial := ansi.Strip(m.View())
	for _, expected := range []string{"Build your repository map", "CURRENT COVERAGE", "1 Configured Root · 753 repositories indexed", "Enter start · d config · Ctrl+C cancel", "Nothing changes until you apply the preview."} {
		if !strings.Contains(initial, expected) {
			t.Fatalf("concise review page is missing %q:\n%s", expected, initial)
		}
	}
	for _, hidden := range []string{"/home/george/.config/rog/config.yml", "/home/george/dev/projects", "node_modules", "vendor", "Cross-OS bridge:"} {
		if strings.Contains(initial, hidden) {
			t.Fatalf("review details %q should be collapsed by default:\n%s", hidden, initial)
		}
	}
	for _, line := range strings.Split(initial, "\n") {
		if width := ansi.StringWidth(line); width > m.width {
			t.Fatalf("review line width %d exceeds terminal width %d: %q", width, m.width, line)
		}
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(model)
	details := ansi.Strip(m.View())
	for _, expected := range []string{"CURRENT CONFIGURATION", "/home/george/.config/rog/config.yml", "/home/george/dev/projects", "node_modules", "vendor", "Cross-OS bridge:", "d hide"} {
		if !strings.Contains(details, expected) {
			t.Fatalf("expanded review page is missing %q:\n%s", expected, details)
		}
	}
}

func TestWizardTextEntryAcceptsUnicodeAndSpaces(t *testing.T) {
	m := model{addingEmail: true, config: config.DefaultConfig(), selectedEmails: map[string]bool{}}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jane é@example.test")})
	m = next.(model)
	if m.emailInput != "jane é@example.test" {
		t.Fatalf("text input=%q", m.emailInput)
	}
	if m.out.Cancelled {
		t.Fatal("typing q or Unicode must not cancel setup")
	}
}

func TestWizardEditsExclusionsBeforeDiscovery(t *testing.T) {
	m := model{step: 2, cursor: 0, excludes: []string{"node_modules"}, selectedExcludes: map[string]bool{"node_modules": true}}
	update := func(msg tea.KeyMsg) {
		next, _ := m.Update(msg)
		m = next.(model)
	}
	update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if m.selectedExcludes["node_modules"] {
		t.Fatal("space did not disable exclusion")
	}
	update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("generated files")})
	update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.excludes) != 2 || !m.selectedExcludes["generated files"] {
		t.Fatalf("exclusions not edited: %#v %#v", m.excludes, m.selectedExcludes)
	}
}

func TestWizardShowsNestedRootsAndAccurateSelectedCoverage(t *testing.T) {
	parent := config.Root{Name: "projects", Path: "/home/me/projects", MaxDepth: 1}
	child := config.Root{Name: "repos", Path: "/home/me/projects/repos", MaxDepth: 1}
	m := model{
		step:   4,
		config: config.DefaultConfig(),
		result: setup.DiscoveryResult{Candidates: []setup.Candidate{
			{Path: "/home/me/projects/app", Root: "projects"},
			{Path: "/home/me/projects/repos/tool", Root: "projects"},
		}},
		suggestions: []setup.RootSuggestion{
			{Root: parent, Count: 1, Selected: true},
			{Root: child, Count: 1},
		},
	}
	view := m.View()
	for _, expected := range []string{"nested under projects", "Selected roots cover 1 of 2 valid discoveries"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("root selection view missing %q:\n%s", expected, view)
		}
	}
	m.step = 6
	view = m.View()
	for _, expected := range []string{"1 covered by selected Configured Roots", "1 valid discoveries are outside selected roots"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("preview does not call out uncovered discovery %q:\n%s", expected, view)
		}
	}
}

func TestWizardColorAddsHierarchyWithoutChangingLayout(t *testing.T) {
	m := model{
		step: 4, width: 44,
		config: config.DefaultConfig(),
		result: setup.DiscoveryResult{Candidates: []setup.Candidate{{Path: "/home/me/projects/app"}}},
		suggestions: []setup.RootSuggestion{{
			Root: config.Root{Name: "projects", Path: "/home/me/projects", MaxDepth: 2}, Count: 1, Selected: true,
		}},
	}
	plain := m.View()
	m.color = true
	colored := m.View()
	if !strings.Contains(colored, "\x1b[1;36mROG SETUP") || !strings.Contains(colored, "\x1b[32m[✓]") || !strings.Contains(colored, "\x1b[2mSpace toggles") {
		t.Fatalf("colored wizard is missing title, selection, or hint hierarchy:\n%q", colored)
	}
	if got := ansi.Strip(colored); got != plain {
		t.Fatalf("color changed visible content\nplain:\n%s\ncolored:\n%s", plain, got)
	}
	for _, line := range strings.Split(colored, "\n") {
		if width := ansi.StringWidth(line); width > m.width {
			t.Fatalf("colored line width %d exceeds terminal width %d: %q", width, m.width, line)
		}
	}
}

func TestSetupColorRespectsNoColorAndDumbTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm-256color")
	if supportsColor() {
		t.Fatal("NO_COLOR should disable wizard colors")
	}
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	if supportsColor() {
		t.Fatal("TERM=dumb should disable wizard colors")
	}
}

func TestWizardApplyPreservesUnknownConfigAndCreatesPreviewedBackup(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "projects")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.yml")
	original := []byte("roots:\n  - name: old\n    path: " + root + "\n    max_depth: 1\nglobal_excludes: [node_modules]\neditor: vi\nfuture_key: retained\n")
	if err := os.WriteFile(configPath, original, 0600); err != nil {
		t.Fatal(err)
	}
	previewAt := time.Date(2026, 9, 30, 15, 4, 5, 123000000, time.FixedZone("test", -4*60*60))
	m := model{
		config: &config.Config{Roots: []config.Root{{Name: "old", Path: root, MaxDepth: 1}}, GlobalExcludes: []string{"node_modules"}, Editor: "vi"},
		path:   configPath, step: 6, previewAt: previewAt,
		suggestions: []setup.RootSuggestion{{Root: config.Root{Name: "projects", Path: root, MaxDepth: 2}, Count: 1, Selected: true}},
		excludes:    []string{"node_modules"}, selectedExcludes: map[string]bool{"node_modules": true},
	}
	message := m.apply()()
	updated := message.(applyMessage).model
	if updated.step != 7 {
		t.Fatalf("apply did not advance to saved screen: step=%d status=%q", updated.step, updated.status)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"name: projects", "future_key: retained", "global_excludes:"} {
		if !strings.Contains(string(data), needle) {
			t.Fatalf("applied config lost %q: %s", needle, data)
		}
	}
	backupName := previewAt.Local().Format("20060102-150405.000000000") + ".yml"
	backup := filepath.Join(dir, "setup-history", backupName)
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("previewed backup %q was not created: %v", backup, err)
	}
}

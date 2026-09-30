package setupui

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
	initialText := strings.Join(strings.Fields(initial), " ")
	for _, expected := range []string{"Ready to map your repositories?", "CURRENT INDEX", "753 repositories", "1 Configured Root", "Press Enter to choose locations", "Nothing is saved until you confirm the preview."} {
		if !strings.Contains(initialText, expected) {
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

func TestAddWindowsLocationFromWSL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows paths are native search locations on Windows")
	}
	m := model{step: 1, addingLocation: true, locationInput: `C:\Users\me\dev projects`, selected: map[string]bool{}}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if len(m.external) != 1 || !m.external[0].Windows || m.external[0].Name != "dev projects" || !m.selected[searchRootKey(m.external[0])] {
		t.Fatalf("Windows path was not added as a selected bridge location: %+v", m)
	}
}

func TestInvalidAddedLocationExplainsCorrection(t *testing.T) {
	m := model{step: 1, addingLocation: true, locationInput: "relative/path", selected: map[string]bool{}}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if !m.addingLocation || !strings.Contains(m.status, "absolute") || len(m.local)+len(m.external) != 0 {
		t.Fatalf("invalid path should remain editable with an explanation: %+v", m)
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

func TestWizardShowsRootChoicesAndAccurateSelectedCoverage(t *testing.T) {
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
	for _, expected := range []string{"projects · depth 1", "repos · depth 1", "Coverage: 1/2 discoveries covered"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("root selection view missing %q:\n%s", expected, view)
		}
	}
	m.step = 6
	view = m.View()
	for _, expected := range []string{"1 of 2 discovered repositories covered", "1 are outside selected roots"} {
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
	if !strings.Contains(colored, "\x1b[1;36mROG SETUP") || !strings.Contains(colored, "\x1b[32m[✓]") || !strings.Contains(colored, "\x1b[2m    ↑/↓ move · Space toggle") {
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

func TestEachWizardPageLeadsWithOneClearQuestion(t *testing.T) {
	actions := []string{
		"Press Enter to choose locations",
		"Press Enter to review exclusions",
		"Press Enter to start discovery",
		"Press Enter to choose roots",
		"Press Enter to review report emails",
		"Press Enter to preview changes",
		"Press y to save this configuration",
		"Press s to scan the Configured Roots",
	}
	m := model{width: 44, height: 24, config: config.DefaultConfig(), status: "Configuration saved."}
	for step, prompt := range setupQuestions {
		m.step = step
		view := ansi.Strip(m.View())
		if !strings.Contains(view, prompt) {
			t.Fatalf("step %d is missing its prompt %q:\n%s", step+1, prompt, view)
		}
		if questions := strings.Count(view, "?"); questions != 1 {
			t.Fatalf("step %d should present one question, found %d:\n%s", step+1, questions, view)
		}
		if !strings.Contains(view, actions[step]) {
			t.Fatalf("step %d does not make the next action clear (%q):\n%s", step+1, actions[step], view)
		}
		for _, line := range strings.Split(view, "\n") {
			if width := ansi.StringWidth(line); width > m.width {
				t.Fatalf("step %d line width %d exceeds terminal width %d: %q", step+1, width, m.width, line)
			}
		}
	}
}

func TestWelcomePageHighlightsTheNextAction(t *testing.T) {
	m := model{step: 0, width: 80, color: true, config: config.DefaultConfig()}
	view := m.View()
	for _, expected := range []string{"\x1b[1;36mReady to map your repositories?", "\x1b[1;32mPress Enter to choose locations", "\x1b[2mNothing is saved"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("welcome page lacks visual emphasis %q:\n%q", expected, view)
		}
	}
	m.width = 20
	view = m.View()
	if !strings.Contains(view, "\x1b[1;36mReady to map your") || !strings.Contains(view, "\x1b[1;36mrepositories?") {
		t.Fatalf("wrapped question should retain its emphasis on narrow terminals:\n%q", view)
	}
}

func TestWizardActionsStayVisibleAndListsAdaptToTerminalHeight(t *testing.T) {
	roots := make([]setup.SearchRoot, 12)
	for i := range roots {
		roots[i] = setup.SearchRoot{Name: "projects", Path: filepath.Join("/home/george/dev", string(rune('a'+i)))}
	}
	m := model{step: 1, width: 50, height: 14, config: config.DefaultConfig(), local: roots, selected: map[string]bool{}}
	view := ansi.Strip(m.View())
	if lines := strings.Split(view, "\n"); len(lines) > m.height {
		t.Fatalf("environment page exceeds terminal height: %d > %d\n%s", len(lines), m.height, view)
	}
	if !strings.Contains(view, "Press Enter to review exclusions") || !strings.Contains(view, "Space toggle") {
		t.Fatalf("environment page hides its next action or controls:\n%s", view)
	}
	if !strings.Contains(view, "projects") {
		t.Fatalf("short terminal should keep the current location visible:\n%s", view)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	view = ansi.Strip(next.(model).View())
	if !strings.Contains(view, "/home/george/dev/b") {
		t.Fatalf("down should move the visible location on a short terminal:\n%s", view)
	}
}

func TestEnvironmentChoicesLabelCrossOperatingSystemRoots(t *testing.T) {
	m := model{
		step: 1, width: 80, height: 24, config: config.DefaultConfig(),
		external: []setup.SearchRoot{
			{Name: "windows-projects", Path: `C:\Users\george\dev\projects`, Windows: true},
			{Name: "ubuntu-projects", Path: "/home/george/dev/projects", WSL: true, Distro: "Ubuntu"},
		},
		selected: map[string]bool{},
	}
	view := ansi.Strip(m.View())
	for _, expected := range []string{"windows-projects · Windows", "ubuntu-projects · WSL Ubuntu", `C:\Users\george\dev\projects`, "/home/george/dev/projects"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("environment choices missing %q:\n%s", expected, view)
		}
	}
}

func TestEnvironmentHintDescribesOwningOSBridge(t *testing.T) {
	m := model{step: 1, width: 80, height: 24, config: config.DefaultConfig()}
	view := ansi.Strip(m.View())
	want := "Windows locations use the installed rog.exe"
	if runtime.GOOS == "windows" {
		want = "Selected WSL distros start during discovery"
	}
	if !strings.Contains(view, want) {
		t.Fatalf("environment hint does not describe this host's bridge: %s", view)
	}
}

func TestRootChoicesKeepTheFocusedRootVisibleOnShortTerminal(t *testing.T) {
	roots := make([]setup.RootSuggestion, 8)
	candidates := make([]setup.Candidate, 8)
	for i := range roots {
		path := filepath.Join("/home/george/dev/projects", string(rune('a'+i)))
		root := config.Root{Name: "project-" + string(rune('a'+i)), Path: path, MaxDepth: 3}
		roots[i] = setup.RootSuggestion{Root: root, Count: 1, Selected: true}
		candidates[i] = setup.Candidate{Path: path, Root: root.Name}
	}
	m := model{
		step: 4, width: 44, height: 14, config: config.DefaultConfig(),
		suggestions: roots, result: setup.DiscoveryResult{Candidates: candidates},
	}
	view := ansi.Strip(m.View())
	if lines := strings.Split(view, "\n"); len(lines) > m.height {
		t.Fatalf("root page exceeds terminal height: %d > %d\n%s", len(lines), m.height, view)
	}
	if !strings.Contains(view, "project-a · depth 3") || !strings.Contains(view, "Coverage:") || !strings.Contains(view, "Press Enter to review report emails") {
		t.Fatalf("short root page should show its focused choice, coverage, and next action:\n%s", view)
	}
}

func TestSetupPreviewKeepsActionsVisibleAndScrollsLongChanges(t *testing.T) {
	roots := make([]setup.RootSuggestion, 8)
	candidates := make([]setup.Candidate, 8)
	for i := range roots {
		path := filepath.Join("/home/george/dev/projects", string(rune('a'+i)))
		root := config.Root{Name: "project-" + string(rune('a'+i)), Path: path, MaxDepth: 3}
		roots[i] = setup.RootSuggestion{Root: root, Count: 1, Selected: true}
		candidates[i] = setup.Candidate{Path: path, Root: root.Name}
	}
	m := model{
		step: 6, width: 54, height: 14, config: config.DefaultConfig(),
		path: "/home/george/.config/rog/config.yml", suggestions: roots,
		result: setup.DiscoveryResult{Candidates: candidates},
	}
	first := ansi.Strip(m.View())
	if lines := strings.Split(first, "\n"); len(lines) > m.height {
		t.Fatalf("preview exceeds terminal height: %d > %d\n%s", len(lines), m.height, first)
	}
	if !strings.Contains(first, "Press y to save this configuration") || !strings.Contains(first, "more below") {
		t.Fatalf("preview should keep its action visible and signal more content:\n%s", first)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	second := ansi.Strip(m.View())
	if !strings.Contains(second, "more above") || !strings.Contains(second, "Press y to save this configuration") {
		t.Fatalf("preview scrolling should preserve its action and show scroll position:\n%s", second)
	}
	for _, step := range []string{"CONFIGURED ROOTS", "COVERAGE", "EXCLUDED DIRECTORY NAMES", "REPORT AUTHOR EMAILS", "SAVE DETAILS"} {
		m.height = 80
		m.scroll = 0
		if view := ansi.Strip(m.View()); !strings.Contains(view, step) {
			t.Fatalf("preview missing %q section:\n%s", step, view)
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
	t.Setenv("TERM", "")
	t.Setenv("WT_SESSION", "")
	t.Setenv("ANSICON", "")
	t.Setenv("ConEmuANSI", "")
	if supportsColor() {
		t.Fatal("color should be disabled when the terminal has no color capability")
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

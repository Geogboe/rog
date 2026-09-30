// Package setupui implements rog's guided configuration wizard.
package setupui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"gopkg.in/yaml.v3"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/setup"
)

var ErrCancelled = errors.New("setup cancelled")
var ErrTerminal = errors.New("rog setup needs an interactive terminal; use rog init for a starter config")

type DiscoverFunc func(context.Context, []setup.SearchRoot, []string, func(string, string, int, int)) setup.DiscoveryResult
type Result struct {
	Config    *config.Config
	ScanNow   bool
	Cancelled bool
}
type IndexSummary struct {
	Count        int
	UpdatedAt    time.Time
	BridgeStatus string
}
type discoveryDone struct {
	result      setup.DiscoveryResult
	suggestions []setup.RootSuggestion
	emails      []string
}
type progressTick time.Time
type setupContextDone struct{}
type progressState struct {
	mu   sync.Mutex
	text string
}

func nextProgress() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg { return progressTick(t) })
}

func watchContext(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		<-ctx.Done()
		return setupContextDone{}
	}
}

type applyMessage struct{ model model }

type model struct {
	ctx                 context.Context
	config              *config.Config
	path                string
	local, external     []setup.SearchRoot
	selected            map[string]bool
	excludes            []string
	selectedExcludes    map[string]bool
	discover            DiscoverFunc
	step, cursor, width int
	busy                bool
	result              setup.DiscoveryResult
	suggestions         []setup.RootSuggestion
	progress            *progressState
	emails              []string
	selectedEmails      map[string]bool
	emailInput          string
	addingEmail         bool
	excludeInput        string
	addingExclude       bool
	status              string
	indexSummary        IndexSummary
	addingLocation      bool
	locationInput       string
	previewAt           time.Time
	finished            bool
	out                 Result
}

func Run(ctx context.Context, path string, cfg *config.Config, local, external []setup.SearchRoot, summary IndexSummary, discover DiscoverFunc) (Result, error) {
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stderr.Fd()) {
		return Result{}, ErrTerminal
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stdin, stderr := os.Stdin, os.Stderr
	if runtime.GOOS != "windows" {
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			return Result{}, ErrTerminal
		}
		stdin, stderr = tty, tty
		defer tty.Close()
	}
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	m := model{ctx: ctx, config: cloneConfig(cfg), path: path, local: local, external: external, indexSummary: summary, discover: discover, selected: map[string]bool{}, selectedExcludes: map[string]bool{}, selectedEmails: map[string]bool{}, width: 80}
	m.excludes = append([]string(nil), cfg.GlobalExcludes...)
	for _, recommended := range setup.RecommendedExcludes() {
		present := false
		for _, current := range m.excludes {
			if strings.EqualFold(current, recommended) {
				present = true
				break
			}
		}
		if !present {
			m.excludes = append(m.excludes, recommended)
		}
	}
	for _, exclude := range m.excludes {
		m.selectedExcludes[exclude] = true
	}
	for _, r := range external {
		for _, old := range cfg.Roots {
			if old.Name == r.Name && old.Path == r.Path && old.WSL == r.WSL && old.Windows == r.Windows && old.WSLDistro == r.Distro {
				m.selected[searchRootKey(r)] = true
			}
		}
	}
	for _, r := range local {
		m.selected[searchRootKey(r)] = true
	}
	if cfg.Report != nil {
		for _, email := range cfg.Report.AuthorEmails {
			m.selectedEmails[email] = true
		}
	}
	final, err := tea.NewProgram(m, tea.WithInput(stdin), tea.WithOutput(stderr), tea.WithAltScreen()).Run()
	if err != nil {
		return Result{}, err
	}
	done := final.(model)
	if done.out.Cancelled {
		return done.out, ErrCancelled
	}
	if !done.finished {
		return Result{}, ErrCancelled
	}
	return done.out, nil
}

type rollbackModel struct {
	ctx       context.Context
	path      string
	revisions []string
	selected  int
	width     int
	preview   bool
	finished  bool
	saved     bool
	scan      bool
	status    string
	previewAt time.Time
}

// RunRollback previews a saved config before restoring it transactionally.
func RunRollback(ctx context.Context, path string, revisions []string) (bool, bool, error) {
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stderr.Fd()) {
		return false, false, ErrTerminal
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stdin, stderr := os.Stdin, os.Stderr
	if runtime.GOOS != "windows" {
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			return false, false, ErrTerminal
		}
		stdin, stderr = tty, tty
		defer tty.Close()
	}
	m := rollbackModel{ctx: ctx, path: path, revisions: revisions, width: 80}
	final, err := tea.NewProgram(m, tea.WithInput(stdin), tea.WithOutput(stderr), tea.WithAltScreen()).Run()
	if err != nil {
		return false, false, err
	}
	done := final.(rollbackModel)
	return done.scan, done.saved, nil
}

func (m rollbackModel) Init() tea.Cmd { return watchContext(m.ctx) }
func (m rollbackModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(setupContextDone); ok {
		m.finished = true
		return m, tea.Quit
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		if m.saved {
			switch key.String() {
			case "s", "S":
				m.scan = true
				return m, tea.Quit
			case "r", "R", "q", "esc", "ctrl+c":
				return m, tea.Quit
			default:
				return m, nil
			}
		}
		switch key.String() {
		case "ctrl+c", "q", "esc":
			m.finished = true
			return m, tea.Quit
		case "up", "k":
			if !m.preview && m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if !m.preview && m.selected+1 < len(m.revisions) {
				m.selected++
			}
		case "enter":
			if len(m.revisions) > 0 {
				m.preview = true
				m.previewAt = time.Now()
			}
		case "b", "left":
			m.preview = false
		case "y", "Y":
			if m.preview && len(m.revisions) > 0 {
				at := m.previewAt
				if at.IsZero() {
					at = time.Now()
				}
				_, err := setup.RestoreConfig(m.path, m.revisions[m.selected], at)
				if err != nil {
					m.status = "Restore failed: " + err.Error()
				} else {
					m.status = "Previous configuration restored and current config backed up."
					m.saved = true
					m.preview = false
				}
			}
		case "s", "S":
			if m.saved {
				m.scan = true
				return m, tea.Quit
			}
		case "r", "R":
			if m.saved {
				return m, tea.Quit
			}
		}
	}
	return m, nil
}
func (m rollbackModel) View() string {
	var b strings.Builder
	b.WriteString("ROG SETUP ROLLBACK\n\n")
	if m.saved {
		fmt.Fprintf(&b, "%s\n\nPress s to scan now, or r to finish and scan later.\n", m.status)
	} else if m.preview && len(m.revisions) > 0 {
		revision := m.revisions[m.selected]
		fmt.Fprintf(&b, "Restore %s?\n\n", revision)
		currentData, currentErr := os.ReadFile(m.path)
		priorData, priorErr := os.ReadFile(filepath.Join(filepath.Dir(m.path), "setup-history", revision))
		var current, prior config.Config
		if currentErr == nil && priorErr == nil && yaml.Unmarshal(currentData, &current) == nil && yaml.Unmarshal(priorData, &prior) == nil {
			fmt.Fprintf(&b, "Current Configured Roots (%d):\n", len(current.Roots))
			writeRootSummary(&b, current.Roots)
			fmt.Fprintf(&b, "Restored Configured Roots (%d):\n", len(prior.Roots))
			writeRootSummary(&b, prior.Roots)
			var currentEmails, priorEmails []string
			if current.Report != nil {
				currentEmails = current.Report.AuthorEmails
			}
			if prior.Report != nil {
				priorEmails = prior.Report.AuthorEmails
			}
			fmt.Fprintf(&b, "\nExclusions: %s → %s\nReport author emails: %s → %s\n", strings.Join(current.GlobalExcludes, ", "), strings.Join(prior.GlobalExcludes, ", "), strings.Join(currentEmails, ", "), strings.Join(priorEmails, ", "))
			fmt.Fprintf(&b, "Editor: %s → %s\n", current.Editor, prior.Editor)
			b.WriteString("Other saved YAML settings will also be restored. LLM key values are hidden.\n")
		} else {
			b.WriteString("Could not parse the current config or selected revision for preview.\n")
		}
		at := m.previewAt
		if at.IsZero() {
			at = time.Now()
		}
		backup := filepath.Join(filepath.Dir(m.path), "setup-history", at.Local().Format("20060102-150405.000000000")+".yml")
		fmt.Fprintf(&b, "Current config backup: %s\n\ny applies it. b returns to revisions.\n", backup)
	} else {
		start, end := listWindow(len(m.revisions), m.selected, 12)
		if start > 0 {
			fmt.Fprintf(&b, "Showing revisions %d-%d of %d\n", start+1, end, len(m.revisions))
		}
		for i := start; i < end; i++ {
			r := m.revisions[i]
			cursor := " "
			if i == m.selected {
				cursor = ">"
			}
			fmt.Fprintf(&b, "%s %s\n", cursor, r)
		}
		b.WriteString("\nUp/Down choose a revision; Enter previews it.\n")
	}
	if m.status != "" {
		fmt.Fprintf(&b, "\n%s\n", m.status)
	}
	if !m.saved {
		b.WriteString("\nCtrl+C cancels")
	}
	return fitTerminal(b.String(), m.width)
}

func (m model) Init() tea.Cmd { return watchContext(m.ctx) }
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case setupContextDone:
		m.out.Cancelled = true
		m.finished = true
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width = x.Width
	case progressTick:
		if m.busy && m.progress != nil {
			m.progress.mu.Lock()
			m.status = m.progress.text
			m.progress.mu.Unlock()
			return m, nextProgress()
		}
	case discoveryDone:
		m.busy = false
		m.result = x.result
		m.suggestions = x.suggestions
		m.emails = x.emails
		if m.config.Report != nil {
			for _, email := range m.config.Report.AuthorEmails {
				if !contains(m.emails, email) {
					m.emails = append(m.emails, email)
				}
			}
			sort.Strings(m.emails)
		}
		m.cursor = 0
		m.status = fmt.Sprintf("Found %d valid repositories; %d invalid Git markers were rejected.", len(m.result.Candidates), m.result.Rejected)
	case applyMessage:
		return x.model, nil
	case tea.KeyMsg:
		key := x.String()
		if m.step == 7 {
			switch key {
			case "s", "S":
				m.out.ScanNow = true
				m.finished = true
				return m, tea.Quit
			case "r", "R", "q", "esc", "ctrl+c":
				m.finished = true
				return m, tea.Quit
			default:
				return m, nil
			}
		}
		if key == "ctrl+c" {
			m.out.Cancelled = true
			m.finished = true
			return m, tea.Quit
		}
		if m.addingLocation {
			switch key {
			case "esc":
				m.addingLocation = false
				m.locationInput = ""
			case "enter":
				path := strings.TrimSpace(m.locationInput)
				if filepath.IsAbs(path) {
					root := setup.SearchRoot{Name: filepath.Base(path), Path: path}
					m.local = append(m.local, root)
					m.selected[searchRootKey(root)] = true
				}
				m.addingLocation = false
				m.locationInput = ""
			case "backspace":
			default:
				if x.Type == tea.KeyRunes {
					m.locationInput += string(x.Runes)
				}
			}
			if key == "backspace" {
				m.locationInput = dropLastRune(m.locationInput)
			}
			return m, nil
		}
		if m.addingExclude {
			switch key {
			case "esc":
				m.addingExclude = false
				m.excludeInput = ""
			case "enter":
				value := strings.TrimSpace(m.excludeInput)
				if value != "" && !contains(m.excludes, value) {
					m.excludes = append(m.excludes, value)
					m.selectedExcludes[value] = true
				}
				m.addingExclude = false
				m.excludeInput = ""
			case "backspace":
				m.excludeInput = dropLastRune(m.excludeInput)
			default:
				if x.Type == tea.KeyRunes {
					m.excludeInput += string(x.Runes)
				}
			}
			return m, nil
		}
		if m.addingEmail {
			switch key {
			case "esc":
				m.addingEmail = false
				m.emailInput = ""
			case "enter":
				v := strings.TrimSpace(m.emailInput)
				if v != "" && !contains(m.emails, v) {
					m.emails = append(m.emails, v)
					sort.Strings(m.emails)
					m.selectedEmails[v] = true
				}
				m.addingEmail = false
				m.emailInput = ""
			case "backspace":
			default:
				if x.Type == tea.KeyRunes {
					m.emailInput += string(x.Runes)
				}
			}
			if key == "backspace" {
				m.emailInput = dropLastRune(m.emailInput)
			}
			return m, nil
		}
		switch key {
		case "q", "esc":
			m.out.Cancelled = true
			m.finished = true
			return m, tea.Quit
		case "b", "left", "shift+tab":
			if m.step > 0 && !m.busy {
				m.step--
				m.cursor = 0
			}
		case "enter", "right", "tab":
			if m.busy {
				return m, nil
			}
			if m.step == 2 {
				m.step = 3
				m.busy = true
				m.progress = &progressState{text: "Searching selected filesystems…"}
				return m, m.startDiscovery()
			}
			if m.step == 5 {
				m.previewAt = time.Now()
			}
			if m.step < 6 {
				m.step++
				m.cursor = 0
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			m.cursor++
		case " ":
			if m.step == 1 {
				if m.cursor < len(m.local) {
					r := m.local[m.cursor]
					key := searchRootKey(r)
					m.selected[key] = !m.selected[key]
				} else {
					idx := m.cursor - len(m.local)
					if idx >= 0 && idx < len(m.external) {
						r := m.external[idx]
						key := searchRootKey(r)
						m.selected[key] = !m.selected[key]
					}
				}
			}
			if m.step == 2 && m.cursor < len(m.excludes) {
				value := m.excludes[m.cursor]
				m.selectedExcludes[value] = !m.selectedExcludes[value]
			}
			if m.step == 4 && m.cursor < len(m.suggestions) {
				m.suggestions[m.cursor].Selected = !m.suggestions[m.cursor].Selected
			}
			if m.step == 5 && m.cursor < len(m.emails) {
				email := m.emails[m.cursor]
				m.selectedEmails[email] = !m.selectedEmails[email]
			}
		case "+", "=":
			if m.step == 4 && m.cursor < len(m.suggestions) {
				m.suggestions[m.cursor].Root.MaxDepth++
			}
		case "-":
			if m.step == 4 && m.cursor < len(m.suggestions) && m.suggestions[m.cursor].Root.MaxDepth > 1 {
				m.suggestions[m.cursor].Root.MaxDepth--
			}
		case "e":
			if m.step == 2 {
				m.addingExclude = true
				m.excludeInput = ""
			} else if m.step == 5 {
				m.addingEmail = true
				m.emailInput = ""
			}
		case "a":
			if m.step == 1 {
				m.addingLocation = true
				m.locationInput = ""
			}
		case "d", "delete", "backspace":
			if m.step == 2 && m.cursor < len(m.excludes) {
				value := m.excludes[m.cursor]
				m.excludes = append(m.excludes[:m.cursor], m.excludes[m.cursor+1:]...)
				delete(m.selectedExcludes, value)
				if m.cursor >= len(m.excludes) && m.cursor > 0 {
					m.cursor--
				}
			}
		case "y", "Y":
			if m.step == 6 {
				return m, m.apply()
			}
		}
	}
	return m, nil
}

func (m model) startDiscovery() tea.Cmd {
	var roots []setup.SearchRoot
	for _, r := range m.local {
		if m.selected[searchRootKey(r)] {
			roots = append(roots, r)
		}
	}
	for _, r := range m.external {
		if m.selected[searchRootKey(r)] {
			roots = append(roots, r)
		}
	}
	discover := m.discover
	ctx := m.ctx
	excludes := make([]string, 0, len(m.excludes))
	for _, exclude := range m.excludes {
		if m.selectedExcludes[exclude] {
			excludes = append(excludes, exclude)
		}
	}
	existing := append([]config.Root(nil), m.config.Roots...)
	progress := m.progress
	return tea.Batch(nextProgress(), func() tea.Msg {
		result := discover(ctx, roots, excludes, func(root, name string, done, total int) {
			progress.mu.Lock()
			if total == 0 {
				progress.text = fmt.Sprintf("%s · %s", root, name)
			} else {
				progress.text = fmt.Sprintf("Validating %s · %s · %d/%d Git markers", root, name, done, total)
			}
			progress.mu.Unlock()
		})
		if len(roots) == 0 {
			result.Warnings = append(result.Warnings, "No search locations were selected; repository discovery was skipped.")
		}
		emails := observedEmails(result.Candidates)
		return discoveryDone{result: result, suggestions: setup.Suggestions(result.Candidates, existing), emails: emails}
	})
}

func (m model) apply() tea.Cmd {
	return func() tea.Msg {
		var roots []config.Root
		for _, s := range m.suggestions {
			if s.Selected {
				roots = append(roots, s.Root)
			}
		}
		if len(roots) == 0 {
			m.status = "Select at least one root before applying."
			return applyMessage{m}
		}
		next := cloneConfig(m.config)
		next.Roots = roots
		next.GlobalExcludes = make([]string, 0, len(m.excludes))
		for _, exclude := range m.excludes {
			if m.selectedExcludes[exclude] {
				next.GlobalExcludes = append(next.GlobalExcludes, exclude)
			}
		}
		if err := setup.ValidateSetupConfig(next); err != nil {
			m.status = "Setup was not applied: " + err.Error()
			return applyMessage{m}
		}
		var selected []string
		for _, email := range m.emails {
			if m.selectedEmails[email] {
				selected = append(selected, email)
			}
		}
		if len(m.selectedEmails) > 0 || next.Report != nil {
			if next.Report == nil {
				next.Report = &config.ReportConfig{}
			}
			next.Report.AuthorEmails = selected
		}
		original, err := os.ReadFile(m.path)
		if os.IsNotExist(err) {
			original = nil
		} else if err != nil {
			m.status = err.Error()
			return applyMessage{m}
		}
		data, err := setup.PreviewConfig(original, next)
		if err == nil {
			appliedAt := m.previewAt
			if appliedAt.IsZero() {
				appliedAt = time.Now()
			}
			_, err = setup.ApplyConfig(m.path, data, appliedAt)
		}
		if err != nil {
			m.status = "Setup was not applied: " + err.Error()
			return applyMessage{m}
		}
		m.config = next
		m.step = 7
		m.status = "Configuration saved. The index is separate; you can cancel the initial scan independently."
		return applyMessage{m}
	}
}

func (m model) View() string {
	labels := []string{"Review", "Environments", "Exclusions", "Discovery", "Roots & depth", "Report identity", "Preview & apply", "Saved"}
	var b strings.Builder
	fmt.Fprintf(&b, "ROG SETUP   %s   %d/%d\n\n", strings.ToUpper(labels[min(m.step, len(labels)-1)]), m.step+1, len(labels))
	switch m.step {
	case 0:
		fmt.Fprintf(&b, "Config: %s\nConfigured Roots: %d\n", m.path, len(m.config.Roots))
		indexDate := "never"
		if !m.indexSummary.UpdatedAt.IsZero() {
			indexDate = m.indexSummary.UpdatedAt.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(&b, "Index: %d repositories · updated %s\nBridge: %s\n", m.indexSummary.Count, indexDate, m.indexSummary.BridgeStatus)
		for _, r := range m.config.Roots {
			fmt.Fprintf(&b, "  • %s  %s  depth %d\n", r.Name, r.Path, r.MaxDepth)
		}
		fmt.Fprintf(&b, "Exclusions: %s\n", strings.Join(m.excludes, ", "))
		b.WriteString("\nSetup searches for Git repositories, suggests roots and depths, and checks report author matching. Nothing changes until the final Apply step.\nPress Enter to review environments.\n")
	case 1:
		b.WriteString("The local filesystem roots below are searched by default. Other operating systems use their rog worker; setup does not walk a mounted share.\n\n")
		totalRoots := len(m.local) + len(m.external)
		start, end := listWindow(totalRoots, m.cursor, 12)
		if start > 0 || end < totalRoots {
			fmt.Fprintf(&b, "Showing locations %d-%d of %d\n", start+1, end, totalRoots)
		}
		for i := start; i < end; i++ {
			var r setup.SearchRoot
			if i < len(m.local) {
				r = m.local[i]
			} else {
				r = m.external[i-len(m.local)]
			}
			mark := " "
			if m.selected[searchRootKey(r)] {
				mark = "✓"
			}
			cursor := " "
			if m.cursor == i {
				cursor = ">"
			}
			fmt.Fprintf(&b, "%s [%s] %s  %s", cursor, mark, r.Name, r.Path)
			if r.Distro != "" {
				fmt.Fprintf(&b, "  (starts %s for discovery)", r.Distro)
			}
			b.WriteByte('\n')
		}
		if m.addingLocation {
			fmt.Fprintf(&b, "Additional local search path: %s_\n", m.locationInput)
		}
		b.WriteString("\nSpace includes or skips another OS; a adds a local search path. Enter reviews exclusions.\n")
	case 2:
		b.WriteString("Directory names here are skipped during repository discovery. Space toggles an item, d removes it, and e adds one. Built-in system and cache exclusions still apply.\n\n")
		start, end := listWindow(len(m.excludes), m.cursor, 12)
		if start > 0 || end < len(m.excludes) {
			fmt.Fprintf(&b, "Showing exclusions %d-%d of %d\n", start+1, end, len(m.excludes))
		}
		for i := start; i < end; i++ {
			exclude := m.excludes[i]
			mark, cursor := " ", " "
			if m.selectedExcludes[exclude] {
				mark = "✓"
			}
			if i == m.cursor {
				cursor = ">"
			}
			fmt.Fprintf(&b, "%s [%s] %s\n", cursor, mark, exclude)
		}
		if m.addingExclude {
			fmt.Fprintf(&b, "Add excluded directory name: %s_\n", m.excludeInput)
		}
		b.WriteString("\nEnter begins discovery; b returns to environments.\n")
	case 3:
		if m.busy {
			b.WriteString("Discovering repositories with bounded workers…\n")
			fmt.Fprintf(&b, "%s\n", m.status)
		} else {
			fmt.Fprintf(&b, "%s\n%d excluded directories · %d overlapping markers\n", m.status, m.result.ExcludedDirectories, m.result.Overlaps)
			for _, w := range m.result.Warnings {
				fmt.Fprintf(&b, "  ! %s\n", w)
			}
			b.WriteString("\nEnter continues; b returns to environment choices.\n")
		}
	case 4:
		b.WriteString("A depth of 2 reaches one folder below a root; depth 4 reaches three folders below. +/- changes the selected depth.\n\n")
		start, end := listWindow(len(m.suggestions), m.cursor, 12)
		if start > 0 || end < len(m.suggestions) {
			fmt.Fprintf(&b, "Showing roots %d-%d of %d\n", start+1, end, len(m.suggestions))
		}
		for i := start; i < end; i++ {
			s := m.suggestions[i]
			mark := " "
			if s.Selected {
				mark = "✓"
			}
			cursor := " "
			if i == m.cursor {
				cursor = ">"
			}
			coverage := setup.CountCoveredCandidates(m.result.Candidates, []config.Root{s.Root}, m.selectedExcludesList())
			nested := ""
			if parent := setup.NestedWithin(s.Root, m.suggestions); parent != "" {
				nested = " · nested under " + parent
			}
			fmt.Fprintf(&b, "%s [%s] %-14s depth %-2d · %d in group · covers %d%s · %s\n", cursor, mark, s.Root.Name, s.Root.MaxDepth, s.Count, coverage, nested, s.Root.Path)
		}
		if len(m.suggestions) == 0 {
			b.WriteString("No valid discoveries. Existing roots are shown only if their filesystem was searched.\n")
		}
		covered := setup.CountCoveredCandidates(m.result.Candidates, selectedRoots(m.suggestions), m.selectedExcludesList())
		fmt.Fprintf(&b, "Selected roots cover %d of %d valid discoveries.\n", covered, len(m.result.Candidates))
		b.WriteString("\nSpace toggles a root. Enter continues.\n")
	case 5:
		b.WriteString("Reports match commits by Git author email. Suggestions come from repository-local config and a five-commit author sample; they are not selected automatically.\n\n")
		start, end := listWindow(len(m.emails), m.cursor, 12)
		if start > 0 || end < len(m.emails) {
			fmt.Fprintf(&b, "Showing identities %d-%d of %d\n", start+1, end, len(m.emails))
		}
		for i := start; i < end; i++ {
			email := m.emails[i]
			mark := " "
			if m.selectedEmails[email] {
				mark = "✓"
			}
			cursor := " "
			if i == m.cursor {
				cursor = ">"
			}
			fmt.Fprintf(&b, "%s [%s] %s\n", cursor, mark, email)
		}
		if m.addingEmail {
			fmt.Fprintf(&b, "Add email: %s_\n", m.emailInput)
		} else {
			b.WriteString("Space selects; e adds an email; Enter previews.\n")
		}
	case 6:
		b.WriteString("Proposed configuration\n\n")
		var selectedExcludes []string
		for _, exclude := range m.excludes {
			if m.selectedExcludes[exclude] {
				selectedExcludes = append(selectedExcludes, exclude)
			}
		}
		fmt.Fprintf(&b, "Directory exclusions: %s → %s\n", strings.Join(m.config.GlobalExcludes, ", "), strings.Join(selectedExcludes, ", "))
		proposedRoots := selectedRoots(m.suggestions)
		covered := setup.CountCoveredCandidates(m.result.Candidates, proposedRoots, selectedExcludes)
		fmt.Fprintf(&b, "Coverage estimate: %d valid repositories discovered; %d covered by selected Configured Roots.\n", len(m.result.Candidates), covered)
		uncovered := len(m.result.Candidates) - covered
		if uncovered > 0 {
			fmt.Fprintf(&b, "  ! %d valid discoveries are outside selected roots, beyond their depth limits, or excluded. Increase a root depth, include a nested root, or revise exclusions.\n", uncovered)
		}
		for _, current := range m.config.Roots {
			if !containsConfigRoot(proposedRoots, current) {
				fmt.Fprintf(&b, "  − %s  depth %d · %s\n", current.Name, current.MaxDepth, current.Path)
			}
		}
		for _, s := range m.suggestions {
			if s.Selected {
				fmt.Fprintf(&b, "  • %s  depth %d · %d discovered · %s\n", s.Root.Name, s.Root.MaxDepth, s.Count, s.Root.Path)
			}
		}
		if len(m.selectedEmails) > 0 || (m.config.Report != nil && len(m.config.Report.AuthorEmails) > 0) {
			var picked []string
			for _, e := range m.emails {
				if m.selectedEmails[e] {
					picked = append(picked, e)
				}
			}
			var oldEmails []string
			if m.config.Report != nil {
				oldEmails = m.config.Report.AuthorEmails
			}
			fmt.Fprintf(&b, "Report author emails: %s → %s\n", strings.Join(oldEmails, ", "), strings.Join(picked, ", "))
		}
		backup := "No current config file; no backup will be created."
		if _, err := os.Stat(m.path); err == nil {
			at := m.previewAt
			if at.IsZero() {
				at = time.Now()
			}
			backup = filepath.Join(filepath.Dir(m.path), "setup-history", at.Local().Format("20060102-150405.000000000")+".yml")
		}
		fmt.Fprintf(&b, "\nConfig: %s\nProposed backup: %s (mode 0600)\nHistory keeps five revisions. Other config keys are preserved.\n", m.path, backup)
		warningCount := len(m.result.Warnings)
		if uncovered > 0 {
			warningCount++
		}
		if warningCount > 0 {
			fmt.Fprintf(&b, "\nCoverage warnings (%d):\n", warningCount)
			if uncovered > 0 {
				fmt.Fprintf(&b, "  ! %d valid discoveries are not covered by the proposed Configured Roots.\n", uncovered)
			}
			for _, warning := range m.result.Warnings {
				fmt.Fprintf(&b, "  ! %s\n", warning)
			}
		}
		b.WriteString("Press y to apply; b to edit.\n")
	case 7:
		fmt.Fprintf(&b, "%s\n\nPress s to run rog scan now, or r to finish and run it later.\n", m.status)
	}
	if m.step != 7 {
		b.WriteString("\nTab/Enter next · b back · Ctrl+C cancel")
	}
	return fitTerminal(b.String(), m.width)
}

func fitTerminal(value string, width int) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		line = cleanDisplay(line)
		if width > 0 {
			line = ansi.Truncate(line, width, "…")
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func listWindow(total, cursor, limit int) (int, int) {
	if total <= limit {
		return 0, total
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= total {
		cursor = total - 1
	}
	start := cursor - limit/2
	if start < 0 {
		start = 0
	}
	if start+limit > total {
		start = total - limit
	}
	return start, start + limit
}

func cleanDisplay(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return -1
		}
		return r
	}, value)
}

func writeRootSummary(b *strings.Builder, roots []config.Root) {
	for _, root := range roots {
		depth := root.MaxDepth
		if depth == 0 {
			depth = 4
		}
		fmt.Fprintf(b, "  • %s  depth %d  %s", root.Name, depth, root.Path)
		if root.WSL {
			fmt.Fprintf(b, "  (WSL %s)", root.WSLDistro)
		} else if root.Windows {
			b.WriteString("  (Windows)")
		}
		b.WriteByte('\n')
	}
}

func cloneConfig(c *config.Config) *config.Config {
	n := *c
	n.Roots = append([]config.Root(nil), c.Roots...)
	n.GlobalExcludes = append([]string(nil), c.GlobalExcludes...)
	if c.Report != nil {
		x := *c.Report
		x.AuthorEmails = append([]string(nil), c.Report.AuthorEmails...)
		n.Report = &x
	}
	return &n
}
func observedEmails(candidates []setup.Candidate) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range candidates {
		values := append([]string{c.Email}, c.AuthorEmails...)
		for _, e := range values {
			e = strings.TrimSpace(e)
			if e != "" && !seen[strings.ToLower(e)] {
				seen[strings.ToLower(e)] = true
				out = append(out, e)
			}
		}
	}
	sort.Strings(out)
	return out
}
func contains(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

func searchRootKey(root setup.SearchRoot) string {
	source := "native"
	switch {
	case root.WSL:
		source = "wsl:" + strings.ToLower(root.Distro)
	case root.Windows:
		source = "windows"
	}
	return strings.ToLower(source + "|" + root.Name + "|" + root.Path)
}

func selectedRoots(suggestions []setup.RootSuggestion) []config.Root {
	var roots []config.Root
	for _, suggestion := range suggestions {
		if suggestion.Selected {
			roots = append(roots, suggestion.Root)
		}
	}
	return roots
}

func (m model) selectedExcludesList() []string {
	var excludes []string
	for _, exclude := range m.excludes {
		if m.selectedExcludes[exclude] {
			excludes = append(excludes, exclude)
		}
	}
	return excludes
}

func containsConfigRoot(roots []config.Root, target config.Root) bool {
	for _, root := range roots {
		if root.Name == target.Name && root.Path == target.Path && root.WSL == target.WSL && root.WSLDistro == target.WSLDistro && root.Windows == target.Windows {
			return true
		}
	}
	return false
}

func dropLastRune(s string) string {
	if s == "" {
		return s
	}
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
}

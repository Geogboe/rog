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

var setupQuestions = []string{
	"Ready to map your repositories?",
	"Which locations should rog search?",
	"Which directory names should rog skip?",
	"What did rog find?",
	"Which Configured Roots should rog use?",
	"Which author emails should reports include?",
	"Is this configuration ready to save?",
	"Run the initial scan now?",
}

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
	height              int
	scroll              int
	color               bool
	showReviewDetails   bool
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
	m := model{ctx: ctx, config: cloneConfig(cfg), path: path, local: local, external: external, indexSummary: summary, discover: discover, selected: map[string]bool{}, selectedExcludes: map[string]bool{}, selectedEmails: map[string]bool{}, width: 80, height: 24, color: supportsColor()}
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
	color     bool
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
	m := rollbackModel{ctx: ctx, path: path, revisions: revisions, width: 80, color: supportsColor()}
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
	b.WriteString("ROG SETUP  ·  ROLLBACK  ·  CONFIG HISTORY\n")
	writeRule(&b, m.width)
	b.WriteByte('\n')
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
	return fitTerminal(b.String(), m.width, m.color)
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
		m.height = x.Height
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
		if m.step == 0 && key == "d" {
			m.showReviewDetails = !m.showReviewDetails
			return m, nil
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
				m.scroll = 0
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
				m.scroll = 0
			}
		case "up", "k":
			if m.step == 6 || (m.step == 0 && m.showReviewDetails) || (m.step == 3 && !m.busy) {
				m.scroll = max(0, m.scroll-1)
			} else if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.step == 6 || (m.step == 0 && m.showReviewDetails) || (m.step == 3 && !m.busy) {
				m.scroll++
			} else {
				m.cursor++
			}
		case "pgup":
			if m.step == 6 || (m.step == 0 && m.showReviewDetails) || (m.step == 3 && !m.busy) {
				m.scroll = max(0, m.scroll-5)
			}
		case "pgdown":
			if m.step == 6 || (m.step == 0 && m.showReviewDetails) || (m.step == 3 && !m.busy) {
				m.scroll += 5
			}
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
		m.status = "Configuration saved. The repository index was not changed."
		return applyMessage{m}
	}
}

func (m model) View() string {
	if m.width <= 0 {
		m.width = 80
	}
	labels := []string{"Review", "Environments", "Exclusions", "Discovery", "Roots & depth", "Report identity", "Preview & apply", "Saved"}
	var b strings.Builder
	fmt.Fprintf(&b, "ROG SETUP  ·  %s  ·  %02d/%02d\n", strings.ToUpper(labels[min(m.step, len(labels)-1)]), m.step+1, len(labels))
	writeRule(&b, m.width)
	b.WriteByte('\n')
	writeWrapped(&b, setupQuestions[min(m.step, len(setupQuestions)-1)], m.width)
	b.WriteString("\n\n")
	header := b.String()
	b.Reset()
	switch m.step {
	case 0:
		if m.showReviewDetails {
			b.WriteString("CURRENT CONFIGURATION\n")
			b.WriteString("Config file\n")
			writeIndentedWrapped(&b, "  ", compactPath(m.path, max(1, m.width-2)), m.width)
			fmt.Fprintf(&b, "\nConfigured Roots · %d\n", len(m.config.Roots))
			if len(m.config.Roots) == 0 {
				b.WriteString("  None\n")
			}
			for _, r := range m.config.Roots {
				fmt.Fprintf(&b, "  • %s · depth %d\n", r.Name, r.MaxDepth)
				writeIndentedWrapped(&b, "    ", compactPath(r.Path, max(1, m.width-4)), m.width)
			}
			indexSummary := fmt.Sprintf("Index: %d repositories", m.indexSummary.Count)
			if m.indexSummary.UpdatedAt.IsZero() {
				indexSummary += " · never scanned"
			} else {
				indexSummary += " · updated " + m.indexSummary.UpdatedAt.Format("Jan 2, 2006 at 15:04")
			}
			writeIndentedWrapped(&b, "  ", indexSummary, m.width)
			writeWrapped(&b, "Cross-OS bridge: "+m.indexSummary.BridgeStatus, m.width)
			fmt.Fprintf(&b, "\nExclusions · %d\n", len(m.excludes))
			writeIndentedWrapped(&b, "  ", listOrNone(m.excludes), m.width)
		} else {
			writeWrapped(&b, "Discover repositories, review suggested Configured Roots, then save.", m.width)
			b.WriteString("\nCURRENT INDEX\n")
			rootLabel := "Configured Roots"
			if len(m.config.Roots) == 1 {
				rootLabel = "Configured Root"
			}
			fmt.Fprintf(&b, "  %d repositories\n  %d %s\n", m.indexSummary.Count, len(m.config.Roots), rootLabel)
			if m.indexSummary.UpdatedAt.IsZero() {
				b.WriteString("  Last scan: never\n")
			} else {
				fmt.Fprintf(&b, "  Last scan: %s\n", m.indexSummary.UpdatedAt.Format("Jan 2, 2006 at 15:04"))
			}
			b.WriteByte('\n')
			writeWrapped(&b, "Nothing is saved until you confirm the preview.", m.width)
			b.WriteByte('\n')
		}
	case 1:
		writeWrapped(&b, "Selected WSL distros start during discovery.", m.width)
		b.WriteString("\n\n")
		totalRoots := len(m.local) + len(m.external)
		start, end := listWindow(totalRoots, m.cursor, m.visibleRows(2))
		if (start > 0 || end < totalRoots) && m.visibleRows(2) > 1 {
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
			name := displayRootName(r.Name, r.Windows, r.WSL, r.Distro)
			fmt.Fprintf(&b, "%s [%s] %s\n", cursor, mark, name)
			writeIndentedWrapped(&b, "    ", compactPath(r.Path, max(1, m.width-4)), m.width)
		}
		if m.addingLocation {
			fmt.Fprintf(&b, "Additional local search path: %s_\n", m.locationInput)
		}
	case 2:
		writeWrapped(&b, "Checked names are skipped during discovery. Built-in system and cache exclusions always apply.", m.width)
		b.WriteString("\n\n")
		start, end := listWindow(len(m.excludes), m.cursor, m.visibleRows(1))
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
	case 3:
		if m.busy {
			b.WriteString("Searching selected locations\n")
			writeWrapped(&b, m.status, m.width)
			b.WriteByte('\n')
		} else {
			writeWrapped(&b, m.status, m.width)
			fmt.Fprintf(&b, "\n\n%d directories skipped · %d overlapping Git markers\n", m.result.ExcludedDirectories, m.result.Overlaps)
			for _, w := range m.result.Warnings {
				writeIndentedWrapped(&b, "  ! ", w, m.width)
			}
		}
	case 4:
		writeWrapped(&b, "Depth 4 reaches 3 folders below a root.", m.width)
		b.WriteString("\n\n")
		start, end := listWindow(len(m.suggestions), m.cursor, m.visibleRows(2))
		if (start > 0 || end < len(m.suggestions)) && m.visibleRows(2) > 1 {
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
			fmt.Fprintf(&b, "%s [%s] %s · depth %d\n", cursor, mark, displayRootName(s.Root.Name, s.Root.Windows, s.Root.WSL, s.Root.WSLDistro), s.Root.MaxDepth)
			prefix := fmt.Sprintf("    %d found · %d covered · ", s.Count, coverage)
			pathWidth := max(1, m.width-ansi.StringWidth(prefix))
			writeIndentedWrapped(&b, prefix, compactPath(s.Root.Path, pathWidth), m.width)
		}
		if len(m.suggestions) == 0 {
			b.WriteString("No valid discoveries. Existing roots are shown only if their filesystem was searched.\n")
		}
		covered := setup.CountCoveredCandidates(m.result.Candidates, selectedRoots(m.suggestions), m.selectedExcludesList())
		writeWrapped(&b, fmt.Sprintf("Coverage: %d/%d discoveries covered", covered, len(m.result.Candidates)), m.width)
		b.WriteByte('\n')
	case 5:
		writeWrapped(&b, "Reports match commits by Git author email.", m.width)
		b.WriteByte('\n')
		writeWrapped(&b, "No selection uses each repo's Git email.", m.width)
		b.WriteString("\n\n")
		start, end := listWindow(len(m.emails), m.cursor, m.visibleRows(1))
		if (start > 0 || end < len(m.emails)) && m.visibleRows(1) > 1 {
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
		} else if len(m.emails) == 0 {
			b.WriteString("No email suggestions found.\n")
		}
	case 6:
		var selectedExcludes []string
		for _, exclude := range m.excludes {
			if m.selectedExcludes[exclude] {
				selectedExcludes = append(selectedExcludes, exclude)
			}
		}
		proposedRoots := selectedRoots(m.suggestions)
		covered := setup.CountCoveredCandidates(m.result.Candidates, proposedRoots, selectedExcludes)
		b.WriteString("PROPOSED CHANGES\n\nCONFIGURED ROOTS\n  Before\n")
		writeRootPreview(&b, m.config.Roots, m.width)
		b.WriteString("  After\n")
		writeRootPreview(&b, proposedRoots, m.width)
		fmt.Fprintf(&b, "\nCOVERAGE\n  %d of %d discovered repositories covered\n", covered, len(m.result.Candidates))
		uncovered := len(m.result.Candidates) - covered
		if uncovered > 0 {
			writeIndentedWrapped(&b, "  ! ", fmt.Sprintf("%d are outside selected roots, beyond depth limits, or excluded.", uncovered), m.width)
		}
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
		b.WriteString("\nEXCLUDED DIRECTORY NAMES\n")
		writeIndentedWrapped(&b, "  Before: ", listOrNone(m.config.GlobalExcludes), m.width)
		writeIndentedWrapped(&b, "  After:  ", listOrNone(selectedExcludes), m.width)
		b.WriteString("\nREPORT AUTHOR EMAILS\n")
		writeIndentedWrapped(&b, "  Before: ", reportEmailValue(oldEmails), m.width)
		writeIndentedWrapped(&b, "  After:  ", reportEmailValue(picked), m.width)
		backup := "No current config file; no backup will be created."
		if _, err := os.Stat(m.path); err == nil {
			at := m.previewAt
			if at.IsZero() {
				at = time.Now()
			}
			backup = filepath.Join(filepath.Dir(m.path), "setup-history", at.Local().Format("20060102-150405.000000000")+".yml")
		}
		b.WriteString("\nSAVE DETAILS\n  Config file\n")
		writeIndentedWrapped(&b, "    ", compactPath(m.path, max(1, m.width-4)), m.width)
		b.WriteString("  Backup · mode 0600\n")
		writeIndentedWrapped(&b, "    ", compactPath(backup, max(1, m.width-4)), m.width)
		writeIndentedWrapped(&b, "  ", "Keeps five revisions; other YAML keys are preserved.", m.width)
		warningCount := len(m.result.Warnings)
		if uncovered > 0 {
			warningCount++
		}
		if warningCount > 0 {
			fmt.Fprintf(&b, "\nWARNINGS · %d\n", warningCount)
			if uncovered > 0 {
				writeIndentedWrapped(&b, "  ! ", fmt.Sprintf("%d valid discoveries are not covered by the proposed Configured Roots.", uncovered), m.width)
			}
			for _, warning := range m.result.Warnings {
				writeIndentedWrapped(&b, "  ! ", warning, m.width)
			}
		}
		if m.status != "" {
			fmt.Fprintf(&b, "\n%s\n", m.status)
		}
	case 7:
		writeWrapped(&b, m.status, m.width)
		b.WriteByte('\n')
	}
	footer := m.actionBlock(strings.Count(b.String(), "\n")+1 > max(1, m.height-9))
	return fitSetupViewport(header+b.String()+footer, m.width, m.color, m.height, m.scroll)
}

func (m model) actionBlock(previewScroll bool) string {
	var b strings.Builder
	title, primary, hint := "NEXT", "", ""
	switch m.step {
	case 0:
		primary = "Press Enter to choose locations"
		hint = "d view settings · Ctrl+C cancel"
		if m.showReviewDetails {
			hint = "d hide settings · Ctrl+C cancel"
			if previewScroll {
				hint = "d hide settings · ↑/↓ review · Ctrl+C cancel"
			}
		}
	case 1:
		primary, hint = "Press Enter to review exclusions", "↑/↓ move · Space toggle · a add · b back"
		if m.addingLocation {
			primary, hint = "Press Enter to add this location", "Esc cancel entry · Ctrl+C cancel setup"
		}
	case 2:
		primary, hint = "Press Enter to start discovery", "↑/↓ move · Space toggle · e add · d remove"
		if m.addingExclude {
			primary, hint = "Press Enter to add this name", "Esc cancel entry · Ctrl+C cancel setup"
		}
	case 3:
		if m.busy {
			title, primary, hint = "WORKING", "Discovery is running", "Ctrl+C cancel discovery"
		} else {
			primary, hint = "Press Enter to choose roots", "b back to locations · Ctrl+C cancel"
			if previewScroll {
				hint = "↑/↓ review findings · b back · Ctrl+C cancel"
			}
		}
	case 4:
		primary, hint = "Press Enter to review report emails", "↑/↓ move · Space toggle · +/− depth"
	case 5:
		primary, hint = "Press Enter to preview changes", "↑/↓ move · Space select · e add"
		if m.addingEmail {
			primary, hint = "Press Enter to add this email", "Esc cancel entry · Ctrl+C cancel setup"
		}
	case 6:
		primary, hint = "Press y to save this configuration", "b return to report emails · Ctrl+C cancel"
		if previewScroll {
			hint = "↑/↓ review changes · b return to report emails · Ctrl+C cancel"
		}
	case 7:
		primary, hint = "Press s to scan the Configured Roots", "Press r to finish without scanning · q quit"
	}
	fmt.Fprintf(&b, "\n%s\n", title)
	writeIndentedWrapped(&b, "  › ", primary, m.width)
	writeIndentedWrapped(&b, "    ", hint, m.width)
	return strings.TrimSuffix(b.String(), "\n")
}

func (m model) visibleRows(itemLines int) int {
	if itemLines < 1 {
		itemLines = 1
	}
	if m.height <= 0 {
		return 12
	}
	rows := (m.height - 13) / itemLines
	if rows < 1 {
		return 1
	}
	return min(rows, 12)
}

func fitSetupViewport(value string, width int, color bool, height, scroll int) string {
	value = fitTerminal(value, width, color)
	if height <= 0 {
		return value
	}
	lines := strings.Split(value, "\n")
	actionStart := -1
	for i, line := range lines {
		if ansi.Strip(line) == "NEXT" || ansi.Strip(line) == "WORKING" {
			actionStart = i
			break
		}
	}
	if actionStart < 0 {
		if len(lines) <= height {
			return value
		}
		return strings.Join(lines[:height], "\n")
	}
	footerStart := actionStart
	if footerStart > 0 && ansi.Strip(lines[footerStart-1]) == "" {
		footerStart--
	}
	footer := lines[footerStart:]
	questionEnd := min(3, len(lines))
	for questionEnd < len(lines) && ansi.Strip(lines[questionEnd]) != "" {
		questionEnd++
	}
	headerCount := min(len(lines), questionEnd+1)
	if footerStart < headerCount {
		headerCount = footerStart
	}
	header := lines[:headerCount]
	body := lines[headerCount:footerStart]
	available := height - len(header) - len(footer)
	if available <= 0 {
		view := append(append([]string{}, header...), footer...)
		if len(view) > height && len(header) > 0 {
			view = append(append([]string{}, header[:max(1, len(header)-1)]...), footer...)
		}
		return strings.Join(view[:min(height, len(view))], "\n")
	}
	if len(body) <= available {
		return strings.Join(append(append(append([]string{}, header...), body...), footer...), "\n")
	}
	showScrollIndicators := available >= 3
	contentSlots := available
	if showScrollIndicators {
		contentSlots -= 2
	}
	contentSlots = max(1, contentSlots)
	maxScroll := max(0, len(body)-contentSlots)
	start := min(max(scroll, 0), maxScroll)
	end := min(len(body), start+contentSlots)
	viewport := make([]string, 0, len(header)+available+len(footer))
	viewport = append(viewport, header...)
	if showScrollIndicators && start > 0 {
		viewport = append(viewport, "  ↑ more above")
	}
	viewport = append(viewport, body[start:end]...)
	if showScrollIndicators && end < len(body) {
		viewport = append(viewport, "  ↓ more below")
	}
	viewport = append(viewport, footer...)
	return strings.Join(viewport, "\n")
}

func writeIndentedWrapped(b *strings.Builder, prefix, value string, width int) {
	if width <= 0 {
		width = 80
	}
	words := strings.Fields(value)
	if len(words) == 0 {
		b.WriteString(prefix)
		b.WriteByte('\n')
		return
	}
	line := prefix
	for _, word := range words {
		separator := ""
		if line != prefix {
			separator = " "
		}
		if ansi.StringWidth(line)+ansi.StringWidth(separator)+ansi.StringWidth(word) > width && line != prefix {
			b.WriteString(line)
			b.WriteByte('\n')
			line = prefix + word
			continue
		}
		line += separator + word
	}
	b.WriteString(line)
	b.WriteByte('\n')
}

func compactPath(value string, width int) string {
	if width <= 0 || ansi.StringWidth(value) <= width {
		return value
	}
	if width < 5 {
		return ansi.Truncate(value, width, "…")
	}
	leftWidth := (width - 1) / 2
	rightWidth := width - 1 - leftWidth
	var left, right strings.Builder
	used := 0
	for _, r := range value {
		runeWidth := ansi.StringWidth(string(r))
		if used+runeWidth > leftWidth {
			break
		}
		left.WriteRune(r)
		used += runeWidth
	}
	used = 0
	runes := []rune(value)
	for i := len(runes) - 1; i >= 0; i-- {
		runeWidth := ansi.StringWidth(string(runes[i]))
		if used+runeWidth > rightWidth {
			break
		}
		right.WriteRune(runes[i])
		used += runeWidth
	}
	suffix := []rune(right.String())
	for i, j := 0, len(suffix)-1; i < j; i, j = i+1, j-1 {
		suffix[i], suffix[j] = suffix[j], suffix[i]
	}
	return left.String() + "…" + string(suffix)
}

func listOrNone(items []string) string {
	if len(items) == 0 {
		return "None"
	}
	return strings.Join(items, ", ")
}

func reportEmailValue(items []string) string {
	if len(items) == 0 {
		return "Use each repository's Git email"
	}
	return strings.Join(items, ", ")
}

func writeRootPreview(b *strings.Builder, roots []config.Root, width int) {
	if len(roots) == 0 {
		b.WriteString("    None\n")
		return
	}
	for _, root := range roots {
		fmt.Fprintf(b, "    • %s · depth %d\n", displayRootName(root.Name, root.Windows, root.WSL, root.WSLDistro), root.MaxDepth)
		writeIndentedWrapped(b, "      ", compactPath(root.Path, max(1, width-6)), width)
	}
}

func displayRootName(name string, windows, wsl bool, distro string) string {
	if wsl {
		if distro != "" {
			return name + " · WSL " + distro
		}
		return name + " · WSL"
	}
	if windows {
		return name + " · Windows"
	}
	return name
}

func writeWrapped(b *strings.Builder, value string, width int) {
	if width <= 0 {
		width = 80
	}
	var line strings.Builder
	for _, word := range strings.Fields(value) {
		if line.Len() == 0 {
			line.WriteString(word)
			continue
		}
		if ansi.StringWidth(line.String())+1+ansi.StringWidth(word) > width {
			b.WriteString(line.String())
			b.WriteByte('\n')
			line.Reset()
			line.WriteString(word)
			continue
		}
		line.WriteByte(' ')
		line.WriteString(word)
	}
	b.WriteString(line.String())
}

func writeRule(b *strings.Builder, width int) {
	if width <= 0 || width > 60 {
		width = 60
	}
	b.WriteString(strings.Repeat("─", max(1, width)))
	b.WriteByte('\n')
}

func isSetupQuestion(line string) bool {
	for _, question := range setupQuestions {
		if line == question || (len(line) >= 5 && strings.Contains(question, line)) {
			return true
		}
	}
	return false
}

func fitTerminal(value string, width int, color bool) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		line = cleanDisplay(line)
		if width > 0 {
			line = ansi.Truncate(line, width, "…")
		}
		lines[i] = styleTerminalLine(line, color)
	}
	return strings.Join(lines, "\n")
}

func supportsColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	termName := strings.ToLower(os.Getenv("TERM"))
	if runtime.GOOS != "windows" {
		return termName != "" && termName != "dumb"
	}
	return os.Getenv("WT_SESSION") != "" || os.Getenv("ANSICON") != "" ||
		strings.EqualFold(os.Getenv("ConEmuANSI"), "ON") ||
		strings.Contains(termName, "xterm") || strings.Contains(termName, "ansi")
}

func styleTerminalLine(line string, color bool) string {
	if !color || line == "" {
		return line
	}
	if isSetupQuestion(line) {
		return ansiStyle("1;36", line)
	}
	switch {
	case strings.HasPrefix(line, "ROG SETUP  ·  "):
		parts := strings.Split(line, "  ·  ")
		if len(parts) == 3 {
			return ansiStyle("1;36", parts[0]) + "  ·  " + ansiStyle("1", parts[1]) + "  ·  " + ansiStyle("2", parts[2])
		}
	case strings.HasPrefix(line, "─"):
		return ansiStyle("2", line)
	case line == "CURRENT CONFIGURATION":
		return ansiStyle("1;36", line)
	case line == "CURRENT INDEX":
		return ansiStyle("2", line)
	case line == "NEXT":
		return ansiStyle("1;32", line)
	case line == "WORKING":
		return ansiStyle("1;33", line)
	case strings.HasPrefix(line, "  › "):
		return "  " + ansiStyle("32", "›") + " " + ansiStyle("1;32", strings.TrimPrefix(line, "  › "))
	case strings.HasPrefix(line, "    "):
		return ansiStyle("2", line)
	case strings.HasPrefix(line, "Nothing is saved until"):
		return ansiStyle("2", line)
	case line == "PROPOSED CHANGES" || line == "CONFIGURED ROOTS" || line == "COVERAGE" || line == "EXCLUDED DIRECTORY NAMES" || line == "REPORT AUTHOR EMAILS" || line == "SAVE DETAILS" || strings.HasPrefix(line, "WARNINGS ·"):
		return ansiStyle("1;36", line)
	case strings.HasPrefix(line, "  • "):
		item := strings.TrimPrefix(line, "  • ")
		nameEnd := strings.Index(item, "  ")
		if nameEnd > 0 {
			return "  " + ansiStyle("36", "•") + " " + ansiStyle("1;36", item[:nameEnd]) + ansiStyle("2", item[nameEnd:])
		}
	case strings.HasPrefix(line, "> [") || strings.HasPrefix(line, "  ["):
		return styleChoice(line)
	case strings.HasPrefix(line, "  ! ") || strings.HasPrefix(line, "Coverage warnings") || strings.HasPrefix(line, "No valid discoveries"):
		return ansiStyle("33", line)
	case strings.HasPrefix(line, "Found ") || strings.HasPrefix(line, "Configuration saved") || strings.HasPrefix(line, "Previous configuration restored"):
		return ansiStyle("32", line)
	case strings.HasPrefix(line, "Coverage: "):
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			counts := strings.Split(fields[1], "/")
			if len(counts) == 2 && counts[0] == counts[1] {
				return ansiStyle("32", line)
			}
		}
		return ansiStyle("33", line)
	case strings.HasPrefix(line, "Setup was not applied") || strings.HasPrefix(line, "Restore failed") || strings.HasPrefix(line, "Could not parse"):
		return ansiStyle("31", line)
	case line == "Proposed configuration" || strings.HasPrefix(line, "Restore ") && strings.HasSuffix(line, "?"):
		return ansiStyle("1;36", line)
	case strings.HasPrefix(line, "The local filesystem") || strings.HasPrefix(line, "Directory names") || strings.HasPrefix(line, "A depth of ") || strings.HasPrefix(line, "Reports match ") || strings.HasPrefix(line, "Setup searches ") || strings.HasPrefix(line, "Other saved YAML") || strings.HasPrefix(line, "Nothing changes until"):
		return ansiStyle("2", line)
	}
	for _, label := range []string{"Config:", "Configured Roots:", "Index:", "Bridge:", "Exclusions:", "Coverage estimate:", "Report author emails:", "Additional local search path:", "Add excluded directory name:", "Add email:", "Current Configured Roots", "Restored Configured Roots", "Current config backup:"} {
		if strings.HasPrefix(line, label) {
			return ansiStyle("2", label) + line[len(label):]
		}
	}
	return line
}

func styleChoice(line string) string {
	start := strings.Index(line, "[")
	if start < 0 {
		return line
	}
	end := strings.Index(line[start:], "]")
	if end < 0 {
		return line
	}
	end += start
	cursor, marker, rest := line[:start], line[start:end+1], line[end+1:]
	if strings.HasPrefix(cursor, ">") {
		cursor = ansiStyle("1;36", cursor)
	}
	if strings.Contains(marker, "✓") {
		marker = ansiStyle("32", marker)
	} else {
		marker = ansiStyle("2", marker)
	}
	trimmed := strings.TrimLeft(rest, " ")
	spaces := len(rest) - len(trimmed)
	if trimmed != "" {
		nameEnd := strings.IndexAny(trimmed, " \t")
		if nameEnd < 0 {
			nameEnd = len(trimmed)
		}
		if strings.Contains(marker, "✓") {
			rest = strings.Repeat(" ", spaces) + ansiStyle("1;36", trimmed[:nameEnd]) + trimmed[nameEnd:]
		}
	}
	return cursor + marker + rest
}

func ansiStyle(code, value string) string {
	if value == "" {
		return value
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
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
	value = ansi.Strip(value)
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

func dropLastRune(s string) string {
	if s == "" {
		return s
	}
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
}

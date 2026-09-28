// Package reportui provides the terminal presentation of a factual report.
package reportui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"

	"github.com/Geogboe/rog/internal/report"
)

var tabs = []string{"weekly", "dashboard", "log", "ai"}

var ErrCancelled = errors.New("report cancelled")

type generated struct {
	text string
	err  error
}
type model struct {
	doc                        report.Document
	tab, offset, width, height int
	color, confirm, busy       bool
	cancelled                  bool
	status, provider           string
	generate                   func(context.Context) (string, error)
	ctx                        context.Context
}

func (m model) Init() tea.Cmd { return nil }
func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case generated:
		m.busy = false
		if msg.err != nil {
			m.status = "AI failed: " + msg.err.Error()
			m.doc.Warnings = append(m.doc.Warnings, "AI Summary: "+msg.err.Error())
			m.doc.Complete = false
		} else {
			m.doc.AISummary = msg.text
			m.status = "AI Summary generated"
		}
	case tea.KeyMsg:
		key := msg.String()
		if m.confirm {
			m.confirm = false
			if key == "y" || key == "Y" {
				m.busy = true
				m.status = "Generating AI Summary..."
				return m, func() tea.Msg { s, e := m.generate(m.ctx); return generated{s, e} }
			}
			m.status = "AI request cancelled"
			return m, nil
		}
		switch key {
		case "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "q", "esc":
			return m, tea.Quit
		case "tab", "right", "l":
			m.tab = (m.tab + 1) % len(tabs)
			m.offset = 0
		case "shift+tab", "left", "h":
			m.tab = (m.tab + len(tabs) - 1) % len(tabs)
			m.offset = 0
		case "down", "j":
			m.offset++
		case "up", "k":
			if m.offset > 0 {
				m.offset--
			}
		case "pgdown":
			m.offset += max(1, m.height-6)
		case "pgup":
			m.offset = max(0, m.offset-max(1, m.height-6))
		case "g":
			if m.tab == 3 && m.generate != nil && !m.busy {
				m.confirm = true
				m.status = fmt.Sprintf("Send bounded evidence to %s? y/N", m.provider)
			} else if m.tab == 3 && m.generate == nil {
				m.status = m.provider
			}
		}
	}
	return m, nil
}
func (m model) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}
	var b strings.Builder
	title := "ROG REPORT  ·  Configured Roots: " + strings.Join(m.doc.ConfiguredRoots, ", ")
	b.WriteString(ansi.Truncate(title, w, "…"))
	b.WriteByte('\n')
	var tabLine strings.Builder
	for i, t := range tabs {
		label := " " + strings.ToUpper(t) + " "
		if i == m.tab {
			if m.color {
				label = "\x1b[1;36m[" + label + "]\x1b[0m"
			} else {
				label = "[" + label + "]"
			}
		}
		tabLine.WriteString(label)
	}
	b.WriteString(ansi.Truncate(tabLine.String(), w, "…"))
	b.WriteByte('\n')
	indexTime := "never"
	if !m.doc.IndexUpdatedAt.IsZero() {
		indexTime = m.doc.IndexUpdatedAt.In(time.Local).Format("Jan 2 15:04 MST")
	}
	b.WriteString(ansi.Truncate(fmt.Sprintf("%s · %d projects · %s · index %s", m.doc.Since.Format("2006-01-02"), len(m.doc.Projects), map[bool]string{true: "complete", false: "incomplete"}[m.doc.Complete], indexTime), w, "…"))
	b.WriteByte('\n')
	content := m.content(w)
	lines := strings.Split(content, "\n")
	rows := max(1, h-5)
	if m.offset > max(0, len(lines)-rows) {
		m.offset = max(0, len(lines)-rows)
	}
	for i := m.offset; i < len(lines) && i < m.offset+rows; i++ {
		b.WriteString(ansi.Truncate(lines[i], w, "…"))
		b.WriteByte('\n')
	}
	for i := len(lines) - m.offset; i < rows; i++ {
		b.WriteByte('\n')
	}
	status := "Tab/←/→ switch · ↑/↓ scroll · q quit"
	if m.tab == 3 {
		status = "g generate · " + status
	}
	if m.status != "" {
		status = m.status
	}
	b.WriteString(ansi.Truncate(status, w, "…"))
	return b.String()
}

func (m model) content(width int) string {
	var b strings.Builder
	label := func(s string) string {
		if m.color {
			return "\x1b[1;36m" + s + "\x1b[0m"
		}
		return s
	}
	if !m.doc.Complete {
		fmt.Fprintf(&b, "INCOMPLETE  ·  %d warnings; run rog report -o json for details.\n\n", len(m.doc.Warnings))
	}
	switch m.tab {
	case 0:
		fmt.Fprintf(&b, "%s  %s to %s\n\n", label("WEEKLY"), m.doc.Since.In(time.Local).Format("Jan 2"), m.doc.Until.In(time.Local).Format("Jan 2"))
		projects := m.doc.WeeklyProjects()
		if len(projects) == 0 {
			b.WriteString("No matching authored commits in this period.\nRun rog scan after adding repositories.\n")
		}
		for _, p := range projects {
			fmt.Fprintf(&b, "%s  %d commits · %d active days · +%d/−%d lines\n", label(sanitize(p.Name)), p.Metrics.Commits, p.Metrics.ActiveDays, p.Metrics.Additions, p.Metrics.Deletions)
			if !p.Metrics.FirstActivity.IsZero() {
				fmt.Fprintf(&b, "  Activity span: %s–%s (not hours worked)\n", p.Metrics.FirstActivity.In(time.Local).Format("Jan 2"), p.Metrics.LastActivity.In(time.Local).Format("Jan 2"))
			}
			for i, c := range p.Commits {
				if i >= 5 {
					fmt.Fprintf(&b, "  … %d more commits\n", len(p.Commits)-i)
					break
				}
				fmt.Fprintf(&b, "  • %s  %s\n", sanitize(c.Subject), short(c.Hash))
			}
			b.WriteByte('\n')
		}
		current := m.doc.CurrentProjects()
		if len(current) > 0 {
			fmt.Fprintf(&b, "%s  %d repositories have current edits (undated)\n", label("CURRENT CHANGES"), len(current))
			for i, p := range current {
				if i >= 10 {
					fmt.Fprintf(&b, "  … %d more repositories\n", len(current)-i)
					break
				}
				fmt.Fprintf(&b, "  • %s  %d paths\n", sanitize(p.Name), p.Metrics.CurrentChangedPaths)
			}
		}
	case 1:
		fmt.Fprintf(&b, "%s  Top %d of %d active projects\n", label("DASHBOARD"), len(m.doc.DashboardProjects()), m.doc.ActiveCount())
		b.WriteString("Ranked by active days, commits, then changed paths.\nThese numbers do not measure hours worked.\n\n")
		for _, p := range m.doc.DashboardProjects() {
			if width < 72 {
				fmt.Fprintf(&b, "%s\n  %d days · %d commits · %d paths · +%d/−%d lines · %d current\n", label(sanitize(p.Name)), p.Metrics.ActiveDays, p.Metrics.Commits, p.Metrics.ChangedPaths, p.Metrics.Additions, p.Metrics.Deletions, p.Metrics.CurrentChangedPaths)
			} else {
				fmt.Fprintf(&b, "%-28s %3d days  %3d commits  %3d paths  +%d/−%d\n", sanitize(p.Name), p.Metrics.ActiveDays, p.Metrics.Commits, p.Metrics.ChangedPaths, p.Metrics.Additions, p.Metrics.Deletions)
			}
		}
	case 2:
		b.WriteString(label("LOG"))
		b.WriteString("  All matching commits and current changes\n\n")
		for _, p := range m.doc.Projects {
			if len(p.Commits) == 0 && p.Metrics.CurrentChangedPaths == 0 && len(p.Warnings) == 0 {
				continue
			}
			b.WriteString(label(sanitize(p.Name)))
			b.WriteByte('\n')
			for _, c := range p.Commits {
				fmt.Fprintf(&b, "  %s  %s  %s", c.AuthorTime.In(time.Local).Format("Jan 2"), short(c.Hash), sanitize(c.Subject))
				if c.Merge {
					b.WriteString(" [merge]")
				}
				if c.BinaryChanges > 0 {
					fmt.Fprintf(&b, " [%d binary paths]", c.BinaryChanges)
				}
				b.WriteByte('\n')
			}
			for _, wt := range p.Worktrees {
				if len(wt.ChangedPaths) > 0 {
					fmt.Fprintf(&b, "  Current edits: %d paths\n", len(wt.ChangedPaths))
				}
			}
			for _, warning := range p.Warnings {
				fmt.Fprintf(&b, "  Warning: %s\n", sanitize(warning))
			}
			b.WriteByte('\n')
		}
	case 3:
		b.WriteString(label("AI SUMMARY"))
		b.WriteString("\n\n")
		if m.doc.AISummary != "" {
			b.WriteString(sanitize(m.doc.AISummary))
		} else if m.generate == nil {
			b.WriteString(m.provider)
			b.WriteString(" in rog config before generating.")
		} else {
			b.WriteString("Press g to generate using the configured provider.\nNo request is made until you confirm.")
		}
	}
	return b.String()
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' || r >= 0x7f && r <= 0x9f {
			return ' '
		}
		return r
	}, s)
}
func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func Run(ctx context.Context, d report.Document, provider string, generate func(context.Context) (string, error)) (report.Document, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	input, output, err := terminal()
	if err != nil {
		return d, err
	}
	if input != os.Stdin {
		defer input.Close()
	}
	if output != input && output != os.Stderr {
		defer output.Close()
	}
	m := model{doc: d, width: 80, height: 24, provider: provider, generate: generate, ctx: ctx, color: os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"}
	result, err := tea.NewProgram(m, tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen()).Run()
	if err != nil {
		return d, err
	}
	final := result.(model)
	if final.cancelled || ctx.Err() != nil {
		return final.doc, ErrCancelled
	}
	return final.doc, nil
}

func terminal() (*os.File, *os.File, error) {
	if runtime.GOOS == "windows" {
		if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stderr.Fd()) {
			return nil, nil, fmt.Errorf("interactive terminal unavailable")
		}
		return os.Stdin, os.Stderr, nil
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	return tty, tty, nil
}

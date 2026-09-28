// Package picker contains rog's terminal repository selector. CLI wiring lives in cmd.
package picker

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

type Item struct {
	ID, Name, Root, Path, Language, Status, Description string
}

type match struct {
	item  Item
	score int
}

type model struct {
	items                         []Item
	matches                       []match
	query                         []rune
	cursor, offset, width, height int
	chosen                        string
	color                         bool
}

func newModel(items []Item) model {
	m := model{items: items, width: 80, height: 24}
	m.refilter()
	return m
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ensureVisible()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "enter":
			if len(m.matches) > 0 {
				m.chosen = m.matches[m.cursor].item.ID
			}
			return m, tea.Quit
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "ctrl+n":
			if m.cursor+1 < len(m.matches) {
				m.cursor++
			}
		case "pgup":
			m.cursor -= m.rows()
			if m.cursor < 0 {
				m.cursor = 0
			}
		case "pgdown":
			m.cursor += m.rows()
			if m.cursor >= len(m.matches) {
				m.cursor = len(m.matches) - 1
			}
		case "backspace", "ctrl+h":
			if len(m.query) > 0 {
				m.query = m.query[:len(m.query)-1]
				m.refilter()
			}
		case "ctrl+u":
			m.query = nil
			m.refilter()
		default:
			if msg.Type == tea.KeyRunes {
				m.query = append(m.query, msg.Runes...)
				m.refilter()
			}
		}
		m.ensureVisible()
	}
	return m, nil
}

func (m *model) rows() int {
	n := m.height - 8
	if n < 1 {
		n = 1
	}
	if n > 20 {
		n = 20
	}
	return n
}

func (m *model) ensureVisible() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.rows() {
		m.offset = m.cursor - m.rows() + 1
	}
}

func (m *model) refilter() {
	q := string(m.query)
	m.matches = m.matches[:0]
	for _, item := range m.items {
		score, ok := scoreItem(q, item)
		if ok {
			m.matches = append(m.matches, match{item, score})
		}
	}
	sort.SliceStable(m.matches, func(i, j int) bool { return m.matches[i].score > m.matches[j].score })
	m.cursor, m.offset = 0, 0
}

// scoreItem rewards word starts and consecutive matches. Each query word must
// match the repository's name, root, language, or path in order.
func scoreItem(query string, item Item) (int, bool) {
	if strings.TrimSpace(query) == "" {
		return 0, true
	}
	fields := []string{item.Name, item.Root, item.Language, item.Path}
	total := 0
	for _, term := range strings.Fields(query) {
		best := -1
		for i, field := range fields {
			if score, ok := scoreSubsequence(term, field); ok {
				if i == 0 {
					score += 20
				}
				if score > best {
					best = score
				}
			}
		}
		if best < 0 {
			return 0, false
		}
		total += best
	}
	return total, true
}

func scoreSubsequence(needle, haystack string) (int, bool) {
	target := []rune(strings.ToLower(needle))
	text := []rune(haystack)
	if len(target) == 0 {
		return 0, true
	}
	at, last, score := 0, -1, 0
	for i, original := range text {
		if unicode.ToLower(original) != target[at] {
			continue
		}
		score += 10
		if i == 0 || !unicode.IsLetter(text[i-1]) && !unicode.IsDigit(text[i-1]) {
			score += 8
		}
		if last >= 0 {
			if i == last+1 {
				score += 5
			} else {
				score -= i - last - 1
			}
		}
		last = i
		at++
		if at == len(target) {
			return score - i/4, true
		}
	}
	return 0, false
}

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '\u009b' {
			return -1
		}
		return r
	}, s)
}

func clip(s string, width int) string {
	if width < 1 {
		return ""
	}
	return ansi.Truncate(clean(s), width, "…")
}

func (m model) View() string {
	var b strings.Builder
	heading := fmt.Sprintf("Find repository  %d/%d", len(m.matches), len(m.items))
	b.WriteString(m.paint("1;36", clip(heading, m.width)) + "\n")
	fmt.Fprintf(&b, "%s %s\n", m.paint("36", ">"), clip(string(m.query), m.width-3))
	b.WriteString(m.paint("2", strings.Repeat("─", max(1, min(m.width, 80)))) + "\n")
	end := min(len(m.matches), m.offset+m.rows())
	for i := m.offset; i < end; i++ {
		entry := m.matches[i].item
		indicator := "  "
		if i == m.cursor {
			indicator = m.paint("1;36", "❯") + " "
		}
		name := highlight(clean(entry.Name), string(m.query), m.color)
		if i == m.cursor && m.color {
			name = m.paint("1;36", name)
		} else if m.color {
			name = m.paint("36", name)
		}
		status := clean(entry.Status)
		switch status {
		case "dirty":
			status = m.paint("33", status)
		case "status unavailable":
			status = m.paint("31", status)
		}
		label := fmt.Sprintf("%s%s  %s  %s  %s", indicator, name, m.paint("2", "["+clean(entry.Root)+"]"), m.paint("2", clean(entry.Language)), status)
		label = ansi.Truncate(label, m.width, "…")
		b.WriteString(label + "\n")
	}
	b.WriteString(m.paint("2", strings.Repeat("─", max(1, min(m.width, 80)))) + "\n")
	if len(m.matches) > 0 {
		selected := m.matches[m.cursor].item
		b.WriteString(m.paint("36", clip(selected.Path, m.width)) + "\n")
		b.WriteString(m.paint("2", clip(selected.Description, m.width)) + "\n")
	} else {
		b.WriteString(m.paint("33", "No matching repositories") + "\n\n")
	}
	hints := "↑/↓ move  PgUp/PgDn page  Enter select  Esc cancel"
	if m.width < 55 {
		hints = "↑/↓ move  Enter select  Esc cancel"
	}
	if m.width < 35 {
		hints = "Enter select  Esc cancel"
	}
	b.WriteString(m.paint("2", clip(hints, m.width)))
	return b.String()
}

func (m model) paint(code, value string) string {
	if !m.color || value == "" {
		return value
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}

// Run draws on the controlling terminal and returns the selected repository ID.
// An empty ID means the user cancelled.
func Run(items []Item) (string, error) {
	input, output, err := terminal()
	if err != nil {
		return "", err
	}
	if input != os.Stdin {
		defer input.Close()
	}
	if output != input && output != os.Stderr {
		defer output.Close()
	}
	m := newModel(items)
	m.color = supportsColor()
	program := tea.NewProgram(m, tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen())
	result, err := program.Run()
	if err != nil {
		return "", err
	}
	return result.(model).chosen, nil
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

func terminal() (*os.File, *os.File, error) {
	if runtime.GOOS == "windows" {
		if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stderr.Fd()) {
			return nil, nil, fmt.Errorf("interactive terminal unavailable")
		}
		return os.Stdin, os.Stderr, nil
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("interactive terminal unavailable: %w", err)
	}
	return tty, tty, nil
}

// highlight marks the first query word's subsequence inside the repository name.
func highlight(name, query string, color bool) string {
	words := strings.Fields(query)
	if len(words) == 0 || !color {
		return name
	}
	target := []rune(strings.ToLower(words[0]))
	if _, ok := scoreSubsequence(words[0], name); !ok {
		return name
	}
	var b strings.Builder
	at := 0
	for _, r := range name {
		if at < len(target) && unicode.ToLower(r) == target[at] {
			b.WriteString("\x1b[1;35m")
			b.WriteRune(r)
			b.WriteString("\x1b[36m")
			at++
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

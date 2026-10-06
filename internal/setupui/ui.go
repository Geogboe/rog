// Package setupui implements rog's guided configuration wizard.
package setupui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

var setupQuestions = []string{"Include WSL?", "Which project roots should rog use?", "Save these settings?", "Run a full project scan now?"}

var ErrCancelled = errors.New("setup cancelled")
var ErrTerminal = errors.New("rog setup needs an interactive terminal; use rog init for a starter config")

type DiscoverFunc func(context.Context, []setup.SearchRoot, []string, func(string, string, int, int)) setup.DiscoveryResult
type Result struct {
	Config    *config.Config
	ScanNow   bool
	Cancelled bool
}
type discoveryDone struct {
	result      setup.DiscoveryResult
	suggestions []setup.RootSuggestion
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
	ctx                                 context.Context
	config                              *config.Config
	path                                string
	local, external                     []setup.SearchRoot
	selected                            map[string]bool
	discover                            DiscoverFunc
	step, cursor, width, height, scroll int
	color, busy                         bool
	discovered                          bool
	result                              setup.DiscoveryResult
	suggestions                         []setup.RootSuggestion
	progress                            *progressState
	status                              string
	addingLocation                      bool
	locationInput, inputDistro          string
	editIndex, inputDepth               int
	previewAt                           time.Time
	original, preview                   []byte
	finished                            bool
	out                                 Result
}

func Run(ctx context.Context, path string, cfg *config.Config, local, external []setup.SearchRoot, discover DiscoverFunc) (Result, error) {
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
	original, err := readSetupFile(path)
	if err != nil {
		return Result{}, err
	}
	m := newModel(ctx, path, cfg, local, external, discover, original)
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
func addedSearchRoot(value string) (setup.SearchRoot, bool) {
	if runtime.GOOS != "windows" && setup.IsWindowsDrivePath(value) {
		trimmed := strings.TrimRight(value, `\/`)
		name := trimmed
		if at := strings.LastIndexAny(trimmed, `\/`); at >= 0 {
			name = trimmed[at+1:]
		}
		if name == "" || strings.HasSuffix(name, ":") {
			name = "drive-" + strings.ToUpper(value[:1])
		}
		return setup.SearchRoot{Name: name, Path: value, Windows: true}, true
	}
	if filepath.IsAbs(value) {
		name := filepath.Base(filepath.Clean(value))
		if name == string(filepath.Separator) {
			name = "filesystem"
		}
		return setup.SearchRoot{Name: name, Path: value}, true
	}
	return setup.SearchRoot{}, false
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
func dropLastRune(s string) string {
	if s == "" {
		return s
	}
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
}

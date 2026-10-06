package setupui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/setup"
	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"
)

func readSetupFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

func newModel(ctx context.Context, file string, cfg *config.Config, local, external []setup.SearchRoot, discover DiscoverFunc, original []byte) model {
	m := model{ctx: ctx, path: file, config: cloneConfig(cfg), local: local, discover: discover,
		selected: map[string]bool{}, width: 80, height: 24, color: supportsColor(), original: original, editIndex: -1}
	// Only actual saved roots are preloaded; default sample roots are not user choices.
	if original == nil {
		m.config.Roots = nil
	}
	for _, r := range external {
		if r.WSL {
			duplicate := false
			for _, old := range m.external {
				if old.WSL && old.Distro == r.Distro {
					duplicate = true
				}
			}
			if duplicate {
				continue
			}
			r.Path = "/"
		}
		m.external = append(m.external, r)
	}
	for _, r := range m.config.Roots {
		m.suggestions = append(m.suggestions, setup.RootSuggestion{Root: r, Selected: true, Existing: true})
		if r.WSL {
			m.selected[r.WSLDistro] = true
		}
	}
	if len(m.distros()) == 0 {
		m.step = 1
	}
	return m
}

func (m model) distros() []string {
	var out []string
	for _, r := range m.external {
		if r.WSL {
			out = append(out, r.Distro)
		}
	}
	return out
}

func (m model) draftRoots() []config.Root {
	var out []config.Root
	for _, s := range m.suggestions {
		if s.Selected && (!s.Root.WSL || m.selected[s.Root.WSLDistro]) {
			out = append(out, s.Root)
		}
	}
	return out
}

func (m *model) prepareReview() error {
	m.preview = nil // Failed review generation must invalidate any prior proposal.
	next := cloneConfig(m.config)
	next.Roots = m.draftRoots()
	if err := setup.ValidateSetupConfig(next); err != nil {
		return err
	}
	// This wizard owns roots only. Preserve all other YAML exactly as settings,
	// rather than persisting Load's environment overrides or inferred defaults.
	if m.original != nil {
		next.GlobalExcludes, next.Report = nil, nil
		var raw config.Config
		if err := yaml.Unmarshal(m.original, &raw); err != nil {
			return err
		}
		for i, root := range next.Roots {
			for j, old := range m.config.Roots {
				if rootKey(root) == rootKey(old) && root.Name == old.Name && j < len(raw.Roots) {
					next.Roots[i].Path = raw.Roots[j].Path
				}
			}
		}
	}
	data, err := setup.PreviewConfig(m.original, next)
	if err != nil {
		return err
	}
	m.preview = data
	m.previewAt = time.Now()
	m.step, m.cursor, m.scroll = 2, 0, 0
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case setupContextDone:
		m.finished = true
		m.out.Cancelled = m.step != 3
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width, m.height = x.Width, x.Height
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
		m.discovered = true
		// Merge recommendations without replacing manual choices or removed rows.
		for _, recommendation := range x.suggestions {
			found := false
			for i := range m.suggestions {
				if rootKey(m.suggestions[i].Root) == rootKey(recommendation.Root) {
					m.suggestions[i].Count = recommendation.Count
					m.suggestions[i].MaxDepth = recommendation.Root.MaxDepth
					found = true
					break
				}
			}
			if !found {
				base := recommendation.Root.Name
				for suffix := 2; ; suffix++ {
					taken := false
					for _, old := range m.suggestions {
						if strings.EqualFold(old.Root.Name, recommendation.Root.Name) {
							taken = true
						}
					}
					if !taken {
						break
					}
					recommendation.Root.Name = fmt.Sprintf("%s-%d", base, suffix)
				}
				m.suggestions = append(m.suggestions, recommendation)
			}
		}
		m.cursor, m.scroll = 0, 0
		m.status = fmt.Sprintf("Found %d repository locations pending Git validation. Accept or reject recommendations with Space; adjust depth with +/−.", len(m.result.Candidates))
	case applyMessage:
		return x.model, nil
	case tea.KeyMsg:
		key := x.String()
		if m.step == 3 {
			if key == "s" || key == "y" {
				m.out.ScanNow = true
				m.finished = true
				return m, tea.Quit
			}
			if key == "n" || key == "r" || key == "q" || key == "esc" || key == "ctrl+c" {
				m.finished = true
				return m, tea.Quit
			}
			return m, nil
		}
		if key == "ctrl+c" {
			m.out.Cancelled = true
			m.finished = true
			return m, tea.Quit
		}
		if m.busy {
			return m, nil
		}
		if m.addingLocation {
			return m.updateLocation(x), nil
		}
		switch key {
		case "q", "esc":
			m.out.Cancelled = true
			m.finished = true
			return m, tea.Quit
		case "b", "left", "shift+tab":
			if m.step == 2 {
				m.step = 1
			} else if m.step == 1 && len(m.distros()) > 0 {
				m.step = 0
			}
			m.cursor, m.scroll, m.status = 0, 0, ""
		case "enter", "right", "tab":
			if m.step == 0 {
				m.step, m.cursor = 1, 0
			} else if m.step == 1 {
				if err := m.prepareReview(); err != nil {
					m.status = err.Error() + ". Add roots with a or discover with d."
				}
			}
		case "up", "k":
			if m.step == 2 {
				m.scroll = max(0, m.scroll-1)
			} else {
				m.cursor = max(0, m.cursor-1)
			}
		case "down", "j":
			if m.step == 2 {
				m.scroll++
			} else {
				count := len(m.suggestions)
				if m.step == 0 {
					count = len(m.distros())
				}
				m.cursor = min(max(0, count-1), m.cursor+1)
			}
		case "pgup":
			m.scroll = max(0, m.scroll-5)
		case "pgdown":
			m.scroll += 5
		case " ":
			if m.step == 0 && m.cursor < len(m.distros()) {
				distro := m.distros()[m.cursor]
				m.selected[distro] = !m.selected[distro]
			} else if m.step == 1 && m.cursor < len(m.suggestions) {
				r := m.suggestions[m.cursor].Root
				if r.WSL && !m.selected[r.WSLDistro] {
					m.status = "Select this distribution in the WSL step first."
				} else {
					m.suggestions[m.cursor].Selected = !m.suggestions[m.cursor].Selected
				}
			}
		case "n":
			if m.step == 0 {
				for _, d := range m.distros() {
					m.selected[d] = false
				}
				m.step, m.cursor = 1, 0
			}
		case "+", "=", "-":
			if m.step == 1 && m.cursor < len(m.suggestions) {
				delta := 1
				if key == "-" {
					delta = -1
				}
				m.suggestions[m.cursor].Root.MaxDepth = max(1, m.suggestions[m.cursor].Root.MaxDepth+delta)
			}
		case "a", "e":
			if m.step == 1 {
				m.addingLocation, m.locationInput, m.inputDistro, m.editIndex, m.inputDepth = true, "", "", -1, 4
				if key == "e" {
					if m.cursor >= len(m.suggestions) {
						m.addingLocation = false
						break
					}
					r := m.suggestions[m.cursor].Root
					m.editIndex, m.locationInput, m.inputDistro, m.inputDepth = m.cursor, r.Path, r.WSLDistro, r.MaxDepth
				}
			}
		case "delete", "backspace":
			if m.step == 1 && m.cursor < len(m.suggestions) {
				m.suggestions[m.cursor].Selected = false
			}
		case "d":
			if m.step == 1 {
				m.busy = true
				m.progress = &progressState{text: "Discovering repository locations…"}
				m.status = m.progress.text
				return m, m.startDiscovery()
			}
		case "y", "Y":
			if m.step == 2 {
				m.busy = true
				return m, m.apply()
			}
		}
	}
	return m, nil
}

func rootKey(r config.Root) string {
	// Normalize within the filesystem owner; distributions remain distinct.
	owner := "native"
	value := r.Path
	if r.WSL {
		owner = "wsl:" + r.WSLDistro
	} else if r.Windows || setup.IsWindowsDrivePath(value) || runtime.GOOS == "windows" {
		owner = "windows"
		value = strings.ToLower(strings.ReplaceAll(value, "\\", "/"))
	}
	return owner + "|" + path.Clean(value)
}

func (m model) updateLocation(key tea.KeyMsg) model {
	switch key.String() {
	case "esc":
		m.addingLocation = false
		m.status = ""
	case "backspace":
		m.locationInput = dropLastRune(m.locationInput)
	case "tab":
		sources := []string{""}
		for _, d := range m.distros() {
			if m.selected[d] {
				sources = append(sources, d)
			}
		}
		for i, d := range sources {
			if d == m.inputDistro {
				m.inputDistro = sources[(i+1)%len(sources)]
				break
			}
		}
	case "enter":
		value := strings.TrimSpace(m.locationInput)
		sr, ok := addedSearchRoot(value)
		if m.inputDistro != "" {
			ok = path.IsAbs(value) && m.selected[m.inputDistro]
			sr = setup.SearchRoot{Name: path.Base(path.Clean(value)), Path: value, WSL: true, Distro: m.inputDistro}
		}
		if !ok {
			m.status = "Enter an absolute path for the chosen environment (for example /home/me/dev or C:\\dev)."
			return m
		}
		r := config.Root{Name: sr.Name, Path: sr.Path, WSL: sr.WSL, WSLDistro: sr.Distro, Windows: sr.Windows, MaxDepth: m.inputDepth}
		if r.Name == "/" || r.Name == "." {
			r.Name = "filesystem"
		}
		if m.editIndex >= 0 {
			old := m.suggestions[m.editIndex].Root
			r.Name, r.Exclude = old.Name, old.Exclude
		}
		for i, s := range m.suggestions {
			if i != m.editIndex && rootKey(s.Root) == rootKey(r) {
				m.status = "This root is already listed; select or edit that row."
				return m
			}
		}
		base := r.Name
		for suffix := 2; ; suffix++ {
			taken := false
			for i, s := range m.suggestions {
				if i != m.editIndex && strings.EqualFold(s.Root.Name, r.Name) {
					taken = true
				}
			}
			if !taken {
				break
			}
			r.Name = fmt.Sprintf("%s-%d", base, suffix)
		}
		suggestion := setup.RootSuggestion{Root: r, Selected: true}
		if m.editIndex >= 0 {
			m.suggestions[m.editIndex] = suggestion
		} else {
			m.suggestions = append(m.suggestions, suggestion)
			m.cursor = len(m.suggestions) - 1
		}
		m.addingLocation, m.status = false, ""
	default:
		if key.Type == tea.KeyRunes {
			m.locationInput += string(key.Runes)
		}
	}
	return m
}

func (m model) startDiscovery() tea.Cmd {
	roots := append([]setup.SearchRoot(nil), m.local...)
	for _, r := range m.external {
		if !r.WSL || m.selected[r.Distro] {
			roots = append(roots, r)
		}
	}
	// Preserve reverse Windows discovery from WSL, including newly entered roots.
	for _, r := range m.draftRoots() {
		if r.Windows {
			roots = append(roots, setup.SearchRoot{Name: r.Name, Path: r.Path, Windows: true})
		}
	}
	return tea.Batch(nextProgress(), func() tea.Msg {
		result := m.discover(m.ctx, roots, m.config.GlobalExcludes, func(root, name string, done, total int) {
			m.progress.mu.Lock()
			defer m.progress.mu.Unlock()
			m.progress.text = fmt.Sprintf("%s · %s", root, name)
		})
		return discoveryDone{result: result, suggestions: setup.Suggestions(result.Candidates, m.draftRoots())}
	})
}

func (m model) apply() tea.Cmd {
	return func() tea.Msg {
		m.busy = false
		current, err := readSetupFile(m.path)
		if err != nil {
			m.status = "Setup was not applied: " + err.Error()
			return applyMessage{m}
		}
		if !bytes.Equal(current, m.original) || (current == nil) != (m.original == nil) {
			latest := config.DefaultConfig()
			latest.Roots = nil
			if current != nil {
				if err := yaml.Unmarshal(current, latest); err != nil {
					m.status = "Config changed and cannot be parsed; nothing was saved: " + err.Error()
					return applyMessage{m}
				}
			}
			m.config, m.original = latest, current
			if err := m.prepareReview(); err != nil {
				m.status = "Config changed, but the refreshed proposal is invalid: " + err.Error() + ". Correct the config or go back to edit roots; nothing was saved."
				return applyMessage{m}
			}
			m.status = "Config changed while setup was open. Review the refreshed proposal and press y again to save."
			return applyMessage{m}
		}
		if len(m.preview) == 0 {
			m.status = "Review the proposal before saving."
			return applyMessage{m}
		}
		backup, err := setup.ApplyConfig(m.path, m.preview, m.previewAt)
		if err != nil {
			m.status = "Setup was not applied: " + err.Error()
			return applyMessage{m}
		}
		m.out.Config = cloneConfig(m.config)
		m.out.Config.Roots = m.draftRoots()
		m.step, m.scroll = 3, 0
		m.status = "Configuration saved to " + m.path + "."
		if backup != "" {
			m.status += " Backup: " + backup + "."
		}
		return applyMessage{m}
	}
}

func (m model) View() string {
	if m.width <= 0 {
		m.width = 80
	}
	var b strings.Builder
	labels := []string{"WSL · 1/3", "ROOTS · 2/3", "REVIEW · 3/3", "SAVED"}
	fmt.Fprintf(&b, "ROG SETUP · %s\n", labels[m.step])
	writeRule(&b, m.width)
	fmt.Fprintf(&b, "\n%s\n\n", setupQuestions[m.step])
	if m.step < 3 {
		writeWrapped(&b, "Setup will save your agreed settings to "+m.path+". Nothing is saved until you confirm.", m.width)
		b.WriteString("\n\n")
	}
	primary, hint := "", ""
	switch m.step {
	case 0:
		b.WriteString("Select distributions to include. Discovery may start selected distributions.\nDeselecting a distribution removes its roots from the proposed config.\n\n")
		start, end := listWindow(len(m.distros()), m.cursor, m.visibleRows(1))
		for i := start; i < end; i++ {
			mark, cursor := " ", " "
			if m.selected[m.distros()[i]] {
				mark = "✓"
			}
			if i == m.cursor {
				cursor = ">"
			}
			fmt.Fprintf(&b, "%s [%s] %s\n", cursor, mark, m.distros()[i])
		}
		primary, hint = "Enter: continue with selected distributions", "↑/↓ move · Space toggle · n no WSL · Ctrl+C cancel"
	case 1:
		b.WriteString("Add project roots and choose how far below each root to search.\nDepth 1 finds dev/repo; depth 2 also finds dev/team/repo.\n\n")
		if m.addingLocation {
			source := "native (Windows drive paths use Windows)"
			if m.inputDistro != "" {
				source = "WSL " + m.inputDistro
			}
			fmt.Fprintf(&b, "\nEnvironment: %s\nRoot path: %s_\nDepth: %d (adjust after adding)\n", source, m.locationInput, m.inputDepth)
			primary, hint = "Enter: accept root path", "Tab choose environment · Esc cancel entry · Ctrl+C cancel setup"
			break
		}
		start, end := listWindow(len(m.suggestions), m.cursor, m.visibleRows(3))
		for i := start; i < end; i++ {
			s := m.suggestions[i]
			mark, cursor := " ", " "
			if s.Selected && (!s.Root.WSL || m.selected[s.Root.WSLDistro]) {
				mark = "✓"
			}
			if i == m.cursor {
				cursor = ">"
			}
			source := "native"
			if s.Root.WSL {
				source = "WSL " + s.Root.WSLDistro
			}
			if s.Root.Windows {
				source = "Windows"
			}
			fmt.Fprintf(&b, "%s [%s] %s · %s · depth %d\n", cursor, mark, s.Root.Name, source, s.Root.MaxDepth)
			writeIndentedWrapped(&b, "    ", s.Root.Path, m.width)
			if m.discovered {
				count := setup.CountCoveredCandidates(m.result.Candidates, []config.Root{s.Root}, m.config.GlobalExcludes)
				fmt.Fprintf(&b, "    %d repository locations within this depth, pending validation\n", count)
			} else {
				b.WriteString("    Discovery has not checked this root yet.\n")
			}
			if s.MaxDepth > s.Root.MaxDepth {
				fmt.Fprintf(&b, "    Recommended depth: %d\n", s.MaxDepth)
			}
		}
		if len(m.draftRoots()) == 0 {
			b.WriteString("No roots selected. Add roots, or press d to discover project roots on this computer.\n")
		}
		b.WriteString("Discovery finds locations only; Git information is collected by the optional full scan after saving.\n")
		primary, hint = "Enter: accept selected roots and review settings", "a add · e edit · Space select/remove · +/− depth · d discover · b back"

	case 2:
		if len(m.result.Warnings) > 0 {
			fmt.Fprintf(&b, "Partial discovery: %d warnings (scroll to review).\n\n", len(m.result.Warnings))
		}
		b.WriteString("ROOTS BEFORE\n")
		writeRootPreview(&b, m.config.Roots, m.width)
		b.WriteString("\nROOTS AFTER\n")
		writeRootPreview(&b, m.draftRoots(), m.width)
		b.WriteString("\nSAVE DETAILS\n")
		writeIndentedWrapped(&b, "Config: ", m.path, m.width)
		backup := "No existing config; no backup needed."
		if m.original != nil {
			backup = filepath.Join(filepath.Dir(m.path), "setup-history", m.previewAt.Local().Format("20060102-150405.000000000")+".yml")
		}
		writeIndentedWrapped(&b, "Backup: ", backup, m.width)
		b.WriteString("Keeps five backup revisions. Exclusions, report settings, and unrelated YAML are preserved.\n")
		primary, hint = "y: save this configuration", "↑/↓ review · b edit roots · Ctrl+C cancel"
	case 3:
		primary, hint = "s or y: run a full project scan", "n or r: finish without scanning"
	}
	for _, warning := range m.result.Warnings {
		writeIndentedWrapped(&b, "! Partial discovery: ", warning, m.width)
		b.WriteByte('\n')
	}
	if m.status != "" {
		b.WriteByte('\n')
		writeWrapped(&b, m.status, m.width)
		b.WriteByte('\n')
	}
	if m.busy {
		primary, hint = "Discovery is running", "Ctrl+C cancel setup without saving"
		if m.step == 2 {
			primary, hint = "Saving reviewed settings…", ""
		}
	}
	b.WriteString("\nNEXT\n")
	writeIndentedWrapped(&b, "  › ", primary, m.width)
	writeIndentedWrapped(&b, "    ", hint, m.width)
	return fitSetupViewport(strings.TrimSuffix(b.String(), "\n"), m.width, m.color, m.height, m.scroll)
}

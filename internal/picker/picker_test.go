package picker

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestPickerFilteringAndSelection(t *testing.T) {
	m := newModel([]Item{{ID: "/one/rog", Name: "rog", Root: "one"}, {ID: "/two/rog", Name: "rog", Root: "two"}})
	for _, r := range []rune("two") {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(model)
	}
	if len(m.matches) != 1 || m.matches[0].item.ID != "/two/rog" {
		t.Fatalf("wrong matches: %+v", m.matches)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(model).chosen != "/two/rog" {
		t.Fatal("wrong selected ID")
	}
}

func TestPickerCancelAndSanitize(t *testing.T) {
	m := newModel([]Item{{ID: "safe", Name: "escape\x1b[31m", Root: "root"}})
	if strings.Contains(m.View(), "\x1b[31m") {
		t.Fatal("untrusted control character reached terminal")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(model).chosen != "" {
		t.Fatal("cancel selected item")
	}
}

func TestMatcherTenThousandEntries(t *testing.T) {
	items := make([]Item, 10000)
	for i := range items {
		items[i] = Item{ID: fmt.Sprint(i), Name: fmt.Sprintf("project-%05d", i)}
	}
	m := newModel(items)
	m.query = []rune("project-09999")
	m.refilter()
	if len(m.matches) != 1 || m.matches[0].item.ID != "9999" {
		t.Fatalf("unexpected matches %d", len(m.matches))
	}
}

func BenchmarkMatcherTenThousandEntries(b *testing.B) {
	items := make([]Item, 10000)
	for i := range items {
		items[i] = Item{ID: fmt.Sprint(i), Name: fmt.Sprintf("project-%05d", i)}
	}
	m := newModel(items)
	m.query = []rune("project-09999")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.refilter()
	}
}

func TestPickerDetailsFollowShortResultList(t *testing.T) {
	m := newModel([]Item{{ID: "/one/service", Name: "service", Path: "/one/service"}, {ID: "/two/service", Name: "service", Path: "/two/service"}})
	m.height = 40
	lines := strings.Split(m.View(), "\n")
	if len(lines) != 9 || lines[6] != "/one/service" {
		t.Fatalf("picker left blank space before details: %q", m.View())
	}
}

func TestPickerColorAndPlainViews(t *testing.T) {
	m := newModel([]Item{{ID: "/one/rog", Name: "rog", Root: "one", Path: "/one/rog", Status: "dirty"}})
	m.query = []rune("ro")
	m.refilter()
	m.width = 40
	plain := m.View()
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("plain view contains ANSI escapes: %q", plain)
	}
	m.color = true
	colored := m.View()
	if !strings.Contains(colored, "\x1b[1;35m") || !strings.Contains(colored, "\x1b[33mdirty") {
		t.Fatalf("colored view lacks match or status color: %q", colored)
	}
	if ansi.Strip(colored) != plain {
		t.Fatalf("color changed visible content:\nplain: %q\ncolor: %q", plain, ansi.Strip(colored))
	}
	for _, line := range strings.Split(colored, "\n") {
		if ansi.StringWidth(line) > m.width {
			t.Fatalf("line exceeds width %d: %q", m.width, line)
		}
	}
}

package picker

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
